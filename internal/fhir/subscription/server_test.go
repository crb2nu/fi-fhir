package subscription

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// =============================================================================
// Server Tests
// =============================================================================

func TestNewServer_DefaultConfig(t *testing.T) {
	router := NoOpRouter{}
	receiver := NewReceiver(router, nil)

	server := NewServer(receiver, nil)

	if server == nil {
		t.Fatal("NewServer returned nil")
	}
	if server.receiver != receiver {
		t.Error("Receiver not set correctly")
	}
	if server.httpServer == nil {
		t.Error("httpServer not created")
	}
	if server.httpServer.Addr != "0.0.0.0:8081" {
		t.Errorf("Expected default addr '0.0.0.0:8081', got '%s'", server.httpServer.Addr)
	}
	if server.httpServer.ReadTimeout != 30*time.Second {
		t.Errorf("Expected default ReadTimeout 30s, got %v", server.httpServer.ReadTimeout)
	}
	if server.httpServer.WriteTimeout != 30*time.Second {
		t.Errorf("Expected default WriteTimeout 30s, got %v", server.httpServer.WriteTimeout)
	}
}

func TestNewServer_CustomConfig(t *testing.T) {
	router := NoOpRouter{}
	receiver := NewReceiver(router, nil)

	config := &ServerConfig{
		Host:         "127.0.0.1",
		Port:         9090,
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 120 * time.Second,
	}

	server := NewServer(receiver, config)

	if server.httpServer.Addr != "127.0.0.1:9090" {
		t.Errorf("Expected addr '127.0.0.1:9090', got '%s'", server.httpServer.Addr)
	}
	if server.httpServer.ReadTimeout != 60*time.Second {
		t.Errorf("Expected ReadTimeout 60s, got %v", server.httpServer.ReadTimeout)
	}
	if server.httpServer.WriteTimeout != 120*time.Second {
		t.Errorf("Expected WriteTimeout 120s, got %v", server.httpServer.WriteTimeout)
	}
}

// Note: Server.Start and Server.Shutdown require actual network operations
// which are better tested in integration tests. However, we can test the
// shutdown behavior with a mock.

func TestServer_Shutdown(t *testing.T) {
	router := NoOpRouter{}
	receiver := NewReceiver(router, nil)

	// Create server but don't start it
	server := NewServer(receiver, &ServerConfig{
		Host: "127.0.0.1",
		Port: 0, // Use port 0 for testing
	})

	// Shutdown should work even if server wasn't started
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// This might error because server wasn't started, but shouldn't panic
	_ = server.Shutdown(ctx)
}

// =============================================================================
// Receiver isAllowedSource Tests
// =============================================================================

func TestReceiver_isAllowedSource_EmptySource(t *testing.T) {
	router := NoOpRouter{}
	receiver := NewReceiver(router, &ReceiverOptions{
		AllowedSources: []string{"https://fhir.example.com"},
	})

	// Empty source should not be allowed
	allowed := receiver.isAllowedSource("", nil)
	if allowed {
		t.Error("Empty source should not be allowed")
	}
}

func TestReceiver_isAllowedSource_GlobalAllowed(t *testing.T) {
	router := NoOpRouter{}
	receiver := NewReceiver(router, &ReceiverOptions{
		AllowedSources: []string{
			"https://fhir.example.com",
			"https://api.hospital.org",
		},
	})

	tests := []struct {
		source  string
		allowed bool
	}{
		{"https://fhir.example.com", true},
		{"https://fhir.example.com/r4/Patient", true},
		{"https://api.hospital.org", true},
		{"https://api.hospital.org/Subscription", true},
		{"https://unknown.com", false},
		{"https://malicious.com", false},
	}

	for _, tt := range tests {
		t.Run(tt.source, func(t *testing.T) {
			result := receiver.isAllowedSource(tt.source, nil)
			if result != tt.allowed {
				t.Errorf("isAllowedSource(%q) = %v, want %v", tt.source, result, tt.allowed)
			}
		})
	}
}

func TestReceiver_isAllowedSource_ConfigSpecific(t *testing.T) {
	router := NoOpRouter{}
	// No global allowed sources
	receiver := NewReceiver(router, &ReceiverOptions{})

	configSources := []string{"https://config-specific.com"}

	tests := []struct {
		source  string
		allowed bool
	}{
		{"https://config-specific.com", true},
		{"https://config-specific.com/path", true},
		{"https://not-allowed.com", false},
	}

	for _, tt := range tests {
		t.Run(tt.source, func(t *testing.T) {
			result := receiver.isAllowedSource(tt.source, configSources)
			if result != tt.allowed {
				t.Errorf("isAllowedSource(%q) = %v, want %v", tt.source, result, tt.allowed)
			}
		})
	}
}

func TestReceiver_isAllowedSource_BothSourceLists(t *testing.T) {
	router := NoOpRouter{}
	receiver := NewReceiver(router, &ReceiverOptions{
		AllowedSources: []string{"https://global.com"},
	})

	configSources := []string{"https://config.com"}

	// Both global and config-specific should work
	if !receiver.isAllowedSource("https://global.com", configSources) {
		t.Error("Global source should be allowed")
	}
	if !receiver.isAllowedSource("https://config.com", configSources) {
		t.Error("Config-specific source should be allowed")
	}
}

