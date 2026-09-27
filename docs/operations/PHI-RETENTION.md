# PHI Retention Posture

**Status**: current as of Slice 4.1e (2026-08-08)
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

One more is required as `test:phi-retention-purge`:

| Gate | Proves |
|---|---|
| `TestPhiRetention_PurgeIsStructurallyBlockedToday` (`internal/integration/retention/purge_gate_integration_test.go`) | That a purge is neither a `DELETE` nor a free redaction — section 2 |
| `TestPhiRetention_PostgresExpiryPurgeAndAuditedTombstone` (`internal/integration/retention/purge_integration_test.go`) | Sections 2, 3, and 6 below |

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

## 2. Canonical event payloads are PHI, retained under a per-tenant policy, purged by tombstone

What production *does* persist is the canonical clinical event:

- Written on every successful admission into `integration_canonical_events`
  (`internal/integration/processor/postgres_submission.go:262-278`), into a table
  whose `classification` column is constrained to exactly `'phi'`
  (`internal/integration/processor/migrations/0001_atomic_submission.sql:26`).
- The ADT A01 projector sets that classification
  (`internal/integration/processor/adt_a01.go:209`).

### Why a purge here is a tombstone and not a deletion

Slice 4.1d C1 put a blanket `BEFORE UPDATE OR DELETE` guard on this table
(`internal/integration/processor/migrations/0004_audit_immutability.sql:29-32`).
A purge is either a `DELETE` of the row or an `UPDATE` replacing the payload.
**C1 blocked both**, and nothing said so until Slice 4.1e's day-1 gate asserted
it. Even with the trigger lifted the row would still be undeletable:
`integration_message_lineage` and `integration_delivery_attempts` reference it
`ON DELETE RESTRICT` (`0001_atomic_submission.sql:52-54,73-75`) and both are
themselves undeletable.

So the purge replaces the payload with a **canonical tombstone** and stamps
`purged_at`. The exemption that permits it is column-scoped and enforced by the
schema, not by a role or a convention
(`internal/integration/processor/migrations/0005_retention_expiry.sql`):

| Operation on `integration_canonical_events` | Result |
|---|---|
| `DELETE` | raises, always |
| `UPDATE` of `tenant_id`, `event_id`, `receipt_id`, `event_type`, `source_message_id`, `correlation_id`, `classification`, `recorded_at` | raises, always |
| `UPDATE` of `purge_after` alone, on an unpurged row | permitted — this is the policy stamp, and it touches no payload |
| `UPDATE` setting `payload_json` to the canonical tombstone **and** `purged_at`, once, on an unpurged row | permitted — this is the purge |
| any other `UPDATE` of `payload_json`, or a second tombstone | raises |

**A tombstone is not a backup-inclusive deletion.** The row, its identity, its
classification, and its `recorded_at` survive on purpose, so an audit can still
show what existed. **A database backup taken before the purge still contains the
payload.** Purge bounds retention in the live database only; expiring backup
copies remains a storage-layer control operated outside this codebase.

### The policy that decides when

Retention lives in `integration_retention_policies`: a mutable, attributed,
versioned, per-tenant record (`0005_retention_expiry.sql`). It is deliberately
**not** part of the integration revision — a revision is immutable and
content-addressed, the retained data outlives it, and a retention change must not
require minting a revision and redeploying — and **not** deployment configuration
alone, which has no audit trail and no per-tenant scope. Every change writes an
append-only row to `integration_retention_policy_audit`. See
`.loom/40-decisions.md` (2026-08-08, "Slice 4.1e").

The deployment supplies the document, loaded the way the destination registry is:

