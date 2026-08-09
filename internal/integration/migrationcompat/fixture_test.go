//go:build integration

package migrationcompat

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/batch"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/destination"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/lifecycle"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/processor"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/session"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
	termdb "gitlab.flexinfer.ai/libs/fi-fhir/pkg/terminology/db"
)

// ledgerExpectation names one forward-only ledger, the version this binary
// expects of it, and the SQL that reads the version actually applied.
type ledgerExpectation struct {
	name     string
	declared int
	query    string
}

// ledgerExpectations is the full set. Five ledgers keep their version in a
// `*_schema_migrations` table; terminology keeps its own in
// `terminology.schema_version`, which is why the version read is per-ledger SQL
// rather than one generic query.
func ledgerExpectations() []ledgerExpectation {
	return []ledgerExpectation{
		{"submission", processor.SchemaVersion,
			`SELECT coalesce(max(version), 0) FROM integration_submission_schema_migrations`},
		{"session", session.SchemaVersion,
			`SELECT coalesce(max(version), 0) FROM integration_session_schema_migrations`},
		{"lifecycle", lifecycle.SchemaVersion,
			`SELECT coalesce(max(version), 0) FROM integration_lifecycle_schema_migrations`},
		{"batch", batch.SchemaVersion,
			`SELECT coalesce(max(version), 0) FROM integration_batch_schema_migrations`},
		{"destination", destination.SchemaVersion,
			`SELECT coalesce(max(version), 0) FROM integration_destination_schema_migrations`},
		{"terminology", termdb.SchemaVersion,
			`SELECT coalesce(max(version), 0) FROM terminology.schema_version`},
	}
}

// migrateEveryLedger runs all six migrators against one database, in the order
// `fi-fhir serve` runs them. Returning the first error rather than aggregating
// is deliberate: a replica that cannot migrate one ledger must not start.
func migrateEveryLedger(ctx context.Context, db *sql.DB, tenantID string) error {
	submission, err := processor.NewPostgresSubmissionStore(db, processor.PostgresSubmissionConfig{})
	if err != nil {
		return fmt.Errorf("construct submission store: %w", err)
	}
	if err := submission.Migrate(ctx); err != nil {
		return fmt.Errorf("submission: %w", err)
	}

	sessions, err := session.NewPostgresStore(db, session.PostgresConfig{TenantID: tenantID})
	if err != nil {
		return fmt.Errorf("construct session store: %w", err)
	}
	if err := sessions.Migrate(ctx); err != nil {
		return fmt.Errorf("session: %w", err)
	}

	catalog, err := lifecycle.NewPostgresCatalog(db, lifecycle.Config{})
	if err != nil {
		return fmt.Errorf("construct lifecycle catalog: %w", err)
	}
	if err := catalog.Migrate(ctx); err != nil {
		return fmt.Errorf("lifecycle: %w", err)
	}

	batches, err := batch.NewPostgresStore(db, nil)
	if err != nil {
		return fmt.Errorf("construct batch store: %w", err)
	}
	if err := batches.Migrate(ctx); err != nil {
		return fmt.Errorf("batch: %w", err)
	}

	provenance, err := destination.NewPostgresProvenance(db)
	if err != nil {
		return fmt.Errorf("construct destination provenance: %w", err)
	}
	if err := provenance.Migrate(ctx); err != nil {
		return fmt.Errorf("destination: %w", err)
	}

	if _, err := termdb.NewMigrator(db).Initialize(ctx); err != nil {
		return fmt.Errorf("terminology: %w", err)
	}
	return nil
}

// durableFixture is one complete chain through the durable classes plus the
// session workspace, seeded so a dump/restore proof has something to compare.
//
// The three `Guard*` identifiers are a second, deliberately childless copy of
// the rows the immutability assertions mutate. Slice 4.4c added them because
// the chain rows cannot carry those assertions: a canonical event with lineage,
// a receipt with events, and an attempt with an outbox row are each refused by
// a foreign key with every trigger dropped, so an assertion aimed at them stays
// green after the guard it claims to watch is gone (.loom/33 defect D3, proved
// by TestChaosRecovery_RestoreProofAssertionsAreTriggerAttributed). A row with
// no dependents can only be refused by its trigger.
type durableFixture struct {
	ReceiptID string
	EventID   string
	LineageID string
	AttemptID string
	OutboxID  string
	SessionID string
	ExportID  string
	SampleID  string
	// StreamSeq is the fanout log row the prune-floor guard protects. It is
	// seeded at clock_timestamp() rather than at the fixture's frozen instant
	// so it is inside the schema's 24-hour floor
	// (session/migrations/0006_retention_expiry.sql:147-157) and its DELETE
	// therefore raises.
	StreamSeq int64
	// PolicyVersion identifies both retention audit rows.
	PolicyVersion int64

	// Childless targets for the immutability assertions.
	GuardReceiptID string
	GuardEventID   string
	GuardAttemptID string

	PHISentinel string
}

