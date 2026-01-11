package eventsourcing

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// Projection builds a read model from events.
// Projections process events sequentially and maintain a checkpoint
// of the last processed position for recovery.
type Projection interface {
	// Name returns the projection name (used for checkpointing).
	Name() string

	// Handle processes an event and updates the read model.
	// Should be idempotent - processing the same event twice should be safe.
	Handle(ctx context.Context, event StoredEvent) error
}

// CheckpointStore manages projection checkpoint persistence.
type CheckpointStore interface {
	// GetCheckpoint returns the last processed position for a projection.
	// Returns -1 if no checkpoint exists.
	GetCheckpoint(ctx context.Context, projectionName string) (int64, error)

	// SetCheckpoint saves the last processed position.
	SetCheckpoint(ctx context.Context, projectionName string, position int64) error
}

// ProjectionRunner manages projection lifecycle and event processing.
type ProjectionRunner struct {
	store           EventStore
	checkpointStore CheckpointStore
	projections     []Projection
	batchSize       int
	pollInterval    time.Duration
	checkpointEvery int64
	logger          *log.Logger
}

// ProjectionRunnerConfig configures the projection runner.
type ProjectionRunnerConfig struct {
	// BatchSize is the number of events to read per batch (default: 100)
	BatchSize int
	// PollInterval is how often to check for new events (default: 1s)
	PollInterval time.Duration
	// CheckpointEvery is how often to save checkpoints (default: 100 events)
	CheckpointEvery int64
	// Logger for projection runner (optional)
	Logger *log.Logger
}

// DefaultProjectionRunnerConfig returns sensible defaults.
func DefaultProjectionRunnerConfig() ProjectionRunnerConfig {
	return ProjectionRunnerConfig{
		BatchSize:       100,
		PollInterval:    time.Second,
		CheckpointEvery: 100,
	}
}

// NewProjectionRunner creates a new projection runner.
func NewProjectionRunner(store EventStore, checkpointStore CheckpointStore, config ProjectionRunnerConfig) *ProjectionRunner {
	if config.BatchSize <= 0 {
		config.BatchSize = 100
	}
	if config.PollInterval <= 0 {
		config.PollInterval = time.Second
	}
	if config.CheckpointEvery <= 0 {
		config.CheckpointEvery = 100
	}

	return &ProjectionRunner{
		store:           store,
		checkpointStore: checkpointStore,
		projections:     make([]Projection, 0),
		batchSize:       config.BatchSize,
		pollInterval:    config.PollInterval,
		checkpointEvery: config.CheckpointEvery,
		logger:          config.Logger,
	}
}

// RegisterProjection adds a projection to the runner.
func (r *ProjectionRunner) RegisterProjection(projection Projection) {
	r.projections = append(r.projections, projection)
}

// Run starts the projection runner (blocking).
// It will process events for all registered projections until the context is cancelled.
func (r *ProjectionRunner) Run(ctx context.Context) error {
	if len(r.projections) == 0 {
		return fmt.Errorf("no projections registered")
	}

	// Process each projection concurrently
	var wg sync.WaitGroup
	errCh := make(chan error, len(r.projections))

	for _, proj := range r.projections {
		wg.Add(1)
		go func(p Projection) {
			defer wg.Done()
			if err := r.runProjection(ctx, p); err != nil {
				errCh <- fmt.Errorf("projection %s failed: %w", p.Name(), err)
			}
		}(proj)
	}

	// Wait for all projections or context cancellation
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// All projections finished
	case <-ctx.Done():
		// Context cancelled
	}

	// Check for errors
	close(errCh)
	var errors []error
	for err := range errCh {
		errors = append(errors, err)
	}

	if len(errors) > 0 {
		return errors[0]
	}

	return nil
}

