package graphql_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"

	graphqlapi "gitlab.flexinfer.ai/libs/fi-fhir/internal/api/graphql"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/graphql/resolvers"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/requestsecurity"
)

const testBearerToken = "correct-horse-battery-staple"

func TestServerRejectsIncompleteSecurityConfiguration(t *testing.T) {
	authenticator := testAuthenticator(t)
	tests := []struct {
		name   string
		mutate func(*graphqlapi.ServerConfig)
	}{
		{name: "missing authenticator", mutate: func(c *graphqlapi.ServerConfig) { c.Authenticator = nil }},
		{name: "empty origins", mutate: func(c *graphqlapi.ServerConfig) { c.AllowedOrigins = nil }},
		{name: "wildcard origin", mutate: func(c *graphqlapi.ServerConfig) { c.AllowedOrigins = []string{"*"} }},
		{name: "origin with path", mutate: func(c *graphqlapi.ServerConfig) { c.AllowedOrigins = []string{"https://ide.example.test/path"} }},
		{name: "unbounded body", mutate: func(c *graphqlapi.ServerConfig) { c.MaxRequestBodyBytes = 0 }},
		{name: "unbounded depth", mutate: func(c *graphqlapi.ServerConfig) { c.MaxDepth = 0 }},
		{name: "unbounded complexity", mutate: func(c *graphqlapi.ServerConfig) { c.MaxComplexity = 0 }},
		{name: "zero timeout", mutate: func(c *graphqlapi.ServerConfig) { c.Timeout = 0 }},
		{name: "negative timeout", mutate: func(c *graphqlapi.ServerConfig) { c.Timeout = -1 }},
		{name: "HTTP path collision", mutate: func(c *graphqlapi.ServerConfig) { c.Path = "/health" }},
		{name: "WebSocket path collision", mutate: func(c *graphqlapi.ServerConfig) { c.WebSocketPath = "/health" }},
		{name: "playground path collision", mutate: func(c *graphqlapi.ServerConfig) {
			c.PlaygroundEnabled = true
			c.PlaygroundPath = c.Path
		}},
		{name: "root HTTP catch-all", mutate: func(c *graphqlapi.ServerConfig) { c.Path = "/" }},
		{name: "ServeMux wildcard syntax", mutate: func(c *graphqlapi.ServerConfig) { c.Path = "/graphql/{operation}" }},
		{name: "non-canonical path", mutate: func(c *graphqlapi.ServerConfig) { c.Path = "/api/../graphql" }},
		{name: "double-slash path", mutate: func(c *graphqlapi.ServerConfig) { c.Path = "/api//graphql" }},
		{name: "subtree pattern", mutate: func(c *graphqlapi.ServerConfig) { c.Path = "/graphql/" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := secureServerConfig(authenticator)
			tt.mutate(config)
			if _, err := graphqlapi.NewServer(resolvers.NewResolver(), config); err == nil {
				t.Fatal("unsafe server configuration was accepted")
			}
		})
	}
}

