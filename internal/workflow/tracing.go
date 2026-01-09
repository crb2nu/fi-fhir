package workflow

import (
	"context"
	"sync"
)

// Tracer defines the interface for distributed tracing.
// Users can implement this interface to integrate with their preferred
// tracing backend (OpenTelemetry, Jaeger, Zipkin, Datadog, etc.).
type Tracer interface {
	// StartSpan starts a new span with the given name.
	// The returned context contains the new span.
	StartSpan(ctx context.Context, name string, opts ...SpanOption) (context.Context, Span)
}

// Span represents a single operation within a trace.
type Span interface {
	// End completes the span. Must be called when the operation is done.
	End()

	// SetAttribute adds a key-value attribute to the span.
	SetAttribute(key string, value interface{})

	// SetStatus sets the span status (OK, Error).
	SetStatus(code SpanStatus, message string)

	// RecordError records an error on the span.
	RecordError(err error)

	// AddEvent adds a timestamped event to the span.
	AddEvent(name string, attrs ...SpanAttribute)
}

// SpanStatus represents the status of a span.
type SpanStatus int

const (
	SpanStatusUnset SpanStatus = iota
	SpanStatusOK
	SpanStatusError
)

// SpanAttribute represents a key-value pair for span events.
type SpanAttribute struct {
	Key   string
	Value interface{}
}

// SpanOption configures a span at creation time.
type SpanOption func(*spanConfig)

type spanConfig struct {
	kind       SpanKind
	attributes []SpanAttribute
}

// SpanKind represents the type of span.
type SpanKind int

const (
	SpanKindInternal SpanKind = iota
	SpanKindServer
	SpanKindClient
	SpanKindProducer
	SpanKindConsumer
)

// WithSpanKind sets the span kind.
func WithSpanKind(kind SpanKind) SpanOption {
	return func(cfg *spanConfig) {
		cfg.kind = kind
	}
}

// WithAttributes sets initial attributes on the span.
func WithAttributes(attrs ...SpanAttribute) SpanOption {
	return func(cfg *spanConfig) {
		cfg.attributes = append(cfg.attributes, attrs...)
	}
}

// Attr is a convenience function to create a SpanAttribute.
func Attr(key string, value interface{}) SpanAttribute {
	return SpanAttribute{Key: key, Value: value}
}

// NoOpTracer is a no-op implementation that creates no-op spans.
// Use this as the default when tracing is not needed.
type NoOpTracer struct{}

// StartSpan returns a no-op span.
func (n *NoOpTracer) StartSpan(ctx context.Context, name string, opts ...SpanOption) (context.Context, Span) {
	return ctx, &noOpSpan{}
}

type noOpSpan struct{}

func (n *noOpSpan) End()                                          {}
func (n *noOpSpan) SetAttribute(key string, value interface{})    {}
func (n *noOpSpan) SetStatus(code SpanStatus, message string)     {}
func (n *noOpSpan) RecordError(err error)                         {}
func (n *noOpSpan) AddEvent(name string, attrs ...SpanAttribute) {}

// Global tracer instance (can be set by user)
var globalTracer Tracer = &NoOpTracer{}
var globalTracerMu sync.RWMutex

// SetGlobalTracer sets the global tracer.
func SetGlobalTracer(t Tracer) {
	globalTracerMu.Lock()
	defer globalTracerMu.Unlock()
	if t == nil {
		globalTracer = &NoOpTracer{}
	} else {
		globalTracer = t
	}
}

// GetGlobalTracer returns the current global tracer.
func GetGlobalTracer() Tracer {
	globalTracerMu.RLock()
	defer globalTracerMu.RUnlock()
	return globalTracer
}

// TracerConfig holds configuration for tracing.
type TracerConfig struct {
	// ServiceName is the name of the service for tracing
	ServiceName string

	// ServiceVersion is the version of the service
	ServiceVersion string

	// Environment is the deployment environment (production, staging, etc.)
	Environment string
}

// DefaultTracerConfig returns sensible defaults.
func DefaultTracerConfig() TracerConfig {
	return TracerConfig{
		ServiceName:    "fi-fhir",
		ServiceVersion: "1.0.0",
		Environment:    "development",
	}
}

// Span names for workflow operations
const (
	SpanNameProcess   = "workflow.process"
	SpanNameRoute     = "workflow.route"
	SpanNameTransform = "workflow.transform"
	SpanNameAction    = "workflow.action"
	SpanNameHTTP      = "http.request"
)

// Common attribute keys
const (
	AttrEventType     = "event.type"
	AttrEventSource   = "event.source"
	AttrRouteName     = "route.name"
	AttrRouteMatched  = "route.matched"
	AttrActionType    = "action.type"
	AttrActionSuccess = "action.success"
	AttrTransformType = "transform.type"
	AttrHTTPMethod    = "http.method"
	AttrHTTPURL       = "http.url"
	AttrHTTPStatus    = "http.status_code"
	AttrErrorType     = "error.type"
	AttrErrorMessage  = "error.message"
)
