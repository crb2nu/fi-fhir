package workflow

import (
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"go.temporal.io/sdk/testsuite"
)

// TestTerminologyReview_HumanRejected verifies that a signal with Approved=false
// triggers the rejection path and records the decision correctly.
func TestTerminologyReview_HumanRejected(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	env.RegisterWorkflow(TerminologyReview)

	env.OnActivity((*Activities).CheckExistingMapping, mock.Anything, mock.Anything, mock.Anything).
		Return(&CheckExistingMappingOutput{Exists: false}, nil).Once()
	env.OnActivity((*Activities).SuggestMapping, mock.Anything, mock.Anything, mock.Anything).
		Return(&SuggestMappingOutput{
			BestMatch:  &CandidateResult{Code: "4548-4", Display: "Hemoglobin A1c", Equivalence: "equivalent"},
			Confidence: 0.82,
		}, nil).Once()
	env.OnActivity((*Activities).CreatePendingAutoroute, mock.Anything, mock.Anything, mock.Anything).
		Return(&CreatePendingAutorouteOutput{ID: 20}, nil).Once()
	env.OnActivity((*Activities).RejectMapping, mock.Anything, mock.Anything, mock.Anything).
		Return(nil).Once()
	env.OnActivity((*Activities).RecordDecision, mock.Anything, mock.Anything, mock.Anything).
		Return(nil).Once()

	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalNameReviewDecision, ReviewDecisionSignal{
			Approved:  false,
			DecidedBy: "rejector@example.com",
			Comment:   "wrong code",
		})
	}, time.Second)

	env.ExecuteWorkflow(TerminologyReview, TerminologyReviewInput{
		SourceCode:   "HBA1C",
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
	if out.Status != "rejected" {
		t.Fatalf("status=%q want rejected", out.Status)
	}
	if out.DecidedBy != "rejector@example.com" {
		t.Fatalf("decided by=%q want rejector@example.com", out.DecidedBy)
	}
	if out.PendingID != 20 {
		t.Fatalf("pending id=%d want 20", out.PendingID)
	}
	env.AssertExpectations(t)
}

// TestTerminologyReview_CustomThreshold verifies that the auto-approve threshold
// from the input takes precedence over the constant default.
func TestTerminologyReview_CustomThreshold(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	env.RegisterWorkflow(TerminologyReview)

	env.OnActivity((*Activities).CheckExistingMapping, mock.Anything, mock.Anything, mock.Anything).
		Return(&CheckExistingMappingOutput{Exists: false}, nil).Once()
	// Confidence=0.80 is below the default threshold (0.95) but above the custom 0.75
	env.OnActivity((*Activities).SuggestMapping, mock.Anything, mock.Anything, mock.Anything).
		Return(&SuggestMappingOutput{
			BestMatch:  &CandidateResult{Code: "789-8", Display: "Erythrocytes", Equivalence: "equivalent"},
			Confidence: 0.80,
		}, nil).Once()
	env.OnActivity((*Activities).CreatePendingAutoroute, mock.Anything, mock.Anything, mock.Anything).
		Return(&CreatePendingAutorouteOutput{ID: 30}, nil).Once()
	env.OnActivity((*Activities).ApproveMapping, mock.Anything, mock.Anything, mock.Anything).
		Return(&ApproveMappingOutput{MappingID: 31}, nil).Once()
	env.OnActivity((*Activities).RecordDecision, mock.Anything, mock.Anything, mock.Anything).
		Return(nil).Once()

	env.ExecuteWorkflow(TerminologyReview, TerminologyReviewInput{
		SourceCode:           "RBC001",
		SourceSystem:         "epic_labs",
		TargetSystem:         "http://loinc.org",
		AutoApproveThreshold: 0.75,
	})

	if err := env.GetWorkflowError(); err != nil {
		t.Fatalf("workflow error: %v", err)
	}

	var out TerminologyReviewOutput
	if err := env.GetWorkflowResult(&out); err != nil {
		t.Fatalf("get result: %v", err)
	}
	if out.Status != "auto_approved" {
		t.Fatalf("status=%q want auto_approved (custom threshold 0.75 should trigger auto-approve at 0.80)", out.Status)
	}
	if out.MappingID != 31 {
		t.Fatalf("mapping id=%d want 31", out.MappingID)
	}
	env.AssertExpectations(t)
}

// TestTerminologyReview_WithAlternates verifies that alternate candidates
// are serialized and passed through the workflow correctly.
func TestTerminologyReview_WithAlternates(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	env.RegisterWorkflow(TerminologyReview)

	env.OnActivity((*Activities).CheckExistingMapping, mock.Anything, mock.Anything, mock.Anything).
		Return(&CheckExistingMappingOutput{Exists: false}, nil).Once()
	env.OnActivity((*Activities).SuggestMapping, mock.Anything, mock.Anything, mock.Anything).
		Return(&SuggestMappingOutput{
			BestMatch: &CandidateResult{Code: "2345-7", Display: "Glucose", Equivalence: "equivalent"},
			Alternates: []CandidateResult{
				{Code: "2339-0", Display: "Glucose [Mass]", Equivalence: "wider", Confidence: 0.85},
				{Code: "6777-7", Display: "Glucose tolerance", Equivalence: "narrower", Confidence: 0.70},
			},
			Confidence: 0.99,
		}, nil).Once()
	env.OnActivity((*Activities).CreatePendingAutoroute, mock.Anything, mock.Anything, mock.Anything).
		Return(&CreatePendingAutorouteOutput{ID: 40}, nil).Once()
	env.OnActivity((*Activities).ApproveMapping, mock.Anything, mock.Anything, mock.Anything).
		Return(&ApproveMappingOutput{MappingID: 41}, nil).Once()
	env.OnActivity((*Activities).RecordDecision, mock.Anything, mock.Anything, mock.Anything).
		Return(nil).Once()

	env.ExecuteWorkflow(TerminologyReview, TerminologyReviewInput{
		SourceCode:    "GLU002",
		SourceSystem:  "epic_labs",
		TargetSystem:  "http://loinc.org",
		MaxCandidates: 3,
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
	if out.FinalCode != "2345-7" {
		t.Fatalf("final code=%q want 2345-7", out.FinalCode)
	}
	env.AssertExpectations(t)
}

// TestTerminologyReview_WithSourceDisplay verifies the source display is propagated.
func TestTerminologyReview_WithSourceDisplay(t *testing.T) {
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
		SourceCode:    "CUSTOM_LAB",
		SourceSystem:  "epic_labs",
		SourceDisplay: "Custom Lab Display Name",
		TargetSystem:  "http://loinc.org",
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

// TestNewActivities_NilDependencies verifies that Activities struct can be created
// with nil dependencies (for testing).
func TestNewActivities_NilDependencies(t *testing.T) {
	a := NewActivities(nil, nil)
	if a == nil {
		t.Fatal("expected non-nil Activities")
	}
	if a.Engine != nil {
		t.Error("expected nil Engine")
	}
	if a.MappingStore != nil {
		t.Error("expected nil MappingStore")
	}
}

// TestCandidateResult_Serialization verifies CandidateResult JSON round-trip.
func TestCandidateResult_Serialization(t *testing.T) {
	c := CandidateResult{
		Code:        "12345",
		Display:     "Test",
		System:      "http://loinc.org",
		Confidence:  0.95,
		Equivalence: "equivalent",
		Reasoning:   "exact match",
		Score:       0.95,
	}

	data := marshalAlternates([]CandidateResult{c})
	if data == nil {
		t.Fatal("expected non-nil marshaled data")
	}
}
