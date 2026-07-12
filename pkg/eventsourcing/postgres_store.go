package eventsourcing

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// PostgresStore is a PostgreSQL-backed event store.
// It provides durable, production-ready event storage with ACID guarantees.
type PostgresStore struct {
	db                 *sql.DB
	tableName          string
	physicalTableName  string
	streamIndexName    string
	typeIndexName      string
	timestampIndexName string
	pollInterval       time.Duration
}

// PostgresStoreConfig configures the PostgreSQL event store.
type PostgresStoreConfig struct {
	// TableName is one raw, unqualified events table name (default: "events").
	// The store quotes it as a PostgreSQL identifier before constructing SQL.
	TableName string
	// PollInterval for subscriptions (default: 100ms)
	PollInterval time.Duration
}

// DefaultPostgresStoreConfig returns sensible defaults.
func DefaultPostgresStoreConfig() PostgresStoreConfig {
	return PostgresStoreConfig{
		TableName:    "events",
		PollInterval: 100 * time.Millisecond,
	}
}

// NewPostgresStore creates a new PostgreSQL event store.
// The database connection should already be established.
func NewPostgresStore(db *sql.DB, config PostgresStoreConfig) *PostgresStore {
	if config.TableName == "" {
		config.TableName = "events"
	}
	if config.PollInterval <= 0 {
		config.PollInterval = 100 * time.Millisecond
	}

	return &PostgresStore{
		db:                 db,
		tableName:          quotePostgresIdentifier(config.TableName),
		physicalTableName:  normalizePostgresIdentifier(config.TableName),
		streamIndexName:    quotePostgresIndexIdentifier(config.TableName, "stream"),
		typeIndexName:      quotePostgresIndexIdentifier(config.TableName, "type"),
		timestampIndexName: quotePostgresIndexIdentifier(config.TableName, "timestamp"),
		pollInterval:       config.PollInterval,
	}
}

// PhysicalTableName returns the normalized PostgreSQL table name used by the store.
func (s *PostgresStore) PhysicalTableName() string {
	return s.physicalTableName
}

