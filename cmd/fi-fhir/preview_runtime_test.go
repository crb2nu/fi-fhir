package main

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/requestsecurity/oidctest"
	integrationingress "gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/ingress"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/mllp"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/processor"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

const runtimeProfileJSON = `{"hl7v2":{"default_version":"2.5.1","timezone":"UTC","event_classifications":[{"message_type":"ADT^A01","event_type":"patient_admit","priority":1}]},"identifiers":{"assigning_authorities":[{"code":"HOSP","system":"urn:oid:1.2.3"}],"normalization":{"ssn_strip_dashes":true,"phone_normalize":false}}}`

const runtimeWorkflowYAML = `dsl_version: "1"
name: adt-preview
version: "1"
routes:
  - name: admit
    filter:
      event_type: patient_admit
    actions:
      - id: send-fhir
        type: fhir
        destination: fhir-primary
`

func TestLoadPreviewRuntimeFromEnv(t *testing.T) {
	registryPath := writeRuntimeRegistry(t, "tenant-a")
	t.Setenv("FI_FHIR_DEPLOYMENT_TENANT_ID", "tenant-a")
	t.Setenv("FI_FHIR_GRAPHQL_BEARER_TOKEN", "correct-horse-battery-staple")
	t.Setenv("FI_FHIR_GRAPHQL_BEARER_TOKEN_FILE", "")
	t.Setenv("FI_FHIR_GRAPHQL_PRINCIPAL_ID", "engineer-1")
	t.Setenv("FI_FHIR_GRAPHQL_ROLES", "integration:preview,author")
	t.Setenv("FI_FHIR_GRAPHQL_ALLOWED_ORIGINS", "https://ide.example.test,http://localhost:5173")
	t.Setenv("FI_FHIR_GRAPHQL_TRUSTED_CIDRS", "192.168.50.0/24")
	t.Setenv("FI_FHIR_INTEGRATION_REGISTRY_PATH", registryPath)

	runtime, err := loadPreviewRuntimeFromEnv()
	if err != nil {
		t.Fatalf("loadPreviewRuntimeFromEnv: %v", err)
	}
	if runtime.previewService == nil || runtime.authenticator == nil || runtime.trustedNetwork == nil {
		t.Fatalf("incomplete preview runtime: %#v", runtime)
	}
	if got := strings.Join(runtime.allowedOrigins, ","); got != "https://ide.example.test,http://localhost:5173" {
		t.Fatalf("allowed origins = %q", got)
	}
	security, err := runtime.authenticator.Authenticate(context.Background(), "Bearer correct-horse-battery-staple")
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if security.TenantID != "tenant-a" || security.Principal.ID != "engineer-1" {
		t.Fatalf("security = %#v", security)
	}
}

func TestCheckedInPreviewRegistryLoads(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "golden", "integration", "adt-http", "preview-registry.json")
	t.Setenv("FI_FHIR_DEPLOYMENT_TENANT_ID", "tenant-a")
	t.Setenv("FI_FHIR_GRAPHQL_BEARER_TOKEN", "correct-horse-battery-staple")
	t.Setenv("FI_FHIR_GRAPHQL_BEARER_TOKEN_FILE", "")
	t.Setenv("FI_FHIR_GRAPHQL_PRINCIPAL_ID", "engineer-1")
	t.Setenv("FI_FHIR_GRAPHQL_ROLES", "integration:preview")
	t.Setenv("FI_FHIR_GRAPHQL_ALLOWED_ORIGINS", "http://localhost:5173")
	t.Setenv("FI_FHIR_INTEGRATION_REGISTRY_PATH", path)

	runtime, err := loadPreviewRuntimeFromEnv()
	if err != nil {
		t.Fatalf("load checked-in preview registry: %v", err)
	}
	if runtime.previewService == nil {
		t.Fatal("checked-in preview registry did not configure the preview service")
	}
}

