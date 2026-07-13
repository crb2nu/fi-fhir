package processor

import (
	"bytes"
	"context"
	"errors"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

var (
	// ErrDefinitionRevisionNotFound means the exact requested deployment artifact is absent.
	ErrDefinitionRevisionNotFound = errors.New("integration definition revision not found")
	// ErrDefinitionRevisionUnavailable means the server-owned definition catalog could not be read.
	ErrDefinitionRevisionUnavailable = errors.New("integration definition revision unavailable")
	// ErrInvalidDefinitionRevisionReference means the caller's requested identity is malformed.
	ErrInvalidDefinitionRevisionReference = errors.New("invalid integration definition revision reference")
	// ErrInvalidDefinitionRevisionContent means stored server bytes failed strict contract decoding.
	ErrInvalidDefinitionRevisionContent = errors.New("invalid integration definition revision content")
	// ErrDefinitionRevisionReferenceMismatch means stored bytes do not equal the exact requested reference.
	ErrDefinitionRevisionReferenceMismatch = errors.New("integration definition revision reference mismatch")
)

const maxExecutableDefinitionJSONBytes = 1 << 20

// DefinitionRevisionLoader loads exact server-owned integration definition JSON.
// Implementations must scope lookup by all supplied tenant, definition, and revision IDs.
type DefinitionRevisionLoader interface {
	LoadDefinitionRevision(ctx context.Context, tenantID, definitionID, revisionID string) ([]byte, error)
}

// DefinitionRevisionResolver resolves executable definitions inside one deployment tenant.
type DefinitionRevisionResolver struct {
	deploymentTenantID string
	loader             DefinitionRevisionLoader
}

// NewDefinitionRevisionResolver constructs a deployment-tenant-bound definition resolver.
func NewDefinitionRevisionResolver(deploymentTenantID string, loader DefinitionRevisionLoader) (*DefinitionRevisionResolver, error) {
	if err := validateIdentity("deployment tenant ID", deploymentTenantID); err != nil {
		return nil, ErrTenantMismatch
	}
	if loader == nil {
		return nil, ErrDefinitionRevisionUnavailable
	}
	return &DefinitionRevisionResolver{
		deploymentTenantID: deploymentTenantID,
		loader:             loader,
	}, nil
}

// Resolve strictly decodes the exact server-owned revision requested by the caller.
// Loader and decoder details are deliberately mapped to bounded catalog errors.
func (r *DefinitionRevisionResolver) Resolve(
	ctx context.Context,
	tenantID string,
	requested integration.ArtifactRevisionRef,
) (integration.IntegrationDefinitionRevision, error) {
	if r == nil || r.loader == nil {
		return integration.IntegrationDefinitionRevision{}, ErrDefinitionRevisionUnavailable
	}
	if tenantID != r.deploymentTenantID {
		return integration.IntegrationDefinitionRevision{}, ErrTenantMismatch
	}
	if !validDefinitionRevisionReference(requested) {
		return integration.IntegrationDefinitionRevision{}, ErrInvalidDefinitionRevisionReference
	}

	raw, err := r.loader.LoadDefinitionRevision(
		ctx,
		r.deploymentTenantID,
		requested.ArtifactID,
		requested.RevisionID,
	)
	if err != nil {
		switch {
		case errors.Is(err, context.Canceled):
			return integration.IntegrationDefinitionRevision{}, context.Canceled
		case errors.Is(err, context.DeadlineExceeded):
			return integration.IntegrationDefinitionRevision{}, context.DeadlineExceeded
		case errors.Is(err, ErrDefinitionRevisionNotFound):
			return integration.IntegrationDefinitionRevision{}, ErrDefinitionRevisionNotFound
		default:
			return integration.IntegrationDefinitionRevision{}, ErrDefinitionRevisionUnavailable
		}
	}
	if len(raw) == 0 || len(raw) > maxExecutableDefinitionJSONBytes {
		return integration.IntegrationDefinitionRevision{}, ErrInvalidDefinitionRevisionContent
	}

	trustedJSON := bytes.Clone(raw)
	revision, err := integration.DecodeIntegrationDefinitionRevision(bytes.NewReader(trustedJSON))
	if err != nil {
		return integration.IntegrationDefinitionRevision{}, ErrInvalidDefinitionRevisionContent
	}
	if revision.TenantID != r.deploymentTenantID {
		return integration.IntegrationDefinitionRevision{}, ErrTenantMismatch
	}
	if revision.Reference() != requested {
		return integration.IntegrationDefinitionRevision{}, ErrDefinitionRevisionReferenceMismatch
	}
	return revision, nil
}

func validDefinitionRevisionReference(ref integration.ArtifactRevisionRef) bool {
	if validateIdentity("definition ID", ref.ArtifactID) != nil {
		return false
	}
	if validateIdentity("definition revision ID", ref.RevisionID) != nil {
		return false
	}
	return validateDigest(ref.Digest) == nil
}
