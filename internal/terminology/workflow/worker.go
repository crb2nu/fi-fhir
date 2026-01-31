package workflow

import (
	"context"
	"fmt"

	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/terminology/autoroute"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/terminology/db"
)

// WorkerConfig configures the Temporal worker.
type WorkerConfig struct {
	// Temporal client options
	HostPort  string // Default: "localhost:7233"
	Namespace string // Default: "default"

	// Worker options
	MaxConcurrentActivityExecutionSize     int // Default: 10
	MaxConcurrentWorkflowTaskExecutionSize int // Default: 10
}

// DefaultWorkerConfig returns default worker configuration.
func DefaultWorkerConfig() WorkerConfig {
	return WorkerConfig{
		HostPort:                               "localhost:7233",
		Namespace:                              "default",
		MaxConcurrentActivityExecutionSize:     10,
		MaxConcurrentWorkflowTaskExecutionSize: 10,
	}
}

// Worker manages the Temporal worker for terminology workflows.
type Worker struct {
	client     client.Client
	worker     worker.Worker
	activities *Activities
}

// NewWorker creates a new Temporal worker with the given dependencies.
func NewWorker(ctx context.Context, cfg WorkerConfig, engine *autoroute.Engine, store *db.MappingStore) (*Worker, error) {
	// Apply defaults
	if cfg.HostPort == "" {
		cfg.HostPort = "localhost:7233"
	}
	if cfg.Namespace == "" {
		cfg.Namespace = "default"
	}

	// Create Temporal client
	c, err := client.Dial(client.Options{
		HostPort:  cfg.HostPort,
		Namespace: cfg.Namespace,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create Temporal client: %w", err)
	}

	// Create activities with dependencies
	activities := NewActivities(engine, store)

	// Configure worker options
	workerOpts := worker.Options{}
	if cfg.MaxConcurrentActivityExecutionSize > 0 {
		workerOpts.MaxConcurrentActivityExecutionSize = cfg.MaxConcurrentActivityExecutionSize
	}
	if cfg.MaxConcurrentWorkflowTaskExecutionSize > 0 {
		workerOpts.MaxConcurrentWorkflowTaskExecutionSize = cfg.MaxConcurrentWorkflowTaskExecutionSize
	}

	// Create worker
	w := worker.New(c, TaskQueue, workerOpts)

	// Register workflow
	w.RegisterWorkflow(TerminologyReview)

	// Register activities
	w.RegisterActivity(activities.CheckExistingMapping)
	w.RegisterActivity(activities.SuggestMapping)
	w.RegisterActivity(activities.CreatePendingAutoroute)
	w.RegisterActivity(activities.ApproveMapping)
	w.RegisterActivity(activities.RejectMapping)
	w.RegisterActivity(activities.RecordDecision)

	return &Worker{
		client:     c,
		worker:     w,
		activities: activities,
	}, nil
}

// Start starts the worker in blocking mode.
// Call this from a goroutine if you need non-blocking behavior.
func (w *Worker) Start() error {
	return w.worker.Run(worker.InterruptCh())
}

// Stop gracefully stops the worker.
func (w *Worker) Stop() {
	w.worker.Stop()
	w.client.Close()
}

// Client returns the Temporal client for starting workflows.
func (w *Worker) Client() client.Client {
	return w.client
}

// StartReviewWorkflow starts a new terminology review workflow.
func (w *Worker) StartReviewWorkflow(ctx context.Context, input TerminologyReviewInput) (client.WorkflowRun, error) {
	options := client.StartWorkflowOptions{
		ID:        fmt.Sprintf("term-review-%s-%s-%s", input.SourceSystem, input.SourceCode, input.TargetSystem),
		TaskQueue: TaskQueue,
	}

	return w.client.ExecuteWorkflow(ctx, options, TerminologyReview, input)
}

// SignalReviewDecision sends a review decision signal to a running workflow.
func (w *Worker) SignalReviewDecision(ctx context.Context, workflowID string, decision ReviewDecisionSignal) error {
	return w.client.SignalWorkflow(ctx, workflowID, "", SignalNameReviewDecision, decision)
}

// GetWorkflowResult waits for a workflow to complete and returns its result.
func (w *Worker) GetWorkflowResult(ctx context.Context, run client.WorkflowRun) (*TerminologyReviewOutput, error) {
	var result TerminologyReviewOutput
	if err := run.Get(ctx, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
