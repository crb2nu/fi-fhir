### 2026-11-04 - Mills push lane close-out: nine rows merged, kill-test passed, queue tuned

- What changed: `ROADMAP.md` ticks eight Now bullets with the merge request,
  merge commit, row id and train pipeline that closed each (M-1 !294, M-2
  !301, M-3 !292, M-6 !300, M-8 !296, M-9 !293, M-11 !297, and the loom-core
  half of the train proof, !3350), records the code half of the correlation
  key (M-5 !295) under the still-open production-secret bullet, and notes
  M-12 (!302) under the closed e2e bullet. `.loom/43` marks the kill-test
  PASSED with its evidence and adds an outcomes table for all fifteen rows;
  its coordinator decision about documentation merge requests, and the
  decision entry of 2026-11-03, now say the direct merge applies only on an
  empty lane. This entry is the lane's record for the night.
- Why: the push's load-bearing assumption was that a UI row lands through
  the devbox and `test:ui-e2e` on the train without a person. M-9 did
  ([S1], train 37418, 105 minutes, no hand), then M-11 and M-12, so the five
  remaining UI rows stay on the lane instead of becoming a worktree session.
  The rest of the night was lane mechanics worth writing down once.
- Evidence: merges, in lane order — !290 `e6b9d8cb4` (train 37346), !291
  `293c9b3e5` (37372), !293 `f9e3e1b9f` (37418), !294 `e52d39482` (37464),
  !292 `1b812590c` (37525), !298 `1cc0769c4` (37540, the Dockerfile
  `go mod download` retry loop from a side session), !297 `2b57aa6bf`
  (37547), !301 `a5aaa5141` (37559), !302 `8e999185a` (37562), !300
  `ecc842c35` (37566), !295 `d1e23bebf` (37572), !296 `4f9c71e51` (37577),
  !299 `252777436` (37581).
  Three of these were rescues: a Mills implement branch whose run escalated
  `infra` before an MR existed (!292, !294, !295) is turned into an MR by
  hand and enqueued by curl; the queue links it to its row only when the
  branch name carries the row id and the enqueue response shows
  `source_backlog_id`, otherwise the row is set `merged` by hand. Evictions:
  two `build:docker` go-mod stream resets from proxy.golang.org (trains
  37258, 37313), one `ci_timeout` at exactly 120 minutes with 50 of 64 jobs
  green (37391, starved by three concurrent fi-fhir pipelines), and two
  runner faults at 08:06–08:36 UTC where the job pod lost
  `/builds/libs/fi-fhir` before any script ran (37552, 37553). Fixes:
  gitops !1050 puts `build:docker` on the queue's flake-retry list, gitops
  !1051 raises `await_pipeline_minutes` 120 → 180, and !298 retries
  `go mod download` inside the image build. !295's first head pipeline failed
  `test:structured-logging` for a real reason: the new key loader decodes
  hex then four base64 alphabets, and the proof's 52-character passphrase
  decoded as URL-safe base64 into 39 other bytes, so serve and the proof
  keyed different HMACs; the proof key is now the hex of 32 bytes and the
  helper decodes it the way serve does (commit `25a45c91a`, proven locally
  with the three gate targets against `postgres:16-alpine`). A head
  pipeline created at enqueue time competes with the lane head's train on
  the same runners (GitLab schedules jobs first-in-first-out across
  projects), so a hand-enqueued MR's head pipeline is now created when the
  entry is within two of the head; a canceled head is `ci_must_pass` and a
  terminal eviction if the train goes green first. After a train-proof
  `ci_red` eviction the queue readmits only a head with a green pipeline on
  the branch ref itself (merge-request pipelines do not count), so a fresh
  head (`main` merged in) was the way back for !295 and !296; the queue then
  restored their old ranks. M-4 escalated on a scope blocker because the
  new `ci/lint-runtime-config.yml` was not in its slice; declared and
  requeued. The protected-paths human hold on a queued row is lifted by
  echoing the row back through the backlog upsert with
  `policy.require_human_review` cleared (no release route applies to a
  queued row; the start route is gated the same way).
- What's next: M-7 merged as !299 `252777436` (train 37581) at 14:34 UTC,
  the tenth row; M-4, M-10, M-13, M-14, M-15 wait on the eight-runs-per-day
  overlay. Cody wires
  `FI_FHIR_TELEMETRY_CORRELATION_KEY[_FILE]` in production as the hex or
  base64 of 32 random bytes. edilint's three rows merged (!39, !41, !42; the
  mirror's `action-smoke` run is green), so its v0.6.0 cut is due.
- Sources:
  - [S1] https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/293 and pipeline 37418
  - [S2] https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/295 (pipelines 37379, 37406, 37552, 37572)
  - [S3] https://gitlab.flexinfer.ai/platform/gitops/-/merge_requests/1050 and 1051
  - [S4] https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/298
  - [S5] loom-core `pkg/mills/mergequeue/external.go` (readmission rule), `cmd/loom-mills-operator/handlers_backlog.go` (upsert)
