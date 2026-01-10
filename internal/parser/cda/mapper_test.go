package cda

import (
	"testing"
	"time"

	"github.com/crb2nu/fi-fhir/pkg/events"
)

func TestMapper_Map(t *testing.T) {
	// Parse the sample CCDA
	parser := NewParser("test", nil)
	doc, err := parser.Parse([]byte(sampleCCDA))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// Map to canonical events
	mapper := NewMapper(&MapperConfig{
		Source:             "test_source",
		EmitDocumentEvents: true,
		EmitSectionEvents:  true,
	})

	result, err := mapper.Map(doc)
	if err != nil {
		t.Fatalf("Map failed: %v", err)
	}

	// Should have patient
	if result.Patient == nil {
		t.Fatal("Expected patient")
	}
	if result.Patient.FamilyName != "Smith" {
		t.Errorf("Expected family name Smith, got %s", result.Patient.FamilyName)
	}

	// Should have events from sections
	if len(result.Events) == 0 {
		t.Fatal("Expected events")
	}

	// Log event types for debugging
	eventTypes := make(map[events.EventType]int)
	for _, evt := range result.Events {
		switch e := evt.(type) {
		case *events.DocumentEvent:
			eventTypes[e.Type]++
		case *events.LabResultEvent:
			eventTypes[e.Type]++
		case *events.VitalSignEvent:
			eventTypes[e.Type]++
		case *events.ConditionEvent:
			eventTypes[e.Type]++
		}
	}

	// Should have document event (CCD -> patient_summary)
	if count := eventTypes["patient_summary"]; count == 0 {
		t.Error("Expected patient_summary document event")
	}

	// Should have lab result events
	if count := eventTypes[events.EventLabResult]; count == 0 {
		t.Error("Expected lab_result events")
	}

	// Should have vital sign events
	if count := eventTypes[events.EventVitalSign]; count < 2 {
		t.Errorf("Expected at least 2 vital_sign events, got %d", count)
	}

	// Should have condition events
	if count := eventTypes[events.EventCondition]; count < 2 {
		t.Errorf("Expected at least 2 condition events, got %d", count)
	}
}

func TestMapper_MapPatient(t *testing.T) {
	parser := NewParser("test", nil)
	doc, err := parser.Parse([]byte(sampleCCDA))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	mapper := NewMapper(nil)
	result, err := mapper.Map(doc)
	if err != nil {
		t.Fatalf("Map failed: %v", err)
	}

	patient := result.Patient
	if patient == nil {
		t.Fatal("Expected patient")
	}

	// Verify patient data
	if patient.MRN != "MRN12345" {
		t.Errorf("Expected MRN MRN12345, got %s", patient.MRN)
	}
	if patient.FamilyName != "Smith" {
		t.Errorf("Expected family name Smith, got %s", patient.FamilyName)
	}
	if patient.GivenName != "John" {
		t.Errorf("Expected given name John, got %s", patient.GivenName)
	}
	if patient.MiddleName != "Robert" {
		t.Errorf("Expected middle name Robert, got %s", patient.MiddleName)
	}
	if patient.Gender != "male" {
		t.Errorf("Expected gender male, got %s", patient.Gender)
	}
	if patient.DateOfBirth.Year() != 1985 || patient.DateOfBirth.Month() != 3 {
		t.Errorf("Expected birth date 1985-03-15, got %v", patient.DateOfBirth)
	}
}

