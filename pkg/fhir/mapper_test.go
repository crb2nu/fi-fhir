package fhir

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/cblevins/fi-fhir/pkg/events"
)

func TestUSCoreMapperMapPatient(t *testing.T) {
	mapper := NewUSCoreMapper()

	dob := time.Date(1980, 5, 15, 0, 0, 0, 0, time.UTC)
	patient := &events.Patient{
		MRN:        "123456",
		FamilyName: "Doe",
		GivenName:  "John",
		MiddleName: "William",
		Prefix:     "Mr.",
		Gender:     "M",
		DateOfBirth: dob,
		Race:       "White",
		Ethnicity:  "Not Hispanic",
		Address: events.Address{
			Line1:      "123 Main St",
			City:       "Anytown",
			State:      "VA",
			PostalCode: "24101",
		},
		Phone: "555-123-4567",
		Email: "john.doe@example.com",
		Identifiers: events.IdentifierSet{
			Identifiers: []events.Identifier{
				{Type: "MR", Value: "123456"},
				{Type: "SS", Value: "123-45-6789"},
			},
		},
	}

	fhirPatient := mapper.MapPatient(patient)

	// Verify resource type and profile
	if fhirPatient.ResourceType != "Patient" {
		t.Errorf("ResourceType = %q, want 'Patient'", fhirPatient.ResourceType)
	}
	if len(fhirPatient.Meta.Profile) != 1 || fhirPatient.Meta.Profile[0] != USCorePatientProfile {
		t.Errorf("Profile = %v, want [%s]", fhirPatient.Meta.Profile, USCorePatientProfile)
	}

	// Verify identifiers
	if len(fhirPatient.Identifier) != 2 {
		t.Errorf("Identifier count = %d, want 2", len(fhirPatient.Identifier))
	}

	// Verify name
	if len(fhirPatient.Name) != 1 {
		t.Fatalf("Name count = %d, want 1", len(fhirPatient.Name))
	}
	if fhirPatient.Name[0].Family != "Doe" {
		t.Errorf("Family = %q, want 'Doe'", fhirPatient.Name[0].Family)
	}
	if len(fhirPatient.Name[0].Given) != 2 {
		t.Errorf("Given count = %d, want 2", len(fhirPatient.Name[0].Given))
	}

	// Verify gender
	if fhirPatient.Gender != "male" {
		t.Errorf("Gender = %q, want 'male'", fhirPatient.Gender)
	}

	// Verify birth date
	if fhirPatient.BirthDate != "1980-05-15" {
		t.Errorf("BirthDate = %q, want '1980-05-15'", fhirPatient.BirthDate)
	}

	// Verify address
	if len(fhirPatient.Address) != 1 {
		t.Fatalf("Address count = %d, want 1", len(fhirPatient.Address))
	}
	if fhirPatient.Address[0].City != "Anytown" {
		t.Errorf("Address.City = %q, want 'Anytown'", fhirPatient.Address[0].City)
	}

	// Verify telecom
	if len(fhirPatient.Telecom) != 2 {
		t.Errorf("Telecom count = %d, want 2 (phone + email)", len(fhirPatient.Telecom))
	}

	// Verify extensions (race, ethnicity)
	if len(fhirPatient.Extension) != 2 {
		t.Errorf("Extension count = %d, want 2 (race + ethnicity)", len(fhirPatient.Extension))
	}
}

func TestUSCoreMapperMapPatientGender(t *testing.T) {
	mapper := NewUSCoreMapper()

	tests := []struct {
		input string
		want  string
	}{
		{"M", "male"},
		{"m", "male"},
		{"MALE", "male"},
		{"F", "female"},
		{"f", "female"},
		{"FEMALE", "female"},
		{"O", "other"},
		{"OTHER", "other"},
		{"", "unknown"},
		{"X", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			patient := &events.Patient{Gender: tt.input}
			fhirPatient := mapper.MapPatient(patient)
			if fhirPatient.Gender != tt.want {
				t.Errorf("Gender for %q = %q, want %q", tt.input, fhirPatient.Gender, tt.want)
			}
		})
	}
}

