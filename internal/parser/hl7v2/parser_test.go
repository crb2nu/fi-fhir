package hl7v2

import (
	"fmt"
	"testing"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/profile"
)

func TestParseADT_A01(t *testing.T) {
	parser := NewParser("test_source", ParserConfig{})

	msg := `MSH|^~\&|SENDING_APP|SENDING_FAC|RECEIVING_APP|RECEIVING_FAC|20240115120000||ADT^A01|MSG00001|P|2.5
EVN|A01|20240115120000
PID|1||123456789^^^HOSPITAL^MRN||DOE^JOHN^WILLIAM^^^MR||19800315|M|||123 MAIN ST^^ANYTOWN^VA^24101^USA||5551234567
PV1|1|I|ICU^101^A^HOSPITAL||||1234567890^SMITH^JANE^M^^^MD|||MED||||||||VISIT001`

	result, err := parser.Parse(msg)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	event, ok := result.(*events.PatientAdmitEvent)
	if !ok {
		t.Fatalf("Expected PatientAdmitEvent, got %T", result)
	}

	// Check event metadata
	if event.Type != events.EventPatientAdmit {
		t.Errorf("Expected event type %s, got %s", events.EventPatientAdmit, event.Type)
	}
	if event.Source != "test_source" {
		t.Errorf("Expected source 'test_source', got '%s'", event.Source)
	}
	if event.SourceFormat != events.FormatHL7v2 {
		t.Errorf("Expected format HL7v2, got %s", event.SourceFormat)
	}
	if event.SourceMessageID != "MSG00001" {
		t.Errorf("Expected message ID 'MSG00001', got '%s'", event.SourceMessageID)
	}

	// Check patient data
	if event.Patient.MRN != "123456789" {
		t.Errorf("Expected MRN '123456789', got '%s'", event.Patient.MRN)
	}
	if event.Patient.FamilyName != "DOE" {
		t.Errorf("Expected family name 'DOE', got '%s'", event.Patient.FamilyName)
	}
	if event.Patient.GivenName != "JOHN" {
		t.Errorf("Expected given name 'JOHN', got '%s'", event.Patient.GivenName)
	}
	if event.Patient.Gender != "M" {
		t.Errorf("Expected gender 'M', got '%s'", event.Patient.Gender)
	}

	// Check encounter data
	if event.Encounter.Class != "I" {
		t.Errorf("Expected encounter class 'I', got '%s'", event.Encounter.Class)
	}
	if event.Encounter.Location.Unit != "ICU" {
		t.Errorf("Expected location unit 'ICU', got '%s'", event.Encounter.Location.Unit)
	}
}

func TestParseORU_R01(t *testing.T) {
	parser := NewParser("lab_system", ParserConfig{})

	msg := `MSH|^~\&|LAB|LAB_FAC|EHR|HOSPITAL|20240115143000||ORU^R01|LAB001|P|2.5
PID|1||987654321^^^HOSPITAL^MRN||SMITH^JANE|||F
OBX|1|NM|WBC^WHITE BLOOD CELL COUNT^L^6690-2^WBC^LN||12.5|10*3/uL|4.5-11.0|H|||F|||20240115142500`

	result, err := parser.Parse(msg)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	event, ok := result.(*events.LabResultEvent)
	if !ok {
		t.Fatalf("Expected LabResultEvent, got %T", result)
	}

	// Check event type
	if event.Type != events.EventLabResult {
		t.Errorf("Expected event type %s, got %s", events.EventLabResult, event.Type)
	}

	// Check patient
	if event.Patient.MRN != "987654321" {
		t.Errorf("Expected MRN '987654321', got '%s'", event.Patient.MRN)
	}

	// Check test
	if event.Test.LocalCode != "WBC" {
		t.Errorf("Expected test local code 'WBC', got '%s'", event.Test.LocalCode)
	}
	if event.Test.LOINCCode != "6690-2" {
		t.Errorf("Expected LOINC code '6690-2', got '%s'", event.Test.LOINCCode)
	}

	// Check result
	if event.Result.Value != "12.5" {
		t.Errorf("Expected value '12.5', got '%s'", event.Result.Value)
	}
	if event.Result.Interpretation != "H" {
		t.Errorf("Expected interpretation 'H', got '%s'", event.Result.Interpretation)
	}
	if event.Result.ReferenceRange != "4.5-11.0" {
		t.Errorf("Expected reference range '4.5-11.0', got '%s'", event.Result.ReferenceRange)
	}
}

func TestParseADT_A03_Discharge(t *testing.T) {
	parser := NewParser("test_source", ParserConfig{})

	msg := `MSH|^~\&|APP|FAC|APP|FAC|20240116080000||ADT^A03|MSG00002|P|2.5
PID|1||111222333^^^HOSPITAL^MRN||JONES^BOB|||M
PV1|1|I|MED^201^B||||||||||||||||VISIT002||||||||||||||||||20240116080000`

	result, err := parser.Parse(msg)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	event, ok := result.(*events.PatientDischargeEvent)
	if !ok {
		t.Fatalf("Expected PatientDischargeEvent, got %T", result)
	}

	if event.Type != events.EventPatientDischarge {
		t.Errorf("Expected event type %s, got %s", events.EventPatientDischarge, event.Type)
	}
	if event.Patient.FamilyName != "JONES" {
		t.Errorf("Expected family name 'JONES', got '%s'", event.Patient.FamilyName)
	}
}

func TestParseRDE_O11_MedicationRequest(t *testing.T) {
	parser := NewParser("test_source", ParserConfig{})

	msg := `MSH|^~\&|APP|FAC|APP|FAC|20240115120000||RDE^O11|MSG123|P|2.5
PID|1||123456789^^^HOSPITAL^MRN||DOE^JOHN||19800101|M
ORC|NW|ORD123|||||||20240115120000|||1234^Smith^Jane
RXE|^^^|1049502^amoxicillin 500 MG Oral Capsule^RXNORM|1|CAP`

	result, err := parser.ParseWithResult(msg)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	event, ok := result.Event.(*events.MedicationRequestEvent)
	if !ok {
		t.Fatalf("expected MedicationRequestEvent, got %T", result.Event)
	}

	if event.Type != events.EventMedicationRequest {
		t.Fatalf("event type = %s, want %s", event.Type, events.EventMedicationRequest)
	}

	if event.Patient == nil || event.Patient.FamilyName != "DOE" {
		t.Fatalf("unexpected patient: %+v", event.Patient)
	}

	if event.MedicationRequest.Medication.Code != "1049502" {
		t.Fatalf("medication code = %q, want %q", event.MedicationRequest.Medication.Code, "1049502")
	}
	if event.MedicationRequest.Medication.Name != "amoxicillin 500 MG Oral Capsule" {
		t.Fatalf("medication name = %q", event.MedicationRequest.Medication.Name)
	}

	if event.Prescriber == nil || event.Prescriber.FamilyName != "Smith" {
		t.Fatalf("unexpected prescriber: %+v", event.Prescriber)
	}
}

