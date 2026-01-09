package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// RecordedEvent represents an event captured during processing with its metadata.
type RecordedEvent struct {
	// ID is a unique identifier for this recorded event
	ID string `json:"id"`

	// Event is the original event data
	Event interface{} `json:"event"`

	// EventType is the extracted event type (for filtering)
	EventType string `json:"event_type"`

	// Source is the extracted event source (for filtering)
	Source string `json:"source"`

	// RecordedAt is when the event was recorded
	RecordedAt time.Time `json:"recorded_at"`

	// ProcessingResult captures the outcome of processing (if available)
	ProcessingResult *RecordedResult `json:"processing_result,omitempty"`

	// Metadata holds additional context
	Metadata map[string]string `json:"metadata,omitempty"`
}

// RecordedResult captures the processing outcome for comparison.
type RecordedResult struct {
	// RouteMatches lists which routes matched
	RouteMatches []string `json:"route_matches"`

	// ActionsExecuted counts actions by route
	ActionsExecuted map[string]int `json:"actions_executed"`

	// HasErrors indicates if processing had any errors
	HasErrors bool `json:"has_errors"`

	// ErrorMessages captures error strings (for comparison)
	ErrorMessages []string `json:"error_messages,omitempty"`

	// ProcessingDuration is how long processing took
	ProcessingDuration time.Duration `json:"processing_duration_ns"`
}

// EventRecorder captures events for later replay.
type EventRecorder interface {
	// Record stores an event with optional processing result
	Record(event interface{}, result *Result) error

	// RecordWithMetadata stores an event with custom metadata
	RecordWithMetadata(event interface{}, result *Result, metadata map[string]string) error

	// List returns all recorded events (up to limit, 0 = all)
	List(limit int) ([]*RecordedEvent, error)

	// Get retrieves a specific recorded event by ID
	Get(id string) (*RecordedEvent, error)

	// Clear removes all recorded events
	Clear() error

	// Len returns the number of recorded events
	Len() int

	// Export writes all events to a JSON file
	Export(path string) error

	// Import loads events from a JSON file
	Import(path string) error
}

// MemoryRecorder is an in-memory event recorder.
type MemoryRecorder struct {
	mu       sync.RWMutex
	events   []*RecordedEvent
	byID     map[string]*RecordedEvent
	maxSize  int // 0 = unlimited
	idPrefix string
	counter  int64
}

// RecorderOption configures a MemoryRecorder.
type RecorderOption func(*MemoryRecorder)

// WithMaxSize limits the recorder to a maximum number of events (oldest dropped).
func WithMaxSize(max int) RecorderOption {
	return func(r *MemoryRecorder) {
		r.maxSize = max
	}
}

// WithIDPrefix sets a custom prefix for generated event IDs.
func WithIDPrefix(prefix string) RecorderOption {
	return func(r *MemoryRecorder) {
		r.idPrefix = prefix
	}
}