func TestUSCoreMapperMapEncounter(t *testing.T) {
	mapper := NewUSCoreMapper()

	admitTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	encounter := &events.Encounter{
		ID:            "ENC123",
		Class:         "I",
		Status:        "active",
		AdmitDateTime: admitTime,
		Location: events.Location{
			Facility: "Main Hospital",
			Unit:     "ICU",
			Room:     "101",
			Bed:      "A",
		},
		AttendingProvider: &events.Provider{
			NPI:        "1234567890",
			FamilyName: "Smith",
			GivenName:  "Jane",
			Prefix:     "Dr.",
		},
	}

	fhirEnc := mapper.MapEncounter(encounter, "Patient/123456")

	// Verify resource type and profile
	if fhirEnc.ResourceType != "Encounter" {
		t.Errorf("ResourceType = %q, want 'Encounter'", fhirEnc.ResourceType)
	}

	// Verify class
	if fhirEnc.Class.Code != "IMP" {
		t.Errorf("Class.Code = %q, want 'IMP'", fhirEnc.Class.Code)
	}

	// Verify status
	if fhirEnc.Status != "in-progress" {
		t.Errorf("Status = %q, want 'in-progress'", fhirEnc.Status)
	}

	// Verify subject
	if fhirEnc.Subject == nil || fhirEnc.Subject.Reference != "Patient/123456" {
		t.Errorf("Subject.Reference = %v, want 'Patient/123456'", fhirEnc.Subject)
	}

	// Verify period
	if fhirEnc.Period == nil || fhirEnc.Period.Start == nil {
		t.Errorf("Period.Start is nil")
	}

	// Verify participant
	if len(fhirEnc.Participant) != 1 {
		t.Errorf("Participant count = %d, want 1", len(fhirEnc.Participant))
	}

	// Verify location
	if len(fhirEnc.Location) != 1 {
		t.Errorf("Location count = %d, want 1", len(fhirEnc.Location))
	}
}

func TestUSCoreMapperMapEncounterClass(t *testing.T) {
	mapper := NewUSCoreMapper()

	tests := []struct {
		input    string
		wantCode string
	}{
		{"I", "IMP"},
		{"INPATIENT", "IMP"},
		{"O", "AMB"},
		{"OUTPATIENT", "AMB"},
		{"E", "EMER"},
		{"EMERGENCY", "EMER"},
		{"P", "PRENC"},
		{"PREADMIT", "PRENC"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			enc := &events.Encounter{Class: tt.input}
			fhirEnc := mapper.MapEncounter(enc, "")
			if fhirEnc.Class.Code != tt.wantCode {
				t.Errorf("Class.Code for %q = %q, want %q", tt.input, fhirEnc.Class.Code, tt.wantCode)
			}
		})
	}
}

func TestUSCoreMapperMapLabObservation(t *testing.T) {
	mapper := NewUSCoreMapper()

	obsTime := time.Date(2024, 1, 15, 14, 30, 0, 0, time.UTC)
	lab := &events.LabObservation{
		Test: events.LabTest{
			LOINCCode:   "6690-2",
			Description: "Leukocytes [#/volume] in Blood",
		},
		Result: events.LabValue{
			Value:           "12.5",
			Unit:            "10*3/uL",
			ReferenceRange:  "4.5-11.0",
			Interpretation:  "H",
			Status:          "F",
			ObservationTime: obsTime,
		},
	}

	obs := mapper.MapLabObservation(lab, "Patient/123")

	// Verify resource type and profile
	if obs.ResourceType != "Observation" {
		t.Errorf("ResourceType = %q, want 'Observation'", obs.ResourceType)
	}
	if len(obs.Meta.Profile) != 1 || obs.Meta.Profile[0] != USCoreObservationLabProfile {
		t.Errorf("Profile = %v, want [%s]", obs.Meta.Profile, USCoreObservationLabProfile)
	}

	// Verify status
	if obs.Status != "final" {
		t.Errorf("Status = %q, want 'final'", obs.Status)
	}

	// Verify category
	if len(obs.Category) != 1 || obs.Category[0].Coding[0].Code != "laboratory" {
		t.Errorf("Category = %v, want 'laboratory'", obs.Category)
	}

	// Verify code (LOINC)
	if len(obs.Code.Coding) == 0 || obs.Code.Coding[0].Code != "6690-2" {
		t.Errorf("Code = %v, want LOINC 6690-2", obs.Code.Coding)
	}

	// Verify value
	if obs.ValueQuantity == nil || obs.ValueQuantity.Value != 12.5 {
		t.Errorf("ValueQuantity = %v, want 12.5", obs.ValueQuantity)
	}
	if obs.ValueQuantity.Unit != "10*3/uL" {
		t.Errorf("ValueQuantity.Unit = %q, want '10*3/uL'", obs.ValueQuantity.Unit)
	}

	// Verify interpretation
	if len(obs.Interpretation) != 1 || obs.Interpretation[0].Coding[0].Code != "H" {
		t.Errorf("Interpretation = %v, want 'H'", obs.Interpretation)
	}

	// Verify reference range
	if len(obs.ReferenceRange) != 1 || obs.ReferenceRange[0].Text != "4.5-11.0" {
		t.Errorf("ReferenceRange = %v, want '4.5-11.0'", obs.ReferenceRange)
	}
}

