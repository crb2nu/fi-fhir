package workflow

import (
	"context"
	"errors"
	"testing"

	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
)

// stubClient embeds client.Client so it satisfies the interface.
// Only the methods used by Worker are implemented; others panic if called.
type stubClient struct {
	client.Client // nil-embed – satisfies the interface
	executeFn     func(ctx context.Context, options client.StartWorkflowOptions, workflow interface{}, args ...interface{}) (client.WorkflowRun, error)
	signalFn      func(ctx context.Context, workflowID string, runID string, signalName string, arg interface{}) error
	closed        bool
}

func (s *stubClient) ExecuteWorkflow(ctx context.Context, options client.StartWorkflowOptions, wf interface{}, args ...interface{}) (client.WorkflowRun, error) {
	return s.executeFn(ctx, options, wf, args...)
}

func (s *stubClient) SignalWorkflow(ctx context.Context, workflowID string, runID string, signalName string, arg interface{}) error {
	return s.signalFn(ctx, workflowID, runID, signalName, arg)
}

func (s *stubClient) Close() {
	s.closed = true
}

// stubWorker satisfies worker.Worker for testing Start/Stop.
type stubWorker struct {
	worker.Worker // nil-embed
	runFn         func(interruptCh <-chan interface{}) error
	stopped       bool
}

func (s *stubWorker) Run(interruptCh <-chan interface{}) error {
	return s.runFn(interruptCh)
}

func (s *stubWorker) Stop() {
	s.stopped = true
}

// stubWorkflowRun satisfies client.WorkflowRun.
type stubWorkflowRun struct {
	id    string
	runID string
	getFn func(ctx context.Context, valuePtr interface{}) error
}

func (s *stubWorkflowRun) GetID() string    { return s.id }
func (s *stubWorkflowRun) GetRunID() string { return s.runID }
func (s *stubWorkflowRun) Get(ctx context.Context, valuePtr interface{}) error {
	return s.getFn(ctx, valuePtr)
}
func (s *stubWorkflowRun) GetWithOptions(ctx context.Context, valuePtr interface{}, _ client.WorkflowRunGetOptions) error {
	return s.getFn(ctx, valuePtr)
}

// newTestWorker creates a Worker with stub dependencies for unit testing.
func newTestWorker(c client.Client, w worker.Worker) *Worker {
	return &Worker{
		client:     c,
		worker:     w,
		activities: NewActivities(nil, nil),
	}
}

func TestWorker_Client(t *testing.T) {
	sc := &stubClient{}
	sw := &stubWorker{}
	w := newTestWorker(sc, sw)

	if got := w.Client(); got != sc {
		t.Fatal("Client() returned unexpected value")
	}
}

func TestWorker_Stop(t *testing.T) {
	sc := &stubClient{}
	sw := &stubWorker{}
	w := newTestWorker(sc, sw)

	w.Stop()

	if !sw.stopped {
		t.Fatal("expected worker.Stop() to be called")
	}
	if !sc.closed {
		t.Fatal("expected client.Close() to be called")
	}
}

func TestWorker_Start(t *testing.T) {
	sc := &stubClient{}
	sw := &stubWorker{
		runFn: func(interruptCh <-chan interface{}) error {
			return nil
		},
	}
	w := newTestWorker(sc, sw)

	if err := w.Start(); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
}

func TestWorker_Start_Error(t *testing.T) {
	sc := &stubClient{}
	sw := &stubWorker{
		runFn: func(interruptCh <-chan interface{}) error {
			return errors.New("worker failed")
		},
	}
	w := newTestWorker(sc, sw)

	if err := w.Start(); err == nil {
		t.Fatal("expected error from Start()")
	}
}

func TestWorker_StartAsync(t *testing.T) {
	sc := &stubClient{}
	sw := &stubWorker{
		runFn: func(interruptCh <-chan interface{}) error {
			return nil
		},
	}
	w := newTestWorker(sc, sw)

	errCh := w.StartAsync()
	if err := <-errCh; err != nil {
		t.Fatalf("StartAsync error: %v", err)
	}
}

