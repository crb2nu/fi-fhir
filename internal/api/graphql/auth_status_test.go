package graphql_test

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	graphqlapi "gitlab.flexinfer.ai/libs/fi-fhir/internal/api/graphql"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/graphql/resolvers"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/requestsecurity"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/requestsecurity/oidctest"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/delivery"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/destination"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/lifecycle"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/operator"
)

const (
	trustedClient   = "192.168.50.24"
	untrustedClient = "203.0.113.9"
	serviceToken    = "service-horse-battery-staple-0001"
)

// productionGrant is what the static bearer, the trusted network, and both
// Access principals held in production from 2026-09-05 to 2026-09-25.
var productionGrant = []string{"integration:preview", graphqlapi.GraphQLOperatorRole, "clinical:read"}

func operatorBundle() []string {
	return append(append([]string{}, productionGrant...), graphqlapi.OperatorControlPlaneRoles()...)
}

func trustedNetwork(t *testing.T, roles ...string) *requestsecurity.TrustedNetworkAuthenticator {
	t.Helper()
	trusted, err := requestsecurity.NewTrustedNetworkAuthenticator(requestsecurity.TrustedNetworkConfig{
		CIDRs:       "192.168.50.0/24",
		TenantID:    "tenant-a",
		PrincipalID: "fi-fhir-ide-operator",
		Roles:       roles,
	})
	if err != nil {
		t.Fatalf("NewTrustedNetworkAuthenticator: %v", err)
	}
	return trusted
}

func getAuthStatus(t *testing.T, handler http.Handler, configure func(*http.Request)) map[string]any {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, graphqlapi.AuthStatusPath, nil)
	configure(req)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if recorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control = %q", recorder.Header().Get("Cache-Control"))
	}
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v body=%s", err, recorder.Body.String())
	}
	return body
}

// TestAuthStatusTrustedNetworkShape is the R-A day-1 gate, inverted. On main
// at a62e9ee6 the same request returned exactly
// {"authenticated":true,"authVia":"network"} (verified before this change);
// it now carries the principal, roles, capabilities, and missing roles, and
// with production's 2026-09 grant it says what the operator page could not.
func TestAuthStatusTrustedNetworkShape(t *testing.T) {
	config := secureServerConfig(testAuthenticator(t))
	config.TrustedNetworkAuthenticator = trustedNetwork(t, productionGrant...)
	config.LLMConfigured = true
	server, err := graphqlapi.NewServer(resolvers.NewResolver(), config)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	got := getAuthStatus(t, server.Handler(), func(r *http.Request) { r.Header.Set("X-Real-IP", trustedClient) })
	var want map[string]any
	if err := json.Unmarshal([]byte(`{
		"authenticated": true,
		"authVia": "network",
		"principal": "fi-fhir-ide-operator",
		"roles": ["integration:preview", "graphql:operator", "clinical:read"],
		"capabilities": {
			"operatorRead": false, "operatorDelivery": false, "operatorDeployment": false,
			"clinicalRead": true, "integrationSessions": false, "streaming": false,
			"subscriptions": [], "llm": {"configured": true}
		},
		"missingRoles": {
			"operatorRead": ["integration.operator"],
			"operatorDelivery": ["integration.operator", "integration.delivery.operator"],
			"operatorDeployment": ["integration.operator", "integration.deployment.operator"],
			"clinicalRead": []
		}
	}`), &want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		gotJSON, _ := json.Marshal(got)
		t.Fatalf("status body = %s", gotJSON)
	}
}

