package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
)

// TerminologyMapperInterface defines the interface for terminology mapping.
// This avoids import cycles with pkg/terminology.
type TerminologyMapperInterface interface {
	Map(sourceSystem, sourceCode, targetSystem string) []CodeMappingResult
}

// CodeMappingResult represents a terminology mapping result.
type CodeMappingResult struct {
	TargetCode    string
	TargetDisplay string
}

// WarningExplainerInterface defines the interface for warning explanation.
// This avoids import cycles with internal/llm/explain.
type WarningExplainerInterface interface {
	Explain(ctx context.Context, warning events.ParseWarning, format events.SourceFormat) (*ExplainedWarningResult, error)
}

// ExplainedWarningResult represents an explained warning.
type ExplainedWarningResult struct {
	Explanation   string
	FixSuggestion string
	Impact        string
}

// Transformer applies transforms to events.
type Transformer struct {
	terminologyMapper TerminologyMapperInterface
	warningExplainer  WarningExplainerInterface
}

// NewTransformer creates a new transformer.
// terminologyMapper can be nil if terminology mapping is not needed.
func NewTransformer(terminologyMapper TerminologyMapperInterface) *Transformer {
	return &Transformer{
		terminologyMapper: terminologyMapper,
	}
}

// SetWarningExplainer sets the warning explainer for explain_warnings transforms.
func (t *Transformer) SetWarningExplainer(explainer WarningExplainerInterface) {
	t.warningExplainer = explainer
}

// Apply applies a transform to an event and returns the transformed event.
// The original event is not modified; a copy is returned.
func (t *Transformer) Apply(event interface{}, transform Transform) (interface{}, error) {
	// Convert event to a modifiable map
	eventMap, err := toMap(event)
	if err != nil {
		return event, fmt.Errorf("failed to convert event for transform: %w", err)
	}

	// Apply the appropriate transform
	if transform.SetField != "" {
		if err := t.applySetField(eventMap, transform.SetField); err != nil {
			return event, fmt.Errorf("set_field transform failed: %w", err)
		}
	}

	if transform.MapTerminology != nil {
		if err := t.applyMapTerminology(eventMap, transform.MapTerminology); err != nil {
			return event, fmt.Errorf("map_terminology transform failed: %w", err)
		}
	}

	if transform.Redact != nil {
		t.applyRedact(eventMap, transform.Redact)
	}

	return eventMap, nil
}

// ApplyWithContext applies a transform with context support.
// This is required for transforms that need external service calls (e.g., explain_warnings).
// For transforms that don't need context, this delegates to Apply().
func (t *Transformer) ApplyWithContext(ctx context.Context, event interface{}, transform Transform) (interface{}, error) {
	// First apply non-context transforms via Apply()
	result, err := t.Apply(event, transform)
	if err != nil {
		return result, err
	}

	// Apply context-dependent transforms
	if transform.ExplainWarnings != nil {
		eventMap, ok := result.(map[string]interface{})
		if !ok {
			eventMap, err = toMap(result)
			if err != nil {
				return result, fmt.Errorf("failed to convert event for explain_warnings: %w", err)
			}
		}

		if err := t.applyExplainWarnings(ctx, eventMap, transform.ExplainWarnings); err != nil {
			return result, fmt.Errorf("explain_warnings transform failed: %w", err)
		}
		return eventMap, nil
	}

	return result, nil
}

// applyExplainWarnings adds LLM-generated explanations to parse warnings.
func (t *Transformer) applyExplainWarnings(ctx context.Context, event map[string]interface{}, config *ExplainWarningsConfig) error {
	if t.warningExplainer == nil {
		return fmt.Errorf("warning explainer not configured")
	}

	// Determine the warnings field path
	warningsField := config.WarningsField
	if warningsField == "" {
		warningsField = "warnings"
	}

	// Get warnings from the event
	warningsVal, err := getNestedValue(event, warningsField)
	if err != nil {
		// No warnings field, nothing to explain
		return nil
	}

	// Convert warnings to []events.ParseWarning
	warnings, err := toParseWarnings(warningsVal)
	if err != nil {
		// Not the expected type, skip silently
		return nil
	}

	if len(warnings) == 0 {
		return nil
	}

	// Determine source format from event
	format := detectSourceFormat(event)

	// Explain each warning
	explainedWarnings := make([]map[string]interface{}, 0, len(warnings))
	for _, warning := range warnings {
		result, explainErr := t.warningExplainer.Explain(ctx, warning, format)
		if explainErr != nil {
			// Add warning without explanation on error
			explainedWarnings = append(explainedWarnings, map[string]interface{}{
				"code":     warning.Code,
				"message":  warning.Message,
				"path":     warning.Path,
				"phase":    warning.Phase,
				"severity": warning.Severity,
			})
			continue
		}

		// Build explained warning map
		explained := map[string]interface{}{
			"code":        warning.Code,
			"message":     warning.Message,
			"path":        warning.Path,
			"phase":       warning.Phase,
			"severity":    warning.Severity,
			"explanation": result.Explanation,
		}
		if result.FixSuggestion != "" && config.IncludeFix {
			explained["fix_suggestion"] = result.FixSuggestion
		}
		if result.Impact != "" {
			explained["impact"] = result.Impact
		}
		explainedWarnings = append(explainedWarnings, explained)
	}

	// Update the warnings field with explained warnings
	return setNestedValue(event, warningsField, explainedWarnings)
}

