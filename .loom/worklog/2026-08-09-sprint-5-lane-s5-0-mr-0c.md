### 2026-08-09 - Sprint 5 Lane S5-0 MR 0c: D4 migration rule enforces both forms

Third and last MR of lane S5-0. **Must merge before Lane S5-D and Lane S5-F
author migrations** — that is the same reasoning that made slice 4.4a's task 4 a
prerequisite for 4.1e, except this time the guard was installed backwards.

- Owned files (recorded before first commit, per `.loom/33` coordination rules):
  - `internal/integration/migrationcompat/migration_rule_test.go` — the rule,
    the gate, the re-baselined exemption list. Lane S5-B owns
    `compatibility_integration_test.go` in the same package; disjoint files, and
    S5-0 merges first so S5-B rebases onto a settled rule.
  - `internal/integration/migrationcompat/negative_control_integration_test.go`
    — comment and message text only, four stale `0006` references.
  - `AGENTS.md` "Migration authoring" rule 1.
  - **No migration number claimed.** This is a test-only repair. The processor
    `0006` the exemption list points at belongs to Lane S5-F.

- Day-1 gate — `TestMigrationRule_AddColumnNotNullWithoutDefaultIsFlagged`.
  **Expected: FAIL, silently skipped because `tightened == 0`. Result: exactly
  that.** On unmodified `main`:

  ```
  --- FAIL: .../add_column_not_null_without_a_default
      the migration rule did NOT flag a rollback-unsafe column.
        SQL:      ALTER TABLE integration_widgets ADD COLUMN owner_id TEXT NOT NULL;
        tightenedColumns() returned nothing for this statement, so
        rollbackUnsafeColumns returns early and the file is never inspected.
  ```

  The `alter column set not null` control passed in the same run, which is what
  proves the red result is the rule's hole and not a broken fixture.

- What changed:
  - **`ADD COLUMN [IF NOT EXISTS] <name> … NOT NULL` is now a tightened
    column.** `addColumnClause` captures the name and the clause body;
    `notNullPhrase` and `defaultPhrase` are then tested against the body
    independently, because PostgreSQL accepts the modifiers in either order and
    a single pattern that assumes one ordering rejects correct migrations.
  - **The clause body allows a parenthesised group**, so the comma in
    `NUMERIC(10,2)` does not end the clause while a comma between two
    `ADD COLUMN`s still does. A clause that stops early loses the `NOT NULL` and
    admits the violation silently, which is the same class of bug as D4 itself.
  - **`--` comments are stripped before any pattern runs.** Migration files here
    carry long prose rationales, and prose about a rule is exactly where the
    rule's keywords appear.
  - **The exemption list is re-baselined and its two stale facts corrected.** It
    said the processor ledger head was 4 (it is **5**,
    `processor/postgres_submission.go`) and deferred the repair to "processor
    0005, owned by Lane S4-B" — a number that shipped in Sprint 4 as
    `0005_retention_expiry.sql` without the repair, and a lane that no longer
    exists. Both entries stay exempt on the merits: the columns entered at
    ledger 2 and head is 5, so nothing inside the one-version window can hit
    `SQLSTATE 23502` on them. The repair needs processor `0006`, which is Lane
    S5-F's number this sprint, so this test-only MR must not consume it.
  - **`AGENTS.md` rule 1 now shows both statements side by side**, notes that
    the `ADD COLUMN` half went unenforced from 4.4a until now, and lists the
    three ways a naive pattern fails in the other direction.
  - **Four stale `0006` references corrected to `0007`** in
    `negative_control_integration_test.go` (`:64,:72,:85,:93`) and two in
    `AGENTS.md` (`:228,:232`). The `.loom/` historical records that mention
    `0006_export_attribution_defaults.sql` — `.loom/30`, `.loom/32`, `.loom/40`,
    and the 4.4a worklog entry — are **left alone**: they are dated records of
    what was believed at the time, and `.loom/32:148` already carries its own
    correction. Rewriting history to look correct is the opposite of a worklog.

- Evidence:
  - Gate red before the fix, green after, with all nine sub-cases passing.
  - **Negative control**: deleting the `addColumnClause` branch from
    `tightenedColumns` puts the gate straight back to red with the original
    message. The gate watches the mechanism.
  - The real corpus is unchanged by the new pattern: 18 migration files across 5
    ledgers, the same two baseline entries logged, no new violation. So the
    repair is a guard against the next migration, not a retroactive failure.
  - `go build ./...`, `go vet` with and without `-tags=integration`,
    `golangci-lint run` (0 issues), `scripts/validate-docs.sh`,
    `scripts/worklog.sh check` — all pass.

- A limit, stated rather than hidden: a comma inside a string literal
  (`DEFAULT 'a,b'`) ends the clause early. Splitting SQL properly needs a
  parser, and this file deliberately is not one. The case does not occur in this
  repository's 18 migrations and the comment says so.

- What's next: lane S5-0 is complete. Merge order for the sprint is
  0a (!170) → 0b (!171) → 0c, then S5-C → S5-D → S5-F → S5-B → S5-A.

- Sources:
  - [S1] `.loom/33-sprint5-execution-specs.md` Found Defects D4; Lane S5-0 MR 0c
  - [S2] `internal/integration/processor/postgres_submission.go` `SchemaVersion`
    — the ledger head, which is the authority the stale exemption text ignored
  - [S3] `internal/integration/processor/migrations/0002_delivery_reliability.sql:13,32`
  - [S4] `internal/integration/session/migrations/0007_export_attribution_defaults.sql`