func TestUSCoreMapperMapLabResult(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.LabResultEvent{
		Patient: events.Patient{MRN: "123456"},
		Results: []events.LabObservation{
			{
				Test:   events.LabTest{LOINCCode: "6690-2", Description: "WBC"},
				Result: events.LabValue{Value: "12.5", Unit: "10*3/uL"},
			},
			{
				Test:   events.LabTest{LOINCCode: "789-8", Description: "RBC"},
				Result: events.LabValue{Value: "5.2", Unit: "10*6/uL"},
			},
		},
	}

	report, observations := mapper.MapLabResult(event)

	// Verify DiagnosticReport
	if report == nil {
		t.Fatal("DiagnosticReport is nil")
	}
	if report.ResourceType != "DiagnosticReport" {
		t.Errorf("Report.ResourceType = %q, want 'DiagnosticReport'", report.ResourceType)
	}

	// Verify observations
	if len(observations) != 2 {
		t.Errorf("Observations count = %d, want 2", len(observations))
	}

	// Verify result references in report
	if len(report.Result) != 2 {
		t.Errorf("Report.Result count = %d, want 2", len(report.Result))
	}
}

func TestUSCoreMapperInterpretationMapping(t *testing.T) {
	mapper := NewUSCoreMapper()

	tests := []struct {
		input    string
		wantCode string
	}{
		{"H", "H"},
		{"HIGH", "H"},
		{"HH", "HH"},
		{"CRITICAL HIGH", "HH"},
		{"L", "L"},
		{"LOW", "L"},
		{"LL", "LL"},
		{"N", "N"},
		{"NORMAL", "N"},
		{"A", "A"},
		{"ABNORMAL", "A"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			lab := &events.LabObservation{
				Test:   events.LabTest{Description: "Test"},
				Result: events.LabValue{Value: "1.0", Interpretation: tt.input},
			}
			obs := mapper.MapLabObservation(lab, "")
			if len(obs.Interpretation) == 0 || obs.Interpretation[0].Coding[0].Code != tt.wantCode {
				t.Errorf("Interpretation for %q = %v, want %q", tt.input, obs.Interpretation, tt.wantCode)
			}
		})
	}
}

func TestPatientJSONSerialization(t *testing.T) {
	patient := &Patient{
		ResourceType: "Patient",
		Meta: &Meta{
			Profile: []string{USCorePatientProfile},
		},
		Identifier: []Identifier{
			{System: "http://hospital.example.org/mrn", Value: "123456"},
		},
		Name: []HumanName{
			{Family: "Doe", Given: []string{"John"}},
		},
		Gender:    "male",
		BirthDate: "1980-05-15",
	}

	data, err := json.Marshal(patient)
	if err != nil {
		t.Fatalf("Failed to marshal patient: %v", err)
	}

	// Verify JSON contains required fields
	jsonStr := string(data)
	if !strings.Contains(jsonStr, `"resourceType":"Patient"`) {
		t.Error("JSON missing resourceType")
	}
	if !strings.Contains(jsonStr, USCorePatientProfile) {
		t.Error("JSON missing US Core profile")
	}
}

