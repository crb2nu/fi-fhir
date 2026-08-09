//go:build integration

package migrationcompat

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/delivery"
)

// TestMigrationCompatibility_ConcurrentReplicaMigrationRollbackAndRestore is
// Slice 4.4a's kill-test. It establishes the CI-runnable half of the product
// spec's budget 6 — "one-version rolling upgrade and rollback preserve
// receipts, revisions, and resumable work without schema downgrade corruption"
// (.loom/20-product-spec-integration-engine-ide-completion.md:279-280) — plus
// the restore half of budget 5 that does not need WAL archiving.
//
// Five assertions, each of which was false or unproven before this slice:
//
//  1. two replicas migrating one fresh database concurrently both succeed,
//     across all six ledgers including terminology, which took no advisory
//     lock until now (.loom/32 correction 25);
//  2. the same holds against a database already at head, which is what every
//     restart after the first actually does;
//  3. every ledger's declared SchemaVersion equals the version actually
//     applied, so the number `fi-fhir version` and fi_fhir_schema_ledger_version
//     report cannot drift from the schema;
//  4. a pg_dump/restore round-trip through the documented runbook preserves
//     every durable row, every immutability trigger, and the 4.1c-a NOT VALID
//     provenance CHECK — a dump that silently drops a trigger is a PHI
//     governance regression, not a backup;
//  5. the delivery worker claims and publishes a queued attempt from the
//     restored database with no manual repair.
func TestMigrationCompatibility_ConcurrentReplicaMigrationRollbackAndRestore(t *testing.T) {
	ctx := t.Context()
	dsn := requireCompatDSN(t)

	t.Run("concurrent replicas converge on a fresh database", func(t *testing.T) {
		freshDSN, _ := newCompatDatabase(t, dsn, "migcompat_fresh")
		db := openCompatDB(t, freshDSN)
		assertConcurrentMigrationSucceeds(ctx, t, db)
		assertEveryLedgerAtDeclaredVersion(ctx, t, db)
	})

	t.Run("concurrent replicas converge on an already-migrated database", func(t *testing.T) {
		// The steady state of a rolling upgrade: the schema is already at head
		// and the arriving replicas must recognise that rather than reapply.
		headDSN, _ := newCompatDatabase(t, dsn, "migcompat_head")
		db := openCompatDB(t, headDSN)
		if err := migrateEveryLedger(ctx, db, compatTenantID); err != nil {
			t.Fatalf("initial migration: %v", err)
		}
		assertConcurrentMigrationSucceeds(ctx, t, db)
		assertEveryLedgerAtDeclaredVersion(ctx, t, db)
	})

	t.Run("concurrent replicas converge on a database one version back", func(t *testing.T) {
		// The upgrade path, which is what a rolling upgrade actually runs: the
		// database is at N-1 and two arriving replicas both try to advance it.
		// This is distinct from the two cases above — fresh-install and no-op —
		// and it is the one the terminology migrator's missing lock broke,
		// because both replicas would observe "needs v3" and both would apply it.
		//
		// Terminology is the ledger seeded here because it is the only one whose
		// migration bodies are exported, and it is the ledger slice 4.4a fixed.
		// The five integration ledgers apply every step inside one locked
		// transaction, so their N-1 state is not reachable without exporting
		// their embedded SQL — which would widen those packages' API to serve a
		// test. Recorded rather than silently skipped.
		backDSN, _ := newCompatDatabase(t, dsn, "migcompat_nminus1")
		db := openCompatDB(t, backDSN)
		seedTerminologyAtVersion(ctx, t, db, 2)
		assertConcurrentMigrationSucceeds(ctx, t, db)
		assertEveryLedgerAtDeclaredVersion(ctx, t, db)
	})

	t.Run("restore round-trip preserves rows, guards, and resumable work", func(t *testing.T) {
		sourceDSN, _ := newCompatDatabase(t, dsn, "migcompat_src")
		_, targetName := newCompatDatabase(t, dsn, "migcompat_dst")
		targetDSN := splitDSN(dsn, targetName)

		source := openCompatDB(t, sourceDSN)
		if err := migrateEveryLedger(ctx, source, compatTenantID); err != nil {
			t.Fatalf("migrate source database: %v", err)
		}
		fixture := seedDurableFixture(ctx, t, source)
		assertLivePathStillAttributesExports(ctx, t, source, fixture)
		before := durableRowCounts(ctx, t, source)

		// pg_dump holds no lock that would block this, but closing the pool
		// first keeps the dump from racing a lingering idle transaction.
		if err := source.Close(); err != nil {
			t.Fatalf("close source pool before dump: %v", err)
		}
		runRoundTripScript(t, sourceDSN, targetName)

		restored := openCompatDB(t, targetDSN)
		after := durableRowCounts(ctx, t, restored)
		assertRowCountsEqual(t, before, after)
		assertPHISurvived(ctx, t, restored, fixture)
		assertImmutabilityGuardsSurvived(ctx, t, restored, fixture)
		assertProvenanceCheckSurvivedAndIsStillNotValid(ctx, t, restored)
		assertQueuedAttemptResumes(ctx, t, restored, fixture)
	})
}

