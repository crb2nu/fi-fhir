// Package sink provides data destination handlers for the ETL pipeline.
package sink

import (
	"archive/zip"
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/crb2nu/fi-fhir/pkg/storage"
	"github.com/crb2nu/fi-fhir/pkg/terminology/db"
)

// PostgresSink loads terminology data into PostgreSQL.
// It downloads from a storage provider to a temp directory,
// then uses the appropriate loader (UMLS, RxNorm, LOINC).
type PostgresSink struct {
	name     string
	database *sql.DB
	provider storage.Provider
	tempDir  string
}

// PostgresSinkConfig configures the PostgreSQL sink.
type PostgresSinkConfig struct {
	Name     string
	DB       *sql.DB
	Provider storage.Provider // MinIO or local provider for downloading
	TempDir  string           // Optional temp directory (default: os.TempDir())
}

// NewPostgresSink creates a new PostgreSQL sink.
func NewPostgresSink(cfg PostgresSinkConfig) *PostgresSink {
	tempDir := cfg.TempDir
	if tempDir == "" {
		tempDir = os.TempDir()
	}

	return &PostgresSink{
		name:     cfg.Name,
		database: cfg.DB,
		provider: cfg.Provider,
		tempDir:  tempDir,
	}
}

// Name returns the sink identifier.
func (s *PostgresSink) Name() string {
	return s.name
}

// LoadResult contains the results of a load operation.
type LoadResult struct {
	Source     string
	Version    string
	RowsLoaded int64
	Duration   time.Duration
	ReleaseID  int
	Details    map[string]int64 // Per-table counts
}

// LoadUMLS loads UMLS data from storage into PostgreSQL.
func (s *PostgresSink) LoadUMLS(ctx context.Context, storagePath, version string, opts *db.UMLSLoadOptions, progress db.ProgressReporter) (*LoadResult, error) {
	start := time.Now()

	// Download to temp directory
	localDir, cleanup, err := s.downloadAndExtract(ctx, storagePath, "umls-"+version)
	if err != nil {
		return nil, fmt.Errorf("failed to download UMLS: %w", err)
	}
	defer cleanup()

	// Find the META directory
	metaDir, err := findSubdir(localDir, "META")
	if err != nil {
		// Maybe the files are directly in the downloaded directory
		metaDir = localDir
	}

	// Create loader and load
	loader := db.NewUMLSLoader(s.database)
	result, err := loader.LoadMETA(ctx, metaDir, version, nil, opts, progress)
	if err != nil {
		return nil, fmt.Errorf("failed to load UMLS: %w", err)
	}

	return &LoadResult{
		Source:     "umls",
		Version:    version,
		RowsLoaded: result.ConceptsLoaded + result.RelationsLoaded + result.SemTypesLoaded,
		Duration:   time.Since(start),
		ReleaseID:  result.ReleaseID,
		Details: map[string]int64{
			"concepts":  result.ConceptsLoaded,
			"relations": result.RelationsLoaded,
			"semtypes":  result.SemTypesLoaded,
		},
	}, nil
}

// LoadRxNorm loads RxNorm data from storage into PostgreSQL.
func (s *PostgresSink) LoadRxNorm(ctx context.Context, storagePath, version string, opts *db.RxNormLoadOptions, progress db.ProgressReporter) (*LoadResult, error) {
	start := time.Now()

	// Download to temp directory
	localDir, cleanup, err := s.downloadAndExtract(ctx, storagePath, "rxnorm-"+version)
	if err != nil {
		return nil, fmt.Errorf("failed to download RxNorm: %w", err)
	}
	defer cleanup()

	// Find the rrf directory
	rrfDir, err := findSubdir(localDir, "rrf")
	if err != nil {
		rrfDir = localDir
	}

	// Create loader and load
	loader := db.NewRxNormLoader(s.database)
	result, err := loader.LoadRRF(ctx, rrfDir, version, nil, opts, progress)
	if err != nil {
		return nil, fmt.Errorf("failed to load RxNorm: %w", err)
	}

	return &LoadResult{
		Source:     "rxnorm",
		Version:    version,
		RowsLoaded: result.ConceptsLoaded + result.RelationsLoaded + result.NDCLoaded,
		Duration:   time.Since(start),
		ReleaseID:  result.ReleaseID,
		Details: map[string]int64{
			"concepts":  result.ConceptsLoaded,
			"relations": result.RelationsLoaded,
			"ndc":       result.NDCLoaded,
		},
	}, nil
}