func TestObservationJSONSerialization(t *testing.T) {
	obs := &Observation{
		ResourceType: "Observation",
		Status:       "final",
		Code: CodeableConcept{
			Coding: []Coding{
				{System: SystemLOINC, Code: "6690-2", Display: "WBC"},
			},
		},
		ValueQuantity: &Quantity{
			Value:  12.5,
			Unit:   "10*3/uL",
			System: SystemUCUM,
		},
	}

	data, err := json.Marshal(obs)
	if err != nil {
		t.Fatalf("Failed to marshal observation: %v", err)
	}

	// Verify JSON
	jsonStr := string(data)
	if !strings.Contains(jsonStr, `"resourceType":"Observation"`) {
		t.Error("JSON missing resourceType")
	}
	if !strings.Contains(jsonStr, `"value":12.5`) {
		t.Error("JSON missing value")
	}
}

func TestCreateTransactionBundle(t *testing.T) {
	mapper := NewUSCoreMapper()

	patient := mapper.MapPatient(&events.Patient{
		MRN:        "123",
		FamilyName: "Test",
		Gender:     "M",
	})

	bundle := CreateTransactionBundle([]Resource{patient})

	if bundle.Type != "transaction" {
		t.Errorf("Bundle.Type = %q, want 'transaction'", bundle.Type)
	}
	if len(bundle.Entry) != 1 {
		t.Errorf("Bundle.Entry count = %d, want 1", len(bundle.Entry))
	}
	if bundle.Entry[0].Request == nil || bundle.Entry[0].Request.Method != "POST" {
		t.Error("Bundle entry missing POST request")
	}
}

func TestCreateSearchsetBundle(t *testing.T) {
	mapper := NewUSCoreMapper()

	patients := []Resource{
		mapper.MapPatient(&events.Patient{MRN: "1", FamilyName: "A"}),
		mapper.MapPatient(&events.Patient{MRN: "2", FamilyName: "B"}),
	}

	bundle := CreateSearchsetBundle(patients, 100)

	if bundle.Type != "searchset" {
		t.Errorf("Bundle.Type = %q, want 'searchset'", bundle.Type)
	}
	if bundle.Total != 100 {
		t.Errorf("Bundle.Total = %d, want 100", bundle.Total)
	}
	if len(bundle.Entry) != 2 {
		t.Errorf("Bundle.Entry count = %d, want 2", len(bundle.Entry))
	}
	if bundle.Entry[0].Search == nil || bundle.Entry[0].Search.Mode != "match" {
		t.Error("Bundle entry missing search mode")
	}
}

func TestRaceExtensionMapping(t *testing.T) {
	mapper := NewUSCoreMapper()

	tests := []struct {
		race     string
		wantCode string
	}{
		{"White", "2106-3"},
		{"WHITE", "2106-3"},
		{"Black", "2054-5"},
		{"AFRICAN AMERICAN", "2054-5"},
		{"Asian", "2028-9"},
		{"Native American", "1002-5"},
		{"Pacific Islander", "2076-8"},
	}

	for _, tt := range tests {
		t.Run(tt.race, func(t *testing.T) {
			patient := mapper.MapPatient(&events.Patient{
				FamilyName: "Test",
				Race:       tt.race,
			})

			found := false
			for _, ext := range patient.Extension {
				if ext.URL == USCoreRaceExtension {
					for _, nested := range ext.Extension {
						if nested.URL == "ombCategory" && nested.ValueCoding != nil {
							if nested.ValueCoding.Code == tt.wantCode {
								found = true
							}
						}
					}
				}
			}
			if !found {
				t.Errorf("Race extension for %q not found or wrong code", tt.race)
			}
		})
	}
}

func TestNilInputHandling(t *testing.T) {
	mapper := NewUSCoreMapper()

	// All nil inputs should return nil without panicking
	if mapper.MapPatient(nil) != nil {
		t.Error("MapPatient(nil) should return nil")
	}
	if mapper.MapEncounter(nil, "") != nil {
		t.Error("MapEncounter(nil) should return nil")
	}
	if mapper.MapLabObservation(nil, "") != nil {
		t.Error("MapLabObservation(nil) should return nil")
	}

	report, obs := mapper.MapLabResult(nil)
	if report != nil || obs != nil {
		t.Error("MapLabResult(nil) should return nil")
	}
}
