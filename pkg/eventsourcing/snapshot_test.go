package eventsourcing

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestMemorySnapshotStore_SaveAndGet(t *testing.T) {
	store := NewMemorySnapshotStore()
	ctx := context.Background()

	snapshot := Snapshot{
		ProjectionName: "test_projection",
		Position:       100,
		Data:           []byte(`{"key":"value"}`),
		CreatedAt:      time.Now(),
	}

	err := store.SaveSnapshot(ctx, snapshot)
	if err != nil {
		t.Fatalf("SaveSnapshot failed: %v", err)
	}

	latest, err := store.GetLatestSnapshot(ctx, "test_projection")
	if err != nil {
		t.Fatalf("GetLatestSnapshot failed: %v", err)
	}

	if latest == nil {
		t.Fatal("Expected snapshot, got nil")
	}

	if latest.Position != 100 {
		t.Errorf("Expected position 100, got %d", latest.Position)
	}
}

func TestMemorySnapshotStore_LatestOnly(t *testing.T) {
	store := NewMemorySnapshotStore()
	ctx := context.Background()

	// Save multiple snapshots
	for i := int64(0); i < 5; i++ {
		err := store.SaveSnapshot(ctx, Snapshot{
			ProjectionName: "test_projection",
			Position:       i * 100,
			Data:           []byte(`{}`),
		})
		if err != nil {
			t.Fatalf("SaveSnapshot failed: %v", err)
		}
	}

	// Should return the latest (position 400)
	latest, err := store.GetLatestSnapshot(ctx, "test_projection")
	if err != nil {
		t.Fatalf("GetLatestSnapshot failed: %v", err)
	}

	if latest.Position != 400 {
		t.Errorf("Expected position 400, got %d", latest.Position)
	}
}

func TestMemorySnapshotStore_NoSnapshot(t *testing.T) {
	store := NewMemorySnapshotStore()
	ctx := context.Background()

	latest, err := store.GetLatestSnapshot(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("GetLatestSnapshot failed: %v", err)
	}

	if latest != nil {
		t.Error("Expected nil for nonexistent projection")
	}
}

