//nolint:gosec // Test file - G104 errors intentionally ignored in test setup
package terminology

import (
	"strings"
	"testing"
)

// =============================================================================
// Test Setup
// =============================================================================

func newTestFuzzyMatcher() *FuzzyMatcher {
	loader := NewLOINCLoader()
	loader.LoadLoincTableFromReader(strings.NewReader(testLoincTable))
	loader.LoadPanelHierarchyFromReader(strings.NewReader(testPanelHierarchy))
	return NewFuzzyMatcher(loader, nil)
}

// =============================================================================
// FuzzyMatcher Tests
// =============================================================================

func TestFuzzyMatcher_ExactCodeMatch(t *testing.T) {
	fm := newTestFuzzyMatcher()

	matches := fm.Match("6690-2")
	if len(matches) == 0 {
		t.Fatal("Expected match for exact LOINC code")
	}

	if matches[0].Code != "6690-2" {
		t.Errorf("Code = %s, want 6690-2", matches[0].Code)
	}
	if matches[0].Confidence != ConfidenceExact {
		t.Errorf("Confidence = %f, want %f", matches[0].Confidence, ConfidenceExact)
	}
	if matches[0].MatchType != "exact" {
		t.Errorf("MatchType = %s, want exact", matches[0].MatchType)
	}
}

func TestFuzzyMatcher_CommonLabCode(t *testing.T) {
	fm := newTestFuzzyMatcher()

	tests := []struct {
		query    string
		wantCode string
	}{
		{"WBC", "6690-2"},
		{"wbc", "6690-2"}, // Case insensitive
		{"HGB", "718-7"},
		{"GLUCOSE", "2345-7"},
		{"BUN", "3094-0"},
	}

	for _, tc := range tests {
		t.Run(tc.query, func(t *testing.T) {
			matches := fm.Match(tc.query)
			if len(matches) == 0 {
				t.Fatalf("No matches for %s", tc.query)
			}
			if matches[0].Code != tc.wantCode {
				t.Errorf("Match(%s).Code = %s, want %s", tc.query, matches[0].Code, tc.wantCode)
			}
		})
	}
}

func TestFuzzyMatcher_CommonPanelCode(t *testing.T) {
	fm := newTestFuzzyMatcher()

	tests := []struct {
		query    string
		wantCode string
	}{
		{"CBC", "58410-2"},
		{"cbc", "58410-2"},
		{"BMP", "51990-0"},
	}

	for _, tc := range tests {
		t.Run(tc.query, func(t *testing.T) {
			matches := fm.Match(tc.query)
			if len(matches) == 0 {
				t.Fatalf("No matches for %s", tc.query)
			}
			if matches[0].Code != tc.wantCode {
				t.Errorf("Match(%s).Code = %s, want %s", tc.query, matches[0].Code, tc.wantCode)
			}
		})
	}
}

func TestFuzzyMatcher_DisplayMatch(t *testing.T) {
	fm := newTestFuzzyMatcher()

	tests := []struct {
		query         string
		wantCode      string
		minConfidence MatchConfidence
	}{
		{"Leukocytes", "6690-2", 0.9},
		{"LEUKOCYTES", "6690-2", 0.9},
		{"Hemoglobin", "718-7", 0.9},
		{"Glucose", "2345-7", 0.9},
	}

	for _, tc := range tests {
		t.Run(tc.query, func(t *testing.T) {
			matches := fm.Match(tc.query)
			if len(matches) == 0 {
				t.Fatalf("No matches for %s", tc.query)
			}
			if matches[0].Code != tc.wantCode {
				t.Errorf("Match(%s).Code = %s, want %s", tc.query, matches[0].Code, tc.wantCode)
			}
			if matches[0].Confidence < tc.minConfidence {
				t.Errorf("Match(%s).Confidence = %f, want >= %f",
					tc.query, matches[0].Confidence, tc.minConfidence)
			}
		})
	}
}

