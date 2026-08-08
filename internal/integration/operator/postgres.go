package operator

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

// PostgresReadStore is the read half of the control plane. Every statement is
// tenant-scoped by a server-owned tenant ID and selects only PHI-minimal
// columns: no raw envelope, no request fingerprint, no execution result
// document, and no canonical event payload value.
type PostgresReadStore struct {
	db *sql.DB
}

// NewPostgresReadStore constructs the control-plane read surface over the
// existing submission database.
func NewPostgresReadStore(db *sql.DB) (*PostgresReadStore, error) {
	if db == nil {
		return nil, ErrUnavailable
	}
	return &PostgresReadStore{db: db}, nil
}

const receiptSelect = `
	SELECT r.tenant_id, r.receipt_id, r.status, r.recorded_at, r.correlation_id,
		r.raw_retention_mode, r.integration_revision, r.principal_json, r.reason,
		(SELECT COUNT(*) FROM integration_canonical_events e
			WHERE e.tenant_id = r.tenant_id AND e.receipt_id = r.receipt_id),
		(SELECT COUNT(*) FROM integration_delivery_attempts a
			WHERE a.tenant_id = r.tenant_id AND a.receipt_id = r.receipt_id),
		(SELECT COUNT(*) FROM integration_delivery_attempts a
			WHERE a.tenant_id = r.tenant_id AND a.receipt_id = r.receipt_id
			  AND a.status = 'failed'),
		(SELECT COUNT(*) FROM integration_delivery_attempts a
			JOIN integration_delivery_dlq d
			  ON d.tenant_id = a.tenant_id AND d.attempt_id = a.attempt_id
			WHERE a.tenant_id = r.tenant_id AND a.receipt_id = r.receipt_id
			  AND d.active = true)
	FROM integration_receipts r
`

// ListReceipts browses durable admissions newest first.
func (s *PostgresReadStore) ListReceipts(
	ctx context.Context,
	tenantID string,
	filter ReceiptFilter,
	request PageRequest,
) (Page[ReceiptSummary], error) {
	if s == nil || s.db == nil || ctx == nil || !validToken(tenantID, 256) {
		return Page[ReceiptSummary]{}, ErrUnavailable
	}
	if err := filter.validate(); err != nil {
		return Page[ReceiptSummary]{}, err
	}
	size, cursorTime, cursorID, err := normalizePage(request)
	if err != nil {
		return Page[ReceiptSummary]{}, err
	}
	rows, err := s.db.QueryContext(ctx, receiptSelect+`
		WHERE r.tenant_id = $1
		  AND ($2 = '' OR r.status = $2)
		  AND ($3 = '' OR r.integration_revision->>'artifact_id' = $3)
		  AND ($4 = '' OR r.correlation_id = $4)
		  AND ($5 = '' OR EXISTS (
				SELECT 1 FROM integration_canonical_events e
				WHERE e.tenant_id = r.tenant_id AND e.receipt_id = r.receipt_id
				  AND e.source_message_id = $5))
		  AND ($6::timestamptz IS NULL OR r.recorded_at >= $6)
		  AND ($7::timestamptz IS NULL OR r.recorded_at <= $7)
		  AND ($8::timestamptz IS NULL OR (r.recorded_at, r.receipt_id) < ($8, $9))
		ORDER BY r.recorded_at DESC, r.receipt_id DESC
		LIMIT $10
	`,
		tenantID, filter.Status, filter.IntegrationArtifactID, filter.CorrelationID,
		filter.SourceMessageID, filter.From, filter.To,
		nullableTime(cursorTime), cursorID, size+1,
	)
	if err != nil {
		return Page[ReceiptSummary]{}, fmt.Errorf("list operator receipts: %w", err)
	}
	defer func() { _ = rows.Close() }()
	items := make([]ReceiptSummary, 0, size)
	for rows.Next() {
		receipt, err := scanReceipt(rows)
		if err != nil {
			return Page[ReceiptSummary]{}, err
		}
		items = append(items, receipt)
	}
	if err := rows.Err(); err != nil {
		return Page[ReceiptSummary]{}, fmt.Errorf("iterate operator receipts: %w", err)
	}
	return paginate(items, size, func(item ReceiptSummary) (time.Time, string) {
		return item.RecordedAt, item.ReceiptID
	}), nil
}

