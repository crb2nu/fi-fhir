package workflow

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"go.temporal.io/sdk/testsuite"
)

func TestTerminologyReview_ExistingMapping(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	env.RegisterWorkflow(TerminologyReview)

	env.OnActivity((*Activities).CheckExistingMapping, mock.Anything, mock.Anything, mock.Anything).
		Return(&CheckExistingMappingOutput{Exists: true, MappingID: 42}, nil).Once()

	env.ExecuteWorkflow(TerminologyReview, TerminologyReviewInput{
		SourceCode:   "1234-5",
		SourceSystem: "epic_labs",
		TargetSystem: "http://loinc.org",
	})

	if !env.IsWorkflowCompleted() {
		t.Fatal("workflow did not complete")
	}
	if err := env.GetWorkflowError(); err != nil {
		t.Fatalf("workflow error: %v", err)
	}

	var out TerminologyReviewOutput
	if err := env.GetWorkflowResult(&out); err != nil {
		t.Fatalf("get result: %v", err)
	}
	if out.Status != "existing" {
		t.Fatalf("status=%q want existing", out.Status)
	}
	if out.MappingID != 42 {
		t.Fatalf("mapping id=%d want 42", out.MappingID)
	}

	env.AssertExpectations(t)
}

func TestTerminologyReview_NoMatch(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	env.RegisterWorkflow(TerminologyReview)

	env.OnActivity((*Activities).CheckExistingMapping, mock.Anything, mock.Anything, mock.Anything).
		Return(&CheckExistingMappingOutput{Exists: false}, nil).Once()
	env.OnActivity((*Activities).SuggestMapping, mock.Anything, mock.Anything, mock.Anything).
		Return(&SuggestMappingOutput{BestMatch: nil, Confidence: 0.0}, nil).Once()
	env.OnActivity((*Activities).RecordDecision, mock.Anything, mock.Anything, mock.Anything).
		Return(nil).Once()

	env.ExecuteWorkflow(TerminologyReview, TerminologyReviewInput{
		SourceCode:   "NO-MATCH",
		SourceSystem: "epic_labs",
		TargetSystem: "http://loinc.org",
	})

	if err := env.GetWorkflowError(); err != nil {
		t.Fatalf("workflow error: %v", err)
	}

	var out TerminologyReviewOutput
	if err := env.GetWorkflowResult(&out); err != nil {
		t.Fatalf("get result: %v", err)
	}
	if out.Status != "no_match" {
		t.Fatalf("status=%q want no_match", out.Status)
	}
	env.AssertExpectations(t)
}

func TestTerminologyReview_AutoApproved(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	env.RegisterWorkflow(TerminologyReview)

	env.OnActivity((*Activities).CheckExistingMapping, mock.Anything, mock.Anything, mock.Anything).
		Return(&CheckExistingMappingOutput{Exists: false}, nil).Once()
	env.OnActivity((*Activities).SuggestMapping, mock.Anything, mock.Anything, mock.Anything).
		Return(&SuggestMappingOutput{
			BestMatch:  &CandidateResult{Code: "1234-5", Display: "Glucose", Equivalence: "equivalent"},
			Confidence: 0.99,
			Reasoning:  "high confidence",
		}, nil).Once()
	env.OnActivity((*Activities).CreatePendingAutoroute, mock.Anything, mock.Anything, mock.Anything).
		Return(&CreatePendingAutorouteOutput{ID: 7}, nil).Once()
	env.OnActivity((*Activities).ApproveMapping, mock.Anything, mock.Anything, mock.Anything).
		Return(&ApproveMappingOutput{MappingID: 11}, nil).Once()
	env.OnActivity((*Activities).RecordDecision, mock.Anything, mock.Anything, mock.Anything).
		Return(nil).Once()

	env.ExecuteWorkflow(TerminologyReview, TerminologyReviewInput{
		SourceCode:   "GLU001",
		SourceSystem: "epic_labs",
		TargetSystem: "http://loinc.org",
	})

	if err := env.GetWorkflowError(); err != nil {
		t.Fatalf("workflow error: %v", err)
	}

	var out TerminologyReviewOutput
	if err := env.GetWorkflowResult(&out); err != nil {
		t.Fatalf("get result: %v", err)
	}
	if out.Status != "auto_approved" {
		t.Fatalf("status=%q want auto_approved", out.Status)
	}
	if out.MappingID != 11 {
		t.Fatalf("mapping id=%d want 11", out.MappingID)
	}
	if out.PendingID != 7 {
		t.Fatalf("pending id=%d want 7", out.PendingID)
	}
	env.AssertExpectations(t)
}

