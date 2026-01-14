package workflow

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

// OTelTracer wraps the OpenTelemetry tracer to implement our Tracer interface.
type OTelTracer struct {
	tracer trace.Tracer
	tp     *sdktrace.TracerProvider
}

// OTelConfig configures the OpenTelemetry tracer.
type OTelConfig struct {
	// ServiceName is the name of the service for tracing
	ServiceName string

	// ServiceVersion is the version of the service
	ServiceVersion string

	// Environment is the deployment environment
	Environment string

	// Exporter is the trace exporter (e.g., OTLP, Jaeger, Zipkin, stdout)
	// If nil, a no-op exporter is used
	Exporter sdktrace.SpanExporter

	// Sampler controls trace sampling (default: AlwaysSample)
	Sampler sdktrace.Sampler
}

// DefaultOTelConfig returns sensible defaults.
func DefaultOTelConfig() OTelConfig {
	return OTelConfig{
		ServiceName:    "fi-fhir",
		ServiceVersion: "1.0.0",
		Environment:    "development",
		Sampler:        sdktrace.AlwaysSample(),
	}
}

// NewOTelTracer creates a new OpenTelemetry tracer.
func NewOTelTracer(config *OTelConfig) (*OTelTracer, error) {
	cfg := DefaultOTelConfig()
	if config != nil {
		if config.ServiceName != "" {
			cfg.ServiceName = config.ServiceName
		}
		if config.ServiceVersion != "" {
			cfg.ServiceVersion = config.ServiceVersion
		}
		if config.Environment != "" {
			cfg.Environment = config.Environment
		}
		if config.Exporter != nil {
			cfg.Exporter = config.Exporter
		}
		if config.Sampler != nil {
			cfg.Sampler = config.Sampler
		}
	}

	// Create resource with service info
	// Note: We don't merge with resource.Default() to avoid schema URL conflicts
	// between the SDK version and semconv version
	res := resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName(cfg.ServiceName),
		semconv.ServiceVersion(cfg.ServiceVersion),
		attribute.String("deployment.environment", cfg.Environment),
	)

	// Build tracer provider options
	opts := []sdktrace.TracerProviderOption{
		sdktrace.WithResource(res),
		sdktrace.WithSampler(cfg.Sampler),
	}

	// Add exporter if provided
	if cfg.Exporter != nil {
		opts = append(opts, sdktrace.WithBatcher(cfg.Exporter))
	}

	// Create tracer provider
	tp := sdktrace.NewTracerProvider(opts...)

	// Set as global tracer provider
	otel.SetTracerProvider(tp)

	// Create tracer
	tracer := tp.Tracer(
		"gitlab.flexinfer.ai/libs/fi-fhir/internal/workflow",
		trace.WithInstrumentationVersion(cfg.ServiceVersion),
	)

	return &OTelTracer{
		tracer: tracer,
		tp:     tp,
	}, nil
}

// StartSpan starts a new OpenTelemetry span.
func (t *OTelTracer) StartSpan(ctx context.Context, name string, opts ...SpanOption) (context.Context, Span) {
	// Apply options
	cfg := &spanConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	// Convert span kind
	var otelKind trace.SpanKind
	switch cfg.kind {
	case SpanKindServer:
		otelKind = trace.SpanKindServer
	case SpanKindClient:
		otelKind = trace.SpanKindClient
	case SpanKindProducer:
		otelKind = trace.SpanKindProducer
	case SpanKindConsumer:
		otelKind = trace.SpanKindConsumer
	default:
		otelKind = trace.SpanKindInternal
	}

	// Convert initial attributes
	otelAttrs := make([]attribute.KeyValue, 0, len(cfg.attributes))
	for _, attr := range cfg.attributes {
		otelAttrs = append(otelAttrs, toOTelAttribute(attr.Key, attr.Value))
	}

	// Start span
	ctx, span := t.tracer.Start(ctx, name,
		trace.WithSpanKind(otelKind),
		trace.WithAttributes(otelAttrs...),
	)

	return ctx, &otelSpan{span: span}
}

// Shutdown shuts down the tracer provider.
func (t *OTelTracer) Shutdown(ctx context.Context) error {
	return t.tp.Shutdown(ctx)
}

// ForceFlush flushes any pending spans.
func (t *OTelTracer) ForceFlush(ctx context.Context) error {
	return t.tp.ForceFlush(ctx)
}

// TracerProvider returns the underlying OpenTelemetry tracer provider.
func (t *OTelTracer) TracerProvider() *sdktrace.TracerProvider {
	return t.tp
}

// otelSpan wraps an OpenTelemetry span.
type otelSpan struct {
	span trace.Span
}

func (s *otelSpan) End() {
	s.span.End()
}

func (s *otelSpan) SetAttribute(key string, value interface{}) {
	s.span.SetAttributes(toOTelAttribute(key, value))
}

func (s *otelSpan) SetStatus(code SpanStatus, message string) {
	switch code {
	case SpanStatusOK:
		s.span.SetStatus(codes.Ok, message)
	case SpanStatusError:
		s.span.SetStatus(codes.Error, message)
	default:
		s.span.SetStatus(codes.Unset, message)
	}
}

func (s *otelSpan) RecordError(err error) {
	if err != nil {
		s.span.RecordError(err)
	}
}

func (s *otelSpan) AddEvent(name string, attrs ...SpanAttribute) {
	otelAttrs := make([]attribute.KeyValue, 0, len(attrs))
	for _, attr := range attrs {
		otelAttrs = append(otelAttrs, toOTelAttribute(attr.Key, attr.Value))
	}
	s.span.AddEvent(name, trace.WithAttributes(otelAttrs...))
}

// toOTelAttribute converts our attribute to OpenTelemetry attribute.
func toOTelAttribute(key string, value interface{}) attribute.KeyValue {
	switch v := value.(type) {
	case string:
		return attribute.String(key, v)
	case int:
		return attribute.Int(key, v)
	case int64:
		return attribute.Int64(key, v)
	case float64:
		return attribute.Float64(key, v)
	case bool:
		return attribute.Bool(key, v)
	case []string:
		return attribute.StringSlice(key, v)
	case []int:
		return attribute.IntSlice(key, v)
	case []int64:
		return attribute.Int64Slice(key, v)
	case []float64:
		return attribute.Float64Slice(key, v)
	case []bool:
		return attribute.BoolSlice(key, v)
	default:
		return attribute.String(key, fmt.Sprintf("%v", v))
	}
}

// SpanFromContext extracts the span from context (useful for adding attributes later).
func SpanFromContext(ctx context.Context) Span {
	s := trace.SpanFromContext(ctx)
	if s == nil || !s.IsRecording() {
		return &noOpSpan{}
	}
	return &otelSpan{span: s}
}

// ContextWithSpan returns a new context with the given span.
// This is useful when you need to propagate spans manually.
func ContextWithSpan(ctx context.Context, span Span) context.Context {
	if otelS, ok := span.(*otelSpan); ok {
		return trace.ContextWithSpan(ctx, otelS.span)
	}
	return ctx
}
