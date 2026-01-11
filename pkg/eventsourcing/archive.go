package eventsourcing

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// =============================================================================
// Retention Policy
// =============================================================================

// RetentionPolicy defines when events become eligible for archival or deletion.
// Healthcare systems have strict retention requirements (HIPAA: minimum 6 years),
// so this system defaults to conservative settings.
type RetentionPolicy struct {
	// DefaultRetention for all events if no specific rule matches.
	// Events younger than this are never archived/deleted.
	DefaultRetention time.Duration

	// EventTypeRetention overrides for specific event types.
	// Key is the event type name, value is the retention duration.
	// Example: {"audit_log": 3*365*24*time.Hour, "patient_created": 7*365*24*time.Hour}
	EventTypeRetention map[string]time.Duration

	// StreamPrefixRetention overrides for stream patterns.
	// Key is the stream prefix (e.g., "patient:", "claim:"), value is retention.
	// Uses prefix matching (longest match wins).
	StreamPrefixRetention map[string]time.Duration

	// MinRetention is the absolute minimum retention period (safety floor).
	// Even if other rules specify shorter retention, this minimum is enforced.
	// Defaults to 6 years (HIPAA minimum) if not set.
	MinRetention time.Duration
}

// DefaultRetentionPolicy returns a policy suitable for healthcare data.
// Defaults to 7 years retention with 6 year minimum (HIPAA compliance).
func DefaultRetentionPolicy() *RetentionPolicy {
	return &RetentionPolicy{
		DefaultRetention:      7 * 365 * 24 * time.Hour, // 7 years
		MinRetention:          6 * 365 * 24 * time.Hour, // 6 years (HIPAA minimum)
		EventTypeRetention:    make(map[string]time.Duration),
		StreamPrefixRetention: make(map[string]time.Duration),
	}
}

// GetRetention returns the retention duration for a specific event.
// Order of precedence: EventTypeRetention > StreamPrefixRetention > DefaultRetention.
// The result is always >= MinRetention.
func (p *RetentionPolicy) GetRetention(eventType, streamID string) time.Duration {
	retention := p.DefaultRetention

	// Check stream prefix (longest match wins)
	longestPrefix := ""
	for prefix, r := range p.StreamPrefixRetention {
		if strings.HasPrefix(streamID, prefix) && len(prefix) > len(longestPrefix) {
			longestPrefix = prefix
			retention = r
		}
	}

	// Check event type (highest priority)
	if r, ok := p.EventTypeRetention[eventType]; ok {
		retention = r
	}

	// Enforce minimum retention
	if retention < p.MinRetention {
		retention = p.MinRetention
	}

	return retention
}

// IsEligibleForArchival returns true if the event is old enough to archive.
func (p *RetentionPolicy) IsEligibleForArchival(event StoredEvent) bool {
	retention := p.GetRetention(event.EventType, event.StreamID)
	cutoff := time.Now().Add(-retention)
	return event.Timestamp.Before(cutoff)
}

// =============================================================================
// Archive Store Interface
// =============================================================================

// ArchiveStore is the interface for archive destinations.
// Implementations may write to files, S3, GCS, or other storage.
type ArchiveStore interface {
	// WriteEvents writes a batch of events to the archive.
	// Events should be written atomically if possible.
	WriteEvents(ctx context.Context, events []StoredEvent) error

	// Close finalizes the archive (flush buffers, close files, etc.).
	Close() error
}

// =============================================================================
// Archive Configuration and Progress
// =============================================================================

// ArchiveConfig configures an archive operation.
type ArchiveConfig struct {
	// Policy determines which events are eligible for archival.
	// If nil, DefaultRetentionPolicy() is used.
	Policy *RetentionPolicy

	// ArchiveStore is where archived events are written.
	// Required unless DryRun is true.
	ArchiveStore ArchiveStore

	// DeleteAfterArchive removes events from primary store after successful archive.
	// Use with extreme caution in production.
	DeleteAfterArchive bool

	// BatchSize for reading events (default: 1000).
	BatchSize int

	// Progress callback called after each batch.
	Progress func(*ArchiveProgress)

	// DryRun only counts and reports events without actually archiving.
	DryRun bool

	// BeforePosition only archives events before this position (0 = no limit).
	// Useful for archiving up to a known safe point.
	BeforePosition int64

	// BeforeTime only archives events before this timestamp.
	// If set, BeforePosition is ignored.
	BeforeTime *time.Time
}

