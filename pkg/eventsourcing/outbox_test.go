package eventsourcing

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// =============================================================================
// MemoryOutboxStore Tests
// =============================================================================

func TestMemoryOutboxStore_SaveAndGetMessage(t *testing.T) {
	store := NewMemoryOutboxStore()
	ctx := context.Background()

	msg := &OutboxMessage{
		ID:            "msg-001",
		AggregateType: "Patient",
		AggregateID:   "patient:123",
		EventType:     "PatientCreated",
		Payload:       []byte(`{"name":"John Doe"}`),
		Topic:         "healthcare.patient.events",
	}

	// Save message
	if err := store.SaveMessage(ctx, msg); err != nil {
		t.Fatalf("SaveMessage failed: %v", err)
	}

	// Retrieve message
	got, err := store.GetMessage(ctx, "msg-001")
	if err != nil {
		t.Fatalf("GetMessage failed: %v", err)
	}
	if got == nil {
		t.Fatal("Expected message, got nil")
	}

	// Verify fields
	if got.ID != "msg-001" {
		t.Errorf("ID = %s, want msg-001", got.ID)
	}
	if got.AggregateType != "Patient" {
		t.Errorf("AggregateType = %s, want Patient", got.AggregateType)
	}
	if got.EventType != "PatientCreated" {
		t.Errorf("EventType = %s, want PatientCreated", got.EventType)
	}
	if got.Status != OutboxStatusPending {
		t.Errorf("Status = %s, want pending", got.Status)
	}
	if got.CreatedAt.IsZero() {
		t.Error("CreatedAt should be set")
	}
}

func TestMemoryOutboxStore_SaveMessages(t *testing.T) {
	store := NewMemoryOutboxStore()
	ctx := context.Background()

	messages := []*OutboxMessage{
		{ID: "msg-001", EventType: "Event1", Topic: "topic1"},
		{ID: "msg-002", EventType: "Event2", Topic: "topic2"},
		{ID: "msg-003", EventType: "Event3", Topic: "topic3"},
	}

	if err := store.SaveMessages(ctx, messages); err != nil {
		t.Fatalf("SaveMessages failed: %v", err)
	}

	// Verify all messages exist
	for _, expected := range messages {
		got, err := store.GetMessage(ctx, expected.ID)
		if err != nil {
			t.Errorf("GetMessage(%s) failed: %v", expected.ID, err)
		}
		if got == nil {
			t.Errorf("Message %s not found", expected.ID)
		}
	}
}

func TestMemoryOutboxStore_GetPendingMessages(t *testing.T) {
	store := NewMemoryOutboxStore()
	ctx := context.Background()

	// Add messages with different statuses
	store.SaveMessage(ctx, &OutboxMessage{
		ID:        "msg-pending-1",
		EventType: "Event1",
		Status:    OutboxStatusPending,
		Topic:     "topic",
	})
	store.SaveMessage(ctx, &OutboxMessage{
		ID:        "msg-pending-2",
		EventType: "Event2",
		Status:    OutboxStatusPending,
		Topic:     "topic",
	})
	store.SaveMessage(ctx, &OutboxMessage{
		ID:        "msg-published",
		EventType: "Event3",
		Status:    OutboxStatusPublished,
		Topic:     "topic",
	})
	store.SaveMessage(ctx, &OutboxMessage{
		ID:        "msg-failed",
		EventType: "Event4",
		Status:    OutboxStatusFailed,
		Topic:     "topic",
	})

	// Get pending messages
	pending, err := store.GetPendingMessages(ctx, 10)
	if err != nil {
		t.Fatalf("GetPendingMessages failed: %v", err)
	}

	if len(pending) != 2 {
		t.Errorf("Expected 2 pending messages, got %d", len(pending))
	}

	// Verify only pending messages are returned
	for _, msg := range pending {
		if msg.Status != OutboxStatusPending {
			t.Errorf("Got non-pending message: %s (status=%s)", msg.ID, msg.Status)
		}
	}
}