// InitSchema creates the events table and indexes.
// Safe to call multiple times (uses IF NOT EXISTS).
func (s *PostgresStore) InitSchema(ctx context.Context) error {
	schema := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			position BIGSERIAL PRIMARY KEY,
			stream_id TEXT NOT NULL,
			stream_version BIGINT NOT NULL,
			event_type TEXT NOT NULL,
			data JSONB NOT NULL,
			metadata JSONB DEFAULT '{}',
			timestamp TIMESTAMPTZ DEFAULT NOW(),
			UNIQUE (stream_id, stream_version)
		);

		CREATE INDEX IF NOT EXISTS %s ON %s (stream_id, stream_version);
		CREATE INDEX IF NOT EXISTS %s ON %s (event_type);
		CREATE INDEX IF NOT EXISTS %s ON %s (timestamp);
	`, s.tableName, s.streamIndexName, s.tableName, s.typeIndexName, s.tableName, s.timestampIndexName, s.tableName)

	_, err := s.db.ExecContext(ctx, schema)
	return err
}

// Append adds events to a stream with optimistic concurrency control.
func (s *PostgresStore) Append(ctx context.Context, streamID string, expectedVersion int64, events []EventData) (int64, error) {
	if len(events) == 0 {
		return 0, nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // Rollback is no-op after Commit

	// Lock the stream by selecting its rows (FOR UPDATE can't be used with aggregates)
	// First, try to lock any existing rows for this stream
	_, err = tx.ExecContext(ctx, fmt.Sprintf(
		"SELECT 1 FROM %s WHERE stream_id = $1 FOR UPDATE",
		s.tableName,
	), streamID)
	if err != nil {
		return 0, fmt.Errorf("failed to lock stream: %w", err)
	}

	// Now get current stream version
	var currentVersion int64 = -1
	row := tx.QueryRowContext(ctx, fmt.Sprintf(
		"SELECT COALESCE(MAX(stream_version), -1) FROM %s WHERE stream_id = $1",
		s.tableName,
	), streamID)
	if err := row.Scan(&currentVersion); err != nil {
		return 0, fmt.Errorf("failed to get stream version: %w", err)
	}

	// Check optimistic concurrency
	if expectedVersion >= 0 {
		if currentVersion != expectedVersion {
			return 0, ErrConcurrencyConflict
		}
	} else if expectedVersion == VersionNone {
		if currentVersion >= 0 {
			return 0, ErrConcurrencyConflict
		}
	}
	// VersionAny: no check needed

	// Insert events
	newVersion := currentVersion
	for _, event := range events {
		newVersion++

		metadataJSON, err := json.Marshal(event.Metadata)
		if err != nil {
			return 0, fmt.Errorf("failed to marshal metadata: %w", err)
		}

		_, err = tx.ExecContext(ctx, fmt.Sprintf(`
			INSERT INTO %s (stream_id, stream_version, event_type, data, metadata)
			VALUES ($1, $2, $3, $4, $5)
		`, s.tableName), streamID, newVersion, event.EventType, event.Data, metadataJSON)

		if err != nil {
			return 0, fmt.Errorf("failed to insert event: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return newVersion, nil
}

// ReadStream reads events from a specific stream.
func (s *PostgresStore) ReadStream(ctx context.Context, streamID string, fromVersion int64, maxCount int) ([]StoredEvent, error) {
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`
		SELECT position, stream_id, stream_version, event_type, data, metadata, timestamp
		FROM %s
		WHERE stream_id = $1 AND stream_version >= $2
		ORDER BY stream_version ASC
		LIMIT $3
	`, s.tableName), streamID, fromVersion, maxCount)
	if err != nil {
		return nil, fmt.Errorf("failed to query events: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return s.scanEvents(rows)
}

// ReadAll reads events across all streams in global position order.
func (s *PostgresStore) ReadAll(ctx context.Context, fromPosition int64, maxCount int) ([]StoredEvent, error) {
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`
		SELECT position, stream_id, stream_version, event_type, data, metadata, timestamp
		FROM %s
		WHERE position >= $1
		ORDER BY position ASC
		LIMIT $2
	`, s.tableName), fromPosition, maxCount)
	if err != nil {
		return nil, fmt.Errorf("failed to query events: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return s.scanEvents(rows)
}

// Subscribe returns a channel of new events starting from the given position.
// Uses polling since PostgreSQL LISTEN/NOTIFY has message size limits.
func (s *PostgresStore) Subscribe(ctx context.Context, fromPosition int64) (<-chan StoredEvent, error) {
	ch := make(chan StoredEvent, 100)

	go func() {
		defer close(ch)

		position := fromPosition

		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			events, err := s.ReadAll(ctx, position, 100)
			if err != nil {
				// Log error and continue
				time.Sleep(s.pollInterval)
				continue
			}

			for _, event := range events {
				select {
				case ch <- event:
					position = event.Position + 1
				case <-ctx.Done():
					return
				}
			}

			if len(events) == 0 {
				select {
				case <-ctx.Done():
					return
				case <-time.After(s.pollInterval):
				}
			}
		}
	}()

	return ch, nil
}

// GetStreamVersion returns the current version of a stream.
func (s *PostgresStore) GetStreamVersion(ctx context.Context, streamID string) (int64, error) {
	var version int64 = -1
	row := s.db.QueryRowContext(ctx, fmt.Sprintf(
		"SELECT COALESCE(MAX(stream_version), -1) FROM %s WHERE stream_id = $1",
		s.tableName,
	), streamID)

	if err := row.Scan(&version); err != nil {
		return -1, fmt.Errorf("failed to get stream version: %w", err)
	}

	return version, nil
}

// GetLastPosition returns the global position of the most recent event.
func (s *PostgresStore) GetLastPosition(ctx context.Context) (int64, error) {
	var position int64 = -1
	row := s.db.QueryRowContext(ctx, fmt.Sprintf(
		"SELECT COALESCE(MAX(position), -1) FROM %s",
		s.tableName,
	))

	if err := row.Scan(&position); err != nil {
		return -1, fmt.Errorf("failed to get last position: %w", err)
	}

	return position, nil
}

// GetStats returns statistics about the event store.
func (s *PostgresStore) GetStats(ctx context.Context) (*PostgresStoreStats, error) {
	stats := &PostgresStoreStats{}

	// Total events
	row := s.db.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s", s.tableName))
	if err := row.Scan(&stats.TotalEvents); err != nil {
		return nil, fmt.Errorf("failed to count events: %w", err)
	}

	// Stream count
	row = s.db.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(DISTINCT stream_id) FROM %s", s.tableName))
	if err := row.Scan(&stats.StreamCount); err != nil {
		return nil, fmt.Errorf("failed to count streams: %w", err)
	}

	// Event types
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`
		SELECT event_type, COUNT(*)
		FROM %s
		GROUP BY event_type
		ORDER BY COUNT(*) DESC
	`, s.tableName))
	if err != nil {
		return nil, fmt.Errorf("failed to query event types: %w", err)
	}
	defer func() { _ = rows.Close() }()

	stats.EventTypes = make(map[string]int64)
	for rows.Next() {
		var eventType string
		var count int64
		if err := rows.Scan(&eventType, &count); err != nil {
			return nil, fmt.Errorf("failed to scan event type: %w", err)
		}
		stats.EventTypes[eventType] = count
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating event types: %w", err)
	}

	return stats, nil
}

// PostgresStoreStats contains statistics about the PostgreSQL event store.
type PostgresStoreStats struct {
	TotalEvents int64
	StreamCount int64
	EventTypes  map[string]int64
}

func (s *PostgresStore) scanEvents(rows *sql.Rows) ([]StoredEvent, error) {
	var events []StoredEvent

	for rows.Next() {
		var event StoredEvent
		var metadataJSON []byte

		err := rows.Scan(
			&event.Position,
			&event.StreamID,
			&event.StreamVersion,
			&event.EventType,
			&event.Data,
			&metadataJSON,
			&event.Timestamp,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan event: %w", err)
		}

		if len(metadataJSON) > 0 {
			if err := json.Unmarshal(metadataJSON, &event.Metadata); err != nil {
				event.Metadata = make(map[string]string)
			}
		} else {
			event.Metadata = make(map[string]string)
		}

		events = append(events, event)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration error: %w", err)
	}

	return events, nil
}

// ReadAllByTimeRange reads events within a time range in global position order.
// fromTime is inclusive, toTime is exclusive.
func (s *PostgresStore) ReadAllByTimeRange(ctx context.Context, fromTime, toTime time.Time, maxCount int) ([]StoredEvent, error) {
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`
		SELECT position, stream_id, stream_version, event_type, data, metadata, timestamp
		FROM %s
		WHERE timestamp >= $1 AND timestamp < $2
		ORDER BY position ASC
		LIMIT $3
	`, s.tableName), fromTime, toTime, maxCount)
	if err != nil {
		return nil, fmt.Errorf("failed to query events by time range: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return s.scanEvents(rows)
}

// ReadAllByTimeRangeAfterPosition reads events within a time range starting after a position.
// This is useful for paginated time-range queries.
func (s *PostgresStore) ReadAllByTimeRangeAfterPosition(ctx context.Context, fromTime, toTime time.Time, afterPosition int64, maxCount int) ([]StoredEvent, error) {
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`
		SELECT position, stream_id, stream_version, event_type, data, metadata, timestamp
		FROM %s
		WHERE timestamp >= $1 AND timestamp < $2 AND position > $3
		ORDER BY position ASC
		LIMIT $4
	`, s.tableName), fromTime, toTime, afterPosition, maxCount)
	if err != nil {
		return nil, fmt.Errorf("failed to query events by time range: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return s.scanEvents(rows)
}

// GetPositionAtTime returns the position of the event at or just before the given time.
// Returns -1 if no events exist before that time.
func (s *PostgresStore) GetPositionAtTime(ctx context.Context, t time.Time) (int64, error) {
	var position int64 = -1
	row := s.db.QueryRowContext(ctx, fmt.Sprintf(`
		SELECT position FROM %s
		WHERE timestamp <= $1
		ORDER BY timestamp DESC, position DESC
		LIMIT 1
	`, s.tableName), t)

	err := row.Scan(&position)
	if err != nil && err.Error() != "sql: no rows in result set" {
		return -1, fmt.Errorf("failed to get position at time: %w", err)
	}

	return position, nil
}

// CountEventsInTimeRange returns the number of events within a time range.
// Useful for estimating rebuild duration.
func (s *PostgresStore) CountEventsInTimeRange(ctx context.Context, fromTime, toTime time.Time) (int64, error) {
	var count int64
	row := s.db.QueryRowContext(ctx, fmt.Sprintf(
		"SELECT COUNT(*) FROM %s WHERE timestamp >= $1 AND timestamp < $2",
		s.tableName,
	), fromTime, toTime)

	if err := row.Scan(&count); err != nil {
		return 0, fmt.Errorf("failed to count events in time range: %w", err)
	}

	return count, nil
}

// PostgresCheckpointStore is a PostgreSQL-backed checkpoint store.
type PostgresCheckpointStore struct {
	db                *sql.DB
	tableName         string
	physicalTableName string
}

// NewPostgresCheckpointStore creates a new PostgreSQL checkpoint store.
// tableName is treated as one raw, unqualified identifier and is safely quoted.
func NewPostgresCheckpointStore(db *sql.DB, tableName string) *PostgresCheckpointStore {
	if tableName == "" {
		tableName = "projection_checkpoints"
	}
	return &PostgresCheckpointStore{
		db:                db,
		tableName:         quotePostgresIdentifier(tableName),
		physicalTableName: normalizePostgresIdentifier(tableName),
	}
}

// PhysicalTableName returns the normalized PostgreSQL table name used by the store.
func (s *PostgresCheckpointStore) PhysicalTableName() string {
	return s.physicalTableName
}

// InitSchema creates the checkpoints table.
func (s *PostgresCheckpointStore) InitSchema(ctx context.Context) error {
	schema := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			projection_name TEXT PRIMARY KEY,
			position BIGINT NOT NULL,
			updated_at TIMESTAMPTZ DEFAULT NOW()
		)
	`, s.tableName)

	_, err := s.db.ExecContext(ctx, schema)
	return err
}

