package resolvers

import (
	"context"
	"errors"
	"strings"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/graphql/model"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/connection"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/lifecycle"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/lifecycle/authoring"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

// The definition editor's resolvers (.loom/42 E-1). They follow
// connection_catalog.go: a fail-closed service accessor, an inventory-safe
// error catalog, and projections that add nothing to what the service
// returns. Every role check happens twice — at the transport gate
// (rootFieldRoles) and again inside authoring.Service.

// ErrDefinitionAuthoringUnavailable keeps authoring closed until serve wires
// it; /api/auth/status reports the deployment fact (definitionAuthoring).
var ErrDefinitionAuthoringUnavailable = errors.New("integration definition authoring unavailable")

// WithDefinitionAuthoring enables the definition editor. A nil service leaves
// every field fail-closed.
func WithDefinitionAuthoring(service *authoring.Service) ResolverOption {
	return func(r *Resolver) {
		r.DefinitionAuthoring = service
	}
}

func (r *Resolver) definitionAuthoring() (*authoring.Service, error) {
	if r == nil || r.DefinitionAuthoring == nil {
		return nil, ErrDefinitionAuthoringUnavailable
	}
	return r.DefinitionAuthoring, nil
}

// definitionAuthoringError maps authoring and lifecycle failures onto stable,
// inventory-safe messages (server.go catalogSafeErrorPresenter admits exactly
// these). Another tenant's definition is "not found", never "forbidden".
func definitionAuthoringError(err error) error {
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return err
	case errors.Is(err, authoring.ErrUnauthenticated), errors.Is(err, connection.ErrUnauthenticated):
		return errors.New("authentication required")
	case errors.Is(err, authoring.ErrForbidden), errors.Is(err, connection.ErrForbidden):
		return errors.New("integration definition authoring forbidden")
	case errors.Is(err, authoring.ErrInvalidRequest), errors.Is(err, lifecycle.ErrInvalidCommand),
		errors.Is(err, connection.ErrInvalidRequest):
		return errors.New("invalid integration definition request")
	case errors.Is(err, lifecycle.ErrNotFound):
		return errors.New("integration definition not found")
	case errors.Is(err, lifecycle.ErrVersionConflict):
		return errors.New("integration definition version conflict")
	case errors.Is(err, lifecycle.ErrInvalidTransition):
		return errors.New("invalid integration definition transition")
	case errors.Is(err, lifecycle.ErrConnectionValidationRequired):
		return errors.New("current connection validation required")
	case errors.Is(err, lifecycle.ErrActiveDeployment):
		return errors.New("another revision of this definition is deployed or paused")
	case errors.Is(err, authoring.ErrRealUnavailable):
		return errors.New("real validation is unavailable for this source on this replica")
	case errors.Is(err, authoring.ErrRealBusy):
		return errors.New("a real validation is already running on this replica")
	case errors.Is(err, authoring.ErrUnavailable), errors.Is(err, authoring.ErrRegistryUnavailable),
		errors.Is(err, lifecycle.ErrUnavailable), errors.Is(err, connection.ErrUnavailable),
		errors.Is(err, ErrDefinitionAuthoringUnavailable):
		return ErrDefinitionAuthoringUnavailable
	default:
		return errors.New("integration definition request failed")
	}
}

func (r *queryResolver) integrationDefinitions(ctx context.Context, includeRetired *bool) ([]model.IntegrationDefinition, error) {
	service, err := r.definitionAuthoring()
	if err != nil {
		return nil, definitionAuthoringError(err)
	}
	rows, err := service.List(ctx, includeRetired != nil && *includeRetired)
	if err != nil {
		return nil, definitionAuthoringError(err)
	}
	projected := make([]model.IntegrationDefinition, 0, len(rows))
	for _, row := range rows {
		projected = append(projected, projectIntegrationDefinition(row.Snapshot, row.Revision))
	}
	return projected, nil
}

