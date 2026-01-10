// Package terminology provides code system mapping for healthcare terminologies.
package terminology

import (
	"sort"
	"strings"
	"sync"
	"unicode"
)

// MatchConfidence represents the confidence level of a terminology match.
type MatchConfidence float64

const (
	// ConfidenceExact indicates an exact match (1.0)
	ConfidenceExact MatchConfidence = 1.0

	// ConfidenceHigh indicates a high-confidence match (>= 0.9)
	ConfidenceHigh MatchConfidence = 0.9

	// ConfidenceMedium indicates a medium-confidence match (>= 0.7)
	ConfidenceMedium MatchConfidence = 0.7

	// ConfidenceLow indicates a low-confidence match (>= 0.5)
	ConfidenceLow MatchConfidence = 0.5

	// ConfidenceNone indicates no meaningful match (< 0.5)
	ConfidenceNone MatchConfidence = 0.0
)

// FuzzyMatch represents a fuzzy match result with confidence score.
type FuzzyMatch struct {
	Code       string          `json:"code"`
	Display    string          `json:"display"`
	System     string          `json:"system"`
	Confidence MatchConfidence `json:"confidence"`
	MatchType  string          `json:"match_type"` // exact, synonym, fuzzy, partial
	Reason     string          `json:"reason,omitempty"`
}

// IsHighConfidence returns true if confidence >= 0.9.
func (m *FuzzyMatch) IsHighConfidence() bool {
	return m.Confidence >= ConfidenceHigh
}

// IsMediumConfidence returns true if confidence >= 0.7.
func (m *FuzzyMatch) IsMediumConfidence() bool {
	return m.Confidence >= ConfidenceMedium
}

// IsAcceptable returns true if confidence >= threshold.
func (m *FuzzyMatch) IsAcceptable(threshold MatchConfidence) bool {
	return m.Confidence >= threshold
}

// FuzzyMatcherConfig configures the fuzzy matcher behavior.
type FuzzyMatcherConfig struct {
	// MinConfidence is the minimum confidence to return a match
	MinConfidence MatchConfidence

	// MaxResults limits the number of results returned
	MaxResults int

	// EnableSynonyms enables synonym matching
	EnableSynonyms bool

	// EnableAbbreviations enables abbreviation expansion
	EnableAbbreviations bool

	// CaseSensitive enables case-sensitive matching
	CaseSensitive bool
}

// DefaultFuzzyMatcherConfig returns sensible defaults.
func DefaultFuzzyMatcherConfig() *FuzzyMatcherConfig {
	return &FuzzyMatcherConfig{
		MinConfidence:       ConfidenceLow,
		MaxResults:          10,
		EnableSynonyms:      true,
		EnableAbbreviations: true,
		CaseSensitive:       false,
	}
}

// FuzzyMatcher provides fuzzy matching for terminology codes.
type FuzzyMatcher struct {
	loincLoader   *LOINCLoader
	synonyms      map[string][]string // normalized term -> synonyms
	abbreviations map[string]string   // abbreviation -> expansion
	config        *FuzzyMatcherConfig
	mu            sync.RWMutex
}

// NewFuzzyMatcher creates a new fuzzy matcher.
func NewFuzzyMatcher(loincLoader *LOINCLoader, config *FuzzyMatcherConfig) *FuzzyMatcher {
	if config == nil {
		config = DefaultFuzzyMatcherConfig()
	}

	fm := &FuzzyMatcher{
		loincLoader:   loincLoader,
		synonyms:      make(map[string][]string),
		abbreviations: make(map[string]string),
		config:        config,
	}

	// Load default medical abbreviations
	fm.loadDefaultAbbreviations()
	fm.loadDefaultSynonyms()

	return fm
}

