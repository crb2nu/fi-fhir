package fhir

import "testing"

func TestValidateJSON_PatientMissingRequiredFields(t *testing.T) {
	data := []byte(`{
  "resourceType": "Patient",
  "identifier": [],
  "name": [],
  "gender": "",
  "birthDate": ""
}`)

	outcome, err := ValidateJSON(data, ValidationOptions{Mode: "us-core"})
	if err != nil {
		t.Fatalf("ValidateJSON: %v", err)
	}
	if outcome == nil {
		t.Fatalf("expected outcome")
	}
	if len(outcome.Issue) == 0 {
		t.Fatalf("expected issues")
	}

	hasError := false
	for _, iss := range outcome.Issue {
		if iss.Severity == "error" {
			hasError = true
			break
		}
	}
	if !hasError {
		t.Fatalf("expected at least one error issue")
	}
}

func TestValidateJSON_PatientOKWithUSCoreWarningForMissingMetaProfile(t *testing.T) {
	data := []byte(`{
  "resourceType": "Patient",
  "identifier": [{"system":"http://example.org","value":"123"}],
  "name": [{"family":"Doe"}],
  "gender": "male",
  "birthDate": "1980-01-01"
}`)

	outcome, err := ValidateJSON(data, ValidationOptions{Mode: "us-core"})
	if err != nil {
		t.Fatalf("ValidateJSON: %v", err)
	}
	if outcome == nil {
		t.Fatalf("expected outcome")
	}

	for _, iss := range outcome.Issue {
		if iss.Severity == "error" {
			t.Fatalf("unexpected error issue: %+v", iss)
		}
	}
}

func TestValidateJSON_ModeNone_IsStructuralOnly(t *testing.T) {
	data := []byte(`{
  "resourceType": "Patient",
  "identifier": [],
  "name": [],
  "gender": "",
  "birthDate": ""
}`)

	outcome, err := ValidateJSON(data, ValidationOptions{Mode: "none"})
	if err != nil {
		t.Fatalf("ValidateJSON: %v", err)
	}
	if outcome == nil {
		t.Fatalf("expected outcome")
	}
	if len(outcome.Issue) != 0 {
		t.Fatalf("expected no issues in mode=none, got %d", len(outcome.Issue))
	}
}