func TestWorker_StartReviewWorkflow(t *testing.T) {
	expectedRun := &stubWorkflowRun{id: "term-review-epic_labs-GLU-http://loinc.org", runID: "run-1"}
	sc := &stubClient{
		executeFn: func(ctx context.Context, options client.StartWorkflowOptions, wf interface{}, args ...interface{}) (client.WorkflowRun, error) {
			if options.TaskQueue != TaskQueue {
				t.Fatalf("unexpected task queue: %s", options.TaskQueue)
			}
			if options.ID != "term-review-epic_labs-GLU-http://loinc.org" {
				t.Fatalf("unexpected workflow ID: %s", options.ID)
			}
			return expectedRun, nil
		},
	}
	sw := &stubWorker{}
	w := newTestWorker(sc, sw)

	run, err := w.StartReviewWorkflow(t.Context(), TerminologyReviewInput{
		SourceCode:   "GLU",
		SourceSystem: "epic_labs",
		TargetSystem: "http://loinc.org",
	})
	if err != nil {
		t.Fatalf("StartReviewWorkflow error: %v", err)
	}
	if run.GetID() != expectedRun.id {
		t.Fatalf("workflow ID=%q want %q", run.GetID(), expectedRun.id)
	}
}

func TestWorker_StartReviewWorkflow_Error(t *testing.T) {
	sc := &stubClient{
		executeFn: func(ctx context.Context, options client.StartWorkflowOptions, wf interface{}, args ...interface{}) (client.WorkflowRun, error) {
			return nil, errors.New("execution failed")
		},
	}
	sw := &stubWorker{}
	w := newTestWorker(sc, sw)

	_, err := w.StartReviewWorkflow(t.Context(), TerminologyReviewInput{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestWorker_SignalReviewDecision(t *testing.T) {
	var capturedSignalName string
	sc := &stubClient{
		signalFn: func(ctx context.Context, workflowID, runID, signalName string, arg interface{}) error {
			capturedSignalName = signalName
			if workflowID != "wf-123" {
				t.Fatalf("workflow ID=%q want wf-123", workflowID)
			}
			return nil
		},
	}
	sw := &stubWorker{}
	w := newTestWorker(sc, sw)

	err := w.SignalReviewDecision(t.Context(), "wf-123", ReviewDecisionSignal{
		Approved:  true,
		DecidedBy: "test@example.com",
	})
	if err != nil {
		t.Fatalf("SignalReviewDecision error: %v", err)
	}
	if capturedSignalName != SignalNameReviewDecision {
		t.Fatalf("signal name=%q want %q", capturedSignalName, SignalNameReviewDecision)
	}
}

func TestWorker_SignalReviewDecision_Error(t *testing.T) {
	sc := &stubClient{
		signalFn: func(ctx context.Context, workflowID, runID, signalName string, arg interface{}) error {
			return errors.New("signal failed")
		},
	}
	sw := &stubWorker{}
	w := newTestWorker(sc, sw)

	err := w.SignalReviewDecision(t.Context(), "wf-123", ReviewDecisionSignal{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestWorker_GetWorkflowResult(t *testing.T) {
	run := &stubWorkflowRun{
		id:    "wf-123",
		runID: "run-1",
		getFn: func(ctx context.Context, valuePtr interface{}) error {
			out := valuePtr.(*TerminologyReviewOutput)
			out.Status = "approved"
			out.MappingID = 42
			return nil
		},
	}

	sc := &stubClient{}
	sw := &stubWorker{}
	w := newTestWorker(sc, sw)

	result, err := w.GetWorkflowResult(t.Context(), run)
	if err != nil {
		t.Fatalf("GetWorkflowResult error: %v", err)
	}
	if result.Status != "approved" {
		t.Fatalf("status=%q want approved", result.Status)
	}
	if result.MappingID != 42 {
		t.Fatalf("mapping id=%d want 42", result.MappingID)
	}
}

func TestWorker_GetWorkflowResult_Error(t *testing.T) {
	run := &stubWorkflowRun{
		getFn: func(ctx context.Context, valuePtr interface{}) error {
			return errors.New("workflow failed")
		},
	}

	sc := &stubClient{}
	sw := &stubWorker{}
	w := newTestWorker(sc, sw)

	_, err := w.GetWorkflowResult(t.Context(), run)
	if err == nil {
		t.Fatal("expected error")
	}
}
