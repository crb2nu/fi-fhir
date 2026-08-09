package destination

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

// Mode selects how a destination without a declared client identity is treated.
//
// The two modes are mutually exclusive settings that reject each other's
// configuration, exactly as the OIDC and static identity modes do. A deployment
// states which one it runs; there is no implicit fallback and no per-destination
// override.
type Mode string

const (
	// ModeStrict refuses to start when any destination in the deployed set is
	// unbound. Every dispatch then runs under a destination-declared subject.
	ModeStrict Mode = "strict"
	// ModeCompatibility authorizes unbound destinations under one explicit,
	// server-issued compatibility grant, so a deployment can adopt the contract
	// before every destination revision declares an identity.
	ModeCompatibility Mode = "compatibility"
)

const registryDocumentSchema = "fi-fhir/destination-registry/v1"

var (
	// ErrInvalidRegistry means the destination registry document is malformed,
	// internally inconsistent, or illegal for the configured mode.
	ErrInvalidRegistry = errors.New("invalid destination registry")
	// ErrDestinationUnknown means the attempt names a destination that is not in
	// the deployed destination set.
	ErrDestinationUnknown = errors.New("destination is not in the deployed revision")
	// ErrDestinationUnverified means the attempt's destination reference does not
	// match the deployed revision byte for byte.
	ErrDestinationUnverified = errors.New("destination revision digest is unverified")
)

// registryDocument is the server-owned destination set for one tenant and one
// integration revision. It is loaded like the static integration registry and
// the lifecycle release: never authored over GraphQL, never sender-supplied.
type registryDocument struct {
	Schema              string                          `json:"schema"`
	TenantID            string                          `json:"tenant_id"`
	IntegrationRevision integration.ArtifactRevisionRef `json:"integration_revision"`
	SecretBindings      []integration.SecretBinding     `json:"secret_bindings,omitempty"`
	Destinations        []Revision                      `json:"destinations"`
}

// Registry resolves a delivery attempt's destination reference to the exact
// deployed destination revision, or refuses.
type Registry struct {
	mode                Mode
	tenantID            string
	integrationRevision integration.ArtifactRevisionRef
	secretBindings      []integration.SecretBinding
	byArtifactID        map[string]Revision
}

