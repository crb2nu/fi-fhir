package eventsourcing

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// RebuildConfig configures a projection rebuild operation.
type RebuildConfig struct {
	// FromPosition starts rebuild from a specific position (0 = from beginning)
	FromPosition int64

	// FromSnapshot restores from latest snapshot before replaying events
	FromSnapshot bool

	// BatchSize for reading events (default: 100)
	BatchSize int

	// Progress callback called after each batch
	Progress func(stats *RebuildProgress)

	// DryRun only counts events without actually rebuilding
	DryRun bool

	// StopPosition stops at this position (0 = no limit)
	StopPosition int64
}

// RebuildProgress reports rebuild progress.
type RebuildProgress struct {
	// ProjectionName being rebuilt
	ProjectionName string

	// EventsProcessed total events processed so far
	EventsProcessed int64

	// CurrentPosition in the event stream
	CurrentPosition int64

	// TotalEvents estimated total (may be 0 if unknown)
	TotalEvents int64

	// Duration elapsed so far
	Duration time.Duration

	// EventsPerSecond processing rate
	EventsPerSecond float64

	// SnapshotRestored whether we restored from a snapshot
	SnapshotRestored bool

	// SnapshotPosition position of restored snapshot (0 if none)
	SnapshotPosition int64

	// Complete indicates rebuild is finished
	Complete bool

	// Error if rebuild failed
	Error error
}

// RebuildResult contains the final result of a rebuild.
type RebuildResult struct {
	ProjectionName   string
	EventsProcessed  int64
	StartPosition    int64
	EndPosition      int64
	SnapshotRestored bool
	SnapshotPosition int64
	Duration         time.Duration
	EventsPerSecond  float64
	Error            error
}

// ProjectionRebuilder handles projection rebuilding with progress reporting.
type ProjectionRebuilder struct {
	store           EventStore
	checkpointStore CheckpointStore
	snapshotStore   SnapshotStore
	projections     map[string]Projection
	batchSize       int
	mu              sync.RWMutex
}

// NewProjectionRebuilder creates a new rebuilder.
func NewProjectionRebuilder(store EventStore, checkpointStore CheckpointStore, snapshotStore SnapshotStore) *ProjectionRebuilder {
	return &ProjectionRebuilder{
		store:           store,
		checkpointStore: checkpointStore,
		snapshotStore:   snapshotStore,
		projections:     make(map[string]Projection),
		batchSize:       100,
	}
}

// RegisterProjection adds a projection to the rebuilder.
func (r *ProjectionRebuilder) RegisterProjection(projection Projection) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.projections[projection.Name()] = projection
}

// GetProjection returns a registered projection by name.
func (r *ProjectionRebuilder) GetProjection(name string) (Projection, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.projections[name]
	return p, ok
}

// ListProjections returns all registered projection names.
func (r *ProjectionRebuilder) ListProjections() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.projections))
	for name := range r.projections {
		names = append(names, name)
	}
	return names
}

// Rebuild rebuilds a single projection with progress reporting.
func (r *ProjectionRebuilder) Rebuild(ctx context.Context, projectionName string, config *RebuildConfig) (*RebuildResult, error) {
	if config == nil {
		config = &RebuildConfig{}
	}
	if config.BatchSize <= 0 {
		config.BatchSize = r.batchSize
	}

	projection, ok := r.GetProjection(projectionName)
	if !ok {
		return nil, fmt.Errorf("projection not found: %s", projectionName)
	}

	result := &RebuildResult{
		ProjectionName: projectionName,
	}
	startTime := time.Now()

	// Clear projection state if it supports it
	if clearable, ok := projection.(interface{ Clear() }); ok {
		clearable.Clear()
	}

	// Determine start position
	startPosition := config.FromPosition

	// Try to restore from snapshot if requested
	if config.FromSnapshot && r.snapshotStore != nil {
		if snapshotable, ok := projection.(Snapshotable); ok {
			snapshot, err := r.snapshotStore.GetLatestSnapshot(ctx, projectionName)
			if err == nil && snapshot != nil {
				if err := snapshotable.Restore(snapshot.Data); err == nil {
					startPosition = snapshot.Position + 1
					result.SnapshotRestored = true
					result.SnapshotPosition = snapshot.Position
				}
			}
		}
	}

	result.StartPosition = startPosition

	// Reset checkpoint
	if err := r.checkpointStore.SetCheckpoint(ctx, projectionName, startPosition-1); err != nil {
		result.Error = fmt.Errorf("failed to reset checkpoint: %w", err)
		return result, result.Error
	}

	// Process events
	position := startPosition
	var eventsProcessed int64

	for {
		select {
		case <-ctx.Done():
			result.Error = ctx.Err()
			return result, result.Error
		default:
		}

		// Check stop position
		if config.StopPosition > 0 && position >= config.StopPosition {
			break
		}

		// Read batch
		maxCount := config.BatchSize
		if config.StopPosition > 0 && position+int64(maxCount) > config.StopPosition {
			maxCount = int(config.StopPosition - position)
		}

		events, err := r.store.ReadAll(ctx, position, maxCount)
		if err != nil {
			result.Error = fmt.Errorf("failed to read events: %w", err)
			return result, result.Error
		}

		if len(events) == 0 {
			break
		}

		// Process events (unless dry run)
		for _, event := range events {
			if !config.DryRun {
				if err := projection.Handle(ctx, event); err != nil {
					result.Error = fmt.Errorf("failed to handle event at position %d: %w", event.Position, err)
					return result, result.Error
				}
			}
			eventsProcessed++
			position = event.Position + 1
		}

		// Update checkpoint (unless dry run)
		if !config.DryRun {
			if err := r.checkpointStore.SetCheckpoint(ctx, projectionName, position-1); err != nil {
				result.Error = fmt.Errorf("failed to save checkpoint: %w", err)
				return result, result.Error
			}
		}

		// Report progress
		if config.Progress != nil {
			elapsed := time.Since(startTime)
			rate := float64(eventsProcessed) / elapsed.Seconds()
			config.Progress(&RebuildProgress{
				ProjectionName:   projectionName,
				EventsProcessed:  eventsProcessed,
				CurrentPosition:  position - 1,
				Duration:         elapsed,
				EventsPerSecond:  rate,
				SnapshotRestored: result.SnapshotRestored,
				SnapshotPosition: result.SnapshotPosition,
			})
		}
	}

	// Final result
	result.EventsProcessed = eventsProcessed
	result.EndPosition = position - 1
	result.Duration = time.Since(startTime)
	if result.Duration.Seconds() > 0 {
		result.EventsPerSecond = float64(eventsProcessed) / result.Duration.Seconds()
	}

	// Final progress callback
	if config.Progress != nil {
		config.Progress(&RebuildProgress{
			ProjectionName:   projectionName,
			EventsProcessed:  eventsProcessed,
			CurrentPosition:  position - 1,
			Duration:         result.Duration,
			EventsPerSecond:  result.EventsPerSecond,
			SnapshotRestored: result.SnapshotRestored,
			SnapshotPosition: result.SnapshotPosition,
			Complete:         true,
		})
	}

	return result, nil
}