// =============================================================================
// Receiver Source Verification Tests
// =============================================================================

func TestReceiver_SourceVerification_Forbidden(t *testing.T) {
	router := NoOpRouter{}
	receiver := NewReceiver(router, &ReceiverOptions{
		PathPrefix:     "/fhir/notify",
		VerifySource:   true,
		AllowedSources: []string{"https://allowed-fhir.com"},
	})

	receiver.RegisterSubscription(&SubscriptionConfig{
		Name: "test_sub",
	})

	// Create a valid bundle
	bundle := NotificationBundle{
		ResourceType: "Bundle",
		Type:         "history",
		Entry:        []NotificationEntry{},
	}
	body, _ := json.Marshal(bundle)

	req := httptest.NewRequest("POST", "/fhir/notify/test_sub", strings.NewReader(string(body)))
	req.Header.Set("Origin", "https://malicious-server.com")
	w := httptest.NewRecorder()

	receiver.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("Expected status 403 for unauthorized source, got %d", w.Code)
	}
}

func TestReceiver_SourceVerification_XForwardedFor(t *testing.T) {
	router := NoOpRouter{}
	receiver := NewReceiver(router, &ReceiverOptions{
		PathPrefix:     "/fhir/notify",
		VerifySource:   true,
		AllowedSources: []string{"10.0.0.1"},
	})

	receiver.RegisterSubscription(&SubscriptionConfig{
		Name: "test_sub",
	})

	bundle := NotificationBundle{
		ResourceType: "Bundle",
		Type:         "history",
		Entry:        []NotificationEntry{},
	}
	body, _ := json.Marshal(bundle)

	// Use X-Forwarded-For instead of Origin
	req := httptest.NewRequest("POST", "/fhir/notify/test_sub", strings.NewReader(string(body)))
	req.Header.Set("X-Forwarded-For", "10.0.0.1")
	w := httptest.NewRecorder()

	receiver.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200 for allowed X-Forwarded-For, got %d: %s", w.Code, w.Body.String())
	}
}

func TestReceiver_SourceVerification_PerSubscriptionAllowed(t *testing.T) {
	router := NoOpRouter{}
	receiver := NewReceiver(router, &ReceiverOptions{
		PathPrefix:   "/fhir/notify",
		VerifySource: true,
		// No global allowed sources
	})

	// Register subscription with specific allowed sources
	receiver.RegisterSubscription(&SubscriptionConfig{
		Name:           "test_sub",
		AllowedSources: []string{"https://subscription-specific.com"},
	})

	bundle := NotificationBundle{
		ResourceType: "Bundle",
		Type:         "history",
		Entry:        []NotificationEntry{},
	}
	body, _ := json.Marshal(bundle)

	req := httptest.NewRequest("POST", "/fhir/notify/test_sub", strings.NewReader(string(body)))
	req.Header.Set("Origin", "https://subscription-specific.com")
	w := httptest.NewRecorder()

	receiver.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200 for subscription-specific source, got %d: %s", w.Code, w.Body.String())
	}
}

// =============================================================================
// NoOpReceiverMetrics Tests
// =============================================================================

func TestNoOpReceiverMetrics(t *testing.T) {
	metrics := NoOpReceiverMetrics{}

	// All methods should be callable without panic or error
	metrics.NotificationReceived("sub1", "Patient")
	metrics.NotificationProcessed("sub1", true, time.Second)
	metrics.NotificationError("sub1", "parse_error")

	// If we get here without panic, the test passes
}

// =============================================================================
// Receiver Edge Cases
// =============================================================================

func TestReceiver_EmptySubscriptionName(t *testing.T) {
	router := NoOpRouter{}
	receiver := NewReceiver(router, &ReceiverOptions{
		PathPrefix: "/fhir/notify",
	})

	req := httptest.NewRequest("POST", "/fhir/notify/", strings.NewReader("{}"))
	w := httptest.NewRecorder()

	receiver.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400 for empty subscription name, got %d", w.Code)
	}
}

func TestReceiver_NotBundleResourceType(t *testing.T) {
	router := NoOpRouter{}
	receiver := NewReceiver(router, nil)

	receiver.RegisterSubscription(&SubscriptionConfig{
		Name: "test_sub",
	})

	// Send a Patient instead of a Bundle
	body := `{"resourceType": "Patient", "id": "pat-123"}`
	req := httptest.NewRequest("POST", "/fhir/notify/test_sub", strings.NewReader(body))
	w := httptest.NewRecorder()

	receiver.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400 for non-Bundle resource, got %d", w.Code)
	}
}

