package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"
)

// RxNormLoader handles loading RxNorm RRF files into PostgreSQL.
type RxNormLoader struct {
	db       *sql.DB
	migrator *Migrator
}

// NewRxNormLoader creates a new RxNorm loader.
func NewRxNormLoader(db *sql.DB) *RxNormLoader {
	return &RxNormLoader{
		db:       db,
		migrator: NewMigrator(db),
	}
}

// RxNormLoadResult contains statistics from an RxNorm load operation.
type RxNormLoadResult struct {
	ReleaseID       int
	Version         string
	ConceptsLoaded  int64 // RXNCONSO rows
	RelationsLoaded int64 // RXNREL rows
	NDCLoaded       int64 // NDC cross-references
	Duration        time.Duration
}

// RxNormLoadOptions configures the RxNorm load process.
type RxNormLoadOptions struct {
	// SkipSuppressed skips rows with SUPPRESS != 'N'.
	SkipSuppressed bool

	// SkipRelations skips loading RXNREL.
	SkipRelations bool

	// LoadNDC controls whether to extract NDC cross-references.
	LoadNDC bool

	// FilterTTY limits to specific term types (SCD, SBD, IN, BN, etc.).
	FilterTTY []string
}

// RXNCONSO column indices.
const (
	RXNCONSOColRXCUI    = 0  // RxNorm CUI
	RXNCONSOColLAT      = 1  // Language
	RXNCONSOColTS       = 2  // Term Status
	RXNCONSOColLUI      = 3  // LUI
	RXNCONSOColSTT      = 4  // String Type
	RXNCONSOColSUI      = 5  // SUI
	RXNCONSOColISPREF   = 6  // Is Preferred
	RXNCONSOColRXAUI    = 7  // RxNorm AUI
	RXNCONSOColSAUI     = 8  // Source AUI
	RXNCONSOColSCUI     = 9  // Source CUI
	RXNCONSOColSDUI     = 10 // Source DUI
	RXNCONSOColSAB      = 11 // Source Abbreviation
	RXNCONSOColTTY      = 12 // Term Type
	RXNCONSOColCODE     = 13 // Code
	RXNCONSOColSTR      = 14 // String
	RXNCONSOColSRL      = 15 // Source Restriction Level
	RXNCONSOColSUPPRESS = 16 // Suppress
	RXNCONSOColCVF      = 17 // CVF
	RXNCONSOColumns     = 18
)

// RXNREL column indices.
const (
	RXNRELColRXCUI1   = 0  // RxCUI 1
	RXNRELColRXAUI1   = 1  // RXAUI 1
	RXNRELColSTYPE1   = 2  // STYPE1
	RXNRELColREL      = 3  // REL
	RXNRELColRXCUI2   = 4  // RxCUI 2
	RXNRELColRXAUI2   = 5  // RXAUI 2
	RXNRELColSTYPE2   = 6  // STYPE2
	RXNRELColRELA     = 7  // RELA
	RXNRELColRUI      = 8  // RUI
	RXNRELColSRUI     = 9  // SRUI
	RXNRELColSAB      = 10 // SAB
	RXNRELColSL       = 11 // SL
	RXNRELColRG       = 12 // RG
	RXNRELColDIR      = 13 // DIR
	RXNRELColSUPPRESS = 14 // SUPPRESS
	RXNRELColCVF      = 15 // CVF
	RXNRELColumns     = 16
)

// RXNSAT column indices (for NDC extraction).
const (
	RXNSATColRXCUI    = 0  // RxCUI
	RXNSATColLUI      = 1  // LUI
	RXNSATColSUI      = 2  // SUI
	RXNSATColRXAUI    = 3  // RXAUI
	RXNSATColSTYPE    = 4  // STYPE
	RXNSATColCODE     = 5  // CODE
	RXNSATColATUI     = 6  // ATUI
	RXNSATColSATUI    = 7  // SATUI
	RXNSATColATN      = 8  // Attribute name
	RXNSATColSAB      = 9  // SAB
	RXNSATColATV      = 10 // Attribute value
	RXNSATColSUPPRESS = 11 // SUPPRESS
	RXNSATColCVF      = 12 // CVF
	RXNSATColumns     = 13
)

