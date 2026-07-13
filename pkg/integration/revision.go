package integration

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
)

// PrincipalKind distinguishes people from non-human runtime identities.
type PrincipalKind string

const (
	PrincipalKindHuman   PrincipalKind = "human"
	PrincipalKindService PrincipalKind = "service"
)

// SecretProviderKind identifies a supported out-of-band secret provider.
type SecretProviderKind string

const (
	SecretProviderEnvironment SecretProviderKind = "env"
	SecretProviderFile        SecretProviderKind = "file"
	SecretProviderVault       SecretProviderKind = "vault"
	SecretProviderAWSSSM      SecretProviderKind = "aws-ssm"
	SecretProviderKubernetes  SecretProviderKind = "k8s"
)

// DestinationClass separates live destinations from preview-safe sandboxes.
type DestinationClass string

const (
	DestinationClassProduction DestinationClass = "production"
	DestinationClassSandbox    DestinationClass = "sandbox"
)

// DataClassification describes the sensitivity of integration data.
type DataClassification string

const (
	DataClassificationPHI DataClassification = "phi"
)

// RawRetentionMode controls whether source bytes may survive processing.
type RawRetentionMode string

const (
	RawRetentionModeEphemeral RawRetentionMode = "ephemeral"
	RawRetentionModeEncrypted RawRetentionMode = "encrypted"
)

// ArtifactRevisionRef binds an artifact to one immutable, content-addressed revision.
type ArtifactRevisionRef struct {
	ArtifactID string `json:"artifact_id"`
	RevisionID string `json:"revision_id"`
	Digest     string `json:"digest"`
}

// SourceRevisionRef adds the runtime source identity to an artifact revision.
type SourceRevisionRef struct {
	ArtifactRevisionRef
	SourceID string `json:"source_id"`
}

// DestinationRevisionRef classifies a destination revision for delivery safety.
type DestinationRevisionRef struct {
	ArtifactRevisionRef
	Class DestinationClass `json:"class"`
}

// Principal is the authenticated identity responsible for an operation.
type Principal struct {
	ID         string        `json:"id"`
	Kind       PrincipalKind `json:"kind"`
	AuthMethod string        `json:"auth_method"`
	Roles      []string      `json:"roles,omitempty"`
	SourceID   string        `json:"source_id,omitempty"`
}

// SecretReference points to secret material without embedding its value.
type SecretReference struct {
	Provider SecretProviderKind `json:"provider"`
	Key      string             `json:"key"`
	Version  string             `json:"version,omitempty"`
}

// SecretBinding gives a stable integration-local name to a secret reference.
type SecretBinding struct {
	Name      string          `json:"name"`
	Reference SecretReference `json:"reference"`
}

// AuditEnvelope records who created a revision, for which tenant, and why.
type AuditEnvelope struct {
	TenantID   string    `json:"tenant_id"`
	Principal  Principal `json:"principal"`
	Reason     string    `json:"reason,omitempty"`
	OccurredAt time.Time `json:"occurred_at"`
}

// RawRetentionPolicy is deny-by-default: its zero value means ephemeral bytes.
type RawRetentionPolicy struct {
	Mode                RawRetentionMode     `json:"mode,omitempty"`
	TTLSeconds          int64                `json:"ttl_seconds,omitempty"`
	Purpose             string               `json:"purpose,omitempty"`
	StorageRevision     *ArtifactRevisionRef `json:"storage_revision,omitempty"`
	EncryptionKey       *SecretReference     `json:"encryption_key,omitempty"`
	AuthorizedBy        Principal            `json:"authorized_by,omitzero"`
	AccessAuditRequired bool                 `json:"access_audit_required"`
}

// EffectiveMode returns the fail-closed retention mode for an omitted policy.
func (p RawRetentionPolicy) EffectiveMode() RawRetentionMode {
	if p.Mode == "" {
		return RawRetentionModeEphemeral
	}
	return p.Mode
}

