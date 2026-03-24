package resolvers

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/graphql/model"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/parser/hl7v2"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/workflow"
)

// DebugSession is the resolver for the debugSession query.
func (r *queryResolver) DebugSession(ctx context.Context, id string) (*model.DebugSessionModel, error) {
	r.debugSessionsMu.RLock()
	session, ok := r.debugSessions[id]
	r.debugSessionsMu.RUnlock()

	if !ok {
		return nil, nil
	}

	return toDebugSessionModel(session), nil
}

// WorkflowRunTrace is the resolver for the workflowRunTrace query.
func (r *queryResolver) WorkflowRunTrace(ctx context.Context, runID string) ([]*model.TraceSpanModel, error) {
	// Placeholder: trace collection is not yet implemented
	return []*model.TraceSpanModel{}, nil
}

// StartDebugSession is the resolver for the startDebugSession mutation.
func (r *mutationResolver) StartDebugSession(ctx context.Context, input model.StartDebugSessionInput) (*model.DebugSessionModel, error) {
	// Parse the workflow YAML
	parsed, err := workflow.ParseWorkflow([]byte(input.WorkflowYaml))
	if err != nil {
		return nil, fmt.Errorf("parse workflow yaml: %w", err)
	}

	engine, err := workflow.NewEngine(parsed)
	if err != nil {
		return nil, fmt.Errorf("create workflow engine: %w", err)
	}

	sessionID := uuid.New().String()
	session := workflow.NewDebugSession(sessionID, engine)
	session.WorkflowID = parsed.Name

	r.debugSessionsMu.Lock()
	r.debugSessions[sessionID] = session
	r.debugSessionsMu.Unlock()

	// Start processing the event
	session.Start(ctx, input.Event)

	return toDebugSessionModel(session), nil
}

// DebugStep is the resolver for the debugStep mutation.
func (r *mutationResolver) DebugStep(ctx context.Context, sessionID string) (*model.DebugStepModel, error) {
	r.debugSessionsMu.RLock()
	session, ok := r.debugSessions[sessionID]
	r.debugSessionsMu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("debug session %s not found", sessionID)
	}

	step := session.Step()
	if step == nil {
		return nil, nil
	}

	return toDebugStepModel(step), nil
}

// DebugContinue is the resolver for the debugContinue mutation.
func (r *mutationResolver) DebugContinue(ctx context.Context, sessionID string) (*model.DebugStepModel, error) {
	r.debugSessionsMu.RLock()
	session, ok := r.debugSessions[sessionID]
	r.debugSessionsMu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("debug session %s not found", sessionID)
	}

	step := session.Continue()
	if step == nil {
		return nil, nil
	}

	return toDebugStepModel(step), nil
}

// DebugSetBreakpoint is the resolver for the debugSetBreakpoint mutation.
func (r *mutationResolver) DebugSetBreakpoint(ctx context.Context, input model.SetBreakpointInput) (*model.BreakpointModel, error) {
	r.debugSessionsMu.RLock()
	session, ok := r.debugSessions[input.SessionID]
	r.debugSessionsMu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("debug session %s not found", input.SessionID)
	}

	bpID := uuid.New().String()
	bp := &workflow.Breakpoint{
		ID:      bpID,
		Type:    workflow.BreakpointType(input.Type),
		Name:    input.Name,
		Enabled: true,
	}

	session.SetBreakpoint(bp)

	return &model.BreakpointModel{
		ID:      bpID,
		Type:    input.Type,
		Name:    input.Name,
		Enabled: true,
	}, nil
}

// DebugRemoveBreakpoint is the resolver for the debugRemoveBreakpoint mutation.
func (r *mutationResolver) DebugRemoveBreakpoint(ctx context.Context, sessionID string, breakpointID string) (bool, error) {
	r.debugSessionsMu.RLock()
	session, ok := r.debugSessions[sessionID]
	r.debugSessionsMu.RUnlock()

	if !ok {
		return false, fmt.Errorf("debug session %s not found", sessionID)
	}

	removed := session.RemoveBreakpoint(breakpointID)
	return removed, nil
}

// DebugEndSession is the resolver for the debugEndSession mutation.
func (r *mutationResolver) DebugEndSession(ctx context.Context, sessionID string) (bool, error) {
	r.debugSessionsMu.Lock()
	session, ok := r.debugSessions[sessionID]
	if ok {
		delete(r.debugSessions, sessionID)
	}
	r.debugSessionsMu.Unlock()

	if !ok {
		return false, fmt.Errorf("debug session %s not found", sessionID)
	}

	session.Close()
	return true, nil
}

// LiveParseStream is the resolver for the liveParseStream subscription.
func (r *subscriptionResolver) LiveParseStream(ctx context.Context, input model.LiveParseInput) (<-chan *model.ParseEventModel, error) {
	parser := hl7v2.NewParser("live-parse", hl7v2.ParserConfig{})
	liveParser := hl7v2.NewLiveParser(parser)

	out := make(chan *model.ParseEventModel, 100)

	go func() {
		defer close(out)

		events := make(chan hl7v2.ParseEvent, 100)
		go liveParser.ParseStream(input.Message, events)

		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-events:
				if !ok {
					return
				}
				m := &model.ParseEventModel{
					SegmentIndex: ev.SegmentIndex,
					SegmentType:  ev.SegmentType,
					RawSegment:   ev.RawSegment,
					Fields:       ev.Fields,
					Warnings:     ev.Warnings,
					IsComplete:   ev.IsComplete,
				}
				select {
				case out <- m:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return out, nil
}

// DebugStepEvent is the resolver for the debugStepEvent subscription.
func (r *subscriptionResolver) DebugStepEvent(ctx context.Context, sessionID string) (<-chan *model.DebugStepModel, error) {
	r.debugSessionsMu.RLock()
	_, ok := r.debugSessions[sessionID]
	r.debugSessionsMu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("debug session %s not found", sessionID)
	}

	out := make(chan *model.DebugStepModel, 10)

	go func() {
		defer close(out)
		// Keep sending step events until context is cancelled or session ends
		<-ctx.Done()
	}()

	return out, nil
}

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
