package fhir

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/crb2nu/fi-fhir/pkg/events"
)

func TestUSCoreMapperMapPatient(t *testing.T) {
	mapper := NewUSCoreMapper()

	dob := time.Date(1980, 5, 15, 0, 0, 0, 0, time.UTC)
	patient := &events.Patient{
		MRN:         "123456",
		FamilyName:  "Doe",
		GivenName:   "John",
		MiddleName:  "William",
		Prefix:      "Mr.",
		Gender:      "M",
		DateOfBirth: dob,
		Race:        "White",
		Ethnicity:   "Not Hispanic",
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

	if mapper.MapCondition(nil, "") != nil {
		t.Error("MapCondition(nil) should return nil")
	}
	if mapper.MapCoverage(nil, "") != nil {
		t.Error("MapCoverage(nil) should return nil")
	}
}

// =============================================================================
// Condition Tests
// =============================================================================

func TestUSCoreMapperMapCondition(t *testing.T) {
	mapper := NewUSCoreMapper()
	ts := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)

	event := &events.ConditionEvent{
		EventMeta: events.EventMeta{
			Timestamp: ts,
		},
		Patient: &events.Patient{MRN: "123456"},
		Condition: events.Condition{
			Name:       "Type 2 Diabetes Mellitus",
			Code:       "E11.9",
			CodeSystem: SystemICD10CM,
			Category:   "problem-list-item",
		},
		ClinicalStatus: "active",
		OnsetDate:      "2020-03-15",
		Encounter: &events.Encounter{
			ID: "ENC456",
		},
	}

	cond := mapper.MapCondition(event, "Patient/123456")

	// Verify resource type and profile
	if cond.ResourceType != "Condition" {
		t.Errorf("ResourceType = %q, want 'Condition'", cond.ResourceType)
	}
	if len(cond.Meta.Profile) != 1 || cond.Meta.Profile[0] != USCoreConditionProfile {
		t.Errorf("Profile = %v, want [%s]", cond.Meta.Profile, USCoreConditionProfile)
	}

	// Verify subject reference
	if cond.Subject == nil || cond.Subject.Reference != "Patient/123456" {
		t.Errorf("Subject.Reference = %v, want 'Patient/123456'", cond.Subject)
	}

	// Verify clinical status
	if cond.ClinicalStatus == nil || len(cond.ClinicalStatus.Coding) == 0 {
		t.Fatal("ClinicalStatus is nil or empty")
	}
	if cond.ClinicalStatus.Coding[0].Code != "active" {
		t.Errorf("ClinicalStatus.Code = %q, want 'active'", cond.ClinicalStatus.Coding[0].Code)
	}

	// Verify verification status
	if cond.VerificationStatus == nil || len(cond.VerificationStatus.Coding) == 0 {
		t.Fatal("VerificationStatus is nil or empty")
	}
	if cond.VerificationStatus.Coding[0].Code != "confirmed" {
		t.Errorf("VerificationStatus.Code = %q, want 'confirmed'", cond.VerificationStatus.Coding[0].Code)
	}

	// Verify category
	if len(cond.Category) != 1 || len(cond.Category[0].Coding) == 0 {
		t.Fatal("Category is nil or empty")
	}
	if cond.Category[0].Coding[0].Code != "problem-list-item" {
		t.Errorf("Category.Code = %q, want 'problem-list-item'", cond.Category[0].Coding[0].Code)
	}

	// Verify code
	if len(cond.Code.Coding) == 0 {
		t.Fatal("Code.Coding is empty")
	}
	if cond.Code.Coding[0].Code != "E11.9" {
		t.Errorf("Code.Code = %q, want 'E11.9'", cond.Code.Coding[0].Code)
	}
	if cond.Code.Coding[0].System != SystemICD10CM {
		t.Errorf("Code.System = %q, want %q", cond.Code.Coding[0].System, SystemICD10CM)
	}
	if cond.Code.Text != "Type 2 Diabetes Mellitus" {
		t.Errorf("Code.Text = %q, want 'Type 2 Diabetes Mellitus'", cond.Code.Text)
	}

	// Verify onset date
	if cond.OnsetDateTime != "2020-03-15" {
		t.Errorf("OnsetDateTime = %q, want '2020-03-15'", cond.OnsetDateTime)
	}

	// Verify encounter reference
	if cond.Encounter == nil || cond.Encounter.Reference != "Encounter/ENC456" {
		t.Errorf("Encounter.Reference = %v, want 'Encounter/ENC456'", cond.Encounter)
	}

	// Verify recorded date
	if cond.RecordedDate != "2024-01-15" {
		t.Errorf("RecordedDate = %q, want '2024-01-15'", cond.RecordedDate)
	}
}

func TestUSCoreMapperMapConditionClinicalStatus(t *testing.T) {
	mapper := NewUSCoreMapper()

	tests := []struct {
		input string
		want  string
	}{
		{"active", "active"},
		{"ACTIVE", "active"},
		{"", "active"},
		{"resolved", "resolved"},
		{"inactive", "inactive"},
		{"remission", "remission"},
		{"recurrence", "recurrence"},
		{"relapse", "relapse"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			event := &events.ConditionEvent{
				ClinicalStatus: tt.input,
				Condition:      events.Condition{Name: "Test"},
			}
			cond := mapper.MapCondition(event, "")
			if cond.ClinicalStatus.Coding[0].Code != tt.want {
				t.Errorf("ClinicalStatus for %q = %q, want %q", tt.input, cond.ClinicalStatus.Coding[0].Code, tt.want)
			}
		})
	}
}

func TestUSCoreMapperMapConditionCategory(t *testing.T) {
	mapper := NewUSCoreMapper()

	tests := []struct {
		input string
		want  string
	}{
		{"problem-list-item", "problem-list-item"},
		{"problem", "problem-list-item"},
		{"", "problem-list-item"},
		{"encounter-diagnosis", "encounter-diagnosis"},
		{"diagnosis", "encounter-diagnosis"},
		{"health-concern", "health-concern"},
		{"concern", "health-concern"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			event := &events.ConditionEvent{
				Condition: events.Condition{
					Name:     "Test",
					Category: tt.input,
				},
			}
			cond := mapper.MapCondition(event, "")
			if cond.Category[0].Coding[0].Code != tt.want {
				t.Errorf("Category for %q = %q, want %q", tt.input, cond.Category[0].Coding[0].Code, tt.want)
			}
		})
	}
}

func TestUSCoreMapperInferConditionCodeSystem(t *testing.T) {
	mapper := NewUSCoreMapper()

	tests := []struct {
		code       string
		wantSystem string
	}{
		{"E11.9", SystemICD10CM},    // ICD-10-CM diabetes
		{"J18.9", SystemICD10CM},    // ICD-10-CM pneumonia
		{"A01.0", SystemICD10CM},    // ICD-10-CM typhoid fever
		{"44054006", SystemSNOMED},  // SNOMED diabetes type 2
		{"233604007", SystemSNOMED}, // SNOMED pneumonia
		{"12345678", SystemSNOMED},  // Generic numeric (assume SNOMED)
		{"ABC", SystemSNOMED},       // Short non-numeric (default to SNOMED)
	}

	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			event := &events.ConditionEvent{
				Condition: events.Condition{
					Name: "Test",
					Code: tt.code,
					// CodeSystem intentionally left empty to test inference
				},
			}
			cond := mapper.MapCondition(event, "")
			if len(cond.Code.Coding) == 0 {
				t.Fatal("Code.Coding is empty")
			}
			if cond.Code.Coding[0].System != tt.wantSystem {
				t.Errorf("Inferred system for %q = %q, want %q", tt.code, cond.Code.Coding[0].System, tt.wantSystem)
			}
		})
	}
}

func TestUSCoreMapperMapConditionWithAbatement(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.ConditionEvent{
		Patient: &events.Patient{MRN: "123"},
		Condition: events.Condition{
			Name: "Acute Bronchitis",
			Code: "J20.9",
		},
		ClinicalStatus: "resolved",
		OnsetDate:      "2024-01-01",
		AbatementDate:  "2024-01-15",
	}

	cond := mapper.MapCondition(event, "")

	if cond.ClinicalStatus.Coding[0].Code != "resolved" {
		t.Errorf("ClinicalStatus = %q, want 'resolved'", cond.ClinicalStatus.Coding[0].Code)
	}
	if cond.OnsetDateTime != "2024-01-01" {
		t.Errorf("OnsetDateTime = %q, want '2024-01-01'", cond.OnsetDateTime)
	}
	if cond.AbatementDateTime != "2024-01-15" {
		t.Errorf("AbatementDateTime = %q, want '2024-01-15'", cond.AbatementDateTime)
	}
}

func TestConditionJSONSerialization(t *testing.T) {
	cond := &Condition{
		ResourceType: "Condition",
		Meta: &Meta{
			Profile: []string{USCoreConditionProfile},
		},
		ClinicalStatus: &CodeableConcept{
			Coding: []Coding{
				{System: SystemConditionClinicalStatus, Code: "active"},
			},
		},
		Code: CodeableConcept{
			Coding: []Coding{
				{System: SystemICD10CM, Code: "E11.9", Display: "Type 2 diabetes"},
			},
			Text: "Type 2 Diabetes Mellitus",
		},
		Subject: &Reference{Reference: "Patient/123"},
	}

	data, err := json.Marshal(cond)
	if err != nil {
		t.Fatalf("Failed to marshal condition: %v", err)
	}

	jsonStr := string(data)
	if !strings.Contains(jsonStr, `"resourceType":"Condition"`) {
		t.Error("JSON missing resourceType")
	}
	if !strings.Contains(jsonStr, USCoreConditionProfile) {
		t.Error("JSON missing US Core Condition profile")
	}
	if !strings.Contains(jsonStr, `"E11.9"`) {
		t.Error("JSON missing ICD-10 code")
	}
}

// =============================================================================
// Coverage Tests
// =============================================================================

func TestUSCoreMapperMapCoverage(t *testing.T) {
	mapper := NewUSCoreMapper()

	planStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	planEnd := time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC)

	event := &events.EligibilityResponseEvent{
		InformationSource: events.Provider{
			NPI:              "1234567890",
			OrganizationName: "Blue Cross Blue Shield",
		},
		Subscriber: events.Patient{
			MRN:        "SUB123",
			FamilyName: "Doe",
			GivenName:  "John",
			Identifiers: events.IdentifierSet{
				Identifiers: []events.Identifier{
					{Type: "MB", Value: "MEM987654"},
				},
			},
		},
		Status:        events.EligibilityStatusActive,
		PlanBeginDate: planStart,
		PlanEndDate:   planEnd,
		Benefits: []events.EligibilityBenefit{
			{
				InformationCode: "1", // Active coverage
				PlanDescription: "PPO Gold Plan",
				InsuranceType:   "PR", // PPO
			},
			{
				InformationCode: "C", // Deductible
				Amount:          1500.00,
				ServiceType:     "30", // Health benefit plan coverage
			},
			{
				InformationCode: "B", // Copay
				Amount:          30.00,
				ServiceType:     "1", // Medical care
			},
			{
				InformationCode: "A", // Coinsurance
				Percent:         20.0,
				ServiceType:     "30",
			},
		},
	}

	coverage := mapper.MapCoverage(event, "Patient/PAT123")

	// Verify resource type and profile
	if coverage.ResourceType != "Coverage" {
		t.Errorf("ResourceType = %q, want 'Coverage'", coverage.ResourceType)
	}
	if len(coverage.Meta.Profile) != 1 || coverage.Meta.Profile[0] != USCoreCoverageProfile {
		t.Errorf("Profile = %v, want [%s]", coverage.Meta.Profile, USCoreCoverageProfile)
	}

	// Verify status
	if coverage.Status != "active" {
		t.Errorf("Status = %q, want 'active'", coverage.Status)
	}

	// Verify beneficiary (from explicit ref)
	if coverage.Beneficiary == nil || coverage.Beneficiary.Reference != "Patient/PAT123" {
		t.Errorf("Beneficiary.Reference = %v, want 'Patient/PAT123'", coverage.Beneficiary)
	}

	// Verify subscriber
	if coverage.Subscriber == nil || coverage.Subscriber.Reference != "Patient/SUB123" {
		t.Errorf("Subscriber.Reference = %v, want 'Patient/SUB123'", coverage.Subscriber)
	}

	// Verify subscriber ID (member ID)
	if coverage.SubscriberId != "MEM987654" {
		t.Errorf("SubscriberId = %q, want 'MEM987654'", coverage.SubscriberId)
	}

	// Verify payor
	if len(coverage.Payor) != 1 {
		t.Fatalf("Payor count = %d, want 1", len(coverage.Payor))
	}
	if coverage.Payor[0].Reference != "Organization/1234567890" {
		t.Errorf("Payor.Reference = %q, want 'Organization/1234567890'", coverage.Payor[0].Reference)
	}
	if coverage.Payor[0].Display != "Blue Cross Blue Shield" {
		t.Errorf("Payor.Display = %q, want 'Blue Cross Blue Shield'", coverage.Payor[0].Display)
	}

	// Verify period
	if coverage.Period == nil || coverage.Period.Start == nil || coverage.Period.End == nil {
		t.Fatal("Period is nil or incomplete")
	}
	if coverage.Period.Start.Year() != 2024 || coverage.Period.Start.Month() != 1 {
		t.Error("Period.Start incorrect")
	}
	if coverage.Period.End.Month() != 12 || coverage.Period.End.Day() != 31 {
		t.Error("Period.End incorrect")
	}

	// Verify class (plan)
	if len(coverage.Class) != 1 {
		t.Fatalf("Class count = %d, want 1", len(coverage.Class))
	}
	if coverage.Class[0].Value != "PPO Gold Plan" {
		t.Errorf("Class.Value = %q, want 'PPO Gold Plan'", coverage.Class[0].Value)
	}

	// Verify type (insurance type)
	if coverage.Type == nil || len(coverage.Type.Coding) == 0 {
		t.Fatal("Type is nil or empty")
	}
	if coverage.Type.Coding[0].Code != "PPO" {
		t.Errorf("Type.Code = %q, want 'PPO'", coverage.Type.Coding[0].Code)
	}

	// Verify cost-to-beneficiary (should have deductible, copay, coinsurance)
	if len(coverage.CostToBeneficiary) != 3 {
		t.Fatalf("CostToBeneficiary count = %d, want 3", len(coverage.CostToBeneficiary))
	}
}

func TestUSCoreMapperMapCoverageStatus(t *testing.T) {
	mapper := NewUSCoreMapper()

	tests := []struct {
		input events.EligibilityStatus
		want  string
	}{
		{events.EligibilityStatusActive, "active"},
		{events.EligibilityStatusInactive, "cancelled"},
		{events.EligibilityStatusRejected, "entered-in-error"},
		{events.EligibilityStatusUnknown, "draft"},
	}

	for _, tt := range tests {
		t.Run(string(tt.input), func(t *testing.T) {
			event := &events.EligibilityResponseEvent{
				Status:     tt.input,
				Subscriber: events.Patient{MRN: "123"},
			}
			coverage := mapper.MapCoverage(event, "")
			if coverage.Status != tt.want {
				t.Errorf("Status for %v = %q, want %q", tt.input, coverage.Status, tt.want)
			}
		})
	}
}

func TestUSCoreMapperMapInsuranceType(t *testing.T) {
	mapper := NewUSCoreMapper()

	tests := []struct {
		insuranceType string
		wantCode      string
	}{
		{"HM", "HMO"},
		{"PR", "PPO"},
		{"PS", "POS"},
		{"EP", "EPO"},
		{"MC", "MCPOL"},
		{"IN", "PUBLICPOL"},
		{"MA", "MCPOL"},
		{"MB", "PUBLICPOL"},
		{"XX", "EHCPOL"}, // Unknown defaults to extended healthcare
	}

	for _, tt := range tests {
		t.Run(tt.insuranceType, func(t *testing.T) {
			event := &events.EligibilityResponseEvent{
				Status:     events.EligibilityStatusActive,
				Subscriber: events.Patient{MRN: "123"},
				Benefits: []events.EligibilityBenefit{
					{InsuranceType: tt.insuranceType},
				},
			}
			coverage := mapper.MapCoverage(event, "")
			if coverage.Type == nil || len(coverage.Type.Coding) == 0 {
				t.Fatal("Type is nil or empty")
			}
			if coverage.Type.Coding[0].Code != tt.wantCode {
				t.Errorf("Type for %q = %q, want %q", tt.insuranceType, coverage.Type.Coding[0].Code, tt.wantCode)
			}
		})
	}
}

func TestUSCoreMapperMapCoverageBeneficiaryFallback(t *testing.T) {
	mapper := NewUSCoreMapper()

	// Test fallback to dependent when explicit ref not provided
	event := &events.EligibilityResponseEvent{
		Subscriber: events.Patient{MRN: "SUB123"},
		Dependent:  &events.Patient{MRN: "DEP456"},
		Status:     events.EligibilityStatusActive,
	}

	coverage := mapper.MapCoverage(event, "")

	// Should use dependent's MRN for beneficiary
	if coverage.Beneficiary == nil || coverage.Beneficiary.Reference != "Patient/DEP456" {
		t.Errorf("Beneficiary.Reference = %v, want 'Patient/DEP456'", coverage.Beneficiary)
	}

	// Test fallback to subscriber when no dependent
	event2 := &events.EligibilityResponseEvent{
		Subscriber: events.Patient{MRN: "SUB789"},
		Status:     events.EligibilityStatusActive,
	}

	coverage2 := mapper.MapCoverage(event2, "")

	if coverage2.Beneficiary == nil || coverage2.Beneficiary.Reference != "Patient/SUB789" {
		t.Errorf("Beneficiary.Reference = %v, want 'Patient/SUB789'", coverage2.Beneficiary)
	}
}

func TestCoverageJSONSerialization(t *testing.T) {
	coverage := &Coverage{
		ResourceType: "Coverage",
		Meta: &Meta{
			Profile: []string{USCoreCoverageProfile},
		},
		Status:       "active",
		SubscriberId: "MEM123456",
		Beneficiary: &Reference{
			Reference: "Patient/123",
		},
		Payor: []Reference{
			{
				Reference: "Organization/BCBS",
				Display:   "Blue Cross Blue Shield",
			},
		},
		CostToBeneficiary: []CostToBeneficiary{
			{
				Type: &CodeableConcept{
					Coding: []Coding{
						{System: SystemCopayType, Code: "deductible"},
					},
				},
				ValueMoney: &Money{Value: 1500.00, Currency: "USD"},
			},
		},
	}

	data, err := json.Marshal(coverage)
	if err != nil {
		t.Fatalf("Failed to marshal coverage: %v", err)
	}

	jsonStr := string(data)
	if !strings.Contains(jsonStr, `"resourceType":"Coverage"`) {
		t.Error("JSON missing resourceType")
	}
	if !strings.Contains(jsonStr, USCoreCoverageProfile) {
		t.Error("JSON missing US Core Coverage profile")
	}
	if !strings.Contains(jsonStr, `"subscriberId":"MEM123456"`) {
		t.Error("JSON missing subscriberId")
	}
	if !strings.Contains(jsonStr, `"deductible"`) {
		t.Error("JSON missing deductible")
	}
}

// --- Claim Tests ---

func TestUSCoreMapperMapClaim(t *testing.T) {
	mapper := NewUSCoreMapper()

	serviceDate := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	event := &events.ClaimSubmittedEvent{
		EventMeta: events.EventMeta{
			Type:      events.EventClaimSubmitted,
			Timestamp: serviceDate,
		},
		Patient: events.Patient{
			MRN:        "PAT123",
			FamilyName: "Smith",
			GivenName:  "Jane",
		},
		BillingProvider: events.Provider{
			NPI:              "1234567890",
			OrganizationName: "Acme Medical Group",
		},
		RenderingProvider: &events.Provider{
			NPI:        "9876543210",
			FamilyName: "Johnson",
			GivenName:  "Robert",
		},
		Payer: events.Provider{
			NPI:              "5555555555",
			OrganizationName: "Blue Cross",
		},
		Subscriber: events.Patient{
			MRN: "SUB456",
			Identifiers: events.IdentifierSet{
				Identifiers: []events.Identifier{
					{Type: "MB", Value: "MEM789"},
				},
			},
		},
		Claim: events.Claim{
			ControlNumber:  "CLM-001",
			TotalAmount:    250.00,
			PlaceOfService: "11",
			ServiceDate:    serviceDate,
			DiagnosisCodes: []string{"J06.9", "R05.9"},
			ServiceLines: []events.ServiceLine{
				{
					LineNumber:        1,
					ProcedureCode:     "99213",
					Modifiers:         []string{"25"},
					ChargeAmount:      150.00,
					Units:             1,
					UnitType:          "UN",
					ServiceDate:       serviceDate,
					DiagnosisPointers: []int{1, 2},
				},
				{
					LineNumber:    2,
					ProcedureCode: "87880",
					ChargeAmount:  100.00,
					Units:         1,
					ServiceDate:   serviceDate,
				},
			},
		},
	}

	claim := mapper.MapClaim(event, "claim")

	// Verify resource type
	if claim.ResourceType != "Claim" {
		t.Errorf("ResourceType = %q, want 'Claim'", claim.ResourceType)
	}

	// Verify status
	if claim.Status != "active" {
		t.Errorf("Status = %q, want 'active'", claim.Status)
	}

	// Verify use
	if claim.Use != "claim" {
		t.Errorf("Use = %q, want 'claim'", claim.Use)
	}

	// Verify type (professional)
	if len(claim.Type.Coding) == 0 || claim.Type.Coding[0].Code != "professional" {
		t.Errorf("Type = %v, want 'professional'", claim.Type)
	}

	// Verify patient reference
	if claim.Patient == nil || claim.Patient.Reference != "Patient/PAT123" {
		t.Errorf("Patient = %v, want 'Patient/PAT123'", claim.Patient)
	}

	// Verify provider reference
	if claim.Provider == nil || claim.Provider.Reference != "Organization/1234567890" {
		t.Errorf("Provider = %v, want 'Organization/1234567890'", claim.Provider)
	}

	// Verify insurer reference
	if claim.Insurer == nil || claim.Insurer.Reference != "Organization/5555555555" {
		t.Errorf("Insurer = %v, want 'Organization/5555555555'", claim.Insurer)
	}

	// Verify identifier
	if len(claim.Identifier) == 0 || claim.Identifier[0].Value != "CLM-001" {
		t.Errorf("Identifier = %v, want 'CLM-001'", claim.Identifier)
	}

	// Verify total
	if claim.Total == nil || claim.Total.Value != 250.00 {
		t.Errorf("Total = %v, want 250.00", claim.Total)
	}

	// Verify diagnosis codes
	if len(claim.Diagnosis) != 2 {
		t.Fatalf("Diagnosis count = %d, want 2", len(claim.Diagnosis))
	}
	if claim.Diagnosis[0].DiagnosisCodeable.Coding[0].Code != "J06.9" {
		t.Errorf("Diagnosis[0] = %v, want 'J06.9'", claim.Diagnosis[0])
	}
	// First diagnosis should be principal
	if len(claim.Diagnosis[0].Type) == 0 || claim.Diagnosis[0].Type[0].Coding[0].Code != "principal" {
		t.Error("First diagnosis should be marked as principal")
	}

	// Verify care team
	if len(claim.CareTeam) != 2 {
		t.Fatalf("CareTeam count = %d, want 2", len(claim.CareTeam))
	}
	if claim.CareTeam[0].Role.Coding[0].Code != "primary" {
		t.Errorf("CareTeam[0].Role = %v, want 'primary'", claim.CareTeam[0].Role)
	}
	if claim.CareTeam[1].Role.Coding[0].Code != "rendering" {
		t.Errorf("CareTeam[1].Role = %v, want 'rendering'", claim.CareTeam[1].Role)
	}

	// Verify service line items
	if len(claim.Item) != 2 {
		t.Fatalf("Item count = %d, want 2", len(claim.Item))
	}
	if claim.Item[0].ProductOrService.Coding[0].Code != "99213" {
		t.Errorf("Item[0].ProductOrService = %v, want '99213'", claim.Item[0].ProductOrService)
	}
	if len(claim.Item[0].Modifier) != 1 || claim.Item[0].Modifier[0].Coding[0].Code != "25" {
		t.Errorf("Item[0].Modifier = %v, want '25'", claim.Item[0].Modifier)
	}
	if claim.Item[0].Net.Value != 150.00 {
		t.Errorf("Item[0].Net = %v, want 150.00", claim.Item[0].Net)
	}

	// Verify insurance
	if len(claim.Insurance) == 0 || !claim.Insurance[0].Focal {
		t.Error("Insurance should have focal=true")
	}
	if claim.Insurance[0].Coverage == nil || claim.Insurance[0].Coverage.Reference != "Coverage/MEM789" {
		t.Errorf("Insurance.Coverage = %v, want 'Coverage/MEM789'", claim.Insurance[0].Coverage)
	}
}

func TestUSCoreMapperMapClaimPreauthorization(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.ClaimSubmittedEvent{
		Patient: events.Patient{MRN: "PAT123"},
		BillingProvider: events.Provider{
			NPI:              "1234567890",
			OrganizationName: "Test Provider",
		},
		Subscriber: events.Patient{MRN: "SUB123"},
		Claim: events.Claim{
			TotalAmount: 1000.00,
		},
	}

	claim := mapper.MapClaim(event, "preauthorization")

	// Verify use is preauthorization
	if claim.Use != "preauthorization" {
		t.Errorf("Use = %q, want 'preauthorization'", claim.Use)
	}

	// Verify Da Vinci PAS profile
	if claim.Meta == nil || len(claim.Meta.Profile) == 0 {
		t.Fatal("Expected Da Vinci PAS profile")
	}
	if claim.Meta.Profile[0] != DaVinciPASClaimProfile {
		t.Errorf("Profile = %v, want Da Vinci PAS profile", claim.Meta.Profile)
	}
}

