-- Slice 4.1d C1: a session export is a PHI disclosure, so it must name who
-- exported, why, and whether raw payloads were included.
--
-- Before this migration integration_session_exports carried only
-- (tenant_id, session_id, export_id, exported_at, record_json) — no principal
-- and no reason — which contradicts the product spec's requirement that data
-- export record actor, reason, timestamp, and revision
-- (.loom/20-product-spec-integration-engine-ide-completion.md:220-222).
--
-- The reason CHECK matches the delivery-operations convention
-- (internal/integration/processor/migrations/0002_delivery_reliability.sql:107-108)
-- and principal_json matches the receipt/audit convention of a JSON object
-- holding the verified integration.Principal.
--
-- Pre-migration rows cannot be attributed after the fact: backfilling a
-- principal would be retroactively vouching for a disclosure nobody recorded.
-- They are instead marked with an explicit unattributed sentinel so they stay
-- visibly distinguishable, following 4.1b3's provenance idiom.

ALTER TABLE integration_session_exports
    ADD COLUMN principal_json JSONB,
    ADD COLUMN reason TEXT,
    ADD COLUMN include_raw_payload BOOLEAN;

UPDATE integration_session_exports
SET principal_json = '{"id": "", "kind": "", "auth_method": "", "unattributed_legacy_export": true}'::jsonb,
    reason = 'unattributed export recorded before slice 4.1d export attribution',
    include_raw_payload = false
WHERE principal_json IS NULL;

ALTER TABLE integration_session_exports
    ALTER COLUMN principal_json SET NOT NULL,
    ALTER COLUMN reason SET NOT NULL,
    ALTER COLUMN include_raw_payload SET NOT NULL,
    ADD CONSTRAINT integration_session_exports_principal_shape_check
        CHECK (jsonb_typeof(principal_json) = 'object'),
    ADD CONSTRAINT integration_session_exports_reason_check
        CHECK (octet_length(reason) BETWEEN 1 AND 1024);

COMMENT ON COLUMN integration_session_exports.principal_json IS
    'Verified caller identity from the request security context. Never client-supplied.';
COMMENT ON COLUMN integration_session_exports.reason IS
    'Operator-supplied disclosure reason, 1-1024 bytes, required.';
COMMENT ON COLUMN integration_session_exports.include_raw_payload IS
    'True only when the caller held integration.phi.export and asked for raw payloads.';

-- An export record is evidence of a disclosure. It is append-only.
CREATE OR REPLACE FUNCTION reject_integration_session_export_mutation()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'integration session exports are append-only';
END;
$$;

DROP TRIGGER IF EXISTS integration_session_exports_immutable ON integration_session_exports;
CREATE TRIGGER integration_session_exports_immutable
    BEFORE UPDATE OR DELETE ON integration_session_exports
    FOR EACH ROW EXECUTE FUNCTION reject_integration_session_export_mutation();