// NewMemoryRecorder creates a new in-memory event recorder.
func NewMemoryRecorder(opts ...RecorderOption) *MemoryRecorder {
	r := &MemoryRecorder{
		events:   make([]*RecordedEvent, 0),
		byID:     make(map[string]*RecordedEvent),
		idPrefix: "evt",
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Record stores an event with optional processing result.
func (r *MemoryRecorder) Record(event interface{}, result *Result) error {
	return r.RecordWithMetadata(event, result, nil)
}

// RecordWithMetadata stores an event with custom metadata.
func (r *MemoryRecorder) RecordWithMetadata(event interface{}, result *Result, metadata map[string]string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.counter++
	id := fmt.Sprintf("%s-%d-%d", r.idPrefix, time.Now().UnixNano(), r.counter)

	recorded := &RecordedEvent{
		ID:         id,
		Event:      event,
		EventType:  extractEventType(event),
		Source:     extractEventSource(event),
		RecordedAt: time.Now(),
		Metadata:   metadata,
	}

	if result != nil {
		recorded.ProcessingResult = resultToRecorded(result)
	}

	// Enforce max size by removing oldest
	if r.maxSize > 0 && len(r.events) >= r.maxSize {
		oldest := r.events[0]
		r.events = r.events[1:]
		delete(r.byID, oldest.ID)
	}

	r.events = append(r.events, recorded)
	r.byID[id] = recorded

	return nil
}

// List returns all recorded events.
func (r *MemoryRecorder) List(limit int) ([]*RecordedEvent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if limit <= 0 || limit > len(r.events) {
		limit = len(r.events)
	}

	result := make([]*RecordedEvent, limit)
	copy(result, r.events[:limit])
	return result, nil
}

// Get retrieves a specific recorded event by ID.
func (r *MemoryRecorder) Get(id string) (*RecordedEvent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	event, ok := r.byID[id]
	if !ok {
		return nil, nil
	}
	return event, nil
}

// Clear removes all recorded events.
func (r *MemoryRecorder) Clear() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.events = make([]*RecordedEvent, 0)
	r.byID = make(map[string]*RecordedEvent)
	return nil
}

// Len returns the number of recorded events.
func (r *MemoryRecorder) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.events)
}

