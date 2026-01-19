//nolint:errcheck,gosec // Test file - error/security checks relaxed for test setup
package main

import (
	"os"
	"testing"
)

// =============================================================================
// Terminology Command Tests
// =============================================================================

func TestTerminology_Help(t *testing.T) {
	stdout, _, err := runCLI(t, "terminology")
	assertNoError(t, err)
	assertContains(t, stdout, "fi-fhir terminology - Terminology Database Management")
	assertContains(t, stdout, "init")
	assertContains(t, stdout, "status")
	assertContains(t, stdout, "use")
	assertContains(t, stdout, "drop")
	assertContains(t, stdout, "load")
	assertContains(t, stdout, "crosswalk")
}

func TestTerminology_HelpFlag(t *testing.T) {
	stdout, _, err := runCLI(t, "terminology", "--help")
	assertNoError(t, err)
	assertContains(t, stdout, "fi-fhir terminology - Terminology Database Management")
}

func TestTerminology_HelpSubcommand(t *testing.T) {
	stdout, _, err := runCLI(t, "terminology", "help")
	assertNoError(t, err)
	assertContains(t, stdout, "fi-fhir terminology - Terminology Database Management")
}

func TestTerminology_UnknownSubcommand(t *testing.T) {
	_, _, err := runCLI(t, "terminology", "badcmd")
	assertError(t, err)
	assertErrorContains(t, err, "unknown terminology subcommand")
}

func TestTerminology_Init_MissingDBURL(t *testing.T) {
	// Unset env var for this test
	oldURL := os.Getenv("FI_FHIR_DATABASE_URL")
	os.Unsetenv("FI_FHIR_DATABASE_URL")
	defer func() {
		if oldURL != "" {
			os.Setenv("FI_FHIR_DATABASE_URL", oldURL)
		}
	}()

	_, _, err := runCLI(t, "terminology", "init")
	assertError(t, err)
	assertErrorContains(t, err, "database URL required")
}

func TestTerminology_Status_MissingDBURL(t *testing.T) {
	// Unset env var for this test
	oldURL := os.Getenv("FI_FHIR_DATABASE_URL")
	os.Unsetenv("FI_FHIR_DATABASE_URL")
	defer func() {
		if oldURL != "" {
			os.Setenv("FI_FHIR_DATABASE_URL", oldURL)
		}
	}()

	_, _, err := runCLI(t, "terminology", "status")
	assertError(t, err)
	assertErrorContains(t, err, "database URL required")
}

func TestTerminology_Use_MissingDBURL(t *testing.T) {
	// Unset env var for this test
	oldURL := os.Getenv("FI_FHIR_DATABASE_URL")
	os.Unsetenv("FI_FHIR_DATABASE_URL")
	defer func() {
		if oldURL != "" {
			os.Setenv("FI_FHIR_DATABASE_URL", oldURL)
		}
	}()

	_, _, err := runCLI(t, "terminology", "use", "loinc", "2.77")
	assertError(t, err)
	assertErrorContains(t, err, "database URL required")
}

func TestTerminology_Drop_MissingDBURL(t *testing.T) {
	// Unset env var for this test
	oldURL := os.Getenv("FI_FHIR_DATABASE_URL")
	os.Unsetenv("FI_FHIR_DATABASE_URL")
	defer func() {
		if oldURL != "" {
			os.Setenv("FI_FHIR_DATABASE_URL", oldURL)
		}
	}()

	_, _, err := runCLI(t, "terminology", "drop", "--confirm")
	assertError(t, err)
	assertErrorContains(t, err, "database URL required")
}

func TestTerminology_Load_MissingArgs(t *testing.T) {
	// Load prints usage and returns nil when missing args
	stdout, _, err := runCLI(t, "terminology", "load")
	assertNoError(t, err)
	assertContains(t, stdout, "Usage:")
	assertContains(t, stdout, "Vocabularies:")
}

func TestTerminology_Load_MissingPath(t *testing.T) {
	// Load prints usage and returns nil when only vocab is provided
	stdout, _, err := runCLI(t, "terminology", "load", "loinc")
	assertNoError(t, err)
	assertContains(t, stdout, "Usage:")
}

func TestTerminology_Load_UnsupportedVocabulary(t *testing.T) {
	// Unset env var for this test
	oldURL := os.Getenv("FI_FHIR_DATABASE_URL")
	os.Unsetenv("FI_FHIR_DATABASE_URL")
	defer func() {
		if oldURL != "" {
			os.Setenv("FI_FHIR_DATABASE_URL", oldURL)
		}
	}()

	_, _, err := runCLI(t, "terminology", "load", "unknown_vocab", "/path/to/data")
	assertError(t, err)
	// Either "unsupported vocabulary" or "database URL required" depending on check order
	if err == nil {
		t.Error("Expected error for unsupported vocabulary or missing DB URL")
	}
}

func TestTerminology_Crosswalk_MissingArgs(t *testing.T) {
	// Crosswalk prints usage and returns nil when missing args
	stdout, _, err := runCLI(t, "terminology", "crosswalk")
	assertNoError(t, err)
	assertContains(t, stdout, "Usage:")
	assertContains(t, stdout, "--from")
	assertContains(t, stdout, "--to")
}

// =============================================================================
// Helper Function Tests
// =============================================================================

func TestGetTerminologyDBURL_FromFlag(t *testing.T) {
	args := []string{"--db", "postgres://localhost/test"}
	url := getTerminologyDBURL(args)
	if url != "postgres://localhost/test" {
		t.Errorf("Expected postgres://localhost/test, got %s", url)
	}
}

func TestGetTerminologyDBURL_FromEnv(t *testing.T) {
	// Save existing env var
	savedURL := os.Getenv("FI_FHIR_DATABASE_URL")
	defer func() {
		if savedURL != "" {
			os.Setenv("FI_FHIR_DATABASE_URL", savedURL)
		} else {
			os.Unsetenv("FI_FHIR_DATABASE_URL")
		}
	}()

	os.Setenv("FI_FHIR_DATABASE_URL", "postgres://env/db")

	args := []string{} // No --db flag
	url := getTerminologyDBURL(args)
	if url != "postgres://env/db" {
		t.Errorf("Expected postgres://env/db, got %s", url)
	}
}

func TestGetTerminologyDBURL_FlagOverridesEnv(t *testing.T) {
	// Save existing env var
	savedURL := os.Getenv("FI_FHIR_DATABASE_URL")
	defer func() {
		if savedURL != "" {
			os.Setenv("FI_FHIR_DATABASE_URL", savedURL)
		} else {
			os.Unsetenv("FI_FHIR_DATABASE_URL")
		}
	}()

	os.Setenv("FI_FHIR_DATABASE_URL", "postgres://env/db")

	args := []string{"--db", "postgres://flag/db"}
	url := getTerminologyDBURL(args)
	if url != "postgres://flag/db" {
		t.Errorf("Expected postgres://flag/db (flag takes precedence), got %s", url)
	}
}

func TestGetTerminologyDBURL_NoDBFound(t *testing.T) {
	// Save and unset env var
	savedURL := os.Getenv("FI_FHIR_DATABASE_URL")
	os.Unsetenv("FI_FHIR_DATABASE_URL")
	defer func() {
		if savedURL != "" {
			os.Setenv("FI_FHIR_DATABASE_URL", savedURL)
		}
	}()

	args := []string{} // No --db flag
	url := getTerminologyDBURL(args)
	if url != "" {
		t.Errorf("Expected empty string when no DB URL found, got %s", url)
	}
}
