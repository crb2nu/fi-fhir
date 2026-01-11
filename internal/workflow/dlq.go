package workflow

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// FailedEvent represents an event that failed processing.
type FailedEvent struct {
	// ID is a unique identifier for this failed event
	ID string `json:"id"`

	// Event is the original event that failed
	Event interface{} `json:"event"`

	// RouteName is the route where the failure occurred
	RouteName string `json:"route_name"`

	// ActionType is the action type that failed (e.g., "fhir", "webhook")
	ActionType string `json:"action_type"`

	// Error is the error message
	Error string `json:"error"`

	// ErrorType categorizes the error (e.g., "circuit_open", "timeout", "server_error")
	ErrorType string `json:"error_type"`

	// Attempts is the number of times this event has been attempted
	Attempts int `json:"attempts"`

	// FirstFailure is when the event first failed
	FirstFailure time.Time `json:"first_failure"`

	// LastFailure is when the event most recently failed
	LastFailure time.Time `json:"last_failure"`

	// Metadata holds additional context (e.g., HTTP status code, circuit breaker state)
	Metadata map[string]string `json:"metadata,omitempty"`
}

// DeadLetterQueue defines the interface for storing failed events.
type DeadLetterQueue interface {
	// Push adds a failed event to the queue
	Push(event *FailedEvent) error

	// Pop retrieves and removes the oldest failed event
	Pop() (*FailedEvent, error)

	// Peek retrieves the oldest failed event without removing it
	Peek() (*FailedEvent, error)

	// List returns all failed events (up to limit, 0 = all)
	List(limit int) ([]*FailedEvent, error)

	// Get retrieves a specific failed event by ID
	Get(id string) (*FailedEvent, error)

	// Remove removes a specific failed event by ID
	Remove(id string) error

	// Len returns the number of events in the queue
	Len() int

	// Clear removes all events from the queue
	Clear() error
}

// MemoryDLQ is an in-memory dead letter queue implementation.
type MemoryDLQ struct {
	mu     sync.RWMutex
	events []*FailedEvent
	byID   map[string]*FailedEvent
}

// NewMemoryDLQ creates a new in-memory dead letter queue.
func NewMemoryDLQ() *MemoryDLQ {
	return &MemoryDLQ{
		events: make([]*FailedEvent, 0),
		byID:   make(map[string]*FailedEvent),
	}
}

// Push adds a failed event to the queue.
func (q *MemoryDLQ) Push(event *FailedEvent) error {
	if event == nil {
		return errors.New("cannot push nil event")
	}
	if event.ID == "" {
		return errors.New("event must have an ID")
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	// Check if event already exists (update it)
	if existing, ok := q.byID[event.ID]; ok {
		existing.Attempts = event.Attempts
		existing.LastFailure = event.LastFailure
		existing.Error = event.Error
		existing.Metadata = event.Metadata
		return nil
	}

	q.events = append(q.events, event)
	q.byID[event.ID] = event
	return nil
}

// Pop retrieves and removes the oldest failed event.
func (q *MemoryDLQ) Pop() (*FailedEvent, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.events) == 0 {
		return nil, nil
	}

	event := q.events[0]
	q.events = q.events[1:]
	delete(q.byID, event.ID)
	return event, nil
}

// Peek retrieves the oldest failed event without removing it.
func (q *MemoryDLQ) Peek() (*FailedEvent, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	if len(q.events) == 0 {
		return nil, nil
	}

	return q.events[0], nil
}

// List returns all failed events.
func (q *MemoryDLQ) List(limit int) ([]*FailedEvent, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	if limit <= 0 || limit > len(q.events) {
		limit = len(q.events)
	}

	result := make([]*FailedEvent, limit)
	copy(result, q.events[:limit])
	return result, nil
}

// Get retrieves a specific failed event by ID.
func (q *MemoryDLQ) Get(id string) (*FailedEvent, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	event, ok := q.byID[id]
	if !ok {
		return nil, nil
	}
	return event, nil
}

