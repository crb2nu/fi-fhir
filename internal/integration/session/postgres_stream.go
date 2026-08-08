package session

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// AppendStreamEvent records one envelope in the durable fanout log.
//
// The payload is deliberately not written. See
// migrations/0005_session_stream_events.sql: the GraphQL projection reproduces
// a subscriber's view from (session_id, run_id, event_type) by re-reading the
// durable session and run, so persisting clinical content here would add PHI at
// rest for no observable benefit.
func (s *PostgresStore) AppendStreamEvent(ctx context.Context, event StreamEvent) (int64, error) {
	if !s.available(ctx) {
		return 0, ErrInvalid
	}
	if event.SessionID == "" || event.Type == "" {
		return 0, fmt.Errorf("%w: stream event requires a session and a type", ErrInvalid)
	}
	at := event.At
	if at.IsZero() {
		at = s.clock().UTC()
	}
	var seq int64
	if err := s.db.QueryRowContext(ctx, `
		INSERT INTO integration_session_stream_events
			(tenant_id, event_id, session_id, run_id, event_type, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING seq
	`, s.tenantID, event.ID, event.SessionID, event.RunID, string(event.Type), at.UTC()).Scan(&seq); err != nil {
		return 0, fmt.Errorf("append session stream event: %w", err)
	}
	return seq, nil
}

// ListStreamEventsAfter returns envelopes past a relay's cursor, oldest first.
func (s *PostgresStore) ListStreamEventsAfter(ctx context.Context, afterSeq int64, limit int) ([]StreamEvent, error) {
	if !s.available(ctx) {
		return nil, ErrInvalid
	}
	if limit <= 0 {
		limit = DefaultRelayBatchSize
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT seq, event_id, session_id, run_id, event_type, created_at
		FROM integration_session_stream_events
		WHERE seq > $1 AND tenant_id = $2
		ORDER BY seq
		LIMIT $3
	`, afterSeq, s.tenantID, limit)
	if err != nil {
		return nil, fmt.Errorf("list session stream events: %w", err)
	}
	defer func() { _ = rows.Close() }()

	events := make([]StreamEvent, 0, limit)
	for rows.Next() {
		var event StreamEvent
		var eventType string
		if err := rows.Scan(&event.Seq, &event.ID, &event.SessionID, &event.RunID, &eventType, &event.At); err != nil {
			return nil, fmt.Errorf("scan session stream event: %w", err)
		}
		event.Type = StreamEventType(eventType)
		event.At = event.At.UTC()
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate session stream events: %w", err)
	}
	return events, nil
}

// LatestStreamSeq returns the log tail, or 0 when the log is empty.
func (s *PostgresStore) LatestStreamSeq(ctx context.Context) (int64, error) {
	if !s.available(ctx) {
		return 0, ErrInvalid
	}
	var seq sql.NullInt64
	if err := s.db.QueryRowContext(ctx, `
		SELECT MAX(seq) FROM integration_session_stream_events WHERE tenant_id = $1
	`, s.tenantID).Scan(&seq); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil
		}
		return 0, fmt.Errorf("read session stream tail: %w", err)
	}
	if !seq.Valid {
		return 0, nil
	}
	return seq.Int64, nil
}