func TestMapper_MapLabResults(t *testing.T) {
	parser := NewParser("test", nil)
	doc, err := parser.Parse([]byte(sampleCCDA))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	mapper := NewMapper(nil)
	result, err := mapper.Map(doc)
	if err != nil {
		t.Fatalf("Map failed: %v", err)
	}

	// Find lab result events
	var labEvents []*events.LabResultEvent
	for _, evt := range result.Events {
		if e, ok := evt.(*events.LabResultEvent); ok {
			labEvents = append(labEvents, e)
		}
	}

	if len(labEvents) == 0 {
		t.Fatal("Expected lab result events")
	}

	// Check first lab result (HbA1c)
	lab := labEvents[0]
	if lab.Test.LOINCCode != "4548-4" {
		t.Errorf("Expected LOINC code 4548-4, got %s", lab.Test.LOINCCode)
	}
	if lab.Result.Value != "7.2" {
		t.Errorf("Expected value 7.2, got %s", lab.Result.Value)
	}
	if lab.Result.Unit != "%" {
		t.Errorf("Expected unit %%, got %s", lab.Result.Unit)
	}
	if lab.SourceFormat != events.FormatCDA {
		t.Errorf("Expected source format cda, got %s", lab.SourceFormat)
	}
}

func TestMapper_MapVitalSigns(t *testing.T) {
	parser := NewParser("test", nil)
	doc, err := parser.Parse([]byte(sampleCCDA))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	mapper := NewMapper(nil)
	result, err := mapper.Map(doc)
	if err != nil {
		t.Fatalf("Map failed: %v", err)
	}

	// Find vital sign events
	var vitalEvents []*events.VitalSignEvent
	for _, evt := range result.Events {
		if e, ok := evt.(*events.VitalSignEvent); ok {
			vitalEvents = append(vitalEvents, e)
		}
	}

	if len(vitalEvents) != 2 {
		t.Fatalf("Expected 2 vital sign events, got %d", len(vitalEvents))
	}

	// Check systolic BP
	systolic := vitalEvents[0]
	if systolic.VitalSign.LOINCCode != "8480-6" {
		t.Errorf("Expected LOINC code 8480-6, got %s", systolic.VitalSign.LOINCCode)
	}
	if systolic.VitalSign.Value != "120" {
		t.Errorf("Expected value 120, got %s", systolic.VitalSign.Value)
	}

	// Check diastolic BP
	diastolic := vitalEvents[1]
	if diastolic.VitalSign.LOINCCode != "8462-4" {
		t.Errorf("Expected LOINC code 8462-4, got %s", diastolic.VitalSign.LOINCCode)
	}
	if diastolic.VitalSign.Value != "80" {
		t.Errorf("Expected value 80, got %s", diastolic.VitalSign.Value)
	}
}

func TestMapper_MapConditions(t *testing.T) {
	parser := NewParser("test", nil)
	doc, err := parser.Parse([]byte(sampleCCDA))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	mapper := NewMapper(nil)
	result, err := mapper.Map(doc)
	if err != nil {
		t.Fatalf("Map failed: %v", err)
	}

	// Find condition events
	var conditionEvents []*events.ConditionEvent
	for _, evt := range result.Events {
		if e, ok := evt.(*events.ConditionEvent); ok {
			conditionEvents = append(conditionEvents, e)
		}
	}

	if len(conditionEvents) != 2 {
		t.Fatalf("Expected 2 condition events, got %d", len(conditionEvents))
	}

	// Check first condition (Asthma)
	asthma := conditionEvents[0]
	if asthma.Condition.Code != "195967001" {
		t.Errorf("Expected SNOMED code 195967001, got %s", asthma.Condition.Code)
	}
	if asthma.Condition.Name != "Asthma" {
		t.Errorf("Expected name 'Asthma', got '%s'", asthma.Condition.Name)
	}
	if asthma.ClinicalStatus != "active" {
		t.Errorf("Expected status 'active', got '%s'", asthma.ClinicalStatus)
	}
	if asthma.OnsetDate != "2012-08-06" {
		t.Errorf("Expected onset date 2012-08-06, got %s", asthma.OnsetDate)
	}

	// Check second condition (Diabetes)
	diabetes := conditionEvents[1]
	if diabetes.Condition.Code != "44054006" {
		t.Errorf("Expected SNOMED code 44054006, got %s", diabetes.Condition.Code)
	}
	if diabetes.OnsetDate != "2018-03-20" {
		t.Errorf("Expected onset date 2018-03-20, got %s", diabetes.OnsetDate)
	}
}

