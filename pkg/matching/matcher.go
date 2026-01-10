package matching

// Matcher combines deterministic and probabilistic matching.
type Matcher struct {
	deterministic *DeterministicMatcher
	probabilistic *ProbabilisticScorer
	config        MatcherConfig
}

// MatcherConfig configures the combined matcher.
type MatcherConfig struct {
	// DeterministicRules overrides default deterministic rules.
	DeterministicRules []DeterministicRule

	// ScorerConfig overrides default probabilistic scoring.
	ScorerConfig ScorerConfig

	// UseDeterministicFirst tries deterministic matching before probabilistic.
	// Default: true
	UseDeterministicFirst bool

	// ConfirmThreshold is the probabilistic score above which to auto-confirm.
	// Default: 0.95
	ConfirmThreshold float64

	// ReviewThreshold is the probabilistic score above which to flag for review.
	// Default: 0.70
	ReviewThreshold float64
}

// DefaultMatcherConfig returns the default matcher configuration.
func DefaultMatcherConfig() MatcherConfig {
	return MatcherConfig{
		UseDeterministicFirst: true,
		ConfirmThreshold:      0.95,
		ReviewThreshold:       0.70,
	}
}

// CombinedMatchResult contains the full matching result.
type CombinedMatchResult struct {
	// Result is the final match classification.
	Result MatchResult `json:"result"`

	// DeterministicRule is the rule that matched (if any).
	DeterministicRule string `json:"deterministic_rule,omitempty"`

	// ProbabilisticScore contains the probabilistic scoring details.
	ProbabilisticScore *MatchScore `json:"probabilistic_score,omitempty"`

	// Source indicates which matcher determined the result.
	Source string `json:"source"` // "deterministic", "probabilistic", "combined"
}

// NewMatcher creates a combined matcher with the given configuration.
func NewMatcher(config MatcherConfig) *Matcher {
	var detRules []DeterministicRule
	if len(config.DeterministicRules) > 0 {
		detRules = config.DeterministicRules
	}

	var det *DeterministicMatcher
	if len(detRules) > 0 {
		det = NewDeterministicMatcherWithRules(detRules)
	} else {
		det = NewDeterministicMatcher()
	}

	scorerConfig := config.ScorerConfig
	if len(scorerConfig.Weights) == 0 {
		scorerConfig = DefaultScorerConfig()
	}

	if config.ConfirmThreshold == 0 {
		config.ConfirmThreshold = 0.95
	}
	if config.ReviewThreshold == 0 {
		config.ReviewThreshold = 0.70
	}

	return &Matcher{
		deterministic: det,
		probabilistic: NewProbabilisticScorer(scorerConfig),
		config:        config,
	}
}

// Match attempts to match two patient records.
func (m *Matcher) Match(a, b *Patient) CombinedMatchResult {
	// Try deterministic first if configured
	if m.config.UseDeterministicFirst {
		detResult, ruleName := m.deterministic.Match(a, b)

		if detResult == MatchConfirmed {
			return CombinedMatchResult{
				Result:            MatchConfirmed,
				DeterministicRule: ruleName,
				Source:            "deterministic",
			}
		}

		// For probable/possible, also compute probabilistic for details
		if detResult == MatchProbable || detResult == MatchPossible {
			probScore := m.probabilistic.Score(a, b)
			return CombinedMatchResult{
				Result:             detResult,
				DeterministicRule:  ruleName,
				ProbabilisticScore: &probScore,
				Source:             "combined",
			}
		}
	}

	// Fall back to probabilistic scoring
	probScore := m.probabilistic.Score(a, b)

	var result MatchResult
	switch {
	case probScore.Score >= m.config.ConfirmThreshold:
		result = MatchConfirmed
	case probScore.Score >= m.config.ReviewThreshold:
		result = MatchProbable
	case probScore.Score >= 0.50:
		result = MatchPossible
	default:
		result = MatchNoMatch
	}

	return CombinedMatchResult{
		Result:             result,
		ProbabilisticScore: &probScore,
		Source:             "probabilistic",
	}
}

