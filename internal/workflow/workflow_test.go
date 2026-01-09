package workflow

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cblevins/fi-fhir/pkg/events"
)

func TestParseWorkflow(t *testing.T) {
	yaml := `
workflow:
  name: test_workflow
  version: "1.0"
  routes:
    - name: all_events
      filter:
        event_type: patient_admit
      actions:
        - type: log
          level: info
          message: "Test message"
`

	w, err := ParseWorkflow([]byte(yaml))
	if err != nil {
		t.Fatalf("ParseWorkflow failed: %v", err)
	}

	if w.Name != "test_workflow" {
		t.Errorf("Expected name 'test_workflow', got '%s'", w.Name)
	}
	if w.Version != "1.0" {
		t.Errorf("Expected version '1.0', got '%s'", w.Version)
	}
	if len(w.Routes) != 1 {
		t.Fatalf("Expected 1 route, got %d", len(w.Routes))
	}

	route := w.Routes[0]
	if route.Name != "all_events" {
		t.Errorf("Expected route name 'all_events', got '%s'", route.Name)
	}
	if len(route.Filter.EventType) != 1 || route.Filter.EventType[0] != "patient_admit" {
		t.Errorf("Expected filter event_type ['patient_admit'], got %v", route.Filter.EventType)
	}
	if len(route.Actions) != 1 {
		t.Fatalf("Expected 1 action, got %d", len(route.Actions))
	}
	if route.Actions[0].Type != "log" {
		t.Errorf("Expected action type 'log', got '%s'", route.Actions[0].Type)
	}
}

func TestParseWorkflowMultipleEventTypes(t *testing.T) {
	yaml := `
workflow:
  name: multi_type
  version: "1.0"
  routes:
    - name: patient_events
      filter:
        event_type: [patient_admit, patient_update, patient_discharge]
      actions:
        - type: log
`

	w, err := ParseWorkflow([]byte(yaml))
	if err != nil {
		t.Fatalf("ParseWorkflow failed: %v", err)
	}

	route := w.Routes[0]
	if len(route.Filter.EventType) != 3 {
		t.Errorf("Expected 3 event types, got %d", len(route.Filter.EventType))
	}
}

func TestWorkflowValidation(t *testing.T) {
	tests := []struct {
		name      string
		workflow  Workflow
		wantError bool
	}{
		{
			name: "valid workflow",
			workflow: Workflow{
				Name:    "test",
				Version: "1.0",
				Routes: []Route{
					{
						Name:    "route1",
						Actions: []Action{{Type: "log"}},
					},
				},
			},
			wantError: false,
		},
		{
			name: "missing name",
			workflow: Workflow{
				Version: "1.0",
				Routes: []Route{
					{
						Name:    "route1",
						Actions: []Action{{Type: "log"}},
					},
				},
			},
			wantError: true,
		},
		{
			name: "no routes",
			workflow: Workflow{
				Name:    "test",
				Version: "1.0",
				Routes:  []Route{},
			},
			wantError: true,
		},
		{
			name: "route without name",
			workflow: Workflow{
				Name:    "test",
				Version: "1.0",
				Routes: []Route{
					{
						Actions: []Action{{Type: "log"}},
					},
				},
			},
			wantError: true,
		},
		{
			name: "route without actions",
			workflow: Workflow{
				Name:    "test",
				Version: "1.0",
				Routes: []Route{
					{
						Name:    "route1",
						Actions: []Action{},
					},
				},
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errors := tt.workflow.Validate()
			if tt.wantError && len(errors) == 0 {
				t.Error("Expected validation errors, got none")
			}
			if !tt.wantError && len(errors) > 0 {
				t.Errorf("Expected no validation errors, got: %v", errors)
			}
		})
	}
}

func TestEngineFilterMatching(t *testing.T) {
	workflow := &Workflow{
		Name:    "test",
		Version: "1.0",
		Routes: []Route{
			{
				Name: "admits_only",
				Filter: Filter{
					EventType: StringOrSlice{"patient_admit"},
				},
				Actions: []Action{{Type: "log"}},
			},
			{
				Name: "epic_only",
				Filter: Filter{
					Source: StringOrSlice{"epic_adt"},
				},
				Actions: []Action{{Type: "log"}},
			},
			{
				Name:    "all_events",
				Filter:  Filter{}, // Empty filter matches all
				Actions: []Action{{Type: "log"}},
			},
		},
	}

	engine, err := NewEngine(workflow)
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	tests := []struct {
		name           string
		event          interface{}
		expectedRoutes []string
	}{
		{
			name: "patient admit from epic",
			event: &events.PatientAdmitEvent{
				EventMeta: events.EventMeta{
					Type:   events.EventPatientAdmit,
					Source: "epic_adt",
				},
			},
			expectedRoutes: []string{"admits_only", "epic_only", "all_events"},
		},
		{
			name: "patient admit from cerner",
			event: &events.PatientAdmitEvent{
				EventMeta: events.EventMeta{
					Type:   events.EventPatientAdmit,
					Source: "cerner_adt",
				},
			},
			expectedRoutes: []string{"admits_only", "all_events"},
		},
		{
			name: "lab result from epic",
			event: &events.LabResultEvent{
				EventMeta: events.EventMeta{
					Type:   events.EventLabResult,
					Source: "epic_adt",
				},
			},
			expectedRoutes: []string{"epic_only", "all_events"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := engine.Process(tt.event)

			var matchedRoutes []string
			for _, rr := range result.RouteResults {
				if rr.Matched {
					matchedRoutes = append(matchedRoutes, rr.RouteName)
				}
			}

			if len(matchedRoutes) != len(tt.expectedRoutes) {
				t.Errorf("Expected %d matched routes, got %d: %v",
					len(tt.expectedRoutes), len(matchedRoutes), matchedRoutes)
				return
			}

			for i, expected := range tt.expectedRoutes {
				if matchedRoutes[i] != expected {
					t.Errorf("Expected route %d to be '%s', got '%s'",
						i, expected, matchedRoutes[i])
				}
			}
		})
	}
}

