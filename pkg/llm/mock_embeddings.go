package llm

import (
	"context"
	"sync"
)

// MockEmbeddingClient is a test double for the EmbeddingClient interface.
type MockEmbeddingClient struct {
	// EmbedFunc is called when Embed is invoked.
	// If nil, returns mock embeddings.
	EmbedFunc func(ctx context.Context, texts []string) ([][]float64, error)

	// EmbedSingleFunc is called when EmbedSingle is invoked.
	// If nil, uses EmbedFunc or returns mock embedding.
	EmbedSingleFunc func(ctx context.Context, text string) ([]float64, error)

	// DimensionsValue is returned by Dimensions().
	DimensionsValue int

	// ModelValue is returned by Model().
	ModelValue string

	// Calls records all texts passed to Embed.
	Calls [][]string

	// SingleCalls records all texts passed to EmbedSingle.
	SingleCalls []string

	// mu protects Calls and SingleCalls.
	mu sync.Mutex
}

// NewMockEmbeddingClient creates a new MockEmbeddingClient with default values.
func NewMockEmbeddingClient() *MockEmbeddingClient {
	return &MockEmbeddingClient{
		DimensionsValue: 1024,
		ModelValue:      "mock-embedding-model",
	}
}

// Embed implements EmbeddingClient.Embed.
func (m *MockEmbeddingClient) Embed(ctx context.Context, texts []string) ([][]float64, error) {
	m.mu.Lock()
	m.Calls = append(m.Calls, texts)
	m.mu.Unlock()

	if m.EmbedFunc != nil {
		return m.EmbedFunc(ctx, texts)
	}

	// Generate mock embeddings
	embeddings := make([][]float64, len(texts))
	for i := range texts {
		embeddings[i] = m.generateMockEmbedding()
	}
	return embeddings, nil
}

// EmbedSingle implements EmbeddingClient.EmbedSingle.
func (m *MockEmbeddingClient) EmbedSingle(ctx context.Context, text string) ([]float64, error) {
	m.mu.Lock()
	m.SingleCalls = append(m.SingleCalls, text)
	m.mu.Unlock()

	if m.EmbedSingleFunc != nil {
		return m.EmbedSingleFunc(ctx, text)
	}

	if m.EmbedFunc != nil {
		embeddings, err := m.EmbedFunc(ctx, []string{text})
		if err != nil {
			return nil, err
		}
		if len(embeddings) > 0 {
			return embeddings[0], nil
		}
	}

	return m.generateMockEmbedding(), nil
}

// Dimensions implements EmbeddingClient.Dimensions.
func (m *MockEmbeddingClient) Dimensions() int {
	return m.DimensionsValue
}

// Model implements EmbeddingClient.Model.
func (m *MockEmbeddingClient) Model() string {
	return m.ModelValue
}

// generateMockEmbedding generates a deterministic mock embedding vector.
func (m *MockEmbeddingClient) generateMockEmbedding() []float64 {
	dims := m.DimensionsValue
	if dims <= 0 {
		dims = 1024
	}
	embedding := make([]float64, dims)
	for i := range embedding {
		// Simple deterministic pattern
		embedding[i] = float64(i%10) / 10.0
	}
	return embedding
}

// GetCalls returns a copy of all recorded batch calls.
func (m *MockEmbeddingClient) GetCalls() [][]string {
	m.mu.Lock()
	defer m.mu.Unlock()
	calls := make([][]string, len(m.Calls))
	for i, c := range m.Calls {
		calls[i] = make([]string, len(c))
		copy(calls[i], c)
	}
	return calls
}

// GetSingleCalls returns a copy of all recorded single calls.
func (m *MockEmbeddingClient) GetSingleCalls() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	calls := make([]string, len(m.SingleCalls))
	copy(calls, m.SingleCalls)
	return calls
}

// Reset clears all recorded calls.
func (m *MockEmbeddingClient) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Calls = nil
	m.SingleCalls = nil
}

// CallCount returns the total number of calls (batch + single).
func (m *MockEmbeddingClient) CallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.Calls) + len(m.SingleCalls)
}

// WithError configures the mock to return an error.
func (m *MockEmbeddingClient) WithError(err error) *MockEmbeddingClient {
	m.EmbedFunc = func(ctx context.Context, texts []string) ([][]float64, error) {
		return nil, err
	}
	m.EmbedSingleFunc = func(ctx context.Context, text string) ([]float64, error) {
		return nil, err
	}
	return m
}

// WithFixedEmbedding configures the mock to always return the same embedding.
func (m *MockEmbeddingClient) WithFixedEmbedding(embedding []float64) *MockEmbeddingClient {
	m.DimensionsValue = len(embedding)
	m.EmbedFunc = func(ctx context.Context, texts []string) ([][]float64, error) {
		embeddings := make([][]float64, len(texts))
		for i := range texts {
			emb := make([]float64, len(embedding))
			copy(emb, embedding)
			embeddings[i] = emb
		}
		return embeddings, nil
	}
	m.EmbedSingleFunc = func(ctx context.Context, text string) ([]float64, error) {
		emb := make([]float64, len(embedding))
		copy(emb, embedding)
		return emb, nil
	}
	return m
}

// Ensure MockEmbeddingClient implements EmbeddingClient.
var _ EmbeddingClient = (*MockEmbeddingClient)(nil)
