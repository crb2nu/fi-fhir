// Package processor contains storage-neutral runtime integration orchestration.
package processor

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

const (
	profileRevisionDigestDomain  = "fi-fhir/profile-revision/v1\x00"
	workflowRevisionDigestDomain = "fi-fhir/workflow-revision/v1\x00"
)

var (
	// ErrTenantMismatch means a request is outside the resolver deployment's security domain.
	ErrTenantMismatch = errors.New("artifact resolver tenant mismatch")
	// ErrInvalidArtifactReference means an artifact identity or digest is malformed.
	ErrInvalidArtifactReference = errors.New("invalid artifact revision reference")
	// ErrInvalidArtifactContent means stored bytes cannot represent the referenced artifact kind.
	ErrInvalidArtifactContent = errors.New("invalid artifact revision content")
	// ErrArtifactDigestMismatch means stored bytes do not match their content-addressed reference.
	ErrArtifactDigestMismatch = errors.New("artifact revision digest mismatch")

	sha256DigestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
)

// ArtifactRevisionLoader loads exact immutable artifact bytes without exposing storage types.
type ArtifactRevisionLoader interface {
	LoadProfileRevision(ctx context.Context, artifactID, revisionID string) ([]byte, error)
	LoadWorkflowRevision(ctx context.Context, artifactID, revisionID string) ([]byte, error)
}

// ResolvedArtifactRevisions contains one verified profile/workflow pair.
// Content is sealed so every accessor can return a defensive copy.
type ResolvedArtifactRevisions struct {
	profileRef   integration.ArtifactRevisionRef
	workflowRef  integration.ArtifactRevisionRef
	profileJSON  []byte
	workflowYAML []byte
}

// ProfileReference returns the verified immutable profile reference.
func (r ResolvedArtifactRevisions) ProfileReference() integration.ArtifactRevisionRef {
	return r.profileRef
}

// WorkflowReference returns the verified immutable workflow reference.
func (r ResolvedArtifactRevisions) WorkflowReference() integration.ArtifactRevisionRef {
	return r.workflowRef
}

// ProfileJSON returns an exact defensive copy of the stored profile revision.
func (r ResolvedArtifactRevisions) ProfileJSON() []byte {
	return append([]byte(nil), r.profileJSON...)
}

// WorkflowYAML returns an exact defensive copy of the stored workflow revision.
func (r ResolvedArtifactRevisions) WorkflowYAML() []byte {
	return append([]byte(nil), r.workflowYAML...)
}

// RevisionResolver verifies immutable executable artifacts for one deployment tenant.
type RevisionResolver struct {
	deploymentTenantID string
	loader             ArtifactRevisionLoader
}

// NewRevisionResolver constructs a resolver for a single deployment security domain.
func NewRevisionResolver(deploymentTenantID string, loader ArtifactRevisionLoader) (*RevisionResolver, error) {
	if err := validateIdentity("deployment tenant ID", deploymentTenantID); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrTenantMismatch, err)
	}
	if loader == nil {
		return nil, fmt.Errorf("artifact revision loader is required")
	}
	return &RevisionResolver{deploymentTenantID: deploymentTenantID, loader: loader}, nil
}

