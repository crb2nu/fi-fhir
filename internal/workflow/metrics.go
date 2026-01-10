package workflow

import (
	"sync"
	"sync/atomic"
	"time"
)

// Metrics defines the interface for workflow metrics collection.
// Users can implement this interface to integrate with their preferred
// metrics backend (Prometheus, StatsD, OpenTelemetry, etc.).
type Metrics interface {
	// Event processing metrics
	EventProcessed(eventType, source string, success bool, duration time.Duration)
	EventRouted(eventType, routeName string)

	// Action execution metrics
	ActionExecuted(actionType, routeName string, success bool, duration time.Duration)
	ActionRetried(actionType, routeName string, attempt int)

	// Circuit breaker metrics
	CircuitBreakerStateChanged(endpoint string, fromState, toState CircuitState)
	CircuitBreakerRejected(endpoint string)

	// Rate limiter metrics
	RateLimitWaited(endpoint string, waitDuration time.Duration)
	RateLimitRejected(endpoint string)

	// Dead letter queue metrics
	DLQPushed(routeName, actionType, errorType string)
	DLQPopped(routeName string, success bool)
	DLQDepth() int64

	// HTTP metrics (for webhook/FHIR actions)
	HTTPRequestCompleted(endpoint, method string, statusCode int, duration time.Duration)
}

// NoOpMetrics is a no-op implementation that discards all metrics.
// Use this as the default when metrics collection is not needed.
type NoOpMetrics struct{}

func (n *NoOpMetrics) EventProcessed(eventType, source string, success bool, duration time.Duration) {
}
func (n *NoOpMetrics) EventRouted(eventType, routeName string) {}
func (n *NoOpMetrics) ActionExecuted(actionType, routeName string, success bool, duration time.Duration) {
}
func (n *NoOpMetrics) ActionRetried(actionType, routeName string, attempt int) {}
func (n *NoOpMetrics) CircuitBreakerStateChanged(endpoint string, fromState, toState CircuitState) {
}
func (n *NoOpMetrics) CircuitBreakerRejected(endpoint string)                      {}
func (n *NoOpMetrics) RateLimitWaited(endpoint string, waitDuration time.Duration) {}
func (n *NoOpMetrics) RateLimitRejected(endpoint string)                           {}
func (n *NoOpMetrics) DLQPushed(routeName, actionType, errorType string)           {}
func (n *NoOpMetrics) DLQPopped(routeName string, success bool)                    {}
func (n *NoOpMetrics) DLQDepth() int64                                             { return 0 }
func (n *NoOpMetrics) HTTPRequestCompleted(endpoint, method string, statusCode int, duration time.Duration) {
}

// InMemoryMetrics provides a simple in-memory metrics implementation
// useful for testing and debugging. Not recommended for production.
type InMemoryMetrics struct {
	mu sync.RWMutex

	// Counters
	eventsProcessed     map[string]*CounterValue // key: eventType:source:success
	eventsRouted        map[string]*CounterValue // key: eventType:routeName
	actionsExecuted     map[string]*CounterValue // key: actionType:routeName:success
	actionRetries       map[string]*CounterValue // key: actionType:routeName
	cbStateChanges      map[string]*CounterValue // key: endpoint:fromState:toState
	cbRejections        map[string]*CounterValue // key: endpoint
	rateLimitRejections map[string]*CounterValue // key: endpoint
	dlqPushed           map[string]*CounterValue // key: routeName:actionType:errorType
	dlqPopped           map[string]*CounterValue // key: routeName:success
	httpRequests        map[string]*CounterValue // key: endpoint:method:statusCode

	// Histograms (simplified as min/max/sum/count)
	eventDurations       map[string]*HistogramValue // key: eventType:source
	actionDurations      map[string]*HistogramValue // key: actionType:routeName
	rateLimitWaits       map[string]*HistogramValue // key: endpoint
	httpRequestDurations map[string]*HistogramValue // key: endpoint:method

	// Gauges
	dlqDepth int64
}