| Variable | Meaning |
|---|---|
| `FI_FHIR_RETENTION_POLICY_PATH` | Path to the retention policy document. **Unset means no purge component, no policy record, and nothing purged.** |
| `FI_FHIR_RETENTION_PURGE_INTERVAL` | Purge cadence, Go duration. Default `1h`. How often a purge tick starts — **not** a bound on throughput; see "Throughput" below. |
| `FI_FHIR_RETENTION_PURGE_BATCH_SIZE` | Records per class per **store pass**. Default `200`. One tick makes as many passes as the backlog needs, within its drain budget. |

An omitted window means **retain indefinitely** for that class. An absent policy
record means the same for every class. Fail-closed is the only safe default for
a control whose failure mode is destroying clinical data.

`purge_after` and `purged_at` are `NULL`-able and the migration **backfills
nothing**. A row admitted before any policy existed has no policy; inventing a
deadline for it in a migration would be retroactively vouching for a retention
decision nobody made, the same reason 4.1b3 and C1 refused to backfill
provenance. Such a row becomes purgeable only once an operator records an
attributed policy, at which point the purge component stamps it under that
operator's authority.

### Throughput: one tick drains the backlog, bounded by a wall-clock budget

Cadence and batch size are **not** independent knobs, and treating them as such
is how this shipped with a hard ceiling.

Until Sprint 5 the purge component called the store exactly once per tick and
then blocked on the ticker. Every purge and stamp statement carries a `LIMIT`
bound to the batch size, so the sustained rate was **batch size × 1 per
interval** — at the shipped defaults, **200 records per class per hour, or
0.056/sec**, on the busiest table in the system. A tenant producing records
faster than that fell permanently behind its own retention policy, and nothing
in the exposition said so. That was found defect D1 in
`.loom/33-sprint5-execution-specs.md`.

One purge tick now **drains**: while a bounded statement comes back full there
is more backlog, so the tick keeps going rather than waiting a whole interval
per batch. This is the shape `internal/integration/session`'s stream relay
already used.

The drain is bounded by a **wall-clock budget per tick**, `5m` by default and
clamped to half the interval, because the purge holds row locks and writes an
audit row per record: a tenant ingesting faster than the purge drains must not
be able to hold a connection for the whole interval. The budget is checked
*between* passes, so a tick always makes at least one, and exhausting it is not
an error — it is reported on the result, logged as a warning naming the
remaining backlog, and visible on the gauge below.

**The documented bound is one tick.** A backlog of 10,000 expired canonical
events clears in a single tick — 51 store passes at the shipped batch size of
200 — and that is asserted at that scale rather than reasoned about
(`make phi-retention-throughput`). A backlog large enough to exhaust the budget
resumes on the next tick; the number of ticks is then the total work divided by
the budget, and the gauge shows the remainder throughout.

Stamping counts toward the drain as well as purging. A canonical event is
unpurgeable until `purge_after` is stamped, and the stamp carries the same
`LIMIT`, so a stamp-bound backlog was exactly as invisible as a purge-bound one.

**One failing class no longer stops the others.** The pass attempts every class
and joins the failures, so a revoked grant or a lock timeout on one class costs
that class one interval rather than costing every class one interval.

### The backlog gauge

`fi_fhir_retention_backlog_records{record_class}` — a **gauge**, one series per
record class, published on every tick including the zeroes.

It counts the records the purge is eligible to act on right now: the same
eligibility each purge statement uses, delivery interlock included, so a gauge
of zero and a purge that acts on nothing are the same statement. It recomputes
each deadline from the record's own timestamp and the policy window in force
rather than reading `purge_after`, so a row that has never been stamped still
counts.

Retention had counters only before Sprint 5 — passes and records purged — and
both climb whether the purge is keeping up or falling a thousand records an hour
behind. A backlog that only ever grows is the difference, and only a gauge can
show it. Alert on it being non-zero and rising across ticks, not on any single
sample: a busy deployment is briefly non-zero between ticks by design.

The zeroes matter. A gauge written only when non-zero goes stale rather than
going to zero, which makes "the backlog cleared" indistinguishable from "the
purge component died".

### What the purge will never touch

