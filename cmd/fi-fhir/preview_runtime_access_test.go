package main

import (
	"context"
	"strings"
	"testing"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/requestsecurity/oidctest"
)

const testAccessAudience = "6834f3234782f45980c750ae8711c0a95f9b834c2ccbc909ac47252c060c8e53"

func setStaticGraphQLEnv(t *testing.T) {
	t.Helper()
	clearGraphQLAuthenticationEnv(t)
	t.Setenv("FI_FHIR_GRAPHQL_AUTH_MODE", graphqlAuthModeStatic)
	t.Setenv("FI_FHIR_GRAPHQL_BEARER_TOKEN", "correct-horse-battery-staple")
	t.Setenv("FI_FHIR_GRAPHQL_PRINCIPAL_ID", "engineer-1")
	t.Setenv("FI_FHIR_GRAPHQL_ROLES", "integration:preview")
}

func TestLoadGraphQLAuthenticationWithoutAccessSettingsConfiguresNoAccessLayer(t *testing.T) {
	setStaticGraphQLEnv(t)
	_, _, access, err := loadGraphQLAuthenticationFromEnv(context.Background(), "tenant-a")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if access != nil {
		t.Fatal("no FI_FHIR_GRAPHQL_ACCESS_* setting was present, yet an Access authenticator was built")
	}
}

func TestLoadGraphQLAuthenticationLayersCloudflareAccessOnEitherMode(t *testing.T) {
	issuer, err := oidctest.New()
	if err != nil {
		t.Fatal(err)
	}
	defer issuer.Close()

	claims := issuer.Claims()
	delete(claims, "roles")
	delete(claims, "tenant_id")
	claims["aud"] = []string{testAccessAudience}
	claims["email"] = "Cody@Flexinfer.ai"
	claims["type"] = "app"
	assertion, err := issuer.SignWithType(claims, "RS256", "JWT")
	if err != nil {
		t.Fatal(err)
	}

	for _, mode := range []string{graphqlAuthModeStatic, graphqlAuthModeOIDC} {
		t.Run("mode="+mode, func(t *testing.T) {
			clearGraphQLAuthenticationEnv(t)
			t.Setenv("FI_FHIR_GRAPHQL_AUTH_MODE", mode)
			if mode == graphqlAuthModeStatic {
				t.Setenv("FI_FHIR_GRAPHQL_BEARER_TOKEN", "correct-horse-battery-staple")
				t.Setenv("FI_FHIR_GRAPHQL_PRINCIPAL_ID", "engineer-1")
				t.Setenv("FI_FHIR_GRAPHQL_ROLES", "integration:preview")
			} else {
				t.Setenv("FI_FHIR_GRAPHQL_OIDC_ISSUER_URL", issuer.IssuerURL())
				t.Setenv("FI_FHIR_GRAPHQL_OIDC_AUDIENCE", "fi-fhir-graphql")
			}
			t.Setenv(envGraphQLAccessTeamDomain, issuer.IssuerURL())
			t.Setenv(envGraphQLAccessAudience, testAccessAudience)
			t.Setenv(envGraphQLAccessPrincipals, "cody@flexinfer.ai=integration:preview,graphql:operator;auditor@flexinfer.ai=integration:preview,clinical:read")

			_, _, access, err := loadGraphQLAuthenticationFromEnv(issuer.Context(), "tenant-a")
			if err != nil {
				t.Fatalf("load: %v", err)
			}
			if access == nil {
				t.Fatal("Access settings were present, yet no Access authenticator was built")
			}
			security, err := access.Authenticate(issuer.Context(), assertion)
			if err != nil {
				t.Fatalf("Authenticate: %v", err)
			}
			if security.Principal.ID != "cody@flexinfer.ai" || security.Principal.AuthMethod != "cloudflare-access" {
				t.Fatalf("security = %#v", security)
			}
			if len(security.Principal.Roles) != 2 || security.Principal.Roles[0] != "graphql:operator" || security.Principal.Roles[1] != "integration:preview" {
				t.Fatalf("roles = %v", security.Principal.Roles)
			}
		})
	}
}

func TestLoadGraphQLAuthenticationRejectsPartialOrUnsafeAccessSettings(t *testing.T) {
	cases := map[string]struct {
		env  map[string]string
		want string
	}{
		"team domain alone": {
			env:  map[string]string{envGraphQLAccessTeamDomain: "https://flexinfer.cloudflareaccess.com"},
			want: "must be set together",
		},
		"audience and principals without a team domain": {
			env: map[string]string{
				envGraphQLAccessAudience:   testAccessAudience,
				envGraphQLAccessPrincipals: "cody@flexinfer.ai=integration:preview",
			},
			want: "must be set together",
		},
		"principal without the preview floor": {
			env: map[string]string{
				envGraphQLAccessTeamDomain: "https://flexinfer.cloudflareaccess.com",
				envGraphQLAccessAudience:   testAccessAudience,
				envGraphQLAccessPrincipals: "cody@flexinfer.ai=graphql:operator",
			},
			want: "must include \"integration:preview\"",
		},
		"malformed entry": {
			env: map[string]string{
				envGraphQLAccessTeamDomain: "https://flexinfer.cloudflareaccess.com",
				envGraphQLAccessAudience:   testAccessAudience,
				envGraphQLAccessPrincipals: "cody@flexinfer.ai",
			},
			want: "must be <email>=<role>,<role>",
		},
		"same principal twice": {
			env: map[string]string{
				envGraphQLAccessTeamDomain: "https://flexinfer.cloudflareaccess.com",
				envGraphQLAccessAudience:   testAccessAudience,
				envGraphQLAccessPrincipals: "cody@flexinfer.ai=integration:preview;cody@flexinfer.ai=integration:preview",
			},
			want: "twice",
		},
		"non-canonical value": {
			env: map[string]string{
				envGraphQLAccessTeamDomain: "https://flexinfer.cloudflareaccess.com ",
				envGraphQLAccessAudience:   testAccessAudience,
				envGraphQLAccessPrincipals: "cody@flexinfer.ai=integration:preview",
			},
			want: "must be canonical",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			setStaticGraphQLEnv(t)
			for key, value := range tc.env {
				t.Setenv(key, value)
			}
			_, _, _, err := loadGraphQLAuthenticationFromEnv(context.Background(), "tenant-a")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want it to mention %q", err, tc.want)
			}
		})
	}
}
