// Package etl provides ETL (Extract-Transform-Load) pipeline orchestration
// for terminology data from various sources to MinIO storage and PostgreSQL.
package etl

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"
)

// Source defines the interface for data sources.
type Source interface {
	// Name returns the source identifier (e.g., "umls", "rxnorm", "loinc").
	Name() string

	// AvailableVersions returns versions available from this source.
	AvailableVersions(ctx context.Context) ([]VersionInfo, error)

	// Download streams the source data to the writer.
	// Returns the number of bytes written.
	Download(ctx context.Context, version string, w io.Writer) (int64, error)

	// Validate checks if the source is accessible.
	Validate(ctx context.Context) error
}

// Sink defines the interface for data destinations.
type Sink interface {
	// Name returns the sink identifier.
	Name() string

	// Write uploads data from the reader.
	Write(ctx context.Context, path string, r io.Reader, size int64) error

	// Exists checks if data already exists at the path.
	Exists(ctx context.Context, path string) (bool, error)

	// Validate checks if the sink is accessible.
	Validate(ctx context.Context) error
}

// VersionInfo describes an available version of terminology data.
type VersionInfo struct {
	Version     string    `json:"version"`
	ReleaseDate time.Time `json:"release_date"`
	DownloadURL string    `json:"download_url,omitempty"`
	FileSize    int64     `json:"file_size,omitempty"`
	Checksum    string    `json:"checksum,omitempty"`
	IsLatest    bool      `json:"is_latest"`
}

// RunStatus represents the status of a pipeline run.
type RunStatus string

const (
	RunStatusPending   RunStatus = "pending"
	RunStatusRunning   RunStatus = "running"
	RunStatusCompleted RunStatus = "completed"
	RunStatusFailed    RunStatus = "failed"
	RunStatusCancelled RunStatus = "cancelled"
)

// PipelineRun tracks the state of a single ETL execution.
type PipelineRun struct {
	ID              string        `json:"id"`
	PipelineName    string        `json:"pipeline_name"`
	SourceName      string        `json:"source_name"`
	Version         string        `json:"version"`
	Status          RunStatus     `json:"status"`
	StartedAt       time.Time     `json:"started_at"`
	CompletedAt     *time.Time    `json:"completed_at,omitempty"`
	BytesDownloaded int64         `json:"bytes_downloaded"`
	RowsLoaded      int64         `json:"rows_loaded"`
	ErrorMessage    string        `json:"error_message,omitempty"`
	DestinationPath string        `json:"destination_path,omitempty"`
	Duration        time.Duration `json:"duration,omitempty"`
}

// ProgressCallback is called periodically during pipeline execution.
type ProgressCallback func(run *PipelineRun)

// Pipeline orchestrates data extraction, transformation, and loading.
type Pipeline struct {
	name    string
	source  Source
	sink    Sink
	options PipelineOptions

	mu         sync.Mutex
	currentRun *PipelineRun
}

// PipelineOptions configures pipeline behavior.
type PipelineOptions struct {
	// BufferSize is the size of the transfer buffer in bytes.
	BufferSize int

	// Timeout is the maximum time for the entire pipeline run.
	Timeout time.Duration

	// RetryCount is the number of times to retry on transient failures.
	RetryCount int

	// RetryDelay is the initial delay between retries.
	RetryDelay time.Duration

	// OnProgress is called periodically during execution.
	OnProgress ProgressCallback

	// OverwriteExisting allows overwriting existing files.
	OverwriteExisting bool

	// DryRun simulates the pipeline without actually transferring data.
	DryRun bool
}

// DefaultPipelineOptions returns sensible defaults.
func DefaultPipelineOptions() PipelineOptions {
	return PipelineOptions{
		BufferSize: 32 * 1024, // 32KB buffer
		Timeout:    30 * time.Minute,
		RetryCount: 3,
		RetryDelay: 5 * time.Second,
	}
}

// NewPipeline creates a new ETL pipeline.
func NewPipeline(name string, source Source, sink Sink, opts PipelineOptions) *Pipeline {
	if opts.BufferSize == 0 {
		opts.BufferSize = 32 * 1024
	}
	if opts.Timeout == 0 {
		opts.Timeout = 30 * time.Minute
	}

	return &Pipeline{
		name:    name,
		source:  source,
		sink:    sink,
		options: opts,
	}
}

// Run executes the pipeline for the specified version.
// If version is empty, it fetches the latest available version.
func (p *Pipeline) Run(ctx context.Context, version string) (*PipelineRun, error) {
	p.mu.Lock()
	if p.currentRun != nil && p.currentRun.Status == RunStatusRunning {
		p.mu.Unlock()
		return nil, fmt.Errorf("pipeline %s is already running", p.name)
	}

	run := &PipelineRun{
		ID:           generateRunID(),
		PipelineName: p.name,
		SourceName:   p.source.Name(),
		Version:      version,
		Status:       RunStatusPending,
		StartedAt:    time.Now(),
	}
	p.currentRun = run
	p.mu.Unlock()

	// Create timeout context
	ctx, cancel := context.WithTimeout(ctx, p.options.Timeout)
	defer cancel()

	// Execute pipeline
	err := p.execute(ctx, run)

	// Update final status
	p.mu.Lock()
	now := time.Now()
	run.CompletedAt = &now
	run.Duration = now.Sub(run.StartedAt)
	if err != nil {
		run.Status = RunStatusFailed
		run.ErrorMessage = err.Error()
	} else {
		run.Status = RunStatusCompleted
	}
	p.mu.Unlock()

	if p.options.OnProgress != nil {
		p.options.OnProgress(run)
	}

	if err != nil {
		return run, err
	}
	return run, nil
}

