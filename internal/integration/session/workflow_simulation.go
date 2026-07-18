package session

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/workflow"
)

const (
	maxWorkflowSimulationRuns   = 64
	maxWorkflowSimulationEvents = 4096
	maxWorkflowTraceValueBytes  = 256
)

// WorkflowSimulator binds durable session events to one exact workflow draft
// revision and records only the production planner's configuration-free trace.
type WorkflowSimulator struct {
	store Store
}

func NewWorkflowSimulator(store Store) *WorkflowSimulator {
	return &WorkflowSimulator{store: store}
}

func (s *WorkflowSimulator) Simulate(ctx context.Context, req SimulateWorkflowRequest) (*WorkflowSimulation, error) {
	if s == nil || s.store == nil || ctx == nil || ctx.Err() != nil {
		return nil, fmt.Errorf("%w: workflow simulator is not configured", ErrInvalid)
	}
	if req.SessionID == "" || req.WorkflowRevisionID == "" || len(req.SourceRunIDs) == 0 || len(req.SourceRunIDs) > maxWorkflowSimulationRuns {
		return nil, fmt.Errorf("%w: session, workflow revision, and source runs are required", ErrInvalid)
	}
	revision, err := s.store.GetArtifactRevision(ctx, req.SessionID, req.WorkflowRevisionID)
	if err != nil {
		return nil, err
	}
	if revision.Kind != ArtifactKindWorkflowDraft || revision.RevisionID != req.WorkflowRevisionID {
		return nil, fmt.Errorf("%w: workflow revision is not a session workflow draft", ErrInvalid)
	}
	definition, err := workflow.ParseDraftWorkflow(revision.Content)
	if err != nil {
		return nil, fmt.Errorf("%w: workflow revision is not simulatable", ErrInvalid)
	}
	planner, err := workflow.NewPlanner(definition)
	if err != nil {
		return nil, fmt.Errorf("%w: workflow revision cannot be planned", ErrInvalid)
	}

	seenRuns := make(map[string]struct{}, len(req.SourceRunIDs))
	traces := make([]WorkflowEventTrace, 0)
	for _, runID := range req.SourceRunIDs {
		if runID == "" {
			return nil, fmt.Errorf("%w: source run ID is required", ErrInvalid)
		}
		if _, duplicate := seenRuns[runID]; duplicate {
			return nil, fmt.Errorf("%w: source run IDs must be unique", ErrInvalid)
		}
		seenRuns[runID] = struct{}{}
		run, err := s.store.GetRun(ctx, req.SessionID, runID)
		if err != nil {
			return nil, err
		}
		if run.Status != RunStatusSucceeded || len(run.Events) == 0 {
			return nil, fmt.Errorf("%w: source runs must be successful and contain events", ErrInvalid)
		}
		for _, event := range run.Events {
			if len(traces) >= maxWorkflowSimulationEvents {
				return nil, fmt.Errorf("%w: workflow simulation event limit exceeded", ErrInvalid)
			}
			payload, err := workflowEventPayload(event)
			if err != nil {
				return nil, fmt.Errorf("%w: source run event is invalid", ErrImmutable)
			}
			plan, err := planner.Plan(payload)
			if err != nil {
				return nil, fmt.Errorf("%w: workflow planning failed", ErrInvalid)
			}
			trace := WorkflowEventTrace{RunID: run.ID, EventID: event.ID, EventType: event.Type, Routes: make([]WorkflowRouteTrace, len(plan.Routes))}
			for routeIndex, route := range plan.Routes {
				routeTrace := WorkflowRouteTrace{
					Name: route.Name, Matched: route.Matched, SkipReason: route.SkipReason,
					DiagnosticCodes: cloneStrings(route.DiagnosticCodes),
					Transforms:      make([]WorkflowTransformTrace, len(route.Transforms)),
					Actions:         make([]WorkflowActionTrace, len(route.Actions)),
				}
				for transformIndex, transform := range route.Transforms {
					routeTrace.Transforms[transformIndex] = WorkflowTransformTrace{Index: transform.Index, Type: transform.Type, Status: "planned"}
				}
				for actionIndex, action := range route.Actions {
					routeTrace.Actions[actionIndex] = WorkflowActionTrace{ID: action.ID, Type: action.Type, DestinationArtifactID: action.DestinationArtifactID}
				}
				trace.Routes[routeIndex] = routeTrace
			}
			traces = append(traces, trace)
		}
	}

	return s.store.CreateWorkflowSimulation(ctx, req.SessionID, CreateWorkflowSimulationRequest{
		WorkflowArtifactID: revision.ID, WorkflowRevisionID: revision.RevisionID,
		WorkflowRevisionDigest: revision.Digest, SourceRunIDs: req.SourceRunIDs, Events: traces,
	})
}