func TestMemorySnapshotStore_DeleteSnapshots(t *testing.T) {
	store := NewMemorySnapshotStore()
	ctx := context.Background()

	err := store.SaveSnapshot(ctx, Snapshot{
		ProjectionName: "test_projection",
		Position:       100,
		Data:           []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("SaveSnapshot failed: %v", err)
	}

	err = store.DeleteSnapshots(ctx, "test_projection")
	if err != nil {
		t.Fatalf("DeleteSnapshots failed: %v", err)
	}

	latest, err := store.GetLatestSnapshot(ctx, "test_projection")
	if err != nil {
		t.Fatalf("GetLatestSnapshot failed: %v", err)
	}

	if latest != nil {
		t.Error("Expected nil after delete")
	}
}

func TestSnapshotManager_ShouldSnapshot(t *testing.T) {
	store := NewMemorySnapshotStore()
	config := SnapshotConfig{
		Enabled:  true,
		Interval: 10,
		MinAge:   0, // No minimum age for testing
	}
	mgr := NewSnapshotManager(store, config)

	// Initially should not snapshot
	if mgr.ShouldSnapshot("test") {
		t.Error("Should not snapshot with 0 events")
	}

	// Record 9 events - still below threshold
	for i := 0; i < 9; i++ {
		mgr.RecordEvent("test")
	}
	if mgr.ShouldSnapshot("test") {
		t.Error("Should not snapshot with 9 events (threshold is 10)")
	}

	// Record 1 more - now at threshold
	mgr.RecordEvent("test")
	if !mgr.ShouldSnapshot("test") {
		t.Error("Should snapshot with 10 events")
	}
}

func TestSnapshotManager_ShouldSnapshot_Disabled(t *testing.T) {
	store := NewMemorySnapshotStore()
	config := SnapshotConfig{
		Enabled:  false,
		Interval: 10,
	}
	mgr := NewSnapshotManager(store, config)

	// Record many events
	for i := 0; i < 100; i++ {
		mgr.RecordEvent("test")
	}

	if mgr.ShouldSnapshot("test") {
		t.Error("Should never snapshot when disabled")
	}
}

func TestSnapshotManager_ShouldSnapshot_MinAge(t *testing.T) {
	store := NewMemorySnapshotStore()
	config := SnapshotConfig{
		Enabled:  true,
		Interval: 10,
		MinAge:   1 * time.Hour, // Long minimum age
	}
	mgr := NewSnapshotManager(store, config)

	// Record events and simulate a snapshot was just taken
	for i := 0; i < 100; i++ {
		mgr.RecordEvent("test")
	}
	mgr.lastSnap["test"] = time.Now()

	if mgr.ShouldSnapshot("test") {
		t.Error("Should not snapshot due to minAge")
	}
}

// mockSnapshotableProjection implements both Projection and Snapshotable
type mockSnapshotableProjection struct {
	name  string
	state map[string]int
}

func newMockSnapshotableProjection(name string) *mockSnapshotableProjection {
	return &mockSnapshotableProjection{
		name:  name,
		state: make(map[string]int),
	}
}

func (p *mockSnapshotableProjection) Name() string {
	return p.name
}

func (p *mockSnapshotableProjection) Handle(ctx context.Context, event StoredEvent) error {
	p.state[event.EventType]++
	return nil
}

func (p *mockSnapshotableProjection) Snapshot() ([]byte, error) {
	return json.Marshal(p.state)
}

func (p *mockSnapshotableProjection) Restore(data []byte) error {
	return json.Unmarshal(data, &p.state)
}

func (p *mockSnapshotableProjection) Clear() {
	p.state = make(map[string]int)
}

func TestSnapshotManager_TakeAndRestoreSnapshot(t *testing.T) {
	ctx := context.Background()
	store := NewMemorySnapshotStore()
	config := DefaultSnapshotConfig()
	mgr := NewSnapshotManager(store, config)

	// Create projection with some state
	projection := newMockSnapshotableProjection("test")
	projection.state["event_a"] = 5
	projection.state["event_b"] = 10

	// Take snapshot
	err := mgr.TakeSnapshot(ctx, projection, 100)
	if err != nil {
		t.Fatalf("TakeSnapshot failed: %v", err)
	}

	// Clear projection state
	projection.Clear()
	if len(projection.state) != 0 {
		t.Error("State should be empty after clear")
	}

	// Restore from snapshot
	position, err := mgr.RestoreSnapshot(ctx, projection)
	if err != nil {
		t.Fatalf("RestoreSnapshot failed: %v", err)
	}

	if position != 101 {
		t.Errorf("Expected resume position 101, got %d", position)
	}

	if projection.state["event_a"] != 5 {
		t.Errorf("Expected event_a=5, got %d", projection.state["event_a"])
	}
	if projection.state["event_b"] != 10 {
		t.Errorf("Expected event_b=10, got %d", projection.state["event_b"])
	}
}

func TestSnapshotManager_RestoreNoSnapshot(t *testing.T) {
	ctx := context.Background()
	store := NewMemorySnapshotStore()
	config := DefaultSnapshotConfig()
	mgr := NewSnapshotManager(store, config)

	projection := newMockSnapshotableProjection("test")

	// No snapshot exists
	position, err := mgr.RestoreSnapshot(ctx, projection)
	if err != nil {
		t.Fatalf("RestoreSnapshot failed: %v", err)
	}

	if position != 0 {
		t.Errorf("Expected position 0 when no snapshot, got %d", position)
	}
}

// mockNonSnapshotableProjection only implements Projection
type mockNonSnapshotableProjection struct {
	name string
}

func (p *mockNonSnapshotableProjection) Name() string { return p.name }
func (p *mockNonSnapshotableProjection) Handle(ctx context.Context, event StoredEvent) error {
	return nil
}

func TestSnapshotManager_NonSnapshotableProjection(t *testing.T) {
	ctx := context.Background()
	store := NewMemorySnapshotStore()
	config := DefaultSnapshotConfig()
	mgr := NewSnapshotManager(store, config)

	projection := &mockNonSnapshotableProjection{name: "non_snapshotable"}

	// Should not error on take
	err := mgr.TakeSnapshot(ctx, projection, 100)
	if err != nil {
		t.Errorf("TakeSnapshot should not error for non-snapshotable: %v", err)
	}

	// Should return 0 on restore
	position, err := mgr.RestoreSnapshot(ctx, projection)
	if err != nil {
		t.Errorf("RestoreSnapshot should not error: %v", err)
	}
	if position != 0 {
		t.Errorf("Expected position 0, got %d", position)
	}
}

func TestSnapshotAwareRunner_RebuildWithSnapshots(t *testing.T) {
	ctx := context.Background()
	eventStore := NewMemoryStore()
	checkpointStore := NewMemoryCheckpointStore()
	snapshotStore := NewMemorySnapshotStore()

	// Add events
	eventStore.Append(ctx, "stream:1", VersionNone, []EventData{
		{EventType: "event_a", Data: []byte(`{}`)},
	})
	eventStore.Append(ctx, "stream:2", VersionNone, []EventData{
		{EventType: "event_b", Data: []byte(`{}`)},
	})
	eventStore.Append(ctx, "stream:3", VersionNone, []EventData{
		{EventType: "event_a", Data: []byte(`{}`)},
	})

	// Create runner
	config := DefaultProjectionRunnerConfig()
	snapshotConfig := SnapshotConfig{
		Enabled:  true,
		Interval: 2, // Snapshot every 2 events
		MinAge:   0,
	}
	runner := NewSnapshotAwareRunner(eventStore, checkpointStore, snapshotStore, config, snapshotConfig)

	// Register projection
	projection := newMockSnapshotableProjection("test")
	runner.RegisterProjection(projection)

	// First run - processes all events
	err := runner.RebuildWithSnapshots(ctx, "test")
	if err != nil {
		t.Fatalf("RebuildWithSnapshots failed: %v", err)
	}

	if projection.state["event_a"] != 2 {
		t.Errorf("Expected event_a=2, got %d", projection.state["event_a"])
	}
	if projection.state["event_b"] != 1 {
		t.Errorf("Expected event_b=1, got %d", projection.state["event_b"])
	}

	// Should have created a snapshot
	snapshot, _ := snapshotStore.GetLatestSnapshot(ctx, "test")
	if snapshot == nil {
		t.Error("Expected snapshot to be created")
	}
}
