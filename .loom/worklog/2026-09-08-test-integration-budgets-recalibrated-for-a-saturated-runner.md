### 2026-09-08 - test:integration budgets recalibrated for a saturated runner

- What changed:
  - `.gitlab-ci.yml` `test:integration`: the `pkg/terminology/db` step's
    `-timeout` goes 900s → 1800s and the `cmd/fi-fhir` step's 300s → 600s,
    with the measurements in the job comments. No test, no code, no other job.
- Why:
  - Sprint 6 Lane S6-A's day-1 gate MR (!203, six unit-test files, no database
    access) failed `test:integration` four times in one afternoon: three
    `panic: test timed out after 15m0s` in `pkg/terminology/db` (jobs 263076,
    263271, 263272) and one `after 5m0s` in `cmd/fi-fhir` (263220). Lane S6-0's
    !202 (CI and docs only) failed the same job once (262964) before passing on
    retry (263124).
  - The passing run measures the two packages at 383s and 46.6s. The 900s
    budget was set when the terminology package needed 204.7s, a 4.4x margin
    that had shrunk to 2.3x. At every timeout the test then running was
    seconds old and the goroutine dump was idle `database/sql` connection
    openers — the package was slow, not stuck.
  - The slowness is the pool, not the code: runner 1 was executing twelve jobs
    for five pipelines (three `services/loom-core` Mills pushes) with
    `k3s-w-12` at 99% CPU. Mills runs continuously, so that is the normal
    state now and every Sprint 6 MR would pay the same four-retry tax.
- Evidence:
  - `GET /runners/1/jobs?status=running` → 12; `kubectl top nodes` → k3s-w-12
    7975m/99%; `GET /projects/19` → `build_timeout: 3600`.
  - Trace 263272: `running tests: TestUMLSLoader_Integration_LoadMETA_SkipRelationsAndSemanticTypes (10s)`
    at the 900s mark; 78 goroutines in `select` under
    `database/sql.(*DB).connectionOpener`.
  - Worst case inside the ceiling: apt (~3 min) + 600s + 1800s + covdata < 60 min.
- What's next:
  - !203 was closed as superseded by !206, which carries its gate commit; one
    pipeline per lane while the pool is saturated.
  - A real fix is shorter terminology fixtures or a split job, not a bigger
    budget; filed as a Wave 3 note in `.loom/34`'s successor when Sprint 6
    closes. Memory: `ci-gate-patterns` "Runner Saturation Looks Like Four
    Different Flakes".
- Sources:
  - [S1] `.gitlab-ci.yml` `test:integration` (script steps and comments)
  - [S2] GitLab jobs 262964, 263076, 263124, 263220, 263271, 263272
  - [S3] `.loom/34-sprint6-execution-specs.md` correction 19 and Lane S6-0
