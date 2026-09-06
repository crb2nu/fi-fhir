package requestsecurity

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

const (
	// CloudflareAccessAssertionHeader is set by Cloudflare's edge on every
	// request that passed a Cloudflare Access policy. The browser never sends
	// it; the browser sends the CF_Authorization cookie, which the edge
	// validates and forwards to the origin as this header.
	CloudflareAccessAssertionHeader = "Cf-Access-Jwt-Assertion"
	// CloudflareAccessCookie is the browser-held copy of the same token, read
	// only when the header is absent (a direct on-LAN request, for example).
	CloudflareAccessCookie = "CF_Authorization"

	cloudflareAccessAuthMethod    = "cloudflare-access"
	cloudflareAccessTokenType     = "app"
	maxCloudflareAccessEmailBytes = 254
)

// CloudflareAccessConfig trusts one Cloudflare Access application in front of
// this deployment. Access decides who may reach the origin at all (its policy,
// its identity provider); this configuration decides what each admitted
// identity may do here, by mapping the verified email claim to roles.
type CloudflareAccessConfig struct {
	// TeamDomainURL is the Access team domain, https://<team>.cloudflareaccess.com.
	// It is the token issuer and serves OIDC discovery and the signing keys.
	TeamDomainURL string
	// Audience is the Access application's AUD tag. Tokens for any other
	// application on the same team domain are rejected.
	Audience string
	TenantID string
	// Principals maps an email address (matched case-insensitively) to the
	// roles that identity holds. An identity Access admits but this map does
	// not name is rejected — fail closed, because Access policies are edited
	// outside this repository.
	Principals map[string][]string
	// HTTPClient optionally supplies private trust roots or other deployment
	// transport settings. The client is cloned and hardened before use.
	HTTPClient *http.Client
	// JWKSRefreshMinInterval bounds outbound refreshes caused by unknown key IDs.
	// Zero selects the hardened default; negative values are rejected.
	JWKSRefreshMinInterval time.Duration
}

// CloudflareAccessAuthenticator verifies the signed assertion Cloudflare Access
// attaches to a request and projects the deployment's own view of that
// identity. It never accepts roles from the token: Access application tokens
// carry none, and a claim the caller could influence would not be a bound.
type CloudflareAccessAuthenticator struct {
	verifier   *oidc.IDTokenVerifier
	audience   string
	tenantID   string
	principals map[string][]string
}

