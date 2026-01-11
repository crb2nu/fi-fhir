package companion

import "testing"

func TestNewValidationResult(t *testing.T) {
	result := NewValidationResult("test_guide", "837P")

	if result == nil {
		t.Fatal("NewValidationResult returned nil")
	}
	if !result.Valid {
		t.Error("New result should be valid by default")
	}
	if result.GuideID != "test_guide" {
		t.Errorf("GuideID = %q, want test_guide", result.GuideID)
	}
	if result.TransactionType != "837P" {
		t.Errorf("TransactionType = %q, want 837P", result.TransactionType)
	}
	if len(result.Errors) != 0 {
		t.Errorf("Errors should be empty, got %d", len(result.Errors))
	}
	if len(result.Warnings) != 0 {
		t.Errorf("Warnings should be empty, got %d", len(result.Warnings))
	}
	if len(result.Info) != 0 {
		t.Errorf("Info should be empty, got %d", len(result.Info))
	}
}

func TestValidationResult_AddError(t *testing.T) {
	result := NewValidationResult("test", "837P")

	issue := ValidationIssue{
		Code:    "TEST_ERROR",
		Message: "Test error message",
		Path:    "CLM.01",
	}

	result.AddError(issue)

	if result.Valid {
		t.Error("Result should be invalid after adding error")
	}
	if len(result.Errors) != 1 {
		t.Fatalf("Errors count = %d, want 1", len(result.Errors))
	}
	if result.Errors[0].Code != "TEST_ERROR" {
		t.Errorf("Error code = %q, want TEST_ERROR", result.Errors[0].Code)
	}
	if result.Errors[0].Severity != SeverityError {
		t.Errorf("Error severity = %q, want error", result.Errors[0].Severity)
	}
}

func TestValidationResult_AddWarning(t *testing.T) {
	result := NewValidationResult("test", "837P")

	issue := ValidationIssue{
		Code:    "TEST_WARNING",
		Message: "Test warning message",
	}

	result.AddWarning(issue)

	if !result.Valid {
		t.Error("Result should still be valid after adding warning")
	}
	if len(result.Warnings) != 1 {
		t.Fatalf("Warnings count = %d, want 1", len(result.Warnings))
	}
	if result.Warnings[0].Code != "TEST_WARNING" {
		t.Errorf("Warning code = %q, want TEST_WARNING", result.Warnings[0].Code)
	}
	if result.Warnings[0].Severity != SeverityWarning {
		t.Errorf("Warning severity = %q, want warning", result.Warnings[0].Severity)
	}
}

func TestValidationResult_AddInfo(t *testing.T) {
	result := NewValidationResult("test", "837P")

	issue := ValidationIssue{
		Code:    "TEST_INFO",
		Message: "Test info message",
	}

	result.AddInfo(issue)

	if !result.Valid {
		t.Error("Result should still be valid after adding info")
	}
	if len(result.Info) != 1 {
		t.Fatalf("Info count = %d, want 1", len(result.Info))
	}
	if result.Info[0].Code != "TEST_INFO" {
		t.Errorf("Info code = %q, want TEST_INFO", result.Info[0].Code)
	}
	if result.Info[0].Severity != SeverityInfo {
		t.Errorf("Info severity = %q, want info", result.Info[0].Severity)
	}
}

func TestValidationResult_TotalIssues(t *testing.T) {
	result := NewValidationResult("test", "837P")

	if result.TotalIssues() != 0 {
		t.Errorf("TotalIssues() = %d, want 0", result.TotalIssues())
	}

	result.AddError(ValidationIssue{Code: "E1", Message: "Error 1"})
	if result.TotalIssues() != 1 {
		t.Errorf("TotalIssues() after 1 error = %d, want 1", result.TotalIssues())
	}

	result.AddWarning(ValidationIssue{Code: "W1", Message: "Warning 1"})
	if result.TotalIssues() != 2 {
		t.Errorf("TotalIssues() after 1 error + 1 warning = %d, want 2", result.TotalIssues())
	}

	result.AddInfo(ValidationIssue{Code: "I1", Message: "Info 1"})
	if result.TotalIssues() != 2 {
		t.Errorf("TotalIssues() should not count info, got %d", result.TotalIssues())
	}

	result.AddError(ValidationIssue{Code: "E2", Message: "Error 2"})
	result.AddWarning(ValidationIssue{Code: "W2", Message: "Warning 2"})
	if result.TotalIssues() != 4 {
		t.Errorf("TotalIssues() = %d, want 4", result.TotalIssues())
	}
}

