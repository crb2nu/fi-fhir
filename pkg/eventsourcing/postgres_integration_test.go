//go:build integration

// Package eventsourcing provides integration tests for PostgreSQL stores.
// These tests use testcontainers to automatically spin up PostgreSQL.
//
// To run these tests:
//
//	go test -tags=integration ./pkg/eventsourcing/...
//
// Or to run without testcontainers (manual PostgreSQL):
//
//	POSTGRES_TEST_URL=postgres://user:pass@localhost:5432/testdb go test ./pkg/eventsourcing/...
package eventsourcing

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// TestPostgresContainer holds the test container and database connection.
type TestPostgresContainer struct {
	Container testcontainers.Container
	DB        *sql.DB
	DSN       string
}

type postgresDBAdapter struct {
	db *sql.DB
}

func (a postgresDBAdapter) ExecContext(ctx context.Context, query string, args ...interface{}) (Result, error) {
	return a.db.ExecContext(ctx, query, args...)
}

func (a postgresDBAdapter) QueryRowContext(ctx context.Context, query string, args ...interface{}) Row {
	return a.db.QueryRowContext(ctx, query, args...)
}

func (a postgresDBAdapter) QueryContext(ctx context.Context, query string, args ...interface{}) (Rows, error) {
	return a.db.QueryContext(ctx, query, args...)
}

// setupPostgresContainer creates a PostgreSQL testcontainer for integration tests.
// Returns nil if Docker is not available or if POSTGRES_TEST_URL is set.
func setupPostgresContainer(t *testing.T) *TestPostgresContainer {
	t.Helper()

	// testcontainers-go may panic when Docker is not configured (e.g. rootless Docker
	// missing on a developer machine). In that case, treat it as "Docker not
	// available" and skip the integration tests unless CI is explicitly running.
	defer func() {
		if r := recover(); r != nil {
			if os.Getenv("CI") != "" {
				t.Fatalf("Docker/testcontainers panic in CI: %v", r)
			}
			t.Skipf("Docker not available, skipping integration test: %v", r)
		}
	}()

	// Check if manual DSN is provided
	if dsn := os.Getenv("POSTGRES_TEST_URL"); dsn != "" {
		db, err := sql.Open("postgres", dsn)
		if err != nil {
			t.Fatalf("Failed to connect to manual PostgreSQL: %v", err)
		}
		if err := db.Ping(); err != nil {
			db.Close()
			t.Fatalf("Failed to ping manual PostgreSQL: %v", err)
		}
		return &TestPostgresContainer{DB: db, DSN: dsn}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Create PostgreSQL container
	container, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("testdb"),
		postgres.WithUsername("testuser"),
		postgres.WithPassword("testpass"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	if err != nil {
		if os.Getenv("CI") != "" {
			t.Fatalf("Failed to start PostgreSQL container in CI: %v", err)
		}
		t.Skipf("Failed to start PostgreSQL container (Docker not available?): %v", err)
		return nil
	}

	// Get connection string
	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		container.Terminate(ctx)
		t.Fatalf("Failed to get connection string: %v", err)
	}

	// Connect to the database
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		container.Terminate(ctx)
		t.Fatalf("Failed to open database: %v", err)
	}

	if err := db.Ping(); err != nil {
		db.Close()
		container.Terminate(ctx)
		t.Fatalf("Failed to ping database: %v", err)
	}

	t.Cleanup(func() {
		db.Close()
		container.Terminate(context.Background())
	})

	return &TestPostgresContainer{
		Container: container,
		DB:        db,
		DSN:       connStr,
	}
}

// =============================================================================
// PostgresStore Integration Tests
// =============================================================================

func TestPostgresStore_Integration_InitSchema(t *testing.T) {
	tc := setupPostgresContainer(t)
	if tc == nil {
		return
	}

	ctx := context.Background()
	store := NewPostgresStore(tc.DB, PostgresStoreConfig{
		TableName: "test_events_init",
	})

	// Clean up
	tc.DB.ExecContext(ctx, "DROP TABLE IF EXISTS test_events_init")
	defer tc.DB.ExecContext(ctx, "DROP TABLE IF EXISTS test_events_init")

	// Initialize schema
	err := store.InitSchema(ctx)
	if err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}

	// Verify table exists
	var tableName string
	err = tc.DB.QueryRowContext(ctx,
		"SELECT table_name FROM information_schema.tables WHERE table_name = 'test_events_init'",
	).Scan(&tableName)
	if err != nil {
		t.Fatalf("Table not created: %v", err)
	}

	// Should be idempotent
	err = store.InitSchema(ctx)
	if err != nil {
		t.Fatalf("Second InitSchema failed: %v", err)
	}
}

func TestPostgresStores_Integration_MaxLengthDerivedIdentifiers(t *testing.T) {
	tc := setupPostgresContainer(t)
	if tc == nil {
		return
	}

	ctx := context.Background()
	base := strings.Repeat("e", postgresIdentifierMaxBytes)
	tableNames := []string{
		base,
		base + "_checkpoints",
		base + "_snapshots",
		base + "_stream_snapshots",
	}

	eventStore := NewPostgresStore(tc.DB, PostgresStoreConfig{TableName: tableNames[0]})
	checkpointStore := NewPostgresCheckpointStore(tc.DB, tableNames[1])
	snapshotStore := NewPostgresSnapshotStore(tc.DB, tableNames[2])
	streamSnapshotStore := NewPostgresStreamSnapshotStore(postgresDBAdapter{db: tc.DB}, tableNames[3])

	quotedTables := []string{
		eventStore.tableName,
		checkpointStore.tableName,
		snapshotStore.tableName,
		streamSnapshotStore.tableName,
	}
	for _, table := range quotedTables {
		_, _ = tc.DB.ExecContext(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s CASCADE", table))
	}
	t.Cleanup(func() {
		for _, table := range quotedTables {
			_, _ = tc.DB.ExecContext(context.Background(), fmt.Sprintf("DROP TABLE IF EXISTS %s CASCADE", table))
		}
	})

	for name, initSchema := range map[string]func(context.Context) error{
		"event":           eventStore.InitSchema,
		"checkpoint":      checkpointStore.InitSchema,
		"snapshot":        snapshotStore.InitSchema,
		"stream snapshot": streamSnapshotStore.InitSchema,
	} {
		if err := initSchema(ctx); err != nil {
			t.Fatalf("initialize %s schema: %v", name, err)
		}
	}

	for _, table := range tableNames {
		var exists bool
		err := tc.DB.QueryRowContext(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM information_schema.tables
				WHERE table_schema = current_schema() AND table_name = $1
			)
		`, normalizePostgresIdentifier(table)).Scan(&exists)
		if err != nil {
			t.Fatalf("query table %q: %v", table, err)
		}
		if !exists {
			t.Fatalf("expected normalized table for %q", table)
		}
	}

	indexNames := []string{
		"idx_" + tableNames[0] + "_stream",
		"idx_" + tableNames[0] + "_type",
		"idx_" + tableNames[0] + "_timestamp",
		"idx_" + tableNames[2] + "_projection",
		"idx_" + tableNames[3] + "_type",
	}
	for _, index := range indexNames {
		var exists bool
		err := tc.DB.QueryRowContext(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM pg_indexes
				WHERE schemaname = current_schema() AND indexname = $1
			)
		`, normalizePostgresIdentifier(index)).Scan(&exists)
		if err != nil {
			t.Fatalf("query index %q: %v", index, err)
		}
		if !exists {
			t.Fatalf("expected normalized index for %q", index)
		}
	}
}

