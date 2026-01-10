package fhir

import (
	"testing"
	"time"

	"github.com/cblevins/fi-fhir/pkg/events"
)

func TestParserParsePatient(t *testing.T) {
	parser := NewParser("test-source")

	patientJSON := `{
		"resourceType": "Patient",
		"id": "12345",
		"name": [
			{
				"family": "Smith",
				"given": ["John", "William"]
			}
		],
		"gender": "male",
		"birthDate": "1985-03-15"
	}`

	result, err := parser.ParseWithResult([]byte(patientJSON))
	if err != nil {
		t.Fatalf("ParseWithResult() error = %v", err)
	}

	if len(result.Events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(result.Events))
	}

	docEvent, ok := result.Events[0].(*events.DocumentEvent)
	if !ok {
		t.Fatalf("Expected *events.DocumentEvent, got %T", result.Events[0])
	}

	if docEvent.DocumentType != "Patient" {
		t.Errorf("DocumentType = %q, want %q", docEvent.DocumentType, "Patient")
	}

	if docEvent.Patient == nil {
		t.Fatal("Patient is nil")
	}

	if docEvent.Patient.FamilyName != "Smith" {
		t.Errorf("FamilyName = %q, want %q", docEvent.Patient.FamilyName, "Smith")
	}

	if docEvent.Patient.GivenName != "John" {
		t.Errorf("GivenName = %q, want %q", docEvent.Patient.GivenName, "John")
	}

	if docEvent.Patient.Gender != "male" {
		t.Errorf("Gender = %q, want %q", docEvent.Patient.Gender, "male")
	}

	expectedDOB := time.Date(1985, 3, 15, 0, 0, 0, 0, time.UTC)
	if !docEvent.Patient.DateOfBirth.Equal(expectedDOB) {
		t.Errorf("DateOfBirth = %v, want %v", docEvent.Patient.DateOfBirth, expectedDOB)
	}
}

func TestParserParseLabObservation(t *testing.T) {
	parser := NewParser("lab-source")

	observationJSON := `{
		"resourceType": "Observation",
		"id": "obs-123",
		"status": "final",
		"category": [
			{
				"coding": [
					{"code": "laboratory", "system": "http://terminology.hl7.org/CodeSystem/observation-category"}
				]
			}
		],
		"code": {
			"coding": [
				{"code": "2093-3", "display": "Cholesterol", "system": "http://loinc.org"}
			],
			"text": "Total Cholesterol"
		},
		"subject": {"reference": "Patient/12345"},
		"effectiveDateTime": "2025-01-10T10:30:00Z",
		"valueQuantity": {
			"value": 180,
			"unit": "mg/dL"
		}
	}`

	result, err := parser.ParseWithResult([]byte(observationJSON))
	if err != nil {
		t.Fatalf("ParseWithResult() error = %v", err)
	}

	if len(result.Events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(result.Events))
	}

	labEvent, ok := result.Events[0].(*events.LabResultEvent)
	if !ok {
		t.Fatalf("Expected *events.LabResultEvent, got %T", result.Events[0])
	}

	if labEvent.Type != events.EventLabResult {
		t.Errorf("Type = %v, want %v", labEvent.Type, events.EventLabResult)
	}

	if labEvent.Test.LOINCCode != "2093-3" {
		t.Errorf("LOINCCode = %q, want %q", labEvent.Test.LOINCCode, "2093-3")
	}

	if labEvent.Test.Description != "Total Cholesterol" {
		t.Errorf("Description = %q, want %q", labEvent.Test.Description, "Total Cholesterol")
	}

	if labEvent.Result.Value != "180" {
		t.Errorf("Value = %q, want %q", labEvent.Result.Value, "180")
	}

	if labEvent.Result.Unit != "mg/dL" {
		t.Errorf("Unit = %q, want %q", labEvent.Result.Unit, "mg/dL")
	}

	if labEvent.Patient.MRN != "12345" {
		t.Errorf("Patient.MRN = %q, want %q", labEvent.Patient.MRN, "12345")
	}
}

