package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client provides an interface for LLM completions.
type Client interface {
	// Complete generates a completion for the given request.
	Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error)

	// CompleteJSON generates a completion with JSON output format.
	// Returns the raw JSON bytes from the model response.
	CompleteJSON(ctx context.Context, req CompletionRequest) (json.RawMessage, error)

	// CompleteStructured generates a completion with JSON schema enforcement.
	// The schema parameter should be a valid JSON Schema object.
	CompleteStructured(ctx context.Context, req CompletionRequest, schemaName string, schema interface{}) (json.RawMessage, error)
}

// CompletionRequest represents a completion request to the LLM.
type CompletionRequest struct {
	// Messages is the conversation history.
	Messages []Message `json:"messages"`

	// Model overrides the default model. If empty, uses client's default.
	Model string `json:"model,omitempty"`

	// Temperature controls randomness. 0.0 = deterministic, 1.0 = creative.
	Temperature float64 `json:"temperature,omitempty"`

	// MaxTokens limits the response length.
	MaxTokens int `json:"max_tokens,omitempty"`

	// TopP controls nucleus sampling.
	TopP float64 `json:"top_p,omitempty"`

	// Stop sequences to stop generation.
	Stop []string `json:"stop,omitempty"`
}

// Message represents a chat message in the conversation.
type Message struct {
	Role    string `json:"role"`    // "system", "user", or "assistant"
	Content string `json:"content"` // The message content
}

// CompletionResponse represents the response from a completion request.
type CompletionResponse struct {
	// ID is the unique identifier for this completion.
	ID string `json:"id"`

	// Model is the model used for generation.
	Model string `json:"model"`

	// Choices contains the generated completions.
	Choices []Choice `json:"choices"`

	// Usage contains token usage statistics.
	Usage Usage `json:"usage"`

	// Created is the Unix timestamp of when the completion was created.
	Created int64 `json:"created"`
}

// Choice represents a single completion choice.
type Choice struct {
	Index        int     `json:"index"`
	Message      Message `json:"message"`
	FinishReason string  `json:"finish_reason"`
}

// Usage contains token usage statistics.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Content returns the content of the first choice, or empty string if none.
func (r *CompletionResponse) Content() string {
	if len(r.Choices) == 0 {
		return ""
	}
	return r.Choices[0].Message.Content
}

// openAIClient implements Client using the OpenAI-compatible API.
type openAIClient struct {
	config     Config
	httpClient *http.Client
	retryer    *Retryer
}

// New creates a new LLM client with the given configuration.
// The configuration is automatically augmented with environment variable overrides.
func New(cfg Config) (Client, error) {
	cfg = cfg.WithEnv()
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &openAIClient{
		config: cfg,
		httpClient: &http.Client{
			Timeout: cfg.Timeout,
		},
		retryer: NewRetryer(RetryConfig{
			MaxRetries: cfg.MaxRetries,
			BaseDelay:  cfg.RetryBaseDelay,
			MaxDelay:   cfg.RetryMaxDelay,
		}),
	}, nil
}

// NewWithDefaults creates a new LLM client with default configuration.
func NewWithDefaults() (Client, error) {
	return New(DefaultConfig())
}

// Complete generates a completion for the given request.
func (c *openAIClient) Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
	return c.complete(ctx, req, nil)
}

// CompleteJSON generates a completion with JSON output format.
func (c *openAIClient) CompleteJSON(ctx context.Context, req CompletionRequest) (json.RawMessage, error) {
	resp, err := c.complete(ctx, req, &responseFormat{Type: "json_object"})
	if err != nil {
		return nil, err
	}
	if len(resp.Choices) == 0 {
		return nil, ErrNoChoices
	}
	return json.RawMessage(resp.Choices[0].Message.Content), nil
}

// CompleteStructured generates a completion with JSON schema enforcement.
func (c *openAIClient) CompleteStructured(ctx context.Context, req CompletionRequest, schemaName string, schema interface{}) (json.RawMessage, error) {
	format := &responseFormat{
		Type: "json_schema",
		JSONSchema: &jsonSchemaFormat{
			Name:   schemaName,
			Schema: schema,
			Strict: true,
		},
	}

	resp, err := c.complete(ctx, req, format)
	if err != nil {
		return nil, err
	}
	if len(resp.Choices) == 0 {
		return nil, ErrNoChoices
	}
	return json.RawMessage(resp.Choices[0].Message.Content), nil
}

// Internal types for OpenAI API

type chatRequest struct {
	Model          string          `json:"model"`
	Messages       []Message       `json:"messages"`
	Temperature    float64         `json:"temperature,omitempty"`
	MaxTokens      int             `json:"max_tokens,omitempty"`
	TopP           float64         `json:"top_p,omitempty"`
	Stop           []string        `json:"stop,omitempty"`
	ResponseFormat *responseFormat `json:"response_format,omitempty"`
}

type responseFormat struct {
	Type       string            `json:"type"`
	JSONSchema *jsonSchemaFormat `json:"json_schema,omitempty"`
}

type jsonSchemaFormat struct {
	Name   string      `json:"name"`
	Schema interface{} `json:"schema"`
	Strict bool        `json:"strict"`
}

type errorResponse struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error"`
}

