package db

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCountCSVRows(t *testing.T) {
	tmpDir := t.TempDir()
	p := filepath.Join(tmpDir, "data.csv")

	content := "a,b,c\n1,2,3\n4,5,6\n"
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	n, err := CountCSVRows(p)
	if err != nil {
		t.Fatalf("CountCSVRows: %v", err)
	}
	if n != 2 {
		t.Fatalf("got %d, want %d", n, 2)
	}
}

func TestOpenFile_PlainAndGzip(t *testing.T) {
	tmpDir := t.TempDir()

	plainPath := filepath.Join(tmpDir, "plain.txt")
	if err := os.WriteFile(plainPath, []byte("hello"), 0o600); err != nil {
		t.Fatalf("write plain: %v", err)
	}

	rc, err := OpenFile(plainPath)
	if err != nil {
		t.Fatalf("OpenFile plain: %v", err)
	}
	defer func() { _ = rc.Close() }()
	plainBytes, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("read plain: %v", err)
	}
	if string(plainBytes) != "hello" {
		t.Fatalf("plain got %q", string(plainBytes))
	}

	gzPath := filepath.Join(tmpDir, "data.txt.gz")
	var gzBuf bytes.Buffer
	gzw := gzip.NewWriter(&gzBuf)
	if _, err := gzw.Write([]byte("gzip")); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	if err := gzw.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	if err := os.WriteFile(gzPath, gzBuf.Bytes(), 0o600); err != nil {
		t.Fatalf("write gz: %v", err)
	}

	gzrc, err := OpenFile(gzPath)
	if err != nil {
		t.Fatalf("OpenFile gz: %v", err)
	}
	defer func() { _ = gzrc.Close() }()
	gzBytes, err := io.ReadAll(gzrc)
	if err != nil {
		t.Fatalf("read gz: %v", err)
	}
	if string(gzBytes) != "gzip" {
		t.Fatalf("gz got %q", string(gzBytes))
	}
}

