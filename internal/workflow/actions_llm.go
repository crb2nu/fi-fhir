package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/llm/extract"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/llm/quality"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/llm"
)

// llmExtractAction extracts clinical entities from unstructured text using LLM.
//
// Config options:
//   - model: LLM model to use (optional, uses client's quality model if not specified)
//   - document_type: Type of document (progress_note, discharge_summary, etc.)
//   - min_confidence: Minimum confidence threshold (0.0-1.0, default: 0.5)
//   - text_field: JSON path to extract text from (default: tries common paths)
//   - enable_cache: "true" to enable caching (default: false)
//   - cache_ttl: Cache TTL duration (default: 1h)
//   - extract_conditions: "true" to extract conditions (default: true)
//   - extract_medications: "true" to extract medications (default: true)
//   - extract_vital_signs: "true" to extract vital signs (default: true)
//   - extract_allergies: "true" to extract allergies (default: true)
//   - extract_procedures: "true" to extract procedures (default: true)
func makeLLMExtractAction(client llm.Client) ContextActionHandlerFunc {
	return func(ctx context.Context, event interface{}, config map[string]string) error {
		if client == nil {
			return fmt.Errorf("LLM client not configured")
		}

		// Extract text from event
		text, err := extractTextFromEvent(event, config)
		if err != nil {
			return fmt.Errorf("extract text: %w", err)
		}

		if text == "" {
			// No text to extract from, skip silently
			return nil
		}

		// Parse configuration
		minConfidence := 0.5
		if mc := config["min_confidence"]; mc != "" {
			if parsed, err := strconv.ParseFloat(mc, 64); err == nil {
				minConfidence = parsed
			}
		}

		enableCache := strings.ToLower(config["enable_cache"]) == "true"
		cacheTTL := time.Hour
		if ttl := config["cache_ttl"]; ttl != "" {
			if parsed, err := time.ParseDuration(ttl); err == nil {
				cacheTTL = parsed
			}
		}

		// Create extractor
		extractor, err := extract.NewExtractor(extract.Config{
			Client:      client,
			Model:       config["model"],
			EnableCache: enableCache,
			CacheTTL:    cacheTTL,
		})
		if err != nil {
			return fmt.Errorf("create extractor: %w", err)
		}

		// Build extraction options
		opts := extract.ExtractionOptions{
			DocumentType:       config["document_type"],
			ExtractConditions:  parseOptBool(config["extract_conditions"], true),
			ExtractMedications: parseOptBool(config["extract_medications"], true),
			ExtractVitalSigns:  parseOptBool(config["extract_vital_signs"], true),
			ExtractAllergies:   parseOptBool(config["extract_allergies"], true),
			ExtractProcedures:  parseOptBool(config["extract_procedures"], true),
		}

		// Perform extraction
		result, err := extractor.Extract(ctx, text, opts)
		if err != nil {
			return fmt.Errorf("extraction failed: %w", err)
		}

		// Check confidence threshold
		if result.Confidence < minConfidence {
			// Below confidence threshold, don't add entities
			return nil
		}

		// Add extracted entities to event
		if err := addExtractedEntitiesToEvent(event, result); err != nil {
			return fmt.Errorf("add extracted entities: %w", err)
		}

		return nil
	}
}

