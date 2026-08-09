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

// ExpiredRecordPurger purges every record past its retention deadline, bounded
// by batchSize per class, and reports how many of each class it purged.
//
// *PostgresStore satisfies this. The purger depends on the narrow behaviour
// rather than the store so scheduling stays out of the SQL and so the loop is
// testable without PostgreSQL — the shape internal/terminology/autoroute's
// sweeper established.
type ExpiredRecordPurger interface {
	PurgeExpired(ctx context.Context, batchSize int) (PurgeCounts, error)
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
}

// Total reports every record the pass acted on.
func (c PurgeCounts) Total() int64 {
	return c.CanonicalEvents + c.SessionSamples + c.SessionExports + c.StreamEvents
}

// PurgeResult reports one purge iteration.
type PurgeResult struct {
	PurgeCounts

	// Duration is how long the underlying store call took.
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
	store     ExpiredRecordPurger
	interval  time.Duration
	batchSize int
	observe   func(PurgeResult, error)
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
	return &Purger{
		store:     config.Store,
		interval:  config.Interval,
		batchSize: batchSize,
		observe:   config.Observe,
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

// PurgeOnce runs a single purge pass.
func (p *Purger) PurgeOnce(ctx context.Context) (PurgeResult, error) {
	if p == nil || p.store == nil || ctx == nil {
		return PurgeResult{}, ErrPurgerUnavailable
	}
	started := time.Now()
	counts, err := p.store.PurgeExpired(ctx, p.batchSize)
	result := PurgeResult{Duration: time.Since(started)}
	if err != nil {
		return result, fmt.Errorf("purging expired records: %w", err)
	}
	result.PurgeCounts = counts
	return result, nil
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