func TestParserParseVitalSignObservation(t *testing.T) {
	parser := NewParser("vitals-source")

	observationJSON := `{
		"resourceType": "Observation",
		"id": "bp-123",
		"status": "final",
		"category": [
			{
				"coding": [
					{"code": "vital-signs", "system": "http://terminology.hl7.org/CodeSystem/observation-category"}
				]
			}
		],
		"code": {
			"coding": [
				{"code": "85354-9", "display": "Blood pressure panel", "system": "http://loinc.org"}
			]
		},
		"subject": {"reference": "Patient/54321"},
		"effectiveDateTime": "2025-01-10T09:00:00Z",
		"valueQuantity": {
			"value": 120,
			"unit": "mmHg"
		}
	}`

	result, err := parser.ParseWithResult([]byte(observationJSON))
	if err != nil {
		t.Fatalf("ParseWithResult() error = %v", err)
	}

	if len(result.Events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(result.Events))
	}

	vitalEvent, ok := result.Events[0].(*events.VitalSignEvent)
	if !ok {
		t.Fatalf("Expected *events.VitalSignEvent (for vital-signs category), got %T", result.Events[0])
	}

	if vitalEvent.Type != events.EventVitalSign {
		t.Errorf("Type = %v, want %v", vitalEvent.Type, events.EventVitalSign)
	}

	if vitalEvent.VitalSign.LOINCCode != "85354-9" {
		t.Errorf("LOINCCode = %q, want %q", vitalEvent.VitalSign.LOINCCode, "85354-9")
	}

	if vitalEvent.VitalSign.Value != "120" {
		t.Errorf("Value = %q, want %q", vitalEvent.VitalSign.Value, "120")
	}
}

func TestParserParseCondition(t *testing.T) {
	parser := NewParser("ehr-source")

	conditionJSON := `{
		"resourceType": "Condition",
		"id": "cond-456",
		"clinicalStatus": {
			"coding": [
				{"code": "active"}
			]
		},
		"code": {
			"coding": [
				{"code": "E11.9", "display": "Type 2 diabetes mellitus", "system": "http://hl7.org/fhir/sid/icd-10-cm"}
			],
			"text": "Diabetes Type 2"
		},
		"subject": {"reference": "Patient/12345"},
		"recordedDate": "2024-06-15"
	}`

	result, err := parser.ParseWithResult([]byte(conditionJSON))
	if err != nil {
		t.Fatalf("ParseWithResult() error = %v", err)
	}

	if len(result.Events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(result.Events))
	}

	condEvent, ok := result.Events[0].(*events.ConditionEvent)
	if !ok {
		t.Fatalf("Expected *events.ConditionEvent, got %T", result.Events[0])
	}

	if condEvent.Condition.Code != "E11.9" {
		t.Errorf("Code = %q, want %q", condEvent.Condition.Code, "E11.9")
	}

	if condEvent.Condition.Name != "Diabetes Type 2" {
		t.Errorf("Name = %q, want %q", condEvent.Condition.Name, "Diabetes Type 2")
	}

	if condEvent.ClinicalStatus != "active" {
		t.Errorf("ClinicalStatus = %q, want %q", condEvent.ClinicalStatus, "active")
	}
}

func TestParserParseEncounter(t *testing.T) {
	parser := NewParser("hospital-source")

	encounterJSON := `{
		"resourceType": "Encounter",
		"id": "enc-789",
		"status": "in-progress",
		"class": {
			"code": "IMP",
			"display": "inpatient encounter"
		},
		"subject": {"reference": "Patient/12345"},
		"period": {
			"start": "2025-01-08T14:00:00Z"
		}
	}`

	result, err := parser.ParseWithResult([]byte(encounterJSON))
	if err != nil {
		t.Fatalf("ParseWithResult() error = %v", err)
	}

	if len(result.Events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(result.Events))
	}

	admitEvent, ok := result.Events[0].(*events.PatientAdmitEvent)
	if !ok {
		t.Fatalf("Expected *events.PatientAdmitEvent, got %T", result.Events[0])
	}

	if admitEvent.Encounter.ID != "enc-789" {
		t.Errorf("Encounter.ID = %q, want %q", admitEvent.Encounter.ID, "enc-789")
	}

	if admitEvent.Encounter.Status != "in-progress" {
		t.Errorf("Status = %q, want %q", admitEvent.Encounter.Status, "in-progress")
	}

	if admitEvent.Encounter.Class != "IMP" {
		t.Errorf("Class = %q, want %q", admitEvent.Encounter.Class, "IMP")
	}
}

