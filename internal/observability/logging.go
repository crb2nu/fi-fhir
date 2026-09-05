package observability

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sort"
	"strings"
	"sync/atomic"

	"go.opentelemetry.io/otel/trace"
)

// Slice 4.4d: the process's structured logger.
//
// It lives here, beside the metrics substrate, for the same reason the metrics
// substrate does: `internal/integration/*` must hold no observability
// dependency (see the rule recorded at the bottom of metrics.go), so both the
// counters and the log fields are defined on this side of the seam and reach
// the components through the `Observe`/`SetObserver` callbacks that
// `cmd/fi-fhir/serve_observability.go` adapts.
//
// The design mirrors the metric-label posture deliberately. `Metrics.inc`
// coerces an unrecognised Outcome to `error` rather than emitting it, because a
// label value that escapes a bounded set is how PHI reaches a metrics backend.
// A log field is the same hazard with a larger surface, so `LogField` is a
// closed set and `boundedHandler` drops any attribute outside it rather than
// writing it. The count of dropped attributes is reported on the line, so a
// silent drop is still visible to a reader — the same reasoning as
// `GatheredLabelValues`.
//
// What is deliberately NOT here: a `WithField(string, any)` escape hatch. The
// moment one exists, the allowlist is advisory.

// LogField is a permitted structured-log attribute key.
//
// Adding a value here is the whole review surface for "may this appear in a log
// line". Every constant below is either a bounded enumeration (component,
// outcome), a durable identifier the product already treats as non-PHI
// (tenant_id, correlation_id, trace_id), or a scalar the caller computes
// (counts, durations, sizes, error text). No value derived from a message
// payload belongs in this list.
type LogField string

const (
	// Stage and identity.
	FieldComponent LogField = "component"
	FieldOutcome   LogField = "outcome"
	FieldTenantID  LogField = "tenant_id"

	// Correlation. There is no single identifier: `pkg/integration/contracts.go`
	// declares an eight-field lineage bundle on purpose, and the durable schema
	// follows it — integration_receipts, integration_canonical_events, and
	// integration_message_lineage carry correlation_id NOT NULL, while
	// integration_delivery_attempts carries trace_id NOT NULL and joins back by
	// foreign key. Each stage emits the identifier it actually holds.
	FieldCorrelationID     LogField = "correlation_id"
	FieldTraceID           LogField = "trace_id"
	FieldSpanID            LogField = "span_id"
	FieldSourceMessageID   LogField = "source_message_id"
	FieldReceiptID         LogField = "receipt_id"
	FieldWorkflowRunID     LogField = "workflow_run_id"
	FieldDeliveryAttemptID LogField = "delivery_attempt_id"

	// Deployment and routing identifiers. All are operator-chosen artifact
	// names, never message content.
	FieldIntegrationID LogField = "integration_id"
	FieldDefinitionID  LogField = "definition_id"
	FieldRevisionID    LogField = "revision_id"
	FieldDigest        LogField = "digest"
	FieldDestinationID LogField = "destination_id"
	FieldEnvironment   LogField = "environment"
	FieldPrincipalID   LogField = "principal_id"
	FieldField         LogField = "field"
	FieldGrant         LogField = "grant"
	FieldOperation     LogField = "operation"

	// Process facts.
	FieldAddress  LogField = "address"
	FieldPort     LogField = "port"
	FieldPath     LogField = "path"
	FieldDriver   LogField = "driver"
	FieldEnabled  LogField = "enabled"
	FieldMode     LogField = "mode"
	FieldVersion  LogField = "version"
	FieldReason   LogField = "reason"
	FieldSignal   LogField = "signal"
	FieldWorkerID LogField = "worker_id"

	// Scalars the caller computes. `error` carries a bounded error string; the
	// caller is responsible for not building one out of a payload, which is what
	// `boundedErrorText` exists for on the workflow side.
	FieldError      LogField = "error"
	FieldCount      LogField = "count"
	FieldDurationMs LogField = "duration_ms"
	FieldBytes      LogField = "bytes"
	FieldAttempt    LogField = "attempt"
	FieldStatus     LogField = "status"
)

