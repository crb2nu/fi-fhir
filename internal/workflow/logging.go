package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"go.opentelemetry.io/otel/trace"
)

// Logger defines the interface for structured logging with trace correlation.
// Implementations should extract trace context from the provided context.Context.
type Logger interface {
	// Debug logs a debug-level message with optional fields
	Debug(ctx context.Context, msg string, fields ...Field)

	// Info logs an info-level message with optional fields
	Info(ctx context.Context, msg string, fields ...Field)

	// Warn logs a warning-level message with optional fields
	Warn(ctx context.Context, msg string, fields ...Field)

	// Error logs an error-level message with optional fields
	Error(ctx context.Context, msg string, fields ...Field)

	// WithFields returns a new logger with the given fields attached to all messages
	WithFields(fields ...Field) Logger
}

// Field represents a key-value pair for structured logging.
type Field struct {
	Key   string
	Value interface{}
}

// F is a convenience function to create a Field.
func F(key string, value interface{}) Field {
	return Field{Key: key, Value: value}
}

// LogLevel represents the severity level of a log message.
type LogLevel int

const (
	LevelDebug LogLevel = iota
	LevelInfo
	LevelWarn
	LevelError
)

func (l LogLevel) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// LogFormat defines the output format for log messages.
type LogFormat int

const (
	// FormatText outputs human-readable log lines
	FormatText LogFormat = iota
	// FormatJSON outputs JSON-structured log lines (for log aggregators)
	FormatJSON
)

// LoggerConfig configures the structured logger.
type LoggerConfig struct {
	// Level is the minimum level to log (default: LevelInfo)
	Level LogLevel

	// Format is the output format (default: FormatText)
	Format LogFormat

	// Output is the writer for log output (default: os.Stdout)
	Output io.Writer

	// IncludeTraceID includes trace_id in log output when available
	IncludeTraceID bool

	// IncludeSpanID includes span_id in log output when available
	IncludeSpanID bool

	// ServiceName is added to all log entries (optional)
	ServiceName string
}

// DefaultLoggerConfig returns sensible defaults.
func DefaultLoggerConfig() LoggerConfig {
	return LoggerConfig{
		Level:          LevelInfo,
		Format:         FormatText,
		Output:         os.Stdout,
		IncludeTraceID: true,
		IncludeSpanID:  true,
	}
}

// StructuredLogger implements Logger with trace correlation support.
type StructuredLogger struct {
	config     LoggerConfig
	baseFields []Field
	mu         sync.Mutex
}

// NewStructuredLogger creates a new structured logger with trace correlation.
func NewStructuredLogger(config *LoggerConfig) *StructuredLogger {
	cfg := DefaultLoggerConfig()
	if config != nil {
		if config.Level != 0 || config.Level == LevelDebug {
			cfg.Level = config.Level
		}
		if config.Format != 0 {
			cfg.Format = config.Format
		}
		if config.Output != nil {
			cfg.Output = config.Output
		}
		cfg.IncludeTraceID = config.IncludeTraceID
		cfg.IncludeSpanID = config.IncludeSpanID
		if config.ServiceName != "" {
			cfg.ServiceName = config.ServiceName
		}
	}

	return &StructuredLogger{
		config:     cfg,
		baseFields: nil,
	}
}

func (l *StructuredLogger) log(ctx context.Context, level LogLevel, msg string, fields ...Field) {
	if level < l.config.Level {
		return
	}

	// Build all fields
	allFields := make([]Field, 0, len(l.baseFields)+len(fields)+4)

	// Add timestamp
	allFields = append(allFields, F("timestamp", time.Now().Format(time.RFC3339)))

	// Add level
	allFields = append(allFields, F("level", level.String()))

	// Add service name if configured
	if l.config.ServiceName != "" {
		allFields = append(allFields, F("service", l.config.ServiceName))
	}

	// Extract trace context if available
	if ctx != nil && (l.config.IncludeTraceID || l.config.IncludeSpanID) {
		spanCtx := trace.SpanContextFromContext(ctx)
		if spanCtx.IsValid() {
			if l.config.IncludeTraceID {
				allFields = append(allFields, F("trace_id", spanCtx.TraceID().String()))
			}
			if l.config.IncludeSpanID {
				allFields = append(allFields, F("span_id", spanCtx.SpanID().String()))
			}
		}
	}

	// Add base fields
	allFields = append(allFields, l.baseFields...)

	// Add message
	allFields = append(allFields, F("msg", msg))

	// Add caller-provided fields
	allFields = append(allFields, fields...)

	// Format and output
	l.mu.Lock()
	defer l.mu.Unlock()

	switch l.config.Format {
	case FormatJSON:
		l.outputJSON(allFields)
	default:
		l.outputText(level, msg, allFields)
	}
}