func TestLogAction(t *testing.T) {
	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	event := &events.PatientAdmitEvent{
		EventMeta: events.EventMeta{
			Type:   events.EventPatientAdmit,
			Source: "test",
		},
		Patient: events.Patient{
			MRN:        "123456",
			GivenName:  "John",
			FamilyName: "Doe",
		},
	}

	config := map[string]string{
		"level":   "info",
		"message": "Processed patient",
	}

	err := logAction(event, config)
	if err != nil {
		t.Fatalf("logAction failed: %v", err)
	}

	w.Close()
	var buf bytes.Buffer
	buf.ReadFrom(r)
	os.Stdout = old

	output := buf.String()
	if !strings.Contains(output, "INFO") {
		t.Error("Expected output to contain 'INFO'")
	}
	if !strings.Contains(output, "Processed patient") {
		t.Error("Expected output to contain message")
	}
}

func TestWebhookAction(t *testing.T) {
	// Create test server
	var receivedBody []byte
	var receivedMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		receivedBody, _ = json.Marshal(r.Body)

		// Read actual body
		buf := new(bytes.Buffer)
		buf.ReadFrom(r.Body)
		receivedBody = buf.Bytes()

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	event := &events.PatientAdmitEvent{
		EventMeta: events.EventMeta{
			Type:   events.EventPatientAdmit,
			Source: "test",
		},
		Patient: events.Patient{
			MRN: "123456",
		},
	}

	config := map[string]string{
		"url":    server.URL,
		"method": "POST",
	}

	err := webhookAction(event, config)
	if err != nil {
		t.Fatalf("webhookAction failed: %v", err)
	}

	if receivedMethod != "POST" {
		t.Errorf("Expected POST, got %s", receivedMethod)
	}

	// Verify body contains event data
	if !strings.Contains(string(receivedBody), "123456") {
		t.Errorf("Expected body to contain MRN, got: %s", string(receivedBody))
	}
}

func TestWebhookActionError(t *testing.T) {
	// Create test server that returns error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal error"))
	}))
	defer server.Close()

	event := &events.PatientAdmitEvent{}
	config := map[string]string{
		"url": server.URL,
	}

	err := webhookAction(event, config)
	if err == nil {
		t.Error("Expected error for 500 response")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("Expected error to mention status code, got: %v", err)
	}
}

func TestDryRun(t *testing.T) {
	workflow := &Workflow{
		Name:    "test",
		Version: "1.0",
		Routes: []Route{
			{
				Name: "test_route",
				Filter: Filter{
					EventType: StringOrSlice{"patient_admit"},
				},
				Actions: []Action{
					{Type: "log"},
					{Type: "webhook", Config: map[string]string{"url": "http://example.com"}},
				},
			},
		},
	}

	engine, err := NewEngine(workflow)
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	event := &events.PatientAdmitEvent{
		EventMeta: events.EventMeta{
			Type: events.EventPatientAdmit,
		},
	}

	result := engine.DryRun(event)

	if len(result.RouteResults) != 1 {
		t.Fatalf("Expected 1 route result, got %d", len(result.RouteResults))
	}

	rr := result.RouteResults[0]
	if !rr.Matched {
		t.Error("Expected route to match")
	}
	if !rr.Skipped {
		t.Error("Expected route to be skipped in dry-run")
	}
	if rr.ActionsRun != 2 {
		t.Errorf("Expected 2 actions would run, got %d", rr.ActionsRun)
	}
}

func TestStringOrSlice(t *testing.T) {
	tests := []struct {
		name     string
		slice    StringOrSlice
		value    string
		expected bool
	}{
		{"empty matches all", StringOrSlice{}, "anything", true},
		{"contains value", StringOrSlice{"a", "b", "c"}, "b", true},
		{"missing value", StringOrSlice{"a", "b", "c"}, "d", false},
		{"single value match", StringOrSlice{"patient_admit"}, "patient_admit", true},
		{"single value miss", StringOrSlice{"patient_admit"}, "lab_result", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.slice.Contains(tt.value)
			if result != tt.expected {
				t.Errorf("Contains(%q) = %v, want %v", tt.value, result, tt.expected)
			}
		})
	}
}

func TestCustomActionHandler(t *testing.T) {
	workflow := &Workflow{
		Name:    "test",
		Version: "1.0",
		Routes: []Route{
			{
				Name:    "custom_route",
				Actions: []Action{{Type: "custom"}},
			},
		},
	}

	engine, err := NewEngine(workflow)
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	var customCalled bool
	engine.RegisterAction("custom", ActionHandlerFunc(func(event interface{}, config map[string]string) error {
		customCalled = true
		return nil
	}))

	event := &events.Event{
		EventMeta: events.EventMeta{Type: "test"},
	}

	result := engine.Process(event)

	if !customCalled {
		t.Error("Custom action handler was not called")
	}
	if result.HasErrors() {
		t.Errorf("Unexpected errors: %v", result.AllErrors())
	}
}

