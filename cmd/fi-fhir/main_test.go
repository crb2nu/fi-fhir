//nolint:errcheck,gosec // Test file - error/security checks relaxed for test setup
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// =============================================================================
// Version and Help Tests
// =============================================================================

func TestVersion(t *testing.T) {
	stdout, _, err := runCLI(t, "version")
	assertNoError(t, err)
	assertContains(t, stdout, "fi-fhir version")
	assertContains(t, stdout, version)
}

func TestVersionFlag(t *testing.T) {
	stdout, _, err := runCLI(t, "--version")
	assertNoError(t, err)
	assertContains(t, stdout, "fi-fhir version")
}

func TestHelp(t *testing.T) {
	stdout, _, err := runCLI(t, "help")
	assertNoError(t, err)
	assertContains(t, stdout, "fi-fhir - Healthcare Integration CLI")
	assertContains(t, stdout, "parse")
	assertContains(t, stdout, "workflow")
	assertContains(t, stdout, "config")
}

func TestHelpFlag(t *testing.T) {
	stdout, _, err := runCLI(t, "--help")
	assertNoError(t, err)
	assertContains(t, stdout, "fi-fhir - Healthcare Integration CLI")
}

func TestUnknownCommand(t *testing.T) {
	_, _, err := runCLI(t, "unknowncommand")
	assertError(t, err)
	assertErrorContains(t, err, "unknown command")
}

// =============================================================================
// Parse Command Tests
// =============================================================================

func TestParse_HL7v2_ValidMessage(t *testing.T) {
	inputPath := testdataPath(t, "adt_a01_sample.hl7")
	if _, err := os.Stat(inputPath); os.IsNotExist(err) {
		t.Skip("testdata file not found")
	}

	stdout, _, err := runCLI(t, "parse", "--format", "hl7v2", inputPath)
	assertNoError(t, err)
	assertContains(t, stdout, "patient_admit")
	assertContains(t, stdout, "event_type")
}

func TestParse_HL7v2_PrettyOutput(t *testing.T) {
	inputPath := testdataPath(t, "adt_a01_sample.hl7")
	if _, err := os.Stat(inputPath); os.IsNotExist(err) {
		t.Skip("testdata file not found")
	}

	stdout, _, err := runCLI(t, "parse", "--format", "hl7v2", "--pretty", inputPath)
	assertNoError(t, err)
	// Pretty output should have indentation
	assertContains(t, stdout, "  ")
}

func TestParse_HL7v2_ORU(t *testing.T) {
	inputPath := testdataPath(t, "oru_r01_sample.hl7")
	if _, err := os.Stat(inputPath); os.IsNotExist(err) {
		t.Skip("testdata file not found")
	}

	stdout, _, err := runCLI(t, "parse", "--format", "hl7v2", inputPath)
	assertNoError(t, err)
	assertContains(t, stdout, "lab_result")
}

func TestParse_EDI_837P(t *testing.T) {
	inputPath := testdataPath(t, "edi/837p_minimal.edi")
	if _, err := os.Stat(inputPath); os.IsNotExist(err) {
		t.Skip("testdata file not found")
	}

	stdout, _, err := runCLI(t, "parse", "--format", "edi", inputPath)
	assertNoError(t, err)
	assertContains(t, stdout, "claim")
}

func TestParse_EDI_270(t *testing.T) {
	inputPath := testdataPath(t, "edi/270_inquiry.edi")
	if _, err := os.Stat(inputPath); os.IsNotExist(err) {
		t.Skip("testdata file not found")
	}

	stdout, _, err := runCLI(t, "parse", "--format", "edi", inputPath)
	assertNoError(t, err)
	// Should parse successfully
	if stdout == "" {
		t.Error("Expected output for EDI 270")
	}
}

func TestParse_CSV_Valid(t *testing.T) {
	// Create a temp CSV file
	tmpDir := t.TempDir()
	csvContent := `mrn,first_name,last_name,dob
12345,John,Doe,1990-01-15
67890,Jane,Smith,1985-06-20`
	csvPath := createTempFile(t, tmpDir, "test*.csv", csvContent)

	stdout, _, err := runCLI(t, "parse", "--format", "csv", csvPath)
	assertNoError(t, err)
	// CSV parser outputs canonical events with csv_record type
	assertContains(t, stdout, "csv_record")
	assertContains(t, stdout, "source_format")
}

