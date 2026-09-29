package authoring

import (
	"context"
	"errors"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/connection"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

// CatalogFacts are the facts a STATIC validation reads in `serve`: this
// replica's runtime description, the observations ledger, and the connection
// catalog's compiled revisions. `serve` builds its lifecycle catalog before
// the connection service exists, so the fields are set late (Bind); until
// then a static check finds nothing mounted and the names unknown.
type CatalogFacts struct {
	connections *connection.Service
	description *connection.RuntimeDescription
}

// Bind hands the facts the connection catalog and the runtime description.
// Call it during composition, before the service answers a request.
func (f *CatalogFacts) Bind(connections *connection.Service, description *connection.RuntimeDescription) {
	f.connections = connections
	f.description = description
}

// Checks returns the StaticChecks over these facts. Secret values are not
// resolved: adapters load their own material at startup.
func (f *CatalogFacts) Checks() StaticChecks {
	return StaticChecks{SourceMounted: f.SourceMounted, SourceBindingNames: f.SourceBindingNames}
}

// SourceMounted reports whether this replica mounts the exact source digest,
// or a replica with a fresh heartbeat reports it in the observations ledger.
func (f *CatalogFacts) SourceMounted(ctx context.Context, source integration.ArtifactRevisionRef) (bool, error) {
	if _, mounted := f.description.MountedDigests()[source.Digest]; mounted {
		return true, nil
	}
	if f.connections == nil {
		return false, nil
	}
	observations, err := f.connections.Observations(ctx)
	if err != nil {
		return false, err
	}
	for _, observation := range observations {
		if !observation.Stale && observation.Digest == source.Digest {
			return true, nil
		}
	}
	return false, nil
}

// SourceBindingNames returns the binding names the exact compiled source
// revision requires; found is false when the catalog does not hold that
// revision with that digest, or its stored document no longer decodes.
func (f *CatalogFacts) SourceBindingNames(ctx context.Context, source integration.ArtifactRevisionRef) ([]string, bool, error) {
	if f.connections == nil {
		return nil, false, nil
	}
	revision, err := f.connections.GetRevision(ctx, source.ArtifactID, source.RevisionID)
	if errors.Is(err, connection.ErrNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if revision.Digest != source.Digest {
		return nil, false, nil
	}
	if described, decodeErr := SourceFromDocument(revision.Kind, revision.Document); decodeErr == nil {
		return described.BindingNames, true, nil
	}
	return nil, false, nil
}
