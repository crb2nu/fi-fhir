package semantic

import (
	"os"
	"testing"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/terminology/index"
)

func TestDefaultSearchConfig(t *testing.T) {
	cfg := DefaultSearchConfig()

	if cfg.QdrantURL == "" {
		t.Error("Expected QdrantURL to be set")
	}
	if cfg.EmbeddingBaseURL == "" {
		t.Error("Expected EmbeddingBaseURL to be set")
	}
	if cfg.EmbeddingModel == "" {
		t.Error("Expected EmbeddingModel to be set")
	}
	if cfg.EmbeddingDimensions <= 0 {
		t.Error("Expected positive EmbeddingDimensions")
	}
	if cfg.DefaultMaxResults <= 0 {
		t.Error("Expected positive DefaultMaxResults")
	}
	if cfg.DefaultMinScore <= 0 || cfg.DefaultMinScore >= 1.0 {
		t.Errorf("DefaultMinScore = %f, expected between 0 and 1", cfg.DefaultMinScore)
	}
	if cfg.Timeout <= 0 {
		t.Error("Expected positive Timeout")
	}
	if cfg.CacheTTL <= 0 {
		t.Error("Expected positive CacheTTL")
	}
	if cfg.CacheMaxSize <= 0 {
		t.Error("Expected positive CacheMaxSize")
	}
}

func TestSearchConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(*SearchConfig)
		wantErr bool
	}{
		{
			name:    "valid default config",
			modify:  func(c *SearchConfig) {},
			wantErr: false,
		},
		{
			name:    "empty QdrantURL",
			modify:  func(c *SearchConfig) { c.QdrantURL = "" },
			wantErr: true,
		},
		{
			name:    "empty EmbeddingBaseURL",
			modify:  func(c *SearchConfig) { c.EmbeddingBaseURL = "" },
			wantErr: true,
		},
		{
			name:    "empty EmbeddingModel",
			modify:  func(c *SearchConfig) { c.EmbeddingModel = "" },
			wantErr: true,
		},
		{
			name:    "zero EmbeddingDimensions",
			modify:  func(c *SearchConfig) { c.EmbeddingDimensions = 0 },
			wantErr: true,
		},
		{
			name:    "negative EmbeddingDimensions",
			modify:  func(c *SearchConfig) { c.EmbeddingDimensions = -1 },
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultSearchConfig()
			tt.modify(&cfg)
			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSearchConfig_WithEnv(t *testing.T) {
	// Save and restore env vars
	envVars := []string{"QDRANT_URL", "QDRANT_API_KEY", "EMBEDDING_BASE_URL", "EMBEDDING_API_KEY", "EMBEDDING_MODEL"}
	saved := make(map[string]string)
	for _, k := range envVars {
		saved[k] = os.Getenv(k)
	}
	t.Cleanup(func() {
		for _, k := range envVars {
			if saved[k] != "" {
				if err := os.Setenv(k, saved[k]); err != nil {
					t.Errorf("restore %s: %v", k, err)
				}
			} else {
				if err := os.Unsetenv(k); err != nil {
					t.Errorf("unset %s: %v", k, err)
				}
			}
		}
	})

	// Set test values
	if err := os.Setenv("QDRANT_URL", "http://custom-qdrant:6333"); err != nil {
		t.Fatalf("set QDRANT_URL: %v", err)
	}
	if err := os.Setenv("QDRANT_API_KEY", "test-api-key"); err != nil {
		t.Fatalf("set QDRANT_API_KEY: %v", err)
	}
	if err := os.Setenv("EMBEDDING_BASE_URL", "http://custom-embeddings:8000"); err != nil {
		t.Fatalf("set EMBEDDING_BASE_URL: %v", err)
	}
	if err := os.Setenv("EMBEDDING_API_KEY", "emb-key-123"); err != nil {
		t.Fatalf("set EMBEDDING_API_KEY: %v", err)
	}
	if err := os.Setenv("EMBEDDING_MODEL", "custom-model"); err != nil {
		t.Fatalf("set EMBEDDING_MODEL: %v", err)
	}

	cfg := DefaultSearchConfig().WithEnv()

	if cfg.QdrantURL != "http://custom-qdrant:6333" {
		t.Errorf("QdrantURL = %s, want http://custom-qdrant:6333", cfg.QdrantURL)
	}
	if cfg.QdrantAPIKey != "test-api-key" {
		t.Errorf("QdrantAPIKey = %s, want test-api-key", cfg.QdrantAPIKey)
	}
	if cfg.EmbeddingBaseURL != "http://custom-embeddings:8000" {
		t.Errorf("EmbeddingBaseURL = %s, want http://custom-embeddings:8000", cfg.EmbeddingBaseURL)
	}
	if cfg.EmbeddingAPIKey != "emb-key-123" {
		t.Errorf("EmbeddingAPIKey = %s, want emb-key-123", cfg.EmbeddingAPIKey)
	}
	if cfg.EmbeddingModel != "custom-model" {
		t.Errorf("EmbeddingModel = %s, want custom-model", cfg.EmbeddingModel)
	}
}

func TestSearchConfig_WithEnv_NoOverrideIfUnset(t *testing.T) {
	// Ensure env vars are NOT set
	envVars := []string{"QDRANT_URL", "QDRANT_API_KEY", "EMBEDDING_BASE_URL", "EMBEDDING_API_KEY", "EMBEDDING_MODEL"}
	saved := make(map[string]string)
	for _, k := range envVars {
		saved[k] = os.Getenv(k)
		if err := os.Unsetenv(k); err != nil {
			t.Fatalf("unset %s: %v", k, err)
		}
	}
	t.Cleanup(func() {
		for _, k := range envVars {
			if saved[k] != "" {
				if err := os.Setenv(k, saved[k]); err != nil {
					t.Errorf("restore %s: %v", k, err)
				}
			}
		}
	})

	defaults := DefaultSearchConfig()
	cfg := defaults.WithEnv()

	if cfg.QdrantURL != defaults.QdrantURL {
		t.Errorf("QdrantURL changed when env unset: got %s, want %s", cfg.QdrantURL, defaults.QdrantURL)
	}
	if cfg.EmbeddingBaseURL != defaults.EmbeddingBaseURL {
		t.Errorf("EmbeddingBaseURL changed when env unset")
	}
	if cfg.EmbeddingModel != defaults.EmbeddingModel {
		t.Errorf("EmbeddingModel changed when env unset")
	}
}

func TestSemanticMatch_Confidence(t *testing.T) {
	tests := []struct {
		name           string
		score          float64
		wantHigh       bool
		wantMedium     bool
		wantAcceptable bool
		threshold      float64
	}{
		{
			name:           "score 0.95 - high confidence",
			score:          0.95,
			wantHigh:       true,
			wantMedium:     true,
			wantAcceptable: true,
			threshold:      0.8,
		},
		{
			name:           "score 0.90 - boundary high",
			score:          0.90,
			wantHigh:       true,
			wantMedium:     true,
			wantAcceptable: true,
			threshold:      0.8,
		},
		{
			name:           "score 0.89 - just below high",
			score:          0.89,
			wantHigh:       false,
			wantMedium:     true,
			wantAcceptable: true,
			threshold:      0.8,
		},
		{
			name:           "score 0.70 - boundary medium",
			score:          0.70,
			wantHigh:       false,
			wantMedium:     true,
			wantAcceptable: false,
			threshold:      0.8,
		},
		{
			name:           "score 0.69 - below medium",
			score:          0.69,
			wantHigh:       false,
			wantMedium:     false,
			wantAcceptable: false,
			threshold:      0.8,
		},
		{
			name:           "score 0.0 - zero",
			score:          0.0,
			wantHigh:       false,
			wantMedium:     false,
			wantAcceptable: false,
			threshold:      0.1,
		},
		{
			name:           "score 1.0 - perfect",
			score:          1.0,
			wantHigh:       true,
			wantMedium:     true,
			wantAcceptable: true,
			threshold:      1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &SemanticMatch{Score: tt.score}

			if got := m.IsHighConfidence(); got != tt.wantHigh {
				t.Errorf("IsHighConfidence() = %v, want %v", got, tt.wantHigh)
			}
			if got := m.IsMediumConfidence(); got != tt.wantMedium {
				t.Errorf("IsMediumConfidence() = %v, want %v", got, tt.wantMedium)
			}
			if got := m.IsAcceptable(tt.threshold); got != tt.wantAcceptable {
				t.Errorf("IsAcceptable(%.1f) = %v, want %v", tt.threshold, got, tt.wantAcceptable)
			}
		})
	}
}

func TestVocabularyCodeSystemURL(t *testing.T) {
	tests := []struct {
		vocab index.Vocabulary
		want  string
	}{
		{index.VocabularyLOINC, "http://loinc.org"},
		{index.VocabularySNOMED, "http://snomed.info/sct"},
		{index.VocabularyICD10CM, "http://hl7.org/fhir/sid/icd-10-cm"},
		{index.VocabularyRxNorm, "http://www.nlm.nih.gov/research/umls/rxnorm"},
		{index.VocabularyCPT, "http://www.ama-assn.org/go/cpt"},
		{index.VocabularyCVX, "http://hl7.org/fhir/sid/cvx"},
		{index.Vocabulary("unknown"), ""},
		{index.Vocabulary(""), ""},
	}

	for _, tt := range tests {
		t.Run(string(tt.vocab), func(t *testing.T) {
			got := VocabularyCodeSystemURL(tt.vocab)
			if got != tt.want {
				t.Errorf("VocabularyCodeSystemURL(%s) = %s, want %s", tt.vocab, got, tt.want)
			}
		})
	}
}

func TestDefaultHybridConfig(t *testing.T) {
	cfg := DefaultHybridConfig()

	if cfg.SemanticFallbackThreshold <= 0 || cfg.SemanticFallbackThreshold >= 1.0 {
		t.Errorf("SemanticFallbackThreshold = %f, expected between 0 and 1", cfg.SemanticFallbackThreshold)
	}
	if cfg.SemanticWeight <= 0 || cfg.SemanticWeight >= 1.0 {
		t.Errorf("SemanticWeight = %f, expected between 0 and 1", cfg.SemanticWeight)
	}
	if cfg.MaxResults <= 0 {
		t.Error("Expected positive MaxResults")
	}
	if cfg.PreferSemantic {
		t.Error("Expected PreferSemantic to be false by default")
	}
}
