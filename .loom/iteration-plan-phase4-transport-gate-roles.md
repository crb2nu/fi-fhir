# RALPH Iteration Plan — Lane S4-E: Transport-Gate Role Narrowing

**Created**: 2026-08-08
**Program**: `.loom/32-sprint4-execution-specs.md` Lane S4-E (corrections 33-34)
**Branch**: `feat/phase4-transport-gate-roles`
**Merge order**: last, after S4-C → S4-A → S4-B

## Review

- The lane closes `.loom/31-sprint3-execution-specs.md` correction 20, re-filed
  by S3-C1 (`.loom/slice-handoff-phase-4-slice-4-1d-c1-phi-audit.md:202-206`,
  `docs/operations/PHI-RETENTION.md:194`).
- `operationAuthorization.MutateOperationContext` returns `nil` — allow
  everything — for any caller holding `graphql:operator`
  (`internal/api/graphql/operation_authorization.go:50-52`). Three slices have
  shipped fine-grained roles one layer beneath that binary gate:
  `integration.delivery.operator` (2.3, `internal/integration/delivery/types.go:17`),
  `integration.operator` / `integration.deployment.operator`
  (4.2a, `internal/integration/operator/types.go:17,19`), and
  `integration.phi.export` (S3-C1, `internal/integration/session/types.go:184`).
- The schema is **frozen** for Sprint 4. This lane needs no root field, so
  `schema.graphql`, `generated.go`, `model/models_gen.go`, and
  `ui/src/lib/gen/graphql.ts` are untouched (correction 33).
- Correction 34: `ui/src/**` has zero occurrences of `graphql:operator`, but
  five documents hand it out as the only operator role. The mapping, the
  fixtures, and all five docs ship in one MR.

## Day-1 Gate — result: PASS

`TestTransportGate_LeastPrivilegeIsRefusedToday`, run at `55412bdaa` with **zero
modifications to tracked files** (`git diff --stat HEAD` empty; the only
untracked path is the new test file).

A real 4.1a OIDC token carrying `integration.operator` +
`integration.deployment.operator` and **not** `graphql:operator` is refused with
`FORBIDDEN` / "GraphQL operation forbidden" for all sixteen 4.2a control-plane
operations — nine `operator*` queries, three delivery-recovery mutations, four
deployment commands. 16/16 subtests pass.

The gate's premise holds: the fine-grained roles the merged 4.2b UI depends on
are decorative at the transport gate today, and only the blanket role opens it.
No correction to `.loom/32` is required.

## Riskiest Assumption — DISCONFIRMED, lane re-scoped

`.loom/32:469` states the assumption as *"the fine-grained roles 2.3, 4.2a, and
S3-C1 shipped are already sufficient to express every operation"*, and names the
re-scope trigger: *"if more than a handful of fields land in the
explicit-compatibility bucket … re-scope to the integration surface only,
leaving the legacy catalog behind the compatibility grant with a named
follow-up."*

Enumeration of the 131 root fields against the shipped roles:

| Requirement | Fields |
|---|---:|
| `integration.operator` | 9 |
| `integration.operator` + `integration.delivery.operator` | 3 |
| `integration.operator` + `integration.deployment.operator` | 4 |
| **Fine-grained subtotal** | **16** |
| `graphql:operator` (explicit compatibility bucket) | 115 |
| **Total** | **131** |

115 is not a handful. **The lane re-scopes to the 4.2a integration control
plane.** The legacy catalog — the event/patient browser, the workflow
definition/version/approval catalog, FHIR subscriptions, integration sessions,
profiles, LLM/copilot, terminology mappings and autoroutes, Temporal, the
debugger, and all seven subscriptions — keeps an **explicit**
`graphql:operator` entry plus a `TODO` naming its follow-up slice. Structure,
default-deny, and the exhaustiveness test still cover all 131 fields, so the
bucket is visible and enumerable rather than implicit.

Two mappings were considered and rejected on evidence:

- **`integration.phi.export` is not a field-level role.** It gates the
  `includeRawPayload` *argument* of `exportIntegrationBundle`
  (`internal/api/graphql/resolvers/integration_session_service.go:499`,
  `internal/integration/session/types.go:247`), not the field. Mapping the
  field to it would both widen access and contradict the acceptance criterion
  that `integration.phi.export` alone reaches nothing at the transport gate. It
  therefore appears in no mapping.
- **The `integration:submit` / `integration:mllp` / `integration:batch` grants
  are transport-minted, not token-minted** (`internal/integration/ingress/auth.go:144`,
  `internal/integration/mllp/service.go:136`, `internal/integration/batch/identity.go:49`).
  Mapping `submitMessage` / `submitEvent` / `submitBatch` to them would grant
  ingress principals a GraphQL surface they do not have today. Out of scope.

`health` also stays in the compatibility bucket. It is already reachable by the
least-privileged role in the system through the untouched preview path, and
opening it to any authenticated caller would break the existing
"unprivileged role stops before resolver" case
(`internal/api/graphql/oidc_security_test.go:52`).

