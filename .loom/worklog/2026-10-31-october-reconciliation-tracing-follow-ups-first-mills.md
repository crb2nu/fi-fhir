### 2026-10-31 - October reconciliation: tracing follow-ups, first Mills train rows, edilint v0.5.0 readiness

- What changed: `ROADMAP.md` records the 2026-10-27 → 10-30 merges as
  delivered (MR !280's proof-profile arm and pins, !246 and !253's e2e waits,
  !279's opt-in serve tracing, the go-jose hotfix !284, the three
  `mills-eligible` follow-ups !283, !285 and !282, the QuickBenchmark repair
  !286), rewrites Current Status through `f9144fef7` (production
  `v0.1.36008+f9144fef` read from the LAN), re-triages Now against the tree
  (two items closed, three re-worded, nine added, every other item
  re-verified open by grep), corrects the stale NetworkPolicy claim (gitops
  !842 merged 2026-09-28), adds issues #22–#24 to the Backlog table,
  replaces the Mills posture with the evidence from the lane audit, and
  retires the parked program plan from the header. A decision entry of the
  same date records the train-proof finding and the deferred lane decisions.
  edilint's ROADMAP got the same treatment in its own merge request (v0.5.0
  blockers, the `Fix` BOM idempotence defect, the field-guide samples, its
  Mills posture).
- Why: the 2026-10-27 reconciliation predates everything the Mills lane did;
  its Now list still named three merged MRs as open, and its Mills posture
  described a train that had not yet carried a row. The first five rows
  showed the train's proof pipelines were a single Helm lint, which no
  roadmap or dashboard said.
- Evidence: `git log a9be5577a..f9144fef7` (10 merges, 22 commits, 70
  files); project 19 pipelines 35918/35921/35971/36007 (push, 1 job) and
  36008 (main, 60 jobs green, 74 min); `GET /health` on 2026-10-31
  17:09 UTC; production log line "FI_FHIR_TELEMETRY_CORRELATION_KEY is
  unset" at 2026-10-30T17:36Z; greps for `triggerWorkflow`
  (`WorkflowList.svelte:473`), `observedReplicas` (UI selects none),
  `check-runtime-config.sh` (no CI caller), `serve --help` (no tracing
  keys); Mills run ledger (8 runs, 12.77 USD) and events API
  (`mergequeue.train_built` ×4, `train_tree_mismatch` 0); loom-core
  `processor_train.go`, `gitlab_importer.go`, `carder.go`,
  `council/roadmap.go`; edilint local gate green under Go 1.26.10 and the
  doubled-BOM reproduction.
- What's next: land the `workflow:` never-rule for `mills-mq/` pushes (human
  MR, protected path) and ask loom-core for queue-owned proofs; wire the
  correlation key into production (gitops SOPS, Cody); close #22–#24; run
  `check-runtime-config.sh` in CI; fix the inverted negative-control target;
  edilint: BOM fix, then `chore(release): prepare v0.5.0`, then the tag, then
  the fi-fhir pin bump.
- Sources:
  - [S1] GitLab projects 19 and 74 (MRs, pipelines, issues), read 2026-10-31.
  - [S2] Mills operator read API (`/api/mills/pipeline/runs`, `/events`,
    `/policy`) and `mills_*` MCP tools, read 2026-10-31.
  - [S3] loom-core `origin/main` 786ddf845 and platform/gitops `origin/main`
    21256d33f, read with `git show`.