func TestParserParseProcedure(t *testing.T) {
	parser := NewParser("surgery-source")

	procedureJSON := `{
		"resourceType": "Procedure",
		"id": "proc-101",
		"status": "completed",
		"code": {
			"coding": [
				{"code": "80146002", "display": "Appendectomy", "system": "http://snomed.info/sct"}
			]
		},
		"subject": {"reference": "Patient/12345"},
		"performedDateTime": "2025-01-05T10:00:00Z"
	}`

	result, err := parser.ParseWithResult([]byte(procedureJSON))
	if err != nil {
		t.Fatalf("ParseWithResult() error = %v", err)
	}

	if len(result.Events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(result.Events))
	}

	procEvent, ok := result.Events[0].(*events.ProcedureEvent)
	if !ok {
		t.Fatalf("Expected *events.ProcedureEvent, got %T", result.Events[0])
	}

	if procEvent.Procedure.Code != "80146002" {
		t.Errorf("Code = %q, want %q", procEvent.Procedure.Code, "80146002")
	}

	if procEvent.Procedure.Name != "Appendectomy" {
		t.Errorf("Name = %q, want %q", procEvent.Procedure.Name, "Appendectomy")
	}

	if procEvent.Procedure.Status != "completed" {
		t.Errorf("Status = %q, want %q", procEvent.Procedure.Status, "completed")
	}
}

func TestParserParseImmunization(t *testing.T) {
	parser := NewParser("immunization-source")

	immunizationJSON := `{
		"resourceType": "Immunization",
		"id": "imm-202",
		"status": "completed",
		"vaccineCode": {
			"coding": [
				{"code": "140", "display": "Influenza vaccine", "system": "http://hl7.org/fhir/sid/cvx"}
			]
		},
		"patient": {"reference": "Patient/12345"},
		"occurrenceDateTime": "2025-01-02T09:00:00Z"
	}`

	result, err := parser.ParseWithResult([]byte(immunizationJSON))
	if err != nil {
		t.Fatalf("ParseWithResult() error = %v", err)
	}

	if len(result.Events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(result.Events))
	}

	immEvent, ok := result.Events[0].(*events.ImmunizationEvent)
	if !ok {
		t.Fatalf("Expected *events.ImmunizationEvent, got %T", result.Events[0])
	}

	if immEvent.Immunization.VaccineCode != "140" {
		t.Errorf("VaccineCode = %q, want %q", immEvent.Immunization.VaccineCode, "140")
	}

	if immEvent.Immunization.VaccineName != "Influenza vaccine" {
		t.Errorf("VaccineName = %q, want %q", immEvent.Immunization.VaccineName, "Influenza vaccine")
	}

	if immEvent.Immunization.Status != "completed" {
		t.Errorf("Status = %q, want %q", immEvent.Immunization.Status, "completed")
	}
}

