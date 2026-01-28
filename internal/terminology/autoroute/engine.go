package autoroute

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/llm"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/terminology/index"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/terminology/semantic"
)

// Engine orchestrates LLM-powered autorouting with semantic search.
type Engine struct {
	searcher *semantic.Searcher
	ranker   *Ranker
	cache    *Cache
	config   Config
}

// Config configures the autoroute engine.
type Config struct {
	// Confidence thresholds
	HighConfidenceThreshold float64 // Default: 0.90
	MedConfidenceThreshold  float64 // Default: 0.70

	// Semantic search settings
	SemanticTopK     int     // Number of candidates from semantic search (default: 10)
	SemanticMinScore float64 // Minimum score to consider (default: 0.5)

	// Skip LLM if semantic match is very strong
	SemanticAutoAcceptThreshold float64 // Default: 0.95

	// Caching
	CacheTTL     time.Duration // Default: 15 minutes
	CacheEnabled bool          // Default: true

	// LLM settings
	LLMModel string // Model to use for ranking
}

// DefaultConfig returns default engine configuration.
func DefaultConfig() Config {
	return Config{
		HighConfidenceThreshold:     0.90,
		MedConfidenceThreshold:      0.70,
		SemanticTopK:                10,
		SemanticMinScore:            0.5,
		SemanticAutoAcceptThreshold: 0.95,
		CacheTTL:                    15 * time.Minute,
		CacheEnabled:                true,
	}
}

// NewEngine creates a new autoroute engine.
func NewEngine(searcher *semantic.Searcher, llmClient llm.Client, cfg Config) *Engine {
	// Apply defaults
	if cfg.HighConfidenceThreshold == 0 {
		cfg.HighConfidenceThreshold = 0.90
	}
	if cfg.MedConfidenceThreshold == 0 {
		cfg.MedConfidenceThreshold = 0.70
	}
	if cfg.SemanticTopK == 0 {
		cfg.SemanticTopK = 10
	}
	if cfg.SemanticMinScore == 0 {
		cfg.SemanticMinScore = 0.5
	}
	if cfg.SemanticAutoAcceptThreshold == 0 {
		cfg.SemanticAutoAcceptThreshold = 0.95
	}
	if cfg.CacheTTL == 0 {
		cfg.CacheTTL = 15 * time.Minute
	}

	var cache *Cache
	if cfg.CacheEnabled {
		cache = NewCache(CacheConfig{
			TTL:     cfg.CacheTTL,
			MaxSize: 10000,
		})
	}

	return &Engine{
		searcher: searcher,
		ranker: NewRanker(llmClient, RankerConfig{
			Model:       cfg.LLMModel,
			Temperature: 0.1,
		}),
		cache:  cache,
		config: cfg,
	}
}

