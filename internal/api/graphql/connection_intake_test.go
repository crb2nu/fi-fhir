package graphql_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	graphqlapi "gitlab.flexinfer.ai/libs/fi-fhir/internal/api/graphql"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/graphql/resolvers"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/connection"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/delivery"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/operator"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/session"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
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

// TestSessionSampleRedactedPayloadOverTheWire is review C1 end to end: the
// same session read returns a captured sample's text to a caller holding
// integration.operator and null to a caller holding only the legacy
// graphql:operator role that gates session reads; a pasted sample's text is
// never returned; and an export carries no captured text whatever the grants.
func TestSessionSampleRedactedPayloadOverTheWire(t *testing.T) {
	ctx := context.Background()
	store := session.NewMemoryStore()
	workspace, err := store.CreateSession(ctx, session.CreateSessionRequest{Name: "capture gate"})
	if err != nil {
		t.Fatal(err)
	}
	const message = "MSH|^~\\&|S|F|R|F|20260926||ADT^A01|c-1|P|2.5.1\rPID|1||MRN-SYNTH^^^HOSP^MR||Zyxwpat^Quorbina\r"
	captured, err := store.AddSample(ctx, workspace.ID, session.AddSampleRequest{
		ID: "sample_capture_c-1_1", Name: "capture c-1 #1", Format: events.FormatHL7v2, Source: "capture:c-1",
		Raw: message, PHIPolicy: session.PHIPolicyRedact, Redaction: session.SampleRedactionCapture,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AddSample(ctx, workspace.ID, session.AddSampleRequest{Name: "pasted", Format: events.FormatHL7v2, Raw: message}); err != nil {
		t.Fatal(err)
	}
	serverWith := func(roles ...string) (http.Handler, string) {
		config := secureServerConfig(testAuthenticator(t))
		config.MaxRequestBodyBytes = 1 << 16
		config.TrustedNetworkAuthenticator = trustedNetwork(t, roles...)
		server, err := graphqlapi.NewServer(resolvers.NewResolver(resolvers.WithIntegrationSessionStore(store)), config)
		if err != nil {
			t.Fatalf("NewServer: %v", err)
		}
		return server.Handler(), config.Path
	}
	read := `query Op { integrationSession(id: "` + workspace.ID + `") { samples { name redactedPayload } } }`
	payloads := func(body string) map[string]*string {
		t.Helper()
		var decoded struct {
			Data struct {
				IntegrationSession struct {
					Samples []struct {
						Name            string  `json:"name"`
						RedactedPayload *string `json:"redactedPayload"`
					} `json:"samples"`
				} `json:"integrationSession"`
			} `json:"data"`
			Errors []any `json:"errors"`
		}
		if err := json.Unmarshal([]byte(body), &decoded); err != nil || len(decoded.Errors) != 0 {
			t.Fatalf("session read = %s (%v)", body, err)
		}
		byName := map[string]*string{}
		for _, sample := range decoded.Data.IntegrationSession.Samples {
			byName[sample.Name] = sample.RedactedPayload
		}
		if len(byName) != 2 {
			t.Fatalf("session read returned %d samples: %s", len(byName), body)
		}
		return byName
	}

	legacy, legacyPath := serverWith(graphqlapi.GraphQLOperatorRole)
	for name, payload := range payloads(postTrusted(t, legacy, legacyPath, read)) {
		if payload != nil {
			t.Errorf("graphql:operator alone read %s's text", name)
		}
	}
	operatorHandler, operatorPath := serverWith(graphqlapi.GraphQLOperatorRole, operator.ReadRole)
	granted := payloads(postTrusted(t, operatorHandler, operatorPath, read))
	if granted["capture c-1 #1"] == nil || *granted["capture c-1 #1"] != captured.Raw || granted["pasted"] != nil {
		t.Fatalf("integration.operator read captured %v, pasted %v; want the captured text only",
			granted["capture c-1 #1"], granted["pasted"])
	}
	if strings.Contains(*granted["capture c-1 #1"], "Zyxwpat") {
		t.Fatal("the captured text is not the capture redactor's output")
	}

	exporter, exportPath := serverWith(append(operatorBundle(), session.PHIExportRole)...)
	body := postTrusted(t, exporter, exportPath, `mutation Op { exportIntegrationBundle(input: {sessionId: "`+workspace.ID+
		`", reason: "hand over", includeRawPayload: true}) { samples { name redactedPayload } } }`)
	if !strings.Contains(body, `"capture c-1 #1"`) || strings.Contains(body, "REDACTED") || strings.Contains(body, "MSH|") {
		t.Fatalf("an export carried captured text: %s", body)
	}
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
