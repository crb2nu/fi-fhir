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
		{"E11.9", SystemICD10CM},     // ICD-10-CM diabetes
		{"J18.9", SystemICD10CM},     // ICD-10-CM pneumonia
		{"A01.0", SystemICD10CM},     // ICD-10-CM typhoid fever
		{"44054006", SystemSNOMED},   // SNOMED diabetes type 2
		{"233604007", SystemSNOMED},  // SNOMED pneumonia
		{"12345678", SystemSNOMED},   // Generic numeric (assume SNOMED)
		{"ABC", SystemSNOMED},        // Short non-numeric (default to SNOMED)
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
				Payer:  events.Provider{OrganizationName: "Test"},
				Payee:  events.Provider{OrganizationName: "Test"},
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
		{"C", 1500.00, 0, "deductible"},     // Deductible amount
		{"B", 30.00, 0, "copay"},            // Copay amount
		{"A", 0, 20.00, "coinsurance"},      // Coinsurance percent
		{"1", 0, 0, "benefit"},              // Active coverage
		{"G", 0, 0, "limit"},                // Quantity limit
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
