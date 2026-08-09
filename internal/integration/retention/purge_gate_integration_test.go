//go:build integration

package retention

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/lib/pq"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/processor"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/session"
)

// TestPhiRetention_PurgeIsStructurallyBlockedToday is Slice 4.1e's day-1 gate.
//
// It exists to kill the framing every planning document in front of this lane
// carried — that retention is a policy-design problem and the purge itself is a
// DELETE statement in a lease-fenced sweeper
// (docs/operations/PHI-RETENTION.md section 6 as it then read, and
// .loom/slice-handoff-phase-4-slice-4-1d-c1-phi-audit.md:187-195).
//
// It shipped first as a standalone test-only MR and PASSED against the unmodified
// schema, which is what forced the exemption decision before any migration was
// written. It is kept, unchanged in substance, as a permanent regression guard:
// every assertion below is still true after Slice 4.1e's column-scoped exemption,
// and that is exactly the claim worth guarding. If the exemption ever widens into
// a general write path, this test goes red before anything else does:
//
//	(a) DELETE of a dependent-free canonical event raises. The purge cannot
//	    delete the row.
//	(b) UPDATE of that row's payload_json — redaction, the only other shape a
//	    purge can take — ALSO raises. C1's guard is blanket
//	    (internal/integration/processor/migrations/0004_audit_immutability.sql:29-32),
//	    so it removed both mechanisms, not just one. No document said so.
//	(c) For an exported session, DELETE of the export row raises on C1's
//	    append-only trigger
//	    (internal/integration/session/migrations/0004_export_attribution.sql:55-58)
//	    and DELETE of the session row raises on the export's foreign key
//	    (internal/integration/session/migrations/0001_session_workspace.sql:88-90,
//	    no ON DELETE clause, i.e. NO ACTION). Once a session is exported both rows
//	    are permanently undeletable, while PHI-RETENTION.md:191 promises export
//	    TTL in the very next slice.
//
// Assertions (a) and (c) are aimed at DEPENDENT-FREE rows on purpose. A canonical
// event that still has lineage or an attempt would be refused by the existing
// ON DELETE RESTRICT foreign keys, which proves referential integrity rather than
// immutability — the distinction C1's own kill-test draws
// (internal/integration/session/phi_audit_integration_test.go:249-254). The one
// place a foreign key IS the subject is the second half of (c), and there the
// test asserts the SQLSTATE explicitly so a trigger cannot be mistaken for it.
//
// The one thing this test deliberately does NOT assert is that a purge is
// impossible: after Slice 4.1e it is possible, through exactly one shape, and
// TestPhiRetention_PostgresExpiryPurgeAndAuditedTombstone proves that shape works
// while this test proves nothing else does.
//
// If this test ever fails, the exemption recorded in .loom/40-decisions.md
// (2026-08-08, "Slice 4.1e") has grown beyond a tombstone.
func TestPhiRetention_PurgeIsStructurallyBlockedToday(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	db := newRetentionGateSchema(t, ctx, requireRetentionPostgres(t), "phi_retention_gate")
	migrateDurableSchema(t, ctx, db)
	// The seeded rows are never stamped with a purge_after, so no exemption is
	// reachable for them and the assertions below test the guards, not the policy.
	seedRetentionGateRecords(t, ctx, db)

	before := countRetentionGateRows(t, ctx, db)
	if before.events == 0 || before.exports == 0 || before.sessions == 0 {
		t.Fatalf("a guarded table is empty, so the assertions below would pass vacuously: %+v", before)
	}

	t.Run("a_canonical_event_delete_raises", func(t *testing.T) {
		_, err := db.ExecContext(ctx,
			`DELETE FROM integration_canonical_events WHERE event_id = $1`, gateEventID)
		assertRaised(t, err, "append-only",
			"a dependent-free canonical event was deletable; the purge could be a DELETE after all")
	})

	t.Run("b_canonical_event_payload_redaction_also_raises", func(t *testing.T) {
		_, err := db.ExecContext(ctx,
			`UPDATE integration_canonical_events SET payload_json = '{}'::jsonb WHERE event_id = $1`,
			gateEventID)
		assertRaised(t, err, "append-only",
			"payload redaction succeeded; C1's guard is not blanket and correction 11 is wrong")
	})

	t.Run("c_exported_session_and_its_export_are_both_undeletable", func(t *testing.T) {
		_, err := db.ExecContext(ctx,
			`DELETE FROM integration_session_exports WHERE export_id = $1`, gateExportID)
		assertRaised(t, err, "append-only",
			"an export record was deletable; correction 13 is wrong")

		_, err = db.ExecContext(ctx,
			`DELETE FROM integration_sessions WHERE session_id = $1`, gateSessionID)
		if err == nil {
			t.Fatal("an exported session was deletable; correction 13 is wrong")
		}
		// The subject here is the foreign key, not a trigger: integration_sessions
		// carries no immutability guard at all. Asserting the SQLSTATE keeps the two
		// mechanisms from being confused for one another.
		var pgErr *pq.Error
		if !errors.As(err, &pgErr) || string(pgErr.Code) != foreignKeyViolation {
			t.Fatalf("session DELETE failed with %v; want SQLSTATE %s (foreign_key_violation)",
				err, foreignKeyViolation)
		}
		if !strings.Contains(err.Error(), "integration_session_exports") {
			t.Fatalf("session DELETE was refused by something other than the export foreign key: %v", err)
		}
	})

	after := countRetentionGateRows(t, ctx, db)
	if after != before {
		t.Fatalf("row counts moved despite every mutation raising: before=%+v after=%+v", before, after)
	}
	t.Logf("purge is structurally blocked on unmodified main: DELETE raises, redaction UPDATE raises, "+
		"exported session and export row are both permanently undeletable; counts unchanged at %+v", after)
}

