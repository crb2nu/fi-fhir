//nolint:gosec,errcheck // Test file - G104 errors intentionally ignored in test setup
package eventsourcing

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// testRebuildProjection is a simple counting projection for testing.
type testRebuildProjection struct {
	name   string
	count  int64
	events []StoredEvent
}

func newTestRebuildProjection(name string) *testRebuildProjection {
	return &testRebuildProjection{name: name}
}

func (p *testRebuildProjection) Name() string { return p.name }

func (p *testRebuildProjection) Handle(ctx context.Context, event StoredEvent) error {
	p.count++
	p.events = append(p.events, event)
	return nil
}

func (p *testRebuildProjection) Clear() {
	p.count = 0
	p.events = nil
}

func (p *testRebuildProjection) Snapshot() ([]byte, error) {
	return json.Marshal(map[string]interface{}{
		"count":  p.count,
		"events": len(p.events),
	})
}

func (p *testRebuildProjection) Restore(data []byte) error {
	var state map[string]interface{}
	if err := json.Unmarshal(data, &state); err != nil {
		return err
	}
	p.count = int64(state["count"].(float64))
	return nil
}

func TestProjectionRebuilder_Rebuild(t *testing.T) {
	store := NewMemoryStore()
	checkpointStore := NewMemoryCheckpointStore()
	snapshotStore := NewMemorySnapshotStore()
	ctx := context.Background()

	// Add events
	for i := 0; i < 10; i++ {
		store.Append(ctx, "stream-1", VersionAny, []EventData{{
			EventType: "test_event",
			Data:      []byte(`{}`),
		}})
	}

	// Create rebuilder
	rebuilder := NewProjectionRebuilder(store, checkpointStore, snapshotStore)
	projection := newTestRebuildProjection("test_projection")
	rebuilder.RegisterProjection(projection)

	// Rebuild
	result, err := rebuilder.Rebuild(ctx, "test_projection", nil)
	if err != nil {
		t.Fatalf("Rebuild failed: %v", err)
	}

	if result.EventsProcessed != 10 {
		t.Errorf("Expected 10 events processed, got %d", result.EventsProcessed)
	}

	if projection.count != 10 {
		t.Errorf("Expected projection count 10, got %d", projection.count)
	}
}

func TestProjectionRebuilder_RebuildWithProgress(t *testing.T) {
	store := NewMemoryStore()
	checkpointStore := NewMemoryCheckpointStore()
	ctx := context.Background()

	// Add events
	for i := 0; i < 25; i++ {
		store.Append(ctx, "stream-1", VersionAny, []EventData{{
			EventType: "test_event",
			Data:      []byte(`{}`),
		}})
	}

	rebuilder := NewProjectionRebuilder(store, checkpointStore, nil)
	projection := newTestRebuildProjection("test_projection")
	rebuilder.RegisterProjection(projection)

	// Track progress callbacks
	var progressCalls int32

	result, err := rebuilder.Rebuild(ctx, "test_projection", &RebuildConfig{
		BatchSize: 5,
		Progress: func(stats *RebuildProgress) {
			atomic.AddInt32(&progressCalls, 1)
		},
	})

	if err != nil {
		t.Fatalf("Rebuild failed: %v", err)
	}

	// Should have 5 batches + 1 final callback = 6 calls
	if progressCalls < 5 {
		t.Errorf("Expected at least 5 progress callbacks, got %d", progressCalls)
	}

	if result.EventsProcessed != 25 {
		t.Errorf("Expected 25 events, got %d", result.EventsProcessed)
	}
}

