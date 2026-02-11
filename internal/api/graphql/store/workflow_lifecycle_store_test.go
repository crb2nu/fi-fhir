package store

import (
	"context"
	"testing"
)

func TestMemoryWorkflowLifecycleStore_VersioningAndPublishPointers(t *testing.T) {
	s := NewMemoryWorkflowLifecycleStore()
	ctx := context.Background()

	def, err := s.CreateWorkflowDefinition(ctx, &WorkflowDefinitionRecord{
		Name: "workflow_store_test",
	})
	if err != nil {
		t.Fatalf("CreateWorkflowDefinition failed: %v", err)
	}

	v1, err := s.SaveWorkflowVersion(ctx, &WorkflowVersionRecord{
		WorkflowID: def.ID,
		Yaml:       "name: workflow_store_test\nroutes: []\n",
		Validation: WorkflowValidationRecord{Valid: true},
		CreatedBy:  "test",
	})
	if err != nil {
		t.Fatalf("SaveWorkflowVersion(v1) failed: %v", err)
	}
	if v1.VersionNumber != 1 {
		t.Fatalf("expected v1 version number 1, got %d", v1.VersionNumber)
	}

	v2, err := s.SaveWorkflowVersion(ctx, &WorkflowVersionRecord{
		WorkflowID: def.ID,
		Yaml:       "name: workflow_store_test\nroutes: []\n",
		Validation: WorkflowValidationRecord{Valid: true},
		CreatedBy:  "test",
	})
	if err != nil {
		t.Fatalf("SaveWorkflowVersion(v2) failed: %v", err)
	}
	if v2.VersionNumber != 2 {
		t.Fatalf("expected v2 version number 2, got %d", v2.VersionNumber)
	}

	_, err = s.PublishWorkflowVersion(ctx, &WorkflowReleaseRecord{
		WorkflowID:  def.ID,
		VersionID:   v1.ID,
		Environment: "staging",
		PublishedBy: "test",
	})
	if err != nil {
		t.Fatalf("PublishWorkflowVersion(v1) failed: %v", err)
	}

	release2, err := s.PublishWorkflowVersion(ctx, &WorkflowReleaseRecord{
		WorkflowID:  def.ID,
		VersionID:   v2.ID,
		Environment: "staging",
		PublishedBy: "test",
	})
	if err != nil {
		t.Fatalf("PublishWorkflowVersion(v2) failed: %v", err)
	}

	published, err := s.GetPublishedWorkflowRelease(ctx, def.ID, "staging")
	if err != nil {
		t.Fatalf("GetPublishedWorkflowRelease failed: %v", err)
	}
	if published == nil {
		t.Fatalf("expected published release")
	}
	if published.ID != release2.ID {
		t.Fatalf("expected latest release %q, got %q", release2.ID, published.ID)
	}
	if published.VersionID != v2.ID {
		t.Fatalf("expected version %q, got %q", v2.ID, published.VersionID)
	}
}

func TestMemoryWorkflowLifecycleStore_ApprovalLifecycle(t *testing.T) {
	s := NewMemoryWorkflowLifecycleStore()
	ctx := context.Background()

	req, err := s.CreateWorkflowApprovalRequest(ctx, &WorkflowApprovalRequestRecord{
		WorkflowID:      "wf_1",
		TargetVersionID: "wfv_1",
		Environment:     "production",
		RequestedBy:     "requester",
	})
	if err != nil {
		t.Fatalf("CreateWorkflowApprovalRequest failed: %v", err)
	}
	if req.Status != WorkflowApprovalStatusPending {
		t.Fatalf("expected pending status, got %q", req.Status)
	}

	reviewer := "reviewer"
	req.Status = WorkflowApprovalStatusApproved
	req.ReviewedBy = &reviewer
	updated, err := s.UpdateWorkflowApprovalRequest(ctx, req)
	if err != nil {
		t.Fatalf("UpdateWorkflowApprovalRequest failed: %v", err)
	}
	if updated.Status != WorkflowApprovalStatusApproved {
		t.Fatalf("expected approved status, got %q", updated.Status)
	}

	filter := WorkflowApprovalRequestListFilter{
		WorkflowID: &req.WorkflowID,
		Status:     &req.Status,
	}
	results, err := s.ListWorkflowApprovalRequests(ctx, filter, Paging{Limit: 10})
	if err != nil {
		t.Fatalf("ListWorkflowApprovalRequests failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 approval request, got %d", len(results))
	}
}
