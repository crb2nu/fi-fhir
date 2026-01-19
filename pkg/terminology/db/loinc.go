// Package db provides database operations for terminology data.
package db

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

// LOINCLoader loads LOINC data from official distribution files into PostgreSQL.
type LOINCLoader struct {
	db       *sql.DB
	migrator *Migrator
}

// NewLOINCLoader creates a new LOINC database loader.
func NewLOINCLoader(db *sql.DB) *LOINCLoader {
	return &LOINCLoader{
		db:       db,
		migrator: NewMigrator(db),
	}
}

// LOINCLoadResult contains the results of a LOINC load operation.
type LOINCLoadResult struct {
	ReleaseID    int
	Version      string
	CodesLoaded  int64
	PanelsLoaded int64
	Duration     time.Duration
}

// LoadLoincTable loads LOINC codes from LoincTable.csv.
func (l *LOINCLoader) LoadLoincTable(ctx context.Context, path, version string, releaseDate *time.Time, progress ProgressReporter) (*LOINCLoadResult, error) {
	startTime := time.Now()
	result := &LOINCLoadResult{Version: version}

	// Count rows for progress reporting
	var totalRows int64
	if progress != nil {
		var err error
		totalRows, err = CountCSVRows(path)
		if err != nil {
			return nil, fmt.Errorf("failed to count rows: %w", err)
		}
	}

	// Open the file
	f, err := OpenFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open LOINC table: %w", err)
	}
	defer func() { _ = f.Close() }()

	// Start transaction
	tx, err := l.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to start transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Create or get release record
	releaseID, err := l.createRelease(ctx, tx, version, releaseDate)
	if err != nil {
		return nil, err
	}
	result.ReleaseID = releaseID

	// Delete existing data for this release (in case of reload)
	if err := DeleteByReleaseID(ctx, tx, "terminology.loinc_codes", releaseID); err != nil {
		return nil, fmt.Errorf("failed to clear existing data: %w", err)
	}

	// Parse and load the CSV
	loaded, err := l.loadLoincTableFromReader(ctx, tx, f, releaseID, totalRows, progress, version)
	if err != nil {
		return nil, err
	}
	result.CodesLoaded = loaded

	// Update release row count
	_, err = tx.ExecContext(ctx, `
		UPDATE terminology.releases SET row_count = $1 WHERE id = $2
	`, loaded, releaseID)
	if err != nil {
		return nil, fmt.Errorf("failed to update row count: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit: %w", err)
	}

	if err := l.migrator.SetActiveRelease(ctx, VocabLOINC, version); err != nil {
		return nil, fmt.Errorf("failed to set active release: %w", err)
	}

	result.Duration = time.Since(startTime)
	return result, nil
}

// LoadPanelHierarchy loads panel relationships from PanelHierarchy.csv.
func (l *LOINCLoader) LoadPanelHierarchy(ctx context.Context, path string, releaseID int, progress ProgressReporter, version string) (int64, error) {
	// Count rows for progress
	var totalRows int64
	if progress != nil {
		var err error
		totalRows, err = CountCSVRows(path)
		if err != nil {
			return 0, fmt.Errorf("failed to count rows: %w", err)
		}
	}

	// Open the file
	f, err := OpenFile(path)
	if err != nil {
		return 0, fmt.Errorf("failed to open panel hierarchy: %w", err)
	}
	defer func() { _ = f.Close() }()

	// Start transaction
	tx, err := l.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to start transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Delete existing panel data for this release
	if err := DeleteByReleaseID(ctx, tx, "terminology.loinc_panels", releaseID); err != nil {
		return 0, fmt.Errorf("failed to clear existing panel data: %w", err)
	}

	// Parse and load
	loaded, err := l.loadPanelHierarchyFromReader(ctx, tx, f, releaseID, totalRows, progress, version)
	if err != nil {
		return 0, err
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("failed to commit: %w", err)
	}

	return loaded, nil
}

func (l *LOINCLoader) createRelease(ctx context.Context, tx *sql.Tx, version string, releaseDate *time.Time) (int, error) {
	var id int
	err := tx.QueryRowContext(ctx, `
		INSERT INTO terminology.releases (vocabulary, version, release_date, is_active)
		VALUES ($1, $2, $3, TRUE)
		ON CONFLICT (vocabulary, version) DO UPDATE
		SET loaded_at = NOW(), is_active = TRUE
		RETURNING id
	`, VocabLOINC, version, releaseDate).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("failed to create release: %w", err)
	}
	return id, nil
}