// assertConcurrentMigrationSucceeds is assertion 1/2. Two goroutines is the
// smallest configuration that can expose the race; the failure mode without a
// lock is a duplicate-object error from whichever replica loses.
func assertConcurrentMigrationSucceeds(ctx context.Context, t *testing.T, db *sql.DB) {
	t.Helper()
	const replicas = 2

	var wg sync.WaitGroup
	errs := make([]error, replicas)
	start := make(chan struct{})
	for i := range replicas {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs[i] = migrateEveryLedger(ctx, db, compatTenantID)
		}()
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("replica %d failed to migrate concurrently: %v\n"+
				"  Two replicas starting at once is an ordinary rolling upgrade, not an "+
				"edge case. Every migrator must take its advisory lock and re-read its "+
				"ledger version inside it.", i, err)
		}
	}
	t.Logf("assertion PASSED: %d replicas migrated all six ledgers concurrently with no error", replicas)
}

// assertEveryLedgerAtDeclaredVersion is assertion 3. It is the guard that keeps
// the reported compatibility boundary honest: a migration added without
// bumping the package's SchemaVersion would make `fi-fhir version` and
// fi_fhir_schema_ledger_version lie about what the binary expects.
func assertEveryLedgerAtDeclaredVersion(ctx context.Context, t *testing.T, db *sql.DB) {
	t.Helper()
	for _, ledger := range ledgerExpectations() {
		var applied int
		if err := db.QueryRowContext(ctx, ledger.query).Scan(&applied); err != nil {
			t.Fatalf("read %s ledger version: %v", ledger.name, err)
		}
		if applied != ledger.declared {
			t.Fatalf("%s ledger: applied version %d, but the package declares SchemaVersion = %d.\n"+
				"  The declared version is what `fi-fhir version` and "+
				"fi_fhir_schema_ledger_version report, and it is the compatibility boundary "+
				"an operator uses to decide whether a rollback is safe. It must not drift "+
				"from the schema.", ledger.name, applied, ledger.declared)
		}

		// A ledger must hold exactly one row per version. Two rows for one
		// version would mean a migration applied twice.
		if ledger.name == "terminology" {
			continue // terminology.schema_version has version as its primary key
		}
		var rows, distinct int
		table := strings.TrimPrefix(ledger.query, `SELECT coalesce(max(version), 0) FROM `)
		if err := db.QueryRowContext(ctx,
			fmt.Sprintf(`SELECT count(*), count(DISTINCT version) FROM %s`, table),
		).Scan(&rows, &distinct); err != nil {
			t.Fatalf("count %s ledger rows: %v", ledger.name, err)
		}
		if rows != distinct || rows != ledger.declared {
			t.Fatalf("%s ledger holds %d rows across %d distinct versions, want %d of each",
				ledger.name, rows, distinct, ledger.declared)
		}
	}
	t.Logf("assertion PASSED: all six ledgers are at their declared SchemaVersion with one row per version")
}

// assertLivePathStillAttributesExports is the counterweight to slice 4.4a's
// rollback DEFAULTs. The DEFAULT exists so an N-1 binary's insert survives; it
// must never make the *current* writer's omission survivable too, because that
// would turn a loud attribution bug into a silent one.
func assertLivePathStillAttributesExports(ctx context.Context, t *testing.T, db *sql.DB, fixture durableFixture) {
	t.Helper()
	var principalJSON []byte
	var reason string
	if err := db.QueryRowContext(ctx, `
		SELECT principal_json, reason FROM integration_session_exports
		WHERE tenant_id = $1 AND export_id = $2
	`, compatTenantID, fixture.ExportID).Scan(&principalJSON, &reason); err != nil {
		t.Fatalf("read live-path export row: %v", err)
	}
	if strings.Contains(string(principalJSON), unattributedLegacySentinel) {
		t.Fatalf("the current export writer produced an UNATTRIBUTED row: principal_json = %s.\n"+
			"  Slice 4.4a's DEFAULTs exist for an N-1 binary that cannot name the column. "+
			"The live path names it, so a sentinel here means the writer regressed and the "+
			"DEFAULT hid it.", principalJSON)
	}
	if !strings.Contains(string(principalJSON), "privacy-officer-compat") {
		t.Fatalf("live-path export lost its principal: principal_json = %s", principalJSON)
	}
	if reason != "migration compatibility round-trip fixture" {
		t.Fatalf("live-path export lost its reason: %q", reason)
	}
}

