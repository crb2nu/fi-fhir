-- .loom/38 Lane C-2: what the sample-intake behaviour needs from the capture
-- audit table Lane C-0 shipped (0001_connection_catalog.sql).
--
--   * problems_json records why a peek or capture finished as it did — an
--     unresolvable secret binding, an unreachable source, a session write that
--     failed. It is written by the same expected-version update that finishes
--     the row, so the existing guard (a finished capture is frozen) covers it
--     from then on. NOT NULL with a DEFAULT, so an N-1 binary's INSERT, which
--     does not name it, still succeeds (AGENTS.md, "Migration authoring", rule 1).
--
--   * cancellation_json records who cancelled an armed capture and why. The
--     row's own reason is the request's, and it is frozen.
--
--   * integration_connection_captures_one_armed_stream makes "at most one armed
--     stream capture per source" a schema fact rather than a check-then-insert
--     race: the admission-time tap fills one capture per frame, so two armed
--     captures of one source would leave one of them starved.
--
--   * object_path records which object a peek read. It is provenance, so it is
--     frozen from the INSERT on, by a trigger of its own beside C-0's guard
--     (which predates the column). The sample a peek writes names only this
--     row ("peek:<capture id>"), never the path. NOT NULL with a DEFAULT, for
--     the same N-1 reason as problems_json.

ALTER TABLE integration_connection_captures
    ADD COLUMN problems_json JSONB NOT NULL DEFAULT '[]'::jsonb;

ALTER TABLE integration_connection_captures
    ADD COLUMN cancellation_json JSONB;

ALTER TABLE integration_connection_captures
    ADD CONSTRAINT integration_connection_captures_problems_are_array
        CHECK (jsonb_typeof(problems_json) = 'array');

CREATE UNIQUE INDEX integration_connection_captures_one_armed_stream
    ON integration_connection_captures (tenant_id, source_id)
    WHERE status = 'armed' AND mode = 'stream';

ALTER TABLE integration_connection_captures
    ADD COLUMN object_path TEXT NOT NULL DEFAULT '';

ALTER TABLE integration_connection_captures
    ADD CONSTRAINT integration_connection_captures_object_path_bounded
        CHECK (octet_length(object_path) <= 1024 AND (mode = 'peek' OR object_path = ''));

CREATE OR REPLACE FUNCTION reject_integration_connection_capture_object_path_change()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.object_path IS DISTINCT FROM OLD.object_path THEN
        RAISE EXCEPTION 'integration connection capture provenance is immutable';
    END IF;
    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS integration_connection_captures_object_path_frozen ON integration_connection_captures;
CREATE TRIGGER integration_connection_captures_object_path_frozen
    BEFORE UPDATE ON integration_connection_captures
    FOR EACH ROW EXECUTE FUNCTION reject_integration_connection_capture_object_path_change();

COMMENT ON TRIGGER integration_connection_captures_object_path_frozen ON integration_connection_captures IS
    '.loom/38 C-2: a peek''s object_path is provenance, frozen from the INSERT on.';
