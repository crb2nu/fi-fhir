package resolvers

import (
	"context"
	"testing"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/graphql/model"
)

func testWorkflowYAML(name string) string {
	return `name: ` + name + `
version: "1.0"
routes:
  - name: route_all
    filter:
      event_type: lab_result
    actions:
      - type: log
        level: info
`
}

func TestWorkflowLifecycle_CreateSavePublishTriggerAndRuns(t *testing.T) {
	resolver := NewResolver()
	mutations := &mutationResolver{resolver}
	queries := &queryResolver{resolver}
	ctx := context.Background()

	definition, err := mutations.CreateWorkflowDefinition(ctx, model.CreateWorkflowDefinitionInput{
		Name: "wf_lifecycle_trigger",
	})
	if err != nil {
		t.Fatalf("CreateWorkflowDefinition failed: %v", err)
	}

	version, err := mutations.SaveWorkflowVersion(ctx, model.SaveWorkflowVersionInput{
		WorkflowID: definition.ID,
		Yaml:       testWorkflowYAML(definition.Name),
	})
	if err != nil {
		t.Fatalf("SaveWorkflowVersion failed: %v", err)
	}

	_, err = mutations.PublishWorkflowVersion(ctx, model.PublishWorkflowVersionInput{
		WorkflowID:  definition.ID,
		VersionID:   version.ID,
		Environment: "staging",
	})
	if err != nil {
		t.Fatalf("PublishWorkflowVersion failed: %v", err)
	}

	env := "staging"
	result, err := mutations.TriggerWorkflow(ctx, definition.Name, map[string]any{
		"type":   "lab_result",
		"source": "test-suite",
		"id":     "evt-1",
	}, &env, nil)
	if err != nil {
		t.Fatalf("TriggerWorkflow failed: %v", err)
	}

	if result.RunID == nil || *result.RunID == "" {
		t.Fatalf("expected run ID to be returned")
	}
	if result.Environment == nil || *result.Environment != "staging" {
		t.Fatalf("expected environment 'staging', got %#v", result.Environment)
	}
	if result.VersionID == nil || *result.VersionID != version.ID {
		t.Fatalf("expected version %q, got %#v", version.ID, result.VersionID)
	}

	runs, err := queries.WorkflowRuns(ctx, &model.WorkflowRunFilter{
		WorkflowName: &definition.Name,
	}, nil)
	if err != nil {
		t.Fatalf("WorkflowRuns query failed: %v", err)
	}
	if len(runs) == 0 {
		t.Fatalf("expected at least one workflow run")
	}
}

func TestWorkflowLifecycle_ProductionPublishRequiresApproval(t *testing.T) {
	resolver := NewResolver()
	mutations := &mutationResolver{resolver}
	ctx := context.Background()

	definition, err := mutations.CreateWorkflowDefinition(ctx, model.CreateWorkflowDefinitionInput{
		Name: "wf_prod_approval",
	})
	if err != nil {
		t.Fatalf("CreateWorkflowDefinition failed: %v", err)
	}

	version, err := mutations.SaveWorkflowVersion(ctx, model.SaveWorkflowVersionInput{
		WorkflowID: definition.ID,
		Yaml:       testWorkflowYAML(definition.Name),
	})
	if err != nil {
		t.Fatalf("SaveWorkflowVersion failed: %v", err)
	}

	_, err = mutations.PublishWorkflowVersion(ctx, model.PublishWorkflowVersionInput{
		WorkflowID:  definition.ID,
		VersionID:   version.ID,
		Environment: "production",
	})
	if err == nil {
		t.Fatalf("expected production publish to be blocked without approval")
	}

	request, err := mutations.RequestWorkflowApproval(ctx, model.RequestWorkflowApprovalInput{
		WorkflowID:      definition.ID,
		TargetVersionID: version.ID,
		Environment:     "production",
	})
	if err != nil {
		t.Fatalf("RequestWorkflowApproval failed: %v", err)
	}

	_, err = mutations.ApproveWorkflowVersion(ctx, model.ApproveWorkflowVersionInput{
		ApprovalRequestID: request.ID,
	})
	if err != nil {
		t.Fatalf("ApproveWorkflowVersion failed: %v", err)
	}

	release, err := mutations.PublishWorkflowVersion(ctx, model.PublishWorkflowVersionInput{
		WorkflowID:  definition.ID,
		VersionID:   version.ID,
		Environment: "production",
	})
	if err != nil {
		t.Fatalf("PublishWorkflowVersion after approval failed: %v", err)
	}
	if release.Environment != "production" {
		t.Fatalf("expected production release, got %q", release.Environment)
	}
}

func TestWorkflowLifecycle_RollbackCreatesReleaseLink(t *testing.T) {
	resolver := NewResolver()
	mutations := &mutationResolver{resolver}
	ctx := context.Background()

	definition, err := mutations.CreateWorkflowDefinition(ctx, model.CreateWorkflowDefinitionInput{
		Name: "wf_rollback",
	})
	if err != nil {
		t.Fatalf("CreateWorkflowDefinition failed: %v", err)
	}

	v1, err := mutations.SaveWorkflowVersion(ctx, model.SaveWorkflowVersionInput{
		WorkflowID: definition.ID,
		Yaml:       testWorkflowYAML(definition.Name),
	})
	if err != nil {
		t.Fatalf("SaveWorkflowVersion(v1) failed: %v", err)
	}
	r1, err := mutations.PublishWorkflowVersion(ctx, model.PublishWorkflowVersionInput{
		WorkflowID:  definition.ID,
		VersionID:   v1.ID,
		Environment: "staging",
	})
	if err != nil {
		t.Fatalf("PublishWorkflowVersion(v1) failed: %v", err)
	}

	v2, err := mutations.SaveWorkflowVersion(ctx, model.SaveWorkflowVersionInput{
		WorkflowID: definition.ID,
		Yaml:       testWorkflowYAML(definition.Name),
	})
	if err != nil {
		t.Fatalf("SaveWorkflowVersion(v2) failed: %v", err)
	}
	r2, err := mutations.PublishWorkflowVersion(ctx, model.PublishWorkflowVersionInput{
		WorkflowID:  definition.ID,
		VersionID:   v2.ID,
		Environment: "staging",
	})
	if err != nil {
		t.Fatalf("PublishWorkflowVersion(v2) failed: %v", err)
	}
	if r1.ID == r2.ID {
		t.Fatalf("expected distinct release IDs")
	}

	rollbackRelease, err := mutations.RollbackWorkflowVersion(ctx, model.RollbackWorkflowVersionInput{
		WorkflowID:      definition.ID,
		TargetVersionID: v1.ID,
		Environment:     "staging",
	})
	if err != nil {
		t.Fatalf("RollbackWorkflowVersion failed: %v", err)
	}
	if rollbackRelease.VersionID != v1.ID {
		t.Fatalf("expected rollback to version %q, got %q", v1.ID, rollbackRelease.VersionID)
	}
	if rollbackRelease.RollbackFromReleaseID == nil || *rollbackRelease.RollbackFromReleaseID != r2.ID {
		t.Fatalf("expected rollbackFrom release %q, got %#v", r2.ID, rollbackRelease.RollbackFromReleaseID)
	}
}
