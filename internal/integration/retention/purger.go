package retention

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// ErrPurgerUnavailable is returned when a purger is constructed or run without
// the collaborators it needs.
var ErrPurgerUnavailable = errors.New("retention purger unavailable")

// defaultDrainBudget bounds the wall-clock time one tick may spend draining.
//
// The drain loop exists because a full batch means there is more backlog. The
// budget exists because "keep going until it is empty" on the busiest table in
// the system would let one tick hold a connection for an unbounded time against
// a tenant that is ingesting faster than the purge removes. Five minutes is 8%
// of the shipped hourly cadence: long enough that a realistic backlog clears in
// one tick, short enough that a pathological one yields the connection and
// resumes on the next.
const defaultDrainBudget = 5 * time.Minute

// ExpiredRecordPurger purges every record past its retention deadline, bounded
// by batchSize per class, and reports how many of each class it purged and how
// much remains.
//
// *PostgresStore satisfies this. The purger depends on the narrow behaviour
// rather than the store so scheduling stays out of the SQL and so the loop is
// testable without PostgreSQL — the shape internal/terminology/autoroute's
// sweeper established.
type ExpiredRecordPurger interface {
	PurgeExpired(ctx context.Context, batchSize int) (PurgeCounts, error)

	// Backlog reports the records eligible for purge right now. It is read once
	// per tick, after draining, and published as a gauge. Before Sprint 5
	// retention had counters only, so a purge that could not keep up looked
	// exactly like one that was keeping up.
	Backlog(ctx context.Context) (BacklogCounts, error)
}

// PurgeCounts reports what one pass removed.
type PurgeCounts struct {
	// CanonicalEvents is the number of canonical event payloads tombstoned.
	CanonicalEvents int64
	// SessionSamples is the number of session sample rows deleted outright.
	SessionSamples int64
	// SessionExports is the number of export snapshots tombstoned.
	SessionExports int64
	// StreamEvents is the number of payload-free fanout envelopes pruned.
	StreamEvents int64

	// Saturated reports that at least one bounded statement came back full, so
	// backlog remains and the caller should drain rather than wait a tick. It is
	// deliberately not part of Total: it is a control signal, not a record count.
	Saturated bool
}

// Total reports every record the pass acted on.
func (c PurgeCounts) Total() int64 {
	return c.CanonicalEvents + c.SessionSamples + c.SessionExports + c.StreamEvents
}

// add accumulates one pass into a tick's running total. Saturation is the
// LAST pass's answer, not the union of every pass's: a tick that drained to
// completion has an unsaturated final pass and must not report otherwise.
func (c *PurgeCounts) add(pass PurgeCounts) {
	c.CanonicalEvents += pass.CanonicalEvents
	c.SessionSamples += pass.SessionSamples
	c.SessionExports += pass.SessionExports
	c.StreamEvents += pass.StreamEvents
	c.Saturated = pass.Saturated
}

// PurgeResult reports one purge iteration — one tick, which is one or more
// passes.
type PurgeResult struct {
	PurgeCounts

	// Passes is how many store calls the tick made. One means the first pass
	// found no full batch; more means the tick drained.
	Passes int

	// BudgetExhausted reports that the tick stopped with backlog still draining
	// because it ran out of wall-clock budget. It is the honest signal that the
	// purge is not keeping up, distinct from an error.
	BudgetExhausted bool

	// Backlog is the eligible-record count read after the tick finished. It is
	// what the gauge publishes.
	Backlog BacklogCounts

	// BacklogKnown reports that Backlog was actually read. A zero Backlog with
	// BacklogKnown false means "not measured", which is not the same claim as
	// "nothing is owed" — publishing the second when the first is true would
	// show an empty backlog at exactly the moment the purge is broken.
	BacklogKnown bool

	// Duration is how long the whole tick took, every pass included.
	Duration time.Duration
}

// PurgerConfig configures the retention purge component.
type PurgerConfig struct {
	// Store performs the purge. Required.
	Store ExpiredRecordPurger

	// Interval is the purge cadence. Must be positive; a caller that wants the
	// purge disabled should not construct a Purger at all.
	Interval time.Duration

	// BatchSize bounds one pass per record class. Non-positive means the store's
	// own default.
	BatchSize int

	// DrainBudget bounds the wall-clock time one tick may spend draining a
	// backlog. Non-positive means defaultDrainBudget. It is clamped to half the
	// interval so a tick can never overrun the next one.
	//
	// A tick always makes at least one pass, whatever the budget: the budget is
	// checked after a pass, so it bounds catch-up work rather than the purge
	// itself.
	DrainBudget time.Duration

	// Observe, when set, is called once per iteration with the result and any
	// error. It must not block.
	Observe func(PurgeResult, error)
}

// Purger periodically enforces the tenant's retention policy.
//
// It takes no lease and elects no leader. Every write the store issues is an
// idempotent guarded statement whose RETURNING clause is the claim, and the
// audit row is written by the same statement as the tombstone — so two replicas
// running concurrently produce exactly one tombstone and exactly one audit row
// per record without a lock. S3-A rejected pg_advisory_lock for the autoroute
// notifier on the same reasoning (`.loom/40-decisions.md`).
type Purger struct {
	store       ExpiredRecordPurger
	interval    time.Duration
	batchSize   int
	drainBudget time.Duration
	observe     func(PurgeResult, error)
	now         func() time.Time
}

