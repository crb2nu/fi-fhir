//go:build integration

// Package db provides integration tests for the terminology schema migrations.
// These tests use testcontainers to automatically spin up PostgreSQL.
//
// To run these tests:
//
//	go test -tags=integration ./pkg/terminology/db/...
//
// Or to run without testcontainers (manual PostgreSQL):
//
//	POSTGRES_TEST_URL=postgres://user:pass@localhost:5432/testdb \
//	    go test -tags=integration -p 1 ./pkg/terminology/db/...
package db

import (
	"context"
	"database/sql"
	"os"
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
// Schema Initialization Tests
// =============================================================================

func TestMigrator_Integration_Initialize(t *testing.T) {
	tc := setupPostgresContainer(t)
	if tc == nil {
		return
	}

	ctx := context.Background()
	migrator := NewMigrator(tc.DB)

	// Clean up any existing schema
	tc.DB.ExecContext(ctx, "DROP SCHEMA IF EXISTS terminology CASCADE")

	// Initialize schema
	created, err := migrator.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	if !created {
		t.Error("Expected schema to be created")
	}

	// Verify schema exists
	var schemaExists bool
	err = tc.DB.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.schemata WHERE schema_name = 'terminology'
		)
	`).Scan(&schemaExists)
	if err != nil {
		t.Fatalf("Failed to check schema: %v", err)
	}
	if !schemaExists {
		t.Error("Schema 'terminology' was not created")
	}

	// Verify schema version
	version, err := migrator.CurrentVersion(ctx)
	if err != nil {
		t.Fatalf("CurrentVersion failed: %v", err)
	}
	if version != SchemaVersion {
		t.Errorf("Expected version %d, got %d", SchemaVersion, version)
	}
}

func TestMigrator_Integration_Initialize_Idempotent(t *testing.T) {
	tc := setupPostgresContainer(t)
	if tc == nil {
		return
	}

	ctx := context.Background()
	migrator := NewMigrator(tc.DB)

	// Clean up any existing schema
	tc.DB.ExecContext(ctx, "DROP SCHEMA IF EXISTS terminology CASCADE")

	// First initialization
	created1, err := migrator.Initialize(ctx)
	if err != nil {
		t.Fatalf("First Initialize failed: %v", err)
	}
	if !created1 {
		t.Error("Expected schema to be created on first call")
	}

	// Second initialization - should be idempotent
	created2, err := migrator.Initialize(ctx)
	if err != nil {
		t.Fatalf("Second Initialize failed: %v", err)
	}
	if created2 {
		t.Error("Expected no schema creation on second call (already exists)")
	}

	// Version should still be correct
	version, err := migrator.CurrentVersion(ctx)
	if err != nil {
		t.Fatalf("CurrentVersion failed: %v", err)
	}
	if version != SchemaVersion {
		t.Errorf("Expected version %d, got %d", SchemaVersion, version)
	}
}

func TestMigrator_Integration_CurrentVersion_NoSchema(t *testing.T) {
	tc := setupPostgresContainer(t)
	if tc == nil {
		return
	}

	ctx := context.Background()
	migrator := NewMigrator(tc.DB)

	// Clean up any existing schema
	tc.DB.ExecContext(ctx, "DROP SCHEMA IF EXISTS terminology CASCADE")

	// Version should be 0 when schema doesn't exist
	version, err := migrator.CurrentVersion(ctx)
	if err != nil {
		t.Fatalf("CurrentVersion failed: %v", err)
	}
	if version != 0 {
		t.Errorf("Expected version 0 for non-existent schema, got %d", version)
	}
}

func TestMigrator_Integration_Drop(t *testing.T) {
	tc := setupPostgresContainer(t)
	if tc == nil {
		return
	}

	ctx := context.Background()
	migrator := NewMigrator(tc.DB)

	// Create schema first
	tc.DB.ExecContext(ctx, "DROP SCHEMA IF EXISTS terminology CASCADE")
	_, err := migrator.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Drop schema
	err = migrator.Drop(ctx)
	if err != nil {
		t.Fatalf("Drop failed: %v", err)
	}

	// Verify schema is gone
	version, err := migrator.CurrentVersion(ctx)
	if err != nil {
		t.Fatalf("CurrentVersion failed: %v", err)
	}
	if version != 0 {
		t.Errorf("Expected version 0 after drop, got %d", version)
	}
}

// =============================================================================
// Table Verification Tests
// =============================================================================

func TestMigrator_Integration_TablesCreated(t *testing.T) {
	tc := setupPostgresContainer(t)
	if tc == nil {
		return
	}

	ctx := context.Background()
	migrator := NewMigrator(tc.DB)

	// Clean up and initialize
	tc.DB.ExecContext(ctx, "DROP SCHEMA IF EXISTS terminology CASCADE")
	_, err := migrator.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// List of expected tables
	expectedTables := []string{
		"releases",
		"umls_concepts",
		"umls_relations",
		"umls_semantic_types",
		"rxnorm_concepts",
		"rxnorm_relations",
		"rxnorm_ndc_xref",
		"snomed_concepts",
		"snomed_descriptions",
		"snomed_relationships",
		"snomed_transitive_closure",
		"snomed_us_preferred",
		"loinc_codes",
		"loinc_panels",
		"loinc_hierarchy",
		"loinc_answers",
		"icd10cm_codes",
		"icd10pcs_codes",
		"icd_crosswalk",
		"code_mappings",
		"schema_version",
	}

	for _, table := range expectedTables {
		var exists bool
		err := tc.DB.QueryRowContext(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM information_schema.tables
				WHERE table_schema = 'terminology' AND table_name = $1
			)
		`, table).Scan(&exists)
		if err != nil {
			t.Errorf("Failed to check table %s: %v", table, err)
			continue
		}
		if !exists {
			t.Errorf("Expected table %s to exist", table)
		}
	}
}