func (r *queryResolver) integrationDefinition(ctx context.Context, definitionID, revisionID string) (*model.IntegrationDefinitionDetail, error) {
	service, err := r.definitionAuthoring()
	if err != nil {
		return nil, definitionAuthoringError(err)
	}
	definition, err := service.Get(ctx, definitionID, revisionID)
	if errors.Is(err, lifecycle.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, definitionAuthoringError(err)
	}
	return projectIntegrationDefinitionDetail(service, definition), nil
}

func (r *queryResolver) integrationRegistryArtifacts(ctx context.Context) ([]model.IntegrationRegistryArtifact, error) {
	service, err := r.definitionAuthoring()
	if err != nil {
		return nil, definitionAuthoringError(err)
	}
	artifacts, err := service.RegistryArtifacts(ctx)
	if err != nil {
		return nil, definitionAuthoringError(err)
	}
	projected := make([]model.IntegrationRegistryArtifact, 0, len(artifacts))
	for _, artifact := range artifacts {
		projected = append(projected, model.IntegrationRegistryArtifact{
			IntegrationID: artifact.IntegrationID,
			Profile:       artifactRevisionPointer(artifact.Profile),
			Workflow:      artifactRevisionPointer(artifact.Workflow),
			SourceID:      artifact.SourceID,
			Format:        string(artifact.Format),
		})
	}
	return projected, nil
}

func (r *mutationResolver) validateIntegrationDefinitionDraft(ctx context.Context, input model.IntegrationDefinitionDraftInput) ([]model.ConnectionProblem, error) {
	service, err := r.definitionAuthoring()
	if err != nil {
		return nil, definitionAuthoringError(err)
	}
	problems, err := service.Check(ctx, draftInputFromModel(input))
	if err != nil {
		return nil, definitionAuthoringError(err)
	}
	return projectConnectionProblems(problems), nil
}

func (r *mutationResolver) createIntegrationDefinitionDraft(
	ctx context.Context,
	input model.IntegrationDefinitionDraftInput,
	reason string,
) (*model.IntegrationDefinitionDraftResult, error) {
	service, err := r.definitionAuthoring()
	if err != nil {
		return nil, definitionAuthoringError(err)
	}
	result, err := service.CreateDraft(ctx, draftInputFromModel(input), reason)
	if err != nil {
		return nil, definitionAuthoringError(err)
	}
	projected := &model.IntegrationDefinitionDraftResult{Problems: projectConnectionProblems(result.Problems)}
	if result.Definition != nil {
		projected.Definition = projectIntegrationDefinitionDetail(service, *result.Definition)
	}
	return projected, nil
}

func (r *mutationResolver) validateIntegrationDefinition(ctx context.Context, input model.IntegrationDefinitionValidateInput) (*model.IntegrationDefinitionDetail, error) {
	service, err := r.definitionAuthoring()
	if err != nil {
		return nil, definitionAuthoringError(err)
	}
	var mode authoring.Mode
	switch input.Mode {
	case model.IntegrationValidationModeReal:
		mode = authoring.ModeReal
	case model.IntegrationValidationModeStatic:
		mode = authoring.ModeStatic
	case model.IntegrationValidationModeSkip:
		mode = authoring.ModeSkip
	default:
		return nil, definitionAuthoringError(authoring.ErrInvalidRequest)
	}
	definition, err := service.Validate(ctx, authoring.Command{
		DefinitionID: input.DefinitionID, RevisionID: input.RevisionID,
		ExpectedVersion: int64(input.ExpectedVersion), Reason: input.Reason,
	}, mode)
	if err != nil {
		return nil, definitionAuthoringError(err)
	}
	return projectIntegrationDefinitionDetail(service, definition), nil
}

func (r *mutationResolver) approveIntegrationDefinition(ctx context.Context, input model.IntegrationDefinitionCommandInput) (*model.IntegrationDefinitionDetail, error) {
	return r.definitionTransition(ctx, input, (*authoring.Service).Approve)
}

