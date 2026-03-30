package resolvers

import (
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/graphql/model"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/workflow"
)

// toDebugSessionModel converts a workflow.DebugSession to a GraphQL model.
func toDebugSessionModel(session *workflow.DebugSession) *model.DebugSessionModel {
	breakpoints := make([]*model.BreakpointModel, 0, len(session.Breakpoints))
	for _, bp := range session.Breakpoints {
		breakpoints = append(breakpoints, &model.BreakpointModel{
			ID:      bp.ID,
			Type:    string(bp.Type),
			Name:    bp.Name,
			Enabled: bp.Enabled,
		})
	}

	steps := make([]*model.DebugStepModel, 0, len(session.Steps))
	for i := range session.Steps {
		steps = append(steps, toDebugStepModel(&session.Steps[i]))
	}

	return &model.DebugSessionModel{
		ID:          session.ID,
		WorkflowID:  session.WorkflowID,
		State:       string(session.State),
		Breakpoints: breakpoints,
		Steps:       steps,
		CreatedAt:   session.CreatedAt,
	}
}

// toDebugStepModel converts a workflow.DebugStep to a GraphQL model.
func toDebugStepModel(step *workflow.DebugStep) *model.DebugStepModel {
	return &model.DebugStepModel{
		StepNumber: step.StepNumber,
		Kind:       string(step.Kind),
		Name:       step.Name,
		Variables:  step.Variables,
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
