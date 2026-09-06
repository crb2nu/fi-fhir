### 2026-08-09: Purge role topology — ratify three roles with migrations out of `serve`, and ship D1 first (Lane S5-F)

- Decision:
  - **Option A is the ratified target topology.** Three PostgreSQL roles:
    `fi_fhir_migrator` owns every table, function, and trigger and is the only
    role that may apply a migration; `fi_fhir_app` holds DML only and cannot
    `ALTER`, `DROP TRIGGER`, or `CREATE OR REPLACE FUNCTION`; `fi_fhir_purge`
    additionally holds the narrow grants the tombstone path needs. Migrations
    move out of `serve` into an explicit `fi-fhir migrate` command invoked from
    a Helm pre-install/pre-upgrade hook, a Kustomize init container, and
    `docker-compose.yaml`.
  - **It does not ship in Sprint 5.** Lane S5-F ships D1 — the purge throughput
    repair — and re-files Option A as its own costed slice. This is the fallback
    `.loom/33-sprint5-execution-specs.md` names for this lane, and Wave 3
    already lists it.
  - **This is not Option C.** The follow-up is neither closed nor left filed
    open-endedly. The topology is decided; only the deployment work is deferred,
    and it is costed below. `docs/operations/PHI-RETENTION.md` is rewritten to
    say exactly that, and to point at the characterization test that now
    demonstrates the limitation instead of asserting it in prose.
  - **Lane S5-F releases its claim on `internal/integration/processor/migrations/0006_*.sql`.**
    Without the GRANTs there is no migration to author. D1's backlog gauge is a
    query over the existing partial index
    (`0005_retention_expiry.sql:154-156`), not a schema change. `0006` returns
    to the free pool; the deferred slice re-claims it against `origin/main` at
    the time, per the numbering rule.

- Rationale:
  - **D1 is a release blocker and the two halves have opposite risk profiles.**
    The shipped purge drains 200 records per class per hour with no catch-up on
    the table `internal/integration/retention/store.go:31-33` calls "the busiest
    table in the system". Role separation on top of a purge that cannot honour
    any policy is polishing the lock on a door that does not close. D1 is
    contained in one package plus one metric family; Option A changes the
    startup contract of every deployment path. Landing them in one MR that
    merges fourth in a five-deep queue puts the release blocker behind the
    deployment change's review surface.
  - **Option A is nonetheless correct, and the day-1 gate proves the premise
    rather than assuming it.**
    `TestPurgeRoleSeparation_ApplicationRoleCanDropItsOwnGuardToday` provisions
    an ordinary `NOSUPERUSER` role, runs the shipped migrators through it the
    way `runServe` does, and then — as that role — drops
    `integration_canonical_events_purge_only`, performs the payload rewrite the
    trigger existed to forbid, disables `integration_receipts_undeletable`, and
    takes ownership of `integration_canonical_events` away entirely. Every one
    succeeds. `PHI-RETENTION.md:293` was right, and it is no longer only prose.
  - **The gate found a fourth disarm shape that constrains the GRANTs, and it is
    not in the sprint spec.** The application role can
    `CREATE OR REPLACE FUNCTION reject_integration_submission_mutation()` with a
    `RETURN NEW` body and disarm **all four** triggers that share it
    (`processor/migrations/0004_audit_immutability.sql:30-47`) in one statement,
    without touching a trigger. A topology that revokes only `ALTER TABLE` would
    leave this route wide open. Function ownership must move to
    `fi_fhir_migrator` with `EXECUTE` granted to `fi_fhir_app` — which falls out
    of A for free, because the migrator creates the functions, and which no
    lesser option delivers. This strengthens the case for A and is a further
    argument against a partial one.
  - **A half-A is worse than no A.** Creating roles that nothing enforces would
    put three role names in a migration and in the docs while `serve` continues
    to connect as the owner, which is a compliance claim the deployment does not
    honour.

- Cost of the deferred slice (the reason it is deferred, itemised):
  - `fi-fhir migrate` command covering all six ledgers, and the removal of the
    six `Migrate(ctx)` call sites from the runtime paths
    (`cmd/fi-fhir/retention_runtime.go:71,78`, `batch_runtime.go:69,76`,
    `destination_identity_runtime.go:91`, `delivery_command.go:47`,
    `main.go:4799`, `preview_runtime.go:190,221,275`), replaced by a fail-closed
    ledger-at-head check at startup.
  - `processor/migrations/000N_*.sql` — idempotent role creation plus
    GRANT/REVOKE, safe on a database where the roles already exist, and function
    ownership transfer. If the split must reach session-owned retention state
    (`session/migrations/0006_retention_expiry.sql`) it takes the session
    ledger's next free number and says so in a worklog entry first.
  - Helm pre-install/pre-upgrade hook, Kustomize init container,
    `docker-compose.yaml`, `.env.example`, and the developer setup docs.
  - Relocating 4.4a's concurrent-replica assertion. Step 1 of
    `TestMigrationCompatibility_ConcurrentReplicaMigrationRollbackAndRestore`
    asserts two `serve` processes converge on a fresh database; it must move to
    two `fi-fhir migrate` invocations, not be deleted. That file is Lane S5-B's
    during Sprint 5, which is a second reason this does not belong in S5-F's MR.
  - The inverted proof: as `fi_fhir_app`, all four disarm shapes above must be
    refused **by PostgreSQL**, and as `fi_fhir_purge`, the tombstone path must
    succeed and every other mutation must still raise.

- Alternatives considered:
  - **Option B — two roles, `serve` still migrates under an elevated DSN**
    (rejected: the elevated credential is present in the process for its whole
    lifetime, so the application still holds the privilege it is supposed not to
    have. Harder to explain in a compliance conversation than either A or the
    status quo, for a fraction of A's benefit.)
  - **Option C — keep one role, document the limitation, close the slice**
    (rejected: the follow-up was filed *because* the documentation is not the
    answer, and the day-1 gate has now demonstrated the exposure rather than
    asserted it. Closing it would be choosing the weaker posture at the moment
    the evidence got stronger.)
  - **Shipping A alongside D1 in one MR** (rejected on sizing and on merge
    order, above.)
  - **Shipping the GRANT migration now and the deployment change later**
    (rejected: this is the half-A. It is also precisely the primary risk
    `.loom/33` names for this lane — "writing GRANTs before deciding who runs
    migrations".)

- Consequences:
  - `TestPurgeRoleSeparation_ApplicationRoleCanDropItsOwnGuardToday` stays in
    the tree as a permanent characterization test of the current posture. When
    the deferred slice lands, its assertions invert in place: every `succeeded`
    becomes `must be refused with an insufficient-privilege error`. It is
    therefore the deferred slice's negative control, already written.
  - `docs/operations/PHI-RETENTION.md` names the ratified topology, the four
    disarm shapes, and the test that demonstrates them, instead of the
    open-ended "named follow-up slice" language.
  - Sprint 5's schema-freeze table frees processor `0006` again. Any lane that
    needs it re-verifies against `origin/main` at commit time, per the rule.

- Sources:
  - [S1] `.loom/33-sprint5-execution-specs.md`, Lane S5-F role-topology table,
    corrections 53-55, Found Defect D1
  - [S2] `docs/operations/PHI-RETENTION.md:293`
  - [S3] `.loom/decisions/2026-08-08-slice-4-1e-the-immutability-exemption-for.md` (Slice 4.1e; cited by line number before the journal was split)
  - [S4] `internal/integration/retention/purge_throughput_gate_integration_test.go`
  - [S5] `.loom/worklog/2026-08-09-lane-s5-f-day-1-gates.md`