An event whose delivery attempt is still `queued`, or still active in the
dead-letter queue, is **never** purged
(`internal/integration/retention/store.go`, `purgeCanonicalEvents`). The delivery
`Claim` join reads `integration_canonical_events.payload_json`
(`internal/integration/delivery/store.go:107-113`); if a tombstone could reach
it, the worker would publish a tombstone to a destination. The interlock is
asserted directly by the kill-test.

### Audit

Every purged record writes one row to `integration_retention_purge_audit` — the
tenant, the class, the record identifier, the policy version that authorized it,
the effective `purge_after`, and a server-owned `purged_at` — **in the same
statement as the tombstone**, so a purge without an audit row cannot be
expressed. A `UNIQUE (tenant_id, record_class, record_id)` constraint makes
"exactly one audit row per record" a schema guarantee rather than a property of
the sweeper, which is what lets two replicas run the purge concurrently with no
lease and no leader election.

## 3. Session workspace samples and exports: redacted or encrypted, and now expirable

The Integration Session workspace is the design-time surface, and it holds sample
messages supplied by integration engineers.

| Policy | Storage | Citation |
|---|---|---|
| `redact` (default) | The raw is redacted in place before storage; only the redacted form is persisted in `record_json` | `internal/integration/session/postgres.go:321-323` |
| `retain` (explicit) | The raw is encrypted with AES-256-GCM and stored in `integration_session_samples.raw_cipher`; the plaintext is not stored | `internal/integration/session/postgres.go:324-333`, `internal/integration/session/protector.go:35-51` |

The encryption is real: a random 96-bit nonce per record, a version byte, and
session/sample-scoped additional authenticated data
(`internal/integration/session/protector.go:43-50`).

Slice 4.1e adds expiry to both PHI-bearing classes here, and they get **different
purge shapes because they are different kinds of record**
(`internal/integration/session/migrations/0006_retention_expiry.sql`):

| Table | Column added | Purge shape | Why |
|---|---|---|---|
| `integration_session_samples` | `purge_after` | **row deleted outright**, `raw_cipher` included | The table carries no immutability trigger and never did. Giving it a tombstone would invent a guarantee it never had. |
| `integration_session_exports` | `purge_after`, `purged_at` | **`record_json` tombstoned**; the row stays | The row is evidence of a disclosure. C1 made it append-only, and its foreign key makes the exported session undeletable too (see section 4). |
| `integration_session_stream_events` | — | **pruned** past a schema floor | Envelope log, no PHI. This is a growth control, not a privacy control. |

An export purge destroys the **snapshot**, never the disclosure record:
`principal_json`, `reason`, `include_raw_payload`, and `exported_at` stay frozen
by the same column-scoped guard that permits the tombstone.

### The fanout log is pruned, and the schema sets the floor

`integration_session_stream_events` grew forever and nothing pruned it. It is now
prunable on the policy's `stream_event_retain` window, and the schema refuses to
delete any envelope younger than **24 hours** regardless of what a deployment
configures. The log is a resume cursor: a subscriber away longer than the window
sees a gap — already the documented replica-flip behaviour — but a one-minute
window would turn every reconnect into one. `UPDATE` remains blanket-blocked.

Pruned envelopes write no purge-audit row. They carry no clinical content, and
one audit row per envelope would replace one unbounded table with another.

### Captured and peeked samples

`.loom/38` Lane C-2 adds the two paths by which a **real** message, rather than
an engineer's synthetic one, can reach a session: `peekBatchConnection` reads
the first messages of one object of a compiled batch connection, and
`startConnectionCapture` arms a tap that copies the next frames a source admits
(`internal/integration/connection/peek.go`, `capture.go`). Production raw bytes
stay ephemeral (section 1): the tap copies a frame only after its admission
committed, and only into a session.