func TestMemoryOutboxStore_GetPendingMessages_Limit(t *testing.T) {
	store := NewMemoryOutboxStore()
	ctx := context.Background()

	// Add 10 pending messages
	for i := 0; i < 10; i++ {
		store.SaveMessage(ctx, &OutboxMessage{
			ID:     "msg-" + string(rune('a'+i)),
			Status: OutboxStatusPending,
			Topic:  "topic",
		})
	}

	// Get with limit
	pending, err := store.GetPendingMessages(ctx, 3)
	if err != nil {
		t.Fatalf("GetPendingMessages failed: %v", err)
	}

	if len(pending) != 3 {
		t.Errorf("Expected 3 messages (limit), got %d", len(pending))
	}
}

func TestMemoryOutboxStore_GetPendingMessages_ScheduledAt(t *testing.T) {
	store := NewMemoryOutboxStore()
	ctx := context.Background()

	now := time.Now()
	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)

	// Message scheduled in the past (should be returned)
	store.SaveMessage(ctx, &OutboxMessage{
		ID:          "msg-past",
		Status:      OutboxStatusPending,
		Topic:       "topic",
		ScheduledAt: &past,
	})

	// Message scheduled in the future (should NOT be returned)
	store.SaveMessage(ctx, &OutboxMessage{
		ID:          "msg-future",
		Status:      OutboxStatusPending,
		Topic:       "topic",
		ScheduledAt: &future,
	})

	// Message with no schedule (should be returned)
	store.SaveMessage(ctx, &OutboxMessage{
		ID:     "msg-no-schedule",
		Status: OutboxStatusPending,
		Topic:  "topic",
	})

	pending, err := store.GetPendingMessages(ctx, 10)
	if err != nil {
		t.Fatalf("GetPendingMessages failed: %v", err)
	}

	if len(pending) != 2 {
		t.Errorf("Expected 2 messages (past + no schedule), got %d", len(pending))
	}

	// Verify msg-future is not included
	for _, msg := range pending {
		if msg.ID == "msg-future" {
			t.Error("Future-scheduled message should not be returned")
		}
	}
}

func TestMemoryOutboxStore_MarkPublished(t *testing.T) {
	store := NewMemoryOutboxStore()
	ctx := context.Background()

	store.SaveMessage(ctx, &OutboxMessage{
		ID:     "msg-001",
		Status: OutboxStatusPending,
		Topic:  "topic",
	})

	// Mark as published
	if err := store.MarkPublished(ctx, "msg-001"); err != nil {
		t.Fatalf("MarkPublished failed: %v", err)
	}

	// Verify status
	msg, _ := store.GetMessage(ctx, "msg-001")
	if msg.Status != OutboxStatusPublished {
		t.Errorf("Status = %s, want published", msg.Status)
	}
	if msg.ProcessedAt == nil {
		t.Error("ProcessedAt should be set")
	}
}

func TestMemoryOutboxStore_MarkFailed(t *testing.T) {
	store := NewMemoryOutboxStore()
	ctx := context.Background()

	store.SaveMessage(ctx, &OutboxMessage{
		ID:     "msg-001",
		Status: OutboxStatusPending,
		Topic:  "topic",
	})

	// Mark as failed
	testErr := errors.New("connection refused")
	if err := store.MarkFailed(ctx, "msg-001", testErr); err != nil {
		t.Fatalf("MarkFailed failed: %v", err)
	}

	// Verify status
	msg, _ := store.GetMessage(ctx, "msg-001")
	if msg.Status != OutboxStatusFailed {
		t.Errorf("Status = %s, want failed", msg.Status)
	}
	if msg.LastError != "connection refused" {
		t.Errorf("LastError = %s, want 'connection refused'", msg.LastError)
	}
	if msg.ProcessedAt == nil {
		t.Error("ProcessedAt should be set")
	}
}

func TestMemoryOutboxStore_IncrementRetry(t *testing.T) {
	store := NewMemoryOutboxStore()
	ctx := context.Background()

	store.SaveMessage(ctx, &OutboxMessage{
		ID:         "msg-001",
		Status:     OutboxStatusPending,
		Topic:      "topic",
		RetryCount: 0,
	})

	// Increment retry
	testErr := errors.New("timeout")
	if err := store.IncrementRetry(ctx, "msg-001", testErr); err != nil {
		t.Fatalf("IncrementRetry failed: %v", err)
	}

	msg, _ := store.GetMessage(ctx, "msg-001")
	if msg.RetryCount != 1 {
		t.Errorf("RetryCount = %d, want 1", msg.RetryCount)
	}
	if msg.LastError != "timeout" {
		t.Errorf("LastError = %s, want 'timeout'", msg.LastError)
	}

	// Still pending
	if msg.Status != OutboxStatusPending {
		t.Errorf("Status = %s, want pending", msg.Status)
	}
}