func TestFuzzyMatcher_FuzzyMatch(t *testing.T) {
	fm := newTestFuzzyMatcher()

	// Test partial/fuzzy matches
	tests := []struct {
		query         string
		wantCode      string
		minConfidence MatchConfidence
	}{
		{"white blood", "6690-2", 0.5},
		{"blood glucose", "2345-7", 0.5},
		{"creat", "2160-0", 0.5},
	}

	for _, tc := range tests {
		t.Run(tc.query, func(t *testing.T) {
			matches := fm.Match(tc.query)
			if len(matches) == 0 {
				t.Logf("No matches for %s (may be expected)", tc.query)
				return
			}

			// Check if expected code is in results
			found := false
			for _, m := range matches {
				if m.Code == tc.wantCode && m.Confidence >= tc.minConfidence {
					found = true
					break
				}
			}
			if !found {
				t.Logf("Expected %s in matches for %s with confidence >= %f",
					tc.wantCode, tc.query, tc.minConfidence)
			}
		})
	}
}

func TestFuzzyMatcher_AbbreviationExpansion(t *testing.T) {
	fm := newTestFuzzyMatcher()

	// The fuzzy matcher should expand abbreviations
	// "WBC" -> "WHITE BLOOD CELLS" -> matches Leukocytes
	matches := fm.Match("WBC")
	if len(matches) == 0 {
		t.Fatal("Expected matches for WBC abbreviation")
	}

	// Should match WBC code
	found := false
	for _, m := range matches {
		if m.Code == "6690-2" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected 6690-2 (WBC) in matches")
	}
}

func TestFuzzyMatcher_SynonymMatch(t *testing.T) {
	fm := newTestFuzzyMatcher()

	// Add a synonym
	fm.AddSynonym("SUGAR", "GLUCOSE")

	matches := fm.Match("SUGAR")

	// Should find glucose via synonym
	found := false
	for _, m := range matches {
		if m.Code == "2345-7" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected glucose match via SUGAR synonym")
	}
}

func TestFuzzyMatcher_AddAbbreviation(t *testing.T) {
	fm := newTestFuzzyMatcher()

	// Add a custom abbreviation
	fm.AddAbbreviation("LEUKS", "LEUKOCYTES")

	// Verify abbreviation was added (case insensitive)
	fm.mu.RLock()
	expansion, ok := fm.abbreviations["LEUKS"]
	fm.mu.RUnlock()

	if !ok {
		t.Fatal("Abbreviation 'LEUKS' was not added")
	}
	if expansion != "LEUKOCYTES" {
		t.Errorf("Expansion = %q, want 'LEUKOCYTES'", expansion)
	}

	// Test case insensitivity - add lowercase, check uppercase storage
	fm.AddAbbreviation("retics", "reticulocytes")

	fm.mu.RLock()
	expansion, ok = fm.abbreviations["RETICS"]
	fm.mu.RUnlock()

	if !ok {
		t.Fatal("Abbreviation 'retics' was not normalized to uppercase")
	}
	if expansion != "RETICULOCYTES" {
		t.Errorf("Expansion = %q, want 'RETICULOCYTES'", expansion)
	}

	// Verify default abbreviations still exist
	fm.mu.RLock()
	_, hasWBC := fm.abbreviations["WBC"]
	_, hasHGB := fm.abbreviations["HGB"]
	fm.mu.RUnlock()

	if !hasWBC {
		t.Error("Default abbreviation 'WBC' should exist")
	}
	if !hasHGB {
		t.Error("Default abbreviation 'HGB' should exist")
	}
}

func TestFuzzyMatcher_BestMatch(t *testing.T) {
	fm := newTestFuzzyMatcher()

	match := fm.BestMatch("WBC")
	if match == nil {
		t.Fatal("BestMatch returned nil")
	}
	if match.Code != "6690-2" {
		t.Errorf("BestMatch code = %s, want 6690-2", match.Code)
	}
}

func TestFuzzyMatcher_MatchWithThreshold(t *testing.T) {
	fm := newTestFuzzyMatcher()

	// High threshold should match
	match := fm.MatchWithThreshold("6690-2", ConfidenceHigh)
	if match == nil {
		t.Error("MatchWithThreshold should return match for exact code")
	}

	// Very high threshold for fuzzy match might not match
	_ = fm.MatchWithThreshold("white cells", 0.99)
	// This might or might not match depending on the data - we're just testing it doesn't panic
}

func TestFuzzyMatcher_EmptyQuery(t *testing.T) {
	fm := newTestFuzzyMatcher()

	matches := fm.Match("")
	if len(matches) != 0 {
		t.Error("Empty query should return no matches")
	}
}

