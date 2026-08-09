### 2026-08-08 - Lane S4-E transport-gate role narrowing

- What changed: the GraphQL transport gate stopped being binary. It now
  enumerates all 131 schema root fields (64 Query, 60 Mutation, 7 Subscription)
  in `internal/api/graphql/operation_authorization_roles.go` and refuses any
  root field it has no entry for. Sixteen Slice 4.2a control-plane fields carry
  fine-grained AND-sets that mirror `operator.Service.authorize` exactly — nine
  reads on `integration.operator`, three recovery mutations on
  `integration.operator` + `integration.delivery.operator`, four lifecycle
  commands on `integration.operator` + `integration.deployment.operator`.
  `graphql:operator` is retained as a **named compatibility grant** that expands
  to the full set, and `serve` now prints the mapping's shape at startup.
  `integration:preview` and the SSE stream-context rule are unchanged.
- Why: `.loom/31` correction 20, re-filed by S3-C1. Three slices had shipped
  fine-grained roles one layer beneath a gate that still said yes to all of them
  together for anyone holding the blanket role. Every additional role widened
  the gap; the fix was one Go file (`.loom/32` correction 33).
- Evidence:
  - **Day-1 gate PASSED.** `TestTransportGate_LeastPrivilegeIsRefusedToday`, run
    at `55412bdaa` with `git diff --stat HEAD` empty and the test file as the
    only untracked path: a real 4.1a OIDC token holding `integration.operator` +
    `integration.deployment.operator` and **not** `graphql:operator` was refused
    with `FORBIDDEN` / "GraphQL operation forbidden" on **16/16** control-plane
    operations. The premise held; `.loom/32` needed no correction. The same case
    now asserts the inverse and lives as the kill-test's first subtest.
  - **Riskiest assumption disconfirmed; lane re-scoped.** `.loom/32:469` set the
    trigger: re-scope if more than a handful of fields land in the compatibility
    bucket. Enumeration put **115 of 131** there, so the lane narrowed the 4.2a
    integration control plane only and left the legacy catalog behind the
    compatibility grant with per-group `TODO`s naming follow-up slices.
  - Kill-test `TestTransportGate_FineGrainedRolesReplaceBlanketOperator` plus
    `..._CompatibilityGrantBehavesExactlyAsBefore` and
    `..._PreviewRoleIsUnchanged`: 172 subtests green on the real handler.
  - **Negative control:** `make transport-gate-negative-control`. The
    `transportgateblanket` build tag restores the pre-Sprint-4 blanket allow and
    **106 refusal subtests fail open**; 0 fail without it. The target inverts its
    exit status, so a kill-test that survives the blanket allow is a CI failure.
  - Exhaustiveness is checked against `parsedSchema` — the `*ast.Schema` the
    server executes, not a re-parse — and fails in both directions. Removing one
    mapping entry was verified to break it.
  - Gates: `gofmt` clean, `golangci-lint run` 0 issues, `go vet ./...` clean,
    `go test -race ./...` green, and `generated.go` / `schema.graphql` /
    `models_gen.go` / `ui/src/lib/gen/graphql.ts` untouched (correction 33 —
    the Sprint 4 schema lock was not spent).
  - Atomicity (correction 34): all five documents that hand out
    `graphql:operator` ship in this MR. They document the grant as deprecated
    and explain the new narrow control-plane token — they do **not** tell
    operators to swap it out, because 115 root fields including the whole IDE
    surface are still behind it.
- What's next: narrow the compatibility bucket. The `TODO`s group it into
  `S5-legacy-catalog-roles` (event/patient browser, workflow catalog, Temporal,
  debugger), `S5-session-workspace-roles` (the session workspace and all seven
  subscriptions), `S5-profile-roles`, `S5-llm-roles`,
  `S5-terminology-governance-roles`, and `S5-submit-roles`. Each needs a role
  invented and shipped at the service layer first — this gate can only enforce
  roles that already exist.
- Sources:
  - [S1] `.loom/32-sprint4-execution-specs.md:96-100,144-150,161-163,450-469`
  - [S2] `internal/integration/operator/service.go:71-92,108-299`
  - [S3] `internal/integration/session/types.go:179-184,247-248`
  - [S4] `.loom/iteration-plan-phase4-transport-gate-roles.md`
