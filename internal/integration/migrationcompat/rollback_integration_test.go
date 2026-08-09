//go:build integration

package migrationcompat

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/lib/pq"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/session"
)

// notNullViolation is PostgreSQL's SQLSTATE for a NOT NULL constraint failure.
const notNullViolation = pq.ErrorCode("23502")

// unattributedLegacySentinel is the marker
// internal/integration/session/migrations/0004_export_attribution.sql:26 already
// writes into pre-4.1d rows it cannot attribute after the fact. A rollback-safe
// schema must produce the same visible marker for a rollback-era insert, for the
// same reason: an export nobody recorded a principal for must stay visibly
// unattributed rather than either failing or being retroactively vouched for.
const unattributedLegacySentinel = "unattributed_legacy_export"

// TestMigrationCompatibility_ExportInsertShapeSurvivesOneVersionRollback is Lane
// S4-C's day-1 gate (.loom/32-sprint4-execution-specs.md, "Lane S4-C —
// Kill-Test"). It exists to reproduce a found defect BEFORE the fix, which is
// what distinguishes it from a green test written afterwards.
//
// The product spec's budget 6 is "one-version rolling upgrade and rollback
// preserve receipts, revisions, and resumable work without schema downgrade
// corruption" (.loom/20-product-spec-integration-engine-ide-completion.md:279-280).
// During a rolling upgrade or a rollback, a binary one version behind runs
// against the already-migrated schema. Slice 4.1d C1's own migration set
// integration_session_exports.principal_json, reason, and include_raw_payload
// NOT NULL with no DEFAULT (0004_export_attribution.sql:31-34). The pre-4.1d
// binary issues the five-column insert; the current binary issues the
// eight-column form (internal/integration/session/postgres.go:949-954).
//
// So on unmodified main this test MUST FAIL with a not-null violation on
// principal_json. That failure is the finding. Any other failure means
// correction 23 of .loom/32 is wrong and the lane re-scopes; a pass means the
// defect does not exist and .loom/32 gets corrected before any code is written.
//
// After task 2 lands the three server-side DEFAULTs, the same insert succeeds
// and the row is visibly unattributed — and this test becomes a permanent
// regression guard for rollback safety.
func TestMigrationCompatibility_ExportInsertShapeSurvivesOneVersionRollback(t *testing.T) {
	ctx := t.Context()
	dsn := requireCompatDSN(t)
	schema := newCompatSchema(t, dsn, "migration_compat_rollback")
	db := openCompatDB(t, compatSchemaDSN(t, dsn, schema))

	// Migrate to the CURRENT session ledger head. This is the "upgraded schema"
	// half of a one-version rollback: the schema moved forward, the binary did
	// not.
	store, err := session.NewPostgresStore(db, session.PostgresConfig{TenantID: compatTenantID})
	if err != nil {
		t.Fatalf("NewPostgresStore: %v", err)
	}
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("migrate session ledger to head: %v", err)
	}
	assertSessionLedgerAtHead(ctx, t, db)

	const sessionID = "sess-rollback-compat"
	seedSession(t, db, sessionID)

	// The exact pre-4.1d five-column insert. Written literally rather than
	// through the Go writer, because the point is that a binary that predates
	// the attribution columns cannot name them.
	_, insertErr := db.ExecContext(ctx, `
		INSERT INTO integration_session_exports
			(tenant_id, session_id, export_id, exported_at, record_json)
		VALUES ($1, $2, $3, $4, $5)
	`, compatTenantID, sessionID, "export-rollback-compat",
		time.Date(2026, 8, 8, 9, 5, 0, 0, time.UTC),
		`{"id":"export-rollback-compat","session":{"id":"sess-rollback-compat"}}`)

	if insertErr != nil {
		var pqErr *pq.Error
		if errors.As(insertErr, &pqErr) &&
			pqErr.Code == notNullViolation &&
			(pqErr.Column == "principal_json" || strings.Contains(pqErr.Message, "principal_json")) {
			t.Fatalf("DAY-1 GATE CONFIRMED — one-version rollback is broken today, exactly as "+
				".loom/32-sprint4-execution-specs.md correction 23 predicts.\n"+
				"  The pre-4.1d five-column INSERT INTO integration_session_exports fails against "+
				"the migrated schema:\n"+
				"    SQLSTATE %s (not_null_violation) on column %q of relation %q\n"+
				"    %s\n"+
				"  Cause: internal/integration/session/migrations/0004_export_attribution.sql:31-34 "+
				"sets principal_json, reason, and include_raw_payload NOT NULL with no DEFAULT.\n"+
				"  Fix (Lane S4-C task 2): server-side DEFAULTs using the same "+
				"unattributed_legacy_export sentinel the migration already backfills with.",
				pqErr.Code, pqErr.Column, pqErr.Table, pqErr.Message)
		}
		t.Fatalf("DAY-1 GATE INCONCLUSIVE — the rollback-era insert failed, but NOT with a "+
			"not-null violation on principal_json.\n"+
			"  err = %v\n"+
			"  .loom/32 correction 23 predicted SQLSTATE 23502 on principal_json. A different "+
			"failure means the lane's premise is wrong: correct .loom/32 and re-scope before "+
			"writing production code.", insertErr)
	}

	// Post-fix behaviour. Reached only once task 2 has landed.
	var principalJSON []byte
	var reason string
	var includeRaw bool
	if err := db.QueryRowContext(ctx, `
		SELECT principal_json, reason, include_raw_payload
		FROM integration_session_exports
		WHERE tenant_id = $1 AND export_id = $2
	`, compatTenantID, "export-rollback-compat").Scan(&principalJSON, &reason, &includeRaw); err != nil {
		t.Fatalf("read rollback-era export row: %v", err)
	}

	if !strings.Contains(string(principalJSON), unattributedLegacySentinel) {
		t.Fatalf("rollback-era export row is not visibly unattributed: principal_json = %s, "+
			"want it to carry the %q sentinel so a disclosure nobody recorded a principal for is "+
			"distinguishable from one that was attributed",
			principalJSON, unattributedLegacySentinel)
	}
	if strings.TrimSpace(reason) == "" {
		t.Fatal("rollback-era export row has an empty reason; the DEFAULT must satisfy " +
			"integration_session_exports_reason_check (1-1024 bytes)")
	}
	if includeRaw {
		t.Fatal("rollback-era export row defaulted include_raw_payload to true; an insert that " +
			"cannot name the column must never imply a raw-PHI disclosure")
	}

	t.Logf("one-version rollback safe: pre-4.1d five-column export insert succeeded and produced "+
		"a visibly unattributed row (principal_json = %s, reason = %q, include_raw_payload = %t)",
		principalJSON, reason, includeRaw)
}

// assertSessionLedgerAtHead fails the test if the session ledger is not at the
// version this proof assumes. Without it, a future migration could leave the
// gate asserting against a schema that no longer has the columns in question,
// and it would pass for the wrong reason.
func assertSessionLedgerAtHead(ctx context.Context, t *testing.T, db *sql.DB) {
	t.Helper()
	var head int
	if err := db.QueryRowContext(ctx, `
		SELECT coalesce(max(version), 0) FROM integration_session_schema_migrations
	`).Scan(&head); err != nil {
		t.Fatalf("read session migration ledger: %v", err)
	}
	if head < 4 {
		t.Fatalf("session ledger is at version %d; the export attribution migration (version 4) "+
			"has not been applied, so this gate would assert against the wrong schema", head)
	}
}
