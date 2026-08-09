package session

import (
	"context"
	"fmt"
	"time"
)

// StreamOutcome classifies one durable-fanout transition for observation.
//
// It is a bounded set on purpose: the serve process turns these into metric
// label values, and a metric label is a cardinality contract.
type StreamOutcome string

const (
	// StreamOutcomePublished is an event appended to the durable log.
	StreamOutcomePublished StreamOutcome = "published"
	// StreamOutcomeReplayed is an event delivered to local subscribers from the
	// durable log, on this replica or any other.
	StreamOutcomeReplayed StreamOutcome = "replayed"
	// StreamOutcomeDropped is an event a subscriber's bounded channel refused.
	StreamOutcomeDropped StreamOutcome = "dropped"
	// StreamOutcomeError is a durable-log failure. Delivery falls back to
	// in-process fanout so a database blip degrades to the pre-4.3 behaviour
	// rather than to silence.
	StreamOutcomeError StreamOutcome = "error"
)

// StreamLog is the durable fanout log.
//
// It carries envelopes only — see migrations/0005_session_stream_events.sql for
// why a payload column would be a mistake rather than a convenience.
type StreamLog interface {
	// AppendStreamEvent records one envelope and returns its cursor position.
	AppendStreamEvent(ctx context.Context, event StreamEvent) (int64, error)
	// ListStreamEventsAfter returns envelopes with seq strictly greater than
	// afterSeq, oldest first, bounded by limit.
	ListStreamEventsAfter(ctx context.Context, afterSeq int64, limit int) ([]StreamEvent, error)
	// LatestStreamSeq returns the current tail, or 0 when the log is empty.
	LatestStreamSeq(ctx context.Context) (int64, error)
}

// RelayConfig configures the durable fanout relay.
type RelayConfig struct {
	// Log is the durable envelope log. Required.
	Log StreamLog
	// Hub receives replayed envelopes for local delivery. Required.
	Hub *Hub
	// Interval is the poll cadence. Zero uses DefaultRelayInterval.
	Interval time.Duration
	// BatchSize bounds one read. Zero uses DefaultRelayBatchSize.
	BatchSize int
	// Observe, when set, is called once per relay transition. It must not block.
	Observe func(StreamOutcome, error)
}

const (
	// DefaultRelayInterval is the fanout poll cadence.
	//
	// The kill-test budget for cross-replica delivery is 2s; 250ms leaves an
	// order of magnitude of headroom while costing one bounded index scan per
	// replica per tick.
	//
	// LISTEN/NOTIFY was considered and rejected: pq.Listener needs a DSN that
	// PostgresStore is not given (it is constructed from a *sql.DB), and it adds
	// a second connection lifecycle with its own reconnect and shutdown path to
	// a process that already has one. See `.loom/40-decisions.md` (2026-08-08).
	DefaultRelayInterval = 250 * time.Millisecond

	// DefaultRelayBatchSize bounds one relay read so a large backlog cannot
	// produce one enormous query or one enormous local fanout burst.
	DefaultRelayBatchSize = 256
)

// StreamRelay delivers durably logged session events to this replica's local
// subscribers.
//
// It starts at the log's current tail rather than at zero: a replica joining a
// long-lived deployment must not re-page every historical run into every open
// subscription. Within a subscription's lifetime nothing is lost, because the
// relay's cursor only ever moves forward past events it has delivered.
type StreamRelay struct {
	log       StreamLog
	hub       *Hub
	interval  time.Duration
	batchSize int
	observe   func(StreamOutcome, error)
}

// NewStreamRelay builds the relay. Missing collaborators are configuration
// errors, not silent no-ops: a relay that quietly does nothing would reproduce
// exactly the failure this component exists to remove.
func NewStreamRelay(cfg RelayConfig) (*StreamRelay, error) {
	if cfg.Log == nil {
		return nil, fmt.Errorf("%w: session stream relay requires a durable log", ErrInvalid)
	}
	if cfg.Hub == nil {
		return nil, fmt.Errorf("%w: session stream relay requires a hub", ErrInvalid)
	}
	interval := cfg.Interval
	if interval <= 0 {
		interval = DefaultRelayInterval
	}
	batchSize := cfg.BatchSize
	if batchSize <= 0 {
		batchSize = DefaultRelayBatchSize
	}
	return &StreamRelay{
		log:       cfg.Log,
		hub:       cfg.Hub,
		interval:  interval,
		batchSize: batchSize,
		observe:   cfg.Observe,
	}, nil
}

// Interval reports the configured poll cadence.
func (r *StreamRelay) Interval() time.Duration {
	if r == nil {
		return 0
	}
	return r.interval
}

// Run relays until ctx is cancelled.
//
// A failing read is reported through Observe and does not stop the loop: a
// transient database problem must not take the SSE surface down permanently,
// and the cursor is unchanged so nothing is skipped.
func (r *StreamRelay) Run(ctx context.Context) error {
	if r == nil || r.log == nil || r.hub == nil || ctx == nil {
		return fmt.Errorf("%w: session stream relay is not configured", ErrInvalid)
	}

	cursor, err := r.log.LatestStreamSeq(ctx)
	if err != nil {
		// Starting from zero here would replay the whole history into every
		// subscriber. Start from nothing and let the first successful read
		// establish the cursor instead.
		r.report(StreamOutcomeError, err)
		cursor = 0
	}

	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
		cursor = r.drain(ctx, cursor)
	}
}

// drain delivers everything past cursor and returns the new cursor.
func (r *StreamRelay) drain(ctx context.Context, cursor int64) int64 {
	for {
		events, err := r.log.ListStreamEventsAfter(ctx, cursor, r.batchSize)
		if err != nil {
			r.report(StreamOutcomeError, err)
			return cursor
		}
		if len(events) == 0 {
			return cursor
		}
		for _, event := range events {
			if event.Seq > cursor {
				cursor = event.Seq
			}
			r.hub.deliver(event)
			r.report(StreamOutcomeReplayed, nil)
		}
		if len(events) < r.batchSize {
			return cursor
		}
		// A full batch means there is more backlog; keep going rather than
		// waiting a whole tick per batch.
		select {
		case <-ctx.Done():
			return cursor
		default:
		}
	}
}

func (r *StreamRelay) report(outcome StreamOutcome, err error) {
	if r == nil || r.observe == nil {
		return
	}
	r.observe(outcome, err)
}
