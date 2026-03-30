package workflow

import (
	"context"
	"sync"
	"time"
)

// BreakpointType identifies what kind of span to break on.
type BreakpointType string

const (
	BreakpointRoute     BreakpointType = "route"
	BreakpointAction    BreakpointType = "action"
	BreakpointTransform BreakpointType = "transform"
)

// Breakpoint defines a point where execution should pause.
type Breakpoint struct {
	ID      string
	Type    BreakpointType
	Name    string // Route name, action type, or transform type to match
	Enabled bool
}

// DebugStepKind describes the type of step the debugger paused at.
type DebugStepKind string

const (
	DebugStepRoute     DebugStepKind = "route"
	DebugStepAction    DebugStepKind = "action"
	DebugStepTransform DebugStepKind = "transform"
)

// DebugStep represents a paused execution point with variable snapshot.
type DebugStep struct {
	StepNumber int
	Kind       DebugStepKind
	Name       string
	Variables  map[string]interface{}
	Timestamp  time.Time
	SpanName   string
}

// DebugCommand represents a control command for the debug session.
type DebugCommand int

const (
	DebugCmdStep     DebugCommand = iota // Advance one step
	DebugCmdContinue                     // Run until next breakpoint or end
	DebugCmdStop                         // Stop the session
)

// DebugSessionState represents the current state of a debug session.
type DebugSessionState string

const (
	DebugStateIdle     DebugSessionState = "idle"
	DebugStateRunning  DebugSessionState = "running"
	DebugStatePaused   DebugSessionState = "paused"
	DebugStateComplete DebugSessionState = "completed"
	DebugStateStopped  DebugSessionState = "stopped"
)

// DebugSession provides step-through debugging of workflow execution.
type DebugSession struct {
	ID          string
	WorkflowID  string
	State       DebugSessionState
	CreatedAt   time.Time
	Engine      *Engine
	Breakpoints map[string]*Breakpoint
	Steps       []DebugStep

	mu        sync.Mutex
	controlCh chan DebugCommand
	stepCh    chan DebugStep
	doneCh    chan struct{}
	tracer    *DebugTracer
	stepSubs  map[chan DebugStep]struct{}
}

// NewDebugSession creates a new debug session wrapping an engine.
func NewDebugSession(id string, engine *Engine) *DebugSession {
	ds := &DebugSession{
		ID:          id,
		Engine:      engine,
		State:       DebugStateIdle,
		CreatedAt:   time.Now(),
		Breakpoints: make(map[string]*Breakpoint),
		Steps:       make([]DebugStep, 0),
		controlCh:   make(chan DebugCommand, 1),
		stepCh:      make(chan DebugStep, 10),
		doneCh:      make(chan struct{}),
		stepSubs:    make(map[chan DebugStep]struct{}),
	}

	// Start in stepping mode so a newly created session pauses on the first
	// executable span even when the UI has not pre-seeded breakpoints yet.
	ds.tracer = &DebugTracer{session: ds, stepping: true}
	return ds
}

// SetBreakpoint adds or updates a breakpoint.
func (ds *DebugSession) SetBreakpoint(bp *Breakpoint) {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	ds.Breakpoints[bp.ID] = bp
}

// RemoveBreakpoint removes a breakpoint by ID.
func (ds *DebugSession) RemoveBreakpoint(id string) bool {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	_, exists := ds.Breakpoints[id]
	delete(ds.Breakpoints, id)
	return exists
}

// Start begins processing an event with debug tracing.
// This runs in a goroutine and pauses at breakpoints.
func (ds *DebugSession) Start(ctx context.Context, event interface{}) {
	ds.mu.Lock()
	ds.State = DebugStateRunning
	// Install debug tracer on the engine
	originalTracer := ds.Engine.GetTracer()
	ds.Engine.SetTracer(ds.tracer)
	ds.mu.Unlock()

	go func() {
		defer func() {
			ds.mu.Lock()
			ds.Engine.SetTracer(originalTracer) // Restore original tracer
			if ds.State == DebugStateRunning || ds.State == DebugStatePaused {
				ds.State = DebugStateComplete
			}
			ds.closeSubscribersLocked()
			ds.mu.Unlock()
			close(ds.doneCh)
		}()

		ds.Engine.ProcessWithContext(ctx, event)
	}()
}

