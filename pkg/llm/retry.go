package llm

import (
	"context"
	"math"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"
)

// RetryConfig configures the retry behavior.
type RetryConfig struct {
	// MaxRetries is the maximum number of retry attempts.
	// 0 means no retries.
	MaxRetries int

	// BaseDelay is the initial delay between retries.
	BaseDelay time.Duration

	// MaxDelay is the maximum delay between retries.
	MaxDelay time.Duration

	// Jitter adds randomness to delays to prevent thundering herd.
	// 0.0 = no jitter, 1.0 = full jitter (0 to 2x delay)
	Jitter float64
}

// DefaultRetryConfig returns a default retry configuration.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries: 3,
		BaseDelay:  100 * time.Millisecond,
		MaxDelay:   5 * time.Second,
		Jitter:     0.2,
	}
}

// Retryer implements exponential backoff with jitter.
type Retryer struct {
	config RetryConfig

	// Metrics
	totalRequests   int64
	failedRequests  int64
	retriedRequests int64

	// Circuit breaker state
	mu              sync.RWMutex
	failures        int
	lastFailure     time.Time
	circuitOpen     bool
	circuitOpenedAt time.Time

	// Circuit breaker config
	failureThreshold int
	resetTimeout     time.Duration
	halfOpenRequests int
}

// NewRetryer creates a new Retryer with the given configuration.
func NewRetryer(config RetryConfig) *Retryer {
	if config.BaseDelay == 0 {
		config.BaseDelay = 100 * time.Millisecond
	}
	if config.MaxDelay == 0 {
		config.MaxDelay = 5 * time.Second
	}

	return &Retryer{
		config:           config,
		failureThreshold: 5,                // Open circuit after 5 consecutive failures
		resetTimeout:     30 * time.Second, // Try to reset after 30 seconds
	}
}

// RetryableError wraps an error to indicate it should be retried.
type RetryableError struct {
	Err error
}

func (e *RetryableError) Error() string {
	return e.Err.Error()
}

func (e *RetryableError) Unwrap() error {
	return e.Err
}

// IsRetryable returns true if the error should be retried.
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*RetryableError)
	return ok
}

// Do executes the given function with retries.
// The function should return a *RetryableError if the error is retryable.
func (r *Retryer) Do(ctx context.Context, fn func() error) error {
	atomic.AddInt64(&r.totalRequests, 1)

	// Check circuit breaker
	if r.isCircuitOpen() {
		atomic.AddInt64(&r.failedRequests, 1)
		return ErrCircuitOpen
	}

	var lastErr error
	for attempt := 0; attempt <= r.config.MaxRetries; attempt++ {
		// Check context before each attempt
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// Execute the function
		err := fn()
		if err == nil {
			r.recordSuccess()
			return nil
		}

		lastErr = err

		// Check if error is retryable
		if !IsRetryable(err) {
			r.recordFailure()
			atomic.AddInt64(&r.failedRequests, 1)
			return unwrapRetryable(err)
		}

		// Don't retry if this was the last attempt
		if attempt >= r.config.MaxRetries {
			break
		}

		atomic.AddInt64(&r.retriedRequests, 1)

		// Calculate backoff delay
		delay := r.backoffDelay(attempt)

		// Wait with context cancellation support
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}

	// All retries exhausted
	r.recordFailure()
	atomic.AddInt64(&r.failedRequests, 1)
	return unwrapRetryable(lastErr)
}

// DoWithRetryable is a convenience method that wraps network-like errors as retryable.
func (r *Retryer) DoWithRetryable(ctx context.Context, fn func() error) error {
	return r.Do(ctx, func() error {
		err := fn()
		if err != nil && isNetworkError(err) {
			return &RetryableError{Err: err}
		}
		return err
	})
}

// backoffDelay calculates the delay for a given attempt using exponential backoff.
func (r *Retryer) backoffDelay(attempt int) time.Duration {
	// Exponential backoff: baseDelay * 2^attempt
	delay := float64(r.config.BaseDelay) * math.Pow(2, float64(attempt))

	// Apply jitter
	if r.config.Jitter > 0 {
		jitter := delay * r.config.Jitter * (rand.Float64()*2 - 1)
		delay += jitter
	}

	// Clamp to max delay
	if delay > float64(r.config.MaxDelay) {
		delay = float64(r.config.MaxDelay)
	}

	// Ensure non-negative
	if delay < 0 {
		delay = 0
	}

	return time.Duration(delay)
}

// Circuit breaker methods

func (r *Retryer) isCircuitOpen() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if !r.circuitOpen {
		return false
	}

	// Check if we should try half-open
	if time.Since(r.circuitOpenedAt) > r.resetTimeout {
		return false // Allow one request through (half-open)
	}

	return true
}

func (r *Retryer) recordSuccess() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.failures = 0
	r.circuitOpen = false
}

func (r *Retryer) recordFailure() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.failures++
	r.lastFailure = time.Now()

	if r.failures >= r.failureThreshold && !r.circuitOpen {
		r.circuitOpen = true
		r.circuitOpenedAt = time.Now()
	}
}

// Metrics accessors

// TotalRequests returns the total number of requests made.
func (r *Retryer) TotalRequests() int64 {
	return atomic.LoadInt64(&r.totalRequests)
}

// FailedRequests returns the number of failed requests.
func (r *Retryer) FailedRequests() int64 {
	return atomic.LoadInt64(&r.failedRequests)
}

// RetriedRequests returns the number of retried requests.
func (r *Retryer) RetriedRequests() int64 {
	return atomic.LoadInt64(&r.retriedRequests)
}

// CircuitState returns the current circuit breaker state.
func (r *Retryer) CircuitState() string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if !r.circuitOpen {
		return "closed"
	}

	if time.Since(r.circuitOpenedAt) > r.resetTimeout {
		return "half-open"
	}

	return "open"
}

// ResetCircuit manually resets the circuit breaker.
func (r *Retryer) ResetCircuit() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.failures = 0
	r.circuitOpen = false
}

// Helper functions

func unwrapRetryable(err error) error {
	if re, ok := err.(*RetryableError); ok {
		return re.Err
	}
	return err
}

// isNetworkError checks if an error is likely a network-related error.
func isNetworkError(err error) bool {
	if err == nil {
		return false
	}

	// Check for common network error patterns in the error string
	errStr := err.Error()
	networkPatterns := []string{
		"connection refused",
		"connection reset",
		"no such host",
		"timeout",
		"temporary failure",
		"network is unreachable",
		"EOF",
		"broken pipe",
		"connection timed out",
	}

	for _, pattern := range networkPatterns {
		if contains(errStr, pattern) {
			return true
		}
	}

	return false
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
