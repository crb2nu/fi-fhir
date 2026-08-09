package retention

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

type stubPurgeStore struct {
	calls     atomic.Int64
	batchSeen atomic.Int64
	counts    PurgeCounts
	err       error

	// remaining, when positive, models a real backlog: each pass removes up to
	// batchSize canonical events and reports saturation while any are left.
	remaining  atomic.Int64
	backlog    func() BacklogCounts
	backlogErr error
}

func (s *stubPurgeStore) PurgeExpired(ctx context.Context, batchSize int) (PurgeCounts, error) {
	if err := ctx.Err(); err != nil {
		return PurgeCounts{}, err
	}
	s.calls.Add(1)
	s.batchSeen.Store(int64(batchSize))
	if s.err != nil {
		return PurgeCounts{}, s.err
	}
	if s.remaining.Load() <= 0 {
		return s.counts, nil
	}
	took := int64(batchSize)
	if left := s.remaining.Load(); left < took {
		took = left
	}
	s.remaining.Add(-took)
	return PurgeCounts{CanonicalEvents: took, Saturated: took == int64(batchSize)}, nil
}

func (s *stubPurgeStore) Backlog(context.Context) (BacklogCounts, error) {
	if s.backlogErr != nil {
		return BacklogCounts{}, s.backlogErr
	}
	if s.backlog != nil {
		return s.backlog(), nil
	}
	return BacklogCounts{CanonicalEvents: s.remaining.Load()}, nil
}

func TestNewPurgerRefusesAnUnusableConfiguration(t *testing.T) {
	if _, err := NewPurger(PurgerConfig{Interval: time.Minute}); !errors.Is(err, ErrPurgerUnavailable) {
		t.Fatalf("nil store = %v, want ErrPurgerUnavailable", err)
	}
	if _, err := NewPurger(PurgerConfig{Store: &stubPurgeStore{}}); !errors.Is(err, ErrPurgerUnavailable) {
		t.Fatalf("zero interval = %v, want ErrPurgerUnavailable", err)
	}
	purger, err := NewPurger(PurgerConfig{Store: &stubPurgeStore{}, Interval: 5 * time.Minute})
	if err != nil {
		t.Fatalf("NewPurger: %v", err)
	}
	if purger.Interval() != 5*time.Minute || purger.BatchSize() != defaultBatchSize {
		t.Fatalf("interval=%s batch=%d", purger.Interval(), purger.BatchSize())
	}
}

func TestPurgeOnceReportsCountsAndPassesTheBatchBound(t *testing.T) {
	store := &stubPurgeStore{counts: PurgeCounts{
		CanonicalEvents: 2, SessionSamples: 3, SessionExports: 1, StreamEvents: 9,
	}}
	purger, err := NewPurger(PurgerConfig{Store: store, Interval: time.Minute, BatchSize: 17})
	if err != nil {
		t.Fatalf("NewPurger: %v", err)
	}
	result, err := purger.PurgeOnce(context.Background())
	if err != nil {
		t.Fatalf("PurgeOnce: %v", err)
	}
	if result.Total() != 15 {
		t.Fatalf("total = %d, want 15", result.Total())
	}
	if store.batchSeen.Load() != 17 {
		t.Fatalf("store saw batch size %d, want 17", store.batchSeen.Load())
	}
}

// The D1 repair, at the unit level: one tick drains a backlog rather than
// removing one batch and waiting for the next tick.
func TestPurgeOnceDrainsTheBacklogWithinOneTick(t *testing.T) {
	store := &stubPurgeStore{}
	store.remaining.Store(1000)
	purger, err := NewPurger(PurgerConfig{Store: store, Interval: time.Hour, BatchSize: 200})
	if err != nil {
		t.Fatalf("NewPurger: %v", err)
	}
	result, err := purger.PurgeOnce(context.Background())
	if err != nil {
		t.Fatalf("PurgeOnce: %v", err)
	}
	if result.CanonicalEvents != 1000 {
		t.Fatalf("tick purged %d records, want the whole 1000-record backlog", result.CanonicalEvents)
	}
	// 1000/200 = 5 full batches, then a sixth pass that comes back empty and
	// unsaturated. Asserting the pass count keeps the drain honest: a single
	// unbounded statement would report one pass and would not be this fix.
	if result.Passes != 6 {
		t.Fatalf("tick made %d passes, want 6 (five full batches plus the empty one that ends the drain)",
			result.Passes)
	}
	if result.Saturated || result.BudgetExhausted {
		t.Fatalf("a drained tick reported saturated=%t budgetExhausted=%t",
			result.Saturated, result.BudgetExhausted)
	}
	if result.Backlog.Total() != 0 {
		t.Fatalf("backlog after a full drain = %d, want 0", result.Backlog.Total())
	}
}

