package main

import "testing"

// =============================================================================
// runTerminologySearch — deeper flag parsing tests
// =============================================================================

func TestRunTerminologySearch_LimitMissingValue(t *testing.T) {
	err := runTerminologySearch([]string{"--limit"})
	assertError(t, err)
	assertErrorContains(t, err, "--limit requires a value")
}

func TestRunTerminologySearch_MinScoreMissingValue(t *testing.T) {
	err := runTerminologySearch([]string{"--min-score"})
	assertError(t, err)
	assertErrorContains(t, err, "--min-score requires a value")
}

func TestRunTerminologySearch_QdrantURLMissingValue(t *testing.T) {
	err := runTerminologySearch([]string{"--qdrant-url"})
	assertError(t, err)
	assertErrorContains(t, err, "--qdrant-url requires a value")
}

func TestRunTerminologySearch_EmbeddingURLMissingValue(t *testing.T) {
	err := runTerminologySearch([]string{"--embedding-url"})
	assertError(t, err)
	assertErrorContains(t, err, "--embedding-url requires a value")
}

func TestRunTerminologySearch_EmbeddingModelMissingValue(t *testing.T) {
	err := runTerminologySearch([]string{"--embedding-model"})
	assertError(t, err)
	assertErrorContains(t, err, "--embedding-model requires a value")
}

func TestRunTerminologySearch_ShortQueryFlag(t *testing.T) {
	err := runTerminologySearch([]string{"-q"})
	assertError(t, err)
	assertErrorContains(t, err, "--query requires a value")
}

func TestRunTerminologySearch_ShortVocabularyFlag(t *testing.T) {
	err := runTerminologySearch([]string{"-v"})
	assertError(t, err)
	assertErrorContains(t, err, "--vocabulary requires a value")
}

func TestRunTerminologySearch_ShortLimitFlag(t *testing.T) {
	err := runTerminologySearch([]string{"-l"})
	assertError(t, err)
	assertErrorContains(t, err, "--limit requires a value")
}

func TestRunTerminologySearch_ShortHelpFlag(t *testing.T) {
	stdout, _ := captureOutput(t, func() {
		err := runTerminologySearch([]string{"-h"})
		assertNoError(t, err)
	})
	assertContains(t, stdout, "terminology search")
}

// =============================================================================
// runWorkflowGenerate — short flag aliases
// =============================================================================

func TestRunWorkflowGenerate_ShortHelpFlag(t *testing.T) {
	stdout, _ := captureOutput(t, func() {
		err := runWorkflowGenerate([]string{"-h"})
		assertNoError(t, err)
	})
	assertContains(t, stdout, "workflow generate")
}

// =============================================================================
// runWorkflowExplain — short flag aliases
// =============================================================================

func TestRunWorkflowExplain_ShortHelpFlag(t *testing.T) {
	stdout, _ := captureOutput(t, func() {
		err := runWorkflowExplain([]string{"-h"})
		assertNoError(t, err)
	})
	assertContains(t, stdout, "workflow explain")
}

// =============================================================================
// runWorkflowCEL — short flag aliases
// =============================================================================

func TestRunWorkflowCEL_ShortHelpFlag(t *testing.T) {
	stdout, _ := captureOutput(t, func() {
		err := runWorkflowCEL([]string{"-h"})
		assertNoError(t, err)
	})
	assertContains(t, stdout, "workflow cel")
}
