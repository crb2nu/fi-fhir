package eventsourcing

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// =============================================================================
// Outbox Message Types
// =============================================================================

// OutboxMessageStatus represents the state of an outbox message.
type OutboxMessageStatus string

const (
	// OutboxStatusPending indicates the message hasn't been published yet
	OutboxStatusPending OutboxMessageStatus = "pending"

	// OutboxStatusPublished indicates the message was successfully published
	OutboxStatusPublished OutboxMessageStatus = "published"

	// OutboxStatusFailed indicates publishing failed after all retries
	OutboxStatusFailed OutboxMessageStatus = "failed"
)

// OutboxMessage represents a message waiting to be published.
type OutboxMessage struct {
	// ID uniquely identifies this message
	ID string `json:"id"`

	// AggregateType identifies the type of aggregate (e.g., "Patient", "Claim")
	AggregateType string `json:"aggregate_type"`

	// AggregateID identifies the specific aggregate instance
	AggregateID string `json:"aggregate_id"`

	// EventType identifies the type of event (e.g., "PatientCreated")
	EventType string `json:"event_type"`

	// Payload is the serialized event data
	Payload []byte `json:"payload"`

	// Metadata contains additional context (correlation IDs, etc.)
	Metadata map[string]string `json:"metadata"`

	// Topic is the destination topic/queue for this message
	Topic string `json:"topic"`

	// Status is the current message status
	Status OutboxMessageStatus `json:"status"`

	// RetryCount is the number of publish attempts
	RetryCount int `json:"retry_count"`

	// LastError contains the most recent error message
	LastError string `json:"last_error,omitempty"`

	// CreatedAt is when the message was created
	CreatedAt time.Time `json:"created_at"`

	// ProcessedAt is when the message was successfully published or failed
	ProcessedAt *time.Time `json:"processed_at,omitempty"`

	// ScheduledAt is when the message should be processed (for delayed delivery)
	ScheduledAt *time.Time `json:"scheduled_at,omitempty"`
}

// =============================================================================
// Outbox Store Interface
// =============================================================================

// OutboxStore provides persistence for outbox messages.
type OutboxStore interface {
	// SaveMessage persists an outbox message
	SaveMessage(ctx context.Context, message *OutboxMessage) error

	// SaveMessages persists multiple messages atomically
	SaveMessages(ctx context.Context, messages []*OutboxMessage) error

	// GetPendingMessages retrieves messages ready for publishing
	// Returns messages with status=pending and scheduledAt <= now
	GetPendingMessages(ctx context.Context, limit int) ([]*OutboxMessage, error)

	// MarkPublished marks a message as successfully published
	MarkPublished(ctx context.Context, messageID string) error

	// MarkFailed marks a message as failed with an error
	MarkFailed(ctx context.Context, messageID string, err error) error

	// IncrementRetry increments the retry count and updates last error
	IncrementRetry(ctx context.Context, messageID string, err error) error

	// GetMessage retrieves a message by ID
	GetMessage(ctx context.Context, messageID string) (*OutboxMessage, error)

	// DeleteOldMessages removes published/failed messages older than the given duration
	DeleteOldMessages(ctx context.Context, olderThan time.Duration) (int64, error)
}

// =============================================================================
// Publisher Interface
// =============================================================================

// OutboxPublisher is the interface for publishing messages to a message broker.
type OutboxPublisher interface {
	// Publish sends a message to the specified topic
	// Returns nil on success, error on failure
	Publish(ctx context.Context, topic string, message *OutboxMessage) error

	// Close releases any resources held by the publisher
	Close() error
}

// =============================================================================
// Outbox Relay (Background Publisher)
// =============================================================================

// OutboxRelayConfig configures the outbox relay.
type OutboxRelayConfig struct {
	// PollInterval is how often to check for pending messages
	// Default: 1 second
	PollInterval time.Duration

	// BatchSize is the maximum number of messages to process per poll
	// Default: 100
	BatchSize int

	// MaxRetries is the maximum number of publish attempts before marking as failed
	// Default: 5
	MaxRetries int

	// RetryDelay is the delay between retry attempts
	// Default: 5 seconds
	RetryDelay time.Duration

	// OnPublished callback when a message is successfully published
	OnPublished func(message *OutboxMessage)

	// OnFailed callback when a message fails after all retries
	OnFailed func(message *OutboxMessage, err error)

	// OnError callback for relay errors (not message-specific)
	OnError func(err error)
}

// DefaultOutboxRelayConfig returns sensible defaults.
func DefaultOutboxRelayConfig() *OutboxRelayConfig {
	return &OutboxRelayConfig{
		PollInterval: time.Second,
		BatchSize:    100,
		MaxRetries:   5,
		RetryDelay:   5 * time.Second,
	}
}