// Validate verifies that retention exceptions are explicit and auditable.
func (p RawRetentionPolicy) Validate() error {
	v := &validationCollector{}
	switch p.EffectiveMode() {
	case RawRetentionModeEphemeral:
		v.add(p.TTLSeconds == 0, "FORBIDDEN", "ttl_seconds", "ephemeral retention cannot set a TTL")
		v.add(strings.TrimSpace(p.Purpose) == "", "FORBIDDEN", "purpose", "ephemeral retention cannot set a purpose")
		v.add(p.StorageRevision == nil, "FORBIDDEN", "storage_revision", "ephemeral retention cannot configure storage")
		v.add(p.EncryptionKey == nil, "FORBIDDEN", "encryption_key", "ephemeral retention cannot configure an encryption key")
		v.add(isZeroPrincipal(p.AuthorizedBy), "FORBIDDEN", "authorized_by", "ephemeral retention cannot carry an authorizer")
		v.add(!p.AccessAuditRequired, "FORBIDDEN", "access_audit_required", "ephemeral retention cannot enable retained-data access audit")
	case RawRetentionModeEncrypted:
		v.add(p.TTLSeconds > 0, "REQUIRED", "ttl_seconds", "encrypted retention requires a positive TTL")
		v.add(strings.TrimSpace(p.Purpose) != "", "REQUIRED", "purpose", "encrypted retention requires a purpose")
		if p.StorageRevision == nil {
			v.add(false, "REQUIRED", "storage_revision", "encrypted retention requires a storage revision")
		} else {
			validateArtifactRevision("storage_revision", *p.StorageRevision, v)
		}
		if p.EncryptionKey == nil {
			v.add(false, "REQUIRED", "encryption_key", "encrypted retention requires an encryption key reference")
		} else {
			validateSecretReference("encryption_key", *p.EncryptionKey, v)
		}
		validatePrincipal("authorized_by", p.AuthorizedBy, v)
		v.add(p.AccessAuditRequired, "REQUIRED", "access_audit_required", "encrypted retention requires access auditing")
	default:
		v.add(false, "INVALID_RETENTION_MODE", "mode", "raw retention mode is not supported")
	}
	return v.err()
}

// IntegrationPolicy applies data-handling rules to an integration revision.
type IntegrationPolicy struct {
	Classification DataClassification `json:"classification"`
	RawRetention   RawRetentionPolicy `json:"raw_retention"`
}

// IntegrationDefinitionRevisionInput contains all semantic fields used to create a revision.
type IntegrationDefinitionRevisionInput struct {
	DefinitionID     string
	RevisionID       string
	ParentRevisionID string
	TenantID         string
	Source           SourceRevisionRef
	Format           events.SourceFormat
	Profile          ArtifactRevisionRef
	Workflow         ArtifactRevisionRef
	Destinations     []DestinationRevisionRef
	SecretBindings   []SecretBinding
	Policy           IntegrationPolicy
	Created          AuditEnvelope
}

// IntegrationDefinitionRevision is an immutable-by-contract integration binding.
// Any semantic mutation invalidates Digest.
type IntegrationDefinitionRevision struct {
	DefinitionID     string                   `json:"definition_id"`
	RevisionID       string                   `json:"revision_id"`
	ParentRevisionID string                   `json:"parent_revision_id,omitempty"`
	TenantID         string                   `json:"tenant_id"`
	Source           SourceRevisionRef        `json:"source"`
	Format           events.SourceFormat      `json:"format"`
	Profile          ArtifactRevisionRef      `json:"profile"`
	Workflow         ArtifactRevisionRef      `json:"workflow"`
	Destinations     []DestinationRevisionRef `json:"destinations"`
	SecretBindings   []SecretBinding          `json:"secret_bindings,omitempty"`
	Policy           IntegrationPolicy        `json:"policy"`
	Created          AuditEnvelope            `json:"created"`
	Digest           string                   `json:"digest"`
}