// llmQualityCheckAction performs data quality analysis using LLM.
//
// Config options:
//   - model: LLM model to use (optional)
//   - fail_below: Fail if overall score is below this threshold (0.0-1.0)
//   - warn_below: Log warning if score is below this threshold (0.0-1.0)
//   - event_type: Override event type detection
func makeLLMQualityCheckAction(client llm.Client) ContextActionHandlerFunc {
	return func(ctx context.Context, event interface{}, config map[string]string) error {
		if client == nil {
			return fmt.Errorf("LLM client not configured")
		}

		// Create analyzer
		analyzer, err := quality.NewAnalyzer(quality.AnalyzerConfig{
			Client: client,
			Model:  config["model"],
		})
		if err != nil {
			return fmt.Errorf("create analyzer: %w", err)
		}

		// Determine event type
		eventType := events.EventType(config["event_type"])
		if eventType == "" {
			eventType = detectEventType(event)
		}

		// Perform analysis
		score, err := analyzer.AnalyzeEvent(ctx, event, eventType)
		if err != nil {
			return fmt.Errorf("quality analysis failed: %w", err)
		}

		// Add score to event
		if err := addQualityScoreToEvent(event, score); err != nil {
			return fmt.Errorf("add quality score: %w", err)
		}

		// Check thresholds
		if failBelow := config["fail_below"]; failBelow != "" {
			if threshold, err := strconv.ParseFloat(failBelow, 64); err == nil {
				if score.OverallScore < threshold {
					return fmt.Errorf("quality score %.2f below threshold %.2f", score.OverallScore, threshold)
				}
			}
		}

		// Warn threshold (non-fatal)
		if warnBelow := config["warn_below"]; warnBelow != "" {
			if threshold, err := strconv.ParseFloat(warnBelow, 64); err == nil {
				if score.OverallScore < threshold {
					// Log warning but don't fail
					fmt.Printf("Warning: quality score %.2f below warning threshold %.2f\n", score.OverallScore, threshold)
				}
			}
		}

		return nil
	}
}

// extractTextFromEvent extracts text content from an event for clinical extraction.
func extractTextFromEvent(event interface{}, config map[string]string) (string, error) {
	// Convert event to map for field access
	var eventMap map[string]interface{}

	switch e := event.(type) {
	case map[string]interface{}:
		eventMap = e
	default:
		// Marshal and unmarshal to get map representation
		data, err := json.Marshal(event)
		if err != nil {
			return "", fmt.Errorf("marshal event: %w", err)
		}
		if err := json.Unmarshal(data, &eventMap); err != nil {
			return "", fmt.Errorf("unmarshal event: %w", err)
		}
	}

	// Try configured text_field first
	if textField := config["text_field"]; textField != "" {
		if text := getNestedField(eventMap, textField); text != "" {
			return text, nil
		}
	}

	// Try common paths for clinical text
	commonPaths := []string{
		"document.content",
		"document.text",
		"content",
		"text",
		"narrative",
		"clinical_notes",
		"notes",
		"observation.note",
		"message_text",
	}

	for _, path := range commonPaths {
		if text := getNestedField(eventMap, path); text != "" {
			return text, nil
		}
	}

	return "", nil
}

// getNestedField retrieves a nested field value using dot notation.
func getNestedField(m map[string]interface{}, path string) string {
	parts := strings.Split(path, ".")
	current := m

	for i, part := range parts {
		if i == len(parts)-1 {
			// Last part - get value
			if val, ok := current[part]; ok {
				switch v := val.(type) {
				case string:
					return v
				default:
					// Try to convert to string
					if data, err := json.Marshal(v); err == nil {
						return string(data)
					}
				}
			}
			return ""
		}

		// Navigate deeper
		if next, ok := current[part].(map[string]interface{}); ok {
			current = next
		} else {
			return ""
		}
	}

	return ""
}

