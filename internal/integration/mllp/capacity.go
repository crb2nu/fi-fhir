package mllp

import (
	"context"
	"errors"
	"sync"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

var (
	ErrCapacityExceeded = errors.New("MLLP capacity is exhausted")
	ErrRateExceeded     = errors.New("MLLP message rate is exhausted")
)

type capacityGate struct {
	mu     sync.Mutex
	now    func() time.Time
	notify chan struct{}
	// quota resolves this replica's share of the deployment-wide rate. When it
	// is nil the gate enforces the declared rate per replica, which is the
	// pre-4.4e behaviour and is correct for a single-replica deployment; a
	// multi-replica deployment wires the coordinator in serve.
	quota   RateQuota
	active  int
	pending int
	rateKey string
	tokens  float64
	last    time.Time
}

func newCapacityGate(clock func() time.Time, quota RateQuota) *capacityGate {
	if clock == nil {
		clock = time.Now
	}
	return &capacityGate{now: clock, notify: make(chan struct{}), quota: quota}
}

func (g *capacityGate) acquire(
	ctx context.Context,
	policy integration.CapacityPolicy,
	rateKey string,
) (func(), error) {
	if g == nil || ctx == nil || policy.MaxInFlight < 1 ||
		policy.MaxQueued < policy.MaxInFlight || policy.MaxMessagesPerSecond < 1 || rateKey == "" {
		return nil, ErrCapacityExceeded
	}

	g.mu.Lock()
	if g.active+g.pending >= policy.MaxQueued {
		g.mu.Unlock()
		return nil, ErrCapacityExceeded
	}
	share, admit := g.rateShare(policy.MaxMessagesPerSecond, rateKey)
	if !admit || !g.takeRateToken(share, rateKey) {
		g.mu.Unlock()
		return nil, ErrRateExceeded
	}
	if g.active < policy.MaxInFlight {
		g.active++
		g.mu.Unlock()
		return g.releaseFunc(), nil
	}
	g.pending++
	notify := g.notify
	g.mu.Unlock()

	for {
		select {
		case <-ctx.Done():
			g.mu.Lock()
			g.pending--
			g.signal()
			g.mu.Unlock()
			return nil, ctx.Err()
		case <-notify:
			g.mu.Lock()
			if g.active < policy.MaxInFlight {
				g.pending--
				g.active++
				g.mu.Unlock()
				return g.releaseFunc(), nil
			}
			notify = g.notify
			g.mu.Unlock()
		}
	}
}

func (g *capacityGate) releaseFunc() func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			g.mu.Lock()
			if g.active > 0 {
				g.active--
			}
			g.signal()
			g.mu.Unlock()
		})
	}
}

func (g *capacityGate) signal() {
	close(g.notify)
	g.notify = make(chan struct{})
}

// rateShare resolves how many messages per second this replica may admit for
// the deployed revision. Without a quota coordinator that is the declared rate,
// enforced per replica. With one it is this replica's claimed share, and admit
// is false during the window after a digest change in which the replica has not
// yet claimed — the new revision claims before it admits.
//
// The caller holds g.mu. The coordinator's Share is a mutex and a map lookup
// and never calls back into the gate, so there is no lock-order hazard and no
// I/O inside the admission path.
func (g *capacityGate) rateShare(declared int, key string) (int, bool) {
	if g.quota == nil {
		return declared, true
	}
	return g.quota.Share(key, declared)
}

// takeRateToken is the continuous-refill bucket. Burst equals the refill rate,
// so a replica holding a share of S admits at most S in any second and at most
// S instantaneously.
//
// The bucket is seeded full once, on the process's first frame, and is NOT
// reset when the deployed revision changes. Before slice 4.4e a change of rate
// key refilled it to the brim, so every rolling redeploy handed the new
// revision a fresh full burst on top of whatever the old one had just
// admitted. That was harmless while the rate was per-replica anyway; under a
// deployment-wide quota it is a real over-admission window at exactly the
// moment a deployment is least stable. Now the balance simply carries — a
// token is an allowance for one message and does not change meaning when the
// revision does — clamped to the current share, so a revision that lowers the
// rate takes effect on the next frame rather than after the old burst drains.
//
// Seeding full on the first frame is bounded: a fresh replica's share is a
// slice of the deployment's declared rate, and the live shares sum to it, so
// the aggregate instantaneous burst across a rollout stays under the declared
// rate however many replicas start at once.
func (g *capacityGate) takeRateToken(share int, key string) bool {
	now := g.now().UTC()
	if now.IsZero() || share < 1 {
		return false
	}
	if g.last.IsZero() {
		g.tokens = float64(share)
		g.last = now
	}
	g.rateKey = key
	if elapsed := now.Sub(g.last).Seconds(); elapsed > 0 {
		g.tokens += elapsed * float64(share)
		g.last = now
	}
	if g.tokens > float64(share) {
		g.tokens = float64(share)
	}
	if g.tokens < 1 {
		return false
	}
	g.tokens--
	return true
}