func TestMemoryOutboxStore_DeleteOldMessages(t *testing.T) {
	store := NewMemoryOutboxStore()
	ctx := context.Background()

	now := time.Now()
	oldTime := now.Add(-48 * time.Hour)
	recentTime := now.Add(-1 * time.Hour)

	// Old published message (should be deleted)
	store.SaveMessage(ctx, &OutboxMessage{
		ID:          "msg-old-published",
		Status:      OutboxStatusPublished,
		Topic:       "topic",
		ProcessedAt: &oldTime,
	})

	// Old failed message (should be deleted)
	store.SaveMessage(ctx, &OutboxMessage{
		ID:          "msg-old-failed",
		Status:      OutboxStatusFailed,
		Topic:       "topic",
		ProcessedAt: &oldTime,
	})

	// Recent published message (should NOT be deleted)
	store.SaveMessage(ctx, &OutboxMessage{
		ID:          "msg-recent-published",
		Status:      OutboxStatusPublished,
		Topic:       "topic",
		ProcessedAt: &recentTime,
	})

	// Pending message (should NOT be deleted regardless of age)
	store.SaveMessage(ctx, &OutboxMessage{
		ID:     "msg-pending",
		Status: OutboxStatusPending,
		Topic:  "topic",
	})

	// Delete messages older than 24 hours
	deleted, err := store.DeleteOldMessages(ctx, 24*time.Hour)
	if err != nil {
		t.Fatalf("DeleteOldMessages failed: %v", err)
	}

	if deleted != 2 {
		t.Errorf("Expected 2 deleted, got %d", deleted)
	}

	// Verify remaining messages
	msg1, _ := store.GetMessage(ctx, "msg-old-published")
	if msg1 != nil {
		t.Error("msg-old-published should have been deleted")
	}

	msg2, _ := store.GetMessage(ctx, "msg-old-failed")
	if msg2 != nil {
		t.Error("msg-old-failed should have been deleted")
	}

	msg3, _ := store.GetMessage(ctx, "msg-recent-published")
	if msg3 == nil {
		t.Error("msg-recent-published should NOT have been deleted")
	}

	msg4, _ := store.GetMessage(ctx, "msg-pending")
	if msg4 == nil {
		t.Error("msg-pending should NOT have been deleted")
	}
}

func TestMemoryOutboxStore_GetMessage_NotFound(t *testing.T) {
	store := NewMemoryOutboxStore()
	ctx := context.Background()

	msg, err := store.GetMessage(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("GetMessage failed: %v", err)
	}
	if msg != nil {
		t.Error("Expected nil for nonexistent message")
	}
}

// =============================================================================
// MockPublisher Tests
// =============================================================================

func TestMockPublisher_Publish(t *testing.T) {
	publisher := NewMockPublisher()
	ctx := context.Background()

	msg := &OutboxMessage{
		ID:        "msg-001",
		EventType: "TestEvent",
		Topic:     "test.topic",
	}

	if err := publisher.Publish(ctx, "test.topic", msg); err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	published := publisher.GetPublished()
	if len(published) != 1 {
		t.Fatalf("Expected 1 published message, got %d", len(published))
	}
	if published[0].ID != "msg-001" {
		t.Errorf("Published message ID = %s, want msg-001", published[0].ID)
	}
}

func TestMockPublisher_SetFailure(t *testing.T) {
	publisher := NewMockPublisher()
	ctx := context.Background()

	msg := &OutboxMessage{
		ID:        "msg-001",
		EventType: "TestEvent",
		Topic:     "test.topic",
	}

	// Set failure for this message
	expectedErr := errors.New("broker unavailable")
	publisher.SetFailure("msg-001", expectedErr)

	// Publish should fail
	err := publisher.Publish(ctx, "test.topic", msg)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	if !errors.Is(err, expectedErr) {
		t.Errorf("Error = %v, want %v", err, expectedErr)
	}

	// No messages should be published
	if len(publisher.GetPublished()) != 0 {
		t.Error("Expected no published messages")
	}

	// Clear failure
	publisher.ClearFailure("msg-001")

	// Now publish should succeed
	if err := publisher.Publish(ctx, "test.topic", msg); err != nil {
		t.Fatalf("Publish after clearing failure: %v", err)
	}
}