func TestFHIRActionRegistered(t *testing.T) {
	// Verify FHIR action is registered by default
	workflow := &Workflow{
		Name:    "test",
		Version: "1.0",
		Routes: []Route{
			{
				Name: "fhir_route",
				Filter: Filter{
					EventType: StringOrSlice{"patient_admit"},
				},
				Actions: []Action{
					{Type: "fhir", Config: map[string]string{"endpoint": "http://test.com"}},
				},
			},
		},
	}

	engine, err := NewEngine(workflow)
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	// Verify the fhir action handler exists
	if _, exists := engine.actions["fhir"]; !exists {
		t.Error("Expected fhir action to be registered")
	}
}

func TestFHIRActionRequiresEndpoint(t *testing.T) {
	event := &events.PatientAdmitEvent{
		EventMeta: events.EventMeta{
			Type:   events.EventPatientAdmit,
			Source: "test",
		},
		Patient: events.Patient{
			MRN:        "123456",
			GivenName:  "John",
			FamilyName: "Doe",
		},
	}

	// Missing endpoint should error
	config := map[string]string{}

	err := fhirAction(event, config)
	if err == nil {
		t.Error("Expected error for missing endpoint")
	}
	if !strings.Contains(err.Error(), "endpoint") {
		t.Errorf("Expected error about endpoint, got: %v", err)
	}
}

func TestFHIRActionPatientAdmit(t *testing.T) {
	// Create test FHIR server
	var receivedBody []byte
	var receivedContentType string
	var requestCount int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		receivedContentType = r.Header.Get("Content-Type")

		buf := new(bytes.Buffer)
		buf.ReadFrom(r.Body)
		receivedBody = buf.Bytes()

		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"resourceType":"Bundle","type":"transaction-response"}`))
	}))
	defer server.Close()

	event := &events.PatientAdmitEvent{
		EventMeta: events.EventMeta{
			Type:   events.EventPatientAdmit,
			Source: "test",
		},
		Patient: events.Patient{
			MRN:        "123456",
			GivenName:  "John",
			FamilyName: "Doe",
			Gender:     "M",
		},
		Encounter: events.Encounter{
			ID:    "enc-001",
			Class: "I",
		},
	}

	config := map[string]string{
		"endpoint": server.URL,
	}

	err := fhirAction(event, config)
	if err != nil {
		t.Fatalf("fhirAction failed: %v", err)
	}

	// Should have sent one request (bundle)
	if requestCount != 1 {
		t.Errorf("Expected 1 request, got %d", requestCount)
	}

	// Verify content type
	if receivedContentType != "application/fhir+json" {
		t.Errorf("Expected Content-Type 'application/fhir+json', got '%s'", receivedContentType)
	}

	// Verify body contains FHIR resources
	if !strings.Contains(string(receivedBody), `"resourceType":"Bundle"`) {
		t.Errorf("Expected body to contain Bundle, got: %s", string(receivedBody))
	}
	if !strings.Contains(string(receivedBody), `"resourceType":"Patient"`) {
		t.Errorf("Expected body to contain Patient, got: %s", string(receivedBody))
	}
}

func TestFHIRActionLabResult(t *testing.T) {
	var receivedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := new(bytes.Buffer)
		buf.ReadFrom(r.Body)
		receivedBody = buf.Bytes()
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	event := &events.LabResultEvent{
		EventMeta: events.EventMeta{
			Type:   events.EventLabResult,
			Source: "test_lab",
		},
		Patient: events.Patient{
			MRN:        "LAB123",
			GivenName:  "Jane",
			FamilyName: "Smith",
		},
		Test: events.LabTest{
			LOINCCode:   "2951-2",
			Description: "Sodium, Serum",
		},
		Result: events.LabValue{
			Value:          "142",
			Unit:           "mmol/L",
			Interpretation: "N",
			Status:         "F",
		},
	}

	config := map[string]string{
		"endpoint": server.URL,
	}

	err := fhirAction(event, config)
	if err != nil {
		t.Fatalf("fhirAction failed: %v", err)
	}

	// Verify body contains DiagnosticReport and Observation
	bodyStr := string(receivedBody)
	if !strings.Contains(bodyStr, `"resourceType":"Bundle"`) {
		t.Errorf("Expected body to contain Bundle")
	}
	if !strings.Contains(bodyStr, "DiagnosticReport") {
		t.Errorf("Expected body to contain DiagnosticReport")
	}
	if !strings.Contains(bodyStr, "Observation") {
		t.Errorf("Expected body to contain Observation")
	}
}

func TestFHIRActionServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"resourceType":"OperationOutcome","issue":[{"severity":"error"}]}`))
	}))
	defer server.Close()

	event := &events.PatientAdmitEvent{
		EventMeta: events.EventMeta{
			Type: events.EventPatientAdmit,
		},
		Patient: events.Patient{
			MRN: "ERROR123",
		},
	}

	config := map[string]string{
		"endpoint": server.URL,
	}

	err := fhirAction(event, config)
	if err == nil {
		t.Error("Expected error for 500 response")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("Expected error to mention status code, got: %v", err)
	}
}

