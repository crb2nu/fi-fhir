package hl7v2

import (
	"fmt"
	"testing"

	"github.com/cblevins/fi-fhir/pkg/events"
	"github.com/cblevins/fi-fhir/pkg/profile"
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

func TestParseUnsupportedMessageType(t *testing.T) {
	parser := NewParser("test_source", ParserConfig{})

	msg := `MSH|^~\&|APP|FAC|APP|FAC|20240115120000||RDE^O11|MSG123|P|2.5
PID|1||123456789^^^HOSPITAL^MRN||DOE^JOHN`

	_, err := parser.Parse(msg)
	if err == nil {
		t.Error("Expected error for unsupported message type")
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
		name                string
		pv1Class            string
		expectedClassified  string
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
