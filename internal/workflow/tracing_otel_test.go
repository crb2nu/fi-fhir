package workflow

import (
	"context"
	"testing"
	"time"

	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func TestNewOTelTracer(t *testing.T) {
	// Test with default config
	tracer, err := NewOTelTracer(nil)
	if err != nil {
		t.Fatalf("NewOTelTracer() error = %v", err)
	}
	if tracer == nil {
		t.Fatal("NewOTelTracer() returned nil")
	}

	// Clean up
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := tracer.Shutdown(ctx); err != nil {
		t.Errorf("Shutdown() error = %v", err)
	}
}

func TestNewOTelTracerCustomConfig(t *testing.T) {
	config := &OTelConfig{
		ServiceName:    "test-service",
		ServiceVersion: "2.0.0",
		Environment:    "test",
		Sampler:        sdktrace.NeverSample(),
	}

	tracer, err := NewOTelTracer(config)
	if err != nil {
		t.Fatalf("NewOTelTracer() error = %v", err)
	}
	if tracer == nil {
		t.Fatal("NewOTelTracer() returned nil")
	}

	// Verify tracer provider is set
	if tracer.TracerProvider() == nil {
		t.Error("TracerProvider() should not be nil")
	}

	// Clean up
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tracer.Shutdown(ctx)
}

func TestOTelTracerWithExporter(t *testing.T) {
	// Create stdout exporter (writes to /dev/null effectively since we don't check output)
	exporter, err := stdouttrace.New(stdouttrace.WithWriter(discard{}))
	if err != nil {
		t.Fatalf("Failed to create exporter: %v", err)
	}

	config := &OTelConfig{
		ServiceName: "test-service",
		Exporter:    exporter,
	}

	tracer, err := NewOTelTracer(config)
	if err != nil {
		t.Fatalf("NewOTelTracer() error = %v", err)
	}

	// Create some spans
	ctx, span := tracer.StartSpan(context.Background(), "test_span",
		WithSpanKind(SpanKindServer),
		WithAttributes(Attr("test.key", "test_value")),
	)
	if ctx == nil {
		t.Error("Context should not be nil")
	}
	if span == nil {
		t.Error("Span should not be nil")
	}

	span.SetAttribute("dynamic.key", "dynamic_value")
	span.SetStatus(SpanStatusOK, "all good")
	span.AddEvent("test_event", Attr("event.key", "event_value"))
	span.End()

	// Force flush to ensure span is exported
	flushCtx, flushCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer flushCancel()
	if err := tracer.ForceFlush(flushCtx); err != nil {
		t.Errorf("ForceFlush() error = %v", err)
	}

	// Clean up
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	tracer.Shutdown(shutdownCtx)
}

func TestOTelSpanMethods(t *testing.T) {
	tracer, err := NewOTelTracer(nil)
	if err != nil {
		t.Fatalf("NewOTelTracer() error = %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		tracer.Shutdown(ctx)
	}()

	ctx, span := tracer.StartSpan(context.Background(), "method_test")

	// Test SetAttribute with various types
	span.SetAttribute("string", "value")
	span.SetAttribute("int", 42)
	span.SetAttribute("int64", int64(42))
	span.SetAttribute("float64", 3.14)
	span.SetAttribute("bool", true)
	span.SetAttribute("string_slice", []string{"a", "b"})
	span.SetAttribute("int_slice", []int{1, 2, 3})
	span.SetAttribute("custom", struct{ Name string }{Name: "test"})

	// Test SetStatus
	span.SetStatus(SpanStatusOK, "success")
	span.SetStatus(SpanStatusError, "failure")
	span.SetStatus(SpanStatusUnset, "")

	// Test RecordError
	span.RecordError(nil) // Should not panic
	span.RecordError(context.DeadlineExceeded)

	// Test AddEvent
	span.AddEvent("simple_event")
	span.AddEvent("event_with_attrs",
		Attr("attr1", "value1"),
		Attr("attr2", 123),
	)

	span.End()

	// Verify context was updated
	if ctx == nil {
		t.Error("Context should not be nil")
	}
}

func TestOTelSpanKinds(t *testing.T) {
	tracer, err := NewOTelTracer(nil)
	if err != nil {
		t.Fatalf("NewOTelTracer() error = %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		tracer.Shutdown(ctx)
	}()

	kinds := []SpanKind{
		SpanKindInternal,
		SpanKindServer,
		SpanKindClient,
		SpanKindProducer,
		SpanKindConsumer,
	}

	for _, kind := range kinds {
		_, span := tracer.StartSpan(context.Background(), "kind_test", WithSpanKind(kind))
		span.End()
	}
}

func TestDefaultOTelConfig(t *testing.T) {
	cfg := DefaultOTelConfig()

	if cfg.ServiceName != "fi-fhir" {
		t.Errorf("ServiceName = %s, want fi-fhir", cfg.ServiceName)
	}
	if cfg.ServiceVersion != "1.0.0" {
		t.Errorf("ServiceVersion = %s, want 1.0.0", cfg.ServiceVersion)
	}
	if cfg.Environment != "development" {
		t.Errorf("Environment = %s, want development", cfg.Environment)
	}
	if cfg.Sampler == nil {
		t.Error("Sampler should not be nil")
	}
}

func TestOTelTracerWithEngineIntegration(t *testing.T) {
	// Create OTel tracer
	tracer, err := NewOTelTracer(&OTelConfig{
		ServiceName: "integration-test",
	})
	if err != nil {
		t.Fatalf("NewOTelTracer() error = %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		tracer.Shutdown(ctx)
	}()

	// Create workflow
	wf := &Workflow{
		Name:    "otel_test",
		Version: "1.0",
		Routes: []Route{
			{
				Name: "otel_route",
				Filter: Filter{
					EventType: StringOrSlice{"otel_event"},
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

	// Set OTel tracer
	engine.SetTracer(tracer)

	// Process event
	event := map[string]interface{}{
		"type":   "otel_event",
		"source": "test",
	}

	result := engine.ProcessWithContext(context.Background(), event)

	if result.HasErrors() {
		t.Errorf("ProcessWithContext() had errors: %v", result.AllErrors())
	}

	// Force flush
	flushCtx, flushCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer flushCancel()
	tracer.ForceFlush(flushCtx)
}

func TestSpanFromContext(t *testing.T) {
	// Test with no span in context
	span := SpanFromContext(context.Background())
	if span == nil {
		t.Error("SpanFromContext should return a span (no-op if not found)")
	}
	// Should be no-op span
	if _, ok := span.(*noOpSpan); !ok {
		t.Errorf("Expected noOpSpan when no span in context, got %T", span)
	}
}

func TestContextWithSpan(t *testing.T) {
	tracer, err := NewOTelTracer(nil)
	if err != nil {
		t.Fatalf("NewOTelTracer() error = %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		tracer.Shutdown(ctx)
	}()

	// Create a span
	_, span := tracer.StartSpan(context.Background(), "test")
	defer span.End()

	// Put span in context
	ctx := ContextWithSpan(context.Background(), span)
	if ctx == nil {
		t.Error("ContextWithSpan should return a context")
	}

	// Test with no-op span (should return original context)
	noOpCtx := ContextWithSpan(context.Background(), &noOpSpan{})
	if noOpCtx == nil {
		t.Error("ContextWithSpan with noOpSpan should return a context")
	}
}

// discard is an io.Writer that discards all output
type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }
