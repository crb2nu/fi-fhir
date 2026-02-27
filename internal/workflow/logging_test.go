package workflow

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/trace"
)

func TestStructuredLoggerTextFormat(t *testing.T) {
	var buf bytes.Buffer

	logger := NewStructuredLogger(&LoggerConfig{
		Level:          LevelDebug,
		Format:         FormatText,
		Output:         &buf,
		IncludeTraceID: true,
		IncludeSpanID:  true,
	})

	logger.Info(context.Background(), "test message", F("key", "value"))

	output := buf.String()
	if !strings.Contains(output, "INFO") {
		t.Errorf("Expected INFO level, got: %s", output)
	}
	if !strings.Contains(output, "test message") {
		t.Errorf("Expected message in output, got: %s", output)
	}
	if !strings.Contains(output, "key=value") {
		t.Errorf("Expected field in output, got: %s", output)
	}
}

func TestStructuredLoggerJSONFormat(t *testing.T) {
	var buf bytes.Buffer

	logger := NewStructuredLogger(&LoggerConfig{
		Level:  LevelDebug,
		Format: FormatJSON,
		Output: &buf,
	})

	logger.Info(context.Background(), "test message", F("key", "value"))

	var entry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("Failed to parse JSON log entry: %v", err)
	}

	if entry["level"] != "INFO" {
		t.Errorf("Expected INFO level, got: %v", entry["level"])
	}
	if entry["msg"] != "test message" {
		t.Errorf("Expected message, got: %v", entry["msg"])
	}
	if entry["key"] != "value" {
		t.Errorf("Expected key=value, got: %v", entry["key"])
	}
}

func TestStructuredLoggerLevelFiltering(t *testing.T) {
	var buf bytes.Buffer

	logger := NewStructuredLogger(&LoggerConfig{
		Level:  LevelWarn,
		Format: FormatText,
		Output: &buf,
	})

	// Debug should be filtered
	logger.Debug(context.Background(), "debug message")
	if buf.Len() > 0 {
		t.Error("Debug should be filtered at Warn level")
	}

	// Info should be filtered
	logger.Info(context.Background(), "info message")
	if buf.Len() > 0 {
		t.Error("Info should be filtered at Warn level")
	}

	// Warn should pass
	logger.Warn(context.Background(), "warn message")
	if !strings.Contains(buf.String(), "WARN") {
		t.Error("Warn should not be filtered")
	}

	buf.Reset()

	// Error should pass
	logger.Error(context.Background(), "error message")
	if !strings.Contains(buf.String(), "ERROR") {
		t.Error("Error should not be filtered")
	}
}

func TestStructuredLoggerWithFields(t *testing.T) {
	var buf bytes.Buffer

	logger := NewStructuredLogger(&LoggerConfig{
		Level:  LevelDebug,
		Format: FormatJSON,
		Output: &buf,
	})

	// Create logger with base fields
	childLogger := logger.WithFields(F("service", "test-service"))

	childLogger.Info(context.Background(), "message", F("extra", "value"))

	var entry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	if entry["service"] != "test-service" {
		t.Errorf("Expected base field service=test-service, got: %v", entry["service"])
	}
	if entry["extra"] != "value" {
		t.Errorf("Expected extra=value, got: %v", entry["extra"])
	}
}

func TestStructuredLoggerTraceCorrelation(t *testing.T) {
	var buf bytes.Buffer

	logger := NewStructuredLogger(&LoggerConfig{
		Level:          LevelDebug,
		Format:         FormatJSON,
		Output:         &buf,
		IncludeTraceID: true,
		IncludeSpanID:  true,
	})

	// Create a real tracer using our OTelTracer which sets up a proper provider
	otelTracer, err := NewOTelTracer(&OTelConfig{ServiceName: "test"})
	if err != nil {
		t.Fatalf("NewOTelTracer() error = %v", err)
	}
	defer otelTracer.Shutdown(context.Background())

	// Start a span using our tracer to get valid trace context
	ctx, span := otelTracer.StartSpan(context.Background(), "test-span")
	defer span.End()

	logger.Info(ctx, "message with trace")

	var entry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	// Check that trace_id and span_id are present
	traceID, hasTraceID := entry["trace_id"]
	spanID, hasSpanID := entry["span_id"]

	if !hasTraceID {
		t.Error("Expected trace_id in log entry")
	}
	if !hasSpanID {
		t.Error("Expected span_id in log entry")
	}

	// Verify they're not empty
	if traceID == "" {
		t.Error("trace_id should not be empty")
	}
	if spanID == "" {
		t.Error("span_id should not be empty")
	}
}