func TestParseVXU_V04_Immunization(t *testing.T) {
	parser := NewParser("test_source", ParserConfig{})

	msg := `MSH|^~\&|IMM|FAC|EHR|FAC|20240115120000||VXU^V04|MSG124|P|2.5
PID|1||123456789^^^HOSPITAL^MRN||DOE^JANE||19900101|F
RXA|0|1|20240115100000|20240115100000|207^COVID-19 mRNA^CVX|0.5|mL||||||||LOT123`

	result, err := parser.ParseWithResult(msg)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	event, ok := result.Event.(*events.ImmunizationEvent)
	if !ok {
		t.Fatalf("expected ImmunizationEvent, got %T", result.Event)
	}
	if event.Type != events.EventImmunization {
		t.Fatalf("event type = %s, want %s", event.Type, events.EventImmunization)
	}
	if event.Patient == nil || event.Patient.GivenName != "JANE" {
		t.Fatalf("unexpected patient: %+v", event.Patient)
	}
	if event.Immunization.VaccineCode != "207" {
		t.Fatalf("vaccine code = %q, want %q", event.Immunization.VaccineCode, "207")
	}
	if event.Immunization.LotNumber != "LOT123" {
		t.Fatalf("lot number = %q, want %q", event.Immunization.LotNumber, "LOT123")
	}
	if event.AdministeredDate == "" {
		t.Fatalf("expected administered date to be set")
	}
}

func TestParseInvalidMessage(t *testing.T) {
	parser := NewParser("test_source", ParserConfig{})

	tests := []struct {
		name string
		msg  string
	}{
		{"empty", ""},
		{"no MSH", "PID|1||123"},
		{"invalid format", "not a valid message"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parser.Parse(tt.msg)
			if err == nil {
				t.Error("Expected error for invalid message")
			}
		})
	}
}

func TestEventClassification(t *testing.T) {
	parser := NewParser("test_source", ParserConfig{})
	// Default profile has event classification rules

	tests := []struct {
		name               string
		pv1Class           string
		expectedClassified string
	}{
		{"inpatient", "I", "inpatient_admit"},
		{"outpatient", "O", "outpatient_registration"},
		{"emergency", "E", "emergency_registration"},
		{"preadmit", "P", "preadmit"},
		{"recurring", "R", "recurring_patient"},
		{"unknown defaults", "X", "patient_admit"}, // Unknown class gets default
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := fmt.Sprintf(`MSH|^~\&|APP|FAC|APP|FAC|20240115120000||ADT^A01|MSG00001|P|2.5
PID|1||123456789^^^HOSPITAL^MRN||DOE^JOHN|||M
PV1|1|%s|ICU^101^A|||||||||||||VISIT001`, tt.pv1Class)

			result, err := parser.ParseWithResult(msg)
			if err != nil {
				t.Fatalf("Parse failed: %v", err)
			}

			event, ok := result.Event.(*events.PatientAdmitEvent)
			if !ok {
				t.Fatalf("Expected PatientAdmitEvent, got %T", result.Event)
			}

			if event.Encounter.ClassifiedEventType != tt.expectedClassified {
				t.Errorf("ClassifiedEventType = %q, want %q",
					event.Encounter.ClassifiedEventType, tt.expectedClassified)
			}
		})
	}
}

func TestMissingPV1Tolerated(t *testing.T) {
	parser := NewParser("test_source", ParserConfig{})
	// Default profile tolerates missing PV1

	// ADT message without PV1 segment
	msg := `MSH|^~\&|APP|FAC|APP|FAC|20240115120000||ADT^A01|MSG00001|P|2.5
PID|1||123456789^^^HOSPITAL^MRN||DOE^JOHN|||M`

	result, err := parser.ParseWithResult(msg)
	if err != nil {
		t.Fatalf("Parse should succeed with missing but tolerated PV1: %v", err)
	}

	// Should have a warning about missing PV1
	foundWarning := false
	for _, w := range result.Warnings {
		if w.Code == "MISSING_PV1" {
			foundWarning = true
			if w.Phase != "semantic" {
				t.Errorf("Warning phase = %q, want 'semantic'", w.Phase)
			}
			break
		}
	}

	if !foundWarning {
		t.Error("Expected MISSING_PV1 warning, but none found")
	}

	// Event should still be returned
	event, ok := result.Event.(*events.PatientAdmitEvent)
	if !ok {
		t.Fatalf("Expected PatientAdmitEvent, got %T", result.Event)
	}

	// Patient data should still be extracted
	if event.Patient.FamilyName != "DOE" {
		t.Errorf("Patient family name = %q, want 'DOE'", event.Patient.FamilyName)
	}

	// Encounter should be empty but not cause an error
	if event.Encounter.Class != "" {
		t.Errorf("Encounter class should be empty, got %q", event.Encounter.Class)
	}
}

func TestParseWithResultIncludesProfileID(t *testing.T) {
	parser := NewParser("test_source", ParserConfig{})

	msg := `MSH|^~\&|APP|FAC|APP|FAC|20240115120000||ADT^A01|MSG00001|P|2.5
PID|1||123456789^^^HOSPITAL^MRN||DOE^JOHN|||M
PV1|1|I|ICU^101^A`

	result, err := parser.ParseWithResult(msg)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// Should include the profile ID from the default profile
	if result.ProfileID != "default" {
		t.Errorf("ProfileID = %q, want 'default'", result.ProfileID)
	}

	// Event should also have the profile ID
	event, ok := result.Event.(*events.PatientAdmitEvent)
	if !ok {
		t.Fatalf("Expected PatientAdmitEvent, got %T", result.Event)
	}

	if event.SourceProfileID != "default" {
		t.Errorf("Event SourceProfileID = %q, want 'default'", event.SourceProfileID)
	}
}

func TestWarningsPropagatedToEvent(t *testing.T) {
	parser := NewParser("test_source", ParserConfig{})

	// Message missing PV1 (tolerated by default profile)
	msg := `MSH|^~\&|APP|FAC|APP|FAC|20240115120000||ADT^A01|MSG00001|P|2.5
PID|1||123456789^^^HOSPITAL^MRN||DOE^JOHN|||M`

	result, err := parser.ParseWithResult(msg)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	event, ok := result.Event.(*events.PatientAdmitEvent)
	if !ok {
		t.Fatalf("Expected PatientAdmitEvent, got %T", result.Event)
	}

	// Warnings should be in both result and event
	if len(result.Warnings) == 0 {
		t.Error("Expected warnings in ParseResult")
	}

	if len(event.ParseWarnings) == 0 {
		t.Error("Expected warnings propagated to event")
	}

	// Same warnings in both places
	if len(result.Warnings) != len(event.ParseWarnings) {
		t.Errorf("Warning count mismatch: result has %d, event has %d",
			len(result.Warnings), len(event.ParseWarnings))
	}
}

func TestIdentifierExtraction(t *testing.T) {
	parser := NewParser("test_source", ParserConfig{})

	// PID-3 with multiple identifiers: MRN, SSN, Medicare
	msg := `MSH|^~\&|APP|FAC|APP|FAC|20240115120000||ADT^A01|MSG00001|P|2.5
PID|1||123456789^^^HOSP_A^MR~078-05-1120^^^SSA^SS~1EG4TE58K72^^^CMS^MB||DOE^JOHN|||M
PV1|1|I|ICU^101^A`

	result, err := parser.ParseWithResult(msg)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	event, ok := result.Event.(*events.PatientAdmitEvent)
	if !ok {
		t.Fatalf("Expected PatientAdmitEvent, got %T", result.Event)
	}

	// Should have 3 identifiers
	if len(event.Patient.Identifiers.Identifiers) != 3 {
		t.Errorf("Expected 3 identifiers, got %d", len(event.Patient.Identifiers.Identifiers))
	}

	// Check MRN
	mrn := event.Patient.Identifiers.GetByType("MR")
	if mrn == nil {
		t.Fatal("Expected MR identifier")
	}
	if mrn.Value != "123456789" {
		t.Errorf("MRN value = %q, want '123456789'", mrn.Value)
	}
	if mrn.Assigner != "HOSP_A" {
		t.Errorf("MRN assigner = %q, want 'HOSP_A'", mrn.Assigner)
	}

	// Check SSN (should be normalized - dashes removed)
	ssn := event.Patient.Identifiers.GetByType("SS")
	if ssn == nil {
		t.Fatal("Expected SS identifier")
	}
	if ssn.Value != "078051120" {
		t.Errorf("SSN value = %q, want '078051120' (normalized)", ssn.Value)
	}
	if ssn.IsValid == nil || !*ssn.IsValid {
		t.Error("SSN should be valid")
	}

	// Check MBI
	mbi := event.Patient.Identifiers.GetByType("MB")
	if mbi == nil {
		t.Fatal("Expected MB identifier")
	}
	if mbi.Value != "1EG4TE58K72" {
		t.Errorf("MBI value = %q, want '1EG4TE58K72'", mbi.Value)
	}

	// Convenience MRN field should match
	if event.Patient.MRN != "123456789" {
		t.Errorf("Patient.MRN = %q, want '123456789'", event.Patient.MRN)
	}
}