// Resolve loads and verifies the exact profile and workflow references.
func (r *RevisionResolver) Resolve(
	ctx context.Context,
	tenantID string,
	profileRef integration.ArtifactRevisionRef,
	workflowRef integration.ArtifactRevisionRef,
) (ResolvedArtifactRevisions, error) {
	if r == nil || r.loader == nil {
		return ResolvedArtifactRevisions{}, fmt.Errorf("artifact revision resolver is not configured")
	}
	if tenantID != r.deploymentTenantID {
		return ResolvedArtifactRevisions{}, fmt.Errorf(
			"%w: requested tenant %q is outside deployment tenant %q",
			ErrTenantMismatch,
			tenantID,
			r.deploymentTenantID,
		)
	}
	if err := validateProfileReference(profileRef); err != nil {
		return ResolvedArtifactRevisions{}, err
	}
	if err := validateWorkflowReference(workflowRef); err != nil {
		return ResolvedArtifactRevisions{}, err
	}

	profileJSON, err := r.loader.LoadProfileRevision(ctx, profileRef.ArtifactID, profileRef.RevisionID)
	if err != nil {
		return ResolvedArtifactRevisions{}, fmt.Errorf("load profile revision %s/%s: %w", profileRef.ArtifactID, profileRef.RevisionID, err)
	}
	computedProfileRef, err := newProfileRevisionReference(profileRef.ArtifactID, profileRef.RevisionID, profileJSON)
	if err != nil {
		return ResolvedArtifactRevisions{}, fmt.Errorf("profile %s/%s: %w", profileRef.ArtifactID, profileRef.RevisionID, err)
	}
	if computedProfileRef != profileRef {
		return ResolvedArtifactRevisions{}, fmt.Errorf("%w: profile %s/%s", ErrArtifactDigestMismatch, profileRef.ArtifactID, profileRef.RevisionID)
	}

	workflowYAML, err := r.loader.LoadWorkflowRevision(ctx, workflowRef.ArtifactID, workflowRef.RevisionID)
	if err != nil {
		return ResolvedArtifactRevisions{}, fmt.Errorf("load workflow revision %s/%s: %w", workflowRef.ArtifactID, workflowRef.RevisionID, err)
	}
	computedWorkflowRef, err := newWorkflowRevisionReference(workflowRef.ArtifactID, workflowRef.RevisionID, workflowYAML)
	if err != nil {
		return ResolvedArtifactRevisions{}, fmt.Errorf("workflow %s/%s: %w", workflowRef.ArtifactID, workflowRef.RevisionID, err)
	}
	if computedWorkflowRef != workflowRef {
		return ResolvedArtifactRevisions{}, fmt.Errorf("%w: workflow %s/%s", ErrArtifactDigestMismatch, workflowRef.ArtifactID, workflowRef.RevisionID)
	}

	return ResolvedArtifactRevisions{
		profileRef:   profileRef,
		workflowRef:  workflowRef,
		profileJSON:  append([]byte(nil), profileJSON...),
		workflowYAML: append([]byte(nil), workflowYAML...),
	}, nil
}

// NewProfileRevisionReference creates a content-addressed profile publication reference.
// Profile digests use a canonical JSON object, so key order and whitespace are insignificant.
func NewProfileRevisionReference(
	artifactID string,
	revisionID int,
	profileJSON []byte,
) (integration.ArtifactRevisionRef, error) {
	if revisionID <= 0 {
		return integration.ArtifactRevisionRef{}, fmt.Errorf("%w: profile revision ID must be positive", ErrInvalidArtifactReference)
	}
	return newProfileRevisionReference(artifactID, strconv.Itoa(revisionID), profileJSON)
}

func newProfileRevisionReference(
	artifactID string,
	revisionID string,
	profileJSON []byte,
) (integration.ArtifactRevisionRef, error) {
	if err := validateIdentity("profile artifact ID", artifactID); err != nil {
		return integration.ArtifactRevisionRef{}, fmt.Errorf("%w: %w", ErrInvalidArtifactReference, err)
	}
	if err := validateCanonicalPositiveInteger("profile revision ID", revisionID); err != nil {
		return integration.ArtifactRevisionRef{}, fmt.Errorf("%w: %w", ErrInvalidArtifactReference, err)
	}
	canonical, err := canonicalJSONObject(profileJSON)
	if err != nil {
		return integration.ArtifactRevisionRef{}, fmt.Errorf("%w: %w", ErrInvalidArtifactContent, err)
	}
	return integration.ArtifactRevisionRef{
		ArtifactID: artifactID,
		RevisionID: revisionID,
		Digest:     domainDigest(profileRevisionDigestDomain, canonical),
	}, nil
}

// NewWorkflowRevisionReference creates a content-addressed workflow publication reference.
// Workflow digests bind the exact UTF-8 YAML bytes, including line endings and whitespace.
func NewWorkflowRevisionReference(
	artifactID string,
	revisionID string,
	workflowYAML []byte,
) (integration.ArtifactRevisionRef, error) {
	return newWorkflowRevisionReference(artifactID, revisionID, workflowYAML)
}

func newWorkflowRevisionReference(
	artifactID string,
	revisionID string,
	workflowYAML []byte,
) (integration.ArtifactRevisionRef, error) {
	if err := validateIdentity("workflow artifact ID", artifactID); err != nil {
		return integration.ArtifactRevisionRef{}, fmt.Errorf("%w: %w", ErrInvalidArtifactReference, err)
	}
	if err := validateIdentity("workflow revision ID", revisionID); err != nil {
		return integration.ArtifactRevisionRef{}, fmt.Errorf("%w: %w", ErrInvalidArtifactReference, err)
	}
	if !utf8.Valid(workflowYAML) {
		return integration.ArtifactRevisionRef{}, fmt.Errorf("%w: workflow YAML is not valid UTF-8", ErrInvalidArtifactContent)
	}
	return integration.ArtifactRevisionRef{
		ArtifactID: artifactID,
		RevisionID: revisionID,
		Digest:     domainDigest(workflowRevisionDigestDomain, workflowYAML),
	}, nil
}

