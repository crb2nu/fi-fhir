package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// UMLSLoader handles loading UMLS Metathesaurus files into PostgreSQL.
// It processes the core RRF files: MRCONSO, MRREL, and MRSTY.
type UMLSLoader struct {
	db       *sql.DB
	migrator *Migrator
}

// NewUMLSLoader creates a new UMLS loader.
func NewUMLSLoader(db *sql.DB) *UMLSLoader {
	return &UMLSLoader{
		db:       db,
		migrator: NewMigrator(db),
	}
}

// UMLSLoadResult contains statistics from a UMLS load operation.
type UMLSLoadResult struct {
	ReleaseID       int
	Version         string
	ConceptsLoaded  int64 // MRCONSO rows
	RelationsLoaded int64 // MRREL rows
	SemTypesLoaded  int64 // MRSTY rows
	Duration        time.Duration
	SourcesFiltered []string // If filtered by SAB
}

// UMLSLoadOptions configures the UMLS load process.
type UMLSLoadOptions struct {
	// FilterSources limits loading to specific source vocabularies (SABs).
	// Empty means load all sources. Common values: SNOMEDCT_US, ICD10CM, RXNORM
	FilterSources []string

	// EnglishOnly filters to only English language terms (LAT = ENG).
	EnglishOnly bool

	// SkipSuppressed skips rows with SUPPRESS != 'N'.
	SkipSuppressed bool

	// SkipRelations skips loading MRREL (relationships).
	SkipRelations bool

	// SkipSemanticTypes skips loading MRSTY.
	SkipSemanticTypes bool
}

// LoadMETA loads UMLS Metathesaurus from a META directory.
// The directory should contain MRCONSO.RRF, MRREL.RRF, and MRSTY.RRF files.
func (l *UMLSLoader) LoadMETA(ctx context.Context, metaDir, version string, releaseDate *time.Time, opts *UMLSLoadOptions, progress ProgressReporter) (*UMLSLoadResult, error) {
	start := time.Now()
	result := &UMLSLoadResult{Version: version}

	if opts == nil {
		opts = &UMLSLoadOptions{
			EnglishOnly:    true,
			SkipSuppressed: true,
		}
	}

	result.SourcesFiltered = opts.FilterSources

	// Validate directory structure
	if err := ValidateUMLSDirectory(metaDir); err != nil {
		return nil, fmt.Errorf("invalid UMLS directory: %w", err)
	}

	// Create or get release
	releaseID, err := l.migrator.CreateRelease(ctx, VocabUMLS, version, releaseDate)
	if err != nil {
		return nil, fmt.Errorf("create release: %w", err)
	}
	result.ReleaseID = releaseID

	// Clear existing data for this release
	if err := l.clearReleaseData(ctx, releaseID); err != nil {
		return nil, fmt.Errorf("clear existing data: %w", err)
	}

	// Load MRCONSO (concepts/atoms)
	mrconsoPath := findRRFFile(metaDir, "MRCONSO.RRF")
	if progress != nil {
		progress(LoadProgress{Vocabulary: VocabUMLS, Version: version, Phase: "Counting MRCONSO"})
	}

	consoRows, err := CountRRFRows(mrconsoPath)
	if err != nil {
		return nil, fmt.Errorf("count MRCONSO: %w", err)
	}

	if progress != nil {
		progress(LoadProgress{Vocabulary: VocabUMLS, Version: version, Phase: "Loading MRCONSO", RowsTotal: consoRows})
	}

	loaded, err := l.loadMRCONSO(ctx, mrconsoPath, releaseID, opts, progress, consoRows)
	if err != nil {
		return nil, fmt.Errorf("load MRCONSO: %w", err)
	}
	result.ConceptsLoaded = loaded

	// Load MRREL (relationships)
	if !opts.SkipRelations {
		mrrelPath := findRRFFile(metaDir, "MRREL.RRF")
		if progress != nil {
			progress(LoadProgress{Vocabulary: VocabUMLS, Version: version, Phase: "Counting MRREL"})
		}

		relRows, err := CountRRFRows(mrrelPath)
		if err != nil {
			return nil, fmt.Errorf("count MRREL: %w", err)
		}

		if progress != nil {
			progress(LoadProgress{Vocabulary: VocabUMLS, Version: version, Phase: "Loading MRREL", RowsTotal: relRows})
		}

		loaded, err = l.loadMRREL(ctx, mrrelPath, releaseID, opts, progress, relRows)
		if err != nil {
			return nil, fmt.Errorf("load MRREL: %w", err)
		}
		result.RelationsLoaded = loaded
	}

	// Load MRSTY (semantic types)
	if !opts.SkipSemanticTypes {
		mrstyPath := findRRFFile(metaDir, "MRSTY.RRF")
		if progress != nil {
			progress(LoadProgress{Vocabulary: VocabUMLS, Version: version, Phase: "Counting MRSTY"})
		}

		styRows, err := CountRRFRows(mrstyPath)
		if err != nil {
			return nil, fmt.Errorf("count MRSTY: %w", err)
		}

		if progress != nil {
			progress(LoadProgress{Vocabulary: VocabUMLS, Version: version, Phase: "Loading MRSTY", RowsTotal: styRows})
		}

		loaded, err = l.loadMRSTY(ctx, mrstyPath, releaseID, opts, progress, styRows)
		if err != nil {
			return nil, fmt.Errorf("load MRSTY: %w", err)
		}
		result.SemTypesLoaded = loaded
	}

	// Update release stats
	totalRows := result.ConceptsLoaded + result.RelationsLoaded + result.SemTypesLoaded
	if err := l.migrator.UpdateReleaseRowCount(ctx, releaseID, totalRows); err != nil {
		return nil, fmt.Errorf("update release stats: %w", err)
	}

	// Set as active release
	if err := l.migrator.SetActiveRelease(ctx, VocabUMLS, version); err != nil {
		return nil, fmt.Errorf("set active release: %w", err)
	}

	result.Duration = time.Since(start)
	return result, nil
}

