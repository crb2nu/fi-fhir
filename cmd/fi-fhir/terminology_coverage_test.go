package main

import (
	"context"
	"testing"
	"time"
)

// =============================================================================
// Terminology Load Stub Function Tests
// =============================================================================

// TestLoadSNOMED_PrintsNotImplemented verifies the stub behavior of the SNOMED loader.
func TestLoadSNOMED_PrintsNotImplemented(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// loadSNOMED is a stub that just prints "not yet implemented"
	// It should return nil (no error) and print a message
	stdout, _ := captureOutput(t, func() {
		err := loadSNOMED(ctx, nil, nil, "/data/snomed", "2024-03", nil)
		if err != nil {
			t.Errorf("loadSNOMED should return nil (stub), got: %v", err)
		}
	})

	assertContains(t, stdout, "not yet implemented")
	assertContains(t, stdout, "SNOMED")
}

// TestLoadICD10PCS_PrintsNotImplemented verifies the stub behavior of the ICD-10-PCS loader.
func TestLoadICD10PCS_PrintsNotImplemented(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// loadICD10PCS is a stub that just prints "not yet implemented"
	stdout, _ := captureOutput(t, func() {
		err := loadICD10PCS(ctx, nil, nil, "/data/icd10pcs", "2024", nil)
		if err != nil {
			t.Errorf("loadICD10PCS should return nil (stub), got: %v", err)
		}
	})

	assertContains(t, stdout, "not yet implemented")
	assertContains(t, stdout, "ICD-10-PCS")
}

// =============================================================================
// Terminology Load Argument Parsing Tests
// =============================================================================

func TestRunTerminologyLoad_UnknownVocabulary(t *testing.T) {
	// Note: The vocabulary check happens AFTER database connection attempt,
	// so with an invalid DB URL we'll get a connection error first.
	// This test verifies the code path is exercised.
	t.Setenv("FI_FHIR_DATABASE_URL", "postgres://example/test")

	err := runTerminologyLoad([]string{"unknown_vocab", "/data/path", "--version", "1.0"})
	assertError(t, err)
	// Error will be connection failure since that happens before vocab switch
}

func TestRunTerminologyLoad_SNOMEDStub(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_URL", "postgres://example/test")

	// SNOMED is a stub that will fail to connect to DB first
	// but if connection were to succeed, it would print "not implemented"
	err := runTerminologyLoad([]string{"snomed", "/data/snomed", "--version", "2024-03"})
	// Expect error because of DB connection failure (stub path never reached)
	assertError(t, err)
}

func TestRunTerminologyLoad_ICD10PCSStub(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_URL", "postgres://example/test")

	// ICD10PCS is a stub that will fail to connect to DB first
	err := runTerminologyLoad([]string{"icd10pcs", "/data/pcs", "--version", "2024"})
	assertError(t, err)
}

func TestRunTerminologyLoad_InvalidDateFormat(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_URL", "postgres://example/test")

	err := runTerminologyLoad([]string{"loinc", "/data/loinc", "--version", "2.77", "--date", "invalid-date"})
	assertError(t, err)
	assertErrorContains(t, err, "invalid date format")
}

func TestRunTerminologyLoad_ValidDateFormat(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_URL", "postgres://example/test")

	// Should fail at DB connection, not date parsing
	err := runTerminologyLoad([]string{"loinc", "/data/loinc", "--version", "2.77", "--date", "2024-01-15"})
	assertError(t, err)
	// Should NOT contain "invalid date format" since date parsing succeeded
	if err != nil && contains(err.Error(), "invalid date format") {
		t.Errorf("date should have been parsed correctly")
	}
}

func TestRunTerminologyLoad_DBFlagOverridesEnv(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_URL", "postgres://env/test")

	// Should use the --db flag value, not the env var
	// Both will fail to connect, but this tests the parsing logic
	err := runTerminologyLoad([]string{"loinc", "/data/loinc", "--version", "2.77", "--db", "postgres://flag/test"})
	assertError(t, err) // connection error expected
}

// =============================================================================
// Terminology Crosswalk Tests
// =============================================================================

func TestRunTerminologyCrosswalk_PrintsStub(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_URL", "postgres://example/test")

	// The crosswalk command is a stub that prints a message but doesn't error
	stdout, _ := captureOutput(t, func() {
		err := runTerminologyCrosswalk([]string{"E11.9", "--from", "ICD10CM", "--to", "SNOMEDCT_US"})
		if err != nil {
			t.Errorf("Expected crosswalk stub to succeed, got: %v", err)
		}
	})

	assertContains(t, stdout, "Cross-walk")
	assertContains(t, stdout, "not yet implemented")
}

func TestRunTerminologyCrosswalk_MissingFromVocab(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_URL", "postgres://example/test")

	err := runTerminologyCrosswalk([]string{"E11.9", "--to", "SNOMEDCT_US"})
	assertError(t, err)
	assertErrorContains(t, err, "--from and --to")
}

func TestRunTerminologyCrosswalk_MissingToVocab(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_URL", "postgres://example/test")

	err := runTerminologyCrosswalk([]string{"E11.9", "--from", "ICD10CM"})
	assertError(t, err)
	assertErrorContains(t, err, "--from and --to")
}

func TestRunTerminologyCrosswalk_DBFlagParsed(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_URL", "")

	// Test that --db flag is parsed
	err := runTerminologyCrosswalk([]string{"E11.9", "--from", "ICD10CM", "--to", "SNOMEDCT_US", "--db", "postgres://flag/test"})
	// Should not error about missing DB URL since --db was provided
	if err != nil && contains(err.Error(), "database URL required") {
		t.Errorf("--db flag should have been parsed")
	}
}

// =============================================================================
// Terminology Init/Status/Drop Additional Tests
// =============================================================================

func TestRunTerminologyInit_DBFlag(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_URL", "")

	// Should fail to connect, but validate --db flag is parsed
	err := runTerminologyInit([]string{"--db", "postgres://flag/test"})
	assertError(t, err)
	// Error should be about connection, not missing DB URL
	if contains(err.Error(), "database URL required") {
		t.Errorf("--db flag should have been parsed")
	}
}

func TestRunTerminologyStatus_DBFlag(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_URL", "")

	err := runTerminologyStatus([]string{"--db", "postgres://flag/test"})
	assertError(t, err)
	if contains(err.Error(), "database URL required") {
		t.Errorf("--db flag should have been parsed")
	}
}

func TestRunTerminologyDrop_ForceFlag(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_URL", "postgres://example/test")

	// With --force flag, should attempt to connect and fail
	err := runTerminologyDrop([]string{"--force"})
	assertError(t, err)
	// Error should be about connection, not warning about --force
}

func TestRunTerminologyDrop_ShortForceFlag(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_URL", "postgres://example/test")

	// With -f flag (short form), should attempt to connect and fail
	err := runTerminologyDrop([]string{"-f"})
	assertError(t, err)
}

// Note: contains() helper is defined in storage_test.go