func (l *LOINCLoader) loadLoincTableFromReader(ctx context.Context, tx *sql.Tx, r io.Reader, releaseID int, totalRows int64, progress ProgressReporter, version string) (int64, error) {
	reader := csv.NewReader(bufio.NewReader(r))
	reader.LazyQuotes = true
	reader.FieldsPerRecord = -1

	// Read header
	header, err := reader.Read()
	if err != nil {
		return 0, fmt.Errorf("failed to read header: %w", err)
	}

	// Map column names to indices
	colIdx := make(map[string]int)
	for i, col := range header {
		colIdx[strings.ToUpper(strings.TrimSpace(col))] = i
	}

	// Validate required columns
	required := []string{"LOINC_NUM", "LONG_COMMON_NAME", "STATUS"}
	for _, col := range required {
		if _, ok := colIdx[col]; !ok {
			return 0, fmt.Errorf("missing required column: %s", col)
		}
	}

	// Create batch inserter
	inserter := NewBatchInserter(tx, "terminology.loinc_codes", []string{
		"loinc_num", "component", "property", "time_aspct", "system", "scale_typ",
		"method_typ", "class", "classtype", "long_common_name", "shortname",
		"consumer_name", "status", "version_first_released", "version_last_changed",
		"order_obs", "example_units", "units_required", "release_id",
	}, 2000)

	var loaded int64
	for {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			// Skip malformed rows
			continue
		}

		loincNum := getCol(record, colIdx, "LOINC_NUM")
		if loincNum == "" {
			continue
		}

		longName := getCol(record, colIdx, "LONG_COMMON_NAME")
		if longName == "" {
			continue
		}

		status := getCol(record, colIdx, "STATUS")
		if status == "" {
			status = "ACTIVE"
		}

		err = inserter.Add(ctx,
			loincNum,
			nullIfEmpty(getCol(record, colIdx, "COMPONENT")),
			nullIfEmpty(getCol(record, colIdx, "PROPERTY")),
			nullIfEmpty(getCol(record, colIdx, "TIME_ASPCT")),
			nullIfEmpty(getCol(record, colIdx, "SYSTEM")),
			nullIfEmpty(getCol(record, colIdx, "SCALE_TYP")),
			nullIfEmpty(getCol(record, colIdx, "METHOD_TYP")),
			nullIfEmpty(getCol(record, colIdx, "CLASS")),
			nullIfEmpty(getCol(record, colIdx, "CLASSTYPE")),
			longName,
			nullIfEmpty(getCol(record, colIdx, "SHORTNAME")),
			nullIfEmpty(getCol(record, colIdx, "CONSUMER_NAME")),
			status,
			nullIfEmpty(getCol(record, colIdx, "VERSIONFIRSTRELEASED")),
			nullIfEmpty(getCol(record, colIdx, "VERSIONLASTCHANGED")),
			nullIfEmpty(getCol(record, colIdx, "ORDER_OBS")),
			nullIfEmpty(getCol(record, colIdx, "EXAMPLE_UNITS")),
			nullIfEmpty(getCol(record, colIdx, "UNITSREQUIRED")),
			releaseID,
		)
		if err != nil {
			return loaded, fmt.Errorf("failed to insert LOINC code %s: %w", loincNum, err)
		}

		loaded++
		if progress != nil && loaded%5000 == 0 {
			progress(LoadProgress{
				Vocabulary: VocabLOINC,
				Version:    version,
				Phase:      "Loading codes",
				RowsTotal:  totalRows,
				RowsLoaded: loaded,
			})
		}
	}

	// Flush remaining rows
	if err := inserter.Close(ctx); err != nil {
		return loaded, fmt.Errorf("failed to flush remaining rows: %w", err)
	}

	if progress != nil {
		progress(LoadProgress{
			Vocabulary: VocabLOINC,
			Version:    version,
			Phase:      "Loading codes",
			RowsTotal:  totalRows,
			RowsLoaded: loaded,
		})
	}

	return loaded, nil
}

func (l *LOINCLoader) loadPanelHierarchyFromReader(ctx context.Context, tx *sql.Tx, r io.Reader, releaseID int, totalRows int64, progress ProgressReporter, version string) (int64, error) {
	reader := csv.NewReader(bufio.NewReader(r))
	reader.LazyQuotes = true
	reader.FieldsPerRecord = -1

	// Read header
	header, err := reader.Read()
	if err != nil {
		return 0, fmt.Errorf("failed to read header: %w", err)
	}

	// Map column names
	colIdx := make(map[string]int)
	for i, col := range header {
		colIdx[strings.ToUpper(strings.TrimSpace(col))] = i
	}

	// Create batch inserter
	inserter := NewBatchInserter(tx, "terminology.loinc_panels", []string{
		"parent_loinc", "loinc", "sequence", "cardinality", "release_id",
	}, 2000)

	var loaded int64
	for {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			continue
		}

		parentCode := getCol(record, colIdx, "PARENTLOINC")
		if parentCode == "" {
			parentCode = getCol(record, colIdx, "PARENT_LOINC")
		}
		memberCode := getCol(record, colIdx, "LOINC")

		if parentCode == "" || memberCode == "" {
			continue
		}

		// Parse sequence number
		var sequence interface{}
		if seqStr := getCol(record, colIdx, "SEQUENCE"); seqStr != "" {
			if seq, err := strconv.Atoi(seqStr); err == nil {
				sequence = seq
			}
		}

		cardinality := nullIfEmpty(getCol(record, colIdx, "CARDINALITY"))

		err = inserter.Add(ctx, parentCode, memberCode, sequence, cardinality, releaseID)
		if err != nil {
			return loaded, fmt.Errorf("failed to insert panel member: %w", err)
		}

		loaded++
		if progress != nil && loaded%5000 == 0 {
			progress(LoadProgress{
				Vocabulary: VocabLOINC,
				Version:    version,
				Phase:      "Loading panels",
				RowsTotal:  totalRows,
				RowsLoaded: loaded,
			})
		}
	}

	// Flush remaining rows
	if err := inserter.Close(ctx); err != nil {
		return loaded, fmt.Errorf("failed to flush remaining panel rows: %w", err)
	}

	return loaded, nil
}