// CounterValue holds a simple counter.
type CounterValue struct {
	Value int64
}

// HistogramValue holds simplified histogram data (for testing).
type HistogramValue struct {
	Count int64
	Sum   float64 // milliseconds
	Min   float64
	Max   float64
}

// NewInMemoryMetrics creates a new in-memory metrics collector.
func NewInMemoryMetrics() *InMemoryMetrics {
	return &InMemoryMetrics{
		eventsProcessed:      make(map[string]*CounterValue),
		eventsRouted:         make(map[string]*CounterValue),
		actionsExecuted:      make(map[string]*CounterValue),
		actionRetries:        make(map[string]*CounterValue),
		cbStateChanges:       make(map[string]*CounterValue),
		cbRejections:         make(map[string]*CounterValue),
		rateLimitRejections:  make(map[string]*CounterValue),
		dlqPushed:            make(map[string]*CounterValue),
		dlqPopped:            make(map[string]*CounterValue),
		httpRequests:         make(map[string]*CounterValue),
		eventDurations:       make(map[string]*HistogramValue),
		actionDurations:      make(map[string]*HistogramValue),
		rateLimitWaits:       make(map[string]*HistogramValue),
		httpRequestDurations: make(map[string]*HistogramValue),
	}
}

func (m *InMemoryMetrics) EventProcessed(eventType, source string, success bool, duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	successStr := "false"
	if success {
		successStr = "true"
	}

	key := eventType + ":" + source + ":" + successStr
	if m.eventsProcessed[key] == nil {
		m.eventsProcessed[key] = &CounterValue{}
	}
	m.eventsProcessed[key].Value++

	// Record duration
	durKey := eventType + ":" + source
	m.recordDuration(m.eventDurations, durKey, duration)
}

func (m *InMemoryMetrics) EventRouted(eventType, routeName string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := eventType + ":" + routeName
	if m.eventsRouted[key] == nil {
		m.eventsRouted[key] = &CounterValue{}
	}
	m.eventsRouted[key].Value++
}

func (m *InMemoryMetrics) ActionExecuted(actionType, routeName string, success bool, duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	successStr := "false"
	if success {
		successStr = "true"
	}

	key := actionType + ":" + routeName + ":" + successStr
	if m.actionsExecuted[key] == nil {
		m.actionsExecuted[key] = &CounterValue{}
	}
	m.actionsExecuted[key].Value++

	// Record duration
	durKey := actionType + ":" + routeName
	m.recordDuration(m.actionDurations, durKey, duration)
}

func (m *InMemoryMetrics) ActionRetried(actionType, routeName string, attempt int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := actionType + ":" + routeName
	if m.actionRetries[key] == nil {
		m.actionRetries[key] = &CounterValue{}
	}
	m.actionRetries[key].Value++
}

func (m *InMemoryMetrics) CircuitBreakerStateChanged(endpoint string, fromState, toState CircuitState) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := endpoint + ":" + fromState.String() + ":" + toState.String()
	if m.cbStateChanges[key] == nil {
		m.cbStateChanges[key] = &CounterValue{}
	}
	m.cbStateChanges[key].Value++
}

func (m *InMemoryMetrics) CircuitBreakerRejected(endpoint string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cbRejections[endpoint] == nil {
		m.cbRejections[endpoint] = &CounterValue{}
	}
	m.cbRejections[endpoint].Value++
}

func (m *InMemoryMetrics) RateLimitWaited(endpoint string, waitDuration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.recordDuration(m.rateLimitWaits, endpoint, waitDuration)
}

func (m *InMemoryMetrics) RateLimitRejected(endpoint string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.rateLimitRejections[endpoint] == nil {
		m.rateLimitRejections[endpoint] = &CounterValue{}
	}
	m.rateLimitRejections[endpoint].Value++
}

func (m *InMemoryMetrics) DLQPushed(routeName, actionType, errorType string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := routeName + ":" + actionType + ":" + errorType
	if m.dlqPushed[key] == nil {
		m.dlqPushed[key] = &CounterValue{}
	}
	m.dlqPushed[key].Value++

	atomic.AddInt64(&m.dlqDepth, 1)
}

