### 2026-07-14: Separate Immutable Releases from Optimistic Deployment State

- Decision:
  - Add an optional, digest-bound deployment policy to
    `IntegrationDefinitionRevision`. Existing Slice 1 revisions omit it and keep
    their exact JSON/digest; lifecycle-managed revisions require connection-
    validation freshness, schedule, health, and capacity policy.
  - Persist definition revisions, connection validations, release records, and
    lifecycle events as database-enforced append-only rows. Keep one small
    snapshot mutable under an expected-version predicate.
  - Use the closed state graph draft -> validated -> approved -> published ->
    deployed -> paused/resumed -> retired. Failed validation is recorded but
    does not advance state; stale validation blocks publish, deploy, and resume.
  - Resolve an exact runnable binding only while the release is deployed. Future
    adapters must begin from that server-owned binding rather than a caller or
    mutable current pointer.
- Rationale:
  - Pause/resume and health are operational facts that change independently of
    tested artifact content. Including them in the revision digest would either
    mutate a release or create a new executable identity for every control action.
  - Append-only validation/release/history records make actor, reason, revision,
    and publication evidence reconstructable after restart.
  - A database expected version and one-active-release constraint make concurrent
    operators deterministic across replicas.
- Alternatives considered:
  - Store state directly on the immutable revision (rejected because pause and
    health would invalidate its content identity).
  - Reuse the startup `StaticRegistry` as the deployment catalog (rejected because
    it has no persistence, concurrency, approval, health, or pause boundary).
  - Introduce lifecycle mutations through the legacy GraphQL workflow store
    (rejected because Phase 3 owns IDE/API controls and that store does not bind
    the full integration revision).
- Consequences:
  - Slice 2.2 can add MLLP without inventing artifact selection or channel state.
  - `serve` and the current authenticated HTTP ingress remain on the verified
    static registry until an adapter explicitly consumes runnable catalog state.
  - Staged/canary rollout and shared multi-tenant hosting remain later work.
- Evidence:
  - MR `!101` pipeline `19014` passed 32/32, including required lifecycle job
    `183463`; merge commit `a95bb44f` repeated it in main job `183702`.
  - The first main run exposed a pre-existing concurrent receipt insert that
    could select the deterministic receipt primary key before the named
    tenant/idempotency constraint. MR `!102` replaced the constraint-specific
    insert with `ON CONFLICT DO NOTHING`; the following tenant/idempotency lookup
    and fingerprint validation remain authoritative and fail closed.
  - MR `!102` pipeline `19045` passed 24/24. Final main pipeline `19052` passed
    26/26 with durable-submission job `183938` and lifecycle job `183940` green.
- Sources:
  - [S1] `pkg/integration/deployment.go`
  - [S2] `internal/integration/lifecycle/`
  - [S3] `.loom/iteration-plan-phase-2-slice-2-1-versioned-deployment-lifecycle.md`
  - [S4] `.gitlab-ci.yml`
