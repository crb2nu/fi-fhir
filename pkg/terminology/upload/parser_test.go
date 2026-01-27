package upload

import (
	"strings"
	"testing"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/terminology/db"
)

func TestParser_Parse_StandardFormat(t *testing.T) {
	csv := `source_system,source_code,source_display,target_system,target_code,target_display,equivalence,comment
epic_labs,LAB001,Glucose Fasting,http://loinc.org,1558-6,Fasting glucose,equivalent,Standard mapping
epic_labs,LAB002,Hemoglobin A1c,http://loinc.org,4548-4,Hemoglobin A1c/Hemoglobin.total in Blood,equivalent,
`

	parser := NewParser(ParseOptions{})
	result, err := parser.Parse(strings.NewReader(csv))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.TotalRows != 2 {
		t.Errorf("expected 2 total rows, got %d", result.TotalRows)
	}
	if result.ValidRows != 2 {
		t.Errorf("expected 2 valid rows, got %d", result.ValidRows)
	}
	if result.ErrorRows != 0 {
		t.Errorf("expected 0 error rows, got %d", result.ErrorRows)
	}
	if result.DetectedFormat != "standard" {
		t.Errorf("expected standard format, got %s", result.DetectedFormat)
	}

	if len(result.Rows) != 2 {
		t.Fatalf("expected 2 parsed rows, got %d", len(result.Rows))
	}

	// Check first row
	row := result.Rows[0]
	if row.SourceSystem != "epic_labs" {
		t.Errorf("expected source_system epic_labs, got %s", row.SourceSystem)
	}
	if row.SourceCode != "LAB001" {
		t.Errorf("expected source_code LAB001, got %s", row.SourceCode)
	}
	if row.TargetCode != "1558-6" {
		t.Errorf("expected target_code 1558-6, got %s", row.TargetCode)
	}
	if row.Equivalence != db.EquivalenceEquivalent {
		t.Errorf("expected equivalent equivalence, got %s", row.Equivalence)
	}
}

func TestParser_Parse_SimpleFormat(t *testing.T) {
	csv := `source_code,target_code
LAB001,1558-6
LAB002,4548-4
`

	parser := NewParser(ParseOptions{
		DefaultSourceSystem: "local_labs",
		DefaultTargetSystem: "http://loinc.org",
	})
	result, err := parser.Parse(strings.NewReader(csv))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.TotalRows != 2 {
		t.Errorf("expected 2 total rows, got %d", result.TotalRows)
	}
	if result.ValidRows != 2 {
		t.Errorf("expected 2 valid rows, got %d", result.ValidRows)
	}
	if result.DetectedFormat != "simple" {
		t.Errorf("expected simple format, got %s", result.DetectedFormat)
	}

	// Check defaults applied
	row := result.Rows[0]
	if row.SourceSystem != "local_labs" {
		t.Errorf("expected default source_system local_labs, got %s", row.SourceSystem)
	}
	if row.TargetSystem != "http://loinc.org" {
		t.Errorf("expected default target_system http://loinc.org, got %s", row.TargetSystem)
	}
}

func TestParser_Parse_MissingRequiredColumns(t *testing.T) {
	csv := `source_code,description
LAB001,Some test
`

	parser := NewParser(ParseOptions{})
	_, err := parser.Parse(strings.NewReader(csv))
	if err == nil {
		t.Fatal("expected error for missing required columns")
	}
	if !strings.Contains(err.Error(), "target_code") {
		t.Errorf("expected error to mention target_code, got: %v", err)
	}
}

func TestParser_Parse_ValidationErrors(t *testing.T) {
	csv := `source_system,source_code,target_system,target_code,confidence
epic_labs,LAB001,http://loinc.org,1558-6,0.95
epic_labs,,http://loinc.org,4548-4,0.8
invalid system!,LAB003,http://loinc.org,12345-6,1.5
`

	parser := NewParser(ParseOptions{StrictMode: false})
	result, err := parser.Parse(strings.NewReader(csv))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.TotalRows != 3 {
		t.Errorf("expected 3 total rows, got %d", result.TotalRows)
	}
	// First row valid, second missing code, third invalid system and confidence
	if result.ValidRows != 1 {
		t.Errorf("expected 1 valid row in non-strict mode, got %d", result.ValidRows)
	}
	if result.ErrorRows != 2 {
		t.Errorf("expected 2 error rows, got %d", result.ErrorRows)
	}
	if len(result.Errors) < 2 {
		t.Errorf("expected at least 2 errors, got %d", len(result.Errors))
	}
}

