package resolvers

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/vektah/gqlparser/v2/gqlerror"

	graphqlapi "gitlab.flexinfer.ai/libs/fi-fhir/internal/api/graphql"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/graphql/model"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/connection"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/lifecycle/authoring"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

// The connection catalog and engine runtime resolvers (.loom/38 C-0). They
// follow operator_control_plane.go: a fail-closed service accessor, an
// inventory-safe error catalog, and projections that add nothing to what the
// service returns. Every role check happens twice — at the transport gate
// (rootFieldRoles) and again inside connection.Service.

var (
	// ErrConnectionCatalogUnavailable keeps the catalog closed until serve
	// wires it. Like ErrOperatorControlPlaneUnavailable it is deliberately
	// indistinguishable from a missing capability at the GraphQL layer;
	// /api/auth/status reports the deployment fact (capabilities.connectionCatalog).
	ErrConnectionCatalogUnavailable = errors.New("connection catalog unavailable")
	// ErrEngineRuntimeUnavailable means no runtime description was composed:
	// only `serve` knows what it mounted, so any other composition has none.
	ErrEngineRuntimeUnavailable = errors.New("engine runtime unavailable")
)

// WithConnectionCatalog enables the durable connection catalog. A nil service
// leaves every catalog field fail-closed.
func WithConnectionCatalog(service *connection.Service) ResolverOption {
	return func(r *Resolver) {
		r.ConnectionCatalog = service
	}
}

// WithEngineRuntime installs the description `serve` composed at startup. The
// resolver keeps its own copy and never reads the environment itself.
func WithEngineRuntime(description *connection.RuntimeDescription) ResolverOption {
	return func(r *Resolver) {
		if description == nil {
			r.EngineRuntimeDescription = nil
			return
		}
		clone := description.Clone()
		r.EngineRuntimeDescription = &clone
	}
}

func (r *Resolver) connectionService() (*connection.Service, error) {
	if r == nil || r.ConnectionCatalog == nil {
		return nil, ErrConnectionCatalogUnavailable
	}
	return r.ConnectionCatalog, nil
}

// catalogConnectionError maps catalog failures onto stable, inventory-safe
// GraphQL messages. Not-found and forbidden stay distinct because the service
// already collapses another tenant's connection into not-found.
func catalogConnectionError(err error) error {
	var specErr *connection.SpecError
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return err
	case errors.As(err, &specErr):
		return connectionSpecRejected(specErr.Problems)
	case errors.Is(err, connection.ErrUnauthenticated):
		return errors.New("authentication required")
	case errors.Is(err, connection.ErrForbidden):
		return errors.New("connection catalog action forbidden")
	case errors.Is(err, connection.ErrInvalidRequest):
		return errors.New("invalid connection catalog request")
	case errors.Is(err, connection.ErrNotFound):
		return errors.New("connection not found")
	case errors.Is(err, connection.ErrAlreadyExists):
		return errors.New("connection already exists")
	case errors.Is(err, connection.ErrVersionConflict):
		return errors.New("connection version conflict")
	case errors.Is(err, connection.ErrArchived):
		return errors.New("connection is archived")
	case errors.Is(err, connection.ErrUnavailable), errors.Is(err, ErrConnectionCatalogUnavailable):
		return ErrConnectionCatalogUnavailable
	default:
		return errors.New("connection catalog request failed")
	}
}

// connectionSpecRejected carries the refused paths to the caller so the form
// can mark the field. Nothing in it is inventory: every path and code was
// computed from the request's own spec.
func connectionSpecRejected(problems []connection.Problem) *gqlerror.Error {
	listed := make([]map[string]any, 0, len(problems))
	code := connection.CodeSecretValueForbidden
	for index, problem := range problems {
		if index == 0 {
			code = problem.Code
		}
		listed = append(listed, map[string]any{"code": problem.Code, "path": problem.Path, "message": problem.Message})
	}
	return &gqlerror.Error{
		Message:    graphqlapi.ConnectionSpecRejectedMessage,
		Extensions: map[string]any{"code": code, "problems": listed},
	}
}

