package operator

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/requestsecurity"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/delivery"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/destination"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/lifecycle"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

var (
	// ErrVersionConflict means another writer advanced the lifecycle snapshot.
	ErrVersionConflict = errors.New("integration deployment version conflict")
	// ErrInvalidTransition means the requested lifecycle change is not allowed.
	ErrInvalidTransition = errors.New("invalid integration deployment transition")
	// ErrNotDeadLettered means recovery targeted active or unknown delivery work.
	ErrNotDeadLettered = errors.New("delivery attempt is not dead-lettered")
	// ErrOperationConflict means an idempotency key was reused for other work.
	ErrOperationConflict = errors.New("operator operation idempotency conflict")
)

// DeliveryRecoveryStore is the Slice 2.3 durable recovery machinery. The
// control plane delegates every delivery write to it so idempotency, the
// operation ledger, and the append-only audit trail stay in one place.
type DeliveryRecoveryStore interface {
	Replay(ctx context.Context, tenantID, attemptID string, operation delivery.Operation) (string, error)
	Resubmit(ctx context.Context, tenantID, attemptID string, operation delivery.Operation) (string, error)
	Discard(ctx context.Context, tenantID, attemptID string, operation delivery.Operation) (string, error)
}

// DeploymentCatalog is the Slice 2.1 closed lifecycle state machine.
type DeploymentCatalog interface {
	Deploy(ctx context.Context, command lifecycle.Command) (lifecycle.Snapshot, error)
	Pause(ctx context.Context, command lifecycle.Command) (lifecycle.Snapshot, error)
	Resume(ctx context.Context, command lifecycle.Command) (lifecycle.Snapshot, error)
	Retire(ctx context.Context, command lifecycle.Command) (lifecycle.Snapshot, error)
	ListSnapshots(ctx context.Context, tenantID string, limit int) ([]lifecycle.Snapshot, error)
	ListEvents(ctx context.Context, tenantID, definitionID, revisionID string) ([]lifecycle.EventRecord, error)
}

// DestinationDeliveryReader is the read half of the Slice 4.1c-b/c destination
// provenance ledger. *destination.PostgresProvenance satisfies it. The control
// plane reads the ledger only through this seam, so the destination package
// stays the one owner of that table's SQL.
type DestinationDeliveryReader interface {
	ListDeliveriesForAttempt(ctx context.Context, tenantID, attemptID string, limit int) ([]destination.DeliverySummary, error)
}

// Service is the authorization boundary of the operator control plane. Every
// exported method resolves the verified caller identity from the request
// context, fails closed on a missing role, and scopes work to the caller's
// server-owned tenant.
type Service struct {
	reads      *PostgresReadStore
	deliveries DestinationDeliveryReader
	recovery   DeliveryRecoveryStore
	catalog    DeploymentCatalog
	tenantID   string
}

// NewService binds the control plane to one deployment tenant.
func NewService(
	reads *PostgresReadStore,
	deliveries DestinationDeliveryReader,
	recovery DeliveryRecoveryStore,
	catalog DeploymentCatalog,
	tenantID string,
) (*Service, error) {
	if reads == nil || deliveries == nil || recovery == nil || catalog == nil || !validToken(tenantID, 256) {
		return nil, ErrUnavailable
	}
	return &Service{
		reads:      reads,
		deliveries: deliveries,
		recovery:   recovery,
		catalog:    catalog,
		tenantID:   tenantID,
	}, nil
}

