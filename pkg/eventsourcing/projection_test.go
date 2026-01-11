//nolint:gosec,errcheck // Test file - G104 errors intentionally ignored in test setup
package eventsourcing

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// CountingProjection is a test projection that counts events.
type CountingProjection struct {
	name  string
	count int64
	mu    sync.Mutex
	seen  map[int64]bool
}

func NewCountingProjection(name string) *CountingProjection {
	return &CountingProjection{
		name: name,
		seen: make(map[int64]bool),
	}
}

func (p *CountingProjection) Name() string {
	return p.name
}

func (p *CountingProjection) Handle(ctx context.Context, event StoredEvent) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Track seen events for idempotency testing
	if !p.seen[event.Position] {
		p.seen[event.Position] = true
		atomic.AddInt64(&p.count, 1)
	}

	return nil
}

func (p *CountingProjection) Count() int64 {
	return atomic.LoadInt64(&p.count)
}

// TypeCountingProjection counts events by type.
type TypeCountingProjection struct {
	name   string
	counts map[string]int64
	mu     sync.Mutex
}

func NewTypeCountingProjection(name string) *TypeCountingProjection {
	return &TypeCountingProjection{
		name:   name,
		counts: make(map[string]int64),
	}
}

func (p *TypeCountingProjection) Name() string {
	return p.name
}

func (p *TypeCountingProjection) Handle(ctx context.Context, event StoredEvent) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.counts[event.EventType]++
	return nil
}

func (p *TypeCountingProjection) GetCounts() map[string]int64 {
	p.mu.Lock()
	defer p.mu.Unlock()

	result := make(map[string]int64)
	for k, v := range p.counts {
		result[k] = v
	}
	return result
}

func TestProjectionRunner_RunOnce(t *testing.T) {
	store := NewMemoryStore()
	checkpointStore := NewMemoryCheckpointStore()
	ctx := context.Background()

	// Add events
	store.Append(ctx, "stream:1", VersionNone, []EventData{{EventType: "admit", Data: []byte(`{}`)}})
	store.Append(ctx, "stream:2", VersionNone, []EventData{{EventType: "admit", Data: []byte(`{}`)}})
	store.Append(ctx, "stream:1", 0, []EventData{{EventType: "discharge", Data: []byte(`{}`)}})

	// Create and run projection
	projection := NewCountingProjection("test_counter")
	runner := NewProjectionRunner(store, checkpointStore, DefaultProjectionRunnerConfig())
	runner.RegisterProjection(projection)

	err := runner.RunOnce(ctx)
	if err != nil {
		t.Fatalf("RunOnce failed: %v", err)
	}

	if projection.Count() != 3 {
		t.Errorf("Expected 3 events, got %d", projection.Count())
	}

	// Verify checkpoint was saved
	checkpoint, err := checkpointStore.GetCheckpoint(ctx, "test_counter")
	if err != nil {
		t.Fatalf("GetCheckpoint failed: %v", err)
	}
	if checkpoint != 2 {
		t.Errorf("Expected checkpoint at position 2, got %d", checkpoint)
	}
}

func TestProjectionRunner_Resume(t *testing.T) {
	store := NewMemoryStore()
	checkpointStore := NewMemoryCheckpointStore()
	ctx := context.Background()

	// Add initial events
	store.Append(ctx, "stream:1", VersionNone, []EventData{{EventType: "event1", Data: []byte(`{}`)}})
	store.Append(ctx, "stream:1", 0, []EventData{{EventType: "event2", Data: []byte(`{}`)}})

	// First run
	projection := NewCountingProjection("test_counter")
	runner := NewProjectionRunner(store, checkpointStore, DefaultProjectionRunnerConfig())
	runner.RegisterProjection(projection)

	err := runner.RunOnce(ctx)
	if err != nil {
		t.Fatalf("First RunOnce failed: %v", err)
	}

	if projection.Count() != 2 {
		t.Errorf("Expected 2 events after first run, got %d", projection.Count())
	}

	// Add more events
	store.Append(ctx, "stream:1", 1, []EventData{{EventType: "event3", Data: []byte(`{}`)}})

	// Second run should only process new events
	projection2 := NewCountingProjection("test_counter")
	runner2 := NewProjectionRunner(store, checkpointStore, DefaultProjectionRunnerConfig())
	runner2.RegisterProjection(projection2)

	err = runner2.RunOnce(ctx)
	if err != nil {
		t.Fatalf("Second RunOnce failed: %v", err)
	}

	// Should only process the new event
	if projection2.Count() != 1 {
		t.Errorf("Expected 1 new event after resume, got %d", projection2.Count())
	}
}

