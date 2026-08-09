-- Slice 4.1e: expiry state and purge exemptions for the session workspace.
--
-- Three tables, three different answers, because they are three different kinds
-- of thing. See `.loom/40-decisions.md` (2026-08-08, "Slice 4.1e") and
-- docs/operations/PHI-RETENTION.md sections 3 and 6.
--
--   * integration_session_samples holds PHI and carries NO immutability trigger
--     (correction 14). The honest purge is deletion of the row and its
--     raw_cipher. Giving it a tombstone shape would invent a guarantee the table
--     never had.
--   * integration_session_exports is evidence of a disclosure. Slice 4.1d C1
--     made it append-only and its foreign key makes the exported session itself
--     undeletable (correction 13), so the purge is a tombstone of the snapshot,
--     column-scoped exactly like the canonical event's.
--   * integration_session_stream_events carries no PHI at all — it is an
--     envelope log (0005_session_stream_events.sql:22-30) — but it is unbounded
--     and nothing prunes it (correction 15). It is PRUNED, not purged.
--
-- Every new column is NULL-able and there is NO BACKFILL, for the reason
-- 0005_retention_expiry.sql on the processor side states in full: a row admitted
-- before any policy existed has no policy, and inventing one here would be the
-- migration vouching for a retention decision nobody made.

-- ---------------------------------------------------------------------------
-- Session samples: deletable outright
-- ---------------------------------------------------------------------------

ALTER TABLE integration_session_samples
    ADD COLUMN purge_after TIMESTAMPTZ;

COMMENT ON COLUMN integration_session_samples.purge_after IS
    'Slice 4.1e: expiry deadline stamped from the tenant retention policy. NULL means no policy applies. The row is deleted outright on purge, ciphertext included.';

CREATE INDEX integration_session_samples_purge_idx
    ON integration_session_samples (purge_after)
    WHERE purge_after IS NOT NULL;

-- ---------------------------------------------------------------------------
-- Session exports: tombstoned, never deleted
-- ---------------------------------------------------------------------------

ALTER TABLE integration_session_exports
    ADD COLUMN purge_after TIMESTAMPTZ,
    ADD COLUMN purged_at TIMESTAMPTZ;

COMMENT ON COLUMN integration_session_exports.purge_after IS
    'Slice 4.1e: expiry deadline for the export SNAPSHOT. The disclosure record itself — who, why, when — is never purged.';
COMMENT ON COLUMN integration_session_exports.purged_at IS
    'Slice 4.1e: server-owned instant record_json was tombstoned. NULL means the snapshot is intact.';

CREATE INDEX integration_session_exports_purge_idx
    ON integration_session_exports (purge_after)
    WHERE purged_at IS NULL;

CREATE OR REPLACE FUNCTION integration_session_export_tombstone()
RETURNS jsonb LANGUAGE sql IMMUTABLE AS $$
    SELECT '{"purged": true, "purge_schema": "fi-fhir.retention.tombstone.v1"}'::jsonb
$$;

CREATE OR REPLACE FUNCTION reject_integration_session_export_delete()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'integration session exports are append-only; a retention purge tombstones the snapshot and never deletes the disclosure record';
END;
$$;

