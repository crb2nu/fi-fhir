package workflow

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// TokenBucket Tests

func TestTokenBucketAllow(t *testing.T) {
	// 10 tokens/sec, burst of 5
	tb := NewTokenBucket(10, 5)

	// Should allow burst of 5
	for i := 0; i < 5; i++ {
		if !tb.Allow() {
			t.Errorf("request %d should be allowed", i)
		}
	}

	// 6th request should be denied (no tokens left)
	if tb.Allow() {
		t.Error("6th request should be denied")
	}
}

func TestTokenBucketAllowN(t *testing.T) {
	tb := NewTokenBucket(10, 10)

	// Should allow 5 at once
	if !tb.AllowN(5) {
		t.Error("should allow 5 tokens")
	}

	// Should have approximately 5 left (allow for tiny refills)
	tokens := tb.Tokens()
	if tokens < 4.9 || tokens > 5.1 {
		t.Errorf("expected ~5 tokens, got %f", tokens)
	}

	// Should not allow 6 more
	if tb.AllowN(6) {
		t.Error("should not allow 6 tokens when only ~5 remain")
	}

	// Should still have approximately 5 (AllowN doesn't consume on failure)
	tokens = tb.Tokens()
	if tokens < 4.9 || tokens > 5.2 {
		t.Errorf("expected ~5 tokens after failed AllowN, got %f", tokens)
	}
}

func TestTokenBucketRefill(t *testing.T) {
	// 100 tokens/sec, burst of 10
	tb := NewTokenBucket(100, 10)

	// Consume all tokens
	tb.AllowN(10)
	// Should have approximately 0 tokens (allow for tiny refills during call)
	tokens := tb.Tokens()
	if tokens > 0.1 {
		t.Errorf("expected ~0 tokens, got %f", tokens)
	}

	// Wait for refill (100/sec = 10 tokens in 100ms)
	time.Sleep(110 * time.Millisecond)

	// Should have ~10 tokens (capped at burst)
	tokens = tb.Tokens()
	if tokens < 9 || tokens > 10 {
		t.Errorf("expected ~10 tokens after refill, got %f", tokens)
	}
}

func TestTokenBucketWait(t *testing.T) {
	// 100 tokens/sec, burst of 1
	tb := NewTokenBucket(100, 1)

	// Consume the one token
	tb.Allow()

	// Wait should block briefly then succeed
	ctx := context.Background()
	start := time.Now()
	err := tb.Wait(ctx)
	elapsed := time.Since(start)

	if err != nil {
		t.Errorf("Wait should succeed: %v", err)
	}

	// Should have waited ~10ms for 1 token at 100/sec
	if elapsed < 5*time.Millisecond || elapsed > 50*time.Millisecond {
		t.Errorf("expected wait ~10ms, got %v", elapsed)
	}
}

func TestTokenBucketWaitContext(t *testing.T) {
	// Very slow rate
	tb := NewTokenBucket(0.1, 1) // 1 token per 10 seconds

	// Consume the token
	tb.Allow()

	// Cancel context quickly
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := tb.Wait(ctx)

	if err == nil {
		t.Error("Wait should fail when context cancelled")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected DeadlineExceeded, got %v", err)
	}
}

func TestTokenBucketReserve(t *testing.T) {
	tb := NewTokenBucket(10, 5)

	// Reserve when tokens available - should be immediate
	wait := tb.Reserve()
	if wait != 0 {
		t.Errorf("expected 0 wait when tokens available, got %v", wait)
	}

	// Consume remaining tokens
	tb.AllowN(4)

	// Reserve when empty - should return wait time
	wait = tb.Reserve()
	if wait == 0 {
		t.Error("expected non-zero wait when no tokens")
	}
	// At 10/sec, 1 token takes 100ms
	if wait < 90*time.Millisecond || wait > 110*time.Millisecond {
		t.Errorf("expected ~100ms wait, got %v", wait)
	}
}

func TestTokenBucketLimitAndBurst(t *testing.T) {
	tb := NewTokenBucket(42.5, 100)

	if tb.Limit() != 42.5 {
		t.Errorf("expected limit 42.5, got %f", tb.Limit())
	}
	if tb.Burst() != 100 {
		t.Errorf("expected burst 100, got %d", tb.Burst())
	}
}

func TestTokenBucketDefaults(t *testing.T) {
	// Invalid values should use defaults
	tb := NewTokenBucket(0, 0)

	if tb.Limit() != 1.0 {
		t.Errorf("expected default limit 1.0, got %f", tb.Limit())
	}
	if tb.Burst() != 1 {
		t.Errorf("expected default burst 1, got %d", tb.Burst())
	}
}

