package requestsecurity_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/requestsecurity"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/requestsecurity/oidctest"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

const testAccessAudience = "6834f3234782f45980c750ae8711c0a95f9b834c2ccbc909ac47252c060c8e53"

// accessClaims is the claim set Cloudflare Access puts in an application token
// for an identity-based login: no roles, no tenant, an email, and type "app".
func accessClaims(fixture *oidctest.Fixture) map[string]any {
	claims := fixture.Claims()
	delete(claims, "roles")
	delete(claims, "tenant_id")
	claims["aud"] = []string{testAccessAudience}
	claims["sub"] = "4f1c7d1e-3a4b-4c6d-9e8f-0a1b2c3d4e5f"
	claims["email"] = "Cody@Flexinfer.ai"
	claims["type"] = "app"
	claims["identity_nonce"] = "nonce-1"
	claims["country"] = "US"
	return claims
}

func newAccessAuthenticator(t *testing.T, fixture *oidctest.Fixture, principals map[string][]string) *requestsecurity.CloudflareAccessAuthenticator {
	t.Helper()
	authenticator, err := requestsecurity.NewCloudflareAccessAuthenticator(fixture.Context(), requestsecurity.CloudflareAccessConfig{
		TeamDomainURL: fixture.IssuerURL(),
		Audience:      testAccessAudience,
		TenantID:      "tenant-a",
		Principals:    principals,
		HTTPClient:    fixture.HTTPClient(),
	})
	if err != nil {
		t.Fatalf("NewCloudflareAccessAuthenticator: %v", err)
	}
	return authenticator
}

func signAccess(t *testing.T, fixture *oidctest.Fixture, claims map[string]any) string {
	t.Helper()
	token, err := fixture.SignWithType(claims, "RS256", "JWT")
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return token
}

func TestCloudflareAccessAuthenticator_MapsVerifiedEmailToConfiguredRoles(t *testing.T) {
	fixture, err := oidctest.New()
	if err != nil {
		t.Fatal(err)
	}
	defer fixture.Close()
	authenticator := newAccessAuthenticator(t, fixture, map[string][]string{
		"cody@flexinfer.ai": {"graphql:operator", "integration:preview", "clinical:read"},
	})

	security, err := authenticator.Authenticate(context.Background(), signAccess(t, fixture, accessClaims(fixture)))
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if security.TenantID != "tenant-a" {
		t.Fatalf("tenant = %q", security.TenantID)
	}
	if security.Principal.ID != "cody@flexinfer.ai" {
		t.Fatalf("principal = %q, want the lower-cased email", security.Principal.ID)
	}
	if security.Principal.Kind != integration.PrincipalKindHuman || security.Principal.AuthMethod != "cloudflare-access" {
		t.Fatalf("principal kind/method = %q/%q", security.Principal.Kind, security.Principal.AuthMethod)
	}
	want := []string{"clinical:read", "graphql:operator", "integration:preview"}
	if len(security.Principal.Roles) != len(want) {
		t.Fatalf("roles = %v, want %v", security.Principal.Roles, want)
	}
	for index := range want {
		if security.Principal.Roles[index] != want[index] {
			t.Fatalf("roles = %v, want %v (sorted)", security.Principal.Roles, want)
		}
	}
}

func TestCloudflareAccessAuthenticator_RejectsWhatAccessAdmitsButTheDeploymentDoesNotName(t *testing.T) {
	fixture, err := oidctest.New()
	if err != nil {
		t.Fatal(err)
	}
	defer fixture.Close()
	authenticator := newAccessAuthenticator(t, fixture, map[string][]string{
		"someone-else@flexinfer.ai": {"integration:preview"},
	})

	_, err = authenticator.Authenticate(context.Background(), signAccess(t, fixture, accessClaims(fixture)))
	if !errors.Is(err, requestsecurity.ErrInvalidCredentials) {
		t.Fatalf("unmapped email: err = %v, want ErrInvalidCredentials", err)
	}
}

func TestCloudflareAccessAuthenticator_RejectsTokensThatAreNotThisApplicationsIdentityTokens(t *testing.T) {
	fixture, err := oidctest.New()
	if err != nil {
		t.Fatal(err)
	}
	defer fixture.Close()
	authenticator := newAccessAuthenticator(t, fixture, map[string][]string{
		"cody@flexinfer.ai": {"integration:preview"},
	})

	cases := map[string]func(map[string]any) string{
		"another application's audience": func(claims map[string]any) string {
			claims["aud"] = []string{"0000000000000000000000000000000000000000000000000000000000000000"}
			return signAccess(t, fixture, claims)
		},
		"two audiences, one of them ours": func(claims map[string]any) string {
			claims["aud"] = []string{testAccessAudience, "other"}
			return signAccess(t, fixture, claims)
		},
		"service token, no person behind it": func(claims map[string]any) string {
			delete(claims, "email")
			claims["type"] = "service"
			claims["common_name"] = "ci-bot.access"
			return signAccess(t, fixture, claims)
		},
		"login-time meta token": func(claims map[string]any) string {
			claims["type"] = "meta"
			return signAccess(t, fixture, claims)
		},
		"missing email": func(claims map[string]any) string {
			delete(claims, "email")
			return signAccess(t, fixture, claims)
		},
		"email that is not an address": func(claims map[string]any) string {
			claims["email"] = "not-an-address"
			return signAccess(t, fixture, claims)
		},
		"expired": func(claims map[string]any) string {
			claims["exp"] = claims["iat"].(int64) - 60
			return signAccess(t, fixture, claims)
		},
		"bearer access-token class (typ at+jwt) is a different verifier's business": func(claims map[string]any) string {
			token, err := fixture.Sign(claims, "RS256")
			if err != nil {
				t.Fatal(err)
			}
			return token
		},
		"unsupported algorithm": func(claims map[string]any) string {
			token, err := fixture.SignWithType(claims, "RS512", "JWT")
			if err != nil {
				t.Fatal(err)
			}
			return token
		},
		"not a JWT": func(map[string]any) string { return "definitely.not.a-jwt" },
	}
	for name, build := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := authenticator.Authenticate(context.Background(), build(accessClaims(fixture)))
			if !errors.Is(err, requestsecurity.ErrInvalidCredentials) {
				t.Fatalf("err = %v, want ErrInvalidCredentials", err)
			}
		})
	}

	t.Run("empty assertion is missing, not invalid", func(t *testing.T) {
		_, err := authenticator.Authenticate(context.Background(), "")
		if !errors.Is(err, requestsecurity.ErrMissingCredentials) {
			t.Fatalf("err = %v, want ErrMissingCredentials", err)
		}
	})
}

