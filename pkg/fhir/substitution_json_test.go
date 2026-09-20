package fhir

import (
	"encoding/json"
	"testing"
)

func TestMedicationSubstitutionJSONChoice(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value MedSubstitution
		field string
	}{
		{"not allowed", MedSubstitution{AllowedBoolean: false}, "allowedBoolean"},
		{"allowed", MedSubstitution{AllowedBoolean: true}, "allowedBoolean"},
		{"coded", MedSubstitution{AllowedCodeableConcept: &CodeableConcept{Text: "generic only"}}, "allowedCodeableConcept"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := json.Marshal(tc.value)
			if err != nil {
				t.Fatal(err)
			}
			var body map[string]any
			if err := json.Unmarshal(raw, &body); err != nil {
				t.Fatal(err)
			}
			if len(body) != 1 || body[tc.field] == nil {
				t.Fatalf("allowed[x] choice: %s", raw)
			}
			if tc.field == "allowedBoolean" && body[tc.field] != tc.value.AllowedBoolean {
				t.Fatalf("boolean changed: %s", raw)
			}
		})
	}
}
