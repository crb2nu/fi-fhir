package workflow

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDefaultRetryConfig(t *testing.T) {
	rc := DefaultRetryConfig()

	if rc.MaxRetries != 3 {
		t.Errorf("Expected MaxRetries 3, got %d", rc.MaxRetries)
	}
	if rc.InitialDelay != 1*time.Second {
		t.Errorf("Expected InitialDelay 1s, got %v", rc.InitialDelay)
	}
	if rc.MaxDelay != 30*time.Second {
		t.Errorf("Expected MaxDelay 30s, got %v", rc.MaxDelay)
	}
	if rc.Multiplier != 2.0 {
		t.Errorf("Expected Multiplier 2.0, got %v", rc.Multiplier)
	}
	if rc.Jitter != 0.1 {
		t.Errorf("Expected Jitter 0.1, got %v", rc.Jitter)
	}
	expectedCodes := []int{429, 500, 502, 503, 504}
	if len(rc.RetryableStatusCodes) != len(expectedCodes) {
		t.Errorf("Expected %d status codes, got %d", len(expectedCodes), len(rc.RetryableStatusCodes))
	}
}

func TestParseRetryConfig(t *testing.T) {
	tests := []struct {
		name           string
		config         map[string]string
		expectedMax    int
		expectedDelay  time.Duration
		expectedMult   float64
		expectedJitter float64
	}{
		{
			name:           "default config",
			config:         map[string]string{},
			expectedMax:    3,
			expectedDelay:  1 * time.Second,
			expectedMult:   2.0,
			expectedJitter: 0.1,
		},
		{
			name: "custom max retries",
			config: map[string]string{
				"retry_max": "5",
			},
			expectedMax:    5,
			expectedDelay:  1 * time.Second,
			expectedMult:   2.0,
			expectedJitter: 0.1,
		},
		{
			name: "custom delay",
			config: map[string]string{
				"retry_delay": "2s",
			},
			expectedMax:    3,
			expectedDelay:  2 * time.Second,
			expectedMult:   2.0,
			expectedJitter: 0.1,
		},
		{
			name: "custom multiplier",
			config: map[string]string{
				"retry_multiplier": "3.0",
			},
			expectedMax:    3,
			expectedDelay:  1 * time.Second,
			expectedMult:   3.0,
			expectedJitter: 0.1,
		},
		{
			name: "custom jitter",
			config: map[string]string{
				"retry_jitter": "0.2",
			},
			expectedMax:    3,
			expectedDelay:  1 * time.Second,
			expectedMult:   2.0,
			expectedJitter: 0.2,
		},
		{
			name: "disabled retries via retry=false",
			config: map[string]string{
				"retry": "false",
			},
			expectedMax:    0,
			expectedDelay:  1 * time.Second,
			expectedMult:   2.0,
			expectedJitter: 0.1,
		},
		{
			name: "disabled retries via retry_max=0",
			config: map[string]string{
				"retry_max": "0",
			},
			expectedMax:    0,
			expectedDelay:  1 * time.Second,
			expectedMult:   2.0,
			expectedJitter: 0.1,
		},
		{
			name: "all custom values",
			config: map[string]string{
				"retry_max":        "10",
				"retry_delay":      "500ms",
				"retry_max_delay":  "1m",
				"retry_multiplier": "1.5",
				"retry_jitter":     "0.05",
			},
			expectedMax:    10,
			expectedDelay:  500 * time.Millisecond,
			expectedMult:   1.5,
			expectedJitter: 0.05,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rc := ParseRetryConfig(tt.config)

			if rc.MaxRetries != tt.expectedMax {
				t.Errorf("MaxRetries: expected %d, got %d", tt.expectedMax, rc.MaxRetries)
			}
			if rc.InitialDelay != tt.expectedDelay {
				t.Errorf("InitialDelay: expected %v, got %v", tt.expectedDelay, rc.InitialDelay)
			}
			if rc.Multiplier != tt.expectedMult {
				t.Errorf("Multiplier: expected %v, got %v", tt.expectedMult, rc.Multiplier)
			}
			if rc.Jitter != tt.expectedJitter {
				t.Errorf("Jitter: expected %v, got %v", tt.expectedJitter, rc.Jitter)
			}
		})
	}
}

