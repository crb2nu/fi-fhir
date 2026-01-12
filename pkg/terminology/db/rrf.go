package db

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// RRFReader reads UMLS RRF format files (pipe-delimited).
// RRF files have the following characteristics:
// - Fields separated by | (pipe)
// - Lines terminated by |\n (pipe followed by newline)
// - Fields may contain quotes but are not quote-escaped
// - Empty fields appear as || (two consecutive pipes)
type RRFReader struct {
	r          *bufio.Reader
	columns    int         // Expected number of columns (-1 for auto-detect)
	lineNum    int64       // Current line number (for error reporting)
	totalLines int64       // Total lines if known (for progress)
	closer     io.Closer   // Optional closer for the underlying reader
	progressFn func(int64) // Optional progress callback
}

// NewRRFReader creates a reader for pipe-delimited RRF files.
// If columns > 0, rows with different column counts will be reported.
func NewRRFReader(r io.Reader, columns int) *RRFReader {
	return &RRFReader{
		r:       bufio.NewReaderSize(r, 1024*1024), // 1MB buffer for large lines
		columns: columns,
	}
}

// OpenRRFFile opens an RRF file (supports .gz compression) and returns a reader.
// The caller should call Close() when done.
func OpenRRFFile(path string, columns int) (*RRFReader, error) {
	f, err := OpenFile(path)
	if err != nil {
		return nil, fmt.Errorf("open RRF file: %w", err)
	}

	rr := NewRRFReader(f, columns)
	rr.closer = f
	return rr, nil
}

// SetProgressCallback sets a function called after each row with the line number.
func (r *RRFReader) SetProgressCallback(fn func(int64)) {
	r.progressFn = fn
}

// SetTotalLines sets the expected total lines (for progress reporting).
func (r *RRFReader) SetTotalLines(total int64) {
	r.totalLines = total
}

// TotalLines returns the expected total lines (0 if unknown).
func (r *RRFReader) TotalLines() int64 {
	return r.totalLines
}

// LineNum returns the current line number.
func (r *RRFReader) LineNum() int64 {
	return r.lineNum
}

// Close closes the underlying reader if it implements io.Closer.
func (r *RRFReader) Close() error {
	if r.closer != nil {
		return r.closer.Close()
	}
	return nil
}

// Read reads the next row from the RRF file.
// Returns io.EOF when there are no more rows.
func (r *RRFReader) Read() ([]string, error) {
	for {
		line, err := r.r.ReadString('\n')
		if err != nil && err != io.EOF {
			return nil, fmt.Errorf("read line %d: %w", r.lineNum+1, err)
		}

		// Handle EOF with partial line
		if errors.Is(err, io.EOF) {
			if line == "" {
				return nil, io.EOF
			}
		}

		r.lineNum++

		// Trim trailing newline and the trailing pipe that RRF files have
		line = strings.TrimSuffix(line, "\n")
		line = strings.TrimSuffix(line, "\r")
		line = strings.TrimSuffix(line, "|")

		// Skip empty lines
		if line == "" {
			if errors.Is(err, io.EOF) {
				return nil, io.EOF
			}
			continue
		}

		// Split on pipe
		fields := strings.Split(line, "|")

		// Note: We intentionally don't fail on mismatched columns - some RRF files have
		// variable columns. The caller can validate specific required fields.

		// Progress callback
		if r.progressFn != nil {
			r.progressFn(r.lineNum)
		}

		return fields, nil
	}
}

