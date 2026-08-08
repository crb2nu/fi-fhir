package resolvers

import (
	"context"
	"errors"
	"strconv"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/graphql/model"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/operator"
)

// ErrOperatorControlPlaneUnavailable keeps the control plane closed until a
// durable delivery and lifecycle catalog is configured. It is deliberately
// indistinguishable from a missing capability so an unconfigured deployment
// discloses nothing about its inventory.
var ErrOperatorControlPlaneUnavailable = errors.New("operator control plane unavailable")

// operatorService returns the configured control plane or the fail-closed error.
func (r *Resolver) operatorService() (*operator.Service, error) {
	if r == nil || r.OperatorControlPlane == nil {
		return nil, ErrOperatorControlPlaneUnavailable
	}
	return r.OperatorControlPlane, nil
}

// catalogOperatorError maps internal control-plane failures onto stable,
// inventory-safe GraphQL messages. Not-found and forbidden stay distinct
// because the service already collapses cross-tenant reads into not-found.
func catalogOperatorError(err error) error {
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return err
	case errors.Is(err, operator.ErrUnauthenticated):
		return errors.New("authentication required")
	case errors.Is(err, operator.ErrForbidden):
		return errors.New("operator control-plane action forbidden")
	case errors.Is(err, operator.ErrInvalidRequest):
		return errors.New("invalid operator control-plane request")
	case errors.Is(err, operator.ErrNotFound):
		return errors.New("operator control-plane record not found")
	case errors.Is(err, operator.ErrNotDeadLettered):
		return errors.New("delivery attempt is not dead-lettered")
	case errors.Is(err, operator.ErrOperationConflict):
		return errors.New("operator operation idempotency conflict")
	case errors.Is(err, operator.ErrVersionConflict):
		return errors.New("integration deployment version conflict")
	case errors.Is(err, operator.ErrInvalidTransition):
		return errors.New("invalid integration deployment transition")
	case errors.Is(err, operator.ErrUnavailable), errors.Is(err, ErrOperatorControlPlaneUnavailable):
		return ErrOperatorControlPlaneUnavailable
	default:
		return errors.New("operator control-plane request failed")
	}
}

func operatorPageRequest(page *model.OperatorPageInput) operator.PageRequest {
	request := operator.PageRequest{}
	if page == nil {
		return request
	}
	if page.First != nil {
		request.First = *page.First
	}
	if page.After != nil {
		request.Cursor = *page.After
	}
	return request
}

func operatorPageInfo(cursor string, hasMore bool) *model.OperatorPageInfo {
	info := &model.OperatorPageInfo{HasNextPage: hasMore}
	if cursor != "" {
		endCursor := cursor
		info.EndCursor = &endCursor
	}
	return info
}

func operatorReceiptFilter(filter *model.OperatorReceiptFilter) operator.ReceiptFilter {
	out := operator.ReceiptFilter{}
	if filter == nil {
		return out
	}
	out.Status = optionalStringValue(filter.Status)
	out.IntegrationArtifactID = optionalStringValue(filter.IntegrationArtifactID)
	out.CorrelationID = optionalStringValue(filter.CorrelationID)
	out.SourceMessageID = optionalStringValue(filter.SourceMessageID)
	out.From = filter.From
	out.To = filter.To
	return out
}

func operatorAttemptFilter(filter *model.OperatorAttemptFilter) operator.AttemptFilter {
	out := operator.AttemptFilter{}
	if filter == nil {
		return out
	}
	out.Status = optionalStringValue(filter.Status)
	out.DestinationArtifactID = optionalStringValue(filter.DestinationArtifactID)
	out.ReceiptID = optionalStringValue(filter.ReceiptID)
	out.Route = optionalStringValue(filter.Route)
	out.From = filter.From
	out.To = filter.To
	return out
}

func optionalStringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func projectOperatorPrincipal(principal operator.PrincipalSummary) *model.OperatorPrincipal {
	roles := principal.Roles
	if roles == nil {
		roles = []string{}
	}
	return &model.OperatorPrincipal{
		ID:         principal.ID,
		Kind:       principal.Kind,
		AuthMethod: principal.AuthMethod,
		Roles:      roles,
	}
}