func (r *mutationResolver) publishIntegrationDefinition(ctx context.Context, input model.IntegrationDefinitionCommandInput) (*model.IntegrationDefinitionDetail, error) {
	return r.definitionTransition(ctx, input, (*authoring.Service).Publish)
}

func (r *mutationResolver) definitionTransition(
	ctx context.Context,
	input model.IntegrationDefinitionCommandInput,
	apply func(*authoring.Service, context.Context, authoring.Command) (authoring.Definition, error),
) (*model.IntegrationDefinitionDetail, error) {
	service, err := r.definitionAuthoring()
	if err != nil {
		return nil, definitionAuthoringError(err)
	}
	definition, err := apply(service, ctx, authoring.Command{
		DefinitionID: input.DefinitionID, RevisionID: input.RevisionID,
		ExpectedVersion: int64(input.ExpectedVersion), Reason: input.Reason,
	})
	if err != nil {
		return nil, definitionAuthoringError(err)
	}
	return projectIntegrationDefinitionDetail(service, definition), nil
}

func draftInputFromModel(input model.IntegrationDefinitionDraftInput) authoring.DraftInput {
	draft := authoring.DraftInput{
		DefinitionID:     input.DefinitionID,
		RevisionID:       input.RevisionID,
		ParentRevisionID: optionalStringValue(input.ParentRevisionID),
		Profile:          artifactRevisionFromInput(input.Profile),
		Workflow:         artifactRevisionFromInput(input.Workflow),
		SecretBindings:   secretBindingsFromInput(input.SecretBindings),
	}
	if input.Source != nil {
		draft.Source = authoring.RevisionRef{ArtifactID: input.Source.ArtifactID, RevisionID: input.Source.RevisionID}
	}
	for _, destination := range input.Destinations {
		draft.Destinations = append(draft.Destinations, authoring.RevisionRef{ArtifactID: destination.ArtifactID, RevisionID: destination.RevisionID})
	}
	if input.RawRetention != nil {
		retention := integration.RawRetentionPolicy{
			Mode:    integration.RawRetentionMode(strings.ToLower(input.RawRetention.Mode)),
			Purpose: optionalStringValue(input.RawRetention.Purpose),
		}
		if input.RawRetention.TTLSeconds != nil {
			retention.TTLSeconds = int64(*input.RawRetention.TTLSeconds)
		}
		if input.RawRetention.StorageRevision != nil {
			storage := artifactRevisionFromInput(input.RawRetention.StorageRevision)
			retention.StorageRevision = &storage
		}
		if input.RawRetention.EncryptionKey != nil {
			retention.EncryptionKey = &integration.SecretReference{
				Provider: integration.SecretProviderKind(input.RawRetention.EncryptionKey.Provider),
				Key:      input.RawRetention.EncryptionKey.Key,
				Version:  optionalStringValue(input.RawRetention.EncryptionKey.Version),
			}
		}
		draft.RawRetention = &retention
	}
	if input.Deployment != nil {
		policy := integration.IntegrationDeploymentPolicy{
			ConnectionValidation: integration.ConnectionValidationPolicy{
				TimeoutSeconds: int64(input.Deployment.ValidationTimeoutSeconds),
				MaxAgeSeconds:  int64(input.Deployment.ValidationMaxAgeSeconds),
			},
			Schedule: integration.SchedulePolicy{
				Mode:           integration.ScheduleMode(strings.ToLower(input.Deployment.ScheduleMode)),
				CronExpression: optionalStringValue(input.Deployment.CronExpression),
				Timezone:       optionalStringValue(input.Deployment.Timezone),
			},
			Health: integration.HealthPolicy{
				StartupGraceSeconds:  int64(input.Deployment.HealthStartupGraceSeconds),
				CheckIntervalSeconds: int64(input.Deployment.HealthCheckIntervalSeconds),
				TimeoutSeconds:       int64(input.Deployment.HealthTimeoutSeconds),
				FailureThreshold:     input.Deployment.HealthFailureThreshold,
			},
			Capacity: integration.CapacityPolicy{
				MaxInFlight:          input.Deployment.MaxInFlight,
				MaxQueued:            input.Deployment.MaxQueued,
				MaxMessagesPerSecond: input.Deployment.MaxMessagesPerSecond,
			},
		}
		draft.Deployment = &policy
	}
	return draft
}

