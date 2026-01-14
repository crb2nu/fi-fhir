package companion

import (
	"testing"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/parser/edi"
)

func TestNewValidator(t *testing.T) {
	guide := &CompanionGuide{
		ID:   "test",
		Name: "Test Guide",
	}

	v := NewValidator(guide)
	if v == nil {
		t.Fatal("NewValidator returned nil")
	}
	if v.guide != guide {
		t.Error("Validator guide not set correctly")
	}
	if v.npiVal == nil {
		t.Error("NPI validator not initialized")
	}
	if v.mbiVal == nil {
		t.Error("MBI validator not initialized")
	}
	if v.patterns == nil {
		t.Error("patterns cache not initialized")
	}
}

func TestValidator_ValidateOverrides(t *testing.T) {
	tests := []struct {
		name       string
		guide      *CompanionGuide
		segments   []*edi.Segment
		wantValid  bool
		wantErrors int
	}{
		{
			name: "required element present",
			guide: &CompanionGuide{
				ID:   "test",
				Name: "Test",
				Overrides: []ElementOverride{
					{Path: "CLM.01", Requirement: RequirementRequired},
				},
			},
			segments: []*edi.Segment{
				{ID: "CLM", Elements: []string{"CLAIM001", "1500.00"}},
			},
			wantValid:  true,
			wantErrors: 0,
		},
		{
			name: "required element missing",
			guide: &CompanionGuide{
				ID:   "test",
				Name: "Test",
				Overrides: []ElementOverride{
					{Path: "CLM.03", Requirement: RequirementRequired},
				},
			},
			segments: []*edi.Segment{
				{ID: "CLM", Elements: []string{"CLAIM001", "1500.00"}},
			},
			wantValid:  false,
			wantErrors: 1,
		},
		{
			name: "not_used element present",
			guide: &CompanionGuide{
				ID:   "test",
				Name: "Test",
				Overrides: []ElementOverride{
					{Path: "CLM.01", Requirement: RequirementNotUsed},
				},
			},
			segments: []*edi.Segment{
				{ID: "CLM", Elements: []string{"CLAIM001", "1500.00"}},
			},
			wantValid:  true, // warnings don't affect validity
			wantErrors: 0,
		},
		{
			name: "conditional override - condition met",
			guide: &CompanionGuide{
				ID:   "test",
				Name: "Test",
				Overrides: []ElementOverride{
					{
						Path:        "NM1.09",
						Requirement: RequirementRequired,
						Condition:   "NM1.08=XX",
					},
				},
			},
			segments: []*edi.Segment{
				{ID: "NM1", Elements: []string{"85", "2", "NAME", "", "", "", "", "XX", "1234567890"}},
			},
			wantValid:  true,
			wantErrors: 0,
		},
		{
			name: "conditional override - condition not met",
			guide: &CompanionGuide{
				ID:   "test",
				Name: "Test",
				Overrides: []ElementOverride{
					{
						Path:        "NM1.09",
						Requirement: RequirementRequired,
						Condition:   "NM1.08=XX",
					},
				},
			},
			segments: []*edi.Segment{
				{ID: "NM1", Elements: []string{"85", "2", "NAME", "", "", "", "", "FI", ""}},
			},
			wantValid:  true, // condition not met, so override skipped
			wantErrors: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx := &edi.Transaction{
				SetIdentifier: "837",
				Segments:      tt.segments,
			}

			v := NewValidator(tt.guide)
			result := v.Validate(tx, edi.DefaultDelimiters())

			if result.Valid != tt.wantValid {
				t.Errorf("Valid = %v, want %v", result.Valid, tt.wantValid)
			}
			if len(result.Errors) != tt.wantErrors {
				t.Errorf("Errors = %d, want %d: %+v", len(result.Errors), tt.wantErrors, result.Errors)
			}
		})
	}
}

func TestValidator_ValidateRules_Pattern(t *testing.T) {
	guide := &CompanionGuide{
		ID:   "test",
		Name: "Test",
		Validations: []ValidationRule{
			{
				ID:       "NPI_QUALIFIER",
				Path:     "NM1.08",
				Type:     ValidationPattern,
				Pattern:  "^XX$",
				Message:  "Qualifier must be XX",
				Required: true,
			},
		},
	}

	tests := []struct {
		name       string
		value      string
		wantValid  bool
		wantErrors int
	}{
		{"valid XX", "XX", true, 0},
		{"invalid FI", "FI", false, 1},
		{"invalid empty", "", false, 1}, // required but empty
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx := &edi.Transaction{
				SetIdentifier: "837",
				Segments: []*edi.Segment{
					{ID: "NM1", Elements: []string{"85", "2", "NAME", "", "", "", "", tt.value}},
				},
			}

			v := NewValidator(guide)
			result := v.Validate(tx, edi.DefaultDelimiters())

			if result.Valid != tt.wantValid {
				t.Errorf("Valid = %v, want %v", result.Valid, tt.wantValid)
			}
			if len(result.Errors) != tt.wantErrors {
				t.Errorf("Errors = %d, want %d", len(result.Errors), tt.wantErrors)
			}
		})
	}
}

