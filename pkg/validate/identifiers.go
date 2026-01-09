// Package validate provides healthcare identifier validation.
package validate

import (
	"fmt"
	"regexp"
	"strings"
)

// ValidationResult contains the result of an identifier validation.
type ValidationResult struct {
	Valid   bool   `json:"valid"`
	Code    string `json:"code,omitempty"`    // Error code (e.g., "INVALID_NPI_CHECKSUM")
	Message string `json:"message,omitempty"` // Human-readable message
}

// NPIValidator validates National Provider Identifiers.
type NPIValidator struct{}

// NewNPIValidator creates a new NPI validator.
func NewNPIValidator() *NPIValidator {
	return &NPIValidator{}
}

// Validate checks if an NPI is valid.
// NPI must be exactly 10 digits and pass Luhn check with "80840" prefix.
func (v *NPIValidator) Validate(npi string) ValidationResult {
	// Remove any spaces or dashes
	npi = strings.ReplaceAll(npi, " ", "")
	npi = strings.ReplaceAll(npi, "-", "")

	// Must be exactly 10 digits
	if len(npi) != 10 {
		return ValidationResult{
			Valid:   false,
			Code:    "INVALID_NPI_LENGTH",
			Message: fmt.Sprintf("NPI must be 10 digits, got %d", len(npi)),
		}
	}

	// Must be all digits
	if !isNumeric(npi) {
		return ValidationResult{
			Valid:   false,
			Code:    "INVALID_NPI_FORMAT",
			Message: "NPI must contain only digits",
		}
	}

	// First digit cannot be 0
	if npi[0] == '0' {
		return ValidationResult{
			Valid:   false,
			Code:    "INVALID_NPI_START",
			Message: "NPI cannot start with 0",
		}
	}

	// Luhn check with "80840" prefix (healthcare identifier prefix)
	prefixed := "80840" + npi
	if !luhnCheck(prefixed) {
		return ValidationResult{
			Valid:   false,
			Code:    "INVALID_NPI_CHECKSUM",
			Message: "NPI failed Luhn checksum validation",
		}
	}

	return ValidationResult{Valid: true}
}

// luhnCheck performs the Luhn algorithm check.
func luhnCheck(number string) bool {
	sum := 0
	alternate := false

	for i := len(number) - 1; i >= 0; i-- {
		n := int(number[i] - '0')
		if alternate {
			n *= 2
			if n > 9 {
				n -= 9
			}
		}
		sum += n
		alternate = !alternate
	}

	return sum%10 == 0
}

// DEAValidator validates DEA numbers.
type DEAValidator struct{}

// NewDEAValidator creates a new DEA validator.
func NewDEAValidator() *DEAValidator {
	return &DEAValidator{}
}

// Validate checks if a DEA number is valid.
// Format: 2 letters + 6 digits + 1 check digit
// First letter: registrant type (A,B,F,G,M,P,R)
// Second letter: first letter of last name (usually)
func (v *DEAValidator) Validate(dea string) ValidationResult {
	dea = strings.ToUpper(strings.TrimSpace(dea))

	// Must be exactly 9 characters
	if len(dea) != 9 {
		return ValidationResult{
			Valid:   false,
			Code:    "INVALID_DEA_LENGTH",
			Message: fmt.Sprintf("DEA number must be 9 characters, got %d", len(dea)),
		}
	}

	// First two characters must be letters
	if !isAlpha(dea[0]) || !isAlpha(dea[1]) {
		return ValidationResult{
			Valid:   false,
			Code:    "INVALID_DEA_PREFIX",
			Message: "DEA number must start with two letters",
		}
	}

	// First letter must be valid registrant type
	validFirst := "ABFGMPRX" // X for deprecated/special cases
	if !strings.Contains(validFirst, string(dea[0])) {
		return ValidationResult{
			Valid:   false,
			Code:    "INVALID_DEA_REGISTRANT",
			Message: fmt.Sprintf("Invalid DEA registrant type: %c", dea[0]),
		}
	}

	// Last 7 characters must be digits
	digits := dea[2:]
	if !isNumeric(digits) {
		return ValidationResult{
			Valid:   false,
			Code:    "INVALID_DEA_DIGITS",
			Message: "DEA number positions 3-9 must be digits",
		}
	}

	// Checksum validation
	// Sum of positions 1, 3, 5 + 2*(sum of positions 2, 4, 6)
	// Last digit of result must equal position 7
	d := make([]int, 7)
	for i := 0; i < 7; i++ {
		d[i] = int(digits[i] - '0')
	}

	sum := d[0] + d[2] + d[4] + 2*(d[1]+d[3]+d[5])
	checkDigit := sum % 10

	if checkDigit != d[6] {
		return ValidationResult{
			Valid:   false,
			Code:    "INVALID_DEA_CHECKSUM",
			Message: "DEA number failed checksum validation",
		}
	}

	return ValidationResult{Valid: true}
}

