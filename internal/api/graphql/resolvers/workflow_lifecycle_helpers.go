package resolvers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/graphql/model"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/graphql/store"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/workflow"
)

const defaultWorkflowActor = "service-account"

func actorOrDefault(actor *string) string {
	if actor == nil {
		return defaultWorkflowActor
	}
	v := strings.TrimSpace(*actor)
	if v == "" {
		return defaultWorkflowActor
	}
	return v
}

func ptrToString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func pagingFromInput(paging *model.PagingInput) store.Paging {
	if paging == nil {
		return store.Paging{Limit: 50, Offset: 0}
	}

	limit := 50
	offset := 0
	if paging.Limit != nil {
		limit = *paging.Limit
	}
	if paging.Offset != nil {
		offset = *paging.Offset
	}
	return store.Paging{
		Limit:  limit,
		Offset: offset,
	}
}

func toGraphQLWorkflowValidation(v store.WorkflowValidationRecord) *model.WorkflowValidation {
	return &model.WorkflowValidation{
		Valid:    v.Valid,
		Errors:   append([]string(nil), v.Errors...),
		Warnings: append([]string(nil), v.Warnings...),
		Info:     append([]string(nil), v.Info...),
	}
}

func toGraphQLWorkflowVersion(v *store.WorkflowVersionRecord) *model.WorkflowVersion {
	if v == nil {
		return nil
	}
	var notes *string
	if strings.TrimSpace(v.Notes) != "" {
		n := strings.TrimSpace(v.Notes)
		notes = &n
	}
	return &model.WorkflowVersion{
		ID:            v.ID,
		WorkflowID:    v.WorkflowID,
		VersionNumber: v.VersionNumber,
		Yaml:          v.Yaml,
		Validation:    toGraphQLWorkflowValidation(v.Validation),
		CreatedBy:     v.CreatedBy,
		CreatedAt:     v.CreatedAt,
		Notes:         notes,
	}
}

func toGraphQLWorkflowRelease(r *store.WorkflowReleaseRecord) *model.WorkflowRelease {
	if r == nil {
		return nil
	}
	return &model.WorkflowRelease{
		ID:                    r.ID,
		WorkflowID:            r.WorkflowID,
		Environment:           r.Environment,
		VersionID:             r.VersionID,
		PublishedBy:           r.PublishedBy,
		PublishedAt:           r.PublishedAt,
		RollbackFromReleaseID: r.RollbackFromReleaseID,
	}
}

func toGraphQLWorkflowRun(r *store.WorkflowRunRecord) *model.WorkflowRun {
	if r == nil {
		return nil
	}
	return &model.WorkflowRun{
		ID:              r.ID,
		WorkflowName:    r.WorkflowName,
		Environment:     r.Environment,
		VersionID:       r.VersionID,
		EventID:         r.EventID,
		RoutesMatched:   r.RoutesMatched,
		ActionsExecuted: r.ActionsExecuted,
		Errors:          append([]string(nil), r.Errors...),
		DurationMs:      r.DurationMs,
		StartedAt:       r.StartedAt,
		Status:          r.Status,
	}
}

func toGraphQLWorkflowApprovalRequest(req *store.WorkflowApprovalRequestRecord) *model.WorkflowApprovalRequest {
	if req == nil {
		return nil
	}
	return &model.WorkflowApprovalRequest{
		ID:              req.ID,
		WorkflowID:      req.WorkflowID,
		TargetVersionID: req.TargetVersionID,
		Environment:     req.Environment,
		Status:          req.Status,
		RequestedBy:     req.RequestedBy,
		ReviewedBy:      req.ReviewedBy,
		ReviewedAt:      req.ReviewedAt,
		Comment:         req.Comment,
	}
}

func (r *Resolver) toGraphQLWorkflowDefinition(ctx context.Context, def *store.WorkflowDefinitionRecord) (*model.WorkflowDefinition, error) {
	if def == nil {
		return nil, nil
	}

	var description *string
	if strings.TrimSpace(def.Description) != "" {
		v := strings.TrimSpace(def.Description)
		description = &v
	}

	latestVersion, err := r.WorkflowLifecycleStore.GetLatestWorkflowVersion(ctx, def.ID)
	if err != nil {
		return nil, fmt.Errorf("get latest workflow version: %w", err)
	}

	releases, err := r.WorkflowLifecycleStore.ListWorkflowReleases(ctx, def.ID)
	if err != nil {
		return nil, fmt.Errorf("list workflow releases: %w", err)
	}

	publishedByEnv := make(map[string]any)
	for _, release := range releases {
		if release == nil {
			continue
		}
		// Releases are sorted newest-first in store; keep first value for each env.
		if _, exists := publishedByEnv[release.Environment]; exists {
			continue
		}
		publishedByEnv[release.Environment] = release.VersionID
	}

	return &model.WorkflowDefinition{
		ID:                     def.ID,
		Name:                   def.Name,
		Description:            description,
		Status:                 def.Status,
		CreatedAt:              def.CreatedAt,
		UpdatedAt:              def.UpdatedAt,
		LatestVersion:          toGraphQLWorkflowVersion(latestVersion),
		PublishedVersionsByEnv: publishedByEnv,
	}, nil
}

