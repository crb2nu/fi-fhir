# Slice Handoff — Phase 4, Slice 4.1e: Retention Policy and Purge Runtime

**Lane**: S4-B (`.loom/32-sprint4-execution-specs.md`)
**Branch**: `feat/phase4-slice-4-1e-retention-purge`
**Day-1 gate MR**: !152 (test-only, plus the two decisions)
**Plan**: `.loom/iteration-plan-phase-4-slice-4-1e-retention-purge.md`
**Operator doc**: `docs/operations/PHI-RETENTION.md`

---

## What is true now that was not before

1. **The PHI that persists has a retention policy**, and it can change without
   minting an integration revision. `integration_retention_policies` is mutable,
   attributed, versioned, and per-tenant; every change writes an append-only row
   to `integration_retention_policy_audit`.
2. **A purge exists, and it is a tombstone.** Slice 4.1d C1's blanket guard on
   `integration_canonical_events` is now a blanket `BEFORE DELETE` plus a
   column-scoped `BEFORE UPDATE` that permits exactly two shapes: the
   retention-policy expiry stamp, and the one-time canonical tombstone with
   `purged_at`. Everything else raises. `integration_session_exports` has the
   same shape over `record_json`, with the disclosure attribution frozen.
3. **Session samples are deleted outright**, ciphertext included, because that
   table never had an immutability trigger and a tombstone would have invented a
   guarantee.
4. **The unbounded fanout log is pruned**, on the policy's window, with a 24 hour
   floor the schema enforces regardless of deployment configuration.
5. **A purge without an audit row cannot be expressed.** The audit `INSERT` reads
   from the tombstone `UPDATE`'s `RETURNING` in the same statement, and
   `UNIQUE (tenant_id, record_class, record_id)` makes "exactly one audit row per
   record" a schema guarantee rather than a property of the sweeper.
6. **An unconfigured deployment purges nothing.** No
   `FI_FHIR_RETENTION_POLICY_PATH` means no component and no policy record.

## What is deliberately still not true

- **A tombstone is not a backup-inclusive deletion.** A backup taken before a
  purge still holds the payload. Effective retention is
  `max(policy window, backup retention)` until DR policy accounts for it.
- **The purge has no role separation.** Every migration runs on the same
  connection the runtime uses, so the application role owns the tables it guards
  and can drop any trigger. The schema-enforced exemption is a guard against
  programmatic error, not against a hostile database role.
- **There is no operator-facing purge API.** The GraphQL schema was frozen for
  Sprint 4. The policy is server-owned configuration; the audit is queryable in
  the database.

## Next actions

1. **Purge role separation** — the follow-up this slice filed rather than built
   (correction 16). Needs a de-privileged application role, a separate migration
   runner that owns DDL, and a purge role holding the narrow `UPDATE` grant. This
   is the only remaining way the exemption can be circumvented in-process.
2. **Backup-copy purge interaction, for Slice 4.4c.** DR work must decide whether
   backup retention is shortened to the policy window, whether purges are
   replayed against restored copies, or whether the documented effective
   retention is simply the backup retention. Today `PHI-RETENTION.md` states the
   third honestly; 4.4c should choose.
3. **S3-C2 remainder: nothing outstanding.** Every item `PHI-RETENTION.md`
   section 6 listed as "Not implemented — S3-C2" — canonical event TTL, session
   sample TTL, session export TTL, and the durable purge component — is
   implemented here, plus the fanout-log prune that no document had listed.
   The section is now an implemented/not-implemented split with the two entries
   above as the honest remainder.
4. **Operator-facing purge status**, whenever the schema lock lifts. A read model
   over `integration_retention_purge_audit` plus the current policy version is
   the natural shape; it needs no new durable state.
5. **A restamping cost note for large tenants.** `stampCanonicalEvents` is a
   reconciler: on a policy change it converges stored deadlines over successive
   passes, bounded by `FI_FHIR_RETENTION_PURGE_BATCH_SIZE` per pass. A tenant
   with millions of unstamped events will take many passes to converge. That is
   deliberate — an unbounded restamp would be one long transaction against the
   busiest table in the system — but a deployment shortening a window sharply
   should raise the batch size temporarily and watch
   `fi_fhir_retention_purges_total`.

## Filed corrections to planning documents

Made in this slice, per the coordination rule that a lane corrects the source
document rather than filing the discrepancy:

- `.loom/32-sprint4-execution-specs.md` — **correction 41**: the lane's negative
  control cannot come from one schema; it is split into pre-4.1e and pre-C1.
- `.loom/30-implementation-plan-integration-engine-ide-completion.md:413-418` —
  the `DELETE`-only framing corrected, with the three independent reasons.
- `.loom/slice-handoff-phase-4-slice-4-1d-c1-phi-audit.md:187-195` — item 3
  corrected in place: C1 blocked deletion **and** redaction, row deletion was
  structurally impossible anyway, and the privileged-role option was empty.
- `docs/operations/PHI-RETENTION.md:83-84,144` — the two citations S3-A drifted
  (correction 19) repaired to `:321-323`, `:324-333`, and `:907-911`.

## Gotchas for the next lane

- **The posture gate is designed to break when this table changes, and it did.**
  `TestPhiRetentionPosture_...` now asserts the expiry columns are present *and
  that they are exactly `purge_after` and `purged_at`*, so adding a third
  retention-shaped column to `integration_canonical_events` fails CI until
  `PHI-RETENTION.md` section 2 is rewritten with it. That is the intent.
- **`test:phi-retention-purge` runs `make phi-audit` too.** This lane amends C1's
  triggers on two tables; a green purge beside a red C1 would mean the exemption
  ate the guarantee, so they are proved in one job.
- **Every raise message on the amended tables still contains `append-only`.**
  C1's kill-test matches on that substring. If a future slice rewords those
  `RAISE EXCEPTION` strings, `make phi-audit` goes red for a reason that looks
  unrelated.
- **`delivery-reliability` cannot run locally.** It needs Kafka through
  testcontainers and there is no local Docker Desktop (`AGENTS.md`). CI is the
  proof. Nothing in this slice touches `internal/integration/delivery`; the
  interlock lives in `internal/integration/retention`'s own SQL.
