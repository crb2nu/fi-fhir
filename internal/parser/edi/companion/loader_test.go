package companion

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadGuideFromYAML(t *testing.T) {
	yamlContent := `
id: test_guide
name: Test Guide
payer_id: TEST
base_guide: "005010X222A1"
transaction_types:
  - "837P"
overrides:
  - path: "NM1.09"
    requirement: required
validations:
  - id: NPI_CHECK
    path: "NM1.09"
    type: luhn
    message: Invalid NPI
code_restrictions:
  - path: "SBR.09"
    values: ["MA", "MB"]
`

	guide, err := LoadGuideFromYAML(strings.NewReader(yamlContent))
	if err != nil {
		t.Fatalf("LoadGuideFromYAML failed: %v", err)
	}

	if guide.ID != "test_guide" {
		t.Errorf("ID = %q, want test_guide", guide.ID)
	}
	if guide.Name != "Test Guide" {
		t.Errorf("Name = %q, want Test Guide", guide.Name)
	}
	if guide.PayerID != "TEST" {
		t.Errorf("PayerID = %q, want TEST", guide.PayerID)
	}
	if len(guide.TransactionTypes) != 1 || guide.TransactionTypes[0] != "837P" {
		t.Errorf("TransactionTypes = %v, want [837P]", guide.TransactionTypes)
	}
	if len(guide.Overrides) != 1 {
		t.Errorf("Overrides count = %d, want 1", len(guide.Overrides))
	}
	if len(guide.Validations) != 1 {
		t.Errorf("Validations count = %d, want 1", len(guide.Validations))
	}
	if len(guide.CodeRestrictions) != 1 {
		t.Errorf("CodeRestrictions count = %d, want 1", len(guide.CodeRestrictions))
	}
}

func TestLoadGuideFromJSON(t *testing.T) {
	jsonContent := `{
		"id": "test_guide",
		"name": "Test Guide",
		"payer_id": "TEST",
		"base_guide": "005010X222A1",
		"transaction_types": ["837P"],
		"validations": [
			{
				"id": "NPI_CHECK",
				"path": "NM1.09",
				"type": "luhn",
				"message": "Invalid NPI"
			}
		]
	}`

	guide, err := LoadGuideFromJSON(strings.NewReader(jsonContent))
	if err != nil {
		t.Fatalf("LoadGuideFromJSON failed: %v", err)
	}

	if guide.ID != "test_guide" {
		t.Errorf("ID = %q, want test_guide", guide.ID)
	}
	if guide.Name != "Test Guide" {
		t.Errorf("Name = %q, want Test Guide", guide.Name)
	}
}

func TestLoadGuide_YAML(t *testing.T) {
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "test.yaml")

	content := `
id: yaml_test
name: YAML Test
transaction_types: ["837P"]
`
	if err := os.WriteFile(yamlPath, []byte(content), 0600); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	guide, err := LoadGuide(yamlPath)
	if err != nil {
		t.Fatalf("LoadGuide failed: %v", err)
	}

	if guide.ID != "yaml_test" {
		t.Errorf("ID = %q, want yaml_test", guide.ID)
	}
}

func TestLoadGuide_YML(t *testing.T) {
	dir := t.TempDir()
	ymlPath := filepath.Join(dir, "test.yml")

	content := `
id: yml_test
name: YML Test
transaction_types: ["837P"]
`
	if err := os.WriteFile(ymlPath, []byte(content), 0600); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	guide, err := LoadGuide(ymlPath)
	if err != nil {
		t.Fatalf("LoadGuide failed: %v", err)
	}

	if guide.ID != "yml_test" {
		t.Errorf("ID = %q, want yml_test", guide.ID)
	}
}

func TestLoadGuide_JSON(t *testing.T) {
	dir := t.TempDir()
	jsonPath := filepath.Join(dir, "test.json")

	content := `{"id": "json_test", "name": "JSON Test", "transaction_types": ["837P"]}`
	if err := os.WriteFile(jsonPath, []byte(content), 0600); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	guide, err := LoadGuide(jsonPath)
	if err != nil {
		t.Fatalf("LoadGuide failed: %v", err)
	}

	if guide.ID != "json_test" {
		t.Errorf("ID = %q, want json_test", guide.ID)
	}
}

func TestLoadGuide_UnsupportedExtension(t *testing.T) {
	dir := t.TempDir()
	txtPath := filepath.Join(dir, "test.txt")

	if err := os.WriteFile(txtPath, []byte("content"), 0600); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	_, err := LoadGuide(txtPath)
	if err == nil {
		t.Error("LoadGuide should fail for unsupported extension")
	}
	if !strings.Contains(err.Error(), "unsupported file extension") {
		t.Errorf("Error = %q, should contain 'unsupported file extension'", err)
	}
}

