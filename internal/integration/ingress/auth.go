package ingress

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"unicode"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

const (
	minimumBearerTokenBytes = 24
	minimumHMACSecretBytes  = 32
	signatureHeader         = "X-Fi-Fhir-Signature"
	hmacDomain              = "fi-fhir/http-ingress/v1\n"
)

// AuthMode selects the deployment-owned credential scheme for one ingress.
type AuthMode string

const (
	AuthModeBearer AuthMode = "bearer"
	AuthModeHMAC   AuthMode = "hmac-sha256"
	AuthModeOAuth2 AuthMode = "oauth2"
)

var (
	ErrMissingCredentials = errors.New("authentication required")
	ErrInvalidCredentials = errors.New("invalid credentials")
)

// AuthConfig binds one credential to one service principal and integration.
// Source identity is resolved from the immutable registry, never from a header.
type AuthConfig struct {
	Mode          AuthMode
	Secret        string
	TenantID      string
	PrincipalID   string
	IntegrationID string
}

// RequestAuthenticator authenticates one HTTP request and returns only
// server-owned identity fields. Source identity is bound later from the exact
// immutable integration revision.
type RequestAuthenticator interface {
	IntegrationID() string
	RequiresBody() bool
	AuthenticateRequest(context.Context, *http.Request, []byte) (integration.SecurityContext, error)
}

// Authenticator verifies a bounded request without retaining the credential.
type Authenticator struct {
	mode          AuthMode
	secret        []byte
	bearerHash    [sha256.Size]byte
	tenantID      string
	principalID   string
	integrationID string
}

func NewAuthenticator(config AuthConfig) (*Authenticator, error) {
	if err := validateIdentity("tenant ID", config.TenantID); err != nil {
		return nil, err
	}
	if err := validateIdentity("principal ID", config.PrincipalID); err != nil {
		return nil, err
	}
	if err := validateIdentity("integration ID", config.IntegrationID); err != nil {
		return nil, err
	}
	authenticator := &Authenticator{
		mode:          config.Mode,
		tenantID:      config.TenantID,
		principalID:   config.PrincipalID,
		integrationID: config.IntegrationID,
	}
	switch config.Mode {
	case AuthModeBearer:
		if !validBearerToken(config.Secret) {
			return nil, fmt.Errorf("bearer token must contain at least %d canonical bytes", minimumBearerTokenBytes)
		}
		authenticator.bearerHash = sha256.Sum256([]byte(config.Secret))
	case AuthModeHMAC:
		if len(config.Secret) < minimumHMACSecretBytes || strings.TrimSpace(config.Secret) != config.Secret || containsControl(config.Secret) {
			return nil, fmt.Errorf("HMAC secret must contain at least %d canonical bytes", minimumHMACSecretBytes)
		}
		authenticator.secret = []byte(config.Secret)
	default:
		return nil, fmt.Errorf("unsupported HTTP ingress auth mode %q", config.Mode)
	}
	return authenticator, nil
}

func (a *Authenticator) IntegrationID() string {
	if a == nil {
		return ""
	}
	return a.integrationID
}

func (a *Authenticator) PrincipalID() string {
	if a == nil {
		return ""
	}
	return a.principalID
}

func (a *Authenticator) AuthMethod() string {
	if a == nil {
		return ""
	}
	return string(a.mode)
}

func (a *Authenticator) RequiresBody() bool {
	return a != nil && a.mode == AuthModeHMAC
}

// AuthenticateRequest verifies a static bearer or HMAC credential and returns
// the deployment-bound compatibility identity.
func (a *Authenticator) AuthenticateRequest(ctx context.Context, request *http.Request, body []byte) (integration.SecurityContext, error) {
	if ctx == nil || a == nil || request == nil {
		return integration.SecurityContext{}, ErrInvalidCredentials
	}
	if err := ctx.Err(); err != nil {
		return integration.SecurityContext{}, err
	}
	if err := a.Authenticate(request, body); err != nil {
		return integration.SecurityContext{}, err
	}
	return integration.SecurityContext{
		TenantID: a.tenantID,
		Principal: integration.Principal{
			ID:         a.principalID,
			Kind:       integration.PrincipalKindService,
			AuthMethod: string(a.mode),
			Roles:      []string{SubmitRole},
		},
	}, nil
}

