package workflow

import (
	"testing"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/fhir"
)

func TestRenderTemplate(t *testing.T) {
	tests := []struct {
		name     string
		template string
		data     interface{}
		expected string
	}{
		{
			name:     "no placeholders",
			template: "Hello World",
			data:     nil,
			expected: "Hello World",
		},
		{
			name:     "simple placeholder",
			template: "Hello {{.Name}}",
			data:     map[string]interface{}{"Name": "John"},
			expected: "Hello John",
		},
		{
			name:     "multiple placeholders",
			template: "{{.First}} {{.Last}}",
			data:     map[string]interface{}{"First": "John", "Last": "Doe"},
			expected: "John Doe",
		},
		{
			name:     "nested data",
			template: "Patient: {{.Patient.Name}}",
			data:     map[string]interface{}{"Patient": map[string]interface{}{"Name": "John"}},
			expected: "Patient: John",
		},
		{
			name:     "invalid template syntax returns original",
			template: "Hello {{.Name",
			data:     map[string]interface{}{"Name": "John"},
			expected: "Hello {{.Name",
		},
		{
			name:     "missing field returns empty",
			template: "Hello {{.Missing}}",
			data:     map[string]interface{}{"Name": "John"},
			expected: "Hello <no value>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := renderTemplate(tt.template, tt.data)
			if result != tt.expected {
				t.Errorf("renderTemplate(%q) = %q, want %q", tt.template, result, tt.expected)
			}
		})
	}
}

func TestEventToFHIRResources(t *testing.T) {
	mapper := fhir.NewUSCoreMapper()
	config := map[string]string{}

	t.Run("PatientAdmitEvent pointer", func(t *testing.T) {
		event := &events.PatientAdmitEvent{
			EventMeta: events.EventMeta{Type: events.EventPatientAdmit},
			Patient: events.Patient{
				MRN:        "12345",
				GivenName:  "John",
				FamilyName: "Doe",
				Gender:     "M",
			},
			Encounter: events.Encounter{
				ID:     "E001",
				Class:  "inpatient",
				Status: "in-progress",
			},
		}

		resources, err := eventToFHIRResources(event, mapper, config)
		if err != nil {
			t.Fatalf("eventToFHIRResources error: %v", err)
		}

		if len(resources) < 2 {
			t.Errorf("resources length = %d, want at least 2 (Patient + Encounter)", len(resources))
		}

		// Verify Patient resource
		foundPatient := false
		foundEncounter := false
		for _, r := range resources {
			switch r.GetResourceType() {
			case "Patient":
				foundPatient = true
			case "Encounter":
				foundEncounter = true
			}
		}
		if !foundPatient {
			t.Error("Patient resource not found")
		}
		if !foundEncounter {
			t.Error("Encounter resource not found")
		}
	})

	t.Run("PatientAdmitEvent value", func(t *testing.T) {
		event := events.PatientAdmitEvent{
			EventMeta: events.EventMeta{Type: events.EventPatientAdmit},
			Patient: events.Patient{
				MRN:        "12345",
				GivenName:  "Jane",
				FamilyName: "Doe",
			},
			Encounter: events.Encounter{
				ID: "E002",
			},
		}

		resources, err := eventToFHIRResources(event, mapper, config)
		if err != nil {
			t.Fatalf("eventToFHIRResources error: %v", err)
		}

		if len(resources) < 2 {
			t.Errorf("resources length = %d, want at least 2", len(resources))
		}
	})

	t.Run("PatientDischargeEvent pointer", func(t *testing.T) {
		event := &events.PatientDischargeEvent{
			EventMeta: events.EventMeta{Type: events.EventPatientDischarge},
			Patient: events.Patient{
				MRN: "12345",
			},
			Encounter: events.Encounter{
				ID:     "E001",
				Status: "finished",
			},
		}

		resources, err := eventToFHIRResources(event, mapper, config)
		if err != nil {
			t.Fatalf("eventToFHIRResources error: %v", err)
		}

		if len(resources) < 2 {
			t.Errorf("resources length = %d, want at least 2", len(resources))
		}
	})

	t.Run("PatientDischargeEvent value", func(t *testing.T) {
		event := events.PatientDischargeEvent{
			EventMeta: events.EventMeta{Type: events.EventPatientDischarge},
			Patient: events.Patient{
				MRN: "12345",
			},
			Encounter: events.Encounter{
				ID:     "E002",
				Status: "finished",
			},
		}

		resources, err := eventToFHIRResources(event, mapper, config)
		if err != nil {
			t.Fatalf("eventToFHIRResources error: %v", err)
		}

		if len(resources) < 2 {
			t.Errorf("resources length = %d, want at least 2", len(resources))
		}
	})

	t.Run("LabResultEvent pointer", func(t *testing.T) {
		event := &events.LabResultEvent{
			EventMeta: events.EventMeta{Type: events.EventLabResult},
			Patient: events.Patient{
				MRN: "12345",
			},
			Test: events.LabTest{
				Code: events.CodeableConcept{
					Text: "Glucose",
					Coding: []events.Coding{
						{System: "http://loinc.org", Code: "1234-5", Display: "Glucose"},
					},
				},
				Description: "Glucose",
			},
			Result: events.LabValue{
				Value: "100",
				Unit:  "mg/dL",
			},
		}

		resources, err := eventToFHIRResources(event, mapper, config)
		if err != nil {
			t.Fatalf("eventToFHIRResources error: %v", err)
		}

		// Should have DiagnosticReport and Observation(s)
		if len(resources) < 1 {
			t.Errorf("resources length = %d, want at least 1", len(resources))
		}
	})

	t.Run("LabResultEvent value", func(t *testing.T) {
		event := events.LabResultEvent{
			EventMeta: events.EventMeta{Type: events.EventLabResult},
			Patient: events.Patient{
				MRN: "12345",
			},
			Test: events.LabTest{
				Code: events.CodeableConcept{
					Text: "Hemoglobin",
					Coding: []events.Coding{
						{System: "http://loinc.org", Code: "5678-9", Display: "Hemoglobin"},
					},
				},
				Description: "Hemoglobin",
			},
			Result: events.LabValue{
				Value: "14.5",
				Unit:  "g/dL",
			},
		}

		resources, err := eventToFHIRResources(event, mapper, config)
		if err != nil {
			t.Fatalf("eventToFHIRResources error: %v", err)
		}

		if len(resources) < 1 {
			t.Errorf("resources length = %d, want at least 1", len(resources))
		}
	})

	t.Run("unsupported event type", func(t *testing.T) {
		event := struct{ Name string }{"test"}

		_, err := eventToFHIRResources(event, mapper, config)
		if err == nil {
			t.Error("expected error for unsupported event type")
		}
	})
}

