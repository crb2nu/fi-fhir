package workflow

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/llm"
)

// WorkflowStats holds aggregate counters for the engine.
type WorkflowStats struct {
	EventsProcessed int64
	Errors          int64
	LastEventTime   *time.Time
}

// Engine processes events through workflow routes.
type Engine struct {
	workflow     *Workflow
	actions      map[string]ActionHandler
	celEvaluator *CELEvaluator
	transformer  *Transformer
	dlq          DeadLetterQueue
	dlqConfig    DLQConfig
	metrics      Metrics
	tracer       Tracer
	logger       Logger
	llmClient    llm.Client

	totalEventsProcessed int64
	totalErrors          int64
	lastEventTime        time.Time
	statsMu              sync.RWMutex
}

// ActionHandler executes a specific action type.
type ActionHandler interface {
	Execute(event interface{}, config map[string]string) error
}

// ActionHandlerFunc is a function adapter for ActionHandler.
type ActionHandlerFunc func(event interface{}, config map[string]string) error

// Execute implements ActionHandler.
func (f ActionHandlerFunc) Execute(event interface{}, config map[string]string) error {
	return f(event, config)
}

// ContextActionHandler is an optional interface that action handlers can implement
// to receive context for distributed tracing and cancellation propagation.
type ContextActionHandler interface {
	ExecuteWithContext(ctx context.Context, event interface{}, config map[string]string) error
}

// ContextActionHandlerFunc is a function adapter for ContextActionHandler.
type ContextActionHandlerFunc func(ctx context.Context, event interface{}, config map[string]string) error

// ExecuteWithContext implements ContextActionHandler.
func (f ContextActionHandlerFunc) ExecuteWithContext(ctx context.Context, event interface{}, config map[string]string) error {
	return f(ctx, event, config)
}

// Execute implements ActionHandler (falls back to background context).
func (f ContextActionHandlerFunc) Execute(event interface{}, config map[string]string) error {
	return f(context.Background(), event, config)
}

// Result represents the outcome of processing an event.
type Result struct {
	RouteResults []RouteResult
}

// RouteResult represents the outcome of a single route.
type RouteResult struct {
	RouteName       string
	Matched         bool
	TransformsRun   int
	TransformErrors []error
	ActionsRun      int
	ActionErrors    []error
	Skipped         bool
	SkipReason      string
}

// HasErrors returns true if any route had errors (transform or action).
func (r *Result) HasErrors() bool {
	for _, rr := range r.RouteResults {
		if len(rr.TransformErrors) > 0 || len(rr.ActionErrors) > 0 {
			return true
		}
	}
	return false
}

// AllErrors returns all errors (transform and action) from all routes.
func (r *Result) AllErrors() []error {
	var errors []error
	for _, rr := range r.RouteResults {
		errors = append(errors, rr.TransformErrors...)
		errors = append(errors, rr.ActionErrors...)
	}
	return errors
}

// NewEngine creates a new workflow engine.
func NewEngine(workflow *Workflow) (*Engine, error) {
	celEval, err := NewCELEvaluator()
	if err != nil {
		return nil, fmt.Errorf("failed to create CEL evaluator: %w", err)
	}

	e := &Engine{
		workflow:     workflow,
		actions:      make(map[string]ActionHandler),
		celEvaluator: celEval,
		transformer:  NewTransformer(nil), // Default transformer without terminology
		metrics:      &NoOpMetrics{},      // Default to no-op metrics
		tracer:       &NoOpTracer{},       // Default to no-op tracer
		logger:       &NoOpLogger{},       // Default to no-op logger
	}

	// Register built-in action handlers
	e.RegisterAction("log", ActionHandlerFunc(logAction))
	e.RegisterAction("webhook", ContextActionHandlerFunc(webhookAction))
	e.RegisterAction("fhir", ContextActionHandlerFunc(fhirAction))
	e.RegisterAction("email", ContextActionHandlerFunc(emailAction))
	e.RegisterAction("exec", ContextActionHandlerFunc(execAction))
	e.RegisterAction("file", ActionHandlerFunc(fileAction))
	e.RegisterAction("database", ActionHandlerFunc(databaseAction))
	e.RegisterAction("queue", ActionHandlerFunc(queueAction))
	e.RegisterAction("event_store", ContextActionHandlerFunc(eventStoreAction))
	e.RegisterAction("athena", ContextActionHandlerFunc(athenaAction))

	return e, nil
}