// Suggest finds and ranks potential mappings for a source code.
func (e *Engine) Suggest(ctx context.Context, req SuggestRequest) (*SuggestResult, error) {
	start := time.Now()

	// Apply defaults
	if req.MaxCandidates == 0 {
		req.MaxCandidates = 5
	}

	// Check cache first
	if e.cache != nil {
		if cached := e.cache.Get(req); cached != nil {
			return cached, nil
		}
	}

	// Initialize decision trace
	trace := &DecisionTrace{
		TraceID:   uuid.New().String(),
		Timestamp: start,
		Request: TraceRequest{
			SourceCode:    req.SourceCode,
			SourceSystem:  req.SourceSystem,
			SourceDisplay: req.SourceDisplay,
			TargetSystem:  req.TargetSystem,
			ProfileID:     req.ProfileID,
		},
		Steps: make([]DecisionStep, 0, 3),
	}

	// Step 1: Semantic search for candidates
	searchStart := time.Now()
	searchResults, err := e.semanticSearch(ctx, req)
	searchDuration := time.Since(searchStart)

	trace.Steps = append(trace.Steps, DecisionStep{
		Step:       "semantic_search",
		Result:     fmt.Sprintf("found_%d_candidates", len(searchResults)),
		DurationMs: searchDuration.Milliseconds(),
		Metadata: map[string]interface{}{
			"candidates_found": len(searchResults),
			"target_system":    req.TargetSystem,
		},
	})

	if err != nil {
		return nil, fmt.Errorf("semantic search failed: %w", err)
	}

	if len(searchResults) == 0 {
		result := &SuggestResult{
			BestMatch:      nil,
			Alternates:     nil,
			Confidence:     0,
			Reasoning:      "No candidates found in semantic search",
			SearchDuration: searchDuration,
			TotalDuration:  time.Since(start),
			Trace:          trace,
		}
		return result, nil
	}

	// Check if top semantic match is strong enough to skip LLM
	if searchResults[0].Score >= e.config.SemanticAutoAcceptThreshold {
		trace.Steps = append(trace.Steps, DecisionStep{
			Step:       "semantic_auto_accept",
			Result:     fmt.Sprintf("accepted_%s", searchResults[0].Code),
			DurationMs: 0,
			Metadata: map[string]interface{}{
				"score":     searchResults[0].Score,
				"threshold": e.config.SemanticAutoAcceptThreshold,
			},
		})

		result := e.buildResultFromSemantic(searchResults, req, searchDuration, trace, start)

		if e.cache != nil {
			e.cache.Set(req, result)
		}
		return result, nil
	}

	// Step 2: LLM ranking
	rankStart := time.Now()
	rankResult, err := e.ranker.Rank(ctx, RankRequest{
		SourceCode:    req.SourceCode,
		SourceDisplay: req.SourceDisplay,
		SourceSystem:  req.SourceSystem,
		TargetSystem:  req.TargetSystem,
		Candidates:    searchResults,
		MaxResults:    req.MaxCandidates,
	})
	rankDuration := time.Since(rankStart)

	if err != nil {
		// Fall back to semantic results on LLM failure
		trace.Steps = append(trace.Steps, DecisionStep{
			Step:       "llm_ranking",
			Result:     "error_fallback_to_semantic",
			DurationMs: rankDuration.Milliseconds(),
			Metadata: map[string]interface{}{
				"error": err.Error(),
			},
		})

		result := e.buildResultFromSemantic(searchResults, req, searchDuration, trace, start)
		result.Reasoning = fmt.Sprintf("LLM ranking failed, using semantic search results: %v", err)

		if e.cache != nil {
			e.cache.Set(req, result)
		}
		return result, nil
	}

	trace.Steps = append(trace.Steps, DecisionStep{
		Step:       "llm_ranking",
		Result:     fmt.Sprintf("selected_%s", rankResult.Candidates[0].Code),
		DurationMs: rankDuration.Milliseconds(),
		Metadata: map[string]interface{}{
			"model":          rankResult.Model,
			"top_confidence": rankResult.TopConfidence,
			"candidates":     len(rankResult.Candidates),
		},
	})

	// Build final result
	result := &SuggestResult{
		Confidence:     rankResult.TopConfidence,
		Reasoning:      rankResult.Reasoning,
		Model:          rankResult.Model,
		SearchDuration: searchDuration,
		RankDuration:   rankDuration,
		TotalDuration:  time.Since(start),
		Trace:          trace,
	}

	if len(rankResult.Candidates) > 0 {
		result.BestMatch = &rankResult.Candidates[0]
		if len(rankResult.Candidates) > 1 {
			result.Alternates = rankResult.Candidates[1:]
		}
	}

	// Finalize trace
	if result.BestMatch != nil {
		trace.Result = &TraceResult{
			Code:        result.BestMatch.Code,
			Display:     result.BestMatch.Display,
			System:      result.BestMatch.System,
			Confidence:  result.Confidence,
			Equivalence: string(result.BestMatch.Equivalence),
		}
	}
	trace.Duration = Duration{
		SearchMs: searchDuration.Milliseconds(),
		RankMs:   rankDuration.Milliseconds(),
		TotalMs:  time.Since(start).Milliseconds(),
	}

	// Cache result
	if e.cache != nil {
		e.cache.Set(req, result)
	}

	return result, nil
}

