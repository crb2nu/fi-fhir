package index

import (
	"testing"
)

func TestVocabulary_CollectionName(t *testing.T) {
	tests := []struct {
		v    Vocabulary
		want string
	}{
		{VocabularyLOINC, "fi_fhir_idx_loinc"},
		{VocabularySNOMED, "fi_fhir_idx_snomedct"},
		{VocabularyICD10CM, "fi_fhir_idx_icd10cm"},
		{VocabularyRxNorm, "fi_fhir_idx_rxnorm"},
		{VocabularyCPT, "fi_fhir_idx_cpt"},
		{VocabularyCVX, "fi_fhir_idx_cvx"},
	}
	for _, tc := range tests {
		if got := tc.v.CollectionName(); got != tc.want {
			t.Errorf("Vocabulary(%q).CollectionName() = %q, want %q", tc.v, got, tc.want)
		}
	}
}

func TestVocabulary_String(t *testing.T) {
	if got := VocabularyLOINC.String(); got != "loinc" {
		t.Errorf("String() = %q, want loinc", got)
	}
}

func TestDefaultIndexConfig(t *testing.T) {
	cfg := DefaultIndexConfig()
	if cfg.QdrantURL == "" {
		t.Error("expected non-empty QdrantURL")
	}
	if cfg.EmbeddingDimensions != 1024 {
		t.Errorf("EmbeddingDimensions=%d want 1024", cfg.EmbeddingDimensions)
	}
	if cfg.BatchSize != 32 {
		t.Errorf("BatchSize=%d want 32", cfg.BatchSize)
	}
	if cfg.Timeout == 0 {
		t.Error("expected non-zero Timeout")
	}
}

func TestBuildProgress_PercentComplete(t *testing.T) {
	t.Run("zero total", func(t *testing.T) {
		p := &BuildProgress{TotalItems: 0}
		if got := p.PercentComplete(); got != 0 {
			t.Errorf("PercentComplete()=%f want 0", got)
		}
	})
	t.Run("half done", func(t *testing.T) {
		p := &BuildProgress{TotalItems: 100, IndexedItems: 50}
		if got := p.PercentComplete(); got != 50.0 {
			t.Errorf("PercentComplete()=%f want 50", got)
		}
	})
	t.Run("complete", func(t *testing.T) {
		p := &BuildProgress{TotalItems: 200, IndexedItems: 200}
		if got := p.PercentComplete(); got != 100.0 {
			t.Errorf("PercentComplete()=%f want 100", got)
		}
	})
}

func TestBuildProgress_IsComplete(t *testing.T) {
	if (&BuildProgress{Status: BuildStatusCompleted}).IsComplete() != true {
		t.Error("IsComplete should be true for completed status")
	}
	if (&BuildProgress{Status: BuildStatusRunning}).IsComplete() != false {
		t.Error("IsComplete should be false for running status")
	}
}

func TestBuildProgress_IsFailed(t *testing.T) {
	if (&BuildProgress{Status: BuildStatusFailed}).IsFailed() != true {
		t.Error("IsFailed should be true for failed status")
	}
	if (&BuildProgress{Status: BuildStatusCompleted}).IsFailed() != false {
		t.Error("IsFailed should be false for completed status")
	}
}

func TestLOINCEntry_DisplayName(t *testing.T) {
	tests := []struct {
		name  string
		entry LOINCEntry
		want  string
	}{
		{"consumer preferred", LOINCEntry{Consumer: "Glucose", ShortName: "Gluc", LongName: "Glucose [Mass]", Component: "Glucose"}, "Glucose"},
		{"short name", LOINCEntry{ShortName: "Gluc", LongName: "Glucose [Mass]", Component: "Glucose"}, "Gluc"},
		{"long name", LOINCEntry{LongName: "Glucose [Mass]", Component: "Glucose"}, "Glucose [Mass]"},
		{"component fallback", LOINCEntry{Component: "Glucose"}, "Glucose"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.entry.DisplayName(); got != tc.want {
				t.Errorf("DisplayName()=%q want %q", got, tc.want)
			}
		})
	}
}

