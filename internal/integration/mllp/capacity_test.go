package mllp

import (
	"context"
	"errors"
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

func TestCapacityGateBoundsActiveQueuedAndRate(t *testing.T) {
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	gate := newCapacityGate(func() time.Time { return now })
	policy := integration.CapacityPolicy{MaxInFlight: 1, MaxQueued: 2, MaxMessagesPerSecond: 2}
	first, err := gate.acquire(context.Background(), policy, "release-a")
	if err != nil {
		t.Fatal(err)
	}

	queuedCtx, cancel := context.WithCancel(context.Background())
	queued := make(chan error, 1)
	go func() {
		release, err := gate.acquire(queuedCtx, policy, "release-a")
		if release != nil {
			release()
		}
		queued <- err
	}()
	time.Sleep(10 * time.Millisecond)
	if _, err := gate.acquire(context.Background(), policy, "release-a"); !errors.Is(err, ErrCapacityExceeded) {
		t.Fatalf("got %v", err)
	}
	cancel()
	if err := <-queued; !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
	first()

	if _, err := gate.acquire(context.Background(), policy, "release-a"); !errors.Is(err, ErrRateExceeded) {
		t.Fatalf("got %v", err)
	}
	now = now.Add(time.Second)
	release, err := gate.acquire(context.Background(), policy, "release-a")
	if err != nil {
		t.Fatal(err)
	}
	release()
}

func TestCapacityGateResetsRateForNewRelease(t *testing.T) {
	now := time.Now()
	gate := newCapacityGate(func() time.Time { return now })
	policy := integration.CapacityPolicy{MaxInFlight: 1, MaxQueued: 1, MaxMessagesPerSecond: 1}
	release, err := gate.acquire(context.Background(), policy, "release-a")
	if err != nil {
		t.Fatal(err)
	}
	release()
	release, err = gate.acquire(context.Background(), policy, "release-b")
	if err != nil {
		t.Fatalf("new immutable release should reset capacity key: %v", err)
	}
	release()
}
