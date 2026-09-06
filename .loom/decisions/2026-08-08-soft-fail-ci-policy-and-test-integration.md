### 2026-08-08: Soft-Fail CI Policy and `test:integration` Promotion (Lane E)

- Decision:
  - Adopt an explicit soft-fail policy: every `allow_failure: true` job in
    `.gitlab-ci.yml` must carry an inline comment stating *why* it is advisory
    and what evidence would promote it. "Soft-fail during initial rollout" is no
    longer an acceptable standing reason.
  - Classify the three remaining advisory jobs and act on each:

    | Job | Green streak on main (18521..22333, 2026-07-13..2026-08-07) | Classification | Action in this MR |
    |---|---|---|---|
    | `test:integration` | 24/24 success — but see caveat below | Ready to promote | **Promoted to `allow_failure: false`** |
    | `lint:docs` | 33/33 success | Ready to promote | **Promoted to `allow_failure: false`** |
    | `test:docs-status` | 29/29 success | Intentionally advisory (for now) | Left advisory, promotion criteria documented inline |

  - **Caveat on the `test:integration` streak — it was never a full-execution
    proof.** All 24 of those runs executed with the `minio` service container
    dead, so all 30 MinIO-backed tests in `./cmd/fi-fhir/...` skipped. The
    streak validated the Postgres-only path and nothing more. This MR's own
    pipelines are the first true full-execution evidence, and they immediately
    found two defects the streak could not have caught (the truncated-column
    assertion, and the shared-database contamination below). Read the 24/24 as
    "the Postgres path is stable", not "the job was meaningful".

  - Repair the `minio` service container in `test:integration` before promoting
    it, because the job's green history was partly an artifact of tests that
    never ran.
  - Fix the one real test defect that the dead MinIO service had been masking.
  - Do **not** add a `/ready` smoke assertion; `/ready` is not served by
    `fi-fhir serve`. File a cleanup issue instead of asserting a 404.
- Rationale:
  - `minio/minio:latest` ships `CMD ["minio"]`, which prints usage and exits.
    The service container never listened on `minio:9000`, so
    `setupTestInfra()` failed its readiness probe and every dependent test hit
    `t.Skipf("setupTestInfra: minio not ready")` — a **skip**, not a failure.
    **30 integration tests silently skipped in CI**, including the PostgreSQL
    event-store lifecycle, projections, terminology init/status, storage, and
    mapping-decision CLI suites.
  - Negative-control kill-test: with MinIO unreachable, `./cmd/fi-fhir/...`
    reports `coverage: 73.2% of statements` (1380 pass / 30 skip) — the exact
    figure logged by CI job 218601. With MinIO live it reports `75.9%`
    (1410 pass / 0 skip). The coverage figures match to the decimal, so CI has
    been running the degraded path.
  - Promoting `test:integration` while it silently skipped a third of its
    infrastructure-backed surface would have written the false-coverage problem
    into the merge gate, which is the exact failure mode Gate 0B named for
    security jobs (`.loom/30-implementation-plan-integration-engine-ide-completion.md:58`).
  - With MinIO live, `TestIntegration_TerminologyMappingDecisionCLI` fails
    deterministically: it asserts a 23-character `GLU-<UnixNano>` source code
    appears in the decisions table, but that column is rendered through
    `truncate(decision.SourceCode, 12)`. The fixture, not the CLI, is wrong.
  - `lint:docs` is the safest promotion available: `scripts/validate-docs.sh` is
    deterministic, needs no services, runs in ~90s, and has never failed on main.
  - `test:docs-status` is held back deliberately. Its check is a *numeric*
    function-average coverage comparison recomputed from `test:unit`'s
    `coverage.out`, so promotion makes `make docs-status` mandatory for every Go
    MR author and a red run is not always attributable to the MR under test.
    Promoting it in its own MR keeps the blast radius attributable. This also
    honors the Lane E non-goal "do not promote every `allow_failure` job in one MR".
  - `security:gosec` / `security:govulncheck` / `security:trivy-image` are
    already blocking (`allow_failure` absent) and are **not** touched: gosec and
    govulncheck OOM intermittently on merge-ref pipelines, and trivy-image scans
    against a daily-moving vulnerability database.
  - `test:benchmark` keeps its blocking-manual design (`when: manual` +
    `allow_failure: false`); other tooling depends on that behavior.
- Alternatives considered:
  - Promote `test:integration` without repairing the MinIO service (rejected:
    it would make a hollow gate mandatory and hide 30 skipped tests behind a
    green required check).
  - Repair MinIO but keep `test:integration` advisory (rejected: the green proof
    requirement is satisfied and the C1 autoroute sweep kill-test plus the
    terminology DB store would remain unprotected).
  - Assert `GET /ready` returns 404 in `scripts/smoke-test.sh` (rejected: it
    would freeze the absence of a readiness endpoint into a passing assertion).
  - Promote all three advisory jobs at once (rejected by the Lane E non-goal and
    because a single red pipeline would then be ambiguous).
  - Change `truncate()` so the decisions table stops truncating (rejected:
    column truncation is intended table formatting; the test fixture was written
    without accounting for it).
- Consequences:
  - `test:integration` and `lint:docs` now block merges. Both sibling MRs
    shipping this sprint (`feat/phase4-mllp-cert-identity`,
    `feat/autoroute-notifications`) are gated on them.
  - `test:integration` gets slower and more expensive: the 30 previously-skipped
    tests now execute, and MinIO becomes a real dependency. Job duration on main
    was 350-565s while degraded; expect an increase.
  - If the MinIO service ever breaks again, the failure mode reverts to *skips*,
    not failures — the job would go green while covering less. This is the
    residual risk; see the cleanup issue on `setupTestInfra` skip semantics.
  - `test:docs-status` remains advisory, so STATUS.md coverage drift can still
    reach main. That is an accepted, dated, and now-documented gap.
