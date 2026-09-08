### 2026-09-08 - Sprint 6 merge surface and lane launch

- What changed (Lane S6-0, `.loom/34-sprint6-execution-specs.md`):
  - Pre-registered every shared append point the lanes would otherwise
    collide on: comment-only stubs `ci/test-fhir-destination.yml`,
    `ci/test-e2e-legacy.yml`, `ci/test-fhir-structural.yml` plus their three
    `include: local:` lines; two `.PHONY` lane lines (S6-A, S6-D); four
    fail-loud placeholder Makefile targets (`fhir-destination`,
    `fhir-destination-negative-control`, `fhir-structural`,
    `fhir-structural-negative-control`) that the lanes replace in place. S6-C
    reuses the existing `test-e2e` target and needs no Makefile line.
    `ci/job-inventory.txt` regenerated (unchanged — the stubs add no job).
  - `.loom/30` Slice 5.1 no longer says "blocked on 4.1c-b"; it names 4.1c-c and
    the gate that proved it, and gains a Slice 4.1c-c pointer under 4.1.
  - `ROADMAP.md` restructured to state: Phases 0–2 and 3.1/3.2 marked delivered
    where they were still unchecked; Sprint 6 is "Now"; the 1.0 remainder is
    "Then".
  - Decision entry: `.loom/decisions/2026-09-08-fund-slice-4-1c-c-as-a-fhir-transport-kind.md`
    (Option A ruled).
- Lanes launched the same day, each in its own worktree and branch:
  - S6-A `feat/phase4-slice-4-1c-c-fhir-destination`
  - S6-C `test/e2e-legacy-repair`
  - S6-D `feat/phase5-slice-5-1b-package-pinning`
  - **S6-B is held**: `GET /projects/19/variables/FI_FHIR_PERF_RUNNER` still
    returns 404, so `test:performance-profile` is `when: never` and the lane
    has nothing to play. Setting a project CI variable is operator
    configuration; the coordinator does not set it.
- Why:
  - Sprint 5 re-stacked eight MRs twice in one afternoon because every
    implementation MR appended to the same Makefile block and include list.
    A comment-only CI include is valid and a placeholder target is a valid
    recipe, so the shared lines can exist before the lanes do.
- Evidence:
  - `cd ui && npm audit --audit-level=high` → 3 low, 0 high (no lockfile
    ride-along needed for a `.gitlab-ci.yml` edit; see !197).
  - `bash scripts/ci-job-inventory.sh --check`, `make -n fhir-destination`
    (parses; the recipe exits 1 by design when run), `bash scripts/validate-docs.sh`,
    `scripts/worklog.sh check`, `scripts/decisions.sh check` — all pass.
- What's next:
  - Lanes open test-only day-1 gate MRs immediately and implementation MRs
    after this MR merges; merge order `S6-0 → {S6-B, S6-C} → S6-D → S6-A`.
  - Coordinator writes one `CHANGELOG.md` `[Unreleased]` block at sprint close.
- Sources:
  - [S1] `.loom/34-sprint6-execution-specs.md` — Lane S6-0, corrections 19–21
  - [S2] `.loom/decisions/2026-09-08-fund-slice-4-1c-c-as-a-fhir-transport-kind.md`
  - [S3] `.loom/worklog/2026-09-07-sprint-6-planning-the-critical-path-opens.md`