func TestPostgresStore_Integration_AppendAndRead(t *testing.T) {
	tc := setupPostgresContainer(t)
	if tc == nil {
		return
	}

	ctx := context.Background()
	store := NewPostgresStore(tc.DB, PostgresStoreConfig{
		TableName: "test_events_append",
	})

	tc.DB.ExecContext(ctx, "DROP TABLE IF EXISTS test_events_append")
	defer tc.DB.ExecContext(ctx, "DROP TABLE IF EXISTS test_events_append")

	if err := store.InitSchema(ctx); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}

	// Append events to a new stream
	streamID := "patient-123"
	events := []EventData{
		{
			EventType: "PatientAdmitted",
			Data:      json.RawMessage(`{"mrn": "123", "name": "John Doe"}`),
			Metadata:  map[string]string{"source": "test"},
		},
		{
			EventType: "VitalSignRecorded",
			Data:      json.RawMessage(`{"type": "temperature", "value": "98.6"}`),
			Metadata:  map[string]string{"source": "test"},
		},
	}

	version, err := store.Append(ctx, streamID, VersionNone, events)
	if err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	if version != 1 {
		t.Errorf("Expected version 1, got %d", version)
	}

	// Read events from stream
	readEvents, err := store.ReadStream(ctx, streamID, 0, 100)
	if err != nil {
		t.Fatalf("ReadStream failed: %v", err)
	}

	if len(readEvents) != 2 {
		t.Fatalf("Expected 2 events, got %d", len(readEvents))
	}

	if readEvents[0].EventType != "PatientAdmitted" {
		t.Errorf("Expected PatientAdmitted, got %s", readEvents[0].EventType)
	}
	if readEvents[1].EventType != "VitalSignRecorded" {
		t.Errorf("Expected VitalSignRecorded, got %s", readEvents[1].EventType)
	}

	// Verify stream versions
	if readEvents[0].StreamVersion != 0 {
		t.Errorf("Expected stream version 0, got %d", readEvents[0].StreamVersion)
	}
	if readEvents[1].StreamVersion != 1 {
		t.Errorf("Expected stream version 1, got %d", readEvents[1].StreamVersion)
	}
}

func TestPostgresStore_Integration_OptimisticConcurrency(t *testing.T) {
	tc := setupPostgresContainer(t)
	if tc == nil {
		return
	}

	ctx := context.Background()
	store := NewPostgresStore(tc.DB, PostgresStoreConfig{
		TableName: "test_events_concurrency",
	})

	tc.DB.ExecContext(ctx, "DROP TABLE IF EXISTS test_events_concurrency")
	defer tc.DB.ExecContext(ctx, "DROP TABLE IF EXISTS test_events_concurrency")

	if err := store.InitSchema(ctx); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}

	streamID := "order-456"

	// First write
	event1 := []EventData{{EventType: "OrderCreated", Data: json.RawMessage(`{}`)}}
	v1, err := store.Append(ctx, streamID, VersionNone, event1)
	if err != nil {
		t.Fatalf("First append failed: %v", err)
	}

	// Second write with correct expected version
	event2 := []EventData{{EventType: "OrderShipped", Data: json.RawMessage(`{}`)}}
	v2, err := store.Append(ctx, streamID, v1, event2)
	if err != nil {
		t.Fatalf("Second append failed: %v", err)
	}

	if v2 != 1 {
		t.Errorf("Expected version 1, got %d", v2)
	}

	// Third write with wrong expected version (conflict)
	event3 := []EventData{{EventType: "OrderDelivered", Data: json.RawMessage(`{}`)}}
	_, err = store.Append(ctx, streamID, 0, event3) // Wrong version
	if err != ErrConcurrencyConflict {
		t.Errorf("Expected ErrConcurrencyConflict, got %v", err)
	}

	// VersionAny should always succeed
	event4 := []EventData{{EventType: "OrderUpdated", Data: json.RawMessage(`{}`)}}
	v4, err := store.Append(ctx, streamID, VersionAny, event4)
	if err != nil {
		t.Fatalf("VersionAny append failed: %v", err)
	}
	if v4 != 2 {
		t.Errorf("Expected version 2, got %d", v4)
	}
}

func TestPostgresStore_Integration_ReadAll(t *testing.T) {
	tc := setupPostgresContainer(t)
	if tc == nil {
		return
	}

	ctx := context.Background()
	store := NewPostgresStore(tc.DB, PostgresStoreConfig{
		TableName: "test_events_readall",
	})

	tc.DB.ExecContext(ctx, "DROP TABLE IF EXISTS test_events_readall")
	defer tc.DB.ExecContext(ctx, "DROP TABLE IF EXISTS test_events_readall")

	if err := store.InitSchema(ctx); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}

	// Add events to multiple streams
	for i := 0; i < 3; i++ {
		streamID := fmt.Sprintf("stream-%d", i)
		events := []EventData{
			{EventType: "TestEvent", Data: json.RawMessage(fmt.Sprintf(`{"stream": %d}`, i))},
		}
		_, err := store.Append(ctx, streamID, VersionAny, events)
		if err != nil {
			t.Fatalf("Append failed: %v", err)
		}
	}

	// Read all events
	allEvents, err := store.ReadAll(ctx, 0, 100)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}

	if len(allEvents) != 3 {
		t.Errorf("Expected 3 events, got %d", len(allEvents))
	}

	// Verify global positions are sequential
	for i, e := range allEvents {
		if e.Position != int64(i+1) {
			t.Errorf("Expected position %d, got %d", i+1, e.Position)
		}
	}
}