// SetMetrics configures a metrics collector for observability.
// If not set, metrics are discarded (no-op). Use NewInMemoryMetrics for testing
// or implement the Metrics interface for production backends like Prometheus.
func (e *Engine) SetMetrics(m Metrics) {
	if m == nil {
		e.metrics = &NoOpMetrics{}
	} else {
		e.metrics = m
	}
}

// GetMetrics returns the configured metrics collector.
func (e *Engine) GetMetrics() Metrics {
	return e.metrics
}

// SetTracer configures a tracer for distributed tracing.
// If not set, tracing is disabled (no-op). Use NewOTelTracer for OpenTelemetry.
func (e *Engine) SetTracer(t Tracer) {
	if t == nil {
		e.tracer = &NoOpTracer{}
	} else {
		e.tracer = t
	}
}

// GetTracer returns the configured tracer.
func (e *Engine) GetTracer() Tracer {
	return e.tracer
}

// SetLogger configures a logger for trace-correlated logging.
// If not set, logging is discarded (no-op). Use NewStructuredLogger for
// production with trace ID correlation.
func (e *Engine) SetLogger(l Logger) {
	if l == nil {
		e.logger = &NoOpLogger{}
	} else {
		e.logger = l
	}
}

// GetLogger returns the configured logger.
func (e *Engine) GetLogger() Logger {
	return e.logger
}

// SetTerminologyMapper configures a terminology mapper for transforms.
// Pass a TerminologyMapperInterface implementation or use NewTerminologyMapperAdapter.
func (e *Engine) SetTerminologyMapper(mapper TerminologyMapperInterface) {
	e.transformer = NewTransformer(mapper)
}

// SetDLQ configures a dead letter queue for failed events.
// Events that fail action execution will be stored in the DLQ for later reprocessing.
func (e *Engine) SetDLQ(dlq DeadLetterQueue, config ...DLQConfig) {
	e.dlq = dlq
	if len(config) > 0 {
		e.dlqConfig = config[0]
	} else {
		e.dlqConfig = DefaultDLQConfig()
	}
}

// GetDLQ returns the configured dead letter queue, or nil if not set.
func (e *Engine) GetDLQ() DeadLetterQueue {
	return e.dlq
}

// SetLLMClient configures an LLM client for AI-powered actions.
// When set, this enables the following actions:
//   - llm_extract: Extract clinical entities from unstructured text
//   - llm_quality_check: Perform data quality analysis
func (e *Engine) SetLLMClient(client llm.Client) {
	e.llmClient = client
	if client != nil {
		// Register LLM-powered actions
		e.RegisterAction("llm_extract", makeLLMExtractAction(client))
		e.RegisterAction("llm_quality_check", makeLLMQualityCheckAction(client))
	}
}

// GetLLMClient returns the configured LLM client, or nil if not set.
func (e *Engine) GetLLMClient() llm.Client {
	return e.llmClient
}

// GetWorkflow returns the workflow loaded into this engine.
func (e *Engine) GetWorkflow() *Workflow {
	return e.workflow
}

// GetStats returns aggregate processing statistics.
func (e *Engine) GetStats() WorkflowStats {
	e.statsMu.RLock()
	defer e.statsMu.RUnlock()
	var lastEvent *time.Time
	if !e.lastEventTime.IsZero() {
		t := e.lastEventTime
		lastEvent = &t
	}
	return WorkflowStats{
		EventsProcessed: e.totalEventsProcessed,
		Errors:          e.totalErrors,
		LastEventTime:   lastEvent,
	}
}

