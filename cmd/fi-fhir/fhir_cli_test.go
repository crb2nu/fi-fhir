package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestFHIRValidate_JSONOutput(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "patient.json")

	// Intentionally omit meta.profile to ensure we still pass (warnings only).
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

	stdout, _, err := runCLI(t, "fhir", "validate", "--mode", "us-core", "--allow-warnings", "--json", path)
	if err != nil {
		t.Fatalf("fhir validate: %v", err)
	}

	var got map[string]any
	if jsonErr := json.Unmarshal([]byte(stdout), &got); jsonErr != nil {
		t.Fatalf("invalid JSON output: %v\n%s", jsonErr, stdout)
	}
	if rt, _ := got["resourceType"].(string); rt != "OperationOutcome" {
		t.Fatalf("resourceType=%v, want OperationOutcome", got["resourceType"])
	}
}

func TestFHIR_NoArgs_PrintsUsage(t *testing.T) {
	stdout, _, err := runCLI(t, "fhir")
	assertNoError(t, err)
	assertContains(t, stdout, "fi-fhir fhir")
	assertContains(t, stdout, "validate")
}

func TestFHIRValidate_Help_PrintsUsage(t *testing.T) {
	stdout, _, err := runCLI(t, "fhir", "validate", "--help")
	assertNoError(t, err)
	assertContains(t, stdout, "fi-fhir fhir validate")
	assertContains(t, stdout, "--mode")
}
