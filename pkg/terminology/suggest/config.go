package suggest

import (
	"fmt"
	"os"
	"time"
)

// SuggesterConfig configures the code suggester.
type SuggesterConfig struct {
	// Qdrant configuration
	QdrantURL    string `yaml:"qdrant_url" json:"qdrant_url"`
	QdrantAPIKey string `yaml:"qdrant_api_key" json:"qdrant_api_key"`

	// Embedding configuration
	EmbeddingBaseURL    string `yaml:"embedding_base_url" json:"embedding_base_url"`
	EmbeddingAPIKey     string `yaml:"embedding_api_key" json:"embedding_api_key"`
	EmbeddingModel      string `yaml:"embedding_model" json:"embedding_model"`
	EmbeddingDimensions int    `yaml:"embedding_dimensions" json:"embedding_dimensions"`

	// LLM configuration
	LLMBaseURL string `yaml:"llm_base_url" json:"llm_base_url"`
	LLMAPIKey  string `yaml:"llm_api_key" json:"llm_api_key"`
	LLMModel   string `yaml:"llm_model" json:"llm_model"`

	// Behavior configuration
	DefaultMaxResults    int     `yaml:"default_max_results" json:"default_max_results"`
	DefaultMinConfidence float64 `yaml:"default_min_confidence" json:"default_min_confidence"`
	EnableLLMReasoning   bool    `yaml:"enable_llm_reasoning" json:"enable_llm_reasoning"`
	EnableFeedback       bool    `yaml:"enable_feedback" json:"enable_feedback"`

	// Feedback store configuration
	FeedbackCollection string `yaml:"feedback_collection" json:"feedback_collection"`

	// Timeout
	Timeout time.Duration `yaml:"timeout" json:"timeout"`
}

// DefaultSuggesterConfig returns a configuration with sensible defaults.
func DefaultSuggesterConfig() SuggesterConfig {
	return SuggesterConfig{
		QdrantURL:            "http://qdrant.ai.svc.cluster.local:6333",
		EmbeddingBaseURL:     "http://litellm.ai.svc.cluster.local:8000/v1",
		EmbeddingModel:       "bge-large-embeddings",
		EmbeddingDimensions:  1024,
		LLMBaseURL:           "http://litellm.ai.svc.cluster.local:8000/v1",
		LLMModel:             "qwen3-14b-quality",
		DefaultMaxResults:    5,
		DefaultMinConfidence: 0.5,
		EnableLLMReasoning:   true,
		EnableFeedback:       true,
		FeedbackCollection:   "fi_fhir_code_feedback",
		Timeout:              60 * time.Second,
	}
}

// WithEnv applies environment variable overrides.
func (c SuggesterConfig) WithEnv() SuggesterConfig {
	if v := os.Getenv("QDRANT_URL"); v != "" {
		c.QdrantURL = v
	}
	if v := os.Getenv("QDRANT_API_KEY"); v != "" {
		c.QdrantAPIKey = v
	}
	if v := os.Getenv("EMBEDDING_BASE_URL"); v != "" {
		c.EmbeddingBaseURL = v
	}
	if v := os.Getenv("EMBEDDING_API_KEY"); v != "" {
		c.EmbeddingAPIKey = v
	}
	if v := os.Getenv("EMBEDDING_MODEL"); v != "" {
		c.EmbeddingModel = v
	}
	if v := os.Getenv("LLM_BASE_URL"); v != "" {
		c.LLMBaseURL = v
	}
	if v := os.Getenv("LLM_API_KEY"); v != "" {
		c.LLMAPIKey = v
	}
	if v := os.Getenv("LLM_MODEL"); v != "" {
		c.LLMModel = v
	}
	return c
}

// Validate validates the configuration.
func (c *SuggesterConfig) Validate() error {
	if c.QdrantURL == "" {
		return fmt.Errorf("qdrant_url is required")
	}
	if c.EmbeddingBaseURL == "" {
		return fmt.Errorf("embedding_base_url is required")
	}
	if c.EmbeddingModel == "" {
		return fmt.Errorf("embedding_model is required")
	}
	if c.EnableLLMReasoning && c.LLMBaseURL == "" {
		return fmt.Errorf("llm_base_url is required when llm reasoning is enabled")
	}
	if c.EnableLLMReasoning && c.LLMModel == "" {
		return fmt.Errorf("llm_model is required when llm reasoning is enabled")
	}
	return nil
}