func projectOperatorReceipt(receipt operator.ReceiptSummary) model.OperatorReceipt {
	revision := projectArtifactRevision(receipt.IntegrationRevision)
	return model.OperatorReceipt{
		TenantID:            receipt.TenantID,
		ReceiptID:           receipt.ReceiptID,
		Status:              receipt.Status,
		RecordedAt:          receipt.RecordedAt,
		CorrelationID:       receipt.CorrelationID,
		RawRetentionMode:    receipt.RawRetentionMode,
		IntegrationRevision: &revision,
		Principal:           projectOperatorPrincipal(receipt.Principal),
		Reason:              receipt.Reason,
		EventCount:          receipt.EventCount,
		AttemptCount:        receipt.AttemptCount,
		FailedAttemptCount:  receipt.FailedAttemptCount,
		DeadLetterCount:     receipt.DeadLetterCount,
	}
}

func projectOperatorEvent(event operator.EventSummary) model.OperatorEvent {
	fields := make([]model.OperatorPayloadField, 0, len(event.PayloadFields))
	for _, field := range event.PayloadFields {
		fields = append(fields, model.OperatorPayloadField{
			Path:     field.Path,
			Kind:     field.Kind,
			Repeated: field.Repeated,
		})
	}
	return model.OperatorEvent{
		EventID:          event.EventID,
		ReceiptID:        event.ReceiptID,
		EventType:        event.EventType,
		SourceMessageID:  event.SourceMessageID,
		CorrelationID:    event.CorrelationID,
		Classification:   event.Classification,
		RecordedAt:       event.RecordedAt,
		PayloadFields:    fields,
		PayloadTruncated: event.PayloadTruncated,
	}
}

func projectOperatorLineage(link operator.LineageSummary) model.OperatorLineage {
	routes := make([]model.OperatorRoute, 0, len(link.Routes))
	for _, route := range link.Routes {
		routes = append(routes, model.OperatorRoute{
			Route:           route.Route,
			Matched:         route.Matched,
			Skipped:         route.Skipped,
			SkipReason:      optionalPreviewString(route.SkipReason),
			TransformCount:  route.TransformCount,
			PlannedActions:  nonNilStrings(route.PlannedActions),
			DiagnosticCodes: nonNilStrings(route.DiagnosticCodes),
		})
	}
	diagnostics := make([]model.OperatorDiagnostic, 0, len(link.Diagnostics))
	for _, diagnostic := range link.Diagnostics {
		diagnostics = append(diagnostics, model.OperatorDiagnostic{
			Severity:       diagnostic.Severity,
			Stage:          diagnostic.Stage,
			Code:           diagnostic.Code,
			Path:           optionalPreviewString(diagnostic.Path),
			Classification: diagnostic.Classification,
		})
	}
	return model.OperatorLineage{
		LineageID:       link.LineageID,
		ReceiptID:       link.ReceiptID,
		EventID:         link.EventID,
		TraceID:         link.TraceID,
		CorrelationID:   link.CorrelationID,
		SourceMessageID: link.SourceMessageID,
		ArtifactRevisions: &model.IntegrationExecutionArtifactRevisions{
			Source:   projectArtifactRevision(link.ArtifactRevisions.Source),
			Profile:  projectArtifactRevision(link.ArtifactRevisions.Profile),
			Workflow: projectArtifactRevision(link.ArtifactRevisions.Workflow),
		},
		Routes:      routes,
		Diagnostics: diagnostics,
		RecordedAt:  link.RecordedAt,
	}
}

func projectOperatorDeadLetter(entry operator.DeadLetterSummary) model.OperatorDeadLetter {
	return model.OperatorDeadLetter{
		AttemptID:      entry.AttemptID,
		Active:         entry.Active,
		FailureCode:    entry.FailureCode,
		FailureDetail:  entry.FailureDetail,
		FailedAt:       entry.FailedAt,
		ReplayCount:    entry.ReplayCount,
		LastReplayedAt: entry.LastReplayedAt,
		Resolution:     entry.Resolution,
		ResolvedAt:     entry.ResolvedAt,
	}
}

