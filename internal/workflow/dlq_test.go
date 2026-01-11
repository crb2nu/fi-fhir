//nolint:gosec // Test file - G104 errors intentionally ignored in test setup
package workflow

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// MemoryDLQ Tests

func TestMemoryDLQPushAndPop(t *testing.T) {
	dlq := NewMemoryDLQ()

	event := &FailedEvent{
		ID:           "test-1",
		Event:        map[string]string{"type": "test"},
		RouteName:    "test_route",
		ActionType:   "webhook",
		Error:        "test error",
		ErrorType:    "test_error",
		Attempts:     1,
		FirstFailure: time.Now(),
		LastFailure:  time.Now(),
	}

	// Push
	if err := dlq.Push(event); err != nil {
		t.Fatalf("Push failed: %v", err)
	}

	if dlq.Len() != 1 {
		t.Errorf("expected length 1, got %d", dlq.Len())
	}

	// Pop
	popped, err := dlq.Pop()
	if err != nil {
		t.Fatalf("Pop failed: %v", err)
	}

	if popped.ID != event.ID {
		t.Errorf("expected ID %s, got %s", event.ID, popped.ID)
	}

	if dlq.Len() != 0 {
		t.Errorf("expected length 0 after pop, got %d", dlq.Len())
	}
}

func TestMemoryDLQPushNil(t *testing.T) {
	dlq := NewMemoryDLQ()

	if err := dlq.Push(nil); err == nil {
		t.Error("expected error when pushing nil event")
	}
}

func TestMemoryDLQPushEmptyID(t *testing.T) {
	dlq := NewMemoryDLQ()

	event := &FailedEvent{
		ID:    "",
		Event: map[string]string{"type": "test"},
	}

	if err := dlq.Push(event); err == nil {
		t.Error("expected error when pushing event with empty ID")
	}
}

func TestMemoryDLQPushUpdate(t *testing.T) {
	dlq := NewMemoryDLQ()

	event := &FailedEvent{
		ID:       "test-1",
		Attempts: 1,
		Error:    "first error",
	}

	dlq.Push(event)

	// Update same event
	event.Attempts = 2
	event.Error = "second error"
	dlq.Push(event)

	// Should still have 1 event
	if dlq.Len() != 1 {
		t.Errorf("expected length 1 after update, got %d", dlq.Len())
	}

	// Fetch and verify update
	fetched, _ := dlq.Get("test-1")
	if fetched.Attempts != 2 {
		t.Errorf("expected attempts 2, got %d", fetched.Attempts)
	}
	if fetched.Error != "second error" {
		t.Errorf("expected 'second error', got %s", fetched.Error)
	}
}

func TestMemoryDLQPopEmpty(t *testing.T) {
	dlq := NewMemoryDLQ()

	popped, err := dlq.Pop()
	if err != nil {
		t.Fatalf("Pop on empty should not error: %v", err)
	}
	if popped != nil {
		t.Error("expected nil on empty pop")
	}
}

func TestMemoryDLQPeek(t *testing.T) {
	dlq := NewMemoryDLQ()

	event := &FailedEvent{ID: "test-1"}
	dlq.Push(event)

	// Peek should return event without removing
	peeked, _ := dlq.Peek()
	if peeked.ID != "test-1" {
		t.Errorf("expected ID test-1, got %s", peeked.ID)
	}

	// Should still be there
	if dlq.Len() != 1 {
		t.Error("peek should not remove event")
	}
}

func TestMemoryDLQList(t *testing.T) {
	dlq := NewMemoryDLQ()

	for i := 0; i < 5; i++ {
		dlq.Push(&FailedEvent{ID: string(rune('a' + i))})
	}

	// List all
	events, _ := dlq.List(0)
	if len(events) != 5 {
		t.Errorf("expected 5 events, got %d", len(events))
	}

	// List with limit
	events, _ = dlq.List(3)
	if len(events) != 3 {
		t.Errorf("expected 3 events with limit, got %d", len(events))
	}
}

