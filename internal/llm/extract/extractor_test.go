package extract

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/llm"
)

func TestNewExtractor(t *testing.T) {
	t.Run("returns error when client is nil", func(t *testing.T) {
		_, err := NewExtractor(Config{Client: nil})
		if err == nil {
			t.Fatal("expected error when client is nil")
		}
	})

	t.Run("creates extractor with valid config", func(t *testing.T) {
		client := llm.NewMockClient()
		e, err := NewExtractor(Config{Client: client})
		if err != nil {
			t.Fatalf("NewExtractor error: %v", err)
		}
		if e == nil {
			t.Fatal("expected non-nil extractor")
		}
	})

	t.Run("uses quality model from client when model not specified", func(t *testing.T) {
		client := llm.NewMockClient()
		client.QualityModelValue = "test-quality-model"

		e, err := NewExtractor(Config{Client: client})
		if err != nil {
			t.Fatalf("NewExtractor error: %v", err)
		}
		if e.model != "test-quality-model" {
			t.Errorf("model = %v, want test-quality-model", e.model)
		}
	})

	t.Run("uses specified model over quality model", func(t *testing.T) {
		client := llm.NewMockClient()
		client.QualityModelValue = "quality-model"

		e, err := NewExtractor(Config{
			Client: client,
			Model:  "specified-model",
		})
		if err != nil {
			t.Fatalf("NewExtractor error: %v", err)
		}
		if e.model != "specified-model" {
			t.Errorf("model = %v, want specified-model", e.model)
		}
	})

	t.Run("creates cache when enabled", func(t *testing.T) {
		client := llm.NewMockClient()
		e, err := NewExtractor(Config{
			Client:      client,
			EnableCache: true,
			CacheTTL:    5 * time.Minute,
		})
		if err != nil {
			t.Fatalf("NewExtractor error: %v", err)
		}
		if e.cache == nil {
			t.Error("expected cache to be created")
		}
	})

	t.Run("uses default cache TTL when not specified", func(t *testing.T) {
		client := llm.NewMockClient()
		e, err := NewExtractor(Config{
			Client:      client,
			EnableCache: true,
		})
		if err != nil {
			t.Fatalf("NewExtractor error: %v", err)
		}
		if e.cache == nil {
			t.Error("expected cache to be created")
		}
		if e.cache.ttl != 1*time.Hour {
			t.Errorf("cache.ttl = %v, want 1h", e.cache.ttl)
		}
	})
}

