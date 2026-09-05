package mllp

import (
	"context"
	"errors"
	"sort"
	"sync"
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

var errQuotaStoreDown = errors.New("quota store is unreachable")

// memoryQuotaStore is the reference semantics of the durable quota: one pool
// per deployment, expired claims reaped on every call, holders counted inside
// the same critical section that records the caller's claim, and the share
// computed by partitionShare over holder ids. The PostgreSQL store must agree
// with it; keeping the semantics in one readable place is what makes that
// comparison possible.
type memoryQuotaStore struct {
	mu      sync.Mutex
	now     func() time.Time
	fail    error
	claims  map[QuotaKey]map[string]time.Time
	digests map[QuotaKey]map[string]string
	calls   int
}

func newMemoryQuotaStore(clock func() time.Time) *memoryQuotaStore {
	return &memoryQuotaStore{
		now:     clock,
		claims:  make(map[QuotaKey]map[string]time.Time),
		digests: make(map[QuotaKey]map[string]string),
	}
}

func (s *memoryQuotaStore) Claim(
	_ context.Context,
	key QuotaKey,
	holderID string,
	revisionDigest string,
	declaredRate int,
	lease time.Duration,
) (QuotaClaim, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	if s.fail != nil {
		return QuotaClaim{}, s.fail
	}
	now := s.now()
	holders, ok := s.claims[key]
	if !ok {
		holders = make(map[string]time.Time)
		s.claims[key] = holders
		s.digests[key] = make(map[string]string)
	}
	for id, expiry := range holders {
		if !now.Before(expiry) {
			delete(holders, id)
			delete(s.digests[key], id)
		}
	}
	expiresAt := now.Add(lease)
	holders[holderID] = expiresAt
	s.digests[key][holderID] = revisionDigest

	live := make([]string, 0, len(holders))
	for id := range holders {
		live = append(live, id)
	}
	sort.Strings(live)
	index := sort.SearchStrings(live, holderID)
	return QuotaClaim{
		Share:     partitionShare(declaredRate, len(live), index),
		Holders:   len(live),
		ExpiresAt: expiresAt,
	}, nil
}

func (s *memoryQuotaStore) Release(_ context.Context, key QuotaKey, holderID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	if s.fail != nil {
		return s.fail
	}
	delete(s.claims[key], holderID)
	delete(s.digests[key], holderID)
	return nil
}

func (s *memoryQuotaStore) queryCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func (s *memoryQuotaStore) holderCount(key QuotaKey) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.claims[key])
}

func (s *memoryQuotaStore) recordedDigest(key QuotaKey, holderID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.digests[key][holderID]
}

func TestPartitionShareSumsToTheDeclaredRate(t *testing.T) {
	for _, declared := range []int{1, 2, 3, 7, 100, 250, 1000} {
		for holders := 1; holders <= 12; holders++ {
			total := 0
			for index := 0; index < holders; index++ {
				share := partitionShare(declared, holders, index)
				if share < 1 {
					t.Fatalf("declared=%d holders=%d index=%d granted %d: no holder may be starved",
						declared, holders, index, share)
				}
				total += share
			}
			if holders <= declared && total != declared {
				t.Fatalf("declared=%d holders=%d: shares sum to %d, want exactly the declared rate",
					declared, holders, total)
			}
			// More replicas than messages per second is a misconfiguration and
			// the only case the sum may exceed the declared rate; it may never
			// exceed one per holder.
			if holders > declared && total != holders {
				t.Fatalf("declared=%d holders=%d: shares sum to %d, want one each", declared, holders, total)
			}
		}
	}
}

func TestQuotaCoordinatorClaimsBeforeItAdmits(t *testing.T) {
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	store := newMemoryQuotaStore(func() time.Time { return now })
	coordinator := newTestCoordinator(t, store, "replica-a", func() time.Time { return now })

	// The very first frame for a deployment has no claim behind it. A replica
	// that admitted here would be admitting at the full declared rate while a
	// scale-up or a redeploy was still settling.
	if share, admit := coordinator.Share(testDigest, 100); admit || share != 0 {
		t.Fatalf("an unclaimed replica returned share=%d admit=%v, want 0/false", share, admit)
	}
	if store.queryCount() != 0 {
		t.Fatalf("the admission path made %d store calls, want 0", store.queryCount())
	}

	coordinator.renew(context.Background())

	share, admit := coordinator.Share(testDigest, 100)
	if !admit || share != 100 {
		t.Fatalf("the only holder should hold the whole rate: share=%d admit=%v", share, admit)
	}
	if got := store.recordedDigest(coordinator.key, "replica-a"); got != testDigest {
		t.Fatalf("claim recorded digest %q, want %q — the digest is attribution on the claim", got, testDigest)
	}
}