// Step advances execution by one step. Returns the next step or nil if done.
func (ds *DebugSession) Step() *DebugStep {
	ds.mu.Lock()
	if ds.State != DebugStatePaused {
		ds.mu.Unlock()
		return nil
	}
	ds.State = DebugStateRunning
	ds.mu.Unlock()

	ds.drainPendingSteps()

	// Send step command
	select {
	case ds.controlCh <- DebugCmdStep:
	default:
	}

	// Wait for next pause or completion
	select {
	case step := <-ds.stepCh:
		return &step
	case <-ds.doneCh:
		return nil
	}
}

// Continue resumes execution until the next breakpoint or end.
func (ds *DebugSession) Continue() *DebugStep {
	ds.mu.Lock()
	if ds.State != DebugStatePaused {
		ds.mu.Unlock()
		return nil
	}
	ds.State = DebugStateRunning
	ds.mu.Unlock()

	ds.drainPendingSteps()

	select {
	case ds.controlCh <- DebugCmdContinue:
	default:
	}

	select {
	case step := <-ds.stepCh:
		return &step
	case <-ds.doneCh:
		return nil
	}
}

// GetVariables returns the current variable snapshot.
func (ds *DebugSession) GetVariables() map[string]interface{} {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	if len(ds.Steps) == 0 {
		return nil
	}
	return ds.Steps[len(ds.Steps)-1].Variables
}

// Close terminates the debug session.
func (ds *DebugSession) Close() {
	ds.mu.Lock()
	ds.State = DebugStateStopped
	ds.closeSubscribersLocked()
	ds.mu.Unlock()

	select {
	case ds.controlCh <- DebugCmdStop:
	default:
	}
}

// SubscribeSteps registers a non-blocking stream of paused steps for observers.
func (ds *DebugSession) SubscribeSteps(buffer int) (<-chan DebugStep, func()) {
	if buffer <= 0 {
		buffer = 1
	}

	ch := make(chan DebugStep, buffer)
	ds.mu.Lock()
	ds.stepSubs[ch] = struct{}{}
	ds.mu.Unlock()

	cancel := func() {
		ds.mu.Lock()
		if _, ok := ds.stepSubs[ch]; ok {
			delete(ds.stepSubs, ch)
			close(ch)
		}
		ds.mu.Unlock()
	}

	return ch, cancel
}

func (ds *DebugSession) broadcastStep(step DebugStep) {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	for ch := range ds.stepSubs {
		select {
		case ch <- step:
		default:
		}
	}
}

func (ds *DebugSession) closeSubscribersLocked() {
	for ch := range ds.stepSubs {
		close(ch)
		delete(ds.stepSubs, ch)
	}
}

func (ds *DebugSession) drainPendingSteps() {
	for {
		select {
		case <-ds.stepCh:
		default:
			return
		}
	}
}

// matchesBreakpoint checks if the current span matches any enabled breakpoint.
func (ds *DebugSession) matchesBreakpoint(spanName string, attrs map[string]interface{}) bool {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	for _, bp := range ds.Breakpoints {
		if !bp.Enabled {
			continue
		}

		switch bp.Type {
		case BreakpointRoute:
			if spanName == SpanNameRoute {
				if routeName, ok := attrs[AttrRouteName].(string); ok && routeName == bp.Name {
					return true
				}
			}
		case BreakpointAction:
			if spanName == SpanNameAction {
				if actionType, ok := attrs[AttrActionType].(string); ok && actionType == bp.Name {
					return true
				}
			}
		case BreakpointTransform:
			if spanName == SpanNameTransform {
				if transformType, ok := attrs[AttrTransformType].(string); ok && transformType == bp.Name {
					return true
				}
			}
		}
	}
	return false
}

