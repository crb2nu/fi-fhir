package requestsecurity

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/requestsecurity/oidctest"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

func TestOIDCAuthenticatorAuthenticate(t *testing.T) {
	issuer, err := oidctest.New()
	if err != nil {
		t.Fatalf("new OIDC issuer: %v", err)
	}
	t.Cleanup(issuer.Close)

	authenticator, err := NewOIDCAuthenticator(issuer.Context(), OIDCConfig{
		IssuerURL: issuer.IssuerURL(),
		Audience:  "fi-fhir-graphql",
		TenantID:  "tenant-a",
	})
	if err != nil {
		t.Fatalf("NewOIDCAuthenticator: %v", err)
	}

	claims := issuer.Claims()
	claims["roles"] = []string{"integration:preview", "author"}
	token := mustSignToken(t, issuer, claims, "RS256")
	security, err := authenticator.Authenticate(context.Background(), "Bearer "+token)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if security.TenantID != "tenant-a" || security.Principal.ID != "clinician-1" {
		t.Fatalf("security context = %#v", security)
	}
	if security.Principal.Kind != integration.PrincipalKindHuman || security.Principal.AuthMethod != "oidc" {
		t.Fatalf("principal = %#v", security.Principal)
	}
	if got := strings.Join(security.Principal.Roles, ","); got != "author,integration:preview" {
		t.Fatalf("roles = %q", got)
	}

	tests := []struct {
		name      string
		algorithm string
		mutate    func(map[string]any)
	}{
		{name: "wrong issuer", mutate: func(c map[string]any) { c["iss"] = "https://other.example.test" }},
		{name: "wrong audience", mutate: func(c map[string]any) { c["aud"] = "other-api" }},
		{name: "additional audience", mutate: func(c map[string]any) { c["aud"] = []string{"fi-fhir-graphql", "other-api"} }},
		{name: "expired", mutate: func(c map[string]any) { c["exp"] = time.Now().Add(-time.Minute).Unix() }},
		{name: "not before", mutate: func(c map[string]any) { c["nbf"] = time.Now().Add(10 * time.Minute).Unix() }},
		{name: "missing subject", mutate: func(c map[string]any) { delete(c, "sub") }},
		{name: "blank subject", mutate: func(c map[string]any) { c["sub"] = "" }},
		{name: "missing tenant", mutate: func(c map[string]any) { delete(c, "tenant_id") }},
		{name: "cross tenant", mutate: func(c map[string]any) { c["tenant_id"] = "tenant-b" }},
		{name: "non string tenant", mutate: func(c map[string]any) { c["tenant_id"] = []string{"tenant-a"} }},
		{name: "missing roles", mutate: func(c map[string]any) { delete(c, "roles") }},
		{name: "string roles", mutate: func(c map[string]any) { c["roles"] = "integration:preview" }},
		{name: "empty roles", mutate: func(c map[string]any) { c["roles"] = []string{} }},
		{name: "duplicate roles", mutate: func(c map[string]any) { c["roles"] = []string{"author", "author"} }},
		{name: "invalid role", mutate: func(c map[string]any) { c["roles"] = []string{" bad-role"} }},
		{name: "wrong algorithm", algorithm: "RS384"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims := issuer.Claims()
			if tt.mutate != nil {
				tt.mutate(claims)
			}
			algorithm := tt.algorithm
			if algorithm == "" {
				algorithm = "RS256"
			}
			token := mustSignToken(t, issuer, claims, algorithm)
			_, err := authenticator.Authenticate(context.Background(), "Bearer "+token)
			if !errors.Is(err, ErrInvalidCredentials) {
				t.Fatalf("Authenticate error = %v, want %v", err, ErrInvalidCredentials)
			}
			if err.Error() != ErrInvalidCredentials.Error() || strings.Contains(err.Error(), token) {
				t.Fatalf("authentication disclosed verification detail: %q", err)
			}
		})
	}

	attacker, err := oidctest.New()
	if err != nil {
		t.Fatalf("new attacker issuer: %v", err)
	}
	t.Cleanup(attacker.Close)
	attackerClaims := attacker.Claims()
	attackerClaims["iss"] = issuer.IssuerURL()
	forged := mustSignToken(t, attacker, attackerClaims, "RS256")
	if _, err := authenticator.Authenticate(context.Background(), "Bearer "+forged); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("forged signature error = %v, want %v", err, ErrInvalidCredentials)
	}
}

