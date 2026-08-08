package delivery

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

const (
	resubmitAttemptDomain = "fi-fhir/delivery-resubmit-attempt/v1\x00"
	resubmitOutboxDomain  = "fi-fhir/delivery-resubmit-outbox/v1\x00"
	operationIDDomain     = "fi-fhir/delivery-operation/v1\x00"
)

// PostgresStore owns durable claiming, retry, DLQ, and operator recovery.
type PostgresStore struct {
	db    *sql.DB
	clock func() time.Time
}

// NewPostgresStore constructs a delivery store over the submission database.
func NewPostgresStore(db *sql.DB, clock func() time.Time) (*PostgresStore, error) {
	if db == nil {
		return nil, ErrStoreUnavailable
	}
	if clock == nil {
		clock = time.Now
	}
	return &PostgresStore{db: db, clock: clock}, nil
}

// Claim leases one due item. A nil item means no eligible work is available.
func (s *PostgresStore) Claim(ctx context.Context, workerID string, leaseDuration time.Duration) (*WorkItem, error) {
	if s == nil || s.db == nil || ctx == nil || !validToken(workerID, 128) || leaseDuration <= 0 {
		return nil, ErrStoreUnavailable
	}
	now := s.clock().UTC()
	leaseExpiresAt := now.Add(leaseDuration)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin delivery claim: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `
		WITH reclaimed AS (
			UPDATE integration_delivery_outbox
			SET status = 'pending', lease_owner = '', lease_expires_at = NULL,
				updated_at = $1
			WHERE status = 'leased' AND lease_expires_at <= $1
			RETURNING tenant_id, attempt_id
		)
		INSERT INTO integration_delivery_audit (
			tenant_id, attempt_id, event_kind, attempt_count,
			principal_json, detail_json, recorded_at
		)
		SELECT r.tenant_id, r.attempt_id, 'lease_reclaimed', a.attempt_count,
			'{}'::jsonb, '{}'::jsonb, $1
		FROM reclaimed r
		JOIN integration_delivery_attempts a
		  ON a.tenant_id = r.tenant_id AND a.attempt_id = r.attempt_id
	`, now); err != nil {
		return nil, fmt.Errorf("reclaim expired delivery leases: %w", err)
	}

	row := tx.QueryRowContext(ctx, `
		WITH candidate AS (
			SELECT o.tenant_id, o.outbox_id
			FROM integration_delivery_outbox o
			JOIN integration_delivery_attempts a
			  ON a.tenant_id = o.tenant_id AND a.attempt_id = o.attempt_id
			WHERE o.status = 'pending'
			  AND a.status = 'queued'
			  AND o.scheduled_at <= $1
			  AND NOT EXISTS (
				SELECT 1
				FROM integration_delivery_circuits c
				WHERE c.tenant_id = a.tenant_id
				  AND c.destination_artifact_id = a.destination_revision_json->>'artifact_id'
				  AND c.destination_revision_id = a.destination_revision_json->>'revision_id'
				  AND c.destination_digest = a.destination_revision_json->>'digest'
				  AND c.state = 'open'
				  AND c.open_until > $1
			  )
			ORDER BY o.scheduled_at, o.tenant_id, o.outbox_id
			FOR UPDATE OF o SKIP LOCKED
			LIMIT 1
		), claimed AS (
			UPDATE integration_delivery_outbox o
			SET status = 'leased', lease_owner = $2, lease_expires_at = $3,
				updated_at = $1
			FROM candidate c
			WHERE o.tenant_id = c.tenant_id AND o.outbox_id = c.outbox_id
			RETURNING o.*
		)
		SELECT c.tenant_id, c.outbox_id, c.attempt_id, a.receipt_id,
			a.event_id, a.trace_id, c.topic, a.route_name, a.action_id,
			a.attempt_count, a.destination_revision_json, e.payload_json,
			c.lease_owner, c.lease_expires_at
		FROM claimed c
		JOIN integration_delivery_attempts a
		  ON a.tenant_id = c.tenant_id AND a.attempt_id = c.attempt_id
		JOIN integration_canonical_events e
		  ON e.tenant_id = a.tenant_id AND e.event_id = a.event_id
	`, now, workerID, leaseExpiresAt)

	var item WorkItem
	var destinationJSON []byte
	if err := row.Scan(
		&item.TenantID,
		&item.OutboxID,
		&item.AttemptID,
		&item.ReceiptID,
		&item.EventID,
		&item.TraceID,
		&item.Topic,
		&item.Route,
		&item.Action,
		&item.AttemptCount,
		&destinationJSON,
		&item.EventPayload,
		&item.LeaseOwner,
		&item.LeaseExpiresAt,
	); errors.Is(err, sql.ErrNoRows) {
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("commit empty delivery claim: %w", err)
		}
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("claim due delivery: %w", err)
	}
	if err := json.Unmarshal(destinationJSON, &item.Destination); err != nil {
		return nil, fmt.Errorf("decode delivery destination revision: %w", err)
	}
	detailJSON, err := json.Marshal(map[string]string{"worker_id": workerID})
	if err != nil {
		return nil, fmt.Errorf("encode delivery claim audit: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO integration_delivery_audit (
			tenant_id, attempt_id, event_kind, attempt_count,
			principal_json, detail_json, recorded_at
		) VALUES ($1, $2, 'claimed', $3, '{}', $4, $5)
	`, item.TenantID, item.AttemptID, item.AttemptCount, detailJSON, now); err != nil {
		return nil, fmt.Errorf("audit delivery claim: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit delivery claim: %w", err)
	}
	return &item, nil
}

// MarkPublished completes one live lease and closes its destination circuit.
func (s *PostgresStore) MarkPublished(ctx context.Context, item WorkItem) error {
	if s == nil || s.db == nil || ctx == nil {
		return ErrStoreUnavailable
	}
	now := s.clock().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin delivery completion: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `
		UPDATE integration_delivery_outbox
		SET status = 'published', lease_owner = '', lease_expires_at = NULL,
			published_at = $1, updated_at = $1
		WHERE tenant_id = $2 AND outbox_id = $3 AND attempt_id = $4
		  AND status = 'leased' AND lease_owner = $5 AND lease_expires_at > $1
	`, now, item.TenantID, item.OutboxID, item.AttemptID, item.LeaseOwner)
	if err != nil {
		return fmt.Errorf("complete delivery outbox: %w", err)
	}
	if affected, err := result.RowsAffected(); err != nil || affected != 1 {
		return ErrLeaseLost
	}
	attemptResult, err := tx.ExecContext(ctx, `
		UPDATE integration_delivery_attempts
		SET status = 'succeeded', completed_at = $1,
			last_error_code = '', last_error_detail = ''
		WHERE tenant_id = $2 AND attempt_id = $3 AND status = 'queued'
	`, now, item.TenantID, item.AttemptID)
	if err != nil {
		return fmt.Errorf("complete delivery attempt: %w", err)
	}
	if affected, err := attemptResult.RowsAffected(); err != nil || affected != 1 {
		return ErrLeaseLost
	}
	if err := closeCircuit(ctx, tx, item.TenantID, item.Destination, now); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO integration_delivery_audit (
			tenant_id, attempt_id, event_kind, attempt_count,
			principal_json, detail_json, recorded_at
		) VALUES ($1, $2, 'published', $3, '{}', '{}', $4)
	`, item.TenantID, item.AttemptID, item.AttemptCount, now); err != nil {
		return fmt.Errorf("audit published delivery: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit delivery completion: %w", err)
	}
	return nil
}