func (r *queryResolver) connections(
	ctx context.Context,
	direction *model.ConnectionDirection,
	includeArchived *bool,
) ([]model.Connection, error) {
	service, err := r.connectionService()
	if err != nil {
		return nil, catalogConnectionError(err)
	}
	filter := connection.ListFilter{}
	if direction != nil {
		filter.Direction = connection.Direction(strings.ToLower(string(*direction)))
	}
	if includeArchived != nil {
		filter.IncludeArchived = *includeArchived
	}
	listed, err := service.List(ctx, filter)
	if err != nil {
		return nil, catalogConnectionError(err)
	}
	projected := make([]model.Connection, 0, len(listed))
	for _, item := range listed {
		projected = append(projected, projectConnection(item))
	}
	return projected, nil
}

func (r *queryResolver) connectionByID(ctx context.Context, id string) (*model.Connection, error) {
	service, err := r.connectionService()
	if err != nil {
		return nil, catalogConnectionError(err)
	}
	found, err := service.Get(ctx, id)
	if errors.Is(err, connection.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, catalogConnectionError(err)
	}
	projected := projectConnection(found)
	return &projected, nil
}

func (r *queryResolver) connectionRevisions(ctx context.Context, id string) ([]model.ConnectionRevision, error) {
	service, err := r.connectionService()
	if err != nil {
		return nil, catalogConnectionError(err)
	}
	revisions, err := service.ListRevisions(ctx, id)
	if err != nil {
		return nil, catalogConnectionError(err)
	}
	projected := make([]model.ConnectionRevision, 0, len(revisions))
	for _, revision := range revisions {
		projected = append(projected, projectConnectionRevision(revision))
	}
	return projected, nil
}

func (r *queryResolver) connectionRevision(ctx context.Context, artifactID, revisionID string) (*model.ConnectionRevision, error) {
	service, err := r.connectionService()
	if err != nil {
		return nil, catalogConnectionError(err)
	}
	revision, err := service.GetRevision(ctx, artifactID, revisionID)
	if errors.Is(err, connection.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, catalogConnectionError(err)
	}
	projected := projectConnectionRevision(revision)
	return &projected, nil
}

func (r *queryResolver) engineRuntime(ctx context.Context) (*model.EngineRuntime, error) {
	if r == nil || r.EngineRuntimeDescription == nil {
		return nil, ErrEngineRuntimeUnavailable
	}
	if err := connection.AuthorizeRuntimeRead(ctx, r.EngineRuntimeDescription); err != nil {
		return nil, catalogConnectionError(err)
	}
	projected := projectEngineRuntime(r.EngineRuntimeDescription.Clone())
	if r.ConnectionCatalog != nil {
		observations, err := r.ConnectionCatalog.Observations(ctx)
		if err != nil {
			return nil, catalogConnectionError(err)
		}
		projected.Observations = projectEngineObservations(observations)
	}
	return &projected, nil
}

func projectEngineObservations(observations []connection.ObservationView) []model.EngineObservation {
	projected := make([]model.EngineObservation, 0, len(observations))
	for _, observation := range observations {
		projected = append(projected, model.EngineObservation{
			ReplicaID:   observation.ReplicaID,
			Adapter:     observation.Adapter,
			ArtifactID:  optionalPreviewString(observation.ArtifactID),
			RevisionID:  optionalPreviewString(observation.RevisionID),
			Digest:      optionalPreviewString(observation.Digest),
			HeartbeatAt: observation.HeartbeatAt.UTC(),
			Stale:       observation.Stale,
		})
	}
	return projected
}

func (r *mutationResolver) createConnection(ctx context.Context, input model.CreateConnectionInput) (*model.Connection, error) {
	service, err := r.connectionService()
	if err != nil {
		return nil, catalogConnectionError(err)
	}
	spec, err := json.Marshal(input.Spec)
	if err != nil {
		return nil, catalogConnectionError(connection.ErrInvalidRequest)
	}
	created, err := service.Create(ctx, connection.CreateRequest{
		ID:             input.ID,
		Direction:      connection.Direction(strings.ToLower(string(input.Direction))),
		Kind:           connection.Kind(strings.ToLower(string(input.Kind))),
		Name:           input.Name,
		Description:    optionalStringValue(input.Description),
		Spec:           spec,
		SecretBindings: secretBindingsFromInput(input.SecretBindings),
		Reason:         input.Reason,
	})
	if err != nil {
		return nil, catalogConnectionError(err)
	}
	projected := projectConnection(created)
	return &projected, nil
}