// durableClasses are the tables a restore has to bring back intact. The first
// five are the durable classes the PHI/egress contract enumerates
// (.loom/32 correction 9); the rest are the session workspace and the identity
// provenance ledger.
var durableClasses = []string{
	"integration_receipts",
	"integration_canonical_events",
	"integration_message_lineage",
	"integration_delivery_attempts",
	"integration_delivery_outbox",
	"integration_sessions",
	"integration_session_exports",
	"integration_delivery_identity_decisions",
}

func durableRowCounts(ctx context.Context, t *testing.T, db *sql.DB) map[string]int {
	t.Helper()
	counts := make(map[string]int, len(durableClasses))
	for _, table := range durableClasses {
		var count int
		if err := db.QueryRowContext(ctx, fmt.Sprintf(`SELECT count(*) FROM %s`, table)).Scan(&count); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		counts[table] = count
	}
	return counts
}

func assertRowCountsEqual(t *testing.T, before, after map[string]int) {
	t.Helper()
	for _, table := range durableClasses {
		if before[table] == 0 {
			t.Fatalf("the fixture seeded no rows into %s, so comparing counts would be vacuous", table)
		}
		if before[table] != after[table] {
			t.Fatalf("restore lost rows in %s: %d before, %d after", table, before[table], after[table])
		}
	}
	t.Logf("assertion PASSED: row counts identical across %d durable classes after restore", len(durableClasses))
}

// assertPHISurvived proves the comparison is about content, not just counts: a
// restore that produced the right number of empty rows would pass a count
// check.
func assertPHISurvived(ctx context.Context, t *testing.T, db *sql.DB, fixture durableFixture) {
	t.Helper()
	var payload string
	if err := db.QueryRowContext(ctx, `
		SELECT payload_json->>'patient_name' FROM integration_canonical_events
		WHERE tenant_id = $1 AND event_id = $2
	`, compatTenantID, fixture.EventID).Scan(&payload); err != nil {
		t.Fatalf("read restored canonical event payload: %v", err)
	}
	if payload != fixture.PHISentinel {
		t.Fatalf("restored canonical event payload = %q, want %q", payload, fixture.PHISentinel)
	}
}

// assertImmutabilityGuardsSurvived is the assertion that makes this a PHI
// governance proof rather than a row count. S3-C1's guarantee is that the
// schema, not convention, refuses mutation. A dump/restore that recreated the
// tables without their triggers would leave a database that looks complete and
// silently permits every mutation C1 forbids.
func assertImmutabilityGuardsSurvived(ctx context.Context, t *testing.T, db *sql.DB, fixture durableFixture) {
	t.Helper()

	mutations := []struct {
		name  string
		query string
		args  []any
	}{
		{"delete a canonical event",
			`DELETE FROM integration_canonical_events WHERE tenant_id = $1 AND event_id = $2`,
			[]any{compatTenantID, fixture.EventID}},
		{"redact a canonical event payload",
			`UPDATE integration_canonical_events SET payload_json = '{}'::jsonb WHERE tenant_id = $1 AND event_id = $2`,
			[]any{compatTenantID, fixture.EventID}},
		{"delete a lineage row",
			`DELETE FROM integration_message_lineage WHERE tenant_id = $1 AND lineage_id = $2`,
			[]any{compatTenantID, fixture.LineageID}},
		{"delete a receipt",
			`DELETE FROM integration_receipts WHERE tenant_id = $1 AND receipt_id = $2`,
			[]any{compatTenantID, fixture.ReceiptID}},
		{"delete a delivery attempt",
			`DELETE FROM integration_delivery_attempts WHERE tenant_id = $1 AND attempt_id = $2`,
			[]any{compatTenantID, fixture.AttemptID}},
		{"mutate a session export",
			`UPDATE integration_session_exports SET reason = 'rewritten' WHERE tenant_id = $1 AND export_id = $2`,
			[]any{compatTenantID, fixture.ExportID}},
	}

	for _, mutation := range mutations {
		if _, err := db.ExecContext(ctx, mutation.query, mutation.args...); err == nil {
			t.Fatalf("restored database ALLOWED %q. The C1 immutability guard did not survive "+
				"pg_dump/restore, so the restored deployment has weaker PHI governance than the "+
				"one it replaced.", mutation.name)
		}
	}

	after := durableRowCounts(ctx, t, db)
	for _, table := range durableClasses {
		if after[table] == 0 {
			t.Fatalf("%s is empty after the refused mutations; one of them partially applied", table)
		}
	}
	t.Logf("assertion PASSED: all %d guarded mutations still raise on the restored database", len(mutations))
}