// permittedLogFields is the closed set. `droppedFieldsKey` is written by the
// handler itself and is therefore not a LogField anyone may pass.
var permittedLogFields = map[LogField]struct{}{
	FieldComponent: {}, FieldOutcome: {}, FieldTenantID: {},
	FieldCorrelationID: {}, FieldTraceID: {}, FieldSpanID: {},
	FieldSourceMessageID: {}, FieldReceiptID: {}, FieldWorkflowRunID: {},
	FieldDeliveryAttemptID: {},
	FieldIntegrationID:     {}, FieldDefinitionID: {}, FieldRevisionID: {},
	FieldDigest: {}, FieldDestinationID: {}, FieldEnvironment: {},
	FieldPrincipalID: {}, FieldField: {}, FieldGrant: {}, FieldOperation: {},
	FieldAddress: {}, FieldPort: {}, FieldPath: {}, FieldDriver: {},
	FieldEnabled: {}, FieldMode: {}, FieldVersion: {}, FieldReason: {},
	FieldSignal: {}, FieldWorkerID: {},
	FieldError: {}, FieldCount: {}, FieldDurationMs: {}, FieldBytes: {},
	FieldAttempt: {}, FieldStatus: {},
}

// droppedFieldsKey reports how many attributes the handler refused on a line.
// It is written by the handler, never by a caller, so a non-zero value is
// unambiguous evidence of an allowlist violation rather than of a caller's
// opinion about one.
const droppedFieldsKey = "dropped_fields"

// KnownLogField reports whether a key may be emitted.
func KnownLogField(field LogField) bool {
	_, ok := permittedLogFields[field]
	return ok
}

// PermittedLogFields returns the allowlist, sorted, for tests and for
// documentation generation. It is the log-side counterpart of
// GatheredLabelValues: the proof that the set is bounded has to be able to
// enumerate it.
func PermittedLogFields() []string {
	out := make([]string, 0, len(permittedLogFields))
	for field := range permittedLogFields {
		out = append(out, string(field))
	}
	sort.Strings(out)
	return out
}

// F builds a permitted attribute. Callers pass a LogField constant, so an
// unlisted key is a compile-time typo rather than a runtime leak; the handler
// is the backstop for the dynamic cases (`slog.Any`, a wrapped handler, a
// caller reaching for slog directly).
func F(field LogField, value any) slog.Attr {
	return slog.Any(string(field), value)
}

// LogConfig configures the process logger from settings pkg/config already
// parses and validates (observability.log_level, observability.log_format) and
// which, before this slice, nothing read.
type LogConfig struct {
	// Level is one of debug, info, warn, error. An unrecognised value falls
	// back to info rather than failing startup: a typo in a deployment variable
	// must not be the reason an ingress listener does not come up.
	Level string
	// Format is json or text. Anything else is json — the deployment default,
	// and the only format an aggregator can parse.
	Format string
	// TenantID is stamped on every line. `runServe` already requires a
	// canonical deployment tenant before it configures the integration runtime,
	// so there is always one to stamp.
	TenantID string
	// Output defaults to os.Stderr when nil.
	Output io.Writer
}

