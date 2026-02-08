package semantic

import (
	"testing"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/terminology/index"
)

func TestToFuzzyMatch(t *testing.T) {
	match := SemanticMatch{
		Code:       "2345-7",
		Display:    "Glucose [Mass/volume] in Serum or Plasma",
		System:     "http://loinc.org",
		Vocabulary: index.VocabularyLOINC,
		Score:      0.92,
		MatchType:  "semantic",
		Reason:     "high similarity",
	}

	fm := match.ToFuzzyMatch()

	if fm.Code != match.Code {
		t.Errorf("Code = %s, want %s", fm.Code, match.Code)
	}
	if fm.Display != match.Display {
		t.Errorf("Display = %s, want %s", fm.Display, match.Display)
	}
	if fm.System != match.System {
		t.Errorf("System = %s, want %s", fm.System, match.System)
	}
	if fm.Confidence != match.Score {
		t.Errorf("Confidence = %f, want %f (should equal Score)", fm.Confidence, match.Score)
	}
	if fm.MatchType != match.MatchType {
		t.Errorf("MatchType = %s, want %s", fm.MatchType, match.MatchType)
	}
	if fm.Reason != match.Reason {
		t.Errorf("Reason = %s, want %s", fm.Reason, match.Reason)
	}
}

func TestToFuzzyMatches(t *testing.T) {
	matches := []SemanticMatch{
		{Code: "A", Score: 0.95, MatchType: "semantic"},
		{Code: "B", Score: 0.85, MatchType: "semantic"},
		{Code: "C", Score: 0.75, MatchType: "semantic"},
	}

	result := ToFuzzyMatches(matches)

	if len(result) != len(matches) {
		t.Fatalf("Length = %d, want %d", len(result), len(matches))
	}

	// Verify ordering is preserved
	for i, fm := range result {
		if fm.Code != matches[i].Code {
			t.Errorf("result[%d].Code = %s, want %s", i, fm.Code, matches[i].Code)
		}
		if fm.Confidence != matches[i].Score {
			t.Errorf("result[%d].Confidence = %f, want %f", i, fm.Confidence, matches[i].Score)
		}
	}
}

func TestToFuzzyMatches_Empty(t *testing.T) {
	result := ToFuzzyMatches([]SemanticMatch{})
	if len(result) != 0 {
		t.Errorf("Expected empty result, got %d", len(result))
	}
}

func TestSortByScore(t *testing.T) {
	matches := []SemanticMatch{
		{Code: "C", Score: 0.70},
		{Code: "A", Score: 0.95},
		{Code: "B", Score: 0.85},
	}

	sortByScore(matches)

	if matches[0].Code != "A" {
		t.Errorf("First = %s, want A (highest score)", matches[0].Code)
	}
	if matches[1].Code != "B" {
		t.Errorf("Second = %s, want B", matches[1].Code)
	}
	if matches[2].Code != "C" {
		t.Errorf("Third = %s, want C (lowest score)", matches[2].Code)
	}
}

func TestSortFuzzyByConfidence(t *testing.T) {
	matches := []FuzzyMatch{
		{Code: "C", Confidence: 0.70},
		{Code: "A", Confidence: 0.95},
		{Code: "B", Confidence: 0.85},
	}

	sortFuzzyByConfidence(matches)

	if matches[0].Code != "A" {
		t.Errorf("First = %s, want A (highest confidence)", matches[0].Code)
	}
	if matches[1].Code != "B" {
		t.Errorf("Second = %s, want B", matches[1].Code)
	}
	if matches[2].Code != "C" {
		t.Errorf("Third = %s, want C (lowest confidence)", matches[2].Code)
	}
}

func TestGetString(t *testing.T) {
	payload := map[string]interface{}{
		"code":    "12345",
		"display": "Test Display",
		"count":   42, // not a string
	}

	if got := getString(payload, "code"); got != "12345" {
		t.Errorf("getString(code) = %s, want 12345", got)
	}
	if got := getString(payload, "display"); got != "Test Display" {
		t.Errorf("getString(display) = %s, want Test Display", got)
	}
	if got := getString(payload, "count"); got != "" {
		t.Errorf("getString(count) = %s, want empty (not a string)", got)
	}
	if got := getString(payload, "missing"); got != "" {
		t.Errorf("getString(missing) = %s, want empty", got)
	}
	if got := getString(nil, "key"); got != "" {
		t.Errorf("getString(nil, key) = %s, want empty", got)
	}
}

func TestHitToMatch(t *testing.T) {
	hit := index.SearchHit{
		ID:    "point-1",
		Score: 0.88,
		Payload: map[string]interface{}{
			"code":       "2345-7",
			"display":    "Glucose [Mass/volume]",
			"system":     "http://loinc.org",
			"vocabulary": "loinc",
			"component":  "Glucose",
		},
	}

	match := hitToMatch(hit, index.VocabularyLOINC)

	if match.Code != "2345-7" {
		t.Errorf("Code = %s, want 2345-7", match.Code)
	}
	if match.Display != "Glucose [Mass/volume]" {
		t.Errorf("Display = %s", match.Display)
	}
	if match.System != "http://loinc.org" {
		t.Errorf("System = %s", match.System)
	}
	if match.Vocabulary != index.VocabularyLOINC {
		t.Errorf("Vocabulary = %s, want loinc", match.Vocabulary)
	}
	if match.Score != 0.88 {
		t.Errorf("Score = %f, want 0.88", match.Score)
	}
	if match.MatchType != "semantic" {
		t.Errorf("MatchType = %s, want semantic", match.MatchType)
	}

	// Metadata should contain "component" but not code/display/system/vocabulary
	if _, ok := match.Metadata["component"]; !ok {
		t.Error("Expected 'component' in metadata")
	}
	if _, ok := match.Metadata["code"]; ok {
		t.Error("'code' should not be in metadata (it's a top-level field)")
	}
	if _, ok := match.Metadata["display"]; ok {
		t.Error("'display' should not be in metadata")
	}
	if _, ok := match.Metadata["system"]; ok {
		t.Error("'system' should not be in metadata")
	}
	if _, ok := match.Metadata["vocabulary"]; ok {
		t.Error("'vocabulary' should not be in metadata")
	}
}