func TestProjectionRebuilder_RebuildFromSnapshot(t *testing.T) {
	store := NewMemoryStore()
	checkpointStore := NewMemoryCheckpointStore()
	snapshotStore := NewMemorySnapshotStore()
	ctx := context.Background()

	// Add events
	for i := 0; i < 20; i++ {
		store.Append(ctx, "stream-1", VersionAny, []EventData{{
			EventType: "test_event",
			Data:      []byte(`{}`),
		}})
	}

	// Create snapshot at position 10
	snapshotStore.SaveSnapshot(ctx, Snapshot{
		ProjectionName: "test_projection",
		Position:       9, // After 10 events (positions 0-9)
		Data:           []byte(`{"count":10,"events":10}`),
	})

	rebuilder := NewProjectionRebuilder(store, checkpointStore, snapshotStore)
	projection := newTestRebuildProjection("test_projection")
	rebuilder.RegisterProjection(projection)

	// Rebuild from snapshot
	result, err := rebuilder.Rebuild(ctx, "test_projection", &RebuildConfig{
		FromSnapshot: true,
	})

	if err != nil {
		t.Fatalf("Rebuild failed: %v", err)
	}

	if !result.SnapshotRestored {
		t.Error("Expected snapshot to be restored")
	}

	if result.SnapshotPosition != 9 {
		t.Errorf("Expected snapshot position 9, got %d", result.SnapshotPosition)
	}

	// Should only process 10 events (positions 10-19)
	if result.EventsProcessed != 10 {
		t.Errorf("Expected 10 events processed after snapshot, got %d", result.EventsProcessed)
	}

	// Projection should have count=10 (from snapshot) + 10 more = 20
	if projection.count != 20 {
		t.Errorf("Expected projection count 20, got %d", projection.count)
	}
}

func TestProjectionRebuilder_DryRun(t *testing.T) {
	store := NewMemoryStore()
	checkpointStore := NewMemoryCheckpointStore()
	ctx := context.Background()

	// Add events
	for i := 0; i < 10; i++ {
		store.Append(ctx, "stream-1", VersionAny, []EventData{{
			EventType: "test_event",
			Data:      []byte(`{}`),
		}})
	}

	rebuilder := NewProjectionRebuilder(store, checkpointStore, nil)
	projection := newTestRebuildProjection("test_projection")
	rebuilder.RegisterProjection(projection)

	// Dry run
	result, err := rebuilder.Rebuild(ctx, "test_projection", &RebuildConfig{
		DryRun: true,
	})

	if err != nil {
		t.Fatalf("Dry run failed: %v", err)
	}

	// Events should be counted but not processed
	if result.EventsProcessed != 10 {
		t.Errorf("Expected 10 events counted, got %d", result.EventsProcessed)
	}

	// Projection should not be modified
	if projection.count != 0 {
		t.Errorf("Expected projection count 0 in dry run, got %d", projection.count)
	}
}

func TestProjectionRebuilder_FromPosition(t *testing.T) {
	store := NewMemoryStore()
	checkpointStore := NewMemoryCheckpointStore()
	ctx := context.Background()

	// Add events
	for i := 0; i < 20; i++ {
		store.Append(ctx, "stream-1", VersionAny, []EventData{{
			EventType: "test_event",
			Data:      []byte(`{}`),
		}})
	}

	rebuilder := NewProjectionRebuilder(store, checkpointStore, nil)
	projection := newTestRebuildProjection("test_projection")
	rebuilder.RegisterProjection(projection)

	// Rebuild from position 10
	result, err := rebuilder.Rebuild(ctx, "test_projection", &RebuildConfig{
		FromPosition: 10,
	})

	if err != nil {
		t.Fatalf("Rebuild failed: %v", err)
	}

	if result.StartPosition != 10 {
		t.Errorf("Expected start position 10, got %d", result.StartPosition)
	}

	// Should only process 10 events (positions 10-19)
	if result.EventsProcessed != 10 {
		t.Errorf("Expected 10 events processed, got %d", result.EventsProcessed)
	}
}

func TestProjectionRebuilder_StopPosition(t *testing.T) {
	store := NewMemoryStore()
	checkpointStore := NewMemoryCheckpointStore()
	ctx := context.Background()

	// Add events
	for i := 0; i < 20; i++ {
		store.Append(ctx, "stream-1", VersionAny, []EventData{{
			EventType: "test_event",
			Data:      []byte(`{}`),
		}})
	}

	rebuilder := NewProjectionRebuilder(store, checkpointStore, nil)
	projection := newTestRebuildProjection("test_projection")
	rebuilder.RegisterProjection(projection)

	// Rebuild until position 10
	result, err := rebuilder.Rebuild(ctx, "test_projection", &RebuildConfig{
		StopPosition: 10,
	})

	if err != nil {
		t.Fatalf("Rebuild failed: %v", err)
	}

	// Should only process 10 events (positions 0-9)
	if result.EventsProcessed != 10 {
		t.Errorf("Expected 10 events processed, got %d", result.EventsProcessed)
	}

	if result.EndPosition != 9 {
		t.Errorf("Expected end position 9, got %d", result.EndPosition)
	}
}