func TestOIDCAuthenticatorSupportsConfiguredClaimsAndAlgorithm(t *testing.T) {
	issuer, err := oidctest.New()
	if err != nil {
		t.Fatalf("new OIDC issuer: %v", err)
	}
	t.Cleanup(issuer.Close)
	authenticator, err := NewOIDCAuthenticator(issuer.Context(), OIDCConfig{
		IssuerURL:            issuer.IssuerURL(),
		Audience:             "https://fi-fhir.example.test/graphql",
		TenantID:             "tenant-a",
		TenantClaim:          "organization",
		RolesClaim:           "groups",
		SupportedSigningAlgs: []string{"RS384"},
	})
	if err != nil {
		t.Fatalf("NewOIDCAuthenticator: %v", err)
	}
	claims := issuer.Claims()
	claims["sub"] = "auth0|clinician-1"
	claims["aud"] = "https://fi-fhir.example.test/graphql"
	delete(claims, "tenant_id")
	delete(claims, "roles")
	claims["organization"] = "tenant-a"
	claims["groups"] = []string{"integration:preview"}
	token := mustSignToken(t, issuer, claims, "RS384")
	if _, err := authenticator.Authenticate(context.Background(), "Bearer "+token); err != nil {
		t.Fatalf("Authenticate configured claims and algorithm: %v", err)
	}
}