func TestUSCoreMapperMapClaimDefaultUse(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.ClaimSubmittedEvent{
		Patient:         events.Patient{MRN: "PAT123"},
		BillingProvider: events.Provider{NPI: "1234567890"},
		Subscriber:      events.Patient{MRN: "SUB123"},
	}

	// Empty use should default to "claim"
	claim := mapper.MapClaim(event, "")

	if claim.Use != "claim" {
		t.Errorf("Use = %q, want 'claim' as default", claim.Use)
	}

	// Standard claim should not have Da Vinci profile
	if claim.Meta != nil && len(claim.Meta.Profile) > 0 {
		t.Error("Standard claim should not have Da Vinci profile")
	}
}

func TestUSCoreMapperMapClaimNil(t *testing.T) {
	mapper := NewUSCoreMapper()

	claim := mapper.MapClaim(nil, "claim")
	if claim != nil {
		t.Error("MapClaim(nil) should return nil")
	}
}

func TestClaimJSONSerialization(t *testing.T) {
	claim := &Claim{
		ResourceType: "Claim",
		Status:       "active",
		Use:          "claim",
		Type: CodeableConcept{
			Coding: []Coding{{System: SystemClaimType, Code: "professional"}},
		},
		Patient:  &Reference{Reference: "Patient/123"},
		Provider: &Reference{Reference: "Organization/456"},
		Insurance: []ClaimInsurance{
			{Sequence: 1, Focal: true, Coverage: &Reference{Reference: "Coverage/789"}},
		},
		Total: &Money{Value: 500.00, Currency: "USD"},
	}

	data, err := json.Marshal(claim)
	if err != nil {
		t.Fatalf("Failed to marshal claim: %v", err)
	}

	jsonStr := string(data)
	if !strings.Contains(jsonStr, `"resourceType":"Claim"`) {
		t.Error("JSON missing resourceType")
	}
	if !strings.Contains(jsonStr, `"use":"claim"`) {
		t.Error("JSON missing use")
	}
	if !strings.Contains(jsonStr, `"professional"`) {
		t.Error("JSON missing claim type")
	}
}

// --- ExplanationOfBenefit Tests ---

func TestUSCoreMapperMapExplanationOfBenefit(t *testing.T) {
	mapper := NewUSCoreMapper()

	checkDate := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
	event := &events.ClaimAdjudicatedEvent{
		EventMeta: events.EventMeta{
			Type:      events.EventClaimAdjudicated,
			Timestamp: checkDate,
		},
		Payer: events.Provider{
			NPI:              "5555555555",
			OrganizationName: "Blue Cross",
		},
		Payee: events.Provider{
			NPI:              "1234567890",
			OrganizationName: "Acme Medical Group",
		},
		CheckNumber: "CHK123456",
		CheckDate:   checkDate,
		TotalPaid:   180.00,
		Payment: events.ClaimPayment{
			ClaimID:       "CLM-001",
			PayerClaimID:  "PCN-999",
			Status:        "Processed",
			ChargedAmount: 250.00,
			PaidAmount:    180.00,
			Adjustments: []events.ClaimAdjustment{
				{Group: "CO", ReasonCode: "45", Amount: 50.00},
				{Group: "PR", ReasonCode: "1", Amount: 20.00},
			},
			ServiceLinePayments: []events.ServiceLinePayment{
				{
					ProcedureCode: "99213",
					ChargedAmount: 150.00,
					PaidAmount:    120.00,
					Adjustments: []events.ClaimAdjustment{
						{Group: "CO", ReasonCode: "45", Amount: 20.00},
						{Group: "PR", ReasonCode: "1", Amount: 10.00},
					},
				},
				{
					ProcedureCode: "87880",
					ChargedAmount: 100.00,
					PaidAmount:    60.00,
					Adjustments: []events.ClaimAdjustment{
						{Group: "CO", ReasonCode: "45", Amount: 30.00},
						{Group: "PR", ReasonCode: "1", Amount: 10.00},
					},
				},
			},
		},
	}

	eob := mapper.MapExplanationOfBenefit(event)

	// Verify resource type
	if eob.ResourceType != "ExplanationOfBenefit" {
		t.Errorf("ResourceType = %q, want 'ExplanationOfBenefit'", eob.ResourceType)
	}

	// Verify PDex profile
	if eob.Meta == nil || len(eob.Meta.Profile) == 0 {
		t.Fatal("Expected PDex profile")
	}
	if eob.Meta.Profile[0] != PDexEOBProfile {
		t.Errorf("Profile = %v, want PDex profile", eob.Meta.Profile)
	}

	// Verify status
	if eob.Status != "active" {
		t.Errorf("Status = %q, want 'active'", eob.Status)
	}

	// Verify outcome
	if eob.Outcome != "complete" {
		t.Errorf("Outcome = %q, want 'complete'", eob.Outcome)
	}

	// Verify insurer
	if eob.Insurer == nil || eob.Insurer.Reference != "Organization/5555555555" {
		t.Errorf("Insurer = %v, want 'Organization/5555555555'", eob.Insurer)
	}

	// Verify provider
	if eob.Provider == nil || eob.Provider.Reference != "Organization/1234567890" {
		t.Errorf("Provider = %v, want 'Organization/1234567890'", eob.Provider)
	}

	// Verify identifiers
	if len(eob.Identifier) != 2 {
		t.Fatalf("Identifier count = %d, want 2", len(eob.Identifier))
	}

	// Verify service line items
	if len(eob.Item) != 2 {
		t.Fatalf("Item count = %d, want 2", len(eob.Item))
	}
	if eob.Item[0].ProductOrService.Coding[0].Code != "99213" {
		t.Errorf("Item[0].ProductOrService = %v, want '99213'", eob.Item[0].ProductOrService)
	}

	// Verify line-level adjudication
	if len(eob.Item[0].Adjudication) < 2 {
		t.Fatalf("Item[0].Adjudication count = %d, want at least 2", len(eob.Item[0].Adjudication))
	}

	// Verify totals
	if len(eob.Total) < 2 {
		t.Fatalf("Total count = %d, want at least 2", len(eob.Total))
	}
	// Find benefit total
	var foundBenefit bool
	for _, total := range eob.Total {
		if len(total.Category.Coding) > 0 && total.Category.Coding[0].Code == "benefit" {
			foundBenefit = true
			if total.Amount.Value != 180.00 {
				t.Errorf("Benefit total = %v, want 180.00", total.Amount.Value)
			}
		}
	}
	if !foundBenefit {
		t.Error("Missing benefit total")
	}

	// Verify payment
	if eob.Payment == nil {
		t.Fatal("Payment should not be nil")
	}
	if eob.Payment.Amount.Value != 180.00 {
		t.Errorf("Payment.Amount = %v, want 180.00", eob.Payment.Amount)
	}
	if eob.Payment.Date != "2024-02-01" {
		t.Errorf("Payment.Date = %q, want '2024-02-01'", eob.Payment.Date)
	}
	if eob.Payment.Identifier == nil || eob.Payment.Identifier.Value != "CHK123456" {
		t.Errorf("Payment.Identifier = %v, want 'CHK123456'", eob.Payment.Identifier)
	}
}

func TestUSCoreMapperMapEOBOutcome(t *testing.T) {
	mapper := NewUSCoreMapper()

	tests := []struct {
		status   string
		expected string
	}{
		{"Processed", "complete"},
		{"Paid", "complete"},
		{"Complete", "complete"},
		{"Denied", "error"},
		{"Pending", "queued"},
		{"In Process", "queued"},
		{"Partial", "partial"},
		{"Unknown", "complete"}, // default
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			event := &events.ClaimAdjudicatedEvent{
				Payer: events.Provider{OrganizationName: "Test"},
				Payee: events.Provider{OrganizationName: "Test"},
				Payment: events.ClaimPayment{
					Status: tt.status,
				},
			}

			eob := mapper.MapExplanationOfBenefit(event)
			if eob.Outcome != tt.expected {
				t.Errorf("Outcome for %q = %q, want %q", tt.status, eob.Outcome, tt.expected)
			}
		})
	}
}

func TestUSCoreMapperMapEOBPatientResponsibility(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.ClaimAdjudicatedEvent{
		Payer: events.Provider{OrganizationName: "Test Payer"},
		Payee: events.Provider{OrganizationName: "Test Payee"},
		Payment: events.ClaimPayment{
			Status:        "Processed",
			ChargedAmount: 500.00,
			PaidAmount:    350.00,
			Adjustments: []events.ClaimAdjustment{
				{Group: "PR", ReasonCode: "1", Amount: 50.00},  // Deductible
				{Group: "PR", ReasonCode: "2", Amount: 25.00},  // Copay
				{Group: "CO", ReasonCode: "45", Amount: 75.00}, // Contractual
			},
		},
	}

	eob := mapper.MapExplanationOfBenefit(event)

	// Find patient responsibility total (PR adjustments sum to 75.00)
	var patientResponsibility float64
	for _, total := range eob.Total {
		if len(total.Category.Coding) > 0 && total.Category.Coding[0].Code == "deductible" {
			patientResponsibility = total.Amount.Value
		}
	}

	if patientResponsibility != 75.00 {
		t.Errorf("Patient responsibility = %v, want 75.00", patientResponsibility)
	}
}

func TestUSCoreMapperMapEOBNil(t *testing.T) {
	mapper := NewUSCoreMapper()

	eob := mapper.MapExplanationOfBenefit(nil)
	if eob != nil {
		t.Error("MapExplanationOfBenefit(nil) should return nil")
	}
}

func TestEOBJSONSerialization(t *testing.T) {
	eob := &ExplanationOfBenefit{
		ResourceType: "ExplanationOfBenefit",
		Meta: &Meta{
			Profile: []string{PDexEOBProfile},
		},
		Status:   "active",
		Use:      "claim",
		Outcome:  "complete",
		Type:     CodeableConcept{Coding: []Coding{{System: SystemClaimType, Code: "professional"}}},
		Insurer:  &Reference{Reference: "Organization/123"},
		Provider: &Reference{Reference: "Organization/456"},
		Insurance: []EOBInsurance{
			{Focal: true, Coverage: &Reference{Display: "Test Coverage"}},
		},
		Total: []EOBTotal{
			{
				Category: CodeableConcept{Coding: []Coding{{Code: "benefit"}}},
				Amount:   Money{Value: 100.00, Currency: "USD"},
			},
		},
		Payment: &EOBPayment{
			Amount: &Money{Value: 100.00, Currency: "USD"},
		},
	}

	data, err := json.Marshal(eob)
	if err != nil {
		t.Fatalf("Failed to marshal EOB: %v", err)
	}

	jsonStr := string(data)
	if !strings.Contains(jsonStr, `"resourceType":"ExplanationOfBenefit"`) {
		t.Error("JSON missing resourceType")
	}
	if !strings.Contains(jsonStr, PDexEOBProfile) {
		t.Error("JSON missing PDex profile")
	}
	if !strings.Contains(jsonStr, `"outcome":"complete"`) {
		t.Error("JSON missing outcome")
	}
	if !strings.Contains(jsonStr, `"benefit"`) {
		t.Error("JSON missing benefit category")
	}
}

func TestNilInputHandlingClaimAndEOB(t *testing.T) {
	mapper := NewUSCoreMapper()

	if mapper.MapClaim(nil, "claim") != nil {
		t.Error("MapClaim(nil) should return nil")
	}

	if mapper.MapExplanationOfBenefit(nil) != nil {
		t.Error("MapExplanationOfBenefit(nil) should return nil")
	}
}

// --- CoverageEligibilityResponse Tests ---

func TestUSCoreMapperMapCoverageEligibilityResponse(t *testing.T) {
	mapper := NewUSCoreMapper()

	planStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	planEnd := time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC)
	event := &events.EligibilityResponseEvent{
		EventMeta: events.EventMeta{
			Type:      events.EventEligibilityResponse,
			Timestamp: time.Now(),
		},
		InformationSource: events.Provider{
			NPI:              "5555555555",
			OrganizationName: "Blue Cross Blue Shield",
		},
		InformationReceiver: events.Provider{
			NPI:              "1234567890",
			OrganizationName: "Acme Medical Group",
		},
		Subscriber: events.Patient{
			MRN:        "SUB123",
			FamilyName: "Smith",
			GivenName:  "John",
			Identifiers: events.IdentifierSet{
				Identifiers: []events.Identifier{
					{Type: "MB", Value: "MEM456789"},
				},
			},
		},
		Status:        events.EligibilityStatusActive,
		TraceNumber:   "TRN-001",
		PlanBeginDate: planStart,
		PlanEndDate:   planEnd,
		Benefits: []events.EligibilityBenefit{
			{
				InformationCode:            "1",
				InformationCodeDescription: "Active Coverage",
				ServiceType:                "30",
				ServiceTypeDescription:     "Health Benefit Plan Coverage",
				InNetworkIndicator:         "Y",
				CoverageLevel:              "IND",
				PlanDescription:            "PPO Gold Plan",
			},
			{
				InformationCode:            "C",
				InformationCodeDescription: "Deductible",
				ServiceType:                "30",
				ServiceTypeDescription:     "Health Benefit Plan Coverage",
				InNetworkIndicator:         "Y",
				CoverageLevel:              "IND",
				Amount:                     1500.00,
			},
			{
				InformationCode:            "B",
				InformationCodeDescription: "Co-Payment",
				ServiceType:                "1",
				ServiceTypeDescription:     "Medical Care",
				InNetworkIndicator:         "Y",
				Amount:                     30.00,
			},
			{
				InformationCode:            "A",
				InformationCodeDescription: "Coinsurance",
				ServiceType:                "47",
				ServiceTypeDescription:     "Hospital - Inpatient",
				InNetworkIndicator:         "Y",
				Percent:                    20.00,
			},
		},
	}

	cer := mapper.MapCoverageEligibilityResponse(event, "")

	// Verify resource type
	if cer.ResourceType != "CoverageEligibilityResponse" {
		t.Errorf("ResourceType = %q, want 'CoverageEligibilityResponse'", cer.ResourceType)
	}

	// Verify status
	if cer.Status != "active" {
		t.Errorf("Status = %q, want 'active'", cer.Status)
	}

	// Verify purpose
	if len(cer.Purpose) == 0 || cer.Purpose[0] != "benefits" {
		t.Errorf("Purpose = %v, want ['benefits']", cer.Purpose)
	}

	// Verify outcome
	if cer.Outcome != "complete" {
		t.Errorf("Outcome = %q, want 'complete'", cer.Outcome)
	}

	// Verify patient (should use subscriber since no explicit ref)
	if cer.Patient == nil || cer.Patient.Reference != "Patient/SUB123" {
		t.Errorf("Patient = %v, want 'Patient/SUB123'", cer.Patient)
	}

	// Verify insurer
	if cer.Insurer == nil || cer.Insurer.Reference != "Organization/5555555555" {
		t.Errorf("Insurer = %v, want 'Organization/5555555555'", cer.Insurer)
	}

	// Verify requestor
	if cer.Requestor == nil || cer.Requestor.Reference != "Organization/1234567890" {
		t.Errorf("Requestor = %v, want 'Organization/1234567890'", cer.Requestor)
	}

	// Verify identifier (trace number)
	if len(cer.Identifier) == 0 || cer.Identifier[0].Value != "TRN-001" {
		t.Errorf("Identifier = %v, want trace number 'TRN-001'", cer.Identifier)
	}

	// Verify insurance section
	if len(cer.Insurance) == 0 {
		t.Fatal("Insurance section should not be empty")
	}

	insurance := cer.Insurance[0]

	// Verify inforce
	if !insurance.Inforce {
		t.Error("Insurance.Inforce should be true for active coverage")
	}

	// Verify coverage reference
	if insurance.Coverage == nil || insurance.Coverage.Reference != "Coverage/MEM456789" {
		t.Errorf("Insurance.Coverage = %v, want 'Coverage/MEM456789'", insurance.Coverage)
	}

	// Verify benefit period
	if insurance.BenefitPeriod == nil {
		t.Fatal("BenefitPeriod should not be nil")
	}
	if insurance.BenefitPeriod.Start == nil || insurance.BenefitPeriod.Start.Year() != 2024 {
		t.Errorf("BenefitPeriod.Start = %v, want 2024-01-01", insurance.BenefitPeriod.Start)
	}

	// Verify items (should have 3 unique service type + network combinations)
	if len(insurance.Item) < 3 {
		t.Errorf("Insurance.Item count = %d, want at least 3", len(insurance.Item))
	}
}

func TestUSCoreMapperMapCERWithDependent(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.EligibilityResponseEvent{
		InformationSource: events.Provider{OrganizationName: "Test Payer"},
		Subscriber: events.Patient{
			MRN:        "SUB123",
			FamilyName: "Parent",
			GivenName:  "John",
		},
		Dependent: &events.Patient{
			MRN:        "DEP456",
			FamilyName: "Child",
			GivenName:  "Jane",
		},
		Status: events.EligibilityStatusActive,
	}

	cer := mapper.MapCoverageEligibilityResponse(event, "")

	// Verify dependent is used as patient
	if cer.Patient == nil || cer.Patient.Reference != "Patient/DEP456" {
		t.Errorf("Patient = %v, want 'Patient/DEP456' (dependent)", cer.Patient)
	}
}

func TestUSCoreMapperMapCERExplicitPatientRef(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.EligibilityResponseEvent{
		InformationSource: events.Provider{OrganizationName: "Test Payer"},
		Subscriber:        events.Patient{MRN: "SUB123"},
		Status:            events.EligibilityStatusActive,
	}

	// Explicit patient ref should take precedence
	cer := mapper.MapCoverageEligibilityResponse(event, "Patient/explicit-ref")

	if cer.Patient == nil || cer.Patient.Reference != "Patient/explicit-ref" {
		t.Errorf("Patient = %v, want 'Patient/explicit-ref'", cer.Patient)
	}
}

func TestUSCoreMapperMapCEROutcome(t *testing.T) {
	mapper := NewUSCoreMapper()

	tests := []struct {
		status   events.EligibilityStatus
		hasError bool
		expected string
	}{
		{events.EligibilityStatusActive, false, "complete"},
		{events.EligibilityStatusInactive, false, "complete"},
		{events.EligibilityStatusRejected, false, "error"},
		{events.EligibilityStatusActive, true, "error"}, // Error takes precedence
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			event := &events.EligibilityResponseEvent{
				InformationSource: events.Provider{OrganizationName: "Test"},
				Subscriber:        events.Patient{MRN: "SUB123"},
				Status:            tt.status,
			}
			if tt.hasError {
				event.Errors = []events.EligibilityValidationError{
					{Code: "TEST", Message: "Test error"},
				}
			}

			cer := mapper.MapCoverageEligibilityResponse(event, "")
			if cer.Outcome != tt.expected {
				t.Errorf("Outcome for %s (error=%v) = %q, want %q",
					tt.status, tt.hasError, cer.Outcome, tt.expected)
			}
		})
	}
}

func TestUSCoreMapperMapCERNetworkIndicator(t *testing.T) {
	mapper := NewUSCoreMapper()

	tests := []struct {
		indicator string
		expected  string
	}{
		{"Y", "in"},
		{"N", "out"},
		{"W", "other"},
		{"", "other"},
	}

	for _, tt := range tests {
		t.Run(tt.indicator, func(t *testing.T) {
			event := &events.EligibilityResponseEvent{
				InformationSource: events.Provider{OrganizationName: "Test"},
				Subscriber:        events.Patient{MRN: "SUB123"},
				Status:            events.EligibilityStatusActive,
				Benefits: []events.EligibilityBenefit{
					{
						InformationCode:    "1",
						ServiceType:        "30",
						InNetworkIndicator: tt.indicator,
					},
				},
			}

			cer := mapper.MapCoverageEligibilityResponse(event, "")
			if len(cer.Insurance) == 0 || len(cer.Insurance[0].Item) == 0 {
				t.Fatal("Expected insurance with items")
			}

			item := cer.Insurance[0].Item[0]
			if item.Network != nil && len(item.Network.Coding) > 0 {
				if item.Network.Coding[0].Code != tt.expected {
					t.Errorf("Network.Code = %q, want %q",
						item.Network.Coding[0].Code, tt.expected)
				}
			} else if tt.indicator != "" {
				t.Error("Expected network to be set")
			}
		})
	}
}

func TestUSCoreMapperMapCERBenefitTypes(t *testing.T) {
	mapper := NewUSCoreMapper()

	tests := []struct {
		infoCode     string
		amount       float64
		percent      float64
		expectedType string
	}{
		{"C", 1500.00, 0, "deductible"}, // Deductible amount
		{"B", 30.00, 0, "copay"},        // Copay amount
		{"A", 0, 20.00, "coinsurance"},  // Coinsurance percent
		{"1", 0, 0, "benefit"},          // Active coverage
		{"G", 0, 0, "limit"},            // Quantity limit
	}

	for _, tt := range tests {
		t.Run(tt.infoCode, func(t *testing.T) {
			event := &events.EligibilityResponseEvent{
				InformationSource: events.Provider{OrganizationName: "Test"},
				Subscriber:        events.Patient{MRN: "SUB123"},
				Status:            events.EligibilityStatusActive,
				Benefits: []events.EligibilityBenefit{
					{
						InformationCode:            tt.infoCode,
						InformationCodeDescription: "Test",
						ServiceType:                "30",
						Amount:                     tt.amount,
						Percent:                    tt.percent,
						Quantity:                   10, // For quantity tests
					},
				},
			}

			cer := mapper.MapCoverageEligibilityResponse(event, "")
			if len(cer.Insurance) == 0 || len(cer.Insurance[0].Item) == 0 {
				t.Fatal("Expected insurance with items")
			}

			item := cer.Insurance[0].Item[0]
			if len(item.Benefit) == 0 {
				// Some codes may not produce benefits (info-only)
				if tt.infoCode == "1" {
					// Active coverage should produce a benefit
					t.Fatal("Expected benefit for active coverage")
				}
				return
			}

			benefit := item.Benefit[0]
			if len(benefit.Type.Coding) == 0 || benefit.Type.Coding[0].Code != tt.expectedType {
				t.Errorf("Benefit.Type = %v, want code %q", benefit.Type.Coding, tt.expectedType)
			}
		})
	}
}

func TestUSCoreMapperMapCERWithErrors(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.EligibilityResponseEvent{
		InformationSource: events.Provider{OrganizationName: "Test"},
		Subscriber:        events.Patient{MRN: "SUB123"},
		Status:            events.EligibilityStatusRejected,
		Errors: []events.EligibilityValidationError{
			{Code: "72", Message: "Invalid/Missing Subscriber ID"},
			{Code: "73", Message: "Invalid/Missing Dependent ID"},
		},
	}

	cer := mapper.MapCoverageEligibilityResponse(event, "")

	// Verify outcome is error
	if cer.Outcome != "error" {
		t.Errorf("Outcome = %q, want 'error'", cer.Outcome)
	}

	// Verify errors are mapped
	if len(cer.Error) != 2 {
		t.Fatalf("Error count = %d, want 2", len(cer.Error))
	}

	if cer.Error[0].Code.Coding[0].Code != "72" {
		t.Errorf("Error[0].Code = %q, want '72'", cer.Error[0].Code.Coding[0].Code)
	}
}

func TestUSCoreMapperMapCERNil(t *testing.T) {
	mapper := NewUSCoreMapper()

	cer := mapper.MapCoverageEligibilityResponse(nil, "")
	if cer != nil {
		t.Error("MapCoverageEligibilityResponse(nil) should return nil")
	}
}

func TestCoverageEligibilityResponseJSONSerialization(t *testing.T) {
	cer := &CoverageEligibilityResponse{
		ResourceType: "CoverageEligibilityResponse",
		Status:       "active",
		Purpose:      []string{"benefits"},
		Patient:      &Reference{Reference: "Patient/123"},
		Insurer:      &Reference{Reference: "Organization/456"},
		Outcome:      "complete",
		Insurance: []CERInsurance{
			{
				Coverage: &Reference{Reference: "Coverage/789"},
				Inforce:  true,
				Item: []CERItem{
					{
						Name: "Medical Care",
						Benefit: []CERBenefit{
							{
								Type:         CodeableConcept{Coding: []Coding{{Code: "deductible"}}},
								AllowedMoney: &Money{Value: 1500.00, Currency: "USD"},
							},
						},
					},
				},
			},
		},
	}

	data, err := json.Marshal(cer)
	if err != nil {
		t.Fatalf("Failed to marshal CoverageEligibilityResponse: %v", err)
	}

	jsonStr := string(data)
	if !strings.Contains(jsonStr, `"resourceType":"CoverageEligibilityResponse"`) {
		t.Error("JSON missing resourceType")
	}
	if !strings.Contains(jsonStr, `"outcome":"complete"`) {
		t.Error("JSON missing outcome")
	}
	if !strings.Contains(jsonStr, `"inforce":true`) {
		t.Error("JSON missing inforce")
	}
	if !strings.Contains(jsonStr, `"deductible"`) {
		t.Error("JSON missing deductible benefit type")
	}
}

// ========== Procedure Tests ==========