// authorize resolves verified caller identity and requires every listed role.
// Identity is never read from arguments: only the server-owned security
// context populated by the authenticated transport can satisfy this check.
func (s *Service) authorize(ctx context.Context, roles ...string) (integration.SecurityContext, error) {
	if s == nil || s.reads == nil {
		return integration.SecurityContext{}, ErrUnavailable
	}
	security, authenticated := requestsecurity.SecurityContextFromContext(ctx)
	if !authenticated {
		return integration.SecurityContext{}, ErrUnauthenticated
	}
	if !validToken(security.TenantID, 256) || security.TenantID != s.tenantID {
		return integration.SecurityContext{}, ErrForbidden
	}
	if !validToken(security.Principal.ID, 256) || !validToken(security.Principal.AuthMethod, 128) {
		return integration.SecurityContext{}, ErrForbidden
	}
	if security.Principal.Kind != integration.PrincipalKindHuman &&
		security.Principal.Kind != integration.PrincipalKindService {
		return integration.SecurityContext{}, ErrForbidden
	}
	for _, required := range roles {
		if !hasRole(security.Principal.Roles, required) {
			return integration.SecurityContext{}, ErrForbidden
		}
	}
	return security, nil
}

func hasRole(roles []string, wanted string) bool {
	for _, role := range roles {
		if role == wanted {
			return true
		}
	}
	return false
}

// ListReceipts browses durable admissions for the caller's tenant.
func (s *Service) ListReceipts(ctx context.Context, filter ReceiptFilter, page PageRequest) (Page[ReceiptSummary], error) {
	security, err := s.authorize(ctx, ReadRole)
	if err != nil {
		return Page[ReceiptSummary]{}, err
	}
	return s.reads.ListReceipts(ctx, security.TenantID, filter, page)
}

// GetMessageTrace returns one receipt-to-delivery lineage for the caller's
// tenant, each attempt carrying its newest MaxListedAttemptDeliveries
// provenance-ledger rows.
func (s *Service) GetMessageTrace(ctx context.Context, receiptID string) (MessageTrace, error) {
	security, err := s.authorize(ctx, ReadRole)
	if err != nil {
		return MessageTrace{}, err
	}
	trace, err := s.reads.GetMessageTrace(ctx, security.TenantID, receiptID)
	if err != nil {
		return MessageTrace{}, err
	}
	if err := s.attachDeliveries(ctx, security.TenantID, trace.Attempts, MaxListedAttemptDeliveries); err != nil {
		return MessageTrace{}, err
	}
	return trace, nil
}

// ListAttempts browses durable delivery attempts for the caller's tenant, each
// carrying its newest MaxListedAttemptDeliveries provenance-ledger rows.
func (s *Service) ListAttempts(ctx context.Context, filter AttemptFilter, page PageRequest) (Page[DeliveryAttemptSummary], error) {
	security, err := s.authorize(ctx, ReadRole)
	if err != nil {
		return Page[DeliveryAttemptSummary]{}, err
	}
	result, err := s.reads.ListAttempts(ctx, security.TenantID, filter, page)
	if err != nil {
		return Page[DeliveryAttemptSummary]{}, err
	}
	if err := s.attachDeliveries(ctx, security.TenantID, result.Items, MaxListedAttemptDeliveries); err != nil {
		return Page[DeliveryAttemptSummary]{}, err
	}
	return result, nil
}

// GetAttempt returns one delivery attempt for the caller's tenant, carrying its
// newest MaxAttemptDeliveries provenance-ledger rows.
func (s *Service) GetAttempt(ctx context.Context, attemptID string) (DeliveryAttemptSummary, error) {
	security, err := s.authorize(ctx, ReadRole)
	if err != nil {
		return DeliveryAttemptSummary{}, err
	}
	return s.getAttempt(ctx, security.TenantID, attemptID)
}

func (s *Service) getAttempt(ctx context.Context, tenantID, attemptID string) (DeliveryAttemptSummary, error) {
	attempt, err := s.reads.GetAttempt(ctx, tenantID, attemptID)
	if err != nil {
		return DeliveryAttemptSummary{}, err
	}
	attempts := []DeliveryAttemptSummary{attempt}
	if err := s.attachDeliveries(ctx, tenantID, attempts, MaxAttemptDeliveries); err != nil {
		return DeliveryAttemptSummary{}, err
	}
	return attempts[0], nil
}