func TestProjectionRunner_Rebuild(t *testing.T) {
	store := NewMemoryStore()
	checkpointStore := NewMemoryCheckpointStore()
	ctx := context.Background()

	// Add events
	store.Append(ctx, "stream:1", VersionNone, []EventData{{EventType: "event1", Data: []byte(`{}`)}})
	store.Append(ctx, "stream:1", 0, []EventData{{EventType: "event2", Data: []byte(`{}`)}})
	store.Append(ctx, "stream:1", 1, []EventData{{EventType: "event3", Data: []byte(`{}`)}})

	// First run
	projection := NewCountingProjection("test_counter")
	runner := NewProjectionRunner(store, checkpointStore, DefaultProjectionRunnerConfig())
	runner.RegisterProjection(projection)

	err := runner.RunOnce(ctx)
	if err != nil {
		t.Fatalf("RunOnce failed: %v", err)
	}

	// Create new projection instance for rebuild
	projection2 := NewCountingProjection("test_counter")
	runner2 := NewProjectionRunner(store, checkpointStore, DefaultProjectionRunnerConfig())
	runner2.RegisterProjection(projection2)

	// Rebuild should reprocess all events
	err = runner2.Rebuild(ctx, "test_counter")
	if err != nil {
		t.Fatalf("Rebuild failed: %v", err)
	}

	if projection2.Count() != 3 {
		t.Errorf("Expected 3 events after rebuild, got %d", projection2.Count())
	}
}

func TestProjectionRunner_MultipleProjections(t *testing.T) {
	store := NewMemoryStore()
	checkpointStore := NewMemoryCheckpointStore()
	ctx := context.Background()

	// Add events
	store.Append(ctx, "stream:1", VersionNone, []EventData{{EventType: "admit", Data: []byte(`{}`)}})
	store.Append(ctx, "stream:2", VersionNone, []EventData{{EventType: "discharge", Data: []byte(`{}`)}})
	store.Append(ctx, "stream:3", VersionNone, []EventData{{EventType: "admit", Data: []byte(`{}`)}})

	// Create multiple projections
	countProjection := NewCountingProjection("counter")
	typeProjection := NewTypeCountingProjection("type_counter")

	runner := NewProjectionRunner(store, checkpointStore, DefaultProjectionRunnerConfig())
	runner.RegisterProjection(countProjection)
	runner.RegisterProjection(typeProjection)

	err := runner.RunOnce(ctx)
	if err != nil {
		t.Fatalf("RunOnce failed: %v", err)
	}

	// Both projections should have processed all events
	if countProjection.Count() != 3 {
		t.Errorf("Expected 3 events in counter, got %d", countProjection.Count())
	}

	typeCounts := typeProjection.GetCounts()
	if typeCounts["admit"] != 2 {
		t.Errorf("Expected 2 admit events, got %d", typeCounts["admit"])
	}
	if typeCounts["discharge"] != 1 {
		t.Errorf("Expected 1 discharge event, got %d", typeCounts["discharge"])
	}
}

func TestProjectionRunner_Run(t *testing.T) {
	store := NewMemoryStore()
	checkpointStore := NewMemoryCheckpointStore()
	ctx, cancel := context.WithCancel(context.Background())

	projection := NewCountingProjection("live_counter")
	config := DefaultProjectionRunnerConfig()
	config.PollInterval = 50 * time.Millisecond
	runner := NewProjectionRunner(store, checkpointStore, config)
	runner.RegisterProjection(projection)

	// Start runner in background
	runErr := make(chan error, 1)
	go func() {
		runErr <- runner.Run(ctx)
	}()

	// Add events while running
	time.Sleep(100 * time.Millisecond)
	store.Append(context.Background(), "stream:1", VersionNone, []EventData{{EventType: "event1", Data: []byte(`{}`)}})

	time.Sleep(100 * time.Millisecond)
	store.Append(context.Background(), "stream:1", 0, []EventData{{EventType: "event2", Data: []byte(`{}`)}})

	// Wait for processing
	time.Sleep(200 * time.Millisecond)

	// Cancel and check results
	cancel()

	select {
	case err := <-runErr:
		if err != nil {
			t.Fatalf("Run failed: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Runner did not stop")
	}

	if projection.Count() != 2 {
		t.Errorf("Expected 2 events, got %d", projection.Count())
	}
}

func TestMemoryCheckpointStore(t *testing.T) {
	store := NewMemoryCheckpointStore()
	ctx := context.Background()

	// Initial state
	pos, err := store.GetCheckpoint(ctx, "test")
	if err != nil {
		t.Fatalf("GetCheckpoint failed: %v", err)
	}
	if pos != -1 {
		t.Errorf("Expected initial position -1, got %d", pos)
	}

	// Set checkpoint
	err = store.SetCheckpoint(ctx, "test", 42)
	if err != nil {
		t.Fatalf("SetCheckpoint failed: %v", err)
	}

	// Get checkpoint
	pos, err = store.GetCheckpoint(ctx, "test")
	if err != nil {
		t.Fatalf("GetCheckpoint failed: %v", err)
	}
	if pos != 42 {
		t.Errorf("Expected position 42, got %d", pos)
	}
}

func TestProjectionRunner_NoProjections(t *testing.T) {
	store := NewMemoryStore()
	checkpointStore := NewMemoryCheckpointStore()
	ctx := context.Background()

	runner := NewProjectionRunner(store, checkpointStore, DefaultProjectionRunnerConfig())

	err := runner.Run(ctx)
	if err == nil {
		t.Error("Expected error for no projections")
	}
}
