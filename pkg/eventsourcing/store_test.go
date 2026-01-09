package eventsourcing

import (
	"context"
	"testing"
	"time"
)

func TestMemoryStore_Append(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	events := []EventData{
		{EventType: "patient_admit", Data: []byte(`{"mrn":"123"}`), Metadata: map[string]string{"source": "adt"}},
	}

	// First append should succeed with VersionNone
	version, err := store.Append(ctx, "patient:123", VersionNone, events)
	if err != nil {
		t.Fatalf("Append failed: %v", err)
	}
	if version != 0 {
		t.Errorf("Expected version 0, got %d", version)
	}

	// Second append should succeed with expected version 0
	version, err = store.Append(ctx, "patient:123", 0, events)
	if err != nil {
		t.Fatalf("Second append failed: %v", err)
	}
	if version != 1 {
		t.Errorf("Expected version 1, got %d", version)
	}
}

func TestMemoryStore_ConcurrencyConflict(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	events := []EventData{
		{EventType: "patient_admit", Data: []byte(`{}`)},
	}

	// First append
	_, err := store.Append(ctx, "patient:123", VersionNone, events)
	if err != nil {
		t.Fatalf("First append failed: %v", err)
	}

	// Second append with wrong version should fail
	_, err = store.Append(ctx, "patient:123", VersionNone, events)
	if err != ErrConcurrencyConflict {
		t.Errorf("Expected ErrConcurrencyConflict, got %v", err)
	}

	// Append with wrong expected version should fail
	_, err = store.Append(ctx, "patient:123", 5, events)
	if err != ErrConcurrencyConflict {
		t.Errorf("Expected ErrConcurrencyConflict, got %v", err)
	}
}

func TestMemoryStore_VersionAny(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	events := []EventData{
		{EventType: "patient_admit", Data: []byte(`{}`)},
	}

	// VersionAny should always succeed
	for i := 0; i < 5; i++ {
		version, err := store.Append(ctx, "patient:123", VersionAny, events)
		if err != nil {
			t.Fatalf("Append with VersionAny failed: %v", err)
		}
		if version != int64(i) {
			t.Errorf("Expected version %d, got %d", i, version)
		}
	}
}

func TestMemoryStore_ReadStream(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	// Add 5 events
	for i := 0; i < 5; i++ {
		events := []EventData{
			{EventType: "event_type", Data: []byte(`{}`)},
		}
		_, err := store.Append(ctx, "stream:1", VersionAny, events)
		if err != nil {
			t.Fatalf("Append failed: %v", err)
		}
	}

	// Read all events
	events, err := store.ReadStream(ctx, "stream:1", 0, 100)
	if err != nil {
		t.Fatalf("ReadStream failed: %v", err)
	}
	if len(events) != 5 {
		t.Errorf("Expected 5 events, got %d", len(events))
	}

	// Read with offset
	events, err = store.ReadStream(ctx, "stream:1", 2, 100)
	if err != nil {
		t.Fatalf("ReadStream failed: %v", err)
	}
	if len(events) != 3 {
		t.Errorf("Expected 3 events, got %d", len(events))
	}

	// Read with limit
	events, err = store.ReadStream(ctx, "stream:1", 0, 2)
	if err != nil {
		t.Fatalf("ReadStream failed: %v", err)
	}
	if len(events) != 2 {
		t.Errorf("Expected 2 events, got %d", len(events))
	}
}

func TestMemoryStore_ReadAll(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	// Add events to different streams
	store.Append(ctx, "patient:A", VersionNone, []EventData{{EventType: "admit", Data: []byte(`{}`)}})
	store.Append(ctx, "patient:B", VersionNone, []EventData{{EventType: "admit", Data: []byte(`{}`)}})
	store.Append(ctx, "patient:A", 0, []EventData{{EventType: "discharge", Data: []byte(`{}`)}})

	// Read all events
	events, err := store.ReadAll(ctx, 0, 100)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}
	if len(events) != 3 {
		t.Errorf("Expected 3 events, got %d", len(events))
	}

	// Verify global ordering
	for i, event := range events {
		if event.Position != int64(i) {
			t.Errorf("Expected position %d, got %d", i, event.Position)
		}
	}
}