// LoadRRF loads RxNorm from an RRF directory.
func (l *RxNormLoader) LoadRRF(ctx context.Context, rrfDir, version string, releaseDate *time.Time, opts *RxNormLoadOptions, progress ProgressReporter) (*RxNormLoadResult, error) {
	start := time.Now()
	result := &RxNormLoadResult{Version: version}

	if opts == nil {
		opts = &RxNormLoadOptions{
			SkipSuppressed: true,
			LoadNDC:        true,
		}
	}

	// Validate directory
	if err := validateRxNormDirectory(rrfDir); err != nil {
		return nil, fmt.Errorf("invalid RxNorm directory: %w", err)
	}

	// Create or get release
	releaseID, err := l.migrator.CreateRelease(ctx, VocabRxNorm, version, releaseDate)
	if err != nil {
		return nil, fmt.Errorf("create release: %w", err)
	}
	result.ReleaseID = releaseID

	// Clear existing data
	if err := l.clearReleaseData(ctx, releaseID); err != nil {
		return nil, fmt.Errorf("clear existing data: %w", err)
	}

	// Load RXNCONSO
	consoPath := findRRFFile(rrfDir, "RXNCONSO.RRF")
	if progress != nil {
		progress(LoadProgress{Vocabulary: VocabRxNorm, Version: version, Phase: "Counting RXNCONSO"})
	}

	consoRows, err := CountRRFRows(consoPath)
	if err != nil {
		return nil, fmt.Errorf("count RXNCONSO: %w", err)
	}

	if progress != nil {
		progress(LoadProgress{Vocabulary: VocabRxNorm, Version: version, Phase: "Loading RXNCONSO", RowsTotal: consoRows})
	}

	loaded, err := l.loadRXNCONSO(ctx, consoPath, releaseID, opts, progress, consoRows)
	if err != nil {
		return nil, fmt.Errorf("load RXNCONSO: %w", err)
	}
	result.ConceptsLoaded = loaded

	// Load RXNREL
	if !opts.SkipRelations {
		relPath := findRRFFile(rrfDir, "RXNREL.RRF")
		if progress != nil {
			progress(LoadProgress{Vocabulary: VocabRxNorm, Version: version, Phase: "Counting RXNREL"})
		}

		relRows, err := CountRRFRows(relPath)
		if err != nil {
			return nil, fmt.Errorf("count RXNREL: %w", err)
		}

		if progress != nil {
			progress(LoadProgress{Vocabulary: VocabRxNorm, Version: version, Phase: "Loading RXNREL", RowsTotal: relRows})
		}

		loaded, err = l.loadRXNREL(ctx, relPath, releaseID, opts, progress, relRows)
		if err != nil {
			return nil, fmt.Errorf("load RXNREL: %w", err)
		}
		result.RelationsLoaded = loaded
	}

	// Extract NDC cross-references from RXNSAT
	if opts.LoadNDC {
		satPath := findRRFFile(rrfDir, "RXNSAT.RRF")
		if fileExists(satPath) {
			if progress != nil {
				progress(LoadProgress{Vocabulary: VocabRxNorm, Version: version, Phase: "Loading NDC"})
			}

			loaded, err = l.loadNDCFromRXNSAT(ctx, satPath, releaseID, progress)
			if err != nil {
				return nil, fmt.Errorf("load NDC: %w", err)
			}
			result.NDCLoaded = loaded
		}
	}

	// Update release stats
	totalRows := result.ConceptsLoaded + result.RelationsLoaded + result.NDCLoaded
	if err := l.migrator.UpdateReleaseRowCount(ctx, releaseID, totalRows); err != nil {
		return nil, fmt.Errorf("update release stats: %w", err)
	}

	// Set as active release
	if err := l.migrator.SetActiveRelease(ctx, VocabRxNorm, version); err != nil {
		return nil, fmt.Errorf("set active release: %w", err)
	}

	result.Duration = time.Since(start)
	return result, nil
}