// RegisterAction registers a custom action handler.
func (e *Engine) RegisterAction(name string, handler ActionHandler) {
	e.actions[name] = handler
}

// Process routes an event through the workflow.
// For distributed tracing support, use ProcessWithContext instead.
func (e *Engine) Process(event interface{}) *Result {
	return e.ProcessWithContext(context.Background(), event)
}

// ProcessWithContext routes an event through the workflow with tracing context.
// The context can carry trace information for distributed tracing.
func (e *Engine) ProcessWithContext(ctx context.Context, event interface{}) *Result {
	startTime := time.Now()
	eventType := e.getEventType(event)
	source := e.getEventSource(event)

	// Start root span for event processing
	ctx, rootSpan := e.tracer.StartSpan(ctx, SpanNameProcess,
		WithSpanKind(SpanKindConsumer),
		WithAttributes(
			Attr(AttrEventType, eventType),
			Attr(AttrEventSource, source),
		),
	)
	defer rootSpan.End()

	// Log event processing start with trace correlation
	e.logger.Debug(ctx, "processing event",
		F("event_type", eventType),
		F("source", source),
		F("workflow", e.workflow.Name),
	)

	result := &Result{
		RouteResults: make([]RouteResult, 0, len(e.workflow.Routes)),
	}

	for _, route := range e.workflow.Routes {
		rr := RouteResult{
			RouteName: route.Name,
		}

		if !e.matches(event, route.Filter) {
			rr.Matched = false
			result.RouteResults = append(result.RouteResults, rr)
			continue
		}

		rr.Matched = true

		// Start span for route processing
		routeCtx, routeSpan := e.tracer.StartSpan(ctx, SpanNameRoute,
			WithAttributes(
				Attr(AttrRouteName, route.Name),
				Attr(AttrRouteMatched, true),
			),
		)

		// Record that event was routed to this route
		e.metrics.EventRouted(eventType, route.Name)

		// Apply transforms
		transformed := event
		for i, transform := range route.Transforms {
			transformCtx, transformSpan := e.tracer.StartSpan(routeCtx, SpanNameTransform,
				WithAttributes(
					Attr(AttrTransformType, getTransformType(transform)),
					Attr("transform.index", i),
				),
			)

			var err error
			transformed, err = e.transformer.ApplyWithContext(transformCtx, transformed, transform)
			if err != nil {
				transformSpan.RecordError(err)
				transformSpan.SetStatus(SpanStatusError, err.Error())
				rr.TransformErrors = append(rr.TransformErrors,
					fmt.Errorf("transform failed: %w", err))
				// Continue with other transforms despite errors
			} else {
				transformSpan.SetStatus(SpanStatusOK, "")
			}
			transformSpan.End()
			rr.TransformsRun++
		}

		// Execute actions
		for _, action := range route.Actions {
			actionCtx, actionSpan := e.tracer.StartSpan(routeCtx, SpanNameAction,
				WithAttributes(
					Attr(AttrActionType, action.Type),
					Attr(AttrRouteName, route.Name),
				),
			)

			handler, ok := e.actions[action.Type]
			if !ok {
				err := fmt.Errorf("unknown action type: %s", action.Type)
				actionSpan.RecordError(err)
				actionSpan.SetStatus(SpanStatusError, err.Error())
				actionSpan.End()
				rr.ActionErrors = append(rr.ActionErrors, err)
				e.metrics.ActionExecuted(action.Type, route.Name, false, 0)
				continue
			}

			actionStart := time.Now()

			// Check if handler supports context-aware execution for tracing
			var err error
			if ctxHandler, ok := handler.(ContextActionHandler); ok {
				err = ctxHandler.ExecuteWithContext(actionCtx, transformed, action.Config)
			} else {
				err = handler.Execute(transformed, action.Config)
			}
			actionDuration := time.Since(actionStart)

			if err != nil {
				actionErr := fmt.Errorf("action %s failed: %w", action.Type, err)
				actionSpan.RecordError(err)
				actionSpan.SetStatus(SpanStatusError, err.Error())
				actionSpan.SetAttribute(AttrActionSuccess, false)
				rr.ActionErrors = append(rr.ActionErrors, actionErr)

				// Log action failure with trace correlation
				e.logger.Error(actionCtx, "action failed",
					F("action_type", action.Type),
					F("route", route.Name),
					F("error", err.Error()),
					F("duration_ms", actionDuration.Milliseconds()),
				)

				// Record failed action metric
				e.metrics.ActionExecuted(action.Type, route.Name, false, actionDuration)

				// Send to DLQ if configured
				if e.dlq != nil {
					e.sendToDLQ(transformed, route.Name, action.Type, err)
				}
			} else {
				// Record successful action metric
				actionSpan.SetStatus(SpanStatusOK, "")
				actionSpan.SetAttribute(AttrActionSuccess, true)
				e.metrics.ActionExecuted(action.Type, route.Name, true, actionDuration)
			}
			actionSpan.End()
			rr.ActionsRun++
		}

		routeSpan.End()
		result.RouteResults = append(result.RouteResults, rr)
	}

	// Record overall event processing metric
	duration := time.Since(startTime)
	success := !result.HasErrors()
	e.metrics.EventProcessed(eventType, source, success, duration)

	// Update aggregate stats
	e.statsMu.Lock()
	e.totalEventsProcessed++
	e.lastEventTime = time.Now()
	for _, rr := range result.RouteResults {
		e.totalErrors += int64(len(rr.ActionErrors) + len(rr.TransformErrors))
	}
	e.statsMu.Unlock()

	// Set root span status
	if success {
		rootSpan.SetStatus(SpanStatusOK, "")
		e.logger.Info(ctx, "event processed",
			F("event_type", eventType),
			F("source", source),
			F("success", true),
			F("duration_ms", duration.Milliseconds()),
			F("routes_matched", countMatchedRoutes(result)),
		)
	} else {
		rootSpan.SetStatus(SpanStatusError, "event processing had errors")
		rootSpan.SetAttribute("error.count", len(result.AllErrors()))
		e.logger.Warn(ctx, "event processed with errors",
			F("event_type", eventType),
			F("source", source),
			F("success", false),
			F("duration_ms", duration.Milliseconds()),
			F("error_count", len(result.AllErrors())),
		)
	}

	return result
}