func validateWorkflowYAML(yamlContent string, expectedName string) (*workflow.Workflow, store.WorkflowValidationRecord, error) {
	parsed, err := workflow.ParseWorkflow([]byte(yamlContent))
	if err != nil {
		return nil, store.WorkflowValidationRecord{
			Valid:  false,
			Errors: []string{fmt.Sprintf("invalid workflow yaml: %v", err)},
		}, fmt.Errorf("parse workflow yaml: %w", err)
	}

	if strings.TrimSpace(parsed.Name) == "" {
		return nil, store.WorkflowValidationRecord{
			Valid:  false,
			Errors: []string{"workflow name is required"},
		}, fmt.Errorf("workflow yaml is missing name")
	}

	if expectedName != "" && parsed.Name != expectedName {
		return nil, store.WorkflowValidationRecord{
			Valid: false,
			Errors: []string{
				fmt.Sprintf("workflow name %q does not match definition %q", parsed.Name, expectedName),
			},
		}, fmt.Errorf("workflow yaml name does not match definition")
	}

	validationResult, err := workflow.ValidateWorkflow(parsed)
	if err != nil {
		return nil, store.WorkflowValidationRecord{
			Valid:  false,
			Errors: []string{fmt.Sprintf("workflow validation failed: %v", err)},
		}, fmt.Errorf("validate workflow: %w", err)
	}

	validation := store.WorkflowValidationRecord{
		Valid: validationResult.Valid,
	}
	for _, issue := range validationResult.Errors {
		validation.Errors = append(validation.Errors, fmt.Sprintf("%s: %s", issue.Path, issue.Message))
	}
	for _, issue := range validationResult.Warnings {
		validation.Warnings = append(validation.Warnings, fmt.Sprintf("%s: %s", issue.Path, issue.Message))
	}
	for _, issue := range validationResult.Info {
		validation.Info = append(validation.Info, fmt.Sprintf("%s: %s", issue.Path, issue.Message))
	}

	return parsed, validation, nil
}

func summarizeWorkflowResult(result *workflow.Result) (routesMatched int, actionsExecuted int, errors []string, routeNames []string, actionsSummary []string) {
	for _, rr := range result.RouteResults {
		if !rr.Matched {
			continue
		}
		routesMatched++
		routeNames = append(routeNames, rr.RouteName)
		actionsExecuted += rr.ActionsRun
		if rr.ActionsRun > 0 {
			actionsSummary = append(actionsSummary, fmt.Sprintf("%s:%d", rr.RouteName, rr.ActionsRun))
		}
		for _, err := range rr.ActionErrors {
			errors = append(errors, err.Error())
		}
		for _, err := range rr.TransformErrors {
			errors = append(errors, err.Error())
		}
	}
	return routesMatched, actionsExecuted, errors, routeNames, actionsSummary
}

func approvalForTarget(ctx context.Context, lifecycleStore store.WorkflowLifecycleStore, workflowID, versionID, environment, status string) (bool, error) {
	filter := store.WorkflowApprovalRequestListFilter{
		WorkflowID:  &workflowID,
		Environment: &environment,
		Status:      &status,
	}
	requests, err := lifecycleStore.ListWorkflowApprovalRequests(ctx, filter, store.Paging{Limit: 200, Offset: 0})
	if err != nil {
		return false, err
	}

	for _, req := range requests {
		if req.TargetVersionID == versionID {
			return true, nil
		}
	}
	return false, nil
}

func nowUTC() time.Time {
	return time.Now().UTC()
}

func (r *Resolver) appendWorkflowAudit(ctx context.Context, workflowID, eventType, actor string, metadata map[string]any) error {
	if r.WorkflowLifecycleStore == nil {
		return nil
	}
	_, err := r.WorkflowLifecycleStore.CreateWorkflowAuditLog(ctx, &store.WorkflowAuditLogRecord{
		WorkflowID: workflowID,
		EventType:  eventType,
		Actor:      actor,
		OccurredAt: nowUTC(),
		Metadata:   metadata,
	})
	if err != nil {
		return fmt.Errorf("create workflow audit log: %w", err)
	}
	return nil
}
