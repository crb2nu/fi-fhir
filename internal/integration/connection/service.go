package connection

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/requestsecurity"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/lifecycle"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

// DefinitionCatalog is the lifecycle read the reference projection needs.
// *lifecycle.PostgresCatalog satisfies it. It is optional: without it every
// connection reports no references, which is also the honest answer on a
// deployment whose lifecycle catalog nobody seeded (.loom/38, "What exists").
type DefinitionCatalog interface {
	ListSnapshots(ctx context.Context, tenantID string, limit int) ([]lifecycle.Snapshot, error)
	LoadDefinitionRevision(ctx context.Context, tenantID, definitionID, revisionID string) ([]byte, error)
}

// Service is the authorization boundary of the connection catalog. Every
// exported method resolves the verified caller from the request context —
// never from an argument — re-checks the roles the GraphQL transport gate
// already checked, and scopes every read and write to the one deployment
// tenant it is bound to, exactly as operator.Service does.
type Service struct {
	store    *PostgresStore
	catalog  DefinitionCatalog
	runtime  *RuntimeDescription
	tenantID string
}

// NewService binds the catalog to one deployment tenant. catalog and runtime
// may be nil: references are then empty and no connection reads as mounted.
func NewService(store *PostgresStore, catalog DefinitionCatalog, runtime *RuntimeDescription, tenantID string) (*Service, error) {
	if store == nil || !validIdentity(tenantID) {
		return nil, ErrUnavailable
	}
	var described *RuntimeDescription
	if runtime != nil {
		clone := runtime.Clone()
		described = &clone
	}
	return &Service{store: store, catalog: catalog, runtime: described, tenantID: tenantID}, nil
}

// authorize resolves verified caller identity and requires every listed role.
func authorize(ctx context.Context, tenantID string, roles ...string) (integration.SecurityContext, error) {
	security, authenticated := requestsecurity.SecurityContextFromContext(ctx)
	if !authenticated {
		return integration.SecurityContext{}, ErrUnauthenticated
	}
	if !validIdentity(security.TenantID) || security.TenantID != tenantID {
		return integration.SecurityContext{}, ErrForbidden
	}
	if !validIdentity(security.Principal.ID) || strings.TrimSpace(security.Principal.AuthMethod) == "" {
		return integration.SecurityContext{}, ErrForbidden
	}
	if security.Principal.Kind != integration.PrincipalKindHuman &&
		security.Principal.Kind != integration.PrincipalKindService {
		return integration.SecurityContext{}, ErrForbidden
	}
	for _, required := range roles {
		if !hasRole(security.Principal.Roles, required) {
			return integration.SecurityContext{}, ErrForbidden
		}
	}
	return security, nil
}

func hasRole(roles []string, wanted string) bool {
	for _, role := range roles {
		if role == wanted {
			return true
		}
	}
	return false
}

// AuthorizeRuntimeRead is the service-layer check behind engineRuntime, which
// has no store: the caller must hold ReadRole for the description's tenant.
func AuthorizeRuntimeRead(ctx context.Context, description *RuntimeDescription) error {
	if description == nil {
		return ErrUnavailable
	}
	_, err := authorize(ctx, description.TenantID, ReadRole)
	return err
}

func (s *Service) authorize(ctx context.Context, roles ...string) (integration.SecurityContext, error) {
	if s == nil || s.store == nil {
		return integration.SecurityContext{}, ErrUnavailable
	}
	return authorize(ctx, s.tenantID, roles...)
}

// ListFilter narrows List. An empty Direction lists both.
type ListFilter struct {
	Direction       Direction
	IncludeArchived bool
}

// CreateRequest declares a new connection at draft version one.
type CreateRequest struct {
	ID             string
	Direction      Direction
	Kind           Kind
	Name           string
	Description    string
	Spec           json.RawMessage
	SecretBindings []integration.SecretBinding
	Reason         string
}

// UpdateRequest changes a draft's mutable fields. A nil field keeps the stored
// value; a non-nil SecretBindings replaces the list, even with an empty one.
type UpdateRequest struct {
	ID              string
	ExpectedVersion int64
	Name            *string
	Description     *string
	Spec            json.RawMessage
	SecretBindings  *[]integration.SecretBinding
	Reason          string
}

