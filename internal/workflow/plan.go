package workflow

import (
	"fmt"
	"reflect"
	"strings"
)

// PlanDiagnostic is a bounded, configuration-free planning diagnostic.
type PlanDiagnostic struct {
	Code string `json:"code"`
	Path string `json:"path"`
}

// ActionPlan identifies one action without exposing its execution configuration.
type ActionPlan struct {
	ID                    string `json:"id"`
	Type                  string `json:"type"`
	DestinationArtifactID string `json:"destination_artifact_id,omitempty"`
}

// RoutePlan is the deterministic, side-effect-free plan for one workflow route.
type RoutePlan struct {
	Name            string       `json:"name"`
	Matched         bool         `json:"matched"`
	Skipped         bool         `json:"skipped,omitempty"`
	SkipReason      string       `json:"skip_reason,omitempty"`
	TransformCount  int          `json:"transform_count"`
	Actions         []ActionPlan `json:"actions,omitempty"`
	DiagnosticCodes []string     `json:"diagnostic_codes,omitempty"`
}

// PlanResult contains route plans and safe diagnostics in declaration order.
type PlanResult struct {
	Routes      []RoutePlan      `json:"routes"`
	Diagnostics []PlanDiagnostic `json:"diagnostics,omitempty"`
}

type plannerRoute struct {
	name           string
	filter         Filter
	invalidCEL     bool
	transformCount int
	actions        []ActionPlan
}

// Planner evaluates route filters without owning any execution-capable component.
type Planner struct {
	routes       []plannerRoute
	celEvaluator *CELEvaluator
}

// NewPlanner copies only filter and public planning metadata from a workflow.
// Legacy actions without IDs receive a revision-local deterministic ordinal ID.
func NewPlanner(workflow *Workflow) (*Planner, error) {
	if workflow == nil {
		return nil, fmt.Errorf("workflow is required")
	}
	if len(workflow.Routes) == 0 {
		return nil, fmt.Errorf("workflow must contain at least one route")
	}
	evaluator, err := newPublishedCELEvaluator()
	if err != nil {
		return nil, fmt.Errorf("create planner CEL evaluator: %w", err)
	}

	routes := make([]plannerRoute, len(workflow.Routes))
	seenRoutes := make(map[string]struct{}, len(workflow.Routes))
	for routeIndex, route := range workflow.Routes {
		if strings.TrimSpace(route.Name) == "" {
			return nil, fmt.Errorf("route %d name is required", routeIndex)
		}
		if _, duplicate := seenRoutes[route.Name]; duplicate {
			return nil, fmt.Errorf("route %d name is duplicated", routeIndex)
		}
		seenRoutes[route.Name] = struct{}{}
		if len(route.Actions) == 0 {
			return nil, fmt.Errorf("route %d must contain at least one action", routeIndex)
		}

		plannedRoute := plannerRoute{
			name: route.Name,
			filter: Filter{
				EventType: append(StringOrSlice(nil), route.Filter.EventType...),
				Source:    append(StringOrSlice(nil), route.Filter.Source...),
				Condition: route.Filter.Condition,
			},
			transformCount: len(route.Transforms),
			actions:        make([]ActionPlan, len(route.Actions)),
		}
		if plannedRoute.filter.Condition != "" {
			if err := evaluator.CompileBoolean(plannedRoute.filter.Condition); err != nil {
				plannedRoute.invalidCEL = true
			}
		}
		seenActions := make(map[string]struct{}, len(route.Actions))
		for actionIndex, action := range route.Actions {
			if strings.TrimSpace(action.Type) == "" {
				return nil, fmt.Errorf("route %d action %d type is required", routeIndex, actionIndex)
			}
			actionID := action.ID
			if actionID == "" {
				actionID = fmt.Sprintf("%s%04d", legacyActionIDPrefix, actionIndex+1)
			} else {
				if !validPublishedActionID(actionID) {
					return nil, fmt.Errorf("route %d action %d ID is invalid", routeIndex, actionIndex)
				}
				if strings.HasPrefix(actionID, legacyActionIDPrefix) {
					return nil, fmt.Errorf("route %d action %d ID uses a reserved prefix", routeIndex, actionIndex)
				}
			}
			if _, duplicate := seenActions[actionID]; duplicate {
				return nil, fmt.Errorf("route %d action %d ID is duplicated", routeIndex, actionIndex)
			}
			seenActions[actionID] = struct{}{}
			plannedRoute.actions[actionIndex] = ActionPlan{
				ID:                    actionID,
				Type:                  action.Type,
				DestinationArtifactID: action.Destination,
			}
		}
		routes[routeIndex] = plannedRoute
	}

	return &Planner{routes: routes, celEvaluator: evaluator}, nil
}

