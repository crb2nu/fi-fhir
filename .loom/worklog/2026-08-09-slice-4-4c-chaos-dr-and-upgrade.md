### 2026-08-09 - Slice 4.4c chaos, DR, and Kubernetes upgrade/rollback

Lane S5-B's implementation. The day-1 gate
(`2026-08-09-slice-4-4c-day-1-gate-restore.md`) and the WAL/PITR posture
decision (`2026-08-09-slice-4-4c-wal-pitr-posture-decision.md`) landed first, in
that order, and this entry is the work they authorised.

- Owned files, recorded before the first commit per the lane spec:
  `internal/integration/migrationcompat/{compatibility,restore_attribution,recovery_time}_integration_test.go`
  and `fixture_test.go`; `internal/integration/delivery/chaos_recovery_integration_test.go`;
  `test/e2e/**`; `deploy/**`; `scripts/validate-k8s-schema.sh`;
  `ci/s5b-chaos-dr.yml`; `docs/operations/PRODUCTION-HARDENING.md` (recovery
  sections only — Lane S5-C owns `:583-598`); `docs/operations/SUPPORTED-1.0.md`
  (evidence items 3 and 4); `.loom/30` (the 4.4c section); `Makefile` (one
  `.PHONY` line, three new targets); one `include:` line in `.gitlab-ci.yml`
  plus edits to the existing `test:migration-compatibility` and `lint:helm`
  jobs. No new durable schema, no `internal/integration/**` production code.

- What changed:
  - **Task 2 — the restore proof is repaired.** `assertImmutabilityGuardsSurvived`
    asserts the SQLSTATE, not merely that an error came back. Three of the six
    mutations now target childless copies of their rows, which
    `seedDurableFixture` writes for the purpose. `durableClasses` gains the
    whole 4.1e surface, taking the durable set from 8 tables to 13 and the
    guarded set from 6 mutations to 11. `assertEveryLedgerAtDeclaredVersion`
    runs against the restored database. The day-1 gate becomes the round-trip's
    permanent attribution control and joins `make migration-compatibility`,
    with the CI job's `-list` arity guard moved 3 → 4.
  - **Task 7 — RTO.** The round-trip times itself from the dump to the first
    successful delivery `Claim` and archives `recovery-rto.json`, carrying the
    row counts it was measured against.
  - **Task 3 — the deployment artifacts.** PostgreSQL gets `strategy: Recreate`,
    a `pg_ctl -m fast` preStop, and a 90s grace period. Both application
    Deployments get `maxSurge: 1` / `maxUnavailable: 0`, `minReadySeconds: 15`,
    `terminationGracePeriodSeconds: 60`, and a preStop.
  - **Task 4 — schema validation.** `scripts/validate-k8s-schema.sh` renders
    four artifacts and validates them `-strict` against the pinned 1.36 API
    schemas, with a negative control in the same invocation. Wired into the
    existing `lint:helm`.
  - **Task 6 — budget 4.** `TestChaosRecovery_DestinationOutageOpensTheCircuitAndResumesOnRepair`,
    with its own CI job in `ci/s5b-chaos-dr.yml`.
  - **Task 5 — the first e2e job**, scoped to what it can hold green; see the
    finding below.
  - **Tasks 1b and 8 — the docs.** `PRODUCTION-HARDENING.md` and
    `SUPPORTED-1.0.md` now state the decision rather than the gap, and `.loom/30`'s
    4.4c section is rewritten from findings it predated.

- Why the shape it has:
  The lane opened on the day-1 gate rather than on PITR because the gate's
  answer reorders everything after it. Slice 4.4a's restore proof claims, in its
  own doc comment, that a restore which dropped the triggers "would leave a
  database that looks complete and silently permits every mutation C1 forbids".
  That claim was half false. Half of the proof's refusals were foreign keys, and
  a foreign key is not PHI governance. Building disaster recovery on top of it
  would have been building on a proof that would not notice the disaster it was
  written to catch.

  The fault injection uses an in-test TCP proxy rather than a container stop for
  the reason S3-A recorded in-source: a runner has no Docker socket. Severing
  the socket is also the stronger fault — it exercises the transport's own
  classification and the dispatcher's circuit rather than a container's restart
  timing.