// CommandRequest targets one draft version for archive or compile.
type CommandRequest struct {
	ID              string
	ExpectedVersion int64
	Reason          string
}

// ValidateRequest is a spec checked without any write.
type ValidateRequest struct {
	Kind           Kind
	Spec           json.RawMessage
	SecretBindings []integration.SecretBinding
}

// List returns the caller's tenant's connections in ID order, bounded by
// MaxConnections.
func (s *Service) List(ctx context.Context, filter ListFilter) ([]Connection, error) {
	security, err := s.authorize(ctx, ReadRole)
	if err != nil {
		return nil, err
	}
	if filter.Direction != "" && !filter.Direction.Valid() {
		return nil, ErrInvalidRequest
	}
	drafts, err := s.store.ListDrafts(ctx, security.TenantID, filter.Direction, filter.IncludeArchived)
	if err != nil {
		return nil, err
	}
	connections := make([]Connection, 0, len(drafts))
	if len(drafts) == 0 {
		return connections, nil
	}
	latest, err := s.store.LatestRevisions(ctx, security.TenantID)
	if err != nil {
		return nil, err
	}
	digests, err := s.store.RevisionDigests(ctx, security.TenantID, "")
	if err != nil {
		return nil, err
	}
	references, err := s.referencesByDigest(ctx, security.TenantID, len(digests) > 0)
	if err != nil {
		return nil, err
	}
	mounted := s.runtime.MountedDigests()
	for _, draft := range drafts {
		var latestRevision *Revision
		if revision, ok := latest[draft.ID]; ok {
			latestRevision = &revision
		}
		connections = append(connections, project(draft, latestRevision, digests[draft.ID], references, mounted))
	}
	return connections, nil
}

// Get returns one connection of the caller's tenant, or ErrNotFound.
func (s *Service) Get(ctx context.Context, id string) (Connection, error) {
	security, err := s.authorize(ctx, ReadRole)
	if err != nil {
		return Connection{}, err
	}
	if !validConnectionID(id) {
		return Connection{}, ErrNotFound
	}
	draft, err := s.store.GetDraft(ctx, security.TenantID, id)
	if err != nil {
		return Connection{}, err
	}
	return s.connection(ctx, draft)
}

// connection projects one stored draft with its latest revision, references,
// and runtime state.
func (s *Service) connection(ctx context.Context, draft Draft) (Connection, error) {
	latest, err := s.store.LatestRevision(ctx, draft.TenantID, draft.ID)
	if err != nil {
		return Connection{}, err
	}
	digests, err := s.store.RevisionDigests(ctx, draft.TenantID, draft.ID)
	if err != nil {
		return Connection{}, err
	}
	references, err := s.referencesByDigest(ctx, draft.TenantID, len(digests[draft.ID]) > 0)
	if err != nil {
		return Connection{}, err
	}
	return project(draft, latest, digests[draft.ID], references, s.runtime.MountedDigests()), nil
}

// ListRevisions returns one connection's revisions, newest first. A connection
// that does not exist — or is another tenant's — has none.
func (s *Service) ListRevisions(ctx context.Context, id string) ([]Revision, error) {
	security, err := s.authorize(ctx, ReadRole)
	if err != nil {
		return nil, err
	}
	if !validConnectionID(id) {
		return []Revision{}, nil
	}
	return s.store.ListRevisions(ctx, security.TenantID, id)
}

// GetRevision returns one revision of the caller's tenant, or ErrNotFound.
func (s *Service) GetRevision(ctx context.Context, artifactID, revisionID string) (Revision, error) {
	security, err := s.authorize(ctx, ReadRole)
	if err != nil {
		return Revision{}, err
	}
	if !validConnectionID(artifactID) || !validIdentity(revisionID) {
		return Revision{}, ErrNotFound
	}
	return s.store.GetRevision(ctx, security.TenantID, artifactID, revisionID)
}

