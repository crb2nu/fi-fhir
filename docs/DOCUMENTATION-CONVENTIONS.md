# Documentation Conventions

Rules for maintaining fi-fhir documentation. Follow these to keep docs accurate and prevent drift.

## Document Hierarchy

| Document | Purpose | Update Frequency |
|----------|---------|-----------------|
| `docs/STATUS.md` | Component maturity, coverage, freshness — **single source of truth** | After significant merges (`make docs-status`) |
| `CHANGELOG.md` | Feature-level history for users and contributors | Every feature PR (add to `[Unreleased]`) |
| `AGENTS.md` | AI assistant guidance — architecture, conventions, concise roadmap | When architecture changes |
| `docs/planning/README.md` | Feature Builds and prioritized backlog (P0–P3) | When priorities shift |
| `docs/planning/*.md` | Design documents for major features | When designs are created or completed |

## CHANGELOG Rules

1. **Every feature PR** adds an entry under `[Unreleased]` in the appropriate category.
2. **On version tag**, move `[Unreleased]` entries to a new versioned section (e.g., `## [0.2.0] - 2026-03-01`).
3. Use [Keep a Changelog](https://keepachangelog.com/) categories: Added, Changed, Deprecated, Removed, Fixed, Security.
4. Group entries by domain (Parsers, Workflow, FHIR, Terminology, etc.) within each category.
5. Reference file paths for significant additions (e.g., `pkg/matching/matcher.go`).

## STATUS.md Rules

1. **Refresh after significant merges**: Run `make docs-status` to regenerate coverage and freshness data.
2. **Maturity levels**: Production → Beta → Alpha → Planned (promote when criteria are met).
3. **Coverage column**: Average function-level coverage percentage from `coverage.out`.
4. **Last Updated column**: `git log -1 --format="%Y-%m-%d"` for the component path.
5. **Staleness threshold**: Entries >30 days old should be reviewed. `scripts/docs-status.sh` flags entries with >7 days drift from git.

### Maturity Criteria

| Level | Criteria |
|-------|----------|
| **Production** | Feature-complete, ≥70% coverage, used in deployment |
| **Beta** | Feature-complete, <70% coverage or limited deployment |
| **Alpha** | Functional but incomplete scope or <30% coverage |
| **Planned** | Design document exists, no implementation |

## AGENTS.md Rules

1. **Roadmap section stays concise** — maximum ~25 lines summarizing current state and linking to STATUS.md and planning README.
2. **No duplicate sections** — a single "Roadmap Context" block, not multiple "Completed" lists.
3. **No shipped features in "Remaining Backlog"** — remove items as they ship.
4. **Key Files Reference** stays current — add entries for new significant files.

## Planning README Rules

1. **Feature Build statuses** use ✅ Shipped / 🔄 In Progress / 🔲 Planned / 🟡 Partial.
2. **Backlog priorities** are P0 (release blocker) through P3 (future enhancement).
3. **Coverage table** in P2 section should reference `make docs-status` for latest numbers.
4. **When adding new planning docs**, update the Document Overview table in this README.

## New Component Checklist

When adding a new package or service:

1. **Add STATUS.md entry** — component name, path, maturity (start at Alpha), initial coverage.
2. **Add tests** — at least smoke tests; aim for 70%+ coverage.
3. **Add CHANGELOG entry** — under `[Unreleased] > Added`.
4. **Run validation** — `make docs-validate` should pass.
5. **Update AGENTS.md** — add to Key Files Reference if the component is architecturally significant.

## CI Enforcement

The `lint:docs` CI job runs `scripts/validate-docs.sh` on merge requests to check:
- Every component directory has a STATUS.md entry.
- CHANGELOG.md has an `[Unreleased]` section.
- AGENTS.md doesn't contain known stale patterns.

This job is a **blocking merge gate** (`allow_failure: false`) as of 2026-08-08,
promoted after 33 consecutive green runs on `main`. Run `make docs-validate`
before pushing.

The related `test:docs-status` job (STATUS.md coverage drift) remains advisory —
see `.loom/40-decisions.md` (2026-08-08) for the promotion criteria.

## Quick Reference

```bash
make docs-status          # Full refresh (re-runs tests + generates data)
make docs-status-quick    # Quick refresh (uses existing coverage.out)
make docs-validate        # Check documentation consistency
make docs-all             # Full doc maintenance (mermaid + status + validate)
```
