package workflow

import (
	"encoding/json"
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

// TaskQueue is the Temporal task queue name for terminology workflows.
const TaskQueue = "terminology-mapping"

// TerminologyReviewInput is the input for the TerminologyReview workflow.
type TerminologyReviewInput struct {
	// Source code information
	SourceCode    string `json:"source_code"`
	SourceSystem  string `json:"source_system"`
	SourceDisplay string `json:"source_display,omitempty"`

	// Target system for the mapping
	TargetSystem string `json:"target_system"`

	// Optional profile context
	ProfileID string `json:"profile_id,omitempty"`

	// Configuration
	AutoApproveThreshold float64       `json:"auto_approve_threshold,omitempty"` // Default: 0.95
	ReviewTimeout        time.Duration `json:"review_timeout,omitempty"`         // Default: 7 days
	MaxCandidates        int           `json:"max_candidates,omitempty"`         // Default: 5
}

// TerminologyReviewOutput is the output of the TerminologyReview workflow.
type TerminologyReviewOutput struct {
	// Outcome
	Status    string `json:"status"` // "approved", "rejected", "auto_approved", "expired", "existing", "no_match"
	MappingID int64  `json:"mapping_id,omitempty"`

	// Decision details
	FinalCode        string  `json:"final_code,omitempty"`
	FinalDisplay     string  `json:"final_display,omitempty"`
	FinalEquivalence string  `json:"final_equivalence,omitempty"`
	Confidence       float64 `json:"confidence,omitempty"`

	// Metadata
	DecidedBy  string        `json:"decided_by,omitempty"`
	ReviewTime time.Duration `json:"review_time,omitempty"`
	PendingID  int64         `json:"pending_id,omitempty"`
}

// ReviewDecisionSignal is sent to the workflow when a human makes a decision.
type ReviewDecisionSignal struct {
	Approved            bool   `json:"approved"`
	DecidedBy           string `json:"decided_by"`
	EquivalenceOverride string `json:"equivalence_override,omitempty"`
	Comment             string `json:"comment,omitempty"`
	RejectionReason     string `json:"rejection_reason,omitempty"`
}

// SignalNameReviewDecision is the signal name for review decisions.
const SignalNameReviewDecision = "review-decision"

// TerminologyReview orchestrates the full terminology mapping review process.
//
// Workflow steps:
// 1. Check if a mapping already exists (skip if found)
// 2. Run the autoroute engine to get suggestions
// 3. If confidence >= threshold, auto-approve
// 4. Otherwise, create pending autoroute and wait for human decision
// 5. On decision (or timeout), finalize the mapping
// 6. Record telemetry for all decisions
func TerminologyReview(ctx workflow.Context, input TerminologyReviewInput) (*TerminologyReviewOutput, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting TerminologyReview workflow",
		"sourceCode", input.SourceCode,
		"sourceSystem", input.SourceSystem,
		"targetSystem", input.TargetSystem,
	)

	// Apply defaults
	if input.AutoApproveThreshold == 0 {
		input.AutoApproveThreshold = AutoApproveThreshold
	}
	if input.ReviewTimeout == 0 {
		input.ReviewTimeout = 7 * 24 * time.Hour
	}
	if input.MaxCandidates == 0 {
		input.MaxCandidates = 5
	}

	// Configure activity options
	activityOpts := workflow.ActivityOptions{
		StartToCloseTimeout: 2 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    time.Second,
			BackoffCoefficient: 2.0,
			MaximumInterval:    time.Minute,
			MaximumAttempts:    3,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, activityOpts)

	var a *Activities

	// Step 1: Check if mapping already exists
	var existingCheck CheckExistingMappingOutput
	err := workflow.ExecuteActivity(ctx, a.CheckExistingMapping, CheckExistingMappingInput{
		SourceSystem: input.SourceSystem,
		SourceCode:   input.SourceCode,
		TargetSystem: input.TargetSystem,
		ProfileID:    input.ProfileID,
	}).Get(ctx, &existingCheck)
	if err != nil {
		return nil, workflow.NewContinueAsNewError(ctx, TerminologyReview, input)
	}

	if existingCheck.Exists {
		logger.Info("Mapping already exists", "mappingId", existingCheck.MappingID)
		return &TerminologyReviewOutput{
			Status:    "existing",
			MappingID: existingCheck.MappingID,
		}, nil
	}

	// Step 2: Run autoroute engine
	startTime := workflow.Now(ctx)
	var suggestion SuggestMappingOutput
	err = workflow.ExecuteActivity(ctx, a.SuggestMapping, SuggestMappingInput{
		SourceCode:    input.SourceCode,
		SourceSystem:  input.SourceSystem,
		SourceDisplay: input.SourceDisplay,
		TargetSystem:  input.TargetSystem,
		ProfileID:     input.ProfileID,
		MaxCandidates: input.MaxCandidates,
	}).Get(ctx, &suggestion)
	if err != nil {
		return nil, err
	}
	suggestionDuration := workflow.Now(ctx).Sub(startTime)

	// No candidates found
	if suggestion.BestMatch == nil {
		logger.Info("No mapping candidates found")

		// Record telemetry for no-match case
		_ = workflow.ExecuteActivity(ctx, a.RecordDecision, RecordDecisionInput{
			SourceCode:    input.SourceCode,
			SourceSystem:  input.SourceSystem,
			SourceDisplay: input.SourceDisplay,
			TargetSystem:  input.TargetSystem,
			DecisionType:  "NO_MATCH",
			ProfileID:     input.ProfileID,
			RequestSource: "workflow",
			DurationMs:    int(suggestionDuration.Milliseconds()),
		}).Get(ctx, nil)

		return &TerminologyReviewOutput{
			Status: "no_match",
		}, nil
	}

	// Step 3: Check for auto-approval (high confidence threshold)
	if suggestion.Confidence >= input.AutoApproveThreshold {
		logger.Info("Auto-approving high-confidence suggestion",
			"confidence", suggestion.Confidence,
			"threshold", input.AutoApproveThreshold,
		)

		// Create pending and immediately approve it
		var pending CreatePendingAutorouteOutput
		err = workflow.ExecuteActivity(ctx, a.CreatePendingAutoroute, CreatePendingAutorouteInput{
			SourceSystem:     input.SourceSystem,
			SourceCode:       input.SourceCode,
			SourceDisplay:    input.SourceDisplay,
			TargetSystem:     input.TargetSystem,
			SuggestedCode:    suggestion.BestMatch.Code,
			SuggestedDisplay: suggestion.BestMatch.Display,
			Confidence:       suggestion.Confidence,
			Equivalence:      suggestion.BestMatch.Equivalence,
			Reasoning:        suggestion.Reasoning,
			DecisionTrace:    suggestion.TraceJSON,
			Alternates:       marshalAlternates(suggestion.Alternates),
			TTL:              time.Hour, // Short TTL for auto-approved
		}).Get(ctx, &pending)
		if err != nil {
			return nil, err
		}

		var approved ApproveMappingOutput
		err = workflow.ExecuteActivity(ctx, a.ApproveMapping, ApproveMappingInput{
			PendingID:  pending.ID,
			ApprovedBy: "system:auto-approve",
		}).Get(ctx, &approved)
		if err != nil {
			return nil, err
		}

		// Record telemetry
		_ = workflow.ExecuteActivity(ctx, a.RecordDecision, RecordDecisionInput{
			SourceCode:      input.SourceCode,
			SourceSystem:    input.SourceSystem,
			SourceDisplay:   input.SourceDisplay,
			TargetSystem:    input.TargetSystem,
			DecisionType:    "AUTOROUTE_HIGH_CONF",
			Confidence:      suggestion.Confidence,
			SelectedCode:    suggestion.BestMatch.Code,
			SelectedDisplay: suggestion.BestMatch.Display,
			DecisionTree:    suggestion.TraceJSON,
			ProfileID:       input.ProfileID,
			RequestSource:   "workflow",
			DurationMs:      int(suggestionDuration.Milliseconds()),
		}).Get(ctx, nil)

		return &TerminologyReviewOutput{
			Status:           "auto_approved",
			MappingID:        approved.MappingID,
			FinalCode:        suggestion.BestMatch.Code,
			FinalDisplay:     suggestion.BestMatch.Display,
			FinalEquivalence: suggestion.BestMatch.Equivalence,
			Confidence:       suggestion.Confidence,
			DecidedBy:        "system:auto-approve",
			PendingID:        pending.ID,
		}, nil
	}

	// Step 4: Create pending autoroute for human review
	// Confidence is below auto-approve threshold
	decisionType := "AUTOROUTE_MED_CONF"
	if suggestion.Confidence < 0.70 {
		decisionType = "AUTOROUTE_LOW_CONF"
	}

	logger.Info("Creating pending autoroute for review",
		"confidence", suggestion.Confidence,
		"decisionType", decisionType,
	)

	var pending CreatePendingAutorouteOutput
	err = workflow.ExecuteActivity(ctx, a.CreatePendingAutoroute, CreatePendingAutorouteInput{
		SourceSystem:     input.SourceSystem,
		SourceCode:       input.SourceCode,
		SourceDisplay:    input.SourceDisplay,
		TargetSystem:     input.TargetSystem,
		SuggestedCode:    suggestion.BestMatch.Code,
		SuggestedDisplay: suggestion.BestMatch.Display,
		Confidence:       suggestion.Confidence,
		Equivalence:      suggestion.BestMatch.Equivalence,
		Reasoning:        suggestion.Reasoning,
		DecisionTrace:    suggestion.TraceJSON,
		Alternates:       marshalAlternates(suggestion.Alternates),
		TTL:              input.ReviewTimeout,
	}).Get(ctx, &pending)
	if err != nil {
		return nil, err
	}

	// Record the initial suggestion telemetry (before human review)
	_ = workflow.ExecuteActivity(ctx, a.RecordDecision, RecordDecisionInput{
		SourceCode:      input.SourceCode,
		SourceSystem:    input.SourceSystem,
		SourceDisplay:   input.SourceDisplay,
		TargetSystem:    input.TargetSystem,
		DecisionType:    decisionType,
		Confidence:      suggestion.Confidence,
		SelectedCode:    suggestion.BestMatch.Code,
		SelectedDisplay: suggestion.BestMatch.Display,
		DecisionTree:    suggestion.TraceJSON,
		ProfileID:       input.ProfileID,
		RequestSource:   "workflow",
		DurationMs:      int(suggestionDuration.Milliseconds()),
	}).Get(ctx, nil)

	// Step 5: Wait for human decision or timeout
	reviewStartTime := workflow.Now(ctx)
	var decision ReviewDecisionSignal
	signalChan := workflow.GetSignalChannel(ctx, SignalNameReviewDecision)

	selector := workflow.NewSelector(ctx)
	var signalReceived bool

	selector.AddReceive(signalChan, func(c workflow.ReceiveChannel, more bool) {
		c.Receive(ctx, &decision)
		signalReceived = true
	})

	// Set up timeout
	timerFuture := workflow.NewTimer(ctx, input.ReviewTimeout)
	selector.AddFuture(timerFuture, func(f workflow.Future) {
		// Timeout occurred - signalReceived remains false
	})

	selector.Select(ctx)
	reviewDuration := workflow.Now(ctx).Sub(reviewStartTime)

	// Step 6: Process decision
	if signalReceived {
		if decision.Approved {
			logger.Info("Processing approval",
				"decidedBy", decision.DecidedBy,
				"reviewDuration", reviewDuration,
			)

			var approved ApproveMappingOutput
			err = workflow.ExecuteActivity(ctx, a.ApproveMapping, ApproveMappingInput{
				PendingID:           pending.ID,
				ApprovedBy:          decision.DecidedBy,
				EquivalenceOverride: decision.EquivalenceOverride,
				Comment:             decision.Comment,
			}).Get(ctx, &approved)
			if err != nil {
				return nil, err
			}

			finalEquivalence := suggestion.BestMatch.Equivalence
			if decision.EquivalenceOverride != "" {
				finalEquivalence = decision.EquivalenceOverride
			}

			return &TerminologyReviewOutput{
				Status:           "approved",
				MappingID:        approved.MappingID,
				FinalCode:        suggestion.BestMatch.Code,
				FinalDisplay:     suggestion.BestMatch.Display,
				FinalEquivalence: finalEquivalence,
				Confidence:       suggestion.Confidence,
				DecidedBy:        decision.DecidedBy,
				ReviewTime:       reviewDuration,
				PendingID:        pending.ID,
			}, nil
		}

		// Rejected
		logger.Info("Processing rejection",
			"decidedBy", decision.DecidedBy,
			"reason", decision.RejectionReason,
		)

		err = workflow.ExecuteActivity(ctx, a.RejectMapping, RejectMappingInput{
			PendingID:  pending.ID,
			RejectedBy: decision.DecidedBy,
			Reason:     decision.RejectionReason,
		}).Get(ctx, nil)
		if err != nil {
			return nil, err
		}

		return &TerminologyReviewOutput{
			Status:     "rejected",
			DecidedBy:  decision.DecidedBy,
			ReviewTime: reviewDuration,
			PendingID:  pending.ID,
		}, nil
	}

	// Timeout - mark as expired (the pending autoroute will be expired by TTL)
	logger.Info("Review timeout reached", "timeout", input.ReviewTimeout)

	return &TerminologyReviewOutput{
		Status:     "expired",
		ReviewTime: reviewDuration,
		PendingID:  pending.ID,
	}, nil
}

// marshalAlternates converts alternates to JSON for storage.
func marshalAlternates(alternates []CandidateResult) json.RawMessage {
	if len(alternates) == 0 {
		return nil
	}
	data, _ := json.Marshal(alternates)
	return data
}
