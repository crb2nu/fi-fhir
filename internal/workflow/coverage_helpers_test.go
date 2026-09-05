package workflow

import (
	"testing"
	"time"

	"github.com/google/cel-go/common/types"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/llm/extract"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/llm/quality"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
)

// ---------------------------------------------------------------------------
// Group 1: Pure helpers — no dependencies, table-driven
// ---------------------------------------------------------------------------

func TestIsTruthy(t *testing.T) {
	tests := []struct {
		name string
		val  interface{} // will be wrapped into a ref.Val via types.DefaultTypeAdapter
		want bool
	}{
		{"bool true", true, true},
		{"bool false", false, false},
		{"int64 nonzero", int64(42), true},
		{"int64 zero", int64(0), false},
		{"float64 nonzero", float64(3.14), true},
		{"float64 zero", float64(0), false},
		{"string nonempty", "hello", true},
		{"string empty", "", false},
		// NativeToValue(nil) wraps as NullValue whose .Value() is structpb.NullValue(0),
		// which hits the default branch → true. The nil case in isTruthy is for when
		// .Value() literally returns Go nil (e.g., custom ref.Val implementations).
		{"null value (cel null)", nil, true},
		{"unknown type (slice)", []int{1, 2}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			refVal := types.DefaultTypeAdapter.NativeToValue(tt.val)
			if got := isTruthy(refVal); got != tt.want {
				t.Errorf("isTruthy(%v) = %v, want %v", tt.val, got, tt.want)
			}
		})
	}
}

func TestTruncateForError(t *testing.T) {
	tests := []struct {
		name  string
		s     string
		limit int
		want  string
	}{
		{"short string", "hi", 10, "hi"},
		{"exact limit", "hello", 5, "hello"},
		{"over limit", "hello world", 5, "hello...(truncated)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := truncateForError(tt.s, tt.limit); got != tt.want {
				t.Errorf("truncateForError(%q, %d) = %q, want %q", tt.s, tt.limit, got, tt.want)
			}
		})
	}
}

func TestAllowFHIRWarnings(t *testing.T) {
	tests := []struct {
		name   string
		config map[string]string
		want   bool
	}{
		{"true", map[string]string{"allow_warnings": "true"}, true},
		{"TRUE", map[string]string{"allow_warnings": "TRUE"}, true},
		{"padded true", map[string]string{"allow_warnings": " true "}, true},
		{"false", map[string]string{"allow_warnings": "false"}, false},
		{"empty", map[string]string{"allow_warnings": ""}, false},
		{"missing key", map[string]string{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := allowFHIRWarnings(tt.config); got != tt.want {
				t.Errorf("allowFHIRWarnings(%v) = %v, want %v", tt.config, got, tt.want)
			}
		})
	}
}

func TestFhirValidationMode(t *testing.T) {
	tests := []struct {
		name   string
		config map[string]string
		want   string
	}{
		{"explicit validate_mode", map[string]string{"validate_mode": "strict"}, "strict"},
		{"profile us-core", map[string]string{"profile": "us-core"}, "us-core"},
		{"profile uscore", map[string]string{"profile": "uscore"}, "us-core"},
		{"profile us_core", map[string]string{"profile": "us_core"}, "us-core"},
		{"profile base", map[string]string{"profile": "base"}, "none"},
		{"profile none", map[string]string{"profile": "none"}, "none"},
		{"empty config", map[string]string{}, "us-core"},
		{"unknown profile defaults to us-core", map[string]string{"profile": "custom"}, "us-core"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fhirValidationMode(tt.config); got != tt.want {
				t.Errorf("fhirValidationMode(%v) = %q, want %q", tt.config, got, tt.want)
			}
		})
	}
}

