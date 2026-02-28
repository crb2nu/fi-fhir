package suggest

import (
	"os"
	"testing"
	"time"
)

func TestDefaultSuggesterConfig(t *testing.T) {
	cfg := DefaultSuggesterConfig()

	if cfg.QdrantURL == "" {
		t.Error("QdrantURL should have a default value")
	}
	if cfg.EmbeddingBaseURL == "" {
		t.Error("EmbeddingBaseURL should have a default value")
	}
	if cfg.EmbeddingModel == "" {
		t.Error("EmbeddingModel should have a default value")
	}
	if cfg.EmbeddingDimensions != 1024 {
		t.Errorf("EmbeddingDimensions = %d, want 1024", cfg.EmbeddingDimensions)
	}
	if cfg.LLMBaseURL == "" {
		t.Error("LLMBaseURL should have a default value")
	}
	if cfg.LLMModel == "" {
		t.Error("LLMModel should have a default value")
	}
	if cfg.DefaultMaxResults != 5 {
		t.Errorf("DefaultMaxResults = %d, want 5", cfg.DefaultMaxResults)
	}
	if cfg.DefaultMinConfidence != 0.5 {
		t.Errorf("DefaultMinConfidence = %f, want 0.5", cfg.DefaultMinConfidence)
	}
	if !cfg.EnableLLMReasoning {
		t.Error("EnableLLMReasoning should default to true")
	}
	if !cfg.EnableFeedback {
		t.Error("EnableFeedback should default to true")
	}
	if cfg.FeedbackCollection != "fi_fhir_code_feedback" {
		t.Errorf("FeedbackCollection = %q, want %q", cfg.FeedbackCollection, "fi_fhir_code_feedback")
	}
	if cfg.Timeout != 60*time.Second {
		t.Errorf("Timeout = %v, want 60s", cfg.Timeout)
	}
}

func TestSuggesterConfig_WithEnv(t *testing.T) {
	// Set env vars for the test.
	envVars := map[string]string{
		"QDRANT_URL":         "http://test-qdrant:6333",
		"QDRANT_API_KEY":     "test-key",
		"EMBEDDING_BASE_URL": "http://test-embedding:8000",
		"EMBEDDING_API_KEY":  "embed-key",
		"EMBEDDING_MODEL":    "test-model",
		"LLM_BASE_URL":       "http://test-llm:8000",
		"LLM_API_KEY":        "llm-key",
		"LLM_MODEL":          "test-llm-model",
	}

	for k, v := range envVars {
		t.Setenv(k, v)
	}

	cfg := DefaultSuggesterConfig().WithEnv()

	if cfg.QdrantURL != "http://test-qdrant:6333" {
		t.Errorf("QdrantURL = %q, want env override", cfg.QdrantURL)
	}
	if cfg.QdrantAPIKey != "test-key" {
		t.Errorf("QdrantAPIKey = %q, want env override", cfg.QdrantAPIKey)
	}
	if cfg.EmbeddingBaseURL != "http://test-embedding:8000" {
		t.Errorf("EmbeddingBaseURL = %q, want env override", cfg.EmbeddingBaseURL)
	}
	if cfg.EmbeddingAPIKey != "embed-key" {
		t.Errorf("EmbeddingAPIKey = %q, want env override", cfg.EmbeddingAPIKey)
	}
	if cfg.EmbeddingModel != "test-model" {
		t.Errorf("EmbeddingModel = %q, want env override", cfg.EmbeddingModel)
	}
	if cfg.LLMBaseURL != "http://test-llm:8000" {
		t.Errorf("LLMBaseURL = %q, want env override", cfg.LLMBaseURL)
	}
	if cfg.LLMAPIKey != "llm-key" {
		t.Errorf("LLMAPIKey = %q, want env override", cfg.LLMAPIKey)
	}
	if cfg.LLMModel != "test-llm-model" {
		t.Errorf("LLMModel = %q, want env override", cfg.LLMModel)
	}
}

func TestSuggesterConfig_WithEnv_EmptyPreservesDefaults(t *testing.T) {
	// Ensure env vars are NOT set.
	for _, key := range []string{
		"QDRANT_URL", "QDRANT_API_KEY", "EMBEDDING_BASE_URL",
		"EMBEDDING_API_KEY", "EMBEDDING_MODEL",
		"LLM_BASE_URL", "LLM_API_KEY", "LLM_MODEL",
	} {
		os.Unsetenv(key)
	}

	defaults := DefaultSuggesterConfig()
	withEnv := defaults.WithEnv()

	if withEnv.QdrantURL != defaults.QdrantURL {
		t.Error("WithEnv() should preserve defaults when env vars are empty")
	}
	if withEnv.EmbeddingModel != defaults.EmbeddingModel {
		t.Error("WithEnv() should preserve EmbeddingModel when unset")
	}
}

func TestSuggesterConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(*SuggesterConfig)
		wantErr bool
	}{
		{
			name:    "valid defaults",
			modify:  func(c *SuggesterConfig) {},
			wantErr: false,
		},
		{
			name:    "missing qdrant_url",
			modify:  func(c *SuggesterConfig) { c.QdrantURL = "" },
			wantErr: true,
		},
		{
			name:    "missing embedding_base_url",
			modify:  func(c *SuggesterConfig) { c.EmbeddingBaseURL = "" },
			wantErr: true,
		},
		{
			name:    "missing embedding_model",
			modify:  func(c *SuggesterConfig) { c.EmbeddingModel = "" },
			wantErr: true,
		},
		{
			name: "llm reasoning enabled but missing llm_base_url",
			modify: func(c *SuggesterConfig) {
				c.EnableLLMReasoning = true
				c.LLMBaseURL = ""
			},
			wantErr: true,
		},
		{
			name: "llm reasoning enabled but missing llm_model",
			modify: func(c *SuggesterConfig) {
				c.EnableLLMReasoning = true
				c.LLMModel = ""
			},
			wantErr: true,
		},
		{
			name: "llm reasoning disabled allows empty llm fields",
			modify: func(c *SuggesterConfig) {
				c.EnableLLMReasoning = false
				c.LLMBaseURL = ""
				c.LLMModel = ""
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultSuggesterConfig()
			tt.modify(&cfg)

			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
