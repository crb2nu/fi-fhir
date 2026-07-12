# MCP Inventory Snapshot

**Refreshed**: 2026-07-12
**Project**: `libs/fi-fhir`

## Runtime detection

- Loom mode is active through `loom://config`, `loom://servers`,
  `loom://tools/index`, and `loom://health`.
- Inventory at review time: 25 registered servers and 317 aggregated tools.
- Agent Context is available and holds session
  `ed063fdc58fe44d8` in namespace
  `fi-fhir/integration-engine-ide-completion`.
- GitLab, GitHub, Kubernetes, observability, research, time, and local execution
  capabilities are discoverable through Loom.

## Known capability failures

- Codebase-memory stats/index access is not usable in this environment. The
  configured Qdrant endpoint returns no route to host, despite the aggregate health
  surface reporting the server healthy.
- The agent-context worktree allocator failed because its hub process could not
  find `git` on PATH. The compliant repo-local worktree was therefore created
  manually at:
  `/Users/cblevins/workspace/libs/fi-fhir/.worktrees/integration-engine-completion`.
- These are tooling limitations, not reasons to block file/command-backed review.
  They must not be represented as a successful semantic index or allocator run.

## Evidence and fallback used

- Refreshed `origin/main`, inspected commits/MRs/pipelines, and used `rg`,
  targeted file reads, and tests as the source of code truth.
- Delegated independent read-only engine, IDE, delivery, planning, and security
  reviews; reconciled their evidence in the completion spec.
- Verified the merged Integration Session core rather than relying on stale plan
  state.
- Proved GraphQL WebSocket transport manually by subscribing before a session
  run and observing ordered run/stage/diagnostic/completion events.
- Used official primary product/standard sources for external comparison.

## Verification snapshot

- Focused Go integration-session/GraphQL tests: pass.
- Full Go suite: one clean pass during audit; an internal/workflow repeat run
  exposed an intermittent/noisy flake that passed in isolation and remains a
  quality item rather than a suppressed result.
- UI `npm ci --no-audit --no-fund`: pass.
- UI Vitest: 571 passed; 2 live integration tests skipped.
- UI type/check/lint/style/build: pass with 9 unused-CSS warnings from Svelte
  check.
- Frozen pnpm install: fails because the pnpm lock omits current
  `postcss-html` and `stylelint` dependencies. npm/package-lock is the current
  reproducible path and Gate 0B will make it canonical.
- Live backend smoke review: `scripts/smoke-test.sh` exits after its first
  successful check because arithmetic increment returns status 1 under `set -e`.
- Go 1.26.5 compatibility, pinned govulncheck v1.6.0, and pinned gosec v2.27.1:
  pass on the stable Gate 0A candidate.

## Operating rule

Until the two Loom failures are repaired:

1. use direct Git/file/test evidence for implementation decisions;
2. record plan/session state in Agent Context when available;
3. create worktrees only under the owning repo after the workspace sprawl audit;
4. never claim index-backed coverage or allocator success when the fallback path
   was used;
5. track codebase-memory and hub-PATH repair as enablement work outside the
   clinical data-plane completion slices.

## Sources

- Loom resources: `loom://config`, `loom://servers`,
  `loom://tools/index`, `loom://health`
- Agent Context session: `ed063fdc58fe44d8`
- Commands: `rg`, `git status/log/diff`, targeted/full Go tests, npm/Vitest,
  Svelte checks, live GraphQL WebSocket proof
- Tool errors: Qdrant no-route-to-host; worktree allocator missing `git` on PATH
