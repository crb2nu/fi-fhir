//go:build integration

package retention

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/authorization"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/delivery"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/processor"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/session"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

const (
	purgedSentinel   = "MRN-PURGE-SENTINEL-A"
	retainedSentinel = "MRN-RETAINED-SENTINEL-B"
	rawCipherPlain   = "MRN-RAW-CIPHER-SENTINEL"
	tombstoneJSON    = `{"purge_schema": "fi-fhir.retention.tombstone.v1", "purged": true}`
)

// TestPhiRetention_PostgresExpiryPurgeAndAuditedTombstone is Slice 4.1e's
// kill-test, against a live PostgreSQL 16 with -race and two purge components
// running concurrently against one database.
//
// It proves, in this order:
//
//  1. NEGATIVE CONTROL on the pre-4.1e schema — the schema this MR changes.
//     The purge fails outright there, and no shape of UPDATE can tombstone a
//     payload, so the tombstone the primary proof observes is attributable to
//     this migration rather than to something already present.
//  2. NEGATIVE CONTROL on the pre-4.1d-C1 schema. There the very mutations the
//     primary proof requires to raise all SUCCEED, so the primary proof's
//     refusals are attributable to a guard and not to referential integrity or
//     to a malformed statement.
//  3. Expiry, purge, and audit: a canonical event past its purge_after is
//     tombstoned exactly once with exactly one audit row; a session sample is
//     deleted outright, ciphertext included; an export snapshot is tombstoned
//     while its disclosure attribution survives.
//  4. The delivery interlock: an event whose attempt is still queued is NEVER
//     purged, and the delivery Claim join goes on returning its real payload.
//  5. The exemption is narrow: every other mutation of a canonical event still
//     raises, and row counts are identical before and after.
//  6. The posture gate inverted: the expiry columns now exist and
//     docs/operations/PHI-RETENTION.md was rewritten in the same commit.
//
// A NOTE ON THE SPEC'S NEGATIVE CONTROL. `.loom/32-sprint4-execution-specs.md`
// describes one control — "the pre-migration database, where the purge must fail
// and step 4's mutations must SUCCEED". Those two outcomes cannot come from one
// schema: on the pre-4.1e schema C1's blanket guard is active, so the mutations
// raise there too, for a different reason. The control is therefore split in
// two, which is strictly stronger. Recorded as correction 41 in `.loom/32`.
func TestPhiRetention_PostgresExpiryPurgeAndAuditedTombstone(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	base := requireRetentionPostgres(t)

	t.Run("negative_control_pre_slice_schema_cannot_purge", func(t *testing.T) {
		legacy := newRetentionGateSchema(t, ctx, base, "phi_retention_pre_4_1e")
		applyPre41eSchema(t, ctx, legacy)
		seedRawCanonicalEvent(t, ctx, legacy, "rcpt-pre", "evt-pre", purgedSentinel)

		for _, column := range []string{"purge_after", "purged_at"} {
			if columnExists(t, ctx, legacy, "integration_canonical_events", column) {
				t.Fatalf("pre-4.1e schema already has integration_canonical_events.%s; "+
					"the control is asserting against the wrong schema", column)
			}
		}
		store, err := NewPostgresStore(legacy, PostgresConfig{TenantID: "tenant-a"})
		if err != nil {
			t.Fatalf("NewPostgresStore: %v", err)
		}
		if _, err := store.PurgeExpired(ctx, 10); err == nil {
			t.Fatal("the purge succeeded against the pre-4.1e schema; the primary proof would be vacuous")
		}
		// And there is no exemption to reach for: the only UPDATE that could
		// redact a payload raises on C1's blanket guard.
		if _, err := legacy.ExecContext(ctx,
			`UPDATE integration_canonical_events SET payload_json = $1::jsonb`, tombstoneJSON); err == nil {
			t.Fatal("the pre-4.1e schema accepted a tombstone UPDATE; correction 11 is wrong")
		}
		if !strings.Contains(dumpColumn(t, ctx, legacy,
			"integration_canonical_events", "payload_json"), purgedSentinel) {
			t.Fatal("the pre-4.1e payload was modified despite every write raising")
		}
		t.Log("negative control: the pre-4.1e schema has no expiry columns, no policy record, " +
			"and no shape of UPDATE that tombstones a payload")
	})

	t.Run("negative_control_pre_c1_schema_permits_every_mutation", func(t *testing.T) {
		legacy := newRetentionGateSchema(t, ctx, base, "phi_retention_pre_c1")
		applyPreC1Schema(t, ctx, legacy)
		seedRawCanonicalEvent(t, ctx, legacy, "rcpt-pre-c1", "evt-pre-c1", purgedSentinel)

		for _, mutation := range canonicalEventMutations("evt-pre-c1") {
			if _, err := legacy.ExecContext(ctx, mutation.forLegacySchema()); err != nil {
				t.Fatalf("pre-C1 schema rejected %q: %v — the primary proof's refusals "+
					"would prove nothing about this slice's guard", mutation.name, err)
			}
		}
		t.Log("negative control: every mutation the primary proof requires to raise " +
			"succeeds on the pre-C1 schema")
	})

	// ---- Primary proof -----------------------------------------------------
	db := newRetentionGateSchema(t, ctx, base, "phi_retention_purge")
	backdated := time.Now().UTC().Add(-30 * 24 * time.Hour)
	fixture := seedRetentionFixture(t, ctx, db, backdated)

	policy := Policy{
		TenantID: "tenant-a",
		// Every window is far shorter than the 30 days the fixture was backdated
		// by, so every seeded record is already expired against the real clock.
		CanonicalEventRetain: time.Hour,
		SessionSampleRetain:  time.Hour,
		SessionExportRetain:  time.Hour,
		StreamEventRetain:    StreamEventPruneFloor,
		Principal: integration.Principal{
			ID: "privacy-officer-7", Kind: integration.PrincipalKindHuman, AuthMethod: "oidc",
		},
		Reason:         "HIPAA minimum-necessary retention, approved 2026-08-08",
		DocumentDigest: "sha256:" + strings.Repeat("c", 64),
	}
	store := newRetentionStore(t, db)
	recorded, err := store.PutPolicy(ctx, policy)
	if err != nil {
		t.Fatalf("PutPolicy: %v", err)
	}
	if recorded.Version != 1 {
		t.Fatalf("first policy version = %d, want 1", recorded.Version)
	}
	if again, err := store.PutPolicy(ctx, policy); err != nil || again.Version != 1 {
		t.Fatalf("unchanged policy document minted a version: %+v, %v", again, err)
	}

	t.Run("simultaneous_replica_boot_records_one_policy_version", func(t *testing.T) {
		// Two replicas starting at the same moment against the same document must
		// not race the policy audit's UNIQUE (tenant_id, policy_version). Without
		// the advisory lock in PutPolicy both compute the same next version and one
		// replica dies at startup over a policy neither was changing.
		fresh := newRetentionGateSchema(t, ctx, base, "phi_retention_policy_race")
		migrateDurableSchema(t, ctx, fresh)
		start := make(chan struct{})
		errs := make([]error, 2)
		versions := make([]int64, 2)
		var wait sync.WaitGroup
		for index := range 2 {
			replica := newRetentionStore(t, fresh)
			wait.Add(1)
			go func() {
				defer wait.Done()
				<-start
				recorded, err := replica.PutPolicy(ctx, policy)
				versions[index], errs[index] = recorded.Version, err
			}()
		}
		close(start)
		wait.Wait()
		for index, err := range errs {
			if err != nil {
				t.Fatalf("replica %d failed to record the policy: %v", index, err)
			}
			if versions[index] != 1 {
				t.Fatalf("replica %d recorded policy version %d, want 1", index, versions[index])
			}
		}
		if audited := countRows(t, ctx, fresh,
			`SELECT count(*) FROM integration_retention_policy_audit`); audited != 1 {
			t.Fatalf("policy audit rows = %d, want exactly 1", audited)
		}
	})

	before := countRetentionRows(t, ctx, db)

	t.Run("two_replicas_purge_each_record_exactly_once", func(t *testing.T) {
		results := runConcurrentPurgers(t, ctx, db, 2)
		total := PurgeCounts{}
		for _, result := range results {
			total.CanonicalEvents += result.CanonicalEvents
			total.SessionSamples += result.SessionSamples
			total.SessionExports += result.SessionExports
			total.StreamEvents += result.StreamEvents
		}
		if total.CanonicalEvents != 1 {
			t.Fatalf("canonical events purged across both replicas = %d, want exactly 1", total.CanonicalEvents)
		}
		if total.SessionSamples != 1 {
			t.Fatalf("session samples purged across both replicas = %d, want exactly 1", total.SessionSamples)
		}
		if total.SessionExports != 1 {
			t.Fatalf("session exports purged across both replicas = %d, want exactly 1", total.SessionExports)
		}
		if total.StreamEvents != 1 {
			t.Fatalf("stream envelopes pruned across both replicas = %d, want exactly 1", total.StreamEvents)
		}

		audit := readPurgeAudit(t, ctx, db)
		want := map[string]string{
			"canonical_event:" + fixture.purgedEventID: "tombstone",
			"session_sample:" + fixture.sampleID:       "deleted",
			"session_export:" + fixture.exportID:       "tombstone",
		}
		if len(audit) != len(want) {
			t.Fatalf("purge audit rows = %+v, want exactly %+v", audit, want)
		}
		for key, mode := range want {
			row, ok := audit[key]
			if !ok {
				t.Fatalf("no purge audit row for %s; a purge without an audit row must be impossible", key)
			}
			if row.mode != mode {
				t.Fatalf("%s purge_mode = %q, want %q", key, row.mode, mode)
			}
			if row.policyVersion != recorded.Version {
				t.Fatalf("%s policy_version = %d, want %d", key, row.policyVersion, recorded.Version)
			}
			if row.purgeAfter.After(row.purgedAt) {
				t.Fatalf("%s was purged at %s, before its deadline %s", key, row.purgedAt, row.purgeAfter)
			}
		}
	})

	t.Run("both_phi_sentinels_are_gone_and_the_row_survives_as_a_tombstone", func(t *testing.T) {
		payloads := dumpColumn(t, ctx, db, "integration_canonical_events", "payload_json")
		if strings.Contains(payloads, purgedSentinel) {
			t.Fatal("the purged canonical payload is still readable in integration_canonical_events")
		}
		if !strings.Contains(payloads, retainedSentinel) {
			t.Fatal("the retained event's payload vanished; the purge is over-reaching")
		}

		row := readCanonicalEvent(t, ctx, db, fixture.purgedEventID)
		if row.purgedAt == nil {
			t.Fatal("purged canonical event has no purged_at")
		}
		if !jsonEqual(t, row.payload, tombstoneJSON) {
			t.Fatalf("purged payload = %s, want the canonical tombstone", row.payload)
		}
		// The row, its identity, and its classification survive on purpose: an
		// audit must still be able to show what existed.
		if row.classification != "phi" || row.correlationID == "" || row.recordedAt.IsZero() {
			t.Fatalf("the tombstone destroyed identity as well as payload: %+v", row)
		}

		// The sample row and its ciphertext are gone, not tombstoned.
		if count := countRows(t, ctx, db,
			`SELECT count(*) FROM integration_session_samples WHERE sample_id = $1`, fixture.sampleID); count != 0 {
			t.Fatalf("purged session sample still present (%d rows)", count)
		}
		ciphertexts := dumpColumn(t, ctx, db, "integration_session_samples", "encode(raw_cipher, 'hex')")
		if fixture.sampleCipherHex != "" && strings.Contains(ciphertexts, fixture.sampleCipherHex) {
			t.Fatal("the purged sample's ciphertext is still present in integration_session_samples")
		}

		// The export snapshot is tombstoned; the disclosure attribution is not.
		export := readExport(t, ctx, db, fixture.exportID)
		if export.purgedAt == nil || !jsonEqual(t, export.record, tombstoneJSON) {
			t.Fatalf("export snapshot was not tombstoned: %+v", export)
		}
		if export.principalID != "privacy-officer-7" || export.reason == "" {
			t.Fatalf("the export purge destroyed the disclosure attribution: %+v", export)
		}
	})

	t.Run("an_unresolved_delivery_attempt_protects_its_event_from_the_purge", func(t *testing.T) {
		row := readCanonicalEvent(t, ctx, db, fixture.retainedEventID)
		if row.purgedAt != nil {
			t.Fatal("an event with a queued delivery attempt was purged; " +
				"the delivery Claim join can now observe a tombstone")
		}
		if !strings.Contains(row.payload, retainedSentinel) {
			t.Fatalf("the retained event's payload changed: %s", row.payload)
		}

		// And the shipped state machine still claims and publishes it normally,
		// with its real payload rather than a tombstone.
		deliveryStore, err := delivery.NewPostgresStore(db, time.Now)
		if err != nil {
			t.Fatalf("delivery.NewPostgresStore: %v", err)
		}
		item, err := deliveryStore.Claim(ctx, "retention-purge-test", 30*time.Second)
		if err != nil || item == nil {
			t.Fatalf("Claim after purge = %#v, %v", item, err)
		}
		if item.EventID != fixture.retainedEventID {
			t.Fatalf("Claim returned event %q, want the unpurged %q", item.EventID, fixture.retainedEventID)
		}
		if !strings.Contains(string(item.EventPayload), retainedSentinel) {
			t.Fatalf("Claim returned a payload without the sentinel: %s", item.EventPayload)
		}
		if strings.Contains(string(item.EventPayload), "purge_schema") {
			t.Fatal("Claim returned a tombstone to the delivery worker")
		}
		if err := deliveryStore.MarkPublished(ctx, *item); err != nil {
			t.Fatalf("MarkPublished after purge: %v", err)
		}
	})

	t.Run("the_fanout_log_is_pruned_only_past_the_schema_floor", func(t *testing.T) {
		remaining := countRows(t, ctx, db, `SELECT count(*) FROM integration_session_stream_events`)
		if remaining != 1 {
			t.Fatalf("stream envelopes remaining = %d, want 1 (the young one)", remaining)
		}
		var youngSeq int64
		if err := db.QueryRowContext(ctx,
			`SELECT seq FROM integration_session_stream_events`).Scan(&youngSeq); err != nil {
			t.Fatalf("read remaining stream envelope: %v", err)
		}
		if _, err := db.ExecContext(ctx,
			`DELETE FROM integration_session_stream_events WHERE seq = $1`, youngSeq); err == nil {
			t.Fatal("a stream envelope younger than the 24 hour floor was deletable")
		}
		if _, err := db.ExecContext(ctx,
			`UPDATE integration_session_stream_events SET event_type = 'forged'`); err == nil {
			t.Fatal("a stream envelope was rewritable")
		}
	})

	t.Run("every_other_canonical_event_mutation_still_raises", func(t *testing.T) {
		counts := countRetentionRows(t, ctx, db)
		for _, mutation := range canonicalEventMutations(fixture.retainedEventID) {
			if _, err := db.ExecContext(ctx, mutation.statement); err == nil {
				t.Fatalf("%s succeeded; the exemption is wider than a tombstone", mutation.name)
			} else if !strings.Contains(err.Error(), "append-only") {
				t.Fatalf("%s raised %q, want an append-only guard message", mutation.name, err)
			}
		}
		// A second tombstone on an already-purged row must also raise: the
		// exemption is one-shot.
		if _, err := db.ExecContext(ctx, `
			UPDATE integration_canonical_events SET payload_json = $1::jsonb, purged_at = now()
			WHERE event_id = $2
		`, tombstoneJSON, fixture.purgedEventID); err == nil {
			t.Fatal("an already-purged canonical event was tombstoned twice")
		}
		if after := countRetentionRows(t, ctx, db); after != counts {
			t.Fatalf("row counts moved while every mutation raised: before=%+v after=%+v", counts, after)
		}
		if before.events != counts.events {
			t.Fatalf("the purge deleted a canonical event row: before=%d after=%d", before.events, counts.events)
		}
	})

	t.Run("the_retention_audit_ledgers_are_append_only", func(t *testing.T) {
		// The purge audit is the only durable record of what the platform
		// destroyed, and the policy audit the only record of who authorized it.
		// Both are claimed append-only by 0005_retention_expiry.sql; a claim this
		// slice makes and does not prove is exactly the kind of gap the lane was
		// written to remove.
		before := countRows(t, ctx, db, `SELECT count(*) FROM integration_retention_purge_audit`) +
			countRows(t, ctx, db, `SELECT count(*) FROM integration_retention_policy_audit`)
		if before == 0 {
			t.Fatal("both retention audit ledgers are empty, so these assertions would pass vacuously")
		}
		for _, mutation := range []struct{ name, statement string }{
			{"UPDATE purge audit", `UPDATE integration_retention_purge_audit SET purge_mode = 'deleted'`},
			{"DELETE purge audit", `DELETE FROM integration_retention_purge_audit`},
			{"UPDATE policy audit", `UPDATE integration_retention_policy_audit SET reason = 'rewritten'`},
			{"DELETE policy audit", `DELETE FROM integration_retention_policy_audit`},
		} {
			if _, err := db.ExecContext(ctx, mutation.statement); err == nil {
				t.Fatalf("%s succeeded; the retention audit is not append-only", mutation.name)
			} else if !strings.Contains(err.Error(), "append-only") {
				t.Fatalf("%s raised %q, want an append-only guard message", mutation.name, err)
			}
		}
		after := countRows(t, ctx, db, `SELECT count(*) FROM integration_retention_purge_audit`) +
			countRows(t, ctx, db, `SELECT count(*) FROM integration_retention_policy_audit`)
		if after != before {
			t.Fatalf("retention audit rows changed: before=%d after=%d", before, after)
		}
	})

	t.Run("the_posture_gate_is_inverted_and_the_document_was_rewritten", func(t *testing.T) {
		for _, column := range []string{"purge_after", "purged_at"} {
			if !columnExists(t, ctx, db, "integration_canonical_events", column) {
				t.Fatalf("integration_canonical_events.%s is missing", column)
			}
		}
		if !columnExists(t, ctx, db, "integration_session_samples", "purge_after") {
			t.Fatal("integration_session_samples.purge_after is missing")
		}
		doc, err := os.ReadFile(filepath.Clean("../../../docs/operations/PHI-RETENTION.md"))
		if err != nil {
			t.Fatalf("read PHI-RETENTION.md: %v", err)
		}
		body := string(doc)
		for _, stale := range []string{
			"Not implemented** — S3-C2",
			"internal/integration/session/postgres.go:303-305",
			"internal/integration/session/postgres.go:889-893",
		} {
			if strings.Contains(body, stale) {
				t.Fatalf("PHI-RETENTION.md still carries the stale claim %q; sections 2, 3, and 6 "+
					"and the drifted citations must be rewritten in this same commit", stale)
			}
		}
		for _, required := range []string{"purge_after", "tombstone", "integration_retention_policies"} {
			if !strings.Contains(body, required) {
				t.Fatalf("PHI-RETENTION.md does not mention %q", required)
			}
		}
	})
}