// complete performs the actual API call with optional response format.
func (c *openAIClient) complete(ctx context.Context, req CompletionRequest, format *responseFormat) (*CompletionResponse, error) {
	// Use default model if not specified
	model := req.Model
	if model == "" {
		model = c.config.DefaultModel
	}

	// Build the API request
	apiReq := chatRequest{
		Model:          model,
		Messages:       req.Messages,
		Temperature:    req.Temperature,
		MaxTokens:      req.MaxTokens,
		TopP:           req.TopP,
		Stop:           req.Stop,
		ResponseFormat: format,
	}

	// Marshal request body
	body, err := json.Marshal(apiReq)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	// Build endpoint URL
	endpoint := strings.TrimSuffix(c.config.BaseURL, "/") + "/chat/completions"

	// Execute with retry
	var resp *CompletionResponse
	err = c.retryer.Do(ctx, func() error {
		httpReq, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("create request: %w", err)
		}

		httpReq.Header.Set("Content-Type", "application/json")
		if c.config.APIKey != "" {
			httpReq.Header.Set("Authorization", "Bearer "+c.config.APIKey)
		}

		httpResp, err := c.httpClient.Do(httpReq)
		if err != nil {
			return &RetryableError{Err: fmt.Errorf("send request: %w", err)}
		}
		defer httpResp.Body.Close()

		respBody, err := io.ReadAll(httpResp.Body)
		if err != nil {
			return fmt.Errorf("read response: %w", err)
		}

		// Handle error responses
		if httpResp.StatusCode != http.StatusOK {
			// Check if retryable (5xx errors)
			if httpResp.StatusCode >= 500 {
				return &RetryableError{
					Err: &APIError{
						StatusCode: httpResp.StatusCode,
						Body:       string(respBody),
					},
				}
			}

			// Parse error response for better error message
			var errResp errorResponse
			if json.Unmarshal(respBody, &errResp) == nil && errResp.Error.Message != "" {
				return &APIError{
					StatusCode: httpResp.StatusCode,
					Body:       errResp.Error.Message,
					Code:       errResp.Error.Code,
				}
			}

			return &APIError{
				StatusCode: httpResp.StatusCode,
				Body:       string(respBody),
			}
		}

		// Parse successful response
		resp = &CompletionResponse{}
		if err := json.Unmarshal(respBody, resp); err != nil {
			return fmt.Errorf("unmarshal response: %w", err)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return resp, nil
}

// WithModel returns a new CompletionRequest with the specified model.
func (r CompletionRequest) WithModel(model string) CompletionRequest {
	r.Model = model
	return r
}

// WithTemperature returns a new CompletionRequest with the specified temperature.
func (r CompletionRequest) WithTemperature(temp float64) CompletionRequest {
	r.Temperature = temp
	return r
}

// WithMaxTokens returns a new CompletionRequest with the specified max tokens.
func (r CompletionRequest) WithMaxTokens(maxTokens int) CompletionRequest {
	r.MaxTokens = maxTokens
	return r
}

// SystemMessage creates a system message.
func SystemMessage(content string) Message {
	return Message{Role: "system", Content: content}
}

// UserMessage creates a user message.
func UserMessage(content string) Message {
	return Message{Role: "user", Content: content}
}

// AssistantMessage creates an assistant message.
func AssistantMessage(content string) Message {
	return Message{Role: "assistant", Content: content}
}

// QualityModel returns the quality model from the client's configuration.
// This is useful for callers who want to use the higher-quality model
// for specific tasks like clinical extraction.
func (c *openAIClient) QualityModel() string {
	return c.config.QualityModel
}

// ClientWithQualityModel is an extended interface for clients that support
// a quality model for complex reasoning tasks.
type ClientWithQualityModel interface {
	Client
	QualityModel() string
}

// GetMetrics returns current client metrics.
func (c *openAIClient) GetMetrics() ClientMetrics {
	return ClientMetrics{
		TotalRequests:   c.retryer.TotalRequests(),
		FailedRequests:  c.retryer.FailedRequests(),
		RetriedRequests: c.retryer.RetriedRequests(),
	}
}

// ClientMetrics contains client statistics.
type ClientMetrics struct {
	TotalRequests   int64
	FailedRequests  int64
	RetriedRequests int64
}

// Ensure openAIClient implements all interfaces
var _ Client = (*openAIClient)(nil)
var _ ClientWithQualityModel = (*openAIClient)(nil)

// Helper function to create a basic completion request
func NewCompletionRequest(systemPrompt, userPrompt string) CompletionRequest {
	messages := []Message{}
	if systemPrompt != "" {
		messages = append(messages, SystemMessage(systemPrompt))
	}
	if userPrompt != "" {
		messages = append(messages, UserMessage(userPrompt))
	}
	return CompletionRequest{
		Messages:    messages,
		Temperature: 0.2, // Default to low temperature for deterministic results
	}
}

// NewConversation creates a CompletionRequest with a conversation history.
func NewConversation(systemPrompt string, messages ...Message) CompletionRequest {
	allMessages := []Message{}
	if systemPrompt != "" {
		allMessages = append(allMessages, SystemMessage(systemPrompt))
	}
	allMessages = append(allMessages, messages...)
	return CompletionRequest{
		Messages:    allMessages,
		Temperature: 0.2,
	}
}

// WaitForReady waits for the LLM endpoint to become available.
func WaitForReady(ctx context.Context, client Client, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Try a simple completion
		_, err := client.Complete(ctx, CompletionRequest{
			Messages:  []Message{UserMessage("ping")},
			MaxTokens: 1,
		})
		if err == nil {
			return nil
		}

		// Wait before retry
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(1 * time.Second):
		}
	}

	return fmt.Errorf("LLM endpoint not ready after %v", timeout)
}