func TestParseRetryConfigStatusCodes(t *testing.T) {
	tests := []struct {
		name     string
		config   map[string]string
		expected []int
	}{
		{
			name:     "default codes",
			config:   map[string]string{},
			expected: []int{429, 500, 502, 503, 504},
		},
		{
			name: "custom codes",
			config: map[string]string{
				"retry_on_status": "408,429,503",
			},
			expected: []int{408, 429, 503},
		},
		{
			name: "codes with spaces",
			config: map[string]string{
				"retry_on_status": "429, 500, 503",
			},
			expected: []int{429, 500, 503},
		},
		{
			name: "invalid codes ignored",
			config: map[string]string{
				"retry_on_status": "429,invalid,500",
			},
			expected: []int{429, 500},
		},
		{
			name: "out of range codes ignored",
			config: map[string]string{
				"retry_on_status": "50,429,700",
			},
			expected: []int{429},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rc := ParseRetryConfig(tt.config)

			if len(rc.RetryableStatusCodes) != len(tt.expected) {
				t.Errorf("Expected %d codes, got %d: %v", len(tt.expected), len(rc.RetryableStatusCodes), rc.RetryableStatusCodes)
				return
			}

			for i, code := range tt.expected {
				if rc.RetryableStatusCodes[i] != code {
					t.Errorf("Code[%d]: expected %d, got %d", i, code, rc.RetryableStatusCodes[i])
				}
			}
		})
	}
}

func TestIsRetryable(t *testing.T) {
	rc := RetryConfig{
		RetryableStatusCodes: []int{429, 500, 502, 503, 504},
	}

	tests := []struct {
		code     int
		expected bool
	}{
		{200, false},
		{400, false},
		{401, false},
		{404, false},
		{429, true},
		{500, true},
		{501, false},
		{502, true},
		{503, true},
		{504, true},
	}

	for _, tt := range tests {
		t.Run(http.StatusText(tt.code), func(t *testing.T) {
			result := rc.IsRetryable(tt.code)
			if result != tt.expected {
				t.Errorf("IsRetryable(%d): expected %v, got %v", tt.code, tt.expected, result)
			}
		})
	}
}

func TestCalculateDelay(t *testing.T) {
	rc := RetryConfig{
		InitialDelay: 1 * time.Second,
		MaxDelay:     30 * time.Second,
		Multiplier:   2.0,
		Jitter:       0, // No jitter for deterministic testing
	}

	tests := []struct {
		attempt  int
		expected time.Duration
	}{
		{0, 1 * time.Second},  // Initial delay
		{1, 1 * time.Second},  // First retry: 1s * 2^0 = 1s
		{2, 2 * time.Second},  // Second retry: 1s * 2^1 = 2s
		{3, 4 * time.Second},  // Third retry: 1s * 2^2 = 4s
		{4, 8 * time.Second},  // Fourth retry: 1s * 2^3 = 8s
		{5, 16 * time.Second}, // Fifth retry: 1s * 2^4 = 16s
	}

	for _, tt := range tests {
		t.Run(string(rune('0'+tt.attempt)), func(t *testing.T) {
			delay := rc.CalculateDelay(tt.attempt)
			if delay != tt.expected {
				t.Errorf("CalculateDelay(%d): expected %v, got %v", tt.attempt, tt.expected, delay)
			}
		})
	}
}