// LoadLOINC loads LOINC data from storage into PostgreSQL.
func (s *PostgresSink) LoadLOINC(ctx context.Context, storagePath, version string, progress db.ProgressReporter) (*LoadResult, error) {
	start := time.Now()

	// Download to temp directory
	localDir, cleanup, err := s.downloadAndExtract(ctx, storagePath, "loinc-"+version)
	if err != nil {
		return nil, fmt.Errorf("failed to download LOINC: %w", err)
	}
	defer cleanup()

	// Find LoincTable.csv
	loincTable, err := findFile(localDir, "LoincTable.csv", "Loinc.csv")
	if err != nil {
		return nil, fmt.Errorf("failed to find LOINC table: %w", err)
	}

	// Create loader and load
	loader := db.NewLOINCLoader(s.database)
	result, err := loader.LoadLoincTable(ctx, loincTable, version, nil, progress)
	if err != nil {
		return nil, fmt.Errorf("failed to load LOINC: %w", err)
	}

	return &LoadResult{
		Source:     "loinc",
		Version:    version,
		RowsLoaded: result.CodesLoaded + result.PanelsLoaded,
		Duration:   time.Since(start),
		ReleaseID:  result.ReleaseID,
		Details: map[string]int64{
			"codes":  result.CodesLoaded,
			"panels": result.PanelsLoaded,
		},
	}, nil
}

// downloadAndExtract downloads files from storage and extracts if needed.
// Returns the local directory path and a cleanup function.
func (s *PostgresSink) downloadAndExtract(ctx context.Context, storagePath, prefix string) (string, func(), error) {
	// Create temp directory
	localDir, err := os.MkdirTemp(s.tempDir, prefix+"-*")
	if err != nil {
		return "", nil, fmt.Errorf("failed to create temp directory: %w", err)
	}

	cleanup := func() {
		_ = os.RemoveAll(localDir)
	}

	// List files in storage path recursively
	files, err := s.provider.ListRecursive(ctx, storagePath)
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("failed to list storage path: %w", err)
	}

	if len(files) == 0 {
		cleanup()
		return "", nil, fmt.Errorf("no files found at %s", storagePath)
	}

	// Download each file
	for _, file := range files {
		if file.IsDir {
			continue
		}

		// Open source file
		reader, err := s.provider.Open(ctx, file.Path)
		if err != nil {
			cleanup()
			return "", nil, fmt.Errorf("failed to open %s: %w", file.Path, err)
		}

		// Determine local filename (strip prefix)
		localName := filepath.Base(file.Path)
		localPath := filepath.Join(localDir, localName)

		// Create local file
		localFile, err := os.Create(localPath) //nolint:gosec // G304: controlled path
		if err != nil {
			_ = reader.Close()
			cleanup()
			return "", nil, fmt.Errorf("failed to create local file: %w", err)
		}

		// Copy content
		_, err = io.Copy(localFile, reader)
		_ = reader.Close()
		if cerr := localFile.Close(); cerr != nil && err == nil {
			err = cerr
		}
		if err != nil {
			cleanup()
			return "", nil, fmt.Errorf("failed to download %s: %w", file.Path, err)
		}

		// Extract if it's a zip file
		if strings.HasSuffix(localName, ".zip") {
			if err := extractZip(localPath, localDir); err != nil {
				cleanup()
				return "", nil, fmt.Errorf("failed to extract %s: %w", localName, err)
			}
			// Remove the zip after extraction
			_ = os.Remove(localPath)
		}
	}

	return localDir, cleanup, nil
}

// extractZip extracts a zip file to the target directory.
func extractZip(zipPath, targetDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer func() { _ = r.Close() }()

	for _, f := range r.File {
		// Sanitize path to prevent zip slip
		fpath := filepath.Join(targetDir, filepath.Clean(f.Name))
		if !strings.HasPrefix(fpath, filepath.Clean(targetDir)+string(os.PathSeparator)) {
			return fmt.Errorf("invalid file path in zip: %s", f.Name)
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(fpath, 0o750); err != nil {
				return err
			}
			continue
		}

		// Create parent directories
		if err := os.MkdirAll(filepath.Dir(fpath), 0o750); err != nil {
			return err
		}

		// Extract file
		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode()) //nolint:gosec // G304: controlled path
		if err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			_ = outFile.Close()
			return err
		}

		//nolint:gosec // G110: Trusted source - files come from our MinIO storage
		_, err = io.Copy(outFile, rc)
		_ = rc.Close()
		if cerr := outFile.Close(); cerr != nil && err == nil {
			err = cerr
		}
		if err != nil {
			return err
		}
	}

	return nil
}

