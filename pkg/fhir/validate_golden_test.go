package fhir

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateJSON_USCoreGoldenFixtures(t *testing.T) {
	fixtures := []struct {
		name string
		path string
	}{
		{name: "patient", path: filepath.Join("..", "..", "testdata", "fhir", "patient_uscore.json")},
		{name: "encounter", path: filepath.Join("..", "..", "testdata", "fhir", "encounter_uscore.json")},
		{name: "observation", path: filepath.Join("..", "..", "testdata", "fhir", "observation_uscore_lab.json")},
		{name: "diagnosticreport", path: filepath.Join("..", "..", "testdata", "fhir", "diagnosticreport_uscore_note.json")},
		{name: "bundle", path: filepath.Join("..", "..", "testdata", "fhir", "bundle_transaction_uscore.json")},
	}

	for _, tc := range fixtures {
		t.Run(tc.name, func(t *testing.T) {
			data, err := os.ReadFile(tc.path)
			if err != nil {
				t.Fatalf("read fixture %s: %v", tc.path, err)
			}

			outcome, err := ValidateJSON(data, ValidationOptions{Mode: "us-core"})
			if err != nil {
				t.Fatalf("ValidateJSON: %v", err)
			}
			if outcome == nil {
				t.Fatalf("expected outcome")
			}
			if len(outcome.Issue) != 0 {
				t.Fatalf("expected no issues, got %d", len(outcome.Issue))
			}
		})
	}
}
