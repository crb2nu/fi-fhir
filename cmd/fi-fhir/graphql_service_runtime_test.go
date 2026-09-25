package main

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

func TestGraphQLServiceCredentialConfiguration(t *testing.T) {
	configureServiceBearerTest(t)
	tokenPath := filepath.Join(t.TempDir(), "service-token")
	const token = "mentatlab-service-token-for-testing-only"
	if err := os.WriteFile(tokenPath, []byte(token+"\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(envGraphQLServiceBearerFile, tokenPath)
	t.Setenv(envGraphQLServicePrincipalID, "mentatlab")
	authenticator, _, _, err := loadGraphQLAuthenticationFromEnv(context.Background(), "tenant-a")
	if err != nil {
		t.Fatal(err)
	}
	security, err := authenticator.Authenticate(context.Background(), "Bearer "+token)
	if err != nil {
		t.Fatal(err)
	}
	if security.TenantID != "tenant-a" || security.Principal.ID != "mentatlab" || security.Principal.Kind != integration.PrincipalKindService || !reflect.DeepEqual(security.Principal.Roles, []string{"integration.operator", "integration:preview"}) {
		t.Fatalf("service identity = %#v", security)
	}
	legacy, err := authenticator.Authenticate(context.Background(), "Bearer legacy-ide-token-for-testing-only")
	if err != nil || legacy.Principal.ID != "ide-operator" || !reflect.DeepEqual(legacy.Principal.Roles, []string{"integration:preview", "graphql:operator", "clinical:read"}) {
		t.Fatal("existing IDE identity changed")
	}
}

func TestGraphQLServiceCredentialFailsClosed(t *testing.T) {
	for _, tc := range []struct {
		name, token, principal string
		omitPath               bool
	}{
		{"file only", "mentatlab-service-token-for-testing-only", "", false},
		{"principal only", "", "mentatlab", true},
		{"blank principal", "mentatlab-service-token-for-testing-only", " ", false},
		{"short secret", "short", "mentatlab", false},
		{"multiline secret", "mentatlab-service-token-for-testing-only\nextra", "mentatlab", false},
		{"oversized secret", strings.Repeat("s", maxBearerTokenFileBytes+1), "mentatlab", false},
		{"same secret", "legacy-ide-token-for-testing-only", "mentatlab", false},
		{"same principal", "mentatlab-service-token-for-testing-only", "ide-operator", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			configureServiceBearerTest(t)
			if !tc.omitPath {
				path := filepath.Join(t.TempDir(), "service-token")
				if err := os.WriteFile(path, []byte(tc.token), 0o600); err != nil {
					t.Fatal(err)
				}
				t.Setenv(envGraphQLServiceBearerFile, path)
			}
			t.Setenv(envGraphQLServicePrincipalID, tc.principal)
			_, _, _, err := loadGraphQLAuthenticationFromEnv(context.Background(), "tenant-a")
			if err == nil {
				t.Fatal("invalid service credential configuration accepted")
			}
			if tc.token != "" && strings.Contains(err.Error(), tc.token) {
				t.Fatal("configuration error disclosed a credential")
			}
		})
	}
}

func configureServiceBearerTest(t *testing.T) {
	t.Helper()
	clearGraphQLAuthenticationEnv(t)
	t.Setenv("FI_FHIR_GRAPHQL_AUTH_MODE", "static")
	t.Setenv("FI_FHIR_GRAPHQL_BEARER_TOKEN", "legacy-ide-token-for-testing-only")
	t.Setenv("FI_FHIR_GRAPHQL_PRINCIPAL_ID", "ide-operator")
	t.Setenv("FI_FHIR_GRAPHQL_ROLES", "integration:preview,graphql:operator,clinical:read")
	for _, name := range []string{envGraphQLAccessTeamDomain, envGraphQLAccessAudience, envGraphQLAccessPrincipals} {
		t.Setenv(name, "")
	}
}