func TestParser_Parse_StrictMode(t *testing.T) {
	csv := `source_system,source_code,target_system,target_code
epic_labs,LAB001,http://loinc.org,1558-6
epic_labs,,http://loinc.org,4548-4
`

	parser := NewParser(ParseOptions{StrictMode: true})
	result, err := parser.Parse(strings.NewReader(csv))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// In strict mode, invalid rows are not added to results
	if result.ValidRows != 1 {
		t.Errorf("expected 1 valid row in strict mode, got %d", result.ValidRows)
	}
	if len(result.Rows) != 1 {
		t.Errorf("expected 1 row in results, got %d", len(result.Rows))
	}
}

func TestParser_Parse_EquivalenceValues(t *testing.T) {
	csv := `source_system,source_code,target_system,target_code,equivalence
sys,C1,http://test,T1,equivalent
sys,C2,http://test,T2,wider
sys,C3,http://test,T3,narrower
sys,C4,http://test,T4,inexact
sys,C5,http://test,T5,exact
sys,C6,http://test,T6,~
sys,C7,http://test,T7,unknown_value
`

	parser := NewParser(ParseOptions{})
	result, err := parser.Parse(strings.NewReader(csv))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := []db.MappingEquivalence{
		db.EquivalenceEquivalent,
		db.EquivalenceWider,
		db.EquivalenceNarrower,
		db.EquivalenceInexact,
		db.EquivalenceEquivalent, // "exact" maps to equivalent
		db.EquivalenceInexact,    // "~" maps to inexact
		db.EquivalenceEquivalent, // unknown defaults to equivalent
	}

	if len(result.Rows) != len(expected) {
		t.Fatalf("expected %d rows, got %d", len(expected), len(result.Rows))
	}

	for i, row := range result.Rows {
		if row.Equivalence != expected[i] {
			t.Errorf("row %d: expected equivalence %s, got %s", i+1, expected[i], row.Equivalence)
		}
	}
}

func TestParser_Parse_AlternateColumnNames(t *testing.T) {
	csv := `from_system,local_code,local_name,to_system,standard_code,standard_name
epic,LAB001,Glucose,http://loinc.org,1558-6,Fasting glucose
`

	parser := NewParser(ParseOptions{})
	result, err := parser.Parse(strings.NewReader(csv))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ValidRows != 1 {
		t.Errorf("expected 1 valid row, got %d", result.ValidRows)
	}

	row := result.Rows[0]
	if row.SourceSystem != "epic" {
		t.Errorf("expected source_system epic, got %s", row.SourceSystem)
	}
	if row.SourceCode != "LAB001" {
		t.Errorf("expected source_code LAB001, got %s", row.SourceCode)
	}
	if row.SourceDisplay != "Glucose" {
		t.Errorf("expected source_display Glucose, got %s", row.SourceDisplay)
	}
}

func TestParser_Parse_MaxRows(t *testing.T) {
	csv := `source_system,source_code,target_system,target_code
sys,C1,http://test,T1
sys,C2,http://test,T2
sys,C3,http://test,T3
sys,C4,http://test,T4
sys,C5,http://test,T5
`

	parser := NewParser(ParseOptions{MaxRows: 3})
	result, err := parser.Parse(strings.NewReader(csv))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.TotalRows != 3 {
		t.Errorf("expected 3 total rows (limited), got %d", result.TotalRows)
	}
}

func TestParser_Parse_EmptyFile(t *testing.T) {
	parser := NewParser(ParseOptions{})
	_, err := parser.Parse(strings.NewReader(""))
	if err == nil {
		t.Fatal("expected error for empty file")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("expected error about empty file, got: %v", err)
	}
}

func TestIsValidCode(t *testing.T) {
	tests := []struct {
		code  string
		valid bool
	}{
		{"1558-6", true},
		{"E11.9", true},
		{"ABC_123", true},
		{"code/path", true},
		{"12345-6", true},
		{"", false},
		{"code with space", false},
		{strings.Repeat("x", 101), false},
	}

	for _, tc := range tests {
		got := isValidCode(tc.code)
		if got != tc.valid {
			t.Errorf("isValidCode(%q) = %v, want %v", tc.code, got, tc.valid)
		}
	}
}

func TestIsValidSystemURI(t *testing.T) {
	tests := []struct {
		uri   string
		valid bool
	}{
		{"http://loinc.org", true},
		{"https://snomed.info/sct", true},
		{"local_system", true},
		{"RXNORM", true},
		{"", false},
		{"not a valid uri!", false},
		{strings.Repeat("x", 256), false},
	}

	for _, tc := range tests {
		got := isValidSystemURI(tc.uri)
		if got != tc.valid {
			t.Errorf("isValidSystemURI(%q) = %v, want %v", tc.uri, got, tc.valid)
		}
	}
}