// Export writes all events to a JSON file.
func (r *MemoryRecorder) Export(path string) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	data, err := json.MarshalIndent(r.events, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal events: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// Import loads events from a JSON file.
func (r *MemoryRecorder) Import(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	var events []*RecordedEvent
	if err := json.Unmarshal(data, &events); err != nil {
		return fmt.Errorf("failed to unmarshal events: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for _, event := range events {
		r.events = append(r.events, event)
		r.byID[event.ID] = event
	}

	return nil
}

// FileRecorder persists events to individual JSON files.
type FileRecorder struct {
	mu       sync.RWMutex
	dir      string
	idPrefix string
	counter  int64
}

// NewFileRecorder creates a new file-based event recorder.
func NewFileRecorder(dir string) (*FileRecorder, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}
	return &FileRecorder{
		dir:      dir,
		idPrefix: "evt",
	}, nil
}

// Record stores an event with optional processing result.
func (r *FileRecorder) Record(event interface{}, result *Result) error {
	return r.RecordWithMetadata(event, result, nil)
}

// RecordWithMetadata stores an event with custom metadata.
func (r *FileRecorder) RecordWithMetadata(event interface{}, result *Result, metadata map[string]string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.counter++
	id := fmt.Sprintf("%s-%d-%d", r.idPrefix, time.Now().UnixNano(), r.counter)

	recorded := &RecordedEvent{
		ID:         id,
		Event:      event,
		EventType:  extractEventType(event),
		Source:     extractEventSource(event),
		RecordedAt: time.Now(),
		Metadata:   metadata,
	}

	if result != nil {
		recorded.ProcessingResult = resultToRecorded(result)
	}

	data, err := json.MarshalIndent(recorded, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	filename := filepath.Join(r.dir, id+".json")
	if err := os.WriteFile(filename, data, 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// List returns all recorded events.
func (r *FileRecorder) List(limit int) ([]*RecordedEvent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	files, err := r.listFiles()
	if err != nil {
		return nil, err
	}

	if limit <= 0 || limit > len(files) {
		limit = len(files)
	}

	events := make([]*RecordedEvent, 0, limit)
	for _, file := range files[:limit] {
		event, err := r.readFile(file)
		if err != nil {
			continue
		}
		events = append(events, event)
	}

	return events, nil
}

// Get retrieves a specific recorded event by ID.
func (r *FileRecorder) Get(id string) (*RecordedEvent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	filename := filepath.Join(r.dir, id+".json")
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		return nil, nil
	}

	return r.readFile(filename)
}

// Clear removes all recorded events.
func (r *FileRecorder) Clear() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	files, err := r.listFiles()
	if err != nil {
		return err
	}

	for _, file := range files {
		if err := os.Remove(file); err != nil {
			return fmt.Errorf("failed to remove file: %w", err)
		}
	}

	return nil
}

// Len returns the number of recorded events.
func (r *FileRecorder) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	files, _ := r.listFiles()
	return len(files)
}

// Export writes all events to a single JSON file.
func (r *FileRecorder) Export(path string) error {
	events, err := r.List(0)
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(events, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal events: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// Import loads events from a JSON file.
func (r *FileRecorder) Import(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	var events []*RecordedEvent
	if err := json.Unmarshal(data, &events); err != nil {
		return fmt.Errorf("failed to unmarshal events: %w", err)
	}

	for _, event := range events {
		data, err := json.MarshalIndent(event, "", "  ")
		if err != nil {
			continue
		}
		filename := filepath.Join(r.dir, event.ID+".json")
		if err := os.WriteFile(filename, data, 0644); err != nil {
			return fmt.Errorf("failed to write file: %w", err)
		}
	}

	return nil
}

// listFiles returns all JSON files in the directory sorted by modification time.
func (r *FileRecorder) listFiles() ([]string, error) {
	entries, err := os.ReadDir(r.dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	type fileInfo struct {
		path    string
		modTime time.Time
	}

	files := make([]fileInfo, 0)
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		files = append(files, fileInfo{
			path:    filepath.Join(r.dir, entry.Name()),
			modTime: info.ModTime(),
		})
	}

	// Sort by modification time (oldest first)
	sort.Slice(files, func(i, j int) bool {
		return files[i].modTime.Before(files[j].modTime)
	})

	result := make([]string, len(files))
	for i, f := range files {
		result[i] = f.path
	}

	return result, nil
}

// readFile reads a RecordedEvent from a JSON file.
func (r *FileRecorder) readFile(filename string) (*RecordedEvent, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	var event RecordedEvent
	if err := json.Unmarshal(data, &event); err != nil {
		return nil, fmt.Errorf("failed to unmarshal event: %w", err)
	}

	return &event, nil
}

// EventReplayer replays recorded events through an engine.
type EventReplayer struct {
	engine   *Engine
	recorder EventRecorder
}

// NewEventReplayer creates a new event replayer.
func NewEventReplayer(engine *Engine, recorder EventRecorder) *EventReplayer {
	return &EventReplayer{
		engine:   engine,
		recorder: recorder,
	}
}

// ReplayConfig configures replay behavior.
type ReplayConfig struct {
	// Filter by event type (empty = all)
	EventTypes []string

	// Filter by source (empty = all)
	Sources []string

	// Limit number of events to replay (0 = all)
	Limit int

	// RealTime paces events according to recorded timestamps
	RealTime bool

	// StopOnError stops replay when an error occurs
	StopOnError bool

	// Callback is called for each replayed event
	Callback func(recorded *RecordedEvent, result *Result, diff *ReplayDiff)
}

// ReplaySummary summarizes a replay session.
type ReplaySummary struct {
	// TotalEvents is how many events were replayed
	TotalEvents int `json:"total_events"`

	// MatchedRouting is how many had same routing behavior
	MatchedRouting int `json:"matched_routing"`

	// DifferentRouting is how many had different routing
	DifferentRouting int `json:"different_routing"`

	// MatchedErrors is how many had same error state
	MatchedErrors int `json:"matched_errors"`

	// DifferentErrors is how many had different error state
	DifferentErrors int `json:"different_errors"`

	// TotalDuration is how long replay took
	TotalDuration time.Duration `json:"total_duration_ns"`

	// Diffs contains detailed differences for each event
	Diffs []*ReplayDiff `json:"diffs,omitempty"`
}

// ReplayDiff represents differences between original and replayed processing.
type ReplayDiff struct {
	// EventID is the recorded event ID
	EventID string `json:"event_id"`

	// RoutingMatch indicates if routing was the same
	RoutingMatch bool `json:"routing_match"`

	// ErrorMatch indicates if error state was the same
	ErrorMatch bool `json:"error_match"`

	// AddedRoutes are routes matched in replay but not original
	AddedRoutes []string `json:"added_routes,omitempty"`

	// RemovedRoutes are routes matched in original but not replay
	RemovedRoutes []string `json:"removed_routes,omitempty"`

	// OriginalErrors from original processing
	OriginalErrors []string `json:"original_errors,omitempty"`

	// ReplayErrors from replay processing
	ReplayErrors []string `json:"replay_errors,omitempty"`
}

// Replay processes all recorded events and compares results.
func (p *EventReplayer) Replay(ctx context.Context, config *ReplayConfig) (*ReplaySummary, error) {
	if config == nil {
		config = &ReplayConfig{}
	}

	events, err := p.recorder.List(config.Limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list events: %w", err)
	}

	summary := &ReplaySummary{
		Diffs: make([]*ReplayDiff, 0),
	}

	startTime := time.Now()
	var lastEventTime time.Time

	for _, recorded := range events {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		// Apply filters
		if !matchesFilter(recorded, config.EventTypes, config.Sources) {
			continue
		}

		// Real-time pacing
		if config.RealTime && !lastEventTime.IsZero() {
			delay := recorded.RecordedAt.Sub(lastEventTime)
			if delay > 0 {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(delay):
				}
			}
		}
		lastEventTime = recorded.RecordedAt

		// Process the event
		result := p.engine.ProcessWithContext(ctx, recorded.Event)

		// Compare results
		diff := compareResults(recorded, result)
		summary.TotalEvents++

		if diff.RoutingMatch {
			summary.MatchedRouting++
		} else {
			summary.DifferentRouting++
		}

		if diff.ErrorMatch {
			summary.MatchedErrors++
		} else {
			summary.DifferentErrors++
		}

		// Only keep diffs that show differences
		if !diff.RoutingMatch || !diff.ErrorMatch {
			summary.Diffs = append(summary.Diffs, diff)
		}

		// Call callback if configured
		if config.Callback != nil {
			config.Callback(recorded, result, diff)
		}

		// Stop on error if configured
		if config.StopOnError && result.HasErrors() {
			break
		}
	}

	summary.TotalDuration = time.Since(startTime)
	return summary, nil
}

// ReplayOne replays a single event by ID.
func (p *EventReplayer) ReplayOne(ctx context.Context, id string) (*Result, *ReplayDiff, error) {
	recorded, err := p.recorder.Get(id)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get event: %w", err)
	}
	if recorded == nil {
		return nil, nil, errors.New("event not found")
	}

	result := p.engine.ProcessWithContext(ctx, recorded.Event)
	diff := compareResults(recorded, result)

	return result, diff, nil
}

// RecordingEngine wraps an engine to automatically record events.
type RecordingEngine struct {
	*Engine
	recorder EventRecorder
}

// NewRecordingEngine creates an engine that records all processed events.
func NewRecordingEngine(workflow *Workflow, recorder EventRecorder) (*RecordingEngine, error) {
	engine, err := NewEngine(workflow)
	if err != nil {
		return nil, err
	}
	return &RecordingEngine{
		Engine:   engine,
		recorder: recorder,
	}, nil
}

// Process routes an event and records it.
func (e *RecordingEngine) Process(event interface{}) *Result {
	result := e.Engine.Process(event)
	e.recorder.Record(event, result)
	return result
}

// ProcessWithContext routes an event with tracing and records it.
func (e *RecordingEngine) ProcessWithContext(ctx context.Context, event interface{}) *Result {
	result := e.Engine.ProcessWithContext(ctx, event)
	e.recorder.Record(event, result)
	return result
}

// GetRecorder returns the underlying event recorder.
func (e *RecordingEngine) GetRecorder() EventRecorder {
	return e.recorder
}

// Helper functions

// extractEventType gets the event type from various formats.
func extractEventType(event interface{}) string {
	if m, ok := event.(map[string]interface{}); ok {
		if t, ok := m["type"].(string); ok {
			return t
		}
	}
	return ""
}

// extractEventSource gets the event source from various formats.
func extractEventSource(event interface{}) string {
	if m, ok := event.(map[string]interface{}); ok {
		if s, ok := m["source"].(string); ok {
			return s
		}
	}
	return ""
}

// resultToRecorded converts a Result to RecordedResult.
func resultToRecorded(result *Result) *RecordedResult {
	if result == nil {
		return nil
	}

	recorded := &RecordedResult{
		RouteMatches:    make([]string, 0),
		ActionsExecuted: make(map[string]int),
		HasErrors:       result.HasErrors(),
	}

	for _, rr := range result.RouteResults {
		if rr.Matched {
			recorded.RouteMatches = append(recorded.RouteMatches, rr.RouteName)
			recorded.ActionsExecuted[rr.RouteName] = rr.ActionsRun
		}
	}

	if recorded.HasErrors {
		recorded.ErrorMessages = make([]string, 0)
		for _, err := range result.AllErrors() {
			recorded.ErrorMessages = append(recorded.ErrorMessages, err.Error())
		}
	}

	return recorded
}

// matchesFilter checks if an event matches the filter criteria.
func matchesFilter(event *RecordedEvent, eventTypes, sources []string) bool {
	if len(eventTypes) > 0 {
		found := false
		for _, t := range eventTypes {
			if event.EventType == t {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	if len(sources) > 0 {
		found := false
		for _, s := range sources {
			if event.Source == s {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}

// compareResults compares original and replayed results.
func compareResults(recorded *RecordedEvent, result *Result) *ReplayDiff {
	diff := &ReplayDiff{
		EventID:      recorded.ID,
		RoutingMatch: true,
		ErrorMatch:   true,
	}

	original := recorded.ProcessingResult
	if original == nil {
		// No original result to compare
		return diff
	}

	// Compare routing
	originalRoutes := make(map[string]bool)
	for _, route := range original.RouteMatches {
		originalRoutes[route] = true
	}

	replayRoutes := make(map[string]bool)
	for _, rr := range result.RouteResults {
		if rr.Matched {
			replayRoutes[rr.RouteName] = true
		}
	}

	// Find added routes (in replay but not original)
	for route := range replayRoutes {
		if !originalRoutes[route] {
			diff.AddedRoutes = append(diff.AddedRoutes, route)
			diff.RoutingMatch = false
		}
	}

	// Find removed routes (in original but not replay)
	for route := range originalRoutes {
		if !replayRoutes[route] {
			diff.RemovedRoutes = append(diff.RemovedRoutes, route)
			diff.RoutingMatch = false
		}
	}

	// Compare errors
	if original.HasErrors != result.HasErrors() {
		diff.ErrorMatch = false
	}

	if original.HasErrors {
		diff.OriginalErrors = original.ErrorMessages
	}
	if result.HasErrors() {
		diff.ReplayErrors = make([]string, 0)
		for _, err := range result.AllErrors() {
			diff.ReplayErrors = append(diff.ReplayErrors, err.Error())
		}
	}

	return diff
}

// LoadRecordedEvents loads events from a JSON file (utility function).
func LoadRecordedEvents(path string) ([]*RecordedEvent, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	var events []*RecordedEvent
	if err := json.Unmarshal(data, &events); err != nil {
		return nil, fmt.Errorf("failed to unmarshal events: %w", err)
	}

	return events, nil
}

// SaveRecordedEvents saves events to a JSON file (utility function).
func SaveRecordedEvents(path string, events []*RecordedEvent) error {
	data, err := json.MarshalIndent(events, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal events: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}
