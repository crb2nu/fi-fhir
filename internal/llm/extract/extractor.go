package extract

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/llm"
)

// Extractor provides LLM-powered clinical entity extraction.
type Extractor struct {
	client llm.Client
	model  string
	cache  *extractionCache
	mu     sync.RWMutex
}

// Config configures the extractor.
type Config struct {
	// Client is the LLM client to use.
	Client llm.Client

	// Model is the model to use for extraction.
	// If empty, uses the client's quality model.
	Model string

	// EnableCache enables caching of extraction results.
	EnableCache bool

	// CacheTTL is how long to cache results.
	CacheTTL time.Duration
}

// NewExtractor creates a new clinical entity extractor.
func NewExtractor(cfg Config) (*Extractor, error) {
	if cfg.Client == nil {
		return nil, fmt.Errorf("client is required")
	}

	// Get quality model from client if available
	model := cfg.Model
	if model == "" {
		if qm, ok := cfg.Client.(llm.ClientWithQualityModel); ok {
			model = qm.QualityModel()
		}
	}

	var cache *extractionCache
	if cfg.EnableCache {
		ttl := cfg.CacheTTL
		if ttl == 0 {
			ttl = 1 * time.Hour
		}
		cache = newExtractionCache(ttl)
	}

	return &Extractor{
		client: cfg.Client,
		model:  model,
		cache:  cache,
	}, nil
}

// Extract extracts clinical entities from the given text.
func (e *Extractor) Extract(ctx context.Context, text string, opts ExtractionOptions) (*ExtractionResult, error) {
	if text == "" {
		return &ExtractionResult{Confidence: 0}, nil
	}

	startTime := time.Now()

	// Check cache
	if e.cache != nil {
		cacheKey := e.cacheKey(text, opts)
		if cached, ok := e.cache.get(cacheKey); ok {
			return cached, nil
		}
	}

	// Build prompts
	systemPrompt := buildSystemPrompt()
	userPrompt := buildExtractionPrompt(text, opts)

	// Create completion request
	req := llm.CompletionRequest{
		Messages: []llm.Message{
			llm.SystemMessage(systemPrompt),
			llm.UserMessage(userPrompt),
		},
		Model:       e.model,
		Temperature: 0.1, // Low temperature for deterministic extraction
		MaxTokens:   opts.MaxTokens,
	}
	if req.MaxTokens == 0 {
		req.MaxTokens = 4096
	}

	// Execute extraction with structured output
	rawJSON, err := e.client.CompleteStructured(ctx, req, "clinical_extraction", extractionSchema)
	if err != nil {
		return nil, fmt.Errorf("extraction failed: %w", err)
	}

	// Parse the response
	result, err := e.parseExtractionResponse(rawJSON, opts)
	if err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	// Set metadata
	result.ProcessingTime = time.Since(startTime)
	result.Model = e.model
	result.Metadata = ExtractionMetadata{
		DocumentType: opts.DocumentType,
		TextLength:   len(text),
		ExtractedAt:  time.Now(),
	}

	// Cache result
	if e.cache != nil {
		cacheKey := e.cacheKey(text, opts)
		e.cache.set(cacheKey, result)
	}

	return result, nil
}

// extractionResponse is the raw JSON response from the LLM.
type extractionResponse struct {
	Conditions        []rawCondition  `json:"conditions,omitempty"`
	Medications       []rawMedication `json:"medications,omitempty"`
	VitalSigns        []rawVitalSign  `json:"vital_signs,omitempty"`
	Allergies         []rawAllergy    `json:"allergies,omitempty"`
	Procedures        []rawProcedure  `json:"procedures,omitempty"`
	OverallConfidence float64         `json:"overall_confidence"`
}

type rawCondition struct {
	Name       string  `json:"name"`
	Code       string  `json:"code,omitempty"`
	CodeSystem string  `json:"code_system,omitempty"`
	Status     string  `json:"status,omitempty"`
	Confidence float64 `json:"confidence"`
	Negated    bool    `json:"negated,omitempty"`
	TextSpan   string  `json:"text_span,omitempty"`
	OnsetDate  string  `json:"onset_date,omitempty"`
	Severity   string  `json:"severity,omitempty"`
}

type rawMedication struct {
	Name       string  `json:"name"`
	Code       string  `json:"code,omitempty"`
	CodeSystem string  `json:"code_system,omitempty"`
	Dose       string  `json:"dose,omitempty"`
	Route      string  `json:"route,omitempty"`
	Frequency  string  `json:"frequency,omitempty"`
	Confidence float64 `json:"confidence"`
	Negated    bool    `json:"negated,omitempty"`
	TextSpan   string  `json:"text_span,omitempty"`
}

type rawVitalSign struct {
	Name       string  `json:"name"`
	LOINCCode  string  `json:"loinc_code,omitempty"`
	Value      string  `json:"value"`
	Unit       string  `json:"unit,omitempty"`
	Confidence float64 `json:"confidence"`
	TextSpan   string  `json:"text_span,omitempty"`
}

type rawAllergy struct {
	Name       string  `json:"name"`
	Code       string  `json:"code,omitempty"`
	CodeSystem string  `json:"code_system,omitempty"`
	Type       string  `json:"type,omitempty"`
	Severity   string  `json:"severity,omitempty"`
	Reaction   string  `json:"reaction,omitempty"`
	Confidence float64 `json:"confidence"`
	Negated    bool    `json:"negated,omitempty"`
	TextSpan   string  `json:"text_span,omitempty"`
}