// GetMessageTrace returns one receipt with its events, lineage, delivery
// attempts, and append-only delivery audit.
func (s *PostgresReadStore) GetMessageTrace(ctx context.Context, tenantID, receiptID string) (MessageTrace, error) {
	if s == nil || s.db == nil || ctx == nil || !validToken(tenantID, 256) {
		return MessageTrace{}, ErrUnavailable
	}
	if !validToken(receiptID, 256) {
		return MessageTrace{}, ErrInvalidRequest
	}
	row := s.db.QueryRowContext(ctx, receiptSelect+`
		WHERE r.tenant_id = $1 AND r.receipt_id = $2
	`, tenantID, receiptID)
	receipt, err := scanReceipt(row)
	if errors.Is(err, sql.ErrNoRows) {
		return MessageTrace{}, ErrNotFound
	}
	if err != nil {
		return MessageTrace{}, err
	}
	events, err := s.listEvents(ctx, tenantID, receiptID)
	if err != nil {
		return MessageTrace{}, err
	}
	lineage, err := s.listLineage(ctx, tenantID, receiptID)
	if err != nil {
		return MessageTrace{}, err
	}
	attempts, err := s.listAttemptsForReceipt(ctx, tenantID, receiptID)
	if err != nil {
		return MessageTrace{}, err
	}
	audit, err := s.listReceiptAudit(ctx, tenantID, receiptID)
	if err != nil {
		return MessageTrace{}, err
	}
	return MessageTrace{
		Receipt:  receipt,
		Events:   events,
		Lineage:  lineage,
		Attempts: attempts,
		Audit:    audit,
	}, nil
}

func (s *PostgresReadStore) listEvents(ctx context.Context, tenantID, receiptID string) ([]EventSummary, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT tenant_id, event_id, receipt_id, event_type, source_message_id,
			correlation_id, classification, recorded_at, payload_json
		FROM integration_canonical_events
		WHERE tenant_id = $1 AND receipt_id = $2
		ORDER BY recorded_at, event_id
		LIMIT $3
	`, tenantID, receiptID, MaxPageSize)
	if err != nil {
		return nil, fmt.Errorf("list operator events: %w", err)
	}
	defer func() { _ = rows.Close() }()
	events := make([]EventSummary, 0)
	for rows.Next() {
		var event EventSummary
		var payload []byte
		if err := rows.Scan(
			&event.TenantID, &event.EventID, &event.ReceiptID, &event.EventType,
			&event.SourceMessageID, &event.CorrelationID, &event.Classification,
			&event.RecordedAt, &payload,
		); err != nil {
			return nil, fmt.Errorf("scan operator event: %w", err)
		}
		event.RecordedAt = event.RecordedAt.UTC()
		event.PayloadFields, event.PayloadTruncated = summarizePayload(payload)
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate operator events: %w", err)
	}
	return events, nil
}

func (s *PostgresReadStore) listLineage(ctx context.Context, tenantID, receiptID string) ([]LineageSummary, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT lineage_id, receipt_id, event_id, trace_id, correlation_id,
			source_message_id, artifact_revisions_json, routes_json,
			diagnostics_json, recorded_at
		FROM integration_message_lineage
		WHERE tenant_id = $1 AND receipt_id = $2
		ORDER BY recorded_at, lineage_id
		LIMIT $3
	`, tenantID, receiptID, MaxPageSize)
	if err != nil {
		return nil, fmt.Errorf("list operator lineage: %w", err)
	}
	defer func() { _ = rows.Close() }()
	links := make([]LineageSummary, 0)
	for rows.Next() {
		var link LineageSummary
		var artifactsJSON, routesJSON, diagnosticsJSON []byte
		if err := rows.Scan(
			&link.LineageID, &link.ReceiptID, &link.EventID, &link.TraceID,
			&link.CorrelationID, &link.SourceMessageID, &artifactsJSON,
			&routesJSON, &diagnosticsJSON, &link.RecordedAt,
		); err != nil {
			return nil, fmt.Errorf("scan operator lineage: %w", err)
		}
		link.RecordedAt = link.RecordedAt.UTC()
		if err := json.Unmarshal(artifactsJSON, &link.ArtifactRevisions); err != nil {
			return nil, fmt.Errorf("decode operator lineage artifacts: %w", err)
		}
		var routes []integration.RouteResult
		if err := json.Unmarshal(routesJSON, &routes); err != nil {
			return nil, fmt.Errorf("decode operator lineage routes: %w", err)
		}
		link.Routes = make([]RouteSummary, 0, len(routes))
		for _, route := range routes {
			link.Routes = append(link.Routes, RouteSummary{
				Route:           route.Route,
				Matched:         route.Matched,
				Skipped:         route.Skipped,
				SkipReason:      route.SkipReason,
				TransformCount:  route.TransformCount,
				PlannedActions:  append([]string(nil), route.PlannedActions...),
				DiagnosticCodes: append([]string(nil), route.DiagnosticCodes...),
			})
		}
		var diagnostics []integration.Diagnostic
		if err := json.Unmarshal(diagnosticsJSON, &diagnostics); err != nil {
			return nil, fmt.Errorf("decode operator lineage diagnostics: %w", err)
		}
		link.Diagnostics = make([]DiagnosticSummary, 0, len(diagnostics))
		for _, diagnostic := range diagnostics {
			link.Diagnostics = append(link.Diagnostics, DiagnosticSummary{
				Severity:       string(diagnostic.Severity),
				Stage:          diagnostic.Stage,
				Code:           diagnostic.Code,
				Path:           diagnostic.Path,
				Classification: string(diagnostic.Classification),
			})
		}
		links = append(links, link)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate operator lineage: %w", err)
	}
	return links, nil
}