// loadRXNCONSO loads the RXNCONSO.RRF file.
func (l *RxNormLoader) loadRXNCONSO(ctx context.Context, path string, releaseID int, opts *RxNormLoadOptions, progress ProgressReporter, totalRows int64) (int64, error) {
	reader, err := OpenRRFFile(path, RXNCONSOColumns)
	if err != nil {
		return 0, err
	}
	defer func() { _ = reader.Close() }()
	reader.SetTotalLines(totalRows)

	// Build TTY filter set
	ttyFilter := make(map[string]bool)
	for _, tty := range opts.FilterTTY {
		ttyFilter[tty] = true
	}
	hasFilter := len(ttyFilter) > 0

	tx, err := l.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	inserter := NewBatchInserter(tx, "terminology.rxnorm_concepts",
		[]string{"rxcui", "lat", "ts", "lui", "stt", "sui", "rxaui", "saui", "scui", "sdui", "sab", "tty", "code", "str", "suppress", "cvf", "release_id"},
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

		tty := RRFField(row, RXNCONSOColTTY)
		suppress := RRFField(row, RXNCONSOColSUPPRESS)

		if hasFilter && !ttyFilter[tty] {
			skipped++
			continue
		}
		if opts.SkipSuppressed && suppress != "N" {
			skipped++
			continue
		}

		if err := inserter.Add(ctx,
			RRFField(row, RXNCONSOColRXCUI),
			RRFField(row, RXNCONSOColLAT),
			nullableString(RRFField(row, RXNCONSOColTS)),
			nullableString(RRFField(row, RXNCONSOColLUI)),
			nullableString(RRFField(row, RXNCONSOColSTT)),
			nullableString(RRFField(row, RXNCONSOColSUI)),
			RRFField(row, RXNCONSOColRXAUI),
			nullableString(RRFField(row, RXNCONSOColSAUI)),
			nullableString(RRFField(row, RXNCONSOColSCUI)),
			nullableString(RRFField(row, RXNCONSOColSDUI)),
			RRFField(row, RXNCONSOColSAB),
			tty,
			RRFField(row, RXNCONSOColCODE),
			RRFField(row, RXNCONSOColSTR),
			suppress,
			nullableInt(RRFFieldInt(row, RXNCONSOColCVF)),
			releaseID,
		); err != nil {
			return 0, fmt.Errorf("insert row %d: %w", reader.LineNum(), err)
		}
		loaded++

		if progress != nil && time.Since(lastProgress) > 2*time.Second {
			progress(LoadProgress{
				Vocabulary:  VocabRxNorm,
				Phase:       "Loading RXNCONSO",
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

// loadRXNREL loads the RXNREL.RRF file.
func (l *RxNormLoader) loadRXNREL(ctx context.Context, path string, releaseID int, opts *RxNormLoadOptions, progress ProgressReporter, totalRows int64) (int64, error) {
	reader, err := OpenRRFFile(path, RXNRELColumns)
	if err != nil {
		return 0, err
	}
	defer func() { _ = reader.Close() }()
	reader.SetTotalLines(totalRows)

	tx, err := l.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	inserter := NewBatchInserter(tx, "terminology.rxnorm_relations",
		[]string{"rxcui1", "rxaui1", "stype1", "rel", "rxcui2", "rxaui2", "stype2", "rela", "rui", "srui", "sab", "sl", "rg", "dir", "suppress", "release_id"},
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

		suppress := RRFField(row, RXNRELColSUPPRESS)
		if opts.SkipSuppressed && suppress != "N" {
			skipped++
			continue
		}

		if err := inserter.Add(ctx,
			RRFField(row, RXNRELColRXCUI1),
			nullableString(RRFField(row, RXNRELColRXAUI1)),
			nullableString(RRFField(row, RXNRELColSTYPE1)),
			RRFField(row, RXNRELColREL),
			RRFField(row, RXNRELColRXCUI2),
			nullableString(RRFField(row, RXNRELColRXAUI2)),
			nullableString(RRFField(row, RXNRELColSTYPE2)),
			nullableString(RRFField(row, RXNRELColRELA)),
			nullableString(RRFField(row, RXNRELColRUI)),
			nullableString(RRFField(row, RXNRELColSRUI)),
			RRFField(row, RXNRELColSAB),
			nullableString(RRFField(row, RXNRELColSL)),
			nullableString(RRFField(row, RXNRELColRG)),
			nullableString(RRFField(row, RXNRELColDIR)),
			suppress,
			releaseID,
		); err != nil {
			return 0, fmt.Errorf("insert row %d: %w", reader.LineNum(), err)
		}
		loaded++

		if progress != nil && time.Since(lastProgress) > 2*time.Second {
			progress(LoadProgress{
				Vocabulary:  VocabRxNorm,
				Phase:       "Loading RXNREL",
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

// loadNDCFromRXNSAT extracts NDC-RXCUI mappings from RXNSAT.RRF.
// NDC codes are stored as attributes with ATN='NDC'.
func (l *RxNormLoader) loadNDCFromRXNSAT(ctx context.Context, path string, releaseID int, progress ProgressReporter) (int64, error) {
	reader, err := OpenRRFFile(path, RXNSATColumns)
	if err != nil {
		return 0, err
	}
	defer func() { _ = reader.Close() }()

	tx, err := l.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	inserter := NewBatchInserter(tx, "terminology.rxnorm_ndc_xref",
		[]string{"ndc", "ndc_formatted", "rxcui", "release_id"},
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

		// Only process NDC attributes
		atn := RRFField(row, RXNSATColATN)
		if atn != "NDC" {
			continue
		}

		rxcui := RRFField(row, RXNSATColRXCUI)
		ndc := RRFField(row, RXNSATColATV) // NDC is in the attribute value

		// Normalize NDC (remove dashes, spaces)
		ndcClean := strings.ReplaceAll(ndc, "-", "")
		ndcClean = strings.ReplaceAll(ndcClean, " ", "")

		// Format as 5-4-2
		ndcFormatted := formatNDC(ndcClean)

		if err := inserter.Add(ctx,
			ndcClean,
			ndcFormatted,
			rxcui,
			releaseID,
		); err != nil {
			// Skip duplicate NDC-RXCUI pairs
			if !strings.Contains(err.Error(), "duplicate") {
				return 0, fmt.Errorf("insert NDC: %w", err)
			}
		}
		loaded++

		if progress != nil && time.Since(lastProgress) > 2*time.Second {
			progress(LoadProgress{
				Vocabulary: VocabRxNorm,
				Phase:      "Loading NDC",
				RowsLoaded: loaded,
			})
			lastProgress = time.Now()
		}
	}

	if err := inserter.Flush(ctx); err != nil {
		// Ignore duplicate errors on flush
		if !strings.Contains(err.Error(), "duplicate") {
			return 0, fmt.Errorf("flush remaining: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}

	return loaded, nil
}

// formatNDC formats an 11-digit NDC as 5-4-2 format.
func formatNDC(ndc string) string {
	if len(ndc) != 11 {
		return ndc
	}
	return ndc[0:5] + "-" + ndc[5:9] + "-" + ndc[9:11]
}

// clearReleaseData removes existing data for a release.
func (l *RxNormLoader) clearReleaseData(ctx context.Context, releaseID int) error {
	tables := []string{
		"terminology.rxnorm_concepts",
		"terminology.rxnorm_relations",
		"terminology.rxnorm_ndc_xref",
	}

	for _, table := range tables {
		_, err := l.db.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s WHERE release_id = $1", table), releaseID)
		if err != nil {
			return fmt.Errorf("clear %s: %w", table, err)
		}
	}

	return nil
}

// validateRxNormDirectory checks if a directory contains valid RxNorm RRF files.
func validateRxNormDirectory(path string) error {
	requiredFile := "RXNCONSO.RRF"
	fullPath := filepath.Join(path, requiredFile)

	if !fileExists(fullPath) && !fileExists(fullPath+".gz") {
		return fmt.Errorf("required file not found: %s", requiredFile)
	}

	return nil
}

// =============================================================================
// RxNorm Query Functions
// =============================================================================

// RxNormQueries provides query functions for RxNorm data.
type RxNormQueries struct {
	db *sql.DB
}

// NewRxNormQueries creates a new RxNorm query helper.
func NewRxNormQueries(db *sql.DB) *RxNormQueries {
	return &RxNormQueries{db: db}
}

// RxNormConcept represents a row from rxnorm_concepts.
type RxNormConcept struct {
	ID        int64
	RXCUI     string
	RXAUI     string
	SAB       string // Source (RXNORM, MTHSPL, etc.)
	TTY       string // Term type (SCD, SBD, IN, BN, etc.)
	Code      string
	Str       string // Drug name
	Suppress  string
	ReleaseID int64
}

// GetByRXCUI retrieves concepts by RxNorm CUI.
func (q *RxNormQueries) GetByRXCUI(ctx context.Context, rxcui string) ([]*RxNormConcept, error) {
	query := `
		SELECT id, rxcui, rxaui, sab, tty, code, str, suppress, release_id
		FROM terminology.rxnorm_concepts
		WHERE rxcui = $1 AND suppress = 'N'
		ORDER BY
			CASE tty
				WHEN 'SCD' THEN 1
				WHEN 'SBD' THEN 2
				WHEN 'GPCK' THEN 3
				WHEN 'BPCK' THEN 4
				WHEN 'IN' THEN 5
				WHEN 'BN' THEN 6
				ELSE 10
			END
	`

	rows, err := q.db.QueryContext(ctx, query, rxcui)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var concepts []*RxNormConcept
	for rows.Next() {
		c := &RxNormConcept{}
		if err := rows.Scan(&c.ID, &c.RXCUI, &c.RXAUI, &c.SAB, &c.TTY, &c.Code, &c.Str, &c.Suppress, &c.ReleaseID); err != nil {
			return nil, err
		}
		concepts = append(concepts, c)
	}

	return concepts, rows.Err()
}

// LookupNDC finds RxNorm concepts for an NDC code.
func (q *RxNormQueries) LookupNDC(ctx context.Context, ndc string) ([]*RxNormConcept, error) {
	// Normalize NDC
	ndcClean := strings.ReplaceAll(ndc, "-", "")
	ndcClean = strings.ReplaceAll(ndcClean, " ", "")

	query := `
		SELECT c.id, c.rxcui, c.rxaui, c.sab, c.tty, c.code, c.str, c.suppress, c.release_id
		FROM terminology.rxnorm_ndc_xref x
		JOIN terminology.rxnorm_concepts c ON x.rxcui = c.rxcui AND c.release_id = x.release_id
		WHERE x.ndc = $1 AND c.suppress = 'N' AND c.tty IN ('SCD', 'SBD', 'GPCK', 'BPCK')
		ORDER BY c.tty
	`

	rows, err := q.db.QueryContext(ctx, query, ndcClean)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var concepts []*RxNormConcept
	for rows.Next() {
		c := &RxNormConcept{}
		if err := rows.Scan(&c.ID, &c.RXCUI, &c.RXAUI, &c.SAB, &c.TTY, &c.Code, &c.Str, &c.Suppress, &c.ReleaseID); err != nil {
			return nil, err
		}
		concepts = append(concepts, c)
	}

	return concepts, rows.Err()
}

// SearchByName searches RxNorm concepts by drug name.
func (q *RxNormQueries) SearchByName(ctx context.Context, name string, limit int) ([]*RxNormConcept, error) {
	searchPattern := "%" + strings.ToLower(name) + "%"

	query := `
		SELECT id, rxcui, rxaui, sab, tty, code, str, suppress, release_id
		FROM terminology.rxnorm_concepts
		WHERE LOWER(str) LIKE $1 AND suppress = 'N' AND sab = 'RXNORM'
		ORDER BY
			CASE tty
				WHEN 'IN' THEN 1
				WHEN 'BN' THEN 2
				WHEN 'SCD' THEN 3
				WHEN 'SBD' THEN 4
				ELSE 10
			END,
			LENGTH(str)
		LIMIT $2
	`

	rows, err := q.db.QueryContext(ctx, query, searchPattern, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var concepts []*RxNormConcept
	for rows.Next() {
		c := &RxNormConcept{}
		if err := rows.Scan(&c.ID, &c.RXCUI, &c.RXAUI, &c.SAB, &c.TTY, &c.Code, &c.Str, &c.Suppress, &c.ReleaseID); err != nil {
			return nil, err
		}
		concepts = append(concepts, c)
	}

	return concepts, rows.Err()
}

// GetIngredients returns ingredients for an SCD/SBD drug.
func (q *RxNormQueries) GetIngredients(ctx context.Context, rxcui string) ([]*RxNormConcept, error) {
	query := `
		SELECT c.id, c.rxcui, c.rxaui, c.sab, c.tty, c.code, c.str, c.suppress, c.release_id
		FROM terminology.rxnorm_relations r
		JOIN terminology.rxnorm_concepts c ON r.rxcui2 = c.rxcui AND c.release_id = r.release_id
		WHERE r.rxcui1 = $1 AND r.rela = 'has_ingredient' AND c.suppress = 'N' AND c.tty = 'IN'
		ORDER BY c.str
	`

	rows, err := q.db.QueryContext(ctx, query, rxcui)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var ingredients []*RxNormConcept
	for rows.Next() {
		c := &RxNormConcept{}
		if err := rows.Scan(&c.ID, &c.RXCUI, &c.RXAUI, &c.SAB, &c.TTY, &c.Code, &c.Str, &c.Suppress, &c.ReleaseID); err != nil {
			return nil, err
		}
		ingredients = append(ingredients, c)
	}

	return ingredients, rows.Err()
}

// GetBrandNames returns brand names for a generic ingredient.
func (q *RxNormQueries) GetBrandNames(ctx context.Context, ingredientRXCUI string) ([]*RxNormConcept, error) {
	query := `
		SELECT DISTINCT c.id, c.rxcui, c.rxaui, c.sab, c.tty, c.code, c.str, c.suppress, c.release_id
		FROM terminology.rxnorm_relations r
		JOIN terminology.rxnorm_concepts c ON r.rxcui1 = c.rxcui AND c.release_id = r.release_id
		WHERE r.rxcui2 = $1 AND r.rela = 'tradename_of' AND c.suppress = 'N' AND c.tty = 'BN'
		ORDER BY c.str
	`

	rows, err := q.db.QueryContext(ctx, query, ingredientRXCUI)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var brands []*RxNormConcept
	for rows.Next() {
		c := &RxNormConcept{}
		if err := rows.Scan(&c.ID, &c.RXCUI, &c.RXAUI, &c.SAB, &c.TTY, &c.Code, &c.Str, &c.Suppress, &c.ReleaseID); err != nil {
			return nil, err
		}
		brands = append(brands, c)
	}

	return brands, rows.Err()
}

// GetNDCs returns all NDCs for an RXCUI.
func (q *RxNormQueries) GetNDCs(ctx context.Context, rxcui string) ([]string, error) {
	query := `
		SELECT ndc_formatted FROM terminology.rxnorm_ndc_xref
		WHERE rxcui = $1
		ORDER BY ndc
	`

	rows, err := q.db.QueryContext(ctx, query, rxcui)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var ndcs []string
	for rows.Next() {
		var ndc sql.NullString
		if err := rows.Scan(&ndc); err != nil {
			return nil, err
		}
		if ndc.Valid {
			ndcs = append(ndcs, ndc.String)
		}
	}

	return ndcs, rows.Err()
}

// Count returns the total number of RxNorm concepts.
func (q *RxNormQueries) Count(ctx context.Context) (int64, error) {
	var count int64
	err := q.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM terminology.rxnorm_concepts WHERE suppress = 'N'").Scan(&count)
	return count, err
}

// CountNDC returns the number of NDC cross-references.
func (q *RxNormQueries) CountNDC(ctx context.Context) (int64, error) {
	var count int64
	err := q.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM terminology.rxnorm_ndc_xref").Scan(&count)
	return count, err
}
