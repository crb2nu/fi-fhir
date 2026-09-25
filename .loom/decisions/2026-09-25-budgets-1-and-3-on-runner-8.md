### 2026-09-25: Budgets 1 and 3 on runner 8: certified and not measured

- Decision:
  - **Budget 1 is certified on the serial durable-accept paths** (ingress and
    MLLP `Submit` against PostgreSQL 16, one replica, in-process — excluding
    the HTTP handler, bearer/HMAC validation, TLS, and MLLP framing) on runner
    8 (`fi-fhir-perf`, `Intel(R) Xeon(R) CPU E5-2680 v4 @ 2.40GHz`, 4 CPU /
    8 GiB). Evidence: negative control job 308698 (`-tags perfregress`,
    `certified: false`, `failed_budget: 1`, p95 ≈ 311 ms) and three plays on
    identical `main` code `4d8f31a8c` (jobs 308841, 309005, 309029,
    `certified: true`, `runner_id: 8`). Serial p95 9.4–10.0 ms, p99
    10.3–13.3 ms; parallel p95 38.9–50.6 ms, p99 53.5–65.2 ms; targets 250 /
    500 ms.
  - **The parallel figures are reported, not certified.** The Sprint 6 spec's
    stability rule (p95 spread ≤ 1.15× across three identical runs) holds on
    `IngressSubmit` (1.06×), `MLLPSubmit` (1.06×) and `MLLPSubmitParallel`
    (1.11×) and fails on `IngressSubmitParallel` (1.24×, 38.9–48.1 ms).
  - **Budget 2 stays "Harnessed, uncertified".** A one-hour variant of the
    single-process harness is rejected: the budget is 250 two-KiB msg/s for
    one hour against two replicas of the reference profile, and a one-process
    soak would certify a different claim.
  - **Budget 3 becomes "Not measured — sampler only".** "Harnessed" overstated
    it: no workload imports a batch, and `perf.HeapSampler` reads `HeapAlloc`
    in a 32 MiB unit test, not the cgroup RSS the budget is written in.
  - **The run window was opened by an in-place pod resize, not by scaling the
    model.** Runner 8's job pod is fixed at 11 GiB of requests (build 8 GiB,
    helper 1 GiB, PostgreSQL 2 GiB). `cblevins-5930k` exposes 27.7 GiB
    allocatable of 62.7 GiB (systemReserved 24 GiB, kubeReserved 2 GiB) and
    had 24.8 GiB requested, 16 GiB of it the `qwen38-27b-autoround-workhorse`
    pod whose 7-day peak working set is 4.3 GiB (weights live in VRAM). With
    Kubernetes 1.33 in-place resize the pod's memory request was lowered
    16Gi → 6Gi (limit 40Gi untouched, Burstable stays Burstable, no restart,
    138 proxy requests/24 h kept flowing), the four jobs ran, and the request
    was restored to 16Gi. Recipe: `kubectl patch pod -n flexinfer-system
    <pod> --subresource resize --type json -p '[{"op":"replace","path":
    "/spec/containers/0/resources/requests/memory","value":"6Gi"}]'`; a
    `--type merge` patch on that subresource replaces the whole resources
    block and is refused.
- Rationale:
  - The serial benchmarks isolate the host: same code, same node, three runs,
    ≤ 1.06× apart. `RunParallel` on four CPUs beside the database in the same
    pod is contention the scheduler decides, which Slice 4.4b already recorded
    as report-only for `allocs/op`; the same holds for its wall-clock. Reading
    the 1.15× rule as a host-stability test, the host passes; reading it as a
    per-benchmark certification bar, the HTTP-parallel path does not — so that
    path is reported with its spread rather than certified.
  - Widening the rule to make the parallel path green would be the ceiling
    move `ci/test-performance-profile.yml` forbids in its own comments.
- Alternatives considered:
  - Scaling the 27B model to zero for the window: rejected — it serves 0–22
    requests per hour continuously and the resize made it unnecessary.
  - Shrinking the runner's pod to fit 2.9 GiB of headroom: rejected — a
    `platform/gitops` change that changes the reference profile the rows
    describe, with OOM risk below 4 GiB.
  - Moving runner 8 to another node: rejected for this window — it changes
    the reference host and answers a question nobody has asked yet.
  - A fourth or fifth run to shrink the HTTP-parallel spread: rejected — the
    max/min ratio can only grow with more samples; the spec asked for three.
- Consequences:
  - `SUPPORTED-1.0.md` rows 1–3 and the paragraph after the table now say
    exactly this; `ROADMAP.md` carries budgets 2 and 3 as open.
  - Any future certification window on this node needs the same 11 GiB of
    schedulable headroom; the resize recipe above opens it without touching
    the model, and reverts on its own when the model pod is next replaced.
  - Budget 3 is its own slice (a 1-GiB batch-import workload reading cgroup
    RSS, with restart-from-checkpoint). Budget 2 needs a two-replica soak the
    harness does not have.
- Sources:
  - [S1] `.loom/34-sprint6-execution-specs.md`, Lane S6-B
  - [S2] performance-report.json artifacts of jobs 308698, 308841, 309005, 309029
  - [S3] `.loom/worklog/2026-09-25-lane-s6-b-budget-1-certified-on.md`
  - [S4] MR !215 (the harness fix that made `certified` mean something)
  - [S5] `platform/gitops` `k3s/ci/gitlab/helmrelease-runner-perf.yaml` and
    `k3s/nodes/node-cblevins-5930k.yaml`
