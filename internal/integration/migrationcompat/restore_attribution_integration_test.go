//go:build integration

package migrationcompat

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/lib/pq"
)

// raiseException is PostgreSQL's SQLSTATE for `RAISE EXCEPTION` in a PL/pgSQL
// trigger body — the code every immutability guard in this repository produces
// (internal/integration/processor/migrations/0004_audit_immutability.sql:22-27).
const raiseException = pq.ErrorCode("P0001")

// foreignKeyViolation is the SQLSTATE a referential-integrity refusal produces.
// A guarded mutation that comes back with this code was refused by a foreign
// key, not by the guard the assertion claims to be watching.
const foreignKeyViolation = pq.ErrorCode("23503")

// guardedMutation is one mutation that slice 4.1d C1's immutability guards are
// supposed to refuse on a restored database.
type guardedMutation struct {
	name  string
	query string
	args  []any
}

// guardedMutations is the exact list assertImmutabilityGuardsSurvived runs
// (compatibility_integration_test.go:278-301), lifted into a named function so
// the proof and this control cannot drift apart. Slice 4.4c's repair rewires
// the proof onto this function; day 1 only reads it.
func guardedMutations(fixture durableFixture) []guardedMutation {
	return []guardedMutation{
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
}

// TestChaosRecovery_RestoreProofAssertionsAreTriggerAttributed is Lane S5-B's
// day-1 gate (.loom/33-sprint5-execution-specs.md, "Lane S5-B — Kill-Test",
// found defect D3). Like every day-1 gate in this repository it exists to
// reproduce a found defect BEFORE the fix, which is what distinguishes it from
// a green test written afterwards.
//
// The claim under test is the one slice 4.4a's restore proof makes about
// itself, in its own doc comment
// (compatibility_integration_test.go:270-274): "A dump/restore that recreated
// the tables without their triggers would leave a database that looks complete
// and silently permits every mutation C1 forbids."
//
// That claim is only true if each of the six mutations is refused BY ITS
// TRIGGER. The proof asserts `err != nil` and nothing more, so a mutation that
// a foreign key would refuse anyway is indistinguishable from one the guard
// refuses — and the FK shadowing is not hypothetical. It is spelled out in
// internal/integration/processor/migrations/0005_retention_expiry.sql:9-12.
//
// Two halves, run against the restored database produced by the documented
// runbook (scripts/pgdump-roundtrip.sh):
//
//	A. With every trigger intact, each mutation must be refused with SQLSTATE
//	   P0001. A refusal carrying any other code is not the guard speaking.
//	B. With every non-internal trigger dropped, each mutation must SUCCEED.
//	   This is the negative control slice 4.4a never wrote: a mutation that is
//	   still refused once its guard is gone would keep the proof green with the
//	   guard gone, which is precisely the regression the proof exists to catch.
//
// On unmodified `main` this test MUST FAIL in half B, with three of the six
// mutations — delete a canonical event, delete a receipt, delete a delivery
// attempt — still refused, each with SQLSTATE 23503. That failure is the
// finding. Any other outcome means .loom/33 defect D3 is wrong and Lane S5-B
// corrects .loom/33 before writing any production code.
//
// After slice 4.4c's task 2 lands, the guarded mutations target rows with no
// dependents, both halves pass, and this test becomes the permanent negative
// control for the restore round-trip.
func TestChaosRecovery_RestoreProofAssertionsAreTriggerAttributed(t *testing.T) {
	ctx := t.Context()
	dsn := requireCompatDSN(t)

	sourceDSN, _ := newCompatDatabase(t, dsn, "migcompat_attrib_src")
	_, targetName := newCompatDatabase(t, dsn, "migcompat_attrib_dst")
	targetDSN := splitDSN(dsn, targetName)

	source := openCompatDB(t, sourceDSN)
	if err := migrateEveryLedger(ctx, source, compatTenantID); err != nil {
		t.Fatalf("migrate source database: %v", err)
	}
	fixture := seedDurableFixture(ctx, t, source)
	if err := source.Close(); err != nil {
		t.Fatalf("close source pool before dump: %v", err)
	}
	runRoundTripScript(t, sourceDSN, targetName)

	restored := openCompatDB(t, targetDSN)
	mutations := guardedMutations(fixture)

	// ---- Half A: the refusals must be attributable to the guards. ----------
	var misattributed []string
	for _, mutation := range mutations {
		err := applyInRolledBackTx(ctx, restored, mutation)
		if err == nil {
			t.Fatalf("restored database ALLOWED %q with every trigger in place. The C1 "+
				"immutability guard did not survive pg_dump/restore.", mutation.name)
		}
		code := sqlStateOf(err)
		if code != raiseException {
			misattributed = append(misattributed,
				fmt.Sprintf("    %-32s refused with SQLSTATE %s (want %s): %v",
					mutation.name, describeSQLState(code), raiseException, err))
		}
	}
	if len(misattributed) > 0 {
		t.Fatalf("DAY-1 GATE CONFIRMED (half A) — the restore proof's refusals are not the "+
			"immutability guards speaking:\n%s\n"+
			"  A trigger refusal is SQLSTATE P0001. Anything else means the assertion would "+
			"stay green with the guard dropped.", strings.Join(misattributed, "\n"))
	}

	// ---- Half B: the negative control slice 4.4a never wrote. --------------
	dropped := dropEveryNonInternalTrigger(ctx, t, restored)
	if dropped == 0 {
		t.Fatal("the restored database carries no non-internal triggers at all, so half A " +
			"passed for some reason other than the guards; the restore proof is vacuous")
	}
	t.Logf("dropped %d non-internal triggers on the restored database", dropped)

	var shadowed []string
	for _, mutation := range mutations {
		err := applyInRolledBackTx(ctx, restored, mutation)
		if err == nil {
			continue
		}
		shadowed = append(shadowed,
			fmt.Sprintf("    %-32s still refused with SQLSTATE %s: %v",
				mutation.name, describeSQLState(sqlStateOf(err)), err))
	}
	if len(shadowed) > 0 {
		t.Fatalf("DAY-1 GATE CONFIRMED (half B) — %d of %d immutability assertions in slice "+
			"4.4a's restore round-trip are shadowed and would stay green with their triggers "+
			"dropped:\n%s\n"+
			"  Every non-internal trigger was dropped before these ran, so nothing the C1 "+
			"guards do can be refusing them. .loom/33 defect D3 predicts exactly three, each "+
			"with SQLSTATE 23503 (foreign_key_violation), because the fixture row each one "+
			"targets still has dependents — the referencing tables are declared in "+
			"internal/integration/processor/migrations/0001_atomic_submission.sql:31,50,53,71,74,91.\n"+
			"  The FK shadowing is documented in "+
			"internal/integration/processor/migrations/0005_retention_expiry.sql:9-12, so it "+
			"was known when the proof was written.\n"+
			"  Fix (slice 4.4c task 2): give each guarded mutation a dependency-free target row "+
			"so the guard is the only thing that can refuse it.",
			len(shadowed), len(mutations), strings.Join(shadowed, "\n"))
	}

	t.Logf("restore proof attribution CONFIRMED: all %d guarded mutations raise P0001 with the "+
		"triggers in place and all %d succeed with the triggers dropped", len(mutations), len(mutations))
}

// applyInRolledBackTx runs one mutation inside a transaction that is always
// rolled back.
//
// Half B's mutations are expected to succeed, and a succeeded DELETE would
// remove rows every later mutation depends on — the second half of the list
// would then be measuring a fixture the first half consumed. Rolling back keeps
// each mutation independent, and it costs nothing in half A where every
// statement raises.
func applyInRolledBackTx(ctx context.Context, db *sql.DB, mutation guardedMutation) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction for %q: %w", mutation.name, err)
	}
	defer func() { _ = tx.Rollback() }()
	_, execErr := tx.ExecContext(ctx, mutation.query, mutation.args...)
	return execErr
}

