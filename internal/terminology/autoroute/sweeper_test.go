package autoroute

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// stubExpirer records calls and returns scripted results.
type stubExpirer struct {
	mu      sync.Mutex
	calls   int
	expired int64
	err     error
	// notify, when set, receives one value per call so tests can wait on
	// iteration counts instead of sleeping.
	notify chan struct{}
}

func (s *stubExpirer) ExpirePendingAutoroutes(ctx context.Context) (int64, error) {
	s.mu.Lock()
	s.calls++
	expired, err := s.expired, s.err
	notify := s.notify
	s.mu.Unlock()

	if notify != nil {
		select {
		case notify <- struct{}{}:
		default:
		}
	}
	if err != nil {
		return 0, err
	}
	return expired, nil
}

func (s *stubExpirer) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func TestNewSweeper_RejectsInvalidConfig(t *testing.T) {
	tests := []struct {
		name string
		cfg  SweeperConfig
	}{
		{"nil store", SweeperConfig{Interval: time.Minute}},
		{"zero interval", SweeperConfig{Store: &stubExpirer{}}},
		{"negative interval", SweeperConfig{Store: &stubExpirer{}, Interval: -time.Second}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sweeper, err := NewSweeper(tt.cfg)
			if err == nil {
				t.Fatalf("NewSweeper(%s) = nil error, want error", tt.name)
			}
			if !errors.Is(err, ErrSweeperUnavailable) {
				t.Errorf("error = %v, want ErrSweeperUnavailable", err)
			}
			if sweeper != nil {
				t.Errorf("sweeper = %v, want nil on error", sweeper)
			}
		})
	}
}

func TestNewSweeper_Valid(t *testing.T) {
	sweeper, err := NewSweeper(SweeperConfig{Store: &stubExpirer{}, Interval: 30 * time.Second})
	if err != nil {
		t.Fatalf("NewSweeper failed: %v", err)
	}
	if got := sweeper.Interval(); got != 30*time.Second {
		t.Errorf("Interval() = %s, want 30s", got)
	}
}

func TestSweeper_SweepOnce_ReportsExpiredCount(t *testing.T) {
	store := &stubExpirer{expired: 7}
	sweeper, err := NewSweeper(SweeperConfig{Store: store, Interval: time.Minute})
	if err != nil {
		t.Fatalf("NewSweeper failed: %v", err)
	}

	result, err := sweeper.SweepOnce(context.Background())
	if err != nil {
		t.Fatalf("SweepOnce failed: %v", err)
	}
	if result.Expired != 7 {
		t.Errorf("Expired = %d, want 7", result.Expired)
	}
	if result.Duration < 0 {
		t.Errorf("Duration = %s, want non-negative", result.Duration)
	}
	if store.callCount() != 1 {
		t.Errorf("store calls = %d, want 1", store.callCount())
	}
}

func TestSweeper_SweepOnce_WrapsStoreError(t *testing.T) {
	storeErr := errors.New("connection refused")
	store := &stubExpirer{err: storeErr}
	sweeper, err := NewSweeper(SweeperConfig{Store: store, Interval: time.Minute})
	if err != nil {
		t.Fatalf("NewSweeper failed: %v", err)
	}

	result, err := sweeper.SweepOnce(context.Background())
	if err == nil {
		t.Fatal("SweepOnce = nil error, want error")
	}
	if !errors.Is(err, storeErr) {
		t.Errorf("error = %v, want wrapped %v", err, storeErr)
	}
	if result.Expired != 0 {
		t.Errorf("Expired = %d, want 0 on failure", result.Expired)
	}
}

