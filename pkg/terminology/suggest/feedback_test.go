package suggest

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/terminology/index"
)

// --- Tests for NewFeedbackStore ---

func TestNewFeedbackStore(t *testing.T) {
	cfg := FeedbackStoreConfig{
		QdrantURL:    "http://localhost:6333",
		QdrantAPIKey: "test-key",
		Collection:   "test_feedback",
	}

	store := NewFeedbackStore(cfg)
	if store == nil {
		t.Fatal("NewFeedbackStore() returned nil")
	}
	if store.collection != "test_feedback" {
		t.Errorf("collection = %q, want %q", store.collection, "test_feedback")
	}
	if store.qdrant == nil {
		t.Error("qdrant client should not be nil")
	}
	if store.codeIndex == nil {
		t.Error("codeIndex map should be initialized")
	}
	if store.cacheTTL != 10*time.Minute {
		t.Errorf("cacheTTL = %v, want 10m", store.cacheTTL)
	}
}

// --- Tests for generateFeedbackID ---

func TestGenerateFeedbackID(t *testing.T) {
	fb := Feedback{
		SourceCode:       "LAB-001",
		SourceSystem:     "hospital",
		SuggestedCode:    "58410-2",
		TargetVocabulary: index.VocabularyLOINC,
	}

	id := generateFeedbackID(fb)
	if id == "" {
		t.Error("ID should not be empty")
	}
	expected := "LAB-001:hospital:58410-2:loinc"
	if id != expected {
		t.Errorf("ID = %q, want %q", id, expected)
	}
}

func TestGenerateFeedbackID_EmptyFields(t *testing.T) {
	fb := Feedback{}
	id := generateFeedbackID(fb)
	if id != ":::" {
		t.Errorf("ID = %q, want %q", id, ":::")
	}
}

// --- Tests for payloadToFeedback ---

func TestPayloadToFeedback(t *testing.T) {
	payload := map[string]interface{}{
		"source_code":       "LAB-001",
		"source_display":    "Complete Blood Count",
		"source_system":     "hospital",
		"suggested_code":    "58410-2",
		"suggested_display": "CBC panel",
		"target_vocabulary": "loinc",
		"accepted":          true,
		"accepted_code":     "58410-2",
		"accepted_display":  "CBC panel",
		"confidence":        0.85,
		"strategy":          "semantic",
		"accept_count":      float64(3), // JSON numbers are float64
	}

	fb, err := payloadToFeedback(payload)
	if err != nil {
		t.Fatalf("payloadToFeedback() error = %v", err)
	}

	if fb.SourceCode != "LAB-001" {
		t.Errorf("SourceCode = %q", fb.SourceCode)
	}
	if fb.SourceDisplay != "Complete Blood Count" {
		t.Errorf("SourceDisplay = %q", fb.SourceDisplay)
	}
	if fb.SourceSystem != "hospital" {
		t.Errorf("SourceSystem = %q", fb.SourceSystem)
	}
	if !fb.Accepted {
		t.Error("Accepted should be true")
	}
	if fb.AcceptedCode != "58410-2" {
		t.Errorf("AcceptedCode = %q", fb.AcceptedCode)
	}
	if fb.Strategy != StrategySemantic {
		t.Errorf("Strategy = %q", fb.Strategy)
	}
}

func TestPayloadToFeedback_EmptyPayload(t *testing.T) {
	fb, err := payloadToFeedback(map[string]interface{}{})
	if err != nil {
		t.Fatalf("payloadToFeedback() error = %v", err)
	}
	if fb.SourceCode != "" {
		t.Error("expected empty SourceCode for empty payload")
	}
}

func TestPayloadToFeedback_NilPayload(t *testing.T) {
	_, err := payloadToFeedback(nil)
	if err != nil {
		t.Fatalf("payloadToFeedback(nil) error = %v", err)
	}
}

// --- Tests for helper functions ---

func TestGetString(t *testing.T) {
	payload := map[string]interface{}{
		"key":    "value",
		"number": 42,
		"nil":    nil,
	}

	if got := getString(payload, "key"); got != "value" {
		t.Errorf("getString(key) = %q, want %q", got, "value")
	}
	if got := getString(payload, "number"); got != "" {
		t.Errorf("getString(number) = %q, want empty (not a string)", got)
	}
	if got := getString(payload, "missing"); got != "" {
		t.Errorf("getString(missing) = %q, want empty", got)
	}
	if got := getString(payload, "nil"); got != "" {
		t.Errorf("getString(nil) = %q, want empty", got)
	}
	if got := getString(nil, "key"); got != "" {
		t.Errorf("getString(nil map) = %q, want empty", got)
	}
}

func TestGetBool(t *testing.T) {
	payload := map[string]interface{}{
		"true_val":  true,
		"false_val": false,
		"string":    "true",
		"nil":       nil,
	}

	if got := getBool(payload, "true_val"); !got {
		t.Error("getBool(true_val) = false, want true")
	}
	if got := getBool(payload, "false_val"); got {
		t.Error("getBool(false_val) = true, want false")
	}
	if got := getBool(payload, "string"); got {
		t.Error("getBool(string) should return false for non-bool type")
	}
	if got := getBool(payload, "missing"); got {
		t.Error("getBool(missing) should return false")
	}
	if got := getBool(nil, "key"); got {
		t.Error("getBool(nil map) should return false")
	}
}

