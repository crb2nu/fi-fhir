package resolvers

import (
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/graphql/model"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/workflow"
)

// toDebugSessionModel converts a workflow.DebugSession to a GraphQL model.
func toDebugSessionModel(session *workflow.DebugSession) *model.DebugSessionModel {
	snapshot := session.Snapshot()

	breakpoints := make([]*model.BreakpointModel, 0, len(snapshot.Breakpoints))
	for _, bp := range snapshot.Breakpoints {
		breakpoints = append(breakpoints, &model.BreakpointModel{
			ID:      bp.ID,
			Type:    string(bp.Type),
			Name:    bp.Name,
			Enabled: bp.Enabled,
		})
	}

	steps := make([]*model.DebugStepModel, 0, len(snapshot.Steps))
	for i := range snapshot.Steps {
		steps = append(steps, toDebugStepModel(&snapshot.Steps[i]))
	}

	return &model.DebugSessionModel{
		ID:          snapshot.ID,
		WorkflowID:  snapshot.WorkflowID,
		State:       string(snapshot.State),
		Breakpoints: breakpoints,
		Steps:       steps,
		CreatedAt:   snapshot.CreatedAt,
	}
}

// toDebugStepModel converts a workflow.DebugStep to a GraphQL model.
func toDebugStepModel(step *workflow.DebugStep) *model.DebugStepModel {
	return &model.DebugStepModel{
		StepNumber: step.StepNumber,
		Kind:       string(step.Kind),
		Name:       step.Name,
		Variables:  copyGraphQLVariables(step.Variables),
		Timestamp:  step.Timestamp,
		SpanName:   step.SpanName,
	}
}

func toGraphQLTraceSpans(spans []workflow.RecordedSpan) []model.TraceSpanModel {
	results := make([]model.TraceSpanModel, 0, len(spans))
	for _, span := range spans {
		events := make([]*model.TraceSpanEventModel, 0, len(span.Events))
		for _, event := range span.Events {
			events = append(events, &model.TraceSpanEventModel{
				Name:       event.Name,
				Timestamp:  event.Timestamp,
				Attributes: event.Attributes,
			})
		}

		results = append(results, model.TraceSpanModel{
			ID:         span.ID,
			Name:       span.Name,
			ParentID:   span.ParentID,
			StartTime:  span.StartTime,
			EndTime:    span.EndTime,
			Status:     traceStatusString(span.Status),
			Attributes: span.Attributes,
			Events:     events,
		})
	}
	return results
}

func traceStatusString(status workflow.SpanStatus) string {
	switch status {
	case workflow.SpanStatusOK:
		return "ok"
	case workflow.SpanStatusError:
		return "error"
	default:
		return "unset"
	}
}

func copyGraphQLVariables(input map[string]interface{}) map[string]interface{} {
	if input == nil {
		return nil
	}

	cloned := make(map[string]interface{}, len(input))
	for key, value := range input {
		switch typed := value.(type) {
		case map[string]interface{}:
			cloned[key] = copyGraphQLVariables(typed)
		case []interface{}:
			cloned[key] = copyGraphQLSlice(typed)
		default:
			cloned[key] = typed
		}
	}
	return cloned
}

func copyGraphQLSlice(input []interface{}) []interface{} {
	cloned := make([]interface{}, len(input))
	for i, value := range input {
		switch typed := value.(type) {
		case map[string]interface{}:
			cloned[i] = copyGraphQLVariables(typed)
		case []interface{}:
			cloned[i] = copyGraphQLSlice(typed)
		default:
			cloned[i] = typed
		}
	}
	return cloned
}
