package autoroute

import (
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/llm"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/terminology/index"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/terminology/semantic"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.HighConfidenceThreshold != 0.90 {
		t.Errorf("HighConfidenceThreshold = %v, want 0.90", cfg.HighConfidenceThreshold)
	}
	if cfg.MedConfidenceThreshold != 0.70 {
		t.Errorf("MedConfidenceThreshold = %v, want 0.70", cfg.MedConfidenceThreshold)
	}
	if cfg.SemanticTopK != 10 {
		t.Errorf("SemanticTopK = %v, want 10", cfg.SemanticTopK)
	}
	if cfg.SemanticMinScore != 0.5 {
		t.Errorf("SemanticMinScore = %v, want 0.5", cfg.SemanticMinScore)
	}
	if cfg.SemanticAutoAcceptThreshold != 0.95 {
		t.Errorf("SemanticAutoAcceptThreshold = %v, want 0.95", cfg.SemanticAutoAcceptThreshold)
	}
	if cfg.CacheTTL != 15*time.Minute {
		t.Errorf("CacheTTL = %v, want 15m", cfg.CacheTTL)
	}
	if cfg.CacheEnabled != true {
		t.Errorf("CacheEnabled = %v, want true", cfg.CacheEnabled)
	}
}

func TestVocabularyFromSystem(t *testing.T) {
	tests := []struct {
		system string
		want   index.Vocabulary
	}{
		{
			system: "http://loinc.org",
			want:   index.VocabularyLOINC,
		},
		{
			system: "http://snomed.info/sct",
			want:   index.VocabularySNOMED,
		},
		{
			system: "http://hl7.org/fhir/sid/icd-10-cm",
			want:   index.VocabularyICD10CM,
		},
		{
			system: "http://www.nlm.nih.gov/research/umls/rxnorm",
			want:   index.VocabularyRxNorm,
		},
		{
			system: "http://www.ama-assn.org/go/cpt",
			want:   index.VocabularyCPT,
		},
		{
			system: "http://hl7.org/fhir/sid/cvx",
			want:   index.VocabularyCVX,
		},
		{
			system: "custom_system",
			want:   index.Vocabulary("custom_system"),
		},
		{
			system: "",
			want:   index.Vocabulary(""),
		},
	}

	for _, tt := range tests {
		t.Run(tt.system, func(t *testing.T) {
			got := vocabularyFromSystem(tt.system)
			if got != tt.want {
				t.Errorf("vocabularyFromSystem(%q) = %v, want %v", tt.system, got, tt.want)
			}
		})
	}
}

func TestNewEngineAppliesDefaults(t *testing.T) {
	// Pass a config with zero values - should apply defaults
	cfg := Config{
		CacheEnabled: false, // Explicitly disable cache for easier testing
	}

	// NewEngine requires a searcher and llmClient, but we can pass nil
	// since we're just testing config defaults (we won't call Suggest)
	// In a real scenario, we'd need mocks
	engine := NewEngine(nil, nil, cfg)

	if engine.config.HighConfidenceThreshold != 0.90 {
		t.Errorf("HighConfidenceThreshold = %v, want 0.90", engine.config.HighConfidenceThreshold)
	}
	if engine.config.MedConfidenceThreshold != 0.70 {
		t.Errorf("MedConfidenceThreshold = %v, want 0.70", engine.config.MedConfidenceThreshold)
	}
	if engine.config.SemanticTopK != 10 {
		t.Errorf("SemanticTopK = %v, want 10", engine.config.SemanticTopK)
	}
	if engine.config.SemanticMinScore != 0.5 {
		t.Errorf("SemanticMinScore = %v, want 0.5", engine.config.SemanticMinScore)
	}
	if engine.config.SemanticAutoAcceptThreshold != 0.95 {
		t.Errorf("SemanticAutoAcceptThreshold = %v, want 0.95", engine.config.SemanticAutoAcceptThreshold)
	}
	if engine.config.CacheTTL != 15*time.Minute {
		t.Errorf("CacheTTL = %v, want 15m", engine.config.CacheTTL)
	}
}

func TestNewEnginePreservesCustomConfig(t *testing.T) {
	cfg := Config{
		HighConfidenceThreshold:     0.85,
		MedConfidenceThreshold:      0.65,
		SemanticTopK:                20,
		SemanticMinScore:            0.6,
		SemanticAutoAcceptThreshold: 0.98,
		CacheTTL:                    30 * time.Minute,
		CacheEnabled:                false,
		LLMModel:                    "custom-model",
	}

	engine := NewEngine(nil, nil, cfg)

	if engine.config.HighConfidenceThreshold != 0.85 {
		t.Errorf("HighConfidenceThreshold = %v, want 0.85", engine.config.HighConfidenceThreshold)
	}
	if engine.config.MedConfidenceThreshold != 0.65 {
		t.Errorf("MedConfidenceThreshold = %v, want 0.65", engine.config.MedConfidenceThreshold)
	}
	if engine.config.SemanticTopK != 20 {
		t.Errorf("SemanticTopK = %v, want 20", engine.config.SemanticTopK)
	}
	if engine.config.SemanticMinScore != 0.6 {
		t.Errorf("SemanticMinScore = %v, want 0.6", engine.config.SemanticMinScore)
	}
	if engine.config.SemanticAutoAcceptThreshold != 0.98 {
		t.Errorf("SemanticAutoAcceptThreshold = %v, want 0.98", engine.config.SemanticAutoAcceptThreshold)
	}
	if engine.config.CacheTTL != 30*time.Minute {
		t.Errorf("CacheTTL = %v, want 30m", engine.config.CacheTTL)
	}
	if engine.config.LLMModel != "custom-model" {
		t.Errorf("LLMModel = %v, want custom-model", engine.config.LLMModel)
	}
}