func TestMemoryStore_GetStreamVersion(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	// Non-existent stream
	version, err := store.GetStreamVersion(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("GetStreamVersion failed: %v", err)
	}
	if version != -1 {
		t.Errorf("Expected version -1 for nonexistent stream, got %d", version)
	}

	// Add events
	store.Append(ctx, "patient:123", VersionNone, []EventData{{EventType: "admit", Data: []byte(`{}`)}})
	store.Append(ctx, "patient:123", 0, []EventData{{EventType: "discharge", Data: []byte(`{}`)}})

	version, err = store.GetStreamVersion(ctx, "patient:123")
	if err != nil {
		t.Fatalf("GetStreamVersion failed: %v", err)
	}
	if version != 1 {
		t.Errorf("Expected version 1, got %d", version)
	}
}

func TestMemoryStore_Subscribe(t *testing.T) {
	store := NewMemoryStore()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Subscribe from beginning
	eventCh, err := store.Subscribe(ctx, 0)
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}

	// Add an event
	go func() {
		time.Sleep(50 * time.Millisecond)
		store.Append(context.Background(), "patient:123", VersionNone, []EventData{
			{EventType: "admit", Data: []byte(`{"mrn":"123"}`)},
		})
	}()

	// Should receive the event
	select {
	case event := <-eventCh:
		if event.EventType != "admit" {
			t.Errorf("Expected event type 'admit', got '%s'", event.EventType)
		}
	case <-time.After(time.Second):
		t.Error("Timeout waiting for subscribed event")
	}
}

func TestMemoryStore_MultipleStreams(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	// Add events to multiple streams
	streams := []string{"patient:A", "patient:B", "encounter:E1", "claim:C1"}
	for _, streamID := range streams {
		_, err := store.Append(ctx, streamID, VersionNone, []EventData{
			{EventType: "created", Data: []byte(`{}`)},
		})
		if err != nil {
			t.Fatalf("Append to %s failed: %v", streamID, err)
		}
	}

	stats := store.Stats()
	if stats.StreamCount != 4 {
		t.Errorf("Expected 4 streams, got %d", stats.StreamCount)
	}
	if stats.TotalEvents != 4 {
		t.Errorf("Expected 4 total events, got %d", stats.TotalEvents)
	}
}

func TestNewEventData(t *testing.T) {
	type TestEvent struct {
		MRN  string `json:"mrn"`
		Name string `json:"name"`
	}

	event := TestEvent{MRN: "123", Name: "John Doe"}
	metadata := map[string]string{"source": "adt", "correlation_id": "corr-123"}

	eventData, err := NewEventData("patient_admit", event, metadata)
	if err != nil {
		t.Fatalf("NewEventData failed: %v", err)
	}

	if eventData.EventType != "patient_admit" {
		t.Errorf("Expected event type 'patient_admit', got '%s'", eventData.EventType)
	}
	if eventData.Metadata["source"] != "adt" {
		t.Errorf("Expected metadata source 'adt', got '%s'", eventData.Metadata["source"])
	}
}

func TestStoredEvent_Decode(t *testing.T) {
	type TestEvent struct {
		MRN  string `json:"mrn"`
		Name string `json:"name"`
	}

	stored := StoredEvent{
		EventType: "patient_admit",
		Data:      []byte(`{"mrn":"123","name":"John Doe"}`),
	}

	var decoded TestEvent
	err := stored.Decode(&decoded)
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	if decoded.MRN != "123" {
		t.Errorf("Expected MRN '123', got '%s'", decoded.MRN)
	}
	if decoded.Name != "John Doe" {
		t.Errorf("Expected Name 'John Doe', got '%s'", decoded.Name)
	}
}

func TestStreamIDBuilders(t *testing.T) {
	tests := []struct {
		fn       func(string) string
		input    string
		expected string
	}{
		{PatientStreamID, "MRN001", "patient:MRN001"},
		{EncounterStreamID, "ENC-123", "encounter:ENC-123"},
		{ClaimStreamID, "CLM-837-001", "claim:CLM-837-001"},
		{SourceStreamID, "epic_adt", "source:epic_adt"},
	}

	for _, tc := range tests {
		result := tc.fn(tc.input)
		if result != tc.expected {
			t.Errorf("Expected '%s', got '%s'", tc.expected, result)
		}
	}
}

func TestMemoryStore_Clear(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	// Add events
	store.Append(ctx, "patient:123", VersionNone, []EventData{{EventType: "admit", Data: []byte(`{}`)}})

	stats := store.Stats()
	if stats.TotalEvents != 1 {
		t.Errorf("Expected 1 event before clear, got %d", stats.TotalEvents)
	}

	// Clear
	store.Clear()

	stats = store.Stats()
	if stats.TotalEvents != 0 {
		t.Errorf("Expected 0 events after clear, got %d", stats.TotalEvents)
	}
	if stats.StreamCount != 0 {
		t.Errorf("Expected 0 streams after clear, got %d", stats.StreamCount)
	}
}