// ReadAll reads all remaining rows into memory.
// Use with caution for large files - prefer streaming with Read().
func (r *RRFReader) ReadAll() ([][]string, error) {
	var rows [][]string
	for {
		row, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// CountRRFRows counts the number of rows in an RRF file.
// This is useful for progress reporting before loading.
func CountRRFRows(path string) (int64, error) {
	f, err := OpenFile(path)
	if err != nil {
		return 0, err
	}
	defer func() { _ = f.Close() }()

	var count int64
	scanner := bufio.NewScanner(f)
	// Use a large buffer for long lines
	buf := make([]byte, 1024*1024)
	scanner.Buffer(buf, len(buf))

	for scanner.Scan() {
		line := scanner.Text()
		if line != "" {
			count++
		}
	}

	if err := scanner.Err(); err != nil {
		return 0, fmt.Errorf("count RRF rows: %w", err)
	}

	return count, nil
}

// RRFField safely retrieves a field from an RRF row, returning empty string if index out of bounds.
func RRFField(row []string, index int) string {
	if index < 0 || index >= len(row) {
		return ""
	}
	return row[index]
}

// RRFFieldInt safely retrieves an integer field, returning 0 if empty or invalid.
func RRFFieldInt(row []string, index int) int {
	s := RRFField(row, index)
	if s == "" {
		return 0
	}
	var v int
	_, _ = fmt.Sscanf(s, "%d", &v)
	return v
}

// RRFFieldInt64 safely retrieves an int64 field.
func RRFFieldInt64(row []string, index int) int64 {
	s := RRFField(row, index)
	if s == "" {
		return 0
	}
	var v int64
	_, _ = fmt.Sscanf(s, "%d", &v)
	return v
}

// UMLSFileInfo describes a UMLS META file.
type UMLSFileInfo struct {
	Name        string   // File name (e.g., "MRCONSO.RRF")
	Description string   // Human-readable description
	Columns     []string // Column names
}

// MRCONSO column indices (UMLS 2024AB format).
// These correspond to the columns in MRCONSO.RRF.
const (
	MRCONSOColCUI      = 0  // Concept Unique Identifier
	MRCONSOColLAT      = 1  // Language
	MRCONSOColTS       = 2  // Term Status
	MRCONSOColLUI      = 3  // Lexical Unique Identifier
	MRCONSOColSTT      = 4  // String Type
	MRCONSOColSUI      = 5  // String Unique Identifier
	MRCONSOColISPREF   = 6  // Is Preferred
	MRCONSOColAUI      = 7  // Atom Unique Identifier
	MRCONSOColSAUI     = 8  // Source Atom Unique Identifier
	MRCONSOColSCUI     = 9  // Source Concept Unique Identifier
	MRCONSOColSDUI     = 10 // Source Descriptor Unique Identifier
	MRCONSOColSAB      = 11 // Source Abbreviation
	MRCONSOColTTY      = 12 // Term Type
	MRCONSOColCODE     = 13 // Source Code
	MRCONSOColSTR      = 14 // String (term/name)
	MRCONSOColSRL      = 15 // Source Restriction Level
	MRCONSOColSUPPRESS = 16 // Suppress flag
	MRCONSOColCVF      = 17 // Content View Flag
	MRCONSOColumns     = 18 // Total columns
)

// MRREL column indices.
const (
	MRRELColCUI1     = 0  // CUI1
	MRRELColAUI1     = 1  // AUI1
	MRRELColSTYPE1   = 2  // STYPE1
	MRRELColREL      = 3  // REL
	MRRELColCUI2     = 4  // CUI2
	MRRELColAUI2     = 5  // AUI2
	MRRELColSTYPE2   = 6  // STYPE2
	MRRELColRELA     = 7  // RELA
	MRRELColRUI      = 8  // RUI
	MRRELColSRUI     = 9  // SRUI
	MRRELColSAB      = 10 // SAB
	MRRELColSL       = 11 // SL
	MRRELColRG       = 12 // RG
	MRRELColDIR      = 13 // DIR
	MRRELColSUPPRESS = 14 // SUPPRESS
	MRRELColCVF      = 15 // CVF
	MRRELColumns     = 16 // Total columns
)

// MRSTY column indices.
const (
	MRSTYColCUI  = 0 // CUI
	MRSTYColTUI  = 1 // TUI
	MRSTYColSTN  = 2 // STN
	MRSTYColSTY  = 3 // STY
	MRSTYColATUI = 4 // ATUI
	MRSTYColCVF  = 5 // CVF
	MRSTYColumns = 6 // Total columns
)

// ValidateUMLSDirectory checks if a directory contains valid UMLS META files.
func ValidateUMLSDirectory(path string) error {
	requiredFiles := []string{"MRCONSO.RRF", "MRREL.RRF", "MRSTY.RRF"}

	for _, file := range requiredFiles {
		fullPath := path + "/" + file
		// Also check for gzipped versions
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			fullPath = path + "/" + file + ".gz"
			if _, err := os.Stat(fullPath); os.IsNotExist(err) {
				return fmt.Errorf("required file not found: %s (or %s.gz)", file, file)
			}
		}
	}

	return nil
}

// StreamRRFFile streams an RRF file through a processing function.
// This is memory-efficient for large files.
func StreamRRFFile(ctx context.Context, path string, columns int, processFn func(row []string) error) error {
	reader, err := OpenRRFFile(path, columns)
	if err != nil {
		return err
	}
	defer func() { _ = reader.Close() }()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		row, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}

		if err := processFn(row); err != nil {
			return fmt.Errorf("process row %d: %w", reader.LineNum(), err)
		}
	}

	return nil
}