// NewIntegrationDefinitionRevision validates, copies, and content-addresses a revision.
func NewIntegrationDefinitionRevision(input IntegrationDefinitionRevisionInput) (IntegrationDefinitionRevision, error) {
	revision := IntegrationDefinitionRevision{
		DefinitionID:     input.DefinitionID,
		RevisionID:       input.RevisionID,
		ParentRevisionID: input.ParentRevisionID,
		TenantID:         input.TenantID,
		Source:           input.Source,
		Format:           input.Format,
		Profile:          input.Profile,
		Workflow:         input.Workflow,
		Destinations:     append([]DestinationRevisionRef(nil), input.Destinations...),
		SecretBindings:   append([]SecretBinding(nil), input.SecretBindings...),
		Policy:           clonePolicy(input.Policy),
		Created:          cloneAuditEnvelope(input.Created),
	}
	if revision.Policy.RawRetention.Mode == "" {
		revision.Policy.RawRetention.Mode = RawRetentionModeEphemeral
	}
	if !revision.Created.OccurredAt.IsZero() {
		revision.Created.OccurredAt = revision.Created.OccurredAt.UTC()
	}

	if err := revision.validateSemanticFields(); err != nil {
		return IntegrationDefinitionRevision{}, err
	}
	if revision.Policy.RawRetention.EffectiveMode() == RawRetentionModeEphemeral {
		revision.Policy.RawRetention = RawRetentionPolicy{Mode: RawRetentionModeEphemeral}
	}
	digest, err := revision.semanticDigest()
	if err != nil {
		return IntegrationDefinitionRevision{}, fmt.Errorf("compute integration revision digest: %w", err)
	}
	revision.Digest = digest
	return revision, nil
}

// DecodeIntegrationDefinitionRevision performs strict JSON decoding and validation.
func DecodeIntegrationDefinitionRevision(reader io.Reader) (IntegrationDefinitionRevision, error) {
	const maxRevisionBytes = 1 << 20
	raw, err := io.ReadAll(io.LimitReader(reader, maxRevisionBytes+1))
	if err != nil {
		return IntegrationDefinitionRevision{}, fmt.Errorf("read integration revision: %w", err)
	}
	if len(raw) > maxRevisionBytes {
		return IntegrationDefinitionRevision{}, fmt.Errorf("decode integration revision: document exceeds %d bytes", maxRevisionBytes)
	}
	if err := rejectDuplicateJSONKeys(raw); err != nil {
		return IntegrationDefinitionRevision{}, fmt.Errorf("decode integration revision: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()

	var revision IntegrationDefinitionRevision
	if err := decoder.Decode(&revision); err != nil {
		return IntegrationDefinitionRevision{}, fmt.Errorf("decode integration revision: %w", err)
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return IntegrationDefinitionRevision{}, fmt.Errorf("decode integration revision: trailing JSON value")
		}
		return IntegrationDefinitionRevision{}, fmt.Errorf("decode integration revision trailer: %w", err)
	}
	if err := validateCanonicalJSONKeys(raw, revision); err != nil {
		return IntegrationDefinitionRevision{}, fmt.Errorf("decode integration revision: %w", err)
	}
	if err := revision.Validate(); err != nil {
		return IntegrationDefinitionRevision{}, err
	}
	return revision, nil
}

func validateCanonicalJSONKeys(raw []byte, decoded any) error {
	canonicalRaw, err := json.Marshal(decoded)
	if err != nil {
		return fmt.Errorf("marshal canonical JSON shape: %w", err)
	}
	inputShape, err := decodeJSONShape(raw)
	if err != nil {
		return err
	}
	canonicalShape, err := decodeJSONShape(canonicalRaw)
	if err != nil {
		return err
	}
	return compareCanonicalJSONKeys(inputShape, canonicalShape, "$")
}

func decodeJSONShape(raw []byte) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	return value, nil
}

func compareCanonicalJSONKeys(input, canonical any, path string) error {
	switch inputValue := input.(type) {
	case map[string]any:
		canonicalValue, ok := canonical.(map[string]any)
		if !ok {
			return fmt.Errorf("%s: value does not match canonical object shape", path)
		}
		for key, child := range inputValue {
			canonicalChild, exists := canonicalValue[key]
			if !exists {
				return fmt.Errorf("%s.%s: key is not in canonical JSON form", path, key)
			}
			if err := compareCanonicalJSONKeys(child, canonicalChild, path+"."+key); err != nil {
				return err
			}
		}
	case []any:
		canonicalValue, ok := canonical.([]any)
		if !ok || len(inputValue) != len(canonicalValue) {
			return fmt.Errorf("%s: value does not match canonical array shape", path)
		}
		for index, child := range inputValue {
			if err := compareCanonicalJSONKeys(child, canonicalValue[index], fmt.Sprintf("%s[%d]", path, index)); err != nil {
				return err
			}
		}
	}
	return nil
}

