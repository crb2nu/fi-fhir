package storage

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalProvider_Open_NonExistent(t *testing.T) {
	provider := NewLocalProvider(t.TempDir())
	ctx := context.Background()

	_, err := provider.Open(ctx, "does-not-exist.txt")
	if err == nil {
		t.Fatal("expected error for non-existent file")
	}
	if !strings.Contains(err.Error(), "failed to open file") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestLocalProvider_Open_MalformedGzip(t *testing.T) {
	tmpDir := t.TempDir()
	provider := NewLocalProvider(tmpDir)
	ctx := context.Background()

	// Write invalid gzip data with .gz extension
	if err := os.WriteFile(filepath.Join(tmpDir, "bad.txt.gz"), []byte("not gzip data"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := provider.Open(ctx, "bad.txt.gz")
	if err == nil {
		t.Fatal("expected error for malformed gzip file")
	}
	if !strings.Contains(err.Error(), "failed to create gzip reader") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestLocalProvider_Stat_NonExistent(t *testing.T) {
	provider := NewLocalProvider(t.TempDir())
	ctx := context.Background()

	_, err := provider.Stat(ctx, "does-not-exist.txt")
	if err == nil {
		t.Fatal("expected error for non-existent file")
	}
	if !strings.Contains(err.Error(), "file not found") {
		t.Errorf("expected 'file not found' error, got: %v", err)
	}
}

func TestLocalProvider_Stat_Directory(t *testing.T) {
	tmpDir := t.TempDir()
	provider := NewLocalProvider(tmpDir)
	ctx := context.Background()

	subDir := filepath.Join(tmpDir, "mydir")
	if err := os.Mkdir(subDir, 0o750); err != nil {
		t.Fatal(err)
	}

	info, err := provider.Stat(ctx, "mydir")
	if err != nil {
		t.Fatalf("Stat directory failed: %v", err)
	}
	if !info.IsDir {
		t.Error("expected IsDir=true for directory")
	}
	if info.Path != "mydir" {
		t.Errorf("Path = %q, want %q", info.Path, "mydir")
	}
}

func TestLocalProvider_List_NonExistent(t *testing.T) {
	provider := NewLocalProvider(t.TempDir())
	ctx := context.Background()

	files, err := provider.List(ctx, "no-such-dir")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if files != nil {
		t.Errorf("expected nil for non-existent path, got %d entries", len(files))
	}
}

func TestLocalProvider_List_SingleFile(t *testing.T) {
	tmpDir := t.TempDir()
	provider := NewLocalProvider(tmpDir)
	ctx := context.Background()

	if err := os.WriteFile(filepath.Join(tmpDir, "solo.txt"), []byte("solo"), 0o600); err != nil {
		t.Fatal(err)
	}

	files, err := provider.List(ctx, "solo.txt")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	if files[0].Path != "solo.txt" {
		t.Errorf("Path = %q, want %q", files[0].Path, "solo.txt")
	}
	if files[0].IsDir {
		t.Error("expected IsDir=false for single file")
	}
}

func TestLocalProvider_ListRecursive(t *testing.T) {
	tmpDir := t.TempDir()
	provider := NewLocalProvider(tmpDir)
	ctx := context.Background()

	// Create nested structure: root/a.txt, root/sub/b.txt, root/sub/deep/c.txt
	for _, p := range []string{"a.txt", "sub/b.txt", "sub/deep/c.txt"} {
		full := filepath.Join(tmpDir, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("data"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	files, err := provider.ListRecursive(ctx, "")
	if err != nil {
		t.Fatalf("ListRecursive failed: %v", err)
	}

	// Should find: a.txt, sub/, sub/b.txt, sub/deep/, sub/deep/c.txt = 5 entries
	if len(files) < 3 {
		t.Errorf("expected at least 3 entries, got %d", len(files))
	}

	// Verify that nested files are included
	var foundDeep bool
	for _, f := range files {
		if strings.Contains(f.Path, "deep") && strings.Contains(f.Path, "c.txt") {
			foundDeep = true
			break
		}
	}
	if !foundDeep {
		t.Error("expected to find sub/deep/c.txt in recursive listing")
	}
}

func TestLocalProvider_ListRecursive_NonExistent(t *testing.T) {
	provider := NewLocalProvider(t.TempDir())
	ctx := context.Background()

	files, err := provider.ListRecursive(ctx, "no-such-dir")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if files != nil {
		t.Errorf("expected nil for non-existent path, got %d entries", len(files))
	}
}

func TestLocalProvider_ListRecursive_SingleFile(t *testing.T) {
	tmpDir := t.TempDir()
	provider := NewLocalProvider(tmpDir)
	ctx := context.Background()

	if err := os.WriteFile(filepath.Join(tmpDir, "single.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	files, err := provider.ListRecursive(ctx, "single.txt")
	if err != nil {
		t.Fatalf("ListRecursive failed: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	if files[0].Path != "single.txt" {
		t.Errorf("Path = %q, want %q", files[0].Path, "single.txt")
	}
}

func TestLocalProvider_Exists(t *testing.T) {
	tmpDir := t.TempDir()
	provider := NewLocalProvider(tmpDir)
	ctx := context.Background()

	// Non-existent file
	exists, err := provider.Exists(ctx, "nope.txt")
	if err != nil {
		t.Fatalf("Exists error: %v", err)
	}
	if exists {
		t.Error("expected false for non-existent file")
	}

	// Create and check
	if err := os.WriteFile(filepath.Join(tmpDir, "yes.txt"), []byte("here"), 0o600); err != nil {
		t.Fatal(err)
	}
	exists, err = provider.Exists(ctx, "yes.txt")
	if err != nil {
		t.Fatalf("Exists error: %v", err)
	}
	if !exists {
		t.Error("expected true for existing file")
	}
}

func TestLocalProvider_Delete_NonExistent(t *testing.T) {
	provider := NewLocalProvider(t.TempDir())
	ctx := context.Background()

	// Delete non-existent file — should be idempotent (no error)
	err := provider.Delete(ctx, "ghost.txt")
	if err != nil {
		t.Errorf("expected nil error for non-existent delete, got: %v", err)
	}
}

func TestLocalProvider_resolvePath(t *testing.T) {
	tests := []struct {
		name     string
		basePath string
		path     string
		want     string
	}{
		{
			name:     "no base path",
			basePath: "",
			path:     "relative/file.txt",
			want:     "relative/file.txt",
		},
		{
			name:     "with base path",
			basePath: "/data",
			path:     "file.txt",
			want:     "/data/file.txt",
		},
		{
			name:     "absolute path overrides base",
			basePath: "/data",
			path:     "/absolute/file.txt",
			want:     "/absolute/file.txt",
		},
		{
			name:     "nested relative with base",
			basePath: "/data",
			path:     "sub/dir/file.txt",
			want:     "/data/sub/dir/file.txt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewLocalProvider(tt.basePath)
			got := p.resolvePath(tt.path)
			if got != tt.want {
				t.Errorf("resolvePath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestLocalProvider_Put_ReadError(t *testing.T) {
	provider := NewLocalProvider(t.TempDir())
	ctx := context.Background()

	// Use a reader that always fails
	err := provider.Put(ctx, "fail.txt", &errorReader{}, -1)
	if err == nil {
		t.Fatal("expected error from failing reader")
	}
	if !strings.Contains(err.Error(), "failed to write file content") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestLocalProvider_Put_GzipReadError(t *testing.T) {
	provider := NewLocalProvider(t.TempDir())
	ctx := context.Background()

	// Use a reader that always fails, but with .gz extension
	err := provider.Put(ctx, "fail.txt.gz", &errorReader{}, -1)
	if err == nil {
		t.Fatal("expected error from failing reader on gzip write")
	}
	if !strings.Contains(err.Error(), "failed to write gzip content") {
		t.Errorf("unexpected error: %v", err)
	}
}

// errorReader is an io.Reader that always returns an error.
type errorReader struct{}

func (e *errorReader) Read([]byte) (int, error) {
	return 0, io.ErrUnexpectedEOF
}
