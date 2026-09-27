package connection

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

// The capture audit rows (.loom/38 Lane C-2). Every write here is either the
// row's one INSERT or an UPDATE that advances version by exactly one — the
// rule the integration_connection_captures trigger enforces — and every UPDATE
// is conditional on the row still being armed, because a finished capture is
// frozen. Nothing here is ever deleted.

const captureColumns = `
	tenant_id, capture_id, session_id, mode, source_id, connection_artifact_id,
	connection_digest, status, captured, max_messages, version, principal_json,
	reason, requested_at, expires_at, completed_at, problems_json
`

func scanCapture(row rowScanner) (Capture, error) {
	var capture Capture
	var mode, status string
	var principal, problems []byte
	var completedAt sql.NullTime
	if err := row.Scan(
		&capture.TenantID, &capture.ID, &capture.SessionID, &mode, &capture.SourceID,
		&capture.ConnectionArtifactID, &capture.ConnectionDigest, &status, &capture.Captured,
		&capture.MaxMessages, &capture.Version, &principal, &capture.Reason,
		&capture.RequestedAt, &capture.ExpiresAt, &completedAt, &problems,
	); err != nil {
		return Capture{}, err
	}
	capture.Mode = CaptureMode(mode)
	capture.Status = CaptureStatus(status)
	capture.RequestedAt = capture.RequestedAt.UTC()
	capture.ExpiresAt = capture.ExpiresAt.UTC()
	if completedAt.Valid {
		completed := completedAt.Time.UTC()
		capture.CompletedAt = &completed
	}
	if err := json.Unmarshal(principal, &capture.RequestedBy); err != nil {
		return Capture{}, fmt.Errorf("decode capture principal: %w", err)
	}
	if err := json.Unmarshal(problems, &capture.Problems); err != nil {
		return Capture{}, fmt.Errorf("decode capture problems: %w", err)
	}
	if capture.Problems == nil {
		capture.Problems = []Problem{}
	}
	return capture, nil
}