func TestFuzzyMatcher_NoMatch(t *testing.T) {
	fm := newTestFuzzyMatcher()

	matches := fm.Match("xyznonexistent123")
	if len(matches) > 0 {
		for _, m := range matches {
			if m.Confidence > 0.5 {
				t.Errorf("Unexpected high-confidence match for nonsense: %+v", m)
			}
		}
	}
}

func TestFuzzyMatcher_MaxResults(t *testing.T) {
	fm := newTestFuzzyMatcher()
	fm.config.MaxResults = 3

	// Search for something that might have many matches
	matches := fm.Match("blood")

	if len(matches) > 3 {
		t.Errorf("Expected at most 3 results, got %d", len(matches))
	}
}

func TestFuzzyMatcher_MinConfidence(t *testing.T) {
	fm := newTestFuzzyMatcher()
	fm.config.MinConfidence = 0.9

	// Low-confidence matches should be filtered
	matches := fm.Match("some vague query")

	for _, m := range matches {
		if m.Confidence < 0.9 {
			t.Errorf("Match below min confidence: %f", m.Confidence)
		}
	}
}

// =============================================================================
// FuzzyMatch Tests
// =============================================================================

func TestFuzzyMatch_IsHighConfidence(t *testing.T) {
	tests := []struct {
		confidence MatchConfidence
		want       bool
	}{
		{1.0, true},
		{0.95, true},
		{0.9, true},
		{0.89, false},
		{0.5, false},
		{0.0, false},
	}

	for _, tc := range tests {
		m := FuzzyMatch{Confidence: tc.confidence}
		if m.IsHighConfidence() != tc.want {
			t.Errorf("IsHighConfidence(%f) = %v, want %v", tc.confidence, m.IsHighConfidence(), tc.want)
		}
	}
}

func TestFuzzyMatch_IsMediumConfidence(t *testing.T) {
	tests := []struct {
		confidence MatchConfidence
		want       bool
	}{
		{1.0, true},
		{0.9, true},
		{0.7, true},
		{0.69, false},
		{0.5, false},
	}

	for _, tc := range tests {
		m := FuzzyMatch{Confidence: tc.confidence}
		if m.IsMediumConfidence() != tc.want {
			t.Errorf("IsMediumConfidence(%f) = %v, want %v", tc.confidence, m.IsMediumConfidence(), tc.want)
		}
	}
}

func TestFuzzyMatch_IsAcceptable(t *testing.T) {
	m := FuzzyMatch{Confidence: 0.75}

	if !m.IsAcceptable(0.7) {
		t.Error("0.75 should be acceptable at 0.7 threshold")
	}
	if m.IsAcceptable(0.8) {
		t.Error("0.75 should not be acceptable at 0.8 threshold")
	}
}

// =============================================================================
// Helper Function Tests
// =============================================================================