- Evidence:
  All against PostgreSQL 16 on the remote Docker context.

  Restore round-trip, repaired:

  ```
  assertion PASSED: row counts identical across 13 durable classes after restore
  assertion PASSED: all six ledgers are at their declared SchemaVersion
  assertion PASSED: all 11 guarded mutations still raise P0001 on the restored database
  assertion PASSED: the 4.1c-a provenance CHECK survived and is still NOT VALID
  assertion PASSED: the queued attempt was claimed, published, and not re-claimed
  RECOVERY TIME: restore 5.295s, resume-to-first-claim 5.789s, budget 30m0s,
                 16 rows across 13 durable classes
  ```

  The attribution control, and its own mechanism check. Mid-repair, with the
  guard attempt still pointing at the guard event, it caught the one remaining
  shadow — `1 of 11 immutability assertions ... still refused with SQLSTATE
  23503` — which is the control doing exactly its job on a real mistake made
  while writing the fix. Repaired:

  ```
  dropped 22 non-internal triggers on the restored database
  restore proof attribution CONFIRMED: all 11 guarded mutations raise P0001 with
  the triggers in place and all 11 succeed with the triggers dropped
  ```

  Budget 4, `-race`, 3s. Mechanism confirmed by removing the fault: with
  `proxy.Break()` deleted the test fails naming it — `the outage produced 0
  retry-scheduled outcomes, want exactly 2 (CircuitFailureThreshold)`.

  Schema validation, and its version sensitivity: the preStop `sleep` handler
  both Deployments now use is **rejected** by the 1.28 schema and accepted by
  1.36, so the pin is doing work rather than decorating the command. The
  negative control (an unknown `DeploymentSpec` field) is rejected.

  `requireService`, both directions: with the server up,
  `--- PASS: TestObservabilityEndpoints`; with `TEST_FIFHIR_URL` pointed at a
  dead port and `fi-fhir` still declared, `--- FAIL ... is declared required by
  FI_FHIR_E2E_REQUIRED_SERVICES but is unreachable`.

- Findings this lane produced that were not in its brief:
  1. **`.loom/33` correction 19 is wrong about the consequence, and is corrected
     in place.** The e2e tree is not correct-but-unrun; it is red.
     `test/e2e/e2e_test.go` — nine tests, no external infrastructure, the file
     `make test-e2e` runs — fails four ways including a nil-interface panic at
     `:493`, and two of `integration_test.go`'s tests fail with a reachable
     database. One cause covers most of it: the tests write Go dot-path
     templates while the engine binds JSON snake_case keys, which is the drift
     commit `5d07101c4` rewrote every *document* to correct three commits
     before this sprint. The skips were hiding failures, not absence. Repairing
     nine legacy-engine tests against a moved schema is its own slice and is
     filed rather than smuggled into a chaos lane; `test:e2e` makes the
     live-server assertion blocking and names the rest with executed evidence.
  2. **D3's trigger census is 22, not 17.** The finding is unaffected — three
     shadowed assertions, `23503` each, exactly as predicted — but a later
     reader should not treat 17 as the count.
  3. **The Kustomize base does not schema-validate as-is**, because
     `fi-fhir-umls.sops.yaml` is a SOPS document carrying a top-level `sops:`
     key that is not part of the Secret schema. `Secret` is skipped for that
     reason, and only `Secret`; every kind this slice is about is validated.

- What's next:
  - Rebase onto Lane S5-D's lifecycle `0002` token-bucket ledger and add it to
    `durableClasses` before merge. That is the one piece of new Sprint 5 durable
    state the repaired proof must cover — Lane S5-F released its processor
    `0006` claim and ships no schema change. The obligation is written into
    `durableClasses`' comment so it is not carried in someone's head, which is
    what D3 cost.
  - When Lane S5-0's `ci/_shared.yml` lands, the image/cache/rules blocks in
    `ci/s5b-chaos-dr.yml` collapse into `extends:`. They are inline today
    because YAML aliases are file-scoped and do not cross an `include`
    boundary.
  - Filed, with cost, in the posture decision: a reference WAL archiving
    configuration proved end to end in CI. Filed here: repairing
    `test/e2e/e2e_test.go` against the engine's real workflow schema.

- Sources:
  - [S1] `.loom/33-sprint5-execution-specs.md` — Lane S5-B, defect D3,
    corrections 12-22
  - [S2] `.loom/40-decisions.md` — 2026-08-09, "WAL/PITR posture"
  - [S3] `internal/observability/replicas_integration_test.go:875-969` — the
    TCP-proxy pattern and the no-Docker-socket rationale
  - [S4] `internal/integration/processor/migrations/0004_audit_immutability.sql:22-64`
  - [S5] `docs/operations/SUPPORTED-1.0.md:24` — the pinned Kubernetes minor