// GetCheckpoint returns the checkpoint for a projection.
func (s *PostgresCheckpointStore) GetCheckpoint(ctx context.Context, projectionName string) (int64, error) {
	var position int64 = -1
	row := s.db.QueryRowContext(ctx, fmt.Sprintf(
		"SELECT position FROM %s WHERE projection_name = $1",
		s.tableName,
	), projectionName)

	err := row.Scan(&position)
	if err == sql.ErrNoRows {
		return -1, nil
	}
	if err != nil {
		return -1, fmt.Errorf("failed to get checkpoint: %w", err)
	}

	return position, nil
}

// SetCheckpoint saves a checkpoint.
func (s *PostgresCheckpointStore) SetCheckpoint(ctx context.Context, projectionName string, position int64) error {
	_, err := s.db.ExecContext(ctx, fmt.Sprintf(`
		INSERT INTO %s (projection_name, position, updated_at)
		VALUES ($1, $2, NOW())
		ON CONFLICT (projection_name) DO UPDATE SET
			position = EXCLUDED.position,
			updated_at = NOW()
	`, s.tableName), projectionName, position)

	if err != nil {
		return fmt.Errorf("failed to set checkpoint: %w", err)
	}

	return nil
}

// =============================================================================
// PostgreSQL Snapshot Store
// =============================================================================

