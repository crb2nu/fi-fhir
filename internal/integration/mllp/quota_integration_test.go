//go:build integration

package mllp

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lib/pq"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/lifecycle"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

// checkViolation is PostgreSQL's SQLSTATE for a failed CHECK constraint.
const checkViolation = pq.ErrorCode("23514")

// countingQuotaStore wraps the real store so the test can assert how many
// database round trips the whole run cost. The claim that admission stays
// in-memory is only worth as much as the count that proves it.
type countingQuotaStore struct {
	inner QuotaStore
	mu    sync.Mutex
	calls int
	fail  error
}

func (s *countingQuotaStore) Claim(
	ctx context.Context,
	key QuotaKey,
	holderID string,
	revisionDigest string,
	declaredRate int,
	lease time.Duration,
) (QuotaClaim, error) {
	s.mu.Lock()
	s.calls++
	failure := s.fail
	s.mu.Unlock()
	if failure != nil {
		return QuotaClaim{}, failure
	}
	return s.inner.Claim(ctx, key, holderID, revisionDigest, declaredRate, lease)
}

func (s *countingQuotaStore) Release(ctx context.Context, key QuotaKey, holderID string) error {
	s.mu.Lock()
	s.calls++
	s.mu.Unlock()
	return s.inner.Release(ctx, key, holderID)
}

func (s *countingQuotaStore) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func (s *countingQuotaStore) breakRenewal(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fail = err
}