func TestMapProcedure_Basic(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.ProcedureEvent{
		EventMeta: events.NewEventMeta(events.EventProcedure, "test", events.FormatCDA),
		Procedure: events.Procedure{
			Name:   "Appendectomy",
			Code:   "80146002",
			Status: "completed",
		},
		PerformedDate: "2024-01-15",
	}
	event.ID = "proc-123"

	proc := mapper.MapProcedure(event, "Patient/test-patient")

	if proc == nil {
		t.Fatal("Expected non-nil Procedure")
	}

	// Verify resource type and profile
	if proc.ResourceType != "Procedure" {
		t.Errorf("ResourceType = %q, want 'Procedure'", proc.ResourceType)
	}
	if len(proc.Meta.Profile) != 1 || proc.Meta.Profile[0] != USCoreProcedureProfile {
		t.Errorf("Profile = %v, want [%s]", proc.Meta.Profile, USCoreProcedureProfile)
	}

	// Verify required elements
	if proc.Status != "completed" {
		t.Errorf("Status = %q, want 'completed'", proc.Status)
	}
	if proc.Subject == nil || proc.Subject.Reference != "Patient/test-patient" {
		t.Errorf("Subject reference incorrect")
	}

	// Verify code
	if len(proc.Code.Coding) != 1 {
		t.Errorf("Expected 1 coding, got %d", len(proc.Code.Coding))
	}
	if proc.Code.Coding[0].Code != "80146002" {
		t.Errorf("Code = %q, want '80146002'", proc.Code.Coding[0].Code)
	}
	if proc.Code.Coding[0].Display != "Appendectomy" {
		t.Errorf("Display = %q, want 'Appendectomy'", proc.Code.Coding[0].Display)
	}

	// Verify performed date
	if proc.PerformedDateTime != "2024-01-15" {
		t.Errorf("PerformedDateTime = %q, want '2024-01-15'", proc.PerformedDateTime)
	}
}

func TestMapProcedure_WithPerformer(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.ProcedureEvent{
		EventMeta: events.NewEventMeta(events.EventProcedure, "test", events.FormatCDA),
		Procedure: events.Procedure{
			Name:   "Colonoscopy",
			Code:   "73761001",
			Status: "completed",
		},
		PerformedDate: "2024-02-20",
		Performer: &events.Provider{
			NPI:        "1234567890",
			FamilyName: "Smith",
			GivenName:  "Jane",
		},
	}

	proc := mapper.MapProcedure(event, "Patient/123")

	if len(proc.Performer) != 1 {
		t.Fatalf("Expected 1 performer, got %d", len(proc.Performer))
	}

	performer := proc.Performer[0]
	if performer.Actor == nil {
		t.Fatal("Expected non-nil performer actor")
	}
	if performer.Actor.Reference != "Practitioner/1234567890" {
		t.Errorf("Performer reference = %q, want 'Practitioner/1234567890'", performer.Actor.Reference)
	}
	if performer.Actor.Display != "Smith, Jane" {
		t.Errorf("Performer display = %q, want 'Smith, Jane'", performer.Actor.Display)
	}
}

func TestMapProcedure_WithEncounter(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.ProcedureEvent{
		EventMeta: events.NewEventMeta(events.EventProcedure, "test", events.FormatCDA),
		Procedure: events.Procedure{
			Name: "Blood Draw",
			Code: "82078001",
		},
		Encounter: &events.Encounter{
			ID: "enc-456",
		},
	}

	proc := mapper.MapProcedure(event, "Patient/123")

	if proc.Encounter == nil {
		t.Fatal("Expected encounter reference")
	}
	if proc.Encounter.Reference != "Encounter/enc-456" {
		t.Errorf("Encounter reference = %q, want 'Encounter/enc-456'", proc.Encounter.Reference)
	}
}

func TestMapProcedure_WithLocation(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.ProcedureEvent{
		EventMeta: events.NewEventMeta(events.EventProcedure, "test", events.FormatCDA),
		Procedure: events.Procedure{
			Name: "X-Ray",
			Code: "168537006",
		},
		Location: &events.Location{
			Facility: "General Hospital",
			Building: "Main",
			Unit:     "Radiology",
		},
	}

	proc := mapper.MapProcedure(event, "Patient/123")

	if proc.Location == nil {
		t.Fatal("Expected location reference")
	}
	if proc.Location.Display != "General Hospital - Main - Radiology" {
		t.Errorf("Location display = %q, want 'General Hospital - Main - Radiology'", proc.Location.Display)
	}
}

func TestMapProcedure_StatusMapping(t *testing.T) {
	mapper := NewUSCoreMapper()

	tests := []struct {
		input    string
		expected string
	}{
		{"completed", "completed"},
		{"complete", "completed"},
		{"done", "completed"},
		{"in-progress", "in-progress"},
		{"inprogress", "in-progress"},
		{"active", "in-progress"},
		{"preparation", "preparation"},
		{"scheduled", "preparation"},
		{"not-done", "not-done"},
		{"cancelled", "not-done"},
		{"on-hold", "on-hold"},
		{"paused", "on-hold"},
		{"stopped", "stopped"},
		{"aborted", "stopped"},
		{"entered-in-error", "entered-in-error"},
		{"error", "entered-in-error"},
		{"", "completed"},        // Default
		{"unknown", "completed"}, // Default
		{"xyz", "completed"},     // Unknown maps to default
	}

	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			event := &events.ProcedureEvent{
				Procedure: events.Procedure{
					Name:   "Test",
					Code:   "123",
					Status: test.input,
				},
			}

			proc := mapper.MapProcedure(event, "Patient/123")

			if proc.Status != test.expected {
				t.Errorf("Status for %q = %q, want %q", test.input, proc.Status, test.expected)
			}
		})
	}
}

func TestMapProcedure_CodeSystemDetection(t *testing.T) {
	mapper := NewUSCoreMapper()

	tests := []struct {
		code           string
		expectedSystem string
		description    string
	}{
		{"80146002", SystemSNOMED, "SNOMED code (8 digits)"},
		{"73761001", SystemSNOMED, "SNOMED code (8 digits)"},
		{"99213", SystemCPT, "CPT code (5 digits)"},
		{"12345", SystemCPT, "CPT code (5 digits)"},
		{"0DB64ZZ", SystemICD10PCS, "ICD-10-PCS code (7 chars)"},
	}

	for _, test := range tests {
		t.Run(test.description, func(t *testing.T) {
			event := &events.ProcedureEvent{
				Procedure: events.Procedure{
					Name: "Test Procedure",
					Code: test.code,
				},
			}

			proc := mapper.MapProcedure(event, "Patient/123")

			if len(proc.Code.Coding) != 1 {
				t.Fatalf("Expected 1 coding, got %d", len(proc.Code.Coding))
			}
			if proc.Code.Coding[0].System != test.expectedSystem {
				t.Errorf("System for %q = %q, want %q", test.code, proc.Code.Coding[0].System, test.expectedSystem)
			}
		})
	}
}

func TestMapProcedure_ExplicitCodeSystem(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.ProcedureEvent{
		Procedure: events.Procedure{
			Name:       "Test",
			Code:       "ABC123",
			CodeSystem: "http://custom.system/codes",
		},
	}

	proc := mapper.MapProcedure(event, "Patient/123")

	if proc.Code.Coding[0].System != "http://custom.system/codes" {
		t.Errorf("System = %q, want 'http://custom.system/codes'", proc.Code.Coding[0].System)
	}
}

func TestMapProcedure_NilEvent(t *testing.T) {
	mapper := NewUSCoreMapper()

	proc := mapper.MapProcedure(nil, "Patient/123")

	if proc != nil {
		t.Error("Expected nil for nil event")
	}
}

func TestProcedureJSONSerialization(t *testing.T) {
	proc := &Procedure{
		ResourceType: "Procedure",
		ID:           "test-proc",
		Status:       "completed",
		Code: CodeableConcept{
			Coding: []Coding{
				{
					System:  SystemSNOMED,
					Code:    "80146002",
					Display: "Appendectomy",
				},
			},
		},
		Subject: &Reference{Reference: "Patient/123"},
	}

	data, err := json.Marshal(proc)
	if err != nil {
		t.Fatalf("Failed to marshal Procedure: %v", err)
	}

	jsonStr := string(data)
	if !strings.Contains(jsonStr, `"resourceType":"Procedure"`) {
		t.Error("JSON missing resourceType")
	}
	if !strings.Contains(jsonStr, `"status":"completed"`) {
		t.Error("JSON missing status")
	}
	if !strings.Contains(jsonStr, `"80146002"`) {
		t.Error("JSON missing code")
	}
}

// ========== Immunization Tests ==========

func TestMapImmunization_Basic(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.ImmunizationEvent{
		EventMeta: events.NewEventMeta(events.EventImmunization, "test", events.FormatCDA),
		Immunization: events.Immunization{
			VaccineCode: "140",
			VaccineName: "Influenza, seasonal, injectable, preservative free",
			Status:      "completed",
		},
		AdministeredDate: "2024-10-15",
	}
	event.ID = "imm-123"

	imm := mapper.MapImmunization(event, "Patient/test-patient")

	if imm == nil {
		t.Fatal("Expected non-nil Immunization")
	}

	// Verify resource type and profile
	if imm.ResourceType != "Immunization" {
		t.Errorf("ResourceType = %q, want 'Immunization'", imm.ResourceType)
	}
	if len(imm.Meta.Profile) != 1 || imm.Meta.Profile[0] != USCoreImmunizationProfile {
		t.Errorf("Profile = %v, want [%s]", imm.Meta.Profile, USCoreImmunizationProfile)
	}

	// Verify required elements
	if imm.Status != "completed" {
		t.Errorf("Status = %q, want 'completed'", imm.Status)
	}
	if imm.Patient == nil || imm.Patient.Reference != "Patient/test-patient" {
		t.Errorf("Patient reference incorrect")
	}

	// Verify vaccine code uses CVX system
	if len(imm.VaccineCode.Coding) != 1 {
		t.Errorf("Expected 1 coding, got %d", len(imm.VaccineCode.Coding))
	}
	if imm.VaccineCode.Coding[0].System != SystemCVX {
		t.Errorf("System = %q, want %q", imm.VaccineCode.Coding[0].System, SystemCVX)
	}
	if imm.VaccineCode.Coding[0].Code != "140" {
		t.Errorf("Code = %q, want '140'", imm.VaccineCode.Coding[0].Code)
	}

	// Verify occurrence date
	if imm.OccurrenceDateTime != "2024-10-15" {
		t.Errorf("OccurrenceDateTime = %q, want '2024-10-15'", imm.OccurrenceDateTime)
	}

	// Verify primary source is set
	if imm.PrimarySource == nil || !*imm.PrimarySource {
		t.Error("Expected PrimarySource to be true")
	}
}

func TestMapImmunization_WithDetails(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.ImmunizationEvent{
		EventMeta: events.NewEventMeta(events.EventImmunization, "test", events.FormatCDA),
		Immunization: events.Immunization{
			VaccineCode:  "208",
			VaccineName:  "Pfizer-BioNTech COVID-19 Vaccine",
			Status:       "completed",
			LotNumber:    "EW0150",
			Site:         "LA",
			Route:        "IM",
			DoseQuantity: "0.3 mL",
		},
		AdministeredDate: "2024-03-15",
	}

	imm := mapper.MapImmunization(event, "Patient/123")

	// Verify lot number
	if imm.LotNumber != "EW0150" {
		t.Errorf("LotNumber = %q, want 'EW0150'", imm.LotNumber)
	}

	// Verify site (should be mapped to SNOMED)
	if imm.Site == nil {
		t.Fatal("Expected site to be set")
	}
	if len(imm.Site.Coding) != 1 {
		t.Errorf("Expected 1 site coding, got %d", len(imm.Site.Coding))
	}
	if imm.Site.Coding[0].System != SystemSNOMED {
		t.Errorf("Site system = %q, want %q", imm.Site.Coding[0].System, SystemSNOMED)
	}
	if imm.Site.Coding[0].Display != "Left arm" {
		t.Errorf("Site display = %q, want 'Left arm'", imm.Site.Coding[0].Display)
	}

	// Verify route (should be mapped to NCIT)
	if imm.Route == nil {
		t.Fatal("Expected route to be set")
	}
	if len(imm.Route.Coding) != 1 {
		t.Errorf("Expected 1 route coding, got %d", len(imm.Route.Coding))
	}
	if imm.Route.Coding[0].Display != "Intramuscular" {
		t.Errorf("Route display = %q, want 'Intramuscular'", imm.Route.Coding[0].Display)
	}

	// Verify dose quantity
	if imm.DoseQuantity == nil {
		t.Fatal("Expected dose quantity to be set")
	}
	if imm.DoseQuantity.Value != 0.3 {
		t.Errorf("DoseQuantity value = %f, want 0.3", imm.DoseQuantity.Value)
	}
	if imm.DoseQuantity.Unit != "mL" {
		t.Errorf("DoseQuantity unit = %q, want 'mL'", imm.DoseQuantity.Unit)
	}
}

func TestMapImmunization_WithPerformer(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.ImmunizationEvent{
		EventMeta: events.NewEventMeta(events.EventImmunization, "test", events.FormatCDA),
		Immunization: events.Immunization{
			VaccineCode: "140",
			VaccineName: "Flu Shot",
		},
		AdministeredDate: "2024-10-15",
		Performer: &events.Provider{
			NPI:        "9876543210",
			FamilyName: "Johnson",
			GivenName:  "Mary",
		},
	}

	imm := mapper.MapImmunization(event, "Patient/123")

	if len(imm.Performer) != 1 {
		t.Fatalf("Expected 1 performer, got %d", len(imm.Performer))
	}

	performer := imm.Performer[0]

	// Verify function code (AP = Administering Provider)
	if performer.Function == nil || len(performer.Function.Coding) != 1 {
		t.Fatal("Expected performer function with coding")
	}
	if performer.Function.Coding[0].Code != "AP" {
		t.Errorf("Function code = %q, want 'AP'", performer.Function.Coding[0].Code)
	}

	// Verify actor
	if performer.Actor == nil {
		t.Fatal("Expected performer actor")
	}
	if performer.Actor.Reference != "Practitioner/9876543210" {
		t.Errorf("Actor reference = %q, want 'Practitioner/9876543210'", performer.Actor.Reference)
	}
	if performer.Actor.Display != "Johnson, Mary" {
		t.Errorf("Actor display = %q, want 'Johnson, Mary'", performer.Actor.Display)
	}
}

func TestMapImmunization_StatusMapping(t *testing.T) {
	mapper := NewUSCoreMapper()

	tests := []struct {
		input    string
		expected string
	}{
		{"completed", "completed"},
		{"complete", "completed"},
		{"done", "completed"},
		{"given", "completed"},
		{"administered", "completed"},
		{"not-done", "not-done"},
		{"not_given", "not-done"},
		{"refused", "not-done"},
		{"contraindicated", "not-done"},
		{"entered-in-error", "entered-in-error"},
		{"error", "entered-in-error"},
		{"", "completed"}, // Default
	}

	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			event := &events.ImmunizationEvent{
				Immunization: events.Immunization{
					VaccineCode: "140",
					VaccineName: "Test",
					Status:      test.input,
				},
			}

			imm := mapper.MapImmunization(event, "Patient/123")

			if imm.Status != test.expected {
				t.Errorf("Status for %q = %q, want %q", test.input, imm.Status, test.expected)
			}
		})
	}
}

func TestMapImmunization_SiteMapping(t *testing.T) {
	mapper := NewUSCoreMapper()

	tests := []struct {
		site     string
		expected string
	}{
		{"LA", "Left arm"},
		{"RA", "Right arm"},
		{"LT", "Left thigh"},
		{"RT", "Right thigh"},
		{"LD", "Left deltoid"},
		{"RD", "Right deltoid"},
	}

	for _, test := range tests {
		t.Run(test.site, func(t *testing.T) {
			event := &events.ImmunizationEvent{
				Immunization: events.Immunization{
					VaccineCode: "140",
					Site:        test.site,
				},
			}

			imm := mapper.MapImmunization(event, "Patient/123")

			if imm.Site == nil {
				t.Fatal("Expected site to be set")
			}
			if imm.Site.Text != test.expected {
				t.Errorf("Site text for %q = %q, want %q", test.site, imm.Site.Text, test.expected)
			}
		})
	}
}

func TestMapImmunization_RouteMapping(t *testing.T) {
	mapper := NewUSCoreMapper()

	tests := []struct {
		route    string
		expected string
	}{
		{"IM", "Intramuscular"},
		{"SC", "Subcutaneous"},
		{"SQ", "Subcutaneous"},
		{"ID", "Intradermal"},
		{"PO", "Oral"},
		{"IN", "Intranasal"},
		{"NASAL", "Intranasal"},
		{"ORAL", "Oral"},
	}

	for _, test := range tests {
		t.Run(test.route, func(t *testing.T) {
			event := &events.ImmunizationEvent{
				Immunization: events.Immunization{
					VaccineCode: "140",
					Route:       test.route,
				},
			}

			imm := mapper.MapImmunization(event, "Patient/123")

			if imm.Route == nil {
				t.Fatal("Expected route to be set")
			}
			if imm.Route.Text != test.expected {
				t.Errorf("Route text for %q = %q, want %q", test.route, imm.Route.Text, test.expected)
			}
		})
	}
}

func TestMapImmunization_DoseQuantityParsing(t *testing.T) {
	mapper := NewUSCoreMapper()

	tests := []struct {
		input         string
		expectedValue float64
		expectedUnit  string
	}{
		{"0.5 mL", 0.5, "mL"},
		{"0.3 mL", 0.3, "mL"},
		{"1.0 mL", 1.0, "mL"},
		{"0.5", 0.5, "mL"}, // Default unit
	}

	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			event := &events.ImmunizationEvent{
				Immunization: events.Immunization{
					VaccineCode:  "140",
					DoseQuantity: test.input,
				},
			}

			imm := mapper.MapImmunization(event, "Patient/123")

			if imm.DoseQuantity == nil {
				t.Fatal("Expected dose quantity to be set")
			}
			if imm.DoseQuantity.Value != test.expectedValue {
				t.Errorf("Value for %q = %f, want %f", test.input, imm.DoseQuantity.Value, test.expectedValue)
			}
			if imm.DoseQuantity.Unit != test.expectedUnit {
				t.Errorf("Unit for %q = %q, want %q", test.input, imm.DoseQuantity.Unit, test.expectedUnit)
			}
		})
	}
}

func TestMapImmunization_NilEvent(t *testing.T) {
	mapper := NewUSCoreMapper()

	imm := mapper.MapImmunization(nil, "Patient/123")

	if imm != nil {
		t.Error("Expected nil for nil event")
	}
}

func TestMapImmunization_WithEncounterAndLocation(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.ImmunizationEvent{
		EventMeta: events.NewEventMeta(events.EventImmunization, "test", events.FormatCDA),
		Immunization: events.Immunization{
			VaccineCode: "140",
			VaccineName: "Flu Shot",
		},
		AdministeredDate: "2024-10-15",
		Encounter: &events.Encounter{
			ID: "enc-789",
		},
		Location: &events.Location{
			Facility:    "Community Clinic",
			Description: "Main Building",
		},
	}

	imm := mapper.MapImmunization(event, "Patient/123")

	// Verify encounter
	if imm.Encounter == nil {
		t.Fatal("Expected encounter reference")
	}
	if imm.Encounter.Reference != "Encounter/enc-789" {
		t.Errorf("Encounter reference = %q, want 'Encounter/enc-789'", imm.Encounter.Reference)
	}

	// Verify location
	if imm.Location == nil {
		t.Fatal("Expected location reference")
	}
	if imm.Location.Display != "Community Clinic" {
		t.Errorf("Location display = %q, want 'Community Clinic'", imm.Location.Display)
	}
}

func TestImmunizationJSONSerialization(t *testing.T) {
	primarySource := true
	imm := &Immunization{
		ResourceType: "Immunization",
		ID:           "test-imm",
		Status:       "completed",
		VaccineCode: CodeableConcept{
			Coding: []Coding{
				{
					System:  SystemCVX,
					Code:    "140",
					Display: "Flu Shot",
				},
			},
		},
		Patient:            &Reference{Reference: "Patient/123"},
		OccurrenceDateTime: "2024-10-15",
		PrimarySource:      &primarySource,
	}

	data, err := json.Marshal(imm)
	if err != nil {
		t.Fatalf("Failed to marshal Immunization: %v", err)
	}

	jsonStr := string(data)
	if !strings.Contains(jsonStr, `"resourceType":"Immunization"`) {
		t.Error("JSON missing resourceType")
	}
	if !strings.Contains(jsonStr, `"status":"completed"`) {
		t.Error("JSON missing status")
	}
	if !strings.Contains(jsonStr, SystemCVX) {
		t.Error("JSON missing CVX system")
	}
	if !strings.Contains(jsonStr, `"140"`) {
		t.Error("JSON missing vaccine code")
	}
	if !strings.Contains(jsonStr, `"primarySource":true`) {
		t.Error("JSON missing primarySource")
	}
}

// ========== Vital Signs Tests ==========

func TestMapVitalSign_HeartRate(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.VitalSignEvent{
		EventMeta: events.NewEventMeta(events.EventVitalSign, "test", events.FormatCDA),
		VitalSign: events.VitalSign{
			Name:           "Heart Rate",
			LOINCCode:      LOINCHeartRate,
			Value:          "72",
			Unit:           "bpm",
			Interpretation: "normal",
		},
	}
	event.ID = "vs-123"

	obs := mapper.MapVitalSign(event, "Patient/test-patient")

	if obs == nil {
		t.Fatal("Expected non-nil Observation")
	}

	// Verify resource type and profile
	if obs.ResourceType != "Observation" {
		t.Errorf("ResourceType = %q, want 'Observation'", obs.ResourceType)
	}
	if len(obs.Meta.Profile) != 1 || obs.Meta.Profile[0] != USCoreHeartRateProfile {
		t.Errorf("Profile = %v, want [%s]", obs.Meta.Profile, USCoreHeartRateProfile)
	}

	// Verify status
	if obs.Status != "final" {
		t.Errorf("Status = %q, want 'final'", obs.Status)
	}

	// Verify category is "vital-signs"
	if len(obs.Category) != 1 {
		t.Fatalf("Expected 1 category, got %d", len(obs.Category))
	}
	if len(obs.Category[0].Coding) != 1 || obs.Category[0].Coding[0].Code != VitalSignsCategory {
		t.Errorf("Category code = %q, want %q", obs.Category[0].Coding[0].Code, VitalSignsCategory)
	}

	// Verify code is LOINC
	if len(obs.Code.Coding) != 1 {
		t.Fatalf("Expected 1 code, got %d", len(obs.Code.Coding))
	}
	if obs.Code.Coding[0].System != SystemLOINC {
		t.Errorf("Code system = %q, want %q", obs.Code.Coding[0].System, SystemLOINC)
	}
	if obs.Code.Coding[0].Code != LOINCHeartRate {
		t.Errorf("LOINC code = %q, want %q", obs.Code.Coding[0].Code, LOINCHeartRate)
	}

	// Verify value
	if obs.ValueQuantity == nil {
		t.Fatal("Expected ValueQuantity to be set")
	}
	if obs.ValueQuantity.Value != 72 {
		t.Errorf("Value = %f, want 72", obs.ValueQuantity.Value)
	}
	if obs.ValueQuantity.Unit != "bpm" {
		t.Errorf("Unit = %q, want 'bpm'", obs.ValueQuantity.Unit)
	}
	if obs.ValueQuantity.Code != "/min" {
		t.Errorf("UCUM code = %q, want '/min'", obs.ValueQuantity.Code)
	}

	// Verify interpretation
	if len(obs.Interpretation) != 1 {
		t.Fatalf("Expected 1 interpretation, got %d", len(obs.Interpretation))
	}
	if len(obs.Interpretation[0].Coding) != 1 || obs.Interpretation[0].Coding[0].Code != "N" {
		t.Errorf("Interpretation code = %q, want 'N'", obs.Interpretation[0].Coding[0].Code)
	}

	// Verify subject
	if obs.Subject == nil || obs.Subject.Reference != "Patient/test-patient" {
		t.Error("Subject reference incorrect")
	}
}

func TestMapVitalSign_BodyTemperature(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.VitalSignEvent{
		EventMeta: events.NewEventMeta(events.EventVitalSign, "test", events.FormatCDA),
		VitalSign: events.VitalSign{
			Name:      "Body Temperature",
			LOINCCode: LOINCBodyTemperature,
			Value:     "37.2",
			Unit:      "°C",
		},
	}

	obs := mapper.MapVitalSign(event, "Patient/123")

	if obs.Meta.Profile[0] != USCoreBodyTemperatureProfile {
		t.Errorf("Profile = %q, want %q", obs.Meta.Profile[0], USCoreBodyTemperatureProfile)
	}

	if obs.ValueQuantity.Value != 37.2 {
		t.Errorf("Value = %f, want 37.2", obs.ValueQuantity.Value)
	}
	if obs.ValueQuantity.Code != "Cel" {
		t.Errorf("UCUM code = %q, want 'Cel'", obs.ValueQuantity.Code)
	}
}

func TestMapVitalSign_BloodPressure(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.VitalSignEvent{
		EventMeta: events.NewEventMeta(events.EventVitalSign, "test", events.FormatCDA),
		VitalSign: events.VitalSign{
			Name:      "Systolic Blood Pressure",
			LOINCCode: LOINCSystolicBP,
			Value:     "120",
			Unit:      "mmHg",
		},
	}

	obs := mapper.MapVitalSign(event, "Patient/123")

	if obs.Meta.Profile[0] != USCoreBloodPressureProfile {
		t.Errorf("Profile = %q, want %q", obs.Meta.Profile[0], USCoreBloodPressureProfile)
	}

	if obs.Code.Coding[0].Code != LOINCSystolicBP {
		t.Errorf("LOINC code = %q, want %q", obs.Code.Coding[0].Code, LOINCSystolicBP)
	}

	if obs.ValueQuantity.Code != "mm[Hg]" {
		t.Errorf("UCUM code = %q, want 'mm[Hg]'", obs.ValueQuantity.Code)
	}
}