func TestLoadPreviewRuntimeFromEnvFailsClosed(t *testing.T) {
	validRegistry := writeRuntimeRegistry(t, "tenant-a")
	valid := map[string]string{
		"FI_FHIR_DEPLOYMENT_TENANT_ID":      "tenant-a",
		"FI_FHIR_GRAPHQL_BEARER_TOKEN":      "correct-horse-battery-staple",
		"FI_FHIR_GRAPHQL_BEARER_TOKEN_FILE": "",
		"FI_FHIR_GRAPHQL_PRINCIPAL_ID":      "engineer-1",
		"FI_FHIR_GRAPHQL_ROLES":             "integration:preview",
		"FI_FHIR_GRAPHQL_ALLOWED_ORIGINS":   "https://ide.example.test",
		"FI_FHIR_INTEGRATION_REGISTRY_PATH": validRegistry,
	}

	for missing := range valid {
		if missing == "FI_FHIR_GRAPHQL_BEARER_TOKEN_FILE" {
			continue
		}
		t.Run("missing "+missing, func(t *testing.T) {
			for key, value := range valid {
				t.Setenv(key, value)
			}
			t.Setenv(missing, "")
			if _, err := loadPreviewRuntimeFromEnv(); err == nil {
				t.Fatal("incomplete preview runtime was accepted")
			}
		})
	}

	t.Run("tenant mismatch", func(t *testing.T) {
		for key, value := range valid {
			t.Setenv(key, value)
		}
		t.Setenv("FI_FHIR_DEPLOYMENT_TENANT_ID", "tenant-b")
		if _, err := loadPreviewRuntimeFromEnv(); err == nil {
			t.Fatal("cross-tenant registry was accepted")
		}
	})

	t.Run("ambiguous token sources", func(t *testing.T) {
		for key, value := range valid {
			t.Setenv(key, value)
		}
		tokenFile := filepath.Join(t.TempDir(), "token")
		if err := os.WriteFile(tokenFile, []byte("another-correct-bearer-token"), 0o600); err != nil {
			t.Fatalf("write token: %v", err)
		}
		t.Setenv("FI_FHIR_GRAPHQL_BEARER_TOKEN_FILE", tokenFile)
		if _, err := loadPreviewRuntimeFromEnv(); err == nil {
			t.Fatal("multiple bearer token sources were accepted")
		}
	})

	t.Run("invalid trusted network", func(t *testing.T) {
		for key, value := range valid {
			t.Setenv(key, value)
		}
		t.Setenv("FI_FHIR_GRAPHQL_TRUSTED_CIDRS", "192.168.50.0/24,not-a-network")
		if _, err := loadPreviewRuntimeFromEnv(); err == nil || !strings.Contains(err.Error(), "trusted network") {
			t.Fatalf("invalid trusted network error = %v", err)
		}
	})
}

func TestLoadGraphQLAuthenticationStaticModeCompatibility(t *testing.T) {
	for _, mode := range []string{"", graphqlAuthModeStatic} {
		t.Run("mode="+mode, func(t *testing.T) {
			clearGraphQLAuthenticationEnv(t)
			t.Setenv("FI_FHIR_GRAPHQL_AUTH_MODE", mode)
			t.Setenv("FI_FHIR_GRAPHQL_BEARER_TOKEN", "correct-horse-battery-staple")
			t.Setenv("FI_FHIR_GRAPHQL_PRINCIPAL_ID", "engineer-1")
			t.Setenv("FI_FHIR_GRAPHQL_ROLES", "integration:preview,author")
			t.Setenv("FI_FHIR_GRAPHQL_TRUSTED_CIDRS", "192.168.50.0/24")

			authenticator, trustedNetwork, err := loadGraphQLAuthenticationFromEnv(context.Background(), "tenant-a")
			if err != nil {
				t.Fatalf("load GraphQL authentication: %v", err)
			}
			if trustedNetwork == nil {
				t.Fatal("static mode did not configure the trusted network authenticator")
			}
			security, err := authenticator.Authenticate(context.Background(), "Bearer correct-horse-battery-staple")
			if err != nil {
				t.Fatalf("Authenticate: %v", err)
			}
			if security.TenantID != "tenant-a" || security.Principal.ID != "engineer-1" {
				t.Fatalf("security = %#v", security)
			}
		})
	}
}

func TestLoadGraphQLOIDCSettingsFromEnv(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		clearGraphQLAuthenticationEnv(t)
		t.Setenv("FI_FHIR_GRAPHQL_AUTH_MODE", graphqlAuthModeOIDC)
		t.Setenv("FI_FHIR_GRAPHQL_OIDC_ISSUER_URL", "https://identity.example.test")
		t.Setenv("FI_FHIR_GRAPHQL_OIDC_AUDIENCE", "fi-fhir")

		settings, err := loadGraphQLOIDCSettingsFromEnv()
		if err != nil {
			t.Fatalf("load OIDC settings: %v", err)
		}
		if settings.issuerURL != "https://identity.example.test" || settings.audience != "fi-fhir" {
			t.Fatalf("issuer/audience = %q/%q", settings.issuerURL, settings.audience)
		}
		if settings.tenantClaim != "tenant_id" || settings.rolesClaim != "roles" {
			t.Fatalf("default claims = %q/%q", settings.tenantClaim, settings.rolesClaim)
		}
		if got := strings.Join(settings.signingAlgorithms, ","); got != "RS256" {
			t.Fatalf("default signing algorithms = %q", got)
		}
	})

	t.Run("custom claims and algorithms", func(t *testing.T) {
		clearGraphQLAuthenticationEnv(t)
		t.Setenv("FI_FHIR_GRAPHQL_AUTH_MODE", graphqlAuthModeOIDC)
		t.Setenv("FI_FHIR_GRAPHQL_OIDC_ISSUER_URL", "https://identity.example.test")
		t.Setenv("FI_FHIR_GRAPHQL_OIDC_AUDIENCE", "fi-fhir")
		t.Setenv("FI_FHIR_GRAPHQL_OIDC_TENANT_CLAIM", "organization_id")
		t.Setenv("FI_FHIR_GRAPHQL_OIDC_ROLES_CLAIM", "permissions")
		t.Setenv("FI_FHIR_GRAPHQL_OIDC_SIGNING_ALGS", "RS256,ES256")

		settings, err := loadGraphQLOIDCSettingsFromEnv()
		if err != nil {
			t.Fatalf("load OIDC settings: %v", err)
		}
		if settings.tenantClaim != "organization_id" || settings.rolesClaim != "permissions" {
			t.Fatalf("custom claims = %q/%q", settings.tenantClaim, settings.rolesClaim)
		}
		if got := strings.Join(settings.signingAlgorithms, ","); got != "RS256,ES256" {
			t.Fatalf("custom signing algorithms = %q", got)
		}
	})
}

