# RALPH Iteration Plan — Phase 2 Slice 2.1 Versioned Deployment Lifecycle

**Status**: Merged; MR and default-branch proof complete
**Date**: 2026-07-14
**Plan**: `plan-complete-fi-fhir-as-a-production-integration-engine-and-ide-341d98#7`
**Branch**: `codex/phase2-versioned-deployments`

## Riskiest assumption + kill-test

**Load-bearing assumption**: One PostgreSQL-backed lifecycle catalog can keep an
`IntegrationDefinitionRevision` immutable while separately advancing its
deployment state with optimistic concurrency, and can expose that exact revision
to a future source adapter only while its release is deployed.

**Kill test**: A required PostgreSQL 16 integration test completes the exact
`draft -> validate -> approve -> publish -> deploy -> pause -> resume -> retire`
journey within 30 minutes and proves all of the following:

1. a failed connection validation remains auditable but cannot leave `draft`;
2. successful validation is bound to the exact source/integration revision and
   must still be current when approval, publication, and deployment occur;
3. a published release row and revision document reject `UPDATE` and `DELETE`;
4. many callers using one expected version produce exactly one accepted state
   change and bounded optimistic-concurrency conflicts;
5. a fresh database handle after restart returns the byte-identical release and
   append-only transition history; and
6. runtime resolution returns the exact immutable revision only in `deployed`,
   and fails closed in `published`, `paused`, and `retired`.

**Failure mode if the assumption is wrong**: MLLP or batch adapters could execute
mutable/current artifacts, continue after an operator pause, or race lifecycle
commands into an unauditable state. Slice 2.2 must remain blocked until the
catalog/state boundary is redesigned.

**Status**: passed locally where dependencies allowed and in required PostgreSQL
CI. Focused race tests, vet, golangci-lint, the full `go test -count=1 ./...`
suite, documentation validation, and CI test discovery were locally green. MR
`!101` pipeline `19014` passed 32/32; required lifecycle job `183463` completed
the full state, 32-caller race, restart, mutation-rejection, and leakage proof.
Merge commit `a95bb44f` repeated the lifecycle proof in main job `183702`.

That first main pipeline found disconfirming evidence outside the new catalog:
concurrent durable receipt insertion could encounter the deterministic receipt
primary key before the named tenant/idempotency constraint. MR `!102` changed the
insert to `ON CONFLICT DO NOTHING` so either deterministic uniqueness claim is
arbitrated before the authoritative tenant/idempotency lookup and fingerprint
check. Its pipeline `19045` passed 24/24. Final main pipeline `19052` passed
26/26 with durable-submission job `183938` and lifecycle job `183940` green.
The remaining disconfirming boundary is intentional: startup-only
`internal/integration/registry.StaticRegistry` has no durable lifecycle and will
not be replaced until Slice 2.2 consumes the catalog's runnable binding.

## Review

- Roadmap milestone: Engine Beta / Phase 2 production channel runtime.
- Spec sections: product-spec secure production data plane and truthful
  reliability; implementation-plan Slice 2.1.
- Prior decisions to preserve:
  - one logical tenant/security domain per 1.0 deployment;
  - exact, content-addressed definition/profile/workflow/destination revisions;
  - secret references only and raw-PHI-free lifecycle storage;
  - authenticated actor, reason, tenant, and time on human lifecycle actions;
  - adapters must resolve server-owned identity and fail closed.

## Align

- Slice name: versioned integration deployment lifecycle.
- Scope in:
  - backward-compatible deployment policy on immutable integration revisions:
    connection-validation freshness, continuous/cron schedule, health thresholds,
    and bounded capacity;
  - PostgreSQL schema/migration for immutable revision and release records,
    append-only validation/transition history, and an optimistic snapshot;
  - strict state machine: draft, validated, approved, published, deployed,
    paused/resumed, retired;
  - deployment health updates while running;
  - exact runnable-revision resolution only for a deployed release;
  - required PostgreSQL 16 race/restart/immutability proof and synchronized docs.
- Scope out:
  - MLLP, S3/SFTP, Kafka, outbox workers, retry/DLQ/replay/resubmit;
  - GraphQL/REST/UI lifecycle mutations and durable Integration Sessions;
  - OIDC/fine-grained RBAC, staged/canary rollout, multi-tenant hosting;
  - production GitOps activation or live connection-provider implementations.
- Acceptance criteria:
  - legacy Slice 1 revisions retain their existing digest and decode unchanged;
  - deployable revisions reject invalid schedules, health, capacity, and
    validation-freshness policy;
  - illegal/stale transitions fail without partial rows;
  - lifecycle history, failed validations, and releases contain no raw bytes or
    secret values and cannot be mutated;
  - the PostgreSQL kill-test is present, executed, and blocking in CI;
  - roadmap, decision log, status, and operator/developer docs match the boundary.
- Dependencies/blockers:
  - Phase 1 immutable revision and PostgreSQL durability contracts are merged;
  - PostgreSQL 16 is required for the blocking proof;
  - GitLab connectivity is required only for push/MR/terminal CI evidence.
- Rollback:
  - the lifecycle catalog is additive and not yet runtime-wired; reverting the
    slice leaves the existing static registry and authenticated HTTP path intact.

## Land

- Planned file areas:
  - `pkg/integration/`
  - `internal/integration/lifecycle/`
  - `.gitlab-ci.yml`
  - `.loom/`, `docs/`, and `CHANGELOG.md`.
- Implementation steps:
  1. Add deployment-policy/state contracts without changing legacy revision
     digests or audit/identity semantics.
  2. Add the PostgreSQL lifecycle service, fixed migration, strict transitions,
     immutable records, health reporting, and runnable resolver.
  3. Add unit/contract tests and the PostgreSQL race/restart/mutation kill-test.
  4. Add the blocking CI job and synchronize plan/status/decision documentation.

## Prove

- Tests to run:
  - `go test -race -count=1 ./pkg/integration ./internal/integration/lifecycle`
  - `go test -count=1 ./...`
  - PostgreSQL 16 lifecycle test with `-tags=integration`.
- Lint/static checks:
  - `go vet ./pkg/integration ./internal/integration/lifecycle`
  - focused golangci-lint, `git diff --check`, docs validation.
- CI checks:
  - required `test:deployment-lifecycle` PostgreSQL 16 job;
  - existing artifact, durable submission, Golden Path, race, security, build,
    image, and deployment gates.

## Handoff/Harvest

- Docs to update: this plan, `.loom/30-implementation-plan-*`,
  `.loom/40-decisions.md`, `ROADMAP.md`, `docs/STATUS.md`, operations/developer
  documentation, and changelog.
- Agent-context entries: lifecycle/state decision, kill-test outcome, exact
  pipeline/job evidence, and any disconfirming finding.
- Next-slice candidate: Phase 2 Slice 2.2 MLLP adapter, consuming only the
  lifecycle catalog's runnable exact revision.
