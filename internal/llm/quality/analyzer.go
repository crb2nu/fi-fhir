// Package quality provides LLM-powered data quality analysis for healthcare events.
package quality

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/llm"
)

// Analyzer provides LLM-powered data quality analysis.
type Analyzer struct {
	client llm.Client
	model  string
}

// AnalyzerConfig configures the quality analyzer.
type AnalyzerConfig struct {
	// Client is the LLM client to use.
	Client llm.Client

	// Model is the model to use for analysis.
	Model string
}

// NewAnalyzer creates a new data quality analyzer.
func NewAnalyzer(cfg AnalyzerConfig) (*Analyzer, error) {
	if cfg.Client == nil {
		return nil, fmt.Errorf("client is required")
	}

	return &Analyzer{
		client: cfg.Client,
		model:  cfg.Model,
	}, nil
}

// DataQualityScore contains the overall quality assessment.
type DataQualityScore struct {
	// OverallScore is the aggregate quality score (0.0-1.0).
	OverallScore float64 `json:"overall_score"`

	// Dimensions contains scores for each quality dimension.
	Dimensions map[string]float64 `json:"dimensions"`

	// Issues contains identified data quality issues.
	Issues []DataQualityIssue `json:"issues,omitempty"`

	// Recommendations contains actionable recommendations.
	Recommendations []QualityRecommendation `json:"recommendations,omitempty"`

	// Metadata contains analysis metadata.
	Metadata QualityMetadata `json:"metadata,omitempty"`
}

// DataQualityIssue represents a specific data quality issue.
type DataQualityIssue struct {
	// Dimension is the quality dimension affected.
	Dimension string `json:"dimension"`

	// Severity is the issue severity: low, medium, high, critical.
	Severity string `json:"severity"`

	// Field is the affected field path.
	Field string `json:"field,omitempty"`

	// Description explains the issue.
	Description string `json:"description"`

	// ActualValue is the problematic value.
	ActualValue string `json:"actual_value,omitempty"`

	// ExpectedValue or pattern.
	ExpectedValue string `json:"expected_value,omitempty"`
}

// QualityRecommendation provides actionable guidance.
type QualityRecommendation struct {
	// Priority indicates importance: 1 (highest) to 5 (lowest).
	Priority int `json:"priority"`

	// Category groups related recommendations.
	Category string `json:"category"`

	// Title is a short description.
	Title string `json:"title"`

	// Description provides detailed guidance.
	Description string `json:"description"`

	// Impact explains what fixing this would improve.
	Impact string `json:"impact,omitempty"`
}

// QualityMetadata contains analysis metadata.
type QualityMetadata struct {
	// AnalyzedAt is when the analysis was performed.
	AnalyzedAt time.Time `json:"analyzed_at"`

	// Model is the LLM model used.
	Model string `json:"model,omitempty"`

	// ProcessingTime is how long analysis took.
	ProcessingTime time.Duration `json:"processing_time,omitempty"`

	// EventType is the type of event analyzed.
	EventType string `json:"event_type,omitempty"`
}

// Quality dimension constants.
const (
	DimensionCompleteness = "completeness" // All required fields present
	DimensionAccuracy     = "accuracy"     // Values are correct/valid
	DimensionConsistency  = "consistency"  // Values are internally consistent
	DimensionConformance  = "conformance"  // Follows standards/specifications
	DimensionTimeliness   = "timeliness"   // Data is current/not stale
)