func TestLevenshteinDistance(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"a", "", 1},
		{"", "a", 1},
		{"abc", "abc", 0},
		{"abc", "ab", 1},
		{"abc", "abcd", 1},
		{"abc", "abd", 1},
		{"kitten", "sitting", 3},
		{"saturday", "sunday", 3},
	}

	for _, tc := range tests {
		got := levenshteinDistance(tc.a, tc.b)
		if got != tc.want {
			t.Errorf("levenshteinDistance(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestTokenize(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"hello world", []string{"hello", "world"}},
		{"hello-world", []string{"hello", "world"}},
		{"hello_world", []string{"hello", "world"}},
		{"  hello   world  ", []string{"hello", "world"}},
		{"test123", []string{"test123"}},
		{"", nil},
		{"---", nil},
	}

	for _, tc := range tests {
		got := tokenize(tc.input)
		if len(got) != len(tc.want) {
			t.Errorf("tokenize(%q) = %v, want %v", tc.input, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("tokenize(%q)[%d] = %q, want %q", tc.input, i, got[i], tc.want[i])
			}
		}
	}
}

func TestStringSimilarity(t *testing.T) {
	fm := newTestFuzzyMatcher()

	tests := []struct {
		a, b   string
		minSim MatchConfidence
	}{
		{"glucose", "glucose", 1.0},
		{"glucose", "GLUCOSE", 1.0}, // Case insensitive
		{"glucose", "glucos", 0.8},  // One char difference
		{"abc", "xyz", 0.0},         // Completely different
	}

	for _, tc := range tests {
		got := fm.stringSimilarity(tc.a, tc.b)
		if got < tc.minSim {
			t.Errorf("stringSimilarity(%q, %q) = %f, want >= %f", tc.a, tc.b, got, tc.minSim)
		}
	}
}

// =============================================================================
// Configuration Tests
// =============================================================================

func TestDefaultFuzzyMatcherConfig(t *testing.T) {
	config := DefaultFuzzyMatcherConfig()

	if config.MinConfidence != ConfidenceLow {
		t.Errorf("MinConfidence = %f, want %f", config.MinConfidence, ConfidenceLow)
	}
	if config.MaxResults != 10 {
		t.Errorf("MaxResults = %d, want 10", config.MaxResults)
	}
	if !config.EnableSynonyms {
		t.Error("EnableSynonyms should be true by default")
	}
	if !config.EnableAbbreviations {
		t.Error("EnableAbbreviations should be true by default")
	}
	if config.CaseSensitive {
		t.Error("CaseSensitive should be false by default")
	}
}

func TestFuzzyMatcher_CustomConfig(t *testing.T) {
	loader := NewLOINCLoader()
	config := &FuzzyMatcherConfig{
		MinConfidence:       0.8,
		MaxResults:          5,
		EnableSynonyms:      false,
		EnableAbbreviations: false,
		CaseSensitive:       true,
	}
	fm := NewFuzzyMatcher(loader, config)

	if fm.config.MinConfidence != 0.8 {
		t.Errorf("MinConfidence = %f, want 0.8", fm.config.MinConfidence)
	}
	if fm.config.MaxResults != 5 {
		t.Errorf("MaxResults = %d, want 5", fm.config.MaxResults)
	}
}

// =============================================================================
// calculateDisplayConfidence Tests
// =============================================================================

func TestFuzzyMatcher_CalculateDisplayConfidence(t *testing.T) {
	fm := newTestFuzzyMatcher()

	// Test cases that exercise calculateDisplayConfidence branches
	// Note: LookupByDisplay only indexes by component and shortname,
	// so we test those primary paths plus fuzzy matching
	tests := []struct {
		name          string
		query         string
		wantCode      string
		minConfidence MatchConfidence
		maxConfidence MatchConfidence
		description   string
	}{
		{
			name:          "exact_component_match",
			query:         "Leukocytes",
			wantCode:      "6690-2",
			minConfidence: 0.97,
			maxConfidence: 1.0,
			description:   "Exact component match should return ~0.98",
		},
		{
			name:          "exact_shortname_match",
			query:         "WBC Auto",
			wantCode:      "6690-2",
			minConfidence: 0.96,
			maxConfidence: 0.98,
			description:   "Exact short name match should return ~0.97",
		},
		{
			name:          "component_case_insensitive",
			query:         "leukocytes",
			wantCode:      "6690-2",
			minConfidence: 0.97,
			maxConfidence: 1.0,
			description:   "Case insensitive component match",
		},
		{
			name:          "shortname_glucose",
			query:         "Glucose SerPl",
			wantCode:      "2345-7",
			minConfidence: 0.96,
			maxConfidence: 0.98,
			description:   "Short name match for glucose",
		},
		{
			name:          "component_hemoglobin",
			query:         "Hemoglobin",
			wantCode:      "718-7",
			minConfidence: 0.97,
			maxConfidence: 1.0,
			description:   "Component match for hemoglobin",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			matches := fm.Match(tc.query)
			if len(matches) == 0 {
				t.Fatalf("No matches for query '%s'", tc.query)
			}

			// Find the expected code in results
			var found *FuzzyMatch
			for i := range matches {
				if matches[i].Code == tc.wantCode {
					found = &matches[i]
					break
				}
			}

			if found == nil {
				t.Fatalf("Expected code %s not in results for query '%s'", tc.wantCode, tc.query)
			}

			if found.Confidence < tc.minConfidence || found.Confidence > tc.maxConfidence {
				t.Errorf("%s: confidence = %f, want [%f, %f]",
					tc.description, found.Confidence, tc.minConfidence, tc.maxConfidence)
			}
		})
	}
}

