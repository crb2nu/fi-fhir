//nolint:bodyclose // Mock responses in tests don't require body closing
package workflow

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestCircuitBreakerInitialState(t *testing.T) {
	cb := NewCircuitBreaker(DefaultCircuitBreakerConfig())

	if cb.State() != CircuitClosed {
		t.Errorf("expected initial state to be closed, got %v", cb.State())
	}

	stats := cb.Stats()
	if stats.Failures != 0 || stats.Successes != 0 {
		t.Errorf("expected zero counters, got failures=%d successes=%d", stats.Failures, stats.Successes)
	}
}

func TestCircuitBreakerStateString(t *testing.T) {
	tests := []struct {
		state    CircuitState
		expected string
	}{
		{CircuitClosed, "closed"},
		{CircuitOpen, "open"},
		{CircuitHalfOpen, "half-open"},
		{CircuitState(99), "unknown"},
	}

	for _, tt := range tests {
		if got := tt.state.String(); got != tt.expected {
			t.Errorf("CircuitState(%d).String() = %q, want %q", tt.state, got, tt.expected)
		}
	}
}

func TestCircuitBreakerAllowInClosedState(t *testing.T) {
	cb := NewCircuitBreaker(DefaultCircuitBreakerConfig())

	// All requests should be allowed in closed state
	for i := 0; i < 10; i++ {
		if !cb.Allow() {
			t.Errorf("request %d should be allowed in closed state", i)
		}
	}
}

func TestCircuitBreakerTransitionToOpen(t *testing.T) {
	config := CircuitBreakerConfig{
		FailureThreshold: 3,
		SuccessThreshold: 2,
		Timeout:          1 * time.Second,
	}
	cb := NewCircuitBreaker(config)

	// Record failures up to threshold
	for i := 0; i < 3; i++ {
		cb.RecordFailure()
	}

	// Should now be open
	if cb.State() != CircuitOpen {
		t.Errorf("expected circuit to be open after %d failures, got %v", 3, cb.State())
	}

	// Requests should be rejected
	if cb.Allow() {
		t.Error("requests should be rejected when circuit is open")
	}

	stats := cb.Stats()
	if stats.TotalRejected != 1 {
		t.Errorf("expected 1 rejected request, got %d", stats.TotalRejected)
	}
}

func TestCircuitBreakerSuccessResetsFailures(t *testing.T) {
	config := CircuitBreakerConfig{
		FailureThreshold: 3,
		SuccessThreshold: 2,
		Timeout:          1 * time.Second,
	}
	cb := NewCircuitBreaker(config)

	// Record some failures (but not enough to open)
	cb.RecordFailure()
	cb.RecordFailure()

	// A success should reset the failure counter
	cb.RecordSuccess()

	// Now more failures shouldn't immediately open the circuit
	cb.RecordFailure()
	cb.RecordFailure()

	// Still closed because counter was reset
	if cb.State() != CircuitClosed {
		t.Errorf("expected circuit to remain closed, got %v", cb.State())
	}

	// One more failure should now open it (3 consecutive)
	cb.RecordFailure()
	if cb.State() != CircuitOpen {
		t.Errorf("expected circuit to be open, got %v", cb.State())
	}
}

func TestCircuitBreakerTimeoutTransition(t *testing.T) {
	config := CircuitBreakerConfig{
		FailureThreshold: 2,
		SuccessThreshold: 1,
		Timeout:          50 * time.Millisecond, // Short timeout for test
	}
	cb := NewCircuitBreaker(config)

	// Open the circuit
	cb.RecordFailure()
	cb.RecordFailure()

	if cb.State() != CircuitOpen {
		t.Fatalf("expected circuit to be open, got %v", cb.State())
	}

	// Wait for timeout
	time.Sleep(60 * time.Millisecond)

	// State should now be half-open (lazy transition on next check)
	if cb.State() != CircuitHalfOpen {
		t.Errorf("expected circuit to be half-open after timeout, got %v", cb.State())
	}

	// Allow should now permit a request
	if !cb.Allow() {
		t.Error("one request should be allowed in half-open state")
	}
}

