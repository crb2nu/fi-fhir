//go:build integration

package migrationcompat

import (
	"encoding/json"
	"os"
	"sort"
	"testing"
	"time"
)

// recoveryTimeBudget is the product spec's RTO for a database failure:
// 30 minutes (.loom/20-product-spec-integration-engine-ide-completion.md:277-278).
// Slice 4.4c's WAL/PITR posture decision (.loom/40-decisions.md, 2026-08-09)
// certifies this half of budget 5 against the documented method and hands the
// RPO half to the operator, because no method this repository ships can bound
// data loss to five minutes.
const recoveryTimeBudget = 30 * time.Minute

// recoveryReportSchema versions the archived artifact. A consumer that cannot
// read this string must not guess at the shape.
const recoveryReportSchema = "fi-fhir.recovery.rto.v1"

// recoveryReport is what the round-trip archives. It carries the row counts it
// was measured against for the same reason
// deploy/helm/fi-fhir/values-reference-profile.yaml says it is not a capacity
// claim: a recovery time measured on a fixture is evidence about the
// *procedure*, not about a production data volume, and a number published
// without its denominator invites the opposite reading.
type recoveryReport struct {
	Schema string `json:"schema"`
	// Method is the artifact whose runtime was measured, so the number cannot
	// drift away from the procedure the runbook documents.
	Method        string         `json:"method"`
	MeasuredAt    time.Time      `json:"measured_at"`
	RestoreMillis int64          `json:"restore_millis"`
	ResumeMillis  int64          `json:"resume_millis"`
	BudgetMillis  int64          `json:"budget_millis"`
	Rows          map[string]int `json:"rows"`
	TotalRows     int            `json:"total_rows"`
	Note          string         `json:"note"`
}

// reportRecoveryTime is slice 4.4c task 7. It closes the RTO half of budget 5
// against the documented method and archives the number.
//
// Two spans are recorded because they answer different questions. RestoreMillis
// is scripts/pgdump-roundtrip.sh alone — dump, recreate, restore — which is what
// an operator following the runbook waits for. ResumeMillis runs from the same
// start to the point where the delivery worker has claimed and published a
// restored attempt, which is what "recovered" means for this product; it is
// measured *through* the round-trip's faithfulness assertions, so it is an
// upper bound on the procedure rather than a floor.
//
// What this is not: a claim about production recovery time. The fixture is a
// few dozen rows. It bounds the procedure's fixed cost and it catches a
// catastrophic regression — a restore that starts taking minutes per table, or
// a resume path that waits on a lease — and the row counts travel with the
// number so nobody reads it as a capacity statement.
func reportRecoveryTime(t *testing.T, start time.Time, restoreElapsed time.Duration, rows map[string]int) {
	t.Helper()
	resumeElapsed := time.Since(start)

	total := 0
	tables := make([]string, 0, len(rows))
	for table, count := range rows {
		total += count
		tables = append(tables, table)
	}
	sort.Strings(tables)

	if resumeElapsed > recoveryTimeBudget {
		t.Fatalf("recovery took %s against a %s RTO budget, on a fixture of %d rows across %d "+
			"durable classes.\n"+
			"  This is the documented method (scripts/pgdump-roundtrip.sh) timed end to end, "+
			"dump through the first successful delivery Claim. Exceeding the budget at fixture "+
			"scale means the procedure's fixed cost alone consumes it.",
			resumeElapsed.Round(time.Millisecond), recoveryTimeBudget, total, len(rows))
	}

	report := recoveryReport{
		Schema:        recoveryReportSchema,
		Method:        "scripts/pgdump-roundtrip.sh",
		MeasuredAt:    time.Now().UTC(),
		RestoreMillis: restoreElapsed.Milliseconds(),
		ResumeMillis:  resumeElapsed.Milliseconds(),
		BudgetMillis:  recoveryTimeBudget.Milliseconds(),
		Rows:          rows,
		TotalRows:     total,
		Note: "Measured on a CI fixture, not a production data volume. RestoreMillis is the " +
			"documented dump/restore procedure; ResumeMillis runs to the first successful " +
			"delivery Claim and is measured through the round-trip's faithfulness assertions, " +
			"so it is an upper bound. This certifies the RTO half of budget 5 only; the RPO " +
			"half is an operator responsibility (.loom/40-decisions.md, 2026-08-09 WAL/PITR " +
			"posture).",
	}

	t.Logf("RECOVERY TIME: restore %s, resume-to-first-claim %s, budget %s, %d rows across %d "+
		"durable classes",
		restoreElapsed.Round(time.Millisecond), resumeElapsed.Round(time.Millisecond),
		recoveryTimeBudget, total, len(rows))

	// The artifact is written only when a caller asks for one, so the proof
	// stays runnable on a workstation without leaving files behind. The CI job
	// sets the variable and archives the path.
	path := os.Getenv("FI_FHIR_RECOVERY_REPORT")
	if path == "" {
		return
	}
	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatalf("encode recovery report: %v", err)
	}
	if err := os.WriteFile(path, append(encoded, '\n'), 0o644); err != nil {
		t.Fatalf("write recovery report to %s: %v", path, err)
	}
	t.Logf("recovery report archived to %s", path)
}