func (m *InMemoryMetrics) DLQPopped(routeName string, success bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	successStr := "false"
	if success {
		successStr = "true"
	}

	key := routeName + ":" + successStr
	if m.dlqPopped[key] == nil {
		m.dlqPopped[key] = &CounterValue{}
	}
	m.dlqPopped[key].Value++

	atomic.AddInt64(&m.dlqDepth, -1)
}

func (m *InMemoryMetrics) DLQDepth() int64 {
	return atomic.LoadInt64(&m.dlqDepth)
}

func (m *InMemoryMetrics) HTTPRequestCompleted(endpoint, method string, statusCode int, duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := endpoint + ":" + method + ":" + itoa(statusCode)
	if m.httpRequests[key] == nil {
		m.httpRequests[key] = &CounterValue{}
	}
	m.httpRequests[key].Value++

	// Record duration
	durKey := endpoint + ":" + method
	m.recordDuration(m.httpRequestDurations, durKey, duration)
}

// recordDuration records a duration value in a histogram map.
func (m *InMemoryMetrics) recordDuration(histMap map[string]*HistogramValue, key string, duration time.Duration) {
	ms := float64(duration.Milliseconds())
	if histMap[key] == nil {
		histMap[key] = &HistogramValue{Min: ms, Max: ms}
	}
	h := histMap[key]
	h.Count++
	h.Sum += ms
	if ms < h.Min {
		h.Min = ms
	}
	if ms > h.Max {
		h.Max = ms
	}
}

// Snapshot returns a copy of all current metrics for inspection.
type MetricsSnapshot struct {
	EventsProcessed     map[string]int64
	EventsRouted        map[string]int64
	ActionsExecuted     map[string]int64
	ActionRetries       map[string]int64
	CBStateChanges      map[string]int64
	CBRejections        map[string]int64
	RateLimitRejections map[string]int64
	DLQPushed           map[string]int64
	DLQPopped           map[string]int64
	HTTPRequests        map[string]int64
	DLQDepth            int64

	// Duration stats
	EventDurations       map[string]DurationStats
	ActionDurations      map[string]DurationStats
	RateLimitWaits       map[string]DurationStats
	HTTPRequestDurations map[string]DurationStats
}

// DurationStats holds duration statistics.
type DurationStats struct {
	Count int64
	Sum   float64 // milliseconds
	Min   float64
	Max   float64
	Avg   float64
}

// Snapshot returns a point-in-time snapshot of all metrics.
func (m *InMemoryMetrics) Snapshot() MetricsSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	snapshot := MetricsSnapshot{
		EventsProcessed:      copyCounterMap(m.eventsProcessed),
		EventsRouted:         copyCounterMap(m.eventsRouted),
		ActionsExecuted:      copyCounterMap(m.actionsExecuted),
		ActionRetries:        copyCounterMap(m.actionRetries),
		CBStateChanges:       copyCounterMap(m.cbStateChanges),
		CBRejections:         copyCounterMap(m.cbRejections),
		RateLimitRejections:  copyCounterMap(m.rateLimitRejections),
		DLQPushed:            copyCounterMap(m.dlqPushed),
		DLQPopped:            copyCounterMap(m.dlqPopped),
		HTTPRequests:         copyCounterMap(m.httpRequests),
		DLQDepth:             atomic.LoadInt64(&m.dlqDepth),
		EventDurations:       copyHistogramMap(m.eventDurations),
		ActionDurations:      copyHistogramMap(m.actionDurations),
		RateLimitWaits:       copyHistogramMap(m.rateLimitWaits),
		HTTPRequestDurations: copyHistogramMap(m.httpRequestDurations),
	}

	return snapshot
}