func TestPostgresStore_Integration_Subscribe(t *testing.T) {
	tc := setupPostgresContainer(t)
	if tc == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	store := NewPostgresStore(tc.DB, PostgresStoreConfig{
		TableName:    "test_events_subscribe",
		PollInterval: 50 * time.Millisecond,
	})

	tc.DB.ExecContext(ctx, "DROP TABLE IF EXISTS test_events_subscribe")
	defer tc.DB.ExecContext(context.Background(), "DROP TABLE IF EXISTS test_events_subscribe")

	if err := store.InitSchema(ctx); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}

	// Start subscription from position 0
	eventCh, err := store.Subscribe(ctx, 0)
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}

	// Give subscription time to start
	time.Sleep(100 * time.Millisecond)

	// Append an event
	events := []EventData{{EventType: "SubscriptionTest", Data: json.RawMessage(`{"test": true}`)}}
	_, err = store.Append(ctx, "subscribe-stream", VersionAny, events)
	if err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	// Wait for event via subscription
	select {
	case event := <-eventCh:
		if event.EventType != "SubscriptionTest" {
			t.Errorf("Expected SubscriptionTest, got %s", event.EventType)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Timeout waiting for subscribed event")
	}
}

func TestPostgresStore_Integration_GetStats(t *testing.T) {
	tc := setupPostgresContainer(t)
	if tc == nil {
		return
	}

	ctx := context.Background()
	store := NewPostgresStore(tc.DB, PostgresStoreConfig{
		TableName: "test_events_stats",
	})

	tc.DB.ExecContext(ctx, "DROP TABLE IF EXISTS test_events_stats")
	defer tc.DB.ExecContext(ctx, "DROP TABLE IF EXISTS test_events_stats")

	if err := store.InitSchema(ctx); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}

	// Add events of different types
	store.Append(ctx, "stream-1", VersionAny, []EventData{
		{EventType: "TypeA", Data: json.RawMessage(`{}`)},
		{EventType: "TypeA", Data: json.RawMessage(`{}`)},
	})
	store.Append(ctx, "stream-2", VersionAny, []EventData{
		{EventType: "TypeB", Data: json.RawMessage(`{}`)},
	})

	stats, err := store.GetStats(ctx)
	if err != nil {
		t.Fatalf("GetStats failed: %v", err)
	}

	if stats.TotalEvents != 3 {
		t.Errorf("Expected 3 total events, got %d", stats.TotalEvents)
	}
	if stats.StreamCount != 2 {
		t.Errorf("Expected 2 streams, got %d", stats.StreamCount)
	}
	if stats.EventTypes["TypeA"] != 2 {
		t.Errorf("Expected 2 TypeA events, got %d", stats.EventTypes["TypeA"])
	}
	if stats.EventTypes["TypeB"] != 1 {
		t.Errorf("Expected 1 TypeB event, got %d", stats.EventTypes["TypeB"])
	}
}

// =============================================================================
// PostgresCheckpointStore Integration Tests
// =============================================================================

func TestPostgresCheckpointStore_Integration_SetAndGet(t *testing.T) {
	tc := setupPostgresContainer(t)
	if tc == nil {
		return
	}

	ctx := context.Background()
	store := NewPostgresCheckpointStore(tc.DB, "test_checkpoints")

	tc.DB.ExecContext(ctx, "DROP TABLE IF EXISTS test_checkpoints")
	defer tc.DB.ExecContext(ctx, "DROP TABLE IF EXISTS test_checkpoints")

	if err := store.InitSchema(ctx); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}

	// Get non-existent checkpoint
	pos, err := store.GetCheckpoint(ctx, "projection-1")
	if err != nil {
		t.Fatalf("GetCheckpoint failed: %v", err)
	}
	if pos != -1 {
		t.Errorf("Expected -1 for non-existent checkpoint, got %d", pos)
	}

	// Set checkpoint
	if err := store.SetCheckpoint(ctx, "projection-1", 100); err != nil {
		t.Fatalf("SetCheckpoint failed: %v", err)
	}

	// Get checkpoint
	pos, err = store.GetCheckpoint(ctx, "projection-1")
	if err != nil {
		t.Fatalf("GetCheckpoint failed: %v", err)
	}
	if pos != 100 {
		t.Errorf("Expected 100, got %d", pos)
	}

	// Update checkpoint (upsert)
	if err := store.SetCheckpoint(ctx, "projection-1", 200); err != nil {
		t.Fatalf("SetCheckpoint update failed: %v", err)
	}

	pos, err = store.GetCheckpoint(ctx, "projection-1")
	if err != nil {
		t.Fatalf("GetCheckpoint failed: %v", err)
	}
	if pos != 200 {
		t.Errorf("Expected 200, got %d", pos)
	}
}

func TestPostgresCheckpointStore_Integration_MultipleProjections(t *testing.T) {
	tc := setupPostgresContainer(t)
	if tc == nil {
		return
	}

	ctx := context.Background()
	store := NewPostgresCheckpointStore(tc.DB, "test_checkpoints_multi")

	tc.DB.ExecContext(ctx, "DROP TABLE IF EXISTS test_checkpoints_multi")
	defer tc.DB.ExecContext(ctx, "DROP TABLE IF EXISTS test_checkpoints_multi")

	if err := store.InitSchema(ctx); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}

	// Set checkpoints for multiple projections
	store.SetCheckpoint(ctx, "patient_timeline", 50)
	store.SetCheckpoint(ctx, "event_statistics", 100)
	store.SetCheckpoint(ctx, "active_encounters", 75)

	// Verify each is independent
	p1, _ := store.GetCheckpoint(ctx, "patient_timeline")
	p2, _ := store.GetCheckpoint(ctx, "event_statistics")
	p3, _ := store.GetCheckpoint(ctx, "active_encounters")

	if p1 != 50 || p2 != 100 || p3 != 75 {
		t.Errorf("Checkpoints incorrect: %d, %d, %d", p1, p2, p3)
	}
}

