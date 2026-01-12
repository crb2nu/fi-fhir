package etl

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"
)

// mockSource implements Source for testing.
type mockSource struct {
	name     string
	versions []VersionInfo
	data     map[string]string // version -> content
	err      error
}

func (m *mockSource) Name() string { return m.name }

func (m *mockSource) AvailableVersions(ctx context.Context) ([]VersionInfo, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.versions, nil
}

func (m *mockSource) Download(ctx context.Context, version string, w io.Writer) (int64, error) {
	if m.err != nil {
		return 0, m.err
	}
	data, ok := m.data[version]
	if !ok {
		return 0, io.ErrUnexpectedEOF
	}
	n, err := w.Write([]byte(data))
	return int64(n), err
}

func (m *mockSource) Validate(ctx context.Context) error {
	return m.err
}

// mockSink implements Sink for testing.
type mockSink struct {
	name    string
	storage map[string]string // path -> content
	err     error
}

func (m *mockSink) Name() string { return m.name }

func (m *mockSink) Write(ctx context.Context, path string, r io.Reader, size int64) error {
	if m.err != nil {
		return m.err
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	if m.storage == nil {
		m.storage = make(map[string]string)
	}
	m.storage[path] = string(data)
	return nil
}

func (m *mockSink) Exists(ctx context.Context, path string) (bool, error) {
	if m.err != nil {
		return false, m.err
	}
	_, ok := m.storage[path]
	return ok, nil
}

func (m *mockSink) Validate(ctx context.Context) error {
	return m.err
}

func TestPipeline_Run(t *testing.T) {
	source := &mockSource{
		name: "test-source",
		versions: []VersionInfo{
			{Version: "v1.0", IsLatest: true},
			{Version: "v0.9"},
		},
		data: map[string]string{
			"v1.0": "test data for v1.0",
			"v0.9": "test data for v0.9",
		},
	}

	sink := &mockSink{
		name:    "test-sink",
		storage: make(map[string]string),
	}

	opts := DefaultPipelineOptions()
	p := NewPipeline("test-pipeline", source, sink, opts)

	// Run with specific version
	run, err := p.Run(context.Background(), "v1.0")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if run.Status != RunStatusCompleted {
		t.Errorf("Status = %v, want %v", run.Status, RunStatusCompleted)
	}
	if run.Version != "v1.0" {
		t.Errorf("Version = %q, want %q", run.Version, "v1.0")
	}
	if run.BytesDownloaded != int64(len("test data for v1.0")) {
		t.Errorf("BytesDownloaded = %d, want %d", run.BytesDownloaded, len("test data for v1.0"))
	}

	// Verify data was written
	expectedPath := "test-source/v1.0/data"
	if data, ok := sink.storage[expectedPath]; !ok {
		t.Error("data not written to sink")
	} else if data != "test data for v1.0" {
		t.Errorf("sink data = %q, want %q", data, "test data for v1.0")
	}
}

func TestPipeline_RunLatestVersion(t *testing.T) {
	source := &mockSource{
		name: "test-source",
		versions: []VersionInfo{
			{Version: "v2.0", IsLatest: true},
			{Version: "v1.0"},
		},
		data: map[string]string{
			"v2.0": "latest data",
			"v1.0": "old data",
		},
	}

	sink := &mockSink{
		name:    "test-sink",
		storage: make(map[string]string),
	}

	opts := DefaultPipelineOptions()
	p := NewPipeline("test-pipeline", source, sink, opts)

	// Run without specifying version - should use latest
	run, err := p.Run(context.Background(), "")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if run.Version != "v2.0" {
		t.Errorf("Version = %q, want %q (latest)", run.Version, "v2.0")
	}
}

func TestPipeline_DryRun(t *testing.T) {
	source := &mockSource{
		name: "test-source",
		versions: []VersionInfo{
			{Version: "v1.0", IsLatest: true},
		},
		data: map[string]string{
			"v1.0": "test data",
		},
	}

	sink := &mockSink{
		name:    "test-sink",
		storage: make(map[string]string),
	}

	opts := DefaultPipelineOptions()
	opts.DryRun = true
	p := NewPipeline("test-pipeline", source, sink, opts)

	run, err := p.Run(context.Background(), "v1.0")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if run.Status != RunStatusCompleted {
		t.Errorf("Status = %v, want %v", run.Status, RunStatusCompleted)
	}

	// Verify no data was written
	if len(sink.storage) > 0 {
		t.Error("data was written in dry run mode")
	}
}

func TestPipeline_ExistingDestination(t *testing.T) {
	source := &mockSource{
		name: "test-source",
		versions: []VersionInfo{
			{Version: "v1.0", IsLatest: true},
		},
		data: map[string]string{
			"v1.0": "test data",
		},
	}

	sink := &mockSink{
		name: "test-sink",
		storage: map[string]string{
			"test-source/v1.0/data": "existing data",
		},
	}

	opts := DefaultPipelineOptions()
	opts.OverwriteExisting = false
	p := NewPipeline("test-pipeline", source, sink, opts)

	_, err := p.Run(context.Background(), "v1.0")
	if err == nil {
		t.Error("expected error for existing destination")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("error = %q, want to contain 'already exists'", err.Error())
	}
}

func TestPipeline_Timeout(t *testing.T) {
	// Source that blocks
	source := &mockSource{
		name: "slow-source",
		versions: []VersionInfo{
			{Version: "v1.0", IsLatest: true},
		},
	}

	// Override Download to block
	originalData := source.data
	source.data = nil // Will cause Download to fail immediately

	sink := &mockSink{
		name:    "test-sink",
		storage: make(map[string]string),
	}

	opts := DefaultPipelineOptions()
	opts.Timeout = 1 * time.Millisecond
	p := NewPipeline("test-pipeline", source, sink, opts)

	run, err := p.Run(context.Background(), "v1.0")
	if err == nil {
		t.Error("expected error for timeout/download failure")
	}
	if run.Status != RunStatusFailed {
		t.Errorf("Status = %v, want %v", run.Status, RunStatusFailed)
	}

	source.data = originalData // Restore
}

func TestManager_RegisterAndRun(t *testing.T) {
	source := &mockSource{
		name: "test-source",
		versions: []VersionInfo{
			{Version: "v1.0", IsLatest: true},
		},
		data: map[string]string{
			"v1.0": "test data",
		},
	}

	sink := &mockSink{
		name:    "test-sink",
		storage: make(map[string]string),
	}

	manager := NewManager()

	opts := DefaultPipelineOptions()
	p := NewPipeline("my-pipeline", source, sink, opts)
	manager.Register(p)

	// Verify registration
	names := manager.List()
	if len(names) != 1 || names[0] != "my-pipeline" {
		t.Errorf("List() = %v, want [my-pipeline]", names)
	}

	// Get pipeline
	got, ok := manager.Get("my-pipeline")
	if !ok {
		t.Error("Get(my-pipeline) returned false")
	}
	if got != p {
		t.Error("Get returned different pipeline")
	}

	// Run through manager
	run, err := manager.Run(context.Background(), "my-pipeline", "v1.0")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if run.Status != RunStatusCompleted {
		t.Errorf("Status = %v, want %v", run.Status, RunStatusCompleted)
	}
}

func TestManager_UnknownPipeline(t *testing.T) {
	manager := NewManager()

	_, err := manager.Run(context.Background(), "nonexistent", "v1.0")
	if err == nil {
		t.Error("expected error for unknown pipeline")
	}
}

func TestGenerateRunID(t *testing.T) {
	id1 := generateRunID()
	time.Sleep(1 * time.Nanosecond)
	id2 := generateRunID()

	if id1 == "" {
		t.Error("generateRunID returned empty string")
	}
	if id1 == id2 {
		t.Error("generateRunID returned duplicate IDs")
	}
	if !strings.HasPrefix(id1, "run-") {
		t.Errorf("ID = %q, want prefix 'run-'", id1)
	}
}
