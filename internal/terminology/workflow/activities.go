// Package workflow provides Temporal workflows for terminology mapping review.
//
// This package wraps the existing autoroute engine and database operations
// as Temporal activities, enabling durable, auditable workflow orchestration
// for the terminology mapping review process.
package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.temporal.io/sdk/activity"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/terminology/autoroute"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/terminology/db"
)

// suggestEngine abstracts the autoroute engine for testability.
type suggestEngine interface {
	Suggest(ctx context.Context, req autoroute.SuggestRequest) (*autoroute.SuggestResult, error)
}

// mappingRepository abstracts the mapping store for testability.
type mappingRepository interface {
	LookupMapping(ctx context.Context, sourceSystem, sourceCode, targetSystem, profileID string) (*db.CustomMapping, error)
	CreatePendingAutoroute(ctx context.Context, p *db.PendingAutoroute) error
	ApprovePendingAutoroute(ctx context.Context, id int64, approvedBy, equivalenceOverride, comment string) (*db.CustomMapping, error)
	RejectPendingAutoroute(ctx context.Context, id int64, rejectedBy, reason string) error
	RecordMappingDecision(ctx context.Context, d *db.MappingDecision) error
}

// Activities holds the dependencies needed by activity implementations.
type Activities struct {
	engine suggestEngine
	store  mappingRepository
}

// NewActivities creates a new Activities instance with the required dependencies.
func NewActivities(engine suggestEngine, store mappingRepository) *Activities {
	return &Activities{
		engine: engine,
		store:  store,
	}
}

// SuggestMappingInput contains the input for the SuggestMapping activity.
type SuggestMappingInput struct {
	SourceCode    string `json:"source_code"`
	SourceSystem  string `json:"source_system"`
	SourceDisplay string `json:"source_display,omitempty"`
	TargetSystem  string `json:"target_system"`
	ProfileID     string `json:"profile_id,omitempty"`
	MaxCandidates int    `json:"max_candidates,omitempty"`
}

// SuggestMappingOutput contains the result of the SuggestMapping activity.
type SuggestMappingOutput struct {
	BestMatch    *CandidateResult       `json:"best_match,omitempty"`
	Alternates   []CandidateResult      `json:"alternates,omitempty"`
	Confidence   float64                `json:"confidence"`
	Reasoning    string                 `json:"reasoning"`
	Model        string                 `json:"model,omitempty"`
	TraceJSON    json.RawMessage        `json:"trace_json,omitempty"`
	DurationMs   int64                  `json:"duration_ms"`
	DecisionType autoroute.DecisionType `json:"decision_type"`
}

// CandidateResult is a serializable representation of a mapping candidate.
type CandidateResult struct {
	Code        string  `json:"code"`
	Display     string  `json:"display"`
	System      string  `json:"system"`
	Confidence  float64 `json:"confidence"`
	Equivalence string  `json:"equivalence"`
	Reasoning   string  `json:"reasoning,omitempty"`
	Score       float64 `json:"score"`
}

// SuggestMapping runs the autoroute engine to find candidate mappings.
// This activity is idempotent for the same input within a short time window
// due to caching in the autoroute engine.
func (a *Activities) SuggestMapping(ctx context.Context, input SuggestMappingInput) (*SuggestMappingOutput, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("Starting SuggestMapping activity",
		"sourceCode", input.SourceCode,
		"sourceSystem", input.SourceSystem,
		"targetSystem", input.TargetSystem,
	)

	req := autoroute.SuggestRequest{
		SourceCode:    input.SourceCode,
		SourceSystem:  input.SourceSystem,
		SourceDisplay: input.SourceDisplay,
		TargetSystem:  input.TargetSystem,
		ProfileID:     input.ProfileID,
		MaxCandidates: input.MaxCandidates,
	}

	result, err := a.engine.Suggest(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("autoroute suggestion failed: %w", err)
	}

	// Convert to output format
	output := &SuggestMappingOutput{
		Confidence:   result.Confidence,
		Reasoning:    result.Reasoning,
		Model:        result.Model,
		DurationMs:   result.TotalDuration.Milliseconds(),
		DecisionType: result.Classify(0.90, 0.70),
	}

	// Convert best match
	if result.BestMatch != nil {
		output.BestMatch = &CandidateResult{
			Code:        result.BestMatch.Code,
			Display:     result.BestMatch.Display,
			System:      result.BestMatch.System,
			Confidence:  result.BestMatch.Confidence,
			Equivalence: string(result.BestMatch.Equivalence),
			Reasoning:   result.BestMatch.Reasoning,
			Score:       result.BestMatch.Score,
		}
	}

	// Convert alternates
	for _, alt := range result.Alternates {
		output.Alternates = append(output.Alternates, CandidateResult{
			Code:        alt.Code,
			Display:     alt.Display,
			System:      alt.System,
			Confidence:  alt.Confidence,
			Equivalence: string(alt.Equivalence),
			Reasoning:   alt.Reasoning,
			Score:       alt.Score,
		})
	}

	// Serialize trace for storage
	if result.Trace != nil {
		traceBytes, err := json.Marshal(result.Trace)
		if err == nil {
			output.TraceJSON = traceBytes
		}
	}

	logger.Info("SuggestMapping activity completed",
		"confidence", output.Confidence,
		"decisionType", output.DecisionType,
		"hasBestMatch", output.BestMatch != nil,
	)

	return output, nil
}