func TestCloudflareAccessAuthenticator_AssertionPrefersTheEdgeHeaderThenTheCookie(t *testing.T) {
	fixture, err := oidctest.New()
	if err != nil {
		t.Fatal(err)
	}
	defer fixture.Close()
	authenticator := newAccessAuthenticator(t, fixture, map[string][]string{"cody@flexinfer.ai": {"integration:preview"}})

	t.Run("header", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "/api/auth/status", nil)
		request.Header.Set(requestsecurity.CloudflareAccessAssertionHeader, "from-header")
		request.AddCookie(&http.Cookie{Name: requestsecurity.CloudflareAccessCookie, Value: "from-cookie"})
		if got, ok := authenticator.Assertion(request); !ok || got != "from-header" {
			t.Fatalf("assertion = %q, %v", got, ok)
		}
	})
	t.Run("cookie when the header is absent", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "/api/auth/status", nil)
		request.AddCookie(&http.Cookie{Name: requestsecurity.CloudflareAccessCookie, Value: "from-cookie"})
		if got, ok := authenticator.Assertion(request); !ok || got != "from-cookie" {
			t.Fatalf("assertion = %q, %v", got, ok)
		}
	})
	t.Run("neither", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "/api/auth/status", nil)
		if _, ok := authenticator.Assertion(request); ok {
			t.Fatal("assertion reported present on a bare request")
		}
	})
	t.Run("nil authenticator never finds one", func(t *testing.T) {
		var none *requestsecurity.CloudflareAccessAuthenticator
		request := httptest.NewRequest(http.MethodGet, "/api/auth/status", nil)
		request.Header.Set(requestsecurity.CloudflareAccessAssertionHeader, "from-header")
		if _, ok := none.Assertion(request); ok {
			t.Fatal("nil authenticator reported an assertion")
		}
	})
}

func TestNewCloudflareAccessAuthenticator_RejectsUnsafeConfiguration(t *testing.T) {
	fixture, err := oidctest.New()
	if err != nil {
		t.Fatal(err)
	}
	defer fixture.Close()
	valid := func() requestsecurity.CloudflareAccessConfig {
		return requestsecurity.CloudflareAccessConfig{
			TeamDomainURL: fixture.IssuerURL(),
			Audience:      testAccessAudience,
			TenantID:      "tenant-a",
			Principals:    map[string][]string{"cody@flexinfer.ai": {"integration:preview"}},
			HTTPClient:    fixture.HTTPClient(),
		}
	}
	cases := map[string]func(*requestsecurity.CloudflareAccessConfig){
		"no principals": func(c *requestsecurity.CloudflareAccessConfig) { c.Principals = nil },
		"principal is not an email": func(c *requestsecurity.CloudflareAccessConfig) {
			c.Principals = map[string][]string{"operator": {"integration:preview"}}
		},
		"principal without roles": func(c *requestsecurity.CloudflareAccessConfig) {
			c.Principals = map[string][]string{"cody@flexinfer.ai": nil}
		},
		"principal with an invalid role": func(c *requestsecurity.CloudflareAccessConfig) {
			c.Principals = map[string][]string{"cody@flexinfer.ai": {"bad role"}}
		},
		"principal with a duplicate role": func(c *requestsecurity.CloudflareAccessConfig) {
			c.Principals = map[string][]string{"cody@flexinfer.ai": {"integration:preview", "integration:preview"}}
		},
		"same address twice by case": func(c *requestsecurity.CloudflareAccessConfig) {
			c.Principals = map[string][]string{"cody@flexinfer.ai": {"integration:preview"}, "Cody@flexinfer.ai": {"integration:preview"}}
		},
		"team domain over http": func(c *requestsecurity.CloudflareAccessConfig) {
			c.TeamDomainURL = "http://flexinfer.cloudflareaccess.com"
		},
		"empty audience": func(c *requestsecurity.CloudflareAccessConfig) { c.Audience = "" },
		"invalid tenant": func(c *requestsecurity.CloudflareAccessConfig) { c.TenantID = "tenant a" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			config := valid()
			mutate(&config)
			if _, err := requestsecurity.NewCloudflareAccessAuthenticator(fixture.Context(), config); err == nil {
				t.Fatal("expected a configuration error")
			}
		})
	}
	if _, err := requestsecurity.NewCloudflareAccessAuthenticator(fixture.Context(), valid()); err != nil {
		t.Fatalf("valid configuration rejected: %v", err)
	}
}