func validateProfileReference(ref integration.ArtifactRevisionRef) error {
	if err := validateIdentity("profile artifact ID", ref.ArtifactID); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidArtifactReference, err)
	}
	if err := validateCanonicalPositiveInteger("profile revision ID", ref.RevisionID); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidArtifactReference, err)
	}
	return validateDigest(ref.Digest)
}

func validateWorkflowReference(ref integration.ArtifactRevisionRef) error {
	if err := validateIdentity("workflow artifact ID", ref.ArtifactID); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidArtifactReference, err)
	}
	if err := validateIdentity("workflow revision ID", ref.RevisionID); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidArtifactReference, err)
	}
	return validateDigest(ref.Digest)
}

func validateDigest(digest string) error {
	if !sha256DigestPattern.MatchString(digest) {
		return fmt.Errorf("%w: digest must be sha256 followed by 64 lowercase hexadecimal characters", ErrInvalidArtifactReference)
	}
	return nil
}

func validateIdentity(name, value string) error {
	if value == "" {
		return fmt.Errorf("%s is required", name)
	}
	if strings.TrimSpace(value) != value {
		return fmt.Errorf("%s must not contain surrounding whitespace", name)
	}
	return nil
}

func validateCanonicalPositiveInteger(name, value string) error {
	if err := validateIdentity(name, value); err != nil {
		return err
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 || strconv.Itoa(parsed) != value {
		return fmt.Errorf("%s must be a canonical positive decimal integer", name)
	}
	return nil
}

func domainDigest(domain string, content []byte) string {
	hash := sha256.New()
	_, _ = hash.Write([]byte(domain))
	_, _ = hash.Write(content)
	return "sha256:" + hex.EncodeToString(hash.Sum(nil))
}

func canonicalJSONObject(raw []byte) ([]byte, error) {
	if err := validateJSONUnicode(raw); err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	value, err := decodeUniqueJSONValue(decoder, "$")
	if err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, fmt.Errorf("profile JSON contains a trailing value")
		}
		return nil, fmt.Errorf("profile JSON trailing content: %w", err)
	}
	if _, ok := value.(map[string]any); !ok {
		return nil, fmt.Errorf("profile JSON must be an object")
	}
	canonical, err := marshalCanonicalJSONValue(value)
	if err != nil {
		return nil, fmt.Errorf("marshal canonical profile JSON: %w", err)
	}
	return canonical, nil
}

func marshalCanonicalJSONValue(value any) ([]byte, error) {
	switch typed := value.(type) {
	case nil:
		return []byte("null"), nil
	case bool, string:
		return json.Marshal(typed)
	case json.Number:
		canonical, err := canonicalJSONNumber(typed.String())
		if err != nil {
			return nil, err
		}
		return []byte(canonical), nil
	case []any:
		var buffer bytes.Buffer
		buffer.WriteByte('[')
		for index, child := range typed {
			if index > 0 {
				buffer.WriteByte(',')
			}
			encoded, err := marshalCanonicalJSONValue(child)
			if err != nil {
				return nil, err
			}
			buffer.Write(encoded)
		}
		buffer.WriteByte(']')
		return buffer.Bytes(), nil
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		var buffer bytes.Buffer
		buffer.WriteByte('{')
		for index, key := range keys {
			if index > 0 {
				buffer.WriteByte(',')
			}
			encodedKey, err := json.Marshal(key)
			if err != nil {
				return nil, err
			}
			buffer.Write(encodedKey)
			buffer.WriteByte(':')
			encodedValue, err := marshalCanonicalJSONValue(typed[key])
			if err != nil {
				return nil, err
			}
			buffer.Write(encodedValue)
		}
		buffer.WriteByte('}')
		return buffer.Bytes(), nil
	default:
		return nil, fmt.Errorf("unsupported canonical profile JSON value %T", value)
	}
}

