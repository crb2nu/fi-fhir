### 2026-08-08 - Enforce that the worklog pointer page stays a pointer

- What changed:
  - `scripts/worklog.sh check` now fails when `.loom/50-worklog.md` contains a
    dated entry heading (`##` or `###`), with a message pointing at
    `worklog.sh new`.
  - Added `.loom/50-worklog.md` to the `lint:docs` `changes:` rules so an MR
    that touches only that file still runs the check.
  - Corrected `.loom/worklog/README.md`, which said the check runs in
    `test:docs-status`. It runs in `lint:docs` — the distinction matters
    because `test:docs-status` is `allow_failure: true` and would not block.
  - The pointer page now states that the rule is enforced rather than merely
    requested.
- Why:
  - The previous change moved entries into `.loom/worklog/` and left
    `50-worklog.md` as a landing page for the ~20 documents that link to it.
    The check validated the directory but never the pointer page, so appending
    an entry to `50-worklog.md` passed cleanly — and that is precisely the
    end-of-file append that produced the original merge conflicts. The
    convention was documented but only half enforced.
- Evidence:
  - Reproduced the gap on `origin/main` before fixing: appending
    `### 2026-08-09 - ...` to `.loom/50-worklog.md` and running the check gave
    `worklog: OK (27 entries)`, exit 0.
  - After the fix, five cases verified: clean tree exits 0; a `###` entry in
    the pointer page fails; the `##` variant (the stray style two migrated
    entries used) also fails; the pointer page's own `YYYY-MM-DD` template
    block does not false-positive, because the literal placeholder does not
    match the date pattern; `shellcheck` clean.
- What's next:
  - Unchanged from the previous entry: a deterministic
    duplicate-migration-number check in CI is still the open follow-up.
- Sources:
  - [S1] `scripts/worklog.sh` `cmd_check`, pointer-page rule
  - [S2] `.loom/worklog/2026-08-08-worklog-becomes-one-file-per-entry.md`
  - [S3] MR !147 (merge `503c4a37`), which introduced the gap
