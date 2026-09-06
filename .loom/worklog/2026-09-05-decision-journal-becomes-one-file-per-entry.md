### 2026-09-05 - Decision journal becomes one file per entry

- What changed:
  - `.loom/40-decisions.md` is now a pointer page, like `.loom/50-worklog.md`.
    Its 43 entries were split into `.loom/decisions/YYYY-MM-DD-<slug>.md`, one
    file each, with the split verified to round-trip byte-for-byte against the
    original entry text.
  - `scripts/decisions.sh` (`new` / `render` / `check`) mirrors
    `scripts/worklog.sh`; `make decisions`, `make decisions-recent`, and
    `make decisions-new TITLE=...` wrap it; `make docs-validate` and the blocking
    `lint:docs` CI job run the check.
  - The one code citation by line number
    (`internal/integration/retention/purge_throughput_gate_integration_test.go`)
    now names the entry file; the same stale citation inside the purge-role
    decision's own sources was corrected too.
- Why:
  - Sprint 5's revival (2026-09-04/05) re-stacked eight merge requests twice in
    one afternoon, and after the first round the only file that conflicted was
    this journal: every MR appended one entry at end-of-file. The workaround was
    a single linear chain of MRs, which is a merge strategy standing in for a
    file layout. The worklog had the same disease until August; this is the same
    cure.
- Evidence:
  - `bash scripts/decisions.sh check` → `decisions: OK (44 entries)` (43 migrated
    plus the entry recording this change).
  - `bash scripts/worklog.sh check` unchanged.
  - The migration's own assertion: concatenating the new files in the original
    order equals the original body with trailing blank lines normalised.
  - The line citation `.loom/40-decisions.md:1631-1632,1659,1667`, written for
    the Slice 4.1e decision, already pointed into the FHIR conformance decision
    by the time of the split — line numbers into an append-only file rot as
    earlier entries are amended. Titles do not.
- What's next:
  - `.loom/32` and `.loom/33` still cite the old file by line number; they are
    dated program artifacts and the pointer page says how to reproduce the file
    they cite (`git show 17d41eabc:.loom/40-decisions.md`).
- Sources:
  - [S1] `.loom/decisions/README.md`
  - [S2] `.loom/worklog/2026-08-08-worklog-becomes-one-file-per-entry.md`
  - [S3] `.loom/decisions/2026-09-05-record-decisions-one-file-per-entry.md`