// Match finds the best matches for a query string.
func (fm *FuzzyMatcher) Match(query string) []FuzzyMatch {
	if query == "" {
		return nil
	}

	fm.mu.RLock()
	defer fm.mu.RUnlock()

	normalizedQuery := fm.normalize(query)
	var matches []FuzzyMatch

	// 1. Check for exact LOINC code match
	if code := fm.loincLoader.GetCode(query); code != nil {
		matches = append(matches, FuzzyMatch{
			Code:       code.Code,
			Display:    code.DisplayName(),
			System:     SystemLOINC,
			Confidence: ConfidenceExact,
			MatchType:  "exact",
			Reason:     "Exact LOINC code match",
		})
		return matches
	}

	// 2. Check common lab code mappings
	if loincCode := GetCommonLabCode(query); loincCode != "" {
		if code := fm.loincLoader.GetCode(loincCode); code != nil {
			matches = append(matches, FuzzyMatch{
				Code:       code.Code,
				Display:    code.DisplayName(),
				System:     SystemLOINC,
				Confidence: ConfidenceExact,
				MatchType:  "synonym",
				Reason:     "Common lab code alias",
			})
		}
	}

	// 3. Check common panel mappings
	if loincCode := GetCommonPanelCode(query); loincCode != "" {
		if code := fm.loincLoader.GetCode(loincCode); code != nil {
			matches = append(matches, FuzzyMatch{
				Code:       code.Code,
				Display:    code.DisplayName(),
				System:     SystemLOINC,
				Confidence: ConfidenceExact,
				MatchType:  "synonym",
				Reason:     "Common panel alias",
			})
		}
	}

	// 4. Expand abbreviations and try again
	if fm.config.EnableAbbreviations {
		expanded := fm.expandAbbreviations(normalizedQuery)
		if expanded != normalizedQuery {
			expandedMatches := fm.matchByDisplay(expanded)
			for _, m := range expandedMatches {
				m.Reason = "Abbreviation expanded: " + expanded
				matches = append(matches, m)
			}
		}
	}

	// 5. Check synonyms
	if fm.config.EnableSynonyms {
		if synonyms, ok := fm.synonyms[normalizedQuery]; ok {
			for _, syn := range synonyms {
				synMatches := fm.matchByDisplay(syn)
				for _, m := range synMatches {
					m.MatchType = "synonym"
					m.Reason = "Synonym: " + syn
					// Slightly reduce confidence for synonym matches
					m.Confidence = MatchConfidence(float64(m.Confidence) * 0.95)
					matches = append(matches, m)
				}
			}
		}
	}

	// 6. Direct display match
	displayMatches := fm.matchByDisplay(normalizedQuery)
	matches = append(matches, displayMatches...)

	// 7. Fuzzy string matching on LOINC codes
	if len(matches) == 0 || matches[0].Confidence < ConfidenceHigh {
		fuzzyMatches := fm.fuzzyMatchLOINC(normalizedQuery)
		matches = append(matches, fuzzyMatches...)
	}

	// Deduplicate and sort by confidence
	matches = fm.deduplicateMatches(matches)
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].Confidence > matches[j].Confidence
	})

	// Apply filters
	var filtered []FuzzyMatch
	for _, m := range matches {
		if m.Confidence >= fm.config.MinConfidence {
			filtered = append(filtered, m)
			if fm.config.MaxResults > 0 && len(filtered) >= fm.config.MaxResults {
				break
			}
		}
	}

	return filtered
}

// MatchWithThreshold returns the best match if it meets the threshold.
func (fm *FuzzyMatcher) MatchWithThreshold(query string, threshold MatchConfidence) *FuzzyMatch {
	matches := fm.Match(query)
	if len(matches) == 0 {
		return nil
	}
	if matches[0].Confidence >= threshold {
		return &matches[0]
	}
	return nil
}

// BestMatch returns the single best match or nil.
func (fm *FuzzyMatcher) BestMatch(query string) *FuzzyMatch {
	matches := fm.Match(query)
	if len(matches) == 0 {
		return nil
	}
	return &matches[0]
}

// matchByDisplay finds matches by display name/component.
func (fm *FuzzyMatcher) matchByDisplay(query string) []FuzzyMatch {
	var matches []FuzzyMatch

	// Use LOINC loader's lookup
	codes := fm.loincLoader.LookupByDisplay(query)
	for _, code := range codes {
		if !code.IsActive() {
			continue
		}

		// Calculate confidence based on match quality
		confidence := fm.calculateDisplayConfidence(query, code)
		if confidence >= fm.config.MinConfidence {
			matches = append(matches, FuzzyMatch{
				Code:       code.Code,
				Display:    code.DisplayName(),
				System:     SystemLOINC,
				Confidence: confidence,
				MatchType:  "display",
				Reason:     "Display name match",
			})
		}
	}

	return matches
}

