package autoroute

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// ErrSweeperUnavailable is returned when a sweeper is constructed or run
// without the collaborators it needs.
var ErrSweeperUnavailable = errors.New("autoroute sweeper unavailable")

// PendingAutorouteExpirer marks pending autoroutes whose expiry has passed as
// expired, returning how many rows changed.
//
// *db.MappingStore satisfies this. The sweeper depends on the narrow behavior
// rather than the store so scheduling stays out of the database package and so
// sweep logic is testable without Postgres.
type PendingAutorouteExpirer interface {
	ExpirePendingAutoroutes(ctx context.Context) (int64, error)
}

// SweepResult reports one sweep iteration.
type SweepResult struct {
	// Expired is the number of pending rows moved to expired status.
	Expired int64

	// Duration is how long the underlying store call took.
	Duration time.Duration
}

// SweeperConfig configures the pending autoroute expiry sweeper.
type SweeperConfig struct {
	// Store performs the expiry. Required.
	Store PendingAutorouteExpirer

	// Interval is the sweep cadence. Must be positive; callers that want the
	// sweep disabled should not construct a Sweeper at all.
	Interval time.Duration

	// Observe, when set, is called once per sweep iteration with the result and
	// any error. It must not block.
	Observe func(SweepResult, error)
}

// Sweeper periodically reconciles the stored status of expired pending
// autoroutes.
//
// The review queue's truthfulness does not depend on this sweeper: reads
// already treat time-expired pending rows as expired. The sweeper exists so the
// stored column eventually agrees with those reads.
type Sweeper struct {
	store    PendingAutorouteExpirer
	interval time.Duration
	observe  func(SweepResult, error)
}

// NewSweeper builds a sweeper. A nil store or non-positive interval is a
// configuration error, not a silent no-op.
func NewSweeper(cfg SweeperConfig) (*Sweeper, error) {
	if cfg.Store == nil {
		return nil, fmt.Errorf("%w: no pending autoroute store", ErrSweeperUnavailable)
	}
	if cfg.Interval <= 0 {
		return nil, fmt.Errorf("%w: sweep interval must be positive, got %s", ErrSweeperUnavailable, cfg.Interval)
	}
	return &Sweeper{
		store:    cfg.Store,
		interval: cfg.Interval,
		observe:  cfg.Observe,
	}, nil
}

// Interval reports the configured sweep cadence.
func (s *Sweeper) Interval() time.Duration {
	if s == nil {
		return 0
	}
	return s.interval
}

// SweepOnce runs a single expiry pass.
func (s *Sweeper) SweepOnce(ctx context.Context) (SweepResult, error) {
	if s == nil || s.store == nil || ctx == nil {
		return SweepResult{}, ErrSweeperUnavailable
	}
	started := time.Now()
	expired, err := s.store.ExpirePendingAutoroutes(ctx)
	result := SweepResult{Duration: time.Since(started)}
	if err != nil {
		return result, fmt.Errorf("sweeping pending autoroutes: %w", err)
	}
	result.Expired = expired
	return result, nil
}

// Run sweeps immediately, then on every interval tick, until ctx is cancelled.
//
// A failing iteration is reported through Observe and does not stop the loop: a
// transient database problem must not take down the server, and the next tick
// retries. Only cancellation ends Run, and it does so without error so a normal
// shutdown is not reported as a component failure.
func (s *Sweeper) Run(ctx context.Context) error {
	if s == nil || s.store == nil || ctx == nil {
		return ErrSweeperUnavailable
	}
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		result, err := s.SweepOnce(ctx)
		if s.observe != nil {
			s.observe(result, err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}
