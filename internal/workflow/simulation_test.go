//nolint:gosec // Test file - G104 errors intentionally ignored in test setup
package workflow

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestMockAction_Execute(t *testing.T) {
	mock := NewMockAction()

	event := map[string]interface{}{"type": "test"}
	config := map[string]string{"key": "value"}

	err := mock.Execute(event, config)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if mock.CallCount() != 1 {
		t.Errorf("Expected 1 call, got %d", mock.CallCount())
	}

	inv := mock.LastInvocation()
	if inv == nil {
		t.Fatal("Expected invocation, got nil")
	}

	if inv.Config["key"] != "value" {
		t.Errorf("Expected config key=value, got %v", inv.Config)
	}
}

func TestMockAction_WithError(t *testing.T) {
	expectedErr := errors.New("test error")
	mock := NewMockAction().WithError(expectedErr)

	err := mock.Execute(nil, nil)
	if !errors.Is(err, expectedErr) {
		t.Errorf("Expected %v, got %v", expectedErr, err)
	}

	inv := mock.LastInvocation()
	if inv.Error != expectedErr.Error() {
		t.Errorf("Expected error in invocation, got %q", inv.Error)
	}
}

func TestMockAction_WithDelay(t *testing.T) {
	mock := NewMockAction().WithDelay(50 * time.Millisecond)

	start := time.Now()
	mock.Execute(nil, nil)
	elapsed := time.Since(start)

	if elapsed < 50*time.Millisecond {
		t.Errorf("Expected at least 50ms delay, got %v", elapsed)
	}
}

func TestMockAction_FailAfter(t *testing.T) {
	expectedErr := errors.New("fail after 2")
	mock := NewMockAction().FailAfter(2, expectedErr)

	// First two calls succeed
	if err := mock.Execute(nil, nil); err != nil {
		t.Errorf("Call 1: expected success, got %v", err)
	}
	if err := mock.Execute(nil, nil); err != nil {
		t.Errorf("Call 2: expected success, got %v", err)
	}

	// Third call fails
	if err := mock.Execute(nil, nil); !errors.Is(err, expectedErr) {
		t.Errorf("Call 3: expected %v, got %v", expectedErr, err)
	}

	// Fourth call also fails
	if err := mock.Execute(nil, nil); !errors.Is(err, expectedErr) {
		t.Errorf("Call 4: expected %v, got %v", expectedErr, err)
	}
}

func TestMockAction_Reset(t *testing.T) {
	mock := NewMockAction()

	mock.Execute(nil, nil)
	mock.Execute(nil, nil)

	if mock.CallCount() != 2 {
		t.Fatalf("Expected 2 calls before reset")
	}

	mock.Reset()

	if mock.CallCount() != 0 {
		t.Errorf("Expected 0 calls after reset, got %d", mock.CallCount())
	}
}

func TestMockAction_WithResponse(t *testing.T) {
	response := map[string]string{"status": "ok"}
	mock := NewMockAction().WithResponse(response)

	mock.Execute(nil, nil)

	inv := mock.LastInvocation()
	if inv.Response == nil {
		t.Fatal("Expected response, got nil")
	}

	respMap, ok := inv.Response.(map[string]string)
	if !ok {
		t.Fatalf("Expected map[string]string response, got %T", inv.Response)
	}

	if respMap["status"] != "ok" {
		t.Errorf("Expected status=ok, got %v", respMap["status"])
	}
}

func TestSimulationEngine_BasicProcessing(t *testing.T) {
	workflow := &Workflow{
		Name: "test",
		Routes: []Route{
			{
				Name:   "route1",
				Filter: Filter{EventType: StringOrSlice{"patient_admit"}},
				Actions: []Action{
					{Type: "log", Config: map[string]string{"level": "info"}},
					{Type: "webhook", Config: map[string]string{"url": "http://test.com"}},
				},
			},
		},
	}

	sim, err := NewSimulationEngine(workflow)
	if err != nil {
		t.Fatalf("NewSimulationEngine failed: %v", err)
	}

	event := map[string]interface{}{"type": "patient_admit"}
	result := sim.Process(event)

	if result.HasErrors() {
		t.Errorf("Expected no errors, got %v", result.AllErrors())
	}

	// Verify both actions were called
	invs := sim.AllInvocations()
	if len(invs) != 2 {
		t.Errorf("Expected 2 invocations, got %d", len(invs))
	}
}