// attachDeliveries projects each attempt's provenance-ledger rows onto it, in
// place. The read is tenant-scoped by the same server-owned tenant that scoped
// the attempt read, and it is one bounded, indexed lookup per attempt; the
// attempt set itself is already bounded by MaxPageSize.
//
// A ledger failure fails the whole read rather than returning the attempt
// with an empty list: an empty list is a statement that this process contacted
// no destination for the attempt, and it must not be made on a read error.
func (s *Service) attachDeliveries(
	ctx context.Context,
	tenantID string,
	attempts []DeliveryAttemptSummary,
	limit int,
) error {
	for index := range attempts {
		records, err := s.deliveries.ListDeliveriesForAttempt(ctx, tenantID, attempts[index].AttemptID, limit)
		if err != nil {
			return mapLedgerError(err)
		}
		attempts[index].Deliveries = summarizeDeliveries(records)
	}
	return nil
}

func summarizeDeliveries(records []destination.DeliverySummary) []DestinationDeliverySummary {
	summaries := make([]DestinationDeliverySummary, 0, len(records))
	for _, record := range records {
		summaries = append(summaries, DestinationDeliverySummary{
			Transport:                        string(record.Transport),
			DestinationArtifactID:            record.DestinationArtifactID,
			DestinationRevisionID:            record.DestinationRevisionID,
			DestinationClass:                 record.DestinationClass,
			DigestVerified:                   record.DestinationDigestVerified,
			Outcome:                          record.Outcome,
			FailureCode:                      record.FailureCode,
			HTTPStatusClass:                  record.HTTPStatusClass,
			EndpointAdvisory:                 record.EndpointAdvisory,
			ServedCertificateSubjectAdvisory: record.ServedCertificateSubjectAdvisory,
			CompletedAt:                      record.CompletedAt.UTC(),
			FHIRResourceTypes:                append([]string{}, record.FHIRResourceTypes...),
			FHIREntryCount:                   record.FHIREntryCount,
			FHIROutcomeCodesAdvisory:         append([]string{}, record.FHIROutcomeCodesAdvisory...),
		})
	}
	return summaries
}

// ListDeadLetters browses the durable DLQ for the caller's tenant.
func (s *Service) ListDeadLetters(ctx context.Context, activeOnly bool, page PageRequest) (Page[DeadLetterSummary], error) {
	security, err := s.authorize(ctx, ReadRole)
	if err != nil {
		return Page[DeadLetterSummary]{}, err
	}
	return s.reads.ListDeadLetters(ctx, security.TenantID, activeOnly, page)
}

// ListCircuits returns destination circuit state for the caller's tenant.
func (s *Service) ListCircuits(ctx context.Context) ([]CircuitSummary, error) {
	security, err := s.authorize(ctx, ReadRole)
	if err != nil {
		return nil, err
	}
	return s.reads.ListCircuits(ctx, security.TenantID)
}

// ListAttemptAudit returns the append-only audit trail for one attempt.
func (s *Service) ListAttemptAudit(ctx context.Context, attemptID string, page PageRequest) (Page[AuditSummary], error) {
	security, err := s.authorize(ctx, ReadRole)
	if err != nil {
		return Page[AuditSummary]{}, err
	}
	return s.reads.ListAttemptAudit(ctx, security.TenantID, attemptID, page)
}

// ListDeployments returns the lifecycle inventory for the caller's tenant.
func (s *Service) ListDeployments(ctx context.Context) ([]DeploymentSummary, error) {
	security, err := s.authorize(ctx, ReadRole)
	if err != nil {
		return nil, err
	}
	snapshots, err := s.catalog.ListSnapshots(ctx, security.TenantID, maxLifecycleSnapshots)
	if err != nil {
		return nil, mapLifecycleError(err)
	}
	summaries := make([]DeploymentSummary, 0, len(snapshots))
	for _, snapshot := range snapshots {
		summaries = append(summaries, summarizeSnapshot(snapshot))
	}
	return summaries, nil
}

