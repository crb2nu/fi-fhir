package workflow

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestStringOrSliceUnmarshalYAML(t *testing.T) {
	t.Run("single string", func(t *testing.T) {
		yamlData := `filter: "ADT"`

		var result struct {
			Filter StringOrSlice `yaml:"filter"`
		}
		if err := yaml.Unmarshal([]byte(yamlData), &result); err != nil {
			t.Fatalf("Unmarshal error: %v", err)
		}

		if len(result.Filter) != 1 {
			t.Fatalf("Filter length = %d, want 1", len(result.Filter))
		}
		if result.Filter[0] != "ADT" {
			t.Errorf("Filter[0] = %q, want ADT", result.Filter[0])
		}
	})

	t.Run("slice of strings", func(t *testing.T) {
		yamlData := `filter:
  - ADT
  - ORU
  - ORM`

		var result struct {
			Filter StringOrSlice `yaml:"filter"`
		}
		if err := yaml.Unmarshal([]byte(yamlData), &result); err != nil {
			t.Fatalf("Unmarshal error: %v", err)
		}

		if len(result.Filter) != 3 {
			t.Fatalf("Filter length = %d, want 3", len(result.Filter))
		}
		expected := []string{"ADT", "ORU", "ORM"}
		for i, v := range expected {
			if result.Filter[i] != v {
				t.Errorf("Filter[%d] = %q, want %q", i, result.Filter[i], v)
			}
		}
	})

	t.Run("empty slice", func(t *testing.T) {
		yamlData := `filter: []`

		var result struct {
			Filter StringOrSlice `yaml:"filter"`
		}
		if err := yaml.Unmarshal([]byte(yamlData), &result); err != nil {
			t.Fatalf("Unmarshal error: %v", err)
		}

		if len(result.Filter) != 0 {
			t.Errorf("Filter length = %d, want 0", len(result.Filter))
		}
	})

	t.Run("invalid type", func(t *testing.T) {
		yamlData := `filter:
  key: value`

		var result struct {
			Filter StringOrSlice `yaml:"filter"`
		}
		err := yaml.Unmarshal([]byte(yamlData), &result)
		if err == nil {
			t.Error("expected error for invalid type")
		}
	})
}

