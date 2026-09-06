# Decision entries

One file per decision, named `YYYY-MM-DD-<slug>.md`.

## Why not one file

`.loom/40-decisions.md` used to hold every decision, appended in date order,
and it was the last shared append-only file in the repo after the worklog was
split in August. With several branches open at once, that made it the most
conflict-prone file in the tree: every branch that records a decision adds
lines at the same end-of-file position, git cannot auto-merge that, and every
sibling merge request has to be rebased and hand-resolved — the diff is never a
real disagreement, just two unrelated entries landing at once.

The Sprint 5 revival is the worked example. On 2026-09-04 eight merge requests
that each appended one decision were re-stacked twice in one afternoon: first
when `!195` landed, then when `!183` landed and re-conflicted all six remaining
siblings on this file alone. The only way to stop it was to chain every open MR
linearly on top of the next, so that a merge could never add an entry the MRs
above did not already carry. That is a workaround for a file layout, not a
merge strategy.

Separate files cannot collide. Two branches adding
`2026-09-05-lane-a-thing.md` and `2026-09-05-lane-b-other.md` merge cleanly with
no rebase.

## Adding an entry

```bash
bash scripts/decisions.sh new "Short title of the decision"
# or
make decisions-new TITLE="Short title of the decision"
```

Commit the new file with the change it describes. Never append a second entry
to an existing file — `scripts/decisions.sh check` fails on that, because it
recreates the conflict this layout exists to remove.

## Reading

```bash
make decisions            # whole journal, oldest first
make decisions-recent     # newest first
```

Both are `scripts/decisions.sh render`, which concatenates every entry in date
order. Nothing regenerates a committed copy: a generated file that each merge
request rewrites would reintroduce exactly the conflict this layout removes.

## Naming

- `YYYY-MM-DD` is the date the decision was made, and must match the entry
  heading, which keeps the journal's existing `### YYYY-MM-DD: Title` form.
- The slug is lowercase, hyphen-separated, and short — `decisions.sh new`
  derives it from the title and caps it at seven words.
- Several decisions on one day are normal (Sprint 5 recorded ten on 2026-08-09);
  give them different slugs. Same-day entries render in slug order, not in the
  order they were written — the date is the unit of ordering, as in the worklog.

`scripts/decisions.sh check` enforces all of this, and runs in CI as part of the
blocking `lint:docs` job. It also rejects a dated entry heading appearing in
`.loom/40-decisions.md`, because appending there is the old habit and brings the
end-of-file conflict straight back.

## Citing a decision

Cite by date and title, the way the code already does:
`.loom/40-decisions.md (2026-08-09, "What one version means")`. Those citations
still resolve — the pointer page sends the reader here — and the title is
stable where a line number is not. Line-number citations into the old single
file (for example `.loom/40-decisions.md:1428` in the Sprint 4 and 5 specs)
describe the file as it stood before 2026-09-05; `git show 17d41eabc:.loom/40-decisions.md`
reproduces it exactly.

## Amending a decision

A decision that is later corrected or superseded is amended **in its own file**,
the way `.loom/33-sprint5-execution-specs.md` correction 47 amended the FHIR
validator decision in place. Add a dated paragraph inside the entry rather than
a second `###` heading; the check treats a second heading as an append.