// TestAuthStatusUnauthenticatedShapeIsUnchanged: every caller the GraphQL route
// would answer 401 keeps the pre-contract body, exactly.
func TestAuthStatusUnauthenticatedShapeIsUnchanged(t *testing.T) {
	config := secureServerConfig(testAuthenticator(t))
	config.TrustedNetworkAuthenticator = trustedNetwork(t, operatorBundle()...)
	server, err := graphqlapi.NewServer(resolvers.NewResolver(), config)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	for name, configure := range map[string]func(*http.Request){
		"off-network, no credential": func(r *http.Request) { r.Header.Set("X-Real-IP", untrustedClient) },
		"stale bearer on the LAN": func(r *http.Request) {
			r.Header.Set("X-Real-IP", trustedClient)
			r.Header.Set("Authorization", "Bearer stale-browser-token")
		},
		"repeated bearer headers": func(r *http.Request) {
			r.Header.Set("X-Real-IP", trustedClient)
			r.Header.Add("Authorization", "Bearer "+testBearerToken)
			r.Header.Add("Authorization", "Bearer "+testBearerToken)
		},
	} {
		t.Run(name, func(t *testing.T) {
			got := getAuthStatus(t, server.Handler(), configure)
			if want := map[string]any{"authenticated": false}; !reflect.DeepEqual(got, want) {
				t.Fatalf("status body = %v, want exactly %v", got, want)
			}
		})
	}
}

// TestAuthStatusReportsEachAuthVia drives every deployment-owned identity
// through the real handler. A bearer is judged alone even from the LAN, as the
// GraphQL route judges it, so the status describes the identity the gate will
// actually authorize.
func TestAuthStatusReportsEachAuthVia(t *testing.T) {
	primary, err := requestsecurity.NewStaticBearerAuthenticator(requestsecurity.StaticBearerConfig{
		Token: testBearerToken, TenantID: "tenant-a", PrincipalID: "fi-fhir-ide-operator", Roles: productionGrant,
	})
	if err != nil {
		t.Fatalf("NewStaticBearerAuthenticator: %v", err)
	}
	pair, err := requestsecurity.NewStaticBearerPairAuthenticator(primary, requestsecurity.StaticBearerConfig{
		Token: serviceToken, TenantID: "tenant-a", PrincipalID: "mentatlab", Roles: []string{operator.ReadRole, "integration:preview"},
	})
	if err != nil {
		t.Fatalf("NewStaticBearerPairAuthenticator: %v", err)
	}
	issuer, err := oidctest.New()
	if err != nil {
		t.Fatal(err)
	}
	defer issuer.Close()
	const audience = "6834f3234782f45980c750ae8711c0a95f9b834c2ccbc909ac47252c060c8e53"
	access, err := requestsecurity.NewCloudflareAccessAuthenticator(issuer.Context(), requestsecurity.CloudflareAccessConfig{
		TeamDomainURL: issuer.IssuerURL(),
		Audience:      audience,
		TenantID:      "tenant-a",
		Principals:    map[string][]string{"operator@example.com": operatorBundle()},
		HTTPClient:    issuer.HTTPClient(),
	})
	if err != nil {
		t.Fatalf("NewCloudflareAccessAuthenticator: %v", err)
	}
	claims := issuer.Claims()
	delete(claims, "roles")
	delete(claims, "tenant_id")
	claims["aud"] = []string{audience}
	claims["email"] = "Operator@Example.com"
	claims["type"] = "app"
	assertion, err := issuer.SignWithType(claims, "RS256", "JWT")
	if err != nil {
		t.Fatalf("sign assertion: %v", err)
	}

	config := secureServerConfig(pair)
	config.TrustedNetworkAuthenticator = trustedNetwork(t, productionGrant...)
	config.CloudflareAccessAuthenticator = access
	server, err := graphqlapi.NewServer(resolvers.NewResolver(), config)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	tests := []struct {
		name         string
		configure    func(*http.Request)
		authVia      string
		principal    string
		operatorRead bool
		clinicalRead bool
	}{
		{name: "network", configure: func(r *http.Request) { r.Header.Set("X-Real-IP", trustedClient) },
			authVia: "network", principal: "fi-fhir-ide-operator", clinicalRead: true},
		{name: "bearer from the LAN is judged alone", configure: func(r *http.Request) {
			r.Header.Set("X-Real-IP", trustedClient)
			r.Header.Set("Authorization", "Bearer "+testBearerToken)
		}, authVia: "bearer", principal: "fi-fhir-ide-operator", clinicalRead: true},
		{name: "service bearer", configure: func(r *http.Request) {
			r.Header.Set("X-Real-IP", untrustedClient)
			r.Header.Set("Authorization", "Bearer "+serviceToken)
		}, authVia: "service-bearer", principal: "mentatlab", operatorRead: true},
		{name: "cloudflare-access", configure: func(r *http.Request) {
			r.Header.Set("X-Real-IP", untrustedClient)
			r.Header.Set(requestsecurity.CloudflareAccessAssertionHeader, assertion)
		}, authVia: "cloudflare-access", principal: "operator@example.com", operatorRead: true, clinicalRead: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getAuthStatus(t, server.Handler(), tt.configure)
			if got["authenticated"] != true || got["authVia"] != tt.authVia || got["principal"] != tt.principal {
				t.Fatalf("identity = %v", got)
			}
			capabilities, _ := got["capabilities"].(map[string]any)
			if capabilities["operatorRead"] != tt.operatorRead || capabilities["clinicalRead"] != tt.clinicalRead {
				t.Fatalf("capabilities = %v", capabilities)
			}
			assertNoSecretMaterial(t, got)
		})
	}
}

