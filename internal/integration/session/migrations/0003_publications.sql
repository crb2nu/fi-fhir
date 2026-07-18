CREATE TABLE integration_session_publications (
    tenant_id TEXT NOT NULL,
    session_id TEXT NOT NULL,
    publication_id TEXT NOT NULL,
    version BIGINT NOT NULL CHECK (version > 0),
    definition_id TEXT NOT NULL,
    definition_revision_id TEXT NOT NULL,
    workflow_simulation_id TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    record_json JSONB NOT NULL,
    manifest_bytes BYTEA NOT NULL,
    signature_bytes BYTEA NOT NULL,
    PRIMARY KEY (tenant_id, publication_id),
    UNIQUE (tenant_id, session_id, version),
    FOREIGN KEY (tenant_id, session_id)
        REFERENCES integration_sessions (tenant_id, session_id),
    FOREIGN KEY (tenant_id, workflow_simulation_id)
        REFERENCES integration_session_workflow_simulations (tenant_id, simulation_id)
);

CREATE INDEX integration_session_publications_list_idx
    ON integration_session_publications
        (tenant_id, session_id, version, publication_id);

CREATE OR REPLACE FUNCTION reject_integration_session_publication_mutation()
RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'integration session publications are append-only';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER integration_session_publications_immutable
    BEFORE UPDATE OR DELETE ON integration_session_publications
    FOR EACH ROW EXECUTE FUNCTION reject_integration_session_publication_mutation();

CREATE OR REPLACE FUNCTION reject_integration_session_workflow_simulation_mutation()
RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'integration session workflow simulations are append-only';
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS integration_session_workflow_simulations_immutable
    ON integration_session_workflow_simulations;
CREATE TRIGGER integration_session_workflow_simulations_immutable
    BEFORE UPDATE OR DELETE ON integration_session_workflow_simulations
    FOR EACH ROW EXECUTE FUNCTION reject_integration_session_workflow_simulation_mutation();