func TestValidator_ValidateRules_Luhn(t *testing.T) {
	guide := &CompanionGuide{
		ID:   "test",
		Name: "Test",
		Validations: []ValidationRule{
			{
				ID:       "NPI_CHECK",
				Path:     "NM1.09",
				Type:     ValidationLuhn,
				Message:  "NPI must pass Luhn check",
				Required: false,
			},
		},
	}

	tests := []struct {
		name       string
		npi        string
		wantValid  bool
		wantErrors int
	}{
		{"valid NPI", "1234567893", true, 0},
		{"invalid NPI", "1234567890", false, 1},
		{"empty NPI", "", true, 0}, // not required
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx := &edi.Transaction{
				SetIdentifier: "837",
				Segments: []*edi.Segment{
					{ID: "NM1", Elements: []string{"85", "2", "NAME", "", "", "", "", "XX", tt.npi}},
				},
			}

			v := NewValidator(guide)
			result := v.Validate(tx, edi.DefaultDelimiters())

			if result.Valid != tt.wantValid {
				t.Errorf("Valid = %v, want %v", result.Valid, tt.wantValid)
			}
			if len(result.Errors) != tt.wantErrors {
				t.Errorf("Errors = %d, want %d", len(result.Errors), tt.wantErrors)
			}
		})
	}
}

func TestValidator_ValidateRules_MBI(t *testing.T) {
	guide := &CompanionGuide{
		ID:   "test",
		Name: "Test",
		Validations: []ValidationRule{
			{
				ID:       "MBI_CHECK",
				Path:     "NM1.09",
				Type:     ValidationMBI,
				Message:  "MBI format invalid",
				Required: true,
			},
		},
	}

	// MBI format: Position 1=1-9, 2=alpha, 3=alphanum, 4=digit, 5=alpha, 6-7=alphanum, 8=digit, 9-11=alphanum
	// Example: 1EG4TE58K73
	tests := []struct {
		name       string
		mbi        string
		wantValid  bool
		wantErrors int
	}{
		{"valid MBI", "1EG4TE58K73", true, 0},
		{"invalid MBI", "123456789", false, 1},
		{"empty MBI", "", false, 1}, // required
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx := &edi.Transaction{
				SetIdentifier: "837",
				Segments: []*edi.Segment{
					{ID: "NM1", Elements: []string{"85", "2", "NAME", "", "", "", "", "MI", tt.mbi}},
				},
			}

			v := NewValidator(guide)
			result := v.Validate(tx, edi.DefaultDelimiters())

			if result.Valid != tt.wantValid {
				t.Errorf("Valid = %v, want %v", result.Valid, tt.wantValid)
			}
			if len(result.Errors) != tt.wantErrors {
				t.Errorf("Errors = %d, want %d", len(result.Errors), tt.wantErrors)
			}
		})
	}
}

func TestValidator_ValidateRules_Length(t *testing.T) {
	guide := &CompanionGuide{
		ID:   "test",
		Name: "Test",
		Validations: []ValidationRule{
			{
				ID:        "ID_LENGTH",
				Path:      "CLM.01",
				Type:      ValidationLength,
				MinLength: 5,
				MaxLength: 20,
				Message:   "Claim ID must be 5-20 characters",
			},
		},
	}

	tests := []struct {
		name       string
		claimID    string
		wantValid  bool
		wantErrors int
	}{
		{"valid length", "CLAIM001", true, 0},
		{"too short", "ABC", false, 1},
		{"too long", "ABCDEFGHIJKLMNOPQRSTUVWXYZ", false, 1},
		{"min boundary", "12345", true, 0},
		{"max boundary", "12345678901234567890", true, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx := &edi.Transaction{
				SetIdentifier: "837",
				Segments: []*edi.Segment{
					{ID: "CLM", Elements: []string{tt.claimID, "1500.00"}},
				},
			}

			v := NewValidator(guide)
			result := v.Validate(tx, edi.DefaultDelimiters())

			if result.Valid != tt.wantValid {
				t.Errorf("Valid = %v, want %v", result.Valid, tt.wantValid)
			}
			if len(result.Errors) != tt.wantErrors {
				t.Errorf("Errors = %d, want %d", len(result.Errors), tt.wantErrors)
			}
		})
	}
}