// assertProvenanceCheckSurvivedAndIsStillNotValid guards the other half of the
// schema-object question. 4.1c-a deliberately added its provenance CHECK
// NOT VALID so it governs rows written forward without vouching for anything
// backfilled. A restore that validated it would silently convert a
// forward-only guarantee into a retroactive claim about historical rows.
func assertProvenanceCheckSurvivedAndIsStillNotValid(ctx context.Context, t *testing.T, db *sql.DB) {
	t.Helper()
	var convalidated bool
	err := db.QueryRowContext(ctx, `
		SELECT convalidated FROM pg_constraint
		WHERE conname = 'integration_delivery_identity_decisions_provenance_chk'
	`).Scan(&convalidated)
	if err != nil {
		t.Fatalf("the 4.1c-a provenance CHECK did not survive the restore: %v", err)
	}
	if convalidated {
		t.Fatal("the provenance CHECK came back VALIDATED. It was authored NOT VALID so it " +
			"governs rows from 4.1c-a forward without asserting anything about backfilled " +
			"rows; a restore that validates it turns a forward-only guarantee into a " +
			"retroactive one nobody decided to make.")
	}
	t.Log("assertion PASSED: the 4.1c-a provenance CHECK survived the restore and is still NOT VALID")
}

// assertQueuedAttemptResumes is assertion 5 and the one that makes this a
// recovery proof rather than a schema proof: "resumable work" in budget 6 means
// the delivery worker picks up where it left off with no manual repair.
//
// The publisher is a capture rather than Kafka on purpose. Publisher is
// broker-neutral by contract (internal/integration/delivery/types.go:62-65), and
// the claim under test is that the *restored durable state* still yields a
// claimable, publishable work item — not that a broker is reachable. Requiring
// Kafka here would add a service container to prove something about PostgreSQL.
func assertQueuedAttemptResumes(ctx context.Context, t *testing.T, db *sql.DB, fixture durableFixture) {
	t.Helper()

	store, err := delivery.NewPostgresStore(db, func() time.Time { return time.Date(2026, 8, 8, 10, 0, 0, 0, time.UTC) })
	if err != nil {
		t.Fatalf("construct delivery store against the restored database: %v", err)
	}

	item, err := store.Claim(ctx, "worker-restored-1", 30*time.Second)
	if err != nil {
		t.Fatalf("claim from the restored database: %v", err)
	}
	if item == nil {
		t.Fatal("the restored database yielded no claimable work item. The queued attempt and " +
			"its pending outbox row were restored, so a worker must resume them without " +
			"manual repair — that is what budget 6's \"resumable work\" means.")
	}
	if item.AttemptID != fixture.AttemptID {
		t.Fatalf("claimed attempt %q, want %q", item.AttemptID, fixture.AttemptID)
	}
	if item.EventID != fixture.EventID {
		t.Fatalf("claimed work item carries event %q, want %q", item.EventID, fixture.EventID)
	}
	if !strings.Contains(string(item.EventPayload), fixture.PHISentinel) {
		t.Fatalf("claimed work item lost its canonical payload: %s", item.EventPayload)
	}

	if err := store.MarkPublished(ctx, *item); err != nil {
		t.Fatalf("mark the resumed attempt published: %v", err)
	}

	var outboxStatus, attemptStatus string
	if err := db.QueryRowContext(ctx, `
		SELECT o.status, a.status
		FROM integration_delivery_outbox o
		JOIN integration_delivery_attempts a
		  ON a.tenant_id = o.tenant_id AND a.attempt_id = o.attempt_id
		WHERE o.tenant_id = $1 AND o.outbox_id = $2
	`, compatTenantID, fixture.OutboxID).Scan(&outboxStatus, &attemptStatus); err != nil {
		t.Fatalf("read resumed delivery state: %v", err)
	}
	if outboxStatus != "published" || attemptStatus != "succeeded" {
		t.Fatalf("resumed delivery ended at outbox=%q attempt=%q, want published/succeeded",
			outboxStatus, attemptStatus)
	}

	// A second claim must find nothing: resuming restored work must not
	// manufacture a duplicate delivery.
	again, err := store.Claim(ctx, "worker-restored-1", 30*time.Second)
	if err != nil {
		t.Fatalf("second claim against the restored database: %v", err)
	}
	if again != nil {
		t.Fatalf("the restored database yielded a second claimable item (%s); resuming must not "+
			"duplicate work", again.AttemptID)
	}
	t.Log("assertion PASSED: the queued attempt was claimed, published, and not re-claimed from the restored state")
}