func TestFHIRActionWithAuth(t *testing.T) {
	var receivedAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	event := &events.PatientAdmitEvent{
		EventMeta: events.EventMeta{Type: events.EventPatientAdmit},
		Patient:   events.Patient{MRN: "AUTH123"},
	}

	config := map[string]string{
		"endpoint": server.URL,
		"token":    "test-token-12345",
	}

	err := fhirAction(event, config)
	if err != nil {
		t.Fatalf("fhirAction failed: %v", err)
	}

	if receivedAuth != "Bearer test-token-12345" {
		t.Errorf("Expected Authorization 'Bearer test-token-12345', got '%s'", receivedAuth)
	}
}

func TestFHIRActionMapEvent(t *testing.T) {
	var receivedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := new(bytes.Buffer)
		buf.ReadFrom(r.Body)
		receivedBody = buf.Bytes()
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	// Test with map-based event (simulating JSON parsed by workflow engine)
	event := map[string]interface{}{
		"type":   "patient_admit",
		"source": "test_system",
		"patient": map[string]interface{}{
			"mrn":         "MAP123",
			"given_name":  "Map",
			"family_name": "Test",
			"gender":      "F",
		},
		"encounter": map[string]interface{}{
			"id":    "enc-map-001",
			"class": "O",
		},
	}

	config := map[string]string{
		"endpoint": server.URL,
	}

	err := fhirAction(event, config)
	if err != nil {
		t.Fatalf("fhirAction with map event failed: %v", err)
	}

	// Verify body contains patient data
	if !strings.Contains(string(receivedBody), "Patient") {
		t.Errorf("Expected body to contain Patient, got: %s", string(receivedBody))
	}
}

func TestFHIRActionUnsupportedEvent(t *testing.T) {
	event := &events.AppointmentEvent{
		EventMeta: events.EventMeta{
			Type: events.EventAppointmentScheduled,
		},
	}

	config := map[string]string{
		"endpoint": "http://test.com",
	}

	err := fhirAction(event, config)
	if err == nil {
		t.Error("Expected error for unsupported event type")
	}
	if !strings.Contains(err.Error(), "unsupported") {
		t.Errorf("Expected 'unsupported' in error, got: %v", err)
	}
}

func TestFHIRActionInWorkflow(t *testing.T) {
	var fhirCalled bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fhirCalled = true
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	workflow := &Workflow{
		Name:    "fhir_test",
		Version: "1.0",
		Routes: []Route{
			{
				Name: "patient_to_fhir",
				Filter: Filter{
					EventType: StringOrSlice{"patient_admit"},
				},
				Actions: []Action{
					{
						Type: "fhir",
						Config: map[string]string{
							"endpoint": server.URL,
						},
					},
				},
			},
		},
	}

	engine, err := NewEngine(workflow)
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	event := &events.PatientAdmitEvent{
		EventMeta: events.EventMeta{
			Type:   events.EventPatientAdmit,
			Source: "workflow_test",
		},
		Patient: events.Patient{
			MRN:        "WF123",
			GivenName:  "Workflow",
			FamilyName: "Test",
		},
	}

	result := engine.Process(event)

	if result.HasErrors() {
		t.Errorf("Workflow processing had errors: %v", result.AllErrors())
	}

	if !fhirCalled {
		t.Error("FHIR action was not called")
	}

	// Verify route matched
	if len(result.RouteResults) != 1 {
		t.Fatalf("Expected 1 route result, got %d", len(result.RouteResults))
	}
	if !result.RouteResults[0].Matched {
		t.Error("Expected route to match")
	}
}

func TestCELConditionEvaluation(t *testing.T) {
	tests := []struct {
		name          string
		condition     string
		event         interface{}
		shouldMatch   bool
	}{
		{
			name:      "simple equality match",
			condition: `event.patient.mrn == "TEST123"`,
			event: &events.PatientAdmitEvent{
				EventMeta: events.EventMeta{Type: events.EventPatientAdmit},
				Patient:   events.Patient{MRN: "TEST123"},
			},
			shouldMatch: true,
		},
		{
			name:      "simple equality no match",
			condition: `event.patient.mrn == "OTHER"`,
			event: &events.PatientAdmitEvent{
				EventMeta: events.EventMeta{Type: events.EventPatientAdmit},
				Patient:   events.Patient{MRN: "TEST123"},
			},
			shouldMatch: false,
		},
		{
			name:      "string contains",
			condition: `event.patient.mrn.startsWith("TEST")`,
			event: &events.PatientAdmitEvent{
				EventMeta: events.EventMeta{Type: events.EventPatientAdmit},
				Patient:   events.Patient{MRN: "TEST456"},
			},
			shouldMatch: true,
		},
		{
			name:      "numeric comparison",
			condition: `event.encounter.class == "I"`,
			event: &events.PatientAdmitEvent{
				EventMeta: events.EventMeta{Type: events.EventPatientAdmit},
				Patient:   events.Patient{MRN: "123"},
				Encounter: events.Encounter{Class: "I"},
			},
			shouldMatch: true,
		},
		{
			name:      "logical AND",
			condition: `event.type == "patient_admit" && event.source == "epic_adt"`,
			event: &events.PatientAdmitEvent{
				EventMeta: events.EventMeta{
					Type:   events.EventPatientAdmit,
					Source: "epic_adt",
				},
			},
			shouldMatch: true,
		},
		{
			name:      "logical OR",
			condition: `event.source == "epic_adt" || event.source == "cerner_adt"`,
			event: &events.PatientAdmitEvent{
				EventMeta: events.EventMeta{
					Type:   events.EventPatientAdmit,
					Source: "cerner_adt",
				},
			},
			shouldMatch: true,
		},
		{
			name:      "empty condition matches all",
			condition: "",
			event: &events.PatientAdmitEvent{
				EventMeta: events.EventMeta{Type: events.EventPatientAdmit},
			},
			shouldMatch: true,
		},
		{
			name:      "map event with condition",
			condition: `event.patient.mrn == "MAP123"`,
			event: map[string]interface{}{
				"type": "patient_admit",
				"patient": map[string]interface{}{
					"mrn": "MAP123",
				},
			},
			shouldMatch: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workflow := &Workflow{
				Name:    "cel_test",
				Version: "1.0",
				Routes: []Route{
					{
						Name: "cel_route",
						Filter: Filter{
							Condition: tt.condition,
						},
						Actions: []Action{{Type: "log"}},
					},
				},
			}

			engine, err := NewEngine(workflow)
			if err != nil {
				t.Fatalf("NewEngine failed: %v", err)
			}

			result := engine.Process(tt.event)

			if len(result.RouteResults) != 1 {
				t.Fatalf("Expected 1 route result, got %d", len(result.RouteResults))
			}

			if result.RouteResults[0].Matched != tt.shouldMatch {
				t.Errorf("Expected matched=%v, got %v", tt.shouldMatch, result.RouteResults[0].Matched)
			}
		})
	}
}

