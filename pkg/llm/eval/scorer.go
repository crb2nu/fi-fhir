package eval

import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
)

// Scorer computes a similarity score between expected and actual outputs.
type Scorer interface {
	// Score returns a value between 0.0 (no match) and 1.0 (perfect match).
	Score(expected, actual json.RawMessage) float64
}

// JSONMatchScorer uses deep equality on parsed JSON with partial match support.
type JSONMatchScorer struct{}

func (s *JSONMatchScorer) Score(expected, actual json.RawMessage) float64 {
	var exp, act interface{}
	if err := json.Unmarshal(expected, &exp); err != nil {
		return 0
	}
	if err := json.Unmarshal(actual, &act); err != nil {
		return 0
	}

	return deepScore(exp, act)
}

// deepScore recursively compares two values and returns a similarity score.
func deepScore(expected, actual interface{}) float64 {
	if expected == nil && actual == nil {
		return 1.0
	}
	if expected == nil || actual == nil {
		return 0.0
	}

	switch exp := expected.(type) {
	case map[string]interface{}:
		act, ok := actual.(map[string]interface{})
		if !ok {
			return 0
		}
		return objectScore(exp, act)

	case []interface{}:
		act, ok := actual.([]interface{})
		if !ok {
			return 0
		}
		return arrayScore(exp, act)

	case string:
		act, ok := actual.(string)
		if !ok {
			return 0
		}
		return stringScore(exp, act)

	case float64:
		act, ok := actual.(float64)
		if !ok {
			return 0
		}
		return numberScore(exp, act)

	case bool:
		act, ok := actual.(bool)
		if !ok {
			return 0
		}
		if exp == act {
			return 1.0
		}
		return 0.0

	default:
		if reflect.DeepEqual(expected, actual) {
			return 1.0
		}
		return 0.0
	}
}

// objectScore computes weighted field-level matching.
func objectScore(exp, act map[string]interface{}) float64 {
	if len(exp) == 0 {
		return 1.0
	}

	var totalScore float64
	for key, expVal := range exp {
		actVal, exists := act[key]
		if !exists {
			continue // Missing field = 0 score for this key
		}
		totalScore += deepScore(expVal, actVal)
	}

	return totalScore / float64(len(exp))
}

// arrayScore compares arrays element-wise.
func arrayScore(exp, act []interface{}) float64 {
	if len(exp) == 0 {
		return 1.0
	}
	if len(act) == 0 {
		return 0
	}

	maxLen := len(exp)
	if len(act) > maxLen {
		maxLen = len(act)
	}

	var totalScore float64
	for i := 0; i < len(exp) && i < len(act); i++ {
		totalScore += deepScore(exp[i], act[i])
	}

	return totalScore / float64(maxLen)
}

// stringScore does case-insensitive fuzzy comparison.
func stringScore(expected, actual string) float64 {
	if expected == actual {
		return 1.0
	}

	// Case-insensitive exact match
	if strings.EqualFold(expected, actual) {
		return 0.95
	}

	// Jaccard similarity on words
	return jaccardSimilarity(expected, actual)
}

// numberScore compares numbers with tolerance.
func numberScore(expected, actual float64) float64 {
	if expected == actual {
		return 1.0
	}

	// For confidence scores (0-1 range), use absolute difference
	if expected >= 0 && expected <= 1 && actual >= 0 && actual <= 1 {
		return 1.0 - math.Abs(expected-actual)
	}

	// For other numbers, use relative difference
	maxVal := math.Max(math.Abs(expected), math.Abs(actual))
	if maxVal == 0 {
		return 1.0
	}
	relDiff := math.Abs(expected-actual) / maxVal
	return math.Max(0, 1.0-relDiff)
}

// jaccardSimilarity computes word-level Jaccard similarity.
func jaccardSimilarity(a, b string) float64 {
	wordsA := tokenize(a)
	wordsB := tokenize(b)

	if len(wordsA) == 0 && len(wordsB) == 0 {
		return 1.0
	}

	intersection := 0
	setB := make(map[string]bool, len(wordsB))
	for _, w := range wordsB {
		setB[w] = true
	}
	setA := make(map[string]bool, len(wordsA))
	for _, w := range wordsA {
		setA[w] = true
		if setB[w] {
			intersection++
		}
	}

	union := len(setA) + len(setB) - intersection
	if union == 0 {
		return 1.0
	}

	return float64(intersection) / float64(union)
}

// tokenize splits text into lowercase words.
func tokenize(s string) []string {
	s = strings.ToLower(s)
	return strings.Fields(s)
}

// ConfidenceDeltaScorer checks that an actual confidence value is
// within a tolerance of the expected value.
type ConfidenceDeltaScorer struct {
	// Tolerance is the maximum allowed difference (default: 0.15).
	Tolerance float64
	// FieldPath is the JSON path to the confidence field (default: "confidence").
	FieldPath string
}

func (s *ConfidenceDeltaScorer) Score(expected, actual json.RawMessage) float64 {
	tolerance := s.Tolerance
	if tolerance == 0 {
		tolerance = 0.15
	}
	fieldPath := s.FieldPath
	if fieldPath == "" {
		fieldPath = "confidence"
	}

	expVal := extractFloat(expected, fieldPath)
	actVal := extractFloat(actual, fieldPath)

	if math.IsNaN(expVal) || math.IsNaN(actVal) {
		return 0
	}

	delta := math.Abs(expVal - actVal)
	if delta <= tolerance {
		return 1.0 - (delta / tolerance)
	}
	return 0
}

// extractFloat extracts a float64 from a JSON object at a simple key path.
func extractFloat(data json.RawMessage, key string) float64 {
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return math.NaN()
	}

	parts := strings.Split(key, ".")
	var current interface{} = m

	for _, part := range parts {
		obj, ok := current.(map[string]interface{})
		if !ok {
			return math.NaN()
		}
		current, ok = obj[part]
		if !ok {
			return math.NaN()
		}
	}

	val, ok := current.(float64)
	if !ok {
		return math.NaN()
	}
	return val
}

// CompositeScorer combines multiple scorers with weights.
type CompositeScorer struct {
	Scorers []WeightedScorer
}

// WeightedScorer pairs a scorer with a weight.
type WeightedScorer struct {
	Scorer Scorer
	Weight float64
}

func (cs *CompositeScorer) Score(expected, actual json.RawMessage) float64 {
	var totalWeight, totalScore float64
	for _, ws := range cs.Scorers {
		totalScore += ws.Weight * ws.Scorer.Score(expected, actual)
		totalWeight += ws.Weight
	}
	if totalWeight == 0 {
		return 0
	}
	return totalScore / totalWeight
}
