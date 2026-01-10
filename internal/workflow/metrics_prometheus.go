package workflow

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// PrometheusMetrics implements the Metrics interface using Prometheus client.
// This is a reference implementation showing how to integrate with Prometheus.
type PrometheusMetrics struct {
	registry *prometheus.Registry
	config   PrometheusConfig

	// Event metrics
	eventsProcessedTotal    *prometheus.CounterVec
	eventsProcessedDuration *prometheus.HistogramVec
	eventsRoutedTotal       *prometheus.CounterVec

	// Action metrics
	actionsExecutedTotal    *prometheus.CounterVec
	actionsExecutedDuration *prometheus.HistogramVec
	actionRetriesTotal      *prometheus.CounterVec

	// Circuit breaker metrics
	circuitBreakerStateChanges *prometheus.CounterVec
	circuitBreakerRejections   *prometheus.CounterVec

	// Rate limiter metrics
	rateLimitWaitsTotal      *prometheus.CounterVec
	rateLimitWaitsDuration   *prometheus.HistogramVec
	rateLimitRejectionsTotal *prometheus.CounterVec

	// DLQ metrics
	dlqPushedTotal *prometheus.CounterVec
	dlqPoppedTotal *prometheus.CounterVec
	dlqDepth       prometheus.Gauge
	dlqDepthValue  int64 // atomic counter for DLQDepth() method

	// HTTP metrics
	httpRequestsTotal    *prometheus.CounterVec
	httpRequestsDuration *prometheus.HistogramVec
}

// PrometheusConfig configures the Prometheus metrics adapter.
type PrometheusConfig struct {
	// Namespace is prepended to all metric names (default: "fi_fhir")
	Namespace string

	// Subsystem is inserted between namespace and metric name (default: "workflow")
	Subsystem string

	// ConstLabels are added to all metrics
	ConstLabels prometheus.Labels

	// DurationBuckets defines histogram buckets for duration metrics in seconds
	// Default: [0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10]
	DurationBuckets []float64

	// Registry allows using a custom Prometheus registry
	// If nil, a new registry is created
	Registry *prometheus.Registry
}

// DefaultPrometheusConfig returns sensible defaults for Prometheus metrics.
func DefaultPrometheusConfig() PrometheusConfig {
	return PrometheusConfig{
		Namespace:   "fi_fhir",
		Subsystem:   "workflow",
		ConstLabels: nil,
		DurationBuckets: []float64{
			0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10,
		},
	}
}

// NewPrometheusMetrics creates a new Prometheus metrics collector.
// If config is nil, defaults are used.
func NewPrometheusMetrics(config *PrometheusConfig) (*PrometheusMetrics, error) {
	cfg := DefaultPrometheusConfig()
	if config != nil {
		if config.Namespace != "" {
			cfg.Namespace = config.Namespace
		}
		if config.Subsystem != "" {
			cfg.Subsystem = config.Subsystem
		}
		if config.ConstLabels != nil {
			cfg.ConstLabels = config.ConstLabels
		}
		if len(config.DurationBuckets) > 0 {
			cfg.DurationBuckets = config.DurationBuckets
		}
		if config.Registry != nil {
			cfg.Registry = config.Registry
		}
	}

	// Create or use provided registry
	registry := cfg.Registry
	if registry == nil {
		registry = prometheus.NewRegistry()
	}

	m := &PrometheusMetrics{
		registry: registry,
		config:   cfg,
	}

	// Initialize all metrics
	if err := m.initMetrics(); err != nil {
		return nil, fmt.Errorf("failed to initialize metrics: %w", err)
	}

	return m, nil
}