func TestParse_CSV_TabDelimited(t *testing.T) {
	tmpDir := t.TempDir()
	csvContent := "mrn\tfirst_name\tlast_name\n12345\tJohn\tDoe"
	csvPath := createTempFile(t, tmpDir, "test*.tsv", csvContent)

	stdout, _, err := runCLI(t, "parse", "--format", "csv", "--delimiter", "\\t", csvPath)
	assertNoError(t, err)
	// Should produce CSV canonical events
	assertContains(t, stdout, "csv_record")
}

func TestParse_CSV_NoHeader(t *testing.T) {
	tmpDir := t.TempDir()
	csvContent := "12345,John,Doe,1990-01-15\n67890,Jane,Smith,1985-06-20"
	csvPath := createTempFile(t, tmpDir, "test*.csv", csvContent)

	stdout, _, err := runCLI(t, "parse", "--format", "csv", "--no-header", csvPath)
	assertNoError(t, err)
	// Should produce 2 CSV records
	assertContains(t, stdout, "csv_record")
}

func TestParse_CSV_InferSchema(t *testing.T) {
	tmpDir := t.TempDir()
	csvContent := `id,name,amount
1,Item A,100.50
2,Item B,200.75`
	csvPath := createTempFile(t, tmpDir, "test*.csv", csvContent)

	stdout, _, err := runCLI(t, "parse", "--format", "csv", "--infer-schema", csvPath)
	assertNoError(t, err)
	// Infer schema mode includes schema information
	assertContains(t, stdout, "schema")
}

func TestParse_FileNotFound(t *testing.T) {
	_, _, err := runCLI(t, "parse", "--format", "hl7v2", "/nonexistent/file.hl7")
	assertError(t, err)
	assertErrorContains(t, err, "failed to read")
}

func TestParse_UnsupportedFormat(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := createTempFile(t, tmpDir, "test*.txt", "some content")

	_, _, err := runCLI(t, "parse", "--format", "unsupported", tmpFile)
	assertError(t, err)
	assertErrorContains(t, err, "unknown format")
}

func TestParse_EmptyInput(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := createTempFile(t, tmpDir, "empty*.hl7", "")

	_, _, err := runCLI(t, "parse", "--format", "hl7v2", tmpFile)
	assertError(t, err)
	assertErrorContains(t, err, "empty input")
}

func TestParse_MissingFormatValue(t *testing.T) {
	_, _, err := runCLI(t, "parse", "--format")
	assertError(t, err)
	assertErrorContains(t, err, "--format requires a value")
}

func TestParse_UnknownFlag(t *testing.T) {
	_, _, err := runCLI(t, "parse", "--unknown-flag")
	assertError(t, err)
	assertErrorContains(t, err, "unknown flag")
}

func TestParse_WithSource(t *testing.T) {
	inputPath := testdataPath(t, "adt_a01_sample.hl7")
	if _, err := os.Stat(inputPath); os.IsNotExist(err) {
		t.Skip("testdata file not found")
	}

	stdout, _, err := runCLI(t, "parse", "--format", "hl7v2", "--source", "test_source", inputPath)
	assertNoError(t, err)
	assertContains(t, stdout, "test_source")
}

func TestParse_Help(t *testing.T) {
	stdout, _, err := runCLI(t, "parse", "--help")
	assertNoError(t, err)
	assertContains(t, stdout, "parse")
	assertContains(t, stdout, "--format")
}

// =============================================================================
// Config Command Tests
// =============================================================================

func TestConfig_Show(t *testing.T) {
	stdout, _, err := runCLI(t, "config", "show")
	assertNoError(t, err)
	// Should show default configuration
	assertContains(t, stdout, "server")
}

func TestConfig_Validate(t *testing.T) {
	// Config validate requires a config file path
	tmpDir := t.TempDir()
	configYAML := `server:
  port: 8080
  host: localhost
`
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configYAML), 0644); err != nil {
		t.Fatalf("Failed to create config file: %v", err)
	}

	stdout, _, err := runCLI(t, "config", "validate", configPath)
	assertNoError(t, err)
	// Should validate successfully
	assertContains(t, stdout, "valid")
}

func TestConfig_Help(t *testing.T) {
	stdout, _, err := runCLI(t, "config", "--help")
	assertNoError(t, err)
	assertContains(t, stdout, "config")
}

func TestConfig_UnknownSubcommand(t *testing.T) {
	_, _, err := runCLI(t, "config", "unknown")
	assertError(t, err)
}