func TestOIDCAuthenticatorUsesStrictBearerGrammar(t *testing.T) {
	issuer, err := oidctest.New()
	if err != nil {
		t.Fatalf("new OIDC issuer: %v", err)
	}
	t.Cleanup(issuer.Close)
	authenticator, err := NewOIDCAuthenticator(issuer.Context(), OIDCConfig{
		IssuerURL: issuer.IssuerURL(), Audience: "fi-fhir-graphql", TenantID: "tenant-a",
	})
	if err != nil {
		t.Fatalf("NewOIDCAuthenticator: %v", err)
	}
	token := mustSignToken(t, issuer, issuer.Claims(), "RS256")

	tests := []struct {
		name          string
		authorization string
		want          error
	}{
		{name: "missing", want: ErrMissingCredentials},
		{name: "wrong scheme", authorization: "Basic " + token, want: ErrInvalidCredentials},
		{name: "tab separator", authorization: "Bearer\t" + token, want: ErrInvalidCredentials},
		{name: "repeated separator", authorization: "Bearer  " + token, want: ErrInvalidCredentials},
		{name: "extra credential", authorization: "Bearer " + token + " extra", want: ErrInvalidCredentials},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := authenticator.Authenticate(context.Background(), tt.authorization); !errors.Is(err, tt.want) {
				t.Fatalf("Authenticate error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestOIDCAuthenticatorRefreshesUnknownJWKSKey(t *testing.T) {
	issuer, err := oidctest.New()
	if err != nil {
		t.Fatalf("new OIDC issuer: %v", err)
	}
	t.Cleanup(issuer.Close)
	authenticator, err := NewOIDCAuthenticator(issuer.Context(), OIDCConfig{
		IssuerURL:              issuer.IssuerURL(),
		Audience:               "fi-fhir-graphql",
		TenantID:               "tenant-a",
		JWKSRefreshMinInterval: time.Second,
	})
	if err != nil {
		t.Fatalf("NewOIDCAuthenticator: %v", err)
	}

	first := mustSignToken(t, issuer, issuer.Claims(), "RS256")
	if _, err := authenticator.Authenticate(context.Background(), "Bearer "+first); err != nil {
		t.Fatalf("authenticate initial key: %v", err)
	}
	firstFetches := issuer.JWKSRequests()
	if firstFetches != 1 {
		t.Fatalf("initial JWKS requests = %d, want 1", firstFetches)
	}
	if _, err := authenticator.Authenticate(context.Background(), "Bearer "+first); err != nil {
		t.Fatalf("authenticate cached key: %v", err)
	}
	if got := issuer.JWKSRequests(); got != firstFetches {
		t.Fatalf("cached key caused JWKS refetch: got %d, want %d", got, firstFetches)
	}

	if err := issuer.Rotate("key-2"); err != nil {
		t.Fatalf("rotate key: %v", err)
	}
	rotated := mustSignToken(t, issuer, issuer.Claims(), "RS256")
	for attempt := 0; attempt < 5; attempt++ {
		if _, err := authenticator.Authenticate(context.Background(), "Bearer "+rotated); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("rate-limited rotation error = %v, want %v", err, ErrInvalidCredentials)
		}
	}
	if got := issuer.JWKSRequests(); got != firstFetches {
		t.Fatalf("unknown key bypassed refresh bound: got %d requests, want %d", got, firstFetches)
	}
	time.Sleep(1100 * time.Millisecond)
	if _, err := authenticator.Authenticate(context.Background(), "Bearer "+rotated); err != nil {
		t.Fatalf("authenticate rotated key after refresh bound: %v", err)
	}
	if got := issuer.JWKSRequests(); got != firstFetches+1 {
		t.Fatalf("rotated key JWKS requests = %d, want %d", got, firstFetches+1)
	}
}

func TestOIDCAuthenticatorRequiresAccessTokenTypeAndBoundsHeader(t *testing.T) {
	issuer, err := oidctest.New()
	if err != nil {
		t.Fatalf("new OIDC issuer: %v", err)
	}
	t.Cleanup(issuer.Close)
	authenticator, err := NewOIDCAuthenticator(issuer.Context(), OIDCConfig{
		IssuerURL: issuer.IssuerURL(), Audience: "fi-fhir-graphql", TenantID: "tenant-a",
	})
	if err != nil {
		t.Fatalf("NewOIDCAuthenticator: %v", err)
	}

	tests := []struct {
		name  string
		token func() string
	}{
		{name: "wrong token type", token: func() string {
			token, signErr := issuer.SignWithType(issuer.Claims(), "RS256", "JWT")
			if signErr != nil {
				t.Fatalf("sign wrong-type token: %v", signErr)
			}
			return token
		}},
		{name: "missing token type", token: func() string {
			token, signErr := issuer.SignWithType(issuer.Claims(), "RS256", "")
			if signErr != nil {
				t.Fatalf("sign typeless token: %v", signErr)
			}
			return token
		}},
		{name: "oversized bearer", token: func() string {
			return strings.Repeat("a", maxOIDCBearerTokenBytes+1)
		}},
		{name: "oversized protected header", token: func() string {
			return strings.Repeat("a", base64HeaderLimit()+1) + ".payload.signature"
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			token := test.token()
			_, err := authenticator.Authenticate(context.Background(), "Bearer "+token)
			assertGenericInvalidCredentials(t, err, token)
		})
	}
	if got := issuer.JWKSRequests(); got != 0 {
		t.Fatalf("rejected token headers caused %d JWKS requests, want 0", got)
	}
}

func TestOIDCAuthenticatorRejectsUnsafeDiscoveredJWKSTransport(t *testing.T) {
	t.Run("HTTP jwks_uri", func(t *testing.T) {
		issuer, err := oidctest.New()
		if err != nil {
			t.Fatalf("new OIDC issuer: %v", err)
		}
		t.Cleanup(issuer.Close)
		issuer.SetJWKSURI("http://keys.example.test/jwks.json")
		if _, err := NewOIDCAuthenticator(issuer.Context(), OIDCConfig{
			IssuerURL: issuer.IssuerURL(), Audience: "fi-fhir-graphql", TenantID: "tenant-a",
		}); err == nil || !strings.Contains(err.Error(), "absolute HTTPS") {
			t.Fatalf("NewOIDCAuthenticator error = %v, want safe JWKS URL rejection", err)
		}
	})

	t.Run("HTTPS redirect to HTTP", func(t *testing.T) {
		var insecureRequests atomic.Int64
		insecure := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			insecureRequests.Add(1)
			http.Error(w, "must not be reached", http.StatusInternalServerError)
		}))
		t.Cleanup(insecure.Close)

		issuer, err := oidctest.New()
		if err != nil {
			t.Fatalf("new OIDC issuer: %v", err)
		}
		t.Cleanup(issuer.Close)
		issuer.SetJWKSRedirect(insecure.URL + "/keys")
		authenticator, err := NewOIDCAuthenticator(issuer.Context(), OIDCConfig{
			IssuerURL: issuer.IssuerURL(), Audience: "fi-fhir-graphql", TenantID: "tenant-a",
		})
		if err != nil {
			t.Fatalf("NewOIDCAuthenticator: %v", err)
		}
		token := mustSignToken(t, issuer, issuer.Claims(), "RS256")
		_, err = authenticator.Authenticate(context.Background(), "Bearer "+token)
		assertGenericInvalidCredentials(t, err, token)
		if got := insecureRequests.Load(); got != 0 {
			t.Fatalf("followed HTTPS-to-HTTP JWKS redirect %d times", got)
		}
	})

	t.Run("HTTPS redirect is not followed", func(t *testing.T) {
		issuer, err := oidctest.New()
		if err != nil {
			t.Fatalf("new OIDC issuer: %v", err)
		}
		t.Cleanup(issuer.Close)
		issuer.SetJWKSRedirect(issuer.IssuerURL() + "/keys")
		authenticator, err := NewOIDCAuthenticator(issuer.Context(), OIDCConfig{
			IssuerURL: issuer.IssuerURL(), Audience: "fi-fhir-graphql", TenantID: "tenant-a",
		})
		if err != nil {
			t.Fatalf("NewOIDCAuthenticator: %v", err)
		}
		token := mustSignToken(t, issuer, issuer.Claims(), "RS256")
		_, err = authenticator.Authenticate(context.Background(), "Bearer "+token)
		assertGenericInvalidCredentials(t, err, token)
		if got := issuer.JWKSRequests(); got != 0 {
			t.Fatalf("followed HTTPS JWKS redirect %d times", got)
		}
	})
}