// countMatchedRoutes counts how many routes matched in the result.
func countMatchedRoutes(result *Result) int {
	count := 0
	for _, rr := range result.RouteResults {
		if rr.Matched {
			count++
		}
	}
	return count
}

// getTransformType returns a string identifier for the transform type.
func getTransformType(t Transform) string {
	if t.SetField != "" {
		return "set_field"
	}
	if t.MapTerminology != nil {
		return "map_terminology"
	}
	if t.Redact != nil {
		return "redact"
	}
	return "unknown"
}

// sendToDLQ sends a failed event to the dead letter queue.
func (e *Engine) sendToDLQ(event interface{}, routeName, actionType string, err error) {
	now := time.Now()
	errorType := ClassifyError(err)

	failedEvent := &FailedEvent{
		ID:           GenerateFailedEventID(),
		Event:        event,
		RouteName:    routeName,
		ActionType:   actionType,
		Error:        err.Error(),
		ErrorType:    errorType,
		Attempts:     1,
		FirstFailure: now,
		LastFailure:  now,
		Metadata:     make(map[string]string),
	}

	if pushErr := e.dlq.Push(failedEvent); pushErr != nil {
		// Log error but don't fail the processing
		fmt.Printf("Warning: failed to push event to DLQ: %v\n", pushErr)
	} else {
		// Record DLQ push metric
		e.metrics.DLQPushed(routeName, actionType, errorType)
	}

	// Call notification callback if configured
	if e.dlqConfig.OnDeadLetter != nil {
		e.dlqConfig.OnDeadLetter(failedEvent)
	}
}