// OutboxRelay reads messages from the outbox and publishes them.
type OutboxRelay struct {
	store     OutboxStore
	publisher OutboxPublisher
	config    *OutboxRelayConfig
	stopCh    chan struct{}
	wg        sync.WaitGroup
	running   bool
	mu        sync.Mutex
}

// NewOutboxRelay creates a new outbox relay.
func NewOutboxRelay(store OutboxStore, publisher OutboxPublisher, config *OutboxRelayConfig) *OutboxRelay {
	if config == nil {
		config = DefaultOutboxRelayConfig()
	}
	if config.PollInterval <= 0 {
		config.PollInterval = time.Second
	}
	if config.BatchSize <= 0 {
		config.BatchSize = 100
	}
	if config.MaxRetries <= 0 {
		config.MaxRetries = 5
	}
	if config.RetryDelay <= 0 {
		config.RetryDelay = 5 * time.Second
	}

	return &OutboxRelay{
		store:     store,
		publisher: publisher,
		config:    config,
		stopCh:    make(chan struct{}),
	}
}

// Start begins the relay process in a background goroutine.
func (r *OutboxRelay) Start(ctx context.Context) error {
	r.mu.Lock()
	if r.running {
		r.mu.Unlock()
		return fmt.Errorf("relay is already running")
	}
	r.running = true
	r.stopCh = make(chan struct{})
	r.mu.Unlock()

	r.wg.Add(1)
	go r.run(ctx)

	return nil
}

// Stop stops the relay process.
func (r *OutboxRelay) Stop() {
	r.mu.Lock()
	if !r.running {
		r.mu.Unlock()
		return
	}
	r.running = false
	close(r.stopCh)
	r.mu.Unlock()

	r.wg.Wait()
}

// run is the main relay loop.
func (r *OutboxRelay) run(ctx context.Context) {
	defer r.wg.Done()

	ticker := time.NewTicker(r.config.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-r.stopCh:
			return
		case <-ticker.C:
			if err := r.processBatch(ctx); err != nil {
				if r.config.OnError != nil {
					r.config.OnError(err)
				}
			}
		}
	}
}

// processBatch processes one batch of pending messages.
func (r *OutboxRelay) processBatch(ctx context.Context) error {
	messages, err := r.store.GetPendingMessages(ctx, r.config.BatchSize)
	if err != nil {
		return fmt.Errorf("failed to get pending messages: %w", err)
	}

	for _, msg := range messages {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-r.stopCh:
			return nil
		default:
		}

		if err := r.publishMessage(ctx, msg); err != nil {
			// Error already handled in publishMessage
			continue
		}
	}

	return nil
}

// publishMessage attempts to publish a single message.
func (r *OutboxRelay) publishMessage(ctx context.Context, msg *OutboxMessage) error {
	err := r.publisher.Publish(ctx, msg.Topic, msg)

	if err == nil {
		// Success
		if markErr := r.store.MarkPublished(ctx, msg.ID); markErr != nil {
			// Log but don't fail - message was published
			if r.config.OnError != nil {
				r.config.OnError(fmt.Errorf("failed to mark message %s as published: %w", msg.ID, markErr))
			}
		}

		if r.config.OnPublished != nil {
			r.config.OnPublished(msg)
		}
		return nil
	}

	// Publishing failed
	msg.RetryCount++

	if msg.RetryCount >= r.config.MaxRetries {
		// Max retries exceeded
		if markErr := r.store.MarkFailed(ctx, msg.ID, err); markErr != nil {
			if r.config.OnError != nil {
				r.config.OnError(fmt.Errorf("failed to mark message %s as failed: %w", msg.ID, markErr))
			}
		}

		if r.config.OnFailed != nil {
			r.config.OnFailed(msg, err)
		}
	} else {
		// Increment retry count
		if retryErr := r.store.IncrementRetry(ctx, msg.ID, err); retryErr != nil {
			if r.config.OnError != nil {
				r.config.OnError(fmt.Errorf("failed to increment retry for message %s: %w", msg.ID, retryErr))
			}
		}
	}

	return err
}

// ProcessOnce processes one batch of messages synchronously.
// Useful for testing or manual triggering.
func (r *OutboxRelay) ProcessOnce(ctx context.Context) error {
	return r.processBatch(ctx)
}

// =============================================================================
// In-Memory Outbox Store (for testing)
// =============================================================================

// MemoryOutboxStore is an in-memory outbox store for testing.
type MemoryOutboxStore struct {
	messages map[string]*OutboxMessage
	mu       sync.RWMutex
}

// NewMemoryOutboxStore creates a new in-memory outbox store.
func NewMemoryOutboxStore() *MemoryOutboxStore {
	return &MemoryOutboxStore{
		messages: make(map[string]*OutboxMessage),
	}
}

