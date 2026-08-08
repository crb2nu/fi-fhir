// Package destination owns the immutable, content-addressed contract for one
// delivery destination and the identity a durable dispatch presents on its
// behalf.
//
// Before Slice 4.1c-a a destination was only integration.DestinationRevisionRef
// — {artifact_id, revision_id, digest, class} — with no resolvable bytes behind
// the digest, no transport, and no credential binding. Nothing loaded
// destination content and nothing verified it, so the "digest" named an artifact
// that did not exist. This package supplies the missing artifact and gives the
// dispatch path something exact to verify against.
//
// No transport in this package contacts a destination. Slice 4.1c-a ships the
// contract and the authorization decision; the first durable HTTPS consumer is
// 4.1c-b. TestDeliveryDispatch_ContactsNoDestination is the boundary marker.
package destination

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strings"
	"unicode"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/lifecycle"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

const (
	// RevisionSchemaVersion pins the destination revision wire contract.
	RevisionSchemaVersion = "1"

	// AuthMethodClientIdentity attributes a dispatch to the canonical client
	// subject declared by the destination's immutable revision.
	AuthMethodClientIdentity = "destination-client-identity"
	// AuthMethodCompatibility attributes a dispatch to the deployment-fixed
	// compatibility subject used by unbound destinations.
	AuthMethodCompatibility = "destination-compatibility"

	maxRevisionSize    = 1 << 20
	maxIdentityGrants  = 16
	maxDestinationsPer = 256
	revisionDigestDom  = "fi-fhir/destination-revision/v1\x00"
)

// TransportKind names the delivery mechanism a destination declares.
type TransportKind string

const (
	// TransportKafka is the only transport the engine executes today: the
	// dispatcher publishes one command to the constant delivery topic and an
	// external consumer performs the destination call.
	TransportKafka TransportKind = "kafka"
	// TransportHTTPS declares a direct HTTPS destination. Slice 4.1c-a resolves,
	// verifies, and authorizes it but executes nothing; 4.1c-b adds the consumer.
	TransportHTTPS TransportKind = "https"
)

// Errors are deliberately coarse so a caller cannot probe the destination
// inventory by distinguishing failures.
var (
	// ErrInvalidRevision means the destination revision is malformed or its
	// digest does not cover its own semantic fields.
	ErrInvalidRevision = errors.New("invalid destination revision")
	// ErrRevisionMismatch means the destination revision does not match the
	// deployed release binding it was resolved for.
	ErrRevisionMismatch = errors.New("destination revision does not match deployed release")
)

// KafkaPolicy is the non-secret Kafka delivery configuration of a destination.
type KafkaPolicy struct {
	Topic string `json:"topic"`
}

// HTTPSPolicy is the non-secret HTTPS delivery configuration of a destination.
// It names binding names only; no credential value is ever carried here.
type HTTPSPolicy struct {
	URL             string `json:"url"`
	Method          string `json:"method"`
	TokenBinding    string `json:"token_binding"`
	CABundleBinding string `json:"ca_bundle_binding,omitempty"`
}

// ClientIdentity binds one destination to one canonical service subject.
//
// The subject and its grants are deployment configuration carried inside the
// content-addressed revision. Nothing observed on the destination side — a
// response header, a redirect, a served certificate, or any future 4.1c-b
// transport output — can select, influence, or impersonate them.
type ClientIdentity struct {
	Subject string   `json:"subject"`
	Grants  []string `json:"grants,omitempty"`
}

// RevisionInput supplies the semantic fields of a content-addressed destination.
type RevisionInput struct {
	ArtifactID    string
	RevisionID    string
	DestinationID string
	Class         integration.DestinationClass
	Transport     TransportKind
	Kafka         *KafkaPolicy
	HTTPS         *HTTPSPolicy
	Identity      *ClientIdentity
}

// Revision is the immutable runtime contract for one delivery destination.
// Secret values remain out of band; only lifecycle binding names are stored.
type Revision struct {
	SchemaVersion string                       `json:"schema_version"`
	ArtifactID    string                       `json:"artifact_id"`
	RevisionID    string                       `json:"revision_id"`
	DestinationID string                       `json:"destination_id"`
	Class         integration.DestinationClass `json:"class"`
	Transport     TransportKind                `json:"transport"`
	Kafka         *KafkaPolicy                 `json:"kafka,omitempty"`
	HTTPS         *HTTPSPolicy                 `json:"https,omitempty"`
	Identity      *ClientIdentity              `json:"identity,omitempty"`
	Digest        string                       `json:"digest"`
}