func TestCircuitBreakerHalfOpenToClosedTransition(t *testing.T) {
	config := CircuitBreakerConfig{
		FailureThreshold: 2,
		SuccessThreshold: 2,
		Timeout:          10 * time.Millisecond,
	}
	cb := NewCircuitBreaker(config)

	// Open the circuit
	cb.RecordFailure()
	cb.RecordFailure()

	// Wait for half-open
	time.Sleep(20 * time.Millisecond)
	cb.Allow() // Trigger transition to half-open

	// Record successes to reach threshold
	cb.RecordSuccess()
	if cb.State() != CircuitHalfOpen {
		t.Errorf("expected circuit to still be half-open after 1 success, got %v", cb.State())
	}

	cb.RecordSuccess()
	if cb.State() != CircuitClosed {
		t.Errorf("expected circuit to be closed after 2 successes, got %v", cb.State())
	}

	// Failure counter should be reset
	stats := cb.Stats()
	if stats.Failures != 0 {
		t.Errorf("expected failures to be reset to 0, got %d", stats.Failures)
	}
}

func TestCircuitBreakerHalfOpenToOpenTransition(t *testing.T) {
	config := CircuitBreakerConfig{
		FailureThreshold: 2,
		SuccessThreshold: 2,
		Timeout:          10 * time.Millisecond,
	}
	cb := NewCircuitBreaker(config)

	// Open the circuit
	cb.RecordFailure()
	cb.RecordFailure()

	// Wait for half-open
	time.Sleep(20 * time.Millisecond)
	cb.Allow() // Trigger transition to half-open

	// A failure in half-open should immediately reopen
	cb.RecordFailure()

	if cb.State() != CircuitOpen {
		t.Errorf("expected circuit to be open after failure in half-open, got %v", cb.State())
	}
}

func TestCircuitBreakerIsFailure(t *testing.T) {
	cb := NewCircuitBreaker(DefaultCircuitBreakerConfig())

	tests := []struct {
		statusCode int
		expected   bool
	}{
		{200, false},
		{201, false},
		{400, false},
		{401, false},
		{404, false},
		{500, true},
		{502, true},
		{503, true},
		{504, true},
		{505, false}, // Not in default list
	}

	for _, tt := range tests {
		if got := cb.IsFailure(tt.statusCode); got != tt.expected {
			t.Errorf("IsFailure(%d) = %v, want %v", tt.statusCode, got, tt.expected)
		}
	}
}

func TestCircuitBreakerCustomFailureCodes(t *testing.T) {
	config := CircuitBreakerConfig{
		FailureThreshold:   5,
		SuccessThreshold:   2,
		Timeout:            30 * time.Second,
		FailureStatusCodes: []int{429, 500, 503}, // Custom codes including rate limit
	}
	cb := NewCircuitBreaker(config)

	if !cb.IsFailure(429) {
		t.Error("expected 429 to be a failure with custom codes")
	}
	if cb.IsFailure(502) {
		t.Error("expected 502 to NOT be a failure with custom codes")
	}
}

func TestCircuitBreakerReset(t *testing.T) {
	config := CircuitBreakerConfig{
		FailureThreshold: 2,
		SuccessThreshold: 1,
		Timeout:          1 * time.Second,
	}
	cb := NewCircuitBreaker(config)

	// Open the circuit
	cb.RecordFailure()
	cb.RecordFailure()

	if cb.State() != CircuitOpen {
		t.Fatalf("expected circuit to be open, got %v", cb.State())
	}

	// Reset should return to closed state
	cb.Reset()

	if cb.State() != CircuitClosed {
		t.Errorf("expected circuit to be closed after reset, got %v", cb.State())
	}

	stats := cb.Stats()
	if stats.Failures != 0 || stats.Successes != 0 {
		t.Errorf("expected counters to be reset, got failures=%d successes=%d", stats.Failures, stats.Successes)
	}
}

