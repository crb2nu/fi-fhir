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
	"regexp"
	"strings"
	"time"
)

// ICD10Loader loads ICD-10 data from official distribution files into PostgreSQL.
type ICD10Loader struct {
	db       *sql.DB
	migrator *Migrator
}

// NewICD10Loader creates a new ICD-10 database loader.
func NewICD10Loader(db *sql.DB) *ICD10Loader {
	return &ICD10Loader{
		db:       db,
		migrator: NewMigrator(db),
	}
}

// ICD10CMLoadResult contains the results of an ICD-10-CM load operation.
type ICD10CMLoadResult struct {
	ReleaseID     int
	Version       string
	CodesLoaded   int64
	HeadersLoaded int64
	Duration      time.Duration
}

// ICD10LoadOptions provides options for loading ICD-10 data.
type ICD10LoadOptions struct {
	// IncludeHeaders loads category headers in addition to billable codes.
	// Headers are codes like E11 that represent a category but are not billable.
	IncludeHeaders bool
}

// LoadICD10CMCSV loads ICD-10-CM codes from a CSV file.
// Expected CSV format: code,description,category,chapter,is_billable
func (l *ICD10Loader) LoadICD10CMCSV(ctx context.Context, path, version string, releaseDate *time.Time, progress ProgressReporter, opts *ICD10LoadOptions) (*ICD10CMLoadResult, error) {
	startTime := time.Now()
	result := &ICD10CMLoadResult{Version: version}

	if opts == nil {
		opts = &ICD10LoadOptions{IncludeHeaders: true}
	}

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
		return nil, fmt.Errorf("failed to open ICD-10-CM file: %w", err)
	}
	defer func() { _ = f.Close() }()

	// Start transaction
	tx, err := l.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to start transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Create or get release record
	releaseID, err := l.createRelease(ctx, tx, VocabICD10CM, version, releaseDate)
	if err != nil {
		return nil, err
	}
	result.ReleaseID = releaseID

	// Delete existing data for this release (in case of reload)
	if err := DeleteByReleaseID(ctx, tx, "terminology.icd10cm_codes", releaseID); err != nil {
		return nil, fmt.Errorf("failed to clear existing data: %w", err)
	}

	// Parse and load the CSV
	codesLoaded, headersLoaded, err := l.loadICD10CMFromReader(ctx, tx, f, releaseID, totalRows, progress, version, opts)
	if err != nil {
		return nil, err
	}
	result.CodesLoaded = codesLoaded
	result.HeadersLoaded = headersLoaded

	// Update release row count
	_, err = tx.ExecContext(ctx, `
		UPDATE terminology.releases SET row_count = $1 WHERE id = $2
	`, codesLoaded+headersLoaded, releaseID)
	if err != nil {
		return nil, fmt.Errorf("failed to update row count: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit: %w", err)
	}

	if err := l.migrator.SetActiveRelease(ctx, VocabICD10CM, version); err != nil {
		return nil, fmt.Errorf("failed to set active release: %w", err)
	}

	result.Duration = time.Since(startTime)
	return result, nil
}

func (l *ICD10Loader) createRelease(ctx context.Context, tx *sql.Tx, vocab, version string, releaseDate *time.Time) (int, error) {
	var id int
	err := tx.QueryRowContext(ctx, `
		INSERT INTO terminology.releases (vocabulary, version, release_date, is_active)
		VALUES ($1, $2, $3, TRUE)
		ON CONFLICT (vocabulary, version) DO UPDATE
		SET loaded_at = NOW(), is_active = TRUE
		RETURNING id
	`, vocab, version, releaseDate).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("failed to create release: %w", err)
	}
	return id, nil
}

// chapterPattern extracts chapter number from strings like "Chapter 4: ..."
var chapterPattern = regexp.MustCompile(`^Chapter\s+(\d+)`)

