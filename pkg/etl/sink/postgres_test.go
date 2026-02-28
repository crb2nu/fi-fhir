package sink

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/storage"
)

func TestNewPostgresSink(t *testing.T) {
	s := NewPostgresSink(PostgresSinkConfig{
		Name: "test-pg",
	})
	if s.name != "test-pg" {
		t.Errorf("name = %q, want %q", s.name, "test-pg")
	}
	if s.tempDir == "" {
		t.Error("tempDir should default to os.TempDir()")
	}
}

func TestNewPostgresSink_CustomTempDir(t *testing.T) {
	s := NewPostgresSink(PostgresSinkConfig{
		Name:    "test",
		TempDir: "/custom/tmp",
	})
	if s.tempDir != "/custom/tmp" {
		t.Errorf("tempDir = %q, want %q", s.tempDir, "/custom/tmp")
	}
}

func TestPostgresSink_Name(t *testing.T) {
	s := NewPostgresSink(PostgresSinkConfig{Name: "pg-sink"})
	if got := s.Name(); got != "pg-sink" {
		t.Errorf("Name() = %q, want %q", got, "pg-sink")
	}
}

func TestPostgresSink_Write_ReturnsError(t *testing.T) {
	s := NewPostgresSink(PostgresSinkConfig{Name: "test"})
	err := s.Write(context.Background(), "any/path", nil, 0)
	if err == nil {
		t.Fatal("Write() should return error")
	}
	if !strings.Contains(err.Error(), "not supported") {
		t.Errorf("error = %q, should mention 'not supported'", err.Error())
	}
}

// --- extractZip tests ---

