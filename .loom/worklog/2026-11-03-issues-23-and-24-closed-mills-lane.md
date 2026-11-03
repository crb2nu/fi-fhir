### 2026-11-03 - Issues 23 and 24 closed, Mills lane intake and the train acceptance probe

- What changed: `ROADMAP.md` ticks the Now item "Close issues #22, #23 and
  #24". #22 was already closed; #23 and #24 were closed on 2026-11-03 with
  notes naming the merge request and Mills row that delivered each
  ([MR !283](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/283)
  / gl-19-23 and
  [MR !282](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/282)
  / gl-19-24), what each proved (the default-off proof is an in-process
  composition test in `cmd/fi-fhir/serve_tracing_test.go`, not a booted
  `serve`; production still sets no `FI_FHIR_TELEMETRY_CORRELATION_KEY`) and
  why they had stayed open (no merge request carried a `Closes` line). No
  `mills-eligible` issue is open on this project. Nothing else changes in
  the repository.
- Why: the Now item asked for exactly this. The merge request that carries
  it is also the first row to enter the Mills merge queue since
  [MR !288](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/288)
  landed the `workflow:` never-rule for push pipelines on `mills-mq/` refs,
  so it doubles as that rule's acceptance probe: the queue builds the merged
  result on a `mills-mq/train-libs-fi-fhir-*` ref, the ref update must mint
  no push pipeline, and the queue must therefore create its own `api`-source
  pipeline carrying `MILLS_MERGE_QUEUE=1`, which runs the gating set through
  the arm from
  [MR !280](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/280)
  for the first time. A documentation row is the cheapest real probe: the
  queue mints the same proof for any change.
- Evidence: issue notes 41709 (#23) and 41711 (#24) on
  `gitlab.flexinfer.ai/libs/fi-fhir`; `glab issue list --label
  mills-eligible` returns nothing open. The train observation (pipeline
  source and job count on this row's train ref, against the one-job push
  pipelines 35918, 35921, 35971 and 36007 of 2026-10-29/30) is recorded in
  the ROADMAP's "Make the merge-train proof real" item once this row has
  merged, by the next entry that touches `ROADMAP.md`.
- What's next: the same day, platform/gitops adds `ci/**` (fi-fhir) and
  `scripts/mirror-github.sh` plus `internal/releasecheck/**` (edilint) to the
  Mills protected paths and re-syncs the policy checksum that had drifted
  since 2026-10-31; edilint gets the same never-rule; the loom-core operator
  half (adopt only the `api` proof on a train ref) is posted as a loom-core
  backlog row; and, once this row's train shows the API proof, four Now items
  are posted as `bl-fifhir-*` rows with declared files (the tracing negative
  control's sentinel assertion, `scripts/check-runtime-config.sh` in CI, the
  Engine tab's tracing and delivery keys, the telemetry docs drift).
- Sources:
  - [S1] fi-fhir issues #23 and #24, closed 2026-11-03.
  - [S2] `.loom/decisions/2026-10-31-mills-train-proofs-are-hollow-until-the.md`
    and the 2026-10-31 worklog entry on the hollow train proofs.
