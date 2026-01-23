package llm

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestDefaultRetryConfig(t *testing.T) {
	cfg := DefaultRetryConfig()

	if cfg.MaxRetries <= 0 {
		t.Error("MaxRetries should be positive")
	}
	if cfg.BaseDelay <= 0 {
		t.Error("BaseDelay should be positive")
	}
	if cfg.MaxDelay <= 0 {
		t.Error("MaxDelay should be positive")
	}
	if cfg.Jitter < 0 || cfg.Jitter > 1 {
		t.Error("Jitter should be between 0 and 1")
	}
}

func TestNewRetryer(t *testing.T) {
	t.Run("with config", func(t *testing.T) {
		cfg := RetryConfig{
			MaxRetries: 5,
			BaseDelay:  200 * time.Millisecond,
			MaxDelay:   10 * time.Second,
		}
		r := NewRetryer(cfg)

		if r == nil {
			t.Fatal("NewRetryer returned nil")
		}
		if r.config.MaxRetries != 5 {
			t.Errorf("MaxRetries = %v, want 5", r.config.MaxRetries)
		}
	})

	t.Run("zero values get defaults", func(t *testing.T) {
		cfg := RetryConfig{} // All zeros
		r := NewRetryer(cfg)

		if r.config.BaseDelay == 0 {
			t.Error("BaseDelay should have a default")
		}
		if r.config.MaxDelay == 0 {
			t.Error("MaxDelay should have a default")
		}
	})
}

func TestRetryerDo_Success(t *testing.T) {
	r := NewRetryer(DefaultRetryConfig())
	ctx := context.Background()

	calls := 0
	err := r.Do(ctx, func() error {
		calls++
		return nil
	})

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if calls != 1 {
		t.Errorf("calls = %v, want 1", calls)
	}
}

func TestRetryerDo_RetryableError(t *testing.T) {
	cfg := RetryConfig{
		MaxRetries: 3,
		BaseDelay:  10 * time.Millisecond,
		MaxDelay:   50 * time.Millisecond,
	}
	r := NewRetryer(cfg)
	ctx := context.Background()

	calls := 0
	testErr := errors.New("test error")

	err := r.Do(ctx, func() error {
		calls++
		if calls < 3 {
			return &RetryableError{Err: testErr}
		}
		return nil
	})

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if calls != 3 {
		t.Errorf("calls = %v, want 3", calls)
	}
}

func TestRetryerDo_NonRetryableError(t *testing.T) {
	r := NewRetryer(DefaultRetryConfig())
	ctx := context.Background()

	calls := 0
	testErr := errors.New("non-retryable error")

	err := r.Do(ctx, func() error {
		calls++
		return testErr
	})

	if err != testErr {
		t.Errorf("error = %v, want %v", err, testErr)
	}
	if calls != 1 {
		t.Errorf("calls = %v, want 1 (no retries for non-retryable error)", calls)
	}
}

func TestRetryerDo_ExhaustedRetries(t *testing.T) {
	cfg := RetryConfig{
		MaxRetries: 2,
		BaseDelay:  10 * time.Millisecond,
		MaxDelay:   50 * time.Millisecond,
	}
	r := NewRetryer(cfg)
	ctx := context.Background()

	calls := 0
	testErr := errors.New("always fails")

	err := r.Do(ctx, func() error {
		calls++
		return &RetryableError{Err: testErr}
	})

	if err != testErr {
		t.Errorf("error = %v, want %v", err, testErr)
	}
	// Initial call + 2 retries = 3 calls
	if calls != 3 {
		t.Errorf("calls = %v, want 3", calls)
	}
}

func TestRetryerDo_ContextCanceled(t *testing.T) {
	cfg := RetryConfig{
		MaxRetries: 10,
		BaseDelay:  100 * time.Millisecond,
		MaxDelay:   1 * time.Second,
	}
	r := NewRetryer(cfg)
	ctx, cancel := context.WithCancel(context.Background())

	calls := 0
	err := r.Do(ctx, func() error {
		calls++
		if calls == 2 {
			cancel()
		}
		return &RetryableError{Err: errors.New("retry me")}
	})

	if !errors.Is(err, context.Canceled) {
		t.Errorf("error = %v, want context.Canceled", err)
	}
}

