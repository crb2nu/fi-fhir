package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"go.temporal.io/sdk/testsuite"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/terminology/autoroute"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/terminology/db"
)

// ---------------------------------------------------------------------------
// Mock implementations
// ---------------------------------------------------------------------------

type mockEngine struct {
	suggestFn func(ctx context.Context, req autoroute.SuggestRequest) (*autoroute.SuggestResult, error)
}

func (m *mockEngine) Suggest(ctx context.Context, req autoroute.SuggestRequest) (*autoroute.SuggestResult, error) {
	return m.suggestFn(ctx, req)
}

type mockStore struct {
	lookupFn         func(ctx context.Context, sourceSystem, sourceCode, targetSystem, profileID string) (*db.CustomMapping, error)
	createPendingFn  func(ctx context.Context, p *db.PendingAutoroute) error
	approvePendingFn func(ctx context.Context, id int64, approvedBy, equivalenceOverride, comment string) (*db.CustomMapping, error)
	rejectPendingFn  func(ctx context.Context, id int64, rejectedBy, reason string) error
	recordDecisionFn func(ctx context.Context, d *db.MappingDecision) error
}

func (m *mockStore) LookupMapping(ctx context.Context, sourceSystem, sourceCode, targetSystem, profileID string) (*db.CustomMapping, error) {
	return m.lookupFn(ctx, sourceSystem, sourceCode, targetSystem, profileID)
}

func (m *mockStore) CreatePendingAutoroute(ctx context.Context, p *db.PendingAutoroute) error {
	return m.createPendingFn(ctx, p)
}

func (m *mockStore) ApprovePendingAutoroute(ctx context.Context, id int64, approvedBy, equivalenceOverride, comment string) (*db.CustomMapping, error) {
	return m.approvePendingFn(ctx, id, approvedBy, equivalenceOverride, comment)
}

func (m *mockStore) RejectPendingAutoroute(ctx context.Context, id int64, rejectedBy, reason string) error {
	return m.rejectPendingFn(ctx, id, rejectedBy, reason)
}

func (m *mockStore) RecordMappingDecision(ctx context.Context, d *db.MappingDecision) error {
	return m.recordDecisionFn(ctx, d)
}

// execActivity runs a single activity function through the Temporal test
// activity environment, which provides proper activity.GetLogger(ctx) support.
func execActivity[I any, O any](t *testing.T, acts *Activities, fn func(context.Context, I) (O, error), input I) (O, error) {
	t.Helper()
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestActivityEnvironment()
	env.RegisterActivity(fn)

	encoded, err := env.ExecuteActivity(fn, input)
	if err != nil {
		var zero O
		return zero, err
	}

	var result O
	if err := encoded.Get(&result); err != nil {
		t.Fatalf("decode activity result: %v", err)
	}
	return result, nil
}

// execActivityNoOutput runs an activity that returns only error.
func execActivityNoOutput[I any](t *testing.T, acts *Activities, fn func(context.Context, I) error, input I) error {
	t.Helper()
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestActivityEnvironment()
	env.RegisterActivity(fn)

	_, err := env.ExecuteActivity(fn, input)
	return err
}

// ---------------------------------------------------------------------------
// CheckExistingMapping activity tests
// ---------------------------------------------------------------------------

