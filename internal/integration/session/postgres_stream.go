package session

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/lib/pq"
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

// AppendStreamEvents records a run's envelopes in one INSERT.
//
// One statement means one round trip and one commit for the whole batch. The
// envelopes arrive as parallel arrays and are inserted through
// unnest ... WITH ORDINALITY ordered by that ordinality, so the BIGSERIAL
// assigns contiguous, increasing seqs in slice order by construction rather
// than by the conventional (but unspecified) evaluation order of a VALUES
// list. The result is mapped back through RETURNING event_id rather than by
// row position for the same reason: RETURNING order is not part of the
// contract.
func (s *PostgresStore) AppendStreamEvents(ctx context.Context, events []StreamEvent) ([]int64, error) {
	if !s.available(ctx) {
		return nil, ErrInvalid
	}
	if len(events) == 0 {
		return nil, nil
	}
	if len(events) == 1 {
		seq, err := s.AppendStreamEvent(ctx, events[0])
		if err != nil {
			return nil, err
		}
		return []int64{seq}, nil
	}

	now := s.clock().UTC()
	ids := make(map[string]int, len(events))
	eventIDs := make([]string, len(events))
	sessionIDs := make([]string, len(events))
	runIDs := make([]string, len(events))
	types := make([]string, len(events))
	createdAt := make([]string, len(events))
	for i, event := range events {
		if event.SessionID == "" || event.Type == "" {
			return nil, fmt.Errorf("%w: stream event %d requires a session and a type", ErrInvalid, i)
		}
		if event.ID == "" {
			return nil, fmt.Errorf("%w: stream event %d requires an id", ErrInvalid, i)
		}
		if _, duplicate := ids[event.ID]; duplicate {
			return nil, fmt.Errorf("%w: stream event id %q repeats within one batch", ErrInvalid, event.ID)
		}
		ids[event.ID] = i
		at := event.At
		if at.IsZero() {
			at = now
		}
		eventIDs[i] = event.ID
		sessionIDs[i] = event.SessionID
		runIDs[i] = event.RunID
		types[i] = string(event.Type)
		createdAt[i] = at.UTC().Format(time.RFC3339Nano)
	}

	rows, err := s.db.QueryContext(ctx, `
		INSERT INTO integration_session_stream_events
			(tenant_id, event_id, session_id, run_id, event_type, created_at)
		SELECT $1, e.event_id, e.session_id, e.run_id, e.event_type, e.created_at::timestamptz
		FROM unnest($2::text[], $3::text[], $4::text[], $5::text[], $6::text[])
			WITH ORDINALITY AS e(event_id, session_id, run_id, event_type, created_at, ord)
		ORDER BY e.ord
		RETURNING seq, event_id
	`, s.tenantID, pq.Array(eventIDs), pq.Array(sessionIDs), pq.Array(runIDs), pq.Array(types), pq.Array(createdAt))
	if err != nil {
		return nil, fmt.Errorf("append session stream events: %w", err)
	}
	defer func() { _ = rows.Close() }()

	seqs := make([]int64, len(events))
	assigned := 0
	for rows.Next() {
		var seq int64
		var id string
		if err := rows.Scan(&seq, &id); err != nil {
			return nil, fmt.Errorf("scan appended session stream event: %w", err)
		}
		index, ok := ids[id]
		if !ok {
			return nil, fmt.Errorf("append session stream events: returned unknown event id %q", id)
		}
		seqs[index] = seq
		assigned++
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate appended session stream events: %w", err)
	}
	if assigned != len(events) {
		return nil, fmt.Errorf("append session stream events: %d of %d rows returned", assigned, len(events))
	}
	for i := 1; i < len(seqs); i++ {
		if seqs[i] <= seqs[i-1] {
			return nil, fmt.Errorf("append session stream events: seq %d for envelope %d is not after %d", seqs[i], i, seqs[i-1])
		}
	}
	return seqs, nil
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
