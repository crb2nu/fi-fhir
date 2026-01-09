package workflow

import (
	"strings"
	"testing"
)

func TestValidatorValidWorkflow(t *testing.T) {
	wf := &Workflow{
		Name:    "test_workflow",
		Version: "1.0",
		Routes: []Route{
			{
				Name: "test_route",
				Filter: Filter{
					EventType: StringOrSlice{"patient_admit"},
				},
				Actions: []Action{
					{Type: "log", Config: map[string]string{"level": "info"}},
				},
			},
		},
	}

	result, err := ValidateWorkflow(wf)
	if err != nil {
		t.Fatalf("ValidateWorkflow() error = %v", err)
	}

	if !result.Valid {
		t.Errorf("Expected valid workflow, got errors: %v", result.Errors)
	}
}

func TestValidatorMissingName(t *testing.T) {
	wf := &Workflow{
		Version: "1.0",
		Routes: []Route{
			{
				Name: "route1",
				Filter: Filter{
					EventType: StringOrSlice{"test"},
				},
				Actions: []Action{
					{Type: "log"},
				},
			},
		},
	}

	result, err := ValidateWorkflow(wf)
	if err != nil {
		t.Fatalf("ValidateWorkflow() error = %v", err)
	}

	if result.Valid {
		t.Error("Expected invalid workflow due to missing name")
	}

	foundError := false
	for _, e := range result.Errors {
		if e.Code == "MISSING_NAME" {
			foundError = true
			break
		}
	}
	if !foundError {
		t.Error("Expected MISSING_NAME error")
	}
}

func TestValidatorDuplicateRouteNames(t *testing.T) {
	wf := &Workflow{
		Name:    "test",
		Version: "1.0",
		Routes: []Route{
			{
				Name: "duplicate_route",
				Filter: Filter{
					EventType: StringOrSlice{"test"},
				},
				Actions: []Action{{Type: "log"}},
			},
			{
				Name: "duplicate_route",
				Filter: Filter{
					EventType: StringOrSlice{"test"},
				},
				Actions: []Action{{Type: "log"}},
			},
		},
	}

	result, err := ValidateWorkflow(wf)
	if err != nil {
		t.Fatalf("ValidateWorkflow() error = %v", err)
	}

	if result.Valid {
		t.Error("Expected invalid workflow due to duplicate route names")
	}

	foundError := false
	for _, e := range result.Errors {
		if e.Code == "DUPLICATE_ROUTE_NAME" {
			foundError = true
			break
		}
	}
	if !foundError {
		t.Error("Expected DUPLICATE_ROUTE_NAME error")
	}
}

func TestValidatorInvalidCEL(t *testing.T) {
	wf := &Workflow{
		Name:    "test",
		Version: "1.0",
		Routes: []Route{
			{
				Name: "route1",
				Filter: Filter{
					Condition: "invalid {{ cel expression",
				},
				Actions: []Action{{Type: "log"}},
			},
		},
	}

	result, err := ValidateWorkflow(wf)
	if err != nil {
		t.Fatalf("ValidateWorkflow() error = %v", err)
	}

	if result.Valid {
		t.Error("Expected invalid workflow due to bad CEL")
	}

	foundError := false
	for _, e := range result.Errors {
		if e.Code == "INVALID_CEL" {
			foundError = true
			break
		}
	}
	if !foundError {
		t.Error("Expected INVALID_CEL error")
	}
}

func TestValidatorValidCEL(t *testing.T) {
	wf := &Workflow{
		Name:    "test",
		Version: "1.0",
		Routes: []Route{
			{
				Name: "route1",
				Filter: Filter{
					Condition: `event.type == "patient_admit" && event.source == "epic"`,
				},
				Actions: []Action{{Type: "log"}},
			},
		},
	}

	result, err := ValidateWorkflow(wf)
	if err != nil {
		t.Fatalf("ValidateWorkflow() error = %v", err)
	}

	// Should not have CEL errors
	for _, e := range result.Errors {
		if e.Code == "INVALID_CEL" {
			t.Errorf("Unexpected INVALID_CEL error: %s", e.Message)
		}
	}
}

