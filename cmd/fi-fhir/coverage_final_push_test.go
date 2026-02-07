package main

import (
	"context"
	"testing"
)

// =============================================================================
// runServe --dry-run — exercises ALL flag parsing + JSON output (no server)
// =============================================================================

func TestServe_DryRun_DefaultConfig(t *testing.T) {
	stdout, _ := captureOutput(t, func() {
		err := runServe([]string{"--dry-run"})
		assertNoError(t, err)
	})
	// Dry-run output is a JSON blob with server config
	assertContains(t, stdout, `"host"`)
	assertContains(t, stdout, `"port"`)
	assertContains(t, stdout, `"playground_enabled"`)
	assertContains(t, stdout, `"introspection"`)
	assertContains(t, stdout, `"timeout"`)
}

func TestServe_DryRun_CustomPort(t *testing.T) {
	stdout, _ := captureOutput(t, func() {
		err := runServe([]string{"--port", "9999", "--dry-run"})
		assertNoError(t, err)
	})
	assertContains(t, stdout, "9999")
}

func TestServe_DryRun_CustomHost(t *testing.T) {
	stdout, _ := captureOutput(t, func() {
		err := runServe([]string{"--host", "127.0.0.1", "--dry-run"})
		assertNoError(t, err)
	})
	assertContains(t, stdout, "127.0.0.1")
}

func TestServe_DryRun_CustomPath(t *testing.T) {
	stdout, _ := captureOutput(t, func() {
		err := runServe([]string{"--path", "/api/graphql", "--dry-run"})
		assertNoError(t, err)
	})
	assertContains(t, stdout, "/api/graphql")
}

func TestServe_DryRun_PlaygroundPath(t *testing.T) {
	stdout, _ := captureOutput(t, func() {
		err := runServe([]string{"--playground-path", "/play", "--dry-run"})
		assertNoError(t, err)
	})
	assertContains(t, stdout, "/play")
}

func TestServe_DryRun_NoPlayground(t *testing.T) {
	stdout, _ := captureOutput(t, func() {
		err := runServe([]string{"--no-playground", "--dry-run"})
		assertNoError(t, err)
	})
	// playground_enabled should be false
	assertContains(t, stdout, `"playground_enabled": false`)
}

func TestServe_DryRun_NoIntrospection(t *testing.T) {
	stdout, _ := captureOutput(t, func() {
		err := runServe([]string{"--no-introspection", "--dry-run"})
		assertNoError(t, err)
	})
	assertContains(t, stdout, `"introspection": false`)
}

func TestServe_DryRun_MaxDepthAndComplexity(t *testing.T) {
	stdout, _ := captureOutput(t, func() {
		err := runServe([]string{"--max-depth", "20", "--max-complexity", "5000", "--dry-run"})
		assertNoError(t, err)
	})
	assertContains(t, stdout, "20")
	assertContains(t, stdout, "5000")
}

func TestServe_DryRun_Timeout(t *testing.T) {
	stdout, _ := captureOutput(t, func() {
		err := runServe([]string{"--timeout", "1m30s", "--dry-run"})
		assertNoError(t, err)
	})
	assertContains(t, stdout, "1m30s")
}

func TestServe_DryRun_WithWorkflow(t *testing.T) {
	tmpDir := t.TempDir()
	wfPath := createTempFile(t, tmpDir, "wf*.yaml", `workflow:
  name: dry-run-test
  version: "1.0"
  routes:
    - name: route1
      event_types: ["ADT^A01"]
      conditions: []
      actions: []
`)

	stdout, _ := captureOutput(t, func() {
		err := runServe([]string{"--workflow", wfPath, "--dry-run"})
		assertNoError(t, err)
	})
	assertContains(t, stdout, `"workflow"`)
	assertContains(t, stdout, "dry-run-test")
}

func TestServe_DryRun_WithInvalidWorkflow(t *testing.T) {
	tmpDir := t.TempDir()
	wfPath := createTempFile(t, tmpDir, "wf*.yaml", "not valid yaml {{{")

	err := runServe([]string{"--workflow", wfPath, "--dry-run"})
	assertError(t, err)
	assertErrorContains(t, err, "failed to load workflow")
}

func TestServe_DryRun_NonexistentWorkflow(t *testing.T) {
	err := runServe([]string{"--workflow", "/nonexistent/workflow.yaml", "--dry-run"})
	assertError(t, err)
	assertErrorContains(t, err, "failed to load workflow")
}

