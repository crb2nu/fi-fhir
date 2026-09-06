### 2026-08-09: WAL/PITR posture — this repository certifies RTO and hands RPO to the operator, with a method

- Decision:
  - **Option A.** `fi-fhir` ships the point-in-time-recovery posture as
    *documentation plus a verified restore procedure*, and states the RPO a
    deployment achieves as a function of the operator's archiving choice. It
    does **not** ship an `archive_mode`/`archive_command` configuration.
  - The **RTO half of budget 5 (≤ 30 minutes) is measured and certified** by
    this repository, against the documented method: slice 4.4c times
    `scripts/pgdump-roundtrip.sh` end to end — dump, restore, first successful
    delivery `Claim` from the restored database — in the same CI job that
    already proves the restore is faithful, and archives the number.
  - The **RPO half is an operator responsibility with a stated method**, not a
    product guarantee. `docs/operations/PRODUCTION-HARDENING.md` and
    `docs/operations/SUPPORTED-1.0.md` say so in the same words, and
    `SUPPORTED-1.0.md`'s required-evidence item 4 is split accordingly: the
    backup/restore proof and the RTO measurement close; the RPO number does
    not, and the reason is named rather than left as an empty checkbox.
  - **Option B — a reference archiving configuration proved end to end in CI —
    is filed as a named follow-up** with its cost written down (below), not
    silently deferred.
  - **Option C — relax budget 5 — is presented and rejected.** The 5-minute
    target is right for a PHI integration engine. What is wrong is the
    *documented method*, and amending a product-spec target to match a weak
    method is the wrong repair.
- Rationale:
  - The gap is already recorded honestly in shipped documentation
    (`PRODUCTION-HARDENING.md` "Recovery objectives, honestly", and the
    2026-08-09 `pg_dump` decision above). The open question for 4.4c was never
    *whether* the logical dump meets the RPO — it does not — but *what this
    repository ships in response*.
  - PITR belongs to whoever runs the database. The only PostgreSQL in
    `deploy/` is a single-replica `Deployment` on a ReadWriteOnce PVC
    (`deploy/kubernetes/base/postgres.yaml`), which is a development
    convenience; a production deployment uses a managed service or a
    PostgreSQL operator, both of which own WAL archiving through their own
    interfaces. A reference `archive_command` written against the dev manifest
    would be a configuration almost nobody runs, carrying the authority of a
    product guarantee.
  - Option A spends the lane's budget on the parts only this repository can
    prove, and every one of them is CI-runnable today: that a restored database
    is *faithful* (rows, PHI payloads, immutability triggers attributable to
    those triggers, the `NOT VALID` provenance CHECK, and all six schema
    ledgers at their declared versions), that the application *resumes* from it
    with no manual repair, and that recovery *time* is bounded and recorded.
  - Option B is not merely larger; it changes what this repository is
    accountable for. It requires turning the dev `Deployment` into a real
    archiving setup, an object-storage service container in CI, a WAL-replay
    assertion, and thereafter ownership of an operational posture the product
    does not otherwise own — while competing for the same lane budget as
    budget 4 (destination recovery under an injected fault) and budget 7
    (Kubernetes upgrade/rollback provability), both of which are provable now.
  - Stating an uncertified RPO would be worse than stating none. The repository
    has a standing rule against retroactive vouching in its schema; the same
    rule applies to operational claims.
- Alternatives considered:
  - **Ship `archive_mode = on` with a placeholder `archive_command` and no
    proof** (rejected: an unproven archiving configuration in `deploy/` reads
    as a supported capability and is the exact shape of the tracing-enabled
    artifacts slice 4.4d is currently removing).
  - **Measure RPO by dump interval and publish that number** (rejected: the
    interval is not the loss bound — loss is the interval plus the dump
    duration, which grows with the data. Publishing the interval as an RPO is
    a claim the method cannot support.)
  - **Leave item 4 of `SUPPORTED-1.0.md` wholly blocking** (rejected: it
    conflates two independently provable things. The restore proof and the RTO
    measurement are done and should be recorded as done; the RPO number is not
    and should be recorded as an operator responsibility with a method.)
- Consequences:
  - `SUPPORTED-1.0.md` item 4 splits into a closed half (backup/restore
    faithfulness plus a measured RTO against the documented method) and an open
    half (RPO, operator-owned, achieved by the operator's archiving choice).
    The product does not claim a 5-minute RPO.
  - `PRODUCTION-HARDENING.md`'s RTO/RPO table stops reading as an operational
    commitment. The two rows whose RPO is unachievable with logical dumps say
    who owns the number and what method achieves it, and the RTO column carries
    the measured value for the database-failure row rather than a target.
  - The archived RTO is a measurement of the *documented procedure* on a CI
    fixture, not a capacity claim about production data volumes. It is recorded
    with the row counts it was measured against, for the same reason
    `values-reference-profile.yaml` says it is not a capacity claim.
  - **Follow-up filed (Wave 3): "reference WAL archiving configuration".**
    Cost, written down so it can be scheduled rather than rediscovered: convert
    `deploy/kubernetes/base/postgres.yaml` to a `StatefulSet` with a WAL volume
    or adopt an operator; an `archive_command` targeting object storage plus
    credentials plumbing; a MinIO service container in CI; a restore-to-
    timestamp script; and a WAL-replay assertion that proves a transaction
    committed after the base backup is recovered. Only then does budget 5's RPO
    become a product claim.
  - Slice 4.4c ships **no new durable schema** under this decision, so the two
    pre-existing rollback-unsafe columns filed to 4.4c by the 2026-08-09
    rollback decision — `integration_delivery_attempts.scheduled_at` and
    `integration_delivery_outbox.updated_at`, both from processor ledger 2 —
    are **re-filed to Lane S5-F**, which owns processor `0006`
    (`.loom/33-sprint5-execution-specs.md`, Schema Freeze Status Per Ledger).
    They stay in `knownRollbackUnsafeColumns` with their dated reason until
    then.
- Sources:
  - [S1] `.loom/33-sprint5-execution-specs.md` — Lane S5-B, "The WAL/PITR
    posture decision (required deliverable)"; corrections 12-14
  - [S2] `docs/operations/PRODUCTION-HARDENING.md` "Recovery objectives,
    honestly" and "What the restore proof covers"
  - [S3] `.loom/40-decisions.md`, 2026-08-09 "A logical `pg_dump` cannot meet
    the 5-minute RPO"
  - [S4] `.loom/20-product-spec-integration-engine-ide-completion.md:277-278`
  - [S5] `deploy/kubernetes/base/postgres.yaml`; `scripts/pgdump-roundtrip.sh`