func rejectDuplicateJSONKeys(raw []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := walkJSONValue(decoder, "$"); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return fmt.Errorf("trailing JSON value")
		}
		return err
	}
	return nil
}

func walkJSONValue(decoder *json.Decoder, path string) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, composite := token.(json.Delim)
	if !composite {
		return nil
	}

	switch delimiter {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return fmt.Errorf("%s: object member name is not a string", path)
			}
			if _, duplicate := seen[key]; duplicate {
				return fmt.Errorf("%s.%s: duplicate JSON key", path, key)
			}
			seen[key] = struct{}{}
			if err := walkJSONValue(decoder, path+"."+key); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil {
			return err
		}
		if closing != json.Delim('}') {
			return fmt.Errorf("%s: malformed JSON object", path)
		}
	case '[':
		index := 0
		for decoder.More() {
			if err := walkJSONValue(decoder, fmt.Sprintf("%s[%d]", path, index)); err != nil {
				return err
			}
			index++
		}
		closing, err := decoder.Token()
		if err != nil {
			return err
		}
		if closing != json.Delim(']') {
			return fmt.Errorf("%s: malformed JSON array", path)
		}
	default:
		return fmt.Errorf("%s: unexpected JSON delimiter %q", path, delimiter)
	}
	return nil
}

// Validate checks all bindings and verifies the semantic digest.
func (r IntegrationDefinitionRevision) Validate() error {
	v := &validationCollector{}
	v.merge("", r.validateSemanticFields())
	v.add(sha256DigestPattern.MatchString(r.Digest), "INVALID_DIGEST", "digest", "revision digest must be sha256 followed by 64 lowercase hexadecimal characters")
	if sha256DigestPattern.MatchString(r.Digest) {
		expected, err := r.semanticDigest()
		if err != nil {
			v.add(false, "DIGEST_FAILURE", "digest", err.Error())
		} else {
			v.add(r.Digest == expected, "DIGEST_MISMATCH", "digest", fmt.Sprintf("semantic content requires digest %s", expected))
		}
	}
	return v.err()
}

// Reference returns this integration definition's immutable artifact identity.
func (r IntegrationDefinitionRevision) Reference() ArtifactRevisionRef {
	return ArtifactRevisionRef{
		ArtifactID: r.DefinitionID,
		RevisionID: r.RevisionID,
		Digest:     r.Digest,
	}
}

