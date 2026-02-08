package copilot

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/llm"
)

// makeWorkflowJSON builds a valid workflow generation JSON response.
func makeWorkflowJSON(yaml, explanation string, warnings []string) json.RawMessage {
	resp := map[string]interface{}{
		"yaml":        yaml,
		"explanation": explanation,
	}
	if warnings != nil {
		resp["warnings"] = warnings
	}
	b, _ := json.Marshal(resp)
	return b
}

// makeInteractiveJSON builds a valid interactive response JSON.
func makeInteractiveJSON(message, workflowYAML string, questions []string, isComplete bool) json.RawMessage {
	resp := map[string]interface{}{
		"message":     message,
		"is_complete": isComplete,
	}
	if workflowYAML != "" {
		resp["workflow_yaml"] = workflowYAML
	}
	if questions != nil {
		resp["questions"] = questions
	}
	b, _ := json.Marshal(resp)
	return b
}

// --- WorkflowCopilot tests ---

func TestNewWorkflowCopilot(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		mock := llm.NewMockClient()
		copilot, err := NewWorkflowCopilot(CopilotConfig{Client: mock, Model: "test-model"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if copilot == nil {
			t.Fatal("copilot should not be nil")
		}
	})

	t.Run("nil client returns error", func(t *testing.T) {
		_, err := NewWorkflowCopilot(CopilotConfig{})
		if err == nil {
			t.Fatal("expected error for nil client")
		}
		if err.Error() != "client is required" {
			t.Errorf("expected 'client is required', got %q", err.Error())
		}
	})
}