func TestBatchInserter_TotalInsertedAndValidation(t *testing.T) {
	b := NewBatchInserter(nil, "terminology.test", []string{"a", "b"}, 999999)

	if err := b.Add(context.Background(), "x"); err == nil {
		t.Fatalf("expected value count error")
	}

	if err := b.Add(context.Background(), "x", "y"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if got := b.TotalInserted(); got != 1 {
		t.Fatalf("TotalInserted got %d, want 1", got)
	}
}

func TestDefaultProgressReporter(t *testing.T) {
	var buf bytes.Buffer
	report := DefaultProgressReporter(&buf)

	report(LoadProgress{
		Vocabulary: "umls",
		Version:    "2024AB",
		Phase:      "Loading",
		RowsTotal:  10,
		RowsLoaded: 3,
	})

	out := buf.String()
	if !strings.Contains(out, "[umls 2024AB]") {
		t.Fatalf("unexpected output: %q", out)
	}
	if !strings.Contains(out, "Loading") {
		t.Fatalf("unexpected output: %q", out)
	}
	if !strings.Contains(out, "3/10") {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestValidateUMLSDirectory_GzAccepted(t *testing.T) {
	tmpDir := t.TempDir()

	// Missing files should fail.
	if err := ValidateUMLSDirectory(tmpDir); err == nil {
		t.Fatalf("expected error for missing required files")
	}

	// Create required files.
	for _, name := range []string{"MRCONSO.RRF", "MRREL.RRF.gz", "MRSTY.RRF"} {
		if err := os.WriteFile(filepath.Join(tmpDir, name), []byte("x"), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	if err := ValidateUMLSDirectory(tmpDir); err != nil {
		t.Fatalf("ValidateUMLSDirectory: %v", err)
	}
}

func TestStreamRRFFile(t *testing.T) {
	tmpDir := t.TempDir()
	p := filepath.Join(tmpDir, "MRCONSO.RRF")

	// Lines end with trailing pipe in real RRF; our reader trims it.
	content := "a|b|c|\n1||3|\n\n"
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	var rows [][]string
	err := StreamRRFFile(context.Background(), p, -1, func(row []string) error {
		rows = append(rows, row)
		return nil
	})
	if err != nil {
		t.Fatalf("StreamRRFFile: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows=%d, want 2", len(rows))
	}
	if strings.Join(rows[0], ",") != "a,b,c" {
		t.Fatalf("row0=%v", rows[0])
	}
	if strings.Join(rows[1], ",") != "1,,3" {
		t.Fatalf("row1=%v", rows[1])
	}
}

func TestStreamRRFFile_ContextCancel(t *testing.T) {
	tmpDir := t.TempDir()
	p := filepath.Join(tmpDir, "MRCONSO.RRF")
	if err := os.WriteFile(p, []byte("a|b|\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := StreamRRFFile(ctx, p, -1, func(row []string) error { return nil })
	if err == nil || !strings.Contains(err.Error(), context.Canceled.Error()) {
		t.Fatalf("expected context canceled, got %v", err)
	}
}

func TestLOINCCode_DisplayNameAndIsActive(t *testing.T) {
	c := &LOINCCode{
		LongCommonName: "Long",
		Status:         "ACTIVE",
	}
	if got := c.DisplayName(); got != "Long" {
		t.Fatalf("DisplayName got %q", got)
	}
	if !c.IsActive() {
		t.Fatalf("expected active")
	}
}

func TestGetColAndNullIfEmpty(t *testing.T) {
	record := []string{"  X  ", ""}
	colIdx := map[string]int{"A": 0, "B": 1, "C": 2}

	if got := getCol(record, colIdx, "A"); got != "X" {
		t.Fatalf("getCol A got %q", got)
	}
	if got := getCol(record, colIdx, "C"); got != "" {
		t.Fatalf("getCol out of bounds got %q", got)
	}
	if nullIfEmpty("") != nil {
		t.Fatalf("nullIfEmpty empty expected nil")
	}
	if nullIfEmpty("x") == nil {
		t.Fatalf("nullIfEmpty non-empty expected value")
	}
}

func TestUMLSAndRxNormHelpers(t *testing.T) {
	tmpDir := t.TempDir()

	if got := formatNDC("12345678901"); got != "12345-6789-01" {
		t.Fatalf("formatNDC got %q", got)
	}
	if got := formatNDC("short"); got != "short" {
		t.Fatalf("formatNDC passthrough got %q", got)
	}

	// validateRxNormDirectory requires RXNCONSO.RRF or .gz
	if err := validateRxNormDirectory(tmpDir); err == nil {
		t.Fatalf("expected missing RXNCONSO.RRF error")
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "RXNCONSO.RRF.gz"), []byte("x"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := validateRxNormDirectory(tmpDir); err != nil {
		t.Fatalf("validateRxNormDirectory: %v", err)
	}

	if nullableString("") != nil {
		t.Fatalf("nullableString empty expected nil")
	}
	if nullableString("x") == nil {
		t.Fatalf("nullableString non-empty expected value")
	}
	if nullableInt(0) != nil {
		t.Fatalf("nullableInt zero expected nil")
	}
	if nullableInt(1) == nil {
		t.Fatalf("nullableInt non-zero expected value")
	}

	// findRRFFile prefers plain, then .gz.
	dir2 := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir2, "MRREL.RRF.gz"), []byte("x"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	got := findRRFFile(dir2, "MRREL.RRF")
	if !strings.HasSuffix(got, "MRREL.RRF.gz") {
		t.Fatalf("findRRFFile got %q", got)
	}

	// fileExists sanity
	if !fileExists(got) {
		t.Fatalf("expected fileExists true")
	}

	// Touch formatting: ensure progress reporter doesn't panic on zeros.
	var buf bytes.Buffer
	DefaultProgressReporter(&buf)(LoadProgress{Vocabulary: "x", Version: "v", Phase: "p", RowsTotal: 0, RowsLoaded: 1})
}
