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
}

func (s *stubPurgeStore) PurgeExpired(ctx context.Context, batchSize int) (PurgeCounts, error) {
	if err := ctx.Err(); err != nil {
		return PurgeCounts{}, err
	}
	s.calls.Add(1)
	s.batchSeen.Store(int64(batchSize))
	return s.counts, s.err
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
