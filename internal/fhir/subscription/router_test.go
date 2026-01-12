//nolint:gosec,errcheck // Test file - G104 errors intentionally ignored in test setup
package subscription

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// =============================================================================
// WorkflowRouter Tests
// =============================================================================

// Note: WorkflowRouter requires a real workflow.Engine which has many dependencies.
// For proper unit testing, we test through the interface it implements (EventRouter)
// and verify behavior via the Manager which uses it.

// =============================================================================
// MetricsRouter Tests
// =============================================================================

// mockReceiverMetrics implements ReceiverMetrics for testing
type mockReceiverMetrics struct {
	receivedCalls  []string
	processedCalls []struct {
		subscription string
		success      bool
		duration     time.Duration
	}
	errorCalls []struct {
		subscription string
		errorType    string
	}
}

func (m *mockReceiverMetrics) NotificationReceived(subscription, resourceType string) {
	m.receivedCalls = append(m.receivedCalls, subscription+":"+resourceType)
}

func (m *mockReceiverMetrics) NotificationProcessed(subscription string, success bool, duration time.Duration) {
	m.processedCalls = append(m.processedCalls, struct {
		subscription string
		success      bool
		duration     time.Duration
	}{subscription, success, duration})
}

func (m *mockReceiverMetrics) NotificationError(subscription, errorType string) {
	m.errorCalls = append(m.errorCalls, struct {
		subscription string
		errorType    string
	}{subscription, errorType})
}

func TestNewMetricsRouter(t *testing.T) {
	inner := NoOpRouter{}
	metrics := &mockReceiverMetrics{}

	router := NewMetricsRouter(inner, metrics, "test_router")

	if router == nil {
		t.Fatal("NewMetricsRouter returned nil")
	}
	if router.router != inner {
		t.Error("Router not set correctly")
	}
	if router.name != "test_router" {
		t.Errorf("Expected name 'test_router', got '%s'", router.name)
	}
}

func TestMetricsRouter_Route_Success(t *testing.T) {
	inner := NewCallbackRouter(func(ctx context.Context, event interface{}) error {
		return nil
	})
	metrics := &mockReceiverMetrics{}
	router := NewMetricsRouter(inner, metrics, "test_router")

	err := router.Route(context.Background(), "test event")
	if err != nil {
		t.Fatalf("Route failed: %v", err)
	}

	// Should not record error on success
	if len(metrics.errorCalls) != 0 {
		t.Errorf("Expected no error calls, got %d", len(metrics.errorCalls))
	}
}

func TestMetricsRouter_Route_Error(t *testing.T) {
	testErr := errors.New("routing failed")
	inner := NewCallbackRouter(func(ctx context.Context, event interface{}) error {
		return testErr
	})
	metrics := &mockReceiverMetrics{}
	router := NewMetricsRouter(inner, metrics, "test_router")

	err := router.Route(context.Background(), "test event")
	if err == nil {
		t.Fatal("Expected error")
	}

	// Should record error
	if len(metrics.errorCalls) != 1 {
		t.Fatalf("Expected 1 error call, got %d", len(metrics.errorCalls))
	}
	if metrics.errorCalls[0].subscription != "test_router" {
		t.Errorf("Expected subscription 'test_router', got '%s'", metrics.errorCalls[0].subscription)
	}
	if metrics.errorCalls[0].errorType != "routing_error" {
		t.Errorf("Expected errorType 'routing_error', got '%s'", metrics.errorCalls[0].errorType)
	}
}

// =============================================================================
// MultiRouterError Tests
// =============================================================================

func TestMultiRouterError_Error(t *testing.T) {
	err := &MultiRouterError{
		Errors: []error{
			errors.New("error 1"),
			errors.New("error 2"),
			errors.New("error 3"),
		},
	}

	msg := err.Error()
	if !strings.Contains(msg, "3 destination(s)") {
		t.Errorf("Error message should contain count: %s", msg)
	}
}

func TestMultiRouterError_Error_SingleError(t *testing.T) {
	err := &MultiRouterError{
		Errors: []error{
			errors.New("single error"),
		},
	}

	msg := err.Error()
	if !strings.Contains(msg, "1 destination(s)") {
		t.Errorf("Error message should contain count: %s", msg)
	}
}

// =============================================================================
// NoOpRouter Tests
// =============================================================================

