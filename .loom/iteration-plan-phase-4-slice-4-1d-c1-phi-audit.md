# Iteration Plan — Phase 4, Slice 4.1d C1: Audit Immutability and Export Attribution

**Status**: In progress (opened 2026-08-08)
**Lane**: S3-C1 (`.loom/31-sprint3-execution-specs.md`, "Lane S3-C — Slice 4.1d")
**Branch**: `feat/phase4-slice-4-1d-phi-audit`
**Base**: `origin/main` @ `7111cca1` (merge `feat/phase4-operator-control-plane`, i.e. 4.2a)

## The C1/C2 split, and why it is not the slice as written

Slice 4.1d in `.loom/30-implementation-plan-integration-engine-ide-completion.md`
bundles "immutable audit storage, retention/TTL/encryption, and export controls".
Read against the code, those three are not one slice, and one of them has no
subject:

| Piece | Actual state | Size |
|---|---|---|
| Immutable audit storage | 60% shipped — six tables carry `BEFORE UPDATE OR DELETE` triggers, six durable-runtime tables do not | Small |
| Export controls | 80% shipped but **unattributed** — `integration_session_exports` has no principal and no reason | Small, highest value per line |
| Retention/TTL/encryption enforcement | **No subject.** Production refuses every non-ephemeral raw retention mode, so there is no retained raw PHI to expire. The PHI that *is* retained forever carries no policy field at all | Large |

- **S3-C1 (this slice)**: audit immutability, export attribution, a raw-export
  grant, and a truthful retention-posture document.
- **S3-C2 (next sprint)**: design a retention policy for the data that actually
  persists — canonical event payloads, session sample ciphertext, session export
  snapshots — add the TTL columns, and build a durable purge component.

Splitting is not a scope reduction. Enforcing a TTL against
`RawRetentionPolicy` would be enforcing a policy over an empty set: the tests
would pass, the plan text would be satisfied, and every byte of really-retained
PHI would be untouched.

## Day-1 gate (run before any migration was written)

The spec names one riskiest assumption:

> "Retention/TTL/encryption fields exist in the revision contract, so 4.1d is
> about enforcing them."

Two assertions kill it. Both were run first, on PostgreSQL 16, as
`TestPhiRetentionPosture_ProductionRejectsRetainedRawAndCanonicalEventsCarryNoPolicy`
(`internal/integration/processor/phi_retention_posture_integration_test.go`),
which is kept as a permanent regression guard for `docs/operations/PHI-RETENTION.md`.

**Assertion (a) — PASSED.** A production submission whose revision declares
`raw_retention.mode = "encrypted"` with a positive TTL, a purpose, a storage
revision, an encryption-key `SecretReference`, an authorizing principal, and
`access_audit_required = true` is **rejected** with `ErrUnsupportedRawRetention`
(`internal/integration/processor/postgres_submission.go:175-177`). The policy is
asserted contract-valid first (`RawRetentionPolicy.Validate()`,
`pkg/integration/revision.go:128-157`), so the rejection proves production
posture and not contract validation. Every durable record class is unchanged
after the rejection.

```
phi_retention_posture_integration_test.go:85: assertion (a) PASSED:
  encrypted raw retention rejected with production raw retention policy is unsupported
```

**Assertion (b) — PASSED.** Immediately after a successful ephemeral submission,
`integration_canonical_events` holds the clinical payload, classified `phi`, and
`information_schema.columns` for that table contains **zero** columns matching
`%ttl%`, `%expire%`, `%retention%`, or `%purge%`.

```
phi_retention_posture_integration_test.go:119: assertion (b) PASSED:
  1 PHI-classified canonical event row(s), 1040 payload bytes,
  zero ttl/expires/retention/purge columns
```

Both assertions agree with `.loom/31`. **No correction to the spec is required**;
the C1/C2 split stands as written.

## Scope

### In

1. **Audit immutability.** Blanket `BEFORE UPDATE OR DELETE` guards on the
   append-only durable-runtime tables: `integration_delivery_audit`,
   `integration_delivery_operations`, `integration_message_lineage`,
   `integration_canonical_events`, and `integration_batch_audit`.
   Column-scoped `BEFORE UPDATE` guards on the two **state** tables the runtime
   must keep mutating — `integration_receipts` and
   `integration_delivery_attempts` — raising only when an identity, provenance,
   or attribution column changes. Deletion is blocked on both.
