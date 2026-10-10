package observability

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"net/url"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// TraceConfig is the serve process's opt-in OTLP/HTTP configuration.
type TraceConfig struct {
	Enabled     bool
	Endpoint    string
	SampleRatio float64
	Version     string
	TenantID    string
	Logger      *slog.Logger
}

// Tracing owns a private provider. Installing it globally would also export
// legacy workflow instrumentation whose attributes have no PHI allowlist.
type Tracing struct {
	provider *sdktrace.TracerProvider
	tracer   trace.Tracer
	tenantID string
}

func NewTracing(ctx context.Context, cfg TraceConfig) (*Tracing, error) {
	if !cfg.Enabled {
		return nil, nil
	}
	if math.IsNaN(cfg.SampleRatio) || math.IsInf(cfg.SampleRatio, 0) || cfg.SampleRatio < 0 || cfg.SampleRatio > 1 {
		return nil, errors.New("tracing sampler must be between 0 and 1")
	}
	u, err := url.Parse(cfg.Endpoint)
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("tracing endpoint must be an HTTP(S) URL without credentials, query, or fragment")
	}
	if u.Path == "" || u.Path == "/" {
		u.Path = "/v1/traces"
	}
	exporter, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(u.String()), otlptracehttp.WithTimeout(3*time.Second))
	if err != nil {
		return nil, errors.New("initialize OTLP HTTP exporter")
	}
	logger := cfg.Logger
	if logger == nil {
		logger = NewDiscardLogger()
	}
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithResource(resource.NewSchemaless(attribute.String("service.name", "fi-fhir"), attribute.String("service.version", cfg.Version))),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(cfg.SampleRatio))),
		sdktrace.WithBatcher(&reportingExporter{SpanExporter: exporter, logger: logger}, sdktrace.WithExportTimeout(4*time.Second)),
	)
	return &Tracing{provider: provider, tracer: provider.Tracer("fi-fhir/serve"), tenantID: cfg.TenantID}, nil
}

// Start accepts only flat scalar attributes from the structured-log allowlist.
// Call sites pass catalog outcomes; exception events and raw errors are never
// recorded. A nil receiver does not create a provider, span, or connection.
func (t *Tracing) Start(ctx context.Context, name string, attrs ...slog.Attr) (context.Context, func(bool, ...slog.Attr)) {
	if t == nil {
		return ctx, func(bool, ...slog.Attr) {}
	}
	attrs = append(attrs, F(FieldTenantID, t.tenantID))
	ctx, span := t.tracer.Start(ctx, name, trace.WithAttributes(traceAttributes(attrs)...))
	return ctx, func(failed bool, fields ...slog.Attr) {
		span.SetAttributes(traceAttributes(fields)...)
		if failed {
			span.SetStatus(codes.Error, "operation failed")
		} else {
			span.SetStatus(codes.Ok, "")
		}
		span.End()
	}
}

func (t *Tracing) Shutdown(ctx context.Context) error {
	if t == nil {
		return nil
	}
	if err := t.provider.Shutdown(ctx); err != nil {
		return errors.New("trace exporter shutdown failed")
	}
	return nil
}

func traceAttributes(attrs []slog.Attr) []attribute.KeyValue {
	values := make([]attribute.KeyValue, 0, len(attrs))
	for _, attr := range attrs {
		if !KnownLogField(LogField(attr.Key)) {
			continue
		}
		switch attr.Value.Kind() {
		case slog.KindString:
			values = append(values, attribute.String(attr.Key, attr.Value.String()))
		case slog.KindInt64:
			values = append(values, attribute.Int64(attr.Key, attr.Value.Int64()))
		case slog.KindBool:
			values = append(values, attribute.Bool(attr.Key, attr.Value.Bool()))
		case slog.KindFloat64:
			values = append(values, attribute.Float64(attr.Key, attr.Value.Float64()))
		}
	}
	return values
}

// The exporter owns retry. Once retries are exhausted report the lost batch
// through our logger, without letting collector bodies/URLs reach the SDK's
// global stderr error handler. Telemetry failures never fail clinical work.
type reportingExporter struct {
	sdktrace.SpanExporter
	logger *slog.Logger
}

func (e *reportingExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	if err := e.SpanExporter.ExportSpans(ctx, spans); err != nil {
		e.logger.Warn("trace export failed", F(FieldComponent, "tracing"), F(FieldOutcome, "error"), F(FieldCount, len(spans)))
	}
	return nil
}
