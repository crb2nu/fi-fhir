package graphql_test

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	graphqlapi "gitlab.flexinfer.ai/libs/fi-fhir/internal/api/graphql"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/graphql/resolvers"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/connection"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/delivery"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/operator"
)

// The .loom/38 C-0 connection catalog at the GraphQL boundary: the transport
// gate's role decisions, /api/auth/status's claims about them checked against
// the real catalog service, and the wire shape of the fields that need no
// database.

func connectionReadOperations() []controlPlaneOperation {
	return []controlPlaneOperation{
		{field: "connections", document: `query Op { connections { id } }`},
		{field: "connection", document: `query Op { connection(id: "c-1") { id } }`},
		{field: "connectionRevisions", document: `query Op { connectionRevisions(id: "c-1") { digest } }`},
		{field: "connectionRevision", document: `query Op { connectionRevision(artifactId: "c-1", revisionId: "1") { digest } }`},
		{field: "engineRuntime", document: `query Op { engineRuntime { version } }`},
	}
}

func connectionWriteOperations() []controlPlaneOperation {
	const command = `(input: {id: "c-1", expectedVersion: 1, reason: "gate"})`
	return []controlPlaneOperation{
		{field: "createConnection", document: `mutation Op { createConnection(input: {id: "c-1", direction: DESTINATION, kind: KAFKA, name: "gate", spec: {}, reason: "gate"}) { id } }`},
		{field: "updateConnection", document: `mutation Op { updateConnection(input: {id: "c-1", expectedVersion: 1, name: "gate", reason: "gate"}) { id } }`},
		{field: "archiveConnection", document: `mutation Op { archiveConnection` + command + ` { id } }`},
		{field: "compileConnection", document: `mutation Op { compileConnection` + command + ` { problems { code } } }`},
		{field: "validateConnectionSpec", document: `mutation Op { validateConnectionSpec(input: {kind: KAFKA, spec: {}}) { code } }`},
	}
}

func TestTransportGate_ConnectionCatalogRoles(t *testing.T) {
	handler, path, issuer := newTransportGateHandler(t)

	t.Run("integration.operator reaches every read and no write", func(t *testing.T) {
		token := transportGateToken(t, issuer, "operator-read-only", operator.ReadRole)
		for _, operation := range connectionReadOperations() {
			assertGatePassed(t, postTransportGate(t, handler, path, token, operation.document), operation.field)
		}
		for _, operation := range connectionWriteOperations() {
			assertGateRefused(t, postTransportGate(t, handler, path, token, operation.document), operation.field)
		}
	})
	t.Run("adding integration.deployment.operator reaches every write", func(t *testing.T) {
		token := transportGateToken(t, issuer, "operator-deployment", operator.ReadRole, operator.DeploymentOperatorRole)
		for _, operation := range append(connectionReadOperations(), connectionWriteOperations()...) {
			assertGatePassed(t, postTransportGate(t, handler, path, token, operation.document), operation.field)
		}
	})
	t.Run("the deployment grant without the read role reaches nothing", func(t *testing.T) {
		token := transportGateToken(t, issuer, "deployment-only", operator.DeploymentOperatorRole, delivery.OperatorRole)
		for _, operation := range append(connectionReadOperations(), connectionWriteOperations()...) {
			assertGateRefused(t, postTransportGate(t, handler, path, token, operation.document), operation.field)
		}
	})
	t.Run("the compatibility grant clears the gate as it does for every field", func(t *testing.T) {
		token := transportGateToken(t, issuer, "legacy-operator", graphqlapi.GraphQLOperatorRole)
		for _, operation := range append(connectionReadOperations(), connectionWriteOperations()...) {
			assertGatePassed(t, postTransportGate(t, handler, path, token, operation.document), operation.field)
		}
	})
}