func artifactRevisionFromInput(input *model.IntegrationArtifactRevisionInput) integration.ArtifactRevisionRef {
	if input == nil {
		return integration.ArtifactRevisionRef{}
	}
	return integration.ArtifactRevisionRef{ArtifactID: input.ArtifactID, RevisionID: input.RevisionID, Digest: input.Digest}
}

func artifactRevisionPointer(ref integration.ArtifactRevisionRef) *model.IntegrationArtifactRevision {
	projected := projectArtifactRevision(ref)
	return &projected
}

func projectIntegrationDefinition(snapshot lifecycle.Snapshot, revision integration.IntegrationDefinitionRevision) model.IntegrationDefinition {
	destinations := make([]model.IntegrationDefinitionDestination, 0, len(revision.Destinations))
	for _, destination := range revision.Destinations {
		destinations = append(destinations, model.IntegrationDefinitionDestination{
			ArtifactID: destination.ArtifactID, RevisionID: destination.RevisionID,
			Digest: destination.Digest, Class: string(destination.Class),
		})
	}
	bindings := make([]model.ConnectionSecretBinding, 0, len(revision.SecretBindings))
	for _, binding := range revision.SecretBindings {
		bindings = append(bindings, model.ConnectionSecretBinding{
			Name: binding.Name, Provider: string(binding.Reference.Provider),
			Key: binding.Reference.Key, Version: optionalPreviewString(binding.Reference.Version),
		})
	}
	retention := revision.Policy.RawRetention
	projectedRetention := &model.IntegrationRawRetention{
		Mode:                string(retention.EffectiveMode()),
		Purpose:             optionalPreviewString(retention.Purpose),
		AccessAuditRequired: retention.AccessAuditRequired,
	}
	if retention.TTLSeconds > 0 {
		ttl := int(retention.TTLSeconds)
		projectedRetention.TTLSeconds = &ttl
	}
	if retention.StorageRevision != nil {
		projectedRetention.StorageRevision = artifactRevisionPointer(*retention.StorageRevision)
	}
	if retention.EncryptionKey != nil {
		projectedRetention.EncryptionKey = &model.IntegrationSecretReference{
			Provider: string(retention.EncryptionKey.Provider), Key: retention.EncryptionKey.Key,
			Version: optionalPreviewString(retention.EncryptionKey.Version),
		}
	}
	projected := model.IntegrationDefinition{
		DefinitionID:     revision.DefinitionID,
		RevisionID:       revision.RevisionID,
		Digest:           revision.Digest,
		ParentRevisionID: optionalPreviewString(revision.ParentRevisionID),
		State:            string(snapshot.State),
		Version:          int(snapshot.Version),
		Health:           string(snapshot.Health),
		ReleaseID:        optionalPreviewString(snapshot.ReleaseID),
		ValidationPassed: snapshot.ValidationPassed,
		Source: &model.IntegrationDefinitionSource{
			ArtifactID: revision.Source.ArtifactID, RevisionID: revision.Source.RevisionID,
			Digest: revision.Source.Digest, SourceID: revision.Source.SourceID,
		},
		Profile:        artifactRevisionPointer(revision.Profile),
		Workflow:       artifactRevisionPointer(revision.Workflow),
		Destinations:   destinations,
		SecretBindings: bindings,
		Policy: &model.IntegrationDefinitionPolicy{
			Classification: string(revision.Policy.Classification),
			RawRetention:   projectedRetention,
		},
		CreatedBy:     projectAuditPrincipal(revision.Created.Principal),
		CreatedReason: revision.Created.Reason,
		CreatedAt:     revision.Created.OccurredAt.UTC(),
		UpdatedBy:     projectAuditPrincipal(snapshot.Updated.Principal),
		UpdatedReason: snapshot.Updated.Reason,
		UpdatedAt:     snapshot.Updated.OccurredAt.UTC(),
	}
	if !snapshot.ValidationCheckedAt.IsZero() {
		checked := snapshot.ValidationCheckedAt.UTC()
		projected.ValidationCheckedAt = &checked
	}
	if !snapshot.ValidationExpiresAt.IsZero() {
		expires := snapshot.ValidationExpiresAt.UTC()
		projected.ValidationExpiresAt = &expires
	}
	if policy := revision.Deployment; policy != nil {
		projected.Deployment = &model.IntegrationDeploymentPolicyView{
			ValidationTimeoutSeconds:   int(policy.ConnectionValidation.TimeoutSeconds),
			ValidationMaxAgeSeconds:    int(policy.ConnectionValidation.MaxAgeSeconds),
			ScheduleMode:               string(policy.Schedule.Mode),
			CronExpression:             optionalPreviewString(policy.Schedule.CronExpression),
			Timezone:                   optionalPreviewString(policy.Schedule.Timezone),
			HealthStartupGraceSeconds:  int(policy.Health.StartupGraceSeconds),
			HealthCheckIntervalSeconds: int(policy.Health.CheckIntervalSeconds),
			HealthTimeoutSeconds:       int(policy.Health.TimeoutSeconds),
			HealthFailureThreshold:     policy.Health.FailureThreshold,
			MaxInFlight:                policy.Capacity.MaxInFlight,
			MaxQueued:                  policy.Capacity.MaxQueued,
			MaxMessagesPerSecond:       policy.Capacity.MaxMessagesPerSecond,
		}
	} else {
		// A catalog revision always carries one (ValidateForDeployment); an
		// empty view is never a claimed default.
		projected.Deployment = &model.IntegrationDeploymentPolicyView{}
	}
	return projected
}