Both paths store samples through `session.AddSample` with `PHIPolicyRedact` and
`Redaction: SampleRedactionCapture`, which selects
`session.RedactCapturedHL7v2` (`internal/integration/session/redact_capture.go`)
instead of the pasted-sample redactor. A captured sample is never retained:
`SampleRedactionCapture` with `PHIPolicyRetain` is refused before anything is
written. The pasted-sample redactor, `redactHL7v2`, is unchanged, and
`TestRedactHL7v2_BehaviourPin` pins its exact output.

The capture redactor masks every field that names, locates, contacts, dates, or
numbers a person — the patient, the next of kin and associated parties, the
insured, the guarantor, and their employers and household — and every person
name in these segments whoever it belongs to: the HIPAA Safe Harbor identifier
categories (names, geography below the state, dates, telephone and fax, email,
SSN, MRN, health plan beneficiary, account, certificate and licence numbers, and
any other unique identifying number). Rules:

- A masked field is replaced whole — every repetition and component — by
  `REDACTED`; an empty field stays empty.
- **Dates are masked whole, year included.** That is stricter than Safe Harbor's
  year exception, and it is what makes the redactor a strict superset:
  `redactHL7v2` already masks PID-7 whole, and
  `TestRedactCapturedHL7v2_IsASupersetOfTheLegacyRedactor` holds the superset.
- The field separator is read from MSH-1, so a message that declares another
  one is masked by field rather than passed through.
- Fields that identify the payer's organisation and product (IN1-2 through
  IN1-5, IN1-7, IN2-25, IN2-58) are kept: they name no person, and a profile
  maps them to Coverage.

The table, HL7 v2.5.1 field numbers and names (104 fields):

| Segment | Masked fields |
|---|---|
| PID (17) | 2 Patient ID · 3 Patient Identifier List · 4 Alternate Patient ID · 5 Patient Name · 6 Mother's Maiden Name · 7 Date/Time of Birth · 9 Patient Alias · 11 Patient Address · 12 County Code · 13 Phone Number - Home · 14 Phone Number - Business · 18 Patient Account Number · 19 SSN Number - Patient · 20 Driver's License Number - Patient · 21 Mother's Identifier · 23 Birth Place · 29 Patient Death Date and Time |
| NK1 (16) | 2 Name · 4 Address · 5 Phone Number · 6 Business Phone Number · 8 Start Date · 9 End Date · 12 Associated Parties Employee Number · 13 Organization Name - NK1 · 16 Date/Time of Birth · 26 Mother's Maiden Name · 30 Contact Person's Name · 31 Contact Person's Telephone Number · 32 Contact Person's Address · 33 Associated Party's Identifiers · 37 Contact Person Social Security Number · 38 Next of Kin Birth Place |
| IN1 (21) | 6 Insurance Co Contact Person · 8 Group Number · 9 Group Name · 10 Insured's Group Emp ID · 11 Insured's Group Emp Name · 12 Plan Effective Date · 13 Plan Expiration Date · 14 Authorization Information · 16 Name Of Insured · 18 Insured's Date Of Birth · 19 Insured's Address · 24 Notice Of Admission Date · 26 Report Of Eligibility Date · 28 Pre-Admit Cert (PAC) · 29 Verification Date/Time · 30 Verification By · 36 Policy Number · 44 Insured's Employer's Address · 49 Insured's ID Number · 51 Signature Code Date · 52 Insured's Birth Place |
| IN2 (26) | 1 Insured's Employee ID · 2 Insured's Social Security Number · 3 Insured's Employer's Name and ID · 6 Medicare Health Ins Card Number · 7 Medicaid Case Name · 8 Medicaid Case Number · 9 Military Sponsor Name · 10 Military ID Number · 13 Military Station · 17 Military Retire Date · 22 Special Coverage Approval Name · 26 Payor Subscriber ID · 40 Mother's Maiden Name · 44 Insured's Employment Start Date · 45 Employment Stop Date · 49 Employer Contact Person Name · 50 Employer Contact Person Phone Number · 52 Insured's Contact Person's Name · 53 Insured's Contact Person Phone Number · 55 Relationship to the Patient Start Date · 56 Relationship to the Patient Stop Date · 61 Patient Member Number · 63 Insured's Phone Number - Home · 64 Insured's Employer Phone Number · 69 Insured Organization Name and ID · 70 Insured Employer Organization Name and ID |
| GT1 (24) | 2 Guarantor Number · 3 Guarantor Name · 4 Guarantor Spouse Name · 5 Guarantor Address · 6 Guarantor Ph Num - Home · 7 Guarantor Ph Num - Business · 8 Guarantor Date/Time Of Birth · 12 Guarantor SSN · 13 Guarantor Date - Begin · 14 Guarantor Date - End · 16 Guarantor Employer Name · 17 Guarantor Employer Address · 18 Guarantor Employer Phone Number · 19 Guarantor Employee ID Number · 21 Guarantor Organization Name · 24 Guarantor Death Date And Time · 29 Guarantor Employer ID Number · 31 Guarantor Hire Effective Date · 32 Employment Stop Date · 42 Mother's Maiden Name · 45 Contact Person's Name · 46 Contact Person's Telephone Number · 51 Guarantor Employer's Organization Name · 56 Guarantor Birth Place |

