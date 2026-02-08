//go:build integration

package db

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

// =============================================================================
// Approval Workflow End-to-End Integration Tests
// =============================================================================

func TestApprovalWorkflow_FullLifecycle(t *testing.T) {
	store, ctx := initStoreForTest(t)

	// Step 1: Create a pending autoroute suggestion
	decisionTrace := json.RawMessage(`{"steps":["cache_miss","semantic_search","autoroute"],"model":"gpt-4"}`)
	pending := &PendingAutoroute{
		SourceSystem:     "epic_labs",
		SourceCode:       "WF001",
		SourceDisplay:    "Hemoglobin A1c",
		TargetSystem:     "http://loinc.org",
		SuggestedCode:    "4548-4",
		SuggestedDisplay: "Hemoglobin A1c/Hemoglobin.total in Blood",
		Confidence:       0.94,
		Equivalence:      "equivalent",
		Reasoning:        "High semantic similarity for HbA1c lab test",
		DecisionTrace:    decisionTrace,
	}

	err := store.CreatePendingAutoroute(ctx, pending)
	if err != nil {
		t.Fatalf("Step 1 - Create pending failed: %v", err)
	}

	// Verify it shows up in pending list
	results, total, err := store.ListPendingAutoroutes(ctx, ListPendingAutoroutesFilter{Status: StatusPending})
	if err != nil {
		t.Fatalf("Step 1 - List pending failed: %v", err)
	}
	if total != 1 {
		t.Fatalf("Step 1 - Expected 1 pending, got %d", total)
	}
	if results[0].ID != pending.ID {
		t.Errorf("Step 1 - Pending ID mismatch")
	}

	// Step 2: Approve the pending autoroute
	mapping, err := store.ApprovePendingAutoroute(ctx, pending.ID, "dr.smith@hospital.org", "", "Verified mapping for HbA1c")
	if err != nil {
		t.Fatalf("Step 2 - Approve failed: %v", err)
	}

	// Step 3: Verify persistent mapping exists and has correct fields
	found, err := store.LookupMapping(ctx, "epic_labs", "WF001", "http://loinc.org", "")
	if err != nil {
		t.Fatalf("Step 3 - Lookup failed: %v", err)
	}
	if found == nil {
		t.Fatal("Step 3 - Expected mapping to exist")
	}
	if found.ID != mapping.ID {
		t.Errorf("Step 3 - ID mismatch: lookup=%d, approve=%d", found.ID, mapping.ID)
	}
	if found.Origin != OriginApprovedAutoroute {
		t.Errorf("Step 3 - Origin = %s, want approved_autoroute", found.Origin)
	}
	if found.TargetCode != "4548-4" {
		t.Errorf("Step 3 - TargetCode = %s, want 4548-4", found.TargetCode)
	}

	// Step 4: Record a decision telemetry entry for this approval
	conf := 0.94
	decision := &MappingDecision{
		TraceID:         "wf-trace-001",
		SourceSystem:    "epic_labs",
		SourceCode:      "WF001",
		TargetSystem:    "http://loinc.org",
		DecisionType:    DecisionAutorouteHighConf,
		Confidence:      &conf,
		SelectedCode:    "4548-4",
		SelectedDisplay: "Hemoglobin A1c/Hemoglobin.total in Blood",
		DecisionTree:    decisionTrace,
		RequestSource:   "cli",
		DurationMs:      200,
	}
	err = store.RecordMappingDecision(ctx, decision)
	if err != nil {
		t.Fatalf("Step 4 - Record decision failed: %v", err)
	}

	// Step 5: Verify telemetry was recorded
	gotDecision, err := store.GetMappingDecision(ctx, decision.ID)
	if err != nil {
		t.Fatalf("Step 5 - Get decision failed: %v", err)
	}
	if gotDecision.DecisionType != DecisionAutorouteHighConf {
		t.Errorf("Step 5 - DecisionType = %s, want AUTOROUTE_HIGH_CONF", gotDecision.DecisionType)
	}
	if gotDecision.SelectedCode != "4548-4" {
		t.Errorf("Step 5 - SelectedCode = %s, want 4548-4", gotDecision.SelectedCode)
	}

	// Verify pending count is now 0 pending, 1 approved
	counts, err := store.CountPendingAutoroutes(ctx)
	if err != nil {
		t.Fatalf("Count failed: %v", err)
	}
	if counts[StatusPending] != 0 {
		t.Errorf("Pending = %d, want 0", counts[StatusPending])
	}
	if counts[StatusApproved] != 1 {
		t.Errorf("Approved = %d, want 1", counts[StatusApproved])
	}
}

