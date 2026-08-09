-- Slice 4.4a: make one-version rollback survivable.
--
-- 0004_export_attribution.sql set principal_json, reason, and
-- include_raw_payload NOT NULL with no DEFAULT (:31-34). That is correct for
-- the current writer, which names all eight columns
-- (internal/integration/session/postgres.go:949-954). It is fatal for the one
-- before it, which names five.
--
-- During a rolling upgrade both binaries run against the migrated schema at
-- once, and a rollback runs the older binary against it indefinitely. Every
-- export from an N-1 replica therefore died on
--   ERROR 23502: null value in column "principal_json" ... violates not-null
-- which is a straight contradiction of the product spec's budget 6:
-- "one-version rolling upgrade and rollback preserve receipts, revisions, and
-- resumable work without schema downgrade corruption"
-- (.loom/20-product-spec-integration-engine-ide-completion.md:279-280).
-- TestMigrationCompatibility_ExportInsertShapeSurvivesOneVersionRollback
-- reproduced it against unmodified main before this migration was written.
--
-- The repair reuses the exact sentinel 0004 already backfills pre-migration
-- rows with (:26-28). That is 4.1b3's no-retroactive-vouching idiom applied
-- forward rather than backward: an insert that cannot name a principal must
-- not be given a plausible one, and must not silently look attributed. It gets
-- the same visibly-unattributed marker an unattributable historical row gets,
-- so `principal_json ? 'unattributed_legacy_export'` finds both classes with
-- one predicate.
--
-- The cost, stated rather than hidden: a future writer that forgets the column
-- now records an unattributed export instead of failing loudly. That trade is
-- accepted because the alternative — a rollback that cannot export at all —
-- is a worse failure, and because it is bounded by a proof:
-- TestMigrationCompatibility_ConcurrentReplicaMigrationRollbackAndRestore
-- asserts that the *current* Go writer still records a real principal, so the
-- DEFAULT can never mask a regression on the live path.
--
-- include_raw_payload defaults to false. An insert that cannot name the column
-- must never imply that raw PHI was disclosed.
--
-- Adding a DEFAULT to an existing column is a catalog-only change in
-- PostgreSQL 11+: it does not rewrite the table and takes only a brief
-- ACCESS EXCLUSIVE lock.

ALTER TABLE integration_session_exports
    ALTER COLUMN principal_json
        SET DEFAULT '{"id": "", "kind": "", "auth_method": "", "unattributed_legacy_export": true}'::jsonb,
    ALTER COLUMN reason
        SET DEFAULT 'unattributed export recorded by a binary predating slice 4.1d export attribution',
    ALTER COLUMN include_raw_payload
        SET DEFAULT false;

COMMENT ON COLUMN integration_session_exports.principal_json IS
    'Verified caller identity from the request security context. Never client-supplied. '
    'Defaults to the unattributed_legacy_export sentinel so a one-version binary rollback '
    'records a visibly unattributed disclosure instead of failing (slice 4.4a).';
COMMENT ON COLUMN integration_session_exports.reason IS
    'Operator-supplied disclosure reason, 1-1024 bytes, required. Defaults to the '
    'unattributed-rollback text for the same reason as principal_json (slice 4.4a).';
COMMENT ON COLUMN integration_session_exports.include_raw_payload IS
    'True only when the caller held integration.phi.export and asked for raw payloads. '
    'Defaults to false: an insert that cannot name the column must never imply a raw '
    'PHI disclosure (slice 4.4a).';
