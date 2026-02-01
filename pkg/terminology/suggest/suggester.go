// Package suggest provides intelligent code suggestion for mapping unknown terminology codes.
package suggest

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/llm"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/terminology/index"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/terminology/semantic"
)

// Suggester provides intelligent suggestions for mapping unknown codes.
type Suggester struct {
	semanticSearcher *semantic.Searcher
	llmClient        llm.Client
	feedbackStore    *FeedbackStore
	config           *SuggesterConfig
}

// NewSuggester creates a new code suggester.
func NewSuggester(cfg SuggesterConfig) (*Suggester, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	// Create semantic searcher
	semanticCfg := semantic.SearchConfig{
		QdrantURL:           cfg.QdrantURL,
		QdrantAPIKey:        cfg.QdrantAPIKey,
		EmbeddingBaseURL:    cfg.EmbeddingBaseURL,
		EmbeddingAPIKey:     cfg.EmbeddingAPIKey,
		EmbeddingModel:      cfg.EmbeddingModel,
		EmbeddingDimensions: cfg.EmbeddingDimensions,
		DefaultMaxResults:   10,
		DefaultMinScore:     0.5,
		Timeout:             cfg.Timeout,
		EnableCache:         true,
	}

	semanticSearcher, err := semantic.NewSearcher(semanticCfg)
	if err != nil {
		return nil, fmt.Errorf("create semantic searcher: %w", err)
	}

	// Create LLM client for reasoning
	llmCfg := llm.Config{
		BaseURL:      cfg.LLMBaseURL,
		APIKey:       cfg.LLMAPIKey,
		DefaultModel: cfg.LLMModel,
		Timeout:      cfg.Timeout,
	}

	llmClient, err := llm.New(llmCfg)
	if err != nil {
		return nil, fmt.Errorf("create llm client: %w", err)
	}

	// Create feedback store
	var feedbackStore *FeedbackStore
	if cfg.EnableFeedback {
		feedbackStore = NewFeedbackStore(FeedbackStoreConfig{
			QdrantURL:    cfg.QdrantURL,
			QdrantAPIKey: cfg.QdrantAPIKey,
			Collection:   cfg.FeedbackCollection,
		})
	}

	return &Suggester{
		semanticSearcher: semanticSearcher,
		llmClient:        llmClient,
		feedbackStore:    feedbackStore,
		config:           &cfg,
	}, nil
}

