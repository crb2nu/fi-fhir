### 2026-08-09 - Sprint 5 Lane S5-0 MR 0a: per-lane CI include files

First MR of the sprint. Nothing else may append to `.gitlab-ci.yml` until this
merges (`.loom/33` coordination rules, the sprint's one hard sequencing rule).

- Owned files for the whole lane (recorded before first commit, per `.loom/33`
  coordination rules). Three MRs, disjoint file sets, merged 0a → 0b → 0c:
  - **0a** — `.gitlab-ci.yml`, `ci/**` (new), `scripts/ci-job-inventory.sh`
    (new), `Makefile` `.PHONY` block, `AGENTS.md` "CI job layout" (new section)
  - **0b** — `internal/integration/destination/transport.go` and
    `transport_test.go`; one characterization test in
    `internal/integration/delivery/dispatcher_test.go`
  - **0c** — `internal/integration/migrationcompat/migration_rule_test.go`,
    `negative_control_integration_test.go` (comment text only), `AGENTS.md`
    "Migration authoring" rule 1
  - No migration number claimed. This lane authors no schema change; D2's fix is
    a context lifetime and D4's is a regexp.

- Day-1 gate — `scripts/ci-job-inventory.sh`, run on unmodified `main`
  (`2f8b3f609`, docs-only over `852d7f3ee`). **Expected: PASS. Result: PASS, and
  it disconfirmed the spec's job count.**
  - It emits **55** jobs, not the 59 `.loom/33` correction 56 claimed. The file
    has 59 top-level keys; four of them — `include`, `stages`, `variables`,
    `default` — are not jobs. `.loom/33` corrected in four places before this
    code was written.
  - The misplacement claim is confirmed exactly as stated. With
    `--with-source`, every other `test:` job is declared between
    `.gitlab-ci.yml:441` and `:1656`; `test:destination-transport` is at
    `:2672`, after `mirror:github` (`:2626`) and `radar-scan` (`:2650`), at
    `stage: test` with `allow_failure: false`.

- Found while implementing — **D5, a live defect in the second file this fix
  covers.** `Makefile:725` was a lone `\tdestination-transport \` line wedged
  between `dev-ui-down`'s recipe and the `destination-transport` comment block:
  a `.PHONY` continuation a Sprint 4 rebase dropped 700 lines from its block.
  Make treats it as a continuation of the open recipe and the trailing backslash
  swallows the next comment line, so `make dev-ui-down` ran
  `destination-transport # Slice 4.1c-b: ...` as a shell command. Proved with
  `make -n dev-ui-down` before and after. No CI job invokes `dev-ui-down`, which
  is why it survived. Filed as D5 in `.loom/33` and fixed here.

- What changed:
  - **Six proof jobs moved to one file each under `ci/`**, with their comment
    blocks, plus seven `- local:` lines in the root `include:`. Only those six
    moved; `test:unit`, `test:integration`, the lint stage, and everything in
    `security`/`build`/`deploy`/`release` are untouched.
  - **`.go-mr-rules`** — a hidden job carrying the full four-entry `rules:`
    block, added to `.gitlab-ci.yml` beside `.go-changes`. This is the
    load-bearing part: YAML anchors are file-scoped, so a moved job cannot alias
    `*go-changes`, and `.go-changes` is a bare sequence that neither `extends:`
    nor a two-element `!reference` can reach. It lives in the root rather than
    in `ci/_shared.yml` so the change-set list has exactly one copy.
  - **`ci/_shared.yml`** — `.integration-proof` (stage, the extends chain, the
    PostgreSQL-16 service, the shared variables, `allow_failure: false`, a
    parameterised failure banner) and `.integration-proof-toolchain` (the
    `apt-get` line five jobs repeated), referenced rather than extended because
    `extends:` replaces a `script` array wholesale.
  - **`scripts/ci-job-inventory.sh`** with `--with-source`, `--write` and
    `--check`, the generated `ci/job-inventory.txt`, and `--check` wired into
    the blocking `lint:docs` job beside `worklog.sh check`. `lint:docs` now also
    triggers on `.gitlab-ci.yml`, `ci/**/*`, and the script itself.
  - **`Makefile`**: `.PHONY` proof targets split one line per lane with the
    convention written above them, and D5's stray fragment deleted.
  - **`AGENTS.md`**: a "CI job layout" section stating the rule, the two GitLab
    mechanics that make it work (anchors are file-scoped; `extends:` merges maps
    and replaces arrays), and the regeneration command.

- Why: `.loom/50-worklog.md` is the precedent. A shared append point was this
  repo's worst conflict source until it became one file per entry with a
  blocking check. `.gitlab-ci.yml` is the same shape with a worse failure mode —
  not a conflict, which is loud, but a job that lands in the wrong place,
  inherits the wrong rules by proximity, and is never noticed because it is
  green. Sprint 4 had five lanes and produced two such artifacts (`:2672` and
  D5). Sprint 5 has six more lanes.

- Evidence:
  - **The inventory is byte-identical across the split.**
    `scripts/ci-job-inventory.sh` on `origin/main` and on this branch produce
    the same 55 lines; `diff` is empty.
  - **Stronger: the resolved job configuration is identical.** A resolver that
    applies GitLab's rules (merge maps, replace arrays, splice `!reference`)
    was run over all six jobs before and after. `test:migration-compatibility`
    and `test:transport-gate` are byte-identical. The other four differ in
    exactly one intended way: the `after_script` banner is now
    `${CI_JOB_NAME} FAILED — ${PROOF_FAILURE_SUMMARY}` plus three new `PROOF_*`
    variables, which renders the identical four lines (verified by executing the
    banner with the variables set). `test:observability-replicas` gains a banner
    it did not have. Image, `before_script`, cache, services, every other
    variable, the full `script` array, `rules`, `stage`, and `allow_failure` are
    unchanged for all six.
  - `make -n dev-ui-down`, `make -n destination-transport`, `make -n phi-audit`,
    `make -n transport-gate-negative-control` all resolve after the `.PHONY`
    split.
  - `bash scripts/validate-docs.sh` and `bash scripts/worklog.sh check` pass.

- What's next: MR 0b (D2 — the HTTPS provenance write gets its own context
  budget; release blocker) and MR 0c (D4 — the migration rule enforces the rule
  it documents). 0c must merge before S5-D and S5-F author migrations.

- Sources:
  - [S1] `.loom/33-sprint5-execution-specs.md` — Lane S5-0 MR 0a, corrections
    56 and 57, File-Ownership Map, Coordination Rules
  - [S2] `.gitlab-ci.yml:1416-1685,2653-2734` at `852d7f3ee` — the six moved
    jobs in their original positions
  - [S3] `Makefile:1-17,725` at `852d7f3ee` — the `.PHONY` block and D5
  - [S4] `.loom/worklog/` and `scripts/worklog.sh` — the precedent this copies
