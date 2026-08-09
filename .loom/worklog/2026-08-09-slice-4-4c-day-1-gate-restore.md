### 2026-08-09 - Slice 4.4c day-1 gate: restore proof trigger attribution

Lane S5-B's day-1 gate, landed deliberately red. This entry is the record the
lane spec requires before the fix may be written
(`.loom/33-sprint5-execution-specs.md`, "Lane S5-B — Kill-Test", found defect
D3).

- What changed:
  - New test-only file
    `internal/integration/migrationcompat/restore_attribution_integration_test.go`
    holding `TestChaosRecovery_RestoreProofAssertionsAreTriggerAttributed`. No
    production code, no schema, no CI job.
  - `guardedMutations(fixture)` lifts slice 4.4a's six immutability mutations
    (`compatibility_integration_test.go:278-301`) into a named function so the
    proof and this control cannot drift apart. The implementation MR rewires
    the proof onto it; this MR only reads it.
  - The gate is **not** wired into `test:migration-compatibility`. That job
    runs an explicit `-run` list and an arity-3 `-list` existence guard, and
    `.loom/33`'s one hard sequencing rule is that no lane appends to
    `.gitlab-ci.yml` before Lane S5-0's include split merges. The gate is wired
    in, green, as the round-trip's negative control in the implementation MR.

- Why:
  Slice 4.4a's restore proof states its own claim in its doc comment
  (`compatibility_integration_test.go:270-274`): "A dump/restore that recreated
  the tables without their triggers would leave a database that looks complete
  and silently permits every mutation C1 forbids." That claim is only true if
  each of the six mutations is refused *by its trigger*. The proof asserts
  `err != nil` and nothing more, and it has no negative control, so a mutation
  a foreign key would refuse anyway is indistinguishable from one the guard
  refuses.

  Reproducing that before repairing it is what separates a regression guard
  from a test written after the code. Had the gate passed, `.loom/33` defect D3
  would be wrong and the lane would correct `.loom/33` before writing a line of
  production code.

- Evidence:
  Local run against PostgreSQL 16 on the remote Docker context, through the
  documented runbook (`scripts/pgdump-roundtrip.sh`), client pg_dump 16.14
  against server 16:

  ```
  restore_attribution_integration_test.go:148: dropped 22 non-internal triggers
      on the restored database
  --- FAIL: TestChaosRecovery_RestoreProofAssertionsAreTriggerAttributed (7.46s)
      DAY-1 GATE CONFIRMED (half B) — 3 of 6 immutability assertions in slice
      4.4a's restore round-trip are shadowed and would stay green with their
      triggers dropped:
        delete a canonical event   still refused with SQLSTATE 23503:
          violates foreign key constraint
          "integration_delivery_attempts_tenant_id_event_id_fkey"
        delete a receipt           still refused with SQLSTATE 23503:
          violates foreign key constraint
          "integration_canonical_events_tenant_id_receipt_id_fkey"
        delete a delivery attempt  still refused with SQLSTATE 23503:
          violates foreign key constraint
          "integration_delivery_outbox_tenant_id_attempt_id_fkey"
  ```

  Predicted result: **fail**, three of six, each SQLSTATE `23503` rather than
  `P0001`. Observed result: **fail**, three of six, each SQLSTATE `23503`.
  D3 stands exactly as written; the lane proceeds as specified.

  Half A passed, and that matters: with every trigger in place all six
  mutations are refused with `P0001`, because every C1 guard is a `BEFORE ROW`
  trigger (`processor/migrations/0004_audit_immutability.sql:30-64`) and a
  `BEFORE` trigger raises before PostgreSQL's referential-integrity check runs.
  So the shadowing is invisible from inside the proof — it only appears once
  the guards are removed, which is what the missing negative control would
  have done.

  One incidental number in D3's narrative is off and is recorded here rather
  than silently carried: D3 says the reproduction dropped "all 17" non-internal
  triggers. Against a database restored from all six migrated ledgers the count
  is **22**. The finding is unaffected — the three shadowed assertions and
  their SQLSTATEs are exactly as predicted — but a later reader should not
  treat 17 as the trigger census.

- What's next:
  The slice 4.4c implementation MR, which lands:
  1. task 2 — repair the restore proof: give each guarded mutation a
     dependency-free target row so the guard is the only thing that can refuse
     it, assert `P0001` on all six, run `assertEveryLedgerAtDeclaredVersion`
     against the restored database (`compatibility_integration_test.go:103-109`
     omits it), and extend `durableClasses` (`:216-225`) with
     `integration_session_samples`, `integration_session_stream_events`, and
     the three retention tables so the five 4.1e triggers are exercised
     post-restore;
  2. this gate, wired into `test:migration-compatibility` as the round-trip's
     permanent negative control, after Lane S5-0's CI include split merges;
  3. tasks 3-8: the deployment-artifact upgrade properties, reference-profile
     rendering, the first executing e2e job, budget 4 destination recovery
     under an injected `tcpProxy` fault, the RTO measurement, and the
     `.loom/30` correction.

  The lane rebases onto Lane S5-D's token-bucket ledger and Lane S5-F's
  role-scoped retention state before task 2's `durableClasses` extension is
  final — an out-of-date `durableClasses` list is precisely what D3 costs.

- Sources:
  - [S1] `.loom/33-sprint5-execution-specs.md` — Lane S5-B, found defect D3,
    day-1 gates table
  - [S2] `internal/integration/migrationcompat/compatibility_integration_test.go:270-318`
  - [S3] `internal/integration/processor/migrations/0004_audit_immutability.sql:22-64`
  - [S4] `internal/integration/processor/migrations/0005_retention_expiry.sql:9-12`
  - [S5] `scripts/pgdump-roundtrip.sh`
