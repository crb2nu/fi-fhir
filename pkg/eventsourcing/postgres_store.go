package eventsourcing

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// PostgresStore is a PostgreSQL-backed event store.
// It provides durable, production-ready event storage with ACID guarantees.
type PostgresStore struct {
	db            *sql.DB
	tableName     string
	pollInterval  time.Duration
}

// PostgresStoreConfig configures the PostgreSQL event store.
type PostgresStoreConfig struct {
	// TableName is the events table name (default: "events")
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
		db:           db,
		tableName:    config.TableName,
		pollInterval: config.PollInterval,
	}
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

		CREATE INDEX IF NOT EXISTS idx_%s_stream ON %s (stream_id, stream_version);
		CREATE INDEX IF NOT EXISTS idx_%s_type ON %s (event_type);
		CREATE INDEX IF NOT EXISTS idx_%s_timestamp ON %s (timestamp);
	`, s.tableName, s.tableName, s.tableName, s.tableName, s.tableName, s.tableName, s.tableName)

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
	defer tx.Rollback()

	// Get current stream version
	var currentVersion int64 = -1
	row := tx.QueryRowContext(ctx, fmt.Sprintf(
		"SELECT COALESCE(MAX(stream_version), -1) FROM %s WHERE stream_id = $1 FOR UPDATE",
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
	defer rows.Close()

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
	defer rows.Close()

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
	defer rows.Close()

	stats.EventTypes = make(map[string]int64)
	for rows.Next() {
		var eventType string
		var count int64
		if err := rows.Scan(&eventType, &count); err != nil {
			return nil, fmt.Errorf("failed to scan event type: %w", err)
		}
		stats.EventTypes[eventType] = count
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

// PostgresCheckpointStore is a PostgreSQL-backed checkpoint store.
type PostgresCheckpointStore struct {
	db        *sql.DB
	tableName string
}

// NewPostgresCheckpointStore creates a new PostgreSQL checkpoint store.
func NewPostgresCheckpointStore(db *sql.DB, tableName string) *PostgresCheckpointStore {
	if tableName == "" {
		tableName = "projection_checkpoints"
	}
	return &PostgresCheckpointStore{
		db:        db,
		tableName: tableName,
	}
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
