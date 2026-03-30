package workflow

import (
	"context"
	"testing"
	"time"
)

func TestNewDebugSession(t *testing.T) {
	w := &Workflow{
		Name: "test-debug",
		Routes: []Route{
			{
				Name:    "route-a",
				Filter:  Filter{EventType: StringOrSlice{"TEST"}},
				Actions: []Action{{Type: "log", Config: map[string]string{"message": "hit route-a"}}},
			},
		},
	}

	engine, err := NewEngine(w)
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	session := NewDebugSession("test-session-1", engine)

	if session.ID != "test-session-1" {
		t.Errorf("Expected ID 'test-session-1', got '%s'", session.ID)
	}
	if session.State != DebugStateIdle {
		t.Errorf("Expected state idle, got '%s'", session.State)
	}
	if session.Engine != engine {
		t.Error("Expected engine to be set")
	}
	if len(session.Breakpoints) != 0 {
		t.Errorf("Expected 0 breakpoints, got %d", len(session.Breakpoints))
	}
	if len(session.Steps) != 0 {
		t.Errorf("Expected 0 steps, got %d", len(session.Steps))
	}
	if session.tracer == nil {
		t.Error("Expected tracer to be set")
	}
}

func TestSetBreakpoint(t *testing.T) {
	w := &Workflow{
		Name: "test-debug",
		Routes: []Route{
			{
				Name:    "route-a",
				Filter:  Filter{EventType: StringOrSlice{"TEST"}},
				Actions: []Action{{Type: "log", Config: map[string]string{"message": "hit"}}},
			},
		},
	}

	engine, err := NewEngine(w)
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	session := NewDebugSession("test-bp", engine)

	bp := &Breakpoint{
		ID:      "bp-1",
		Type:    BreakpointRoute,
		Name:    "route-a",
		Enabled: true,
	}
	session.SetBreakpoint(bp)

	if len(session.Breakpoints) != 1 {
		t.Fatalf("Expected 1 breakpoint, got %d", len(session.Breakpoints))
	}
	if session.Breakpoints["bp-1"].Name != "route-a" {
		t.Errorf("Expected breakpoint name 'route-a', got '%s'", session.Breakpoints["bp-1"].Name)
	}
	if session.Breakpoints["bp-1"].Type != BreakpointRoute {
		t.Errorf("Expected breakpoint type 'route', got '%s'", session.Breakpoints["bp-1"].Type)
	}
	if !session.Breakpoints["bp-1"].Enabled {
		t.Error("Expected breakpoint to be enabled")
	}

	// Update existing breakpoint
	bp2 := &Breakpoint{
		ID:      "bp-1",
		Type:    BreakpointAction,
		Name:    "log",
		Enabled: false,
	}
	session.SetBreakpoint(bp2)

	if len(session.Breakpoints) != 1 {
		t.Fatalf("Expected 1 breakpoint after update, got %d", len(session.Breakpoints))
	}
	if session.Breakpoints["bp-1"].Name != "log" {
		t.Errorf("Expected updated name 'log', got '%s'", session.Breakpoints["bp-1"].Name)
	}
}

func TestRemoveBreakpoint(t *testing.T) {
	w := &Workflow{
		Name: "test-debug",
		Routes: []Route{
			{
				Name:    "route-a",
				Filter:  Filter{EventType: StringOrSlice{"TEST"}},
				Actions: []Action{{Type: "log", Config: map[string]string{"message": "hit"}}},
			},
		},
	}

	engine, err := NewEngine(w)
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	session := NewDebugSession("test-rm", engine)

	session.SetBreakpoint(&Breakpoint{ID: "bp-1", Type: BreakpointRoute, Name: "route-a", Enabled: true})
	session.SetBreakpoint(&Breakpoint{ID: "bp-2", Type: BreakpointAction, Name: "log", Enabled: true})

	// Remove existing breakpoint
	removed := session.RemoveBreakpoint("bp-1")
	if !removed {
		t.Error("Expected RemoveBreakpoint to return true for existing breakpoint")
	}
	if len(session.Breakpoints) != 1 {
		t.Errorf("Expected 1 breakpoint after removal, got %d", len(session.Breakpoints))
	}

	// Remove non-existent breakpoint
	removed = session.RemoveBreakpoint("bp-nonexistent")
	if removed {
		t.Error("Expected RemoveBreakpoint to return false for non-existent breakpoint")
	}
	if len(session.Breakpoints) != 1 {
		t.Errorf("Expected 1 breakpoint after non-existent removal, got %d", len(session.Breakpoints))
	}
}