func TestValidationResult_MultipleErrors(t *testing.T) {
	result := NewValidationResult("test", "837P")

	for i := 0; i < 5; i++ {
		result.AddError(ValidationIssue{Code: "E", Message: "Error"})
	}

	if result.Valid {
		t.Error("Result should be invalid after multiple errors")
	}
	if len(result.Errors) != 5 {
		t.Errorf("Errors count = %d, want 5", len(result.Errors))
	}
}

func TestRequirementLevel_Constants(t *testing.T) {
	// Verify the constants are as expected
	if RequirementRequired != "required" {
		t.Errorf("RequirementRequired = %q, want required", RequirementRequired)
	}
	if RequirementOptional != "optional" {
		t.Errorf("RequirementOptional = %q, want optional", RequirementOptional)
	}
	if RequirementSituational != "situational" {
		t.Errorf("RequirementSituational = %q, want situational", RequirementSituational)
	}
	if RequirementNotUsed != "not_used" {
		t.Errorf("RequirementNotUsed = %q, want not_used", RequirementNotUsed)
	}
}

func TestValidationType_Constants(t *testing.T) {
	// Verify the constants are as expected
	if ValidationPattern != "pattern" {
		t.Errorf("ValidationPattern = %q, want pattern", ValidationPattern)
	}
	if ValidationLuhn != "luhn" {
		t.Errorf("ValidationLuhn = %q, want luhn", ValidationLuhn)
	}
	if ValidationMBI != "mbi" {
		t.Errorf("ValidationMBI = %q, want mbi", ValidationMBI)
	}
	if ValidationLength != "length" {
		t.Errorf("ValidationLength = %q, want length", ValidationLength)
	}
	if ValidationRange != "range" {
		t.Errorf("ValidationRange = %q, want range", ValidationRange)
	}
	if ValidationDate != "date" {
		t.Errorf("ValidationDate = %q, want date", ValidationDate)
	}
	if ValidationCustom != "custom" {
		t.Errorf("ValidationCustom = %q, want custom", ValidationCustom)
	}
}

func TestIssueSeverity_Constants(t *testing.T) {
	// Verify the constants are as expected
	if SeverityError != "error" {
		t.Errorf("SeverityError = %q, want error", SeverityError)
	}
	if SeverityWarning != "warning" {
		t.Errorf("SeverityWarning = %q, want warning", SeverityWarning)
	}
	if SeverityInfo != "info" {
		t.Errorf("SeverityInfo = %q, want info", SeverityInfo)
	}
}

