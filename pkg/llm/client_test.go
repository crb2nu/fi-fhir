//nolint:errcheck,errorlint,gosec // Test file - error checking omitted for brevity
package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClientNew(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		cfg := Config{
			BaseURL:      "http://localhost:8000/v1",
			DefaultModel: "test-model",
			Timeout:      30 * time.Second,
		}
		client, err := New(cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if client == nil {
			t.Fatal("client should not be nil")
		}
	})

	t.Run("invalid config - missing base URL", func(t *testing.T) {
		cfg := Config{
			DefaultModel: "test-model",
		}
		_, err := New(cfg)
		if err == nil {
			t.Fatal("expected error for missing base URL")
		}
	})

	t.Run("invalid config - missing model", func(t *testing.T) {
		cfg := Config{
			BaseURL: "http://localhost:8000/v1",
		}
		_, err := New(cfg)
		if err == nil {
			t.Fatal("expected error for missing model")
		}
	})
}

func TestClientNewWithDefaults(t *testing.T) {
	client, err := NewWithDefaults()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client == nil {
		t.Fatal("client should not be nil")
	}
}

func TestClientComplete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/chat/completions" {
			t.Errorf("expected /chat/completions, got %s", r.URL.Path)
		}

		// Check authorization header
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-key" {
			t.Errorf("expected 'Bearer test-key', got '%s'", auth)
		}

		// Parse request
		var req chatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request: %v", err)
		}

		// Verify request
		if req.Model != "test-model" {
			t.Errorf("expected model 'test-model', got '%s'", req.Model)
		}
		if len(req.Messages) != 2 {
			t.Errorf("expected 2 messages, got %d", len(req.Messages))
		}

		// Send response
		resp := map[string]interface{}{
			"id":    "test-id",
			"model": req.Model,
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"message": map[string]string{
						"role":    "assistant",
						"content": "test response",
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]int{
				"prompt_tokens":     10,
				"completion_tokens": 5,
				"total_tokens":      15,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := Config{
		BaseURL:      server.URL,
		APIKey:       "test-key",
		DefaultModel: "test-model",
		Timeout:      5 * time.Second,
	}
	client, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	ctx := context.Background()
	req := CompletionRequest{
		Messages: []Message{
			SystemMessage("You are a helpful assistant."),
			UserMessage("Hello!"),
		},
	}

	resp, err := client.Complete(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.ID != "test-id" {
		t.Errorf("ID = %v, want 'test-id'", resp.ID)
	}
	if resp.Content() != "test response" {
		t.Errorf("Content() = %v, want 'test response'", resp.Content())
	}
	if resp.Usage.TotalTokens != 15 {
		t.Errorf("TotalTokens = %v, want 15", resp.Usage.TotalTokens)
	}
}

func TestClientCompleteJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req chatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request: %v", err)
		}

		// Check response format
		if req.ResponseFormat == nil {
			t.Error("expected response_format to be set")
		}
		if req.ResponseFormat.Type != "json_object" {
			t.Errorf("expected response_format.type = 'json_object', got '%s'", req.ResponseFormat.Type)
		}

		resp := map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]string{
						"content": `{"name": "test", "value": 42}`,
					},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := Config{
		BaseURL:      server.URL,
		DefaultModel: "test-model",
		Timeout:      5 * time.Second,
	}
	client, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	ctx := context.Background()
	req := CompletionRequest{
		Messages: []Message{
			UserMessage("Return a JSON object"),
		},
	}

	rawJSON, err := client.CompleteJSON(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(rawJSON, &result); err != nil {
		t.Fatalf("failed to parse JSON response: %v", err)
	}

	if result["name"] != "test" {
		t.Errorf("name = %v, want 'test'", result["name"])
	}
}

func TestClientCompleteStructured(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req chatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request: %v", err)
		}

		// Check response format
		if req.ResponseFormat == nil {
			t.Error("expected response_format to be set")
		}
		if req.ResponseFormat.Type != "json_schema" {
			t.Errorf("expected response_format.type = 'json_schema', got '%s'", req.ResponseFormat.Type)
		}
		if req.ResponseFormat.JSONSchema == nil {
			t.Error("expected json_schema to be set")
		}
		if req.ResponseFormat.JSONSchema.Name != "test_schema" {
			t.Errorf("expected schema name 'test_schema', got '%s'", req.ResponseFormat.JSONSchema.Name)
		}

		resp := map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]string{
						"content": `{"result": "structured output"}`,
					},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := Config{
		BaseURL:      server.URL,
		DefaultModel: "test-model",
		Timeout:      5 * time.Second,
	}
	client, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	ctx := context.Background()
	req := CompletionRequest{
		Messages: []Message{
			UserMessage("Return structured output"),
		},
	}

	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"result": map[string]string{"type": "string"},
		},
	}

	rawJSON, err := client.CompleteStructured(ctx, req, "test_schema", schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(rawJSON, &result); err != nil {
		t.Fatalf("failed to parse JSON response: %v", err)
	}

	if result["result"] != "structured output" {
		t.Errorf("result = %v, want 'structured output'", result["result"])
	}
}

