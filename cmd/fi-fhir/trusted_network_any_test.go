package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/observability"
)

// configureDemoIdentityTest sets the hosted demo's static identity (.loom/40
// D-3): a preview-only principal, a bearer nobody is given, and the trusted
// network under test.
func configureDemoIdentityTest(t *testing.T, cidrs, allowAny string) {
	t.Helper()
	clearGraphQLAuthenticationEnv(t)
	for _, name := range []string{envGraphQLAccessTeamDomain, envGraphQLAccessAudience, envGraphQLAccessPrincipals} {
		t.Setenv(name, "")
	}
	t.Setenv("FI_FHIR_GRAPHQL_AUTH_MODE", graphqlAuthModeStatic)
	t.Setenv("FI_FHIR_GRAPHQL_BEARER_TOKEN", "demo-bearer-nobody-is-given-for-testing")
	t.Setenv("FI_FHIR_GRAPHQL_PRINCIPAL_ID", "fi-fhir-demo-visitor")
	t.Setenv("FI_FHIR_GRAPHQL_ROLES", "integration:preview")
	t.Setenv("FI_FHIR_GRAPHQL_TRUSTED_CIDRS", cidrs)
	t.Setenv(envGraphQLTrustedCIDRsAllowAny, allowAny)
}

func TestTrustedCIDRsAnyAddressIsRefusedWithoutOptIn(t *testing.T) {
	for _, allowAny := range []string{"", "false"} {
		t.Run("allow_any="+allowAny, func(t *testing.T) {
			configureDemoIdentityTest(t, "0.0.0.0/0,::/0", allowAny)
			_, _, _, err := loadGraphQLAuthenticationFromEnv(context.Background(), "tenant-a")
			if err == nil {
				t.Fatal("0.0.0.0/0 was accepted without the opt-in")
			}
			if !strings.Contains(err.Error(), envGraphQLTrustedCIDRsAllowAny) {
				t.Fatalf("error does not name the opt-in: %v", err)
			}
		})
	}
}

func TestTrustedCIDRsAllowAnyRejectsANonBoolean(t *testing.T) {
	configureDemoIdentityTest(t, "0.0.0.0/0", "yes please")
	_, _, _, err := loadGraphQLAuthenticationFromEnv(context.Background(), "tenant-a")
	if err == nil || !strings.Contains(err.Error(), envGraphQLTrustedCIDRsAllowAny) {
		t.Fatalf("error = %v", err)
	}
}

// With the opt-in, serve logs exactly one WARN naming the principal and the
// roles every caller receives; a bounded allowlist logs nothing even with the
// opt-in set.
func TestTrustedCIDRsAllowAnyWarnsNamingTheRoles(t *testing.T) {
	for _, tc := range []struct {
		cidrs string
		warns bool
	}{
		{cidrs: "0.0.0.0/0,::/0", warns: true},
		{cidrs: "192.168.50.0/24", warns: false},
	} {
		t.Run(tc.cidrs, func(t *testing.T) {
			configureDemoIdentityTest(t, tc.cidrs, "true")
			_, trustedNetwork, _, err := loadGraphQLAuthenticationFromEnv(context.Background(), "tenant-a")
			if err != nil {
				t.Fatalf("load: %v", err)
			}
			var output bytes.Buffer
			logger := observability.NewLogger(observability.LogConfig{Level: "info", Format: "json", TenantID: "tenant-a", Output: &output})
			warnTrustedNetworkAdmitsAnyAddress(logger, trustedNetwork)

			raw := strings.TrimSpace(output.String())
			if !tc.warns {
				if raw != "" {
					t.Fatalf("a bounded allowlist warned: %s", raw)
				}
				return
			}
			if strings.Count(raw, "\n") != 0 {
				t.Fatalf("want exactly one line, got %q", raw)
			}
			var line warningLine
			if err := json.Unmarshal([]byte(raw), &line); err != nil {
				t.Fatalf("decode %q: %v", raw, err)
			}
			if line.Level != "WARN" || line.Message != trustedNetworkAdmitsAnyWarning || line.Component != "trusted-network" {
				t.Fatalf("line = %+v", line)
			}
			if line.Mode != "network" || line.PrincipalID != "fi-fhir-demo-visitor" || line.Grant != "integration:preview" {
				t.Fatalf("line does not name the identity and roles: %+v", line)
			}
			if line.DroppedFields != 0 {
				t.Fatalf("dropped %d fields outside the log allowlist", line.DroppedFields)
			}
		})
	}
}

func TestTrustedCIDRsWarningIsSilentWithoutTrustedNetwork(t *testing.T) {
	var output bytes.Buffer
	logger := observability.NewLogger(observability.LogConfig{Level: "info", Format: "json", TenantID: "tenant-a", Output: &output})
	warnTrustedNetworkAdmitsAnyAddress(logger, nil)
	if output.Len() != 0 {
		t.Fatalf("nil trusted network warned: %s", output.String())
	}
}
