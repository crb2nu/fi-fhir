### 2026-11-03 - Mills push: fifteen rows posted, spec 43, train acceptance recorded

- What changed: `.loom/43-mills-push-2026-11-execution-specs.md` is the
  program view of the November Mills push (rows M-1..M-15 with declared
  files and acceptance, order and dependencies, budget, the lane's
  riskiest assumption and kill-test, what stays with a person).
  `ROADMAP.md`: a "Mills push (2026-11-03)" subsection opens the Now list;
  every Now item that a row owns has its first line reshaped to the action,
  its paths and the row id; "Make the merge-train proof real" records the
  acceptance (train ref `mills-mq/train-libs-fi-fhir-290-1cd5e1c6`, no push
  pipeline, API pipeline 37258 with 64 jobs; edilint's first row the same
  way on pipeline 37257) and now names the loom-core half as its own row;
  Current Status and Mills posture carry dated updates; the Backlog table
  marks #22–#24 closed and #15 in the lane. A decision entry records the
  intake choice. No code changes.
- Why: the merge-train proof became real today, so the lane can carry code
  again, and what limited it in October was intake shape (issues without
  files; prose bullets the carder cannot use), not the queue. Fifteen rows
  with real paths and one merge request of work each are the form the scope
  gate admits without a hand; the roadmap has to say which bullets are
  owned so nobody posts them twice.
- Evidence: `mills_backlog_post` responses for the fifteen ids (all
  `queued`, two scope warnings about root-level envelopes accepted); two
  fi-fhir rows and the loom-core row were implementing within a minute of
  posting; pipelines 37258 and 37257 read from the API as `source: api`
  with 64 and 9 jobs (fi-fhir stages lint 11, test 42, security 6, build 3,
  scan 2); edilint's first row, MR !39, was held `review:spec-touching` at
  17:29Z because its only file is a test, reviewed and enqueued at 19:05Z.
- What's next: watch the first UI row through the devbox and `test:ui-e2e`
  (the kill-test in `.loom/43`); review every `review:*` hold the same way;
  Cody wires the production correlation secret once M-5 lands; the next
  entry that touches `ROADMAP.md` fills in merge ids.
- Sources:
  - [S1] `.loom/43-mills-push-2026-11-execution-specs.md`
  - [S2] `.loom/decisions/2026-11-03-mills-push-intake-is-direct-rows-with.md`
  - [S3] platform/gitops !1049 (`ci/**` protected, checksum re-synced);
    loom-core !3349 (policy mirror) and row
    `bl-mills-mergequeue-train-proof-api-source-only-202611`
