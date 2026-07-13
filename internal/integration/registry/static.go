// Package registry provides immutable, server-owned integration bindings for
// preview activation before the durable deployment catalog ships in Slice 2.1.
package registry

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/processor"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

const (
	// MaxArtifactBytes bounds every immutable registry document independently.
	MaxArtifactBytes = 1 << 20
	// MaxRegistryBytes bounds the complete startup registry document.
	MaxRegistryBytes = 8 << 20
)

var (
	// ErrTenantMismatch prevents one deployment registry from crossing security domains.
	ErrTenantMismatch = errors.New("integration registry tenant mismatch")
	// ErrIntegrationNotFound hides registry inventory from callers.
	ErrIntegrationNotFound = errors.New("integration is not available")
	// ErrInvalidRegistry means server-owned activation data is not safe to execute.
	ErrInvalidRegistry = errors.New("invalid integration registry")
)

// Entry is one immutable activation bundle. The browser receives only
// IntegrationID; every executable byte and reference remains server-owned.
type Entry struct {
	IntegrationID  string
	DefinitionJSON []byte
	ProfileJSON    []byte
	WorkflowYAML   []byte
}

type registryDocument struct {
	TenantID     string                  `json:"tenant_id"`
	Integrations []registryDocumentEntry `json:"integrations"`
}

type registryDocumentEntry struct {
	IntegrationID string          `json:"integration_id"`
	Definition    json.RawMessage `json:"definition"`
	Profile       json.RawMessage `json:"profile"`
	Workflow      string          `json:"workflow"`
}

// PreviewBinding contains only trusted metadata needed to construct RawEnvelope.
type PreviewBinding struct {
	IntegrationRevision integration.ArtifactRevisionRef
	SourceID            string
	Format              events.SourceFormat
	Classification      integration.DataClassification
}

type definitionKey struct {
	tenantID   string
	definition string
	revisionID string
}

type artifactKey struct {
	artifactID string
	revisionID string
}

// StaticRegistry is immutable after construction and safe for concurrent reads.
type StaticRegistry struct {
	deploymentTenantID string
	bindings           map[string]PreviewBinding
	definitions        map[definitionKey][]byte
	profiles           map[artifactKey][]byte
	workflows          map[artifactKey][]byte
}

// NewStaticRegistry validates exact definition/profile/workflow provenance and
// seals defensive copies for one deployment tenant.
func NewStaticRegistry(deploymentTenantID string, entries []Entry) (*StaticRegistry, error) {
	if err := validateIdentity("deployment tenant ID", deploymentTenantID); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidRegistry, err)
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("%w: at least one integration is required", ErrInvalidRegistry)
	}
	registry := &StaticRegistry{
		deploymentTenantID: deploymentTenantID,
		bindings:           make(map[string]PreviewBinding, len(entries)),
		definitions:        make(map[definitionKey][]byte, len(entries)),
		profiles:           make(map[artifactKey][]byte, len(entries)),
		workflows:          make(map[artifactKey][]byte, len(entries)),
	}
	for _, entry := range entries {
		if err := registry.add(entry); err != nil {
			return nil, err
		}
	}
	return registry, nil
}

// DecodeStaticRegistry reads one bounded, strict startup registry document.
func DecodeStaticRegistry(reader io.Reader) (*StaticRegistry, error) {
	if reader == nil {
		return nil, fmt.Errorf("%w: registry reader is required", ErrInvalidRegistry)
	}
	raw, err := io.ReadAll(io.LimitReader(reader, MaxRegistryBytes+1))
	if err != nil {
		return nil, fmt.Errorf("%w: read registry", ErrInvalidRegistry)
	}
	if len(raw) == 0 || len(raw) > MaxRegistryBytes {
		return nil, fmt.Errorf("%w: registry must contain between 1 and %d bytes", ErrInvalidRegistry, MaxRegistryBytes)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var document registryDocument
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("%w: decode registry", ErrInvalidRegistry)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, fmt.Errorf("%w: registry contains trailing data", ErrInvalidRegistry)
	}
	entries := make([]Entry, 0, len(document.Integrations))
	for _, item := range document.Integrations {
		entries = append(entries, Entry{
			IntegrationID:  item.IntegrationID,
			DefinitionJSON: bytes.Clone(item.Definition),
			ProfileJSON:    bytes.Clone(item.Profile),
			WorkflowYAML:   []byte(item.Workflow),
		})
	}
	return NewStaticRegistry(document.TenantID, entries)
}

// DeploymentTenantID returns the immutable registry security-domain identity.
func (r *StaticRegistry) DeploymentTenantID() string {
	if r == nil {
		return ""
	}
	return r.deploymentTenantID
}

