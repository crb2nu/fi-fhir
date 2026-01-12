package db

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRRFReader_Read(t *testing.T) {
	// Simple RRF content - RRF format has trailing | before newline
	// MRCONSO: CUI|LAT|TS|LUI|STT|SUI|ISPREF|AUI|SAUI|SCUI|SDUI|SAB|TTY|CODE|STR|SRL|SUPPRESS|CVF|
	content := "C0000001|ENG|P|L0000001|PF|S0000001|Y|A0000001|||C0000001|SNOMEDCT_US|PT|12345|Test Term|0|N||\n"
	reader := NewRRFReader(strings.NewReader(content), -1) // -1 means auto-detect columns

	row, err := reader.Read()
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	if len(row) < 15 {
		t.Fatalf("Expected at least 15 fields, got %d", len(row))
	}

	if row[0] != "C0000001" {
		t.Errorf("Expected CUI C0000001, got %s", row[0])
	}
	if row[11] != "SNOMEDCT_US" {
		t.Errorf("Expected SAB SNOMEDCT_US, got %s", row[11])
	}
	if row[14] != "Test Term" {
		t.Errorf("Expected STR 'Test Term', got %s", row[14])
	}

	// Should return EOF on second read
	_, err = reader.Read()
	if !errors.Is(err, io.EOF) {
		t.Errorf("Expected EOF, got %v", err)
	}
}

func TestRRFReader_MultipleRows(t *testing.T) {
	content := `C0000001|ENG|P|L0000001|PF|S0000001|Y|A0000001|||C0000001||SNOMEDCT_US|PT|12345|Term1|0|N||
C0000002|ENG|S|L0000002|VO|S0000002|N|A0000002|||C0000002||ICD10CM|SY|67890|Term2|0|N||
`
	reader := NewRRFReader(strings.NewReader(content), 18)

	rows, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}

	if len(rows) != 2 {
		t.Errorf("Expected 2 rows, got %d", len(rows))
	}

	if rows[0][0] != "C0000001" {
		t.Errorf("First row CUI: expected C0000001, got %s", rows[0][0])
	}
	if rows[1][0] != "C0000002" {
		t.Errorf("Second row CUI: expected C0000002, got %s", rows[1][0])
	}
}

func TestRRFReader_EmptyFields(t *testing.T) {
	// RRF with empty fields
	content := "C0000001|ENG|P||PF|||A0000001|||C0000001||SNOMEDCT_US|PT|12345|Term|0|N||\n"
	reader := NewRRFReader(strings.NewReader(content), 18)

	row, err := reader.Read()
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	if row[3] != "" {
		t.Errorf("Expected empty LUI, got %s", row[3])
	}
	if row[5] != "" {
		t.Errorf("Expected empty SUI, got %s", row[5])
	}
}

func TestRRFField(t *testing.T) {
	row := []string{"A", "B", "C", "D"}

	tests := []struct {
		index    int
		expected string
	}{
		{0, "A"},
		{1, "B"},
		{3, "D"},
		{4, ""},  // Out of bounds
		{-1, ""}, // Negative
	}

	for _, tt := range tests {
		got := RRFField(row, tt.index)
		if got != tt.expected {
			t.Errorf("RRFField(%v, %d) = %q, want %q", row, tt.index, got, tt.expected)
		}
	}
}

func TestRRFFieldInt(t *testing.T) {
	row := []string{"123", "456", "", "abc"}

	tests := []struct {
		index    int
		expected int
	}{
		{0, 123},
		{1, 456},
		{2, 0},  // Empty
		{3, 0},  // Non-numeric
		{99, 0}, // Out of bounds
	}

	for _, tt := range tests {
		got := RRFFieldInt(row, tt.index)
		if got != tt.expected {
			t.Errorf("RRFFieldInt(%v, %d) = %d, want %d", row, tt.index, got, tt.expected)
		}
	}
}

func TestCountRRFRows(t *testing.T) {
	// Skip if test data doesn't exist
	testDataPath := findTestDataPath("MRCONSO_sample.RRF")
	if testDataPath == "" {
		t.Skip("Test data not found")
	}

	count, err := CountRRFRows(testDataPath)
	if err != nil {
		t.Fatalf("CountRRFRows failed: %v", err)
	}

	if count != 5 {
		t.Errorf("Expected 5 rows, got %d", count)
	}
}

func TestOpenRRFFile(t *testing.T) {
	testDataPath := findTestDataPath("MRCONSO_sample.RRF")
	if testDataPath == "" {
		t.Skip("Test data not found")
	}

	reader, err := OpenRRFFile(testDataPath, MRCONSOColumns)
	if err != nil {
		t.Fatalf("OpenRRFFile failed: %v", err)
	}
	defer func() { _ = reader.Close() }()

	// Read all rows
	var count int
	for {
		_, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("Read failed at row %d: %v", count+1, err)
		}
		count++
	}

	if count != 5 {
		t.Errorf("Expected 5 rows, got %d", count)
	}
}

