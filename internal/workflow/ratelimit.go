package workflow

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// RateLimiter defines the interface for rate limiting.
type RateLimiter interface {
	// Allow checks if a request is allowed. Returns true if allowed.
	Allow() bool

	// AllowN checks if n requests are allowed. Returns true if all allowed.
	AllowN(n int) bool

	// Wait blocks until a request is allowed or context is cancelled.
	// Returns nil if allowed, context error if cancelled.
	Wait(ctx context.Context) error

	// WaitN blocks until n requests are allowed or context is cancelled.
	WaitN(ctx context.Context, n int) error

	// Reserve reserves a token and returns the time to wait before using it.
	// Returns 0 if token is immediately available.
	Reserve() time.Duration

	// Tokens returns the current number of available tokens.
	Tokens() float64

	// Limit returns the rate limit (tokens per second).
	Limit() float64

	// Burst returns the maximum burst size.
	Burst() int
}

// TokenBucket implements a token bucket rate limiter.
// Tokens are added at a fixed rate up to a maximum burst capacity.
type TokenBucket struct {
	mu sync.Mutex

	// Configuration
	rate  float64 // Tokens per second
	burst int     // Maximum tokens (bucket capacity)

	// State
	tokens     float64   // Current available tokens
	lastUpdate time.Time // Last time tokens were updated
}

// NewTokenBucket creates a new token bucket rate limiter.
// rate: tokens added per second
// burst: maximum tokens (bucket capacity), also initial token count
func NewTokenBucket(rate float64, burst int) *TokenBucket {
	if rate <= 0 {
		rate = 1.0
	}
	if burst <= 0 {
		burst = 1
	}

	return &TokenBucket{
		rate:       rate,
		burst:      burst,
		tokens:     float64(burst), // Start full
		lastUpdate: time.Now(),
	}
}

// Allow checks if a single request is allowed.
func (tb *TokenBucket) Allow() bool {
	return tb.AllowN(1)
}

// AllowN checks if n requests are allowed.
func (tb *TokenBucket) AllowN(n int) bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	tb.refill()

	if tb.tokens >= float64(n) {
		tb.tokens -= float64(n)
		return true
	}
	return false
}

// Wait blocks until a request is allowed or context is cancelled.
func (tb *TokenBucket) Wait(ctx context.Context) error {
	return tb.WaitN(ctx, 1)
}

// WaitN blocks until n requests are allowed or context is cancelled.
func (tb *TokenBucket) WaitN(ctx context.Context, n int) error {
	// Try immediate allow first
	if tb.AllowN(n) {
		return nil
	}

	// Calculate wait time
	waitDuration := tb.reserveN(n)
	if waitDuration == 0 {
		return nil
	}

	// Wait with context
	timer := time.NewTimer(waitDuration)
	defer timer.Stop()

	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		// Return the reserved tokens since we're not using them
		tb.mu.Lock()
		tb.tokens += float64(n)
		if tb.tokens > float64(tb.burst) {
			tb.tokens = float64(tb.burst)
		}
		tb.mu.Unlock()
		return ctx.Err()
	}
}

// Reserve reserves a token and returns the time to wait before using it.
func (tb *TokenBucket) Reserve() time.Duration {
	return tb.reserveN(1)
}

// reserveN reserves n tokens and returns wait time.
func (tb *TokenBucket) reserveN(n int) time.Duration {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	tb.refill()

	// If we have enough tokens, consume them immediately
	if tb.tokens >= float64(n) {
		tb.tokens -= float64(n)
		return 0
	}

	// Calculate how long until we have enough tokens
	needed := float64(n) - tb.tokens
	waitDuration := time.Duration(needed / tb.rate * float64(time.Second))

	// Reserve by going negative
	tb.tokens -= float64(n)

	return waitDuration
}

// Tokens returns the current number of available tokens.
func (tb *TokenBucket) Tokens() float64 {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	tb.refill()
	return tb.tokens
}

// Limit returns the rate limit (tokens per second).
func (tb *TokenBucket) Limit() float64 {
	return tb.rate
}

// Burst returns the maximum burst size.
func (tb *TokenBucket) Burst() int {
	return tb.burst
}

// refill adds tokens based on elapsed time. Must be called with lock held.
func (tb *TokenBucket) refill() {
	now := time.Now()
	elapsed := now.Sub(tb.lastUpdate).Seconds()
	tb.lastUpdate = now

	// Add tokens based on elapsed time
	tb.tokens += elapsed * tb.rate

	// Cap at burst capacity
	if tb.tokens > float64(tb.burst) {
		tb.tokens = float64(tb.burst)
	}
}