func TestLOINCEntry_ToIndexEntry(t *testing.T) {
	entry := LOINCEntry{
		Code:         "2345-7",
		Component:    "Glucose",
		Property:     "MCnc",
		TimeAspect:   "Pt",
		System:       "Ser/Plas",
		Scale:        "Qn",
		Method:       "Enzymatic",
		ShortName:    "Glucose SerPl-mCnc",
		LongName:     "Glucose [Mass/volume] in Serum or Plasma",
		Consumer:     "Glucose, Blood",
		RelatedNames: "Blood sugar;Gluc",
		Status:       "ACTIVE",
	}

	idx := entry.ToIndexEntry()

	if idx.ID != "loinc:2345-7" {
		t.Errorf("ID=%q", idx.ID)
	}
	if idx.Code != "2345-7" {
		t.Errorf("Code=%q", idx.Code)
	}
	if idx.System != "http://loinc.org" {
		t.Errorf("System=%q", idx.System)
	}
	if idx.Vocabulary != VocabularyLOINC {
		t.Errorf("Vocabulary=%q", idx.Vocabulary)
	}
	if idx.Display != "Glucose, Blood" {
		t.Errorf("Display=%q want 'Glucose, Blood'", idx.Display)
	}
	// EmbeddingText should contain all parts
	if idx.EmbeddingText == "" {
		t.Error("expected non-empty EmbeddingText")
	}
	if idx.Metadata == nil {
		t.Error("expected non-nil Metadata")
	}
}

func TestSNOMEDEntry_ToIndexEntry(t *testing.T) {
	t.Run("with synonyms", func(t *testing.T) {
		entry := SNOMEDEntry{
			ConceptID:   "387517004",
			FSN:         "Paracetamol (substance)",
			Description: "Paracetamol",
			Synonyms:    "Acetaminophen;Tylenol",
			Semantic:    "substance",
			Active:      true,
		}
		idx := entry.ToIndexEntry()
		if idx.ID != "snomedct:387517004" {
			t.Errorf("ID=%q", idx.ID)
		}
		if idx.System != "http://snomed.info/sct" {
			t.Errorf("System=%q", idx.System)
		}
		if idx.Vocabulary != VocabularySNOMED {
			t.Errorf("Vocabulary=%q", idx.Vocabulary)
		}
	})

	t.Run("without synonyms", func(t *testing.T) {
		entry := SNOMEDEntry{
			ConceptID:   "12345",
			FSN:         "Test (finding)",
			Description: "Test",
			Active:      true,
		}
		idx := entry.ToIndexEntry()
		if idx.EmbeddingText == "" {
			t.Error("expected non-empty EmbeddingText")
		}
	})

	t.Run("same FSN and description", func(t *testing.T) {
		entry := SNOMEDEntry{
			ConceptID:   "99999",
			FSN:         "Same",
			Description: "Same",
			Active:      true,
		}
		idx := entry.ToIndexEntry()
		// Should not duplicate "Same | Same"
		if idx.EmbeddingText != "Same" {
			t.Errorf("EmbeddingText=%q want 'Same'", idx.EmbeddingText)
		}
	})
}

func TestICD10Entry_ToIndexEntry(t *testing.T) {
	entry := ICD10Entry{
		Code:        "E11.9",
		Description: "Type 2 diabetes mellitus without complications",
		LongDesc:    "Type 2 diabetes mellitus without complications, unspecified",
		Category:    "E11",
		Valid:       true,
	}
	idx := entry.ToIndexEntry()
	if idx.ID != "icd10cm:E11.9" {
		t.Errorf("ID=%q", idx.ID)
	}
	if idx.System != "http://hl7.org/fhir/sid/icd-10-cm" {
		t.Errorf("System=%q", idx.System)
	}
	if idx.Vocabulary != VocabularyICD10CM {
		t.Errorf("Vocabulary=%q", idx.Vocabulary)
	}
	// EmbeddingText should include both descriptions
	if idx.EmbeddingText == entry.Description {
		t.Error("expected EmbeddingText to include LongDesc")
	}
}

func TestGetString(t *testing.T) {
	payload := map[string]interface{}{
		"key1": "value1",
		"key2": 42,
	}
	if got := getString(payload, "key1"); got != "value1" {
		t.Errorf("getString(key1)=%q", got)
	}
	if got := getString(payload, "key2"); got != "" {
		t.Errorf("getString(key2)=%q, want empty (not a string)", got)
	}
	if got := getString(payload, "missing"); got != "" {
		t.Errorf("getString(missing)=%q", got)
	}
	if got := getString(nil, "key"); got != "" {
		t.Errorf("getString(nil, key)=%q", got)
	}
}