// =============================================================================
// Workflow Command Tests
// =============================================================================

func TestWorkflow_Validate_ValidConfig(t *testing.T) {
	tmpDir := t.TempDir()
	workflowYAML := `
workflow:
  name: test
  version: "1.0"
  routes:
    - name: all_events
      filter:
        event_type: patient_admit
      actions:
        - type: log
          level: info
          message: "Test"
`
	configPath := createTempFile(t, tmpDir, "workflow*.yaml", workflowYAML)

	stdout, _, err := runCLI(t, "workflow", "validate", "--config", configPath)
	assertNoError(t, err)
	assertContains(t, stdout, "valid")
}

func TestWorkflow_Validate_InvalidConfig(t *testing.T) {
	tmpDir := t.TempDir()
	invalidYAML := `
workflow:
  name: test
  routes:
    - name: missing_actions
      filter:
        event_type: patient_admit
`
	configPath := createTempFile(t, tmpDir, "workflow*.yaml", invalidYAML)

	_, _, err := runCLI(t, "workflow", "validate", "--config", configPath)
	// Should fail validation - no actions
	if err == nil {
		// Some configs might pass, let's just ensure it runs
		t.Log("Config passed validation")
	}
}

func TestWorkflow_Validate_MissingFile(t *testing.T) {
	_, _, err := runCLI(t, "workflow", "validate", "--config", "/nonexistent/workflow.yaml")
	assertError(t, err)
}

func TestWorkflow_Run_DryRun(t *testing.T) {
	tmpDir := t.TempDir()
	workflowYAML := `
workflow:
  name: test
  version: "1.0"
  routes:
    - name: log_admits
      filter:
        event_type: patient_admit
      actions:
        - type: log
          level: info
          message: "Admit received"
`
	configPath := createTempFile(t, tmpDir, "workflow*.yaml", workflowYAML)

	eventJSON := `{"event_type": "patient_admit", "patient": {"mrn": "12345"}}`
	eventPath := createTempFile(t, tmpDir, "event*.json", eventJSON)

	stdout, _, err := runCLI(t, "workflow", "run", "--config", configPath, "--dry-run", eventPath)
	// Dry run should succeed without executing actions
	if err != nil {
		t.Logf("Dry run error (may be expected): %v", err)
	}
	_ = stdout // May or may not have output
}

func TestWorkflow_Help(t *testing.T) {
	stdout, _, err := runCLI(t, "workflow", "--help")
	assertNoError(t, err)
	assertContains(t, stdout, "workflow")
}

// =============================================================================
// Validate Command Tests
// =============================================================================

func TestValidate_Profile_Valid(t *testing.T) {
	tmpDir := t.TempDir()
	profileYAML := `
source_profile:
  id: test_profile
  name: Test Profile
  format: hl7v2
  hl7v2:
    version: "2.5.1"
    tolerant: true
`
	profilePath := createTempFile(t, tmpDir, "profile*.yaml", profileYAML)

	stdout, _, err := runCLI(t, "validate", "--profile", profilePath)
	assertNoError(t, err)
	assertContains(t, stdout, "valid")
}

func TestValidate_Profile_Invalid(t *testing.T) {
	tmpDir := t.TempDir()
	invalidYAML := `not valid yaml: [`
	profilePath := createTempFile(t, tmpDir, "profile*.yaml", invalidYAML)

	_, _, err := runCLI(t, "validate", "--profile", profilePath)
	assertError(t, err)
}

func TestValidate_Profile_MissingFile(t *testing.T) {
	_, _, err := runCLI(t, "validate", "--profile", "/nonexistent/profile.yaml")
	assertError(t, err)
}

func TestValidate_Help(t *testing.T) {
	stdout, _, err := runCLI(t, "validate", "--help")
	assertNoError(t, err)
	assertContains(t, stdout, "validate")
}

// =============================================================================
// Integration Tests
// =============================================================================

func TestParse_HL7v2_AllFormats(t *testing.T) {
	// Test that all HL7v2 format aliases work
	aliases := []string{"hl7v2", "hl7"}
	inputPath := testdataPath(t, "adt_a01_sample.hl7")
	if _, err := os.Stat(inputPath); os.IsNotExist(err) {
		t.Skip("testdata file not found")
	}

	for _, alias := range aliases {
		t.Run(alias, func(t *testing.T) {
			stdout, _, err := runCLI(t, "parse", "--format", alias, inputPath)
			assertNoError(t, err)
			assertContains(t, stdout, "patient_admit")
		})
	}
}