func TestMapVitalSign_OxygenSaturation(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.VitalSignEvent{
		EventMeta: events.NewEventMeta(events.EventVitalSign, "test", events.FormatCDA),
		VitalSign: events.VitalSign{
			Name:      "Oxygen Saturation",
			LOINCCode: LOINCPulseOximetry,
			Value:     "98",
			Unit:      "%",
		},
	}

	obs := mapper.MapVitalSign(event, "Patient/123")

	if obs.Meta.Profile[0] != USCorePulseOximetryProfile {
		t.Errorf("Profile = %q, want %q", obs.Meta.Profile[0], USCorePulseOximetryProfile)
	}

	if obs.ValueQuantity.Value != 98 {
		t.Errorf("Value = %f, want 98", obs.ValueQuantity.Value)
	}
	if obs.ValueQuantity.Code != "%" {
		t.Errorf("UCUM code = %q, want '%%'", obs.ValueQuantity.Code)
	}
}

func TestMapVitalSign_BMI(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.VitalSignEvent{
		EventMeta: events.NewEventMeta(events.EventVitalSign, "test", events.FormatCDA),
		VitalSign: events.VitalSign{
			Name:      "Body Mass Index",
			LOINCCode: LOINCBodyMassIndex,
			Value:     "24.5",
			Unit:      "kg/m2",
		},
	}

	obs := mapper.MapVitalSign(event, "Patient/123")

	if obs.Meta.Profile[0] != USCoreBMIProfile {
		t.Errorf("Profile = %q, want %q", obs.Meta.Profile[0], USCoreBMIProfile)
	}

	if obs.ValueQuantity.Value != 24.5 {
		t.Errorf("Value = %f, want 24.5", obs.ValueQuantity.Value)
	}
	if obs.ValueQuantity.Code != "kg/m2" {
		t.Errorf("UCUM code = %q, want 'kg/m2'", obs.ValueQuantity.Code)
	}
}

func TestMapVitalSign_ProfileDetectionByName(t *testing.T) {
	mapper := NewUSCoreMapper()

	tests := []struct {
		name     string
		expected string
	}{
		{"Heart Rate", USCoreHeartRateProfile},
		{"Pulse Rate", USCoreHeartRateProfile},
		{"Respiratory Rate", USCoreRespiratoryRateProfile},
		{"Body Temperature", USCoreBodyTemperatureProfile},
		{"Temperature", USCoreBodyTemperatureProfile},
		{"Height", USCoreBodyHeightProfile},
		{"Body Weight", USCoreBodyWeightProfile},
		{"Weight", USCoreBodyWeightProfile},
		{"BMI", USCoreBMIProfile},
		{"Body Mass Index", USCoreBMIProfile},
		{"Oxygen Saturation", USCorePulseOximetryProfile},
		{"SpO2", USCorePulseOximetryProfile},
		{"Blood Pressure", USCoreBloodPressureProfile},
		{"Unknown Vital Sign", USCoreVitalSignsProfile},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			event := &events.VitalSignEvent{
				VitalSign: events.VitalSign{
					Name:  test.name,
					Value: "100",
				},
			}

			obs := mapper.MapVitalSign(event, "Patient/123")

			if obs.Meta.Profile[0] != test.expected {
				t.Errorf("Profile for %q = %q, want %q", test.name, obs.Meta.Profile[0], test.expected)
			}
		})
	}
}

func TestMapVitalSign_LOINCCodeInference(t *testing.T) {
	mapper := NewUSCoreMapper()

	tests := []struct {
		name         string
		expectedCode string
	}{
		{"Heart Rate", LOINCHeartRate},
		{"Pulse Rate", LOINCHeartRate},
		{"Respiratory Rate", LOINCRespiratoryRate},
		{"Body Temperature", LOINCBodyTemperature},
		{"Body Height", LOINCBodyHeight},
		{"Body Weight", LOINCBodyWeight},
		{"BMI", LOINCBodyMassIndex},
		{"Oxygen Saturation", LOINCPulseOximetry},
		{"SpO2", LOINCPulseOximetry},
		{"Systolic BP", LOINCSystolicBP},
		{"Diastolic BP", LOINCDiastolicBP},
		{"Blood Pressure", LOINCBloodPressurePanel},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			event := &events.VitalSignEvent{
				VitalSign: events.VitalSign{
					Name:  test.name,
					Value: "100",
					// No LOINCCode provided - should be inferred
				},
			}

			obs := mapper.MapVitalSign(event, "Patient/123")

			if obs.Code.Coding[0].Code != test.expectedCode {
				t.Errorf("Inferred LOINC for %q = %q, want %q", test.name, obs.Code.Coding[0].Code, test.expectedCode)
			}
		})
	}
}

func TestMapVitalSign_UnitToUCUM(t *testing.T) {
	mapper := NewUSCoreMapper()

	tests := []struct {
		unit      string
		loincCode string
		expected  string
	}{
		{"bpm", LOINCHeartRate, "/min"},
		{"beats/min", LOINCHeartRate, "/min"},
		{"°C", LOINCBodyTemperature, "Cel"},
		{"°F", LOINCBodyTemperature, "[degF]"},
		{"cm", LOINCBodyHeight, "cm"},
		{"in", LOINCBodyHeight, "[in_i]"},
		{"kg", LOINCBodyWeight, "kg"},
		{"lb", LOINCBodyWeight, "[lb_av]"},
		{"mmHg", LOINCSystolicBP, "mm[Hg]"},
		{"%", LOINCPulseOximetry, "%"},
		{"kg/m2", LOINCBodyMassIndex, "kg/m2"},
	}

	for _, test := range tests {
		t.Run(test.unit, func(t *testing.T) {
			event := &events.VitalSignEvent{
				VitalSign: events.VitalSign{
					Name:      "Test",
					LOINCCode: test.loincCode,
					Value:     "100",
					Unit:      test.unit,
				},
			}

			obs := mapper.MapVitalSign(event, "Patient/123")

			if obs.ValueQuantity.Code != test.expected {
				t.Errorf("UCUM for %q = %q, want %q", test.unit, obs.ValueQuantity.Code, test.expected)
			}
		})
	}
}

func TestMapVitalSign_InterpretationMapping(t *testing.T) {
	mapper := NewUSCoreMapper()

	tests := []struct {
		interpretation string
		expectedCode   string
	}{
		{"normal", "N"},
		{"Normal", "N"},
		{"high", "H"},
		{"High", "H"},
		{"low", "L"},
		{"Low", "L"},
		{"critical", "AA"},
		{"critical high", "HH"},
		{"critical low", "LL"},
		{"abnormal", "A"},
	}

	for _, test := range tests {
		t.Run(test.interpretation, func(t *testing.T) {
			event := &events.VitalSignEvent{
				VitalSign: events.VitalSign{
					Name:           "Heart Rate",
					LOINCCode:      LOINCHeartRate,
					Value:          "100",
					Interpretation: test.interpretation,
				},
			}

			obs := mapper.MapVitalSign(event, "Patient/123")

			if len(obs.Interpretation) != 1 {
				t.Fatalf("Expected 1 interpretation, got %d", len(obs.Interpretation))
			}
			if len(obs.Interpretation[0].Coding) != 1 {
				t.Fatal("Expected interpretation coding")
			}
			if obs.Interpretation[0].Coding[0].Code != test.expectedCode {
				t.Errorf("Interpretation for %q = %q, want %q", test.interpretation, obs.Interpretation[0].Coding[0].Code, test.expectedCode)
			}
		})
	}
}

func TestMapVitalSign_WithEncounter(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.VitalSignEvent{
		EventMeta: events.NewEventMeta(events.EventVitalSign, "test", events.FormatCDA),
		VitalSign: events.VitalSign{
			Name:      "Heart Rate",
			LOINCCode: LOINCHeartRate,
			Value:     "72",
		},
		Encounter: &events.Encounter{
			ID: "enc-123",
		},
	}

	obs := mapper.MapVitalSign(event, "Patient/123")

	if obs.Encounter == nil {
		t.Fatal("Expected encounter reference")
	}
	if obs.Encounter.Reference != "Encounter/enc-123" {
		t.Errorf("Encounter reference = %q, want 'Encounter/enc-123'", obs.Encounter.Reference)
	}
}

func TestMapVitalSign_NonNumericValue(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.VitalSignEvent{
		VitalSign: events.VitalSign{
			Name:  "Pain Scale",
			Value: "moderate",
		},
	}

	obs := mapper.MapVitalSign(event, "Patient/123")

	// Non-numeric value should be stored as ValueString
	if obs.ValueQuantity != nil {
		t.Error("Expected ValueQuantity to be nil for non-numeric value")
	}
	if obs.ValueString != "moderate" {
		t.Errorf("ValueString = %q, want 'moderate'", obs.ValueString)
	}
}

func TestMapVitalSign_NilEvent(t *testing.T) {
	mapper := NewUSCoreMapper()

	obs := mapper.MapVitalSign(nil, "Patient/123")

	if obs != nil {
		t.Error("Expected nil for nil event")
	}
}

func TestMapVitalSign_WithEffectiveDateTime(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.VitalSignEvent{
		EventMeta: events.NewEventMeta(events.EventVitalSign, "test", events.FormatCDA),
		VitalSign: events.VitalSign{
			Name:      "Heart Rate",
			LOINCCode: LOINCHeartRate,
			Value:     "72",
		},
	}

	obs := mapper.MapVitalSign(event, "Patient/123")

	// EffectiveDateTime should be set from event timestamp
	if obs.EffectiveDateTime == "" {
		t.Error("Expected EffectiveDateTime to be set")
	}
}

func TestVitalSignObservationJSONSerialization(t *testing.T) {
	obs := &Observation{
		ResourceType: "Observation",
		ID:           "vs-test",
		Meta: &Meta{
			Profile: []string{USCoreHeartRateProfile},
		},
		Status: "final",
		Category: []CodeableConcept{
			{
				Coding: []Coding{
					{
						System:  SystemObservationCategory,
						Code:    VitalSignsCategory,
						Display: "Vital Signs",
					},
				},
			},
		},
		Code: CodeableConcept{
			Coding: []Coding{
				{
					System:  SystemLOINC,
					Code:    LOINCHeartRate,
					Display: "Heart Rate",
				},
			},
		},
		Subject: &Reference{Reference: "Patient/123"},
		ValueQuantity: &Quantity{
			Value:  72,
			Unit:   "bpm",
			System: SystemUCUM,
			Code:   "/min",
		},
	}

	data, err := json.Marshal(obs)
	if err != nil {
		t.Fatalf("Failed to marshal Observation: %v", err)
	}

	jsonStr := string(data)
	if !strings.Contains(jsonStr, `"resourceType":"Observation"`) {
		t.Error("JSON missing resourceType")
	}
	if !strings.Contains(jsonStr, USCoreHeartRateProfile) {
		t.Error("JSON missing profile")
	}
	if !strings.Contains(jsonStr, VitalSignsCategory) {
		t.Error("JSON missing vital-signs category")
	}
	if !strings.Contains(jsonStr, LOINCHeartRate) {
		t.Error("JSON missing LOINC code")
	}
	if !strings.Contains(jsonStr, `"value":72`) {
		t.Error("JSON missing value")
	}
}

// =============================================================================
// MedicationRequest Tests
// =============================================================================

func TestMapMedicationRequest_Basic(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.MedicationRequestEvent{
		EventMeta: events.EventMeta{ID: "med-req-001"},
		Patient:   &events.Patient{MRN: "12345"},
		MedicationRequest: events.MedicationRequest{
			Medication: events.Medication{
				Code: "197361",
				Name: "Lisinopril 10 MG Oral Tablet",
			},
			Status:            "active",
			Intent:            "order",
			AuthoredOn:        "2024-01-15T10:30:00Z",
			DosageInstruction: "Take 1 tablet by mouth daily",
			DispenseQuantity:  30,
			DispenseUnit:      "tablet",
			DaysSupply:        30,
			NumberOfRefills:   3,
		},
	}

	result := mapper.MapMedicationRequest(event, "Patient/12345")

	if result == nil {
		t.Fatal("Expected MedicationRequest, got nil")
	}
	if result.ID != "med-req-001" {
		t.Errorf("Expected ID 'med-req-001', got '%s'", result.ID)
	}
	if result.Status != "active" {
		t.Errorf("Expected status 'active', got '%s'", result.Status)
	}
	if result.Intent != "order" {
		t.Errorf("Expected intent 'order', got '%s'", result.Intent)
	}
	if result.Subject == nil || result.Subject.Reference != "Patient/12345" {
		t.Error("Expected patient reference")
	}
	if result.Meta == nil || len(result.Meta.Profile) == 0 {
		t.Error("Expected US Core profile in meta")
	}
	if result.Meta.Profile[0] != USCoreMedicationRequestProfile {
		t.Errorf("Expected US Core MedicationRequest profile, got '%s'", result.Meta.Profile[0])
	}
}

func TestMapMedicationRequest_MedicationCode(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.MedicationRequestEvent{
		EventMeta: events.EventMeta{ID: "med-req-002"},
		MedicationRequest: events.MedicationRequest{
			Medication: events.Medication{
				Code:     "197361",
				Name:     "Lisinopril",
				Strength: "10 MG",
				Form:     "Oral Tablet",
			},
			Status: "active",
			Intent: "order",
		},
	}

	result := mapper.MapMedicationRequest(event, "Patient/12345")

	if result.MedicationCodeableConcept == nil {
		t.Fatal("Expected medication code")
	}
	if len(result.MedicationCodeableConcept.Coding) == 0 {
		t.Fatal("Expected medication coding")
	}
	if result.MedicationCodeableConcept.Coding[0].System != SystemRxNorm {
		t.Errorf("Expected RxNorm system, got '%s'", result.MedicationCodeableConcept.Coding[0].System)
	}
	if result.MedicationCodeableConcept.Coding[0].Code != "197361" {
		t.Errorf("Expected code '197361', got '%s'", result.MedicationCodeableConcept.Coding[0].Code)
	}
	// Check text includes strength and form
	if !strings.Contains(result.MedicationCodeableConcept.Text, "10 MG") {
		t.Error("Expected text to include strength")
	}
	if !strings.Contains(result.MedicationCodeableConcept.Text, "Oral Tablet") {
		t.Error("Expected text to include form")
	}
}

func TestMapMedicationRequest_DosageInstruction(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.MedicationRequestEvent{
		EventMeta: events.EventMeta{ID: "med-req-003"},
		MedicationRequest: events.MedicationRequest{
			Medication: events.Medication{
				Code: "197361",
				Name: "Lisinopril",
			},
			Status:            "active",
			Intent:            "order",
			DosageInstruction: "Take 1 tablet by mouth once daily",
			DoseQuantity:      "1",
			DoseUnit:          "tablet",
			Route:             "oral",
			Frequency:         "QD",
		},
	}

	result := mapper.MapMedicationRequest(event, "Patient/12345")

	if len(result.DosageInstruction) == 0 {
		t.Fatal("Expected dosage instruction")
	}

	dosage := result.DosageInstruction[0]
	if dosage.Text != "Take 1 tablet by mouth once daily" {
		t.Errorf("Expected dosage text, got '%s'", dosage.Text)
	}
	if dosage.Route == nil {
		t.Fatal("Expected route")
	}
	if len(dosage.Route.Coding) == 0 || dosage.Route.Coding[0].Code != "26643006" {
		t.Error("Expected SNOMED oral route code")
	}
	if dosage.Timing == nil {
		t.Fatal("Expected timing")
	}
	if dosage.Timing.Code == nil || dosage.Timing.Code.Text != "QD" {
		t.Error("Expected timing code QD")
	}
}

func TestMapMedicationRequest_DispenseRequest(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.MedicationRequestEvent{
		EventMeta:    events.EventMeta{ID: "med-req-004"},
		PharmacyID:   "pharmacy-123",
		PharmacyName: "CVS Pharmacy",
		MedicationRequest: events.MedicationRequest{
			Medication: events.Medication{
				Code: "197361",
				Name: "Lisinopril",
			},
			Status:           "active",
			Intent:           "order",
			DispenseQuantity: 30,
			DispenseUnit:     "tablet",
			DaysSupply:       30,
			NumberOfRefills:  3,
		},
	}

	result := mapper.MapMedicationRequest(event, "Patient/12345")

	if result.DispenseRequest == nil {
		t.Fatal("Expected dispense request")
	}
	if result.DispenseRequest.Quantity == nil || result.DispenseRequest.Quantity.Value != 30 {
		t.Error("Expected dispense quantity 30")
	}
	if result.DispenseRequest.NumberOfRepeatsAllowed != 3 {
		t.Errorf("Expected 3 refills, got %d", result.DispenseRequest.NumberOfRepeatsAllowed)
	}
	if result.DispenseRequest.ExpectedSupplyDuration == nil || result.DispenseRequest.ExpectedSupplyDuration.Value != 30 {
		t.Error("Expected 30 days supply")
	}
	if result.DispenseRequest.Performer == nil || !strings.Contains(result.DispenseRequest.Performer.Reference, "pharmacy-123") {
		t.Error("Expected pharmacy reference")
	}
}

func TestMapMedicationRequest_Prescriber(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.MedicationRequestEvent{
		EventMeta: events.EventMeta{ID: "med-req-005"},
		Prescriber: &events.Provider{
			NPI:        "1234567890",
			GivenName:  "John",
			FamilyName: "Smith",
			Prefix:     "Dr.",
			Suffix:     "MD",
		},
		MedicationRequest: events.MedicationRequest{
			Medication: events.Medication{Code: "197361", Name: "Lisinopril"},
			Status:     "active",
			Intent:     "order",
		},
	}

	result := mapper.MapMedicationRequest(event, "Patient/12345")

	if result.Requester == nil {
		t.Fatal("Expected requester")
	}
	if !strings.Contains(result.Requester.Reference, "1234567890") {
		t.Error("Expected NPI in requester reference")
	}
	if !strings.Contains(result.Requester.Display, "Dr.") {
		t.Error("Expected prefix in display")
	}
	if !strings.Contains(result.Requester.Display, "John") {
		t.Error("Expected given name in display")
	}
	if !strings.Contains(result.Requester.Display, "Smith") {
		t.Error("Expected family name in display")
	}
}

func TestMapMedicationRequest_StatusMapping(t *testing.T) {
	mapper := NewUSCoreMapper()

	tests := []struct {
		input    string
		expected string
	}{
		{"active", "active"},
		{"completed", "completed"},
		{"cancelled", "cancelled"},
		{"canceled", "cancelled"},
		{"stopped", "stopped"},
		{"on-hold", "on-hold"},
		{"on hold", "on-hold"},
		{"", "active"},         // Default
		{"invalid", "unknown"}, // Unknown
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			event := &events.MedicationRequestEvent{
				MedicationRequest: events.MedicationRequest{
					Medication: events.Medication{Code: "123", Name: "Test"},
					Status:     tt.input,
					Intent:     "order",
				},
			}
			result := mapper.MapMedicationRequest(event, "Patient/1")
			if result.Status != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result.Status)
			}
		})
	}
}

func TestMapMedicationRequest_RouteMapping(t *testing.T) {
	mapper := NewUSCoreMapper()

	tests := []struct {
		route      string
		snomedCode string
	}{
		{"oral", "26643006"},
		{"PO", "26643006"},
		{"IV", "47625008"},
		{"intravenous", "47625008"},
		{"IM", "78421000"},
		{"SubQ", "34206005"},
		{"topical", "6064005"},
		{"inhaled", "447694001"},
	}

	for _, tt := range tests {
		t.Run(tt.route, func(t *testing.T) {
			event := &events.MedicationRequestEvent{
				MedicationRequest: events.MedicationRequest{
					Medication: events.Medication{Code: "123", Name: "Test"},
					Status:     "active",
					Intent:     "order",
					Route:      tt.route,
				},
			}
			result := mapper.MapMedicationRequest(event, "Patient/1")
			if len(result.DosageInstruction) == 0 {
				t.Fatal("Expected dosage instruction")
			}
			if result.DosageInstruction[0].Route == nil {
				t.Fatal("Expected route")
			}
			if len(result.DosageInstruction[0].Route.Coding) == 0 {
				t.Fatal("Expected route coding")
			}
			if result.DosageInstruction[0].Route.Coding[0].Code != tt.snomedCode {
				t.Errorf("Expected SNOMED code '%s', got '%s'", tt.snomedCode, result.DosageInstruction[0].Route.Coding[0].Code)
			}
		})
	}
}

func TestMapMedicationRequest_FrequencyMapping(t *testing.T) {
	mapper := NewUSCoreMapper()

	tests := []struct {
		freq      string
		frequency int
		period    float64
	}{
		{"QD", 1, 1},
		{"BID", 2, 1},
		{"TID", 3, 1},
		{"QID", 4, 1},
		{"Q8H", 1, 8},
	}

	for _, tt := range tests {
		t.Run(tt.freq, func(t *testing.T) {
			event := &events.MedicationRequestEvent{
				MedicationRequest: events.MedicationRequest{
					Medication: events.Medication{Code: "123", Name: "Test"},
					Status:     "active",
					Intent:     "order",
					Frequency:  tt.freq,
				},
			}
			result := mapper.MapMedicationRequest(event, "Patient/1")
			if len(result.DosageInstruction) == 0 {
				t.Fatal("Expected dosage instruction")
			}
			if result.DosageInstruction[0].Timing == nil {
				t.Fatal("Expected timing")
			}
			if result.DosageInstruction[0].Timing.Repeat == nil {
				t.Fatal("Expected timing repeat")
			}
			if result.DosageInstruction[0].Timing.Repeat.Frequency != tt.frequency {
				t.Errorf("Expected frequency %d, got %d", tt.frequency, result.DosageInstruction[0].Timing.Repeat.Frequency)
			}
		})
	}
}

func TestMapMedicationRequest_NilEvent(t *testing.T) {
	mapper := NewUSCoreMapper()
	result := mapper.MapMedicationRequest(nil, "Patient/12345")
	if result != nil {
		t.Error("Expected nil for nil event")
	}
}

func TestMapMedicationRequest_Substitution(t *testing.T) {
	mapper := NewUSCoreMapper()

	// Test with substitution not allowed (default false)
	event := &events.MedicationRequestEvent{
		MedicationRequest: events.MedicationRequest{
			Medication:   events.Medication{Code: "123", Name: "Test"},
			Status:       "active",
			Intent:       "order",
			Substitution: false,
		},
	}
	result := mapper.MapMedicationRequest(event, "Patient/1")
	if result.Substitution == nil {
		t.Fatal("Expected substitution")
	}
	if result.Substitution.AllowedBoolean != false {
		t.Error("Expected substitution not allowed")
	}
}

// =============================================================================
// AllergyIntolerance Tests
// =============================================================================

func TestMapAllergyIntolerance_Basic(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.AllergyIntoleranceEvent{
		EventMeta: events.EventMeta{ID: "allergy-001"},
		Patient:   &events.Patient{MRN: "12345"},
		AllergyIntolerance: events.AllergyIntolerance{
			Code:               "7980",
			Name:               "Penicillin",
			Category:           "medication",
			ClinicalStatus:     "active",
			VerificationStatus: "confirmed",
			Criticality:        "high",
			Type:               "allergy",
		},
	}

	result := mapper.MapAllergyIntolerance(event, "Patient/12345")

	if result == nil {
		t.Fatal("Expected AllergyIntolerance, got nil")
	}
	if result.ID != "allergy-001" {
		t.Errorf("Expected ID 'allergy-001', got '%s'", result.ID)
	}
	if result.Patient == nil || result.Patient.Reference != "Patient/12345" {
		t.Error("Expected patient reference")
	}
	if result.Meta == nil || len(result.Meta.Profile) == 0 {
		t.Error("Expected US Core profile in meta")
	}
	if result.Meta.Profile[0] != USCoreAllergyIntoleranceProfile {
		t.Errorf("Expected US Core AllergyIntolerance profile, got '%s'", result.Meta.Profile[0])
	}
}

func TestMapAllergyIntolerance_AllergenCode(t *testing.T) {
	mapper := NewUSCoreMapper()

	// Test medication allergy (should use RxNorm)
	event := &events.AllergyIntoleranceEvent{
		AllergyIntolerance: events.AllergyIntolerance{
			Code:     "7980",
			Name:     "Penicillin",
			Category: "medication",
		},
	}

	result := mapper.MapAllergyIntolerance(event, "Patient/12345")

	if result.Code == nil {
		t.Fatal("Expected allergen code")
	}
	if len(result.Code.Coding) == 0 {
		t.Fatal("Expected allergen coding")
	}
	if result.Code.Coding[0].System != SystemRxNorm {
		t.Errorf("Expected RxNorm for medication allergy, got '%s'", result.Code.Coding[0].System)
	}
	if result.Code.Text != "Penicillin" {
		t.Errorf("Expected text 'Penicillin', got '%s'", result.Code.Text)
	}
}

func TestMapAllergyIntolerance_FoodAllergy(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.AllergyIntoleranceEvent{
		AllergyIntolerance: events.AllergyIntolerance{
			Code:     "91935009",
			Name:     "Peanut",
			Category: "food",
		},
	}

	result := mapper.MapAllergyIntolerance(event, "Patient/12345")

	if result.Code == nil {
		t.Fatal("Expected allergen code")
	}
	if len(result.Code.Coding) == 0 {
		t.Fatal("Expected allergen coding")
	}
	// Food allergies should use SNOMED
	if result.Code.Coding[0].System != SystemSNOMED {
		t.Errorf("Expected SNOMED for food allergy, got '%s'", result.Code.Coding[0].System)
	}
	if len(result.Category) == 0 || result.Category[0] != "food" {
		t.Error("Expected food category")
	}
}

func TestMapAllergyIntolerance_ClinicalStatus(t *testing.T) {
	mapper := NewUSCoreMapper()

	tests := []struct {
		input    string
		expected string
	}{
		{"active", "active"},
		{"inactive", "inactive"},
		{"resolved", "resolved"},
		{"", "active"}, // Default
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			event := &events.AllergyIntoleranceEvent{
				AllergyIntolerance: events.AllergyIntolerance{
					Name:           "Test Allergen",
					ClinicalStatus: tt.input,
				},
			}
			result := mapper.MapAllergyIntolerance(event, "Patient/1")
			if tt.input != "" && result.ClinicalStatus == nil {
				t.Fatal("Expected clinical status")
			}
			if tt.input != "" && len(result.ClinicalStatus.Coding) > 0 {
				if result.ClinicalStatus.Coding[0].Code != tt.expected {
					t.Errorf("Expected '%s', got '%s'", tt.expected, result.ClinicalStatus.Coding[0].Code)
				}
			}
		})
	}
}

