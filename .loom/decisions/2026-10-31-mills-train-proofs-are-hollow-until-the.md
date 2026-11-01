### 2026-10-31: Mills train proofs are hollow until the queue mints its own API proof

- Decision:
  - **A train merge in this repository is proven by main, not by the train,
    until two fixes land.** The four rows that rode the merged-result train
    (!282, !283, !285, !286) were each "proven" by a push pipeline on the
    `mills-mq/train-*` ref that ran one job, `lint:helm`; the
    `$MILLS_MERGE_QUEUE == "1"` arm from MR !280 has never executed. Repo
    half: add a `workflow:rules` never-rule for push pipelines on `mills-mq/`
    refs (`$CI_PIPELINE_SOURCE == "push" && $CI_COMMIT_BRANCH =~ /^mills-mq\//`),
    as loom-core has, so the queue must mint the API pipeline that carries
    the variable; this touches `.gitlab-ci.yml`, a protected path, so it is a
    human merge request. Operator half: the queue adopts only its own `api`
    proof on a train ref (loom-core `processor_train.go`).
  - **The deferred lane decisions stay deferred, with these answers**: the
    HUD merge-request watcher does not add fi-fhir or edilint (its shepherd
    would enqueue every green non-draft MR onto today's hollow proof); the
    hub's `MILLS_MERGE_QUEUE_PROJECTS` adds both repositories after the proof
    fix (workstations already route them through the queue); main-pipeline
    dedup stays off (it would have skipped main's only full proof on three of
    four merges); `train_window` stays 1; the budget overlays stay 3 USD and 8
    runs per day here, 2 USD and 3 per day for edilint (five runs a day at
    most, 1.60 USD mean).
  - **Intake is the GitLab issue path, and its recipe is now written down**
    (ROADMAP, Mills posture): the description is the spec, a `priority:Pn`
    label sets priority, `mills-eligible` goes on last, files are declared
    before the first run, the MR says `Closes #n`. Issues #22–#24 are closed
    by hand.
  - **Protected paths widen**: fi-fhir adds `ci/**`; edilint adds
    `scripts/mirror-github.sh` and `internal/releasecheck/**` (gitops
    `configmap-policy.yaml`, with the checksum bump the 2026-10-31 edit
    missed).
  - **Roadmap carding stays off for both repositories** until each Now
    bullet's first line carries the action and its paths; edilint's heading
    becomes exactly `## Now` now. The two 2026-10-30 Spinning Room drafts are
    not approved as spun (both name files that do not exist).
  - **The July program plan is superseded by this file's Now list**; the
    ROADMAP header says so, and the plan should be closed in the store.
- Rationale: the merged-result premise is that the tree about to land has
  passed the gating set. On fi-fhir the proof pipelines were push pipelines
  because `lint:helm`'s last rule is a bare `changes:` and nothing refuses
  pushes to `mills-mq/` refs; the queue (`trainProofPipeline` →
  `BranchPipelineStatus`, sources `push` and `api`) adopted them and logged
  `minted: false`. Nothing broken landed only because each MR's own pipeline
  and main were green. Every row also needed a hand (scope amendments, the
  go-jose P1, rebases, a review release), so the lane is not yet autonomous
  for this repository; widening the watcher or enabling dedup now would
  remove the human checks that caught the gaps.
- Alternatives considered: an extra rule arm
  `$CI_COMMIT_REF_NAME =~ /^mills-mq\/train-/` on every gating job, so push
  pipelines on train refs run the full set (rejected: it duplicates 44 rules,
  keeps `changes:` evaluation on push pipelines, and leaves the queue trusting
  a pipeline it did not mint); trusting the train and enabling dedup
  (rejected: three of four merges had no full proof before main); raising the
  per-run cap after one 3.71 USD run (rejected: the cap is admission-time
  only and the spend was a devbox lint timeout, not the work).
- Consequences: fi-fhir Now gains "Make the merge-train proof real" at the
  top; both ROADMAP Mills posture sections carry the evidence and the intake
  recipe; the follow-up merge requests are the fi-fhir `workflow:` rule, the
  gitops protected-path and checksum change, and loom-core items for
  queue-owned proofs and for classifying a base advisory as something other
  than `code`; the Mills `test:ui-e2e` flake retry can be reconsidered now
  that !246 and !253 are merged.
- Sources:
  - [S1] GitLab project 19 pipelines 35918, 35921, 35971, 36007 (source
    `push`, one job each) and the absence of any `api`-source pipeline since
    2026-10-04.
  - [S2] Operator events `mergequeue.train_built` ×4 and `mergequeue.merged`
    with `proof_source: "train"`; operator log `train proof pipeline recorded
    … minted: false`; `train_tree_mismatch` 0.
  - [S3] loom-core `pkg/mills/mergequeue/processor_train.go`
    (`mintTrainProof`, `trainProofPipeline`),
    `pkg/mills/clients/gitlab_mergequeue.go` (`BranchPipelineStatus`),
    `.gitlab-ci.yml:70` (the `mills-mq/` push never-rule).
  - [S4] Run ledger for gl-19-22/23/24, the go-jose and QuickBenchmark items
    (eight runs, 12.77 USD); escalation diagnoses; MR !282/!283/!285 first
    pipelines failing only `security:trivy-image`.
  - [S5] loom-core `pkg/mills/intake/gitlab_importer.go`,
    `pkg/mills/intake/carder.go`, `pkg/mills/council/roadmap.go` (`Extract`),
    `cmd/loom-mills-operator/handlers_spin.go`; briefs
    `brief-roadmap-fi-fhir-…-6900be` and `brief-roadmap-edilint-…-ad1103`
    (`source: operator`); live policy `intake.carding.target_projects: null`.
  - [S6] Review notes in the session scratchpad (fi-fhir October review,
    edilint release readiness, Mills lane audit), 2026-10-31.