// fuzzyMatchLOINC performs fuzzy string matching against LOINC codes.
func (fm *FuzzyMatcher) fuzzyMatchLOINC(query string) []FuzzyMatch {
	var matches []FuzzyMatch

	// Search LOINC codes
	codes := fm.loincLoader.SearchCodes(query, 50)
	for _, code := range codes {
		confidence := fm.calculateFuzzyConfidence(query, code)
		if confidence >= fm.config.MinConfidence {
			matches = append(matches, FuzzyMatch{
				Code:       code.Code,
				Display:    code.DisplayName(),
				System:     SystemLOINC,
				Confidence: confidence,
				MatchType:  "fuzzy",
				Reason:     "Fuzzy string match",
			})
		}
	}

	return matches
}

// calculateDisplayConfidence calculates confidence for a display match.
func (fm *FuzzyMatcher) calculateDisplayConfidence(query string, code *LOINCCode) MatchConfidence {
	queryUpper := strings.ToUpper(query)

	// Exact component match
	if strings.ToUpper(code.Component) == queryUpper {
		return 0.98
	}

	// Exact short name match
	if strings.ToUpper(code.ShortName) == queryUpper {
		return 0.97
	}

	// Exact consumer name match
	if strings.ToUpper(code.Consumer) == queryUpper {
		return 0.96
	}

	// Contains in long name
	if strings.Contains(strings.ToUpper(code.LongName), queryUpper) {
		// Score based on how much of the long name is matched
		ratio := float64(len(query)) / float64(len(code.LongName))
		return MatchConfidence(0.7 + (ratio * 0.2))
	}

	// Token overlap
	return fm.tokenOverlapScore(query, code.LongName+" "+code.Component)
}

// calculateFuzzyConfidence calculates confidence for a fuzzy match.
func (fm *FuzzyMatcher) calculateFuzzyConfidence(query string, code *LOINCCode) MatchConfidence {
	// Combine multiple signals
	var scores []float64

	// String similarity to component
	scores = append(scores, float64(fm.stringSimilarity(query, code.Component)))

	// String similarity to short name
	if code.ShortName != "" {
		scores = append(scores, float64(fm.stringSimilarity(query, code.ShortName)))
	}

	// Token overlap with long name
	scores = append(scores, float64(fm.tokenOverlapScore(query, code.LongName)))

	// Take the best score
	var best float64
	for _, s := range scores {
		if s > best {
			best = s
		}
	}

	return MatchConfidence(best)
}

// stringSimilarity calculates similarity between two strings (0-1).
func (fm *FuzzyMatcher) stringSimilarity(a, b string) MatchConfidence {
	if !fm.config.CaseSensitive {
		a = strings.ToUpper(a)
		b = strings.ToUpper(b)
	}

	if a == b {
		return 1.0
	}

	// Levenshtein-based similarity
	distance := levenshteinDistance(a, b)
	maxLen := len(a)
	if len(b) > maxLen {
		maxLen = len(b)
	}
	if maxLen == 0 {
		return 1.0
	}

	similarity := 1.0 - (float64(distance) / float64(maxLen))
	return MatchConfidence(similarity)
}

// tokenOverlapScore calculates overlap between query tokens and target tokens.
func (fm *FuzzyMatcher) tokenOverlapScore(query, target string) MatchConfidence {
	queryTokens := tokenize(query)
	targetTokens := tokenize(target)

	if len(queryTokens) == 0 || len(targetTokens) == 0 {
		return 0
	}

	// Count matching tokens
	matches := 0
	targetSet := make(map[string]bool)
	for _, t := range targetTokens {
		targetSet[strings.ToUpper(t)] = true
	}

	for _, qt := range queryTokens {
		qtUpper := strings.ToUpper(qt)
		if targetSet[qtUpper] {
			matches++
		} else {
			// Check for partial matches
			for tt := range targetSet {
				if strings.Contains(tt, qtUpper) || strings.Contains(qtUpper, tt) {
					matches++
					break
				}
			}
		}
	}

	// Jaccard-like score
	score := float64(matches) / float64(len(queryTokens))
	return MatchConfidence(score * 0.9) // Cap at 0.9 for token matches
}

