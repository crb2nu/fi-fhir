package fhirout

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/fhir"
)

func clinicalTestEvents(t *testing.T) []map[string]any {
	t.Helper()
	raw, err := os.ReadFile("../../../testdata/fhir/clinical-events.json")
	if err != nil {
		t.Fatal(err)
	}
	var events []map[string]any
	if err := json.Unmarshal(raw, &events); err != nil {
		t.Fatal(err)
	}
	return events
}

func TestClinicalProjections(t *testing.T) {
	set, err := fhir.LoadPinnedPackages("../../../testdata/fhir/packages")
	if err != nil {
		t.Fatal(err)
	}
	wantTypes := map[string]string{
		"condition": "Condition", "procedure": "Procedure", "immunization": "Immunization",
		"vital_sign": "Observation", "medication_request": "MedicationRequest", "allergy_intolerance": "AllergyIntolerance",
	}
	for _, event := range clinicalTestEvents(t) {
		t.Run(event["type"].(string), func(t *testing.T) {
			before, _ := json.Marshal(event)
			eventType := events.EventType(event["type"].(string))
			typed := supportedEventTypes[eventType]()
			if err := json.Unmarshal(before, typed); err != nil {
				t.Fatal(err)
			}
			var original []byte
			for _, input := range []any{event, typed, reflect.ValueOf(typed).Elem().Interface()} {
				projection, err := ProjectEvent(input)
				if err != nil {
					t.Fatal(err)
				}
				if len(projection.Resources) != 1 || projection.Resources[0].Type != wantTypes[string(eventType)] {
					t.Fatalf("resources = %+v", projection.Resources)
				}
				bundle, err := CreateConditionalTransactionBundle(projection)
				if err != nil {
					t.Fatal(err)
				}
				encoded, _ := json.Marshal(bundle)
				if original == nil {
					original = encoded
				} else if !bytes.Equal(original, encoded) {
					t.Fatalf("typed/value/JSON delivery differs:\n%s\n%s", original, encoded)
				}
				if bundle.Entry[0].Request.Method != "PUT" || !strings.Contains(bundle.Entry[0].Request.URL, "identifier=") {
					t.Fatalf("not a conditional update: %+v", bundle.Entry[0].Request)
				}
				var body map[string]any
				if err := json.Unmarshal(bundle.Entry[0].Resource, &body); err != nil {
					t.Fatal(err)
				}
				if _, present := body["id"]; present {
					t.Fatal("client ID leaked into conditional update")
				}
				for _, reference := range resourceReferences(body) {
					if !strings.HasPrefix(reference, "Patient?identifier=") && !strings.HasPrefix(reference, "Encounter?identifier=") {
						t.Fatalf("unresolved clinical reference %q", reference)
					}
				}
				outcome, err := fhir.ValidateStructuralJSON(encoded, set, fhir.StructuralOptions{})
				if err != nil || len(outcome.Issue) != 0 {
					t.Fatalf("structural validation: %v %+v", err, outcome)
				}
			}
			// The stored-payload decoder and direct projector must agree too.
			projection, err := Project(eventType, projectTestPayload(t, typed))
			if err != nil {
				t.Fatal(err)
			}
			bundle, err := CreateConditionalTransactionBundle(projection)
			if err != nil {
				t.Fatal(err)
			}
			encoded, _ := json.Marshal(bundle)
			if !bytes.Equal(original, encoded) {
				t.Fatal("stored projection differs from typed event")
			}
			after, _ := json.Marshal(event)
			if !bytes.Equal(before, after) {
				t.Fatal("projection modified canonical event")
			}
		})
	}
}

func TestClinicalProjectionIdentityAndMissingData(t *testing.T) {
	for _, event := range clinicalTestEvents(t) {
		t.Run(event["type"].(string), func(t *testing.T) {
			first, err := ProjectEvent(event)
			if err != nil {
				t.Fatal(err)
			}
			for _, field := range []string{"id", "source"} {
				original := event[field]
				event[field] = original.(string) + "-second"
				second, err := ProjectEvent(event)
				if err != nil {
					t.Fatal(err)
				}
				if first.Resources[0].FullURL == second.Resources[0].FullURL {
					t.Fatalf("different %s overwrites first record", field)
				}
				event[field] = ""
				if _, err := ProjectEvent(event); !errors.Is(err, ErrNoUsableIdentifier) {
					t.Fatalf("missing %s: %v", field, err)
				}
				event[field] = original
			}
			event["patient"] = nil
			if _, err := ProjectEvent(event); !errors.Is(err, ErrInvalidPayload) {
				t.Fatalf("nil patient: %v", err)
			}
			event["patient"] = map[string]any{}
			if _, err := ProjectEvent(event); !errors.Is(err, ErrNoUsableIdentifier) {
				t.Fatalf("no patient identifier: %v", err)
			}
			typedNil := reflect.Zero(reflect.TypeOf(supportedEventTypes[events.EventType(event["type"].(string))]())).Interface()
			if _, err := ProjectEvent(typedNil); !errors.Is(err, ErrInvalidPayload) {
				t.Fatalf("nil event: %v", err)
			}
		})
	}
}
