// Package db provides database operations for terminology data.
package db

import (
	"compress/gzip"
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/storage"
)

// BulkLoader provides efficient bulk loading of terminology data into PostgreSQL.
// It uses COPY protocol when possible, falling back to batched inserts.
type BulkLoader struct {
	db        *sql.DB
	batchSize int
}

// NewBulkLoader creates a new bulk loader with the specified batch size.
func NewBulkLoader(db *sql.DB, batchSize int) *BulkLoader {
	if batchSize <= 0 {
		batchSize = 5000 // Default batch size
	}
	return &BulkLoader{
		db:        db,
		batchSize: batchSize,
	}
}

// BatchInserter handles batched INSERT operations for a specific table.
type BatchInserter struct {
	tx           *sql.Tx
	table        string
	columns      []string
	batchSize    int
	pendingRows  [][]interface{}
	totalInserts int64
	onProgress   func(inserted int64)
}

// NewBatchInserter creates a new batch inserter for the specified table.
func NewBatchInserter(tx *sql.Tx, table string, columns []string, batchSize int) *BatchInserter {
	if batchSize <= 0 {
		batchSize = 5000
	}
	return &BatchInserter{
		tx:          tx,
		table:       table,
		columns:     columns,
		batchSize:   batchSize,
		pendingRows: make([][]interface{}, 0, batchSize),
	}
}

// SetProgressCallback sets a callback to report progress during bulk inserts.
func (b *BatchInserter) SetProgressCallback(fn func(inserted int64)) {
	b.onProgress = fn
}

// Add queues a row for insertion. Automatically flushes when batch is full.
func (b *BatchInserter) Add(ctx context.Context, values ...interface{}) error {
	if len(values) != len(b.columns) {
		return fmt.Errorf("expected %d values, got %d", len(b.columns), len(values))
	}

	b.pendingRows = append(b.pendingRows, values)

	if len(b.pendingRows) >= b.batchSize {
		return b.Flush(ctx)
	}

	return nil
}

// Flush writes all pending rows to the database.
func (b *BatchInserter) Flush(ctx context.Context) error {
	if len(b.pendingRows) == 0 {
		return nil
	}

	// Build the INSERT statement with multiple value clauses
	// INSERT INTO table (col1, col2) VALUES ($1, $2), ($3, $4), ...
	var placeholders []string
	var args []interface{}

	colList := strings.Join(b.columns, ", ")
	paramNum := 1

	for _, row := range b.pendingRows {
		var rowPlaceholders []string
		for range row {
			rowPlaceholders = append(rowPlaceholders, fmt.Sprintf("$%d", paramNum))
			paramNum++
		}
		placeholders = append(placeholders, "("+strings.Join(rowPlaceholders, ", ")+")")
		args = append(args, row...)
	}

	//nolint:gosec // G201: table name from trusted internal code, not user input
	query := fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES %s",
		b.table,
		colList,
		strings.Join(placeholders, ", "),
	)

	_, err := b.tx.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("batch insert to %s failed: %w", b.table, err)
	}

	b.totalInserts += int64(len(b.pendingRows))
	if b.onProgress != nil {
		b.onProgress(b.totalInserts)
	}

	// Reset pending rows
	b.pendingRows = b.pendingRows[:0]

	return nil
}

// TotalInserted returns the total number of rows inserted.
func (b *BatchInserter) TotalInserted() int64 {
	return b.totalInserts + int64(len(b.pendingRows))
}

// Close flushes any remaining rows.
func (b *BatchInserter) Close(ctx context.Context) error {
	return b.Flush(ctx)
}

// LoadProgress tracks the progress of a terminology load operation.
type LoadProgress struct {
	Vocabulary  string
	Version     string
	Phase       string
	RowsTotal   int64
	RowsLoaded  int64
	RowsSkipped int64
	CurrentFile string
	ErrorCount  int
	LastError   string
}

// ProgressReporter is called periodically during load to report progress.
type ProgressReporter func(progress LoadProgress)

// DefaultProgressReporter prints progress to stdout.
func DefaultProgressReporter(w io.Writer) ProgressReporter {
	return func(p LoadProgress) {
		if p.RowsTotal > 0 {
			pct := float64(p.RowsLoaded) / float64(p.RowsTotal) * 100
			_, _ = fmt.Fprintf(w, "\r[%s %s] %s: %.1f%% (%d/%d rows)",
				p.Vocabulary, p.Version, p.Phase, pct, p.RowsLoaded, p.RowsTotal)
		} else {
			_, _ = fmt.Fprintf(w, "\r[%s %s] %s: %d rows loaded",
				p.Vocabulary, p.Version, p.Phase, p.RowsLoaded)
		}
	}
}