func TestMapAllergyIntolerance_Criticality(t *testing.T) {
	mapper := NewUSCoreMapper()

	tests := []struct {
		input    string
		expected string
	}{
		{"low", "low"},
		{"high", "high"},
		{"unable-to-assess", "unable-to-assess"},
		{"critical", "high"},
		{"life-threatening", "high"},
		{"unknown", "unable-to-assess"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			event := &events.AllergyIntoleranceEvent{
				AllergyIntolerance: events.AllergyIntolerance{
					Name:        "Test Allergen",
					Criticality: tt.input,
				},
			}
			result := mapper.MapAllergyIntolerance(event, "Patient/1")
			if result.Criticality != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result.Criticality)
			}
		})
	}
}

func TestMapAllergyIntolerance_Category(t *testing.T) {
	mapper := NewUSCoreMapper()

	tests := []struct {
		input    string
		expected string
	}{
		{"food", "food"},
		{"medication", "medication"},
		{"drug", "medication"},
		{"environment", "environment"},
		{"biologic", "biologic"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			event := &events.AllergyIntoleranceEvent{
				AllergyIntolerance: events.AllergyIntolerance{
					Name:     "Test Allergen",
					Category: tt.input,
				},
			}
			result := mapper.MapAllergyIntolerance(event, "Patient/1")
			if len(result.Category) == 0 {
				t.Fatal("Expected category")
			}
			if result.Category[0] != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result.Category[0])
			}
		})
	}
}

func TestMapAllergyIntolerance_Type(t *testing.T) {
	mapper := NewUSCoreMapper()

	tests := []struct {
		input    string
		expected string
	}{
		{"allergy", "allergy"},
		{"intolerance", "intolerance"},
		{"true allergy", "allergy"},
		{"", "allergy"}, // Default
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			event := &events.AllergyIntoleranceEvent{
				AllergyIntolerance: events.AllergyIntolerance{
					Name: "Test Allergen",
					Type: tt.input,
				},
			}
			result := mapper.MapAllergyIntolerance(event, "Patient/1")
			if tt.input != "" && result.Type != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result.Type)
			}
		})
	}
}

func TestMapAllergyIntolerance_Reactions(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.AllergyIntoleranceEvent{
		AllergyIntolerance: events.AllergyIntolerance{
			Name: "Penicillin",
			Reactions: []events.AllergyReaction{
				{
					Manifestation:     "rash",
					ManifestationText: "Skin rash on arms",
					Severity:          "moderate",
					OnsetDate:         "2023-01-15",
					Note:              "Occurred 2 hours after first dose",
				},
				{
					Manifestation:     "hives",
					ManifestationText: "Urticaria",
					Severity:          "severe",
				},
			},
		},
	}

	result := mapper.MapAllergyIntolerance(event, "Patient/12345")

	if len(result.Reaction) != 2 {
		t.Fatalf("Expected 2 reactions, got %d", len(result.Reaction))
	}

	// Check first reaction
	r1 := result.Reaction[0]
	if len(r1.Manifestation) == 0 {
		t.Fatal("Expected manifestation")
	}
	// Should have SNOMED code for "rash"
	if len(r1.Manifestation[0].Coding) > 0 && r1.Manifestation[0].Coding[0].Code != "271807003" {
		t.Errorf("Expected SNOMED code for rash")
	}
	if r1.Severity != "moderate" {
		t.Errorf("Expected moderate severity, got '%s'", r1.Severity)
	}
	if r1.Description != "Occurred 2 hours after first dose" {
		t.Error("Expected reaction description")
	}

	// Check second reaction
	r2 := result.Reaction[1]
	if r2.Severity != "severe" {
		t.Errorf("Expected severe severity, got '%s'", r2.Severity)
	}
}

func TestMapAllergyIntolerance_ReactionManifestation(t *testing.T) {
	mapper := NewUSCoreMapper()

	tests := []struct {
		manifestation string
		snomedCode    string
	}{
		{"rash", "271807003"},
		{"hives", "126485001"},
		{"urticaria", "126485001"},
		{"anaphylaxis", "39579001"},
		{"nausea", "422587007"},
		{"wheezing", "56018004"},
		{"dyspnea", "267036007"},
	}

	for _, tt := range tests {
		t.Run(tt.manifestation, func(t *testing.T) {
			event := &events.AllergyIntoleranceEvent{
				AllergyIntolerance: events.AllergyIntolerance{
					Name: "Test Allergen",
					Reactions: []events.AllergyReaction{
						{Manifestation: tt.manifestation},
					},
				},
			}
			result := mapper.MapAllergyIntolerance(event, "Patient/1")
			if len(result.Reaction) == 0 {
				t.Fatal("Expected reaction")
			}
			if len(result.Reaction[0].Manifestation) == 0 {
				t.Fatal("Expected manifestation")
			}
			if len(result.Reaction[0].Manifestation[0].Coding) == 0 {
				t.Fatal("Expected SNOMED coding")
			}
			if result.Reaction[0].Manifestation[0].Coding[0].Code != tt.snomedCode {
				t.Errorf("Expected SNOMED code '%s', got '%s'", tt.snomedCode, result.Reaction[0].Manifestation[0].Coding[0].Code)
			}
		})
	}
}

func TestMapAllergyIntolerance_Recorder(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.AllergyIntoleranceEvent{
		AllergyIntolerance: events.AllergyIntolerance{
			Name: "Penicillin",
		},
		Recorder: &events.Provider{
			NPI:        "1234567890",
			GivenName:  "Jane",
			FamilyName: "Doe",
			Suffix:     "RN",
		},
	}

	result := mapper.MapAllergyIntolerance(event, "Patient/12345")

	if result.Recorder == nil {
		t.Fatal("Expected recorder")
	}
	if !strings.Contains(result.Recorder.Reference, "1234567890") {
		t.Error("Expected NPI in recorder reference")
	}
	if !strings.Contains(result.Recorder.Display, "Jane") {
		t.Error("Expected given name in display")
	}
	if !strings.Contains(result.Recorder.Display, "Doe") {
		t.Error("Expected family name in display")
	}
}

func TestMapAllergyIntolerance_WithEncounter(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.AllergyIntoleranceEvent{
		AllergyIntolerance: events.AllergyIntolerance{
			Name: "Penicillin",
		},
		Encounter: &events.Encounter{
			ID: "enc-12345",
		},
	}

	result := mapper.MapAllergyIntolerance(event, "Patient/12345")

	if result.Encounter == nil {
		t.Fatal("Expected encounter reference")
	}
	if result.Encounter.Reference != "Encounter/enc-12345" {
		t.Errorf("Expected 'Encounter/enc-12345', got '%s'", result.Encounter.Reference)
	}
}

func TestMapAllergyIntolerance_NilEvent(t *testing.T) {
	mapper := NewUSCoreMapper()
	result := mapper.MapAllergyIntolerance(nil, "Patient/12345")
	if result != nil {
		t.Error("Expected nil for nil event")
	}
}

func TestMapAllergyIntolerance_Dates(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.AllergyIntoleranceEvent{
		AllergyIntolerance: events.AllergyIntolerance{
			Name:         "Penicillin",
			OnsetDate:    "2020-05-15",
			RecordedDate: "2020-05-16T10:30:00Z",
		},
	}

	result := mapper.MapAllergyIntolerance(event, "Patient/12345")

	if result.OnsetDateTime != "2020-05-15" {
		t.Errorf("Expected onset '2020-05-15', got '%s'", result.OnsetDateTime)
	}
	if result.RecordedDate != "2020-05-16T10:30:00Z" {
		t.Errorf("Expected recorded date, got '%s'", result.RecordedDate)
	}
}

func TestMedicationRequestJSONSerialization(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.MedicationRequestEvent{
		EventMeta: events.EventMeta{ID: "med-req-json"},
		MedicationRequest: events.MedicationRequest{
			Medication: events.Medication{
				Code: "197361",
				Name: "Lisinopril 10 MG",
			},
			Status:            "active",
			Intent:            "order",
			DosageInstruction: "Take 1 tablet daily",
		},
	}

	result := mapper.MapMedicationRequest(event, "Patient/12345")
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	jsonStr := string(data)
	if !strings.Contains(jsonStr, `"resourceType":"MedicationRequest"`) {
		t.Error("JSON missing resourceType")
	}
	if !strings.Contains(jsonStr, USCoreMedicationRequestProfile) {
		t.Error("JSON missing profile")
	}
	if !strings.Contains(jsonStr, "197361") {
		t.Error("JSON missing medication code")
	}
}

func TestAllergyIntoleranceJSONSerialization(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.AllergyIntoleranceEvent{
		EventMeta: events.EventMeta{ID: "allergy-json"},
		AllergyIntolerance: events.AllergyIntolerance{
			Code:           "7980",
			Name:           "Penicillin",
			Category:       "medication",
			ClinicalStatus: "active",
			Criticality:    "high",
		},
	}

	result := mapper.MapAllergyIntolerance(event, "Patient/12345")
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	jsonStr := string(data)
	if !strings.Contains(jsonStr, `"resourceType":"AllergyIntolerance"`) {
		t.Error("JSON missing resourceType")
	}
	if !strings.Contains(jsonStr, USCoreAllergyIntoleranceProfile) {
		t.Error("JSON missing profile")
	}
	if !strings.Contains(jsonStr, "Penicillin") {
		t.Error("JSON missing allergen name")
	}
	if !strings.Contains(jsonStr, `"criticality":"high"`) {
		t.Error("JSON missing criticality")
	}
}

// ============================================================================
// CarePlan Tests
// ============================================================================

func TestMapCarePlan_BasicMapping(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.CarePlanEvent{
		EventMeta: events.EventMeta{ID: "cp-001"},
		CarePlan: events.CarePlan{
			Title:       "Diabetes Management Plan",
			Description: "Comprehensive plan for managing Type 2 diabetes",
			Status:      "active",
			Intent:      "plan",
			Category:    "discharge",
			PeriodStart: "2024-01-01",
			PeriodEnd:   "2024-12-31",
		},
	}

	result := mapper.MapCarePlan(event, "Patient/12345")

	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	// Check profile
	if len(result.Meta.Profile) != 1 || result.Meta.Profile[0] != USCoreCarePlanProfile {
		t.Errorf("Expected US Core CarePlan profile, got %v", result.Meta.Profile)
	}

	// Check required fields
	if result.Status != "active" {
		t.Errorf("Expected status 'active', got '%s'", result.Status)
	}
	if result.Intent != "plan" {
		t.Errorf("Expected intent 'plan', got '%s'", result.Intent)
	}
	if result.Subject == nil || result.Subject.Reference != "Patient/12345" {
		t.Error("Expected patient subject reference")
	}

	// Check title and description
	if result.Title != "Diabetes Management Plan" {
		t.Errorf("Expected title, got '%s'", result.Title)
	}
	if result.Description != "Comprehensive plan for managing Type 2 diabetes" {
		t.Errorf("Expected description, got '%s'", result.Description)
	}

	// Check category (must include assess-plan for US Core)
	if len(result.Category) < 1 {
		t.Fatal("Expected at least one category")
	}
	foundAssessPlan := false
	for _, cat := range result.Category {
		for _, coding := range cat.Coding {
			if coding.Code == "assess-plan" {
				foundAssessPlan = true
				break
			}
		}
	}
	if !foundAssessPlan {
		t.Error("Expected 'assess-plan' category for US Core compliance")
	}

	// Check period
	if result.Period == nil {
		t.Fatal("Expected period")
	}
	if result.Period.Start == nil || result.Period.Start.Year() != 2024 {
		t.Error("Expected start period in 2024")
	}
}

func TestMapCarePlan_StatusMapping(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"draft", "draft"},
		{"active", "active"},
		{"on-hold", "on-hold"},
		{"onhold", "on-hold"},
		{"revoked", "revoked"},
		{"cancelled", "revoked"},
		{"completed", "completed"},
		{"entered-in-error", "entered-in-error"},
		{"unknown", "unknown"},
		{"invalid", "active"}, // Default
	}

	mapper := NewUSCoreMapper()

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			event := &events.CarePlanEvent{
				CarePlan: events.CarePlan{
					Status: tc.input,
				},
			}
			result := mapper.MapCarePlan(event, "Patient/12345")
			if result.Status != tc.expected {
				t.Errorf("Status '%s': expected '%s', got '%s'", tc.input, tc.expected, result.Status)
			}
		})
	}
}

func TestMapCarePlan_IntentMapping(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"proposal", "proposal"},
		{"plan", "plan"},
		{"order", "order"},
		{"option", "option"},
		{"invalid", "plan"}, // Default
	}

	mapper := NewUSCoreMapper()

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			event := &events.CarePlanEvent{
				CarePlan: events.CarePlan{
					Intent: tc.input,
				},
			}
			result := mapper.MapCarePlan(event, "Patient/12345")
			if result.Intent != tc.expected {
				t.Errorf("Intent '%s': expected '%s', got '%s'", tc.input, tc.expected, result.Intent)
			}
		})
	}
}

func TestMapCarePlan_CategoryMapping(t *testing.T) {
	tests := []struct {
		name            string
		input           string
		additionalCodes []string
	}{
		{"empty", "", nil},
		{"discharge", "discharge", []string{"discharge"}},
		{"hospital", "hospital", []string{"hospital"}},
		{"home-health", "home-health", []string{"home-health"}},
		{"custom", "My Custom Plan", nil}, // Should be text-only
	}

	mapper := NewUSCoreMapper()

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			event := &events.CarePlanEvent{
				CarePlan: events.CarePlan{
					Category: tc.input,
				},
			}
			result := mapper.MapCarePlan(event, "Patient/12345")

			// Should always have assess-plan
			foundAssessPlan := false
			for _, cat := range result.Category {
				for _, coding := range cat.Coding {
					if coding.Code == "assess-plan" {
						foundAssessPlan = true
					}
				}
			}
			if !foundAssessPlan {
				t.Error("Missing required assess-plan category")
			}
		})
	}
}

func TestMapCarePlan_WithActivities(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.CarePlanEvent{
		CarePlan: events.CarePlan{
			Status: "active",
			Activities: []events.CarePlanActivity{
				{
					Description:        "Daily blood glucose monitoring",
					Status:             "in-progress",
					Code:               "308113006",
					CodeSystem:         "http://snomed.info/sct",
					OutcomeDescription: "Blood glucose within target range",
				},
				{
					Description:   "Weekly exercise regimen",
					Status:        "scheduled",
					ScheduledDate: "2024-02-01",
				},
			},
		},
	}

	result := mapper.MapCarePlan(event, "Patient/12345")

	if len(result.Activity) != 2 {
		t.Fatalf("Expected 2 activities, got %d", len(result.Activity))
	}

	// Check first activity
	act1 := result.Activity[0]
	if act1.Detail == nil {
		t.Fatal("Expected activity detail")
	}
	if act1.Detail.Status != "in-progress" {
		t.Errorf("Expected status 'in-progress', got '%s'", act1.Detail.Status)
	}
	if act1.Detail.Code == nil || len(act1.Detail.Code.Coding) == 0 {
		t.Fatal("Expected activity code")
	}
	if act1.Detail.Code.Coding[0].Code != "308113006" {
		t.Errorf("Expected code '308113006', got '%s'", act1.Detail.Code.Coding[0].Code)
	}

	// Check outcome
	if len(act1.OutcomeCodeableConcept) == 0 {
		t.Error("Expected outcome codeable concept")
	}

	// Check second activity scheduled date
	act2 := result.Activity[1]
	if act2.Detail.ScheduledString != "2024-02-01" {
		t.Errorf("Expected scheduled date '2024-02-01', got '%s'", act2.Detail.ScheduledString)
	}
}

func TestMapCarePlan_ActivityStatusMapping(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"not-started", "not-started"},
		{"pending", "not-started"},
		{"scheduled", "scheduled"},
		{"in-progress", "in-progress"},
		{"active", "in-progress"},
		{"completed", "completed"},
		{"done", "completed"},
		{"cancelled", "cancelled"},
		{"stopped", "stopped"},
		{"unknown", "unknown"},
		{"invalid", "not-started"}, // Default
	}

	mapper := NewUSCoreMapper()

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			event := &events.CarePlanEvent{
				CarePlan: events.CarePlan{
					Activities: []events.CarePlanActivity{
						{Status: tc.input, Description: "Test"},
					},
				},
			}
			result := mapper.MapCarePlan(event, "Patient/12345")
			if len(result.Activity) == 0 || result.Activity[0].Detail == nil {
				t.Fatal("Expected activity with detail")
			}
			if result.Activity[0].Detail.Status != tc.expected {
				t.Errorf("Activity status '%s': expected '%s', got '%s'", tc.input, tc.expected, result.Activity[0].Detail.Status)
			}
		})
	}
}

func TestMapCarePlan_WithGoalsAndConditions(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.CarePlanEvent{
		CarePlan: events.CarePlan{
			Status:       "active",
			GoalIDs:      []string{"goal-1", "goal-2"},
			ConditionIDs: []string{"cond-diabetes", "cond-hypertension"},
		},
	}

	result := mapper.MapCarePlan(event, "Patient/12345")

	// Check goal references
	if len(result.Goal) != 2 {
		t.Fatalf("Expected 2 goals, got %d", len(result.Goal))
	}
	if result.Goal[0].Reference != "Goal/goal-1" {
		t.Errorf("Expected 'Goal/goal-1', got '%s'", result.Goal[0].Reference)
	}

	// Check condition references
	if len(result.Addresses) != 2 {
		t.Fatalf("Expected 2 addresses (conditions), got %d", len(result.Addresses))
	}
	if result.Addresses[0].Reference != "Condition/cond-diabetes" {
		t.Errorf("Expected 'Condition/cond-diabetes', got '%s'", result.Addresses[0].Reference)
	}
}

func TestMapCarePlan_WithAuthorAndCareTeam(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.CarePlanEvent{
		CarePlan: events.CarePlan{Status: "active"},
		Author: &events.Provider{
			NPI:        "1234567890",
			FamilyName: "Smith",
			GivenName:  "John",
		},
		CareTeam: []*events.Provider{
			{NPI: "0987654321", FamilyName: "Doe", GivenName: "Jane"},
			{ID: "nurse-001", FamilyName: "Brown", GivenName: "Alice"},
		},
	}

	result := mapper.MapCarePlan(event, "Patient/12345")

	// Check author
	if result.Author == nil {
		t.Fatal("Expected author reference")
	}
	if result.Author.Reference != "Practitioner/1234567890" {
		t.Errorf("Expected 'Practitioner/1234567890', got '%s'", result.Author.Reference)
	}

	// Check care team
	if len(result.CareTeam) != 2 {
		t.Fatalf("Expected 2 care team members, got %d", len(result.CareTeam))
	}
}

func TestMapCarePlan_NilEvent(t *testing.T) {
	mapper := NewUSCoreMapper()
	result := mapper.MapCarePlan(nil, "Patient/12345")
	if result != nil {
		t.Error("Expected nil for nil event")
	}
}

func TestMapCarePlan_JSONSerialization(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.CarePlanEvent{
		CarePlan: events.CarePlan{
			Title:       "Diabetes Management",
			Status:      "active",
			Intent:      "plan",
			PeriodStart: "2024-01-01",
		},
	}

	result := mapper.MapCarePlan(event, "Patient/12345")
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	jsonStr := string(data)
	if !strings.Contains(jsonStr, `"resourceType":"CarePlan"`) {
		t.Error("JSON missing resourceType")
	}
	if !strings.Contains(jsonStr, USCoreCarePlanProfile) {
		t.Error("JSON missing profile")
	}
	if !strings.Contains(jsonStr, "Diabetes Management") {
		t.Error("JSON missing title")
	}
	if !strings.Contains(jsonStr, "assess-plan") {
		t.Error("JSON missing required category")
	}
}

// ============================================================================
// Goal Tests
// ============================================================================

func TestMapGoal_BasicMapping(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.GoalEvent{
		EventMeta: events.EventMeta{ID: "goal-001"},
		Goal: events.Goal{
			Description:       "Maintain HbA1c below 7%",
			LifecycleStatus:   "active",
			AchievementStatus: "in-progress",
			Category:          "dietary",
			Priority:          "high",
			StartDate:         "2024-01-01",
			TargetDate:        "2024-06-01",
		},
	}

	result := mapper.MapGoal(event, "Patient/12345")

	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	// Check profile
	if len(result.Meta.Profile) != 1 || result.Meta.Profile[0] != USCoreGoalProfile {
		t.Errorf("Expected US Core Goal profile, got %v", result.Meta.Profile)
	}

	// Check required fields
	if result.LifecycleStatus != "active" {
		t.Errorf("Expected status 'active', got '%s'", result.LifecycleStatus)
	}
	if result.Description == nil || result.Description.Text != "Maintain HbA1c below 7%" {
		t.Error("Expected description")
	}
	if result.Subject == nil || result.Subject.Reference != "Patient/12345" {
		t.Error("Expected patient subject reference")
	}

	// Check optional fields
	if result.StartDate != "2024-01-01" {
		t.Errorf("Expected start date '2024-01-01', got '%s'", result.StartDate)
	}

	// Check achievement status
	if result.AchievementStatus == nil {
		t.Fatal("Expected achievement status")
	}
	if len(result.AchievementStatus.Coding) == 0 || result.AchievementStatus.Coding[0].Code != "in-progress" {
		t.Error("Expected achievement status 'in-progress'")
	}

	// Check priority
	if result.Priority == nil {
		t.Fatal("Expected priority")
	}
	if len(result.Priority.Coding) == 0 || result.Priority.Coding[0].Code != "high-priority" {
		t.Error("Expected priority 'high-priority'")
	}
}

func TestMapGoal_LifecycleStatusMapping(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"proposed", "proposed"},
		{"planned", "planned"},
		{"accepted", "accepted"},
		{"active", "active"},
		{"on-hold", "on-hold"},
		{"onhold", "on-hold"},
		{"completed", "completed"},
		{"cancelled", "cancelled"},
		{"canceled", "cancelled"},
		{"entered-in-error", "entered-in-error"},
		{"rejected", "rejected"},
		{"invalid", "active"}, // Default
	}

	mapper := NewUSCoreMapper()

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			event := &events.GoalEvent{
				Goal: events.Goal{
					Description:     "Test goal",
					LifecycleStatus: tc.input,
				},
			}
			result := mapper.MapGoal(event, "Patient/12345")
			if result.LifecycleStatus != tc.expected {
				t.Errorf("Status '%s': expected '%s', got '%s'", tc.input, tc.expected, result.LifecycleStatus)
			}
		})
	}
}

func TestMapGoal_AchievementStatusMapping(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"in-progress", "in-progress"},
		{"improving", "improving"},
		{"worsening", "worsening"},
		{"no-change", "no-change"},
		{"achieved", "achieved"},
		{"sustaining", "sustaining"},
		{"not-achieved", "not-achieved"},
		{"no-progress", "no-progress"},
		{"not-attainable", "not-attainable"},
	}

	mapper := NewUSCoreMapper()

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			event := &events.GoalEvent{
				Goal: events.Goal{
					Description:       "Test goal",
					AchievementStatus: tc.input,
				},
			}
			result := mapper.MapGoal(event, "Patient/12345")
			if result.AchievementStatus == nil || len(result.AchievementStatus.Coding) == 0 {
				t.Fatal("Expected achievement status")
			}
			if result.AchievementStatus.Coding[0].Code != tc.expected {
				t.Errorf("Achievement status '%s': expected '%s', got '%s'", tc.input, tc.expected, result.AchievementStatus.Coding[0].Code)
			}
		})
	}
}

func TestMapGoal_PriorityMapping(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"high", "high-priority"},
		{"high-priority", "high-priority"},
		{"medium", "medium-priority"},
		{"normal", "medium-priority"},
		{"low", "low-priority"},
		{"low-priority", "low-priority"},
	}

	mapper := NewUSCoreMapper()

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			event := &events.GoalEvent{
				Goal: events.Goal{
					Description: "Test goal",
					Priority:    tc.input,
				},
			}
			result := mapper.MapGoal(event, "Patient/12345")
			if result.Priority == nil || len(result.Priority.Coding) == 0 {
				t.Fatal("Expected priority")
			}
			if result.Priority.Coding[0].Code != tc.expected {
				t.Errorf("Priority '%s': expected '%s', got '%s'", tc.input, tc.expected, result.Priority.Coding[0].Code)
			}
		})
	}
}

func TestMapGoal_CategoryMapping(t *testing.T) {
	tests := []struct {
		input      string
		hasSNOMED  bool
		snomedCode string
	}{
		{"dietary", true, "289141003"},
		{"safety", true, "410518001"},
		{"behavioral", true, "363879005"},
		{"nursing", true, "365857007"},
		{"physiotherapy", true, "410602005"},
		{"custom category", false, ""}, // Text only
	}

	mapper := NewUSCoreMapper()

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			event := &events.GoalEvent{
				Goal: events.Goal{
					Description: "Test goal",
					Category:    tc.input,
				},
			}
			result := mapper.MapGoal(event, "Patient/12345")
			if len(result.Category) == 0 {
				t.Fatal("Expected category")
			}
			cat := result.Category[0]

			if tc.hasSNOMED {
				if len(cat.Coding) == 0 || cat.Coding[0].Code != tc.snomedCode {
					t.Errorf("Category '%s': expected SNOMED code '%s'", tc.input, tc.snomedCode)
				}
			}
		})
	}
}

