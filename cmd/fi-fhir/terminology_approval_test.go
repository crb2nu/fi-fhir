package main

import (
	"testing"
)

// ---------------------------------------------------------------------------
// runTerminologyMappingPending — validation tests
// ---------------------------------------------------------------------------

func TestRunTerminologyMappingPending_NoDB(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "")
	t.Setenv("FI_FHIR_DATABASE_URL", "")
	err := runTerminologyMappingPending([]string{})
	assertError(t, err)
	assertErrorContains(t, err, "database URL required")
}

// ---------------------------------------------------------------------------
// runTerminologyMappingApprove — validation tests
// ---------------------------------------------------------------------------

func TestRunTerminologyMappingApprove_NoArgs(t *testing.T) {
	err := runTerminologyMappingApprove([]string{})
	assertError(t, err)
	assertErrorContains(t, err, "pending autoroute ID required")
}

func TestRunTerminologyMappingApprove_InvalidID(t *testing.T) {
	err := runTerminologyMappingApprove([]string{"not-a-number"})
	assertError(t, err)
	assertErrorContains(t, err, "invalid pending ID")
}

func TestRunTerminologyMappingApprove_NoDB(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "")
	t.Setenv("FI_FHIR_DATABASE_URL", "")
	err := runTerminologyMappingApprove([]string{"42"})
	assertError(t, err)
	assertErrorContains(t, err, "database URL required")
}

// ---------------------------------------------------------------------------
// runTerminologyMappingReject — validation tests
// ---------------------------------------------------------------------------

func TestRunTerminologyMappingReject_NoArgs(t *testing.T) {
	err := runTerminologyMappingReject([]string{})
	assertError(t, err)
	assertErrorContains(t, err, "pending autoroute ID required")
}

func TestRunTerminologyMappingReject_InvalidID(t *testing.T) {
	err := runTerminologyMappingReject([]string{"not-a-number"})
	assertError(t, err)
	assertErrorContains(t, err, "invalid pending ID")
}

func TestRunTerminologyMappingReject_MissingReason(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "postgres://localhost/test")
	err := runTerminologyMappingReject([]string{"42", "--db", "postgres://localhost/test"})
	assertError(t, err)
	assertErrorContains(t, err, "--reason is required")
}

func TestRunTerminologyMappingReject_NoDB(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "")
	t.Setenv("FI_FHIR_DATABASE_URL", "")
	err := runTerminologyMappingReject([]string{"42", "--reason", "bad mapping"})
	assertError(t, err)
	assertErrorContains(t, err, "database URL required")
}

// ---------------------------------------------------------------------------
// runTerminologyAutoroute — validation tests
// ---------------------------------------------------------------------------

func TestRunTerminologyAutoroute_NoArgs(t *testing.T) {
	err := runTerminologyAutoroute([]string{})
	assertError(t, err)
	assertErrorContains(t, err, "source code required")
}

func TestRunTerminologyAutoroute_NoDB(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "")
	t.Setenv("FI_FHIR_DATABASE_URL", "")
	err := runTerminologyAutoroute([]string{"GLU001"})
	assertError(t, err)
	assertErrorContains(t, err, "database URL required")
}

func TestRunTerminologyAutoroute_MissingSourceSystem(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "postgres://localhost/test")
	err := runTerminologyAutoroute([]string{"GLU001", "--target-system", "http://loinc.org"})
	assertError(t, err)
	assertErrorContains(t, err, "--source-system is required")
}

func TestRunTerminologyAutoroute_MissingTargetSystem(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "postgres://localhost/test")
	err := runTerminologyAutoroute([]string{"GLU001", "--source-system", "epic_labs"})
	assertError(t, err)
	assertErrorContains(t, err, "--target-system is required")
}

// ---------------------------------------------------------------------------
// runTerminologyMapping — dispatch tests for new subcommands
// ---------------------------------------------------------------------------

func TestRunTerminologyMapping_PendingDispatch(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "")
	t.Setenv("FI_FHIR_DATABASE_URL", "")
	// pending without DB → known error path
	err := runTerminologyMapping([]string{"pending"})
	assertError(t, err)
	assertErrorContains(t, err, "database URL required")
}

func TestRunTerminologyMapping_ApproveDispatch(t *testing.T) {
	// approve without args → known error path
	err := runTerminologyMapping([]string{"approve"})
	assertError(t, err)
	assertErrorContains(t, err, "pending autoroute ID required")
}

func TestRunTerminologyMapping_RejectDispatch(t *testing.T) {
	// reject without args → known error path
	err := runTerminologyMapping([]string{"reject"})
	assertError(t, err)
	assertErrorContains(t, err, "pending autoroute ID required")
}

// ---------------------------------------------------------------------------
// runTerminology — dispatch test for autoroute
// ---------------------------------------------------------------------------

func TestRunTerminology_AutorouteDispatch(t *testing.T) {
	// autoroute without args → known error path
	err := runTerminology([]string{"autoroute"})
	assertError(t, err)
	assertErrorContains(t, err, "source code required")
}

// ---------------------------------------------------------------------------
// Usage text tests — ensure new commands appear in help
// ---------------------------------------------------------------------------

func TestPrintTerminologyUsage_IncludesAutoroute(t *testing.T) {
	stdout, _ := captureOutput(t, func() {
		printTerminologyUsage()
	})
	assertContains(t, stdout, "autoroute")
}

func TestPrintTerminologyMappingUsage_IncludesPendingApproveReject(t *testing.T) {
	stdout, _ := captureOutput(t, func() {
		printTerminologyMappingUsage()
	})
	assertContains(t, stdout, "pending")
	assertContains(t, stdout, "approve")
	assertContains(t, stdout, "reject")
}
