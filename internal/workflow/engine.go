package workflow

import (
	"fmt"
	"reflect"
)

// Engine processes events through workflow routes.
type Engine struct {
	workflow     *Workflow
	actions      map[string]ActionHandler
	celEvaluator *CELEvaluator
	transformer  *Transformer
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
	}

	// Register built-in action handlers
	e.RegisterAction("log", ActionHandlerFunc(logAction))
	e.RegisterAction("webhook", ActionHandlerFunc(webhookAction))
	e.RegisterAction("fhir", ActionHandlerFunc(fhirAction))

	return e, nil
}

// SetTerminologyMapper configures a terminology mapper for transforms.
// Pass a TerminologyMapperInterface implementation or use NewTerminologyMapperAdapter.
func (e *Engine) SetTerminologyMapper(mapper TerminologyMapperInterface) {
	e.transformer = NewTransformer(mapper)
}

// RegisterAction registers a custom action handler.
func (e *Engine) RegisterAction(name string, handler ActionHandler) {
	e.actions[name] = handler
}

// Process routes an event through the workflow.
func (e *Engine) Process(event interface{}) *Result {
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

		// Apply transforms
		transformed := event
		for _, transform := range route.Transforms {
			var err error
			transformed, err = e.transformer.Apply(transformed, transform)
			if err != nil {
				rr.TransformErrors = append(rr.TransformErrors,
					fmt.Errorf("transform failed: %w", err))
				// Continue with other transforms despite errors
			}
			rr.TransformsRun++
		}

		// Execute actions
		for _, action := range route.Actions {
			handler, ok := e.actions[action.Type]
			if !ok {
				rr.ActionErrors = append(rr.ActionErrors,
					fmt.Errorf("unknown action type: %s", action.Type))
				continue
			}

			if err := handler.Execute(transformed, action.Config); err != nil {
				rr.ActionErrors = append(rr.ActionErrors,
					fmt.Errorf("action %s failed: %w", action.Type, err))
			}
			rr.ActionsRun++
		}

		result.RouteResults = append(result.RouteResults, rr)
	}

	return result
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