// =============================================================================
// PostgresSnapshotStore Integration Tests (with testcontainers)
// =============================================================================

func TestPostgresSnapshotStore_Integration_SaveAndGet(t *testing.T) {
	tc := setupPostgresContainer(t)
	if tc == nil {
		return
	}

	ctx := context.Background()
	store := NewPostgresSnapshotStore(tc.DB, "test_snapshots_tc")

	tc.DB.ExecContext(ctx, "DROP TABLE IF EXISTS test_snapshots_tc")
	defer tc.DB.ExecContext(ctx, "DROP TABLE IF EXISTS test_snapshots_tc")

	if err := store.InitSchema(ctx); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}

	// Save a snapshot
	snapshot := Snapshot{
		ProjectionName: "patient_timeline",
		Position:       100,
		Data:           []byte(`{"events": 100, "patients": 50}`),
		CreatedAt:      time.Now(),
	}

	if err := store.SaveSnapshot(ctx, snapshot); err != nil {
		t.Fatalf("SaveSnapshot failed: %v", err)
	}

	// Retrieve snapshot
	retrieved, err := store.GetLatestSnapshot(ctx, "patient_timeline")
	if err != nil {
		t.Fatalf("GetLatestSnapshot failed: %v", err)
	}
	if retrieved == nil {
		t.Fatal("Expected snapshot, got nil")
	}

	if retrieved.Position != 100 {
		t.Errorf("Expected position 100, got %d", retrieved.Position)
	}
	if string(retrieved.Data) != `{"events": 100, "patients": 50}` {
		t.Errorf("Unexpected data: %s", retrieved.Data)
	}
}

func TestPostgresSnapshotStore_Integration_GetLatestOnly(t *testing.T) {
	tc := setupPostgresContainer(t)
	if tc == nil {
		return
	}

	ctx := context.Background()
	store := NewPostgresSnapshotStore(tc.DB, "test_snapshots_latest")

	tc.DB.ExecContext(ctx, "DROP TABLE IF EXISTS test_snapshots_latest")
	defer tc.DB.ExecContext(ctx, "DROP TABLE IF EXISTS test_snapshots_latest")

	if err := store.InitSchema(ctx); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}

	// Save multiple snapshots
	for i := 1; i <= 5; i++ {
		snapshot := Snapshot{
			ProjectionName: "test_projection",
			Position:       int64(i * 100),
			Data:           []byte(fmt.Sprintf(`{"version": %d}`, i)),
		}
		store.SaveSnapshot(ctx, snapshot)
	}

	// Should get the latest
	latest, err := store.GetLatestSnapshot(ctx, "test_projection")
	if err != nil {
		t.Fatalf("GetLatestSnapshot failed: %v", err)
	}

	if latest.Position != 500 {
		t.Errorf("Expected position 500, got %d", latest.Position)
	}
}

func TestPostgresSnapshotStore_Integration_DeleteOldSnapshots(t *testing.T) {
	tc := setupPostgresContainer(t)
	if tc == nil {
		return
	}

	ctx := context.Background()
	store := NewPostgresSnapshotStore(tc.DB, "test_snapshots_delete")

	tc.DB.ExecContext(ctx, "DROP TABLE IF EXISTS test_snapshots_delete")
	defer tc.DB.ExecContext(ctx, "DROP TABLE IF EXISTS test_snapshots_delete")

	if err := store.InitSchema(ctx); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}

	// Save 5 snapshots
	for i := 1; i <= 5; i++ {
		snapshot := Snapshot{
			ProjectionName: "cleanup_test",
			Position:       int64(i * 100),
			Data:           []byte(`{}`),
		}
		store.SaveSnapshot(ctx, snapshot)
	}

	// Keep only 2
	if err := store.DeleteOldSnapshots(ctx, "cleanup_test", 2); err != nil {
		t.Fatalf("DeleteOldSnapshots failed: %v", err)
	}

	// List remaining
	metas, err := store.ListSnapshots(ctx, "cleanup_test")
	if err != nil {
		t.Fatalf("ListSnapshots failed: %v", err)
	}

	if len(metas) != 2 {
		t.Errorf("Expected 2 snapshots remaining, got %d", len(metas))
	}

	// Should keep positions 500 and 400 (most recent)
	if metas[0].Position != 500 || metas[1].Position != 400 {
		t.Errorf("Kept wrong snapshots: %d, %d", metas[0].Position, metas[1].Position)
	}
}

func TestPostgresSnapshotStore_Integration_GetSnapshotAtOrBefore(t *testing.T) {
	tc := setupPostgresContainer(t)
	if tc == nil {
		return
	}

	ctx := context.Background()
	store := NewPostgresSnapshotStore(tc.DB, "test_snapshots_temporal")

	tc.DB.ExecContext(ctx, "DROP TABLE IF EXISTS test_snapshots_temporal")
	defer tc.DB.ExecContext(ctx, "DROP TABLE IF EXISTS test_snapshots_temporal")

	if err := store.InitSchema(ctx); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}

	// Save snapshots at positions 100, 200, 300
	for _, pos := range []int64{100, 200, 300} {
		store.SaveSnapshot(ctx, Snapshot{
			ProjectionName: "temporal_test",
			Position:       pos,
			Data:           []byte(fmt.Sprintf(`{"pos": %d}`, pos)),
		})
	}

	// Query at position 250 - should return 200
	snap, err := store.GetSnapshotAtOrBefore(ctx, "temporal_test", 250)
	if err != nil {
		t.Fatalf("GetSnapshotAtOrBefore failed: %v", err)
	}
	if snap.Position != 200 {
		t.Errorf("Expected position 200, got %d", snap.Position)
	}

	// Query at position 50 - should return nil
	snap, err = store.GetSnapshotAtOrBefore(ctx, "temporal_test", 50)
	if err != nil {
		t.Fatalf("GetSnapshotAtOrBefore failed: %v", err)
	}
	if snap != nil {
		t.Errorf("Expected nil for position before all snapshots")
	}
}

// =============================================================================
// Time Range Query Integration Tests
// =============================================================================

