package matching

import (
	"strings"
)

// MatchScore contains the result of probabilistic matching.
type MatchScore struct {
	// Score is the overall match score (0.0 to 1.0).
	Score float64 `json:"score"`

	// Confidence is the qualitative confidence level.
	Confidence string `json:"confidence"` // "high", "medium", "low", "none"

	// FieldScores contains the individual field scores.
	FieldScores map[string]FieldScore `json:"field_scores"`
}

// FieldScore contains the score for an individual field.
type FieldScore struct {
	// Weight is the configured weight for this field.
	Weight float64 `json:"weight"`

	// Score is the match score for this field (0.0 to 1.0).
	Score float64 `json:"score"`

	// Contribution is Weight * Score.
	Contribution float64 `json:"contribution"`

	// Available indicates whether both records had values.
	Available bool `json:"available"`

	// Method describes how the score was calculated.
	Method string `json:"method,omitempty"`
}

// FieldWeight defines the weight configuration for a matching field.
type FieldWeight struct {
	// Field is the field name.
	Field string

	// Weight is the importance weight (0.0 to 1.0).
	Weight float64

	// Algorithm is the matching algorithm to use.
	Algorithm MatchAlgorithm
}

// MatchAlgorithm defines how a field is compared.
type MatchAlgorithm string

const (
	// AlgorithmExact requires an exact match.
	AlgorithmExact MatchAlgorithm = "exact"

	// AlgorithmJaroWinkler uses Jaro-Winkler string similarity.
	AlgorithmJaroWinkler MatchAlgorithm = "jaro_winkler"

	// AlgorithmSoundex uses phonetic matching.
	AlgorithmSoundex MatchAlgorithm = "soundex"

	// AlgorithmLevenshtein uses normalized Levenshtein distance.
	AlgorithmLevenshtein MatchAlgorithm = "levenshtein"

	// AlgorithmPhone uses phone number comparison.
	AlgorithmPhone MatchAlgorithm = "phone"

	// AlgorithmDate requires exact date match.
	AlgorithmDate MatchAlgorithm = "date"
)

// ScorerConfig configures the probabilistic scorer.
type ScorerConfig struct {
	// Weights defines the field weights.
	Weights []FieldWeight

	// Thresholds define confidence cutoffs.
	HighConfidenceThreshold   float64 // Default: 0.90
	MediumConfidenceThreshold float64 // Default: 0.70
	LowConfidenceThreshold    float64 // Default: 0.50
}

// ProbabilisticScorer computes match scores using weighted field comparison.
type ProbabilisticScorer struct {
	weights    []FieldWeight
	thresholds ScorerConfig
}

// DefaultScorerConfig returns the default healthcare scoring configuration.
func DefaultScorerConfig() ScorerConfig {
	return ScorerConfig{
		Weights: []FieldWeight{
			{Field: "ssn", Weight: 0.95, Algorithm: AlgorithmExact},
			{Field: "mbi", Weight: 0.90, Algorithm: AlgorithmExact},
			{Field: "dob", Weight: 0.80, Algorithm: AlgorithmDate},
			{Field: "phone", Weight: 0.70, Algorithm: AlgorithmPhone},
			{Field: "family_name", Weight: 0.60, Algorithm: AlgorithmJaroWinkler},
			{Field: "given_name", Weight: 0.50, Algorithm: AlgorithmJaroWinkler},
			{Field: "postal_code", Weight: 0.40, Algorithm: AlgorithmExact},
			{Field: "gender", Weight: 0.30, Algorithm: AlgorithmExact},
			{Field: "email", Weight: 0.45, Algorithm: AlgorithmExact},
		},
		HighConfidenceThreshold:   0.90,
		MediumConfidenceThreshold: 0.70,
		LowConfidenceThreshold:    0.50,
	}
}

// NewProbabilisticScorer creates a scorer with the given configuration.
func NewProbabilisticScorer(config ScorerConfig) *ProbabilisticScorer {
	if config.HighConfidenceThreshold == 0 {
		config.HighConfidenceThreshold = 0.90
	}
	if config.MediumConfidenceThreshold == 0 {
		config.MediumConfidenceThreshold = 0.70
	}
	if config.LowConfidenceThreshold == 0 {
		config.LowConfidenceThreshold = 0.50
	}

	return &ProbabilisticScorer{
		weights:    config.Weights,
		thresholds: config,
	}
}

