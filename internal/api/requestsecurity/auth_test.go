package requestsecurity

import (
	"context"
	"errors"
	"strings"
	"testing"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

func TestStaticBearerAuthenticator(t *testing.T) {
	authenticator, err := NewStaticBearerAuthenticator(StaticBearerConfig{
		Token:       "correct-horse-battery-staple",
		TenantID:    "tenant-a",
		PrincipalID: "engineer-1",
		Roles:       []string{"integration:preview", "author"},
	})
	if err != nil {
		t.Fatalf("NewStaticBearerAuthenticator: %v", err)
	}

	tests := []struct {
		name          string
		authorization string
		want          error
	}{
		{name: "missing", want: ErrMissingCredentials},
		{name: "wrong scheme", authorization: "Basic abc", want: ErrInvalidCredentials},
		{name: "empty bearer", authorization: "Bearer ", want: ErrInvalidCredentials},
		{name: "wrong token", authorization: "Bearer wrong", want: ErrInvalidCredentials},
		{name: "extra token", authorization: "Bearer correct-horse-battery-staple extra", want: ErrInvalidCredentials},
		{name: "tab separator", authorization: "Bearer\tcorrect-horse-battery-staple", want: ErrInvalidCredentials},
		{name: "repeated separator", authorization: "Bearer  correct-horse-battery-staple", want: ErrInvalidCredentials},
		{name: "control in token", authorization: "Bearer correct-horse\tbattery-staple", want: ErrInvalidCredentials},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := authenticator.Authenticate(context.Background(), tt.authorization); !errors.Is(err, tt.want) {
				t.Fatalf("Authenticate error = %v, want %v", err, tt.want)
			}
			if err != nil && strings.Contains(err.Error(), "correct-horse-battery-staple") {
				t.Fatal("authentication error disclosed configured credential")
			}
		})
	}

	security, err := authenticator.Authenticate(context.Background(), "Bearer correct-horse-battery-staple")
	if err != nil {
		t.Fatalf("Authenticate valid token: %v", err)
	}
	if security.TenantID != "tenant-a" || security.Principal.ID != "engineer-1" {
		t.Fatalf("security context = %#v", security)
	}
	if security.Principal.Kind != integration.PrincipalKindHuman || security.Principal.AuthMethod != "bearer" {
		t.Fatalf("principal metadata = %#v", security.Principal)
	}
	security.Principal.Roles[0] = "mutated"
	again, err := authenticator.Authenticate(context.Background(), "Bearer correct-horse-battery-staple")
	if err != nil {
		t.Fatalf("Authenticate second token: %v", err)
	}
	if again.Principal.Roles[0] != "integration:preview" {
		t.Fatal("authenticator returned mutable role backing storage")
	}
}

func TestStaticBearerAuthenticatorRejectsUnsafeConfiguration(t *testing.T) {
	tests := []StaticBearerConfig{
		{},
		{Token: "short", TenantID: "tenant-a", PrincipalID: "engineer-1", Roles: []string{"integration:preview"}},
		{Token: "correct horse battery staple", TenantID: "tenant-a", PrincipalID: "engineer-1", Roles: []string{"integration:preview"}},
		{Token: "correct-horse-battery:staple", TenantID: "tenant-a", PrincipalID: "engineer-1", Roles: []string{"integration:preview"}},
		{Token: "correct-horse=battery-staple", TenantID: "tenant-a", PrincipalID: "engineer-1", Roles: []string{"integration:preview"}},
		{Token: "correct-horse-battery-staple", PrincipalID: "engineer-1", Roles: []string{"integration:preview"}},
		{Token: "correct-horse-battery-staple", TenantID: " tenant-a", PrincipalID: "engineer-1", Roles: []string{"integration:preview"}},
		{Token: "correct-horse-battery-staple", TenantID: "tenant-a", Roles: []string{"integration:preview"}},
		{Token: "correct-horse-battery-staple", TenantID: "tenant-a", PrincipalID: "engineer-1"},
		{Token: "correct-horse-battery-staple", TenantID: "tenant-a", PrincipalID: "engineer-1", Roles: []string{"bad role"}},
	}
	for index, cfg := range tests {
		if _, err := NewStaticBearerAuthenticator(cfg); err == nil {
			t.Fatalf("unsafe configuration %d was accepted", index)
		}
	}
}

func TestSecurityContextRoundTripCopiesRoles(t *testing.T) {
	security := integration.SecurityContext{
		TenantID:  "tenant-a",
		Principal: integration.Principal{ID: "engineer-1", Kind: integration.PrincipalKindHuman, AuthMethod: "bearer", Roles: []string{"integration:preview"}},
	}
	ctx := WithSecurityContext(context.Background(), security)
	security.Principal.Roles[0] = "caller-mutated"

	fromContext, ok := SecurityContextFromContext(ctx)
	if !ok || fromContext.Principal.Roles[0] != "integration:preview" {
		t.Fatalf("SecurityContextFromContext = %#v, %v", fromContext, ok)
	}
	fromContext.Principal.Roles[0] = "reader-mutated"
	again, ok := SecurityContextFromContext(ctx)
	if !ok || again.Principal.Roles[0] != "integration:preview" {
		t.Fatal("context security roles were not defensively copied")
	}
	if _, ok := SecurityContextFromContext(context.Background()); ok {
		t.Fatal("empty context reported authenticated security")
	}
}
