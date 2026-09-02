package graphql_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	graphqlapi "gitlab.flexinfer.ai/libs/fi-fhir/internal/api/graphql"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/graphql/resolvers"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/fhir"
)

func TestCapabilityStatementEndpoint(t *testing.T) {
	// The server fails closed without an authenticator and explicit origins;
	// reuse the package's secure test config. /metadata itself stays
	// unauthenticated — the request below carries no bearer token.
	config := secureServerConfig(testAuthenticator(t))
	config.SoftwareVersion = "test-version"
	server, err := graphqlapi.NewServer(resolvers.NewResolver(), config)
	if err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, graphqlapi.CapabilityStatementPath, nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Header().Get("Content-Type"); got != "application/fhir+json; charset=utf-8" {
		t.Fatalf("Content-Type = %q", got)
	}
	var statement fhir.CapabilityStatement
	if err := json.Unmarshal(recorder.Body.Bytes(), &statement); err != nil {
		t.Fatalf("decode statement: %v", err)
	}
	if statement.ResourceType != "CapabilityStatement" || statement.Software.Version != "test-version" {
		t.Fatalf("unexpected statement: %#v", statement)
	}
	if len(statement.Rest) != 1 || len(statement.Rest[0].Resource) != len(fhir.SupportedResourceTypes()) {
		t.Fatalf("resource count = %d, want %d", len(statement.Rest[0].Resource), len(fhir.SupportedResourceTypes()))
	}
}

func TestCapabilityStatementEndpointRejectsPost(t *testing.T) {
	config := secureServerConfig(testAuthenticator(t))
	server, err := graphqlapi.NewServer(resolvers.NewResolver(), config)
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, graphqlapi.CapabilityStatementPath, nil))
	if recorder.Code != http.StatusMethodNotAllowed || recorder.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("POST status/Allow = %d/%q", recorder.Code, recorder.Header().Get("Allow"))
	}
}