// =============================================================================
// OutboxRelay Tests
// =============================================================================

func TestOutboxRelay_ProcessOnce_Success(t *testing.T) {
	store := NewMemoryOutboxStore()
	publisher := NewMockPublisher()
	ctx := context.Background()

	// Add pending messages
	for i := 0; i < 3; i++ {
		store.SaveMessage(ctx, &OutboxMessage{
			ID:        "msg-" + string(rune('a'+i)),
			EventType: "TestEvent",
			Topic:     "test.topic",
		})
	}

	relay := NewOutboxRelay(store, publisher, nil)

	// Process once
	if err := relay.ProcessOnce(ctx); err != nil {
		t.Fatalf("ProcessOnce failed: %v", err)
	}

	// Verify all messages published
	if len(publisher.GetPublished()) != 3 {
		t.Errorf("Expected 3 published messages, got %d", len(publisher.GetPublished()))
	}

	// Verify messages marked as published
	for i := 0; i < 3; i++ {
		msg, _ := store.GetMessage(ctx, "msg-"+string(rune('a'+i)))
		if msg.Status != OutboxStatusPublished {
			t.Errorf("Message %s status = %s, want published", msg.ID, msg.Status)
		}
	}
}

func TestOutboxRelay_ProcessOnce_Failure(t *testing.T) {
	store := NewMemoryOutboxStore()
	publisher := NewMockPublisher()
	ctx := context.Background()

	// Add a pending message
	store.SaveMessage(ctx, &OutboxMessage{
		ID:        "msg-001",
		EventType: "TestEvent",
		Topic:     "test.topic",
	})

	// Configure publisher to fail
	publisher.SetFailure("msg-001", errors.New("connection refused"))

	config := &OutboxRelayConfig{
		MaxRetries: 3,
	}
	relay := NewOutboxRelay(store, publisher, config)

	// Process once - should fail but not crash
	relay.ProcessOnce(ctx)

	// Verify retry was incremented
	msg, _ := store.GetMessage(ctx, "msg-001")
	if msg.RetryCount != 1 {
		t.Errorf("RetryCount = %d, want 1", msg.RetryCount)
	}
	if msg.Status != OutboxStatusPending {
		t.Errorf("Status = %s, want pending (should still retry)", msg.Status)
	}
}

func TestOutboxRelay_ProcessOnce_MaxRetriesExceeded(t *testing.T) {
	store := NewMemoryOutboxStore()
	publisher := NewMockPublisher()
	ctx := context.Background()

	// Add message with retries already at max-1
	store.SaveMessage(ctx, &OutboxMessage{
		ID:         "msg-001",
		EventType:  "TestEvent",
		Topic:      "test.topic",
		RetryCount: 4, // One more will exceed max of 5
	})

	// Configure publisher to fail
	publisher.SetFailure("msg-001", errors.New("permanent failure"))

	var failedCalled bool
	var failedMsg *OutboxMessage
	config := &OutboxRelayConfig{
		MaxRetries: 5,
		OnFailed: func(msg *OutboxMessage, err error) {
			failedCalled = true
			failedMsg = msg
		},
	}
	relay := NewOutboxRelay(store, publisher, config)

	// Process once
	relay.ProcessOnce(ctx)

	// Verify message marked as failed
	msg, _ := store.GetMessage(ctx, "msg-001")
	if msg.Status != OutboxStatusFailed {
		t.Errorf("Status = %s, want failed", msg.Status)
	}

	// Verify callback was called
	if !failedCalled {
		t.Error("OnFailed callback should have been called")
	}
	if failedMsg == nil || failedMsg.ID != "msg-001" {
		t.Error("OnFailed callback received wrong message")
	}
}

func TestOutboxRelay_Callbacks(t *testing.T) {
	store := NewMemoryOutboxStore()
	publisher := NewMockPublisher()
	ctx := context.Background()

	store.SaveMessage(ctx, &OutboxMessage{
		ID:        "msg-001",
		EventType: "TestEvent",
		Topic:     "test.topic",
	})

	var publishedCalled bool
	var publishedMsg *OutboxMessage
	config := &OutboxRelayConfig{
		OnPublished: func(msg *OutboxMessage) {
			publishedCalled = true
			publishedMsg = msg
		},
	}
	relay := NewOutboxRelay(store, publisher, config)

	relay.ProcessOnce(ctx)

	if !publishedCalled {
		t.Error("OnPublished callback should have been called")
	}
	if publishedMsg == nil || publishedMsg.ID != "msg-001" {
		t.Error("OnPublished callback received wrong message")
	}
}

