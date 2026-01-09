package workflow

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMemoryRecorder_Record(t *testing.T) {
	recorder := NewMemoryRecorder()

	event := map[string]interface{}{
		"type":   "patient_admit",
		"source": "epic_adt",
	}

	err := recorder.Record(event, nil)
	if err != nil {
		t.Fatalf("Record failed: %v", err)
	}

	if recorder.Len() != 1 {
		t.Errorf("Expected 1 event, got %d", recorder.Len())
	}

	events, err := recorder.List(0)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(events))
	}

	recorded := events[0]
	if recorded.EventType != "patient_admit" {
		t.Errorf("Expected event_type 'patient_admit', got %q", recorded.EventType)
	}
	if recorded.Source != "epic_adt" {
		t.Errorf("Expected source 'epic_adt', got %q", recorded.Source)
	}
}

func TestMemoryRecorder_RecordWithResult(t *testing.T) {
	recorder := NewMemoryRecorder()

	event := map[string]interface{}{
		"type": "lab_result",
	}

	result := &Result{
		RouteResults: []RouteResult{
			{RouteName: "route1", Matched: true, ActionsRun: 2},
			{RouteName: "route2", Matched: false},
		},
	}

	err := recorder.Record(event, result)
	if err != nil {
		t.Fatalf("Record failed: %v", err)
	}

	events, _ := recorder.List(0)
	recorded := events[0]

	if recorded.ProcessingResult == nil {
		t.Fatal("Expected ProcessingResult to be set")
	}

	if len(recorded.ProcessingResult.RouteMatches) != 1 {
		t.Errorf("Expected 1 route match, got %d", len(recorded.ProcessingResult.RouteMatches))
	}

	if recorded.ProcessingResult.RouteMatches[0] != "route1" {
		t.Errorf("Expected route1, got %s", recorded.ProcessingResult.RouteMatches[0])
	}

	if recorded.ProcessingResult.ActionsExecuted["route1"] != 2 {
		t.Errorf("Expected 2 actions for route1, got %d", recorded.ProcessingResult.ActionsExecuted["route1"])
	}
}

func TestMemoryRecorder_WithMaxSize(t *testing.T) {
	recorder := NewMemoryRecorder(WithMaxSize(3))

	for i := 0; i < 5; i++ {
		event := map[string]interface{}{
			"type":  "event",
			"index": i,
		}
		recorder.Record(event, nil)
	}

	if recorder.Len() != 3 {
		t.Errorf("Expected max 3 events, got %d", recorder.Len())
	}

	events, _ := recorder.List(0)
	// Should have events 2, 3, 4 (oldest dropped)
	for i, e := range events {
		eventMap := e.Event.(map[string]interface{})
		expectedIndex := i + 2
		if int(eventMap["index"].(int)) != expectedIndex {
			t.Errorf("Expected index %d, got %v", expectedIndex, eventMap["index"])
		}
	}
}

func TestMemoryRecorder_GetByID(t *testing.T) {
	recorder := NewMemoryRecorder()

	event := map[string]interface{}{"type": "test"}
	recorder.Record(event, nil)

	events, _ := recorder.List(0)
	id := events[0].ID

	got, err := recorder.Get(id)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if got == nil {
		t.Fatal("Expected event, got nil")
	}

	if got.ID != id {
		t.Errorf("Expected ID %s, got %s", id, got.ID)
	}

	// Non-existent ID
	got, err = recorder.Get("nonexistent")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got != nil {
		t.Errorf("Expected nil for nonexistent ID, got %v", got)
	}
}

func TestMemoryRecorder_ExportImport(t *testing.T) {
	tmpDir := t.TempDir()
	exportPath := filepath.Join(tmpDir, "events.json")

	// Create and populate recorder
	recorder := NewMemoryRecorder()
	for i := 0; i < 3; i++ {
		event := map[string]interface{}{
			"type":  "test",
			"index": i,
		}
		recorder.Record(event, nil)
	}

	// Export
	err := recorder.Export(exportPath)
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(exportPath); os.IsNotExist(err) {
		t.Fatal("Export file not created")
	}

	// Import into new recorder
	recorder2 := NewMemoryRecorder()
	err = recorder2.Import(exportPath)
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	if recorder2.Len() != 3 {
		t.Errorf("Expected 3 events after import, got %d", recorder2.Len())
	}
}

func TestFileRecorder_RecordAndList(t *testing.T) {
	tmpDir := t.TempDir()

	recorder, err := NewFileRecorder(tmpDir)
	if err != nil {
		t.Fatalf("NewFileRecorder failed: %v", err)
	}

	for i := 0; i < 3; i++ {
		event := map[string]interface{}{
			"type":  "test",
			"index": i,
		}
		if err := recorder.Record(event, nil); err != nil {
			t.Fatalf("Record failed: %v", err)
		}
		// Small delay to ensure different timestamps
		time.Sleep(10 * time.Millisecond)
	}

	if recorder.Len() != 3 {
		t.Errorf("Expected 3 events, got %d", recorder.Len())
	}

	events, err := recorder.List(0)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(events) != 3 {
		t.Errorf("Expected 3 events, got %d", len(events))
	}

	// Verify files exist
	files, _ := os.ReadDir(tmpDir)
	jsonCount := 0
	for _, f := range files {
		if filepath.Ext(f.Name()) == ".json" {
			jsonCount++
		}
	}
	if jsonCount != 3 {
		t.Errorf("Expected 3 JSON files, got %d", jsonCount)
	}
}