func TestGenerate_Valid(t *testing.T) {
	mock := llm.NewMockClient()
	yamlContent := "name: test-workflow\nversion: 1.0.0"
	mock.CompleteStructuredFunc = func(ctx context.Context, req llm.CompletionRequest, schemaName string, schema interface{}) (json.RawMessage, error) {
		return makeWorkflowJSON(yamlContent, "Routes ADT events", []string{"no error handling"}), nil
	}

	copilot, err := NewWorkflowCopilot(CopilotConfig{Client: mock, Model: "test-model"})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	result, err := copilot.Generate(context.Background(), GenerateRequest{
		Description: "Route ADT events to webhook",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.YAML != yamlContent {
		t.Errorf("expected YAML %q, got %q", yamlContent, result.YAML)
	}
	if result.Explanation != "Routes ADT events" {
		t.Errorf("expected explanation 'Routes ADT events', got %q", result.Explanation)
	}
	if len(result.Warnings) != 1 || result.Warnings[0] != "no error handling" {
		t.Errorf("expected 1 warning, got %v", result.Warnings)
	}
}

func TestGenerate_EmptyDescription(t *testing.T) {
	mock := llm.NewMockClient()
	copilot, err := NewWorkflowCopilot(CopilotConfig{Client: mock})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	_, err = copilot.Generate(context.Background(), GenerateRequest{Description: ""})
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

func TestGenerate_LLMError(t *testing.T) {
	mock := llm.NewMockClient().WithError(fmt.Errorf("connection refused"))
	copilot, err := NewWorkflowCopilot(CopilotConfig{Client: mock})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	_, err = copilot.Generate(context.Background(), GenerateRequest{Description: "test"})
	if err == nil {
		t.Fatal("expected error from LLM")
	}
	if !strings.Contains(err.Error(), "generate workflow:") {
		t.Errorf("expected wrapped error with 'generate workflow:', got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "connection refused") {
		t.Errorf("expected original error preserved, got %q", err.Error())
	}
}

func TestGenerate_MalformedJSON(t *testing.T) {
	mock := llm.NewMockClient()
	mock.CompleteStructuredFunc = func(ctx context.Context, req llm.CompletionRequest, schemaName string, schema interface{}) (json.RawMessage, error) {
		return json.RawMessage(`{not valid json`), nil
	}

	copilot, err := NewWorkflowCopilot(CopilotConfig{Client: mock})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	_, err = copilot.Generate(context.Background(), GenerateRequest{Description: "test"})
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
	if !strings.Contains(err.Error(), "parse response:") {
		t.Errorf("expected 'parse response:' error, got %q", err.Error())
	}
}

func TestGenerate_RequestParameters(t *testing.T) {
	mock := llm.NewMockClient()
	var capturedSchemaName string
	mock.CompleteStructuredFunc = func(ctx context.Context, req llm.CompletionRequest, schemaName string, schema interface{}) (json.RawMessage, error) {
		capturedSchemaName = schemaName
		return makeWorkflowJSON("yaml: true", "ok", nil), nil
	}

	copilot, err := NewWorkflowCopilot(CopilotConfig{Client: mock, Model: "gpt-4o"})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	_, err = copilot.Generate(context.Background(), GenerateRequest{Description: "test workflow"})
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
	if call.MaxTokens != 2048 {
		t.Errorf("expected max tokens 2048, got %d", call.MaxTokens)
	}
	if call.Model != "gpt-4o" {
		t.Errorf("expected model 'gpt-4o', got %q", call.Model)
	}
	if len(call.Messages) != 2 {
		t.Fatalf("expected 2 messages (system+user), got %d", len(call.Messages))
	}
	if call.Messages[0].Role != "system" {
		t.Errorf("first message should be system, got %q", call.Messages[0].Role)
	}
	if call.Messages[1].Role != "user" {
		t.Errorf("second message should be user, got %q", call.Messages[1].Role)
	}
	if capturedSchemaName != "workflow_generation" {
		t.Errorf("expected schema name 'workflow_generation', got %q", capturedSchemaName)
	}
}

func TestGenerate_WithEventAndActionTypes(t *testing.T) {
	mock := llm.NewMockClient()
	mock.CompleteStructuredFunc = func(ctx context.Context, req llm.CompletionRequest, schemaName string, schema interface{}) (json.RawMessage, error) {
		return makeWorkflowJSON("yaml: true", "ok", nil), nil
	}

	copilot, err := NewWorkflowCopilot(CopilotConfig{Client: mock})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	_, err = copilot.Generate(context.Background(), GenerateRequest{
		Description: "test",
		EventTypes:  []string{"CUSTOM_EVENT", "SPECIAL_ADT"},
		ActionTypes: []string{"custom_action", "special_webhook"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	call := mock.LastCall()
	systemPrompt := call.Messages[0].Content
	if !strings.Contains(systemPrompt, "CUSTOM_EVENT") {
		t.Error("system prompt should contain custom event type 'CUSTOM_EVENT'")
	}
	if !strings.Contains(systemPrompt, "SPECIAL_ADT") {
		t.Error("system prompt should contain custom event type 'SPECIAL_ADT'")
	}
	if !strings.Contains(systemPrompt, "custom_action") {
		t.Error("system prompt should contain custom action type 'custom_action'")
	}
	if !strings.Contains(systemPrompt, "special_webhook") {
		t.Error("system prompt should contain custom action type 'special_webhook'")
	}
	// Default types should NOT appear when custom types are provided
	if strings.Contains(systemPrompt, "ADT (Admit/Discharge/Transfer)") {
		t.Error("system prompt should not contain default ADT event type when custom types are provided")
	}
}

func TestGenerate_WithExamples(t *testing.T) {
	mock := llm.NewMockClient()
	mock.CompleteStructuredFunc = func(ctx context.Context, req llm.CompletionRequest, schemaName string, schema interface{}) (json.RawMessage, error) {
		return makeWorkflowJSON("yaml: true", "ok", nil), nil
	}

	copilot, err := NewWorkflowCopilot(CopilotConfig{Client: mock})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	_, err = copilot.Generate(context.Background(), GenerateRequest{
		Description: "test",
		Examples: []WorkflowExample{
			{Description: "simple routing", YAML: "name: simple\nversion: 1.0.0"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	call := mock.LastCall()
	userPrompt := call.Messages[1].Content
	if !strings.Contains(userPrompt, "Examples for Reference") {
		t.Error("user prompt should contain examples section header")
	}
	if !strings.Contains(userPrompt, "simple routing") {
		t.Error("user prompt should contain example description")
	}
	if !strings.Contains(userPrompt, "name: simple") {
		t.Error("user prompt should contain example YAML")
	}
}

func TestGenerate_WarningsEdgeCases(t *testing.T) {
	t.Run("empty warnings array", func(t *testing.T) {
		mock := llm.NewMockClient()
		mock.CompleteStructuredFunc = func(ctx context.Context, req llm.CompletionRequest, schemaName string, schema interface{}) (json.RawMessage, error) {
			return makeWorkflowJSON("yaml: true", "ok", []string{}), nil
		}

		copilot, err := NewWorkflowCopilot(CopilotConfig{Client: mock})
		if err != nil {
			t.Fatalf("setup: %v", err)
		}

		result, err := copilot.Generate(context.Background(), GenerateRequest{Description: "test"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Warnings == nil {
			t.Error("expected non-nil empty slice for explicit empty array")
		}
		if len(result.Warnings) != 0 {
			t.Errorf("expected 0 warnings, got %d", len(result.Warnings))
		}
	})

	t.Run("omitted warnings key", func(t *testing.T) {
		mock := llm.NewMockClient()
		mock.CompleteStructuredFunc = func(ctx context.Context, req llm.CompletionRequest, schemaName string, schema interface{}) (json.RawMessage, error) {
			return makeWorkflowJSON("yaml: true", "ok", nil), nil
		}

		copilot, err := NewWorkflowCopilot(CopilotConfig{Client: mock})
		if err != nil {
			t.Fatalf("setup: %v", err)
		}

		result, err := copilot.Generate(context.Background(), GenerateRequest{Description: "test"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Warnings != nil {
			t.Errorf("expected nil warnings when key is omitted, got %v", result.Warnings)
		}
	})
}

// --- Prompt builder tests ---

func TestBuildWorkflowSystemPrompt(t *testing.T) {
	t.Run("default event types", func(t *testing.T) {
		prompt := buildWorkflowSystemPrompt(nil, nil)
		if !strings.Contains(prompt, "ADT (Admit/Discharge/Transfer)") {
			t.Error("default prompt should contain ADT event type")
		}
		if !strings.Contains(prompt, "ORU (Observation Result - Lab)") {
			t.Error("default prompt should contain ORU event type")
		}
	})

	t.Run("custom event types override defaults", func(t *testing.T) {
		prompt := buildWorkflowSystemPrompt([]string{"CUSTOM_A", "CUSTOM_B"}, nil)
		if !strings.Contains(prompt, "CUSTOM_A") {
			t.Error("should contain custom event type A")
		}
		if !strings.Contains(prompt, "CUSTOM_B") {
			t.Error("should contain custom event type B")
		}
		if strings.Contains(prompt, "ADT (Admit/Discharge/Transfer)") {
			t.Error("should not contain default ADT when custom types provided")
		}
	})

	t.Run("default action types", func(t *testing.T) {
		prompt := buildWorkflowSystemPrompt(nil, nil)
		if !strings.Contains(prompt, "webhook: Send HTTP POST to URL") {
			t.Error("default prompt should contain webhook action type")
		}
		if !strings.Contains(prompt, "fhir: Convert to FHIR and send") {
			t.Error("default prompt should contain fhir action type")
		}
	})

	t.Run("custom action types override defaults", func(t *testing.T) {
		prompt := buildWorkflowSystemPrompt(nil, []string{"custom_action"})
		if !strings.Contains(prompt, "custom_action") {
			t.Error("should contain custom action type")
		}
		if strings.Contains(prompt, "webhook: Send HTTP POST to URL") {
			t.Error("should not contain default webhook when custom types provided")
		}
	})

	t.Run("CEL examples present", func(t *testing.T) {
		prompt := buildWorkflowSystemPrompt(nil, nil)
		if !strings.Contains(prompt, "CEL Expression Examples") {
			t.Error("prompt should contain CEL expression examples")
		}
		if !strings.Contains(prompt, `event.event_type == "ADT"`) {
			t.Error("prompt should contain ADT CEL example")
		}
	})
}

func TestBuildWorkflowUserPrompt(t *testing.T) {
	t.Run("contains description", func(t *testing.T) {
		prompt := buildWorkflowUserPrompt(GenerateRequest{Description: "Route ADT events"})
		if !strings.Contains(prompt, "Route ADT events") {
			t.Error("user prompt should contain the description")
		}
	})

	t.Run("examples section present", func(t *testing.T) {
		prompt := buildWorkflowUserPrompt(GenerateRequest{
			Description: "test",
			Examples:    []WorkflowExample{{Description: "ex1", YAML: "yaml: true"}},
		})
		if !strings.Contains(prompt, "Examples for Reference") {
			t.Error("user prompt should contain examples section when examples provided")
		}
	})

	t.Run("examples section absent when no examples", func(t *testing.T) {
		prompt := buildWorkflowUserPrompt(GenerateRequest{Description: "test"})
		if strings.Contains(prompt, "Examples for Reference") {
			t.Error("user prompt should not contain examples section when no examples")
		}
	})

	t.Run("ends with generate instruction", func(t *testing.T) {
		prompt := buildWorkflowUserPrompt(GenerateRequest{Description: "test"})
		if !strings.Contains(prompt, "Generate valid YAML with explanation.") {
			t.Error("user prompt should end with generate instruction")
		}
	})
}

// --- InteractiveSession tests ---

func TestNewInteractiveSession(t *testing.T) {
	mock := llm.NewMockClient()
	copilot, err := NewWorkflowCopilot(CopilotConfig{Client: mock})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	session := copilot.NewInteractiveSession()
	if session == nil {
		t.Fatal("session should not be nil")
	}
	if session.GetCurrentYAML() != "" {
		t.Errorf("initial YAML should be empty, got %q", session.GetCurrentYAML())
	}
	if len(session.conversation) != 1 {
		t.Fatalf("expected 1 initial message (system), got %d", len(session.conversation))
	}
	if session.conversation[0].Role != "system" {
		t.Errorf("first message should be system, got %q", session.conversation[0].Role)
	}
}

func TestChat_BasicResponse(t *testing.T) {
	mock := llm.NewMockClient()
	mock.CompleteStructuredFunc = func(ctx context.Context, req llm.CompletionRequest, schemaName string, schema interface{}) (json.RawMessage, error) {
		return makeInteractiveJSON("I can help with that!", "", nil, false), nil
	}

	copilot, err := NewWorkflowCopilot(CopilotConfig{Client: mock})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	session := copilot.NewInteractiveSession()
	resp, err := session.Chat(context.Background(), "I need a workflow")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Message != "I can help with that!" {
		t.Errorf("expected message 'I can help with that!', got %q", resp.Message)
	}
	if session.GetCurrentYAML() != "" {
		t.Error("YAML should remain empty when response has no workflow_yaml")
	}
}

func TestChat_WithWorkflowYAML(t *testing.T) {
	yamlContent := "name: my-workflow\nversion: 1.0.0"
	mock := llm.NewMockClient()
	mock.CompleteStructuredFunc = func(ctx context.Context, req llm.CompletionRequest, schemaName string, schema interface{}) (json.RawMessage, error) {
		return makeInteractiveJSON("Here's the workflow", yamlContent, nil, false), nil
	}

	copilot, err := NewWorkflowCopilot(CopilotConfig{Client: mock})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	session := copilot.NewInteractiveSession()
	_, err = session.Chat(context.Background(), "generate a workflow")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if session.GetCurrentYAML() != yamlContent {
		t.Errorf("expected YAML %q, got %q", yamlContent, session.GetCurrentYAML())
	}
}

func TestChat_EmptyYAMLPreservesPrevious(t *testing.T) {
	firstYAML := "name: first\nversion: 1.0.0"
	callCount := 0
	mock := llm.NewMockClient()
	mock.CompleteStructuredFunc = func(ctx context.Context, req llm.CompletionRequest, schemaName string, schema interface{}) (json.RawMessage, error) {
		callCount++
		if callCount == 1 {
			return makeInteractiveJSON("Here's v1", firstYAML, nil, false), nil
		}
		// Second call returns empty YAML
		return makeInteractiveJSON("No changes needed", "", nil, false), nil
	}

	copilot, err := NewWorkflowCopilot(CopilotConfig{Client: mock})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	session := copilot.NewInteractiveSession()

	// First chat sets YAML
	_, err = session.Chat(context.Background(), "generate a workflow")
	if err != nil {
		t.Fatalf("chat 1: %v", err)
	}
	if session.GetCurrentYAML() != firstYAML {
		t.Fatalf("after chat 1: expected YAML %q, got %q", firstYAML, session.GetCurrentYAML())
	}

	// Second chat returns empty YAML — previous should be preserved
	_, err = session.Chat(context.Background(), "looks good")
	if err != nil {
		t.Fatalf("chat 2: %v", err)
	}
	if session.GetCurrentYAML() != firstYAML {
		t.Errorf("after chat 2: expected previous YAML preserved %q, got %q", firstYAML, session.GetCurrentYAML())
	}
}

func TestChat_ConversationGrowth(t *testing.T) {
	mock := llm.NewMockClient()
	mock.CompleteStructuredFunc = func(ctx context.Context, req llm.CompletionRequest, schemaName string, schema interface{}) (json.RawMessage, error) {
		return makeInteractiveJSON("response", "", nil, false), nil
	}

	copilot, err := NewWorkflowCopilot(CopilotConfig{Client: mock})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	session := copilot.NewInteractiveSession()

	// Start with 1 system message
	if len(session.conversation) != 1 {
		t.Fatalf("expected 1 initial message, got %d", len(session.conversation))
	}

	// After N chats, conversation should be 1 + 2*N (user + assistant per chat)
	numChats := 3
	for i := 0; i < numChats; i++ {
		_, err := session.Chat(context.Background(), fmt.Sprintf("message %d", i))
		if err != nil {
			t.Fatalf("chat %d: %v", i, err)
		}
	}

	expected := 1 + 2*numChats
	if len(session.conversation) != expected {
		t.Errorf("expected %d messages after %d chats, got %d", expected, numChats, len(session.conversation))
	}

	// Verify last call sent full conversation history
	call := mock.LastCall()
	if call == nil {
		t.Fatal("expected at least one call")
	}
	// The request should have 1 (system) + 2*(numChats-1) (prior exchanges) + 1 (last user message)
	// = 1 + 2*2 + 1 = 6 for the 3rd call
	expectedMsgs := 1 + 2*(numChats-1) + 1
	if len(call.Messages) != expectedMsgs {
		t.Errorf("last call should have %d messages (full history before assistant response), got %d", expectedMsgs, len(call.Messages))
	}
}

func TestChat_LLMError(t *testing.T) {
	mock := llm.NewMockClient().WithError(fmt.Errorf("timeout"))
	copilot, err := NewWorkflowCopilot(CopilotConfig{Client: mock})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	session := copilot.NewInteractiveSession()
	_, err = session.Chat(context.Background(), "hello")
	if err == nil {
		t.Fatal("expected error from LLM")
	}
	if !strings.Contains(err.Error(), "chat:") {
		t.Errorf("expected error wrapped with 'chat:', got %q", err.Error())
	}
}

func TestChat_RequestParameters(t *testing.T) {
	mock := llm.NewMockClient()
	var capturedSchemaName string
	mock.CompleteStructuredFunc = func(ctx context.Context, req llm.CompletionRequest, schemaName string, schema interface{}) (json.RawMessage, error) {
		capturedSchemaName = schemaName
		return makeInteractiveJSON("ok", "", nil, false), nil
	}

	copilot, err := NewWorkflowCopilot(CopilotConfig{Client: mock, Model: "gpt-4o"})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	session := copilot.NewInteractiveSession()
	_, err = session.Chat(context.Background(), "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	call := mock.LastCall()
	if call.Temperature != 0.3 {
		t.Errorf("expected temperature 0.3, got %v", call.Temperature)
	}
	if call.MaxTokens != 2048 {
		t.Errorf("expected max tokens 2048, got %d", call.MaxTokens)
	}
	if capturedSchemaName != "interactive_response" {
		t.Errorf("expected schema name 'interactive_response', got %q", capturedSchemaName)
	}
}
