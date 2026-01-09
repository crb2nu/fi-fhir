package workflow

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

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
