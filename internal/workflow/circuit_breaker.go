package workflow

import (
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// CircuitState represents the current state of a circuit breaker.
type CircuitState int

const (
	// CircuitClosed is the normal state where requests pass through.
	CircuitClosed CircuitState = iota
	// CircuitOpen is the tripped state where requests fail immediately.
	CircuitOpen
	// CircuitHalfOpen allows one request through to test if service recovered.
	CircuitHalfOpen
)

func (s CircuitState) String() string {
	switch s {
	case CircuitClosed:
		return "closed"
	case CircuitOpen:
		return "open"
	case CircuitHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// CircuitBreakerConfig holds configuration for a circuit breaker.
type CircuitBreakerConfig struct {
	// FailureThreshold is the number of consecutive failures before opening the circuit.
	FailureThreshold int

	// SuccessThreshold is the number of consecutive successes in half-open state
	// before closing the circuit.
	SuccessThreshold int

	// Timeout is the duration to wait in open state before transitioning to half-open.
	Timeout time.Duration

	// FailureStatusCodes are HTTP status codes that count as failures.
	// Default: 500, 502, 503, 504
	FailureStatusCodes []int
}

// DefaultCircuitBreakerConfig returns sensible defaults for a circuit breaker.
func DefaultCircuitBreakerConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		FailureThreshold:   5,
		SuccessThreshold:   2,
		Timeout:            30 * time.Second,
		FailureStatusCodes: []int{500, 502, 503, 504},
	}
}

// CircuitBreaker implements the circuit breaker pattern for HTTP requests.
type CircuitBreaker struct {
	mu sync.RWMutex

	config CircuitBreakerConfig

	state            CircuitState
	failures         int       // Consecutive failures in closed state
	successes        int       // Consecutive successes in half-open state
	lastFailureTime  time.Time // Time of last failure (for open->half-open transition)
	lastStateChange  time.Time // Time of last state change
	totalFailures    int64     // Total failures (for metrics)
	totalSuccesses   int64     // Total successes (for metrics)
	totalRejected    int64     // Total rejected by open circuit (for metrics)
}

// NewCircuitBreaker creates a new circuit breaker with the given configuration.
func NewCircuitBreaker(config CircuitBreakerConfig) *CircuitBreaker {
	if config.FailureThreshold <= 0 {
		config.FailureThreshold = 5
	}
	if config.SuccessThreshold <= 0 {
		config.SuccessThreshold = 2
	}
	if config.Timeout <= 0 {
		config.Timeout = 30 * time.Second
	}
	if len(config.FailureStatusCodes) == 0 {
		config.FailureStatusCodes = []int{500, 502, 503, 504}
	}

	return &CircuitBreaker{
		config:          config,
		state:           CircuitClosed,
		lastStateChange: time.Now(),
	}
}

// ErrCircuitOpen is returned when the circuit breaker is open.
var ErrCircuitOpen = errors.New("circuit breaker is open")

// State returns the current circuit state.
func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.currentState()
}

// currentState returns the effective state, checking for timeout transition.
// Must be called with at least a read lock held.
func (cb *CircuitBreaker) currentState() CircuitState {
	if cb.state == CircuitOpen {
		// Check if timeout has elapsed
		if time.Since(cb.lastFailureTime) >= cb.config.Timeout {
			return CircuitHalfOpen
		}
	}
	return cb.state
}

// Allow checks if a request should be allowed through.
// Returns true if allowed, false if circuit is open.
func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	state := cb.currentState()

	switch state {
	case CircuitClosed:
		return true
	case CircuitOpen:
		cb.totalRejected++
		return false
	case CircuitHalfOpen:
		// In half-open, allow one request through
		// Transition to half-open state officially
		if cb.state == CircuitOpen {
			cb.state = CircuitHalfOpen
			cb.lastStateChange = time.Now()
			cb.successes = 0
		}
		return true
	default:
		return false
	}
}

// RecordSuccess records a successful request.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.totalSuccesses++

	switch cb.state {
	case CircuitClosed:
		// Reset failure count on success
		cb.failures = 0
	case CircuitHalfOpen:
		cb.successes++
		// Check if we've reached success threshold
		if cb.successes >= cb.config.SuccessThreshold {
			cb.state = CircuitClosed
			cb.lastStateChange = time.Now()
			cb.failures = 0
			cb.successes = 0
		}
	}
}

// RecordFailure records a failed request.
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.totalFailures++
	cb.lastFailureTime = time.Now()

	switch cb.state {
	case CircuitClosed:
		cb.failures++
		// Check if we've reached failure threshold
		if cb.failures >= cb.config.FailureThreshold {
			cb.state = CircuitOpen
			cb.lastStateChange = time.Now()
		}
	case CircuitHalfOpen:
		// Any failure in half-open immediately opens the circuit
		cb.state = CircuitOpen
		cb.lastStateChange = time.Now()
		cb.successes = 0
	}
}

// IsFailure checks if an HTTP status code should count as a failure.
func (cb *CircuitBreaker) IsFailure(statusCode int) bool {
	for _, code := range cb.config.FailureStatusCodes {
		if code == statusCode {
			return true
		}
	}
	return false
}