func TestParserParseBundle(t *testing.T) {
	parser := NewParser("bundle-source")

	bundleJSON := `{
		"resourceType": "Bundle",
		"type": "searchset",
		"entry": [
			{
				"resource": {
					"resourceType": "Patient",
					"id": "p1",
					"name": [{"family": "Doe", "given": ["Jane"]}]
				}
			},
			{
				"resource": {
					"resourceType": "Condition",
					"id": "c1",
					"code": {"coding": [{"code": "J06.9", "display": "Acute upper respiratory infection"}]},
					"subject": {"reference": "Patient/p1"}
				}
			}
		]
	}`

	result, err := parser.ParseWithResult([]byte(bundleJSON))
	if err != nil {
		t.Fatalf("ParseWithResult() error = %v", err)
	}

	if len(result.Events) != 2 {
		t.Fatalf("Expected 2 events from bundle, got %d", len(result.Events))
	}

	// First should be Patient (as DocumentEvent)
	if _, ok := result.Events[0].(*events.DocumentEvent); !ok {
		t.Errorf("First event expected *events.DocumentEvent, got %T", result.Events[0])
	}

	// Second should be Condition
	if _, ok := result.Events[1].(*events.ConditionEvent); !ok {
		t.Errorf("Second event expected *events.ConditionEvent, got %T", result.Events[1])
	}
}

func TestParserParseEmptyBundle(t *testing.T) {
	parser := NewParser("test-source")

	bundleJSON := `{
		"resourceType": "Bundle",
		"type": "searchset"
	}`

	result, err := parser.ParseWithResult([]byte(bundleJSON))
	if err != nil {
		t.Fatalf("ParseWithResult() error = %v", err)
	}

	if len(result.Events) != 0 {
		t.Errorf("Expected 0 events from empty bundle, got %d", len(result.Events))
	}

	// Should have warning about empty bundle
	if len(result.Warnings) != 1 {
		t.Errorf("Expected 1 warning for empty bundle, got %d", len(result.Warnings))
	}

	if result.Warnings[0].Code != "EMPTY_BUNDLE" {
		t.Errorf("Warning code = %q, want %q", result.Warnings[0].Code, "EMPTY_BUNDLE")
	}
}

func TestParserParseUnknownResource(t *testing.T) {
	parser := NewParser("test-source")

	unknownJSON := `{
		"resourceType": "MedicationRequest",
		"id": "med-123",
		"status": "active"
	}`

	result, err := parser.ParseWithResult([]byte(unknownJSON))
	if err != nil {
		t.Fatalf("ParseWithResult() error = %v", err)
	}

	if len(result.Events) != 1 {
		t.Fatalf("Expected 1 event for unknown resource, got %d", len(result.Events))
	}

	// Unknown resources become DocumentEvents
	docEvent, ok := result.Events[0].(*events.DocumentEvent)
	if !ok {
		t.Fatalf("Expected *events.DocumentEvent for unknown resource, got %T", result.Events[0])
	}

	if docEvent.DocumentType != "MedicationRequest" {
		t.Errorf("DocumentType = %q, want %q", docEvent.DocumentType, "MedicationRequest")
	}
}

func TestParserParseInvalidJSON(t *testing.T) {
	parser := NewParser("test-source")

	invalidJSON := `{not valid json}`

	_, err := parser.ParseWithResult([]byte(invalidJSON))
	if err == nil {
		t.Error("Expected error for invalid JSON, got nil")
	}
}

func TestParserParseMissingResourceType(t *testing.T) {
	parser := NewParser("test-source")

	noTypeJSON := `{
		"id": "12345",
		"name": "John"
	}`

	_, err := parser.ParseWithResult([]byte(noTypeJSON))
	if err == nil {
		t.Error("Expected error for missing resourceType, got nil")
	}
}

func TestParserSourceFormat(t *testing.T) {
	parser := NewParser("my-fhir-server")

	patientJSON := `{
		"resourceType": "Patient",
		"id": "p1"
	}`

	result, err := parser.ParseWithResult([]byte(patientJSON))
	if err != nil {
		t.Fatalf("ParseWithResult() error = %v", err)
	}

	docEvent := result.Events[0].(*events.DocumentEvent)

	if docEvent.Source != "my-fhir-server" {
		t.Errorf("Source = %q, want %q", docEvent.Source, "my-fhir-server")
	}

	if docEvent.SourceFormat != events.FormatFHIR {
		t.Errorf("SourceFormat = %v, want %v", docEvent.SourceFormat, events.FormatFHIR)
	}
}

