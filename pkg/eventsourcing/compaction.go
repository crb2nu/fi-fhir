package eventsourcing

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// =============================================================================
// Stream Compaction Types
// =============================================================================

// StreamSnapshot represents a point-in-time snapshot of an aggregate/stream state.
// Unlike projection snapshots (which span all streams), stream snapshots capture
// the state of a single aggregate, allowing individual streams to be compacted.
type StreamSnapshot struct {
	// StreamID identifies which stream this snapshot belongs to
	StreamID string `json:"stream_id"`

	// Version is the stream version at which this snapshot was taken
	Version int64 `json:"version"`

	// Position is the global position of the last event included in this snapshot
	Position int64 `json:"position"`

	// AggregateType identifies the type of aggregate (e.g., "Patient", "Claim")
	AggregateType string `json:"aggregate_type"`

	// Data is the serialized aggregate state
	Data []byte `json:"data"`

	// EventCount is the number of events compressed into this snapshot
	EventCount int64 `json:"event_count"`

	// CreatedAt is when this snapshot was taken
	CreatedAt time.Time `json:"created_at"`
}

// StreamSnapshotStore provides persistence for stream-level snapshots.
type StreamSnapshotStore interface {
	// SaveStreamSnapshot persists a stream snapshot
	SaveStreamSnapshot(ctx context.Context, snapshot StreamSnapshot) error

	// GetStreamSnapshot retrieves the latest snapshot for a stream
	GetStreamSnapshot(ctx context.Context, streamID string) (*StreamSnapshot, error)

	// DeleteStreamSnapshot removes a stream's snapshot
	DeleteStreamSnapshot(ctx context.Context, streamID string) error

	// ListStreamSnapshots returns metadata for all stream snapshots
	ListStreamSnapshots(ctx context.Context) ([]StreamSnapshotMetadata, error)
}

// StreamSnapshotMetadata provides information about a stream snapshot without data.
type StreamSnapshotMetadata struct {
	StreamID      string    `json:"stream_id"`
	Version       int64     `json:"version"`
	Position      int64     `json:"position"`
	AggregateType string    `json:"aggregate_type"`
	EventCount    int64     `json:"event_count"`
	SizeBytes     int64     `json:"size_bytes"`
	CreatedAt     time.Time `json:"created_at"`
}

// =============================================================================
// Compactable Interface
// =============================================================================

// Compactable is implemented by aggregates that support compaction.
// The aggregate must be able to serialize its current state and restore from it.
type Compactable interface {
	// AggregateType returns the type name (e.g., "Patient", "Claim")
	AggregateType() string

	// StreamID returns the stream identifier for this aggregate
	StreamID() string

	// CompactSnapshot serializes the current aggregate state for compaction
	CompactSnapshot() ([]byte, error)

	// RestoreFromSnapshot loads aggregate state from a compaction snapshot
	RestoreFromSnapshot(data []byte) error

	// Apply applies an event to update the aggregate state
	Apply(event StoredEvent) error
}

// =============================================================================
// Compaction Configuration
// =============================================================================

// CompactionConfig configures when and how streams should be compacted.
type CompactionConfig struct {
	// MinEvents is the minimum number of events before compaction is considered
	// Default: 100
	MinEvents int64

	// MinAge is the minimum age of the oldest event before compaction
	// Default: 24 hours
	MinAge time.Duration

	// MaxEvents triggers compaction when a stream exceeds this many events
	// Default: 10000
	MaxEvents int64

	// DeleteAfterCompaction removes original events after successful compaction
	// Default: false (keep events for audit trail)
	DeleteAfterCompaction bool

	// ArchiveBeforeDelete archives events before deleting them
	// Only applies if DeleteAfterCompaction is true
	// Default: true
	ArchiveBeforeDelete bool

	// ArchiveStore for archiving events before deletion
	// Required if ArchiveBeforeDelete is true and DeleteAfterCompaction is true
	ArchiveStore ArchiveStore

	// DryRun only reports what would be compacted without doing it
	DryRun bool

	// Progress callback called during compaction
	Progress func(*CompactionProgress)
}

// DefaultCompactionConfig returns conservative defaults.
func DefaultCompactionConfig() *CompactionConfig {
	return &CompactionConfig{
		MinEvents:             100,
		MinAge:                24 * time.Hour,
		MaxEvents:             10000,
		DeleteAfterCompaction: false,
		ArchiveBeforeDelete:   true,
	}
}

// =============================================================================
// Compaction Progress and Results
// =============================================================================