func TestQuotaCoordinatorDegradesRatherThanBlackHolingOrOverAdmitting(t *testing.T) {
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	store := newMemoryQuotaStore(func() time.Time { return now })
	store.fail = errQuotaStoreDown
	coordinator := newTestCoordinator(t, store, "replica-a", func() time.Time { return now })

	if _, admit := coordinator.Share(testDigest, 100); admit {
		t.Fatal("a replica must not admit before it has even tried to claim")
	}
	coordinator.renew(context.Background())

	share, admit := coordinator.Share(testDigest, 100)
	if !admit {
		t.Fatal("an unreachable store must not black-hole a live listener")
	}
	if share == 100 {
		t.Fatal("an unreachable store must not leave the replica admitting at the full declared rate")
	}
	if want := degradedShare(100); share != want {
		t.Fatalf("degraded share is %d, want the documented conservative share %d", share, want)
	}
	if !coordinator.Degraded() {
		t.Fatal("the replica should report itself degraded so the condition is observable")
	}
}

func TestQuotaCoordinatorSurvivesOneLostRenewalThenDegrades(t *testing.T) {
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	store := newMemoryQuotaStore(clock)
	coordinator := newTestCoordinator(t, store, "replica-a", clock)

	coordinator.Share(testDigest, 100)
	coordinator.renew(context.Background())
	if share, _ := coordinator.Share(testDigest, 100); share != 100 {
		t.Fatalf("initial claim granted %d, want 100", share)
	}

	// One lost round trip inside the lease is a non-event: the existing grant
	// stands. A throughput cliff on every transient error would be worse than
	// the problem the lease solves.
	store.fail = errQuotaStoreDown
	now = now.Add(DefaultClaimInterval)
	coordinator.renew(context.Background())
	if share, admit := coordinator.Share(testDigest, 100); !admit || share != 100 {
		t.Fatalf("a single lost renewal dropped the share to %d (admit=%v)", share, admit)
	}
	if coordinator.Degraded() {
		t.Fatal("a replica inside its lease is not degraded")
	}

	// Past the lease it degrades, and only then.
	now = now.Add(DefaultLeaseTTL)
	share, admit := coordinator.Share(testDigest, 100)
	if !admit || share != degradedShare(100) {
		t.Fatalf("after the lease lapsed: share=%d admit=%v, want %d/true", share, admit, degradedShare(100))
	}
}

func TestQuotaCoordinatorIsKeyedOnTheDeploymentNotTheRevisionDigest(t *testing.T) {
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	store := newMemoryQuotaStore(clock)
	coordinator := newTestCoordinator(t, store, "replica-a", clock)

	coordinator.Share(testDigest, 100)
	coordinator.renew(context.Background())

	// A rolling redeploy. If the pool were keyed on the digest, this would open
	// a second pool and the deployment would admit twice its declared rate for
	// the length of the rollout — the exact thing a deployment-wide bucket is
	// for.
	const nextDigest = "sha256:fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210"
	coordinator.Share(nextDigest, 100)
	coordinator.renew(context.Background())

	if holders := store.holderCount(coordinator.key); holders != 1 {
		t.Fatalf("the deployment has %d holders after a digest change, want 1", holders)
	}
	if len(store.claims) != 1 {
		t.Fatalf("a digest change opened %d quota pools, want 1 per deployment", len(store.claims))
	}
	if got := store.recordedDigest(coordinator.key, "replica-a"); got != nextDigest {
		t.Fatalf("claim still attributed to %q after the redeploy, want %q", got, nextDigest)
	}
}

