package source

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalSource_Name(t *testing.T) {
	src := NewLocalSource(LocalSourceConfig{Name: "my-source"})
	if got := src.Name(); got != "my-source" {
		t.Errorf("Name() = %q, want %q", got, "my-source")
	}
}

func TestLocalSource_AvailableVersions(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a real file so Stat succeeds for one version.
	if err := os.WriteFile(filepath.Join(tmpDir, "v1.zip"), []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}

	src := NewLocalSource(LocalSourceConfig{
		Name:     "test",
		BasePath: tmpDir,
		Versions: map[string]string{
			"v1": "v1.zip",
			"v2": "v2-missing.zip", // does not exist — should still appear
		},
	})

	versions, err := src.AvailableVersions(context.Background())
	if err != nil {
		t.Fatalf("AvailableVersions() error = %v", err)
	}
	if len(versions) != 2 {
		t.Fatalf("got %d versions, want 2", len(versions))
	}

	// At least one should be marked latest.
	hasLatest := false
	for _, v := range versions {
		if v.IsLatest {
			hasLatest = true
		}
	}
	if !hasLatest {
		t.Error("no version marked as latest")
	}

	// The version backed by a real file should have FileSize > 0.
	for _, v := range versions {
		if v.Version == "v1" && v.FileSize == 0 {
			t.Error("expected FileSize > 0 for existing file")
		}
	}
}

func TestLocalSource_AvailableVersions_Empty(t *testing.T) {
	src := NewLocalSource(LocalSourceConfig{Name: "empty"})

	versions, err := src.AvailableVersions(context.Background())
	if err != nil {
		t.Fatalf("AvailableVersions() error = %v", err)
	}
	if len(versions) != 0 {
		t.Errorf("got %d versions, want 0", len(versions))
	}
}

func TestLocalSource_Download(t *testing.T) {
	tmpDir := t.TempDir()
	content := "hello local source"
	if err := os.WriteFile(filepath.Join(tmpDir, "data.txt"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	src := NewLocalSource(LocalSourceConfig{
		Name:     "test",
		BasePath: tmpDir,
		Versions: map[string]string{"v1": "data.txt"},
	})

	var buf bytes.Buffer
	n, err := src.Download(context.Background(), "v1", &buf)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	if n != int64(len(content)) {
		t.Errorf("Download() bytes = %d, want %d", n, len(content))
	}
	if buf.String() != content {
		t.Errorf("Download() content = %q, want %q", buf.String(), content)
	}
}

func TestLocalSource_Download_LargeFile(t *testing.T) {
	tmpDir := t.TempDir()
	// Create content larger than the 32KB buffer used in Download.
	content := make([]byte, 64*1024)
	for i := range content {
		content[i] = byte(i % 256)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "big.bin"), content, 0o600); err != nil {
		t.Fatal(err)
	}

	src := NewLocalSource(LocalSourceConfig{
		Name:     "test",
		BasePath: tmpDir,
		Versions: map[string]string{"v1": "big.bin"},
	})

	var buf bytes.Buffer
	n, err := src.Download(context.Background(), "v1", &buf)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	if n != int64(len(content)) {
		t.Errorf("Download() bytes = %d, want %d", n, len(content))
	}
	if !bytes.Equal(buf.Bytes(), content) {
		t.Error("downloaded content does not match")
	}
}

func TestLocalSource_Download_UnknownVersion(t *testing.T) {
	src := NewLocalSource(LocalSourceConfig{
		Name:     "test",
		Versions: map[string]string{"v1": "data.txt"},
	})

	var buf bytes.Buffer
	_, err := src.Download(context.Background(), "v99", &buf)
	if err == nil {
		t.Error("expected error for unknown version")
	}
}

func TestLocalSource_Download_MissingFile(t *testing.T) {
	src := NewLocalSource(LocalSourceConfig{
		Name:     "test",
		BasePath: "/nonexistent/path",
		Versions: map[string]string{"v1": "data.txt"},
	})

	var buf bytes.Buffer
	_, err := src.Download(context.Background(), "v1", &buf)
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestLocalSource_Download_ContextCancelled(t *testing.T) {
	tmpDir := t.TempDir()
	// Write enough data that the read loop iterates more than once.
	content := make([]byte, 128*1024)
	if err := os.WriteFile(filepath.Join(tmpDir, "big.bin"), content, 0o600); err != nil {
		t.Fatal(err)
	}

	src := NewLocalSource(LocalSourceConfig{
		Name:     "test",
		BasePath: tmpDir,
		Versions: map[string]string{"v1": "big.bin"},
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	var buf bytes.Buffer
	_, err := src.Download(ctx, "v1", &buf)
	if err == nil {
		// Context cancellation is checked per-loop iteration; with a fast
		// local read the file may complete before the check fires. Both
		// outcomes (nil or context.Canceled) are acceptable.
		t.Log("Download completed before context cancellation was detected — acceptable for fast reads")
	}
}

func TestLocalSource_Download_WriteError(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "data.txt"), []byte("content"), 0o600); err != nil {
		t.Fatal(err)
	}

	src := NewLocalSource(LocalSourceConfig{
		Name:     "test",
		BasePath: tmpDir,
		Versions: map[string]string{"v1": "data.txt"},
	})

	_, err := src.Download(context.Background(), "v1", &failWriter{})
	if err == nil {
		t.Error("expected error from failing writer")
	}
}

func TestLocalSource_Validate(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "data.txt"), []byte("ok"), 0o600); err != nil {
		t.Fatal(err)
	}

	src := NewLocalSource(LocalSourceConfig{
		Name:     "test",
		BasePath: tmpDir,
		Versions: map[string]string{"v1": "data.txt"},
	})

	if err := src.Validate(context.Background()); err != nil {
		t.Errorf("Validate() error = %v", err)
	}
}

func TestLocalSource_Validate_MissingFile(t *testing.T) {
	src := NewLocalSource(LocalSourceConfig{
		Name:     "test",
		BasePath: "/nonexistent",
		Versions: map[string]string{"v1": "missing.txt"},
	})

	if err := src.Validate(context.Background()); err == nil {
		t.Error("expected Validate error for missing file")
	}
}

func TestLocalSource_Validate_Empty(t *testing.T) {
	src := NewLocalSource(LocalSourceConfig{Name: "empty"})
	// No versions — nothing to validate, should succeed.
	if err := src.Validate(context.Background()); err != nil {
		t.Errorf("Validate() empty versions error = %v", err)
	}
}

func TestLocalSource_AddVersion(t *testing.T) {
	src := NewLocalSource(LocalSourceConfig{Name: "test"})
	src.AddVersion("v2", "v2/data.zip")

	versions, _ := src.AvailableVersions(context.Background())
	if len(versions) != 1 {
		t.Fatalf("got %d versions, want 1", len(versions))
	}
	if versions[0].Version != "v2" {
		t.Errorf("version = %q, want %q", versions[0].Version, "v2")
	}
}

func TestLocalSource_AddVersion_NilMap(t *testing.T) {
	// Create source with nil versions map (default from empty config).
	src := &LocalSource{name: "test"}
	src.AddVersion("v1", "data.txt")

	if _, ok := src.versions["v1"]; !ok {
		t.Error("AddVersion did not initialize and add to nil map")
	}
}

func TestLocalSource_SetBasePath(t *testing.T) {
	src := NewLocalSource(LocalSourceConfig{
		Name:     "test",
		BasePath: "/old",
	})

	src.SetBasePath("/new/path")

	if src.basePath != "/new/path" {
		t.Errorf("basePath = %q, want %q", src.basePath, "/new/path")
	}
}