// CompactionProgress reports progress during compaction.
type CompactionProgress struct {
	StreamID        string
	EventsProcessed int64
	SnapshotCreated bool
	EventsArchived  int64
	EventsDeleted   int64
	Duration        time.Duration
	Complete        bool
	Error           error
}

// CompactionResult contains the result of a compaction operation.
type CompactionResult struct {
	StreamID          string
	AggregateType     string
	EventsProcessed   int64
	SnapshotVersion   int64
	SnapshotPosition  int64
	EventsArchived    int64
	EventsDeleted     int64
	Duration          time.Duration
	DryRun            bool
	SnapshotSizeBytes int64
	Error             error
}

// BulkCompactionResult contains results from compacting multiple streams.
type BulkCompactionResult struct {
	StreamsProcessed   int64
	StreamsCompacted   int64
	StreamsSkipped     int64
	StreamsFailed      int64
	TotalEvents        int64
	TotalEventsDeleted int64
	Duration           time.Duration
	Results            []*CompactionResult
}

// =============================================================================
// Stream Compactor
// =============================================================================

// StreamCompactor manages stream-level compaction.
type StreamCompactor struct {
	eventStore    EventStore
	snapshotStore StreamSnapshotStore
}

// NewStreamCompactor creates a new stream compactor.
func NewStreamCompactor(eventStore EventStore, snapshotStore StreamSnapshotStore) *StreamCompactor {
	return &StreamCompactor{
		eventStore:    eventStore,
		snapshotStore: snapshotStore,
	}
}

// CompactStream compacts a single stream using the provided aggregate loader.
// The aggregateLoader function should return a fresh Compactable aggregate for the stream.
func (c *StreamCompactor) CompactStream(
	ctx context.Context,
	streamID string,
	aggregateLoader func(streamID string) Compactable,
	config *CompactionConfig,
) (*CompactionResult, error) {
	if config == nil {
		config = DefaultCompactionConfig()
	}

	result := &CompactionResult{
		StreamID: streamID,
		DryRun:   config.DryRun,
	}
	startTime := time.Now()

	// Load existing snapshot if any
	existingSnapshot, _ := c.snapshotStore.GetStreamSnapshot(ctx, streamID)
	startVersion := int64(0)
	if existingSnapshot != nil {
		startVersion = existingSnapshot.Version + 1
	}

	// Read all events for the stream from the snapshot point
	events, err := c.eventStore.ReadStream(ctx, streamID, startVersion, 100000) // Large limit
	if err != nil {
		result.Error = fmt.Errorf("failed to read stream events: %w", err)
		return result, result.Error
	}

	if len(events) == 0 {
		// No new events to compact
		result.Duration = time.Since(startTime)
		return result, nil
	}

	result.EventsProcessed = int64(len(events))

	// Check if compaction is warranted
	totalEvents := int64(len(events))
	if existingSnapshot != nil {
		totalEvents += existingSnapshot.EventCount
	}

	if totalEvents < config.MinEvents {
		// Not enough events to compact
		result.Duration = time.Since(startTime)
		return result, nil
	}

	// Check age of oldest event
	oldestEvent := events[0]
	if time.Since(oldestEvent.Timestamp) < config.MinAge {
		// Events too recent
		result.Duration = time.Since(startTime)
		return result, nil
	}

	// Create aggregate and replay events
	aggregate := aggregateLoader(streamID)
	if aggregate == nil {
		result.Error = fmt.Errorf("aggregate loader returned nil for stream %s", streamID)
		return result, result.Error
	}

	result.AggregateType = aggregate.AggregateType()

	// Restore from existing snapshot if present
	if existingSnapshot != nil {
		if err := aggregate.RestoreFromSnapshot(existingSnapshot.Data); err != nil {
			result.Error = fmt.Errorf("failed to restore from existing snapshot: %w", err)
			return result, result.Error
		}
	}

	// Apply all events
	var lastEvent StoredEvent
	for _, event := range events {
		if err := aggregate.Apply(event); err != nil {
			result.Error = fmt.Errorf("failed to apply event at position %d: %w", event.Position, err)
			return result, result.Error
		}
		lastEvent = event

		// Report progress
		if config.Progress != nil {
			config.Progress(&CompactionProgress{
				StreamID:        streamID,
				EventsProcessed: result.EventsProcessed,
			})
		}
	}

	// Create snapshot
	snapshotData, err := aggregate.CompactSnapshot()
	if err != nil {
		result.Error = fmt.Errorf("failed to create snapshot: %w", err)
		return result, result.Error
	}

	result.SnapshotVersion = lastEvent.StreamVersion
	result.SnapshotPosition = lastEvent.Position
	result.SnapshotSizeBytes = int64(len(snapshotData))

	if !config.DryRun {
		snapshot := StreamSnapshot{
			StreamID:      streamID,
			Version:       lastEvent.StreamVersion,
			Position:      lastEvent.Position,
			AggregateType: aggregate.AggregateType(),
			Data:          snapshotData,
			EventCount:    totalEvents,
			CreatedAt:     time.Now(),
		}

		if err := c.snapshotStore.SaveStreamSnapshot(ctx, snapshot); err != nil {
			result.Error = fmt.Errorf("failed to save snapshot: %w", err)
			return result, result.Error
		}
	}

	// Archive and/or delete events if configured
	if config.DeleteAfterCompaction && !config.DryRun {
		// Archive first if configured
		if config.ArchiveBeforeDelete && config.ArchiveStore != nil {
			if err := config.ArchiveStore.WriteEvents(ctx, events); err != nil {
				result.Error = fmt.Errorf("failed to archive events: %w", err)
				return result, result.Error
			}
			result.EventsArchived = int64(len(events))
		}

		// Delete events
		deletable, ok := c.eventStore.(DeletableEventStore)
		if !ok {
			result.Error = fmt.Errorf("event store does not support deletion")
			return result, result.Error
		}

		positions := make([]int64, len(events))
		for i, e := range events {
			positions[i] = e.Position
		}

		deleted, err := deletable.DeleteEventsByPosition(ctx, positions)
		if err != nil {
			result.Error = fmt.Errorf("failed to delete events: %w", err)
			return result, result.Error
		}
		result.EventsDeleted = deleted
	}

	result.Duration = time.Since(startTime)

	// Final progress
	if config.Progress != nil {
		config.Progress(&CompactionProgress{
			StreamID:        streamID,
			EventsProcessed: result.EventsProcessed,
			SnapshotCreated: true,
			EventsArchived:  result.EventsArchived,
			EventsDeleted:   result.EventsDeleted,
			Duration:        result.Duration,
			Complete:        true,
		})
	}

	return result, nil
}

