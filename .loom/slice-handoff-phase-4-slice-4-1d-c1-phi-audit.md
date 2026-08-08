# Slice Handoff — Phase 4, Slice 4.1d C1: Audit Immutability and Export Attribution

**Status**: Complete
**Lane**: S3-C1 (`.loom/31-sprint3-execution-specs.md`, "Lane S3-C")
**Branch**: `feat/phase4-slice-4-1d-phi-audit`
**Merge request**: `!139`
**Base**: `origin/main` @ `7111cca1` (4.2a merged)
**Date**: 2026-08-08

## What shipped

### 1. Schema-level audit immutability

Before this slice, six tables carried append-only triggers (four lifecycle, two
session) and the six durable-runtime tables were append-only **by code convention
only** — the schema permitted `UPDATE` and `DELETE` from the application role.

Two guard shapes, chosen from the actual `UPDATE` statements in
`internal/integration/delivery/store.go`:

| Table | Guard | Migration |
|---|---|---|
| `integration_canonical_events` | blanket `BEFORE UPDATE OR DELETE` | processor `0004` |
| `integration_message_lineage` | blanket | processor `0004` |
| `integration_delivery_audit` | blanket | processor `0004` |
| `integration_delivery_operations` | blanket | processor `0004` |
| `integration_batch_audit` | blanket | batch `0003` |
| `integration_session_exports` | blanket | session `0004` |
| `integration_receipts` | column-scoped `BEFORE UPDATE` + blanket `BEFORE DELETE` | processor `0004` |
| `integration_delivery_attempts` | column-scoped `BEFORE UPDATE` + blanket `BEFORE DELETE` | processor `0004` |

The column-scoped guards freeze identity, provenance, lineage, destination
binding, and attribution while leaving `status`, `attempt_count`,
`scheduled_at`, `completed_at`, `last_error_code`, and `last_error_detail`
writable. Blanket-guarding those two would have broken the shipped Slice 2.3
delivery state machine.

### 2. Export attribution

`integration_session_exports` gains `principal_json`, `reason`, and
`include_raw_payload`, all `NOT NULL`, with
`CHECK (octet_length(reason) BETWEEN 1 AND 1024)` matching the delivery-operations
convention. The verified caller identity is threaded from
`requestsecurity.SecurityContextFromContext` through a new
`session.ExportRequest` into both store implementations. `reason: String!` was
added to `ExportIntegrationBundleInput` — the sprint's only schema change.

Validation runs at the top of `ExportBundle`, before any session read, so a
refused export assembles no bundle and writes no row.