// Suggestion represents a suggested code mapping.
type Suggestion struct {
	// Code is the suggested terminology code.
	Code string `json:"code"`

	// Display is the display name.
	Display string `json:"display"`

	// System is the code system URI.
	System string `json:"system"`

	// Vocabulary is the vocabulary type.
	Vocabulary index.Vocabulary `json:"vocabulary"`

	// Confidence is the confidence score (0-1).
	Confidence float64 `json:"confidence"`

	// Rationale explains why this suggestion was made.
	Rationale string `json:"rationale,omitempty"`

	// Strategy indicates which strategy produced this suggestion.
	Strategy SuggestionStrategy `json:"strategy"`

	// Metadata contains additional information about the match.
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// SuggestionStrategy indicates how a suggestion was generated.
type SuggestionStrategy string

const (
	StrategySemantic  SuggestionStrategy = "semantic"
	StrategyFeedback  SuggestionStrategy = "feedback"
	StrategyLLM       SuggestionStrategy = "llm"
	StrategyCrossWalk SuggestionStrategy = "crosswalk"
	StrategyHybrid    SuggestionStrategy = "hybrid"
)

// SuggestRequest contains parameters for a suggestion request.
type SuggestRequest struct {
	// LocalCode is the local/source code to find a mapping for.
	LocalCode string `json:"local_code"`

	// DisplayText is the display name or description of the local code.
	DisplayText string `json:"display_text"`

	// SourceSystem identifies the source of the local code.
	SourceSystem string `json:"source_system,omitempty"`

	// TargetVocabulary is the desired target vocabulary.
	TargetVocabulary index.Vocabulary `json:"target_vocabulary"`

	// MaxResults limits the number of suggestions.
	MaxResults int `json:"max_results,omitempty"`

	// MinConfidence sets the minimum confidence threshold.
	MinConfidence float64 `json:"min_confidence,omitempty"`

	// Context provides additional context for better suggestions.
	Context *SuggestionContext `json:"context,omitempty"`
}

// SuggestionContext provides additional context to improve suggestions.
type SuggestionContext struct {
	// PatientAge can help disambiguate pediatric vs adult codes.
	PatientAge int `json:"patient_age,omitempty"`

	// ClinicalDomain helps narrow down the search space.
	ClinicalDomain string `json:"clinical_domain,omitempty"`

	// SpecimenType for lab codes.
	SpecimenType string `json:"specimen_type,omitempty"`

	// RelatedCodes are other codes in the same message/encounter.
	RelatedCodes []string `json:"related_codes,omitempty"`

	// ExistingMappings are known mappings from the same source.
	ExistingMappings map[string]string `json:"existing_mappings,omitempty"`
}

// Suggest generates mapping suggestions for an unknown code.
func (s *Suggester) Suggest(ctx context.Context, req SuggestRequest) ([]Suggestion, error) {
	// Set defaults
	if req.MaxResults == 0 {
		req.MaxResults = s.config.DefaultMaxResults
	}
	if req.MinConfidence == 0 {
		req.MinConfidence = s.config.DefaultMinConfidence
	}

	var allSuggestions []Suggestion

	// Strategy 1: Check feedback store for previously accepted mappings
	if s.feedbackStore != nil && s.config.EnableFeedback {
		feedbackSuggestions, err := s.suggestFromFeedback(ctx, req)
		if err == nil && len(feedbackSuggestions) > 0 {
			allSuggestions = append(allSuggestions, feedbackSuggestions...)

			// If we have a high-confidence feedback match, return early
			for _, sug := range feedbackSuggestions {
				if sug.Confidence >= 0.95 {
					return []Suggestion{sug}, nil
				}
			}
		}
	}

	// Strategy 2: Semantic search
	semanticSuggestions, err := s.suggestFromSemantic(ctx, req)
	if err == nil {
		allSuggestions = append(allSuggestions, semanticSuggestions...)
	}

	// Strategy 3: LLM reasoning (if semantic results are insufficient)
	if s.config.EnableLLMReasoning {
		hasHighConfidence := false
		for _, sug := range allSuggestions {
			if sug.Confidence >= 0.85 {
				hasHighConfidence = true
				break
			}
		}

		if !hasHighConfidence {
			llmSuggestions, err := s.suggestFromLLM(ctx, req, allSuggestions)
			if err == nil {
				allSuggestions = append(allSuggestions, llmSuggestions...)
			}
		}
	}

	// Deduplicate and rank
	allSuggestions = s.deduplicateAndRank(allSuggestions)

	// Filter by minimum confidence
	var filtered []Suggestion
	for _, sug := range allSuggestions {
		if sug.Confidence >= req.MinConfidence {
			filtered = append(filtered, sug)
		}
	}

	// Limit results
	if len(filtered) > req.MaxResults {
		filtered = filtered[:req.MaxResults]
	}

	return filtered, nil
}

// suggestFromFeedback looks for suggestions based on previously accepted mappings.
func (s *Suggester) suggestFromFeedback(ctx context.Context, req SuggestRequest) ([]Suggestion, error) {
	if s.feedbackStore == nil {
		return nil, nil
	}

	// Look for exact code matches first
	feedbacks, err := s.feedbackStore.FindBySourceCode(ctx, req.LocalCode, req.SourceSystem)
	if err != nil {
		return nil, err
	}

	var suggestions []Suggestion
	for _, fb := range feedbacks {
		if fb.Accepted && fb.TargetVocabulary == req.TargetVocabulary {
			suggestions = append(suggestions, Suggestion{
				Code:       fb.AcceptedCode,
				Display:    fb.AcceptedDisplay,
				System:     semantic.VocabularyCodeSystemURL(fb.TargetVocabulary),
				Vocabulary: fb.TargetVocabulary,
				Confidence: 0.95 + (float64(fb.AcceptCount) * 0.01), // Boost with accept count
				Rationale:  fmt.Sprintf("Previously accepted mapping (accepted %d times)", fb.AcceptCount),
				Strategy:   StrategyFeedback,
			})
		}
	}

	// Also check similar display text
	if req.DisplayText != "" && len(suggestions) == 0 {
		similarFeedbacks, err := s.feedbackStore.FindSimilar(ctx, req.DisplayText, req.TargetVocabulary, 5)
		if err == nil {
			for _, fb := range similarFeedbacks {
				if fb.Accepted {
					suggestions = append(suggestions, Suggestion{
						Code:       fb.AcceptedCode,
						Display:    fb.AcceptedDisplay,
						System:     semantic.VocabularyCodeSystemURL(fb.TargetVocabulary),
						Vocabulary: fb.TargetVocabulary,
						Confidence: 0.7 + (fb.Similarity * 0.2),
						Rationale:  fmt.Sprintf("Similar to accepted mapping for '%s'", fb.SourceDisplay),
						Strategy:   StrategyFeedback,
					})
				}
			}
		}
	}

	return suggestions, nil
}

// suggestFromSemantic uses semantic search to find suggestions.
func (s *Suggester) suggestFromSemantic(ctx context.Context, req SuggestRequest) ([]Suggestion, error) {
	// Build search query
	query := req.DisplayText
	if query == "" {
		query = req.LocalCode
	}

	// Add context to query if available
	if req.Context != nil {
		if req.Context.ClinicalDomain != "" {
			query += " " + req.Context.ClinicalDomain
		}
		if req.Context.SpecimenType != "" {
			query += " " + req.Context.SpecimenType
		}
	}

	// Search
	opts := semantic.SearchOptions{
		Vocabularies: []index.Vocabulary{req.TargetVocabulary},
		MaxResults:   req.MaxResults * 2, // Get extra to filter
		MinScore:     0.4,
	}

	matches, err := s.semanticSearcher.Search(ctx, query, opts)
	if err != nil {
		return nil, fmt.Errorf("semantic search: %w", err)
	}

	// Convert to suggestions
	var suggestions []Suggestion
	for _, m := range matches {
		suggestions = append(suggestions, Suggestion{
			Code:       m.Code,
			Display:    m.Display,
			System:     m.System,
			Vocabulary: m.Vocabulary,
			Confidence: m.Score,
			Rationale:  fmt.Sprintf("Semantic similarity: %.0f%%", m.Score*100),
			Strategy:   StrategySemantic,
			Metadata:   m.Metadata,
		})
	}

	return suggestions, nil
}

// suggestFromLLM uses LLM reasoning for complex cases.
func (s *Suggester) suggestFromLLM(ctx context.Context, req SuggestRequest, existing []Suggestion) ([]Suggestion, error) {
	systemPrompt := buildSuggesterSystemPrompt(req.TargetVocabulary)
	userPrompt := buildSuggesterUserPrompt(req, existing)

	llmReq := llm.CompletionRequest{
		Messages: []llm.Message{
			llm.SystemMessage(systemPrompt),
			llm.UserMessage(userPrompt),
		},
		Model:       s.config.LLMModel,
		Temperature: 0.2,
		MaxTokens:   1024,
	}

	rawJSON, err := s.llmClient.CompleteStructured(ctx, llmReq, "code_suggestions", suggesterSchema)
	if err != nil {
		return nil, fmt.Errorf("llm completion: %w", err)
	}

	var resp llmSuggestionResponse
	if err := json.Unmarshal(rawJSON, &resp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	var suggestions []Suggestion
	for _, s := range resp.Suggestions {
		suggestions = append(suggestions, Suggestion{
			Code:       s.Code,
			Display:    s.Display,
			System:     semantic.VocabularyCodeSystemURL(req.TargetVocabulary),
			Vocabulary: req.TargetVocabulary,
			Confidence: s.Confidence,
			Rationale:  s.Rationale,
			Strategy:   StrategyLLM,
		})
	}

	return suggestions, nil
}

// deduplicateAndRank removes duplicates and ranks suggestions.
func (s *Suggester) deduplicateAndRank(suggestions []Suggestion) []Suggestion {
	seen := make(map[string]int) // code -> index
	var result []Suggestion

	for _, sug := range suggestions {
		if idx, exists := seen[sug.Code]; exists {
			// Keep higher confidence
			if sug.Confidence > result[idx].Confidence {
				result[idx] = sug
			} else if sug.Strategy == StrategyFeedback {
				// Prefer feedback strategy
				result[idx].Confidence = result[idx].Confidence * 1.1 // Boost
				if result[idx].Confidence > 1.0 {
					result[idx].Confidence = 1.0
				}
			}
		} else {
			seen[sug.Code] = len(result)
			result = append(result, sug)
		}
	}

	// Sort by confidence descending
	sortSuggestions(result)

	return result
}

// sortSuggestions sorts suggestions by confidence descending.
func sortSuggestions(suggestions []Suggestion) {
	for i := 1; i < len(suggestions); i++ {
		key := suggestions[i]
		j := i - 1
		for j >= 0 && suggestions[j].Confidence < key.Confidence {
			suggestions[j+1] = suggestions[j]
			j--
		}
		suggestions[j+1] = key
	}
}

// RecordFeedback records user feedback on a suggestion.
func (s *Suggester) RecordFeedback(ctx context.Context, fb Feedback) error {
	if s.feedbackStore == nil {
		return fmt.Errorf("feedback store not enabled")
	}
	return s.feedbackStore.Record(ctx, fb)
}

// GetSuggestionHistory returns the suggestion history for a source code.
func (s *Suggester) GetSuggestionHistory(ctx context.Context, sourceCode, sourceSystem string) ([]Feedback, error) {
	if s.feedbackStore == nil {
		return nil, fmt.Errorf("feedback store not enabled")
	}
	return s.feedbackStore.FindBySourceCode(ctx, sourceCode, sourceSystem)
}

// llmSuggestionResponse is the LLM response structure.
type llmSuggestionResponse struct {
	Suggestions []struct {
		Code       string  `json:"code"`
		Display    string  `json:"display"`
		Confidence float64 `json:"confidence"`
		Rationale  string  `json:"rationale"`
	} `json:"suggestions"`
}

var suggesterSchema = map[string]interface{}{
	"type": "object",
	"properties": map[string]interface{}{
		"suggestions": map[string]interface{}{
			"type": "array",
			"items": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"code": map[string]interface{}{
						"type":        "string",
						"description": "The suggested terminology code",
					},
					"display": map[string]interface{}{
						"type":        "string",
						"description": "The display name for the code",
					},
					"confidence": map[string]interface{}{
						"type":        "number",
						"description": "Confidence score from 0.0 to 1.0",
					},
					"rationale": map[string]interface{}{
						"type":        "string",
						"description": "Explanation for why this code was suggested",
					},
				},
				"required": []string{"code", "display", "confidence", "rationale"},
			},
		},
	},
	"required": []string{"suggestions"},
}