// initMetrics creates and registers all Prometheus metrics.
func (m *PrometheusMetrics) initMetrics() error {
	ns := m.config.Namespace
	sub := m.config.Subsystem
	constLabels := m.config.ConstLabels
	buckets := m.config.DurationBuckets

	// Event metrics
	m.eventsProcessedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace:   ns,
			Subsystem:   sub,
			Name:        "events_processed_total",
			Help:        "Total number of events processed",
			ConstLabels: constLabels,
		},
		[]string{"event_type", "source", "success"},
	)

	m.eventsProcessedDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace:   ns,
			Subsystem:   sub,
			Name:        "events_processed_duration_seconds",
			Help:        "Duration of event processing in seconds",
			ConstLabels: constLabels,
			Buckets:     buckets,
		},
		[]string{"event_type", "source"},
	)

	m.eventsRoutedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace:   ns,
			Subsystem:   sub,
			Name:        "events_routed_total",
			Help:        "Total number of events routed to routes",
			ConstLabels: constLabels,
		},
		[]string{"event_type", "route_name"},
	)

	// Action metrics
	m.actionsExecutedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace:   ns,
			Subsystem:   sub,
			Name:        "actions_executed_total",
			Help:        "Total number of actions executed",
			ConstLabels: constLabels,
		},
		[]string{"action_type", "route_name", "success"},
	)

	m.actionsExecutedDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace:   ns,
			Subsystem:   sub,
			Name:        "actions_executed_duration_seconds",
			Help:        "Duration of action execution in seconds",
			ConstLabels: constLabels,
			Buckets:     buckets,
		},
		[]string{"action_type", "route_name"},
	)

	m.actionRetriesTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace:   ns,
			Subsystem:   sub,
			Name:        "action_retries_total",
			Help:        "Total number of action retry attempts",
			ConstLabels: constLabels,
		},
		[]string{"action_type", "route_name"},
	)

	// Circuit breaker metrics
	m.circuitBreakerStateChanges = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace:   ns,
			Subsystem:   sub,
			Name:        "circuit_breaker_state_changes_total",
			Help:        "Total number of circuit breaker state transitions",
			ConstLabels: constLabels,
		},
		[]string{"endpoint", "from_state", "to_state"},
	)

	m.circuitBreakerRejections = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace:   ns,
			Subsystem:   sub,
			Name:        "circuit_breaker_rejections_total",
			Help:        "Total number of requests rejected by open circuit breakers",
			ConstLabels: constLabels,
		},
		[]string{"endpoint"},
	)

	// Rate limiter metrics
	m.rateLimitWaitsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace:   ns,
			Subsystem:   sub,
			Name:        "rate_limit_waits_total",
			Help:        "Total number of requests that waited for rate limit",
			ConstLabels: constLabels,
		},
		[]string{"endpoint"},
	)

	m.rateLimitWaitsDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace:   ns,
			Subsystem:   sub,
			Name:        "rate_limit_wait_duration_seconds",
			Help:        "Duration spent waiting for rate limit in seconds",
			ConstLabels: constLabels,
			Buckets:     buckets,
		},
		[]string{"endpoint"},
	)

	m.rateLimitRejectionsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace:   ns,
			Subsystem:   sub,
			Name:        "rate_limit_rejections_total",
			Help:        "Total number of requests rejected by rate limiter",
			ConstLabels: constLabels,
		},
		[]string{"endpoint"},
	)

	// DLQ metrics
	m.dlqPushedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace:   ns,
			Subsystem:   sub,
			Name:        "dlq_pushed_total",
			Help:        "Total number of events pushed to dead letter queue",
			ConstLabels: constLabels,
		},
		[]string{"route_name", "action_type", "error_type"},
	)

	m.dlqPoppedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace:   ns,
			Subsystem:   sub,
			Name:        "dlq_popped_total",
			Help:        "Total number of events popped from dead letter queue",
			ConstLabels: constLabels,
		},
		[]string{"route_name", "success"},
	)

	m.dlqDepth = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace:   ns,
			Subsystem:   sub,
			Name:        "dlq_depth",
			Help:        "Current depth of the dead letter queue",
			ConstLabels: constLabels,
		},
	)

	// HTTP metrics
	m.httpRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace:   ns,
			Subsystem:   sub,
			Name:        "http_requests_total",
			Help:        "Total number of HTTP requests made",
			ConstLabels: constLabels,
		},
		[]string{"endpoint", "method", "status_code"},
	)

	m.httpRequestsDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace:   ns,
			Subsystem:   sub,
			Name:        "http_requests_duration_seconds",
			Help:        "Duration of HTTP requests in seconds",
			ConstLabels: constLabels,
			Buckets:     buckets,
		},
		[]string{"endpoint", "method"},
	)

	// Register all metrics
	collectors := []prometheus.Collector{
		m.eventsProcessedTotal,
		m.eventsProcessedDuration,
		m.eventsRoutedTotal,
		m.actionsExecutedTotal,
		m.actionsExecutedDuration,
		m.actionRetriesTotal,
		m.circuitBreakerStateChanges,
		m.circuitBreakerRejections,
		m.rateLimitWaitsTotal,
		m.rateLimitWaitsDuration,
		m.rateLimitRejectionsTotal,
		m.dlqPushedTotal,
		m.dlqPoppedTotal,
		m.dlqDepth,
		m.httpRequestsTotal,
		m.httpRequestsDuration,
	}

	for _, c := range collectors {
		if err := m.registry.Register(c); err != nil {
			return fmt.Errorf("failed to register metric: %w", err)
		}
	}

	return nil
}

