package workflow

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestNoOpTracer(t *testing.T) {
	tracer := &NoOpTracer{}

	ctx, span := tracer.StartSpan(context.Background(), "test_span")
	if ctx == nil {
		t.Error("Context should not be nil")
	}
	if span == nil {
		t.Error("Span should not be nil")
	}

	// These should not panic
	span.SetAttribute("key", "value")
	span.SetStatus(SpanStatusOK, "")
	span.RecordError(nil)
	span.AddEvent("event")
	span.End()
}

func TestGlobalTracer(t *testing.T) {
	// Default should be NoOpTracer
	tracer := GetGlobalTracer()
	if _, ok := tracer.(*NoOpTracer); !ok {
		t.Errorf("Default tracer should be NoOpTracer, got %T", tracer)
	}

	// Set custom tracer
	mockTracer := &mockTracer{}
	SetGlobalTracer(mockTracer)

	if GetGlobalTracer() != mockTracer {
		t.Error("Global tracer should be the mock tracer")
	}

	// Set nil should reset to NoOpTracer
	SetGlobalTracer(nil)
	if _, ok := GetGlobalTracer().(*NoOpTracer); !ok {
		t.Errorf("Setting nil should reset to NoOpTracer, got %T", GetGlobalTracer())
	}
}

func TestSpanOptions(t *testing.T) {
	tracer := &mockTracer{}

	ctx, span := tracer.StartSpan(context.Background(), "test",
		WithSpanKind(SpanKindClient),
		WithAttributes(
			Attr("key1", "value1"),
			Attr("key2", 42),
		),
	)

	if ctx == nil {
		t.Error("Context should not be nil")
	}

	mockSpan := span.(*mockSpan)
	if mockSpan.kind != SpanKindClient {
		t.Errorf("SpanKind = %v, want SpanKindClient", mockSpan.kind)
	}

	if len(mockSpan.initialAttrs) != 2 {
		t.Errorf("Expected 2 initial attributes, got %d", len(mockSpan.initialAttrs))
	}
}

func TestEngineSetTracer(t *testing.T) {
	wf := &Workflow{Name: "test", Version: "1.0"}
	engine, err := NewEngine(wf)
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}

	// Default should be NoOpTracer
	if _, ok := engine.GetTracer().(*NoOpTracer); !ok {
		t.Errorf("Default tracer should be NoOpTracer, got %T", engine.GetTracer())
	}

	// Set mock tracer
	mockTracer := &mockTracer{}
	engine.SetTracer(mockTracer)

	if engine.GetTracer() != mockTracer {
		t.Error("Tracer should be the mock tracer")
	}

	// Set nil should reset to NoOpTracer
	engine.SetTracer(nil)
	if _, ok := engine.GetTracer().(*NoOpTracer); !ok {
		t.Errorf("Setting nil should reset to NoOpTracer, got %T", engine.GetTracer())
	}
}