func TestInvalidSSNWarning(t *testing.T) {
	parser := NewParser("test_source", ParserConfig{})

	// PID-3 with invalid SSN (area 666 is invalid)
	msg := `MSH|^~\&|APP|FAC|APP|FAC|20240115120000||ADT^A01|MSG00001|P|2.5
PID|1||123456789^^^HOSP_A^MR~666-12-3456^^^SSA^SS||DOE^JOHN|||M
PV1|1|I|ICU^101^A`

	result, err := parser.ParseWithResult(msg)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// Should have a warning about invalid SSN
	foundWarning := false
	for _, w := range result.Warnings {
		if w.Code == "INVALID_SSN_AREA" {
			foundWarning = true
			break
		}
	}
	if !foundWarning {
		t.Error("Expected INVALID_SSN_AREA warning")
	}

	// The SSN identifier should be marked as invalid
	event := result.Event.(*events.PatientAdmitEvent)
	ssn := event.Patient.Identifiers.GetByType("SS")
	if ssn == nil {
		t.Fatal("Expected SS identifier")
	}
	if ssn.IsValid == nil || *ssn.IsValid {
		t.Error("SSN should be marked as invalid")
	}
}

func TestAssigningAuthorityMapping(t *testing.T) {
	parser := NewParser("test_source", ParserConfig{})

	// Set up a custom profile with assigning authority mapping
	customProfile := &profile.SourceProfile{
		ID:   "test_profile",
		Name: "Test Profile",
		Identifiers: &profile.IdentifierConfig{
			AssigningAuthorityMap: map[string]string{
				"HOSP_A": "urn:oid:1.2.3.4.5",
				"SSA":    "urn:oid:2.16.840.1.113883.4.1",
			},
		},
	}
	parser.SetProfile(customProfile)

	msg := `MSH|^~\&|APP|FAC|APP|FAC|20240115120000||ADT^A01|MSG00001|P|2.5
PID|1||123456789^^^HOSP_A^MR~078-05-1120^^^SSA^SS||DOE^JOHN|||M
PV1|1|I|ICU^101^A`

	result, err := parser.ParseWithResult(msg)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	event := result.Event.(*events.PatientAdmitEvent)

	// MRN should have mapped system
	mrn := event.Patient.Identifiers.GetByType("MR")
	if mrn == nil {
		t.Fatal("Expected MR identifier")
	}
	if mrn.System != "urn:oid:1.2.3.4.5" {
		t.Errorf("MRN system = %q, want 'urn:oid:1.2.3.4.5'", mrn.System)
	}

	// SSN should have mapped system
	ssn := event.Patient.Identifiers.GetByType("SS")
	if ssn == nil {
		t.Fatal("Expected SS identifier")
	}
	if ssn.System != "urn:oid:2.16.840.1.113883.4.1" {
		t.Errorf("SSN system = %q, want 'urn:oid:2.16.840.1.113883.4.1'", ssn.System)
	}
}

func TestHL7DateParsing(t *testing.T) {
	parser := NewParser("test", ParserConfig{})

	tests := []struct {
		input    string
		expected string
	}{
		{"19800315", "1980-03-15"},
		{"20240115120000", "2024-01-15"},
		{"202401151430", "2024-01-15"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			date, err := parser.parseHL7Date(tt.input)
			if err != nil {
				t.Fatalf("parseHL7Date failed: %v", err)
			}
			if date.Format("2006-01-02") != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, date.Format("2006-01-02"))
			}
		})
	}
}

func TestUnescapeHL7(t *testing.T) {
	delim := DefaultDelimiters()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"no escapes", "plain text", "plain text"},
		{"field separator", `test\F\value`, "test|value"},
		{"component separator", `test\S\value`, "test^value"},
		{"subcomponent separator", `test\T\value`, "test&value"},
		{"repetition separator", `test\R\value`, "test~value"},
		{"escape character", `test\E\value`, `test\value`},
		{"multiple escapes", `a\F\b\S\c`, "a|b^c"},
		{"hex escape single byte", `test\X0D\value`, "test\rvalue"},
		{"hex escape two bytes", `test\X0D0A\value`, "test\r\nvalue"},
		{"highlighting ignored", `\H\bold text\N\`, "bold text"},
		{"line break", `line1\.br\line2`, "line1\nline2"},
		{"unclosed escape", `test\Fvalue`, `test\Fvalue`},
		{"unknown escape kept", `test\Z\value`, `test\Z\value`},
		{"empty string", "", ""},
		{"escape at end", `test\F\`, "test|"},
		{"consecutive escapes", `\F\\S\`, "|^"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := UnescapeHL7(tt.input, delim)
			if got != tt.want {
				t.Errorf("UnescapeHL7(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestUnescapeHL7WithCustomDelimiters(t *testing.T) {
	// Some systems use non-standard delimiters
	delim := Delimiters{
		Field:        '|',
		Component:    '^',
		Repetition:   '~',
		Escape:       '#', // Non-standard escape
		Subcomponent: '&',
	}

	input := `test#F#value#S#component`
	want := "test|value^component"

	got := UnescapeHL7(input, delim)
	if got != want {
		t.Errorf("UnescapeHL7(%q) = %q, want %q", input, got, want)
	}
}

func TestZSegmentExtraction(t *testing.T) {
	// Create parser with custom profile containing Z-segment mappings
	parser := NewParser("test_source", ParserConfig{})

	customProfile := &profile.SourceProfile{
		ID:   "epic_test",
		Name: "Epic Test Profile",
		ZSegments: &profile.ZSegmentConfig{
			PreserveRaw: true,
			Mappings: map[string][]profile.ZFieldMapping{
				"ZPI": {
					{Field: 1, Target: "external_id", Type: "string"},
					{Field: 2, Target: "vip_status", Type: "boolean"},
					{Field: 3, Target: "risk_score", Type: "integer"},
				},
			},
		},
	}
	parser.SetProfile(customProfile)

	// Message with ZPI segment
	msg := `MSH|^~\&|APP|FAC|APP|FAC|20240115120000||ADT^A01|MSG00001|P|2.5
PID|1||123456789^^^HOSPITAL^MRN||DOE^JOHN|||M
PV1|1|I|ICU^101^A
ZPI|EXT123|Y|85`

	result, err := parser.ParseWithResult(msg)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	event, ok := result.Event.(*events.PatientAdmitEvent)
	if !ok {
		t.Fatalf("Expected PatientAdmitEvent, got %T", result.Event)
	}

	// Check extensions were extracted
	if event.Patient.Extensions == nil {
		t.Fatal("Expected extensions to be populated")
	}

	// Check external_id
	if ext, ok := event.Patient.Extensions["external_id"].(string); !ok || ext != "EXT123" {
		t.Errorf("external_id = %v, want 'EXT123'", event.Patient.Extensions["external_id"])
	}

	// Check vip_status (boolean conversion)
	if ext, ok := event.Patient.Extensions["vip_status"].(bool); !ok || !ext {
		t.Errorf("vip_status = %v, want true", event.Patient.Extensions["vip_status"])
	}

	// Check risk_score (integer conversion)
	if ext, ok := event.Patient.Extensions["risk_score"].(int); !ok || ext != 85 {
		t.Errorf("risk_score = %v, want 85", event.Patient.Extensions["risk_score"])
	}
}

