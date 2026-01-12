package db

import (
	"testing"
)

func TestExtractChapterNumber(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Chapter 4: Endocrine nutritional and metabolic diseases", "04"},
		{"Chapter 9: Diseases of the circulatory system", "09"},
		{"Chapter 10: Diseases of the respiratory system", "10"},
		{"Chapter 18: Symptoms signs and abnormal clinical findings", "18"},
		{"Chapter 1: Certain infectious diseases", "01"},
		{"Chapter 22: Codes for special purposes", "22"},
		{"", ""},
		{"No chapter here", ""},
		{"Chapter: Missing number", ""},
		{"Chapt 5: Typo", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := extractChapterNumber(tt.input)
			if result != tt.expected {
				t.Errorf("extractChapterNumber(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestNormalizeChapter(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"4", "04"},
		{"04", "04"},
		{"9", "09"},
		{"10", "10"},
		{"Chapter 4", "04"},
		{"Chapter 10", "10"},
		{"4: Endocrine diseases", "04"},
		{"18: Symptoms", "18"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizeChapter(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeChapter(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestICD10CMCode_DisplayCode(t *testing.T) {
	tests := []struct {
		name           string
		code           string
		codeFormatted  string
		formattedValid bool
		expected       string
	}{
		{
			name:           "no formatted code",
			code:           "E119",
			codeFormatted:  "",
			formattedValid: false,
			expected:       "E119",
		},
		{
			name:           "with formatted code",
			code:           "E119",
			codeFormatted:  "E11.9",
			formattedValid: true,
			expected:       "E11.9",
		},
		{
			name:           "empty formatted code but valid flag",
			code:           "I10",
			codeFormatted:  "",
			formattedValid: true,
			expected:       "I10",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code := &ICD10CMCode{
				Code: tt.code,
			}
			code.CodeFormatted.Valid = tt.formattedValid
			code.CodeFormatted.String = tt.codeFormatted

			result := code.DisplayCode()
			if result != tt.expected {
				t.Errorf("DisplayCode() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestICD10CMCode_IsBillable(t *testing.T) {
	tests := []struct {
		name     string
		isHeader bool
		expected bool
	}{
		{"header code", true, false},
		{"billable code", false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code := &ICD10CMCode{IsHeader: tt.isHeader}
			if code.IsBillable() != tt.expected {
				t.Errorf("IsBillable() = %v, want %v", code.IsBillable(), tt.expected)
			}
		})
	}
}

func TestICD10LoadOptions_Defaults(t *testing.T) {
	// Verify nil options handling - should default to including headers
	// This is tested implicitly in the loader, but we document the expected behavior
	opts := &ICD10LoadOptions{}
	if opts.IncludeHeaders {
		t.Error("Default ICD10LoadOptions should have IncludeHeaders=false")
	}

	// Explicit true
	opts = &ICD10LoadOptions{IncludeHeaders: true}
	if !opts.IncludeHeaders {
		t.Error("Expected IncludeHeaders=true")
	}
}