func TestMigrator_Integration_IndexesCreated(t *testing.T) {
	tc := setupPostgresContainer(t)
	if tc == nil {
		return
	}

	ctx := context.Background()
	migrator := NewMigrator(tc.DB)

	// Clean up and initialize
	tc.DB.ExecContext(ctx, "DROP SCHEMA IF EXISTS terminology CASCADE")
	_, err := migrator.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Check some key indexes exist
	keyIndexes := []string{
		"idx_umls_concepts_cui",
		"idx_umls_concepts_sab_code",
		"idx_snomed_tc_ancestor",
		"idx_snomed_tc_descendant",
		"idx_loinc_num",
	}

	for _, idx := range keyIndexes {
		var exists bool
		err := tc.DB.QueryRowContext(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM pg_indexes
				WHERE schemaname = 'terminology' AND indexname = $1
			)
		`, idx).Scan(&exists)
		if err != nil {
			t.Errorf("Failed to check index %s: %v", idx, err)
			continue
		}
		if !exists {
			t.Errorf("Expected index %s to exist", idx)
		}
	}
}

// =============================================================================
// Stats Tests
// =============================================================================

func TestMigrator_Integration_Stats_EmptyDatabase(t *testing.T) {
	tc := setupPostgresContainer(t)
	if tc == nil {
		return
	}

	ctx := context.Background()
	migrator := NewMigrator(tc.DB)

	// Clean up and initialize
	tc.DB.ExecContext(ctx, "DROP SCHEMA IF EXISTS terminology CASCADE")
	_, err := migrator.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Get stats
	stats, err := migrator.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats failed: %v", err)
	}

	if stats.SchemaVersion != SchemaVersion {
		t.Errorf("Expected schema version %d, got %d", SchemaVersion, stats.SchemaVersion)
	}

	if stats.TotalRows != 0 {
		t.Errorf("Expected 0 total rows, got %d", stats.TotalRows)
	}

	if len(stats.Releases) != 0 {
		t.Errorf("Expected 0 releases, got %d", len(stats.Releases))
	}
}

func TestMigrator_Integration_Stats_NoSchema(t *testing.T) {
	tc := setupPostgresContainer(t)
	if tc == nil {
		return
	}

	ctx := context.Background()
	migrator := NewMigrator(tc.DB)

	// Ensure schema doesn't exist
	tc.DB.ExecContext(ctx, "DROP SCHEMA IF EXISTS terminology CASCADE")

	// Get stats - should return version 0
	stats, err := migrator.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats failed: %v", err)
	}

	if stats.SchemaVersion != 0 {
		t.Errorf("Expected schema version 0, got %d", stats.SchemaVersion)
	}
}

// =============================================================================
// Release Management Tests
// =============================================================================

func TestMigrator_Integration_CreateRelease(t *testing.T) {
	tc := setupPostgresContainer(t)
	if tc == nil {
		return
	}

	ctx := context.Background()
	migrator := NewMigrator(tc.DB)

	// Clean up and initialize
	tc.DB.ExecContext(ctx, "DROP SCHEMA IF EXISTS terminology CASCADE")
	_, err := migrator.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Create a release
	releaseDate := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	id, err := migrator.CreateRelease(ctx, VocabLOINC, "2.77", &releaseDate)
	if err != nil {
		t.Fatalf("CreateRelease failed: %v", err)
	}

	if id <= 0 {
		t.Errorf("Expected positive release ID, got %d", id)
	}

	// Verify release was created
	release, err := migrator.GetActiveRelease(ctx, VocabLOINC)
	if err != nil {
		t.Fatalf("GetActiveRelease failed: %v", err)
	}

	if release == nil {
		t.Fatal("Expected release to exist")
	}

	if release.Vocabulary != VocabLOINC {
		t.Errorf("Expected vocabulary %s, got %s", VocabLOINC, release.Vocabulary)
	}
	if release.Version != "2.77" {
		t.Errorf("Expected version 2.77, got %s", release.Version)
	}
	if !release.IsActive {
		t.Error("Expected release to be active")
	}
}

func TestMigrator_Integration_CreateRelease_Upsert(t *testing.T) {
	tc := setupPostgresContainer(t)
	if tc == nil {
		return
	}

	ctx := context.Background()
	migrator := NewMigrator(tc.DB)

	// Clean up and initialize
	tc.DB.ExecContext(ctx, "DROP SCHEMA IF EXISTS terminology CASCADE")
	_, err := migrator.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Create initial release
	releaseDate := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	id1, err := migrator.CreateRelease(ctx, VocabLOINC, "2.77", &releaseDate)
	if err != nil {
		t.Fatalf("First CreateRelease failed: %v", err)
	}

	// Re-create same release (upsert)
	id2, err := migrator.CreateRelease(ctx, VocabLOINC, "2.77", &releaseDate)
	if err != nil {
		t.Fatalf("Second CreateRelease failed: %v", err)
	}

	// Should return same ID
	if id1 != id2 {
		t.Errorf("Upsert should return same ID: got %d and %d", id1, id2)
	}
}

func TestMigrator_Integration_SetActiveRelease(t *testing.T) {
	tc := setupPostgresContainer(t)
	if tc == nil {
		return
	}

	ctx := context.Background()
	migrator := NewMigrator(tc.DB)

	// Clean up and initialize
	tc.DB.ExecContext(ctx, "DROP SCHEMA IF EXISTS terminology CASCADE")
	_, err := migrator.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Create two releases
	date1 := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	date2 := time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC)

	_, err = migrator.CreateRelease(ctx, VocabLOINC, "2.76", &date1)
	if err != nil {
		t.Fatalf("CreateRelease 2.76 failed: %v", err)
	}

	_, err = migrator.CreateRelease(ctx, VocabLOINC, "2.77", &date2)
	if err != nil {
		t.Fatalf("CreateRelease 2.77 failed: %v", err)
	}

	// Note: CreateRelease sets is_active=TRUE for the created release,
	// but doesn't deactivate others. Use SetActiveRelease to explicitly
	// mark one as the sole active release.

	// Set 2.77 as the only active release
	err = migrator.SetActiveRelease(ctx, VocabLOINC, "2.77")
	if err != nil {
		t.Fatalf("SetActiveRelease 2.77 failed: %v", err)
	}

	// Verify 2.77 is active
	active, err := migrator.GetActiveRelease(ctx, VocabLOINC)
	if err != nil {
		t.Fatalf("GetActiveRelease failed: %v", err)
	}
	if active.Version != "2.77" {
		t.Errorf("Expected active version 2.77, got %s", active.Version)
	}

	// Set 2.76 as active
	err = migrator.SetActiveRelease(ctx, VocabLOINC, "2.76")
	if err != nil {
		t.Fatalf("SetActiveRelease 2.76 failed: %v", err)
	}

	// Verify 2.76 is now the only active
	active, err = migrator.GetActiveRelease(ctx, VocabLOINC)
	if err != nil {
		t.Fatalf("GetActiveRelease failed: %v", err)
	}
	if active.Version != "2.76" {
		t.Errorf("Expected active version 2.76, got %s", active.Version)
	}
}

func TestMigrator_Integration_UpdateReleaseRowCount(t *testing.T) {
	tc := setupPostgresContainer(t)
	if tc == nil {
		return
	}

	ctx := context.Background()
	migrator := NewMigrator(tc.DB)

	// Clean up and initialize
	tc.DB.ExecContext(ctx, "DROP SCHEMA IF EXISTS terminology CASCADE")
	_, err := migrator.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Create release
	releaseDate := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	id, err := migrator.CreateRelease(ctx, VocabLOINC, "2.77", &releaseDate)
	if err != nil {
		t.Fatalf("CreateRelease failed: %v", err)
	}

	// Update row count
	err = migrator.UpdateReleaseRowCount(ctx, id, 100000)
	if err != nil {
		t.Fatalf("UpdateReleaseRowCount failed: %v", err)
	}

	// Verify row count
	release, err := migrator.GetActiveRelease(ctx, VocabLOINC)
	if err != nil {
		t.Fatalf("GetActiveRelease failed: %v", err)
	}
	if release.RowCount != 100000 {
		t.Errorf("Expected row count 100000, got %d", release.RowCount)
	}
}

func TestMigrator_Integration_DeleteRelease(t *testing.T) {
	tc := setupPostgresContainer(t)
	if tc == nil {
		return
	}

	ctx := context.Background()
	migrator := NewMigrator(tc.DB)

	// Clean up and initialize
	tc.DB.ExecContext(ctx, "DROP SCHEMA IF EXISTS terminology CASCADE")
	_, err := migrator.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Create release
	releaseDate := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	_, err = migrator.CreateRelease(ctx, VocabLOINC, "2.77", &releaseDate)
	if err != nil {
		t.Fatalf("CreateRelease failed: %v", err)
	}

	// Delete release
	err = migrator.DeleteRelease(ctx, VocabLOINC, "2.77")
	if err != nil {
		t.Fatalf("DeleteRelease failed: %v", err)
	}

	// Verify release is gone
	release, err := migrator.GetActiveRelease(ctx, VocabLOINC)
	if err != nil {
		t.Fatalf("GetActiveRelease failed: %v", err)
	}
	if release != nil {
		t.Error("Expected release to be deleted")
	}
}

func TestMigrator_Integration_GetActiveRelease_None(t *testing.T) {
	tc := setupPostgresContainer(t)
	if tc == nil {
		return
	}

	ctx := context.Background()
	migrator := NewMigrator(tc.DB)

	// Clean up and initialize
	tc.DB.ExecContext(ctx, "DROP SCHEMA IF EXISTS terminology CASCADE")
	_, err := migrator.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Get active release for vocabulary with no releases
	release, err := migrator.GetActiveRelease(ctx, VocabSNOMEDCT)
	if err != nil {
		t.Fatalf("GetActiveRelease failed: %v", err)
	}

	if release != nil {
		t.Error("Expected nil release for vocabulary with no releases")
	}
}

// =============================================================================
// Stats with Data Tests
// =============================================================================

func TestMigrator_Integration_Stats_WithReleases(t *testing.T) {
	tc := setupPostgresContainer(t)
	if tc == nil {
		return
	}

	ctx := context.Background()
	migrator := NewMigrator(tc.DB)

	// Clean up and initialize
	tc.DB.ExecContext(ctx, "DROP SCHEMA IF EXISTS terminology CASCADE")
	_, err := migrator.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Create some releases
	date1 := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	date2 := time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)

	id1, _ := migrator.CreateRelease(ctx, VocabLOINC, "2.77", &date1)
	migrator.UpdateReleaseRowCount(ctx, id1, 100000)

	id2, _ := migrator.CreateRelease(ctx, VocabSNOMEDCT, "2024-03-01", &date2)
	migrator.UpdateReleaseRowCount(ctx, id2, 500000)

	// Get stats
	stats, err := migrator.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats failed: %v", err)
	}

	if len(stats.Releases) != 2 {
		t.Errorf("Expected 2 releases, got %d", len(stats.Releases))
	}

	// Find LOINC release
	var foundLOINC, foundSNOMED bool
	for _, r := range stats.Releases {
		if r.Vocabulary == VocabLOINC && r.Version == "2.77" {
			foundLOINC = true
			if r.RowCount != 100000 {
				t.Errorf("Expected LOINC row count 100000, got %d", r.RowCount)
			}
		}
		if r.Vocabulary == VocabSNOMEDCT && r.Version == "2024-03-01" {
			foundSNOMED = true
			if r.RowCount != 500000 {
				t.Errorf("Expected SNOMED row count 500000, got %d", r.RowCount)
			}
		}
	}

	if !foundLOINC {
		t.Error("LOINC release not found in stats")
	}
	if !foundSNOMED {
		t.Error("SNOMED release not found in stats")
	}
}

// =============================================================================
// MustInitialize Tests
// =============================================================================

func TestMigrator_Integration_MustInitialize(t *testing.T) {
	tc := setupPostgresContainer(t)
	if tc == nil {
		return
	}

	ctx := context.Background()
	migrator := NewMigrator(tc.DB)

	// Clean up any existing schema
	tc.DB.ExecContext(ctx, "DROP SCHEMA IF EXISTS terminology CASCADE")

	// MustInitialize should not panic
	created := migrator.MustInitialize(ctx)
	if !created {
		t.Error("Expected schema to be created")
	}

	// Verify schema exists
	version, err := migrator.CurrentVersion(ctx)
	if err != nil {
		t.Fatalf("CurrentVersion failed: %v", err)
	}
	if version != SchemaVersion {
		t.Errorf("Expected version %d, got %d", SchemaVersion, version)
	}
}