func TestReceiver_WithMetrics(t *testing.T) {
	metrics := &mockReceiverMetrics{}
	router := NoOpRouter{}
	receiver := NewReceiver(router, &ReceiverOptions{
		Metrics: metrics,
	})

	receiver.RegisterSubscription(&SubscriptionConfig{
		Name: "test_sub",
	})

	bundle := NotificationBundle{
		ResourceType: "Bundle",
		Type:         "history",
		Entry: []NotificationEntry{
			{
				Resource: map[string]interface{}{
					"resourceType": "Patient",
					"id":           "pat-123",
				},
				Request: &EntryRequest{Method: "POST", URL: "Patient"},
			},
		},
	}
	body, _ := json.Marshal(bundle)

	req := httptest.NewRequest("POST", "/fhir/notify/test_sub", strings.NewReader(string(body)))
	w := httptest.NewRecorder()

	receiver.ServeHTTP(w, req)

	// Verify metrics were recorded
	if len(metrics.receivedCalls) != 1 {
		t.Errorf("Expected 1 received call, got %d", len(metrics.receivedCalls))
	}
	if metrics.receivedCalls[0] != "test_sub:Patient" {
		t.Errorf("Expected 'test_sub:Patient', got '%s'", metrics.receivedCalls[0])
	}

	if len(metrics.processedCalls) != 1 {
		t.Errorf("Expected 1 processed call, got %d", len(metrics.processedCalls))
	}
	if metrics.processedCalls[0].subscription != "test_sub" {
		t.Errorf("Expected subscription 'test_sub', got '%s'", metrics.processedCalls[0].subscription)
	}
}

func TestReceiver_MetricsOnError(t *testing.T) {
	metrics := &mockReceiverMetrics{}
	router := NoOpRouter{}
	receiver := NewReceiver(router, &ReceiverOptions{
		Metrics:       metrics,
		MaxBundleSize: 1,
	})

	receiver.RegisterSubscription(&SubscriptionConfig{
		Name: "test_sub",
	})

	// Create bundle that's too large
	bundle := NotificationBundle{
		ResourceType: "Bundle",
		Type:         "history",
		Entry: []NotificationEntry{
			{Resource: map[string]interface{}{"resourceType": "Patient", "id": "1"}},
			{Resource: map[string]interface{}{"resourceType": "Patient", "id": "2"}},
		},
	}
	body, _ := json.Marshal(bundle)

	req := httptest.NewRequest("POST", "/fhir/notify/test_sub", strings.NewReader(string(body)))
	w := httptest.NewRecorder()

	receiver.ServeHTTP(w, req)

	// Verify error metric was recorded
	if len(metrics.errorCalls) != 1 {
		t.Fatalf("Expected 1 error call, got %d", len(metrics.errorCalls))
	}
	if metrics.errorCalls[0].errorType != "bundle_too_large" {
		t.Errorf("Expected errorType 'bundle_too_large', got '%s'", metrics.errorCalls[0].errorType)
	}
}

// =============================================================================
// Receiver Handler Tests
// =============================================================================

func TestReceiver_Handler(t *testing.T) {
	router := NoOpRouter{}
	receiver := NewReceiver(router, &ReceiverOptions{
		PathPrefix: "/custom/path",
	})

	handler := receiver.Handler()
	if handler == nil {
		t.Error("Handler returned nil")
	}

	// Verify health endpoint works
	req := httptest.NewRequest("GET", "/custom/path/health", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

// =============================================================================
// Receiver with Routing Errors
// =============================================================================

func TestReceiver_RoutingErrorStillReturns200(t *testing.T) {
	// Router that always fails
	router := NewCallbackRouter(func(ctx context.Context, event interface{}) error {
		return context.DeadlineExceeded
	})

	receiver := NewReceiver(router, nil)
	receiver.RegisterSubscription(&SubscriptionConfig{
		Name: "test_sub",
	})

	bundle := NotificationBundle{
		ResourceType: "Bundle",
		Type:         "history",
		Entry: []NotificationEntry{
			{
				Resource: map[string]interface{}{
					"resourceType": "Patient",
					"id":           "pat-123",
					"name":         []interface{}{map[string]interface{}{"family": "Test"}},
				},
				Request: &EntryRequest{Method: "POST", URL: "Patient"},
			},
		},
	}
	body, _ := json.Marshal(bundle)

	req := httptest.NewRequest("POST", "/fhir/notify/test_sub", strings.NewReader(string(body)))
	w := httptest.NewRecorder()

	receiver.ServeHTTP(w, req)

	// Should still return 200 to prevent FHIR server from retrying entire bundle
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200 even on routing error, got %d: %s", w.Code, w.Body.String())
	}
}

// =============================================================================
// Receiver AllowedSources Trailing Slash Handling
// =============================================================================

func TestReceiver_AllowedSourcesTrailingSlash(t *testing.T) {
	router := NoOpRouter{}
	receiver := NewReceiver(router, &ReceiverOptions{
		AllowedSources: []string{
			"https://example.com/", // With trailing slash
		},
	})

	// The trailing slash should be trimmed during initialization
	// so both with and without slash should match
	if !receiver.isAllowedSource("https://example.com", nil) {
		t.Error("Source without trailing slash should match")
	}
	if !receiver.isAllowedSource("https://example.com/path", nil) {
		t.Error("Source with path should match")
	}
}