// MBIValidator validates Medicare Beneficiary Identifiers.
type MBIValidator struct{}

// NewMBIValidator creates a new MBI validator.
func NewMBIValidator() *MBIValidator {
	return &MBIValidator{}
}

// Validate checks if an MBI is valid.
// Format: XAXX-XXX-XXXX (11 characters, dashes optional)
// Position rules for allowed characters.
func (v *MBIValidator) Validate(mbi string) ValidationResult {
	// Remove dashes and spaces
	mbi = strings.ReplaceAll(mbi, "-", "")
	mbi = strings.ReplaceAll(mbi, " ", "")
	mbi = strings.ToUpper(mbi)

	// Must be exactly 11 characters
	if len(mbi) != 11 {
		return ValidationResult{
			Valid:   false,
			Code:    "INVALID_MBI_LENGTH",
			Message: fmt.Sprintf("MBI must be 11 characters, got %d", len(mbi)),
		}
	}

	// Excluded letters (can be confused with numbers or ambiguous)
	excluded := "SLOIBZ"

	// Position 1: 1-9 (no 0)
	if mbi[0] < '1' || mbi[0] > '9' {
		return ValidationResult{
			Valid:   false,
			Code:    "INVALID_MBI_POS1",
			Message: "MBI position 1 must be 1-9",
		}
	}

	// Position 2: A-Z (no SLOIBZ)
	if !isAlpha(mbi[1]) || strings.Contains(excluded, string(mbi[1])) {
		return ValidationResult{
			Valid:   false,
			Code:    "INVALID_MBI_POS2",
			Message: "MBI position 2 must be a letter (excluding S,L,O,I,B,Z)",
		}
	}

	// Position 3: Alphanumeric (no SLOIBZ)
	if !isAlphanumeric(mbi[2]) || strings.Contains(excluded, string(mbi[2])) {
		return ValidationResult{
			Valid:   false,
			Code:    "INVALID_MBI_POS3",
			Message: "MBI position 3 must be alphanumeric (excluding S,L,O,I,B,Z)",
		}
	}

	// Position 4: 0-9
	if !isDigit(mbi[3]) {
		return ValidationResult{
			Valid:   false,
			Code:    "INVALID_MBI_POS4",
			Message: "MBI position 4 must be a digit",
		}
	}

	// Position 5: A-Z (no SLOIBZ)
	if !isAlpha(mbi[4]) || strings.Contains(excluded, string(mbi[4])) {
		return ValidationResult{
			Valid:   false,
			Code:    "INVALID_MBI_POS5",
			Message: "MBI position 5 must be a letter (excluding S,L,O,I,B,Z)",
		}
	}

	// Positions 6-7: Alphanumeric (no SLOIBZ)
	for i := 5; i <= 6; i++ {
		if !isAlphanumeric(mbi[i]) || strings.Contains(excluded, string(mbi[i])) {
			return ValidationResult{
				Valid:   false,
				Code:    fmt.Sprintf("INVALID_MBI_POS%d", i+1),
				Message: fmt.Sprintf("MBI position %d must be alphanumeric (excluding S,L,O,I,B,Z)", i+1),
			}
		}
	}

	// Position 8: 0-9
	if !isDigit(mbi[7]) {
		return ValidationResult{
			Valid:   false,
			Code:    "INVALID_MBI_POS8",
			Message: "MBI position 8 must be a digit",
		}
	}

	// Positions 9-11: Alphanumeric (no SLOIBZ)
	for i := 8; i <= 10; i++ {
		if !isAlphanumeric(mbi[i]) || strings.Contains(excluded, string(mbi[i])) {
			return ValidationResult{
				Valid:   false,
				Code:    fmt.Sprintf("INVALID_MBI_POS%d", i+1),
				Message: fmt.Sprintf("MBI position %d must be alphanumeric (excluding S,L,O,I,B,Z)", i+1),
			}
		}
	}

	return ValidationResult{Valid: true}
}