`session.CaptureRedactedFields` is this table in code;
`TestCaptureRedactedFieldsMatchesThePublishedTable` holds the two equal, and
`TestRedactCapturedHL7v2_MasksEveryTableField` puts a synthetic value in every
field of the five segments and requires every listed field masked with no
fragment surviving, and every other field unchanged.

**What is not masked, and why a captured sample is still PHI.** Segments outside
the five pass through: MSH and EVN timestamps, PV1/PV2 (visit number,
admission and discharge dates, attending clinicians), ORC/OBR/OBX/NTE (orders,
results, free text), DG1, AL1, and every Z-segment. Free text can name anyone.
A captured sample is therefore **minimised, not de-identified**: it stays in the
session under the session's own retention (`purge_after`, above) and is treated
as PHI everywhere the session is.

**Who can read the text.** `SessionSample.redactedPayload` returns the stored
text of a capture-redacted sample and nothing else — a pasted sample's stored
text, redacted by the narrower pasted-sample redactor, is still never returned.
`exportIntegrationBundle` without `integration.phi.export` strips it with
`rawPayload`.

**Audit.** Every peek and every capture is one reason-required row in
`integration_connection_captures`
(`internal/integration/connection/migrations/0001_connection_catalog.sql`,
`0002_connection_capture_intake.sql`): the verified principal, the reason, the
session, the runtime source or the connection and the revision digest a peek
read, the limits, the count, the status, why it finished (`problems_json`), and
who cancelled it and why (`cancellation_json`). A trigger freezes the row's
provenance; while the row is armed, `status`, `captured`, and `completed_at`
advance — and the finishing update writes `problems_json` and
`cancellation_json` — each by an update that raises `version` by exactly one;
once the row is finished it is frozen whole. No row is ever deleted.
`make connection-capture` proves the tap and the peek against PostgreSQL, a
real MLLP listener, and MinIO.

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
the store (`internal/integration/session/postgres.go:907-911`); the grant governs
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
| `integration_canonical_events` | blanket `BEFORE DELETE` + column-scoped `BEFORE UPDATE` | insert-only ledger holding the retained PHI. Slice 4.1e narrowed C1's blanket guard to permit exactly the retention-policy stamp and the one-time tombstone — see section 2 |
| `integration_message_lineage` | blanket | insert-only provenance |
| `integration_delivery_audit` | blanket | insert-only delivery ledger |
| `integration_delivery_operations` | blanket | insert-only replay/resubmit/discard ledger |
| `integration_batch_audit` | blanket | insert-only batch ledger (`internal/integration/batch/migrations/0003_batch_audit_immutability.sql`) |
| `integration_receipts` | column-scoped `BEFORE UPDATE` + blanket `BEFORE DELETE` | admission identity, provenance, and attribution frozen; the table is a state table by design |
| `integration_delivery_attempts` | column-scoped `BEFORE UPDATE` + blanket `BEFORE DELETE` | lineage and destination binding frozen, while `status` / `attempt_count` / `scheduled_at` / `completed_at` / error columns stay writable for the delivery state machine |