func TestEngineProcessWithTracing(t *testing.T) {
	// Create workflow with a route
	wf := &Workflow{
		Name:    "test_workflow",
		Version: "1.0",
		Routes: []Route{
			{
				Name: "test_route",
				Filter: Filter{
					EventType: StringOrSlice{"test_event"},
				},
				Transforms: []Transform{
					{SetField: "processed = true"},
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

	// Set up mock tracer
	tracer := &mockTracer{}
	engine.SetTracer(tracer)

	// Process event
	event := map[string]interface{}{
		"type":   "test_event",
		"source": "test_source",
	}

	result := engine.Process(event)

	if result.HasErrors() {
		t.Errorf("Process() had errors: %v", result.AllErrors())
	}

	// Verify spans were created
	spans := tracer.getSpans()
	if len(spans) == 0 {
		t.Error("Expected spans to be created")
	}

	// Check for root span
	var foundRoot, foundRoute, foundTransform, foundAction bool
	for _, s := range spans {
		switch s.name {
		case SpanNameProcess:
			foundRoot = true
			// Check attributes
			if s.getAttr(AttrEventType) != "test_event" {
				t.Errorf("Root span event_type = %v, want test_event", s.getAttr(AttrEventType))
			}
			if s.getAttr(AttrEventSource) != "test_source" {
				t.Errorf("Root span event_source = %v, want test_source", s.getAttr(AttrEventSource))
			}
		case SpanNameRoute:
			foundRoute = true
			if s.getAttr(AttrRouteName) != "test_route" {
				t.Errorf("Route span route_name = %v, want test_route", s.getAttr(AttrRouteName))
			}
		case SpanNameTransform:
			foundTransform = true
			if s.getAttr(AttrTransformType) != "set_field" {
				t.Errorf("Transform span transform_type = %v, want set_field", s.getAttr(AttrTransformType))
			}
		case SpanNameAction:
			foundAction = true
			if s.getAttr(AttrActionType) != "log" {
				t.Errorf("Action span action_type = %v, want log", s.getAttr(AttrActionType))
			}
		}
	}

	if !foundRoot {
		t.Error("Expected root span (workflow.process)")
	}
	if !foundRoute {
		t.Error("Expected route span (workflow.route)")
	}
	if !foundTransform {
		t.Error("Expected transform span (workflow.transform)")
	}
	if !foundAction {
		t.Error("Expected action span (workflow.action)")
	}
}

func TestEngineProcessWithContextTracing(t *testing.T) {
	wf := &Workflow{
		Name:    "test_workflow",
		Version: "1.0",
		Routes: []Route{
			{
				Name: "simple_route",
				Filter: Filter{
					EventType: StringOrSlice{"simple_event"},
				},
				Actions: []Action{
					{
						Type:   "log",
						Config: map[string]string{},
					},
				},
			},
		},
	}

	engine, err := NewEngine(wf)
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}

	tracer := &mockTracer{}
	engine.SetTracer(tracer)

	// Process with context
	ctx := context.Background()
	event := map[string]interface{}{
		"type": "simple_event",
	}

	result := engine.ProcessWithContext(ctx, event)

	if result.HasErrors() {
		t.Errorf("ProcessWithContext() had errors: %v", result.AllErrors())
	}

	spans := tracer.getSpans()
	if len(spans) < 3 { // root, route, action
		t.Errorf("Expected at least 3 spans, got %d", len(spans))
	}
}

func TestEngineProcessTracingWithErrors(t *testing.T) {
	wf := &Workflow{
		Name:    "test_workflow",
		Version: "1.0",
		Routes: []Route{
			{
				Name: "error_route",
				Filter: Filter{
					EventType: StringOrSlice{"error_event"},
				},
				Actions: []Action{
					{
						Type:   "unknown_action", // This will fail
						Config: map[string]string{},
					},
				},
			},
		},
	}

	engine, err := NewEngine(wf)
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}

	tracer := &mockTracer{}
	engine.SetTracer(tracer)

	event := map[string]interface{}{
		"type": "error_event",
	}

	result := engine.Process(event)

	if !result.HasErrors() {
		t.Error("Expected errors from unknown action")
	}

	// Check that error status was set on spans
	spans := tracer.getSpans()
	var foundErrorSpan bool
	for _, s := range spans {
		if s.status == SpanStatusError {
			foundErrorSpan = true
			break
		}
	}

	if !foundErrorSpan {
		t.Error("Expected at least one span with error status")
	}
}

func TestTracingSpanHierarchy(t *testing.T) {
	wf := &Workflow{
		Name:    "hierarchy_test",
		Version: "1.0",
		Routes: []Route{
			{
				Name: "route1",
				Filter: Filter{
					EventType: StringOrSlice{"hierarchy_event"},
				},
				Transforms: []Transform{
					{SetField: "field1 = value1"},
				},
				Actions: []Action{
					{Type: "log", Config: map[string]string{}},
				},
			},
		},
	}

	engine, err := NewEngine(wf)
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}

	tracer := &mockTracer{}
	engine.SetTracer(tracer)

	event := map[string]interface{}{
		"type": "hierarchy_event",
	}

	engine.Process(event)

	spans := tracer.getSpans()

	// Verify all spans were ended
	for _, s := range spans {
		if !s.ended {
			t.Errorf("Span %s was not ended", s.name)
		}
	}
}

func TestDefaultTracerConfig(t *testing.T) {
	cfg := DefaultTracerConfig()

	if cfg.ServiceName != "fi-fhir" {
		t.Errorf("ServiceName = %s, want fi-fhir", cfg.ServiceName)
	}
	if cfg.ServiceVersion != "1.0.0" {
		t.Errorf("ServiceVersion = %s, want 1.0.0", cfg.ServiceVersion)
	}
	if cfg.Environment != "development" {
		t.Errorf("Environment = %s, want development", cfg.Environment)
	}
}

func TestAttrHelper(t *testing.T) {
	attr := Attr("test_key", "test_value")

	if attr.Key != "test_key" {
		t.Errorf("Attr.Key = %s, want test_key", attr.Key)
	}
	if attr.Value != "test_value" {
		t.Errorf("Attr.Value = %v, want test_value", attr.Value)
	}
}

// mockTracer implements Tracer for testing
type mockTracer struct {
	mu    sync.Mutex
	spans []*mockSpan
}

func (t *mockTracer) StartSpan(ctx context.Context, name string, opts ...SpanOption) (context.Context, Span) {
	cfg := &spanConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	span := &mockSpan{
		name:         name,
		kind:         cfg.kind,
		initialAttrs: cfg.attributes,
		attrs:        make(map[string]interface{}),
		startTime:    time.Now(),
	}

	// Copy initial attrs to attrs map
	for _, a := range cfg.attributes {
		span.attrs[a.Key] = a.Value
	}

	t.mu.Lock()
	t.spans = append(t.spans, span)
	t.mu.Unlock()

	return ctx, span
}

func (t *mockTracer) getSpans() []*mockSpan {
	t.mu.Lock()
	defer t.mu.Unlock()
	result := make([]*mockSpan, len(t.spans))
	copy(result, t.spans)
	return result
}

// mockSpan implements Span for testing
type mockSpan struct {
	name         string
	kind         SpanKind
	initialAttrs []SpanAttribute
	attrs        map[string]interface{}
	status       SpanStatus
	statusMsg    string
	errors       []error
	events       []string
	startTime    time.Time
	ended        bool
	mu           sync.Mutex
}

func (s *mockSpan) End() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ended = true
}

func (s *mockSpan) SetAttribute(key string, value interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.attrs[key] = value
}

func (s *mockSpan) SetStatus(code SpanStatus, message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status = code
	s.statusMsg = message
}

func (s *mockSpan) RecordError(err error) {
	if err == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.errors = append(s.errors, err)
}

func (s *mockSpan) AddEvent(name string, attrs ...SpanAttribute) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, name)
}

func (s *mockSpan) getAttr(key string) interface{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.attrs[key]
}
