//nolint:gosec // Test file - G104 errors intentionally ignored in test setup
package subscription

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/crb2nu/fi-fhir/pkg/events"
)

func TestFHIRMapper_MapPatient(t *testing.T) {
	mapper := NewFHIRMapper()

	resource := map[string]interface{}{
		"resourceType": "Patient",
		"id":           "pat-123",
		"identifier": []interface{}{
			map[string]interface{}{
				"value":  "MRN12345",
				"system": "http://hospital.example.org/mrn",
				"type": map[string]interface{}{
					"coding": []interface{}{
						map[string]interface{}{
							"code": "MR",
						},
					},
				},
			},
		},
		"name": []interface{}{
			map[string]interface{}{
				"family": "Smith",
				"given":  []interface{}{"John", "Robert"},
			},
		},
		"birthDate": "1985-03-15",
		"gender":    "male",
	}

	event, err := mapper.MapResource(resource, "create", nil)
	if err != nil {
		t.Fatalf("MapResource failed: %v", err)
	}

	if event == nil {
		t.Fatal("Expected event, got nil")
	}

	patientEvent, ok := event.(*PatientEvent)
	if !ok {
		t.Fatalf("Expected *PatientEvent, got %T", event)
	}

	if patientEvent.Type != "patient_created" {
		t.Errorf("Expected type patient_created, got %s", patientEvent.Type)
	}

	if patientEvent.Patient.FamilyName != "Smith" {
		t.Errorf("Expected family name Smith, got %s", patientEvent.Patient.FamilyName)
	}

	if patientEvent.Patient.GivenName != "John" {
		t.Errorf("Expected given name John, got %s", patientEvent.Patient.GivenName)
	}

	if patientEvent.Patient.MRN != "MRN12345" {
		t.Errorf("Expected MRN MRN12345, got %s", patientEvent.Patient.MRN)
	}
}

func TestFHIRMapper_MapEncounter(t *testing.T) {
	mapper := NewFHIRMapper()

	tests := []struct {
		name         string
		status       string
		class        string
		expectedType events.EventType
	}{
		{"inpatient_admit", "in-progress", "IMP", events.EventPatientAdmit},
		{"inpatient_discharge", "finished", "IMP", events.EventPatientDischarge},
		{"outpatient_visit", "in-progress", "AMB", events.EventPatientAdmit},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resource := map[string]interface{}{
				"resourceType": "Encounter",
				"id":           "enc-123",
				"status":       tt.status,
				"class": map[string]interface{}{
					"code": tt.class,
				},
				"subject": map[string]interface{}{
					"reference": "Patient/pat-456",
					"display":   "John Smith",
				},
			}

			event, err := mapper.MapResource(resource, "update", nil)
			if err != nil {
				t.Fatalf("MapResource failed: %v", err)
			}

			if event == nil {
				t.Fatal("Expected event, got nil")
			}

			// Check event type
			switch e := event.(type) {
			case *events.PatientAdmitEvent:
				if tt.expectedType != events.EventPatientAdmit {
					t.Errorf("Expected %s, got PatientAdmitEvent", tt.expectedType)
				}
			case *events.PatientDischargeEvent:
				if tt.expectedType != events.EventPatientDischarge {
					t.Errorf("Expected %s, got PatientDischargeEvent", tt.expectedType)
				}
			default:
				t.Errorf("Unexpected event type: %T", e)
			}
		})
	}
}

func TestFHIRMapper_MapObservation(t *testing.T) {
	mapper := NewFHIRMapper()

	resource := map[string]interface{}{
		"resourceType": "Observation",
		"id":           "obs-123",
		"status":       "final",
		"category": []interface{}{
			map[string]interface{}{
				"coding": []interface{}{
					map[string]interface{}{
						"code": "laboratory",
					},
				},
			},
		},
		"code": map[string]interface{}{
			"coding": []interface{}{
				map[string]interface{}{
					"system":  "http://loinc.org",
					"code":    "2093-3",
					"display": "Cholesterol [Mass/volume] in Serum or Plasma",
				},
			},
			"text": "Cholesterol",
		},
		"subject": map[string]interface{}{
			"reference": "Patient/pat-456",
		},
		"valueQuantity": map[string]interface{}{
			"value": 185.0,
			"unit":  "mg/dL",
		},
		"interpretation": []interface{}{
			map[string]interface{}{
				"coding": []interface{}{
					map[string]interface{}{
						"code": "N",
					},
				},
			},
		},
	}

	event, err := mapper.MapResource(resource, "create", nil)
	if err != nil {
		t.Fatalf("MapResource failed: %v", err)
	}

	if event == nil {
		t.Fatal("Expected event, got nil")
	}

	labEvent, ok := event.(*events.LabResultEvent)
	if !ok {
		t.Fatalf("Expected *LabResultEvent, got %T", event)
	}

	if labEvent.Type != events.EventLabResult {
		t.Errorf("Expected type lab_result, got %s", labEvent.Type)
	}

	if labEvent.Result.Value != "185" {
		t.Errorf("Expected value 185, got %s", labEvent.Result.Value)
	}

	if labEvent.Test.LOINCCode != "2093-3" {
		t.Errorf("Expected LOINC code 2093-3, got %s", labEvent.Test.LOINCCode)
	}
}

