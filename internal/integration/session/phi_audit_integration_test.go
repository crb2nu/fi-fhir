//go:build integration

package session

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lib/pq"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/authorization"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/delivery"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/processor"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

// TestPhiAudit_PostgresImmutableRecordsAndAttributedExport is Slice 4.1d C1's
// kill-test. It proves, against a live PostgreSQL 16:
//
//  1. every newly guarded durable audit record refuses UPDATE and DELETE from the
//     application role, with row counts unchanged;
//  2. the shipped delivery state machine still advances claim -> retry -> DLQ ->
//     replay, so the column-scoped guards on the state tables did not over-lock;
//  3. an attributed export records the exact verified principal, the reason, and
//     include_raw_payload = false;
//  4. an export without a reason is refused and writes no row;
//  5. includeRawPayload without the integration.phi.export grant is refused and
//     writes no row, and with the grant is allowed and recorded.
//
// The NEGATIVE CONTROL runs assertions 1, 3, 4, and 5 against a second schema
// provisioned from the PRE-migration migration set with the pre-change export
// write. There, the UPDATE/DELETE must SUCCEED and the export must be written
// with no principal and no reason. If the pre-migration schema also raises, this
// test is asserting against the wrong schema and proves nothing.
func TestPhiAudit_PostgresImmutableRecordsAndAttributedExport(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	base := requirePhiAuditPostgres(t)

	// ---- Negative control first: it must disconfirm before the real proof runs.
	legacy := newPhiAuditSchema(t, ctx, base, "phi_audit_pre")
	applyPreMigrationSchema(t, ctx, legacy)
	seedPhiAuditRecords(t, ctx, legacy)
	seedOrphanRecords(t, ctx, legacy)
	legacyCounts := assertGuardedTablesPopulated(t, ctx, legacy, "pre-migration")

	t.Run("negative_control_pre_migration_schema_permits_mutation", func(t *testing.T) {
		for _, mutation := range phiAuditMutations() {
			if _, err := legacy.ExecContext(ctx, mutation.statement); err != nil {
				t.Fatalf("pre-migration schema rejected %q: %v — the negative control "+
					"is asserting against the wrong schema, so the primary proof is vacuous",
					mutation.name, err)
			}
		}
		after := countPhiAuditRows(t, ctx, legacy)
		if after == legacyCounts {
			t.Fatalf("pre-migration mutations changed nothing: before=%+v after=%+v", legacyCounts, after)
		}
		t.Logf("negative control: pre-migration schema accepted every UPDATE/DELETE; "+
			"counts moved from %+v to %+v", legacyCounts, after)
	})

	t.Run("negative_control_pre_migration_export_is_unattributed", func(t *testing.T) {
		columns := exportColumns(t, ctx, legacy)
		for _, absent := range []string{"principal_json", "reason", "include_raw_payload"} {
			if columns[absent] {
				t.Fatalf("pre-migration integration_session_exports already has %q", absent)
			}
		}
		// This is verbatim the pre-change write: no principal, no reason.
		if _, err := legacy.ExecContext(ctx, `
			INSERT INTO integration_session_exports
				(tenant_id, session_id, export_id, exported_at, record_json)
			VALUES ($1, $2, $3, $4, $5)
		`, "tenant-a", legacySessionID, "export-legacy", time.Now().UTC(),
			`{"id":"export-legacy"}`); err != nil {
			t.Fatalf("pre-migration unattributed export rejected: %v", err)
		}
		var rows int
		if err := legacy.QueryRowContext(ctx,
			`SELECT count(*) FROM integration_session_exports`).Scan(&rows); err != nil {
			t.Fatalf("count pre-migration exports: %v", err)
		}
		if rows != 1 {
			t.Fatalf("pre-migration exports = %d, want 1", rows)
		}
		t.Log("negative control: pre-migration export written with no principal and no reason")
	})

	// ---- Primary proof on the fully migrated schema.
	migrated := newPhiAuditSchema(t, ctx, base, "phi_audit_post")
	submissions := applyCurrentSchema(t, ctx, migrated)
	seedPhiAuditRecordsThroughProcessor(t, ctx, migrated, submissions)

	// The state machine runs FIRST for two reasons: with the guards already
	// active it proves the column-scoped triggers did not over-lock, and it is
	// what populates integration_delivery_audit, without which the mutation
	// assertions below would match zero rows and pass vacuously.
	t.Run("delivery_state_machine_still_advances", func(t *testing.T) {
		runDeliveryStateMachine(t, ctx, migrated)
	})

	seedOrphanRecords(t, ctx, migrated)
	beforeCounts := assertGuardedTablesPopulated(t, ctx, migrated, "migrated")

	t.Run("guarded_audit_records_refuse_update_and_delete", func(t *testing.T) {
		for _, mutation := range phiAuditMutations() {
			if _, err := migrated.ExecContext(ctx, mutation.statement); err == nil {
				t.Fatalf("%s succeeded against the migrated schema; the guard is missing", mutation.name)
			} else if !strings.Contains(err.Error(), mutation.wantMessage) {
				t.Fatalf("%s raised %q, want a message containing %q", mutation.name, err, mutation.wantMessage)
			}
		}
		after := countPhiAuditRows(t, ctx, migrated)
		if after != beforeCounts {
			t.Fatalf("guarded tables changed: before=%+v after=%+v", beforeCounts, after)
		}
		t.Logf("every guarded mutation raised; counts unchanged at %+v", after)
	})

	store := newAttributedExportStore(t, ctx, migrated)
	sess, err := store.CreateSession(ctx, CreateSessionRequest{Name: "PHI export attribution"})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if _, err := store.AddSample(ctx, sess.ID, AddSampleRequest{
		Name: "admit", Format: events.FormatHL7v2, Raw: rawSessionPHI, PHIPolicy: PHIPolicyRetain,
	}); err != nil {
		t.Fatalf("AddSample: %v", err)
	}

	auditor := integration.Principal{
		ID: "privacy-officer-7", Kind: integration.PrincipalKindHuman,
		AuthMethod: "oidc", Roles: []string{"graphql:operator"},
	}

	t.Run("attributed_export_records_actor_reason_and_disclosure", func(t *testing.T) {
		bundle, err := store.ExportBundle(ctx, ExportRequest{
			SessionID: sess.ID, Principal: auditor, Reason: "HIPAA access review 2026-Q3",
		})
		if err != nil {
			t.Fatalf("ExportBundle: %v", err)
		}
		row := readExportRow(t, ctx, migrated, bundle.ID)
		if row.principalID != auditor.ID || row.principalKind != string(auditor.Kind) ||
			row.principalAuth != auditor.AuthMethod {
			t.Fatalf("export principal = %+v, want %+v", row, auditor)
		}
		if row.reason != "HIPAA access review 2026-Q3" {
			t.Fatalf("export reason = %q", row.reason)
		}
		if row.includeRaw {
			t.Fatal("default export recorded include_raw_payload = true")
		}
	})

	t.Run("export_without_reason_is_refused_and_writes_nothing", func(t *testing.T) {
		before := countExports(t, ctx, migrated)
		if _, err := store.ExportBundle(ctx, ExportRequest{
			SessionID: sess.ID, Principal: auditor, Reason: "   ",
		}); !errors.Is(err, ErrInvalid) {
			t.Fatalf("ExportBundle(no reason) = %v, want ErrInvalid", err)
		}
		if after := countExports(t, ctx, migrated); after != before {
			t.Fatalf("refused export wrote a row: before=%d after=%d", before, after)
		}
	})

	t.Run("raw_payload_export_requires_the_phi_export_grant", func(t *testing.T) {
		before := countExports(t, ctx, migrated)
		_, err := store.ExportBundle(ctx, ExportRequest{
			SessionID: sess.ID, Principal: auditor,
			Reason: "raw payload review", IncludeRawPayload: true,
		})
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("ungranted raw export = %v, want ErrForbidden", err)
		}
		if !strings.Contains(err.Error(), PHIExportRole) {
			t.Fatalf("refusal did not name the required decision: %v", err)
		}
		if strings.Contains(err.Error(), sess.ID) {
			t.Fatalf("refusal named the inventory rather than the decision: %v", err)
		}
		if after := countExports(t, ctx, migrated); after != before {
			t.Fatalf("refused raw export wrote a row: before=%d after=%d", before, after)
		}

		granted := auditor
		granted.Roles = append(append([]string(nil), auditor.Roles...), PHIExportRole)
		bundle, err := store.ExportBundle(ctx, ExportRequest{
			SessionID: sess.ID, Principal: granted,
			Reason: "raw payload review", IncludeRawPayload: true,
		})
		if err != nil {
			t.Fatalf("granted raw export: %v", err)
		}
		row := readExportRow(t, ctx, migrated, bundle.ID)
		if !row.includeRaw {
			t.Fatal("granted raw export did not record include_raw_payload = true")
		}
		if row.reason != "raw payload review" || row.principalID != granted.ID {
			t.Fatalf("granted raw export attribution = %+v", row)
		}
	})

	t.Run("export_records_are_append_only", func(t *testing.T) {
		before := countExports(t, ctx, migrated)
		if _, err := migrated.ExecContext(ctx,
			`UPDATE integration_session_exports SET reason = 'rewritten'`); err == nil {
			t.Fatal("export reason was rewritable")
		}
		if _, err := migrated.ExecContext(ctx,
			`DELETE FROM integration_session_exports`); err == nil {
			t.Fatal("export record was deletable")
		}
		if after := countExports(t, ctx, migrated); after != before {
			t.Fatalf("export rows changed: before=%d after=%d", before, after)
		}
	})
}

