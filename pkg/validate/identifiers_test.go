package validate

import "testing"

func TestNPIValidator(t *testing.T) {
	v := NewNPIValidator()

	tests := []struct {
		name  string
		npi   string
		valid bool
		code  string
	}{
		{"valid NPI", "1234567893", true, ""},
		{"valid NPI with spaces", "123 456 7893", true, ""},
		{"another valid NPI", "1497758544", true, ""},
		{"invalid checksum", "1234567890", false, "INVALID_NPI_CHECKSUM"},
		{"too short", "123456789", false, "INVALID_NPI_LENGTH"},
		{"too long", "12345678901", false, "INVALID_NPI_LENGTH"},
		{"starts with 0", "0234567893", false, "INVALID_NPI_START"},
		{"contains letters", "123456789A", false, "INVALID_NPI_FORMAT"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := v.Validate(tt.npi)
			if result.Valid != tt.valid {
				t.Errorf("Validate(%q) = %v, want %v (code: %s, msg: %s)",
					tt.npi, result.Valid, tt.valid, result.Code, result.Message)
			}
			if !result.Valid && tt.code != "" && result.Code != tt.code {
				t.Errorf("Validate(%q) code = %q, want %q",
					tt.npi, result.Code, tt.code)
			}
		})
	}
}

func TestDEAValidator(t *testing.T) {
	v := NewDEAValidator()

	// DEA checksum: sum of pos 1,3,5 + 2*(sum of pos 2,4,6), last digit = result mod 10
	// For AB1234563: 1+3+5 + 2*(2+4+6) = 9 + 24 = 33, check digit = 3 ✓
	// For AB5836412: 5+3+4 + 2*(8+6+1) = 12 + 30 = 42, check digit = 2 ✓

	tests := []struct {
		name  string
		dea   string
		valid bool
		code  string
	}{
		{"valid DEA", "AB1234563", true, ""},
		{"valid DEA lowercase", "ab1234563", true, ""},
		{"another valid DEA", "AB5836412", true, ""},
		{"invalid checksum", "AB1234560", false, "INVALID_DEA_CHECKSUM"},
		{"too short", "AB123456", false, "INVALID_DEA_LENGTH"},
		{"too long", "AB12345678", false, "INVALID_DEA_LENGTH"},
		{"invalid first letter", "ZB1234563", false, "INVALID_DEA_REGISTRANT"},
		{"letters in digit section", "AB123A563", false, "INVALID_DEA_DIGITS"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := v.Validate(tt.dea)
			if result.Valid != tt.valid {
				t.Errorf("Validate(%q) = %v, want %v (code: %s, msg: %s)",
					tt.dea, result.Valid, tt.valid, result.Code, result.Message)
			}
			if !result.Valid && tt.code != "" && result.Code != tt.code {
				t.Errorf("Validate(%q) code = %q, want %q",
					tt.dea, result.Code, tt.code)
			}
		})
	}
}

func TestMBIValidator(t *testing.T) {
	v := NewMBIValidator()

	// MBI Format: C A AN N A AN AN N AN AN AN
	// Position 1: 1-9 (no 0)
	// Position 2: A-Z (no SLOIBZ)
	// Position 3: 0-9 or A-Z (no SLOIBZ)
	// Position 4: 0-9
	// Position 5: A-Z (no SLOIBZ)
	// Position 6-7: 0-9 or A-Z (no SLOIBZ)
	// Position 8: 0-9
	// Position 9-11: 0-9 or A-Z (no SLOIBZ)
	// Example: 1EG4-TE5-8K72 where position 8 is digit '8'

	tests := []struct {
		name  string
		mbi   string
		valid bool
		code  string
	}{
		{"valid MBI", "1EG4TE58K72", true, ""},
		{"valid MBI with dashes", "1EG4-TE5-8K72", true, ""},
		{"valid MBI lowercase", "1eg4te58k72", true, ""},
		{"starts with 0", "0EG4TE58K72", false, "INVALID_MBI_POS1"},
		{"excluded letter S", "1SG4TE58K72", false, "INVALID_MBI_POS2"},
		{"excluded letter O", "1OG4TE58K72", false, "INVALID_MBI_POS2"},
		{"pos 8 must be digit", "1EG4TE5MK72", false, "INVALID_MBI_POS8"},
		{"too short", "1EG4TE58K7", false, "INVALID_MBI_LENGTH"},
		{"too long", "1EG4TE58K721", false, "INVALID_MBI_LENGTH"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := v.Validate(tt.mbi)
			if result.Valid != tt.valid {
				t.Errorf("Validate(%q) = %v, want %v (code: %s, msg: %s)",
					tt.mbi, result.Valid, tt.valid, result.Code, result.Message)
			}
			if !result.Valid && tt.code != "" && result.Code != tt.code {
				t.Errorf("Validate(%q) code = %q, want %q",
					tt.mbi, result.Code, tt.code)
			}
		})
	}
}