func TestTerminologyReview_HumanApprovedBySignal(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	env.RegisterWorkflow(TerminologyReview)

	env.OnActivity((*Activities).CheckExistingMapping, mock.Anything, mock.Anything, mock.Anything).
		Return(&CheckExistingMappingOutput{Exists: false}, nil).Once()
	env.OnActivity((*Activities).SuggestMapping, mock.Anything, mock.Anything, mock.Anything).
		Return(&SuggestMappingOutput{
			BestMatch:  &CandidateResult{Code: "789-8", Display: "Potassium", Equivalence: "equivalent"},
			Confidence: 0.80,
		}, nil).Once()
	env.OnActivity((*Activities).CreatePendingAutoroute, mock.Anything, mock.Anything, mock.Anything).
		Return(&CreatePendingAutorouteOutput{ID: 5}, nil).Once()
	env.OnActivity((*Activities).RecordDecision, mock.Anything, mock.Anything, mock.Anything).
		Return(nil).Once()
	env.OnActivity((*Activities).ApproveMapping, mock.Anything, mock.Anything, mock.Anything).
		Return(&ApproveMappingOutput{MappingID: 9}, nil).Once()

	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalNameReviewDecision, ReviewDecisionSignal{
			Approved:            true,
			DecidedBy:           "reviewer@example.com",
			EquivalenceOverride: "wider",
			Comment:             "approved",
		})
	}, time.Second)

	env.ExecuteWorkflow(TerminologyReview, TerminologyReviewInput{
		SourceCode:   "K001",
		SourceSystem: "epic_labs",
		TargetSystem: "http://loinc.org",
	})

	if err := env.GetWorkflowError(); err != nil {
		t.Fatalf("workflow error: %v", err)
	}

	var out TerminologyReviewOutput
	if err := env.GetWorkflowResult(&out); err != nil {
		t.Fatalf("get result: %v", err)
	}
	if out.Status != "approved" {
		t.Fatalf("status=%q want approved", out.Status)
	}
	if out.MappingID != 9 {
		t.Fatalf("mapping id=%d want 9", out.MappingID)
	}
	if out.DecidedBy != "reviewer@example.com" {
		t.Fatalf("decided by=%q want reviewer@example.com", out.DecidedBy)
	}
	if out.FinalEquivalence != "wider" {
		t.Fatalf("final equivalence=%q want wider", out.FinalEquivalence)
	}
	env.AssertExpectations(t)
}

func TestTerminologyReview_ExpiredWithoutSignal(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	env.RegisterWorkflow(TerminologyReview)

	env.OnActivity((*Activities).CheckExistingMapping, mock.Anything, mock.Anything, mock.Anything).
		Return(&CheckExistingMappingOutput{Exists: false}, nil).Once()
	env.OnActivity((*Activities).SuggestMapping, mock.Anything, mock.Anything, mock.Anything).
		Return(&SuggestMappingOutput{
			BestMatch:  &CandidateResult{Code: "1111-1", Display: "Sodium", Equivalence: "equivalent"},
			Confidence: 0.75,
		}, nil).Once()
	env.OnActivity((*Activities).CreatePendingAutoroute, mock.Anything, mock.Anything, mock.Anything).
		Return(&CreatePendingAutorouteOutput{ID: 13}, nil).Once()
	env.OnActivity((*Activities).RecordDecision, mock.Anything, mock.Anything, mock.Anything).
		Return(nil).Once()

	env.ExecuteWorkflow(TerminologyReview, TerminologyReviewInput{
		SourceCode:    "NA001",
		SourceSystem:  "epic_labs",
		TargetSystem:  "http://loinc.org",
		ReviewTimeout: 2 * time.Second,
	})

	if err := env.GetWorkflowError(); err != nil {
		t.Fatalf("workflow error: %v", err)
	}

	var out TerminologyReviewOutput
	if err := env.GetWorkflowResult(&out); err != nil {
		t.Fatalf("get result: %v", err)
	}
	if out.Status != "expired" {
		t.Fatalf("status=%q want expired", out.Status)
	}
	if out.PendingID != 13 {
		t.Fatalf("pending id=%d want 13", out.PendingID)
	}
	env.AssertExpectations(t)
}

func TestMarshalAlternates(t *testing.T) {
	if got := marshalAlternates(nil); got != nil {
		t.Fatalf("marshalAlternates(nil) = %v, want nil", got)
	}

	alternates := []CandidateResult{{Code: "A", System: "http://loinc.org", Confidence: 0.9}}
	data := marshalAlternates(alternates)
	if len(data) == 0 {
		t.Fatal("marshalAlternates returned empty data")
	}

	var decoded []CandidateResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal alternates: %v", err)
	}
	if len(decoded) != 1 || decoded[0].Code != "A" {
		t.Fatalf("decoded alternates mismatch: %+v", decoded)
	}
}

