# PHI Retention Posture

**Status**: current as of Slice 4.1d C1 (2026-08-08)
**Audience**: platform operators, privacy officers, security reviewers
**Scope**: what protected health information the integration engine persists, for
how long, under what protection, and what is *not* implemented.

This document states the posture the code actually implements. Every claim below
carries a `file:line` citation. Where a control does not exist, this document
says so rather than describing an intended design.

Two automated gates keep it honest, both required in CI as `test:phi-audit`:

| Gate | Proves |
|---|---|
| `TestPhiRetentionPosture_ProductionRejectsRetainedRawAndCanonicalEventsCarryNoPolicy` (`internal/integration/processor/phi_retention_posture_integration_test.go`) | Sections 1 and 2 below |
| `TestPhiAudit_PostgresImmutableRecordsAndAttributedExport` (`internal/integration/session/phi_audit_integration_test.go`) | Sections 4 and 5 below |

---

## 1. Production raw message bytes are ephemeral, and every alternative is refused

The revision contract carries a full raw-retention policy — mode, TTL, purpose,
storage revision, encryption key reference, authorizing principal, and an
access-audit flag — with deny-by-default semantics and cross-field validation
(`pkg/integration/revision.go:109-157`). Its zero value means ephemeral
(`pkg/integration/revision.go:120-126`).

**The durable committer accepts only `ephemeral`.** Any revision whose effective
mode is anything else is rejected before a single row is written:

```go
if revision.Policy.RawRetention.EffectiveMode() != integration.RawRetentionModeEphemeral {
    return integration.ProcessResult{}, ErrUnsupportedRawRetention
}
```

`internal/integration/processor/postgres_submission.go:179-181`, with the error
declared at `:38-39` as "keeps production fail-closed until encrypted raw storage
exists".

**Operational consequence:** there is no retained production raw PHI. A retention
TTL over production raw bytes would be a policy over an empty set. `encrypted`
mode is *declarable* in an artifact and *unimplemented* in the runtime; a
deployment that declares it fails closed at submission rather than silently
degrading to plaintext storage.

---

## 2. Canonical event payloads are PHI, retained indefinitely, with no policy field

What production *does* persist is the canonical clinical event:

- Written on every successful admission into `integration_canonical_events`
  (`internal/integration/processor/postgres_submission.go:262-278`), into a table
  whose `classification` column is constrained to exactly `'phi'`
  (`internal/integration/processor/migrations/0001_atomic_submission.sql:26`).
- The ADT A01 projector sets that classification
  (`internal/integration/processor/adt_a01.go:209`).
- The table has **no** TTL, expiry, retention, or purge column. The only
  time-bounded columns anywhere in the durable migration set are *lease* columns.

**This is the gap.** The PHI the platform retains forever is the PHI that carries
no retention policy at all. Designing that policy, adding the expiry columns, and
building a durable purge component is **Slice S3-C2**, tracked in
`.loom/31-sprint3-execution-specs.md` ("Lane S3-C", correction 23). Until it
ships, canonical event payload retention is governed by database backup and
deletion policy operated outside this codebase, not by the application.

Assertion (b) of the retention-posture gate queries `information_schema.columns`
for `%ttl%`, `%expire%`, `%retention%`, and `%purge%` on that table and fails if
any appears — so if S3-C2 lands, this section must be rewritten before CI passes.

---

## 3. Session workspace samples: redacted by default, encrypted when retained, never expired

The Integration Session workspace is the design-time surface, and it holds sample
messages supplied by integration engineers.

| Policy | Storage | Citation |
|---|---|---|
| `redact` (default) | The raw is redacted in place before storage; only the redacted form is persisted in `record_json` | `internal/integration/session/postgres.go:303-305` |
| `retain` (explicit) | The raw is encrypted with AES-256-GCM and stored in `integration_session_samples.raw_cipher`; the plaintext is not stored | `internal/integration/session/postgres.go:306-317`, `internal/integration/session/protector.go:35-51` |

The encryption is real: a random 96-bit nonce per record, a version byte, and
session/sample-scoped additional authenticated data
(`internal/integration/session/protector.go:43-50`).

**There is no TTL and no purge job for either form.** A retained sample's
ciphertext persists until the row is deleted by an operator. This is the second
half of the S3-C2 gap.

---

## 4. Session exports are attributed disclosures

Before Slice 4.1d C1, `integration_session_exports` carried only
`(tenant_id, session_id, export_id, exported_at, record_json)` — an export
snapshot of everything in section 3, with no record of who took it or why. That
contradicted the product spec's requirement that data export record actor,
reason, timestamp, and revision
(`.loom/20-product-spec-integration-engine-ide-completion.md:220-222`).

It now records all three, `NOT NULL`
(`internal/integration/session/migrations/0004_export_attribution.sql:21-38`):

| Column | Meaning |
|---|---|
| `principal_json` | The **verified** caller identity, read from the request security context — never client-supplied |
| `reason` | Operator-supplied disclosure reason, `CHECK (octet_length(reason) BETWEEN 1 AND 1024)` |
| `include_raw_payload` | Whether raw sample payloads were included in this disclosure |

Enforcement:

- `reason` is required by the GraphQL schema (`reason: String!` on
  `ExportIntegrationBundleInput`, `internal/api/graphql/schema.graphql`), and an
  empty or oversized reason is refused by `ExportRequest.Validate()`
  (`internal/integration/session/types.go`) **before** any session is read, so a
  refused export assembles no bundle and writes no row.