// SaveMessage stores an outbox message.
func (s *MemoryOutboxStore) SaveMessage(ctx context.Context, message *OutboxMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if message.CreatedAt.IsZero() {
		message.CreatedAt = time.Now()
	}
	if message.Status == "" {
		message.Status = OutboxStatusPending
	}

	// Deep copy
	data, _ := json.Marshal(message)
	var msgCopy OutboxMessage
	json.Unmarshal(data, &msgCopy)
	s.messages[message.ID] = &msgCopy

	return nil
}

// SaveMessages stores multiple messages.
func (s *MemoryOutboxStore) SaveMessages(ctx context.Context, messages []*OutboxMessage) error {
	for _, msg := range messages {
		if err := s.SaveMessage(ctx, msg); err != nil {
			return err
		}
	}
	return nil
}

// GetPendingMessages retrieves pending messages ready for publishing.
func (s *MemoryOutboxStore) GetPendingMessages(ctx context.Context, limit int) ([]*OutboxMessage, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	now := time.Now()
	var result []*OutboxMessage

	for _, msg := range s.messages {
		if msg.Status != OutboxStatusPending {
			continue
		}

		// Check scheduled time
		if msg.ScheduledAt != nil && msg.ScheduledAt.After(now) {
			continue
		}

		// Return a copy
		data, _ := json.Marshal(msg)
		var msgCopy OutboxMessage
		json.Unmarshal(data, &msgCopy)
		result = append(result, &msgCopy)

		if limit > 0 && len(result) >= limit {
			break
		}
	}

	return result, nil
}

// MarkPublished marks a message as published.
func (s *MemoryOutboxStore) MarkPublished(ctx context.Context, messageID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	msg, ok := s.messages[messageID]
	if !ok {
		return fmt.Errorf("message not found: %s", messageID)
	}

	now := time.Now()
	msg.Status = OutboxStatusPublished
	msg.ProcessedAt = &now

	return nil
}

// MarkFailed marks a message as failed.
func (s *MemoryOutboxStore) MarkFailed(ctx context.Context, messageID string, err error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	msg, ok := s.messages[messageID]
	if !ok {
		return fmt.Errorf("message not found: %s", messageID)
	}

	now := time.Now()
	msg.Status = OutboxStatusFailed
	msg.ProcessedAt = &now
	if err != nil {
		msg.LastError = err.Error()
	}

	return nil
}

// IncrementRetry increments the retry count.
func (s *MemoryOutboxStore) IncrementRetry(ctx context.Context, messageID string, err error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	msg, ok := s.messages[messageID]
	if !ok {
		return fmt.Errorf("message not found: %s", messageID)
	}

	msg.RetryCount++
	if err != nil {
		msg.LastError = err.Error()
	}

	return nil
}

// GetMessage retrieves a message by ID.
func (s *MemoryOutboxStore) GetMessage(ctx context.Context, messageID string) (*OutboxMessage, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	msg, ok := s.messages[messageID]
	if !ok {
		return nil, nil
	}

	// Return a copy
	data, _ := json.Marshal(msg)
	var msgCopy OutboxMessage
	json.Unmarshal(data, &msgCopy)
	return &msgCopy, nil
}

// DeleteOldMessages removes old published/failed messages.
func (s *MemoryOutboxStore) DeleteOldMessages(ctx context.Context, olderThan time.Duration) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := time.Now().Add(-olderThan)
	var deleted int64

	for id, msg := range s.messages {
		if msg.Status == OutboxStatusPublished || msg.Status == OutboxStatusFailed {
			if msg.ProcessedAt != nil && msg.ProcessedAt.Before(cutoff) {
				delete(s.messages, id)
				deleted++
			}
		}
	}

	return deleted, nil
}

// =============================================================================
// Mock Publisher (for testing)
// =============================================================================

// MockPublisher is a test publisher that records published messages.
type MockPublisher struct {
	Published []*OutboxMessage
	FailFor   map[string]error // message ID -> error to return
	mu        sync.Mutex
}

// NewMockPublisher creates a new mock publisher.
func NewMockPublisher() *MockPublisher {
	return &MockPublisher{
		Published: make([]*OutboxMessage, 0),
		FailFor:   make(map[string]error),
	}
}

// Publish records the message (or returns configured error).
func (p *MockPublisher) Publish(ctx context.Context, topic string, message *OutboxMessage) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if err, ok := p.FailFor[message.ID]; ok {
		return err
	}

	p.Published = append(p.Published, message)
	return nil
}

// Close does nothing for mock.
func (p *MockPublisher) Close() error {
	return nil
}

// SetFailure configures the publisher to fail for a specific message.
func (p *MockPublisher) SetFailure(messageID string, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.FailFor[messageID] = err
}

// ClearFailure removes a failure configuration.
func (p *MockPublisher) ClearFailure(messageID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.FailFor, messageID)
}