func TestParseOptBool(t *testing.T) {
	tests := []struct {
		name       string
		value      string
		defaultVal bool
		want       bool
	}{
		{"empty with default true", "", true, true},
		{"empty with default false", "", false, false},
		{"true string", "true", false, true},
		{"TRUE string", "TRUE", false, true},
		{"false string", "false", true, false},
		{"random string", "maybe", true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseOptBool(tt.value, tt.defaultVal); got != tt.want {
				t.Errorf("parseOptBool(%q, %v) = %v, want %v", tt.value, tt.defaultVal, got, tt.want)
			}
		})
	}
}

func TestDetectEventType(t *testing.T) {
	tests := []struct {
		name  string
		event interface{}
		want  events.EventType
	}{
		{
			"map with type key",
			map[string]interface{}{"type": "patient_admit"},
			events.EventType("patient_admit"),
		},
		{
			"struct with json type tag",
			struct {
				Type string `json:"type"`
			}{Type: "observation"},
			events.EventType("observation"),
		},
		{
			"map without type key",
			map[string]interface{}{"name": "test"},
			events.EventType(""),
		},
		{
			"empty map",
			map[string]interface{}{},
			events.EventType(""),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := detectEventType(tt.event); got != tt.want {
				t.Errorf("detectEventType() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetNestedField(t *testing.T) {
	tests := []struct {
		name string
		m    map[string]interface{}
		path string
		want string
	}{
		{
			"single level",
			map[string]interface{}{"name": "test"},
			"name",
			"test",
		},
		{
			"dot nested",
			map[string]interface{}{"document": map[string]interface{}{"content": "hello"}},
			"document.content",
			"hello",
		},
		{
			"missing key",
			map[string]interface{}{"name": "test"},
			"missing",
			"",
		},
		{
			"non-map intermediate",
			map[string]interface{}{"name": "test"},
			"name.sub",
			"",
		},
		{
			"non-string leaf marshals to JSON",
			map[string]interface{}{"count": 42},
			"count",
			"42",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getNestedField(tt.m, tt.path); got != tt.want {
				t.Errorf("getNestedField(%v, %q) = %q, want %q", tt.m, tt.path, got, tt.want)
			}
		})
	}
}

func TestExtractTextFromEvent(t *testing.T) {
	tests := []struct {
		name    string
		event   interface{}
		config  map[string]string
		want    string
		wantErr bool
	}{
		{
			"configured text_field hits",
			map[string]interface{}{"body": "clinical note"},
			map[string]string{"text_field": "body"},
			"clinical note",
			false,
		},
		{
			"common path fallback (content)",
			map[string]interface{}{"content": "some content"},
			map[string]string{},
			"some content",
			false,
		},
		{
			"typed struct event",
			struct {
				Text string `json:"text"`
			}{Text: "struct text"},
			map[string]string{},
			"struct text",
			false,
		},
		{
			"no text found returns empty",
			map[string]interface{}{"id": "123"},
			map[string]string{},
			"",
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractTextFromEvent(tt.event, tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("extractTextFromEvent() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("extractTextFromEvent() = %q, want %q", got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Group 2: Validation functions — test error paths
// ---------------------------------------------------------------------------

func TestValidateExecAction(t *testing.T) {
	v, err := NewValidator()
	if err != nil {
		t.Fatalf("NewValidator() error = %v", err)
	}

	tests := []struct {
		name       string
		config     map[string]string
		wantCodes  []string
		wantErrors int
	}{
		{
			"missing command",
			map[string]string{"allowlist": "/bin/echo"},
			[]string{"MISSING_EXEC_COMMAND"},
			1,
		},
		{
			"missing allowlist",
			map[string]string{"command": "/bin/echo"},
			[]string{"MISSING_EXEC_ALLOWLIST"},
			1,
		},
		{
			"invalid timeout",
			map[string]string{"command": "/bin/echo", "allowlist": "/bin/echo", "timeout": "nope"},
			[]string{"INVALID_EXEC_TIMEOUT"},
			1,
		},
		{
			"invalid stdin mode",
			map[string]string{"command": "/bin/echo", "allowlist": "/bin/echo", "stdin": "xml"},
			[]string{"INVALID_EXEC_STDIN"},
			1,
		},
		{
			"valid config",
			map[string]string{"command": "/bin/echo hello", "allowlist": "/bin/echo", "timeout": "5s", "stdin": "json"},
			nil,
			0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := &ValidationResult{Valid: true}
			action := &Action{Type: "exec", Config: tt.config}
			v.validateExecAction(action, "test", result)

			if len(result.Errors) != tt.wantErrors {
				t.Errorf("got %d errors, want %d: %v", len(result.Errors), tt.wantErrors, result.Errors)
			}
			for _, code := range tt.wantCodes {
				found := false
				for _, e := range result.Errors {
					if e.Code == code {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected error code %q not found in %v", code, result.Errors)
				}
			}
		})
	}
}

func TestValidateEmailAction(t *testing.T) {
	v, err := NewValidator()
	if err != nil {
		t.Fatalf("NewValidator() error = %v", err)
	}

	validConfig := map[string]string{
		"smtp_host": "mail.example.com",
		"smtp_port": "587",
		"from":      "sender@example.com",
		"to":        "recipient@example.com",
		"subject":   "Alert",
	}

	tests := []struct {
		name         string
		config       map[string]string
		wantCodes    []string
		wantErrors   int
		wantWarnings int
	}{
		{
			"missing all required",
			map[string]string{},
			[]string{"MISSING_SMTP_HOST", "MISSING_SMTP_PORT", "MISSING_EMAIL_FROM", "MISSING_EMAIL_TO", "MISSING_EMAIL_SUBJECT"},
			5,
			0,
		},
		{
			"non-numeric port",
			map[string]string{"smtp_host": "mail", "smtp_port": "abc", "from": "a@b", "to": "c@d", "subject": "hi"},
			[]string{"INVALID_SMTP_PORT"},
			1,
			0,
		},
		{
			"invalid starttls warning",
			func() map[string]string {
				c := copyMap(validConfig)
				c["starttls"] = "maybe"
				return c
			}(),
			[]string{"INVALID_STARTTLS"},
			0,
			1,
		},
		{
			"invalid tls_insecure warning",
			func() map[string]string {
				c := copyMap(validConfig)
				c["tls_insecure"] = "yes"
				return c
			}(),
			[]string{"INVALID_TLS_INSECURE"},
			0,
			1,
		},
		{
			"invalid timeout",
			func() map[string]string {
				c := copyMap(validConfig)
				c["timeout"] = "not-a-duration"
				return c
			}(),
			[]string{"INVALID_EMAIL_TIMEOUT"},
			1,
			0,
		},
		{
			"valid config",
			validConfig,
			nil,
			0,
			0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := &ValidationResult{Valid: true}
			action := &Action{Type: "email", Config: tt.config}
			v.validateEmailAction(action, "test", result)

			if len(result.Errors) != tt.wantErrors {
				t.Errorf("got %d errors, want %d: %v", len(result.Errors), tt.wantErrors, result.Errors)
			}
			if len(result.Warnings) != tt.wantWarnings {
				t.Errorf("got %d warnings, want %d: %v", len(result.Warnings), tt.wantWarnings, result.Warnings)
			}
			for _, code := range tt.wantCodes {
				found := false
				for _, e := range result.Errors {
					if e.Code == code {
						found = true
						break
					}
				}
				if !found {
					for _, w := range result.Warnings {
						if w.Code == code {
							found = true
							break
						}
					}
				}
				if !found {
					t.Errorf("expected code %q not found", code)
				}
			}
		})
	}
}

func TestValidateFileAction(t *testing.T) {
	v, err := NewValidator()
	if err != nil {
		t.Fatalf("NewValidator() error = %v", err)
	}

	tests := []struct {
		name       string
		config     map[string]string
		wantCodes  []string
		wantErrors int
	}{
		{
			"missing path",
			map[string]string{},
			[]string{"MISSING_FILE_PATH"},
			1,
		},
		{
			"invalid format",
			map[string]string{"path": "/tmp/out", "format": "xml"},
			[]string{"INVALID_FILE_FORMAT"},
			1,
		},
		{
			"invalid perm",
			map[string]string{"path": "/tmp/out", "perm": "notoctal"},
			[]string{"INVALID_FILE_PERM"},
			1,
		},
		{
			"valid config",
			map[string]string{"path": "/tmp/out.json", "format": "json", "perm": "0644"},
			nil,
			0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := &ValidationResult{Valid: true}
			action := &Action{Type: "file", Config: tt.config}
			v.validateFileAction(action, "test", result)

			if len(result.Errors) != tt.wantErrors {
				t.Errorf("got %d errors, want %d: %v", len(result.Errors), tt.wantErrors, result.Errors)
			}
			for _, code := range tt.wantCodes {
				found := false
				for _, e := range result.Errors {
					if e.Code == code {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected error code %q not found", code)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Group 3: Type methods & event enrichment
// ---------------------------------------------------------------------------

func TestValidationErrorFormat(t *testing.T) {
	e := ValidationError{Path: "routes[0].filter", Message: "invalid condition"}
	want := "routes[0].filter: invalid condition"
	if got := e.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestValidationResultHasErrors(t *testing.T) {
	tests := []struct {
		name   string
		result ValidationResult
		want   bool
	}{
		{"empty result", ValidationResult{}, false},
		{"warnings only", ValidationResult{Warnings: []ValidationError{{}}}, false},
		{"with errors", ValidationResult{Errors: []ValidationError{{}}}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.result.HasErrors(); got != tt.want {
				t.Errorf("HasErrors() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAddExtractedEntitiesToEvent(t *testing.T) {
	t.Run("map with existing meta", func(t *testing.T) {
		event := map[string]interface{}{
			"type": "clinical_note",
			"meta": map[string]interface{}{"source": "hl7"},
		}
		result := &extract.ExtractionResult{
			Conditions: []events.ExtractedCondition{{Code: "E11", Name: "Diabetes"}},
			Confidence: 0.95,
			Model:      "gpt-4",
		}
		if err := addExtractedEntitiesToEvent(event, result); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		meta := event["meta"].(map[string]interface{})
		if _, ok := meta["extracted_entities"]; !ok {
			t.Error("expected extracted_entities in meta")
		}
		if meta["source"] != "hl7" {
			t.Error("existing meta key was overwritten")
		}
	})

	t.Run("map without meta", func(t *testing.T) {
		event := map[string]interface{}{"type": "note"}
		result := &extract.ExtractionResult{Confidence: 0.8}
		if err := addExtractedEntitiesToEvent(event, result); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		meta, ok := event["meta"].(map[string]interface{})
		if !ok {
			t.Fatal("expected meta to be created")
		}
		if _, ok := meta["extracted_entities"]; !ok {
			t.Error("expected extracted_entities in meta")
		}
	})

	t.Run("non-map event returns nil", func(t *testing.T) {
		event := struct{ Type string }{Type: "note"}
		result := &extract.ExtractionResult{Confidence: 0.5}
		if err := addExtractedEntitiesToEvent(event, result); err != nil {
			t.Errorf("expected nil error for non-map, got: %v", err)
		}
	})
}

func TestAddQualityScoreToEvent(t *testing.T) {
	t.Run("priority conversion", func(t *testing.T) {
		event := map[string]interface{}{"type": "test"}
		score := &quality.DataQualityScore{
			OverallScore: 0.75,
			Dimensions:   map[string]float64{"completeness": 0.8},
			Recommendations: []quality.QualityRecommendation{
				{Priority: 1, Category: "completeness", Title: "Add fields", Description: "Missing required fields", Impact: "Improves interop"},
				{Priority: 3, Category: "accuracy", Title: "Review codes", Description: "Some codes are stale", Impact: "Better matching"},
				{Priority: 4, Category: "timeliness", Title: "Reduce lag", Description: "Data is delayed", Impact: "Faster alerts"},
			},
			Metadata: quality.QualityMetadata{
				AnalyzedAt: time.Now(),
				Model:      "gpt-4",
			},
		}

		if err := addQualityScoreToEvent(event, score); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		meta, ok := event["meta"].(map[string]interface{})
		if !ok {
			t.Fatal("expected meta to be created")
		}
		qs, ok := meta["quality_score"].(events.DataQualityScore)
		if !ok {
			t.Fatal("expected quality_score in meta")
		}
		if qs.OverallScore != 0.75 {
			t.Errorf("OverallScore = %v, want 0.75", qs.OverallScore)
		}
		if len(qs.Recommendations) != 3 {
			t.Fatalf("expected 3 recommendations, got %d", len(qs.Recommendations))
		}
		// Priority 1 → "high"
		if qs.Recommendations[0].Priority != "high" {
			t.Errorf("rec[0].Priority = %q, want %q", qs.Recommendations[0].Priority, "high")
		}
		// Priority 3 → "medium"
		if qs.Recommendations[1].Priority != "medium" {
			t.Errorf("rec[1].Priority = %q, want %q", qs.Recommendations[1].Priority, "medium")
		}
		// Priority 4 → "low"
		if qs.Recommendations[2].Priority != "low" {
			t.Errorf("rec[2].Priority = %q, want %q", qs.Recommendations[2].Priority, "low")
		}
	})

	t.Run("non-map event returns nil", func(t *testing.T) {
		event := struct{ ID string }{ID: "123"}
		score := &quality.DataQualityScore{OverallScore: 0.5}
		if err := addQualityScoreToEvent(event, score); err != nil {
			t.Errorf("expected nil error for non-map, got: %v", err)
		}
	})
}

// ---------------------------------------------------------------------------
// Group 4: Engine getter/setter coverage
// ---------------------------------------------------------------------------

func TestEngineSetGetMetrics(t *testing.T) {
	wf := &Workflow{Name: "test", Routes: []Route{}}
	engine, err := NewEngine(wf)
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	// Default is NoOpMetrics
	if _, ok := engine.GetMetrics().(*NoOpMetrics); !ok {
		t.Error("expected default metrics to be *NoOpMetrics")
	}

	// Set nil → falls back to NoOpMetrics
	engine.SetMetrics(nil)
	if _, ok := engine.GetMetrics().(*NoOpMetrics); !ok {
		t.Error("SetMetrics(nil) should fall back to *NoOpMetrics")
	}

	// Set custom metrics
	custom := &NoOpMetrics{} // reuse as a distinguishable instance
	engine.SetMetrics(custom)
	if engine.GetMetrics() != custom {
		t.Error("SetMetrics did not persist custom metrics")
	}
}

func TestEngineSetGetTracer(t *testing.T) {
	wf := &Workflow{Name: "test", Routes: []Route{}}
	engine, err := NewEngine(wf)
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	// Default is NoOpTracer
	if _, ok := engine.GetTracer().(*NoOpTracer); !ok {
		t.Error("expected default tracer to be *NoOpTracer")
	}

	// Set nil → falls back to NoOpTracer
	engine.SetTracer(nil)
	if _, ok := engine.GetTracer().(*NoOpTracer); !ok {
		t.Error("SetTracer(nil) should fall back to *NoOpTracer")
	}

	// Set custom tracer
	custom := &NoOpTracer{}
	engine.SetTracer(custom)
	if engine.GetTracer() != custom {
		t.Error("SetTracer did not persist custom tracer")
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func copyMap(m map[string]string) map[string]string {
	c := make(map[string]string, len(m))
	for k, v := range m {
		c[k] = v
	}
	return c
}