func (l *StructuredLogger) outputJSON(fields []Field) {
	m := make(map[string]interface{}, len(fields))
	for _, f := range fields {
		m[f.Key] = f.Value
	}

	data, err := json.Marshal(m)
	if err != nil {
		fmt.Fprintf(l.config.Output, `{"error":"failed to marshal log entry: %v"}`, err)
		return
	}

	fmt.Fprintln(l.config.Output, string(data))
}

func (l *StructuredLogger) outputText(level LogLevel, msg string, fields []Field) {
	// Build text output: [timestamp] LEVEL: msg field=value field=value
	var traceID, spanID string
	extraFields := make([]Field, 0, len(fields))

	for _, f := range fields {
		switch f.Key {
		case "timestamp", "level", "msg":
			// Skip these, handled separately
		case "trace_id":
			if s, ok := f.Value.(string); ok {
				traceID = s
			}
		case "span_id":
			if s, ok := f.Value.(string); ok {
				spanID = s
			}
		default:
			extraFields = append(extraFields, f)
		}
	}

	// Format: [2024-01-01T12:00:00Z] INFO: message [trace_id=xxx span_id=yyy] key=value
	timestamp := time.Now().Format(time.RFC3339)
	output := fmt.Sprintf("[%s] %s: %s", timestamp, level.String(), msg)

	// Add trace context if available
	if traceID != "" || spanID != "" {
		output += " ["
		if traceID != "" {
			output += "trace_id=" + traceID
			if spanID != "" {
				output += " "
			}
		}
		if spanID != "" {
			output += "span_id=" + spanID
		}
		output += "]"
	}

	// Add extra fields
	for _, f := range extraFields {
		output += fmt.Sprintf(" %s=%v", f.Key, f.Value)
	}

	fmt.Fprintln(l.config.Output, output)
}

func (l *StructuredLogger) Debug(ctx context.Context, msg string, fields ...Field) {
	l.log(ctx, LevelDebug, msg, fields...)
}

func (l *StructuredLogger) Info(ctx context.Context, msg string, fields ...Field) {
	l.log(ctx, LevelInfo, msg, fields...)
}

func (l *StructuredLogger) Warn(ctx context.Context, msg string, fields ...Field) {
	l.log(ctx, LevelWarn, msg, fields...)
}

func (l *StructuredLogger) Error(ctx context.Context, msg string, fields ...Field) {
	l.log(ctx, LevelError, msg, fields...)
}

func (l *StructuredLogger) WithFields(fields ...Field) Logger {
	newLogger := &StructuredLogger{
		config:     l.config,
		baseFields: make([]Field, len(l.baseFields)+len(fields)),
	}
	copy(newLogger.baseFields, l.baseFields)
	copy(newLogger.baseFields[len(l.baseFields):], fields)
	return newLogger
}

// NoOpLogger is a logger that discards all output.
type NoOpLogger struct{}

func (n *NoOpLogger) Debug(ctx context.Context, msg string, fields ...Field) {}
func (n *NoOpLogger) Info(ctx context.Context, msg string, fields ...Field)  {}
func (n *NoOpLogger) Warn(ctx context.Context, msg string, fields ...Field)  {}
func (n *NoOpLogger) Error(ctx context.Context, msg string, fields ...Field) {}
func (n *NoOpLogger) WithFields(fields ...Field) Logger                      { return n }

// Global logger instance
var globalLogger Logger = &NoOpLogger{}
var globalLoggerMu sync.RWMutex

// SetGlobalLogger sets the global logger.
func SetGlobalLogger(l Logger) {
	globalLoggerMu.Lock()
	defer globalLoggerMu.Unlock()
	if l == nil {
		globalLogger = &NoOpLogger{}
	} else {
		globalLogger = l
	}
}

// GetGlobalLogger returns the current global logger.
func GetGlobalLogger() Logger {
	globalLoggerMu.RLock()
	defer globalLoggerMu.RUnlock()
	return globalLogger
}

// TraceIDFromContext extracts the trace ID from the context.
// Returns empty string if no trace context is available.
func TraceIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	spanCtx := trace.SpanContextFromContext(ctx)
	if !spanCtx.IsValid() {
		return ""
	}
	return spanCtx.TraceID().String()
}

// SpanIDFromContext extracts the span ID from the context.
// Returns empty string if no trace context is available.
func SpanIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	spanCtx := trace.SpanContextFromContext(ctx)
	if !spanCtx.IsValid() {
		return ""
	}
	return spanCtx.SpanID().String()
}

// TraceContextFields returns logging fields for the trace context.
// Useful for manually adding trace context to existing loggers.
func TraceContextFields(ctx context.Context) []Field {
	if ctx == nil {
		return nil
	}
	spanCtx := trace.SpanContextFromContext(ctx)
	if !spanCtx.IsValid() {
		return nil
	}
	return []Field{
		F("trace_id", spanCtx.TraceID().String()),
		F("span_id", spanCtx.SpanID().String()),
	}
}
