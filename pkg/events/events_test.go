package events

import (
	"encoding/json"
	"testing"
	"time"
)

func TestIdentifierSetGetByType(t *testing.T) {
	idSet := IdentifierSet{
		Identifiers: []Identifier{
			{Value: "12345", Type: "MR", Assigner: "HOSP_A"},
			{Value: "999-99-9999", Type: "SS"},
			{Value: "1EG4TE58K72", Type: "MB"},
		},
	}

	tests := []struct {
		idType string
		want   string
	}{
		{"MR", "12345"},
		{"SS", "999-99-9999"},
		{"MB", "1EG4TE58K72"},
		{"NPI", ""}, // Not found
	}

	for _, tt := range tests {
		t.Run(tt.idType, func(t *testing.T) {
			id := idSet.GetByType(tt.idType)
			if tt.want == "" {
				if id != nil {
					t.Errorf("GetByType(%q) = %v, want nil", tt.idType, id)
				}
			} else {
				if id == nil {
					t.Errorf("GetByType(%q) = nil, want %q", tt.idType, tt.want)
				} else if id.Value != tt.want {
					t.Errorf("GetByType(%q).Value = %q, want %q", tt.idType, id.Value, tt.want)
				}
			}
		})
	}
}

func TestIdentifierSetGetBySystem(t *testing.T) {
	idSet := IdentifierSet{
		Identifiers: []Identifier{
			{Value: "12345", Type: "MR", System: "urn:oid:1.2.3.4"},
			{Value: "67890", Type: "PI", System: "urn:oid:1.2.3.4"},
			{Value: "ABC", Type: "EI", System: "urn:oid:5.6.7.8"},
		},
	}

	// Should find 2 identifiers with system urn:oid:1.2.3.4
	ids := idSet.GetBySystem("urn:oid:1.2.3.4")
	if len(ids) != 2 {
		t.Errorf("GetBySystem() returned %d identifiers, want 2", len(ids))
	}

	// Should find 0 with unknown system
	ids = idSet.GetBySystem("urn:oid:unknown")
	if len(ids) != 0 {
		t.Errorf("GetBySystem(unknown) returned %d identifiers, want 0", len(ids))
	}
}