func TestQuotaCoordinatorReleasesItsShareOnShutdown(t *testing.T) {
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	store := newMemoryQuotaStore(clock)
	coordinator := newTestCoordinator(t, store, "replica-a", clock)
	coordinator.Share(testDigest, 100)

	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan error, 1)
	go func() { stopped <- coordinator.Run(ctx) }()

	deadline := time.Now().Add(2 * time.Second)
	for store.holderCount(coordinator.key) != 1 {
		if time.Now().After(deadline) {
			t.Fatal("the claim loop never claimed a share")
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	select {
	case err := <-stopped:
		if err != nil {
			t.Fatalf("run: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the claim loop did not stop")
	}
	// Waiting out the lease would work too, but a graceful shutdown must hand
	// the quota back at once so a rolling redeploy's next replica gets it.
	if holders := store.holderCount(coordinator.key); holders != 0 {
		t.Fatalf("%d claims survived shutdown, want 0", holders)
	}
}

// TestQuotaBoundsTheDeploymentRateAcrossTwoReplicas is the in-memory half of
// the lane's kill-test: two replicas of one deployment declaring 100 msg/s
// admit at most 100 in aggregate over a measured second, where before slice
// 4.4e they admitted 200. The durable half — the same assertion against
// PostgreSQL over TCP — belongs with the migration.
func TestQuotaBoundsTheDeploymentRateAcrossTwoReplicas(t *testing.T) {
	const declared = 100
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	store := newMemoryQuotaStore(clock)

	replicas := []*QuotaCoordinator{
		newTestCoordinator(t, store, "replica-a", clock),
		newTestCoordinator(t, store, "replica-b", clock),
	}
	gates := []*capacityGate{
		newCapacityGate(clock, replicas[0]),
		newCapacityGate(clock, replicas[1]),
	}
	policy := integration.CapacityPolicy{
		MaxInFlight: declared * 4, MaxQueued: declared * 8, MaxMessagesPerSecond: declared,
	}

	// Both replicas register and claim. Each holds half.
	for _, replica := range replicas {
		replica.Share(testDigest, declared)
	}
	for _, replica := range replicas {
		replica.renew(context.Background())
	}
	// The second claim changed the holder count, so the first replica's share
	// is stale until it renews — which is the claim interval's cost, and it
	// errs downward.
	for _, replica := range replicas {
		replica.renew(context.Background())
	}
	for index, replica := range replicas {
		if share, _ := replica.Share(testDigest, declared); share != declared/2 {
			t.Fatalf("replica %d holds %d, want %d", index, share, declared/2)
		}
	}

	queriesBefore := store.queryCount()
	admitted := 0
	// Drain the seeded burst first, then measure one second in ten slices.
	for _, gate := range gates {
		admitted += drainGate(t, gate, policy)
	}
	burst := admitted
	if burst > declared {
		t.Fatalf("aggregate instantaneous burst was %d against a declared %d", burst, declared)
	}
	const slices = 10
	windowAdmitted := 0
	for i := 0; i < slices; i++ {
		now = now.Add(time.Second / slices)
		for _, gate := range gates {
			windowAdmitted += drainGate(t, gate, policy)
		}
	}
	if windowAdmitted > declared {
		t.Fatalf("two replicas admitted %d msg in one second against a declared %d/s; "+
			"before slice 4.4e this was %d", windowAdmitted, declared, 2*declared)
	}
	if windowAdmitted < declared-declared/50 {
		t.Fatalf("two replicas admitted only %d msg/s against a declared %d/s: the quota is leaking capacity",
			windowAdmitted, declared)
	}
	// Admission remains an in-memory decision: not one query was made while
	// more than a thousand frames were admitted or refused.
	if queries := store.queryCount() - queriesBefore; queries != 0 {
		t.Fatalf("admitting %d frames cost %d store queries, want 0", burst+windowAdmitted, queries)
	}
	t.Logf("two replicas admitted %d msg/s against a declared %d msg/s with %d store queries",
		windowAdmitted, declared, store.queryCount()-queriesBefore)
}

func newTestCoordinator(
	t *testing.T,
	store QuotaStore,
	holderID string,
	clock func() time.Time,
) *QuotaCoordinator {
	t.Helper()
	coordinator, err := NewQuotaCoordinator(QuotaConfig{
		Store: store, TenantID: "tenant-a", DefinitionID: "definition-a",
		HolderID: holderID, ClaimInterval: time.Millisecond, LeaseTTL: DefaultLeaseTTL, Clock: clock,
	})
	if err != nil {
		t.Fatalf("build coordinator: %v", err)
	}
	return coordinator
}

// drainGate admits until the rate gate refuses, and returns the count.
func drainGate(t *testing.T, gate *capacityGate, policy integration.CapacityPolicy) int {
	t.Helper()
	ceiling := policy.MaxMessagesPerSecond * 10
	for admitted := 0; admitted <= ceiling; admitted++ {
		release, err := gate.acquire(context.Background(), policy, testDigest)
		if err == nil {
			release()
			continue
		}
		if errors.Is(err, ErrRateExceeded) {
			return admitted
		}
		t.Fatalf("after %d admissions the gate refused with %v", admitted, err)
	}
	t.Fatalf("the gate admitted more than %d frames without refusing", ceiling)
	return 0
}
