//nolint:gosec,errcheck // Test file - G104 errors intentionally ignored in test setup
package eventsourcing

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

// =============================================================================
// Test Aggregate Implementation
// =============================================================================

// testPatientAggregate is a simple aggregate for testing compaction.
type testPatientAggregate struct {
	JSONAggregate
	MRN        string   `json:"mrn"`
	Name       string   `json:"name"`
	EventCount int      `json:"event_count"`
	Events     []string `json:"events"`
}

func newTestPatientAggregate(streamID string) *testPatientAggregate {
	return &testPatientAggregate{
		JSONAggregate: NewJSONAggregate("Patient", streamID),
		Events:        make([]string, 0),
	}
}

func (p *testPatientAggregate) CompactSnapshot() ([]byte, error) {
	return p.MarshalState(p)
}

func (p *testPatientAggregate) RestoreFromSnapshot(data []byte) error {
	return p.UnmarshalState(data, p)
}

func (p *testPatientAggregate) Apply(event StoredEvent) error {
	p.EventCount++

	var payload map[string]interface{}
	if err := json.Unmarshal(event.Data, &payload); err == nil {
		if name, ok := payload["name"].(string); ok {
			p.Name = name
		}
		if mrn, ok := payload["mrn"].(string); ok {
			p.MRN = mrn
		}
	}

	p.Events = append(p.Events, event.EventType)
	return nil
}

// =============================================================================
// Unit Tests
// =============================================================================