// CompactStreamsByPrefix compacts all streams matching a prefix.
// For example, "patient:" would compact all patient streams.
func (c *StreamCompactor) CompactStreamsByPrefix(
	ctx context.Context,
	prefix string,
	aggregateLoader func(streamID string) Compactable,
	config *CompactionConfig,
) (*BulkCompactionResult, error) {
	if config == nil {
		config = DefaultCompactionConfig()
	}

	result := &BulkCompactionResult{}
	startTime := time.Now()

	// Find streams with this prefix
	// This requires scanning events - in production you'd want an index
	streamIDs, err := c.findStreamsByPrefix(ctx, prefix)
	if err != nil {
		return nil, fmt.Errorf("failed to find streams: %w", err)
	}

	for _, streamID := range streamIDs {
		select {
		case <-ctx.Done():
			result.Duration = time.Since(startTime)
			return result, ctx.Err()
		default:
		}

		result.StreamsProcessed++

		compactResult, err := c.CompactStream(ctx, streamID, aggregateLoader, config)
		if err != nil {
			result.StreamsFailed++
			compactResult.Error = err
		} else if compactResult.SnapshotVersion > 0 {
			result.StreamsCompacted++
		} else {
			result.StreamsSkipped++
		}

		result.Results = append(result.Results, compactResult)
		result.TotalEvents += compactResult.EventsProcessed
		result.TotalEventsDeleted += compactResult.EventsDeleted
	}

	result.Duration = time.Since(startTime)
	return result, nil
}

// findStreamsByPrefix scans events to find unique stream IDs with the given prefix.
// This is not efficient for large stores - production should use an index.
func (c *StreamCompactor) findStreamsByPrefix(ctx context.Context, prefix string) ([]string, error) {
	seen := make(map[string]bool)
	var streamIDs []string

	position := int64(0)
	batchSize := 1000

	for {
		events, err := c.eventStore.ReadAll(ctx, position, batchSize)
		if err != nil {
			return nil, err
		}

		if len(events) == 0 {
			break
		}

		for _, event := range events {
			if len(event.StreamID) >= len(prefix) && event.StreamID[:len(prefix)] == prefix {
				if !seen[event.StreamID] {
					seen[event.StreamID] = true
					streamIDs = append(streamIDs, event.StreamID)
				}
			}
			position = event.Position + 1
		}
	}

	return streamIDs, nil
}

