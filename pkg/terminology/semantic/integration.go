package semantic

import (
	"context"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/terminology/index"
)

// FuzzyMatch is a re-export compatible type for integration with terminology.FuzzyMatch.
// This allows semantic search results to be used alongside fuzzy matching results.
type FuzzyMatch struct {
	Code       string  `json:"code"`
	Display    string  `json:"display"`
	System     string  `json:"system"`
	Confidence float64 `json:"confidence"`
	MatchType  string  `json:"match_type"`
	Reason     string  `json:"reason,omitempty"`
}

// ToFuzzyMatch converts a SemanticMatch to a FuzzyMatch-compatible struct.
func (m *SemanticMatch) ToFuzzyMatch() FuzzyMatch {
	return FuzzyMatch{
		Code:       m.Code,
		Display:    m.Display,
		System:     m.System,
		Confidence: m.Score,
		MatchType:  m.MatchType,
		Reason:     m.Reason,
	}
}

// ToFuzzyMatches converts a slice of SemanticMatch to FuzzyMatch-compatible structs.
func ToFuzzyMatches(matches []SemanticMatch) []FuzzyMatch {
	result := make([]FuzzyMatch, len(matches))
	for i, m := range matches {
		result[i] = m.ToFuzzyMatch()
	}
	return result
}

// HybridSearcher combines fuzzy and semantic search for optimal results.
type HybridSearcher struct {
	semantic *Searcher
	config   HybridConfig
}

// HybridConfig configures the hybrid searcher.
type HybridConfig struct {
	// SemanticFallbackThreshold is the fuzzy match score below which
	// semantic search is triggered as a fallback.
	SemanticFallbackThreshold float64 `yaml:"semantic_fallback_threshold" json:"semantic_fallback_threshold"`

	// PreferSemantic always includes semantic results, not just as fallback.
	PreferSemantic bool `yaml:"prefer_semantic" json:"prefer_semantic"`

	// SemanticWeight is the weight given to semantic scores when combining.
	// Range: 0.0 (ignore semantic) to 1.0 (only semantic).
	SemanticWeight float64 `yaml:"semantic_weight" json:"semantic_weight"`

	// MaxResults limits the combined result count.
	MaxResults int `yaml:"max_results" json:"max_results"`
}

// DefaultHybridConfig returns default hybrid search configuration.
func DefaultHybridConfig() HybridConfig {
	return HybridConfig{
		SemanticFallbackThreshold: 0.7,
		PreferSemantic:            false,
		SemanticWeight:            0.3,
		MaxResults:                10,
	}
}

// NewHybridSearcher creates a new hybrid searcher.
func NewHybridSearcher(semantic *Searcher, cfg HybridConfig) *HybridSearcher {
	return &HybridSearcher{
		semantic: semantic,
		config:   cfg,
	}
}

// SearchWithFallback performs fuzzy search first, then falls back to semantic
// search if results are below the confidence threshold.
// The fuzzyMatches parameter should be the results from an existing FuzzyMatcher.
func (h *HybridSearcher) SearchWithFallback(ctx context.Context, query string, fuzzyMatches []FuzzyMatch, vocab index.Vocabulary) ([]FuzzyMatch, error) {
	// Check if fuzzy matches are good enough
	hasSufficientMatch := false
	for _, m := range fuzzyMatches {
		if m.Confidence >= h.config.SemanticFallbackThreshold {
			hasSufficientMatch = true
			break
		}
	}

	// If we have good fuzzy matches and don't prefer semantic, return as-is
	if hasSufficientMatch && !h.config.PreferSemantic {
		return fuzzyMatches, nil
	}

	// Perform semantic search
	semanticOpts := SearchOptions{
		Vocabularies: []index.Vocabulary{vocab},
		MaxResults:   h.config.MaxResults,
		MinScore:     0.5,
	}

	semanticMatches, err := h.semantic.Search(ctx, query, semanticOpts)
	if err != nil {
		// On semantic search failure, gracefully fall back to fuzzy matches
		return fuzzyMatches, nil //nolint:nilerr // Intentional: semantic search is optional enhancement
	}

	// Combine results
	return h.combineResults(fuzzyMatches, semanticMatches), nil
}

// combineResults merges fuzzy and semantic results, deduplicating by code.
func (h *HybridSearcher) combineResults(fuzzy []FuzzyMatch, semantic []SemanticMatch) []FuzzyMatch {
	seen := make(map[string]int) // code -> index in result
	var result []FuzzyMatch

	// Add fuzzy matches first
	for _, m := range fuzzy {
		seen[m.Code] = len(result)
		result = append(result, m)
	}

	// Add or merge semantic matches
	for _, sm := range semantic {
		fm := sm.ToFuzzyMatch()
		if idx, exists := seen[sm.Code]; exists {
			// Code already exists - update if semantic score would improve it
			existing := result[idx]
			combinedScore := (1-h.config.SemanticWeight)*existing.Confidence +
				h.config.SemanticWeight*sm.Score
			if combinedScore > existing.Confidence {
				result[idx].Confidence = combinedScore
				result[idx].Reason = existing.Reason + " (semantic boost)"
			}
		} else {
			// New code from semantic search
			fm.Reason = "semantic match"
			seen[fm.Code] = len(result)
			result = append(result, fm)
		}
	}

	// Sort by confidence
	sortFuzzyByConfidence(result)

	// Limit results
	if h.config.MaxResults > 0 && len(result) > h.config.MaxResults {
		result = result[:h.config.MaxResults]
	}

	return result
}