func TestValidator_ValidateRules_Range(t *testing.T) {
	minVal := 0.01
	maxVal := 999999.99
	guide := &CompanionGuide{
		ID:   "test",
		Name: "Test",
		Validations: []ValidationRule{
			{
				ID:       "AMOUNT_RANGE",
				Path:     "CLM.02",
				Type:     ValidationRange,
				MinValue: &minVal,
				MaxValue: &maxVal,
				Message:  "Amount must be between 0.01 and 999999.99",
			},
		},
	}

	tests := []struct {
		name       string
		amount     string
		wantValid  bool
		wantErrors int
	}{
		{"valid amount", "1500.00", true, 0},
		{"too low", "0.00", false, 1},
		{"too high", "1000000.00", false, 1},
		{"not numeric", "ABC", false, 1},
		{"min boundary", "0.01", true, 0},
		{"max boundary", "999999.99", true, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx := &edi.Transaction{
				SetIdentifier: "837",
				Segments: []*edi.Segment{
					{ID: "CLM", Elements: []string{"CLAIM001", tt.amount}},
				},
			}

			v := NewValidator(guide)
			result := v.Validate(tx, edi.DefaultDelimiters())

			if result.Valid != tt.wantValid {
				t.Errorf("Valid = %v, want %v", result.Valid, tt.wantValid)
			}
			if len(result.Errors) != tt.wantErrors {
				t.Errorf("Errors = %d, want %d", len(result.Errors), tt.wantErrors)
			}
		})
	}
}

func TestValidator_ValidateRules_Date(t *testing.T) {
	guide := &CompanionGuide{
		ID:   "test",
		Name: "Test",
		Validations: []ValidationRule{
			{
				ID:      "DATE_FORMAT",
				Path:    "DTP.03",
				Type:    ValidationDate,
				Message: "Invalid date format",
			},
		},
	}

	tests := []struct {
		name       string
		date       string
		wantValid  bool
		wantErrors int
	}{
		{"valid 8-digit date", "20240115", true, 0},
		{"valid 6-digit date", "240115", true, 0},
		{"invalid date", "99999999", false, 1},
		{"invalid format", "2024-01-15", false, 1},
		{"invalid length", "202401", false, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx := &edi.Transaction{
				SetIdentifier: "837",
				Segments: []*edi.Segment{
					{ID: "DTP", Elements: []string{"472", "D8", tt.date}},
				},
			}

			v := NewValidator(guide)
			result := v.Validate(tx, edi.DefaultDelimiters())

			if result.Valid != tt.wantValid {
				t.Errorf("Valid = %v, want %v", result.Valid, tt.wantValid)
			}
			if len(result.Errors) != tt.wantErrors {
				t.Errorf("Errors = %d, want %d", len(result.Errors), tt.wantErrors)
			}
		})
	}
}

func TestValidator_ValidateCodeRestrictions(t *testing.T) {
	guide := &CompanionGuide{
		ID:   "test",
		Name: "Test",
		CodeRestrictions: []CodeRestriction{
			{
				Path:   "SBR.09",
				Values: []string{"MA", "MB", "MC"},
			},
		},
	}

	tests := []struct {
		name       string
		code       string
		wantValid  bool
		wantErrors int
	}{
		{"valid code MA", "MA", true, 0},
		{"valid code MB", "MB", true, 0},
		{"invalid code ZZ", "ZZ", false, 1},
		{"empty code", "", true, 0}, // empty is allowed (not a value check)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx := &edi.Transaction{
				SetIdentifier: "837",
				Segments: []*edi.Segment{
					{ID: "SBR", Elements: []string{"P", "18", "", "", "", "", "", "", tt.code}},
				},
			}

			v := NewValidator(guide)
			result := v.Validate(tx, edi.DefaultDelimiters())

			if result.Valid != tt.wantValid {
				t.Errorf("Valid = %v, want %v", result.Valid, tt.wantValid)
			}
			if len(result.Errors) != tt.wantErrors {
				t.Errorf("Errors = %d, want %d", len(result.Errors), tt.wantErrors)
			}
		})
	}
}

