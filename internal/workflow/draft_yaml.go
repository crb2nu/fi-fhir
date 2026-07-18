package workflow

import (
	"fmt"
	"unicode/utf8"
)

// ParseDraftWorkflow applies the published grammar's resource and YAML-safety
// bounds to an authoring draft while retaining the legacy authoring shape. A
// draft may contain inline action configuration and may omit dsl_version and
// action IDs; the pure Planner strips configuration and assigns stable
// revision-local IDs before any simulation result is exposed.
func ParseDraftWorkflow(data []byte) (*Workflow, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("workflow draft is empty")
	}
	if len(data) > MaxPublishedWorkflowYAMLBytes {
		return nil, fmt.Errorf("workflow draft exceeds byte limit")
	}
	if !utf8.Valid(data) {
		return nil, fmt.Errorf("workflow draft is not valid UTF-8")
	}
	if err := preflightPublishedYAMLBytes(data); err != nil {
		return nil, fmt.Errorf("workflow draft preflight: %w", err)
	}
	if containsYAMLDirective(data) || containsExplicitNonSpecificTag(data) {
		return nil, fmt.Errorf("workflow draft contains unsupported YAML features")
	}
	document, err := decodeSingleYAMLDocument(data)
	if err != nil {
		return nil, fmt.Errorf("workflow draft decode: %w", err)
	}
	if err := preflightPublishedYAML(document); err != nil {
		return nil, fmt.Errorf("workflow draft preflight: %w", err)
	}

	definition, err := ParseWorkflow(data)
	if err != nil {
		return nil, err
	}
	if validation := definition.Validate(); len(validation) > 0 {
		return nil, fmt.Errorf("workflow draft validation: %w", validation[0])
	}
	return definition, nil
}