const attemptSelect = `
	SELECT a.tenant_id, a.attempt_id, COALESCE(a.parent_attempt_id, ''),
		a.receipt_id, a.event_id, a.trace_id, a.destination_revision_json,
		a.route_name, a.action_id, a.status, a.attempt_count, a.recorded_at,
		a.scheduled_at, a.completed_at, a.last_error_code, a.last_error_detail,
		o.status, o.topic, o.lease_owner, o.lease_expires_at,
		d.attempt_id, d.active, d.failure_code, d.failure_detail, d.failed_at,
		d.replay_count, d.last_replayed_at, d.resolution, d.resolved_at
	FROM integration_delivery_attempts a
	JOIN integration_delivery_outbox o
	  ON o.tenant_id = a.tenant_id AND o.attempt_id = a.attempt_id
	LEFT JOIN integration_delivery_dlq d
	  ON d.tenant_id = a.tenant_id AND d.attempt_id = a.attempt_id
`

// ListAttempts browses durable delivery attempts newest first.
func (s *PostgresReadStore) ListAttempts(
	ctx context.Context,
	tenantID string,
	filter AttemptFilter,
	request PageRequest,
) (Page[DeliveryAttemptSummary], error) {
	if s == nil || s.db == nil || ctx == nil || !validToken(tenantID, 256) {
		return Page[DeliveryAttemptSummary]{}, ErrUnavailable
	}
	if err := filter.validate(); err != nil {
		return Page[DeliveryAttemptSummary]{}, err
	}
	size, cursorTime, cursorID, err := normalizePage(request)
	if err != nil {
		return Page[DeliveryAttemptSummary]{}, err
	}
	rows, err := s.db.QueryContext(ctx, attemptSelect+`
		WHERE a.tenant_id = $1
		  AND ($2 = '' OR a.status = $2)
		  AND ($3 = '' OR a.destination_revision_json->>'artifact_id' = $3)
		  AND ($4 = '' OR a.receipt_id = $4)
		  AND ($5 = '' OR a.route_name = $5)
		  AND ($6::timestamptz IS NULL OR a.recorded_at >= $6)
		  AND ($7::timestamptz IS NULL OR a.recorded_at <= $7)
		  AND ($8::timestamptz IS NULL OR (a.recorded_at, a.attempt_id) < ($8, $9))
		ORDER BY a.recorded_at DESC, a.attempt_id DESC
		LIMIT $10
	`,
		tenantID, filter.Status, filter.DestinationArtifactID, filter.ReceiptID,
		filter.Route, filter.From, filter.To,
		nullableTime(cursorTime), cursorID, size+1,
	)
	if err != nil {
		return Page[DeliveryAttemptSummary]{}, fmt.Errorf("list operator attempts: %w", err)
	}
	attempts, err := scanAttempts(rows)
	if err != nil {
		return Page[DeliveryAttemptSummary]{}, err
	}
	return paginate(attempts, size, func(item DeliveryAttemptSummary) (time.Time, string) {
		return item.RecordedAt, item.AttemptID
	}), nil
}

