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
	reason, requested_at, expires_at, completed_at, problems_json, object_path
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
		&capture.RequestedAt, &capture.ExpiresAt, &completedAt, &problems, &capture.ObjectPath,
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
			reason, requested_at, expires_at, object_path
		) VALUES ($1, $2, $3, $4, $5, $6, $7, 'armed', 0, $8, 1, $9, $10, $11, $12, $13)
		RETURNING `+captureColumns,
		capture.TenantID, capture.ID, capture.SessionID, string(capture.Mode), capture.SourceID,
		capture.ConnectionArtifactID, capture.ConnectionDigest, capture.MaxMessages, string(principal),
		capture.Reason, capture.RequestedAt.UTC(), capture.ExpiresAt.UTC(), capture.ObjectPath,
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

// ExpireCaptures advances to expired every armed stream capture of one tenant
// whose expires_at has passed, and every armed peek row orphaned past
// peekOrphanGrace, and returns how many it advanced.
//
// A peek's row is armed exactly while its own request reads and then records
// the outcome (PeekBatch), and expires_at is only when that read must stop, so
// expiring it at expires_at would race the finish and discard what the peek
// wrote. Only a row still armed peekOrphanGrace later — its replica died
// between insert and finish — is expired here, as it stands.
func (s *PostgresStore) ExpireCaptures(ctx context.Context, tenantID string, now time.Time) (int64, error) {
	if s == nil || s.db == nil || ctx == nil {
		return 0, ErrUnavailable
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE integration_connection_captures
		SET status = 'expired', completed_at = $2, version = version + 1
		WHERE tenant_id = $1 AND status = 'armed'
		  AND ((mode = 'stream' AND expires_at <= $2) OR (mode = 'peek' AND expires_at <= $3))
	`, tenantID, now.UTC(), now.Add(-peekOrphanGrace).UTC())
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
	// fillCountFailed: the sample was written but its count did not commit, so
	// the capture was finished as failed with the sample counted.
	fillCountFailed
)

// captureFill is FillCaptureSlot's result. cause is the write's or the
// count's own failure, for the observer. counted says the row's count covers
// this frame's sample; finished says the row is terminal.
type captureFill struct {
	slot     int
	outcome  fillOutcome
	cause    error
	counted  bool
	finished bool
}

// The slot fill's time budget. One frame's capture adds at most
// captureFillTimeout + captureFailTimeout (2 s) to its admission: the fill —
// lock, write, count, commit — runs under captureFillTimeout, of which the
// write may use all but captureAdvanceReserve, kept for the count and the
// commit; then, only when the fill failed after locking the slot, the capture
// is finished as failed on a fresh context under captureFailTimeout, because
// the fill's may be what expired.
const (
	captureFillTimeout    = 1500 * time.Millisecond
	captureAdvanceReserve = 500 * time.Millisecond
	captureFailTimeout    = 500 * time.Millisecond
)

// captureSampleID names the sample of one capture slot. It is derived, never
// generated, so a slot written twice — a retry after a count that did not
// commit — is one sample (session.AddSampleRequest.ID).
func captureSampleID(captureID string, slot int) string {
	return fmt.Sprintf("sample_capture_%s_%d", captureID, slot)
}

// FillCaptureSlot writes the next message of an armed, unexpired stream
// capture and counts it, in that order. It locks the row with
// `FOR UPDATE SKIP LOCKED`, calls write with the 1-based slot, and only then
// advances the row under the version it read: captured becomes the slot,
// and a filled last slot completes the capture in the same update.
//
// What that guarantees:
//
//   - a session never holds more than max_messages samples of one capture.
//     A slot is handed out only under the row's lock and only up to
//     max_messages, and write names the slot's sample captureSampleID, so a
//     slot written again — after an earlier write whose count did not commit —
//     is the same one sample, not a second;
//   - captured never counts a sample that was not written;
//   - captured is exact for every capture that completes. Otherwise it can
//     undercount the session by one, and only when a fill failed around its
//     write: a write that failed ambiguously (the session committed it, the
//     tap saw an error) finishes the capture failed without it, and a count
//     that failed and whose failure could not be recorded either leaves the
//     row armed one behind — which the next frame's idempotent rewrite of the
//     same slot then settles, or the TTL expires as it stands.
//
// A fill that fails after locking its slot finishes the capture as failed:
// SAMPLE_WRITE_FAILED with nothing counted when the write failed, and
// CAPTURE_COUNT_FAILED with the slot counted when the write succeeded and the
// count did not commit. SKIP LOCKED is what keeps all of this from costing
// admission anything: a frame that finds the row held by another frame's write
// skips the capture at once rather than waiting — holding a pooled connection
// — behind it.
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
	if writeErr := s.writeSlot(ctx, slot, write); writeErr != nil {
		_ = tx.Rollback()
		return s.failFill(ctx, tenantID, captureID, version, captureFill{slot: slot, outcome: fillWriteFailed, cause: writeErr},
			slot-1, Problem{
				Code:    CodeSampleWriteFailed,
				Message: fmt.Sprintf("message %d could not be written to the session, so the capture stopped", slot),
			})
	}
	complete := slot >= maxMessages
	if countErr := s.countSlot(ctx, tx, tenantID, captureID, slot, complete, version); countErr != nil {
		_ = tx.Rollback()
		return s.failFill(ctx, tenantID, captureID, version, captureFill{slot: slot, outcome: fillCountFailed, cause: countErr},
			slot, Problem{
				Code:    CodeCaptureCountFailed,
				Message: fmt.Sprintf("message %d was written to the session but could not be counted, so the capture stopped", slot),
			})
	}
	if complete {
		return captureFill{slot: slot, outcome: fillCompleted, counted: true, finished: true}, nil
	}
	return captureFill{slot: slot, outcome: fillWritten, counted: true}, nil
}