func canonicalJSONNumber(raw string) (string, error) {
	mantissa := raw
	exponentText := "0"
	if exponentIndex := strings.IndexAny(raw, "eE"); exponentIndex >= 0 {
		mantissa = raw[:exponentIndex]
		exponentText = raw[exponentIndex+1:]
	}

	negative := strings.HasPrefix(mantissa, "-")
	if negative {
		mantissa = mantissa[1:]
	}
	integerPart := mantissa
	fractionPart := ""
	if decimalIndex := strings.IndexByte(mantissa, '.'); decimalIndex >= 0 {
		integerPart = mantissa[:decimalIndex]
		fractionPart = mantissa[decimalIndex+1:]
	}

	digits := strings.TrimLeft(integerPart+fractionPart, "0")
	if digits == "" {
		return "0", nil
	}

	exponent, ok := new(big.Int).SetString(exponentText, 10)
	if !ok {
		return "", fmt.Errorf("invalid JSON number exponent %q", exponentText)
	}
	exponent.Sub(exponent, new(big.Int).SetUint64(uint64(len(fractionPart))))
	trimmedDigits := strings.TrimRight(digits, "0")
	exponent.Add(exponent, new(big.Int).SetUint64(uint64(len(digits)-len(trimmedDigits))))
	digits = trimmedDigits

	sign := ""
	if negative {
		sign = "-"
	}
	return sign + digits + "e" + exponent.String(), nil
}

func validateJSONUnicode(raw []byte) error {
	if !utf8.Valid(raw) {
		return fmt.Errorf("profile JSON is not valid UTF-8")
	}

	inString := false
	for index := 0; index < len(raw); index++ {
		switch raw[index] {
		case '"':
			inString = !inString
		case '\\':
			if !inString || index+1 >= len(raw) {
				continue
			}
			index++
			if raw[index] != 'u' {
				continue
			}
			codePoint, ok := parseJSONHex4(raw, index+1)
			if !ok {
				return fmt.Errorf("profile JSON contains an invalid Unicode escape")
			}
			index += 4
			switch {
			case codePoint >= 0xd800 && codePoint <= 0xdbff:
				if index+6 >= len(raw) || raw[index+1] != '\\' || raw[index+2] != 'u' {
					return fmt.Errorf("profile JSON contains an unpaired high surrogate")
				}
				lowSurrogate, valid := parseJSONHex4(raw, index+3)
				if !valid || lowSurrogate < 0xdc00 || lowSurrogate > 0xdfff {
					return fmt.Errorf("profile JSON contains an unpaired high surrogate")
				}
				index += 6
			case codePoint >= 0xdc00 && codePoint <= 0xdfff:
				return fmt.Errorf("profile JSON contains an unpaired low surrogate")
			}
		}
	}
	return nil
}

func parseJSONHex4(raw []byte, start int) (uint16, bool) {
	if start < 0 || start+4 > len(raw) {
		return 0, false
	}
	var value uint16
	for _, character := range raw[start : start+4] {
		value <<= 4
		switch {
		case character >= '0' && character <= '9':
			value += uint16(character - '0')
		case character >= 'a' && character <= 'f':
			value += uint16(character-'a') + 10
		case character >= 'A' && character <= 'F':
			value += uint16(character-'A') + 10
		default:
			return 0, false
		}
	}
	return value, true
}

func decodeUniqueJSONValue(decoder *json.Decoder, path string) (any, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, fmt.Errorf("decode profile JSON at %s: %w", path, err)
	}
	delimiter, isDelimiter := token.(json.Delim)
	if !isDelimiter {
		return token, nil
	}

	switch delimiter {
	case '{':
		object := make(map[string]any)
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return nil, fmt.Errorf("decode profile JSON key at %s: %w", path, err)
			}
			key, ok := keyToken.(string)
			if !ok {
				return nil, fmt.Errorf("profile JSON key at %s is not a string", path)
			}
			if _, duplicate := object[key]; duplicate {
				return nil, fmt.Errorf("duplicate profile JSON key %q at %s", key, path)
			}
			value, err := decodeUniqueJSONValue(decoder, path+"."+key)
			if err != nil {
				return nil, err
			}
			object[key] = value
		}
		if _, err := decoder.Token(); err != nil {
			return nil, fmt.Errorf("close profile JSON object at %s: %w", path, err)
		}
		return object, nil
	case '[':
		array := make([]any, 0)
		for index := 0; decoder.More(); index++ {
			value, err := decodeUniqueJSONValue(decoder, fmt.Sprintf("%s[%d]", path, index))
			if err != nil {
				return nil, err
			}
			array = append(array, value)
		}
		if _, err := decoder.Token(); err != nil {
			return nil, fmt.Errorf("close profile JSON array at %s: %w", path, err)
		}
		return array, nil
	default:
		return nil, fmt.Errorf("unexpected profile JSON delimiter %q at %s", delimiter, path)
	}
}