// GetAttempt returns one tenant-scoped delivery attempt.
func (s *PostgresReadStore) GetAttempt(ctx context.Context, tenantID, attemptID string) (DeliveryAttemptSummary, error) {
	if s == nil || s.db == nil || ctx == nil || !validToken(tenantID, 256) {
		return DeliveryAttemptSummary{}, ErrUnavailable
	}
	if !validToken(attemptID, 256) {
		return DeliveryAttemptSummary{}, ErrInvalidRequest
	}
	rows, err := s.db.QueryContext(ctx, attemptSelect+`
		WHERE a.tenant_id = $1 AND a.attempt_id = $2
	`, tenantID, attemptID)
	if err != nil {
		return DeliveryAttemptSummary{}, fmt.Errorf("load operator attempt: %w", err)
	}
	attempts, err := scanAttempts(rows)
	if err != nil {
		return DeliveryAttemptSummary{}, err
	}
	if len(attempts) != 1 {
		return DeliveryAttemptSummary{}, ErrNotFound
	}
	return attempts[0], nil
}

func (s *PostgresReadStore) listAttemptsForReceipt(ctx context.Context, tenantID, receiptID string) ([]DeliveryAttemptSummary, error) {
	rows, err := s.db.QueryContext(ctx, attemptSelect+`
		WHERE a.tenant_id = $1 AND a.receipt_id = $2
		ORDER BY a.recorded_at, a.attempt_id
		LIMIT $3
	`, tenantID, receiptID, MaxPageSize)
	if err != nil {
		return nil, fmt.Errorf("list operator receipt attempts: %w", err)
	}
	return scanAttempts(rows)
}