func TestCELEvaluator(t *testing.T) {
	eval, err := NewCELEvaluator()
	if err != nil {
		t.Fatalf("NewCELEvaluator failed: %v", err)
	}

	tests := []struct {
		name      string
		condition string
		event     map[string]interface{}
		want      bool
		wantErr   bool
	}{
		{
			name:      "boolean literal true",
			condition: "true",
			event:     map[string]interface{}{},
			want:      true,
		},
		{
			name:      "boolean literal false",
			condition: "false",
			event:     map[string]interface{}{},
			want:      false,
		},
		{
			name:      "field access",
			condition: `event.name == "test"`,
			event:     map[string]interface{}{"name": "test"},
			want:      true,
		},
		{
			name:      "nested field access",
			condition: `event.patient.id == "P001"`,
			event: map[string]interface{}{
				"patient": map[string]interface{}{"id": "P001"},
			},
			want: true,
		},
		{
			name:      "invalid expression",
			condition: `event.nonexistent.field.deep`,
			event:     map[string]interface{}{},
			want:      false,
			wantErr:   true,
		},
		{
			name:      "empty condition",
			condition: "",
			event:     map[string]interface{}{},
			want:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := eval.Evaluate(tt.condition, tt.event)
			if (err != nil) != tt.wantErr {
				t.Errorf("Evaluate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("Evaluate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCELCaching(t *testing.T) {
	eval, err := NewCELEvaluator()
	if err != nil {
		t.Fatalf("NewCELEvaluator failed: %v", err)
	}

	condition := `event.test == "value"`
	event := map[string]interface{}{"test": "value"}

	// First evaluation - should compile and cache
	result1, err := eval.Evaluate(condition, event)
	if err != nil {
		t.Fatalf("First Evaluate failed: %v", err)
	}

	// Second evaluation - should use cache
	result2, err := eval.Evaluate(condition, event)
	if err != nil {
		t.Fatalf("Second Evaluate failed: %v", err)
	}

	if result1 != result2 {
		t.Error("Cached result differs from original")
	}

	// Verify cache has one entry
	if len(eval.cache) != 1 {
		t.Errorf("Expected 1 cached program, got %d", len(eval.cache))
	}
}

func TestTransformSetField(t *testing.T) {
	transformer := NewTransformer(nil)

	tests := []struct {
		name      string
		event     map[string]interface{}
		setField  string
		wantPath  string
		wantValue interface{}
		wantErr   bool
	}{
		{
			name:      "set string field",
			event:     map[string]interface{}{"name": "old"},
			setField:  `name = "new"`,
			wantPath:  "name",
			wantValue: "new",
		},
		{
			name:      "set nested field",
			event:     map[string]interface{}{"patient": map[string]interface{}{"name": "old"}},
			setField:  `patient.status = "active"`,
			wantPath:  "patient.status",
			wantValue: "active",
		},
		{
			name:      "set boolean true",
			event:     map[string]interface{}{},
			setField:  "active = true",
			wantPath:  "active",
			wantValue: true,
		},
		{
			name:      "set boolean false",
			event:     map[string]interface{}{},
			setField:  "active = false",
			wantPath:  "active",
			wantValue: false,
		},
		{
			name:      "set integer",
			event:     map[string]interface{}{},
			setField:  "count = 42",
			wantPath:  "count",
			wantValue: int64(42),
		},
		{
			name:      "set null",
			event:     map[string]interface{}{"value": "something"},
			setField:  "value = null",
			wantPath:  "value",
			wantValue: nil,
		},
		{
			name:      "create nested path",
			event:     map[string]interface{}{},
			setField:  `deeply.nested.field = "value"`,
			wantPath:  "deeply.nested.field",
			wantValue: "value",
		},
		{
			name:     "invalid format",
			event:    map[string]interface{}{},
			setField: "no_equals_sign",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transform := Transform{SetField: tt.setField}
			result, err := transformer.Apply(tt.event, transform)

			if tt.wantErr {
				if err == nil {
					t.Error("Expected error, got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("Apply failed: %v", err)
			}

			resultMap := result.(map[string]interface{})
			gotValue, err := getNestedValueForTest(resultMap, tt.wantPath)
			if err != nil {
				t.Fatalf("Failed to get value at path %s: %v", tt.wantPath, err)
			}

			if gotValue != tt.wantValue {
				t.Errorf("Expected %v (%T), got %v (%T)", tt.wantValue, tt.wantValue, gotValue, gotValue)
			}
		})
	}
}

func TestTransformRedact(t *testing.T) {
	transformer := NewTransformer(nil)

	tests := []struct {
		name         string
		event        map[string]interface{}
		redactFields []string
		checkMissing []string
	}{
		{
			name: "redact top-level field",
			event: map[string]interface{}{
				"name": "John",
				"ssn":  "123-45-6789",
			},
			redactFields: []string{"ssn"},
			checkMissing: []string{"ssn"},
		},
		{
			name: "redact nested field",
			event: map[string]interface{}{
				"patient": map[string]interface{}{
					"name": "John",
					"ssn":  "123-45-6789",
				},
			},
			redactFields: []string{"patient.ssn"},
			checkMissing: []string{"patient.ssn"},
		},
		{
			name: "redact multiple fields",
			event: map[string]interface{}{
				"name": "John",
				"ssn":  "123-45-6789",
				"dob":  "1990-01-01",
			},
			redactFields: []string{"ssn", "dob"},
			checkMissing: []string{"ssn", "dob"},
		},
		{
			name:         "redact non-existent field (no error)",
			event:        map[string]interface{}{"name": "John"},
			redactFields: []string{"nonexistent"},
			checkMissing: []string{"nonexistent"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transform := Transform{
				Redact: &RedactConfig{Fields: tt.redactFields},
			}
			result, err := transformer.Apply(tt.event, transform)
			if err != nil {
				t.Fatalf("Apply failed: %v", err)
			}

			resultMap := result.(map[string]interface{})
			for _, field := range tt.checkMissing {
				_, err := getNestedValueForTest(resultMap, field)
				if err == nil {
					t.Errorf("Expected field %s to be redacted, but it exists", field)
				}
			}
		})
	}
}

func TestTransformInWorkflow(t *testing.T) {
	workflow := &Workflow{
		Name:    "transform_test",
		Version: "1.0",
		Routes: []Route{
			{
				Name:   "transform_route",
				Filter: Filter{}, // Match all
				Transforms: []Transform{
					{SetField: `patient.status = "active"`},
					{SetField: `processed = true`},
				},
				Actions: []Action{{Type: "log"}},
			},
		},
	}

	engine, err := NewEngine(workflow)
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	event := map[string]interface{}{
		"type": "patient_admit",
		"patient": map[string]interface{}{
			"name": "John",
		},
	}

	result := engine.Process(event)

	if len(result.RouteResults) != 1 {
		t.Fatalf("Expected 1 route result, got %d", len(result.RouteResults))
	}

	rr := result.RouteResults[0]
	if !rr.Matched {
		t.Error("Expected route to match")
	}
	if rr.TransformsRun != 2 {
		t.Errorf("Expected 2 transforms run, got %d", rr.TransformsRun)
	}
	if len(rr.TransformErrors) > 0 {
		t.Errorf("Expected no transform errors, got: %v", rr.TransformErrors)
	}
}

func TestTransformWithRedactInWorkflow(t *testing.T) {
	workflow := &Workflow{
		Name:    "redact_test",
		Version: "1.0",
		Routes: []Route{
			{
				Name:   "redact_route",
				Filter: Filter{},
				Transforms: []Transform{
					{Redact: &RedactConfig{Fields: []string{"patient.ssn", "patient.dob"}}},
				},
				Actions: []Action{{Type: "log"}},
			},
		},
	}

	engine, err := NewEngine(workflow)
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	event := map[string]interface{}{
		"type": "patient_admit",
		"patient": map[string]interface{}{
			"name": "John",
			"ssn":  "123-45-6789",
			"dob":  "1990-01-01",
		},
	}

	result := engine.Process(event)

	if result.HasErrors() {
		t.Errorf("Unexpected errors: %v", result.AllErrors())
	}

	rr := result.RouteResults[0]
	if rr.TransformsRun != 1 {
		t.Errorf("Expected 1 transform run, got %d", rr.TransformsRun)
	}
}

func TestTransformPreservesOriginal(t *testing.T) {
	transformer := NewTransformer(nil)

	original := map[string]interface{}{
		"name":  "original",
		"value": 100,
	}

	transform := Transform{SetField: `name = "modified"`}
	result, err := transformer.Apply(original, transform)
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	// Verify original is unchanged
	if original["name"] != "original" {
		t.Errorf("Original was modified: name = %v", original["name"])
	}

	// Verify result has new value
	resultMap := result.(map[string]interface{})
	if resultMap["name"] != "modified" {
		t.Errorf("Result should have modified value: name = %v", resultMap["name"])
	}
}

// Helper to get nested values for testing
func getNestedValueForTest(m map[string]interface{}, path string) (interface{}, error) {
	parts := strings.Split(path, ".")
	var current interface{} = m

	for _, key := range parts {
		currentMap, ok := current.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("not a map")
		}
		next, exists := currentMap[key]
		if !exists {
			return nil, fmt.Errorf("key not found: %s", key)
		}
		current = next
	}
	return current, nil
}

// OAuth2 Tests

func TestOAuthTokenManager(t *testing.T) {
	// Create a mock OAuth server
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify it's a POST request with form data
		if r.Method != "POST" {
			t.Errorf("Expected POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
			t.Errorf("Expected form content type, got %s", r.Header.Get("Content-Type"))
		}

		// Parse form data
		r.ParseForm()
		if r.Form.Get("grant_type") != "client_credentials" {
			t.Errorf("Expected client_credentials grant, got %s", r.Form.Get("grant_type"))
		}
		if r.Form.Get("client_id") != "test_client" {
			t.Errorf("Expected client_id test_client, got %s", r.Form.Get("client_id"))
		}
		if r.Form.Get("client_secret") != "test_secret" {
			t.Errorf("Expected client_secret test_secret, got %s", r.Form.Get("client_secret"))
		}

		// Return a token
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(TokenResponse{
			AccessToken: "oauth_access_token_123",
			TokenType:   "Bearer",
			ExpiresIn:   3600,
		})
	}))
	defer tokenServer.Close()

	manager := NewOAuthTokenManager()

	config := OAuthConfig{
		TokenURL:     tokenServer.URL,
		ClientID:     "test_client",
		ClientSecret: "test_secret",
	}

	token, err := manager.GetToken(config)
	if err != nil {
		t.Fatalf("GetToken failed: %v", err)
	}

	if token != "oauth_access_token_123" {
		t.Errorf("Expected token 'oauth_access_token_123', got '%s'", token)
	}
}

func TestOAuthTokenCaching(t *testing.T) {
	requestCount := 0
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(TokenResponse{
			AccessToken: fmt.Sprintf("token_%d", requestCount),
			TokenType:   "Bearer",
			ExpiresIn:   3600,
		})
	}))
	defer tokenServer.Close()

	manager := NewOAuthTokenManager()

	config := OAuthConfig{
		TokenURL:     tokenServer.URL,
		ClientID:     "cache_client",
		ClientSecret: "cache_secret",
	}

	// First call should fetch token
	token1, err := manager.GetToken(config)
	if err != nil {
		t.Fatalf("First GetToken failed: %v", err)
	}

	// Second call should use cache
	token2, err := manager.GetToken(config)
	if err != nil {
		t.Fatalf("Second GetToken failed: %v", err)
	}

	// Both should return same token (from cache)
	if token1 != token2 {
		t.Errorf("Expected cached token '%s', got '%s'", token1, token2)
	}

	// Should only have made one request
	if requestCount != 1 {
		t.Errorf("Expected 1 request (caching), got %d", requestCount)
	}
}