// CreatePendingAutorouteInput contains the input for creating a pending autoroute.
type CreatePendingAutorouteInput struct {
	SourceSystem     string          `json:"source_system"`
	SourceCode       string          `json:"source_code"`
	SourceDisplay    string          `json:"source_display,omitempty"`
	TargetSystem     string          `json:"target_system"`
	SuggestedCode    string          `json:"suggested_code"`
	SuggestedDisplay string          `json:"suggested_display,omitempty"`
	Confidence       float64         `json:"confidence"`
	Equivalence      string          `json:"equivalence,omitempty"`
	Reasoning        string          `json:"reasoning,omitempty"`
	DecisionTrace    json.RawMessage `json:"decision_trace,omitempty"`
	Alternates       json.RawMessage `json:"alternates,omitempty"`
	TTL              time.Duration   `json:"ttl,omitempty"` // How long to wait for review before expiring
}

// CreatePendingAutorouteOutput contains the result of creating a pending autoroute.
type CreatePendingAutorouteOutput struct {
	ID        int64     `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// CreatePendingAutoroute stores a mapping suggestion for human review.
func (a *Activities) CreatePendingAutoroute(ctx context.Context, input CreatePendingAutorouteInput) (*CreatePendingAutorouteOutput, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("Creating pending autoroute",
		"sourceCode", input.SourceCode,
		"suggestedCode", input.SuggestedCode,
		"confidence", input.Confidence,
	)

	// Default TTL of 7 days
	ttl := input.TTL
	if ttl == 0 {
		ttl = 7 * 24 * time.Hour
	}
	expiresAt := time.Now().Add(ttl)

	pending := &db.PendingAutoroute{
		SourceSystem:     input.SourceSystem,
		SourceCode:       input.SourceCode,
		SourceDisplay:    input.SourceDisplay,
		TargetSystem:     input.TargetSystem,
		SuggestedCode:    input.SuggestedCode,
		SuggestedDisplay: input.SuggestedDisplay,
		Confidence:       input.Confidence,
		Equivalence:      input.Equivalence,
		Reasoning:        input.Reasoning,
		DecisionTrace:    input.DecisionTrace,
		Alternates:       input.Alternates,
		Status:           db.StatusPending,
		ExpiresAt:        &expiresAt,
	}

	if err := a.store.CreatePendingAutoroute(ctx, pending); err != nil {
		return nil, fmt.Errorf("failed to create pending autoroute: %w", err)
	}

	logger.Info("Pending autoroute created",
		"id", pending.ID,
		"expiresAt", expiresAt,
	)

	return &CreatePendingAutorouteOutput{
		ID:        pending.ID,
		CreatedAt: pending.CreatedAt,
		ExpiresAt: expiresAt,
	}, nil
}

// ApproveMappingInput contains the input for approving a pending autoroute.
type ApproveMappingInput struct {
	PendingID           int64  `json:"pending_id"`
	ApprovedBy          string `json:"approved_by"`
	EquivalenceOverride string `json:"equivalence_override,omitempty"`
	Comment             string `json:"comment,omitempty"`
}

// ApproveMappingOutput contains the result of approving a mapping.
type ApproveMappingOutput struct {
	MappingID int64     `json:"mapping_id"`
	CreatedAt time.Time `json:"created_at"`
}

// ApproveMapping converts a pending autoroute into a persistent mapping.
func (a *Activities) ApproveMapping(ctx context.Context, input ApproveMappingInput) (*ApproveMappingOutput, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("Approving pending autoroute",
		"pendingId", input.PendingID,
		"approvedBy", input.ApprovedBy,
	)

	mapping, err := a.store.ApprovePendingAutoroute(
		ctx,
		input.PendingID,
		input.ApprovedBy,
		input.EquivalenceOverride,
		input.Comment,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to approve pending autoroute: %w", err)
	}

	logger.Info("Pending autoroute approved",
		"mappingId", mapping.ID,
	)

	return &ApproveMappingOutput{
		MappingID: mapping.ID,
		CreatedAt: mapping.CreatedAt,
	}, nil
}

// RejectMappingInput contains the input for rejecting a pending autoroute.
type RejectMappingInput struct {
	PendingID  int64  `json:"pending_id"`
	RejectedBy string `json:"rejected_by"`
	Reason     string `json:"reason,omitempty"`
}

// RejectMapping marks a pending autoroute as rejected.
func (a *Activities) RejectMapping(ctx context.Context, input RejectMappingInput) error {
	logger := activity.GetLogger(ctx)
	logger.Info("Rejecting pending autoroute",
		"pendingId", input.PendingID,
		"rejectedBy", input.RejectedBy,
		"reason", input.Reason,
	)

	if err := a.store.RejectPendingAutoroute(
		ctx,
		input.PendingID,
		input.RejectedBy,
		input.Reason,
	); err != nil {
		return fmt.Errorf("failed to reject pending autoroute: %w", err)
	}

	logger.Info("Pending autoroute rejected", "pendingId", input.PendingID)
	return nil
}

// RecordDecisionInput contains the input for recording a mapping decision.
type RecordDecisionInput struct {
	TraceID         string          `json:"trace_id,omitempty"` // Unique trace ID for correlation
	SourceCode      string          `json:"source_code"`
	SourceSystem    string          `json:"source_system"`
	SourceDisplay   string          `json:"source_display,omitempty"`
	TargetSystem    string          `json:"target_system"`
	DecisionType    string          `json:"decision_type"` // APPROVE, REJECT, AUTO_APPROVE, EXPIRE, NO_MATCH
	Confidence      float64         `json:"confidence,omitempty"`
	SelectedCode    string          `json:"selected_code,omitempty"`
	SelectedDisplay string          `json:"selected_display,omitempty"`
	DecisionTree    json.RawMessage `json:"decision_tree,omitempty"` // Full decision trace
	ProfileID       string          `json:"profile_id,omitempty"`
	RequestSource   string          `json:"request_source,omitempty"` // "workflow", "graphql", "cli", "batch"
	DurationMs      int             `json:"duration_ms,omitempty"`
}

// RecordDecision records a mapping decision for telemetry and model improvement.
func (a *Activities) RecordDecision(ctx context.Context, input RecordDecisionInput) error {
	logger := activity.GetLogger(ctx)
	logger.Info("Recording mapping decision",
		"decisionType", input.DecisionType,
		"sourceCode", input.SourceCode,
		"requestSource", input.RequestSource,
	)

	// Generate trace ID if not provided
	traceID := input.TraceID
	if traceID == "" {
		traceID = fmt.Sprintf("wf-%d", time.Now().UnixNano())
	}

	// Request source defaults to "workflow"
	requestSource := input.RequestSource
	if requestSource == "" {
		requestSource = "workflow"
	}

	var confidencePtr *float64
	if input.Confidence > 0 {
		confidencePtr = &input.Confidence
	}

	decision := &db.MappingDecision{
		TraceID:         traceID,
		SourceCode:      input.SourceCode,
		SourceSystem:    input.SourceSystem,
		SourceDisplay:   input.SourceDisplay,
		TargetSystem:    input.TargetSystem,
		DecisionType:    db.DecisionType(input.DecisionType),
		Confidence:      confidencePtr,
		SelectedCode:    input.SelectedCode,
		SelectedDisplay: input.SelectedDisplay,
		DecisionTree:    input.DecisionTree,
		ProfileID:       input.ProfileID,
		RequestSource:   requestSource,
		DurationMs:      input.DurationMs,
	}

	if err := a.store.RecordMappingDecision(ctx, decision); err != nil {
		return fmt.Errorf("failed to record decision: %w", err)
	}

	logger.Info("Mapping decision recorded", "id", decision.ID, "traceId", traceID)
	return nil
}

// CheckExistingMappingInput contains the input for checking if a mapping exists.
type CheckExistingMappingInput struct {
	SourceSystem string `json:"source_system"`
	SourceCode   string `json:"source_code"`
	TargetSystem string `json:"target_system"`
	ProfileID    string `json:"profile_id,omitempty"`
}

// CheckExistingMappingOutput contains the result of the check.
type CheckExistingMappingOutput struct {
	Exists    bool  `json:"exists"`
	MappingID int64 `json:"mapping_id,omitempty"`
}

// CheckExistingMapping checks if a mapping already exists for the given source/target.
func (a *Activities) CheckExistingMapping(ctx context.Context, input CheckExistingMappingInput) (*CheckExistingMappingOutput, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("Checking for existing mapping",
		"sourceCode", input.SourceCode,
		"sourceSystem", input.SourceSystem,
		"targetSystem", input.TargetSystem,
	)

	mapping, err := a.store.LookupMapping(
		ctx,
		input.SourceSystem,
		input.SourceCode,
		input.TargetSystem,
		input.ProfileID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to lookup mapping: %w", err)
	}

	output := &CheckExistingMappingOutput{
		Exists: mapping != nil,
	}
	if mapping != nil {
		output.MappingID = mapping.ID
	}

	logger.Info("Existing mapping check complete", "exists", output.Exists)
	return output, nil
}

// AutoApproveThreshold is the confidence threshold for automatic approval.
const AutoApproveThreshold = 0.95