// seedDurableFixture writes one receipt, canonical event, lineage row, queued
// delivery attempt, pending outbox row, and identity decision with raw SQL; one
// session with a retained sample and an export through the session package's
// own writer; the 4.1e surface — a session sample, a fanout-log envelope, a
// retention policy and both of its audit ledgers; and a childless copy of each
// row the immutability assertions mutate.
//
// The processor chain is raw SQL on purpose: the properties under test are
// "every row survives a dump/restore" and "the delivery worker resumes from the
// restored state". Both are statements about rows and schema objects, and every
// CHECK and foreign key still has to be satisfied for the insert to land. The
// export goes through the real writer because one assertion *is* about the
// writer: the DEFAULTs slice 4.4a adds must not be able to mask a live-path
// regression.
func seedDurableFixture(ctx context.Context, t *testing.T, db *sql.DB) durableFixture {
	t.Helper()
	now := time.Date(2026, 8, 8, 9, 0, 0, 0, time.UTC)
	fixture := durableFixture{
		ReceiptID:      "rcpt-compat-1",
		EventID:        "evt-compat-1",
		LineageID:      "lin-compat-1",
		AttemptID:      "att-compat-1",
		OutboxID:       "out-compat-1",
		SessionID:      "",
		SampleID:       "smp-compat-1",
		PolicyVersion:  1,
		GuardReceiptID: "rcpt-compat-guard",
		GuardEventID:   "evt-compat-guard",
		GuardAttemptID: "att-compat-guard",
		PHISentinel:    "PHI-SENTINEL-8f2c41",
	}

	exec := func(label, query string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, query, args...); err != nil {
			t.Fatalf("seed %s: %v", label, err)
		}
	}

	exec("receipt", `
		INSERT INTO integration_receipts (
			tenant_id, receipt_id, idempotency_key, request_fingerprint,
			integration_revision, status, recorded_at, correlation_id,
			raw_retention_mode, principal_json, reason, result_json)
		VALUES ($1, $2, 'idem-compat-1', 'fingerprint-compat-1',
			'{"artifact_id":"integration-compat","revision_id":"integration-compat-1"}'::jsonb,
			'accepted', $3, 'corr-compat-1', 'ephemeral',
			'{"id":"svc-compat","kind":"service","auth_method":"oidc"}'::jsonb,
			'', '{"status":"accepted"}'::jsonb)
	`, compatTenantID, fixture.ReceiptID, now)

	exec("canonical event", `
		INSERT INTO integration_canonical_events (
			tenant_id, event_id, receipt_id, event_type, source_message_id,
			correlation_id, classification, payload_json, recorded_at)
		VALUES ($1, $2, $3, 'ADT_A01', 'msg-compat-1', 'corr-compat-1', 'phi',
			jsonb_build_object('patient_name', $4::text), $5)
	`, compatTenantID, fixture.EventID, fixture.ReceiptID, fixture.PHISentinel, now)

	exec("lineage", `
		INSERT INTO integration_message_lineage (
			tenant_id, lineage_id, receipt_id, event_id, trace_id, correlation_id,
			source_message_id, artifact_revisions_json, routes_json,
			diagnostics_json, recorded_at)
		VALUES ($1, $2, $3, $4, 'trace-compat-1', 'corr-compat-1', 'msg-compat-1',
			'{"workflow":"wf-compat-1"}'::jsonb, '["route-compat"]'::jsonb,
			'[]'::jsonb, $5)
	`, compatTenantID, fixture.LineageID, fixture.ReceiptID, fixture.EventID, now)

	exec("delivery attempt", `
		INSERT INTO integration_delivery_attempts (
			tenant_id, attempt_id, receipt_id, event_id, trace_id,
			destination_revision_json, route_name, action_id, status,
			attempt_count, recorded_at, scheduled_at)
		VALUES ($1, $2, $3, $4, 'trace-compat-1',
			jsonb_build_object(
				'artifact_id', 'dest-compat',
				'revision_id', 'dest-compat-1',
				'class', 'production',
				'digest', 'sha256:' || repeat('a', 64)),
			'route-compat', 'action-compat', 'queued', 1, $5, $5)
	`, compatTenantID, fixture.AttemptID, fixture.ReceiptID, fixture.EventID, now)

	exec("outbox", `
		INSERT INTO integration_delivery_outbox (
			tenant_id, outbox_id, attempt_id, topic, status, payload_json,
			created_at, scheduled_at, updated_at)
		VALUES ($1, $2, $3, 'integration.delivery.v1', 'pending',
			'{"attempt":"att-compat-1"}'::jsonb, $4, $4, $4)
	`, compatTenantID, fixture.OutboxID, fixture.AttemptID, now)

	exec("identity decision", `
		INSERT INTO integration_delivery_identity_decisions (
			tenant_id, attempt_id, decision, identity_mode, principal_subject,
			principal_auth_method, granted_role, destination_artifact_id,
			destination_revision_id, destination_class,
			destination_digest_verified, destination_endpoint_advisory, decided_at)
		VALUES ($1, $2, 'authorized', 'strict', 'svc-compat', 'oidc',
			'integration.deliver', 'dest-compat', 'dest-compat-1', 'production',
			'sha256:' || repeat('a', 64), 'https://destination.example/ingest', $3)
	`, compatTenantID, fixture.AttemptID, now)

	// ---- Childless copies for the immutability assertions -------------------
	//
	// Identical in kind to the chain rows above and referenced by nothing, so a
	// refusal can only have come from the guard. See durableFixture's comment.
	exec("guard receipt", `
		INSERT INTO integration_receipts (
			tenant_id, receipt_id, idempotency_key, request_fingerprint,
			integration_revision, status, recorded_at, correlation_id,
			raw_retention_mode, principal_json, reason, result_json)
		VALUES ($1, $2, 'idem-compat-guard', 'fingerprint-compat-guard',
			'{"artifact_id":"integration-compat","revision_id":"integration-compat-1"}'::jsonb,
			'accepted', $3, 'corr-compat-guard', 'ephemeral',
			'{"id":"svc-compat","kind":"service","auth_method":"oidc"}'::jsonb,
			'', '{"status":"accepted"}'::jsonb)
	`, compatTenantID, fixture.GuardReceiptID, now)

	exec("guard canonical event", `
		INSERT INTO integration_canonical_events (
			tenant_id, event_id, receipt_id, event_type, source_message_id,
			correlation_id, classification, payload_json, recorded_at)
		VALUES ($1, $2, $3, 'ADT_A01', 'msg-compat-guard', 'corr-compat-1', 'phi',
			jsonb_build_object('patient_name', $4::text), $5)
	`, compatTenantID, fixture.GuardEventID, fixture.ReceiptID, fixture.PHISentinel, now)

	exec("guard delivery attempt", `
		INSERT INTO integration_delivery_attempts (
			tenant_id, attempt_id, receipt_id, event_id, trace_id,
			destination_revision_json, route_name, action_id, status,
			attempt_count, recorded_at, scheduled_at)
		VALUES ($1, $2, $3, $4, 'trace-compat-guard',
			jsonb_build_object(
				'artifact_id', 'dest-compat',
				'revision_id', 'dest-compat-1',
				'class', 'production',
				'digest', 'sha256:' || repeat('a', 64)),
			'route-compat', 'action-compat', 'queued', 1, $5, $5)
	`, compatTenantID, fixture.GuardAttemptID, fixture.ReceiptID, fixture.EventID, now)

	// ---- The 4.1e surface ---------------------------------------------------
	//
	// Slice 4.4a's durableClasses omitted every table 4.1e added, so five of the
	// newest immutability triggers were never exercised after a restore
	// (.loom/33 defect D3). Seeding them is what makes the round-trip cover the
	// retention control at all.
	exec("retention policy", `
		INSERT INTO integration_retention_policies (
			tenant_id, policy_version, canonical_event_retain_seconds,
			session_sample_retain_seconds, session_export_retain_seconds,
			stream_event_retain_seconds, principal_json, reason,
			document_digest, updated_at)
		VALUES ($1, $2, 2592000, 604800, 2592000, 172800,
			'{"id":"privacy-officer-compat","kind":"human","auth_method":"oidc"}'::jsonb,
			'migration compatibility round-trip fixture',
			'sha256:' || repeat('b', 64), $3)
	`, compatTenantID, fixture.PolicyVersion, now)

	exec("retention policy audit", `
		INSERT INTO integration_retention_policy_audit (
			tenant_id, policy_version, canonical_event_retain_seconds,
			session_sample_retain_seconds, session_export_retain_seconds,
			stream_event_retain_seconds, principal_json, reason,
			document_digest, recorded_at)
		VALUES ($1, $2, 2592000, 604800, 2592000, 172800,
			'{"id":"privacy-officer-compat","kind":"human","auth_method":"oidc"}'::jsonb,
			'migration compatibility round-trip fixture',
			'sha256:' || repeat('b', 64), $3)
	`, compatTenantID, fixture.PolicyVersion, now)

	// A purged record's audit row. record_id points at a canonical event that
	// no longer needs to exist — the audit is deliberately FK-free so the
	// evidence outlives the record (0005_retention_expiry.sql:95-101).
	exec("retention purge audit", `
		INSERT INTO integration_retention_purge_audit (
			tenant_id, record_class, record_id, policy_version, purge_mode,
			purge_after, purged_at)
		VALUES ($1, 'canonical_event', 'evt-compat-purged', $2, 'tombstone', $3, $3)
	`, compatTenantID, fixture.PolicyVersion, now)

	// Session workspace, through the real writer.
	store, err := session.NewPostgresStore(db, session.PostgresConfig{TenantID: compatTenantID})
	if err != nil {
		t.Fatalf("construct session store for fixture: %v", err)
	}
	created, err := store.CreateSession(ctx, session.CreateSessionRequest{Name: "migration compatibility fixture"})
	if err != nil {
		t.Fatalf("seed session: %v", err)
	}
	fixture.SessionID = created.ID

	// The sample goes through the real writer. Unlike the processor chain, this
	// row's record_json is a serialized domain record that ExportBundle decodes
	// on its way past ListSamples, so a hand-written JSON object would fail the
	// decode rather than exercise the restore.
	sample, err := store.AddSample(ctx, fixture.SessionID, session.AddSampleRequest{
		Name:      "migration compatibility fixture sample",
		Format:    events.FormatHL7v2,
		Raw:       "MSH|^~\\&|COMPAT|" + fixture.PHISentinel,
		PHIPolicy: session.PHIPolicyRedact,
	})
	if err != nil {
		t.Fatalf("seed session sample: %v", err)
	}
	fixture.SampleID = sample.ID

	// The fanout log envelope is stamped at clock_timestamp() so the restored
	// row is inside the schema's 24-hour prune floor. A frozen 2026-08-08
	// timestamp would age past the floor and the prune guard would permit the
	// delete, which would make that assertion vacuous on any day but one.
	if err := db.QueryRowContext(ctx, `
		INSERT INTO integration_session_stream_events (
			tenant_id, event_id, session_id, run_id, event_type, created_at)
		VALUES ($1, 'stream-compat-1', $2, '', 'session.created', clock_timestamp())
		RETURNING seq
	`, compatTenantID, fixture.SessionID).Scan(&fixture.StreamSeq); err != nil {
		t.Fatalf("seed session stream event: %v", err)
	}

	bundle, err := store.ExportBundle(ctx, session.ExportRequest{
		SessionID: fixture.SessionID,
		Principal: attributedPrincipal(),
		Reason:    "migration compatibility round-trip fixture",
	})
	if err != nil {
		t.Fatalf("seed session export: %v", err)
	}
	fixture.ExportID = bundle.ID

	return fixture
}