// assertNoSecretMaterial fails if the status body carries a credential, the
// tenant, or any key outside the contract.
func assertNoSecretMaterial(t *testing.T, body map[string]any) {
	t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{testBearerToken, serviceToken, "tenant-a", "eyJ"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("status body leaks %q: %s", forbidden, encoded)
		}
	}
	for key := range body {
		switch key {
		case "authenticated", "authVia", "principal", "roles", "capabilities", "missingRoles":
		default:
			t.Fatalf("status body has an uncontracted key %q: %s", key, encoded)
		}
	}
}

// TestAuthStatusStreamingFollowsTheServerFlag: streaming and the accepted
// subscription roots flip with IntegrationSessionStreaming, and the list
// matches what the SSE transport really admits for the same caller.
func TestAuthStatusStreamingFollowsTheServerFlag(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		config := secureServerConfig(testAuthenticator(t))
		config.TrustedNetworkAuthenticator = trustedNetwork(t, operatorBundle()...)
		config.IntegrationSessionStreaming = streaming
		config.IntegrationSessionsConfigured = streaming
		server, err := graphqlapi.NewServer(resolvers.NewResolver(), config)
		if err != nil {
			t.Fatalf("NewServer: %v", err)
		}
		got := getAuthStatus(t, server.Handler(), func(r *http.Request) { r.Header.Set("X-Real-IP", trustedClient) })
		capabilities, _ := got["capabilities"].(map[string]any)
		want := []any{}
		if streaming {
			want = []any{"integrationSessionEvents", "sessionRunEvents"}
		}
		if capabilities["streaming"] != streaming || capabilities["integrationSessions"] != streaming || !reflect.DeepEqual(capabilities["subscriptions"], want) {
			t.Fatalf("streaming=%v: capabilities = %v", streaming, capabilities)
		}
	}
}

// ---- Kill-test: the contract against the real operator service ----
//
// The program's riskiest assumption is that a capability derived from the
// transport identity is enough for the IDE to be honest. For the operator
// plane the real gate lives one layer deeper, in operator.Service.authorize.
// This test puts the real service behind the real handler and asserts, for
// each role set, that the status endpoint's capability is true exactly when a
// request is admitted by both the transport gate and the service — observed as
// the service reaching its store — and false exactly when either refuses.

type unreachableConnector struct{}

var errUnreachableStore = errors.New("kill-test: the operator read store is never queried")

