package quality

import (
	"context"
	"errors"
	"testing"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/llm"
)

func TestNewAnalyzer(t *testing.T) {
	t.Run("returns error when client is nil", func(t *testing.T) {
		_, err := NewAnalyzer(AnalyzerConfig{Client: nil})
		if err == nil {
			t.Fatal("expected error when client is nil")
		}
	})

	t.Run("creates analyzer with valid config", func(t *testing.T) {
		client := llm.NewMockClient()
		a, err := NewAnalyzer(AnalyzerConfig{Client: client})
		if err != nil {
			t.Fatalf("NewAnalyzer error: %v", err)
		}
		if a == nil {
			t.Fatal("expected non-nil analyzer")
		}
	})

	t.Run("uses specified model", func(t *testing.T) {
		client := llm.NewMockClient()
		a, err := NewAnalyzer(AnalyzerConfig{
			Client: client,
			Model:  "test-model",
		})
		if err != nil {
			t.Fatalf("NewAnalyzer error: %v", err)
		}
		if a.model != "test-model" {
			t.Errorf("model = %v, want test-model", a.model)
		}
	})
}

func TestAnalyzer_AnalyzeEvent(t *testing.T) {
	ctx := context.Background()

	t.Run("analyzes event successfully", func(t *testing.T) {
		client := llm.NewMockClient()
		client.WithJSONResponse(map[string]interface{}{
			"overall_score": 0.85,
			"dimensions": map[string]interface{}{
				"completeness": 0.90,
				"accuracy":     0.85,
				"consistency":  0.80,
				"conformance":  0.88,
				"timeliness":   0.82,
			},
			"issues": []map[string]interface{}{
				{
					"dimension":   "completeness",
					"severity":    "medium",
					"field":       "patient.phone",
					"description": "Phone number missing",
				},
			},
			"recommendations": []map[string]interface{}{
				{
					"priority":    1,
					"category":    "data_collection",
					"title":       "Add phone number",
					"description": "Ensure phone is collected at registration",
					"impact":      "Improves completeness score",
				},
			},
		})

		a, _ := NewAnalyzer(AnalyzerConfig{Client: client, Model: "test-model"})

		event := map[string]interface{}{
			"type":    "patient_admit",
			"patient": map[string]interface{}{"name": "John Doe"},
		}

		result, err := a.AnalyzeEvent(ctx, event, events.EventPatientAdmit)
		if err != nil {
			t.Fatalf("AnalyzeEvent error: %v", err)
		}

		if result.OverallScore != 0.85 {
			t.Errorf("OverallScore = %v, want 0.85", result.OverallScore)
		}

		if result.Dimensions["completeness"] != 0.90 {
			t.Errorf("Dimensions[completeness] = %v, want 0.90", result.Dimensions["completeness"])
		}

		if len(result.Issues) != 1 {
			t.Fatalf("len(Issues) = %d, want 1", len(result.Issues))
		}
		if result.Issues[0].Field != "patient.phone" {
			t.Errorf("Issues[0].Field = %v", result.Issues[0].Field)
		}

		if len(result.Recommendations) != 1 {
			t.Fatalf("len(Recommendations) = %d, want 1", len(result.Recommendations))
		}
		if result.Recommendations[0].Priority != 1 {
			t.Errorf("Recommendations[0].Priority = %v", result.Recommendations[0].Priority)
		}

		// Check metadata
		if result.Metadata.Model != "test-model" {
			t.Errorf("Metadata.Model = %v, want test-model", result.Metadata.Model)
		}
		if result.Metadata.EventType != string(events.EventPatientAdmit) {
			t.Errorf("Metadata.EventType = %v", result.Metadata.EventType)
		}
		if result.Metadata.ProcessingTime == 0 {
			t.Error("ProcessingTime should be > 0")
		}
	})

	t.Run("returns error on LLM failure", func(t *testing.T) {
		client := llm.NewMockClient()
		client.WithError(errors.New("LLM unavailable"))

		a, _ := NewAnalyzer(AnalyzerConfig{Client: client})

		event := map[string]interface{}{"type": "test"}
		_, err := a.AnalyzeEvent(ctx, event, events.EventPatientAdmit)
		if err == nil {
			t.Fatal("expected error on LLM failure")
		}
	})

	t.Run("handles struct event", func(t *testing.T) {
		client := llm.NewMockClient()
		client.WithJSONResponse(map[string]interface{}{
			"overall_score": 0.75,
			"dimensions":    map[string]interface{}{},
		})

		a, _ := NewAnalyzer(AnalyzerConfig{Client: client})

		type TestEvent struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		}

		event := TestEvent{Type: "test", Message: "hello"}
		result, err := a.AnalyzeEvent(ctx, event, events.EventType("test"))
		if err != nil {
			t.Fatalf("AnalyzeEvent error: %v", err)
		}

		if result.OverallScore != 0.75 {
			t.Errorf("OverallScore = %v, want 0.75", result.OverallScore)
		}
	})
}