// ---------------------------------------------------------------------------
// Mutations under test
// ---------------------------------------------------------------------------

type canonicalEventMutation struct {
	name      string
	statement string

	// legacyStatement is the same mutation written against the pre-C1 schema,
	// which has no expiry columns to name. It exists so the negative control can
	// exercise the identical intent without the assertion silently degrading into
	// "the column does not exist".
	legacyStatement string
}

func (m canonicalEventMutation) forLegacySchema() string {
	if m.legacyStatement != "" {
		return m.legacyStatement
	}
	return m.statement
}

// canonicalEventMutations is the exact list from the lane spec's kill-test step
// 4. Every one of them succeeds on the pre-C1 schema (proved by the second
// negative control) and every one must raise once this slice's exemption is in
// place, because none of them is the canonical tombstone.
func canonicalEventMutations(eventID string) []canonicalEventMutation {
	return []canonicalEventMutation{
		{
			name:      "DELETE integration_canonical_events",
			statement: `DELETE FROM integration_canonical_events WHERE event_id = '` + eventID + `'`,
		},
		{
			name: "UPDATE classification and payload",
			statement: `UPDATE integration_canonical_events SET classification = 'phi', payload_json = '{"x":1}'::jsonb ` +
				`WHERE event_id = '` + eventID + `'`,
		},
		{
			name:      "UPDATE recorded_at",
			statement: `UPDATE integration_canonical_events SET recorded_at = now() WHERE event_id = '` + eventID + `'`,
		},
		{
			name: "UPDATE tombstone plus correlation_id",
			statement: `UPDATE integration_canonical_events SET payload_json = '` + tombstoneJSON + `'::jsonb, ` +
				`purged_at = now(), correlation_id = 'x' WHERE event_id = '` + eventID + `'`,
			legacyStatement: `UPDATE integration_canonical_events SET payload_json = '` + tombstoneJSON + `'::jsonb, ` +
				`correlation_id = 'x' WHERE event_id = '` + eventID + `'`,
		},
	}
}