func TestPostgresStore_Integration_ReadAllByTimeRange(t *testing.T) {
	tc := setupPostgresContainer(t)
	if tc == nil {
		return
	}

	ctx := context.Background()
	store := NewPostgresStore(tc.DB, PostgresStoreConfig{
		TableName: "test_events_timerange",
	})

	tc.DB.ExecContext(ctx, "DROP TABLE IF EXISTS test_events_timerange")
	defer tc.DB.ExecContext(ctx, "DROP TABLE IF EXISTS test_events_timerange")

	if err := store.InitSchema(ctx); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}

	// We can't control timestamps directly since PostgreSQL sets them on insert,
	// but we can test that the time range queries work
	baseTime := time.Now()

	// Add events
	for i := 0; i < 5; i++ {
		events := []EventData{{EventType: "TestEvent", Data: json.RawMessage(fmt.Sprintf(`{"seq": %d}`, i))}}
		_, err := store.Append(ctx, fmt.Sprintf("stream-%d", i), VersionAny, events)
		if err != nil {
			t.Fatalf("Append failed: %v", err)
		}
		// Small delay to ensure different timestamps
		time.Sleep(10 * time.Millisecond)
	}

	// Read all events in time range (from baseTime to now+1sec)
	events, err := store.ReadAllByTimeRange(ctx, baseTime.Add(-1*time.Second), time.Now().Add(time.Second), 100)
	if err != nil {
		t.Fatalf("ReadAllByTimeRange failed: %v", err)
	}

	if len(events) != 5 {
		t.Errorf("Expected 5 events, got %d", len(events))
	}

	// Read with limit
	events, err = store.ReadAllByTimeRange(ctx, baseTime.Add(-1*time.Second), time.Now().Add(time.Second), 2)
	if err != nil {
		t.Fatalf("ReadAllByTimeRange with limit failed: %v", err)
	}

	if len(events) != 2 {
		t.Errorf("Expected 2 events with limit, got %d", len(events))
	}
}

func TestPostgresStore_Integration_GetPositionAtTime(t *testing.T) {
	tc := setupPostgresContainer(t)
	if tc == nil {
		return
	}

	ctx := context.Background()
	store := NewPostgresStore(tc.DB, PostgresStoreConfig{
		TableName: "test_events_postime",
	})

	tc.DB.ExecContext(ctx, "DROP TABLE IF EXISTS test_events_postime")
	defer tc.DB.ExecContext(ctx, "DROP TABLE IF EXISTS test_events_postime")

	if err := store.InitSchema(ctx); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}

	// Position at time for empty store should be -1
	pos, err := store.GetPositionAtTime(ctx, time.Now())
	if err != nil {
		t.Fatalf("GetPositionAtTime failed: %v", err)
	}
	if pos != -1 {
		t.Errorf("Expected -1 for empty store, got %d", pos)
	}

	// Add events
	for i := 0; i < 3; i++ {
		store.Append(ctx, "stream-1", VersionAny, []EventData{
			{EventType: "TestEvent", Data: json.RawMessage(`{}`)},
		})
		time.Sleep(50 * time.Millisecond)
	}

	// Position at current time should be the last position
	pos, err = store.GetPositionAtTime(ctx, time.Now().Add(time.Second))
	if err != nil {
		t.Fatalf("GetPositionAtTime failed: %v", err)
	}
	if pos < 1 {
		t.Errorf("Expected position >= 1, got %d", pos)
	}
}

func TestPostgresStore_Integration_CountEventsInTimeRange(t *testing.T) {
	tc := setupPostgresContainer(t)
	if tc == nil {
		return
	}

	ctx := context.Background()
	store := NewPostgresStore(tc.DB, PostgresStoreConfig{
		TableName: "test_events_count",
	})

	tc.DB.ExecContext(ctx, "DROP TABLE IF EXISTS test_events_count")
	defer tc.DB.ExecContext(ctx, "DROP TABLE IF EXISTS test_events_count")

	if err := store.InitSchema(ctx); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}

	baseTime := time.Now()

	// Add events
	for i := 0; i < 7; i++ {
		store.Append(ctx, "stream-1", VersionAny, []EventData{
			{EventType: "TestEvent", Data: json.RawMessage(`{}`)},
		})
	}

	// Count events
	count, err := store.CountEventsInTimeRange(ctx, baseTime.Add(-1*time.Second), time.Now().Add(time.Second))
	if err != nil {
		t.Fatalf("CountEventsInTimeRange failed: %v", err)
	}

	if count != 7 {
		t.Errorf("Expected 7 events, got %d", count)
	}

	// Count with no events in range
	count, err = store.CountEventsInTimeRange(ctx, time.Now().Add(time.Hour), time.Now().Add(2*time.Hour))
	if err != nil {
		t.Fatalf("CountEventsInTimeRange failed: %v", err)
	}

	if count != 0 {
		t.Errorf("Expected 0 events in future range, got %d", count)
	}
}

// =============================================================================
// Projection Rebuild by Time Range Integration Tests
// =============================================================================

// testRebuildProjectionInt is a simple counting projection for integration tests.
type testRebuildProjectionInt struct {
	name   string
	count  int64
	events []StoredEvent
}

func newTestRebuildProjectionInt(name string) *testRebuildProjectionInt {
	return &testRebuildProjectionInt{name: name}
}

func (p *testRebuildProjectionInt) Name() string { return p.name }

func (p *testRebuildProjectionInt) Handle(ctx context.Context, event StoredEvent) error {
	p.count++
	p.events = append(p.events, event)
	return nil
}

func (p *testRebuildProjectionInt) Clear() {
	p.count = 0
	p.events = nil
}

