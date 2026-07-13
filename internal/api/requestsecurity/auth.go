// Package requestsecurity authenticates API transports and carries a trusted
// single-deployment security context to application adapters.
package requestsecurity

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

const minimumBearerTokenBytes = 24

var (
	// ErrMissingCredentials means the transport supplied no credential.
	ErrMissingCredentials = errors.New("authentication required")
	// ErrInvalidCredentials deliberately does not reveal why a credential failed.
	ErrInvalidCredentials = errors.New("invalid credentials")
)

// Authenticator turns one transport credential into server-owned identity data.
type Authenticator interface {
	Authenticate(ctx context.Context, authorization string) (integration.SecurityContext, error)
}

// StaticBearerConfig configures the temporary single-security-domain bearer
// boundary used before the Phase 4 OIDC/RBAC implementation.
type StaticBearerConfig struct {
	Token       string
	TenantID    string
	PrincipalID string
	Roles       []string
}

// StaticBearerAuthenticator performs constant-time comparison against one
// deployment secret and never accepts identity claims from the caller.
type StaticBearerAuthenticator struct {
	tokenHash [sha256.Size]byte
	security  integration.SecurityContext
}

// NewStaticBearerAuthenticator validates and defensively copies deployment config.
func NewStaticBearerAuthenticator(config StaticBearerConfig) (*StaticBearerAuthenticator, error) {
	if !validBearerToken(config.Token) {
		return nil, fmt.Errorf("bearer token must contain at least %d canonical RFC 6750 token bytes", minimumBearerTokenBytes)
	}
	if err := validateIdentity("tenant ID", config.TenantID); err != nil {
		return nil, err
	}
	if err := validateIdentity("principal ID", config.PrincipalID); err != nil {
		return nil, err
	}
	if len(config.Roles) == 0 {
		return nil, fmt.Errorf("at least one role is required")
	}
	roles := make([]string, len(config.Roles))
	seen := make(map[string]struct{}, len(config.Roles))
	for index, role := range config.Roles {
		if err := validateIdentity("role", role); err != nil {
			return nil, err
		}
		if _, exists := seen[role]; exists {
			return nil, fmt.Errorf("role %q is duplicated", role)
		}
		seen[role] = struct{}{}
		roles[index] = role
	}
	return &StaticBearerAuthenticator{
		tokenHash: sha256.Sum256([]byte(config.Token)),
		security: integration.SecurityContext{
			TenantID: config.TenantID,
			Principal: integration.Principal{
				ID:         config.PrincipalID,
				Kind:       integration.PrincipalKindHuman,
				AuthMethod: "bearer",
				Roles:      roles,
			},
		},
	}, nil
}

// Authenticate validates an RFC 6750-style Authorization value.
func (a *StaticBearerAuthenticator) Authenticate(ctx context.Context, authorization string) (integration.SecurityContext, error) {
	if ctx == nil {
		return integration.SecurityContext{}, ErrInvalidCredentials
	}
	if err := ctx.Err(); err != nil {
		return integration.SecurityContext{}, err
	}
	if strings.TrimSpace(authorization) == "" {
		return integration.SecurityContext{}, ErrMissingCredentials
	}
	// RFC 6750 uses one ASCII SP between the scheme and credentials. Do not
	// normalize tabs, controls, Unicode whitespace, or repeated spaces because
	// intermediaries can disagree about their meaning.
	parts := strings.Split(authorization, " ")
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" || containsControl(authorization) || a == nil {
		return integration.SecurityContext{}, ErrInvalidCredentials
	}
	candidateHash := sha256.Sum256([]byte(parts[1]))
	if subtle.ConstantTimeCompare(candidateHash[:], a.tokenHash[:]) != 1 {
		return integration.SecurityContext{}, ErrInvalidCredentials
	}
	return cloneSecurityContext(a.security), nil
}

type securityContextKey struct{}

// WithSecurityContext stores a defensive copy of authenticated identity data.
func WithSecurityContext(ctx context.Context, security integration.SecurityContext) context.Context {
	return context.WithValue(ctx, securityContextKey{}, cloneSecurityContext(security))
}

// SecurityContextFromContext returns a defensive copy of authenticated identity data.
func SecurityContextFromContext(ctx context.Context) (integration.SecurityContext, bool) {
	if ctx == nil {
		return integration.SecurityContext{}, false
	}
	security, ok := ctx.Value(securityContextKey{}).(integration.SecurityContext)
	if !ok {
		return integration.SecurityContext{}, false
	}
	return cloneSecurityContext(security), true
}

func cloneSecurityContext(security integration.SecurityContext) integration.SecurityContext {
	security.Principal.Roles = append([]string(nil), security.Principal.Roles...)
	return security
}

func validateIdentity(label, value string) error {
	if value == "" || len(value) > 256 || strings.TrimSpace(value) != value || containsControl(value) {
		return fmt.Errorf("%s is invalid", label)
	}
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("._:/@+-", r) {
			continue
		}
		return fmt.Errorf("%s is invalid", label)
	}
	return nil
}

func containsControl(value string) bool {
	for _, r := range value {
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
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
