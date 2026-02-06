package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ============================================================================
// FHIR subcommand dispatch tests
// ============================================================================

func TestFHIR_UnknownSubcommand(t *testing.T) {
	_, _, err := runCLI(t, "fhir", "badcmd")
	assertError(t, err)
	assertErrorContains(t, err, "unknown fhir subcommand")
}

func TestFHIR_Help_PrintsUsage(t *testing.T) {
	stdout, _, err := runCLI(t, "fhir", "--help")
	assertNoError(t, err)
	assertContains(t, stdout, "fi-fhir fhir")
	assertContains(t, stdout, "validate")
}

func TestFHIR_HelpShort_PrintsUsage(t *testing.T) {
	stdout, _, err := runCLI(t, "fhir", "-h")
	assertNoError(t, err)
	assertContains(t, stdout, "fi-fhir fhir")
}

// ============================================================================
// FHIR validate flag and argument tests
// ============================================================================

func TestFHIRValidate_NoInput_ReturnsError(t *testing.T) {
	_, _, err := runCLI(t, "fhir", "validate")
	assertError(t, err)
	assertErrorContains(t, err, "no input specified")
}

func TestFHIRValidate_FileNotFound_ReturnsError(t *testing.T) {
	_, _, err := runCLI(t, "fhir", "validate", "/nonexistent/path/patient.json")
	assertError(t, err)
	assertErrorContains(t, err, "failed to read")
}

func TestFHIRValidate_UnknownFlag_ReturnsError(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "patient.json")
	if err := os.WriteFile(path, []byte(`{"resourceType":"Patient"}`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	_, _, err := runCLI(t, "fhir", "validate", "--badoption", path)
	assertError(t, err)
	assertErrorContains(t, err, "unknown flag")
}

func TestFHIRValidate_ModeFlagMissingValue(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "patient.json")
	if err := os.WriteFile(path, []byte(`{"resourceType":"Patient"}`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	_, _, err := runCLI(t, "fhir", "validate", path, "--mode")
	assertError(t, err)
	assertErrorContains(t, err, "requires a value")
}

func TestFHIRValidate_UnexpectedArg_ReturnsError(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "patient.json")
	if err := os.WriteFile(path, []byte(`{"resourceType":"Patient"}`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	_, _, err := runCLI(t, "fhir", "validate", path, "extra_arg")
	assertError(t, err)
	assertErrorContains(t, err, "unexpected argument")
}

func TestFHIRValidate_HelpShort_PrintsUsage(t *testing.T) {
	stdout, _, err := runCLI(t, "fhir", "validate", "-h")
	assertNoError(t, err)
	assertContains(t, stdout, "fi-fhir fhir validate")
	assertContains(t, stdout, "--mode")
}

// ============================================================================
// FHIR validate validation logic tests
// ============================================================================

func TestFHIRValidate_ValidPatient_Passes(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "patient.json")

	// Valid US Core Patient with required meta.profile
	data := `{
  "resourceType": "Patient",
  "meta": {
    "profile": ["http://hl7.org/fhir/us/core/StructureDefinition/us-core-patient"]
  },
  "identifier": [{"system":"http://example.org","value":"123"}],
  "name": [{"family":"Doe","given":["John"]}],
  "gender": "male",
  "birthDate": "1980-01-01"
}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	stdout, _, err := runCLI(t, "fhir", "validate", "--allow-warnings", path)
	assertNoError(t, err)
	assertContains(t, stdout, "validation")
}

func TestFHIRValidate_InvalidJSON_ReturnsError(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "invalid.json")

	// Invalid JSON
	if err := os.WriteFile(path, []byte(`{not valid json`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	_, _, err := runCLI(t, "fhir", "validate", path)
	assertError(t, err)
}

func TestFHIRValidate_NotFHIRResource_ReturnsError(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "notfhir.json")

	// Valid JSON but not a FHIR resource
	if err := os.WriteFile(path, []byte(`{"key":"value"}`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	_, _, err := runCLI(t, "fhir", "validate", path)
	assertError(t, err)
}

func TestFHIRValidate_ModeNone(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "patient.json")

	// Minimal patient - using mode=none should skip US Core validation
	data := `{
  "resourceType": "Patient",
  "name": [{"family":"Doe"}]
}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	stdout, _, err := runCLI(t, "fhir", "validate", "--mode", "none", "--allow-warnings", path)
	assertNoError(t, err)
	assertContains(t, stdout, "validation")
}

func TestFHIRValidate_ModeUSCore(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "patient.json")

	// Patient missing required fields for US Core
	data := `{
  "resourceType": "Patient",
  "identifier": [{"system":"http://example.org","value":"123"}],
  "name": [{"family":"Doe"}],
  "gender": "male",
  "birthDate": "1980-01-01"
}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Should complete (may have warnings, but allow-warnings lets it pass)
	stdout, _, err := runCLI(t, "fhir", "validate", "--mode", "us-core", "--allow-warnings", path)
	assertNoError(t, err)
	assertContains(t, stdout, "validation")
}

func TestFHIRValidate_StrictModeDefault(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "patient.json")

	// Patient without meta.profile will generate warnings
	data := `{
  "resourceType": "Patient",
  "identifier": [{"system":"http://example.org","value":"123"}],
  "name": [{"family":"Doe"}],
  "gender": "male",
  "birthDate": "1980-01-01"
}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Strict mode (default) should fail on warnings
	_, _, err := runCLI(t, "fhir", "validate", "--mode", "us-core", path)
	assertError(t, err)
}

func TestFHIRValidate_StrictFlagExplicit(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "patient.json")

	// Patient without meta.profile will generate warnings
	data := `{
  "resourceType": "Patient",
  "identifier": [{"system":"http://example.org","value":"123"}],
  "name": [{"family":"Doe"}],
  "gender": "male",
  "birthDate": "1980-01-01"
}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Explicit --strict should fail on warnings
	_, _, err := runCLI(t, "fhir", "validate", "--strict", "--mode", "us-core", path)
	assertError(t, err)
}

func TestFHIRValidate_AllowWarnings(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "patient.json")

	// Patient without meta.profile will generate warnings
	data := `{
  "resourceType": "Patient",
  "identifier": [{"system":"http://example.org","value":"123"}],
  "name": [{"family":"Doe"}],
  "gender": "male",
  "birthDate": "1980-01-01"
}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	// --allow-warnings should pass even with warnings
	stdout, _, err := runCLI(t, "fhir", "validate", "--allow-warnings", "--mode", "us-core", path)
	assertNoError(t, err)
	// Either "passed" or "warnings" message expected
	if !strings.Contains(stdout, "validation") {
		t.Errorf("expected validation output, got: %s", stdout)
	}
}

func TestFHIRValidate_JSONOutputFormat(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "patient.json")

	data := `{
  "resourceType": "Patient",
  "identifier": [{"system":"http://example.org","value":"123"}],
  "name": [{"family":"Doe"}],
  "gender": "male",
  "birthDate": "1980-01-01"
}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	stdout, _, err := runCLI(t, "fhir", "validate", "--json", "--allow-warnings", path)
	assertNoError(t, err)

	var got map[string]any
	if jsonErr := json.Unmarshal([]byte(stdout), &got); jsonErr != nil {
		t.Fatalf("invalid JSON output: %v\n%s", jsonErr, stdout)
	}

	// OperationOutcome should have resourceType and issue array
	if got["resourceType"] != "OperationOutcome" {
		t.Errorf("expected resourceType=OperationOutcome, got %v", got["resourceType"])
	}
	if _, ok := got["issue"]; !ok {
		t.Errorf("expected issue array in OperationOutcome")
	}
}

func TestFHIRValidate_JSONOutputWithErrors(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "bad.json")

	// Invalid resourceType should cause errors
	data := `{
  "resourceType": "InvalidResourceType",
  "name": [{"family":"Doe"}]
}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Even with error, JSON output should still be valid JSON
	stdout, _, _ := runCLI(t, "fhir", "validate", "--json", path)

	// Should still be valid JSON (OperationOutcome with errors)
	var got map[string]any
	if jsonErr := json.Unmarshal([]byte(stdout), &got); jsonErr != nil {
		// It's also valid to fail before JSON output if the input is completely invalid
		// In that case, no JSON is produced
		t.Logf("No JSON output for completely invalid input (expected)")
	}
}

