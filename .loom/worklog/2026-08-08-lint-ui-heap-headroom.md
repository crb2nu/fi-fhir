### 2026-08-08

- What changed:
  - Gave `lint:ui` an explicit resource envelope in `.gitlab-ci.yml`:
    `KUBERNETES_MEMORY_REQUEST: 2Gi`, `KUBERNETES_MEMORY_LIMIT: 4Gi`, and
    `NODE_OPTIONS: --max-old-space-size=3072`. It was the only UI job running a
    full Vite production build while still inheriting the global 2Gi/512Mi
    default, and the only pipeline job doing a heavy Node build without an
    override (~25 Go/image jobs already set 4Gi).
- Why:
  - `lint:ui` failed 4 times in 15 successes on 2026-08-08 with
    `FATAL ERROR: Reached heap limit` during `rendering chunks`, across
    unrelated MRs (!139, !142, !143) including one touching zero UI files.
  - The reported cause (a node-saturation flake) does not hold: a cgroup memory
    limit is a hard per-container ceiling, so a busy node cannot change it.
    Node 22 derives its default old-space cap from the cgroup limit at roughly
    half, so the global 2Gi default silently capped the V8 heap at 1048Mi. The
    build peaks at 1040Mi — 99.2% of that ceiling on every single run. The
    intermittency is GC/allocation nondeterminism at the margin, not neighbours.
  - Raising `NODE_OPTIONS` alone was rejected: at a 3072Mi cap inside the
    unchanged 2Gi container, V8 GCs less aggressively and container anonymous
    memory rose from 1530Mi to 1736Mi — 85% of a hard 2048Mi ceiling. That
    trades a diagnosable V8 abort for a silent kernel OOM-kill (exit 137).
    The heap cap and the container limit have to move together.
- Evidence:
  - Reproduced on the exact CI image (`node:22-bookworm`) under CI's resource
    envelope, with per-process V8 sampling and cgroup `memory.stat` anon peaks:
    - 2Gi + default cap: V8 peak 1040Mi / 1048Mi ceiling (99.2%), build 31s.
    - 2Gi + `--max-old-space-size=3072`: container anon peak 1736Mi / 2048Mi (85%).
    - 4Gi + `--max-old-space-size=3072`: V8 peak 1201Mi / 3120Mi (38.5%),
      container anon peak 1717Mi / 4096Mi (42%), build 22s. Shipped as verified.
  - `heap_size_limit` under a 2Gi cgroup measured directly at 1048Mi, confirming
    the cap is derived from the container limit rather than the host.
  - `ci-jobs` quota has headroom for the raised request: `requests.memory`
    22Gi/128Gi, `limits.memory` 103.5Gi/256Gi, pods 21/100.
  - Scope check: `test:ui` runs against the server binary and
    `lint:sdk-typescript` is a small tsc build; neither runs `vite build`, so
    neither shares the defect.
- What's next:
  - Watch `lint:ui` over the next batch of UI-touching MRs; the heap headroom
    should now absorb UI growth well past the current 2660-module graph.
  - If the build ever approaches the 3072Mi cap, revisit rather than raise
    blindly — total footprint is roughly cap + ~520Mi of non-heap anon memory,
    which must stay under the 4Gi container limit.
- Sources:
  - [S1] Job traces: `lint:ui` 221197 (!139), 221359/221410 (!142), 221504 (!143)
  - [S2] `.gitlab-ci.yml` global defaults block (`KUBERNETES_MEMORY_LIMIT: 2Gi`)
  - [S3] Command: `docker --context 7900xtx run --memory=2g node:22-bookworm`
         with `v8.getHeapStatistics()` sampling and `/sys/fs/cgroup/memory.stat`
  - [S4] Command: `kubectl describe resourcequota -n ci-jobs`
