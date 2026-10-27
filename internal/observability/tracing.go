package observability

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"net/url"
	"sort"
	"sync/atomic"
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

// Span attributes are a closed set, narrower than the structured-log allowlist.
//
// A log line stays with the process's own aggregator; a span is exported to a
// collector the operator may not run, where OTLP attributes are indexed and
// retained. So a span may carry only a bounded enumeration, a durable identifier
// the product already treats as non-PHI, or a numeric scalar the caller
// computed. Three log fields are refused here by design:
//
//   - source_message_id is sender-controlled text (MSH-10). The processor's
//     Observation already withholds it from the seam; this set keeps a future
//     call site from re-admitting it.
//   - error and reason are free text. Errf bounds an error's length, not its
//     content, and a storage error is free to quote the record it could not
//     write.
//
// correlation_id is admitted on the understanding that the call site hashes the
// caller's header (cmd/fi-fhir, spanCorrelationID); the `tracingleak` negative
// control proves the structured-logging proof notices when it does not. The
// remaining log fields (field, grant, path, address, port, driver, enabled,
// version, signal, worker_id, principal_id, digest) have no span call site, and
// trace_id and span_id are the span's own identity. Adding a key here is the
// whole review surface for "may this reach a collector".
var permittedSpanAttributes = map[LogField]struct{}{
	FieldComponent: {}, FieldOperation: {}, FieldMode: {}, FieldOutcome: {},
	FieldTenantID: {}, FieldCorrelationID: {}, FieldReceiptID: {}, FieldLineageTraceID: {},
	FieldWorkflowRunID: {}, FieldDeliveryAttemptID: {},
	FieldIntegrationID: {}, FieldDefinitionID: {}, FieldRevisionID: {}, FieldDestinationID: {},
	FieldEnvironment: {},
	FieldDurationMs:  {}, FieldCount: {}, FieldBytes: {}, FieldAttempt: {}, FieldStatus: {},
}

// droppedAttributesKey reports how many attributes a span refused. It is written
// by Tracing itself, never by a caller — a caller passing it is refused like any
// other unlisted key — so a non-zero value on an exported span is unambiguous
// evidence that a call site tried to write outside the allowlist. It is the span
// counterpart of droppedFieldsKey, and the structured-logging proof fails on it
// for the same reason it fails on dropped_fields.
const droppedAttributesKey = "dropped_attributes"

// rejectedSpanAttributes counts attributes refused on spans, process-wide, for
// the allowlist proofs; nothing branches on it.
var rejectedSpanAttributes atomic.Uint64

// KnownSpanAttribute reports whether a key may be exported on a span.
func KnownSpanAttribute(field LogField) bool {
	_, ok := permittedSpanAttributes[field]
	return ok
}

// PermittedSpanAttributes returns the span allowlist, sorted, for tests and for
// documentation generation.
func PermittedSpanAttributes() []string {
	out := make([]string, 0, len(permittedSpanAttributes))
	for field := range permittedSpanAttributes {
		out = append(out, string(field))
	}
	sort.Strings(out)
	return out
}

// RejectedSpanAttributes returns the number of attributes refused on spans since
// process start.
func RejectedSpanAttributes() uint64 { return rejectedSpanAttributes.Load() }

// Start accepts only flat scalar attributes from the span allowlist; anything
// else is refused, counted, and reported on the span as dropped_attributes.
// Call sites pass catalog outcomes; exception events and raw errors are never
// recorded. A nil receiver does not create a provider, span, or connection.
func (t *Tracing) Start(ctx context.Context, name string, attrs ...slog.Attr) (context.Context, func(bool, ...slog.Attr)) {
	if t == nil {
		return ctx, func(bool, ...slog.Attr) {}
	}
	attrs = append(attrs, F(FieldTenantID, t.tenantID))
	values, dropped := spanAttributes(attrs)
	if dropped > 0 {
		values = append(values, attribute.Int(droppedAttributesKey, dropped))
	}
	ctx, span := t.tracer.Start(ctx, name, trace.WithAttributes(values...))
	return ctx, func(failed bool, fields ...slog.Attr) {
		finishValues, finishDropped := spanAttributes(fields)
		if dropped+finishDropped > 0 {
			finishValues = append(finishValues, attribute.Int(droppedAttributesKey, dropped+finishDropped))
		}
		span.SetAttributes(finishValues...)
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

// spanAttributes converts the permitted flat scalars and counts what it refused:
// an unlisted key, or a listed key carrying a non-scalar value (a slog.Any of a
// map or struct is how a payload would travel under a listed name).
func spanAttributes(attrs []slog.Attr) ([]attribute.KeyValue, int) {
	values := make([]attribute.KeyValue, 0, len(attrs))
	dropped := 0
	for _, attr := range attrs {
		if !KnownSpanAttribute(LogField(attr.Key)) {
			dropped++
			rejectedSpanAttributes.Add(1)
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
		default:
			dropped++
			rejectedSpanAttributes.Add(1)
		}
	}
	return values, dropped
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