// getCol safely gets a column value from a CSV record.
func getCol(record []string, colIdx map[string]int, colName string) string {
	idx, ok := colIdx[colName]
	if !ok || idx >= len(record) {
		return ""
	}
	return strings.TrimSpace(record[idx])
}

// nullIfEmpty returns nil for empty strings, otherwise the string value.
func nullIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// =============================================================================
// LOINC Query Functions
// =============================================================================

// LOINCCode represents a LOINC code from the database.
type LOINCCode struct {
	ID             int64
	LOINCNum       string
	Component      sql.NullString
	Property       sql.NullString
	TimeAspct      sql.NullString
	System         sql.NullString
	ScaleTyp       sql.NullString
	MethodTyp      sql.NullString
	Class          sql.NullString
	ClassType      sql.NullString
	LongCommonName string
	ShortName      sql.NullString
	ConsumerName   sql.NullString
	Status         string
	OrderObs       sql.NullString
	ExampleUnits   sql.NullString
	ReleaseID      int
}

// DisplayName returns the best display name for the code.
func (c *LOINCCode) DisplayName() string {
	if c.ConsumerName.Valid && c.ConsumerName.String != "" {
		return c.ConsumerName.String
	}
	if c.ShortName.Valid && c.ShortName.String != "" {
		return c.ShortName.String
	}
	return c.LongCommonName
}

// IsActive returns true if the code is active.
func (c *LOINCCode) IsActive() bool {
	return c.Status == "ACTIVE"
}

// LOINCQueries provides query methods for LOINC codes.
type LOINCQueries struct {
	db *sql.DB
}

// NewLOINCQueries creates a new LOINC query interface.
func NewLOINCQueries(db *sql.DB) *LOINCQueries {
	return &LOINCQueries{db: db}
}

// GetByCode retrieves a LOINC code by its code number.
func (q *LOINCQueries) GetByCode(ctx context.Context, loincNum string) (*LOINCCode, error) {
	code := &LOINCCode{}
	err := q.db.QueryRowContext(ctx, `
		SELECT id, loinc_num, component, property, time_aspct, system, scale_typ,
		       method_typ, class, classtype, long_common_name, shortname,
		       consumer_name, status, order_obs, example_units, release_id
		FROM terminology.loinc_codes
		WHERE loinc_num = $1 AND status = 'ACTIVE'
		LIMIT 1
	`, loincNum).Scan(
		&code.ID, &code.LOINCNum, &code.Component, &code.Property, &code.TimeAspct,
		&code.System, &code.ScaleTyp, &code.MethodTyp, &code.Class, &code.ClassType,
		&code.LongCommonName, &code.ShortName, &code.ConsumerName, &code.Status,
		&code.OrderObs, &code.ExampleUnits, &code.ReleaseID,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get LOINC code: %w", err)
	}
	return code, nil
}