// GetStreamWithSnapshot loads a stream's events, starting from its snapshot if available.
// This is how you'd typically load an aggregate for modification.
func (c *StreamCompactor) GetStreamWithSnapshot(
	ctx context.Context,
	streamID string,
	aggregate Compactable,
) (int64, error) {
	// Try to load snapshot
	snapshot, err := c.snapshotStore.GetStreamSnapshot(ctx, streamID)
	if err != nil {
		return 0, fmt.Errorf("failed to get snapshot: %w", err)
	}

	startVersion := int64(0)
	if snapshot != nil {
		// Restore from snapshot
		if err := aggregate.RestoreFromSnapshot(snapshot.Data); err != nil {
			return 0, fmt.Errorf("failed to restore snapshot: %w", err)
		}
		startVersion = snapshot.Version + 1
	}

	// Read events after snapshot
	events, err := c.eventStore.ReadStream(ctx, streamID, startVersion, 100000)
	if err != nil {
		return 0, fmt.Errorf("failed to read events: %w", err)
	}

	// Apply events
	var lastVersion int64 = -1
	if snapshot != nil {
		lastVersion = snapshot.Version
	}

	for _, event := range events {
		if err := aggregate.Apply(event); err != nil {
			return 0, fmt.Errorf("failed to apply event: %w", err)
		}
		lastVersion = event.StreamVersion
	}

	return lastVersion, nil
}

// =============================================================================
// In-Memory Stream Snapshot Store (for testing)
// =============================================================================

// MemoryStreamSnapshotStore is an in-memory stream snapshot store.
type MemoryStreamSnapshotStore struct {
	snapshots map[string]*StreamSnapshot
	mu        sync.RWMutex
}

// NewMemoryStreamSnapshotStore creates a new in-memory stream snapshot store.
func NewMemoryStreamSnapshotStore() *MemoryStreamSnapshotStore {
	return &MemoryStreamSnapshotStore{
		snapshots: make(map[string]*StreamSnapshot),
	}
}

// SaveStreamSnapshot stores a stream snapshot.
func (s *MemoryStreamSnapshotStore) SaveStreamSnapshot(ctx context.Context, snapshot StreamSnapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if snapshot.CreatedAt.IsZero() {
		snapshot.CreatedAt = time.Now()
	}

	s.snapshots[snapshot.StreamID] = &snapshot
	return nil
}

// GetStreamSnapshot retrieves a stream's snapshot.
func (s *MemoryStreamSnapshotStore) GetStreamSnapshot(ctx context.Context, streamID string) (*StreamSnapshot, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	snap, ok := s.snapshots[streamID]
	if !ok {
		return nil, nil
	}

	return snap, nil
}

// DeleteStreamSnapshot removes a stream's snapshot.
func (s *MemoryStreamSnapshotStore) DeleteStreamSnapshot(ctx context.Context, streamID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.snapshots, streamID)
	return nil
}

// ListStreamSnapshots returns metadata for all snapshots.
func (s *MemoryStreamSnapshotStore) ListStreamSnapshots(ctx context.Context) ([]StreamSnapshotMetadata, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []StreamSnapshotMetadata
	for _, snap := range s.snapshots {
		result = append(result, StreamSnapshotMetadata{
			StreamID:      snap.StreamID,
			Version:       snap.Version,
			Position:      snap.Position,
			AggregateType: snap.AggregateType,
			EventCount:    snap.EventCount,
			SizeBytes:     int64(len(snap.Data)),
			CreatedAt:     snap.CreatedAt,
		})
	}

	return result, nil
}

// =============================================================================
// PostgreSQL Stream Snapshot Store
// =============================================================================

// PostgresStreamSnapshotStore is a PostgreSQL-backed stream snapshot store.
type PostgresStreamSnapshotStore struct {
	db        DB
	tableName string
}

// DB interface for database operations (allows testing with mocks).
type DB interface {
	ExecContext(ctx context.Context, query string, args ...interface{}) (Result, error)
	QueryRowContext(ctx context.Context, query string, args ...interface{}) Row
	QueryContext(ctx context.Context, query string, args ...interface{}) (Rows, error)
}

// Result interface for exec results.
type Result interface {
	RowsAffected() (int64, error)
}

// Row interface for single row queries.
type Row interface {
	Scan(dest ...interface{}) error
}

// Rows interface for multi-row queries.
type Rows interface {
	Next() bool
	Scan(dest ...interface{}) error
	Close() error
	Err() error
}

// NewPostgresStreamSnapshotStore creates a new PostgreSQL stream snapshot store.
func NewPostgresStreamSnapshotStore(db DB, tableName string) *PostgresStreamSnapshotStore {
	if tableName == "" {
		tableName = "stream_snapshots"
	}
	return &PostgresStreamSnapshotStore{
		db:        db,
		tableName: tableName,
	}
}