// Create registers a new draft. The spec may be incomplete — compile reports
// what is missing — but it may never carry a secret value.
func (s *Service) Create(ctx context.Context, request CreateRequest) (Connection, error) {
	security, err := s.authorize(ctx, ReadRole, WriteRole)
	if err != nil {
		return Connection{}, err
	}
	reason, err := validateWrite(request.ID, request.Reason)
	if err != nil {
		return Connection{}, err
	}
	direction, ok := request.Kind.Direction()
	if !ok || request.Direction != direction {
		return Connection{}, ErrInvalidRequest
	}
	if !validName(request.Name) || !validDescription(request.Description) {
		return Connection{}, ErrInvalidRequest
	}
	spec, err := writableSpec(request.Spec, request.SecretBindings)
	if err != nil {
		return Connection{}, err
	}
	audit := s.audit(security, reason)
	draft, err := s.store.CreateDraft(ctx, Draft{
		TenantID:       security.TenantID,
		ID:             request.ID,
		Direction:      direction,
		Kind:           request.Kind,
		Name:           request.Name,
		Description:    request.Description,
		Spec:           spec,
		SecretBindings: cloneBindings(request.SecretBindings),
		Created:        audit,
	})
	if err != nil {
		return Connection{}, err
	}
	return s.connection(ctx, draft)
}

// Update changes a draft under its expected version.
func (s *Service) Update(ctx context.Context, request UpdateRequest) (Connection, error) {
	security, err := s.authorize(ctx, ReadRole, WriteRole)
	if err != nil {
		return Connection{}, err
	}
	reason, err := validateWrite(request.ID, request.Reason)
	if err != nil {
		return Connection{}, err
	}
	if request.ExpectedVersion <= 0 ||
		(request.Name != nil && !validName(*request.Name)) ||
		(request.Description != nil && !validDescription(*request.Description)) {
		return Connection{}, ErrInvalidRequest
	}
	draft, err := s.store.GetDraft(ctx, security.TenantID, request.ID)
	if err != nil {
		return Connection{}, err
	}
	if draft.Archived() {
		return Connection{}, ErrArchived
	}
	if draft.Version != request.ExpectedVersion {
		return Connection{}, ErrVersionConflict
	}
	if request.Name != nil {
		draft.Name = *request.Name
	}
	if request.Description != nil {
		draft.Description = *request.Description
	}
	if request.SecretBindings != nil {
		draft.SecretBindings = cloneBindings(*request.SecretBindings)
	}
	if request.Spec != nil {
		draft.Spec = request.Spec
	}
	spec, err := writableSpec(draft.Spec, draft.SecretBindings)
	if err != nil {
		return Connection{}, err
	}
	draft.Spec = spec
	draft.Updated = s.audit(security, reason)
	updated, err := s.store.UpdateDraft(ctx, draft, request.ExpectedVersion)
	if err != nil {
		return Connection{}, err
	}
	return s.connection(ctx, updated)
}

// Archive freezes a draft under its expected version. Its revisions stay
// readable: a definition that references one keeps resolving it.
func (s *Service) Archive(ctx context.Context, request CommandRequest) (Connection, error) {
	security, err := s.authorize(ctx, ReadRole, WriteRole)
	if err != nil {
		return Connection{}, err
	}
	reason, err := validateWrite(request.ID, request.Reason)
	if err != nil {
		return Connection{}, err
	}
	if request.ExpectedVersion <= 0 {
		return Connection{}, ErrInvalidRequest
	}
	draft, err := s.store.ArchiveDraft(ctx, security.TenantID, request.ID, request.ExpectedVersion, s.audit(security, reason))
	if err != nil {
		return Connection{}, err
	}
	return s.connection(ctx, draft)
}