func TestLoadGuide_FileNotFound(t *testing.T) {
	_, err := LoadGuide("/nonexistent/path/guide.yaml")
	if err == nil {
		t.Error("LoadGuide should fail for non-existent file")
	}
}

func TestLoadGuidesFromDirectory(t *testing.T) {
	dir := t.TempDir()

	// Create valid YAML file
	yamlPath := filepath.Join(dir, "guide1.yaml")
	yamlContent := `id: guide1
name: Guide 1
transaction_types: ["837P"]`
	if err := os.WriteFile(yamlPath, []byte(yamlContent), 0600); err != nil {
		t.Fatalf("Failed to write YAML file: %v", err)
	}

	// Create valid JSON file
	jsonPath := filepath.Join(dir, "guide2.json")
	jsonContent := `{"id": "guide2", "name": "Guide 2", "transaction_types": ["835"]}`
	if err := os.WriteFile(jsonPath, []byte(jsonContent), 0600); err != nil {
		t.Fatalf("Failed to write JSON file: %v", err)
	}

	// Create non-guide file (should be ignored)
	txtPath := filepath.Join(dir, "readme.txt")
	if err := os.WriteFile(txtPath, []byte("readme"), 0600); err != nil {
		t.Fatalf("Failed to write txt file: %v", err)
	}

	guides, err := LoadGuidesFromDirectory(dir)
	if err != nil {
		t.Fatalf("LoadGuidesFromDirectory failed: %v", err)
	}

	if len(guides) != 2 {
		t.Fatalf("Loaded %d guides, want 2", len(guides))
	}

	ids := make(map[string]bool)
	for _, g := range guides {
		ids[g.ID] = true
	}
	if !ids["guide1"] || !ids["guide2"] {
		t.Errorf("Missing expected guide IDs: %v", ids)
	}
}

func TestLoadGuidesFromDirectory_WithInvalid(t *testing.T) {
	dir := t.TempDir()

	// Create valid file
	validPath := filepath.Join(dir, "valid.yaml")
	validContent := `id: valid
name: Valid
transaction_types: ["837P"]`
	if err := os.WriteFile(validPath, []byte(validContent), 0600); err != nil {
		t.Fatalf("Failed to write valid file: %v", err)
	}

	// Create invalid file
	invalidPath := filepath.Join(dir, "invalid.yaml")
	invalidContent := `id: invalid
name: ""` // Missing required fields
	if err := os.WriteFile(invalidPath, []byte(invalidContent), 0600); err != nil {
		t.Fatalf("Failed to write invalid file: %v", err)
	}

	guides, err := LoadGuidesFromDirectory(dir)
	// Should still load valid guides but return error about invalid ones
	if err == nil {
		t.Error("Expected error about invalid guide")
	}
	if len(guides) != 1 {
		t.Errorf("Loaded %d guides, want 1 valid", len(guides))
	}
}

func TestLoadGuidesFromDirectory_NotFound(t *testing.T) {
	_, err := LoadGuidesFromDirectory("/nonexistent/directory")
	if err == nil {
		t.Error("LoadGuidesFromDirectory should fail for non-existent directory")
	}
}

func TestSaveGuideToYAML(t *testing.T) {
	guide := &CompanionGuide{
		ID:               "save_test",
		Name:             "Save Test",
		PayerID:          "TEST",
		TransactionTypes: []string{"837P"},
	}

	var buf strings.Builder
	if err := SaveGuideToYAML(guide, &buf); err != nil {
		t.Fatalf("SaveGuideToYAML failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "id: save_test") {
		t.Errorf("YAML output missing id: %s", output)
	}
	if !strings.Contains(output, "name: Save Test") {
		t.Errorf("YAML output missing name: %s", output)
	}
}

func TestSaveGuideToJSON(t *testing.T) {
	guide := &CompanionGuide{
		ID:               "save_test",
		Name:             "Save Test",
		TransactionTypes: []string{"837P"},
	}

	var buf strings.Builder
	if err := SaveGuideToJSON(guide, &buf, true); err != nil {
		t.Fatalf("SaveGuideToJSON failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, `"id": "save_test"`) {
		t.Errorf("JSON output missing id: %s", output)
	}
}

func TestSaveGuideToJSON_Compact(t *testing.T) {
	guide := &CompanionGuide{
		ID:               "save_test",
		Name:             "Save Test",
		TransactionTypes: []string{"837P"},
	}

	var buf strings.Builder
	if err := SaveGuideToJSON(guide, &buf, false); err != nil {
		t.Fatalf("SaveGuideToJSON failed: %v", err)
	}

	output := buf.String()
	// Compact JSON should not have indentation
	if strings.Contains(output, "  ") {
		t.Errorf("Compact JSON should not have indentation: %s", output)
	}
}

func TestSaveGuide_YAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "output.yaml")

	guide := &CompanionGuide{
		ID:               "file_save",
		Name:             "File Save Test",
		TransactionTypes: []string{"837P"},
	}

	if err := SaveGuide(guide, path); err != nil {
		t.Fatalf("SaveGuide failed: %v", err)
	}

	// Verify by loading
	loaded, err := LoadGuide(path)
	if err != nil {
		t.Fatalf("Failed to load saved guide: %v", err)
	}
	if loaded.ID != "file_save" {
		t.Errorf("Loaded ID = %q, want file_save", loaded.ID)
	}
}

