package fhirout

import (
	"encoding/json"
	"strings"
	"testing"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/fhir"
)

func TestWorkflowTransactionReferencesAndSelection(t *testing.T) {
	admit := projectTestAdmit()
	for _, selected := range []string{"", "Patient", "Encounter"} {
		t.Run(selected, func(t *testing.T) {
			projection, err := ProjectWorkflow(admit, selected)
			if err != nil {
				t.Fatal(err)
			}
			bundle, err := CreateWorkflowTransactionBundle(projection)
			if err != nil {
				t.Fatal(err)
			}
			wantCount := 1
			if selected == "" {
				wantCount = 2
			}
			if len(bundle.Entry) != wantCount {
				t.Fatalf("entries = %d", len(bundle.Entry))
			}
			for _, entry := range bundle.Entry {
				if entry.FullURL == "" || entry.Request.Method != "POST" {
					t.Fatalf("entry = %+v", entry)
				}
				if selected != "" && entry.Request.URL != selected {
					t.Fatalf("wrong resource: %+v", entry.Request)
				}
				if entry.Request.URL == "Encounter" {
					var body map[string]any
					if err := json.Unmarshal(entry.Resource, &body); err != nil {
						t.Fatal(err)
					}
					refs := resourceReferences(body)
					want := bundle.Entry[0].FullURL
					if selected == "Encounter" {
						want = ConditionalReference("Patient", fhir.Identifier{System: "urn:oid:1.2.3", Value: admit.Patient.MRN})
					}
					if len(refs) != 1 || refs[0] != want {
						t.Fatalf("refs = %v, want %s", refs, want)
					}
				}
			}
		})
	}
	if _, err := ProjectWorkflow(admit, "Observation"); err == nil {
		t.Fatal("unknown selection accepted")
	}
	// Selecting Patient must neither emit nor demand identifiers from Encounter.
	admit.Encounter = events.Encounter{}
	projection, err := ProjectWorkflow(admit, "Patient")
	if err != nil {
		t.Fatal(err)
	}
	patient, err := WorkflowPatient(projection)
	if err != nil || patient.GetResourceType() != "Patient" {
		t.Fatalf("Patient = %v, %v", patient, err)
	}
}

func TestWorkflowLabReferencesAndOmittedDependencies(t *testing.T) {
	event := projectTestEventFor(t, events.EventLabResult)
	projection, err := ProjectWorkflow(event, "")
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := CreateWorkflowTransactionBundle(projection)
	if err != nil {
		t.Fatal(err)
	}
	var report map[string]any
	if err := json.Unmarshal(bundle.Entry[0].Resource, &report); err != nil {
		t.Fatal(err)
	}
	var foundObservation bool
	for _, ref := range resourceReferences(report) {
		if ref == bundle.Entry[1].FullURL {
			foundObservation = true
			continue
		}
		if !strings.HasPrefix(ref, "Patient?identifier=") {
			t.Fatalf("unresolved reference %s", ref)
		}
	}
	if !foundObservation {
		t.Fatal("report does not reference its Observation entry")
	}
	if _, err := ProjectWorkflow(event, "DiagnosticReport"); err == nil {
		t.Fatal("selection accepted dangling report results")
	}
	if _, err := ProjectWorkflow(event, "Observation"); err != nil {
		t.Fatal(err)
	}
}

func TestWorkflowClinicalBundleReferences(t *testing.T) {
	for _, event := range clinicalTestEvents(t) {
		t.Run(event["type"].(string), func(t *testing.T) {
			projection, err := ProjectWorkflow(event, "")
			if err != nil {
				t.Fatal(err)
			}
			bundle, err := CreateWorkflowTransactionBundle(projection)
			if err != nil {
				t.Fatal(err)
			}
			if len(bundle.Entry) != 1 {
				t.Fatalf("unexpected demographic overwrite: %+v", bundle.Entry)
			}
			var body map[string]any
			if err := json.Unmarshal(bundle.Entry[0].Resource, &body); err != nil {
				t.Fatal(err)
			}
			refs := resourceReferences(body)
			if len(refs) == 0 {
				t.Fatal("clinical resource lost patient reference")
			}
			for _, ref := range refs {
				if !strings.Contains(ref, "?identifier=") {
					t.Fatalf("unresolved reference %s", ref)
				}
			}
		})
	}
}
