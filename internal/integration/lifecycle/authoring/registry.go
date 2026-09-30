package authoring

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/processor"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/registry"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

// ErrRegistryUnavailable means no static integration registry is loaded, so
// no profile or workflow ref can be proven.
var ErrRegistryUnavailable = errors.New("static integration registry unavailable")

// ErrUnknownIntegration means the registry has no entry with that ID.
var ErrUnknownIntegration = errors.New("integration is not in the integration registry")

// RegistryArtifact is one static-registry integration's profile and workflow
// refs, proven with the runtime's own resolver. It is the honest source of
// refs a definition may bind until resolution moves onto the catalog
// (.loom/39, .loom/41).
type RegistryArtifact struct {
	IntegrationID string
	Profile       integration.ArtifactRevisionRef
	Workflow      integration.ArtifactRevisionRef
	SourceID      string
	Format        events.SourceFormat
	// WorkflowYAML is the exact resolved workflow, for the planner rule. It
	// never leaves the server.
	WorkflowYAML []byte
}

// Registry proves profile and workflow refs against one static registry with
// the processor.RevisionResolver `serve` and the batch runner use.
type Registry struct {
	tenantID string
	static   *registry.StaticRegistry
	resolver *processor.RevisionResolver
}

// NewRegistry binds a static registry to the deployment tenant.
func NewRegistry(tenantID string, static *registry.StaticRegistry) (*Registry, error) {
	if static == nil {
		return nil, ErrRegistryUnavailable
	}
	if static.DeploymentTenantID() != tenantID {
		return nil, fmt.Errorf("integration registry tenant does not match deployment tenant %q", tenantID)
	}
	resolver, err := processor.NewRevisionResolver(tenantID, static)
	if err != nil {
		return nil, fmt.Errorf("configure artifact resolver: %w", err)
	}
	return &Registry{tenantID: tenantID, static: static, resolver: resolver}, nil
}

// Artifact returns one registry entry's refs after resolving them exactly as
// the runtime does. Only HL7v2 entries are offered: every durable adapter
// admits only HL7v2.
func (r *Registry) Artifact(ctx context.Context, integrationID string) (RegistryArtifact, error) {
	if r == nil || r.static == nil {
		return RegistryArtifact{}, ErrRegistryUnavailable
	}
	binding, err := r.static.LookupPreviewBinding(ctx, r.tenantID, integrationID)
	if err != nil {
		return RegistryArtifact{}, fmt.Errorf("%w: %q", ErrUnknownIntegration, integrationID)
	}
	raw, err := r.static.LoadDefinitionRevision(
		ctx, r.tenantID, binding.IntegrationRevision.ArtifactID, binding.IntegrationRevision.RevisionID,
	)
	if err != nil {
		return RegistryArtifact{}, fmt.Errorf("load registry definition for %q: %w", integrationID, err)
	}
	entry, err := integration.DecodeIntegrationDefinitionRevision(bytes.NewReader(raw))
	if err != nil {
		return RegistryArtifact{}, fmt.Errorf("decode registry definition for %q: %w", integrationID, err)
	}
	if entry.Format != events.FormatHL7v2 {
		return RegistryArtifact{}, fmt.Errorf("integration %q is format %q; the durable adapters admit only %q", integrationID, entry.Format, events.FormatHL7v2)
	}
	resolved, err := r.resolver.Resolve(ctx, r.tenantID, entry.Profile, entry.Workflow)
	if err != nil {
		return RegistryArtifact{}, fmt.Errorf("registry artifacts for %q do not resolve as the runtime resolves them: %w", integrationID, err)
	}
	return RegistryArtifact{
		IntegrationID: integrationID,
		Profile:       resolved.ProfileReference(),
		Workflow:      resolved.WorkflowReference(),
		SourceID:      entry.Source.SourceID,
		Format:        entry.Format,
		WorkflowYAML:  resolved.WorkflowYAML(),
	}, nil
}

// Artifacts lists every registry entry that resolves, in integration-ID
// order. An entry that does not resolve is omitted, because a definition
// could never bind it.
func (r *Registry) Artifacts(ctx context.Context) ([]RegistryArtifact, error) {
	if r == nil || r.static == nil {
		return nil, ErrRegistryUnavailable
	}
	artifacts := make([]RegistryArtifact, 0)
	for _, summary := range r.static.Integrations() {
		artifact, err := r.Artifact(ctx, summary.IntegrationID)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			continue
		}
		artifacts = append(artifacts, artifact)
	}
	return artifacts, nil
}

// ErrMixedPair means the profile and workflow refs do not come from one
// registry entry, as the seed always takes them.
var ErrMixedPair = errors.New("profile and workflow refs are not one registry entry's pair")

// Resolve proves one profile/workflow pair — both refs from the same registry
// entry, resolved as the runtime resolves them — and returns the workflow bytes.
func (r *Registry) Resolve(
	ctx context.Context,
	profile, workflowRef integration.ArtifactRevisionRef,
) ([]byte, error) {
	if r == nil || r.resolver == nil {
		return nil, ErrRegistryUnavailable
	}
	for _, summary := range r.static.Integrations() {
		artifact, err := r.Artifact(ctx, summary.IntegrationID)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			continue
		}
		if artifact.Profile == profile && artifact.Workflow == workflowRef {
			return artifact.WorkflowYAML, nil
		}
	}
	return nil, ErrMixedPair
}