func (unreachableConnector) Connect(context.Context) (driver.Conn, error) {
	return nil, errUnreachableStore
}
func (unreachableConnector) Driver() driver.Driver { return unreachableDriver{} }

type unreachableDriver struct{}

func (unreachableDriver) Open(string) (driver.Conn, error) { return nil, errUnreachableStore }

type countingDeliveries struct{}

func (countingDeliveries) ListDeliveriesForAttempt(context.Context, string, string, int) ([]destination.DeliverySummary, error) {
	return nil, nil
}

type countingRecovery struct{ replays atomic.Int32 }

func (r *countingRecovery) Replay(context.Context, string, string, delivery.Operation) (string, error) {
	r.replays.Add(1)
	return "", delivery.ErrNotDeadLettered
}
func (*countingRecovery) Resubmit(context.Context, string, string, delivery.Operation) (string, error) {
	return "", delivery.ErrNotDeadLettered
}
func (*countingRecovery) Discard(context.Context, string, string, delivery.Operation) (string, error) {
	return "", delivery.ErrNotDeadLettered
}

type countingCatalog struct{ lists, pauses atomic.Int32 }

func (c *countingCatalog) ListSnapshots(context.Context, string, int) ([]lifecycle.Snapshot, error) {
	c.lists.Add(1)
	return nil, nil
}
func (c *countingCatalog) Pause(context.Context, lifecycle.Command) (lifecycle.Snapshot, error) {
	c.pauses.Add(1)
	return lifecycle.Snapshot{}, lifecycle.ErrVersionConflict
}
func (*countingCatalog) Deploy(context.Context, lifecycle.Command) (lifecycle.Snapshot, error) {
	return lifecycle.Snapshot{}, lifecycle.ErrVersionConflict
}
func (*countingCatalog) Resume(context.Context, lifecycle.Command) (lifecycle.Snapshot, error) {
	return lifecycle.Snapshot{}, lifecycle.ErrVersionConflict
}
func (*countingCatalog) Retire(context.Context, lifecycle.Command) (lifecycle.Snapshot, error) {
	return lifecycle.Snapshot{}, lifecycle.ErrVersionConflict
}
func (*countingCatalog) ListEvents(context.Context, string, string, string) ([]lifecycle.EventRecord, error) {
	return nil, nil
}

