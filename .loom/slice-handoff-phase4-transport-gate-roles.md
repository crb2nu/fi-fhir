# Slice Handoff — Sprint 4, Lane S4-E: Transport-Gate Role Narrowing

**Status**: Complete
**Lane**: S4-E (`.loom/32-sprint4-execution-specs.md:450-469`, corrections 33-34)
**Branch**: `feat/phase4-transport-gate-roles`
**Base**: `origin/main` @ `55412bdaa`
**Date**: 2026-08-08

## Day-1 gate — PASS

`TestTransportGate_LeastPrivilegeIsRefusedToday`, run at `55412bdaa` with
`git diff --stat HEAD` empty and the test file the only untracked path.

A real 4.1a OIDC token holding `integration.operator` +
`integration.deployment.operator` and **not** `graphql:operator` was refused
with `"code":"FORBIDDEN"` and `GraphQL operation forbidden` on **16/16** Slice
4.2a control-plane operations. The gate's premise — that the fine-grained roles
the merged 4.2b UI depends on are decorative at the transport gate — held
exactly as `.loom/32` predicted, so no correction to the spec was needed.

That refusal is what this slice removes, so the case now asserts the inverse and
lives as the first subtest of
`TestTransportGate_FineGrainedRolesReplaceBlanketOperator`.

## Riskiest assumption — disconfirmed, lane re-scoped