// attributedPrincipal is a fully attributed caller: the opposite of the
// unattributed sentinel the rollback DEFAULT produces. An assertion that the
// live path still records this is what stops the DEFAULT from masking a
// regression in the current writer.
func attributedPrincipal() integration.Principal {
	return integration.Principal{
		ID:         "privacy-officer-compat",
		Kind:       integration.PrincipalKindHuman,
		AuthMethod: "oidc",
		Roles:      []string{"integration.phi.export"},
	}
}

// seedTerminologyAtVersion brings the terminology schema to an older version so
// the upgrade path can be exercised, rather than only fresh-install and no-op.
//
// It applies the same exported migration bodies Initialize applies, in the same
// order, stopping early. Reproducing the SQL here instead would make the test
// assert against a copy of the schema rather than the schema.
func seedTerminologyAtVersion(ctx context.Context, t *testing.T, db *sql.DB, version int) {
	t.Helper()
	if version < 1 || version >= termdb.SchemaVersion {
		t.Fatalf("seed version %d must be between 1 and %d (exclusive)", version, termdb.SchemaVersion)
	}

	steps := []struct {
		version int
		body    string
	}{
		{1, termdb.Schema},
		{2, termdb.SchemaV2Migration},
		{3, termdb.SchemaV3Migration},
	}
	for _, step := range steps {
		if step.version > version {
			break
		}
		if _, err := db.ExecContext(ctx, step.body); err != nil {
			t.Fatalf("seed terminology schema v%d: %v", step.version, err)
		}
	}

	applied, err := termdb.NewMigrator(db).CurrentVersion(ctx)
	if err != nil {
		t.Fatalf("read seeded terminology version: %v", err)
	}
	if applied != version {
		t.Fatalf("seeded terminology at version %d, want %d", applied, version)
	}
	if applied >= termdb.SchemaVersion {
		t.Fatalf("seeded terminology at head (%d); the upgrade path would not be exercised", applied)
	}
}