- Follow-up within the same MR: shared-database contamination (found by this gate).
  - Once the MinIO fix let the 30 skipped tests actually run in CI,
    `test:integration` failed deterministically (jobs 220075 and 220246, retried
    on a quiet runner). `./cmd/fi-fhir/...` passed at the full 75.9%, then
    `./pkg/terminology/db/` panicked with `test timed out after 5m0s` at ~47.5%
    coverage, blocked in `Migrator.Initialize`
    (`pkg/terminology/db/migrations.go:79`) inside a lib/pq `simpleExec` network
    read.
  - Root cause is budget exhaustion from shared state, not a deadlock. The two
    steps are separate `go test` invocations, so no connection or lock can
    survive between them, and the panic's `running tests:` line shows the
    blamed test had run only 3s — the 300s was consumed by everything before it.
    Both suites reset the same `terminology` schema in the same database. While
    the cmd suite was skipping, it left that database empty; now it populates it,
    so every subsequent schema teardown/rebuild in `pkg/terminology/db` does real
    work. That package already needed 204.7s of a 300s budget on main (job
    218601) — only 32% headroom — so the added cost pushed it over.
  - Fix: give `pkg/terminology/db` its own database (`fi_fhir_terms_test`,
    created in the job script), which is exactly the isolation Lane D
    recommended, and raise that step's `-timeout` to 900s to remove the
    shared-runner cliff. The pre-existing `-p 1` was never sufficient: it limits
    parallel *packages* within one `go test` invocation and does nothing across
    two separate commands, and it never addressed data contamination at all.
  - Verified locally against PostgreSQL 16 and a live MinIO, running the CI
    steps in CI order: shared database reproduced no failure on fast hardware
    (33.5s vs a 33.4s clean baseline — the contamination cost is only visible on
    the constrained CI Postgres), and with separate databases both steps pass
    (cmd 23.2s / 75.6%, terminology 36.4s / 69.7%).
  - The timeout increase is a slow-suite allowance, not a weakened gate: a
    genuine hang still fails the job, just later.

- Cleanup issues to file:
  1. **`setupTestInfra` skips instead of failing when CI infra is down.** In CI,
     unavailable Postgres/MinIO should be a hard failure, not `t.Skipf` — the
     skip path is what allowed 30 tests to disappear from a "green" required job.
     Gate on `CI=true` (or an explicit `FI_FHIR_REQUIRE_TEST_INFRA=1`) so local
     runs keep the developer-friendly skip and CI cannot silently degrade.
  2. **`/ready` is dead code in `serve`.** `internal/workflow/health.go:68`
     defines a `/ready` readiness path but `NewHealthService` has zero non-test
     callers, so `fi-fhir serve` never mounts it. Helm's readiness probe uses
     `/health` (`deploy/helm/fi-fhir/templates/deployment.yaml:173`) and the
     kustomize base uses an exec probe, so nothing is broken in production —
     but either wire `/ready` into serve and add the smoke assertion, or delete
     the unused readiness path.
  3. **Promote `test:docs-status` to blocking** in a dedicated MR once the team
     accepts `make docs-status` as a mandatory pre-commit step for Go MRs.
- Sources:
  - [S1] `.gitlab-ci.yml` — `test:integration`, `lint:docs`, `test:docs-status`.
  - [S2] `cmd/fi-fhir/integration_helpers_test.go:56-118` — `setupTestInfra`
    skip-on-unavailable path and `ensureMinioBucket` readiness probe.
  - [S3] CI job 218601 (pipeline 22333) trace:
    `ok gitlab.flexinfer.ai/libs/fi-fhir/cmd/fi-fhir 60.783s coverage: 73.2% of statements`.
  - [S4] Command: `docker --context 7900xtx run --rm minio/minio:latest` →
    prints usage and exits; never starts a server.
  - [S5] Local negative control, MinIO unreachable:
    `go test -tags=integration ./cmd/fi-fhir/...` → `73.2%`, 1380 pass / 30 skip.
  - [S6] Local positive control, MinIO live on `cblevins-7900xtx:15504`:
    `go test -tags=integration ./cmd/fi-fhir/...` → `75.9%`, 1410 pass / 0 skip.
  - [S7] `cmd/fi-fhir/terminology.go:1866` — `truncate(decision.SourceCode, 12)`.
  - [S8] Local kill-test of the second script step:
    `POSTGRES_TEST_URL=... go test -tags=integration -p 1 ./pkg/terminology/db/`
    → `ok ... 34.519s coverage: 69.7%`, including
    `--- PASS: TestAutorouteExpirySweep_FlipsStoredStatus`.
  - [S9] `scripts/smoke-test.sh:98-104` — `/health`, `/graphql`, `/graphql/ws`
    assertions already present (Gate 0B); `/ready` absent by design.
  - [S10] `.loom/24-parallel-execution-specs.md` — Lane E scope and kill-test;
    line 306 records Lane D's "distinct databases/schemas" recommendation.
  - [S11] CI jobs 220075 and 220246 — `panic: test timed out after 5m0s`,
    `FAIL pkg/terminology/db 300.05s`, after `ok cmd/fi-fhir 50.436s coverage: 75.9%`.
  - [S12] CI job 218601 — `ok pkg/terminology/db 204.737s`, the pre-existing
    68%-of-budget baseline.