func (r IntegrationDefinitionRevision) validateSemanticFields() error {
	v := &validationCollector{}
	v.add(strings.TrimSpace(r.DefinitionID) != "", "REQUIRED", "definition_id", "definition ID is required")
	v.add(strings.TrimSpace(r.RevisionID) != "", "REQUIRED", "revision_id", "revision ID is required")
	v.add(strings.TrimSpace(r.TenantID) != "", "REQUIRED", "tenant_id", "tenant ID is required")
	validateArtifactRevision("source", r.Source.ArtifactRevisionRef, v)
	v.add(strings.TrimSpace(r.Source.SourceID) != "", "REQUIRED", "source.source_id", "source ID is required")
	validateSourceFormat("format", r.Format, v)
	validateArtifactRevision("profile", r.Profile, v)
	validateArtifactRevision("workflow", r.Workflow, v)
	v.add(len(r.Destinations) > 0, "REQUIRED", "destinations", "at least one destination revision is required")
	seenDestinations := make(map[string]struct{}, len(r.Destinations))
	for i, destination := range r.Destinations {
		path := fmt.Sprintf("destinations[%d]", i)
		validateArtifactRevision(path, destination.ArtifactRevisionRef, v)
		v.add(destination.Class == DestinationClassProduction || destination.Class == DestinationClassSandbox, "INVALID_DESTINATION_CLASS", joinPath(path, "class"), "destination class must be production or sandbox")
		key := destination.ArtifactID + "\x00" + destination.RevisionID
		_, duplicate := seenDestinations[key]
		v.add(!duplicate, "DUPLICATE", path, "destination revision is duplicated")
		seenDestinations[key] = struct{}{}
	}
	seenBindings := make(map[string]struct{}, len(r.SecretBindings))
	for i, binding := range r.SecretBindings {
		path := fmt.Sprintf("secret_bindings[%d]", i)
		v.add(strings.TrimSpace(binding.Name) != "", "REQUIRED", joinPath(path, "name"), "secret binding name is required")
		_, duplicate := seenBindings[binding.Name]
		v.add(!duplicate, "DUPLICATE", joinPath(path, "name"), "secret binding name is duplicated")
		seenBindings[binding.Name] = struct{}{}
		validateSecretReference(joinPath(path, "reference"), binding.Reference, v)
	}
	v.add(r.Policy.Classification == DataClassificationPHI, "INVALID_CLASSIFICATION", "policy.classification", "data classification must be phi")
	v.merge("policy.raw_retention", r.Policy.RawRetention.Validate())
	v.add(strings.TrimSpace(r.Created.TenantID) != "", "REQUIRED", "created.tenant_id", "audit tenant ID is required")
	v.add(r.Created.TenantID == r.TenantID, "TENANT_MISMATCH", "created.tenant_id", "audit tenant must match revision tenant")
	validatePrincipal("created.principal", r.Created.Principal, v)
	if r.Created.Principal.Kind == PrincipalKindHuman {
		v.add(strings.TrimSpace(r.Created.Reason) != "", "REQUIRED", "created.reason", "human-authored revisions require a reason")
	}
	v.add(!r.Created.OccurredAt.IsZero(), "REQUIRED", "created.occurred_at", "audit timestamp is required")
	return v.err()
}

