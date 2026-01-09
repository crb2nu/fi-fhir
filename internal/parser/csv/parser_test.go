package csv

import (
	"testing"

	"github.com/cblevins/fi-fhir/pkg/events"
)

func TestParsePatientCSV(t *testing.T) {
	parser := NewParser("test_source", ParserConfig{
		HasHeader: true,
		EventType: events.EventPatientUpdate,
	})

	csv := `mrn,first_name,last_name,dob,gender,address,city,state,zip,phone
123456,John,Doe,1980-03-15,M,123 Main St,Anytown,VA,24101,555-123-4567
789012,Jane,Smith,1992-07-22,F,456 Oak Ave,Springfield,IL,62701,555-987-6543`

	result, err := parser.ParseString(csv)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(result.Events) != 2 {
		t.Fatalf("Expected 2 events, got %d", len(result.Events))
	}

	// Check first patient
	event1 := result.Events[0].(*events.PatientAdmitEvent)
	if event1.Patient.MRN != "123456" {
		t.Errorf("Expected MRN '123456', got '%s'", event1.Patient.MRN)
	}
	if event1.Patient.GivenName != "John" {
		t.Errorf("Expected given name 'John', got '%s'", event1.Patient.GivenName)
	}
	if event1.Patient.FamilyName != "Doe" {
		t.Errorf("Expected family name 'Doe', got '%s'", event1.Patient.FamilyName)
	}
	if event1.Patient.Gender != "M" {
		t.Errorf("Expected gender 'M', got '%s'", event1.Patient.Gender)
	}
	if event1.Patient.Address.City != "Anytown" {
		t.Errorf("Expected city 'Anytown', got '%s'", event1.Patient.Address.City)
	}

	// Check second patient
	event2 := result.Events[1].(*events.PatientAdmitEvent)
	if event2.Patient.MRN != "789012" {
		t.Errorf("Expected MRN '789012', got '%s'", event2.Patient.MRN)
	}
}

func TestParseLabResultCSV(t *testing.T) {
	parser := NewParser("lab_system", ParserConfig{
		HasHeader: true,
		EventType: events.EventLabResult,
	})

	csv := `mrn,patient_last_name,patient_first_name,test_code,test_name,result,unit,reference_range,interpretation,collection_date
123456,Doe,John,GLU,Glucose,95,mg/dL,70-100,N,2024-01-15
123456,Doe,John,HGB,Hemoglobin,14.2,g/dL,12.0-17.5,N,2024-01-15
789012,Smith,Jane,GLU,Glucose,142,mg/dL,70-100,H,2024-01-16`

	result, err := parser.ParseString(csv)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(result.Events) != 3 {
		t.Fatalf("Expected 3 events, got %d", len(result.Events))
	}

	// Check first lab result
	event1 := result.Events[0].(*events.LabResultEvent)
	if event1.Patient.MRN != "123456" {
		t.Errorf("Expected MRN '123456', got '%s'", event1.Patient.MRN)
	}
	if event1.Test.LocalCode != "GLU" {
		t.Errorf("Expected test code 'GLU', got '%s'", event1.Test.LocalCode)
	}
	if event1.Result.Value != "95" {
		t.Errorf("Expected result '95', got '%s'", event1.Result.Value)
	}
	if event1.Result.Unit != "mg/dL" {
		t.Errorf("Expected unit 'mg/dL', got '%s'", event1.Result.Unit)
	}
	if event1.Result.Interpretation != "N" {
		t.Errorf("Expected interpretation 'N', got '%s'", event1.Result.Interpretation)
	}

	// Check high glucose
	event3 := result.Events[2].(*events.LabResultEvent)
	if event3.Result.Interpretation != "H" {
		t.Errorf("Expected interpretation 'H', got '%s'", event3.Result.Interpretation)
	}
}

func TestSchemaInference(t *testing.T) {
	parser := NewParser("test", ParserConfig{
		HasHeader:   true,
		InferSchema: true,
	})

	csv := `mrn,first_name,last_name,dob,gender,ssn,phone,email
123456,John,Doe,1980-03-15,M,123-45-6789,555-123-4567,john@example.com
789012,Jane,Smith,1992-07-22,F,987-65-4321,555-987-6543,jane@example.com
345678,Bob,Wilson,1975-11-30,M,456-78-9012,555-456-7890,bob@example.com`

	result, err := parser.ParseString(csv)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if result.Schema == nil {
		t.Fatal("Expected schema to be inferred")
	}

	// Check inferred types
	expectedTypes := map[string]ColumnType{
		"mrn":        TypeMRN,
		"first_name": TypeString,
		"last_name":  TypeString,
		"dob":        TypeDate,
		"gender":     TypeGender,
		"ssn":        TypeSSN,
		"phone":      TypePhone,
		"email":      TypeEmail,
	}

	for _, col := range result.Schema.Columns {
		if expected, ok := expectedTypes[col.Name]; ok {
			if col.InferredType != expected {
				t.Errorf("Column %s: expected type %s, got %s", col.Name, expected, col.InferredType)
			}
		}
	}
}

