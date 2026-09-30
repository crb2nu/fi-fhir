// Package authoring builds, checks, and validates integration definition
// revisions for the lifecycle catalog. It is the one implementation behind
// both `fi-fhir lifecycle seed` and the GraphQL definition editor
// (.loom/42 E-1), so the two cannot author different bytes for the same
// inputs: the CLI assembles a Draft from files, the API assembles one from
// compiled connection revisions and the static registry, and both call
// BuildDefinition, Check, and the validators here.
//
// Where the profile and workflow refs come from, and why. The durable
// processor resolves a deployed definition from the catalog and then calls
// processor.RevisionResolver over the STATIC integration registry
// (FI_FHIR_INTEGRATION_REGISTRY_PATH) with the definition's profile and
// workflow refs. That resolver looks the bytes up by (artifact_id,
// revision_id), recomputes the domain-separated digest, and refuses unless it
// equals the ref byte for byte. A profile or workflow published only into the
// Studio's own stores is invisible to it. So every ref a definition carries is
// proven with that resolver before anything is written (RegistryArtifacts,
// Check).
package authoring

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	"strings"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/batch"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/connection"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/destination"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/mllp"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/workflow"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

// SourceKind is the runtime adapter family a source revision belongs to. It
// decides which connection validation is possible (.loom/42 Decision 3).
type SourceKind string

const (
	SourceKindMLLP  SourceKind = "mllp"
	SourceKindHTTP  SourceKind = "http"
	SourceKindBatch SourceKind = "batch"
)

// Source is the definition's source: the exact compiled revision, the
// runtime source identity it carries, and the secret binding names the
// revision's own ValidateAgainst requires. Never a secret value.
type Source struct {
	Ref          integration.ArtifactRevisionRef
	SourceID     string
	Kind         SourceKind
	BindingNames []string
}

// Destination is one destination revision the definition may deliver to.
type Destination struct {
	Ref          integration.DestinationRevisionRef
	BindingNames []string
}

// Draft is everything a definition revision binds, before it is
// content-addressed. A zero Policy classification defaults to phi and an
// empty raw retention mode to ephemeral, as the seed has always written.
type Draft struct {
	DefinitionID     string
	RevisionID       string
	ParentRevisionID string
	TenantID         string
	Source           Source
	Profile          integration.ArtifactRevisionRef
	Workflow         integration.ArtifactRevisionRef
	Destinations     []Destination
	SecretBindings   []integration.SecretBinding
	Policy           integration.IntegrationPolicy
	Deployment       integration.IntegrationDeploymentPolicy
	Created          integration.AuditEnvelope
}

// DefaultValidationMaxAgeSeconds is the evidence freshness the seed has
// always used and the API proposes unless FI_FHIR_LIFECYCLE_VALIDATION_MAX_AGE
// says otherwise.
const DefaultValidationMaxAgeSeconds = 300

// DefaultDeploymentPolicy is the policy `lifecycle seed` writes by default:
// validation timeout 5 s and max age maxAgeSeconds (DefaultValidationMaxAgeSeconds
// when zero); continuous schedule; health grace 5 s, interval 30 s, timeout
// 5 s, threshold 3; capacity in-flight 2, queued 10, 100 messages/s.
func DefaultDeploymentPolicy(maxAgeSeconds int64) integration.IntegrationDeploymentPolicy {
	if maxAgeSeconds <= 0 {
		maxAgeSeconds = DefaultValidationMaxAgeSeconds
	}
	return integration.IntegrationDeploymentPolicy{
		ConnectionValidation: integration.ConnectionValidationPolicy{TimeoutSeconds: 5, MaxAgeSeconds: maxAgeSeconds},
		Schedule:             integration.SchedulePolicy{Mode: integration.ScheduleModeContinuous},
		Health: integration.HealthPolicy{
			StartupGraceSeconds: 5, CheckIntervalSeconds: 30, TimeoutSeconds: 5, FailureThreshold: 3,
		},
		Capacity: integration.CapacityPolicy{MaxInFlight: 2, MaxQueued: 10, MaxMessagesPerSecond: 100},
	}
}

// DefaultPolicy is the data-handling policy every definition gets unless the
// author chose otherwise: PHI, raw bytes ephemeral.
func DefaultPolicy() integration.IntegrationPolicy {
	return integration.IntegrationPolicy{
		Classification: integration.DataClassificationPHI,
		RawRetention:   integration.RawRetentionPolicy{Mode: integration.RawRetentionModeEphemeral},
	}
}