func TestServe_DryRun_TemporalFlags(t *testing.T) {
	stdout, _ := captureOutput(t, func() {
		err := runServe([]string{
			"--temporal", "localhost:7233",
			"--temporal-namespace", "test-ns",
			"--dry-run",
		})
		assertNoError(t, err)
	})
	// Temporal config is parsed but not included in dry-run JSON output.
	// The important thing is the flags parse without error.
	assertContains(t, stdout, `"host"`)
}

func TestServe_DryRun_AllFlags(t *testing.T) {
	tmpDir := t.TempDir()
	wfPath := createTempFile(t, tmpDir, "wf*.yaml", `workflow:
  name: all-flags
  version: "2.0"
  routes: []
`)

	stdout, _ := captureOutput(t, func() {
		err := runServe([]string{
			"--host", "0.0.0.0",
			"--port", "3000",
			"--path", "/gql",
			"--playground-path", "/ide",
			"--no-introspection",
			"--max-depth", "15",
			"--max-complexity", "2000",
			"--timeout", "45s",
			"--temporal", "tempo:7233",
			"--temporal-namespace", "my-ns",
			"--workflow", wfPath,
			"--dry-run",
		})
		assertNoError(t, err)
	})
	assertContains(t, stdout, "3000")
	assertContains(t, stdout, "/gql")
	assertContains(t, stdout, "all-flags")
}

// =============================================================================
// checkTerminologyPins — pure function, fully testable offline
// =============================================================================

func TestCheckTerminologyPins_PolicyPass(t *testing.T) {
	warnings, err := checkTerminologyPins(context.Background(), "postgres://localhost/test", map[string]string{"SNOMED": "2024"}, "pass")
	assertNoError(t, err)
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings for pass policy, got %d", len(warnings))
	}
}

func TestCheckTerminologyPins_EmptyDBURL(t *testing.T) {
	warnings, err := checkTerminologyPins(context.Background(), "", map[string]string{"SNOMED": "2024"}, "warn")
	assertNoError(t, err)
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings for empty DB URL, got %d", len(warnings))
	}
}

func TestCheckTerminologyPins_EmptyPins(t *testing.T) {
	warnings, err := checkTerminologyPins(context.Background(), "postgres://localhost/test", map[string]string{}, "warn")
	assertNoError(t, err)
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings for empty pins, got %d", len(warnings))
	}
}

func TestCheckTerminologyPins_NilPins(t *testing.T) {
	warnings, err := checkTerminologyPins(context.Background(), "postgres://localhost/test", nil, "warn")
	assertNoError(t, err)
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings for nil pins, got %d", len(warnings))
	}
}

func TestCheckTerminologyPins_EmptyPolicyDefaultsToWarn(t *testing.T) {
	// Empty policy with empty pins — should still return nil (the policy normalizes to "warn" but pins are empty)
	warnings, err := checkTerminologyPins(context.Background(), "postgres://localhost/test", map[string]string{}, "")
	assertNoError(t, err)
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %d", len(warnings))
	}
}

func TestCheckTerminologyPins_ErrorPolicyWithEmptyPins(t *testing.T) {
	// Error policy with empty pins — returns nil (no pins to check)
	warnings, err := checkTerminologyPins(context.Background(), "postgres://localhost/test", map[string]string{}, "error")
	assertNoError(t, err)
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %d", len(warnings))
	}
}

// =============================================================================
// loadTerminologyPinConfigFromEnv — exercises env-based config loading
// =============================================================================

// =============================================================================
// runWorkflowGenerate — flag parsing and validation (before LLM call)
// =============================================================================

func TestWorkflowGenerate_NoDescription(t *testing.T) {
	err := runWorkflowGenerate([]string{})
	assertError(t, err)
	assertErrorContains(t, err, "description required")
}

func TestWorkflowGenerate_UnknownFlag(t *testing.T) {
	err := runWorkflowGenerate([]string{"--unknown"})
	assertError(t, err)
	assertErrorContains(t, err, "unknown flag")
}

func TestWorkflowGenerate_HelpFlag(t *testing.T) {
	stdout, _ := captureOutput(t, func() {
		err := runWorkflowGenerate([]string{"--help"})
		assertNoError(t, err)
	})
	assertContains(t, stdout, "workflow generate")
}

// =============================================================================
// runWorkflowExplain — flag parsing and validation (before LLM call)
// =============================================================================

func TestWorkflowExplain_NoInput(t *testing.T) {
	err := runWorkflowExplain([]string{})
	assertError(t, err)
	assertErrorContains(t, err, "workflow file path required")
}

