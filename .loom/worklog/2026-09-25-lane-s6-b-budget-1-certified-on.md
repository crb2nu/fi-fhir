### 2026-09-25 - Lane S6-B budget 1 certified on the pinned runner

- What changed:
  - Runner 8 (`fi-fhir-perf`) ran its first fi-fhir jobs. Negative control job
    308698 (`FI_FHIR_PERF_BUILD_TAGS=perfregress`, main pipeline 28948, sha
    `7ae14604c`): `certified: false`, `negative_control.failed_budget: 1`,
    p95 ≈ 311 ms, runner id 8. Three clean plays on identical code (main
    pipeline 28955, sha `4d8f31a8c`): jobs 308841, 309005, 309029, all
    `certified: true`, runner id 8, `Intel(R) Xeon(R) CPU E5-2680 v4 @ 2.40GHz`.
  - `docs/operations/SUPPORTED-1.0.md` row 1 → Certified on the serial accept
    paths (scope stated; HTTP-parallel spread 1.24× reported, not certified);
    row 2 unchanged in status with the gap now quoted from the report; row 3 →
    Not measured — sampler only. The "what unblocks certification" paragraph
    now records how the window was opened and what remains.
  - `ROADMAP.md`: S6-B moves to Delivered with the job ids; budgets 2 and 3
    stay in Now.
- Why:
  - The 2026-09-25 morning attempt found runner 8's 11 GiB pod unschedulable
    beside the resident 27B model (2.9 GiB headroom). Rather than scale the
    model — 138 proxy requests in the preceding 24 h — its pod's memory
    request was lowered in place from 16Gi to 6Gi (peak working set 4.3 GiB;
    Kubernetes 1.33 resize subresource), which freed 10 GiB of schedulable
    memory with no restart, and was restored to 16Gi after the fourth job.
- Evidence:
  - Node `cblevins-5930k` requests: 24,790 Mi → 14,550 Mi during the window →
    24,790 Mi after; model pod restarts 0 throughout; ready throughout.
  - Per-run p95 (ms): IngressSubmit 9.93 / 9.38 / 9.56; IngressSubmitParallel
    44.09 / 38.85 / 48.12; MLLPSubmit 9.93 / 9.97 / 9.44; MLLPSubmitParallel
    50.56 / 45.40 / 46.10. Spread (max/min): 1.059, 1.239, 1.057, 1.114.
    `allocs/op` serial: 4644 / 4644 / 4644 and 4694 / 4694 / 4692.
  - Job durations 399 s (control), 162 s, 145 s, 154 s.
- What's next:
  - Budget 2: a one-hour, two-replica run at 250 msg/s — a new harness shape.
  - Budget 3: a 1-GiB batch-import workload reading cgroup RSS on runner 8.
  - Delete the retained `docs/perf-budgets-certified` remote branch once this
    entry's MR merges; the harness fix it carried is on `main` (MR !215).
- Sources:
  - [S1] `.loom/decisions/2026-09-25-budgets-1-and-3-on-runner-8.md`
  - [S2] GitLab jobs 308698, 308841, 309005, 309029 and their
    `performance-report.json` artifacts
  - [S3] `.loom/34-sprint6-execution-specs.md`, Lane S6-B
