### 2026-08-09: A logical `pg_dump` cannot meet the 5-minute RPO, and a newer client makes it unrestorable

- Decision:
  - `docs/operations/PRODUCTION-HARDENING.md` now states that the documented
    `pg_dump` backup **cannot** meet the product spec's RPO of 5 minutes, and
    marks the two affected rows of the RTO/RPO table as requiring WAL archiving
    / point-in-time recovery. PITR is recorded as the slice 4.4c prerequisite
    for budget 5.
  - The runbook's restore command gains `-v ON_ERROR_STOP=1`, and the backup
    command gains `--no-owner --no-privileges`.
  - The runbook now requires client tools whose **major version matches the
    server**, and `scripts/pgdump-roundtrip.sh` refuses to run on a mismatch.
- Rationale:
  - A periodic logical snapshot bounds loss to the dump interval plus the dump
    duration, on a database whose dump time grows with the data. Scheduling it
    more often does not converge on five minutes; it converges on a permanently
    running dump. Only continuous WAL shipping bounds loss to minutes, and
    nothing in this repository configures it.
  - Without `ON_ERROR_STOP=1`, `psql` prints errors, continues, and exits 0. A
    restore that failed to recreate the Slice 4.1d C1 immutability triggers
    would look like a success, and the recovered deployment would silently have
    weaker PHI governance than the one it replaced. This is the difference
    between a backup and the appearance of one.
  - The version-skew rule was found by running the proof, not by reading docs.
    pg_dump 17 and later write `SET transaction_timeout = 0` into the archive
    preamble; PostgreSQL 16 has no such GUC and rejects it. An operator on a
    workstation with newer client tools produces a dump that exits 0 and cannot
    be restored into the very server it came from — discovered during recovery,
    which is the worst possible moment.
- Alternatives considered:
  - **Quietly restoring with a newer client and filtering the offending `SET`**
    (rejected: it would make the proof pass while leaving every operator exposed
    to the same trap, and it asserts something upstream does not promise —
    archives are intended for a server of the dumping client's version or newer.)
  - **Leaving the RTO/RPO table as aspirational without comment** (rejected:
    it is presented as an operational commitment and read as one.)
  - **Implementing WAL archiving in this slice** (rejected: it is an
    infrastructure change spanning the chart, the storage class, and an object
    store, with no CI-runnable proof. That is 4.4c.)
- Consequences:
  - CI's `test:migration-compatibility` installs `postgresql-client-16` from
    PGDG, because Debian trixie — the `golang:1.26.5` base — ships only
    `postgresql-client-17`, which cannot restore into the PostgreSQL 16 service.
  - Local reproduction needs a matching client too:
    `brew install postgresql@16` and
    `FI_FHIR_PG_BIN_DIR=/opt/homebrew/opt/postgresql@16/bin`.
  - `docs/operations/SUPPORTED-1.0.md` item 4 (backup/restore and RPO/RTO proof)
    stays blocking, and now has a stated reason rather than an empty checkbox.
- Sources:
  - [S1] `.loom/32-sprint4-execution-specs.md` correction 27
  - [S2] `docs/operations/PRODUCTION-HARDENING.md` "Recovery objectives, honestly"
  - [S3] `scripts/pgdump-roundtrip.sh`
  - [S4] `.loom/20-product-spec-integration-engine-ide-completion.md:277-278`