func TestStringOrSliceContains(t *testing.T) {
	tests := []struct {
		name  string
		slice StringOrSlice
		value string
		want  bool
	}{
		{"empty slice matches all", StringOrSlice{}, "anything", true},
		{"found at start", StringOrSlice{"ADT", "ORU"}, "ADT", true},
		{"found at end", StringOrSlice{"ADT", "ORU"}, "ORU", true},
		{"found in middle", StringOrSlice{"ADT", "ORU", "ORM"}, "ORU", true},
		{"not found", StringOrSlice{"ADT", "ORU"}, "OBR", false},
		{"single element match", StringOrSlice{"ADT"}, "ADT", true},
		{"single element no match", StringOrSlice{"ADT"}, "ORU", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.slice.Contains(tt.value)
			if got != tt.want {
				t.Errorf("Contains(%q) = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}

func TestActionUnmarshalYAML(t *testing.T) {
	t.Run("simple action", func(t *testing.T) {
		yamlData := `type: log
level: info
message: "Event received"`

		var action Action
		if err := yaml.Unmarshal([]byte(yamlData), &action); err != nil {
			t.Fatalf("Unmarshal error: %v", err)
		}

		if action.Type != "log" {
			t.Errorf("Type = %q, want log", action.Type)
		}
		if action.Config["level"] != "info" {
			t.Errorf("Config[level] = %q, want info", action.Config["level"])
		}
		if action.Config["message"] != "Event received" {
			t.Errorf("Config[message] = %q, want 'Event received'", action.Config["message"])
		}
	})

	t.Run("action with numeric config", func(t *testing.T) {
		yamlData := `type: webhook
timeout: 30`

		var action Action
		if err := yaml.Unmarshal([]byte(yamlData), &action); err != nil {
			t.Fatalf("Unmarshal error: %v", err)
		}

		if action.Type != "webhook" {
			t.Errorf("Type = %q, want webhook", action.Type)
		}
		if action.Config["timeout"] != "30" {
			t.Errorf("Config[timeout] = %q, want 30", action.Config["timeout"])
		}
	})

	t.Run("action with boolean config", func(t *testing.T) {
		yamlData := `type: fhir
validate: true`

		var action Action
		if err := yaml.Unmarshal([]byte(yamlData), &action); err != nil {
			t.Fatalf("Unmarshal error: %v", err)
		}

		if action.Config["validate"] != "true" {
			t.Errorf("Config[validate] = %q, want true", action.Config["validate"])
		}
	})

	t.Run("action with float config", func(t *testing.T) {
		yamlData := `type: rate_limit
rate: 100.5`

		var action Action
		if err := yaml.Unmarshal([]byte(yamlData), &action); err != nil {
			t.Fatalf("Unmarshal error: %v", err)
		}

		if action.Config["rate"] != "100.5" {
			t.Errorf("Config[rate] = %q, want 100.5", action.Config["rate"])
		}
	})

	t.Run("action missing type", func(t *testing.T) {
		yamlData := `level: info
message: "No type"`

		var action Action
		err := yaml.Unmarshal([]byte(yamlData), &action)
		if err == nil {
			t.Error("expected error for missing type")
		}
	})

	t.Run("action with only type", func(t *testing.T) {
		yamlData := `type: noop`

		var action Action
		if err := yaml.Unmarshal([]byte(yamlData), &action); err != nil {
			t.Fatalf("Unmarshal error: %v", err)
		}

		if action.Type != "noop" {
			t.Errorf("Type = %q, want noop", action.Type)
		}
		if len(action.Config) != 0 {
			t.Errorf("Config length = %d, want 0", len(action.Config))
		}
	})
}

func TestParseWorkflowVariations(t *testing.T) {
	t.Run("direct workflow", func(t *testing.T) {
		yamlData := `name: test-workflow
version: "1.0"
routes:
  - name: log-all
    filter:
      event_type: ADT
    actions:
      - type: log
        level: info`

		w, err := ParseWorkflow([]byte(yamlData))
		if err != nil {
			t.Fatalf("ParseWorkflow error: %v", err)
		}

		if w.Name != "test-workflow" {
			t.Errorf("Name = %q, want test-workflow", w.Name)
		}
		if w.Version != "1.0" {
			t.Errorf("Version = %q, want 1.0", w.Version)
		}
		if len(w.Routes) != 1 {
			t.Fatalf("Routes length = %d, want 1", len(w.Routes))
		}
		if w.Routes[0].Name != "log-all" {
			t.Errorf("Routes[0].Name = %q, want log-all", w.Routes[0].Name)
		}
	})

	t.Run("nested workflow key", func(t *testing.T) {
		yamlData := `workflow:
  name: nested-workflow
  version: "2.0"
  routes:
    - name: route1
      filter:
        event_type: ORU
      actions:
        - type: webhook
          url: "http://example.com"`

		w, err := ParseWorkflow([]byte(yamlData))
		if err != nil {
			t.Fatalf("ParseWorkflow error: %v", err)
		}

		if w.Name != "nested-workflow" {
			t.Errorf("Name = %q, want nested-workflow", w.Name)
		}
		if w.Version != "2.0" {
			t.Errorf("Version = %q, want 2.0", w.Version)
		}
	})

	t.Run("invalid yaml", func(t *testing.T) {
		yamlData := `name: test
  invalid: yaml
   indentation: wrong`

		_, err := ParseWorkflow([]byte(yamlData))
		if err == nil {
			t.Error("expected error for invalid YAML")
		}
	})

	t.Run("workflow with transforms", func(t *testing.T) {
		yamlData := `name: transform-workflow
version: "1.0"
routes:
  - name: with-transforms
    filter:
      event_type: ADT
    transform:
      - set_field: "status = active"
      - redact:
          fields:
            - ssn
            - phone
    actions:
      - type: log`

		w, err := ParseWorkflow([]byte(yamlData))
		if err != nil {
			t.Fatalf("ParseWorkflow error: %v", err)
		}

		if len(w.Routes[0].Transforms) != 2 {
			t.Fatalf("Transforms length = %d, want 2", len(w.Routes[0].Transforms))
		}
		if w.Routes[0].Transforms[0].SetField != "status = active" {
			t.Errorf("SetField = %q", w.Routes[0].Transforms[0].SetField)
		}
		if w.Routes[0].Transforms[1].Redact == nil {
			t.Fatal("Redact should not be nil")
		}
		if len(w.Routes[0].Transforms[1].Redact.Fields) != 2 {
			t.Errorf("Redact.Fields length = %d, want 2", len(w.Routes[0].Transforms[1].Redact.Fields))
		}
	})

	t.Run("workflow with terminology mapping", func(t *testing.T) {
		yamlData := `name: terminology-workflow
version: "1.0"
routes:
  - name: map-codes
    filter: {}
    transform:
      - map_terminology:
          field: diagnosis_code
          from: ICD9
          to: ICD10
    actions:
      - type: fhir`

		w, err := ParseWorkflow([]byte(yamlData))
		if err != nil {
			t.Fatalf("ParseWorkflow error: %v", err)
		}

		transform := w.Routes[0].Transforms[0]
		if transform.MapTerminology == nil {
			t.Fatal("MapTerminology should not be nil")
		}
		if transform.MapTerminology.Field != "diagnosis_code" {
			t.Errorf("Field = %q, want diagnosis_code", transform.MapTerminology.Field)
		}
		if transform.MapTerminology.From != "ICD9" {
			t.Errorf("From = %q, want ICD9", transform.MapTerminology.From)
		}
		if transform.MapTerminology.To != "ICD10" {
			t.Errorf("To = %q, want ICD10", transform.MapTerminology.To)
		}
	})
}

func TestLoadWorkflow(t *testing.T) {
	t.Run("load valid file", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "workflow.yaml")

		yamlContent := `name: file-workflow
version: "1.0"
routes:
  - name: test-route
    filter:
      event_type: ADT
    actions:
      - type: log
`
		if err := os.WriteFile(filePath, []byte(yamlContent), 0600); err != nil {
			t.Fatalf("WriteFile error: %v", err)
		}

		w, err := LoadWorkflow(filePath)
		if err != nil {
			t.Fatalf("LoadWorkflow error: %v", err)
		}

		if w.Name != "file-workflow" {
			t.Errorf("Name = %q, want file-workflow", w.Name)
		}
	})

	t.Run("file not found", func(t *testing.T) {
		_, err := LoadWorkflow("/nonexistent/path/workflow.yaml")
		if err == nil {
			t.Error("expected error for nonexistent file")
		}
	})

	t.Run("invalid yaml in file", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "invalid.yaml")

		invalidContent := `name: test
  invalid: indentation
   broken: yaml`
		if err := os.WriteFile(filePath, []byte(invalidContent), 0600); err != nil {
			t.Fatalf("WriteFile error: %v", err)
		}

		_, err := LoadWorkflow(filePath)
		if err == nil {
			t.Error("expected error for invalid YAML")
		}
	})
}

func TestWorkflow_Validate(t *testing.T) {
	t.Run("valid workflow", func(t *testing.T) {
		w := &Workflow{
			Name:    "valid-workflow",
			Version: "1.0",
			Routes: []Route{
				{
					Name: "route1",
					Actions: []Action{
						{Type: "log"},
					},
				},
			},
		}

		errors := w.Validate()
		if len(errors) != 0 {
			t.Errorf("Validate returned %d errors, want 0: %v", len(errors), errors)
		}
	})

	t.Run("missing workflow name", func(t *testing.T) {
		w := &Workflow{
			Routes: []Route{
				{
					Name: "route1",
					Actions: []Action{
						{Type: "log"},
					},
				},
			},
		}

		errors := w.Validate()
		if len(errors) == 0 {
			t.Error("expected error for missing workflow name")
		}
		found := false
		for _, err := range errors {
			if err.Error() == "workflow name is required" {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected 'workflow name is required' error")
		}
	})

	t.Run("no routes", func(t *testing.T) {
		w := &Workflow{
			Name:   "no-routes",
			Routes: []Route{},
		}

		errors := w.Validate()
		if len(errors) == 0 {
			t.Error("expected error for no routes")
		}
		found := false
		for _, err := range errors {
			if err.Error() == "workflow must have at least one route" {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected 'workflow must have at least one route' error")
		}
	})

	t.Run("route without name", func(t *testing.T) {
		w := &Workflow{
			Name: "valid-name",
			Routes: []Route{
				{
					Name: "", // Missing name
					Actions: []Action{
						{Type: "log"},
					},
				},
			},
		}

		errors := w.Validate()
		if len(errors) == 0 {
			t.Error("expected error for route without name")
		}
	})

	t.Run("route without actions", func(t *testing.T) {
		w := &Workflow{
			Name: "valid-name",
			Routes: []Route{
				{
					Name:    "route1",
					Actions: []Action{}, // No actions
				},
			},
		}

		errors := w.Validate()
		if len(errors) == 0 {
			t.Error("expected error for route without actions")
		}
	})

	t.Run("action without type", func(t *testing.T) {
		w := &Workflow{
			Name: "valid-name",
			Routes: []Route{
				{
					Name: "route1",
					Actions: []Action{
						{Type: ""}, // Missing type
					},
				},
			},
		}

		errors := w.Validate()
		if len(errors) == 0 {
			t.Error("expected error for action without type")
		}
	})

	t.Run("multiple errors", func(t *testing.T) {
		w := &Workflow{
			Name:   "",        // Error 1
			Routes: []Route{}, // Error 2
		}

		errors := w.Validate()
		if len(errors) < 2 {
			t.Errorf("expected at least 2 errors, got %d", len(errors))
		}
	})

	t.Run("multiple routes with mixed validity", func(t *testing.T) {
		w := &Workflow{
			Name: "multi-route",
			Routes: []Route{
				{
					Name: "valid-route",
					Actions: []Action{
						{Type: "log"},
					},
				},
				{
					Name: "", // Invalid - no name
					Actions: []Action{
						{Type: "webhook"},
					},
				},
				{
					Name:    "no-actions",
					Actions: []Action{}, // Invalid - no actions
				},
			},
		}

		errors := w.Validate()
		if len(errors) < 2 {
			t.Errorf("expected at least 2 errors, got %d: %v", len(errors), errors)
		}
	})
}

func TestFilter_Structure(t *testing.T) {
	t.Run("parse filter with event_type and source", func(t *testing.T) {
		yamlData := `filter:
  event_type: ADT
  source: hospital-a
  condition: "event.patient.age > 18"`

		var result struct {
			Filter Filter `yaml:"filter"`
		}
		if err := yaml.Unmarshal([]byte(yamlData), &result); err != nil {
			t.Fatalf("Unmarshal error: %v", err)
		}

		if len(result.Filter.EventType) != 1 || result.Filter.EventType[0] != "ADT" {
			t.Errorf("EventType = %v, want [ADT]", result.Filter.EventType)
		}
		if len(result.Filter.Source) != 1 || result.Filter.Source[0] != "hospital-a" {
			t.Errorf("Source = %v, want [hospital-a]", result.Filter.Source)
		}
		if result.Filter.Condition != "event.patient.age > 18" {
			t.Errorf("Condition = %q", result.Filter.Condition)
		}
	})

	t.Run("parse filter with multiple event types", func(t *testing.T) {
		yamlData := `filter:
  event_type:
    - ADT
    - ORU
    - ORM`

		var result struct {
			Filter Filter `yaml:"filter"`
		}
		if err := yaml.Unmarshal([]byte(yamlData), &result); err != nil {
			t.Fatalf("Unmarshal error: %v", err)
		}

		if len(result.Filter.EventType) != 3 {
			t.Errorf("EventType length = %d, want 3", len(result.Filter.EventType))
		}
	})
}

func TestTransform_Structure(t *testing.T) {
	t.Run("parse set_field transform", func(t *testing.T) {
		yamlData := `transform:
  set_field: "status = active"`

		var result struct {
			Transform Transform `yaml:"transform"`
		}
		if err := yaml.Unmarshal([]byte(yamlData), &result); err != nil {
			t.Fatalf("Unmarshal error: %v", err)
		}

		if result.Transform.SetField != "status = active" {
			t.Errorf("SetField = %q", result.Transform.SetField)
		}
	})

	t.Run("parse redact transform", func(t *testing.T) {
		yamlData := `transform:
  redact:
    fields:
      - ssn
      - phone
      - address`

		var result struct {
			Transform Transform `yaml:"transform"`
		}
		if err := yaml.Unmarshal([]byte(yamlData), &result); err != nil {
			t.Fatalf("Unmarshal error: %v", err)
		}

		if result.Transform.Redact == nil {
			t.Fatal("Redact should not be nil")
		}
		if len(result.Transform.Redact.Fields) != 3 {
			t.Errorf("Redact.Fields length = %d, want 3", len(result.Transform.Redact.Fields))
		}
	})

	t.Run("parse map_terminology transform", func(t *testing.T) {
		yamlData := `transform:
  map_terminology:
    field: code
    from: SNOMED
    to: LOINC`

		var result struct {
			Transform Transform `yaml:"transform"`
		}
		if err := yaml.Unmarshal([]byte(yamlData), &result); err != nil {
			t.Fatalf("Unmarshal error: %v", err)
		}

		if result.Transform.MapTerminology == nil {
			t.Fatal("MapTerminology should not be nil")
		}
		if result.Transform.MapTerminology.Field != "code" {
			t.Errorf("Field = %q", result.Transform.MapTerminology.Field)
		}
	})
}

func TestRoute_Structure(t *testing.T) {
	yamlData := `name: complete-route
filter:
  event_type:
    - ADT
    - ORU
  source: hospital-a
  condition: "event.active == true"
transform:
  - set_field: "processed = true"
  - redact:
      fields:
        - ssn
actions:
  - type: log
    level: info
  - type: webhook
    url: "http://example.com"`

	var route Route
	if err := yaml.Unmarshal([]byte(yamlData), &route); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if route.Name != "complete-route" {
		t.Errorf("Name = %q", route.Name)
	}
	if len(route.Filter.EventType) != 2 {
		t.Errorf("EventType length = %d", len(route.Filter.EventType))
	}
	if len(route.Transforms) != 2 {
		t.Errorf("Transforms length = %d", len(route.Transforms))
	}
	if len(route.Actions) != 2 {
		t.Errorf("Actions length = %d", len(route.Actions))
	}
	if route.Actions[0].Type != "log" {
		t.Errorf("Actions[0].Type = %q", route.Actions[0].Type)
	}
	if route.Actions[1].Config["url"] != "http://example.com" {
		t.Errorf("Actions[1].Config[url] = %q", route.Actions[1].Config["url"])
	}
}