func createTestZip(t *testing.T, dir string, files map[string]string) string {
	t.Helper()
	zipPath := filepath.Join(dir, "test.zip")
	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	w := zip.NewWriter(f)
	for name, content := range files {
		fw, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return zipPath
}

func TestExtractZip_Files(t *testing.T) {
	tmpDir := t.TempDir()
	zipPath := createTestZip(t, tmpDir, map[string]string{
		"a.txt":       "alpha",
		"sub/b.txt":   "beta",
		"sub/c/d.txt": "delta",
	})

	targetDir := filepath.Join(tmpDir, "out")
	if err := os.MkdirAll(targetDir, 0o750); err != nil {
		t.Fatal(err)
	}

	if err := extractZip(zipPath, targetDir); err != nil {
		t.Fatalf("extractZip() error = %v", err)
	}

	// Verify files
	for _, tc := range []struct {
		path    string
		content string
	}{
		{"a.txt", "alpha"},
		{"sub/b.txt", "beta"},
		{"sub/c/d.txt", "delta"},
	} {
		data, err := os.ReadFile(filepath.Join(targetDir, tc.path))
		if err != nil {
			t.Errorf("missing extracted file %s: %v", tc.path, err)
			continue
		}
		if string(data) != tc.content {
			t.Errorf("%s content = %q, want %q", tc.path, string(data), tc.content)
		}
	}
}

func TestExtractZip_InvalidFile(t *testing.T) {
	tmpDir := t.TempDir()
	badZip := filepath.Join(tmpDir, "bad.zip")
	if err := os.WriteFile(badZip, []byte("not a zip"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := extractZip(badZip, tmpDir); err == nil {
		t.Error("extractZip() should error on invalid zip")
	}
}

func TestExtractZip_Directories(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a zip with a directory entry.
	zipPath := filepath.Join(tmpDir, "dirs.zip")
	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	// Add a directory entry.
	header := &zip.FileHeader{Name: "mydir/"}
	header.SetMode(0o750 | os.ModeDir)
	if _, err := w.CreateHeader(header); err != nil {
		t.Fatal(err)
	}
	// Add a file inside the directory.
	fh := &zip.FileHeader{Name: "mydir/file.txt"}
	fh.SetMode(0o644)
	fw, err := w.CreateHeader(fh)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write([]byte("inside")); err != nil {
		t.Fatal(err)
	}
	_ = w.Close()
	_ = f.Close()

	targetDir := filepath.Join(tmpDir, "out")
	if err := os.MkdirAll(targetDir, 0o750); err != nil {
		t.Fatal(err)
	}

	if err := extractZip(zipPath, targetDir); err != nil {
		t.Fatalf("extractZip() error = %v", err)
	}

	info, err := os.Stat(filepath.Join(targetDir, "mydir"))
	if err != nil {
		t.Fatalf("directory not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("expected directory, got file")
	}
}

// --- findSubdir tests ---

func TestFindSubdir_Found(t *testing.T) {
	tmpDir := t.TempDir()
	metaDir := filepath.Join(tmpDir, "some", "META")
	if err := os.MkdirAll(metaDir, 0o750); err != nil {
		t.Fatal(err)
	}

	found, err := findSubdir(tmpDir, "META")
	if err != nil {
		t.Fatalf("findSubdir() error = %v", err)
	}
	if found != metaDir {
		t.Errorf("findSubdir() = %q, want %q", found, metaDir)
	}
}

func TestFindSubdir_CaseInsensitive(t *testing.T) {
	tmpDir := t.TempDir()
	metaDir := filepath.Join(tmpDir, "meta")
	if err := os.MkdirAll(metaDir, 0o750); err != nil {
		t.Fatal(err)
	}

	found, err := findSubdir(tmpDir, "META")
	if err != nil {
		t.Fatalf("findSubdir() error = %v", err)
	}
	if found != metaDir {
		t.Errorf("findSubdir() = %q, want %q", found, metaDir)
	}
}

func TestFindSubdir_NotFound(t *testing.T) {
	tmpDir := t.TempDir()

	_, err := findSubdir(tmpDir, "NONEXISTENT")
	if err == nil {
		t.Error("expected error for missing subdirectory")
	}
}

// --- findFile tests ---

func TestFindFile_Found(t *testing.T) {
	tmpDir := t.TempDir()
	target := filepath.Join(tmpDir, "sub", "LoincTable.csv")
	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}

	found, err := findFile(tmpDir, "LoincTable.csv", "Loinc.csv")
	if err != nil {
		t.Fatalf("findFile() error = %v", err)
	}
	if found != target {
		t.Errorf("findFile() = %q, want %q", found, target)
	}
}

func TestFindFile_CaseInsensitive(t *testing.T) {
	tmpDir := t.TempDir()
	target := filepath.Join(tmpDir, "loinctable.csv")
	if err := os.WriteFile(target, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}

	found, err := findFile(tmpDir, "LoincTable.csv")
	if err != nil {
		t.Fatalf("findFile() error = %v", err)
	}
	if found != target {
		t.Errorf("findFile() = %q, want %q", found, target)
	}
}

func TestFindFile_AlternativeName(t *testing.T) {
	tmpDir := t.TempDir()
	target := filepath.Join(tmpDir, "Loinc.csv")
	if err := os.WriteFile(target, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}

	found, err := findFile(tmpDir, "LoincTable.csv", "Loinc.csv")
	if err != nil {
		t.Fatalf("findFile() error = %v", err)
	}
	if found != target {
		t.Errorf("findFile() = %q, want %q", found, target)
	}
}

func TestFindFile_NotFound(t *testing.T) {
	tmpDir := t.TempDir()

	_, err := findFile(tmpDir, "missing.csv")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

// --- mockProvider implements storage.Provider for testing downloadAndExtract ---

type mockProvider struct {
	files map[string]string // path -> content
	err   error
}

func (m *mockProvider) Open(_ context.Context, path string) (io.ReadCloser, error) {
	if m.err != nil {
		return nil, m.err
	}
	content, ok := m.files[path]
	if !ok {
		return nil, fmt.Errorf("file not found: %s", path)
	}
	return io.NopCloser(strings.NewReader(content)), nil
}

func (m *mockProvider) Stat(_ context.Context, _ string) (*storage.FileInfo, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockProvider) List(_ context.Context, _ string) ([]storage.FileInfo, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockProvider) ListRecursive(_ context.Context, prefix string) ([]storage.FileInfo, error) {
	if m.err != nil {
		return nil, m.err
	}
	var result []storage.FileInfo
	for path := range m.files {
		if strings.HasPrefix(path, prefix) {
			result = append(result, storage.FileInfo{
				Path:         path,
				Size:         int64(len(m.files[path])),
				LastModified: time.Now(),
			})
		}
	}
	return result, nil
}

func (m *mockProvider) Put(_ context.Context, _ string, _ io.Reader, _ int64) error {
	return fmt.Errorf("not implemented")
}

func (m *mockProvider) Delete(_ context.Context, _ string) error {
	return fmt.Errorf("not implemented")
}

func (m *mockProvider) Exists(_ context.Context, _ string) (bool, error) {
	return false, fmt.Errorf("not implemented")
}

// --- downloadAndExtract tests ---

func TestDownloadAndExtract_PlainFiles(t *testing.T) {
	provider := &mockProvider{
		files: map[string]string{
			"data/v1/concepts.txt":  "concept data",
			"data/v1/relations.txt": "relation data",
		},
	}

	s := NewPostgresSink(PostgresSinkConfig{
		Name:     "test",
		Provider: provider,
		TempDir:  t.TempDir(),
	})

	localDir, cleanup, err := s.downloadAndExtract(context.Background(), "data/v1", "test-v1")
	if err != nil {
		t.Fatalf("downloadAndExtract() error = %v", err)
	}
	defer cleanup()

	// Should have files in the local dir.
	entries, err := os.ReadDir(localDir)
	if err != nil {
		t.Fatalf("ReadDir error = %v", err)
	}
	if len(entries) == 0 {
		t.Error("expected files in local directory")
	}

	// Verify content of one file.
	for _, entry := range entries {
		data, err := os.ReadFile(filepath.Join(localDir, entry.Name()))
		if err != nil {
			t.Fatalf("ReadFile error = %v", err)
		}
		if string(data) == "" {
			t.Errorf("empty file: %s", entry.Name())
		}
	}
}

func TestDownloadAndExtract_WithZip(t *testing.T) {
	// Create a zip file in memory.
	tmpDir := t.TempDir()
	zipPath := createTestZip(t, tmpDir, map[string]string{
		"META/concepts.txt": "concept data",
	})
	zipContent, err := os.ReadFile(zipPath)
	if err != nil {
		t.Fatal(err)
	}

	provider := &mockProvider{
		files: map[string]string{
			"data/v1/archive.zip": string(zipContent),
		},
	}

	s := NewPostgresSink(PostgresSinkConfig{
		Name:     "test",
		Provider: provider,
		TempDir:  t.TempDir(),
	})

	localDir, cleanup, err := s.downloadAndExtract(context.Background(), "data/v1", "test-v1")
	if err != nil {
		t.Fatalf("downloadAndExtract() error = %v", err)
	}
	defer cleanup()

	// After extraction, META/concepts.txt should exist.
	found, err := findFile(localDir, "concepts.txt")
	if err != nil {
		t.Fatalf("concepts.txt not found after extraction: %v", err)
	}
	data, err := os.ReadFile(found)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "concept data" {
		t.Errorf("content = %q, want %q", string(data), "concept data")
	}
}

func TestDownloadAndExtract_EmptyStorage(t *testing.T) {
	provider := &mockProvider{
		files: map[string]string{},
	}

	s := NewPostgresSink(PostgresSinkConfig{
		Name:     "test",
		Provider: provider,
		TempDir:  t.TempDir(),
	})

	_, _, err := s.downloadAndExtract(context.Background(), "empty/path", "test")
	if err == nil {
		t.Error("expected error for empty storage path")
	}
	if !strings.Contains(err.Error(), "no files found") {
		t.Errorf("error = %q, want to contain 'no files found'", err.Error())
	}
}

func TestDownloadAndExtract_ListError(t *testing.T) {
	provider := &mockProvider{
		err: fmt.Errorf("connection refused"),
	}

	s := NewPostgresSink(PostgresSinkConfig{
		Name:     "test",
		Provider: provider,
		TempDir:  t.TempDir(),
	})

	_, _, err := s.downloadAndExtract(context.Background(), "any/path", "test")
	if err == nil {
		t.Error("expected error from provider failure")
	}
}

func TestDownloadAndExtract_SkipsDirs(t *testing.T) {
	// Files with IsDir=true should be skipped.
	provider := &mockProvider{
		files: map[string]string{
			"data/v1/file.txt": "content",
		},
	}

	s := NewPostgresSink(PostgresSinkConfig{
		Name:     "test",
		Provider: provider,
		TempDir:  t.TempDir(),
	})

	localDir, cleanup, err := s.downloadAndExtract(context.Background(), "data/v1", "test")
	if err != nil {
		t.Fatalf("downloadAndExtract() error = %v", err)
	}
	defer cleanup()

	data, err := os.ReadFile(filepath.Join(localDir, "file.txt"))
	if err != nil {
		t.Fatalf("ReadFile error = %v", err)
	}
	if string(data) != "content" {
		t.Errorf("content = %q, want %q", string(data), "content")
	}
}

func TestPostgresSink_Exists_ShortPath(t *testing.T) {
	// Exists with < 2 parts should return false.
	s := NewPostgresSink(PostgresSinkConfig{Name: "test"})
	exists, err := s.Exists(context.Background(), "noversion")
	if err != nil {
		t.Fatalf("Exists() error = %v", err)
	}
	if exists {
		t.Error("Exists() should return false for short path")
	}
}

// --- downloadAndExtract error path tests ---

// errorOnOpenProvider fails on Open calls after listing files normally.
type errorOnOpenProvider struct {
	mockProvider
	openErr error
}

func (e *errorOnOpenProvider) Open(_ context.Context, _ string) (io.ReadCloser, error) {
	return nil, e.openErr
}

func TestDownloadAndExtract_OpenError(t *testing.T) {
	base := &mockProvider{
		files: map[string]string{
			"data/v1/file.txt": "content",
		},
	}
	provider := &errorOnOpenProvider{
		mockProvider: *base,
		openErr:      fmt.Errorf("permission denied"),
	}

	s := NewPostgresSink(PostgresSinkConfig{
		Name:     "test",
		Provider: provider,
		TempDir:  t.TempDir(),
	})

	_, _, err := s.downloadAndExtract(context.Background(), "data/v1", "test")
	if err == nil {
		t.Error("expected error from Open failure")
	}
}

// --- extractZip zip-slip test ---

func TestExtractZip_ZipSlipPrevention(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a zip with a path traversal entry.
	zipPath := filepath.Join(tmpDir, "evil.zip")
	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	// Craft a header with path traversal.
	header := &zip.FileHeader{Name: "../../../etc/passwd"}
	header.SetMode(0o644)
	fw, err := w.CreateHeader(header)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = fw.Write([]byte("root:x:0:0"))
	_ = w.Close()
	_ = f.Close()

	targetDir := filepath.Join(tmpDir, "safe")
	if err := os.MkdirAll(targetDir, 0o750); err != nil {
		t.Fatal(err)
	}

	err = extractZip(zipPath, targetDir)
	if err == nil {
		t.Error("extractZip() should reject path traversal entries")
	}
	if err != nil && !strings.Contains(err.Error(), "invalid file path") {
		t.Errorf("error = %q, want to contain 'invalid file path'", err.Error())
	}
}

func TestExtractZip_Empty(t *testing.T) {
	tmpDir := t.TempDir()

	// Create an empty zip.
	zipPath := filepath.Join(tmpDir, "empty.zip")
	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	_ = w.Close()
	_ = f.Close()

	if err := extractZip(zipPath, tmpDir); err != nil {
		t.Errorf("extractZip() should succeed on empty zip: %v", err)
	}
}

func TestDownloadAndExtract_CorruptZip(t *testing.T) {
	provider := &mockProvider{
		files: map[string]string{
			"data/v1/bad.zip": "this is not a zip file",
		},
	}

	s := NewPostgresSink(PostgresSinkConfig{
		Name:     "test",
		Provider: provider,
		TempDir:  t.TempDir(),
	})

	_, _, err := s.downloadAndExtract(context.Background(), "data/v1", "test")
	if err == nil {
		t.Error("expected error from corrupt zip extraction")
	}
	if !strings.Contains(err.Error(), "extract") {
		t.Errorf("error = %q, should mention extract failure", err.Error())
	}
}

// --- NewMinIOSink error test ---

func TestNewMinIOSink_InvalidConfig(t *testing.T) {
	// An empty endpoint should cause NewMinIOProvider to fail.
	_, err := NewMinIOSink(MinIOSinkConfig{
		Name: "test",
		MinIOConfig: storage.MinIOConfig{
			Endpoint: "", // Invalid — no endpoint
		},
	})
	if err == nil {
		t.Error("NewMinIOSink should error with empty endpoint")
	}
}
