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
