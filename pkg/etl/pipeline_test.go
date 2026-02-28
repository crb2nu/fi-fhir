package etl

import (
	"context"
	"fmt"
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

func TestPipeline_Status_BeforeRun(t *testing.T) {
	source := &mockSource{name: "s"}
	sink := &mockSink{name: "k"}
	p := NewPipeline("test", source, sink, DefaultPipelineOptions())

	if status := p.Status(); status != nil {
		t.Errorf("Status() before run = %v, want nil", status)
	}
}

func TestPipeline_Status_AfterRun(t *testing.T) {
	source := &mockSource{
		name:     "s",
		versions: []VersionInfo{{Version: "v1", IsLatest: true}},
		data:     map[string]string{"v1": "data"},
	}
	sink := &mockSink{name: "k", storage: make(map[string]string)}
	p := NewPipeline("test", source, sink, DefaultPipelineOptions())

	_, _ = p.Run(context.Background(), "v1")

	status := p.Status()
	if status == nil {
		t.Fatal("Status() after run = nil")
	}
	if status.Status != RunStatusCompleted {
		t.Errorf("Status().Status = %v, want %v", status.Status, RunStatusCompleted)
	}
	if status.Version != "v1" {
		t.Errorf("Status().Version = %q, want %q", status.Version, "v1")
	}
}

func TestPipeline_ProgressCallback(t *testing.T) {
	source := &mockSource{
		name:     "s",
		versions: []VersionInfo{{Version: "v1", IsLatest: true}},
		data:     map[string]string{"v1": "data"},
	}
	sink := &mockSink{name: "k", storage: make(map[string]string)}

	var callbackCount int
	opts := DefaultPipelineOptions()
	opts.OnProgress = func(run *PipelineRun) {
		callbackCount++
	}
	p := NewPipeline("test", source, sink, opts)

	_, _ = p.Run(context.Background(), "v1")

	// Progress is called during execute (status=running) and after completion.
	if callbackCount < 2 {
		t.Errorf("progress callback called %d times, want >= 2", callbackCount)
	}
}

func TestPipeline_VersionFallback_NoLatestFlag(t *testing.T) {
	// When no version is marked IsLatest, the pipeline should fall back to
	// the first version in the slice.
	source := &mockSource{
		name: "s",
		versions: []VersionInfo{
			{Version: "v1"},
			{Version: "v2"},
		},
		data: map[string]string{"v1": "data1", "v2": "data2"},
	}
	sink := &mockSink{name: "k", storage: make(map[string]string)}
	p := NewPipeline("test", source, sink, DefaultPipelineOptions())

	run, err := p.Run(context.Background(), "")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if run.Version != "v1" {
		t.Errorf("Version = %q, want fallback to %q", run.Version, "v1")
	}
}

func TestPipeline_NoVersionsAvailable(t *testing.T) {
	source := &mockSource{
		name:     "empty-source",
		versions: []VersionInfo{},
		data:     map[string]string{},
	}
	sink := &mockSink{name: "k", storage: make(map[string]string)}
	p := NewPipeline("test", source, sink, DefaultPipelineOptions())

	_, err := p.Run(context.Background(), "")
	if err == nil {
		t.Error("expected error when no versions available")
	}
	if !strings.Contains(err.Error(), "no versions available") {
		t.Errorf("error = %q, want to contain 'no versions available'", err.Error())
	}
}

func TestPipeline_OverwriteExisting(t *testing.T) {
	source := &mockSource{
		name:     "s",
		versions: []VersionInfo{{Version: "v1", IsLatest: true}},
		data:     map[string]string{"v1": "new data"},
	}
	sink := &mockSink{
		name:    "k",
		storage: map[string]string{"s/v1/data": "old data"},
	}

	opts := DefaultPipelineOptions()
	opts.OverwriteExisting = true
	p := NewPipeline("test", source, sink, opts)

	run, err := p.Run(context.Background(), "v1")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if run.Status != RunStatusCompleted {
		t.Errorf("Status = %v, want %v", run.Status, RunStatusCompleted)
	}
	if sink.storage["s/v1/data"] != "new data" {
		t.Errorf("sink data = %q, want %q", sink.storage["s/v1/data"], "new data")
	}
}

func TestPipeline_SourceError(t *testing.T) {
	source := &mockSource{
		name:     "s",
		versions: []VersionInfo{{Version: "v1", IsLatest: true}},
		data:     map[string]string{"v1": "data"},
	}
	sink := &mockSink{
		name:    "k",
		storage: make(map[string]string),
		err:     fmt.Errorf("sink write failure"),
	}
	p := NewPipeline("test", source, sink, DefaultPipelineOptions())

	run, err := p.Run(context.Background(), "v1")
	if err == nil {
		t.Fatal("expected error from sink failure")
	}
	if run.Status != RunStatusFailed {
		t.Errorf("Status = %v, want %v", run.Status, RunStatusFailed)
	}
	if run.ErrorMessage == "" {
		t.Error("ErrorMessage should be populated on failure")
	}
}

func TestPipeline_RunMetadata(t *testing.T) {
	source := &mockSource{
		name:     "my-src",
		versions: []VersionInfo{{Version: "v1", IsLatest: true}},
		data:     map[string]string{"v1": "content"},
	}
	sink := &mockSink{name: "k", storage: make(map[string]string)}
	p := NewPipeline("my-pipeline", source, sink, DefaultPipelineOptions())

	run, err := p.Run(context.Background(), "v1")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if run.PipelineName != "my-pipeline" {
		t.Errorf("PipelineName = %q, want %q", run.PipelineName, "my-pipeline")
	}
	if run.SourceName != "my-src" {
		t.Errorf("SourceName = %q, want %q", run.SourceName, "my-src")
	}
	if run.DestinationPath != "my-src/v1/data" {
		t.Errorf("DestinationPath = %q, want %q", run.DestinationPath, "my-src/v1/data")
	}
	if run.CompletedAt == nil {
		t.Error("CompletedAt should be set")
	}
	if run.Duration == 0 {
		t.Error("Duration should be > 0")
	}
	if run.ID == "" {
		t.Error("ID should be populated")
	}
}

func TestNewPipeline_Defaults(t *testing.T) {
	source := &mockSource{name: "s"}
	sink := &mockSink{name: "k"}

	// Zero-value options should get filled with defaults.
	p := NewPipeline("test", source, sink, PipelineOptions{})

	if p.options.BufferSize == 0 {
		t.Error("BufferSize should default to non-zero")
	}
	if p.options.Timeout == 0 {
		t.Error("Timeout should default to non-zero")
	}
}

func TestManager_RunAll(t *testing.T) {
	source1 := &mockSource{
		name:     "src1",
		versions: []VersionInfo{{Version: "v1", IsLatest: true}},
		data:     map[string]string{"v1": "data1"},
	}
	source2 := &mockSource{
		name:     "src2",
		versions: []VersionInfo{{Version: "v1", IsLatest: true}},
		data:     map[string]string{"v1": "data2"},
	}

	sink := &mockSink{name: "k", storage: make(map[string]string)}

	manager := NewManager()
	manager.Register(NewPipeline("p1", source1, sink, DefaultPipelineOptions()))
	manager.Register(NewPipeline("p2", source2, sink, DefaultPipelineOptions()))

	results := manager.RunAll(context.Background(), "v1")

	if len(results) != 2 {
		t.Fatalf("RunAll() returned %d results, want 2", len(results))
	}

	for name, run := range results {
		if run.Status != RunStatusCompleted {
			t.Errorf("pipeline %q status = %v, want %v", name, run.Status, RunStatusCompleted)
		}
	}
}

func TestManager_RunAll_WithFailure(t *testing.T) {
	goodSource := &mockSource{
		name:     "good",
		versions: []VersionInfo{{Version: "v1", IsLatest: true}},
		data:     map[string]string{"v1": "ok"},
	}
	badSource := &mockSource{
		name:     "bad",
		versions: []VersionInfo{{Version: "v1", IsLatest: true}},
		data:     map[string]string{}, // v1 not in data → download will fail
	}

	sink := &mockSink{name: "k", storage: make(map[string]string)}

	manager := NewManager()
	manager.Register(NewPipeline("good-pipeline", goodSource, sink, DefaultPipelineOptions()))
	manager.Register(NewPipeline("bad-pipeline", badSource, sink, DefaultPipelineOptions()))

	results := manager.RunAll(context.Background(), "v1")

	if len(results) != 2 {
		t.Fatalf("RunAll() returned %d results, want 2", len(results))
	}

	if results["good-pipeline"].Status != RunStatusCompleted {
		t.Errorf("good-pipeline status = %v, want %v", results["good-pipeline"].Status, RunStatusCompleted)
	}
	if results["bad-pipeline"].Status != RunStatusFailed {
		t.Errorf("bad-pipeline status = %v, want %v", results["bad-pipeline"].Status, RunStatusFailed)
	}
}

func TestManager_Status(t *testing.T) {
	source := &mockSource{
		name:     "s",
		versions: []VersionInfo{{Version: "v1", IsLatest: true}},
		data:     map[string]string{"v1": "data"},
	}
	sink := &mockSink{name: "k", storage: make(map[string]string)}

	manager := NewManager()
	p := NewPipeline("p1", source, sink, DefaultPipelineOptions())
	manager.Register(p)

	// Before any run, status should exist but run should be nil.
	status := manager.Status()
	if len(status) != 1 {
		t.Fatalf("Status() returned %d entries, want 1", len(status))
	}
	if status["p1"] != nil {
		t.Error("Status() before run should be nil")
	}

	// After a run, status should reflect completion.
	_, _ = manager.Run(context.Background(), "p1", "v1")

	status = manager.Status()
	if status["p1"] == nil {
		t.Fatal("Status() after run should not be nil")
	}
	if status["p1"].Status != RunStatusCompleted {
		t.Errorf("status = %v, want %v", status["p1"].Status, RunStatusCompleted)
	}
}

func TestManager_Get_NotFound(t *testing.T) {
	manager := NewManager()
	_, ok := manager.Get("nonexistent")
	if ok {
		t.Error("Get() should return false for unknown pipeline")
	}
}