`.loom/32:469` named the assumption ("the shipped fine-grained roles are already
sufficient to express every operation") and the trigger: re-scope if more than a
handful of fields land in the compatibility bucket.

**115 of 131 landed there.** The lane re-scoped to the Slice 4.2a integration
control plane, exactly as the spec instructed.

| Requirement | Fields |
|---|---:|
| `integration.operator` | 9 |
| `integration.operator` + `integration.delivery.operator` | 3 |
| `integration.operator` + `integration.deployment.operator` | 4 |
| **Fine-grained subtotal** | **16** |
| `graphql:operator` (explicit compatibility bucket) | 115 |
| **Total** | **131** |

Two candidate mappings were rejected on evidence rather than deferred:

- **`integration.phi.export` is not a field-level role.** It gates the
  `includeRawPayload` *argument* of `exportIntegrationBundle`
  (`internal/integration/session/types.go:247`), not the field. Mapping it would
  have widened access and contradicted the acceptance criterion that it reaches
  nothing at the transport gate. `TestTransportGateNeverRequiresPHIExportRole`
  enforces its absence.
- **`integration:submit` / `integration:mllp` / `integration:batch` are
  transport-minted**, never carried by a GraphQL token. Mapping the submit
  mutations to them would hand ingress principals a GraphQL surface.

`health` also stays in the compatibility bucket: opening it to any authenticated
caller would undo the existing "unprivileged role stops before resolver" case in
`oidc_security_test.go:52`. Its low-privilege path remains the untouched
`integration:preview` allowlist.

## What shipped

1. `internal/api/graphql/operation_authorization_roles.go` — the 131-field map.
   Values are AND-sets mirroring `operator.Service.authorize`
   (`internal/integration/operator/service.go:88-92`), so the gate can never be
   more permissive than the service behind it.
2. Default-deny in `transportGateRolesSatisfied`: an unmapped root field, an
   operation with no resolvable roots, and an unknown operation type all refuse.
   Aliases, fragments, and inline fragments resolve through the existing
   `rootFieldNames` walk, so they cannot smuggle a field past the map.

   Two meta-field decisions are load-bearing and were made deliberately rather
   than by omission. `__schema` and `__type` are left unmapped and therefore
   refused; introspection stays behind the compatibility grant, which
   short-circuits earlier. `__typename` is skipped when it rides along with real
   root fields, but **cannot carry an operation on its own** — the function
   requires at least one authorized non-meta field. Without that counter, a bare
   `query { __typename }` would have been admitted for any authenticated caller,
   a small but real widening in a slice whose entire purpose is narrowing. It
   was caught in self-review, not by a test, and now has one.
3. `graphql:operator` retained as a named compatibility grant that
   short-circuits to allow. Every documented operator token is unaffected.
4. `TestTransportGateRoleMapIsExhaustive` compares the map against
   `parsedSchema` — the `*ast.Schema` the server actually executes, not a
   re-parse of the file — and fails in **both** directions: a missing entry is an
   accidental production default-deny, a stale entry is a decision about a field
   that no longer exists.
5. A startup line beside `graphql.NewServer` in `runServe`:
   `GraphQL transport gate: 131 root fields mapped (16 fine-grained, 115 behind
   the "graphql:operator" compatibility grant, which expands to all 131)`.
6. `integration:preview` and the SSE stream-context rule are unchanged. The
   preview branch moved from a negated guard to an early return, which
   `TestTransportGate_PreviewRoleIsUnchanged` pins.
7. `test:transport-gate` (blocking, `-list | rg -x | awk` guard, arity 7) plus
   `make transport-gate` and `make transport-gate-negative-control`.
   `test:operator-control-plane`'s arity is untouched.

**No schema change.** `schema.graphql`, `generated.go`, `model/models_gen.go`,
and `ui/src/lib/gen/graphql.ts` are byte-identical: the Sprint 4 schema lock was
not spent. **No resolver or service-layer change** — defence in depth, not a
relocation.

## Negative control

`make transport-gate-negative-control`. The `transportgateblanket` build tag
compiles `transportGateBlanketAllow() == true`, restoring the pre-Sprint-4
allow-everything gate. **106 refusal subtests fail open**; 0 fail without the
tag. The make target inverts its exit status, so a kill-test that survives the
blanket allow is a CI failure rather than a silent nothing.

## Atomicity (correction 34)

All five documents that hand out `graphql:operator` ship here:
`docs/developer-guide/development-setup.md`,
`docs/operations/INTEGRATION-SESSIONS.md`,
`docs/operations/PRODUCTION-HARDENING.md`, `docs/planning/GRAPHQL-API.md`,
`ui/docs/DEVELOPER-GUIDE.md`.

They document the grant as **deprecated but required**, and each says
explicitly: do not swap `graphql:operator` out of an existing operator token for
the fine-grained roles. That would keep the control plane and lose the entire
IDE, because the session workspace, the legacy catalog, profiles, terminology,
the debugger, and every subscription are still in the compatibility bucket.

**One-line overlap outside this lane's ownership:** `docs/operations/PHI-RETENTION.md:194`
is S4-B's file, but its "Control | Status" row asserted that `graphql:operator`
*remains* a blanket allow — a live claim this slice falsifies. The single row was
corrected rather than left wrong. S4-B can take either version on conflict.

## Next actions — narrow the compatibility bucket

Every bucket entry carries a `TODO` naming its follow-up, so the remaining
ungoverned surface is greppable:

| TODO tag | Fields | Blocked on |
|---|---:|---|
| `S5-session-workspace-roles` | 21 + 7 subscriptions | A session role at the service layer; session authorization exists but is not expressed as a transport role |
| `S5-legacy-catalog-roles` | 30 | A clinical-read role and an authoring role. The legacy workflow catalog may be retired rather than governed (`.loom/31:95`) |
| `S5-terminology-governance-roles` | 20 | `.loom/27` already defers role expectations here |
| `S5-llm-roles` | 9 | Cost and PHI exposure argue for a dedicated grant |
| `S5-profile-roles` | 7 | `.loom/29` |
| `S5-submit-roles` | 4 | Whether GraphQL submit should exist at all, given the durable ingress |

The ordering constraint is the same in every row: **this gate can only enforce
roles that already exist.** Each follow-up has to invent and ship its role at
the service layer first, then add it here — doing it in the other order would
put a role in the transport gate that nothing below it honours, which is the
inverse of the layering this slice preserves.

Two smaller follow-ups:

- `health` is the one field where the bucket is arguably wrong. It is reachable
  by the least-privileged role in the system through the preview path but not by
  `integration.operator`. Fixing it needs OR-of-AND requirements in the map,
  which nothing else needs yet.
- `TestTransportGateRoleMapShape` pins 131/16/115. Those numbers are the lane's
  public claim about how much of the surface is still ungoverned; a follow-up
  that narrows part of the bucket must move them deliberately.

## Sources

- [S1] `.loom/32-sprint4-execution-specs.md:96-100,144-150,161-163,450-469`
- [S2] `.loom/iteration-plan-phase4-transport-gate-roles.md`
- [S3] `internal/integration/operator/service.go:71-92,108-299`
- [S4] `internal/integration/session/types.go:179-184,247-248`
- [S5] `internal/api/graphql/operation_authorization.go`,
  `internal/api/graphql/operation_authorization_roles.go`