// findSubdir searches for a subdirectory with the given name.
func findSubdir(root, name string) (string, error) {
	var found string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() && strings.EqualFold(info.Name(), name) {
			found = path
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if found == "" {
		return "", fmt.Errorf("directory %s not found", name)
	}
	return found, nil
}

// findFile searches for a file with one of the given names.
func findFile(root string, names ...string) (string, error) {
	nameSet := make(map[string]bool)
	for _, n := range names {
		nameSet[strings.ToLower(n)] = true
	}

	var found string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && nameSet[strings.ToLower(info.Name())] {
			found = path
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if found == "" {
		return "", fmt.Errorf("file %v not found", names)
	}
	return found, nil
}

// LoadICD10CM loads ICD-10-CM data from storage into PostgreSQL.
func (s *PostgresSink) LoadICD10CM(ctx context.Context, storagePath, version string, opts *db.ICD10LoadOptions, progress db.ProgressReporter) (*LoadResult, error) {
	start := time.Now()

	// Download to temp directory
	localDir, cleanup, err := s.downloadAndExtract(ctx, storagePath, "icd10cm-"+version)
	if err != nil {
		return nil, fmt.Errorf("failed to download ICD-10-CM: %w", err)
	}
	defer cleanup()

	// Find ICD-10-CM CSV file (try common names)
	csvFile, err := findFile(localDir, "icd10cm_codes.csv", "icd10cm_codes_sample.csv", "icd10cm.csv", "codes.csv")
	if err != nil {
		return nil, fmt.Errorf("failed to find ICD-10-CM file: %w", err)
	}

	// Default options if not provided
	if opts == nil {
		opts = &db.ICD10LoadOptions{IncludeHeaders: true}
	}

	// Create loader and load
	loader := db.NewICD10Loader(s.database)
	result, err := loader.LoadICD10CMCSV(ctx, csvFile, version, nil, progress, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to load ICD-10-CM: %w", err)
	}

	return &LoadResult{
		Source:     "icd10cm",
		Version:    version,
		RowsLoaded: result.CodesLoaded + result.HeadersLoaded,
		Duration:   time.Since(start),
		ReleaseID:  result.ReleaseID,
		Details: map[string]int64{
			"codes":   result.CodesLoaded,
			"headers": result.HeadersLoaded,
		},
	}, nil
}

// Write implements the Sink interface but is not used for PostgresSink.
// Use LoadUMLS, LoadRxNorm, LoadLOINC, or LoadICD10CM instead.
func (s *PostgresSink) Write(ctx context.Context, path string, r io.Reader, size int64) error {
	return fmt.Errorf("PostgresSink.Write not supported; use LoadUMLS, LoadRxNorm, LoadLOINC, or LoadICD10CM")
}

// Exists checks if data exists for the given source/version.
func (s *PostgresSink) Exists(ctx context.Context, path string) (bool, error) {
	// Parse path as source/version
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		return false, nil
	}

	source := parts[0]
	version := parts[1]

	// Check releases table
	var count int
	err := s.database.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM terminology.releases WHERE vocabulary = $1 AND version = $2",
		strings.ToUpper(source), version,
	).Scan(&count)
	if err != nil {
		return false, err
	}

	return count > 0, nil
}

// Validate checks if the database connection is valid.
func (s *PostgresSink) Validate(ctx context.Context) error {
	if err := s.database.PingContext(ctx); err != nil {
		return fmt.Errorf("database connection failed: %w", err)
	}

	// Check if terminology schema exists
	var schemaExists bool
	err := s.database.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM information_schema.schemata WHERE schema_name = 'terminology')",
	).Scan(&schemaExists)
	if err != nil {
		return fmt.Errorf("failed to check schema: %w", err)
	}

	if !schemaExists {
		return fmt.Errorf("terminology schema not found; run 'fi-fhir terminology init' first")
	}

	return nil
}
