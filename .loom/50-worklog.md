# Worklog

**Entries now live in [`.loom/worklog/`](worklog/), one file per entry.**

This file is a stable landing page for the many documents that link to
`.loom/50-worklog.md`. Do not add entries here — see
[`.loom/worklog/README.md`](worklog/README.md) for why.

That is enforced, not just asked: `scripts/worklog.sh check` fails on a dated
entry heading in this file, and runs in the blocking `lint:docs` CI job.

## Add an entry

```bash
bash scripts/worklog.sh new "Short title of what you did"
```

That creates `.loom/worklog/YYYY-MM-DD-<slug>.md` pre-filled with the template.
Commit it alongside the change it describes.

## Read the whole log

```bash
make worklog              # oldest first
make worklog-recent       # newest first
```

Both are `scripts/worklog.sh render`, which concatenates every entry in date
order. Nothing regenerates a committed copy: a generated file that each merge
request rewrites would reintroduce exactly the conflict this layout removes.

## Entry template

```markdown
### YYYY-MM-DD - Short title

- What changed:
- Why:
- Evidence:
- What's next:
- Sources:
  - [S1]
```