func workflowEventPayload(event ParsedEvent) (map[string]any, error) {
	if event.ID == "" || event.Type == "" || len(event.Payload) == 0 {
		return nil, ErrImmutable
	}
	var payload map[string]any
	if err := json.Unmarshal(event.Payload, &payload); err != nil || payload == nil {
		return nil, ErrImmutable
	}
	if payloadType, ok := payload["type"].(string); !ok || payloadType != event.Type {
		return nil, ErrImmutable
	}
	if payloadID, ok := payload["id"].(string); ok && payloadID != event.ID {
		return nil, ErrImmutable
	}
	return payload, nil
}

func validateWorkflowSimulationReferences(ctx context.Context, store Store, sessionID string, req CreateWorkflowSimulationRequest) error {
	if store == nil || sessionID == "" || len(req.SourceRunIDs) == 0 || len(req.SourceRunIDs) > maxWorkflowSimulationRuns || len(req.Events) == 0 || len(req.Events) > maxWorkflowSimulationEvents {
		return fmt.Errorf("%w: workflow simulation record is incomplete", ErrInvalid)
	}
	revision, err := store.GetArtifactRevision(ctx, sessionID, req.WorkflowRevisionID)
	if err != nil {
		return err
	}
	if revision.Kind != ArtifactKindWorkflowDraft || revision.ID != req.WorkflowArtifactID || revision.RevisionID != req.WorkflowRevisionID || revision.Digest != req.WorkflowRevisionDigest {
		return ErrImmutable
	}

	expectedEvents := make(map[string]string)
	seenRuns := make(map[string]struct{}, len(req.SourceRunIDs))
	for _, runID := range req.SourceRunIDs {
		if _, duplicate := seenRuns[runID]; duplicate {
			return fmt.Errorf("%w: source run IDs must be unique", ErrInvalid)
		}
		seenRuns[runID] = struct{}{}
		run, err := store.GetRun(ctx, sessionID, runID)
		if err != nil {
			return err
		}
		if run.Status != RunStatusSucceeded || len(run.Events) == 0 {
			return fmt.Errorf("%w: source runs must be successful", ErrInvalid)
		}
		for _, event := range run.Events {
			if event.ID == "" || event.Type == "" {
				return ErrImmutable
			}
			key := workflowEventKey(runID, event.ID)
			if _, duplicate := expectedEvents[key]; duplicate {
				return ErrImmutable
			}
			expectedEvents[key] = event.Type
		}
	}
	if len(expectedEvents) != len(req.Events) {
		return ErrImmutable
	}
	seenEvents := make(map[string]struct{}, len(req.Events))
	for _, event := range req.Events {
		key := workflowEventKey(event.RunID, event.EventID)
		if expectedEvents[key] != event.EventType || event.EventType == "" {
			return ErrImmutable
		}
		if _, duplicate := seenEvents[key]; duplicate {
			return ErrImmutable
		}
		seenEvents[key] = struct{}{}
		if len(event.Routes) == 0 || len(event.Routes) > 256 {
			return fmt.Errorf("%w: workflow trace route count is invalid", ErrInvalid)
		}
		for _, route := range event.Routes {
			if !validWorkflowTraceValue(route.Name) || !optionalWorkflowTraceValue(route.SkipReason) || len(route.DiagnosticCodes) > 128 || len(route.Transforms) > 128 || len(route.Actions) > 128 {
				return fmt.Errorf("%w: workflow route trace is invalid", ErrInvalid)
			}
			for _, code := range route.DiagnosticCodes {
				if !validWorkflowTraceValue(code) {
					return ErrImmutable
				}
			}
			if !route.Matched && (len(route.Transforms) != 0 || len(route.Actions) != 0) {
				return ErrImmutable
			}
			for _, transform := range route.Transforms {
				if transform.Index < 0 || !validWorkflowTraceValue(transform.Type) || transform.Status != "planned" {
					return ErrImmutable
				}
			}
			for _, action := range route.Actions {
				if !validWorkflowTraceValue(action.ID) || !validWorkflowTraceValue(action.Type) || !optionalWorkflowTraceValue(action.DestinationArtifactID) {
					return ErrImmutable
				}
			}
		}
	}
	return nil
}

