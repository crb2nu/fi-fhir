package storage

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseS3URL(t *testing.T) {
	tests := []struct {
		name       string
		url        string
		wantBucket string
		wantKey    string
		wantErr    bool
	}{
		{
			name:       "valid s3 URL",
			url:        "s3://mybucket/path/to/file.txt",
			wantBucket: "mybucket",
			wantKey:    "path/to/file.txt",
		},
		{
			name:       "valid minio URL",
			url:        "minio://mybucket/path/to/file.txt",
			wantBucket: "mybucket",
			wantKey:    "path/to/file.txt",
		},
		{
			name:       "root level file",
			url:        "s3://mybucket/file.txt",
			wantBucket: "mybucket",
			wantKey:    "file.txt",
		},
		{
			name:       "bucket only",
			url:        "s3://mybucket/",
			wantBucket: "mybucket",
			wantKey:    "",
		},
		{
			name:    "invalid scheme",
			url:     "http://mybucket/file.txt",
			wantErr: true,
		},
		{
			name:    "missing bucket",
			url:     "s3:///file.txt",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bucket, key, err := ParseS3URL(tt.url)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if bucket != tt.wantBucket {
				t.Errorf("bucket = %q, want %q", bucket, tt.wantBucket)
			}
			if key != tt.wantKey {
				t.Errorf("key = %q, want %q", key, tt.wantKey)
			}
		})
	}
}

func TestFormatS3URL(t *testing.T) {
	tests := []struct {
		bucket string
		key    string
		want   string
	}{
		{"mybucket", "path/to/file.txt", "s3://mybucket/path/to/file.txt"},
		{"mybucket", "file.txt", "s3://mybucket/file.txt"},
		{"mybucket", "", "s3://mybucket/"},
	}

	for _, tt := range tests {
		got := FormatS3URL(tt.bucket, tt.key)
		if got != tt.want {
			t.Errorf("FormatS3URL(%q, %q) = %q, want %q", tt.bucket, tt.key, got, tt.want)
		}
	}
}

func TestLocalProvider_OpenReadWrite(t *testing.T) {
	tmpDir := t.TempDir()
	provider := NewLocalProvider(tmpDir)
	ctx := context.Background()

	// Write a file
	content := "Hello, World!"
	err := provider.Put(ctx, "test.txt", strings.NewReader(content), int64(len(content)))
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Read it back
	r, err := provider.Open(ctx, "test.txt")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
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

func TestLocalProvider_Stat(t *testing.T) {
	tmpDir := t.TempDir()
	provider := NewLocalProvider(tmpDir)
	ctx := context.Background()

	// Create a file
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test content"), 0o600); err != nil {
		t.Fatal(err)
	}

	info, err := provider.Stat(ctx, "test.txt")
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}

	if info.Path != "test.txt" {
		t.Errorf("Path = %q, want %q", info.Path, "test.txt")
	}
	if info.Size != 12 {
		t.Errorf("Size = %d, want %d", info.Size, 12)
	}
	if info.IsDir {
		t.Error("IsDir = true, want false")
	}
}

func TestLocalProvider_List(t *testing.T) {
	tmpDir := t.TempDir()
	provider := NewLocalProvider(tmpDir)
	ctx := context.Background()

	// Create some files
	for _, name := range []string{"a.txt", "b.txt", "c.txt"} {
		if err := os.WriteFile(filepath.Join(tmpDir, name), []byte("test"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	// Create a subdirectory
	if err := os.Mkdir(filepath.Join(tmpDir, "subdir"), 0o750); err != nil {
		t.Fatal(err)
	}

	files, err := provider.List(ctx, "")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(files) != 4 {
		t.Errorf("len(files) = %d, want 4", len(files))
	}

	// Check that we have both files and the directory
	var fileCount, dirCount int
	for _, f := range files {
		if f.IsDir {
			dirCount++
		} else {
			fileCount++
		}
	}

	if fileCount != 3 {
		t.Errorf("fileCount = %d, want 3", fileCount)
	}
	if dirCount != 1 {
		t.Errorf("dirCount = %d, want 1", dirCount)
	}
}

func TestLocalProvider_Delete(t *testing.T) {
	tmpDir := t.TempDir()
	provider := NewLocalProvider(tmpDir)
	ctx := context.Background()

	// Create a file
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Verify it exists
	exists, err := provider.Exists(ctx, "test.txt")
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if !exists {
		t.Error("file should exist before delete")
	}

	// Delete it
	if err := provider.Delete(ctx, "test.txt"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify it's gone
	exists, err = provider.Exists(ctx, "test.txt")
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if exists {
		t.Error("file should not exist after delete")
	}
}

func TestLocalProvider_GzipTransparency(t *testing.T) {
	tmpDir := t.TempDir()
	provider := NewLocalProvider(tmpDir)
	ctx := context.Background()

	// Write gzip content
	content := "This is compressed content that should be readable"
	err := provider.Put(ctx, "test.txt.gz", strings.NewReader(content), int64(len(content)))
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Read it back - should auto-decompress
	r, err := provider.Open(ctx, "test.txt.gz")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
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

func TestLocalProvider_CreateNestedDirectories(t *testing.T) {
	tmpDir := t.TempDir()
	provider := NewLocalProvider(tmpDir)
	ctx := context.Background()

	// Write to nested path
	content := "nested content"
	err := provider.Put(ctx, "a/b/c/file.txt", strings.NewReader(content), int64(len(content)))
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Verify the file was created
	exists, err := provider.Exists(ctx, "a/b/c/file.txt")
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if !exists {
		t.Error("nested file should exist")
	}
}

func TestMultiProvider_URLRouting(t *testing.T) {
	tmpDir := t.TempDir()
	local := NewLocalProvider(tmpDir)
	multi := NewMultiProvider(local, nil) // No MinIO for this test
	ctx := context.Background()

	// Write using file:// URL
	content := "test content"
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	// Read using file:// URL
	r, err := multi.Open(ctx, "file://"+testFile)
	if err != nil {
		t.Fatalf("Open file:// failed: %v", err)
	}
	defer func() { _ = r.Close() }()

	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}

	if string(data) != content {
		t.Errorf("content = %q, want %q", string(data), content)
	}

	// Try s3:// without MinIO configured - should error
	_, err = multi.Open(ctx, "s3://bucket/key")
	if err == nil {
		t.Error("expected error for s3:// without MinIO configured")
	}
}