// toParseWarnings converts a value to []events.ParseWarning.
func toParseWarnings(val interface{}) ([]events.ParseWarning, error) {
	switch v := val.(type) {
	case []events.ParseWarning:
		return v, nil
	case []interface{}:
		var warnings []events.ParseWarning
		for _, item := range v {
			if m, ok := item.(map[string]interface{}); ok {
				warning := events.ParseWarning{
					Code:     getString(m, "code"),
					Message:  getString(m, "message"),
					Path:     getString(m, "path"),
					Phase:    getString(m, "phase"),
					Severity: getString(m, "severity"),
				}
				warnings = append(warnings, warning)
			}
		}
		return warnings, nil
	default:
		return nil, fmt.Errorf("unsupported warnings type: %T", val)
	}
}

// getString safely extracts a string from a map.
func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

// detectSourceFormat attempts to detect the source format from event metadata.
func detectSourceFormat(event map[string]interface{}) events.SourceFormat {
	// Try meta.source_format first
	if meta, ok := event["meta"].(map[string]interface{}); ok {
		if format, ok := meta["source_format"].(string); ok {
			return events.SourceFormat(format)
		}
	}

	// Try source field
	if source, ok := event["source"].(string); ok {
		lowerSource := strings.ToLower(source)
		switch {
		case strings.Contains(lowerSource, "hl7"):
			return events.FormatHL7v2
		case strings.Contains(lowerSource, "fhir"):
			return events.FormatFHIR
		case strings.Contains(lowerSource, "cda"):
			return events.FormatCDA
		case strings.Contains(lowerSource, "835"):
			return events.FormatEDI835
		case strings.Contains(lowerSource, "837"):
			return events.FormatEDI837
		case strings.Contains(lowerSource, "270"):
			return events.FormatEDI270
		case strings.Contains(lowerSource, "271"):
			return events.FormatEDI271
		}
	}

	return events.FormatUnknown
}

// applySetField parses and applies a set_field transform.
// Format: "path.to.field = value" or "path.to.field = \"string value\""
func (t *Transformer) applySetField(event map[string]interface{}, expr string) error {
	// Split on '=' to get path and value
	parts := strings.SplitN(expr, "=", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid set_field format, expected 'path = value': %s", expr)
	}

	path := strings.TrimSpace(parts[0])
	valueStr := strings.TrimSpace(parts[1])

	// Parse the value
	value := parseValue(valueStr)

	// Set the value at the path
	return setNestedValue(event, path, value)
}

// applyMapTerminology applies terminology mapping to a field.
func (t *Transformer) applyMapTerminology(event map[string]interface{}, config *TerminologyMap) error {
	if t.terminologyMapper == nil {
		return fmt.Errorf("terminology mapper not configured")
	}

	// Get the current value at the field path
	currentValue, err := getNestedValue(event, config.Field)
	if err != nil {
		// Field doesn't exist - skip silently
		return nil //nolint:nilerr // Intentional: missing field is not an error for terminology mapping
	}

	codeStr, ok := currentValue.(string)
	if !ok {
		// Not a string - skip silently
		return nil
	}

	// Map the code
	mappings := t.terminologyMapper.Map(config.From, codeStr, config.To)
	if len(mappings) == 0 {
		// No mapping found - leave unchanged
		return nil
	}

	// Use the first mapping
	mapping := mappings[0]

	// Set the mapped code
	if err := setNestedValue(event, config.Field, mapping.TargetCode); err != nil {
		return err
	}

	// Optionally set display if there's a parallel _display field
	if mapping.TargetDisplay != "" {
		displayField := config.Field + "_display"
		_ = setNestedValue(event, displayField, mapping.TargetDisplay)
	}

	return nil
}

// TerminologyMapperAdapter wraps a terminology.Mapper to implement TerminologyMapperInterface.
// This allows integration with pkg/terminology without import cycles.
type TerminologyMapperAdapter struct {
	mapper interface{}
}

// NewTerminologyMapperAdapter creates an adapter for a terminology.Mapper.
func NewTerminologyMapperAdapter(mapper interface{}) *TerminologyMapperAdapter {
	return &TerminologyMapperAdapter{mapper: mapper}
}

// Map implements TerminologyMapperInterface by calling the underlying mapper.
func (a *TerminologyMapperAdapter) Map(sourceSystem, sourceCode, targetSystem string) []CodeMappingResult {
	// Use reflection to call the Map method
	// This avoids import cycles while maintaining type safety at runtime
	type mapperWithMap interface {
		Map(sourceSystem, sourceCode, targetSystem string) interface{}
	}

	if m, ok := a.mapper.(mapperWithMap); ok {
		result := m.Map(sourceSystem, sourceCode, targetSystem)
		// Convert the result to our interface type
		return convertMappings(result)
	}
	return nil
}