func TestFHIRMapper_MapAppointment(t *testing.T) {
	mapper := NewFHIRMapper()

	tests := []struct {
		name         string
		status       string
		expectedType events.EventType
	}{
		{"booked", "booked", events.EventAppointmentScheduled},
		{"cancelled", "cancelled", events.EventAppointmentCancelled},
		{"noshow", "noshow", events.EventAppointmentNoShow},
		{"checked_in", "checked-in", events.EventAppointmentCheckedIn},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resource := map[string]interface{}{
				"resourceType": "Appointment",
				"id":           "appt-123",
				"status":       tt.status,
				"start":        "2024-01-15T10:00:00Z",
				"end":          "2024-01-15T10:30:00Z",
				"participant": []interface{}{
					map[string]interface{}{
						"actor": map[string]interface{}{
							"reference": "Patient/pat-456",
							"display":   "John Smith",
						},
					},
				},
			}

			event, err := mapper.MapResource(resource, "update", nil)
			if err != nil {
				t.Fatalf("MapResource failed: %v", err)
			}

			if event == nil {
				t.Fatal("Expected event, got nil")
			}

			apptEvent, ok := event.(*events.AppointmentEvent)
			if !ok {
				t.Fatalf("Expected *AppointmentEvent, got %T", event)
			}

			if apptEvent.Type != tt.expectedType {
				t.Errorf("Expected type %s, got %s", tt.expectedType, apptEvent.Type)
			}
		})
	}
}

func TestReceiver_HandleNotification(t *testing.T) {
	// Create a mock router that captures events
	var capturedEvents []interface{}
	router := NewCallbackRouter(func(ctx context.Context, event interface{}) error {
		capturedEvents = append(capturedEvents, event)
		return nil
	})

	receiver := NewReceiver(router, &ReceiverOptions{
		PathPrefix:    "/fhir/notify",
		MaxBundleSize: 10,
	})

	// Register subscription
	receiver.RegisterSubscription(&SubscriptionConfig{
		Name: "test_sub",
	})

	// Create test notification bundle
	bundle := NotificationBundle{
		ResourceType: "Bundle",
		Type:         "history",
		Entry: []NotificationEntry{
			{
				Resource: map[string]interface{}{
					"resourceType": "Patient",
					"id":           "pat-123",
					"name": []interface{}{
						map[string]interface{}{
							"family": "Test",
							"given":  []interface{}{"User"},
						},
					},
				},
				Request: &EntryRequest{
					Method: "POST",
					URL:    "Patient/pat-123",
				},
			},
		},
	}

	body, _ := json.Marshal(bundle)

	// Create request
	req := httptest.NewRequest("POST", "/fhir/notify/test_sub", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/fhir+json")

	// Create response recorder
	w := httptest.NewRecorder()

	// Handle request
	receiver.ServeHTTP(w, req)

	// Check response
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	// Check captured events
	if len(capturedEvents) != 1 {
		t.Errorf("Expected 1 event, got %d", len(capturedEvents))
	}
}

func TestReceiver_UnknownSubscription(t *testing.T) {
	router := NoOpRouter{}
	receiver := NewReceiver(router, nil)

	req := httptest.NewRequest("POST", "/fhir/notify/unknown", strings.NewReader("{}"))
	w := httptest.NewRecorder()

	receiver.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", w.Code)
	}
}

