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
	mu      sync.Mutex
	now     func() time.Time
	notify  chan struct{}
	active  int
	pending int
	rateKey string
	tokens  float64
	last    time.Time
}

func newCapacityGate(clock func() time.Time) *capacityGate {
	if clock == nil {
		clock = time.Now
	}
	return &capacityGate{now: clock, notify: make(chan struct{})}
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
	if !g.takeRateToken(policy.MaxMessagesPerSecond, rateKey) {
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

func (g *capacityGate) takeRateToken(limit int, key string) bool {
	now := g.now().UTC()
	if now.IsZero() {
		return false
	}
	if g.rateKey != key || g.last.IsZero() {
		g.rateKey = key
		g.tokens = float64(limit)
		g.last = now
	}
	if elapsed := now.Sub(g.last).Seconds(); elapsed > 0 {
		g.tokens += elapsed * float64(limit)
		if g.tokens > float64(limit) {
			g.tokens = float64(limit)
		}
		g.last = now
	}
	if g.tokens < 1 {
		return false
	}
	g.tokens--
	return true
}
