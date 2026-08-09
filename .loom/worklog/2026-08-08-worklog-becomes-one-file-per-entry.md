### 2026-08-08 - Worklog becomes one file per entry

- What changed:
  - Split `.loom/50-worklog.md` into 24 files under `.loom/worklog/`, named
    `YYYY-MM-DD-<slug>.md`. Entry content is unchanged; the two stray `##`
    headings were normalised to `###` so the corpus is uniform.
  - `.loom/50-worklog.md` is now a short pointer page. It is not generated and
    no merge request edits it, so it cannot conflict. It stays because roughly
    twenty planning documents link to that path.
  - Added `scripts/worklog.sh` with `new`, `render`, and `check`, plus
    `make worklog-new TITLE="..."`, `make worklog`, and `make worklog-recent`.
  - Wired `worklog.sh check` into the blocking `lint:docs` job and into
    `make docs-validate`, and added `.loom/worklog/**/*` and
    `scripts/worklog.sh` to that job's `changes:` rules so a worklog-only
    merge request still runs it.
  - Repointed the live coordination guidance in
    `.loom/31-sprint3-execution-specs.md`, `.loom/24-parallel-execution-specs.md`,
    and `.loom/00-index.md`.
- Why:
  - A single append-only worklog was the most conflict-prone file in the repo.
    Two branches each appending an entry both add lines at the same
    end-of-file position, which git cannot auto-merge. With several agents
    shipping in parallel this fired constantly, and the conflict never
    represented a real disagreement — just two unrelated entries landing at
    once.
  - MR !143 hit it four times in one afternoon. Each round cost a rebase, a
    force-push, a re-armed merge-when-pipeline-succeeds, and a replayed manual
    `test:benchmark` job: roughly fifteen minutes of wall-clock per collision,
    none of it reviewing anything.
- Evidence:
  - The split is lossless: all 863 non-blank lines of the entry region are
    preserved across the 24 files, verified by comparing the concatenation
    against the original (modulo the two `##` to `###` heading fixes).
  - `scripts/worklog.sh render` reproduces the document: 866 non-blank lines
    against the original's 872, the difference being the nine-line header and
    template block replaced by a three-line generated header.
  - `worklog.sh check` was negative-controlled against four defects, each
    producing its own message and a non-zero exit: a filename that is not
    `YYYY-MM-DD-<slug>.md`, a heading date that disagrees with the filename, a
    file containing two entry headings, and a file whose first line is not a
    heading. The clean tree exits 0.
  - The two-headings case is the one that matters: it rejects appending a
    second entry into an existing file, which is how the old conflict would
    come back.
- What's next:
  - A deterministic duplicate-migration-number check in CI. Lanes record their
    claimed number in the worklog, and it was fair to ask whether splitting the
    file removes a guard — but Sprint 3 shows there was nothing to remove.
    S3-C and S3-A both needed a session migration and it was settled by merge
    order, not by the claim: S3-C merged first and took
    `0004_export_attribution.sql`, so S3-A renumbered to `0005`
    (`.loom/31-sprint3-execution-specs.md`, session-migrations row). A single
    file only conflicts when both lanes edit it before either merges, which is
    not what the claim protocol promises. The docs and
    `.loom/worklog/README.md` now say the directory on `origin/main` is the
    authority; a CI check is the durable fix and is not in this change.
- Sources:
  - [S1] `.loom/worklog/README.md` — the convention and its rationale
  - [S2] `scripts/worklog.sh`
  - [S3] MR !143, four rebases across pipelines 22592, 22613, 22630, 22634