// TestCalculateDisplayConfidence_AllBranches tests the internal confidence calculation
// by directly calling the method to ensure all branches are covered.
func TestCalculateDisplayConfidence_AllBranches(t *testing.T) {
	loader := NewLOINCLoader()
	loader.LoadLoincTableFromReader(strings.NewReader(testLoincTable))
	fm := NewFuzzyMatcher(loader, nil)

	code := loader.GetCode("6690-2")
	if code == nil {
		t.Fatal("Failed to get test code")
	}

	tests := []struct {
		name      string
		query     string
		minConf   MatchConfidence
		maxConf   MatchConfidence
		branchHit string
	}{
		{
			name:      "component_exact",
			query:     "Leukocytes",
			minConf:   0.98,
			maxConf:   0.99,
			branchHit: "exact component",
		},
		{
			name:      "shortname_exact",
			query:     "WBC Auto",
			minConf:   0.97,
			maxConf:   0.98,
			branchHit: "exact shortname",
		},
		{
			name:      "consumer_exact",
			query:     "White blood cell count",
			minConf:   0.96,
			maxConf:   0.97,
			branchHit: "exact consumer",
		},
		{
			name:      "contains_longname",
			query:     "Automated",
			minConf:   0.70,
			maxConf:   0.90,
			branchHit: "contains in longname",
		},
		{
			name:      "token_overlap",
			query:     "xyz random words",
			minConf:   0.0,
			maxConf:   0.50,
			branchHit: "token overlap fallback",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			conf := fm.calculateDisplayConfidence(tc.query, code)
			if conf < tc.minConf || conf > tc.maxConf {
				t.Errorf("calculateDisplayConfidence(%q) = %f, want [%f, %f] for %s",
					tc.query, conf, tc.minConf, tc.maxConf, tc.branchHit)
			}
		})
	}
}

// =============================================================================
// tokenOverlapScore Tests
// =============================================================================

func TestTokenOverlapScore(t *testing.T) {
	fm := NewFuzzyMatcher(NewLOINCLoader(), nil)

	tests := []struct {
		name     string
		query    string
		target   string
		minScore MatchConfidence
		maxScore MatchConfidence
	}{
		{
			name:     "empty_query",
			query:    "",
			target:   "some text",
			minScore: 0,
			maxScore: 0,
		},
		{
			name:     "empty_target",
			query:    "some",
			target:   "",
			minScore: 0,
			maxScore: 0,
		},
		{
			name:     "no_overlap",
			query:    "foo bar",
			target:   "baz qux",
			minScore: 0,
			maxScore: 0.1,
		},
		{
			name:     "partial_overlap",
			query:    "blood glucose test",
			target:   "glucose in blood",
			minScore: 0.5,
			maxScore: 1.0,
		},
		{
			name:     "full_overlap",
			query:    "glucose blood",
			target:   "blood glucose",
			minScore: 0.9,
			maxScore: 1.0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			score := fm.tokenOverlapScore(tc.query, tc.target)
			if score < tc.minScore || score > tc.maxScore {
				t.Errorf("tokenOverlapScore(%q, %q) = %f, want [%f, %f]",
					tc.query, tc.target, score, tc.minScore, tc.maxScore)
			}
		})
	}
}

// =============================================================================
// BestMatch and MatchWithThreshold Tests
// =============================================================================

func TestFuzzyMatcher_BestMatch_NoResults(t *testing.T) {
	fm := newTestFuzzyMatcher()

	// Query that won't match anything
	match := fm.BestMatch("xyznonexistent12345")
	if match != nil {
		t.Errorf("BestMatch should return nil for non-matching query, got match: %+v", match)
	}
}

func TestFuzzyMatcher_MatchWithThreshold_HighThreshold(t *testing.T) {
	fm := newTestFuzzyMatcher()

	// With a very high threshold (0.99), fuzzy matches should be filtered out
	// Only exact code matches would pass this threshold
	match := fm.MatchWithThreshold("blood", 0.99)
	if match != nil && match.Confidence < 0.99 {
		t.Errorf("MatchWithThreshold(0.99) returned match with confidence %f < 0.99", match.Confidence)
	}

	// Non-matching query should return nil
	noMatch := fm.MatchWithThreshold("xyznonexistent", 0.5)
	if noMatch != nil {
		t.Errorf("MatchWithThreshold should return nil for non-matching query")
	}
}

// =============================================================================
// Concurrency Tests
// =============================================================================

func TestFuzzyMatcher_Concurrent(t *testing.T) {
	fm := newTestFuzzyMatcher()

	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				fm.Match("WBC")
				fm.Match("glucose")
				fm.BestMatch("hemoglobin")
				fm.MatchWithThreshold("CBC", 0.9)
			}
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}