// ScoreOnly computes only the probabilistic score without classification.
func (m *Matcher) ScoreOnly(a, b *Patient) MatchScore {
	return m.probabilistic.Score(a, b)
}

// FindCandidates searches for matching candidates in a list.
// Returns candidates sorted by score (highest first).
func (m *Matcher) FindCandidates(target *Patient, candidates []*Patient, minScore float64) []CandidateMatch {
	var results []CandidateMatch

	for _, candidate := range candidates {
		result := m.Match(target, candidate)
		probScore := 0.0
		if result.ProbabilisticScore != nil {
			probScore = result.ProbabilisticScore.Score
		} else if result.Result == MatchConfirmed {
			probScore = 1.0 // Deterministic match
		}

		if probScore >= minScore || result.Result == MatchConfirmed || result.Result == MatchProbable {
			results = append(results, CandidateMatch{
				Patient: candidate,
				Result:  result,
				Score:   probScore,
			})
		}
	}

	// Sort by score descending
	for i := range results {
		for j := i + 1; j < len(results); j++ {
			if results[j].Score > results[i].Score {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

	return results
}

// CandidateMatch represents a potential match.
type CandidateMatch struct {
	Patient *Patient            `json:"patient"`
	Result  CombinedMatchResult `json:"result"`
	Score   float64             `json:"score"`
}

// BatchMatcher provides efficient batch matching with blocking.
type BatchMatcher struct {
	matcher *Matcher
}

// NewBatchMatcher creates a batch matcher.
func NewBatchMatcher(matcher *Matcher) *BatchMatcher {
	return &BatchMatcher{matcher: matcher}
}

// FindDuplicates identifies potential duplicate patients in a list.
func (b *BatchMatcher) FindDuplicates(patients []*Patient, minScore float64) []DuplicateGroup {
	// Build blocking index
	blocks := make(map[string][]*Patient)
	for _, p := range patients {
		key := BlockingKey(p)
		if key != "" {
			blocks[key] = append(blocks[key], p)
		}
		// Also add Soundex-based key for phonetic matching
		sKey := SoundexBlockingKey(p)
		if sKey != "" && sKey != key {
			blocks[sKey] = append(blocks[sKey], p)
		}
	}

	// Track which pairs we've already compared
	compared := make(map[string]bool)
	var groups []DuplicateGroup

	for _, block := range blocks {
		for i := 0; i < len(block); i++ {
			for j := i + 1; j < len(block); j++ {
				// Create unique pair key
				pairKey := pairID(block[i], block[j])
				if compared[pairKey] {
					continue
				}
				compared[pairKey] = true

				result := b.matcher.Match(block[i], block[j])
				probScore := 0.0
				if result.ProbabilisticScore != nil {
					probScore = result.ProbabilisticScore.Score
				} else if result.Result == MatchConfirmed {
					probScore = 1.0
				}

				if probScore >= minScore || result.Result == MatchConfirmed || result.Result == MatchProbable {
					groups = append(groups, DuplicateGroup{
						Patients: []*Patient{block[i], block[j]},
						Result:   result,
						Score:    probScore,
					})
				}
			}
		}
	}

	// Sort by score descending
	for i := range groups {
		for j := i + 1; j < len(groups); j++ {
			if groups[j].Score > groups[i].Score {
				groups[i], groups[j] = groups[j], groups[i]
			}
		}
	}

	return groups
}

// DuplicateGroup represents a group of potentially duplicate patients.
type DuplicateGroup struct {
	Patients []*Patient          `json:"patients"`
	Result   CombinedMatchResult `json:"result"`
	Score    float64             `json:"score"`
}

// pairID creates a unique identifier for a patient pair.
func pairID(a, b *Patient) string {
	// Use MRN + System as unique identifier, or fall back to pointer address
	idA := a.MRN + "|" + a.MRNSystem
	idB := b.MRN + "|" + b.MRNSystem

	if idA < idB {
		return idA + ":" + idB
	}
	return idB + ":" + idA
}