// ---------------------------------------------------------------------------
// Fixture
// ---------------------------------------------------------------------------

type retentionFixture struct {
	purgedEventID   string
	retainedEventID string
	sampleID        string
	sampleCipherHex string
	exportID        string
}

// seedRetentionFixture builds the durable state the purge acts on, through the
// production stores rather than by hand, with a clock backdated 30 days so every
// record is genuinely expired against the real clock the purge runs on.
func seedRetentionFixture(t *testing.T, ctx context.Context, db *sql.DB, backdated time.Time) retentionFixture {
	t.Helper()
	clock := func() time.Time { return backdated }
	submissions, err := processor.NewPostgresSubmissionStore(db, processor.PostgresSubmissionConfig{Clock: clock})
	if err != nil {
		t.Fatalf("NewPostgresSubmissionStore: %v", err)
	}
	if err := submissions.Migrate(ctx); err != nil {
		t.Fatalf("processor Migrate: %v", err)
	}

	// The event that will be purged. Its delivery attempt is resolved first, so
	// nothing is still owed to a destination when the purge runs.
	purgedEventID := submitRetentionMessage(t, ctx, submissions, "purge", purgedSentinel)
	deliveryStore, err := delivery.NewPostgresStore(db, time.Now)
	if err != nil {
		t.Fatalf("delivery.NewPostgresStore: %v", err)
	}
	item, err := deliveryStore.Claim(ctx, "retention-fixture", 30*time.Second)
	if err != nil || item == nil {
		t.Fatalf("Claim(purged fixture) = %#v, %v", item, err)
	}
	if item.EventID != purgedEventID {
		t.Fatalf("fixture claimed %q, want %q", item.EventID, purgedEventID)
	}
	if err := deliveryStore.MarkPublished(ctx, *item); err != nil {
		t.Fatalf("MarkPublished(purged fixture): %v", err)
	}

	// The event that must survive: its attempt stays queued.
	retainedEventID := submitRetentionMessage(t, ctx, submissions, "retain", retainedSentinel)

	protector, err := session.NewAESGCMProtector(bytes.Repeat([]byte{0x52}, 32))
	if err != nil {
		t.Fatalf("NewAESGCMProtector: %v", err)
	}
	sessions, err := session.NewPostgresStore(db, session.PostgresConfig{
		TenantID: "tenant-a", Protector: protector, Clock: clock,
	})
	if err != nil {
		t.Fatalf("session.NewPostgresStore: %v", err)
	}
	if err := sessions.Migrate(ctx); err != nil {
		t.Fatalf("session Migrate: %v", err)
	}
	workspace, err := sessions.CreateSession(ctx, session.CreateSessionRequest{Name: "retention purge"})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	sample, err := sessions.AddSample(ctx, workspace.ID, session.AddSampleRequest{
		Name: "admit", Format: events.FormatHL7v2,
		Raw: retentionHL7Message("sample", rawCipherPlain), PHIPolicy: session.PHIPolicyRetain,
	})
	if err != nil {
		t.Fatalf("AddSample: %v", err)
	}
	var cipherHex string
	if err := db.QueryRowContext(ctx,
		`SELECT coalesce(encode(raw_cipher, 'hex'), '') FROM integration_session_samples WHERE sample_id = $1`,
		sample.ID).Scan(&cipherHex); err != nil {
		t.Fatalf("read sample ciphertext: %v", err)
	}
	if cipherHex == "" {
		t.Fatal("retained sample stored no ciphertext, so the purge assertion would be vacuous")
	}
	bundle, err := sessions.ExportBundle(ctx, session.ExportRequest{
		SessionID: workspace.ID,
		Principal: integration.Principal{
			ID: "privacy-officer-7", Kind: integration.PrincipalKindHuman, AuthMethod: "oidc",
		},
		Reason: "retention purge fixture",
	})
	if err != nil {
		t.Fatalf("ExportBundle: %v", err)
	}

	// The fanout log: one envelope past the schema floor and one inside it.
	for _, age := range []time.Duration{48 * time.Hour, time.Hour} {
		if _, err := db.ExecContext(ctx, `
			INSERT INTO integration_session_stream_events
				(tenant_id, event_id, session_id, run_id, event_type, created_at)
			VALUES ('tenant-a', $1, $2, '', 'run_completed', $3)
		`, fmt.Sprintf("stream-%s", age), workspace.ID, time.Now().UTC().Add(-age)); err != nil {
			t.Fatalf("seed stream envelope: %v", err)
		}
	}

	payloads := dumpColumn(t, ctx, db, "integration_canonical_events", "payload_json")
	for _, sentinel := range []string{purgedSentinel, retainedSentinel} {
		if !strings.Contains(payloads, sentinel) {
			t.Fatalf("sentinel %q never reached the canonical payload, so the purge assertions "+
				"would pass vacuously", sentinel)
		}
	}
	return retentionFixture{
		purgedEventID:   purgedEventID,
		retainedEventID: retainedEventID,
		sampleID:        sample.ID,
		sampleCipherHex: cipherHex,
		exportID:        bundle.ID,
	}
}

