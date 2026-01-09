package workflow

import (
	"testing"
	"time"
)

func TestNoOpMetricsDoesNotPanic(t *testing.T) {
	m := &NoOpMetrics{}

	// Ensure none of these methods panic
	m.EventProcessed("patient_admit", "epic", true, 100*time.Millisecond)
	m.EventRouted("patient_admit", "test_route")
	m.ActionExecuted("webhook", "test_route", true, 50*time.Millisecond)
	m.ActionRetried("webhook", "test_route", 2)
	m.CircuitBreakerStateChanged("http://example.com", CircuitClosed, CircuitOpen)
	m.CircuitBreakerRejected("http://example.com")
	m.RateLimitWaited("http://example.com", 10*time.Millisecond)
	m.RateLimitRejected("http://example.com")
	m.DLQPushed("test_route", "webhook", "timeout")
	m.DLQPopped("test_route", true)
	_ = m.DLQDepth()
	m.HTTPRequestCompleted("http://example.com", "POST", 200, 100*time.Millisecond)
}

func TestInMemoryMetricsEventProcessed(t *testing.T) {
	m := NewInMemoryMetrics()

	// Record some events
	m.EventProcessed("patient_admit", "epic", true, 100*time.Millisecond)
	m.EventProcessed("patient_admit", "epic", true, 200*time.Millisecond)
	m.EventProcessed("patient_admit", "epic", false, 50*time.Millisecond)
	m.EventProcessed("lab_result", "cerner", true, 150*time.Millisecond)

	// Check counters
	if got := m.GetCounter("events_processed", "patient_admit:epic:true"); got != 2 {
		t.Errorf("expected 2 successful patient_admit events, got %d", got)
	}
	if got := m.GetCounter("events_processed", "patient_admit:epic:false"); got != 1 {
		t.Errorf("expected 1 failed patient_admit event, got %d", got)
	}
	if got := m.GetCounter("events_processed", "lab_result:cerner:true"); got != 1 {
		t.Errorf("expected 1 lab_result event, got %d", got)
	}

	// Check snapshot
	snapshot := m.Snapshot()
	if snapshot.EventsProcessed["patient_admit:epic:true"] != 2 {
		t.Errorf("snapshot mismatch for patient_admit")
	}

	// Check duration stats
	durStats := snapshot.EventDurations["patient_admit:epic"]
	if durStats.Count != 3 {
		t.Errorf("expected 3 duration records, got %d", durStats.Count)
	}
	if durStats.Min != 50 {
		t.Errorf("expected min 50ms, got %f", durStats.Min)
	}
	if durStats.Max != 200 {
		t.Errorf("expected max 200ms, got %f", durStats.Max)
	}
}

func TestInMemoryMetricsEventRouted(t *testing.T) {
	m := NewInMemoryMetrics()

	m.EventRouted("patient_admit", "fhir_route")
	m.EventRouted("patient_admit", "fhir_route")
	m.EventRouted("patient_admit", "log_route")

	if got := m.GetCounter("events_routed", "patient_admit:fhir_route"); got != 2 {
		t.Errorf("expected 2 events to fhir_route, got %d", got)
	}
	if got := m.GetCounter("events_routed", "patient_admit:log_route"); got != 1 {
		t.Errorf("expected 1 event to log_route, got %d", got)
	}
}

func TestInMemoryMetricsActionExecuted(t *testing.T) {
	m := NewInMemoryMetrics()

	m.ActionExecuted("webhook", "alerts", true, 100*time.Millisecond)
	m.ActionExecuted("webhook", "alerts", true, 150*time.Millisecond)
	m.ActionExecuted("webhook", "alerts", false, 5000*time.Millisecond)
	m.ActionExecuted("fhir", "patient_sync", true, 200*time.Millisecond)

	if got := m.GetCounter("actions_executed", "webhook:alerts:true"); got != 2 {
		t.Errorf("expected 2 successful webhook actions, got %d", got)
	}
	if got := m.GetCounter("actions_executed", "webhook:alerts:false"); got != 1 {
		t.Errorf("expected 1 failed webhook action, got %d", got)
	}

	snapshot := m.Snapshot()
	durStats := snapshot.ActionDurations["webhook:alerts"]
	if durStats.Count != 3 {
		t.Errorf("expected 3 duration records, got %d", durStats.Count)
	}
}

