//nolint:errcheck,errorlint,gosec // Test file - error checking omitted for brevity
package llm

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewEmbeddingClient(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		cfg := EmbeddingConfig{
			BaseURL:    "http://localhost:8000/v1",
			Model:      "test-model",
			Dimensions: 1024,
			Timeout:    30 * time.Second,
		}
		client, err := NewEmbeddingClient(cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if client == nil {
			t.Fatal("client should not be nil")
		}
	})

	t.Run("invalid config - missing base URL", func(t *testing.T) {
		cfg := EmbeddingConfig{
			Model:      "test-model",
			Dimensions: 1024,
		}
		_, err := NewEmbeddingClient(cfg)
		if err == nil {
			t.Fatal("expected error for missing base URL")
		}
	})

	t.Run("invalid config - missing model", func(t *testing.T) {
		cfg := EmbeddingConfig{
			BaseURL:    "http://localhost:8000/v1",
			Dimensions: 1024,
		}
		_, err := NewEmbeddingClient(cfg)
		if err == nil {
			t.Fatal("expected error for missing model")
		}
	})

	t.Run("invalid config - invalid dimensions", func(t *testing.T) {
		cfg := EmbeddingConfig{
			BaseURL:    "http://localhost:8000/v1",
			Model:      "test-model",
			Dimensions: 0,
		}
		_, err := NewEmbeddingClient(cfg)
		if err == nil {
			t.Fatal("expected error for invalid dimensions")
		}
	})
}

func TestNewEmbeddingClientWithDefaults(t *testing.T) {
	client, err := NewEmbeddingClientWithDefaults()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client == nil {
		t.Fatal("client should not be nil")
	}
}

