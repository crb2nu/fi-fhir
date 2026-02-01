// Package explain provides LLM-powered explanations for parse warnings and workflows.
package explain

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/llm"
)

// WarningExplainer provides human-readable explanations for ParseWarnings.
type WarningExplainer struct {
	client llm.Client
	model  string
	cache  *explanationCache
}

// ExplainerConfig configures the warning explainer.
type ExplainerConfig struct {
	// Client is the LLM client to use.
	Client llm.Client

	// Model is the model to use for explanations.
	// If empty, uses the client's default model.
	Model string

	// EnableCache enables caching of explanations.
	EnableCache bool

	// CacheTTL is how long to cache results.
	CacheTTL time.Duration
}

// NewWarningExplainer creates a new warning explainer.
func NewWarningExplainer(cfg ExplainerConfig) (*WarningExplainer, error) {
	if cfg.Client == nil {
		return nil, fmt.Errorf("client is required")
	}

	var cache *explanationCache
	if cfg.EnableCache {
		ttl := cfg.CacheTTL
		if ttl == 0 {
			ttl = 24 * time.Hour
		}
		cache = newExplanationCache(ttl)
	}

	return &WarningExplainer{
		client: cfg.Client,
		model:  cfg.Model,
		cache:  cache,
	}, nil
}

// ExplainedWarning contains a ParseWarning with its explanation.
type ExplainedWarning struct {
	// Warning is the original ParseWarning.
	Warning events.ParseWarning `json:"warning"`

	// Explanation is the human-readable explanation.
	Explanation string `json:"explanation"`

	// FixSuggestion provides guidance on how to fix the issue.
	FixSuggestion string `json:"fix_suggestion,omitempty"`

	// Impact describes the potential impact of this warning.
	Impact string `json:"impact,omitempty"`

	// FromCache indicates if this explanation came from cache.
	FromCache bool `json:"from_cache,omitempty"`
}

