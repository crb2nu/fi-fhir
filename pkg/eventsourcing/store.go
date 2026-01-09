// Package eventsourcing provides event sourcing primitives for healthcare data.
// Events are stored immutably in append-only streams, enabling complete audit trails,
// temporal queries, and projection-based read models.
package eventsourcing

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// Common errors for event store operations.
var (
	// ErrConcurrencyConflict is returned when the expected version doesn't match.
	ErrConcurrencyConflict = errors.New("concurrency conflict: stream version mismatch")

	// ErrStreamNotFound is returned when a stream doesn't exist.
	ErrStreamNotFound = errors.New("stream not found")

	// ErrInvalidVersion is returned for invalid version numbers.
	ErrInvalidVersion = errors.New("invalid version number")
)

// StreamVersion constants for special cases.
const (
	// VersionAny indicates no version check (use with caution).
	VersionAny int64 = -2

	// VersionNone indicates the stream should not exist.
	VersionNone int64 = -1
)

// EventStore is the core interface for append-only event storage.
// It supports optimistic concurrency control and stream-based organization.
type EventStore interface {
	// Append adds events to a stream atomically.
	// Returns the new stream version after appending.
	//
	// Optimistic concurrency control:
	// - If expectedVersion >= 0, fails with ErrConcurrencyConflict if current version differs.
	// - If expectedVersion == VersionNone (-1), fails if stream already exists.
	// - If expectedVersion == VersionAny (-2), always appends (no version check).
	Append(ctx context.Context, streamID string, expectedVersion int64, events []EventData) (int64, error)

	// ReadStream reads events from a stream.
	// Returns events starting from fromVersion (inclusive) up to maxCount.
	// If fromVersion is 0, reads from the beginning.
	ReadStream(ctx context.Context, streamID string, fromVersion int64, maxCount int) ([]StoredEvent, error)

	// ReadAll reads events across all streams in global position order.
	// Used by projections that need to process all events.
	ReadAll(ctx context.Context, fromPosition int64, maxCount int) ([]StoredEvent, error)

	// Subscribe returns a channel of new events starting from the given position.
	// The channel is closed when the context is cancelled.
	Subscribe(ctx context.Context, fromPosition int64) (<-chan StoredEvent, error)

	// GetStreamVersion returns the current version of a stream.
	// Returns -1 if the stream doesn't exist.
	GetStreamVersion(ctx context.Context, streamID string) (int64, error)

	// GetLastPosition returns the global position of the most recent event.
	// Returns -1 if no events exist.
	GetLastPosition(ctx context.Context) (int64, error)
}

// EventData represents an event to be appended to a stream.
type EventData struct {
	// EventType identifies the type of event (e.g., "patient_admit", "lab_result").
	EventType string

	// Data is the event payload, typically a serialized struct.
	Data []byte

	// Metadata contains additional context like correlation IDs, user info.
	Metadata map[string]string
}

// StoredEvent represents an event as stored in the event store.
type StoredEvent struct {
	// Position is the global ordering position across all streams.
	// Monotonically increasing, used for projection checkpointing.
	Position int64

	// StreamID identifies the aggregate/entity this event belongs to.
	StreamID string

	// StreamVersion is the version within the stream (starts at 0).
	StreamVersion int64

	// EventType identifies the type of event.
	EventType string

	// Data is the serialized event payload.
	Data []byte

	// Metadata contains correlation IDs, user info, etc.
	Metadata map[string]string

	// Timestamp when the event was stored.
	Timestamp time.Time
}

// NewEventData creates event data from a typed event.
func NewEventData(eventType string, event interface{}, metadata map[string]string) (EventData, error) {
	data, err := json.Marshal(event)
	if err != nil {
		return EventData{}, err
	}

	if metadata == nil {
		metadata = make(map[string]string)
	}

	return EventData{
		EventType: eventType,
		Data:      data,
		Metadata:  metadata,
	}, nil
}

// Decode unmarshals the event data into the provided target.
func (e *StoredEvent) Decode(target interface{}) error {
	return json.Unmarshal(e.Data, target)
}

// TimeRangeEventStore extends EventStore with time-based queries.
// This is optional - not all event stores support efficient time-based access.
type TimeRangeEventStore interface {
	EventStore

	// ReadAllByTimeRange reads events within a time range in global position order.
	// fromTime is inclusive, toTime is exclusive.
	// Used for temporal rebuilds and point-in-time recovery.
	ReadAllByTimeRange(ctx context.Context, fromTime, toTime time.Time, maxCount int) ([]StoredEvent, error)

	// GetPositionAtTime returns the approximate position at or just before the given time.
	// Returns -1 if no events exist before that time.
	GetPositionAtTime(ctx context.Context, t time.Time) (int64, error)
}

// StreamID builders for common healthcare patterns.

// PatientStreamID returns a stream ID for patient-centric events.
func PatientStreamID(mrn string) string {
	return "patient:" + mrn
}

// EncounterStreamID returns a stream ID for encounter-specific events.
func EncounterStreamID(encounterID string) string {
	return "encounter:" + encounterID
}

// ClaimStreamID returns a stream ID for claim lifecycle events.
func ClaimStreamID(claimID string) string {
	return "claim:" + claimID
}

// SourceStreamID returns a stream ID for events from a specific source.
func SourceStreamID(sourceID string) string {
	return "source:" + sourceID
}