func TestProjectionRebuilder_RebuildAll(t *testing.T) {
	store := NewMemoryStore()
	checkpointStore := NewMemoryCheckpointStore()
	ctx := context.Background()

	// Add events
	for i := 0; i < 10; i++ {
		store.Append(ctx, "stream-1", VersionAny, []EventData{{
			EventType: "test_event",
			Data:      []byte(`{}`),
		}})
	}

	rebuilder := NewProjectionRebuilder(store, checkpointStore, nil)
	rebuilder.RegisterProjection(newTestRebuildProjection("projection1"))
	rebuilder.RegisterProjection(newTestRebuildProjection("projection2"))

	results, err := rebuilder.RebuildAll(ctx, nil)
	if err != nil {
		t.Fatalf("RebuildAll failed: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("Expected 2 results, got %d", len(results))
	}

	for _, result := range results {
		if result.EventsProcessed != 10 {
			t.Errorf("Expected 10 events for %s, got %d", result.ProjectionName, result.EventsProcessed)
		}
	}
}

func TestProjectionRebuilder_RebuildAllParallel(t *testing.T) {
	store := NewMemoryStore()
	checkpointStore := NewMemoryCheckpointStore()
	ctx := context.Background()

	// Add events
	for i := 0; i < 10; i++ {
		store.Append(ctx, "stream-1", VersionAny, []EventData{{
			EventType: "test_event",
			Data:      []byte(`{}`),
		}})
	}

	rebuilder := NewProjectionRebuilder(store, checkpointStore, nil)
	rebuilder.RegisterProjection(newTestRebuildProjection("projection1"))
	rebuilder.RegisterProjection(newTestRebuildProjection("projection2"))
	rebuilder.RegisterProjection(newTestRebuildProjection("projection3"))

	results, err := rebuilder.RebuildAllParallel(ctx, nil)
	if err != nil {
		t.Fatalf("RebuildAllParallel failed: %v", err)
	}

	if len(results) != 3 {
		t.Errorf("Expected 3 results, got %d", len(results))
	}

	for _, result := range results {
		if result.EventsProcessed != 10 {
			t.Errorf("Expected 10 events for %s, got %d", result.ProjectionName, result.EventsProcessed)
		}
	}
}

func TestProjectionRebuilder_ContextCancellation(t *testing.T) {
	store := NewMemoryStore()
	checkpointStore := NewMemoryCheckpointStore()

	// Add many events
	ctx := context.Background()
	for i := 0; i < 100; i++ {
		store.Append(ctx, "stream-1", VersionAny, []EventData{{
			EventType: "test_event",
			Data:      []byte(`{}`),
		}})
	}

	rebuilder := NewProjectionRebuilder(store, checkpointStore, nil)
	projection := newTestRebuildProjection("test_projection")
	rebuilder.RegisterProjection(projection)

	// Cancel context after a short delay
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err := rebuilder.Rebuild(ctx, "test_projection", &RebuildConfig{
		BatchSize: 1, // Process slowly
	})

	// Should either complete or be cancelled
	if err != nil && !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestProjectionRebuilder_UnknownProjection(t *testing.T) {
	store := NewMemoryStore()
	checkpointStore := NewMemoryCheckpointStore()
	ctx := context.Background()

	rebuilder := NewProjectionRebuilder(store, checkpointStore, nil)

	_, err := rebuilder.Rebuild(ctx, "unknown_projection", nil)
	if err == nil {
		t.Error("Expected error for unknown projection")
	}
}

func TestProjectionRebuilder_ListProjections(t *testing.T) {
	rebuilder := NewProjectionRebuilder(nil, nil, nil)
	rebuilder.RegisterProjection(newTestRebuildProjection("projection1"))
	rebuilder.RegisterProjection(newTestRebuildProjection("projection2"))

	names := rebuilder.ListProjections()
	if len(names) != 2 {
		t.Errorf("Expected 2 projections, got %d", len(names))
	}
}