func TestDebugSession_StepThrough(t *testing.T) {
	w := &Workflow{
		Name: "test-debug-step",
		Routes: []Route{
			{
				Name:    "route-a",
				Filter:  Filter{EventType: StringOrSlice{"TEST"}},
				Actions: []Action{{Type: "log", Config: map[string]string{"message": "hit route-a"}}},
			},
			{
				Name:    "route-b",
				Filter:  Filter{EventType: StringOrSlice{"TEST"}},
				Actions: []Action{{Type: "log", Config: map[string]string{"message": "hit route-b"}}},
			},
		},
	}

	engine, err := NewEngine(w)
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	session := NewDebugSession("test-step", engine)

	// Set breakpoint on the first route
	session.SetBreakpoint(&Breakpoint{
		ID:      "bp-route-a",
		Type:    BreakpointRoute,
		Name:    "route-a",
		Enabled: true,
	})

	event := map[string]interface{}{
		"type":   "TEST",
		"source": "test-source",
	}

	// Start the session
	ctx := context.Background()
	session.Start(ctx, event)

	// Wait for first pause (should hit breakpoint on route-a)
	var firstStep DebugStep
	select {
	case firstStep = <-session.stepCh:
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for first step")
	}

	if firstStep.Kind != DebugStepRoute {
		t.Errorf("Expected first step kind 'route', got '%s'", firstStep.Kind)
	}
	if firstStep.Name != "route-a" {
		t.Errorf("Expected first step name 'route-a', got '%s'", firstStep.Name)
	}
	if firstStep.StepNumber != 1 {
		t.Errorf("Expected step number 1, got %d", firstStep.StepNumber)
	}

	// Step to next (should hit action for route-a since we're in stepping mode)
	nextStep := session.Step()
	if nextStep == nil {
		t.Fatal("Expected a step after Step(), got nil")
	}
	if nextStep.Kind != DebugStepAction {
		t.Errorf("Expected step kind 'action', got '%s'", nextStep.Kind)
	}
	if nextStep.Name != "log" {
		t.Errorf("Expected step name 'log', got '%s'", nextStep.Name)
	}

	// Step to next (should hit route-b)
	nextStep = session.Step()
	if nextStep == nil {
		t.Fatal("Expected a step after second Step(), got nil")
	}
	if nextStep.Kind != DebugStepRoute {
		t.Errorf("Expected step kind 'route', got '%s'", nextStep.Kind)
	}
	if nextStep.Name != "route-b" {
		t.Errorf("Expected step name 'route-b', got '%s'", nextStep.Name)
	}

	// Step to next (should hit action for route-b)
	nextStep = session.Step()
	if nextStep == nil {
		t.Fatal("Expected a step after third Step(), got nil")
	}
	if nextStep.Kind != DebugStepAction {
		t.Errorf("Expected step kind 'action', got '%s'", nextStep.Kind)
	}

	// Step again - should complete (no more spans)
	nextStep = session.Step()
	if nextStep != nil {
		t.Errorf("Expected nil step after completion, got %+v", nextStep)
	}

	// Verify session completed
	session.mu.Lock()
	state := session.State
	session.mu.Unlock()
	if state != DebugStateComplete {
		t.Errorf("Expected state 'completed', got '%s'", state)
	}
}

