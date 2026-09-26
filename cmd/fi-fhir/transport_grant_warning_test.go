package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/requestsecurity/oidctest"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/observability"
)

const (
	productionGraphQLRoles = "integration:preview,graphql:operator,clinical:read"
	operatorBundleRoles    = productionGraphQLRoles + ",integration.operator,integration.delivery.operator,integration.deployment.operator"
)

type warningLine struct {
	Level         string `json:"level"`
	Message       string `json:"msg"`
	Component     string `json:"component"`
	Mode          string `json:"mode"`
	PrincipalID   string `json:"principal_id"`
	Grant         string `json:"grant"`
	Reason        string `json:"reason"`
	DroppedFields int    `json:"dropped_fields"`
}

// loadWarnedIdentities configures every deployment-owned identity the way the
// fi-fhir-api Deployment did in production — a static bearer, the trusted
// network that inherits its roles, the fixed-role service bearer, and two
// Access principals — loads them through the real env loader, and returns the
// warning lines serve would log.
func loadWarnedIdentities(t *testing.T, staticRoles, accessRoles string) []warningLine {
	t.Helper()
	issuer, err := oidctest.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(issuer.Close)

	configureServiceBearerTest(t)
	t.Setenv("FI_FHIR_GRAPHQL_ROLES", staticRoles)
	t.Setenv("FI_FHIR_GRAPHQL_TRUSTED_CIDRS", "192.168.50.0/24")
	tokenPath := filepath.Join(t.TempDir(), "service-token")
	if err := os.WriteFile(tokenPath, []byte("mentatlab-service-token-for-testing-only\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(envGraphQLServiceBearerFile, tokenPath)
	t.Setenv(envGraphQLServicePrincipalID, "mentatlab")
	t.Setenv(envGraphQLAccessTeamDomain, issuer.IssuerURL())
	t.Setenv(envGraphQLAccessAudience, testAccessAudience)
	t.Setenv(envGraphQLAccessPrincipals, "operator@example.com="+accessRoles+";second@example.com="+accessRoles)

	authenticator, trustedNetwork, access, err := loadGraphQLAuthenticationFromEnv(issuer.Context(), "tenant-a")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	runtime := &previewRuntime{authenticator: authenticator, trustedNetwork: trustedNetwork, accessIdentity: access}
	return captureTransportGrantWarnings(t, runtime)
}

func captureTransportGrantWarnings(t *testing.T, runtime *previewRuntime) []warningLine {
	t.Helper()
	var output bytes.Buffer
	logger := observability.NewLogger(observability.LogConfig{Level: "info", Format: "json", TenantID: "tenant-a", Output: &output})
	warnTransportGrantWithoutControlPlaneRole(logger, runtime.configuredGraphQLPrincipals())

	var lines []warningLine
	for _, raw := range strings.Split(strings.TrimSpace(output.String()), "\n") {
		if raw == "" {
			continue
		}
		var line warningLine
		if err := json.Unmarshal([]byte(raw), &line); err != nil {
			t.Fatalf("decode %q: %v", raw, err)
		}
		lines = append(lines, line)
	}
	sort.Slice(lines, func(i, j int) bool { return lines[i].Mode+lines[i].PrincipalID < lines[j].Mode+lines[j].PrincipalID })
	return lines
}

// TestTransportGrantWarningNamesEveryMisconfiguredIdentity: with production's
// 2026-09 grant, serve names each identity that will be refused by the operator
// service — and not the service bearer, which holds integration.operator.
func TestTransportGrantWarningNamesEveryMisconfiguredIdentity(t *testing.T) {
	lines := loadWarnedIdentities(t, productionGraphQLRoles, productionGraphQLRoles)
	want := []struct{ mode, principal string }{
		{"bearer", "ide-operator"},
		{"cloudflare-access", "operator@example.com"},
		{"cloudflare-access", "second@example.com"},
		{"network", "ide-operator"},
	}
	if len(lines) != len(want) {
		t.Fatalf("got %d warning lines, want %d: %+v", len(lines), len(want), lines)
	}
	for i, line := range lines {
		if line.Mode != want[i].mode || line.PrincipalID != want[i].principal {
			t.Errorf("line %d names %s/%s, want %s/%s", i, line.Mode, line.PrincipalID, want[i].mode, want[i].principal)
		}
		if line.Level != "WARN" || line.Message != transportGrantWarning || line.Component != "transport-gate" || line.Grant != "graphql:operator" {
			t.Errorf("line %d = %+v", i, line)
		}
		if line.Reason != "missing roles: integration.operator,integration.delivery.operator,integration.deployment.operator" {
			t.Errorf("line %d reason = %q", i, line.Reason)
		}
		if line.DroppedFields != 0 {
			t.Errorf("line %d dropped %d fields outside the log allowlist", i, line.DroppedFields)
		}
	}
}

func TestTransportGrantWarningIsSilentForTheOperatorBundle(t *testing.T) {
	if lines := loadWarnedIdentities(t, operatorBundleRoles, operatorBundleRoles); len(lines) != 0 {
		t.Fatalf("the documented bundle produced warnings: %+v", lines)
	}
}

// A grant that reaches the control plane's reads but not its controls is least
// privilege, not the outage; only the missing read role warns.
func TestTransportGrantWarningIgnoresAReadOnlyOperator(t *testing.T) {
	readOnly := productionGraphQLRoles + ",integration.operator"
	if lines := loadWarnedIdentities(t, readOnly, readOnly); len(lines) != 0 {
		t.Fatalf("a read-only operator produced warnings: %+v", lines)
	}
}

func TestTransportGrantWarningHasNothingToCheckInOIDCMode(t *testing.T) {
	issuer, err := oidctest.New()
	if err != nil {
		t.Fatal(err)
	}
	defer issuer.Close()
	clearGraphQLAuthenticationEnv(t)
	for _, name := range []string{envGraphQLAccessTeamDomain, envGraphQLAccessAudience, envGraphQLAccessPrincipals} {
		t.Setenv(name, "")
	}
	t.Setenv("FI_FHIR_GRAPHQL_AUTH_MODE", graphqlAuthModeOIDC)
	t.Setenv("FI_FHIR_GRAPHQL_OIDC_ISSUER_URL", issuer.IssuerURL())
	t.Setenv("FI_FHIR_GRAPHQL_OIDC_AUDIENCE", "fi-fhir-graphql")
	authenticator, trustedNetwork, access, err := loadGraphQLAuthenticationFromEnv(issuer.Context(), "tenant-a")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	runtime := &previewRuntime{authenticator: authenticator, trustedNetwork: trustedNetwork, accessIdentity: access}
	if principals := runtime.configuredGraphQLPrincipals(); len(principals) != 0 {
		t.Fatalf("OIDC mode listed deployment-owned principals: %+v", principals)
	}
}
