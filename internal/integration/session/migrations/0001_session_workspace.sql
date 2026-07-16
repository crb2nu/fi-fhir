CREATE TABLE integration_sessions (
    tenant_id TEXT NOT NULL,
    session_id TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('active', 'archived')),
    created_at TIMESTAMPTZ NOT NULL,
    record_json JSONB NOT NULL,
    PRIMARY KEY (tenant_id, session_id)
);

CREATE INDEX integration_sessions_list_idx
    ON integration_sessions (tenant_id, status, created_at, session_id);

CREATE TABLE integration_session_samples (
    tenant_id TEXT NOT NULL,
    session_id TEXT NOT NULL,
    sample_id TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    record_json JSONB NOT NULL,
    raw_cipher BYTEA,
    PRIMARY KEY (tenant_id, sample_id),
    FOREIGN KEY (tenant_id, session_id)
        REFERENCES integration_sessions (tenant_id, session_id)
);

CREATE INDEX integration_session_samples_list_idx
    ON integration_session_samples (tenant_id, session_id, created_at, sample_id);

CREATE TABLE integration_session_artifact_revisions (
    tenant_id TEXT NOT NULL,
    session_id TEXT NOT NULL,
    artifact_id TEXT NOT NULL,
    revision_id TEXT NOT NULL,
    version BIGINT NOT NULL CHECK (version > 0),
    kind TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    record_json JSONB NOT NULL,
    content_bytes BYTEA NOT NULL,
    PRIMARY KEY (tenant_id, revision_id),
    UNIQUE (tenant_id, artifact_id, version),
    FOREIGN KEY (tenant_id, session_id)
        REFERENCES integration_sessions (tenant_id, session_id)
);

CREATE INDEX integration_session_artifacts_list_idx
    ON integration_session_artifact_revisions
        (tenant_id, session_id, created_at, artifact_id, version);

CREATE TABLE integration_session_runs (
    tenant_id TEXT NOT NULL,
    session_id TEXT NOT NULL,
    run_id TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('pending', 'running', 'succeeded', 'failed')),
    created_at TIMESTAMPTZ NOT NULL,
    record_json JSONB NOT NULL,
    PRIMARY KEY (tenant_id, run_id),
    FOREIGN KEY (tenant_id, session_id)
        REFERENCES integration_sessions (tenant_id, session_id)
);

CREATE INDEX integration_session_runs_list_idx
    ON integration_session_runs (tenant_id, session_id, created_at, run_id);

CREATE TABLE integration_session_decisions (
    tenant_id TEXT NOT NULL,
    session_id TEXT NOT NULL,
    decision_id TEXT NOT NULL,
    run_id TEXT NOT NULL,
    diagnostic_id TEXT NOT NULL,
    accepted_at TIMESTAMPTZ NOT NULL,
    record_json JSONB NOT NULL,
    PRIMARY KEY (tenant_id, decision_id),
    UNIQUE (tenant_id, run_id, diagnostic_id),
    FOREIGN KEY (tenant_id, session_id)
        REFERENCES integration_sessions (tenant_id, session_id),
    FOREIGN KEY (tenant_id, run_id)
        REFERENCES integration_session_runs (tenant_id, run_id)
);

CREATE INDEX integration_session_decisions_list_idx
    ON integration_session_decisions (tenant_id, session_id, accepted_at, decision_id);

CREATE TABLE integration_session_exports (
    tenant_id TEXT NOT NULL,
    session_id TEXT NOT NULL,
    export_id TEXT NOT NULL,
    exported_at TIMESTAMPTZ NOT NULL,
    record_json JSONB NOT NULL,
    PRIMARY KEY (tenant_id, export_id),
    FOREIGN KEY (tenant_id, session_id)
        REFERENCES integration_sessions (tenant_id, session_id)
);

CREATE INDEX integration_session_exports_list_idx
    ON integration_session_exports (tenant_id, session_id, exported_at, export_id);