func TestOAuthTokenExpiration(t *testing.T) {
	requestCount := 0
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "application/json")
		// Return token that expires in 30 seconds (within buffer window)
		json.NewEncoder(w).Encode(TokenResponse{
			AccessToken: fmt.Sprintf("token_%d", requestCount),
			TokenType:   "Bearer",
			ExpiresIn:   30, // Less than the 60s buffer
		})
	}))
	defer tokenServer.Close()

	manager := NewOAuthTokenManager()

	config := OAuthConfig{
		TokenURL:     tokenServer.URL,
		ClientID:     "expire_client",
		ClientSecret: "expire_secret",
	}

	// First call should fetch token
	_, err := manager.GetToken(config)
	if err != nil {
		t.Fatalf("First GetToken failed: %v", err)
	}

	// Second call should refetch because token expires within buffer
	_, err = manager.GetToken(config)
	if err != nil {
		t.Fatalf("Second GetToken failed: %v", err)
	}

	// Should have made two requests (no caching due to expiration)
	if requestCount != 2 {
		t.Errorf("Expected 2 requests (token expired), got %d", requestCount)
	}
}

func TestOAuthConfigParsing(t *testing.T) {
	tests := []struct {
		name      string
		config    map[string]string
		expectNil bool
		scopes    []string
	}{
		{
			name: "complete config",
			config: map[string]string{
				"token_url":     "https://auth.example.com/token",
				"client_id":     "my_client",
				"client_secret": "my_secret",
				"scopes":        "read write",
			},
			expectNil: false,
			scopes:    []string{"read", "write"},
		},
		{
			name: "missing token_url",
			config: map[string]string{
				"client_id":     "my_client",
				"client_secret": "my_secret",
			},
			expectNil: true,
		},
		{
			name: "missing client_id",
			config: map[string]string{
				"token_url":     "https://auth.example.com/token",
				"client_secret": "my_secret",
			},
			expectNil: true,
		},
		{
			name: "missing client_secret",
			config: map[string]string{
				"token_url": "https://auth.example.com/token",
				"client_id": "my_client",
			},
			expectNil: true,
		},
		{
			name: "comma separated scopes",
			config: map[string]string{
				"token_url":     "https://auth.example.com/token",
				"client_id":     "my_client",
				"client_secret": "my_secret",
				"scopes":        "read,write,delete",
			},
			expectNil: false,
			scopes:    []string{"read", "write", "delete"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseOAuthConfig(tt.config)

			if tt.expectNil {
				if result != nil {
					t.Errorf("Expected nil, got %+v", result)
				}
				return
			}

			if result == nil {
				t.Fatal("Expected non-nil result")
			}

			if len(tt.scopes) > 0 {
				if len(result.Scopes) != len(tt.scopes) {
					t.Errorf("Expected %d scopes, got %d", len(tt.scopes), len(result.Scopes))
				}
				for i, scope := range tt.scopes {
					if i < len(result.Scopes) && result.Scopes[i] != scope {
						t.Errorf("Expected scope[%d] = '%s', got '%s'", i, scope, result.Scopes[i])
					}
				}
			}
		})
	}
}