func TestSimulationEngine_NoMatchingRoutes(t *testing.T) {
	workflow := &Workflow{
		Name: "test",
		Routes: []Route{
			{
				Name:    "route1",
				Filter:  Filter{EventType: StringOrSlice{"patient_admit"}},
				Actions: []Action{{Type: "log"}},
			},
		},
	}

	sim, _ := NewSimulationEngine(workflow)

	// Event that doesn't match
	event := map[string]interface{}{"type": "lab_result"}
	sim.Process(event)

	if len(sim.AllInvocations()) != 0 {
		t.Errorf("Expected 0 invocations for non-matching event, got %d", len(sim.AllInvocations()))
	}
}

func TestSimulationEngine_InvocationsOf(t *testing.T) {
	workflow := &Workflow{
		Name: "test",
		Routes: []Route{
			{
				Name:   "route1",
				Filter: Filter{},
				Actions: []Action{
					{Type: "log"},
					{Type: "webhook"},
					{Type: "log"},
				},
			},
		},
	}

	sim, _ := NewSimulationEngine(workflow)
	sim.Process(map[string]interface{}{"type": "test"})

	logInvs := sim.InvocationsOf("log")
	if len(logInvs) != 2 {
		t.Errorf("Expected 2 log invocations, got %d", len(logInvs))
	}

	webhookInvs := sim.InvocationsOf("webhook")
	if len(webhookInvs) != 1 {
		t.Errorf("Expected 1 webhook invocation, got %d", len(webhookInvs))
	}
}

func TestSimulationEngine_SetMock(t *testing.T) {
	workflow := &Workflow{
		Name: "test",
		Routes: []Route{
			{
				Name:    "route1",
				Filter:  Filter{},
				Actions: []Action{{Type: "webhook"}},
			},
		},
	}

	sim, _ := NewSimulationEngine(workflow)

	// Configure webhook to fail
	webhookErr := errors.New("webhook failed")
	sim.SetMock("webhook", NewMockAction().WithError(webhookErr))

	result := sim.Process(map[string]interface{}{"type": "test"})

	if !result.HasErrors() {
		t.Error("Expected errors from failing webhook")
	}

	// Check invocation recorded error
	invs := sim.InvocationsOf("webhook")
	if len(invs) != 1 {
		t.Fatalf("Expected 1 webhook invocation, got %d", len(invs))
	}

	if invs[0].Error == "" {
		t.Error("Expected error in invocation")
	}
}

func TestSimulationEngine_Reset(t *testing.T) {
	workflow := &Workflow{
		Name:   "test",
		Routes: []Route{{Name: "r", Filter: Filter{}, Actions: []Action{{Type: "log"}}}},
	}

	sim, _ := NewSimulationEngine(workflow)

	sim.Process(map[string]interface{}{"type": "test"})
	sim.Process(map[string]interface{}{"type": "test"})

	if len(sim.AllInvocations()) != 2 {
		t.Fatalf("Expected 2 invocations before reset")
	}

	sim.Reset()

	if len(sim.AllInvocations()) != 0 {
		t.Errorf("Expected 0 invocations after reset, got %d", len(sim.AllInvocations()))
	}
}

func TestAssertions_ActionCalled(t *testing.T) {
	workflow := &Workflow{
		Name:   "test",
		Routes: []Route{{Name: "r", Filter: Filter{}, Actions: []Action{{Type: "log"}}}},
	}

	sim, _ := NewSimulationEngine(workflow)
	sim.Process(map[string]interface{}{"type": "test"})

	a := sim.Assert()

	if err := a.ActionCalled("log"); err != nil {
		t.Errorf("ActionCalled(log) failed: %v", err)
	}

	if err := a.ActionCalled("webhook"); err == nil {
		t.Error("ActionCalled(webhook) should have failed")
	}
}

func TestAssertions_ActionNotCalled(t *testing.T) {
	workflow := &Workflow{
		Name:   "test",
		Routes: []Route{{Name: "r", Filter: Filter{}, Actions: []Action{{Type: "log"}}}},
	}

	sim, _ := NewSimulationEngine(workflow)
	sim.Process(map[string]interface{}{"type": "test"})

	a := sim.Assert()

	if err := a.ActionNotCalled("webhook"); err != nil {
		t.Errorf("ActionNotCalled(webhook) failed: %v", err)
	}

	if err := a.ActionNotCalled("log"); err == nil {
		t.Error("ActionNotCalled(log) should have failed")
	}
}