func TestReceiver_InvalidBundle(t *testing.T) {
	router := NoOpRouter{}
	receiver := NewReceiver(router, nil)

	receiver.RegisterSubscription(&SubscriptionConfig{
		Name: "test_sub",
	})

	// Send invalid JSON
	req := httptest.NewRequest("POST", "/fhir/notify/test_sub", strings.NewReader("not json"))
	w := httptest.NewRecorder()

	receiver.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

func TestReceiver_BundleTooLarge(t *testing.T) {
	router := NoOpRouter{}
	receiver := NewReceiver(router, &ReceiverOptions{
		MaxBundleSize: 2,
	})

	receiver.RegisterSubscription(&SubscriptionConfig{
		Name: "test_sub",
	})

	// Create bundle with too many entries
	bundle := NotificationBundle{
		ResourceType: "Bundle",
		Type:         "history",
		Entry: []NotificationEntry{
			{Resource: map[string]interface{}{"resourceType": "Patient", "id": "1"}},
			{Resource: map[string]interface{}{"resourceType": "Patient", "id": "2"}},
			{Resource: map[string]interface{}{"resourceType": "Patient", "id": "3"}},
		},
	}

	body, _ := json.Marshal(bundle)
	req := httptest.NewRequest("POST", "/fhir/notify/test_sub", strings.NewReader(string(body)))
	w := httptest.NewRecorder()

	receiver.ServeHTTP(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("Expected status 413, got %d", w.Code)
	}
}

func TestClient_Create(t *testing.T) {
	// Create mock FHIR server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/Subscription" {
			t.Errorf("Unexpected request: %s %s", r.Method, r.URL.Path)
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		// Return created subscription
		w.Header().Set("Content-Type", "application/fhir+json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(Subscription{
			ResourceType: "Subscription",
			ID:           "sub-123",
			Status:       StatusActive,
			Criteria:     "Patient",
			Channel: Channel{
				Type:     ChannelRestHook,
				Endpoint: "https://example.com/notify",
			},
		})
	}))
	defer server.Close()

	client, err := NewClient(&ClientConfig{
		FHIREndpoint: server.URL,
	})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	sub, err := client.Create(context.Background(), &Subscription{
		Criteria: "Patient",
		Channel: Channel{
			Type:     ChannelRestHook,
			Endpoint: "https://example.com/notify",
		},
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if sub.ID != "sub-123" {
		t.Errorf("Expected ID sub-123, got %s", sub.ID)
	}

	if sub.Status != StatusActive {
		t.Errorf("Expected status active, got %s", sub.Status)
	}
}

func TestMultiRouter(t *testing.T) {
	var calls []string

	router1 := NewCallbackRouter(func(ctx context.Context, event interface{}) error {
		calls = append(calls, "router1")
		return nil
	})

	router2 := NewCallbackRouter(func(ctx context.Context, event interface{}) error {
		calls = append(calls, "router2")
		return nil
	})

	multi := NewMultiRouter(MultiRouterAll, router1, router2)

	err := multi.Route(context.Background(), "test event")
	if err != nil {
		t.Fatalf("Route failed: %v", err)
	}

	if len(calls) != 2 {
		t.Errorf("Expected 2 calls, got %d", len(calls))
	}

	if calls[0] != "router1" || calls[1] != "router2" {
		t.Errorf("Unexpected call order: %v", calls)
	}
}

func TestFilterRouter(t *testing.T) {
	var capturedEvents []interface{}

	inner := NewCallbackRouter(func(ctx context.Context, event interface{}) error {
		capturedEvents = append(capturedEvents, event)
		return nil
	})

	// Filter that only passes strings containing "pass"
	filter := NewFilterRouter(inner, func(event interface{}) bool {
		s, ok := event.(string)
		return ok && strings.Contains(s, "pass")
	})

	// Should pass
	filter.Route(context.Background(), "should pass")
	// Should be filtered
	filter.Route(context.Background(), "should fail")
	// Should pass
	filter.Route(context.Background(), "also pass")

	if len(capturedEvents) != 2 {
		t.Errorf("Expected 2 events, got %d", len(capturedEvents))
	}
}

// =============================================================================
// Client CRUD Tests
// =============================================================================

func TestClient_Get(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("Expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/Subscription/sub-123" {
			t.Errorf("Expected /Subscription/sub-123, got %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/fhir+json")
		json.NewEncoder(w).Encode(Subscription{
			ResourceType: "Subscription",
			ID:           "sub-123",
			Status:       StatusActive,
			Criteria:     "Patient",
		})
	}))
	defer server.Close()

	client, _ := NewClient(&ClientConfig{FHIREndpoint: server.URL})

	sub, err := client.Get(context.Background(), "sub-123")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if sub.ID != "sub-123" {
		t.Errorf("Expected ID sub-123, got %s", sub.ID)
	}
}

func TestClient_Get_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client, _ := NewClient(&ClientConfig{FHIREndpoint: server.URL})

	_, err := client.Get(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("Expected ErrNotFound, got %v", err)
	}
}

func TestClient_List(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/Subscription" {
			t.Errorf("Unexpected request: %s %s", r.Method, r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/fhir+json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"resourceType": "Bundle",
			"entry": []map[string]interface{}{
				{
					"resource": Subscription{
						ResourceType: "Subscription",
						ID:           "sub-1",
						Status:       StatusActive,
					},
				},
				{
					"resource": Subscription{
						ResourceType: "Subscription",
						ID:           "sub-2",
						Status:       StatusOff,
					},
				},
			},
		})
	}))
	defer server.Close()

	client, _ := NewClient(&ClientConfig{FHIREndpoint: server.URL})

	subs, err := client.List(context.Background())
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(subs) != 2 {
		t.Errorf("Expected 2 subscriptions, got %d", len(subs))
	}
}

func TestClient_Delete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" {
			t.Errorf("Expected DELETE, got %s", r.Method)
		}
		if r.URL.Path != "/Subscription/sub-123" {
			t.Errorf("Expected /Subscription/sub-123, got %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client, _ := NewClient(&ClientConfig{FHIREndpoint: server.URL})

	err := client.Delete(context.Background(), "sub-123")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
}

func TestClient_Delete_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client, _ := NewClient(&ClientConfig{FHIREndpoint: server.URL})

	err := client.Delete(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("Expected ErrNotFound, got %v", err)
	}
}

func TestClient_Pause(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			// First call: Get
			if r.Method != "GET" {
				t.Errorf("Expected GET, got %s", r.Method)
			}
			w.Header().Set("Content-Type", "application/fhir+json")
			json.NewEncoder(w).Encode(Subscription{
				ResourceType: "Subscription",
				ID:           "sub-123",
				Status:       StatusActive,
				Criteria:     "Patient",
			})
		} else {
			// Second call: Put
			if r.Method != "PUT" {
				t.Errorf("Expected PUT, got %s", r.Method)
			}

			var sub Subscription
			json.NewDecoder(r.Body).Decode(&sub)
			if sub.Status != StatusOff {
				t.Errorf("Expected status 'off', got '%s'", sub.Status)
			}

			w.Header().Set("Content-Type", "application/fhir+json")
			json.NewEncoder(w).Encode(sub)
		}
	}))
	defer server.Close()

	client, _ := NewClient(&ClientConfig{FHIREndpoint: server.URL})

	err := client.Pause(context.Background(), "sub-123")
	if err != nil {
		t.Fatalf("Pause failed: %v", err)
	}
}