func TestValidateUMLSDirectory(t *testing.T) {
	// Create temp directory with required files
	tmpDir := t.TempDir()

	// Missing files - should fail
	err := ValidateUMLSDirectory(tmpDir)
	if err == nil {
		t.Error("Expected error for empty directory")
	}

	// Create required files
	for _, file := range []string{"MRCONSO.RRF", "MRREL.RRF", "MRSTY.RRF"} {
		f, err := os.Create(filepath.Join(tmpDir, file)) //nolint:gosec // G304: test file, path from trusted test code
		if err != nil {
			t.Fatalf("Failed to create %s: %v", file, err)
		}
		_ = f.Close()
	}

	// Should succeed now
	err = ValidateUMLSDirectory(tmpDir)
	if err != nil {
		t.Errorf("Expected success, got error: %v", err)
	}
}

func TestMRCONSOColumnConstants(t *testing.T) {
	// Verify column constants match expected UMLS MRCONSO format
	tests := []struct {
		name     string
		constant int
		expected int
	}{
		{"CUI", MRCONSOColCUI, 0},
		{"LAT", MRCONSOColLAT, 1},
		{"SAB", MRCONSOColSAB, 11},
		{"TTY", MRCONSOColTTY, 12},
		{"CODE", MRCONSOColCODE, 13},
		{"STR", MRCONSOColSTR, 14},
		{"Total", MRCONSOColumns, 18},
	}

	for _, tt := range tests {
		if tt.constant != tt.expected {
			t.Errorf("%s: expected %d, got %d", tt.name, tt.expected, tt.constant)
		}
	}
}

func TestMRRELColumnConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant int
		expected int
	}{
		{"CUI1", MRRELColCUI1, 0},
		{"REL", MRRELColREL, 3},
		{"CUI2", MRRELColCUI2, 4},
		{"RELA", MRRELColRELA, 7},
		{"SAB", MRRELColSAB, 10},
		{"Total", MRRELColumns, 16},
	}

	for _, tt := range tests {
		if tt.constant != tt.expected {
			t.Errorf("%s: expected %d, got %d", tt.name, tt.expected, tt.constant)
		}
	}
}

// findTestDataPath locates test data file
func findTestDataPath(filename string) string {
	paths := []string{
		filepath.Join("testdata", "terminology", filename),
		filepath.Join("..", "..", "..", "testdata", "terminology", filename),
	}

	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			abs, _ := filepath.Abs(path)
			return abs
		}
	}

	return ""
}

func TestIsS3URL(t *testing.T) {
	tests := []struct {
		path   string
		expect bool
	}{
		{"s3://bucket/key", true},
		{"s3://bucket/path/to/file.rrf", true},
		{"minio://bucket/key", true},
		{"/local/path/file.rrf", false},
		{"file:///local/path", false},
		{"http://example.com/file", false},
	}

	for _, tt := range tests {
		got := IsS3URL(tt.path)
		if got != tt.expect {
			t.Errorf("IsS3URL(%q) = %v, want %v", tt.path, got, tt.expect)
		}
	}
}

func TestJoinPath(t *testing.T) {
	tests := []struct {
		base   string
		elem   string
		expect string
	}{
		{"s3://bucket/prefix", "file.rrf", "s3://bucket/prefix/file.rrf"},
		{"s3://bucket/prefix/", "file.rrf", "s3://bucket/prefix/file.rrf"},
		{"s3://bucket", "path/file.rrf", "s3://bucket/path/file.rrf"},
		{"/local/path", "file.rrf", filepath.Join("/local/path", "file.rrf")},
	}

	for _, tt := range tests {
		got := JoinPath(tt.base, tt.elem)
		if got != tt.expect {
			t.Errorf("JoinPath(%q, %q) = %q, want %q", tt.base, tt.elem, got, tt.expect)
		}
	}
}

func TestFileOpener_LocalFile(t *testing.T) {
	// Create temp file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.rrf")
	content := "C0000001|ENG|P|L0000001|PF|S0000001|Y|A0000001|||C0000001|SNOMEDCT_US|PT|12345|Test Term|0|N||\n"
	if err := os.WriteFile(testFile, []byte(content), 0o644); err != nil { //nolint:gosec // G306: test file, 0644 perms acceptable
		t.Fatal(err)
	}

	opener := NewFileOpener(nil) // No S3 provider

	// Test Open
	ctx := t.Context()
	r, err := opener.Open(ctx, testFile)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = r.Close() }()

	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}
	if string(data) != content {
		t.Errorf("content mismatch: got %q, want %q", string(data), content)
	}

	// Test Stat
	info, err := opener.Stat(ctx, testFile)
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}
	if info.Size != int64(len(content)) {
		t.Errorf("Size = %d, want %d", info.Size, len(content))
	}
}

func TestFileOpener_S3WithoutProvider(t *testing.T) {
	opener := NewFileOpener(nil)
	ctx := t.Context()

	_, err := opener.Open(ctx, "s3://bucket/file.rrf")
	if err == nil {
		t.Error("expected error for S3 URL without provider")
	}
}