func TestProjectionRebuilder_Integration_RebuildByTimeRange(t *testing.T) {
	tc := setupPostgresContainer(t)
	if tc == nil {
		return
	}

	ctx := context.Background()

	// Set up stores
	eventStore := NewPostgresStore(tc.DB, PostgresStoreConfig{TableName: "test_rebuild_events"})
	checkpointStore := NewPostgresCheckpointStore(tc.DB, "test_rebuild_checkpoints")

	tc.DB.ExecContext(ctx, "DROP TABLE IF EXISTS test_rebuild_events")
	tc.DB.ExecContext(ctx, "DROP TABLE IF EXISTS test_rebuild_checkpoints")
	defer func() {
		tc.DB.ExecContext(ctx, "DROP TABLE IF EXISTS test_rebuild_events")
		tc.DB.ExecContext(ctx, "DROP TABLE IF EXISTS test_rebuild_checkpoints")
	}()

	eventStore.InitSchema(ctx)
	checkpointStore.InitSchema(ctx)

	// Record start time with buffer to account for timing precision
	startTime := time.Now().Add(-100 * time.Millisecond)

	// Add events
	for i := 0; i < 10; i++ {
		eventStore.Append(ctx, fmt.Sprintf("stream-%d", i%3), VersionAny, []EventData{
			{EventType: "TestEvent", Data: json.RawMessage(fmt.Sprintf(`{"seq": %d}`, i))},
		})
		time.Sleep(10 * time.Millisecond)
	}

	// Record end time with buffer
	endTime := time.Now().Add(time.Second)

	// Create rebuilder
	rebuilder := NewProjectionRebuilder(eventStore, checkpointStore, nil)
	projection := newTestRebuildProjectionInt("time_range_projection")
	rebuilder.RegisterProjection(projection)

	// Rebuild by time range
	result, err := rebuilder.Rebuild(ctx, "time_range_projection", &RebuildConfig{
		FromTimestamp: &startTime,
		ToTimestamp:   &endTime,
		BatchSize:     3,
	})
	if err != nil {
		t.Fatalf("RebuildByTimeRange failed: %v", err)
	}

	// Verify result
	if !result.TimeRangeMode {
		t.Error("Expected TimeRangeMode to be true")
	}
	if result.EventsProcessed != 10 {
		t.Errorf("Expected 10 events processed, got %d", result.EventsProcessed)
	}
	if projection.count != 10 {
		t.Errorf("Expected projection count 10, got %d", projection.count)
	}
	if result.FromTimestamp == nil || result.ToTimestamp == nil {
		t.Error("Expected timestamps to be set in result")
	}
	if result.FirstEventTime == nil || result.LastEventTime == nil {
		t.Error("Expected first/last event times to be set")
	}
}

func TestProjectionRebuilder_Integration_RebuildByTimeRange_NoEvents(t *testing.T) {
	tc := setupPostgresContainer(t)
	if tc == nil {
		return
	}

	ctx := context.Background()

	eventStore := NewPostgresStore(tc.DB, PostgresStoreConfig{TableName: "test_rebuild_empty"})
	checkpointStore := NewPostgresCheckpointStore(tc.DB, "test_rebuild_empty_cp")

	tc.DB.ExecContext(ctx, "DROP TABLE IF EXISTS test_rebuild_empty")
	tc.DB.ExecContext(ctx, "DROP TABLE IF EXISTS test_rebuild_empty_cp")
	defer func() {
		tc.DB.ExecContext(ctx, "DROP TABLE IF EXISTS test_rebuild_empty")
		tc.DB.ExecContext(ctx, "DROP TABLE IF EXISTS test_rebuild_empty_cp")
	}()

	eventStore.InitSchema(ctx)
	checkpointStore.InitSchema(ctx)

	// Create rebuilder with no events
	rebuilder := NewProjectionRebuilder(eventStore, checkpointStore, nil)
	projection := newTestRebuildProjectionInt("empty_projection")
	rebuilder.RegisterProjection(projection)

	// Rebuild by future time range (no events)
	futureStart := time.Now().Add(time.Hour)
	futureEnd := time.Now().Add(2 * time.Hour)

	result, err := rebuilder.Rebuild(ctx, "empty_projection", &RebuildConfig{
		FromTimestamp: &futureStart,
		ToTimestamp:   &futureEnd,
	})
	if err != nil {
		t.Fatalf("RebuildByTimeRange failed: %v", err)
	}

	if result.EventsProcessed != 0 {
		t.Errorf("Expected 0 events processed, got %d", result.EventsProcessed)
	}
}

func TestProjectionRebuilder_Integration_RebuildByTimeRange_DryRun(t *testing.T) {
	tc := setupPostgresContainer(t)
	if tc == nil {
		return
	}

	ctx := context.Background()

	eventStore := NewPostgresStore(tc.DB, PostgresStoreConfig{TableName: "test_rebuild_dryrun"})
	checkpointStore := NewPostgresCheckpointStore(tc.DB, "test_rebuild_dryrun_cp")

	tc.DB.ExecContext(ctx, "DROP TABLE IF EXISTS test_rebuild_dryrun")
	tc.DB.ExecContext(ctx, "DROP TABLE IF EXISTS test_rebuild_dryrun_cp")
	defer func() {
		tc.DB.ExecContext(ctx, "DROP TABLE IF EXISTS test_rebuild_dryrun")
		tc.DB.ExecContext(ctx, "DROP TABLE IF EXISTS test_rebuild_dryrun_cp")
	}()

	eventStore.InitSchema(ctx)
	checkpointStore.InitSchema(ctx)

	// Record start time with buffer to account for timing precision
	startTime := time.Now().Add(-100 * time.Millisecond)

	// Add events
	for i := 0; i < 5; i++ {
		eventStore.Append(ctx, "stream-1", VersionAny, []EventData{
			{EventType: "TestEvent", Data: json.RawMessage(`{}`)},
		})
	}

	// Record end time with buffer
	endTime := time.Now().Add(time.Second)

	rebuilder := NewProjectionRebuilder(eventStore, checkpointStore, nil)
	projection := newTestRebuildProjectionInt("dryrun_projection")
	rebuilder.RegisterProjection(projection)

	// Dry run rebuild
	result, err := rebuilder.Rebuild(ctx, "dryrun_projection", &RebuildConfig{
		FromTimestamp: &startTime,
		ToTimestamp:   &endTime,
		DryRun:        true,
	})
	if err != nil {
		t.Fatalf("DryRun RebuildByTimeRange failed: %v", err)
	}

	// Events should be counted
	if result.EventsProcessed != 5 {
		t.Errorf("Expected 5 events counted in dry run, got %d", result.EventsProcessed)
	}

	// But projection should not be updated
	if projection.count != 0 {
		t.Errorf("Expected projection count 0 in dry run, got %d", projection.count)
	}
}