// ---------------------------------------------------------------------------
// Mutations under test
// ---------------------------------------------------------------------------

type phiAuditMutation struct {
	name        string
	statement   string
	wantMessage string
}

// phiAuditMutations enumerates every mutation this slice must block. Each one is
// reachable on the pre-migration schema — the negative control asserts exactly
// that — so the post-migration failure is attributable to the new guard and not
// to pre-existing referential integrity.
//
// DELETE on the tables with dependents (canonical events, receipts, attempts) is
// deliberately aimed at purpose-seeded ORPHAN rows. Aiming it at a row that still
// has dependents would be blocked by the existing ON DELETE RESTRICT foreign keys
// in both schemas, which proves referential integrity, not immutability — and
// referential integrity is not immutability: the last row of a chain, or any row
// whose dependents were removed first, is otherwise freely deletable.
func phiAuditMutations() []phiAuditMutation {
	const appendOnly = "append-only"
	const receiptFrozen = "receipt provenance is immutable"
	const attemptFrozen = "attempt provenance is immutable"
	const undeletable = "cannot be deleted"
	return []phiAuditMutation{
		{"UPDATE integration_delivery_audit", `UPDATE integration_delivery_audit SET reason = 'rewritten'`, appendOnly},
		{"DELETE integration_delivery_audit", `DELETE FROM integration_delivery_audit`, appendOnly},
		{"UPDATE integration_message_lineage", `UPDATE integration_message_lineage SET correlation_id = 'forged'`, appendOnly},
		{"DELETE integration_message_lineage", `DELETE FROM integration_message_lineage`, appendOnly},
		{
			"UPDATE integration_canonical_events payload",
			`UPDATE integration_canonical_events SET payload_json = '{}'::jsonb`,
			appendOnly,
		},
		{
			"DELETE integration_canonical_events (orphan)",
			`DELETE FROM integration_canonical_events WHERE event_id = '` + orphanEventID + `'`,
			appendOnly,
		},
		{"UPDATE integration_receipts principal", `UPDATE integration_receipts SET principal_json = '{}'::jsonb`, receiptFrozen},
		{
			"DELETE integration_receipts (orphan)",
			`DELETE FROM integration_receipts WHERE receipt_id = '` + orphanReceiptID + `'`,
			undeletable,
		},
		{"UPDATE integration_delivery_attempts lineage", `UPDATE integration_delivery_attempts SET trace_id = 'forged-trace'`, attemptFrozen},
		{
			"DELETE integration_delivery_attempts (orphan)",
			`DELETE FROM integration_delivery_attempts WHERE attempt_id = '` + orphanAttemptID + `'`,
			undeletable,
		},
	}
}