// Remove removes a specific failed event by ID.
func (q *MemoryDLQ) Remove(id string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if _, ok := q.byID[id]; !ok {
		return nil // Not an error if doesn't exist
	}

	delete(q.byID, id)

	// Remove from slice
	for i, e := range q.events {
		if e.ID == id {
			q.events = append(q.events[:i], q.events[i+1:]...)
			break
		}
	}

	return nil
}

// Len returns the number of events in the queue.
func (q *MemoryDLQ) Len() int {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return len(q.events)
}

// Clear removes all events from the queue.
func (q *MemoryDLQ) Clear() error {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.events = make([]*FailedEvent, 0)
	q.byID = make(map[string]*FailedEvent)
	return nil
}

// FileDLQ is a file-based dead letter queue implementation.
// Each failed event is stored as a separate JSON file.
type FileDLQ struct {
	mu  sync.RWMutex
	dir string
}

// NewFileDLQ creates a new file-based dead letter queue.
func NewFileDLQ(dir string) (*FileDLQ, error) {
	// Create directory if it doesn't exist
	if err := os.MkdirAll(dir, 0755); err != nil { //nolint:gosec // G301: DLQ dir needs read access for debugging
		return nil, fmt.Errorf("failed to create DLQ directory: %w", err)
	}

	return &FileDLQ{dir: dir}, nil
}

// Push adds a failed event to the queue.
func (q *FileDLQ) Push(event *FailedEvent) error {
	if event == nil {
		return errors.New("cannot push nil event")
	}
	if event.ID == "" {
		return errors.New("event must have an ID")
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	data, err := json.MarshalIndent(event, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	filename := filepath.Join(q.dir, event.ID+".json")
	if err := os.WriteFile(filename, data, 0644); err != nil { //nolint:gosec // G306: DLQ files need read access
		return fmt.Errorf("failed to write event file: %w", err)
	}

	return nil
}

// Pop retrieves and removes the oldest failed event.
func (q *FileDLQ) Pop() (*FailedEvent, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	files, err := q.listFiles()
	if err != nil {
		return nil, err
	}

	if len(files) == 0 {
		return nil, nil
	}

	// Sort by modification time to get oldest
	oldest := files[0]
	event, err := q.readFile(oldest)
	if err != nil {
		return nil, err
	}

	// Remove the file
	if err := os.Remove(oldest); err != nil {
		return nil, fmt.Errorf("failed to remove event file: %w", err)
	}

	return event, nil
}

// Peek retrieves the oldest failed event without removing it.
func (q *FileDLQ) Peek() (*FailedEvent, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	files, err := q.listFiles()
	if err != nil {
		return nil, err
	}

	if len(files) == 0 {
		return nil, nil
	}

	return q.readFile(files[0])
}

// List returns all failed events.
func (q *FileDLQ) List(limit int) ([]*FailedEvent, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	files, err := q.listFiles()
	if err != nil {
		return nil, err
	}

	if limit <= 0 || limit > len(files) {
		limit = len(files)
	}

	events := make([]*FailedEvent, 0, limit)
	for _, file := range files[:limit] {
		event, err := q.readFile(file)
		if err != nil {
			continue // Skip files that can't be read
		}
		events = append(events, event)
	}

	return events, nil
}

// Get retrieves a specific failed event by ID.
func (q *FileDLQ) Get(id string) (*FailedEvent, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	filename := filepath.Join(q.dir, id+".json")
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		return nil, nil
	}

	return q.readFile(filename)
}

// Remove removes a specific failed event by ID.
func (q *FileDLQ) Remove(id string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	filename := filepath.Join(q.dir, id+".json")
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		return nil // Not an error if doesn't exist
	}

	return os.Remove(filename)
}

// Len returns the number of events in the queue.
func (q *FileDLQ) Len() int {
	q.mu.RLock()
	defer q.mu.RUnlock()

	files, err := q.listFiles()
	if err != nil {
		return 0
	}
	return len(files)
}