// sortFuzzyByConfidence sorts FuzzyMatch slice by confidence descending.
func sortFuzzyByConfidence(matches []FuzzyMatch) {
	for i := 1; i < len(matches); i++ {
		key := matches[i]
		j := i - 1
		for j >= 0 && matches[j].Confidence < key.Confidence {
			matches[j+1] = matches[j]
			j--
		}
		matches[j+1] = key
	}
}

// SemanticEnhancer wraps a semantic searcher to enhance terminology lookups.
type SemanticEnhancer struct {
	searcher *Searcher
}

// NewSemanticEnhancer creates a new semantic enhancer.
func NewSemanticEnhancer(searcher *Searcher) *SemanticEnhancer {
	return &SemanticEnhancer{searcher: searcher}
}

// EnhanceCode attempts to find semantically similar codes for an unmapped code.
// This is useful when a local code doesn't have a direct mapping.
func (e *SemanticEnhancer) EnhanceCode(ctx context.Context, localCode, displayText string, targetVocab index.Vocabulary) ([]SemanticMatch, error) {
	// Use display text for semantic search
	if displayText == "" {
		displayText = localCode
	}

	opts := SearchOptions{
		Vocabularies: []index.Vocabulary{targetVocab},
		MaxResults:   5,
		MinScore:     0.6,
	}

	matches, err := e.searcher.Search(ctx, displayText, opts)
	if err != nil {
		return nil, err
	}

	// Add context to reason
	for i := range matches {
		matches[i].Reason = "semantic match for '" + displayText + "'"
	}

	return matches, nil
}

// FindSimilarCodes finds codes semantically similar to a given code.
func (e *SemanticEnhancer) FindSimilarCodes(ctx context.Context, code, display string, vocab index.Vocabulary, limit int) ([]SemanticMatch, error) {
	searchText := display
	if searchText == "" {
		// If no display, we need to look up the code first
		// This would require access to the vocabulary data
		searchText = code
	}

	opts := SearchOptions{
		Vocabularies: []index.Vocabulary{vocab},
		MaxResults:   limit,
		MinScore:     0.7, // Higher threshold for "similar" codes
	}

	matches, err := e.searcher.Search(ctx, searchText, opts)
	if err != nil {
		return nil, err
	}

	// Filter out the original code
	var filtered []SemanticMatch
	for _, m := range matches {
		if m.Code != code {
			filtered = append(filtered, m)
		}
	}

	return filtered, nil
}

// MultiVocabularySearch searches across multiple vocabularies simultaneously.
type MultiVocabularySearch struct {
	searcher *Searcher
}

// NewMultiVocabularySearch creates a new multi-vocabulary searcher.
func NewMultiVocabularySearch(searcher *Searcher) *MultiVocabularySearch {
	return &MultiVocabularySearch{searcher: searcher}
}

// SearchAll searches all supported vocabularies.
func (m *MultiVocabularySearch) SearchAll(ctx context.Context, query string, maxPerVocab int) (map[index.Vocabulary][]SemanticMatch, error) {
	vocabs := []index.Vocabulary{
		index.VocabularyLOINC,
		index.VocabularySNOMED,
		index.VocabularyICD10CM,
	}

	results := make(map[index.Vocabulary][]SemanticMatch)

	for _, vocab := range vocabs {
		opts := SearchOptions{
			Vocabularies: []index.Vocabulary{vocab},
			MaxResults:   maxPerVocab,
			MinScore:     0.5,
		}

		matches, err := m.searcher.Search(ctx, query, opts)
		if err != nil {
			// Skip vocabularies with errors
			continue
		}
		if len(matches) > 0 {
			results[vocab] = matches
		}
	}

	return results, nil
}

// SearchWithPreference searches with vocabulary preference order.
func (m *MultiVocabularySearch) SearchWithPreference(ctx context.Context, query string, preferences []index.Vocabulary, maxResults int) ([]SemanticMatch, error) {
	var allMatches []SemanticMatch

	for _, vocab := range preferences {
		opts := SearchOptions{
			Vocabularies: []index.Vocabulary{vocab},
			MaxResults:   maxResults,
			MinScore:     0.6,
		}

		matches, err := m.searcher.Search(ctx, query, opts)
		if err != nil {
			continue
		}

		// If we found high-confidence matches, return them
		for _, m := range matches {
			if m.IsHighConfidence() {
				return matches, nil
			}
		}

		allMatches = append(allMatches, matches...)
	}

	// Sort all matches and return top results
	sortByScore(allMatches)
	if len(allMatches) > maxResults {
		allMatches = allMatches[:maxResults]
	}

	return allMatches, nil
}
