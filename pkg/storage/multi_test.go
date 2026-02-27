package storage

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMultiProvider_PlainPath(t *testing.T) {
	tmpDir := t.TempDir()
	local := NewLocalProvider(tmpDir)
	multi := NewMultiProvider(local, nil)
	ctx := context.Background()

	content := "plain path content"
	if err := os.WriteFile(filepath.Join(tmpDir, "plain.txt"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	// Open via plain path (no scheme)
	r, err := multi.Open(ctx, "plain.txt")
	if err != nil {
		t.Fatalf("Open plain path failed: %v", err)
	}
	defer func() { _ = r.Close() }()

	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}
	if string(data) != content {
		t.Errorf("content = %q, want %q", string(data), content)
	}
}

func TestMultiProvider_Stat(t *testing.T) {
	tmpDir := t.TempDir()
	local := NewLocalProvider(tmpDir)
	multi := NewMultiProvider(local, nil)
	ctx := context.Background()

	if err := os.WriteFile(filepath.Join(tmpDir, "stat.txt"), []byte("stat me"), 0o600); err != nil {
		t.Fatal(err)
	}

	info, err := multi.Stat(ctx, "stat.txt")
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}
	if info.Size != 7 {
		t.Errorf("Size = %d, want 7", info.Size)
	}
}

