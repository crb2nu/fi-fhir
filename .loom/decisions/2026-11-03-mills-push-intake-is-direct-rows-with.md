### 2026-11-03: Mills push intake is direct rows with declared files; carding stays off

- Decision:
  - **Intake for the November push is the direct `bl-fifhir-*` row**
    (`mills_backlog_post` with files, tests and declared protected touches);
    fifteen rows were posted from the Now list and
    `.loom/43-mills-push-2026-11-execution-specs.md` is the program view.
    Not the GitLab issue path (no slice files; the five-minute declaration
    window stopped gl-19-23 and gl-19-24 once) and not the roadmap carder
    (it reads one line per bullet and spins briefs nobody approves).
  - **Carding stays off**, but every unchecked Now bullet that a row owns
    now opens with the action, its paths and the row id, so turning the
    carder on later cards the right thing and a reader sees ownership at
    once.
  - **`ci/**` is a protected path** (platform/gitops !1049). A row that
    needs a CI include declares the touch and waits for a person; it does
    not route around it.
  - **Coordinator documentation merge requests merge directly** on a green
    head pipeline; every code row rides the train. A 64-job train proof for
    a docs-only tree re-proves nothing main's pipeline does not prove minutes
    later.
  - **The lane's kill-test** is the first UI row landing through the devbox
    (vitest, `svelte-check`) and the blocking `test:ui-e2e` on the train
    without a hand. If it escalates on infrastructure, the UI rows move to a
    coordinated worktree session as `.loom/42` was run; Go, docs and CI rows
    keep riding the lane.
- Rationale: the merge-train proof is real since today (MR !290: API
  pipeline 37258, 64 jobs on the train ref, no push pipeline), so the lane
  can carry code again. October's limit was intake shape: rows need real
  paths for the scope gate, and the Now list was prose.
- Alternatives considered: GitLab issues labelled `mills-eligible`
  (rejected: no files, late declaration); enabling the carder for this
  repository (rejected for now: the first lines were not carder-shaped, and
  spun drafts need an approver); an approved plan in the agent-context store
  routed to Mills (`plan_slice_emitter`; viable, but the direct row was
  proven the same day and needs no second bookkeeping surface).
- Consequences: `ROADMAP.md` Now reshaped; `.loom/43` added; fifteen
  fi-fhir rows and three edilint rows in flight within the existing budget
  overlays (3 USD × 8 per day here, 2 USD × 3 per day there); the operator
  half of the proof fix is loom-core row
  `bl-mills-mergequeue-train-proof-api-source-only-202611`; protected-path
  rows (M-3, M-4, M-7, M-8) will wait for a human read.
- Sources:
  - [S1] `.loom/43-mills-push-2026-11-execution-specs.md`
  - [S2] `.loom/decisions/2026-10-31-mills-train-proofs-are-hollow-until-the.md`
  - [S3] platform/gitops !1049; loom-core !3349