func TestLoadGraphQLAuthenticationOIDCMode(t *testing.T) {
	issuer, err := oidctest.New()
	if err != nil {
		t.Fatalf("create OIDC issuer: %v", err)
	}
	t.Cleanup(issuer.Close)
	clearGraphQLAuthenticationEnv(t)
	t.Setenv("FI_FHIR_GRAPHQL_AUTH_MODE", graphqlAuthModeOIDC)
	t.Setenv("FI_FHIR_GRAPHQL_OIDC_ISSUER_URL", issuer.IssuerURL())
	t.Setenv("FI_FHIR_GRAPHQL_OIDC_AUDIENCE", "fi-fhir-graphql")

	authenticator, trustedNetwork, err := loadGraphQLAuthenticationFromEnv(issuer.Context(), "tenant-a")
	if err != nil {
		t.Fatalf("load OIDC authentication: %v", err)
	}
	if trustedNetwork != nil {
		t.Fatal("OIDC mode configured a trusted-network compatibility bypass")
	}
	token, err := issuer.Sign(issuer.Claims(), "RS256")
	if err != nil {
		t.Fatalf("sign OIDC token: %v", err)
	}
	security, err := authenticator.Authenticate(issuer.Context(), "Bearer "+token)
	if err != nil {
		t.Fatalf("authenticate OIDC token: %v", err)
	}
	if security.TenantID != "tenant-a" || security.Principal.ID != "clinician-1" || security.Principal.AuthMethod != graphqlAuthModeOIDC {
		t.Fatalf("OIDC security context = %#v", security)
	}
}

func TestLoadGraphQLAuthenticationModesFailClosed(t *testing.T) {
	t.Run("unknown mode", func(t *testing.T) {
		clearGraphQLAuthenticationEnv(t)
		t.Setenv("FI_FHIR_GRAPHQL_AUTH_MODE", "legacy")
		_, _, err := loadGraphQLAuthenticationFromEnv(context.Background(), "tenant-a")
		if err == nil || !strings.Contains(err.Error(), "must be") {
			t.Fatalf("unknown mode error = %v", err)
		}
	})

	t.Run("noncanonical mode", func(t *testing.T) {
		clearGraphQLAuthenticationEnv(t)
		t.Setenv("FI_FHIR_GRAPHQL_AUTH_MODE", " oidc")
		_, _, err := loadGraphQLAuthenticationFromEnv(context.Background(), "tenant-a")
		if err == nil || !strings.Contains(err.Error(), "canonical") {
			t.Fatalf("noncanonical mode error = %v", err)
		}
	})

	for _, name := range []string{
		"FI_FHIR_GRAPHQL_OIDC_ISSUER_URL",
		"FI_FHIR_GRAPHQL_OIDC_AUDIENCE",
		"FI_FHIR_GRAPHQL_OIDC_TENANT_CLAIM",
		"FI_FHIR_GRAPHQL_OIDC_ROLES_CLAIM",
		"FI_FHIR_GRAPHQL_OIDC_SIGNING_ALGS",
	} {
		t.Run("static rejects "+name, func(t *testing.T) {
			clearGraphQLAuthenticationEnv(t)
			t.Setenv("FI_FHIR_GRAPHQL_AUTH_MODE", graphqlAuthModeStatic)
			t.Setenv("FI_FHIR_GRAPHQL_BEARER_TOKEN", "correct-horse-battery-staple")
			t.Setenv("FI_FHIR_GRAPHQL_PRINCIPAL_ID", "engineer-1")
			t.Setenv("FI_FHIR_GRAPHQL_ROLES", "integration:preview")
			t.Setenv(name, "configured")
			_, _, err := loadGraphQLAuthenticationFromEnv(context.Background(), "tenant-a")
			if err == nil || !strings.Contains(err.Error(), name) {
				t.Fatalf("static conflict error = %v", err)
			}
		})
	}

	for _, name := range []string{
		"FI_FHIR_GRAPHQL_BEARER_TOKEN",
		"FI_FHIR_GRAPHQL_BEARER_TOKEN_FILE",
		"FI_FHIR_GRAPHQL_PRINCIPAL_ID",
		"FI_FHIR_GRAPHQL_ROLES",
		"FI_FHIR_GRAPHQL_TRUSTED_CIDRS",
	} {
		t.Run("oidc rejects "+name, func(t *testing.T) {
			clearGraphQLAuthenticationEnv(t)
			t.Setenv("FI_FHIR_GRAPHQL_AUTH_MODE", graphqlAuthModeOIDC)
			t.Setenv("FI_FHIR_GRAPHQL_OIDC_ISSUER_URL", "https://identity.example.test")
			t.Setenv("FI_FHIR_GRAPHQL_OIDC_AUDIENCE", "fi-fhir")
			t.Setenv(name, "configured")
			_, _, err := loadGraphQLAuthenticationFromEnv(context.Background(), "tenant-a")
			if err == nil || !strings.Contains(err.Error(), name) {
				t.Fatalf("OIDC conflict error = %v", err)
			}
		})
	}

	for _, missing := range []string{
		"FI_FHIR_GRAPHQL_OIDC_ISSUER_URL",
		"FI_FHIR_GRAPHQL_OIDC_AUDIENCE",
	} {
		t.Run("oidc requires "+missing, func(t *testing.T) {
			clearGraphQLAuthenticationEnv(t)
			t.Setenv("FI_FHIR_GRAPHQL_AUTH_MODE", graphqlAuthModeOIDC)
			t.Setenv("FI_FHIR_GRAPHQL_OIDC_ISSUER_URL", "https://identity.example.test")
			t.Setenv("FI_FHIR_GRAPHQL_OIDC_AUDIENCE", "fi-fhir")
			t.Setenv(missing, "")
			_, _, err := loadGraphQLAuthenticationFromEnv(context.Background(), "tenant-a")
			if err == nil || !strings.Contains(err.Error(), missing) {
				t.Fatalf("missing OIDC setting error = %v", err)
			}
		})
	}
}