func TestMultipleOBXSegments(t *testing.T) {
	parser := NewParser("lab_system", ParserConfig{})

	// CBC panel with multiple OBX segments
	msg := `MSH|^~\&|LAB|LAB_FAC|EHR|HOSPITAL|20240115143000||ORU^R01|LAB001|P|2.5
PID|1||987654321^^^HOSPITAL^MRN||SMITH^JANE|||F
OBR|1||12345|CBC^Complete Blood Count|||20240115142000
OBX|1|NM|WBC^WHITE BLOOD CELL COUNT^L^6690-2^WBC^LN||12.5|10*3/uL|4.5-11.0|H|||F|||20240115142500
OBX|2|NM|RBC^RED BLOOD CELL COUNT^L^789-8^RBC^LN||4.8|10*6/uL|4.2-5.4||||F|||20240115142500
OBX|3|NM|HGB^HEMOGLOBIN^L^718-7^HGB^LN||14.2|g/dL|12.0-16.0||||F|||20240115142500
OBX|4|NM|HCT^HEMATOCRIT^L^4544-3^HCT^LN||42.5|%|36-46||||F|||20240115142500
OBX|5|NM|PLT^PLATELET COUNT^L^777-3^PLT^LN||250|10*3/uL|150-400||||F|||20240115142500`

	result, err := parser.Parse(msg)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	event, ok := result.(*events.LabResultEvent)
	if !ok {
		t.Fatalf("Expected LabResultEvent, got %T", result)
	}

	// Should have 5 results
	if len(event.Results) != 5 {
		t.Errorf("Expected 5 results, got %d", len(event.Results))
	}

	// Check first result (WBC) - should also be in primary fields
	if event.Test.LocalCode != "WBC" {
		t.Errorf("Primary test local code = %q, want 'WBC'", event.Test.LocalCode)
	}
	if event.Result.Value != "12.5" {
		t.Errorf("Primary result value = %q, want '12.5'", event.Result.Value)
	}
	if event.Result.Interpretation != "H" {
		t.Errorf("Primary result interpretation = %q, want 'H'", event.Result.Interpretation)
	}

	// Check that all results are present
	expectedCodes := []string{"WBC", "RBC", "HGB", "HCT", "PLT"}
	for i, expected := range expectedCodes {
		if i >= len(event.Results) {
			t.Errorf("Missing result at index %d", i)
			continue
		}
		if event.Results[i].Test.LocalCode != expected {
			t.Errorf("Result[%d].Test.LocalCode = %q, want %q",
				i, event.Results[i].Test.LocalCode, expected)
		}
	}

	// Check LOINC codes were extracted
	loincCodes := map[string]string{
		"WBC": "6690-2",
		"RBC": "789-8",
		"HGB": "718-7",
		"HCT": "4544-3",
		"PLT": "777-3",
	}

	for _, obs := range event.Results {
		expectedLOINC, ok := loincCodes[obs.Test.LocalCode]
		if !ok {
			continue
		}
		if obs.Test.LOINCCode != expectedLOINC {
			t.Errorf("Result %s LOINC = %q, want %q",
				obs.Test.LocalCode, obs.Test.LOINCCode, expectedLOINC)
		}
	}
}

func TestZSegmentPreserveRaw(t *testing.T) {
	parser := NewParser("test_source", ParserConfig{})

	// Profile with PreserveRaw but no specific mappings for ZXX
	customProfile := &profile.SourceProfile{
		ID:   "raw_test",
		Name: "Raw Test Profile",
		ZSegments: &profile.ZSegmentConfig{
			PreserveRaw: true,
			Mappings:    make(map[string][]profile.ZFieldMapping),
		},
	}
	parser.SetProfile(customProfile)

	// Message with unknown Z-segment
	msg := `MSH|^~\&|APP|FAC|APP|FAC|20240115120000||ADT^A01|MSG00001|P|2.5
PID|1||123456789^^^HOSPITAL^MRN||DOE^JOHN|||M
PV1|1|I|ICU^101^A
ZXX|field1|field2|field3`

	result, err := parser.ParseWithResult(msg)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	event := result.Event.(*events.PatientAdmitEvent)

	// Check raw_ZXX was preserved
	if event.Patient.Extensions == nil {
		t.Fatal("Expected extensions to be populated")
	}

	rawZXX, ok := event.Patient.Extensions["raw_ZXX"].(string)
	if !ok {
		t.Fatalf("raw_ZXX not found or wrong type: %v", event.Patient.Extensions["raw_ZXX"])
	}

	expected := "field1|field2|field3"
	if rawZXX != expected {
		t.Errorf("raw_ZXX = %q, want %q", rawZXX, expected)
	}
}

func TestEscapeSequenceIntegration(t *testing.T) {
	parser := NewParser("test_source", ParserConfig{})

	// Message with escape sequences in name and address fields
	// \T\ is ampersand, \S\ is caret, \F\ is pipe
	msg := `MSH|^~\&|APP|FAC|APP|FAC|20240115120000||ADT^A01|MSG00001|P|2.5
PID|1||123456^^^HOSPITAL^MRN||O'BRIEN^MARY\T\ANN|||F|||123 MAIN ST \T\ APT 5^SUITE B^ANYTOWN^VA^24101
PV1|1|I|ICU^101^A`

	result, err := parser.ParseWithResult(msg)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	event := result.Event.(*events.PatientAdmitEvent)

	// Check that given name has escape sequence decoded (& becomes actual &)
	if event.Patient.GivenName != "MARY&ANN" {
		t.Errorf("GivenName = %q, want 'MARY&ANN'", event.Patient.GivenName)
	}

	// Check that address has escape sequence decoded
	if event.Patient.Address.Line1 != "123 MAIN ST & APT 5" {
		t.Errorf("Address.Line1 = %q, want '123 MAIN ST & APT 5'", event.Patient.Address.Line1)
	}
}

func TestEscapeSequenceInLabResult(t *testing.T) {
	parser := NewParser("test_source", ParserConfig{})

	// ORU message with escape sequences in OBX fields
	msg := `MSH|^~\&|LAB|HOSP|APP|FAC|20240115143000||ORU^R01|MSG00002|P|2.5
PID|1||123456^^^HOSPITAL^MRN||DOE^JOHN|||M
OBX|1|ST|TEST01^Test \T\ Result^L||Value with \T\ ampersand||mg/dL|N|||F`

	result, err := parser.ParseWithResult(msg)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	event := result.Event.(*events.LabResultEvent)

	// Check that test description has escape sequence decoded
	if event.Test.Description != "Test & Result" {
		t.Errorf("Test.Description = %q, want 'Test & Result'", event.Test.Description)
	}

	// Check that value has escape sequence decoded
	if event.Result.Value != "Value with & ampersand" {
		t.Errorf("Result.Value = %q, want 'Value with & ampersand'", event.Result.Value)
	}
}

