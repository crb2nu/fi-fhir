CREATE TABLE integration_definition_revisions (
    tenant_id TEXT NOT NULL,
    definition_id TEXT NOT NULL,
    revision_id TEXT NOT NULL,
    digest TEXT NOT NULL,
    revision_json JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (tenant_id, definition_id, revision_id),
    UNIQUE (tenant_id, definition_id, revision_id, digest)
);

CREATE TABLE integration_connection_validations (
    validation_id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    definition_id TEXT NOT NULL,
    revision_id TEXT NOT NULL,
    revision_digest TEXT NOT NULL,
    source_revision JSONB NOT NULL,
    passed BOOLEAN NOT NULL,
    codes JSONB NOT NULL,
    checked_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    audit_json JSONB NOT NULL,
    FOREIGN KEY (tenant_id, definition_id, revision_id, revision_digest)
        REFERENCES integration_definition_revisions (tenant_id, definition_id, revision_id, digest)
);

CREATE TABLE integration_release_records (
    release_id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    definition_id TEXT NOT NULL,
    revision_id TEXT NOT NULL,
    revision_digest TEXT NOT NULL,
    validation_id TEXT NOT NULL REFERENCES integration_connection_validations(validation_id),
    approval_event_id TEXT NOT NULL,
    published_json JSONB NOT NULL,
    published_at TIMESTAMPTZ NOT NULL,
    release_digest TEXT NOT NULL UNIQUE,
    FOREIGN KEY (tenant_id, definition_id, revision_id, revision_digest)
        REFERENCES integration_definition_revisions (tenant_id, definition_id, revision_id, digest)
);

CREATE TABLE integration_lifecycle_snapshots (
    tenant_id TEXT NOT NULL,
    definition_id TEXT NOT NULL,
    revision_id TEXT NOT NULL,
    revision_digest TEXT NOT NULL,
    state TEXT NOT NULL CHECK (state IN ('draft', 'validated', 'approved', 'published', 'deployed', 'paused', 'retired')),
    version BIGINT NOT NULL CHECK (version > 0),
    release_id TEXT REFERENCES integration_release_records(release_id),
    health TEXT NOT NULL CHECK (health IN ('unknown', 'starting', 'healthy', 'degraded', 'unhealthy')),
    last_validation_id TEXT REFERENCES integration_connection_validations(validation_id),
    validation_passed BOOLEAN NOT NULL DEFAULT FALSE,
    validation_checked_at TIMESTAMPTZ,
    validation_expires_at TIMESTAMPTZ,
    approval_event_id TEXT,
    updated_json JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (tenant_id, definition_id, revision_id),
    FOREIGN KEY (tenant_id, definition_id, revision_id, revision_digest)
        REFERENCES integration_definition_revisions (tenant_id, definition_id, revision_id, digest)
);

CREATE UNIQUE INDEX integration_one_active_deployment_per_definition
    ON integration_lifecycle_snapshots (tenant_id, definition_id)
    WHERE state IN ('deployed', 'paused');

CREATE TABLE integration_lifecycle_events (
    event_id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    definition_id TEXT NOT NULL,
    revision_id TEXT NOT NULL,
    revision_digest TEXT NOT NULL,
    version BIGINT NOT NULL,
    action TEXT NOT NULL,
    from_state TEXT,
    to_state TEXT NOT NULL,
    health TEXT NOT NULL,
    release_id TEXT REFERENCES integration_release_records(release_id),
    audit_json JSONB NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL,
    UNIQUE (tenant_id, definition_id, revision_id, version),
    FOREIGN KEY (tenant_id, definition_id, revision_id, revision_digest)
        REFERENCES integration_definition_revisions (tenant_id, definition_id, revision_id, digest)
);

CREATE OR REPLACE FUNCTION reject_integration_lifecycle_mutation()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'integration lifecycle records are append-only';
END;
$$;

CREATE TRIGGER integration_definition_revisions_immutable
    BEFORE UPDATE OR DELETE ON integration_definition_revisions
    FOR EACH ROW EXECUTE FUNCTION reject_integration_lifecycle_mutation();
CREATE TRIGGER integration_connection_validations_immutable
    BEFORE UPDATE OR DELETE ON integration_connection_validations
    FOR EACH ROW EXECUTE FUNCTION reject_integration_lifecycle_mutation();
CREATE TRIGGER integration_release_records_immutable
    BEFORE UPDATE OR DELETE ON integration_release_records
    FOR EACH ROW EXECUTE FUNCTION reject_integration_lifecycle_mutation();
CREATE TRIGGER integration_lifecycle_events_immutable
    BEFORE UPDATE OR DELETE ON integration_lifecycle_events
    FOR EACH ROW EXECUTE FUNCTION reject_integration_lifecycle_mutation();