// matches checks if an event matches a route filter.
func (e *Engine) matches(event interface{}, filter Filter) bool {
	eventType := e.getEventType(event)
	source := e.getEventSource(event)

	// Check event type filter
	if len(filter.EventType) > 0 && !filter.EventType.Contains(eventType) {
		return false
	}

	// Check source filter
	if len(filter.Source) > 0 && !filter.Source.Contains(source) {
		return false
	}

	// CEL condition evaluation
	if filter.Condition != "" {
		match, err := e.celEvaluator.Evaluate(filter.Condition, event)
		if err != nil {
			// Log error but don't match on evaluation failure
			return false
		}
		if !match {
			return false
		}
	}

	return true
}

// getEventType extracts the event type from an event using reflection or map access.
func (e *Engine) getEventType(event interface{}) string {
	// Handle map types (from JSON parsing)
	if m, ok := event.(map[string]interface{}); ok {
		if t, ok := m["type"].(string); ok {
			return t
		}
		return ""
	}

	v := reflect.ValueOf(event)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		return ""
	}

	// Look for EventMeta.Type or Type field
	if meta := v.FieldByName("EventMeta"); meta.IsValid() && meta.Kind() == reflect.Struct {
		if typeField := meta.FieldByName("Type"); typeField.IsValid() {
			return fmt.Sprintf("%v", typeField.Interface())
		}
	}

	if typeField := v.FieldByName("Type"); typeField.IsValid() {
		return fmt.Sprintf("%v", typeField.Interface())
	}

	return ""
}

// getEventSource extracts the source from an event using reflection or map access.
func (e *Engine) getEventSource(event interface{}) string {
	// Handle map types (from JSON parsing)
	if m, ok := event.(map[string]interface{}); ok {
		if s, ok := m["source"].(string); ok {
			return s
		}
		return ""
	}

	v := reflect.ValueOf(event)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		return ""
	}

	// Look for EventMeta.Source or Source field
	if meta := v.FieldByName("EventMeta"); meta.IsValid() && meta.Kind() == reflect.Struct {
		if sourceField := meta.FieldByName("Source"); sourceField.IsValid() {
			return fmt.Sprintf("%v", sourceField.Interface())
		}
	}

	if sourceField := v.FieldByName("Source"); sourceField.IsValid() {
		return fmt.Sprintf("%v", sourceField.Interface())
	}

	return ""
}

// DryRun simulates processing without executing actions.
func (e *Engine) DryRun(event interface{}) *Result {
	result := &Result{
		RouteResults: make([]RouteResult, 0, len(e.workflow.Routes)),
	}

	for _, route := range e.workflow.Routes {
		rr := RouteResult{
			RouteName:  route.Name,
			Skipped:    true,
			SkipReason: "dry-run mode",
		}

		if !e.matches(event, route.Filter) {
			rr.Matched = false
			rr.SkipReason = "filter did not match"
		} else {
			rr.Matched = true
			rr.ActionsRun = len(route.Actions)
		}

		result.RouteResults = append(result.RouteResults, rr)
	}

	return result
}

// ReprocessDLQResult represents the outcome of reprocessing DLQ events.
type ReprocessDLQResult struct {
	Processed int     // Events successfully reprocessed
	Failed    int     // Events that failed again
	Skipped   int     // Events skipped (e.g., exceeded max attempts)
	Errors    []error // Non-fatal errors during reprocessing
}