// SSNValidator validates Social Security Numbers.
type SSNValidator struct {
	rejectPatterns []string
}

// NewSSNValidator creates a new SSN validator.
func NewSSNValidator(rejectPatterns []string) *SSNValidator {
	if rejectPatterns == nil {
		rejectPatterns = []string{"000000000", "123456789", "111111111", "999999999"}
	}
	return &SSNValidator{rejectPatterns: rejectPatterns}
}

// Validate checks if an SSN is valid.
func (v *SSNValidator) Validate(ssn string) ValidationResult {
	// Remove dashes and spaces
	ssn = strings.ReplaceAll(ssn, "-", "")
	ssn = strings.ReplaceAll(ssn, " ", "")

	// Must be exactly 9 digits
	if len(ssn) != 9 {
		return ValidationResult{
			Valid:   false,
			Code:    "INVALID_SSN_LENGTH",
			Message: fmt.Sprintf("SSN must be 9 digits, got %d", len(ssn)),
		}
	}

	if !isNumeric(ssn) {
		return ValidationResult{
			Valid:   false,
			Code:    "INVALID_SSN_FORMAT",
			Message: "SSN must contain only digits",
		}
	}

	// Area number (first 3 digits) cannot be 000, 666, or 900-999
	area := ssn[0:3]
	if area == "000" || area == "666" {
		return ValidationResult{
			Valid:   false,
			Code:    "INVALID_SSN_AREA",
			Message: "SSN area number cannot be 000 or 666",
		}
	}
	if area[0] == '9' {
		return ValidationResult{
			Valid:   false,
			Code:    "INVALID_SSN_AREA",
			Message: "SSN area number cannot start with 9",
		}
	}

	// Group number (middle 2 digits) cannot be 00
	if ssn[3:5] == "00" {
		return ValidationResult{
			Valid:   false,
			Code:    "INVALID_SSN_GROUP",
			Message: "SSN group number cannot be 00",
		}
	}

	// Serial number (last 4 digits) cannot be 0000
	if ssn[5:9] == "0000" {
		return ValidationResult{
			Valid:   false,
			Code:    "INVALID_SSN_SERIAL",
			Message: "SSN serial number cannot be 0000",
		}
	}

	// Check reject patterns
	for _, pattern := range v.rejectPatterns {
		if ssn == pattern {
			return ValidationResult{
				Valid:   false,
				Code:    "INVALID_SSN_PATTERN",
				Message: "SSN matches invalid pattern",
			}
		}
	}

	return ValidationResult{Valid: true}
}

// Helper functions

func isNumeric(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func isAlpha(c byte) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
}

func isDigit(c byte) bool {
	return c >= '0' && c <= '9'
}

func isAlphanumeric(c byte) bool {
	return isAlpha(c) || isDigit(c)
}

// PhoneNormalizer normalizes phone numbers.
type PhoneNormalizer struct {
	stripCountryCode bool
}

// NewPhoneNormalizer creates a new phone normalizer.
func NewPhoneNormalizer(stripCountryCode bool) *PhoneNormalizer {
	return &PhoneNormalizer{stripCountryCode: stripCountryCode}
}

// Normalize normalizes a phone number to digits only.
func (n *PhoneNormalizer) Normalize(phone string) string {
	// Extract only digits
	re := regexp.MustCompile(`\d`)
	digits := strings.Join(re.FindAllString(phone, -1), "")

	// Strip US country code if present
	if n.stripCountryCode && len(digits) == 11 && digits[0] == '1' {
		digits = digits[1:]
	}

	return digits
}

// SSNNormalizer normalizes SSNs.
type SSNNormalizer struct{}

// NewSSNNormalizer creates a new SSN normalizer.
func NewSSNNormalizer() *SSNNormalizer {
	return &SSNNormalizer{}
}

// Normalize normalizes an SSN to digits only.
func (n *SSNNormalizer) Normalize(ssn string) string {
	re := regexp.MustCompile(`\d`)
	return strings.Join(re.FindAllString(ssn, -1), "")
}