// runProjection processes events for a single projection.
func (r *ProjectionRunner) runProjection(ctx context.Context, projection Projection) error {
	// Get starting position from checkpoint
	position, err := r.checkpointStore.GetCheckpoint(ctx, projection.Name())
	if err != nil {
		return fmt.Errorf("failed to get checkpoint: %w", err)
	}

	// Start from next position after checkpoint
	fromPosition := position + 1
	eventsProcessed := int64(0)

	r.log("Starting projection %s from position %d", projection.Name(), fromPosition)

	for {
		select {
		case <-ctx.Done():
			// Save final checkpoint
			if eventsProcessed > 0 {
				r.checkpointStore.SetCheckpoint(ctx, projection.Name(), position) //nolint:gosec // G104: best-effort checkpoint on shutdown
			}
			return nil
		default:
		}

		// Read batch of events
		events, err := r.store.ReadAll(ctx, fromPosition, r.batchSize)
		if err != nil {
			return fmt.Errorf("failed to read events: %w", err)
		}

		if len(events) == 0 {
			// No more events, wait and poll again
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(r.pollInterval):
				continue
			}
		}

		// Process events
		for _, event := range events {
			if err := projection.Handle(ctx, event); err != nil {
				return fmt.Errorf("failed to handle event %d: %w", event.Position, err)
			}

			position = event.Position
			eventsProcessed++
			fromPosition = position + 1

			// Checkpoint periodically
			if eventsProcessed%r.checkpointEvery == 0 {
				if err := r.checkpointStore.SetCheckpoint(ctx, projection.Name(), position); err != nil {
					r.log("Warning: failed to save checkpoint for %s: %v", projection.Name(), err)
				}
			}
		}
	}
}

// RunOnce processes all available events once and returns.
// Useful for testing or batch processing.
func (r *ProjectionRunner) RunOnce(ctx context.Context) error {
	for _, projection := range r.projections {
		if err := r.processProjectionOnce(ctx, projection); err != nil {
			return err
		}
	}
	return nil
}

// processProjectionOnce processes all events for a projection without polling.
func (r *ProjectionRunner) processProjectionOnce(ctx context.Context, projection Projection) error {
	position, err := r.checkpointStore.GetCheckpoint(ctx, projection.Name())
	if err != nil {
		return fmt.Errorf("failed to get checkpoint: %w", err)
	}

	fromPosition := position + 1
	eventsProcessed := int64(0)

	for {
		events, err := r.store.ReadAll(ctx, fromPosition, r.batchSize)
		if err != nil {
			return fmt.Errorf("failed to read events: %w", err)
		}

		if len(events) == 0 {
			break
		}

		for _, event := range events {
			if err := projection.Handle(ctx, event); err != nil {
				return fmt.Errorf("failed to handle event %d: %w", event.Position, err)
			}

			position = event.Position
			eventsProcessed++
			fromPosition = position + 1
		}
	}

	// Save final checkpoint
	if eventsProcessed > 0 {
		if err := r.checkpointStore.SetCheckpoint(ctx, projection.Name(), position); err != nil {
			return fmt.Errorf("failed to save checkpoint: %w", err)
		}
	}

	r.log("Projection %s processed %d events, now at position %d", projection.Name(), eventsProcessed, position)

	return nil
}

// Rebuild resets a projection and rebuilds from scratch.
func (r *ProjectionRunner) Rebuild(ctx context.Context, projectionName string) error {
	var projection Projection
	for _, p := range r.projections {
		if p.Name() == projectionName {
			projection = p
			break
		}
	}

	if projection == nil {
		return fmt.Errorf("projection not found: %s", projectionName)
	}

	// Reset checkpoint to -1 (start from beginning)
	if err := r.checkpointStore.SetCheckpoint(ctx, projectionName, -1); err != nil {
		return fmt.Errorf("failed to reset checkpoint: %w", err)
	}

	r.log("Rebuilding projection %s from scratch", projectionName)

	return r.processProjectionOnce(ctx, projection)
}

func (r *ProjectionRunner) log(format string, args ...interface{}) {
	if r.logger != nil {
		r.logger.Printf(format, args...)
	}
}

// MemoryCheckpointStore is an in-memory checkpoint store for testing.
type MemoryCheckpointStore struct {
	mu          sync.RWMutex
	checkpoints map[string]int64
}

// NewMemoryCheckpointStore creates a new in-memory checkpoint store.
func NewMemoryCheckpointStore() *MemoryCheckpointStore {
	return &MemoryCheckpointStore{
		checkpoints: make(map[string]int64),
	}
}

// GetCheckpoint returns the checkpoint for a projection.
func (s *MemoryCheckpointStore) GetCheckpoint(ctx context.Context, projectionName string) (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	pos, ok := s.checkpoints[projectionName]
	if !ok {
		return -1, nil
	}
	return pos, nil
}

// SetCheckpoint saves a checkpoint.
func (s *MemoryCheckpointStore) SetCheckpoint(ctx context.Context, projectionName string, position int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.checkpoints[projectionName] = position
	return nil
}
