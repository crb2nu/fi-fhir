# RALPH Slice Handoff: Phase 2 Slice 2.1 Versioned Deployment Lifecycle

## Slice Summary

- Milestone: Phase 2 production channel runtime / Engine Beta
- Slice: versioned integration deployment lifecycle
- Status: complete, merged, and independently verified on the default branch

## What Landed

- Immutable integration revisions can carry digest-bound connection-validation
  freshness, continuous or cron schedules, health thresholds, and capacity.
- PostgreSQL owns the closed draft/validate/approve/publish/deploy/pause/resume/
  retire state machine with optimistic versions, append-only validation and
  transition evidence, immutable releases, health projection, and exact runnable
  resolution only while deployed.
- The required PostgreSQL 16 job proves the full lifecycle, failed validation,
  32-caller concurrency, immutable-row rejection, restart reconstruction, and
  raw/secret leakage boundary.
- MR `!101` merged as `a95bb44f`; pipeline `19014` passed 32/32 and lifecycle job
  `183463` passed. Main lifecycle job `183702` independently repeated the proof.
- The first main run exposed an existing durable-receipt arbitration race. A
  concurrent insert could conflict on the deterministic receipt primary key
  before the named tenant/idempotency constraint. MR `!102` merged as `bfc24357`
  after changing insertion to arbitrate either unique key, followed by the same
  authoritative lookup and fail-closed fingerprint validation.
- MR `!102` pipeline `19045` passed 24/24. Final main pipeline `19052` passed
  26/26, including durable-submission job `183938` and lifecycle job `183940`.

## Proof and Artifacts

- Feature MR: `https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/101`
- Feature pipeline: `https://gitlab.flexinfer.ai/libs/fi-fhir/-/pipelines/19014`
- Recovery MR: `https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/102`
- Recovery pipeline: `https://gitlab.flexinfer.ai/libs/fi-fhir/-/pipelines/19045`
- Final main pipeline: `https://gitlab.flexinfer.ai/libs/fi-fhir/-/pipelines/19052`
- Published API image:
  `sha256:c87037a14869477d6be057179ee19113bd0e83a3a9b3031763d6bd17aa6878b6`
- Key files:
  - `pkg/integration/deployment.go`
  - `internal/integration/lifecycle/`
  - `internal/integration/processor/postgres_submission.go`
  - `.gitlab-ci.yml`
  - `docs/operations/INTEGRATION-DEPLOYMENT-LIFECYCLE.md`

## What Is Still Open

- `serve`, GraphQL, and Mapping Studio do not mutate or consume the lifecycle
  catalog.
- Production GitOps activation remains intentionally pending.
- MLLP, destination workers/replay, S3/SFTP runtime wiring, IDE lifecycle
  controls, staged rollout, and multi-tenant hosting remain later slices.

## Next Action

Implement Slice 2.2 production MLLP with bounded concurrency, framing/timeouts,
TLS/client policy, and durable ACK/NACK. It must consume only the lifecycle
catalog's deployed runnable binding and must fail closed while paused or retired.

## Context

- Iteration plan:
  `.loom/iteration-plan-phase-2-slice-2-1-versioned-deployment-lifecycle.md`
- Product spec: `.loom/20-product-spec-integration-engine-ide-completion.md`
- Implementation plan: `.loom/30-implementation-plan-integration-engine-ide-completion.md`
- Decision log: `.loom/40-decisions.md`
- Persistent agent-context was unavailable during this slice; this handoff and
  the synchronized repository documents are the durable context record.