// normalize normalizes a string for matching.
func (fm *FuzzyMatcher) normalize(s string) string {
	if !fm.config.CaseSensitive {
		s = strings.ToUpper(s)
	}
	// Remove extra whitespace
	s = strings.Join(strings.Fields(s), " ")
	return s
}

// expandAbbreviations expands known abbreviations in a string.
func (fm *FuzzyMatcher) expandAbbreviations(s string) string {
	words := strings.Fields(s)
	for i, word := range words {
		wordUpper := strings.ToUpper(word)
		if expansion, ok := fm.abbreviations[wordUpper]; ok {
			words[i] = expansion
		}
	}
	return strings.Join(words, " ")
}

// deduplicateMatches removes duplicate codes, keeping highest confidence.
func (fm *FuzzyMatcher) deduplicateMatches(matches []FuzzyMatch) []FuzzyMatch {
	seen := make(map[string]int) // code -> index in result
	var result []FuzzyMatch

	for _, m := range matches {
		if idx, ok := seen[m.Code]; ok {
			// Keep higher confidence
			if m.Confidence > result[idx].Confidence {
				result[idx] = m
			}
		} else {
			seen[m.Code] = len(result)
			result = append(result, m)
		}
	}

	return result
}

// AddSynonym adds a synonym mapping.
func (fm *FuzzyMatcher) AddSynonym(term string, synonyms ...string) {
	fm.mu.Lock()
	defer fm.mu.Unlock()

	termUpper := strings.ToUpper(term)
	fm.synonyms[termUpper] = append(fm.synonyms[termUpper], synonyms...)

	// Also add reverse mappings
	for _, syn := range synonyms {
		synUpper := strings.ToUpper(syn)
		fm.synonyms[synUpper] = append(fm.synonyms[synUpper], term)
	}
}

// AddAbbreviation adds an abbreviation expansion.
func (fm *FuzzyMatcher) AddAbbreviation(abbrev, expansion string) {
	fm.mu.Lock()
	defer fm.mu.Unlock()
	fm.abbreviations[strings.ToUpper(abbrev)] = strings.ToUpper(expansion)
}

// loadDefaultAbbreviations loads common medical abbreviations.
func (fm *FuzzyMatcher) loadDefaultAbbreviations() {
	abbrevs := map[string]string{
		"WBC":   "WHITE BLOOD CELLS",
		"RBC":   "RED BLOOD CELLS",
		"HGB":   "HEMOGLOBIN",
		"HCT":   "HEMATOCRIT",
		"PLT":   "PLATELETS",
		"MCV":   "MEAN CORPUSCULAR VOLUME",
		"MCH":   "MEAN CORPUSCULAR HEMOGLOBIN",
		"MCHC":  "MEAN CORPUSCULAR HEMOGLOBIN CONCENTRATION",
		"GLU":   "GLUCOSE",
		"BUN":   "BLOOD UREA NITROGEN",
		"CREAT": "CREATININE",
		"NA":    "SODIUM",
		"K":     "POTASSIUM",
		"CL":    "CHLORIDE",
		"CO2":   "CARBON DIOXIDE",
		"CA":    "CALCIUM",
		"MG":    "MAGNESIUM",
		"PHOS":  "PHOSPHORUS",
		"ALT":   "ALANINE AMINOTRANSFERASE",
		"AST":   "ASPARTATE AMINOTRANSFERASE",
		"ALP":   "ALKALINE PHOSPHATASE",
		"TBIL":  "TOTAL BILIRUBIN",
		"ALB":   "ALBUMIN",
		"TP":    "TOTAL PROTEIN",
		"CHOL":  "CHOLESTEROL",
		"TRIG":  "TRIGLYCERIDES",
		"HDL":   "HIGH DENSITY LIPOPROTEIN",
		"LDL":   "LOW DENSITY LIPOPROTEIN",
		"TSH":   "THYROID STIMULATING HORMONE",
		"T3":    "TRIIODOTHYRONINE",
		"T4":    "THYROXINE",
		"HBA1C": "HEMOGLOBIN A1C",
		"A1C":   "HEMOGLOBIN A1C",
		"PT":    "PROTHROMBIN TIME",
		"INR":   "INTERNATIONAL NORMALIZED RATIO",
		"PTT":   "PARTIAL THROMBOPLASTIN TIME",
		"APTT":  "ACTIVATED PARTIAL THROMBOPLASTIN TIME",
		"CBC":   "COMPLETE BLOOD COUNT",
		"BMP":   "BASIC METABOLIC PANEL",
		"CMP":   "COMPREHENSIVE METABOLIC PANEL",
		"LFT":   "LIVER FUNCTION TESTS",
		"UA":    "URINALYSIS",
		"ABG":   "ARTERIAL BLOOD GAS",
		"ESR":   "ERYTHROCYTE SEDIMENTATION RATE",
		"CRP":   "C-REACTIVE PROTEIN",
		"PSA":   "PROSTATE SPECIFIC ANTIGEN",
		"BNP":   "BRAIN NATRIURETIC PEPTIDE",
		"TROP":  "TROPONIN",
	}

	for abbrev, expansion := range abbrevs {
		fm.abbreviations[abbrev] = expansion
	}
}

