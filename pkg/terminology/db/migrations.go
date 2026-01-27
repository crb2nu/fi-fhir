package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

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
	// Check if schema exists
	var schemaExists bool
	err := m.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.schemata WHERE schema_name = 'terminology'
		)
	`).Scan(&schemaExists)
	if err != nil {
		return 0, fmt.Errorf("checking schema existence: %w", err)
	}

	if !schemaExists {
		return 0, nil
	}

	// Check if version table exists
	var tableExists bool
	err = m.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_schema = 'terminology' AND table_name = 'schema_version'
		)
	`).Scan(&tableExists)
	if err != nil {
		return 0, fmt.Errorf("checking version table: %w", err)
	}

	if !tableExists {
		return 0, nil
	}

	// Get current version
	var version int
	err = m.db.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(version), 0) FROM terminology.schema_version
	`).Scan(&version)
	if err != nil {
		return 0, fmt.Errorf("getting current version: %w", err)
	}

	return version, nil
}

// Initialize creates the terminology schema if it doesn't exist.
// Returns true if the schema was created, false if it already existed.
func (m *Migrator) Initialize(ctx context.Context) (bool, error) {
	currentVersion, err := m.CurrentVersion(ctx)
	if err != nil {
		return false, fmt.Errorf("checking current version: %w", err)
	}

	if currentVersion >= SchemaVersion {
		return false, nil // Already up to date
	}

	// Fresh install: execute full schema
	if currentVersion == 0 {
		_, err = m.db.ExecContext(ctx, Schema)
		if err != nil {
			return false, fmt.Errorf("creating schema: %w", err)
		}
		// Apply v2 tables for fresh installs
		_, err = m.db.ExecContext(ctx, SchemaV2Migration)
		if err != nil {
			return false, fmt.Errorf("applying v2 migration: %w", err)
		}
		return true, nil
	}

	// Upgrade path: apply migrations incrementally
	if currentVersion < 2 {
		_, err = m.db.ExecContext(ctx, SchemaV2Migration)
		if err != nil {
			return false, fmt.Errorf("applying v2 migration: %w", err)
		}
	}

	return false, nil
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
		//nolint:gosec // G201: table name from trusted internal list, not user input
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
