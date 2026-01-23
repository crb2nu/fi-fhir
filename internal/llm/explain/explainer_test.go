package explain

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/llm"
)

func TestNewWarningExplainer(t *testing.T) {
	t.Run("returns error when client is nil", func(t *testing.T) {
		_, err := NewWarningExplainer(ExplainerConfig{Client: nil})
		if err == nil {
			t.Fatal("expected error when client is nil")
		}
	})

	t.Run("creates explainer with valid config", func(t *testing.T) {
		client := llm.NewMockClient()
		e, err := NewWarningExplainer(ExplainerConfig{Client: client})
		if err != nil {
			t.Fatalf("NewWarningExplainer error: %v", err)
		}
		if e == nil {
			t.Fatal("expected non-nil explainer")
		}
	})

	t.Run("creates cache when enabled", func(t *testing.T) {
		client := llm.NewMockClient()
		e, err := NewWarningExplainer(ExplainerConfig{
			Client:      client,
			EnableCache: true,
			CacheTTL:    5 * time.Minute,
		})
		if err != nil {
			t.Fatalf("NewWarningExplainer error: %v", err)
		}
		if e.cache == nil {
			t.Error("expected cache to be created")
		}
	})

	t.Run("uses default cache TTL when not specified", func(t *testing.T) {
		client := llm.NewMockClient()
		e, err := NewWarningExplainer(ExplainerConfig{
			Client:      client,
			EnableCache: true,
		})
		if err != nil {
			t.Fatalf("NewWarningExplainer error: %v", err)
		}
		if e.cache == nil {
			t.Fatal("expected cache to be created")
		}
		if e.cache.ttl != 24*time.Hour {
			t.Errorf("cache.ttl = %v, want 24h", e.cache.ttl)
		}
	})
}

func TestWarningExplainer_Explain(t *testing.T) {
	ctx := context.Background()

	t.Run("returns template for known warning codes", func(t *testing.T) {
		client := llm.NewMockClient()
		e, _ := NewWarningExplainer(ExplainerConfig{Client: client})

		warning := events.ParseWarning{
			Code:     "INVALID_NPI",
			Message:  "NPI validation failed",
			Severity: "warning",
		}

		result, err := e.Explain(ctx, warning, events.FormatHL7v2)
		if err != nil {
			t.Fatalf("Explain error: %v", err)
		}

		// Should use template, not call LLM
		if client.CallCount() != 0 {
			t.Error("expected no LLM call for templated warning")
		}

		if result.Explanation == "" {
			t.Error("expected non-empty explanation")
		}
		if result.FixSuggestion == "" {
			t.Error("expected non-empty fix suggestion")
		}
		if !result.FromCache {
			t.Error("expected FromCache to be true for template")
		}
	})

	t.Run("calls LLM for unknown warning codes", func(t *testing.T) {
		client := llm.NewMockClient()
		client.WithJSONResponse(map[string]interface{}{
			"explanation":    "This is an unknown warning",
			"fix_suggestion": "Contact support",
			"impact":         "Unknown impact",
		})

		e, _ := NewWarningExplainer(ExplainerConfig{Client: client})

		warning := events.ParseWarning{
			Code:     "UNKNOWN_WARNING_CODE",
			Message:  "Something went wrong",
			Severity: "warning",
		}

		result, err := e.Explain(ctx, warning, events.FormatHL7v2)
		if err != nil {
			t.Fatalf("Explain error: %v", err)
		}

		if client.CallCount() != 1 {
			t.Errorf("CallCount = %d, want 1", client.CallCount())
		}

		if result.Explanation != "This is an unknown warning" {
			t.Errorf("Explanation = %v", result.Explanation)
		}
		if result.FixSuggestion != "Contact support" {
			t.Errorf("FixSuggestion = %v", result.FixSuggestion)
		}
	})

	t.Run("returns error on LLM failure", func(t *testing.T) {
		client := llm.NewMockClient()
		client.WithError(errors.New("LLM unavailable"))

		e, _ := NewWarningExplainer(ExplainerConfig{Client: client})

		warning := events.ParseWarning{
			Code:    "UNKNOWN_CODE",
			Message: "Test message",
		}

		_, err := e.Explain(ctx, warning, events.FormatHL7v2)
		if err == nil {
			t.Fatal("expected error on LLM failure")
		}
	})

	t.Run("uses cache for repeated unknown warnings", func(t *testing.T) {
		client := llm.NewMockClient()
		client.WithJSONResponse(map[string]interface{}{
			"explanation": "Cached explanation",
		})

		e, _ := NewWarningExplainer(ExplainerConfig{
			Client:      client,
			EnableCache: true,
			CacheTTL:    1 * time.Minute,
		})

		warning := events.ParseWarning{
			Code:     "CUSTOM_CODE",
			Message:  "Custom message",
			Phase:    "validation",
			Severity: "warning",
		}

		// First call
		result1, err := e.Explain(ctx, warning, events.FormatHL7v2)
		if err != nil {
			t.Fatalf("Explain error: %v", err)
		}

		// Second call - should use cache
		result2, err := e.Explain(ctx, warning, events.FormatHL7v2)
		if err != nil {
			t.Fatalf("Explain error: %v", err)
		}

		if client.CallCount() != 1 {
			t.Errorf("CallCount = %d, want 1 (cached)", client.CallCount())
		}

		if !result2.FromCache {
			t.Error("expected FromCache to be true")
		}

		if result1.Explanation != result2.Explanation {
			t.Error("cached result differs from original")
		}
	})
}