// TestAuthStatusConnectionCapabilitiesAgreeWithTheCatalogService is the R-A
// kill-test shape applied to the catalog: for each role set, connectionsRead
// and connectionsWrite are true exactly when a request clears both the
// transport gate and connection.Service — observed as the service reaching its
// store, which here fails every query — and false exactly when either refuses.
func TestAuthStatusConnectionCapabilitiesAgreeWithTheCatalogService(t *testing.T) {
	db := sql.OpenDB(unreachableConnector{})
	t.Cleanup(func() { _ = db.Close() })
	store, err := connection.NewPostgresStore(db, nil)
	if err != nil {
		t.Fatalf("NewPostgresStore: %v", err)
	}
	service, err := connection.NewService(store, nil, nil, "tenant-a")
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	// The store behind this service fails every query, so a request that
	// cleared both gates ends in the catalog's generic failure.
	const reachedStore = "connection catalog request failed"
	probes := []struct {
		capability string
		document   string
	}{
		{"connectionsRead", `query Op { connections { id } }`},
		{"connectionsWrite", `mutation Op { createConnection(input: {id: "kill-test", direction: DESTINATION, kind: KAFKA, name: "kill test", spec: {}, reason: "kill test"}) { id } }`},
	}
	roleSets := map[string][]string{
		"production grant (transport grant, no control-plane role)": productionGrant,
		"documented operator bundle":                                operatorBundle(),
		"control-plane roles without the transport grant":           append([]string{"integration:preview"}, graphqlapi.OperatorControlPlaneRoles()...),
		"grant plus read role only":                                 {"integration:preview", graphqlapi.GraphQLOperatorRole, operator.ReadRole},
		"read plus deployment":                                      {"integration:preview", operator.ReadRole, operator.DeploymentOperatorRole},
		"deployment grant without the read role":                    {"integration:preview", graphqlapi.GraphQLOperatorRole, operator.DeploymentOperatorRole},
		"preview only":                                              {"integration:preview"},
	}
	for name, roles := range roleSets {
		t.Run(name, func(t *testing.T) {
			config := secureServerConfig(testAuthenticator(t))
			config.MaxRequestBodyBytes = 1 << 16
			config.TrustedNetworkAuthenticator = trustedNetwork(t, roles...)
			server, err := graphqlapi.NewServer(resolvers.NewResolver(resolvers.WithConnectionCatalog(service)), config)
			if err != nil {
				t.Fatalf("NewServer: %v", err)
			}
			handler := server.Handler()
			status := getAuthStatus(t, handler, func(r *http.Request) { r.Header.Set("X-Real-IP", trustedClient) })
			capabilities, _ := status["capabilities"].(map[string]any)
			missing, _ := status["missingRoles"].(map[string]any)
			for _, probe := range probes {
				body := postTrusted(t, handler, config.Path, probe.document)
				reached := strings.Contains(body, reachedStore)
				forbidden := strings.Contains(body, "GraphQL operation forbidden") ||
					strings.Contains(body, "connection catalog action forbidden")
				claimed, _ := capabilities[probe.capability].(bool)
				if claimed != reached || claimed == forbidden {
					t.Errorf("%s: status says %v, but the request reached the store=%v forbidden=%v (body %s)",
						probe.capability, claimed, reached, forbidden, body)
				}
				if list, _ := missing[probe.capability].([]any); claimed != (len(list) == 0) {
					t.Errorf("%s: capability %v with missing roles %v", probe.capability, claimed, list)
				}
			}
		})
	}
}

// catalogServer is a trusted-network server with the full operator bundle and
// the given resolver options.
func catalogServer(t *testing.T, options ...resolvers.ResolverOption) (http.Handler, string) {
	t.Helper()
	config := secureServerConfig(testAuthenticator(t))
	config.MaxRequestBodyBytes = 1 << 16
	config.TrustedNetworkAuthenticator = trustedNetwork(t, operatorBundle()...)
	server, err := graphqlapi.NewServer(resolvers.NewResolver(options...), config)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return server.Handler(), config.Path
}

func unreachableCatalog(t *testing.T) *connection.Service {
	t.Helper()
	db := sql.OpenDB(unreachableConnector{})
	t.Cleanup(func() { _ = db.Close() })
	store, err := connection.NewPostgresStore(db, nil)
	if err != nil {
		t.Fatalf("NewPostgresStore: %v", err)
	}
	service, err := connection.NewService(store, nil, nil, "tenant-a")
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return service
}

type graphQLResponse struct {
	Data   map[string]json.RawMessage `json:"data"`
	Errors []struct {
		Message    string         `json:"message"`
		Extensions map[string]any `json:"extensions"`
	} `json:"errors"`
}

func decodeGraphQL(t *testing.T, body string) graphQLResponse {
	t.Helper()
	var response graphQLResponse
	if err := json.Unmarshal([]byte(body), &response); err != nil {
		t.Fatalf("decode GraphQL response: %v (%s)", err, body)
	}
	return response
}

