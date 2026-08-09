### 2026-08-08 - CI benchmark gate: per-CPU thresholds

- What changed:
  - Reworked the blocking `test:benchmark` gate around the CPU model the
    benchmarks actually ran on, instead of one flat set of ceilings.
  - `ParseBenchmarkOutput` now retains the `cpu:` and `pkg:` headers it was
    discarding; `BenchmarkSuite.CPU` and `BenchmarkResult.Package` carry them.
  - Added `CPUProfile` plus `ResolveWorkflowThresholds`, calibrated for the
    three hardware classes in the k3s pool at `LatencyMarginFactor` (1.6x) of
    each class median. Unrecognized hardware falls back to the slowest profile
    and warns rather than failing closed.
  - Tightened `allocs/op` ceilings from ~2x slack to just above the observed
    counts, and extended them to the two throughput benchmarks.
  - `bench-check -confirm=N` re-runs only a violating benchmark before failing,
    and refuses to clear a violation if the re-run lands on different hardware.
  - `bench-check -suggest <artifacts>` emits calibrated profile blocks so
    adding a node type is mechanical. Wired into `make bench-calibrate`.
  - Added `PerformanceThresholds.Check` (violations with benchmark + metric)
    and `BenchmarkSuite.ReduceToBest` (best-of-N collapse).
- Why:
  - Pipeline 22521 / job 220702 failed `BenchmarkEngineProcess` at 12194 ns/op
    against a 12000 ns/op ceiling and passed on retry with identical code. The
    reported cause, 1.6% overage under contention, was not the cause: the job
    landed on an emulated QEMU Broadwell node and the retry landed on a Xeon
    roughly 2.5x faster. A single absolute ceiling cannot be tight on the fast
    nodes and non-flaky on the slow ones.
- Evidence:
  - Analyzed 87 `benchmark.txt` artifacts from GitLab project 19
    (2026-05-22..2026-08-08); 78 parseable, 9 predating a fix that silenced
    engine logging during benchmarks and therefore unparseable under both the
    old and new gate.
  - The pool spans three CPU classes with a 5.3x spread on
    `BenchmarkEngineProcess` (Ryzen 7900X3D p50 1869 ns/op, Xeon E5-2680 v4
    6231, QEMU Broadwell 7842). Within a class the spread is 15-19% CV.
  - Decomposed the noise: whole-run contention p99 is 1.37x of the run's own
    class median, single-benchmark bursts reach 1.55x. Backtesting margins from
    1.4x to 2.5x, failures fall to one run at 1.6x and stay flat at one run
    through 2.5x, so anything above 1.6x costs sensitivity and buys nothing.
  - `allocs/op` was bit-identical in every artifact that measured it (78 to 87
    runs per benchmark), spanning all three CPU classes, which is why it can
    carry a tight ceiling with no flake risk.
  - Replay over the 78 parseable artifacts, comparing a binary built from
    `HEAD` against this branch: old gate fails 2 runs (178247, 220702), new
    gate fails 1 (178247), zero new failures. Regression sensitivity goes from
    1.53x-10.01x of median depending on the node to a uniform 1.6x.
  - The remaining failure, job 178247, is a contention burst rather than a slow
    run: its median across 22 benchmarks is 1.09x of class median while
    individual benchmarks spike to 4.96x, including `BenchmarkNPIValidation`
    (pure integer math, zero allocations). No ceiling absorbs that, which is
    what `-confirm` is for.
  - `bench-check -suggest` over the same 78 artifacts independently
    reproduces the committed table, and caught a hand-transcription error in
    the Ryzen CEL ceiling.
  - `gofmt` clean, `go vet` clean, `golangci-lint run` 0 issues on both changed
    packages, `go test ./...` green.
- What's next:
  - Recalibrate when the runner pool changes; `make bench-calibrate
    ARTIFACTS="..."` prints the block to paste.
  - Optional follow-up: `B/op` is bit-stable for the gated benchmarks too and
    could carry a ceiling the same way `allocs/op` now does.
- Sources:
  - [S1] Job 220702 trace: `BenchmarkEngineProcess: 12194 ns/op exceeds
    threshold of 12000 ns/op`; job 220916 same commit, 4818 ns/op on a Xeon.
  - [S2] `internal/workflow/benchmark_util.go` (profiles and margin rationale)
  - [S3] `internal/workflow/benchmark_util_test.go`
    `TestGate_RegressionForPipeline22521`
  - [S4] Command: `go run ./cmd/bench-check -suggest <78 artifacts>`