func TestExtractHelpers(t *testing.T) {
	t.Run("extractString", func(t *testing.T) {
		m := map[string]interface{}{"key": "value", "num": 123}
		if v := extractString(m, "key"); v != "value" {
			t.Errorf("extractString(key) = %q, want %q", v, "value")
		}
		if v := extractString(m, "num"); v != "" {
			t.Errorf("extractString(num) = %q, want empty", v)
		}
		if v := extractString(m, "missing"); v != "" {
			t.Errorf("extractString(missing) = %q, want empty", v)
		}
	})

	t.Run("extractNestedString", func(t *testing.T) {
		m := map[string]interface{}{
			"level1": map[string]interface{}{
				"level2": "deep-value",
			},
			"array": []interface{}{
				map[string]interface{}{"item": "first"},
			},
		}
		if v := extractNestedString(m, "level1", "level2"); v != "deep-value" {
			t.Errorf("extractNestedString nested = %q, want %q", v, "deep-value")
		}
		if v := extractNestedString(m, "array", "item"); v != "first" {
			t.Errorf("extractNestedString array = %q, want %q", v, "first")
		}
		if v := extractNestedString(m, "missing", "path"); v != "" {
			t.Errorf("extractNestedString missing = %q, want empty", v)
		}
	})

	t.Run("extractTime", func(t *testing.T) {
		m := map[string]interface{}{
			"rfc3339": "2025-01-10T10:30:00Z",
			"date":    "2025-01-10",
			"invalid": "not-a-date",
		}
		if tm := extractTime(m, "rfc3339"); tm.Year() != 2025 || tm.Month() != 1 || tm.Day() != 10 {
			t.Errorf("extractTime(rfc3339) = %v, want 2025-01-10", tm)
		}
		if tm := extractTime(m, "date"); tm.Year() != 2025 || tm.Month() != 1 || tm.Day() != 10 {
			t.Errorf("extractTime(date) = %v, want 2025-01-10", tm)
		}
		// Invalid should return current time (not panic)
		tm := extractTime(m, "invalid")
		if tm.Year() < 2020 {
			t.Errorf("extractTime(invalid) should return reasonable default")
		}
	})

	t.Run("extractCodeableConcept", func(t *testing.T) {
		m := map[string]interface{}{
			"code": map[string]interface{}{
				"coding": []interface{}{
					map[string]interface{}{
						"code":    "12345",
						"display": "Test Code",
						"system":  "http://example.org",
					},
				},
				"text": "Override Text",
			},
		}
		cv := extractCodeableConcept(m, "code")
		if cv.Code != "12345" {
			t.Errorf("Code = %q, want %q", cv.Code, "12345")
		}
		if cv.Display != "Override Text" {
			t.Errorf("Display = %q, want %q", cv.Display, "Override Text")
		}
		if cv.System != "http://example.org" {
			t.Errorf("System = %q, want %q", cv.System, "http://example.org")
		}
	})

	t.Run("extractValue", func(t *testing.T) {
		m1 := map[string]interface{}{
			"valueQuantity": map[string]interface{}{
				"value": 98.6,
				"unit":  "degF",
			},
		}
		vq := extractValue(m1)
		if vq.Value != "98.6" {
			t.Errorf("Value = %q, want %q", vq.Value, "98.6")
		}
		if vq.Unit != "degF" {
			t.Errorf("Unit = %q, want %q", vq.Unit, "degF")
		}

		m2 := map[string]interface{}{
			"valueString": "positive",
		}
		vs := extractValue(m2)
		if vs.Value != "positive" {
			t.Errorf("valueString = %q, want %q", vs.Value, "positive")
		}
	})

	t.Run("extractCategory", func(t *testing.T) {
		m := map[string]interface{}{
			"category": []interface{}{
				map[string]interface{}{
					"coding": []interface{}{
						map[string]interface{}{"code": "vital-signs"},
					},
				},
			},
		}
		if cat := extractCategory(m); cat != "vital-signs" {
			t.Errorf("extractCategory = %q, want %q", cat, "vital-signs")
		}
	})
}