// convertMappings converts terminology.CodeMapping slice to CodeMappingResult slice.
func convertMappings(result interface{}) []CodeMappingResult {
	if result == nil {
		return nil
	}

	// Handle slice of structs with TargetCode and TargetDisplay fields
	switch v := result.(type) {
	case []interface{}:
		var mappings []CodeMappingResult
		for _, item := range v {
			if m, ok := item.(map[string]interface{}); ok {
				mapping := CodeMappingResult{}
				if tc, ok := m["TargetCode"].(string); ok {
					mapping.TargetCode = tc
				}
				if td, ok := m["TargetDisplay"].(string); ok {
					mapping.TargetDisplay = td
				}
				if mapping.TargetCode != "" {
					mappings = append(mappings, mapping)
				}
			}
		}
		return mappings
	}

	return nil
}

// applyRedact removes or masks specified fields.
func (t *Transformer) applyRedact(event map[string]interface{}, config *RedactConfig) {
	for _, field := range config.Fields {
		deleteNestedValue(event, field)
	}
}

// toMap converts an event to a map[string]interface{}.
func toMap(event interface{}) (map[string]interface{}, error) {
	// If already a map, make a deep copy
	if m, ok := event.(map[string]interface{}); ok {
		return deepCopyMap(m), nil
	}

	// Convert via JSON for struct types
	data, err := json.Marshal(event)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// deepCopyMap creates a deep copy of a map.
func deepCopyMap(m map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range m {
		switch val := v.(type) {
		case map[string]interface{}:
			result[k] = deepCopyMap(val)
		case []interface{}:
			result[k] = deepCopySlice(val)
		default:
			result[k] = v
		}
	}
	return result
}

// deepCopySlice creates a deep copy of a slice.
func deepCopySlice(s []interface{}) []interface{} {
	result := make([]interface{}, len(s))
	for i, v := range s {
		switch val := v.(type) {
		case map[string]interface{}:
			result[i] = deepCopyMap(val)
		case []interface{}:
			result[i] = deepCopySlice(val)
		default:
			result[i] = v
		}
	}
	return result
}

// parseValue parses a string value into the appropriate type.
func parseValue(s string) interface{} {
	// Check for quoted string
	if (strings.HasPrefix(s, `"`) && strings.HasSuffix(s, `"`)) ||
		(strings.HasPrefix(s, `'`) && strings.HasSuffix(s, `'`)) {
		return s[1 : len(s)-1]
	}

	// Check for boolean
	lower := strings.ToLower(s)
	if lower == "true" {
		return true
	}
	if lower == "false" {
		return false
	}

	// Check for null/nil
	if lower == "null" || lower == "nil" {
		return nil
	}

	// Check for number (integer first, then float)
	var intVal int64
	if _, err := fmt.Sscanf(s, "%d", &intVal); err == nil {
		// Make sure the entire string was consumed
		if fmt.Sprintf("%d", intVal) == s {
			return intVal
		}
	}

	var floatVal float64
	if _, err := fmt.Sscanf(s, "%f", &floatVal); err == nil {
		return floatVal
	}

	// Default to string without quotes
	return s
}

// setNestedValue sets a value at a dot-separated path.
func setNestedValue(m map[string]interface{}, path string, value interface{}) error {
	parts := strings.Split(path, ".")
	current := m

	// Navigate to the parent
	for i := 0; i < len(parts)-1; i++ {
		key := parts[i]
		next, exists := current[key]
		if !exists {
			// Create intermediate maps
			newMap := make(map[string]interface{})
			current[key] = newMap
			current = newMap
		} else {
			nextMap, ok := next.(map[string]interface{})
			if !ok {
				return fmt.Errorf("path %s: %s is not an object", path, key)
			}
			current = nextMap
		}
	}

	// Set the final value
	current[parts[len(parts)-1]] = value
	return nil
}

// getNestedValue gets a value at a dot-separated path.
func getNestedValue(m map[string]interface{}, path string) (interface{}, error) {
	parts := strings.Split(path, ".")
	var current interface{} = m

	for _, key := range parts {
		currentMap, ok := current.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("path %s: not an object", path)
		}

		next, exists := currentMap[key]
		if !exists {
			return nil, fmt.Errorf("path %s: key %s not found", path, key)
		}
		current = next
	}

	return current, nil
}

// deleteNestedValue deletes a value at a dot-separated path.
func deleteNestedValue(m map[string]interface{}, path string) {
	parts := strings.Split(path, ".")
	current := m

	// Navigate to the parent
	for i := 0; i < len(parts)-1; i++ {
		key := parts[i]
		next, exists := current[key]
		if !exists {
			return // Path doesn't exist
		}
		nextMap, ok := next.(map[string]interface{})
		if !ok {
			return // Not a map
		}
		current = nextMap
	}

	// Delete the final key
	delete(current, parts[len(parts)-1])
}