func TestStructuredLoggerNoTraceContext(t *testing.T) {
	var buf bytes.Buffer

	logger := NewStructuredLogger(&LoggerConfig{
		Level:          LevelDebug,
		Format:         FormatJSON,
		Output:         &buf,
		IncludeTraceID: true,
		IncludeSpanID:  true,
	})

	// Log without trace context
	logger.Info(context.Background(), "message without trace")

	var entry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	// trace_id and span_id should not be present
	if _, hasTraceID := entry["trace_id"]; hasTraceID {
		t.Error("trace_id should not be present without trace context")
	}
	if _, hasSpanID := entry["span_id"]; hasSpanID {
		t.Error("span_id should not be present without trace context")
	}
}

func TestNoOpLogger(t *testing.T) {
	logger := &NoOpLogger{}

	// These should not panic
	logger.Debug(context.Background(), "debug")
	logger.Info(context.Background(), "info")
	logger.Warn(context.Background(), "warn")
	logger.Error(context.Background(), "error")

	child := logger.WithFields(F("key", "value"))
	if _, ok := child.(*NoOpLogger); !ok {
		t.Error("WithFields should return NoOpLogger")
	}
}

func TestGlobalLogger(t *testing.T) {
	// Default should be NoOpLogger
	logger := GetGlobalLogger()
	if _, ok := logger.(*NoOpLogger); !ok {
		t.Errorf("Default global logger should be NoOpLogger, got %T", logger)
	}

	// Set custom logger
	var buf bytes.Buffer
	customLogger := NewStructuredLogger(&LoggerConfig{
		Level:  LevelDebug,
		Format: FormatText,
		Output: &buf,
	})
	SetGlobalLogger(customLogger)

	if GetGlobalLogger() != customLogger {
		t.Error("Global logger should be the custom logger")
	}

	// Test logging through global
	GetGlobalLogger().Info(context.Background(), "global test")
	if !strings.Contains(buf.String(), "global test") {
		t.Error("Global logger should output message")
	}

	// Setting nil should reset to NoOpLogger
	SetGlobalLogger(nil)
	if _, ok := GetGlobalLogger().(*NoOpLogger); !ok {
		t.Error("Setting nil should reset to NoOpLogger")
	}
}

func TestTraceIDFromContext(t *testing.T) {
	// No trace context
	traceID := TraceIDFromContext(context.Background())
	if traceID != "" {
		t.Error("TraceIDFromContext should return empty string without trace context")
	}

	// Nil context - intentionally testing nil handling
	traceID = TraceIDFromContext(nil) //nolint:staticcheck // SA1012: intentionally testing nil context behavior
	if traceID != "" {
		t.Error("TraceIDFromContext should return empty string for nil context")
	}

	// With trace context - use our OTelTracer for valid span context
	otelTracer, err := NewOTelTracer(&OTelConfig{ServiceName: "test"})
	if err != nil {
		t.Fatalf("NewOTelTracer() error = %v", err)
	}
	defer otelTracer.Shutdown(context.Background())

	ctx, span := otelTracer.StartSpan(context.Background(), "test-span")
	defer span.End()

	traceID = TraceIDFromContext(ctx)
	if traceID == "" {
		t.Error("TraceIDFromContext should return trace ID when available")
	}
}

func TestSpanIDFromContext(t *testing.T) {
	// No trace context
	spanID := SpanIDFromContext(context.Background())
	if spanID != "" {
		t.Error("SpanIDFromContext should return empty string without trace context")
	}

	// With trace context - use our OTelTracer for valid span context
	otelTracer, err := NewOTelTracer(&OTelConfig{ServiceName: "test"})
	if err != nil {
		t.Fatalf("NewOTelTracer() error = %v", err)
	}
	defer otelTracer.Shutdown(context.Background())

	ctx, span := otelTracer.StartSpan(context.Background(), "test-span")
	defer span.End()

	spanID = SpanIDFromContext(ctx)
	if spanID == "" {
		t.Error("SpanIDFromContext should return span ID when available")
	}
}

func TestTraceContextFields(t *testing.T) {
	// No trace context
	fields := TraceContextFields(context.Background())
	if len(fields) != 0 {
		t.Error("TraceContextFields should return empty slice without trace context")
	}

	// Nil context - intentionally testing nil handling
	fields = TraceContextFields(nil) //nolint:staticcheck // SA1012: intentionally testing nil context behavior
	if fields != nil {
		t.Error("TraceContextFields should return nil for nil context")
	}

	// With trace context - use our OTelTracer for valid span context
	otelTracer, err := NewOTelTracer(&OTelConfig{ServiceName: "test"})
	if err != nil {
		t.Fatalf("NewOTelTracer() error = %v", err)
	}
	defer otelTracer.Shutdown(context.Background())

	ctx, span := otelTracer.StartSpan(context.Background(), "test-span")
	defer span.End()

	fields = TraceContextFields(ctx)
	if len(fields) != 2 {
		t.Errorf("Expected 2 fields, got %d", len(fields))
	}

	hasTraceID := false
	hasSpanID := false
	for _, f := range fields {
		if f.Key == "trace_id" {
			hasTraceID = true
		}
		if f.Key == "span_id" {
			hasSpanID = true
		}
	}

	if !hasTraceID {
		t.Error("Expected trace_id field")
	}
	if !hasSpanID {
		t.Error("Expected span_id field")
	}
}