func TestOIDCAuthenticatorBoundsStalledJWKSFetch(t *testing.T) {
	issuer, err := oidctest.New()
	if err != nil {
		t.Fatalf("new OIDC issuer: %v", err)
	}
	t.Cleanup(issuer.Close)
	issuer.SetJWKSDelay(500 * time.Millisecond)
	client := *issuer.HTTPClient()
	client.Timeout = 40 * time.Millisecond
	authenticator, err := NewOIDCAuthenticator(context.Background(), OIDCConfig{
		IssuerURL: issuer.IssuerURL(), Audience: "fi-fhir-graphql", TenantID: "tenant-a", HTTPClient: &client,
	})
	if err != nil {
		t.Fatalf("NewOIDCAuthenticator: %v", err)
	}
	token := mustSignToken(t, issuer, issuer.Claims(), "RS256")
	started := time.Now()
	_, err = authenticator.Authenticate(context.Background(), "Bearer "+token)
	assertGenericInvalidCredentials(t, err, token)
	if elapsed := time.Since(started); elapsed > 250*time.Millisecond {
		t.Fatalf("stalled JWKS fetch took %s, want bounded client timeout", elapsed)
	}
	if got := issuer.JWKSRequests(); got != 1 {
		t.Fatalf("stalled JWKS requests = %d, want 1", got)
	}
}

func TestHardenedOIDCHTTPClientRejectsCallerRedirectPolicy(t *testing.T) {
	configured := &http.Client{
		Timeout: time.Hour,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return nil
		},
	}
	client := hardenedOIDCHTTPClient(configured)
	if client.Timeout != defaultOIDCHTTPTimeout {
		t.Fatalf("hardened timeout = %s, want %s", client.Timeout, defaultOIDCHTTPTimeout)
	}
	request := &http.Request{URL: mustURL(t, "https://issuer.example.test/redirect")}
	if err := client.CheckRedirect(request, nil); err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("HTTPS redirect rejection error = %v", err)
	}
	request.URL = mustURL(t, "http://issuer.example.test/downgrade")
	if err := client.CheckRedirect(request, nil); err == nil || !strings.Contains(err.Error(), "HTTPS") {
		t.Fatalf("redirect downgrade error = %v", err)
	}
}

func TestHardenedOIDCHTTPClientBoundsEveryResponseBody(t *testing.T) {
	client := hardenedOIDCHTTPClient(&http.Client{Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode:    http.StatusOK,
			Body:          io.NopCloser(strings.NewReader(strings.Repeat("x", maxOIDCResponseBytes+1))),
			ContentLength: -1,
			Request:       request,
		}, nil
	})})
	response, err := client.Get("https://issuer.example.test/.well-known/openid-configuration")
	if err != nil {
		t.Fatalf("GET discovery response: %v", err)
	}
	t.Cleanup(func() { _ = response.Body.Close() })
	if _, err := io.ReadAll(response.Body); !errors.Is(err, errOIDCResponseTooLarge) {
		t.Fatalf("oversized response error = %v, want %v", err, errOIDCResponseTooLarge)
	}
}