func TestOAuthWithScopes(t *testing.T) {
	var receivedScopes string
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		receivedScopes = r.Form.Get("scope")

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(TokenResponse{
			AccessToken: "scoped_token",
			TokenType:   "Bearer",
			ExpiresIn:   3600,
		})
	}))
	defer tokenServer.Close()

	manager := NewOAuthTokenManager()

	config := OAuthConfig{
		TokenURL:     tokenServer.URL,
		ClientID:     "scoped_client",
		ClientSecret: "scoped_secret",
		Scopes:       []string{"patient/*.read", "system/*.write"},
	}

	_, err := manager.GetToken(config)
	if err != nil {
		t.Fatalf("GetToken failed: %v", err)
	}

	if receivedScopes != "patient/*.read system/*.write" {
		t.Errorf("Expected scopes 'patient/*.read system/*.write', got '%s'", receivedScopes)
	}
}

func TestOAuthTokenServerError(t *testing.T) {
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error": "invalid_client"}`))
	}))
	defer tokenServer.Close()

	manager := NewOAuthTokenManager()

	config := OAuthConfig{
		TokenURL:     tokenServer.URL,
		ClientID:     "bad_client",
		ClientSecret: "bad_secret",
	}

	_, err := manager.GetToken(config)
	if err == nil {
		t.Fatal("Expected error for 401 response")
	}

	if !strings.Contains(err.Error(), "401") {
		t.Errorf("Expected error to mention 401, got: %v", err)
	}
}