// LoadRegistry decodes and validates one destination registry document.
//
// In ModeStrict a document containing any unbound destination is rejected at
// load, so a strict deployment fails startup rather than silently authorizing an
// unbound destination later. In ModeCompatibility both bound and unbound
// destinations are accepted.
func LoadRegistry(reader io.Reader, mode Mode) (*Registry, error) {
	if reader == nil || (mode != ModeStrict && mode != ModeCompatibility) {
		return nil, ErrInvalidRegistry
	}
	raw, err := io.ReadAll(io.LimitReader(reader, maxRevisionSize*maxDestinationsPer+1))
	if err != nil || len(raw) == 0 || rejectDuplicateJSONKeys(raw) != nil {
		return nil, ErrInvalidRegistry
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var document registryDocument
	if err := decoder.Decode(&document); err != nil {
		return nil, ErrInvalidRegistry
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, ErrInvalidRegistry
	}
	if document.Schema != registryDocumentSchema || !validIdentity(document.TenantID) ||
		!validIdentity(document.IntegrationRevision.ArtifactID) ||
		!validIdentity(document.IntegrationRevision.RevisionID) ||
		!validDigest(document.IntegrationRevision.Digest) ||
		len(document.Destinations) == 0 || len(document.Destinations) > maxDestinationsPer {
		return nil, ErrInvalidRegistry
	}
	if err := validateSecretBindings(document.SecretBindings); err != nil {
		return nil, err
	}
	registry := &Registry{
		mode:                mode,
		tenantID:            document.TenantID,
		integrationRevision: document.IntegrationRevision,
		secretBindings:      append([]integration.SecretBinding(nil), document.SecretBindings...),
		byArtifactID:        make(map[string]Revision, len(document.Destinations)),
	}
	seenDestinationID := make(map[string]struct{}, len(document.Destinations))
	for _, revision := range document.Destinations {
		if revision.Validate() != nil {
			return nil, ErrInvalidRegistry
		}
		if _, duplicate := registry.byArtifactID[revision.ArtifactID]; duplicate {
			return nil, ErrInvalidRegistry
		}
		if _, duplicate := seenDestinationID[revision.DestinationID]; duplicate {
			return nil, ErrInvalidRegistry
		}
		if mode == ModeStrict && !revision.IdentityBound() {
			return nil, ErrInvalidRegistry
		}
		// Same discipline as mllp.SourceRevision and batch.SourceRevision: every
		// binding a destination names must be present in the deployment's binding
		// set. A destination that names a credential nobody declared is a
		// configuration error, not a runtime surprise.
		for _, name := range revision.SecretBindingNames() {
			if !hasSecretBinding(document.SecretBindings, name) {
				return nil, ErrInvalidRegistry
			}
		}
		seenDestinationID[revision.DestinationID] = struct{}{}
		registry.byArtifactID[revision.ArtifactID] = revision
	}
	return registry, nil
}

func validateSecretBindings(bindings []integration.SecretBinding) error {
	seen := make(map[string]struct{}, len(bindings))
	for _, binding := range bindings {
		if !validIdentity(binding.Name) ||
			integration.ValidateSecretReference(binding.Reference) != nil {
			return ErrInvalidRegistry
		}
		if _, duplicate := seen[binding.Name]; duplicate {
			return ErrInvalidRegistry
		}
		seen[binding.Name] = struct{}{}
	}
	return nil
}

// Mode reports the configured identity mode.
func (r *Registry) Mode() Mode {
	if r == nil {
		return ""
	}
	return r.mode
}

// TenantID reports the tenant this destination set belongs to.
func (r *Registry) TenantID() string {
	if r == nil {
		return ""
	}
	return r.tenantID
}

// IntegrationRevision reports the integration revision this destination set was
// deployed under. It is the authorization object's integration revision; nothing
// on the attempt can assert a different one.
func (r *Registry) IntegrationRevision() integration.ArtifactRevisionRef {
	if r == nil {
		return integration.ArtifactRevisionRef{}
	}
	return r.integrationRevision
}

// Resolve returns the deployed destination revision for one attempt reference.
//
// Resolution is by artifact ID, and the deployed revision's own reference must
// then equal the attempt's reference exactly — revision ID, digest, and class
// included. An attempt carrying another destination's digest therefore fails
// here rather than being published under the wrong identity.
func (r *Registry) Resolve(
	tenantID string,
	reference integration.DestinationRevisionRef,
) (Revision, error) {
	if r == nil || r.byArtifactID == nil {
		return Revision{}, ErrInvalidRegistry
	}
	if tenantID == "" || tenantID != r.tenantID {
		return Revision{}, ErrDestinationUnknown
	}
	revision, found := r.byArtifactID[reference.ArtifactID]
	if !found {
		return Revision{}, ErrDestinationUnknown
	}
	if revision.Reference() != reference {
		return Revision{}, ErrDestinationUnverified
	}
	if revision.Validate() != nil {
		return Revision{}, ErrDestinationUnverified
	}
	return revision, nil
}

// HasTransport reports whether the deployed destination set contains at least
// one destination declaring the given transport.
//
// It exists so startup can decide whether this deployment needs a destination
// transport at all, without cmd/ having to iterate revisions to find out.
func (r *Registry) HasTransport(kind TransportKind) bool {
	if r == nil {
		return false
	}
	for _, revision := range r.byArtifactID {
		if revision.Transport == kind {
			return true
		}
	}
	return false
}

// SecretBindings returns the deployment's binding set: names paired with
// references. It carries no material, so it is safe to hold and to log the names
// of. Startup resolves each reference once to prove the credential exists.
func (r *Registry) SecretBindings() []integration.SecretBinding {
	if r == nil {
		return nil
	}
	return append([]integration.SecretBinding(nil), r.secretBindings...)
}

// Destinations returns the deployed destination set in artifact-ID order for
// startup reporting. It never exposes secret material because a revision holds
// only binding names.
func (r *Registry) Destinations() []Revision {
	if r == nil {
		return nil
	}
	revisions := make([]Revision, 0, len(r.byArtifactID))
	for _, revision := range r.byArtifactID {
		revisions = append(revisions, revision)
	}
	sortRevisions(revisions)
	return revisions
}

func sortRevisions(revisions []Revision) {
	for outer := 1; outer < len(revisions); outer++ {
		current := revisions[outer]
		inner := outer - 1
		for inner >= 0 && revisions[inner].ArtifactID > current.ArtifactID {
			revisions[inner+1] = revisions[inner]
			inner--
		}
		revisions[inner+1] = current
	}
}

func validDigest(value string) bool {
	if len(value) != len("sha256:")+64 || value[:7] != "sha256:" {
		return false
	}
	for _, character := range value[7:] {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}
