package eval

import (
	"encoding/json"
	"math"
	"testing"
)

func TestJSONMatchScorer_ExactMatch(t *testing.T) {
	s := &JSONMatchScorer{}
	exp := json.RawMessage(`{"code":"E11.9","confidence":0.95}`)
	act := json.RawMessage(`{"code":"E11.9","confidence":0.95}`)
	score := s.Score(exp, act)
	if score < 0.99 {
		t.Errorf("expected ~1.0 for exact match, got %.2f", score)
	}
}

func TestJSONMatchScorer_PartialMatch(t *testing.T) {
	s := &JSONMatchScorer{}
	exp := json.RawMessage(`{"code":"E11.9","confidence":0.95}`)
	act := json.RawMessage(`{"code":"E11.9","confidence":0.80}`)
	score := s.Score(exp, act)
	if score < 0.5 || score > 0.99 {
		t.Errorf("expected partial match score, got %.2f", score)
	}
}

func TestJSONMatchScorer_NoMatch(t *testing.T) {
	s := &JSONMatchScorer{}
	exp := json.RawMessage(`{"code":"E11.9"}`)
	act := json.RawMessage(`{"code":"J45.0"}`)
	score := s.Score(exp, act)
	if score > 0.5 {
		t.Errorf("expected low score for different codes, got %.2f", score)
	}
}

func TestJSONMatchScorer_InvalidJSON(t *testing.T) {
	s := &JSONMatchScorer{}
	score := s.Score(json.RawMessage(`invalid`), json.RawMessage(`{}`))
	if score != 0 {
		t.Errorf("expected 0 for invalid JSON, got %.2f", score)
	}
}

func TestJSONMatchScorer_ArrayMatch(t *testing.T) {
	s := &JSONMatchScorer{}
	exp := json.RawMessage(`{"items":[{"code":"A"},{"code":"B"}]}`)
	act := json.RawMessage(`{"items":[{"code":"A"},{"code":"B"}]}`)
	score := s.Score(exp, act)
	if score < 0.99 {
		t.Errorf("expected ~1.0 for matching arrays, got %.2f", score)
	}
}

func TestJaccardSimilarity(t *testing.T) {
	tests := []struct {
		a, b   string
		minExp float64
	}{
		{"diabetes mellitus type 2", "type 2 diabetes mellitus", 0.9},
		{"hello world", "hello world", 1.0},
		{"foo", "bar", 0.0},
		{"", "", 1.0},
	}

	for _, tt := range tests {
		score := jaccardSimilarity(tt.a, tt.b)
		if score < tt.minExp {
			t.Errorf("jaccardSimilarity(%q, %q) = %.2f, expected >= %.2f", tt.a, tt.b, score, tt.minExp)
		}
	}
}

func TestStringScore(t *testing.T) {
	// Exact match
	if stringScore("hello", "hello") != 1.0 {
		t.Error("exact match should be 1.0")
	}

	// Case insensitive
	score := stringScore("Hello", "hello")
	if score < 0.9 {
		t.Errorf("case insensitive should be high, got %.2f", score)
	}
}

func TestNumberScore(t *testing.T) {
	// Exact match
	if numberScore(0.95, 0.95) != 1.0 {
		t.Error("exact number match should be 1.0")
	}

	// Close confidence scores
	score := numberScore(0.95, 0.90)
	if score < 0.9 {
		t.Errorf("close confidence scores should be high, got %.2f", score)
	}

	// Far apart
	score = numberScore(0.95, 0.1)
	if score > 0.2 {
		t.Errorf("far apart scores should be low, got %.2f", score)
	}
}

func TestConfidenceDeltaScorer(t *testing.T) {
	s := &ConfidenceDeltaScorer{Tolerance: 0.1}

	exp := json.RawMessage(`{"confidence":0.95}`)
	act := json.RawMessage(`{"confidence":0.90}`)
	score := s.Score(exp, act)
	if score < 0.4 {
		t.Errorf("expected reasonable score within tolerance, got %.2f", score)
	}

	// Out of tolerance
	actFar := json.RawMessage(`{"confidence":0.5}`)
	score = s.Score(exp, actFar)
	if score != 0 {
		t.Errorf("expected 0 for out-of-tolerance, got %.2f", score)
	}
}

func TestConfidenceDeltaScorer_NestedPath(t *testing.T) {
	s := &ConfidenceDeltaScorer{FieldPath: "best_match.confidence"}

	exp := json.RawMessage(`{"best_match":{"confidence":0.95}}`)
	act := json.RawMessage(`{"best_match":{"confidence":0.92}}`)
	score := s.Score(exp, act)
	if score < 0.5 {
		t.Errorf("expected reasonable score for nested path, got %.2f", score)
	}
}

func TestCompositeScorer(t *testing.T) {
	cs := &CompositeScorer{
		Scorers: []WeightedScorer{
			{Scorer: &JSONMatchScorer{}, Weight: 0.7},
			{Scorer: &ConfidenceDeltaScorer{}, Weight: 0.3},
		},
	}

	exp := json.RawMessage(`{"code":"E11.9","confidence":0.95}`)
	act := json.RawMessage(`{"code":"E11.9","confidence":0.90}`)
	score := cs.Score(exp, act)
	if score < 0.5 {
		t.Errorf("expected reasonable composite score, got %.2f", score)
	}
}

func TestExtractFloat_Invalid(t *testing.T) {
	result := extractFloat(json.RawMessage(`invalid`), "key")
	if !math.IsNaN(result) {
		t.Errorf("expected NaN for invalid JSON, got %f", result)
	}
}

func TestDeepScore_Booleans(t *testing.T) {
	if deepScore(true, true) != 1.0 {
		t.Error("matching bools should score 1.0")
	}
	if deepScore(true, false) != 0.0 {
		t.Error("mismatched bools should score 0.0")
	}
}