func TestSaveGuide_JSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "output.json")

	guide := &CompanionGuide{
		ID:               "json_save",
		Name:             "JSON Save Test",
		TransactionTypes: []string{"837P"},
	}

	if err := SaveGuide(guide, path); err != nil {
		t.Fatalf("SaveGuide failed: %v", err)
	}

	// Verify by loading
	loaded, err := LoadGuide(path)
	if err != nil {
		t.Fatalf("Failed to load saved guide: %v", err)
	}
	if loaded.ID != "json_save" {
		t.Errorf("Loaded ID = %q, want json_save", loaded.ID)
	}
}

func TestSaveGuide_UnsupportedExtension(t *testing.T) {
	guide := &CompanionGuide{
		ID:               "test",
		Name:             "Test",
		TransactionTypes: []string{"837P"},
	}

	err := SaveGuide(guide, "/tmp/test.txt")
	if err == nil {
		t.Error("SaveGuide should fail for unsupported extension")
	}
}

func TestValidateGuide(t *testing.T) {
	tests := []struct {
		name    string
		guide   *CompanionGuide
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid guide",
			guide: &CompanionGuide{
				ID:               "valid",
				Name:             "Valid Guide",
				TransactionTypes: []string{"837P"},
			},
			wantErr: false,
		},
		{
			name: "missing ID",
			guide: &CompanionGuide{
				Name:             "Missing ID",
				TransactionTypes: []string{"837P"},
			},
			wantErr: true,
			errMsg:  "ID is required",
		},
		{
			name: "missing name",
			guide: &CompanionGuide{
				ID:               "missing_name",
				TransactionTypes: []string{"837P"},
			},
			wantErr: true,
			errMsg:  "name is required",
		},
		{
			name: "missing transaction types",
			guide: &CompanionGuide{
				ID:   "missing_tx",
				Name: "Missing Transaction Types",
			},
			wantErr: true,
			errMsg:  "transaction type",
		},
		{
			name: "validation rule missing ID",
			guide: &CompanionGuide{
				ID:               "rule_no_id",
				Name:             "Rule No ID",
				TransactionTypes: []string{"837P"},
				Validations: []ValidationRule{
					{Path: "CLM.01", Type: ValidationPattern},
				},
			},
			wantErr: true,
			errMsg:  "ID is required",
		},
		{
			name: "validation rule missing path",
			guide: &CompanionGuide{
				ID:               "rule_no_path",
				Name:             "Rule No Path",
				TransactionTypes: []string{"837P"},
				Validations: []ValidationRule{
					{ID: "TEST", Type: ValidationPattern},
				},
			},
			wantErr: true,
			errMsg:  "path is required",
		},
		{
			name: "validation rule missing type",
			guide: &CompanionGuide{
				ID:               "rule_no_type",
				Name:             "Rule No Type",
				TransactionTypes: []string{"837P"},
				Validations: []ValidationRule{
					{ID: "TEST", Path: "CLM.01"},
				},
			},
			wantErr: true,
			errMsg:  "type is required",
		},
		{
			name: "validation rule invalid path",
			guide: &CompanionGuide{
				ID:               "rule_bad_path",
				Name:             "Rule Bad Path",
				TransactionTypes: []string{"837P"},
				Validations: []ValidationRule{
					{ID: "TEST", Path: "INVALID", Type: ValidationPattern},
				},
			},
			wantErr: true,
			errMsg:  "invalid path",
		},
		{
			name: "override missing path",
			guide: &CompanionGuide{
				ID:               "override_no_path",
				Name:             "Override No Path",
				TransactionTypes: []string{"837P"},
				Overrides: []ElementOverride{
					{Requirement: RequirementRequired},
				},
			},
			wantErr: true,
			errMsg:  "path is required",
		},
		{
			name: "override missing requirement",
			guide: &CompanionGuide{
				ID:               "override_no_req",
				Name:             "Override No Requirement",
				TransactionTypes: []string{"837P"},
				Overrides: []ElementOverride{
					{Path: "CLM.01"},
				},
			},
			wantErr: true,
			errMsg:  "requirement is required",
		},
		{
			name: "code restriction missing path",
			guide: &CompanionGuide{
				ID:               "code_no_path",
				Name:             "Code No Path",
				TransactionTypes: []string{"837P"},
				CodeRestrictions: []CodeRestriction{
					{Values: []string{"A", "B"}},
				},
			},
			wantErr: true,
			errMsg:  "path is required",
		},
		{
			name: "code restriction missing values",
			guide: &CompanionGuide{
				ID:               "code_no_values",
				Name:             "Code No Values",
				TransactionTypes: []string{"837P"},
				CodeRestrictions: []CodeRestriction{
					{Path: "SBR.09"},
				},
			},
			wantErr: true,
			errMsg:  "values are required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateGuide(tt.guide)
			if tt.wantErr {
				if err == nil {
					t.Errorf("validateGuide() should have returned error")
				} else if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("Error = %q, should contain %q", err, tt.errMsg)
				}
			} else if err != nil {
				t.Errorf("validateGuide() unexpected error: %v", err)
			}
		})
	}
}