func TestMapGoal_WithTarget(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.GoalEvent{
		Goal: events.Goal{
			Description: "Reduce HbA1c",
			Target: &events.GoalTarget{
				Measure:        "4548-4",
				MeasureSystem:  "http://loinc.org",
				DetailQuantity: 6.5,
				DetailUnit:     "%",
				DueDate:        "2024-06-01",
			},
		},
	}

	result := mapper.MapGoal(event, "Patient/12345")

	if len(result.Target) != 1 {
		t.Fatalf("Expected 1 target, got %d", len(result.Target))
	}

	target := result.Target[0]

	// Check measure
	if target.Measure == nil || len(target.Measure.Coding) == 0 {
		t.Fatal("Expected target measure")
	}
	if target.Measure.Coding[0].Code != "4548-4" {
		t.Errorf("Expected measure code '4548-4', got '%s'", target.Measure.Coding[0].Code)
	}

	// Check detail quantity
	if target.DetailQuantity == nil {
		t.Fatal("Expected detail quantity")
	}
	if target.DetailQuantity.Value != 6.5 {
		t.Errorf("Expected value 6.5, got %f", target.DetailQuantity.Value)
	}
	if target.DetailQuantity.Unit != "%" {
		t.Errorf("Expected unit '%%', got '%s'", target.DetailQuantity.Unit)
	}

	// Check due date
	if target.DueDate != "2024-06-01" {
		t.Errorf("Expected due date '2024-06-01', got '%s'", target.DueDate)
	}
}

func TestMapGoal_TargetWithStringDetail(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.GoalEvent{
		Goal: events.Goal{
			Description: "Improve mobility",
			Target: &events.GoalTarget{
				DetailString: "Walk 30 minutes daily without pain",
				DueDate:      "2024-03-01",
			},
		},
	}

	result := mapper.MapGoal(event, "Patient/12345")

	if len(result.Target) != 1 {
		t.Fatalf("Expected 1 target, got %d", len(result.Target))
	}

	target := result.Target[0]
	if target.DetailString != "Walk 30 minutes daily without pain" {
		t.Errorf("Expected detail string, got '%s'", target.DetailString)
	}
}

func TestMapGoal_TargetUnitMapping(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"kg", "kg"},
		{"pounds", "[lb_av]"},
		{"mmHg", "mm[Hg]"},
		{"mg/dL", "mg/dL"},
		{"%", "%"},
		{"steps", "{steps}"},
		{"min", "min"},
	}

	mapper := NewUSCoreMapper()

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			event := &events.GoalEvent{
				Goal: events.Goal{
					Description: "Test goal",
					Target: &events.GoalTarget{
						DetailQuantity: 100,
						DetailUnit:     tc.input,
					},
				},
			}
			result := mapper.MapGoal(event, "Patient/12345")
			if len(result.Target) == 0 || result.Target[0].DetailQuantity == nil {
				t.Fatal("Expected target with quantity")
			}
			if result.Target[0].DetailQuantity.Code != tc.expected {
				t.Errorf("Unit '%s': expected code '%s', got '%s'", tc.input, tc.expected, result.Target[0].DetailQuantity.Code)
			}
		})
	}
}

func TestMapGoal_WithExpressedBy(t *testing.T) {
	mapper := NewUSCoreMapper()

	tests := []struct {
		name        string
		expressedBy string
		author      *events.Provider
		expectType  string
	}{
		{"patient", "patient", nil, "Patient"},
		{"practitioner", "practitioner", nil, "Practitioner"},
		{"related person", "family", nil, "RelatedPerson"},
		{"with author", "", &events.Provider{NPI: "1234567890", FamilyName: "Smith"}, ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			event := &events.GoalEvent{
				Goal: events.Goal{
					Description: "Test goal",
					ExpressedBy: tc.expressedBy,
				},
				Author: tc.author,
			}
			result := mapper.MapGoal(event, "Patient/12345")

			if result.ExpressedBy == nil {
				t.Fatal("Expected expressedBy")
			}

			if tc.author != nil {
				if result.ExpressedBy.Reference != "Practitioner/1234567890" {
					t.Errorf("Expected practitioner reference from author")
				}
			} else if tc.expectType != "" {
				if result.ExpressedBy.Type != tc.expectType {
					t.Errorf("Expected type '%s', got '%s'", tc.expectType, result.ExpressedBy.Type)
				}
			}
		})
	}
}

func TestMapGoal_WithAddresses(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.GoalEvent{
		Goal: events.Goal{
			Description:  "Lower blood pressure",
			AddressesIDs: []string{"cond-hypertension", "cond-heart-disease"},
		},
	}

	result := mapper.MapGoal(event, "Patient/12345")

	if len(result.Addresses) != 2 {
		t.Fatalf("Expected 2 addresses, got %d", len(result.Addresses))
	}
	if result.Addresses[0].Reference != "Condition/cond-hypertension" {
		t.Errorf("Expected 'Condition/cond-hypertension', got '%s'", result.Addresses[0].Reference)
	}
}

func TestMapGoal_WithNote(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.GoalEvent{
		Goal: events.Goal{
			Description: "Test goal",
			Note:        "Patient motivated to achieve this goal",
		},
	}

	result := mapper.MapGoal(event, "Patient/12345")

	if len(result.Note) != 1 {
		t.Fatalf("Expected 1 note, got %d", len(result.Note))
	}
	if result.Note[0].Text != "Patient motivated to achieve this goal" {
		t.Errorf("Expected note text, got '%s'", result.Note[0].Text)
	}
}

func TestMapGoal_StatusDates(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.GoalEvent{
		Goal: events.Goal{
			Description:  "Test goal",
			StatusDate:   "2024-02-15",
			StatusReason: "Patient making good progress",
		},
	}

	result := mapper.MapGoal(event, "Patient/12345")

	if result.StatusDate != "2024-02-15" {
		t.Errorf("Expected status date '2024-02-15', got '%s'", result.StatusDate)
	}
	if result.StatusReason != "Patient making good progress" {
		t.Errorf("Expected status reason, got '%s'", result.StatusReason)
	}
}

func TestMapGoal_TargetDateWithoutTarget(t *testing.T) {
	mapper := NewUSCoreMapper()

	// When targetDate is set but no Target struct, should create minimal target
	event := &events.GoalEvent{
		Goal: events.Goal{
			Description: "Test goal",
			TargetDate:  "2024-12-31",
		},
	}

	result := mapper.MapGoal(event, "Patient/12345")

	if len(result.Target) != 1 {
		t.Fatalf("Expected 1 target from targetDate, got %d", len(result.Target))
	}
	if result.Target[0].DueDate != "2024-12-31" {
		t.Errorf("Expected due date '2024-12-31', got '%s'", result.Target[0].DueDate)
	}
}

func TestMapGoal_NilEvent(t *testing.T) {
	mapper := NewUSCoreMapper()
	result := mapper.MapGoal(nil, "Patient/12345")
	if result != nil {
		t.Error("Expected nil for nil event")
	}
}

func TestMapGoal_JSONSerialization(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.GoalEvent{
		Goal: events.Goal{
			Description:       "Maintain healthy weight",
			LifecycleStatus:   "active",
			AchievementStatus: "in-progress",
			Priority:          "high",
			Target: &events.GoalTarget{
				DetailQuantity: 75,
				DetailUnit:     "kg",
				DueDate:        "2024-06-01",
			},
		},
	}

	result := mapper.MapGoal(event, "Patient/12345")
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	jsonStr := string(data)
	if !strings.Contains(jsonStr, `"resourceType":"Goal"`) {
		t.Error("JSON missing resourceType")
	}
	if !strings.Contains(jsonStr, USCoreGoalProfile) {
		t.Error("JSON missing profile")
	}
	if !strings.Contains(jsonStr, "Maintain healthy weight") {
		t.Error("JSON missing description")
	}
	if !strings.Contains(jsonStr, `"lifecycleStatus":"active"`) {
		t.Error("JSON missing lifecycle status")
	}
	if !strings.Contains(jsonStr, "high-priority") {
		t.Error("JSON missing priority")
	}
}

// ============================================================================
// CareTeam Tests
// ============================================================================

func TestMapCareTeam_BasicMapping(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.CareTeamEvent{
		EventMeta: events.EventMeta{ID: "ct-001"},
		CareTeam: events.CareTeam{
			Name:        "Diabetes Care Team",
			Status:      "active",
			Category:    "longitudinal",
			PeriodStart: "2024-01-01",
			PeriodEnd:   "2024-12-31",
			Note:        "Primary care team for ongoing diabetes management",
		},
	}

	result := mapper.MapCareTeam(event, "Patient/12345")

	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	// Check profile
	if len(result.Meta.Profile) != 1 || result.Meta.Profile[0] != USCoreCareTeamProfile {
		t.Errorf("Expected US Core CareTeam profile, got %v", result.Meta.Profile)
	}

	// Check required fields
	if result.Status != "active" {
		t.Errorf("Expected status 'active', got '%s'", result.Status)
	}
	if result.Subject == nil || result.Subject.Reference != "Patient/12345" {
		t.Error("Expected patient subject reference")
	}

	// Check name
	if result.Name != "Diabetes Care Team" {
		t.Errorf("Expected name, got '%s'", result.Name)
	}

	// Check period
	if result.Period == nil {
		t.Fatal("Expected period")
	}
	if result.Period.Start == nil || result.Period.Start.Year() != 2024 {
		t.Error("Expected start period in 2024")
	}

	// Check note
	if len(result.Note) != 1 {
		t.Fatal("Expected one note")
	}
	if result.Note[0].Text != "Primary care team for ongoing diabetes management" {
		t.Error("Note text not mapped correctly")
	}
}

func TestMapCareTeam_StatusMapping(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"proposed", "proposed"},
		{"active", "active"},
		{"suspended", "suspended"},
		{"inactive", "inactive"},
		{"entered-in-error", "entered-in-error"},
		{"on-hold", "suspended"},
		{"onhold", "suspended"},
		{"error", "entered-in-error"},
		{"invalid", "active"}, // Default
	}

	mapper := NewUSCoreMapper()

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			event := &events.CareTeamEvent{
				CareTeam: events.CareTeam{
					Status: tc.input,
				},
			}
			result := mapper.MapCareTeam(event, "Patient/12345")
			if result.Status != tc.expected {
				t.Errorf("Status '%s': expected '%s', got '%s'", tc.input, tc.expected, result.Status)
			}
		})
	}
}

func TestMapCareTeam_CategoryMapping(t *testing.T) {
	tests := []struct {
		input       string
		expectedSys string
		expected    string
	}{
		{"longitudinal", SystemCareTeamCategory, "LA27976-2"},
		{"longitudinal-care", SystemCareTeamCategory, "LA27976-2"},
		{"episode", SystemCareTeamCategory, "LA27977-0"},
		{"episode-of-care", SystemCareTeamCategory, "LA27977-0"},
		{"condition", SystemCareTeamCategory, "LA27978-8"},
		{"condition-focused", SystemCareTeamCategory, "LA27978-8"},
		{"encounter", SystemCareTeamCategory, "LA28865-6"},
		{"home-health", SystemCareTeamCategory, "LA28866-4"},
		{"hcbs", SystemCareTeamCategory, "LA28866-4"},
		{"clinical-research", SystemCareTeamCategory, "LA28867-2"},
		{"public-health", SystemCareTeamCategory, "LA28868-0"},
	}

	mapper := NewUSCoreMapper()

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			event := &events.CareTeamEvent{
				CareTeam: events.CareTeam{
					Category: tc.input,
				},
			}
			result := mapper.MapCareTeam(event, "Patient/12345")
			if len(result.Category) < 1 {
				t.Fatal("Expected at least one category")
			}
			found := false
			for _, cat := range result.Category {
				for _, coding := range cat.Coding {
					if coding.Code == tc.expected && coding.System == tc.expectedSys {
						found = true
						break
					}
				}
			}
			if !found {
				t.Errorf("Category '%s': expected LOINC code '%s'", tc.input, tc.expected)
			}
		})
	}
}

func TestMapCareTeam_WithParticipants(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.CareTeamEvent{
		CareTeam: events.CareTeam{
			Status: "active",
			Members: []events.CareTeamMember{
				{
					Role: "primary care provider",
					Provider: &events.Provider{
						ID:         "prov-001",
						GivenName:  "Jane",
						FamilyName: "Smith",
						Suffix:     "MD",
					},
					PeriodStart: "2024-01-01",
				},
				{
					Role:             "case manager",
					OrganizationID:   "org-001",
					OrganizationName: "Care Management Services",
				},
			},
		},
	}

	result := mapper.MapCareTeam(event, "Patient/12345")

	if len(result.Participant) != 2 {
		t.Fatalf("Expected 2 participants, got %d", len(result.Participant))
	}

	// Check first participant (provider)
	if result.Participant[0].Member == nil {
		t.Error("Expected first participant to have member reference")
	}
	if result.Participant[0].Member.Reference != "Practitioner/prov-001" {
		t.Errorf("Expected Practitioner reference, got %s", result.Participant[0].Member.Reference)
	}
	if result.Participant[0].Period == nil {
		t.Error("Expected period on first participant")
	}

	// Check second participant (organization)
	if result.Participant[1].Member == nil {
		t.Error("Expected second participant to have member reference")
	}
	if result.Participant[1].Member.Reference != "Organization/org-001" {
		t.Errorf("Expected Organization reference, got %s", result.Participant[1].Member.Reference)
	}
}

func TestMapCareTeam_ParticipantRoleMapping(t *testing.T) {
	tests := []struct {
		role         string
		expectedCode string
	}{
		{"primary care provider", "446050000"},
		{"pcp", "446050000"},
		{"nurse", "224535009"},
		{"rn", "224535009"},
		{"case manager", "768820003"},
		{"specialist", "309395003"},
		{"pharmacist", "46255001"},
		{"social worker", "106328005"},
		{"physical therapist", "36682004"},
		{"dietitian", "159033005"},
		{"psychologist", "59944000"},
	}

	mapper := NewUSCoreMapper()

	for _, tc := range tests {
		t.Run(tc.role, func(t *testing.T) {
			event := &events.CareTeamEvent{
				CareTeam: events.CareTeam{
					Status: "active",
					Members: []events.CareTeamMember{
						{
							Role: tc.role,
							Provider: &events.Provider{
								ID: "prov-001",
							},
						},
					},
				},
			}
			result := mapper.MapCareTeam(event, "Patient/12345")
			if len(result.Participant) < 1 {
				t.Fatal("Expected at least one participant")
			}
			if len(result.Participant[0].Role) < 1 {
				t.Fatal("Expected at least one role")
			}
			found := false
			for _, role := range result.Participant[0].Role {
				for _, coding := range role.Coding {
					if coding.Code == tc.expectedCode {
						found = true
						break
					}
				}
			}
			if !found {
				t.Errorf("Role '%s': expected SNOMED code '%s'", tc.role, tc.expectedCode)
			}
		})
	}
}

func TestMapCareTeam_WithManagingOrganization(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.CareTeamEvent{
		CareTeam: events.CareTeam{
			Status:                   "active",
			ManagingOrganizationID:   "org-manage-001",
			ManagingOrganizationName: "Primary Care Associates",
		},
	}

	result := mapper.MapCareTeam(event, "Patient/12345")

	if len(result.ManagingOrganization) != 1 {
		t.Fatalf("Expected 1 managing organization, got %d", len(result.ManagingOrganization))
	}
	if result.ManagingOrganization[0].Reference != "Organization/org-manage-001" {
		t.Errorf("Expected Organization reference, got %s", result.ManagingOrganization[0].Reference)
	}
	if result.ManagingOrganization[0].Display != "Primary Care Associates" {
		t.Errorf("Expected display name, got %s", result.ManagingOrganization[0].Display)
	}
}

func TestMapCareTeam_WithReasonAndConditions(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.CareTeamEvent{
		CareTeam: events.CareTeam{
			Status:           "active",
			ReasonCode:       "E11.9",
			ReasonCodeSystem: "http://hl7.org/fhir/sid/icd-10-cm",
			ReasonText:       "Type 2 diabetes mellitus",
			ConditionIDs:     []string{"cond-001", "cond-002"},
		},
	}

	result := mapper.MapCareTeam(event, "Patient/12345")

	// Check reason code
	if len(result.ReasonCode) != 1 {
		t.Fatalf("Expected 1 reason code, got %d", len(result.ReasonCode))
	}
	if len(result.ReasonCode[0].Coding) < 1 {
		t.Fatal("Expected reason code coding")
	}
	if result.ReasonCode[0].Coding[0].Code != "E11.9" {
		t.Errorf("Expected reason code E11.9, got %s", result.ReasonCode[0].Coding[0].Code)
	}

	// Check reason references (conditions)
	if len(result.ReasonReference) != 2 {
		t.Fatalf("Expected 2 reason references, got %d", len(result.ReasonReference))
	}
	if result.ReasonReference[0].Reference != "Condition/cond-001" {
		t.Errorf("Expected Condition reference, got %s", result.ReasonReference[0].Reference)
	}
}

func TestMapCareTeam_NilEvent(t *testing.T) {
	mapper := NewUSCoreMapper()
	result := mapper.MapCareTeam(nil, "Patient/12345")
	if result != nil {
		t.Error("Expected nil result for nil event")
	}
}

func TestMapCareTeam_JSONSerialization(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.CareTeamEvent{
		CareTeam: events.CareTeam{
			Name:     "Oncology Care Team",
			Status:   "active",
			Category: "condition-focused",
			Members: []events.CareTeamMember{
				{
					Role: "specialist",
					Provider: &events.Provider{
						ID:        "onc-001",
						GivenName: "Dr. Smith",
					},
				},
			},
		},
	}

	result := mapper.MapCareTeam(event, "Patient/12345")
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	jsonStr := string(data)
	if !strings.Contains(jsonStr, `"resourceType":"CareTeam"`) {
		t.Error("JSON missing resourceType")
	}
	if !strings.Contains(jsonStr, USCoreCareTeamProfile) {
		t.Error("JSON missing profile")
	}
	if !strings.Contains(jsonStr, "Oncology Care Team") {
		t.Error("JSON missing name")
	}
	if !strings.Contains(jsonStr, `"status":"active"`) {
		t.Error("JSON missing status")
	}
}

// ============================================================================
// ServiceRequest Tests
// ============================================================================

func TestMapServiceRequest_BasicMapping(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.ServiceRequestEvent{
		EventMeta: events.EventMeta{ID: "sr-001"},
		ServiceRequest: events.ServiceRequest{
			Status:             "active",
			Intent:             "order",
			Category:           "laboratory",
			Code:               "80053",
			CodeSystem:         "http://www.ama-assn.org/go/cpt",
			CodeText:           "Comprehensive metabolic panel",
			AuthoredOn:         "2024-01-15",
			OccurrenceDateTime: "2024-01-20",
			Note:               "Fasting required",
		},
		Requester: &events.Provider{
			ID:         "doc-001",
			GivenName:  "John",
			FamilyName: "Doe",
		},
	}

	result := mapper.MapServiceRequest(event, "Patient/12345")

	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	// Check profile
	if len(result.Meta.Profile) != 1 || result.Meta.Profile[0] != USCoreServiceRequestProfile {
		t.Errorf("Expected US Core ServiceRequest profile, got %v", result.Meta.Profile)
	}

	// Check required fields
	if result.Status != "active" {
		t.Errorf("Expected status 'active', got '%s'", result.Status)
	}
	if result.Intent != "order" {
		t.Errorf("Expected intent 'order', got '%s'", result.Intent)
	}
	if result.Subject == nil || result.Subject.Reference != "Patient/12345" {
		t.Error("Expected patient subject reference")
	}

	// Check code
	if result.Code == nil || len(result.Code.Coding) < 1 {
		t.Fatal("Expected code with coding")
	}
	if result.Code.Coding[0].Code != "80053" {
		t.Errorf("Expected code 80053, got %s", result.Code.Coding[0].Code)
	}
	if result.Code.Text != "Comprehensive metabolic panel" {
		t.Errorf("Expected code text, got %s", result.Code.Text)
	}

	// Check requester
	if result.Requester == nil {
		t.Fatal("Expected requester")
	}
	if result.Requester.Reference != "Practitioner/doc-001" {
		t.Errorf("Expected Practitioner reference, got %s", result.Requester.Reference)
	}

	// Check note
	if len(result.Note) != 1 || result.Note[0].Text != "Fasting required" {
		t.Error("Expected note")
	}
}

func TestMapServiceRequest_StatusMapping(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"draft", "draft"},
		{"active", "active"},
		{"on-hold", "on-hold"},
		{"revoked", "revoked"},
		{"completed", "completed"},
		{"entered-in-error", "entered-in-error"},
		{"unknown", "unknown"},
		{"cancelled", "revoked"},
		{"pending", "active"},
		{"invalid", "active"}, // Default
	}

	mapper := NewUSCoreMapper()

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			event := &events.ServiceRequestEvent{
				ServiceRequest: events.ServiceRequest{
					Status: tc.input,
					Intent: "order",
				},
			}
			result := mapper.MapServiceRequest(event, "Patient/12345")
			if result.Status != tc.expected {
				t.Errorf("Status '%s': expected '%s', got '%s'", tc.input, tc.expected, result.Status)
			}
		})
	}
}

func TestMapServiceRequest_IntentMapping(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"proposal", "proposal"},
		{"plan", "plan"},
		{"directive", "directive"},
		{"order", "order"},
		{"original-order", "original-order"},
		{"reflex-order", "reflex-order"},
		{"filler-order", "filler-order"},
		{"instance-order", "instance-order"},
		{"option", "option"},
		{"invalid", "order"}, // Default
	}

	mapper := NewUSCoreMapper()

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			event := &events.ServiceRequestEvent{
				ServiceRequest: events.ServiceRequest{
					Intent: tc.input,
				},
			}
			result := mapper.MapServiceRequest(event, "Patient/12345")
			if result.Intent != tc.expected {
				t.Errorf("Intent '%s': expected '%s', got '%s'", tc.input, tc.expected, result.Intent)
			}
		})
	}
}

func TestMapServiceRequest_CategoryMapping(t *testing.T) {
	tests := []struct {
		input       string
		expectedSys string
		expected    string
	}{
		{"laboratory", SystemServiceRequestCategory, "108252007"},
		{"lab", SystemServiceRequestCategory, "108252007"},
		{"imaging", SystemServiceRequestCategory, "363679005"},
		{"radiology", SystemServiceRequestCategory, "363679005"},
		{"referral", SystemServiceRequestCategory, "3457005"},
		{"consultation", SystemServiceRequestCategory, "11429006"},
		{"consult", SystemServiceRequestCategory, "11429006"},
		{"procedure", SystemServiceRequestCategory, "387713003"},
		{"surgical", SystemServiceRequestCategory, "387713003"},
		{"counseling", SystemServiceRequestCategory, "409063005"},
		{"therapy", SystemServiceRequestCategory, "276239002"},
		{"education", SystemServiceRequestCategory, "311401005"},
		{"patient-education", SystemServiceRequestCategory, "311401005"},
		{"screening", SystemServiceRequestCategory, "360156006"},
		{"assessment", SystemServiceRequestCategory, "386053000"},
		{"evaluation", SystemServiceRequestCategory, "386053000"},
	}

	mapper := NewUSCoreMapper()

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			event := &events.ServiceRequestEvent{
				ServiceRequest: events.ServiceRequest{
					Category: tc.input,
				},
			}
			result := mapper.MapServiceRequest(event, "Patient/12345")
			if len(result.Category) < 1 {
				t.Fatal("Expected at least one category")
			}
			found := false
			for _, cat := range result.Category {
				for _, coding := range cat.Coding {
					if coding.Code == tc.expected && coding.System == tc.expectedSys {
						found = true
						break
					}
				}
			}
			if !found {
				t.Errorf("Category '%s': expected SNOMED code '%s'", tc.input, tc.expected)
			}
		})
	}
}

func TestMapServiceRequest_PriorityMapping(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"routine", "routine"},
		{"normal", "routine"},
		{"urgent", "urgent"},
		{"asap", "asap"},
		{"stat", "stat"},
		{"emergent", "stat"},
		{"emergency", "stat"},
		{"invalid", "routine"}, // Default
	}

	mapper := NewUSCoreMapper()

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			event := &events.ServiceRequestEvent{
				ServiceRequest: events.ServiceRequest{
					Priority: tc.input,
				},
			}
			result := mapper.MapServiceRequest(event, "Patient/12345")
			if result.Priority != tc.expected {
				t.Errorf("Priority '%s': expected '%s', got '%s'", tc.input, tc.expected, result.Priority)
			}
		})
	}
}

func TestMapServiceRequest_CodeSystemDetection(t *testing.T) {
	tests := []struct {
		name           string
		code           string
		expectedSystem string
	}{
		{"CPT code", "80053", "http://www.ama-assn.org/go/cpt"},
		{"LOINC code", "2951-2", "http://loinc.org"},
		{"HCPCS code", "G0101", "http://www.cms.gov/Medicare/Coding/HCPCSReleaseCodeSets"},
		{"SNOMED code", "122869004", "http://snomed.info/sct"},
	}

	mapper := NewUSCoreMapper()

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			event := &events.ServiceRequestEvent{
				ServiceRequest: events.ServiceRequest{
					Code: tc.code,
				},
			}
			result := mapper.MapServiceRequest(event, "Patient/12345")
			if result.Code == nil || len(result.Code.Coding) < 1 {
				t.Fatal("Expected code with coding")
			}
			if result.Code.Coding[0].System != tc.expectedSystem {
				t.Errorf("Code '%s': expected system '%s', got '%s'", tc.code, tc.expectedSystem, result.Code.Coding[0].System)
			}
		})
	}
}

