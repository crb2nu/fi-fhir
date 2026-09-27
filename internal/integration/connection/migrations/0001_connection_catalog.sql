-- .loom/38 Lane C-0: the connection catalog. This is the seventh forward-only
-- ledger (integration_connection_schema_migrations), with its own advisory lock
-- key, so it claims no number in any other package's sequence.
--
-- Three tables, three guard shapes, in the 0004_audit_immutability.sql idiom:
--
--   * integration_connection_drafts is a STATE table. A row changes only under
--     an expected-version update (version advances by exactly one); its
--     identity and creation audit never change; an archived row is frozen; and
--     no row is ever deleted — archive is the only retirement.
--
--   * integration_connection_revisions is an append-only ledger. Each row is one
--     compile: the exact document bytes the constructor produced
--     (revision_text, returned verbatim as `revisionJson`), the same document as
--     JSONB for querying (revision_json, checked equal), and its digest. JSONB
--     alone could not return the exact bytes: it reorders keys.
--
--   * integration_connection_captures is the peek/capture audit Lane C-2 writes.
--     It is append-only except status, captured, completed_at, and version,
--     which advance under an expected-version update while the capture is
--     armed; a finished capture is frozen.
--
-- Row-level triggers do not affect DDL, so the integration suites' schema
-- teardown (DROP SCHEMA ... CASCADE) is unaffected.

CREATE TABLE integration_connection_drafts (
    tenant_id TEXT NOT NULL,
    artifact_id TEXT NOT NULL,
    direction TEXT NOT NULL CHECK (direction IN ('source', 'destination')),
    kind TEXT NOT NULL CHECK (kind IN ('mllp', 'http', 'batch_s3', 'batch_sftp', 'https', 'fhir', 'kafka')),
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    spec_json JSONB NOT NULL,
    secret_bindings_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    version BIGINT NOT NULL CHECK (version > 0),
    archived_at TIMESTAMPTZ,
    created_json JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_json JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (tenant_id, artifact_id),
    CONSTRAINT integration_connection_drafts_direction_matches_kind CHECK (
        (direction = 'source' AND kind IN ('mllp', 'http', 'batch_s3', 'batch_sftp'))
        OR (direction = 'destination' AND kind IN ('https', 'fhir', 'kafka'))
    ),
    CONSTRAINT integration_connection_drafts_spec_is_object CHECK (jsonb_typeof(spec_json) = 'object'),
    CONSTRAINT integration_connection_drafts_bindings_are_array CHECK (jsonb_typeof(secret_bindings_json) = 'array')
);

CREATE INDEX integration_connection_drafts_by_direction
    ON integration_connection_drafts (tenant_id, direction, artifact_id);

CREATE TABLE integration_connection_revisions (
    tenant_id TEXT NOT NULL,
    artifact_id TEXT NOT NULL,
    revision_id TEXT NOT NULL,
    revision_number BIGINT NOT NULL CHECK (revision_number > 0),
    digest TEXT NOT NULL CHECK (digest ~ '^sha256:[0-9a-f]{64}$'),
    direction TEXT NOT NULL CHECK (direction IN ('source', 'destination')),
    kind TEXT NOT NULL CHECK (kind IN ('mllp', 'http', 'batch_s3', 'batch_sftp', 'https', 'fhir', 'kafka')),
    revision_json JSONB NOT NULL,
    revision_text TEXT NOT NULL,
    compiled_from_version BIGINT NOT NULL CHECK (compiled_from_version > 0),
    created_json JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (tenant_id, artifact_id, revision_id),
    UNIQUE (tenant_id, artifact_id, revision_number),
    UNIQUE (tenant_id, digest),
    FOREIGN KEY (tenant_id, artifact_id) REFERENCES integration_connection_drafts (tenant_id, artifact_id),
    CONSTRAINT integration_connection_revisions_id_is_number CHECK (revision_id = revision_number::text),
    CONSTRAINT integration_connection_revisions_text_matches_json CHECK (revision_text::jsonb = revision_json)
);

CREATE TABLE integration_connection_captures (
    tenant_id TEXT NOT NULL,
    capture_id TEXT NOT NULL,
    session_id TEXT NOT NULL,
    mode TEXT NOT NULL CHECK (mode IN ('peek', 'stream')),
    source_id TEXT NOT NULL DEFAULT '',
    connection_artifact_id TEXT NOT NULL DEFAULT '',
    connection_digest TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL CHECK (status IN ('armed', 'complete', 'expired', 'cancelled', 'failed')),
    captured INTEGER NOT NULL DEFAULT 0,
    max_messages INTEGER NOT NULL CHECK (max_messages BETWEEN 1 AND 100),
    version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    principal_json JSONB NOT NULL,
    reason TEXT NOT NULL CHECK (btrim(reason) <> '' AND octet_length(reason) <= 1024),
    requested_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    completed_at TIMESTAMPTZ,
    PRIMARY KEY (tenant_id, capture_id),
    CONSTRAINT integration_connection_captures_count_bounded CHECK (captured BETWEEN 0 AND max_messages),
    CONSTRAINT integration_connection_captures_expiry_after_request CHECK (expires_at > requested_at),
    CONSTRAINT integration_connection_captures_target_named CHECK (
        (mode = 'stream' AND source_id <> '')
        OR (mode = 'peek' AND connection_artifact_id <> '' AND connection_digest <> '')
    ),
    CONSTRAINT integration_connection_captures_completion_matches_status CHECK (
        (status = 'armed') = (completed_at IS NULL)
    )
);

