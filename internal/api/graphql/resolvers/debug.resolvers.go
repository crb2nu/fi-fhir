package resolvers

import (
	"time"

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
		CreatedAt:   time.Now(),
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
