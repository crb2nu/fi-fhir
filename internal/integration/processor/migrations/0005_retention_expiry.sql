-- Slice 4.1e: retention policy, expiry state, and the immutability exemption
-- that makes a purge possible without dissolving Slice 4.1d C1's guarantee.
--
-- Why this migration exists in this shape is recorded in `.loom/40-decisions.md`
-- (2026-08-08, "Slice 4.1e") and proved by
-- `internal/integration/retention/purge_gate_integration_test.go`, which passes
-- against the schema as it stood BEFORE this file:
--
--   * a DELETE of a canonical event raises (0004_audit_immutability.sql:29-32),
--     and even with the trigger lifted the ON DELETE RESTRICT chains from
--     integration_message_lineage and integration_delivery_attempts, both of
--     which are themselves undeletable, make row removal impossible; and
--   * the redaction UPDATE raises too, because C1's guard is blanket rather
--     than DELETE-scoped.
--
-- So a purge here is a TOMBSTONE, never a deletion, and the exemption that
-- permits it is column-scoped and schema-enforced rather than role-based —
-- C1's own reject_integration_receipt_provenance_mutation idiom
-- (0004_audit_immutability.sql:69-91) applied in reverse.
--
-- A tombstone is NOT a backup-inclusive deletion. The row, its identity, its
-- classification, and its recorded_at survive on purpose so an audit still
-- shows what existed, and a database backup taken before the purge still holds
-- the payload. Purge bounds retention in the live database only; backup-copy
-- expiry stays a storage-layer control. docs/operations/PHI-RETENTION.md says
-- this in the operator's own words.

-- ---------------------------------------------------------------------------
-- The retention policy record
-- ---------------------------------------------------------------------------
--
-- Correction 17 of `.loom/32-sprint4-execution-specs.md`: the policy belongs in
-- neither the revision contract nor deployment configuration.
--
--   * An integration revision is immutable and content-addressed, and the
--     retained data outlives it. Putting retention there would pin the policy to
--     the artifact that produced the data rather than to the tenant that owns
--     it, and every retention change would mint a revision and redeploy.
--   * Deployment configuration alone has no audit trail of who changed a PHI
--     retention window and why, and no per-tenant scope.
--
-- This record is therefore mutable, attributed, versioned, and per-tenant. It is
-- loaded from a deployment-supplied document the same way the destination
-- registry is, and every change writes an append-only audit row.
--
-- A NULL retain window means RETAIN INDEFINITELY for that class. An absent
-- policy row means the same for every class, so an unconfigured deployment
-- purges nothing. Fail-closed is the only safe default for a control whose
-- failure mode is destroying clinical data.
CREATE TABLE integration_retention_policies (
    tenant_id TEXT PRIMARY KEY,
    policy_version BIGINT NOT NULL CHECK (policy_version > 0),
    canonical_event_retain_seconds BIGINT CHECK (canonical_event_retain_seconds > 0),
    session_sample_retain_seconds BIGINT CHECK (session_sample_retain_seconds > 0),
    session_export_retain_seconds BIGINT CHECK (session_export_retain_seconds > 0),
    stream_event_retain_seconds BIGINT CHECK (stream_event_retain_seconds > 0),
    principal_json JSONB NOT NULL CHECK (jsonb_typeof(principal_json) = 'object'),
    reason TEXT NOT NULL CHECK (octet_length(reason) BETWEEN 1 AND 1024),
    document_digest TEXT NOT NULL CHECK (octet_length(document_digest) BETWEEN 1 AND 256),
    updated_at TIMESTAMPTZ NOT NULL
);

COMMENT ON TABLE integration_retention_policies IS
    'Slice 4.1e: mutable, attributed, per-tenant retention policy. A NULL window means retain indefinitely; an absent row means purge nothing.';

-- Every version of the policy that was ever in force, append-only. This is the
-- audit trail that deployment configuration alone could not provide.
CREATE TABLE integration_retention_policy_audit (
    audit_id BIGSERIAL PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    policy_version BIGINT NOT NULL CHECK (policy_version > 0),
    canonical_event_retain_seconds BIGINT,
    session_sample_retain_seconds BIGINT,
    session_export_retain_seconds BIGINT,
    stream_event_retain_seconds BIGINT,
    principal_json JSONB NOT NULL CHECK (jsonb_typeof(principal_json) = 'object'),
    reason TEXT NOT NULL CHECK (octet_length(reason) BETWEEN 1 AND 1024),
    document_digest TEXT NOT NULL,
    recorded_at TIMESTAMPTZ NOT NULL,
    UNIQUE (tenant_id, policy_version)
);

