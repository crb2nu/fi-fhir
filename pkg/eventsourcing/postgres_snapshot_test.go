//nolint:gosec // Test file - G104 errors intentionally ignored in test setup
package eventsourcing

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq" // PostgreSQL driver
)

// getTestDB returns a database connection for testing, or nil if unavailable.
// Set POSTGRES_TEST_URL environment variable to run these tests.
func getTestDB(t *testing.T) *sql.DB {
	dsn := os.Getenv("POSTGRES_TEST_URL")
	if dsn == "" {
		t.Skip("POSTGRES_TEST_URL not set, skipping PostgreSQL tests")
		return nil
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("Failed to connect to database: %v", err)
	}

	// Verify connection
	if err := db.Ping(); err != nil {
		db.Close()
		t.Fatalf("Failed to ping database: %v", err)
	}

	return db
}

func TestPostgresSnapshotStore_InitSchema(t *testing.T) {
	db := getTestDB(t)
	if db == nil {
		return
	}
	defer db.Close()

	ctx := context.Background()

	// Use a unique table name for this test
	store := NewPostgresSnapshotStore(db, "test_snapshots_init")

	// Clean up any existing table
	db.ExecContext(ctx, "DROP TABLE IF EXISTS test_snapshots_init")

	err := store.InitSchema(ctx)
	if err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}

	// Verify table exists
	var exists bool
	err = db.QueryRowContext(ctx,
		"SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'test_snapshots_init')").
		Scan(&exists)
	if err != nil {
		t.Fatalf("Failed to check table existence: %v", err)
	}
	if !exists {
		t.Error("Expected table to exist after InitSchema")
	}

	// Clean up
	db.ExecContext(ctx, "DROP TABLE IF EXISTS test_snapshots_init")
}

func TestPostgresSnapshotStore_SaveAndGet(t *testing.T) {
	db := getTestDB(t)
	if db == nil {
		return
	}
	defer db.Close()

	ctx := context.Background()
	store := NewPostgresSnapshotStore(db, "test_snapshots_saveget")

	// Setup
	db.ExecContext(ctx, "DROP TABLE IF EXISTS test_snapshots_saveget")
	if err := store.InitSchema(ctx); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}
	defer db.ExecContext(ctx, "DROP TABLE IF EXISTS test_snapshots_saveget")

	// Test save and get
	snapshot := Snapshot{
		ProjectionName: "test_projection",
		Position:       100,
		Data:           []byte(`{"key":"value","count":42}`),
		CreatedAt:      time.Now().Truncate(time.Microsecond), // Truncate for PostgreSQL precision
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

	if string(latest.Data) != `{"key":"value","count":42}` {
		t.Errorf("Data mismatch: %s", string(latest.Data))
	}
}

func TestPostgresSnapshotStore_LatestOnly(t *testing.T) {
	db := getTestDB(t)
	if db == nil {
		return
	}
	defer db.Close()

	ctx := context.Background()
	store := NewPostgresSnapshotStore(db, "test_snapshots_latest")

	// Setup
	db.ExecContext(ctx, "DROP TABLE IF EXISTS test_snapshots_latest")
	if err := store.InitSchema(ctx); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}
	defer db.ExecContext(ctx, "DROP TABLE IF EXISTS test_snapshots_latest")

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

func TestPostgresSnapshotStore_NoSnapshot(t *testing.T) {
	db := getTestDB(t)
	if db == nil {
		return
	}
	defer db.Close()

	ctx := context.Background()
	store := NewPostgresSnapshotStore(db, "test_snapshots_nosnapshot")

	// Setup
	db.ExecContext(ctx, "DROP TABLE IF EXISTS test_snapshots_nosnapshot")
	if err := store.InitSchema(ctx); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}
	defer db.ExecContext(ctx, "DROP TABLE IF EXISTS test_snapshots_nosnapshot")

	latest, err := store.GetLatestSnapshot(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("GetLatestSnapshot failed: %v", err)
	}

	if latest != nil {
		t.Error("Expected nil for nonexistent projection")
	}
}

func TestPostgresSnapshotStore_DeleteSnapshots(t *testing.T) {
	db := getTestDB(t)
	if db == nil {
		return
	}
	defer db.Close()

	ctx := context.Background()
	store := NewPostgresSnapshotStore(db, "test_snapshots_delete")

	// Setup
	db.ExecContext(ctx, "DROP TABLE IF EXISTS test_snapshots_delete")
	if err := store.InitSchema(ctx); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}
	defer db.ExecContext(ctx, "DROP TABLE IF EXISTS test_snapshots_delete")

	// Save a snapshot
	err := store.SaveSnapshot(ctx, Snapshot{
		ProjectionName: "test_projection",
		Position:       100,
		Data:           []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("SaveSnapshot failed: %v", err)
	}

	// Delete snapshots
	err = store.DeleteSnapshots(ctx, "test_projection")
	if err != nil {
		t.Fatalf("DeleteSnapshots failed: %v", err)
	}

	// Verify deletion
	latest, err := store.GetLatestSnapshot(ctx, "test_projection")
	if err != nil {
		t.Fatalf("GetLatestSnapshot failed: %v", err)
	}
	if latest != nil {
		t.Error("Expected nil after deletion")
	}
}