func TestApprovalWorkflow_RejectAndResubmit(t *testing.T) {
	store, ctx := initStoreForTest(t)

	// Create initial pending autoroute
	pending := &PendingAutoroute{
		SourceSystem:  "epic_labs",
		SourceCode:    "WF010",
		TargetSystem:  "http://loinc.org",
		SuggestedCode: "9999-0",
		Confidence:    0.65,
		Reasoning:     "low confidence initial suggestion",
	}
	err := store.CreatePendingAutoroute(ctx, pending)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Reject the initial suggestion
	err = store.RejectPendingAutoroute(ctx, pending.ID, "reviewer", "Confidence too low, wrong code suggested")
	if err != nil {
		t.Fatalf("Reject failed: %v", err)
	}

	// Resubmit with improved suggestion (upsert resets to pending)
	resubmitted := &PendingAutoroute{
		SourceSystem:  "epic_labs",
		SourceCode:    "WF010",
		TargetSystem:  "http://loinc.org",
		SuggestedCode: "9999-0",
		Confidence:    0.92,
		Reasoning:     "improved suggestion with better model",
	}
	err = store.CreatePendingAutoroute(ctx, resubmitted)
	if err != nil {
		t.Fatalf("Resubmit failed: %v", err)
	}

	// Verify it's back to pending with updated confidence
	got, err := store.GetPendingAutoroute(ctx, resubmitted.ID)
	if err != nil {
		t.Fatalf("Get resubmitted failed: %v", err)
	}
	if got.Status != StatusPending {
		t.Errorf("Status = %s, want pending", got.Status)
	}
	if got.Confidence != 0.92 {
		t.Errorf("Confidence = %f, want 0.92", got.Confidence)
	}

	// Now approve the resubmitted version
	mapping, err := store.ApprovePendingAutoroute(ctx, resubmitted.ID, "reviewer", "", "")
	if err != nil {
		t.Fatalf("Approve resubmitted failed: %v", err)
	}

	if mapping.Origin != OriginApprovedAutoroute {
		t.Errorf("Origin = %s, want approved_autoroute", mapping.Origin)
	}
}

func TestApprovalWorkflow_BulkApproveByThreshold(t *testing.T) {
	store, ctx := initStoreForTest(t)

	// Create 10 pending autoroutes with confidence ranging from 0.50 to 0.95
	for i := 0; i < 10; i++ {
		conf := 0.50 + float64(i)*0.05 // 0.50, 0.55, 0.60, 0.65, 0.70, 0.75, 0.80, 0.85, 0.90, 0.95
		p := &PendingAutoroute{
			SourceSystem:  "epic_labs",
			SourceCode:    fmt.Sprintf("BKWF%03d", i+1),
			TargetSystem:  "http://loinc.org",
			SuggestedCode: fmt.Sprintf("B%04d-1", i+1),
			Confidence:    conf,
			Reasoning:     fmt.Sprintf("confidence %.2f", conf),
		}
		if err := store.CreatePendingAutoroute(ctx, p); err != nil {
			t.Fatalf("Create %d failed: %v", i, err)
		}
	}

	// Bulk approve with threshold 0.85 → should approve 3 (0.85, 0.90, 0.95)
	count, mappings, err := store.BulkApprovePendingAutoroutes(ctx, 0.85, 100, "bulk-admin")
	if err != nil {
		t.Fatalf("BulkApprove failed: %v", err)
	}

	if count != 3 {
		t.Errorf("Approved = %d, want 3 (0.85, 0.90, 0.95)", count)
	}
	if len(mappings) != 3 {
		t.Errorf("Mappings = %d, want 3", len(mappings))
	}

	// Verify all approved mappings have correct origin
	for _, m := range mappings {
		if m.Origin != OriginApprovedAutoroute {
			t.Errorf("Mapping %d origin = %s, want approved_autoroute", m.ID, m.Origin)
		}
	}

	// Verify counts
	counts, err := store.CountPendingAutoroutes(ctx)
	if err != nil {
		t.Fatalf("Count failed: %v", err)
	}
	if counts[StatusPending] != 7 {
		t.Errorf("Remaining pending = %d, want 7", counts[StatusPending])
	}
	if counts[StatusApproved] != 3 {
		t.Errorf("Approved = %d, want 3", counts[StatusApproved])
	}
}

