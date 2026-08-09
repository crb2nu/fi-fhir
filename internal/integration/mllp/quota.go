package mllp

import (
	"context"
	"sync"
	"time"
)

// Lease-partitioned rate quota.
//
// A deployment declares one max_messages_per_second. Before slice 4.4e each
// replica enforced that number on its own, so N replicas admitted N x the
// declared rate — documented behaviour, and a figure that made the product's
// steady-state throughput budget unmeasurable.
//
// The distribution mechanism is a lease, not a counter. Each replica claims a
// share of the declared rate from a durable per-deployment record, refills its
// in-memory bucket from that share, and releases the claim on shutdown.
// Admission itself never touches the store: it stays the same O(1) in-memory
// token take it always was, so the per-frame latency budget is unaffected. A
// per-frame durable counter under a row lock was rejected for exactly that
// reason. See `.loom/40-decisions.md` (2026-08-09, "Distribute the MLLP rate
// across replicas with a durable lease-partitioned quota").
const (
	// DefaultClaimInterval is how often a replica renews its share. At the
	// reference profile's 250 msg/s this is one query per replica per 500
	// frames.
	DefaultClaimInterval = 2 * time.Second

	// DefaultLeaseTTL is how long a granted share survives without renewal:
	// three claim intervals, so a replica tolerates two lost round trips
	// before its quota returns to the pool.
	DefaultLeaseTTL = 6 * time.Second

	// degradedShareDivisor bounds the fail-closed share a replica falls back to
	// when it cannot reach the store. It assumes the deployment may be running
	// up to this many replicas. A replica never degrades to the full declared
	// rate, and never to zero, which would black-hole a live listener that is
	// the deployment's only survivor.
	degradedShareDivisor = 10
)

// QuotaKey identifies one deployment's rate budget.
//
// It is deliberately NOT keyed on the revision digest. Capacity is declared on
// the deployed revision, so keying the pool by digest is the obvious reading —
// and it defeats the requirement it was meant to serve. During a rolling
// redeploy two digests are live at once; two digest-keyed pools each admit the
// full declared rate, and the deployment bursts to twice it for the length of
// the rollout. One pool per deployment, with the digest recorded on each claim
// for attribution, is what actually bounds the aggregate across a redeploy.
type QuotaKey struct {
	TenantID     string
	DefinitionID string
}

func (k QuotaKey) valid() bool {
	return validIdentity(k.TenantID) && validIdentity(k.DefinitionID)
}

// QuotaClaim is the share of a deployment's declared rate granted to one
// holder, and how long the grant survives without renewal. Every field is
// server-owned: the caller proposes nothing.
type QuotaClaim struct {
	Share     int
	Holders   int
	ExpiresAt time.Time
}

// QuotaStore is the durable side of the lease-partitioned quota.
//
// One Claim call reaps expired claims, records the caller's, counts the live
// holders, and returns this holder's share — all in one transaction, because a
// share computed against a holder count that changes before it is written is
// not a bound. Implementations are called from the claim loop, never from the
// admission path.
type QuotaStore interface {
	Claim(
		ctx context.Context,
		key QuotaKey,
		holderID string,
		revisionDigest string,
		declaredRate int,
		lease time.Duration,
	) (QuotaClaim, error)
	Release(ctx context.Context, key QuotaKey, holderID string) error
}

// RateQuota resolves the share of the declared rate this replica may admit.
//
// It is consulted once per frame from inside the capacity gate, so it must not
// block, allocate unboundedly, or perform I/O. admit is false only in the
// window before this replica's first successful or failed claim: a replica
// claims before it admits, so a scale-up does not admit at the full declared
// rate while it waits for its share.
type RateQuota interface {
	Share(revisionDigest string, declaredRate int) (share int, admit bool)
}

// QuotaOutcome reports one claim attempt. Same shape as SubmitResult and
// autoroute.SweeperConfig.Observe: a typed result, an optional non-blocking
// hook, no metrics dependency inside this package. Nothing unbounded is
// carried — the digest is one deployed revision at a time per definition.
type QuotaOutcome struct {
	RevisionDigest string
	DeclaredRate   int
	Share          int
	Holders        int
	Degraded       bool
	Released       bool
	Err            error
}

