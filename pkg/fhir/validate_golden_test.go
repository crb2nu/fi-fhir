package fhir

import (
	"os"
	"path/filepath"
	"testing"
)

// TestValidateJSON_USCoreGoldenFixtures exercises the curated, hand-written
// fixture set.
//
// Hand-written fixtures test the checker against a third artefact, never against
// the mapper. That is how Slice 5.1a's day-1 gate stayed invisible: the only
// DiagnosticReport fixture here was `-note`, testdata/fhir/ held no lab
// DiagnosticReport, and the one input MapLabResult actually produces was never
// validated in CI (correction 41). The gap is closed twice over — by
// `diagnosticreport_uscore_lab.json` below, and by the generated set under
// testdata/fhir/mapper/, which TestFHIRConformance_MapperGoldenFixtures holds
// byte-equal to the mapper's current output.
func TestValidateJSON_USCoreGoldenFixtures(t *testing.T) {
	fixtures := []struct {
		name string
		path string
	}{
		{name: "patient", path: filepath.Join("..", "..", "testdata", "fhir", "patient_uscore.json")},
		{name: "encounter", path: filepath.Join("..", "..", "testdata", "fhir", "encounter_uscore.json")},
		{name: "observation", path: filepath.Join("..", "..", "testdata", "fhir", "observation_uscore_lab.json")},
		{name: "diagnosticreport_note", path: filepath.Join("..", "..", "testdata", "fhir", "diagnosticreport_uscore_note.json")},
		// The input MapLabResult actually produces. Generated from the mapper;
		// regenerate with `go test ./pkg/fhir -run MapperGoldenFixtures -update-fhir-golden`
		// and copy testdata/fhir/mapper/labresult_1.json over it.
		{name: "diagnosticreport_lab", path: filepath.Join("..", "..", "testdata", "fhir", "diagnosticreport_uscore_lab.json")},
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