func TestIdentifierSetGetMRN(t *testing.T) {
	tests := []struct {
		name   string
		idSet  IdentifierSet
		want   string
	}{
		{
			name: "MRN present",
			idSet: IdentifierSet{
				Identifiers: []Identifier{
					{Value: "12345", Type: "MR"},
					{Value: "SSN123", Type: "SS"},
				},
			},
			want: "12345",
		},
		{
			name: "primary fallback",
			idSet: IdentifierSet{
				Identifiers: []Identifier{
					{Value: "SSN123", Type: "SS"},
				},
				Primary: &Identifier{Value: "PRIMARY1", Type: "PI"},
			},
			want: "PRIMARY1",
		},
		{
			name: "first identifier fallback",
			idSet: IdentifierSet{
				Identifiers: []Identifier{
					{Value: "FIRST", Type: "PI"},
					{Value: "SECOND", Type: "SS"},
				},
			},
			want: "FIRST",
		},
		{
			name:  "empty set",
			idSet: IdentifierSet{},
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.idSet.GetMRN()
			if got != tt.want {
				t.Errorf("GetMRN() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNewEventMeta(t *testing.T) {
	before := time.Now().UTC()
	meta := NewEventMeta(EventPatientAdmit, "test_source", FormatHL7v2)
	after := time.Now().UTC()

	if meta.Type != EventPatientAdmit {
		t.Errorf("Type = %q, want %q", meta.Type, EventPatientAdmit)
	}
	if meta.Source != "test_source" {
		t.Errorf("Source = %q, want 'test_source'", meta.Source)
	}
	if meta.SourceFormat != FormatHL7v2 {
		t.Errorf("SourceFormat = %q, want %q", meta.SourceFormat, FormatHL7v2)
	}
	if meta.ID == "" {
		t.Error("ID should not be empty")
	}
	if meta.Timestamp.Before(before) || meta.Timestamp.After(after) {
		t.Errorf("Timestamp %v not in expected range", meta.Timestamp)
	}
	if meta.ReceivedAt.Before(before) || meta.ReceivedAt.After(after) {
		t.Errorf("ReceivedAt %v not in expected range", meta.ReceivedAt)
	}
}

func TestPatientAdmitEventJSON(t *testing.T) {
	event := PatientAdmitEvent{
		EventMeta: EventMeta{
			ID:           "test-id",
			Type:         EventPatientAdmit,
			Source:       "test",
			SourceFormat: FormatHL7v2,
		},
		Patient: Patient{
			MRN:        "12345",
			FamilyName: "DOE",
			GivenName:  "JOHN",
			Gender:     "M",
		},
		Encounter: Encounter{
			ID:    "V001",
			Class: "I",
		},
	}

	// Marshal to JSON
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	// Unmarshal back
	var decoded PatientAdmitEvent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	// Verify round-trip
	if decoded.Patient.MRN != event.Patient.MRN {
		t.Errorf("Patient.MRN = %q, want %q", decoded.Patient.MRN, event.Patient.MRN)
	}
	if decoded.Encounter.Class != event.Encounter.Class {
		t.Errorf("Encounter.Class = %q, want %q", decoded.Encounter.Class, event.Encounter.Class)
	}
}

func TestLabResultEventWithMultipleResults(t *testing.T) {
	event := LabResultEvent{
		EventMeta: EventMeta{
			ID:           "lab-001",
			Type:         EventLabResult,
			Source:       "lab_system",
			SourceFormat: FormatHL7v2,
		},
		Patient: Patient{
			MRN:        "67890",
			FamilyName: "SMITH",
		},
		Test: LabTest{
			LocalCode:   "WBC",
			Description: "White Blood Cell Count",
		},
		Result: LabValue{
			Value: "12.5",
			Unit:  "10*3/uL",
		},
		Results: []LabObservation{
			{
				Test:   LabTest{LocalCode: "WBC", Description: "White Blood Cell Count"},
				Result: LabValue{Value: "12.5", Unit: "10*3/uL"},
			},
			{
				Test:   LabTest{LocalCode: "RBC", Description: "Red Blood Cell Count"},
				Result: LabValue{Value: "4.8", Unit: "10*6/uL"},
			},
		},
	}

	// Marshal to JSON
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	// Unmarshal back
	var decoded LabResultEvent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	// Verify Results array
	if len(decoded.Results) != 2 {
		t.Fatalf("Results length = %d, want 2", len(decoded.Results))
	}

	if decoded.Results[0].Test.LocalCode != "WBC" {
		t.Errorf("Results[0].Test.LocalCode = %q, want 'WBC'", decoded.Results[0].Test.LocalCode)
	}
	if decoded.Results[1].Test.LocalCode != "RBC" {
		t.Errorf("Results[1].Test.LocalCode = %q, want 'RBC'", decoded.Results[1].Test.LocalCode)
	}
}

func TestCodeableConceptWithCodings(t *testing.T) {
	code := CodeableConcept{
		Text: "Glucose, Serum",
		Coding: []Coding{
			{
				System:  "http://loinc.org",
				Code:    "2345-7",
				Display: "Glucose [Mass/volume] in Serum or Plasma",
			},
			{
				System:         "LOCAL_LAB",
				Code:           "GLUC",
				Display:        "Glucose",
				OriginalSystem: "LOCAL_LAB",
				OriginalCode:   "GLUC",
			},
		},
	}

	data, err := json.Marshal(code)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var decoded CodeableConcept
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if len(decoded.Coding) != 2 {
		t.Errorf("Coding length = %d, want 2", len(decoded.Coding))
	}
	if decoded.Text != "Glucose, Serum" {
		t.Errorf("Text = %q, want 'Glucose, Serum'", decoded.Text)
	}
}

func TestParseWarning(t *testing.T) {
	warning := ParseWarning{
		Phase:    "semantic",
		Code:     "INVALID_NPI",
		Message:  "NPI failed checksum validation",
		Path:     "PID.3[2]",
		Severity: "warning",
	}

	data, err := json.Marshal(warning)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var decoded ParseWarning
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if decoded.Phase != "semantic" {
		t.Errorf("Phase = %q, want 'semantic'", decoded.Phase)
	}
	if decoded.Code != "INVALID_NPI" {
		t.Errorf("Code = %q, want 'INVALID_NPI'", decoded.Code)
	}
}

func TestPatientExtensions(t *testing.T) {
	patient := Patient{
		MRN:        "12345",
		FamilyName: "DOE",
		Extensions: map[string]interface{}{
			"vip_status":  true,
			"risk_score":  85,
			"external_id": "EXT123",
		},
	}

	data, err := json.Marshal(patient)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var decoded Patient
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if decoded.Extensions == nil {
		t.Fatal("Extensions should not be nil")
	}

	// Check VIP status (bool)
	if vip, ok := decoded.Extensions["vip_status"].(bool); !ok || !vip {
		t.Errorf("vip_status = %v, want true", decoded.Extensions["vip_status"])
	}

	// Check external_id (string)
	if ext, ok := decoded.Extensions["external_id"].(string); !ok || ext != "EXT123" {
		t.Errorf("external_id = %v, want 'EXT123'", decoded.Extensions["external_id"])
	}
}