func TestEngineSetLogger(t *testing.T) {
	wf := &Workflow{Name: "test", Version: "1.0"}
	engine, err := NewEngine(wf)
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}

	// Default should be NoOpLogger
	if _, ok := engine.GetLogger().(*NoOpLogger); !ok {
		t.Errorf("Default logger should be NoOpLogger, got %T", engine.GetLogger())
	}

	// Set custom logger
	var buf bytes.Buffer
	customLogger := NewStructuredLogger(&LoggerConfig{
		Level:  LevelDebug,
		Output: &buf,
	})
	engine.SetLogger(customLogger)

	if engine.GetLogger() != customLogger {
		t.Error("Logger should be the custom logger")
	}

	// Set nil should reset to NoOpLogger
	engine.SetLogger(nil)
	if _, ok := engine.GetLogger().(*NoOpLogger); !ok {
		t.Errorf("Setting nil should reset to NoOpLogger, got %T", engine.GetLogger())
	}
}

func TestEngineLoggingDuringProcess(t *testing.T) {
	var buf bytes.Buffer

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
					{Type: "log", Config: map[string]string{}},
				},
			},
		},
	}

	engine, err := NewEngine(wf)
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}

	// Set up tracer and logger
	tracer, err := NewOTelTracer(&OTelConfig{ServiceName: "test"})
	if err != nil {
		t.Fatalf("NewOTelTracer() error = %v", err)
	}
	defer tracer.Shutdown(context.Background())

	logger := NewStructuredLogger(&LoggerConfig{
		Level:          LevelDebug,
		Format:         FormatJSON,
		Output:         &buf,
		IncludeTraceID: true,
		IncludeSpanID:  true,
	})

	engine.SetTracer(tracer)
	engine.SetLogger(logger)

	// Process event
	event := map[string]interface{}{
		"type":   "test_event",
		"source": "test",
	}

	result := engine.Process(event)
	if result.HasErrors() {
		t.Errorf("Process() had errors: %v", result.AllErrors())
	}

	// Check that logs were written
	output := buf.String()
	if output == "" {
		t.Error("Expected log output")
	}

	// Parse log entries
	lines := strings.Split(strings.TrimSpace(output), "\n")
	foundProcessingLog := false
	foundTraceID := false

	for _, line := range lines {
		if line == "" {
			continue
		}
		var entry map[string]interface{}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}

		if msg, ok := entry["msg"].(string); ok {
			if strings.Contains(msg, "event processed") || strings.Contains(msg, "processing event") {
				foundProcessingLog = true
			}
		}

		if _, ok := entry["trace_id"]; ok {
			foundTraceID = true
		}
	}

	if !foundProcessingLog {
		t.Error("Expected processing log entry")
	}
	if !foundTraceID {
		t.Error("Expected trace_id in log entries")
	}
}

func TestLogLevelString(t *testing.T) {
	tests := []struct {
		level    LogLevel
		expected string
	}{
		{LevelDebug, "DEBUG"},
		{LevelInfo, "INFO"},
		{LevelWarn, "WARN"},
		{LevelError, "ERROR"},
		{LogLevel(99), "UNKNOWN"},
	}

	for _, tt := range tests {
		if got := tt.level.String(); got != tt.expected {
			t.Errorf("LogLevel(%d).String() = %s, want %s", tt.level, got, tt.expected)
		}
	}
}

func TestFieldHelper(t *testing.T) {
	field := F("test_key", "test_value")

	if field.Key != "test_key" {
		t.Errorf("Field.Key = %s, want test_key", field.Key)
	}
	if field.Value != "test_value" {
		t.Errorf("Field.Value = %v, want test_value", field.Value)
	}
}

func TestStructuredLoggerServiceName(t *testing.T) {
	var buf bytes.Buffer

	logger := NewStructuredLogger(&LoggerConfig{
		Level:       LevelDebug,
		Format:      FormatJSON,
		Output:      &buf,
		ServiceName: "my-service",
	})

	logger.Info(context.Background(), "test")

	var entry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	if entry["service"] != "my-service" {
		t.Errorf("Expected service=my-service, got: %v", entry["service"])
	}
}

// Ensure trace package is used (avoids unused import error)
var _ trace.Tracer