func TestInMemoryMetricsActionRetried(t *testing.T) {
	m := NewInMemoryMetrics()

	m.ActionRetried("webhook", "alerts", 1)
	m.ActionRetried("webhook", "alerts", 2)
	m.ActionRetried("webhook", "alerts", 3)
	m.ActionRetried("fhir", "sync", 1)

	if got := m.GetCounter("action_retries", "webhook:alerts"); got != 3 {
		t.Errorf("expected 3 retries for webhook:alerts, got %d", got)
	}
	if got := m.GetCounter("action_retries", "fhir:sync"); got != 1 {
		t.Errorf("expected 1 retry for fhir:sync, got %d", got)
	}
}

func TestInMemoryMetricsCircuitBreaker(t *testing.T) {
	m := NewInMemoryMetrics()

	m.CircuitBreakerStateChanged("http://api.example.com", CircuitClosed, CircuitOpen)
	m.CircuitBreakerStateChanged("http://api.example.com", CircuitOpen, CircuitHalfOpen)
	m.CircuitBreakerRejected("http://api.example.com")
	m.CircuitBreakerRejected("http://api.example.com")

	if got := m.GetCounter("cb_state_changes", "http://api.example.com:closed:open"); got != 1 {
		t.Errorf("expected 1 closed->open transition, got %d", got)
	}
	if got := m.GetCounter("cb_rejections", "http://api.example.com"); got != 2 {
		t.Errorf("expected 2 rejections, got %d", got)
	}
}

func TestInMemoryMetricsRateLimit(t *testing.T) {
	m := NewInMemoryMetrics()

	m.RateLimitWaited("http://api.example.com", 10*time.Millisecond)
	m.RateLimitWaited("http://api.example.com", 20*time.Millisecond)
	m.RateLimitRejected("http://api.example.com")

	if got := m.GetCounter("rate_limit_rejections", "http://api.example.com"); got != 1 {
		t.Errorf("expected 1 rejection, got %d", got)
	}

	snapshot := m.Snapshot()
	durStats := snapshot.RateLimitWaits["http://api.example.com"]
	if durStats.Count != 2 {
		t.Errorf("expected 2 wait records, got %d", durStats.Count)
	}
	if durStats.Avg != 15 {
		t.Errorf("expected avg 15ms, got %f", durStats.Avg)
	}
}

func TestInMemoryMetricsDLQ(t *testing.T) {
	m := NewInMemoryMetrics()

	// Push some events to DLQ
	m.DLQPushed("route1", "webhook", "timeout")
	m.DLQPushed("route1", "webhook", "timeout")
	m.DLQPushed("route2", "fhir", "auth_error")

	if got := m.DLQDepth(); got != 3 {
		t.Errorf("expected DLQ depth 3, got %d", got)
	}

	if got := m.GetCounter("dlq_pushed", "route1:webhook:timeout"); got != 2 {
		t.Errorf("expected 2 timeout pushes, got %d", got)
	}

	// Pop some events
	m.DLQPopped("route1", true)
	m.DLQPopped("route1", false)

	if got := m.DLQDepth(); got != 1 {
		t.Errorf("expected DLQ depth 1 after pops, got %d", got)
	}

	if got := m.GetCounter("dlq_popped", "route1:true"); got != 1 {
		t.Errorf("expected 1 successful pop, got %d", got)
	}
}

func TestInMemoryMetricsHTTPRequest(t *testing.T) {
	m := NewInMemoryMetrics()

	m.HTTPRequestCompleted("http://api.example.com", "POST", 200, 100*time.Millisecond)
	m.HTTPRequestCompleted("http://api.example.com", "POST", 200, 150*time.Millisecond)
	m.HTTPRequestCompleted("http://api.example.com", "POST", 500, 50*time.Millisecond)
	m.HTTPRequestCompleted("http://api.example.com", "GET", 200, 30*time.Millisecond)

	if got := m.GetCounter("http_requests", "http://api.example.com:POST:200"); got != 2 {
		t.Errorf("expected 2 POST 200 requests, got %d", got)
	}
	if got := m.GetCounter("http_requests", "http://api.example.com:POST:500"); got != 1 {
		t.Errorf("expected 1 POST 500 request, got %d", got)
	}

	snapshot := m.Snapshot()
	durStats := snapshot.HTTPRequestDurations["http://api.example.com:POST"]
	if durStats.Count != 3 {
		t.Errorf("expected 3 POST duration records, got %d", durStats.Count)
	}
}

