package workflow

import (
	"testing"
)

func TestGetWorkflow(t *testing.T) {
	wf := &Workflow{
		Name:    "test-workflow",
		Version: "1.0",
		Routes: []Route{
			{
				Name:   "route1",
				Filter: Filter{EventType: StringOrSlice{"PATIENT_ADMIT"}},
				Actions: []Action{
					{Type: "log", Config: map[string]string{"level": "info"}},
				},
			},
		},
	}

	engine, err := NewEngine(wf)
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	got := engine.GetWorkflow()
	if got == nil {
		t.Fatal("GetWorkflow returned nil")
	}
	if got.Name != "test-workflow" {
		t.Errorf("GetWorkflow().Name = %q, want %q", got.Name, "test-workflow")
	}
	if len(got.Routes) != 1 {
		t.Errorf("GetWorkflow().Routes length = %d, want 1", len(got.Routes))
	}
}

func TestGetStats_ZeroBeforeProcessing(t *testing.T) {
	wf := &Workflow{Name: "test", Routes: []Route{}}
	engine, err := NewEngine(wf)
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	stats := engine.GetStats()
	if stats.EventsProcessed != 0 {
		t.Errorf("EventsProcessed = %d, want 0", stats.EventsProcessed)
	}
	if stats.Errors != 0 {
		t.Errorf("Errors = %d, want 0", stats.Errors)
	}
	if stats.LastEventTime != nil {
		t.Errorf("LastEventTime = %v, want nil", stats.LastEventTime)
	}
}

func TestGetStats_IncrementsAfterProcessing(t *testing.T) {
	wf := &Workflow{
		Name: "test",
		Routes: []Route{
			{
				Name:   "catch-all",
				Filter: Filter{},
				Actions: []Action{
					{Type: "log", Config: map[string]string{"level": "info"}},
				},
			},
		},
	}

	engine, err := NewEngine(wf)
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	event := map[string]interface{}{"type": "PATIENT_ADMIT", "source": "epic"}
	engine.Process(event)

	stats := engine.GetStats()
	if stats.EventsProcessed != 1 {
		t.Errorf("EventsProcessed = %d, want 1", stats.EventsProcessed)
	}
	if stats.LastEventTime == nil {
		t.Error("LastEventTime should not be nil after processing")
	}
	if stats.Errors != 0 {
		t.Errorf("Errors = %d, want 0", stats.Errors)
	}

	// Process another event
	engine.Process(event)
	stats = engine.GetStats()
	if stats.EventsProcessed != 2 {
		t.Errorf("EventsProcessed = %d, want 2", stats.EventsProcessed)
	}
}

func TestGetStats_CountsErrors(t *testing.T) {
	wf := &Workflow{
		Name: "test",
		Routes: []Route{
			{
				Name:   "catch-all",
				Filter: Filter{},
				Actions: []Action{
					{Type: "nonexistent_action", Config: map[string]string{}},
				},
			},
		},
	}

	engine, err := NewEngine(wf)
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	event := map[string]interface{}{"type": "TEST", "source": "test"}
	engine.Process(event)

	stats := engine.GetStats()
	if stats.EventsProcessed != 1 {
		t.Errorf("EventsProcessed = %d, want 1", stats.EventsProcessed)
	}
	if stats.Errors != 1 {
		t.Errorf("Errors = %d, want 1", stats.Errors)
	}
}