func (r *mutationResolver) updateConnection(ctx context.Context, input model.UpdateConnectionInput) (*model.Connection, error) {
	service, err := r.connectionService()
	if err != nil {
		return nil, catalogConnectionError(err)
	}
	request := connection.UpdateRequest{
		ID:              input.ID,
		ExpectedVersion: int64(input.ExpectedVersion),
		Name:            input.Name,
		Description:     input.Description,
		Reason:          input.Reason,
	}
	if input.Spec != nil {
		spec, err := json.Marshal(input.Spec)
		if err != nil {
			return nil, catalogConnectionError(connection.ErrInvalidRequest)
		}
		request.Spec = spec
	}
	// A present list — even an empty one — replaces the bindings; an absent
	// one keeps them. gqlgen decodes `[]` to an empty, non-nil slice.
	if input.SecretBindings != nil {
		bindings := secretBindingsFromInput(input.SecretBindings)
		request.SecretBindings = &bindings
	}
	updated, err := service.Update(ctx, request)
	if err != nil {
		return nil, catalogConnectionError(err)
	}
	projected := projectConnection(updated)
	return &projected, nil
}

func (r *mutationResolver) archiveConnection(ctx context.Context, input model.ConnectionCommandInput) (*model.Connection, error) {
	service, err := r.connectionService()
	if err != nil {
		return nil, catalogConnectionError(err)
	}
	archived, err := service.Archive(ctx, connectionCommand(input))
	if err != nil {
		return nil, catalogConnectionError(err)
	}
	projected := projectConnection(archived)
	return &projected, nil
}

func (r *mutationResolver) compileConnection(ctx context.Context, input model.ConnectionCommandInput) (*model.ConnectionCompileResult, error) {
	service, err := r.connectionService()
	if err != nil {
		return nil, catalogConnectionError(err)
	}
	result, err := service.Compile(ctx, connectionCommand(input))
	if err != nil {
		return nil, catalogConnectionError(err)
	}
	projected := projectConnection(result.Connection)
	compiled := &model.ConnectionCompileResult{
		Connection: &projected,
		Problems:   projectConnectionProblems(result.Problems),
	}
	if result.Revision != nil {
		revision := projectConnectionRevision(*result.Revision)
		compiled.Revision = &revision
	}
	return compiled, nil
}

func (r *mutationResolver) validateConnectionSpec(ctx context.Context, input model.ValidateConnectionSpecInput) ([]model.ConnectionProblem, error) {
	service, err := r.connectionService()
	if err != nil {
		return nil, catalogConnectionError(err)
	}
	spec, err := json.Marshal(input.Spec)
	if err != nil {
		return nil, catalogConnectionError(connection.ErrInvalidRequest)
	}
	problems, err := service.ValidateSpec(ctx, connection.ValidateRequest{
		Kind:           connection.Kind(strings.ToLower(string(input.Kind))),
		Spec:           spec,
		SecretBindings: secretBindingsFromInput(input.SecretBindings),
	})
	if err != nil {
		return nil, catalogConnectionError(err)
	}
	return projectConnectionProblems(problems), nil
}

func connectionCommand(input model.ConnectionCommandInput) connection.CommandRequest {
	return connection.CommandRequest{
		ID:              input.ID,
		ExpectedVersion: int64(input.ExpectedVersion),
		Reason:          input.Reason,
	}
}

func secretBindingsFromInput(inputs []model.ConnectionSecretBindingInput) []integration.SecretBinding {
	bindings := make([]integration.SecretBinding, 0, len(inputs))
	for _, input := range inputs {
		bindings = append(bindings, integration.SecretBinding{
			Name: input.Name,
			Reference: integration.SecretReference{
				Provider: integration.SecretProviderKind(input.Provider),
				Key:      input.Key,
				Version:  optionalStringValue(input.Version),
			},
		})
	}
	return bindings
}