// Compile turns the current draft into an immutable revision with the kind's
// own constructor. A blocking problem returns the problems, a nil revision,
// and writes nothing. Compiling a draft whose current version was already
// compiled returns that revision and writes nothing either.
func (s *Service) Compile(ctx context.Context, request CommandRequest) (CompileResult, error) {
	security, err := s.authorize(ctx, ReadRole, WriteRole)
	if err != nil {
		return CompileResult{}, err
	}
	reason, err := validateWrite(request.ID, request.Reason)
	if err != nil {
		return CompileResult{}, err
	}
	if request.ExpectedVersion <= 0 {
		return CompileResult{}, ErrInvalidRequest
	}
	draft, err := s.store.GetDraft(ctx, security.TenantID, request.ID)
	if err != nil {
		return CompileResult{}, err
	}
	if draft.Archived() {
		return CompileResult{}, ErrArchived
	}
	if draft.Version != request.ExpectedVersion {
		return CompileResult{}, ErrVersionConflict
	}
	latest, err := s.store.LatestRevision(ctx, security.TenantID, draft.ID)
	if err != nil {
		return CompileResult{}, err
	}
	if latest != nil && latest.CompiledFromVersion == draft.Version {
		connection, err := s.connection(ctx, draft)
		if err != nil {
			return CompileResult{}, err
		}
		return CompileResult{
			Connection: connection,
			Revision:   latest,
			Problems:   CheckSpec(draft.Kind, draft.Spec, draft.SecretBindings),
		}, nil
	}
	number := int64(1)
	if latest != nil {
		number = latest.Number + 1
	}
	revision, problems := BuildRevision(draft, number, s.audit(security, reason))
	if revision == nil {
		connection, err := s.connection(ctx, draft)
		if err != nil {
			return CompileResult{}, err
		}
		return CompileResult{Connection: connection, Problems: problems}, nil
	}
	stored, err := s.store.InsertRevision(ctx, *revision)
	if err != nil {
		return CompileResult{}, err
	}
	connection, err := s.connection(ctx, draft)
	if err != nil {
		return CompileResult{}, err
	}
	return CompileResult{Connection: connection, Revision: &stored, Problems: problems}, nil
}

// ValidateSpec runs compile's checks with no write and no store read.
func (s *Service) ValidateSpec(ctx context.Context, request ValidateRequest) ([]Problem, error) {
	if _, err := s.authorize(ctx, ReadRole, WriteRole); err != nil {
		return nil, err
	}
	if !request.Kind.Valid() {
		return nil, ErrInvalidRequest
	}
	problems := CheckSpec(request.Kind, request.Spec, request.SecretBindings)
	if problems == nil {
		problems = []Problem{}
	}
	return problems, nil
}

func (s *Service) audit(security integration.SecurityContext, reason string) integration.AuditEnvelope {
	principal := security.Principal
	principal.Roles = append([]string(nil), security.Principal.Roles...)
	return integration.AuditEnvelope{
		TenantID:   security.TenantID,
		Principal:  principal,
		Reason:     reason,
		OccurredAt: s.store.Now(),
	}
}

// referencesByDigest maps every connection-revision digest a lifecycle
// definition revision names — as its source or as one of its destinations —
// to the definitions that name it. It reads at most maxDefinitionSnapshots
// snapshots. Without a lifecycle catalog, or when needed is false, it reads
// nothing and every connection has no references.
func (s *Service) referencesByDigest(ctx context.Context, tenantID string, needed bool) (map[string][]Reference, error) {
	references := make(map[string][]Reference)
	if s.catalog == nil || !needed {
		return references, nil
	}
	snapshots, err := s.catalog.ListSnapshots(ctx, tenantID, maxDefinitionSnapshots)
	if errors.Is(err, lifecycle.ErrUnavailable) {
		return references, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list lifecycle snapshots for connection references: %w", err)
	}
	for _, snapshot := range snapshots {
		raw, err := s.catalog.LoadDefinitionRevision(ctx, tenantID,
			snapshot.DefinitionRevision.ArtifactID, snapshot.DefinitionRevision.RevisionID)
		if errors.Is(err, lifecycle.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("load definition revision for connection references: %w", err)
		}
		var named namedDigests
		if err := json.Unmarshal(raw, &named); err != nil {
			return nil, fmt.Errorf("decode definition revision for connection references: %w", err)
		}
		for _, digest := range named.all() {
			references[digest] = append(references[digest], Reference{
				DefinitionID: snapshot.DefinitionRevision.ArtifactID,
				RevisionID:   snapshot.DefinitionRevision.RevisionID,
				Digest:       digest,
				State:        string(snapshot.State),
				Health:       string(snapshot.Health),
			})
		}
	}
	return references, nil
}