// ListDeploymentEvents returns the append-only lifecycle history for one revision.
func (s *Service) ListDeploymentEvents(ctx context.Context, definitionID, revisionID string) ([]LifecycleEventSummary, error) {
	security, err := s.authorize(ctx, ReadRole)
	if err != nil {
		return nil, err
	}
	if !validToken(definitionID, 256) || !validToken(revisionID, 256) {
		return nil, ErrInvalidRequest
	}
	records, err := s.catalog.ListEvents(ctx, security.TenantID, definitionID, revisionID)
	if err != nil {
		return nil, mapLifecycleError(err)
	}
	events := make([]LifecycleEventSummary, 0, len(records))
	for _, record := range records {
		events = append(events, LifecycleEventSummary{
			EventID:    record.ID,
			Version:    record.Version,
			Action:     record.Action,
			FromState:  string(record.FromState),
			ToState:    string(record.ToState),
			Health:     string(record.Health),
			ReleaseID:  record.ReleaseID,
			Actor:      summarizePrincipal(record.Audit.Principal),
			Reason:     record.Audit.Reason,
			OccurredAt: record.Audit.OccurredAt.UTC(),
		})
	}
	return events, nil
}

// ReplayDelivery requeues the exact dead-lettered attempt once. It is also the
// DLQ requeue action: Slice 2.3's Replay refuses anything that is not an
// active dead letter.
func (s *Service) ReplayDelivery(ctx context.Context, request ControlRequest) (ControlResult, error) {
	return s.recover(ctx, request, "replay")
}

// ResubmitMessage creates one idempotent child attempt from a dead letter.
func (s *Service) ResubmitMessage(ctx context.Context, request ControlRequest) (ControlResult, error) {
	return s.recover(ctx, request, "resubmit")
}

// DiscardDeadLetter abandons one dead letter with an attributable reason.
func (s *Service) DiscardDeadLetter(ctx context.Context, request ControlRequest) (ControlResult, error) {
	return s.recover(ctx, request, "discard")
}

func (s *Service) recover(ctx context.Context, request ControlRequest, kind string) (ControlResult, error) {
	security, err := s.authorize(ctx, ReadRole, delivery.OperatorRole)
	if err != nil {
		return ControlResult{}, err
	}
	reason := strings.TrimSpace(request.Reason)
	if !validToken(request.AttemptID, 256) || !validToken(request.IdempotencyKey, 512) ||
		reason == "" || len(reason) > 1024 {
		return ControlResult{}, ErrInvalidRequest
	}
	operation := delivery.Operation{
		IdempotencyKey: request.IdempotencyKey,
		Principal:      security.Principal,
		Reason:         reason,
	}
	var resultAttemptID string
	switch kind {
	case "replay":
		resultAttemptID, err = s.recovery.Replay(ctx, security.TenantID, request.AttemptID, operation)
	case "resubmit":
		resultAttemptID, err = s.recovery.Resubmit(ctx, security.TenantID, request.AttemptID, operation)
	case "discard":
		resultAttemptID, err = s.recovery.Discard(ctx, security.TenantID, request.AttemptID, operation)
	default:
		return ControlResult{}, ErrInvalidRequest
	}
	if err != nil {
		return ControlResult{}, mapDeliveryError(err)
	}
	attempt, err := s.getAttempt(ctx, security.TenantID, resultAttemptID)
	if err != nil {
		return ControlResult{}, err
	}
	return ControlResult{
		Kind:            kind,
		SourceAttemptID: request.AttemptID,
		ResultAttemptID: resultAttemptID,
		Attempt:         attempt,
		Reason:          reason,
		IdempotencyKey:  request.IdempotencyKey,
		Actor:           summarizePrincipal(security.Principal),
	}, nil
}

// PauseDeployment suspends a deployed integration channel.
func (s *Service) PauseDeployment(ctx context.Context, command DeploymentCommand) (DeploymentSummary, error) {
	return s.changeState(ctx, command, "pause")
}

// ResumeDeployment returns a paused integration channel to service.
func (s *Service) ResumeDeployment(ctx context.Context, command DeploymentCommand) (DeploymentSummary, error) {
	return s.changeState(ctx, command, "resume")
}