// execute performs the actual ETL work.
func (p *Pipeline) execute(ctx context.Context, run *PipelineRun) error {
	// Resolve version if not specified
	if run.Version == "" {
		versions, err := p.source.AvailableVersions(ctx)
		if err != nil {
			return fmt.Errorf("failed to get available versions: %w", err)
		}
		if len(versions) == 0 {
			return fmt.Errorf("no versions available from source %s", p.source.Name())
		}
		// Find latest
		for _, v := range versions {
			if v.IsLatest {
				run.Version = v.Version
				break
			}
		}
		if run.Version == "" {
			run.Version = versions[0].Version // Fallback to first
		}
	}

	// Update status
	p.mu.Lock()
	run.Status = RunStatusRunning
	p.mu.Unlock()

	if p.options.OnProgress != nil {
		p.options.OnProgress(run)
	}

	// Build destination path
	destPath := fmt.Sprintf("%s/%s/data", p.source.Name(), run.Version)
	run.DestinationPath = destPath

	// Check if already exists
	if !p.options.OverwriteExisting {
		exists, err := p.sink.Exists(ctx, destPath)
		if err != nil {
			return fmt.Errorf("failed to check destination: %w", err)
		}
		if exists {
			return fmt.Errorf("destination already exists: %s (use --overwrite to replace)", destPath)
		}
	}

	// Dry run - stop here
	if p.options.DryRun {
		return nil
	}

	// Create pipe for streaming
	pr, pw := io.Pipe()

	// Download in goroutine
	var downloadErr error
	var bytesDownloaded int64
	downloadDone := make(chan struct{})

	go func() {
		defer close(downloadDone)
		defer pw.Close() //nolint:errcheck // Error passed through pipe
		bytesDownloaded, downloadErr = p.source.Download(ctx, run.Version, pw)
	}()

	// Upload from pipe
	err := p.sink.Write(ctx, destPath, pr, -1) // Unknown size
	if err != nil {
		_ = pr.Close()
		<-downloadDone // Wait for download to finish
		return fmt.Errorf("failed to write to sink: %w", err)
	}

	<-downloadDone

	if downloadErr != nil {
		return fmt.Errorf("failed to download from source: %w", downloadErr)
	}

	// Update bytes downloaded
	p.mu.Lock()
	run.BytesDownloaded = bytesDownloaded
	p.mu.Unlock()

	return nil
}

// Status returns the current run status, if any.
func (p *Pipeline) Status() *PipelineRun {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.currentRun
}

// generateRunID creates a unique run identifier.
func generateRunID() string {
	return fmt.Sprintf("run-%d", time.Now().UnixNano())
}

// Manager coordinates multiple pipelines.
type Manager struct {
	pipelines map[string]*Pipeline
	mu        sync.RWMutex
}

// NewManager creates a pipeline manager.
func NewManager() *Manager {
	return &Manager{
		pipelines: make(map[string]*Pipeline),
	}
}

// Register adds a pipeline to the manager.
func (m *Manager) Register(p *Pipeline) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pipelines[p.name] = p
}

// Get retrieves a pipeline by name.
func (m *Manager) Get(name string) (*Pipeline, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p, ok := m.pipelines[name]
	return p, ok
}

// List returns all registered pipeline names.
func (m *Manager) List() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	names := make([]string, 0, len(m.pipelines))
	for name := range m.pipelines {
		names = append(names, name)
	}
	return names
}

// Run executes a pipeline by name.
func (m *Manager) Run(ctx context.Context, name, version string) (*PipelineRun, error) {
	p, ok := m.Get(name)
	if !ok {
		return nil, fmt.Errorf("unknown pipeline: %s", name)
	}
	return p.Run(ctx, version)
}

// RunAll executes all registered pipelines.
func (m *Manager) RunAll(ctx context.Context, version string) map[string]*PipelineRun {
	results := make(map[string]*PipelineRun)
	for name, p := range m.pipelines {
		run, err := p.Run(ctx, version)
		if err != nil && run == nil {
			run = &PipelineRun{
				PipelineName: name,
				Status:       RunStatusFailed,
				ErrorMessage: err.Error(),
			}
		}
		results[name] = run
	}
	return results
}

// Status returns status for all pipelines.
func (m *Manager) Status() map[string]*PipelineRun {
	m.mu.RLock()
	defer m.mu.RUnlock()

	status := make(map[string]*PipelineRun)
	for name, p := range m.pipelines {
		status[name] = p.Status()
	}
	return status
}