func TestWarningExplainer_ExplainBatch(t *testing.T) {
	ctx := context.Background()

	t.Run("returns nil for empty batch", func(t *testing.T) {
		client := llm.NewMockClient()
		e, _ := NewWarningExplainer(ExplainerConfig{Client: client})

		results, err := e.ExplainBatch(ctx, nil, events.FormatHL7v2)
		if err != nil {
			t.Fatalf("ExplainBatch error: %v", err)
		}
		if results != nil {
			t.Error("expected nil for empty batch")
		}
	})

	t.Run("explains multiple warnings", func(t *testing.T) {
		client := llm.NewMockClient()
		client.WithJSONResponse(map[string]interface{}{
			"explanation": "Generic explanation",
		})

		e, _ := NewWarningExplainer(ExplainerConfig{Client: client})

		warnings := []events.ParseWarning{
			{Code: "INVALID_NPI", Message: "NPI error"},   // Template
			{Code: "CUSTOM_CODE", Message: "Custom"},      // LLM
			{Code: "MISSING_PV1", Message: "PV1 missing"}, // Template
		}

		results, err := e.ExplainBatch(ctx, warnings, events.FormatHL7v2)
		if err != nil {
			t.Fatalf("ExplainBatch error: %v", err)
		}

		if len(results) != 3 {
			t.Fatalf("len(results) = %d, want 3", len(results))
		}

		// First and third should be templates, second should be LLM
		if client.CallCount() != 1 {
			t.Errorf("CallCount = %d, want 1 (2 templates, 1 LLM)", client.CallCount())
		}

		// All should have explanations
		for i, r := range results {
			if r.Explanation == "" {
				t.Errorf("results[%d].Explanation is empty", i)
			}
		}
	})

	t.Run("continues on individual warning error", func(t *testing.T) {
		callCount := 0
		client := llm.NewMockClient()
		client.CompleteStructuredFunc = func(ctx context.Context, req llm.CompletionRequest, schemaName string, schema interface{}) (json.RawMessage, error) {
			callCount++
			if callCount == 1 {
				return nil, errors.New("first call fails")
			}
			return json.RawMessage(`{"explanation": "success"}`), nil
		}

		e, _ := NewWarningExplainer(ExplainerConfig{Client: client})

		warnings := []events.ParseWarning{
			{Code: "CUSTOM1", Message: "First custom"},  // Will fail
			{Code: "CUSTOM2", Message: "Second custom"}, // Will succeed
		}

		results, err := e.ExplainBatch(ctx, warnings, events.FormatHL7v2)
		if err != nil {
			t.Fatalf("ExplainBatch should not return error: %v", err)
		}

		if len(results) != 2 {
			t.Fatalf("len(results) = %d, want 2", len(results))
		}

		// First result should have error in explanation
		if results[0].Explanation == "" {
			t.Error("results[0].Explanation should contain error message")
		}

		// Second result should succeed
		if results[1].Explanation != "success" {
			t.Errorf("results[1].Explanation = %v, want 'success'", results[1].Explanation)
		}
	})
}