// AnalyzeEvent performs quality analysis on a healthcare event.
func (a *Analyzer) AnalyzeEvent(ctx context.Context, event interface{}, eventType events.EventType) (*DataQualityScore, error) {
	startTime := time.Now()

	// Convert event to JSON for analysis
	eventJSON, err := json.MarshalIndent(event, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal event: %w", err)
	}

	// Build prompts
	systemPrompt := buildQualitySystemPrompt()
	userPrompt := buildQualityUserPrompt(string(eventJSON), string(eventType))

	req := llm.CompletionRequest{
		Messages: []llm.Message{
			llm.SystemMessage(systemPrompt),
			llm.UserMessage(userPrompt),
		},
		Model:       a.model,
		Temperature: 0.2,
		MaxTokens:   2048,
	}

	rawJSON, err := a.client.CompleteStructured(ctx, req, "quality_analysis", qualitySchema)
	if err != nil {
		return nil, fmt.Errorf("analyze quality: %w", err)
	}

	var result DataQualityScore
	if err := json.Unmarshal(rawJSON, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	result.Metadata = QualityMetadata{
		AnalyzedAt:     time.Now(),
		Model:          a.model,
		ProcessingTime: time.Since(startTime),
		EventType:      string(eventType),
	}

	return &result, nil
}

// QuickScore performs a lightweight quality assessment without LLM.
func QuickScore(event interface{}) *DataQualityScore {
	score := &DataQualityScore{
		Dimensions: make(map[string]float64),
	}

	// Basic completeness check
	eventJSON, err := json.Marshal(event)
	if err != nil {
		score.OverallScore = 0
		return score
	}

	var data map[string]interface{}
	if err := json.Unmarshal(eventJSON, &data); err != nil {
		score.OverallScore = 0.5
		return score
	}

	// Count non-empty fields
	total := 0
	nonEmpty := 0
	countFields(data, &total, &nonEmpty)

	if total > 0 {
		score.Dimensions[DimensionCompleteness] = float64(nonEmpty) / float64(total)
	} else {
		score.Dimensions[DimensionCompleteness] = 0
	}

	score.OverallScore = score.Dimensions[DimensionCompleteness]

	return score
}

func countFields(data map[string]interface{}, total, nonEmpty *int) {
	for _, v := range data {
		*total++
		if v != nil {
			switch val := v.(type) {
			case string:
				if val != "" {
					*nonEmpty++
				}
			case map[string]interface{}:
				countFields(val, total, nonEmpty)
			case []interface{}:
				if len(val) > 0 {
					*nonEmpty++
				}
			default:
				*nonEmpty++
			}
		}
	}
}

func buildQualitySystemPrompt() string {
	return `You are a healthcare data quality analyst. Your task is to assess the quality of healthcare data events across multiple dimensions.

Evaluate the following quality dimensions:
1. COMPLETENESS: Are all required fields present? Are optional fields populated when they should be?
2. ACCURACY: Are values in the correct format? Do they pass validation rules?
3. CONSISTENCY: Are related fields consistent with each other? (e.g., dates in order, codes match descriptions)
4. CONFORMANCE: Does the data follow HL7/FHIR/EDI standards?
5. TIMELINESS: Is the data current? Are timestamps reasonable?

For each issue found:
- Assign a severity (low, medium, high, critical)
- Identify the affected field
- Describe what's wrong
- Suggest what the correct value should be

Provide actionable recommendations prioritized by impact.`
}

func buildQualityUserPrompt(eventJSON, eventType string) string {
	return fmt.Sprintf(`Analyze the data quality of this %s event:

%s

Provide a comprehensive quality assessment with:
1. Overall score (0.0-1.0)
2. Scores for each dimension
3. List of issues found
4. Recommendations for improvement`, eventType, eventJSON)
}

var qualitySchema = map[string]interface{}{
	"type": "object",
	"properties": map[string]interface{}{
		"overall_score": map[string]interface{}{
			"type":        "number",
			"description": "Overall quality score from 0.0 to 1.0",
		},
		"dimensions": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"completeness": map[string]interface{}{"type": "number"},
				"accuracy":     map[string]interface{}{"type": "number"},
				"consistency":  map[string]interface{}{"type": "number"},
				"conformance":  map[string]interface{}{"type": "number"},
				"timeliness":   map[string]interface{}{"type": "number"},
			},
		},
		"issues": map[string]interface{}{
			"type": "array",
			"items": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"dimension":      map[string]interface{}{"type": "string"},
					"severity":       map[string]interface{}{"type": "string"},
					"field":          map[string]interface{}{"type": "string"},
					"description":    map[string]interface{}{"type": "string"},
					"actual_value":   map[string]interface{}{"type": "string"},
					"expected_value": map[string]interface{}{"type": "string"},
				},
			},
		},
		"recommendations": map[string]interface{}{
			"type": "array",
			"items": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"priority":    map[string]interface{}{"type": "integer"},
					"category":    map[string]interface{}{"type": "string"},
					"title":       map[string]interface{}{"type": "string"},
					"description": map[string]interface{}{"type": "string"},
					"impact":      map[string]interface{}{"type": "string"},
				},
			},
		},
	},
	"required": []string{"overall_score", "dimensions"},
}