func TestGetInt(t *testing.T) {
	payload := map[string]interface{}{
		"int_val":     42,
		"int64_val":   int64(100),
		"float64_val": float64(7.0),
		"float_frac":  float64(7.5), // Truncated to 7
		"string":      "42",
		"nil":         nil,
	}

	if got := getInt(payload, "int_val"); got != 42 {
		t.Errorf("getInt(int_val) = %d, want 42", got)
	}
	if got := getInt(payload, "int64_val"); got != 100 {
		t.Errorf("getInt(int64_val) = %d, want 100", got)
	}
	if got := getInt(payload, "float64_val"); got != 7 {
		t.Errorf("getInt(float64_val) = %d, want 7", got)
	}
	if got := getInt(payload, "float_frac"); got != 7 {
		t.Errorf("getInt(float_frac) = %d, want 7 (truncated)", got)
	}
	if got := getInt(payload, "string"); got != 0 {
		t.Errorf("getInt(string) = %d, want 0 (not a number)", got)
	}
	if got := getInt(payload, "missing"); got != 0 {
		t.Errorf("getInt(missing) = %d, want 0", got)
	}
	if got := getInt(nil, "key"); got != 0 {
		t.Errorf("getInt(nil map) = %d, want 0", got)
	}
}

// --- Tests for invalidateCache ---

func TestInvalidateCache(t *testing.T) {
	store := NewFeedbackStore(FeedbackStoreConfig{
		QdrantURL:  "http://localhost:6333",
		Collection: "test",
	})

	// Populate cache
	store.codeIndex["key"] = []Feedback{{SourceCode: "test"}}
	store.cacheTime = time.Now()

	store.invalidateCache()

	if len(store.codeIndex) != 0 {
		t.Error("cache should be cleared after invalidation")
	}
	if !store.cacheTime.IsZero() {
		t.Error("cache time should be zero after invalidation")
	}
}

// --- Tests for Feedback struct ---

func TestFeedback_JSONRoundTrip(t *testing.T) {
	fb := Feedback{
		ID:               "test-id",
		SourceCode:       "LAB-001",
		SourceDisplay:    "CBC",
		SourceSystem:     "hospital",
		SuggestedCode:    "58410-2",
		SuggestedDisplay: "CBC panel",
		TargetVocabulary: index.VocabularyLOINC,
		Accepted:         true,
		AcceptedCode:     "58410-2",
		AcceptedDisplay:  "CBC panel",
		Confidence:       0.85,
		Strategy:         StrategySemantic,
		AcceptCount:      3,
		UserID:           "user-1",
		Notes:            "Verified by clinician",
	}

	// Marshal and unmarshal to verify JSON tags work.
	data, err := payloadFromFeedback(fb)
	if err != nil {
		t.Fatalf("marshal error = %v", err)
	}

	fb2, err := payloadToFeedback(data)
	if err != nil {
		t.Fatalf("payloadToFeedback() error = %v", err)
	}

	if fb2.ID != fb.ID {
		t.Errorf("ID = %q, want %q", fb2.ID, fb.ID)
	}
	if fb2.SourceCode != fb.SourceCode {
		t.Errorf("SourceCode = %q, want %q", fb2.SourceCode, fb.SourceCode)
	}
	if fb2.Accepted != fb.Accepted {
		t.Errorf("Accepted = %v, want %v", fb2.Accepted, fb.Accepted)
	}
}

// --- Tests for FeedbackStore methods requiring Qdrant ---

func TestFeedbackStore_FindBySourceCode_CacheHit(t *testing.T) {
	store := NewFeedbackStore(FeedbackStoreConfig{
		QdrantURL:  "http://localhost:6333",
		Collection: "test",
	})

	// Pre-populate cache
	cached := []Feedback{{SourceCode: "code1", SourceSystem: "sys1"}}
	store.mu.Lock()
	store.codeIndex["code1:sys1"] = cached
	store.cacheTime = time.Now()
	store.mu.Unlock()

	// Should return from cache without hitting Qdrant.
	results, err := store.FindBySourceCode(context.Background(), "code1", "sys1")
	if err != nil {
		t.Fatalf("FindBySourceCode() error = %v", err)
	}
	if len(results) != 1 {
		t.Errorf("got %d results, want 1 (from cache)", len(results))
	}
}

func TestFeedbackStore_FindSimilar_NoEmbedder(t *testing.T) {
	store := NewFeedbackStore(FeedbackStoreConfig{
		QdrantURL:  "http://localhost:6333",
		Collection: "test",
	})
	// embedder is nil by default.

	_, err := store.FindSimilar(context.Background(), "test text", index.VocabularyLOINC, 5)
	if err == nil {
		t.Error("FindSimilar() should error when embedder is nil")
	}
}

// --- Tests for FeedbackStats struct ---

func TestFeedbackStats_Structure(t *testing.T) {
	stats := FeedbackStats{
		TotalEntries:   100,
		AcceptedCount:  80,
		RejectedCount:  20,
		ByVocabulary:   map[string]int64{"loinc": 50, "snomedct": 50},
		ByStrategy:     map[string]int64{"semantic": 60, "llm": 40},
		AvgAcceptCount: 2.5,
	}

	if stats.TotalEntries != 100 {
		t.Error("TotalEntries mismatch")
	}
	if stats.AcceptedCount+stats.RejectedCount != stats.TotalEntries {
		t.Error("accepted + rejected should equal total")
	}
}

// --- Helper to convert Feedback to payload map (for testing round-trips) ---

func payloadFromFeedback(fb Feedback) (map[string]interface{}, error) {
	data, err := json.Marshal(fb)
	if err != nil {
		return nil, err
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return m, nil
}