// dropEveryNonInternalTrigger removes every user-defined trigger in the public
// schema and returns how many it dropped.
//
// System-internal triggers — the ones PostgreSQL creates to enforce foreign
// keys and deferrable constraints — are excluded by `NOT tgisinternal`, and
// that exclusion is the whole point: what remains after this runs is a database
// whose referential integrity is intact and whose PHI governance is gone. If a
// guarded mutation is still refused in that state, the assertion protecting it
// was never watching the governance.
func dropEveryNonInternalTrigger(ctx context.Context, t *testing.T, db *sql.DB) int {
	t.Helper()
	rows, err := db.QueryContext(ctx, `
		SELECT c.relname, t.tgname
		FROM pg_trigger t
		JOIN pg_class c ON c.oid = t.tgrelid
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE NOT t.tgisinternal AND n.nspname = 'public'
	`)
	if err != nil {
		t.Fatalf("enumerate triggers on the restored database: %v", err)
	}
	defer func() { _ = rows.Close() }()

	type trigger struct{ table, name string }
	var triggers []trigger
	for rows.Next() {
		var tr trigger
		if err := rows.Scan(&tr.table, &tr.name); err != nil {
			t.Fatalf("scan trigger row: %v", err)
		}
		triggers = append(triggers, tr)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read trigger rows: %v", err)
	}
	sort.Slice(triggers, func(i, j int) bool {
		if triggers[i].table != triggers[j].table {
			return triggers[i].table < triggers[j].table
		}
		return triggers[i].name < triggers[j].name
	})

	for _, tr := range triggers {
		if _, err := db.ExecContext(ctx,
			fmt.Sprintf(`DROP TRIGGER %q ON %q`, tr.name, tr.table),
		); err != nil {
			t.Fatalf("drop trigger %s on %s: %v", tr.name, tr.table, err)
		}
	}
	return len(triggers)
}

// sqlStateOf returns the SQLSTATE a PostgreSQL error carries, or the empty code
// when the error did not come from the server.
func sqlStateOf(err error) pq.ErrorCode {
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		return pqErr.Code
	}
	return ""
}

// describeSQLState renders a SQLSTATE with the name that makes the attribution
// argument readable in a failure message.
func describeSQLState(code pq.ErrorCode) string {
	switch code {
	case "":
		return "(no SQLSTATE — not a server error)"
	case raiseException:
		return "P0001 (raise_exception)"
	case foreignKeyViolation:
		return "23503 (foreign_key_violation)"
	default:
		return string(code) + " (" + code.Name() + ")"
	}
}