-- ---------------------------------------------------------------------------
-- The purge audit
-- ---------------------------------------------------------------------------
--
-- One append-only row per purged record, written in the SAME TRANSACTION as the
-- tombstone, so a purge with no audit row is impossible rather than merely
-- unlikely. The UNIQUE constraint is what makes "exactly one audit row per
-- record" a schema guarantee instead of a property of the sweeper: two replicas
-- racing produce one tombstone (the guarded UPDATE ... RETURNING is the claim)
-- and could not write a second audit row even if they tried.
--
-- record_id is the durable identifier that already exists in the durable set —
-- an event_id, a sample_id, an export_id. Nothing clinical is copied here.
--
-- integration_session_stream_events is deliberately absent from record_class.
-- It carries no PHI (0005_session_stream_events.sql:22-30 is an envelope log),
-- it is pruned rather than purged, and writing one audit row per pruned envelope
-- would replace one unbounded table with another.
CREATE TABLE integration_retention_purge_audit (
    audit_id BIGSERIAL PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    record_class TEXT NOT NULL CHECK (
        record_class IN ('canonical_event', 'session_sample', 'session_export')
    ),
    record_id TEXT NOT NULL CHECK (octet_length(record_id) BETWEEN 1 AND 512),
    policy_version BIGINT NOT NULL CHECK (policy_version > 0),
    purge_mode TEXT NOT NULL CHECK (purge_mode IN ('tombstone', 'deleted')),
    purge_after TIMESTAMPTZ NOT NULL,
    purged_at TIMESTAMPTZ NOT NULL,
    UNIQUE (tenant_id, record_class, record_id)
);

CREATE INDEX integration_retention_purge_audit_recent_idx
    ON integration_retention_purge_audit (tenant_id, purged_at, audit_id);

CREATE OR REPLACE FUNCTION reject_integration_retention_audit_mutation()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'integration retention audit records are append-only';
END;
$$;

DROP TRIGGER IF EXISTS integration_retention_purge_audit_immutable ON integration_retention_purge_audit;
CREATE TRIGGER integration_retention_purge_audit_immutable
    BEFORE UPDATE OR DELETE ON integration_retention_purge_audit
    FOR EACH ROW EXECUTE FUNCTION reject_integration_retention_audit_mutation();

DROP TRIGGER IF EXISTS integration_retention_policy_audit_immutable ON integration_retention_policy_audit;
CREATE TRIGGER integration_retention_policy_audit_immutable
    BEFORE UPDATE OR DELETE ON integration_retention_policy_audit
    FOR EACH ROW EXECUTE FUNCTION reject_integration_retention_audit_mutation();

-- ---------------------------------------------------------------------------
-- Expiry state on the canonical event
-- ---------------------------------------------------------------------------
--
-- Both columns are NULL-able and there is NO BACKFILL. A row admitted before any
-- policy existed has no policy, and inventing a purge_after for it here would be
-- the migration retroactively vouching for a retention decision nobody made —
-- the same reason 4.1b3 and C1 refused to backfill provenance. Such a row stays
-- unpurgeable until an operator writes an attributed policy record, at which
-- point the purge component stamps it under that operator's authority.
ALTER TABLE integration_canonical_events
    ADD COLUMN purge_after TIMESTAMPTZ,
    ADD COLUMN purged_at TIMESTAMPTZ;

COMMENT ON COLUMN integration_canonical_events.purge_after IS
    'Slice 4.1e: the effective expiry deadline stamped from the tenant retention policy. NULL means no policy applies and the row is never purged.';
COMMENT ON COLUMN integration_canonical_events.purged_at IS
    'Slice 4.1e: server-owned instant the payload was tombstoned. NULL means the payload is intact.';

CREATE INDEX integration_canonical_events_purge_idx
    ON integration_canonical_events (purge_after)
    WHERE purged_at IS NULL;