func TestDelimitersParsedFromMSH(t *testing.T) {
	parser := NewParser("test_source", ParserConfig{})

	// Standard message
	msg := `MSH|^~\&|APP|FAC|APP|FAC|20240115120000||ADT^A01|MSG00001|P|2.5
PID|1||123456^^^HOSPITAL^MRN||DOE^JOHN|||M
PV1|1|I|ICU^101^A`

	result, err := parser.ParseWithResult(msg)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// The event should have been parsed correctly with standard delimiters
	event := result.Event.(*events.PatientAdmitEvent)
	if event.Patient.FamilyName != "DOE" {
		t.Errorf("FamilyName = %q, want 'DOE'", event.Patient.FamilyName)
	}
}

func TestOBRExtraction(t *testing.T) {
	parser := NewParser("test_source", ParserConfig{})

	// ORU message with OBR segment containing order information
	msg := `MSH|^~\&|LAB|HOSP|APP|FAC|20240115143000||ORU^R01|MSG00002|P|2.5
PID|1||123456^^^HOSPITAL^MRN||DOE^JOHN|||M
OBR|1|ORD001^EPIC|FIL001^LAB|CBC^Complete Blood Count^L|||20240115140000|||||||||1234567890^SMITH^JANE^M^MD^^NPI
OBX|1|NM|WBC^White Blood Count^L||12.5|10*3/uL|4.5-11.0|H|||F
OBX|2|NM|RBC^Red Blood Count^L||5.2|10*6/uL|4.5-5.9||||F`

	result, err := parser.ParseWithResult(msg)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	event := result.Event.(*events.LabResultEvent)

	// Check ordering provider
	if event.OrderingProvider == nil {
		t.Fatal("OrderingProvider is nil")
	}
	if event.OrderingProvider.ID != "1234567890" {
		t.Errorf("OrderingProvider.ID = %q, want '1234567890'", event.OrderingProvider.ID)
	}
	if event.OrderingProvider.FamilyName != "SMITH" {
		t.Errorf("OrderingProvider.FamilyName = %q, want 'SMITH'", event.OrderingProvider.FamilyName)
	}
	if event.OrderingProvider.GivenName != "JANE" {
		t.Errorf("OrderingProvider.GivenName = %q, want 'JANE'", event.OrderingProvider.GivenName)
	}

	// Check order ID on test
	if event.Test.OrderID != "FIL001" {
		t.Errorf("Test.OrderID = %q, want 'FIL001'", event.Test.OrderID)
	}

	// Check panel name
	if event.Test.Panel != "Complete Blood Count" {
		t.Errorf("Test.Panel = %q, want 'Complete Blood Count'", event.Test.Panel)
	}

	// Check all observations have order ID
	for i, obs := range event.Results {
		if obs.Test.OrderID != "FIL001" {
			t.Errorf("Results[%d].Test.OrderID = %q, want 'FIL001'", i, obs.Test.OrderID)
		}
	}
}

func TestOBRExtractionNPIDetection(t *testing.T) {
	parser := NewParser("test_source", ParserConfig{})

	// OBR with NPI in assigning authority (XCN.9 = index 8)
	// Format: ID^family^given^middle^suffix^prefix^degree^sourceTable^assigningAuth
	// 6 carets after given name to reach index 8
	msg := `MSH|^~\&|LAB|HOSP|APP|FAC|20240115143000||ORU^R01|MSG00003|P|2.5
PID|1||123456^^^HOSPITAL^MRN||DOE^JOHN|||M
OBR|1|ORD001|FIL001|GLU^Glucose|||20240115140000|||||||||9876543210^JONES^BOB^^^^^^NPI
OBX|1|NM|GLU^Glucose||120|mg/dL|70-100|H|||F`

	result, err := parser.ParseWithResult(msg)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	event := result.Event.(*events.LabResultEvent)

	// Check NPI was detected
	if event.OrderingProvider == nil {
		t.Fatal("OrderingProvider is nil")
	}
	if event.OrderingProvider.NPI != "9876543210" {
		t.Errorf("OrderingProvider.NPI = %q, want '9876543210'", event.OrderingProvider.NPI)
	}
}

func TestOBRMissing(t *testing.T) {
	parser := NewParser("test_source", ParserConfig{})

	// ORU message without OBR segment (should still work)
	msg := `MSH|^~\&|LAB|HOSP|APP|FAC|20240115143000||ORU^R01|MSG00004|P|2.5
PID|1||123456^^^HOSPITAL^MRN||DOE^JOHN|||M
OBX|1|NM|GLU^Glucose||120|mg/dL|70-100|H|||F`

	result, err := parser.ParseWithResult(msg)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	event := result.Event.(*events.LabResultEvent)

	// Without OBR, these should be empty/nil
	if event.OrderingProvider != nil {
		t.Error("OrderingProvider should be nil without OBR")
	}
	if event.Test.OrderID != "" {
		t.Errorf("Test.OrderID = %q, want empty", event.Test.OrderID)
	}
}

// =============================================================================
// SIU (Scheduling) Message Tests
// =============================================================================

// TestSIUMessageTypes verifies all SIU message types produce correct event types.
// This is the primary test for SIU message routing.
func TestSIUMessageTypes(t *testing.T) {
	tests := []struct {
		name         string
		msgType      string
		expectedType events.EventType
	}{
		{"S12 New Booking", "SIU^S12", events.EventAppointmentScheduled},
		{"S13 Reschedule", "SIU^S13", events.EventAppointmentRescheduled},
		{"S14 Modification", "SIU^S14", events.EventAppointmentModified},
		{"S15 Cancellation", "SIU^S15", events.EventAppointmentCancelled},
		{"S26 No-Show", "SIU^S26", events.EventAppointmentNoShow},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewParser("test", ParserConfig{})

			// Minimal valid SIU message - just needs SCH and PID
			msg := fmt.Sprintf(`MSH|^~\&|SCHED|HOSP|EHR|HOSP|20240115100000||%s|MSG001|P|2.5
SCH|APPT001
PID|1||123456^^^HOSPITAL^MRN||DOE^JOHN|||M`, tt.msgType)

			result, err := parser.Parse(msg)
			if err != nil {
				t.Fatalf("Parse failed for %s: %v", tt.msgType, err)
			}

			event, ok := result.(*events.AppointmentEvent)
			if !ok {
				t.Fatalf("Expected AppointmentEvent for %s, got %T", tt.msgType, result)
			}

			if event.Type != tt.expectedType {
				t.Errorf("%s: expected event type %s, got %s", tt.msgType, tt.expectedType, event.Type)
			}

			// Verify appointment ID is extracted
			if event.Appointment.ID != "APPT001" {
				t.Errorf("%s: expected appointment ID 'APPT001', got '%s'", tt.msgType, event.Appointment.ID)
			}

			// Verify patient is extracted
			if event.Patient.MRN != "123456" {
				t.Errorf("%s: expected patient MRN '123456', got '%s'", tt.msgType, event.Patient.MRN)
			}
		})
	}
}

// TestSIU_S15_CancellationDefaults verifies S15 sets cancelled status.
func TestSIU_S15_CancellationDefaults(t *testing.T) {
	parser := NewParser("test", ParserConfig{})

	// S15 with no status field - should default to "cancelled"
	msg := `MSH|^~\&|SCHED|HOSP|EHR|HOSP|20240115100000||SIU^S15|MSG001|P|2.5
SCH|APPT001
PID|1||123456^^^HOSPITAL^MRN||DOE^JOHN|||M`

	result, err := parser.Parse(msg)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	event := result.(*events.AppointmentEvent)

	if event.Appointment.Status != "cancelled" {
		t.Errorf("S15 should set status to 'cancelled', got '%s'", event.Appointment.Status)
	}
}