func TestInMemoryMetricsReset(t *testing.T) {
	m := NewInMemoryMetrics()

	m.EventProcessed("patient_admit", "epic", true, 100*time.Millisecond)
	m.ActionExecuted("webhook", "alerts", true, 50*time.Millisecond)
	m.DLQPushed("route1", "webhook", "timeout")

	if m.DLQDepth() != 1 {
		t.Error("expected DLQ depth 1 before reset")
	}

	m.Reset()

	if got := m.GetCounter("events_processed", "patient_admit:epic:true"); got != 0 {
		t.Errorf("expected 0 after reset, got %d", got)
	}
	if got := m.GetCounter("actions_executed", "webhook:alerts:true"); got != 0 {
		t.Errorf("expected 0 after reset, got %d", got)
	}
	if m.DLQDepth() != 0 {
		t.Error("expected DLQ depth 0 after reset")
	}
}

func TestInMemoryMetricsSnapshot(t *testing.T) {
	m := NewInMemoryMetrics()

	m.EventProcessed("patient_admit", "epic", true, 100*time.Millisecond)
	m.EventRouted("patient_admit", "fhir_route")
	m.ActionExecuted("fhir", "fhir_route", true, 200*time.Millisecond)
	m.DLQPushed("dlq_route", "webhook", "connection_error")

	snapshot := m.Snapshot()

	// Verify all categories are present
	if len(snapshot.EventsProcessed) == 0 {
		t.Error("expected EventsProcessed in snapshot")
	}
	if len(snapshot.EventsRouted) == 0 {
		t.Error("expected EventsRouted in snapshot")
	}
	if len(snapshot.ActionsExecuted) == 0 {
		t.Error("expected ActionsExecuted in snapshot")
	}
	if snapshot.DLQDepth != 1 {
		t.Errorf("expected DLQ depth 1 in snapshot, got %d", snapshot.DLQDepth)
	}
}

func TestGlobalMetrics(t *testing.T) {
	// Default should be NoOp
	original := GetGlobalMetrics()
	if _, ok := original.(*NoOpMetrics); !ok {
		t.Error("expected default global metrics to be NoOpMetrics")
	}

	// Set custom metrics
	custom := NewInMemoryMetrics()
	SetGlobalMetrics(custom)

	if GetGlobalMetrics() != custom {
		t.Error("expected custom metrics to be set")
	}

	// Record some metrics
	GetGlobalMetrics().EventProcessed("test", "source", true, time.Millisecond)

	// Verify through custom instance
	if got := custom.GetCounter("events_processed", "test:source:true"); got != 1 {
		t.Errorf("expected 1 event, got %d", got)
	}

	// Reset to NoOp
	SetGlobalMetrics(&NoOpMetrics{})
}

func TestDefaultMetricsConfig(t *testing.T) {
	config := DefaultMetricsConfig()

	if config.Enabled {
		t.Error("expected Enabled to be false by default")
	}
	if config.Prefix != "fi_fhir_workflow" {
		t.Errorf("expected prefix 'fi_fhir_workflow', got '%s'", config.Prefix)
	}
	if config.Labels == nil {
		t.Error("expected Labels to be initialized")
	}
}