func TestLoadHTTPIngressAuthenticatorFromEnv(t *testing.T) {
	clearHTTPIngressAuthenticationEnv(t)
	t.Setenv("FI_FHIR_HTTP_INGRESS_AUTH_MODE", "bearer")
	t.Setenv("FI_FHIR_HTTP_INGRESS_PRINCIPAL_ID", "adt-service")
	t.Setenv("FI_FHIR_HTTP_INGRESS_INTEGRATION_ID", "adt-east")
	t.Setenv("FI_FHIR_HTTP_INGRESS_SECRET", "correct-http-ingress-token-001")
	t.Setenv("FI_FHIR_HTTP_INGRESS_SECRET_FILE", "")
	t.Setenv("FI_FHIR_HTTP_INGRESS_MAX_BODY_BYTES", "4096")

	authenticator, maxBodyBytes, err := loadHTTPIngressAuthenticatorFromEnv(context.Background(), "tenant-a")
	if err != nil {
		t.Fatalf("loadHTTPIngressAuthenticatorFromEnv: %v", err)
	}
	request := httptest.NewRequest("POST", integrationingress.Path, nil)
	request.Header.Set("Authorization", "Bearer correct-http-ingress-token-001")
	security, err := authenticator.AuthenticateRequest(context.Background(), request, nil)
	if err != nil {
		t.Fatalf("authenticate configured ingress: %v", err)
	}
	if security.TenantID != "tenant-a" || security.Principal.ID != "adt-service" || authenticator.IntegrationID() != "adt-east" || maxBodyBytes != 4096 {
		t.Fatalf("ingress configuration = security %#v integration %q max %d", security, authenticator.IntegrationID(), maxBodyBytes)
	}

	secretFile := filepath.Join(t.TempDir(), "ingress-token")
	if err := os.WriteFile(secretFile, []byte("file-backed-http-ingress-token-001\n"), 0o600); err != nil {
		t.Fatalf("write ingress secret: %v", err)
	}
	t.Setenv("FI_FHIR_HTTP_INGRESS_SECRET", "")
	t.Setenv("FI_FHIR_HTTP_INGRESS_SECRET_FILE", secretFile)
	if _, _, err := loadHTTPIngressAuthenticatorFromEnv(context.Background(), "tenant-a"); err != nil {
		t.Fatalf("file-backed ingress secret: %v", err)
	}
}

func TestLoadHTTPIngressOAuthAuthenticatorFromEnv(t *testing.T) {
	issuer, err := oidctest.New()
	if err != nil {
		t.Fatalf("create OIDC issuer: %v", err)
	}
	t.Cleanup(issuer.Close)
	clearHTTPIngressAuthenticationEnv(t)
	t.Setenv("FI_FHIR_HTTP_INGRESS_AUTH_MODE", "oauth2")
	t.Setenv("FI_FHIR_HTTP_INGRESS_INTEGRATION_ID", "adt-east")
	t.Setenv("FI_FHIR_HTTP_INGRESS_OAUTH_ISSUER_URL", issuer.IssuerURL())
	t.Setenv("FI_FHIR_HTTP_INGRESS_OAUTH_AUDIENCE", "fi-fhir-http-ingress")
	t.Setenv("FI_FHIR_HTTP_INGRESS_OAUTH_ALLOWED_CLIENT_IDS", "client-a,client-b")

	authenticator, maxBodyBytes, err := loadHTTPIngressAuthenticatorFromEnv(issuer.Context(), "tenant-a")
	if err != nil {
		t.Fatalf("load OAuth ingress authenticator: %v", err)
	}
	claims := issuer.Claims()
	claims["sub"] = "client-a"
	claims["client_id"] = "client-a"
	claims["aud"] = "fi-fhir-http-ingress"
	claims["roles"] = []string{integrationingress.SubmitRole}
	token, err := issuer.Sign(claims, "RS256")
	if err != nil {
		t.Fatalf("sign service token: %v", err)
	}
	request := httptest.NewRequest("POST", integrationingress.Path, nil)
	request.Header.Set("Authorization", "Bearer "+token)
	security, err := authenticator.AuthenticateRequest(issuer.Context(), request, nil)
	if err != nil {
		t.Fatalf("authenticate service token: %v", err)
	}
	if authenticator.IntegrationID() != "adt-east" || maxBodyBytes != integrationingress.DefaultMaxBodyBytes || security.TenantID != "tenant-a" || security.Principal.ID != "client-a" || security.Principal.AuthMethod != "oauth2-client-credentials" {
		t.Fatalf("OAuth ingress configuration = integration %q max %d security %#v", authenticator.IntegrationID(), maxBodyBytes, security)
	}
}

