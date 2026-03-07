package main

import (
	"testing"
)

// ---------------------------------------------------------------------------
// runLLMEval — flag parsing & early-exit tests
// ---------------------------------------------------------------------------

func TestRunLLMEval_Help(t *testing.T) {
	stdout, _ := captureOutput(t, func() {
		err := runLLMEval([]string{"--help"})
		assertNoError(t, err)
	})
	assertContains(t, stdout, "llm eval")
}

func TestRunLLMEval_MissingTaskTypeAndInput(t *testing.T) {
	err := runLLMEval([]string{})
	assertError(t, err)
	assertErrorContains(t, err, "either --task-type or --input is required")
}

func TestRunLLMEval_TaskTypeMissingValue(t *testing.T) {
	err := runLLMEval([]string{"--task-type"})
	assertError(t, err)
	assertErrorContains(t, err, "--task-type requires a value")
}

func TestRunLLMEval_ModelMissingValue(t *testing.T) {
	err := runLLMEval([]string{"--model"})
	assertError(t, err)
	assertErrorContains(t, err, "--model requires a value")
}

func TestRunLLMEval_PromptMissingValue(t *testing.T) {
	err := runLLMEval([]string{"--prompt"})
	assertError(t, err)
	assertErrorContains(t, err, "--prompt requires a value")
}

func TestRunLLMEval_InputMissingValue(t *testing.T) {
	err := runLLMEval([]string{"--input"})
	assertError(t, err)
	assertErrorContains(t, err, "--input requires a value")
}

func TestRunLLMEval_UnknownFlag(t *testing.T) {
	err := runLLMEval([]string{"--bogus"})
	assertError(t, err)
	assertErrorContains(t, err, "unknown flag")
}

func TestPrintLLMEvalUsage(t *testing.T) {
	stdout, _ := captureOutput(t, func() {
		printLLMEvalUsage()
	})
	assertContains(t, stdout, "llm eval")
	assertContains(t, stdout, "Usage:")
}

func TestRunCLI_LLMEvalHelp(t *testing.T) {
	stdout, _, err := runCLI(t, "llm", "eval", "--help")
	assertNoError(t, err)
	assertContains(t, stdout, "llm eval")
}