// ListDeadLetters browses the durable DLQ newest failure first.
func (s *PostgresReadStore) ListDeadLetters(
	ctx context.Context,
	tenantID string,
	activeOnly bool,
	request PageRequest,
) (Page[DeadLetterSummary], error) {
	if s == nil || s.db == nil || ctx == nil || !validToken(tenantID, 256) {
		return Page[DeadLetterSummary]{}, ErrUnavailable
	}
	size, cursorTime, cursorID, err := normalizePage(request)
	if err != nil {
		return Page[DeadLetterSummary]{}, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT attempt_id, active, failure_code, failure_detail, failed_at,
			replay_count, last_replayed_at, resolution, resolved_at
		FROM integration_delivery_dlq
		WHERE tenant_id = $1
		  AND ($2 = false OR active = true)
		  AND ($3::timestamptz IS NULL OR (failed_at, attempt_id) < ($3, $4))
		ORDER BY failed_at DESC, attempt_id DESC
		LIMIT $5
	`, tenantID, activeOnly, nullableTime(cursorTime), cursorID, size+1)
	if err != nil {
		return Page[DeadLetterSummary]{}, fmt.Errorf("list operator dead letters: %w", err)
	}
	defer func() { _ = rows.Close() }()
	items := make([]DeadLetterSummary, 0, size)
	for rows.Next() {
		entry, err := scanDeadLetter(rows)
		if err != nil {
			return Page[DeadLetterSummary]{}, err
		}
		items = append(items, *entry)
	}
	if err := rows.Err(); err != nil {
		return Page[DeadLetterSummary]{}, fmt.Errorf("iterate operator dead letters: %w", err)
	}
	return paginate(items, size, func(item DeadLetterSummary) (time.Time, string) {
		return item.FailedAt, item.AttemptID
	}), nil
}

// ListCircuits returns the bounded destination circuit inventory.
func (s *PostgresReadStore) ListCircuits(ctx context.Context, tenantID string) ([]CircuitSummary, error) {
	if s == nil || s.db == nil || ctx == nil || !validToken(tenantID, 256) {
		return nil, ErrUnavailable
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT destination_artifact_id, destination_revision_id, destination_digest,
			state, consecutive_failures, open_until, updated_at
		FROM integration_delivery_circuits
		WHERE tenant_id = $1
		ORDER BY destination_artifact_id, destination_revision_id
		LIMIT $2
	`, tenantID, MaxPageSize)
	if err != nil {
		return nil, fmt.Errorf("list operator circuits: %w", err)
	}
	defer func() { _ = rows.Close() }()
	circuits := make([]CircuitSummary, 0)
	for rows.Next() {
		var circuit CircuitSummary
		var openUntil sql.NullTime
		if err := rows.Scan(
			&circuit.Destination.ArtifactID, &circuit.Destination.RevisionID,
			&circuit.Destination.Digest, &circuit.State, &circuit.Failures,
			&openUntil, &circuit.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan operator circuit: %w", err)
		}
		circuit.UpdatedAt = circuit.UpdatedAt.UTC()
		if openUntil.Valid {
			circuit.OpenUntil = optionalTime(openUntil.Time)
		}
		circuits = append(circuits, circuit)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate operator circuits: %w", err)
	}
	return circuits, nil
}

const auditSelect = `
	SELECT audit_id, attempt_id, event_kind, attempt_count, principal_json,
		reason, detail_json, recorded_at
	FROM integration_delivery_audit
`

// ListAttemptAudit returns the append-only audit trail for one attempt.
func (s *PostgresReadStore) ListAttemptAudit(
	ctx context.Context,
	tenantID string,
	attemptID string,
	request PageRequest,
) (Page[AuditSummary], error) {
	if s == nil || s.db == nil || ctx == nil || !validToken(tenantID, 256) {
		return Page[AuditSummary]{}, ErrUnavailable
	}
	if !validToken(attemptID, 256) {
		return Page[AuditSummary]{}, ErrInvalidRequest
	}
	size, cursorTime, cursorID, err := normalizePage(request)
	if err != nil {
		return Page[AuditSummary]{}, err
	}
	cursorAuditID := int64(0)
	if cursorID != "" {
		if cursorAuditID, err = strconv.ParseInt(cursorID, 10, 64); err != nil {
			return Page[AuditSummary]{}, ErrInvalidRequest
		}
	}
	rows, err := s.db.QueryContext(ctx, auditSelect+`
		WHERE tenant_id = $1 AND attempt_id = $2
		  AND ($3::timestamptz IS NULL OR (recorded_at, audit_id) < ($3, $4))
		ORDER BY recorded_at DESC, audit_id DESC
		LIMIT $5
	`, tenantID, attemptID, nullableTime(cursorTime), cursorAuditID, size+1)
	if err != nil {
		return Page[AuditSummary]{}, fmt.Errorf("list operator attempt audit: %w", err)
	}
	records, err := scanAudit(rows)
	if err != nil {
		return Page[AuditSummary]{}, err
	}
	return paginate(records, size, func(item AuditSummary) (time.Time, string) {
		return item.RecordedAt, fmt.Sprintf("%d", item.AuditID)
	}), nil
}