func TestCompanionGuide_Structure(t *testing.T) {
	guide := &CompanionGuide{
		ID:               "test_guide",
		Name:             "Test Guide",
		PayerID:          "PAYER",
		ReceiverIDs:      []string{"REC1", "REC2"},
		BaseGuide:        "005010X222A1",
		TransactionTypes: []string{"837P", "835"},
		Description:      "Test description",
		Version:          "1.0.0",
		Overrides: []ElementOverride{
			{Path: "CLM.01", Requirement: RequirementRequired},
		},
		Validations: []ValidationRule{
			{ID: "TEST", Path: "CLM.01", Type: ValidationPattern, Message: "Test"},
		},
		CodeRestrictions: []CodeRestriction{
			{Path: "SBR.09", Values: []string{"MA", "MB"}},
		},
		SegmentRequirements: []SegmentRequirement{
			{Loop: "2010AA", Segment: "NM1", Requirement: RequirementRequired},
		},
	}

	// Verify all fields are accessible
	if guide.ID != "test_guide" {
		t.Errorf("ID = %q, want test_guide", guide.ID)
	}
	if guide.Name != "Test Guide" {
		t.Errorf("Name = %q, want Test Guide", guide.Name)
	}
	if len(guide.ReceiverIDs) != 2 {
		t.Errorf("ReceiverIDs count = %d, want 2", len(guide.ReceiverIDs))
	}
	if len(guide.TransactionTypes) != 2 {
		t.Errorf("TransactionTypes count = %d, want 2", len(guide.TransactionTypes))
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
	if len(guide.SegmentRequirements) != 1 {
		t.Errorf("SegmentRequirements count = %d, want 1", len(guide.SegmentRequirements))
	}
}

func TestElementOverride_Structure(t *testing.T) {
	override := ElementOverride{
		Path:        "2010AA.NM1.09",
		Requirement: RequirementRequired,
		Note:        "NPI required",
		Condition:   "NM1.08=XX",
	}

	if override.Path != "2010AA.NM1.09" {
		t.Errorf("Path = %q", override.Path)
	}
	if override.Requirement != RequirementRequired {
		t.Errorf("Requirement = %q", override.Requirement)
	}
	if override.Note != "NPI required" {
		t.Errorf("Note = %q", override.Note)
	}
	if override.Condition != "NM1.08=XX" {
		t.Errorf("Condition = %q", override.Condition)
	}
}

func TestValidationRule_Structure(t *testing.T) {
	minVal := 0.01
	maxVal := 999.99
	rule := ValidationRule{
		ID:         "TEST_RULE",
		Path:       "CLM.02",
		Type:       ValidationRange,
		Pattern:    "^\\d+$",
		MinLength:  1,
		MaxLength:  10,
		MinValue:   &minVal,
		MaxValue:   &maxVal,
		DateFormat: "20060102",
		Message:    "Test message",
		Severity:   SeverityError,
		Required:   true,
		Condition:  "exists(CLM.01)",
	}

	if rule.ID != "TEST_RULE" {
		t.Errorf("ID = %q", rule.ID)
	}
	if *rule.MinValue != 0.01 {
		t.Errorf("MinValue = %f", *rule.MinValue)
	}
	if *rule.MaxValue != 999.99 {
		t.Errorf("MaxValue = %f", *rule.MaxValue)
	}
}

func TestCodeRestriction_Structure(t *testing.T) {
	restriction := CodeRestriction{
		Path:      "SBR.09",
		Values:    []string{"MA", "MB", "MC"},
		Message:   "Invalid code",
		Severity:  SeverityError,
		Condition: "SBR.01=P",
	}

	if len(restriction.Values) != 3 {
		t.Errorf("Values count = %d, want 3", len(restriction.Values))
	}
	if restriction.Severity != SeverityError {
		t.Errorf("Severity = %q", restriction.Severity)
	}
}

func TestSegmentRequirement_Structure(t *testing.T) {
	req := SegmentRequirement{
		Loop:        "2010AA",
		Segment:     "NM1",
		Requirement: RequirementRequired,
		Note:        "Billing provider name",
		Condition:   "exists(2000A)",
	}

	if req.Loop != "2010AA" {
		t.Errorf("Loop = %q", req.Loop)
	}
	if req.Segment != "NM1" {
		t.Errorf("Segment = %q", req.Segment)
	}
	if req.Requirement != RequirementRequired {
		t.Errorf("Requirement = %q", req.Requirement)
	}
}

func TestValidationIssue_Structure(t *testing.T) {
	issue := ValidationIssue{
		Code:            "INVALID_NPI",
		Message:         "NPI is invalid",
		Path:            "2010AA.NM1.09",
		Value:           "123456789",
		Severity:        SeverityError,
		RuleID:          "NPI_CHECK",
		SegmentID:       "NM1",
		SegmentPosition: 5,
	}

	if issue.Code != "INVALID_NPI" {
		t.Errorf("Code = %q", issue.Code)
	}
	if issue.Path != "2010AA.NM1.09" {
		t.Errorf("Path = %q", issue.Path)
	}
	if issue.SegmentPosition != 5 {
		t.Errorf("SegmentPosition = %d", issue.SegmentPosition)
	}
}
