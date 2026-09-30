package authoring

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/requestsecurity"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/connection"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/lifecycle"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

// Roles. Reads ride the operator read role; writes add the deployment grant,
// as the connection catalog's do (.loom/38 Decision 4: no new role).
const (
	ReadRole  = connection.ReadRole
	WriteRole = connection.WriteRole
	// MaxReasonBytes bounds every write's reason.
	MaxReasonBytes = 1024
	// MaxDefinitions bounds one List call.
	MaxDefinitions = 500
	// MaxDestinations bounds one draft's destination list.
	MaxDestinations = 32
)

var (
	// ErrUnavailable means authoring is not configured on this deployment:
	// no lifecycle catalog, no connection catalog, or no static registry.
	ErrUnavailable = errors.New("integration definition authoring unavailable")
	// ErrUnauthenticated means no verified caller identity reached the service.
	ErrUnauthenticated = errors.New("authentication required")
	// ErrForbidden means the caller lacks a required role or tenant.
	ErrForbidden = errors.New("integration definition authoring forbidden")
	// ErrInvalidRequest is a malformed identifier, reason, version, or mode.
	ErrInvalidRequest = errors.New("invalid integration definition request")
	// ErrRealUnavailable means this replica cannot run a real check for the
	// definition's source: only the batch source it mounts, whose credentials
	// it holds, can be validated for real.
	ErrRealUnavailable = errors.New("real validation is unavailable for this source on this replica")
	// ErrRealBusy means another real validation is running on this replica.
	ErrRealBusy = errors.New("a real validation is already running on this replica")
)

// Catalog is the lifecycle surface authoring drives; *lifecycle.PostgresCatalog
// satisfies it.
type Catalog interface {
	CreateDraft(context.Context, integration.IntegrationDefinitionRevision) (lifecycle.Snapshot, error)
	GetSnapshot(ctx context.Context, tenantID, definitionID, revisionID string) (lifecycle.Snapshot, error)
	GetDefinition(ctx context.Context, tenantID, definitionID, revisionID string) (lifecycle.DefinitionRow, error)
	ListDefinitions(ctx context.Context, tenantID string, includeRetired bool, limit int) ([]lifecycle.DefinitionRow, error)
	GetValidation(ctx context.Context, validationID string) (lifecycle.ValidationRecord, error)
	GetRelease(ctx context.Context, releaseID string) (lifecycle.ReleaseRecord, error)
	ListEvents(ctx context.Context, tenantID, definitionID, revisionID string) ([]lifecycle.EventRecord, error)
	ValidateConnection(context.Context, lifecycle.Command) (lifecycle.Snapshot, error)
	Approve(context.Context, lifecycle.Command) (lifecycle.Snapshot, error)
	Publish(context.Context, lifecycle.Command) (lifecycle.Snapshot, error)
}

// Revisions reads compiled connection revisions; *connection.Service
// satisfies it and re-checks the read role itself.
type Revisions interface {
	GetRevision(ctx context.Context, artifactID, revisionID string) (connection.Revision, error)
}

// Config composes a Service.
type Config struct {
	Catalog   Catalog
	Revisions Revisions
	Registry  *Registry
	TenantID  string
	// ValidationMaxAgeSeconds is the max age a new draft's deployment policy
	// defaults to (FI_FHIR_LIFECYCLE_VALIDATION_MAX_AGE).
	ValidationMaxAgeSeconds int64
	// RealSource is the batch source whose credentials this process holds,
	// or nil. REAL is offered only for a definition naming exactly it.
	RealSource *integration.SourceRevisionRef
	Clock      func() time.Time
}

// Service is the authorization boundary of definition authoring. Every
// method resolves the verified caller from the context, re-checks the roles
// the GraphQL transport gate already checked, and scopes to one tenant.
type Service struct {
	catalog    Catalog
	revisions  Revisions
	registry   *Registry
	tenantID   string
	maxAge     int64
	realSource *integration.SourceRevisionRef
	clock      func() time.Time
	realSlot   chan struct{}
}

