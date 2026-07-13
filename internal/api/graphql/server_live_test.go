package graphql_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	graphqlapi "gitlab.flexinfer.ai/libs/fi-fhir/internal/api/graphql"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/graphql/model"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/graphql/resolvers"
)

type observedResolver struct {
	graphqlapi.ResolverRoot
	subscribed chan struct{}
	once       sync.Once
}

func (r *observedResolver) Subscription() graphqlapi.SubscriptionResolver {
	return &observedSubscriptionResolver{
		SubscriptionResolver: r.ResolverRoot.Subscription(),
		onSubscribed: func() {
			r.once.Do(func() {
				close(r.subscribed)
			})
		},
	}
}

type observedSubscriptionResolver struct {
	graphqlapi.SubscriptionResolver
	onSubscribed func()
}

func (r *observedSubscriptionResolver) IntegrationSessionEvents(
	ctx context.Context,
	sessionID string,
) (<-chan *model.IntegrationSessionEvent, error) {
	events, err := r.SubscriptionResolver.IntegrationSessionEvents(ctx, sessionID)
	if err == nil {
		r.onSubscribed()
	}
	return events, err
}

type graphQLError struct {
	Message string
}

func postGraphQL[T any](
	t *testing.T,
	client *http.Client,
	endpoint string,
	query string,
	variables map[string]any,
) T {
	t.Helper()

	var request bytes.Buffer
	if err := json.NewEncoder(&request).Encode(map[string]any{
		"query":     query,
		"variables": variables,
	}); err != nil {
		t.Fatalf("encode GraphQL request: %v", err)
	}

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, endpoint, &request)
	if err != nil {
		t.Fatalf("create GraphQL request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	response, err := client.Do(req)
	if err != nil {
		t.Fatalf("execute GraphQL request: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		t.Fatalf("GraphQL status = %d, want %d", response.StatusCode, http.StatusOK)
	}

	var result struct {
		Data   T
		Errors []graphQLError
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatalf("decode GraphQL response: %v", err)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("GraphQL errors: %+v", result.Errors)
	}
	return result.Data
}

func TestLiveIntegrationSessionSubscription(t *testing.T) {
	baseResolver := resolvers.NewResolver()
	resolver := &observedResolver{
		ResolverRoot: baseResolver,
		subscribed:   make(chan struct{}),
	}
	config := graphqlapi.DefaultServerConfig()
	config.PlaygroundEnabled = false

	server := graphqlapi.NewServer(resolver, config)
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(httpServer.Close)

	const createSessionMutation = "mutation CreateLiveSession($input: CreateIntegrationSessionInput!) {" +
		" createIntegrationSession(input: $input) { id name }" +
		"}"
	type createSessionData struct {
		CreateIntegrationSession struct {
			ID   string
			Name string
		}
	}
	created := postGraphQL[createSessionData](
		t,
		httpServer.Client(),
		httpServer.URL+config.Path,
		createSessionMutation,
		map[string]any{"input": map[string]any{"name": "Live WebSocket test"}},
	)
	sessionID := created.CreateIntegrationSession.ID
	if sessionID == "" {
		t.Fatal("created integration session has an empty ID")
	}

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http") + config.WebSocketPath
	dialer := websocket.Dialer{Subprotocols: []string{"graphql-transport-ws"}}
	connection, response, err := dialer.Dial(wsURL, nil)
	if response != nil {
		defer response.Body.Close()
	}
	if err != nil {
		status := 0
		if response != nil {
			status = response.StatusCode
		}
		t.Fatalf("dial GraphQL WebSocket (status %d): %v", status, err)
	}
	t.Cleanup(func() {
		_ = connection.Close()
	})
	if got := connection.Subprotocol(); got != "graphql-transport-ws" {
		t.Fatalf("WebSocket subprotocol = %q, want graphql-transport-ws", got)
	}
	if err := connection.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatalf("set WebSocket read deadline: %v", err)
	}

	if err := connection.WriteJSON(map[string]any{"type": "connection_init"}); err != nil {
		t.Fatalf("write connection_init: %v", err)
	}
	var frame struct {
		ID      string
		Type    string
		Payload json.RawMessage
	}
	if err := connection.ReadJSON(&frame); err != nil {
		t.Fatalf("read connection acknowledgement: %v", err)
	}
	if frame.Type != "connection_ack" {
		t.Fatalf("first WebSocket frame type = %q, want connection_ack", frame.Type)
	}

	const subscriptionID = "session-events"
	const subscriptionQuery = "subscription LiveSessionEvents($sessionId: ID!) {" +
		" integrationSessionEvents(sessionId: $sessionId) {" +
		" id type sessionId message session { id }" +
		" }" +
		"}"
	if err := connection.WriteJSON(map[string]any{
		"id":   subscriptionID,
		"type": "subscribe",
		"payload": map[string]any{
			"query":     subscriptionQuery,
			"variables": map[string]any{"sessionId": sessionID},
		},
	}); err != nil {
		t.Fatalf("write subscription: %v", err)
	}

	select {
	case <-resolver.subscribed:
	case <-time.After(5 * time.Second):
		t.Fatal("integration session subscription was not registered")
	}

	const addSampleMutation = "mutation AddLiveSample($input: AddSessionSampleInput!) {" +
		" addSessionSample(input: $input) {" +
		" id sessionId name format payloadChecksum" +
		" }" +
		"}"
	type addSampleData struct {
		AddSessionSample struct {
			ID              string
			SessionID       string
			Name            string
			Format          string
			PayloadChecksum string
		}
	}
	added := postGraphQL[addSampleData](
		t,
		httpServer.Client(),
		httpServer.URL+config.Path,
		addSampleMutation,
		map[string]any{"input": map[string]any{
			"sessionId": sessionID,
			"name":      "ADT A01",
			"format":    "HL7V2",
			"data": "MSH|^~\\&|SENDING|FACILITY|RECEIVING|FACILITY|" +
				"20240115120000||ADT^A01|MSG00001|P|2.5\r" +
				"PID|1||123456^^^HOSPITAL^MRN",
		}},
	)
	if added.AddSessionSample.ID == "" {
		t.Fatal("added session sample has an empty ID")
	}
	if added.AddSessionSample.SessionID != sessionID {
		t.Fatalf(
			"added sample session ID = %q, want %q",
			added.AddSessionSample.SessionID,
			sessionID,
		)
	}
	if added.AddSessionSample.Format != "HL7V2" {
		t.Fatalf("added sample format = %q, want HL7V2", added.AddSessionSample.Format)
	}
	if added.AddSessionSample.PayloadChecksum == "" {
		t.Fatal("added session sample has an empty payload checksum")
	}

	frame = struct {
		ID      string
		Type    string
		Payload json.RawMessage
	}{}
	if err := connection.ReadJSON(&frame); err != nil {
		t.Fatalf("read subscription event: %v", err)
	}
	if frame.ID != subscriptionID {
		t.Fatalf("subscription frame ID = %q, want %q", frame.ID, subscriptionID)
	}
	if frame.Type != "next" {
		t.Fatalf("subscription frame type = %q, want next", frame.Type)
	}

	var payload struct {
		Data struct {
			IntegrationSessionEvents struct {
				ID        string
				Type      string
				SessionID string
				Message   string
				Session   *struct {
					ID string
				}
			}
		}
		Errors []graphQLError
	}
	if err := json.Unmarshal(frame.Payload, &payload); err != nil {
		t.Fatalf("decode subscription payload: %v", err)
	}
	if len(payload.Errors) != 0 {
		t.Fatalf("subscription GraphQL errors: %+v", payload.Errors)
	}

	event := payload.Data.IntegrationSessionEvents
	if event.ID == "" {
		t.Fatal("subscription event has an empty ID")
	}
	if event.Type != "sample.added" {
		t.Fatalf("subscription event type = %q, want sample.added", event.Type)
	}
	if event.SessionID != sessionID {
		t.Fatalf("subscription session ID = %q, want %q", event.SessionID, sessionID)
	}
	if event.Message != "session sample added" {
		t.Fatalf(
			"subscription event message = %q, want session sample added",
			event.Message,
		)
	}
	if event.Session == nil || event.Session.ID != sessionID {
		t.Fatalf("subscription nested session = %+v, want ID %q", event.Session, sessionID)
	}

	if err := connection.WriteJSON(map[string]any{
		"id":   subscriptionID,
		"type": "complete",
	}); err != nil {
		t.Fatalf("write subscription completion: %v", err)
	}

	t.Logf(
		"verified %s -> connection_ack -> subscribe -> sample.added for session %s",
		fmt.Sprintf("%s%s", httpServer.URL, config.Path),
		sessionID,
	)
}