func TestInMemoryMetricsConcurrency(t *testing.T) {
	m := NewInMemoryMetrics()

	// Spawn goroutines that record metrics concurrently
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 100; j++ {
				m.EventProcessed("test", "source", true, time.Millisecond)
				m.ActionExecuted("webhook", "route", true, time.Millisecond)
				m.DLQPushed("route", "webhook", "timeout")
				m.DLQPopped("route", true)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify counts
	if got := m.GetCounter("events_processed", "test:source:true"); got != 1000 {
		t.Errorf("expected 1000 events, got %d", got)
	}
	if got := m.GetCounter("actions_executed", "webhook:route:true"); got != 1000 {
		t.Errorf("expected 1000 actions, got %d", got)
	}

	// DLQ depth should be 0 (equal pushes and pops)
	if m.DLQDepth() != 0 {
		t.Errorf("expected DLQ depth 0, got %d", m.DLQDepth())
	}
}

func TestItoa(t *testing.T) {
	tests := []struct {
		input    int
		expected string
	}{
		{0, "0"},
		{1, "1"},
		{10, "10"},
		{123, "123"},
		{200, "200"},
		{500, "500"},
		{-1, "-1"},
		{-123, "-123"},
	}

	for _, tt := range tests {
		if got := itoa(tt.input); got != tt.expected {
			t.Errorf("itoa(%d) = %s, want %s", tt.input, got, tt.expected)
		}
	}
}

// Engine Integration Tests

func TestEngineMetricsIntegration(t *testing.T) {
	workflow := &Workflow{
		Name:    "test",
		Version: "1.0",
		Routes: []Route{
			{
				Name:   "log_route",
				Filter: Filter{EventType: StringOrSlice{"test_event"}},
				Actions: []Action{
					{Type: "log", Config: map[string]string{"level": "info"}},
				},
			},
		},
	}

	engine, err := NewEngine(workflow)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	// Configure in-memory metrics
	metrics := NewInMemoryMetrics()
	engine.SetMetrics(metrics)

	// Process an event
	event := map[string]interface{}{
		"type":   "test_event",
		"source": "test_source",
		"data":   "test_data",
	}

	result := engine.Process(event)
	if result.HasErrors() {
		t.Errorf("unexpected errors: %v", result.AllErrors())
	}

	// Verify metrics were recorded
	if got := metrics.GetCounter("events_processed", "test_event:test_source:true"); got != 1 {
		t.Errorf("expected 1 event processed, got %d", got)
	}

	if got := metrics.GetCounter("events_routed", "test_event:log_route"); got != 1 {
		t.Errorf("expected 1 event routed to log_route, got %d", got)
	}

	if got := metrics.GetCounter("actions_executed", "log:log_route:true"); got != 1 {
		t.Errorf("expected 1 successful log action, got %d", got)
	}

	// Verify duration stats
	snapshot := metrics.Snapshot()
	if len(snapshot.EventDurations) == 0 {
		t.Error("expected event duration to be recorded")
	}
	if len(snapshot.ActionDurations) == 0 {
		t.Error("expected action duration to be recorded")
	}
}

func TestEngineMetricsFailedAction(t *testing.T) {
	workflow := &Workflow{
		Name:    "test",
		Version: "1.0",
		Routes: []Route{
			{
				Name:   "webhook_route",
				Filter: Filter{EventType: StringOrSlice{"test_event"}},
				Actions: []Action{
					{Type: "webhook", Config: map[string]string{
						"url":       "http://localhost:99999/invalid", // Will fail
						"retry_max": "0",                              // No retries
					}},
				},
			},
		},
	}

	engine, err := NewEngine(workflow)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	// Configure in-memory metrics and DLQ
	metrics := NewInMemoryMetrics()
	engine.SetMetrics(metrics)
	engine.SetDLQ(NewMemoryDLQ())

	// Process an event (should fail)
	event := map[string]interface{}{
		"type":   "test_event",
		"source": "test_source",
	}

	result := engine.Process(event)
	if !result.HasErrors() {
		t.Error("expected errors from failed webhook")
	}

	// Verify metrics show failure
	if got := metrics.GetCounter("events_processed", "test_event:test_source:false"); got != 1 {
		t.Errorf("expected 1 failed event, got %d", got)
	}

	if got := metrics.GetCounter("actions_executed", "webhook:webhook_route:false"); got != 1 {
		t.Errorf("expected 1 failed webhook action, got %d", got)
	}

	// Verify DLQ push metric
	snapshot := metrics.Snapshot()
	dlqPushCount := int64(0)
	for k, v := range snapshot.DLQPushed {
		if v > 0 {
			t.Logf("DLQ push recorded: %s = %d", k, v)
			dlqPushCount += v
		}
	}
	if dlqPushCount != 1 {
		t.Errorf("expected 1 DLQ push, got %d", dlqPushCount)
	}
}

func TestEngineSetMetricsNil(t *testing.T) {
	workflow := &Workflow{Name: "test", Version: "1.0"}
	engine, _ := NewEngine(workflow)

	// Setting nil should default to NoOpMetrics
	engine.SetMetrics(nil)

	if _, ok := engine.GetMetrics().(*NoOpMetrics); !ok {
		t.Error("expected NoOpMetrics when setting nil")
	}
}

func TestEngineGetMetrics(t *testing.T) {
	workflow := &Workflow{Name: "test", Version: "1.0"}
	engine, _ := NewEngine(workflow)

	// Default should be NoOpMetrics
	if _, ok := engine.GetMetrics().(*NoOpMetrics); !ok {
		t.Error("expected NoOpMetrics as default")
	}

	// Set custom metrics
	custom := NewInMemoryMetrics()
	engine.SetMetrics(custom)

	if engine.GetMetrics() != custom {
		t.Error("expected custom metrics to be returned")
	}
}
