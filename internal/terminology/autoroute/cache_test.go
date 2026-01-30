package autoroute

import (
	"testing"
	"time"
)

func TestNewCache(t *testing.T) {
	tests := []struct {
		name        string
		cfg         CacheConfig
		wantTTL     time.Duration
		wantMaxSize int
	}{
		{
			name:        "default values when zero",
			cfg:         CacheConfig{},
			wantTTL:     15 * time.Minute,
			wantMaxSize: 10000,
		},
		{
			name: "custom values",
			cfg: CacheConfig{
				TTL:     5 * time.Minute,
				MaxSize: 100,
			},
			wantTTL:     5 * time.Minute,
			wantMaxSize: 100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewCache(tt.cfg)
			if c.ttl != tt.wantTTL {
				t.Errorf("TTL = %v, want %v", c.ttl, tt.wantTTL)
			}
			if c.maxSize != tt.wantMaxSize {
				t.Errorf("maxSize = %v, want %v", c.maxSize, tt.wantMaxSize)
			}
		})
	}
}

func TestCacheGetSet(t *testing.T) {
	c := NewCache(CacheConfig{TTL: 1 * time.Hour, MaxSize: 100})

	req := SuggestRequest{
		SourceCode:   "LAB001",
		SourceSystem: "custom",
		TargetSystem: "http://loinc.org",
	}

	// Initially empty
	if got := c.Get(req); got != nil {
		t.Error("expected nil for missing key")
	}

	// Set and retrieve
	result := &SuggestResult{
		Confidence: 0.95,
		Reasoning:  "test",
	}
	c.Set(req, result)

	got := c.Get(req)
	if got == nil {
		t.Fatal("expected cached result")
	}
	if got.Confidence != result.Confidence {
		t.Errorf("Confidence = %v, want %v", got.Confidence, result.Confidence)
	}
	if got.Reasoning != result.Reasoning {
		t.Errorf("Reasoning = %v, want %v", got.Reasoning, result.Reasoning)
	}
}

func TestCacheExpiration(t *testing.T) {
	c := NewCache(CacheConfig{TTL: 10 * time.Millisecond, MaxSize: 100})

	req := SuggestRequest{
		SourceCode:   "LAB001",
		SourceSystem: "custom",
		TargetSystem: "http://loinc.org",
	}

	result := &SuggestResult{Confidence: 0.95}
	c.Set(req, result)

	// Should be cached
	if c.Get(req) == nil {
		t.Error("expected cached result immediately after set")
	}

	// Wait for expiration
	time.Sleep(20 * time.Millisecond)

	// Should be expired
	if c.Get(req) != nil {
		t.Error("expected nil for expired entry")
	}
}

func TestCacheInvalidate(t *testing.T) {
	c := NewCache(CacheConfig{TTL: 1 * time.Hour, MaxSize: 100})

	req := SuggestRequest{
		SourceCode:   "LAB001",
		SourceSystem: "custom",
		TargetSystem: "http://loinc.org",
	}

	result := &SuggestResult{Confidence: 0.95}
	c.Set(req, result)

	// Verify cached
	if c.Get(req) == nil {
		t.Fatal("expected cached result")
	}

	// Invalidate
	c.Invalidate(req)

	// Should be gone
	if c.Get(req) != nil {
		t.Error("expected nil after invalidation")
	}
}

func TestCacheClear(t *testing.T) {
	c := NewCache(CacheConfig{TTL: 1 * time.Hour, MaxSize: 100})

	// Add multiple entries
	for i := 0; i < 5; i++ {
		req := SuggestRequest{
			SourceCode:   "LAB00" + string(rune('1'+i)),
			SourceSystem: "custom",
			TargetSystem: "http://loinc.org",
		}
		c.Set(req, &SuggestResult{Confidence: float64(i) * 0.1})
	}

	if c.Size() != 5 {
		t.Errorf("Size = %d, want 5", c.Size())
	}

	c.Clear()

	if c.Size() != 0 {
		t.Errorf("Size after clear = %d, want 0", c.Size())
	}
}

func TestCacheSize(t *testing.T) {
	c := NewCache(CacheConfig{TTL: 1 * time.Hour, MaxSize: 100})

	if c.Size() != 0 {
		t.Errorf("initial Size = %d, want 0", c.Size())
	}

	req1 := SuggestRequest{SourceCode: "A", SourceSystem: "x", TargetSystem: "y"}
	req2 := SuggestRequest{SourceCode: "B", SourceSystem: "x", TargetSystem: "y"}

	c.Set(req1, &SuggestResult{})
	if c.Size() != 1 {
		t.Errorf("Size after 1 set = %d, want 1", c.Size())
	}

	c.Set(req2, &SuggestResult{})
	if c.Size() != 2 {
		t.Errorf("Size after 2 sets = %d, want 2", c.Size())
	}

	// Setting same key should not increase size
	c.Set(req1, &SuggestResult{Confidence: 0.5})
	if c.Size() != 2 {
		t.Errorf("Size after re-set = %d, want 2", c.Size())
	}
}

func TestCacheEviction(t *testing.T) {
	maxSize := 10
	c := NewCache(CacheConfig{TTL: 1 * time.Hour, MaxSize: maxSize})

	// Fill cache beyond capacity
	for i := 0; i < maxSize+5; i++ {
		req := SuggestRequest{
			SourceCode:   string(rune('A' + i)),
			SourceSystem: "x",
			TargetSystem: "y",
		}
		c.Set(req, &SuggestResult{Confidence: float64(i) * 0.01})
	}

	// Size should not exceed maxSize
	if c.Size() > maxSize {
		t.Errorf("Size = %d, exceeds maxSize %d", c.Size(), maxSize)
	}
}

func TestCacheKeyDeterminism(t *testing.T) {
	c := NewCache(CacheConfig{TTL: 1 * time.Hour, MaxSize: 100})

	req := SuggestRequest{
		SourceCode:    "LAB001",
		SourceSystem:  "custom",
		SourceDisplay: "Hemoglobin",
		TargetSystem:  "http://loinc.org",
		ProfileID:     "profile-1",
	}

	key1 := c.key(req)
	key2 := c.key(req)

	if key1 != key2 {
		t.Errorf("key not deterministic: %s != %s", key1, key2)
	}

	// Different request should produce different key
	req2 := req
	req2.SourceCode = "LAB002"
	key3 := c.key(req2)

	if key1 == key3 {
		t.Error("different requests should produce different keys")
	}
}

func TestDefaultCacheConfig(t *testing.T) {
	cfg := DefaultCacheConfig()

	if cfg.TTL != 15*time.Minute {
		t.Errorf("TTL = %v, want 15m", cfg.TTL)
	}
	if cfg.MaxSize != 10000 {
		t.Errorf("MaxSize = %d, want 10000", cfg.MaxSize)
	}
}
