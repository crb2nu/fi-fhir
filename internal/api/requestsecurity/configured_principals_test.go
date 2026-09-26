package requestsecurity_test

import (
	"reflect"
	"testing"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/requestsecurity"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/requestsecurity/oidctest"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

func TestConfiguredPrincipalsReportWhatEachAuthenticatorStamps(t *testing.T) {
	ideRoles := []string{"integration:preview", "graphql:operator", "clinical:read"}
	primary, err := requestsecurity.NewStaticBearerAuthenticator(requestsecurity.StaticBearerConfig{
		Token: "legacy-ide-token-for-testing-only", TenantID: "tenant-a", PrincipalID: "ide-operator", Roles: ideRoles,
	})
	if err != nil {
		t.Fatal(err)
	}
	pair, err := requestsecurity.NewStaticBearerPairAuthenticator(primary, requestsecurity.StaticBearerConfig{
		Token: "mentatlab-service-token-for-testing-only", TenantID: "tenant-a", PrincipalID: "mentatlab",
		Roles: []string{"integration.operator", "integration:preview"},
	})
	if err != nil {
		t.Fatal(err)
	}
	trusted, err := requestsecurity.NewTrustedNetworkAuthenticator(requestsecurity.TrustedNetworkConfig{
		CIDRs: "192.168.50.0/24", TenantID: "tenant-a", PrincipalID: "ide-operator", Roles: ideRoles,
	})
	if err != nil {
		t.Fatal(err)
	}
	fixture, err := oidctest.New()
	if err != nil {
		t.Fatal(err)
	}
	defer fixture.Close()
	access := newAccessAuthenticator(t, fixture, map[string][]string{
		"Zed@Example.com":  {"integration:preview", "clinical:read"},
		"amy@example.com":  {"integration:preview", "graphql:operator"},
		"bob@example.com":  {"integration:preview"},
		"carl@example.com": {"integration:preview", "integration.operator"},
	})

	human := func(id, method string, roles ...string) integration.Principal {
		return integration.Principal{ID: id, Kind: integration.PrincipalKindHuman, AuthMethod: method, Roles: roles}
	}
	tests := []struct {
		name string
		got  []integration.Principal
		want []integration.Principal
	}{
		{"static bearer", primary.ConfiguredPrincipals(), []integration.Principal{human("ide-operator", "bearer", ideRoles...)}},
		{"bearer pair", pair.ConfiguredPrincipals(), []integration.Principal{
			human("ide-operator", "bearer", ideRoles...),
			{ID: "mentatlab", Kind: integration.PrincipalKindService, AuthMethod: "service-bearer", Roles: []string{"integration.operator", "integration:preview"}},
		}},
		{"trusted network", trusted.ConfiguredPrincipals(), []integration.Principal{human("ide-operator", "network", ideRoles...)}},
		{"cloudflare access, sorted by canonical email", access.ConfiguredPrincipals(), []integration.Principal{
			human("amy@example.com", "cloudflare-access", "graphql:operator", "integration:preview"),
			human("bob@example.com", "cloudflare-access", "integration:preview"),
			human("carl@example.com", "cloudflare-access", "integration.operator", "integration:preview"),
			human("zed@example.com", "cloudflare-access", "clinical:read", "integration:preview"),
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !reflect.DeepEqual(tt.got, tt.want) {
				t.Fatalf("ConfiguredPrincipals() = %+v\nwant %+v", tt.got, tt.want)
			}
		})
	}

	t.Run("results are copies", func(t *testing.T) {
		primary.ConfiguredPrincipals()[0].Roles[0] = "mutated"
		access.ConfiguredPrincipals()[0].Roles[0] = "mutated"
		if primary.ConfiguredPrincipals()[0].Roles[0] != "integration:preview" || access.ConfiguredPrincipals()[0].Roles[0] != "graphql:operator" {
			t.Fatal("ConfiguredPrincipals aliases the authenticator's roles")
		}
	})

	t.Run("nil authenticators list nothing", func(t *testing.T) {
		var (
			static *requestsecurity.StaticBearerAuthenticator
			both   *requestsecurity.StaticBearerPairAuthenticator
			lan    *requestsecurity.TrustedNetworkAuthenticator
			edge   *requestsecurity.CloudflareAccessAuthenticator
		)
		if static.ConfiguredPrincipals() != nil || both.ConfiguredPrincipals() != nil || lan.ConfiguredPrincipals() != nil || edge.ConfiguredPrincipals() != nil {
			t.Fatal("a nil authenticator listed a principal")
		}
	})
}