func TestConnectionCatalogValidateSpecOverTheWire(t *testing.T) {
	handler, path := catalogServer(t, resolvers.WithConnectionCatalog(unreachableCatalog(t)))
	body := postTrusted(t, handler, path, `mutation Op { validateConnectionSpec(input: {
		kind: HTTPS,
		spec: {destination_id: "d", class: "production", colour: "blue",
		       https: {url: "http://insecure.example", method: "POST", token_binding: "t", token: "synthetic"}},
		secretBindings: [{name: "t", provider: "file", key: "destinations/t"}]
	}) { code path message } }`)
	response := decodeGraphQL(t, body)
	if len(response.Errors) != 0 {
		t.Fatalf("validate returned errors: %s", body)
	}
	var problems []connection.Problem
	if err := json.Unmarshal(response.Data["validateConnectionSpec"], &problems); err != nil {
		t.Fatalf("decode problems: %v", err)
	}
	want := map[string]string{
		"colour":      connection.CodeUnknownField,
		"https.url":   connection.CodeInvalidURL,
		"https.token": connection.CodeSecretValueForbidden,
	}
	for path, code := range want {
		found := false
		for _, problem := range problems {
			found = found || (problem.Path == path && problem.Code == code)
		}
		if !found {
			t.Errorf("no %s at %s in %+v", code, path, problems)
		}
	}
	if strings.Contains(body, "synthetic") {
		t.Fatal("the response echoes the refused value")
	}
}

// TestConnectionCatalogSecretValueRejectionOverTheWire: the write is refused
// before the store, with the contracted message and exactly the contracted
// extensions — the code and the refused paths — and never the value.
func TestConnectionCatalogSecretValueRejectionOverTheWire(t *testing.T) {
	handler, path := catalogServer(t, resolvers.WithConnectionCatalog(unreachableCatalog(t)))
	body := postTrusted(t, handler, path, `mutation Op { createConnection(input: {
		id: "leaky", direction: DESTINATION, kind: HTTPS, name: "leaky", reason: "paste",
		spec: {destination_id: "d", https: {url: "https://x.example", method: "POST", token_binding: "t", token: "synthetic-token"}}
	}) { id } }`)
	response := decodeGraphQL(t, body)
	if len(response.Errors) != 1 || response.Errors[0].Message != graphqlapi.ConnectionSpecRejectedMessage {
		t.Fatalf("response = %s", body)
	}
	extensions := response.Errors[0].Extensions
	if extensions["code"] != connection.CodeSecretValueForbidden || len(extensions) != 2 {
		t.Fatalf("extensions = %v", extensions)
	}
	problems, _ := extensions["problems"].([]any)
	if len(problems) != 1 {
		t.Fatalf("problems = %v", extensions["problems"])
	}
	problem, _ := problems[0].(map[string]any)
	if problem["path"] != "https.token" || problem["code"] != connection.CodeSecretValueForbidden {
		t.Fatalf("problem = %v", problem)
	}
	if strings.Contains(body, "synthetic-token") || strings.Contains(body, "connection catalog request failed") {
		t.Fatalf("the refusal echoed the value or reached the store: %s", body)
	}
}

// TestConnectionCatalogUnknownKeyRejectionOverTheWire: a key the kind does not
// define is refused at write through the same path — nothing is silently
// dropped — with UNKNOWN_FIELD and the key's path in extensions.problems.
func TestConnectionCatalogUnknownKeyRejectionOverTheWire(t *testing.T) {
	handler, path := catalogServer(t, resolvers.WithConnectionCatalog(unreachableCatalog(t)))
	body := postTrusted(t, handler, path, `mutation Op { createConnection(input: {
		id: "typo", direction: DESTINATION, kind: KAFKA, name: "typo", reason: "a key kafka does not have",
		spec: {destination_id: "d", kafka: {topic: "t", partitions: 3}}
	}) { id } }`)
	response := decodeGraphQL(t, body)
	if len(response.Errors) != 1 || response.Errors[0].Message != graphqlapi.ConnectionSpecRejectedMessage {
		t.Fatalf("response = %s", body)
	}
	extensions := response.Errors[0].Extensions
	problems, _ := extensions["problems"].([]any)
	if extensions["code"] != connection.CodeUnknownField || len(problems) != 1 {
		t.Fatalf("extensions = %v", extensions)
	}
	problem, _ := problems[0].(map[string]any)
	if problem["path"] != "kafka.partitions" || problem["code"] != connection.CodeUnknownField {
		t.Fatalf("problem = %v", problem)
	}
	if strings.Contains(body, "connection catalog request failed") {
		t.Fatalf("the refusal reached the store: %s", body)
	}
}