func submitRetentionMessage(
	t *testing.T,
	ctx context.Context,
	submissions *processor.PostgresSubmissionStore,
	label string,
	sentinel string,
) string {
	t.Helper()
	profileRef, err := processor.NewProfileRevisionReference("profile-adt", 7, []byte(retentionProfileJSON))
	if err != nil {
		t.Fatalf("NewProfileRevisionReference: %v", err)
	}
	workflowRef, err := processor.NewWorkflowRevisionReference("workflow-adt", "workflow-1", []byte(retentionWorkflowYAML))
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
	loader := retentionLoader{
		definition: definitionJSON,
		profile:    []byte(retentionProfileJSON),
		workflow:   []byte(retentionWorkflowYAML),
	}
	definitions, err := processor.NewDefinitionRevisionResolver("tenant-a", loader)
	if err != nil {
		t.Fatalf("NewDefinitionRevisionResolver: %v", err)
	}
	artifacts, err := processor.NewRevisionResolver("tenant-a", loader)
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
	}, []byte(retentionHL7Message(label, sentinel)))
	if err != nil {
		t.Fatalf("NewRawEnvelope: %v", err)
	}
	result, err := messageProcessor.Process(ctx, integration.ProcessRequest{
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
		CorrelationID:  "retention-" + label,
		IdempotencyKey: "retention-submission-" + label,
	})
	if err != nil {
		t.Fatalf("durable submission %s: %v", label, err)
	}
	if len(result.Events) != 1 {
		t.Fatalf("submission %s produced %d events, want 1", label, len(result.Events))
	}
	return result.Events[0].ID
}