func (l *ICD10Loader) loadICD10CMFromReader(ctx context.Context, tx *sql.Tx, r io.Reader, releaseID int, totalRows int64, progress ProgressReporter, version string, opts *ICD10LoadOptions) (codesLoaded, headersLoaded int64, err error) {
	reader := csv.NewReader(bufio.NewReader(r))
	reader.LazyQuotes = true
	reader.FieldsPerRecord = -1

	// Read header
	header, err := reader.Read()
	if err != nil {
		return 0, 0, fmt.Errorf("failed to read header: %w", err)
	}

	// Map column names to indices
	colIdx := make(map[string]int)
	for i, col := range header {
		colIdx[strings.ToUpper(strings.TrimSpace(col))] = i
	}

	// Validate required columns
	required := []string{"CODE", "DESCRIPTION"}
	for _, col := range required {
		if _, ok := colIdx[col]; !ok {
			return 0, 0, fmt.Errorf("missing required column: %s", col)
		}
	}

	// Create batch inserter
	inserter := NewBatchInserter(tx, "terminology.icd10cm_codes", []string{
		"code", "code_formatted", "description", "short_desc", "is_header",
		"chapter", "chapter_desc", "parent_code", "release_id",
	}, 2000)

	var processed int64
	for {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			// Skip malformed rows
			continue
		}

		// Parse code (may have dots)
		codeFormatted := getCol(record, colIdx, "CODE")
		if codeFormatted == "" {
			continue
		}

		description := getCol(record, colIdx, "DESCRIPTION")
		if description == "" {
			continue
		}

		// Parse is_billable to is_header
		isBillable := strings.ToLower(getCol(record, colIdx, "IS_BILLABLE"))
		isHeader := isBillable == "false" || isBillable == "0" || isBillable == "no"

		// Skip headers if not requested
		if isHeader && !opts.IncludeHeaders {
			continue
		}

		// Remove dots for the code field (E11.9 → E119)
		code := strings.ReplaceAll(codeFormatted, ".", "")

		// Parse chapter info
		chapterFull := getCol(record, colIdx, "CHAPTER")
		chapter := extractChapterNumber(chapterFull)

		// Get parent code (category)
		parentCode := getCol(record, colIdx, "CATEGORY")
		if parentCode != "" {
			// Remove dots from parent code too
			parentCode = strings.ReplaceAll(parentCode, ".", "")
		}

		err = inserter.Add(ctx,
			code,
			nullIfEmpty(codeFormatted),
			description,
			nil, // short_desc not in our CSV
			isHeader,
			nullIfEmpty(chapter),
			nullIfEmpty(chapterFull),
			nullIfEmpty(parentCode),
			releaseID,
		)
		if err != nil {
			return codesLoaded, headersLoaded, fmt.Errorf("failed to insert ICD-10-CM code %s: %w", code, err)
		}

		if isHeader {
			headersLoaded++
		} else {
			codesLoaded++
		}
		processed++

		if progress != nil && processed%5000 == 0 {
			progress(LoadProgress{
				Vocabulary: VocabICD10CM,
				Version:    version,
				Phase:      "Loading codes",
				RowsTotal:  totalRows,
				RowsLoaded: processed,
			})
		}
	}

	// Flush remaining rows
	if err := inserter.Close(ctx); err != nil {
		return codesLoaded, headersLoaded, fmt.Errorf("failed to flush remaining rows: %w", err)
	}

	if progress != nil {
		progress(LoadProgress{
			Vocabulary: VocabICD10CM,
			Version:    version,
			Phase:      "Loading codes",
			RowsTotal:  totalRows,
			RowsLoaded: processed,
		})
	}

	return codesLoaded, headersLoaded, nil
}

// extractChapterNumber extracts the chapter number from a string like "Chapter 4: ..."
func extractChapterNumber(chapterStr string) string {
	matches := chapterPattern.FindStringSubmatch(chapterStr)
	if len(matches) >= 2 {
		// Pad to 2 digits (4 → 04)
		num := matches[1]
		if len(num) == 1 {
			return "0" + num
		}
		return num
	}
	return ""
}

// =============================================================================
// ICD-10-CM Query Functions
// =============================================================================