// NewService builds the service. Catalog, Revisions, and Registry are all
// required: without any of them nothing can be authored honestly.
func NewService(config Config) (*Service, error) {
	if config.Catalog == nil || config.Revisions == nil || config.Registry == nil || !validIdentity(config.TenantID) {
		return nil, ErrUnavailable
	}
	maxAge := config.ValidationMaxAgeSeconds
	if maxAge <= 0 {
		maxAge = DefaultValidationMaxAgeSeconds
	}
	if err := DefaultDeploymentPolicy(maxAge).Validate(); err != nil {
		return nil, fmt.Errorf("validation max age: %w", err)
	}
	clock := config.Clock
	if clock == nil {
		clock = time.Now
	}
	var realSource *integration.SourceRevisionRef
	if config.RealSource != nil {
		copied := *config.RealSource
		realSource = &copied
	}
	return &Service{
		catalog: config.Catalog, revisions: config.Revisions, registry: config.Registry,
		tenantID: config.TenantID, maxAge: maxAge, realSource: realSource, clock: clock,
		realSlot: make(chan struct{}, 1),
	}, nil
}

// ValidationMaxAgeSeconds is the default evidence max age for new drafts.
func (s *Service) ValidationMaxAgeSeconds() int64 {
	if s == nil {
		return DefaultValidationMaxAgeSeconds
	}
	return s.maxAge
}

func (s *Service) authorize(ctx context.Context, roles ...string) (integration.SecurityContext, error) {
	if s == nil {
		return integration.SecurityContext{}, ErrUnavailable
	}
	security, authenticated := requestsecurity.SecurityContextFromContext(ctx)
	if !authenticated {
		return integration.SecurityContext{}, ErrUnauthenticated
	}
	if !validIdentity(security.TenantID) || security.TenantID != s.tenantID ||
		!validIdentity(security.Principal.ID) || strings.TrimSpace(security.Principal.AuthMethod) == "" ||
		(security.Principal.Kind != integration.PrincipalKindHuman && security.Principal.Kind != integration.PrincipalKindService) {
		return integration.SecurityContext{}, ErrForbidden
	}
	for _, required := range roles {
		held := false
		for _, role := range security.Principal.Roles {
			if role == required {
				held = true
				break
			}
		}
		if !held {
			return integration.SecurityContext{}, ErrForbidden
		}
	}
	return security, nil
}

// Definition is one definition revision as the editor reads it: its
// lifecycle snapshot, the stored revision, and — when present — its current
// validation record, release record, and approval event.
type Definition struct {
	Snapshot   lifecycle.Snapshot
	Revision   integration.IntegrationDefinitionRevision
	Validation *lifecycle.ValidationRecord
	Release    *lifecycle.ReleaseRecord
	Approval   *lifecycle.EventRecord
}

// List returns the tenant's definition revisions with their snapshots.
func (s *Service) List(ctx context.Context, includeRetired bool) ([]lifecycle.DefinitionRow, error) {
	security, err := s.authorize(ctx, ReadRole)
	if err != nil {
		return nil, err
	}
	return s.catalog.ListDefinitions(ctx, security.TenantID, includeRetired, MaxDefinitions)
}

// Get returns one definition revision with its evidence, or lifecycle.ErrNotFound.
func (s *Service) Get(ctx context.Context, definitionID, revisionID string) (Definition, error) {
	security, err := s.authorize(ctx, ReadRole)
	if err != nil {
		return Definition{}, err
	}
	if !validIdentity(definitionID) || !validIdentity(revisionID) {
		return Definition{}, lifecycle.ErrNotFound
	}
	return s.definition(ctx, security.TenantID, definitionID, revisionID)
}