func buildSuggesterSystemPrompt(vocab index.Vocabulary) string {
	vocabInfo := ""
	switch vocab {
	case index.VocabularyLOINC:
		vocabInfo = `LOINC (Logical Observation Identifiers Names and Codes) is used for laboratory tests, clinical observations, and document types.
Key components: Component (analyte), Property, Time Aspect, System (specimen), Scale, Method.
Format: 5-7 digit numeric code with a check digit (e.g., "2345-7").`
	case index.VocabularySNOMED:
		vocabInfo = `SNOMED CT is a comprehensive clinical terminology covering clinical findings, procedures, body structures, substances, etc.
Concept IDs are numeric identifiers (e.g., "73211009" for Diabetes mellitus).`
	case index.VocabularyICD10CM:
		vocabInfo = `ICD-10-CM is used for diagnosis coding in healthcare.
Format: 3-7 alphanumeric characters (e.g., "E11.9" for Type 2 diabetes without complications).`
	}

	return fmt.Sprintf(`You are a healthcare terminology expert specializing in code mapping. Your task is to suggest appropriate %s codes for local/proprietary codes.

%s

Guidelines:
1. Only suggest codes that actually exist in the standard vocabulary
2. Provide confidence scores based on semantic match quality
3. Explain your reasoning for each suggestion
4. Consider clinical context when available
5. If no good match exists, suggest the most appropriate parent/broader code
6. Never invent or guess code values`, vocab.String(), vocabInfo)
}