// ICD10CMCode represents an ICD-10-CM code from the database.
type ICD10CMCode struct {
	ID            int64
	Code          string         // Code without dots (E119)
	CodeFormatted sql.NullString // Code with dots (E11.9)
	Description   string
	ShortDesc     sql.NullString
	IsHeader      bool
	Chapter       sql.NullString
	ChapterDesc   sql.NullString
	ParentCode    sql.NullString
	ReleaseID     int
}

// DisplayCode returns the formatted code if available, otherwise the raw code.
func (c *ICD10CMCode) DisplayCode() string {
	if c.CodeFormatted.Valid && c.CodeFormatted.String != "" {
		return c.CodeFormatted.String
	}
	return c.Code
}

// IsBillable returns true if the code is billable (not a header/category).
func (c *ICD10CMCode) IsBillable() bool {
	return !c.IsHeader
}

// ICD10Queries provides query methods for ICD-10 codes.
type ICD10Queries struct {
	db *sql.DB
}

// NewICD10Queries creates a new ICD-10 query interface.
func NewICD10Queries(db *sql.DB) *ICD10Queries {
	return &ICD10Queries{db: db}
}

// GetByCode retrieves an ICD-10-CM code by its code (with or without dots).
func (q *ICD10Queries) GetByCode(ctx context.Context, code string) (*ICD10CMCode, error) {
	// Normalize: remove dots for lookup
	normalizedCode := strings.ReplaceAll(code, ".", "")

	icd := &ICD10CMCode{}
	err := q.db.QueryRowContext(ctx, `
		SELECT id, code, code_formatted, description, short_desc, is_header,
		       chapter, chapter_desc, parent_code, release_id
		FROM terminology.icd10cm_codes
		WHERE code = $1
		LIMIT 1
	`, normalizedCode).Scan(
		&icd.ID, &icd.Code, &icd.CodeFormatted, &icd.Description, &icd.ShortDesc,
		&icd.IsHeader, &icd.Chapter, &icd.ChapterDesc, &icd.ParentCode, &icd.ReleaseID,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get ICD-10-CM code: %w", err)
	}
	return icd, nil
}