func TestNoOpRouter_Route(t *testing.T) {
	router := NoOpRouter{}

	err := router.Route(context.Background(), "any event")
	if err != nil {
		t.Errorf("NoOpRouter should never return error, got: %v", err)
	}

	// Test with nil event
	err = router.Route(context.Background(), nil)
	if err != nil {
		t.Errorf("NoOpRouter should never return error for nil, got: %v", err)
	}
}

// =============================================================================
// Manager Tests
// =============================================================================

func TestNewManager(t *testing.T) {
	router := NoOpRouter{}
	manager := NewManager(router)

	if manager == nil {
		t.Fatal("NewManager returned nil")
	}
	if manager.clients == nil {
		t.Error("clients map not initialized")
	}
	if manager.configs == nil {
		t.Error("configs map not initialized")
	}
	if manager.receiver == nil {
		t.Error("receiver not created")
	}
	if manager.router != router {
		t.Error("router not set correctly")
	}
}

func TestManager_RegisterSubscription(t *testing.T) {
	router := NoOpRouter{}
	manager := NewManager(router)

	def := &SubscriptionDefinition{
		Name:     "patient_updates",
		Server:   "https://fhir.example.com",
		Criteria: "Patient",
		Channel: ChannelConfig{
			Endpoint: "https://callback.example.com/notify",
		},
	}

	err := manager.RegisterSubscription(def)
	if err != nil {
		t.Fatalf("RegisterSubscription failed: %v", err)
	}

	// Verify it was stored
	manager.mu.RLock()
	stored, exists := manager.configs["patient_updates"]
	manager.mu.RUnlock()

	if !exists {
		t.Error("Subscription not stored in configs")
	}
	if stored.Name != "patient_updates" {
		t.Errorf("Expected name 'patient_updates', got '%s'", stored.Name)
	}
}

func TestManager_CreateSubscription_NotRegistered(t *testing.T) {
	router := NoOpRouter{}
	manager := NewManager(router)

	// Try to create without registering first
	_, err := manager.CreateSubscription(context.Background(), "unknown")
	if err == nil {
		t.Error("Expected error for unregistered subscription")
	}
	if !strings.Contains(err.Error(), "not registered") {
		t.Errorf("Error should mention 'not registered': %v", err)
	}
}

func TestManager_CreateSubscription(t *testing.T) {
	// Create a mock FHIR server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path == "/Subscription" {
			w.Header().Set("Content-Type", "application/fhir+json")
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{
				"resourceType": "Subscription",
				"id": "created-sub-123",
				"status": "active",
				"criteria": "Patient"
			}`))
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer server.Close()

	router := NoOpRouter{}
	manager := NewManager(router)

	def := &SubscriptionDefinition{
		Name:        "patient_updates",
		Description: "Track patient updates",
		Server:      server.URL,
		Criteria:    "Patient",
		Channel: ChannelConfig{
			Endpoint: "https://callback.example.com/notify",
		},
	}

	manager.RegisterSubscription(def)

	sub, err := manager.CreateSubscription(context.Background(), "patient_updates")
	if err != nil {
		t.Fatalf("CreateSubscription failed: %v", err)
	}

	if sub.ID != "created-sub-123" {
		t.Errorf("Expected ID 'created-sub-123', got '%s'", sub.ID)
	}
}

func TestManager_CreateSubscription_WithBearerAuth(t *testing.T) {
	var receivedAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/fhir+json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"resourceType": "Subscription", "id": "sub-123", "status": "active"}`))
	}))
	defer server.Close()

	router := NoOpRouter{}
	manager := NewManager(router)

	def := &SubscriptionDefinition{
		Name:     "secure_sub",
		Server:   server.URL,
		Criteria: "Patient",
		Auth: AuthConfig{
			Type:  "bearer",
			Token: "my-secret-token",
		},
		Channel: ChannelConfig{
			Endpoint: "https://callback.example.com/notify",
		},
	}

	manager.RegisterSubscription(def)
	manager.CreateSubscription(context.Background(), "secure_sub")

	if receivedAuth != "Bearer my-secret-token" {
		t.Errorf("Expected 'Bearer my-secret-token', got '%s'", receivedAuth)
	}
}