// TestSIU_S26_NoShowDefaults verifies S26 sets noshow status and flag.
func TestSIU_S26_NoShowDefaults(t *testing.T) {
	parser := NewParser("test", ParserConfig{})

	// S26 with no status field - should default to "noshow" and set NoShow flag
	msg := `MSH|^~\&|SCHED|HOSP|EHR|HOSP|20240115100000||SIU^S26|MSG001|P|2.5
SCH|APPT001
PID|1||123456^^^HOSPITAL^MRN||DOE^JOHN|||M`

	result, err := parser.Parse(msg)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	event := result.(*events.AppointmentEvent)

	if event.Appointment.Status != "noshow" {
		t.Errorf("S26 should set status to 'noshow', got '%s'", event.Appointment.Status)
	}
	if !event.Appointment.NoShow {
		t.Error("S26 should set NoShow flag to true")
	}
}

// TestSIUWithAISSegment verifies location is extracted from AIS segment.
func TestSIUWithAISSegment(t *testing.T) {
	parser := NewParser("test", ParserConfig{})

	msg := `MSH|^~\&|SCHED|HOSP|EHR|HOSP|20240115100000||SIU^S12|MSG001|P|2.5
SCH|APPT001
PID|1||123456^^^HOSPITAL^MRN||DOE^JOHN|||M
AIS|1|A|CARDIO^Cardiology Department`

	result, err := parser.Parse(msg)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	event := result.(*events.AppointmentEvent)

	if event.Appointment.Location.Description != "Cardiology Department" {
		t.Errorf("Expected location 'Cardiology Department', got '%s'", event.Appointment.Location.Description)
	}
}

// ========================================
// MDM (Medical Document Management) Tests
// ========================================

func TestParseMDM_T02_OriginalDocument(t *testing.T) {
	parser := NewParser("dictation_system", ParserConfig{})

	msg := `MSH|^~\&|DICTATION|HOSPITAL|EMR|HOSPITAL|20240115120000||MDM^T02^MDM_T02|MSG00001|P|2.5|||AL|NE
EVN|T02|20240115120000
PID|1||123456789^^^HOSPITAL^MRN||DOE^JOHN^WILLIAM^^^MR||19800315|M|||123 MAIN ST^^ANYTOWN^VA^24101^USA
PV1|1|I|ICU^101^A^HOSPITAL||||1234567890^SMITH^JANE^M^^^MD
TXA|1|HP^History and Physical^HL70270||20240115100000|1234567890^SMITH^JANE^M^^^MD|20240115110000|20240115115000|||||DOC001||||History and Physical - John Doe|AU|||20240115120000|1234567890^SMITH^JANE^M^^^MD
OBX|1|TX|HP^History and Physical||HISTORY AND PHYSICAL~~Date: January 15, 2024~~CHIEF COMPLAINT:~Chest pain and shortness of breath~~HISTORY OF PRESENT ILLNESS:~Patient is a 43-year-old male presenting with acute onset chest pain.||||||F`

	result, err := parser.Parse(msg)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	event, ok := result.(*events.DocumentEvent)
	if !ok {
		t.Fatalf("Expected DocumentEvent, got %T", result)
	}

	// Check event metadata
	if event.Type != events.EventDocumentOriginal {
		t.Errorf("Expected event type %s, got %s", events.EventDocumentOriginal, event.Type)
	}
	if event.Source != "dictation_system" {
		t.Errorf("Expected source 'dictation_system', got '%s'", event.Source)
	}
	if event.SourceFormat != events.FormatHL7v2 {
		t.Errorf("Expected format HL7v2, got %s", event.SourceFormat)
	}
	if event.SourceMessageID != "MSG00001" {
		t.Errorf("Expected message ID 'MSG00001', got '%s'", event.SourceMessageID)
	}

	// Check patient data
	if event.Patient == nil {
		t.Fatal("Expected patient to be set")
	}
	if event.Patient.MRN != "123456789" {
		t.Errorf("Expected MRN '123456789', got '%s'", event.Patient.MRN)
	}
	if event.Patient.FamilyName != "DOE" {
		t.Errorf("Expected family name 'DOE', got '%s'", event.Patient.FamilyName)
	}

	// Check TXA fields
	if event.DocumentType != "HP" {
		t.Errorf("Expected document type 'HP', got '%s'", event.DocumentType)
	}
	if event.Title != "History and Physical - John Doe" {
		t.Errorf("Expected title 'History and Physical - John Doe', got '%s'", event.Title)
	}
	if event.UniqueDocumentNumber != "DOC001" {
		t.Errorf("Expected unique document number 'DOC001', got '%s'", event.UniqueDocumentNumber)
	}
	if event.DocumentStatus != "authenticated" {
		t.Errorf("Expected document status 'authenticated', got '%s'", event.DocumentStatus)
	}

	// Check author
	if event.Author == nil {
		t.Fatal("Expected author to be set")
	}
	if event.Author.ID != "1234567890" {
		t.Errorf("Expected author ID '1234567890', got '%s'", event.Author.ID)
	}
	if event.Author.FamilyName != "SMITH" {
		t.Errorf("Expected author family name 'SMITH', got '%s'", event.Author.FamilyName)
	}

	// Check content
	if event.ContentType != "TX" {
		t.Errorf("Expected content type 'TX', got '%s'", event.ContentType)
	}
	if event.Content == "" {
		t.Error("Expected content to be set")
	}
	if event.ContentEncoding != "text" {
		t.Errorf("Expected content encoding 'text', got '%s'", event.ContentEncoding)
	}
}

func TestParseMDM_T03_StatusChange(t *testing.T) {
	parser := NewParser("dictation_system", ParserConfig{})

	msg := `MSH|^~\&|DICTATION|HOSPITAL|EMR|HOSPITAL|20240115140000||MDM^T03^MDM_T03|MSG00002|P|2.5|||AL|NE
EVN|T03|20240115140000
PID|1||123456789^^^HOSPITAL^MRN||DOE^JOHN^WILLIAM^^^MR||19800315|M|||123 MAIN ST^^ANYTOWN^VA^24101^USA
TXA|1|HP^History and Physical^HL70270||20240115100000|1234567890^SMITH^JANE^M^^^MD|20240115110000|20240115130000|||||DOC001||||History and Physical - John Doe|LA|||20240115140000|1234567890^SMITH^JANE^M^^^MD`

	result, err := parser.Parse(msg)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	event, ok := result.(*events.DocumentEvent)
	if !ok {
		t.Fatalf("Expected DocumentEvent, got %T", result)
	}

	// Check event type
	if event.Type != events.EventDocumentStatusChange {
		t.Errorf("Expected event type %s, got %s", events.EventDocumentStatusChange, event.Type)
	}

	// Check TXA fields
	if event.UniqueDocumentNumber != "DOC001" {
		t.Errorf("Expected unique document number 'DOC001', got '%s'", event.UniqueDocumentNumber)
	}
	if event.DocumentStatus != "legally_authenticated" {
		t.Errorf("Expected document status 'legally_authenticated', got '%s'", event.DocumentStatus)
	}
}