func projectOperatorAttempt(attempt operator.DeliveryAttemptSummary) model.OperatorDeliveryAttempt {
	projected := model.OperatorDeliveryAttempt{
		TenantID:        attempt.TenantID,
		AttemptID:       attempt.AttemptID,
		ParentAttemptID: optionalPreviewString(attempt.ParentAttemptID),
		ReceiptID:       attempt.ReceiptID,
		EventID:         attempt.EventID,
		TraceID:         attempt.TraceID,
		Destination: &model.IntegrationPreviewDestination{
			ArtifactID: attempt.Destination.ArtifactID,
			RevisionID: attempt.Destination.RevisionID,
			Digest:     attempt.Destination.Digest,
			Class:      string(attempt.Destination.Class),
		},
		Route:           attempt.Route,
		Action:          attempt.Action,
		Status:          attempt.Status,
		AttemptCount:    attempt.AttemptCount,
		RecordedAt:      attempt.RecordedAt,
		ScheduledAt:     attempt.ScheduledAt,
		CompletedAt:     attempt.CompletedAt,
		LastErrorCode:   attempt.LastErrorCode,
		LastErrorDetail: attempt.LastErrorDetail,
		OutboxStatus:    attempt.OutboxStatus,
		Topic:           attempt.Topic,
		LeaseOwner:      attempt.LeaseOwner,
		LeaseExpiresAt:  attempt.LeaseExpiresAt,
	}
	if attempt.DeadLetter != nil {
		deadLetter := projectOperatorDeadLetter(*attempt.DeadLetter)
		projected.DeadLetter = &deadLetter
	}
	return projected
}

func projectOperatorAudit(record operator.AuditSummary) model.OperatorAuditRecord {
	detail := record.Detail
	if detail == nil {
		detail = map[string]any{}
	}
	return model.OperatorAuditRecord{
		AuditID:      strconv.FormatInt(record.AuditID, 10),
		AttemptID:    record.AttemptID,
		EventKind:    record.EventKind,
		AttemptCount: record.AttemptCount,
		Principal:    projectOperatorPrincipal(record.Principal),
		Reason:       record.Reason,
		Detail:       detail,
		RecordedAt:   record.RecordedAt,
	}
}