// TestMLLPCapacity_DeploymentWideRateIsBoundedAcrossReplicas is Lane S5-D's
// kill-test, against a real PostgreSQL.
//
// Its negative control is TestMLLPCapacity_TwoReplicasAdmitTwiceTheDeclaredRateToday,
// which drives the identical shape with no quota bound and asserts ~200 — the
// pre-slice behaviour. A change that quietly stopped consulting the quota would
// turn this red and leave the control green, which is what a control is for.
func TestMLLPCapacity_DeploymentWideRateIsBoundedAcrossReplicas(t *testing.T) {
	const declared = 100
	ctx := t.Context()
	db := quotaTestDB(t)

	inner, err := NewPostgresQuotaStore(db)
	if err != nil {
		t.Fatal(err)
	}
	store := &countingQuotaStore{inner: inner}

	// Two clocks, deliberately. The coordinators compare lease expiries that
	// PostgreSQL generated with clock_timestamp(), so their clock has to be
	// real time — offset by the test rather than frozen, or every server-issued
	// lease would look valid forever. The capacity gates only need monotonic
	// steps to drive the refill, so theirs is frozen and stepped explicitly:
	// with a real clock a slow runner would add tokens mid-window and the
	// measurement would drift over the bound it is asserting.
	// One offset per replica, not one shared offset: replicas keep their own
	// clocks, and the point of assertion 3 is that time passing for one of them
	// says nothing about the other. A shared offset would also make every
	// subsequent server-issued lease look expired to both, since the offset
	// permanently outruns the six seconds PostgreSQL grants.
	var offsetA, offsetB atomic.Int64
	clockA := func() time.Time { return time.Now().UTC().Add(time.Duration(offsetA.Load())) }
	clockB := func() time.Time { return time.Now().UTC().Add(time.Duration(offsetB.Load())) }
	step := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	stepClock := func() time.Time { return step }

	replicaA := newTestCoordinator(t, store, "replica-a", clockA)
	replicaB := newTestCoordinator(t, store, "replica-b", clockB)
	gateA := newCapacityGate(stepClock, replicaA)
	gateB := newCapacityGate(stepClock, replicaB)
	policy := integration.CapacityPolicy{
		MaxInFlight: declared * 4, MaxQueued: declared * 8, MaxMessagesPerSecond: declared,
	}
	key := QuotaKey{TenantID: "tenant-a", DefinitionID: "definition-a"}

	// Both replicas register and claim. The second claim changes the holder
	// count, so both renew again before the split has settled — the claim
	// interval's cost, and it errs downward.
	for round := 0; round < 2; round++ {
		for _, replica := range []*QuotaCoordinator{replicaA, replicaB} {
			replica.Share(testDigest, declared)
			replica.renew(ctx)
		}
	}

	// Assertion: the durable shares sum to exactly the declared rate. Not
	// "approximately", and not "each replica believes it has half" — read back
	// out of the table.
	if total, holders := quotaTotals(ctx, t, db, key); total != declared || holders != 2 {
		t.Fatalf("durable shares sum to %d across %d holders, want %d across 2", total, holders, declared)
	}

	// Assertion 1: one measured second across both replicas.
	baseline := store.count()
	burst := drainGate(t, gateA, policy) + drainGate(t, gateB, policy)
	if burst > declared {
		t.Fatalf("aggregate instantaneous burst was %d against a declared %d", burst, declared)
	}
	admitted := 0
	const slices = 10
	for i := 0; i < slices; i++ {
		step = step.Add(time.Second / slices)
		admitted += drainGate(t, gateA, policy)
		admitted += drainGate(t, gateB, policy)
	}
	if admitted > declared {
		t.Fatalf("two replicas admitted %d msg in one second against a declared %d/s", admitted, declared)
	}
	if admitted >= 2*declared-declared/50 {
		t.Fatalf("two replicas admitted %d msg/s: the pre-slice N x behaviour is still in force", admitted)
	}
	if admitted < declared-declared/50 {
		t.Fatalf("two replicas admitted only %d msg/s against a declared %d/s: the quota is leaking capacity",
			admitted, declared)
	}

	// Assertion 5: the cost of admitting was zero round trips. More than a
	// thousand acquire calls ran between these two counts.
	if queries := store.count() - baseline; queries != 0 {
		t.Fatalf("admitting %d frames cost %d database round trips, want 0", burst+admitted, queries)
	}

	// Assertion 4: a revision digest change mid-run does not produce a burst.
	// The measured window left both buckets empty, which is what makes this
	// assertion mean something: before this slice a digest change refilled the
	// bucket to the brim, so the drain below would have returned a full share
	// rather than zero. That refill is what made a rolling redeploy an
	// over-admission window.
	const nextDigest = "sha256:fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210"
	replicaA.Share(nextDigest, declared)
	replicaA.renew(ctx)
	if redeployBurst := drainGate(t, gateA, policy); redeployBurst > 0 {
		t.Fatalf("a revision digest change handed the replica %d fresh tokens, want 0", redeployBurst)
	}
	if _, holders := quotaTotals(ctx, t, db, key); holders != 2 {
		t.Fatalf("a digest change produced %d holders, want 2: the pool is keyed on the deployment", holders)
	}

	// Assertion 3: kill one replica's renewal. Inside the lease nothing
	// changes; past it, the replica degrades to the conservative share and the
	// survivor reclaims the freed quota.
	store.breakRenewal(errQuotaStoreDown)
	offsetB.Add(int64(DefaultClaimInterval))
	replicaB.renew(ctx)
	if share, admit := replicaB.Share(testDigest, declared); !admit || share != declared/2 {
		t.Fatalf("inside its lease the replica holds %d (admit=%v), want %d", share, admit, declared/2)
	}

	offsetB.Add(int64(DefaultLeaseTTL))
	share, admit := replicaB.Share(testDigest, declared)
	if !admit {
		t.Fatal("a replica that lost its claim must not black-hole its listener")
	}
	if share == declared {
		t.Fatal("a replica that lost its claim must not admit at the full declared rate")
	}
	if want := degradedShare(declared); share != want {
		t.Fatalf("degraded to %d, want the documented conservative share %d", share, want)
	}

	// The survivor reclaims. Age the dead replica's row the way a dead replica
	// actually looks to the server — a lease in the past — and let replicaA's
	// ordinary renewal reap it. Deleting the row here instead would prove the
	// test can delete a row; reaping through the shipping query is the point.
	// claimed_at moves with it so the row still satisfies the share CHECK.
	if _, err := db.ExecContext(ctx, `
		UPDATE integration_mllp_rate_claims
		SET claimed_at = clock_timestamp() - interval '30 seconds',
		    expires_at = clock_timestamp() - interval '1 second'
		WHERE tenant_id = $1 AND definition_id = $2 AND holder_id = 'replica-b'
	`, key.TenantID, key.DefinitionID); err != nil {
		t.Fatalf("age the dead replica's lease: %v", err)
	}
	store.breakRenewal(nil)
	replicaA.renew(ctx)
	if share, _ := replicaA.Share(nextDigest, declared); share != declared {
		t.Fatalf("the surviving replica holds %d after the other's lease expired, want the whole %d",
			share, declared)
	}
	if total, holders := quotaTotals(ctx, t, db, key); holders != 1 || total != declared {
		t.Fatalf("after reaping: %d holders totalling %d, want 1 totalling %d", holders, total, declared)
	}

	// A graceful shutdown hands the share back at once rather than after the
	// lease, so a rolling redeploy's replacement does not wait it out.
	replicaA.release()
	if _, holders := quotaTotals(ctx, t, db, key); holders != 0 {
		t.Fatalf("%d claims survived a graceful release, want 0", holders)
	}

	t.Logf("two replicas admitted %d msg/s against a declared %d msg/s (control: %d) "+
		"at a total cost of %d database round trips",
		admitted, declared, 2*declared, store.count())
}