func TestClient_Resume(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			// First call: Get
			w.Header().Set("Content-Type", "application/fhir+json")
			json.NewEncoder(w).Encode(Subscription{
				ResourceType: "Subscription",
				ID:           "sub-123",
				Status:       StatusOff,
				Criteria:     "Patient",
			})
		} else {
			// Second call: Put
			var sub Subscription
			json.NewDecoder(r.Body).Decode(&sub)
			if sub.Status != StatusRequested {
				t.Errorf("Expected status 'requested', got '%s'", sub.Status)
			}

			w.Header().Set("Content-Type", "application/fhir+json")
			json.NewEncoder(w).Encode(sub)
		}
	}))
	defer server.Close()

	client, _ := NewClient(&ClientConfig{FHIREndpoint: server.URL})

	err := client.Resume(context.Background(), "sub-123")
	if err != nil {
		t.Fatalf("Resume failed: %v", err)
	}
}

func TestClient_ParseError_OperationOutcome(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/fhir+json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"resourceType": "OperationOutcome",
			"issue": []map[string]interface{}{
				{
					"severity":    "error",
					"code":        "invalid",
					"diagnostics": "Invalid subscription criteria",
				},
			},
		})
	}))
	defer server.Close()

	client, _ := NewClient(&ClientConfig{FHIREndpoint: server.URL})

	_, err := client.Get(context.Background(), "bad-sub")
	if err == nil {
		t.Fatal("Expected error")
	}

	var fhirErr *FHIRError
	if !errors.As(err, &fhirErr) {
		t.Fatalf("Expected *FHIRError, got %T", err)
	}

	if fhirErr.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", fhirErr.StatusCode)
	}
	if fhirErr.Diagnostics != "Invalid subscription criteria" {
		t.Errorf("Unexpected diagnostics: %s", fhirErr.Diagnostics)
	}
}

func TestClient_WithAuth(t *testing.T) {
	var receivedAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/fhir+json")
		json.NewEncoder(w).Encode(Subscription{ID: "sub-123"})
	}))
	defer server.Close()

	client, _ := NewClient(&ClientConfig{
		FHIREndpoint: server.URL,
		AuthProvider: &StaticTokenAuth{Token: "test-token"},
	})

	client.Get(context.Background(), "sub-123")

	if receivedAuth != "Bearer test-token" {
		t.Errorf("Expected 'Bearer test-token', got '%s'", receivedAuth)
	}
}

func TestFHIRError_Error(t *testing.T) {
	err := &FHIRError{
		StatusCode:  400,
		Severity:    "error",
		Code:        "invalid",
		Diagnostics: "Something went wrong",
	}

	msg := err.Error()
	if !strings.Contains(msg, "400") {
		t.Errorf("Error message should contain status code: %s", msg)
	}
	if !strings.Contains(msg, "Something went wrong") {
		t.Errorf("Error message should contain diagnostics: %s", msg)
	}
}

func TestNewClient_EmptyEndpoint(t *testing.T) {
	_, err := NewClient(&ClientConfig{FHIREndpoint: ""})
	if err == nil {
		t.Error("Expected error for empty endpoint")
	}
}

// =============================================================================
// Config Tests
// =============================================================================

// =============================================================================
// Config Utility Function Tests
// =============================================================================