func TestValidatorWebhookAction(t *testing.T) {
	tests := []struct {
		name      string
		config    map[string]string
		wantError string
	}{
		{
			name:      "missing url",
			config:    map[string]string{},
			wantError: "MISSING_WEBHOOK_URL",
		},
		{
			name:      "valid url",
			config:    map[string]string{"url": "https://example.com/webhook"},
			wantError: "",
		},
		{
			name:      "valid url template",
			config:    map[string]string{"url": "https://example.com/{{.type}}"},
			wantError: "",
		},
		{
			name:      "invalid url template",
			config:    map[string]string{"url": "https://example.com/{{.type"},
			wantError: "INVALID_URL_TEMPLATE",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wf := &Workflow{
				Name: "test",
				Routes: []Route{
					{
						Name:    "route1",
						Filter:  Filter{EventType: StringOrSlice{"test"}},
						Actions: []Action{{Type: "webhook", Config: tt.config}},
					},
				},
			}

			result, err := ValidateWorkflow(wf)
			if err != nil {
				t.Fatalf("ValidateWorkflow() error = %v", err)
			}

			if tt.wantError == "" {
				for _, e := range result.Errors {
					if strings.Contains(e.Path, "actions[0]") {
						t.Errorf("Unexpected action error: %s", e.Message)
					}
				}
			} else {
				found := false
				for _, e := range result.Errors {
					if e.Code == tt.wantError {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected error %s, got: %v", tt.wantError, result.Errors)
				}
			}
		})
	}
}