// RateLimiterRegistry manages rate limiters for different keys.
type RateLimiterRegistry struct {
	mu       sync.RWMutex
	limiters map[string]RateLimiter
	config   RateLimitConfig
}

// RateLimitConfig holds default configuration for rate limiters.
type RateLimitConfig struct {
	// Rate is the default tokens per second
	Rate float64

	// Burst is the default maximum burst size
	Burst int
}

// DefaultRateLimitConfig returns sensible defaults.
func DefaultRateLimitConfig() RateLimitConfig {
	return RateLimitConfig{
		Rate:  10.0, // 10 requests per second
		Burst: 20,   // Allow burst of 20
	}
}

// NewRateLimiterRegistry creates a new registry with default configuration.
func NewRateLimiterRegistry() *RateLimiterRegistry {
	return &RateLimiterRegistry{
		limiters: make(map[string]RateLimiter),
		config:   DefaultRateLimitConfig(),
	}
}

// NewRateLimiterRegistryWithConfig creates a new registry with custom configuration.
func NewRateLimiterRegistryWithConfig(config RateLimitConfig) *RateLimiterRegistry {
	return &RateLimiterRegistry{
		limiters: make(map[string]RateLimiter),
		config:   config,
	}
}

// GetLimiter returns the rate limiter for a given key.
// Creates a new one if it doesn't exist.
func (r *RateLimiterRegistry) GetLimiter(key string) RateLimiter {
	r.mu.RLock()
	limiter, exists := r.limiters[key]
	r.mu.RUnlock()

	if exists {
		return limiter
	}

	// Create new limiter
	r.mu.Lock()
	defer r.mu.Unlock()

	// Double-check after acquiring write lock
	if limiter, exists = r.limiters[key]; exists {
		return limiter
	}

	limiter = NewTokenBucket(r.config.Rate, r.config.Burst)
	r.limiters[key] = limiter
	return limiter
}

// SetLimiter sets a custom rate limiter for a key.
func (r *RateLimiterRegistry) SetLimiter(key string, limiter RateLimiter) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.limiters[key] = limiter
}

// Remove removes the rate limiter for a key.
func (r *RateLimiterRegistry) Remove(key string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.limiters, key)
}

// Global rate limiter registry for the workflow package.
var globalRateLimiterRegistry = NewRateLimiterRegistry()

// endpointRateLimiters stores rate limiters keyed by endpoint with custom configs.
var endpointRateLimiters = &RateLimiterRegistry{
	limiters: make(map[string]RateLimiter),
	config:   DefaultRateLimitConfig(),
}
var endpointRateLimitersMu sync.RWMutex

// GetRateLimiter returns the rate limiter for a given key (usually endpoint URL).
func GetRateLimiter(key string) RateLimiter {
	return globalRateLimiterRegistry.GetLimiter(key)
}

// GetOrCreateRateLimiter gets an existing limiter or creates one with specific config.
// This is used when actions have custom rate limit configurations.
func GetOrCreateRateLimiter(key string, config RateLimitConfig) RateLimiter {
	endpointRateLimitersMu.RLock()
	if limiter, exists := endpointRateLimiters.limiters[key]; exists {
		endpointRateLimitersMu.RUnlock()
		return limiter
	}
	endpointRateLimitersMu.RUnlock()

	// Create new limiter with specific config
	endpointRateLimitersMu.Lock()
	defer endpointRateLimitersMu.Unlock()

	// Double-check after acquiring write lock
	if limiter, exists := endpointRateLimiters.limiters[key]; exists {
		return limiter
	}

	limiter := NewTokenBucket(config.Rate, config.Burst)
	endpointRateLimiters.limiters[key] = limiter
	return limiter
}

// ResetEndpointRateLimiters clears all endpoint-specific rate limiters (used in tests).
func ResetEndpointRateLimiters() {
	endpointRateLimitersMu.Lock()
	defer endpointRateLimitersMu.Unlock()
	endpointRateLimiters.limiters = make(map[string]RateLimiter)
}

// SetGlobalRateLimitConfig sets the default rate limit configuration for new limiters.
func SetGlobalRateLimitConfig(config RateLimitConfig) {
	globalRateLimiterRegistry = NewRateLimiterRegistryWithConfig(config)
}