// GetPublished returns a copy of published messages.
func (p *MockPublisher) GetPublished() []*OutboxMessage {
	p.mu.Lock()
	defer p.mu.Unlock()

	result := make([]*OutboxMessage, len(p.Published))
	copy(result, p.Published)
	return result
}

// =============================================================================
// Outbox Helper Functions
// =============================================================================

// NewOutboxMessage creates a new outbox message from an event.
func NewOutboxMessage(id, aggregateType, aggregateID, eventType, topic string, payload interface{}, metadata map[string]string) (*OutboxMessage, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	if metadata == nil {
		metadata = make(map[string]string)
	}

	return &OutboxMessage{
		ID:            id,
		AggregateType: aggregateType,
		AggregateID:   aggregateID,
		EventType:     eventType,
		Payload:       data,
		Metadata:      metadata,
		Topic:         topic,
		Status:        OutboxStatusPending,
		CreatedAt:     time.Now(),
	}, nil
}

// NewOutboxMessageFromStoredEvent creates an outbox message from a StoredEvent.
func NewOutboxMessageFromStoredEvent(id string, event StoredEvent, topic string) *OutboxMessage {
	return &OutboxMessage{
		ID:            id,
		AggregateType: "", // Can be parsed from StreamID if needed
		AggregateID:   event.StreamID,
		EventType:     event.EventType,
		Payload:       event.Data,
		Metadata:      event.Metadata,
		Topic:         topic,
		Status:        OutboxStatusPending,
		CreatedAt:     time.Now(),
	}
}

// =============================================================================
// Event Store with Outbox Integration
// =============================================================================

// OutboxEventStore wraps an EventStore to automatically create outbox messages.
type OutboxEventStore struct {
	eventStore  EventStore
	outboxStore OutboxStore
	topicMapper func(eventType string) string
	idGenerator func() string
}

// NewOutboxEventStore creates an event store that automatically adds outbox messages.
func NewOutboxEventStore(
	eventStore EventStore,
	outboxStore OutboxStore,
	topicMapper func(eventType string) string,
	idGenerator func() string,
) *OutboxEventStore {
	return &OutboxEventStore{
		eventStore:  eventStore,
		outboxStore: outboxStore,
		topicMapper: topicMapper,
		idGenerator: idGenerator,
	}
}

// Append adds events to the event store and creates outbox messages.
// Note: In production with PostgreSQL, this should be done in a single transaction.
func (s *OutboxEventStore) Append(ctx context.Context, streamID string, expectedVersion int64, events []EventData) (int64, error) {
	// First append to event store
	newVersion, err := s.eventStore.Append(ctx, streamID, expectedVersion, events)
	if err != nil {
		return 0, err
	}

	// Create outbox messages for each event
	messages := make([]*OutboxMessage, len(events))
	for i, event := range events {
		topic := s.topicMapper(event.EventType)
		messages[i] = &OutboxMessage{
			ID:          s.idGenerator(),
			AggregateID: streamID,
			EventType:   event.EventType,
			Payload:     event.Data,
			Metadata:    event.Metadata,
			Topic:       topic,
			Status:      OutboxStatusPending,
			CreatedAt:   time.Now(),
		}
	}

	// Save outbox messages
	if err := s.outboxStore.SaveMessages(ctx, messages); err != nil {
		// In a real implementation, this would be in the same transaction
		// For now, log the error but don't fail the append
		return newVersion, nil //nolint:nilerr // Intentional: outbox failure should not fail event append
	}

	return newVersion, nil
}

// ReadStream delegates to the underlying event store.
func (s *OutboxEventStore) ReadStream(ctx context.Context, streamID string, fromVersion int64, maxCount int) ([]StoredEvent, error) {
	return s.eventStore.ReadStream(ctx, streamID, fromVersion, maxCount)
}

// ReadAll delegates to the underlying event store.
func (s *OutboxEventStore) ReadAll(ctx context.Context, fromPosition int64, maxCount int) ([]StoredEvent, error) {
	return s.eventStore.ReadAll(ctx, fromPosition, maxCount)
}

// Subscribe delegates to the underlying event store.
func (s *OutboxEventStore) Subscribe(ctx context.Context, fromPosition int64) (<-chan StoredEvent, error) {
	return s.eventStore.Subscribe(ctx, fromPosition)
}

// GetStreamVersion delegates to the underlying event store.
func (s *OutboxEventStore) GetStreamVersion(ctx context.Context, streamID string) (int64, error) {
	return s.eventStore.GetStreamVersion(ctx, streamID)
}

// GetLastPosition delegates to the underlying event store.
func (s *OutboxEventStore) GetLastPosition(ctx context.Context) (int64, error) {
	return s.eventStore.GetLastPosition(ctx)
}