func TestConnectionCatalogFailsClosedWhenNotConfigured(t *testing.T) {
	handler, path := catalogServer(t)
	for _, operation := range append(connectionReadOperations(), connectionWriteOperations()...) {
		body := postTrusted(t, handler, path, operation.document)
		want := "connection catalog unavailable"
		if operation.field == "engineRuntime" {
			want = "engine runtime unavailable"
		}
		if !strings.Contains(body, want) {
			t.Errorf("%s without a catalog: %s", operation.field, body)
		}
	}
}

func TestEngineRuntimeOverTheWire(t *testing.T) {
	description := &connection.RuntimeDescription{
		Version: "v-test", TenantID: "tenant-a", ReplicaID: "fi-fhir-0-42", AuthMode: "static",
		TrustedNetwork: true, ControlPlane: true,
		Registry: connection.RuntimeRegistry{IntegrationCount: 1, Integrations: []connection.RuntimeRegistryIntegration{{
			IntegrationID: "adt-east", DefinitionID: "integration-adt", RevisionID: "definition-revision-1",
			Digest: "sha256:" + strings.Repeat("d", 64), SourceID: "adt-east", Format: "hl7v2",
		}}},
		Ledgers: []connection.RuntimeLedger{{Name: "connection", Version: connection.SchemaVersion}},
		Properties: []connection.RuntimeProperty{
			connection.NewRuntimeProperty("FI_FHIR_GRAPHQL_BEARER_TOKEN", true, "synthetic-bearer-value", true, ""),
			connection.NewRuntimeProperty("FI_FHIR_GRAPHQL_AUTH_MODE", false, "", false, "static"),
		},
	}
	for index, kind := range connection.AdapterOrder {
		description.Adapters[index] = connection.RuntimeAdapter{Kind: kind}
	}
	if err := description.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	handler, path := catalogServer(t, resolvers.WithEngineRuntime(description))
	body := postTrusted(t, handler, path, `query Op { engineRuntime {
		version tenantId replicaId authMode trustedNetwork controlPlane llmConfigured
		registry { integrationCount integrations { integrationId digest } }
		adapters { kind enabled sourceDigest pollSeconds requireClientIdentity }
		destinationIdentity { mode }
		ledgers { name version }
		properties { key value secret source }
	} }`)
	response := decodeGraphQL(t, body)
	if len(response.Errors) != 0 {
		t.Fatalf("engineRuntime errors: %s", body)
	}
	var runtime struct {
		ReplicaID string `json:"replicaId"`
		Adapters  []struct {
			Kind                  string `json:"kind"`
			Enabled               bool   `json:"enabled"`
			SourceDigest          *string
			PollSeconds           *int
			RequireClientIdentity *bool
		} `json:"adapters"`
		DestinationIdentity *struct{ Mode string } `json:"destinationIdentity"`
		Properties          []struct {
			Key, Value, Source string
			Secret             bool
		} `json:"properties"`
	}
	if err := json.Unmarshal(response.Data["engineRuntime"], &runtime); err != nil {
		t.Fatalf("decode engineRuntime: %v", err)
	}
	if runtime.ReplicaID != "fi-fhir-0-42" || len(runtime.Adapters) != 4 || runtime.DestinationIdentity != nil {
		t.Fatalf("runtime = %+v", runtime)
	}
	for index, adapter := range runtime.Adapters {
		if adapter.Kind != connection.AdapterOrder[index] || adapter.Enabled ||
			adapter.SourceDigest != nil || adapter.PollSeconds != nil || adapter.RequireClientIdentity != nil {
			t.Fatalf("a disabled adapter reports a value: %+v", adapter)
		}
	}
	if strings.Contains(body, "synthetic-bearer-value") {
		t.Fatal("a secret property's value reached the response")
	}
	if runtime.Properties[0].Value != "set" || !runtime.Properties[0].Secret || runtime.Properties[1].Value != "static" ||
		runtime.Properties[1].Source != "default" {
		t.Fatalf("properties = %+v", runtime.Properties)
	}

	// The resolver re-checks the read role behind the transport gate: the
	// compatibility grant alone clears the gate and is refused here.
	config := secureServerConfig(testAuthenticator(t))
	config.MaxRequestBodyBytes = 1 << 16
	config.TrustedNetworkAuthenticator = trustedNetwork(t, productionGrant...)
	server, err := graphqlapi.NewServer(resolvers.NewResolver(resolvers.WithEngineRuntime(description)), config)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	if body := postTrusted(t, server.Handler(), config.Path, `query Op { engineRuntime { version } }`); !strings.Contains(body, "connection catalog action forbidden") {
		t.Fatalf("engineRuntime without %s: %s", operator.ReadRole, body)
	}
}