func TestAuthStatusCapabilitiesAgreeWithTheOperatorService(t *testing.T) {
	db := sql.OpenDB(unreachableConnector{})
	t.Cleanup(func() { _ = db.Close() })
	reads, err := operator.NewPostgresReadStore(db)
	if err != nil {
		t.Fatalf("NewPostgresReadStore: %v", err)
	}

	roleSets := map[string][]string{
		"production grant (transport grant, no control-plane role)": productionGrant,
		"documented operator bundle":                                operatorBundle(),
		"control-plane roles without the transport grant":           append([]string{"integration:preview"}, graphqlapi.OperatorControlPlaneRoles()...),
		"grant plus read role only":                                 {"integration:preview", graphqlapi.GraphQLOperatorRole, operator.ReadRole},
		"read plus delivery":                                        {"integration:preview", operator.ReadRole, delivery.OperatorRole},
		"delivery grant without the read role":                      {"integration:preview", graphqlapi.GraphQLOperatorRole, delivery.OperatorRole},
		"preview only":                                              {"integration:preview"},
	}
	probes := []struct {
		capability string
		document   string
		reached    func(*countingRecovery, *countingCatalog) int32
	}{
		{capability: "operatorRead", document: `query Op { operatorDeployments { version } }`,
			reached: func(_ *countingRecovery, c *countingCatalog) int32 { return c.lists.Load() }},
		{capability: "operatorDelivery", document: `mutation Op { replayDelivery(input: {attemptId: "a-1", reason: "kill test", idempotencyKey: "k-1"}) { kind } }`,
			reached: func(r *countingRecovery, _ *countingCatalog) int32 { return r.replays.Load() }},
		{capability: "operatorDeployment", document: `mutation Op { pauseIntegrationDeployment(input: {definitionId: "d-1", revisionId: "v-1", expectedVersion: 1, reason: "kill test"}) { version } }`,
			reached: func(_ *countingRecovery, c *countingCatalog) int32 { return c.pauses.Load() }},
	}

	for name, roles := range roleSets {
		t.Run(name, func(t *testing.T) {
			recovery := &countingRecovery{}
			catalog := &countingCatalog{}
			service, err := operator.NewService(reads, countingDeliveries{}, recovery, catalog, "tenant-a")
			if err != nil {
				t.Fatalf("operator.NewService: %v", err)
			}
			config := secureServerConfig(testAuthenticator(t))
			config.MaxRequestBodyBytes = 1 << 16
			config.TrustedNetworkAuthenticator = trustedNetwork(t, roles...)
			server, err := graphqlapi.NewServer(resolvers.NewResolver(resolvers.WithOperatorControlPlane(service)), config)
			if err != nil {
				t.Fatalf("NewServer: %v", err)
			}
			handler := server.Handler()
			status := getAuthStatus(t, handler, func(r *http.Request) { r.Header.Set("X-Real-IP", trustedClient) })
			capabilities, _ := status["capabilities"].(map[string]any)
			missing, _ := status["missingRoles"].(map[string]any)

			for _, probe := range probes {
				before := probe.reached(recovery, catalog)
				body := postTrusted(t, handler, config.Path, probe.document)
				reached := probe.reached(recovery, catalog) > before
				forbidden := strings.Contains(body, "GraphQL operation forbidden") ||
					strings.Contains(body, "operator control-plane action forbidden")
				claimed, _ := capabilities[probe.capability].(bool)
				if claimed != reached || claimed == forbidden {
					t.Errorf("%s: status says %v, but the request reached the service=%v forbidden=%v (body %s)",
						probe.capability, claimed, reached, forbidden, body)
				}
				if list, _ := missing[probe.capability].([]any); claimed != (len(list) == 0) {
					t.Errorf("%s: capability %v with missing roles %v", probe.capability, claimed, list)
				}
			}
		})
	}
}

func postTrusted(t *testing.T, handler http.Handler, path, document string) string {
	t.Helper()
	body, err := json.Marshal(map[string]any{"query": document})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Real-IP", trustedClient)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	return recorder.Body.String()
}

// TestAuthStatusDocumentedExampleMatches keeps the contract example in
// docs/planning/GRAPHQL-API.md honest: it must decode to exactly the keys the
// handler emits.
func TestAuthStatusDocumentedExampleMatches(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "docs", "planning", "GRAPHQL-API.md"))
	if err != nil {
		t.Fatalf("read GRAPHQL-API.md: %v", err)
	}
	const begin, end = "<!-- auth-status-example -->\n```json\n", "```\n<!-- /auth-status-example -->"
	start := bytes.Index(raw, []byte(begin))
	stop := bytes.Index(raw, []byte(end))
	if start < 0 || stop < start {
		t.Fatal("GRAPHQL-API.md has no marked /api/auth/status example")
	}
	var documented map[string]any
	if err := json.Unmarshal(raw[start+len(begin):stop], &documented); err != nil {
		t.Fatalf("documented example is not JSON: %v", err)
	}

	config := secureServerConfig(testAuthenticator(t))
	config.TrustedNetworkAuthenticator = trustedNetwork(t, productionGrant...)
	config.LLMConfigured = true
	server, err := graphqlapi.NewServer(resolvers.NewResolver(), config)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	served := getAuthStatus(t, server.Handler(), func(r *http.Request) { r.Header.Set("X-Real-IP", trustedClient) })
	if !reflect.DeepEqual(documented, served) {
		servedJSON, _ := json.Marshal(served)
		t.Fatalf("the documented example drifted from the handler; served %s", servedJSON)
	}
}