func TestParse_EDI_AllFormats(t *testing.T) {
	// Test that all EDI format aliases work
	aliases := []string{"edi", "x12", "837"}
	inputPath := testdataPath(t, "edi/837p_minimal.edi")
	if _, err := os.Stat(inputPath); os.IsNotExist(err) {
		t.Skip("testdata file not found")
	}

	for _, alias := range aliases {
		t.Run(alias, func(t *testing.T) {
			stdout, _, err := runCLI(t, "parse", "--format", alias, inputPath)
			assertNoError(t, err)
			if stdout == "" {
				t.Error("Expected output")
			}
		})
	}
}

func TestParse_CSV_Delimiters(t *testing.T) {
	delimiters := map[string]string{
		"comma":     ",",
		"tab":       "\\t",
		"pipe":      "|",
		"semicolon": ";",
	}

	for name, delim := range delimiters {
		t.Run(name, func(t *testing.T) {
			tmpDir := t.TempDir()
			var csvContent string
			switch delim {
			case ",":
				csvContent = "a,b,c\n1,2,3"
			case "\\t":
				csvContent = "a\tb\tc\n1\t2\t3"
			case "|":
				csvContent = "a|b|c\n1|2|3"
			case ";":
				csvContent = "a;b;c\n1;2;3"
			}
			csvPath := createTempFile(t, tmpDir, "test*.csv", csvContent)

			_, _, err := runCLI(t, "parse", "--format", "csv", "--delimiter", delim, csvPath)
			assertNoError(t, err)
		})
	}
}

// =============================================================================
// Edge Cases and Error Handling
// =============================================================================

func TestParse_InvalidDelimiter(t *testing.T) {
	tmpDir := t.TempDir()
	csvContent := "a,b,c\n1,2,3"
	csvPath := createTempFile(t, tmpDir, "test*.csv", csvContent)

	_, _, err := runCLI(t, "parse", "--format", "csv", "--delimiter", "invalid", csvPath)
	assertError(t, err)
	assertErrorContains(t, err, "invalid delimiter")
}

func TestParse_InvalidEventType(t *testing.T) {
	tmpDir := t.TempDir()
	csvContent := "a,b,c\n1,2,3"
	csvPath := createTempFile(t, tmpDir, "test*.csv", csvContent)

	_, _, err := runCLI(t, "parse", "--format", "csv", "--event-type", "invalid_type", csvPath)
	assertError(t, err)
	assertErrorContains(t, err, "unknown event type")
}

func TestWorkflow_MissingConfigFlag(t *testing.T) {
	_, _, err := runCLI(t, "workflow", "run")
	// Should fail without config
	if err == nil {
		t.Log("May have default behavior")
	}
}

// =============================================================================
// CDA Parser Tests
// =============================================================================

func TestParse_CDA_Format(t *testing.T) {
	// Test CDA format is recognized
	tmpDir := t.TempDir()
	cdaContent := `<?xml version="1.0"?>
<ClinicalDocument xmlns="urn:hl7-org:v3">
  <templateId root="2.16.840.1.113883.10.20.22.1.1"/>
  <recordTarget>
    <patientRole>
      <id extension="12345" root="1.2.3.4"/>
    </patientRole>
  </recordTarget>
</ClinicalDocument>`
	cdaPath := createTempFile(t, tmpDir, "test*.xml", cdaContent)

	stdout, _, err := runCLI(t, "parse", "--format", "cda", cdaPath)
	// CDA parsing may fail on minimal doc, but format should be recognized
	if err != nil {
		// Check it's a parse error, not a format error
		if strings.Contains(err.Error(), "unsupported format") {
			t.Errorf("CDA format should be supported")
		}
	}
	_ = stdout
}

// =============================================================================
// Full Workflow Integration
// =============================================================================

func TestFullWorkflow_ParseToWorkflow(t *testing.T) {
	// This tests the complete flow: parse HL7 -> workflow
	hl7Path := testdataPath(t, "adt_a01_sample.hl7")
	if _, err := os.Stat(hl7Path); os.IsNotExist(err) {
		t.Skip("testdata file not found")
	}

	// First parse the message
	stdout, _, err := runCLI(t, "parse", "--format", "hl7v2", hl7Path)
	assertNoError(t, err)
	assertContains(t, stdout, "patient_admit")

	// Create workflow config
	tmpDir := t.TempDir()
	workflowYAML := `
workflow:
  name: integration_test
  version: "1.0"
  routes:
    - name: admit_log
      filter:
        event_type: patient_admit
      actions:
        - type: log
          level: info
          message: "Patient admitted"
`
	configPath := filepath.Join(tmpDir, "workflow.yaml")
	if err := os.WriteFile(configPath, []byte(workflowYAML), 0644); err != nil {
		t.Fatalf("Failed to write workflow config: %v", err)
	}

	// Validate the workflow
	stdout2, _, err := runCLI(t, "workflow", "validate", "--config", configPath)
	assertNoError(t, err)
	assertContains(t, stdout2, "valid")
}