// Explain generates an explanation for a single ParseWarning.
func (e *WarningExplainer) Explain(ctx context.Context, warning events.ParseWarning, format events.SourceFormat) (*ExplainedWarning, error) {
	// Check if we have a pre-computed template
	if explanation := e.getTemplateExplanation(warning); explanation != nil {
		return explanation, nil
	}

	// Check cache
	cacheKey := e.cacheKey(warning)
	if e.cache != nil {
		if cached, ok := e.cache.get(cacheKey); ok {
			cached.FromCache = true
			return cached, nil
		}
	}

	// Generate explanation using LLM
	systemPrompt := buildExplainerSystemPrompt()
	userPrompt := buildExplainerUserPrompt(warning, format)

	req := llm.CompletionRequest{
		Messages: []llm.Message{
			llm.SystemMessage(systemPrompt),
			llm.UserMessage(userPrompt),
		},
		Model:       e.model,
		Temperature: 0.3,
		MaxTokens:   512,
	}

	rawJSON, err := e.client.CompleteStructured(ctx, req, "warning_explanation", explanationSchema)
	if err != nil {
		return nil, fmt.Errorf("generate explanation: %w", err)
	}

	var resp explanationResponse
	if err := json.Unmarshal(rawJSON, &resp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	result := &ExplainedWarning{
		Warning:       warning,
		Explanation:   resp.Explanation,
		FixSuggestion: resp.FixSuggestion,
		Impact:        resp.Impact,
	}

	// Cache the result
	if e.cache != nil {
		e.cache.set(cacheKey, result)
	}

	return result, nil
}

// ExplainBatch generates explanations for multiple ParseWarnings.
func (e *WarningExplainer) ExplainBatch(ctx context.Context, warnings []events.ParseWarning, format events.SourceFormat) ([]ExplainedWarning, error) {
	if len(warnings) == 0 {
		return nil, nil
	}

	results := make([]ExplainedWarning, len(warnings))

	// Process each warning
	for i, w := range warnings {
		explained, err := e.Explain(ctx, w, format)
		if err != nil {
			// Return partial results with error info
			results[i] = ExplainedWarning{
				Warning:     w,
				Explanation: fmt.Sprintf("Failed to generate explanation: %v", err),
			}
			continue
		}
		results[i] = *explained
	}

	return results, nil
}

// getTemplateExplanation returns a pre-computed explanation for common warning codes.
func (e *WarningExplainer) getTemplateExplanation(w events.ParseWarning) *ExplainedWarning {
	templates := getWarningTemplates()

	key := w.Code
	if tmpl, ok := templates[key]; ok {
		return &ExplainedWarning{
			Warning:       w,
			Explanation:   tmpl.Explanation,
			FixSuggestion: tmpl.FixSuggestion,
			Impact:        tmpl.Impact,
			FromCache:     true,
		}
	}

	return nil
}

// cacheKey generates a cache key for a warning.
func (e *WarningExplainer) cacheKey(w events.ParseWarning) string {
	return fmt.Sprintf("%s:%s:%s", w.Code, w.Phase, w.Severity)
}

// explanationResponse is the LLM response structure.
type explanationResponse struct {
	Explanation   string `json:"explanation"`
	FixSuggestion string `json:"fix_suggestion,omitempty"`
	Impact        string `json:"impact,omitempty"`
}

var explanationSchema = map[string]interface{}{
	"type": "object",
	"properties": map[string]interface{}{
		"explanation": map[string]interface{}{
			"type":        "string",
			"description": "Human-readable explanation of the warning",
		},
		"fix_suggestion": map[string]interface{}{
			"type":        "string",
			"description": "How to fix or address this warning",
		},
		"impact": map[string]interface{}{
			"type":        "string",
			"description": "Potential impact if this warning is ignored",
		},
	},
	"required": []string{"explanation"},
}

func buildExplainerSystemPrompt() string {
	return `You are a healthcare data integration expert. Your task is to explain parsing warnings in plain English for analysts who may not have deep expertise in HL7v2, FHIR, or EDI formats.

Guidelines:
1. Use clear, non-technical language where possible
2. Explain what the warning means in practical terms
3. Provide actionable suggestions for fixing the issue
4. Describe the potential impact on downstream systems
5. Keep explanations concise (2-3 sentences)
6. Reference the specific field/segment when applicable`
}

func buildExplainerUserPrompt(w events.ParseWarning, format events.SourceFormat) string {
	var sb strings.Builder
	sb.WriteString("Explain this parsing warning:\n\n")
	sb.WriteString(fmt.Sprintf("Format: %s\n", format))
	sb.WriteString(fmt.Sprintf("Phase: %s\n", w.Phase))
	sb.WriteString(fmt.Sprintf("Code: %s\n", w.Code))
	sb.WriteString(fmt.Sprintf("Message: %s\n", w.Message))
	if w.Path != "" {
		sb.WriteString(fmt.Sprintf("Path: %s\n", w.Path))
	}
	if w.Severity != "" {
		sb.WriteString(fmt.Sprintf("Severity: %s\n", w.Severity))
	}
	return sb.String()
}

// warningTemplate holds a pre-computed explanation template.
type warningTemplate struct {
	Explanation   string
	FixSuggestion string
	Impact        string
}

// getWarningTemplates returns pre-computed explanations for common warning codes.
func getWarningTemplates() map[string]warningTemplate {
	return map[string]warningTemplate{
		"INVALID_NPI": {
			Explanation:   "The National Provider Identifier (NPI) failed validation. NPIs are 10-digit numbers with a Luhn check digit. The provided value either has an incorrect length or an invalid check digit.",
			FixSuggestion: "Verify the NPI in the source system. Use the NPI Registry (nppes.cms.hhs.gov) to look up the correct NPI for this provider.",
			Impact:        "Claims and transactions using this NPI may be rejected by payers. Provider matching may fail.",
		},
		"INVALID_MBI": {
			Explanation:   "The Medicare Beneficiary Identifier (MBI) failed validation. MBIs follow a specific format: 11 characters with specific letter/number patterns.",
			FixSuggestion: "Verify the MBI format matches the CMS specification. Check for transcription errors or leading/trailing spaces.",
			Impact:        "Medicare claims using this identifier may be rejected. Patient matching may fail for Medicare beneficiaries.",
		},
		"INVALID_SSN": {
			Explanation:   "The Social Security Number (SSN) failed validation. SSNs must be 9 digits and cannot start with 9, have 000 in the first group, or have 00 in the second group.",
			FixSuggestion: "Verify the SSN in the source system. Consider if SSN is actually required for this use case.",
			Impact:        "Patient matching using SSN may fail. Some downstream systems may reject records with invalid SSNs.",
		},
		"MISSING_PV1": {
			Explanation:   "The HL7v2 message is missing the PV1 (Patient Visit) segment. This segment contains important encounter information like patient class, location, and attending physician.",
			FixSuggestion: "Check if PV1 is optional for this message type in your interface specification. If required, contact the sending system to include it.",
			Impact:        "Encounter information will be incomplete. Patient class, location, and provider assignments may be missing from the event.",
		},
		"MISSING_PID": {
			Explanation:   "The HL7v2 message is missing the PID (Patient Identification) segment. This segment contains essential patient demographics and identifiers.",
			FixSuggestion: "This is usually a critical issue. Contact the sending system to include the PID segment.",
			Impact:        "Cannot process the message without patient identification. The event may need to be rejected.",
		},
		"EMPTY_MRN": {
			Explanation:   "The Medical Record Number (MRN) field is empty. MRNs are primary identifiers used to link patient records within a healthcare facility.",
			FixSuggestion: "Check PID-3 in the source message. Ensure the sending system is configured to include the MRN.",
			Impact:        "Patient matching and record linkage will be impaired. May cause duplicate records or matching failures.",
		},
		"INVALID_DATE": {
			Explanation:   "A date/time field contains an invalid or unparseable value. HL7v2 dates should follow YYYYMMDD or YYYYMMDDHHMM formats.",
			FixSuggestion: "Check the date format in the source message. Common issues include: wrong separators, invalid month/day values, or timezone formatting.",
			Impact:        "The date will be missing or defaulted. This may affect clinical timelines, billing periods, or regulatory reporting.",
		},
		"UNKNOWN_MESSAGE_TYPE": {
			Explanation:   "The message type in MSH-9 is not recognized or not supported by the current configuration.",
			FixSuggestion: "Verify the message type is correct. Check if this message type needs to be added to the interface configuration.",
			Impact:        "The message may be routed incorrectly or dropped. Configure handling for this message type if it's expected.",
		},
		"SEGMENT_TRUNCATED": {
			Explanation:   "A segment appears to be truncated or cut off. This often happens due to buffer limits or transmission issues.",
			FixSuggestion: "Check the source system for buffer size limits. Review the transmission log for network issues.",
			Impact:        "Data in the truncated portion is lost. Fields after the truncation point will be missing.",
		},
		"DUPLICATE_IDENTIFIER": {
			Explanation:   "Multiple identifiers of the same type were found where only one was expected. This may indicate data quality issues or incorrect field repetition.",
			FixSuggestion: "Review the source data to determine which identifier is correct. Check if multiple identifiers are intentional (e.g., merged patients).",
			Impact:        "May cause confusion in downstream systems. Patient matching may produce inconsistent results.",
		},
		"CODE_NOT_MAPPED": {
			Explanation:   "A local code could not be mapped to a standard code system (LOINC, SNOMED, ICD-10, etc.).",
			FixSuggestion: "Add a mapping for this code in the terminology mapping table. Consider if the code is valid or a typo.",
			Impact:        "The standard code will be empty. Interoperability with other systems may be affected.",
		},
	}
}

// explanationCache provides caching for explanations.
type explanationCache struct {
	entries map[string]*cacheEntry
	ttl     time.Duration
	mu      sync.RWMutex
}

type cacheEntry struct {
	result    *ExplainedWarning
	expiresAt time.Time
}

func newExplanationCache(ttl time.Duration) *explanationCache {
	return &explanationCache{
		entries: make(map[string]*cacheEntry),
		ttl:     ttl,
	}
}

func (c *explanationCache) get(key string) (*ExplainedWarning, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.entries[key]
	if !ok || time.Now().After(entry.expiresAt) {
		return nil, false
	}
	return entry.result, true
}

func (c *explanationCache) set(key string, result *ExplainedWarning) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[key] = &cacheEntry{
		result:    result,
		expiresAt: time.Now().Add(c.ttl),
	}
}
