package autoroute

import (
	"strings"
	"testing"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/terminology/db"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/terminology/semantic"
)

func TestParseEquivalence(t *testing.T) {
	tests := []struct {
		input string
		want  db.MappingEquivalence
	}{
		{"equivalent", db.EquivalenceEquivalent},
		{"EQUIVALENT", db.EquivalenceEquivalent},
		{"Equivalent", db.EquivalenceEquivalent},
		{"wider", db.EquivalenceWider},
		{"WIDER", db.EquivalenceWider},
		{"narrower", db.EquivalenceNarrower},
		{"NARROWER", db.EquivalenceNarrower},
		{"inexact", db.EquivalenceInexact},
		{"INEXACT", db.EquivalenceInexact},
		{"unknown", db.EquivalenceInexact}, // Default
		{"", db.EquivalenceInexact},        // Default
		{"related", db.EquivalenceInexact}, // Unknown value defaults
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseEquivalence(tt.input)
			if got != tt.want {
				t.Errorf("parseEquivalence(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestFindDisplay(t *testing.T) {
	candidates := []semantic.SemanticMatch{
		{Code: "2345-7", Display: "Glucose [Mass/volume]"},
		{Code: "4548-4", Display: "Hemoglobin A1c"},
		{Code: "718-7", Display: "Hemoglobin"},
	}

	tests := []struct {
		code string
		want string
	}{
		{"2345-7", "Glucose [Mass/volume]"},
		{"4548-4", "Hemoglobin A1c"},
		{"718-7", "Hemoglobin"},
		{"99999", ""},  // Not found
		{"", ""},       // Empty code
		{"2345-8", ""}, // Similar but not matching
	}

	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			got := findDisplay(candidates, tt.code)
			if got != tt.want {
				t.Errorf("findDisplay(candidates, %q) = %q, want %q", tt.code, got, tt.want)
			}
		})
	}
}

func TestBuildRankingPrompt(t *testing.T) {
	req := RankRequest{
		SourceCode:    "GLU001",
		SourceDisplay: "Glucose Level",
		SourceSystem:  "epic_labs",
		TargetSystem:  "http://loinc.org",
		Candidates: []semantic.SemanticMatch{
			{Code: "2345-7", Display: "Glucose [Mass/volume] in Serum or Plasma", Score: 0.95},
			{Code: "2339-0", Display: "Glucose [Mass/volume] in Blood", Score: 0.88},
		},
	}

	prompt := buildRankingPrompt(req)

	// Verify prompt structure
	if !strings.Contains(prompt, "## Source Code to Map") {
		t.Error("prompt should contain source code header")
	}
	if !strings.Contains(prompt, "GLU001") {
		t.Error("prompt should contain source code")
	}
	if !strings.Contains(prompt, "Glucose Level") {
		t.Error("prompt should contain source display")
	}
	if !strings.Contains(prompt, "epic_labs") {
		t.Error("prompt should contain source system")
	}
	if !strings.Contains(prompt, "http://loinc.org") {
		t.Error("prompt should contain target system")
	}
	if !strings.Contains(prompt, "## Candidate Matches") {
		t.Error("prompt should contain candidates header")
	}
	if !strings.Contains(prompt, "2345-7") {
		t.Error("prompt should contain first candidate code")
	}
	if !strings.Contains(prompt, "2339-0") {
		t.Error("prompt should contain second candidate code")
	}
	if !strings.Contains(prompt, "0.95") {
		t.Error("prompt should contain similarity score")
	}
	if !strings.Contains(prompt, "## Task") {
		t.Error("prompt should contain task header")
	}
}

func TestBuildRankingPromptWithoutDisplay(t *testing.T) {
	req := RankRequest{
		SourceCode:   "GLU001",
		SourceSystem: "epic_labs",
		TargetSystem: "http://loinc.org",
		Candidates: []semantic.SemanticMatch{
			{Code: "2345-7", Display: "Glucose", Score: 0.90},
		},
	}

	prompt := buildRankingPrompt(req)

	// Should not contain Display line if empty
	lines := strings.Split(prompt, "\n")
	for _, line := range lines {
		if strings.Contains(line, "**Display**:") && strings.TrimSpace(line) == "- **Display**:" {
			t.Error("prompt should not contain empty display line")
		}
	}
}

func TestBuildRankingPromptNumbering(t *testing.T) {
	req := RankRequest{
		SourceCode:   "TEST",
		SourceSystem: "test",
		TargetSystem: "http://loinc.org",
		Candidates: []semantic.SemanticMatch{
			{Code: "A", Display: "First", Score: 0.9},
			{Code: "B", Display: "Second", Score: 0.8},
			{Code: "C", Display: "Third", Score: 0.7},
		},
	}

	prompt := buildRankingPrompt(req)

	// Verify candidates are numbered correctly
	if !strings.Contains(prompt, "1. **A**") {
		t.Error("first candidate should be numbered 1")
	}
	if !strings.Contains(prompt, "2. **B**") {
		t.Error("second candidate should be numbered 2")
	}
	if !strings.Contains(prompt, "3. **C**") {
		t.Error("third candidate should be numbered 3")
	}
}

func TestRankingSystemPrompt(t *testing.T) {
	// Verify the system prompt contains key guidance
	if !strings.Contains(rankingSystemPrompt, "healthcare terminology expert") {
		t.Error("system prompt should mention healthcare terminology expertise")
	}
	if !strings.Contains(rankingSystemPrompt, "Semantic equivalence") {
		t.Error("system prompt should mention semantic equivalence")
	}
	if !strings.Contains(rankingSystemPrompt, "Confidence score guidelines") {
		t.Error("system prompt should include confidence guidelines")
	}
	if !strings.Contains(rankingSystemPrompt, "equivalent") {
		t.Error("system prompt should explain equivalent type")
	}
	if !strings.Contains(rankingSystemPrompt, "wider") {
		t.Error("system prompt should explain wider type")
	}
	if !strings.Contains(rankingSystemPrompt, "narrower") {
		t.Error("system prompt should explain narrower type")
	}
	if !strings.Contains(rankingSystemPrompt, "inexact") {
		t.Error("system prompt should explain inexact type")
	}
}

func TestRankingOutputSchema(t *testing.T) {
	// Verify schema structure exists and has required fields
	schema := rankingOutputSchema

	if schema["type"] != "object" {
		t.Error("schema type should be object")
	}

	props, ok := schema["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("schema should have properties")
	}

	if _, ok := props["best_match"]; !ok {
		t.Error("schema should have best_match property")
	}
	if _, ok := props["alternates"]; !ok {
		t.Error("schema should have alternates property")
	}
	if _, ok := props["overall_reasoning"]; !ok {
		t.Error("schema should have overall_reasoning property")
	}

	required, ok := schema["required"].([]string)
	if !ok {
		t.Fatal("schema should have required fields")
	}

	hasRequired := map[string]bool{}
	for _, r := range required {
		hasRequired[r] = true
	}

	if !hasRequired["best_match"] {
		t.Error("best_match should be required")
	}
	if !hasRequired["overall_reasoning"] {
		t.Error("overall_reasoning should be required")
	}
}

func TestNewRanker(t *testing.T) {
	cfg := RankerConfig{
		Model:       "test-model",
		Temperature: 0.2,
	}

	// NewRanker should not panic with nil client (for config testing)
	ranker := NewRanker(nil, cfg)

	if ranker.model != "test-model" {
		t.Errorf("model = %v, want test-model", ranker.model)
	}
}
