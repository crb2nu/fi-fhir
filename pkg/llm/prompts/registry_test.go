package prompts

import (
	"strings"
	"testing"
)

func TestDefault_LoadsEmbeddedTemplates(t *testing.T) {
	reg := Default()

	if reg.Size() == 0 {
		t.Fatal("expected default registry to have embedded prompts")
	}

	// Check known prompts are loaded
	for _, id := range []PromptID{
		RankingSystemV1,
		RankingUserV1,
		ExtractionSystemV1,
		ExtractionUserV1,
	} {
		p, err := reg.Get(id)
		if err != nil {
			t.Errorf("expected prompt %s to be loaded: %v", id, err)
			continue
		}
		if p.Meta.TaskType == "" {
			t.Errorf("prompt %s has empty TaskType", id)
		}
	}
}

func TestRegistry_Get_NotFound(t *testing.T) {
	reg := NewRegistry()
	_, err := reg.Get("nonexistent")
	if err == nil {
		t.Fatal("expected error for non-existent prompt")
	}
}

func TestRegistry_MustGet_Panics(t *testing.T) {
	reg := NewRegistry()
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected MustGet to panic for non-existent prompt")
		}
	}()
	reg.MustGet("nonexistent")
}

func TestRegistry_RegisterFromString(t *testing.T) {
	reg := NewRegistry()
	err := reg.RegisterFromString(
		Metadata{
			ID:       "test_prompt",
			Version:  1,
			TaskType: "test",
		},
		"Hello, {{.Name}}!",
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	p, err := reg.Get("test_prompt")
	if err != nil {
		t.Fatalf("expected prompt to be registered: %v", err)
	}

	result, err := p.Render(map[string]string{"Name": "World"})
	if err != nil {
		t.Fatalf("unexpected render error: %v", err)
	}
	if result != "Hello, World!" {
		t.Errorf("expected 'Hello, World!', got '%s'", result)
	}
}

func TestPrompt_Render_RankingSystem(t *testing.T) {
	reg := Default()
	p, err := reg.Get(RankingSystemV1)
	if err != nil {
		t.Fatalf("expected ranking_system_v1: %v", err)
	}

	result, err := p.Render(nil)
	if err != nil {
		t.Fatalf("unexpected render error: %v", err)
	}

	if !strings.Contains(result, "healthcare terminology expert") {
		t.Error("expected ranking system prompt to contain 'healthcare terminology expert'")
	}
	if !strings.Contains(result, "Confidence score guidelines") {
		t.Error("expected ranking system prompt to contain 'Confidence score guidelines'")
	}
}

func TestPrompt_HasSchema(t *testing.T) {
	reg := Default()

	// ranking_user_v1 should have a co-located schema
	p, err := reg.Get(RankingUserV1)
	if err != nil {
		t.Fatalf("expected ranking_user_v1: %v", err)
	}
	if !p.HasSchema() {
		t.Error("expected ranking_user_v1 to have a schema")
	}

	// ranking_system_v1 should NOT have a schema
	p, err = reg.Get(RankingSystemV1)
	if err != nil {
		t.Fatalf("expected ranking_system_v1: %v", err)
	}
	if p.HasSchema() {
		t.Error("expected ranking_system_v1 to NOT have a schema")
	}
}

func TestRegistry_ListByTaskType(t *testing.T) {
	reg := Default()

	ranking := reg.ListByTaskType("ranking")
	if len(ranking) < 2 {
		t.Errorf("expected at least 2 ranking prompts, got %d", len(ranking))
	}

	extraction := reg.ListByTaskType("extraction")
	if len(extraction) < 2 {
		t.Errorf("expected at least 2 extraction prompts, got %d", len(extraction))
	}
}

func TestRegistry_List(t *testing.T) {
	reg := Default()
	ids := reg.List()
	if len(ids) == 0 {
		t.Fatal("expected at least one prompt ID")
	}
}

func TestMetadataFromID(t *testing.T) {
	tests := []struct {
		id       PromptID
		taskType string
	}{
		{RankingSystemV1, "ranking"},
		{ExtractionSystemV1, "extraction"},
		{QualitySystemV1, "quality"},
		{ExplanationSystemV1, "explanation"},
		{"unknown_v1", ""},
	}

	for _, tt := range tests {
		meta := metadataFromID(tt.id)
		if meta.TaskType != tt.taskType {
			t.Errorf("metadataFromID(%s): expected TaskType=%q, got %q", tt.id, tt.taskType, meta.TaskType)
		}
	}
}

func TestPrompt_Render_ExtractionSystem(t *testing.T) {
	reg := Default()
	p, err := reg.Get(ExtractionSystemV1)
	if err != nil {
		t.Fatalf("expected extraction_system_v1: %v", err)
	}

	result, err := p.Render(nil)
	if err != nil {
		t.Fatalf("unexpected render error: %v", err)
	}

	if !strings.Contains(result, "clinical entity extraction") {
		t.Error("expected extraction system prompt to contain 'clinical entity extraction'")
	}
}

func TestRegistry_RegisterOverwrite(t *testing.T) {
	reg := NewRegistry()

	_ = reg.RegisterFromString(
		Metadata{ID: "test", Version: 1},
		"version 1",
		nil,
	)
	_ = reg.RegisterFromString(
		Metadata{ID: "test", Version: 2},
		"version 2",
		nil,
	)

	p, _ := reg.Get("test")
	if p.Meta.Version != 2 {
		t.Errorf("expected version 2, got %d", p.Meta.Version)
	}

	result, _ := p.Render(nil)
	if result != "version 2" {
		t.Errorf("expected 'version 2', got '%s'", result)
	}
}