// NewRevision validates the semantic fields and content-addresses them.
func NewRevision(input RevisionInput) (Revision, error) {
	revision := Revision{
		SchemaVersion: RevisionSchemaVersion,
		ArtifactID:    input.ArtifactID,
		RevisionID:    input.RevisionID,
		DestinationID: input.DestinationID,
		Class:         input.Class,
		Transport:     input.Transport,
		Kafka:         cloneKafka(input.Kafka),
		HTTPS:         cloneHTTPS(input.HTTPS),
		Identity:      cloneIdentity(input.Identity),
	}
	if err := revision.validateSemanticFields(); err != nil {
		return Revision{}, err
	}
	digest, err := revision.semanticDigest()
	if err != nil {
		return Revision{}, fmt.Errorf("%w: compute digest", ErrInvalidRevision)
	}
	revision.Digest = digest
	return revision, nil
}

// DecodeRevision reads exactly one destination revision, rejecting unknown
// fields, duplicate keys, trailing content, and any semantic mutation of the
// bytes the digest covers.
func DecodeRevision(reader io.Reader) (Revision, error) {
	if reader == nil {
		return Revision{}, ErrInvalidRevision
	}
	raw, err := io.ReadAll(io.LimitReader(reader, maxRevisionSize+1))
	if err != nil || len(raw) == 0 || len(raw) > maxRevisionSize || rejectDuplicateJSONKeys(raw) != nil {
		return Revision{}, ErrInvalidRevision
	}
	return decodeRevisionBytes(raw)
}

func decodeRevisionBytes(raw []byte) (Revision, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var revision Revision
	if err := decoder.Decode(&revision); err != nil {
		return Revision{}, ErrInvalidRevision
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return Revision{}, ErrInvalidRevision
	}
	if err := revision.Validate(); err != nil {
		return Revision{}, err
	}
	return revision, nil
}

// Validate proves the revision is semantically legal and that its digest covers
// its own fields. A single mutated byte in any semantic field fails here.
func (r Revision) Validate() error {
	if err := r.validateSemanticFields(); err != nil {
		return err
	}
	expected, err := r.semanticDigest()
	if err != nil || r.Digest != expected {
		return ErrInvalidRevision
	}
	return nil
}

// Reference is the exact destination reference a delivery attempt must carry.
func (r Revision) Reference() integration.DestinationRevisionRef {
	return integration.DestinationRevisionRef{
		ArtifactRevisionRef: integration.ArtifactRevisionRef{
			ArtifactID: r.ArtifactID,
			RevisionID: r.RevisionID,
			Digest:     r.Digest,
		},
		Class: r.Class,
	}
}

// IdentityBound reports whether this destination declares its own client
// subject. Binding is all-or-nothing per destination: a bound destination never
// falls back to the deployment-fixed compatibility subject.
func (r Revision) IdentityBound() bool { return r.Identity != nil }

// SecretBindingNames lists every lifecycle binding name this destination
// requires. Names only — never a provider value and never material.
func (r Revision) SecretBindingNames() []string {
	if r.HTTPS == nil {
		return nil
	}
	names := []string{r.HTTPS.TokenBinding}
	if r.HTTPS.CABundleBinding != "" {
		names = append(names, r.HTTPS.CABundleBinding)
	}
	return names
}

// EndpointAdvisory returns the destination's declared remote address. It is
// operator-facing diagnostic context and is never a trust input: the deliver
// decision is grounded on the verified digest and the server-issued grant.
func (r Revision) EndpointAdvisory() string {
	if r.HTTPS != nil {
		return r.HTTPS.URL
	}
	if r.Kafka != nil {
		return r.Kafka.Topic
	}
	return ""
}

// ValidateAgainst proves this destination belongs to the deployed release and
// that every secret binding it names is actually present in that release. It is
// the same hasSecretBinding discipline mllp.SourceRevision and
// batch.SourceRevision apply to their sources.
func (r Revision) ValidateAgainst(binding lifecycle.RunnableBinding) error {
	if r.Validate() != nil || binding.IntegrationRevision.ArtifactID == "" ||
		binding.Deployment.Validate() != nil {
		return ErrRevisionMismatch
	}
	for _, name := range r.SecretBindingNames() {
		if !hasSecretBinding(binding.SecretBindings, name) {
			return ErrRevisionMismatch
		}
	}
	return nil
}

