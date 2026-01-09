package eventsourcing

import (
	"context"
	"sync"
	"time"
)

// MemoryStore is an in-memory implementation of EventStore.
// Suitable for testing and development, not for production use.
type MemoryStore struct {
	mu          sync.RWMutex
	events      []StoredEvent
	streams     map[string][]StoredEvent
	subscribers []chan StoredEvent
}

// NewMemoryStore creates a new in-memory event store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		events:  make([]StoredEvent, 0),
		streams: make(map[string][]StoredEvent),
	}
}

// Append adds events to a stream with optimistic concurrency control.
func (s *MemoryStore) Append(ctx context.Context, streamID string, expectedVersion int64, events []EventData) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Get current stream version
	streamEvents := s.streams[streamID]
	currentVersion := int64(len(streamEvents)) - 1

	// Check optimistic concurrency
	if expectedVersion >= 0 {
		if currentVersion != expectedVersion {
			return 0, ErrConcurrencyConflict
		}
	} else if expectedVersion == VersionNone {
		if len(streamEvents) > 0 {
			return 0, ErrConcurrencyConflict
		}
	}
	// VersionAny: no check needed

	// Append events
	now := time.Now()
	newVersion := currentVersion

	for _, event := range events {
		newVersion++
		stored := StoredEvent{
			Position:      int64(len(s.events)),
			StreamID:      streamID,
			StreamVersion: newVersion,
			EventType:     event.EventType,
			Data:          event.Data,
			Metadata:      event.Metadata,
			Timestamp:     now,
		}

		s.events = append(s.events, stored)
		s.streams[streamID] = append(s.streams[streamID], stored)

		// Notify subscribers
		for _, ch := range s.subscribers {
			select {
			case ch <- stored:
			default:
				// Skip if subscriber is not ready
			}
		}
	}

	return newVersion, nil
}

// ReadStream reads events from a specific stream.
func (s *MemoryStore) ReadStream(ctx context.Context, streamID string, fromVersion int64, maxCount int) ([]StoredEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	streamEvents, ok := s.streams[streamID]
	if !ok {
		return []StoredEvent{}, nil
	}

	// Convert version to index (version 0 = index 0)
	startIdx := int(fromVersion)
	if startIdx < 0 {
		startIdx = 0
	}
	if startIdx >= len(streamEvents) {
		return []StoredEvent{}, nil
	}

	endIdx := startIdx + maxCount
	if endIdx > len(streamEvents) {
		endIdx = len(streamEvents)
	}

	// Return a copy to avoid data races
	result := make([]StoredEvent, endIdx-startIdx)
	copy(result, streamEvents[startIdx:endIdx])

	return result, nil
}

// ReadAll reads events across all streams in global position order.
func (s *MemoryStore) ReadAll(ctx context.Context, fromPosition int64, maxCount int) ([]StoredEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	startIdx := int(fromPosition)
	if startIdx < 0 {
		startIdx = 0
	}
	if startIdx >= len(s.events) {
		return []StoredEvent{}, nil
	}

	endIdx := startIdx + maxCount
	if endIdx > len(s.events) {
		endIdx = len(s.events)
	}

	// Return a copy
	result := make([]StoredEvent, endIdx-startIdx)
	copy(result, s.events[startIdx:endIdx])

	return result, nil
}

// Subscribe returns a channel of new events.
func (s *MemoryStore) Subscribe(ctx context.Context, fromPosition int64) (<-chan StoredEvent, error) {
	ch := make(chan StoredEvent, 100)

	s.mu.Lock()
	s.subscribers = append(s.subscribers, ch)
	s.mu.Unlock()

	// Send existing events from position
	go func() {
		s.mu.RLock()
		for i := int(fromPosition); i < len(s.events); i++ {
			select {
			case ch <- s.events[i]:
			case <-ctx.Done():
				s.mu.RUnlock()
				return
			}
		}
		s.mu.RUnlock()

		// Wait for context cancellation
		<-ctx.Done()

		// Clean up subscriber
		s.mu.Lock()
		for i, sub := range s.subscribers {
			if sub == ch {
				s.subscribers = append(s.subscribers[:i], s.subscribers[i+1:]...)
				break
			}
		}
		s.mu.Unlock()
		close(ch)
	}()

	return ch, nil
}

// GetStreamVersion returns the current version of a stream.
func (s *MemoryStore) GetStreamVersion(ctx context.Context, streamID string) (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	streamEvents, ok := s.streams[streamID]
	if !ok || len(streamEvents) == 0 {
		return -1, nil
	}

	return int64(len(streamEvents)) - 1, nil
}

// GetLastPosition returns the global position of the most recent event.
func (s *MemoryStore) GetLastPosition(ctx context.Context) (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.events) == 0 {
		return -1, nil
	}

	return int64(len(s.events)) - 1, nil
}

// Stats returns statistics about the in-memory store.
func (s *MemoryStore) Stats() MemoryStoreStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return MemoryStoreStats{
		TotalEvents:  len(s.events),
		StreamCount:  len(s.streams),
		Subscribers:  len(s.subscribers),
	}
}

// MemoryStoreStats contains statistics about the memory store.
type MemoryStoreStats struct {
	TotalEvents int
	StreamCount int
	Subscribers int
}

// Clear removes all events (for testing).
func (s *MemoryStore) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.events = make([]StoredEvent, 0)
	s.streams = make(map[string][]StoredEvent)
}