func projectConnection(item connection.Connection) model.Connection {
	spec := map[string]any{}
	if len(item.Spec) > 0 {
		// The store only ever holds a JSON object here (a CHECK constraint
		// says so); an unreadable one projects as empty rather than failing
		// the whole list.
		if err := json.Unmarshal(item.Spec, &spec); err != nil || spec == nil {
			spec = map[string]any{}
		}
	}
	bindings := make([]model.ConnectionSecretBinding, 0, len(item.SecretBindings))
	for _, binding := range item.SecretBindings {
		bindings = append(bindings, model.ConnectionSecretBinding{
			Name:     binding.Name,
			Provider: string(binding.Reference.Provider),
			Key:      binding.Reference.Key,
			Version:  optionalPreviewString(binding.Reference.Version),
		})
	}
	references := make([]model.ConnectionReference, 0, len(item.References))
	for _, reference := range item.References {
		references = append(references, model.ConnectionReference{
			DefinitionID: reference.DefinitionID,
			RevisionID:   reference.RevisionID,
			Digest:       reference.Digest,
			State:        reference.State,
			Health:       reference.Health,
		})
	}
	projected := model.Connection{
		ID:             item.ID,
		Direction:      model.ConnectionDirection(strings.ToUpper(string(item.Direction))),
		Kind:           model.ConnectionKind(strings.ToUpper(string(item.Kind))),
		Name:           item.Name,
		Description:    item.Description,
		Spec:           spec,
		SecretBindings: bindings,
		Version:        int(item.Version),
		Archived:       item.Archived(),
		References:     references,
		Runtime: &model.ConnectionRuntimeState{
			Mounted:          item.Runtime.Mounted,
			Role:             optionalPreviewString(item.Runtime.Role),
			Detail:           optionalPreviewString(item.Runtime.Detail),
			RevisionID:       optionalPreviewString(item.Runtime.RevisionID),
			Digest:           optionalPreviewString(item.Runtime.Digest),
			ObservedReplicas: item.Runtime.ObservedReplicas,
			TotalReplicas:    item.Runtime.TotalReplicas,
		},
		CreatedBy:     projectAuditPrincipal(item.Created.Principal),
		CreatedAt:     item.Created.OccurredAt.UTC(),
		UpdatedBy:     projectAuditPrincipal(item.Updated.Principal),
		UpdatedReason: item.Updated.Reason,
		UpdatedAt:     item.Updated.OccurredAt.UTC(),
	}
	if item.LatestRevision != nil {
		revision := projectConnectionRevision(*item.LatestRevision)
		projected.LatestRevision = &revision
	}
	return projected
}

func projectConnectionRevision(revision connection.Revision) model.ConnectionRevision {
	projected := model.ConnectionRevision{
		ArtifactID:          revision.ArtifactID,
		RevisionID:          revision.RevisionID,
		Digest:              revision.Digest,
		Direction:           model.ConnectionDirection(strings.ToUpper(string(revision.Direction))),
		Kind:                model.ConnectionKind(strings.ToUpper(string(revision.Kind))),
		RevisionJSON:        string(revision.Document),
		CompiledFromVersion: int(revision.CompiledFromVersion),
		SecretBindingNames:  []string{},
		CreatedBy:           projectAuditPrincipal(revision.Created.Principal),
		CreatedReason:       revision.Created.Reason,
		CreatedAt:           revision.Created.OccurredAt.UTC(),
	}
	// .loom/42 E-1: what a definition binds, decoded with the kind's own
	// decoder. A document that does not decode projects none of it.
	if source, err := authoring.SourceFromDocument(revision.Kind, revision.Document); err == nil {
		projected.SourceID = optionalPreviewString(source.SourceID)
		projected.SecretBindingNames = nonNilStrings(source.BindingNames)
	} else if destination, err := authoring.DestinationFromDocument(revision.Kind, revision.Document); err == nil {
		projected.DestinationClass = optionalPreviewString(string(destination.Ref.Class))
		projected.SecretBindingNames = nonNilStrings(destination.BindingNames)
	}
	return projected
}

func projectConnectionProblems(problems []connection.Problem) []model.ConnectionProblem {
	projected := make([]model.ConnectionProblem, 0, len(problems))
	for _, problem := range problems {
		projected = append(projected, model.ConnectionProblem{Code: problem.Code, Path: problem.Path, Message: problem.Message})
	}
	return projected
}

func projectAuditPrincipal(principal integration.Principal) *model.OperatorPrincipal {
	return &model.OperatorPrincipal{
		ID:         principal.ID,
		Kind:       string(principal.Kind),
		AuthMethod: principal.AuthMethod,
		Roles:      nonNilStrings(append([]string(nil), principal.Roles...)),
	}
}