// BuildDefinition content-addresses a draft. It is deterministic: equal
// drafts, including an equal creation audit, produce an equal digest. The
// result passes ValidateForDeployment or an error is returned.
func BuildDefinition(draft Draft) (integration.IntegrationDefinitionRevision, error) {
	destinations := make([]integration.DestinationRevisionRef, 0, len(draft.Destinations))
	for _, item := range draft.Destinations {
		destinations = append(destinations, item.Ref)
	}
	policy := draft.Policy
	if policy.Classification == "" {
		policy.Classification = integration.DataClassificationPHI
	}
	deployment := draft.Deployment
	revision, err := integration.NewIntegrationDefinitionRevision(integration.IntegrationDefinitionRevisionInput{
		DefinitionID:     draft.DefinitionID,
		RevisionID:       draft.RevisionID,
		ParentRevisionID: draft.ParentRevisionID,
		TenantID:         draft.TenantID,
		Source: integration.SourceRevisionRef{
			ArtifactRevisionRef: draft.Source.Ref, SourceID: draft.Source.SourceID,
		},
		Format:         events.FormatHL7v2,
		Profile:        draft.Profile,
		Workflow:       draft.Workflow,
		Destinations:   destinations,
		SecretBindings: sortedBindings(draft.SecretBindings),
		Policy:         policy,
		Deployment:     &deployment,
		Created:        draft.Created,
	})
	if err != nil {
		return integration.IntegrationDefinitionRevision{}, fmt.Errorf("build integration definition revision: %w", err)
	}
	if err := revision.ValidateForDeployment(); err != nil {
		return integration.IntegrationDefinitionRevision{}, fmt.Errorf("integration definition revision is not deployable: %w", err)
	}
	return revision, nil
}

// File-provider key prefixes the seed binds, as the batch proof does.
const (
	SourceFileKeyPrefix      = "batch/"
	DestinationFileKeyPrefix = "destinations/"
)

// FileBindings binds every name the source declares (file key batch/<name>)
// and every name a destination declares (file key destinations/<name>, the
// reference the destination registry carries). It is the seed's binding
// convention; the API takes explicit references instead.
func FileBindings(source Source, destinations []Destination) ([]integration.SecretBinding, error) {
	byName := make(map[string]integration.SecretBinding)
	add := func(name, prefix string) error {
		binding := integration.SecretBinding{
			Name:      name,
			Reference: integration.SecretReference{Provider: integration.SecretProviderFile, Key: prefix + name},
		}
		if existing, found := byName[name]; found && existing != binding {
			return fmt.Errorf("secret binding %q is declared by both the source and a destination; rename one of them", name)
		}
		byName[name] = binding
		return nil
	}
	for _, name := range source.BindingNames {
		if err := add(name, SourceFileKeyPrefix); err != nil {
			return nil, err
		}
	}
	for _, item := range destinations {
		for _, name := range item.BindingNames {
			if err := add(name, DestinationFileKeyPrefix); err != nil {
				return nil, err
			}
		}
	}
	bindings := make([]integration.SecretBinding, 0, len(byName))
	for _, binding := range byName {
		bindings = append(bindings, binding)
	}
	return sortedBindings(bindings), nil
}

func sortedBindings(bindings []integration.SecretBinding) []integration.SecretBinding {
	sorted := append([]integration.SecretBinding(nil), bindings...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })
	return sorted
}

// BatchSource describes a decoded batch source revision.
func BatchSource(source batch.SourceRevision) Source {
	return Source{
		Ref: source.Reference(), SourceID: source.SourceID, Kind: SourceKindBatch,
		BindingNames: nonNil(source.SecretBindingNames()),
	}
}

// MLLPSource describes a decoded MLLP source revision. A mutual-TLS listener
// requires its three certificate bindings (mllp.SourceRevision.ValidateAgainst).
func MLLPSource(source mllp.SourceRevision) Source {
	names := []string{}
	if source.TLS.Mode == mllp.TLSModeMutual {
		names = append(names, source.TLS.ServerCertificateBinding, source.TLS.ServerPrivateKeyBinding, source.TLS.ClientCABinding)
	}
	return Source{Ref: source.Reference(), SourceID: source.SourceID, Kind: SourceKindMLLP, BindingNames: names}
}

