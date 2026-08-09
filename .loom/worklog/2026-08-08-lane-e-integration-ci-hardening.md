### 2026-08-08 - Lane E integration CI hardening

- What changed:
  - **Lane E (integration/contract CI hardening) shipped.** Branch
    `ci/integration-ci-hardening`.
  - Inventoried all three `allow_failure: true` jobs in `.gitlab-ci.yml` and
    classified them against 40 pipelines of main history
    (18521..22333, 2026-07-13..2026-08-07):
    `test:integration` 24/24 green → promoted; `lint:docs` 33/33 green →
    promoted; `test:docs-status` 29/29 green → deliberately held advisory with
    inline promotion criteria.
  - **Repaired the `minio` service container before promoting.**
    `minio/minio:latest` ships `CMD ["minio"]`, which prints usage and exits, so
    the service never listened on `minio:9000`. `setupTestInfra()` responded with
    `t.Skipf`, not a failure, so **30 integration tests silently skipped in CI** —
    event store, projections, terminology init/status, storage, mapping-decision
    CLI. Added `command: ["server", "/data", "--console-address", ":9001"]`.
  - Fixed the one real defect the dead service had been masking:
    `TestIntegration_TerminologyMappingDecisionCLI` asserted a 23-character
    `GLU-<UnixNano>` code appeared in a column rendered through
    `truncate(decision.SourceCode, 12)`. The fixture now fits the column width.
  - Removed the stale `MINIO_DEFAULT_BUCKETS` variable (a bitnami-only
    convention, inert against `minio/minio`; the bucket is created by
    `ensureMinioBucket`).
  - Corrected docs that described CI inaccurately: `AGENTS.md` still claimed
    `pkg/terminology/db` is not exercised by CI (Lane D wired it in),
    `docs/DOCUMENTATION-CONVENTIONS.md` said `lint:docs` is advisory, and
    `docs/developer-guide/testing.md` showed a fabricated `test:integration`
    snippet (postgres:14 / hapiproject / `./test/e2e/...`).
- Why:
  - Lane E's acceptance criteria require a recent green proof for any promoted
    job and that `test:integration` "no longer gives a false sense of coverage".
    The kill-test proved the green history was partly an artifact of skipped
    tests, so promoting without the MinIO repair would have made a hollow gate
    mandatory — the exact failure mode Gate 0B named for security jobs.
  - Negative-control evidence: with MinIO unreachable `./cmd/fi-fhir/...` reports
    `coverage: 73.2%` (1380 pass / 30 skip), matching CI job 218601 to the
    decimal. With MinIO live it reports `75.9%` (1410 pass / 0 skip).
- What's next:
  - File the three cleanup issues recorded in `.loom/40-decisions.md`:
    (1) make `setupTestInfra` fail rather than skip when CI infra is down;
    (2) resolve `/ready` — either mount it in `serve` and assert it in
    `scripts/smoke-test.sh`, or delete the unused readiness path;
    (3) promote `test:docs-status` to blocking in a dedicated MR.
  - Lane C2 (pending-autoroute notifications) remains the last open lane.
- Sources:
  - [S1] `.gitlab-ci.yml` — `test:integration`, `lint:docs`, `test:docs-status`.
  - [S2] `cmd/fi-fhir/integration_helpers_test.go:56-118`.
  - [S3] CI job 218601 trace (pipeline 22333) — `coverage: 73.2% of statements`.
  - [S4] Command: `docker --context 7900xtx run --rm minio/minio:latest`.
  - [S5] `.loom/40-decisions.md` — 2026-08-08 soft-fail policy entry.
