### 2026-08-09 - Sprint 5 Lane S5-A file ownership and day-1 perf gate

- What changed:
  - Opened Lane S5-A (slice 4.4b, performance budget harness) on
    `feat/phase4-slice-4-4b-performance-harness`, branched from `origin/main`
    @ `2f8b3f609` (the spec cites `852d7f3ee`; `main` moved before launch).
  - Landed the lane's day-1 gate, `TestPerformanceHarness_NothingMeasuresAnyProductBudgetToday`
    (`cmd/bench-check/inventory_test.go`), as a standalone test-only change.
- Why:
  - `.loom/33-sprint5-execution-specs.md` requires each lane to record the files
    it owns before its first commit, and requires the day-1 gate to produce its
    stated result for its stated reason before the lane writes production code.
- Files this lane owns (from `.loom/33` File-Ownership Map):
  - `internal/workflow/benchmark_util.go` — threshold and CPU-profile tables.
  - `cmd/bench-check/**` — the gate program, its suggester, and its tests.
  - `internal/workflow/benchmark_test.go` — the legacy benchmark set.
  - `internal/workflow/loadtest.go` and the `workflow loadtest` region of
    `cmd/fi-fhir/main.go` — delete-or-relabel, never reuse (correction 8).
  - `internal/integration/perf/**` — new; the first benchmarks under
    `internal/integration` in the repo's history (correction 3).
  - `ci/test-performance-profile.yml` — new; **not written until S5-0 MR 0a
    merges**, and then exactly one `include:` line in `.gitlab-ci.yml`.
  - `docs/operations/SUPPORTED-1.0.md` profile and budget rows.
  - `deploy/helm/fi-fhir/values-reference-profile.yaml` — S5-A decides the
    numbers, S5-B enforces that it renders.
  - `.loom/40-decisions.md` — appended dated entries only.
- Shared surfaces this lane must coordinate rather than own:
  - `test:benchmark`'s package list (`.gitlab-ci.yml:1250`, `Makefile:253,260`)
    contains `./pkg/validate/...`, which is S5-E's file territory. Narrowing it
    is a one-line coordination with S5-E, not an edit to S5-E's files.
  - No migration ledger. This lane authors no SQL, so no ledger number is
    claimed and nothing here unfreezes.
- Evidence:
  - `go test ./cmd/bench-check/ -run TestPerformanceHarness -v` passes on an
    unmodified tree, for the stated reason: zero `func Benchmark` declarations
    exist anywhere under `internal/integration/`.
- What's next:
  - The pinned-runner decision entry in `.loom/40-decisions.md` (coordinator
    ruling 2 of `.loom/33` is the input; the lane records it, it does not make
    it), then tasks 2-8.
  - Budget-2 measurement is deliberately sequenced after S5-D merges; a
    250 msg/s run on two replicas against a revision declaring 250 admits up to
    500 and certifies nothing (correction 11).
- Sources:
  - [S1] `.loom/33-sprint5-execution-specs.md` — Lane S5-A, File-Ownership Map,
    Coordination Rules, Merge Order, day-1 gate table, coordinator ruling 2
  - [S2] `internal/workflow/benchmark_util.go:321-335` — the alloc signal's
    measured CPU-independence
  - [S3] `.gitlab-ci.yml:1238-1293` — `test:benchmark`, its package list, and
    its `allow_failure: false`