func TestClientErrorHandling(t *testing.T) {
	t.Run("API error 4xx", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error": map[string]string{
					"message": "Invalid request",
					"code":    "invalid_request",
				},
			})
		}))
		defer server.Close()

		cfg := Config{
			BaseURL:      server.URL,
			DefaultModel: "test-model",
			Timeout:      5 * time.Second,
		}
		client, _ := New(cfg)

		_, err := client.Complete(context.Background(), CompletionRequest{
			Messages: []Message{UserMessage("test")},
		})

		if err == nil {
			t.Fatal("expected error for 400 response")
		}

		apiErr := GetAPIError(err)
		if apiErr == nil {
			t.Fatal("expected APIError")
		}
		if apiErr.StatusCode != 400 {
			t.Errorf("StatusCode = %v, want 400", apiErr.StatusCode)
		}
		if !apiErr.IsBadRequest() {
			t.Error("expected IsBadRequest() to return true")
		}
	})

	t.Run("API error 5xx (retryable)", func(t *testing.T) {
		attempts := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			attempts++
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("Internal Server Error"))
		}))
		defer server.Close()

		cfg := Config{
			BaseURL:        server.URL,
			DefaultModel:   "test-model",
			Timeout:        5 * time.Second,
			MaxRetries:     2,
			RetryBaseDelay: 10 * time.Millisecond,
			RetryMaxDelay:  50 * time.Millisecond,
		}
		client, _ := New(cfg)

		_, err := client.Complete(context.Background(), CompletionRequest{
			Messages: []Message{UserMessage("test")},
		})

		if err == nil {
			t.Fatal("expected error for 500 response")
		}

		// Should have retried
		if attempts < 2 {
			t.Errorf("attempts = %v, want >= 2 (retries)", attempts)
		}
	})

	t.Run("no choices in response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := map[string]interface{}{
				"choices": []map[string]interface{}{},
			}
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		cfg := Config{
			BaseURL:      server.URL,
			DefaultModel: "test-model",
			Timeout:      5 * time.Second,
		}
		client, _ := New(cfg)

		_, err := client.CompleteJSON(context.Background(), CompletionRequest{
			Messages: []Message{UserMessage("test")},
		})

		if err != ErrNoChoices {
			t.Errorf("error = %v, want ErrNoChoices", err)
		}
	})
}

func TestClientQualityModel(t *testing.T) {
	cfg := Config{
		BaseURL:      "http://localhost:8000/v1",
		DefaultModel: "fast-model",
		QualityModel: "quality-model",
		Timeout:      5 * time.Second,
	}
	client, _ := New(cfg)

	qClient, ok := client.(ClientWithQualityModel)
	if !ok {
		t.Fatal("client should implement ClientWithQualityModel")
	}

	if qClient.QualityModel() != "quality-model" {
		t.Errorf("QualityModel() = %v, want 'quality-model'", qClient.QualityModel())
	}
}

func TestCompletionRequest_WithMethods(t *testing.T) {
	req := CompletionRequest{
		Messages: []Message{UserMessage("test")},
	}

	req = req.WithModel("custom-model")
	if req.Model != "custom-model" {
		t.Errorf("Model = %v, want 'custom-model'", req.Model)
	}

	req = req.WithTemperature(0.8)
	if req.Temperature != 0.8 {
		t.Errorf("Temperature = %v, want 0.8", req.Temperature)
	}

	req = req.WithMaxTokens(1000)
	if req.MaxTokens != 1000 {
		t.Errorf("MaxTokens = %v, want 1000", req.MaxTokens)
	}
}

func TestCompletionResponse_Content(t *testing.T) {
	t.Run("with choices", func(t *testing.T) {
		resp := &CompletionResponse{
			Choices: []Choice{
				{Message: Message{Content: "hello"}},
			},
		}
		if resp.Content() != "hello" {
			t.Errorf("Content() = %v, want 'hello'", resp.Content())
		}
	})

	t.Run("without choices", func(t *testing.T) {
		resp := &CompletionResponse{
			Choices: []Choice{},
		}
		if resp.Content() != "" {
			t.Errorf("Content() = %v, want empty string", resp.Content())
		}
	})
}

func TestMessageHelpers(t *testing.T) {
	sys := SystemMessage("system prompt")
	if sys.Role != "system" || sys.Content != "system prompt" {
		t.Errorf("SystemMessage() = %+v, unexpected", sys)
	}

	user := UserMessage("user prompt")
	if user.Role != "user" || user.Content != "user prompt" {
		t.Errorf("UserMessage() = %+v, unexpected", user)
	}

	assistant := AssistantMessage("assistant response")
	if assistant.Role != "assistant" || assistant.Content != "assistant response" {
		t.Errorf("AssistantMessage() = %+v, unexpected", assistant)
	}
}