func TestFileRecorder_Clear(t *testing.T) {
	tmpDir := t.TempDir()

	recorder, _ := NewFileRecorder(tmpDir)
	for i := 0; i < 3; i++ {
		recorder.Record(map[string]interface{}{"type": "test"}, nil)
	}

	if recorder.Len() != 3 {
		t.Fatalf("Setup: expected 3 events")
	}

	if err := recorder.Clear(); err != nil {
		t.Fatalf("Clear failed: %v", err)
	}

	if recorder.Len() != 0 {
		t.Errorf("Expected 0 events after clear, got %d", recorder.Len())
	}
}

func TestEventReplayer_Replay(t *testing.T) {
	// Create workflow
	workflow := &Workflow{
		Name: "test",
		Routes: []Route{
			{
				Name: "patient_route",
				Filter: Filter{
					EventType: StringOrSlice{"patient_admit"},
				},
				Actions: []Action{
					{Type: "log", Config: map[string]string{"level": "info"}},
				},
			},
		},
	}

	engine, _ := NewEngine(workflow)

	// Record some events
	recorder := NewMemoryRecorder()
	events := []map[string]interface{}{
		{"type": "patient_admit", "source": "epic"},
		{"type": "patient_admit", "source": "cerner"},
		{"type": "lab_result", "source": "epic"},
	}

	for _, event := range events {
		result := engine.Process(event)
		recorder.Record(event, result)
	}

	// Create replayer and replay
	replayer := NewEventReplayer(engine, recorder)
	summary, err := replayer.Replay(context.Background(), nil)
	if err != nil {
		t.Fatalf("Replay failed: %v", err)
	}

	if summary.TotalEvents != 3 {
		t.Errorf("Expected 3 events replayed, got %d", summary.TotalEvents)
	}

	// All routing should match (same workflow)
	if summary.MatchedRouting != 3 {
		t.Errorf("Expected 3 routing matches, got %d", summary.MatchedRouting)
	}

	if summary.DifferentRouting != 0 {
		t.Errorf("Expected 0 routing differences, got %d", summary.DifferentRouting)
	}
}

func TestEventReplayer_ReplayWithFilter(t *testing.T) {
	workflow := &Workflow{
		Name: "test",
		Routes: []Route{
			{
				Name:    "default",
				Filter:  Filter{},
				Actions: []Action{{Type: "log"}},
			},
		},
	}

	engine, _ := NewEngine(workflow)
	recorder := NewMemoryRecorder()

	// Record events with different types
	for _, eventType := range []string{"patient_admit", "lab_result", "patient_admit"} {
		event := map[string]interface{}{"type": eventType}
		recorder.Record(event, nil)
	}

	replayer := NewEventReplayer(engine, recorder)

	// Filter by event type
	summary, _ := replayer.Replay(context.Background(), &ReplayConfig{
		EventTypes: []string{"patient_admit"},
	})

	if summary.TotalEvents != 2 {
		t.Errorf("Expected 2 filtered events, got %d", summary.TotalEvents)
	}
}

func TestEventReplayer_ReplayDetectsDifferences(t *testing.T) {
	// Create original workflow
	workflow1 := &Workflow{
		Name: "v1",
		Routes: []Route{
			{
				Name:    "route1",
				Filter:  Filter{EventType: StringOrSlice{"patient_admit"}},
				Actions: []Action{{Type: "log"}},
			},
		},
	}

	engine1, _ := NewEngine(workflow1)
	recorder := NewMemoryRecorder()

	// Record with original workflow
	event := map[string]interface{}{"type": "patient_admit"}
	result := engine1.Process(event)
	recorder.Record(event, result)

	// Create modified workflow with additional route
	workflow2 := &Workflow{
		Name: "v2",
		Routes: []Route{
			{
				Name:    "route1",
				Filter:  Filter{EventType: StringOrSlice{"patient_admit"}},
				Actions: []Action{{Type: "log"}},
			},
			{
				Name:    "route2", // New route
				Filter:  Filter{EventType: StringOrSlice{"patient_admit"}},
				Actions: []Action{{Type: "webhook"}},
			},
		},
	}

	engine2, _ := NewEngine(workflow2)

	// Replay with modified workflow
	replayer := NewEventReplayer(engine2, recorder)
	summary, _ := replayer.Replay(context.Background(), nil)

	if summary.DifferentRouting != 1 {
		t.Errorf("Expected 1 routing difference, got %d", summary.DifferentRouting)
	}

	if len(summary.Diffs) != 1 {
		t.Fatalf("Expected 1 diff, got %d", len(summary.Diffs))
	}

	diff := summary.Diffs[0]
	if len(diff.AddedRoutes) != 1 || diff.AddedRoutes[0] != "route2" {
		t.Errorf("Expected route2 to be added, got %v", diff.AddedRoutes)
	}
}