func TestRetryerDo_ContextDeadline(t *testing.T) {
	cfg := RetryConfig{
		MaxRetries: 10,
		BaseDelay:  100 * time.Millisecond,
		MaxDelay:   1 * time.Second,
	}
	r := NewRetryer(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := r.Do(ctx, func() error {
		return &RetryableError{Err: errors.New("retry me")}
	})

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("error = %v, want context.DeadlineExceeded", err)
	}
}

func TestRetryerBackoffDelay(t *testing.T) {
	cfg := RetryConfig{
		MaxRetries: 5,
		BaseDelay:  100 * time.Millisecond,
		MaxDelay:   5 * time.Second,
		Jitter:     0, // No jitter for deterministic testing
	}
	r := NewRetryer(cfg)

	tests := []struct {
		attempt int
		wantMin time.Duration
		wantMax time.Duration
	}{
		{0, 100 * time.Millisecond, 100 * time.Millisecond},
		{1, 200 * time.Millisecond, 200 * time.Millisecond},
		{2, 400 * time.Millisecond, 400 * time.Millisecond},
		{3, 800 * time.Millisecond, 800 * time.Millisecond},
		{4, 1600 * time.Millisecond, 1600 * time.Millisecond},
		{5, 3200 * time.Millisecond, 3200 * time.Millisecond},
		{10, 5 * time.Second, 5 * time.Second}, // Clamped to MaxDelay
	}

	for _, tt := range tests {
		delay := r.backoffDelay(tt.attempt)
		if delay < tt.wantMin || delay > tt.wantMax {
			t.Errorf("backoffDelay(%d) = %v, want between %v and %v", tt.attempt, delay, tt.wantMin, tt.wantMax)
		}
	}
}

func TestRetryerBackoffDelayWithJitter(t *testing.T) {
	cfg := RetryConfig{
		MaxRetries: 5,
		BaseDelay:  100 * time.Millisecond,
		MaxDelay:   5 * time.Second,
		Jitter:     0.5, // 50% jitter
	}
	r := NewRetryer(cfg)

	// With jitter, delays should vary but stay within bounds
	delays := make([]time.Duration, 10)
	for i := 0; i < 10; i++ {
		delays[i] = r.backoffDelay(2) // 400ms base
	}

	// Check that delays are within expected range (400ms +/- 50%)
	for _, d := range delays {
		if d < 200*time.Millisecond || d > 600*time.Millisecond {
			t.Errorf("delay with jitter = %v, should be between 200ms and 600ms", d)
		}
	}
}

