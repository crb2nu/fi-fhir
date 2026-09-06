# Decisions

**Entries now live in [`.loom/decisions/`](decisions/), one file per decision.**

This file is a stable landing page for the code comments, CI banners, and
planning documents that cite `.loom/40-decisions.md`. Do not add entries here —
see [`.loom/decisions/README.md`](decisions/README.md) for why.

That is enforced, not just asked: `scripts/decisions.sh check` fails on a dated
entry heading in this file, and runs in the blocking `lint:docs` CI job.

## Add an entry

```bash
bash scripts/decisions.sh new "Short title of the decision"
# or
make decisions-new TITLE="Short title of the decision"
```

That creates `.loom/decisions/YYYY-MM-DD-<slug>.md` pre-filled with the template
below. Commit it alongside the change it records.

## Read the whole journal

```bash
make decisions            # oldest first
make decisions-recent     # newest first
```

Both are `scripts/decisions.sh render`, which concatenates every entry in date
order. Nothing regenerates a committed copy: a generated file that each merge
request rewrites would reintroduce exactly the conflict this layout removes.

## Citations

Cite a decision by date and title — `.loom/40-decisions.md (2026-08-09, "What
one version means")` — and it resolves through this page to
`.loom/decisions/2026-08-09-what-one-version-means-and-why-rollback.md`. Line
numbers into the old single file (`.loom/40-decisions.md:1428` and friends in
the Sprint 4 and 5 specs) describe it as it stood before 2026-09-05;
`git show 17d41eabc:.loom/40-decisions.md` reproduces that file exactly.

## Entry template

```markdown
### YYYY-MM-DD: Decision title

- Decision:
- Rationale:
- Alternatives considered:
- Consequences:
- Sources:
  - [S1] …
```