// writeSlot runs write with captureAdvanceReserve of ctx's budget held back,
// so a slow session store cannot leave the count and the commit no time.
func (s *PostgresStore) writeSlot(ctx context.Context, slot int, write func(ctx context.Context, slot int) error) error {
	writeCtx := ctx
	if deadline, ok := ctx.Deadline(); ok {
		var cancel context.CancelFunc
		writeCtx, cancel = context.WithDeadline(ctx, deadline.Add(-captureAdvanceReserve))
		defer cancel()
	}
	return write(writeCtx, slot)
}

// countSlot advances the locked row to the written slot and commits.
func (s *PostgresStore) countSlot(ctx context.Context, tx *sql.Tx, tenantID, captureID string, slot int, complete bool, version int64) error {
	result, err := tx.ExecContext(ctx, `
		UPDATE integration_connection_captures
		SET captured = $3, version = version + 1,
			status = CASE WHEN $4::boolean THEN 'complete' ELSE 'armed' END,
			completed_at = CASE WHEN $4::boolean THEN $5::timestamptz ELSE NULL END
		WHERE tenant_id = $1 AND capture_id = $2 AND version = $6
	`, tenantID, captureID, slot, complete, s.Now(), version)
	if err != nil {
		return fmt.Errorf("count captured message: %w", err)
	}
	advanced, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count captured message: %w", err)
	}
	if advanced != 1 {
		return fmt.Errorf("count captured message: %d rows advanced under the locked version, want 1", advanced)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit captured message: %w", err)
	}
	return nil
}

// failFill finishes a capture whose fill failed after locking its slot, on a
// fresh context bounded by captureFailTimeout, under the version the fill read
// and with captured set to what the session holds of it. The finish does not
// apply when the row moved on — a count whose commit reported failure but
// landed — and fill then reports the row as it stands. An error means the row
// may still be armed one slot behind; the next frame's write of the same slot
// is idempotent, so that is safe.
func (s *PostgresStore) failFill(
	ctx context.Context, tenantID, captureID string, version int64, fill captureFill, captured int, problem Problem,
) (captureFill, error) {
	failCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), captureFailTimeout)
	defer cancel()
	row, _, err := s.FinishCapture(failCtx, tenantID, captureID, version, captureFinish{
		mode: CaptureModeStream, status: CaptureStatusFailed, captured: &captured,
		problems: []Problem{problem}, at: s.Now(),
	})
	if err != nil {
		return fill, fmt.Errorf("finish a capture whose slot %d failed: %w", fill.slot, err)
	}
	fill.counted = fill.outcome == fillCountFailed && row.Captured >= fill.slot
	fill.finished = row.Status.Terminal()
	return fill, nil
}

// HasCompiledStreamSource reports whether a compiled MLLP or HTTP source
// revision of one tenant names sourceID: a source the capture tap can see
// frames of once a replica mounts it.
func (s *PostgresStore) HasCompiledStreamSource(ctx context.Context, tenantID, sourceID string) (bool, error) {
	if s == nil || s.db == nil || ctx == nil {
		return false, ErrUnavailable
	}
	var found bool
	if err := s.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM integration_connection_revisions
			WHERE tenant_id = $1 AND direction = 'source' AND kind IN ('mllp', 'http')
			  AND revision_json->>'source_id' = $2
		)
	`, tenantID, sourceID).Scan(&found); err != nil {
		return false, fmt.Errorf("look up compiled stream sources: %w", err)
	}
	return found, nil
}

// captureFinish is one terminal transition of a row of one mode.
type captureFinish struct {
	// mode is the only mode the transition applies to: a cancel or a failed
	// fill never finishes a peek's row, and a peek never finishes a stream's.
	mode   CaptureMode
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

// FinishCapture takes an armed row of finish.mode to a terminal status. With
// expectedVersion above zero it applies only while the row is still at that
// version — the end of a peek, and a capture whose fill failed, whose writers
// know exactly which version they read; with zero it applies at whatever
// version the armed row has reached — a cancel. applied is false when the row
// was no longer armed, no longer at that version, or of the other mode; the
// returned capture is then the row as it stands.
func (s *PostgresStore) FinishCapture(ctx context.Context, tenantID, captureID string, expectedVersion int64, finish captureFinish) (Capture, bool, error) {
	if s == nil || s.db == nil || ctx == nil {
		return Capture{}, false, ErrUnavailable
	}
	if !finish.status.Terminal() || (finish.mode != CaptureModeStream && finish.mode != CaptureModePeek) {
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
		WHERE tenant_id = $1 AND capture_id = $2 AND status = 'armed' AND mode = $9
		  AND ($8::bigint = 0 OR version = $8::bigint)
		RETURNING `+captureColumns,
		tenantID, captureID, string(finish.status), finish.at.UTC(), string(problemsJSON),
		cancellation, captured, expectedVersion, string(finish.mode),
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