func TestEmbeddingClientEmbed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/embeddings" {
			t.Errorf("expected /embeddings, got %s", r.URL.Path)
		}

		// Parse request
		var req embeddingRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request: %v", err)
		}

		if req.Model != "test-model" {
			t.Errorf("expected model 'test-model', got '%s'", req.Model)
		}

		// Generate mock embeddings
		data := make([]embeddingData, len(req.Input))
		for i := range req.Input {
			embedding := make([]float64, 1024)
			for j := range embedding {
				embedding[j] = float64(i+j) / 1000.0
			}
			data[i] = embeddingData{
				Object:    "embedding",
				Embedding: embedding,
				Index:     i,
			}
		}

		resp := embeddingResponse{
			Object: "list",
			Data:   data,
			Model:  req.Model,
			Usage: embeddingUsage{
				PromptTokens: len(req.Input) * 10,
				TotalTokens:  len(req.Input) * 10,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := EmbeddingConfig{
		BaseURL:    server.URL,
		APIKey:     "test-key",
		Model:      "test-model",
		Dimensions: 1024,
		Timeout:    5 * time.Second,
	}
	client, err := NewEmbeddingClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	ctx := context.Background()

	t.Run("embed single text", func(t *testing.T) {
		embeddings, err := client.Embed(ctx, []string{"test text"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(embeddings) != 1 {
			t.Errorf("expected 1 embedding, got %d", len(embeddings))
		}
		if len(embeddings[0]) != 1024 {
			t.Errorf("expected 1024 dimensions, got %d", len(embeddings[0]))
		}
	})

	t.Run("embed multiple texts", func(t *testing.T) {
		texts := []string{"text 1", "text 2", "text 3"}
		embeddings, err := client.Embed(ctx, texts)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(embeddings) != 3 {
			t.Errorf("expected 3 embeddings, got %d", len(embeddings))
		}
	})

	t.Run("embed empty list", func(t *testing.T) {
		embeddings, err := client.Embed(ctx, []string{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if embeddings != nil {
			t.Errorf("expected nil for empty input, got %v", embeddings)
		}
	})
}

func TestEmbeddingClientEmbedSingle(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		embedding := make([]float64, 1024)
		for i := range embedding {
			embedding[i] = float64(i) / 1024.0
		}

		resp := embeddingResponse{
			Object: "list",
			Data: []embeddingData{
				{
					Object:    "embedding",
					Embedding: embedding,
					Index:     0,
				},
			},
			Model: "test-model",
		}

		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := EmbeddingConfig{
		BaseURL:    server.URL,
		Model:      "test-model",
		Dimensions: 1024,
		Timeout:    5 * time.Second,
	}
	client, err := NewEmbeddingClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	embedding, err := client.EmbedSingle(context.Background(), "test text")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(embedding) != 1024 {
		t.Errorf("expected 1024 dimensions, got %d", len(embedding))
	}
}

func TestEmbeddingClientDimensions(t *testing.T) {
	cfg := EmbeddingConfig{
		BaseURL:    "http://localhost:8000/v1",
		Model:      "test-model",
		Dimensions: 768,
		Timeout:    5 * time.Second,
	}
	client, _ := NewEmbeddingClient(cfg)

	if client.Dimensions() != 768 {
		t.Errorf("Dimensions() = %v, want 768", client.Dimensions())
	}
}

func TestEmbeddingClientModel(t *testing.T) {
	cfg := EmbeddingConfig{
		BaseURL:    "http://localhost:8000/v1",
		Model:      "bge-large",
		Dimensions: 1024,
		Timeout:    5 * time.Second,
	}
	client, _ := NewEmbeddingClient(cfg)

	if client.Model() != "bge-large" {
		t.Errorf("Model() = %v, want 'bge-large'", client.Model())
	}
}

func TestEmbeddingClientBatching(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++

		var req embeddingRequest
		json.NewDecoder(r.Body).Decode(&req)

		// Verify batch size
		if len(req.Input) > 10 {
			t.Errorf("batch size exceeded: got %d, max 10", len(req.Input))
		}

		data := make([]embeddingData, len(req.Input))
		for i := range req.Input {
			embedding := make([]float64, 128)
			data[i] = embeddingData{Embedding: embedding, Index: i}
		}

		resp := embeddingResponse{Data: data}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := EmbeddingConfig{
		BaseURL:    server.URL,
		Model:      "test-model",
		Dimensions: 128,
		Timeout:    5 * time.Second,
		BatchSize:  10, // Small batch size to force batching
	}
	client, _ := NewEmbeddingClient(cfg)

	// Create 25 texts, should result in 3 batches (10, 10, 5)
	texts := make([]string, 25)
	for i := range texts {
		texts[i] = "text"
	}

	embeddings, err := client.Embed(context.Background(), texts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(embeddings) != 25 {
		t.Errorf("expected 25 embeddings, got %d", len(embeddings))
	}

	if callCount != 3 {
		t.Errorf("expected 3 API calls (batches), got %d", callCount)
	}
}

func TestEmbeddingClientErrorHandling(t *testing.T) {
	t.Run("API error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("Invalid model"))
		}))
		defer server.Close()

		cfg := EmbeddingConfig{
			BaseURL:    server.URL,
			Model:      "invalid-model",
			Dimensions: 1024,
			Timeout:    5 * time.Second,
		}
		client, _ := NewEmbeddingClient(cfg)

		_, err := client.Embed(context.Background(), []string{"test"})
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
	})

	t.Run("server error with retry", func(t *testing.T) {
		attempts := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			attempts++
			if attempts < 2 {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			resp := embeddingResponse{
				Data: []embeddingData{
					{Embedding: make([]float64, 128), Index: 0},
				},
			}
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		cfg := EmbeddingConfig{
			BaseURL:    server.URL,
			Model:      "test-model",
			Dimensions: 128,
			Timeout:    5 * time.Second,
			MaxRetries: 3,
		}
		client, _ := NewEmbeddingClient(cfg)

		_, err := client.Embed(context.Background(), []string{"test"})
		if err != nil {
			t.Errorf("unexpected error after retry: %v", err)
		}
		if attempts < 2 {
			t.Errorf("expected at least 2 attempts, got %d", attempts)
		}
	})
}

func TestMockEmbeddingClient(t *testing.T) {
	t.Run("default behavior", func(t *testing.T) {
		mock := NewMockEmbeddingClient()

		embeddings, err := mock.Embed(context.Background(), []string{"test1", "test2"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(embeddings) != 2 {
			t.Errorf("expected 2 embeddings, got %d", len(embeddings))
		}
		if len(embeddings[0]) != mock.DimensionsValue {
			t.Errorf("expected %d dimensions, got %d", mock.DimensionsValue, len(embeddings[0]))
		}
	})

	t.Run("embed single", func(t *testing.T) {
		mock := NewMockEmbeddingClient()

		embedding, err := mock.EmbedSingle(context.Background(), "test")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(embedding) != mock.DimensionsValue {
			t.Errorf("expected %d dimensions, got %d", mock.DimensionsValue, len(embedding))
		}
	})

	t.Run("dimensions and model", func(t *testing.T) {
		mock := NewMockEmbeddingClient()
		mock.DimensionsValue = 512
		mock.ModelValue = "custom-model"

		if mock.Dimensions() != 512 {
			t.Errorf("Dimensions() = %v, want 512", mock.Dimensions())
		}
		if mock.Model() != "custom-model" {
			t.Errorf("Model() = %v, want 'custom-model'", mock.Model())
		}
	})

	t.Run("records calls", func(t *testing.T) {
		mock := NewMockEmbeddingClient()

		mock.Embed(context.Background(), []string{"batch1", "batch2"})
		mock.EmbedSingle(context.Background(), "single")

		if mock.CallCount() != 2 {
			t.Errorf("CallCount() = %v, want 2", mock.CallCount())
		}

		calls := mock.GetCalls()
		if len(calls) != 1 {
			t.Errorf("len(GetCalls()) = %v, want 1", len(calls))
		}
		if len(calls[0]) != 2 {
			t.Errorf("first batch should have 2 texts")
		}

		singleCalls := mock.GetSingleCalls()
		if len(singleCalls) != 1 {
			t.Errorf("len(GetSingleCalls()) = %v, want 1", len(singleCalls))
		}
	})

	t.Run("with error", func(t *testing.T) {
		mock := NewMockEmbeddingClient()
		testErr := &APIError{StatusCode: 500, Body: "test error"}
		mock.WithError(testErr)

		_, err := mock.Embed(context.Background(), []string{"test"})
		if err != testErr {
			t.Errorf("error = %v, want %v", err, testErr)
		}

		_, err = mock.EmbedSingle(context.Background(), "test")
		if err != testErr {
			t.Errorf("EmbedSingle error = %v, want %v", err, testErr)
		}
	})

	t.Run("with fixed embedding", func(t *testing.T) {
		mock := NewMockEmbeddingClient()
		fixedEmb := []float64{0.1, 0.2, 0.3, 0.4}
		mock.WithFixedEmbedding(fixedEmb)

		embeddings, err := mock.Embed(context.Background(), []string{"test1", "test2"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		for i, emb := range embeddings {
			for j, v := range emb {
				if v != fixedEmb[j] {
					t.Errorf("embeddings[%d][%d] = %v, want %v", i, j, v, fixedEmb[j])
				}
			}
		}

		if mock.Dimensions() != 4 {
			t.Errorf("Dimensions() = %v, want 4", mock.Dimensions())
		}
	})

	t.Run("reset", func(t *testing.T) {
		mock := NewMockEmbeddingClient()
		mock.Embed(context.Background(), []string{"test"})
		mock.EmbedSingle(context.Background(), "test")
		mock.Reset()

		if mock.CallCount() != 0 {
			t.Errorf("CallCount() after Reset() = %v, want 0", mock.CallCount())
		}
	})

	t.Run("custom embed func", func(t *testing.T) {
		mock := NewMockEmbeddingClient()
		mock.EmbedFunc = func(ctx context.Context, texts []string) ([][]float64, error) {
			result := make([][]float64, len(texts))
			for i := range texts {
				result[i] = []float64{float64(len(texts[i]))}
			}
			return result, nil
		}

		embeddings, err := mock.Embed(context.Background(), []string{"a", "ab", "abc"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if embeddings[0][0] != 1 || embeddings[1][0] != 2 || embeddings[2][0] != 3 {
			t.Errorf("custom EmbedFunc not called correctly")
		}
	})
}

func TestCosineSimilarity(t *testing.T) {
	tests := []struct {
		name string
		a    []float64
		b    []float64
		want float64
	}{
		{
			name: "identical vectors",
			a:    []float64{1, 0, 0},
			b:    []float64{1, 0, 0},
			want: 1.0,
		},
		{
			name: "opposite vectors",
			a:    []float64{1, 0, 0},
			b:    []float64{-1, 0, 0},
			want: -1.0,
		},
		{
			name: "orthogonal vectors",
			a:    []float64{1, 0, 0},
			b:    []float64{0, 1, 0},
			want: 0.0,
		},
		{
			name: "45 degree angle",
			a:    []float64{1, 0},
			b:    []float64{1, 1},
			want: 0.7071067811865476, // 1/sqrt(2)
		},
		{
			name: "empty vectors",
			a:    []float64{},
			b:    []float64{},
			want: 0,
		},
		{
			name: "different lengths",
			a:    []float64{1, 2},
			b:    []float64{1, 2, 3},
			want: 0,
		},
		{
			name: "zero vector",
			a:    []float64{0, 0, 0},
			b:    []float64{1, 2, 3},
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CosineSimilarity(tt.a, tt.b)
			if math.Abs(got-tt.want) > 1e-10 {
				t.Errorf("CosineSimilarity(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestNormalizeEmbedding(t *testing.T) {
	t.Run("unit vector unchanged", func(t *testing.T) {
		v := []float64{1, 0, 0}
		normalized := NormalizeEmbedding(v)

		for i, val := range normalized {
			if math.Abs(val-v[i]) > 1e-10 {
				t.Errorf("normalized[%d] = %v, want %v", i, val, v[i])
			}
		}
	})

	t.Run("scales to unit length", func(t *testing.T) {
		v := []float64{3, 4} // Length = 5
		normalized := NormalizeEmbedding(v)

		// Calculate magnitude
		var mag float64
		for _, val := range normalized {
			mag += val * val
		}
		mag = math.Sqrt(mag)

		if math.Abs(mag-1.0) > 1e-10 {
			t.Errorf("magnitude after normalization = %v, want 1.0", mag)
		}

		// Check values
		if math.Abs(normalized[0]-0.6) > 1e-10 {
			t.Errorf("normalized[0] = %v, want 0.6", normalized[0])
		}
		if math.Abs(normalized[1]-0.8) > 1e-10 {
			t.Errorf("normalized[1] = %v, want 0.8", normalized[1])
		}
	})

	t.Run("empty vector", func(t *testing.T) {
		v := []float64{}
		normalized := NormalizeEmbedding(v)
		if len(normalized) != 0 {
			t.Errorf("normalized empty vector should be empty")
		}
	})

	t.Run("zero vector", func(t *testing.T) {
		v := []float64{0, 0, 0}
		normalized := NormalizeEmbedding(v)
		// Zero vector should remain unchanged
		for i, val := range normalized {
			if val != 0 {
				t.Errorf("normalized[%d] = %v, want 0", i, val)
			}
		}
	})
}