const (
	gateReceiptID       = "rcpt-retention-gate"
	gateEventID         = "evt-retention-gate"
	gateSessionID       = "sess-retention-gate"
	gateExportID        = "export-retention-gate"
	foreignKeyViolation = "23503"
)

// assertRaised requires that a statement failed with a plpgsql RAISE carrying the
// expected message. An error of any other class would mean the statement was
// blocked by something other than the guard under test.
func assertRaised(t *testing.T, err error, want, unexpected string) {
	t.Helper()
	if err == nil {
		t.Fatal(unexpected)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("statement raised %q, want a message containing %q", err, want)
	}
}

type retentionGateCounts struct {
	events   int
	sessions int
	exports  int
}

func countRetentionGateRows(t *testing.T, ctx context.Context, db *sql.DB) retentionGateCounts {
	t.Helper()
	var out retentionGateCounts
	if err := db.QueryRowContext(ctx, `
		SELECT
			(SELECT count(*) FROM integration_canonical_events),
			(SELECT count(*) FROM integration_sessions),
			(SELECT count(*) FROM integration_session_exports)
	`).Scan(&out.events, &out.sessions, &out.exports); err != nil {
		t.Fatalf("count guarded rows: %v", err)
	}
	return out
}

func requireRetentionPostgres(t *testing.T) string {
	t.Helper()
	base := os.Getenv("POSTGRES_TEST_URL")
	if base == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("POSTGRES_TEST_URL is required in CI")
		}
		t.Skip("POSTGRES_TEST_URL is required for the retention purge proofs")
	}
	return base
}

// newRetentionGateSchema provisions one independently migrated schema, dropped on
// cleanup. Every proof in this package gets its own so a negative control can
// never share a schema with the thing it is controlling.
func newRetentionGateSchema(t *testing.T, ctx context.Context, base, prefix string) *sql.DB {
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
		_, _ = admin.ExecContext(context.Background(),
			`DROP SCHEMA IF EXISTS `+pq.QuoteIdentifier(schema)+` CASCADE`)
		_ = admin.Close()
	})
	return db
}

// migrateDurableSchema applies the shipped processor and session migration sets
// through the production stores, so the gate asserts against the schema the
// runtime actually creates rather than a hand-assembled copy of it.
func migrateDurableSchema(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()
	submissions, err := processor.NewPostgresSubmissionStore(db, processor.PostgresSubmissionConfig{})
	if err != nil {
		t.Fatalf("NewPostgresSubmissionStore: %v", err)
	}
	if err := submissions.Migrate(ctx); err != nil {
		t.Fatalf("processor Migrate: %v", err)
	}
	sessions, err := session.NewPostgresStore(db, session.PostgresConfig{TenantID: "tenant-a"})
	if err != nil {
		t.Fatalf("session.NewPostgresStore: %v", err)
	}
	if err := sessions.Migrate(ctx); err != nil {
		t.Fatalf("session Migrate: %v", err)
	}
}

// seedRetentionGateRecords writes one dependent-free canonical event chain and one
// exported session. INSERT is guarded by nothing in the current schema, so direct
// SQL is the honest way to place a row the guards will then refuse to remove.
func seedRetentionGateRecords(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()
	statements := []string{
		`INSERT INTO integration_receipts (
			tenant_id, receipt_id, idempotency_key, request_fingerprint, integration_revision,
			status, recorded_at, correlation_id, raw_retention_mode, principal_json, reason, result_json
		) VALUES ('tenant-a', '` + gateReceiptID + `', 'idem-gate', 'fingerprint-gate', '{}'::jsonb,
			'accepted', now(), 'corr-gate', 'ephemeral', '{"id":"svc-gate"}'::jsonb, '', '{}'::jsonb)`,
		`INSERT INTO integration_canonical_events (
			tenant_id, event_id, receipt_id, event_type, source_message_id,
			correlation_id, classification, payload_json, recorded_at
		) VALUES ('tenant-a', '` + gateEventID + `', '` + gateReceiptID + `', 'patient_admit', 'msg-gate',
			'corr-gate', 'phi', '{"mrn":"RETENTION-GATE-SENTINEL"}'::jsonb, now())`,
		`INSERT INTO integration_sessions (tenant_id, session_id, status, created_at, record_json)
		 VALUES ('tenant-a', '` + gateSessionID + `', 'active', now(),
			'{"id":"` + gateSessionID + `","name":"retention gate","status":"active"}'::jsonb)`,
		`INSERT INTO integration_session_exports (
			tenant_id, session_id, export_id, exported_at, record_json,
			principal_json, reason, include_raw_payload
		) VALUES ('tenant-a', '` + gateSessionID + `', '` + gateExportID + `', now(),
			'{"id":"` + gateExportID + `"}'::jsonb,
			'{"id":"privacy-officer-1","kind":"human","auth_method":"oidc"}'::jsonb,
			'retention gate fixture', false)`,
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			t.Fatalf("seed retention gate record: %v", err)
		}
	}
}