// ParseRateLimitConfig extracts rate limit configuration from an action config map.
// Config keys:
//   - rate_limit: "true" to enable rate limiting (default: disabled)
//   - rate_limit_rate: requests per second (default: 10)
//   - rate_limit_burst: maximum burst size (default: 20)
//   - rate_limit_wait: "true" to wait when limited, "false" to fail fast (default: true)
func ParseRateLimitConfig(config map[string]string) *RateLimitConfig {
	// Check if rate limiting is enabled
	if config["rate_limit"] != "true" {
		return nil
	}

	rlConfig := DefaultRateLimitConfig()

	// Parse rate
	if v := config["rate_limit_rate"]; v != "" {
		var rate float64
		if _, err := fmt.Sscanf(v, "%f", &rate); err == nil && rate > 0 {
			rlConfig.Rate = rate
		}
	}

	// Parse burst
	if v := config["rate_limit_burst"]; v != "" {
		if n, err := parseInt(v); err == nil && n > 0 {
			rlConfig.Burst = n
		}
	}

	return &rlConfig
}

// ShouldWaitOnRateLimit checks if the action should wait when rate limited.
func ShouldWaitOnRateLimit(config map[string]string) bool {
	// Default is to wait
	if v := config["rate_limit_wait"]; v == "false" {
		return false
	}
	return true
}

// ErrRateLimited is returned when a request is rate limited and wait is disabled.
var ErrRateLimited = fmt.Errorf("rate limited")

// WithRateLimit wraps a function with rate limiting.
// If limiter is nil, the function is called directly.
// If wait is true, blocks until allowed. If false, returns ErrRateLimited.
func WithRateLimit(ctx context.Context, limiter RateLimiter, wait bool, fn func() error) error {
	if limiter == nil {
		return fn()
	}

	if wait {
		// Wait for rate limit
		if err := limiter.Wait(ctx); err != nil {
			return fmt.Errorf("rate limit wait cancelled: %w", err)
		}
		return fn()
	}

	// Fail fast if not allowed
	if !limiter.Allow() {
		return ErrRateLimited
	}
	return fn()
}

// MultiLimiter combines multiple rate limiters.
// All limiters must allow for the request to proceed.
type MultiLimiter struct {
	limiters []RateLimiter
}

// NewMultiLimiter creates a rate limiter that enforces multiple limits.
func NewMultiLimiter(limiters ...RateLimiter) *MultiLimiter {
	return &MultiLimiter{limiters: limiters}
}

// Allow checks if all limiters allow the request.
func (ml *MultiLimiter) Allow() bool {
	return ml.AllowN(1)
}

// AllowN checks if all limiters allow n requests.
func (ml *MultiLimiter) AllowN(n int) bool {
	// Check all limiters first (don't consume tokens yet)
	for _, l := range ml.limiters {
		if l.Tokens() < float64(n) {
			return false
		}
	}

	// All have enough tokens, consume from all
	for _, l := range ml.limiters {
		l.AllowN(n)
	}
	return true
}

// Wait blocks until all limiters allow the request.
func (ml *MultiLimiter) Wait(ctx context.Context) error {
	return ml.WaitN(ctx, 1)
}

// WaitN blocks until all limiters allow n requests.
func (ml *MultiLimiter) WaitN(ctx context.Context, n int) error {
	for _, l := range ml.limiters {
		if err := l.WaitN(ctx, n); err != nil {
			return err
		}
	}
	return nil
}

// Reserve returns the maximum wait time across all limiters.
func (ml *MultiLimiter) Reserve() time.Duration {
	var maxWait time.Duration
	for _, l := range ml.limiters {
		if wait := l.Reserve(); wait > maxWait {
			maxWait = wait
		}
	}
	return maxWait
}

// Tokens returns the minimum tokens across all limiters.
func (ml *MultiLimiter) Tokens() float64 {
	if len(ml.limiters) == 0 {
		return 0
	}
	minVal := ml.limiters[0].Tokens()
	for _, l := range ml.limiters[1:] {
		if t := l.Tokens(); t < minVal {
			minVal = t
		}
	}
	return minVal
}

// Limit returns the minimum rate across all limiters.
func (ml *MultiLimiter) Limit() float64 {
	if len(ml.limiters) == 0 {
		return 0
	}
	minVal := ml.limiters[0].Limit()
	for _, l := range ml.limiters[1:] {
		if r := l.Limit(); r < minVal {
			minVal = r
		}
	}
	return minVal
}

// Burst returns the minimum burst across all limiters.
func (ml *MultiLimiter) Burst() int {
	if len(ml.limiters) == 0 {
		return 0
	}
	minVal := ml.limiters[0].Burst()
	for _, l := range ml.limiters[1:] {
		if b := l.Burst(); b < minVal {
			minVal = b
		}
	}
	return minVal
}