func TestParseMDM_T06_Addendum(t *testing.T) {
	parser := NewParser("dictation_system", ParserConfig{})

	msg := `MSH|^~\&|DICTATION|HOSPITAL|EMR|HOSPITAL|20240116080000||MDM^T06^MDM_T06|MSG00003|P|2.5|||AL|NE
EVN|T06|20240116080000
PID|1||123456789^^^HOSPITAL^MRN||DOE^JOHN^WILLIAM^^^MR||19800315|M
PV1|1|I|ICU^101^A^HOSPITAL||||1234567890^SMITH^JANE^M^^^MD
TXA|1|AD^Addendum^HL70270||20240116080000|1234567890^SMITH^JANE^M^^^MD|20240116080000||||||ADD001|DOC001||Addendum to H&P - Cardiology Results|AU|||20240116080000
OBX|1|TX|AD^Addendum||ADDENDUM TO HISTORY AND PHYSICAL~~Date: January 16, 2024~~CARDIOLOGY CONSULTATION RESULTS:~- Troponin I: 0.02 ng/mL (normal)~- EKG: Normal sinus rhythm||||||F`

	result, err := parser.Parse(msg)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	event, ok := result.(*events.DocumentEvent)
	if !ok {
		t.Fatalf("Expected DocumentEvent, got %T", result)
	}

	// Check event type
	if event.Type != events.EventDocumentAddendum {
		t.Errorf("Expected event type %s, got %s", events.EventDocumentAddendum, event.Type)
	}

	// Check TXA fields
	if event.DocumentType != "AD" {
		t.Errorf("Expected document type 'AD', got '%s'", event.DocumentType)
	}
	if event.UniqueDocumentNumber != "ADD001" {
		t.Errorf("Expected unique document number 'ADD001', got '%s'", event.UniqueDocumentNumber)
	}
	if event.ParentDocumentNumber != "DOC001" {
		t.Errorf("Expected parent document number 'DOC001', got '%s'", event.ParentDocumentNumber)
	}

	// Check content is present
	if event.Content == "" {
		t.Error("Expected content to be set for addendum")
	}
}

func TestParseMDM_T10_Replacement(t *testing.T) {
	parser := NewParser("dictation_system", ParserConfig{})

	msg := `MSH|^~\&|DICTATION|HOSPITAL|EMR|HOSPITAL|20240117090000||MDM^T10^MDM_T10|MSG00004|P|2.5|||AL|NE
EVN|T10|20240117090000
PID|1||123456789^^^HOSPITAL^MRN||DOE^JOHN|||M
TXA|1|HP^History and Physical^HL70270||20240117090000|1234567890^SMITH^JANE^M^^^MD|||||||DOC002|DOC001|||Corrected History and Physical|AU
OBX|1|TX|HP^History and Physical||CORRECTED HISTORY AND PHYSICAL~~This document replaces the previous version.||||||F`

	result, err := parser.Parse(msg)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	event, ok := result.(*events.DocumentEvent)
	if !ok {
		t.Fatalf("Expected DocumentEvent, got %T", result)
	}

	// Check event type
	if event.Type != events.EventDocumentReplacement {
		t.Errorf("Expected event type %s, got %s", events.EventDocumentReplacement, event.Type)
	}

	// Check parent reference
	if event.ParentDocumentNumber != "DOC001" {
		t.Errorf("Expected parent document number 'DOC001', got '%s'", event.ParentDocumentNumber)
	}
	if event.UniqueDocumentNumber != "DOC002" {
		t.Errorf("Expected unique document number 'DOC002', got '%s'", event.UniqueDocumentNumber)
	}
}

// ========================================
// DFT (Detail Financial Transaction) Tests
// ========================================

func TestParseDFT_P03_SingleTransaction(t *testing.T) {
	parser := NewParser("billing_system", ParserConfig{})

	msg := `MSH|^~\&|BILLING|HOSPITAL|EMR|HOSPITAL|20240115150000||DFT^P03^DFT_P03|MSG00004|P|2.5|||AL|NE
EVN|P03|20240115150000
PID|1||123456789^^^HOSPITAL^MRN~999-88-7777^^^SSA^SS||DOE^JOHN^WILLIAM^^^MR||19800315|M|||123 MAIN ST^^ANYTOWN^VA^24101^USA||5551234567|||||ACCT001
PV1|1|I|ICU^101^A^HOSPITAL||||1234567890^SMITH^JANE^M^^^MD|||MED||||||||V001
FT1|1|TXN001||20240115|20240115|CG|99223^Initial Hospital Care High^CPT|Initial hospital care, high complexity|||350.00|350.00||||ICU^101^A^HOSPITAL|||J18.9^Pneumonia, unspecified organism^I10||1234567890^SMITH^JANE^M^^^MD||1234567890^SMITH^JANE^M^^^MD||ORD001||99223^Initial Hospital Care^CPT
DG1|1|I10|J18.9^Pneumonia, unspecified organism^ICD10|Pneumonia|20240115|F|||||||||||1234567890^SMITH^JANE^M^^^MD
IN1|1|BCBS001^Blue Cross Blue Shield|123456|Blue Cross Blue Shield of Virginia||||GRP12345|ACME Corporation|||20240101|20241231||||||||||||||||||||||SUB001`

	result, err := parser.Parse(msg)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	event, ok := result.(*events.FinancialTransactionEvent)
	if !ok {
		t.Fatalf("Expected FinancialTransactionEvent, got %T", result)
	}

	// Check event metadata
	if event.Type != events.EventFinancialTransaction {
		t.Errorf("Expected event type %s, got %s", events.EventFinancialTransaction, event.Type)
	}
	if event.Source != "billing_system" {
		t.Errorf("Expected source 'billing_system', got '%s'", event.Source)
	}
	if event.SourceFormat != events.FormatHL7v2 {
		t.Errorf("Expected format HL7v2, got %s", event.SourceFormat)
	}

	// Check patient data
	if event.Patient.MRN != "123456789" {
		t.Errorf("Expected MRN '123456789', got '%s'", event.Patient.MRN)
	}
	if event.Patient.FamilyName != "DOE" {
		t.Errorf("Expected family name 'DOE', got '%s'", event.Patient.FamilyName)
	}

	// Check account number
	if event.AccountNumber != "ACCT001" {
		t.Errorf("Expected account number 'ACCT001', got '%s'", event.AccountNumber)
	}

	// Check transactions
	if len(event.Transactions) != 1 {
		t.Fatalf("Expected 1 transaction, got %d", len(event.Transactions))
	}

	txn := event.Transactions[0]
	if txn.TransactionID != "TXN001" {
		t.Errorf("Expected transaction ID 'TXN001', got '%s'", txn.TransactionID)
	}
	if txn.TransactionType != "CG" {
		t.Errorf("Expected transaction type 'CG', got '%s'", txn.TransactionType)
	}
	if len(txn.TransactionCode.Coding) == 0 || txn.TransactionCode.Coding[0].Code != "99223" {
		t.Errorf("Expected transaction code '99223', got '%v'", txn.TransactionCode.Coding)
	}
	if txn.Amount != 350.00 {
		t.Errorf("Expected amount 350.00, got %f", txn.Amount)
	}

	// Check diagnoses
	if len(event.Transactions[0].Diagnoses) != 1 {
		t.Fatalf("Expected 1 diagnosis, got %d", len(event.Transactions[0].Diagnoses))
	}

	diag := event.Transactions[0].Diagnoses[0]
	if len(diag.Code.Coding) == 0 || diag.Code.Coding[0].Code != "J18.9" {
		t.Errorf("Expected diagnosis code 'J18.9', got '%v'", diag.Code.Coding)
	}

	// Check insurance
	if len(event.InsuranceInfo) != 1 {
		t.Fatalf("Expected 1 insurance record, got %d", len(event.InsuranceInfo))
	}
	if event.InsuranceInfo[0].PlanID != "BCBS001" {
		t.Errorf("Expected plan ID 'BCBS001', got '%s'", event.InsuranceInfo[0].PlanID)
	}

	// Check total charge amount
	if event.TotalChargeAmount != 350.00 {
		t.Errorf("Expected total charge 350.00, got %f", event.TotalChargeAmount)
	}
}

