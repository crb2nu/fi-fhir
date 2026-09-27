package connection

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/ingress"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

// The HTTP source document (.loom/38, Decision 2).
//
// MLLP and batch sources have content-addressed documents; HTTP ingress had
// only FI_FHIR_HTTP_INGRESS_* environment. A definition's source reference
// could therefore name a digest with no bytes behind it — the gap the
// destination package closed for destinations in 4.1c-a. This document gives
// an HTTP source the same idiom as mllp.SourceRevision: a schema version, a
// domain-separated digest over the semantic fields, strict decoding, and one
// coarse error for inventory safety.
//
// It declares what FI_FHIR_HTTP_INGRESS_* configures. The runtime does not
// consume it in this program: `serve` still reads the environment, and the
// HTTP ingress counts as mounting a revision only when the definition it is
// bound to names that revision's digest.

const (
	// HTTPSourceSchemaVersion pins the HTTP source wire contract.
	HTTPSourceSchemaVersion = "1"
	// DefaultHTTPSourcePath is the only path the HTTP ingress mounts today.
	DefaultHTTPSourcePath = ingress.Path

	httpSourceDigestDomain    = "fi-fhir/http-source/v1\x00"
	maxHTTPSourceRevisionSize = 1 << 20
	maxHTTPSourceBodyBytes    = ingress.DefaultMaxBodyBytes
	maxHTTPSourcePathBytes    = 256
	maxHTTPOAuthURLBytes      = 2048
	maxHTTPOAuthAudience      = 512
	maxHTTPOAuthClaimBytes    = 128
	maxHTTPOAuthAlgorithms    = 10
	maxHTTPOAuthClientIDs     = 256
)

// HTTPAuthMode is the ingress authentication mode the document declares.
type HTTPAuthMode string

const (
	HTTPAuthModeBearer HTTPAuthMode = HTTPAuthMode(ingress.AuthModeBearer)
	HTTPAuthModeHMAC   HTTPAuthMode = HTTPAuthMode(ingress.AuthModeHMAC)
	HTTPAuthModeOAuth2 HTTPAuthMode = HTTPAuthMode(ingress.AuthModeOAuth2)
)

// Defaults the runtime applies to an unset OAuth2 claim or algorithm list
// (cmd/fi-fhir loadHTTPIngressAuthenticatorFromEnv). A document always states
// them explicitly so its digest covers the effective value.
const (
	DefaultHTTPOAuthTenantClaim   = "tenant_id"
	DefaultHTTPOAuthRolesClaim    = "roles"
	DefaultHTTPOAuthClientIDClaim = "client_id"
	DefaultHTTPOAuthSigningAlg    = "RS256"
)

// httpOAuthSigningAlgorithms restates requestsecurity's supported set; the
// OIDC service authenticator refuses anything else at startup.
var httpOAuthSigningAlgorithms = map[string]struct{}{
	"RS256": {}, "RS384": {}, "RS512": {},
	"PS256": {}, "PS384": {}, "PS512": {},
	"ES256": {}, "ES384": {}, "ES512": {},
	"EdDSA": {},
}

// ErrInvalidHTTPSourceRevision is deliberately coarse, like
// mllp.ErrInvalidSourceRevision: field-level findings belong to the catalog's
// checker, not to a document decoder a caller could use as an oracle.
var ErrInvalidHTTPSourceRevision = errors.New("invalid HTTP source revision")

// HTTPOAuthPolicy is the OAuth2 client-credentials configuration of an HTTP
// source. It names no secret: the issuer's keys are discovered, not stored.
type HTTPOAuthPolicy struct {
	IssuerURL        string   `json:"issuer_url"`
	Audience         string   `json:"audience"`
	TenantClaim      string   `json:"tenant_claim"`
	RolesClaim       string   `json:"roles_claim"`
	ClientIDClaim    string   `json:"client_id_claim"`
	SigningAlgs      []string `json:"signing_algs"`
	AllowedClientIDs []string `json:"allowed_client_ids"`
}