- The verified identity is threaded from
  `requestsecurity.SecurityContextFromContext`
  (`internal/api/graphql/resolvers/integration_session_service.go`). An
  unauthenticated caller cannot export.
- Export records are append-only: a `BEFORE UPDATE OR DELETE` trigger rejects any
  attempt to rewrite or remove the evidence of a disclosure
  (`internal/integration/session/migrations/0004_export_attribution.sql:46-59`).

Rows written before this migration are **not** backfilled with a synthesized
actor — that would be retroactively vouching for a disclosure nobody recorded.
They carry an explicit `unattributed_legacy_export` sentinel so they stay visibly
distinguishable, following the provenance idiom Slice 4.1b3 established.

### Raw payloads need a distinct grant

`includeRawPayload: true` is a materially larger disclosure than the default, so
it requires the dotted grant **`integration.phi.export`**
(`internal/integration/session/types.go`, `PHIExportRole`). Without it the export
is refused, no bundle is assembled, and no row is written. The refusal names the
required decision, never whether the session or its raw payloads exist.

The default remains strip. Note that even with the grant, samples stored under
the `retain` policy have their raw stripped from the bundle unconditionally by
the store (`internal/integration/session/postgres.go:889-893`); the grant governs
the *redacted* raw the GraphQL layer returns.

---

## 5. Durable audit and provenance records are immutable in the schema

Six tables already carried append-only triggers before this slice — four in the
lifecycle catalog
(`internal/integration/lifecycle/migrations/0001_deployment_lifecycle.sql:87-105`)
and two in the session workspace
(`internal/integration/session/migrations/0003_publications.sql:26-47`).

Slice 4.1d C1 closes the durable-runtime gap. Two guard shapes, because the
runtime legitimately mutates some of these tables:

| Table | Guard | Rationale |
|---|---|---|
| `integration_canonical_events` | blanket `BEFORE UPDATE OR DELETE` | insert-only ledger holding the retained PHI |
| `integration_message_lineage` | blanket | insert-only provenance |
| `integration_delivery_audit` | blanket | insert-only delivery ledger |
| `integration_delivery_operations` | blanket | insert-only replay/resubmit/discard ledger |
| `integration_batch_audit` | blanket | insert-only batch ledger (`internal/integration/batch/migrations/0003_batch_audit_immutability.sql`) |
| `integration_receipts` | column-scoped `BEFORE UPDATE` + blanket `BEFORE DELETE` | admission identity, provenance, and attribution frozen; the table is a state table by design |
| `integration_delivery_attempts` | column-scoped `BEFORE UPDATE` + blanket `BEFORE DELETE` | lineage and destination binding frozen, while `status` / `attempt_count` / `scheduled_at` / `completed_at` / error columns stay writable for the delivery state machine |

Both column-scoped guards are in
`internal/integration/processor/migrations/0004_audit_immutability.sql`.

**Referential integrity is not immutability.** The `ON DELETE RESTRICT` foreign
keys on these tables protect only rows that still have dependents; the last row
of a chain, or any row whose dependents were removed first, was previously
deletable. The kill-test therefore aims its `DELETE` assertions at
purpose-seeded dependent-free rows, and its negative control proves those
deletes succeed on the pre-migration schema.

Row-level triggers do not affect DDL, so `DROP SCHEMA … CASCADE` teardown in the
integration suites is unaffected.

---

## 6. What is explicitly NOT implemented

| Control | Status |
|---|---|
| TTL or expiry on canonical event payloads | **Not implemented** — S3-C2 |
| TTL or expiry on session sample ciphertext or redacted records | **Not implemented** — S3-C2 |
| TTL or expiry on session export snapshots | **Not implemented** — S3-C2 |
| Durable purge component | **Not implemented** — S3-C2 |
| Encrypted production raw retention | **Deliberately unimplemented and fail-closed** (section 1). Do not enable it by lifting `ErrUnsupportedRawRetention` without the storage revision, key resolver, and access-audit path the contract requires |
| Narrowed transport-gate roles | Not in this slice. `graphql:operator` remains a blanket allow at the GraphQL transport gate (`internal/api/graphql/operation_authorization.go:50-52`); fine-grained decisions like `integration.phi.export` sit one layer deeper |

## Operational guidance until S3-C2

1. Treat the submission database as a PHI system of record with indefinite
   retention. Backup encryption, access control, and deletion schedules are
   operated at the database and storage layer.
2. Treat session workspaces as PHI-bearing. Retained samples are encrypted at
   rest by the application; redacted samples are not raw but are not guaranteed
   PHI-free for every format.
3. Every `exportIntegrationBundle` call is a logged disclosure. Review
   `integration_session_exports` — `principal_json`, `reason`,
   `include_raw_payload`, `exported_at` — during access reviews.
4. Grant `integration.phi.export` narrowly. It is the only role that unlocks raw
   sample payloads in an export.

## Related

- `.loom/31-sprint3-execution-specs.md` — Lane S3-C, corrections 21-25, and the
  C1/C2 split
- `.loom/iteration-plan-phase-4-slice-4-1d-c1-phi-audit.md` — this slice's plan
  and day-1 gate results
- `docs/operations/README.md` — general operations entry point