func TestExtractor_Extract(t *testing.T) {
	ctx := context.Background()

	t.Run("returns empty result for empty text", func(t *testing.T) {
		client := llm.NewMockClient()
		e, _ := NewExtractor(Config{Client: client})

		result, err := e.Extract(ctx, "", ExtractionOptions{})
		if err != nil {
			t.Fatalf("Extract error: %v", err)
		}
		if result.Confidence != 0 {
			t.Errorf("Confidence = %v, want 0", result.Confidence)
		}
		if client.CallCount() != 0 {
			t.Error("expected no LLM calls for empty text")
		}
	})

	t.Run("extracts conditions successfully", func(t *testing.T) {
		client := llm.NewMockClient()
		client.WithJSONResponse(map[string]interface{}{
			"conditions": []map[string]interface{}{
				{
					"name":        "Type 2 Diabetes Mellitus",
					"code":        "E11.9",
					"code_system": "ICD-10-CM",
					"status":      "active",
					"confidence":  0.95,
				},
			},
			"overall_confidence": 0.92,
		})

		e, _ := NewExtractor(Config{Client: client})

		result, err := e.Extract(ctx, "Patient has type 2 diabetes.", ExtractionOptions{
			ExtractConditions: true,
		})
		if err != nil {
			t.Fatalf("Extract error: %v", err)
		}

		if len(result.Conditions) != 1 {
			t.Fatalf("len(Conditions) = %d, want 1", len(result.Conditions))
		}
		if result.Conditions[0].Name != "Type 2 Diabetes Mellitus" {
			t.Errorf("Conditions[0].Name = %v", result.Conditions[0].Name)
		}
		if result.Conditions[0].Code != "E11.9" {
			t.Errorf("Conditions[0].Code = %v", result.Conditions[0].Code)
		}
	})

	t.Run("extracts medications successfully", func(t *testing.T) {
		client := llm.NewMockClient()
		client.WithJSONResponse(map[string]interface{}{
			"medications": []map[string]interface{}{
				{
					"name":        "Metformin",
					"code":        "6809",
					"code_system": "RxNorm",
					"dose":        "500mg",
					"route":       "oral",
					"frequency":   "twice daily",
					"confidence":  0.90,
				},
			},
			"overall_confidence": 0.88,
		})

		e, _ := NewExtractor(Config{Client: client})

		result, err := e.Extract(ctx, "Taking Metformin 500mg PO BID.", ExtractionOptions{
			ExtractMedications: true,
		})
		if err != nil {
			t.Fatalf("Extract error: %v", err)
		}

		if len(result.Medications) != 1 {
			t.Fatalf("len(Medications) = %d, want 1", len(result.Medications))
		}
		if result.Medications[0].Name != "Metformin" {
			t.Errorf("Medications[0].Name = %v", result.Medications[0].Name)
		}
		if result.Medications[0].Dosage != "500mg" {
			t.Errorf("Medications[0].Dosage = %v", result.Medications[0].Dosage)
		}
	})

	t.Run("extracts vital signs successfully", func(t *testing.T) {
		client := llm.NewMockClient()
		client.WithJSONResponse(map[string]interface{}{
			"vital_signs": []map[string]interface{}{
				{
					"name":       "Blood Pressure",
					"loinc_code": "85354-9",
					"value":      "120/80",
					"unit":       "mmHg",
					"confidence": 0.95,
				},
			},
			"overall_confidence": 0.93,
		})

		e, _ := NewExtractor(Config{Client: client})

		result, err := e.Extract(ctx, "BP: 120/80 mmHg", ExtractionOptions{
			ExtractVitalSigns: true,
		})
		if err != nil {
			t.Fatalf("Extract error: %v", err)
		}

		if len(result.VitalSigns) != 1 {
			t.Fatalf("len(VitalSigns) = %d, want 1", len(result.VitalSigns))
		}
		if result.VitalSigns[0].Value != "120/80" {
			t.Errorf("VitalSigns[0].Value = %v", result.VitalSigns[0].Value)
		}
	})

	t.Run("extracts allergies successfully", func(t *testing.T) {
		client := llm.NewMockClient()
		client.WithJSONResponse(map[string]interface{}{
			"allergies": []map[string]interface{}{
				{
					"name":        "Penicillin",
					"code":        "7980",
					"code_system": "RxNorm",
					"type":        "drug",
					"severity":    "moderate",
					"reaction":    "rash",
					"confidence":  0.88,
				},
			},
			"overall_confidence": 0.85,
		})

		e, _ := NewExtractor(Config{Client: client})

		result, err := e.Extract(ctx, "Allergic to Penicillin, causes rash.", ExtractionOptions{
			ExtractAllergies: true,
		})
		if err != nil {
			t.Fatalf("Extract error: %v", err)
		}

		if len(result.Allergies) != 1 {
			t.Fatalf("len(Allergies) = %d, want 1", len(result.Allergies))
		}
		if result.Allergies[0].Substance != "Penicillin" {
			t.Errorf("Allergies[0].Substance = %v", result.Allergies[0].Substance)
		}
		if result.Allergies[0].Reaction != "rash" {
			t.Errorf("Allergies[0].Reaction = %v", result.Allergies[0].Reaction)
		}
	})

	t.Run("extracts procedures successfully", func(t *testing.T) {
		client := llm.NewMockClient()
		client.WithJSONResponse(map[string]interface{}{
			"procedures": []map[string]interface{}{
				{
					"name":        "Appendectomy",
					"code":        "47600",
					"code_system": "CPT",
					"status":      "completed",
					"date":        "2024-01-15",
					"confidence":  0.92,
				},
			},
			"overall_confidence": 0.90,
		})

		e, _ := NewExtractor(Config{Client: client})

		result, err := e.Extract(ctx, "History of appendectomy.", ExtractionOptions{
			ExtractProcedures: true,
		})
		if err != nil {
			t.Fatalf("Extract error: %v", err)
		}

		if len(result.Procedures) != 1 {
			t.Fatalf("len(Procedures) = %d, want 1", len(result.Procedures))
		}
		if result.Procedures[0].Name != "Appendectomy" {
			t.Errorf("Procedures[0].Name = %v", result.Procedures[0].Name)
		}
	})

	t.Run("filters by MinConfidence", func(t *testing.T) {
		client := llm.NewMockClient()
		client.WithJSONResponse(map[string]interface{}{
			"conditions": []map[string]interface{}{
				{
					"name":       "High Confidence Condition",
					"confidence": 0.95,
				},
				{
					"name":       "Low Confidence Condition",
					"confidence": 0.40,
				},
			},
			"overall_confidence": 0.70,
		})

		e, _ := NewExtractor(Config{Client: client})

		result, err := e.Extract(ctx, "Clinical notes", ExtractionOptions{
			ExtractConditions: true,
			MinConfidence:     0.50,
		})
		if err != nil {
			t.Fatalf("Extract error: %v", err)
		}

		if len(result.Conditions) != 1 {
			t.Fatalf("len(Conditions) = %d, want 1 (filtered by confidence)", len(result.Conditions))
		}
		if result.Conditions[0].Name != "High Confidence Condition" {
			t.Errorf("wrong condition passed filter")
		}
	})

	t.Run("excludes negated entities by default", func(t *testing.T) {
		client := llm.NewMockClient()
		client.WithJSONResponse(map[string]interface{}{
			"conditions": []map[string]interface{}{
				{
					"name":       "Present Condition",
					"negated":    false,
					"confidence": 0.90,
				},
				{
					"name":       "Negated Condition",
					"negated":    true,
					"confidence": 0.90,
				},
			},
			"overall_confidence": 0.90,
		})

		e, _ := NewExtractor(Config{Client: client})

		result, err := e.Extract(ctx, "Clinical notes", ExtractionOptions{
			ExtractConditions: true,
			IncludeNegated:    false,
		})
		if err != nil {
			t.Fatalf("Extract error: %v", err)
		}

		if len(result.Conditions) != 1 {
			t.Fatalf("len(Conditions) = %d, want 1 (negated filtered)", len(result.Conditions))
		}
		if result.Conditions[0].Name != "Present Condition" {
			t.Error("wrong condition passed filter")
		}
	})

	t.Run("includes negated entities when IncludeNegated is true", func(t *testing.T) {
		client := llm.NewMockClient()
		client.WithJSONResponse(map[string]interface{}{
			"conditions": []map[string]interface{}{
				{
					"name":       "Present Condition",
					"negated":    false,
					"confidence": 0.90,
				},
				{
					"name":       "Negated Condition",
					"negated":    true,
					"confidence": 0.90,
				},
			},
			"overall_confidence": 0.90,
		})

		e, _ := NewExtractor(Config{Client: client})

		result, err := e.Extract(ctx, "Clinical notes", ExtractionOptions{
			ExtractConditions: true,
			IncludeNegated:    true,
		})
		if err != nil {
			t.Fatalf("Extract error: %v", err)
		}

		if len(result.Conditions) != 2 {
			t.Errorf("len(Conditions) = %d, want 2 (negated included)", len(result.Conditions))
		}
	})

	t.Run("returns error on LLM failure", func(t *testing.T) {
		client := llm.NewMockClient()
		client.WithError(errors.New("LLM unavailable"))

		e, _ := NewExtractor(Config{Client: client})

		_, err := e.Extract(ctx, "Clinical notes", ExtractionOptions{})
		if err == nil {
			t.Fatal("expected error on LLM failure")
		}
	})

	t.Run("sets metadata correctly", func(t *testing.T) {
		client := llm.NewMockClient()
		client.WithJSONResponse(map[string]interface{}{
			"overall_confidence": 0.85,
		})

		e, _ := NewExtractor(Config{
			Client: client,
			Model:  "test-model",
		})

		text := "Patient presents with symptoms."
		result, err := e.Extract(ctx, text, ExtractionOptions{
			DocumentType: "progress_note",
		})
		if err != nil {
			t.Fatalf("Extract error: %v", err)
		}

		if result.Model != "test-model" {
			t.Errorf("Model = %v, want test-model", result.Model)
		}
		if result.Metadata.DocumentType != "progress_note" {
			t.Errorf("Metadata.DocumentType = %v", result.Metadata.DocumentType)
		}
		if result.Metadata.TextLength != len(text) {
			t.Errorf("Metadata.TextLength = %v, want %d", result.Metadata.TextLength, len(text))
		}
		if result.ProcessingTime == 0 {
			t.Error("ProcessingTime should be > 0")
		}
	})
}