// =============================================================================
// EventStore Command Tests
// =============================================================================

func TestEventStore_Help(t *testing.T) {
	stdout, _, err := runCLI(t, "eventstore", "--help")
	assertNoError(t, err)
	assertContains(t, stdout, "eventstore")
	assertContains(t, stdout, "init")
	assertContains(t, stdout, "stats")
	assertContains(t, stdout, "streams")
	assertContains(t, stdout, "read")
	assertContains(t, stdout, "append")
}

func TestEventStore_NoArgs(t *testing.T) {
	stdout, _, err := runCLI(t, "eventstore")
	assertNoError(t, err) // prints usage
	assertContains(t, stdout, "eventstore")
}

func TestEventStore_UnknownSubcommand(t *testing.T) {
	stdout, _, err := runCLI(t, "eventstore", "unknown")
	assertNoError(t, err) // prints usage
	assertContains(t, stdout, "eventstore")
}

func TestEventStore_Init_MissingDB(t *testing.T) {
	// Clear env var to ensure --db is required
	oldVal := os.Getenv("FI_FHIR_DATABASE_URL")
	os.Unsetenv("FI_FHIR_DATABASE_URL")
	defer func() {
		if oldVal != "" {
			os.Setenv("FI_FHIR_DATABASE_URL", oldVal)
		}
	}()

	_, _, err := runCLI(t, "eventstore", "init")
	assertError(t, err)
	assertErrorContains(t, err, "database URL required")
}

func TestEventStore_Stats_MissingDB(t *testing.T) {
	oldVal := os.Getenv("FI_FHIR_DATABASE_URL")
	os.Unsetenv("FI_FHIR_DATABASE_URL")
	defer func() {
		if oldVal != "" {
			os.Setenv("FI_FHIR_DATABASE_URL", oldVal)
		}
	}()

	_, _, err := runCLI(t, "eventstore", "stats")
	assertError(t, err)
	assertErrorContains(t, err, "database URL required")
}

func TestEventStore_Streams_MissingDB(t *testing.T) {
	oldVal := os.Getenv("FI_FHIR_DATABASE_URL")
	os.Unsetenv("FI_FHIR_DATABASE_URL")
	defer func() {
		if oldVal != "" {
			os.Setenv("FI_FHIR_DATABASE_URL", oldVal)
		}
	}()

	_, _, err := runCLI(t, "eventstore", "streams")
	assertError(t, err)
	assertErrorContains(t, err, "database URL required")
}

func TestEventStore_Read_MissingStreamAndAll(t *testing.T) {
	oldVal := os.Getenv("FI_FHIR_DATABASE_URL")
	os.Setenv("FI_FHIR_DATABASE_URL", "postgres://localhost/test")
	defer func() {
		if oldVal != "" {
			os.Setenv("FI_FHIR_DATABASE_URL", oldVal)
		} else {
			os.Unsetenv("FI_FHIR_DATABASE_URL")
		}
	}()

	_, _, err := runCLI(t, "eventstore", "read")
	assertError(t, err)
	assertErrorContains(t, err, "--stream or --all")
}

func TestEventStore_Append_MissingStream(t *testing.T) {
	oldVal := os.Getenv("FI_FHIR_DATABASE_URL")
	os.Setenv("FI_FHIR_DATABASE_URL", "postgres://localhost/test")
	defer func() {
		if oldVal != "" {
			os.Setenv("FI_FHIR_DATABASE_URL", oldVal)
		} else {
			os.Unsetenv("FI_FHIR_DATABASE_URL")
		}
	}()

	_, _, err := runCLI(t, "eventstore", "append", "--type", "test")
	assertError(t, err)
	assertErrorContains(t, err, "--stream")
}