The receipt and attempt column-scoped guards are in
`internal/integration/processor/migrations/0004_audit_immutability.sql`; the
canonical event's and the session export's are in
`internal/integration/processor/migrations/0005_retention_expiry.sql` and
`internal/integration/session/migrations/0006_retention_expiry.sql`.

**Referential integrity is not immutability.** The `ON DELETE RESTRICT` foreign
keys on these tables protect only rows that still have dependents; the last row
of a chain, or any row whose dependents were removed first, was previously
deletable. The kill-test therefore aims its `DELETE` assertions at
purpose-seeded dependent-free rows, and its negative control proves those
deletes succeed on the pre-migration schema.

Row-level triggers do not affect DDL, so `DROP SCHEMA … CASCADE` teardown in the
integration suites is unaffected.

---

## 6. What is implemented, and what is still not

| Control | Status |
|---|---|
| TTL or expiry on canonical event payloads | **Implemented** (Slice 4.1e) — `purge_after` / `purged_at`, purged by tombstone, audited |
| TTL or expiry on session sample ciphertext and redacted records | **Implemented** — `purge_after`, row deleted outright |
| TTL or expiry on session export snapshots | **Implemented** — `purge_after` / `purged_at`, snapshot tombstoned, attribution preserved |
| Pruning of the session stream fanout log | **Implemented** — policy window with a 24 hour schema floor |
| Durable purge component | **Implemented** — `internal/integration/retention`, multi-replica safe with no lease |
| Per-tenant, audited, mutable retention policy | **Implemented** — `integration_retention_policies` + `integration_retention_policy_audit` |
| **Purge of backup copies** | **Not implemented and out of scope.** A tombstone is not a backup-inclusive deletion. Backups taken before a purge still hold the payload; expiring them is a storage-layer control. Tracked as a Slice 4.4c interaction |
| **Role separation for the purge** | **Not implemented — topology ratified, deployment work deferred to its own slice.** Every migration runs on the same connection the runtime uses, so the application role owns the tables it guards. This is now **demonstrated, not asserted**: `TestPurgeRoleSeparation_ApplicationRoleCanDropItsOwnGuardToday` connects as an ordinary `NOSUPERUSER` role, runs the shipped migrators through it exactly as `serve` does, and then drops an immutability trigger, performs the mutation that trigger forbade, disables a second trigger, replaces a shared guard **function** — disarming all four triggers that share it in one statement — and takes ownership of the table. All five succeed. The schema-enforced exemption is a guard against programmatic error, not against a hostile or compromised database role. The ratified answer is three roles (`fi_fhir_migrator` / `fi_fhir_app` / `fi_fhir_purge`) with migrations moved out of `serve` into an explicit `fi-fhir migrate` command; see `.loom/40-decisions.md` (2026-08-09) for the costed scope and why it is a deployment slice rather than a set of GRANTs |
| **Purge throughput** | **Repaired** (Lane S5-F). One tick drains the backlog within a per-tick wall-clock budget instead of removing one batch and waiting an interval. Proved at 10,000 records with a build-tagged negative control that restores the single-pass loop |
| **Backlog observability** | **Implemented** (Lane S5-F) — `fi_fhir_retention_backlog_records{record_class}`, a gauge. Before it, retention published counters only, and a purge falling behind was indistinguishable from one keeping up |
| Operator-facing purge status API | Not in this slice. The GraphQL schema was frozen for Sprint 4; the policy is server-owned configuration and the audit is queryable in the database |
| Encrypted production raw retention | **Deliberately unimplemented and fail-closed** (section 1). Do not enable it by lifting `ErrUnsupportedRawRetention` without the storage revision, key resolver, and access-audit path the contract requires |
| Narrowed transport-gate roles | **Partially implemented** — Lane S4-E. The transport gate enumerates all 131 root fields and refuses any it has no role for, with fine-grained roles on the sixteen operator control-plane fields (`internal/api/graphql/operation_authorization_roles.go`). `graphql:operator` is retained as a named compatibility grant covering the remaining 115, including the session workspace and `exportIntegrationBundle`. `integration.phi.export` is deliberately *not* a transport-gate role: it gates that mutation's `includeRawPayload` argument and still sits one layer deeper |