func TestGetAuthTokenPriority(t *testing.T) {
	// Mock OAuth server
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(TokenResponse{
			AccessToken: "oauth_token",
			TokenType:   "Bearer",
			ExpiresIn:   3600,
		})
	}))
	defer tokenServer.Close()

	tests := []struct {
		name     string
		config   map[string]string
		expected string
	}{
		{
			name: "static token only",
			config: map[string]string{
				"token": "static_token",
			},
			expected: "static_token",
		},
		{
			name: "oauth takes priority over static",
			config: map[string]string{
				"token":         "static_token",
				"token_url":     tokenServer.URL,
				"client_id":     "test_client",
				"client_secret": "test_secret",
			},
			expected: "oauth_token",
		},
		{
			name:     "no auth configured",
			config:   map[string]string{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear the cache for each test
			globalTokenManager.ClearCache()

			token, err := getAuthToken(tt.config)
			if err != nil {
				t.Fatalf("getAuthToken failed: %v", err)
			}

			if token != tt.expected {
				t.Errorf("Expected token '%s', got '%s'", tt.expected, token)
			}
		})
	}
}

func TestFHIRActionWithOAuth(t *testing.T) {
	var receivedAuth string

	// FHIR server
	fhirServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusCreated)
	}))
	defer fhirServer.Close()

	// OAuth server
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(TokenResponse{
			AccessToken: "fhir_oauth_token_456",
			TokenType:   "Bearer",
			ExpiresIn:   3600,
		})
	}))
	defer tokenServer.Close()

	// Clear the global token cache
	globalTokenManager.ClearCache()

	event := &events.PatientAdmitEvent{
		EventMeta: events.EventMeta{Type: events.EventPatientAdmit},
		Patient:   events.Patient{MRN: "OAUTH123"},
	}

	config := map[string]string{
		"endpoint":      fhirServer.URL,
		"token_url":     tokenServer.URL,
		"client_id":     "fhir_client",
		"client_secret": "fhir_secret",
	}

	err := fhirAction(event, config)
	if err != nil {
		t.Fatalf("fhirAction with OAuth failed: %v", err)
	}

	if receivedAuth != "Bearer fhir_oauth_token_456" {
		t.Errorf("Expected Authorization 'Bearer fhir_oauth_token_456', got '%s'", receivedAuth)
	}
}

func TestOAuthClearCache(t *testing.T) {
	requestCount := 0
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(TokenResponse{
			AccessToken: fmt.Sprintf("token_%d", requestCount),
			TokenType:   "Bearer",
			ExpiresIn:   3600,
		})
	}))
	defer tokenServer.Close()

	manager := NewOAuthTokenManager()

	config := OAuthConfig{
		TokenURL:     tokenServer.URL,
		ClientID:     "clear_client",
		ClientSecret: "clear_secret",
	}

	// First call
	token1, _ := manager.GetToken(config)

	// Clear cache
	manager.ClearCache()

	// Second call should refetch
	token2, _ := manager.GetToken(config)

	if token1 == token2 {
		t.Errorf("Expected different tokens after cache clear, got same: %s", token1)
	}

	if requestCount != 2 {
		t.Errorf("Expected 2 requests after cache clear, got %d", requestCount)
	}
}

// Unused but needed to prevent compiler from complaining about time import
var _ = time.Second
