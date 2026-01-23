// Package index provides embedding-based terminology indexing for semantic search.
package index

import (
	"time"
)

// Vocabulary represents a supported terminology vocabulary.
type Vocabulary string

const (
	VocabularyLOINC   Vocabulary = "loinc"
	VocabularySNOMED  Vocabulary = "snomedct"
	VocabularyICD10CM Vocabulary = "icd10cm"
	VocabularyRxNorm  Vocabulary = "rxnorm"
	VocabularyCPT     Vocabulary = "cpt"
	VocabularyCVX     Vocabulary = "cvx"
)

// CollectionName returns the Qdrant collection name for a vocabulary.
func (v Vocabulary) CollectionName() string {
	return "fi_fhir_idx_" + string(v)
}

// String returns the string representation.
func (v Vocabulary) String() string {
	return string(v)
}

// IndexConfig configures the terminology index.
type IndexConfig struct {
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

	// BatchSize is the number of items to embed per batch.
	BatchSize int `yaml:"batch_size" json:"batch_size"`

	// Timeout is the request timeout.
	Timeout time.Duration `yaml:"timeout" json:"timeout"`
}

// DefaultIndexConfig returns a configuration with sensible defaults.
func DefaultIndexConfig() IndexConfig {
	return IndexConfig{
		QdrantURL:           "http://qdrant.ai.svc.cluster.local:6333",
		EmbeddingBaseURL:    "http://litellm.ai.svc.cluster.local:8000/v1",
		EmbeddingModel:      "bge-large-embeddings",
		EmbeddingDimensions: 1024,
		BatchSize:           32,
		Timeout:             60 * time.Second,
	}
}