// Authenticate verifies bearer credentials or a domain-separated HMAC over
// integration identity, idempotency key, correlation ID, and exact body bytes.
func (a *Authenticator) Authenticate(r *http.Request, body []byte) error {
	if a == nil || r == nil {
		return ErrInvalidCredentials
	}
	switch a.mode {
	case AuthModeBearer:
		return a.authenticateBearer(r)
	case AuthModeHMAC:
		return a.authenticateHMAC(r, body)
	default:
		return ErrInvalidCredentials
	}
}

func (a *Authenticator) authenticateBearer(r *http.Request) error {
	authorization, found := singleHeader(r.Header, "Authorization")
	if !found || authorization == "" {
		return ErrMissingCredentials
	}
	parts := strings.Split(authorization, " ")
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" || containsControl(authorization) {
		return ErrInvalidCredentials
	}
	candidate := sha256.Sum256([]byte(parts[1]))
	if subtle.ConstantTimeCompare(candidate[:], a.bearerHash[:]) != 1 {
		return ErrInvalidCredentials
	}
	return nil
}

func (a *Authenticator) authenticateHMAC(r *http.Request, body []byte) error {
	value, found := singleHeader(r.Header, signatureHeader)
	if !found || value == "" {
		return ErrMissingCredentials
	}
	hexDigest, ok := strings.CutPrefix(value, "sha256=")
	if !ok || len(hexDigest) != sha256.Size*2 {
		return ErrInvalidCredentials
	}
	provided, err := hex.DecodeString(hexDigest)
	if err != nil {
		return ErrInvalidCredentials
	}
	expected := hmac.New(sha256.New, a.secret)
	_, _ = expected.Write(signaturePayload(
		a.integrationID,
		r.Header.Get("Idempotency-Key"),
		r.Header.Get("X-Correlation-ID"),
		body,
	))
	if !hmac.Equal(provided, expected.Sum(nil)) {
		return ErrInvalidCredentials
	}
	return nil
}

func signaturePayload(integrationID, idempotencyKey, correlationID string, body []byte) []byte {
	prefix := hmacDomain + integrationID + "\n" + idempotencyKey + "\n" + correlationID + "\n"
	payload := make([]byte, 0, len(prefix)+len(body))
	payload = append(payload, prefix...)
	payload = append(payload, body...)
	return payload
}

func singleHeader(header http.Header, name string) (string, bool) {
	values := header.Values(name)
	if len(values) != 1 {
		return "", false
	}
	return values[0], true
}

func validBearerToken(value string) bool {
	if len(value) < minimumBearerTokenBytes {
		return false
	}
	padding := false
	for index := 0; index < len(value); index++ {
		character := value[index]
		if character == '=' {
			padding = true
			continue
		}
		if padding {
			return false
		}
		if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || strings.ContainsRune("-._~+/", rune(character)) {
			continue
		}
		return false
	}
	return true
}

func validateIdentity(label, value string) error {
	if value == "" || len(value) > 256 || strings.TrimSpace(value) != value || containsControl(value) {
		return fmt.Errorf("%s is invalid", label)
	}
	for _, character := range value {
		if unicode.IsLetter(character) || unicode.IsDigit(character) || strings.ContainsRune("._:/@+-", character) {
			continue
		}
		return fmt.Errorf("%s is invalid", label)
	}
	return nil
}

func containsControl(value string) bool {
	for _, character := range value {
		if unicode.IsControl(character) {
			return true
		}
	}
	return false
}