// LogLevel resolves the configured level, defaulting to info.
func (c LogConfig) LogLevel() slog.Level {
	switch strings.ToLower(strings.TrimSpace(c.Level)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// JSONFormat reports whether the configured format is the JSON one.
func (c LogConfig) JSONFormat() bool {
	return !strings.EqualFold(strings.TrimSpace(c.Format), "text")
}

// NewLogger builds the process logger.
//
// Every line it writes carries `tenant_id`; the stage-appropriate correlation
// identifier is the caller's to attach, because only the caller knows which one
// its stage durably holds.
func NewLogger(cfg LogConfig) *slog.Logger {
	output := cfg.Output
	if output == nil {
		output = os.Stderr
	}
	opts := &slog.HandlerOptions{Level: cfg.LogLevel()}

	var inner slog.Handler
	if cfg.JSONFormat() {
		inner = slog.NewJSONHandler(output, opts)
	} else {
		inner = slog.NewTextHandler(output, opts)
	}

	handler := slog.Handler(&boundedHandler{inner: inner})
	logger := slog.New(handler)
	if cfg.TenantID != "" {
		logger = logger.With(F(FieldTenantID, cfg.TenantID))
	}
	return logger
}

// NewDiscardLogger returns a logger that writes nothing. It exists so a code
// path can hold a non-nil *slog.Logger without a nil check at every call site,
// which is how the orphaned workflow logger's NoOpLogger default ended up
// masking the fact that nothing ever replaced it.
func NewDiscardLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError + 1}))
}

// rejectedLogFields counts attributes the handler refused, process-wide. It is
// read by the allowlist proof; production has no reason to consult it, and
// nothing branches on it.
var rejectedLogFields atomic.Uint64

// RejectedLogFields returns the number of attributes dropped for being outside
// the allowlist since process start.
func RejectedLogFields() uint64 { return rejectedLogFields.Load() }

// boundedHandler enforces the field allowlist and attaches OpenTelemetry
// correlation.
//
// The correlation logic is ported from the orphaned
// `internal/workflow/logging.go:164-175`, which had it right and had no
// callers: read the span context off the ctx, and emit trace_id/span_id only
// when it is valid. Emitting a zeroed trace ID on every line would make every
// log line look correlated and no log line be joinable.
type boundedHandler struct {
	inner slog.Handler
}

func (h *boundedHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h *boundedHandler) Handle(ctx context.Context, record slog.Record) error {
	filtered := slog.NewRecord(record.Time, record.Level, record.Message, record.PC)

	dropped := 0
	record.Attrs(func(attr slog.Attr) bool {
		if KnownLogField(LogField(attr.Key)) {
			filtered.AddAttrs(attr)
			return true
		}
		dropped++
		rejectedLogFields.Add(1)
		return true
	})

	if ctx != nil {
		spanCtx := trace.SpanContextFromContext(ctx)
		if spanCtx.IsValid() {
			filtered.AddAttrs(
				F(FieldTraceID, spanCtx.TraceID().String()),
				F(FieldSpanID, spanCtx.SpanID().String()),
			)
		}
	}

	if dropped > 0 {
		filtered.AddAttrs(slog.Int(droppedFieldsKey, dropped))
	}

	return h.inner.Handle(ctx, filtered)
}

func (h *boundedHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	permitted := make([]slog.Attr, 0, len(attrs))
	for _, attr := range attrs {
		if KnownLogField(LogField(attr.Key)) {
			permitted = append(permitted, attr)
			continue
		}
		rejectedLogFields.Add(1)
	}
	return &boundedHandler{inner: h.inner.WithAttrs(permitted)}
}

// WithGroup is refused. A group renames every key beneath it, which would let
// an unlisted key in under a listed prefix and would break the flat schema
// `docs/operations/PRODUCTION-HARDENING.md` publishes.
func (h *boundedHandler) WithGroup(string) slog.Handler {
	return h
}

// Errf renders an error for the `error` field with an explicit ceiling. A
// storage or transport error is free to quote the record it could not write, so
// an unbounded error string is an unbounded payload.
func Errf(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	const maxLoggedErrorBytes = 256
	if len(msg) <= maxLoggedErrorBytes {
		return msg
	}
	cut := maxLoggedErrorBytes
	for cut > 0 && !isRuneStart(msg[cut]) {
		cut--
	}
	return fmt.Sprintf("%s… (%d bytes truncated)", msg[:cut], len(msg)-cut)
}

func isRuneStart(b byte) bool { return b&0xC0 != 0x80 }