func TestManager_DeleteSubscription_NotRegistered(t *testing.T) {
	router := NoOpRouter{}
	manager := NewManager(router)

	err := manager.DeleteSubscription(context.Background(), "unknown", "sub-123")
	if err == nil {
		t.Error("Expected error for unregistered subscription")
	}
	if !strings.Contains(err.Error(), "not registered") {
		t.Errorf("Error should mention 'not registered': %v", err)
	}
}

func TestManager_DeleteSubscription(t *testing.T) {
	var deletedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "DELETE" {
			deletedPath = r.URL.Path
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer server.Close()

	router := NoOpRouter{}
	manager := NewManager(router)

	def := &SubscriptionDefinition{
		Name:     "patient_updates",
		Server:   server.URL,
		Criteria: "Patient",
		Channel: ChannelConfig{
			Endpoint: "https://callback.example.com/notify",
		},
	}

	manager.RegisterSubscription(def)

	err := manager.DeleteSubscription(context.Background(), "patient_updates", "sub-456")
	if err != nil {
		t.Fatalf("DeleteSubscription failed: %v", err)
	}

	if deletedPath != "/Subscription/sub-456" {
		t.Errorf("Expected DELETE to '/Subscription/sub-456', got '%s'", deletedPath)
	}
}

func TestManager_GetReceiver(t *testing.T) {
	router := NoOpRouter{}
	manager := NewManager(router)

	receiver := manager.GetReceiver()
	if receiver == nil {
		t.Error("GetReceiver returned nil")
	}
	if receiver != manager.receiver {
		t.Error("GetReceiver should return the manager's receiver")
	}
}

func TestManager_ClientCaching(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/fhir+json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"resourceType": "Subscription", "id": "sub-123", "status": "active"}`))
	}))
	defer server.Close()

	router := NoOpRouter{}
	manager := NewManager(router)

	// Register two subscriptions pointing to the same server
	manager.RegisterSubscription(&SubscriptionDefinition{
		Name:     "sub1",
		Server:   server.URL,
		Criteria: "Patient",
		Channel:  ChannelConfig{Endpoint: "https://example.com/a"},
	})
	manager.RegisterSubscription(&SubscriptionDefinition{
		Name:     "sub2",
		Server:   server.URL,
		Criteria: "Encounter",
		Channel:  ChannelConfig{Endpoint: "https://example.com/b"},
	})

	// Create both subscriptions
	manager.CreateSubscription(context.Background(), "sub1")
	manager.CreateSubscription(context.Background(), "sub2")

	// Verify we only have one client cached
	manager.mu.RLock()
	clientCount := len(manager.clients)
	manager.mu.RUnlock()

	if clientCount != 1 {
		t.Errorf("Expected 1 cached client, got %d", clientCount)
	}
}

// =============================================================================
// OAuth2Auth Tests
// =============================================================================

func TestNewOAuth2Auth(t *testing.T) {
	auth := NewOAuth2Auth(
		"https://auth.example.com/token",
		"client-123",
		"secret-456",
		[]string{"fhir.read", "fhir.write"},
	)

	if auth == nil {
		t.Fatal("NewOAuth2Auth returned nil")
	}
	if auth.config.TokenURL != "https://auth.example.com/token" {
		t.Errorf("Expected TokenURL 'https://auth.example.com/token', got '%s'", auth.config.TokenURL)
	}
	if auth.config.ClientID != "client-123" {
		t.Errorf("Expected ClientID 'client-123', got '%s'", auth.config.ClientID)
	}
	if auth.config.ClientSecret != "secret-456" {
		t.Errorf("Expected ClientSecret 'secret-456', got '%s'", auth.config.ClientSecret)
	}
	if len(auth.config.Scopes) != 2 {
		t.Errorf("Expected 2 scopes, got %d", len(auth.config.Scopes))
	}
}

// Note: OAuth2Auth.GetAuthHeader requires a real token server or mocking
// the workflow.GetOAuthToken function which is not easily testable without
// refactoring. For integration testing, a real OAuth2 server would be used.

// =============================================================================
// ChannelRouter Additional Tests
// =============================================================================

func TestChannelRouter_BufferFull(t *testing.T) {
	router := NewChannelRouter(1)

	// Fill the buffer
	err := router.Route(context.Background(), "event1")
	if err != nil {
		t.Fatalf("First route should succeed: %v", err)
	}

	// Try to route when buffer is full (non-blocking context)
	err = router.Route(context.Background(), "event2")
	if err == nil {
		t.Error("Expected error when buffer is full")
	}
	if !strings.Contains(err.Error(), "channel full") {
		t.Errorf("Error should mention 'channel full': %v", err)
	}

	router.Close()
}

func TestChannelRouter_MultipleEvents(t *testing.T) {
	router := NewChannelRouter(10)

	// Route multiple events
	for i := 0; i < 5; i++ {
		err := router.Route(context.Background(), i)
		if err != nil {
			t.Fatalf("Route %d failed: %v", i, err)
		}
	}

	// Verify all events are in channel
	events := router.Events()
	for i := 0; i < 5; i++ {
		select {
		case event := <-events:
			if event != i {
				t.Errorf("Expected event %d, got %v", i, event)
			}
		default:
			t.Errorf("Expected event %d, but channel empty", i)
		}
	}

	router.Close()
}

// =============================================================================
// FilterRouter Additional Tests
// =============================================================================

func TestFilterRouter_AllFiltered(t *testing.T) {
	var called bool
	inner := NewCallbackRouter(func(ctx context.Context, event interface{}) error {
		called = true
		return nil
	})

	// Filter that blocks everything
	filter := NewFilterRouter(inner, func(event interface{}) bool {
		return false
	})

	err := filter.Route(context.Background(), "should be filtered")
	if err != nil {
		t.Errorf("FilterRouter should not return error for filtered event: %v", err)
	}
	if called {
		t.Error("Inner router should not be called for filtered events")
	}
}

func TestFilterRouter_PropagatesError(t *testing.T) {
	testErr := errors.New("inner error")
	inner := NewCallbackRouter(func(ctx context.Context, event interface{}) error {
		return testErr
	})

	// Filter that passes everything
	filter := NewFilterRouter(inner, func(event interface{}) bool {
		return true
	})

	err := filter.Route(context.Background(), "event")
	if !errors.Is(err, testErr) {
		t.Errorf("Expected inner error, got: %v", err)
	}
}

// =============================================================================
// CallbackRouter Tests
// =============================================================================

func TestCallbackRouter_NilCallback(t *testing.T) {
	// This would panic, so we just verify the struct can be created
	// In practice, callers should always provide a valid callback
	router := &CallbackRouter{callback: nil}

	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic for nil callback")
		}
	}()

	router.Route(context.Background(), "event")
}

// testContextKey is a custom type for context keys to avoid collisions
type testContextKey string

func TestCallbackRouter_ContextPropagation(t *testing.T) {
	var receivedCtx context.Context

	router := NewCallbackRouter(func(ctx context.Context, event interface{}) error {
		receivedCtx = ctx
		return nil
	})

	ctx := context.WithValue(context.Background(), testContextKey("key"), "value")
	router.Route(ctx, "event")

	if receivedCtx.Value(testContextKey("key")) != "value" {
		t.Error("Context not propagated to callback")
	}
}

// =============================================================================
// Integration Tests for Router Composition
// =============================================================================

func TestRouterComposition(t *testing.T) {
	var events []string

	// Create a capture callback
	capture := NewCallbackRouter(func(ctx context.Context, event interface{}) error {
		events = append(events, event.(string))
		return nil
	})

	// Create a filter that only passes events starting with "PASS:"
	filtered := NewFilterRouter(capture, func(event interface{}) bool {
		s, ok := event.(string)
		return ok && strings.HasPrefix(s, "PASS:")
	})

	// Create metrics wrapper
	metrics := &mockReceiverMetrics{}
	metricsRouter := NewMetricsRouter(filtered, metrics, "composed")

	// Route various events
	metricsRouter.Route(context.Background(), "SKIP: event 1")
	metricsRouter.Route(context.Background(), "PASS: event 1")
	metricsRouter.Route(context.Background(), "SKIP: event 2")
	metricsRouter.Route(context.Background(), "PASS: event 2")

	// Verify only PASS events were captured
	if len(events) != 2 {
		t.Errorf("Expected 2 events, got %d: %v", len(events), events)
	}

	// Verify no errors recorded (all succeeded, filtering is not an error)
	if len(metrics.errorCalls) != 0 {
		t.Errorf("Expected no errors, got %d", len(metrics.errorCalls))
	}
}