// addExtractedEntitiesToEvent adds extracted entities to the event's metadata.
func addExtractedEntitiesToEvent(event interface{}, result *extract.ExtractionResult) error {
	// Convert to map for modification
	eventMap, ok := event.(map[string]interface{})
	if !ok {
		// For typed events, we can't easily modify them
		// This would need event-specific handling
		return nil
	}

	// Build extracted entities structure
	extracted := events.ExtractedEntities{
		Confidence:  result.Confidence,
		ExtractedAt: time.Now(),
		Model:       result.Model,
	}

	// Convert conditions (events.Condition has flat fields)
	for _, c := range result.Conditions {
		extracted.Conditions = append(extracted.Conditions, events.ExtractedCondition{
			Name:       c.Name,
			Code:       c.Code,
			CodeSystem: c.CodeSystem,
			Status:     c.Category,        // Map category to status
			Confidence: result.Confidence, // Use overall confidence
		})
	}

	// Convert medications (events.Medication has flat fields)
	for _, m := range result.Medications {
		extracted.Medications = append(extracted.Medications, events.ExtractedMedication{
			Name:       m.Name,
			Code:       m.Code,
			CodeSystem: m.CodeSystem,
			Dosage:     m.Strength, // Map strength to dosage
			Confidence: result.Confidence,
		})
	}

	// Convert vital signs (events.VitalSign has flat fields)
	for _, v := range result.VitalSigns {
		extracted.VitalSigns = append(extracted.VitalSigns, events.ExtractedVitalSign{
			Name:       v.Name,
			LOINCCode:  v.LOINCCode,
			Value:      v.Value,
			Unit:       v.Unit,
			Confidence: result.Confidence,
		})
	}

	// Convert allergies (events.AllergyIntolerance has flat fields)
	for _, a := range result.Allergies {
		reaction := ""
		if len(a.Reactions) > 0 {
			reaction = a.Reactions[0].ManifestationText
		}
		extracted.Allergies = append(extracted.Allergies, events.ExtractedAllergy{
			Substance:  a.Name,
			Code:       a.Code,
			CodeSystem: a.CodeSystem,
			Reaction:   reaction,
			Severity:   a.Criticality,
			Confidence: result.Confidence,
		})
	}

	// Convert procedures (events.Procedure has flat fields)
	for _, p := range result.Procedures {
		extracted.Procedures = append(extracted.Procedures, events.ExtractedProcedure{
			Name:       p.Name,
			Code:       p.Code,
			CodeSystem: p.CodeSystem,
			Status:     p.Status,
			Confidence: result.Confidence,
		})
	}

	// Add to event meta
	if meta, ok := eventMap["meta"].(map[string]interface{}); ok {
		meta["extracted_entities"] = extracted
	} else {
		eventMap["meta"] = map[string]interface{}{
			"extracted_entities": extracted,
		}
	}

	return nil
}

// addQualityScoreToEvent adds the quality score to the event's metadata.
func addQualityScoreToEvent(event interface{}, score *quality.DataQualityScore) error {
	eventMap, ok := event.(map[string]interface{})
	if !ok {
		return nil
	}

	// Convert to events type
	eventsScore := events.DataQualityScore{
		OverallScore: score.OverallScore,
		Dimensions:   score.Dimensions,
		AnalyzedAt:   score.Metadata.AnalyzedAt,
		Model:        score.Metadata.Model,
	}

	// Convert issues
	for _, issue := range score.Issues {
		eventsScore.Issues = append(eventsScore.Issues, events.DataQualityIssue{
			Dimension:   issue.Dimension,
			Severity:    issue.Severity,
			Field:       issue.Field,
			Description: issue.Description,
			// Note: events.DataQualityIssue has Impact field instead of ActualValue/ExpectedValue
		})
	}

	// Convert recommendations
	for _, rec := range score.Recommendations {
		// Convert int priority to string
		priorityStr := "medium"
		switch {
		case rec.Priority <= 1:
			priorityStr = "high"
		case rec.Priority >= 4:
			priorityStr = "low"
		}
		eventsScore.Recommendations = append(eventsScore.Recommendations, events.QualityRecommendation{
			Priority:            priorityStr,
			Category:            rec.Category,
			Recommendation:      rec.Title + ": " + rec.Description,
			ExpectedImprovement: rec.Impact,
		})
	}

	// Add to event meta
	if meta, ok := eventMap["meta"].(map[string]interface{}); ok {
		meta["quality_score"] = eventsScore
	} else {
		eventMap["meta"] = map[string]interface{}{
			"quality_score": eventsScore,
		}
	}

	return nil
}

// detectEventType attempts to determine the event type from the event data.
func detectEventType(event interface{}) events.EventType {
	// Try map access first
	if m, ok := event.(map[string]interface{}); ok {
		if t, ok := m["type"].(string); ok {
			return events.EventType(t)
		}
	}

	// Try JSON marshal/unmarshal
	data, err := json.Marshal(event)
	if err != nil {
		return events.EventType("") // Unknown event type
	}

	var generic struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &generic); err == nil && generic.Type != "" {
		return events.EventType(generic.Type)
	}

	return events.EventType("") // Unknown event type
}

// parseOptBool parses an optional boolean config value with a default.
func parseOptBool(value string, defaultVal bool) bool {
	if value == "" {
		return defaultVal
	}
	return strings.ToLower(value) == "true"
}