// HTTPSourceRevisionInput supplies the semantic fields of an HTTP source.
type HTTPSourceRevisionInput struct {
	ArtifactID        string
	RevisionID        string
	SourceID          string
	Path              string
	AuthMode          HTTPAuthMode
	PrincipalID       string
	CredentialBinding string
	OAuth             *HTTPOAuthPolicy
	MaxBodyBytes      int64
}

// HTTPSourceRevision is the immutable, content-addressed HTTP ingress source
// contract. Bearer and HMAC sources name a principal and a credential
// binding; OAuth2 sources name neither and carry an OAuth policy instead.
type HTTPSourceRevision struct {
	SchemaVersion     string           `json:"schema_version"`
	ArtifactID        string           `json:"artifact_id"`
	RevisionID        string           `json:"revision_id"`
	SourceID          string           `json:"source_id"`
	Path              string           `json:"path"`
	AuthMode          HTTPAuthMode     `json:"auth_mode"`
	PrincipalID       string           `json:"principal_id,omitempty"`
	CredentialBinding string           `json:"credential_binding,omitempty"`
	OAuth             *HTTPOAuthPolicy `json:"oauth,omitempty"`
	MaxBodyBytes      int64            `json:"max_body_bytes"`
	Digest            string           `json:"digest"`
}

// NewHTTPSourceRevision validates, copies, and content-addresses an HTTP source.
func NewHTTPSourceRevision(input HTTPSourceRevisionInput) (HTTPSourceRevision, error) {
	revision := HTTPSourceRevision{
		SchemaVersion:     HTTPSourceSchemaVersion,
		ArtifactID:        input.ArtifactID,
		RevisionID:        input.RevisionID,
		SourceID:          input.SourceID,
		Path:              input.Path,
		AuthMode:          input.AuthMode,
		PrincipalID:       input.PrincipalID,
		CredentialBinding: input.CredentialBinding,
		OAuth:             cloneHTTPOAuth(input.OAuth),
		MaxBodyBytes:      input.MaxBodyBytes,
	}
	if err := revision.validateSemanticFields(); err != nil {
		return HTTPSourceRevision{}, err
	}
	digest, err := revision.semanticDigest()
	if err != nil {
		return HTTPSourceRevision{}, fmt.Errorf("%w: compute digest", ErrInvalidHTTPSourceRevision)
	}
	revision.Digest = digest
	return revision, nil
}