func TestCircuitBreakerConfigDefaults(t *testing.T) {
	// Test with zero/invalid config values
	config := CircuitBreakerConfig{
		FailureThreshold: 0,  // Invalid, should default to 5
		SuccessThreshold: -1, // Invalid, should default to 2
		Timeout:          0,  // Invalid, should default to 30s
	}
	cb := NewCircuitBreaker(config)

	// Record 5 failures (the default threshold)
	for i := 0; i < 5; i++ {
		cb.RecordFailure()
	}

	if cb.State() != CircuitOpen {
		t.Errorf("expected circuit to open at default threshold 5, got state %v", cb.State())
	}

	// Default failure codes should be set
	if !cb.IsFailure(500) {
		t.Error("expected default failure codes to include 500")
	}
}

func TestCircuitBreakerStats(t *testing.T) {
	cb := NewCircuitBreaker(DefaultCircuitBreakerConfig())

	// Record some activity
	cb.RecordSuccess()
	cb.RecordSuccess()
	cb.RecordFailure()

	stats := cb.Stats()
	if stats.TotalSuccesses != 2 {
		t.Errorf("expected TotalSuccesses=2, got %d", stats.TotalSuccesses)
	}
	if stats.TotalFailures != 1 {
		t.Errorf("expected TotalFailures=1, got %d", stats.TotalFailures)
	}
	if stats.State != CircuitClosed {
		t.Errorf("expected State=closed, got %v", stats.State)
	}
}

// Registry tests

func TestCircuitBreakerRegistryCreateNew(t *testing.T) {
	registry := NewCircuitBreakerRegistry()

	cb1 := registry.GetBreaker("endpoint1")
	cb2 := registry.GetBreaker("endpoint2")

	if cb1 == nil || cb2 == nil {
		t.Fatal("expected non-nil circuit breakers")
	}

	if cb1 == cb2 {
		t.Error("expected different circuit breakers for different keys")
	}
}

func TestCircuitBreakerRegistryReusesExisting(t *testing.T) {
	registry := NewCircuitBreakerRegistry()

	cb1 := registry.GetBreaker("endpoint1")
	cb2 := registry.GetBreaker("endpoint1")

	if cb1 != cb2 {
		t.Error("expected same circuit breaker for same key")
	}
}

func TestCircuitBreakerRegistryWithConfig(t *testing.T) {
	config := CircuitBreakerConfig{
		FailureThreshold: 10,
		SuccessThreshold: 5,
		Timeout:          1 * time.Minute,
	}
	registry := NewCircuitBreakerRegistryWithConfig(config)

	cb := registry.GetBreaker("test")

	// Record 10 failures (matches custom threshold)
	for i := 0; i < 10; i++ {
		cb.RecordFailure()
	}

	if cb.State() != CircuitOpen {
		t.Errorf("expected circuit to open at custom threshold 10, got %v", cb.State())
	}
}

func TestCircuitBreakerRegistryAllStats(t *testing.T) {
	registry := NewCircuitBreakerRegistry()

	cb1 := registry.GetBreaker("endpoint1")
	cb2 := registry.GetBreaker("endpoint2")

	cb1.RecordSuccess()
	cb2.RecordFailure()

	stats := registry.AllStats()

	if len(stats) != 2 {
		t.Errorf("expected 2 stats entries, got %d", len(stats))
	}

	if stats["endpoint1"].TotalSuccesses != 1 {
		t.Errorf("endpoint1 should have 1 success")
	}
	if stats["endpoint2"].TotalFailures != 1 {
		t.Errorf("endpoint2 should have 1 failure")
	}
}