func TestCalculateDelayMaxCap(t *testing.T) {
	rc := RetryConfig{
		InitialDelay: 1 * time.Second,
		MaxDelay:     5 * time.Second,
		Multiplier:   2.0,
		Jitter:       0,
	}

	// After a few retries, delay should be capped at MaxDelay
	delay := rc.CalculateDelay(10) // Would be 512s without cap
	if delay != 5*time.Second {
		t.Errorf("Expected delay capped at 5s, got %v", delay)
	}
}

func TestCalculateDelayWithJitter(t *testing.T) {
	rc := RetryConfig{
		InitialDelay: 1 * time.Second,
		MaxDelay:     30 * time.Second,
		Multiplier:   2.0,
		Jitter:       0.5, // 50% jitter
	}

	// With 50% jitter, delay should be between 0.5x and 1.5x the base
	baseDelay := 1 * time.Second

	// Run multiple times to verify randomness
	for i := 0; i < 100; i++ {
		delay := rc.CalculateDelay(1)
		minExpected := time.Duration(float64(baseDelay) * 0.5)
		maxExpected := time.Duration(float64(baseDelay) * 1.5)

		if delay < minExpected || delay > maxExpected {
			t.Errorf("Delay %v outside expected range [%v, %v]", delay, minExpected, maxExpected)
		}
	}
}

func TestRetryableError(t *testing.T) {
	err := &RetryableError{
		Err:        errors.New("connection refused"),
		StatusCode: 503,
		Attempt:    2,
		MaxRetries: 3,
		WillRetry:  true,
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "attempt 2/4") {
		t.Errorf("Error should mention attempt count, got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "will retry") {
		t.Errorf("Error should mention will retry, got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "connection refused") {
		t.Errorf("Error should contain original error, got: %s", errMsg)
	}

	// Test unwrap
	unwrapped := err.Unwrap()
	if unwrapped.Error() != "connection refused" {
		t.Errorf("Unwrap should return original error, got: %v", unwrapped)
	}
}

func TestWithRetrySuccess(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	}))
	defer server.Close()

	rc := RetryConfig{
		MaxRetries:           3,
		InitialDelay:         10 * time.Millisecond,
		RetryableStatusCodes: []int{500},
	}

	resp, err := WithRetry(nil, rc, func() (*http.Response, error) {
		return http.Get(server.URL)
	})

	if err != nil {
		t.Fatalf("WithRetry failed: %v", err)
	}
	defer resp.Body.Close()

	if callCount != 1 {
		t.Errorf("Expected 1 call (success), got %d", callCount)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected 200, got %d", resp.StatusCode)
	}
}

func TestWithRetryEventualSuccess(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	rc := RetryConfig{
		MaxRetries:           5,
		InitialDelay:         1 * time.Millisecond,
		MaxDelay:             10 * time.Millisecond,
		Multiplier:           1.5,
		Jitter:               0,
		RetryableStatusCodes: []int{503},
	}

	resp, err := WithRetry(nil, rc, func() (*http.Response, error) {
		return http.Get(server.URL)
	})

	if err != nil {
		t.Fatalf("WithRetry failed: %v", err)
	}
	defer resp.Body.Close()

	if callCount != 3 {
		t.Errorf("Expected 3 calls (2 retries then success), got %d", callCount)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected 200, got %d", resp.StatusCode)
	}
}

func TestWithRetryMaxRetriesExhausted(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	rc := RetryConfig{
		MaxRetries:           2,
		InitialDelay:         1 * time.Millisecond,
		MaxDelay:             10 * time.Millisecond,
		Multiplier:           2.0,
		Jitter:               0,
		RetryableStatusCodes: []int{503},
	}

	resp, err := WithRetry(nil, rc, func() (*http.Response, error) {
		return http.Get(server.URL)
	})

	// Should return the response (not error) when max retries exceeded
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if resp == nil {
		t.Fatal("Expected response to be returned")
	}
	defer resp.Body.Close()

	// 1 initial + 2 retries = 3 total calls
	if callCount != 3 {
		t.Errorf("Expected 3 calls (initial + 2 retries), got %d", callCount)
	}
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("Expected 503, got %d", resp.StatusCode)
	}
}

