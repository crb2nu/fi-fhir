### 2026-07-15: Serialize MLLP Admission with Deployment Stops

- Decision:
  - Represent an MLLP listener as a strict, content-addressed UTF-8 source
    revision. The document contains policy and logical secret-binding names,
    never certificate, key, CA, credential, or message bytes.
  - Resolve the lifecycle catalog's deployed binding for every frame. The
    startup registry may supply exact profile/workflow bytes but cannot select
    the executable definition.
  - Repeat exact deployed-release authorization inside the durable submission
    transaction under `FOR SHARE` on the lifecycle snapshot. Pause and retire
    use the existing conflicting `FOR UPDATE` transition lock.
  - Write `AA/CA` only after atomic admission returns an accepted receipt.
    Framing/header/TLS/client failures close without reflection; bounded runtime
    failures receive safe negative codes after a valid header exists.
- Rationale:
  - A preflight runnable lookup alone races an operator stop. Holding the shared
    row lock through commit gives admission and pause/retire one database
    serialization order across processes.
  - Transport acknowledgement means durable acceptance, while downstream
    delivery remains separate at-least-once outbox work.
  - UTF-8 avoids the byte-framing collisions documented for UTF-16/UTF-32.
- Alternatives considered:
  - Select the definition from startup configuration (rejected because pause
    and retirement would be advisory).
  - ACK after parsing or queueing in memory (rejected because process loss could
    discard positively acknowledged work).
  - Hold lifecycle state only in a process cache (rejected because replicas and
    concurrent operators would not share a linearization point).
  - Implement enhanced two-phase commit/application ACK exchange now (deferred;
    v1 supports one configured application or commit response).
- Consequences:
  - MLLP and lifecycle/submission schemas must share PostgreSQL.
  - Profile/workflow artifact storage remains transitional, but artifact
    identity is exact and cannot authorize a non-deployed definition.
  - Plaintext is limited operationally to a protected loopback/sidecar boundary;
    production network exposure requires mutual TLS and reviewed GitOps.
  - Production GitOps activation remains intentionally pending.
- Evidence:
  - Unit and race tests cover framing, ACK safety, TLS, client policy, capacity,
    lifecycle mismatch, and pre-return ACK exclusion.
  - MR `!104` pipeline `19175` passed 33/33; required PostgreSQL 16/TCP job
    `184996` passed. Merge commit `6205fa39` repeated the proof in main job
    `185093`; main pipeline `19193` passed 36/36.
  - CI exposed and closed two test-contract defects before merge: empty
    diagnostics persist as JSON `[]`, and the 32-client duplicate proof now
    declares matching bounded connection/queue capacity.
- Sources:
  - [S1] `internal/integration/mllp/`
  - [S2] `internal/integration/lifecycle/admission.go`
  - [S3] `internal/integration/processor/postgres_submission.go`
  - [S4] `.loom/iteration-plan-phase-2-slice-2-2-production-mllp.md`
  - [S5] `docs/operations/PRODUCTION-MLLP.md`