func TestApprovalWorkflow_ExpiryDoesNotAffectApproved(t *testing.T) {
	store, ctx := initStoreForTest(t)

	past := time.Now().Add(-1 * time.Hour)

	// Create and immediately approve a pending autoroute with past expiration
	pending := &PendingAutoroute{
		SourceSystem:  "epic_labs",
		SourceCode:    "EXPWF001",
		TargetSystem:  "http://loinc.org",
		SuggestedCode: "4444-1",
		Confidence:    0.90,
		ExpiresAt:     &past,
	}
	err := store.CreatePendingAutoroute(ctx, pending)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Approve before expiry logic runs
	_, err = store.ApprovePendingAutoroute(ctx, pending.ID, "reviewer", "", "")
	if err != nil {
		t.Fatalf("Approve failed: %v", err)
	}

	// Run expiry — should NOT affect the already-approved autoroute
	count, err := store.ExpirePendingAutoroutes(ctx)
	if err != nil {
		t.Fatalf("Expire failed: %v", err)
	}
	if count != 0 {
		t.Errorf("Expired = %d, want 0 (approved should not be expired)", count)
	}

	// Verify it's still approved
	got, err := store.GetPendingAutoroute(ctx, pending.ID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got.Status != StatusApproved {
		t.Errorf("Status = %s, want approved", got.Status)
	}
}

func TestApprovalWorkflow_StatsAccuracy(t *testing.T) {
	store, ctx := initStoreForTest(t)

	// Create 5 pending autoroutes
	var ids []int64
	for i := 0; i < 5; i++ {
		p := &PendingAutoroute{
			SourceSystem:  "epic_labs",
			SourceCode:    fmt.Sprintf("STAT%03d", i+1),
			TargetSystem:  "http://loinc.org",
			SuggestedCode: fmt.Sprintf("ST%04d-1", i+1),
			Confidence:    0.80,
		}
		if err := store.CreatePendingAutoroute(ctx, p); err != nil {
			t.Fatalf("Create %d failed: %v", i, err)
		}
		ids = append(ids, p.ID)
	}

	// Approve 2
	for i := 0; i < 2; i++ {
		_, err := store.ApprovePendingAutoroute(ctx, ids[i], "reviewer", "", "")
		if err != nil {
			t.Fatalf("Approve %d failed: %v", i, err)
		}
	}

	// Reject 1
	err := store.RejectPendingAutoroute(ctx, ids[2], "reviewer", "bad")
	if err != nil {
		t.Fatalf("Reject failed: %v", err)
	}

	// Verify counts match expectations: 2 pending, 2 approved, 1 rejected
	counts, err := store.CountPendingAutoroutes(ctx)
	if err != nil {
		t.Fatalf("Count failed: %v", err)
	}

	if counts[StatusPending] != 2 {
		t.Errorf("Pending = %d, want 2", counts[StatusPending])
	}
	if counts[StatusApproved] != 2 {
		t.Errorf("Approved = %d, want 2", counts[StatusApproved])
	}
	if counts[StatusRejected] != 1 {
		t.Errorf("Rejected = %d, want 1", counts[StatusRejected])
	}

	// Total should be 5
	totalFromCounts := counts[StatusPending] + counts[StatusApproved] + counts[StatusRejected]
	if totalFromCounts != 5 {
		t.Errorf("Total from counts = %d, want 5", totalFromCounts)
	}
}

// initStoreForTestCtx is a variant that also returns the underlying DB for direct SQL operations.
func initStoreForTestCtx(t *testing.T) (*MappingStore, context.Context) {
	return initStoreForTest(t)
}