func TestLoadHTTPIngressAuthenticationModesFailClosed(t *testing.T) {
	t.Run("unknown mode", func(t *testing.T) {
		clearHTTPIngressAuthenticationEnv(t)
		t.Setenv("FI_FHIR_HTTP_INGRESS_AUTH_MODE", "legacy")
		t.Setenv("FI_FHIR_HTTP_INGRESS_INTEGRATION_ID", "adt-east")
		_, _, err := loadHTTPIngressAuthenticatorFromEnv(context.Background(), "tenant-a")
		if err == nil || !strings.Contains(err.Error(), "must be") {
			t.Fatalf("unknown mode error = %v", err)
		}
	})

	for _, name := range []string{
		"FI_FHIR_HTTP_INGRESS_OAUTH_ISSUER_URL",
		"FI_FHIR_HTTP_INGRESS_OAUTH_AUDIENCE",
		"FI_FHIR_HTTP_INGRESS_OAUTH_TENANT_CLAIM",
		"FI_FHIR_HTTP_INGRESS_OAUTH_ROLES_CLAIM",
		"FI_FHIR_HTTP_INGRESS_OAUTH_CLIENT_ID_CLAIM",
		"FI_FHIR_HTTP_INGRESS_OAUTH_SIGNING_ALGS",
		"FI_FHIR_HTTP_INGRESS_OAUTH_ALLOWED_CLIENT_IDS",
	} {
		t.Run("static rejects "+name, func(t *testing.T) {
			clearHTTPIngressAuthenticationEnv(t)
			t.Setenv("FI_FHIR_HTTP_INGRESS_AUTH_MODE", "bearer")
			t.Setenv("FI_FHIR_HTTP_INGRESS_INTEGRATION_ID", "adt-east")
			t.Setenv(name, "configured")
			_, _, err := loadHTTPIngressAuthenticatorFromEnv(context.Background(), "tenant-a")
			if err == nil || !strings.Contains(err.Error(), name) {
				t.Fatalf("static conflict error = %v", err)
			}
		})
	}

	for _, name := range []string{
		"FI_FHIR_HTTP_INGRESS_SECRET",
		"FI_FHIR_HTTP_INGRESS_SECRET_FILE",
		"FI_FHIR_HTTP_INGRESS_PRINCIPAL_ID",
	} {
		t.Run("oauth2 rejects "+name, func(t *testing.T) {
			clearHTTPIngressAuthenticationEnv(t)
			t.Setenv("FI_FHIR_HTTP_INGRESS_AUTH_MODE", "oauth2")
			t.Setenv("FI_FHIR_HTTP_INGRESS_INTEGRATION_ID", "adt-east")
			t.Setenv(name, "configured")
			_, _, err := loadHTTPIngressAuthenticatorFromEnv(context.Background(), "tenant-a")
			if err == nil || !strings.Contains(err.Error(), name) {
				t.Fatalf("OAuth conflict error = %v", err)
			}
		})
	}

	for _, missing := range []string{
		"FI_FHIR_HTTP_INGRESS_OAUTH_ISSUER_URL",
		"FI_FHIR_HTTP_INGRESS_OAUTH_AUDIENCE",
		"FI_FHIR_HTTP_INGRESS_OAUTH_ALLOWED_CLIENT_IDS",
	} {
		t.Run("oauth2 requires "+missing, func(t *testing.T) {
			clearHTTPIngressAuthenticationEnv(t)
			t.Setenv("FI_FHIR_HTTP_INGRESS_AUTH_MODE", "oauth2")
			t.Setenv("FI_FHIR_HTTP_INGRESS_INTEGRATION_ID", "adt-east")
			t.Setenv("FI_FHIR_HTTP_INGRESS_OAUTH_ISSUER_URL", "https://identity.example.test")
			t.Setenv("FI_FHIR_HTTP_INGRESS_OAUTH_AUDIENCE", "fi-fhir-http-ingress")
			t.Setenv("FI_FHIR_HTTP_INGRESS_OAUTH_ALLOWED_CLIENT_IDS", "client-a")
			t.Setenv(missing, "")
			_, _, err := loadHTTPIngressAuthenticatorFromEnv(context.Background(), "tenant-a")
			if err == nil || !strings.Contains(err.Error(), missing) {
				t.Fatalf("missing OAuth setting error = %v", err)
			}
		})
	}
}

