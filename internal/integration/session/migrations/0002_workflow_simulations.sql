CREATE TABLE integration_session_workflow_simulations (
    tenant_id TEXT NOT NULL,
    session_id TEXT NOT NULL,
    simulation_id TEXT NOT NULL,
    workflow_revision_id TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    record_json JSONB NOT NULL,
    PRIMARY KEY (tenant_id, simulation_id),
    FOREIGN KEY (tenant_id, session_id)
        REFERENCES integration_sessions (tenant_id, session_id),
    FOREIGN KEY (tenant_id, workflow_revision_id)
        REFERENCES integration_session_artifact_revisions (tenant_id, revision_id)
);

CREATE INDEX integration_session_workflow_simulations_list_idx
    ON integration_session_workflow_simulations
        (tenant_id, session_id, created_at, simulation_id);