// DebugTracer implements the Tracer interface for debug sessions.
type DebugTracer struct {
	session   *DebugSession
	stepping  bool // If true, pause at every span (not just breakpoints)
	stepCount int
}

// StartSpan implements Tracer. It pauses at breakpoints or on step commands.
func (dt *DebugTracer) StartSpan(ctx context.Context, name string, opts ...SpanOption) (context.Context, Span) {
	dt.session.mu.Lock()
	stopped := dt.session.State == DebugStateStopped
	dt.session.mu.Unlock()
	if stopped {
		cancelCtx, cancel := context.WithCancel(ctx)
		cancel()
		return cancelCtx, &noOpSpan{}
	}

	cfg := &spanConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	// Extract attributes for breakpoint matching
	attrs := make(map[string]interface{})
	for _, attr := range cfg.attributes {
		attrs[attr.Key] = attr.Value
	}

	// Determine if we should pause
	shouldPause := false
	isDebugSpan := name == SpanNameRoute || name == SpanNameAction || name == SpanNameTransform

	if dt.stepping && isDebugSpan {
		// In stepping mode, pause at every route/action/transform span
		shouldPause = true
	}

	// Check breakpoints
	if !shouldPause && dt.session.matchesBreakpoint(name, attrs) {
		shouldPause = true
	}

	if shouldPause && isDebugSpan {
		dt.stepCount++
		step := DebugStep{
			StepNumber: dt.stepCount,
			Kind:       spanNameToDebugKind(name),
			Name:       getNameFromAttrs(name, attrs),
			Variables:  attrs,
			Timestamp:  time.Now(),
			SpanName:   name,
		}

		dt.session.mu.Lock()
		dt.session.Steps = append(dt.session.Steps, step)
		dt.session.State = DebugStatePaused
		dt.session.mu.Unlock()

		// Send step to subscriber
		select {
		case dt.session.stepCh <- step:
		default:
		}
		dt.session.broadcastStep(step)

		// Wait for control command
		cmd := <-dt.session.controlCh
		switch cmd {
		case DebugCmdStep:
			dt.stepping = true
		case DebugCmdContinue:
			dt.stepping = false
		case DebugCmdStop:
			dt.session.mu.Lock()
			dt.session.State = DebugStateStopped
			dt.session.mu.Unlock()
			// Return context that will be cancelled
			cancelCtx, cancel := context.WithCancel(ctx)
			cancel()
			return cancelCtx, &noOpSpan{}
		}
	}

	return ctx, &debugSpan{tracer: dt, name: name, attrs: attrs}
}

// debugSpan is a span that records attributes for the debug session.
type debugSpan struct {
	tracer *DebugTracer
	name   string
	attrs  map[string]interface{}
}

func (s *debugSpan) End()                                         {}
func (s *debugSpan) SetAttribute(key string, value interface{})   { s.attrs[key] = value }
func (s *debugSpan) SetStatus(code SpanStatus, message string)    {}
func (s *debugSpan) RecordError(err error)                        {}
func (s *debugSpan) AddEvent(name string, attrs ...SpanAttribute) {}

func spanNameToDebugKind(name string) DebugStepKind {
	switch name {
	case SpanNameRoute:
		return DebugStepRoute
	case SpanNameAction:
		return DebugStepAction
	case SpanNameTransform:
		return DebugStepTransform
	default:
		return DebugStepRoute
	}
}

func getNameFromAttrs(spanName string, attrs map[string]interface{}) string {
	switch spanName {
	case SpanNameRoute:
		if v, ok := attrs[AttrRouteName].(string); ok {
			return v
		}
	case SpanNameAction:
		if v, ok := attrs[AttrActionType].(string); ok {
			return v
		}
	case SpanNameTransform:
		if v, ok := attrs[AttrTransformType].(string); ok {
			return v
		}
	}
	return spanName
}