func TestActivity_CheckExistingMapping_Found(t *testing.T) {
	store := &mockStore{
		lookupFn: func(_ context.Context, sourceSystem, sourceCode, targetSystem, profileID string) (*db.CustomMapping, error) {
			if sourceSystem != "epic" || sourceCode != "GLU" || targetSystem != "loinc" {
				t.Fatalf("unexpected params: %s/%s/%s", sourceSystem, sourceCode, targetSystem)
			}
			return &db.CustomMapping{ID: 99}, nil
		},
	}
	acts := NewActivities(nil, store)

	out, err := execActivity(t, acts, acts.CheckExistingMapping, CheckExistingMappingInput{
		SourceSystem: "epic",
		SourceCode:   "GLU",
		TargetSystem: "loinc",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !out.Exists {
		t.Fatal("expected Exists=true")
	}
	if out.MappingID != 99 {
		t.Fatalf("MappingID=%d want 99", out.MappingID)
	}
}

func TestActivity_CheckExistingMapping_NotFound(t *testing.T) {
	store := &mockStore{
		lookupFn: func(_ context.Context, _, _, _, _ string) (*db.CustomMapping, error) {
			return nil, nil
		},
	}
	acts := NewActivities(nil, store)

	out, err := execActivity(t, acts, acts.CheckExistingMapping, CheckExistingMappingInput{
		SourceSystem: "epic",
		SourceCode:   "MISSING",
		TargetSystem: "loinc",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Exists {
		t.Fatal("expected Exists=false")
	}
	if out.MappingID != 0 {
		t.Fatalf("MappingID=%d want 0", out.MappingID)
	}
}

func TestActivity_CheckExistingMapping_StoreError(t *testing.T) {
	store := &mockStore{
		lookupFn: func(_ context.Context, _, _, _, _ string) (*db.CustomMapping, error) {
			return nil, errors.New("db down")
		},
	}
	acts := NewActivities(nil, store)

	_, err := execActivity(t, acts, acts.CheckExistingMapping, CheckExistingMappingInput{
		SourceSystem: "epic",
		SourceCode:   "ERR",
		TargetSystem: "loinc",
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestActivity_CheckExistingMapping_WithProfileID(t *testing.T) {
	var receivedProfileID string
	store := &mockStore{
		lookupFn: func(_ context.Context, _, _, _, profileID string) (*db.CustomMapping, error) {
			receivedProfileID = profileID
			return nil, nil
		},
	}
	acts := NewActivities(nil, store)

	_, err := execActivity(t, acts, acts.CheckExistingMapping, CheckExistingMappingInput{
		SourceSystem: "epic",
		SourceCode:   "X",
		TargetSystem: "loinc",
		ProfileID:    "profile-123",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if receivedProfileID != "profile-123" {
		t.Fatalf("profileID=%q want profile-123", receivedProfileID)
	}
}

// ---------------------------------------------------------------------------
// SuggestMapping activity tests
// ---------------------------------------------------------------------------

func TestActivity_SuggestMapping_WithBestMatch(t *testing.T) {
	engine := &mockEngine{
		suggestFn: func(_ context.Context, req autoroute.SuggestRequest) (*autoroute.SuggestResult, error) {
			if req.SourceCode != "GLU001" {
				t.Fatalf("SourceCode=%q want GLU001", req.SourceCode)
			}
			return &autoroute.SuggestResult{
				BestMatch: &autoroute.Candidate{
					Code:        "2345-7",
					Display:     "Glucose [Mass/volume] in Serum",
					System:      "http://loinc.org",
					Confidence:  0.95,
					Equivalence: "equivalent",
					Reasoning:   "exact semantic match",
					Score:       0.95,
				},
				Alternates: []autoroute.Candidate{
					{Code: "2339-0", Display: "Glucose [Mass]", System: "http://loinc.org", Confidence: 0.80, Equivalence: "wider"},
				},
				Confidence:    0.95,
				Reasoning:     "high confidence",
				Model:         "gpt-4",
				TotalDuration: 250 * time.Millisecond,
			}, nil
		},
	}
	acts := NewActivities(engine, nil)

	out, err := execActivity(t, acts, acts.SuggestMapping, SuggestMappingInput{
		SourceCode:   "GLU001",
		SourceSystem: "epic_labs",
		TargetSystem: "http://loinc.org",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.BestMatch == nil {
		t.Fatal("expected non-nil BestMatch")
	}
	if out.BestMatch.Code != "2345-7" {
		t.Fatalf("BestMatch.Code=%q want 2345-7", out.BestMatch.Code)
	}
	if out.Confidence != 0.95 {
		t.Fatalf("Confidence=%f want 0.95", out.Confidence)
	}
	if out.Model != "gpt-4" {
		t.Fatalf("Model=%q want gpt-4", out.Model)
	}
	if out.DurationMs != 250 {
		t.Fatalf("DurationMs=%d want 250", out.DurationMs)
	}
	if len(out.Alternates) != 1 {
		t.Fatalf("len(Alternates)=%d want 1", len(out.Alternates))
	}
	if out.Alternates[0].Code != "2339-0" {
		t.Fatalf("Alternates[0].Code=%q want 2339-0", out.Alternates[0].Code)
	}
}

func TestActivity_SuggestMapping_NoMatch(t *testing.T) {
	engine := &mockEngine{
		suggestFn: func(_ context.Context, _ autoroute.SuggestRequest) (*autoroute.SuggestResult, error) {
			return &autoroute.SuggestResult{
				BestMatch:     nil,
				Confidence:    0.0,
				TotalDuration: 100 * time.Millisecond,
			}, nil
		},
	}
	acts := NewActivities(engine, nil)

	out, err := execActivity(t, acts, acts.SuggestMapping, SuggestMappingInput{
		SourceCode:   "UNKNOWN",
		SourceSystem: "epic_labs",
		TargetSystem: "http://loinc.org",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.BestMatch != nil {
		t.Fatal("expected nil BestMatch")
	}
	if out.Confidence != 0.0 {
		t.Fatalf("Confidence=%f want 0.0", out.Confidence)
	}
}

func TestActivity_SuggestMapping_WithTrace(t *testing.T) {
	engine := &mockEngine{
		suggestFn: func(_ context.Context, _ autoroute.SuggestRequest) (*autoroute.SuggestResult, error) {
			return &autoroute.SuggestResult{
				BestMatch: &autoroute.Candidate{
					Code: "789-8", Display: "Erythrocytes", System: "http://loinc.org",
					Confidence: 0.92, Equivalence: "equivalent",
				},
				Confidence:    0.92,
				TotalDuration: 200 * time.Millisecond,
				Trace: &autoroute.DecisionTrace{
					TraceID: "trace-001",
				},
			}, nil
		},
	}
	acts := NewActivities(engine, nil)

	out, err := execActivity(t, acts, acts.SuggestMapping, SuggestMappingInput{
		SourceCode:   "RBC",
		SourceSystem: "epic_labs",
		TargetSystem: "http://loinc.org",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.TraceJSON == nil {
		t.Fatal("expected non-nil TraceJSON")
	}
	// Verify trace is valid JSON
	var traceMap map[string]interface{}
	if err := json.Unmarshal(out.TraceJSON, &traceMap); err != nil {
		t.Fatalf("TraceJSON is not valid JSON: %v", err)
	}
}

func TestActivity_SuggestMapping_EngineError(t *testing.T) {
	engine := &mockEngine{
		suggestFn: func(_ context.Context, _ autoroute.SuggestRequest) (*autoroute.SuggestResult, error) {
			return nil, errors.New("LLM timeout")
		},
	}
	acts := NewActivities(engine, nil)

	_, err := execActivity(t, acts, acts.SuggestMapping, SuggestMappingInput{
		SourceCode:   "ERR",
		SourceSystem: "epic_labs",
		TargetSystem: "http://loinc.org",
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestActivity_SuggestMapping_PropagatesInputFields(t *testing.T) {
	var receivedReq autoroute.SuggestRequest
	engine := &mockEngine{
		suggestFn: func(_ context.Context, req autoroute.SuggestRequest) (*autoroute.SuggestResult, error) {
			receivedReq = req
			return &autoroute.SuggestResult{TotalDuration: time.Millisecond}, nil
		},
	}
	acts := NewActivities(engine, nil)

	_, err := execActivity(t, acts, acts.SuggestMapping, SuggestMappingInput{
		SourceCode:    "ABC",
		SourceSystem:  "epic",
		SourceDisplay: "Test Display",
		TargetSystem:  "http://loinc.org",
		ProfileID:     "prof-1",
		MaxCandidates: 3,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if receivedReq.SourceDisplay != "Test Display" {
		t.Fatalf("SourceDisplay=%q want 'Test Display'", receivedReq.SourceDisplay)
	}
	if receivedReq.ProfileID != "prof-1" {
		t.Fatalf("ProfileID=%q want prof-1", receivedReq.ProfileID)
	}
	if receivedReq.MaxCandidates != 3 {
		t.Fatalf("MaxCandidates=%d want 3", receivedReq.MaxCandidates)
	}
}

// ---------------------------------------------------------------------------
// CreatePendingAutoroute activity tests
// ---------------------------------------------------------------------------

func TestActivity_CreatePendingAutoroute_Success(t *testing.T) {
	store := &mockStore{
		createPendingFn: func(_ context.Context, p *db.PendingAutoroute) error {
			if p.SourceCode != "GLU" {
				t.Fatalf("SourceCode=%q want GLU", p.SourceCode)
			}
			if p.Status != db.StatusPending {
				t.Fatalf("Status=%q want pending", p.Status)
			}
			if p.ExpiresAt == nil {
				t.Fatal("expected non-nil ExpiresAt")
			}
			p.ID = 42
			p.CreatedAt = time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC)
			return nil
		},
	}
	acts := NewActivities(nil, store)

	out, err := execActivity(t, acts, acts.CreatePendingAutoroute, CreatePendingAutorouteInput{
		SourceSystem:     "epic",
		SourceCode:       "GLU",
		TargetSystem:     "loinc",
		SuggestedCode:    "2345-7",
		SuggestedDisplay: "Glucose",
		Confidence:       0.88,
		Equivalence:      "equivalent",
		Reasoning:        "semantic match",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.ID != 42 {
		t.Fatalf("ID=%d want 42", out.ID)
	}
}

func TestActivity_CreatePendingAutoroute_DefaultTTL(t *testing.T) {
	var receivedExpiry time.Time
	store := &mockStore{
		createPendingFn: func(_ context.Context, p *db.PendingAutoroute) error {
			receivedExpiry = *p.ExpiresAt
			p.ID = 1
			return nil
		},
	}
	acts := NewActivities(nil, store)

	_, err := execActivity(t, acts, acts.CreatePendingAutoroute, CreatePendingAutorouteInput{
		SourceSystem:  "epic",
		SourceCode:    "X",
		TargetSystem:  "loinc",
		SuggestedCode: "Y",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Default TTL is 7 days
	expectedMin := time.Now().Add(6*24*time.Hour + 23*time.Hour)
	if receivedExpiry.Before(expectedMin) {
		t.Fatalf("ExpiresAt=%v is less than ~7 days from now", receivedExpiry)
	}
}

func TestActivity_CreatePendingAutoroute_CustomTTL(t *testing.T) {
	var receivedExpiry time.Time
	store := &mockStore{
		createPendingFn: func(_ context.Context, p *db.PendingAutoroute) error {
			receivedExpiry = *p.ExpiresAt
			p.ID = 1
			return nil
		},
	}
	acts := NewActivities(nil, store)

	_, err := execActivity(t, acts, acts.CreatePendingAutoroute, CreatePendingAutorouteInput{
		SourceSystem:  "epic",
		SourceCode:    "X",
		TargetSystem:  "loinc",
		SuggestedCode: "Y",
		TTL:           2 * time.Hour,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Custom TTL of 2 hours
	expectedMax := time.Now().Add(2*time.Hour + time.Minute)
	if receivedExpiry.After(expectedMax) {
		t.Fatalf("ExpiresAt=%v exceeds 2h TTL", receivedExpiry)
	}
}

func TestActivity_CreatePendingAutoroute_StoreError(t *testing.T) {
	store := &mockStore{
		createPendingFn: func(_ context.Context, _ *db.PendingAutoroute) error {
			return errors.New("duplicate key")
		},
	}
	acts := NewActivities(nil, store)

	_, err := execActivity(t, acts, acts.CreatePendingAutoroute, CreatePendingAutorouteInput{
		SourceSystem:  "epic",
		SourceCode:    "DUP",
		TargetSystem:  "loinc",
		SuggestedCode: "Y",
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

// ---------------------------------------------------------------------------
// ApproveMapping activity tests
// ---------------------------------------------------------------------------

func TestActivity_ApproveMapping_Success(t *testing.T) {
	store := &mockStore{
		approvePendingFn: func(_ context.Context, id int64, approvedBy, equivalenceOverride, comment string) (*db.CustomMapping, error) {
			if id != 10 {
				t.Fatalf("id=%d want 10", id)
			}
			if approvedBy != "reviewer@test.com" {
				t.Fatalf("approvedBy=%q want reviewer@test.com", approvedBy)
			}
			return &db.CustomMapping{
				ID:        55,
				CreatedAt: time.Date(2026, 2, 28, 12, 0, 0, 0, time.UTC),
			}, nil
		},
	}
	acts := NewActivities(nil, store)

	out, err := execActivity(t, acts, acts.ApproveMapping, ApproveMappingInput{
		PendingID:  10,
		ApprovedBy: "reviewer@test.com",
		Comment:    "looks good",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.MappingID != 55 {
		t.Fatalf("MappingID=%d want 55", out.MappingID)
	}
}

func TestActivity_ApproveMapping_WithOverride(t *testing.T) {
	var receivedOverride string
	store := &mockStore{
		approvePendingFn: func(_ context.Context, _ int64, _, equivalenceOverride, _ string) (*db.CustomMapping, error) {
			receivedOverride = equivalenceOverride
			return &db.CustomMapping{ID: 1}, nil
		},
	}
	acts := NewActivities(nil, store)

	_, err := execActivity(t, acts, acts.ApproveMapping, ApproveMappingInput{
		PendingID:           5,
		ApprovedBy:          "user",
		EquivalenceOverride: "wider",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if receivedOverride != "wider" {
		t.Fatalf("equivalenceOverride=%q want wider", receivedOverride)
	}
}

func TestActivity_ApproveMapping_StoreError(t *testing.T) {
	store := &mockStore{
		approvePendingFn: func(_ context.Context, _ int64, _, _, _ string) (*db.CustomMapping, error) {
			return nil, errors.New("not found")
		},
	}
	acts := NewActivities(nil, store)

	_, err := execActivity(t, acts, acts.ApproveMapping, ApproveMappingInput{
		PendingID:  999,
		ApprovedBy: "user",
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

// ---------------------------------------------------------------------------
// RejectMapping activity tests
// ---------------------------------------------------------------------------

func TestActivity_RejectMapping_Success(t *testing.T) {
	var receivedID int64
	var receivedBy, receivedReason string
	store := &mockStore{
		rejectPendingFn: func(_ context.Context, id int64, rejectedBy, reason string) error {
			receivedID = id
			receivedBy = rejectedBy
			receivedReason = reason
			return nil
		},
	}
	acts := NewActivities(nil, store)

	err := execActivityNoOutput(t, acts, acts.RejectMapping, RejectMappingInput{
		PendingID:  15,
		RejectedBy: "admin@test.com",
		Reason:     "incorrect code",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if receivedID != 15 {
		t.Fatalf("id=%d want 15", receivedID)
	}
	if receivedBy != "admin@test.com" {
		t.Fatalf("rejectedBy=%q want admin@test.com", receivedBy)
	}
	if receivedReason != "incorrect code" {
		t.Fatalf("reason=%q want 'incorrect code'", receivedReason)
	}
}

func TestActivity_RejectMapping_StoreError(t *testing.T) {
	store := &mockStore{
		rejectPendingFn: func(_ context.Context, _ int64, _, _ string) error {
			return errors.New("already reviewed")
		},
	}
	acts := NewActivities(nil, store)

	err := execActivityNoOutput(t, acts, acts.RejectMapping, RejectMappingInput{
		PendingID:  1,
		RejectedBy: "user",
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

// ---------------------------------------------------------------------------
// RecordDecision activity tests
// ---------------------------------------------------------------------------

func TestActivity_RecordDecision_FullInput(t *testing.T) {
	var recorded *db.MappingDecision
	store := &mockStore{
		recordDecisionFn: func(_ context.Context, d *db.MappingDecision) error {
			recorded = d
			d.ID = 100
			return nil
		},
	}
	acts := NewActivities(nil, store)

	err := execActivityNoOutput(t, acts, acts.RecordDecision, RecordDecisionInput{
		TraceID:         "trace-xyz",
		SourceCode:      "GLU",
		SourceSystem:    "epic",
		SourceDisplay:   "Glucose",
		TargetSystem:    "loinc",
		DecisionType:    "AUTOROUTE_HIGH_CONF",
		Confidence:      0.98,
		SelectedCode:    "2345-7",
		SelectedDisplay: "Glucose [Mass/volume]",
		ProfileID:       "prof-1",
		RequestSource:   "graphql",
		DurationMs:      150,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if recorded.TraceID != "trace-xyz" {
		t.Fatalf("TraceID=%q want trace-xyz", recorded.TraceID)
	}
	if recorded.RequestSource != "graphql" {
		t.Fatalf("RequestSource=%q want graphql", recorded.RequestSource)
	}
	if recorded.Confidence == nil || *recorded.Confidence != 0.98 {
		t.Fatalf("Confidence=%v want 0.98", recorded.Confidence)
	}
}

func TestActivity_RecordDecision_DefaultTraceID(t *testing.T) {
	var recorded *db.MappingDecision
	store := &mockStore{
		recordDecisionFn: func(_ context.Context, d *db.MappingDecision) error {
			recorded = d
			return nil
		},
	}
	acts := NewActivities(nil, store)

	err := execActivityNoOutput(t, acts, acts.RecordDecision, RecordDecisionInput{
		SourceCode:   "X",
		SourceSystem: "epic",
		TargetSystem: "loinc",
		DecisionType: "NO_MATCH",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if recorded.TraceID == "" {
		t.Fatal("expected auto-generated TraceID")
	}
	if len(recorded.TraceID) < 4 {
		t.Fatalf("TraceID=%q seems too short", recorded.TraceID)
	}
}

func TestActivity_RecordDecision_DefaultRequestSource(t *testing.T) {
	var recorded *db.MappingDecision
	store := &mockStore{
		recordDecisionFn: func(_ context.Context, d *db.MappingDecision) error {
			recorded = d
			return nil
		},
	}
	acts := NewActivities(nil, store)

	err := execActivityNoOutput(t, acts, acts.RecordDecision, RecordDecisionInput{
		SourceCode:   "X",
		SourceSystem: "epic",
		TargetSystem: "loinc",
		DecisionType: "NO_MATCH",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if recorded.RequestSource != "workflow" {
		t.Fatalf("RequestSource=%q want 'workflow' (default)", recorded.RequestSource)
	}
}

func TestActivity_RecordDecision_ZeroConfidence(t *testing.T) {
	var recorded *db.MappingDecision
	store := &mockStore{
		recordDecisionFn: func(_ context.Context, d *db.MappingDecision) error {
			recorded = d
			return nil
		},
	}
	acts := NewActivities(nil, store)

	err := execActivityNoOutput(t, acts, acts.RecordDecision, RecordDecisionInput{
		SourceCode:   "X",
		SourceSystem: "epic",
		TargetSystem: "loinc",
		DecisionType: "NO_MATCH",
		Confidence:   0.0,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if recorded.Confidence != nil {
		t.Fatalf("Confidence=%v want nil for zero input", recorded.Confidence)
	}
}

func TestActivity_RecordDecision_StoreError(t *testing.T) {
	store := &mockStore{
		recordDecisionFn: func(_ context.Context, _ *db.MappingDecision) error {
			return errors.New("write failed")
		},
	}
	acts := NewActivities(nil, store)

	err := execActivityNoOutput(t, acts, acts.RecordDecision, RecordDecisionInput{
		SourceCode:   "X",
		SourceSystem: "epic",
		TargetSystem: "loinc",
		DecisionType: "NO_MATCH",
	})
	if err == nil {
		t.Fatal("expected error")
	}
}
