package copilot

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/llm"
)

// makeCELGenJSON builds a valid CEL generation JSON response.
func makeCELGenJSON(expression, explanation string, alternatives, fieldsUsed []string) json.RawMessage {
	resp := map[string]interface{}{
		"expression":  expression,
		"explanation": explanation,
	}
	if alternatives != nil {
		resp["alternatives"] = alternatives
	}
	if fieldsUsed != nil {
		resp["fields_used"] = fieldsUsed
	}
	b, _ := json.Marshal(resp)
	return b
}

// makeValidationJSON builds a valid CEL validation JSON response.
func makeValidationJSON(valid bool, errMsg, corrected string, suggestions []string) json.RawMessage {
	resp := map[string]interface{}{
		"valid": valid,
	}
	if errMsg != "" {
		resp["error"] = errMsg
	}
	if corrected != "" {
		resp["corrected"] = corrected
	}
	if suggestions != nil {
		resp["suggestions"] = suggestions
	}
	b, _ := json.Marshal(resp)
	return b
}

// --- CELAssistant constructor tests ---

func TestNewCELAssistant(t *testing.T) {
	mock := llm.NewMockClient()
	assistant := NewCELAssistant(mock, "test-model")
	if assistant == nil {
		t.Fatal("assistant should not be nil")
	}
	if assistant.client != mock {
		t.Error("assistant should store the provided client")
	}
	if assistant.model != "test-model" {
		t.Errorf("expected model 'test-model', got %q", assistant.model)
	}
}

// --- CEL Generate tests ---