func TestGenderNormalization(t *testing.T) {
	parser := NewParser("test", ParserConfig{})

	tests := []struct {
		input    string
		expected string
	}{
		{"M", "M"},
		{"F", "F"},
		{"male", "M"},
		{"FEMALE", "F"},
		{"1", "M"},
		{"2", "F"},
		{"Other", "O"},
		{"unknown", "U"},
		{"", "U"},
	}

	for _, tt := range tests {
		result := parser.normalizeGender(tt.input)
		if result != tt.expected {
			t.Errorf("normalizeGender(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestDateParsing(t *testing.T) {
	parser := NewParser("test", ParserConfig{})

	tests := []struct {
		input string
		valid bool
	}{
		{"2024-01-15", true},
		{"01/15/2024", true},
		{"01-15-2024", true},
		{"2024/01/15", true},
		{"20240115", true},
		{"not a date", false},
	}

	for _, tt := range tests {
		_, err := parser.parseDate(tt.input)
		if (err == nil) != tt.valid {
			t.Errorf("parseDate(%q): valid=%v, want valid=%v", tt.input, err == nil, tt.valid)
		}
	}
}

func TestCustomDelimiter(t *testing.T) {
	parser := NewParser("test", ParserConfig{
		HasHeader: true,
		Delimiter: '\t',
		EventType: events.EventPatientUpdate,
	})

	tsv := "mrn\tfirst_name\tlast_name\n123456\tJohn\tDoe"

	result, err := parser.ParseString(tsv)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(result.Events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(result.Events))
	}

	event := result.Events[0].(*events.PatientAdmitEvent)
	if event.Patient.MRN != "123456" {
		t.Errorf("Expected MRN '123456', got '%s'", event.Patient.MRN)
	}
}

func TestColumnMapping(t *testing.T) {
	parser := NewParser("test", ParserConfig{
		HasHeader: true,
		EventType: events.EventPatientUpdate,
		ColumnMapping: map[string]string{
			"pat_id": "mrn",
			"fname":  "first_name",
			"lname":  "last_name",
		},
	})

	csv := `pat_id,fname,lname,dob
123456,John,Doe,1980-03-15`

	result, err := parser.ParseString(csv)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	event := result.Events[0].(*events.PatientAdmitEvent)
	if event.Patient.MRN != "123456" {
		t.Errorf("Expected MRN '123456', got '%s'", event.Patient.MRN)
	}
	if event.Patient.GivenName != "John" {
		t.Errorf("Expected given name 'John', got '%s'", event.Patient.GivenName)
	}
}

func TestEmptyCSV(t *testing.T) {
	parser := NewParser("test", ParserConfig{})

	_, err := parser.ParseString("")
	if err == nil {
		t.Error("Expected error for empty CSV")
	}
}

func TestNoHeader(t *testing.T) {
	parser := NewParser("test", ParserConfig{
		HasHeader: false,
		ColumnMapping: map[string]string{
			"col_0": "mrn",
			"col_1": "first_name",
			"col_2": "last_name",
		},
		EventType: events.EventPatientUpdate,
	})

	csv := `123456,John,Doe
789012,Jane,Smith`

	result, err := parser.ParseString(csv)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(result.Events) != 2 {
		t.Fatalf("Expected 2 events, got %d", len(result.Events))
	}

	event := result.Events[0].(*events.PatientAdmitEvent)
	if event.Patient.MRN != "123456" {
		t.Errorf("Expected MRN '123456', got '%s'", event.Patient.MRN)
	}
}

func TestSemanticHints(t *testing.T) {
	parser := NewParser("test", ParserConfig{})

	tests := []struct {
		name     string
		colType  ColumnType
		expected string
	}{
		{"mrn", TypeMRN, "patient_mrn"},
		{"patient_id", TypeString, "patient_mrn"},
		{"first_name", TypeString, "patient_given_name"},
		{"dob", TypeDate, "patient_dob"},
		{"loinc_code", TypeString, "lab_loinc_code"},
		{"icd10", TypeCode, "diagnosis_icd_code"},
		{"random_column", TypeString, ""},
	}

	for _, tt := range tests {
		hint := parser.inferSemanticHint(tt.name, tt.colType)
		if hint != tt.expected {
			t.Errorf("inferSemanticHint(%q, %s) = %q, want %q", tt.name, tt.colType, hint, tt.expected)
		}
	}
}

func TestInferColumnType(t *testing.T) {
	parser := NewParser("test", ParserConfig{})

	tests := []struct {
		name     string
		samples  []string
		expected ColumnType
	}{
		{"ssn_col", []string{"123-45-6789", "987-65-4321", "456-78-9012"}, TypeSSN},
		{"email_col", []string{"a@b.com", "x@y.org", "test@example.com"}, TypeEmail},
		{"gender_col", []string{"M", "F", "M", "F"}, TypeGender},
		{"date_col", []string{"2024-01-15", "2024-02-20", "2024-03-25"}, TypeDate},
		{"int_col", []string{"123", "456", "789"}, TypeInteger},
		{"float_col", []string{"1.5", "2.7", "3.14"}, TypeFloat},
		{"code_col", []string{"A123", "B456", "C789.1"}, TypeCode},
		{"mixed", []string{"abc", "123", "def"}, TypeString},
	}

	for _, tt := range tests {
		result := parser.inferColumnType(tt.name, tt.samples)
		if result != tt.expected {
			t.Errorf("inferColumnType(%q, %v) = %s, want %s", tt.name, tt.samples, result, tt.expected)
		}
	}
}
