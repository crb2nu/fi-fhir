### 2026-08-09: Distribute the MLLP rate across replicas with a durable lease-partitioned quota, not a per-frame counter

- Decision:
  - **Option A — durable lease-partitioned quota.** Each MLLP replica claims a
    share of the deployment's declared `max_messages_per_second` from a durable
    per-deployment, per-revision-digest record, refills its in-memory bucket
    from that share, and releases the claim on shutdown. **Admission itself
    stays an in-memory, O(1), lock-free-of-PostgreSQL decision** — there is no
    database round trip on the MLLP hot path, and slice 4.4e therefore cannot
    regress budget 1 (per-frame latency) while fixing budget 2 (deployment-wide
    throughput).
  - **Claim interval: 2 seconds; lease TTL: 6 seconds** (three renewals of
    headroom). Both are constants in `internal/integration/mllp`, documented in
    `docs/operations/PRODUCTION-MLLP.md`, and not environment-configurable —
    capacity policy lives on the deployed revision
    (`pkg/integration/deployment.go:74-78`) and this is its distribution
    mechanism, not a separate knob. At the reference profile's 250 msg/s that is
    one query per replica per 500 frames, ~0.4% overhead.
  - **Share arithmetic**: a renewal reaps claims whose `claim_expires_at` has
    passed, upserts the caller's claim, counts the live holders `N`, and grants
    `floor(rate / N)` with the first `rate mod N` holders (ordered by holder id)
    granted one extra. The sum of live shares is therefore exactly the declared
    rate, and the aggregate is bounded by construction rather than by
    convention. A holder always gets at least 1 msg/s.
  - **Fail-closed degraded share**: a replica that cannot read or renew its
    claim keeps its last granted share until the lease expires, then drops to
    `max(1, floor(rate / 10))` — the documented conservative share. It never
    degrades to the full declared rate, and never to zero, which would
    black-hole a live listener that is the deployment's only survivor. The
    residual: if a deployment runs more than ten replicas **and** every one of
    them loses PostgreSQL simultaneously, the aggregate can exceed the declared
    rate — bounded, documented, and strictly better than today's unbounded
    `N ×`.
  - **The quota pool is keyed on the deployment — `(tenant_id, definition_id)`
    — and not on the revision digest.** `.loom/33`'s task 3 said
    "per-deployment, per-revision-digest quota state", which is the obvious
    reading given that capacity is declared on the deployed revision. It is
    wrong, and it defeats one of the lane's own acceptance criteria: a rolling
    redeploy runs two digests at once, two digest-keyed pools each admit the
    full declared rate, and the deployment bursts to twice it for the length of
    every rollout. The digest is recorded **on the claim row** instead, as
    attribution, so an operator can still see which revision each holder is
    serving. `.loom/33` task 3 is corrected in the same commit as this entry.
  - **Redeploy (`capacity.go:109-113`, correction 36): stop resetting the
    bucket at all.** The bucket is seeded full exactly once, on the process's
    first frame, and the balance then carries across every revision change,
    clamped to the current share. Two alternatives were considered and both are
    worse. *Resetting to full per digest* is the defect itself: it hands each
    rolling redeploy a fresh burst on top of what was just admitted. *Starting
    empty per digest* fixes that but transiently NAKs the first frame after
    every redeploy — and, because a fresh process is also a fresh key, the
    first frame a new replica ever serves, which is a gratuitous regression for
    the single-replica case. Seeding once is bounded in a way that per-digest
    refilling is not: what it seeds is this replica's *share*, and the live
    shares sum to the declared rate, so the aggregate instantaneous burst across
    a rollout stays under it however many replicas start at once.
  - **Claim before admitting.** A replica with no claim yet refuses rather than
    admitting, so a scale-up cannot admit at the full declared rate while it
    waits for its share. This is enforced by the quota coordinator, not by the
    bucket: the bucket has no way to know whether a share is authoritative.
  - **No GraphQL surface.** The schema stays frozen for Sprint 5; capacity is
    server-owned deployment config and needs no root field.
- Rationale:
  - The repo's own precedents argue both ways and neither transfers.
    `.loom/40-decisions.md` (2026-08-08, slice 4.1e) rejected `pg_advisory_lock`
    for the autoroute notifier because "a lock serialises scanners without
    making the *decision* durable"; the same entry accepted the sweeper's
    duplicate scan as benign because its `UPDATE` is idempotent. Rate limiting
    is **neither** idempotent nor safely serialisable, and unlike both
    precedents it sits on a per-frame hot path.
  - Option A preserves the hot path. The claim/lease shape is not novel here:
    `internal/integration/processor/migrations/0002_delivery_reliability.sql:22-45`
    already ships `status`/`lease_owner`/`lease_expires_at` with a shape CHECK
    and a partial index, and the team has built, tested, and operated it.
  - It degrades in the safe direction. A replica that cannot reach PostgreSQL
    falls back to a fraction of the rate, not to the whole of it — the opposite
    of today, where a replica that cannot reach PostgreSQL could not have
    resolved a binding at all but, once running, enforces the full declared rate
    on its own.