func TestGraphQLHTTPBoundary(t *testing.T) {
	config := secureServerConfig(testAuthenticator(t))
	server, err := graphqlapi.NewServer(resolvers.NewResolver(), config)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	handler := server.Handler()
	query := []byte(`{"query":"query { health { status } }"}`)

	t.Run("authenticated exact origin POST", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, config.Path, bytes.NewReader(query))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+testBearerToken)
		req.Header.Set("Origin", "https://ide.example.test")
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, req)
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, body=%s", recorder.Code, recorder.Body.String())
		}
		if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "https://ide.example.test" {
			t.Fatalf("allow origin = %q", got)
		}
		if got := recorder.Header().Values("Vary"); !containsHeaderToken(got, "Origin") {
			t.Fatalf("Vary = %v, want Origin", got)
		}
	})

	tests := []struct {
		name       string
		method     string
		origin     string
		auth       string
		body       io.Reader
		chunked    bool
		wantStatus int
	}{
		{name: "GET", method: http.MethodGet, auth: "Bearer " + testBearerToken, wantStatus: http.StatusMethodNotAllowed},
		{name: "multipart", method: http.MethodPost, auth: "Bearer " + testBearerToken, body: bytes.NewReader(query), wantStatus: http.StatusUnsupportedMediaType},
		{name: "missing auth", method: http.MethodPost, body: bytes.NewReader(query), wantStatus: http.StatusUnauthorized},
		{name: "bad auth", method: http.MethodPost, auth: "Bearer wrong", body: bytes.NewReader(query), wantStatus: http.StatusUnauthorized},
		{name: "disallowed origin", method: http.MethodPost, origin: "https://evil.example.test", auth: "Bearer " + testBearerToken, body: bytes.NewReader(query), wantStatus: http.StatusForbidden},
		{name: "fixed oversized", method: http.MethodPost, auth: "Bearer " + testBearerToken, body: strings.NewReader(strings.Repeat("x", int(config.MaxRequestBodyBytes)+1)), wantStatus: http.StatusRequestEntityTooLarge},
		{name: "chunked oversized", method: http.MethodPost, auth: "Bearer " + testBearerToken, body: strings.NewReader(strings.Repeat("x", int(config.MaxRequestBodyBytes)+1)), chunked: true, wantStatus: http.StatusRequestEntityTooLarge},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := tt.body
			if body == nil {
				body = http.NoBody
			}
			req := httptest.NewRequest(tt.method, config.Path, body)
			if tt.chunked {
				req.ContentLength = -1
			}
			if tt.name != "multipart" {
				req.Header.Set("Content-Type", "application/json")
			} else {
				req.Header.Set("Content-Type", "multipart/form-data; boundary=test")
			}
			if tt.auth != "" {
				req.Header.Set("Authorization", tt.auth)
			}
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, req)
			if recorder.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d, body=%s", recorder.Code, tt.wantStatus, recorder.Body.String())
			}
			if tt.origin == "https://evil.example.test" && recorder.Header().Get("Access-Control-Allow-Origin") != "" {
				t.Fatal("disallowed origin was reflected")
			}
		})
	}

	t.Run("exact preflight", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodOptions, config.Path, nil)
		req.Header.Set("Origin", "https://ide.example.test")
		req.Header.Set("Access-Control-Request-Method", http.MethodPost)
		req.Header.Set("Access-Control-Request-Headers", "authorization, content-type")
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, req)
		if recorder.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want 204", recorder.Code)
		}
		if recorder.Header().Get("Access-Control-Allow-Origin") != "https://ide.example.test" {
			t.Fatalf("allow origin = %q", recorder.Header().Get("Access-Control-Allow-Origin"))
		}
		if strings.Contains(recorder.Header().Get("Access-Control-Allow-Origin"), "*") {
			t.Fatal("preflight emitted wildcard origin")
		}
	})

	t.Run("depth limit", func(t *testing.T) {
		limitedConfig := secureServerConfig(testOperatorAuthenticator(t))
		limitedConfig.MaxDepth = 2
		limitedServer, err := graphqlapi.NewServer(resolvers.NewResolver(), limitedConfig)
		if err != nil {
			t.Fatalf("NewServer: %v", err)
		}
		deepQuery := []byte(`{"query":"query { events(first: 1) { edges { node { id } } } }"}`)
		req := httptest.NewRequest(http.MethodPost, limitedConfig.Path, bytes.NewReader(deepQuery))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+testBearerToken)
		recorder := httptest.NewRecorder()
		limitedServer.Handler().ServeHTTP(recorder, req)
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 GraphQL rejection, body=%s", recorder.Code, recorder.Body.String())
		}
		if !strings.Contains(recorder.Body.String(), "QUERY_DEPTH_LIMIT_EXCEEDED") {
			t.Fatalf("depth rejection omitted safe error code: %s", recorder.Body.String())
		}
	})

	t.Run("preview role cannot invoke legacy operations", func(t *testing.T) {
		legacyQuery := []byte(`{"query":"mutation { createIntegrationSession(input: {name: \"blocked\"}) { id } }"}`)
		req := httptest.NewRequest(http.MethodPost, config.Path, bytes.NewReader(legacyQuery))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+testBearerToken)
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, req)
		if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"code":"FORBIDDEN"`) {
			t.Fatalf("legacy operation authorization = status %d body=%s", recorder.Code, recorder.Body.String())
		}
	})

	t.Run("preview role allowlist follows resolved root fields", func(t *testing.T) {
		tests := []struct {
			name          string
			query         string
			operationName string
			wantForbidden bool
		}{
			{name: "health alias", query: `query Safe { ready: health { status } }`},
			{name: "selected safe operation", query: `query Safe { health { status } } query Unsafe { events(first: 1) { edges { node { id } } } }`, operationName: "Safe"},
			{name: "mixed root", query: `query Mixed { health { status } events(first: 1) { edges { node { id } } } }`, wantForbidden: true},
			{name: "fragment spread", query: `query Hidden { ...LegacyFields } fragment LegacyFields on Query { events(first: 1) { edges { node { id } } } }`, wantForbidden: true},
			{name: "inline fragment", query: `query Hidden { ... on Query { events(first: 1) { edges { node { id } } } } }`, wantForbidden: true},
			{name: "aliased legacy field", query: `query Hidden { health: events(first: 1) { edges { node { id } } } }`, wantForbidden: true},
			{name: "schema introspection", query: `query Hidden { __schema { queryType { name } } }`, wantForbidden: true},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				payload := map[string]any{"query": tt.query}
				if tt.operationName != "" {
					payload["operationName"] = tt.operationName
				}
				body, err := json.Marshal(payload)
				if err != nil {
					t.Fatalf("marshal request: %v", err)
				}
				req := httptest.NewRequest(http.MethodPost, config.Path, bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", "Bearer "+testBearerToken)
				recorder := httptest.NewRecorder()
				handler.ServeHTTP(recorder, req)
				forbidden := strings.Contains(recorder.Body.String(), `"code":"FORBIDDEN"`)
				if forbidden != tt.wantForbidden {
					t.Fatalf("forbidden = %v, want %v, status=%d body=%s", forbidden, tt.wantForbidden, recorder.Code, recorder.Body.String())
				}
			})
		}
	})
}

func TestGraphQLWebSocketTransportIsDisabled(t *testing.T) {
	config := secureServerConfig(testAuthenticator(t))
	server, err := graphqlapi.NewServer(resolvers.NewResolver(), config)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(httpServer.Close)
	dialer := websocket.Dialer{Subprotocols: []string{"graphql-transport-ws"}}
	for _, path := range []string{config.Path, config.WebSocketPath} {
		t.Run(path, func(t *testing.T) {
			wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http") + path
			connection, response, err := dialer.Dial(wsURL, http.Header{"Origin": []string{"https://ide.example.test"}})
			if connection != nil {
				_ = connection.Close()
			}
			if response != nil {
				defer response.Body.Close()
			}
			if err == nil {
				t.Fatal("disabled WebSocket transport upgraded")
			}
			if response == nil || response.StatusCode == http.StatusSwitchingProtocols {
				t.Fatalf("disabled WebSocket response = %#v", response)
			}
		})
	}
}

func TestGraphQLMalformedBodiesNeverReflectPHI(t *testing.T) {
	config := secureServerConfig(testAuthenticator(t))
	server, err := graphqlapi.NewServer(resolvers.NewResolver(), config)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	const sentinel = "PHI-SENTINEL-DOE-JANE-MRN123"
	tests := []struct {
		name string
		body string
	}{
		{name: "malformed JSON", body: `{"query":"mutation { previewIntegrationMessage(input: {data: \"` + sentinel + `\"})`},
		{name: "variables wrong type", body: `{"query":"query { health { status } }","variables":"` + sentinel + `"}`},
		{name: "extensions wrong type", body: `{"query":"query { health { status } }","extensions":"` + sentinel + `"}`},
		{name: "unknown field", body: `{"query":"query { health { status } }","rawPayload":"` + sentinel + `"}`},
		{name: "trailing JSON", body: `{"query":"query { health { status } }"} "` + sentinel + `"`},
		{name: "malformed GraphQL", body: `{"query":"query { PHI_SENTINEL_DOE_JANE("}`},
		{name: "duplicate query", body: `{"query":123,"query":"query { health { status } }","variables":{"sentinel":"` + sentinel + `"}}`},
		{name: "case-aliased query", body: `{"query":123,"Query":"query { health { status } }","variables":{"sentinel":"` + sentinel + `"}}`},
		{name: "duplicate operation name", body: `{"query":"query Safe { health { status } }","operationName":"` + sentinel + `","operationName":"Safe"}`},
		{name: "duplicate variables", body: `{"query":"query { health { status } }","variables":"` + sentinel + `","variables":{}}`},
		{name: "duplicate extensions", body: `{"query":"query { health { status } }","extensions":"` + sentinel + `","extensions":{}}`},
		{name: "nested duplicate", body: `{"query":"query { health { status } }","variables":{"patient":"` + sentinel + `","patient":"safe"}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, config.Path, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+testBearerToken)
			recorder := httptest.NewRecorder()
			server.Handler().ServeHTTP(recorder, req)
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400, body=%s", recorder.Code, recorder.Body.String())
			}
			if got := recorder.Body.String(); got != "invalid GraphQL JSON request\n" {
				t.Fatalf("unsafe error body = %q", got)
			}
			if strings.Contains(recorder.Body.String(), sentinel) {
				t.Fatal("response reflected raw PHI sentinel")
			}
		})
	}
}

func TestGraphQLPreviewErrorsNeverReflectQueryInput(t *testing.T) {
	config := secureServerConfig(testAuthenticator(t))
	server, err := graphqlapi.NewServer(resolvers.NewResolver(), config)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	const sentinel = "PHISENTINELDOEJANEMRN123"
	body := `{"query":"query { ` + sentinel + ` }"}`
	req := httptest.NewRequest(http.MethodPost, config.Path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+testBearerToken)
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, req)
	if strings.Contains(recorder.Body.String(), sentinel) {
		t.Fatalf("GraphQL error reflected query input: %s", recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "GraphQL request is invalid") {
		t.Fatalf("GraphQL error was not catalog-safe: %s", recorder.Body.String())
	}
}

func secureServerConfig(authenticator requestsecurity.Authenticator) *graphqlapi.ServerConfig {
	config := graphqlapi.DefaultServerConfig()
	config.PlaygroundEnabled = false
	config.AllowedOrigins = []string{"https://ide.example.test"}
	config.MaxRequestBodyBytes = 1024
	config.Authenticator = authenticator
	return config
}

func testAuthenticator(t *testing.T) requestsecurity.Authenticator {
	t.Helper()
	authenticator, err := requestsecurity.NewStaticBearerAuthenticator(requestsecurity.StaticBearerConfig{
		Token:       testBearerToken,
		TenantID:    "tenant-a",
		PrincipalID: "engineer-1",
		Roles:       []string{"integration:preview"},
	})
	if err != nil {
		t.Fatalf("NewStaticBearerAuthenticator: %v", err)
	}
	return authenticator
}

func testOperatorAuthenticator(t *testing.T) requestsecurity.Authenticator {
	t.Helper()
	authenticator, err := requestsecurity.NewStaticBearerAuthenticator(requestsecurity.StaticBearerConfig{
		Token:       testBearerToken,
		TenantID:    "tenant-a",
		PrincipalID: "operator-1",
		Roles:       []string{graphqlapi.GraphQLOperatorRole},
	})
	if err != nil {
		t.Fatalf("NewStaticBearerAuthenticator: %v", err)
	}
	return authenticator
}

func containsHeaderToken(values []string, want string) bool {
	for _, value := range values {
		for _, token := range strings.Split(value, ",") {
			if strings.EqualFold(strings.TrimSpace(token), want) {
				return true
			}
		}
	}
	return false
}