func TestQuickScore(t *testing.T) {
	t.Run("calculates completeness for map event", func(t *testing.T) {
		event := map[string]interface{}{
			"name":   "John",
			"age":    30,
			"email":  "", // Empty string
			"phone":  nil,
			"active": true,
		}

		score := QuickScore(event)

		// 3 out of 5 fields are non-empty (name, age, active)
		expectedCompleteness := 0.6
		if score.Dimensions[DimensionCompleteness] != expectedCompleteness {
			t.Errorf("Completeness = %v, want %v", score.Dimensions[DimensionCompleteness], expectedCompleteness)
		}

		if score.OverallScore != expectedCompleteness {
			t.Errorf("OverallScore = %v, want %v", score.OverallScore, expectedCompleteness)
		}
	})

	t.Run("handles nested map", func(t *testing.T) {
		event := map[string]interface{}{
			"patient": map[string]interface{}{
				"name": "John",
				"dob":  "1990-01-01",
			},
		}

		score := QuickScore(event)

		// Should count nested fields
		if score.OverallScore == 0 {
			t.Error("OverallScore should be > 0 for nested map")
		}
	})

	t.Run("handles struct event", func(t *testing.T) {
		type Person struct {
			Name  string `json:"name"`
			Email string `json:"email"`
		}

		event := Person{Name: "John", Email: ""}

		score := QuickScore(event)

		// name is set, email is empty
		if score.OverallScore == 0 {
			t.Error("OverallScore should be > 0")
		}
	})

	t.Run("handles empty map", func(t *testing.T) {
		event := map[string]interface{}{}

		score := QuickScore(event)

		if score.Dimensions[DimensionCompleteness] != 0 {
			t.Errorf("Completeness = %v, want 0 for empty map", score.Dimensions[DimensionCompleteness])
		}
	})

	t.Run("handles slice values", func(t *testing.T) {
		event := map[string]interface{}{
			"items":       []interface{}{"a", "b"},
			"empty_items": []interface{}{},
		}

		score := QuickScore(event)

		// items has elements, empty_items doesn't
		if score.Dimensions[DimensionCompleteness] != 0.5 {
			t.Errorf("Completeness = %v, want 0.5", score.Dimensions[DimensionCompleteness])
		}
	})
}

func TestDimensionConstants(t *testing.T) {
	// Verify dimension constants are defined correctly
	if DimensionCompleteness != "completeness" {
		t.Errorf("DimensionCompleteness = %v", DimensionCompleteness)
	}
	if DimensionAccuracy != "accuracy" {
		t.Errorf("DimensionAccuracy = %v", DimensionAccuracy)
	}
	if DimensionConsistency != "consistency" {
		t.Errorf("DimensionConsistency = %v", DimensionConsistency)
	}
	if DimensionConformance != "conformance" {
		t.Errorf("DimensionConformance = %v", DimensionConformance)
	}
	if DimensionTimeliness != "timeliness" {
		t.Errorf("DimensionTimeliness = %v", DimensionTimeliness)
	}
}

func TestBuildQualityPrompts(t *testing.T) {
	t.Run("system prompt contains dimension descriptions", func(t *testing.T) {
		prompt := buildQualitySystemPrompt()
		if prompt == "" {
			t.Fatal("system prompt should not be empty")
		}
		// Check it mentions key dimensions
		if !contains(prompt, "COMPLETENESS") || !contains(prompt, "ACCURACY") {
			t.Error("system prompt should mention quality dimensions")
		}
	})

	t.Run("user prompt includes event JSON", func(t *testing.T) {
		eventJSON := `{"type": "test"}`
		prompt := buildQualityUserPrompt(eventJSON, "patient_admit")

		if !contains(prompt, eventJSON) {
			t.Error("user prompt should include event JSON")
		}
		if !contains(prompt, "patient_admit") {
			t.Error("user prompt should include event type")
		}
	})
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