func TestLoadServeIntegrationRuntimeFailsBeforeDatabaseMutation(t *testing.T) {
	configurePreviewRuntimeForTest(t)
	t.Setenv("FI_FHIR_HTTP_INGRESS_AUTH_MODE", "bearer")
	t.Setenv("FI_FHIR_HTTP_INGRESS_PRINCIPAL_ID", "adt-service")
	t.Setenv("FI_FHIR_HTTP_INGRESS_INTEGRATION_ID", "adt-east")
	t.Setenv("FI_FHIR_HTTP_INGRESS_SECRET", "")
	t.Setenv("FI_FHIR_HTTP_INGRESS_SECRET_FILE", "")
	for _, name := range []string{
		"FI_FHIR_DATABASE_HOST",
		"FI_FHIR_DATABASE_NAME",
		"FI_FHIR_DATABASE_USERNAME",
		"FI_FHIR_DATABASE_USER",
	} {
		t.Setenv(name, "")
	}

	_, err := loadServeIntegrationRuntimeFromEnv(context.Background())
	if err == nil || !strings.Contains(err.Error(), "FI_FHIR_HTTP_INGRESS_SECRET") {
		t.Fatalf("missing ingress secret error = %v", err)
	}

	t.Setenv("FI_FHIR_HTTP_INGRESS_SECRET", "correct-http-ingress-token-001")
	_, err = loadServeIntegrationRuntimeFromEnv(context.Background())
	if err == nil || !strings.Contains(err.Error(), "FI_FHIR_DATABASE_HOST") {
		t.Fatalf("missing PostgreSQL configuration error = %v", err)
	}
}

func TestLoadMLLPRuntimeFromEnv(t *testing.T) {
	source := writeMLLPSourceRevision(t, mllp.TLSPolicy{Mode: mllp.TLSModeDisabled})
	t.Setenv("FI_FHIR_MLLP_DEFINITION_ID", "integration-mllp")
	t.Setenv("FI_FHIR_MLLP_PRINCIPAL_ID", "mllp-listener")
	loaded, definitionID, principalID, material, err := loadMLLPRuntimeFromEnv(source)
	if err != nil {
		t.Fatalf("loadMLLPRuntimeFromEnv: %v", err)
	}
	if loaded.SourceID != "adt-east" || definitionID != "integration-mllp" || principalID != "mllp-listener" {
		t.Fatalf("unexpected MLLP runtime: %#v %q %q", loaded, definitionID, principalID)
	}
	if len(material.CertificatePEM) != 0 || len(material.PrivateKeyPEM) != 0 || len(material.ClientCAPEM) != 0 {
		t.Fatalf("plaintext mode loaded TLS material: %#v", material)
	}
}

func TestLoadMLLPRuntimeFromEnvFailsClosedBeforeDatabase(t *testing.T) {
	source := writeMLLPSourceRevision(t, mllp.TLSPolicy{Mode: mllp.TLSModeDisabled})
	t.Setenv("FI_FHIR_MLLP_DEFINITION_ID", "")
	t.Setenv("FI_FHIR_MLLP_PRINCIPAL_ID", "mllp-listener")
	if _, _, _, _, err := loadMLLPRuntimeFromEnv(source); err == nil || !strings.Contains(err.Error(), "FI_FHIR_MLLP_DEFINITION_ID") {
		t.Fatalf("missing definition error = %v", err)
	}

	mutualSource := writeMLLPSourceRevision(t, mllp.TLSPolicy{
		Mode: mllp.TLSModeMutual, ServerCertificateBinding: "mllp-cert",
		ServerPrivateKeyBinding: "mllp-key", ClientCABinding: "mllp-client-ca",
	})
	t.Setenv("FI_FHIR_MLLP_DEFINITION_ID", "integration-mllp")
	t.Setenv("FI_FHIR_MLLP_TLS_CERT_FILE", "")
	if _, _, _, _, err := loadMLLPRuntimeFromEnv(mutualSource); err == nil || !strings.Contains(err.Error(), "FI_FHIR_MLLP_TLS_CERT_FILE") {
		t.Fatalf("missing TLS file error = %v", err)
	}
}