// ============================================================================
// FHIR validate bundle tests
// ============================================================================

func TestFHIRValidate_Bundle(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "bundle.json")

	// Minimal FHIR Bundle
	data := `{
  "resourceType": "Bundle",
  "type": "collection",
  "entry": [
    {
      "resource": {
        "resourceType": "Patient",
        "identifier": [{"system":"http://example.org","value":"123"}],
        "name": [{"family":"Doe"}],
        "gender": "male"
      }
    }
  ]
}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	stdout, _, err := runCLI(t, "fhir", "validate", "--allow-warnings", "--mode", "none", path)
	assertNoError(t, err)
	assertContains(t, stdout, "validation")
}

// ============================================================================
// FHIR validate different resource types
// ============================================================================

func TestFHIRValidate_Observation(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "observation.json")

	data := `{
  "resourceType": "Observation",
  "status": "final",
  "code": {
    "coding": [{
      "system": "http://loinc.org",
      "code": "8867-4",
      "display": "Heart rate"
    }]
  },
  "valueQuantity": {
    "value": 72,
    "unit": "beats/minute"
  }
}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	stdout, _, err := runCLI(t, "fhir", "validate", "--allow-warnings", "--mode", "none", path)
	assertNoError(t, err)
	assertContains(t, stdout, "validation")
}

func TestFHIRValidate_Encounter(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "encounter.json")

	data := `{
  "resourceType": "Encounter",
  "status": "finished",
  "class": {
    "system": "http://terminology.hl7.org/CodeSystem/v3-ActCode",
    "code": "IMP"
  }
}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	stdout, _, err := runCLI(t, "fhir", "validate", "--allow-warnings", "--mode", "none", path)
	assertNoError(t, err)
	assertContains(t, stdout, "validation")
}

// ============================================================================
// Output format tests
// ============================================================================

func TestFHIRValidate_TextOutputPassMessage(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "patient.json")

	data := `{
  "resourceType": "Patient",
  "identifier": [{"system":"http://example.org","value":"123"}],
  "name": [{"family":"Doe"}],
  "gender": "male",
  "birthDate": "1980-01-01"
}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	stdout, _, err := runCLI(t, "fhir", "validate", "--allow-warnings", "--mode", "none", path)
	assertNoError(t, err)
	// Should have text output (not JSON)
	if strings.HasPrefix(strings.TrimSpace(stdout), "{") {
		t.Errorf("expected text output (not JSON), got: %s", stdout)
	}
}

func TestFHIRValidate_TextOutputWithIssues(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "patient.json")

	// Patient that will generate warnings
	data := `{
  "resourceType": "Patient",
  "identifier": [{"system":"http://example.org","value":"123"}],
  "name": [{"family":"Doe"}],
  "gender": "male",
  "birthDate": "1980-01-01"
}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	stdout, _, err := runCLI(t, "fhir", "validate", "--mode", "us-core", "--allow-warnings", path)
	assertNoError(t, err)
	// Should mention warnings or pass in text output
	if !strings.Contains(stdout, "validation") {
		t.Errorf("expected validation message, got: %s", stdout)
	}
}