func buildSuggesterUserPrompt(req SuggestRequest, existing []Suggestion) string {
	prompt := fmt.Sprintf(`Find the best %s code mapping for:

Local Code: %s
Display Text: %s`, req.TargetVocabulary.String(), req.LocalCode, req.DisplayText)

	if req.SourceSystem != "" {
		prompt += fmt.Sprintf("\nSource System: %s", req.SourceSystem)
	}

	if req.Context != nil {
		if req.Context.ClinicalDomain != "" {
			prompt += fmt.Sprintf("\nClinical Domain: %s", req.Context.ClinicalDomain)
		}
		if req.Context.SpecimenType != "" {
			prompt += fmt.Sprintf("\nSpecimen Type: %s", req.Context.SpecimenType)
		}
	}

	if len(existing) > 0 {
		prompt += "\n\nCandidate matches from semantic search (validate and refine):"
		for i, sug := range existing {
			if i >= 5 {
				break
			}
			prompt += fmt.Sprintf("\n- %s: %s (%.0f%% match)", sug.Code, sug.Display, sug.Confidence*100)
		}
	}

	prompt += "\n\nProvide your best suggestions with confidence scores and rationale."

	return prompt
}

// BatchSuggest generates suggestions for multiple codes.
func (s *Suggester) BatchSuggest(ctx context.Context, requests []SuggestRequest) (map[string][]Suggestion, error) {
	results := make(map[string][]Suggestion)

	for _, req := range requests {
		suggestions, err := s.Suggest(ctx, req)
		if err != nil {
			// Store empty result on error
			results[req.LocalCode] = nil
			continue
		}
		results[req.LocalCode] = suggestions
	}

	return results, nil
}