// loadDefaultSynonyms loads common medical synonyms.
func (fm *FuzzyMatcher) loadDefaultSynonyms() {
	synonymGroups := [][]string{
		{"GLUCOSE", "BLOOD SUGAR", "BS", "FBS", "FASTING GLUCOSE"},
		{"WHITE BLOOD CELLS", "LEUKOCYTES", "WBC", "WHITE COUNT"},
		{"RED BLOOD CELLS", "ERYTHROCYTES", "RBC", "RED COUNT"},
		{"HEMOGLOBIN", "HGB", "HB"},
		{"PLATELETS", "PLT", "THROMBOCYTES"},
		{"CREATININE", "CREAT", "CR"},
		{"BLOOD UREA NITROGEN", "BUN", "UREA"},
		{"POTASSIUM", "K+", "K"},
		{"SODIUM", "NA+", "NA"},
		{"CHOLESTEROL", "TOTAL CHOLESTEROL", "TC", "CHOL"},
		{"TRIGLYCERIDES", "TG", "TRIG"},
		{"HEMOGLOBIN A1C", "HBA1C", "A1C", "GLYCATED HEMOGLOBIN"},
		{"THYROID STIMULATING HORMONE", "TSH", "THYROTROPIN"},
		{"ALANINE AMINOTRANSFERASE", "ALT", "SGPT"},
		{"ASPARTATE AMINOTRANSFERASE", "AST", "SGOT"},
	}

	for _, group := range synonymGroups {
		for i, term := range group {
			termUpper := strings.ToUpper(term)
			for j, syn := range group {
				if i != j {
					fm.synonyms[termUpper] = append(fm.synonyms[termUpper], strings.ToUpper(syn))
				}
			}
		}
	}
}

// =============================================================================
// Helper Functions
// =============================================================================

// levenshteinDistance calculates the Levenshtein distance between two strings.
func levenshteinDistance(a, b string) int {
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}

	// Create matrix
	matrix := make([][]int, len(a)+1)
	for i := range matrix {
		matrix[i] = make([]int, len(b)+1)
		matrix[i][0] = i
	}
	for j := range matrix[0] {
		matrix[0][j] = j
	}

	// Fill matrix
	for i := 1; i <= len(a); i++ {
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			matrix[i][j] = min3(
				matrix[i-1][j]+1,      // deletion
				matrix[i][j-1]+1,      // insertion
				matrix[i-1][j-1]+cost, // substitution
			)
		}
	}

	return matrix[len(a)][len(b)]
}

// min3 returns the minimum of three integers.
func min3(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}

// tokenize splits a string into tokens (words).
func tokenize(s string) []string {
	var tokens []string
	var current strings.Builder

	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			current.WriteRune(r)
		} else if current.Len() > 0 {
			tokens = append(tokens, current.String())
			current.Reset()
		}
	}

	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}

	return tokens
}
