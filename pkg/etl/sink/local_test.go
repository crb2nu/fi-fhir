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
