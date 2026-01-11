//nolint:errcheck // Test file - error checking intentionally relaxed
package workflow

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

func TestNewPrometheusMetrics(t *testing.T) {
	// Test with default config
	m, err := NewPrometheusMetrics(nil)
	if err != nil {
		t.Fatalf("NewPrometheusMetrics() error = %v", err)
	}
	if m == nil {
		t.Fatal("NewPrometheusMetrics() returned nil")
	}
	if m.registry == nil {
		t.Error("Registry should not be nil")
	}
}

func TestNewPrometheusMetricsCustomConfig(t *testing.T) {
	customRegistry := prometheus.NewRegistry()
	config := &PrometheusConfig{
		Namespace:   "custom",
		Subsystem:   "test",
		ConstLabels: prometheus.Labels{"env": "test"},
		Registry:    customRegistry,
	}

	m, err := NewPrometheusMetrics(config)
	if err != nil {
		t.Fatalf("NewPrometheusMetrics() error = %v", err)
	}
	if m.Registry() != customRegistry {
		t.Error("Should use custom registry")
	}
}

func TestPrometheusMetricsEventProcessed(t *testing.T) {
	m, err := NewPrometheusMetrics(nil)
	if err != nil {
		t.Fatalf("NewPrometheusMetrics() error = %v", err)
	}

	// Record some events
	m.EventProcessed("patient_admit", "epic", true, 100*time.Millisecond)
	m.EventProcessed("patient_admit", "epic", false, 50*time.Millisecond)
	m.EventProcessed("lab_result", "cerner", true, 200*time.Millisecond)

	// Verify via handler output
	handler := m.Handler()
	req := httptest.NewRequest("GET", "/metrics", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	body := rr.Body.String()

	// Check counter metrics exist
	if !strings.Contains(body, "fi_fhir_workflow_events_processed_total") {
		t.Error("Expected events_processed_total metric")
	}

	// Check histogram metrics exist
	if !strings.Contains(body, "fi_fhir_workflow_events_processed_duration_seconds") {
		t.Error("Expected events_processed_duration_seconds metric")
	}

	// Check specific labels
	if !strings.Contains(body, `event_type="patient_admit"`) {
		t.Error("Expected event_type label")
	}
	if !strings.Contains(body, `source="epic"`) {
		t.Error("Expected source label")
	}
}

func TestPrometheusMetricsActionExecuted(t *testing.T) {
	m, err := NewPrometheusMetrics(nil)
	if err != nil {
		t.Fatalf("NewPrometheusMetrics() error = %v", err)
	}

	m.ActionExecuted("webhook", "alert_route", true, 150*time.Millisecond)
	m.ActionExecuted("fhir", "fhir_route", false, 500*time.Millisecond)

	body := getMetricsBody(t, m)

	if !strings.Contains(body, "fi_fhir_workflow_actions_executed_total") {
		t.Error("Expected actions_executed_total metric")
	}
	if !strings.Contains(body, `action_type="webhook"`) {
		t.Error("Expected action_type label")
	}
	if !strings.Contains(body, `route_name="alert_route"`) {
		t.Error("Expected route_name label")
	}
}

func TestPrometheusMetricsActionRetried(t *testing.T) {
	m, err := NewPrometheusMetrics(nil)
	if err != nil {
		t.Fatalf("NewPrometheusMetrics() error = %v", err)
	}

	m.ActionRetried("webhook", "retry_route", 1)
	m.ActionRetried("webhook", "retry_route", 2)
	m.ActionRetried("webhook", "retry_route", 3)

	body := getMetricsBody(t, m)

	if !strings.Contains(body, "fi_fhir_workflow_action_retries_total") {
		t.Error("Expected action_retries_total metric")
	}
}

func TestPrometheusMetricsCircuitBreaker(t *testing.T) {
	m, err := NewPrometheusMetrics(nil)
	if err != nil {
		t.Fatalf("NewPrometheusMetrics() error = %v", err)
	}

	m.CircuitBreakerStateChanged("https://api.example.com", CircuitClosed, CircuitOpen)
	m.CircuitBreakerRejected("https://api.example.com")

	body := getMetricsBody(t, m)

	if !strings.Contains(body, "fi_fhir_workflow_circuit_breaker_state_changes_total") {
		t.Error("Expected circuit_breaker_state_changes_total metric")
	}
	if !strings.Contains(body, "fi_fhir_workflow_circuit_breaker_rejections_total") {
		t.Error("Expected circuit_breaker_rejections_total metric")
	}
	if !strings.Contains(body, `from_state="closed"`) {
		t.Error("Expected from_state label")
	}
	if !strings.Contains(body, `to_state="open"`) {
		t.Error("Expected to_state label")
	}
}

func TestPrometheusMetricsRateLimit(t *testing.T) {
	m, err := NewPrometheusMetrics(nil)
	if err != nil {
		t.Fatalf("NewPrometheusMetrics() error = %v", err)
	}

	m.RateLimitWaited("https://api.example.com", 50*time.Millisecond)
	m.RateLimitRejected("https://api.example.com")

	body := getMetricsBody(t, m)

	if !strings.Contains(body, "fi_fhir_workflow_rate_limit_waits_total") {
		t.Error("Expected rate_limit_waits_total metric")
	}
	if !strings.Contains(body, "fi_fhir_workflow_rate_limit_wait_duration_seconds") {
		t.Error("Expected rate_limit_wait_duration_seconds metric")
	}
	if !strings.Contains(body, "fi_fhir_workflow_rate_limit_rejections_total") {
		t.Error("Expected rate_limit_rejections_total metric")
	}
}

func TestPrometheusMetricsDLQ(t *testing.T) {
	m, err := NewPrometheusMetrics(nil)
	if err != nil {
		t.Fatalf("NewPrometheusMetrics() error = %v", err)
	}

	// Push 3 events
	m.DLQPushed("route1", "webhook", "timeout")
	m.DLQPushed("route1", "webhook", "server_error")
	m.DLQPushed("route2", "fhir", "auth_error")

	if m.DLQDepth() != 3 {
		t.Errorf("DLQDepth() = %d, want 3", m.DLQDepth())
	}

	// Pop 2 events
	m.DLQPopped("route1", true)
	m.DLQPopped("route1", false)

	if m.DLQDepth() != 1 {
		t.Errorf("DLQDepth() = %d, want 1", m.DLQDepth())
	}

	body := getMetricsBody(t, m)

	if !strings.Contains(body, "fi_fhir_workflow_dlq_pushed_total") {
		t.Error("Expected dlq_pushed_total metric")
	}
	if !strings.Contains(body, "fi_fhir_workflow_dlq_popped_total") {
		t.Error("Expected dlq_popped_total metric")
	}
	if !strings.Contains(body, "fi_fhir_workflow_dlq_depth") {
		t.Error("Expected dlq_depth metric")
	}
}

func TestPrometheusMetricsHTTPRequest(t *testing.T) {
	m, err := NewPrometheusMetrics(nil)
	if err != nil {
		t.Fatalf("NewPrometheusMetrics() error = %v", err)
	}

	m.HTTPRequestCompleted("https://api.example.com/v1/patients", "POST", 201, 100*time.Millisecond)
	m.HTTPRequestCompleted("https://api.example.com/v1/patients", "POST", 500, 200*time.Millisecond)
	m.HTTPRequestCompleted("https://api.example.com/v1/patients", "GET", 200, 50*time.Millisecond)

	body := getMetricsBody(t, m)

	if !strings.Contains(body, "fi_fhir_workflow_http_requests_total") {
		t.Error("Expected http_requests_total metric")
	}
	if !strings.Contains(body, "fi_fhir_workflow_http_requests_duration_seconds") {
		t.Error("Expected http_requests_duration_seconds metric")
	}
	if !strings.Contains(body, `method="POST"`) {
		t.Error("Expected method label")
	}
	if !strings.Contains(body, `status_code="201"`) {
		t.Error("Expected status_code label")
	}
}

func TestPrometheusMetricsHandler(t *testing.T) {
	m, err := NewPrometheusMetrics(nil)
	if err != nil {
		t.Fatalf("NewPrometheusMetrics() error = %v", err)
	}

	// Record some metrics
	m.EventProcessed("test", "source", true, 10*time.Millisecond)

	// Create test server
	handler := m.Handler()
	server := httptest.NewServer(handler)
	defer server.Close()

	// Make request
	resp, err := http.Get(server.URL)
	if err != nil {
		t.Fatalf("GET /metrics failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Status = %d, want 200", resp.StatusCode)
	}

	// Check content type
	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "text/plain") && !strings.Contains(contentType, "openmetrics") {
		t.Errorf("Content-Type = %s, want text/plain or openmetrics", contentType)
	}
}