func TestExpandEnvVars(t *testing.T) {
	// Set test environment variables
	t.Setenv("TEST_VAR", "hello")
	t.Setenv("TEST_SECRET", "secret123")

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple variable",
			input:    "value: ${TEST_VAR}",
			expected: "value: hello",
		},
		{
			name:     "variable with default - var exists",
			input:    "value: ${TEST_VAR:-default}",
			expected: "value: hello",
		},
		{
			name:     "variable with default - var missing",
			input:    "value: ${MISSING_VAR:-fallback}",
			expected: "value: fallback",
		},
		{
			name:     "missing variable no default",
			input:    "value: ${MISSING_VAR}",
			expected: "value: ",
		},
		{
			name:     "multiple variables",
			input:    "${TEST_VAR} and ${TEST_SECRET}",
			expected: "hello and secret123",
		},
		{
			name:     "no variables",
			input:    "plain text",
			expected: "plain text",
		},
		{
			name:     "empty default",
			input:    "${MISSING:-}",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := expandEnvVars(tt.input)
			if result != tt.expected {
				t.Errorf("expandEnvVars(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestDefaultReceiverConfig(t *testing.T) {
	cfg := DefaultReceiverConfig()

	if cfg == nil {
		t.Fatal("DefaultReceiverConfig returned nil")
	}

	// Check default values
	if cfg.Enabled {
		t.Error("Expected Enabled to be false by default")
	}
	if cfg.Host != "0.0.0.0" {
		t.Errorf("Expected Host 0.0.0.0, got %s", cfg.Host)
	}
	if cfg.Port != 8081 {
		t.Errorf("Expected Port 8081, got %d", cfg.Port)
	}
	if cfg.PathPrefix != "/fhir/notify" {
		t.Errorf("Expected PathPrefix /fhir/notify, got %s", cfg.PathPrefix)
	}
	if cfg.MaxBundleSize != 100 {
		t.Errorf("Expected MaxBundleSize 100, got %d", cfg.MaxBundleSize)
	}
	if cfg.Timeout != "30s" {
		t.Errorf("Expected Timeout 30s, got %s", cfg.Timeout)
	}
	if !cfg.Retry.Enabled {
		t.Error("Expected Retry.Enabled to be true")
	}
	if cfg.Retry.MaxAttempts != 3 {
		t.Errorf("Expected Retry.MaxAttempts 3, got %d", cfg.Retry.MaxAttempts)
	}
}

func TestBuildEndpointURL(t *testing.T) {
	tests := []struct {
		name       string
		baseURL    string
		pathPrefix string
		subName    string
		expected   string
	}{
		{
			name:       "basic",
			baseURL:    "https://example.com",
			pathPrefix: "fhir/notify",
			subName:    "patient_updates",
			expected:   "https://example.com/fhir/notify/patient_updates",
		},
		{
			name:       "trailing slash on base",
			baseURL:    "https://example.com/",
			pathPrefix: "fhir/notify",
			subName:    "test",
			expected:   "https://example.com/fhir/notify/test",
		},
		{
			name:       "leading slash on prefix",
			baseURL:    "https://example.com",
			pathPrefix: "/fhir/notify",
			subName:    "test",
			expected:   "https://example.com/fhir/notify/test",
		},
		{
			name:       "both slashes",
			baseURL:    "https://example.com/",
			pathPrefix: "/fhir/notify/",
			subName:    "test",
			expected:   "https://example.com/fhir/notify/test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildEndpointURL(tt.baseURL, tt.pathPrefix, tt.subName)
			if result != tt.expected {
				t.Errorf("BuildEndpointURL(%q, %q, %q) = %q, want %q",
					tt.baseURL, tt.pathPrefix, tt.subName, result, tt.expected)
			}
		})
	}
}

func TestMergeReceiverConfig(t *testing.T) {
	dst := &ReceiverConfig{
		Host:          "0.0.0.0",
		Port:          8081,
		PathPrefix:    "/default",
		MaxBundleSize: 100,
		Timeout:       "30s",
	}

	src := &ReceiverConfig{
		Enabled:        true,
		Host:           "127.0.0.1",
		Port:           9090,
		PathPrefix:     "",    // Empty should NOT override
		MaxBundleSize:  0,     // Zero should NOT override
		Timeout:        "60s", // Non-empty should override
		VerifySource:   true,
		AllowedSources: []string{"10.0.0.0/8"},
		TLS: TLSConfig{
			Enabled:  true,
			CertFile: "/path/to/cert",
			KeyFile:  "/path/to/key",
		},
	}

	mergeReceiverConfig(dst, src)

	if !dst.Enabled {
		t.Error("Expected Enabled to be true after merge")
	}
	if dst.Host != "127.0.0.1" {
		t.Errorf("Expected Host 127.0.0.1, got %s", dst.Host)
	}
	if dst.Port != 9090 {
		t.Errorf("Expected Port 9090, got %d", dst.Port)
	}
	if dst.PathPrefix != "/default" {
		t.Errorf("Expected PathPrefix to remain /default, got %s", dst.PathPrefix)
	}
	if dst.MaxBundleSize != 100 {
		t.Errorf("Expected MaxBundleSize to remain 100, got %d", dst.MaxBundleSize)
	}
	if dst.Timeout != "60s" {
		t.Errorf("Expected Timeout 60s, got %s", dst.Timeout)
	}
	if !dst.VerifySource {
		t.Error("Expected VerifySource to be true")
	}
	if len(dst.AllowedSources) != 1 || dst.AllowedSources[0] != "10.0.0.0/8" {
		t.Errorf("Expected AllowedSources [10.0.0.0/8], got %v", dst.AllowedSources)
	}
	if !dst.TLS.Enabled {
		t.Error("Expected TLS.Enabled to be true")
	}
}

func TestLoadConfig(t *testing.T) {
	t.Setenv("FHIR_SERVER", "https://fhir.example.com")
	t.Setenv("CALLBACK_URL", "https://callback.example.com/notify")

	// Create temp config file
	content := `subscriptions:
  - name: patient_updates
    server: ${FHIR_SERVER}
    criteria: Patient
    channel:
      endpoint: ${CALLBACK_URL}
  - name: encounter_events
    server: ${FHIR_SERVER:-https://default.com}
    criteria: Encounter
    channel:
      endpoint: https://static.example.com/notify
`

	tmpFile, err := os.CreateTemp("", "subscriptions-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()

	config, err := LoadConfig(tmpFile.Name())
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if len(config.Subscriptions) != 2 {
		t.Fatalf("Expected 2 subscriptions, got %d", len(config.Subscriptions))
	}

	// Verify env var expansion
	if config.Subscriptions[0].Server != "https://fhir.example.com" {
		t.Errorf("Expected server from env var, got %s", config.Subscriptions[0].Server)
	}
	if config.Subscriptions[0].Channel.Endpoint != "https://callback.example.com/notify" {
		t.Errorf("Expected endpoint from env var, got %s", config.Subscriptions[0].Channel.Endpoint)
	}
}

func TestLoadConfig_FileNotFound(t *testing.T) {
	_, err := LoadConfig("/nonexistent/path/config.yaml")
	if err == nil {
		t.Error("Expected error for nonexistent file")
	}
}

func TestLoadConfig_InvalidYAML(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "invalid-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	tmpFile.WriteString("not: valid: yaml: {{")
	tmpFile.Close()

	_, err = LoadConfig(tmpFile.Name())
	if err == nil {
		t.Error("Expected error for invalid YAML")
	}
}

func TestLoadConfig_ValidationError(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "invalid-config-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	// Missing required name field
	content := `subscriptions:
  - server: https://fhir.example.com
    criteria: Patient
    channel:
      endpoint: https://callback.example.com/notify
`
	tmpFile.WriteString(content)
	tmpFile.Close()

	_, err = LoadConfig(tmpFile.Name())
	if err == nil {
		t.Error("Expected validation error for missing name")
	}
}

func TestLoadFullConfig(t *testing.T) {
	t.Setenv("FHIR_SERVER", "https://fhir.example.com")

	// Create subscriptions file
	subContent := `subscriptions:
  - name: test_sub
    server: ${FHIR_SERVER}
    criteria: Patient
    channel:
      endpoint: https://callback.example.com/notify
`
	subFile, err := os.CreateTemp("", "subscriptions-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(subFile.Name())
	subFile.WriteString(subContent)
	subFile.Close()

	// Create receiver config file
	recvContent := `subscription_receiver:
  enabled: true
  host: 127.0.0.1
  port: 9000
  path_prefix: /custom/path
  timeout: 60s
`
	recvFile, err := os.CreateTemp("", "receiver-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(recvFile.Name())
	recvFile.WriteString(recvContent)
	recvFile.Close()

	config, err := LoadFullConfig(subFile.Name(), recvFile.Name())
	if err != nil {
		t.Fatalf("LoadFullConfig failed: %v", err)
	}

	// Check subscriptions loaded
	if len(config.Subscriptions) != 1 {
		t.Errorf("Expected 1 subscription, got %d", len(config.Subscriptions))
	}

	// Check receiver config merged
	if !config.Receiver.Enabled {
		t.Error("Expected Receiver.Enabled to be true")
	}
	if config.Receiver.Host != "127.0.0.1" {
		t.Errorf("Expected Host 127.0.0.1, got %s", config.Receiver.Host)
	}
	if config.Receiver.Port != 9000 {
		t.Errorf("Expected Port 9000, got %d", config.Receiver.Port)
	}
	if config.Receiver.PathPrefix != "/custom/path" {
		t.Errorf("Expected PathPrefix /custom/path, got %s", config.Receiver.PathPrefix)
	}
}

func TestLoadFullConfig_NoFiles(t *testing.T) {
	// Should return defaults when no files provided
	config, err := LoadFullConfig("", "")
	if err != nil {
		t.Fatalf("LoadFullConfig failed: %v", err)
	}

	if len(config.Subscriptions) != 0 {
		t.Errorf("Expected 0 subscriptions, got %d", len(config.Subscriptions))
	}

	// Should have default receiver config
	if config.Receiver.Port != 8081 {
		t.Errorf("Expected default port 8081, got %d", config.Receiver.Port)
	}
}

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: Config{
				Subscriptions: []SubscriptionDefinition{
					{
						Name:     "test",
						Server:   "https://fhir.example.com",
						Criteria: "Patient",
						Channel: ChannelConfig{
							Endpoint: "https://callback.example.com/notify",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "missing name",
			config: Config{
				Subscriptions: []SubscriptionDefinition{
					{
						Server:   "https://fhir.example.com",
						Criteria: "Patient",
						Channel: ChannelConfig{
							Endpoint: "https://callback.example.com/notify",
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "missing server",
			config: Config{
				Subscriptions: []SubscriptionDefinition{
					{
						Name:     "test",
						Criteria: "Patient",
						Channel: ChannelConfig{
							Endpoint: "https://callback.example.com/notify",
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "duplicate names",
			config: Config{
				Subscriptions: []SubscriptionDefinition{
					{
						Name:     "test",
						Server:   "https://fhir.example.com",
						Criteria: "Patient",
						Channel:  ChannelConfig{Endpoint: "https://example.com/a"},
					},
					{
						Name:     "test",
						Server:   "https://fhir.example.com",
						Criteria: "Encounter",
						Channel:  ChannelConfig{Endpoint: "https://example.com/b"},
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// =============================================================================
// Mapper Tests
// =============================================================================

func TestFHIRMapper_RegisterMapper(t *testing.T) {
	mapper := NewFHIRMapper()

	// Create a custom mapper that always returns a specific event
	customMapper := &mockMapper{
		mapFunc: func(resource map[string]interface{}, action string, config *EventMappingConfig) (interface{}, error) {
			return &PatientEvent{
				EventMeta: events.NewEventMeta("custom_event", "test", events.FormatFHIR),
			}, nil
		},
	}

	// Register for a new resource type
	mapper.RegisterMapper("CustomResource", customMapper)

	// Test that the custom mapper is used
	event, err := mapper.MapResource(map[string]interface{}{
		"resourceType": "CustomResource",
		"id":           "custom-123",
	}, "create", nil)

	if err != nil {
		t.Fatalf("MapResource failed: %v", err)
	}
	if event == nil {
		t.Fatal("Expected event, got nil")
	}

	patientEvent, ok := event.(*PatientEvent)
	if !ok {
		t.Fatalf("Expected *PatientEvent, got %T", event)
	}
	if patientEvent.Type != "custom_event" {
		t.Errorf("Expected type custom_event, got %s", patientEvent.Type)
	}
}

func TestFHIRMapper_RegisterMapper_Override(t *testing.T) {
	mapper := NewFHIRMapper()

	// Override the built-in Patient mapper
	customMapper := &mockMapper{
		mapFunc: func(resource map[string]interface{}, action string, config *EventMappingConfig) (interface{}, error) {
			return &PatientEvent{
				EventMeta: events.NewEventMeta("overridden_patient", "test", events.FormatFHIR),
			}, nil
		},
	}

	mapper.RegisterMapper("Patient", customMapper)

	// Test that the custom mapper overrides the built-in
	event, err := mapper.MapResource(map[string]interface{}{
		"resourceType": "Patient",
		"id":           "pat-123",
	}, "create", nil)

	if err != nil {
		t.Fatalf("MapResource failed: %v", err)
	}

	patientEvent := event.(*PatientEvent)
	if patientEvent.Type != "overridden_patient" {
		t.Errorf("Expected type overridden_patient, got %s", patientEvent.Type)
	}
}

func TestFHIRMapper_MapBundle(t *testing.T) {
	mapper := NewFHIRMapper()

	bundle := &NotificationBundle{
		ResourceType: "Bundle",
		Type:         "history",
		Entry: []NotificationEntry{
			{
				Resource: map[string]interface{}{
					"resourceType": "Patient",
					"id":           "pat-1",
					"name":         []interface{}{map[string]interface{}{"family": "Smith"}},
				},
				Request: &EntryRequest{Method: "POST", URL: "Patient"},
			},
			{
				Resource: map[string]interface{}{
					"resourceType": "Patient",
					"id":           "pat-2",
					"name":         []interface{}{map[string]interface{}{"family": "Jones"}},
				},
				Request: &EntryRequest{Method: "PUT", URL: "Patient/pat-2"},
			},
			{
				Resource: map[string]interface{}{
					"resourceType": "Patient",
					"id":           "pat-3",
				},
				Request: &EntryRequest{Method: "DELETE", URL: "Patient/pat-3"},
			},
		},
	}

	mappedEvents, err := mapper.MapBundle(bundle, nil)
	if err != nil {
		t.Fatalf("MapBundle failed: %v", err)
	}

	if len(mappedEvents) != 3 {
		t.Fatalf("Expected 3 events, got %d", len(mappedEvents))
	}

	// Check first event (POST -> create)
	pe1 := mappedEvents[0].(*PatientEvent)
	if pe1.Type != "patient_created" {
		t.Errorf("Expected patient_created, got %s", pe1.Type)
	}

	// Check second event (PUT -> update)
	pe2 := mappedEvents[1].(*PatientEvent)
	if pe2.Type != events.EventPatientUpdate {
		t.Errorf("Expected patient_update, got %s", pe2.Type)
	}

	// Check third event (DELETE -> delete)
	pe3 := mappedEvents[2].(*PatientEvent)
	if pe3.Type != "patient_deleted" {
		t.Errorf("Expected patient_deleted, got %s", pe3.Type)
	}
}

func TestFHIRMapper_MapBundle_WithConfig(t *testing.T) {
	mapper := NewFHIRMapper()

	bundle := &NotificationBundle{
		ResourceType: "Bundle",
		Type:         "history",
		Entry: []NotificationEntry{
			{
				Resource: map[string]interface{}{
					"resourceType": "Patient",
					"id":           "pat-1",
				},
				Request: &EntryRequest{Method: "POST", URL: "Patient"},
			},
		},
	}

	config := &EventMappingConfig{
		CreateEvent: "custom_patient_created",
	}

	mappedEvents, err := mapper.MapBundle(bundle, config)
	if err != nil {
		t.Fatalf("MapBundle failed: %v", err)
	}

	if len(mappedEvents) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(mappedEvents))
	}

	pe := mappedEvents[0].(*PatientEvent)
	if pe.Type != "custom_patient_created" {
		t.Errorf("Expected custom_patient_created, got %s", pe.Type)
	}
}

func TestFHIRMapper_MapBundle_NoRequestMethod(t *testing.T) {
	mapper := NewFHIRMapper()

	// Bundle entry without Request (defaults to update)
	bundle := &NotificationBundle{
		ResourceType: "Bundle",
		Type:         "history",
		Entry: []NotificationEntry{
			{
				Resource: map[string]interface{}{
					"resourceType": "Patient",
					"id":           "pat-1",
				},
				// No Request field
			},
		},
	}

	mappedEvents, err := mapper.MapBundle(bundle, nil)
	if err != nil {
		t.Fatalf("MapBundle failed: %v", err)
	}

	if len(mappedEvents) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(mappedEvents))
	}

	pe := mappedEvents[0].(*PatientEvent)
	if pe.Type != events.EventPatientUpdate {
		t.Errorf("Expected patient_update (default), got %s", pe.Type)
	}
}

func TestFHIRMapper_MapBundle_SkipsUnknownResourceTypes(t *testing.T) {
	mapper := NewFHIRMapper()

	bundle := &NotificationBundle{
		ResourceType: "Bundle",
		Type:         "history",
		Entry: []NotificationEntry{
			{
				Resource: map[string]interface{}{
					"resourceType": "Patient",
					"id":           "pat-1",
				},
				Request: &EntryRequest{Method: "POST", URL: "Patient"},
			},
			{
				Resource: map[string]interface{}{
					"resourceType": "UnknownResource",
					"id":           "unknown-1",
				},
				Request: &EntryRequest{Method: "POST", URL: "UnknownResource"},
			},
		},
	}

	mappedEvents, err := mapper.MapBundle(bundle, nil)
	if err != nil {
		t.Fatalf("MapBundle failed: %v", err)
	}

	// Should only have 1 event (Patient), UnknownResource is skipped
	if len(mappedEvents) != 1 {
		t.Errorf("Expected 1 event, got %d", len(mappedEvents))
	}
}

func TestMapFHIRDiagnosticReport(t *testing.T) {
	mapper := NewFHIRMapper()

	resource := map[string]interface{}{
		"resourceType": "DiagnosticReport",
		"id":           "dr-123",
		"status":       "final",
		"code": map[string]interface{}{
			"coding": []interface{}{
				map[string]interface{}{
					"system":  "http://loinc.org",
					"code":    "24323-8",
					"display": "Comprehensive metabolic panel",
				},
			},
			"text": "Comprehensive Metabolic Panel",
		},
		"subject": map[string]interface{}{
			"reference": "Patient/pat-456",
			"display":   "John Smith",
		},
		"effectiveDateTime": "2024-01-15T10:30:00Z",
		"conclusion":        "All results within normal limits",
	}

	event, err := mapper.MapResource(resource, "create", nil)
	if err != nil {
		t.Fatalf("MapResource failed: %v", err)
	}

	if event == nil {
		t.Fatal("Expected event, got nil")
	}

	labEvent, ok := event.(*events.LabResultEvent)
	if !ok {
		t.Fatalf("Expected *LabResultEvent, got %T", event)
	}

	if labEvent.Type != events.EventLabResult {
		t.Errorf("Expected type lab_result, got %s", labEvent.Type)
	}

	if labEvent.Test.Description != "Comprehensive Metabolic Panel" {
		t.Errorf("Expected test description 'Comprehensive Metabolic Panel', got %s", labEvent.Test.Description)
	}

	if labEvent.Result.Value != "All results within normal limits" {
		t.Errorf("Expected result value from conclusion, got %s", labEvent.Result.Value)
	}

	if labEvent.Result.Status != "final" {
		t.Errorf("Expected status final, got %s", labEvent.Result.Status)
	}

	if labEvent.Patient.MRN != "pat-456" {
		t.Errorf("Expected patient MRN pat-456, got %s", labEvent.Patient.MRN)
	}
}

func TestMapFHIRDiagnosticReport_DeleteAction(t *testing.T) {
	mapper := NewFHIRMapper()

	resource := map[string]interface{}{
		"resourceType": "DiagnosticReport",
		"id":           "dr-123",
	}

	event, err := mapper.MapResource(resource, "delete", nil)
	if err != nil {
		t.Fatalf("MapResource failed: %v", err)
	}

	// Delete action should return nil for DiagnosticReport
	if event != nil {
		t.Errorf("Expected nil for delete action, got %v", event)
	}
}

func TestMapFHIRAddress(t *testing.T) {
	// Create a patient with a full address to test mapFHIRAddress
	mapper := NewFHIRMapper()

	resource := map[string]interface{}{
		"resourceType": "Patient",
		"id":           "pat-123",
		"name": []interface{}{
			map[string]interface{}{"family": "Smith"},
		},
		"address": []interface{}{
			map[string]interface{}{
				"use":        "home",
				"line":       []interface{}{"123 Main Street", "Apt 4B"},
				"city":       "Boston",
				"state":      "MA",
				"postalCode": "02101",
				"country":    "USA",
			},
		},
	}

	event, err := mapper.MapResource(resource, "create", nil)
	if err != nil {
		t.Fatalf("MapResource failed: %v", err)
	}

	patientEvent := event.(*PatientEvent)
	addr := patientEvent.Patient.Address

	if addr.Line1 != "123 Main Street" {
		t.Errorf("Expected Line1 '123 Main Street', got %s", addr.Line1)
	}
	if addr.Line2 != "Apt 4B" {
		t.Errorf("Expected Line2 'Apt 4B', got %s", addr.Line2)
	}
	if addr.City != "Boston" {
		t.Errorf("Expected City 'Boston', got %s", addr.City)
	}
	if addr.State != "MA" {
		t.Errorf("Expected State 'MA', got %s", addr.State)
	}
	if addr.PostalCode != "02101" {
		t.Errorf("Expected PostalCode '02101', got %s", addr.PostalCode)
	}
	if addr.Country != "USA" {
		t.Errorf("Expected Country 'USA', got %s", addr.Country)
	}
	if addr.Type != "home" {
		t.Errorf("Expected Type 'home', got %s", addr.Type)
	}
}

func TestFHIRMapper_MapResource_MissingResourceType(t *testing.T) {
	mapper := NewFHIRMapper()

	// Resource without resourceType
	resource := map[string]interface{}{
		"id": "some-id",
	}

	_, err := mapper.MapResource(resource, "create", nil)
	if err == nil {
		t.Error("Expected error for missing resourceType")
	}
	if !strings.Contains(err.Error(), "resourceType") {
		t.Errorf("Error should mention resourceType: %v", err)
	}
}

func TestReceiver_UnregisterSubscription(t *testing.T) {
	router := NoOpRouter{}
	receiver := NewReceiver(router, nil)

	// Register a subscription
	receiver.RegisterSubscription(&SubscriptionConfig{
		Name: "test_sub",
	})

	// Send a request - should succeed
	bundle := NotificationBundle{ResourceType: "Bundle", Type: "history", Entry: []NotificationEntry{}}
	body, _ := json.Marshal(bundle)
	req := httptest.NewRequest("POST", "/fhir/notify/test_sub", strings.NewReader(string(body)))
	w := httptest.NewRecorder()
	receiver.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("Expected 200 before unregister, got %d", w.Code)
	}

	// Unregister the subscription
	receiver.UnregisterSubscription("test_sub")

	// Send another request - should fail with 404
	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/fhir/notify/test_sub", strings.NewReader(string(body)))
	receiver.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("Expected 404 after unregister, got %d", w.Code)
	}
}

// mockMapper is a helper for testing custom mappers
type mockMapper struct {
	mapFunc func(resource map[string]interface{}, action string, config *EventMappingConfig) (interface{}, error)
}

func (m *mockMapper) Map(resource map[string]interface{}, action string, config *EventMappingConfig) (interface{}, error) {
	return m.mapFunc(resource, action, config)
}
