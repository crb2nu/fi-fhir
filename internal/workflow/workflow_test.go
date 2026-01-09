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

	engine := NewEngine(workflow)

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

	engine := NewEngine(workflow)

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

	engine := NewEngine(workflow)

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
