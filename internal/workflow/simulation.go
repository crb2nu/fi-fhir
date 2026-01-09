package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// ActionInvocation records a single action call during simulation.
type ActionInvocation struct {
	// ActionType is the type of action invoked
	ActionType string `json:"action_type"`

	// RouteName is which route triggered the action
	RouteName string `json:"route_name"`

	// Event is the event data passed to the action
	Event interface{} `json:"event"`

	// Config is the action configuration
	Config map[string]string `json:"config"`

	// InvokedAt is when the action was called
	InvokedAt time.Time `json:"invoked_at"`

	// Duration is how long the action took (simulated)
	Duration time.Duration `json:"duration_ns"`

	// Error is any error returned by the mock
	Error string `json:"error,omitempty"`

	// Response is any mock response returned
	Response interface{} `json:"response,omitempty"`
}

// MockActionBehavior configures how a mock action behaves.
type MockActionBehavior struct {
	// Error to return (nil for success)
	Error error

	// Delay before returning (simulates latency)
	Delay time.Duration

	// Response data (for inspection in tests)
	Response interface{}

	// CallCount tracks how many times to use this behavior (0 = unlimited)
	CallCount int

	// Then is the next behavior after CallCount exhausted
	Then *MockActionBehavior
}

// MockAction is a test double for action handlers.
type MockAction struct {
	mu          sync.Mutex
	invocations []*ActionInvocation
	behavior    *MockActionBehavior
	callIndex   int
}

// NewMockAction creates a mock action with default success behavior.
func NewMockAction() *MockAction {
	return &MockAction{
		invocations: make([]*ActionInvocation, 0),
	}
}

// WithBehavior sets the mock behavior.
func (m *MockAction) WithBehavior(behavior *MockActionBehavior) *MockAction {
	m.behavior = behavior
	return m
}

// WithError configures the mock to always return an error.
func (m *MockAction) WithError(err error) *MockAction {
	m.behavior = &MockActionBehavior{Error: err}
	return m
}

// WithDelay configures the mock to delay before returning.
func (m *MockAction) WithDelay(delay time.Duration) *MockAction {
	if m.behavior == nil {
		m.behavior = &MockActionBehavior{}
	}
	m.behavior.Delay = delay
	return m
}

// WithResponse configures the mock to return a response.
func (m *MockAction) WithResponse(response interface{}) *MockAction {
	if m.behavior == nil {
		m.behavior = &MockActionBehavior{}
	}
	m.behavior.Response = response
	return m
}

// FailAfter configures the mock to fail after N successful calls.
func (m *MockAction) FailAfter(n int, err error) *MockAction {
	m.behavior = &MockActionBehavior{
		CallCount: n,
		Then: &MockActionBehavior{
			Error: err,
		},
	}
	return m
}

// Execute implements ActionHandler for the mock.
func (m *MockAction) Execute(event interface{}, config map[string]string) error {
	return m.ExecuteWithContext(context.Background(), event, config)
}

// ExecuteWithContext implements ContextActionHandler for the mock.
func (m *MockAction) ExecuteWithContext(ctx context.Context, event interface{}, config map[string]string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	start := time.Now()

	// Get current behavior
	behavior := m.getCurrentBehavior()
	m.callIndex++

	// Simulate delay
	if behavior != nil && behavior.Delay > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(behavior.Delay):
		}
	}

	// Record invocation
	inv := &ActionInvocation{
		ActionType: "mock",
		Event:      event,
		Config:     config,
		InvokedAt:  start,
		Duration:   time.Since(start),
	}

	if behavior != nil && behavior.Response != nil {
		inv.Response = behavior.Response
	}

	var err error
	if behavior != nil && behavior.Error != nil {
		err = behavior.Error
		inv.Error = err.Error()
	}

	m.invocations = append(m.invocations, inv)
	return err
}

// getCurrentBehavior returns the current behavior based on call count.
func (m *MockAction) getCurrentBehavior() *MockActionBehavior {
	if m.behavior == nil {
		return nil
	}

	behavior := m.behavior
	index := m.callIndex

	// Walk through behavior chain based on call count
	for behavior != nil {
		if behavior.CallCount == 0 || index < behavior.CallCount {
			return behavior
		}
		index -= behavior.CallCount
		behavior = behavior.Then
	}

	return m.behavior
}