func (s *Service) definition(ctx context.Context, tenantID, definitionID, revisionID string) (Definition, error) {
	row, err := s.catalog.GetDefinition(ctx, tenantID, definitionID, revisionID)
	if err != nil {
		return Definition{}, err
	}
	definition := Definition{Snapshot: row.Snapshot, Revision: row.Revision}
	if row.Snapshot.LastValidationID != "" {
		record, err := s.catalog.GetValidation(ctx, row.Snapshot.LastValidationID)
		if err != nil {
			return Definition{}, err
		}
		definition.Validation = &record
	}
	if row.Snapshot.ReleaseID != "" {
		release, err := s.catalog.GetRelease(ctx, row.Snapshot.ReleaseID)
		if err != nil {
			return Definition{}, err
		}
		definition.Release = &release
	}
	if row.Snapshot.ApprovalEventID != "" {
		events, err := s.catalog.ListEvents(ctx, tenantID, definitionID, revisionID)
		if err != nil {
			return Definition{}, err
		}
		for index := range events {
			if events[index].ID == row.Snapshot.ApprovalEventID {
				approval := events[index]
				definition.Approval = &approval
				break
			}
		}
	}
	return definition, nil
}

// RegistryArtifacts lists the profile/workflow pairs a definition may bind.
func (s *Service) RegistryArtifacts(ctx context.Context) ([]RegistryArtifact, error) {
	if _, err := s.authorize(ctx, ReadRole); err != nil {
		return nil, err
	}
	return s.registry.Artifacts(ctx)
}

// RevisionRef names one compiled connection revision.
type RevisionRef struct {
	ArtifactID string
	RevisionID string
}

// DraftInput is a definition as the editor submits it: connection revisions
// by reference, the registry pair, explicit binding references, and policy.
// A nil Deployment takes the default policy with this deployment's max age.
type DraftInput struct {
	DefinitionID     string
	RevisionID       string
	ParentRevisionID string
	Source           RevisionRef
	Destinations     []RevisionRef
	Profile          integration.ArtifactRevisionRef
	Workflow         integration.ArtifactRevisionRef
	SecretBindings   []integration.SecretBinding
	RawRetention     *integration.RawRetentionPolicy
	Deployment       *integration.IntegrationDeploymentPolicy
}

// Problem codes Check adds beyond the connection catalog's.
const (
	CodeNotFound                   = "NOT_FOUND"
	CodeWrongDirection             = "WRONG_DIRECTION"
	CodeAlreadyExists              = "ALREADY_EXISTS"
	CodeArtifactUnresolved         = "ARTIFACT_UNRESOLVED"
	CodeWorkflowDestinationMissing = "WORKFLOW_DESTINATION_MISSING"
	CodeDefinitionInvalid          = "DEFINITION_INVALID"
)

// Check runs every pre-flight the seed runs, without writing: the connection
// revisions exist and point the right way; the profile and workflow resolve
// with the runtime's resolver; every non-log workflow action delivers to one
// of the chosen destinations; every binding the chosen revisions name is
// bound; the revision builds and passes ValidateForDeployment; the revision
// ID is not taken. Only UNUSED_BINDING is a warning.
func (s *Service) Check(ctx context.Context, input DraftInput) ([]connection.Problem, error) {
	security, err := s.authorize(ctx, ReadRole, WriteRole)
	if err != nil {
		return nil, err
	}
	created := s.created(security, "pre-flight check of a definition draft")
	_, problems, err := s.assemble(ctx, security, input, created)
	if err != nil || connection.HasBlocking(problems) {
		return problems, err
	}
	if _, conflict, err := s.existing(ctx, security, input); err != nil {
		return nil, err
	} else if conflict != nil {
		problems = append(problems, *conflict)
	}
	return problems, nil
}

// CreateResult is a draft write's outcome: Definition is nil exactly when a
// blocking problem was found, and then nothing was written.
type CreateResult struct {
	Definition *Definition
	Problems   []connection.Problem
}

