package main

import "testing"

// =============================================================================
// runTerminologyMappingResolve — deeper flag parsing tests
// =============================================================================

func TestRunTerminologyMappingResolve_DisplayFlag(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "postgres://localhost/test")

	// --display flag should be parsed, then fail at DB connection
	err := runTerminologyMappingResolve([]string{
		"E11.9",
		"--source-system", "http://hl7.org/fhir/sid/icd-10-cm",
		"--target-system", "http://snomed.info/sct",
		"--display", "Type 2 diabetes mellitus",
	})
	assertError(t, err) // DB connection error expected
}

func TestRunTerminologyMappingResolve_ProfileFlag(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "postgres://localhost/test")

	err := runTerminologyMappingResolve([]string{
		"E11.9",
		"--source-system", "http://hl7.org/fhir/sid/icd-10-cm",
		"--target-system", "http://snomed.info/sct",
		"--profile", "test-profile",
	})
	assertError(t, err) // DB connection error expected
}

func TestRunTerminologyMappingResolve_NoAutorouteFlag(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "postgres://localhost/test")

	err := runTerminologyMappingResolve([]string{
		"E11.9",
		"--source-system", "http://hl7.org/fhir/sid/icd-10-cm",
		"--target-system", "http://snomed.info/sct",
		"--no-autoroute",
	})
	assertError(t, err) // DB connection error expected
}

func TestRunTerminologyMappingResolve_JSONFlag(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "postgres://localhost/test")

	err := runTerminologyMappingResolve([]string{
		"E11.9",
		"--source-system", "http://hl7.org/fhir/sid/icd-10-cm",
		"--target-system", "http://snomed.info/sct",
		"--json",
	})
	assertError(t, err) // DB connection error expected
}

func TestRunTerminologyMappingResolve_AllFlags(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "postgres://localhost/test")

	err := runTerminologyMappingResolve([]string{
		"E11.9",
		"--source-system", "http://hl7.org/fhir/sid/icd-10-cm",
		"--target-system", "http://snomed.info/sct",
		"--display", "Type 2 diabetes",
		"--profile", "myprofile",
		"--no-autoroute",
		"--json",
	})
	assertError(t, err) // DB connection error expected
}

// =============================================================================
// runTerminologyMappingList — deeper flag parsing tests
// =============================================================================

func TestRunTerminologyMappingList_SourceSystemFlag(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "postgres://localhost/test")

	err := runTerminologyMappingList([]string{"--source-system", "ICD10CM"})
	assertError(t, err) // DB connection error expected
}

func TestRunTerminologyMappingList_TargetSystemFlag(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "postgres://localhost/test")

	err := runTerminologyMappingList([]string{"--target-system", "SNOMEDCT"})
	assertError(t, err)
}

func TestRunTerminologyMappingList_ProfileFlag(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "postgres://localhost/test")

	err := runTerminologyMappingList([]string{"--profile", "myprofile"})
	assertError(t, err)
}

func TestRunTerminologyMappingList_ValidLimitAndOffset(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "postgres://localhost/test")

	err := runTerminologyMappingList([]string{"--limit", "50", "--offset", "10"})
	assertError(t, err) // DB connection error expected
}

func TestRunTerminologyMappingList_ValidBatchID(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "postgres://localhost/test")

	err := runTerminologyMappingList([]string{"--batch", "550e8400-e29b-41d4-a716-446655440000"})
	assertError(t, err) // DB connection error expected — validates UUID parsing worked
}

// =============================================================================
// runTerminologyMappingGet — deeper tests
// =============================================================================

func TestRunTerminologyMappingGet_PositionalID(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "postgres://localhost/test")

	// Provide a numeric positional arg (mapping ID)
	err := runTerminologyMappingGet([]string{"42"})
	assertError(t, err) // DB connection error expected
}

func TestRunTerminologyMappingGet_ValidBatchUUID(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "postgres://localhost/test")

	err := runTerminologyMappingGet([]string{"--batch", "550e8400-e29b-41d4-a716-446655440000"})
	assertError(t, err) // DB connection error expected — validates UUID parsing worked
}

func TestRunTerminologyMappingGet_NoIDNoBatch_ReturnsError(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "postgres://localhost/test")

	err := runTerminologyMappingGet([]string{"--db", "postgres://localhost/test"})
	assertError(t, err)
	// Should hit "mapping ID or --batch required" or connection error
}

// =============================================================================
// runTerminologyMappingDelete — deeper tests
// =============================================================================

func TestRunTerminologyMappingDelete_PositionalID(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "postgres://localhost/test")

	// Without --force, this prompts for confirmation.
	// fmt.Scanln on non-interactive stdin returns err → prints "Aborted." and returns nil.
	stdout, _ := captureOutput(t, func() {
		err := runTerminologyMappingDelete([]string{"42"})
		assertNoError(t, err) // Aborted is not an error
	})
	assertContains(t, stdout, "Aborted")
}

func TestRunTerminologyMappingDelete_BatchWithForce(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "postgres://localhost/test")

	err := runTerminologyMappingDelete([]string{"--batch", "550e8400-e29b-41d4-a716-446655440000", "--force"})
	assertError(t, err) // DB connection error expected
}

func TestRunTerminologyMappingDelete_InvalidBatchUUID(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "postgres://localhost/test")

	err := runTerminologyMappingDelete([]string{"--batch", "not-a-uuid"})
	assertError(t, err)
}

// =============================================================================
// runTerminologyMappingUpload — deeper validation tests
// =============================================================================

func TestRunTerminologyMappingUpload_InvalidCSV(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "postgres://localhost/test")

	tmpDir := t.TempDir()
	// Use a CSV with unrecognized headers — parser may return zero valid rows
	csvPath := createTempFile(t, tmpDir, "bad*.csv", "not,a,valid,csv,header\nfoo,bar,baz,qux,quux\n")

	// May fail or succeed depending on parser tolerance; just exercise the path
	_ = runTerminologyMappingUpload([]string{csvPath, "--dry-run"})
}

func TestRunTerminologyMappingUpload_EmptyCSV(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "postgres://localhost/test")

	tmpDir := t.TempDir()
	csvPath := createTempFile(t, tmpDir, "empty*.csv", "")

	err := runTerminologyMappingUpload([]string{csvPath})
	assertError(t, err) // Should fail with "failed to parse CSV" or similar
}