// ArchiveProgress reports progress during an archive operation.
type ArchiveProgress struct {
	// EventsProcessed total events examined so far
	EventsProcessed int64

	// EventsArchived events actually written to archive
	EventsArchived int64

	// EventsSkipped events not eligible for archival
	EventsSkipped int64

	// EventsDeleted events removed from primary store
	EventsDeleted int64

	// CurrentPosition in the event stream
	CurrentPosition int64

	// Duration elapsed so far
	Duration time.Duration

	// EventsPerSecond processing rate
	EventsPerSecond float64

	// Complete indicates archival is finished
	Complete bool

	// Error if archival failed
	Error error
}

// ArchiveResult contains the final result of an archive operation.
type ArchiveResult struct {
	EventsProcessed int64
	EventsArchived  int64
	EventsSkipped   int64
	EventsDeleted   int64
	StartPosition   int64
	EndPosition     int64
	Duration        time.Duration
	EventsPerSecond float64
	DryRun          bool
	Error           error
}

// =============================================================================
// Event Archiver
// =============================================================================

// EventArchiver manages event archival and retention.
type EventArchiver struct {
	store EventStore
}

// NewEventArchiver creates a new archiver for the given event store.
func NewEventArchiver(store EventStore) *EventArchiver {
	return &EventArchiver{
		store: store,
	}
}

// Archive processes events according to the given configuration.
// Events matching the retention policy are written to the archive store.
// If DeleteAfterArchive is true, archived events are removed from the primary store.
func (a *EventArchiver) Archive(ctx context.Context, config *ArchiveConfig) (*ArchiveResult, error) {
	if config == nil {
		config = &ArchiveConfig{}
	}
	if config.Policy == nil {
		config.Policy = DefaultRetentionPolicy()
	}
	if config.BatchSize <= 0 {
		config.BatchSize = 1000
	}
	if !config.DryRun && config.ArchiveStore == nil {
		return nil, fmt.Errorf("ArchiveStore is required when DryRun is false")
	}

	result := &ArchiveResult{
		DryRun: config.DryRun,
	}
	startTime := time.Now()

	// Determine cutoff point
	var cutoffTime time.Time
	if config.BeforeTime != nil {
		cutoffTime = *config.BeforeTime
	} else {
		// Use policy's default retention as cutoff
		cutoffTime = time.Now().Add(-config.Policy.DefaultRetention)
	}

	position := int64(0)
	result.StartPosition = position

	// Collect positions to delete if DeleteAfterArchive is true
	var positionsToDelete []int64

	for {
		select {
		case <-ctx.Done():
			result.Error = ctx.Err()
			return result, result.Error
		default:
		}

		// Check position limit
		if config.BeforePosition > 0 && position >= config.BeforePosition {
			break
		}

		// Read batch
		events, err := a.store.ReadAll(ctx, position, config.BatchSize)
		if err != nil {
			result.Error = fmt.Errorf("failed to read events: %w", err)
			return result, result.Error
		}

		if len(events) == 0 {
			break
		}

		// Filter eligible events
		eligible := make([]StoredEvent, 0, len(events))
		for _, event := range events {
			result.EventsProcessed++
			position = event.Position + 1

			// Check time cutoff first
			if event.Timestamp.After(cutoffTime) || event.Timestamp.Equal(cutoffTime) {
				result.EventsSkipped++
				continue
			}

			// Check policy
			if !config.Policy.IsEligibleForArchival(event) {
				result.EventsSkipped++
				continue
			}

			eligible = append(eligible, event)
		}

		// Archive eligible events
		if len(eligible) > 0 && !config.DryRun {
			if err := config.ArchiveStore.WriteEvents(ctx, eligible); err != nil {
				result.Error = fmt.Errorf("failed to write events to archive: %w", err)
				return result, result.Error
			}
		}

		result.EventsArchived += int64(len(eligible))

		// Track positions for deletion
		if config.DeleteAfterArchive && !config.DryRun {
			for _, event := range eligible {
				positionsToDelete = append(positionsToDelete, event.Position)
			}
		}

		// Report progress
		if config.Progress != nil {
			elapsed := time.Since(startTime)
			rate := float64(result.EventsProcessed) / elapsed.Seconds()
			config.Progress(&ArchiveProgress{
				EventsProcessed: result.EventsProcessed,
				EventsArchived:  result.EventsArchived,
				EventsSkipped:   result.EventsSkipped,
				CurrentPosition: position - 1,
				Duration:        elapsed,
				EventsPerSecond: rate,
			})
		}

		// If all events were too recent, we've reached the end of archivable events
		if len(eligible) == 0 && len(events) > 0 {
			break
		}
	}

	// Delete archived events from primary store
	if config.DeleteAfterArchive && !config.DryRun && len(positionsToDelete) > 0 {
		deletable, ok := a.store.(DeletableEventStore)
		if !ok {
			result.Error = fmt.Errorf("event store does not support deletion (must implement DeletableEventStore)")
			return result, result.Error
		}

		deleted, err := deletable.DeleteEventsByPosition(ctx, positionsToDelete)
		if err != nil {
			result.Error = fmt.Errorf("failed to delete archived events: %w", err)
			return result, result.Error
		}
		result.EventsDeleted = deleted
	}

	// Close archive store
	if !config.DryRun && config.ArchiveStore != nil {
		if err := config.ArchiveStore.Close(); err != nil {
			result.Error = fmt.Errorf("failed to close archive store: %w", err)
			return result, result.Error
		}
	}

	// Final result
	result.EndPosition = position - 1
	result.Duration = time.Since(startTime)
	if result.Duration.Seconds() > 0 {
		result.EventsPerSecond = float64(result.EventsProcessed) / result.Duration.Seconds()
	}

	// Final progress callback
	if config.Progress != nil {
		config.Progress(&ArchiveProgress{
			EventsProcessed: result.EventsProcessed,
			EventsArchived:  result.EventsArchived,
			EventsSkipped:   result.EventsSkipped,
			EventsDeleted:   result.EventsDeleted,
			CurrentPosition: result.EndPosition,
			Duration:        result.Duration,
			EventsPerSecond: result.EventsPerSecond,
			Complete:        true,
		})
	}

	return result, nil
}