func (r *StaticRegistry) add(entry Entry) error {
	if err := validateIdentity("integration ID", entry.IntegrationID); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidRegistry, err)
	}
	if _, duplicate := r.bindings[entry.IntegrationID]; duplicate {
		return fmt.Errorf("%w: integration ID %q is duplicated", ErrInvalidRegistry, entry.IntegrationID)
	}
	if err := validateArtifactBytes("definition", entry.DefinitionJSON); err != nil {
		return err
	}
	if err := validateArtifactBytes("profile", entry.ProfileJSON); err != nil {
		return err
	}
	if err := validateArtifactBytes("workflow", entry.WorkflowYAML); err != nil {
		return err
	}

	revision, err := integration.DecodeIntegrationDefinitionRevision(bytes.NewReader(entry.DefinitionJSON))
	if err != nil {
		return fmt.Errorf("%w: definition %q is invalid", ErrInvalidRegistry, entry.IntegrationID)
	}
	if revision.TenantID != r.deploymentTenantID {
		return fmt.Errorf("%w: definition %q", ErrTenantMismatch, entry.IntegrationID)
	}
	profileRevisionID, err := strconv.Atoi(revision.Profile.RevisionID)
	if err != nil || profileRevisionID <= 0 || strconv.Itoa(profileRevisionID) != revision.Profile.RevisionID {
		return fmt.Errorf("%w: profile revision ID is not canonical", ErrInvalidRegistry)
	}
	profileRef, err := processor.NewProfileRevisionReference(revision.Profile.ArtifactID, profileRevisionID, entry.ProfileJSON)
	if err != nil || profileRef != revision.Profile {
		return fmt.Errorf("%w: profile revision does not match definition", ErrInvalidRegistry)
	}
	workflowRef, err := processor.NewWorkflowRevisionReference(revision.Workflow.ArtifactID, revision.Workflow.RevisionID, entry.WorkflowYAML)
	if err != nil || workflowRef != revision.Workflow {
		return fmt.Errorf("%w: workflow revision does not match definition", ErrInvalidRegistry)
	}

	definitionID := definitionKey{tenantID: revision.TenantID, definition: revision.DefinitionID, revisionID: revision.RevisionID}
	if err := storeExact(r.definitions, definitionID, entry.DefinitionJSON, "definition"); err != nil {
		return err
	}
	if err := storeExact(r.profiles, artifactKey{artifactID: revision.Profile.ArtifactID, revisionID: revision.Profile.RevisionID}, entry.ProfileJSON, "profile"); err != nil {
		return err
	}
	if err := storeExact(r.workflows, artifactKey{artifactID: revision.Workflow.ArtifactID, revisionID: revision.Workflow.RevisionID}, entry.WorkflowYAML, "workflow"); err != nil {
		return err
	}
	r.bindings[entry.IntegrationID] = PreviewBinding{
		IntegrationRevision: revision.Reference(),
		SourceID:            revision.Source.SourceID,
		Format:              revision.Format,
		Classification:      revision.Policy.Classification,
	}
	return nil
}

// LookupPreviewBinding selects trusted metadata by deployment tenant and public integration key.
func (r *StaticRegistry) LookupPreviewBinding(ctx context.Context, tenantID, integrationID string) (PreviewBinding, error) {
	if ctx == nil {
		return PreviewBinding{}, ErrIntegrationNotFound
	}
	if err := ctx.Err(); err != nil {
		return PreviewBinding{}, err
	}
	if r == nil || tenantID != r.deploymentTenantID {
		return PreviewBinding{}, ErrTenantMismatch
	}
	binding, found := r.bindings[integrationID]
	if !found {
		return PreviewBinding{}, ErrIntegrationNotFound
	}
	return binding, nil
}

// LoadDefinitionRevision implements processor.DefinitionRevisionLoader.
func (r *StaticRegistry) LoadDefinitionRevision(ctx context.Context, tenantID, definitionID, revisionID string) ([]byte, error) {
	if ctx == nil {
		return nil, processor.ErrDefinitionRevisionNotFound
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if r == nil || tenantID != r.deploymentTenantID {
		return nil, processor.ErrDefinitionRevisionNotFound
	}
	raw, found := r.definitions[definitionKey{tenantID: tenantID, definition: definitionID, revisionID: revisionID}]
	if !found {
		return nil, processor.ErrDefinitionRevisionNotFound
	}
	return bytes.Clone(raw), nil
}

// LoadProfileRevision implements processor.ArtifactRevisionLoader.
func (r *StaticRegistry) LoadProfileRevision(ctx context.Context, artifactID, revisionID string) ([]byte, error) {
	if r == nil {
		return nil, fmt.Errorf("profile revision not found")
	}
	return loadArtifact(ctx, r, r.profiles, artifactKey{artifactID: artifactID, revisionID: revisionID}, "profile")
}

// LoadWorkflowRevision implements processor.ArtifactRevisionLoader.
func (r *StaticRegistry) LoadWorkflowRevision(ctx context.Context, artifactID, revisionID string) ([]byte, error) {
	if r == nil {
		return nil, fmt.Errorf("workflow revision not found")
	}
	return loadArtifact(ctx, r, r.workflows, artifactKey{artifactID: artifactID, revisionID: revisionID}, "workflow")
}

func loadArtifact(ctx context.Context, registry *StaticRegistry, artifacts map[artifactKey][]byte, key artifactKey, kind string) ([]byte, error) {
	if ctx == nil || registry == nil {
		return nil, fmt.Errorf("%s revision not found", kind)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	raw, found := artifacts[key]
	if !found {
		return nil, fmt.Errorf("%s revision not found", kind)
	}
	return bytes.Clone(raw), nil
}

func storeExact[K comparable](target map[K][]byte, key K, raw []byte, kind string) error {
	if existing, found := target[key]; found {
		if !bytes.Equal(existing, raw) {
			return fmt.Errorf("%w: conflicting %s revision", ErrInvalidRegistry, kind)
		}
		return nil
	}
	target[key] = bytes.Clone(raw)
	return nil
}

func validateArtifactBytes(kind string, raw []byte) error {
	if len(raw) == 0 || len(raw) > MaxArtifactBytes {
		return fmt.Errorf("%w: %s must contain between 1 and %d bytes", ErrInvalidRegistry, kind, MaxArtifactBytes)
	}
	return nil
}

func validateIdentity(label, value string) error {
	if value == "" || len(value) > 256 || strings.TrimSpace(value) != value {
		return fmt.Errorf("%s is invalid", label)
	}
	for _, r := range value {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			return fmt.Errorf("%s is invalid", label)
		}
	}
	return nil
}
