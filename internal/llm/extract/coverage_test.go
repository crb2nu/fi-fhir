package extract

import (
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// intToString — 0% → target 100%
// ---------------------------------------------------------------------------

func TestIntToString_Zero(t *testing.T) {
	if got := intToString(0); got != "0" {
		t.Errorf("intToString(0) = %q, want %q", got, "0")
	}
}

func TestIntToString_Positive(t *testing.T) {
	tests := []struct {
		input int
		want  string
	}{
		{1, "1"},
		{9, "9"},
		{10, "10"},
		{42, "42"},
		{100, "100"},
		{12345, "12345"},
		{999999, "999999"},
	}
	for _, tt := range tests {
		if got := intToString(tt.input); got != tt.want {
			t.Errorf("intToString(%d) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestIntToString_Negative(t *testing.T) {
	tests := []struct {
		input int
		want  string
	}{
		{-1, "-1"},
		{-42, "-42"},
		{-12345, "-12345"},
	}
	for _, tt := range tests {
		if got := intToString(tt.input); got != tt.want {
			t.Errorf("intToString(%d) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// truncateText — 28.6% → target 100%
// ---------------------------------------------------------------------------

func TestTruncateText_ShortText(t *testing.T) {
	text := "Hello, world!"
	got := truncateText(text, 100)
	if got != text {
		t.Errorf("got %q, want %q (text shorter than maxLen should be unchanged)", got, text)
	}
}

func TestTruncateText_ExactLength(t *testing.T) {
	text := "12345"
	got := truncateText(text, 5)
	if got != text {
		t.Errorf("got %q, want %q (text exactly maxLen should be unchanged)", got, text)
	}
}

func TestTruncateText_TruncatesAtWordBoundary(t *testing.T) {
	text := "The patient presented with chest pain and shortness of breath requiring immediate evaluation"
	got := truncateText(text, 50)
	// Should truncate and preserve word boundary
	if len(got) > 70 { // maxLen + suffix
		t.Errorf("truncated text too long: %d chars", len(got))
	}
	if got == text {
		t.Error("text was not truncated")
	}
	// Should end with truncation marker
	suffix := "...[truncated]"
	if len(got) < len(suffix) || got[len(got)-len(suffix):] != suffix {
		t.Errorf("expected truncation marker, got suffix %q", got[len(got)-min(len(got), 20):])
	}
}

func TestTruncateText_NoWordBoundaryNearEnd(t *testing.T) {
	// A long word with no spaces — should truncate without panic
	text := "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA" +
		"BBBBBBBBBB" // 108 chars total, no spaces
	got := truncateText(text, 50)
	if got == text {
		t.Error("text was not truncated")
	}
	// No spaces → should truncate at maxLen and append marker
	if len(got) < 14 {
		t.Error("truncated text too short")
	}
}

func TestTruncateText_EmptyString(t *testing.T) {
	got := truncateText("", 100)
	if got != "" {
		t.Errorf("got %q, want empty string", got)
	}
}

// ---------------------------------------------------------------------------
// buildExtractionPrompt — 76.9% → exercise patient context branches
// ---------------------------------------------------------------------------

func TestBuildExtractionPrompt_DefaultDocType(t *testing.T) {
	prompt := buildExtractionPrompt("Some clinical text", ExtractionOptions{})
	if prompt == "" {
		t.Error("prompt should not be empty")
	}
	// Default doc type should be "clinical"
	if len(prompt) < 10 {
		t.Error("prompt too short")
	}
}

func TestBuildExtractionPrompt_WithPatientAge(t *testing.T) {
	opts := ExtractionOptions{
		PatientAge: 45,
	}
	prompt := buildExtractionPrompt("Patient has chest pain", opts)
	if prompt == "" {
		t.Error("prompt should not be empty")
	}
}

func TestBuildExtractionPrompt_WithPatientGender(t *testing.T) {
	opts := ExtractionOptions{
		PatientGender: "female",
	}
	prompt := buildExtractionPrompt("Patient has chest pain", opts)
	if prompt == "" {
		t.Error("prompt should not be empty")
	}
}

func TestBuildExtractionPrompt_WithAgeAndGender(t *testing.T) {
	opts := ExtractionOptions{
		PatientAge:    72,
		PatientGender: "male",
		DocumentType:  "discharge_summary",
	}
	prompt := buildExtractionPrompt("Patient discharged in stable condition", opts)
	if prompt == "" {
		t.Error("prompt should not be empty")
	}
}

func TestBuildExtractionPrompt_AllEntityTypes(t *testing.T) {
	opts := ExtractionOptions{
		ExtractConditions:  true,
		ExtractMedications: true,
		ExtractVitalSigns:  true,
		ExtractAllergies:   true,
		ExtractProcedures:  true,
		DocumentType:       "progress_note",
	}
	prompt := buildExtractionPrompt("BP 120/80, HR 72. Continued metformin.", opts)
	if prompt == "" {
		t.Error("prompt should not be empty")
	}
}

func TestBuildExtractionPrompt_LongTextTruncated(t *testing.T) {
	// Create text longer than 12000 chars
	longText := ""
	for i := 0; i < 2000; i++ {
		longText += "Clinical note line with important patient data. "
	}
	opts := ExtractionOptions{DocumentType: "clinical"}
	prompt := buildExtractionPrompt(longText, opts)
	if prompt == "" {
		t.Error("prompt should not be empty")
	}
	// The prompt should be bounded (text gets truncated at 12000 chars)
	if len(prompt) > 20000 {
		t.Errorf("prompt too large: %d chars (should be bounded by truncation)", len(prompt))
	}
}

// ---------------------------------------------------------------------------
// buildSystemPrompt — exercise for coverage
// ---------------------------------------------------------------------------

func TestBuildSystemPrompt_NotEmpty(t *testing.T) {
	prompt := buildSystemPrompt()
	if prompt == "" {
		t.Error("system prompt should not be empty")
	}
}

// ---------------------------------------------------------------------------
// extractionCache.set — 50% → exercise cleanup path (>1000 entries)
// ---------------------------------------------------------------------------

func TestCacheSet_BasicEntry(t *testing.T) {
	cache := newExtractionCache(time.Minute)
	result := &ExtractionResult{Confidence: 0.9}
	cache.set("key1", result)

	got, ok := cache.get("key1")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if got.Confidence != 0.9 {
		t.Errorf("got confidence %f, want 0.9", got.Confidence)
	}
}

func TestCacheSet_CleanupOnOverflow(t *testing.T) {
	cache := newExtractionCache(1 * time.Nanosecond) // very short TTL
	result := &ExtractionResult{Confidence: 0.5}

	// Fill cache past 1000 entries (all will be expired due to 1ns TTL)
	for i := 0; i < 1001; i++ {
		cache.set(intToString(i), result)
	}

	// After set with >1000 entries, cleanup should have run
	// and removed expired entries. The trigger entry itself is fresh,
	// but previous ones should be cleaned.
	time.Sleep(2 * time.Millisecond) // ensure TTLs expire

	// Add one more to trigger cleanup
	cache.set("trigger", result)

	// Cache should be smaller now (most entries expired and cleaned)
	cache.mu.RLock()
	size := len(cache.entries)
	cache.mu.RUnlock()

	if size > 500 {
		t.Errorf("cache cleanup didn't work: still has %d entries", size)
	}
}

func TestCacheGet_Expired(t *testing.T) {
	cache := newExtractionCache(1 * time.Nanosecond)
	result := &ExtractionResult{Confidence: 0.8}
	cache.set("key1", result)

	time.Sleep(2 * time.Millisecond)

	_, ok := cache.get("key1")
	if ok {
		t.Error("expected cache miss for expired entry")
	}
}

func TestCacheGet_Miss(t *testing.T) {
	cache := newExtractionCache(time.Minute)
	_, ok := cache.get("nonexistent")
	if ok {
		t.Error("expected cache miss")
	}
}
