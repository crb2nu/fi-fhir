// Package workflow provides a DSL for event routing, transformation, and action execution.
package workflow

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Workflow represents a complete workflow configuration.
type Workflow struct {
	Name    string  `yaml:"name" json:"name"`
	Version string  `yaml:"version" json:"version"`
	Routes  []Route `yaml:"routes" json:"routes"`
}

// Route defines a single event processing route.
type Route struct {
	Name       string      `yaml:"name" json:"name"`
	Filter     Filter      `yaml:"filter" json:"filter"`
	Transforms []Transform `yaml:"transform,omitempty" json:"transform,omitempty"`
	Actions    []Action    `yaml:"actions" json:"actions"`
}

// Filter matches events for routing.
type Filter struct {
	EventType StringOrSlice `yaml:"event_type,omitempty" json:"event_type,omitempty"`
	Source    StringOrSlice `yaml:"source,omitempty" json:"source,omitempty"`
	Condition string        `yaml:"condition,omitempty" json:"condition,omitempty"` // CEL expression (future)
}

// StringOrSlice handles YAML fields that can be a string or []string.
type StringOrSlice []string

// UnmarshalYAML implements yaml.Unmarshaler for StringOrSlice.
func (s *StringOrSlice) UnmarshalYAML(value *yaml.Node) error {
	var single string
	if err := value.Decode(&single); err == nil {
		*s = []string{single}
		return nil
	}

	var slice []string
	if err := value.Decode(&slice); err != nil {
		return err
	}
	*s = slice
	return nil
}

// Contains checks if the slice contains the given value.
func (s StringOrSlice) Contains(value string) bool {
	if len(s) == 0 {
		return true // Empty filter matches all
	}
	for _, v := range s {
		if v == value {
			return true
		}
	}
	return false
}

// Transform modifies events before action execution.
type Transform struct {
	SetField       string          `yaml:"set_field,omitempty" json:"set_field,omitempty"`
	MapTerminology *TerminologyMap `yaml:"map_terminology,omitempty" json:"map_terminology,omitempty"`
	Redact         *RedactConfig   `yaml:"redact,omitempty" json:"redact,omitempty"`
}

// TerminologyMap configures terminology translation.
type TerminologyMap struct {
	Field string `yaml:"field" json:"field"`
	From  string `yaml:"from" json:"from"`
	To    string `yaml:"to" json:"to"`
}

// RedactConfig specifies fields to redact.
type RedactConfig struct {
	Fields []string `yaml:"fields" json:"fields"`
}

// Action executes on matched events.
type Action struct {
	Type   string            `yaml:"type" json:"type"`
	Config map[string]string `yaml:"-" json:"-"` // Populated from remaining fields
}

// UnmarshalYAML implements yaml.Unmarshaler for Action.
func (a *Action) UnmarshalYAML(value *yaml.Node) error {
	// First, decode into a map to get all fields
	var raw map[string]interface{}
	if err := value.Decode(&raw); err != nil {
		return err
	}

	// Extract type
	if t, ok := raw["type"].(string); ok {
		a.Type = t
	} else {
		return fmt.Errorf("action missing 'type' field")
	}

	// Extract remaining fields as config
	a.Config = make(map[string]string)
	for k, v := range raw {
		if k == "type" {
			continue
		}
		switch val := v.(type) {
		case string:
			a.Config[k] = val
		case int, int64, float64:
			a.Config[k] = fmt.Sprintf("%v", val)
		case bool:
			a.Config[k] = fmt.Sprintf("%v", val)
		}
	}

	return nil
}

// LoadWorkflow loads a workflow from a YAML file.
func LoadWorkflow(path string) (*Workflow, error) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: path from trusted caller
	if err != nil {
		return nil, fmt.Errorf("failed to read workflow file: %w", err)
	}

	return ParseWorkflow(data)
}

// ParseWorkflow parses a workflow from YAML bytes.
func ParseWorkflow(data []byte) (*Workflow, error) {
	// Handle nested "workflow:" key
	var wrapper struct {
		Workflow Workflow `yaml:"workflow"`
	}
	if err := yaml.Unmarshal(data, &wrapper); err == nil && wrapper.Workflow.Name != "" {
		return &wrapper.Workflow, nil
	}

	// Try direct parsing
	var w Workflow
	if err := yaml.Unmarshal(data, &w); err != nil {
		return nil, fmt.Errorf("failed to parse workflow YAML: %w", err)
	}

	return &w, nil
}

// Validate checks the workflow configuration for errors.
func (w *Workflow) Validate() []error {
	var errors []error

	if w.Name == "" {
		errors = append(errors, fmt.Errorf("workflow name is required"))
	}

	if len(w.Routes) == 0 {
		errors = append(errors, fmt.Errorf("workflow must have at least one route"))
	}

	for i, route := range w.Routes {
		if route.Name == "" {
			errors = append(errors, fmt.Errorf("route %d: name is required", i))
		}
		if len(route.Actions) == 0 {
			errors = append(errors, fmt.Errorf("route %q: must have at least one action", route.Name))
		}
		for j, action := range route.Actions {
			if action.Type == "" {
				errors = append(errors, fmt.Errorf("route %q action %d: type is required", route.Name, j))
			}
		}
	}

	return errors
}