// digestRef is the one member of an artifact reference the projection reads.
type digestRef struct {
	Digest string `json:"digest"`
}

// namedDigests is the part of an integration definition revision that names
// connection revisions. Only these two digest sets matter here; the lifecycle
// catalog validated the whole document when its draft was created.
type namedDigests struct {
	Source       digestRef   `json:"source"`
	Destinations []digestRef `json:"destinations"`
}

func (n namedDigests) all() []string {
	digests := make([]string, 0, 1+len(n.Destinations))
	if n.Source.Digest != "" {
		digests = append(digests, n.Source.Digest)
	}
	for _, destination := range n.Destinations {
		if destination.Digest != "" {
			digests = append(digests, destination.Digest)
		}
	}
	return digests
}

// project assembles the read view of one draft. revisions must be ordered
// newest first.
func project(draft Draft, latest *Revision, revisions []RevisionDigest, references map[string][]Reference, mounted map[string]MountedDigest) Connection {
	connection := Connection{
		Draft:          draft,
		LatestRevision: latest,
		References:     []Reference{},
		Runtime:        runtimeStateFor(revisions, mounted),
	}
	for _, revision := range revisions {
		connection.References = append(connection.References, references[revision.Digest]...)
	}
	return connection
}

// validateWrite checks the two things every write carries: a catalog ID and
// a reason. It returns the trimmed reason.
func validateWrite(id, reason string) (string, error) {
	trimmed := strings.TrimSpace(reason)
	if !validConnectionID(id) || trimmed == "" || len(trimmed) > MaxReasonBytes || !printable(trimmed, true) {
		return "", ErrInvalidRequest
	}
	return trimmed, nil
}

func validName(name string) bool {
	return name != "" && len(name) <= maxNameBytes && strings.TrimSpace(name) == name && printable(name, false)
}

func validDescription(description string) bool {
	return len(description) <= maxDescriptionBytes && printable(description, true)
}

// printable refuses control characters; multiline allows newline and tab.
func printable(value string, multiline bool) bool {
	for _, character := range value {
		if multiline && (character == '\n' || character == '\t') {
			continue
		}
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

// writableSpec is the write-time gate on a draft's spec: one JSON object,
// bounded, compacted, and free of secret material. Everything else about the
// spec is a compile-time Problem, so an incomplete draft can be saved.
func writableSpec(raw json.RawMessage, bindings []integration.SecretBinding) (json.RawMessage, error) {
	if len(bindings) > MaxSecretBindings {
		return nil, ErrInvalidRequest
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		trimmed = []byte("{}")
	}
	if len(trimmed) > MaxSpecBytes {
		return nil, ErrInvalidRequest
	}
	scratch := &checker{}
	if _, ok := decodeSpecTree(trimmed, scratch); !ok {
		return nil, ErrInvalidRequest
	}
	problems := SecretValueProblems(trimmed)
	for index, binding := range bindings {
		fields := []struct{ name, value string }{
			{"name", binding.Name}, {"key", binding.Reference.Key}, {"version", binding.Reference.Version},
		}
		for _, field := range fields {
			if strings.Contains(field.value, pemMarker) {
				problems = append(problems, Problem{
					Code: CodeSecretValueForbidden, Path: fmt.Sprintf("secret_bindings[%d].%s", index, field.name),
					Message: "a binding names a secret; it never carries certificate or key material",
				})
			}
		}
	}
	if len(problems) > 0 {
		return nil, &SpecError{Problems: problems}
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, trimmed); err != nil {
		return nil, ErrInvalidRequest
	}
	return compact.Bytes(), nil
}

func cloneBindings(bindings []integration.SecretBinding) []integration.SecretBinding {
	return append([]integration.SecretBinding{}, bindings...)
}
