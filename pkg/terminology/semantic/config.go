package semantic

import (
	"fmt"
	"os"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/terminology/index"
)

// SearchConfig configures the semantic searcher.
type SearchConfig struct {
	// QdrantURL is the Qdrant server URL.
	QdrantURL string `yaml:"qdrant_url" json:"qdrant_url"`

	// QdrantAPIKey is the optional API key for Qdrant.
	QdrantAPIKey string `yaml:"qdrant_api_key" json:"qdrant_api_key"`

	// EmbeddingBaseURL is the embedding API endpoint.
	EmbeddingBaseURL string `yaml:"embedding_base_url" json:"embedding_base_url"`

	// EmbeddingAPIKey is the embedding API key.
	EmbeddingAPIKey string `yaml:"embedding_api_key" json:"embedding_api_key"`

	// EmbeddingModel is the model to use for embeddings.
	EmbeddingModel string `yaml:"embedding_model" json:"embedding_model"`

	// EmbeddingDimensions is the embedding dimensionality.
	EmbeddingDimensions int `yaml:"embedding_dimensions" json:"embedding_dimensions"`

	// DefaultMaxResults is the default number of results to return.
	DefaultMaxResults int `yaml:"default_max_results" json:"default_max_results"`

	// DefaultMinScore is the default minimum similarity score.
	DefaultMinScore float64 `yaml:"default_min_score" json:"default_min_score"`

	// Timeout is the request timeout.
	Timeout time.Duration `yaml:"timeout" json:"timeout"`

	// EnableCache enables result caching.
	EnableCache bool `yaml:"enable_cache" json:"enable_cache"`

	// CacheTTL is the cache time-to-live.
	CacheTTL time.Duration `yaml:"cache_ttl" json:"cache_ttl"`

	// CacheMaxSize is the maximum number of cached entries.
	CacheMaxSize int `yaml:"cache_max_size" json:"cache_max_size"`
}

// DefaultSearchConfig returns a configuration with sensible defaults.
func DefaultSearchConfig() SearchConfig {
	return SearchConfig{
		QdrantURL:           "http://qdrant.ai.svc.cluster.local:6333",
		EmbeddingBaseURL:    "http://litellm.ai.svc.cluster.local:8000/v1",
		EmbeddingModel:      "bge-large-embeddings",
		EmbeddingDimensions: 1024,
		DefaultMaxResults:   10,
		DefaultMinScore:     0.5,
		Timeout:             30 * time.Second,
		EnableCache:         true,
		CacheTTL:            1 * time.Hour,
		CacheMaxSize:        1000,
	}
}

// WithEnv applies environment variable overrides.
func (c SearchConfig) WithEnv() SearchConfig {
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
	return c
}

// Validate validates the configuration.
func (c *SearchConfig) Validate() error {
	if c.QdrantURL == "" {
		return fmt.Errorf("qdrant_url is required")
	}
	if c.EmbeddingBaseURL == "" {
		return fmt.Errorf("embedding_base_url is required")
	}
	if c.EmbeddingModel == "" {
		return fmt.Errorf("embedding_model is required")
	}
	if c.EmbeddingDimensions <= 0 {
		return fmt.Errorf("embedding_dimensions must be positive")
	}
	return nil
}

// SearchOptions configures a specific search request.
type SearchOptions struct {
	// Vocabularies specifies which vocabularies to search.
	// If empty, defaults to LOINC.
	Vocabularies []index.Vocabulary

	// MaxResults limits the number of results.
	MaxResults int

	// MinScore sets the minimum similarity threshold.
	MinScore float64

	// IncludeMetadata controls whether to include full metadata.
	IncludeMetadata bool
}

// SemanticMatch represents a semantic search result.
type SemanticMatch struct {
	// Code is the terminology code.
	Code string `json:"code"`

	// Display is the display name.
	Display string `json:"display"`

	// System is the code system URI.
	System string `json:"system"`

	// Vocabulary is the vocabulary type.
	Vocabulary index.Vocabulary `json:"vocabulary"`

	// Score is the semantic similarity score (0-1).
	Score float64 `json:"score"`

	// MatchType indicates how the match was found.
	MatchType string `json:"match_type"`

	// Reason provides additional context about the match.
	Reason string `json:"reason,omitempty"`

	// Metadata contains additional searchable metadata.
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// IsHighConfidence returns true if score >= 0.9.
func (m *SemanticMatch) IsHighConfidence() bool {
	return m.Score >= 0.9
}

// IsMediumConfidence returns true if score >= 0.7.
func (m *SemanticMatch) IsMediumConfidence() bool {
	return m.Score >= 0.7
}

// IsAcceptable returns true if score >= threshold.
func (m *SemanticMatch) IsAcceptable(threshold float64) bool {
	return m.Score >= threshold
}

// VocabularyCodeSystemURL returns the full code system URL for a vocabulary.
func VocabularyCodeSystemURL(v index.Vocabulary) string {
	switch v {
	case index.VocabularyLOINC:
		return "http://loinc.org"
	case index.VocabularySNOMED:
		return "http://snomed.info/sct"
	case index.VocabularyICD10CM:
		return "http://hl7.org/fhir/sid/icd-10-cm"
	case index.VocabularyRxNorm:
		return "http://www.nlm.nih.gov/research/umls/rxnorm"
	case index.VocabularyCPT:
		return "http://www.ama-assn.org/go/cpt"
	case index.VocabularyCVX:
		return "http://hl7.org/fhir/sid/cvx"
	default:
		return ""
	}
}