// Invocations returns all recorded invocations.
func (m *MockAction) Invocations() []*ActionInvocation {
	m.mu.Lock()
	defer m.mu.Unlock()

	result := make([]*ActionInvocation, len(m.invocations))
	copy(result, m.invocations)
	return result
}

// CallCount returns how many times the mock was called.
func (m *MockAction) CallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.invocations)
}

// Reset clears all recorded invocations.
func (m *MockAction) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.invocations = make([]*ActionInvocation, 0)
	m.callIndex = 0
}

// LastInvocation returns the most recent invocation, or nil.
func (m *MockAction) LastInvocation() *ActionInvocation {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.invocations) == 0 {
		return nil
	}
	return m.invocations[len(m.invocations)-1]
}

// SimulationEngine is a workflow engine configured for testing.
type SimulationEngine struct {
	*Engine
	mocks       map[string]*MockAction
	invocations []*ActionInvocation
	mu          sync.Mutex
}

// NewSimulationEngine creates an engine with all actions mocked.
func NewSimulationEngine(workflow *Workflow) (*SimulationEngine, error) {
	engine, err := NewEngine(workflow)
	if err != nil {
		return nil, err
	}

	sim := &SimulationEngine{
		Engine:      engine,
		mocks:       make(map[string]*MockAction),
		invocations: make([]*ActionInvocation, 0),
	}

	// Replace all built-in actions with mocks
	for _, actionType := range []string{"log", "webhook", "fhir", "database", "queue"} {
		mock := NewMockAction()
		sim.mocks[actionType] = mock
		sim.Engine.RegisterAction(actionType, sim.createTrackedHandler(actionType, mock))
	}

	return sim, nil
}

// createTrackedHandler wraps a mock to track invocations with route context.
func (s *SimulationEngine) createTrackedHandler(actionType string, mock *MockAction) ActionHandler {
	return ContextActionHandlerFunc(func(ctx context.Context, event interface{}, config map[string]string) error {
		start := time.Now()
		err := mock.ExecuteWithContext(ctx, event, config)

		s.mu.Lock()
		// Update the last invocation with action type and extract route from context
		// Note: We track separately since mock doesn't know the route
		inv := &ActionInvocation{
			ActionType: actionType,
			Event:      event,
			Config:     config,
			InvokedAt:  start,
			Duration:   time.Since(start),
		}
		if err != nil {
			inv.Error = err.Error()
		}
		s.invocations = append(s.invocations, inv)
		s.mu.Unlock()

		return err
	})
}

// GetMock returns the mock for a specific action type.
func (s *SimulationEngine) GetMock(actionType string) *MockAction {
	return s.mocks[actionType]
}

// SetMock sets a custom mock for an action type.
func (s *SimulationEngine) SetMock(actionType string, mock *MockAction) {
	s.mocks[actionType] = mock
	s.Engine.RegisterAction(actionType, s.createTrackedHandler(actionType, mock))
}

// AllInvocations returns all action invocations across all mocks.
func (s *SimulationEngine) AllInvocations() []*ActionInvocation {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := make([]*ActionInvocation, len(s.invocations))
	copy(result, s.invocations)
	return result
}

// InvocationsOf returns invocations for a specific action type.
func (s *SimulationEngine) InvocationsOf(actionType string) []*ActionInvocation {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := make([]*ActionInvocation, 0)
	for _, inv := range s.invocations {
		if inv.ActionType == actionType {
			result = append(result, inv)
		}
	}
	return result
}

// Reset clears all recorded invocations.
func (s *SimulationEngine) Reset() {
	s.mu.Lock()
	s.invocations = make([]*ActionInvocation, 0)
	s.mu.Unlock()

	for _, mock := range s.mocks {
		mock.Reset()
	}
}

// Assertions provides test assertion helpers.
type Assertions struct {
	sim *SimulationEngine
}

// Assert returns an Assertions helper for the simulation engine.
func (s *SimulationEngine) Assert() *Assertions {
	return &Assertions{sim: s}
}

// ActionCalled asserts an action was called at least once.
func (a *Assertions) ActionCalled(actionType string) error {
	invs := a.sim.InvocationsOf(actionType)
	if len(invs) == 0 {
		return fmt.Errorf("expected action %q to be called, but was not", actionType)
	}
	return nil
}

// ActionNotCalled asserts an action was never called.
func (a *Assertions) ActionNotCalled(actionType string) error {
	invs := a.sim.InvocationsOf(actionType)
	if len(invs) > 0 {
		return fmt.Errorf("expected action %q to not be called, but was called %d times", actionType, len(invs))
	}
	return nil
}