// NewCloudflareAccessAuthenticator discovers the team domain and creates a
// long-lived verifier, the same way NewOIDCAuthenticator does for a bearer
// issuer.
func NewCloudflareAccessAuthenticator(ctx context.Context, config CloudflareAccessConfig) (*CloudflareAccessAuthenticator, error) {
	if ctx == nil {
		return nil, fmt.Errorf("Cloudflare Access discovery context is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	issuerURL, err := validateOIDCIssuerURL(config.TeamDomainURL)
	if err != nil {
		return nil, fmt.Errorf("Cloudflare Access team domain: %w", err)
	}
	if err := validateOIDCAudience(config.Audience); err != nil {
		return nil, fmt.Errorf("Cloudflare Access audience is invalid")
	}
	if err := validateIdentity("tenant ID", config.TenantID); err != nil {
		return nil, err
	}
	principals, err := validateAccessPrincipals(config.Principals)
	if err != nil {
		return nil, err
	}
	// Access signs application tokens with RS256 only.
	verifier, err := discoverOIDCVerifier(ctx, issuerURL, config.Audience, oidcDiscoveryOptions{
		supportedSigningAlgs:   []string{oidc.RS256},
		httpClient:             config.HTTPClient,
		jwksRefreshMinInterval: config.JWKSRefreshMinInterval,
	})
	if err != nil {
		return nil, err
	}
	return &CloudflareAccessAuthenticator{
		verifier:   verifier,
		audience:   config.Audience,
		tenantID:   config.TenantID,
		principals: principals,
	}, nil
}

// Assertion extracts the Access token from a request: the edge-injected header
// first, then the cookie the browser holds. ok is false when the request
// carries neither, so a caller can fall through to its other credentials
// without treating absence as a failed attempt.
func (a *CloudflareAccessAuthenticator) Assertion(request *http.Request) (string, bool) {
	if a == nil || request == nil {
		return "", false
	}
	if value := request.Header.Get(CloudflareAccessAssertionHeader); value != "" {
		return value, true
	}
	if cookie, err := request.Cookie(CloudflareAccessCookie); err == nil && cookie.Value != "" {
		return cookie.Value, true
	}
	return "", false
}

// Authenticate verifies one Access application token and maps its email to
// the roles this deployment configured for it.
func (a *CloudflareAccessAuthenticator) Authenticate(ctx context.Context, assertion string) (integration.SecurityContext, error) {
	if a == nil || a.verifier == nil || ctx == nil {
		return integration.SecurityContext{}, ErrInvalidCredentials
	}
	if err := ctx.Err(); err != nil {
		return integration.SecurityContext{}, err
	}
	if strings.TrimSpace(assertion) == "" {
		return integration.SecurityContext{}, ErrMissingCredentials
	}
	if containsControl(assertion) || !validCloudflareAccessTokenHeader(assertion) {
		return integration.SecurityContext{}, ErrInvalidCredentials
	}

	token, err := a.verifier.Verify(ctx, assertion)
	if err != nil {
		return integration.SecurityContext{}, ErrInvalidCredentials
	}
	// go-oidc proves audience membership; require exact equality so a token
	// minted for several applications cannot broaden its use here.
	if len(token.Audience) != 1 || token.Audience[0] != a.audience {
		return integration.SecurityContext{}, ErrInvalidCredentials
	}

	claims := make(map[string]json.RawMessage)
	if err := token.Claims(&claims); err != nil {
		return integration.SecurityContext{}, ErrInvalidCredentials
	}
	// Only identity-based application tokens carry an email. Service tokens
	// (type "service", common_name instead of email) and the short-lived
	// "meta" tokens Access uses during login are not people, and must keep
	// using the bearer credential if they need the API at all.
	var tokenType string
	if raw, ok := claims["type"]; !ok || json.Unmarshal(raw, &tokenType) != nil || tokenType != cloudflareAccessTokenType {
		return integration.SecurityContext{}, ErrInvalidCredentials
	}
	email, ok := accessEmailClaim(claims["email"])
	if !ok {
		return integration.SecurityContext{}, ErrInvalidCredentials
	}
	roles, mapped := a.principals[email]
	if !mapped {
		return integration.SecurityContext{}, ErrInvalidCredentials
	}

	return integration.SecurityContext{
		TenantID: a.tenantID,
		Principal: integration.Principal{
			ID:         email,
			Kind:       integration.PrincipalKindHuman,
			AuthMethod: cloudflareAccessAuthMethod,
			Roles:      append([]string(nil), roles...),
		},
	}, nil
}

// validCloudflareAccessTokenHeader accepts the protected header Access emits:
// a compact JWS whose typ is "JWT" or absent. The bearer verifiers require
// typ "at+jwt"; keeping the two classes disjoint means a token cannot be
// replayed from one verifier to the other.
func validCloudflareAccessTokenHeader(token string) bool {
	if len(token) == 0 || len(token) > maxOIDCBearerTokenBytes {
		return false
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 || len(parts[0]) == 0 || len(parts[0]) > base64.RawURLEncoding.EncodedLen(maxOIDCProtectedHeaderBytes) {
		return false
	}
	header, err := base64.RawURLEncoding.Strict().DecodeString(parts[0])
	if err != nil || len(header) == 0 || len(header) > maxOIDCProtectedHeaderBytes {
		return false
	}
	values, ok := uniqueJSONObject(header)
	if !ok {
		return false
	}
	raw, present := values["typ"]
	if !present {
		return true
	}
	var tokenType string
	if err := json.Unmarshal(raw, &tokenType); err != nil {
		return false
	}
	return strings.EqualFold(tokenType, "JWT")
}

// accessEmailClaim decodes and canonicalises the email claim. Addresses are
// compared case-insensitively because identity providers disagree about case
// and the map key must not.
func accessEmailClaim(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 {
		return "", false
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", false
	}
	return canonicalAccessEmail(value)
}

func canonicalAccessEmail(value string) (string, bool) {
	if value == "" || len(value) > maxCloudflareAccessEmailBytes || strings.TrimSpace(value) != value || containsControl(value) {
		return "", false
	}
	at := strings.IndexByte(value, '@')
	if at <= 0 || at != strings.LastIndexByte(value, '@') || at == len(value)-1 {
		return "", false
	}
	if strings.ContainsAny(value, " \t\"<>,;") {
		return "", false
	}
	return strings.ToLower(value), true
}

func validateAccessPrincipals(configured map[string][]string) (map[string][]string, error) {
	if len(configured) == 0 {
		return nil, fmt.Errorf("Cloudflare Access principals must not be empty")
	}
	principals := make(map[string][]string, len(configured))
	for email, roles := range configured {
		canonical, ok := canonicalAccessEmail(email)
		if !ok {
			return nil, fmt.Errorf("Cloudflare Access principal %q is not an email address", email)
		}
		if _, duplicate := principals[canonical]; duplicate {
			return nil, fmt.Errorf("Cloudflare Access principal %q is duplicated (addresses are case-insensitive)", canonical)
		}
		if len(roles) == 0 {
			return nil, fmt.Errorf("Cloudflare Access principal %q needs at least one role", canonical)
		}
		seen := make(map[string]struct{}, len(roles))
		copied := make([]string, 0, len(roles))
		for _, role := range roles {
			if err := validateIdentity("role", role); err != nil {
				return nil, fmt.Errorf("Cloudflare Access principal %q: %w", canonical, err)
			}
			if _, exists := seen[role]; exists {
				return nil, fmt.Errorf("Cloudflare Access principal %q: role %q is duplicated", canonical, role)
			}
			seen[role] = struct{}{}
			copied = append(copied, role)
		}
		// Canonical order, as strictRolesClaim does, so identical grants produce
		// identical security contexts.
		slices.Sort(copied)
		principals[canonical] = copied
	}
	return principals, nil
}
