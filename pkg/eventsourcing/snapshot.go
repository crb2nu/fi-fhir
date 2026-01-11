package eventsourcing

import (
	"context"
	"sync"
	"time"
)

// Snapshot represents a point-in-time capture of projection state.
type Snapshot struct {
	// ProjectionName identifies which projection this snapshot belongs to
	ProjectionName string `json:"projection_name"`

	// Position is the event position this snapshot was taken at
	Position int64 `json:"position"`

	// Data is the serialized projection state
	Data []byte `json:"data"`

	// CreatedAt is when this snapshot was taken
	CreatedAt time.Time `json:"created_at"`
}

// SnapshotMetadata provides information about a snapshot without the data payload.
// Useful for listing snapshots without loading potentially large data blobs.
type SnapshotMetadata struct {
	// ProjectionName identifies which projection this snapshot belongs to
	ProjectionName string `json:"projection_name"`

	// Position is the event position this snapshot was taken at
	Position int64 `json:"position"`

	// SizeBytes is the size of the snapshot data in bytes
	SizeBytes int64 `json:"size_bytes"`

	// CreatedAt is when this snapshot was taken
	CreatedAt time.Time `json:"created_at"`
}

// SnapshotStore provides persistence for projection snapshots.
type SnapshotStore interface {
	// SaveSnapshot persists a snapshot for a projection
	SaveSnapshot(ctx context.Context, snapshot Snapshot) error

	// GetLatestSnapshot retrieves the most recent snapshot for a projection
	GetLatestSnapshot(ctx context.Context, projectionName string) (*Snapshot, error)

	// DeleteSnapshots removes all snapshots for a projection
	DeleteSnapshots(ctx context.Context, projectionName string) error
}

// Snapshotable is implemented by projections that support snapshotting.
type Snapshotable interface {
	// Snapshot serializes the current projection state
	Snapshot() ([]byte, error)

	// Restore loads projection state from a snapshot
	Restore(data []byte) error
}

// SnapshotConfig configures automatic snapshotting behavior.
type SnapshotConfig struct {
	// Enabled controls whether snapshotting is active
	Enabled bool

	// Interval is the number of events between snapshots
	// After processing this many events, a snapshot will be taken
	Interval int64

	// MinAge prevents taking snapshots too frequently (time-based throttle)
	MinAge time.Duration
}

// DefaultSnapshotConfig returns sensible defaults for snapshotting.
func DefaultSnapshotConfig() SnapshotConfig {
	return SnapshotConfig{
		Enabled:  true,
		Interval: 1000, // Snapshot every 1000 events
		MinAge:   5 * time.Minute,
	}
}

// MemorySnapshotStore is an in-memory snapshot store for testing.
type MemorySnapshotStore struct {
	snapshots map[string][]Snapshot // projection -> snapshots (in order)
	mu        sync.RWMutex
}

// NewMemorySnapshotStore creates a new in-memory snapshot store.
func NewMemorySnapshotStore() *MemorySnapshotStore {
	return &MemorySnapshotStore{
		snapshots: make(map[string][]Snapshot),
	}
}

// SaveSnapshot stores a snapshot in memory.
func (s *MemorySnapshotStore) SaveSnapshot(ctx context.Context, snapshot Snapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Add timestamp if not set
	if snapshot.CreatedAt.IsZero() {
		snapshot.CreatedAt = time.Now()
	}

	s.snapshots[snapshot.ProjectionName] = append(
		s.snapshots[snapshot.ProjectionName],
		snapshot,
	)
	return nil
}

// GetLatestSnapshot returns the most recent snapshot for a projection.
func (s *MemorySnapshotStore) GetLatestSnapshot(ctx context.Context, projectionName string) (*Snapshot, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	snaps, ok := s.snapshots[projectionName]
	if !ok || len(snaps) == 0 {
		return nil, nil // No snapshot exists
	}

	// Return the last (most recent) snapshot
	latest := snaps[len(snaps)-1]
	return &latest, nil
}

// DeleteSnapshots removes all snapshots for a projection.
func (s *MemorySnapshotStore) DeleteSnapshots(ctx context.Context, projectionName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.snapshots, projectionName)
	return nil
}

// SnapshotManager coordinates snapshot creation and restoration.
type SnapshotManager struct {
	store      SnapshotStore
	config     SnapshotConfig
	lastSnap   map[string]time.Time // projection -> last snapshot time
	eventCount map[string]int64     // projection -> events since last snapshot
	mu         sync.Mutex
}

// NewSnapshotManager creates a new snapshot manager.
func NewSnapshotManager(store SnapshotStore, config SnapshotConfig) *SnapshotManager {
	return &SnapshotManager{
		store:      store,
		config:     config,
		lastSnap:   make(map[string]time.Time),
		eventCount: make(map[string]int64),
	}
}