-- The attribution — principal_json, reason, include_raw_payload, exported_at —
-- is what makes this row evidence, and it stays frozen in every shape. Only the
-- snapshot payload can be tombstoned, and only once.
CREATE OR REPLACE FUNCTION reject_integration_session_export_mutation()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.tenant_id IS DISTINCT FROM OLD.tenant_id
        OR NEW.session_id IS DISTINCT FROM OLD.session_id
        OR NEW.export_id IS DISTINCT FROM OLD.export_id
        OR NEW.exported_at IS DISTINCT FROM OLD.exported_at
        OR NEW.principal_json IS DISTINCT FROM OLD.principal_json
        OR NEW.reason IS DISTINCT FROM OLD.reason
        OR NEW.include_raw_payload IS DISTINCT FROM OLD.include_raw_payload
    THEN
        RAISE EXCEPTION 'integration session exports are append-only outside a retention purge tombstone';
    END IF;

    -- Shape 1: stamping the expiry deadline. The snapshot does not move.
    IF NEW.record_json IS NOT DISTINCT FROM OLD.record_json
        AND NEW.purged_at IS NOT DISTINCT FROM OLD.purged_at
    THEN
        IF OLD.purged_at IS NOT NULL THEN
            RAISE EXCEPTION 'integration session exports are append-only once purged';
        END IF;
        RETURN NEW;
    END IF;

    -- Shape 2: the tombstone.
    IF OLD.purged_at IS NOT NULL THEN
        RAISE EXCEPTION 'integration session exports are append-only once purged';
    END IF;
    IF NEW.purge_after IS DISTINCT FROM OLD.purge_after THEN
        RAISE EXCEPTION 'integration session exports are append-only outside a retention purge tombstone';
    END IF;
    IF NEW.purged_at IS NULL THEN
        RAISE EXCEPTION 'integration session exports are append-only outside a retention purge tombstone';
    END IF;
    IF NEW.record_json IS DISTINCT FROM integration_session_export_tombstone() THEN
        RAISE EXCEPTION 'integration session exports are append-only outside a retention purge tombstone';
    END IF;
    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS integration_session_exports_immutable ON integration_session_exports;

DROP TRIGGER IF EXISTS integration_session_exports_undeletable ON integration_session_exports;
CREATE TRIGGER integration_session_exports_undeletable
    BEFORE DELETE ON integration_session_exports
    FOR EACH ROW EXECUTE FUNCTION reject_integration_session_export_delete();

DROP TRIGGER IF EXISTS integration_session_exports_purge_only ON integration_session_exports;
CREATE TRIGGER integration_session_exports_purge_only
    BEFORE UPDATE ON integration_session_exports
    FOR EACH ROW EXECUTE FUNCTION reject_integration_session_export_mutation();

COMMENT ON TRIGGER integration_session_exports_purge_only ON integration_session_exports IS
    'Slice 4.1e: permits only the retention-policy expiry stamp and the one-time snapshot tombstone. The disclosure attribution stays frozen.';

-- ---------------------------------------------------------------------------
-- The fanout log: pruned, with a schema floor
-- ---------------------------------------------------------------------------
--
-- Correction 15 asked this lane to decide rather than leave it: the answer is
-- that integration_session_stream_events IS pruned, on the retention policy's
-- stream window, and that the schema enforces a floor no deployment can lower.
--
-- The floor exists because the log is a resume cursor: a subscriber that has
-- been away longer than the window sees a gap, which the table's own comment
-- already documents as the expected replica-flip behaviour, but a misconfigured
-- one-minute window would turn every reconnect into a gap. Twenty-four hours is
-- far longer than any SSE reconnect and far shorter than unbounded growth.
--
-- UPDATE stays blanket-blocked: nothing may ever rewrite an envelope.
CREATE OR REPLACE FUNCTION reject_integration_session_stream_event_update()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'integration session stream events are append-only';
END;
$$;

CREATE OR REPLACE FUNCTION reject_integration_session_stream_event_delete()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF OLD.created_at > clock_timestamp() - interval '24 hours' THEN
        RAISE EXCEPTION 'integration session stream events are append-only until they age past the 24 hour prune floor';
    END IF;
    RETURN OLD;
END;
$$;

DROP TRIGGER IF EXISTS integration_session_stream_events_immutable ON integration_session_stream_events;

DROP TRIGGER IF EXISTS integration_session_stream_events_append_only ON integration_session_stream_events;
CREATE TRIGGER integration_session_stream_events_append_only
    BEFORE UPDATE ON integration_session_stream_events
    FOR EACH ROW EXECUTE FUNCTION reject_integration_session_stream_event_update();

DROP TRIGGER IF EXISTS integration_session_stream_events_prunable ON integration_session_stream_events;
CREATE TRIGGER integration_session_stream_events_prunable
    BEFORE DELETE ON integration_session_stream_events
    FOR EACH ROW EXECUTE FUNCTION reject_integration_session_stream_event_delete();

CREATE INDEX integration_session_stream_events_prune_idx
    ON integration_session_stream_events (created_at, seq);

COMMENT ON TRIGGER integration_session_stream_events_prunable ON integration_session_stream_events IS
    'Slice 4.1e: the fanout log is pruned, never rewritten. Rows younger than the 24 hour schema floor cannot be deleted by any deployment window.';