func TestAssertions_ActionCalledTimes(t *testing.T) {
	workflow := &Workflow{
		Name: "test",
		Routes: []Route{
			{Name: "r", Filter: Filter{}, Actions: []Action{{Type: "log"}, {Type: "log"}}},
		},
	}

	sim, _ := NewSimulationEngine(workflow)
	sim.Process(map[string]interface{}{"type": "test"})

	a := sim.Assert()

	if err := a.ActionCalledTimes("log", 2); err != nil {
		t.Errorf("ActionCalledTimes(log, 2) failed: %v", err)
	}

	if err := a.ActionCalledTimes("log", 3); err == nil {
		t.Error("ActionCalledTimes(log, 3) should have failed")
	}
}

func TestAssertions_TotalActionCalls(t *testing.T) {
	workflow := &Workflow{
		Name: "test",
		Routes: []Route{
			{Name: "r", Filter: Filter{}, Actions: []Action{{Type: "log"}, {Type: "webhook"}}},
		},
	}

	sim, _ := NewSimulationEngine(workflow)
	sim.Process(map[string]interface{}{"type": "test"})

	a := sim.Assert()

	if err := a.TotalActionCalls(2); err != nil {
		t.Errorf("TotalActionCalls(2) failed: %v", err)
	}

	if err := a.TotalActionCalls(5); err == nil {
		t.Error("TotalActionCalls(5) should have failed")
	}
}

func TestAssertions_ActionCalledWithConfig(t *testing.T) {
	workflow := &Workflow{
		Name: "test",
		Routes: []Route{
			{
				Name:   "r",
				Filter: Filter{},
				Actions: []Action{
					{Type: "webhook", Config: map[string]string{"url": "http://test.com", "method": "POST"}},
				},
			},
		},
	}

	sim, _ := NewSimulationEngine(workflow)
	sim.Process(map[string]interface{}{"type": "test"})

	a := sim.Assert()

	// Partial match should work
	if err := a.ActionCalledWithConfig("webhook", map[string]string{"url": "http://test.com"}); err != nil {
		t.Errorf("ActionCalledWithConfig failed: %v", err)
	}

	// Non-matching should fail
	if err := a.ActionCalledWithConfig("webhook", map[string]string{"url": "http://other.com"}); err == nil {
		t.Error("ActionCalledWithConfig should have failed for non-matching config")
	}
}

func TestAssertions_NoErrors(t *testing.T) {
	workflow := &Workflow{
		Name:   "test",
		Routes: []Route{{Name: "r", Filter: Filter{}, Actions: []Action{{Type: "log"}}}},
	}

	sim, _ := NewSimulationEngine(workflow)
	sim.Process(map[string]interface{}{"type": "test"})

	a := sim.Assert()

	if err := a.NoErrors(); err != nil {
		t.Errorf("NoErrors() failed: %v", err)
	}

	// Configure mock to fail
	sim.Reset()
	sim.SetMock("log", NewMockAction().WithError(errors.New("fail")))
	sim.Process(map[string]interface{}{"type": "test"})

	if err := a.NoErrors(); err == nil {
		t.Error("NoErrors() should have failed when action errors occurred")
	}
}

func TestSimulationEngine_Report(t *testing.T) {
	workflow := &Workflow{
		Name: "test",
		Routes: []Route{
			{Name: "r", Filter: Filter{}, Actions: []Action{{Type: "log"}, {Type: "webhook"}}},
		},
	}

	sim, _ := NewSimulationEngine(workflow)
	sim.SetMock("webhook", NewMockAction().WithError(errors.New("test error")))

	sim.Process(map[string]interface{}{"type": "test"})
	sim.Process(map[string]interface{}{"type": "test"})

	report := sim.Report()

	if report.TotalActions != 4 {
		t.Errorf("Expected 4 total actions, got %d", report.TotalActions)
	}

	if report.ActionsByType["log"] != 2 {
		t.Errorf("Expected 2 log actions, got %d", report.ActionsByType["log"])
	}

	if len(report.Errors) != 2 {
		t.Errorf("Expected 2 errors, got %d", len(report.Errors))
	}
}

func TestSimulationEngine_ReportJSON(t *testing.T) {
	workflow := &Workflow{
		Name:   "test",
		Routes: []Route{{Name: "r", Filter: Filter{}, Actions: []Action{{Type: "log"}}}},
	}

	sim, _ := NewSimulationEngine(workflow)
	sim.Process(map[string]interface{}{"type": "test"})

	jsonStr, err := sim.ReportJSON()
	if err != nil {
		t.Fatalf("ReportJSON failed: %v", err)
	}

	if jsonStr == "" {
		t.Error("Expected non-empty JSON string")
	}

	// Basic sanity check
	if len(jsonStr) < 10 {
		t.Errorf("JSON seems too short: %s", jsonStr)
	}
}

