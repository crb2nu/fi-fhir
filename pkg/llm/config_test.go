package llm

import (
	"os"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.BaseURL == "" {
		t.Error("DefaultConfig should have a BaseURL")
	}
	if cfg.DefaultModel == "" {
		t.Error("DefaultConfig should have a DefaultModel")
	}
	if cfg.QualityModel == "" {
		t.Error("DefaultConfig should have a QualityModel")
	}
	if cfg.Timeout == 0 {
		t.Error("DefaultConfig should have a Timeout")
	}
	if cfg.MaxRetries == 0 {
		t.Error("DefaultConfig should have MaxRetries")
	}
}

func TestDefaultEmbeddingConfig(t *testing.T) {
	cfg := DefaultEmbeddingConfig()

	if cfg.BaseURL == "" {
		t.Error("DefaultEmbeddingConfig should have a BaseURL")
	}
	if cfg.Model == "" {
		t.Error("DefaultEmbeddingConfig should have a Model")
	}
	if cfg.Dimensions <= 0 {
		t.Error("DefaultEmbeddingConfig should have positive Dimensions")
	}
	if cfg.BatchSize <= 0 {
		t.Error("DefaultEmbeddingConfig should have positive BatchSize")
	}
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr error
	}{
		{
			name:    "valid config",
			cfg:     DefaultConfig(),
			wantErr: nil,
		},
		{
			name: "missing base URL",
			cfg: Config{
				DefaultModel: "test-model",
			},
			wantErr: ErrMissingBaseURL,
		},
		{
			name: "missing model",
			cfg: Config{
				BaseURL: "http://localhost:8000/v1",
			},
			wantErr: ErrMissingModel,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if err != tt.wantErr {
				t.Errorf("Validate() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestEmbeddingConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     EmbeddingConfig
		wantErr error
	}{
		{
			name:    "valid config",
			cfg:     DefaultEmbeddingConfig(),
			wantErr: nil,
		},
		{
			name: "missing base URL",
			cfg: EmbeddingConfig{
				Model:      "test-model",
				Dimensions: 1024,
			},
			wantErr: ErrMissingBaseURL,
		},
		{
			name: "missing model",
			cfg: EmbeddingConfig{
				BaseURL:    "http://localhost:8000/v1",
				Dimensions: 1024,
			},
			wantErr: ErrMissingModel,
		},
		{
			name: "invalid dimensions",
			cfg: EmbeddingConfig{
				BaseURL:    "http://localhost:8000/v1",
				Model:      "test-model",
				Dimensions: 0,
			},
			wantErr: ErrInvalidDimensions,
		},
		{
			name: "negative dimensions",
			cfg: EmbeddingConfig{
				BaseURL:    "http://localhost:8000/v1",
				Model:      "test-model",
				Dimensions: -1,
			},
			wantErr: ErrInvalidDimensions,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if err != tt.wantErr {
				t.Errorf("Validate() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestConfigWithEnv(t *testing.T) {
	// Save original env vars
	origCanonicalBaseURL := os.Getenv("FI_FHIR_LLM_BASE_URL")
	origCanonicalAPIKey := os.Getenv("FI_FHIR_LLM_API_KEY")
	origCanonicalDefaultModel := os.Getenv("FI_FHIR_LLM_DEFAULT_MODEL")
	origCanonicalQualityModel := os.Getenv("FI_FHIR_LLM_QUALITY_MODEL")
	origBaseURL := os.Getenv("LLM_BASE_URL")
	origAPIKey := os.Getenv("LLM_API_KEY")
	origOpenAIKey := os.Getenv("OPENAI_API_KEY")
	origDefaultModel := os.Getenv("LLM_DEFAULT_MODEL")
	origQualityModel := os.Getenv("LLM_QUALITY_MODEL")
	defer func() {
		os.Setenv("FI_FHIR_LLM_BASE_URL", origCanonicalBaseURL)
		os.Setenv("FI_FHIR_LLM_API_KEY", origCanonicalAPIKey)
		os.Setenv("FI_FHIR_LLM_DEFAULT_MODEL", origCanonicalDefaultModel)
		os.Setenv("FI_FHIR_LLM_QUALITY_MODEL", origCanonicalQualityModel)
		os.Setenv("LLM_BASE_URL", origBaseURL)
		os.Setenv("LLM_API_KEY", origAPIKey)
		os.Setenv("OPENAI_API_KEY", origOpenAIKey)
		os.Setenv("LLM_DEFAULT_MODEL", origDefaultModel)
		os.Setenv("LLM_QUALITY_MODEL", origQualityModel)
	}()

	t.Run("canonical env vars override config", func(t *testing.T) {
		os.Setenv("FI_FHIR_LLM_BASE_URL", "http://canonical-server:8000/v1")
		os.Setenv("FI_FHIR_LLM_API_KEY", "canonical-api-key")
		os.Setenv("FI_FHIR_LLM_DEFAULT_MODEL", "canonical-model")
		os.Setenv("FI_FHIR_LLM_QUALITY_MODEL", "canonical-quality-model")
		os.Unsetenv("LLM_BASE_URL")
		os.Unsetenv("LLM_API_KEY")
		os.Unsetenv("OPENAI_API_KEY")
		os.Unsetenv("LLM_DEFAULT_MODEL")
		os.Unsetenv("LLM_QUALITY_MODEL")

		cfg := Config{
			BaseURL:      "http://original:8000/v1",
			APIKey:       "original-key",
			DefaultModel: "original-model",
			QualityModel: "original-quality-model",
		}

		cfg = cfg.WithEnv()

		if cfg.BaseURL != "http://canonical-server:8000/v1" {
			t.Errorf("BaseURL = %v, want %v", cfg.BaseURL, "http://canonical-server:8000/v1")
		}
		if cfg.APIKey != "canonical-api-key" {
			t.Errorf("APIKey = %v, want %v", cfg.APIKey, "canonical-api-key")
		}
		if cfg.DefaultModel != "canonical-model" {
			t.Errorf("DefaultModel = %v, want %v", cfg.DefaultModel, "canonical-model")
		}
		if cfg.QualityModel != "canonical-quality-model" {
			t.Errorf("QualityModel = %v, want %v", cfg.QualityModel, "canonical-quality-model")
		}
	})

	t.Run("legacy env vars remain fallback", func(t *testing.T) {
		os.Unsetenv("FI_FHIR_LLM_BASE_URL")
		os.Unsetenv("FI_FHIR_LLM_API_KEY")
		os.Unsetenv("FI_FHIR_LLM_DEFAULT_MODEL")
		os.Unsetenv("FI_FHIR_LLM_QUALITY_MODEL")
		os.Setenv("LLM_BASE_URL", "http://test-server:8000/v1")
		os.Setenv("LLM_API_KEY", "test-api-key")
		os.Unsetenv("OPENAI_API_KEY")
		os.Setenv("LLM_DEFAULT_MODEL", "test-model")
		os.Setenv("LLM_QUALITY_MODEL", "test-quality-model")

		cfg := Config{
			BaseURL:      "http://original:8000/v1",
			APIKey:       "original-key",
			DefaultModel: "original-model",
			QualityModel: "original-quality-model",
		}

		cfg = cfg.WithEnv()

		if cfg.BaseURL != "http://test-server:8000/v1" {
			t.Errorf("BaseURL = %v, want %v", cfg.BaseURL, "http://test-server:8000/v1")
		}
		if cfg.APIKey != "test-api-key" {
			t.Errorf("APIKey = %v, want %v", cfg.APIKey, "test-api-key")
		}
		if cfg.DefaultModel != "test-model" {
			t.Errorf("DefaultModel = %v, want %v", cfg.DefaultModel, "test-model")
		}
		if cfg.QualityModel != "test-quality-model" {
			t.Errorf("QualityModel = %v, want %v", cfg.QualityModel, "test-quality-model")
		}
	})

	t.Run("canonical env vars take precedence over legacy", func(t *testing.T) {
		os.Setenv("FI_FHIR_LLM_BASE_URL", "http://canonical-server:8000/v1")
		os.Setenv("FI_FHIR_LLM_API_KEY", "canonical-api-key")
		os.Setenv("FI_FHIR_LLM_DEFAULT_MODEL", "canonical-model")
		os.Setenv("FI_FHIR_LLM_QUALITY_MODEL", "canonical-quality-model")
		os.Setenv("LLM_BASE_URL", "http://legacy-server:8000/v1")
		os.Setenv("LLM_API_KEY", "legacy-api-key")
		os.Setenv("OPENAI_API_KEY", "openai-fallback-key")
		os.Setenv("LLM_DEFAULT_MODEL", "legacy-model")
		os.Setenv("LLM_QUALITY_MODEL", "legacy-quality-model")

		cfg := Config{}.WithEnv()

		if cfg.BaseURL != "http://canonical-server:8000/v1" {
			t.Errorf("BaseURL = %v, want canonical value", cfg.BaseURL)
		}
		if cfg.APIKey != "canonical-api-key" {
			t.Errorf("APIKey = %v, want canonical value", cfg.APIKey)
		}
		if cfg.DefaultModel != "canonical-model" {
			t.Errorf("DefaultModel = %v, want canonical value", cfg.DefaultModel)
		}
		if cfg.QualityModel != "canonical-quality-model" {
			t.Errorf("QualityModel = %v, want canonical value", cfg.QualityModel)
		}
	})

	t.Run("OPENAI_API_KEY fallback", func(t *testing.T) {
		os.Unsetenv("FI_FHIR_LLM_BASE_URL")
		os.Unsetenv("FI_FHIR_LLM_API_KEY")
		os.Unsetenv("FI_FHIR_LLM_DEFAULT_MODEL")
		os.Unsetenv("FI_FHIR_LLM_QUALITY_MODEL")
		os.Unsetenv("LLM_BASE_URL")
		os.Unsetenv("LLM_API_KEY")
		os.Setenv("OPENAI_API_KEY", "openai-fallback-key")
		os.Unsetenv("LLM_DEFAULT_MODEL")
		os.Unsetenv("LLM_QUALITY_MODEL")

		cfg := Config{} // Empty config, no API key
		cfg = cfg.WithEnv()

		if cfg.APIKey != "openai-fallback-key" {
			t.Errorf("APIKey = %v, want %v", cfg.APIKey, "openai-fallback-key")
		}
	})

	t.Run("LLM_API_KEY takes precedence over OPENAI_API_KEY", func(t *testing.T) {
		os.Unsetenv("FI_FHIR_LLM_BASE_URL")
		os.Unsetenv("FI_FHIR_LLM_API_KEY")
		os.Unsetenv("FI_FHIR_LLM_DEFAULT_MODEL")
		os.Unsetenv("FI_FHIR_LLM_QUALITY_MODEL")
		os.Unsetenv("LLM_BASE_URL")
		os.Setenv("LLM_API_KEY", "llm-api-key")
		os.Setenv("OPENAI_API_KEY", "openai-fallback-key")
		os.Unsetenv("LLM_DEFAULT_MODEL")
		os.Unsetenv("LLM_QUALITY_MODEL")

		cfg := Config{}
		cfg = cfg.WithEnv()

		if cfg.APIKey != "llm-api-key" {
			t.Errorf("APIKey = %v, want %v", cfg.APIKey, "llm-api-key")
		}
	})

	t.Run("empty env vars don't override", func(t *testing.T) {
		os.Setenv("FI_FHIR_LLM_BASE_URL", "")
		os.Setenv("FI_FHIR_LLM_API_KEY", "")
		os.Setenv("FI_FHIR_LLM_DEFAULT_MODEL", "")
		os.Setenv("FI_FHIR_LLM_QUALITY_MODEL", "")
		os.Setenv("LLM_BASE_URL", "")
		os.Setenv("LLM_API_KEY", "")
		os.Setenv("OPENAI_API_KEY", "")
		os.Setenv("LLM_DEFAULT_MODEL", "")
		os.Setenv("LLM_QUALITY_MODEL", "")

		cfg := Config{
			BaseURL:      "http://original:8000/v1",
			APIKey:       "original-key",
			DefaultModel: "original-model",
			QualityModel: "original-quality-model",
		}

		cfg = cfg.WithEnv()

		if cfg.BaseURL != "http://original:8000/v1" {
			t.Errorf("BaseURL should not change with empty env var")
		}
		if cfg.APIKey != "original-key" {
			t.Errorf("APIKey should not change with empty env var")
		}
		if cfg.DefaultModel != "original-model" {
			t.Errorf("DefaultModel should not change with empty env var")
		}
		if cfg.QualityModel != "original-quality-model" {
			t.Errorf("QualityModel should not change with empty env var")
		}
	})
}

func TestEmbeddingConfigWithEnv(t *testing.T) {
	// Save original env vars
	origBaseURL := os.Getenv("LLM_EMBEDDING_BASE_URL")
	origAPIKey := os.Getenv("LLM_EMBEDDING_API_KEY")
	origModel := os.Getenv("LLM_EMBEDDING_MODEL")
	origLLMBaseURL := os.Getenv("LLM_BASE_URL")
	origLLMAPIKey := os.Getenv("LLM_API_KEY")
	defer func() {
		os.Setenv("LLM_EMBEDDING_BASE_URL", origBaseURL)
		os.Setenv("LLM_EMBEDDING_API_KEY", origAPIKey)
		os.Setenv("LLM_EMBEDDING_MODEL", origModel)
		os.Setenv("LLM_BASE_URL", origLLMBaseURL)
		os.Setenv("LLM_API_KEY", origLLMAPIKey)
	}()

	t.Run("env vars override config", func(t *testing.T) {
		os.Setenv("LLM_EMBEDDING_BASE_URL", "http://embedding-server:8000/v1")
		os.Setenv("LLM_EMBEDDING_API_KEY", "embedding-api-key")
		os.Setenv("LLM_EMBEDDING_MODEL", "embedding-model")
		os.Unsetenv("LLM_BASE_URL")
		os.Unsetenv("LLM_API_KEY")

		cfg := EmbeddingConfig{
			BaseURL:    "http://original:8000/v1",
			APIKey:     "original-key",
			Model:      "original-model",
			Dimensions: 1024,
		}

		cfg = cfg.WithEnv()

		if cfg.BaseURL != "http://embedding-server:8000/v1" {
			t.Errorf("BaseURL = %v, want %v", cfg.BaseURL, "http://embedding-server:8000/v1")
		}
		if cfg.APIKey != "embedding-api-key" {
			t.Errorf("APIKey = %v, want %v", cfg.APIKey, "embedding-api-key")
		}
		if cfg.Model != "embedding-model" {
			t.Errorf("Model = %v, want %v", cfg.Model, "embedding-model")
		}
	})

	t.Run("LLM_BASE_URL fallback for BaseURL", func(t *testing.T) {
		os.Unsetenv("LLM_EMBEDDING_BASE_URL")
		os.Unsetenv("LLM_EMBEDDING_API_KEY")
		os.Unsetenv("LLM_EMBEDDING_MODEL")
		os.Setenv("LLM_BASE_URL", "http://llm-fallback:8000/v1")
		os.Unsetenv("LLM_API_KEY")

		cfg := EmbeddingConfig{} // Empty config
		cfg = cfg.WithEnv()

		if cfg.BaseURL != "http://llm-fallback:8000/v1" {
			t.Errorf("BaseURL = %v, want %v", cfg.BaseURL, "http://llm-fallback:8000/v1")
		}
	})

	t.Run("LLM_API_KEY fallback for APIKey", func(t *testing.T) {
		os.Unsetenv("LLM_EMBEDDING_BASE_URL")
		os.Unsetenv("LLM_EMBEDDING_API_KEY")
		os.Unsetenv("LLM_EMBEDDING_MODEL")
		os.Unsetenv("LLM_BASE_URL")
		os.Setenv("LLM_API_KEY", "llm-fallback-key")

		cfg := EmbeddingConfig{} // Empty config
		cfg = cfg.WithEnv()

		if cfg.APIKey != "llm-fallback-key" {
			t.Errorf("APIKey = %v, want %v", cfg.APIKey, "llm-fallback-key")
		}
	})
}

func TestConfigDurations(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Timeout < time.Second {
		t.Error("Timeout should be at least 1 second")
	}
	if cfg.RetryBaseDelay < time.Millisecond {
		t.Error("RetryBaseDelay should be at least 1 millisecond")
	}
	if cfg.RetryMaxDelay < cfg.RetryBaseDelay {
		t.Error("RetryMaxDelay should be greater than or equal to RetryBaseDelay")
	}
}
