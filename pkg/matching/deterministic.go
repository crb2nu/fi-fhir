package matching

import (
	"time"
)

// MatchResult represents the outcome of a match attempt.
type MatchResult string

const (
	// MatchConfirmed indicates a definite match.
	MatchConfirmed MatchResult = "confirmed"
	// MatchProbable indicates a likely match requiring review.
	MatchProbable MatchResult = "probable"
	// MatchPossible indicates a potential match with low confidence.
	MatchPossible MatchResult = "possible"
	// MatchNoMatch indicates records are definitely different.
	MatchNoMatch MatchResult = "no_match"
)

// DeterministicRule defines a rule for exact matching.
type DeterministicRule struct {
	// Name identifies this rule for logging/debugging.
	Name string

	// Fields are the identifiers that must match exactly.
	// All fields must be present and match for the rule to fire.
	Fields []string

	// Result is the match outcome when this rule fires.
	Result MatchResult

	// Priority determines which rule takes precedence (higher = earlier).
	Priority int
}

// Patient represents patient demographics for matching.
type Patient struct {
	// Identifiers
	MRN        string // Medical Record Number
	MRNSystem  string // Facility/system that issued the MRN
	SSN        string // Social Security Number (normalized, 9 digits)
	MBI        string // Medicare Beneficiary Identifier
	MedicaidID string // State Medicaid ID

	// Demographics
	FamilyName string // Last name (normalized)
	GivenName  string // First name (normalized)
	MiddleName string // Middle name/initial (normalized)
	DOB        time.Time
	Gender     string // M, F, U, O

	// Contact
	Phone   string // Normalized phone (10 digits)
	Email   string
	Address Address
}

// Address represents a patient address.
type Address struct {
	Line1      string
	Line2      string
	City       string
	State      string
	PostalCode string
}

// DeterministicMatcher applies exact-match rules.
type DeterministicMatcher struct {
	rules []DeterministicRule
}

// NewDeterministicMatcher creates a matcher with default healthcare rules.
func NewDeterministicMatcher() *DeterministicMatcher {
	return &DeterministicMatcher{
		rules: defaultDeterministicRules(),
	}
}

// NewDeterministicMatcherWithRules creates a matcher with custom rules.
func NewDeterministicMatcherWithRules(rules []DeterministicRule) *DeterministicMatcher {
	// Sort by priority (higher first)
	sorted := make([]DeterministicRule, len(rules))
	copy(sorted, rules)
	for i := range sorted {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j].Priority > sorted[i].Priority {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}
	return &DeterministicMatcher{rules: sorted}
}

// defaultDeterministicRules returns standard healthcare matching rules.
func defaultDeterministicRules() []DeterministicRule {
	return []DeterministicRule{
		// Rule 1: SSN alone confirms match (highest confidence)
		{
			Name:     "ssn_exact",
			Fields:   []string{"ssn"},
			Result:   MatchConfirmed,
			Priority: 100,
		},
		// Rule 2: MBI confirms match (Medicare identifier)
		{
			Name:     "mbi_exact",
			Fields:   []string{"mbi"},
			Result:   MatchConfirmed,
			Priority: 95,
		},
		// Rule 3: MRN + System confirms match (same facility)
		{
			Name:     "mrn_system",
			Fields:   []string{"mrn", "mrn_system"},
			Result:   MatchConfirmed,
			Priority: 90,
		},
		// Rule 4: Full demographics match is high confidence
		{
			Name:     "full_demographics",
			Fields:   []string{"family_name", "given_name", "dob", "gender"},
			Result:   MatchProbable,
			Priority: 70,
		},
		// Rule 5: Name + DOB + Phone is probable
		{
			Name:     "name_dob_phone",
			Fields:   []string{"family_name", "given_name", "dob", "phone"},
			Result:   MatchProbable,
			Priority: 65,
		},
		// Rule 6: Name + DOB is possible (common scenario)
		{
			Name:     "name_dob",
			Fields:   []string{"family_name", "given_name", "dob"},
			Result:   MatchPossible,
			Priority: 50,
		},
	}
}

// Match attempts to find a deterministic match between two patients.
// Returns the result and the rule that matched (if any).
func (m *DeterministicMatcher) Match(a, b *Patient) (MatchResult, string) {
	for _, rule := range m.rules {
		if m.ruleMatches(rule, a, b) {
			return rule.Result, rule.Name
		}
	}
	return MatchNoMatch, ""
}

// ruleMatches checks if a rule's fields all match between two patients.
func (m *DeterministicMatcher) ruleMatches(rule DeterministicRule, a, b *Patient) bool {
	for _, field := range rule.Fields {
		if !m.fieldMatches(field, a, b) {
			return false
		}
	}
	return true
}

// fieldMatches checks if a specific field matches between two patients.
func (m *DeterministicMatcher) fieldMatches(field string, a, b *Patient) bool {
	switch field {
	case "ssn":
		return a.SSN != "" && b.SSN != "" && a.SSN == b.SSN
	case "mbi":
		return a.MBI != "" && b.MBI != "" && a.MBI == b.MBI
	case "mrn":
		return a.MRN != "" && b.MRN != "" && a.MRN == b.MRN
	case "mrn_system":
		return a.MRNSystem != "" && b.MRNSystem != "" && a.MRNSystem == b.MRNSystem
	case "medicaid_id":
		return a.MedicaidID != "" && b.MedicaidID != "" && a.MedicaidID == b.MedicaidID
	case "family_name":
		na := NormalizeName(a.FamilyName)
		nb := NormalizeName(b.FamilyName)
		return na != "" && nb != "" && na == nb
	case "given_name":
		na := NormalizeName(a.GivenName)
		nb := NormalizeName(b.GivenName)
		return na != "" && nb != "" && na == nb
	case "dob":
		return !a.DOB.IsZero() && !b.DOB.IsZero() && a.DOB.Equal(b.DOB)
	case "gender":
		return a.Gender != "" && b.Gender != "" && a.Gender == b.Gender
	case "phone":
		pa := NormalizePhone(a.Phone)
		pb := NormalizePhone(b.Phone)
		return pa != "" && pb != "" && pa == pb
	case "email":
		return a.Email != "" && b.Email != "" && a.Email == b.Email
	case "postal_code":
		return a.Address.PostalCode != "" && b.Address.PostalCode != "" &&
			a.Address.PostalCode == b.Address.PostalCode
	default:
		return false
	}
}

// BlockingKey generates a blocking key for efficient candidate selection.
// Records with different blocking keys cannot match.
// Uses DOB + first letter of last name as a simple blocking strategy.
func BlockingKey(p *Patient) string {
	if p.DOB.IsZero() {
		return ""
	}

	key := p.DOB.Format("20060102")

	familyNorm := NormalizeName(p.FamilyName)
	if len(familyNorm) > 0 {
		key += "_" + string(familyNorm[0])
	}

	return key
}

// SoundexBlockingKey generates a blocking key using Soundex.
// Useful for catching phonetic variations.
func SoundexBlockingKey(p *Patient) string {
	if p.DOB.IsZero() {
		return ""
	}

	key := p.DOB.Format("20060102")

	familySoundex := Soundex(p.FamilyName)
	if familySoundex != "" {
		key += "_" + familySoundex
	}

	return key
}
