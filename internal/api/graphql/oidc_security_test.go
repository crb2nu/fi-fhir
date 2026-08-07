package graphql_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	graphqlapi "gitlab.flexinfer.ai/libs/fi-fhir/internal/api/graphql"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/graphql/resolvers"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/requestsecurity"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/requestsecurity/oidctest"
	integrationpreview "gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/preview"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

func TestOIDCIdentityFlowsThroughGraphQLAuthorization(t *testing.T) {
	issuer, err := oidctest.New()
	if err != nil {
		t.Fatalf("new OIDC issuer: %v", err)
	}
	t.Cleanup(issuer.Close)
	authenticator, err := requestsecurity.NewOIDCAuthenticator(issuer.Context(), requestsecurity.OIDCConfig{
		IssuerURL: issuer.IssuerURL(),
		Audience:  "fi-fhir-graphql",
		TenantID:  "tenant-a",
	})
	if err != nil {
		t.Fatalf("NewOIDCAuthenticator: %v", err)
	}
	config := secureServerConfig(authenticator)
	server, err := graphqlapi.NewServer(resolvers.NewResolver(), config)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	query := []byte(`{"query":"query { health { status } }"}`)

	tests := []struct {
		name       string
		mutate     func(map[string]any)
		wantStatus int
		wantData   bool
		forbidden  bool
	}{
		{name: "expected tenant and role reaches resolver", wantStatus: http.StatusOK, wantData: true},
		{name: "cross tenant stops before resolver", mutate: func(c map[string]any) { c["tenant_id"] = "tenant-b" }, wantStatus: http.StatusUnauthorized},
		{name: "missing role stops before resolver", mutate: func(c map[string]any) { delete(c, "roles") }, wantStatus: http.StatusUnauthorized},
		{name: "unprivileged role stops before resolver", mutate: func(c map[string]any) { c["roles"] = []string{"author"} }, wantStatus: http.StatusOK, forbidden: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims := issuer.Claims()
			if tt.mutate != nil {
				tt.mutate(claims)
			}
			token, err := issuer.Sign(claims, "RS256")
			if err != nil {
				t.Fatalf("sign token: %v", err)
			}
			req := httptest.NewRequest(http.MethodPost, config.Path, bytes.NewReader(query))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+token)
			recorder := httptest.NewRecorder()
			server.Handler().ServeHTTP(recorder, req)

			if recorder.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d, body=%s", recorder.Code, tt.wantStatus, recorder.Body.String())
			}
			hasData := strings.Contains(recorder.Body.String(), `"data":{"health"`)
			if hasData != tt.wantData {
				t.Fatalf("resolver data = %v, want %v, body=%s", hasData, tt.wantData, recorder.Body.String())
			}
			if !tt.wantData && strings.Contains(recorder.Body.String(), "healthy") {
				t.Fatalf("unauthorized request exposed resolver result: %s", recorder.Body.String())
			}
			if got := strings.Contains(recorder.Body.String(), `"code":"FORBIDDEN"`); got != tt.forbidden {
				t.Fatalf("forbidden = %v, want %v, body=%s", got, tt.forbidden, recorder.Body.String())
			}
		})
	}
}

func TestOIDCOperatorReachesIntegrationSessionSSE(t *testing.T) {
	issuer, err := oidctest.New()
	if err != nil {
		t.Fatalf("new OIDC issuer: %v", err)
	}
	t.Cleanup(issuer.Close)
	authenticator, err := requestsecurity.NewOIDCAuthenticator(issuer.Context(), requestsecurity.OIDCConfig{
		IssuerURL: issuer.IssuerURL(),
		Audience:  "fi-fhir-graphql",
		TenantID:  "tenant-a",
	})
	if err != nil {
		t.Fatalf("NewOIDCAuthenticator: %v", err)
	}
	config := secureServerConfig(authenticator)
	config.IntegrationSessionStreaming = true
	server, err := graphqlapi.NewServer(resolvers.NewResolver(), config)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	claims := issuer.Claims()
	claims["roles"] = []string{graphqlapi.GraphQLOperatorRole}
	token, err := issuer.Sign(claims, "RS256")
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	body, err := json.Marshal(map[string]any{
		"query": `subscription SessionEvents { integrationSessionEvents(sessionId: "missing") { id type } }`,
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, config.Path, bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "text/event-stream")
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Origin", "https://ide.example.test")
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)

	responseBody := response.Body.String()
	if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "text/event-stream" {
		t.Fatalf("session stream = status %d content-type=%q body=%s", response.Code, response.Header().Get("Content-Type"), responseBody)
	}
	if strings.Contains(responseBody, `"code":"FORBIDDEN"`) || !strings.Contains(responseBody, "event: complete") {
		t.Fatalf("OIDC operator did not reach session resolver: %s", responseBody)
	}
}

func TestOIDCSubjectAndTenantReachPreviewResolver(t *testing.T) {
	issuer, err := oidctest.New()
	if err != nil {
		t.Fatalf("new OIDC issuer: %v", err)
	}
	t.Cleanup(issuer.Close)
	authenticator, err := requestsecurity.NewOIDCAuthenticator(issuer.Context(), requestsecurity.OIDCConfig{
		IssuerURL: issuer.IssuerURL(),
		Audience:  "fi-fhir-graphql",
		TenantID:  "tenant-a",
	})
	if err != nil {
		t.Fatalf("NewOIDCAuthenticator: %v", err)
	}
	capture := &identityCapturingPreviewService{}
	config := secureServerConfig(authenticator)
	server, err := graphqlapi.NewServer(
		resolvers.NewResolver(resolvers.WithPreviewService(capture)),
		config,
	)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	claims := issuer.Claims()
	claims["sub"] = "clinician-oidc-42"
	claims["roles"] = []string{"integration:preview", "author"}
	token, err := issuer.Sign(claims, "RS256")
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	const identityMutation = `mutation VerifyIdentity($input: PreviewIntegrationMessageInput!) {
  previewIntegrationMessage(input: $input) { mode }
}`
	body := graphQLBody(t, identityMutation, map[string]any{"input": map[string]any{
		"integrationId": "adt-east",
		"data":          "MSH|^~\\&|TEST",
		"correlationId": "correlation-oidc-identity",
		"reason":        "prove verified caller propagation",
	}})
	response := postPreviewGraphQL(t, server.Handler(), body, "Bearer "+token)
	if response.Code != http.StatusOK || !capture.called {
		t.Fatalf("preview resolver was not reached: status=%d body=%s", response.Code, response.Body.String())
	}
	security := capture.security
	if security.TenantID != "tenant-a" || security.Principal.ID != "clinician-oidc-42" {
		t.Fatalf("resolver security identity = %#v", security)
	}
	if security.Principal.Kind != integration.PrincipalKindHuman || security.Principal.AuthMethod != "oidc" {
		t.Fatalf("resolver principal = %#v", security.Principal)
	}
	if got := strings.Join(security.Principal.Roles, ","); got != "author,integration:preview" {
		t.Fatalf("resolver roles = %q", got)
	}
}

type identityCapturingPreviewService struct {
	called   bool
	security integration.SecurityContext
}

func (s *identityCapturingPreviewService) Preview(_ context.Context, security integration.SecurityContext, _ integrationpreview.Input) (integration.ProcessResult, error) {
	s.called = true
	s.security = security
	s.security.Principal.Roles = append([]string(nil), security.Principal.Roles...)
	return integration.ProcessResult{}, errors.New("stop after capturing OIDC identity")
}
