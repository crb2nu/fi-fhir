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

	stdout, _, err := runCLI(t, "fhir", "validate", "--mode", "us-core", "--json", path)
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
