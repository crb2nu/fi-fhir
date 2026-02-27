package workflow

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// RetryConfig holds retry behavior configuration.
type RetryConfig struct {
	// MaxRetries is the maximum number of retry attempts (0 = no retries).
	MaxRetries int

	// InitialDelay is the delay before the first retry.
	InitialDelay time.Duration

	// MaxDelay is the maximum delay between retries (caps exponential growth).
	MaxDelay time.Duration

	// Multiplier is the factor by which delay increases each retry (default 2.0).
	Multiplier float64

	// Jitter adds randomness to delays to prevent thundering herd (0.0-1.0).
	Jitter float64

	// RetryableStatusCodes are HTTP status codes that trigger a retry.
	// Default: 429, 500, 502, 503, 504
	RetryableStatusCodes []int
}

// DefaultRetryConfig returns a sensible default retry configuration.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:           3,
		InitialDelay:         1 * time.Second,
		MaxDelay:             30 * time.Second,
		Multiplier:           2.0,
		Jitter:               0.1,
		RetryableStatusCodes: []int{429, 500, 502, 503, 504},
	}
}

// ParseRetryConfig extracts retry configuration from an action config map.
// Config keys:
//   - retry_max: max number of retries (default: 3)
//   - retry_delay: initial delay duration (default: 1s)
//   - retry_max_delay: maximum delay cap (default: 30s)
//   - retry_multiplier: backoff multiplier (default: 2.0)
//   - retry_jitter: jitter factor 0.0-1.0 (default: 0.1)
//   - retry_on_status: comma-separated HTTP status codes (default: 429,500,502,503,504)
func ParseRetryConfig(config map[string]string) RetryConfig {
	rc := DefaultRetryConfig()

	// Check if retries are explicitly disabled
	if config["retry"] == "false" || config["retry_max"] == "0" {
		rc.MaxRetries = 0
		return rc
	}

	// Parse max retries
	if v := config["retry_max"]; v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			rc.MaxRetries = n
		}
	}

	// Parse initial delay
	if v := config["retry_delay"]; v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			rc.InitialDelay = d
		}
	}

	// Parse max delay
	if v := config["retry_max_delay"]; v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			rc.MaxDelay = d
		}
	}

	// Parse multiplier
	if v := config["retry_multiplier"]; v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f >= 1.0 {
			rc.Multiplier = f
		}
	}

	// Parse jitter
	if v := config["retry_jitter"]; v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f >= 0.0 && f <= 1.0 {
			rc.Jitter = f
		}
	}

	// Parse retryable status codes
	if v := config["retry_on_status"]; v != "" {
		codes := []int{}
		for _, s := range strings.Split(v, ",") {
			s = strings.TrimSpace(s)
			if n, err := strconv.Atoi(s); err == nil && n >= 100 && n < 600 {
				codes = append(codes, n)
			}
		}
		if len(codes) > 0 {
			rc.RetryableStatusCodes = codes
		}
	}

	return rc
}

// RetryableError wraps an error with retry information.
type RetryableError struct {
	Err        error
	StatusCode int
	Attempt    int
	MaxRetries int
	WillRetry  bool
}

func (e *RetryableError) Error() string {
	if e.WillRetry {
		return fmt.Sprintf("attempt %d/%d failed (will retry): %v", e.Attempt, e.MaxRetries+1, e.Err)
	}
	return fmt.Sprintf("attempt %d/%d failed (no more retries): %v", e.Attempt, e.MaxRetries+1, e.Err)
}

func (e *RetryableError) Unwrap() error {
	return e.Err
}

// IsRetryable checks if an HTTP status code should trigger a retry.
func (rc RetryConfig) IsRetryable(statusCode int) bool {
	for _, code := range rc.RetryableStatusCodes {
		if code == statusCode {
			return true
		}
	}
	return false
}

// CalculateDelay computes the delay before the next retry attempt.
// Uses exponential backoff with optional jitter.
func (rc RetryConfig) CalculateDelay(attempt int) time.Duration {
	if attempt <= 0 {
		return rc.InitialDelay
	}

	// Calculate exponential delay: initial * multiplier^attempt
	delay := float64(rc.InitialDelay) * math.Pow(rc.Multiplier, float64(attempt-1))

	// Apply jitter: delay * (1 - jitter + rand(0, 2*jitter))
	if rc.Jitter > 0 {
		jitterRange := delay * rc.Jitter * 2
		delay = delay - (delay * rc.Jitter) + (rand.Float64() * jitterRange)
	}

	// Cap at max delay
	if delay > float64(rc.MaxDelay) {
		delay = float64(rc.MaxDelay)
	}

	return time.Duration(delay)
}

// HTTPRequestFunc is a function that makes an HTTP request and returns the response.
type HTTPRequestFunc func() (*http.Response, error)

// WithRetry executes an HTTP request function with retry logic.
// It automatically retries on network errors and retryable status codes.
func WithRetry(ctx context.Context, rc RetryConfig, fn HTTPRequestFunc) (*http.Response, error) {
	var lastErr error
	var resp *http.Response

	for attempt := 1; attempt <= rc.MaxRetries+1; attempt++ {
		// Check context before attempting
		if ctx != nil {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}
		}

		// Make the request
		resp, lastErr = fn()

		// Success - no retry needed
		if lastErr == nil && resp != nil && resp.StatusCode < 400 {
			return resp, nil
		}

		// Check if we should retry
		shouldRetry := false
		statusCode := 0

		if lastErr != nil {
			// Network/connection errors are retryable
			shouldRetry = true
		} else if resp != nil {
			statusCode = resp.StatusCode
			shouldRetry = rc.IsRetryable(statusCode)

			// Close response body if we're going to retry (avoid leaking)
			if shouldRetry && resp.Body != nil {
				_ = resp.Body.Close() // closing before retry, error not actionable
			}
		}

		// If not retryable or out of retries, return the error/response
		if !shouldRetry || attempt > rc.MaxRetries {
			if lastErr == nil && resp != nil {
				// Return the response even if status is bad - let caller handle it
				return resp, nil
			}
			return resp, &RetryableError{
				Err:        lastErr,
				StatusCode: statusCode,
				Attempt:    attempt,
				MaxRetries: rc.MaxRetries,
				WillRetry:  false,
			}
		}

		// Calculate delay and wait
		delay := rc.CalculateDelay(attempt)

		// Wait with context awareness
		if ctx != nil {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		} else {
			time.Sleep(delay)
		}
	}

	// Should not reach here, but return last error if we do
	return resp, lastErr
}

// SimpleRetryFunc is a function that can be retried.
type SimpleRetryFunc func() error

// WithSimpleRetry executes a function with retry logic (for non-HTTP operations).
func WithSimpleRetry(ctx context.Context, rc RetryConfig, fn SimpleRetryFunc) error {
	var lastErr error

	for attempt := 1; attempt <= rc.MaxRetries+1; attempt++ {
		// Check context before attempting
		if ctx != nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
		}

		// Execute the function
		lastErr = fn()

		// Success - no retry needed
		if lastErr == nil {
			return nil
		}

		// If out of retries, return the error
		if attempt > rc.MaxRetries {
			return &RetryableError{
				Err:        lastErr,
				Attempt:    attempt,
				MaxRetries: rc.MaxRetries,
				WillRetry:  false,
			}
		}

		// Calculate delay and wait
		delay := rc.CalculateDelay(attempt)

		// Wait with context awareness
		if ctx != nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		} else {
			time.Sleep(delay)
		}
	}

	return lastErr
}
