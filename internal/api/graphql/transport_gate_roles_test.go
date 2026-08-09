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
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/requestsecurity/oidctest"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/delivery"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/operator"
	enginesession "gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/session"
)

// controlPlaneOperation is one root field expressed as a complete,
// schema-valid document. gqlgen validates a request before the operation-context
// mutator runs, so a document that does not type-check never reaches the
// transport gate and would silently pass a refusal assertion. Every document
// here is therefore a real query against the real schema.
type controlPlaneOperation struct {
	field    string
	document string
}

func operatorReadOperations() []controlPlaneOperation {
	return []controlPlaneOperation{
		{field: "operatorReceipts", document: `query Op { operatorReceipts { pageInfo { hasNextPage } } }`},
		{field: "operatorMessageTrace", document: `query Op { operatorMessageTrace(receiptId: "r-1") { receipt { receiptId } } }`},
		{field: "operatorDeliveryAttempts", document: `query Op { operatorDeliveryAttempts { pageInfo { hasNextPage } } }`},
		{field: "operatorDeliveryAttempt", document: `query Op { operatorDeliveryAttempt(attemptId: "a-1") { attemptId } }`},
		{field: "operatorDeadLetters", document: `query Op { operatorDeadLetters { pageInfo { hasNextPage } } }`},
		{field: "operatorCircuits", document: `query Op { operatorCircuits { state } }`},
		{field: "operatorAttemptAudit", document: `query Op { operatorAttemptAudit(attemptId: "a-1") { pageInfo { hasNextPage } } }`},
		{field: "operatorDeployments", document: `query Op { operatorDeployments { version } }`},
		{field: "operatorDeploymentEvents", document: `query Op { operatorDeploymentEvents(definitionId: "d-1", revisionId: "v-1") { eventId } }`},
	}
}

func operatorRecoveryOperations() []controlPlaneOperation {
	const input = `(input: {attemptId: "a-1", reason: "kill test", idempotencyKey: "k-1"})`
	return []controlPlaneOperation{
		{field: "replayDelivery", document: `mutation Op { replayDelivery` + input + ` { kind } }`},
		{field: "resubmitMessage", document: `mutation Op { resubmitMessage` + input + ` { kind } }`},
		{field: "discardDeadLetter", document: `mutation Op { discardDeadLetter` + input + ` { kind } }`},
	}
}

func operatorDeploymentOperations() []controlPlaneOperation {
	const input = `(input: {definitionId: "d-1", revisionId: "v-1", expectedVersion: 1, reason: "kill test"})`
	return []controlPlaneOperation{
		{field: "pauseIntegrationDeployment", document: `mutation Op { pauseIntegrationDeployment` + input + ` { version } }`},
		{field: "resumeIntegrationDeployment", document: `mutation Op { resumeIntegrationDeployment` + input + ` { version } }`},
		{field: "retireIntegrationDeployment", document: `mutation Op { retireIntegrationDeployment` + input + ` { version } }`},
		{field: "deployIntegrationRelease", document: `mutation Op { deployIntegrationRelease` + input + ` { version } }`},
	}
}

// compatibilityBucketOperations samples the 115 root fields that no shipped
// fine-grained role describes. They must stay reachable under the compatibility
// grant and unreachable without it.
func compatibilityBucketOperations() []controlPlaneOperation {
	return []controlPlaneOperation{
		{field: "events", document: `query Op { events(first: 1) { totalCount } }`},
		{field: "health", document: `query Op { health { status } }`},
		{field: "workflowDefinitions", document: `query Op { workflowDefinitions { id } }`},
		{field: "integrationSessions", document: `query Op { integrationSessions { id } }`},
		{field: "createIntegrationSession", document: `mutation Op { createIntegrationSession(input: {name: "gate"}) { id } }`},
		{field: "exportIntegrationBundle", document: `mutation Op { exportIntegrationBundle(input: {sessionId: "s-1", reason: "gate"}) { sessionId } }`},
	}
}

func allControlPlaneOperations() []controlPlaneOperation {
	operations := operatorReadOperations()
	operations = append(operations, operatorRecoveryOperations()...)
	return append(operations, operatorDeploymentOperations()...)
}