func TestTokenBucketConcurrency(t *testing.T) {
	tb := NewTokenBucket(1000, 100)

	var wg sync.WaitGroup
	allowed := make(chan bool, 1000)

	// Spawn goroutines that try to get tokens
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				allowed <- tb.Allow()
			}
		}()
	}

	wg.Wait()
	close(allowed)

	// Count successes
	successCount := 0
	for success := range allowed {
		if success {
			successCount++
		}
	}

	// Should have allowed exactly 100 (the burst capacity)
	if successCount != 100 {
		t.Errorf("expected 100 allowed, got %d", successCount)
	}
}

// Registry Tests

func TestRateLimiterRegistry(t *testing.T) {
	registry := NewRateLimiterRegistry()

	// Get creates new limiter
	limiter1 := registry.GetLimiter("endpoint1")
	if limiter1 == nil {
		t.Fatal("expected non-nil limiter")
	}

	// Same key returns same limiter
	limiter2 := registry.GetLimiter("endpoint1")
	if limiter1 != limiter2 {
		t.Error("expected same limiter for same key")
	}

	// Different key returns different limiter
	limiter3 := registry.GetLimiter("endpoint2")
	if limiter1 == limiter3 {
		t.Error("expected different limiter for different key")
	}
}

func TestRateLimiterRegistryCustomConfig(t *testing.T) {
	config := RateLimitConfig{
		Rate:  100,
		Burst: 50,
	}
	registry := NewRateLimiterRegistryWithConfig(config)

	limiter := registry.GetLimiter("test")

	if limiter.Limit() != 100 {
		t.Errorf("expected rate 100, got %f", limiter.Limit())
	}
	if limiter.Burst() != 50 {
		t.Errorf("expected burst 50, got %d", limiter.Burst())
	}
}

func TestRateLimiterRegistrySetAndRemove(t *testing.T) {
	registry := NewRateLimiterRegistry()

	// Set custom limiter
	customLimiter := NewTokenBucket(999, 999)
	registry.SetLimiter("custom", customLimiter)

	got := registry.GetLimiter("custom")
	if got != customLimiter {
		t.Error("expected custom limiter")
	}

	// Remove
	registry.Remove("custom")

	// Should create new default limiter
	got2 := registry.GetLimiter("custom")
	if got2 == customLimiter {
		t.Error("expected new limiter after remove")
	}
}

func TestGlobalRateLimiter(t *testing.T) {
	// Reset global registry
	globalRateLimiterRegistry = NewRateLimiterRegistry()

	limiter := GetRateLimiter("https://api.example.com")
	if limiter == nil {
		t.Fatal("expected non-nil limiter")
	}

	// Should return same instance
	limiter2 := GetRateLimiter("https://api.example.com")
	if limiter != limiter2 {
		t.Error("expected same limiter instance")
	}
}

// Config Parsing Tests

func TestParseRateLimitConfigDisabled(t *testing.T) {
	config := map[string]string{
		"rate_limit": "false",
	}

	rlConfig := ParseRateLimitConfig(config)
	if rlConfig != nil {
		t.Error("expected nil config when rate limiting is disabled")
	}
}

func TestParseRateLimitConfigEnabled(t *testing.T) {
	config := map[string]string{
		"rate_limit": "true",
	}

	rlConfig := ParseRateLimitConfig(config)
	if rlConfig == nil {
		t.Fatal("expected non-nil config when rate limiting is enabled")
	}

	// Check defaults
	if rlConfig.Rate != 10.0 {
		t.Errorf("expected default rate 10.0, got %f", rlConfig.Rate)
	}
	if rlConfig.Burst != 20 {
		t.Errorf("expected default burst 20, got %d", rlConfig.Burst)
	}
}

func TestParseRateLimitConfigCustom(t *testing.T) {
	config := map[string]string{
		"rate_limit":       "true",
		"rate_limit_rate":  "50.5",
		"rate_limit_burst": "100",
	}

	rlConfig := ParseRateLimitConfig(config)
	if rlConfig == nil {
		t.Fatal("expected non-nil config")
	}

	if rlConfig.Rate != 50.5 {
		t.Errorf("expected rate 50.5, got %f", rlConfig.Rate)
	}
	if rlConfig.Burst != 100 {
		t.Errorf("expected burst 100, got %d", rlConfig.Burst)
	}
}