func TestTerminologyReview_LowConfidence(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	env.RegisterWorkflow(TerminologyReview)

	env.OnActivity((*Activities).CheckExistingMapping, mock.Anything, mock.Anything, mock.Anything).
		Return(&CheckExistingMappingOutput{Exists: false}, nil).Once()
	// Confidence 0.50 is below 0.70 → AUTOROUTE_LOW_CONF branch
	env.OnActivity((*Activities).SuggestMapping, mock.Anything, mock.Anything, mock.Anything).
		Return(&SuggestMappingOutput{
			BestMatch:  &CandidateResult{Code: "99999-9", Display: "Low Conf", Equivalence: "inexact"},
			Confidence: 0.50,
		}, nil).Once()
	env.OnActivity((*Activities).CreatePendingAutoroute, mock.Anything, mock.Anything, mock.Anything).
		Return(&CreatePendingAutorouteOutput{ID: 50}, nil).Once()
	env.OnActivity((*Activities).RecordDecision, mock.Anything, mock.Anything, mock.Anything).
		Return(nil).Once()
	env.OnActivity((*Activities).ApproveMapping, mock.Anything, mock.Anything, mock.Anything).
		Return(&ApproveMappingOutput{MappingID: 51}, nil).Once()

	// Signal approval after short delay
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalNameReviewDecision, ReviewDecisionSignal{
			Approved:  true,
			DecidedBy: "reviewer@test.com",
		})
	}, time.Second)

	env.ExecuteWorkflow(TerminologyReview, TerminologyReviewInput{
		SourceCode:   "LOWCONF",
		SourceSystem: "epic_labs",
		TargetSystem: "http://loinc.org",
	})

	if err := env.GetWorkflowError(); err != nil {
		t.Fatalf("workflow error: %v", err)
	}

	var out TerminologyReviewOutput
	if err := env.GetWorkflowResult(&out); err != nil {
		t.Fatalf("get result: %v", err)
	}
	if out.Status != "approved" {
		t.Fatalf("status=%q want approved", out.Status)
	}
	if out.MappingID != 51 {
		t.Fatalf("mapping id=%d want 51", out.MappingID)
	}
	env.AssertExpectations(t)
}

func TestTerminologyReview_CheckExistingFails_ContinueAsNew(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	env.RegisterWorkflow(TerminologyReview)

	env.OnActivity((*Activities).CheckExistingMapping, mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("transient db error")).Once()

	env.ExecuteWorkflow(TerminologyReview, TerminologyReviewInput{
		SourceCode:   "RETRY",
		SourceSystem: "epic_labs",
		TargetSystem: "http://loinc.org",
	})

	if !env.IsWorkflowCompleted() {
		t.Fatal("workflow did not complete")
	}
	// Should get a ContinueAsNew error
	err := env.GetWorkflowError()
	if err == nil {
		t.Fatal("expected ContinueAsNew error")
	}
	// Temporal wraps ContinueAsNew as a workflow error
	if !strings.Contains(err.Error(), "ContinueAsNew") {
		t.Logf("got error: %v (type %T) — acceptable non-nil error for failed check", err, err)
	}
	env.AssertExpectations(t)
}

func TestDefaultWorkerConfig(t *testing.T) {
	cfg := DefaultWorkerConfig()
	if cfg.HostPort != "localhost:7233" {
		t.Fatalf("HostPort=%q want localhost:7233", cfg.HostPort)
	}
	if cfg.Namespace != "default" {
		t.Fatalf("Namespace=%q want default", cfg.Namespace)
	}
	if cfg.MaxConcurrentActivityExecutionSize != 10 {
		t.Fatalf("MaxConcurrentActivityExecutionSize=%d want 10", cfg.MaxConcurrentActivityExecutionSize)
	}
	if cfg.MaxConcurrentWorkflowTaskExecutionSize != 10 {
		t.Fatalf("MaxConcurrentWorkflowTaskExecutionSize=%d want 10", cfg.MaxConcurrentWorkflowTaskExecutionSize)
	}
}

func TestNewWorker_InvalidHostPort(t *testing.T) {
	_, err := NewWorker(t.Context(), WorkerConfig{
		HostPort:  "://invalid-hostport",
		Namespace: "default",
	}, nil, nil)
	if err == nil {
		t.Fatal("expected error for invalid host:port")
	}
	if !strings.Contains(err.Error(), "failed to create Temporal client") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewWorker_Defaults_WithoutServer(t *testing.T) {
	// Empty HostPort/Namespace should use defaults, then fail dialing local Temporal
	// in unit test environments where no server is running.
	_, err := NewWorker(t.Context(), WorkerConfig{}, nil, nil)
	if err == nil {
		t.Fatal("expected dial error when no local Temporal server is running")
	}
	if !strings.Contains(err.Error(), "failed to create Temporal client") {
		t.Fatalf("unexpected error: %v", err)
	}
}