func TestMapper_DocumentEvent(t *testing.T) {
	parser := NewParser("test", nil)
	doc, err := parser.Parse([]byte(sampleCCDA))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	mapper := NewMapper(&MapperConfig{
		Source: "test_hospital",
	})
	result, err := mapper.Map(doc)
	if err != nil {
		t.Fatalf("Map failed: %v", err)
	}

	// Find document event
	var docEvent *events.DocumentEvent
	for _, evt := range result.Events {
		if e, ok := evt.(*events.DocumentEvent); ok {
			docEvent = e
			break
		}
	}

	if docEvent == nil {
		t.Fatal("Expected document event")
	}

	// CCD template should map to patient_summary
	if docEvent.Type != "patient_summary" {
		t.Errorf("Expected type patient_summary, got %s", docEvent.Type)
	}
	if docEvent.DocumentType != "CCD" {
		t.Errorf("Expected document type CCD, got %s", docEvent.DocumentType)
	}
	if docEvent.Title != "Continuity of Care Document" {
		t.Errorf("Expected title 'Continuity of Care Document', got '%s'", docEvent.Title)
	}
	if docEvent.Source != "test_hospital" {
		t.Errorf("Expected source test_hospital, got %s", docEvent.Source)
	}
}

func TestMapper_CustomSectionMapper(t *testing.T) {
	parser := NewParser("test", nil)
	doc, err := parser.Parse([]byte(sampleCCDA))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	mapper := NewMapper(nil)

	// Register custom section mapper
	customCalls := 0
	mapper.RegisterSectionMapper(&mockSectionMapper{
		templateOID: TemplateSectionProblems,
		callback: func(section *Section, patient *events.Patient, docTime time.Time) ([]interface{}, error) {
			customCalls++
			return []interface{}{}, nil
		},
	})

	_, err = mapper.Map(doc)
	if err != nil {
		t.Fatalf("Map failed: %v", err)
	}

	if customCalls != 1 {
		t.Errorf("Expected custom mapper to be called once, got %d", customCalls)
	}
}

// mockSectionMapper implements SectionMapper for testing
type mockSectionMapper struct {
	templateOID string
	callback    func(section *Section, patient *events.Patient, docTime time.Time) ([]interface{}, error)
}

func (m *mockSectionMapper) TemplateOID() string {
	return m.templateOID
}

func (m *mockSectionMapper) MapSection(section *Section, patient *events.Patient, docTime time.Time) ([]interface{}, error) {
	return m.callback(section, patient, docTime)
}

func TestOIDToFHIRSystemMapping(t *testing.T) {
	tests := []struct {
		oid          string
		expectedFHIR string
	}{
		{CodeSystemSNOMEDCT, "http://snomed.info/sct"},
		{CodeSystemLOINC, "http://loinc.org"},
		{CodeSystemRxNorm, "http://www.nlm.nih.gov/research/umls/rxnorm"},
		{CodeSystemICD10CM, "http://hl7.org/fhir/sid/icd-10-cm"},
		{CodeSystemCVX, "http://hl7.org/fhir/sid/cvx"},
	}

	for _, tt := range tests {
		fhirSystem, ok := OIDToFHIRSystem[tt.oid]
		if !ok {
			t.Errorf("OID %s not found in OIDToFHIRSystem", tt.oid)
			continue
		}
		if fhirSystem != tt.expectedFHIR {
			t.Errorf("OID %s: expected %s, got %s", tt.oid, tt.expectedFHIR, fhirSystem)
		}
	}
}

func TestProceduresSectionMapper_TemplateOID(t *testing.T) {
	mapper := &ProceduresSectionMapper{}
	if mapper.TemplateOID() != TemplateSectionProcedures {
		t.Errorf("Expected template OID %s, got %s", TemplateSectionProcedures, mapper.TemplateOID())
	}
}