// CreateDraft checks the input and, when nothing blocks, writes the draft at
// version one under the caller's identity and reason.
func (s *Service) CreateDraft(ctx context.Context, input DraftInput, reason string) (CreateResult, error) {
	security, err := s.authorize(ctx, ReadRole, WriteRole)
	if err != nil {
		return CreateResult{}, err
	}
	trimmed, err := validReason(reason)
	if err != nil {
		return CreateResult{}, err
	}
	revision, problems, err := s.assemble(ctx, security, input, s.created(security, trimmed))
	if err != nil {
		return CreateResult{}, err
	}
	if connection.HasBlocking(problems) || revision == nil {
		return CreateResult{Problems: problems}, nil
	}
	// Idempotent, as the seed is: an identical revision already stored is
	// returned; a different one under the same ID is refused.
	existing, conflict, err := s.existing(ctx, security, input)
	if err != nil {
		return CreateResult{}, err
	}
	if conflict != nil {
		return CreateResult{Problems: append(problems, *conflict)}, nil
	}
	if existing == nil {
		if _, err := s.catalog.CreateDraft(ctx, *revision); err != nil {
			if !errors.Is(err, lifecycle.ErrAlreadyExists) {
				return CreateResult{}, err
			}
			// Another writer created it between the read and the insert.
			if _, conflict, err = s.existing(ctx, security, input); err != nil {
				return CreateResult{}, err
			}
			if conflict != nil {
				return CreateResult{Problems: append(problems, *conflict)}, nil
			}
		}
	}
	definition, err := s.definition(ctx, security.TenantID, revision.DefinitionID, revision.RevisionID)
	if err != nil {
		return CreateResult{}, err
	}
	return CreateResult{Definition: &definition, Problems: problems}, nil
}

// created is the creation audit of a draft the caller writes now.
func (s *Service) created(security integration.SecurityContext, reason string) integration.AuditEnvelope {
	principal := security.Principal
	principal.Roles = append([]string(nil), security.Principal.Roles...)
	return integration.AuditEnvelope{
		TenantID: security.TenantID, Principal: principal, Reason: reason,
		OccurredAt: s.clock().UTC().Truncate(time.Second),
	}
}

// existing compares the input with a revision already stored under its
// identity, the way `lifecycle seed` does (verifyExistingSeedDefinition):
// rebuilt under the stored revision's own creation audit, an equal digest is
// the same revision (returned), a different one is a conflict problem. Both
// are nil when nothing is stored.
func (s *Service) existing(
	ctx context.Context,
	security integration.SecurityContext,
	input DraftInput,
) (*lifecycle.DefinitionRow, *connection.Problem, error) {
	row, err := s.catalog.GetDefinition(ctx, security.TenantID, input.DefinitionID, input.RevisionID)
	if errors.Is(err, lifecycle.ErrNotFound) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	rebuilt, problems, err := s.assemble(ctx, security, input, row.Revision.Created)
	if err != nil {
		return nil, nil, err
	}
	if connection.HasBlocking(problems) || rebuilt == nil || rebuilt.Digest != row.Revision.Digest {
		return nil, &connection.Problem{
			Code: CodeAlreadyExists, Path: "revisionId",
			Message: "this definition revision is already stored with different content; revisions are append-only, so choose a new revision ID",
		}, nil
	}
	return &row, nil, nil
}