func (r Revision) validateSemanticFields() error {
	if r.SchemaVersion != RevisionSchemaVersion || !validIdentity(r.ArtifactID) ||
		!validIdentity(r.RevisionID) || !validIdentity(r.DestinationID) ||
		(r.Class != integration.DestinationClassProduction &&
			r.Class != integration.DestinationClassSandbox) {
		return ErrInvalidRevision
	}
	switch r.Transport {
	case TransportKafka:
		if r.Kafka == nil || r.HTTPS != nil || validateKafka(*r.Kafka) != nil {
			return ErrInvalidRevision
		}
	case TransportHTTPS:
		if r.HTTPS == nil || r.Kafka != nil || validateHTTPS(*r.HTTPS) != nil {
			return ErrInvalidRevision
		}
	default:
		return ErrInvalidRevision
	}
	return validateIdentity(r.Identity)
}

func validateKafka(policy KafkaPolicy) error {
	if !validIdentity(policy.Topic) || len(policy.Topic) > 249 {
		return ErrInvalidRevision
	}
	return nil
}

func validateHTTPS(policy HTTPSPolicy) error {
	parsed, err := url.Parse(policy.URL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" ||
		parsed.User != nil || parsed.Fragment != "" || len(policy.URL) > 2048 {
		return ErrInvalidRevision
	}
	switch policy.Method {
	case "POST", "PUT":
	default:
		return ErrInvalidRevision
	}
	if !validIdentity(policy.TokenBinding) {
		return ErrInvalidRevision
	}
	if policy.CABundleBinding != "" {
		if !validIdentity(policy.CABundleBinding) || policy.CABundleBinding == policy.TokenBinding {
			return ErrInvalidRevision
		}
	}
	return nil
}

func validateIdentity(identity *ClientIdentity) error {
	if identity == nil {
		return nil
	}
	if !validIdentity(identity.Subject) || len(identity.Grants) == 0 ||
		len(identity.Grants) > maxIdentityGrants {
		return ErrInvalidRevision
	}
	seen := make(map[string]struct{}, len(identity.Grants))
	for _, grant := range identity.Grants {
		if !validIdentity(grant) {
			return ErrInvalidRevision
		}
		if _, duplicate := seen[grant]; duplicate {
			return ErrInvalidRevision
		}
		seen[grant] = struct{}{}
	}
	return nil
}

func (r Revision) semanticDigest() (string, error) {
	canonical := r
	canonical.Digest = ""
	canonical.Identity = canonicalIdentity(r.Identity)
	encoded, err := json.Marshal(canonical)
	if err != nil {
		return "", err
	}
	hasher := sha256.New()
	_, _ = hasher.Write([]byte(revisionDigestDom))
	_, _ = hasher.Write(encoded)
	return "sha256:" + hex.EncodeToString(hasher.Sum(nil)), nil
}

func cloneKafka(policy *KafkaPolicy) *KafkaPolicy {
	if policy == nil {
		return nil
	}
	clone := *policy
	return &clone
}

func cloneHTTPS(policy *HTTPSPolicy) *HTTPSPolicy {
	if policy == nil {
		return nil
	}
	clone := *policy
	return &clone
}

func cloneIdentity(identity *ClientIdentity) *ClientIdentity {
	if identity == nil {
		return nil
	}
	clone := *identity
	clone.Grants = append([]string(nil), identity.Grants...)
	return &clone
}

func canonicalIdentity(identity *ClientIdentity) *ClientIdentity {
	clone := cloneIdentity(identity)
	if clone == nil {
		return nil
	}
	sort.Strings(clone.Grants)
	return clone
}

func hasSecretBinding(bindings []integration.SecretBinding, name string) bool {
	for _, binding := range bindings {
		if binding.Name == name {
			return true
		}
	}
	return false
}

func validIdentity(value string) bool {
	if value == "" || len(value) > 256 || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) || unicode.IsSpace(character) {
			return false
		}
	}
	return true
}

func rejectDuplicateJSONKeys(raw []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	var walk func() error
	walk = func() error {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		delimiter, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		switch delimiter {
		case '{':
			seen := map[string]struct{}{}
			for decoder.More() {
				keyToken, err := decoder.Token()
				if err != nil {
					return err
				}
				key, ok := keyToken.(string)
				if !ok {
					return ErrInvalidRevision
				}
				if _, duplicate := seen[key]; duplicate {
					return ErrInvalidRevision
				}
				seen[key] = struct{}{}
				if err := walk(); err != nil {
					return err
				}
			}
			_, err = decoder.Token()
			return err
		case '[':
			for decoder.More() {
				if err := walk(); err != nil {
					return err
				}
			}
			_, err = decoder.Token()
			return err
		default:
			return ErrInvalidRevision
		}
	}
	return walk()
}