Pre-migration rows are marked with an explicit `unattributed_legacy_export`
sentinel rather than backfilled with a synthesized actor — backfilling would be
retroactively vouching for a disclosure nobody recorded (4.1b3's idiom).

### 3. Raw-export grant

`includeRawPayload: true` requires the new dotted grant
`integration.phi.export` (`session.PHIExportRole`), enforced twice:

- at the resolver, so the refusal happens before the store is touched
  (`operator.Service.authorize` shape, per 4.2a); and
- as a domain rule in `ExportRequest.Validate()`, reading roles off the
  **verified** principal, so no caller path can bypass it.

The refusal names the decision and the required grant, never whether the session
or its raw payloads exist.

No action was added to `internal/integration/authorization/policy.go` — that file
belongs to lane S3-B this sprint.

### 4. Truthful retention posture

`docs/operations/PHI-RETENTION.md` states, with `file:line` citations, what is
retained and what is not implemented, including an explicit "NOT implemented"
table. The 4.1 bullet in
`.loom/30-implementation-plan-integration-engine-ide-completion.md` was corrected
to stop claiming retention/TTL/encryption enforcement, with the reasoning and
citations inline.

## Day-1 gate results

Both riskiest-assumption assertions ran before any migration was written, and
both **PASSED**, confirming the spec's C1/C2 split. `.loom/31` needed no
correction. Kept as a permanent regression guard,
`TestPhiRetentionPosture_ProductionRejectsRetainedRawAndCanonicalEventsCarryNoPolicy`:

```
assertion (a) PASSED: encrypted raw retention rejected with
  production raw retention policy is unsupported
assertion (b) PASSED: 1 PHI-classified canonical event row(s), 1040 payload bytes,
  zero ttl/expires/retention/purge columns
```

Assertion (a) asserts the retention policy is contract-valid *first*, so the
rejection proves production posture rather than contract validation.

## Kill-test and negative control

`TestPhiAudit_PostgresImmutableRecordsAndAttributedExport`, PostgreSQL 16,
`-race`, 8 subtests, all green. The negative control provisions a second,
independently migrated pre-migration schema and proves the assertions flip:

```
negative control: pre-migration schema accepted every UPDATE/DELETE; counts moved
  from {Receipts:4 Events:3 Lineage:1 Attempts:2 Audit:1 Traces:trace-1,trace-orphan}
  to   {Receipts:3 Events:2 Lineage:0 Attempts:1 Audit:0 Traces:forged-trace}
negative control: pre-migration export written with no principal and no reason
delivery state machine advanced claim -> retry -> DLQ -> replay -> published
  with the column-scoped guards active
every guarded mutation raised; counts unchanged
```

**The negative control caught two real defects in the proof itself.** Both are
now structurally impossible, and both are worth remembering:

1. **Referential integrity is not immutability.** `DELETE` aimed at rows that
   still had dependents was rejected on the *pre-migration* schema too — by the
   existing `ON DELETE RESTRICT` foreign keys, not by anything this slice added.
   A test that only ever deletes rows with dependents proves the FKs work. The
   test now seeds three independent dependent-free orphan chains and aims its
   deletes there. The last row of any chain was otherwise freely deletable.
2. **An `UPDATE` matching zero rows succeeds.** The first version asserted on
   `integration_delivery_audit` before anything had written to it, so the
   assertion passed vacuously. The delivery state machine now runs *first*
   (which both populates the ledger and proves the guards did not over-lock),
   and `assertGuardedTablesPopulated` refuses to run the mutation assertions
   against any empty table.

## Verification

| Gate | Result |
|---|---|
| `gofmt` | clean |
| `golangci-lint run` | 0 issues |
| `go vet ./...`, `go vet -tags=integration ./...` | clean |
| `go test -race ./...` | green |
| `make lint-gqlgen` (post-commit) | up to date |
| `cd ui && npm run codegen:check` | clean |
| `make phi-audit` | green |
| `make integration-session` | green — no over-lock |
| `make delivery-reliability` (Postgres + Kafka) | green — no over-lock |
| `make operator-control-plane` | green |
| `TestPostgresProductionSubmission_64WayDuplicateFaultRestart` | green |
| CI `test:phi-audit` | green |

CI note: `lint:ui` failed once with a Node heap OOM during the Vite build and
passed on retry. Unrelated to this slice — the only `ui/` change is 10 lines of
regenerated type and JSDoc in `ui/src/lib/gen/graphql.ts`.

## Coordination notes for siblings

- **This lane owned `schema.graphql` and every regenerated artifact this
  sprint.** 4.2b must rebase onto the regenerated `ui/src/lib/gen/graphql.ts`;
  that is the only `ui/` file this lane touched.
- Migration numbers consumed: processor `0004_audit_immutability.sql`, session
  `0004_export_attribution.sql`, batch `0003_batch_audit_immutability.sql`. Next
  free: processor `0005`, session `0005`, batch `0004`.
- Untouched: `internal/integration/delivery/store.go` (4.2a),
  `internal/integration/authorization/policy.go` and `dispatcher.go` (S3-B), the
  serve component table, deploy manifests, and smoke/check scripts (S3-A).
- `.gitlab-ci.yml` change is one appended job at the end of the `test` stage;
  no existing job's `services:` block was modified and no existing job was
  promoted to blocking.

## Next Actions

### S3-C2 — retention design and purge runtime (next sprint)

This is the honest remainder of 4.1d, and it is a design slice before it is an
implementation slice.

1. **Design a retention policy for the data that actually persists.** The
   `RawRetentionPolicy` contract governs production raw bytes, which are never
   retained. The PHI retained indefinitely is:
   - `integration_canonical_events.payload_json` — PHI-classified, no policy field
   - `integration_session_samples.raw_cipher` — AES-256-GCM ciphertext, no TTL
   - `integration_session_samples.record_json` — the redacted form, no TTL
   - `integration_session_exports.record_json` — a snapshot of all of the above
   Decide whether the policy is per-tenant, per-integration-revision, or
   deployment-wide, and record it in `.loom/40-decisions.md`.
2. **Add the expiry columns.** Note that
   `TestPhiRetentionPosture_...` asserts `information_schema.columns` shows
   **zero** `%ttl%`/`%expire%`/`%retention%`/`%purge%` columns on
   `integration_canonical_events`. That gate must be rewritten in the same MR
   that adds them, and `docs/operations/PHI-RETENTION.md` sections 2, 3, and 6
   must be updated with it. This is deliberate: the docs cannot silently rot.
3. **Build the durable purge component.** It must be lease-fenced and
   multi-replica safe like the delivery and batch stores, and it must reconcile
   with this slice's immutability triggers — a purge is a `DELETE` on
   `integration_canonical_events`, which now raises. Decide explicitly whether
   purge runs as a privileged role that bypasses the trigger, whether the
   trigger grows a session-variable escape hatch, or whether purge writes a
   tombstone instead of deleting. **Do not weaken the trigger without recording
   the decision** — the whole point of C1 is that the schema, not convention,
   is the guarantee.
4. **Decide whether to lift `ErrUnsupportedRawRetention`.** If encrypted
   production raw retention is ever implemented it needs the storage revision
   resolver, the `SecretReference` key resolver (which S3-B's 4.1c-a builds),
   and the access-audit path the contract already requires. Until all three
   exist, keep it fail-closed.

### Filed, not fixed

- **Correction 20** (`.loom/31-sprint3-execution-specs.md`): `graphql:operator`
  remains a blanket allow at the GraphQL transport gate
  (`internal/api/graphql/operation_authorization.go:50-52`). Every fine-grained
  decision this program has added — `integration.operator`,
  `integration.deployment.operator`, `integration.delivery.operator`, and now
  `integration.phi.export` — sits one layer deeper. That layering is defensible
  but the transport gate is still binary. Belongs to 4.2's follow-up.

## Sources

- [S1] `.loom/31-sprint3-execution-specs.md` — Lane S3-C, corrections 21-25
- [S2] `.loom/iteration-plan-phase-4-slice-4-1d-c1-phi-audit.md`
- [S3] `internal/integration/session/phi_audit_integration_test.go` — kill-test
- [S4] `internal/integration/processor/phi_retention_posture_integration_test.go` — day-1 gate
- [S5] `docs/operations/PHI-RETENTION.md`
- [S6] Commands: `make phi-audit`, `make integration-session`,
  `make delivery-reliability`, `make operator-control-plane`, `make lint-gqlgen`,
  `cd ui && npm run codegen:check`