// assemble resolves the input into a definition revision and the problems
// that block or warn. It returns a nil revision when a problem blocks.
func (s *Service) assemble(
	ctx context.Context,
	security integration.SecurityContext,
	input DraftInput,
	created integration.AuditEnvelope,
) (*integration.IntegrationDefinitionRevision, []connection.Problem, error) {
	problems := make([]connection.Problem, 0)
	add := func(code, path, message string) {
		problems = append(problems, connection.Problem{Code: code, Path: path, Message: message})
	}
	for _, field := range []struct{ path, value string }{
		{"definitionId", input.DefinitionID}, {"revisionId", input.RevisionID},
	} {
		if !validIdentity(field.value) {
			add(connection.CodeRequired, field.path, "a non-empty identifier of at most 256 characters, without surrounding whitespace or control characters, is required")
		}
	}
	if input.ParentRevisionID != "" && !validIdentity(input.ParentRevisionID) {
		add(connection.CodeInvalidValue, "parentRevisionId", "the parent revision ID is malformed")
	}
	// Secret references are checked with the connection catalog's own rules
	// before anything else: a definition revision is append-only, so a pasted
	// value must never reach it.
	for _, problem := range connection.CheckSecretBindingReferences(input.SecretBindings) {
		problem.Path = "secretBindings" + strings.TrimPrefix(problem.Path, "secret_bindings")
		problems = append(problems, problem)
	}
	if input.RawRetention != nil && input.RawRetention.EncryptionKey != nil {
		problems = append(problems, connection.CheckSecretReference("rawRetention.encryptionKey", *input.RawRetention.EncryptionKey)...)
	}
	if input.RawRetention != nil && strings.Contains(input.RawRetention.Purpose, "-----BEGIN") {
		add(connection.CodeSecretValueForbidden, "rawRetention.purpose", "a purpose never carries certificate or key material")
	}

	var source Source
	sourceOK := false
	if !validIdentity(input.Source.ArtifactID) || !validIdentity(input.Source.RevisionID) {
		add(connection.CodeRequired, "source", "choose a compiled source connection revision")
	} else {
		revision, err := s.revisions.GetRevision(ctx, input.Source.ArtifactID, input.Source.RevisionID)
		switch {
		case errors.Is(err, connection.ErrNotFound):
			add(CodeNotFound, "source", fmt.Sprintf("source connection revision %s/%s does not exist", input.Source.ArtifactID, input.Source.RevisionID))
		case err != nil:
			return nil, nil, err
		default:
			source, err = SourceFromDocument(revision.Kind, revision.Document)
			if errors.Is(err, ErrNotSource) {
				add(CodeWrongDirection, "source", fmt.Sprintf("%s/%s is a destination connection, not a source", input.Source.ArtifactID, input.Source.RevisionID))
			} else if err != nil {
				add(CodeDefinitionInvalid, "source", "the stored source revision does not decode with its kind's own decoder")
			} else {
				sourceOK = true
			}
		}
	}

	destinations := make([]Destination, 0, len(input.Destinations))
	destinationIDs := make(map[string]struct{}, len(input.Destinations))
	if len(input.Destinations) == 0 {
		add(connection.CodeRequired, "destinations", "choose at least one compiled destination connection revision")
	}
	if len(input.Destinations) > MaxDestinations {
		add(connection.CodeOutOfRange, "destinations", fmt.Sprintf("at most %d destinations", MaxDestinations))
	}
	for index, ref := range input.Destinations {
		path := fmt.Sprintf("destinations[%d]", index)
		if !validIdentity(ref.ArtifactID) || !validIdentity(ref.RevisionID) {
			add(connection.CodeRequired, path, "choose a compiled destination connection revision")
			continue
		}
		if _, duplicate := destinationIDs[ref.ArtifactID]; duplicate {
			add(connection.CodeDuplicate, path, fmt.Sprintf("destination %s is chosen more than once", ref.ArtifactID))
			continue
		}
		revision, err := s.revisions.GetRevision(ctx, ref.ArtifactID, ref.RevisionID)
		switch {
		case errors.Is(err, connection.ErrNotFound):
			add(CodeNotFound, path, fmt.Sprintf("destination connection revision %s/%s does not exist", ref.ArtifactID, ref.RevisionID))
			continue
		case err != nil:
			return nil, nil, err
		}
		destination, err := DestinationFromDocument(revision.Kind, revision.Document)
		if errors.Is(err, ErrNotDestination) {
			add(CodeWrongDirection, path, fmt.Sprintf("%s/%s is a source connection, not a destination", ref.ArtifactID, ref.RevisionID))
			continue
		}
		if err != nil {
			add(CodeDefinitionInvalid, path, "the stored destination revision does not decode with its kind's own decoder")
			continue
		}
		destinationIDs[ref.ArtifactID] = struct{}{}
		destinations = append(destinations, destination)
	}

	workflowYAML, err := s.registry.Resolve(ctx, input.Profile, input.Workflow)
	resolved := err == nil
	if err != nil {
		if ctx.Err() != nil {
			return nil, nil, ctx.Err()
		}
		add(CodeArtifactUnresolved, "profile",
			"the profile and workflow refs are not one static-registry entry's pair as the runtime's resolver proves it; choose a pair from the registry artifacts")
	}
	if resolved && len(destinations) == len(input.Destinations) && len(destinations) > 0 {
		var planErr *WorkflowDestinationError
		if err := RequireWorkflowDestinations(input.Workflow, workflowYAML, destinationIDs); errors.As(err, &planErr) {
			add(CodeWorkflowDestinationMissing, "destinations", fmt.Sprintf(
				"workflow route %q action %q delivers to destination %q, which is not among the chosen destinations; the runtime planner would refuse every matching message",
				planErr.Route, planErr.Action, planErr.Destination))
		} else if err != nil {
			add(CodeArtifactUnresolved, "workflow", "the registry workflow is not executable")
		}
	}

	// Bindings: every name a chosen revision requires must be bound; a bound
	// name no revision requires is a warning.
	required := make(map[string]string)
	if sourceOK {
		for _, name := range source.BindingNames {
			required[name] = "source"
		}
	}
	for _, destination := range destinations {
		for _, name := range destination.BindingNames {
			if _, seen := required[name]; !seen {
				required[name] = "destination " + destination.Ref.ArtifactID
			}
		}
	}
	bound := make(map[string]struct{}, len(input.SecretBindings))
	for index, binding := range input.SecretBindings {
		bound[binding.Name] = struct{}{}
		if _, needed := required[binding.Name]; !needed && binding.Name != "" {
			add(connection.CodeUnusedBinding, fmt.Sprintf("secretBindings[%d].name", index),
				"no chosen revision names this binding")
		}
	}
	names := make([]string, 0, len(required))
	for name := range required {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if _, ok := bound[name]; !ok {
			add(connection.CodeUnboundSecret, "secretBindings", fmt.Sprintf("%s names binding %q; bind it to a secret reference", required[name], name))
		}
	}

	if connection.HasBlocking(problems) {
		return nil, problems, nil
	}
	policy := DefaultPolicy()
	if input.RawRetention != nil {
		policy.RawRetention = *input.RawRetention
		if policy.RawRetention.EffectiveMode() == integration.RawRetentionModeEncrypted {
			// Encrypted retention is authorized by the author, audited on access.
			policy.RawRetention.AccessAuditRequired = true
		}
	}
	deployment := DefaultDeploymentPolicy(s.maxAge)
	if input.Deployment != nil {
		deployment = *input.Deployment
	}
	if policy.RawRetention.EffectiveMode() == integration.RawRetentionModeEncrypted {
		// The stored authorizer and audit flag, not the caller's, when this
		// rebuilds an existing revision for comparison.
		policy.RawRetention.AuthorizedBy = created.Principal
	}
	revision, err := BuildDefinition(Draft{
		DefinitionID: input.DefinitionID, RevisionID: input.RevisionID, ParentRevisionID: input.ParentRevisionID,
		TenantID: security.TenantID, Source: source, Profile: input.Profile, Workflow: input.Workflow,
		Destinations: destinations, SecretBindings: input.SecretBindings,
		Policy: policy, Deployment: deployment, Created: created,
	})
	if err != nil {
		var violations *integration.ValidationError
		if errors.As(err, &violations) {
			for _, violation := range violations.Violations {
				add(violation.Code, violation.Path, violation.Message)
			}
		} else {
			add(CodeDefinitionInvalid, "", "the definition does not build")
		}
		return nil, problems, nil
	}
	return &revision, problems, nil
}

