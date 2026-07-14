package processor

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/workflow"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

var (
	// ErrInvalidPublishedWorkflow means immutable workflow bytes are not executable DSL v1.
	ErrInvalidPublishedWorkflow = errors.New("invalid published workflow")
	// ErrInvalidWorkflowPlan means a pure plan cannot bind safely to the integration revision.
	ErrInvalidWorkflowPlan = errors.New("invalid workflow plan")
)

func planWorkflow(
	resolved ResolvedArtifactRevisions,
	event integration.ProcessedEvent,
	revision integration.IntegrationDefinitionRevision,
	mode integration.ExecutionMode,
) ([]integration.RouteResult, []integration.DeliveryResult, []integration.Diagnostic, error) {
	if event.TenantID != revision.TenantID || event.Classification != revision.Policy.Classification || event.ID == "" {
		return nil, nil, nil, ErrInvalidWorkflowPlan
	}
	if mode != integration.ExecutionModePreview && mode != integration.ExecutionModeProduction {
		return nil, nil, nil, ErrInvalidWorkflowPlan
	}
	workflowRef := resolved.WorkflowReference()
	if workflowRef != revision.Workflow {
		return nil, nil, nil, ErrInvalidWorkflowPlan
	}
	rawYAML := resolved.WorkflowYAML()
	computedRef, err := newWorkflowRevisionReference(workflowRef.ArtifactID, workflowRef.RevisionID, rawYAML)
	if err != nil || computedRef != workflowRef {
		return nil, nil, nil, ErrInvalidPublishedWorkflow
	}
	published, err := workflow.ParsePublishedWorkflow(rawYAML)
	if err != nil {
		return nil, nil, nil, ErrInvalidPublishedWorkflow
	}
	planner, err := workflow.NewPlanner(published.Workflow())
	if err != nil {
		return nil, nil, nil, ErrInvalidPublishedWorkflow
	}

	workflowEvent, err := processedEventWorkflowInput(event)
	if err != nil {
		return nil, nil, nil, ErrInvalidWorkflowPlan
	}
	plan, err := planner.Plan(workflowEvent)
	if err != nil {
		return nil, nil, nil, ErrInvalidWorkflowPlan
	}

	destinations := make(map[string]integration.DestinationRevisionRef, len(revision.Destinations))
	for _, destination := range revision.Destinations {
		if _, duplicate := destinations[destination.ArtifactID]; duplicate {
			return nil, nil, nil, ErrInvalidWorkflowPlan
		}
		destinations[destination.ArtifactID] = destination
	}

	diagnostics := make([]integration.Diagnostic, 0, len(plan.Diagnostics))
	for _, plannedDiagnostic := range plan.Diagnostics {
		diagnostic, err := integration.NewDiagnostic(integration.DiagnosticInput{
			TenantID:       revision.TenantID,
			Severity:       integration.DiagnosticSeverityError,
			Stage:          "workflow",
			Code:           plannedDiagnostic.Code,
			Path:           plannedDiagnostic.Path,
			Source:         "planner",
			Classification: revision.Policy.Classification,
		})
		if err != nil {
			return nil, nil, nil, ErrInvalidWorkflowPlan
		}
		diagnostics = append(diagnostics, diagnostic)
	}

	routes := make([]integration.RouteResult, 0, len(plan.Routes))
	deliveries := make([]integration.DeliveryResult, 0)
	for _, plannedRoute := range plan.Routes {
		route := integration.RouteResult{
			TenantID:        revision.TenantID,
			EventID:         event.ID,
			Route:           plannedRoute.Name,
			Matched:         plannedRoute.Matched,
			Skipped:         plannedRoute.Skipped,
			SkipReason:      plannedRoute.SkipReason,
			TransformCount:  plannedRoute.TransformCount,
			DiagnosticCodes: append([]string(nil), plannedRoute.DiagnosticCodes...),
		}
		for _, action := range plannedRoute.Actions {
			route.PlannedActions = append(route.PlannedActions, action.ID)
			if action.Type == "log" {
				if action.DestinationArtifactID != "" {
					return nil, nil, nil, ErrInvalidWorkflowPlan
				}
				continue
			}
			if !deliveryActionTypeV1(action.Type) || action.DestinationArtifactID == "" {
				return nil, nil, nil, ErrInvalidWorkflowPlan
			}
			destination, found := destinations[action.DestinationArtifactID]
			if !found {
				return nil, nil, nil, ErrInvalidWorkflowPlan
			}
			deliveries = append(deliveries, integration.DeliveryResult{
				TenantID:    revision.TenantID,
				EventID:     event.ID,
				Destination: destination,
				Route:       plannedRoute.Name,
				Action:      action.ID,
				Status:      deliveryPlanStatus(mode),
			})
		}
		routes = append(routes, route)
	}
	return routes, deliveries, diagnostics, nil
}

func deliveryPlanStatus(mode integration.ExecutionMode) integration.DeliveryStatus {
	if mode == integration.ExecutionModeProduction {
		return integration.DeliveryStatusPlanned
	}
	return integration.DeliveryStatusSuppressed
}

func processedEventWorkflowInput(event integration.ProcessedEvent) (map[string]any, error) {
	raw := event.PayloadJSON()
	decoder := json.NewDecoder(bytes.NewReader(raw))
	var value map[string]any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, errors.New("processed event contains trailing JSON")
	}
	return value, nil
}

func deliveryActionTypeV1(actionType string) bool {
	switch actionType {
	case "webhook", "fhir", "email", "exec", "file", "database", "queue", "event_store", "athena":
		return true
	default:
		return false
	}
}