func TestGlobalCircuitBreaker(t *testing.T) {
	// Reset global registry for clean test
	globalCircuitBreakerRegistry = NewCircuitBreakerRegistry()

	cb := GetCircuitBreaker("https://api.example.com/fhir")

	if cb == nil {
		t.Fatal("expected non-nil circuit breaker from global function")
	}

	// Should return same instance
	cb2 := GetCircuitBreaker("https://api.example.com/fhir")
	if cb != cb2 {
		t.Error("expected same circuit breaker instance")
	}
}

// ParseCircuitBreakerConfig tests

func TestParseCircuitBreakerConfigDisabled(t *testing.T) {
	config := map[string]string{
		"circuit_breaker": "false",
	}

	cbConfig := ParseCircuitBreakerConfig(config)
	if cbConfig != nil {
		t.Error("expected nil config when circuit breaker is disabled")
	}
}

func TestParseCircuitBreakerConfigEnabled(t *testing.T) {
	config := map[string]string{
		"circuit_breaker": "true",
	}

	cbConfig := ParseCircuitBreakerConfig(config)
	if cbConfig == nil {
		t.Fatal("expected non-nil config when circuit breaker is enabled")
	}

	// Check defaults are applied
	if cbConfig.FailureThreshold != 5 {
		t.Errorf("expected default FailureThreshold=5, got %d", cbConfig.FailureThreshold)
	}
	if cbConfig.SuccessThreshold != 2 {
		t.Errorf("expected default SuccessThreshold=2, got %d", cbConfig.SuccessThreshold)
	}
	if cbConfig.Timeout != 30*time.Second {
		t.Errorf("expected default Timeout=30s, got %v", cbConfig.Timeout)
	}
}

func TestParseCircuitBreakerConfigCustomValues(t *testing.T) {
	config := map[string]string{
		"circuit_breaker":           "true",
		"circuit_failure_threshold": "10",
		"circuit_success_threshold": "3",
		"circuit_timeout":           "1m",
	}

	cbConfig := ParseCircuitBreakerConfig(config)
	if cbConfig == nil {
		t.Fatal("expected non-nil config")
	}

	if cbConfig.FailureThreshold != 10 {
		t.Errorf("expected FailureThreshold=10, got %d", cbConfig.FailureThreshold)
	}
	if cbConfig.SuccessThreshold != 3 {
		t.Errorf("expected SuccessThreshold=3, got %d", cbConfig.SuccessThreshold)
	}
	if cbConfig.Timeout != 1*time.Minute {
		t.Errorf("expected Timeout=1m, got %v", cbConfig.Timeout)
	}
}

func TestParseCircuitBreakerConfigInvalidValues(t *testing.T) {
	config := map[string]string{
		"circuit_breaker":           "true",
		"circuit_failure_threshold": "invalid",
		"circuit_success_threshold": "-1",
		"circuit_timeout":           "not-a-duration",
	}

	cbConfig := ParseCircuitBreakerConfig(config)
	if cbConfig == nil {
		t.Fatal("expected non-nil config even with invalid values")
	}

	// Should use defaults when values are invalid
	if cbConfig.FailureThreshold != 5 {
		t.Errorf("expected default FailureThreshold=5 for invalid input, got %d", cbConfig.FailureThreshold)
	}
	if cbConfig.SuccessThreshold != 2 {
		t.Errorf("expected default SuccessThreshold=2 for invalid input, got %d", cbConfig.SuccessThreshold)
	}
	if cbConfig.Timeout != 30*time.Second {
		t.Errorf("expected default Timeout=30s for invalid input, got %v", cbConfig.Timeout)
	}
}

// WithCircuitBreaker wrapper tests

