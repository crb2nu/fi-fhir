### 2026-07-13: Resolve Executable Artifacts by Stored Revision and Digest

- Decision:
  - Preserve the Source Profile serial revision ID as its immutable identity and
    add `source_profiles.current_revision_id` as the mutable pointer. Creation
    writes the initial revision; update locks the pointer, writes the incoming
    revision, then advances it transactionally.
  - Install database compatibility triggers with the schema expansion so a
    pre-upgrade pod that writes the legacy mutable row during a rolling rollout
    still creates or advances the immutable revision. New store code uses the
    same trigger-owned invariant instead of a parallel write algorithm.
  - Treat legacy profile `version` strings as display labels, not identity or
    uniqueness keys. Backfill each legacy current row once without deleting its
    existing history.
  - Hash profile executable content as domain-separated canonical JSON and
    workflow executable content as domain-separated exact UTF-8 YAML bytes.
    Canonicalize equivalent decimal/exponent number spellings independently of
    PostgreSQL `JSONB`; reject duplicate JSON keys, invalid Unicode, malformed
    identities, wrong owners, and digest mismatches before returning content.
  - Keep `internal/integration/processor` storage-neutral. A narrow adapter in
    the existing GraphQL store package performs exact owner-and-revision reads;
    the resolver is configured for one deployment tenant and returns only
    defensive copies.
  - Serialize workflow version allocation by locking its definition row and
    require publication to prove version ownership.
  - Make the PostgreSQL v1-after-v2 restart proof a required CI job independent
    of the broad soft-failing integration suite.
- Rationale:
  - A content-addressed integration is not immutable if its runtime follows a
    mutable profile or release pointer.
  - PostgreSQL stores profiles as `JSONB`, so semantic JSON identity is stable
    across key order/whitespace; workflow YAML is stored as text, so exact bytes
    are the honest compatibility boundary.
  - Loading by a global version ID and checking ownership afterward crosses the
    storage security boundary too late. Exact owner-and-version lookup avoids
    exposing another workflow's bytes even transiently.
  - One deployment tenant makes the 1.0 isolation claim explicit without
    pretending the legacy authoring tables already provide shared-hosting RLS.
  - The supported chart rolls two replicas by default, so a backfill without a
    legacy-write compatibility path could immediately create a null or stale
    current pointer after migration.
- Alternatives considered:
  - Resolve the current profile/workflow at execution time (rejected; published
    integrations would silently change behavior).
  - Snapshot profile JSON and workflow YAML inside every integration revision
    (rejected; duplicates artifacts and weakens shared lifecycle/audit history).
  - Use profile version labels as immutable IDs (rejected; legacy updates can
    reuse a label and existing data must remain migratable).
  - Require a quiesced/recreate-only migration (rejected for the normal rolling
    path; compatibility triggers make the expansion safe while a later release
    may still benchmark the migration lock window).
  - Import GraphQL store records directly into the processor (rejected; reverses
    the application/storage dependency and makes alternate adapters harder).
- Consequences:
  - Digest domain prefixes and canonicalization rules are versioned contracts;
    changing them requires an explicit compatibility migration.
  - The profile backfill briefly locks `source_profiles`; compatibility triggers
    protect writes from old pods after the lock, while large production tables
    still need migration-duration evidence before the 1.0 upgrade claim closes.
  - Slice 1.1b may consume only exact resolved content and must fail production
    closed until Slice 1.2 supplies a durable committer.
- Sources:
  - [S1] `internal/api/graphql/store/profile_store.go`
  - [S2] `internal/api/graphql/store/workflow_lifecycle_pg_store.go`
  - [S3] `internal/api/graphql/store/artifact_revision_loader.go`
  - [S4] `internal/integration/processor/revisions.go`
  - [S5] `internal/integration/processor/revisions_integration_test.go`
  - [S6] `.gitlab-ci.yml`