// MarkFailed schedules a bounded retry or atomically enters the DLQ.
func (s *PostgresStore) MarkFailed(ctx context.Context, item WorkItem, failure Failure, config Config) (bool, error) {
	if s == nil || s.db == nil || ctx == nil || !validFailure(failure) || config.validate() != nil {
		return false, ErrStoreUnavailable
	}
	now := s.clock().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin delivery failure: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var attemptCount int
	var destinationJSON []byte
	if err := tx.QueryRowContext(ctx, `
		SELECT a.attempt_count, a.destination_revision_json
		FROM integration_delivery_outbox o
		JOIN integration_delivery_attempts a
		  ON a.tenant_id = o.tenant_id AND a.attempt_id = o.attempt_id
		WHERE o.tenant_id = $1 AND o.outbox_id = $2 AND o.attempt_id = $3
		  AND o.status = 'leased' AND o.lease_owner = $4
		  AND o.lease_expires_at > $5 AND a.status = 'queued'
		FOR UPDATE OF o, a
	`, item.TenantID, item.OutboxID, item.AttemptID, item.LeaseOwner, now).Scan(&attemptCount, &destinationJSON); errors.Is(err, sql.ErrNoRows) {
		return false, ErrLeaseLost
	} else if err != nil {
		return false, fmt.Errorf("lock failed delivery: %w", err)
	}
	var destination integration.DestinationRevisionRef
	if err := json.Unmarshal(destinationJSON, &destination); err != nil {
		return false, fmt.Errorf("decode failed delivery destination: %w", err)
	}

	if failure.Retryable && attemptCount < config.MaxAttempts {
		nextAttemptCount := attemptCount + 1
		scheduledAt := now.Add(retryDelay(attemptCount, config))
		openUntil, err := recordCircuitFailure(ctx, tx, item.TenantID, destination, now, config)
		if err != nil {
			return false, err
		}
		if openUntil.After(scheduledAt) {
			scheduledAt = openUntil
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE integration_delivery_attempts
			SET attempt_count = $1, scheduled_at = $2,
				last_error_code = $3, last_error_detail = $4
			WHERE tenant_id = $5 AND attempt_id = $6
		`, nextAttemptCount, scheduledAt, failure.Code, failure.Detail,
			item.TenantID, item.AttemptID); err != nil {
			return false, fmt.Errorf("schedule delivery attempt retry: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE integration_delivery_outbox
			SET status = 'pending', lease_owner = '', lease_expires_at = NULL,
				scheduled_at = $1, updated_at = $2
			WHERE tenant_id = $3 AND outbox_id = $4
		`, scheduledAt, now, item.TenantID, item.OutboxID); err != nil {
			return false, fmt.Errorf("schedule delivery outbox retry: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO integration_delivery_audit (
				tenant_id, attempt_id, event_kind, attempt_count,
				principal_json, detail_json, recorded_at
			) VALUES ($1, $2, 'retry_scheduled', $3, '{}',
				jsonb_build_object('code', $4::text, 'scheduled_at', $5::timestamptz), $6)
		`, item.TenantID, item.AttemptID, nextAttemptCount, failure.Code, scheduledAt, now); err != nil {
			return false, fmt.Errorf("audit delivery retry: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return false, fmt.Errorf("commit delivery retry: %w", err)
		}
		return true, nil
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE integration_delivery_attempts
		SET status = 'failed', completed_at = $1,
			last_error_code = $2, last_error_detail = $3
		WHERE tenant_id = $4 AND attempt_id = $5
	`, now, failure.Code, failure.Detail, item.TenantID, item.AttemptID); err != nil {
		return false, fmt.Errorf("fail delivery attempt: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE integration_delivery_outbox
		SET status = 'failed', lease_owner = '', lease_expires_at = NULL,
			updated_at = $1
		WHERE tenant_id = $2 AND outbox_id = $3
	`, now, item.TenantID, item.OutboxID); err != nil {
		return false, fmt.Errorf("fail delivery outbox: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO integration_delivery_dlq (
			tenant_id, attempt_id, outbox_id, failure_code,
			failure_detail, failed_at, active, resolution, resolved_at
		) VALUES ($1, $2, $3, $4, $5, $6, true, '', NULL)
		ON CONFLICT (tenant_id, attempt_id) DO UPDATE
		SET failure_code = EXCLUDED.failure_code,
			failure_detail = EXCLUDED.failure_detail,
			failed_at = EXCLUDED.failed_at,
			active = true,
			resolution = '',
			resolved_at = NULL
	`, item.TenantID, item.AttemptID, item.OutboxID, failure.Code, failure.Detail, now); err != nil {
		return false, fmt.Errorf("dead-letter delivery: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO integration_delivery_audit (
			tenant_id, attempt_id, event_kind, attempt_count,
			principal_json, detail_json, recorded_at
		) VALUES ($1, $2, 'dlq_entered', $3, '{}',
			jsonb_build_object('code', $4::text), $5)
	`, item.TenantID, item.AttemptID, attemptCount, failure.Code, now); err != nil {
		return false, fmt.Errorf("audit dead-letter delivery: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit dead-letter delivery: %w", err)
	}
	return false, nil
}

// Replay requeues the exact failed attempt once for an idempotent operation.
func (s *PostgresStore) Replay(ctx context.Context, tenantID, attemptID string, operation Operation) (string, error) {
	return s.recover(ctx, tenantID, attemptID, operation, false)
}

// Resubmit creates one idempotent child attempt from a failed attempt.
func (s *PostgresStore) Resubmit(ctx context.Context, tenantID, attemptID string, operation Operation) (string, error) {
	return s.recover(ctx, tenantID, attemptID, operation, true)
}

func (s *PostgresStore) recover(
	ctx context.Context,
	tenantID string,
	attemptID string,
	operation Operation,
	resubmit bool,
) (string, error) {
	if s == nil || s.db == nil || ctx == nil || !validToken(tenantID, 256) ||
		!validToken(attemptID, 256) || !validOperation(operation) {
		return "", ErrInvalidOperation
	}
	kind, resolution := "replay", "replayed"
	if resubmit {
		kind, resolution = "resubmit", "resubmitted"
	}
	now := s.clock().UTC()
	principalJSON, err := json.Marshal(operation.Principal)
	if err != nil {
		return "", ErrInvalidOperation
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("begin delivery %s: %w", kind, err)
	}
	defer func() { _ = tx.Rollback() }()

	if existing, err := loadOperation(ctx, tx, tenantID, operation.IdempotencyKey, kind, attemptID); err != nil {
		return "", err
	} else if existing != "" {
		if err := tx.Commit(); err != nil {
			return "", fmt.Errorf("commit duplicate delivery %s: %w", kind, err)
		}
		return existing, nil
	}

	var sourceAttempt deliveryAttemptRow
	if err := tx.QueryRowContext(ctx, `
		SELECT a.receipt_id, a.event_id, a.trace_id, a.destination_revision_json,
			a.route_name, a.action_id, a.attempt_count, o.outbox_id, o.topic,
			o.payload_json
		FROM integration_delivery_attempts a
		JOIN integration_delivery_outbox o
		  ON o.tenant_id = a.tenant_id AND o.attempt_id = a.attempt_id
		JOIN integration_delivery_dlq d
		  ON d.tenant_id = a.tenant_id AND d.attempt_id = a.attempt_id
		WHERE a.tenant_id = $1 AND a.attempt_id = $2
		  AND a.status = 'failed' AND o.status = 'failed' AND d.active = true
		FOR UPDATE OF a, o, d
	`, tenantID, attemptID).Scan(
		&sourceAttempt.receiptID,
		&sourceAttempt.eventID,
		&sourceAttempt.traceID,
		&sourceAttempt.destinationJSON,
		&sourceAttempt.route,
		&sourceAttempt.action,
		&sourceAttempt.attemptCount,
		&sourceAttempt.outboxID,
		&sourceAttempt.topic,
		&sourceAttempt.payloadJSON,
	); errors.Is(err, sql.ErrNoRows) {
		if existing, loadErr := loadOperation(ctx, tx, tenantID, operation.IdempotencyKey, kind, attemptID); loadErr != nil {
			return "", loadErr
		} else if existing != "" {
			return existing, tx.Commit()
		}
		return "", ErrNotDeadLettered
	} else if err != nil {
		return "", fmt.Errorf("lock dead-lettered delivery: %w", err)
	}
	var destination integration.DestinationRevisionRef
	if err := json.Unmarshal(sourceAttempt.destinationJSON, &destination); err != nil {
		return "", fmt.Errorf("decode recovery destination revision: %w", err)
	}

	resultAttemptID := attemptID
	if resubmit {
		resultAttemptID = deterministicID(resubmitAttemptDomain, tenantID, attemptID, operation.IdempotencyKey)
		resultOutboxID := deterministicID(resubmitOutboxDomain, tenantID, resultAttemptID)
		payloadJSON, err := resubmitPayload(sourceAttempt.payloadJSON, resultAttemptID)
		if err != nil {
			return "", err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO integration_delivery_attempts (
				tenant_id, attempt_id, receipt_id, event_id, trace_id,
				destination_revision_json, route_name, action_id, status,
				attempt_count, recorded_at, parent_attempt_id, scheduled_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'queued', 1, $9, $10, $9)
		`, tenantID, resultAttemptID, sourceAttempt.receiptID, sourceAttempt.eventID,
			sourceAttempt.traceID, sourceAttempt.destinationJSON, sourceAttempt.route,
			sourceAttempt.action, now, attemptID); err != nil {
			return "", fmt.Errorf("insert resubmitted delivery attempt: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO integration_delivery_outbox (
				tenant_id, outbox_id, attempt_id, topic, status, payload_json,
				created_at, scheduled_at, updated_at
			) VALUES ($1, $2, $3, $4, 'pending', $5, $6, $6, $6)
		`, tenantID, resultOutboxID, resultAttemptID, sourceAttempt.topic, payloadJSON, now); err != nil {
			return "", fmt.Errorf("insert resubmitted delivery outbox: %w", err)
		}
	} else {
		if _, err := tx.ExecContext(ctx, `
			UPDATE integration_delivery_attempts
			SET status = 'queued', attempt_count = 1, scheduled_at = $1,
				completed_at = NULL, last_error_code = '', last_error_detail = ''
			WHERE tenant_id = $2 AND attempt_id = $3
		`, now, tenantID, attemptID); err != nil {
			return "", fmt.Errorf("requeue replayed delivery attempt: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE integration_delivery_outbox
			SET status = 'pending', scheduled_at = $1, published_at = NULL,
				lease_owner = '', lease_expires_at = NULL, updated_at = $1
			WHERE tenant_id = $2 AND outbox_id = $3
		`, now, tenantID, sourceAttempt.outboxID); err != nil {
			return "", fmt.Errorf("requeue replayed delivery outbox: %w", err)
		}
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE integration_delivery_dlq
		SET active = false, replay_count = replay_count + 1, last_replayed_at = $1,
			resolution = $4, resolved_at = $1
		WHERE tenant_id = $2 AND attempt_id = $3
	`, now, tenantID, attemptID, resolution); err != nil {
		return "", fmt.Errorf("resolve replayed dead letter: %w", err)
	}
	if err := closeCircuit(ctx, tx, tenantID, destination, now); err != nil {
		return "", err
	}
	operationID := deterministicID(operationIDDomain, tenantID, operation.IdempotencyKey)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO integration_delivery_operations (
			tenant_id, operation_id, idempotency_key, operation_kind,
			source_attempt_id, result_attempt_id, principal_json, reason, recorded_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, tenantID, operationID, operation.IdempotencyKey, kind, attemptID,
		resultAttemptID, principalJSON, operation.Reason, now); err != nil {
		return "", fmt.Errorf("record delivery %s operation: %w", kind, err)
	}
	eventKind := "replayed"
	if resubmit {
		eventKind = "resubmitted"
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO integration_delivery_audit (
			tenant_id, attempt_id, event_kind, attempt_count,
			principal_json, reason, detail_json, recorded_at
		) VALUES ($1, $2, $3, 1, $4, $5,
			jsonb_build_object('source_attempt_id', $6::text), $7)
	`, tenantID, resultAttemptID, eventKind, principalJSON, operation.Reason, attemptID, now); err != nil {
		return "", fmt.Errorf("audit delivery %s: %w", kind, err)
	}
	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("commit delivery %s: %w", kind, err)
	}
	return resultAttemptID, nil
}