func TestOutboxRelay_StartStop(t *testing.T) {
	store := NewMemoryOutboxStore()
	publisher := NewMockPublisher()
	ctx := context.Background()

	var processCount int32
	config := &OutboxRelayConfig{
		PollInterval: 10 * time.Millisecond,
		OnPublished: func(msg *OutboxMessage) {
			atomic.AddInt32(&processCount, 1)
		},
	}
	relay := NewOutboxRelay(store, publisher, config)

	// Add messages
	for i := 0; i < 5; i++ {
		store.SaveMessage(ctx, &OutboxMessage{
			ID:        "msg-" + string(rune('a'+i)),
			EventType: "TestEvent",
			Topic:     "test.topic",
		})
	}

	// Start relay
	if err := relay.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Wait for processing
	time.Sleep(50 * time.Millisecond)

	// Stop relay
	relay.Stop()

	// Verify messages were processed
	count := atomic.LoadInt32(&processCount)
	if count != 5 {
		t.Errorf("Expected 5 messages processed, got %d", count)
	}
}

func TestOutboxRelay_StartTwiceFails(t *testing.T) {
	store := NewMemoryOutboxStore()
	publisher := NewMockPublisher()
	ctx := context.Background()

	relay := NewOutboxRelay(store, publisher, nil)

	if err := relay.Start(ctx); err != nil {
		t.Fatalf("First Start failed: %v", err)
	}
	defer relay.Stop()

	// Second start should fail
	if err := relay.Start(ctx); err == nil {
		t.Error("Second Start should fail")
	}
}

func TestOutboxRelay_DefaultConfig(t *testing.T) {
	store := NewMemoryOutboxStore()
	publisher := NewMockPublisher()

	relay := NewOutboxRelay(store, publisher, nil)

	if relay.config.PollInterval != time.Second {
		t.Errorf("Default PollInterval = %v, want 1s", relay.config.PollInterval)
	}
	if relay.config.BatchSize != 100 {
		t.Errorf("Default BatchSize = %d, want 100", relay.config.BatchSize)
	}
	if relay.config.MaxRetries != 5 {
		t.Errorf("Default MaxRetries = %d, want 5", relay.config.MaxRetries)
	}
}

// =============================================================================
// Helper Function Tests
// =============================================================================

func TestNewOutboxMessage(t *testing.T) {
	payload := map[string]string{"name": "John Doe"}
	metadata := map[string]string{"correlation_id": "corr-123"}

	msg, err := NewOutboxMessage(
		"msg-001",
		"Patient",
		"patient:123",
		"PatientCreated",
		"healthcare.patient.events",
		payload,
		metadata,
	)
	if err != nil {
		t.Fatalf("NewOutboxMessage failed: %v", err)
	}

	if msg.ID != "msg-001" {
		t.Errorf("ID = %s, want msg-001", msg.ID)
	}
	if msg.AggregateType != "Patient" {
		t.Errorf("AggregateType = %s, want Patient", msg.AggregateType)
	}
	if msg.EventType != "PatientCreated" {
		t.Errorf("EventType = %s, want PatientCreated", msg.EventType)
	}
	if msg.Status != OutboxStatusPending {
		t.Errorf("Status = %s, want pending", msg.Status)
	}

	// Verify payload was serialized
	var decoded map[string]string
	if err := json.Unmarshal(msg.Payload, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal payload: %v", err)
	}
	if decoded["name"] != "John Doe" {
		t.Errorf("Payload name = %s, want John Doe", decoded["name"])
	}

	// Verify metadata
	if msg.Metadata["correlation_id"] != "corr-123" {
		t.Errorf("Metadata correlation_id = %s, want corr-123", msg.Metadata["correlation_id"])
	}
}

