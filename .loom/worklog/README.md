# Worklog entries

One file per entry, named `YYYY-MM-DD-<slug>.md`.

## Why not one file

`.loom/50-worklog.md` used to hold every entry, appended in date order. With
several agents shipping in parallel, that made the worklog the single most
conflict-prone file in the repo: two branches each appending an entry both add
lines at the same end-of-file position, and git cannot auto-merge that. Every
concurrent merge request had to be rebased and hand-resolved, and the diff was
never a real disagreement — just two unrelated entries landing at once.

Separate files cannot collide. Two branches adding
`2026-08-08-lane-a-thing.md` and `2026-08-08-lane-b-other.md` merge cleanly with
no rebase.

## Adding an entry

```bash
bash scripts/worklog.sh new "Short title of what you did"
```

Commit the new file with the change it describes. Never append a second entry
to an existing file — `scripts/worklog.sh check` fails on that, because it
recreates the conflict this layout exists to remove.

## Reading

```bash
make worklog              # whole log, oldest first
make worklog-recent       # newest first
```

## Naming

- `YYYY-MM-DD` is the date the work happened, and must match the entry heading.
- The slug is lowercase, hyphen-separated, and short — `worklog.sh new` derives
  it from the title and caps it at seven words.
- Two entries on one day are fine; give them different slugs.

`scripts/worklog.sh check` enforces all of this, and runs in CI as part of the
blocking `lint:docs` job. It also rejects a dated entry heading appearing in
`.loom/50-worklog.md`, because appending there is the old habit and brings the
end-of-file conflict straight back.

## The worklog is not a lock

Parallel lanes have used the worklog to claim things before writing them — most
often the next free migration number, per
[`.loom/31-sprint3-execution-specs.md`](../31-sprint3-execution-specs.md).

That never worked, and it did not work before this change either. Sprint 3 is
the worked example: S3-C and S3-A both needed a session migration, and it was
settled by merge order rather than by the claim — S3-C merged first and took
`0004_export_attribution.sql`, so S3-A renumbered to `0005`. A single
append-only file would only have conflicted if both lanes happened to edit it
before either merged, which is not what the claim protocol promises.

So this layout does not remove a working guard; it removes a coincidence that
fired mostly on unrelated entries. Recording the number is still worth doing as
a note to other lanes, but treat the directory on `origin/main` as the
authority: re-check it immediately before you commit, and prefer a
deterministic CI check over the honour system.
