package graphql_test

import (
	"context"
	"strings"
	"testing"

	graphqlapi "gitlab.flexinfer.ai/libs/fi-fhir/internal/api/graphql"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/graphql/resolvers"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/connection"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/delivery"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/operator"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/session"
)

// The .loom/38 C-2 sample-intake fields at the GraphQL boundary: the transport
// gate's decisions and the fail-closed answers that need no database. The
// behaviour behind them is proved over PostgreSQL, a real MLLP listener, and
// MinIO in internal/integration/connection (make connection-capture).

func connectionIntakeOperations() []controlPlaneOperation {
	return []controlPlaneOperation{
		{field: "connectionCaptures", document: `query Op { connectionCaptures(sessionId: "sess-1") { id status } }`},
		{field: "peekBatchConnection", document: `mutation Op { peekBatchConnection(input: {connectionId: "adt-drop", sessionId: "sess-1", reason: "gate"}) { capture { id } } }`},
		{field: "startConnectionCapture", document: `mutation Op { startConnectionCapture(input: {sourceId: "adt-east", sessionId: "sess-1", reason: "gate"}) { id } }`},
		{field: "cancelConnectionCapture", document: `mutation Op { cancelConnectionCapture(id: "cap-1", reason: "gate") { id } }`},
	}
}

func TestTransportGate_ConnectionIntakeRoles(t *testing.T) {
	handler, path, issuer := newTransportGateHandler(t)

	t.Run("integration.operator reaches every intake field", func(t *testing.T) {
		token := transportGateToken(t, issuer, "operator-read-only", operator.ReadRole)
		for _, operation := range connectionIntakeOperations() {
			assertGatePassed(t, postTransportGate(t, handler, path, token, operation.document), operation.field)
		}
	})
	t.Run("the deployment and delivery grants without the read role reach none", func(t *testing.T) {
		token := transportGateToken(t, issuer, "deployment-only", operator.DeploymentOperatorRole, delivery.OperatorRole)
		for _, operation := range connectionIntakeOperations() {
			assertGateRefused(t, postTransportGate(t, handler, path, token, operation.document), operation.field)
		}
	})
	t.Run("integration.phi.export reaches none", func(t *testing.T) {
		token := transportGateToken(t, issuer, "phi-export-only", session.PHIExportRole)
		for _, operation := range connectionIntakeOperations() {
			assertGateRefused(t, postTransportGate(t, handler, path, token, operation.document), operation.field)
		}
	})
}

// TestConnectionIntakeFailsClosed: without a catalog every field answers as the
// catalog does; with a catalog but no session workspace every field answers
// that intake is unavailable, and none reaches the store.
func TestConnectionIntakeFailsClosed(t *testing.T) {
	t.Run("no catalog", func(t *testing.T) {
		handler, path := catalogServer(t)
		for _, operation := range connectionIntakeOperations() {
			if body := postTrusted(t, handler, path, operation.document); !strings.Contains(body, "connection catalog unavailable") {
				t.Errorf("%s without a catalog: %s", operation.field, body)
			}
		}
	})
	t.Run("catalog without a session workspace", func(t *testing.T) {
		handler, path := catalogServer(t, resolvers.WithConnectionCatalog(unreachableCatalog(t)))
		for _, operation := range connectionIntakeOperations() {
			body := postTrusted(t, handler, path, operation.document)
			if !strings.Contains(body, resolvers.ErrConnectionIntakeUnavailable.Error()) ||
				strings.Contains(body, "request failed") {
				t.Errorf("%s without sessions: %s", operation.field, body)
			}
		}
	})
}

// sessionsThatExist reports every session as active and never writes.
type sessionsThatExist struct{}

func (sessionsThatExist) GetSession(_ context.Context, sessionID string) (*session.Session, error) {
	return &session.Session{ID: sessionID, Status: session.SessionStatusActive}, nil
}

func (sessionsThatExist) AddSample(context.Context, string, session.AddSampleRequest) (*session.Sample, error) {
	return nil, session.ErrInvalid
}

func TestConnectionIntakeRefusesAnExplicitZeroBeforeTheStore(t *testing.T) {
	catalog := unreachableCatalog(t)
	if err := catalog.EnableSampleIntake(connection.IntakeConfig{Sessions: sessionsThatExist{}}); err != nil {
		t.Fatal(err)
	}
	handler, path := catalogServer(t, resolvers.WithConnectionCatalog(catalog))
	for name, document := range map[string]string{
		"peek maxMessages 0":     `mutation Op { peekBatchConnection(input: {connectionId: "adt-drop", sessionId: "sess-1", maxMessages: 0, reason: "r"}) { capture { id } } }`,
		"peek maxObjects -1":     `mutation Op { peekBatchConnection(input: {connectionId: "adt-drop", sessionId: "sess-1", maxObjects: -1, reason: "r"}) { capture { id } } }`,
		"capture ttlSeconds 0":   `mutation Op { startConnectionCapture(input: {sourceId: "adt-east", sessionId: "sess-1", ttlSeconds: 0, reason: "r"}) { id } }`,
		"capture 101 messages":   `mutation Op { startConnectionCapture(input: {sourceId: "adt-east", sessionId: "sess-1", maxMessages: 101, reason: "r"}) { id } }`,
		"capture with no reason": `mutation Op { startConnectionCapture(input: {sourceId: "adt-east", sessionId: "sess-1", reason: " "}) { id } }`,
	} {
		body := postTrusted(t, handler, path, document)
		if !strings.Contains(body, "invalid connection sample intake request") {
			t.Errorf("%s: %s", name, body)
		}
	}
	// A well-formed request clears validation and reaches the store, which
	// here fails every query: the generic failure, never a store detail.
	body := postTrusted(t, handler, path, `query Op { connectionCaptures(sessionId: "sess-1") { id } }`)
	if !strings.Contains(body, "connection sample intake request failed") || strings.Contains(body, "unit test") {
		t.Fatalf("store failure over the wire: %s", body)
	}
	if strings.Contains(body, graphqlapi.GraphQLOperatorRole) {
		t.Fatalf("the failure named a role: %s", body)
	}
}