func TestMapEventToFHIR(t *testing.T) {
	mapper := fhir.NewUSCoreMapper()
	config := map[string]string{}

	t.Run("patient_admit map event", func(t *testing.T) {
		event := map[string]interface{}{
			"type": "patient_admit",
			"patient": map[string]interface{}{
				"mrn":         "12345",
				"given_name":  "John",
				"family_name": "Doe",
				"gender":      "M",
			},
			"encounter": map[string]interface{}{
				"id":     "E001",
				"class":  "inpatient",
				"status": "in-progress",
			},
		}

		resources, err := mapEventToFHIR(event, mapper, config)
		if err != nil {
			t.Fatalf("mapEventToFHIR error: %v", err)
		}

		if len(resources) < 2 {
			t.Errorf("resources length = %d, want at least 2", len(resources))
		}
	})

	t.Run("patient_discharge map event", func(t *testing.T) {
		event := map[string]interface{}{
			"type": "patient_discharge",
			"patient": map[string]interface{}{
				"mrn": "12345",
			},
			"encounter": map[string]interface{}{
				"id":     "E001",
				"status": "finished",
			},
		}

		resources, err := mapEventToFHIR(event, mapper, config)
		if err != nil {
			t.Fatalf("mapEventToFHIR error: %v", err)
		}

		if len(resources) < 2 {
			t.Errorf("resources length = %d, want at least 2", len(resources))
		}
	})

	t.Run("lab_result map event", func(t *testing.T) {
		event := map[string]interface{}{
			"type": "lab_result",
			"patient": map[string]interface{}{
				"mrn": "12345",
			},
			"test": map[string]interface{}{
				"code": map[string]interface{}{
					"text": "Glucose",
					"coding": []interface{}{
						map[string]interface{}{
							"system":  "http://loinc.org",
							"code":    "1234-5",
							"display": "Glucose",
						},
					},
				},
				"description": "Glucose",
			},
			"result": map[string]interface{}{
				"value": "100",
				"unit":  "mg/dL",
			},
		}

		resources, err := mapEventToFHIR(event, mapper, config)
		if err != nil {
			t.Fatalf("mapEventToFHIR error: %v", err)
		}

		if len(resources) < 1 {
			t.Errorf("resources length = %d, want at least 1", len(resources))
		}
	})

	t.Run("generic event with patient data", func(t *testing.T) {
		event := map[string]interface{}{
			"type": "unknown_type",
			"patient": map[string]interface{}{
				"mrn":         "12345",
				"given_name":  "John",
				"family_name": "Doe",
			},
		}

		resources, err := mapEventToFHIR(event, mapper, config)
		if err != nil {
			t.Fatalf("mapEventToFHIR error: %v", err)
		}

		// Should extract patient from generic event
		if len(resources) != 1 {
			t.Errorf("resources length = %d, want 1", len(resources))
		}
		if resources[0].GetResourceType() != "Patient" {
			t.Errorf("resource type = %s, want Patient", resources[0].GetResourceType())
		}
	})

	t.Run("unsupported type without patient", func(t *testing.T) {
		event := map[string]interface{}{
			"type": "unknown_type",
			"data": "some data",
		}

		_, err := mapEventToFHIR(event, mapper, config)
		if err == nil {
			t.Error("expected error for unsupported event without patient")
		}
	})

	t.Run("map event passed to eventToFHIRResources", func(t *testing.T) {
		// When a map is passed to eventToFHIRResources, it should delegate to mapEventToFHIR
		event := map[string]interface{}{
			"type": "patient_admit",
			"patient": map[string]interface{}{
				"mrn": "12345",
			},
			"encounter": map[string]interface{}{
				"id": "E001",
			},
		}

		resources, err := eventToFHIRResources(event, mapper, config)
		if err != nil {
			t.Fatalf("eventToFHIRResources error: %v", err)
		}

		if len(resources) < 2 {
			t.Errorf("resources length = %d, want at least 2", len(resources))
		}
	})
}

func TestLogActionCoverage(t *testing.T) {
	// Additional log action tests for coverage
	t.Run("template in message", func(t *testing.T) {
		config := map[string]string{
			"level":   "info",
			"message": "Processing event: {{.type}}",
		}
		event := map[string]interface{}{
			"type": "patient_admit",
			"id":   "123",
		}

		err := logAction(event, config)
		if err != nil {
			t.Errorf("logAction error: %v", err)
		}
	})
}