func TestParseDFT_P03_MultipleFT1(t *testing.T) {
	parser := NewParser("billing_system", ParserConfig{})

	msg := `MSH|^~\&|BILLING|HOSPITAL|EMR|HOSPITAL|20240115160000||DFT^P03^DFT_P03|MSG00005|P|2.5|||AL|NE
EVN|P03|20240115160000
PID|1||987654321^^^HOSPITAL^MRN||SMITH^JANE^A^^^MS||19750420|F|||456 OAK AVE^^SPRINGFIELD^IL^62701^USA||5559876543|||||||ACCT002
PV1|1|O|CLINIC^200^A^HOSPITAL||||2345678901^JONES^ROBERT^K^^^MD|||FAM||||||||V002
FT1|1|TXN002||20240115|20240115|CG|99214^Office Visit Level 4^CPT|Established patient office visit|||150.00|150.00||||CLINIC^200^A|||E11.9^Type 2 diabetes mellitus without complications^I10||2345678901^JONES^ROBERT^K^^^MD||2345678901^JONES^ROBERT^K^^^MD||||99214^Office Visit^CPT|25
FT1|2|TXN003||20240115|20240115|CG|36415^Venipuncture^CPT|Blood draw|||25.00|25.00||||CLINIC^200^A|||||||3456789012^TECH^LAB^A^^^RN||||36415^Venipuncture^CPT
FT1|3|TXN004||20240115|20240115|CG|85025^CBC with Differential^CPT|Complete blood count|||35.00|35.00||||LAB^100^A|||||||||||85025^CBC^CPT
DG1|1|I10|E11.9^Type 2 diabetes mellitus without complications^ICD10|Type 2 Diabetes|20240115|W|||||||||1|2345678901^JONES^ROBERT^K^^^MD
DG1|2|I10|I10^Essential (primary) hypertension^ICD10|Hypertension|20240115|W|||||||||2|2345678901^JONES^ROBERT^K^^^MD
PR1|1|C4|36415^Venipuncture^CPT|Blood Draw|20240115||5||3456789012^TECH^LAB^A^^^RN
IN1|1|AETNA001^Aetna|789012|Aetna Health Insurance||||GRP67890|Beta Corporation|||20230601|20251231||||||||||||||||||||||SUB002
IN1|2|MEDICAID^State Medicaid|456789|State Medicaid Program||||||||||||||||||||||||||||||SUB003`

	result, err := parser.Parse(msg)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	event, ok := result.(*events.FinancialTransactionEvent)
	if !ok {
		t.Fatalf("Expected FinancialTransactionEvent, got %T", result)
	}

	// Check patient
	if event.Patient.MRN != "987654321" {
		t.Errorf("Expected MRN '987654321', got '%s'", event.Patient.MRN)
	}
	if event.Patient.FamilyName != "SMITH" {
		t.Errorf("Expected family name 'SMITH', got '%s'", event.Patient.FamilyName)
	}

	// Check multiple transactions
	if len(event.Transactions) != 3 {
		t.Fatalf("Expected 3 transactions, got %d", len(event.Transactions))
	}

	// Verify transaction IDs
	expectedTxnIDs := []string{"TXN002", "TXN003", "TXN004"}
	for i, txn := range event.Transactions {
		if txn.TransactionID != expectedTxnIDs[i] {
			t.Errorf("Transaction %d: expected ID '%s', got '%s'", i, expectedTxnIDs[i], txn.TransactionID)
		}
	}

	// Verify amounts
	expectedAmounts := []float64{150.00, 25.00, 35.00}
	for i, txn := range event.Transactions {
		if txn.Amount != expectedAmounts[i] {
			t.Errorf("Transaction %d: expected amount %f, got %f", i, expectedAmounts[i], txn.Amount)
		}
	}

	// Check total charge amount
	expectedTotal := 210.00 // 150 + 25 + 35
	if event.TotalChargeAmount != expectedTotal {
		t.Errorf("Expected total charge %f, got %f", expectedTotal, event.TotalChargeAmount)
	}

	// Check diagnoses (shared across transactions)
	if len(event.Transactions[0].Diagnoses) != 2 {
		t.Errorf("Expected 2 diagnoses on first transaction, got %d", len(event.Transactions[0].Diagnoses))
	}

	// Check procedures
	if len(event.Transactions[0].Procedures) != 1 {
		t.Errorf("Expected 1 procedure, got %d", len(event.Transactions[0].Procedures))
	}
	if len(event.Transactions[0].Procedures[0].Code.Coding) == 0 || event.Transactions[0].Procedures[0].Code.Coding[0].Code != "36415" {
		t.Errorf("Expected procedure code '36415', got '%v'", event.Transactions[0].Procedures[0].Code.Coding)
	}

	// Check multiple insurance records
	if len(event.InsuranceInfo) != 2 {
		t.Fatalf("Expected 2 insurance records, got %d", len(event.InsuranceInfo))
	}
	if event.InsuranceInfo[0].PlanID != "AETNA001" {
		t.Errorf("Expected plan ID 'AETNA001', got '%s'", event.InsuranceInfo[0].PlanID)
	}
	if event.InsuranceInfo[1].PlanID != "MEDICAID" {
		t.Errorf("Expected plan ID 'MEDICAID', got '%s'", event.InsuranceInfo[1].PlanID)
	}
}

func TestParseDFT_P03_WithProcedureModifiers(t *testing.T) {
	parser := NewParser("billing_system", ParserConfig{})

	msg := `MSH|^~\&|BILLING|HOSPITAL|EMR|HOSPITAL|20240115170000||DFT^P03^DFT_P03|MSG00006|P|2.5|||AL|NE
EVN|P03|20240115170000
PID|1||111222333^^^HOSPITAL^MRN||TEST^PATIENT|||M
FT1|1|TXN005||20240115|20240115|CG|99214^Office Visit^CPT||||100.00|100.00|||||||||||||99214^Office Visit^CPT|25~59`

	result, err := parser.Parse(msg)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	event, ok := result.(*events.FinancialTransactionEvent)
	if !ok {
		t.Fatalf("Expected FinancialTransactionEvent, got %T", result)
	}

	if len(event.Transactions) != 1 {
		t.Fatalf("Expected 1 transaction, got %d", len(event.Transactions))
	}

	// Check procedure modifiers
	txn := event.Transactions[0]
	if len(txn.ProcedureModifiers) != 2 {
		t.Fatalf("Expected 2 procedure modifiers, got %d", len(txn.ProcedureModifiers))
	}
	if txn.ProcedureModifiers[0] != "25" {
		t.Errorf("Expected modifier '25', got '%s'", txn.ProcedureModifiers[0])
	}
	if txn.ProcedureModifiers[1] != "59" {
		t.Errorf("Expected modifier '59', got '%s'", txn.ProcedureModifiers[1])
	}
}

func TestParseDFT_P11(t *testing.T) {
	parser := NewParser("billing_system", ParserConfig{})

	// DFT^P11 is similar to P03 but for billing inquiries
	msg := `MSH|^~\&|BILLING|HOSPITAL|EMR|HOSPITAL|20240115180000||DFT^P11^DFT_P11|MSG00007|P|2.5|||AL|NE
EVN|P11|20240115180000
PID|1||444555666^^^HOSPITAL^MRN||INQUIRY^TEST|||F
FT1|1|TXN006||20240115|20240115|CG|99213^Office Visit Level 3^CPT||||75.00|75.00`

	result, err := parser.Parse(msg)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	event, ok := result.(*events.FinancialTransactionEvent)
	if !ok {
		t.Fatalf("Expected FinancialTransactionEvent, got %T", result)
	}

	// DFT^P11 should use same event type
	if event.Type != events.EventFinancialTransaction {
		t.Errorf("Expected event type %s, got %s", events.EventFinancialTransaction, event.Type)
	}

	if len(event.Transactions) != 1 {
		t.Fatalf("Expected 1 transaction, got %d", len(event.Transactions))
	}
}
