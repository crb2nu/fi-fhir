// Package prompts provides a versioned prompt template registry for LLM operations.
//
// Prompts are stored as embedded Go template files and loaded at init time.
// Each prompt has a version, recommended model tier, and optional JSON schema
// for structured output.
package prompts

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"text/template"
	"time"
)

//go:embed templates/*.tmpl
//go:embed templates/*.json
var templateFS embed.FS

// templateFuncs provides utility functions available in all prompt templates.
var templateFuncs = template.FuncMap{
	"add": func(a, b int) int { return a + b },
}

// PromptID uniquely identifies a prompt version (e.g. "ranking_system_v1").
type PromptID string

// Metadata describes a prompt version.
type Metadata struct {
	ID          PromptID  `json:"id"`
	Version     int       `json:"version"`
	TaskType    string    `json:"task_type"` // "ranking", "extraction", "explanation", "quality"
	Description string    `json:"description"`
	ModelTier   string    `json:"model_tier"` // "fast", "quality"
	MaxTokens   int       `json:"max_tokens,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// Prompt is a compiled, versioned prompt template with optional schema.
type Prompt struct {
	Meta     Metadata
	template *template.Template
	Schema   json.RawMessage // JSON schema for structured output, if any
}

// Render executes the template with the given data and returns the result.
func (p *Prompt) Render(data interface{}) (string, error) {
	var buf bytes.Buffer
	if err := p.template.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("render prompt %s: %w", p.Meta.ID, err)
	}
	return buf.String(), nil
}

// HasSchema returns true if this prompt has an associated JSON schema.
func (p *Prompt) HasSchema() bool {
	return len(p.Schema) > 0
}

// Registry manages versioned prompt templates.
type Registry struct {
	mu      sync.RWMutex
	prompts map[PromptID]*Prompt
}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry {
	return &Registry{
		prompts: make(map[PromptID]*Prompt),
	}
}

// Register adds a prompt to the registry.
// If a prompt with the same ID already exists, it is overwritten.
func (r *Registry) Register(p *Prompt) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.prompts[p.Meta.ID] = p
}

// Get retrieves a prompt by ID.
func (r *Registry) Get(id PromptID) (*Prompt, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.prompts[id]
	if !ok {
		return nil, fmt.Errorf("prompt not found: %s", id)
	}
	return p, nil
}

// MustGet retrieves a prompt by ID or panics.
func (r *Registry) MustGet(id PromptID) *Prompt {
	p, err := r.Get(id)
	if err != nil {
		panic(err)
	}
	return p
}

// List returns all registered prompt IDs.
func (r *Registry) List() []PromptID {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := make([]PromptID, 0, len(r.prompts))
	for id := range r.prompts {
		ids = append(ids, id)
	}
	return ids
}

// ListByTaskType returns all prompts for a given task type.
func (r *Registry) ListByTaskType(taskType string) []*Prompt {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []*Prompt
	for _, p := range r.prompts {
		if p.Meta.TaskType == taskType {
			result = append(result, p)
		}
	}
	return result
}

// Size returns the number of registered prompts.
func (r *Registry) Size() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.prompts)
}

// RegisterFromString creates and registers a prompt from a raw template string.
func (r *Registry) RegisterFromString(meta Metadata, tmplStr string, schema json.RawMessage) error {
	t, err := template.New(string(meta.ID)).Funcs(templateFuncs).Parse(tmplStr)
	if err != nil {
		return fmt.Errorf("parse template %s: %w", meta.ID, err)
	}
	r.Register(&Prompt{
		Meta:     meta,
		template: t,
		Schema:   schema,
	})
	return nil
}

// Well-known prompt IDs.
const (
	RankingSystemV1     PromptID = "ranking_system_v1"
	RankingUserV1       PromptID = "ranking_user_v1"
	ExtractionSystemV1  PromptID = "extraction_system_v1"
	ExtractionUserV1    PromptID = "extraction_user_v1"
	QualitySystemV1     PromptID = "quality_system_v1"
	ExplanationSystemV1 PromptID = "explanation_system_v1"
)

// Default returns a registry pre-loaded with all embedded prompt templates.
func Default() *Registry {
	r := NewRegistry()
	loadEmbeddedPrompts(r)
	return r
}

// loadEmbeddedPrompts loads all .tmpl files from the embedded filesystem.
func loadEmbeddedPrompts(r *Registry) {
	entries, err := templateFS.ReadDir("templates")
	if err != nil {
		// If templates dir doesn't exist yet, return empty registry
		return
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		content, err := templateFS.ReadFile("templates/" + name)
		if err != nil {
			continue
		}

		// Only process .tmpl files
		if !strings.HasSuffix(name, ".tmpl") {
			continue
		}

		// Derive prompt ID from filename (strip .tmpl extension)
		id := PromptID(name[:len(name)-len(".tmpl")])
		meta := metadataFromID(id)

		t, err := template.New(string(id)).Funcs(templateFuncs).Parse(string(content))
		if err != nil {
			continue
		}

		// Check for co-located schema file
		schemaName := "templates/" + name[:len(name)-len(".tmpl")] + "_schema.json"
		var schema json.RawMessage
		if schemaData, err := templateFS.ReadFile(schemaName); err == nil {
			schema = json.RawMessage(schemaData)
		}

		r.Register(&Prompt{
			Meta:     meta,
			template: t,
			Schema:   schema,
		})
	}
}

// metadataFromID infers metadata from a prompt ID by convention.
func metadataFromID(id PromptID) Metadata {
	meta := Metadata{
		ID:        id,
		Version:   1,
		CreatedAt: time.Date(2026, 3, 6, 0, 0, 0, 0, time.UTC),
	}

	s := string(id)
	switch {
	case len(s) >= 7 && s[:7] == "ranking":
		meta.TaskType = "ranking"
		meta.ModelTier = "fast"
		meta.Description = "Terminology mapping ranking prompt"
		meta.MaxTokens = 1024
	case len(s) >= 10 && s[:10] == "extraction":
		meta.TaskType = "extraction"
		meta.ModelTier = "quality"
		meta.Description = "Clinical entity extraction prompt"
		meta.MaxTokens = 4096
	case len(s) >= 7 && s[:7] == "quality":
		meta.TaskType = "quality"
		meta.ModelTier = "quality"
		meta.Description = "Data quality analysis prompt"
		meta.MaxTokens = 2048
	case len(s) >= 11 && s[:11] == "explanation":
		meta.TaskType = "explanation"
		meta.ModelTier = "fast"
		meta.Description = "Natural language explanation prompt"
		meta.MaxTokens = 2048
	}

	return meta
}