2. **Export attribution.** `principal_json`, `reason`, and `include_raw_payload`
   become `NOT NULL` columns on `integration_session_exports`. The verified
   caller identity from `requestsecurity.SecurityContextFromContext` is threaded
   into the store's export entry point. An export with an empty or oversized
   reason is refused before any bundle is assembled and before any row is written.
3. **Raw-export gate.** `includeRawPayload: true` requires the new dotted grant
   `integration.phi.export`, checked at the session-service layer in the shape
   4.2a established in `operator.Service.authorize`. The decision is recorded on
   the export row.
4. **Schema.** One change: `reason: String!` on `ExportIntegrationBundleInput`.
   Regenerated only through `make lint-gqlgen` plus `npm run codegen:check`.
5. **Docs.** `docs/operations/PHI-RETENTION.md`, plus a correction to the plan's
   4.1 bullet so it stops claiming retention enforcement that does not exist.

### Out (S3-C2 and later)

- TTL columns and a purge component for canonical events, session samples, and
  export snapshots.
- Lifting `ErrUnsupportedRawRetention`. Encrypted production raw retention stays
  unimplemented and fail-closed; this slice documents that, it does not change it.
- A second redaction policy. 4.2a's `internal/integration/operator/payload.go`
  remains the operator-facing rendering policy.
- Narrowing the `graphql:operator` blanket transport role (correction 20). Filed
  against 4.2's follow-up, not fixed here.

## Blanket versus column-scoped guards

A blanket guard on a table the runtime mutates would break the shipped delivery
state machine. The distinction is drawn from the actual `UPDATE` statements in
`internal/integration/delivery/store.go`, which touch only `status`,
`completed_at`, `attempt_count`, `scheduled_at`, `last_error_code`, and
`last_error_detail`:

| Table | Guard | Why |
|---|---|---|
| `integration_delivery_audit` | blanket | append-only ledger, insert-only in code |
| `integration_delivery_operations` | blanket | replay/resubmit ledger, insert-only |
| `integration_message_lineage` | blanket | provenance, insert-only |
| `integration_canonical_events` | blanket | the retained PHI record, insert-only |
| `integration_batch_audit` | blanket | batch ledger, insert-only |
| `integration_receipts` | column-scoped UPDATE + blanket DELETE | admission provenance; no production `UPDATE` exists today, but the table is a state table by design |
| `integration_delivery_attempts` | column-scoped UPDATE + blanket DELETE | the runtime legitimately advances `status`/`attempt_count`/`scheduled_at`/error columns |

Test harnesses drop whole schemas (`DROP SCHEMA … CASCADE`) rather than deleting
rows, so DDL teardown is unaffected by row-level triggers.

## Kill-Test

`TestPhiAudit_PostgresImmutableRecordsAndAttributedExport`
(`internal/integration/session`), on PostgreSQL 16, with the spec's five
assertions and a **negative control**: the same job provisions a second database
on the pre-migration schema with the pre-change export path and proves the
`UPDATE`/`DELETE` succeed there and exports are written unattributed. If the
pre-migration database also raises, the test is asserting against the wrong
schema and the proof is vacuous.

CI job `test:phi-audit` follows the isolated-proof pattern from
`test:mllp-runtime` (`.gitlab-ci.yml:791-823`): a `-list | rg -x | awk` existence
step, a dedicated database, a `make phi-audit` target, `allow_failure: false`.

## Gates

`gofmt`, `golangci-lint run`, `go vet ./...`, focused `-race`, full
`go test -race ./...`, `make lint-gqlgen` clean after the regenerated artifacts
are committed, `cd ui && npm run codegen:check`, and — because the immutability
triggers must not over-lock — `make integration-session` and
`make delivery-reliability` unchanged.

## Sources

- [S1] `.loom/31-sprint3-execution-specs.md` — Lane S3-C, corrections 21-25
- [S2] `internal/integration/processor/postgres_submission.go:38-39,175-177`
- [S3] `internal/integration/processor/migrations/0001_atomic_submission.sql`
- [S4] `internal/integration/session/migrations/0001_session_workspace.sql:82-95`
- [S5] `internal/integration/lifecycle/migrations/0001_deployment_lifecycle.sql:87-105`
- [S6] `internal/integration/operator/service.go:71-104` — the authorize shape reused here
- [S7] Command: `go test -tags=integration -run '^TestPhiRetentionPosture_' ./internal/integration/processor/`