// Score computes the match score between two patients.
func (s *ProbabilisticScorer) Score(a, b *Patient) MatchScore {
	fieldScores := make(map[string]FieldScore)
	totalWeight := 0.0
	totalContribution := 0.0

	for _, fw := range s.weights {
		fs := s.scoreField(fw, a, b)
		fieldScores[fw.Field] = fs

		if fs.Available {
			totalWeight += fw.Weight
			totalContribution += fs.Contribution
		}
	}

	// Calculate normalized score
	var score float64
	if totalWeight > 0 {
		score = totalContribution / totalWeight
	}

	return MatchScore{
		Score:       score,
		Confidence:  s.classifyConfidence(score),
		FieldScores: fieldScores,
	}
}

// scoreField computes the score for a single field.
func (s *ProbabilisticScorer) scoreField(fw FieldWeight, a, b *Patient) FieldScore {
	valueA, valueB := s.getFieldValues(fw.Field, a, b)

	// Check availability
	if valueA == "" || valueB == "" {
		return FieldScore{
			Weight:    fw.Weight,
			Score:     0,
			Available: false,
		}
	}

	// Compute score based on algorithm
	var score float64
	var method string

	switch fw.Algorithm {
	case AlgorithmExact:
		if valueA == valueB {
			score = 1.0
		}
		method = "exact"

	case AlgorithmJaroWinkler:
		score = JaroWinkler(valueA, valueB)
		method = "jaro_winkler"

	case AlgorithmSoundex:
		if SoundexMatch(valueA, valueB) {
			score = 0.8 // Phonetic match is not as strong as exact
		} else {
			score = JaroWinkler(valueA, valueB) * 0.5 // Fallback to string similarity
		}
		method = "soundex"

	case AlgorithmLevenshtein:
		score = NormalizedLevenshtein(valueA, valueB)
		method = "levenshtein"

	case AlgorithmPhone:
		score = PhoneMatch(valueA, valueB)
		method = "phone"

	case AlgorithmDate:
		if valueA == valueB {
			score = 1.0
		}
		method = "date"

	default:
		if valueA == valueB {
			score = 1.0
		}
		method = "exact"
	}

	return FieldScore{
		Weight:       fw.Weight,
		Score:        score,
		Contribution: fw.Weight * score,
		Available:    true,
		Method:       method,
	}
}

// getFieldValues extracts field values from two patients.
func (s *ProbabilisticScorer) getFieldValues(field string, a, b *Patient) (string, string) {
	switch field {
	case "ssn":
		return NormalizeSSN(a.SSN), NormalizeSSN(b.SSN)
	case "mbi":
		return strings.ToUpper(strings.ReplaceAll(a.MBI, "-", "")),
			strings.ToUpper(strings.ReplaceAll(b.MBI, "-", ""))
	case "mrn":
		return a.MRN, b.MRN
	case "family_name":
		return NormalizeName(a.FamilyName), NormalizeName(b.FamilyName)
	case "given_name":
		return NormalizeName(a.GivenName), NormalizeName(b.GivenName)
	case "middle_name":
		return NormalizeName(a.MiddleName), NormalizeName(b.MiddleName)
	case "dob":
		if a.DOB.IsZero() || b.DOB.IsZero() {
			return "", ""
		}
		return a.DOB.Format("20060102"), b.DOB.Format("20060102")
	case "gender":
		return strings.ToUpper(a.Gender), strings.ToUpper(b.Gender)
	case "phone":
		return NormalizePhone(a.Phone), NormalizePhone(b.Phone)
	case "email":
		return strings.ToLower(strings.TrimSpace(a.Email)),
			strings.ToLower(strings.TrimSpace(b.Email))
	case "postal_code":
		return strings.TrimSpace(a.Address.PostalCode),
			strings.TrimSpace(b.Address.PostalCode)
	default:
		return "", ""
	}
}

// classifyConfidence returns a confidence level based on the score.
func (s *ProbabilisticScorer) classifyConfidence(score float64) string {
	switch {
	case score >= s.thresholds.HighConfidenceThreshold:
		return "high"
	case score >= s.thresholds.MediumConfidenceThreshold:
		return "medium"
	case score >= s.thresholds.LowConfidenceThreshold:
		return "low"
	default:
		return "none"
	}
}