// RetireDeployment permanently withdraws an integration revision.
func (s *Service) RetireDeployment(ctx context.Context, command DeploymentCommand) (DeploymentSummary, error) {
	return s.changeState(ctx, command, "retire")
}

// DeployRelease activates the exact published release.
func (s *Service) DeployRelease(ctx context.Context, command DeploymentCommand) (DeploymentSummary, error) {
	return s.changeState(ctx, command, "deploy")
}

func (s *Service) changeState(ctx context.Context, command DeploymentCommand, action string) (DeploymentSummary, error) {
	security, err := s.authorize(ctx, ReadRole, DeploymentOperatorRole)
	if err != nil {
		return DeploymentSummary{}, err
	}
	reason := strings.TrimSpace(command.Reason)
	if !validToken(command.DefinitionID, 256) || !validToken(command.RevisionID, 256) ||
		command.ExpectedVersion <= 0 || reason == "" || len(reason) > 1024 {
		return DeploymentSummary{}, ErrInvalidRequest
	}
	lifecycleCommand := lifecycle.Command{
		TenantID:        security.TenantID,
		DefinitionID:    command.DefinitionID,
		RevisionID:      command.RevisionID,
		ExpectedVersion: command.ExpectedVersion,
		Principal:       security.Principal,
		Reason:          reason,
	}
	var snapshot lifecycle.Snapshot
	switch action {
	case "pause":
		snapshot, err = s.catalog.Pause(ctx, lifecycleCommand)
	case "resume":
		snapshot, err = s.catalog.Resume(ctx, lifecycleCommand)
	case "retire":
		snapshot, err = s.catalog.Retire(ctx, lifecycleCommand)
	case "deploy":
		snapshot, err = s.catalog.Deploy(ctx, lifecycleCommand)
	default:
		return DeploymentSummary{}, ErrInvalidRequest
	}
	if err != nil {
		return DeploymentSummary{}, mapLifecycleError(err)
	}
	return summarizeSnapshot(snapshot), nil
}

func summarizeSnapshot(snapshot lifecycle.Snapshot) DeploymentSummary {
	summary := DeploymentSummary{
		DefinitionRevision: snapshot.DefinitionRevision,
		State:              string(snapshot.State),
		Version:            snapshot.Version,
		ReleaseID:          snapshot.ReleaseID,
		Health:             string(snapshot.Health),
		ValidationPassed:   snapshot.ValidationPassed,
		UpdatedBy:          summarizePrincipal(snapshot.Updated.Principal),
		UpdatedReason:      snapshot.Updated.Reason,
		UpdatedAt:          snapshot.Updated.OccurredAt.UTC(),
	}
	summary.ValidationExpiresAt = optionalTime(snapshot.ValidationExpiresAt)
	return summary
}

func mapDeliveryError(err error) error {
	switch {
	case errors.Is(err, delivery.ErrNotDeadLettered):
		return ErrNotDeadLettered
	case errors.Is(err, delivery.ErrOperationConflict):
		return ErrOperationConflict
	case errors.Is(err, delivery.ErrInvalidOperation):
		return ErrInvalidRequest
	case errors.Is(err, delivery.ErrStoreUnavailable):
		return ErrUnavailable
	default:
		return err
	}
}

func mapLedgerError(err error) error {
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return err
	case errors.Is(err, destination.ErrProvenanceUnavailable):
		return ErrUnavailable
	default:
		return fmt.Errorf("read destination deliveries: %w", err)
	}
}

func mapLifecycleError(err error) error {
	switch {
	case errors.Is(err, lifecycle.ErrNotFound):
		return ErrNotFound
	case errors.Is(err, lifecycle.ErrVersionConflict):
		return ErrVersionConflict
	case errors.Is(err, lifecycle.ErrInvalidTransition):
		return ErrInvalidTransition
	case errors.Is(err, lifecycle.ErrInvalidCommand):
		return ErrInvalidRequest
	case errors.Is(err, lifecycle.ErrUnavailable):
		return ErrUnavailable
	default:
		return err
	}
}