func TestSanitizeEndpoint(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{
			input: "https://api.example.com/v1/patients?query=test",
			want:  "https://api.example.com/v1/patients",
		},
		{
			input: "https://api.example.com/v1/patients/",
			want:  "https://api.example.com/v1/patients",
		},
		{
			input: "https://api.example.com/",
			want:  "https://api.example.com",
		},
		{
			input: "http://localhost:8080/webhook",
			want:  "http://localhost:8080/webhook",
		},
		{
			input: "invalid-url",
			want:  "invalid-url",
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := sanitizeEndpoint(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeEndpoint(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestPrometheusMetricsWithEngine(t *testing.T) {
	// Create a workflow with a simple route
	wf := &Workflow{
		Name:    "test_workflow",
		Version: "1.0",
		Routes: []Route{
			{
				Name: "test_route",
				Filter: Filter{
					EventType: StringOrSlice{"test_event"},
				},
				Actions: []Action{
					{
						Type:   "log",
						Config: map[string]string{"level": "info"},
					},
				},
			},
		},
	}

	engine, err := NewEngine(wf)
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}

	// Set up Prometheus metrics
	promMetrics, err := NewPrometheusMetrics(nil)
	if err != nil {
		t.Fatalf("NewPrometheusMetrics() error = %v", err)
	}
	engine.SetMetrics(promMetrics)

	// Process an event
	event := map[string]interface{}{
		"type":   "test_event",
		"source": "test_source",
		"data":   "test_data",
	}
	result := engine.Process(event)

	if result.HasErrors() {
		t.Errorf("Process() had errors: %v", result.AllErrors())
	}

	// Verify metrics were recorded
	body := getMetricsBody(t, promMetrics)

	if !strings.Contains(body, `event_type="test_event"`) {
		t.Error("Expected event_type label in metrics")
	}
	if !strings.Contains(body, `route_name="test_route"`) {
		t.Error("Expected route_name label in metrics")
	}
	if !strings.Contains(body, `action_type="log"`) {
		t.Error("Expected action_type label in metrics")
	}
}

func TestPrometheusMetricsCustomNamespace(t *testing.T) {
	config := &PrometheusConfig{
		Namespace: "myapp",
		Subsystem: "events",
	}

	m, err := NewPrometheusMetrics(config)
	if err != nil {
		t.Fatalf("NewPrometheusMetrics() error = %v", err)
	}

	m.EventProcessed("test", "source", true, 10*time.Millisecond)

	body := getMetricsBody(t, m)

	if !strings.Contains(body, "myapp_events_events_processed_total") {
		t.Error("Expected custom namespace in metric name")
	}
}

func TestPrometheusMetricsConstLabels(t *testing.T) {
	config := &PrometheusConfig{
		ConstLabels: prometheus.Labels{
			"service": "fi-fhir",
			"env":     "test",
		},
	}

	m, err := NewPrometheusMetrics(config)
	if err != nil {
		t.Fatalf("NewPrometheusMetrics() error = %v", err)
	}

	m.EventProcessed("test", "source", true, 10*time.Millisecond)

	body := getMetricsBody(t, m)

	if !strings.Contains(body, `service="fi-fhir"`) {
		t.Error("Expected service const label")
	}
	if !strings.Contains(body, `env="test"`) {
		t.Error("Expected env const label")
	}
}

// Helper function to get metrics output as string
func getMetricsBody(t *testing.T, m *PrometheusMetrics) string {
	t.Helper()
	handler := m.Handler()
	req := httptest.NewRequest("GET", "/metrics", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	body, err := io.ReadAll(rr.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}
	return string(body)
}
