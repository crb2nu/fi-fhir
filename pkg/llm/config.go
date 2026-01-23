// Package llm provides an OpenAI-compatible LLM client for fi-fhir.
package llm

import (
	"os"
	"time"
)

// Config holds the configuration for the LLM client.
type Config struct {
	// BaseURL is the base URL for the LLM API endpoint.
	// Example: "http://litellm.ai.svc.cluster.local:8000/v1"
	BaseURL string `yaml:"base_url" json:"base_url"`

	// APIKey is the API key for authentication.
	// Can be overridden by environment variables.
	APIKey string `yaml:"api_key" json:"api_key"`

	// DefaultModel is the default model to use for completions.
	// Example: "qwen3-8b-fast"
	DefaultModel string `yaml:"default_model" json:"default_model"`

	// QualityModel is a higher-quality model for clinical extraction and reasoning.
	// Example: "qwen3-14b-quality"
	QualityModel string `yaml:"quality_model" json:"quality_model"`

	// Timeout is the request timeout for API calls.
	Timeout time.Duration `yaml:"timeout" json:"timeout"`

	// MaxRetries is the maximum number of retry attempts.
	MaxRetries int `yaml:"max_retries" json:"max_retries"`

	// RetryBaseDelay is the base delay for exponential backoff.
	RetryBaseDelay time.Duration `yaml:"retry_base_delay" json:"retry_base_delay"`

	// RetryMaxDelay is the maximum delay between retries.
	RetryMaxDelay time.Duration `yaml:"retry_max_delay" json:"retry_max_delay"`
}

// EmbeddingConfig holds configuration for embedding operations.
type EmbeddingConfig struct {
	// BaseURL is the base URL for the embedding API endpoint.
	BaseURL string `yaml:"base_url" json:"base_url"`

	// APIKey is the API key for authentication.
	APIKey string `yaml:"api_key" json:"api_key"`

	// Model is the embedding model to use.
	// Example: "bge-large-embeddings"
	Model string `yaml:"model" json:"model"`

	// Dimensions is the expected embedding dimensionality.
	// Example: 1024 for bge-large
	Dimensions int `yaml:"dimensions" json:"dimensions"`

	// Timeout is the request timeout for embedding API calls.
	Timeout time.Duration `yaml:"timeout" json:"timeout"`

	// MaxRetries is the maximum number of retry attempts.
	MaxRetries int `yaml:"max_retries" json:"max_retries"`

	// BatchSize is the maximum number of texts per embedding request.
	BatchSize int `yaml:"batch_size" json:"batch_size"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		BaseURL:        "http://litellm.ai.svc.cluster.local:8000/v1",
		DefaultModel:   "qwen3-8b-fast",
		QualityModel:   "qwen3-14b-quality",
		Timeout:        30 * time.Second,
		MaxRetries:     3,
		RetryBaseDelay: 100 * time.Millisecond,
		RetryMaxDelay:  5 * time.Second,
	}
}

// DefaultEmbeddingConfig returns an EmbeddingConfig with sensible defaults.
func DefaultEmbeddingConfig() EmbeddingConfig {
	return EmbeddingConfig{
		BaseURL:    "http://litellm.ai.svc.cluster.local:8000/v1",
		Model:      "bge-large-embeddings",
		Dimensions: 1024,
		Timeout:    60 * time.Second,
		MaxRetries: 3,
		BatchSize:  32,
	}
}

// WithEnv returns a new Config with environment variable overrides applied.
// Environment variables take precedence over struct values.
//
// Supported environment variables:
//   - LLM_BASE_URL: Override BaseURL
//   - LLM_API_KEY: Override APIKey
//   - OPENAI_API_KEY: Fallback for APIKey
//   - LLM_DEFAULT_MODEL: Override DefaultModel
//   - LLM_QUALITY_MODEL: Override QualityModel
func (c Config) WithEnv() Config {
	if v := os.Getenv("LLM_BASE_URL"); v != "" {
		c.BaseURL = v
	}

	// API key with fallback chain
	if v := os.Getenv("LLM_API_KEY"); v != "" {
		c.APIKey = v
	} else if v := os.Getenv("OPENAI_API_KEY"); v != "" && c.APIKey == "" {
		c.APIKey = v
	}

	if v := os.Getenv("LLM_DEFAULT_MODEL"); v != "" {
		c.DefaultModel = v
	}

	if v := os.Getenv("LLM_QUALITY_MODEL"); v != "" {
		c.QualityModel = v
	}

	return c
}

// WithEnv returns a new EmbeddingConfig with environment variable overrides applied.
//
// Supported environment variables:
//   - LLM_EMBEDDING_BASE_URL: Override BaseURL
//   - LLM_EMBEDDING_API_KEY: Override APIKey
//   - LLM_API_KEY: Fallback for APIKey
//   - LLM_EMBEDDING_MODEL: Override Model
func (c EmbeddingConfig) WithEnv() EmbeddingConfig {
	if v := os.Getenv("LLM_EMBEDDING_BASE_URL"); v != "" {
		c.BaseURL = v
	} else if v := os.Getenv("LLM_BASE_URL"); v != "" && c.BaseURL == "" {
		c.BaseURL = v
	}

	// API key with fallback chain
	if v := os.Getenv("LLM_EMBEDDING_API_KEY"); v != "" {
		c.APIKey = v
	} else if v := os.Getenv("LLM_API_KEY"); v != "" && c.APIKey == "" {
		c.APIKey = v
	}

	if v := os.Getenv("LLM_EMBEDDING_MODEL"); v != "" {
		c.Model = v
	}

	return c
}

// Validate checks if the Config has all required fields.
func (c Config) Validate() error {
	if c.BaseURL == "" {
		return ErrMissingBaseURL
	}
	if c.DefaultModel == "" {
		return ErrMissingModel
	}
	return nil
}

// Validate checks if the EmbeddingConfig has all required fields.
func (c EmbeddingConfig) Validate() error {
	if c.BaseURL == "" {
		return ErrMissingBaseURL
	}
	if c.Model == "" {
		return ErrMissingModel
	}
	if c.Dimensions <= 0 {
		return ErrInvalidDimensions
	}
	return nil
}