// Command targets one definition revision at its expected snapshot version.
type Command struct {
	DefinitionID    string
	RevisionID      string
	ExpectedVersion int64
	Reason          string
}

// Validate records connection-validation evidence in the chosen mode. A
// failed check is recorded evidence, not an error: the returned definition
// carries the failed record.
func (s *Service) Validate(ctx context.Context, command Command, mode Mode) (Definition, error) {
	security, err := s.authorize(ctx, ReadRole, WriteRole)
	if err != nil {
		return Definition{}, err
	}
	lifecycleCommand, err := s.command(security, command)
	if err != nil {
		return Definition{}, err
	}
	switch mode {
	case ModeSkip:
		if len(lifecycleCommand.Reason) < MinSkipReasonBytes {
			return Definition{}, ErrInvalidRequest
		}
	case ModeStatic:
	case ModeReal:
		row, err := s.catalog.GetDefinition(ctx, security.TenantID, command.DefinitionID, command.RevisionID)
		if err != nil {
			return Definition{}, err
		}
		if s.realSource == nil || row.Revision.Source != *s.realSource {
			return Definition{}, ErrRealUnavailable
		}
		select {
		case s.realSlot <- struct{}{}:
		default:
			return Definition{}, ErrRealBusy
		}
		// The slot is held until the probe goroutine exits, which may be after
		// this request returns (a dial outliving the deadline); when no probe
		// starts, it is released here.
		var ticket *probeTicket
		ctx, ticket = withProbeTicket(ctx, func() { <-s.realSlot })
		defer func() {
			if !ticket.started.Load() {
				ticket.done()
			}
		}()
	default:
		return Definition{}, ErrInvalidRequest
	}
	_, err = s.catalog.ValidateConnection(WithMode(ctx, mode), lifecycleCommand)
	if err != nil && !errors.Is(err, lifecycle.ErrConnectionValidationFailed) {
		return Definition{}, err
	}
	return s.definition(ctx, security.TenantID, command.DefinitionID, command.RevisionID)
}