func TestMemoryDLQGetAndRemove(t *testing.T) {
	dlq := NewMemoryDLQ()

	dlq.Push(&FailedEvent{ID: "test-1"})
	dlq.Push(&FailedEvent{ID: "test-2"})

	// Get
	event, _ := dlq.Get("test-1")
	if event == nil || event.ID != "test-1" {
		t.Error("Get failed to retrieve event")
	}

	// Get non-existent
	event, _ = dlq.Get("non-existent")
	if event != nil {
		t.Error("expected nil for non-existent ID")
	}

	// Remove
	if err := dlq.Remove("test-1"); err != nil {
		t.Errorf("Remove failed: %v", err)
	}

	if dlq.Len() != 1 {
		t.Errorf("expected length 1 after remove, got %d", dlq.Len())
	}

	// Remove non-existent (should not error)
	if err := dlq.Remove("non-existent"); err != nil {
		t.Errorf("Remove non-existent should not error: %v", err)
	}
}

func TestMemoryDLQClear(t *testing.T) {
	dlq := NewMemoryDLQ()

	for i := 0; i < 3; i++ {
		dlq.Push(&FailedEvent{ID: string(rune('a' + i))})
	}

	if err := dlq.Clear(); err != nil {
		t.Errorf("Clear failed: %v", err)
	}

	if dlq.Len() != 0 {
		t.Errorf("expected length 0 after clear, got %d", dlq.Len())
	}
}

func TestMemoryDLQOrdering(t *testing.T) {
	dlq := NewMemoryDLQ()

	dlq.Push(&FailedEvent{ID: "first"})
	dlq.Push(&FailedEvent{ID: "second"})
	dlq.Push(&FailedEvent{ID: "third"})

	// Pop should return in FIFO order
	first, _ := dlq.Pop()
	if first.ID != "first" {
		t.Errorf("expected 'first', got %s", first.ID)
	}

	second, _ := dlq.Pop()
	if second.ID != "second" {
		t.Errorf("expected 'second', got %s", second.ID)
	}
}

// FileDLQ Tests