type rawProcedure struct {
	Name       string  `json:"name"`
	Code       string  `json:"code,omitempty"`
	CodeSystem string  `json:"code_system,omitempty"`
	Status     string  `json:"status,omitempty"`
	Date       string  `json:"date,omitempty"`
	Confidence float64 `json:"confidence"`
	Negated    bool    `json:"negated,omitempty"`
	TextSpan   string  `json:"text_span,omitempty"`
}

// parseExtractionResponse parses the raw JSON response into an ExtractionResult.
func (e *Extractor) parseExtractionResponse(rawJSON json.RawMessage, opts ExtractionOptions) (*ExtractionResult, error) {
	var resp extractionResponse
	if err := json.Unmarshal(rawJSON, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	result := &ExtractionResult{
		Confidence: resp.OverallConfidence,
	}

	// Convert conditions
	for _, c := range resp.Conditions {
		if !opts.IncludeNegated && c.Negated {
			continue
		}
		if c.Confidence < opts.MinConfidence {
			continue
		}
		result.Conditions = append(result.Conditions, events.Condition{
			Name:       c.Name,
			Code:       c.Code,
			CodeSystem: c.CodeSystem,
			Category:   c.Status, // Map status to category
		})
	}

	// Convert medications
	for _, m := range resp.Medications {
		if !opts.IncludeNegated && m.Negated {
			continue
		}
		if m.Confidence < opts.MinConfidence {
			continue
		}
		result.Medications = append(result.Medications, events.Medication{
			Name:       m.Name,
			Code:       m.Code,
			CodeSystem: m.CodeSystem,
			Strength:   m.Dose,  // Map dose to strength
			Form:       m.Route, // Map route to form (approximation)
		})
	}

	// Convert vital signs
	for _, v := range resp.VitalSigns {
		if v.Confidence < opts.MinConfidence {
			continue
		}
		result.VitalSigns = append(result.VitalSigns, events.VitalSign{
			Name:      v.Name,
			LOINCCode: v.LOINCCode,
			Value:     v.Value,
			Unit:      v.Unit,
		})
	}

	// Convert allergies
	for _, a := range resp.Allergies {
		if !opts.IncludeNegated && a.Negated {
			continue
		}
		if a.Confidence < opts.MinConfidence {
			continue
		}
		allergy := events.AllergyIntolerance{
			Name:        a.Name,
			Code:        a.Code,
			CodeSystem:  a.CodeSystem,
			Type:        a.Type,
			Criticality: a.Severity, // Map severity to criticality
		}
		if a.Reaction != "" {
			allergy.Reactions = []events.AllergyReaction{
				{ManifestationText: a.Reaction},
			}
		}
		result.Allergies = append(result.Allergies, allergy)
	}

	// Convert procedures
	for _, p := range resp.Procedures {
		if !opts.IncludeNegated && p.Negated {
			continue
		}
		if p.Confidence < opts.MinConfidence {
			continue
		}
		result.Procedures = append(result.Procedures, events.Procedure{
			Name:       p.Name,
			Code:       p.Code,
			CodeSystem: p.CodeSystem,
			Status:     p.Status,
		})
	}

	return result, nil
}

// cacheKey generates a cache key for the extraction request.
func (e *Extractor) cacheKey(text string, opts ExtractionOptions) string {
	// Simple hash-like key based on text length and options
	return fmt.Sprintf("%d:%s:%t:%t:%t:%t:%t",
		len(text),
		opts.DocumentType,
		opts.ExtractConditions,
		opts.ExtractMedications,
		opts.ExtractVitalSigns,
		opts.ExtractAllergies,
		opts.ExtractProcedures,
	)
}

// extractionCache provides simple in-memory caching for extraction results.
type extractionCache struct {
	entries map[string]*cacheEntry
	ttl     time.Duration
	mu      sync.RWMutex
}

type cacheEntry struct {
	result    *ExtractionResult
	expiresAt time.Time
}

func newExtractionCache(ttl time.Duration) *extractionCache {
	return &extractionCache{
		entries: make(map[string]*cacheEntry),
		ttl:     ttl,
	}
}

func (c *extractionCache) get(key string) (*ExtractionResult, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.entries[key]
	if !ok {
		return nil, false
	}

	if time.Now().After(entry.expiresAt) {
		return nil, false
	}

	return entry.result, true
}

func (c *extractionCache) set(key string, result *ExtractionResult) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[key] = &cacheEntry{
		result:    result,
		expiresAt: time.Now().Add(c.ttl),
	}

	// Simple cleanup: remove expired entries if cache is getting large
	if len(c.entries) > 1000 {
		now := time.Now()
		for k, v := range c.entries {
			if now.After(v.expiresAt) {
				delete(c.entries, k)
			}
		}
	}
}

// ExtractFromDocument extracts entities from a DocumentEvent.
func (e *Extractor) ExtractFromDocument(ctx context.Context, doc *events.DocumentEvent, opts ExtractionOptions) (*ExtractionResult, error) {
	if doc == nil {
		return nil, fmt.Errorf("document is nil")
	}

	// Get document text
	text := doc.Content
	if text == "" && doc.ContentEncoding == "base64" {
		// Content might be base64 encoded - skip for now
		return nil, fmt.Errorf("base64 encoded content not supported")
	}

	if text == "" {
		return &ExtractionResult{Confidence: 0}, nil
	}

	// Set document type from doc if not specified
	if opts.DocumentType == "" {
		opts.DocumentType = doc.DocumentType
	}

	return e.Extract(ctx, text, opts)
}