func TestNewOutboxMessageFromStoredEvent(t *testing.T) {
	event := StoredEvent{
		StreamID:  "patient:123",
		EventType: "PatientUpdated",
		Data:      []byte(`{"name":"Jane Doe"}`),
		Metadata:  map[string]string{"user_id": "user-456"},
	}

	msg := NewOutboxMessageFromStoredEvent("msg-001", event, "healthcare.events")

	if msg.ID != "msg-001" {
		t.Errorf("ID = %s, want msg-001", msg.ID)
	}
	if msg.AggregateID != "patient:123" {
		t.Errorf("AggregateID = %s, want patient:123", msg.AggregateID)
	}
	if msg.EventType != "PatientUpdated" {
		t.Errorf("EventType = %s, want PatientUpdated", msg.EventType)
	}
	if msg.Topic != "healthcare.events" {
		t.Errorf("Topic = %s, want healthcare.events", msg.Topic)
	}
}

// =============================================================================
// OutboxEventStore Tests
// =============================================================================

func TestOutboxEventStore_Append(t *testing.T) {
	eventStore := NewMemoryStore()
	outboxStore := NewMemoryOutboxStore()
	ctx := context.Background()

	msgCounter := 0
	outboxEventStore := NewOutboxEventStore(
		eventStore,
		outboxStore,
		func(eventType string) string {
			return "events." + eventType
		},
		func() string {
			msgCounter++
			return "msg-" + string(rune('0'+msgCounter))
		},
	)

	// Append events
	events := []EventData{
		{EventType: "PatientCreated", Data: json.RawMessage(`{"name":"John"}`)},
		{EventType: "PatientUpdated", Data: json.RawMessage(`{"name":"Jane"}`)},
	}

	version, err := outboxEventStore.Append(ctx, "patient:123", VersionAny, events)
	if err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	if version != 1 {
		t.Errorf("Version = %d, want 1", version)
	}

	// Verify events were stored in event store
	storedEvents, err := eventStore.ReadStream(ctx, "patient:123", 0, 10)
	if err != nil {
		t.Fatalf("ReadStream failed: %v", err)
	}
	if len(storedEvents) != 2 {
		t.Errorf("Expected 2 stored events, got %d", len(storedEvents))
	}

	// Verify outbox messages were created
	pending, err := outboxStore.GetPendingMessages(ctx, 10)
	if err != nil {
		t.Fatalf("GetPendingMessages failed: %v", err)
	}
	if len(pending) != 2 {
		t.Errorf("Expected 2 outbox messages, got %d", len(pending))
	}

	// Verify topics were mapped correctly
	topics := make(map[string]bool)
	for _, msg := range pending {
		topics[msg.Topic] = true
	}
	if !topics["events.PatientCreated"] {
		t.Error("Expected topic events.PatientCreated")
	}
	if !topics["events.PatientUpdated"] {
		t.Error("Expected topic events.PatientUpdated")
	}
}

func TestOutboxEventStore_ReadStream(t *testing.T) {
	eventStore := NewMemoryStore()
	outboxStore := NewMemoryOutboxStore()
	ctx := context.Background()

	outboxEventStore := NewOutboxEventStore(
		eventStore,
		outboxStore,
		func(eventType string) string { return "topic" },
		func() string { return "msg-id" },
	)

	// Add events to underlying store
	eventStore.Append(ctx, "stream-1", VersionAny, []EventData{
		{EventType: "Event1", Data: json.RawMessage(`{}`)},
	})

	// Read through outbox store
	events, err := outboxEventStore.ReadStream(ctx, "stream-1", 0, 10)
	if err != nil {
		t.Fatalf("ReadStream failed: %v", err)
	}
	if len(events) != 1 {
		t.Errorf("Expected 1 event, got %d", len(events))
	}
}

func TestOutboxEventStore_ReadAll(t *testing.T) {
	eventStore := NewMemoryStore()
	outboxStore := NewMemoryOutboxStore()
	ctx := context.Background()

	outboxEventStore := NewOutboxEventStore(
		eventStore,
		outboxStore,
		func(eventType string) string { return "topic" },
		func() string { return "msg-id" },
	)

	// Add events
	eventStore.Append(ctx, "stream-1", VersionAny, []EventData{
		{EventType: "Event1", Data: json.RawMessage(`{}`)},
	})
	eventStore.Append(ctx, "stream-2", VersionAny, []EventData{
		{EventType: "Event2", Data: json.RawMessage(`{}`)},
	})

	// Read all through outbox store
	events, err := outboxEventStore.ReadAll(ctx, 0, 10)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}
	if len(events) != 2 {
		t.Errorf("Expected 2 events, got %d", len(events))
	}
}

