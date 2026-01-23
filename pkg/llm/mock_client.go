package llm

import (
	"context"
	"encoding/json"
	"sync"
)

// MockClient is a test double for the Client interface.
// It records all calls and allows configuring responses.
type MockClient struct {
	// CompleteFunc is called when Complete is invoked.
	// If nil, returns a default response.
	CompleteFunc func(ctx context.Context, req CompletionRequest) (*CompletionResponse, error)

	// CompleteJSONFunc is called when CompleteJSON is invoked.
	// If nil, returns an empty JSON object.
	CompleteJSONFunc func(ctx context.Context, req CompletionRequest) (json.RawMessage, error)

	// CompleteStructuredFunc is called when CompleteStructured is invoked.
	// If nil, returns an empty JSON object.
	CompleteStructuredFunc func(ctx context.Context, req CompletionRequest, schemaName string, schema interface{}) (json.RawMessage, error)

	// QualityModelValue is returned by QualityModel().
	QualityModelValue string

	// Calls records all CompletionRequests made to this mock.
	Calls []CompletionRequest

	// mu protects Calls.
	mu sync.Mutex
}

// NewMockClient creates a new MockClient with default behavior.
func NewMockClient() *MockClient {
	return &MockClient{
		QualityModelValue: "mock-quality-model",
	}
}

// Complete implements Client.Complete.
func (m *MockClient) Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
	m.mu.Lock()
	m.Calls = append(m.Calls, req)
	m.mu.Unlock()

	if m.CompleteFunc != nil {
		return m.CompleteFunc(ctx, req)
	}

	return &CompletionResponse{
		ID:    "mock-completion-id",
		Model: req.Model,
		Choices: []Choice{
			{
				Index: 0,
				Message: Message{
					Role:    "assistant",
					Content: "mock response",
				},
				FinishReason: "stop",
			},
		},
		Usage: Usage{
			PromptTokens:     10,
			CompletionTokens: 5,
			TotalTokens:      15,
		},
	}, nil
}

// CompleteJSON implements Client.CompleteJSON.
func (m *MockClient) CompleteJSON(ctx context.Context, req CompletionRequest) (json.RawMessage, error) {
	m.mu.Lock()
	m.Calls = append(m.Calls, req)
	m.mu.Unlock()

	if m.CompleteJSONFunc != nil {
		return m.CompleteJSONFunc(ctx, req)
	}

	return json.RawMessage(`{}`), nil
}

// CompleteStructured implements Client.CompleteStructured.
func (m *MockClient) CompleteStructured(ctx context.Context, req CompletionRequest, schemaName string, schema interface{}) (json.RawMessage, error) {
	m.mu.Lock()
	m.Calls = append(m.Calls, req)
	m.mu.Unlock()

	if m.CompleteStructuredFunc != nil {
		return m.CompleteStructuredFunc(ctx, req, schemaName, schema)
	}

	return json.RawMessage(`{}`), nil
}

// QualityModel implements ClientWithQualityModel.QualityModel.
func (m *MockClient) QualityModel() string {
	return m.QualityModelValue
}

// GetCalls returns a copy of all recorded calls.
func (m *MockClient) GetCalls() []CompletionRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	calls := make([]CompletionRequest, len(m.Calls))
	copy(calls, m.Calls)
	return calls
}

// Reset clears all recorded calls.
func (m *MockClient) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Calls = nil
}

// LastCall returns the most recent call, or nil if no calls were made.
func (m *MockClient) LastCall() *CompletionRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.Calls) == 0 {
		return nil
	}
	call := m.Calls[len(m.Calls)-1]
	return &call
}

// CallCount returns the number of calls made.
func (m *MockClient) CallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.Calls)
}

// WithCompleteResponse configures the mock to return a specific response.
func (m *MockClient) WithCompleteResponse(content string) *MockClient {
	m.CompleteFunc = func(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
		return &CompletionResponse{
			ID:    "mock-completion-id",
			Model: req.Model,
			Choices: []Choice{
				{
					Index: 0,
					Message: Message{
						Role:    "assistant",
						Content: content,
					},
					FinishReason: "stop",
				},
			},
		}, nil
	}
	return m
}

// WithJSONResponse configures the mock to return a specific JSON response.
func (m *MockClient) WithJSONResponse(data interface{}) *MockClient {
	jsonBytes, _ := json.Marshal(data)
	m.CompleteJSONFunc = func(ctx context.Context, req CompletionRequest) (json.RawMessage, error) {
		return jsonBytes, nil
	}
	m.CompleteStructuredFunc = func(ctx context.Context, req CompletionRequest, schemaName string, schema interface{}) (json.RawMessage, error) {
		return jsonBytes, nil
	}
	return m
}

// WithError configures the mock to return an error.
func (m *MockClient) WithError(err error) *MockClient {
	m.CompleteFunc = func(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
		return nil, err
	}
	m.CompleteJSONFunc = func(ctx context.Context, req CompletionRequest) (json.RawMessage, error) {
		return nil, err
	}
	m.CompleteStructuredFunc = func(ctx context.Context, req CompletionRequest, schemaName string, schema interface{}) (json.RawMessage, error) {
		return nil, err
	}
	return m
}

// Ensure MockClient implements all interfaces.
var _ Client = (*MockClient)(nil)
var _ ClientWithQualityModel = (*MockClient)(nil)