func TestGetWarningTemplates(t *testing.T) {
	templates := getWarningTemplates()

	expectedCodes := []string{
		"INVALID_NPI",
		"INVALID_MBI",
		"INVALID_SSN",
		"MISSING_PV1",
		"MISSING_PID",
		"EMPTY_MRN",
		"INVALID_DATE",
		"UNKNOWN_MESSAGE_TYPE",
		"SEGMENT_TRUNCATED",
		"DUPLICATE_IDENTIFIER",
		"CODE_NOT_MAPPED",
	}

	for _, code := range expectedCodes {
		if _, ok := templates[code]; !ok {
			t.Errorf("missing template for code: %s", code)
		} else {
			tmpl := templates[code]
			if tmpl.Explanation == "" {
				t.Errorf("template %s has empty Explanation", code)
			}
			if tmpl.FixSuggestion == "" {
				t.Errorf("template %s has empty FixSuggestion", code)
			}
			if tmpl.Impact == "" {
				t.Errorf("template %s has empty Impact", code)
			}
		}
	}
}

func TestCacheKey(t *testing.T) {
	client := llm.NewMockClient()
	e, _ := NewWarningExplainer(ExplainerConfig{Client: client})

	warning := events.ParseWarning{
		Code:     "TEST_CODE",
		Phase:    "validation",
		Severity: "warning",
	}

	key := e.cacheKey(warning)
	expected := "TEST_CODE:validation:warning"
	if key != expected {
		t.Errorf("cacheKey = %v, want %v", key, expected)
	}
}

func TestExplanationCache(t *testing.T) {
	t.Run("get returns false for missing key", func(t *testing.T) {
		cache := newExplanationCache(1 * time.Hour)
		_, ok := cache.get("missing")
		if ok {
			t.Error("expected false for missing key")
		}
	})

	t.Run("get returns value for existing key", func(t *testing.T) {
		cache := newExplanationCache(1 * time.Hour)
		result := &ExplainedWarning{Explanation: "test"}
		cache.set("key", result)

		got, ok := cache.get("key")
		if !ok {
			t.Fatal("expected true for existing key")
		}
		if got.Explanation != "test" {
			t.Errorf("Explanation = %v, want 'test'", got.Explanation)
		}
	})

	t.Run("get returns false for expired entry", func(t *testing.T) {
		cache := newExplanationCache(1 * time.Millisecond)
		result := &ExplainedWarning{Explanation: "test"}
		cache.set("key", result)

		// Wait for expiration
		time.Sleep(5 * time.Millisecond)

		_, ok := cache.get("key")
		if ok {
			t.Error("expected false for expired entry")
		}
	})
}

func TestBuildExplainerUserPrompt(t *testing.T) {
	warning := events.ParseWarning{
		Code:     "TEST_CODE",
		Message:  "Test message",
		Path:     "PID-3",
		Phase:    "validation",
		Severity: "warning",
	}

	prompt := buildExplainerUserPrompt(warning, events.FormatHL7v2)

	// Check that all fields are included
	if prompt == "" {
		t.Fatal("prompt should not be empty")
	}
	if !containsAll(prompt, "Format: hl7v2", "Phase: validation", "Code: TEST_CODE", "Message: Test message", "Path: PID-3", "Severity: warning") {
		t.Errorf("prompt missing expected fields: %s", prompt)
	}
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !contains(s, sub) {
			return false
		}
	}
	return true
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