// ActionCalledTimes asserts an action was called exactly N times.
func (a *Assertions) ActionCalledTimes(actionType string, expected int) error {
	invs := a.sim.InvocationsOf(actionType)
	if len(invs) != expected {
		return fmt.Errorf("expected action %q to be called %d times, but was called %d times", actionType, expected, len(invs))
	}
	return nil
}

// TotalActionCalls asserts the total number of action calls.
func (a *Assertions) TotalActionCalls(expected int) error {
	invs := a.sim.AllInvocations()
	if len(invs) != expected {
		return fmt.Errorf("expected %d total action calls, but got %d", expected, len(invs))
	}
	return nil
}

// ActionCalledWithConfig asserts an action was called with specific config values.
func (a *Assertions) ActionCalledWithConfig(actionType string, expectedConfig map[string]string) error {
	invs := a.sim.InvocationsOf(actionType)
	for _, inv := range invs {
		if matchesConfig(inv.Config, expectedConfig) {
			return nil
		}
	}
	return fmt.Errorf("expected action %q to be called with config %v, but no matching call found", actionType, expectedConfig)
}

// matchesConfig checks if actual config contains all expected values.
func matchesConfig(actual, expected map[string]string) bool {
	for k, v := range expected {
		if actual[k] != v {
			return false
		}
	}
	return true
}

// NoErrors asserts no action errors occurred.
func (a *Assertions) NoErrors() error {
	invs := a.sim.AllInvocations()
	for _, inv := range invs {
		if inv.Error != "" {
			return fmt.Errorf("expected no errors, but action %q failed with: %s", inv.ActionType, inv.Error)
		}
	}
	return nil
}

// SimulationReport generates a detailed report of the simulation.
type SimulationReport struct {
	// TotalActions is total action invocations
	TotalActions int `json:"total_actions"`

	// ActionsByType counts invocations per action type
	ActionsByType map[string]int `json:"actions_by_type"`

	// Errors lists all errors that occurred
	Errors []string `json:"errors,omitempty"`

	// Invocations is the detailed invocation list
	Invocations []*ActionInvocation `json:"invocations"`
}

// Report generates a simulation report.
func (s *SimulationEngine) Report() *SimulationReport {
	invs := s.AllInvocations()

	report := &SimulationReport{
		TotalActions:  len(invs),
		ActionsByType: make(map[string]int),
		Errors:        make([]string, 0),
		Invocations:   invs,
	}

	for _, inv := range invs {
		report.ActionsByType[inv.ActionType]++
		if inv.Error != "" {
			report.Errors = append(report.Errors, fmt.Sprintf("%s: %s", inv.ActionType, inv.Error))
		}
	}

	return report
}

// ReportJSON returns the report as formatted JSON.
func (s *SimulationEngine) ReportJSON() (string, error) {
	report := s.Report()
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ScenarioRunner runs test scenarios against the simulation engine.
type ScenarioRunner struct {
	sim *SimulationEngine
}

// NewScenarioRunner creates a scenario runner for the simulation engine.
func NewScenarioRunner(sim *SimulationEngine) *ScenarioRunner {
	return &ScenarioRunner{sim: sim}
}

// Scenario represents a test scenario.
type Scenario struct {
	// Name describes the scenario
	Name string

	// Setup configures mocks before running
	Setup func(sim *SimulationEngine)

	// Events to process
	Events []interface{}

	// Assertions to verify after processing
	Assertions func(a *Assertions) error
}

// Run executes a scenario and returns any assertion errors.
func (r *ScenarioRunner) Run(scenario *Scenario) error {
	// Reset state
	r.sim.Reset()

	// Run setup
	if scenario.Setup != nil {
		scenario.Setup(r.sim)
	}

	// Process events
	for _, event := range scenario.Events {
		r.sim.Process(event)
	}

	// Run assertions
	if scenario.Assertions != nil {
		return scenario.Assertions(r.sim.Assert())
	}

	return nil
}

// RunAll executes multiple scenarios and returns all errors.
func (r *ScenarioRunner) RunAll(scenarios []*Scenario) map[string]error {
	results := make(map[string]error)
	for _, scenario := range scenarios {
		results[scenario.Name] = r.Run(scenario)
	}
	return results
}
