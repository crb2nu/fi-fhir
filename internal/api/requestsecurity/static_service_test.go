package requestsecurity

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

func TestStaticBearerPairKeepsIdentitiesSeparate(t *testing.T) {
	primary, err := NewStaticBearerAuthenticator(StaticBearerConfig{
		Token: "legacy-ide-token-for-testing-only", TenantID: "tenant-a", PrincipalID: "ide-operator",
		Roles: []string{"integration:preview", "graphql:operator", "clinical:read"},
	})
	if err != nil {
		t.Fatal(err)
	}
	serviceRoles := []string{"integration.operator", "integration:preview"}
	authenticator, err := NewStaticBearerPairAuthenticator(primary, StaticBearerConfig{
		Token: "mentatlab-service-token-for-testing-only", TenantID: "tenant-a", PrincipalID: "mentatlab",
		Roles: serviceRoles,
	})
	if err != nil {
		t.Fatal(err)
	}
	serviceRoles[0] = "graphql:operator"

	for _, tc := range []struct {
		name, token, principal, method string
		kind                           integration.PrincipalKind
		roles                          []string
	}{
		{"IDE", "legacy-ide-token-for-testing-only", "ide-operator", "bearer", integration.PrincipalKindHuman, []string{"integration:preview", "graphql:operator", "clinical:read"}},
		{"service", "mentatlab-service-token-for-testing-only", "mentatlab", "service-bearer", integration.PrincipalKindService, []string{"integration.operator", "integration:preview"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			security, err := authenticator.Authenticate(context.Background(), "Bearer "+tc.token)
			if err != nil {
				t.Fatal(err)
			}
			if security.TenantID != "tenant-a" || security.Principal.ID != tc.principal || security.Principal.Kind != tc.kind || security.Principal.AuthMethod != tc.method || !reflect.DeepEqual(security.Principal.Roles, tc.roles) {
				t.Fatalf("incorrect identity: %#v", security)
			}
			security.Principal.Roles[0] = "mutated"
			again, err := authenticator.Authenticate(context.Background(), "Bearer "+tc.token)
			if err != nil || !reflect.DeepEqual(again.Principal.Roles, tc.roles) {
				t.Fatal("returned identity did not preserve its own role copy")
			}
		})
	}

	for _, authorization := range []string{"Bearer invalid", "Bearer\tmentatlab-service-token-for-testing-only", "Bearer  mentatlab-service-token-for-testing-only"} {
		if _, err := authenticator.Authenticate(context.Background(), authorization); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("malformed or unknown credential error = %v", err)
		}
	}
	if _, err := authenticator.Authenticate(context.Background(), ""); !errors.Is(err, ErrMissingCredentials) {
		t.Fatalf("missing credential error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := authenticator.Authenticate(ctx, "Bearer mentatlab-service-token-for-testing-only"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled context error = %v", err)
	}
}

func TestStaticBearerPairRejectsAmbiguousConfiguration(t *testing.T) {
	primary, err := NewStaticBearerAuthenticator(StaticBearerConfig{
		Token: "legacy-ide-token-for-testing-only", TenantID: "tenant-a", PrincipalID: "ide-operator", Roles: []string{"integration:preview"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		edit func(*StaticBearerConfig)
	}{
		{"same token", func(c *StaticBearerConfig) { c.Token = "legacy-ide-token-for-testing-only" }},
		{"same principal", func(c *StaticBearerConfig) { c.PrincipalID = "ide-operator" }},
		{"different tenant", func(c *StaticBearerConfig) { c.TenantID = "tenant-b" }},
		{"invalid token", func(c *StaticBearerConfig) { c.Token = "short" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := StaticBearerConfig{Token: "mentatlab-service-token-for-testing-only", TenantID: "tenant-a", PrincipalID: "mentatlab", Roles: []string{"integration.operator", "integration:preview"}}
			tc.edit(&config)
			if _, err := NewStaticBearerPairAuthenticator(primary, config); err == nil {
				t.Fatal("unsafe service identity was accepted")
			} else if strings.Contains(err.Error(), config.Token) {
				t.Fatal("configuration error exposed credential")
			}
		})
	}
	if _, err := NewStaticBearerPairAuthenticator(nil, StaticBearerConfig{}); err == nil {
		t.Fatal("missing primary identity was accepted")
	}
}