func TestWithCircuitBreakerNilBreaker(t *testing.T) {
	called := false
	resp, err := WithCircuitBreaker(nil, func() (*http.Response, error) {
		called = true
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(""))}, nil
	})

	if !called {
		t.Error("expected function to be called when circuit breaker is nil")
	}
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestWithCircuitBreakerOpenCircuit(t *testing.T) {
	config := CircuitBreakerConfig{
		FailureThreshold: 1,
		SuccessThreshold: 1,
		Timeout:          1 * time.Hour, // Long timeout
	}
	cb := NewCircuitBreaker(config)
	cb.RecordFailure() // Open the circuit

	called := false
	resp, err := WithCircuitBreaker(cb, func() (*http.Response, error) {
		called = true
		return &http.Response{StatusCode: 200}, nil
	})

	if called {
		t.Error("function should not be called when circuit is open")
	}
	if !errors.Is(err, ErrCircuitOpen) {
		t.Errorf("expected ErrCircuitOpen, got %v", err)
	}
	if resp != nil {
		t.Error("expected nil response when circuit is open")
	}
}

func TestWithCircuitBreakerRecordsSuccess(t *testing.T) {
	cb := NewCircuitBreaker(DefaultCircuitBreakerConfig())

	resp, err := WithCircuitBreaker(cb, func() (*http.Response, error) {
		return &http.Response{StatusCode: 200}, nil
	})

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	stats := cb.Stats()
	if stats.TotalSuccesses != 1 {
		t.Errorf("expected 1 success recorded, got %d", stats.TotalSuccesses)
	}
}

func TestWithCircuitBreakerRecordsFailureOnError(t *testing.T) {
	cb := NewCircuitBreaker(DefaultCircuitBreakerConfig())

	testErr := errors.New("connection refused")
	resp, err := WithCircuitBreaker(cb, func() (*http.Response, error) {
		return nil, testErr
	})

	if !errors.Is(err, testErr) {
		t.Errorf("expected original error, got %v", err)
	}
	if resp != nil {
		t.Error("expected nil response on error")
	}

	stats := cb.Stats()
	if stats.TotalFailures != 1 {
		t.Errorf("expected 1 failure recorded, got %d", stats.TotalFailures)
	}
}

func TestWithCircuitBreakerRecordsFailureOnStatusCode(t *testing.T) {
	cb := NewCircuitBreaker(DefaultCircuitBreakerConfig())

	resp, err := WithCircuitBreaker(cb, func() (*http.Response, error) {
		return &http.Response{StatusCode: 503}, nil
	})

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if resp.StatusCode != 503 {
		t.Errorf("expected status 503, got %d", resp.StatusCode)
	}

	stats := cb.Stats()
	if stats.TotalFailures != 1 {
		t.Errorf("expected 1 failure recorded for 503 status, got %d", stats.TotalFailures)
	}
}

func TestWithCircuitBreakerOpensAfterThreshold(t *testing.T) {
	config := CircuitBreakerConfig{
		FailureThreshold: 2,
		SuccessThreshold: 1,
		Timeout:          1 * time.Hour,
	}
	cb := NewCircuitBreaker(config)

	// First two requests fail with 500
	for i := 0; i < 2; i++ {
		WithCircuitBreaker(cb, func() (*http.Response, error) {
			return &http.Response{StatusCode: 500}, nil
		})
	}

	// Third request should be rejected
	_, err := WithCircuitBreaker(cb, func() (*http.Response, error) {
		return &http.Response{StatusCode: 200}, nil
	})

	if !errors.Is(err, ErrCircuitOpen) {
		t.Errorf("expected ErrCircuitOpen after threshold, got %v", err)
	}
}

// Concurrency test

func TestCircuitBreakerConcurrency(t *testing.T) {
	cb := NewCircuitBreaker(DefaultCircuitBreakerConfig())
	done := make(chan bool)

	// Spawn goroutines that hammer the circuit breaker
	for i := 0; i < 100; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				cb.Allow()
				cb.RecordSuccess()
				cb.RecordFailure()
				cb.State()
				cb.Stats()
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 100; i++ {
		<-done
	}

	// If we got here without panic or race detector issues, the test passes
}