func TestMapServiceRequest_WithOccurrence(t *testing.T) {
	mapper := NewUSCoreMapper()

	// Test with date
	event := &events.ServiceRequestEvent{
		ServiceRequest: events.ServiceRequest{
			OccurrenceDateTime: "2024-02-01",
		},
	}
	result := mapper.MapServiceRequest(event, "Patient/12345")
	if result.OccurrenceDateTime == "" {
		t.Error("Expected occurrenceDateTime for date")
	}

	// Test with period
	event = &events.ServiceRequestEvent{
		ServiceRequest: events.ServiceRequest{
			OccurrencePeriodStart: "2024-02-01",
			OccurrencePeriodEnd:   "2024-02-15",
		},
	}
	result = mapper.MapServiceRequest(event, "Patient/12345")
	if result.OccurrencePeriod == nil {
		t.Fatal("Expected occurrencePeriod")
	}
	if result.OccurrencePeriod.Start == nil || result.OccurrencePeriod.End == nil {
		t.Error("Expected period with start and end")
	}
}

func TestMapServiceRequest_WithReasonCodes(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.ServiceRequestEvent{
		ServiceRequest: events.ServiceRequest{
			ReasonCode:       "E11.9",
			ReasonCodeSystem: "http://hl7.org/fhir/sid/icd-10-cm",
			ReasonText:       "Type 2 diabetes",
			ConditionIDs:     []string{"cond-001"},
		},
	}

	result := mapper.MapServiceRequest(event, "Patient/12345")

	if len(result.ReasonCode) != 1 {
		t.Fatalf("Expected 1 reason code, got %d", len(result.ReasonCode))
	}
	if result.ReasonCode[0].Coding[0].Code != "E11.9" {
		t.Errorf("Expected reason code E11.9")
	}

	if len(result.ReasonReference) != 1 {
		t.Fatalf("Expected 1 reason reference, got %d", len(result.ReasonReference))
	}
	if result.ReasonReference[0].Reference != "Condition/cond-001" {
		t.Errorf("Expected Condition reference")
	}
}

func TestMapServiceRequest_WithBodySite(t *testing.T) {
	tests := []struct {
		site         string
		expectedCode string
	}{
		{"head", "69536005"},
		{"neck", "45048000"},
		{"chest", "51185008"},
		{"thorax", "51185008"},
		{"abdomen", "818983003"},
		{"back", "77568009"},
		{"arm", "53120007"},
		{"upper arm", "40983000"},
		{"forearm", "14975008"},
		{"hand", "85562004"},
		{"leg", "61685007"},
		{"thigh", "68367000"},
		{"knee", "72696002"},
		{"foot", "56459004"},
	}

	mapper := NewUSCoreMapper()

	for _, tc := range tests {
		t.Run(tc.site, func(t *testing.T) {
			event := &events.ServiceRequestEvent{
				ServiceRequest: events.ServiceRequest{
					BodySite: tc.site,
				},
			}
			result := mapper.MapServiceRequest(event, "Patient/12345")
			if len(result.BodySite) < 1 {
				t.Fatal("Expected at least one body site")
			}
			found := false
			for _, site := range result.BodySite {
				for _, coding := range site.Coding {
					if coding.Code == tc.expectedCode {
						found = true
						break
					}
				}
			}
			if !found {
				t.Errorf("Body site '%s': expected SNOMED code '%s'", tc.site, tc.expectedCode)
			}
		})
	}
}

func TestMapServiceRequest_WithRequesterAndPerformer(t *testing.T) {
	mapper := NewUSCoreMapper()

	// Test with Practitioner performer (takes precedence over org)
	event := &events.ServiceRequestEvent{
		ServiceRequest: events.ServiceRequest{
			Status: "active",
			Intent: "order",
		},
		Requester: &events.Provider{
			ID:         "doc-001",
			GivenName:  "Jane",
			FamilyName: "Smith",
		},
		Performer: &events.Provider{
			ID:         "spec-001",
			GivenName:  "Bob",
			FamilyName: "Jones",
		},
	}

	result := mapper.MapServiceRequest(event, "Patient/12345")

	// Check requester
	if result.Requester == nil {
		t.Fatal("Expected requester")
	}
	if result.Requester.Reference != "Practitioner/doc-001" {
		t.Errorf("Expected Practitioner reference for requester")
	}

	// Check performer (Practitioner)
	if len(result.Performer) != 1 {
		t.Fatalf("Expected 1 performer, got %d", len(result.Performer))
	}
	if result.Performer[0].Reference != "Practitioner/spec-001" {
		t.Errorf("Expected Practitioner performer, got %s", result.Performer[0].Reference)
	}

	// Test with Organization performer only
	eventOrg := &events.ServiceRequestEvent{
		ServiceRequest: events.ServiceRequest{
			Status: "active",
			Intent: "order",
		},
		PerformerOrgID:   "lab-001",
		PerformerOrgName: "Quest Diagnostics",
	}

	resultOrg := mapper.MapServiceRequest(eventOrg, "Patient/12345")
	if len(resultOrg.Performer) != 1 {
		t.Fatalf("Expected 1 performer for org-only, got %d", len(resultOrg.Performer))
	}
	if resultOrg.Performer[0].Reference != "Organization/lab-001" {
		t.Errorf("Expected Organization performer, got %s", resultOrg.Performer[0].Reference)
	}
}

func TestMapServiceRequest_NilEvent(t *testing.T) {
	mapper := NewUSCoreMapper()
	result := mapper.MapServiceRequest(nil, "Patient/12345")
	if result != nil {
		t.Error("Expected nil result for nil event")
	}
}

func TestMapServiceRequest_JSONSerialization(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.ServiceRequestEvent{
		ServiceRequest: events.ServiceRequest{
			Status:     "active",
			Intent:     "order",
			Category:   "laboratory",
			Priority:   "urgent",
			Code:       "80053",
			CodeText:   "Comprehensive metabolic panel",
			AuthoredOn: "2024-01-15",
		},
	}

	result := mapper.MapServiceRequest(event, "Patient/12345")
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	jsonStr := string(data)
	if !strings.Contains(jsonStr, `"resourceType":"ServiceRequest"`) {
		t.Error("JSON missing resourceType")
	}
	if !strings.Contains(jsonStr, USCoreServiceRequestProfile) {
		t.Error("JSON missing profile")
	}
	if !strings.Contains(jsonStr, `"status":"active"`) {
		t.Error("JSON missing status")
	}
	if !strings.Contains(jsonStr, `"intent":"order"`) {
		t.Error("JSON missing intent")
	}
	if !strings.Contains(jsonStr, `"priority":"urgent"`) {
		t.Error("JSON missing priority")
	}
	if !strings.Contains(jsonStr, "80053") {
		t.Error("JSON missing code")
	}
}

// ============================================================================
// DocumentReference Tests
// ============================================================================

func TestMapDocumentReference_BasicMapping(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.DocumentReferenceEvent{
		EventMeta: events.EventMeta{
			ID:   "doc-001",
			Type: "document_created",
		},
		Patient: &events.Patient{
			MRN:        "12345",
			GivenName:  "John",
			FamilyName: "Doe",
		},
		DocumentReference: events.DocumentReference{
			Status:         "current",
			DocStatus:      "final",
			Type:           "Discharge summary",
			TypeCode:       "18842-5",
			TypeCodeSystem: "http://loinc.org",
			Category:       "Clinical Note",
			Date:           "2024-01-15T10:30:00Z",
			Description:    "Patient discharge summary",
			Content: []events.DocumentReferenceContent{
				{
					AttachmentContentType: "application/pdf",
					AttachmentURL:         "https://example.com/docs/discharge.pdf",
					AttachmentTitle:       "Discharge Summary",
				},
			},
		},
		Author: &events.Provider{
			NPI:        "1234567890",
			GivenName:  "Jane",
			FamilyName: "Smith",
		},
	}

	result := mapper.MapDocumentReference(event, "Patient/12345")

	if result == nil {
		t.Fatal("Expected non-nil result")
	}
	// ResourceType is set via MarshalJSON, verified in JSON serialization test
	if len(result.Meta.Profile) != 1 || result.Meta.Profile[0] != USCoreDocumentReferenceProfile {
		t.Errorf("Profile = %v, want [%s]", result.Meta.Profile, USCoreDocumentReferenceProfile)
	}
	if result.Status != "current" {
		t.Errorf("Status = %q, want 'current'", result.Status)
	}
	if result.DocStatus != "final" {
		t.Errorf("DocStatus = %q, want 'final'", result.DocStatus)
	}
	if result.Subject.Reference != "Patient/12345" {
		t.Errorf("Subject.Reference = %q, want 'Patient/12345'", result.Subject.Reference)
	}
	if result.Type == nil {
		t.Fatal("Type is nil")
	}
	if len(result.Type.Coding) == 0 || result.Type.Coding[0].Code != "18842-5" {
		t.Error("Type.Coding missing LOINC code 18842-5")
	}
	if len(result.Category) == 0 {
		t.Fatal("Category is empty")
	}
	if result.Date != "2024-01-15T10:30:00Z" {
		t.Errorf("Date = %q, want '2024-01-15T10:30:00Z'", result.Date)
	}
	if result.Description != "Patient discharge summary" {
		t.Errorf("Description = %q, want 'Patient discharge summary'", result.Description)
	}
	if len(result.Author) == 0 {
		t.Error("Author is empty")
	}
	if len(result.Content) == 0 {
		t.Fatal("Content is empty")
	}
	if result.Content[0].Attachment.ContentType != "application/pdf" {
		t.Errorf("Content.Attachment.ContentType = %q, want 'application/pdf'", result.Content[0].Attachment.ContentType)
	}
}

func TestMapDocumentReference_StatusMapping(t *testing.T) {
	mapper := NewUSCoreMapper()

	tests := []struct {
		input    string
		expected string
	}{
		{"current", "current"},
		{"active", "current"},
		{"superseded", "superseded"},
		{"replaced", "superseded"},
		{"entered-in-error", "entered-in-error"},
		{"error", "entered-in-error"},
		{"unknown", "current"}, // Default
	}

	for _, tt := range tests {
		event := &events.DocumentReferenceEvent{
			DocumentReference: events.DocumentReference{
				Status: tt.input,
			},
		}
		result := mapper.MapDocumentReference(event, "Patient/12345")
		if result.Status != tt.expected {
			t.Errorf("Status for input %q = %q, want %q", tt.input, result.Status, tt.expected)
		}
	}
}

func TestMapDocumentReference_DocStatusMapping(t *testing.T) {
	mapper := NewUSCoreMapper()

	tests := []struct {
		input    string
		expected string
	}{
		{"preliminary", "preliminary"},
		{"draft", "preliminary"},
		{"final", "final"},
		{"amended", "amended"},
		{"corrected", "amended"},
		{"unknown", "final"}, // Default
	}

	for _, tt := range tests {
		event := &events.DocumentReferenceEvent{
			DocumentReference: events.DocumentReference{
				DocStatus: tt.input,
			},
		}
		result := mapper.MapDocumentReference(event, "Patient/12345")
		if result.DocStatus != tt.expected {
			t.Errorf("DocStatus for input %q = %q, want %q", tt.input, result.DocStatus, tt.expected)
		}
	}
}

func TestMapDocumentReference_TypeMapping(t *testing.T) {
	mapper := NewUSCoreMapper()

	tests := []struct {
		text     string
		code     string
		expected string
	}{
		{"Discharge summary", "", "18842-5"},
		{"Progress note", "", "11506-3"},
		{"H&P", "", "34117-2"},
		{"Consultation", "", "11488-4"},
		{"Operative note", "", "11504-8"},
		{"Referral", "", "57133-1"},
		{"CCD", "", "34133-9"},
		{"Radiology report", "", "18748-4"},
		{"Pathology report", "", "11526-1"},
		{"Lab report", "", "11502-2"},
	}

	for _, tt := range tests {
		event := &events.DocumentReferenceEvent{
			DocumentReference: events.DocumentReference{
				Type:     tt.text,
				TypeCode: tt.code,
			},
		}
		result := mapper.MapDocumentReference(event, "Patient/12345")
		if result.Type == nil || len(result.Type.Coding) == 0 {
			t.Errorf("Type.Coding empty for %q", tt.text)
			continue
		}
		if result.Type.Coding[0].Code != tt.expected {
			t.Errorf("Type code for %q = %q, want %q", tt.text, result.Type.Coding[0].Code, tt.expected)
		}
	}
}

func TestMapDocumentReference_WithSecurityLabel(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.DocumentReferenceEvent{
		DocumentReference: events.DocumentReference{
			Status:        "current",
			SecurityLabel: "R",
		},
	}

	result := mapper.MapDocumentReference(event, "Patient/12345")

	if len(result.SecurityLabel) == 0 {
		t.Fatal("SecurityLabel is empty")
	}
	if len(result.SecurityLabel[0].Coding) == 0 {
		t.Fatal("SecurityLabel.Coding is empty")
	}
	if result.SecurityLabel[0].Coding[0].Code != "R" {
		t.Errorf("SecurityLabel code = %q, want 'R'", result.SecurityLabel[0].Coding[0].Code)
	}
	if result.SecurityLabel[0].Coding[0].System != SystemConfidentiality {
		t.Errorf("SecurityLabel system = %q, want %q", result.SecurityLabel[0].Coding[0].System, SystemConfidentiality)
	}
}

func TestMapDocumentReference_WithContext(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.DocumentReferenceEvent{
		DocumentReference: events.DocumentReference{
			Status: "current",
			Context: &events.DocumentReferenceContext{
				PeriodStart:         "2024-01-15",
				PeriodEnd:           "2024-01-16",
				FacilityType:        "Hospital",
				FacilityTypeCode:    "HOSP",
				PracticeSetting:     "General medicine",
				PracticeSettingCode: "394802001",
			},
		},
		Encounter: &events.Encounter{
			ID: "enc-001",
		},
	}

	result := mapper.MapDocumentReference(event, "Patient/12345")

	if result.Context == nil {
		t.Fatal("Context is nil")
	}
	if result.Context.Period == nil {
		t.Fatal("Context.Period is nil")
	}
	if result.Context.FacilityType == nil {
		t.Error("Context.FacilityType is nil")
	}
	if result.Context.PracticeSetting == nil {
		t.Error("Context.PracticeSetting is nil")
	}
	if len(result.Context.Encounter) == 0 {
		t.Error("Context.Encounter is empty")
	}
}

func TestMapDocumentReference_WithRelatesTo(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.DocumentReferenceEvent{
		DocumentReference: events.DocumentReference{
			Status: "current",
			RelatesTo: []events.DocumentReferenceRelation{
				{Code: "replaces", TargetID: "doc-old-001"},
				{Code: "appends", TargetID: "doc-orig-001"},
			},
		},
	}

	result := mapper.MapDocumentReference(event, "Patient/12345")

	if len(result.RelatesTo) != 2 {
		t.Fatalf("RelatesTo count = %d, want 2", len(result.RelatesTo))
	}
	if result.RelatesTo[0].Code != "replaces" {
		t.Errorf("RelatesTo[0].Code = %q, want 'replaces'", result.RelatesTo[0].Code)
	}
	if result.RelatesTo[0].Target.Reference != "DocumentReference/doc-old-001" {
		t.Errorf("RelatesTo[0].Target = %q, want 'DocumentReference/doc-old-001'", result.RelatesTo[0].Target.Reference)
	}
}

func TestMapDocumentReference_WithCustodian(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.DocumentReferenceEvent{
		DocumentReference: events.DocumentReference{
			Status:        "current",
			CustodianID:   "org-001",
			CustodianName: "General Hospital",
		},
	}

	result := mapper.MapDocumentReference(event, "Patient/12345")

	if result.Custodian == nil {
		t.Fatal("Custodian is nil")
	}
	if result.Custodian.Reference != "Organization/org-001" {
		t.Errorf("Custodian.Reference = %q, want 'Organization/org-001'", result.Custodian.Reference)
	}
	if result.Custodian.Display != "General Hospital" {
		t.Errorf("Custodian.Display = %q, want 'General Hospital'", result.Custodian.Display)
	}
}

func TestMapDocumentReference_WithAuthenticator(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.DocumentReferenceEvent{
		DocumentReference: events.DocumentReference{
			Status: "current",
		},
		Authenticator: &events.Provider{
			NPI:        "9876543210",
			GivenName:  "Bob",
			FamilyName: "Johnson",
		},
	}

	result := mapper.MapDocumentReference(event, "Patient/12345")

	if result.Authenticator == nil {
		t.Fatal("Authenticator is nil")
	}
	if !strings.Contains(result.Authenticator.Reference, "9876543210") {
		t.Error("Authenticator.Reference missing NPI")
	}
}

func TestMapDocumentReference_WithMultipleContent(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.DocumentReferenceEvent{
		DocumentReference: events.DocumentReference{
			Status: "current",
			Content: []events.DocumentReferenceContent{
				{
					AttachmentContentType: "application/pdf",
					AttachmentURL:         "https://example.com/doc.pdf",
					Format:                "urn:ihe:pcc:xphr:2007",
				},
				{
					AttachmentContentType: "text/html",
					AttachmentData:        "PGh0bWw+PC9odG1sPg==",
				},
			},
		},
	}

	result := mapper.MapDocumentReference(event, "Patient/12345")

	if len(result.Content) != 2 {
		t.Fatalf("Content count = %d, want 2", len(result.Content))
	}
	if result.Content[0].Attachment.ContentType != "application/pdf" {
		t.Errorf("Content[0].Attachment.ContentType = %q, want 'application/pdf'", result.Content[0].Attachment.ContentType)
	}
	if result.Content[0].Format == nil {
		t.Error("Content[0].Format is nil")
	}
	if result.Content[1].Attachment.Data != "PGh0bWw+PC9odG1sPg==" {
		t.Error("Content[1].Attachment.Data mismatch")
	}
}

func TestMapDocumentReference_NilEvent(t *testing.T) {
	mapper := NewUSCoreMapper()
	result := mapper.MapDocumentReference(nil, "Patient/12345")
	if result != nil {
		t.Error("Expected nil result for nil event")
	}
}

func TestMapDocumentReference_JSONSerialization(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.DocumentReferenceEvent{
		DocumentReference: events.DocumentReference{
			Status:    "current",
			DocStatus: "final",
			Type:      "Discharge summary",
			TypeCode:  "18842-5",
			Category:  "Clinical Note",
			Content: []events.DocumentReferenceContent{
				{
					AttachmentContentType: "application/pdf",
					AttachmentURL:         "https://example.com/doc.pdf",
				},
			},
		},
	}

	result := mapper.MapDocumentReference(event, "Patient/12345")
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	jsonStr := string(data)
	if !strings.Contains(jsonStr, `"resourceType":"DocumentReference"`) {
		t.Error("JSON missing resourceType")
	}
	if !strings.Contains(jsonStr, USCoreDocumentReferenceProfile) {
		t.Error("JSON missing profile")
	}
	if !strings.Contains(jsonStr, `"status":"current"`) {
		t.Error("JSON missing status")
	}
	if !strings.Contains(jsonStr, "18842-5") {
		t.Error("JSON missing type code")
	}
}

// ============================================================================
// DiagnosticReportNote Tests
// ============================================================================

func TestMapDiagnosticReportNote_BasicMapping(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.DiagnosticReportNoteEvent{
		EventMeta: events.EventMeta{
			ID:   "report-001",
			Type: "diagnostic_report_created",
		},
		Patient: &events.Patient{
			MRN:        "12345",
			GivenName:  "John",
			FamilyName: "Doe",
		},
		DiagnosticReportNote: events.DiagnosticReportNote{
			Status:            "final",
			Category:          "Radiology",
			CategoryCode:      "LP29684-5",
			Code:              "Chest X-ray",
			CodeValue:         "36643-5",
			CodeSystem:        "http://loinc.org",
			EffectiveDateTime: "2024-01-15T10:30:00Z",
			Issued:            "2024-01-15T12:00:00Z",
			Conclusion:        "No acute cardiopulmonary disease",
			PresentedForm: []events.DiagnosticReportAttachment{
				{
					ContentType: "application/pdf",
					URL:         "https://example.com/reports/chest-xray.pdf",
					Title:       "Chest X-Ray Report",
				},
			},
		},
		Performer: &events.Provider{
			NPI:        "1234567890",
			GivenName:  "Jane",
			FamilyName: "Smith",
		},
	}

	result := mapper.MapDiagnosticReportNote(event, "Patient/12345")

	if result == nil {
		t.Fatal("Expected non-nil result")
	}
	// ResourceType is set via MarshalJSON, verified in JSON serialization test
	if len(result.Meta.Profile) != 1 || result.Meta.Profile[0] != USCoreDiagnosticReportNoteProfile {
		t.Errorf("Profile = %v, want [%s]", result.Meta.Profile, USCoreDiagnosticReportNoteProfile)
	}
	if result.Status != "final" {
		t.Errorf("Status = %q, want 'final'", result.Status)
	}
	if result.Subject.Reference != "Patient/12345" {
		t.Errorf("Subject.Reference = %q, want 'Patient/12345'", result.Subject.Reference)
	}
	if len(result.Category) == 0 {
		t.Fatal("Category is empty")
	}
	if result.Code == nil {
		t.Fatal("Code is nil")
	}
	if len(result.Code.Coding) == 0 || result.Code.Coding[0].Code != "36643-5" {
		t.Error("Code.Coding missing LOINC code 36643-5")
	}
	if result.EffectiveDateTime != "2024-01-15T10:30:00Z" {
		t.Errorf("EffectiveDateTime = %q, want '2024-01-15T10:30:00Z'", result.EffectiveDateTime)
	}
	if result.Issued != "2024-01-15T12:00:00Z" {
		t.Errorf("Issued = %q, want '2024-01-15T12:00:00Z'", result.Issued)
	}
	if result.Conclusion != "No acute cardiopulmonary disease" {
		t.Errorf("Conclusion = %q, want 'No acute cardiopulmonary disease'", result.Conclusion)
	}
	if len(result.Performer) == 0 {
		t.Error("Performer is empty")
	}
	if len(result.PresentedForm) == 0 {
		t.Fatal("PresentedForm is empty")
	}
	if result.PresentedForm[0].ContentType != "application/pdf" {
		t.Errorf("PresentedForm.ContentType = %q, want 'application/pdf'", result.PresentedForm[0].ContentType)
	}
}

func TestMapDiagnosticReportNote_StatusMapping(t *testing.T) {
	mapper := NewUSCoreMapper()

	tests := []struct {
		input    string
		expected string
	}{
		{"registered", "registered"},
		{"partial", "partial"},
		{"preliminary", "preliminary"},
		{"p", "preliminary"},
		{"final", "final"},
		{"f", "final"},
		{"amended", "amended"},
		{"a", "amended"},
		{"corrected", "corrected"},
		{"c", "corrected"},
		{"cancelled", "cancelled"},
		{"x", "cancelled"},
		{"unknown", "final"}, // Default
	}

	for _, tt := range tests {
		event := &events.DiagnosticReportNoteEvent{
			DiagnosticReportNote: events.DiagnosticReportNote{
				Status: tt.input,
			},
		}
		result := mapper.MapDiagnosticReportNote(event, "Patient/12345")
		if result.Status != tt.expected {
			t.Errorf("Status for input %q = %q, want %q", tt.input, result.Status, tt.expected)
		}
	}
}

func TestMapDiagnosticReportNote_CategoryMapping(t *testing.T) {
	mapper := NewUSCoreMapper()

	tests := []struct {
		text     string
		code     string
		expected string
	}{
		{"Radiology", "", "LP29684-5"},
		{"Cardiology", "", "LP29708-2"},
		{"Pathology", "", "LP7839-6"},
	}

	for _, tt := range tests {
		event := &events.DiagnosticReportNoteEvent{
			DiagnosticReportNote: events.DiagnosticReportNote{
				Category:     tt.text,
				CategoryCode: tt.code,
			},
		}
		result := mapper.MapDiagnosticReportNote(event, "Patient/12345")
		if len(result.Category) == 0 {
			t.Errorf("Category empty for %q", tt.text)
			continue
		}
		if len(result.Category[0].Coding) == 0 {
			t.Errorf("Category.Coding empty for %q", tt.text)
			continue
		}
		if result.Category[0].Coding[0].Code != tt.expected {
			t.Errorf("Category code for %q = %q, want %q", tt.text, result.Category[0].Coding[0].Code, tt.expected)
		}
	}
}

func TestMapDiagnosticReportNote_WithEncounter(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.DiagnosticReportNoteEvent{
		DiagnosticReportNote: events.DiagnosticReportNote{
			Status: "final",
		},
		Encounter: &events.Encounter{
			ID: "enc-001",
		},
	}

	result := mapper.MapDiagnosticReportNote(event, "Patient/12345")

	if result.Encounter == nil {
		t.Fatal("Encounter is nil")
	}
	if result.Encounter.Reference != "Encounter/enc-001" {
		t.Errorf("Encounter.Reference = %q, want 'Encounter/enc-001'", result.Encounter.Reference)
	}
}

func TestMapDiagnosticReportNote_WithEffectivePeriod(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.DiagnosticReportNoteEvent{
		DiagnosticReportNote: events.DiagnosticReportNote{
			Status:               "final",
			EffectivePeriodStart: "2024-01-15",
			EffectivePeriodEnd:   "2024-01-16",
		},
	}

	result := mapper.MapDiagnosticReportNote(event, "Patient/12345")

	if result.EffectivePeriod == nil {
		t.Fatal("EffectivePeriod is nil")
	}
	if result.EffectivePeriod.Start == nil {
		t.Error("EffectivePeriod.Start is nil")
	}
	if result.EffectivePeriod.End == nil {
		t.Error("EffectivePeriod.End is nil")
	}
}