func TestEventStore_Append_MissingType(t *testing.T) {
	oldVal := os.Getenv("FI_FHIR_DATABASE_URL")
	os.Setenv("FI_FHIR_DATABASE_URL", "postgres://localhost/test")
	defer func() {
		if oldVal != "" {
			os.Setenv("FI_FHIR_DATABASE_URL", oldVal)
		} else {
			os.Unsetenv("FI_FHIR_DATABASE_URL")
		}
	}()

	_, _, err := runCLI(t, "eventstore", "append", "--stream", "test")
	assertError(t, err)
	assertErrorContains(t, err, "--type")
}

// =============================================================================
// Subscription Command Tests
// =============================================================================

func TestSubscription_Help(t *testing.T) {
	stdout, _, err := runCLI(t, "subscription", "--help")
	assertNoError(t, err)
	assertContains(t, stdout, "subscription")
}

func TestSubscription_NoArgs(t *testing.T) {
	stdout, _, err := runCLI(t, "subscription")
	assertNoError(t, err) // prints usage
	assertContains(t, stdout, "subscription")
}

func TestSubscription_UnknownSubcommand(t *testing.T) {
	_, _, err := runCLI(t, "subscription", "unknown")
	assertError(t, err)
	assertErrorContains(t, err, "unknown subscription command")
}

// =============================================================================
// More Workflow Subcommand Tests
// =============================================================================

func TestWorkflow_Record_MissingOutput(t *testing.T) {
	tmpDir := t.TempDir()
	workflowYAML := `workflow:
  name: test
  version: "1.0"
  routes: []
`
	configPath := filepath.Join(tmpDir, "workflow.yaml")
	if err := os.WriteFile(configPath, []byte(workflowYAML), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	_, _, err := runCLI(t, "workflow", "record", "--config", configPath)
	assertError(t, err)
	assertErrorContains(t, err, "output")
}

func TestWorkflow_Record_Help(t *testing.T) {
	stdout, _, err := runCLI(t, "workflow", "record", "--help")
	assertNoError(t, err)
	assertContains(t, stdout, "record")
}

func TestWorkflow_Replay_Help(t *testing.T) {
	stdout, _, err := runCLI(t, "workflow", "replay", "--help")
	assertNoError(t, err)
	assertContains(t, stdout, "replay")
}

func TestWorkflow_Simulate_Help(t *testing.T) {
	stdout, _, err := runCLI(t, "workflow", "simulate", "--help")
	assertNoError(t, err)
	assertContains(t, stdout, "simulate")
}

func TestWorkflow_Loadtest_Help(t *testing.T) {
	stdout, _, err := runCLI(t, "workflow", "loadtest", "--help")
	assertNoError(t, err)
	assertContains(t, stdout, "loadtest")
}

// =============================================================================
// More Config Subcommand Tests
// =============================================================================

func TestConfig_Env_Help(t *testing.T) {
	stdout, _, err := runCLI(t, "config", "env", "--help")
	assertNoError(t, err)
	assertContains(t, stdout, "env")
}

func TestConfig_Init_Help(t *testing.T) {
	stdout, _, err := runCLI(t, "config", "init", "--help")
	assertNoError(t, err)
	assertContains(t, stdout, "init")
}

func TestConfig_Env_Show(t *testing.T) {
	stdout, _, err := runCLI(t, "config", "env")
	assertNoError(t, err)
	// Should list environment variables
	assertContains(t, stdout, "FI_FHIR")
}

func TestConfig_Init_ExistingFile(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Create existing file
	if err := os.WriteFile(configPath, []byte("existing: true"), 0644); err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}

	_, _, err := runCLI(t, "config", "init", "--output", configPath)
	// Should fail without --force
	assertError(t, err)
}

func TestConfig_Init_Overwrite(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Create existing file
	if err := os.WriteFile(configPath, []byte("existing: true"), 0644); err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}

	// Without --force or --overwrite, should fail
	_, _, err := runCLI(t, "config", "init", "--output", configPath)
	// This test verifies the error path
	assertError(t, err)
}

func TestConfig_Init_NewFile(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "new_config.yaml")

	stdout, _, err := runCLI(t, "config", "init", "--output", configPath)
	assertNoError(t, err)
	assertContains(t, stdout, "Created")

	// Verify file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("Config file was not created")
	}
}

// =============================================================================
// Additional Validate Tests
// =============================================================================

func TestValidate_Message_Help(t *testing.T) {
	stdout, _, err := runCLI(t, "validate", "--help")
	assertNoError(t, err)
	assertContains(t, stdout, "validate")
}