## Operational guidance

1. **An unconfigured deployment purges nothing.** Until
   `FI_FHIR_RETENTION_POLICY_PATH` is set, the submission database is a PHI
   system of record with indefinite retention, and backup encryption, access
   control, and deletion schedules are operated at the database and storage
   layer.
2. **Write the policy document as a privacy decision, not as configuration.** It
   carries `authorized_by` and `reason`, both required, both recorded in
   `integration_retention_policy_audit` on every change. Restarting with an
   unchanged document mints no version and forges no audit entry.
3. **A purge is irreversible in the live database and reversible from a backup.**
   Plan backup expiry alongside the retention window, or the effective retention
   is the backup retention.
4. Treat session workspaces as PHI-bearing. Retained samples are encrypted at
   rest by the application; redacted samples are not raw but are not guaranteed
   PHI-free for every format.
5. Every `exportIntegrationBundle` call is a logged disclosure. Review
   `integration_session_exports` — `principal_json`, `reason`,
   `include_raw_payload`, `exported_at` — during access reviews. Purging the
   snapshot does not remove the disclosure record.
6. Grant `integration.phi.export` narrowly. It is the only role that unlocks raw
   sample payloads in an export.
7. Review `integration_retention_purge_audit` during access reviews. It is the
   record of what the platform destroyed, when, and under which policy version.
8. **Alert on the backlog gauge, not on the purge counters.**
   `fi_fhir_retention_backlog_records` rising across consecutive ticks means the
   purge is not keeping up with ingest and the tenant's declared retention window
   is not the effective one. The counters climb either way. A single non-zero
   sample is normal between ticks.
9. **Raising `FI_FHIR_RETENTION_PURGE_BATCH_SIZE` is a transaction-size decision,
   not a throughput decision.** Throughput is the drain loop's job. A larger batch
   holds more row locks per statement against the busiest table in the system;
   raise it only if the per-pass overhead is measurably dominating, and watch the
   gauge rather than the batch size to decide whether the purge is keeping up.

## Related

- `.loom/32-sprint4-execution-specs.md` — Lane S4-B, corrections 11-20
- `.loom/40-decisions.md` — 2026-08-08, "Slice 4.1e": the immutability exemption
  and the policy-placement decision
- `.loom/iteration-plan-phase-4-slice-4-1e-retention-purge.md` — Slice 4.1e's
  plan and day-1 gate results
- `.loom/33-sprint5-execution-specs.md` — Lane S5-F, found defect D1 and
  corrections 53-55
- `.loom/40-decisions.md` — 2026-08-09, "Purge role topology": the ratified
  three-role target, why the deployment work is its own slice, and the four
  disarm shapes an ordinary application role has today
- `.loom/31-sprint3-execution-specs.md` — Lane S3-C, corrections 21-25, and the
  C1/C2 split
- `.loom/iteration-plan-phase-4-slice-4-1d-c1-phi-audit.md` — Slice 4.1d C1's plan
  and day-1 gate results
- `docs/operations/README.md` — general operations entry point
