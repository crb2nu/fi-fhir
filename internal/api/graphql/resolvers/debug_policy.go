package resolvers

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/graphql/model"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/observability"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/workflow"
)

// The interactive workflow debugger runs caller-supplied workflow YAML, so what
// it may execute and how many sessions it may hold are deployment-owned
// (docs/operations/SECURITY.md, SEC-2026-09-27-1).

// Environment keys serve reads to build WorkflowDebugPolicy. The action list
// key is workflow.EnvDebugActions.
const (
	EnvWorkflowDebugMaxSessions = "FI_FHIR_WORKFLOW_DEBUG_MAX_SESSIONS"
	EnvWorkflowDebugSessionTTL  = "FI_FHIR_WORKFLOW_DEBUG_SESSION_TTL"
)

// Debugger bounds applied when the deployment does not set them.
const (
	DefaultWorkflowDebugMaxSessions = 8
	DefaultWorkflowDebugSessionTTL  = 15 * time.Minute
)

// ErrWorkflowDebugCapacity is returned when the process already holds the
// maximum number of debug sessions. It names no other session or principal.
var ErrWorkflowDebugCapacity = errors.New("workflow debugger is at capacity; end a debug session or retry later")

// WorkflowDebugPolicy is what the debugger may do on this deployment.
//
// ExecutableActions are the action types that run for real; every other action
// is a recording no-op stub and `exec` is always stubbed. MaxSessions bounds
// the sessions this process holds (0 disables the debugger). SessionTTL is the
// absolute lifetime of a session from its creation; an expired session is
// stopped and forgotten on the next debugger call.
type WorkflowDebugPolicy struct {
	ExecutableActions []string
	MaxSessions       int
	SessionTTL        time.Duration
}

// DefaultWorkflowDebugPolicy stubs every action and applies the default bounds.
func DefaultWorkflowDebugPolicy() WorkflowDebugPolicy {
	return WorkflowDebugPolicy{
		MaxSessions: DefaultWorkflowDebugMaxSessions,
		SessionTTL:  DefaultWorkflowDebugSessionTTL,
	}
}

// WorkflowDebugObserver receives the debugger's session metrics.
// *observability.Metrics implements it.
type WorkflowDebugObserver interface {
	SetWorkflowDebugSessions(active int)
	RecordWorkflowDebugSession(outcome observability.Outcome)
}

// WithWorkflowDebugPolicy sets the deployment-owned debugger policy.
func WithWorkflowDebugPolicy(policy WorkflowDebugPolicy) ResolverOption {
	return func(r *Resolver) {
		if policy.SessionTTL <= 0 {
			policy.SessionTTL = DefaultWorkflowDebugSessionTTL
		}
		if policy.MaxSessions < 0 {
			policy.MaxSessions = 0
		}
		policy.ExecutableActions = append([]string(nil), policy.ExecutableActions...)
		r.debugPolicy = policy
	}
}

// WithWorkflowDebugObserver publishes debugger session metrics.
func WithWorkflowDebugObserver(observer WorkflowDebugObserver) ResolverOption {
	return func(r *Resolver) {
		r.debugObserver = observer
	}
}

// startDebugSession is the body of the startDebugSession mutation.
func (r *Resolver) startDebugSession(ctx context.Context, input model.StartDebugSessionInput) (*model.DebugSessionModel, error) {
	parsed, err := workflow.ParseWorkflow([]byte(input.WorkflowYaml))
	if err != nil {
		return nil, fmt.Errorf("parse workflow yaml: %w", err)
	}

	engine, err := workflow.NewDebugEngine(parsed, r.debugPolicy.ExecutableActions)
	if err != nil {
		return nil, fmt.Errorf("create workflow engine: %w", err)
	}

	sessionID := uuid.New().String()
	session := workflow.NewDebugSession(sessionID, engine)
	session.WorkflowID = parsed.Name

	r.debugSessionsMu.Lock()
	r.reapExpiredDebugSessionsLocked()
	if len(r.debugSessions) >= r.debugPolicy.MaxSessions {
		active := len(r.debugSessions)
		r.debugSessionsMu.Unlock()
		r.observeDebugSessions(active, observability.OutcomeRejected)
		return nil, ErrWorkflowDebugCapacity
	}
	r.debugSessions[sessionID] = session
	active := len(r.debugSessions)
	r.debugSessionsMu.Unlock()
	r.observeDebugSessions(active, observability.OutcomeAccepted)

	// Start processing the event
	session.Start(ctx, input.Event)

	return toDebugSessionModel(session), nil
}

// lookupDebugSession returns a live session. An expired session is stopped,
// forgotten, and reported as not found.
func (r *Resolver) lookupDebugSession(sessionID string) (*workflow.DebugSession, bool) {
	r.debugSessionsMu.Lock()
	r.reapExpiredDebugSessionsLocked()
	session, ok := r.debugSessions[sessionID]
	r.debugSessionsMu.Unlock()
	return session, ok
}

// endDebugSession removes and stops a session.
func (r *Resolver) endDebugSession(sessionID string) bool {
	r.debugSessionsMu.Lock()
	r.reapExpiredDebugSessionsLocked()
	session, ok := r.debugSessions[sessionID]
	if ok {
		delete(r.debugSessions, sessionID)
	}
	active := len(r.debugSessions)
	r.debugSessionsMu.Unlock()
	if !ok {
		return false
	}
	session.Close()
	r.observeDebugSessions(active, observability.OutcomeProcessed)
	return true
}

// reapExpiredDebugSessionsLocked stops and forgets every session older than
// the TTL. The caller holds debugSessionsMu for writing.
func (r *Resolver) reapExpiredDebugSessionsLocked() {
	now := r.debugClock()
	for id, session := range r.debugSessions {
		if now.Sub(session.CreatedAt) < r.debugPolicy.SessionTTL {
			continue
		}
		delete(r.debugSessions, id)
		session.Close()
		r.observeDebugSessions(len(r.debugSessions), observability.OutcomeDropped)
	}
}

func (r *Resolver) debugClock() time.Time {
	if r.debugNow != nil {
		return r.debugNow()
	}
	return time.Now()
}

func (r *Resolver) observeDebugSessions(active int, outcome observability.Outcome) {
	if r.debugObserver == nil {
		return
	}
	r.debugObserver.RecordWorkflowDebugSession(outcome)
	r.debugObserver.SetWorkflowDebugSessions(active)
}