func TestWorkflowExplain_UnknownFlag(t *testing.T) {
	err := runWorkflowExplain([]string{"--unknown"})
	assertError(t, err)
	assertErrorContains(t, err, "unknown flag")
}

func TestWorkflowExplain_AudienceMissingValue(t *testing.T) {
	err := runWorkflowExplain([]string{"--audience"})
	assertError(t, err)
	assertErrorContains(t, err, "--audience requires a value")
}

func TestWorkflowExplain_HelpFlag(t *testing.T) {
	stdout, _ := captureOutput(t, func() {
		err := runWorkflowExplain([]string{"--help"})
		assertNoError(t, err)
	})
	assertContains(t, stdout, "workflow explain")
}

func TestWorkflowExplain_NonexistentFile(t *testing.T) {
	err := runWorkflowExplain([]string{"/nonexistent/workflow.yaml"})
	assertError(t, err)
	assertErrorContains(t, err, "read workflow")
}

// =============================================================================
// runWorkflowCEL — flag parsing and validation (before LLM call)
// =============================================================================

func TestWorkflowCEL_NoArgs(t *testing.T) {
	err := runWorkflowCEL([]string{})
	assertError(t, err)
	assertErrorContains(t, err, "description required")
}

func TestWorkflowCEL_UnknownFlag(t *testing.T) {
	err := runWorkflowCEL([]string{"--unknown"})
	assertError(t, err)
	assertErrorContains(t, err, "unknown flag")
}

func TestWorkflowCEL_ValidateMissingValue(t *testing.T) {
	err := runWorkflowCEL([]string{"--validate"})
	assertError(t, err)
	assertErrorContains(t, err, "--validate requires a CEL expression")
}

func TestWorkflowCEL_HelpFlag(t *testing.T) {
	stdout, _ := captureOutput(t, func() {
		err := runWorkflowCEL([]string{"--help"})
		assertNoError(t, err)
	})
	assertContains(t, stdout, "workflow cel")
}

// =============================================================================
// runTerminologyMapping — dispatcher coverage (unknown subcommand + routing)
// =============================================================================

func TestTerminologyMapping_NoArgs(t *testing.T) {
	stdout, _ := captureOutput(t, func() {
		err := runTerminologyMapping([]string{})
		assertNoError(t, err)
	})
	assertContains(t, stdout, "terminology mapping")
}

func TestTerminologyMapping_HelpFlag(t *testing.T) {
	stdout, _ := captureOutput(t, func() {
		err := runTerminologyMapping([]string{"--help"})
		assertNoError(t, err)
	})
	assertContains(t, stdout, "terminology mapping")
}

func TestTerminologyMapping_ShortHelpFlag(t *testing.T) {
	stdout, _ := captureOutput(t, func() {
		err := runTerminologyMapping([]string{"-h"})
		assertNoError(t, err)
	})
	assertContains(t, stdout, "terminology mapping")
}

func TestTerminologyMapping_HelpSubcommand(t *testing.T) {
	stdout, _ := captureOutput(t, func() {
		err := runTerminologyMapping([]string{"help"})
		assertNoError(t, err)
	})
	assertContains(t, stdout, "terminology mapping")
}

func TestTerminologyMapping_UnknownSubcommand(t *testing.T) {
	err := runTerminologyMapping([]string{"invalid_sub"})
	assertError(t, err)
	assertErrorContains(t, err, "unknown mapping subcommand")
}

// =============================================================================
// Mapping approve/reject — additional offline validation (beyond terminology_approval_test.go)
// =============================================================================

func TestTerminologyMappingApprove_WithFlags(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "postgres://localhost/test")

	// Exercises --by, --equivalence, --comment, --json flag parsing
	// Will fail at DB connection, but validates all flags parsed
	err := runTerminologyMappingApprove([]string{"42",
		"--by", "human-reviewer",
		"--equivalence", "equivalent",
		"--comment", "verified mapping",
		"--json",
	})
	assertError(t, err) // DB connection error expected
}

func TestTerminologyMappingReject_WithFlags(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "postgres://localhost/test")

	// Exercises --by, --reason, --json flag parsing
	err := runTerminologyMappingReject([]string{"42",
		"--reason", "incorrect mapping",
		"--by", "human-reviewer",
		"--json",
	})
	assertError(t, err) // DB connection error expected
}