// Three INDEPENDENT orphan chains. Sharing one chain would leave each row with a
// dependent and the DELETE would hit a foreign key instead of the guard.
const (
	orphanReceiptID      = "rcpt-orphan-receipt"
	orphanEventReceiptID = "rcpt-orphan-event"
	orphanEventID        = "evt-orphan"
	orphanAttemptChainID = "rcpt-orphan-attempt"
	orphanAttemptEventID = "evt-orphan-attempt"
	orphanAttemptID      = "att-orphan"
)

// seedOrphanRecords inserts one dependent-free row in each of the three tables
// whose real rows are protected by ON DELETE RESTRICT, so the DELETE assertions
// test the immutability trigger rather than a foreign key. INSERT is never
// guarded by this slice, so the same seed works on both schemas.
func seedOrphanRecords(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()
	receipt := func(id string) string {
		return `INSERT INTO integration_receipts (
			tenant_id, receipt_id, idempotency_key, request_fingerprint, integration_revision,
			status, recorded_at, correlation_id, raw_retention_mode, principal_json, reason, result_json
		) VALUES ('tenant-a', '` + id + `', 'idem-` + id + `', 'fingerprint-` + id + `', '{}'::jsonb,
			'accepted', now(), 'corr-` + id + `', 'ephemeral', '{"id":"svc-orphan"}'::jsonb, '', '{}'::jsonb)`
	}
	event := func(id, receiptID string) string {
		return `INSERT INTO integration_canonical_events (
			tenant_id, event_id, receipt_id, event_type, source_message_id,
			correlation_id, classification, payload_json, recorded_at
		) VALUES ('tenant-a', '` + id + `', '` + receiptID + `', 'patient_admit', 'msg-` + id + `',
			'corr-` + id + `', 'phi', '{"mrn":"ORPHAN-SENTINEL"}'::jsonb, now())`
	}
	statements := []string{
		// chain 1: a receipt with no event, lineage, or attempt
		receipt(orphanReceiptID),
		// chain 2: an event with no lineage and no attempt
		receipt(orphanEventReceiptID),
		event(orphanEventID, orphanEventReceiptID),
		// chain 3: an attempt with no outbox, DLQ, or operation
		receipt(orphanAttemptChainID),
		event(orphanAttemptEventID, orphanAttemptChainID),
		`INSERT INTO integration_delivery_attempts (
			tenant_id, attempt_id, receipt_id, event_id, trace_id, destination_revision_json,
			route_name, action_id, status, attempt_count, recorded_at, scheduled_at
		) VALUES ('tenant-a', '` + orphanAttemptID + `', '` + orphanAttemptChainID + `', '` + orphanAttemptEventID + `',
			'trace-orphan', '{}'::jsonb, 'matched', 'send-fhir', 'queued', 1, now(), now())`,
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			t.Fatalf("seed orphan record: %v", err)
		}
	}
}