func TestWithRetryNonRetryableStatus(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusBadRequest) // 400 is not retryable
	}))
	defer server.Close()

	rc := RetryConfig{
		MaxRetries:           3,
		InitialDelay:         1 * time.Millisecond,
		RetryableStatusCodes: []int{500, 502, 503},
	}

	resp, err := WithRetry(nil, rc, func() (*http.Response, error) {
		return http.Get(server.URL)
	})

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if resp == nil {
		t.Fatal("Expected response to be returned")
	}
	defer resp.Body.Close()

	// Should only call once since 400 is not retryable
	if callCount != 1 {
		t.Errorf("Expected 1 call (non-retryable), got %d", callCount)
	}
}

func TestWithRetryNetworkError(t *testing.T) {
	callCount := 0
	rc := RetryConfig{
		MaxRetries:           2,
		InitialDelay:         1 * time.Millisecond,
		MaxDelay:             10 * time.Millisecond,
		Multiplier:           2.0,
		Jitter:               0,
		RetryableStatusCodes: []int{500},
	}

	_, err := WithRetry(nil, rc, func() (*http.Response, error) {
		callCount++
		return nil, errors.New("network error")
	})

	if err == nil {
		t.Fatal("Expected error for network failure")
	}

	// Should retry on network errors
	// 1 initial + 2 retries = 3 total calls
	if callCount != 3 {
		t.Errorf("Expected 3 calls (initial + 2 retries), got %d", callCount)
	}

	// Should be a RetryableError
	var retryErr *RetryableError
	if !errors.As(err, &retryErr) {
		t.Errorf("Expected RetryableError, got %T", err)
	}
}

func TestWithRetryContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately before first call

	callCount := 0
	rc := RetryConfig{
		MaxRetries:           10,
		InitialDelay:         100 * time.Millisecond,
		RetryableStatusCodes: []int{500},
	}

	_, err := WithRetry(ctx, rc, func() (*http.Response, error) {
		callCount++
		return nil, errors.New("network error")
	})

	if !errors.Is(err, context.Canceled) {
		t.Errorf("Expected context.Canceled error, got: %v", err)
	}

	// Should have stopped at context check (0 or 1 call)
	if callCount > 1 {
		t.Errorf("Expected at most 1 call with cancelled context, got %d", callCount)
	}
}

func TestWithRetryContextTimeout(t *testing.T) {
	// Create an already-expired context
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()
	time.Sleep(1 * time.Millisecond) // Ensure timeout has occurred

	callCount := 0
	rc := RetryConfig{
		MaxRetries:           10,
		InitialDelay:         100 * time.Millisecond,
		RetryableStatusCodes: []int{500},
	}

	_, err := WithRetry(ctx, rc, func() (*http.Response, error) {
		callCount++
		return nil, errors.New("network error")
	})

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Expected context.DeadlineExceeded error, got: %v", err)
	}

	// Should have stopped at context check
	if callCount > 1 {
		t.Errorf("Expected at most 1 call with expired context, got %d", callCount)
	}
}

func TestWithRetryNoRetries(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	rc := RetryConfig{
		MaxRetries:           0, // Disabled
		InitialDelay:         1 * time.Millisecond,
		RetryableStatusCodes: []int{503},
	}

	resp, err := WithRetry(nil, rc, func() (*http.Response, error) {
		return http.Get(server.URL)
	})

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	defer resp.Body.Close()

	// Should only call once since retries are disabled
	if callCount != 1 {
		t.Errorf("Expected 1 call (retries disabled), got %d", callCount)
	}
}

func TestWithSimpleRetrySuccess(t *testing.T) {
	callCount := 0
	rc := RetryConfig{
		MaxRetries:   3,
		InitialDelay: 1 * time.Millisecond,
	}

	err := WithSimpleRetry(nil, rc, func() error {
		callCount++
		return nil
	})

	if err != nil {
		t.Fatalf("WithSimpleRetry failed: %v", err)
	}
	if callCount != 1 {
		t.Errorf("Expected 1 call, got %d", callCount)
	}
}