// InsertCapture writes one new capture at version one. A stream capture first
// expires its source's armed row if that row is past its TTL, in the same
// transaction, so a capture nobody refreshed cannot block the next one; a
// source that still has a live armed capture refuses with ErrCaptureConflict,
// which the one-armed-stream unique index makes race-free.
func (s *PostgresStore) InsertCapture(ctx context.Context, capture Capture) (Capture, error) {
	if s == nil || s.db == nil || ctx == nil {
		return Capture{}, ErrUnavailable
	}
	principal, err := json.Marshal(capture.RequestedBy)
	if err != nil {
		return Capture{}, fmt.Errorf("marshal capture principal: %w", err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Capture{}, fmt.Errorf("begin capture insert: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if capture.Mode == CaptureModeStream {
		if _, err := tx.ExecContext(ctx, `
			UPDATE integration_connection_captures
			SET status = 'expired', completed_at = $3, version = version + 1
			WHERE tenant_id = $1 AND source_id = $2 AND mode = 'stream'
			  AND status = 'armed' AND expires_at <= $3
		`, capture.TenantID, capture.SourceID, capture.RequestedAt.UTC()); err != nil {
			return Capture{}, fmt.Errorf("expire the source's stale capture: %w", err)
		}
	}
	stored, err := scanCapture(tx.QueryRowContext(ctx, `
		INSERT INTO integration_connection_captures (
			tenant_id, capture_id, session_id, mode, source_id, connection_artifact_id,
			connection_digest, status, captured, max_messages, version, principal_json,
			reason, requested_at, expires_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, 'armed', 0, $8, 1, $9, $10, $11, $12)
		RETURNING `+captureColumns,
		capture.TenantID, capture.ID, capture.SessionID, string(capture.Mode), capture.SourceID,
		capture.ConnectionArtifactID, capture.ConnectionDigest, capture.MaxMessages, string(principal),
		capture.Reason, capture.RequestedAt.UTC(), capture.ExpiresAt.UTC(),
	))
	if err != nil {
		if uniqueViolation(err) {
			return Capture{}, ErrCaptureConflict
		}
		return Capture{}, fmt.Errorf("insert capture: %w", err)
	}
	if err := tx.Commit(); err != nil {
		if uniqueViolation(err) {
			return Capture{}, ErrCaptureConflict
		}
		return Capture{}, fmt.Errorf("commit capture insert: %w", err)
	}
	return stored, nil
}

// GetCapture loads one capture of one tenant.
func (s *PostgresStore) GetCapture(ctx context.Context, tenantID, captureID string) (Capture, error) {
	if s == nil || s.db == nil || ctx == nil {
		return Capture{}, ErrUnavailable
	}
	capture, err := scanCapture(s.db.QueryRowContext(ctx, `
		SELECT `+captureColumns+` FROM integration_connection_captures
		WHERE tenant_id = $1 AND capture_id = $2
	`, tenantID, captureID))
	if errors.Is(err, sql.ErrNoRows) {
		return Capture{}, ErrCaptureNotFound
	}
	if err != nil {
		return Capture{}, fmt.Errorf("load capture: %w", err)
	}
	return capture, nil
}

// ListSessionCaptures returns one session's captures and peeks, newest first,
// at most limit.
func (s *PostgresStore) ListSessionCaptures(ctx context.Context, tenantID, sessionID string, limit int) ([]Capture, error) {
	if s == nil || s.db == nil || ctx == nil {
		return nil, ErrUnavailable
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+captureColumns+` FROM integration_connection_captures
		WHERE tenant_id = $1 AND session_id = $2
		ORDER BY requested_at DESC, capture_id
		LIMIT $3
	`, tenantID, sessionID, limit)
	if err != nil {
		return nil, fmt.Errorf("list session captures: %w", err)
	}
	return collectCaptures(rows)
}

// ArmedStreamCaptures is the armed-capture cache's one read: every stream
// capture of one tenant that is armed, unexpired at now, and still has a free
// slot, oldest first, at most limit.
func (s *PostgresStore) ArmedStreamCaptures(ctx context.Context, tenantID string, now time.Time, limit int) ([]Capture, error) {
	if s == nil || s.db == nil || ctx == nil {
		return nil, ErrUnavailable
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+captureColumns+` FROM integration_connection_captures
		WHERE tenant_id = $1 AND status = 'armed' AND mode = 'stream'
		  AND expires_at > $2 AND captured < max_messages
		ORDER BY requested_at, capture_id
		LIMIT $3
	`, tenantID, now.UTC(), limit)
	if err != nil {
		return nil, fmt.Errorf("list armed stream captures: %w", err)
	}
	return collectCaptures(rows)
}

func collectCaptures(rows *sql.Rows) ([]Capture, error) {
	defer func() { _ = rows.Close() }()
	captures := make([]Capture, 0)
	for rows.Next() {
		capture, err := scanCapture(rows)
		if err != nil {
			return nil, fmt.Errorf("scan capture: %w", err)
		}
		captures = append(captures, capture)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate captures: %w", err)
	}
	return captures, nil
}

// ExpireCaptures advances every armed capture or peek of one tenant whose
// expires_at has passed to expired, and returns how many it advanced.
func (s *PostgresStore) ExpireCaptures(ctx context.Context, tenantID string, now time.Time) (int64, error) {
	if s == nil || s.db == nil || ctx == nil {
		return 0, ErrUnavailable
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE integration_connection_captures
		SET status = 'expired', completed_at = $2, version = version + 1
		WHERE tenant_id = $1 AND status = 'armed' AND expires_at <= $2
	`, tenantID, now.UTC())
	if err != nil {
		return 0, fmt.Errorf("expire captures: %w", err)
	}
	expired, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("count expired captures: %w", err)
	}
	return expired, nil
}

// fillOutcome is what one FillCaptureSlot call did.
type fillOutcome int

const (
	// fillSkipped: nothing was written. The capture is not armed, has expired,
	// has no free slot, or another frame holds its row right now.
	fillSkipped fillOutcome = iota
	// fillWritten: one sample was written and counted.
	fillWritten
	// fillCompleted: the sample filled the last slot and the capture completed.
	fillCompleted
	// fillWriteFailed: the sample write failed, nothing was counted, and the
	// capture was finished as failed.
	fillWriteFailed
)

// captureFill is FillCaptureSlot's result. writeErr is the write's own
// failure, for the observer.
type captureFill struct {
	slot     int
	outcome  fillOutcome
	writeErr error
}

// captureFailTimeout bounds marking a capture failed after its write failed,
// which runs on a fresh context because the write's may be what expired.
const captureFailTimeout = time.Second

// FillCaptureSlot writes the next message of an armed, unexpired stream
// capture and counts it, in that order. It locks the row with
// `FOR UPDATE SKIP LOCKED`, calls write with the 1-based slot, and only then
// advances the row under the version it read: captured becomes the slot,
// and a filled last slot completes the capture in the same update.
//
// So `captured` never counts a sample that was not written, and a capture
// never collects more than max_messages samples however many connections or
// replicas race for it. SKIP LOCKED is what keeps that from costing admission
// anything: a frame that finds the row held by another frame's write skips the
// capture at once rather than waiting — holding a pooled connection — behind
// it. A write that fails finishes the capture as failed with nothing counted.
func (s *PostgresStore) FillCaptureSlot(
	ctx context.Context, tenantID, captureID string, now time.Time,
	write func(ctx context.Context, slot int) error,
) (captureFill, error) {
	if s == nil || s.db == nil || ctx == nil || write == nil {
		return captureFill{}, ErrUnavailable
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return captureFill{}, fmt.Errorf("begin capture fill: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var slot, maxMessages int
	var version int64
	err = tx.QueryRowContext(ctx, `
		SELECT captured + 1, max_messages, version FROM integration_connection_captures
		WHERE tenant_id = $1 AND capture_id = $2 AND mode = 'stream' AND status = 'armed'
		  AND captured < max_messages AND expires_at > $3
		FOR UPDATE SKIP LOCKED
	`, tenantID, captureID, now.UTC()).Scan(&slot, &maxMessages, &version)
	if errors.Is(err, sql.ErrNoRows) {
		return captureFill{outcome: fillSkipped}, nil
	}
	if err != nil {
		return captureFill{}, fmt.Errorf("lock capture slot: %w", err)
	}
	if writeErr := write(ctx, slot); writeErr != nil {
		_ = tx.Rollback()
		failCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), captureFailTimeout)
		defer cancel()
		fill := captureFill{slot: slot, outcome: fillWriteFailed, writeErr: writeErr}
		if _, _, err := s.FinishCapture(failCtx, tenantID, captureID, version, captureFinish{
			status: CaptureStatusFailed, at: s.Now(),
			problems: []Problem{{
				Code: CodeSampleWriteFailed, Path: "",
				Message: fmt.Sprintf("message %d could not be written to the session, so the capture stopped", slot),
			}},
		}); err != nil {
			return fill, fmt.Errorf("finish a capture whose write failed: %w", err)
		}
		return fill, nil
	}
	complete := slot >= maxMessages
	result, err := tx.ExecContext(ctx, `
		UPDATE integration_connection_captures
		SET captured = $3, version = version + 1,
			status = CASE WHEN $4::boolean THEN 'complete' ELSE 'armed' END,
			completed_at = CASE WHEN $4::boolean THEN $5::timestamptz ELSE NULL END
		WHERE tenant_id = $1 AND capture_id = $2 AND version = $6
	`, tenantID, captureID, slot, complete, s.Now(), version)
	if err != nil {
		return captureFill{}, fmt.Errorf("count captured message: %w", err)
	}
	advanced, err := result.RowsAffected()
	if err != nil {
		return captureFill{}, fmt.Errorf("count captured message: %w", err)
	}
	if advanced != 1 {
		return captureFill{}, fmt.Errorf("count captured message: %d rows advanced under the locked version, want 1", advanced)
	}
	if err := tx.Commit(); err != nil {
		return captureFill{}, fmt.Errorf("commit captured message: %w", err)
	}
	if complete {
		return captureFill{slot: slot, outcome: fillCompleted}, nil
	}
	return captureFill{slot: slot, outcome: fillWritten}, nil
}

// captureFinish is one terminal transition.
type captureFinish struct {
	status CaptureStatus
	// captured, when non-nil, sets the count; a peek records what it added.
	captured     *int
	problems     []Problem
	cancellation *captureCancellation
	at           time.Time
}

// captureCancellation is who cancelled an armed capture, and why.
type captureCancellation struct {
	Principal integration.Principal `json:"principal"`
	Reason    string                `json:"reason"`
	At        time.Time             `json:"at"`
}

// FinishCapture takes an armed row to a terminal status. With expectedVersion
// above zero it applies only while the row is still at that version — the end
// of a peek, and a capture whose write failed, whose writers know exactly
// which version they read; with zero it applies at whatever version the armed
// row has reached — a cancel. applied is false when the row was no longer
// armed (or no longer at that version); the returned capture is then the row
// as it stands.
func (s *PostgresStore) FinishCapture(ctx context.Context, tenantID, captureID string, expectedVersion int64, finish captureFinish) (Capture, bool, error) {
	if s == nil || s.db == nil || ctx == nil {
		return Capture{}, false, ErrUnavailable
	}
	if !finish.status.Terminal() {
		return Capture{}, false, ErrInvalidRequest
	}
	problems := finish.problems
	if problems == nil {
		problems = []Problem{}
	}
	problemsJSON, err := json.Marshal(problems)
	if err != nil {
		return Capture{}, false, fmt.Errorf("marshal capture problems: %w", err)
	}
	var cancellation any
	if finish.cancellation != nil {
		encoded, err := json.Marshal(finish.cancellation)
		if err != nil {
			return Capture{}, false, fmt.Errorf("marshal capture cancellation: %w", err)
		}
		cancellation = string(encoded)
	}
	var captured any
	if finish.captured != nil {
		captured = *finish.captured
	}
	stored, err := scanCapture(s.db.QueryRowContext(ctx, `
		UPDATE integration_connection_captures
		SET status = $3, completed_at = $4, version = version + 1,
			problems_json = $5::jsonb,
			cancellation_json = coalesce($6::jsonb, cancellation_json),
			captured = coalesce($7::integer, captured)
		WHERE tenant_id = $1 AND capture_id = $2 AND status = 'armed'
		  AND ($8::bigint = 0 OR version = $8::bigint)
		RETURNING `+captureColumns,
		tenantID, captureID, string(finish.status), finish.at.UTC(), string(problemsJSON),
		cancellation, captured, expectedVersion,
	))
	if errors.Is(err, sql.ErrNoRows) {
		current, loadErr := s.GetCapture(ctx, tenantID, captureID)
		if loadErr != nil {
			return Capture{}, false, loadErr
		}
		return current, false, nil
	}
	if err != nil {
		return Capture{}, false, fmt.Errorf("finish capture: %w", err)
	}
	return stored, true, nil
}