func TestExtractor_Extract_Cache(t *testing.T) {
	ctx := context.Background()

	t.Run("returns cached result on second call", func(t *testing.T) {
		client := llm.NewMockClient()
		client.WithJSONResponse(map[string]interface{}{
			"conditions": []map[string]interface{}{
				{"name": "Diabetes", "confidence": 0.9},
			},
			"overall_confidence": 0.85,
		})

		e, _ := NewExtractor(Config{
			Client:      client,
			EnableCache: true,
			CacheTTL:    1 * time.Minute,
		})

		text := "Patient has diabetes."
		opts := ExtractionOptions{ExtractConditions: true}

		// First call
		result1, err := e.Extract(ctx, text, opts)
		if err != nil {
			t.Fatalf("Extract error: %v", err)
		}

		// Second call - should use cache
		result2, err := e.Extract(ctx, text, opts)
		if err != nil {
			t.Fatalf("Extract error: %v", err)
		}

		// Only one LLM call should have been made
		if client.CallCount() != 1 {
			t.Errorf("CallCount = %d, want 1 (cached)", client.CallCount())
		}

		// Results should be identical
		if len(result1.Conditions) != len(result2.Conditions) {
			t.Error("cached result differs from original")
		}
	})
}

func TestExtractor_ExtractFromDocument(t *testing.T) {
	ctx := context.Background()

	t.Run("returns error for nil document", func(t *testing.T) {
		client := llm.NewMockClient()
		e, _ := NewExtractor(Config{Client: client})

		_, err := e.ExtractFromDocument(ctx, nil, ExtractionOptions{})
		if err == nil {
			t.Fatal("expected error for nil document")
		}
	})

	t.Run("returns empty result for empty content", func(t *testing.T) {
		client := llm.NewMockClient()
		e, _ := NewExtractor(Config{Client: client})

		doc := &events.DocumentEvent{Content: ""}
		result, err := e.ExtractFromDocument(ctx, doc, ExtractionOptions{})
		if err != nil {
			t.Fatalf("ExtractFromDocument error: %v", err)
		}
		if result.Confidence != 0 {
			t.Errorf("Confidence = %v, want 0", result.Confidence)
		}
	})

	t.Run("returns error for base64 encoded content", func(t *testing.T) {
		client := llm.NewMockClient()
		e, _ := NewExtractor(Config{Client: client})

		doc := &events.DocumentEvent{
			ContentEncoding: "base64",
		}
		_, err := e.ExtractFromDocument(ctx, doc, ExtractionOptions{})
		if err == nil {
			t.Fatal("expected error for base64 encoded content")
		}
	})

	t.Run("extracts from document content", func(t *testing.T) {
		client := llm.NewMockClient()
		client.WithJSONResponse(map[string]interface{}{
			"conditions":         []map[string]interface{}{},
			"overall_confidence": 0.80,
		})

		e, _ := NewExtractor(Config{Client: client})

		doc := &events.DocumentEvent{
			Content:      "Patient notes content here.",
			DocumentType: "progress_note",
		}

		result, err := e.ExtractFromDocument(ctx, doc, ExtractionOptions{
			ExtractConditions: true,
		})
		if err != nil {
			t.Fatalf("ExtractFromDocument error: %v", err)
		}

		if client.CallCount() != 1 {
			t.Errorf("CallCount = %d, want 1", client.CallCount())
		}
		if result.Confidence != 0.80 {
			t.Errorf("Confidence = %v, want 0.80", result.Confidence)
		}
	})

	t.Run("uses document type from doc when not specified in options", func(t *testing.T) {
		client := llm.NewMockClient()
		client.WithJSONResponse(map[string]interface{}{
			"overall_confidence": 0.80,
		})

		e, _ := NewExtractor(Config{Client: client})

		doc := &events.DocumentEvent{
			Content:      "Notes",
			DocumentType: "discharge_summary",
		}

		result, err := e.ExtractFromDocument(ctx, doc, ExtractionOptions{})
		if err != nil {
			t.Fatalf("ExtractFromDocument error: %v", err)
		}

		if result.Metadata.DocumentType != "discharge_summary" {
			t.Errorf("Metadata.DocumentType = %v, want discharge_summary", result.Metadata.DocumentType)
		}
	})
}