func projectOperatorDeployment(deployment operator.DeploymentSummary) model.OperatorDeployment {
	revision := projectArtifactRevision(deployment.DefinitionRevision)
	return model.OperatorDeployment{
		DefinitionRevision:  &revision,
		State:               deployment.State,
		Version:             int(deployment.Version),
		ReleaseID:           optionalPreviewString(deployment.ReleaseID),
		Health:              deployment.Health,
		ValidationPassed:    deployment.ValidationPassed,
		ValidationExpiresAt: deployment.ValidationExpiresAt,
		UpdatedBy:           projectOperatorPrincipal(deployment.UpdatedBy),
		UpdatedReason:       deployment.UpdatedReason,
		UpdatedAt:           deployment.UpdatedAt,
	}
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func (r *queryResolver) operatorReceipts(
	ctx context.Context,
	filter *model.OperatorReceiptFilter,
	page *model.OperatorPageInput,
) (*model.OperatorReceiptConnection, error) {
	service, err := r.operatorService()
	if err != nil {
		return nil, catalogOperatorError(err)
	}
	result, err := service.ListReceipts(ctx, operatorReceiptFilter(filter), operatorPageRequest(page))
	if err != nil {
		return nil, catalogOperatorError(err)
	}
	nodes := make([]model.OperatorReceipt, 0, len(result.Items))
	for _, receipt := range result.Items {
		nodes = append(nodes, projectOperatorReceipt(receipt))
	}
	return &model.OperatorReceiptConnection{
		Nodes:    nodes,
		PageInfo: operatorPageInfo(result.NextCursor, result.HasMore),
	}, nil
}

func (r *queryResolver) operatorMessageTrace(ctx context.Context, receiptID string) (*model.OperatorMessageTrace, error) {
	service, err := r.operatorService()
	if err != nil {
		return nil, catalogOperatorError(err)
	}
	trace, err := service.GetMessageTrace(ctx, receiptID)
	if errors.Is(err, operator.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, catalogOperatorError(err)
	}
	receipt := projectOperatorReceipt(trace.Receipt)
	events := make([]model.OperatorEvent, 0, len(trace.Events))
	for _, event := range trace.Events {
		events = append(events, projectOperatorEvent(event))
	}
	lineage := make([]model.OperatorLineage, 0, len(trace.Lineage))
	for _, link := range trace.Lineage {
		lineage = append(lineage, projectOperatorLineage(link))
	}
	attempts := make([]model.OperatorDeliveryAttempt, 0, len(trace.Attempts))
	for _, attempt := range trace.Attempts {
		attempts = append(attempts, projectOperatorAttempt(attempt))
	}
	audit := make([]model.OperatorAuditRecord, 0, len(trace.Audit))
	for _, record := range trace.Audit {
		audit = append(audit, projectOperatorAudit(record))
	}
	return &model.OperatorMessageTrace{
		Receipt:  &receipt,
		Events:   events,
		Lineage:  lineage,
		Attempts: attempts,
		Audit:    audit,
	}, nil
}

func (r *queryResolver) operatorDeliveryAttempts(
	ctx context.Context,
	filter *model.OperatorAttemptFilter,
	page *model.OperatorPageInput,
) (*model.OperatorDeliveryAttemptConnection, error) {
	service, err := r.operatorService()
	if err != nil {
		return nil, catalogOperatorError(err)
	}
	result, err := service.ListAttempts(ctx, operatorAttemptFilter(filter), operatorPageRequest(page))
	if err != nil {
		return nil, catalogOperatorError(err)
	}
	nodes := make([]model.OperatorDeliveryAttempt, 0, len(result.Items))
	for _, attempt := range result.Items {
		nodes = append(nodes, projectOperatorAttempt(attempt))
	}
	return &model.OperatorDeliveryAttemptConnection{
		Nodes:    nodes,
		PageInfo: operatorPageInfo(result.NextCursor, result.HasMore),
	}, nil
}

func (r *queryResolver) operatorDeliveryAttempt(ctx context.Context, attemptID string) (*model.OperatorDeliveryAttempt, error) {
	service, err := r.operatorService()
	if err != nil {
		return nil, catalogOperatorError(err)
	}
	attempt, err := service.GetAttempt(ctx, attemptID)
	if errors.Is(err, operator.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, catalogOperatorError(err)
	}
	projected := projectOperatorAttempt(attempt)
	return &projected, nil
}

func (r *queryResolver) operatorDeadLetters(
	ctx context.Context,
	activeOnly *bool,
	page *model.OperatorPageInput,
) (*model.OperatorDeadLetterConnection, error) {
	service, err := r.operatorService()
	if err != nil {
		return nil, catalogOperatorError(err)
	}
	onlyActive := true
	if activeOnly != nil {
		onlyActive = *activeOnly
	}
	result, err := service.ListDeadLetters(ctx, onlyActive, operatorPageRequest(page))
	if err != nil {
		return nil, catalogOperatorError(err)
	}
	nodes := make([]model.OperatorDeadLetter, 0, len(result.Items))
	for _, entry := range result.Items {
		nodes = append(nodes, projectOperatorDeadLetter(entry))
	}
	return &model.OperatorDeadLetterConnection{
		Nodes:    nodes,
		PageInfo: operatorPageInfo(result.NextCursor, result.HasMore),
	}, nil
}

func (r *queryResolver) operatorCircuits(ctx context.Context) ([]model.OperatorCircuit, error) {
	service, err := r.operatorService()
	if err != nil {
		return nil, catalogOperatorError(err)
	}
	circuits, err := service.ListCircuits(ctx)
	if err != nil {
		return nil, catalogOperatorError(err)
	}
	projected := make([]model.OperatorCircuit, 0, len(circuits))
	for _, circuit := range circuits {
		destination := projectArtifactRevision(circuit.Destination)
		projected = append(projected, model.OperatorCircuit{
			Destination:         &destination,
			State:               circuit.State,
			ConsecutiveFailures: circuit.Failures,
			OpenUntil:           circuit.OpenUntil,
			UpdatedAt:           circuit.UpdatedAt,
		})
	}
	return projected, nil
}

func (r *queryResolver) operatorAttemptAudit(
	ctx context.Context,
	attemptID string,
	page *model.OperatorPageInput,
) (*model.OperatorAuditConnection, error) {
	service, err := r.operatorService()
	if err != nil {
		return nil, catalogOperatorError(err)
	}
	result, err := service.ListAttemptAudit(ctx, attemptID, operatorPageRequest(page))
	if err != nil {
		return nil, catalogOperatorError(err)
	}
	nodes := make([]model.OperatorAuditRecord, 0, len(result.Items))
	for _, record := range result.Items {
		nodes = append(nodes, projectOperatorAudit(record))
	}
	return &model.OperatorAuditConnection{
		Nodes:    nodes,
		PageInfo: operatorPageInfo(result.NextCursor, result.HasMore),
	}, nil
}

func (r *queryResolver) operatorDeployments(ctx context.Context) ([]model.OperatorDeployment, error) {
	service, err := r.operatorService()
	if err != nil {
		return nil, catalogOperatorError(err)
	}
	deployments, err := service.ListDeployments(ctx)
	if err != nil {
		return nil, catalogOperatorError(err)
	}
	projected := make([]model.OperatorDeployment, 0, len(deployments))
	for _, deployment := range deployments {
		projected = append(projected, projectOperatorDeployment(deployment))
	}
	return projected, nil
}

func (r *queryResolver) operatorDeploymentEvents(
	ctx context.Context,
	definitionID string,
	revisionID string,
) ([]model.OperatorDeploymentEvent, error) {
	service, err := r.operatorService()
	if err != nil {
		return nil, catalogOperatorError(err)
	}
	events, err := service.ListDeploymentEvents(ctx, definitionID, revisionID)
	if err != nil {
		return nil, catalogOperatorError(err)
	}
	projected := make([]model.OperatorDeploymentEvent, 0, len(events))
	for _, event := range events {
		projected = append(projected, model.OperatorDeploymentEvent{
			EventID:    event.EventID,
			Version:    int(event.Version),
			Action:     event.Action,
			FromState:  event.FromState,
			ToState:    event.ToState,
			Health:     event.Health,
			ReleaseID:  optionalPreviewString(event.ReleaseID),
			Actor:      projectOperatorPrincipal(event.Actor),
			Reason:     event.Reason,
			OccurredAt: event.OccurredAt,
		})
	}
	return projected, nil
}

func (r *mutationResolver) operatorDeliveryControl(
	ctx context.Context,
	input model.OperatorDeliveryControlInput,
	action string,
) (*model.OperatorControlResult, error) {
	service, err := r.operatorService()
	if err != nil {
		return nil, catalogOperatorError(err)
	}
	request := operator.ControlRequest{
		AttemptID:      input.AttemptID,
		Reason:         input.Reason,
		IdempotencyKey: input.IdempotencyKey,
	}
	var result operator.ControlResult
	switch action {
	case "replay":
		result, err = service.ReplayDelivery(ctx, request)
	case "resubmit":
		result, err = service.ResubmitMessage(ctx, request)
	case "discard":
		result, err = service.DiscardDeadLetter(ctx, request)
	default:
		return nil, catalogOperatorError(operator.ErrInvalidRequest)
	}
	if err != nil {
		return nil, catalogOperatorError(err)
	}
	attempt := projectOperatorAttempt(result.Attempt)
	return &model.OperatorControlResult{
		Kind:            result.Kind,
		SourceAttemptID: result.SourceAttemptID,
		ResultAttemptID: result.ResultAttemptID,
		Attempt:         &attempt,
		Reason:          result.Reason,
		IdempotencyKey:  result.IdempotencyKey,
		Actor:           projectOperatorPrincipal(result.Actor),
	}, nil
}

func (r *mutationResolver) operatorDeploymentControl(
	ctx context.Context,
	input model.OperatorDeploymentCommandInput,
	action string,
) (*model.OperatorDeployment, error) {
	service, err := r.operatorService()
	if err != nil {
		return nil, catalogOperatorError(err)
	}
	command := operator.DeploymentCommand{
		DefinitionID:    input.DefinitionID,
		RevisionID:      input.RevisionID,
		ExpectedVersion: int64(input.ExpectedVersion),
		Reason:          input.Reason,
	}
	var summary operator.DeploymentSummary
	switch action {
	case "pause":
		summary, err = service.PauseDeployment(ctx, command)
	case "resume":
		summary, err = service.ResumeDeployment(ctx, command)
	case "retire":
		summary, err = service.RetireDeployment(ctx, command)
	case "deploy":
		summary, err = service.DeployRelease(ctx, command)
	default:
		return nil, catalogOperatorError(operator.ErrInvalidRequest)
	}
	if err != nil {
		return nil, catalogOperatorError(err)
	}
	projected := projectOperatorDeployment(summary)
	return &projected, nil
}
