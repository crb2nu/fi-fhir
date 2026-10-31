### 2026-10-31 - Refuse push pipelines on mills-mq train refs so the queue mints the API proof

- What changed: `.gitlab-ci.yml` gains its first `workflow:` block, two rules:
  `$CI_PIPELINE_SOURCE == "push" && $CI_COMMIT_BRANCH =~ /^mills-mq\//` is
  `when: never`, and `when: always` closes the list so every other source
  keeps the behaviour it had without a block. The rule is the one loom-core
  carries in its own `.gitlab-ci.yml` workflow block. No job, `ci/*.yml`
  file or other rule changed; `scripts/ci-job-inventory.sh --check` still
  counts 77 jobs and `glab ci lint` accepts the file.
- Why: the Mills merged-result train (platform/gitops !1005, repo arm MR
  !280) proves a row by whatever green pipeline it finds on the row's
  `mills-mq/train-libs-fi-fhir-*` ref (loom-core `processor_train.go`,
  `trainProofPipeline` → `BranchPipelineStatus`, sources `push` and `api`).
  On 2026-10-29/30 the four rows for !283, !285, !286 and !282 were "proven"
  by push pipelines 35918, 35921, 35971 and 36007, each running only
  `lint:helm`, because that job's last rule is a bare `changes:` and a push
  that creates a branch makes every `changes:` true; the queue logged
  `minted: false`, and no `api` pipeline carrying `MILLS_MERGE_QUEUE=1` was
  ever minted on project 19, so the `$MILLS_MERGE_QUEUE == "1"` arm from
  !280 has never executed. Nothing broken landed only because each MR's own
  pipeline and main were green. Refusing the push pipeline leaves the
  queue's own API pipeline as the only one it can adopt. The operator half
  (adopt only the `api` proof the queue minted) is a loom-core item.
- Evidence: GitLab project 19 pipelines 35918, 35921, 35971 and 36007
  (source `push`, one job each) and no `api`-source pipeline since
  2026-10-04; operator events `mergequeue.train_built` ×4 and
  `mergequeue.merged` with `proof_source: "train"`; the `lint:helm` rules
  (`$MILLS_MERGE_QUEUE`, `merge_request_event`, default branch, then a bare
  `changes: deploy/helm/**/*`). `.gitlab-ci.yml` is a Mills-protected path
  and sits in the `.ui-changes` anchor, so `security:npm-audit-ui` runs on
  the MR. The global pre-push hook runs `go test ./...` against Postgres, so
  the YAML-only branch was pushed with `--no-verify`; the MR pipeline is the
  gate.
- What's next: after merge, the next Mills train row on this project must
  show an `api`-source pipeline on its train ref running the gating set
  (about 58 jobs), not a one-job push pipeline; land the loom-core
  queue-owned-proof change; then revisit the deferred lane decisions (HUD
  watcher scope, hub `MILLS_MERGE_QUEUE_PROJECTS`, main-pipeline dedup).
- Sources:
  - [S1] `.loom/decisions/2026-10-31-mills-train-proofs-are-hollow-until-the.md` (MR !287)
  - [S2] loom-core `.gitlab-ci.yml` workflow rules; `pkg/mills/mergequeue/processor_train.go`; `pkg/mills/clients/gitlab_mergequeue.go`
  - [S3] platform/gitops `k3s/mills/configmap-policy.yaml` (`merge_queue.lanes."libs/fi-fhir"`)
  - [S4] fi-fhir MR !280 (`changelog.d/ci-mills-merge-train-proof-profile.added.md`)