// RebuildAll rebuilds all registered projections sequentially.
func (r *ProjectionRebuilder) RebuildAll(ctx context.Context, config *RebuildConfig) ([]*RebuildResult, error) {
	names := r.ListProjections()
	results := make([]*RebuildResult, 0, len(names))

	for _, name := range names {
		select {
		case <-ctx.Done():
			return results, ctx.Err()
		default:
		}

		result, err := r.Rebuild(ctx, name, config)
		results = append(results, result)
		if err != nil {
			return results, err
		}
	}

	return results, nil
}

// RebuildAllParallel rebuilds all projections in parallel.
func (r *ProjectionRebuilder) RebuildAllParallel(ctx context.Context, config *RebuildConfig) ([]*RebuildResult, error) {
	names := r.ListProjections()
	results := make([]*RebuildResult, len(names))
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error

	for i, name := range names {
		wg.Add(1)
		go func(idx int, projName string) {
			defer wg.Done()

			result, err := r.Rebuild(ctx, projName, config)

			mu.Lock()
			results[idx] = result
			if err != nil && firstErr == nil {
				firstErr = err
			}
			mu.Unlock()
		}(i, name)
	}

	wg.Wait()
	return results, firstErr
}

// EstimateRebuildTime estimates how long a rebuild would take based on event count.
func (r *ProjectionRebuilder) EstimateRebuildTime(ctx context.Context, projectionName string, eventsPerSecond float64) (*RebuildEstimate, error) {
	if eventsPerSecond <= 0 {
		eventsPerSecond = 1000 // Default estimate
	}

	// Get current checkpoint
	checkpoint, err := r.checkpointStore.GetCheckpoint(ctx, projectionName)
	if err != nil {
		return nil, err
	}

	// Get latest snapshot if available
	var snapshotPosition int64
	if r.snapshotStore != nil {
		snapshot, err := r.snapshotStore.GetLatestSnapshot(ctx, projectionName)
		if err == nil && snapshot != nil {
			snapshotPosition = snapshot.Position
		}
	}

	// Count total events (this could be slow for large stores)
	// For now, estimate based on checkpoint
	estimate := &RebuildEstimate{
		ProjectionName:         projectionName,
		CurrentCheckpoint:      checkpoint,
		LatestSnapshotPosition: snapshotPosition,
	}

	// Estimate events to process
	if snapshotPosition > 0 {
		estimate.EventsFromSnapshot = checkpoint - snapshotPosition
		estimate.EstimatedDurationFromSnapshot = time.Duration(float64(estimate.EventsFromSnapshot)/eventsPerSecond) * time.Second
	}

	estimate.EventsFromStart = checkpoint + 1
	estimate.EstimatedDurationFromStart = time.Duration(float64(estimate.EventsFromStart)/eventsPerSecond) * time.Second

	return estimate, nil
}

// RebuildEstimate provides time estimates for rebuilding.
type RebuildEstimate struct {
	ProjectionName               string
	CurrentCheckpoint            int64
	LatestSnapshotPosition       int64
	EventsFromStart              int64
	EventsFromSnapshot           int64
	EstimatedDurationFromStart   time.Duration
	EstimatedDurationFromSnapshot time.Duration
}