// QuotaConfig configures one replica's claim loop.
type QuotaConfig struct {
	Store         QuotaStore
	TenantID      string
	DefinitionID  string
	HolderID      string
	ClaimInterval time.Duration
	LeaseTTL      time.Duration
	Clock         func() time.Time
}

// QuotaCoordinator claims and renews this replica's share of the deployment's
// declared rate, and releases it on shutdown.
type QuotaCoordinator struct {
	store         QuotaStore
	key           QuotaKey
	holderID      string
	claimInterval time.Duration
	leaseTTL      time.Duration
	now           func() time.Time

	mu sync.Mutex
	// serving records that at least one frame has been offered. A listener
	// that has never been asked to admit anything holds no quota, so a
	// deployment's share is split across the replicas actually taking traffic.
	serving   bool
	digest    string
	declared  int
	granted   int
	expiresAt time.Time
	// attempted records that the store has been asked at least once. Before
	// the first attempt the replica refuses rather than admitting, so a new
	// replica claims before it admits. After a failed attempt it degrades, so
	// an unreachable store cannot black-hole a listener.
	attempted bool

	wake chan struct{}

	observeMu sync.RWMutex
	observe   func(QuotaOutcome)
}

// NewQuotaCoordinator constructs the claim loop. It performs no I/O.
func NewQuotaCoordinator(config QuotaConfig) (*QuotaCoordinator, error) {
	key := QuotaKey{TenantID: config.TenantID, DefinitionID: config.DefinitionID}
	if config.Store == nil || !key.valid() || config.HolderID == "" {
		return nil, ErrUnavailable
	}
	claimInterval := config.ClaimInterval
	if claimInterval <= 0 {
		claimInterval = DefaultClaimInterval
	}
	leaseTTL := config.LeaseTTL
	if leaseTTL <= 0 {
		leaseTTL = DefaultLeaseTTL
	}
	// A lease no longer than the renewal interval expires between renewals by
	// construction, which would keep every replica permanently degraded.
	if leaseTTL <= claimInterval {
		return nil, ErrUnavailable
	}
	clock := config.Clock
	if clock == nil {
		clock = time.Now
	}
	return &QuotaCoordinator{
		store: config.Store, key: key, holderID: config.HolderID,
		claimInterval: claimInterval, leaseTTL: leaseTTL, now: clock,
		wake: make(chan struct{}, 1),
	}, nil
}

// SetObserver binds an observation hook. It must not block.
func (c *QuotaCoordinator) SetObserver(observe func(QuotaOutcome)) {
	if c == nil {
		return
	}
	c.observeMu.Lock()
	defer c.observeMu.Unlock()
	c.observe = observe
}

func (c *QuotaCoordinator) report(outcome QuotaOutcome) {
	if c == nil {
		return
	}
	c.observeMu.RLock()
	observe := c.observe
	c.observeMu.RUnlock()
	if observe == nil {
		return
	}
	observe(outcome)
}

// Share implements RateQuota. It is the only method on the admission path: one
// mutex, no allocation, no I/O.
func (c *QuotaCoordinator) Share(revisionDigest string, declaredRate int) (int, bool) {
	if c == nil || revisionDigest == "" || declaredRate < 1 {
		return 0, false
	}
	now := c.now()
	c.mu.Lock()
	defer c.mu.Unlock()
	// A digest or rate change is worth claiming against promptly rather than
	// at the next tick: the claim row carries the digest for attribution and
	// the pool's declared rate comes from its holders.
	if !c.serving || c.digest != revisionDigest || c.declared != declaredRate {
		c.serving, c.digest, c.declared = true, revisionDigest, declaredRate
		c.signal()
	}
	if c.granted > 0 && now.Before(c.expiresAt) {
		return c.granted, true
	}
	if !c.attempted {
		return 0, false
	}
	// The lease lapsed or the claim failed. Degrade — never to the full
	// declared rate, and never to zero.
	c.signal()
	return degradedShare(declaredRate), true
}

// Degraded reports whether this replica is currently running on the
// conservative share rather than a live claim.
func (c *QuotaCoordinator) Degraded() bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.attempted && (c.granted < 1 || !c.now().Before(c.expiresAt))
}