func TestDebugSession_Continue(t *testing.T) {
	w := &Workflow{
		Name: "test-debug-continue",
		Routes: []Route{
			{
				Name:    "route-a",
				Filter:  Filter{EventType: StringOrSlice{"TEST"}},
				Actions: []Action{{Type: "log", Config: map[string]string{"message": "hit route-a"}}},
			},
			{
				Name:    "route-b",
				Filter:  Filter{EventType: StringOrSlice{"TEST"}},
				Actions: []Action{{Type: "log", Config: map[string]string{"message": "hit route-b"}}},
			},
		},
	}

	engine, err := NewEngine(w)
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	session := NewDebugSession("test-continue", engine)

	// Set breakpoints on both routes
	session.SetBreakpoint(&Breakpoint{
		ID:      "bp-route-a",
		Type:    BreakpointRoute,
		Name:    "route-a",
		Enabled: true,
	})
	session.SetBreakpoint(&Breakpoint{
		ID:      "bp-route-b",
		Type:    BreakpointRoute,
		Name:    "route-b",
		Enabled: true,
	})

	event := map[string]interface{}{
		"type":   "TEST",
		"source": "test-source",
	}

	ctx := context.Background()
	session.Start(ctx, event)

	// Wait for first pause (route-a breakpoint)
	var firstStep DebugStep
	select {
	case firstStep = <-session.stepCh:
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for first step")
	}

	if firstStep.Name != "route-a" {
		t.Errorf("Expected first pause at 'route-a', got '%s'", firstStep.Name)
	}

	// Continue should run past route-a actions and pause at route-b breakpoint
	nextStep := session.Continue()
	if nextStep == nil {
		t.Fatal("Expected step after Continue(), got nil")
	}
	if nextStep.Name != "route-b" {
		t.Errorf("Expected continue to pause at 'route-b', got '%s'", nextStep.Name)
	}

	// Continue again - should complete (no more breakpoints to hit)
	finalStep := session.Continue()
	if finalStep != nil {
		t.Errorf("Expected nil step after final Continue(), got %+v", finalStep)
	}
}

func TestDebugSession_Close(t *testing.T) {
	w := &Workflow{
		Name: "test-debug-close",
		Routes: []Route{
			{
				Name:    "route-a",
				Filter:  Filter{EventType: StringOrSlice{"TEST"}},
				Actions: []Action{{Type: "log", Config: map[string]string{"message": "hit"}}},
			},
		},
	}

	engine, err := NewEngine(w)
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	session := NewDebugSession("test-close", engine)

	// Set breakpoint so session pauses
	session.SetBreakpoint(&Breakpoint{
		ID:      "bp-route-a",
		Type:    BreakpointRoute,
		Name:    "route-a",
		Enabled: true,
	})

	event := map[string]interface{}{
		"type":   "TEST",
		"source": "test-source",
	}

	ctx := context.Background()
	session.Start(ctx, event)

	// Wait for pause
	select {
	case <-session.stepCh:
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for pause")
	}

	// Close the session
	session.Close()

	// Wait for the session goroutine to finish
	select {
	case <-session.doneCh:
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for session to stop")
	}

	session.mu.Lock()
	state := session.State
	session.mu.Unlock()
	if state != DebugStateStopped {
		t.Errorf("Expected state 'stopped', got '%s'", state)
	}
}

func TestDebugSession_GetVariables(t *testing.T) {
	w := &Workflow{
		Name: "test-debug-vars",
		Routes: []Route{
			{
				Name:    "route-a",
				Filter:  Filter{EventType: StringOrSlice{"TEST"}},
				Actions: []Action{{Type: "log", Config: map[string]string{"message": "hit"}}},
			},
		},
	}

	engine, err := NewEngine(w)
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	session := NewDebugSession("test-vars", engine)

	// No steps yet, should return nil
	vars := session.GetVariables()
	if vars != nil {
		t.Error("Expected nil variables when no steps exist")
	}

	// Set breakpoint and start
	session.SetBreakpoint(&Breakpoint{
		ID:      "bp-route-a",
		Type:    BreakpointRoute,
		Name:    "route-a",
		Enabled: true,
	})

	event := map[string]interface{}{
		"type":   "TEST",
		"source": "test-source",
	}

	ctx := context.Background()
	session.Start(ctx, event)

	// Wait for pause
	select {
	case <-session.stepCh:
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for pause")
	}

	// Should now have variables
	vars = session.GetVariables()
	if vars == nil {
		t.Fatal("Expected non-nil variables after pause")
	}
	if routeName, ok := vars[AttrRouteName].(string); !ok || routeName != "route-a" {
		t.Errorf("Expected route.name='route-a' in variables, got %v", vars[AttrRouteName])
	}

	// Clean up
	session.Close()
	select {
	case <-session.doneCh:
	case <-time.After(2 * time.Second):
	}
}