// Discard closes one active dead letter without requeueing it. It reuses the
// same idempotent operation ledger and append-only audit trail as replay and
// resubmit so an abandoned message is still attributable to an actor, a
// reason, and one operation key.
func (s *PostgresStore) Discard(ctx context.Context, tenantID, attemptID string, operation Operation) (string, error) {
	if s == nil || s.db == nil || ctx == nil || !validToken(tenantID, 256) ||
		!validToken(attemptID, 256) || !validOperation(operation) {
		return "", ErrInvalidOperation
	}
	now := s.clock().UTC()
	principalJSON, err := json.Marshal(operation.Principal)
	if err != nil {
		return "", ErrInvalidOperation
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("begin delivery discard: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if existing, err := loadOperation(ctx, tx, tenantID, operation.IdempotencyKey, "discard", attemptID); err != nil {
		return "", err
	} else if existing != "" {
		if err := tx.Commit(); err != nil {
			return "", fmt.Errorf("commit duplicate delivery discard: %w", err)
		}
		return existing, nil
	}

	var attemptCount int
	if err := tx.QueryRowContext(ctx, `
		SELECT a.attempt_count
		FROM integration_delivery_attempts a
		JOIN integration_delivery_dlq d
		  ON d.tenant_id = a.tenant_id AND d.attempt_id = a.attempt_id
		WHERE a.tenant_id = $1 AND a.attempt_id = $2
		  AND a.status = 'failed' AND d.active = true
		FOR UPDATE OF a, d
	`, tenantID, attemptID).Scan(&attemptCount); errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotDeadLettered
	} else if err != nil {
		return "", fmt.Errorf("lock discarded dead letter: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE integration_delivery_dlq
		SET active = false, resolution = 'discarded', resolved_at = $1
		WHERE tenant_id = $2 AND attempt_id = $3
	`, now, tenantID, attemptID); err != nil {
		return "", fmt.Errorf("discard dead letter: %w", err)
	}
	operationID := deterministicID(operationIDDomain, tenantID, operation.IdempotencyKey)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO integration_delivery_operations (
			tenant_id, operation_id, idempotency_key, operation_kind,
			source_attempt_id, result_attempt_id, principal_json, reason, recorded_at
		) VALUES ($1, $2, $3, 'discard', $4, $4, $5, $6, $7)
	`, tenantID, operationID, operation.IdempotencyKey, attemptID,
		principalJSON, operation.Reason, now); err != nil {
		return "", fmt.Errorf("record delivery discard operation: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO integration_delivery_audit (
			tenant_id, attempt_id, event_kind, attempt_count,
			principal_json, reason, detail_json, recorded_at
		) VALUES ($1, $2, 'discarded', $3, $4, $5, '{}', $6)
	`, tenantID, attemptID, attemptCount, principalJSON, operation.Reason, now); err != nil {
		return "", fmt.Errorf("audit delivery discard: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("commit delivery discard: %w", err)
	}
	return attemptID, nil
}

type deliveryAttemptRow struct {
	receiptID       string
	eventID         string
	traceID         string
	destinationJSON []byte
	route           string
	action          string
	attemptCount    int
	outboxID        string
	topic           string
	payloadJSON     []byte
}

func loadOperation(
	ctx context.Context,
	tx *sql.Tx,
	tenantID string,
	idempotencyKey string,
	kind string,
	sourceAttemptID string,
) (string, error) {
	var storedKind, storedSource, result string
	err := tx.QueryRowContext(ctx, `
		SELECT operation_kind, source_attempt_id, result_attempt_id
		FROM integration_delivery_operations
		WHERE tenant_id = $1 AND idempotency_key = $2
	`, tenantID, idempotencyKey).Scan(&storedKind, &storedSource, &result)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("load delivery operation: %w", err)
	}
	if storedKind != kind || storedSource != sourceAttemptID {
		return "", ErrOperationConflict
	}
	return result, nil
}

func recordCircuitFailure(
	ctx context.Context,
	tx *sql.Tx,
	tenantID string,
	destination integration.DestinationRevisionRef,
	now time.Time,
	config Config,
) (time.Time, error) {
	var failures int
	var openUntil sql.NullTime
	err := tx.QueryRowContext(ctx, `
		INSERT INTO integration_delivery_circuits (
			tenant_id, destination_artifact_id, destination_revision_id,
			destination_digest, state, consecutive_failures, open_until, updated_at
		) VALUES ($1, $2, $3, $4, 'closed', 1, NULL, $5)
		ON CONFLICT (
			tenant_id, destination_artifact_id, destination_revision_id, destination_digest
		) DO UPDATE
		SET consecutive_failures = integration_delivery_circuits.consecutive_failures + 1,
			updated_at = EXCLUDED.updated_at
		RETURNING consecutive_failures, open_until
	`, tenantID, destination.ArtifactID, destination.RevisionID, destination.Digest, now).Scan(&failures, &openUntil)
	if err != nil {
		return time.Time{}, fmt.Errorf("record delivery circuit failure: %w", err)
	}
	if failures < config.CircuitFailureThreshold {
		return time.Time{}, nil
	}
	nextOpenUntil := now.Add(config.CircuitOpenDuration)
	if openUntil.Valid && openUntil.Time.After(nextOpenUntil) {
		nextOpenUntil = openUntil.Time
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE integration_delivery_circuits
		SET state = 'open', open_until = $1, updated_at = $2
		WHERE tenant_id = $3 AND destination_artifact_id = $4
		  AND destination_revision_id = $5 AND destination_digest = $6
	`, nextOpenUntil, now, tenantID, destination.ArtifactID, destination.RevisionID, destination.Digest); err != nil {
		return time.Time{}, fmt.Errorf("open delivery circuit: %w", err)
	}
	return nextOpenUntil, nil
}

func closeCircuit(
	ctx context.Context,
	tx *sql.Tx,
	tenantID string,
	destination integration.DestinationRevisionRef,
	now time.Time,
) error {
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO integration_delivery_circuits (
			tenant_id, destination_artifact_id, destination_revision_id,
			destination_digest, state, consecutive_failures, open_until, updated_at
		) VALUES ($1, $2, $3, $4, 'closed', 0, NULL, $5)
		ON CONFLICT (
			tenant_id, destination_artifact_id, destination_revision_id, destination_digest
		) DO UPDATE
		SET state = 'closed', consecutive_failures = 0,
			open_until = NULL, updated_at = EXCLUDED.updated_at
	`, tenantID, destination.ArtifactID, destination.RevisionID, destination.Digest, now); err != nil {
		return fmt.Errorf("close delivery circuit: %w", err)
	}
	return nil
}

func retryDelay(attemptCount int, config Config) time.Duration {
	delay := config.RetryBaseDelay
	for count := 1; count < attemptCount && delay < config.RetryMaxDelay; count++ {
		if delay > config.RetryMaxDelay/2 {
			return config.RetryMaxDelay
		}
		delay *= 2
	}
	if delay > config.RetryMaxDelay {
		return config.RetryMaxDelay
	}
	return delay
}

func validOperation(operation Operation) bool {
	if !validToken(operation.IdempotencyKey, 512) ||
		len(operation.Reason) == 0 || len(operation.Reason) > 1024 ||
		strings.TrimSpace(operation.Reason) != operation.Reason ||
		!validToken(operation.Principal.ID, 256) ||
		!validToken(operation.Principal.AuthMethod, 128) ||
		(operation.Principal.Kind != integration.PrincipalKindHuman &&
			operation.Principal.Kind != integration.PrincipalKindService) {
		return false
	}
	for _, role := range operation.Principal.Roles {
		if role == OperatorRole {
			return true
		}
	}
	return false
}

func validFailure(failure Failure) bool {
	return validToken(failure.Code, 128) && len(failure.Detail) > 0 &&
		len(failure.Detail) <= 512 && strings.TrimSpace(failure.Detail) == failure.Detail
}

func validToken(value string, maxBytes int) bool {
	if len(value) == 0 || len(value) > maxBytes || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func deterministicID(domain string, values ...string) string {
	hasher := sha256.New()
	_, _ = hasher.Write([]byte(domain))
	for _, value := range values {
		_, _ = hasher.Write([]byte(value))
		_, _ = hasher.Write([]byte{0})
	}
	return hex.EncodeToString(hasher.Sum(nil))
}

func resubmitPayload(source []byte, attemptID string) ([]byte, error) {
	var payload map[string]any
	if err := json.Unmarshal(source, &payload); err != nil {
		return nil, fmt.Errorf("decode resubmitted delivery payload: %w", err)
	}
	payload["attempt_id"] = attemptID
	payload["attempt_count"] = 1
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode resubmitted delivery payload: %w", err)
	}
	return encoded, nil
}