// =============================================================================
// End-to-End Integration: Event Store + Checkpoint + Snapshot
// =============================================================================

func TestPostgres_Integration_EndToEnd(t *testing.T) {
	tc := setupPostgresContainer(t)
	if tc == nil {
		return
	}

	ctx := context.Background()

	// Set up all three stores
	eventStore := NewPostgresStore(tc.DB, PostgresStoreConfig{TableName: "e2e_events"})
	checkpointStore := NewPostgresCheckpointStore(tc.DB, "e2e_checkpoints")
	snapshotStore := NewPostgresSnapshotStore(tc.DB, "e2e_snapshots")

	// Cleanup
	tc.DB.ExecContext(ctx, "DROP TABLE IF EXISTS e2e_events")
	tc.DB.ExecContext(ctx, "DROP TABLE IF EXISTS e2e_checkpoints")
	tc.DB.ExecContext(ctx, "DROP TABLE IF EXISTS e2e_snapshots")
	defer func() {
		tc.DB.ExecContext(ctx, "DROP TABLE IF EXISTS e2e_events")
		tc.DB.ExecContext(ctx, "DROP TABLE IF EXISTS e2e_checkpoints")
		tc.DB.ExecContext(ctx, "DROP TABLE IF EXISTS e2e_snapshots")
	}()

	// Initialize schemas
	if err := eventStore.InitSchema(ctx); err != nil {
		t.Fatalf("Event store InitSchema failed: %v", err)
	}
	if err := checkpointStore.InitSchema(ctx); err != nil {
		t.Fatalf("Checkpoint store InitSchema failed: %v", err)
	}
	if err := snapshotStore.InitSchema(ctx); err != nil {
		t.Fatalf("Snapshot store InitSchema failed: %v", err)
	}

	// Simulate projection processing
	projectionName := "patient_timeline"

	// 1. Check initial checkpoint
	pos, _ := checkpointStore.GetCheckpoint(ctx, projectionName)
	if pos != -1 {
		t.Errorf("Expected initial checkpoint -1, got %d", pos)
	}

	// 2. Append events
	for i := 0; i < 10; i++ {
		streamID := fmt.Sprintf("patient-%d", i%3)
		events := []EventData{
			{
				EventType: "PatientEvent",
				Data:      json.RawMessage(fmt.Sprintf(`{"seq": %d}`, i)),
			},
		}
		_, err := eventStore.Append(ctx, streamID, VersionAny, events)
		if err != nil {
			t.Fatalf("Append failed: %v", err)
		}
	}

	// 3. "Process" events and update checkpoint
	allEvents, _ := eventStore.ReadAll(ctx, 0, 100)
	for i, e := range allEvents {
		// Simulate processing
		_ = e.Data

		// Update checkpoint every 5 events
		if (i+1)%5 == 0 {
			checkpointStore.SetCheckpoint(ctx, projectionName, e.Position)
		}
	}

	// 4. Create snapshot at final position
	lastEvent := allEvents[len(allEvents)-1]
	checkpointStore.SetCheckpoint(ctx, projectionName, lastEvent.Position)
	snapshotStore.SaveSnapshot(ctx, Snapshot{
		ProjectionName: projectionName,
		Position:       lastEvent.Position,
		Data:           []byte(`{"processed": 10}`),
	})

	// 5. Verify final state
	finalPos, _ := checkpointStore.GetCheckpoint(ctx, projectionName)
	if finalPos != lastEvent.Position {
		t.Errorf("Checkpoint mismatch: expected %d, got %d", lastEvent.Position, finalPos)
	}

	snap, _ := snapshotStore.GetLatestSnapshot(ctx, projectionName)
	if snap == nil || snap.Position != lastEvent.Position {
		t.Error("Snapshot position mismatch")
	}

	// 6. Simulate recovery from snapshot
	recoverySnap, _ := snapshotStore.GetLatestSnapshot(ctx, projectionName)
	if recoverySnap == nil {
		t.Fatal("No snapshot for recovery")
	}

	// Read events after snapshot position
	newEvents, _ := eventStore.ReadAll(ctx, recoverySnap.Position+1, 100)
	if len(newEvents) != 0 {
		t.Errorf("Expected 0 events after snapshot, got %d", len(newEvents))
	}
}

// =============================================================================
// Deletion Integration Tests (DeletableEventStore)
// =============================================================================

func TestPostgresStore_Integration_DeleteEventsByPosition(t *testing.T) {
	tc := setupPostgresContainer(t)
	if tc == nil {
		return
	}

	ctx := context.Background()
	store := NewPostgresStore(tc.DB, PostgresStoreConfig{
		TableName: "test_events_delete",
	})

	tc.DB.ExecContext(ctx, "DROP TABLE IF EXISTS test_events_delete")
	defer tc.DB.ExecContext(ctx, "DROP TABLE IF EXISTS test_events_delete")

	if err := store.InitSchema(ctx); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}

	// Add events
	for i := 0; i < 10; i++ {
		store.Append(ctx, "stream-1", VersionAny, []EventData{
			{EventType: "TestEvent", Data: json.RawMessage(`{}`)},
		})
	}

	// Verify 10 events
	events, _ := store.ReadAll(ctx, 0, 100)
	if len(events) != 10 {
		t.Fatalf("Expected 10 events, got %d", len(events))
	}

	// Delete specific positions (0, 2, 4)
	positionsToDelete := []int64{
		events[0].Position,
		events[2].Position,
		events[4].Position,
	}
	deleted, err := store.DeleteEventsByPosition(ctx, positionsToDelete)
	if err != nil {
		t.Fatalf("DeleteEventsByPosition failed: %v", err)
	}
	if deleted != 3 {
		t.Errorf("Expected 3 events deleted, got %d", deleted)
	}

	// Verify 7 events remain
	remaining, _ := store.ReadAll(ctx, 0, 100)
	if len(remaining) != 7 {
		t.Errorf("Expected 7 events remaining, got %d", len(remaining))
	}
}