func TestOIDCAuthenticatorRejectsUnsafeConfiguration(t *testing.T) {
	issuer, err := oidctest.New()
	if err != nil {
		t.Fatalf("new OIDC issuer: %v", err)
	}
	t.Cleanup(issuer.Close)

	tests := []struct {
		name   string
		ctx    context.Context
		config OIDCConfig
	}{
		{name: "nil context", config: OIDCConfig{IssuerURL: issuer.IssuerURL(), Audience: "api", TenantID: "tenant-a"}},
		{name: "insecure issuer", ctx: context.Background(), config: OIDCConfig{IssuerURL: "http://issuer.example.test", Audience: "api", TenantID: "tenant-a"}},
		{name: "issuer credentials", ctx: context.Background(), config: OIDCConfig{IssuerURL: "https://user@issuer.example.test", Audience: "api", TenantID: "tenant-a"}},
		{name: "issuer query", ctx: context.Background(), config: OIDCConfig{IssuerURL: "https://issuer.example.test?tenant=a", Audience: "api", TenantID: "tenant-a"}},
		{name: "issuer without hostname", ctx: context.Background(), config: OIDCConfig{IssuerURL: "https://:443", Audience: "api", TenantID: "tenant-a"}},
		{name: "missing audience", ctx: context.Background(), config: OIDCConfig{IssuerURL: issuer.IssuerURL(), TenantID: "tenant-a"}},
		{name: "missing tenant", ctx: context.Background(), config: OIDCConfig{IssuerURL: issuer.IssuerURL(), Audience: "api"}},
		{name: "same claim names", ctx: context.Background(), config: OIDCConfig{IssuerURL: issuer.IssuerURL(), Audience: "api", TenantID: "tenant-a", TenantClaim: "scope", RolesClaim: "scope"}},
		{name: "unsafe algorithm", ctx: context.Background(), config: OIDCConfig{IssuerURL: issuer.IssuerURL(), Audience: "api", TenantID: "tenant-a", SupportedSigningAlgs: []string{"HS256"}}},
		{name: "none algorithm", ctx: context.Background(), config: OIDCConfig{IssuerURL: issuer.IssuerURL(), Audience: "api", TenantID: "tenant-a", SupportedSigningAlgs: []string{"none"}}},
		{name: "duplicate algorithm", ctx: context.Background(), config: OIDCConfig{IssuerURL: issuer.IssuerURL(), Audience: "api", TenantID: "tenant-a", SupportedSigningAlgs: []string{"RS256", "RS256"}}},
		{name: "negative JWKS refresh bound", ctx: context.Background(), config: OIDCConfig{IssuerURL: issuer.IssuerURL(), Audience: "api", TenantID: "tenant-a", JWKSRefreshMinInterval: -time.Second}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewOIDCAuthenticator(tt.ctx, tt.config); err == nil {
				t.Fatal("unsafe OIDC configuration was accepted")
			}
		})
	}
}

func TestOIDCAuthenticatorHonorsContextCancellation(t *testing.T) {
	issuer, err := oidctest.New()
	if err != nil {
		t.Fatalf("new OIDC issuer: %v", err)
	}
	t.Cleanup(issuer.Close)
	authenticator, err := NewOIDCAuthenticator(issuer.Context(), OIDCConfig{
		IssuerURL: issuer.IssuerURL(), Audience: "fi-fhir-graphql", TenantID: "tenant-a",
	})
	if err != nil {
		t.Fatalf("NewOIDCAuthenticator: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := authenticator.Authenticate(ctx, "Bearer irrelevant"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Authenticate error = %v, want context cancellation", err)
	}
}

func mustSignToken(t *testing.T, issuer *oidctest.Fixture, claims map[string]any, algorithm string) string {
	t.Helper()
	token, err := issuer.Sign(claims, algorithm)
	if err != nil {
		t.Fatalf("sign OIDC token: %v", err)
	}
	return token
}

func assertGenericInvalidCredentials(t *testing.T, err error, credential string) {
	t.Helper()
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("Authenticate error = %v, want %v", err, ErrInvalidCredentials)
	}
	if err.Error() != ErrInvalidCredentials.Error() || strings.Contains(err.Error(), credential) {
		t.Fatalf("authentication disclosed verification detail: %q", err)
	}
}

func base64HeaderLimit() int {
	return (maxOIDCProtectedHeaderBytes + 2) / 3 * 4
}

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	value, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse URL %q: %v", raw, err)
	}
	return value
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (function roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}