func TestRetryableError(t *testing.T) {
	originalErr := errors.New("original error")
	retryable := &RetryableError{Err: originalErr}

	if retryable.Error() != originalErr.Error() {
		t.Errorf("Error() = %v, want %v", retryable.Error(), originalErr.Error())
	}

	unwrapped := retryable.Unwrap()
	if unwrapped != originalErr {
		t.Errorf("Unwrap() = %v, want %v", unwrapped, originalErr)
	}
}

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "plain error",
			err:  errors.New("plain error"),
			want: false,
		},
		{
			name: "retryable error",
			err:  &RetryableError{Err: errors.New("retryable")},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsRetryable(tt.err)
			if got != tt.want {
				t.Errorf("IsRetryable() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRetryerCircuitBreaker(t *testing.T) {
	cfg := RetryConfig{
		MaxRetries: 0, // No retries, so failures immediately count
		BaseDelay:  10 * time.Millisecond,
		MaxDelay:   50 * time.Millisecond,
	}
	r := NewRetryer(cfg)
	r.failureThreshold = 3                 // Open after 3 failures
	r.resetTimeout = 50 * time.Millisecond // Short timeout for testing

	ctx := context.Background()
	testErr := errors.New("test error")

	// Generate failures to open the circuit
	for i := 0; i < 5; i++ {
		_ = r.Do(ctx, func() error {
			return testErr
		})
	}

	// Circuit should now be open
	if r.CircuitState() != "open" {
		t.Errorf("CircuitState() = %v, want open", r.CircuitState())
	}

	// Requests should fail immediately with circuit open
	err := r.Do(ctx, func() error {
		return nil // Would succeed, but circuit is open
	})
	if err != ErrCircuitOpen {
		t.Errorf("error = %v, want ErrCircuitOpen", err)
	}

	// Wait for reset timeout
	time.Sleep(60 * time.Millisecond)

	// Circuit should be half-open
	if r.CircuitState() != "half-open" {
		t.Errorf("CircuitState() = %v, want half-open", r.CircuitState())
	}

	// Successful request should close the circuit
	err = r.Do(ctx, func() error {
		return nil
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if r.CircuitState() != "closed" {
		t.Errorf("CircuitState() = %v, want closed", r.CircuitState())
	}
}

func TestRetryerResetCircuit(t *testing.T) {
	cfg := RetryConfig{
		MaxRetries: 0,
		BaseDelay:  10 * time.Millisecond,
		MaxDelay:   50 * time.Millisecond,
	}
	r := NewRetryer(cfg)
	r.failureThreshold = 2

	ctx := context.Background()

	// Generate failures
	for i := 0; i < 3; i++ {
		_ = r.Do(ctx, func() error {
			return errors.New("fail")
		})
	}

	if r.CircuitState() != "open" {
		t.Errorf("CircuitState() = %v, want open", r.CircuitState())
	}

	// Manual reset
	r.ResetCircuit()

	if r.CircuitState() != "closed" {
		t.Errorf("CircuitState() = %v, want closed after reset", r.CircuitState())
	}
}

func TestRetryerMetrics(t *testing.T) {
	cfg := RetryConfig{
		MaxRetries: 2,
		BaseDelay:  10 * time.Millisecond,
		MaxDelay:   50 * time.Millisecond,
	}
	r := NewRetryer(cfg)
	ctx := context.Background()

	// Successful request
	_ = r.Do(ctx, func() error {
		return nil
	})

	// Failed request (non-retryable)
	_ = r.Do(ctx, func() error {
		return errors.New("fail")
	})

	// Request with retries
	var callCount int32
	_ = r.Do(ctx, func() error {
		count := atomic.AddInt32(&callCount, 1)
		if count < 3 {
			return &RetryableError{Err: errors.New("retry")}
		}
		return nil
	})

	if r.TotalRequests() < 3 {
		t.Errorf("TotalRequests() = %v, want >= 3", r.TotalRequests())
	}
	if r.FailedRequests() < 1 {
		t.Errorf("FailedRequests() = %v, want >= 1", r.FailedRequests())
	}
	if r.RetriedRequests() < 2 {
		t.Errorf("RetriedRequests() = %v, want >= 2", r.RetriedRequests())
	}
}

func TestDoWithRetryable(t *testing.T) {
	cfg := RetryConfig{
		MaxRetries: 2,
		BaseDelay:  10 * time.Millisecond,
		MaxDelay:   50 * time.Millisecond,
	}
	r := NewRetryer(cfg)
	ctx := context.Background()

	t.Run("network errors are retried", func(t *testing.T) {
		calls := 0
		err := r.DoWithRetryable(ctx, func() error {
			calls++
			if calls < 2 {
				return errors.New("connection refused")
			}
			return nil
		})

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if calls != 2 {
			t.Errorf("calls = %v, want 2", calls)
		}
	})

	t.Run("non-network errors are not retried", func(t *testing.T) {
		calls := 0
		testErr := errors.New("business logic error")
		err := r.DoWithRetryable(ctx, func() error {
			calls++
			return testErr
		})

		if err != testErr {
			t.Errorf("error = %v, want %v", err, testErr)
		}
		if calls != 1 {
			t.Errorf("calls = %v, want 1", calls)
		}
	})
}

func TestIsNetworkError(t *testing.T) {
	tests := []struct {
		err  error
		want bool
	}{
		{nil, false},
		{errors.New("normal error"), false},
		{errors.New("connection refused"), true},
		{errors.New("connection reset by peer"), true},
		{errors.New("no such host"), true},
		{errors.New("request timeout exceeded"), true},
		{errors.New("network is unreachable"), true},
		{errors.New("unexpected EOF"), true},
		{errors.New("broken pipe"), true},
	}

	for _, tt := range tests {
		got := isNetworkError(tt.err)
		if got != tt.want {
			errStr := "nil"
			if tt.err != nil {
				errStr = tt.err.Error()
			}
			t.Errorf("isNetworkError(%v) = %v, want %v", errStr, got, tt.want)
		}
	}
}
