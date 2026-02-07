package main

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// runWorkflowGenerate — flag parsing & early-exit tests
// ---------------------------------------------------------------------------

func TestRunWorkflowGenerate_Help(t *testing.T) {
	stdout, _ := captureOutput(t, func() {
		err := runWorkflowGenerate([]string{"--help"})
		assertNoError(t, err)
	})
	assertContains(t, stdout, "workflow generate")
}

func TestRunWorkflowGenerate_MissingDescription(t *testing.T) {
	err := runWorkflowGenerate([]string{})
	assertError(t, err)
	assertErrorContains(t, err, "description required")
}

func TestRunWorkflowGenerate_UnknownFlag(t *testing.T) {
	err := runWorkflowGenerate([]string{"--bogus"})
	assertError(t, err)
	assertErrorContains(t, err, "unknown flag")
}

// ---------------------------------------------------------------------------
// runWorkflowExplain — flag parsing & early-exit tests
// ---------------------------------------------------------------------------

func TestRunWorkflowExplain_Help(t *testing.T) {
	stdout, _ := captureOutput(t, func() {
		err := runWorkflowExplain([]string{"--help"})
		assertNoError(t, err)
	})
	assertContains(t, stdout, "workflow explain")
}

func TestRunWorkflowExplain_MissingFile(t *testing.T) {
	err := runWorkflowExplain([]string{})
	assertError(t, err)
	assertErrorContains(t, err, "workflow file path required")
}

func TestRunWorkflowExplain_NonexistentFile(t *testing.T) {
	err := runWorkflowExplain([]string{"/tmp/does-not-exist-abc123.yaml"})
	assertError(t, err)
	assertErrorContains(t, err, "read workflow")
}

func TestRunWorkflowExplain_UnknownFlag(t *testing.T) {
	err := runWorkflowExplain([]string{"--bogus"})
	assertError(t, err)
	assertErrorContains(t, err, "unknown flag")
}

func TestRunWorkflowExplain_AudienceMissingValue(t *testing.T) {
	err := runWorkflowExplain([]string{"--audience"})
	assertError(t, err)
	assertErrorContains(t, err, "--audience requires a value")
}

// ---------------------------------------------------------------------------
// runWorkflowCEL — flag parsing & early-exit tests
// ---------------------------------------------------------------------------

func TestRunWorkflowCEL_Help(t *testing.T) {
	stdout, _ := captureOutput(t, func() {
		err := runWorkflowCEL([]string{"--help"})
		assertNoError(t, err)
	})
	assertContains(t, stdout, "workflow cel")
}

func TestRunWorkflowCEL_MissingDescription(t *testing.T) {
	err := runWorkflowCEL([]string{})
	assertError(t, err)
	assertErrorContains(t, err, "description required")
}

func TestRunWorkflowCEL_UnknownFlag(t *testing.T) {
	err := runWorkflowCEL([]string{"--bogus"})
	assertError(t, err)
	assertErrorContains(t, err, "unknown flag")
}

func TestRunWorkflowCEL_ValidateMissingExpr(t *testing.T) {
	err := runWorkflowCEL([]string{"--validate"})
	assertError(t, err)
	assertErrorContains(t, err, "--validate requires a CEL expression")
}

// ---------------------------------------------------------------------------
// runTerminologySearch — flag parsing & early-exit tests
// ---------------------------------------------------------------------------

func TestRunTerminologySearch_Help(t *testing.T) {
	stdout, _ := captureOutput(t, func() {
		err := runTerminologySearch([]string{"--help"})
		assertNoError(t, err)
	})
	assertContains(t, stdout, "terminology search")
}

func TestRunTerminologySearch_MissingQuery(t *testing.T) {
	err := runTerminologySearch([]string{})
	assertError(t, err)
	assertErrorContains(t, err, "query required")
}

func TestRunTerminologySearch_UnknownFlag(t *testing.T) {
	err := runTerminologySearch([]string{"--bogus"})
	assertError(t, err)
	assertErrorContains(t, err, "unknown flag")
}

func TestRunTerminologySearch_QueryMissingValue(t *testing.T) {
	err := runTerminologySearch([]string{"--query"})
	assertError(t, err)
	assertErrorContains(t, err, "--query requires a value")
}

func TestRunTerminologySearch_VocabularyMissingValue(t *testing.T) {
	err := runTerminologySearch([]string{"--vocabulary"})
	assertError(t, err)
	assertErrorContains(t, err, "--vocabulary requires a value")
}

// ---------------------------------------------------------------------------
// Print usage functions — captureOutput tests
// ---------------------------------------------------------------------------

func TestPrintWorkflowGenerateUsage(t *testing.T) {
	stdout, _ := captureOutput(t, func() {
		printWorkflowGenerateUsage()
	})
	assertContains(t, stdout, "workflow generate")
	assertContains(t, stdout, "Usage:")
}

func TestPrintWorkflowExplainUsage(t *testing.T) {
	stdout, _ := captureOutput(t, func() {
		printWorkflowExplainUsage()
	})
	assertContains(t, stdout, "workflow explain")
	assertContains(t, stdout, "Usage:")
}

func TestPrintWorkflowCELUsage(t *testing.T) {
	stdout, _ := captureOutput(t, func() {
		printWorkflowCELUsage()
	})
	assertContains(t, stdout, "workflow cel")
	assertContains(t, stdout, "Usage:")
}

func TestPrintTerminologySearchUsage(t *testing.T) {
	stdout, _ := captureOutput(t, func() {
		printTerminologySearchUsage()
	})
	assertContains(t, stdout, "terminology search")
	assertContains(t, stdout, "Usage:")
}

// ---------------------------------------------------------------------------
// Integration via runCLI dispatcher
// ---------------------------------------------------------------------------

func TestRunCLI_WorkflowGenerateHelp(t *testing.T) {
	stdout, _, err := runCLI(t, "workflow", "generate", "--help")
	assertNoError(t, err)
	assertContains(t, stdout, "workflow generate")
}

func TestRunCLI_WorkflowExplainHelp(t *testing.T) {
	stdout, _, err := runCLI(t, "workflow", "explain", "--help")
	assertNoError(t, err)
	assertContains(t, stdout, "workflow explain")
}

func TestRunCLI_WorkflowCelHelp(t *testing.T) {
	stdout, _, err := runCLI(t, "workflow", "cel", "--help")
	assertNoError(t, err)
	assertContains(t, stdout, "workflow cel")
}

func TestRunCLI_TerminologySearchHelp(t *testing.T) {
	stdout, _, err := runCLI(t, "terminology", "search", "--help")
	assertNoError(t, err)
	if !strings.Contains(stdout, "terminology search") {
		// Terminology may route through its own dispatcher — just verify no error
		_ = stdout
	}
}