// Registry returns the Prometheus registry used by this metrics instance.
func (m *PrometheusMetrics) Registry() *prometheus.Registry {
	return m.registry
}

// Handler returns an HTTP handler for the /metrics endpoint.
func (m *PrometheusMetrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	})
}

// Implement the Metrics interface

func (m *PrometheusMetrics) EventProcessed(eventType, source string, success bool, duration time.Duration) {
	successStr := boolToString(success)
	m.eventsProcessedTotal.WithLabelValues(eventType, source, successStr).Inc()
	m.eventsProcessedDuration.WithLabelValues(eventType, source).Observe(duration.Seconds())
}

func (m *PrometheusMetrics) EventRouted(eventType, routeName string) {
	m.eventsRoutedTotal.WithLabelValues(eventType, routeName).Inc()
}

func (m *PrometheusMetrics) ActionExecuted(actionType, routeName string, success bool, duration time.Duration) {
	successStr := boolToString(success)
	m.actionsExecutedTotal.WithLabelValues(actionType, routeName, successStr).Inc()
	m.actionsExecutedDuration.WithLabelValues(actionType, routeName).Observe(duration.Seconds())
}

func (m *PrometheusMetrics) ActionRetried(actionType, routeName string, attempt int) {
	m.actionRetriesTotal.WithLabelValues(actionType, routeName).Inc()
}

func (m *PrometheusMetrics) CircuitBreakerStateChanged(endpoint string, fromState, toState CircuitState) {
	sanitized := sanitizeEndpoint(endpoint)
	m.circuitBreakerStateChanges.WithLabelValues(sanitized, fromState.String(), toState.String()).Inc()
}

func (m *PrometheusMetrics) CircuitBreakerRejected(endpoint string) {
	sanitized := sanitizeEndpoint(endpoint)
	m.circuitBreakerRejections.WithLabelValues(sanitized).Inc()
}

func (m *PrometheusMetrics) RateLimitWaited(endpoint string, waitDuration time.Duration) {
	sanitized := sanitizeEndpoint(endpoint)
	m.rateLimitWaitsTotal.WithLabelValues(sanitized).Inc()
	m.rateLimitWaitsDuration.WithLabelValues(sanitized).Observe(waitDuration.Seconds())
}

func (m *PrometheusMetrics) RateLimitRejected(endpoint string) {
	sanitized := sanitizeEndpoint(endpoint)
	m.rateLimitRejectionsTotal.WithLabelValues(sanitized).Inc()
}

func (m *PrometheusMetrics) DLQPushed(routeName, actionType, errorType string) {
	m.dlqPushedTotal.WithLabelValues(routeName, actionType, errorType).Inc()
	m.dlqDepth.Inc()
	atomic.AddInt64(&m.dlqDepthValue, 1)
}

func (m *PrometheusMetrics) DLQPopped(routeName string, success bool) {
	successStr := boolToString(success)
	m.dlqPoppedTotal.WithLabelValues(routeName, successStr).Inc()
	m.dlqDepth.Dec()
	atomic.AddInt64(&m.dlqDepthValue, -1)
}

func (m *PrometheusMetrics) DLQDepth() int64 {
	return atomic.LoadInt64(&m.dlqDepthValue)
}

func (m *PrometheusMetrics) HTTPRequestCompleted(endpoint, method string, statusCode int, duration time.Duration) {
	sanitized := sanitizeEndpoint(endpoint)
	statusStr := itoa(statusCode)
	m.httpRequestsTotal.WithLabelValues(sanitized, method, statusStr).Inc()
	m.httpRequestsDuration.WithLabelValues(sanitized, method).Observe(duration.Seconds())
}

// Helper functions

func boolToString(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// sanitizeEndpoint reduces endpoint URL cardinality by extracting host and path pattern.
// Full URLs with query parameters would cause cardinality explosion.
func sanitizeEndpoint(endpoint string) string {
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme == "" || u.Host == "" {
		// If parsing fails or URL is invalid, just use the raw string (truncated)
		if len(endpoint) > 100 {
			return endpoint[:100]
		}
		return endpoint
	}

	// Return scheme://host/path (no query params)
	result := u.Scheme + "://" + u.Host + u.Path

	// Remove trailing slash for consistency
	result = strings.TrimSuffix(result, "/")

	// Truncate very long paths
	if len(result) > 100 {
		return result[:100]
	}

	return result
}