func TestApplyDefaults(t *testing.T) {
	guide := &CompanionGuide{
		ID:               "defaults",
		Name:             "Defaults Test",
		TransactionTypes: []string{"837P"},
		Validations: []ValidationRule{
			{ID: "NO_SEVERITY", Path: "CLM.01", Type: ValidationPattern},
			{ID: "HAS_SEVERITY", Path: "CLM.02", Type: ValidationPattern, Severity: SeverityWarning},
		},
		CodeRestrictions: []CodeRestriction{
			{Path: "SBR.09", Values: []string{"MA"}},
			{Path: "SBR.02", Values: []string{"18"}, Severity: SeverityWarning},
		},
	}

	applyDefaults(guide)

	// Check validation rule defaults
	if guide.Validations[0].Severity != SeverityError {
		t.Errorf("First rule severity = %q, want error", guide.Validations[0].Severity)
	}
	if guide.Validations[1].Severity != SeverityWarning {
		t.Errorf("Second rule severity = %q, want warning (unchanged)", guide.Validations[1].Severity)
	}

	// Check code restriction defaults
	if guide.CodeRestrictions[0].Severity != SeverityError {
		t.Errorf("First restriction severity = %q, want error", guide.CodeRestrictions[0].Severity)
	}
	if guide.CodeRestrictions[1].Severity != SeverityWarning {
		t.Errorf("Second restriction severity = %q, want warning (unchanged)", guide.CodeRestrictions[1].Severity)
	}
}

func TestParseGuideFromString(t *testing.T) {
	yamlContent := `
id: string_test
name: String Test
transaction_types: ["837P"]
`

	guide, err := ParseGuideFromString(yamlContent)
	if err != nil {
		t.Fatalf("ParseGuideFromString failed: %v", err)
	}
	if guide.ID != "string_test" {
		t.Errorf("ID = %q, want string_test", guide.ID)
	}
}

func TestParseGuideFromString_JSON(t *testing.T) {
	jsonContent := `{"id": "json_string", "name": "JSON String", "transaction_types": ["837P"]}`

	guide, err := ParseGuideFromString(jsonContent)
	if err != nil {
		t.Fatalf("ParseGuideFromString failed for JSON: %v", err)
	}
	if guide.ID != "json_string" {
		t.Errorf("ID = %q, want json_string", guide.ID)
	}
}

func TestParseGuideFromString_Invalid(t *testing.T) {
	invalidContent := `{not valid yaml or json`

	_, err := ParseGuideFromString(invalidContent)
	if err == nil {
		t.Error("ParseGuideFromString should fail for invalid content")
	}
}

func TestLoadGuideFromYAML_Invalid(t *testing.T) {
	invalidYAML := `{not: valid: yaml`
	_, err := LoadGuideFromYAML(strings.NewReader(invalidYAML))
	if err == nil {
		t.Error("LoadGuideFromYAML should fail for invalid YAML")
	}
}

func TestLoadGuideFromJSON_Invalid(t *testing.T) {
	invalidJSON := `{not valid json`
	_, err := LoadGuideFromJSON(strings.NewReader(invalidJSON))
	if err == nil {
		t.Error("LoadGuideFromJSON should fail for invalid JSON")
	}
}