func TestWithSimpleRetryEventualSuccess(t *testing.T) {
	callCount := 0
	rc := RetryConfig{
		MaxRetries:   5,
		InitialDelay: 1 * time.Millisecond,
		MaxDelay:     10 * time.Millisecond,
		Multiplier:   2.0,
		Jitter:       0,
	}

	err := WithSimpleRetry(nil, rc, func() error {
		callCount++
		if callCount < 3 {
			return errors.New("temporary error")
		}
		return nil
	})

	if err != nil {
		t.Fatalf("WithSimpleRetry failed: %v", err)
	}
	if callCount != 3 {
		t.Errorf("Expected 3 calls, got %d", callCount)
	}
}

func TestWithSimpleRetryMaxRetriesExhausted(t *testing.T) {
	callCount := 0
	rc := RetryConfig{
		MaxRetries:   2,
		InitialDelay: 1 * time.Millisecond,
		MaxDelay:     10 * time.Millisecond,
		Multiplier:   2.0,
		Jitter:       0,
	}

	err := WithSimpleRetry(nil, rc, func() error {
		callCount++
		return errors.New("persistent error")
	})

	if err == nil {
		t.Fatal("Expected error after exhausting retries")
	}

	// 1 initial + 2 retries = 3 total calls
	if callCount != 3 {
		t.Errorf("Expected 3 calls, got %d", callCount)
	}

	var retryErr *RetryableError
	if !errors.As(err, &retryErr) {
		t.Errorf("Expected RetryableError, got %T", err)
	}
}

func TestWithSimpleRetryContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	callCount := 0
	rc := RetryConfig{
		MaxRetries:   10,
		InitialDelay: 100 * time.Millisecond,
	}

	err := WithSimpleRetry(ctx, rc, func() error {
		callCount++
		return errors.New("error")
	})

	if !errors.Is(err, context.Canceled) {
		t.Errorf("Expected context.Canceled error, got: %v", err)
	}

	// Should have stopped at context check
	if callCount > 1 {
		t.Errorf("Expected at most 1 call with cancelled context, got %d", callCount)
	}
}

func TestWebhookActionWithRetry(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	event := map[string]interface{}{
		"type": "test",
	}

	config := map[string]string{
		"url":         server.URL,
		"retry_max":   "5",
		"retry_delay": "1ms",
	}

	err := webhookAction(context.Background(), event, config)
	if err != nil {
		t.Fatalf("webhookAction failed: %v", err)
	}

	if callCount != 3 {
		t.Errorf("Expected 3 calls (2 retries then success), got %d", callCount)
	}
}

func TestWebhookActionRetryDisabled(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	event := map[string]interface{}{
		"type": "test",
	}

	config := map[string]string{
		"url":       server.URL,
		"retry_max": "0",
	}

	err := webhookAction(context.Background(), event, config)
	if err == nil {
		t.Fatal("Expected error for 503 response")
	}

	if callCount != 1 {
		t.Errorf("Expected 1 call (retries disabled), got %d", callCount)
	}
}

func TestWebhookActionCustomRetryStatus(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount < 3 {
			w.WriteHeader(http.StatusTooManyRequests) // 429
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	event := map[string]interface{}{
		"type": "test",
	}

	config := map[string]string{
		"url":             server.URL,
		"retry_max":       "5",
		"retry_delay":     "1ms",
		"retry_on_status": "429", // Only retry on 429
	}

	err := webhookAction(context.Background(), event, config)
	if err != nil {
		t.Fatalf("webhookAction failed: %v", err)
	}

	if callCount != 3 {
		t.Errorf("Expected 3 calls, got %d", callCount)
	}
}