// PostgresSnapshotStore is a PostgreSQL-backed snapshot store for projection state.
type PostgresSnapshotStore struct {
	db                *sql.DB
	tableName         string
	physicalTableName string
	indexName         string
}

// NewPostgresSnapshotStore creates a new PostgreSQL snapshot store.
// tableName is treated as one raw, unqualified identifier and is safely quoted.
func NewPostgresSnapshotStore(db *sql.DB, tableName string) *PostgresSnapshotStore {
	if tableName == "" {
		tableName = "projection_snapshots"
	}
	return &PostgresSnapshotStore{
		db:                db,
		tableName:         quotePostgresIdentifier(tableName),
		physicalTableName: normalizePostgresIdentifier(tableName),
		indexName:         quotePostgresIndexIdentifier(tableName, "projection"),
	}
}

// PhysicalTableName returns the normalized PostgreSQL table name used by the store.
func (s *PostgresSnapshotStore) PhysicalTableName() string {
	return s.physicalTableName
}

// InitSchema creates the snapshots table.
func (s *PostgresSnapshotStore) InitSchema(ctx context.Context) error {
	schema := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id BIGSERIAL PRIMARY KEY,
			projection_name TEXT NOT NULL,
			position BIGINT NOT NULL,
			data BYTEA NOT NULL,
			created_at TIMESTAMPTZ DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS %s ON %s (projection_name, position DESC);
	`, s.tableName, s.indexName, s.tableName)

	_, err := s.db.ExecContext(ctx, schema)
	return err
}

// SaveSnapshot persists a snapshot.
func (s *PostgresSnapshotStore) SaveSnapshot(ctx context.Context, snapshot Snapshot) error {
	createdAt := snapshot.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now()
	}

	_, err := s.db.ExecContext(ctx, fmt.Sprintf(`
		INSERT INTO %s (projection_name, position, data, created_at)
		VALUES ($1, $2, $3, $4)
	`, s.tableName), snapshot.ProjectionName, snapshot.Position, snapshot.Data, createdAt)

	if err != nil {
		return fmt.Errorf("failed to save snapshot: %w", err)
	}

	return nil
}

// GetLatestSnapshot retrieves the most recent snapshot for a projection.
func (s *PostgresSnapshotStore) GetLatestSnapshot(ctx context.Context, projectionName string) (*Snapshot, error) {
	row := s.db.QueryRowContext(ctx, fmt.Sprintf(`
		SELECT projection_name, position, data, created_at
		FROM %s
		WHERE projection_name = $1
		ORDER BY position DESC
		LIMIT 1
	`, s.tableName), projectionName)

	var snapshot Snapshot
	err := row.Scan(&snapshot.ProjectionName, &snapshot.Position, &snapshot.Data, &snapshot.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil // No snapshot exists
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get latest snapshot: %w", err)
	}

	return &snapshot, nil
}

// DeleteSnapshots removes all snapshots for a projection.
func (s *PostgresSnapshotStore) DeleteSnapshots(ctx context.Context, projectionName string) error {
	_, err := s.db.ExecContext(ctx, fmt.Sprintf(
		"DELETE FROM %s WHERE projection_name = $1",
		s.tableName,
	), projectionName)

	if err != nil {
		return fmt.Errorf("failed to delete snapshots: %w", err)
	}

	return nil
}

// DeleteOldSnapshots removes snapshots older than the keep count for a projection.
// This helps manage storage by keeping only the most recent N snapshots.
func (s *PostgresSnapshotStore) DeleteOldSnapshots(ctx context.Context, projectionName string, keepCount int) error {
	if keepCount <= 0 {
		return nil
	}

	// Delete all but the most recent `keepCount` snapshots
	_, err := s.db.ExecContext(ctx, fmt.Sprintf(`
		DELETE FROM %s
		WHERE projection_name = $1
		AND id NOT IN (
			SELECT id FROM %s
			WHERE projection_name = $1
			ORDER BY position DESC
			LIMIT $2
		)
	`, s.tableName, s.tableName), projectionName, keepCount)

	if err != nil {
		return fmt.Errorf("failed to delete old snapshots: %w", err)
	}

	return nil
}

// GetSnapshotAtOrBefore retrieves the latest snapshot at or before a given position.
// Useful for temporal queries where you need to restore state at a specific point.
func (s *PostgresSnapshotStore) GetSnapshotAtOrBefore(ctx context.Context, projectionName string, position int64) (*Snapshot, error) {
	row := s.db.QueryRowContext(ctx, fmt.Sprintf(`
		SELECT projection_name, position, data, created_at
		FROM %s
		WHERE projection_name = $1 AND position <= $2
		ORDER BY position DESC
		LIMIT 1
	`, s.tableName), projectionName, position)

	var snapshot Snapshot
	err := row.Scan(&snapshot.ProjectionName, &snapshot.Position, &snapshot.Data, &snapshot.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil // No snapshot exists at or before this position
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get snapshot at position: %w", err)
	}

	return &snapshot, nil
}

// ListSnapshots returns metadata about all snapshots for a projection.
// Does not return snapshot data to avoid loading large blobs.
func (s *PostgresSnapshotStore) ListSnapshots(ctx context.Context, projectionName string) ([]SnapshotMetadata, error) {
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`
		SELECT projection_name, position, LENGTH(data) as size, created_at
		FROM %s
		WHERE projection_name = $1
		ORDER BY position DESC
	`, s.tableName), projectionName)
	if err != nil {
		return nil, fmt.Errorf("failed to list snapshots: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var snapshots []SnapshotMetadata
	for rows.Next() {
		var meta SnapshotMetadata
		if err := rows.Scan(&meta.ProjectionName, &meta.Position, &meta.SizeBytes, &meta.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan snapshot metadata: %w", err)
		}
		snapshots = append(snapshots, meta)
	}

	return snapshots, rows.Err()
}

// =============================================================================
// PostgreSQL Deletable Event Store (implements DeletableEventStore)
// =============================================================================

// DeleteEventsByPosition removes events at the specified positions.
// This is a dangerous operation - use with extreme caution.
// In production, prefer archiving events before deletion.
func (s *PostgresStore) DeleteEventsByPosition(ctx context.Context, positions []int64) (int64, error) {
	if len(positions) == 0 {
		return 0, nil
	}

	// Build placeholders for IN clause
	placeholders := make([]string, len(positions))
	args := make([]interface{}, len(positions))
	for i, pos := range positions {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = pos
	}

	query := fmt.Sprintf(
		"DELETE FROM %s WHERE position IN (%s)",
		s.tableName,
		strings.Join(placeholders, ", "),
	)

	result, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("failed to delete events by position: %w", err)
	}

	count, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected: %w", err)
	}

	return count, nil
}

// DeleteEventsBeforePosition removes all events before the given position.
// This is a dangerous operation - use with extreme caution.
func (s *PostgresStore) DeleteEventsBeforePosition(ctx context.Context, position int64) (int64, error) {
	result, err := s.db.ExecContext(ctx, fmt.Sprintf(
		"DELETE FROM %s WHERE position < $1",
		s.tableName,
	), position)
	if err != nil {
		return 0, fmt.Errorf("failed to delete events before position: %w", err)
	}

	count, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected: %w", err)
	}

	return count, nil
}

// DeleteEventsBeforeTime removes all events before the given timestamp.
// This is a dangerous operation - use with extreme caution.
func (s *PostgresStore) DeleteEventsBeforeTime(ctx context.Context, t time.Time) (int64, error) {
	result, err := s.db.ExecContext(ctx, fmt.Sprintf(
		"DELETE FROM %s WHERE timestamp < $1",
		s.tableName,
	), t)
	if err != nil {
		return 0, fmt.Errorf("failed to delete events before time: %w", err)
	}

	count, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected: %w", err)
	}

	return count, nil
}
