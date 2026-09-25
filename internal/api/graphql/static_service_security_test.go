package graphql_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	graphqlapi "gitlab.flexinfer.ai/libs/fi-fhir/internal/api/graphql"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/graphql/resolvers"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/requestsecurity"
)

func TestServiceBearerCannotInheritTrustedNetworkRoles(t *testing.T) {
	const serviceToken = "mentatlab-service-token-for-testing-only"
	legacy := testOperatorAuthenticator(t).(*requestsecurity.StaticBearerAuthenticator)
	authenticator, err := requestsecurity.NewStaticBearerPairAuthenticator(legacy, requestsecurity.StaticBearerConfig{
		Token: serviceToken, TenantID: "tenant-a", PrincipalID: "mentatlab",
		Roles: []string{"integration.operator", "integration:preview"},
	})
	if err != nil {
		t.Fatal(err)
	}
	trusted, err := requestsecurity.NewTrustedNetworkAuthenticator(requestsecurity.TrustedNetworkConfig{
		CIDRs: "192.168.50.0/24", TenantID: "tenant-a", PrincipalID: "ide-operator",
		Roles: []string{"graphql:operator", "clinical:read", "integration:preview"},
	})
	if err != nil {
		t.Fatal(err)
	}
	config := secureServerConfig(authenticator)
	config.TrustedNetworkAuthenticator = trusted
	server, err := graphqlapi.NewServer(resolvers.NewResolver(), config)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, query     string
		authorization   []string
		wantStatus      int
		wantForbidden   bool
		wantUnavailable bool
	}{
		{"narrow service denied clinical reads", `query { events(first: 1) { edges { node { id } } } }`, []string{"Bearer " + serviceToken}, 200, true, false},
		{"narrow service denied session writes", `mutation { createIntegrationSession(input: {name: "blocked"}) { id } }`, []string{"Bearer " + serviceToken}, 200, true, false},
		{"operator read reaches its service", `query { operatorCircuits { __typename } }`, []string{"Bearer " + serviceToken}, 200, false, true},
		{"preview reaches its service", `mutation { previewIntegrationMessage(input: {integrationId: "synthetic", data: "synthetic", correlationId: "test", reason: "test"}) { __typename } }`, []string{"Bearer " + serviceToken}, 200, false, true},
		{"legacy bearer retained", `query { events(first: 1) { edges { node { id } } } }`, []string{"Bearer " + testBearerToken}, 200, false, false},
		{"headerless network retained", `query { events(first: 1) { edges { node { id } } } }`, nil, 200, false, false},
		{"wrong credential not rescued", `query { health { status } }`, []string{"Bearer wrong"}, 401, false, false},
		{"empty credential not rescued", `query { health { status } }`, []string{""}, 401, false, false},
		{"ambiguous credentials rejected", `query { health { status } }`, []string{"Bearer " + serviceToken, "Bearer " + testBearerToken}, 401, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, err := json.Marshal(map[string]string{"query": tc.query})
			if err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest(http.MethodPost, config.Path, bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Real-IP", "192.168.50.24")
			for _, value := range tc.authorization {
				req.Header.Add("Authorization", value)
			}
			recorder := httptest.NewRecorder()
			server.Handler().ServeHTTP(recorder, req)
			response := recorder.Body.String()
			if recorder.Code != tc.wantStatus || strings.Contains(response, `"code":"FORBIDDEN"`) != tc.wantForbidden || strings.Contains(response, "unavailable") != tc.wantUnavailable {
				t.Fatalf("status=%d body=%s", recorder.Code, response)
			}
			if strings.Contains(response, serviceToken) || strings.Contains(response, testBearerToken) {
				t.Fatal("response exposed a bearer credential")
			}
		})
	}
}