func (r IntegrationDefinitionRevision) semanticDigest() (string, error) {
	destinations := append([]DestinationRevisionRef(nil), r.Destinations...)
	secretBindings := append([]SecretBinding(nil), r.SecretBindings...)
	policy := clonePolicy(r.Policy)
	policy.RawRetention.Mode = policy.RawRetention.EffectiveMode()
	if policy.RawRetention.EffectiveMode() == RawRetentionModeEphemeral {
		policy.RawRetention = RawRetentionPolicy{Mode: RawRetentionModeEphemeral}
	}
	createdPrincipal := clonePrincipal(r.Created.Principal)

	sort.Slice(destinations, func(i, j int) bool {
		left := destinations[i]
		right := destinations[j]
		if left.ArtifactID != right.ArtifactID {
			return left.ArtifactID < right.ArtifactID
		}
		if left.RevisionID != right.RevisionID {
			return left.RevisionID < right.RevisionID
		}
		return left.Class < right.Class
	})
	sort.Slice(secretBindings, func(i, j int) bool {
		return secretBindings[i].Name < secretBindings[j].Name
	})
	sort.Strings(createdPrincipal.Roles)
	sort.Strings(policy.RawRetention.AuthorizedBy.Roles)

	// Digest is omitted from its own preimage. Every other field, including the
	// normalized creation audit, is integrity-protected.
	canonical := struct {
		DefinitionID     string                   `json:"definition_id"`
		RevisionID       string                   `json:"revision_id"`
		ParentRevisionID string                   `json:"parent_revision_id,omitempty"`
		TenantID         string                   `json:"tenant_id"`
		Source           SourceRevisionRef        `json:"source"`
		Format           events.SourceFormat      `json:"format"`
		Profile          ArtifactRevisionRef      `json:"profile"`
		Workflow         ArtifactRevisionRef      `json:"workflow"`
		Destinations     []DestinationRevisionRef `json:"destinations"`
		SecretBindings   []SecretBinding          `json:"secret_bindings,omitempty"`
		Policy           IntegrationPolicy        `json:"policy"`
		Created          struct {
			TenantID   string    `json:"tenant_id"`
			Principal  Principal `json:"principal"`
			Reason     string    `json:"reason,omitempty"`
			OccurredAt time.Time `json:"occurred_at"`
		} `json:"created"`
	}{
		DefinitionID:     r.DefinitionID,
		RevisionID:       r.RevisionID,
		ParentRevisionID: r.ParentRevisionID,
		TenantID:         r.TenantID,
		Source:           r.Source,
		Format:           r.Format,
		Profile:          r.Profile,
		Workflow:         r.Workflow,
		Destinations:     destinations,
		SecretBindings:   secretBindings,
		Policy:           policy,
	}
	canonical.Created.TenantID = r.Created.TenantID
	canonical.Created.Principal = createdPrincipal
	canonical.Created.Reason = r.Created.Reason
	if !r.Created.OccurredAt.IsZero() {
		canonical.Created.OccurredAt = r.Created.OccurredAt.UTC()
	}

	encoded, err := json.Marshal(canonical)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func validatePrincipal(path string, principal Principal, v *validationCollector) {
	v.add(strings.TrimSpace(principal.ID) != "", "REQUIRED", joinPath(path, "id"), "principal ID is required")
	v.add(principal.Kind == PrincipalKindHuman || principal.Kind == PrincipalKindService, "INVALID_PRINCIPAL_KIND", joinPath(path, "kind"), "principal kind must be human or service")
	v.add(strings.TrimSpace(principal.AuthMethod) != "", "REQUIRED", joinPath(path, "auth_method"), "authentication method is required")
	if principal.Kind == PrincipalKindHuman {
		v.add(len(principal.Roles) > 0, "REQUIRED", joinPath(path, "roles"), "human principals require at least one role")
	}
	seenRoles := make(map[string]struct{}, len(principal.Roles))
	for i, role := range principal.Roles {
		v.add(strings.TrimSpace(role) != "", "REQUIRED", fmt.Sprintf("%s.roles[%d]", path, i), "role cannot be empty")
		v.add(role == strings.TrimSpace(role), "NON_CANONICAL", fmt.Sprintf("%s.roles[%d]", path, i), "role cannot have surrounding whitespace")
		_, duplicate := seenRoles[role]
		v.add(!duplicate, "DUPLICATE", fmt.Sprintf("%s.roles[%d]", path, i), "role is duplicated")
		seenRoles[role] = struct{}{}
	}
	if principal.Kind == PrincipalKindService {
		v.add(strings.TrimSpace(principal.SourceID) != "", "REQUIRED", joinPath(path, "source_id"), "service principals require a source ID")
	}
}

func validateSecretReference(path string, reference SecretReference, v *validationCollector) {
	validProvider := false
	switch reference.Provider {
	case SecretProviderEnvironment, SecretProviderFile, SecretProviderVault, SecretProviderAWSSSM, SecretProviderKubernetes:
		validProvider = true
	}
	v.add(validProvider, "INVALID_SECRET_PROVIDER", joinPath(path, "provider"), "secret provider is not supported")
	v.add(strings.TrimSpace(reference.Key) != "", "REQUIRED", joinPath(path, "key"), "secret key is required")
}

func cloneAuditEnvelope(audit AuditEnvelope) AuditEnvelope {
	clone := audit
	clone.Principal = clonePrincipal(audit.Principal)
	return clone
}

func clonePrincipal(principal Principal) Principal {
	clone := principal
	clone.Roles = append([]string(nil), principal.Roles...)
	return clone
}

func clonePolicy(policy IntegrationPolicy) IntegrationPolicy {
	clone := policy
	clone.RawRetention.AuthorizedBy = clonePrincipal(policy.RawRetention.AuthorizedBy)
	if policy.RawRetention.StorageRevision != nil {
		storage := *policy.RawRetention.StorageRevision
		clone.RawRetention.StorageRevision = &storage
	}
	if policy.RawRetention.EncryptionKey != nil {
		key := *policy.RawRetention.EncryptionKey
		clone.RawRetention.EncryptionKey = &key
	}
	return clone
}

func isZeroPrincipal(principal Principal) bool {
	return principal.ID == "" && principal.Kind == "" && principal.AuthMethod == "" && len(principal.Roles) == 0 && principal.SourceID == ""
}