// EstimateArchival counts events eligible for archival without performing it.
func (a *EventArchiver) EstimateArchival(ctx context.Context, policy *RetentionPolicy) (*ArchiveEstimate, error) {
	if policy == nil {
		policy = DefaultRetentionPolicy()
	}

	result, err := a.Archive(ctx, &ArchiveConfig{
		Policy: policy,
		DryRun: true,
	})
	if err != nil {
		return nil, err
	}

	return &ArchiveEstimate{
		EligibleEvents:   result.EventsArchived,
		IneligibleEvents: result.EventsSkipped,
		TotalEvents:      result.EventsProcessed,
	}, nil
}

// ArchiveEstimate provides statistics about potential archival.
type ArchiveEstimate struct {
	EligibleEvents   int64
	IneligibleEvents int64
	TotalEvents      int64
}

// =============================================================================
// Deletable Event Store Interface
// =============================================================================

// DeletableEventStore extends EventStore with deletion capabilities.
// This is optional - not all event stores support deletion (immutability is a feature).
type DeletableEventStore interface {
	EventStore

	// DeleteEventsByPosition removes events at the specified positions.
	// Returns the number of events actually deleted.
	// This is a dangerous operation - use with extreme caution.
	DeleteEventsByPosition(ctx context.Context, positions []int64) (int64, error)

	// DeleteEventsBeforePosition removes all events before the given position.
	// Returns the number of events deleted.
	// This is a dangerous operation - use with extreme caution.
	DeleteEventsBeforePosition(ctx context.Context, position int64) (int64, error)

	// DeleteEventsBeforeTime removes all events before the given timestamp.
	// Returns the number of events deleted.
	// This is a dangerous operation - use with extreme caution.
	DeleteEventsBeforeTime(ctx context.Context, t time.Time) (int64, error)
}

