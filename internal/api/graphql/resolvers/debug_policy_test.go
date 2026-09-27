package resolvers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/graphql/model"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/observability"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/workflow"
)

type recordingDebugObserver struct {
	mu       sync.Mutex
	active   int
	outcomes map[observability.Outcome]int
}

func (o *recordingDebugObserver) SetWorkflowDebugSessions(active int) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.active = active
}

func (o *recordingDebugObserver) RecordWorkflowDebugSession(outcome observability.Outcome) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.outcomes == nil {
		o.outcomes = make(map[observability.Outcome]int)
	}
	o.outcomes[outcome]++
}

func (o *recordingDebugObserver) snapshot() (int, map[observability.Outcome]int) {
	o.mu.Lock()
	defer o.mu.Unlock()
	copied := make(map[observability.Outcome]int, len(o.outcomes))
	for k, v := range o.outcomes {
		copied[k] = v
	}
	return o.active, copied
}

const debugPolicyWorkflow = `name: bounded
version: "1.0"
routes:
  - name: r
    filter:
      event_type: TEST
    actions:
      - type: log
        message: "hello"
`

func startTestDebugSession(t *testing.T, mutation *mutationResolver) (*model.DebugSessionModel, error) {
	t.Helper()
	return mutation.StartDebugSession(context.Background(), model.StartDebugSessionInput{
		WorkflowYaml: debugPolicyWorkflow,
		Event:        map[string]interface{}{"type": "TEST", "source": "bounds"},
	})
}

func TestStartDebugSessionIsBoundedPerProcess(t *testing.T) {
	observer := &recordingDebugObserver{}
	resolver := NewResolver(
		WithWorkflowDebugPolicy(WorkflowDebugPolicy{MaxSessions: 2, SessionTTL: time.Hour}),
		WithWorkflowDebugObserver(observer),
	)
	mutation := &mutationResolver{resolver}

	first, err := startTestDebugSession(t, mutation)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := startTestDebugSession(t, mutation); err != nil {
		t.Fatal(err)
	}
	_, err = startTestDebugSession(t, mutation)
	if !errors.Is(err, ErrWorkflowDebugCapacity) {
		t.Fatalf("third session: err = %v, want ErrWorkflowDebugCapacity", err)
	}
	if err.Error() != ErrWorkflowDebugCapacity.Error() {
		t.Fatalf("capacity refusal is not the inventory-safe message: %v", err)
	}

	if ended, err := mutation.DebugEndSession(context.Background(), first.ID); err != nil || !ended {
		t.Fatalf("DebugEndSession: %v %v", ended, err)
	}
	if _, err := startTestDebugSession(t, mutation); err != nil {
		t.Fatalf("session after ending one: %v", err)
	}

	active, outcomes := observer.snapshot()
	if active != 2 {
		t.Errorf("active gauge = %d, want 2", active)
	}
	if outcomes[observability.OutcomeAccepted] != 3 || outcomes[observability.OutcomeRejected] != 1 || outcomes[observability.OutcomeProcessed] != 1 {
		t.Errorf("outcomes = %v", outcomes)
	}
}

func TestDebugSessionsExpireAfterTTL(t *testing.T) {
	observer := &recordingDebugObserver{}
	resolver := NewResolver(
		WithWorkflowDebugPolicy(WorkflowDebugPolicy{MaxSessions: 1, SessionTTL: 15 * time.Minute}),
		WithWorkflowDebugObserver(observer),
	)
	now := time.Now()
	resolver.debugNow = func() time.Time { return now }
	mutation := &mutationResolver{resolver}
	query := &queryResolver{resolver}

	session, err := startTestDebugSession(t, mutation)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := startTestDebugSession(t, mutation); !errors.Is(err, ErrWorkflowDebugCapacity) {
		t.Fatalf("expected capacity refusal before the TTL, got %v", err)
	}

	now = now.Add(15*time.Minute + time.Second)
	if expired, err := query.DebugSession(context.Background(), session.ID); err != nil || expired != nil {
		t.Fatalf("expired session is still reachable: %+v, %v", expired, err)
	}
	if _, err := startTestDebugSession(t, mutation); err != nil {
		t.Fatalf("session after expiry: %v", err)
	}
	_, outcomes := observer.snapshot()
	if outcomes[observability.OutcomeDropped] != 1 {
		t.Errorf("expired sessions counted = %d, want 1 (outcomes %v)", outcomes[observability.OutcomeDropped], outcomes)
	}
}

func TestZeroMaxSessionsDisablesTheDebugger(t *testing.T) {
	mutation := &mutationResolver{NewResolver(WithWorkflowDebugPolicy(WorkflowDebugPolicy{MaxSessions: 0}))}
	if _, err := startTestDebugSession(t, mutation); !errors.Is(err, ErrWorkflowDebugCapacity) {
		t.Fatalf("err = %v, want ErrWorkflowDebugCapacity", err)
	}
}

// TestStartDebugSessionStubsActionsByDefault is the resolver-level regression
// test for SEC-2026-09-27-1: the default resolver's debugger reaches no
// network destination named by the caller's YAML.
func TestStartDebugSessionStubsActionsByDefault(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	resolver := NewResolver()
	mutation := &mutationResolver{resolver}
	session, err := mutation.StartDebugSession(context.Background(), model.StartDebugSessionInput{
		WorkflowYaml: `name: ssrf
version: "1.0"
routes:
  - name: r
    filter:
      event_type: TEST
    actions:
      - type: webhook
        url: ` + server.URL + `/internal
`,
		Event: map[string]interface{}{"type": "TEST", "source": "ssrf"},
	})
	if err != nil {
		t.Fatal(err)
	}
	// Step through every span to completion so each action step is recorded.
	query := &queryResolver{resolver}
	deadline := time.Now().Add(5 * time.Second)
	var current *model.DebugSessionModel
	for {
		current, err = query.DebugSession(context.Background(), session.ID)
		if err != nil {
			t.Fatal(err)
		}
		if current.State == string(workflow.DebugStateComplete) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("session did not complete; state %s", current.State)
		}
		if current.State == string(workflow.DebugStatePaused) {
			if _, err := mutation.DebugStep(context.Background(), session.ID); err != nil {
				t.Fatal(err)
			}
			continue
		}
		time.Sleep(10 * time.Millisecond)
	}
	if current.State != string(workflow.DebugStateComplete) {
		t.Fatalf("session state = %s, want completed", current.State)
	}
	if hits.Load() != 0 {
		t.Fatalf("debugger sent %d requests to the caller's webhook", hits.Load())
	}
	var sawStub bool
	for _, step := range current.Steps {
		if step.Kind == string(workflow.DebugStepAction) {
			sawStub = step.Variables[workflow.AttrActionStubbed] == true
		}
	}
	if !sawStub {
		t.Fatalf("no stubbed action step recorded: %+v", current.Steps)
	}
}

func TestWithWorkflowDebugPolicyNormalises(t *testing.T) {
	resolver := NewResolver(WithWorkflowDebugPolicy(WorkflowDebugPolicy{MaxSessions: -3}))
	if resolver.debugPolicy.MaxSessions != 0 || resolver.debugPolicy.SessionTTL != DefaultWorkflowDebugSessionTTL {
		t.Fatalf("policy = %+v", resolver.debugPolicy)
	}
	if def := NewResolver().debugPolicy; def.MaxSessions != DefaultWorkflowDebugMaxSessions || len(def.ExecutableActions) != 0 {
		t.Fatalf("default policy = %+v", def)
	}
}