func TestEngineCacheOperations(t *testing.T) {
	engine := NewEngine(nil, nil, Config{
		CacheEnabled: true,
		CacheTTL:     1 * time.Hour,
	})

	// Initially empty
	if engine.CacheSize() != 0 {
		t.Errorf("initial CacheSize = %d, want 0", engine.CacheSize())
	}

	// Manually populate cache through direct access (for testing)
	req := SuggestRequest{
		SourceCode:   "TEST",
		SourceSystem: "test",
		TargetSystem: "http://loinc.org",
	}
	engine.cache.Set(req, &SuggestResult{Confidence: 0.9})

	if engine.CacheSize() != 1 {
		t.Errorf("CacheSize after set = %d, want 1", engine.CacheSize())
	}

	// Invalidate
	engine.InvalidateCache(req)
	if engine.CacheSize() != 0 {
		t.Errorf("CacheSize after invalidate = %d, want 0", engine.CacheSize())
	}

	// Add and clear
	engine.cache.Set(req, &SuggestResult{Confidence: 0.9})
	engine.ClearCache()
	if engine.CacheSize() != 0 {
		t.Errorf("CacheSize after clear = %d, want 0", engine.CacheSize())
	}
}

func TestEngineCacheDisabled(t *testing.T) {
	engine := NewEngine(nil, nil, Config{
		CacheEnabled: false,
	})

	// Cache should be nil
	if engine.cache != nil {
		t.Error("expected nil cache when disabled")
	}

	// Operations should not panic
	engine.InvalidateCache(SuggestRequest{})
	engine.ClearCache()

	if engine.CacheSize() != 0 {
		t.Error("CacheSize should return 0 when cache disabled")
	}
}

func TestNewEngineWithRouter(t *testing.T) {
	// Create a mock client to use as the fast tier
	mockClient := &mockLLMClient{}
	router := llm.NewRouter(mockClient, nil, llm.DefaultRouterConfig())

	cfg := Config{
		CacheEnabled: false,
		LLMModel:     "router-model",
	}

	engine := NewEngineWithRouter(nil, router, cfg)

	if engine.router != router {
		t.Error("expected router to be set")
	}
	if engine.ranker == nil {
		t.Error("expected ranker to be created")
	}
	// Defaults should be applied
	if engine.config.HighConfidenceThreshold != 0.90 {
		t.Errorf("HighConfidenceThreshold = %v, want 0.90", engine.config.HighConfidenceThreshold)
	}
}

func TestBuildResultFromSemantic(t *testing.T) {
	engine := NewEngine(nil, nil, Config{CacheEnabled: false})

	matches := []semantic.SemanticMatch{
		{Code: "2345-7", Display: "Glucose [Mass/volume]", System: "http://loinc.org", Score: 0.92},
		{Code: "2339-0", Display: "Glucose in Blood", System: "http://loinc.org", Score: 0.85},
		{Code: "74774-1", Display: "Glucose panel", System: "http://loinc.org", Score: 0.70},
	}

	req := SuggestRequest{
		SourceCode:    "GLU",
		SourceSystem:  "test",
		TargetSystem:  "http://loinc.org",
		MaxCandidates: 3,
	}

	trace := &DecisionTrace{Steps: make([]DecisionStep, 0)}
	start := time.Now()
	searchDuration := 50 * time.Millisecond

	result := engine.buildResultFromSemantic(matches, req, searchDuration, trace, start)

	// Best match
	if result.BestMatch == nil {
		t.Fatal("expected best match")
	}
	if result.BestMatch.Code != "2345-7" {
		t.Errorf("best match code = %q, want 2345-7", result.BestMatch.Code)
	}
	// Confidence should be score * 0.9
	expectedConf := 0.92 * 0.9
	if diff := result.Confidence - expectedConf; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("confidence = %v, want ~%v", result.Confidence, expectedConf)
	}

	// Alternates
	if len(result.Alternates) != 2 {
		t.Fatalf("expected 2 alternates, got %d", len(result.Alternates))
	}
	if result.Alternates[0].Code != "2339-0" {
		t.Errorf("first alternate = %q, want 2339-0", result.Alternates[0].Code)
	}

	// Trace result
	if trace.Result == nil {
		t.Fatal("expected trace result")
	}
	if trace.Result.Code != "2345-7" {
		t.Errorf("trace result code = %q, want 2345-7", trace.Result.Code)
	}

	// Timing
	if result.SearchDuration != searchDuration {
		t.Errorf("search duration = %v, want %v", result.SearchDuration, searchDuration)
	}
}

func TestBuildResultFromSemanticEmpty(t *testing.T) {
	engine := NewEngine(nil, nil, Config{CacheEnabled: false})

	trace := &DecisionTrace{Steps: make([]DecisionStep, 0)}
	start := time.Now()

	result := engine.buildResultFromSemantic(nil, SuggestRequest{MaxCandidates: 5}, 0, trace, start)

	if result.BestMatch != nil {
		t.Error("expected nil best match for empty input")
	}
	if result.Confidence != 0 {
		t.Errorf("confidence = %v, want 0", result.Confidence)
	}
	if len(result.Alternates) != 0 {
		t.Errorf("expected no alternates, got %d", len(result.Alternates))
	}
}