func TestDefaultExtractionOptions(t *testing.T) {
	opts := DefaultExtractionOptions()

	if !opts.ExtractConditions {
		t.Error("ExtractConditions should be true by default")
	}
	if !opts.ExtractMedications {
		t.Error("ExtractMedications should be true by default")
	}
	if !opts.ExtractVitalSigns {
		t.Error("ExtractVitalSigns should be true by default")
	}
	if !opts.ExtractAllergies {
		t.Error("ExtractAllergies should be true by default")
	}
	if !opts.ExtractProcedures {
		t.Error("ExtractProcedures should be true by default")
	}
	if opts.MinConfidence != 0.7 {
		t.Errorf("MinConfidence = %v, want 0.7", opts.MinConfidence)
	}
	if opts.IncludeNegated {
		t.Error("IncludeNegated should be false by default")
	}
	if opts.MaxTokens != 4096 {
		t.Errorf("MaxTokens = %v, want 4096", opts.MaxTokens)
	}
}

func TestExtractionCache(t *testing.T) {
	t.Run("get returns false for missing key", func(t *testing.T) {
		cache := newExtractionCache(1 * time.Hour)
		_, ok := cache.get("missing")
		if ok {
			t.Error("expected false for missing key")
		}
	})

	t.Run("get returns value for existing key", func(t *testing.T) {
		cache := newExtractionCache(1 * time.Hour)
		result := &ExtractionResult{Confidence: 0.9}
		cache.set("key", result)

		got, ok := cache.get("key")
		if !ok {
			t.Fatal("expected true for existing key")
		}
		if got.Confidence != 0.9 {
			t.Errorf("Confidence = %v, want 0.9", got.Confidence)
		}
	})

	t.Run("get returns false for expired entry", func(t *testing.T) {
		cache := newExtractionCache(1 * time.Millisecond)
		result := &ExtractionResult{Confidence: 0.9}
		cache.set("key", result)

		// Wait for expiration
		time.Sleep(5 * time.Millisecond)

		_, ok := cache.get("key")
		if ok {
			t.Error("expected false for expired entry")
		}
	})
}

func TestParseExtractionResponse(t *testing.T) {
	client := llm.NewMockClient()
	e, _ := NewExtractor(Config{Client: client})

	t.Run("handles empty response", func(t *testing.T) {
		result, err := e.parseExtractionResponse(json.RawMessage(`{}`), ExtractionOptions{})
		if err != nil {
			t.Fatalf("parseExtractionResponse error: %v", err)
		}
		if result == nil {
			t.Fatal("expected non-nil result")
		}
	})

	t.Run("handles malformed JSON", func(t *testing.T) {
		_, err := e.parseExtractionResponse(json.RawMessage(`{invalid`), ExtractionOptions{})
		if err == nil {
			t.Fatal("expected error for malformed JSON")
		}
	})
}