// Run claims and renews this replica's share until the context is cancelled,
// then releases the claim so the quota returns to the pool immediately rather
// than after the lease expires.
func (c *QuotaCoordinator) Run(ctx context.Context) error {
	if c == nil || c.store == nil || ctx == nil {
		return ErrUnavailable
	}
	ticker := time.NewTicker(c.claimInterval)
	defer ticker.Stop()
	for {
		c.renew(ctx)
		select {
		case <-ctx.Done():
			c.release()
			return nil
		case <-ticker.C:
		case <-c.wake:
		}
	}
}

// signal wakes the claim loop. The caller holds c.mu. A single buffered slot
// coalesces a burst of changes into one wake-up.
func (c *QuotaCoordinator) signal() {
	select {
	case c.wake <- struct{}{}:
	default:
	}
}

// renew claims or renews this replica's share. A replica that has never been
// offered a frame claims nothing.
func (c *QuotaCoordinator) renew(ctx context.Context) {
	c.mu.Lock()
	serving, digest, declared := c.serving, c.digest, c.declared
	c.mu.Unlock()
	if !serving || ctx.Err() != nil {
		return
	}
	claim, err := c.store.Claim(ctx, c.key, c.holderID, digest, declared, c.leaseTTL)
	c.apply(digest, declared, claim, err)
}

// apply records one claim attempt. A failed renewal does not immediately drop
// the share: the existing grant stands until its lease expires, which is what
// makes a single lost round trip a non-event rather than a throughput cliff.
func (c *QuotaCoordinator) apply(digest string, declared int, claim QuotaClaim, err error) {
	c.mu.Lock()
	c.attempted = true
	if err == nil && claim.Share > 0 {
		c.granted = claim.Share
		c.expiresAt = claim.ExpiresAt
	}
	degraded := c.granted < 1 || !c.now().Before(c.expiresAt)
	share := c.granted
	if degraded {
		share = degradedShare(declared)
	}
	c.mu.Unlock()
	c.report(QuotaOutcome{
		RevisionDigest: digest, DeclaredRate: declared, Share: share,
		Holders: claim.Holders, Degraded: degraded, Err: err,
	})
}

// release hands the share back at shutdown. The run context is already
// cancelled by then, so the release gets its own budget.
func (c *QuotaCoordinator) release() {
	c.mu.Lock()
	serving, digest := c.serving, c.digest
	c.serving, c.granted, c.expiresAt = false, 0, time.Time{}
	c.mu.Unlock()
	if !serving {
		return
	}
	// Budgeted by the lease, not by the claim interval. Releasing is pointless
	// once the lease would have expired anyway, so the TTL is the natural
	// bound — and the claim interval is the wrong one: it can be far shorter
	// than a round trip, which silently turns every graceful release into a
	// timeout and leaves the share stranded until it expires.
	ctx, cancel := context.WithTimeout(context.Background(), c.leaseTTL)
	defer cancel()
	err := c.store.Release(ctx, c.key, c.holderID)
	c.report(QuotaOutcome{RevisionDigest: digest, Released: true, Err: err})
}

// degradedShare is the fail-closed fallback: a fraction of the declared rate,
// never the whole of it and never zero.
func degradedShare(declaredRate int) int {
	share := declaredRate / degradedShareDivisor
	if share < 1 {
		share = 1
	}
	return share
}

// partitionShare splits a declared rate across live holders so the live shares
// sum to exactly the declared rate: everyone gets the floor, and the first
// declaredRate mod holders holders — ordered by holder id, so every replica
// computing this independently reaches the same answer — get one extra.
//
// index is this holder's zero-based rank in that ordering.
//
// The one case where the sum exceeds the declared rate is a deployment running
// more replicas than its declared messages per second, where the floor is zero
// and every holder is still granted one. Bounding that further would
// black-hole replicas; it is a misconfiguration, and it is documented rather
// than silently enforced.
func partitionShare(declaredRate, holders, index int) int {
	if declaredRate < 1 {
		return 0
	}
	if holders < 1 {
		holders = 1
	}
	if index < 0 {
		index = 0
	}
	share := declaredRate / holders
	if index < declaredRate%holders {
		share++
	}
	if share < 1 {
		share = 1
	}
	return share
}
