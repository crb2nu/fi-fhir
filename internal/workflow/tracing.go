package workflow

import (
	"context"
	"fmt"
	"sync"
	"time"
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

func (n *noOpSpan) End()                                         {}
func (n *noOpSpan) SetAttribute(key string, value interface{})   {}
func (n *noOpSpan) SetStatus(code SpanStatus, message string)    {}
func (n *noOpSpan) RecordError(err error)                        {}
func (n *noOpSpan) AddEvent(name string, attrs ...SpanAttribute) {}

// MultiTracer fans out span lifecycle operations to multiple tracers while
// preserving the returned context chain from each delegate.
func MultiTracer(tracers ...Tracer) Tracer {
	filtered := make([]Tracer, 0, len(tracers))
	for _, tracer := range tracers {
		if tracer != nil {
			filtered = append(filtered, tracer)
		}
	}

	switch len(filtered) {
	case 0:
		return &NoOpTracer{}
	case 1:
		return filtered[0]
	default:
		return &multiTracer{tracers: filtered}
	}
}

type multiTracer struct {
	tracers []Tracer
}

func (m *multiTracer) StartSpan(ctx context.Context, name string, opts ...SpanOption) (context.Context, Span) {
	spans := make([]Span, 0, len(m.tracers))
	currentCtx := ctx
	for _, tracer := range m.tracers {
		nextCtx, span := tracer.StartSpan(currentCtx, name, opts...)
		currentCtx = nextCtx
		spans = append(spans, span)
	}
	return currentCtx, &multiSpan{spans: spans}
}

type multiSpan struct {
	spans []Span
}

func (m *multiSpan) End() {
	for i := len(m.spans) - 1; i >= 0; i-- {
		m.spans[i].End()
	}
}

func (m *multiSpan) SetAttribute(key string, value interface{}) {
	for _, span := range m.spans {
		span.SetAttribute(key, value)
	}
}

func (m *multiSpan) SetStatus(code SpanStatus, message string) {
	for _, span := range m.spans {
		span.SetStatus(code, message)
	}
}

func (m *multiSpan) RecordError(err error) {
	for _, span := range m.spans {
		span.RecordError(err)
	}
}

func (m *multiSpan) AddEvent(name string, attrs ...SpanAttribute) {
	for _, span := range m.spans {
		span.AddEvent(name, attrs...)
	}
}

type recordedSpanContextKey struct{}

// RecordedSpanEvent stores a point-in-time event captured by RecordingTracer.
type RecordedSpanEvent struct {
	Name       string
	Timestamp  time.Time
	Attributes map[string]interface{}
}

// RecordedSpan stores a lightweight in-memory trace span tree.
type RecordedSpan struct {
	ID         string
	Name       string
	ParentID   *string
	StartTime  time.Time
	EndTime    *time.Time
	Status     SpanStatus
	Attributes map[string]interface{}
	Events     []RecordedSpanEvent
}

// RecordingTracer captures spans in memory for later inspection.
type RecordingTracer struct {
	mu      sync.Mutex
	nextID  int64
	order   []string
	records map[string]*RecordedSpan
}

// NewRecordingTracer creates an empty in-memory tracer.
func NewRecordingTracer() *RecordingTracer {
	return &RecordingTracer{
		order:   make([]string, 0),
		records: make(map[string]*RecordedSpan),
	}
}

func (r *RecordingTracer) StartSpan(ctx context.Context, name string, opts ...SpanOption) (context.Context, Span) {
	cfg := &spanConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.nextID++
	spanID := fmt.Sprintf("span-%d", r.nextID)
	record := &RecordedSpan{
		ID:         spanID,
		Name:       name,
		StartTime:  time.Now(),
		Status:     SpanStatusUnset,
		Attributes: make(map[string]interface{}),
		Events:     make([]RecordedSpanEvent, 0),
	}
	if parentID, ok := ctx.Value(recordedSpanContextKey{}).(string); ok && parentID != "" {
		parentCopy := parentID
		record.ParentID = &parentCopy
	}
	for _, attr := range cfg.attributes {
		record.Attributes[attr.Key] = attr.Value
	}

	r.records[spanID] = record
	r.order = append(r.order, spanID)

	return context.WithValue(ctx, recordedSpanContextKey{}, spanID), &recordingSpan{
		tracer: r,
		spanID: spanID,
	}
}

// Snapshot returns the recorded spans in creation order.
func (r *RecordingTracer) Snapshot() []RecordedSpan {
	r.mu.Lock()
	defer r.mu.Unlock()

	spans := make([]RecordedSpan, 0, len(r.order))
	for _, spanID := range r.order {
		record, ok := r.records[spanID]
		if !ok {
			continue
		}
		copyRecord := RecordedSpan{
			ID:         record.ID,
			Name:       record.Name,
			StartTime:  record.StartTime,
			Status:     record.Status,
			Attributes: copyInterfaceMap(record.Attributes),
			Events:     make([]RecordedSpanEvent, 0, len(record.Events)),
		}
		if record.ParentID != nil {
			parentCopy := *record.ParentID
			copyRecord.ParentID = &parentCopy
		}
		if record.EndTime != nil {
			endCopy := *record.EndTime
			copyRecord.EndTime = &endCopy
		}
		for _, event := range record.Events {
			copyRecord.Events = append(copyRecord.Events, RecordedSpanEvent{
				Name:       event.Name,
				Timestamp:  event.Timestamp,
				Attributes: copyInterfaceMap(event.Attributes),
			})
		}
		spans = append(spans, copyRecord)
	}
	return spans
}

type recordingSpan struct {
	tracer *RecordingTracer
	spanID string
}

func (r *recordingSpan) End() {
	r.tracer.mu.Lock()
	defer r.tracer.mu.Unlock()

	record, ok := r.tracer.records[r.spanID]
	if !ok || record.EndTime != nil {
		return
	}
	end := time.Now()
	record.EndTime = &end
}

func (r *recordingSpan) SetAttribute(key string, value interface{}) {
	r.tracer.mu.Lock()
	defer r.tracer.mu.Unlock()

	if record, ok := r.tracer.records[r.spanID]; ok {
		record.Attributes[key] = value
	}
}

func (r *recordingSpan) SetStatus(code SpanStatus, message string) {
	r.tracer.mu.Lock()
	defer r.tracer.mu.Unlock()

	if record, ok := r.tracer.records[r.spanID]; ok {
		record.Status = code
		if message != "" {
			record.Attributes["status.message"] = message
		}
	}
}

func (r *recordingSpan) RecordError(err error) {
	if err == nil {
		return
	}
	r.SetStatus(SpanStatusError, err.Error())
}

func (r *recordingSpan) AddEvent(name string, attrs ...SpanAttribute) {
	r.tracer.mu.Lock()
	defer r.tracer.mu.Unlock()

	record, ok := r.tracer.records[r.spanID]
	if !ok {
		return
	}

	event := RecordedSpanEvent{
		Name:       name,
		Timestamp:  time.Now(),
		Attributes: make(map[string]interface{}),
	}
	for _, attr := range attrs {
		event.Attributes[attr.Key] = attr.Value
	}
	record.Events = append(record.Events, event)
}

func copyInterfaceMap(input map[string]interface{}) map[string]interface{} {
	if len(input) == 0 {
		return map[string]interface{}{}
	}
	output := make(map[string]interface{}, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

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