func TestValidatorFHIRAction(t *testing.T) {
	tests := []struct {
		name       string
		config     map[string]string
		wantErrors []string
	}{
		{
			name:       "missing endpoint",
			config:     map[string]string{},
			wantErrors: []string{"MISSING_FHIR_ENDPOINT"},
		},
		{
			name:       "valid with token",
			config:     map[string]string{"endpoint": "https://fhir.example.com/r4", "token": "secret"},
			wantErrors: nil,
		},
		{
			name: "oauth missing client_id",
			config: map[string]string{
				"endpoint":      "https://fhir.example.com/r4",
				"token_url":     "https://auth.example.com/token",
				"client_secret": "secret",
			},
			wantErrors: []string{"MISSING_OAUTH_CLIENT_ID"},
		},
		{
			name: "oauth missing client_secret",
			config: map[string]string{
				"endpoint":  "https://fhir.example.com/r4",
				"token_url": "https://auth.example.com/token",
				"client_id": "my-client",
			},
			wantErrors: []string{"MISSING_OAUTH_CLIENT_SECRET"},
		},
		{
			name: "valid oauth",
			config: map[string]string{
				"endpoint":      "https://fhir.example.com/r4",
				"token_url":     "https://auth.example.com/token",
				"client_id":     "my-client",
				"client_secret": "secret",
			},
			wantErrors: nil,
		},
		{
			name:       "invalid operation",
			config:     map[string]string{"endpoint": "https://fhir.example.com/r4", "token": "x", "operation": "delete"},
			wantErrors: []string{"INVALID_FHIR_OPERATION"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wf := &Workflow{
				Name: "test",
				Routes: []Route{
					{
						Name:    "route1",
						Filter:  Filter{EventType: StringOrSlice{"test"}},
						Actions: []Action{{Type: "fhir", Config: tt.config}},
					},
				},
			}

			result, err := ValidateWorkflow(wf)
			if err != nil {
				t.Fatalf("ValidateWorkflow() error = %v", err)
			}

			for _, wantCode := range tt.wantErrors {
				found := false
				for _, e := range result.Errors {
					if e.Code == wantCode {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected error %s, got: %v", wantCode, result.Errors)
				}
			}
		})
	}
}

func TestValidatorDatabaseAction(t *testing.T) {
	tests := []struct {
		name       string
		config     map[string]string
		wantErrors []string
	}{
		{
			name:       "missing connection",
			config:     map[string]string{"table": "events", "mapping_id": "id"},
			wantErrors: []string{"MISSING_DB_CONNECTION"},
		},
		{
			name:       "missing table",
			config:     map[string]string{"connection": "postgres://...", "mapping_id": "id"},
			wantErrors: []string{"MISSING_DB_TABLE"},
		},
		{
			name:       "missing mappings",
			config:     map[string]string{"connection": "postgres://...", "table": "events"},
			wantErrors: []string{"NO_DB_MAPPINGS"},
		},
		{
			name:       "valid insert",
			config:     map[string]string{"connection": "postgres://...", "table": "events", "mapping_id": "id"},
			wantErrors: nil,
		},
		{
			name: "upsert missing conflict_on",
			config: map[string]string{
				"connection": "postgres://...",
				"table":      "events",
				"operation":  "upsert",
				"mapping_id": "id",
			},
			wantErrors: []string{"MISSING_UPSERT_CONFLICT"},
		},
		{
			name: "valid upsert",
			config: map[string]string{
				"connection":  "postgres://...",
				"table":       "events",
				"operation":   "upsert",
				"conflict_on": "event_id",
				"mapping_id":  "id",
			},
			wantErrors: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wf := &Workflow{
				Name: "test",
				Routes: []Route{
					{
						Name:    "route1",
						Filter:  Filter{EventType: StringOrSlice{"test"}},
						Actions: []Action{{Type: "database", Config: tt.config}},
					},
				},
			}

			result, err := ValidateWorkflow(wf)
			if err != nil {
				t.Fatalf("ValidateWorkflow() error = %v", err)
			}

			for _, wantCode := range tt.wantErrors {
				found := false
				for _, e := range result.Errors {
					if e.Code == wantCode {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected error %s, got: %v", wantCode, result.Errors)
				}
			}
		})
	}
}

func TestValidatorQueueAction(t *testing.T) {
	tests := []struct {
		name       string
		config     map[string]string
		wantErrors []string
	}{
		{
			name:       "missing driver",
			config:     map[string]string{"topic": "events"},
			wantErrors: []string{"MISSING_QUEUE_DRIVER"},
		},
		{
			name:       "missing topic",
			config:     map[string]string{"driver": "kafka"},
			wantErrors: []string{"MISSING_QUEUE_TOPIC"},
		},
		{
			name:       "valid",
			config:     map[string]string{"driver": "kafka", "topic": "events"},
			wantErrors: nil,
		},
		{
			name:       "valid topic template",
			config:     map[string]string{"driver": "kafka", "topic": "events.{{.type}}"},
			wantErrors: nil,
		},
		{
			name:       "invalid topic template",
			config:     map[string]string{"driver": "kafka", "topic": "events.{{.type"},
			wantErrors: []string{"INVALID_TOPIC_TEMPLATE"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wf := &Workflow{
				Name: "test",
				Routes: []Route{
					{
						Name:    "route1",
						Filter:  Filter{EventType: StringOrSlice{"test"}},
						Actions: []Action{{Type: "queue", Config: tt.config}},
					},
				},
			}

			result, err := ValidateWorkflow(wf)
			if err != nil {
				t.Fatalf("ValidateWorkflow() error = %v", err)
			}

			for _, wantCode := range tt.wantErrors {
				found := false
				for _, e := range result.Errors {
					if e.Code == wantCode {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected error %s, got: %v", wantCode, result.Errors)
				}
			}
		})
	}
}

func TestValidatorTransforms(t *testing.T) {
	tests := []struct {
		name       string
		transform  Transform
		wantErrors []string
	}{
		{
			name:       "empty transform",
			transform:  Transform{},
			wantErrors: []string{"EMPTY_TRANSFORM"},
		},
		{
			name:       "invalid set_field format",
			transform:  Transform{SetField: "patient.status"},
			wantErrors: []string{"INVALID_SET_FIELD"},
		},
		{
			name:       "valid set_field",
			transform:  Transform{SetField: "patient.status = active"},
			wantErrors: nil,
		},
		{
			name:       "map_terminology missing field",
			transform:  Transform{MapTerminology: &TerminologyMap{}},
			wantErrors: []string{"MISSING_TERMINOLOGY_FIELD"},
		},
		{
			name:       "redact missing fields",
			transform:  Transform{Redact: &RedactConfig{}},
			wantErrors: []string{"MISSING_REDACT_FIELDS"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wf := &Workflow{
				Name: "test",
				Routes: []Route{
					{
						Name:       "route1",
						Filter:     Filter{EventType: StringOrSlice{"test"}},
						Transforms: []Transform{tt.transform},
						Actions:    []Action{{Type: "log"}},
					},
				},
			}

			result, err := ValidateWorkflow(wf)
			if err != nil {
				t.Fatalf("ValidateWorkflow() error = %v", err)
			}

			for _, wantCode := range tt.wantErrors {
				found := false
				for _, e := range result.Errors {
					if e.Code == wantCode {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected error %s, got: %v", wantCode, result.Errors)
				}
			}
		})
	}
}

func TestValidatorWarnings(t *testing.T) {
	t.Run("no routes warning", func(t *testing.T) {
		wf := &Workflow{Name: "test", Version: "1.0", Routes: []Route{}}

		result, err := ValidateWorkflow(wf)
		if err != nil {
			t.Fatalf("ValidateWorkflow() error = %v", err)
		}

		found := false
		for _, w := range result.Warnings {
			if w.Code == "NO_ROUTES" {
				found = true
				break
			}
		}
		if !found {
			t.Error("Expected NO_ROUTES warning")
		}
	})

	t.Run("no filter warning", func(t *testing.T) {
		wf := &Workflow{
			Name: "test",
			Routes: []Route{
				{
					Name:    "catch_all",
					Filter:  Filter{}, // Empty filter
					Actions: []Action{{Type: "log"}},
				},
			},
		}

		result, err := ValidateWorkflow(wf)
		if err != nil {
			t.Fatalf("ValidateWorkflow() error = %v", err)
		}

		found := false
		for _, w := range result.Warnings {
			if w.Code == "NO_FILTER" {
				found = true
				break
			}
		}
		if !found {
			t.Error("Expected NO_FILTER warning")
		}
	})

	t.Run("no actions warning", func(t *testing.T) {
		wf := &Workflow{
			Name: "test",
			Routes: []Route{
				{
					Name:    "empty_route",
					Filter:  Filter{EventType: StringOrSlice{"test"}},
					Actions: []Action{},
				},
			},
		}

		result, err := ValidateWorkflow(wf)
		if err != nil {
			t.Fatalf("ValidateWorkflow() error = %v", err)
		}

		found := false
		for _, w := range result.Warnings {
			if w.Code == "NO_ACTIONS" {
				found = true
				break
			}
		}
		if !found {
			t.Error("Expected NO_ACTIONS warning")
		}
	})
}

func TestValidationResultSummary(t *testing.T) {
	t.Run("valid no issues", func(t *testing.T) {
		result := &ValidationResult{Valid: true}
		summary := result.Summary()
		if !strings.Contains(summary, "valid") {
			t.Errorf("Expected 'valid' in summary, got: %s", summary)
		}
	})

	t.Run("invalid with errors", func(t *testing.T) {
		result := &ValidationResult{
			Valid: false,
			Errors: []ValidationError{
				{Path: "test", Message: "test error"},
			},
		}
		summary := result.Summary()
		if !strings.Contains(summary, "invalid") {
			t.Errorf("Expected 'invalid' in summary, got: %s", summary)
		}
		if !strings.Contains(summary, "1 error") {
			t.Errorf("Expected '1 error' in summary, got: %s", summary)
		}
	})

	t.Run("valid with warnings", func(t *testing.T) {
		result := &ValidationResult{
			Valid: true,
			Warnings: []ValidationError{
				{Path: "test", Message: "test warning"},
				{Path: "test2", Message: "test warning 2"},
			},
		}
		summary := result.Summary()
		if !strings.Contains(summary, "2 warning") {
			t.Errorf("Expected '2 warning' in summary, got: %s", summary)
		}
	})
}

func TestValidationResultAllIssues(t *testing.T) {
	result := &ValidationResult{
		Errors:   []ValidationError{{Path: "e1"}},
		Warnings: []ValidationError{{Path: "w1"}, {Path: "w2"}},
		Info:     []ValidationError{{Path: "i1"}},
	}

	all := result.AllIssues()
	if len(all) != 4 {
		t.Errorf("Expected 4 issues, got %d", len(all))
	}
}

func TestLogActionValidation(t *testing.T) {
	t.Run("invalid log level", func(t *testing.T) {
		wf := &Workflow{
			Name: "test",
			Routes: []Route{
				{
					Name:    "route1",
					Filter:  Filter{EventType: StringOrSlice{"test"}},
					Actions: []Action{{Type: "log", Config: map[string]string{"level": "fatal"}}},
				},
			},
		}

		result, err := ValidateWorkflow(wf)
		if err != nil {
			t.Fatalf("ValidateWorkflow() error = %v", err)
		}

		found := false
		for _, e := range result.Errors {
			if e.Code == "INVALID_LOG_LEVEL" {
				found = true
				break
			}
		}
		if !found {
			t.Error("Expected INVALID_LOG_LEVEL error")
		}
	})

	t.Run("invalid message template", func(t *testing.T) {
		wf := &Workflow{
			Name: "test",
			Routes: []Route{
				{
					Name:    "route1",
					Filter:  Filter{EventType: StringOrSlice{"test"}},
					Actions: []Action{{Type: "log", Config: map[string]string{"message": "{{.type"}}},
				},
			},
		}

		result, err := ValidateWorkflow(wf)
		if err != nil {
			t.Fatalf("ValidateWorkflow() error = %v", err)
		}

		found := false
		for _, e := range result.Errors {
			if e.Code == "INVALID_TEMPLATE" {
				found = true
				break
			}
		}
		if !found {
			t.Error("Expected INVALID_TEMPLATE error")
		}
	})
}