// DecodeHTTPSourceRevision reads exactly one HTTP source revision, rejecting
// unknown fields, duplicate keys, trailing content, and any semantic mutation
// of the bytes the digest covers.
func DecodeHTTPSourceRevision(reader io.Reader) (HTTPSourceRevision, error) {
	if reader == nil {
		return HTTPSourceRevision{}, ErrInvalidHTTPSourceRevision
	}
	raw, err := io.ReadAll(io.LimitReader(reader, maxHTTPSourceRevisionSize+1))
	if err != nil || len(raw) == 0 || len(raw) > maxHTTPSourceRevisionSize {
		return HTTPSourceRevision{}, ErrInvalidHTTPSourceRevision
	}
	if _, err := duplicateJSONKey(raw); err != nil {
		return HTTPSourceRevision{}, ErrInvalidHTTPSourceRevision
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var revision HTTPSourceRevision
	if err := decoder.Decode(&revision); err != nil {
		return HTTPSourceRevision{}, ErrInvalidHTTPSourceRevision
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return HTTPSourceRevision{}, ErrInvalidHTTPSourceRevision
	}
	if err := revision.Validate(); err != nil {
		return HTTPSourceRevision{}, err
	}
	return revision, nil
}

// Validate proves the revision is semantically legal and that its digest
// covers its own fields.
func (r HTTPSourceRevision) Validate() error {
	if err := r.validateSemanticFields(); err != nil {
		return err
	}
	expected, err := r.semanticDigest()
	if err != nil || r.Digest != expected {
		return ErrInvalidHTTPSourceRevision
	}
	return nil
}

// Reference returns the source's immutable artifact identity.
func (r HTTPSourceRevision) Reference() integration.ArtifactRevisionRef {
	return integration.ArtifactRevisionRef{ArtifactID: r.ArtifactID, RevisionID: r.RevisionID, Digest: r.Digest}
}

func (r HTTPSourceRevision) validateSemanticFields() error {
	if r.SchemaVersion != HTTPSourceSchemaVersion || !validIdentity(r.ArtifactID) ||
		!validIdentity(r.RevisionID) || !validIdentity(r.SourceID) ||
		!validHTTPSourcePath(r.Path) ||
		r.MaxBodyBytes < 1 || r.MaxBodyBytes > maxHTTPSourceBodyBytes {
		return ErrInvalidHTTPSourceRevision
	}
	switch r.AuthMode {
	case HTTPAuthModeBearer, HTTPAuthModeHMAC:
		if !validIdentity(r.PrincipalID) || !validIdentity(r.CredentialBinding) || r.OAuth != nil {
			return ErrInvalidHTTPSourceRevision
		}
	case HTTPAuthModeOAuth2:
		if r.PrincipalID != "" || r.CredentialBinding != "" || r.OAuth == nil {
			return ErrInvalidHTTPSourceRevision
		}
		if len(httpOAuthProblem(*r.OAuth)) > 0 {
			return ErrInvalidHTTPSourceRevision
		}
	default:
		return ErrInvalidHTTPSourceRevision
	}
	return nil
}

// httpOAuthFieldProblem is one refused OAuth field: its path under `oauth`
// and the reason.
type httpOAuthFieldProblem struct {
	code    string
	path    string
	message string
}

// httpOAuthProblem returns every reason an OAuth policy is refused, in field
// order. It is the single definition both the constructor and the catalog
// checker use, so they cannot disagree about OAuth.
func httpOAuthProblem(policy HTTPOAuthPolicy) []httpOAuthFieldProblem {
	var problems []httpOAuthFieldProblem
	add := func(code, path, message string) {
		problems = append(problems, httpOAuthFieldProblem{code: code, path: path, message: message})
	}
	if reason := oauthIssuerProblem(policy.IssuerURL); reason == "required" {
		add(CodeRequired, "issuer_url", "is required")
	} else if reason != "" {
		add(CodeInvalidURL, "issuer_url", reason)
	}
	switch {
	case policy.Audience == "":
		add(CodeRequired, "audience", "is required")
	case len(policy.Audience) > maxHTTPOAuthAudience || !canonicalText(policy.Audience):
		add(CodeInvalidValue, "audience", "must be at most 512 characters with no surrounding whitespace or control characters")
	}
	claims := []struct{ path, value string }{
		{"tenant_claim", policy.TenantClaim},
		{"roles_claim", policy.RolesClaim},
		{"client_id_claim", policy.ClientIDClaim},
	}
	claimsOK := true
	for _, claim := range claims {
		switch {
		case claim.value == "":
			add(CodeRequired, claim.path, "is required")
			claimsOK = false
		case len(claim.value) > maxHTTPOAuthClaimBytes || !canonicalText(claim.value):
			add(CodeInvalidValue, claim.path, "must be at most 128 characters with no surrounding whitespace or control characters")
			claimsOK = false
		}
	}
	if claimsOK {
		if policy.TenantClaim == policy.RolesClaim {
			add(CodeConflict, "roles_claim", "must differ from tenant_claim")
		}
		if policy.ClientIDClaim == policy.TenantClaim || policy.ClientIDClaim == policy.RolesClaim || policy.ClientIDClaim == "sub" {
			add(CodeConflict, "client_id_claim", "must differ from sub, tenant_claim, and roles_claim")
		}
	}
	switch {
	case len(policy.SigningAlgs) == 0:
		add(CodeRequired, "signing_algs", "needs at least one algorithm")
	case len(policy.SigningAlgs) > maxHTTPOAuthAlgorithms:
		add(CodeOutOfRange, "signing_algs", fmt.Sprintf("must hold at most %d algorithms", maxHTTPOAuthAlgorithms))
	}
	seenAlgorithms := make(map[string]struct{}, len(policy.SigningAlgs))
	for index, algorithm := range policy.SigningAlgs {
		elementPath := fmt.Sprintf("signing_algs[%d]", index)
		if _, supported := httpOAuthSigningAlgorithms[algorithm]; !supported {
			add(CodeInvalidEnum, elementPath, "must be one of RS256, RS384, RS512, PS256, PS384, PS512, ES256, ES384, ES512, or EdDSA")
			continue
		}
		if _, duplicate := seenAlgorithms[algorithm]; duplicate {
			add(CodeDuplicate, elementPath, "is repeated")
		}
		seenAlgorithms[algorithm] = struct{}{}
	}
	switch {
	case len(policy.AllowedClientIDs) == 0:
		add(CodeRequired, "allowed_client_ids", "needs at least one client ID")
	case len(policy.AllowedClientIDs) > maxHTTPOAuthClientIDs:
		add(CodeOutOfRange, "allowed_client_ids", fmt.Sprintf("must hold at most %d client IDs", maxHTTPOAuthClientIDs))
	}
	seenClients := make(map[string]struct{}, len(policy.AllowedClientIDs))
	for index, clientID := range policy.AllowedClientIDs {
		elementPath := fmt.Sprintf("allowed_client_ids[%d]", index)
		if !validIdentity(clientID) {
			add(CodeInvalidValue, elementPath, "must be at most 256 characters with no whitespace")
			continue
		}
		if _, duplicate := seenClients[clientID]; duplicate {
			add(CodeDuplicate, elementPath, "is repeated")
		}
		seenClients[clientID] = struct{}{}
	}
	return problems
}

// oauthIssuerProblem mirrors requestsecurity.validateOIDCIssuerURL.
func oauthIssuerProblem(value string) string {
	if value == "" {
		return "required"
	}
	if len(value) > maxHTTPOAuthURLBytes || !canonicalText(value) {
		return "must be at most 2048 characters with no whitespace or control characters"
	}
	parsed, err := url.Parse(value)
	switch {
	case err != nil:
		return "is not a URL"
	case parsed.Scheme != "https":
		return "must use the https scheme"
	case parsed.Hostname() == "":
		return "must name a host"
	case parsed.User != nil:
		return "must not carry credentials"
	case parsed.RawQuery != "" || parsed.Fragment != "":
		return "must not carry a query or fragment"
	}
	return ""
}

// canonicalText has no surrounding whitespace and no control character.
func canonicalText(value string) bool {
	if strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

var httpSourcePathPattern = regexp.MustCompile(`^/[A-Za-z0-9._~/-]*$`)

// validHTTPSourcePath is a clean absolute URL path of unreserved characters.
func validHTTPSourcePath(value string) bool {
	return len(value) <= maxHTTPSourcePathBytes && httpSourcePathPattern.MatchString(value) &&
		path.Clean(value) == value
}

func (r HTTPSourceRevision) semanticDigest() (string, error) {
	canonical := r
	canonical.Digest = ""
	canonical.OAuth = canonicalHTTPOAuth(r.OAuth)
	encoded, err := json.Marshal(canonical)
	if err != nil {
		return "", err
	}
	hasher := sha256.New()
	_, _ = hasher.Write([]byte(httpSourceDigestDomain))
	_, _ = hasher.Write(encoded)
	return "sha256:" + hex.EncodeToString(hasher.Sum(nil)), nil
}

func cloneHTTPOAuth(policy *HTTPOAuthPolicy) *HTTPOAuthPolicy {
	if policy == nil {
		return nil
	}
	clone := *policy
	clone.SigningAlgs = append([]string(nil), policy.SigningAlgs...)
	clone.AllowedClientIDs = append([]string(nil), policy.AllowedClientIDs...)
	return &clone
}

// canonicalHTTPOAuth orders the two sets so the digest does not depend on the
// order an author listed them in.
func canonicalHTTPOAuth(policy *HTTPOAuthPolicy) *HTTPOAuthPolicy {
	clone := cloneHTTPOAuth(policy)
	if clone == nil {
		return nil
	}
	sort.Strings(clone.SigningAlgs)
	sort.Strings(clone.AllowedClientIDs)
	return clone
}
