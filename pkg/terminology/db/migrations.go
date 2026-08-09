package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// terminologyMigrationLockKey serializes terminology schema migration across
// replicas, matching the five integration migrators
// (internal/integration/processor/postgres_submission.go:24 and siblings).
// The value is distinct from all of theirs: advisory locks share one global
// namespace, so a collision would make two unrelated migrators serialize
// against each other.
const terminologyMigrationLockKey = int64(5064657639792058903)

// Migrator handles terminology schema migrations.
type Migrator struct {
	db *sql.DB
}

// NewMigrator creates a new schema migrator.
func NewMigrator(db *sql.DB) *Migrator {
	return &Migrator{db: db}
}

// CurrentVersion returns the current schema version, or 0 if not initialized.
func (m *Migrator) CurrentVersion(ctx context.Context) (int, error) {
	return currentVersionTx(ctx, m.db)
}

// Initialize creates the terminology schema if it doesn't exist.
// Returns true if the schema was created, false if it already existed.
//
// An advisory transaction lock serializes startup across replicas, matching the
// five integration migrators (internal/integration/processor/postgres_submission.go:125-130
// and siblings). Until slice 4.4a this migrator was the only one of six that
// took no lock, so two replicas starting simultaneously against a fresh or v1
// database both executed Schema / SchemaV2Migration / SchemaV3Migration
// concurrently. `IF NOT EXISTS` makes most of that survivable but not all of
// it: two concurrent `CREATE TABLE IF NOT EXISTS` on the same name race between
// the existence check and the create and one of them raises
// `duplicate key value violates unique constraint "pg_type_typname_nsp_index"`.
// That is a rolling-upgrade defect, and it lived in the one migrator slice 4.4a
// exists to prove (.loom/32-sprint4-execution-specs.md correction 25).
//
// The version is re-read *inside* the lock. Reading it outside would leave the
// race intact: both replicas would observe version 0 before either acquired the
// lock, and the second would still try to create everything.
//
// Every statement here is ordinary transactional DDL — no CREATE INDEX
// CONCURRENTLY, no VACUUM — so wrapping the apply in one transaction also makes
// a partial upgrade impossible.
func (m *Migrator) Initialize(ctx context.Context) (bool, error) {
	if m == nil || m.db == nil || ctx == nil {
		return false, fmt.Errorf("terminology migrator is unavailable")
	}

	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin terminology migration: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, terminologyMigrationLockKey); err != nil {
		return false, fmt.Errorf("lock terminology migration: %w", err)
	}

	currentVersion, err := currentVersionTx(ctx, tx)
	if err != nil {
		return false, fmt.Errorf("checking current version: %w", err)
	}

	if currentVersion >= SchemaVersion {
		return false, nil // Already up to date
	}

	created := currentVersion == 0
	if created {
		// Fresh install: execute full schema, then every migration on top.
		if _, err := tx.ExecContext(ctx, Schema); err != nil {
			return false, fmt.Errorf("creating schema: %w", err)
		}
	}
	if currentVersion < 2 {
		if _, err := tx.ExecContext(ctx, SchemaV2Migration); err != nil {
			return false, fmt.Errorf("applying v2 migration: %w", err)
		}
	}
	if currentVersion < 3 {
		if _, err := tx.ExecContext(ctx, SchemaV3Migration); err != nil {
			return false, fmt.Errorf("applying v3 migration: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit terminology migration: %w", err)
	}
	return created, nil
}

// rowQuerier is the read surface currentVersionTx needs, satisfied by both
// *sql.DB and *sql.Tx.
type rowQuerier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// currentVersionTx reads the applied schema version through an arbitrary
// querier so Initialize can read it inside its own transaction and lock.
func currentVersionTx(ctx context.Context, q rowQuerier) (int, error) {
	var schemaExists bool
	if err := q.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.schemata WHERE schema_name = 'terminology'
		)
	`).Scan(&schemaExists); err != nil {
		return 0, fmt.Errorf("checking schema existence: %w", err)
	}
	if !schemaExists {
		return 0, nil
	}

	var tableExists bool
	if err := q.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_schema = 'terminology' AND table_name = 'schema_version'
		)
	`).Scan(&tableExists); err != nil {
		return 0, fmt.Errorf("checking version table: %w", err)
	}
	if !tableExists {
		return 0, nil
	}

	var version int
	if err := q.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(version), 0) FROM terminology.schema_version
	`).Scan(&version); err != nil {
		return 0, fmt.Errorf("getting current version: %w", err)
	}
	return version, nil
}

// MustInitialize creates the schema and panics on error.
func (m *Migrator) MustInitialize(ctx context.Context) bool {
	created, err := m.Initialize(ctx)
	if err != nil {
		panic(err)
	}
	return created
}

// Drop removes the entire terminology schema.
func (m *Migrator) Drop(ctx context.Context) error {
	_, err := m.db.ExecContext(ctx, DropSchema)
	if err != nil {
		return fmt.Errorf("dropping schema: %w", err)
	}
	return nil
}

// Stats returns statistics about the terminology database.
func (m *Migrator) Stats(ctx context.Context) (*SchemaStats, error) {
	stats := &SchemaStats{
		Tables: make(map[string]TableStats),
	}

	// Get schema version
	version, err := m.CurrentVersion(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting version: %w", err)
	}
	stats.SchemaVersion = version

	if version == 0 {
		return stats, nil // Schema not initialized
	}

	// Get table row counts
	tables := []string{
		"releases", "umls_concepts", "umls_relations", "umls_semantic_types",
		"rxnorm_concepts", "rxnorm_relations", "rxnorm_ndc_xref",
		"snomed_concepts", "snomed_descriptions", "snomed_relationships", "snomed_transitive_closure",
		"loinc_codes", "loinc_panels", "loinc_hierarchy", "loinc_answers",
		"icd10cm_codes", "icd10pcs_codes", "icd_crosswalk", "code_mappings",
		// v2 tables
		"upload_batches", "custom_mappings", "pending_autoroutes", "mapping_decisions",
	}

	for _, table := range tables {
		var count int64
		var exists bool

		// Check if table exists first
		err := m.db.QueryRowContext(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM information_schema.tables
				WHERE table_schema = 'terminology' AND table_name = $1
			)
		`, table).Scan(&exists)
		if err != nil {
			continue
		}

		if !exists {
			continue
		}

		// Get row count (use estimate for large tables)
		query := fmt.Sprintf(`SELECT COUNT(*) FROM terminology.%s`, table)
		err = m.db.QueryRowContext(ctx, query).Scan(&count)
		if err != nil {
			continue
		}

		stats.Tables[table] = TableStats{
			RowCount: count,
		}
		stats.TotalRows += count
	}

	// Get loaded releases
	rows, err := m.db.QueryContext(ctx, `
		SELECT vocabulary, version, is_active, row_count, loaded_at
		FROM terminology.releases
		ORDER BY vocabulary, loaded_at DESC
	`)
	if err == nil {
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var r ReleaseInfo
			var loadedAt time.Time
			if err := rows.Scan(&r.Vocabulary, &r.Version, &r.IsActive, &r.RowCount, &loadedAt); err == nil {
				r.LoadedAt = loadedAt
				stats.Releases = append(stats.Releases, r)
			}
		}
		// Check for iteration errors (required by rowserrcheck)
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("reading releases: %w", err)
		}
	}

	return stats, nil
}