// TestTransportGate_FineGrainedRolesReplaceBlanketOperator is Lane S4-E's
// kill-test (.loom/32-sprint4-execution-specs.md:467). It drives the real
// GraphQL handler with real 4.1a OIDC tokens, one case per role combination in
// the lane's acceptance criteria.
//
// Its first case is the day-1 gate inverted. Run verbatim against unmodified
// main at 55412bdaa — asserting refusal rather than admission — it PASSED
// 16/16, proving the fine-grained roles 4.2a ships were decorative at the
// transport gate and that only the blanket role opened it. That refusal is what
// this slice removes, so the same case now asserts the operations are reached.
//
// "Reached" means the transport gate let the request through to the resolver.
// The resolvers here have no control plane configured, so they answer "operator
// control plane unavailable" — a service-layer refusal, which is exactly the
// layering this slice preserves. The assertions therefore test for the gate's
// own error, never for a successful resolve.
//
// Negative control: `go test -tags transportgateblanket` restores the
// pre-Sprint-4 blanket allow and every refusal case below must fail open.
func TestTransportGate_FineGrainedRolesReplaceBlanketOperator(t *testing.T) {
	handler, path, issuer := newTransportGateHandler(t)

	t.Run("least privilege reaches the 4.2a control plane", func(t *testing.T) {
		token := transportGateToken(t, issuer, "operator-full",
			operator.ReadRole, operator.DeploymentOperatorRole, delivery.OperatorRole)
		for _, operation := range allControlPlaneOperations() {
			t.Run(operation.field, func(t *testing.T) {
				assertGatePassed(t, postTransportGate(t, handler, path, token, operation.document), operation.field)
			})
		}
	})

	t.Run("integration.operator alone reaches every read and no mutation", func(t *testing.T) {
		token := transportGateToken(t, issuer, "operator-read-only", operator.ReadRole)
		for _, operation := range operatorReadOperations() {
			t.Run("read/"+operation.field, func(t *testing.T) {
				assertGatePassed(t, postTransportGate(t, handler, path, token, operation.document), operation.field)
			})
		}
		mutations := append(operatorRecoveryOperations(), operatorDeploymentOperations()...)
		for _, operation := range mutations {
			t.Run("mutation/"+operation.field, func(t *testing.T) {
				assertGateRefused(t, postTransportGate(t, handler, path, token, operation.document), operation.field)
			})
		}
	})

	t.Run("adding integration.delivery.operator reaches recovery and no deployment command", func(t *testing.T) {
		token := transportGateToken(t, issuer, "operator-delivery", operator.ReadRole, delivery.OperatorRole)
		for _, operation := range operatorRecoveryOperations() {
			t.Run("recovery/"+operation.field, func(t *testing.T) {
				assertGatePassed(t, postTransportGate(t, handler, path, token, operation.document), operation.field)
			})
		}
		for _, operation := range operatorDeploymentOperations() {
			t.Run("deployment/"+operation.field, func(t *testing.T) {
				assertGateRefused(t, postTransportGate(t, handler, path, token, operation.document), operation.field)
			})
		}
	})

	t.Run("adding integration.deployment.operator reaches deployment and no recovery", func(t *testing.T) {
		token := transportGateToken(t, issuer, "operator-deployment", operator.ReadRole, operator.DeploymentOperatorRole)
		for _, operation := range operatorDeploymentOperations() {
			t.Run("deployment/"+operation.field, func(t *testing.T) {
				assertGatePassed(t, postTransportGate(t, handler, path, token, operation.document), operation.field)
			})
		}
		for _, operation := range operatorRecoveryOperations() {
			t.Run("recovery/"+operation.field, func(t *testing.T) {
				assertGateRefused(t, postTransportGate(t, handler, path, token, operation.document), operation.field)
			})
		}
	})

	t.Run("delivery operator without the read role reaches nothing", func(t *testing.T) {
		// operator.Service.authorize requires the read role alongside the
		// delivery grant, so the gate must not admit the delivery grant alone.
		token := transportGateToken(t, issuer, "delivery-only", delivery.OperatorRole)
		for _, operation := range allControlPlaneOperations() {
			t.Run(operation.field, func(t *testing.T) {
				assertGateRefused(t, postTransportGate(t, handler, path, token, operation.document), operation.field)
			})
		}
	})

	t.Run("integration.phi.export alone reaches nothing", func(t *testing.T) {
		token := transportGateToken(t, issuer, "phi-export-only", enginesession.PHIExportRole)
		operations := append(allControlPlaneOperations(), compatibilityBucketOperations()...)
		for _, operation := range operations {
			t.Run(operation.field, func(t *testing.T) {
				assertGateRefused(t, postTransportGate(t, handler, path, token, operation.document), operation.field)
			})
		}
	})

	t.Run("the compatibility bucket needs the compatibility grant", func(t *testing.T) {
		narrow := transportGateToken(t, issuer, "operator-narrow",
			operator.ReadRole, operator.DeploymentOperatorRole, delivery.OperatorRole)
		for _, operation := range compatibilityBucketOperations() {
			t.Run("narrow/"+operation.field, func(t *testing.T) {
				assertGateRefused(t, postTransportGate(t, handler, path, narrow, operation.document), operation.field)
			})
		}
	})

	t.Run("__typename alone cannot carry an operation", func(t *testing.T) {
		// __typename is skipped when it rides along with real root fields, so a
		// bare selection of it must not become a hole that admits any
		// authenticated caller to a gate that refuses them everything else.
		token := transportGateToken(t, issuer, "operator-read-only", operator.ReadRole)
		for _, document := range []string{
			`query Op { __typename }`,
			`mutation Op { __typename }`,
		} {
			assertGateRefused(t, postTransportGate(t, handler, path, token, document), document)
		}
		// Riding along with an authorized field stays authorized.
		assertGatePassed(t, postTransportGate(t, handler, path, token,
			`query Op { __typename operatorCircuits { state } }`), "__typename+operatorCircuits")
	})

	t.Run("an unmapped role reaches nothing", func(t *testing.T) {
		token := transportGateToken(t, issuer, "author-only", "author")
		operations := append(allControlPlaneOperations(), compatibilityBucketOperations()...)
		for _, operation := range operations {
			t.Run(operation.field, func(t *testing.T) {
				assertGateRefused(t, postTransportGate(t, handler, path, token, operation.document), operation.field)
			})
		}
	})
}