// ShouldSnapshot returns true if a snapshot should be taken.
func (m *SnapshotManager) ShouldSnapshot(projectionName string) bool {
	if !m.config.Enabled {
		return false
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Check event count threshold
	if m.eventCount[projectionName] < m.config.Interval {
		return false
	}

	// Check minimum age
	lastTime := m.lastSnap[projectionName]
	return time.Since(lastTime) >= m.config.MinAge
}

// RecordEvent increments the event counter for a projection.
func (m *SnapshotManager) RecordEvent(projectionName string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.eventCount[projectionName]++
}

// TakeSnapshot captures and stores a snapshot if the projection supports it.
func (m *SnapshotManager) TakeSnapshot(ctx context.Context, projection Projection, position int64) error {
	snapshotable, ok := projection.(Snapshotable)
	if !ok {
		return nil // Projection doesn't support snapshots
	}

	data, err := snapshotable.Snapshot()
	if err != nil {
		return err
	}

	snapshot := Snapshot{
		ProjectionName: projection.Name(),
		Position:       position,
		Data:           data,
		CreatedAt:      time.Now(),
	}

	if err := m.store.SaveSnapshot(ctx, snapshot); err != nil {
		return err
	}

	// Reset counters
	m.mu.Lock()
	m.lastSnap[projection.Name()] = time.Now()
	m.eventCount[projection.Name()] = 0
	m.mu.Unlock()

	return nil
}

// RestoreSnapshot loads the latest snapshot into a projection.
// Returns the position to resume from (snapshot position + 1, or 0 if no snapshot).
func (m *SnapshotManager) RestoreSnapshot(ctx context.Context, projection Projection) (int64, error) {
	snapshotable, ok := projection.(Snapshotable)
	if !ok {
		return 0, nil // Projection doesn't support snapshots
	}

	snapshot, err := m.store.GetLatestSnapshot(ctx, projection.Name())
	if err != nil {
		return 0, err
	}

	if snapshot == nil {
		return 0, nil // No snapshot exists
	}

	if err := snapshotable.Restore(snapshot.Data); err != nil {
		return 0, err
	}

	// Resume from position after the snapshot
	return snapshot.Position + 1, nil
}

// SnapshotAwareRunner extends ProjectionRunner with snapshot support.
type SnapshotAwareRunner struct {
	store           EventStore
	checkpointStore CheckpointStore
	snapshotMgr     *SnapshotManager
	projections     map[string]Projection
	batchSize       int
}

// NewSnapshotAwareRunner creates a runner with snapshot support.
func NewSnapshotAwareRunner(
	store EventStore,
	checkpointStore CheckpointStore,
	snapshotStore SnapshotStore,
	config ProjectionRunnerConfig,
	snapshotConfig SnapshotConfig,
) *SnapshotAwareRunner {
	if config.BatchSize <= 0 {
		config.BatchSize = 100
	}
	return &SnapshotAwareRunner{
		store:           store,
		checkpointStore: checkpointStore,
		snapshotMgr:     NewSnapshotManager(snapshotStore, snapshotConfig),
		projections:     make(map[string]Projection),
		batchSize:       config.BatchSize,
	}
}

// RegisterProjection adds a projection to the runner.
func (r *SnapshotAwareRunner) RegisterProjection(projection Projection) {
	r.projections[projection.Name()] = projection
}

// RebuildWithSnapshots rebuilds a projection, optionally from a snapshot.
func (r *SnapshotAwareRunner) RebuildWithSnapshots(ctx context.Context, projectionName string) error {
	projection, ok := r.projections[projectionName]
	if !ok {
		return nil
	}

	// Clear projection state
	if clearable, ok := projection.(interface{ Clear() }); ok {
		clearable.Clear()
	}

	// Try to restore from snapshot
	startPosition, err := r.snapshotMgr.RestoreSnapshot(ctx, projection)
	if err != nil {
		// If restore fails, start from beginning
		startPosition = 0
	}

	// Process events from snapshot position
	position := startPosition
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		events, err := r.store.ReadAll(ctx, position, r.batchSize)
		if err != nil {
			return err
		}

		if len(events) == 0 {
			break
		}

		for _, event := range events {
			if err := projection.Handle(ctx, event); err != nil {
				return err
			}
			position = event.Position + 1
			r.snapshotMgr.RecordEvent(projectionName)

			// Check if we should snapshot
			if r.snapshotMgr.ShouldSnapshot(projectionName) {
				// Ignore snapshot errors - we continue processing events regardless
				_ = r.snapshotMgr.TakeSnapshot(ctx, projection, event.Position)
			}
		}
	}

	// Save final checkpoint
	return r.checkpointStore.SetCheckpoint(ctx, projectionName, position-1)
}

// RunOnceWithSnapshots runs one cycle of projection updates with snapshot support.
func (r *SnapshotAwareRunner) RunOnceWithSnapshots(ctx context.Context) error {
	for _, projection := range r.projections {
		// Get checkpoint or restore from snapshot
		checkpoint, err := r.checkpointStore.GetCheckpoint(ctx, projection.Name())
		if err != nil {
			return err
		}

		startPosition := checkpoint + 1
		if checkpoint < 0 {
			// No checkpoint - try to restore from snapshot
			snapPos, err := r.snapshotMgr.RestoreSnapshot(ctx, projection)
			if err == nil && snapPos > 0 {
				startPosition = snapPos
			}
		}

		// Read and process events
		position := startPosition
		for {
			events, err := r.store.ReadAll(ctx, position, r.batchSize)
			if err != nil {
				return err
			}

			if len(events) == 0 {
				break
			}

			for _, event := range events {
				if err := projection.Handle(ctx, event); err != nil {
					return err
				}
				position = event.Position + 1
				r.snapshotMgr.RecordEvent(projection.Name())

				// Check if we should snapshot
				if r.snapshotMgr.ShouldSnapshot(projection.Name()) {
					_ = r.snapshotMgr.TakeSnapshot(ctx, projection, event.Position)
				}
			}

			// Checkpoint after each batch
			if err := r.checkpointStore.SetCheckpoint(ctx, projection.Name(), position-1); err != nil {
				return err
			}
		}
	}

	return nil
}