func (s *PostgresReadStore) listReceiptAudit(ctx context.Context, tenantID, receiptID string) ([]AuditSummary, error) {
	rows, err := s.db.QueryContext(ctx, auditSelect+`
		WHERE tenant_id = $1 AND attempt_id IN (
			SELECT attempt_id FROM integration_delivery_attempts
			WHERE tenant_id = $1 AND receipt_id = $2
		)
		ORDER BY recorded_at DESC, audit_id DESC
		LIMIT $3
	`, tenantID, receiptID, MaxPageSize)
	if err != nil {
		return nil, fmt.Errorf("list operator receipt audit: %w", err)
	}
	return scanAudit(rows)
}

func scanReceipt(row interface{ Scan(...any) error }) (ReceiptSummary, error) {
	var receipt ReceiptSummary
	var revisionJSON, principalJSON []byte
	if err := row.Scan(
		&receipt.TenantID, &receipt.ReceiptID, &receipt.Status, &receipt.RecordedAt,
		&receipt.CorrelationID, &receipt.RawRetentionMode, &revisionJSON,
		&principalJSON, &receipt.Reason, &receipt.EventCount, &receipt.AttemptCount,
		&receipt.FailedAttemptCount, &receipt.DeadLetterCount,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ReceiptSummary{}, err
		}
		return ReceiptSummary{}, fmt.Errorf("scan operator receipt: %w", err)
	}
	receipt.RecordedAt = receipt.RecordedAt.UTC()
	if err := json.Unmarshal(revisionJSON, &receipt.IntegrationRevision); err != nil {
		return ReceiptSummary{}, fmt.Errorf("decode operator receipt revision: %w", err)
	}
	var principal integration.Principal
	if err := json.Unmarshal(principalJSON, &principal); err != nil {
		return ReceiptSummary{}, fmt.Errorf("decode operator receipt principal: %w", err)
	}
	receipt.Principal = summarizePrincipal(principal)
	return receipt, nil
}

func scanAttempts(rows *sql.Rows) ([]DeliveryAttemptSummary, error) {
	defer func() { _ = rows.Close() }()
	attempts := make([]DeliveryAttemptSummary, 0)
	for rows.Next() {
		var attempt DeliveryAttemptSummary
		var destinationJSON []byte
		var completedAt, leaseExpiresAt sql.NullTime
		var dlqAttemptID, dlqFailureCode, dlqFailureDetail, dlqResolution sql.NullString
		var dlqActive sql.NullBool
		var dlqFailedAt, dlqLastReplayedAt, dlqResolvedAt sql.NullTime
		var dlqReplayCount sql.NullInt64
		if err := rows.Scan(
			&attempt.TenantID, &attempt.AttemptID, &attempt.ParentAttemptID,
			&attempt.ReceiptID, &attempt.EventID, &attempt.TraceID, &destinationJSON,
			&attempt.Route, &attempt.Action, &attempt.Status, &attempt.AttemptCount,
			&attempt.RecordedAt, &attempt.ScheduledAt, &completedAt,
			&attempt.LastErrorCode, &attempt.LastErrorDetail,
			&attempt.OutboxStatus, &attempt.Topic, &attempt.LeaseOwner, &leaseExpiresAt,
			&dlqAttemptID, &dlqActive, &dlqFailureCode, &dlqFailureDetail, &dlqFailedAt,
			&dlqReplayCount, &dlqLastReplayedAt, &dlqResolution, &dlqResolvedAt,
		); err != nil {
			return nil, fmt.Errorf("scan operator attempt: %w", err)
		}
		attempt.RecordedAt = attempt.RecordedAt.UTC()
		attempt.ScheduledAt = attempt.ScheduledAt.UTC()
		if completedAt.Valid {
			attempt.CompletedAt = optionalTime(completedAt.Time)
		}
		if leaseExpiresAt.Valid {
			attempt.LeaseExpiresAt = optionalTime(leaseExpiresAt.Time)
		}
		if err := json.Unmarshal(destinationJSON, &attempt.Destination); err != nil {
			return nil, fmt.Errorf("decode operator attempt destination: %w", err)
		}
		if dlqAttemptID.Valid {
			entry := DeadLetterSummary{
				AttemptID:     dlqAttemptID.String,
				Active:        dlqActive.Bool,
				FailureCode:   dlqFailureCode.String,
				FailureDetail: dlqFailureDetail.String,
				FailedAt:      dlqFailedAt.Time.UTC(),
				ReplayCount:   int(dlqReplayCount.Int64),
				Resolution:    dlqResolution.String,
			}
			if dlqLastReplayedAt.Valid {
				entry.LastReplayedAt = optionalTime(dlqLastReplayedAt.Time)
			}
			if dlqResolvedAt.Valid {
				entry.ResolvedAt = optionalTime(dlqResolvedAt.Time)
			}
			attempt.DeadLetter = &entry
		}
		attempts = append(attempts, attempt)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate operator attempts: %w", err)
	}
	return attempts, nil
}