func TestProceduresSectionMapper_MapSection(t *testing.T) {
	mapper := &ProceduresSectionMapper{}
	docTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	patient := &events.Patient{
		MRN:        "MRN12345",
		FamilyName: "Smith",
		GivenName:  "John",
	}

	// Test with procedure entries
	section := &Section{
		TemplateID: TemplateSectionProcedures,
		Title:      "Procedures",
		Entries: []Entry{
			{
				ID:         "PROC-001",
				TypeCode:   "procedure",
				StatusCode: "completed",
				Code: CodedValue{
					Code:        "80146002",
					CodeSystem:  CodeSystemSNOMEDCT,
					DisplayName: "Appendectomy",
				},
				EffectiveTime: &TimeInterval{
					Value: timePtr(time.Date(2023, 6, 15, 0, 0, 0, 0, time.UTC)),
				},
			},
			{
				ID:         "PROC-002",
				TypeCode:   "procedure",
				StatusCode: "active",
				Code: CodedValue{
					Code:        "27768002",
					CodeSystem:  CodeSystemSNOMEDCT,
					DisplayName: "Physical therapy",
				},
				EffectiveTime: &TimeInterval{
					Low: timePtr(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)),
				},
			},
			{
				// Non-procedure entry should be ignored
				ID:       "OTHER-001",
				TypeCode: "observation",
			},
		},
	}

	results, err := mapper.MapSection(section, patient, docTime)
	if err != nil {
		t.Fatalf("MapSection failed: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("Expected 2 procedure events, got %d", len(results))
	}

	// Check first procedure
	proc1 := results[0].(*events.ProcedureEvent)
	if proc1.Procedure.Name != "Appendectomy" {
		t.Errorf("Expected name 'Appendectomy', got '%s'", proc1.Procedure.Name)
	}
	if proc1.Procedure.Code != "80146002" {
		t.Errorf("Expected code '80146002', got '%s'", proc1.Procedure.Code)
	}
	if proc1.Procedure.Status != "completed" {
		t.Errorf("Expected status 'completed', got '%s'", proc1.Procedure.Status)
	}
	if proc1.PerformedDate != "2023-06-15" {
		t.Errorf("Expected performed date '2023-06-15', got '%s'", proc1.PerformedDate)
	}
	if proc1.Procedure.CodeSystem != "http://snomed.info/sct" {
		t.Errorf("Expected FHIR code system, got '%s'", proc1.Procedure.CodeSystem)
	}

	// Check second procedure uses low time
	proc2 := results[1].(*events.ProcedureEvent)
	if proc2.PerformedDate != "2024-01-01" {
		t.Errorf("Expected performed date from low time, got '%s'", proc2.PerformedDate)
	}
	if proc2.Procedure.Status != "active" {
		t.Errorf("Expected status 'active', got '%s'", proc2.Procedure.Status)
	}
}