// SearchByDescription searches for codes by description text (case-insensitive partial match).
func (q *ICD10Queries) SearchByDescription(ctx context.Context, query string, limit int) ([]*ICD10CMCode, error) {
	if limit <= 0 {
		limit = 100
	}

	rows, err := q.db.QueryContext(ctx, `
		SELECT id, code, code_formatted, description, short_desc, is_header,
		       chapter, chapter_desc, parent_code, release_id
		FROM terminology.icd10cm_codes
		WHERE description ILIKE $1
		ORDER BY is_header ASC, code
		LIMIT $2
	`, "%"+query+"%", limit)
	if err != nil {
		return nil, fmt.Errorf("failed to search ICD-10-CM codes: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return scanICD10CMRows(rows)
}

// GetByChapter retrieves all codes in a given chapter.
func (q *ICD10Queries) GetByChapter(ctx context.Context, chapter string, limit int) ([]*ICD10CMCode, error) {
	if limit <= 0 {
		limit = 1000
	}

	// Normalize chapter (accept "4", "04", "Chapter 4")
	normalizedChapter := normalizeChapter(chapter)

	rows, err := q.db.QueryContext(ctx, `
		SELECT id, code, code_formatted, description, short_desc, is_header,
		       chapter, chapter_desc, parent_code, release_id
		FROM terminology.icd10cm_codes
		WHERE chapter = $1
		ORDER BY code
		LIMIT $2
	`, normalizedChapter, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get ICD-10-CM codes by chapter: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return scanICD10CMRows(rows)
}

// GetChildren retrieves all child codes for a parent category code.
func (q *ICD10Queries) GetChildren(ctx context.Context, parentCode string) ([]*ICD10CMCode, error) {
	// Normalize: remove dots
	normalizedParent := strings.ReplaceAll(parentCode, ".", "")

	rows, err := q.db.QueryContext(ctx, `
		SELECT id, code, code_formatted, description, short_desc, is_header,
		       chapter, chapter_desc, parent_code, release_id
		FROM terminology.icd10cm_codes
		WHERE parent_code = $1
		ORDER BY code
	`, normalizedParent)
	if err != nil {
		return nil, fmt.Errorf("failed to get ICD-10-CM children: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return scanICD10CMRows(rows)
}

// GetBillableCodes retrieves only billable codes, optionally filtered by prefix.
func (q *ICD10Queries) GetBillableCodes(ctx context.Context, prefix string, limit int) ([]*ICD10CMCode, error) {
	if limit <= 0 {
		limit = 1000
	}

	// Normalize prefix: remove dots
	normalizedPrefix := strings.ReplaceAll(prefix, ".", "")

	var rows *sql.Rows
	var err error

	if normalizedPrefix == "" {
		rows, err = q.db.QueryContext(ctx, `
			SELECT id, code, code_formatted, description, short_desc, is_header,
			       chapter, chapter_desc, parent_code, release_id
			FROM terminology.icd10cm_codes
			WHERE is_header = FALSE
			ORDER BY code
			LIMIT $1
		`, limit)
	} else {
		rows, err = q.db.QueryContext(ctx, `
			SELECT id, code, code_formatted, description, short_desc, is_header,
			       chapter, chapter_desc, parent_code, release_id
			FROM terminology.icd10cm_codes
			WHERE is_header = FALSE AND code LIKE $1
			ORDER BY code
			LIMIT $2
		`, normalizedPrefix+"%", limit)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get billable ICD-10-CM codes: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return scanICD10CMRows(rows)
}

// GetCategories retrieves only category/header codes (non-billable).
func (q *ICD10Queries) GetCategories(ctx context.Context, limit int) ([]*ICD10CMCode, error) {
	if limit <= 0 {
		limit = 1000
	}

	rows, err := q.db.QueryContext(ctx, `
		SELECT id, code, code_formatted, description, short_desc, is_header,
		       chapter, chapter_desc, parent_code, release_id
		FROM terminology.icd10cm_codes
		WHERE is_header = TRUE
		ORDER BY code
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get ICD-10-CM categories: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return scanICD10CMRows(rows)
}

// Count returns the total number of ICD-10-CM codes.
func (q *ICD10Queries) Count(ctx context.Context) (int64, error) {
	var count int64
	err := q.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM terminology.icd10cm_codes
	`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count ICD-10-CM codes: %w", err)
	}
	return count, nil
}

// CountBillable returns the count of billable codes only.
func (q *ICD10Queries) CountBillable(ctx context.Context) (int64, error) {
	var count int64
	err := q.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM terminology.icd10cm_codes WHERE is_header = FALSE
	`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count billable ICD-10-CM codes: %w", err)
	}
	return count, nil
}

func scanICD10CMRows(rows *sql.Rows) ([]*ICD10CMCode, error) {
	var codes []*ICD10CMCode
	for rows.Next() {
		icd := &ICD10CMCode{}
		err := rows.Scan(
			&icd.ID, &icd.Code, &icd.CodeFormatted, &icd.Description, &icd.ShortDesc,
			&icd.IsHeader, &icd.Chapter, &icd.ChapterDesc, &icd.ParentCode, &icd.ReleaseID,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan ICD-10-CM row: %w", err)
		}
		codes = append(codes, icd)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error reading rows: %w", err)
	}
	return codes, nil
}

// normalizeChapter converts various chapter formats to 2-digit format.
func normalizeChapter(chapter string) string {
	// Remove "Chapter " prefix if present
	chapter = strings.TrimPrefix(chapter, "Chapter ")
	chapter = strings.TrimSpace(chapter)

	// Extract just the number if there's more text (e.g., "4: Endocrine...")
	if idx := strings.Index(chapter, ":"); idx > 0 {
		chapter = strings.TrimSpace(chapter[:idx])
	}

	// Pad to 2 digits
	if len(chapter) == 1 {
		return "0" + chapter
	}
	return chapter
}