func projectEngineRuntime(description connection.RuntimeDescription) model.EngineRuntime {
	integrations := make([]model.EngineRegistryIntegration, 0, len(description.Registry.Integrations))
	for _, item := range description.Registry.Integrations {
		integrations = append(integrations, model.EngineRegistryIntegration{
			IntegrationID: item.IntegrationID, DefinitionID: item.DefinitionID, RevisionID: item.RevisionID,
			Digest: item.Digest, SourceID: item.SourceID, Format: item.Format,
		})
	}
	adapters := make([]model.EngineAdapter, 0, len(description.Adapters))
	for _, adapter := range description.Adapters {
		adapters = append(adapters, projectEngineAdapter(adapter))
	}
	ledgers := make([]model.EngineLedger, 0, len(description.Ledgers))
	for _, ledger := range description.Ledgers {
		ledgers = append(ledgers, model.EngineLedger{Name: ledger.Name, Version: ledger.Version})
	}
	properties := make([]model.EngineProperty, 0, len(description.Properties))
	for _, property := range description.Properties {
		properties = append(properties, model.EngineProperty{
			Key: property.Key, Value: property.Value, Secret: property.Secret, Source: property.Source,
		})
	}
	projected := model.EngineRuntime{
		Version:             description.Version,
		TenantID:            description.TenantID,
		ReplicaID:           description.ReplicaID,
		AuthMode:            description.AuthMode,
		TrustedNetwork:      description.TrustedNetwork,
		AccessIdentity:      description.AccessIdentity,
		ControlPlane:        description.ControlPlane,
		IntegrationSessions: description.IntegrationSessions,
		Streaming:           description.Streaming,
		RetentionPurge:      description.RetentionPurge,
		LlmConfigured:       description.LLMConfigured,
		Registry: &model.EngineRegistry{
			IntegrationCount: description.Registry.IntegrationCount,
			Integrations:     integrations,
		},
		Adapters:     adapters,
		Ledgers:      ledgers,
		Properties:   properties,
		Observations: []model.EngineObservation{},
	}
	if description.DestinationIdentity != nil {
		destinations := make([]model.EngineDestination, 0, len(description.DestinationIdentity.Destinations))
		for _, item := range description.DestinationIdentity.Destinations {
			destinations = append(destinations, model.EngineDestination{
				ArtifactID: item.ArtifactID, RevisionID: item.RevisionID, Digest: item.Digest,
				Transport: item.Transport, Class: item.Class, EndpointAdvisory: item.EndpointAdvisory,
			})
		}
		projected.DestinationIdentity = &model.EngineDestinationIdentity{
			Mode:         description.DestinationIdentity.Mode,
			Destinations: destinations,
		}
	}
	return projected
}

// projectEngineAdapter renders an adapter with every field it does not have
// as null: an empty string or a nil number is absent, never a default.
func projectEngineAdapter(adapter connection.RuntimeAdapter) model.EngineAdapter {
	return model.EngineAdapter{
		Kind:                    adapter.Kind,
		Enabled:                 adapter.Enabled,
		DefinitionID:            optionalPreviewString(adapter.DefinitionID),
		IntegrationID:           optionalPreviewString(adapter.IntegrationID),
		SourceID:                optionalPreviewString(adapter.SourceID),
		SourceRevisionID:        optionalPreviewString(adapter.SourceRevisionID),
		SourceDigest:            optionalPreviewString(adapter.SourceDigest),
		ListenAddress:           optionalPreviewString(adapter.ListenAddress),
		Path:                    optionalPreviewString(adapter.Path),
		AuthMode:                optionalPreviewString(adapter.AuthMode),
		TLSMode:                 optionalPreviewString(adapter.TLSMode),
		Provider:                optionalPreviewString(adapter.Provider),
		PollSeconds:             optionalInt(adapter.PollSeconds),
		MaxConnections:          optionalInt(adapter.MaxConnections),
		MaxMessageBytes:         optionalInt(adapter.MaxMessageBytes),
		MaxBodyBytes:            optionalInt(adapter.MaxBodyBytes),
		RequireClientIdentity:   optionalBool(adapter.RequireClientIdentity),
		RequireWorkloadIdentity: optionalBool(adapter.RequireWorkloadIdentity),
		QueueDriver:             optionalPreviewString(adapter.QueueDriver),
		MaxAttempts:             optionalInt(adapter.MaxAttempts),
		WorkerID:                optionalPreviewString(adapter.WorkerID),
	}
}

func optionalInt(value *int64) *int {
	if value == nil {
		return nil
	}
	converted := int(*value)
	return &converted
}

func optionalBool(value *bool) *bool {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}
