package subscription

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cblevins/fi-fhir/pkg/events"
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