CREATE INDEX integration_connection_captures_by_session
    ON integration_connection_captures (tenant_id, session_id, requested_at DESC);

-- The armed-capture cache (Lane C-2) reads only armed rows, every two seconds
-- per replica.
CREATE INDEX integration_connection_captures_armed
    ON integration_connection_captures (tenant_id, source_id)
    WHERE status = 'armed';

-- Drafts: identity and creation audit frozen, archived rows frozen, every
-- change an expected-version update, no delete.
CREATE OR REPLACE FUNCTION reject_integration_connection_draft_mutation()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'integration connection drafts cannot be deleted; archive them instead';
    END IF;
    IF NEW.tenant_id IS DISTINCT FROM OLD.tenant_id
        OR NEW.artifact_id IS DISTINCT FROM OLD.artifact_id
        OR NEW.direction IS DISTINCT FROM OLD.direction
        OR NEW.kind IS DISTINCT FROM OLD.kind
        OR NEW.created_json IS DISTINCT FROM OLD.created_json
        OR NEW.created_at IS DISTINCT FROM OLD.created_at
    THEN
        RAISE EXCEPTION 'integration connection draft identity and creation audit are immutable';
    END IF;
    IF OLD.archived_at IS NOT NULL THEN
        RAISE EXCEPTION 'an archived integration connection draft is frozen';
    END IF;
    IF NEW.version IS DISTINCT FROM OLD.version + 1 THEN
        RAISE EXCEPTION 'integration connection drafts change only under an expected-version update';
    END IF;
    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS integration_connection_drafts_guarded ON integration_connection_drafts;
CREATE TRIGGER integration_connection_drafts_guarded
    BEFORE UPDATE OR DELETE ON integration_connection_drafts
    FOR EACH ROW EXECUTE FUNCTION reject_integration_connection_draft_mutation();

-- Revisions: append-only.
CREATE OR REPLACE FUNCTION reject_integration_connection_revision_mutation()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'integration connection revisions are append-only';
END;
$$;

DROP TRIGGER IF EXISTS integration_connection_revisions_immutable ON integration_connection_revisions;
CREATE TRIGGER integration_connection_revisions_immutable
    BEFORE UPDATE OR DELETE ON integration_connection_revisions
    FOR EACH ROW EXECUTE FUNCTION reject_integration_connection_revision_mutation();

-- Captures: provenance frozen, a finished capture frozen, the count never
-- decreasing, every change an expected-version update, no delete.
CREATE OR REPLACE FUNCTION reject_integration_connection_capture_mutation()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'integration connection capture audit records cannot be deleted';
    END IF;
    IF NEW.tenant_id IS DISTINCT FROM OLD.tenant_id
        OR NEW.capture_id IS DISTINCT FROM OLD.capture_id
        OR NEW.session_id IS DISTINCT FROM OLD.session_id
        OR NEW.mode IS DISTINCT FROM OLD.mode
        OR NEW.source_id IS DISTINCT FROM OLD.source_id
        OR NEW.connection_artifact_id IS DISTINCT FROM OLD.connection_artifact_id
        OR NEW.connection_digest IS DISTINCT FROM OLD.connection_digest
        OR NEW.max_messages IS DISTINCT FROM OLD.max_messages
        OR NEW.principal_json IS DISTINCT FROM OLD.principal_json
        OR NEW.reason IS DISTINCT FROM OLD.reason
        OR NEW.requested_at IS DISTINCT FROM OLD.requested_at
        OR NEW.expires_at IS DISTINCT FROM OLD.expires_at
    THEN
        RAISE EXCEPTION 'integration connection capture provenance is immutable';
    END IF;
    IF OLD.status <> 'armed' THEN
        RAISE EXCEPTION 'a finished integration connection capture is frozen';
    END IF;
    IF NEW.version IS DISTINCT FROM OLD.version + 1 THEN
        RAISE EXCEPTION 'integration connection captures advance only under an expected-version update';
    END IF;
    IF NEW.captured < OLD.captured THEN
        RAISE EXCEPTION 'an integration connection capture count never decreases';
    END IF;
    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS integration_connection_captures_guarded ON integration_connection_captures;
CREATE TRIGGER integration_connection_captures_guarded
    BEFORE UPDATE OR DELETE ON integration_connection_captures
    FOR EACH ROW EXECUTE FUNCTION reject_integration_connection_capture_mutation();

COMMENT ON TRIGGER integration_connection_drafts_guarded ON integration_connection_drafts IS
    '.loom/38 C-0: identity and creation audit frozen; expected-version updates only; archived rows frozen; no delete.';
COMMENT ON TRIGGER integration_connection_revisions_immutable ON integration_connection_revisions IS
    '.loom/38 C-0: compiled connection revisions are append-only.';
COMMENT ON TRIGGER integration_connection_captures_guarded ON integration_connection_captures IS
    '.loom/38 C-0: capture provenance frozen; status/captured/completed_at advance under an expected-version update while armed.';
