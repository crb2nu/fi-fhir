package workflow

import (
	"context"
	"testing"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/eventsourcing"
)

func TestParseEventStoreConfig(t *testing.T) {
	tests := []struct {
		name        string
		config      map[string]string
		wantErr     bool
		errContains string
		check       func(t *testing.T, c *EventStoreConfig)
	}{
		{
			name: "valid config",
			config: map[string]string{
				"connection":      "postgres://localhost/test",
				"table":           "events",
				"stream_template": "patient:{{.Patient.MRN}}",
			},
			wantErr: false,
			check: func(t *testing.T, c *EventStoreConfig) {
				if c.Connection != "postgres://localhost/test" {
					t.Errorf("Connection = %q, want %q", c.Connection, "postgres://localhost/test")
				}
				if c.Table != "events" {
					t.Errorf("Table = %q, want %q", c.Table, "events")
				}
				if c.StreamTemplate != "patient:{{.Patient.MRN}}" {
					t.Errorf("StreamTemplate = %q, want %q", c.StreamTemplate, "patient:{{.Patient.MRN}}")
				}
			},
		},
		{
			name: "dsn alias",
			config: map[string]string{
				"dsn":    "postgres://localhost/test",
				"stream": "test:stream",
			},
			wantErr: false,
			check: func(t *testing.T, c *EventStoreConfig) {
				if c.Connection != "postgres://localhost/test" {
					t.Errorf("Connection = %q, want %q", c.Connection, "postgres://localhost/test")
				}
			},
		},
		{
			name: "db alias",
			config: map[string]string{
				"db":     "postgres://localhost/test",
				"stream": "test:stream",
			},
			wantErr: false,
			check: func(t *testing.T, c *EventStoreConfig) {
				if c.Connection != "postgres://localhost/test" {
					t.Errorf("Connection = %q, want %q", c.Connection, "postgres://localhost/test")
				}
			},
		},
		{
			name: "default table",
			config: map[string]string{
				"connection": "postgres://localhost/test",
				"stream":     "test:stream",
			},
			wantErr: false,
			check: func(t *testing.T, c *EventStoreConfig) {
				if c.Table != "events" {
					t.Errorf("Table = %q, want default %q", c.Table, "events")
				}
			},
		},
		{
			name: "with metadata",
			config: map[string]string{
				"connection":      "postgres://localhost/test",
				"stream":          "test:stream",
				"metadata_source": "adt_interface",
				"metadata_env":    "production",
			},
			wantErr: false,
			check: func(t *testing.T, c *EventStoreConfig) {
				if c.Metadata["source"] != "adt_interface" {
					t.Errorf("Metadata[source] = %q, want %q", c.Metadata["source"], "adt_interface")
				}
				if c.Metadata["env"] != "production" {
					t.Errorf("Metadata[env] = %q, want %q", c.Metadata["env"], "production")
				}
			},
		},
		{
			name: "with event_type override",
			config: map[string]string{
				"connection": "postgres://localhost/test",
				"stream":     "test:stream",
				"event_type": "custom_event",
			},
			wantErr: false,
			check: func(t *testing.T, c *EventStoreConfig) {
				if c.EventType != "custom_event" {
					t.Errorf("EventType = %q, want %q", c.EventType, "custom_event")
				}
			},
		},
		{
			name: "missing connection",
			config: map[string]string{
				"stream": "test:stream",
			},
			wantErr:     true,
			errContains: "connection",
		},
		{
			name: "missing stream",
			config: map[string]string{
				"connection": "postgres://localhost/test",
			},
			wantErr:     true,
			errContains: "stream",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := parseEventStoreConfig(tt.config)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errContains != "" && !containsSubstr(err.Error(), tt.errContains) {
					t.Errorf("error = %q, want to contain %q", err.Error(), tt.errContains)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.check != nil {
				tt.check(t, c)
			}
		})
	}
}

// Note: contains function is already defined in ratelimit_test.go

func containsSubstr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstrInStr(s, substr)))
}

func findSubstrInStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Note: extractEventType tests are in replay_test.go since the function is defined there

// TestEventStoreActionWithMemoryStore tests the event store action logic
// using an in-memory store (doesn't require PostgreSQL)
func TestEventStoreActionIntegration(t *testing.T) {
	// Create a test event
	event := map[string]interface{}{
		"type": "patient_admit",
		"patient": map[string]interface{}{
			"mrn":  "MRN001",
			"name": "John Doe",
		},
		"encounter": map[string]interface{}{
			"id":    "ENC001",
			"class": "inpatient",
		},
	}

	// Test template rendering for stream ID
	streamTemplate := "patient:{{.patient.mrn}}"
	streamID := renderTemplate(streamTemplate, event)
	if streamID != "patient:MRN001" {
		t.Errorf("Stream ID = %q, want %q", streamID, "patient:MRN001")
	}

	// Test event type extraction
	eventType := extractEventType(event)
	if eventType != "patient_admit" {
		t.Errorf("Event type = %q, want %q", eventType, "patient_admit")
	}
}

// TestEventStoreManager tests the connection manager
func TestEventStoreManager(t *testing.T) {
	mgr := NewEventStoreManager()
	defer func() { _ = mgr.Close() }()

	// Manager should be empty initially
	if len(mgr.stores) != 0 {
		t.Errorf("Expected 0 stores, got %d", len(mgr.stores))
	}

	// Close should work on empty manager
	if err := mgr.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}
}

// TestEventStoreActionValidation tests configuration validation
func TestEventStoreActionValidation(t *testing.T) {
	ctx := context.Background()

	// Missing connection should fail
	err := eventStoreAction(ctx, map[string]interface{}{"type": "test"}, map[string]string{
		"stream": "test:stream",
	})
	if err == nil {
		t.Error("Expected error for missing connection")
	}

	// Missing stream should fail
	err = eventStoreAction(ctx, map[string]interface{}{"type": "test"}, map[string]string{
		"connection": "postgres://localhost/test",
	})
	if err == nil {
		t.Error("Expected error for missing stream")
	}
}

// TestEventStoreInWorkflow tests the action is properly registered in the engine
func TestEventStoreInWorkflow(t *testing.T) {
	workflow := &Workflow{
		Name: "test",
		Routes: []Route{
			{
				Name: "store_events",
				Filter: Filter{
					EventType: StringOrSlice{"*"},
				},
				Actions: []Action{
					{
						Type: "event_store",
						Config: map[string]string{
							"connection": "postgres://localhost/test",
							"stream":     "test:{{.type}}",
						},
					},
				},
			},
		},
	}

	engine, err := NewEngine(workflow)
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}

	// Verify event_store action is registered
	if _, exists := engine.actions["event_store"]; !exists {
		t.Error("event_store action not registered in engine")
	}
}

// MockEventStore for testing without a real database
type MockEventStore struct {
	events []eventsourcing.EventData
}

func (m *MockEventStore) Append(ctx context.Context, streamID string, expectedVersion int64, events []eventsourcing.EventData) (int64, error) {
	m.events = append(m.events, events...)
	return int64(len(m.events) - 1), nil
}