type retentionLoader struct {
	definition []byte
	profile    []byte
	workflow   []byte
}

func (l retentionLoader) LoadDefinitionRevision(ctx context.Context, _, _, _ string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return append([]byte(nil), l.definition...), nil
}

func (l retentionLoader) LoadProfileRevision(ctx context.Context, _, _ string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return append([]byte(nil), l.profile...), nil
}

func (l retentionLoader) LoadWorkflowRevision(ctx context.Context, _, _ string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return append([]byte(nil), l.workflow...), nil
}

// ---------------------------------------------------------------------------
// Concurrency
// ---------------------------------------------------------------------------

// runConcurrentPurgers runs N independent purge components against one database
// at the same moment, which is the multi-replica case. Each gets its own store,
// so nothing is shared in process and the only coordination available to them is
// the guarded statement itself.
func runConcurrentPurgers(t *testing.T, ctx context.Context, db *sql.DB, replicas int) []PurgeResult {
	t.Helper()
	results := make([]PurgeResult, replicas)
	errs := make([]error, replicas)
	start := make(chan struct{})
	var wait sync.WaitGroup
	for index := range replicas {
		purger, err := NewPurger(PurgerConfig{Store: newRetentionStore(t, db), Interval: time.Hour})
		if err != nil {
			t.Fatalf("NewPurger: %v", err)
		}
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			results[index], errs[index] = purger.PurgeOnce(ctx)
		}()
	}
	close(start)
	wait.Wait()
	for index, err := range errs {
		if err != nil {
			t.Fatalf("replica %d purge failed: %v", index, err)
		}
	}
	return results
}