// Plan returns a new deterministic plan and never invokes transforms or actions.
func (p *Planner) Plan(event interface{}) (PlanResult, error) {
	if p == nil || p.celEvaluator == nil {
		return PlanResult{}, fmt.Errorf("planner is not configured")
	}
	result := PlanResult{Routes: make([]RoutePlan, len(p.routes))}
	for routeIndex, route := range p.routes {
		plan := RoutePlan{Name: route.name}
		if route.invalidCEL {
			const code = "INVALID_CEL"
			path := fmt.Sprintf("routes[%d].filter.condition", routeIndex)
			plan.DiagnosticCodes = []string{code}
			result.Diagnostics = append(result.Diagnostics, PlanDiagnostic{Code: code, Path: path})
			result.Routes[routeIndex] = plan
			continue
		}
		matched, err := matchWorkflowFilterMode(event, route.filter, p.celEvaluator, true)
		if err != nil {
			const code = "INVALID_CEL"
			path := fmt.Sprintf("routes[%d].filter.condition", routeIndex)
			plan.DiagnosticCodes = []string{code}
			result.Diagnostics = append(result.Diagnostics, PlanDiagnostic{Code: code, Path: path})
			result.Routes[routeIndex] = plan
			continue
		}
		plan.Matched = matched
		if matched {
			plan.TransformCount = route.transformCount
			plan.Actions = append([]ActionPlan(nil), route.actions...)
		}
		result.Routes[routeIndex] = plan
	}
	return result, nil
}

// matchWorkflowFilter is shared by the legacy engine and the pure planner. The
// engine preserves its historical fail-closed behavior; the planner catalogs errors.
func matchWorkflowFilter(event interface{}, filter Filter, evaluator *CELEvaluator) (bool, error) {
	return matchWorkflowFilterMode(event, filter, evaluator, false)
}

func matchWorkflowFilterMode(event interface{}, filter Filter, evaluator *CELEvaluator, requireBoolean bool) (bool, error) {
	eventType := workflowEventType(event)
	source := workflowEventSource(event)
	if len(filter.EventType) > 0 && !filter.EventType.Contains(eventType) {
		return false, nil
	}
	if len(filter.Source) > 0 && !filter.Source.Contains(source) {
		return false, nil
	}
	if filter.Condition == "" {
		return true, nil
	}
	if evaluator == nil {
		return false, fmt.Errorf("CEL evaluator is not configured")
	}
	if requireBoolean {
		return evaluator.EvaluateBoolean(filter.Condition, event)
	}
	return evaluator.Evaluate(filter.Condition, event)
}

func workflowEventType(event interface{}) string {
	if eventMap, ok := event.(map[string]interface{}); ok {
		if value, ok := eventMap["type"].(string); ok {
			return value
		}
		return ""
	}
	value := reflect.ValueOf(event)
	if !value.IsValid() {
		return ""
	}
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return ""
		}
		value = value.Elem()
	}
	if value.Kind() != reflect.Struct {
		return ""
	}
	if metadata := value.FieldByName("EventMeta"); metadata.IsValid() && metadata.Kind() == reflect.Struct {
		if field := metadata.FieldByName("Type"); field.IsValid() {
			return fmt.Sprintf("%v", field.Interface())
		}
	}
	if field := value.FieldByName("Type"); field.IsValid() {
		return fmt.Sprintf("%v", field.Interface())
	}
	return ""
}

func workflowEventSource(event interface{}) string {
	if eventMap, ok := event.(map[string]interface{}); ok {
		if value, ok := eventMap["source"].(string); ok {
			return value
		}
		return ""
	}
	value := reflect.ValueOf(event)
	if !value.IsValid() {
		return ""
	}
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return ""
		}
		value = value.Elem()
	}
	if value.Kind() != reflect.Struct {
		return ""
	}
	if metadata := value.FieldByName("EventMeta"); metadata.IsValid() && metadata.Kind() == reflect.Struct {
		if field := metadata.FieldByName("Source"); field.IsValid() {
			return fmt.Sprintf("%v", field.Interface())
		}
	}
	if field := value.FieldByName("Source"); field.IsValid() {
		return fmt.Sprintf("%v", field.Interface())
	}
	return ""
}