// SearchByComponent searches for codes by component text (case-insensitive partial match).
func (q *LOINCQueries) SearchByComponent(ctx context.Context, query string, limit int) ([]*LOINCCode, error) {
	if limit <= 0 {
		limit = 100
	}

	rows, err := q.db.QueryContext(ctx, `
		SELECT id, loinc_num, component, property, time_aspct, system, scale_typ,
		       method_typ, class, classtype, long_common_name, shortname,
		       consumer_name, status, order_obs, example_units, release_id
		FROM terminology.loinc_codes
		WHERE status = 'ACTIVE' AND component ILIKE $1
		ORDER BY long_common_name
		LIMIT $2
	`, "%"+query+"%", limit)
	if err != nil {
		return nil, fmt.Errorf("failed to search LOINC codes: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return scanLOINCRows(rows)
}

// SearchByName searches for codes by name (searches long_common_name, shortname, consumer_name).
func (q *LOINCQueries) SearchByName(ctx context.Context, query string, limit int) ([]*LOINCCode, error) {
	if limit <= 0 {
		limit = 100
	}

	rows, err := q.db.QueryContext(ctx, `
		SELECT id, loinc_num, component, property, time_aspct, system, scale_typ,
		       method_typ, class, classtype, long_common_name, shortname,
		       consumer_name, status, order_obs, example_units, release_id
		FROM terminology.loinc_codes
		WHERE status = 'ACTIVE'
		  AND (long_common_name ILIKE $1 OR shortname ILIKE $1 OR consumer_name ILIKE $1)
		ORDER BY long_common_name
		LIMIT $2
	`, "%"+query+"%", limit)
	if err != nil {
		return nil, fmt.Errorf("failed to search LOINC codes: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return scanLOINCRows(rows)
}

// GetByClass retrieves all codes in a given class.
func (q *LOINCQueries) GetByClass(ctx context.Context, class string, limit int) ([]*LOINCCode, error) {
	if limit <= 0 {
		limit = 1000
	}

	rows, err := q.db.QueryContext(ctx, `
		SELECT id, loinc_num, component, property, time_aspct, system, scale_typ,
		       method_typ, class, classtype, long_common_name, shortname,
		       consumer_name, status, order_obs, example_units, release_id
		FROM terminology.loinc_codes
		WHERE status = 'ACTIVE' AND class = $1
		ORDER BY long_common_name
		LIMIT $2
	`, class, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get LOINC codes by class: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return scanLOINCRows(rows)
}

// GetPanelMembers retrieves all member codes for a panel.
func (q *LOINCQueries) GetPanelMembers(ctx context.Context, panelCode string) ([]*LOINCCode, error) {
	rows, err := q.db.QueryContext(ctx, `
		SELECT c.id, c.loinc_num, c.component, c.property, c.time_aspct, c.system, c.scale_typ,
		       c.method_typ, c.class, c.classtype, c.long_common_name, c.shortname,
		       c.consumer_name, c.status, c.order_obs, c.example_units, c.release_id
		FROM terminology.loinc_panels p
		JOIN terminology.loinc_codes c ON c.loinc_num = p.loinc
		WHERE p.parent_loinc = $1 AND c.status = 'ACTIVE'
		ORDER BY p.sequence NULLS LAST, c.long_common_name
	`, panelCode)
	if err != nil {
		return nil, fmt.Errorf("failed to get panel members: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return scanLOINCRows(rows)
}

// GetParentPanels retrieves panels that contain the given code.
func (q *LOINCQueries) GetParentPanels(ctx context.Context, loincNum string) ([]*LOINCCode, error) {
	rows, err := q.db.QueryContext(ctx, `
		SELECT c.id, c.loinc_num, c.component, c.property, c.time_aspct, c.system, c.scale_typ,
		       c.method_typ, c.class, c.classtype, c.long_common_name, c.shortname,
		       c.consumer_name, c.status, c.order_obs, c.example_units, c.release_id
		FROM terminology.loinc_panels p
		JOIN terminology.loinc_codes c ON c.loinc_num = p.parent_loinc
		WHERE p.loinc = $1 AND c.status = 'ACTIVE'
		ORDER BY c.long_common_name
	`, loincNum)
	if err != nil {
		return nil, fmt.Errorf("failed to get parent panels: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return scanLOINCRows(rows)
}

// IsPanel returns true if the code is a panel.
func (q *LOINCQueries) IsPanel(ctx context.Context, loincNum string) (bool, error) {
	var count int
	err := q.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM terminology.loinc_panels WHERE parent_loinc = $1 LIMIT 1
	`, loincNum).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check if panel: %w", err)
	}
	return count > 0, nil
}

// Count returns the total number of active LOINC codes.
func (q *LOINCQueries) Count(ctx context.Context) (int64, error) {
	var count int64
	err := q.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM terminology.loinc_codes WHERE status = 'ACTIVE'
	`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count LOINC codes: %w", err)
	}
	return count, nil
}

func scanLOINCRows(rows *sql.Rows) ([]*LOINCCode, error) {
	var codes []*LOINCCode
	for rows.Next() {
		code := &LOINCCode{}
		err := rows.Scan(
			&code.ID, &code.LOINCNum, &code.Component, &code.Property, &code.TimeAspct,
			&code.System, &code.ScaleTyp, &code.MethodTyp, &code.Class, &code.ClassType,
			&code.LongCommonName, &code.ShortName, &code.ConsumerName, &code.Status,
			&code.OrderObs, &code.ExampleUnits, &code.ReleaseID,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan LOINC row: %w", err)
		}
		codes = append(codes, code)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error reading rows: %w", err)
	}
	return codes, nil
}