// ReprocessDLQ attempts to reprocess events from the dead letter queue.
// If limit is 0, all events are attempted. Returns statistics about reprocessing.
func (e *Engine) ReprocessDLQ(limit int) *ReprocessDLQResult {
	if e.dlq == nil {
		return &ReprocessDLQResult{}
	}

	dlqResult := &ReprocessDLQResult{
		Errors: make([]error, 0),
	}

	events, err := e.dlq.List(limit)
	if err != nil {
		dlqResult.Errors = append(dlqResult.Errors, fmt.Errorf("failed to list DLQ events: %w", err))
		return dlqResult
	}

	for _, failedEvent := range events {
		// Check if max attempts exceeded
		if e.dlqConfig.MaxAttempts > 0 && failedEvent.Attempts >= e.dlqConfig.MaxAttempts {
			dlqResult.Skipped++
			continue
		}

		// Remove from DLQ first to prevent duplicate entries during reprocessing
		// (since Process() will add to DLQ if action fails)
		if removeErr := e.dlq.Remove(failedEvent.ID); removeErr != nil {
			dlqResult.Errors = append(dlqResult.Errors,
				fmt.Errorf("failed to remove DLQ event %s before reprocessing: %w", failedEvent.ID, removeErr))
			continue
		}

		// Temporarily disable DLQ to prevent new entries during reprocessing
		savedDLQ := e.dlq
		e.dlq = nil

		// Attempt to reprocess
		processResult := e.Process(failedEvent.Event)

		// Re-enable DLQ
		e.dlq = savedDLQ

		if processResult.HasErrors() {
			// Still failing, re-add to DLQ with updated attempt count
			failedEvent.Attempts++
			failedEvent.LastFailure = time.Now()
			failedEvent.Error = processResult.AllErrors()[0].Error()
			failedEvent.ErrorType = ClassifyError(processResult.AllErrors()[0])

			if pushErr := e.dlq.Push(failedEvent); pushErr != nil {
				dlqResult.Errors = append(dlqResult.Errors,
					fmt.Errorf("failed to re-add DLQ event %s: %w", failedEvent.ID, pushErr))
			}
			dlqResult.Failed++

			// Record failed DLQ pop metric
			e.metrics.DLQPopped(failedEvent.RouteName, false)
		} else {
			// Success! Event already removed from DLQ
			dlqResult.Processed++

			// Record successful DLQ pop metric
			e.metrics.DLQPopped(failedEvent.RouteName, true)
		}
	}

	return dlqResult
}

// ReprocessDLQEvent attempts to reprocess a specific event from the DLQ by ID.
// Returns the processing result and removes the event from DLQ on success.
func (e *Engine) ReprocessDLQEvent(id string) (*Result, error) {
	if e.dlq == nil {
		return nil, fmt.Errorf("DLQ not configured")
	}

	failedEvent, err := e.dlq.Get(id)
	if err != nil {
		return nil, fmt.Errorf("failed to get DLQ event: %w", err)
	}
	if failedEvent == nil {
		return nil, fmt.Errorf("event %s not found in DLQ", id)
	}

	// Remove from DLQ first to prevent duplicate entries
	if removeErr := e.dlq.Remove(id); removeErr != nil {
		return nil, fmt.Errorf("failed to remove DLQ event before reprocessing: %w", removeErr)
	}

	// Temporarily disable DLQ to prevent new entries during reprocessing
	savedDLQ := e.dlq
	e.dlq = nil

	processResult := e.Process(failedEvent.Event)

	// Re-enable DLQ
	e.dlq = savedDLQ

	if processResult.HasErrors() {
		// Still failing, re-add to DLQ with updated attempt count
		failedEvent.Attempts++
		failedEvent.LastFailure = time.Now()
		failedEvent.Error = processResult.AllErrors()[0].Error()
		failedEvent.ErrorType = ClassifyError(processResult.AllErrors()[0])

		if pushErr := e.dlq.Push(failedEvent); pushErr != nil {
			return processResult, fmt.Errorf("event still failing, and failed to re-add to DLQ: %w", pushErr)
		}

		// Record failed DLQ pop metric
		e.metrics.DLQPopped(failedEvent.RouteName, false)
		return processResult, nil
	}

	// Success! Event already removed from DLQ
	// Record successful DLQ pop metric
	e.metrics.DLQPopped(failedEvent.RouteName, true)
	return processResult, nil
}