func TestValidator_CodeRestrictionSeverity(t *testing.T) {
	tests := []struct {
		name         string
		severity     IssueSeverity
		wantValid    bool
		wantErrors   int
		wantWarnings int
	}{
		{"error severity", SeverityError, false, 1, 0},
		{"warning severity", SeverityWarning, true, 0, 1},
		{"info severity", SeverityInfo, true, 0, 0}, // info goes to Info slice
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			guide := &CompanionGuide{
				ID:   "test",
				Name: "Test",
				CodeRestrictions: []CodeRestriction{
					{
						Path:     "SBR.09",
						Values:   []string{"MA", "MB"},
						Severity: tt.severity,
					},
				},
			}

			tx := &edi.Transaction{
				SetIdentifier: "837",
				Segments: []*edi.Segment{
					{ID: "SBR", Elements: []string{"P", "18", "", "", "", "", "", "", "ZZ"}},
				},
			}

			v := NewValidator(guide)
			result := v.Validate(tx, edi.DefaultDelimiters())

			if result.Valid != tt.wantValid {
				t.Errorf("Valid = %v, want %v", result.Valid, tt.wantValid)
			}
			if len(result.Errors) != tt.wantErrors {
				t.Errorf("Errors = %d, want %d", len(result.Errors), tt.wantErrors)
			}
			if len(result.Warnings) != tt.wantWarnings {
				t.Errorf("Warnings = %d, want %d", len(result.Warnings), tt.wantWarnings)
			}
		})
	}
}

func TestValidator_EvaluateCondition(t *testing.T) {
	tx := &edi.Transaction{
		SetIdentifier: "837",
		Segments: []*edi.Segment{
			{ID: "NM1", Elements: []string{"85", "2", "NAME", "", "", "", "", "XX", "1234567890"}},
			{ID: "CLM", Elements: []string{"CLAIM001", "1500.00"}},
		},
	}

	resolver := NewPathResolver(tx, edi.DefaultDelimiters())
	v := NewValidator(&CompanionGuide{})

	tests := []struct {
		condition string
		want      bool
	}{
		// Equality
		{"NM1.08=XX", true},
		{"NM1.08=FI", false},
		// Inequality
		{"NM1.08!=FI", true},
		{"NM1.08!=XX", false},
		// Exists
		{"exists(CLM.01)", true},
		{"exists(CLM.99)", false},
		// Not exists
		{"!exists(CLM.99)", true},
		{"!exists(CLM.01)", false},
		// Invalid condition (returns true by default)
		{"invalid", true},
	}

	for _, tt := range tests {
		t.Run(tt.condition, func(t *testing.T) {
			got := v.evaluateCondition(tt.condition, resolver)
			if got != tt.want {
				t.Errorf("evaluateCondition(%q) = %v, want %v", tt.condition, got, tt.want)
			}
		})
	}
}

func TestValidator_ValidateWithResolver(t *testing.T) {
	guide := &CompanionGuide{
		ID:   "test",
		Name: "Test",
		Validations: []ValidationRule{
			{
				ID:        "CLM_REQUIRED",
				Path:      "CLM.01",
				Type:      ValidationLength,
				MinLength: 1,
				Message:   "Claim ID required",
				Required:  true,
			},
		},
	}

	tx := &edi.Transaction{
		SetIdentifier: "837",
		Segments: []*edi.Segment{
			{ID: "CLM", Elements: []string{"CLAIM001", "1500.00"}},
		},
	}

	resolver := NewPathResolver(tx, edi.DefaultDelimiters())
	v := NewValidator(guide)
	result := v.ValidateWithResolver(resolver, "837P")

	if !result.Valid {
		t.Errorf("ValidateWithResolver returned invalid: %+v", result.Errors)
	}
	if result.GuideID != "test" {
		t.Errorf("GuideID = %q, want test", result.GuideID)
	}
	if result.TransactionType != "837P" {
		t.Errorf("TransactionType = %q, want 837P", result.TransactionType)
	}
}

func TestValidator_InvalidPattern(t *testing.T) {
	guide := &CompanionGuide{
		ID:   "test",
		Name: "Test",
		Validations: []ValidationRule{
			{
				ID:      "INVALID_PATTERN",
				Path:    "CLM.01",
				Type:    ValidationPattern,
				Pattern: "[invalid", // Invalid regex
				Message: "Pattern error",
			},
		},
	}

	tx := &edi.Transaction{
		SetIdentifier: "837",
		Segments: []*edi.Segment{
			{ID: "CLM", Elements: []string{"CLAIM001", "1500.00"}},
		},
	}

	v := NewValidator(guide)
	result := v.Validate(tx, edi.DefaultDelimiters())

	// Should report an error about the invalid pattern
	if result.Valid {
		t.Error("Expected validation to fail due to invalid pattern")
	}
	if len(result.Errors) == 0 {
		t.Error("Expected error about invalid pattern")
	}
	if result.Errors[0].Code != "INVALID_PATTERN" {
		t.Errorf("Error code = %q, want INVALID_PATTERN", result.Errors[0].Code)
	}
}