func TestPostgresStore_Integration_DeleteEventsBeforePosition(t *testing.T) {
	tc := setupPostgresContainer(t)
	if tc == nil {
		return
	}

	ctx := context.Background()
	store := NewPostgresStore(tc.DB, PostgresStoreConfig{
		TableName: "test_events_delete_pos",
	})

	tc.DB.ExecContext(ctx, "DROP TABLE IF EXISTS test_events_delete_pos")
	defer tc.DB.ExecContext(ctx, "DROP TABLE IF EXISTS test_events_delete_pos")

	if err := store.InitSchema(ctx); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}

	// Add events
	for i := 0; i < 10; i++ {
		store.Append(ctx, "stream-1", VersionAny, []EventData{
			{EventType: "TestEvent", Data: json.RawMessage(`{}`)},
		})
	}

	events, _ := store.ReadAll(ctx, 0, 100)
	if len(events) != 10 {
		t.Fatalf("Expected 10 events, got %d", len(events))
	}

	// Delete events before position 5 (should delete positions 0-4)
	cutoffPosition := events[5].Position
	deleted, err := store.DeleteEventsBeforePosition(ctx, cutoffPosition)
	if err != nil {
		t.Fatalf("DeleteEventsBeforePosition failed: %v", err)
	}
	if deleted != 5 {
		t.Errorf("Expected 5 events deleted, got %d", deleted)
	}

	// Verify 5 events remain
	remaining, _ := store.ReadAll(ctx, 0, 100)
	if len(remaining) != 5 {
		t.Errorf("Expected 5 events remaining, got %d", len(remaining))
	}

	// All remaining events should have position >= cutoffPosition
	for _, e := range remaining {
		if e.Position < cutoffPosition {
			t.Errorf("Event with position %d should have been deleted", e.Position)
		}
	}
}

func TestPostgresStore_Integration_DeleteEventsBeforeTime(t *testing.T) {
	tc := setupPostgresContainer(t)
	if tc == nil {
		return
	}

	ctx := context.Background()
	store := NewPostgresStore(tc.DB, PostgresStoreConfig{
		TableName: "test_events_delete_time",
	})

	tc.DB.ExecContext(ctx, "DROP TABLE IF EXISTS test_events_delete_time")
	defer tc.DB.ExecContext(ctx, "DROP TABLE IF EXISTS test_events_delete_time")

	if err := store.InitSchema(ctx); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}

	// Add first batch
	for i := 0; i < 5; i++ {
		store.Append(ctx, "stream-1", VersionAny, []EventData{
			{EventType: "TestEvent", Data: json.RawMessage(`{"batch": 1}`)},
		})
	}

	// Record cutoff time
	time.Sleep(100 * time.Millisecond)
	cutoffTime := time.Now()
	time.Sleep(100 * time.Millisecond)

	// Add second batch
	for i := 0; i < 5; i++ {
		store.Append(ctx, "stream-1", VersionAny, []EventData{
			{EventType: "TestEvent", Data: json.RawMessage(`{"batch": 2}`)},
		})
	}

	// Verify 10 events
	events, _ := store.ReadAll(ctx, 0, 100)
	if len(events) != 10 {
		t.Fatalf("Expected 10 events, got %d", len(events))
	}

	// Delete events before cutoff time
	deleted, err := store.DeleteEventsBeforeTime(ctx, cutoffTime)
	if err != nil {
		t.Fatalf("DeleteEventsBeforeTime failed: %v", err)
	}
	if deleted != 5 {
		t.Errorf("Expected 5 events deleted, got %d", deleted)
	}

	// Verify 5 events remain
	remaining, _ := store.ReadAll(ctx, 0, 100)
	if len(remaining) != 5 {
		t.Errorf("Expected 5 events remaining, got %d", len(remaining))
	}

	// All remaining events should be from batch 2
	for _, e := range remaining {
		if e.Timestamp.Before(cutoffTime) {
			t.Errorf("Event at %v should have been deleted (before cutoff %v)", e.Timestamp, cutoffTime)
		}
	}
}

// =============================================================================
// Archiver Integration Test
// =============================================================================

func TestEventArchiver_Integration_ArchiveAndDelete(t *testing.T) {
	tc := setupPostgresContainer(t)
	if tc == nil {
		return
	}

	ctx := context.Background()
	store := NewPostgresStore(tc.DB, PostgresStoreConfig{
		TableName: "test_events_archive",
	})

	tc.DB.ExecContext(ctx, "DROP TABLE IF EXISTS test_events_archive")
	defer tc.DB.ExecContext(ctx, "DROP TABLE IF EXISTS test_events_archive")

	if err := store.InitSchema(ctx); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}

	// Add events
	for i := 0; i < 10; i++ {
		store.Append(ctx, fmt.Sprintf("stream-%d", i%3), VersionAny, []EventData{
			{EventType: "TestEvent", Data: json.RawMessage(fmt.Sprintf(`{"seq": %d}`, i))},
		})
	}

	// Create archive file
	tmpDir := t.TempDir()
	archivePath := tmpDir + "/archive.jsonl"

	archiveStore, err := NewFileArchiveStore(archivePath)
	if err != nil {
		t.Fatalf("NewFileArchiveStore failed: %v", err)
	}

	// Create archiver
	archiver := NewEventArchiver(store)

	// Very short retention for testing
	policy := &RetentionPolicy{
		DefaultRetention: 1 * time.Millisecond,
		MinRetention:     1 * time.Nanosecond,
	}

	// Wait for events to become "old"
	time.Sleep(10 * time.Millisecond)

	// Archive with deletion
	result, err := archiver.Archive(ctx, &ArchiveConfig{
		Policy:             policy,
		ArchiveStore:       archiveStore,
		DeleteAfterArchive: true,
		BatchSize:          3,
	})
	if err != nil {
		t.Fatalf("Archive failed: %v", err)
	}

	if result.EventsArchived != 10 {
		t.Errorf("EventsArchived = %d, want 10", result.EventsArchived)
	}
	if result.EventsDeleted != 10 {
		t.Errorf("EventsDeleted = %d, want 10", result.EventsDeleted)
	}

	// Verify store is empty
	remaining, _ := store.ReadAll(ctx, 0, 100)
	if len(remaining) != 0 {
		t.Errorf("Expected 0 events in store, got %d", len(remaining))
	}

	// Verify archive has all events
	archived, err := ReadArchiveFile(archivePath)
	if err != nil {
		t.Fatalf("ReadArchiveFile failed: %v", err)
	}
	if len(archived) != 10 {
		t.Errorf("Archive has %d events, want 10", len(archived))
	}
}