func TestEventReplayer_ReplayOne(t *testing.T) {
	workflow := &Workflow{
		Name: "test",
		Routes: []Route{
			{
				Name:    "route1",
				Filter:  Filter{EventType: StringOrSlice{"test"}},
				Actions: []Action{{Type: "log"}},
			},
		},
	}

	engine, _ := NewEngine(workflow)
	recorder := NewMemoryRecorder()

	event := map[string]interface{}{"type": "test"}
	result := engine.Process(event)
	recorder.Record(event, result)

	events, _ := recorder.List(0)
	id := events[0].ID

	replayer := NewEventReplayer(engine, recorder)
	replayResult, diff, err := replayer.ReplayOne(context.Background(), id)

	if err != nil {
		t.Fatalf("ReplayOne failed: %v", err)
	}

	if replayResult == nil {
		t.Fatal("Expected result, got nil")
	}

	if diff == nil {
		t.Fatal("Expected diff, got nil")
	}

	if !diff.RoutingMatch {
		t.Error("Expected routing to match")
	}
}

func TestRecordingEngine(t *testing.T) {
	workflow := &Workflow{
		Name: "test",
		Routes: []Route{
			{
				Name:    "route1",
				Filter:  Filter{EventType: StringOrSlice{"test"}},
				Actions: []Action{{Type: "log"}},
			},
		},
	}

	recorder := NewMemoryRecorder()
	engine, err := NewRecordingEngine(workflow, recorder)
	if err != nil {
		t.Fatalf("NewRecordingEngine failed: %v", err)
	}

	// Process events
	for i := 0; i < 5; i++ {
		event := map[string]interface{}{"type": "test", "index": i}
		engine.Process(event)
	}

	// Verify recording
	if recorder.Len() != 5 {
		t.Errorf("Expected 5 recorded events, got %d", recorder.Len())
	}

	events, _ := recorder.List(0)
	for i, e := range events {
		if e.ProcessingResult == nil {
			t.Errorf("Event %d: expected processing result", i)
		}
	}
}

func TestLoadSaveRecordedEvents(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "events.json")

	events := []*RecordedEvent{
		{
			ID:        "evt-1",
			EventType: "patient_admit",
			Source:    "epic",
			Event:     map[string]interface{}{"type": "patient_admit"},
		},
		{
			ID:        "evt-2",
			EventType: "lab_result",
			Source:    "lab_system",
			Event:     map[string]interface{}{"type": "lab_result"},
		},
	}

	// Save
	if err := SaveRecordedEvents(path, events); err != nil {
		t.Fatalf("SaveRecordedEvents failed: %v", err)
	}

	// Load
	loaded, err := LoadRecordedEvents(path)
	if err != nil {
		t.Fatalf("LoadRecordedEvents failed: %v", err)
	}

	if len(loaded) != 2 {
		t.Errorf("Expected 2 events, got %d", len(loaded))
	}

	if loaded[0].ID != "evt-1" {
		t.Errorf("Expected ID 'evt-1', got %q", loaded[0].ID)
	}
}

func TestReplayWithCallback(t *testing.T) {
	workflow := &Workflow{
		Name: "test",
		Routes: []Route{
			{
				Name:    "route1",
				Filter:  Filter{},
				Actions: []Action{{Type: "log"}},
			},
		},
	}

	engine, _ := NewEngine(workflow)
	recorder := NewMemoryRecorder()

	for i := 0; i < 3; i++ {
		event := map[string]interface{}{"type": "test", "index": i}
		result := engine.Process(event)
		recorder.Record(event, result)
	}

	replayer := NewEventReplayer(engine, recorder)

	callbackCount := 0
	summary, _ := replayer.Replay(context.Background(), &ReplayConfig{
		Callback: func(recorded *RecordedEvent, result *Result, diff *ReplayDiff) {
			callbackCount++
		},
	})

	if callbackCount != summary.TotalEvents {
		t.Errorf("Callback called %d times, expected %d", callbackCount, summary.TotalEvents)
	}
}

func TestReplayContextCancellation(t *testing.T) {
	workflow := &Workflow{
		Name:   "test",
		Routes: []Route{{Name: "r", Filter: Filter{}, Actions: []Action{{Type: "log"}}}},
	}

	engine, _ := NewEngine(workflow)
	recorder := NewMemoryRecorder()

	for i := 0; i < 100; i++ {
		recorder.Record(map[string]interface{}{"type": "test"}, nil)
	}

	replayer := NewEventReplayer(engine, recorder)

	ctx, cancel := context.WithCancel(context.Background())

	processed := 0
	go func() {
		_, _ = replayer.Replay(ctx, &ReplayConfig{
			Callback: func(recorded *RecordedEvent, result *Result, diff *ReplayDiff) {
				processed++
				if processed >= 10 {
					cancel()
				}
			},
		})
	}()

	<-ctx.Done()

	// Should have stopped early
	if processed >= 100 {
		t.Error("Expected replay to stop early due to context cancellation")
	}
}