func TestFileDLQPushAndPop(t *testing.T) {
	dir := t.TempDir()
	dlq, err := NewFileDLQ(dir)
	if err != nil {
		t.Fatalf("Failed to create FileDLQ: %v", err)
	}

	event := &FailedEvent{
		ID:           "test-file-1",
		Event:        map[string]interface{}{"type": "test", "value": 42},
		RouteName:    "test_route",
		ActionType:   "fhir",
		Error:        "test error",
		ErrorType:    "server_error",
		Attempts:     1,
		FirstFailure: time.Now(),
		LastFailure:  time.Now(),
	}

	// Push
	if err := dlq.Push(event); err != nil {
		t.Fatalf("Push failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(filepath.Join(dir, "test-file-1.json")); os.IsNotExist(err) {
		t.Error("expected file to be created")
	}

	// Pop
	popped, err := dlq.Pop()
	if err != nil {
		t.Fatalf("Pop failed: %v", err)
	}

	if popped.ID != event.ID {
		t.Errorf("expected ID %s, got %s", event.ID, popped.ID)
	}

	// Verify file removed
	if _, err := os.Stat(filepath.Join(dir, "test-file-1.json")); !os.IsNotExist(err) {
		t.Error("expected file to be removed after pop")
	}
}

func TestFileDLQGet(t *testing.T) {
	dir := t.TempDir()
	dlq, _ := NewFileDLQ(dir)

	dlq.Push(&FailedEvent{ID: "test-1", Error: "error 1"})

	event, err := dlq.Get("test-1")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if event.Error != "error 1" {
		t.Errorf("expected 'error 1', got %s", event.Error)
	}

	// Get non-existent
	event, err = dlq.Get("non-existent")
	if err != nil {
		t.Fatalf("Get non-existent should not error: %v", err)
	}
	if event != nil {
		t.Error("expected nil for non-existent")
	}
}

func TestFileDLQList(t *testing.T) {
	dir := t.TempDir()
	dlq, _ := NewFileDLQ(dir)

	for i := 0; i < 5; i++ {
		dlq.Push(&FailedEvent{ID: string(rune('a' + i))})
		time.Sleep(10 * time.Millisecond) // Ensure different mod times
	}

	events, err := dlq.List(0)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(events) != 5 {
		t.Errorf("expected 5 events, got %d", len(events))
	}

	// List with limit
	events, _ = dlq.List(2)
	if len(events) != 2 {
		t.Errorf("expected 2 events with limit, got %d", len(events))
	}
}

func TestFileDLQRemove(t *testing.T) {
	dir := t.TempDir()
	dlq, _ := NewFileDLQ(dir)

	dlq.Push(&FailedEvent{ID: "to-remove"})

	if err := dlq.Remove("to-remove"); err != nil {
		t.Errorf("Remove failed: %v", err)
	}

	if dlq.Len() != 0 {
		t.Errorf("expected length 0 after remove, got %d", dlq.Len())
	}
}

func TestFileDLQClear(t *testing.T) {
	dir := t.TempDir()
	dlq, _ := NewFileDLQ(dir)

	for i := 0; i < 3; i++ {
		dlq.Push(&FailedEvent{ID: string(rune('a' + i))})
	}

	if err := dlq.Clear(); err != nil {
		t.Errorf("Clear failed: %v", err)
	}

	if dlq.Len() != 0 {
		t.Errorf("expected length 0 after clear, got %d", dlq.Len())
	}
}

// Error Classification Tests

func TestClassifyError(t *testing.T) {
	tests := []struct {
		err      error
		expected string
	}{
		{ErrCircuitOpen, "circuit_open"},
		{errors.New("connection timeout exceeded"), "timeout"},
		{errors.New("deadline exceeded"), "timeout"},
		{errors.New("connection refused"), "connection_error"},
		{errors.New("no such host"), "connection_error"},
		{errors.New("401 unauthorized"), "auth_error"},
		{errors.New("authentication failed"), "auth_error"},
		{errors.New("server returned 500"), "server_error"},
		{errors.New("503 service unavailable"), "server_error"},
		{errors.New("400 bad request"), "client_error"},
		{errors.New("404 not found"), "client_error"},
		{errors.New("random error"), "unknown"},
		{nil, ""},
	}

	for _, tt := range tests {
		result := ClassifyError(tt.err)
		if result != tt.expected {
			errStr := "<nil>"
			if tt.err != nil {
				errStr = tt.err.Error()
			}
			t.Errorf("ClassifyError(%q) = %q, want %q", errStr, result, tt.expected)
		}
	}
}

// DLQ Config Tests

func TestDefaultDLQConfig(t *testing.T) {
	config := DefaultDLQConfig()

	if config.MaxAttempts != 5 {
		t.Errorf("expected MaxAttempts 5, got %d", config.MaxAttempts)
	}
	if config.RetentionPeriod != 7*24*time.Hour {
		t.Errorf("expected RetentionPeriod 7 days, got %v", config.RetentionPeriod)
	}
}

// Generate ID Test

func TestGenerateFailedEventID(t *testing.T) {
	id1 := GenerateFailedEventID()
	id2 := GenerateFailedEventID()

	if id1 == "" {
		t.Error("generated ID should not be empty")
	}

	if id1 == id2 {
		t.Error("generated IDs should be unique")
	}
}

// Engine Integration Tests

func TestEngineDLQIntegration(t *testing.T) {
	workflow := &Workflow{
		Name: "test_workflow",
		Routes: []Route{
			{
				Name: "failing_route",
				Filter: Filter{
					EventType: StringOrSlice{"test_event"},
				},
				Actions: []Action{
					{
						Type:   "failing_action",
						Config: map[string]string{},
					},
				},
			},
		},
	}

	engine, _ := NewEngine(workflow)

	// Register a failing action
	engine.RegisterAction("failing_action", ActionHandlerFunc(func(event interface{}, config map[string]string) error {
		return errors.New("simulated failure")
	}))

	// Set up DLQ
	dlq := NewMemoryDLQ()
	engine.SetDLQ(dlq)

	// Process an event
	event := map[string]interface{}{
		"type":   "test_event",
		"source": "test",
	}
	result := engine.Process(event)

	// Should have errors
	if !result.HasErrors() {
		t.Error("expected errors from failing action")
	}

	// Should be in DLQ
	if dlq.Len() != 1 {
		t.Errorf("expected 1 event in DLQ, got %d", dlq.Len())
	}

	// Verify DLQ event
	failed, _ := dlq.Peek()
	if failed.RouteName != "failing_route" {
		t.Errorf("expected route name 'failing_route', got %s", failed.RouteName)
	}
	if failed.ActionType != "failing_action" {
		t.Errorf("expected action type 'failing_action', got %s", failed.ActionType)
	}
}

func TestEngineDLQCallback(t *testing.T) {
	workflow := &Workflow{
		Name: "test_workflow",
		Routes: []Route{
			{
				Name:   "test_route",
				Filter: Filter{EventType: StringOrSlice{"test"}},
				Actions: []Action{
					{Type: "fail", Config: map[string]string{}},
				},
			},
		},
	}

	engine, _ := NewEngine(workflow)
	engine.RegisterAction("fail", ActionHandlerFunc(func(event interface{}, config map[string]string) error {
		return errors.New("fail")
	}))

	callbackCalled := false
	var callbackEvent *FailedEvent

	config := DLQConfig{
		MaxAttempts: 5,
		OnDeadLetter: func(event *FailedEvent) {
			callbackCalled = true
			callbackEvent = event
		},
	}

	dlq := NewMemoryDLQ()
	engine.SetDLQ(dlq, config)

	engine.Process(map[string]interface{}{"type": "test"})

	if !callbackCalled {
		t.Error("OnDeadLetter callback should have been called")
	}
	if callbackEvent == nil || callbackEvent.RouteName != "test_route" {
		t.Error("callback should receive the failed event")
	}
}

func TestEngineReprocessDLQ(t *testing.T) {
	failCount := 0

	workflow := &Workflow{
		Name: "test_workflow",
		Routes: []Route{
			{
				Name:   "test_route",
				Filter: Filter{EventType: StringOrSlice{"test"}},
				Actions: []Action{
					{Type: "conditional_fail", Config: map[string]string{}},
				},
			},
		},
	}

	engine, _ := NewEngine(workflow)

	// Action fails first 2 times, then succeeds
	engine.RegisterAction("conditional_fail", ActionHandlerFunc(func(event interface{}, config map[string]string) error {
		failCount++
		if failCount <= 2 {
			return errors.New("temporary failure")
		}
		return nil
	}))

	dlq := NewMemoryDLQ()
	engine.SetDLQ(dlq)

	// First process - should fail and go to DLQ
	engine.Process(map[string]interface{}{"type": "test"})
	if dlq.Len() != 1 {
		t.Fatalf("expected 1 event in DLQ, got %d", dlq.Len())
	}

	// First reprocess - should fail again
	result := engine.ReprocessDLQ(0)
	if result.Failed != 1 || result.Processed != 0 {
		t.Errorf("expected 1 failed, 0 processed, got %d failed, %d processed", result.Failed, result.Processed)
	}

	// Second reprocess - should succeed now (3rd attempt total)
	result = engine.ReprocessDLQ(0)
	if result.Processed != 1 || result.Failed != 0 {
		t.Errorf("expected 1 processed, 0 failed, got %d processed, %d failed", result.Processed, result.Failed)
	}

	// DLQ should be empty
	if dlq.Len() != 0 {
		t.Errorf("expected DLQ to be empty, got %d", dlq.Len())
	}
}

func TestEngineReprocessDLQMaxAttempts(t *testing.T) {
	workflow := &Workflow{
		Name: "test_workflow",
		Routes: []Route{
			{
				Name:   "test_route",
				Filter: Filter{EventType: StringOrSlice{"test"}},
				Actions: []Action{
					{Type: "always_fail", Config: map[string]string{}},
				},
			},
		},
	}

	engine, _ := NewEngine(workflow)
	engine.RegisterAction("always_fail", ActionHandlerFunc(func(event interface{}, config map[string]string) error {
		return errors.New("always fails")
	}))

	dlq := NewMemoryDLQ()
	config := DLQConfig{MaxAttempts: 3}
	engine.SetDLQ(dlq, config)

	// Process - goes to DLQ with 1 attempt
	engine.Process(map[string]interface{}{"type": "test"})

	// Reprocess twice more to hit max attempts
	engine.ReprocessDLQ(0) // 2 attempts
	engine.ReprocessDLQ(0) // 3 attempts

	// Next reprocess should skip due to max attempts
	result := engine.ReprocessDLQ(0)
	if result.Skipped != 1 {
		t.Errorf("expected 1 skipped, got %d", result.Skipped)
	}
}

func TestEngineReprocessDLQEvent(t *testing.T) {
	workflow := &Workflow{
		Name: "test_workflow",
		Routes: []Route{
			{
				Name:   "test_route",
				Filter: Filter{EventType: StringOrSlice{"test"}},
				Actions: []Action{
					{Type: "succeed", Config: map[string]string{}},
				},
			},
		},
	}

	engine, _ := NewEngine(workflow)
	engine.RegisterAction("succeed", ActionHandlerFunc(func(event interface{}, config map[string]string) error {
		return nil
	}))

	dlq := NewMemoryDLQ()
	engine.SetDLQ(dlq)

	// Manually add a failed event
	failedEvent := &FailedEvent{
		ID:        "manual-1",
		Event:     map[string]interface{}{"type": "test"},
		RouteName: "test_route",
		Attempts:  1,
	}
	dlq.Push(failedEvent)

	// Reprocess specific event
	result, err := engine.ReprocessDLQEvent("manual-1")
	if err != nil {
		t.Fatalf("ReprocessDLQEvent failed: %v", err)
	}

	if result.HasErrors() {
		t.Error("expected no errors from reprocessing")
	}

	// Should be removed from DLQ
	if dlq.Len() != 0 {
		t.Errorf("expected DLQ to be empty, got %d", dlq.Len())
	}
}

func TestEngineReprocessDLQEventNotFound(t *testing.T) {
	workflow := &Workflow{Name: "test"}
	engine, _ := NewEngine(workflow)

	dlq := NewMemoryDLQ()
	engine.SetDLQ(dlq)

	_, err := engine.ReprocessDLQEvent("non-existent")
	if err == nil {
		t.Error("expected error for non-existent event")
	}
}

func TestEngineReprocessDLQNoDLQ(t *testing.T) {
	workflow := &Workflow{Name: "test"}
	engine, _ := NewEngine(workflow)

	// No DLQ configured
	result := engine.ReprocessDLQ(0)
	if result.Processed != 0 && result.Failed != 0 && result.Skipped != 0 {
		t.Error("expected empty result when no DLQ configured")
	}

	_, err := engine.ReprocessDLQEvent("test")
	if err == nil {
		t.Error("expected error when no DLQ configured")
	}
}

func TestEngineGetDLQ(t *testing.T) {
	workflow := &Workflow{Name: "test"}
	engine, _ := NewEngine(workflow)

	// No DLQ
	if engine.GetDLQ() != nil {
		t.Error("expected nil DLQ initially")
	}

	// Set DLQ
	dlq := NewMemoryDLQ()
	engine.SetDLQ(dlq)

	if engine.GetDLQ() != dlq {
		t.Error("expected to get the same DLQ that was set")
	}
}