// NewPurger builds a purger. A nil store or non-positive interval is a
// configuration error, not a silent no-op.
func NewPurger(config PurgerConfig) (*Purger, error) {
	if config.Store == nil {
		return nil, fmt.Errorf("%w: no retention store", ErrPurgerUnavailable)
	}
	if config.Interval <= 0 {
		return nil, fmt.Errorf("%w: purge interval must be positive, got %s",
			ErrPurgerUnavailable, config.Interval)
	}
	batchSize := config.BatchSize
	if batchSize <= 0 {
		batchSize = defaultBatchSize
	}
	budget := config.DrainBudget
	if budget <= 0 {
		budget = defaultDrainBudget
	}
	if half := config.Interval / 2; budget > half {
		// A tick that outlasts its own cadence stops being a cadence. The pass
		// itself is never cut short — the budget is checked between passes — so
		// clamping here bounds catch-up, not correctness.
		budget = half
	}
	return &Purger{
		store:       config.Store,
		interval:    config.Interval,
		batchSize:   batchSize,
		drainBudget: budget,
		observe:     config.Observe,
		now:         time.Now,
	}, nil
}

// Interval reports the configured purge cadence.
func (p *Purger) Interval() time.Duration {
	if p == nil {
		return 0
	}
	return p.interval
}

// BatchSize reports the per-class bound on one pass.
func (p *Purger) BatchSize() int {
	if p == nil {
		return 0
	}
	return p.batchSize
}

// DrainBudget reports the wall-clock bound on one tick's catch-up work.
func (p *Purger) DrainBudget() time.Duration {
	if p == nil {
		return 0
	}
	return p.drainBudget
}

// PurgeOnce runs one purge tick: it drains the backlog rather than removing a
// single batch and waiting an hour.
//
// THIS IS THE D1 REPAIR. Before Sprint 5 this made exactly one store call per
// tick against a per-class LIMIT of 200 on an hourly cadence, so the sustained
// ceiling was 200 records per class per hour — 0.056/sec — on the table
// store.go:31-33 calls "the busiest table in the system", with no catch-up and
// no gauge that would have revealed the backlog. A tenant that produced records
// faster than that fell permanently behind its own retention policy, silently.
//
// The shape is the one the repository already uses one package over —
// internal/integration/session/stream.go:174-179, "A full batch means there is
// more backlog; keep going rather than waiting a whole tick per batch" — with
// one addition the stream relay does not need: a wall-clock budget, because the
// purge holds row locks and writes an audit row per record, and a tenant
// ingesting faster than the purge drains must not be able to hold a connection
// for the whole interval.
//
// A tick always makes at least one pass. The budget is checked between passes,
// so it bounds catch-up rather than the purge, and exhausting it is reported on
// the result rather than as an error: falling behind is a real operating
// condition with a gauge to match, not a failure.
func (p *Purger) PurgeOnce(ctx context.Context) (PurgeResult, error) {
	if p == nil || p.store == nil || ctx == nil {
		return PurgeResult{}, ErrPurgerUnavailable
	}
	started := p.clock()
	var result PurgeResult
	var purgeErr error
	for {
		counts, err := p.store.PurgeExpired(ctx, p.batchSize)
		result.Passes++
		result.PurgeCounts.add(counts)
		if err != nil {
			purgeErr = fmt.Errorf("purging expired records: %w", err)
			break
		}
		if !counts.Saturated || !drainOnFullBatch() {
			break
		}
		if ctx.Err() != nil {
			break
		}
		if p.clock().Sub(started) >= p.drainBudget {
			result.BudgetExhausted = true
			break
		}
	}
	result.Duration = p.clock().Sub(started)

	// The gauge is read after the drain so it reports what is still owed rather
	// than what was owed before the tick started — and it is read even when the
	// purge failed, because a failing purge is exactly when an operator needs to
	// know how far behind it is. A read that fails leaves BacklogKnown false so
	// the caller publishes nothing, rather than publishing a zero that would
	// read as "the backlog cleared".
	backlog, backlogErr := p.store.Backlog(ctx)
	if backlogErr == nil {
		result.Backlog = backlog
		result.BacklogKnown = true
	}
	if purgeErr != nil {
		return result, purgeErr
	}
	if backlogErr != nil {
		return result, fmt.Errorf("reading retention backlog: %w", backlogErr)
	}
	return result, nil
}

func (p *Purger) clock() time.Time {
	if p == nil || p.now == nil {
		return time.Now()
	}
	return p.now()
}

// Run purges immediately, then on every interval tick, until ctx is cancelled.
//
// A failing iteration is reported through Observe and does not stop the loop: a
// transient database problem must not take down the server, and the next tick
// retries. Only cancellation ends Run, and it does so without error so a normal
// shutdown is not reported as a component failure.
func (p *Purger) Run(ctx context.Context) error {
	if p == nil || p.store == nil || ctx == nil {
		return ErrPurgerUnavailable
	}
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()
	for {
		result, err := p.PurgeOnce(ctx)
		if p.observe != nil {
			p.observe(result, err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}