// RealAvailable reports whether REAL can run for a definition's source here.
func (s *Service) RealAvailable(source integration.SourceRevisionRef) bool {
	return s != nil && s.realSource != nil && *s.realSource == source
}

// Approve advances a currently validated revision.
func (s *Service) Approve(ctx context.Context, command Command) (Definition, error) {
	return s.transition(ctx, command, s.catalog.Approve)
}

// Publish creates the immutable release of an approved revision.
func (s *Service) Publish(ctx context.Context, command Command) (Definition, error) {
	return s.transition(ctx, command, s.catalog.Publish)
}

func (s *Service) transition(
	ctx context.Context,
	command Command,
	apply func(context.Context, lifecycle.Command) (lifecycle.Snapshot, error),
) (Definition, error) {
	security, err := s.authorize(ctx, ReadRole, WriteRole)
	if err != nil {
		return Definition{}, err
	}
	lifecycleCommand, err := s.command(security, command)
	if err != nil {
		return Definition{}, err
	}
	if _, err := apply(ctx, lifecycleCommand); err != nil {
		return Definition{}, err
	}
	return s.definition(ctx, security.TenantID, command.DefinitionID, command.RevisionID)
}

func (s *Service) command(security integration.SecurityContext, command Command) (lifecycle.Command, error) {
	reason, err := validReason(command.Reason)
	if err != nil {
		return lifecycle.Command{}, err
	}
	if !validIdentity(command.DefinitionID) || !validIdentity(command.RevisionID) || command.ExpectedVersion <= 0 {
		return lifecycle.Command{}, ErrInvalidRequest
	}
	principal := security.Principal
	principal.Roles = append([]string(nil), security.Principal.Roles...)
	return lifecycle.Command{
		TenantID: security.TenantID, DefinitionID: command.DefinitionID, RevisionID: command.RevisionID,
		ExpectedVersion: command.ExpectedVersion, Principal: principal, Reason: reason,
	}, nil
}

func validReason(reason string) (string, error) {
	trimmed := strings.TrimSpace(reason)
	if trimmed == "" || len(trimmed) > MaxReasonBytes {
		return "", ErrInvalidRequest
	}
	for _, character := range trimmed {
		if unicode.IsControl(character) && character != '\n' && character != '\t' {
			return "", ErrInvalidRequest
		}
	}
	return trimmed, nil
}

func validIdentity(value string) bool {
	if value == "" || len(value) > 256 || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value {
		if character < 0x20 || character == 0x7f {
			return false
		}
	}
	return true
}