// TestMLLPQuotaStore_ConcurrentClaimsCannotOverGrant proves the bound is
// transactional rather than advisory. Ten replicas claim at once against one
// deployment; if the holder count were read outside the transaction that writes
// the claim, two of them would compute a share against a stale count and the
// total would exceed the declared rate.
func TestMLLPQuotaStore_ConcurrentClaimsCannotOverGrant(t *testing.T) {
	const (
		declared = 100
		replicas = 10
	)
	ctx := t.Context()
	db := quotaTestDB(t)
	store, err := NewPostgresQuotaStore(db)
	if err != nil {
		t.Fatal(err)
	}
	key := QuotaKey{TenantID: "tenant-a", DefinitionID: "definition-b"}

	var wait sync.WaitGroup
	errs := make(chan error, replicas)
	for index := 0; index < replicas; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			holder := fmt.Sprintf("replica-%02d", index)
			// Twice: the first claim of the last replica changes the count for
			// everyone, so a second round is what lets the split settle.
			for round := 0; round < 2; round++ {
				if _, err := store.Claim(ctx, key, holder, testDigest, declared, DefaultLeaseTTL); err != nil {
					errs <- err
					return
				}
			}
		}(index)
	}
	wait.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent claim: %v", err)
		}
	}

	total, holders := quotaTotals(ctx, t, db, key)
	if holders != replicas {
		t.Fatalf("%d holders recorded, want %d", holders, replicas)
	}
	if total > declared {
		t.Fatalf("%d concurrent replicas were granted %d in total against a declared %d: "+
			"the holder count was read outside the transaction that wrote the claim",
			replicas, total, declared)
	}
	t.Logf("%d concurrent replicas hold %d of a declared %d", holders, total, declared)
}

// TestMLLPQuotaStore_RejectsAnOverGrant proves the migration's CHECK is doing
// work rather than decorating the file: no path may record a share larger than
// the rate it is a share of.
func TestMLLPQuotaStore_RejectsAnOverGrant(t *testing.T) {
	ctx := t.Context()
	db := quotaTestDB(t)
	_, err := db.ExecContext(ctx, `
		INSERT INTO integration_mllp_rate_claims (
			tenant_id, definition_id, holder_id, revision_digest,
			declared_rate, granted_share, holders, expires_at
		) VALUES ('tenant-a', 'definition-c', 'replica-a', $1, 100, 101, 1,
			clock_timestamp() + interval '6 seconds')
	`, testDigest)
	if err == nil {
		t.Fatal("a share larger than the declared rate was accepted; the CHECK is not enforcing the bound")
	}
	var pqErr *pq.Error
	if !errors.As(err, &pqErr) || pqErr.Code != checkViolation {
		t.Fatalf("rejected with %v, want a check_violation (%s)", err, checkViolation)
	}
}

func quotaTotals(ctx context.Context, t *testing.T, db *sql.DB, key QuotaKey) (int, int) {
	t.Helper()
	var total, holders sql.NullInt64
	if err := db.QueryRowContext(ctx, `
		SELECT coalesce(sum(granted_share), 0), count(*)
		FROM integration_mllp_rate_claims
		WHERE tenant_id = $1 AND definition_id = $2 AND expires_at > clock_timestamp()
	`, key.TenantID, key.DefinitionID).Scan(&total, &holders); err != nil {
		t.Fatalf("read quota totals: %v", err)
	}
	return int(total.Int64), int(holders.Int64)
}

// quotaTestDB opens an isolated schema with the lifecycle ledger migrated, so
// each proof owns its own integration_mllp_rate_claims.
func quotaTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("POSTGRES_TEST_URL")
	if dsn == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("POSTGRES_TEST_URL is required in CI")
		}
		t.Skip("POSTGRES_TEST_URL is required for MLLP rate quota integration tests")
	}
	schema := fmt.Sprintf("mllp_quota_%d", time.Now().UnixNano())
	createMLLPSchema(t, dsn, schema)
	db := openMLLPDB(t, mllpSchemaDSN(t, dsn, schema))

	// Migrate through the shipping path, not a hand-copied CREATE TABLE. A test
	// carrying its own copy of the schema proves nothing about the one that
	// ships, and it would not have noticed if migration 0002 were never wired
	// into lifecycleMigrations at all.
	catalog, err := lifecycle.NewPostgresCatalog(db, lifecycle.Config{})
	if err != nil {
		t.Fatalf("build lifecycle catalog: %v", err)
	}
	if err := catalog.Migrate(t.Context()); err != nil {
		t.Fatalf("migrate lifecycle ledger: %v", err)
	}
	var applied int
	if err := db.QueryRowContext(t.Context(),
		`SELECT count(*) FROM integration_lifecycle_schema_migrations WHERE version = $1`,
		lifecycle.SchemaVersion,
	).Scan(&applied); err != nil {
		t.Fatalf("read lifecycle ledger: %v", err)
	}
	if applied != 1 {
		t.Fatalf("lifecycle ledger has no row for the declared version %d", lifecycle.SchemaVersion)
	}
	return db
}