func TestShouldWaitOnRateLimit(t *testing.T) {
	tests := []struct {
		config   map[string]string
		expected bool
	}{
		{map[string]string{}, true},                              // Default is wait
		{map[string]string{"rate_limit_wait": "true"}, true},     // Explicit wait
		{map[string]string{"rate_limit_wait": "false"}, false},   // Explicit no wait
		{map[string]string{"rate_limit_wait": "invalid"}, true},  // Invalid defaults to wait
	}

	for _, tt := range tests {
		result := ShouldWaitOnRateLimit(tt.config)
		if result != tt.expected {
			t.Errorf("ShouldWaitOnRateLimit(%v) = %v, want %v", tt.config, result, tt.expected)
		}
	}
}

// WithRateLimit Tests

func TestWithRateLimitNilLimiter(t *testing.T) {
	called := false
	err := WithRateLimit(context.Background(), nil, true, func() error {
		called = true
		return nil
	})

	if !called {
		t.Error("function should be called when limiter is nil")
	}
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestWithRateLimitAllowed(t *testing.T) {
	limiter := NewTokenBucket(10, 10)

	called := false
	err := WithRateLimit(context.Background(), limiter, false, func() error {
		called = true
		return nil
	})

	if !called {
		t.Error("function should be called when allowed")
	}
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestWithRateLimitDeniedNoWait(t *testing.T) {
	limiter := NewTokenBucket(10, 1)
	limiter.Allow() // Consume the one token

	called := false
	err := WithRateLimit(context.Background(), limiter, false, func() error {
		called = true
		return nil
	})

	if called {
		t.Error("function should not be called when rate limited")
	}
	if !errors.Is(err, ErrRateLimited) {
		t.Errorf("expected ErrRateLimited, got %v", err)
	}
}

func TestWithRateLimitWait(t *testing.T) {
	limiter := NewTokenBucket(100, 1)
	limiter.Allow() // Consume the one token

	called := false
	start := time.Now()
	err := WithRateLimit(context.Background(), limiter, true, func() error {
		called = true
		return nil
	})
	elapsed := time.Since(start)

	if !called {
		t.Error("function should be called after waiting")
	}
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if elapsed < 5*time.Millisecond {
		t.Errorf("expected some wait time, got %v", elapsed)
	}
}

func TestWithRateLimitWaitCancelled(t *testing.T) {
	limiter := NewTokenBucket(0.1, 1) // Very slow
	limiter.Allow()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	called := false
	err := WithRateLimit(ctx, limiter, true, func() error {
		called = true
		return nil
	})

	if called {
		t.Error("function should not be called when cancelled")
	}
	if err == nil {
		t.Error("expected error when cancelled")
	}
}

func TestWithRateLimitFunctionError(t *testing.T) {
	limiter := NewTokenBucket(10, 10)
	expectedErr := errors.New("function error")

	err := WithRateLimit(context.Background(), limiter, true, func() error {
		return expectedErr
	})

	if err != expectedErr {
		t.Errorf("expected function error, got %v", err)
	}
}

// MultiLimiter Tests

func TestMultiLimiterAllow(t *testing.T) {
	limiter1 := NewTokenBucket(10, 5)
	limiter2 := NewTokenBucket(10, 3) // More restrictive

	multi := NewMultiLimiter(limiter1, limiter2)

	// Should allow up to 3 (limited by limiter2)
	for i := 0; i < 3; i++ {
		if !multi.Allow() {
			t.Errorf("request %d should be allowed", i)
		}
	}

	// 4th should be denied
	if multi.Allow() {
		t.Error("4th request should be denied")
	}
}

func TestMultiLimiterTokens(t *testing.T) {
	limiter1 := NewTokenBucket(10, 10)
	limiter2 := NewTokenBucket(10, 5)

	multi := NewMultiLimiter(limiter1, limiter2)

	// Tokens should return minimum
	if multi.Tokens() != 5 {
		t.Errorf("expected tokens 5 (minimum), got %f", multi.Tokens())
	}
}

func TestMultiLimiterLimit(t *testing.T) {
	limiter1 := NewTokenBucket(100, 10)
	limiter2 := NewTokenBucket(50, 10)

	multi := NewMultiLimiter(limiter1, limiter2)

	// Limit should return minimum
	if multi.Limit() != 50 {
		t.Errorf("expected limit 50 (minimum), got %f", multi.Limit())
	}
}

func TestMultiLimiterBurst(t *testing.T) {
	limiter1 := NewTokenBucket(10, 100)
	limiter2 := NewTokenBucket(10, 50)

	multi := NewMultiLimiter(limiter1, limiter2)

	// Burst should return minimum
	if multi.Burst() != 50 {
		t.Errorf("expected burst 50 (minimum), got %d", multi.Burst())
	}
}

func TestMultiLimiterWait(t *testing.T) {
	limiter1 := NewTokenBucket(100, 1)
	limiter2 := NewTokenBucket(100, 1)

	multi := NewMultiLimiter(limiter1, limiter2)

	// Consume tokens from both
	multi.Allow()

	// Wait should wait for both to refill
	ctx := context.Background()
	err := multi.Wait(ctx)

	if err != nil {
		t.Errorf("Wait should succeed: %v", err)
	}
}

func TestMultiLimiterEmpty(t *testing.T) {
	multi := NewMultiLimiter()

	// Empty multi-limiter should have zero values
	if multi.Tokens() != 0 {
		t.Errorf("expected 0 tokens for empty, got %f", multi.Tokens())
	}
	if multi.Limit() != 0 {
		t.Errorf("expected 0 limit for empty, got %f", multi.Limit())
	}
	if multi.Burst() != 0 {
		t.Errorf("expected 0 burst for empty, got %d", multi.Burst())
	}
}

// DefaultRateLimitConfig Test

func TestDefaultRateLimitConfig(t *testing.T) {
	config := DefaultRateLimitConfig()

	if config.Rate != 10.0 {
		t.Errorf("expected default rate 10.0, got %f", config.Rate)
	}
	if config.Burst != 20 {
		t.Errorf("expected default burst 20, got %d", config.Burst)
	}
}

// Action Integration Tests

func TestWebhookActionWithRateLimitEnabled(t *testing.T) {
	// Reset endpoint rate limiters to ensure clean state
	ResetEndpointRateLimiters()

	requestCount := 0
	server := httpTestServer(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.WriteHeader(http.StatusOK)
	})
	defer server.Close()

	config := map[string]string{
		"url":              server.URL,
		"rate_limit":       "true",
		"rate_limit_rate":  "100", // High rate for test speed
		"rate_limit_burst": "5",
		"rate_limit_wait":  "true",
	}

	event := map[string]interface{}{"type": "test"}

	// Make several requests - all should succeed with rate limiting enabled
	for i := 0; i < 3; i++ {
		err := webhookAction(context.Background(), event, config)
		if err != nil {
			t.Errorf("request %d failed: %v", i, err)
		}
	}

	if requestCount != 3 {
		t.Errorf("expected 3 requests, got %d", requestCount)
	}
}

func TestWebhookActionWithRateLimitDenied(t *testing.T) {
	// Reset endpoint rate limiters to ensure clean state
	ResetEndpointRateLimiters()

	requestCount := 0
	server := httpTestServer(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.WriteHeader(http.StatusOK)
	})
	defer server.Close()

	config := map[string]string{
		"url":              server.URL,
		"rate_limit":       "true",
		"rate_limit_rate":  "100",
		"rate_limit_burst": "1", // Only 1 token
		"rate_limit_wait":  "false", // Fail fast, don't wait
	}

	event := map[string]interface{}{"type": "test"}

	// First request should succeed
	err := webhookAction(context.Background(), event, config)
	if err != nil {
		t.Errorf("first request failed: %v", err)
	}

	// Second request should be rate limited (fail fast mode)
	err = webhookAction(context.Background(), event, config)
	if err == nil {
		t.Error("expected rate limit error for second request")
	}
	if !errors.Is(err, ErrRateLimited) && !errors.Is(errors.Unwrap(err), ErrRateLimited) {
		// Check if error message contains rate limit info
		if err != nil && !contains(err.Error(), "rate limit") {
			t.Errorf("expected rate limit error, got: %v", err)
		}
	}

	// Only 1 request should have reached the server
	if requestCount != 1 {
		t.Errorf("expected 1 request to server, got %d", requestCount)
	}
}

func TestWebhookActionWithRateLimitDisabled(t *testing.T) {
	requestCount := 0
	server := httpTestServer(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.WriteHeader(http.StatusOK)
	})
	defer server.Close()

	// Rate limiting disabled (default)
	config := map[string]string{
		"url": server.URL,
	}

	event := map[string]interface{}{"type": "test"}

	// All requests should succeed without rate limiting
	for i := 0; i < 5; i++ {
		err := webhookAction(context.Background(), event, config)
		if err != nil {
			t.Errorf("request %d failed: %v", i, err)
		}
	}

	if requestCount != 5 {
		t.Errorf("expected 5 requests, got %d", requestCount)
	}
}

// Helper function
func httpTestServer(handler func(http.ResponseWriter, *http.Request)) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(handler))
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsAt(s, substr, 0))
}

func containsAt(s, substr string, start int) bool {
	for i := start; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