// HTTPSource describes a decoded HTTP ingress source revision. Bearer and
// HMAC sources name one credential binding; OAuth2 sources name none.
func HTTPSource(source connection.HTTPSourceRevision) Source {
	names := []string{}
	if source.CredentialBinding != "" {
		names = append(names, source.CredentialBinding)
	}
	return Source{Ref: source.Reference(), SourceID: source.SourceID, Kind: SourceKindHTTP, BindingNames: names}
}

// DestinationOf describes a decoded destination revision.
func DestinationOf(revision destination.Revision) Destination {
	return Destination{Ref: revision.Reference(), BindingNames: nonNil(revision.SecretBindingNames())}
}

// ErrNotSource and ErrNotDestination refuse a connection revision of the
// wrong direction.
var (
	ErrNotSource      = errors.New("connection revision is not a source")
	ErrNotDestination = errors.New("connection revision is not a destination")
)

// SourceFromDocument decodes one compiled source connection document with the
// kind's own Decode function — the bytes `serve` mounts — and describes it.
func SourceFromDocument(kind connection.Kind, document []byte) (Source, error) {
	reader := bytes.NewReader(document)
	switch kind {
	case connection.KindMLLP:
		revision, err := mllp.DecodeSourceRevision(reader)
		if err != nil {
			return Source{}, err
		}
		return MLLPSource(revision), nil
	case connection.KindHTTP:
		revision, err := connection.DecodeHTTPSourceRevision(reader)
		if err != nil {
			return Source{}, err
		}
		return HTTPSource(revision), nil
	case connection.KindBatchS3, connection.KindBatchSFTP:
		revision, err := batch.DecodeSourceRevision(reader)
		if err != nil {
			return Source{}, err
		}
		return BatchSource(revision), nil
	default:
		return Source{}, ErrNotSource
	}
}

// DestinationFromDocument decodes one compiled destination connection document.
func DestinationFromDocument(kind connection.Kind, document []byte) (Destination, error) {
	switch kind {
	case connection.KindHTTPS, connection.KindFHIR, connection.KindKafka:
	default:
		return Destination{}, ErrNotDestination
	}
	revision, err := destination.DecodeRevision(bytes.NewReader(document))
	if err != nil {
		return Destination{}, err
	}
	return DestinationOf(revision), nil
}

// RequireWorkflowDestinations mirrors the planner's binding rule
// (processor/workflow_plan.go): a log action names no destination and every
// other action names one of the definition's destination artifact IDs. A miss
// fails every matching message with ErrInvalidWorkflowPlan at runtime, so it
// is refused before a definition is written.
func RequireWorkflowDestinations(
	workflowRef integration.ArtifactRevisionRef,
	workflowYAML []byte,
	destinations map[string]struct{},
) error {
	published, err := workflow.ParsePublishedWorkflow(workflowYAML)
	if err != nil {
		return fmt.Errorf("registry workflow %s/%s is not executable: %w", workflowRef.ArtifactID, workflowRef.RevisionID, err)
	}
	given := make([]string, 0, len(destinations))
	for artifactID := range destinations {
		given = append(given, artifactID)
	}
	sort.Strings(given)
	for _, route := range published.Workflow().Routes {
		for _, action := range route.Actions {
			if action.Type == "log" {
				continue
			}
			if _, found := destinations[action.Destination]; !found {
				return &WorkflowDestinationError{
					Workflow: workflowRef, Route: route.Name, Action: action.ID,
					Destination: action.Destination, Given: given,
				}
			}
		}
	}
	return nil
}

// WorkflowDestinationError is the planner rule refusing a definition.
type WorkflowDestinationError struct {
	Workflow    integration.ArtifactRevisionRef
	Route       string
	Action      string
	Destination string
	Given       []string
}

func (e *WorkflowDestinationError) Error() string {
	return fmt.Sprintf(
		"workflow %s/%s route %q action %q delivers to destination %q, which is not among the --destination artifacts [%s]; the runtime planner would refuse every matching message, so choose a registry entry whose workflow routes to these destinations or add that destination",
		e.Workflow.ArtifactID, e.Workflow.RevisionID, e.Route, e.Action, e.Destination, strings.Join(e.Given, ", "),
	)
}

func nonNil(values []string) []string {
	if values == nil {
		return []string{}
	}
	return append([]string(nil), values...)
}
