package graphql_test

import (
	"strings"
	"testing"

	graphqlapi "gitlab.flexinfer.ai/libs/fi-fhir/internal/api/graphql"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/delivery"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/operator"
)

// The .loom/42 E-1 definition authoring surface at the GraphQL boundary: the
// transport gate's role decisions and the fail-closed wire shape when serve
// did not compose the service. The service itself is proven against
// PostgreSQL in internal/integration/lifecycle/authoring.

func definitionReadOperations() []controlPlaneOperation {
	return []controlPlaneOperation{
		{field: "integrationDefinitions", document: `query Op { integrationDefinitions { definitionId } }`},
		{field: "integrationDefinition", document: `query Op { integrationDefinition(definitionId: "d", revisionId: "v1") { realValidationAvailable } }`},
		{field: "integrationRegistryArtifacts", document: `query Op { integrationRegistryArtifacts { integrationId } }`},
	}
}

func definitionWriteOperations() []controlPlaneOperation {
	const draft = `{definitionId: "d", revisionId: "v1", source: {artifactId: "s", revisionId: "1"},
		destinations: [], profile: {artifactId: "p", revisionId: "1", digest: "x"},
		workflow: {artifactId: "w", revisionId: "1", digest: "x"}, secretBindings: []}`
	const command = `(input: {definitionId: "d", revisionId: "v1", expectedVersion: 1, reason: "gate"})`
	return []controlPlaneOperation{
		{field: "validateIntegrationDefinitionDraft", document: `mutation Op { validateIntegrationDefinitionDraft(input: ` + draft + `) { code } }`},
		{field: "createIntegrationDefinitionDraft", document: `mutation Op { createIntegrationDefinitionDraft(input: ` + draft + `, reason: "gate") { problems { code } } }`},
		{field: "validateIntegrationDefinition", document: `mutation Op { validateIntegrationDefinition(input: {definitionId: "d", revisionId: "v1", expectedVersion: 1, mode: STATIC, reason: "gate"}) { realValidationAvailable } }`},
		{field: "approveIntegrationDefinition", document: `mutation Op { approveIntegrationDefinition` + command + ` { realValidationAvailable } }`},
		{field: "publishIntegrationDefinition", document: `mutation Op { publishIntegrationDefinition` + command + ` { realValidationAvailable } }`},
	}
}

func TestTransportGate_DefinitionAuthoringRoles(t *testing.T) {
	handler, path, issuer := newTransportGateHandler(t)
	t.Run("integration.operator reaches every read and no write", func(t *testing.T) {
		token := transportGateToken(t, issuer, "operator-read-only", operator.ReadRole)
		for _, operation := range definitionReadOperations() {
			assertGatePassed(t, postTransportGate(t, handler, path, token, operation.document), operation.field)
		}
		for _, operation := range definitionWriteOperations() {
			assertGateRefused(t, postTransportGate(t, handler, path, token, operation.document), operation.field)
		}
	})
	t.Run("adding integration.deployment.operator reaches every write", func(t *testing.T) {
		token := transportGateToken(t, issuer, "operator-deployment", operator.ReadRole, operator.DeploymentOperatorRole)
		for _, operation := range append(definitionReadOperations(), definitionWriteOperations()...) {
			assertGatePassed(t, postTransportGate(t, handler, path, token, operation.document), operation.field)
		}
	})
	t.Run("the deployment grant without the read role reaches nothing", func(t *testing.T) {
		token := transportGateToken(t, issuer, "deployment-only", operator.DeploymentOperatorRole, delivery.OperatorRole)
		for _, operation := range append(definitionReadOperations(), definitionWriteOperations()...) {
			assertGateRefused(t, postTransportGate(t, handler, path, token, operation.document), operation.field)
		}
	})
	t.Run("the compatibility grant clears the gate as it does for every field", func(t *testing.T) {
		token := transportGateToken(t, issuer, "legacy-operator", graphqlapi.GraphQLOperatorRole)
		for _, operation := range append(definitionReadOperations(), definitionWriteOperations()...) {
			assertGatePassed(t, postTransportGate(t, handler, path, token, operation.document), operation.field)
		}
	})
}

func TestDefinitionAuthoringFailsClosedWhenNotConfigured(t *testing.T) {
	handler, path := catalogServer(t)
	for _, operation := range append(definitionReadOperations(), definitionWriteOperations()...) {
		body := postTrusted(t, handler, path, operation.document)
		if !strings.Contains(body, "integration definition authoring unavailable") {
			t.Errorf("%s without authoring: %s", operation.field, body)
		}
	}
}