func TestProceduresSectionMapper_EmptySection(t *testing.T) {
	mapper := &ProceduresSectionMapper{}
	docTime := time.Now()
	patient := &events.Patient{MRN: "MRN123"}

	section := &Section{
		TemplateID: TemplateSectionProcedures,
		Title:      "Procedures",
		Entries:    []Entry{},
	}

	results, err := mapper.MapSection(section, patient, docTime)
	if err != nil {
		t.Fatalf("MapSection failed: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("Expected 0 events for empty section, got %d", len(results))
	}
}

func TestImmunizationsSectionMapper_TemplateOID(t *testing.T) {
	mapper := &ImmunizationsSectionMapper{}
	if mapper.TemplateOID() != TemplateSectionImmunizations {
		t.Errorf("Expected template OID %s, got %s", TemplateSectionImmunizations, mapper.TemplateOID())
	}
}

func TestImmunizationsSectionMapper_MapSection(t *testing.T) {
	mapper := &ImmunizationsSectionMapper{}
	docTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	patient := &events.Patient{
		MRN:        "MRN12345",
		FamilyName: "Smith",
		GivenName:  "John",
	}

	// Test with immunization entries
	section := &Section{
		TemplateID: TemplateSectionImmunizations,
		Title:      "Immunizations",
		Entries: []Entry{
			{
				ID:         "IMM-001",
				TypeCode:   "substanceAdministration",
				StatusCode: "completed",
				Code: CodedValue{
					Code:        "140",
					CodeSystem:  CodeSystemCVX,
					DisplayName: "Influenza, seasonal, injectable",
				},
				EffectiveTime: &TimeInterval{
					Value: timePtr(time.Date(2023, 10, 15, 0, 0, 0, 0, time.UTC)),
				},
			},
			{
				ID:         "IMM-002",
				TypeCode:   "substanceAdministration",
				StatusCode: "completed",
				Code: CodedValue{
					Code:        "208",
					CodeSystem:  CodeSystemCVX,
					DisplayName: "COVID-19 vaccine",
				},
				EffectiveTime: &TimeInterval{
					Low: timePtr(time.Date(2023, 11, 1, 0, 0, 0, 0, time.UTC)),
				},
			},
			{
				// Non-immunization entry should be ignored
				ID:       "OTHER-001",
				TypeCode: "procedure",
			},
		},
	}

	results, err := mapper.MapSection(section, patient, docTime)
	if err != nil {
		t.Fatalf("MapSection failed: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("Expected 2 immunization events, got %d", len(results))
	}

	// Check first immunization
	imm1 := results[0].(*events.ImmunizationEvent)
	if imm1.Immunization.VaccineName != "Influenza, seasonal, injectable" {
		t.Errorf("Expected vaccine name 'Influenza, seasonal, injectable', got '%s'", imm1.Immunization.VaccineName)
	}
	if imm1.Immunization.VaccineCode != "140" {
		t.Errorf("Expected vaccine code '140', got '%s'", imm1.Immunization.VaccineCode)
	}
	if imm1.Immunization.Status != "completed" {
		t.Errorf("Expected status 'completed', got '%s'", imm1.Immunization.Status)
	}
	if imm1.AdministeredDate != "2023-10-15" {
		t.Errorf("Expected administered date '2023-10-15', got '%s'", imm1.AdministeredDate)
	}

	// Check second immunization uses low time
	imm2 := results[1].(*events.ImmunizationEvent)
	if imm2.AdministeredDate != "2023-11-01" {
		t.Errorf("Expected administered date from low time, got '%s'", imm2.AdministeredDate)
	}
}

func TestImmunizationsSectionMapper_EmptySection(t *testing.T) {
	mapper := &ImmunizationsSectionMapper{}
	docTime := time.Now()
	patient := &events.Patient{MRN: "MRN123"}

	section := &Section{
		TemplateID: TemplateSectionImmunizations,
		Title:      "Immunizations",
		Entries:    []Entry{},
	}

	results, err := mapper.MapSection(section, patient, docTime)
	if err != nil {
		t.Fatalf("MapSection failed: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("Expected 0 events for empty section, got %d", len(results))
	}
}

func TestMapStatusCode(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"completed", "completed"},
		{"active", "active"},
		{"cancelled", "cancelled"},
		{"aborted", "aborted"},
		{"suspended", "on-hold"},
		{"held", "on-hold"},
		{"unknown", "unknown"}, // Unknown codes pass through unchanged
		{"", ""},               // Empty string passes through unchanged
		{"new", "new"},         // Unknown code passes through
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := mapStatusCode(tt.input)
			if result != tt.expected {
				t.Errorf("mapStatusCode(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// timePtr is a helper to create time.Time pointers for tests
func timePtr(t time.Time) *time.Time {
	return &t
}
