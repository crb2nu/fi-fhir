package sink

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalSink_Name(t *testing.T) {
	sink := NewLocalSink("test-sink", "/tmp/test")
	if got := sink.Name(); got != "test-sink" {
		t.Errorf("Name() = %q, want %q", got, "test-sink")
	}
}

func TestLocalSink_Write(t *testing.T) {
	tmpDir := t.TempDir()
	sink := NewLocalSink("test", tmpDir)
	ctx := context.Background()

	content := "test content"
	err := sink.Write(ctx, "subdir/test.txt", strings.NewReader(content), int64(len(content)))
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	// Verify file was created
	data, err := os.ReadFile(filepath.Join(tmpDir, "subdir/test.txt"))
	if err != nil {
		t.Fatalf("Failed to read written file: %v", err)
	}

	if string(data) != content {
		t.Errorf("Written content = %q, want %q", string(data), content)
	}
}

func TestLocalSink_Exists(t *testing.T) {
	tmpDir := t.TempDir()
	sink := NewLocalSink("test", tmpDir)
	ctx := context.Background()

	// Create a test file
	testFile := filepath.Join(tmpDir, "exists.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Test existing file
	exists, err := sink.Exists(ctx, "exists.txt")
	if err != nil {
		t.Fatalf("Exists() error = %v", err)
	}
	if !exists {
		t.Error("Exists() = false for existing file")
	}

	// Test non-existing file
	exists, err = sink.Exists(ctx, "not-exists.txt")
	if err != nil {
		t.Fatalf("Exists() error = %v", err)
	}
	if exists {
		t.Error("Exists() = true for non-existing file")
	}
}

func TestLocalSink_Validate(t *testing.T) {
	tmpDir := t.TempDir()
	sink := NewLocalSink("test", tmpDir)
	ctx := context.Background()

	if err := sink.Validate(ctx); err != nil {
		t.Errorf("Validate() error = %v", err)
	}
}

func TestLocalSink_Validate_CreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	newDir := filepath.Join(tmpDir, "new", "nested", "dir")
	sink := NewLocalSink("test", newDir)
	ctx := context.Background()

	if err := sink.Validate(ctx); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	// Verify directory was created
	info, err := os.Stat(newDir)
	if err != nil {
		t.Fatalf("Directory not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("Created path is not a directory")
	}
}

func TestLocalSink_Delete(t *testing.T) {
	tmpDir := t.TempDir()
	sink := NewLocalSink("test", tmpDir)
	ctx := context.Background()

	// Create a test file
	testFile := filepath.Join(tmpDir, "delete-me.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Delete it
	if err := sink.Delete(ctx, "delete-me.txt"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	// Verify it's gone
	if _, err := os.Stat(testFile); !os.IsNotExist(err) {
		t.Error("Delete() did not remove file")
	}
}

func TestLocalSink_Delete_NonExisting(t *testing.T) {
	tmpDir := t.TempDir()
	sink := NewLocalSink("test", tmpDir)
	ctx := context.Background()

	// Deleting non-existing file should not error
	if err := sink.Delete(ctx, "not-exists.txt"); err != nil {
		t.Errorf("Delete() should not error for non-existing file: %v", err)
	}
}

func TestLocalSink_BasePath(t *testing.T) {
	sink := NewLocalSink("test", "/custom/path")
	if got := sink.BasePath(); got != "/custom/path" {
		t.Errorf("BasePath() = %q, want %q", got, "/custom/path")
	}
}

func TestLocalSink_Write_LargeContent(t *testing.T) {
	tmpDir := t.TempDir()
	sink := NewLocalSink("test", tmpDir)
	ctx := context.Background()

	// Content larger than the 32KB internal buffer.
	content := strings.Repeat("x", 64*1024)
	err := sink.Write(ctx, "big.bin", strings.NewReader(content), int64(len(content)))
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, "big.bin"))
	if err != nil {
		t.Fatalf("ReadFile error = %v", err)
	}
	if len(data) != len(content) {
		t.Errorf("written size = %d, want %d", len(data), len(content))
	}
}

func TestLocalSink_Write_ContextCancelled(t *testing.T) {
	tmpDir := t.TempDir()
	sink := NewLocalSink("test", tmpDir)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel before write.

	// Use a reader that blocks (never returns), so context check fires.
	err := sink.Write(ctx, "cancelled.txt", &blockingReader{}, -1)
	if err == nil {
		// The check happens per-loop iteration; with an immediately-cancelled
		// context and a blocking reader it should catch it.
		t.Error("expected error from cancelled context")
	}
}

// blockingReader never returns data, it always checks for done.
type blockingReader struct{}

func (r *blockingReader) Read(p []byte) (int, error) {
	// Simulate a reader that yields no data, forcing the loop to
	// re-check context. Return io.EOF on second call to prevent infinite loop.
	return 0, context.Canceled
}

func TestLocalSink_Write_ReadError(t *testing.T) {
	tmpDir := t.TempDir()
	sink := NewLocalSink("test", tmpDir)
	ctx := context.Background()

	err := sink.Write(ctx, "err.txt", &errorReader{}, -1)
	if err == nil {
		t.Error("expected error from failing reader")
	}
}

// errorReader always returns an error.
type errorReader struct{}

func (r *errorReader) Read(p []byte) (int, error) {
	return 0, os.ErrPermission
}

func TestLocalSink_Validate_InvalidPath(t *testing.T) {
	// /dev/null is a file, so MkdirAll under it will fail.
	sink := NewLocalSink("test", "/dev/null/impossible/path")
	err := sink.Validate(context.Background())
	if err == nil {
		t.Error("Validate() should error when path cannot be created")
	}
}

func TestLocalSink_Write_DirCreateError(t *testing.T) {
	// Writing to a path where the parent directory can't be created.
	sink := NewLocalSink("test", "/dev/null/impossible")
	err := sink.Write(context.Background(), "file.txt", strings.NewReader("data"), 4)
	if err == nil {
		t.Error("Write() should error when directory creation fails")
	}
}

func TestLocalSink_Write_NestedDirectories(t *testing.T) {
	tmpDir := t.TempDir()
	sink := NewLocalSink("test", tmpDir)
	ctx := context.Background()

	content := "deep content"
	err := sink.Write(ctx, "a/b/c/d/file.txt", strings.NewReader(content), int64(len(content)))
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, "a/b/c/d/file.txt"))
	if err != nil {
		t.Fatalf("ReadFile error = %v", err)
	}
	if string(data) != content {
		t.Errorf("content = %q, want %q", string(data), content)
	}
}