// =============================================================================
// Concurrency Tests
// =============================================================================

func TestMemoryOutboxStore_Concurrent(t *testing.T) {
	store := NewMemoryOutboxStore()
	ctx := context.Background()

	var wg sync.WaitGroup
	numGoroutines := 10
	numOpsPerGoroutine := 100

	// Concurrent saves
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			for j := 0; j < numOpsPerGoroutine; j++ {
				msgID := "msg-" + string(rune('a'+goroutineID)) + "-" + string(rune('0'+j%10))
				store.SaveMessage(ctx, &OutboxMessage{
					ID:        msgID,
					EventType: "TestEvent",
					Topic:     "topic",
				})
			}
		}(i)
	}

	// Concurrent reads
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < numOpsPerGoroutine; j++ {
				store.GetPendingMessages(ctx, 10)
			}
		}()
	}

	wg.Wait()
	// If we get here without deadlock or panic, the test passes
}

// =============================================================================
// Integration Scenario Test
// =============================================================================

func TestOutbox_EndToEndScenario(t *testing.T) {
	eventStore := NewMemoryStore()
	outboxStore := NewMemoryOutboxStore()
	publisher := NewMockPublisher()
	ctx := context.Background()

	msgCounter := 0
	outboxEventStore := NewOutboxEventStore(
		eventStore,
		outboxStore,
		func(eventType string) string {
			return "healthcare." + eventType
		},
		func() string {
			msgCounter++
			return "msg-" + string(rune('0'+msgCounter))
		},
	)

	var publishedMessages []*OutboxMessage
	var mu sync.Mutex
	config := &OutboxRelayConfig{
		PollInterval: 10 * time.Millisecond,
		BatchSize:    10,
		MaxRetries:   3,
		OnPublished: func(msg *OutboxMessage) {
			mu.Lock()
			publishedMessages = append(publishedMessages, msg)
			mu.Unlock()
		},
	}
	relay := NewOutboxRelay(outboxStore, publisher, config)

	// Start relay
	relay.Start(ctx)
	defer relay.Stop()

	// Simulate business operations
	outboxEventStore.Append(ctx, "patient:P001", VersionAny, []EventData{
		{EventType: "PatientCreated", Data: json.RawMessage(`{"mrn":"P001","name":"John Doe"}`)},
	})

	outboxEventStore.Append(ctx, "patient:P001", VersionAny, []EventData{
		{EventType: "PatientUpdated", Data: json.RawMessage(`{"name":"John Smith"}`)},
	})

	outboxEventStore.Append(ctx, "claim:C001", VersionAny, []EventData{
		{EventType: "ClaimSubmitted", Data: json.RawMessage(`{"patient":"P001","amount":1500}`)},
	})

	// Wait for relay to process
	time.Sleep(100 * time.Millisecond)

	// Verify all events stored
	patientEvents, _ := eventStore.ReadStream(ctx, "patient:P001", 0, 10)
	if len(patientEvents) != 2 {
		t.Errorf("Expected 2 patient events, got %d", len(patientEvents))
	}

	claimEvents, _ := eventStore.ReadStream(ctx, "claim:C001", 0, 10)
	if len(claimEvents) != 1 {
		t.Errorf("Expected 1 claim event, got %d", len(claimEvents))
	}

	// Verify all messages published
	mu.Lock()
	numPublished := len(publishedMessages)
	mu.Unlock()

	if numPublished != 3 {
		t.Errorf("Expected 3 published messages, got %d", numPublished)
	}

	// Verify topics
	publisherMessages := publisher.GetPublished()
	topicCounts := make(map[string]int)
	for _, msg := range publisherMessages {
		topicCounts[msg.Topic]++
	}

	if topicCounts["healthcare.PatientCreated"] != 1 {
		t.Errorf("Expected 1 PatientCreated, got %d", topicCounts["healthcare.PatientCreated"])
	}
	if topicCounts["healthcare.PatientUpdated"] != 1 {
		t.Errorf("Expected 1 PatientUpdated, got %d", topicCounts["healthcare.PatientUpdated"])
	}
	if topicCounts["healthcare.ClaimSubmitted"] != 1 {
		t.Errorf("Expected 1 ClaimSubmitted, got %d", topicCounts["healthcare.ClaimSubmitted"])
	}
}