// SchemaStats contains statistics about the terminology database.
type SchemaStats struct {
	SchemaVersion int
	TotalRows     int64
	Tables        map[string]TableStats
	Releases      []ReleaseInfo
}

// TableStats contains statistics for a single table.
type TableStats struct {
	RowCount int64
}

// ReleaseInfo contains information about a loaded terminology release.
type ReleaseInfo struct {
	Vocabulary string
	Version    string
	IsActive   bool
	RowCount   int64
	LoadedAt   time.Time
}

// Release represents a terminology release record.
type Release struct {
	ID          int
	Vocabulary  string
	Version     string
	ReleaseDate *time.Time
	LoadedAt    time.Time
	IsActive    bool
	RowCount    int64
	SourceFiles []string
	Metadata    map[string]interface{}
}

// CreateRelease inserts a new release record and returns its ID.
func (m *Migrator) CreateRelease(ctx context.Context, vocabulary, version string, releaseDate *time.Time) (int, error) {
	var id int
	err := m.db.QueryRowContext(ctx, `
		INSERT INTO terminology.releases (vocabulary, version, release_date, is_active)
		VALUES ($1, $2, $3, TRUE)
		ON CONFLICT (vocabulary, version) DO UPDATE
		SET loaded_at = NOW(), is_active = TRUE
		RETURNING id
	`, vocabulary, version, releaseDate).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("creating release: %w", err)
	}
	return id, nil
}

// SetActiveRelease marks a specific release as active and deactivates others for that vocabulary.
func (m *Migrator) SetActiveRelease(ctx context.Context, vocabulary, version string) error {
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("starting transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Deactivate all releases for this vocabulary
	_, err = tx.ExecContext(ctx, `
		UPDATE terminology.releases SET is_active = FALSE WHERE vocabulary = $1
	`, vocabulary)
	if err != nil {
		return fmt.Errorf("deactivating releases: %w", err)
	}

	// Activate the specified version
	_, err = tx.ExecContext(ctx, `
		UPDATE terminology.releases SET is_active = TRUE
		WHERE vocabulary = $1 AND version = $2
	`, vocabulary, version)
	if err != nil {
		return fmt.Errorf("activating release: %w", err)
	}

	return tx.Commit()
}

// UpdateReleaseRowCount updates the row count for a release.
func (m *Migrator) UpdateReleaseRowCount(ctx context.Context, releaseID int, rowCount int64) error {
	_, err := m.db.ExecContext(ctx, `
		UPDATE terminology.releases SET row_count = $2 WHERE id = $1
	`, releaseID, rowCount)
	return err
}

// GetActiveRelease returns the active release for a vocabulary.
func (m *Migrator) GetActiveRelease(ctx context.Context, vocabulary string) (*Release, error) {
	var r Release
	var releaseDate sql.NullTime
	err := m.db.QueryRowContext(ctx, `
		SELECT id, vocabulary, version, release_date, loaded_at, is_active, row_count
		FROM terminology.releases
		WHERE vocabulary = $1 AND is_active = TRUE
	`, vocabulary).Scan(&r.ID, &r.Vocabulary, &r.Version, &releaseDate, &r.LoadedAt, &r.IsActive, &r.RowCount)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting active release: %w", err)
	}
	if releaseDate.Valid {
		r.ReleaseDate = &releaseDate.Time
	}
	return &r, nil
}

// DeleteRelease removes a release and all associated data.
// This cascades to delete all terminology data for that release.
func (m *Migrator) DeleteRelease(ctx context.Context, vocabulary, version string) error {
	_, err := m.db.ExecContext(ctx, `
		DELETE FROM terminology.releases WHERE vocabulary = $1 AND version = $2
	`, vocabulary, version)
	if err != nil {
		return fmt.Errorf("deleting release: %w", err)
	}
	return nil
}