// semanticSearch performs vector similarity search for candidate codes.
func (e *Engine) semanticSearch(ctx context.Context, req SuggestRequest) ([]semantic.SemanticMatch, error) {
	// Build search query from source code and display
	query := req.SourceCode
	if req.SourceDisplay != "" {
		query = req.SourceDisplay
	}

	// Determine vocabulary from target system
	vocab := vocabularyFromSystem(req.TargetSystem)

	opts := semantic.SearchOptions{
		MaxResults: e.config.SemanticTopK,
		MinScore:   e.config.SemanticMinScore,
	}

	// Use vocabulary-specific search if we have a known vocabulary
	switch vocab {
	case index.VocabularyLOINC:
		return e.searcher.SearchLOINC(ctx, query, opts.MaxResults)
	case index.VocabularySNOMED:
		return e.searcher.SearchSNOMED(ctx, query, opts.MaxResults)
	case index.VocabularyICD10CM:
		return e.searcher.SearchICD10(ctx, query, opts.MaxResults)
	default:
		// Generic search with vocabulary filter
		opts.Vocabularies = []index.Vocabulary{vocab}
		return e.searcher.Search(ctx, query, opts)
	}
}

// buildResultFromSemantic creates a SuggestResult from semantic search results only.
func (e *Engine) buildResultFromSemantic(
	matches []semantic.SemanticMatch,
	req SuggestRequest,
	searchDuration time.Duration,
	trace *DecisionTrace,
	start time.Time,
) *SuggestResult {
	result := &SuggestResult{
		SearchDuration: searchDuration,
		TotalDuration:  time.Since(start),
		Trace:          trace,
		Reasoning:      "Result based on semantic similarity (LLM ranking skipped or failed)",
	}

	if len(matches) > 0 {
		// Use semantic score as confidence (slightly reduced since no LLM validation)
		confidence := matches[0].Score * 0.9

		result.BestMatch = &Candidate{
			Code:       matches[0].Code,
			Display:    matches[0].Display,
			System:     matches[0].System,
			Confidence: confidence,
			Score:      matches[0].Score,
			Reasoning:  fmt.Sprintf("Semantic similarity: %.2f", matches[0].Score),
		}
		result.Confidence = confidence

		// Add alternates
		for i := 1; i < len(matches) && i < req.MaxCandidates; i++ {
			result.Alternates = append(result.Alternates, Candidate{
				Code:       matches[i].Code,
				Display:    matches[i].Display,
				System:     matches[i].System,
				Confidence: matches[i].Score * 0.9,
				Score:      matches[i].Score,
				Reasoning:  fmt.Sprintf("Semantic similarity: %.2f", matches[i].Score),
			})
		}

		trace.Result = &TraceResult{
			Code:       result.BestMatch.Code,
			Display:    result.BestMatch.Display,
			System:     result.BestMatch.System,
			Confidence: result.Confidence,
		}
	}

	trace.Duration = Duration{
		SearchMs: searchDuration.Milliseconds(),
		TotalMs:  time.Since(start).Milliseconds(),
	}

	return result
}

// vocabularyFromSystem maps a code system URI to a vocabulary constant.
func vocabularyFromSystem(system string) index.Vocabulary {
	switch system {
	case "http://loinc.org":
		return index.VocabularyLOINC
	case "http://snomed.info/sct":
		return index.VocabularySNOMED
	case "http://hl7.org/fhir/sid/icd-10-cm":
		return index.VocabularyICD10CM
	case "http://www.nlm.nih.gov/research/umls/rxnorm":
		return index.VocabularyRxNorm
	case "http://www.ama-assn.org/go/cpt":
		return index.VocabularyCPT
	case "http://hl7.org/fhir/sid/cvx":
		return index.VocabularyCVX
	default:
		// Return as-is, let search handle unknown vocabularies
		return index.Vocabulary(system)
	}
}

// InvalidateCache removes a cached result for a specific request.
func (e *Engine) InvalidateCache(req SuggestRequest) {
	if e.cache != nil {
		e.cache.Invalidate(req)
	}
}

// ClearCache removes all cached results.
func (e *Engine) ClearCache() {
	if e.cache != nil {
		e.cache.Clear()
	}
}

// CacheSize returns the current number of cached entries.
func (e *Engine) CacheSize() int {
	if e.cache != nil {
		return e.cache.Size()
	}
	return 0
}
