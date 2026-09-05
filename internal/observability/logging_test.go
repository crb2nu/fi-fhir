package observability_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/trace"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/observability"
)

// Slice 4.4d: the log-field allowlist is proved the way the metric-label
// allowlist is proved.
//
// `TestUnknownOutcomeIsRefusedRatherThanEmitted` feeds `Outcome("mrn-123456")`
// into the metrics substrate and asserts it never reaches exposition. These
// tests do the same job for log attributes: an unlisted key never reaches the
// stream, and the proof that the set is bounded can enumerate it.

const logSentinel = "MRN4404DLOGFIELD"

func decodeLines(t *testing.T, blob string) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range strings.Split(blob, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("log line is not JSON: %q (%v)", line, err)
		}
		out = append(out, entry)
	}
	return out
}

func TestLoggerEmitsJSONWithTenantOnEveryLine(t *testing.T) {
	var buf bytes.Buffer
	logger := observability.NewLogger(observability.LogConfig{
		Level:    "debug",
		Format:   "json",
		TenantID: "tenant-a",
		Output:   &buf,
	})

	logger.Info("ingress accepted", observability.F(observability.FieldCorrelationID, "corr-1"))
	logger.Warn("delivery retried", observability.F(observability.FieldTraceID, "trace-1"))
	logger.Error("component failed", observability.F(observability.FieldError, "boom"))

	entries := decodeLines(t, buf.String())
	if len(entries) != 3 {
		t.Fatalf("expected 3 lines, got %d: %s", len(entries), buf.String())
	}
	for i, entry := range entries {
		if entry["tenant_id"] != "tenant-a" {
			t.Errorf("line %d carries no tenant_id: %v", i, entry)
		}
		if entry["msg"] == nil {
			t.Errorf("line %d carries no msg: %v", i, entry)
		}
	}
	if entries[0]["correlation_id"] != "corr-1" {
		t.Errorf("ingress line lost its correlation_id: %v", entries[0])
	}
	if entries[1]["trace_id"] != "trace-1" {
		t.Errorf("delivery line lost its trace_id: %v", entries[1])
	}
}

// TestUnlistedLogFieldIsRefusedRatherThanEmitted is the log-side counterpart of
// TestUnknownOutcomeIsRefusedRatherThanEmitted. A caller that reaches around
// observability.F and hands slog an arbitrary key must not be able to put a
// value on the wire.
func TestUnlistedLogFieldIsRefusedRatherThanEmitted(t *testing.T) {
	var buf bytes.Buffer
	logger := observability.NewLogger(observability.LogConfig{
		Format: "json", TenantID: "tenant-a", Output: &buf,
	})

	before := observability.RejectedLogFields()
	logger.Info("submission recorded",
		observability.F(observability.FieldCorrelationID, "corr-1"),
		slog.String("patient_mrn", logSentinel),
		slog.Any("payload", map[string]string{"family": logSentinel}),
	)

	raw := buf.String()
	if strings.Contains(raw, logSentinel) {
		t.Errorf("an unlisted field carried a value onto the wire: %s", raw)
	}
	if strings.Contains(raw, "patient_mrn") || strings.Contains(raw, "payload") {
		t.Errorf("an unlisted field key reached the wire: %s", raw)
	}

	entries := decodeLines(t, raw)
	if len(entries) != 1 {
		t.Fatalf("expected 1 line, got %d", len(entries))
	}
	// Anti-vacuity: the permitted field on the same call still made it, so the
	// handler is filtering rather than discarding the record.
	if entries[0]["correlation_id"] != "corr-1" {
		t.Errorf("the permitted field was dropped too, so this proves nothing: %v", entries[0])
	}
	// The drop is visible rather than silent.
	if got, ok := entries[0]["dropped_fields"].(float64); !ok || int(got) != 2 {
		t.Errorf("expected dropped_fields=2, got %v", entries[0]["dropped_fields"])
	}
	if observability.RejectedLogFields() != before+2 {
		t.Errorf("the process-wide rejection counter did not advance by 2: %d → %d",
			before, observability.RejectedLogFields())
	}
}

func TestUnlistedLogFieldIsRefusedOnWithAttrsToo(t *testing.T) {
	var buf bytes.Buffer
	logger := observability.NewLogger(observability.LogConfig{
		Format: "json", TenantID: "tenant-a", Output: &buf,
	})
	// A component logger built with `With` must not smuggle a key past the
	// handler by binding it once and reusing it.
	scoped := logger.With(
		observability.F(observability.FieldComponent, "delivery"),
		slog.String("patient_mrn", logSentinel),
	)
	scoped.Info("dispatch")

	raw := buf.String()
	if strings.Contains(raw, logSentinel) || strings.Contains(raw, "patient_mrn") {
		t.Errorf("With() smuggled an unlisted field: %s", raw)
	}
	entries := decodeLines(t, raw)
	if entries[0]["component"] != "delivery" {
		t.Errorf("the permitted bound field was dropped too: %v", entries[0])
	}
}

// TestLogGroupsAreRefused: a group renames every key beneath it, which would
// let an unlisted key in under a listed prefix.
func TestLogGroupsAreRefused(t *testing.T) {
	var buf bytes.Buffer
	logger := observability.NewLogger(observability.LogConfig{
		Format: "json", TenantID: "tenant-a", Output: &buf,
	})
	logger.WithGroup("patient").Info("admitted", slog.String("mrn", logSentinel))
	raw := buf.String()
	if strings.Contains(raw, logSentinel) || strings.Contains(raw, "patient") {
		t.Errorf("a group carried an unlisted key onto the wire: %s", raw)
	}
}