-- The canonical tombstone. One definition, referenced by the trigger and read by
-- the application, so the two can never drift into disagreeing about what a
-- tombstone is.
CREATE OR REPLACE FUNCTION integration_canonical_event_tombstone()
RETURNS jsonb LANGUAGE sql IMMUTABLE AS $$
    SELECT '{"purged": true, "purge_schema": "fi-fhir.retention.tombstone.v1"}'::jsonb
$$;

-- ---------------------------------------------------------------------------
-- The exemption
-- ---------------------------------------------------------------------------

-- Deletion stays blanket-blocked. The message keeps the word "append-only" that
-- C1's kill-test matches on, because nothing about deletion changed.
CREATE OR REPLACE FUNCTION reject_integration_canonical_event_delete()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'integration canonical events are append-only; a retention purge tombstones the payload and never deletes the row';
END;
$$;

-- The UPDATE guard permits exactly two shapes and raises on everything else.
-- Read it as security code, not as schema boilerplate.
CREATE OR REPLACE FUNCTION reject_integration_canonical_event_mutation()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    -- Identity, provenance, and classification are frozen unconditionally, in
    -- every shape. A purge changes what the row CONTAINED, never what it WAS.
    IF NEW.tenant_id IS DISTINCT FROM OLD.tenant_id
        OR NEW.event_id IS DISTINCT FROM OLD.event_id
        OR NEW.receipt_id IS DISTINCT FROM OLD.receipt_id
        OR NEW.event_type IS DISTINCT FROM OLD.event_type
        OR NEW.source_message_id IS DISTINCT FROM OLD.source_message_id
        OR NEW.correlation_id IS DISTINCT FROM OLD.correlation_id
        OR NEW.classification IS DISTINCT FROM OLD.classification
        OR NEW.recorded_at IS DISTINCT FROM OLD.recorded_at
    THEN
        RAISE EXCEPTION 'integration canonical events are append-only outside a retention purge tombstone';
    END IF;

    -- Shape 1: stamping the expiry deadline the tenant's retention policy
    -- computes. The payload does not move, so no PHI is touched.
    IF NEW.payload_json IS NOT DISTINCT FROM OLD.payload_json
        AND NEW.purged_at IS NOT DISTINCT FROM OLD.purged_at
    THEN
        IF OLD.purged_at IS NOT NULL THEN
            RAISE EXCEPTION 'integration canonical events are append-only once purged';
        END IF;
        RETURN NEW;
    END IF;

    -- Shape 2: the retention purge tombstone. Once, in place, and nothing else.
    IF OLD.purged_at IS NOT NULL THEN
        RAISE EXCEPTION 'integration canonical events are append-only once purged';
    END IF;
    IF NEW.purge_after IS DISTINCT FROM OLD.purge_after THEN
        RAISE EXCEPTION 'integration canonical events are append-only outside a retention purge tombstone';
    END IF;
    IF NEW.purged_at IS NULL THEN
        RAISE EXCEPTION 'integration canonical events are append-only outside a retention purge tombstone';
    END IF;
    IF NEW.payload_json IS DISTINCT FROM integration_canonical_event_tombstone() THEN
        RAISE EXCEPTION 'integration canonical events are append-only outside a retention purge tombstone';
    END IF;
    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS integration_canonical_events_immutable ON integration_canonical_events;

DROP TRIGGER IF EXISTS integration_canonical_events_undeletable ON integration_canonical_events;
CREATE TRIGGER integration_canonical_events_undeletable
    BEFORE DELETE ON integration_canonical_events
    FOR EACH ROW EXECUTE FUNCTION reject_integration_canonical_event_delete();

DROP TRIGGER IF EXISTS integration_canonical_events_purge_only ON integration_canonical_events;
CREATE TRIGGER integration_canonical_events_purge_only
    BEFORE UPDATE ON integration_canonical_events
    FOR EACH ROW EXECUTE FUNCTION reject_integration_canonical_event_mutation();

COMMENT ON TRIGGER integration_canonical_events_purge_only ON integration_canonical_events IS
    'Slice 4.1e: permits only the retention-policy expiry stamp and the one-time canonical tombstone. Every other UPDATE raises.';