func TestMapDiagnosticReportNote_WithConclusionCode(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.DiagnosticReportNoteEvent{
		DiagnosticReportNote: events.DiagnosticReportNote{
			Status:         "final",
			ConclusionCode: "260385009",
		},
	}

	result := mapper.MapDiagnosticReportNote(event, "Patient/12345")

	if len(result.ConclusionCode) == 0 {
		t.Fatal("ConclusionCode is empty")
	}
	if len(result.ConclusionCode[0].Coding) == 0 {
		t.Fatal("ConclusionCode.Coding is empty")
	}
	if result.ConclusionCode[0].Coding[0].Code != "260385009" {
		t.Errorf("ConclusionCode = %q, want '260385009'", result.ConclusionCode[0].Coding[0].Code)
	}
	if result.ConclusionCode[0].Coding[0].System != SystemSNOMED {
		t.Errorf("ConclusionCode system = %q, want %q", result.ConclusionCode[0].Coding[0].System, SystemSNOMED)
	}
}

func TestMapDiagnosticReportNote_WithMedia(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.DiagnosticReportNoteEvent{
		DiagnosticReportNote: events.DiagnosticReportNote{
			Status: "final",
			Media: []events.DiagnosticReportMedia{
				{Comment: "Chest X-Ray image", LinkID: "media-001"},
				{Comment: "Lateral view", LinkID: "media-002"},
			},
		},
	}

	result := mapper.MapDiagnosticReportNote(event, "Patient/12345")

	if len(result.Media) != 2 {
		t.Fatalf("Media count = %d, want 2", len(result.Media))
	}
	if result.Media[0].Comment != "Chest X-Ray image" {
		t.Errorf("Media[0].Comment = %q, want 'Chest X-Ray image'", result.Media[0].Comment)
	}
	if result.Media[0].Link.Reference != "Media/media-001" {
		t.Errorf("Media[0].Link = %q, want 'Media/media-001'", result.Media[0].Link.Reference)
	}
}

func TestMapDiagnosticReportNote_WithResultReferences(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.DiagnosticReportNoteEvent{
		DiagnosticReportNote: events.DiagnosticReportNote{
			Status:    "final",
			ResultIDs: []string{"obs-001", "obs-002"},
		},
	}

	result := mapper.MapDiagnosticReportNote(event, "Patient/12345")

	if len(result.Result) != 2 {
		t.Fatalf("Result count = %d, want 2", len(result.Result))
	}
	if result.Result[0].Reference != "Observation/obs-001" {
		t.Errorf("Result[0] = %q, want 'Observation/obs-001'", result.Result[0].Reference)
	}
}

func TestMapDiagnosticReportNote_WithImagingStudyReferences(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.DiagnosticReportNoteEvent{
		DiagnosticReportNote: events.DiagnosticReportNote{
			Status:          "final",
			ImagingStudyIDs: []string{"study-001", "study-002"},
		},
	}

	result := mapper.MapDiagnosticReportNote(event, "Patient/12345")

	if len(result.ImagingStudy) != 2 {
		t.Fatalf("ImagingStudy count = %d, want 2", len(result.ImagingStudy))
	}
	if result.ImagingStudy[0].Reference != "ImagingStudy/study-001" {
		t.Errorf("ImagingStudy[0] = %q, want 'ImagingStudy/study-001'", result.ImagingStudy[0].Reference)
	}
}

func TestMapDiagnosticReportNote_WithPerformerOrg(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.DiagnosticReportNoteEvent{
		DiagnosticReportNote: events.DiagnosticReportNote{
			Status: "final",
		},
		PerformerOrgID:   "org-001",
		PerformerOrgName: "Radiology Department",
	}

	result := mapper.MapDiagnosticReportNote(event, "Patient/12345")

	if len(result.Performer) == 0 {
		t.Fatal("Performer is empty")
	}
	if result.Performer[0].Reference != "Organization/org-001" {
		t.Errorf("Performer[0].Reference = %q, want 'Organization/org-001'", result.Performer[0].Reference)
	}
	if result.Performer[0].Display != "Radiology Department" {
		t.Errorf("Performer[0].Display = %q, want 'Radiology Department'", result.Performer[0].Display)
	}
}

func TestMapDiagnosticReportNote_NilEvent(t *testing.T) {
	mapper := NewUSCoreMapper()
	result := mapper.MapDiagnosticReportNote(nil, "Patient/12345")
	if result != nil {
		t.Error("Expected nil result for nil event")
	}
}

func TestMapDiagnosticReportNote_JSONSerialization(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.DiagnosticReportNoteEvent{
		DiagnosticReportNote: events.DiagnosticReportNote{
			Status:            "final",
			Category:          "Radiology",
			Code:              "Chest X-Ray",
			CodeValue:         "36643-5",
			EffectiveDateTime: "2024-01-15T10:30:00Z",
			Conclusion:        "Normal findings",
			PresentedForm: []events.DiagnosticReportAttachment{
				{
					ContentType: "application/pdf",
					URL:         "https://example.com/report.pdf",
				},
			},
		},
	}

	result := mapper.MapDiagnosticReportNote(event, "Patient/12345")
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	jsonStr := string(data)
	if !strings.Contains(jsonStr, `"resourceType":"DiagnosticReport"`) {
		t.Error("JSON missing resourceType")
	}
	if !strings.Contains(jsonStr, USCoreDiagnosticReportNoteProfile) {
		t.Error("JSON missing profile")
	}
	if !strings.Contains(jsonStr, `"status":"final"`) {
		t.Error("JSON missing status")
	}
	if !strings.Contains(jsonStr, "36643-5") {
		t.Error("JSON missing code")
	}
	if !strings.Contains(jsonStr, "Normal findings") {
		t.Error("JSON missing conclusion")
	}
}

// ============================================================================
// Provenance Tests
// ============================================================================

func TestMapProvenance(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.ProvenanceEvent{
		Provenance: events.Provenance{
			TargetReferences:  []string{"Patient/12345", "Observation/obs-1"},
			TargetDisplays:    []string{"John Doe", "Blood Glucose"},
			Recorded:          "2024-01-15T10:30:00Z",
			OccurredDateTime:  "2024-01-15T10:00:00Z",
			Activity:          "Create",
			ActivityCode:      "CREATE",
			LocationReference: "Location/loc-1",
			LocationDisplay:   "Main Hospital",
			Agents: []events.ProvenanceAgent{
				{
					Type:         "Author",
					TypeCode:     "author",
					WhoReference: "Practitioner/prac-1",
					WhoDisplay:   "Dr. Jane Smith",
				},
			},
			Entities: []events.ProvenanceEntity{
				{
					Role:          "source",
					WhatReference: "DocumentReference/doc-1",
					WhatDisplay:   "Original Document",
				},
			},
		},
	}

	result := mapper.MapProvenance(event)

	// Verify basic fields
	if result == nil {
		t.Fatal("Expected non-nil result")
	}
	if len(result.Meta.Profile) != 1 || result.Meta.Profile[0] != USCoreProvenanceProfile {
		t.Errorf("Profile = %v, want [%s]", result.Meta.Profile, USCoreProvenanceProfile)
	}
	if result.Recorded != "2024-01-15T10:30:00Z" {
		t.Errorf("Recorded = %q, want '2024-01-15T10:30:00Z'", result.Recorded)
	}

	// Verify targets
	if len(result.Target) != 2 {
		t.Errorf("Target count = %d, want 2", len(result.Target))
	} else {
		if result.Target[0].Reference != "Patient/12345" {
			t.Errorf("Target[0].Reference = %q, want 'Patient/12345'", result.Target[0].Reference)
		}
		if result.Target[0].Display != "John Doe" {
			t.Errorf("Target[0].Display = %q, want 'John Doe'", result.Target[0].Display)
		}
	}

	// Verify agents
	if len(result.Agent) != 1 {
		t.Errorf("Agent count = %d, want 1", len(result.Agent))
	} else {
		if result.Agent[0].Who.Reference != "Practitioner/prac-1" {
			t.Errorf("Agent[0].Who.Reference = %q, want 'Practitioner/prac-1'", result.Agent[0].Who.Reference)
		}
	}

	// Verify entities
	if len(result.Entity) != 1 {
		t.Errorf("Entity count = %d, want 1", len(result.Entity))
	} else {
		if result.Entity[0].Role != "source" {
			t.Errorf("Entity[0].Role = %q, want 'source'", result.Entity[0].Role)
		}
	}

	// Verify location
	if result.Location == nil {
		t.Error("Expected non-nil Location")
	} else if result.Location.Reference != "Location/loc-1" {
		t.Errorf("Location.Reference = %q, want 'Location/loc-1'", result.Location.Reference)
	}
}

func TestMapProvenance_NilEvent(t *testing.T) {
	mapper := NewUSCoreMapper()
	result := mapper.MapProvenance(nil)
	if result != nil {
		t.Error("Expected nil result for nil event")
	}
}

func TestMapProvenance_JSONSerialization(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.ProvenanceEvent{
		Provenance: events.Provenance{
			TargetReferences: []string{"Observation/obs-1"},
			Recorded:         "2024-01-15T10:30:00Z",
			Agents: []events.ProvenanceAgent{
				{
					TypeCode:     "author",
					WhoReference: "Practitioner/prac-1",
				},
			},
		},
	}

	result := mapper.MapProvenance(event)
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	jsonStr := string(data)
	if !strings.Contains(jsonStr, `"resourceType":"Provenance"`) {
		t.Error("JSON missing resourceType")
	}
	if !strings.Contains(jsonStr, USCoreProvenanceProfile) {
		t.Error("JSON missing profile")
	}
}

// ============================================================================
// Location Tests
// ============================================================================

func TestMapLocation(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.FacilityLocationEvent{
		FacilityLocation: events.FacilityLocation{
			ID:          "loc-1",
			Status:      "active",
			Name:        "Main Hospital",
			Description: "Primary care facility",
			Mode:        "instance",
			Type:        "Hospital",
			TypeCode:    "HOSP",
			Address: &events.Address{
				Line1:      "123 Health St",
				City:       "Boston",
				State:      "MA",
				PostalCode: "02101",
			},
			PhysicalType:             "Building",
			PhysicalTypeCode:         "bu",
			ManagingOrganizationID:   "org-1",
			ManagingOrganizationName: "Health System Inc",
			Phone:                    "555-123-4567",
			Email:                    "info@hospital.org",
		},
	}

	result := mapper.MapLocation(event)

	// Verify basic fields
	if result == nil {
		t.Fatal("Expected non-nil result")
	}
	if len(result.Meta.Profile) != 1 || result.Meta.Profile[0] != USCoreLocationProfile {
		t.Errorf("Profile = %v, want [%s]", result.Meta.Profile, USCoreLocationProfile)
	}
	if result.Name != "Main Hospital" {
		t.Errorf("Name = %q, want 'Main Hospital'", result.Name)
	}
	if result.Status != "active" {
		t.Errorf("Status = %q, want 'active'", result.Status)
	}
	if result.Description != "Primary care facility" {
		t.Errorf("Description = %q, want 'Primary care facility'", result.Description)
	}

	// Verify address
	if result.Address == nil {
		t.Fatal("Expected non-nil Address")
	}
	if result.Address.City != "Boston" {
		t.Errorf("Address.City = %q, want 'Boston'", result.Address.City)
	}

	// Verify managing organization
	if result.ManagingOrganization == nil {
		t.Error("Expected non-nil ManagingOrganization")
	} else if result.ManagingOrganization.Reference != "Organization/org-1" {
		t.Errorf("ManagingOrganization.Reference = %q, want 'Organization/org-1'", result.ManagingOrganization.Reference)
	}

	// Verify telecom
	if len(result.Telecom) != 2 {
		t.Errorf("Telecom count = %d, want 2", len(result.Telecom))
	}
}

func TestMapLocation_NilEvent(t *testing.T) {
	mapper := NewUSCoreMapper()
	result := mapper.MapLocation(nil)
	if result != nil {
		t.Error("Expected nil result for nil event")
	}
}

func TestMapLocation_JSONSerialization(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.FacilityLocationEvent{
		FacilityLocation: events.FacilityLocation{
			Name:   "Test Clinic",
			Status: "active",
		},
	}

	result := mapper.MapLocation(event)
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	jsonStr := string(data)
	if !strings.Contains(jsonStr, `"resourceType":"Location"`) {
		t.Error("JSON missing resourceType")
	}
	if !strings.Contains(jsonStr, USCoreLocationProfile) {
		t.Error("JSON missing profile")
	}
}

// ============================================================================
// Organization Tests
// ============================================================================

func TestMapOrganization(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.OrganizationEvent{
		Organization: events.Organization{
			ID:       "org-1",
			Active:   true,
			Name:     "General Hospital",
			NPI:      "1234567890",
			TIN:      "12-3456789",
			Type:     "Healthcare Provider",
			TypeCode: "prov",
			Alias:    []string{"GH", "Gen Hospital"},
			Address: &events.Address{
				Line1:      "100 Medical Center Dr",
				City:       "Boston",
				State:      "MA",
				PostalCode: "02101",
			},
			Phone: "555-999-0000",
			Email: "admin@genhospital.org",
		},
	}

	result := mapper.MapOrganization(event)

	// Verify basic fields
	if result == nil {
		t.Fatal("Expected non-nil result")
	}
	if len(result.Meta.Profile) != 1 || result.Meta.Profile[0] != USCoreOrganizationProfile {
		t.Errorf("Profile = %v, want [%s]", result.Meta.Profile, USCoreOrganizationProfile)
	}
	if result.Name != "General Hospital" {
		t.Errorf("Name = %q, want 'General Hospital'", result.Name)
	}

	// Verify NPI identifier
	hasNPI := false
	for _, id := range result.Identifier {
		if id.System == "http://hl7.org/fhir/sid/us-npi" && id.Value == "1234567890" {
			hasNPI = true
			break
		}
	}
	if !hasNPI {
		t.Error("Expected NPI identifier")
	}

	// Verify TIN identifier
	hasTIN := false
	for _, id := range result.Identifier {
		if id.Value == "12-3456789" {
			hasTIN = true
			break
		}
	}
	if !hasTIN {
		t.Error("Expected TIN identifier")
	}

	// Verify aliases
	if len(result.Alias) != 2 {
		t.Errorf("Alias count = %d, want 2", len(result.Alias))
	}

	// Verify address
	if len(result.Address) != 1 {
		t.Fatalf("Address count = %d, want 1", len(result.Address))
	}
	if result.Address[0].City != "Boston" {
		t.Errorf("Address[0].City = %q, want 'Boston'", result.Address[0].City)
	}
}

func TestMapOrganization_NilEvent(t *testing.T) {
	mapper := NewUSCoreMapper()
	result := mapper.MapOrganization(nil)
	if result != nil {
		t.Error("Expected nil result for nil event")
	}
}

func TestMapOrganization_JSONSerialization(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.OrganizationEvent{
		Organization: events.Organization{
			Name: "Test Org",
			NPI:  "1234567890",
		},
	}

	result := mapper.MapOrganization(event)
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	jsonStr := string(data)
	if !strings.Contains(jsonStr, `"resourceType":"Organization"`) {
		t.Error("JSON missing resourceType")
	}
	if !strings.Contains(jsonStr, USCoreOrganizationProfile) {
		t.Error("JSON missing profile")
	}
	if !strings.Contains(jsonStr, "1234567890") {
		t.Error("JSON missing NPI")
	}
}

// ============================================================================
// Practitioner Tests
// ============================================================================

func TestMapPractitioner(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.PractitionerEvent{
		Practitioner: events.Practitioner{
			ID:         "prac-1",
			Active:     true,
			NPI:        "1234567890",
			GivenName:  "Jane",
			MiddleName: "Elizabeth",
			FamilyName: "Smith",
			Prefix:     "Dr.",
			Suffix:     "MD",
			Gender:     "F",
			BirthDate:  "1975-03-20",
			Address: &events.Address{
				Line1:      "500 Doctor Way",
				City:       "Cambridge",
				State:      "MA",
				PostalCode: "02139",
			},
			Phone:     "555-DOC-TORS",
			Email:     "jane.smith@hospital.org",
			Languages: []string{"en", "es"},
			Qualifications: []events.PractitionerQualification{
				{
					Code:       "MD",
					Display:    "Doctor of Medicine",
					IssuerName: "Harvard Medical School",
				},
			},
		},
	}

	result := mapper.MapPractitioner(event)

	// Verify basic fields
	if result == nil {
		t.Fatal("Expected non-nil result")
	}
	if len(result.Meta.Profile) != 1 || result.Meta.Profile[0] != USCorePractitionerProfile {
		t.Errorf("Profile = %v, want [%s]", result.Meta.Profile, USCorePractitionerProfile)
	}

	// Verify NPI
	hasNPI := false
	for _, id := range result.Identifier {
		if id.System == "http://hl7.org/fhir/sid/us-npi" && id.Value == "1234567890" {
			hasNPI = true
			break
		}
	}
	if !hasNPI {
		t.Error("Expected NPI identifier")
	}

	// Verify name
	if len(result.Name) != 1 {
		t.Fatalf("Name count = %d, want 1", len(result.Name))
	}
	if result.Name[0].Family != "Smith" {
		t.Errorf("Family = %q, want 'Smith'", result.Name[0].Family)
	}
	if len(result.Name[0].Given) != 2 {
		t.Errorf("Given count = %d, want 2", len(result.Name[0].Given))
	}

	// Verify gender
	if result.Gender != "female" {
		t.Errorf("Gender = %q, want 'female'", result.Gender)
	}

	// Verify qualifications
	if len(result.Qualification) != 1 {
		t.Errorf("Qualification count = %d, want 1", len(result.Qualification))
	}

	// Verify communication
	if len(result.Communication) != 2 {
		t.Errorf("Communication count = %d, want 2", len(result.Communication))
	}
}

func TestMapPractitioner_NilEvent(t *testing.T) {
	mapper := NewUSCoreMapper()
	result := mapper.MapPractitioner(nil)
	if result != nil {
		t.Error("Expected nil result for nil event")
	}
}

func TestMapPractitioner_JSONSerialization(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.PractitionerEvent{
		Practitioner: events.Practitioner{
			NPI:        "1234567890",
			GivenName:  "John",
			FamilyName: "Doe",
		},
	}

	result := mapper.MapPractitioner(event)
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	jsonStr := string(data)
	if !strings.Contains(jsonStr, `"resourceType":"Practitioner"`) {
		t.Error("JSON missing resourceType")
	}
	if !strings.Contains(jsonStr, USCorePractitionerProfile) {
		t.Error("JSON missing profile")
	}
}

// ============================================================================
// PractitionerRole Tests
// ============================================================================

func TestMapPractitionerRole(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.PractitionerRoleEvent{
		PractitionerRole: events.PractitionerRole{
			ID:               "role-1",
			Active:           true,
			PractitionerID:   "prac-1",
			PractitionerName: "Dr. Jane Smith",
			OrganizationID:   "org-1",
			OrganizationName: "General Hospital",
			Code:             "Physician",
			CodeValue:        "physician",
			Specialty:        "Internal Medicine",
			SpecialtyCode:    "207R00000X",
			LocationIDs:      []string{"loc-1", "loc-2"},
			Phone:            "555-111-2222",
			Email:            "jane.smith@hospital.org",
		},
	}

	result := mapper.MapPractitionerRole(event)

	// Verify basic fields
	if result == nil {
		t.Fatal("Expected non-nil result")
	}
	if len(result.Meta.Profile) != 1 || result.Meta.Profile[0] != USCorePractitionerRoleProfile {
		t.Errorf("Profile = %v, want [%s]", result.Meta.Profile, USCorePractitionerRoleProfile)
	}

	// Verify practitioner reference
	if result.Practitioner == nil {
		t.Error("Expected non-nil Practitioner")
	} else if result.Practitioner.Reference != "Practitioner/prac-1" {
		t.Errorf("Practitioner.Reference = %q, want 'Practitioner/prac-1'", result.Practitioner.Reference)
	}

	// Verify organization reference
	if result.Organization == nil {
		t.Error("Expected non-nil Organization")
	} else if result.Organization.Reference != "Organization/org-1" {
		t.Errorf("Organization.Reference = %q, want 'Organization/org-1'", result.Organization.Reference)
	}

	// Verify specialty
	if len(result.Specialty) != 1 {
		t.Errorf("Specialty count = %d, want 1", len(result.Specialty))
	}

	// Verify locations
	if len(result.Location) != 2 {
		t.Errorf("Location count = %d, want 2", len(result.Location))
	}
}

func TestMapPractitionerRole_NilEvent(t *testing.T) {
	mapper := NewUSCoreMapper()
	result := mapper.MapPractitionerRole(nil)
	if result != nil {
		t.Error("Expected nil result for nil event")
	}
}

func TestMapPractitionerRole_JSONSerialization(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.PractitionerRoleEvent{
		PractitionerRole: events.PractitionerRole{
			PractitionerID: "prac-1",
			OrganizationID: "org-1",
		},
	}

	result := mapper.MapPractitionerRole(event)
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	jsonStr := string(data)
	if !strings.Contains(jsonStr, `"resourceType":"PractitionerRole"`) {
		t.Error("JSON missing resourceType")
	}
	if !strings.Contains(jsonStr, USCorePractitionerRoleProfile) {
		t.Error("JSON missing profile")
	}
}

// ============================================================================
// RelatedPerson Tests
// ============================================================================

func TestMapRelatedPerson(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.RelatedPersonEvent{
		RelatedPerson: events.RelatedPerson{
			ID:               "rp-1",
			Active:           true,
			PatientID:        "patient-1",
			Relationship:     "Mother",
			RelationshipCode: "MTH",
			GivenName:        "Mary",
			MiddleName:       "Jane",
			FamilyName:       "Doe",
			Gender:           "F",
			BirthDate:        "1950-06-15",
			Address: &events.Address{
				Line1:      "123 Family St",
				City:       "Boston",
				State:      "MA",
				PostalCode: "02101",
			},
			Phone:     "555-555-1234",
			Email:     "mary.doe@email.com",
			Languages: []string{"en"},
		},
	}

	result := mapper.MapRelatedPerson(event, "Patient/patient-1")

	// Verify basic fields
	if result == nil {
		t.Fatal("Expected non-nil result")
	}
	if len(result.Meta.Profile) != 1 || result.Meta.Profile[0] != USCoreRelatedPersonProfile {
		t.Errorf("Profile = %v, want [%s]", result.Meta.Profile, USCoreRelatedPersonProfile)
	}

	// Verify patient reference
	if result.Patient == nil {
		t.Error("Expected non-nil Patient")
	} else if result.Patient.Reference != "Patient/patient-1" {
		t.Errorf("Patient.Reference = %q, want 'Patient/patient-1'", result.Patient.Reference)
	}

	// Verify relationship
	if len(result.Relationship) != 1 {
		t.Errorf("Relationship count = %d, want 1", len(result.Relationship))
	} else if len(result.Relationship[0].Coding) != 1 {
		t.Error("Expected relationship coding")
	} else if result.Relationship[0].Coding[0].Code != "MTH" {
		t.Errorf("Relationship code = %q, want 'MTH'", result.Relationship[0].Coding[0].Code)
	}

	// Verify name
	if len(result.Name) != 1 {
		t.Fatalf("Name count = %d, want 1", len(result.Name))
	}
	if result.Name[0].Family != "Doe" {
		t.Errorf("Family = %q, want 'Doe'", result.Name[0].Family)
	}

	// Verify gender
	if result.Gender != "female" {
		t.Errorf("Gender = %q, want 'female'", result.Gender)
	}

	// Verify telecom
	if len(result.Telecom) != 2 {
		t.Errorf("Telecom count = %d, want 2", len(result.Telecom))
	}

	// Verify communication
	if len(result.Communication) != 1 {
		t.Errorf("Communication count = %d, want 1", len(result.Communication))
	}
}

func TestMapRelatedPerson_NilEvent(t *testing.T) {
	mapper := NewUSCoreMapper()
	result := mapper.MapRelatedPerson(nil, "Patient/12345")
	if result != nil {
		t.Error("Expected nil result for nil event")
	}
}

func TestMapRelatedPerson_RelationshipMapping(t *testing.T) {
	mapper := NewUSCoreMapper()

	tests := []struct {
		display      string
		expectedCode string
	}{
		{"mother", "MTH"},
		{"father", "FTH"},
		{"spouse", "SPS"},
		{"child", "CHILD"},
		{"guardian", "GUARD"},
		{"caregiver", "CAREGIVER"},
	}

	for _, tc := range tests {
		t.Run(tc.display, func(t *testing.T) {
			event := &events.RelatedPersonEvent{
				RelatedPerson: events.RelatedPerson{
					Relationship: tc.display,
					FamilyName:   "Test",
				},
			}

			result := mapper.MapRelatedPerson(event, "Patient/12345")
			if len(result.Relationship) != 1 {
				t.Fatalf("Relationship count = %d, want 1", len(result.Relationship))
			}
			if len(result.Relationship[0].Coding) != 1 {
				t.Fatal("Expected relationship coding")
			}
			if result.Relationship[0].Coding[0].Code != tc.expectedCode {
				t.Errorf("Relationship code = %q, want %q", result.Relationship[0].Coding[0].Code, tc.expectedCode)
			}
		})
	}
}

func TestMapRelatedPerson_JSONSerialization(t *testing.T) {
	mapper := NewUSCoreMapper()

	event := &events.RelatedPersonEvent{
		RelatedPerson: events.RelatedPerson{
			Relationship:     "Mother",
			RelationshipCode: "MTH",
			GivenName:        "Mary",
			FamilyName:       "Doe",
		},
	}

	result := mapper.MapRelatedPerson(event, "Patient/12345")
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	jsonStr := string(data)
	if !strings.Contains(jsonStr, `"resourceType":"RelatedPerson"`) {
		t.Error("JSON missing resourceType")
	}
	if !strings.Contains(jsonStr, USCoreRelatedPersonProfile) {
		t.Error("JSON missing profile")
	}
	if !strings.Contains(jsonStr, "MTH") {
		t.Error("JSON missing relationship code")
	}
}