func TestSweeper_Run_SweepsImmediatelyAndOnTick(t *testing.T) {
	notify := make(chan struct{}, 8)
	store := &stubExpirer{expired: 1, notify: notify}
	sweeper, err := NewSweeper(SweeperConfig{Store: store, Interval: 5 * time.Millisecond})
	if err != nil {
		t.Fatalf("NewSweeper failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- sweeper.Run(ctx) }()

	// Three iterations proves the immediate sweep plus at least two ticks.
	for i := 0; i < 3; i++ {
		select {
		case <-notify:
		case <-time.After(2 * time.Second):
			t.Fatalf("sweep iteration %d did not run", i+1)
		}
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run after cancel = %v, want nil", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after cancellation")
	}
}

// A failing store must not stop the loop: serve keeps running and the next tick
// retries.
func TestSweeper_Run_ContinuesAfterStoreError(t *testing.T) {
	notify := make(chan struct{}, 8)
	store := &stubExpirer{err: errors.New("transient failure"), notify: notify}

	var mu sync.Mutex
	var observedErrs int
	sweeper, err := NewSweeper(SweeperConfig{
		Store:    store,
		Interval: 5 * time.Millisecond,
		Observe: func(_ SweepResult, err error) {
			if err != nil {
				mu.Lock()
				observedErrs++
				mu.Unlock()
			}
		},
	})
	if err != nil {
		t.Fatalf("NewSweeper failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- sweeper.Run(ctx) }()

	for i := 0; i < 3; i++ {
		select {
		case <-notify:
		case <-time.After(2 * time.Second):
			t.Fatalf("sweep iteration %d did not run after prior failure", i+1)
		}
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run = %v, want nil despite store failures", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after cancellation")
	}

	mu.Lock()
	defer mu.Unlock()
	if observedErrs < 3 {
		t.Errorf("observed errors = %d, want >= 3 (every failure reported)", observedErrs)
	}
}

func TestSweeper_Run_ObservesResults(t *testing.T) {
	notify := make(chan struct{}, 4)
	store := &stubExpirer{expired: 4, notify: notify}

	var mu sync.Mutex
	var results []SweepResult
	sweeper, err := NewSweeper(SweeperConfig{
		Store:    store,
		Interval: 5 * time.Millisecond,
		Observe: func(result SweepResult, err error) {
			if err != nil {
				return
			}
			mu.Lock()
			results = append(results, result)
			mu.Unlock()
		},
	})
	if err != nil {
		t.Fatalf("NewSweeper failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = sweeper.Run(ctx) }()
	select {
	case <-notify:
	case <-time.After(2 * time.Second):
		t.Fatal("first sweep did not run")
	}
	cancel()

	mu.Lock()
	defer mu.Unlock()
	if len(results) == 0 {
		t.Fatal("no sweep results observed")
	}
	if results[0].Expired != 4 {
		t.Errorf("first observed Expired = %d, want 4", results[0].Expired)
	}
}

// Cancelling before Run starts must return immediately without sweeping twice.
func TestSweeper_Run_ReturnsOnPreCancelledContext(t *testing.T) {
	store := &stubExpirer{}
	sweeper, err := NewSweeper(SweeperConfig{Store: store, Interval: time.Hour})
	if err != nil {
		t.Fatalf("NewSweeper failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan error, 1)
	go func() { done <- sweeper.Run(ctx) }()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run = %v, want nil", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return on pre-cancelled context")
	}
	if got := store.callCount(); got != 1 {
		t.Errorf("store calls = %d, want 1 (single startup sweep, then exit)", got)
	}
}

func TestSweeper_NilReceiverAndContext(t *testing.T) {
	var sweeper *Sweeper
	if got := sweeper.Interval(); got != 0 {
		t.Errorf("nil Interval() = %s, want 0", got)
	}
	if _, err := sweeper.SweepOnce(context.Background()); !errors.Is(err, ErrSweeperUnavailable) {
		t.Errorf("nil SweepOnce = %v, want ErrSweeperUnavailable", err)
	}
	if err := sweeper.Run(context.Background()); !errors.Is(err, ErrSweeperUnavailable) {
		t.Errorf("nil Run = %v, want ErrSweeperUnavailable", err)
	}

	valid, err := NewSweeper(SweeperConfig{Store: &stubExpirer{}, Interval: time.Minute})
	if err != nil {
		t.Fatalf("NewSweeper failed: %v", err)
	}
	// Deliberately passing a nil context to prove the guard.
	if err := valid.Run(nil); !errors.Is(err, ErrSweeperUnavailable) {
		t.Errorf("Run(nil ctx) = %v, want ErrSweeperUnavailable", err)
	}
}
