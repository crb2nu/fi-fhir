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

// EmbeddingClient provides an interface for generating embeddings.
type EmbeddingClient interface {
	// Embed generates embeddings for the given texts.
	// Returns a slice of embedding vectors, one per input text.
	Embed(ctx context.Context, texts []string) ([][]float64, error)

	// EmbedSingle generates an embedding for a single text.
	EmbedSingle(ctx context.Context, text string) ([]float64, error)

	// Dimensions returns the expected embedding dimensionality.
	Dimensions() int

	// Model returns the embedding model name.
	Model() string
}

// embeddingClient implements EmbeddingClient using the OpenAI-compatible API.
type embeddingClient struct {
	config     EmbeddingConfig
	httpClient *http.Client
	retryer    *Retryer
}

// NewEmbeddingClient creates a new embedding client with the given configuration.
func NewEmbeddingClient(cfg EmbeddingConfig) (EmbeddingClient, error) {
	cfg = cfg.WithEnv()
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &embeddingClient{
		config: cfg,
		httpClient: &http.Client{
			Timeout: cfg.Timeout,
		},
		retryer: NewRetryer(RetryConfig{
			MaxRetries: cfg.MaxRetries,
			BaseDelay:  100 * time.Millisecond,
			MaxDelay:   5 * time.Second,
		}),
	}, nil
}

// NewEmbeddingClientWithDefaults creates a new embedding client with default configuration.
func NewEmbeddingClientWithDefaults() (EmbeddingClient, error) {
	return NewEmbeddingClient(DefaultEmbeddingConfig())
}

// Embed generates embeddings for the given texts.
func (c *embeddingClient) Embed(ctx context.Context, texts []string) ([][]float64, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	// Handle batching if needed
	if len(texts) > c.config.BatchSize && c.config.BatchSize > 0 {
		return c.embedBatched(ctx, texts)
	}

	return c.embedDirect(ctx, texts)
}

// EmbedSingle generates an embedding for a single text.
func (c *embeddingClient) EmbedSingle(ctx context.Context, text string) ([]float64, error) {
	embeddings, err := c.Embed(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(embeddings) == 0 {
		return nil, fmt.Errorf("no embeddings returned")
	}
	return embeddings[0], nil
}

// Dimensions returns the expected embedding dimensionality.
func (c *embeddingClient) Dimensions() int {
	return c.config.Dimensions
}

// Model returns the embedding model name.
func (c *embeddingClient) Model() string {
	return c.config.Model
}

// embedDirect makes a single embedding API call.
func (c *embeddingClient) embedDirect(ctx context.Context, texts []string) ([][]float64, error) {
	// Build the API request
	apiReq := embeddingRequest{
		Model: c.config.Model,
		Input: texts,
	}

	body, err := json.Marshal(apiReq)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	endpoint := strings.TrimSuffix(c.config.BaseURL, "/") + "/embeddings"

	var result [][]float64
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
		defer httpResp.Body.Close() //nolint:errcheck // Best-effort close after read

		respBody, err := io.ReadAll(httpResp.Body)
		if err != nil {
			return fmt.Errorf("read response: %w", err)
		}

		if httpResp.StatusCode != http.StatusOK {
			if httpResp.StatusCode >= 500 {
				return &RetryableError{
					Err: &APIError{
						StatusCode: httpResp.StatusCode,
						Body:       string(respBody),
					},
				}
			}
			return &APIError{
				StatusCode: httpResp.StatusCode,
				Body:       string(respBody),
			}
		}

		var resp embeddingResponse
		if err := json.Unmarshal(respBody, &resp); err != nil {
			return fmt.Errorf("unmarshal response: %w", err)
		}

		// Extract embeddings in order
		result = make([][]float64, len(texts))
		for _, data := range resp.Data {
			if data.Index < len(result) {
				result[data.Index] = data.Embedding
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}

// embedBatched handles embedding requests that exceed the batch size.
func (c *embeddingClient) embedBatched(ctx context.Context, texts []string) ([][]float64, error) {
	result := make([][]float64, len(texts))
	batchSize := c.config.BatchSize
	if batchSize <= 0 {
		batchSize = 32
	}

	for i := 0; i < len(texts); i += batchSize {
		end := i + batchSize
		if end > len(texts) {
			end = len(texts)
		}

		batch := texts[i:end]
		embeddings, err := c.embedDirect(ctx, batch)
		if err != nil {
			return nil, fmt.Errorf("batch %d: %w", i/batchSize, err)
		}

		copy(result[i:end], embeddings)
	}

	return result, nil
}

// Internal types for embedding API

type embeddingRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type embeddingResponse struct {
	Object string          `json:"object"`
	Data   []embeddingData `json:"data"`
	Model  string          `json:"model"`
	Usage  embeddingUsage  `json:"usage"`
}

type embeddingData struct {
	Object    string    `json:"object"`
	Embedding []float64 `json:"embedding"`
	Index     int       `json:"index"`
}

type embeddingUsage struct {
	PromptTokens int `json:"prompt_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

// EmbeddingBatchResult contains the result of a batched embedding operation.
type EmbeddingBatchResult struct {
	// Embeddings contains the generated embeddings.
	Embeddings [][]float64

	// Texts contains the original texts (for reference).
	Texts []string

	// Errors contains any per-text errors (nil if successful).
	Errors []error
}

// TextWithEmbedding pairs a text with its embedding.
type TextWithEmbedding struct {
	Text      string
	Embedding []float64
}

// EmbedWithTexts generates embeddings and returns them paired with the original texts.
func (c *embeddingClient) EmbedWithTexts(ctx context.Context, texts []string) ([]TextWithEmbedding, error) {
	embeddings, err := c.Embed(ctx, texts)
	if err != nil {
		return nil, err
	}

	result := make([]TextWithEmbedding, len(texts))
	for i, text := range texts {
		result[i] = TextWithEmbedding{
			Text:      text,
			Embedding: embeddings[i],
		}
	}

	return result, nil
}

// CosineSimilarity calculates the cosine similarity between two embedding vectors.
func CosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dotProduct, normA, normB float64
	for i := 0; i < len(a); i++ {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dotProduct / (sqrtFloat64(normA) * sqrtFloat64(normB))
}

// sqrtFloat64 is a simple square root implementation to avoid math import.
func sqrtFloat64(x float64) float64 {
	if x <= 0 {
		return 0
	}
	z := x / 2
	for i := 0; i < 100; i++ {
		z = z - (z*z-x)/(2*z)
		if (z*z - x) < 1e-10 {
			break
		}
	}
	return z
}

// NormalizeEmbedding normalizes an embedding vector to unit length.
func NormalizeEmbedding(embedding []float64) []float64 {
	if len(embedding) == 0 {
		return embedding
	}

	var norm float64
	for _, v := range embedding {
		norm += v * v
	}
	norm = sqrtFloat64(norm)

	if norm == 0 {
		return embedding
	}

	result := make([]float64, len(embedding))
	for i, v := range embedding {
		result[i] = v / norm
	}

	return result
}