// TestTransportGate_CompatibilityGrantBehavesExactlyAsBefore holds correction
// 34: five documents mint operator tokens carrying graphql:operator and nothing
// else, so the grant must still open the whole surface. If this fails, every
// documented operator deployment has lost its control plane.
func TestTransportGate_CompatibilityGrantBehavesExactlyAsBefore(t *testing.T) {
	handler, path, issuer := newTransportGateHandler(t)
	token := transportGateToken(t, issuer, "legacy-operator", graphqlapi.GraphQLOperatorRole)

	operations := append(allControlPlaneOperations(), compatibilityBucketOperations()...)
	for _, operation := range operations {
		t.Run(operation.field, func(t *testing.T) {
			assertGatePassed(t, postTransportGate(t, handler, path, token, operation.document), operation.field)
		})
	}

	t.Run("introspection", func(t *testing.T) {
		assertGatePassed(t, postTransportGate(t, handler, path, token,
			`query Introspect { __schema { queryType { name } } }`), "__schema")
	})
}

// TestTransportGate_PreviewRoleIsUnchanged pins the integration:preview
// allowlist across the narrowing. Task 3 of the lane requires it byte-for-byte,
// and the check matters because the preview branch moved from a negated guard
// to an early return.
func TestTransportGate_PreviewRoleIsUnchanged(t *testing.T) {
	handler, path, issuer := newTransportGateHandler(t)
	token := transportGateToken(t, issuer, "preview-only", "integration:preview")

	t.Run("health is allowed", func(t *testing.T) {
		assertGatePassed(t, postTransportGate(t, handler, path, token, `query Op { health { status } }`), "health")
	})
	t.Run("preview mutation is allowed", func(t *testing.T) {
		const document = `mutation Op { previewIntegrationMessage(input: {integrationId: "adt-east", data: "MSH|^~\\&|TEST", correlationId: "c-1", reason: "gate"}) { mode } }`
		assertGatePassed(t, postTransportGate(t, handler, path, token, document), "previewIntegrationMessage")
	})
	for _, operation := range allControlPlaneOperations() {
		t.Run("refused/"+operation.field, func(t *testing.T) {
			assertGateRefused(t, postTransportGate(t, handler, path, token, operation.document), operation.field)
		})
	}
	t.Run("refused/events", func(t *testing.T) {
		assertGateRefused(t, postTransportGate(t, handler, path, token,
			`query Op { events(first: 1) { totalCount } }`), "events")
	})
}

func newTransportGateHandler(t *testing.T) (http.Handler, string, *oidctest.Fixture) {
	t.Helper()
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
	config.MaxRequestBodyBytes = 1 << 16
	server, err := graphqlapi.NewServer(resolvers.NewResolver(), config)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return server.Handler(), config.Path, issuer
}

func transportGateToken(t *testing.T, issuer *oidctest.Fixture, subject string, roles ...string) string {
	t.Helper()
	claims := issuer.Claims()
	claims["sub"] = subject
	claims["roles"] = roles
	token, err := issuer.Sign(claims, "RS256")
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return token
}

func postTransportGate(t *testing.T, handler http.Handler, path, token, document string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(map[string]any{"query": document})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Origin", "https://ide.example.test")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

// transportGateRefusal is the exact error the operation-authorization extension
// emits. Matching on it rather than on FORBIDDEN alone keeps a service-layer or
// resolver-layer refusal from being mistaken for a transport-gate refusal.
const transportGateRefusal = "GraphQL operation forbidden"

func assertGateRefused(t *testing.T, response *httptest.ResponseRecorder, field string) {
	t.Helper()
	body := response.Body.String()
	if response.Code != http.StatusOK {
		t.Fatalf("%s: status = %d, want 200 GraphQL rejection, body=%s", field, response.Code, body)
	}
	if !strings.Contains(body, `"code":"FORBIDDEN"`) || !strings.Contains(body, transportGateRefusal) {
		t.Fatalf("%s: transport gate admitted a caller it should refuse: %s", field, body)
	}
}

func assertGatePassed(t *testing.T, response *httptest.ResponseRecorder, field string) {
	t.Helper()
	body := response.Body.String()
	if response.Code != http.StatusOK {
		t.Fatalf("%s: status = %d, want 200, body=%s", field, response.Code, body)
	}
	if strings.Contains(body, transportGateRefusal) {
		t.Fatalf("%s: transport gate refused a caller holding the mapped roles: %s", field, body)
	}
	if strings.Contains(body, "GRAPHQL_VALIDATION_FAILED") {
		t.Fatalf("%s: document never reached the transport gate: %s", field, body)
	}
}