func TestMultiProvider_List(t *testing.T) {
	tmpDir := t.TempDir()
	local := NewLocalProvider(tmpDir)
	multi := NewMultiProvider(local, nil)
	ctx := context.Background()

	for _, name := range []string{"x.txt", "y.txt"} {
		if err := os.WriteFile(filepath.Join(tmpDir, name), []byte("data"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	files, err := multi.List(ctx, "")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(files) != 2 {
		t.Errorf("expected 2 files, got %d", len(files))
	}
}

func TestMultiProvider_ListRecursive(t *testing.T) {
	tmpDir := t.TempDir()
	local := NewLocalProvider(tmpDir)
	multi := NewMultiProvider(local, nil)
	ctx := context.Background()

	// Create nested files
	nested := filepath.Join(tmpDir, "sub")
	if err := os.MkdirAll(nested, 0o750); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"top.txt", "sub/nested.txt"} {
		if err := os.WriteFile(filepath.Join(tmpDir, name), []byte("data"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	files, err := multi.ListRecursive(ctx, "")
	if err != nil {
		t.Fatalf("ListRecursive failed: %v", err)
	}
	if len(files) < 2 {
		t.Errorf("expected at least 2 entries, got %d", len(files))
	}
}

func TestMultiProvider_Put(t *testing.T) {
	tmpDir := t.TempDir()
	local := NewLocalProvider(tmpDir)
	multi := NewMultiProvider(local, nil)
	ctx := context.Background()

	content := "put via multi"
	err := multi.Put(ctx, "multi-put.txt", strings.NewReader(content), int64(len(content)))
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Verify file was written
	data, err := os.ReadFile(filepath.Join(tmpDir, "multi-put.txt"))
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if string(data) != content {
		t.Errorf("content = %q, want %q", string(data), content)
	}
}

func TestMultiProvider_Delete(t *testing.T) {
	tmpDir := t.TempDir()
	local := NewLocalProvider(tmpDir)
	multi := NewMultiProvider(local, nil)
	ctx := context.Background()

	testFile := filepath.Join(tmpDir, "delete-me.txt")
	if err := os.WriteFile(testFile, []byte("bye"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := multi.Delete(ctx, "delete-me.txt"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	if _, err := os.Stat(testFile); !os.IsNotExist(err) {
		t.Error("file should not exist after delete")
	}
}

func TestMultiProvider_Exists(t *testing.T) {
	tmpDir := t.TempDir()
	local := NewLocalProvider(tmpDir)
	multi := NewMultiProvider(local, nil)
	ctx := context.Background()

	// Non-existent
	exists, err := multi.Exists(ctx, "nope.txt")
	if err != nil {
		t.Fatalf("Exists error: %v", err)
	}
	if exists {
		t.Error("expected false for non-existent file")
	}

	// Existing
	if err := os.WriteFile(filepath.Join(tmpDir, "there.txt"), []byte("hi"), 0o600); err != nil {
		t.Fatal(err)
	}
	exists, err = multi.Exists(ctx, "there.txt")
	if err != nil {
		t.Fatalf("Exists error: %v", err)
	}
	if !exists {
		t.Error("expected true for existing file")
	}
}

func TestMultiProvider_S3Error(t *testing.T) {
	local := NewLocalProvider(t.TempDir())
	multi := NewMultiProvider(local, nil)
	ctx := context.Background()

	operations := []struct {
		name string
		fn   func() error
	}{
		{"Stat", func() error { _, err := multi.Stat(ctx, "s3://bucket/key"); return err }},
		{"List", func() error { _, err := multi.List(ctx, "s3://bucket/prefix"); return err }},
		{"ListRecursive", func() error { _, err := multi.ListRecursive(ctx, "s3://bucket/prefix"); return err }},
		{"Put", func() error { return multi.Put(ctx, "s3://bucket/key", strings.NewReader("x"), 1) }},
		{"Delete", func() error { return multi.Delete(ctx, "s3://bucket/key") }},
		{"Exists", func() error { _, err := multi.Exists(ctx, "s3://bucket/key"); return err }},
	}

	for _, op := range operations {
		t.Run(op.name, func(t *testing.T) {
			err := op.fn()
			if err == nil {
				t.Error("expected error for s3:// without MinIO")
			}
			if !strings.Contains(err.Error(), "MinIO provider not configured") {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestMultiProvider_MinioScheme(t *testing.T) {
	local := NewLocalProvider(t.TempDir())
	multi := NewMultiProvider(local, nil)
	ctx := context.Background()

	// minio:// should also fail without MinIO configured
	_, err := multi.Open(ctx, "minio://bucket/key")
	if err == nil {
		t.Error("expected error for minio:// without MinIO")
	}
	if !strings.Contains(err.Error(), "MinIO provider not configured") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestMultiProvider_UnknownScheme(t *testing.T) {
	local := NewLocalProvider(t.TempDir())
	multi := NewMultiProvider(local, nil)
	ctx := context.Background()

	_, err := multi.Open(ctx, "ftp://server/file")
	if err == nil {
		t.Error("expected error for unsupported scheme")
	}
	if !strings.Contains(err.Error(), "unsupported URL scheme") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestMultiProvider_NilLocal(t *testing.T) {
	multi := NewMultiProvider(nil, nil)
	ctx := context.Background()

	// Plain path should fail with nil local
	_, err := multi.Open(ctx, "some/path")
	if err == nil {
		t.Error("expected error with nil local provider")
	}
	if !strings.Contains(err.Error(), "local provider not configured") {
		t.Errorf("unexpected error: %v", err)
	}

	// file:// should also fail
	_, err = multi.Open(ctx, "file:///some/path")
	if err == nil {
		t.Error("expected error for file:// with nil local provider")
	}
	if !strings.Contains(err.Error(), "local provider not configured") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestIsS3URL(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"s3://bucket/key", true},
		{"s3://bucket", true},
		{"minio://bucket/key", true},
		{"minio://bucket", true},
		{"file:///path", false},
		{"/local/path", false},
		{"relative/path", false},
		{"", false},
		{"http://server", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := IsS3URL(tt.path)
			if got != tt.want {
				t.Errorf("IsS3URL(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestMultiProvider_FileURL_AllOps(t *testing.T) {
	tmpDir := t.TempDir()
	local := NewLocalProvider("")
	multi := NewMultiProvider(local, nil)
	ctx := context.Background()

	// Write a file directly
	testPath := filepath.Join(tmpDir, "fileurl.txt")
	if err := os.WriteFile(testPath, []byte("file url content"), 0o600); err != nil {
		t.Fatal(err)
	}

	fileURL := "file://" + testPath

	// Stat
	info, err := multi.Stat(ctx, fileURL)
	if err != nil {
		t.Fatalf("Stat via file:// failed: %v", err)
	}
	if info.Size != 16 {
		t.Errorf("Size = %d, want 16", info.Size)
	}

	// Exists
	exists, err := multi.Exists(ctx, fileURL)
	if err != nil {
		t.Fatalf("Exists via file:// failed: %v", err)
	}
	if !exists {
		t.Error("expected true for existing file via file://")
	}

	// Delete
	if err := multi.Delete(ctx, fileURL); err != nil {
		t.Fatalf("Delete via file:// failed: %v", err)
	}

	// Verify gone
	exists, err = multi.Exists(ctx, fileURL)
	if err != nil {
		t.Fatalf("Exists after delete failed: %v", err)
	}
	if exists {
		t.Error("expected false after delete via file://")
	}
}