// IndexEntry represents a single indexed terminology entry.
type IndexEntry struct {
	// ID is the unique identifier (typically vocabulary:code).
	ID string `json:"id"`

	// Code is the terminology code.
	Code string `json:"code"`

	// System is the code system URI.
	System string `json:"system"`

	// Display is the display name.
	Display string `json:"display"`

	// Vocabulary is the vocabulary type.
	Vocabulary Vocabulary `json:"vocabulary"`

	// EmbeddingText is the text used for embedding.
	EmbeddingText string `json:"embedding_text"`

	// Metadata contains additional searchable metadata.
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// SearchResult represents a semantic search result.
type SearchResult struct {
	// Entry is the matched index entry.
	Entry IndexEntry `json:"entry"`

	// Score is the similarity score (0-1).
	Score float64 `json:"score"`

	// Distance is the vector distance.
	Distance float64 `json:"distance,omitempty"`
}

// IndexStats contains statistics about an index.
type IndexStats struct {
	// Vocabulary is the vocabulary type.
	Vocabulary Vocabulary `json:"vocabulary"`

	// Collection is the Qdrant collection name.
	Collection string `json:"collection"`

	// TotalEntries is the number of indexed entries.
	TotalEntries int64 `json:"total_entries"`

	// IndexedAt is when the index was last built.
	IndexedAt time.Time `json:"indexed_at,omitempty"`

	// Version is the vocabulary version.
	Version string `json:"version,omitempty"`

	// EmbeddingModel is the model used for embeddings.
	EmbeddingModel string `json:"embedding_model"`

	// Dimensions is the embedding dimensionality.
	Dimensions int `json:"dimensions"`
}

// BuildProgress tracks index building progress.
type BuildProgress struct {
	// Vocabulary is the vocabulary being indexed.
	Vocabulary Vocabulary `json:"vocabulary"`

	// TotalItems is the total number of items to index.
	TotalItems int `json:"total_items"`

	// ProcessedItems is the number of items processed.
	ProcessedItems int `json:"processed_items"`

	// EmbeddedItems is the number of items embedded.
	EmbeddedItems int `json:"embedded_items"`

	// IndexedItems is the number of items indexed in Qdrant.
	IndexedItems int `json:"indexed_items"`

	// StartedAt is when building started.
	StartedAt time.Time `json:"started_at"`

	// UpdatedAt is the last progress update time.
	UpdatedAt time.Time `json:"updated_at"`

	// Errors contains any errors encountered.
	Errors []string `json:"errors,omitempty"`

	// Status is the current status.
	Status BuildStatus `json:"status"`
}

// BuildStatus represents the status of an index build.
type BuildStatus string

const (
	BuildStatusPending   BuildStatus = "pending"
	BuildStatusRunning   BuildStatus = "running"
	BuildStatusCompleted BuildStatus = "completed"
	BuildStatusFailed    BuildStatus = "failed"
	BuildStatusCanceled  BuildStatus = "canceled"
)

// PercentComplete returns the completion percentage.
func (p *BuildProgress) PercentComplete() float64 {
	if p.TotalItems == 0 {
		return 0
	}
	return float64(p.IndexedItems) / float64(p.TotalItems) * 100
}

// IsComplete returns true if the build is complete.
func (p *BuildProgress) IsComplete() bool {
	return p.Status == BuildStatusCompleted
}

// IsFailed returns true if the build failed.
func (p *BuildProgress) IsFailed() bool {
	return p.Status == BuildStatusFailed
}

// LOINCEntry represents a LOINC entry for indexing.
type LOINCEntry struct {
	Code         string `json:"code"`
	Component    string `json:"component"`
	Property     string `json:"property"`
	TimeAspect   string `json:"time_aspect"`
	System       string `json:"system"`
	Scale        string `json:"scale"`
	Method       string `json:"method,omitempty"`
	ShortName    string `json:"short_name,omitempty"`
	LongName     string `json:"long_name,omitempty"`
	Consumer     string `json:"consumer,omitempty"`
	RelatedNames string `json:"related_names,omitempty"`
	Status       string `json:"status"`
}

// ToIndexEntry converts a LOINC entry to an IndexEntry.
func (e *LOINCEntry) ToIndexEntry() IndexEntry {
	// Build embedding text: combine multiple fields for rich semantic representation
	var parts []string
	if e.ShortName != "" {
		parts = append(parts, e.ShortName)
	}
	if e.LongName != "" {
		parts = append(parts, e.LongName)
	}
	if e.Component != "" {
		parts = append(parts, e.Component)
	}
	if e.Consumer != "" {
		parts = append(parts, e.Consumer)
	}
	if e.RelatedNames != "" {
		parts = append(parts, "Synonyms: "+e.RelatedNames)
	}

	embeddingText := ""
	for i, p := range parts {
		if i > 0 {
			embeddingText += " | "
		}
		embeddingText += p
	}

	return IndexEntry{
		ID:            "loinc:" + e.Code,
		Code:          e.Code,
		System:        "http://loinc.org",
		Display:       e.DisplayName(),
		Vocabulary:    VocabularyLOINC,
		EmbeddingText: embeddingText,
		Metadata: map[string]interface{}{
			"component":   e.Component,
			"property":    e.Property,
			"time_aspect": e.TimeAspect,
			"system":      e.System,
			"scale":       e.Scale,
			"method":      e.Method,
			"short_name":  e.ShortName,
			"long_name":   e.LongName,
			"status":      e.Status,
		},
	}
}

// DisplayName returns the best display name for a LOINC entry.
func (e *LOINCEntry) DisplayName() string {
	if e.Consumer != "" {
		return e.Consumer
	}
	if e.ShortName != "" {
		return e.ShortName
	}
	if e.LongName != "" {
		return e.LongName
	}
	return e.Component
}

// SNOMEDEntry represents a SNOMED CT entry for indexing.
type SNOMEDEntry struct {
	ConceptID   string `json:"concept_id"`
	FSN         string `json:"fsn"`         // Fully Specified Name
	Description string `json:"description"` // Preferred Term
	Synonyms    string `json:"synonyms,omitempty"`
	Semantic    string `json:"semantic,omitempty"` // Semantic tag
	Active      bool   `json:"active"`
}

// ToIndexEntry converts a SNOMED entry to an IndexEntry.
func (e *SNOMEDEntry) ToIndexEntry() IndexEntry {
	embeddingText := e.Description
	if e.FSN != "" && e.FSN != e.Description {
		embeddingText += " | " + e.FSN
	}
	if e.Synonyms != "" {
		embeddingText += " | Synonyms: " + e.Synonyms
	}

	return IndexEntry{
		ID:            "snomedct:" + e.ConceptID,
		Code:          e.ConceptID,
		System:        "http://snomed.info/sct",
		Display:       e.Description,
		Vocabulary:    VocabularySNOMED,
		EmbeddingText: embeddingText,
		Metadata: map[string]interface{}{
			"fsn":      e.FSN,
			"semantic": e.Semantic,
			"active":   e.Active,
		},
	}
}

// ICD10Entry represents an ICD-10-CM entry for indexing.
type ICD10Entry struct {
	Code        string `json:"code"`
	Description string `json:"description"`
	LongDesc    string `json:"long_desc,omitempty"`
	Category    string `json:"category,omitempty"`
	Valid       bool   `json:"valid"` // Whether billable
}

// ToIndexEntry converts an ICD-10 entry to an IndexEntry.
func (e *ICD10Entry) ToIndexEntry() IndexEntry {
	embeddingText := e.Description
	if e.LongDesc != "" && e.LongDesc != e.Description {
		embeddingText += " | " + e.LongDesc
	}

	return IndexEntry{
		ID:            "icd10cm:" + e.Code,
		Code:          e.Code,
		System:        "http://hl7.org/fhir/sid/icd-10-cm",
		Display:       e.Description,
		Vocabulary:    VocabularyICD10CM,
		EmbeddingText: embeddingText,
		Metadata: map[string]interface{}{
			"long_desc": e.LongDesc,
			"category":  e.Category,
			"valid":     e.Valid,
		},
	}
}