// The budget is what keeps a drain from becoming an unbounded transaction
// against the busiest table in the system. It is checked BETWEEN passes, so a
// tick always makes at least one.
func TestPurgeOnceStopsDrainingWhenTheWallClockBudgetIsSpent(t *testing.T) {
	store := &stubPurgeStore{}
	store.remaining.Store(1000)
	purger, err := NewPurger(PurgerConfig{
		Store: store, Interval: time.Hour, BatchSize: 200, DrainBudget: time.Minute,
	})
	if err != nil {
		t.Fatalf("NewPurger: %v", err)
	}
	// An injected clock that advances 40 seconds per reading exhausts a
	// one-minute budget partway through the drain, deterministically.
	var ticks atomic.Int64
	base := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	purger.now = func() time.Time {
		return base.Add(time.Duration(ticks.Add(1)) * 40 * time.Second)
	}

	result, err := purger.PurgeOnce(context.Background())
	if err != nil {
		t.Fatalf("PurgeOnce: %v", err)
	}
	if !result.BudgetExhausted {
		t.Fatal("the tick spent its whole budget without reporting BudgetExhausted; " +
			"an operator cannot tell a purge that is keeping up from one that is not")
	}
	if result.Passes < 1 {
		t.Fatal("the budget cut the tick short of even one pass; it must bound catch-up, not the purge")
	}
	if result.CanonicalEvents >= 1000 {
		t.Fatalf("the tick drained the whole backlog (%d) despite an exhausted budget",
			result.CanonicalEvents)
	}
	if result.Backlog.Total() == 0 {
		t.Fatal("the gauge reported an empty backlog while records remained")
	}
}

// The budget can never outlast the cadence it runs on.
func TestDrainBudgetIsClampedToHalfTheInterval(t *testing.T) {
	purger, err := NewPurger(PurgerConfig{
		Store: &stubPurgeStore{}, Interval: time.Minute, DrainBudget: time.Hour,
	})
	if err != nil {
		t.Fatalf("NewPurger: %v", err)
	}
	if got := purger.DrainBudget(); got != 30*time.Second {
		t.Fatalf("drain budget = %s, want the interval halved (30s)", got)
	}
	unset, err := NewPurger(PurgerConfig{Store: &stubPurgeStore{}, Interval: time.Hour})
	if err != nil {
		t.Fatalf("NewPurger: %v", err)
	}
	if got := unset.DrainBudget(); got != defaultDrainBudget {
		t.Fatalf("default drain budget = %s, want %s", got, defaultDrainBudget)
	}
}

// A transient database failure must be reported and survived, not fatal: the
// next tick retries, and a retention purge must never take the server down.
func TestRunSurvivesAFailingIterationAndStopsOnlyOnCancellation(t *testing.T) {
	store := &stubPurgeStore{err: errors.New("connection reset")}
	observed := make(chan error, 4)
	purger, err := NewPurger(PurgerConfig{
		Store: store, Interval: time.Millisecond,
		Observe: func(_ PurgeResult, err error) {
			select {
			case observed <- err:
			default:
			}
		},
	})
	if err != nil {
		t.Fatalf("NewPurger: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- purger.Run(ctx) }()

	for range 2 {
		select {
		case err := <-observed:
			if err == nil {
				t.Fatal("a failing iteration was observed as a success")
			}
		case <-time.After(5 * time.Second):
			t.Fatal("Run stopped reporting iterations after a failure")
		}
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned %v on cancellation, want nil so shutdown is not a component failure", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after cancellation")
	}
	if store.calls.Load() < 2 {
		t.Fatalf("store was called %d times, want repeated retries", store.calls.Load())
	}
}

func TestNilPurgerIsInertRatherThanPanicking(t *testing.T) {
	var purger *Purger
	if purger.Interval() != 0 || purger.BatchSize() != 0 {
		t.Fatal("nil purger reported a configuration")
	}
	if _, err := purger.PurgeOnce(context.Background()); !errors.Is(err, ErrPurgerUnavailable) {
		t.Fatalf("nil PurgeOnce = %v", err)
	}
	if err := purger.Run(context.Background()); !errors.Is(err, ErrPurgerUnavailable) {
		t.Fatalf("nil Run = %v", err)
	}
}