func TestLoadMLLPRuntimeRequiresConfiguredClientIdentity(t *testing.T) {
	t.Setenv("FI_FHIR_MLLP_DEFINITION_ID", "integration-mllp")
	t.Setenv("FI_FHIR_MLLP_PRINCIPAL_ID", "mllp-listener")
	t.Setenv("FI_FHIR_MLLP_REQUIRE_CLIENT_IDENTITY", "true")

	// Compatibility mode must not start when the deployment demands mapping.
	compatibility := writeMLLPSourceRevision(t, mllp.TLSPolicy{Mode: mllp.TLSModeDisabled})
	if _, _, _, _, err := loadMLLPRuntimeFromEnv(compatibility); err == nil ||
		!strings.Contains(err.Error(), "FI_FHIR_MLLP_REQUIRE_CLIENT_IDENTITY") {
		t.Fatalf("compatibility source started under required identity mapping: %v", err)
	}

	mutual := mllp.TLSPolicy{
		Mode: mllp.TLSModeMutual, ServerCertificateBinding: "mllp-cert",
		ServerPrivateKeyBinding: "mllp-key", ClientCABinding: "mllp-client-ca",
	}
	mapped := writeMLLPSourceRevisionWithIdentities(t, mutual, []mllp.ClientIdentity{{
		Subject: "svc-sender-a", URISAN: "spiffe://hospital-a/mllp/sender-a",
		Grants: []string{mllp.SubmitRole},
	}})
	certificate := writeRuntimeSecretFile(t, "tls.crt", "certificate")
	key := writeRuntimeSecretFile(t, "tls.key", "private key")
	clientCA := writeRuntimeSecretFile(t, "client-ca.crt", "client ca")
	t.Setenv("FI_FHIR_MLLP_TLS_CERT_FILE", certificate)
	t.Setenv("FI_FHIR_MLLP_TLS_KEY_FILE", key)
	t.Setenv("FI_FHIR_MLLP_TLS_CLIENT_CA_FILE", clientCA)
	loaded, _, _, _, err := loadMLLPRuntimeFromEnv(mapped)
	if err != nil {
		t.Fatalf("mapped source: %v", err)
	}
	if !loaded.Clients.IdentityMappingEnabled() || len(loaded.Clients.Identities) != 1 ||
		loaded.Clients.Identities[0].Subject != "svc-sender-a" {
		t.Fatalf("identity map did not survive decoding: %#v", loaded.Clients)
	}

	// An invalid boolean fails closed rather than defaulting to compatibility.
	t.Setenv("FI_FHIR_MLLP_REQUIRE_CLIENT_IDENTITY", "yes-please")
	if _, _, _, _, err := loadMLLPRuntimeFromEnv(mapped); err == nil ||
		!strings.Contains(err.Error(), "FI_FHIR_MLLP_REQUIRE_CLIENT_IDENTITY") {
		t.Fatalf("invalid boolean error = %v", err)
	}
}

func writeRuntimeSecretFile(t *testing.T, name, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadSessionRetentionProtector(t *testing.T) {
	t.Setenv("FI_FHIR_INTEGRATION_SESSION_RETENTION_KEY_FILE", "")
	protector, err := loadSessionRetentionProtector()
	if err != nil || protector != nil {
		t.Fatalf("disabled protector = %#v, %v", protector, err)
	}

	keyPath := filepath.Join(t.TempDir(), "session-retention.key")
	if err := os.WriteFile(keyPath, []byte("0123456789abcdef0123456789abcdef"), 0o600); err != nil {
		t.Fatalf("write retention key: %v", err)
	}
	t.Setenv("FI_FHIR_INTEGRATION_SESSION_RETENTION_KEY_FILE", keyPath)
	protector, err = loadSessionRetentionProtector()
	if err != nil || protector == nil {
		t.Fatalf("configured protector = %#v, %v", protector, err)
	}

	if err := os.WriteFile(keyPath, []byte("too-short"), 0o600); err != nil {
		t.Fatalf("rewrite retention key: %v", err)
	}
	if _, err := loadSessionRetentionProtector(); err == nil || !strings.Contains(err.Error(), "exactly 32 bytes") {
		t.Fatalf("short retention key error = %v", err)
	}
}

func configurePreviewRuntimeForTest(t *testing.T) {
	t.Helper()
	t.Setenv("FI_FHIR_DEPLOYMENT_TENANT_ID", "tenant-a")
	t.Setenv("FI_FHIR_GRAPHQL_BEARER_TOKEN", "correct-horse-battery-staple")
	t.Setenv("FI_FHIR_GRAPHQL_BEARER_TOKEN_FILE", "")
	t.Setenv("FI_FHIR_GRAPHQL_PRINCIPAL_ID", "engineer-1")
	t.Setenv("FI_FHIR_GRAPHQL_ROLES", "integration:preview")
	t.Setenv("FI_FHIR_GRAPHQL_ALLOWED_ORIGINS", "http://localhost:5173")
	t.Setenv("FI_FHIR_INTEGRATION_REGISTRY_PATH", writeRuntimeRegistry(t, "tenant-a"))
}

func clearGraphQLAuthenticationEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		"FI_FHIR_GRAPHQL_AUTH_MODE",
		"FI_FHIR_GRAPHQL_BEARER_TOKEN",
		"FI_FHIR_GRAPHQL_BEARER_TOKEN_FILE",
		"FI_FHIR_GRAPHQL_PRINCIPAL_ID",
		"FI_FHIR_GRAPHQL_ROLES",
		"FI_FHIR_GRAPHQL_TRUSTED_CIDRS",
		"FI_FHIR_GRAPHQL_OIDC_ISSUER_URL",
		"FI_FHIR_GRAPHQL_OIDC_AUDIENCE",
		"FI_FHIR_GRAPHQL_OIDC_TENANT_CLAIM",
		"FI_FHIR_GRAPHQL_OIDC_ROLES_CLAIM",
		"FI_FHIR_GRAPHQL_OIDC_SIGNING_ALGS",
	} {
		t.Setenv(name, "")
	}
}

func clearHTTPIngressAuthenticationEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		"FI_FHIR_HTTP_INGRESS_AUTH_MODE",
		"FI_FHIR_HTTP_INGRESS_PRINCIPAL_ID",
		"FI_FHIR_HTTP_INGRESS_INTEGRATION_ID",
		"FI_FHIR_HTTP_INGRESS_SECRET",
		"FI_FHIR_HTTP_INGRESS_SECRET_FILE",
		"FI_FHIR_HTTP_INGRESS_MAX_BODY_BYTES",
		"FI_FHIR_HTTP_INGRESS_OAUTH_ISSUER_URL",
		"FI_FHIR_HTTP_INGRESS_OAUTH_AUDIENCE",
		"FI_FHIR_HTTP_INGRESS_OAUTH_TENANT_CLAIM",
		"FI_FHIR_HTTP_INGRESS_OAUTH_ROLES_CLAIM",
		"FI_FHIR_HTTP_INGRESS_OAUTH_CLIENT_ID_CLAIM",
		"FI_FHIR_HTTP_INGRESS_OAUTH_SIGNING_ALGS",
		"FI_FHIR_HTTP_INGRESS_OAUTH_ALLOWED_CLIENT_IDS",
	} {
		t.Setenv(name, "")
	}
}

func writeRuntimeRegistry(t *testing.T, tenantID string) string {
	t.Helper()
	profileRef, err := processor.NewProfileRevisionReference("profile-adt", 1, []byte(runtimeProfileJSON))
	if err != nil {
		t.Fatalf("NewProfileRevisionReference: %v", err)
	}
	workflowRef, err := processor.NewWorkflowRevisionReference("workflow-adt", "workflow-version-1", []byte(runtimeWorkflowYAML))
	if err != nil {
		t.Fatalf("NewWorkflowRevisionReference: %v", err)
	}
	digest := func(character string) string { return "sha256:" + strings.Repeat(character, 64) }
	revision, err := integration.NewIntegrationDefinitionRevision(integration.IntegrationDefinitionRevisionInput{
		DefinitionID: "integration-adt",
		RevisionID:   "definition-revision-1",
		TenantID:     tenantID,
		Source: integration.SourceRevisionRef{
			ArtifactRevisionRef: integration.ArtifactRevisionRef{ArtifactID: "source-adt", RevisionID: "source-1", Digest: digest("a")},
			SourceID:            "adt-east",
		},
		Format:   events.FormatHL7v2,
		Profile:  profileRef,
		Workflow: workflowRef,
		Destinations: []integration.DestinationRevisionRef{{
			ArtifactRevisionRef: integration.ArtifactRevisionRef{ArtifactID: "fhir-primary", RevisionID: "destination-1", Digest: digest("d")},
			Class:               integration.DestinationClassProduction,
		}},
		Policy: integration.IntegrationPolicy{Classification: integration.DataClassificationPHI, RawRetention: integration.RawRetentionPolicy{Mode: integration.RawRetentionModeEphemeral}},
		Created: integration.AuditEnvelope{
			TenantID: tenantID, Principal: integration.Principal{ID: "publisher", Kind: integration.PrincipalKindHuman, AuthMethod: "oidc", Roles: []string{"publisher"}}, Reason: "publish", OccurredAt: time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC),
		},
	})
	if err != nil {
		t.Fatalf("NewIntegrationDefinitionRevision: %v", err)
	}
	document, err := json.MarshalIndent(map[string]any{
		"tenant_id": tenantID,
		"integrations": []map[string]any{{
			"integration_id": "adt-east",
			"definition":     revision,
			"profile":        json.RawMessage(runtimeProfileJSON),
			"workflow":       runtimeWorkflowYAML,
		}},
	}, "", "  ")
	if err != nil {
		t.Fatalf("marshal registry: %v", err)
	}
	path := filepath.Join(t.TempDir(), "registry.json")
	if err := os.WriteFile(path, document, 0o600); err != nil {
		t.Fatalf("write registry: %v", err)
	}
	return path
}

func writeMLLPSourceRevision(t *testing.T, tlsPolicy mllp.TLSPolicy) string {
	t.Helper()
	return writeMLLPSourceRevisionWithIdentities(t, tlsPolicy, nil)
}

func writeMLLPSourceRevisionWithIdentities(t *testing.T, tlsPolicy mllp.TLSPolicy, identities []mllp.ClientIdentity) string {
	t.Helper()
	revision, err := mllp.NewSourceRevision(mllp.SourceRevisionInput{
		ArtifactID: "source-adt", RevisionID: "source-1", SourceID: "adt-east",
		ListenAddress: "127.0.0.1:2575", Encoding: "utf-8",
		Framing:          mllp.FramingPolicy{StartByte: mllp.StandardStartByte, EndByte: mllp.StandardEndByte, TrailerByte: mllp.StandardTrailerByte},
		Timeouts:         mllp.TimeoutPolicy{ReadSeconds: 5, WriteSeconds: 5, IdleSeconds: 30, ProcessSeconds: 30},
		TLS:              tlsPolicy,
		Clients:          mllp.ClientPolicy{AllowedCIDRs: []string{"127.0.0.0/8"}, Identities: identities},
		Acknowledgements: mllp.AcknowledgementPolicy{Mode: mllp.AcknowledgementModeApplication, IncludeErrorSegment: true},
		MaxMessageBytes:  1048576, MaxConnections: 32,
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(revision)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "mllp-source.json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