func TestNewCompletionRequest(t *testing.T) {
	req := NewCompletionRequest("system", "user")

	if len(req.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(req.Messages))
	}
	if req.Messages[0].Role != "system" {
		t.Errorf("first message role = %v, want 'system'", req.Messages[0].Role)
	}
	if req.Messages[1].Role != "user" {
		t.Errorf("second message role = %v, want 'user'", req.Messages[1].Role)
	}
	if req.Temperature != 0.2 {
		t.Errorf("Temperature = %v, want 0.2 (default)", req.Temperature)
	}
}

func TestNewConversation(t *testing.T) {
	req := NewConversation("system",
		UserMessage("question"),
		AssistantMessage("answer"),
		UserMessage("follow-up"),
	)

	if len(req.Messages) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(req.Messages))
	}
	if req.Messages[0].Role != "system" {
		t.Errorf("first message should be system")
	}
	if req.Messages[3].Content != "follow-up" {
		t.Errorf("last message content = %v, want 'follow-up'", req.Messages[3].Content)
	}
}

func TestMockClient(t *testing.T) {
	mock := NewMockClient()

	t.Run("default response", func(t *testing.T) {
		resp, err := mock.Complete(context.Background(), CompletionRequest{
			Messages: []Message{UserMessage("test")},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.Content() != "mock response" {
			t.Errorf("Content() = %v, want 'mock response'", resp.Content())
		}
	})

	t.Run("custom response", func(t *testing.T) {
		mock = NewMockClient()
		mock.WithCompleteResponse("custom response")

		resp, err := mock.Complete(context.Background(), CompletionRequest{
			Messages: []Message{UserMessage("test")},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.Content() != "custom response" {
			t.Errorf("Content() = %v, want 'custom response'", resp.Content())
		}
	})

	t.Run("records calls", func(t *testing.T) {
		mock = NewMockClient()
		_, _ = mock.Complete(context.Background(), CompletionRequest{
			Messages: []Message{UserMessage("first")},
		})
		_, _ = mock.Complete(context.Background(), CompletionRequest{
			Messages: []Message{UserMessage("second")},
		})

		if mock.CallCount() != 2 {
			t.Errorf("CallCount() = %v, want 2", mock.CallCount())
		}

		calls := mock.GetCalls()
		if len(calls) != 2 {
			t.Errorf("len(GetCalls()) = %v, want 2", len(calls))
		}

		last := mock.LastCall()
		if last == nil || last.Messages[0].Content != "second" {
			t.Error("LastCall() should return the second request")
		}
	})

	t.Run("with error", func(t *testing.T) {
		mock = NewMockClient()
		testErr := &APIError{StatusCode: 500, Body: "test error"}
		mock.WithError(testErr)

		_, err := mock.Complete(context.Background(), CompletionRequest{})
		if err != testErr {
			t.Errorf("error = %v, want %v", err, testErr)
		}
	})

	t.Run("with JSON response", func(t *testing.T) {
		mock = NewMockClient()
		mock.WithJSONResponse(map[string]string{"key": "value"})

		rawJSON, err := mock.CompleteJSON(context.Background(), CompletionRequest{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var result map[string]string
		json.Unmarshal(rawJSON, &result)
		if result["key"] != "value" {
			t.Errorf("key = %v, want 'value'", result["key"])
		}
	})

	t.Run("quality model", func(t *testing.T) {
		mock = NewMockClient()
		mock.QualityModelValue = "quality-model"

		if mock.QualityModel() != "quality-model" {
			t.Errorf("QualityModel() = %v, want 'quality-model'", mock.QualityModel())
		}
	})

	t.Run("reset", func(t *testing.T) {
		mock = NewMockClient()
		mock.Complete(context.Background(), CompletionRequest{})
		mock.Complete(context.Background(), CompletionRequest{})
		mock.Reset()

		if mock.CallCount() != 0 {
			t.Errorf("CallCount() after Reset() = %v, want 0", mock.CallCount())
		}
	})
}

func TestWaitForReady(t *testing.T) {
	t.Run("immediate success", func(t *testing.T) {
		mock := NewMockClient()
		err := WaitForReady(context.Background(), mock, 1*time.Second)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("context canceled", func(t *testing.T) {
		mock := NewMockClient()
		mock.WithError(&APIError{StatusCode: 500, Body: "not ready"})

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		err := WaitForReady(ctx, mock, 10*time.Second)
		if err != context.Canceled {
			t.Errorf("error = %v, want context.Canceled", err)
		}
	})
}