// Reset clears all metrics (useful for testing).
func (m *InMemoryMetrics) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.eventsProcessed = make(map[string]*CounterValue)
	m.eventsRouted = make(map[string]*CounterValue)
	m.actionsExecuted = make(map[string]*CounterValue)
	m.actionRetries = make(map[string]*CounterValue)
	m.cbStateChanges = make(map[string]*CounterValue)
	m.cbRejections = make(map[string]*CounterValue)
	m.rateLimitRejections = make(map[string]*CounterValue)
	m.dlqPushed = make(map[string]*CounterValue)
	m.dlqPopped = make(map[string]*CounterValue)
	m.httpRequests = make(map[string]*CounterValue)
	m.eventDurations = make(map[string]*HistogramValue)
	m.actionDurations = make(map[string]*HistogramValue)
	m.rateLimitWaits = make(map[string]*HistogramValue)
	m.httpRequestDurations = make(map[string]*HistogramValue)
	atomic.StoreInt64(&m.dlqDepth, 0)
}

// GetCounter returns a specific counter value.
func (m *InMemoryMetrics) GetCounter(category, key string) int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var counterMap map[string]*CounterValue
	switch category {
	case "events_processed":
		counterMap = m.eventsProcessed
	case "events_routed":
		counterMap = m.eventsRouted
	case "actions_executed":
		counterMap = m.actionsExecuted
	case "action_retries":
		counterMap = m.actionRetries
	case "cb_state_changes":
		counterMap = m.cbStateChanges
	case "cb_rejections":
		counterMap = m.cbRejections
	case "rate_limit_rejections":
		counterMap = m.rateLimitRejections
	case "dlq_pushed":
		counterMap = m.dlqPushed
	case "dlq_popped":
		counterMap = m.dlqPopped
	case "http_requests":
		counterMap = m.httpRequests
	default:
		return 0
	}

	if counterMap[key] == nil {
		return 0
	}
	return counterMap[key].Value
}

// Helper functions

func copyCounterMap(src map[string]*CounterValue) map[string]int64 {
	dst := make(map[string]int64, len(src))
	for k, v := range src {
		dst[k] = v.Value
	}
	return dst
}

func copyHistogramMap(src map[string]*HistogramValue) map[string]DurationStats {
	dst := make(map[string]DurationStats, len(src))
	for k, v := range src {
		avg := 0.0
		if v.Count > 0 {
			avg = v.Sum / float64(v.Count)
		}
		dst[k] = DurationStats{
			Count: v.Count,
			Sum:   v.Sum,
			Min:   v.Min,
			Max:   v.Max,
			Avg:   avg,
		}
	}
	return dst
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	if i < 0 {
		return "-" + uitoa(uint(-i))
	}
	return uitoa(uint(i))
}

func uitoa(u uint) string {
	var buf [20]byte
	i := len(buf)
	for u >= 10 {
		i--
		q := u / 10
		buf[i] = byte('0' + u - q*10)
		u = q
	}
	i--
	buf[i] = byte('0' + u)
	return string(buf[i:])
}

// Global metrics instance (can be set by user)
var globalMetrics Metrics = &NoOpMetrics{}
var globalMetricsMu sync.RWMutex

// SetGlobalMetrics sets the global metrics collector.
func SetGlobalMetrics(m Metrics) {
	globalMetricsMu.Lock()
	defer globalMetricsMu.Unlock()
	globalMetrics = m
}

// GetGlobalMetrics returns the current global metrics collector.
func GetGlobalMetrics() Metrics {
	globalMetricsMu.RLock()
	defer globalMetricsMu.RUnlock()
	return globalMetrics
}

// MetricsConfig holds configuration for metrics collection.
type MetricsConfig struct {
	// Enabled controls whether metrics are collected
	Enabled bool

	// Prefix is prepended to all metric names
	Prefix string

	// Labels are added to all metrics
	Labels map[string]string
}

// DefaultMetricsConfig returns sensible defaults.
func DefaultMetricsConfig() MetricsConfig {
	return MetricsConfig{
		Enabled: false,
		Prefix:  "fi_fhir_workflow",
		Labels:  make(map[string]string),
	}
}