// assertGuardedTablesPopulated refuses to run the immutability assertions against
// an empty table: an UPDATE that matches zero rows succeeds trivially and would
// make the whole proof vacuous.
func assertGuardedTablesPopulated(t *testing.T, ctx context.Context, db *sql.DB, label string) phiAuditCounts {
	t.Helper()
	counts := countPhiAuditRows(t, ctx, db)
	if counts.Receipts == 0 || counts.Events == 0 || counts.Lineage == 0 ||
		counts.Attempts == 0 || counts.Audit == 0 {
		t.Fatalf("%s schema has an empty guarded table, so the mutation assertions "+
			"would pass vacuously: %+v", label, counts)
	}
	return counts
}

type phiAuditCounts struct {
	Receipts int
	Events   int
	Lineage  int
	Attempts int
	Audit    int
	Traces   string
}

func countPhiAuditRows(t *testing.T, ctx context.Context, db *sql.DB) phiAuditCounts {
	t.Helper()
	var out phiAuditCounts
	if err := db.QueryRowContext(ctx, `
		SELECT
			(SELECT count(*) FROM integration_receipts),
			(SELECT count(*) FROM integration_canonical_events),
			(SELECT count(*) FROM integration_message_lineage),
			(SELECT count(*) FROM integration_delivery_attempts),
			(SELECT count(*) FROM integration_delivery_audit),
			(SELECT coalesce(string_agg(trace_id, ',' ORDER BY attempt_id), '')
			   FROM integration_delivery_attempts)
	`).Scan(&out.Receipts, &out.Events, &out.Lineage, &out.Attempts, &out.Audit, &out.Traces); err != nil {
		t.Fatalf("count audit rows: %v", err)
	}
	return out
}

func countExports(t *testing.T, ctx context.Context, db *sql.DB) int {
	t.Helper()
	var rows int
	if err := db.QueryRowContext(ctx,
		`SELECT count(*) FROM integration_session_exports`).Scan(&rows); err != nil {
		t.Fatalf("count exports: %v", err)
	}
	return rows
}

type exportRow struct {
	principalID   string
	principalKind string
	principalAuth string
	reason        string
	includeRaw    bool
}