func TestSSNValidator(t *testing.T) {
	v := NewSSNValidator(nil) // uses default reject patterns including 123456789

	tests := []struct {
		name  string
		ssn   string
		valid bool
		code  string
	}{
		{"valid SSN", "078051120", true, ""}, // Example valid SSN
		{"valid SSN with dashes", "078-05-1120", true, ""},
		{"area 000", "000456789", false, "INVALID_SSN_AREA"},
		{"area 666", "666456789", false, "INVALID_SSN_AREA"},
		{"area 900+", "900456789", false, "INVALID_SSN_AREA"},
		{"group 00", "123006789", false, "INVALID_SSN_GROUP"},
		{"serial 0000", "123450000", false, "INVALID_SSN_SERIAL"},
		{"too short", "12345678", false, "INVALID_SSN_LENGTH"},
		{"too long", "1234567890", false, "INVALID_SSN_LENGTH"},
		{"reject pattern 000000000", "000000000", false, "INVALID_SSN_AREA"}, // caught by area check first
		{"reject pattern 111111111", "111111111", false, "INVALID_SSN_PATTERN"},
		{"reject pattern 123456789", "123456789", false, "INVALID_SSN_PATTERN"}, // default reject pattern
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := v.Validate(tt.ssn)
			if result.Valid != tt.valid {
				t.Errorf("Validate(%q) = %v, want %v (code: %s, msg: %s)",
					tt.ssn, result.Valid, tt.valid, result.Code, result.Message)
			}
			if !result.Valid && tt.code != "" && result.Code != tt.code {
				t.Errorf("Validate(%q) code = %q, want %q",
					tt.ssn, result.Code, tt.code)
			}
		})
	}
}

func TestPhoneNormalizer(t *testing.T) {
	tests := []struct {
		name             string
		input            string
		stripCountryCode bool
		want             string
	}{
		{"digits only", "5551234567", true, "5551234567"},
		{"with dashes", "555-123-4567", true, "5551234567"},
		{"with parens", "(555) 123-4567", true, "5551234567"},
		{"with country code", "1-555-123-4567", true, "5551234567"},
		{"keep country code", "1-555-123-4567", false, "15551234567"},
		{"international", "+1 555 123 4567", true, "5551234567"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := NewPhoneNormalizer(tt.stripCountryCode)
			got := n.Normalize(tt.input)
			if got != tt.want {
				t.Errorf("Normalize(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestSSNNormalizer(t *testing.T) {
	n := NewSSNNormalizer()

	tests := []struct {
		input string
		want  string
	}{
		{"123-45-6789", "123456789"},
		{"123 45 6789", "123456789"},
		{"123456789", "123456789"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := n.Normalize(tt.input)
			if got != tt.want {
				t.Errorf("Normalize(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// Benchmark NPI validation
func BenchmarkNPIValidation(b *testing.B) {
	v := NewNPIValidator()
	for i := 0; i < b.N; i++ {
		v.Validate("1234567893")
	}
}
