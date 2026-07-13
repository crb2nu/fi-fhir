package resolvers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/graphql/model"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/requestsecurity"
	integrationpreview "gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/preview"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

func (r *mutationResolver) previewIntegrationMessage(ctx context.Context, input model.PreviewIntegrationMessageInput) (*model.IntegrationPreviewResult, error) {
	security, authenticated := requestsecurity.SecurityContextFromContext(ctx)
	if !authenticated {
		return nil, errors.New("authentication required")
	}
	if r.IntegrationPreview == nil {
		return nil, errors.New("integration preview unavailable")
	}
	result, err := r.IntegrationPreview.Preview(ctx, security, integrationpreview.Input{
		IntegrationID: input.IntegrationID,
		Payload:       []byte(input.Data),
		CorrelationID: input.CorrelationID,
		Reason:        input.Reason,
	})
	if err != nil {
		return nil, catalogPreviewError(err)
	}
	return projectIntegrationPreview(result)
}

func catalogPreviewError(err error) error {
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return err
	case errors.Is(err, integrationpreview.ErrUnauthenticated):
		return errors.New("authentication required")
	case errors.Is(err, integrationpreview.ErrForbidden):
		return errors.New("integration preview forbidden")
	case errors.Is(err, integrationpreview.ErrInvalidInput):
		return errors.New("invalid integration preview request")
	case errors.Is(err, integrationpreview.ErrPayloadTooLarge):
		return errors.New("integration preview payload too large")
	case errors.Is(err, integrationpreview.ErrIntegrationUnavailable), errors.Is(err, integrationpreview.ErrUnavailable):
		return errors.New("integration preview unavailable")
	default:
		return errors.New("integration preview failed")
	}
}

func projectIntegrationPreview(result integration.ProcessResult) (*model.IntegrationPreviewResult, error) {
	if result.ArtifactRevisions == nil {
		return nil, fmt.Errorf("integration preview result is missing artifact provenance")
	}
	events := make([]model.IntegrationPreviewEvent, 0, len(result.Events))
	for _, event := range result.Events {
		payload := make(map[string]any)
		if err := json.Unmarshal(event.PayloadJSON(), &payload); err != nil {
			return nil, fmt.Errorf("project integration preview event: %w", err)
		}
		events = append(events, model.IntegrationPreviewEvent{
			TenantID:        event.TenantID,
			ID:              event.ID,
			Type:            string(event.Type),
			SourceMessageID: optionalPreviewString(event.SourceMessageID),
			CorrelationID:   event.CorrelationID,
			Classification:  string(event.Classification),
			Payload:         payload,
		})
	}
	diagnostics := make([]model.IntegrationPreviewDiagnostic, 0, len(result.Diagnostics))
	for _, diagnostic := range result.Diagnostics {
		diagnostics = append(diagnostics, model.IntegrationPreviewDiagnostic{
			TenantID:       diagnostic.TenantID,
			Severity:       string(diagnostic.Severity),
			Stage:          diagnostic.Stage,
			Code:           diagnostic.Code,
			Message:        diagnostic.Message(),
			Path:           optionalPreviewString(diagnostic.Path),
			Source:         optionalPreviewString(diagnostic.Source()),
			Classification: string(diagnostic.Classification),
		})
	}
	routes := make([]model.IntegrationPreviewRoute, 0, len(result.Routes))
	for _, route := range result.Routes {
		routes = append(routes, model.IntegrationPreviewRoute{
			TenantID:        route.TenantID,
			EventID:         route.EventID,
			Route:           route.Route,
			Matched:         route.Matched,
			Skipped:         route.Skipped,
			SkipReason:      optionalPreviewString(route.SkipReason),
			TransformCount:  route.TransformCount,
			PlannedActions:  append([]string(nil), route.PlannedActions...),
			DiagnosticCodes: append([]string(nil), route.DiagnosticCodes...),
		})
	}
	deliveries := make([]model.IntegrationPreviewDelivery, 0, len(result.Deliveries))
	for _, delivery := range result.Deliveries {
		deliveries = append(deliveries, model.IntegrationPreviewDelivery{
			TenantID: delivery.TenantID,
			EventID:  delivery.EventID,
			Destination: model.IntegrationPreviewDestination{
				ArtifactID: delivery.Destination.ArtifactID,
				RevisionID: delivery.Destination.RevisionID,
				Digest:     delivery.Destination.Digest,
				Class:      string(delivery.Destination.Class),
			},
			Route:           delivery.Route,
			Action:          delivery.Action,
			Status:          string(delivery.Status),
			DiagnosticCodes: append([]string(nil), delivery.DiagnosticCodes...),
		})
	}
	return &model.IntegrationPreviewResult{
		Mode:                string(result.Mode),
		TenantID:            result.TenantID,
		IntegrationRevision: projectArtifactRevision(result.IntegrationRevision),
		ArtifactRevisions: model.IntegrationExecutionArtifactRevisions{
			Source:   projectArtifactRevision(result.ArtifactRevisions.Source),
			Profile:  projectArtifactRevision(result.ArtifactRevisions.Profile),
			Workflow: projectArtifactRevision(result.ArtifactRevisions.Workflow),
		},
		Events:      events,
		Diagnostics: diagnostics,
		Routes:      routes,
		Deliveries:  deliveries,
		Correlations: model.IntegrationPreviewCorrelations{
			TenantID:        result.Correlations.TenantID,
			CorrelationID:   result.Correlations.CorrelationID,
			TraceID:         optionalPreviewString(result.Correlations.TraceID),
			SourceMessageID: optionalPreviewString(result.Correlations.SourceMessageID),
			EventIDs:        append([]string(nil), result.Correlations.EventIDs...),
			WorkflowRunID:   optionalPreviewString(result.Correlations.WorkflowRunID),
		},
	}, nil
}

func projectArtifactRevision(reference integration.ArtifactRevisionRef) model.IntegrationArtifactRevision {
	return model.IntegrationArtifactRevision{
		ArtifactID: reference.ArtifactID,
		RevisionID: reference.RevisionID,
		Digest:     reference.Digest,
	}
}

func optionalPreviewString(value string) *string {
	if value == "" {
		return nil
	}
	valueCopy := value
	return &valueCopy
}