// Clear removes all events from the queue.
func (q *FileDLQ) Clear() error {
	q.mu.Lock()
	defer q.mu.Unlock()

	files, err := q.listFiles()
	if err != nil {
		return err
	}

	for _, file := range files {
		if err := os.Remove(file); err != nil {
			return fmt.Errorf("failed to remove file %s: %w", file, err)
		}
	}

	return nil
}

// listFiles returns all JSON files in the DLQ directory sorted by modification time.
func (q *FileDLQ) listFiles() ([]string, error) {
	entries, err := os.ReadDir(q.dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read DLQ directory: %w", err)
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
			path:    filepath.Join(q.dir, entry.Name()),
			modTime: info.ModTime(),
		})
	}

	// Sort by modification time (oldest first)
	for i := 0; i < len(files)-1; i++ {
		for j := i + 1; j < len(files); j++ {
			if files[j].modTime.Before(files[i].modTime) {
				files[i], files[j] = files[j], files[i]
			}
		}
	}

	result := make([]string, len(files))
	for i, f := range files {
		result[i] = f.path
	}

	return result, nil
}

// readFile reads a FailedEvent from a JSON file.
func (q *FileDLQ) readFile(filename string) (*FailedEvent, error) {
	data, err := os.ReadFile(filename) //nolint:gosec // G304: filename constructed from controlled DLQ directory
	if err != nil {
		return nil, fmt.Errorf("failed to read event file: %w", err)
	}

	var event FailedEvent
	if err := json.Unmarshal(data, &event); err != nil {
		return nil, fmt.Errorf("failed to unmarshal event: %w", err)
	}

	return &event, nil
}

// DLQConfig holds configuration for DLQ behavior.
type DLQConfig struct {
	// MaxAttempts is the maximum number of reprocessing attempts before giving up
	MaxAttempts int

	// RetentionPeriod is how long to keep failed events (0 = forever)
	RetentionPeriod time.Duration

	// OnDeadLetter is called when an event is moved to DLQ (for alerting)
	OnDeadLetter func(event *FailedEvent)
}

// DefaultDLQConfig returns sensible defaults for DLQ configuration.
func DefaultDLQConfig() DLQConfig {
	return DLQConfig{
		MaxAttempts:     5,
		RetentionPeriod: 7 * 24 * time.Hour, // 7 days
	}
}

// ClassifyError categorizes an error for DLQ tracking.
func ClassifyError(err error) string {
	if err == nil {
		return ""
	}

	errStr := err.Error()

	// Circuit breaker errors
	if errors.Is(err, ErrCircuitOpen) {
		return "circuit_open"
	}

	// Timeout errors
	if containsAny(errStr, "timeout", "deadline exceeded", "context deadline") {
		return "timeout"
	}

	// Connection errors
	if containsAny(errStr, "connection refused", "no such host", "network unreachable", "connection reset") {
		return "connection_error"
	}

	// Authentication errors
	if containsAny(errStr, "401", "unauthorized", "authentication failed") {
		return "auth_error"
	}

	// Server errors (5xx)
	if containsAny(errStr, "500", "502", "503", "504", "server error") {
		return "server_error"
	}

	// Client errors (4xx)
	if containsAny(errStr, "400", "404", "422", "bad request", "not found") {
		return "client_error"
	}

	return "unknown"
}

// containsAny checks if s contains any of the substrings.
func containsAny(s string, substrs ...string) bool {
	for _, substr := range substrs {
		if len(s) >= len(substr) {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
		}
	}
	return false
}

// GenerateFailedEventID creates a unique ID for a failed event.
func GenerateFailedEventID() string {
	// Use timestamp + random suffix for uniqueness
	b := make([]byte, 8)
	_, _ = rand.Read(b) // crypto/rand.Read very rarely fails
	return fmt.Sprintf("%d-%s", time.Now().UnixNano(), hex.EncodeToString(b))
}