// loadMRCONSO loads the MRCONSO.RRF file (concept atoms).
func (l *UMLSLoader) loadMRCONSO(ctx context.Context, path string, releaseID int, opts *UMLSLoadOptions, progress ProgressReporter, totalRows int64) (int64, error) {
	reader, err := OpenRRFFile(path, MRCONSOColumns)
	if err != nil {
		return 0, err
	}
	defer func() { _ = reader.Close() }()
	reader.SetTotalLines(totalRows)

	// Build source filter set
	filterSet := make(map[string]bool)
	for _, sab := range opts.FilterSources {
		filterSet[sab] = true
	}
	hasFilter := len(filterSet) > 0

	tx, err := l.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	inserter := NewBatchInserter(tx, "terminology.umls_concepts",
		[]string{"cui", "lat", "ts", "lui", "stt", "sui", "ispref", "aui", "saui", "scui", "sdui", "sab", "tty", "code", "str", "srl", "suppress", "cvf", "release_id"},
		5000, // Larger batch for bulk loading
	)

	var loaded, skipped int64
	lastProgress := time.Now()

	for {
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		default:
		}

		row, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return 0, err
		}

		// Apply filters
		sab := RRFField(row, MRCONSOColSAB)
		lat := RRFField(row, MRCONSOColLAT)
		suppress := RRFField(row, MRCONSOColSUPPRESS)

		if hasFilter && !filterSet[sab] {
			skipped++
			continue
		}
		if opts.EnglishOnly && lat != "ENG" {
			skipped++
			continue
		}
		if opts.SkipSuppressed && suppress != "N" {
			skipped++
			continue
		}

		// Insert row
		if err := inserter.Add(ctx,
			RRFField(row, MRCONSOColCUI),
			lat,
			RRFField(row, MRCONSOColTS),
			RRFField(row, MRCONSOColLUI),
			RRFField(row, MRCONSOColSTT),
			RRFField(row, MRCONSOColSUI),
			RRFField(row, MRCONSOColISPREF),
			RRFField(row, MRCONSOColAUI),
			RRFField(row, MRCONSOColSAUI),
			RRFField(row, MRCONSOColSCUI),
			RRFField(row, MRCONSOColSDUI),
			sab,
			RRFField(row, MRCONSOColTTY),
			RRFField(row, MRCONSOColCODE),
			RRFField(row, MRCONSOColSTR),
			RRFFieldInt(row, MRCONSOColSRL),
			suppress,
			nullableInt(RRFFieldInt(row, MRCONSOColCVF)),
			releaseID,
		); err != nil {
			return 0, fmt.Errorf("insert row %d: %w", reader.LineNum(), err)
		}
		loaded++

		// Progress reporting (every 2 seconds)
		if progress != nil && time.Since(lastProgress) > 2*time.Second {
			progress(LoadProgress{
				Vocabulary:  VocabUMLS,
				Phase:       "Loading MRCONSO",
				RowsTotal:   totalRows,
				RowsLoaded:  loaded,
				RowsSkipped: skipped,
			})
			lastProgress = time.Now()
		}
	}

	// Flush remaining rows
	if err := inserter.Flush(ctx); err != nil {
		return 0, fmt.Errorf("flush remaining: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}

	return loaded, nil
}

// loadMRREL loads the MRREL.RRF file (relationships).
func (l *UMLSLoader) loadMRREL(ctx context.Context, path string, releaseID int, opts *UMLSLoadOptions, progress ProgressReporter, totalRows int64) (int64, error) {
	reader, err := OpenRRFFile(path, MRRELColumns)
	if err != nil {
		return 0, err
	}
	defer func() { _ = reader.Close() }()
	reader.SetTotalLines(totalRows)

	// Build source filter set
	filterSet := make(map[string]bool)
	for _, sab := range opts.FilterSources {
		filterSet[sab] = true
	}
	hasFilter := len(filterSet) > 0

	tx, err := l.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	inserter := NewBatchInserter(tx, "terminology.umls_relations",
		[]string{"cui1", "aui1", "stype1", "rel", "cui2", "aui2", "stype2", "rela", "rui", "srui", "sab", "sl", "rg", "dir", "suppress", "cvf", "release_id"},
		5000,
	)

	var loaded, skipped int64
	lastProgress := time.Now()

	for {
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		default:
		}

		row, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return 0, err
		}

		// Apply filters
		sab := RRFField(row, MRRELColSAB)
		suppress := RRFField(row, MRRELColSUPPRESS)

		if hasFilter && !filterSet[sab] {
			skipped++
			continue
		}
		if opts.SkipSuppressed && suppress != "N" {
			skipped++
			continue
		}

		if err := inserter.Add(ctx,
			RRFField(row, MRRELColCUI1),
			nullableString(RRFField(row, MRRELColAUI1)),
			RRFField(row, MRRELColSTYPE1),
			RRFField(row, MRRELColREL),
			RRFField(row, MRRELColCUI2),
			nullableString(RRFField(row, MRRELColAUI2)),
			RRFField(row, MRRELColSTYPE2),
			nullableString(RRFField(row, MRRELColRELA)),
			nullableString(RRFField(row, MRRELColRUI)),
			nullableString(RRFField(row, MRRELColSRUI)),
			sab,
			RRFField(row, MRRELColSL),
			nullableString(RRFField(row, MRRELColRG)),
			nullableString(RRFField(row, MRRELColDIR)),
			suppress,
			nullableInt(RRFFieldInt(row, MRRELColCVF)),
			releaseID,
		); err != nil {
			return 0, fmt.Errorf("insert row %d: %w", reader.LineNum(), err)
		}
		loaded++

		if progress != nil && time.Since(lastProgress) > 2*time.Second {
			progress(LoadProgress{
				Vocabulary:  VocabUMLS,
				Phase:       "Loading MRREL",
				RowsTotal:   totalRows,
				RowsLoaded:  loaded,
				RowsSkipped: skipped,
			})
			lastProgress = time.Now()
		}
	}

	if err := inserter.Flush(ctx); err != nil {
		return 0, fmt.Errorf("flush remaining: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}

	return loaded, nil
}

// loadMRSTY loads the MRSTY.RRF file (semantic types).
func (l *UMLSLoader) loadMRSTY(ctx context.Context, path string, releaseID int, opts *UMLSLoadOptions, progress ProgressReporter, totalRows int64) (int64, error) {
	reader, err := OpenRRFFile(path, MRSTYColumns)
	if err != nil {
		return 0, err
	}
	defer func() { _ = reader.Close() }()
	reader.SetTotalLines(totalRows)

	// MRSTY.RRF has no SAB column, so it cannot be filtered by source directly.
	// When a source filter is active, restrict semantic types to the CUIs that
	// survived the concept-level filter in MRCONSO; otherwise we would load
	// semantic types for concepts that were never loaded.
	var cuiFilter map[string]struct{}
	if len(opts.FilterSources) > 0 {
		cuiFilter, err = l.loadedConceptCUIs(ctx, releaseID)
		if err != nil {
			return 0, fmt.Errorf("collect filtered CUIs: %w", err)
		}
	}

	tx, err := l.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	inserter := NewBatchInserter(tx, "terminology.umls_semantic_types",
		[]string{"cui", "tui", "stn", "sty", "atui", "cvf", "release_id"},
		5000,
	)

	var loaded int64
	lastProgress := time.Now()

	for {
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		default:
		}

		row, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return 0, err
		}

		if cuiFilter != nil {
			if _, ok := cuiFilter[strings.TrimSpace(RRFField(row, MRSTYColCUI))]; !ok {
				continue
			}
		}

		if err := inserter.Add(ctx,
			RRFField(row, MRSTYColCUI),
			RRFField(row, MRSTYColTUI),
			nullableString(RRFField(row, MRSTYColSTN)),
			RRFField(row, MRSTYColSTY),
			nullableString(RRFField(row, MRSTYColATUI)),
			nullableInt(RRFFieldInt(row, MRSTYColCVF)),
			releaseID,
		); err != nil {
			return 0, fmt.Errorf("insert row %d: %w", reader.LineNum(), err)
		}
		loaded++

		if progress != nil && time.Since(lastProgress) > 2*time.Second {
			progress(LoadProgress{
				Vocabulary: VocabUMLS,
				Phase:      "Loading MRSTY",
				RowsTotal:  totalRows,
				RowsLoaded: loaded,
			})
			lastProgress = time.Now()
		}
	}

	if err := inserter.Flush(ctx); err != nil {
		return 0, fmt.Errorf("flush remaining: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}

	return loaded, nil
}

// loadedConceptCUIs returns the set of distinct CUIs present in umls_concepts
// for a release. Used to scope MRSTY (which has no SAB column) to the concepts
// that survived a source filter.
func (l *UMLSLoader) loadedConceptCUIs(ctx context.Context, releaseID int) (map[string]struct{}, error) {
	rows, err := l.db.QueryContext(ctx,
		"SELECT DISTINCT cui FROM terminology.umls_concepts WHERE release_id = $1", releaseID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	cuis := make(map[string]struct{})
	for rows.Next() {
		var cui string
		if err := rows.Scan(&cui); err != nil {
			return nil, err
		}
		cuis[strings.TrimSpace(cui)] = struct{}{}
	}

	return cuis, rows.Err()
}

// clearReleaseData removes existing data for a release.
func (l *UMLSLoader) clearReleaseData(ctx context.Context, releaseID int) error {
	tables := []string{
		"terminology.umls_concepts",
		"terminology.umls_relations",
		"terminology.umls_semantic_types",
	}

	for _, table := range tables {
		_, err := l.db.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s WHERE release_id = $1", table), releaseID)
		if err != nil {
			return fmt.Errorf("clear %s: %w", table, err)
		}
	}

	return nil
}

// findRRFFile locates an RRF file in a directory, checking for .gz version too.
func findRRFFile(dir, filename string) string {
	path := filepath.Join(dir, filename)
	if fileExists(path) {
		return path
	}
	gzPath := path + ".gz"
	if fileExists(gzPath) {
		return gzPath
	}
	return path // Return original path (will error on open)
}

// fileExists checks if a file exists.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// nullableString returns nil for empty strings.
func nullableString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// nullableInt returns nil for zero values.
func nullableInt(i int) interface{} {
	if i == 0 {
		return nil
	}
	return i
}

// =============================================================================
// UMLS Query Functions
// =============================================================================

// UMLSQueries provides query functions for UMLS data.
type UMLSQueries struct {
	db *sql.DB
}

// NewUMLSQueries creates a new UMLS query helper.
func NewUMLSQueries(db *sql.DB) *UMLSQueries {
	return &UMLSQueries{db: db}
}

// UMLSConcept represents a row from umls_concepts.
type UMLSConcept struct {
	ID        int64
	CUI       string
	LAT       string
	AUI       string
	SAB       string // Source vocabulary (SNOMEDCT_US, ICD10CM, etc.)
	TTY       string // Term type (PT, SY, etc.)
	Code      string // Source code
	Str       string // Term/name
	IsPref    string // Is preferred
	Suppress  string
	ReleaseID int64
}

// CrosswalkResult represents a mapping between two codes via UMLS CUI.
type CrosswalkResult struct {
	SourceSAB   string
	SourceCode  string
	SourceTerm  string
	TargetSAB   string
	TargetCode  string
	TargetTerm  string
	CUI         string // The bridging CUI
	Equivalence string // equivalent, broader, narrower
}

// LookupCode finds UMLS concepts matching a source code.
func (q *UMLSQueries) LookupCode(ctx context.Context, sab, code string) ([]*UMLSConcept, error) {
	query := `
		SELECT id, cui, lat, aui, sab, tty, code, str, ispref, suppress, release_id
		FROM terminology.umls_concepts
		WHERE sab = $1 AND code = $2 AND suppress = 'N'
		ORDER BY
			CASE WHEN ispref = 'Y' THEN 0 ELSE 1 END,
			CASE WHEN ts = 'P' THEN 0 ELSE 1 END
	`

	rows, err := q.db.QueryContext(ctx, query, sab, code)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var concepts []*UMLSConcept
	for rows.Next() {
		c := &UMLSConcept{}
		if err := rows.Scan(&c.ID, &c.CUI, &c.LAT, &c.AUI, &c.SAB, &c.TTY, &c.Code, &c.Str, &c.IsPref, &c.Suppress, &c.ReleaseID); err != nil {
			return nil, err
		}
		concepts = append(concepts, c)
	}

	return concepts, rows.Err()
}

// GetByCUI retrieves all concepts sharing a CUI.
func (q *UMLSQueries) GetByCUI(ctx context.Context, cui string) ([]*UMLSConcept, error) {
	query := `
		SELECT id, cui, lat, aui, sab, tty, code, str, ispref, suppress, release_id
		FROM terminology.umls_concepts
		WHERE cui = $1 AND suppress = 'N'
		ORDER BY sab,
			CASE WHEN ispref = 'Y' THEN 0 ELSE 1 END
	`

	rows, err := q.db.QueryContext(ctx, query, cui)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var concepts []*UMLSConcept
	for rows.Next() {
		c := &UMLSConcept{}
		if err := rows.Scan(&c.ID, &c.CUI, &c.LAT, &c.AUI, &c.SAB, &c.TTY, &c.Code, &c.Str, &c.IsPref, &c.Suppress, &c.ReleaseID); err != nil {
			return nil, err
		}
		concepts = append(concepts, c)
	}

	return concepts, rows.Err()
}

// Crosswalk maps a code from one vocabulary to another via UMLS CUI.
// This is the primary cross-terminology mapping function.
func (q *UMLSQueries) Crosswalk(ctx context.Context, fromSAB, fromCode, toSAB string) ([]*CrosswalkResult, error) {
	query := `
		WITH source_cui AS (
			SELECT DISTINCT cui
			FROM terminology.umls_concepts
			WHERE sab = $1 AND code = $2 AND suppress = 'N'
		)
		SELECT
			src.sab as source_sab,
			src.code as source_code,
			src.str as source_term,
			tgt.sab as target_sab,
			tgt.code as target_code,
			tgt.str as target_term,
			src.cui
		FROM terminology.umls_concepts src
		JOIN source_cui sc ON src.cui = sc.cui
		JOIN terminology.umls_concepts tgt ON src.cui = tgt.cui
		WHERE src.sab = $1 AND src.code = $2
		  AND tgt.sab = $3 AND tgt.suppress = 'N'
		  AND src.suppress = 'N'
		  AND tgt.ispref = 'Y'
		ORDER BY
			CASE WHEN tgt.ts = 'P' THEN 0 ELSE 1 END,
			tgt.code
	`

	rows, err := q.db.QueryContext(ctx, query, fromSAB, fromCode, toSAB)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var results []*CrosswalkResult
	seen := make(map[string]bool) // Dedup by target code

	for rows.Next() {
		r := &CrosswalkResult{Equivalence: "equivalent"}
		if err := rows.Scan(&r.SourceSAB, &r.SourceCode, &r.SourceTerm, &r.TargetSAB, &r.TargetCode, &r.TargetTerm, &r.CUI); err != nil {
			return nil, err
		}
		if !seen[r.TargetCode] {
			results = append(results, r)
			seen[r.TargetCode] = true
		}
	}

	return results, rows.Err()
}

// SearchByTerm searches for concepts containing a term.
func (q *UMLSQueries) SearchByTerm(ctx context.Context, term string, sab string, limit int) ([]*UMLSConcept, error) {
	var query string
	var args []interface{}

	// Use ILIKE for case-insensitive contains search
	searchPattern := "%" + strings.ToLower(term) + "%"

	if sab != "" {
		query = `
			SELECT id, cui, lat, aui, sab, tty, code, str, ispref, suppress, release_id
			FROM terminology.umls_concepts
			WHERE sab = $1 AND LOWER(str) LIKE $2 AND suppress = 'N'
			ORDER BY
				CASE WHEN ispref = 'Y' THEN 0 ELSE 1 END,
				LENGTH(str)
			LIMIT $3
		`
		args = []interface{}{sab, searchPattern, limit}
	} else {
		query = `
			SELECT id, cui, lat, aui, sab, tty, code, str, ispref, suppress, release_id
			FROM terminology.umls_concepts
			WHERE LOWER(str) LIKE $1 AND suppress = 'N'
			ORDER BY
				CASE WHEN ispref = 'Y' THEN 0 ELSE 1 END,
				LENGTH(str)
			LIMIT $2
		`
		args = []interface{}{searchPattern, limit}
	}

	rows, err := q.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var concepts []*UMLSConcept
	for rows.Next() {
		c := &UMLSConcept{}
		if err := rows.Scan(&c.ID, &c.CUI, &c.LAT, &c.AUI, &c.SAB, &c.TTY, &c.Code, &c.Str, &c.IsPref, &c.Suppress, &c.ReleaseID); err != nil {
			return nil, err
		}
		concepts = append(concepts, c)
	}

	return concepts, rows.Err()
}

// GetRelations retrieves relationships for a concept.
func (q *UMLSQueries) GetRelations(ctx context.Context, cui string, relType string) ([]map[string]string, error) {
	query := `
		SELECT r.cui1, r.rel, r.cui2, r.rela, r.sab,
		       c.str as target_name, c.code as target_code
		FROM terminology.umls_relations r
		LEFT JOIN terminology.umls_concepts c ON r.cui2 = c.cui AND c.ispref = 'Y' AND c.suppress = 'N'
		WHERE r.cui1 = $1 AND r.suppress = 'N'
	`
	args := []interface{}{cui}

	if relType != "" {
		query += " AND r.rel = $2"
		args = append(args, relType)
	}

	query += " ORDER BY r.rel, c.str LIMIT 100"

	rows, err := q.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var results []map[string]string
	for rows.Next() {
		var cui1, rel, cui2, sab string
		var rela, targetName, targetCode sql.NullString

		if err := rows.Scan(&cui1, &rel, &cui2, &rela, &sab, &targetName, &targetCode); err != nil {
			return nil, err
		}

		result := map[string]string{
			"cui1":        cui1,
			"rel":         rel,
			"cui2":        cui2,
			"sab":         sab,
			"rela":        rela.String,
			"target_name": targetName.String,
			"target_code": targetCode.String,
		}
		results = append(results, result)
	}

	return results, rows.Err()
}

// GetSemanticTypes retrieves semantic types for a CUI.
func (q *UMLSQueries) GetSemanticTypes(ctx context.Context, cui string) ([]string, error) {
	query := `SELECT DISTINCT sty FROM terminology.umls_semantic_types WHERE cui = $1 ORDER BY sty`

	rows, err := q.db.QueryContext(ctx, query, cui)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var types []string
	for rows.Next() {
		var sty string
		if err := rows.Scan(&sty); err != nil {
			return nil, err
		}
		types = append(types, sty)
	}

	return types, rows.Err()
}

// Count returns the total number of UMLS concepts.
func (q *UMLSQueries) Count(ctx context.Context) (int64, error) {
	var count int64
	err := q.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM terminology.umls_concepts WHERE suppress = 'N'").Scan(&count)
	return count, err
}

// CountBySource returns concept counts by source vocabulary.
func (q *UMLSQueries) CountBySource(ctx context.Context) (map[string]int64, error) {
	query := `
		SELECT sab, COUNT(*)
		FROM terminology.umls_concepts
		WHERE suppress = 'N'
		GROUP BY sab
		ORDER BY COUNT(*) DESC
	`

	rows, err := q.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	counts := make(map[string]int64)
	for rows.Next() {
		var sab string
		var count int64
		if err := rows.Scan(&sab, &count); err != nil {
			return nil, err
		}
		counts[sab] = count
	}

	return counts, rows.Err()
}