func TestScenarioRunner_Run(t *testing.T) {
	workflow := &Workflow{
		Name: "test",
		Routes: []Route{
			{
				Name:    "patient_route",
				Filter:  Filter{EventType: StringOrSlice{"patient_admit"}},
				Actions: []Action{{Type: "webhook"}},
			},
			{
				Name:    "lab_route",
				Filter:  Filter{EventType: StringOrSlice{"lab_result"}},
				Actions: []Action{{Type: "fhir"}},
			},
		},
	}

	sim, _ := NewSimulationEngine(workflow)
	runner := NewScenarioRunner(sim)

	scenario := &Scenario{
		Name: "test patient routing",
		Events: []interface{}{
			map[string]interface{}{"type": "patient_admit"},
			map[string]interface{}{"type": "patient_admit"},
		},
		Assertions: func(a *Assertions) error {
			if err := a.ActionCalledTimes("webhook", 2); err != nil {
				return err
			}
			if err := a.ActionNotCalled("fhir"); err != nil {
				return err
			}
			return nil
		},
	}

	err := runner.Run(scenario)
	if err != nil {
		t.Errorf("Scenario failed: %v", err)
	}
}

func TestScenarioRunner_RunWithSetup(t *testing.T) {
	workflow := &Workflow{
		Name:   "test",
		Routes: []Route{{Name: "r", Filter: Filter{}, Actions: []Action{{Type: "webhook"}}}},
	}

	sim, _ := NewSimulationEngine(workflow)
	runner := NewScenarioRunner(sim)

	scenario := &Scenario{
		Name: "test error handling",
		Setup: func(sim *SimulationEngine) {
			sim.SetMock("webhook", NewMockAction().WithError(errors.New("fail")))
		},
		Events: []interface{}{
			map[string]interface{}{"type": "test"},
		},
		Assertions: func(a *Assertions) error {
			// Should have been called even though it failed
			return a.ActionCalled("webhook")
		},
	}

	err := runner.Run(scenario)
	if err != nil {
		t.Errorf("Scenario failed: %v", err)
	}
}

func TestScenarioRunner_RunAll(t *testing.T) {
	workflow := &Workflow{
		Name:   "test",
		Routes: []Route{{Name: "r", Filter: Filter{}, Actions: []Action{{Type: "log"}}}},
	}

	sim, _ := NewSimulationEngine(workflow)
	runner := NewScenarioRunner(sim)

	scenarios := []*Scenario{
		{
			Name:   "passing",
			Events: []interface{}{map[string]interface{}{"type": "test"}},
			Assertions: func(a *Assertions) error {
				return a.ActionCalled("log")
			},
		},
		{
			Name:   "failing",
			Events: []interface{}{map[string]interface{}{"type": "test"}},
			Assertions: func(a *Assertions) error {
				return a.ActionCalled("nonexistent")
			},
		},
	}

	results := runner.RunAll(scenarios)

	if results["passing"] != nil {
		t.Errorf("'passing' scenario should have passed, got: %v", results["passing"])
	}

	if results["failing"] == nil {
		t.Error("'failing' scenario should have failed")
	}
}

func TestMockAction_ContextCancellation(t *testing.T) {
	mock := NewMockAction().WithDelay(1 * time.Second)

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel immediately
	cancel()

	start := time.Now()
	err := mock.ExecuteWithContext(ctx, nil, nil)
	elapsed := time.Since(start)

	if !errors.Is(err, context.Canceled) {
		t.Errorf("Expected context.Canceled, got %v", err)
	}

	// Should have returned quickly due to cancellation
	if elapsed > 100*time.Millisecond {
		t.Errorf("Expected quick return on cancel, took %v", elapsed)
	}
}

func TestSimulationEngine_MultipleRoutes(t *testing.T) {
	workflow := &Workflow{
		Name: "test",
		Routes: []Route{
			{
				Name:    "route1",
				Filter:  Filter{}, // Matches all
				Actions: []Action{{Type: "log"}},
			},
			{
				Name:    "route2",
				Filter:  Filter{}, // Also matches all
				Actions: []Action{{Type: "webhook"}},
			},
		},
	}

	sim, _ := NewSimulationEngine(workflow)
	sim.Process(map[string]interface{}{"type": "test"})

	// Both routes should have fired
	if err := sim.Assert().ActionCalled("log"); err != nil {
		t.Errorf("Expected log action: %v", err)
	}
	if err := sim.Assert().ActionCalled("webhook"); err != nil {
		t.Errorf("Expected webhook action: %v", err)
	}
}