// =============================================================================
// File Archive Store (JSON Lines format)
// =============================================================================

// ArchivedEvent is the format for events in the archive file.
// Includes all fields from StoredEvent plus archive metadata.
type ArchivedEvent struct {
	Position      int64             `json:"position"`
	StreamID      string            `json:"stream_id"`
	StreamVersion int64             `json:"stream_version"`
	EventType     string            `json:"event_type"`
	Data          json.RawMessage   `json:"data"`
	Metadata      map[string]string `json:"metadata"`
	Timestamp     time.Time         `json:"timestamp"`
	ArchivedAt    time.Time         `json:"archived_at"`
}

// FileArchiveStore writes events to a JSON Lines file.
// Each line is a complete JSON object, making the file easy to process.
type FileArchiveStore struct {
	file    *os.File
	writer  *bufio.Writer
	encoder *json.Encoder
	count   int64
}

// NewFileArchiveStore creates a new file-based archive store.
// The file is created if it doesn't exist, or appended to if it does.
func NewFileArchiveStore(path string) (*FileArchiveStore, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644) //nolint:gosec // G302: archive files need read access
	if err != nil {
		return nil, fmt.Errorf("failed to open archive file: %w", err)
	}

	writer := bufio.NewWriter(file)
	encoder := json.NewEncoder(writer)

	return &FileArchiveStore{
		file:    file,
		writer:  writer,
		encoder: encoder,
	}, nil
}

// WriteEvents writes a batch of events to the archive file.
func (s *FileArchiveStore) WriteEvents(ctx context.Context, events []StoredEvent) error {
	now := time.Now()

	for _, event := range events {
		archived := ArchivedEvent{
			Position:      event.Position,
			StreamID:      event.StreamID,
			StreamVersion: event.StreamVersion,
			EventType:     event.EventType,
			Data:          event.Data,
			Metadata:      event.Metadata,
			Timestamp:     event.Timestamp,
			ArchivedAt:    now,
		}

		if err := s.encoder.Encode(archived); err != nil {
			return fmt.Errorf("failed to encode event: %w", err)
		}
		s.count++
	}

	return nil
}

// Close flushes and closes the archive file.
func (s *FileArchiveStore) Close() error {
	if err := s.writer.Flush(); err != nil {
		return fmt.Errorf("failed to flush archive writer: %w", err)
	}
	return s.file.Close()
}

// Count returns the number of events written.
func (s *FileArchiveStore) Count() int64 {
	return s.count
}

// =============================================================================
// Archive Reader (for reading back archived events)
// =============================================================================

// ArchiveReader reads events from a JSON Lines archive file.
type ArchiveReader struct {
	file    *os.File
	scanner *bufio.Scanner
}

// NewArchiveReader creates a reader for an archive file.
func NewArchiveReader(path string) (*ArchiveReader, error) {
	file, err := os.Open(path) //nolint:gosec // G304: path from trusted caller
	if err != nil {
		return nil, fmt.Errorf("failed to open archive file: %w", err)
	}

	scanner := bufio.NewScanner(file)
	// Increase buffer size for large events
	scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024)

	return &ArchiveReader{
		file:    file,
		scanner: scanner,
	}, nil
}

// Next returns the next event from the archive.
// Returns io.EOF when there are no more events.
func (r *ArchiveReader) Next() (*ArchivedEvent, error) {
	if !r.scanner.Scan() {
		if err := r.scanner.Err(); err != nil {
			return nil, fmt.Errorf("failed to read archive: %w", err)
		}
		return nil, io.EOF
	}

	var event ArchivedEvent
	if err := json.Unmarshal(r.scanner.Bytes(), &event); err != nil {
		return nil, fmt.Errorf("failed to decode archived event: %w", err)
	}

	return &event, nil
}

// Close closes the archive file.
func (r *ArchiveReader) Close() error {
	return r.file.Close()
}

// ReadAll reads all events from an archive file.
func ReadArchiveFile(path string) ([]ArchivedEvent, error) {
	reader, err := NewArchiveReader(path)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	var events []ArchivedEvent
	for {
		event, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		events = append(events, *event)
	}

	return events, nil
}