// CountCSVRows counts the number of data rows in a CSV file (excluding header).
func CountCSVRows(path string) (count int64, err error) {
	// Use a simple line counter - faster than parsing CSV
	f, err := OpenFile(path)
	if err != nil {
		return 0, err
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	buf := make([]byte, 32*1024)
	for {
		n, err := f.Read(buf)
		if n > 0 {
			for _, b := range buf[:n] {
				if b == '\n' {
					count++
				}
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, err
		}
	}

	// Subtract 1 for header row
	if count > 0 {
		count--
	}

	return count, nil
}

// OpenFile opens a file for reading, supporting gzip compression.
func OpenFile(path string) (io.ReadCloser, error) {
	f, err := os.Open(path) //nolint:gosec // G304: path from trusted caller
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}

	// Check for gzip extension
	if filepath.Ext(path) == ".gz" {
		gz, err := gzip.NewReader(f)
		if err != nil {
			_ = f.Close() // Ignore close error on already-failed path
			return nil, fmt.Errorf("failed to create gzip reader: %w", err)
		}
		return &gzipReadCloser{gz: gz, f: f}, nil
	}

	return f, nil
}

type gzipReadCloser struct {
	gz *gzip.Reader
	f  *os.File
}

func (g *gzipReadCloser) Read(p []byte) (int, error) {
	return g.gz.Read(p)
}

func (g *gzipReadCloser) Close() error {
	gzErr := g.gz.Close()
	fErr := g.f.Close()
	if gzErr != nil {
		return gzErr
	}
	return fErr
}

// TruncateTable removes all rows from a table within a transaction.
func TruncateTable(ctx context.Context, tx *sql.Tx, table string) error {
	_, err := tx.ExecContext(ctx, fmt.Sprintf("TRUNCATE %s", table))
	return err
}

// DeleteByReleaseID removes all rows for a specific release from a table.
func DeleteByReleaseID(ctx context.Context, tx *sql.Tx, table string, releaseID int) error {
	_, err := tx.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s WHERE release_id = $1", table), releaseID)
	return err
}

// FileOpener provides methods for opening files from various sources (local, S3).
type FileOpener struct {
	provider storage.Provider
}

// NewFileOpener creates a FileOpener with the given storage provider.
// If provider is nil, only local files can be opened.
func NewFileOpener(provider storage.Provider) *FileOpener {
	return &FileOpener{provider: provider}
}

// Open opens a file from local filesystem or S3 depending on the path.
// Supports:
//   - Local paths: /path/to/file.rrf
//   - file:// URLs: file:///path/to/file.rrf
//   - s3:// URLs: s3://bucket/key/file.rrf
//
// Automatically handles .gz files with transparent decompression.
func (f *FileOpener) Open(ctx context.Context, path string) (io.ReadCloser, error) {
	// Check if it's an S3 URL
	if IsS3URL(path) {
		if f.provider == nil {
			return nil, fmt.Errorf("S3 provider not configured for path: %s", path)
		}
		return f.provider.Open(ctx, path)
	}

	// Handle file:// URLs
	if strings.HasPrefix(path, "file://") {
		u, err := url.Parse(path)
		if err != nil {
			return nil, fmt.Errorf("invalid file URL: %w", err)
		}
		path = u.Path
	}

	// Local file
	return OpenFile(path)
}

// Stat returns metadata for a file.
func (f *FileOpener) Stat(ctx context.Context, path string) (*storage.FileInfo, error) {
	if IsS3URL(path) {
		if f.provider == nil {
			return nil, fmt.Errorf("S3 provider not configured for path: %s", path)
		}
		return f.provider.Stat(ctx, path)
	}

	// Local file stat
	if strings.HasPrefix(path, "file://") {
		u, err := url.Parse(path)
		if err != nil {
			return nil, fmt.Errorf("invalid file URL: %w", err)
		}
		path = u.Path
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	return &storage.FileInfo{
		Path:         path,
		Size:         info.Size(),
		LastModified: info.ModTime(),
		IsDir:        info.IsDir(),
	}, nil
}

// List returns files in a directory or S3 prefix.
func (f *FileOpener) List(ctx context.Context, path string) ([]storage.FileInfo, error) {
	if IsS3URL(path) {
		if f.provider == nil {
			return nil, fmt.Errorf("S3 provider not configured for path: %s", path)
		}
		return f.provider.List(ctx, path)
	}

	// Local directory
	if strings.HasPrefix(path, "file://") {
		u, err := url.Parse(path)
		if err != nil {
			return nil, fmt.Errorf("invalid file URL: %w", err)
		}
		path = u.Path
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}

	var files []storage.FileInfo
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		files = append(files, storage.FileInfo{
			Path:         filepath.Join(path, entry.Name()),
			Size:         info.Size(),
			LastModified: info.ModTime(),
			IsDir:        entry.IsDir(),
		})
	}

	return files, nil
}

// IsS3URL checks if a path is an S3/MinIO URL.
func IsS3URL(path string) bool {
	return strings.HasPrefix(path, "s3://") || strings.HasPrefix(path, "minio://")
}

// JoinPath joins path components, handling both local and S3 paths.
func JoinPath(base, elem string) string {
	if IsS3URL(base) {
		// S3 uses forward slashes
		return strings.TrimSuffix(base, "/") + "/" + strings.TrimPrefix(elem, "/")
	}
	return filepath.Join(base, elem)
}

// CountRowsWithOpener counts rows using a FileOpener (supports S3).
func CountRowsWithOpener(ctx context.Context, opener *FileOpener, path string) (count int64, err error) {
	f, err := opener.Open(ctx, path)
	if err != nil {
		return 0, err
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	buf := make([]byte, 32*1024)
	for {
		n, err := f.Read(buf)
		if n > 0 {
			for _, b := range buf[:n] {
				if b == '\n' {
					count++
				}
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, err
		}
	}

	return count, nil
}
