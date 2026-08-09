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
	gate := newCapacityGate(func() time.Time { return now }, nil)
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

// TestCapacityGateDoesNotRefillTheBucketForANewRelease inverts what this test
// asserted before slice 4.4e.
//
// It used to require that a new immutable release reset the rate key, refilling
// the bucket to the brim. That is the behaviour correction 36 identifies: every
// rolling redeploy handed the new revision a fresh full burst on top of what
// the old one had just admitted. Harmless while the rate was enforced per
// replica anyway; a real over-admission window once the rate is a
// deployment-wide quota, at exactly the moment a deployment is least stable.
//
// The balance now carries across the release change, so the second release
// starts where the first one left off.
func TestCapacityGateDoesNotRefillTheBucketForANewRelease(t *testing.T) {
	now := time.Now()
	gate := newCapacityGate(func() time.Time { return now }, nil)
	policy := integration.CapacityPolicy{MaxInFlight: 1, MaxQueued: 1, MaxMessagesPerSecond: 1}
	release, err := gate.acquire(context.Background(), policy, "release-a")
	if err != nil {
		t.Fatal(err)
	}
	release()

	if _, err := gate.acquire(context.Background(), policy, "release-b"); !errors.Is(err, ErrRateExceeded) {
		t.Fatalf("a new release must not refill the bucket: got %v, want %v", err, ErrRateExceeded)
	}

	// The rate still applies to the new release: once a token has actually
	// refilled, the frame is admitted.
	now = now.Add(time.Second)
	release, err = gate.acquire(context.Background(), policy, "release-b")
	if err != nil {
		t.Fatalf("refill must still serve the new release: %v", err)
	}
	release()
}

// TestCapacityGateSeedsAFullBucketOnTheFirstFrame pins the other half of the
// change above: a fresh process serves immediately rather than transiently
// NAKing its first frame while the bucket fills. The seed is bounded because
// what it seeds is this replica's share, and the live shares sum to the
// deployment's declared rate.
func TestCapacityGateSeedsAFullBucketOnTheFirstFrame(t *testing.T) {
	now := time.Now()
	gate := newCapacityGate(func() time.Time { return now }, nil)
	policy := integration.CapacityPolicy{MaxInFlight: 4, MaxQueued: 8, MaxMessagesPerSecond: 3}
	for admitted := 0; admitted < policy.MaxMessagesPerSecond; admitted++ {
		release, err := gate.acquire(context.Background(), policy, "release-a")
		if err != nil {
			t.Fatalf("frame %d of the initial burst: %v", admitted+1, err)
		}
		release()
	}
	if _, err := gate.acquire(context.Background(), policy, "release-a"); !errors.Is(err, ErrRateExceeded) {
		t.Fatalf("the burst must be bounded by the rate: got %v", err)
	}
}

// TestCapacityGateEnforcesTheClaimedShareNotTheDeclaredRate is the in-memory
// half of the deployment-wide bound: with a quota bound, the gate admits this
// replica's share, and it refuses entirely while the replica has no share yet.
func TestCapacityGateEnforcesTheClaimedShareNotTheDeclaredRate(t *testing.T) {
	now := time.Now()
	quota := &stubRateQuota{share: 2, admit: true}
	gate := newCapacityGate(func() time.Time { return now }, quota)
	policy := integration.CapacityPolicy{MaxInFlight: 8, MaxQueued: 16, MaxMessagesPerSecond: 20}

	admitted := 0
	for {
		release, err := gate.acquire(context.Background(), policy, "release-a")
		if err != nil {
			if !errors.Is(err, ErrRateExceeded) {
				t.Fatalf("unexpected refusal after %d: %v", admitted, err)
			}
			break
		}
		release()
		admitted++
		if admitted > policy.MaxMessagesPerSecond {
			t.Fatalf("gate admitted %d, enforcing the declared rate rather than the claimed share", admitted)
		}
	}
	if admitted != quota.share {
		t.Fatalf("admitted %d, want the claimed share %d", admitted, quota.share)
	}
	if quota.declared != policy.MaxMessagesPerSecond || quota.digest != "release-a" {
		t.Fatalf("quota was asked for %d/%q, want %d/%q",
			quota.declared, quota.digest, policy.MaxMessagesPerSecond, "release-a")
	}

	// A replica with no share yet admits nothing: it claims before it admits.
	quota.admit = false
	now = now.Add(time.Second)
	if _, err := gate.acquire(context.Background(), policy, "release-a"); !errors.Is(err, ErrRateExceeded) {
		t.Fatalf("an unclaimed replica must not admit: got %v", err)
	}
}

type stubRateQuota struct {
	share    int
	admit    bool
	digest   string
	declared int
}

func (s *stubRateQuota) Share(revisionDigest string, declaredRate int) (int, bool) {
	s.digest, s.declared = revisionDigest, declaredRate
	return s.share, s.admit
}