func readExportRow(t *testing.T, ctx context.Context, db *sql.DB, exportID string) exportRow {
	t.Helper()
	var out exportRow
	if err := db.QueryRowContext(ctx, `
		SELECT principal_json ->> 'id', principal_json ->> 'kind',
		       principal_json ->> 'auth_method', reason, include_raw_payload
		FROM integration_session_exports WHERE export_id = $1
	`, exportID).Scan(&out.principalID, &out.principalKind, &out.principalAuth,
		&out.reason, &out.includeRaw); err != nil {
		t.Fatalf("read export row %s: %v", exportID, err)
	}
	return out
}

func exportColumns(t *testing.T, ctx context.Context, db *sql.DB) map[string]bool {
	t.Helper()
	rows, err := db.QueryContext(ctx, `
		SELECT column_name FROM information_schema.columns
		WHERE table_name = 'integration_session_exports'
	`)
	if err != nil {
		t.Fatalf("read export columns: %v", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan export column: %v", err)
		}
		out[name] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate export columns: %v", err)
	}
	return out
}

// ---------------------------------------------------------------------------
// Delivery state machine: proves the column-scoped guards did not over-lock
// ---------------------------------------------------------------------------

func runDeliveryStateMachine(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()
	store, err := delivery.NewPostgresStore(db, time.Now)
	if err != nil {
		t.Fatalf("delivery.NewPostgresStore: %v", err)
	}
	config := delivery.DefaultConfig()
	config.MaxAttempts = 2
	config.RetryBaseDelay = time.Millisecond
	config.RetryMaxDelay = 2 * time.Millisecond
	config.CircuitFailureThreshold = 10

	// claim -> retryable failure -> re-claim -> terminal failure -> DLQ
	first, err := store.Claim(ctx, "phi-audit-worker", 30*time.Second)
	if err != nil || first == nil {
		t.Fatalf("Claim(first) = %#v, %v", first, err)
	}
	retried, err := store.MarkFailed(ctx, *first, delivery.Failure{
		Code: "TRANSIENT", Detail: "downstream busy", Retryable: true,
	}, config)
	if err != nil || !retried {
		t.Fatalf("MarkFailed(retryable) = %v, %v", retried, err)
	}

	second := claimEventually(t, ctx, store)
	deadLettered, err := store.MarkFailed(ctx, *second, delivery.Failure{
		Code: "PERMANENT", Detail: "rejected by destination", Retryable: false,
	}, config)
	if err != nil || deadLettered {
		t.Fatalf("MarkFailed(terminal) = %v, %v", deadLettered, err)
	}
	assertDLQActive(t, ctx, db, first.AttemptID, true)

	// replay out of the DLQ
	if _, err := store.Replay(ctx, first.TenantID, first.AttemptID, delivery.Operation{
		IdempotencyKey: "phi-audit-replay-1",
		Principal: integration.Principal{
			ID: "delivery-operator-1", Kind: integration.PrincipalKindHuman,
			AuthMethod: "oidc", Roles: []string{delivery.OperatorRole},
		},
		Reason: "phi audit state machine regression guard",
	}); err != nil {
		t.Fatalf("Replay: %v", err)
	}
	assertDLQActive(t, ctx, db, first.AttemptID, false)

	// and the replayed work is claimable and publishable again
	replayed := claimEventually(t, ctx, store)
	if err := store.MarkPublished(ctx, *replayed); err != nil {
		t.Fatalf("MarkPublished after replay: %v", err)
	}
	t.Logf("delivery state machine advanced claim -> retry -> DLQ -> replay -> published "+
		"on attempt %s with the column-scoped guards active", first.AttemptID)
}

func claimEventually(t *testing.T, ctx context.Context, store *delivery.PostgresStore) *delivery.WorkItem {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		item, err := store.Claim(ctx, "phi-audit-worker", 30*time.Second)
		if err != nil {
			t.Fatalf("Claim: %v", err)
		}
		if item != nil {
			return item
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("no delivery work became claimable")
	return nil
}

func assertDLQActive(t *testing.T, ctx context.Context, db *sql.DB, attemptID string, want bool) {
	t.Helper()
	var active bool
	if err := db.QueryRowContext(ctx,
		`SELECT active FROM integration_delivery_dlq WHERE attempt_id = $1`, attemptID).Scan(&active); err != nil {
		t.Fatalf("read dlq for %s: %v", attemptID, err)
	}
	if active != want {
		t.Fatalf("dlq active for %s = %v, want %v", attemptID, active, want)
	}
}

// ---------------------------------------------------------------------------
// Schema provisioning
// ---------------------------------------------------------------------------

const legacySessionID = "sess-legacy-1"

func requirePhiAuditPostgres(t *testing.T) string {
	t.Helper()
	base := os.Getenv("POSTGRES_TEST_URL")
	if base == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("POSTGRES_TEST_URL is required in CI")
		}
		t.Skip("POSTGRES_TEST_URL is required for the PHI audit proof")
	}
	return base
}