func scanDeadLetter(rows *sql.Rows) (*DeadLetterSummary, error) {
	var entry DeadLetterSummary
	var lastReplayedAt, resolvedAt sql.NullTime
	if err := rows.Scan(
		&entry.AttemptID, &entry.Active, &entry.FailureCode, &entry.FailureDetail,
		&entry.FailedAt, &entry.ReplayCount, &lastReplayedAt, &entry.Resolution,
		&resolvedAt,
	); err != nil {
		return nil, fmt.Errorf("scan operator dead letter: %w", err)
	}
	entry.FailedAt = entry.FailedAt.UTC()
	if lastReplayedAt.Valid {
		entry.LastReplayedAt = optionalTime(lastReplayedAt.Time)
	}
	if resolvedAt.Valid {
		entry.ResolvedAt = optionalTime(resolvedAt.Time)
	}
	return &entry, nil
}

func scanAudit(rows *sql.Rows) ([]AuditSummary, error) {
	defer func() { _ = rows.Close() }()
	records := make([]AuditSummary, 0)
	for rows.Next() {
		var record AuditSummary
		var principalJSON, detailJSON []byte
		if err := rows.Scan(
			&record.AuditID, &record.AttemptID, &record.EventKind, &record.AttemptCount,
			&principalJSON, &record.Reason, &detailJSON, &record.RecordedAt,
		); err != nil {
			return nil, fmt.Errorf("scan operator audit: %w", err)
		}
		record.RecordedAt = record.RecordedAt.UTC()
		var principal integration.Principal
		if err := json.Unmarshal(principalJSON, &principal); err != nil {
			return nil, fmt.Errorf("decode operator audit principal: %w", err)
		}
		record.Principal = summarizePrincipal(principal)
		record.Detail = boundedDetail(detailJSON)
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate operator audit: %w", err)
	}
	return records, nil
}

func paginate[T any](items []T, size int, key func(T) (time.Time, string)) Page[T] {
	page := Page[T]{Items: items}
	if len(items) > size {
		page.Items = items[:size]
		page.HasMore = true
	}
	if page.HasMore && len(page.Items) > 0 {
		sortTime, id := key(page.Items[len(page.Items)-1])
		page.NextCursor = encodeCursor(sortTime, id)
	}
	return page
}

func nullableTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value.UTC()
}

func (f ReceiptFilter) validate() error {
	if !optionalToken(f.Status, 32) || !optionalToken(f.IntegrationArtifactID, 256) ||
		!optionalToken(f.CorrelationID, 256) || !optionalToken(f.SourceMessageID, 256) {
		return ErrInvalidRequest
	}
	if f.Status != "" && f.Status != "accepted" && f.Status != "rejected" {
		return ErrInvalidRequest
	}
	return validWindow(f.From, f.To)
}

func (f AttemptFilter) validate() error {
	if !optionalToken(f.Status, 32) || !optionalToken(f.DestinationArtifactID, 256) ||
		!optionalToken(f.ReceiptID, 256) || !optionalToken(f.Route, 256) {
		return ErrInvalidRequest
	}
	switch f.Status {
	case "", "queued", "succeeded", "failed":
	default:
		return ErrInvalidRequest
	}
	return validWindow(f.From, f.To)
}

func validWindow(from, to *time.Time) error {
	if from != nil && to != nil && to.Before(*from) {
		return ErrInvalidRequest
	}
	return nil
}