// AutoMap attempts to automatically map codes that have high-confidence suggestions.
type AutoMapResult struct {
	LocalCode   string      `json:"local_code"`
	Suggestion  *Suggestion `json:"suggestion,omitempty"`
	AutoMapped  bool        `json:"auto_mapped"`
	Confidence  float64     `json:"confidence"`
	NeedsReview bool        `json:"needs_review"`
}

// AutoMap attempts to automatically map a code if confidence is high enough.
func (s *Suggester) AutoMap(ctx context.Context, req SuggestRequest, autoMapThreshold float64) (*AutoMapResult, error) {
	suggestions, err := s.Suggest(ctx, req)
	if err != nil {
		return nil, err
	}

	result := &AutoMapResult{
		LocalCode: req.LocalCode,
	}

	if len(suggestions) == 0 {
		result.NeedsReview = true
		return result, nil
	}

	best := suggestions[0]
	result.Suggestion = &best
	result.Confidence = best.Confidence

	if best.Confidence >= autoMapThreshold {
		result.AutoMapped = true
	} else {
		result.NeedsReview = true
	}

	return result, nil
}

// SuggestionReport generates a report of suggestions for review.
type SuggestionReport struct {
	Generated    time.Time       `json:"generated"`
	TotalCodes   int             `json:"total_codes"`
	AutoMapped   int             `json:"auto_mapped"`
	NeedsReview  int             `json:"needs_review"`
	NoSuggestion int             `json:"no_suggestion"`
	Results      []AutoMapResult `json:"results"`
}

// GenerateReport creates a suggestion report for multiple codes.
func (s *Suggester) GenerateReport(ctx context.Context, requests []SuggestRequest, autoMapThreshold float64) (*SuggestionReport, error) {
	report := &SuggestionReport{
		Generated:  time.Now(),
		TotalCodes: len(requests),
	}

	for _, req := range requests {
		result, err := s.AutoMap(ctx, req, autoMapThreshold)
		if err != nil {
			report.NoSuggestion++
			continue
		}

		report.Results = append(report.Results, *result)

		if result.AutoMapped {
			report.AutoMapped++
		} else if result.NeedsReview {
			report.NeedsReview++
		} else {
			report.NoSuggestion++
		}
	}

	return report, nil
}