func TestMemoryStreamSnapshotStore(t *testing.T) {
	store := NewMemoryStreamSnapshotStore()
	ctx := context.Background()

	// Test save and get
	snapshot := StreamSnapshot{
		StreamID:      "patient:123",
		Version:       5,
		Position:      100,
		AggregateType: "Patient",
		Data:          []byte(`{"name":"John"}`),
		EventCount:    6,
	}

	if err := store.SaveStreamSnapshot(ctx, snapshot); err != nil {
		t.Fatalf("SaveStreamSnapshot failed: %v", err)
	}

	// Retrieve
	got, err := store.GetStreamSnapshot(ctx, "patient:123")
	if err != nil {
		t.Fatalf("GetStreamSnapshot failed: %v", err)
	}
	if got == nil {
		t.Fatal("Expected snapshot, got nil")
	}
	if got.Version != 5 {
		t.Errorf("Version = %d, want 5", got.Version)
	}
	if got.EventCount != 6 {
		t.Errorf("EventCount = %d, want 6", got.EventCount)
	}

	// List
	list, err := store.ListStreamSnapshots(ctx)
	if err != nil {
		t.Fatalf("ListStreamSnapshots failed: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("Expected 1 snapshot, got %d", len(list))
	}

	// Delete
	if err := store.DeleteStreamSnapshot(ctx, "patient:123"); err != nil {
		t.Fatalf("DeleteStreamSnapshot failed: %v", err)
	}

	got, _ = store.GetStreamSnapshot(ctx, "patient:123")
	if got != nil {
		t.Error("Expected nil after delete")
	}
}

func TestStreamCompactor_CompactStream(t *testing.T) {
	eventStore := NewMemoryStore()
	snapshotStore := NewMemoryStreamSnapshotStore()
	compactor := NewStreamCompactor(eventStore, snapshotStore)

	ctx := context.Background()

	// Add events to a patient stream
	streamID := "patient:P001"
	for i := 0; i < 10; i++ {
		eventStore.Append(ctx, streamID, VersionAny, []EventData{
			{
				EventType: "PatientUpdated",
				Data:      json.RawMessage(`{"name":"John Doe","mrn":"P001"}`),
			},
		})
	}

	// Create aggregate loader
	loader := func(sid string) Compactable {
		return newTestPatientAggregate(sid)
	}

	// Compact with minimal requirements
	config := &CompactionConfig{
		MinEvents: 5,
		MinAge:    0, // No age requirement for testing
	}

	result, err := compactor.CompactStream(ctx, streamID, loader, config)
	if err != nil {
		t.Fatalf("CompactStream failed: %v", err)
	}

	if result.EventsProcessed != 10 {
		t.Errorf("EventsProcessed = %d, want 10", result.EventsProcessed)
	}
	if result.SnapshotVersion != 9 { // 0-indexed, so 10 events = version 9
		t.Errorf("SnapshotVersion = %d, want 9", result.SnapshotVersion)
	}
	if result.AggregateType != "Patient" {
		t.Errorf("AggregateType = %s, want Patient", result.AggregateType)
	}

	// Verify snapshot was created
	snap, _ := snapshotStore.GetStreamSnapshot(ctx, streamID)
	if snap == nil {
		t.Fatal("Expected snapshot to be created")
	}
	if snap.EventCount != 10 {
		t.Errorf("Snapshot EventCount = %d, want 10", snap.EventCount)
	}
}

func TestStreamCompactor_CompactStream_NotEnoughEvents(t *testing.T) {
	eventStore := NewMemoryStore()
	snapshotStore := NewMemoryStreamSnapshotStore()
	compactor := NewStreamCompactor(eventStore, snapshotStore)

	ctx := context.Background()

	// Add only 3 events
	streamID := "patient:P002"
	for i := 0; i < 3; i++ {
		eventStore.Append(ctx, streamID, VersionAny, []EventData{
			{EventType: "PatientUpdated", Data: json.RawMessage(`{}`)},
		})
	}

	loader := func(sid string) Compactable {
		return newTestPatientAggregate(sid)
	}

	// Require 100 events minimum
	config := &CompactionConfig{
		MinEvents: 100,
		MinAge:    0,
	}

	result, err := compactor.CompactStream(ctx, streamID, loader, config)
	if err != nil {
		t.Fatalf("CompactStream failed: %v", err)
	}

	// Should not have created snapshot (not enough events)
	if result.SnapshotVersion != 0 {
		t.Errorf("Expected no snapshot (version 0), got %d", result.SnapshotVersion)
	}

	snap, _ := snapshotStore.GetStreamSnapshot(ctx, streamID)
	if snap != nil {
		t.Error("Expected no snapshot to be created")
	}
}

func TestStreamCompactor_CompactStream_DryRun(t *testing.T) {
	eventStore := NewMemoryStore()
	snapshotStore := NewMemoryStreamSnapshotStore()
	compactor := NewStreamCompactor(eventStore, snapshotStore)

	ctx := context.Background()

	// Add events
	streamID := "patient:P003"
	for i := 0; i < 20; i++ {
		eventStore.Append(ctx, streamID, VersionAny, []EventData{
			{EventType: "PatientUpdated", Data: json.RawMessage(`{}`)},
		})
	}

	loader := func(sid string) Compactable {
		return newTestPatientAggregate(sid)
	}

	config := &CompactionConfig{
		MinEvents: 5,
		MinAge:    0,
		DryRun:    true,
	}

	result, err := compactor.CompactStream(ctx, streamID, loader, config)
	if err != nil {
		t.Fatalf("CompactStream failed: %v", err)
	}

	if !result.DryRun {
		t.Error("Expected DryRun to be true")
	}
	if result.EventsProcessed != 20 {
		t.Errorf("EventsProcessed = %d, want 20", result.EventsProcessed)
	}

	// Verify no snapshot was actually created
	snap, _ := snapshotStore.GetStreamSnapshot(ctx, streamID)
	if snap != nil {
		t.Error("Expected no snapshot in dry run mode")
	}
}

func TestStreamCompactor_GetStreamWithSnapshot(t *testing.T) {
	eventStore := NewMemoryStore()
	snapshotStore := NewMemoryStreamSnapshotStore()
	compactor := NewStreamCompactor(eventStore, snapshotStore)

	ctx := context.Background()
	streamID := "patient:P004"

	// Add initial events
	for i := 0; i < 5; i++ {
		eventStore.Append(ctx, streamID, VersionAny, []EventData{
			{
				EventType: "PatientUpdated",
				Data:      json.RawMessage(`{"name":"Initial"}`),
			},
		})
	}

	// Create a snapshot at version 4
	snapshotStore.SaveStreamSnapshot(ctx, StreamSnapshot{
		StreamID:      streamID,
		Version:       4,
		Position:      4,
		AggregateType: "Patient",
		Data:          []byte(`{"mrn":"P004","name":"Snapshot State","event_count":5,"events":["PatientUpdated","PatientUpdated","PatientUpdated","PatientUpdated","PatientUpdated"]}`),
		EventCount:    5,
	})

	// Add more events after snapshot
	for i := 0; i < 3; i++ {
		eventStore.Append(ctx, streamID, VersionAny, []EventData{
			{
				EventType: "PatientUpdated",
				Data:      json.RawMessage(`{"name":"After Snapshot"}`),
			},
		})
	}

	// Load aggregate with snapshot
	aggregate := newTestPatientAggregate(streamID)
	version, err := compactor.GetStreamWithSnapshot(ctx, streamID, aggregate)
	if err != nil {
		t.Fatalf("GetStreamWithSnapshot failed: %v", err)
	}

	if version != 7 { // 5 original + 3 new = 8 events, version 7 (0-indexed)
		t.Errorf("Version = %d, want 7", version)
	}

	// Should have applied 3 events after restoring snapshot (which had 5)
	if aggregate.EventCount != 8 { // 5 from snapshot + 3 applied
		t.Errorf("EventCount = %d, want 8", aggregate.EventCount)
	}
	if aggregate.Name != "After Snapshot" {
		t.Errorf("Name = %s, want 'After Snapshot'", aggregate.Name)
	}
}

func TestStreamCompactor_IncrementalCompaction(t *testing.T) {
	eventStore := NewMemoryStore()
	snapshotStore := NewMemoryStreamSnapshotStore()
	compactor := NewStreamCompactor(eventStore, snapshotStore)

	ctx := context.Background()
	streamID := "patient:P005"

	// Add first batch of events
	for i := 0; i < 10; i++ {
		eventStore.Append(ctx, streamID, VersionAny, []EventData{
			{EventType: "PatientUpdated", Data: json.RawMessage(`{"name":"Batch1"}`)},
		})
	}

	loader := func(sid string) Compactable {
		return newTestPatientAggregate(sid)
	}

	config := &CompactionConfig{
		MinEvents: 5,
		MinAge:    0,
	}

	// First compaction
	result1, err := compactor.CompactStream(ctx, streamID, loader, config)
	if err != nil {
		t.Fatalf("First compaction failed: %v", err)
	}
	if result1.SnapshotVersion != 9 {
		t.Errorf("First snapshot version = %d, want 9", result1.SnapshotVersion)
	}

	// Add more events
	for i := 0; i < 10; i++ {
		eventStore.Append(ctx, streamID, VersionAny, []EventData{
			{EventType: "PatientUpdated", Data: json.RawMessage(`{"name":"Batch2"}`)},
		})
	}

	// Second compaction (should start from snapshot)
	result2, err := compactor.CompactStream(ctx, streamID, loader, config)
	if err != nil {
		t.Fatalf("Second compaction failed: %v", err)
	}

	// Should only process the new 10 events
	if result2.EventsProcessed != 10 {
		t.Errorf("Second compaction processed %d events, want 10", result2.EventsProcessed)
	}

	// Final snapshot should be at version 19
	if result2.SnapshotVersion != 19 {
		t.Errorf("Second snapshot version = %d, want 19", result2.SnapshotVersion)
	}

	// Verify snapshot has correct total event count
	snap, _ := snapshotStore.GetStreamSnapshot(ctx, streamID)
	if snap == nil {
		t.Fatal("Expected snapshot")
	}
	if snap.EventCount != 20 { // 10 from first + 10 from second
		t.Errorf("Snapshot EventCount = %d, want 20", snap.EventCount)
	}
}

func TestStreamCompactor_CompactStreamsByPrefix(t *testing.T) {
	eventStore := NewMemoryStore()
	snapshotStore := NewMemoryStreamSnapshotStore()
	compactor := NewStreamCompactor(eventStore, snapshotStore)

	ctx := context.Background()

	// Create multiple patient streams
	for _, mrn := range []string{"P001", "P002", "P003"} {
		streamID := "patient:" + mrn
		for i := 0; i < 10; i++ {
			eventStore.Append(ctx, streamID, VersionAny, []EventData{
				{EventType: "PatientUpdated", Data: json.RawMessage(`{}`)},
			})
		}
	}

	// Create a non-patient stream
	eventStore.Append(ctx, "claim:C001", VersionAny, []EventData{
		{EventType: "ClaimCreated", Data: json.RawMessage(`{}`)},
	})

	loader := func(sid string) Compactable {
		return newTestPatientAggregate(sid)
	}

	config := &CompactionConfig{
		MinEvents: 5,
		MinAge:    0,
	}

	result, err := compactor.CompactStreamsByPrefix(ctx, "patient:", loader, config)
	if err != nil {
		t.Fatalf("CompactStreamsByPrefix failed: %v", err)
	}

	if result.StreamsProcessed != 3 {
		t.Errorf("StreamsProcessed = %d, want 3", result.StreamsProcessed)
	}
	if result.StreamsCompacted != 3 {
		t.Errorf("StreamsCompacted = %d, want 3", result.StreamsCompacted)
	}
	if result.TotalEvents != 30 {
		t.Errorf("TotalEvents = %d, want 30", result.TotalEvents)
	}
}

func TestDefaultCompactionConfig(t *testing.T) {
	config := DefaultCompactionConfig()

	if config.MinEvents != 100 {
		t.Errorf("MinEvents = %d, want 100", config.MinEvents)
	}
	if config.MinAge != 24*time.Hour {
		t.Errorf("MinAge = %v, want 24h", config.MinAge)
	}
	if config.MaxEvents != 10000 {
		t.Errorf("MaxEvents = %d, want 10000", config.MaxEvents)
	}
	if config.DeleteAfterCompaction {
		t.Error("DeleteAfterCompaction should be false by default")
	}
	if !config.ArchiveBeforeDelete {
		t.Error("ArchiveBeforeDelete should be true by default")
	}
}

func TestJSONAggregate(t *testing.T) {
	agg := NewJSONAggregate("TestType", "test:123")

	if agg.AggregateType() != "TestType" {
		t.Errorf("AggregateType = %s, want TestType", agg.AggregateType())
	}
	if agg.StreamID() != "test:123" {
		t.Errorf("StreamID = %s, want test:123", agg.StreamID())
	}

	// Test marshal/unmarshal
	state := map[string]string{"key": "value"}
	data, err := agg.MarshalState(state)
	if err != nil {
		t.Fatalf("MarshalState failed: %v", err)
	}

	var restored map[string]string
	if err := agg.UnmarshalState(data, &restored); err != nil {
		t.Fatalf("UnmarshalState failed: %v", err)
	}

	if restored["key"] != "value" {
		t.Errorf("Restored key = %s, want value", restored["key"])
	}
}