func TestCELGenerate_Valid(t *testing.T) {
	mock := llm.NewMockClient()
	mock.CompleteStructuredFunc = func(ctx context.Context, req llm.CompletionRequest, schemaName string, schema interface{}) (json.RawMessage, error) {
		return makeCELGenJSON(
			`event.patient.age >= 65`,
			"Matches patients 65 or older",
			[]string{`event.patient.age > 64`},
			[]string{"event.patient.age"},
		), nil
	}

	assistant := NewCELAssistant(mock, "test-model")
	result, err := assistant.Generate(context.Background(), GenerateCELRequest{
		Description: "patients over 65",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Expression != `event.patient.age >= 65` {
		t.Errorf("unexpected expression: %q", result.Expression)
	}
	if result.Explanation != "Matches patients 65 or older" {
		t.Errorf("unexpected explanation: %q", result.Explanation)
	}
	if len(result.Alternatives) != 1 {
		t.Fatalf("expected 1 alternative, got %d", len(result.Alternatives))
	}
	if len(result.FieldsUsed) != 1 || result.FieldsUsed[0] != "event.patient.age" {
		t.Errorf("unexpected fields_used: %v", result.FieldsUsed)
	}
}

func TestCELGenerate_EmptyDescription(t *testing.T) {
	mock := llm.NewMockClient()
	assistant := NewCELAssistant(mock, "test-model")

	_, err := assistant.Generate(context.Background(), GenerateCELRequest{Description: ""})
	if err == nil {
		t.Fatal("expected error for empty description")
	}
	if err.Error() != "description is required" {
		t.Errorf("expected 'description is required', got %q", err.Error())
	}
	if mock.CallCount() != 0 {
		t.Error("LLM should not be called for empty description")
	}
}

func TestCELGenerate_LLMError(t *testing.T) {
	mock := llm.NewMockClient().WithError(fmt.Errorf("service unavailable"))
	assistant := NewCELAssistant(mock, "test-model")

	_, err := assistant.Generate(context.Background(), GenerateCELRequest{Description: "test"})
	if err == nil {
		t.Fatal("expected error from LLM")
	}
	if !strings.Contains(err.Error(), "generate cel:") {
		t.Errorf("expected error wrapped with 'generate cel:', got %q", err.Error())
	}
}

func TestCELGenerate_MalformedJSON(t *testing.T) {
	mock := llm.NewMockClient()
	mock.CompleteStructuredFunc = func(ctx context.Context, req llm.CompletionRequest, schemaName string, schema interface{}) (json.RawMessage, error) {
		return json.RawMessage(`{broken`), nil
	}

	assistant := NewCELAssistant(mock, "test-model")
	_, err := assistant.Generate(context.Background(), GenerateCELRequest{Description: "test"})
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
	if !strings.Contains(err.Error(), "parse response:") {
		t.Errorf("expected 'parse response:' error, got %q", err.Error())
	}
}

func TestCELGenerate_RequestParameters(t *testing.T) {
	mock := llm.NewMockClient()
	var capturedSchemaName string
	mock.CompleteStructuredFunc = func(ctx context.Context, req llm.CompletionRequest, schemaName string, schema interface{}) (json.RawMessage, error) {
		capturedSchemaName = schemaName
		return makeCELGenJSON("true", "always matches", nil, nil), nil
	}

	assistant := NewCELAssistant(mock, "gpt-4o")
	_, err := assistant.Generate(context.Background(), GenerateCELRequest{Description: "match everything"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	call := mock.LastCall()
	if call == nil {
		t.Fatal("expected at least one call")
	}
	if call.Temperature != 0.1 {
		t.Errorf("expected temperature 0.1, got %v", call.Temperature)
	}
	if call.MaxTokens != 1024 {
		t.Errorf("expected max tokens 1024, got %d", call.MaxTokens)
	}
	if call.Model != "gpt-4o" {
		t.Errorf("expected model 'gpt-4o', got %q", call.Model)
	}
	if capturedSchemaName != "cel_generation" {
		t.Errorf("expected schema name 'cel_generation', got %q", capturedSchemaName)
	}
}

func TestCELGenerate_WithEventSchema(t *testing.T) {
	mock := llm.NewMockClient()
	mock.CompleteStructuredFunc = func(ctx context.Context, req llm.CompletionRequest, schemaName string, schema interface{}) (json.RawMessage, error) {
		return makeCELGenJSON("true", "ok", nil, nil), nil
	}

	assistant := NewCELAssistant(mock, "test-model")
	_, err := assistant.Generate(context.Background(), GenerateCELRequest{
		Description: "test",
		EventSchema: &EventSchema{
			Fields: map[string]string{
				"event.custom.field": "A custom field for testing",
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	call := mock.LastCall()
	systemPrompt := call.Messages[0].Content
	if !strings.Contains(systemPrompt, "Custom Fields") {
		t.Error("system prompt should contain 'Custom Fields' section when schema has fields")
	}
	if !strings.Contains(systemPrompt, "event.custom.field") {
		t.Error("system prompt should contain the custom field path")
	}
	if !strings.Contains(systemPrompt, "A custom field for testing") {
		t.Error("system prompt should contain the custom field description")
	}
}

func TestCELGenerate_WithExamples(t *testing.T) {
	mock := llm.NewMockClient()
	mock.CompleteStructuredFunc = func(ctx context.Context, req llm.CompletionRequest, schemaName string, schema interface{}) (json.RawMessage, error) {
		return makeCELGenJSON("true", "ok", nil, nil), nil
	}

	assistant := NewCELAssistant(mock, "test-model")
	_, err := assistant.Generate(context.Background(), GenerateCELRequest{
		Description: "test",
		Examples: []CELExample{
			{Description: "elderly patients", Expression: "event.patient.age >= 65"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	call := mock.LastCall()
	userPrompt := call.Messages[1].Content
	if !strings.Contains(userPrompt, "Examples for Reference") {
		t.Error("user prompt should contain examples section")
	}
	if !strings.Contains(userPrompt, "elderly patients") {
		t.Error("user prompt should contain example description")
	}
	if !strings.Contains(userPrompt, "event.patient.age >= 65") {
		t.Error("user prompt should contain example expression")
	}
}

// --- CEL Explain tests ---

func TestCELExplain_Valid(t *testing.T) {
	mock := llm.NewMockClient().WithCompleteResponse("This checks if the patient is 65 or older.")
	assistant := NewCELAssistant(mock, "test-model")

	result, err := assistant.Explain(context.Background(), "event.patient.age >= 65")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "This checks if the patient is 65 or older." {
		t.Errorf("unexpected explanation: %q", result)
	}
}

func TestCELExplain_EmptyExpression(t *testing.T) {
	mock := llm.NewMockClient()
	assistant := NewCELAssistant(mock, "test-model")

	_, err := assistant.Explain(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty expression")
	}
	if err.Error() != "expression is required" {
		t.Errorf("expected 'expression is required', got %q", err.Error())
	}
	if mock.CallCount() != 0 {
		t.Error("LLM should not be called for empty expression")
	}
}

func TestCELExplain_LLMError(t *testing.T) {
	mock := llm.NewMockClient().WithError(fmt.Errorf("rate limited"))
	assistant := NewCELAssistant(mock, "test-model")

	_, err := assistant.Explain(context.Background(), "event.patient.age >= 65")
	if err == nil {
		t.Fatal("expected error from LLM")
	}
	if !strings.Contains(err.Error(), "explain cel:") {
		t.Errorf("expected error wrapped with 'explain cel:', got %q", err.Error())
	}
}

func TestCELExplain_RequestParameters(t *testing.T) {
	mock := llm.NewMockClient().WithCompleteResponse("explanation")
	assistant := NewCELAssistant(mock, "gpt-4o")

	_, err := assistant.Explain(context.Background(), `event.event_type == "ADT"`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	call := mock.LastCall()
	if call == nil {
		t.Fatal("expected at least one call")
	}
	if call.Temperature != 0.2 {
		t.Errorf("expected temperature 0.2, got %v", call.Temperature)
	}
	if call.MaxTokens != 512 {
		t.Errorf("expected max tokens 512, got %d", call.MaxTokens)
	}
	if len(call.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(call.Messages))
	}
	systemMsg := call.Messages[0].Content
	if !strings.Contains(systemMsg, "CEL") && !strings.Contains(systemMsg, "expert") {
		t.Error("system message should mention CEL expert role")
	}
	userMsg := call.Messages[1].Content
	if !strings.Contains(userMsg, `event.event_type == "ADT"`) {
		t.Error("user message should contain the expression")
	}
}

// --- ValidateAndCorrect tests ---

func TestValidateAndCorrect_EmptyExpression(t *testing.T) {
	mock := llm.NewMockClient()
	assistant := NewCELAssistant(mock, "test-model")

	result, err := assistant.ValidateAndCorrect(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Valid {
		t.Error("empty expression should not be valid")
	}
	if result.Error != "empty expression" {
		t.Errorf("expected error 'empty expression', got %q", result.Error)
	}
	if mock.CallCount() != 0 {
		t.Error("LLM should not be called for empty expression")
	}
}

func TestValidateAndCorrect_Valid(t *testing.T) {
	mock := llm.NewMockClient()
	mock.CompleteStructuredFunc = func(ctx context.Context, req llm.CompletionRequest, schemaName string, schema interface{}) (json.RawMessage, error) {
		return makeValidationJSON(true, "", "", nil), nil
	}

	assistant := NewCELAssistant(mock, "test-model")
	result, err := assistant.ValidateAndCorrect(context.Background(), `event.patient.age >= 65`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Valid {
		t.Error("expected valid result")
	}
	if result.Error != "" {
		t.Errorf("expected no error, got %q", result.Error)
	}
}

func TestValidateAndCorrect_InvalidWithCorrection(t *testing.T) {
	mock := llm.NewMockClient()
	mock.CompleteStructuredFunc = func(ctx context.Context, req llm.CompletionRequest, schemaName string, schema interface{}) (json.RawMessage, error) {
		return makeValidationJSON(
			false,
			"missing quotes around string",
			`event.event_type == "ADT"`,
			[]string{"Always quote string literals"},
		), nil
	}

	assistant := NewCELAssistant(mock, "test-model")
	result, err := assistant.ValidateAndCorrect(context.Background(), `event.event_type == ADT`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Valid {
		t.Error("expected invalid result")
	}
	if result.Error != "missing quotes around string" {
		t.Errorf("unexpected error: %q", result.Error)
	}
	if result.Corrected != `event.event_type == "ADT"` {
		t.Errorf("unexpected corrected expression: %q", result.Corrected)
	}
	if len(result.Suggestions) != 1 || result.Suggestions[0] != "Always quote string literals" {
		t.Errorf("unexpected suggestions: %v", result.Suggestions)
	}
}

func TestValidateAndCorrect_LLMError(t *testing.T) {
	mock := llm.NewMockClient().WithError(fmt.Errorf("internal error"))
	assistant := NewCELAssistant(mock, "test-model")

	_, err := assistant.ValidateAndCorrect(context.Background(), "event.patient.age >= 65")
	if err == nil {
		t.Fatal("expected error from LLM")
	}
	if !strings.Contains(err.Error(), "validate cel:") {
		t.Errorf("expected error wrapped with 'validate cel:', got %q", err.Error())
	}
}

func TestValidateAndCorrect_RequestParameters(t *testing.T) {
	mock := llm.NewMockClient()
	var capturedSchemaName string
	mock.CompleteStructuredFunc = func(ctx context.Context, req llm.CompletionRequest, schemaName string, schema interface{}) (json.RawMessage, error) {
		capturedSchemaName = schemaName
		return makeValidationJSON(true, "", "", nil), nil
	}

	assistant := NewCELAssistant(mock, "gpt-4o")
	_, err := assistant.ValidateAndCorrect(context.Background(), "event.patient.age >= 65")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	call := mock.LastCall()
	if call.Temperature != 0.1 {
		t.Errorf("expected temperature 0.1, got %v", call.Temperature)
	}
	if call.MaxTokens != 1024 {
		t.Errorf("expected max tokens 1024, got %d", call.MaxTokens)
	}
	if capturedSchemaName != "cel_validation" {
		t.Errorf("expected schema name 'cel_validation', got %q", capturedSchemaName)
	}
}

// --- Prompt builder tests ---

func TestBuildCELSystemPrompt(t *testing.T) {
	t.Run("nil schema - no custom fields", func(t *testing.T) {
		prompt := buildCELSystemPrompt(nil)
		if strings.Contains(prompt, "Custom Fields") {
			t.Error("prompt should not contain 'Custom Fields' when schema is nil")
		}
		// Should still contain CEL basics
		if !strings.Contains(prompt, "CEL Basics") {
			t.Error("prompt should contain 'CEL Basics' section")
		}
		if !strings.Contains(prompt, "Example Expressions") {
			t.Error("prompt should contain 'Example Expressions' section")
		}
	})

	t.Run("schema with fields", func(t *testing.T) {
		schema := &EventSchema{
			Fields: map[string]string{
				"event.custom.lab_type": "Type of lab test",
			},
		}
		prompt := buildCELSystemPrompt(schema)
		if !strings.Contains(prompt, "Custom Fields") {
			t.Error("prompt should contain 'Custom Fields' section when fields provided")
		}
		if !strings.Contains(prompt, "event.custom.lab_type") {
			t.Error("prompt should contain the custom field path")
		}
		if !strings.Contains(prompt, "Type of lab test") {
			t.Error("prompt should contain the custom field description")
		}
	})

	t.Run("schema with empty fields", func(t *testing.T) {
		schema := &EventSchema{
			Fields: map[string]string{},
		}
		prompt := buildCELSystemPrompt(schema)
		if strings.Contains(prompt, "Custom Fields") {
			t.Error("prompt should not contain 'Custom Fields' when fields map is empty")
		}
	})

	t.Run("always contains CEL basics", func(t *testing.T) {
		prompt := buildCELSystemPrompt(nil)
		if !strings.Contains(prompt, "dot notation") {
			t.Error("prompt should explain dot notation")
		}
		if !strings.Contains(prompt, "Boolean operators") {
			t.Error("prompt should explain boolean operators")
		}
	})

	t.Run("always contains example expressions", func(t *testing.T) {
		prompt := buildCELSystemPrompt(nil)
		if !strings.Contains(prompt, "event.patient.age >= 65") {
			t.Error("prompt should contain age example")
		}
		if !strings.Contains(prompt, `event.observation.interpretation == "critical"`) {
			t.Error("prompt should contain critical lab example")
		}
	})
}

func TestBuildCELUserPrompt(t *testing.T) {
	t.Run("contains description", func(t *testing.T) {
		prompt := buildCELUserPrompt(GenerateCELRequest{Description: "critical lab results"})
		if !strings.Contains(prompt, "critical lab results") {
			t.Error("user prompt should contain the description")
		}
	})

	t.Run("examples present", func(t *testing.T) {
		prompt := buildCELUserPrompt(GenerateCELRequest{
			Description: "test",
			Examples: []CELExample{
				{Description: "elderly", Expression: "event.patient.age >= 65"},
			},
		})
		if !strings.Contains(prompt, "Examples for Reference") {
			t.Error("user prompt should contain examples section")
		}
		if !strings.Contains(prompt, "elderly") {
			t.Error("user prompt should contain example description")
		}
		if !strings.Contains(prompt, "event.patient.age >= 65") {
			t.Error("user prompt should contain example expression")
		}
	})

	t.Run("examples absent", func(t *testing.T) {
		prompt := buildCELUserPrompt(GenerateCELRequest{Description: "test"})
		if strings.Contains(prompt, "Examples for Reference") {
			t.Error("user prompt should not contain examples section when no examples")
		}
	})
}