func TestValidator_RuleSeverity(t *testing.T) {
	tests := []struct {
		name         string
		severity     IssueSeverity
		wantValid    bool
		wantErrors   int
		wantWarnings int
	}{
		{"default severity (error)", "", false, 1, 0},
		{"error severity", SeverityError, false, 1, 0},
		{"warning severity", SeverityWarning, true, 0, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			guide := &CompanionGuide{
				ID:   "test",
				Name: "Test",
				Validations: []ValidationRule{
					{
						ID:       "TEST_RULE",
						Path:     "CLM.01",
						Type:     ValidationPattern,
						Pattern:  "^VALID$",
						Message:  "Must be VALID",
						Severity: tt.severity,
					},
				},
			}

			tx := &edi.Transaction{
				SetIdentifier: "837",
				Segments: []*edi.Segment{
					{ID: "CLM", Elements: []string{"INVALID", "1500.00"}},
				},
			}

			v := NewValidator(guide)
			result := v.Validate(tx, edi.DefaultDelimiters())

			if result.Valid != tt.wantValid {
				t.Errorf("Valid = %v, want %v", result.Valid, tt.wantValid)
			}
			if len(result.Errors) != tt.wantErrors {
				t.Errorf("Errors = %d, want %d", len(result.Errors), tt.wantErrors)
			}
			if len(result.Warnings) != tt.wantWarnings {
				t.Errorf("Warnings = %d, want %d", len(result.Warnings), tt.wantWarnings)
			}
		})
	}
}

func TestValidator_ConditionalValidation(t *testing.T) {
	guide := &CompanionGuide{
		ID:   "test",
		Name: "Test",
		Validations: []ValidationRule{
			{
				ID:        "NPI_REQUIRED_WHEN_XX",
				Path:      "NM1.09",
				Type:      ValidationLuhn,
				Message:   "NPI must be valid when qualifier is XX",
				Required:  true,
				Condition: "NM1.08=XX",
			},
		},
	}

	tests := []struct {
		name       string
		qualifier  string
		npi        string
		wantValid  bool
		wantErrors int
	}{
		{"XX qualifier with valid NPI", "XX", "1234567893", true, 0},
		{"XX qualifier with invalid NPI", "XX", "1234567890", false, 1},
		{"XX qualifier with missing NPI", "XX", "", false, 1},
		{"non-XX qualifier skips validation", "FI", "", true, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx := &edi.Transaction{
				SetIdentifier: "837",
				Segments: []*edi.Segment{
					{ID: "NM1", Elements: []string{"85", "2", "NAME", "", "", "", "", tt.qualifier, tt.npi}},
				},
			}

			v := NewValidator(guide)
			result := v.Validate(tx, edi.DefaultDelimiters())

			if result.Valid != tt.wantValid {
				t.Errorf("Valid = %v, want %v", result.Valid, tt.wantValid)
			}
			if len(result.Errors) != tt.wantErrors {
				t.Errorf("Errors = %d, want %d", len(result.Errors), tt.wantErrors)
			}
		})
	}
}

func TestValidator_AllSegments(t *testing.T) {
	guide := &CompanionGuide{
		ID:   "test",
		Name: "Test",
		Validations: []ValidationRule{
			{
				ID:        "ALL_REF_LENGTH",
				Path:      "REF[*].02",
				Type:      ValidationLength,
				MinLength: 3,
				Message:   "REF values must be at least 3 characters",
			},
		},
	}

	tests := []struct {
		name       string
		refValues  []string
		wantValid  bool
		wantErrors int
	}{
		{"all valid", []string{"AAA", "BBB", "CCC"}, true, 0},
		{"one too short", []string{"AAA", "BB", "CCC"}, false, 1},
		{"two too short", []string{"AA", "BB", "CCC"}, false, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var segments []*edi.Segment
			for i, v := range tt.refValues {
				qualifier := string(rune('A' + i))
				segments = append(segments, &edi.Segment{
					ID:       "REF",
					Elements: []string{qualifier, v},
				})
			}

			tx := &edi.Transaction{
				SetIdentifier: "837",
				Segments:      segments,
			}

			v := NewValidator(guide)
			result := v.Validate(tx, edi.DefaultDelimiters())

			if result.Valid != tt.wantValid {
				t.Errorf("Valid = %v, want %v", result.Valid, tt.wantValid)
			}
			if len(result.Errors) != tt.wantErrors {
				t.Errorf("Errors = %d, want %d", len(result.Errors), tt.wantErrors)
			}
		})
	}
}