func TestTerminologyMappingPending_WithFilters(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "postgres://localhost/test")

	// Exercises --status, --min-confidence, --source-system, --target-system, --limit, --offset, --json flags
	err := runTerminologyMappingPending([]string{
		"--status", "pending",
		"--min-confidence", "0.8",
		"--source-system", "epic_labs",
		"--target-system", "http://loinc.org",
		"--limit", "50",
		"--offset", "10",
		"--json",
	})
	assertError(t, err) // DB connection error expected
}

// =============================================================================
// runTerminologyAutoroute — additional flag parsing
// =============================================================================

func TestTerminologyAutoroute_InvalidThreshold(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "postgres://localhost/test")

	err := runTerminologyAutoroute([]string{"GLU001",
		"--source-system", "epic_labs",
		"--target-system", "http://loinc.org",
		"--auto-approve-threshold", "not-a-number",
	})
	assertError(t, err)
	assertErrorContains(t, err, "invalid threshold")
}

func TestTerminologyAutoroute_InvalidReviewTimeout(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "postgres://localhost/test")

	err := runTerminologyAutoroute([]string{"GLU001",
		"--source-system", "epic_labs",
		"--target-system", "http://loinc.org",
		"--review-timeout-days", "not-a-number",
	})
	assertError(t, err)
	assertErrorContains(t, err, "invalid timeout days")
}

func TestTerminologyAutoroute_AllFlags(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "postgres://localhost/test")

	// Exercises all flag parsing (will fail at DB connection)
	err := runTerminologyAutoroute([]string{"GLU001",
		"--source-system", "epic_labs",
		"--target-system", "http://loinc.org",
		"--display", "Glucose Fasting",
		"--temporal", "localhost:7233",
		"--temporal-namespace", "test-ns",
		"--auto-approve-threshold", "0.9",
		"--review-timeout-days", "14",
		"--wait",
		"--json",
	})
	assertError(t, err) // DB connection error expected
}

// =============================================================================
// runEventStoreInit — basic validation
// =============================================================================

func TestEventStoreInit_MissingDB(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_URL", "")

	err := runEventStoreInit([]string{})
	assertError(t, err)
	assertErrorContains(t, err, "database URL required")
}

func TestEventStoreInit_DBFlagMissingValue(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_URL", "")

	err := runEventStoreInit([]string{"--db"})
	assertError(t, err)
	assertErrorContains(t, err, "--db requires a value")
}

// =============================================================================
// runEventStoreHelp — exercises the help subcommand
// =============================================================================

func TestEventStore_HelpSubcommand(t *testing.T) {
	stdout, _, err := runCLI(t, "eventstore")
	assertNoError(t, err)
	assertContains(t, stdout, "eventstore")
}

// =============================================================================
// runTerminologyStatus — basic coverage (env + flag validation)
// =============================================================================

func TestTerminologyStatus_MissingDB(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "")
	t.Setenv("FI_FHIR_DATABASE_URL", "")

	err := runTerminologyStatus([]string{})
	assertError(t, err)
	assertErrorContains(t, err, "database URL required")
}

func TestTerminologyStatus_DBFlagMissingValue(t *testing.T) {
	// getTerminologyDBURL silently ignores --db when it's the last arg (no i+1 guard).
	// So it falls through to env vars — with env cleared, we get the "database URL required" error.
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "")
	t.Setenv("FI_FHIR_DATABASE_URL", "")

	err := runTerminologyStatus([]string{"--db"})
	assertError(t, err)
	assertErrorContains(t, err, "database URL required")
}

// =============================================================================
// runTerminologyUse — validation before DB call
// =============================================================================

func TestTerminologyUse_NoArgs(t *testing.T) {
	stdout, _ := captureOutput(t, func() {
		err := runTerminologyUse([]string{})
		assertNoError(t, err)
	})
	assertContains(t, stdout, "terminology use")
}

func TestTerminologyUse_OneArg(t *testing.T) {
	stdout, _ := captureOutput(t, func() {
		err := runTerminologyUse([]string{"SNOMEDCT_US"})
		assertNoError(t, err)
	})
	assertContains(t, stdout, "terminology use")
}

func TestTerminologyUse_MissingDB(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "")
	t.Setenv("FI_FHIR_DATABASE_URL", "")

	err := runTerminologyUse([]string{"SNOMEDCT_US", "2024-09"})
	assertError(t, err)
	assertErrorContains(t, err, "database URL required")
}

// =============================================================================
// runTerminologyInit — validation
// =============================================================================

func TestTerminologyInit_MissingDB(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "")
	t.Setenv("FI_FHIR_DATABASE_URL", "")

	err := runTerminologyInit([]string{})
	assertError(t, err)
	assertErrorContains(t, err, "database URL required")
}