func validWorkflowTraceValue(value string) bool {
	return value != "" && len(value) <= maxWorkflowTraceValueBytes && strings.TrimSpace(value) == value && !strings.ContainsAny(value, "\x00\r\n\t")
}

func optionalWorkflowTraceValue(value string) bool {
	return value == "" || validWorkflowTraceValue(value)
}

func CompareWorkflowSimulations(baseline, candidate WorkflowSimulation) (WorkflowSimulationDelta, error) {
	if baseline.ID == "" || candidate.ID == "" || baseline.SessionID == "" || baseline.SessionID != candidate.SessionID {
		return WorkflowSimulationDelta{}, fmt.Errorf("%w: simulations must belong to the same session", ErrInvalid)
	}
	base := workflowSimulationKeySets(baseline)
	next := workflowSimulationKeySets(candidate)
	return WorkflowSimulationDelta{
		BaselineSimulationID: baseline.ID, CandidateSimulationID: candidate.ID,
		AddedEvents: setDifference(next.events, base.events), RemovedEvents: setDifference(base.events, next.events),
		AddedMatchedRoutes: setDifference(next.routes, base.routes), RemovedMatchedRoutes: setDifference(base.routes, next.routes),
		AddedTransforms: setDifference(next.transforms, base.transforms), RemovedTransforms: setDifference(base.transforms, next.transforms),
		AddedActions: setDifference(next.actions, base.actions), RemovedActions: setDifference(base.actions, next.actions),
	}, nil
}

type workflowSimulationSets struct {
	events, routes, transforms, actions map[string]struct{}
}

func workflowSimulationKeySets(simulation WorkflowSimulation) workflowSimulationSets {
	sets := workflowSimulationSets{events: map[string]struct{}{}, routes: map[string]struct{}{}, transforms: map[string]struct{}{}, actions: map[string]struct{}{}}
	for _, event := range simulation.Events {
		eventKey := workflowEventKey(event.RunID, event.EventID) + ":" + event.EventType
		sets.events[eventKey] = struct{}{}
		for _, route := range event.Routes {
			if !route.Matched {
				continue
			}
			routeKey := eventKey + "/route:" + route.Name
			sets.routes[routeKey] = struct{}{}
			for _, transform := range route.Transforms {
				sets.transforms[fmt.Sprintf("%s/transform:%d:%s", routeKey, transform.Index, transform.Type)] = struct{}{}
			}
			for _, action := range route.Actions {
				sets.actions[routeKey+"/action:"+action.ID+":"+action.Type+":"+action.DestinationArtifactID] = struct{}{}
			}
		}
	}
	return sets
}

func workflowEventKey(runID, eventID string) string {
	return "run:" + runID + "/event:" + eventID
}

func setDifference(left, right map[string]struct{}) []string {
	out := make([]string, 0)
	for value := range left {
		if _, found := right[value]; !found {
			out = append(out, value)
		}
	}
	sort.Strings(out)
	return out
}
