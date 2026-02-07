package main

import (
	"testing"
)

// ---------------------------------------------------------------------------
// runTerminologyMapping — subcommand dispatch tests
// ---------------------------------------------------------------------------

func TestRunTerminologyMapping_NoArgs(t *testing.T) {
	// No args → prints usage, no error
	stdout, _ := captureOutput(t, func() {
		err := runTerminologyMapping([]string{})
		assertNoError(t, err)
	})
	assertContains(t, stdout, "terminology mapping")
}

func TestRunTerminologyMapping_Help(t *testing.T) {
	stdout, _ := captureOutput(t, func() {
		err := runTerminologyMapping([]string{"help"})
		assertNoError(t, err)
	})
	assertContains(t, stdout, "terminology mapping")
}

func TestRunTerminologyMapping_UnknownSubcommand(t *testing.T) {
	err := runTerminologyMapping([]string{"bogus"})
	assertError(t, err)
	assertErrorContains(t, err, "unknown mapping subcommand")
}

// ---------------------------------------------------------------------------
// runTerminologyMappingUpload — early validation tests
// ---------------------------------------------------------------------------

func TestRunTerminologyMappingUpload_MissingFile(t *testing.T) {
	err := runTerminologyMappingUpload([]string{})
	assertError(t, err)
	assertErrorContains(t, err, "CSV file path required")
}

func TestRunTerminologyMappingUpload_NoDBURL(t *testing.T) {
	// File provided but no DB URL → error about database
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "")
	t.Setenv("FI_FHIR_DATABASE_URL", "")
	err := runTerminologyMappingUpload([]string{"/tmp/test.csv"})
	assertError(t, err)
	assertErrorContains(t, err, "database URL required")
}

func TestRunTerminologyMappingUpload_NonexistentFile(t *testing.T) {
	// Provide a DB URL but a nonexistent file → file open error
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "postgres://localhost/test")
	err := runTerminologyMappingUpload([]string{"/tmp/does-not-exist-abc123.csv"})
	assertError(t, err)
	assertErrorContains(t, err, "failed to open file")
}

// ---------------------------------------------------------------------------
// runTerminologyMappingList — flag parsing tests
// ---------------------------------------------------------------------------

func TestRunTerminologyMappingList_NoDB(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "")
	t.Setenv("FI_FHIR_DATABASE_URL", "")
	err := runTerminologyMappingList([]string{})
	assertError(t, err)
	assertErrorContains(t, err, "database URL required")
}

// ---------------------------------------------------------------------------
// runTerminologyMappingGet — early validation tests
// ---------------------------------------------------------------------------

func TestRunTerminologyMappingGet_NoDB(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "")
	t.Setenv("FI_FHIR_DATABASE_URL", "")
	err := runTerminologyMappingGet([]string{})
	assertError(t, err)
	assertErrorContains(t, err, "database URL required")
}

// ---------------------------------------------------------------------------
// runTerminologyMappingDelete — early validation tests
// ---------------------------------------------------------------------------

func TestRunTerminologyMappingDelete_NoDB(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "")
	t.Setenv("FI_FHIR_DATABASE_URL", "")
	err := runTerminologyMappingDelete([]string{})
	assertError(t, err)
	assertErrorContains(t, err, "database URL required")
}

// ---------------------------------------------------------------------------
// runTerminologyMappingResolve — early validation tests
// ---------------------------------------------------------------------------

func TestRunTerminologyMappingResolve_MissingCode(t *testing.T) {
	err := runTerminologyMappingResolve([]string{})
	assertError(t, err)
	assertErrorContains(t, err, "source code required")
}

func TestRunTerminologyMappingResolve_NoDB(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "")
	t.Setenv("FI_FHIR_DATABASE_URL", "")
	err := runTerminologyMappingResolve([]string{"E11.9"})
	assertError(t, err)
	assertErrorContains(t, err, "database URL required")
}

func TestRunTerminologyMappingResolve_MissingSourceSystem(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "postgres://localhost/test")
	err := runTerminologyMappingResolve([]string{"E11.9", "--target-system", "http://snomed.info/sct"})
	assertError(t, err)
	assertErrorContains(t, err, "--source-system is required")
}

func TestRunTerminologyMappingResolve_MissingTargetSystem(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "postgres://localhost/test")
	err := runTerminologyMappingResolve([]string{"E11.9", "--source-system", "http://hl7.org/fhir/sid/icd-10-cm"})
	assertError(t, err)
	assertErrorContains(t, err, "--target-system is required")
}

// ---------------------------------------------------------------------------
// Pure helper functions — table-driven tests
// ---------------------------------------------------------------------------

func TestParseInt(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int
		wantErr bool
	}{
		{"valid", "42", 42, false},
		{"zero", "0", 0, false},
		{"negative", "-5", -5, false},
		{"invalid string", "abc", 0, true},
		{"empty", "", 0, true},
		// fmt.Sscanf with %d parses "3" from "3.14" (stops at dot) — no error, returns 3
		{"float string parses leading int", "3.14", 3, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseInt(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseInt(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("parseInt(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseUUID(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid uuid", "550e8400-e29b-41d4-a716-446655440000", false},
		{"invalid string", "not-a-uuid", true},
		{"empty", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseUUID(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseUUID(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		name   string
		s      string
		maxLen int
		want   string
	}{
		{"short string", "hi", 10, "hi"},
		{"exact length", "hello", 5, "hello"},
		{"over limit", "hello world", 8, "hello..."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := truncate(tt.s, tt.maxLen); got != tt.want {
				t.Errorf("truncate(%q, %d) = %q, want %q", tt.s, tt.maxLen, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// printTerminologyMappingUsage — captureOutput test
// ---------------------------------------------------------------------------

func TestPrintTerminologyMappingUsage(t *testing.T) {
	stdout, _ := captureOutput(t, func() {
		printTerminologyMappingUsage()
	})
	assertContains(t, stdout, "terminology mapping")
	assertContains(t, stdout, "upload")
	assertContains(t, stdout, "resolve")
}

// ---------------------------------------------------------------------------
// Integration via runCLI dispatcher
// ---------------------------------------------------------------------------

func TestRunCLI_TerminologyMappingHelp(t *testing.T) {
	stdout, _, err := runCLI(t, "terminology", "mapping", "help")
	assertNoError(t, err)
	assertContains(t, stdout, "terminology mapping")
}

func TestRunCLI_TerminologyMappingUnknown(t *testing.T) {
	_, _, err := runCLI(t, "terminology", "mapping", "invalid-cmd")
	assertError(t, err)
	assertErrorContains(t, err, "unknown mapping subcommand")
}