func TestPostgresSnapshotStore_DeleteOldSnapshots(t *testing.T) {
	db := getTestDB(t)
	if db == nil {
		return
	}
	defer db.Close()

	ctx := context.Background()
	store := NewPostgresSnapshotStore(db, "test_snapshots_deleteold")

	// Setup
	db.ExecContext(ctx, "DROP TABLE IF EXISTS test_snapshots_deleteold")
	if err := store.InitSchema(ctx); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}
	defer db.ExecContext(ctx, "DROP TABLE IF EXISTS test_snapshots_deleteold")

	// Save 5 snapshots
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

	// Keep only 2 most recent
	err := store.DeleteOldSnapshots(ctx, "test_projection", 2)
	if err != nil {
		t.Fatalf("DeleteOldSnapshots failed: %v", err)
	}

	// Verify count
	snapshots, err := store.ListSnapshots(ctx, "test_projection")
	if err != nil {
		t.Fatalf("ListSnapshots failed: %v", err)
	}
	if len(snapshots) != 2 {
		t.Errorf("Expected 2 snapshots after deletion, got %d", len(snapshots))
	}

	// Verify latest is preserved
	latest, err := store.GetLatestSnapshot(ctx, "test_projection")
	if err != nil {
		t.Fatalf("GetLatestSnapshot failed: %v", err)
	}
	if latest.Position != 400 {
		t.Errorf("Expected latest position 400, got %d", latest.Position)
	}
}

func TestPostgresSnapshotStore_GetSnapshotAtOrBefore(t *testing.T) {
	db := getTestDB(t)
	if db == nil {
		return
	}
	defer db.Close()

	ctx := context.Background()
	store := NewPostgresSnapshotStore(db, "test_snapshots_atorbefofe")

	// Setup
	db.ExecContext(ctx, "DROP TABLE IF EXISTS test_snapshots_atorbefofe")
	if err := store.InitSchema(ctx); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}
	defer db.ExecContext(ctx, "DROP TABLE IF EXISTS test_snapshots_atorbefofe")

	// Save snapshots at positions 100, 200, 300
	for _, pos := range []int64{100, 200, 300} {
		err := store.SaveSnapshot(ctx, Snapshot{
			ProjectionName: "test_projection",
			Position:       pos,
			Data:           []byte(`{}`),
		})
		if err != nil {
			t.Fatalf("SaveSnapshot failed: %v", err)
		}
	}

	// Test: get snapshot at or before 250 (should return 200)
	snapshot, err := store.GetSnapshotAtOrBefore(ctx, "test_projection", 250)
	if err != nil {
		t.Fatalf("GetSnapshotAtOrBefore failed: %v", err)
	}
	if snapshot == nil {
		t.Fatal("Expected snapshot, got nil")
	}
	if snapshot.Position != 200 {
		t.Errorf("Expected position 200, got %d", snapshot.Position)
	}

	// Test: get snapshot at or before 300 (should return exactly 300)
	snapshot, err = store.GetSnapshotAtOrBefore(ctx, "test_projection", 300)
	if err != nil {
		t.Fatalf("GetSnapshotAtOrBefore failed: %v", err)
	}
	if snapshot.Position != 300 {
		t.Errorf("Expected position 300, got %d", snapshot.Position)
	}

	// Test: get snapshot at or before 50 (none exists)
	snapshot, err = store.GetSnapshotAtOrBefore(ctx, "test_projection", 50)
	if err != nil {
		t.Fatalf("GetSnapshotAtOrBefore failed: %v", err)
	}
	if snapshot != nil {
		t.Error("Expected nil for position before any snapshot")
	}
}

func TestPostgresSnapshotStore_ListSnapshots(t *testing.T) {
	db := getTestDB(t)
	if db == nil {
		return
	}
	defer db.Close()

	ctx := context.Background()
	store := NewPostgresSnapshotStore(db, "test_snapshots_list")

	// Setup
	db.ExecContext(ctx, "DROP TABLE IF EXISTS test_snapshots_list")
	if err := store.InitSchema(ctx); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}
	defer db.ExecContext(ctx, "DROP TABLE IF EXISTS test_snapshots_list")

	// Save snapshots with varying data sizes
	testData := []struct {
		position int64
		data     string
	}{
		{100, `{"small":true}`},
		{200, `{"medium":"data with more content"}`},
		{300, `{"large":"` + string(make([]byte, 100)) + `"}`},
	}

	for _, td := range testData {
		err := store.SaveSnapshot(ctx, Snapshot{
			ProjectionName: "test_projection",
			Position:       td.position,
			Data:           []byte(td.data),
		})
		if err != nil {
			t.Fatalf("SaveSnapshot failed: %v", err)
		}
	}

	// List snapshots
	snapshots, err := store.ListSnapshots(ctx, "test_projection")
	if err != nil {
		t.Fatalf("ListSnapshots failed: %v", err)
	}

	if len(snapshots) != 3 {
		t.Errorf("Expected 3 snapshots, got %d", len(snapshots))
	}

	// Should be in descending order by position
	if snapshots[0].Position != 300 {
		t.Errorf("Expected first snapshot position 300, got %d", snapshots[0].Position)
	}

	// Verify size is reported
	if snapshots[0].SizeBytes <= 0 {
		t.Error("Expected positive size for snapshot")
	}
}