func projectIntegrationDefinitionDetail(service *authoring.Service, definition authoring.Definition) *model.IntegrationDefinitionDetail {
	projected := projectIntegrationDefinition(definition.Snapshot, definition.Revision)
	detail := &model.IntegrationDefinitionDetail{
		Definition:              &projected,
		RealValidationAvailable: service.RealAvailable(definition.Revision.Source),
	}
	if record := definition.Validation; record != nil {
		detail.Validation = &model.IntegrationDefinitionValidation{
			ValidationID:   record.ID,
			Passed:         record.Passed,
			Codes:          nonNilStrings(append([]string(nil), record.Codes...)),
			CheckedAt:      record.CheckedAt.UTC(),
			ExpiresAt:      record.ExpiresAt.UTC(),
			SourceRevision: artifactRevisionPointer(record.SourceRevision),
			Actor:          projectAuditPrincipal(record.Audit.Principal),
			Reason:         record.Audit.Reason,
		}
	}
	if approval := definition.Approval; approval != nil {
		detail.Approval = &model.IntegrationDefinitionApproval{
			EventID: approval.ID, Actor: projectAuditPrincipal(approval.Audit.Principal),
			Reason: approval.Audit.Reason, OccurredAt: approval.Audit.OccurredAt.UTC(),
		}
	}
	if release := definition.Release; release != nil {
		detail.Release = &model.IntegrationDefinitionRelease{
			ReleaseID: release.ID, Digest: release.Digest, ValidationID: release.ValidationID,
			ApprovalEventID: release.ApprovalEventID, PublishedBy: projectAuditPrincipal(release.Published.Principal),
			PublishedReason: release.Published.Reason, PublishedAt: release.Published.OccurredAt.UTC(),
		}
	}
	return detail
}