// Stats returns current circuit breaker statistics.
func (cb *CircuitBreaker) Stats() CircuitBreakerStats {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	return CircuitBreakerStats{
		State:           cb.currentState(),
		Failures:        cb.failures,
		Successes:       cb.successes,
		TotalFailures:   cb.totalFailures,
		TotalSuccesses:  cb.totalSuccesses,
		TotalRejected:   cb.totalRejected,
		LastFailureTime: cb.lastFailureTime,
		LastStateChange: cb.lastStateChange,
	}
}

// CircuitBreakerStats holds statistics about a circuit breaker.
type CircuitBreakerStats struct {
	State           CircuitState
	Failures        int
	Successes       int
	TotalFailures   int64
	TotalSuccesses  int64
	TotalRejected   int64
	LastFailureTime time.Time
	LastStateChange time.Time
}

// Reset resets the circuit breaker to closed state.
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.state = CircuitClosed
	cb.failures = 0
	cb.successes = 0
	cb.lastStateChange = time.Now()
}

// CircuitBreakerRegistry manages circuit breakers for different endpoints.
type CircuitBreakerRegistry struct {
	mu       sync.RWMutex
	breakers map[string]*CircuitBreaker
	config   CircuitBreakerConfig
}

// NewCircuitBreakerRegistry creates a new registry with default configuration.
func NewCircuitBreakerRegistry() *CircuitBreakerRegistry {
	return &CircuitBreakerRegistry{
		breakers: make(map[string]*CircuitBreaker),
		config:   DefaultCircuitBreakerConfig(),
	}
}

// NewCircuitBreakerRegistryWithConfig creates a new registry with custom configuration.
func NewCircuitBreakerRegistryWithConfig(config CircuitBreakerConfig) *CircuitBreakerRegistry {
	return &CircuitBreakerRegistry{
		breakers: make(map[string]*CircuitBreaker),
		config:   config,
	}
}

// GetBreaker returns the circuit breaker for a given key (usually endpoint URL).
// Creates a new one if it doesn't exist.
func (r *CircuitBreakerRegistry) GetBreaker(key string) *CircuitBreaker {
	r.mu.RLock()
	cb, exists := r.breakers[key]
	r.mu.RUnlock()

	if exists {
		return cb
	}

	// Create new breaker
	r.mu.Lock()
	defer r.mu.Unlock()

	// Double-check after acquiring write lock
	if cb, exists = r.breakers[key]; exists {
		return cb
	}

	cb = NewCircuitBreaker(r.config)
	r.breakers[key] = cb
	return cb
}

// AllStats returns statistics for all circuit breakers.
func (r *CircuitBreakerRegistry) AllStats() map[string]CircuitBreakerStats {
	r.mu.RLock()
	defer r.mu.RUnlock()

	stats := make(map[string]CircuitBreakerStats, len(r.breakers))
	for key, cb := range r.breakers {
		stats[key] = cb.Stats()
	}
	return stats
}

// Global circuit breaker registry for the workflow package.
var globalCircuitBreakerRegistry = NewCircuitBreakerRegistry()

// GetCircuitBreaker returns the circuit breaker for a given endpoint.
func GetCircuitBreaker(endpoint string) *CircuitBreaker {
	return globalCircuitBreakerRegistry.GetBreaker(endpoint)
}

// ParseCircuitBreakerConfig extracts circuit breaker configuration from an action config map.
// Config keys:
//   - circuit_breaker: "true" to enable (default: disabled)
//   - circuit_failure_threshold: failures before opening (default: 5)
//   - circuit_success_threshold: successes before closing (default: 2)
//   - circuit_timeout: time before half-open (default: 30s)
func ParseCircuitBreakerConfig(config map[string]string) *CircuitBreakerConfig {
	// Check if circuit breaker is enabled
	if config["circuit_breaker"] != "true" {
		return nil
	}

	cbConfig := DefaultCircuitBreakerConfig()

	// Parse failure threshold
	if v := config["circuit_failure_threshold"]; v != "" {
		if n, err := parseInt(v); err == nil && n > 0 {
			cbConfig.FailureThreshold = n
		}
	}

	// Parse success threshold
	if v := config["circuit_success_threshold"]; v != "" {
		if n, err := parseInt(v); err == nil && n > 0 {
			cbConfig.SuccessThreshold = n
		}
	}

	// Parse timeout
	if v := config["circuit_timeout"]; v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			cbConfig.Timeout = d
		}
	}

	return &cbConfig
}

// parseInt is a helper to parse int from string.
func parseInt(s string) (int, error) {
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}

// WithCircuitBreaker wraps an HTTP request function with circuit breaker protection.
// If the circuit is open, it returns ErrCircuitOpen immediately.
// The doFn should return the HTTP response and any error.
func WithCircuitBreaker(cb *CircuitBreaker, doFn func() (*http.Response, error)) (*http.Response, error) {
	if cb == nil {
		// No circuit breaker configured, just execute
		return doFn()
	}

	// Check if circuit allows request
	if !cb.Allow() {
		return nil, ErrCircuitOpen
	}

	// Execute the request
	resp, err := doFn()

	// Record result
	if err != nil {
		cb.RecordFailure()
		return resp, err
	}

	if resp != nil && cb.IsFailure(resp.StatusCode) {
		cb.RecordFailure()
	} else {
		cb.RecordSuccess()
	}

	return resp, nil
}