// InitSchema creates the stream snapshots table.
func (s *PostgresStreamSnapshotStore) InitSchema(ctx context.Context) error {
	schema := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			stream_id TEXT PRIMARY KEY,
			version BIGINT NOT NULL,
			position BIGINT NOT NULL,
			aggregate_type TEXT NOT NULL,
			data BYTEA NOT NULL,
			event_count BIGINT NOT NULL DEFAULT 0,
			created_at TIMESTAMPTZ DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_%s_type ON %s (aggregate_type);
	`, s.tableName, s.tableName, s.tableName)

	_, err := s.db.ExecContext(ctx, schema)
	return err
}

// SaveStreamSnapshot stores or updates a stream snapshot.
func (s *PostgresStreamSnapshotStore) SaveStreamSnapshot(ctx context.Context, snapshot StreamSnapshot) error {
	createdAt := snapshot.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now()
	}

	_, err := s.db.ExecContext(ctx, fmt.Sprintf(`
		INSERT INTO %s (stream_id, version, position, aggregate_type, data, event_count, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (stream_id) DO UPDATE SET
			version = EXCLUDED.version,
			position = EXCLUDED.position,
			data = EXCLUDED.data,
			event_count = EXCLUDED.event_count,
			created_at = EXCLUDED.created_at
	`, s.tableName), snapshot.StreamID, snapshot.Version, snapshot.Position,
		snapshot.AggregateType, snapshot.Data, snapshot.EventCount, createdAt)

	return err
}

// GetStreamSnapshot retrieves a stream's snapshot.
func (s *PostgresStreamSnapshotStore) GetStreamSnapshot(ctx context.Context, streamID string) (*StreamSnapshot, error) {
	row := s.db.QueryRowContext(ctx, fmt.Sprintf(`
		SELECT stream_id, version, position, aggregate_type, data, event_count, created_at
		FROM %s WHERE stream_id = $1
	`, s.tableName), streamID)

	var snapshot StreamSnapshot
	err := row.Scan(
		&snapshot.StreamID,
		&snapshot.Version,
		&snapshot.Position,
		&snapshot.AggregateType,
		&snapshot.Data,
		&snapshot.EventCount,
		&snapshot.CreatedAt,
	)

	if err != nil {
		// Check for "no rows" - this is not an error, just no snapshot
		if err.Error() == "sql: no rows in result set" {
			return nil, nil
		}
		return nil, err
	}

	return &snapshot, nil
}

// DeleteStreamSnapshot removes a stream's snapshot.
func (s *PostgresStreamSnapshotStore) DeleteStreamSnapshot(ctx context.Context, streamID string) error {
	_, err := s.db.ExecContext(ctx, fmt.Sprintf(
		"DELETE FROM %s WHERE stream_id = $1",
		s.tableName,
	), streamID)
	return err
}

// ListStreamSnapshots returns metadata for all snapshots.
func (s *PostgresStreamSnapshotStore) ListStreamSnapshots(ctx context.Context) ([]StreamSnapshotMetadata, error) {
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`
		SELECT stream_id, version, position, aggregate_type, event_count, LENGTH(data), created_at
		FROM %s ORDER BY created_at DESC
	`, s.tableName))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var result []StreamSnapshotMetadata
	for rows.Next() {
		var meta StreamSnapshotMetadata
		if err := rows.Scan(
			&meta.StreamID,
			&meta.Version,
			&meta.Position,
			&meta.AggregateType,
			&meta.EventCount,
			&meta.SizeBytes,
			&meta.CreatedAt,
		); err != nil {
			return nil, err
		}
		result = append(result, meta)
	}

	return result, rows.Err()
}

// =============================================================================
// Helper: Generic Aggregate for JSON-serializable state
// =============================================================================

// JSONAggregate is a helper for aggregates that use JSON serialization.
// Embed this in your aggregate types for easy snapshot support.
type JSONAggregate struct {
	aggregateType string
	streamID      string
}

// NewJSONAggregate creates a new JSON aggregate helper.
func NewJSONAggregate(aggregateType, streamID string) JSONAggregate {
	return JSONAggregate{
		aggregateType: aggregateType,
		streamID:      streamID,
	}
}

// AggregateType returns the aggregate type.
func (a JSONAggregate) AggregateType() string {
	return a.aggregateType
}

// StreamID returns the stream ID.
func (a JSONAggregate) StreamID() string {
	return a.streamID
}

// MarshalState serializes the given state to JSON.
func (a JSONAggregate) MarshalState(state interface{}) ([]byte, error) {
	return json.Marshal(state)
}

// UnmarshalState deserializes JSON into the given state.
func (a JSONAggregate) UnmarshalState(data []byte, state interface{}) error {
	return json.Unmarshal(data, state)
}