## Align

- Slice name: **S4-E — per-root-field transport-gate roles with a named
  compatibility grant**.

- Scope in:
  1. A compile-time `map[ast.Operation]map[string][]string` over all 131 root
     fields. The value is an AND-set, mirroring `operator.Service.authorize`,
     which requires every listed role
     (`internal/integration/operator/service.go:71,88-92`).
  2. Default-deny for any root field with no entry.
  3. `graphql:operator` retained as a named compatibility grant that
     short-circuits to allow, so existing tokens behave exactly as today.
  4. An exhaustiveness test that parses `schema.graphql` and fails when a root
     field has no mapping — a new field cannot ship without a role.
  5. A startup line naming the compatibility grant, next to the GraphQL server
     construction in `runServe`.
  6. The kill-test, the day-1 gate retained as a regression guard, the
     build-flag negative control, and the two `graphql:operator` test fixtures.
  7. All five docs that hand out the blanket role.
  8. `test:transport-gate` in CI with the `-list | rg -x | awk` existence guard,
     and a `make` target.

- Scope out:
  - Any schema change. The schema lock is not spent.
  - Any resolver-layer or service-layer change. `operator.Service.authorize`,
    session authorization, and the PHI-export argument check stay exactly as
    they are: this is defence in depth, not a relocation.
  - Any new role. Only the four already-shipped roles are mapped.
  - Narrowing the 115-field compatibility bucket. That is the named follow-up.
  - Changing `test:operator-control-plane`'s `-list` arity.

- **Docs framing (load-bearing).** The docs must not tell operators to *replace*
  `graphql:operator` with the fine-grained roles — the whole IDE surface is in
  the compatibility bucket, so that would break every deployment. They document
  that the blanket role still grants the full surface and is now explicitly a
  deprecated compatibility grant, and that a control-plane-only operator can now
  be issued the narrow set instead.

## Kill-Test and negative control

- `TestTransportGate_FineGrainedRolesReplaceBlanketOperator` — real handler,
  real 4.1a OIDC tokens, one case per role combination in the acceptance
  criteria, plus exhaustiveness against the parsed schema.
- `TestTransportGate_LeastPrivilegeIsRefusedToday` — the day-1 gate, retained as
  a regression guard on the unmapped-and-ungranted case.
- Negative control: build tag `transportgateblanket` swaps
  `transportGateBlanketAllow()` to `true`, restoring the pre-Sprint-4 blanket
  allow. Every least-privilege refusal must then fail open.

## Verification

- `gofmt`, `golangci-lint run`, `go vet ./...`.
- Focused: `go test -race ./internal/api/graphql/... ./cmd/fi-fhir/...`.
- Full: `go test -race ./...`.
- `go test -tags transportgateblanket -run TestTransportGate ./internal/api/graphql/`
  must FAIL, and fail on the least-privilege cases specifically.
- `make lint-gqlgen` must show `generated.go` untouched.

## File ownership (recorded before first commit)

Owned by this lane:

- `internal/api/graphql/operation_authorization.go`
- `internal/api/graphql/operation_authorization_roles.go` (new)
- `internal/api/graphql/transport_gate_narrowed.go` (new)
- `internal/api/graphql/transport_gate_blanket.go` (new)
- `internal/api/graphql/transport_gate_roles_test.go` (new)
- `internal/api/graphql/server_security_test.go` (fixture)
- `internal/api/graphql/resolvers/integration_session_test.go` (fixture)
- `docs/developer-guide/development-setup.md`
- `docs/operations/INTEGRATION-SESSIONS.md`
- `docs/operations/PRODUCTION-HARDENING.md` (the `graphql:operator` paragraph
  only; S4-C owns the rest of the file)
- `docs/planning/GRAPHQL-API.md`
- `ui/docs/DEVELOPER-GUIDE.md`
- `.loom/iteration-plan-phase4-transport-gate-roles.md`,
  `.loom/worklog/2026-08-08-lane-s4-e-transport-gate-role-narrowing.md`

Append-only, shared: `.gitlab-ci.yml` (new `test:transport-gate`), `Makefile`
(new `test-transport-gate`), `cmd/fi-fhir/main.go` (one startup line beside
`graphql.NewServer`, away from the `:5272` autoroute block S4-B appends to).

No migration number is claimed: this lane adds no migration.

## Sources

- [S1] `.loom/32-sprint4-execution-specs.md:96-100,144-150,161-163,450-469`
- [S2] `internal/api/graphql/operation_authorization.go:14-59,70-117`
- [S3] `internal/integration/operator/service.go:71-92,108-299`
- [S4] `internal/integration/operator/types.go:15-19`,
  `internal/integration/delivery/types.go:16-17`,
  `internal/integration/session/types.go:179-184,247-248`
- [S5] `internal/api/graphql/schema.graphql` — 64 Query / 60 Mutation /
  7 Subscription root fields