func TestLoggerCorrelatesWithAValidSpanContextOnly(t *testing.T) {
	var buf bytes.Buffer
	logger := observability.NewLogger(observability.LogConfig{
		Format: "json", TenantID: "tenant-a", Output: &buf,
	})

	// No span in context: no trace_id, and specifically not a zeroed one. A
	// zeroed trace ID on every line makes every line look correlated and no line
	// joinable.
	logger.InfoContext(context.Background(), "no span")
	entries := decodeLines(t, buf.String())
	if _, present := entries[0]["trace_id"]; present {
		t.Errorf("a line with no span carries a trace_id: %v", entries[0])
	}

	buf.Reset()
	traceID, err := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	if err != nil {
		t.Fatalf("trace id: %v", err)
	}
	spanID, err := trace.SpanIDFromHex("00f067aa0ba902b7")
	if err != nil {
		t.Fatalf("span id: %v", err)
	}
	ctx := trace.ContextWithSpanContext(context.Background(), trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: traceID, SpanID: spanID, TraceFlags: trace.FlagsSampled,
	}))
	logger.InfoContext(ctx, "with span")
	entries = decodeLines(t, buf.String())
	if entries[0]["trace_id"] != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Errorf("span context did not reach the line: %v", entries[0])
	}
	if entries[0]["span_id"] != "00f067aa0ba902b7" {
		t.Errorf("span id did not reach the line: %v", entries[0])
	}
}

func TestLogLevelAndFormatComeFromConfiguration(t *testing.T) {
	var buf bytes.Buffer
	logger := observability.NewLogger(observability.LogConfig{
		Level: "warn", Format: "json", TenantID: "tenant-a", Output: &buf,
	})
	logger.Info("suppressed")
	logger.Warn("emitted")
	entries := decodeLines(t, buf.String())
	if len(entries) != 1 || entries[0]["msg"] != "emitted" {
		t.Errorf("level filtering is not applied: %s", buf.String())
	}

	buf.Reset()
	text := observability.NewLogger(observability.LogConfig{
		Format: "text", TenantID: "tenant-a", Output: &buf,
	})
	text.Info("plain")
	if json.Valid([]byte(strings.TrimSpace(buf.String()))) {
		t.Errorf("text format produced JSON: %s", buf.String())
	}
	if !strings.Contains(buf.String(), "tenant_id=tenant-a") {
		t.Errorf("text format lost the tenant: %s", buf.String())
	}

	// An unrecognised level must not fail startup.
	if got := (observability.LogConfig{Level: "shout"}).LogLevel(); got != slog.LevelInfo {
		t.Errorf("unrecognised level should fall back to info, got %v", got)
	}
}

// TestEveryPermittedLogFieldIsBounded is the anti-vacuity guard on the
// allowlist itself: the set must be enumerable, non-empty, and free of the
// handler-owned key.
func TestEveryPermittedLogFieldIsBounded(t *testing.T) {
	fields := observability.PermittedLogFields()
	if len(fields) < 20 {
		t.Fatalf("the allowlist has %d entries, which is too few to be the real one", len(fields))
	}
	seen := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		if _, duplicate := seen[field]; duplicate {
			t.Errorf("duplicate field %q", field)
		}
		seen[field] = struct{}{}
		if field != strings.ToLower(field) || strings.ContainsAny(field, " .-") {
			t.Errorf("field %q is not a flat snake_case key", field)
		}
		if !observability.KnownLogField(observability.LogField(field)) {
			t.Errorf("PermittedLogFields returned %q but KnownLogField refuses it", field)
		}
	}
	if _, present := seen["dropped_fields"]; present {
		t.Error("dropped_fields is written by the handler and must not be caller-settable")
	}
	if observability.KnownLogField(observability.LogField("patient_mrn")) {
		t.Error("anti-vacuity: KnownLogField accepts an obviously unlisted key")
	}
}

// TestPublishedComplianceSchemaIsEmittable ties the allowlist to the schema
// docs/operations/PRODUCTION-HARDENING.md publishes. That block was aspirational
// before this slice; the doc and the code now have to agree.
func TestPublishedComplianceSchemaIsEmittable(t *testing.T) {
	published := []observability.LogField{
		observability.FieldTraceID,
		observability.FieldSpanID,
		observability.FieldTenantID,
		observability.FieldCorrelationID,
		observability.FieldComponent,
		observability.FieldOutcome,
		observability.FieldDurationMs,
		observability.FieldStatus,
	}
	for _, field := range published {
		if !observability.KnownLogField(field) {
			t.Errorf("PRODUCTION-HARDENING publishes %q but the allowlist refuses it", field)
		}
	}
}

func TestErrfBoundsAnUnboundedError(t *testing.T) {
	if got := observability.Errf(nil); got != "" {
		t.Errorf("nil error: %q", got)
	}
	if got := observability.Errf(errors.New("refused")); got != "refused" {
		t.Errorf("short error changed: %q", got)
	}
	long := fmt.Errorf("write failed: %s", strings.Repeat("A", 4096)+logSentinel)
	got := observability.Errf(long)
	if strings.Contains(got, logSentinel) {
		t.Errorf("a 4KB error still reached the tail: %q", got)
	}
	if !strings.Contains(got, "bytes truncated") {
		t.Errorf("truncation must be visible: %q", got)
	}
}