// newPhiAuditSchema provisions one independently migrated schema. The primary
// proof and the negative control never share one.
func newPhiAuditSchema(t *testing.T, ctx context.Context, base, prefix string) *sql.DB {
	t.Helper()
	admin, err := sql.Open("postgres", base)
	if err != nil {
		t.Fatalf("open admin connection: %v", err)
	}
	schema := fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	if _, err := admin.ExecContext(ctx, `CREATE SCHEMA `+pq.QuoteIdentifier(schema)); err != nil {
		t.Fatalf("create schema %s: %v", schema, err)
	}
	parsed, err := url.Parse(base)
	if err != nil {
		t.Fatalf("parse POSTGRES_TEST_URL: %v", err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	db, err := sql.Open("postgres", parsed.String())
	if err != nil || db.PingContext(ctx) != nil {
		t.Fatalf("open schema %s: %v", schema, err)
	}
	t.Cleanup(func() {
		_ = db.Close()
		_, _ = admin.ExecContext(context.Background(), `DROP SCHEMA IF EXISTS `+pq.QuoteIdentifier(schema)+` CASCADE`)
		_ = admin.Close()
	})
	return db
}

// applyPreMigrationSchema reconstructs the schema exactly as it stood before this
// slice: processor migrations 0001-0003 and session migrations 0001-0003. It
// reads the sibling packages' migration files from disk because their embedded
// ledgers are package-private and always apply the full current set — which is
// precisely what a negative control must not do.
func applyPreMigrationSchema(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()
	priorMigrations := []string{
		"../processor/migrations/0001_atomic_submission.sql",
		"../processor/migrations/0002_delivery_reliability.sql",
		"../processor/migrations/0003_operator_control_plane.sql",
		"migrations/0001_session_workspace.sql",
		"migrations/0002_workflow_simulations.sql",
		"migrations/0003_publications.sql",
	}
	for _, path := range priorMigrations {
		body, err := os.ReadFile(filepath.Clean(path))
		if err != nil {
			t.Fatalf("read prior migration %s: %v", path, err)
		}
		if _, err := db.ExecContext(ctx, string(body)); err != nil {
			t.Fatalf("apply prior migration %s: %v", path, err)
		}
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO integration_sessions (tenant_id, session_id, status, created_at, record_json)
		VALUES ($1, $2, 'active', $3, $4)
	`, "tenant-a", legacySessionID, time.Now().UTC(),
		fmt.Sprintf(`{"id":%q,"name":"legacy","status":"active"}`, legacySessionID)); err != nil {
		t.Fatalf("seed pre-migration session: %v", err)
	}
}

// applyCurrentSchema applies the full current processor migration set, including
// this slice's 0004, through the production store.
func applyCurrentSchema(t *testing.T, ctx context.Context, db *sql.DB) *processor.PostgresSubmissionStore {
	t.Helper()
	store, err := processor.NewPostgresSubmissionStore(db, processor.PostgresSubmissionConfig{})
	if err != nil {
		t.Fatalf("NewPostgresSubmissionStore: %v", err)
	}
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("processor Migrate: %v", err)
	}
	return store
}

func newAttributedExportStore(t *testing.T, ctx context.Context, db *sql.DB) *PostgresStore {
	t.Helper()
	protector, err := NewAESGCMProtector(bytes.Repeat([]byte{0x52}, 32))
	if err != nil {
		t.Fatalf("NewAESGCMProtector: %v", err)
	}
	store, err := NewPostgresStore(db, PostgresConfig{TenantID: "tenant-a", Protector: protector})
	if err != nil {
		t.Fatalf("NewPostgresStore: %v", err)
	}
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("session Migrate: %v", err)
	}
	return store
}

// ---------------------------------------------------------------------------
// Durable records under test
// ---------------------------------------------------------------------------

// seedPhiAuditRecordsThroughProcessor runs one real production submission so the
// guarded tables hold records the runtime actually wrote.
func seedPhiAuditRecordsThroughProcessor(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	submissions *processor.PostgresSubmissionStore,
) {
	t.Helper()
	messageProcessor, request := newPhiAuditSubmission(t, submissions)
	if _, err := messageProcessor.Process(ctx, request); err != nil {
		t.Fatalf("durable production submission: %v", err)
	}
	counts := countPhiAuditRows(t, ctx, db)
	if counts.Receipts == 0 || counts.Events == 0 || counts.Lineage == 0 || counts.Attempts == 0 {
		t.Fatalf("submission did not populate the guarded tables: %+v", counts)
	}
}

// seedPhiAuditRecords populates the pre-migration schema with the same record
// shapes through direct SQL, because the production store always applies the
// current migration set.
func seedPhiAuditRecords(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()
	statements := []string{
		`INSERT INTO integration_receipts (
			tenant_id, receipt_id, idempotency_key, request_fingerprint, integration_revision,
			status, recorded_at, correlation_id, raw_retention_mode, principal_json, reason, result_json
		) VALUES ('tenant-a', 'rcpt-1', 'idem-1', 'fingerprint-1', '{}'::jsonb,
			'accepted', now(), 'corr-1', 'ephemeral', '{"id":"svc-1"}'::jsonb, '', '{}'::jsonb)`,
		`INSERT INTO integration_canonical_events (
			tenant_id, event_id, receipt_id, event_type, source_message_id,
			correlation_id, classification, payload_json, recorded_at
		) VALUES ('tenant-a', 'evt-1', 'rcpt-1', 'patient_admit', 'msg-1',
			'corr-1', 'phi', '{"mrn":"RAW-PHI-SENTINEL"}'::jsonb, now())`,
		`INSERT INTO integration_message_lineage (
			tenant_id, lineage_id, receipt_id, event_id, trace_id, correlation_id,
			source_message_id, artifact_revisions_json, routes_json, diagnostics_json, recorded_at
		) VALUES ('tenant-a', 'lin-1', 'rcpt-1', 'evt-1', 'trace-1', 'corr-1',
			'msg-1', '{}'::jsonb, '[]'::jsonb, '[]'::jsonb, now())`,
		`INSERT INTO integration_delivery_attempts (
			tenant_id, attempt_id, receipt_id, event_id, trace_id, destination_revision_json,
			route_name, action_id, status, attempt_count, recorded_at, scheduled_at
		) VALUES ('tenant-a', 'att-1', 'rcpt-1', 'evt-1', 'trace-1', '{}'::jsonb,
			'matched', 'send-fhir', 'queued', 1, now(), now())`,
		`INSERT INTO integration_delivery_audit (
			tenant_id, attempt_id, event_kind, attempt_count, principal_json, detail_json, recorded_at
		) VALUES ('tenant-a', 'att-1', 'claimed', 1, '{}'::jsonb, '{}'::jsonb, now())`,
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			t.Fatalf("seed pre-migration record: %v", err)
		}
	}
}

func newPhiAuditSubmission(
	t *testing.T,
	submissions *processor.PostgresSubmissionStore,
) (*processor.MessageProcessor, integration.ProcessRequest) {
	t.Helper()
	profileRef, err := processor.NewProfileRevisionReference("profile-adt", 7, []byte(phiAuditProfileJSON))
	if err != nil {
		t.Fatalf("NewProfileRevisionReference: %v", err)
	}
	workflowRef, err := processor.NewWorkflowRevisionReference("workflow-adt", "workflow-1", []byte(phiAuditWorkflowYAML))
	if err != nil {
		t.Fatalf("NewWorkflowRevisionReference: %v", err)
	}
	digest := func(character string) string { return "sha256:" + strings.Repeat(character, 64) }
	revision, err := integration.NewIntegrationDefinitionRevision(integration.IntegrationDefinitionRevisionInput{
		DefinitionID: "integration-adt",
		RevisionID:   "revision-1",
		TenantID:     "tenant-a",
		Source: integration.SourceRevisionRef{
			ArtifactRevisionRef: integration.ArtifactRevisionRef{
				ArtifactID: "source-adt", RevisionID: "source-1", Digest: digest("a"),
			},
			SourceID: "adt-east",
		},
		Format:   events.FormatHL7v2,
		Profile:  profileRef,
		Workflow: workflowRef,
		Destinations: []integration.DestinationRevisionRef{{
			ArtifactRevisionRef: integration.ArtifactRevisionRef{
				ArtifactID: "fhir-primary", RevisionID: "destination-1", Digest: digest("d"),
			},
			Class: integration.DestinationClassProduction,
		}},
		Policy: integration.IntegrationPolicy{
			Classification: integration.DataClassificationPHI,
			RawRetention:   integration.RawRetentionPolicy{Mode: integration.RawRetentionModeEphemeral},
		},
		Created: integration.AuditEnvelope{
			TenantID: "tenant-a",
			Principal: integration.Principal{
				ID: "operator-1", Kind: integration.PrincipalKindHuman,
				AuthMethod: "oidc", Roles: []string{"publisher"},
			},
			Reason:     "publish",
			OccurredAt: time.Date(2026, 7, 13, 10, 0, 0, 0, time.UTC),
		},
	})
	if err != nil {
		t.Fatalf("NewIntegrationDefinitionRevision: %v", err)
	}
	definitionJSON, err := json.Marshal(revision)
	if err != nil {
		t.Fatalf("marshal revision: %v", err)
	}
	definitions, err := processor.NewDefinitionRevisionResolver("tenant-a",
		phiAuditLoader{definition: definitionJSON})
	if err != nil {
		t.Fatalf("NewDefinitionRevisionResolver: %v", err)
	}
	artifacts, err := processor.NewRevisionResolver("tenant-a", phiAuditLoader{
		profile: []byte(phiAuditProfileJSON), workflow: []byte(phiAuditWorkflowYAML),
	})
	if err != nil {
		t.Fatalf("NewRevisionResolver: %v", err)
	}
	messageProcessor, err := processor.NewDurableMessageProcessor(definitions, artifacts, submissions)
	if err != nil {
		t.Fatalf("NewDurableMessageProcessor: %v", err)
	}
	envelope, err := integration.NewRawEnvelope(integration.RawEnvelopeMetadata{
		TenantID:       "tenant-a",
		SourceID:       "adt-east",
		Format:         events.FormatHL7v2,
		ContentType:    "x-application/hl7-v2+er7",
		ReceivedAt:     time.Date(2026, 7, 13, 14, 0, 0, 0, time.UTC),
		Classification: integration.DataClassificationPHI,
	}, []byte(phiAuditHL7Message))
	if err != nil {
		t.Fatalf("NewRawEnvelope: %v", err)
	}
	request := integration.ProcessRequest{
		Mode:                integration.ExecutionModeProduction,
		IntegrationRevision: revision.Reference(),
		Security: integration.SecurityContext{
			TenantID: "tenant-a",
			Principal: integration.Principal{
				ID: "source-service", Kind: integration.PrincipalKindService,
				AuthMethod: "mtls", SourceID: "adt-east",
				Roles: []string{authorization.HTTPSubmitGrant},
			},
		},
		Envelope:       envelope,
		CorrelationID:  "phi-audit-correlation",
		IdempotencyKey: "phi-audit-submission-1",
	}
	return messageProcessor, request
}

type phiAuditLoader struct {
	definition []byte
	profile    []byte
	workflow   []byte
}

func (l phiAuditLoader) LoadDefinitionRevision(ctx context.Context, _, _, _ string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return append([]byte(nil), l.definition...), nil
}

func (l phiAuditLoader) LoadProfileRevision(ctx context.Context, _, _ string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return append([]byte(nil), l.profile...), nil
}

func (l phiAuditLoader) LoadWorkflowRevision(ctx context.Context, _, _ string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return append([]byte(nil), l.workflow...), nil
}

const phiAuditProfileJSON = `{"hl7v2":{"default_version":"2.5.1","timezone":"UTC","tolerance":{"missing_segments":["PV1"],"nte_anywhere":false,"extra_components":false,"unknown_segments":false,"non_standard_delimiters":false},"event_classifications":[{"message_type":"ADT^A01","event_type":"patient_admit","priority":1}]},"identifiers":{"assigning_authorities":[{"code":"HOSP","system":"urn:oid:1.2.3"}],"normalization":{"ssn_strip_dashes":true,"phone_normalize":false}}}`

const phiAuditWorkflowYAML = `dsl_version: "1"
name: adt-phi-audit
version: "1"
routes:
  - name: matched
    filter:
      event_type: patient_admit
      source: adt-east
    actions:
      - id: send-fhir
        type: fhir
        destination: fhir-primary
`

const phiAuditHL7Message = "MSH|^~\\&|APP|FAC|EHR|HOSPITAL|20260713120000-0400||ADT^A01^ADT_A01|control-123|P|2.5.1\r" +
	"EVN|A01|20260713120000||||20260713115900-0400\r" +
	"PID|1||MRN-123^^^HOSP^MR||Patient^Test||19800101|F"