func newRetentionStore(t *testing.T, db *sql.DB) *PostgresStore {
	t.Helper()
	store, err := NewPostgresStore(db, PostgresConfig{TenantID: "tenant-a"})
	if err != nil {
		t.Fatalf("NewPostgresStore: %v", err)
	}
	return store
}

// ---------------------------------------------------------------------------
// Readers
// ---------------------------------------------------------------------------

type purgeAuditRow struct {
	mode          string
	policyVersion int64
	purgeAfter    time.Time
	purgedAt      time.Time
}

func readPurgeAudit(t *testing.T, ctx context.Context, db *sql.DB) map[string]purgeAuditRow {
	t.Helper()
	rows, err := db.QueryContext(ctx, `
		SELECT record_class, record_id, purge_mode, policy_version, purge_after, purged_at
		FROM integration_retention_purge_audit ORDER BY audit_id
	`)
	if err != nil {
		t.Fatalf("read purge audit: %v", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]purgeAuditRow{}
	for rows.Next() {
		var class, id string
		var row purgeAuditRow
		if err := rows.Scan(&class, &id, &row.mode, &row.policyVersion, &row.purgeAfter, &row.purgedAt); err != nil {
			t.Fatalf("scan purge audit: %v", err)
		}
		key := class + ":" + id
		if _, duplicate := out[key]; duplicate {
			t.Fatalf("two audit rows for %s; the UNIQUE constraint did not hold", key)
		}
		out[key] = row
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate purge audit: %v", err)
	}
	return out
}

type canonicalEventRow struct {
	payload        string
	classification string
	correlationID  string
	recordedAt     time.Time
	purgedAt       *time.Time
}

func readCanonicalEvent(t *testing.T, ctx context.Context, db *sql.DB, eventID string) canonicalEventRow {
	t.Helper()
	var row canonicalEventRow
	if err := db.QueryRowContext(ctx, `
		SELECT payload_json::text, classification, correlation_id, recorded_at, purged_at
		FROM integration_canonical_events WHERE event_id = $1
	`, eventID).Scan(&row.payload, &row.classification, &row.correlationID, &row.recordedAt, &row.purgedAt); err != nil {
		t.Fatalf("read canonical event %s: %v", eventID, err)
	}
	return row
}

type exportRow struct {
	record      string
	principalID string
	reason      string
	purgedAt    *time.Time
}

func readExport(t *testing.T, ctx context.Context, db *sql.DB, exportID string) exportRow {
	t.Helper()
	var row exportRow
	if err := db.QueryRowContext(ctx, `
		SELECT record_json::text, principal_json ->> 'id', reason, purged_at
		FROM integration_session_exports WHERE export_id = $1
	`, exportID).Scan(&row.record, &row.principalID, &row.reason, &row.purgedAt); err != nil {
		t.Fatalf("read export %s: %v", exportID, err)
	}
	return row
}

type retentionCounts struct {
	events   int
	receipts int
	lineage  int
	attempts int
}

func countRetentionRows(t *testing.T, ctx context.Context, db *sql.DB) retentionCounts {
	t.Helper()
	var out retentionCounts
	if err := db.QueryRowContext(ctx, `
		SELECT
			(SELECT count(*) FROM integration_canonical_events),
			(SELECT count(*) FROM integration_receipts),
			(SELECT count(*) FROM integration_message_lineage),
			(SELECT count(*) FROM integration_delivery_attempts)
	`).Scan(&out.events, &out.receipts, &out.lineage, &out.attempts); err != nil {
		t.Fatalf("count durable rows: %v", err)
	}
	return out
}

func countRows(t *testing.T, ctx context.Context, db *sql.DB, query string, args ...any) int {
	t.Helper()
	var count int
	if err := db.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	return count
}

// dumpColumn concatenates every value of one column so an assertion can prove a
// sentinel is absent from the WHOLE table rather than from one row it chose.
func dumpColumn(t *testing.T, ctx context.Context, db *sql.DB, table, expression string) string {
	t.Helper()
	var dump sql.NullString
	query := fmt.Sprintf(`SELECT string_agg(%s::text, '|') FROM %s`, expression, table) // #nosec G201 -- fixed identifiers from this test only
	if err := db.QueryRowContext(ctx, query).Scan(&dump); err != nil {
		t.Fatalf("dump %s.%s: %v", table, expression, err)
	}
	return dump.String
}

func columnExists(t *testing.T, ctx context.Context, db *sql.DB, table, column string) bool {
	t.Helper()
	var exists bool
	if err := db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_name = $1 AND column_name = $2
			  AND table_schema = current_schema()
		)
	`, table, column).Scan(&exists); err != nil {
		t.Fatalf("inspect %s.%s: %v", table, column, err)
	}
	return exists
}

func jsonEqual(t *testing.T, got, want string) bool {
	t.Helper()
	var left, right any
	if err := json.Unmarshal([]byte(got), &left); err != nil {
		return false
	}
	if err := json.Unmarshal([]byte(want), &right); err != nil {
		t.Fatalf("malformed expected JSON %q: %v", want, err)
	}
	gotCanonical, err := json.Marshal(left)
	if err != nil {
		return false
	}
	wantCanonical, err := json.Marshal(right)
	if err != nil {
		t.Fatalf("re-encode expected JSON: %v", err)
	}
	return string(gotCanonical) == string(wantCanonical)
}

// ---------------------------------------------------------------------------
// Schema provisioning for the negative controls
// ---------------------------------------------------------------------------

// applyPre41eSchema reconstructs the schema exactly as it stood before this
// slice: processor 0001-0004 and session 0001-0005. It reads the migration files
// from disk because the embedded ledgers always apply the full current set,
// which is precisely what a negative control must not do.
func applyPre41eSchema(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()
	applyMigrationFiles(t, ctx, db, []string{
		"../processor/migrations/0001_atomic_submission.sql",
		"../processor/migrations/0002_delivery_reliability.sql",
		"../processor/migrations/0003_operator_control_plane.sql",
		"../processor/migrations/0004_audit_immutability.sql",
		"../session/migrations/0001_session_workspace.sql",
		"../session/migrations/0002_workflow_simulations.sql",
		"../session/migrations/0003_publications.sql",
		"../session/migrations/0004_export_attribution.sql",
		"../session/migrations/0005_session_stream_events.sql",
	})
}

// applyPreC1Schema reconstructs the schema before Slice 4.1d C1 added any
// immutability guard, which is the only schema on which the spec's step-4
// mutations can succeed.
func applyPreC1Schema(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()
	applyMigrationFiles(t, ctx, db, []string{
		"../processor/migrations/0001_atomic_submission.sql",
		"../processor/migrations/0002_delivery_reliability.sql",
		"../processor/migrations/0003_operator_control_plane.sql",
	})
}

func applyMigrationFiles(t *testing.T, ctx context.Context, db *sql.DB, paths []string) {
	t.Helper()
	for _, path := range paths {
		body, err := os.ReadFile(filepath.Clean(path))
		if err != nil {
			t.Fatalf("read migration %s: %v", path, err)
		}
		if _, err := db.ExecContext(ctx, string(body)); err != nil {
			t.Fatalf("apply migration %s: %v", path, err)
		}
	}
}

// seedRawCanonicalEvent writes one dependent-free receipt and event through
// direct SQL, which is the only way to populate a schema whose production store
// would apply the current migration set.
func seedRawCanonicalEvent(t *testing.T, ctx context.Context, db *sql.DB, receiptID, eventID, sentinel string) {
	t.Helper()
	statements := []string{
		`INSERT INTO integration_receipts (
			tenant_id, receipt_id, idempotency_key, request_fingerprint, integration_revision,
			status, recorded_at, correlation_id, raw_retention_mode, principal_json, reason, result_json
		) VALUES ('tenant-a', '` + receiptID + `', 'idem-` + receiptID + `', 'fp-` + receiptID + `', '{}'::jsonb,
			'accepted', now(), 'corr-` + receiptID + `', 'ephemeral', '{"id":"svc"}'::jsonb, '', '{}'::jsonb)`,
		`INSERT INTO integration_canonical_events (
			tenant_id, event_id, receipt_id, event_type, source_message_id,
			correlation_id, classification, payload_json, recorded_at
		) VALUES ('tenant-a', '` + eventID + `', '` + receiptID + `', 'patient_admit', 'msg-` + eventID + `',
			'corr-` + eventID + `', 'phi', '{"mrn":"` + sentinel + `"}'::jsonb, now() - interval '30 days')`,
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			t.Fatalf("seed raw canonical event: %v", err)
		}
	}
}

// ---------------------------------------------------------------------------
// Message fixtures
// ---------------------------------------------------------------------------

func retentionHL7Message(controlID, mrn string) string {
	return "MSH|^~\\&|APP|FAC|EHR|HOSPITAL|20260713120000-0400||ADT^A01^ADT_A01|" + controlID + "|P|2.5.1\r" +
		"EVN|A01|20260713120000||||20260713115900-0400\r" +
		"PID|1||" + mrn + "^^^HOSP^MR||Patient^Test||19800101|F"
}

const retentionProfileJSON = `{"hl7v2":{"default_version":"2.5.1","timezone":"UTC","tolerance":{"missing_segments":["PV1"],"nte_anywhere":false,"extra_components":false,"unknown_segments":false,"non_standard_delimiters":false},"event_classifications":[{"message_type":"ADT^A01","event_type":"patient_admit","priority":1}]},"identifiers":{"assigning_authorities":[{"code":"HOSP","system":"urn:oid:1.2.3"}],"normalization":{"ssn_strip_dashes":true,"phone_normalize":false}}}`

const retentionWorkflowYAML = `dsl_version: "1"
name: adt-retention
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