- Alternatives considered:
  - **B. Per-frame durable counter** — every admission increments a durable
    counter under a row lock. Exact, and rejected: a database round trip on the
    MLLP hot path at 250+ msg/s under a row lock shared by every replica turns a
    rate limiter into a throughput ceiling. It would fail budget 1 in the act of
    fixing budget 2, and "move the state to PostgreSQL" read literally is this
    option — which is precisely why the lane's riskiest assumption is written
    down.
  - **C. Advisory-lock-serialised admission** — rejected on sight. It serialises
    the hot path across replicas without making anything durable: the exact
    shape the 4.1e notifier decision already rejected, in a much hotter loop.
  - **D. Leave it per-replica and divide the declared rate by the replica count
    in config** — the status quo, which
    `docs/operations/PRODUCTION-MLLP.md:51-59` currently recommends. Rejected:
    the deployment does not know its replica count, autoscaling breaks the
    division silently, and a correct-looking config becomes wrong after a scale
    event with no signal. It also composes badly with the existing full-bucket
    reset on digest change (`capacity.go:109-113`), which hands each new replica
    of a rolling redeploy a fresh full share of an already-divided rate.
  - **Keying the quota pool on the revision digest as well as the deployment**
    (rejected: see the keying bullet above — it re-creates the redeploy burst
    the lane exists to close, in the mechanism meant to close it.)
  - **A conservative share of `rate / last known N` instead of `rate / 10`** —
    rejected as fallback: the last known `N` is exactly the number a partitioned
    replica has no way to refresh, and a deployment that scaled up during the
    partition would over-admit by the scale factor. A fixed, documented divisor
    is worse in the common case and better in the failure case, which is the
    right trade for a fail-closed default.
- Consequences:
  - Bursty under uneven load: an idle replica holds quota a busy one wants for
    up to one claim interval (2s) plus the lease TTL (6s) if it dies rather than
    releasing. This is the price of keeping admission in memory, and it is the
    documented tuning knob.
  - `docs/operations/PRODUCTION-MLLP.md:42-71` is rewritten. "This is documented
    behavior, not a pending bug fix" and the two operator workarounds (divide by
    replica count / accept the multiple) are replaced by the new contract, with
    an honest statement of what remains per-replica: `MaxInFlight` and
    `MaxQueued` are legitimately per-replica resource bounds and are unchanged,
    as is `MaxConnections`, which comes from the mounted source JSON.
  - `pkg/integration/deployment.go:60-73`'s `CapacityPolicy` doc comment, which
    currently tells operators to divide by the replica count, is rewritten to
    match.
  - `internal/integration/lifecycle/migrations/0002_*.sql` consumes the lifecycle
    ledger's next free number and bumps `SchemaVersion`. The lifecycle ledger is
    unfrozen for Lane S5-D alone this sprint.
  - `serve` gains one background component, which is what forces the `errCh`
    capacity repair at `cmd/fi-fhir/main.go:5238` (correction 59: nine senders
    against a capacity of ten, and `waitForBackgroundStops` returns early on the
    first non-nil error).
  - Slice 4.4b (Lane S5-A) can measure budget 2 on the two-replica reference
    profile against a number that means something. Measuring it before this
    lands would certify nothing, which is why S5-D merges before S5-A.
- Sources:
  - [S1] `.loom/33-sprint5-execution-specs.md` — Lane S5-D, the distribution decision; corrections 35-39, 11, 36, 59
  - [S2] `internal/integration/mllp/capacity.go:17-126` — the in-memory gate and its digest-keyed reset
  - [S3] `docs/operations/PRODUCTION-MLLP.md:42-71` — the contract being replaced
  - [S4] `.loom/40-decisions.md` (2026-08-08, slice 4.1e) — the `pg_advisory_lock` rejection and the benign-duplicate acceptance
  - [S5] `internal/integration/processor/migrations/0002_delivery_reliability.sql:22-45` — the existing lease idiom
  - [S6] `pkg/integration/deployment.go:60-78` — `CapacityPolicy` and where capacity is declared
