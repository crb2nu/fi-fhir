-- fi-fhir database initialization
-- This script runs on first PostgreSQL container start

-- Create tables for workflow event tracking
CREATE TABLE IF NOT EXISTS workflow_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_type VARCHAR(100) NOT NULL,
    event_id VARCHAR(255),
    source VARCHAR(255),
    payload JSONB NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    processed_at TIMESTAMP WITH TIME ZONE,
    status VARCHAR(50) DEFAULT 'pending'
);

CREATE INDEX idx_workflow_events_type ON workflow_events(event_type);
CREATE INDEX idx_workflow_events_source ON workflow_events(source);
CREATE INDEX idx_workflow_events_status ON workflow_events(status);
CREATE INDEX idx_workflow_events_created ON workflow_events(created_at);

-- Dead letter queue table
CREATE TABLE IF NOT EXISTS dlq_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    original_event_id UUID,
    event_type VARCHAR(100) NOT NULL,
    payload JSONB NOT NULL,
    error_message TEXT,
    error_count INTEGER DEFAULT 1,
    first_failed_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    last_failed_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    route_name VARCHAR(255),
    action_type VARCHAR(100)
);

CREATE INDEX idx_dlq_events_type ON dlq_events(event_type);
CREATE INDEX idx_dlq_events_first_failed ON dlq_events(first_failed_at);

-- Audit log for processed events
CREATE TABLE IF NOT EXISTS audit_log (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id VARCHAR(255),
    event_type VARCHAR(100),
    action VARCHAR(100),
    route_name VARCHAR(255),
    status VARCHAR(50),
    duration_ms INTEGER,
    error_message TEXT,
    metadata JSONB,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_audit_log_event ON audit_log(event_id);
CREATE INDEX idx_audit_log_created ON audit_log(created_at);
CREATE INDEX idx_audit_log_status ON audit_log(status);

-- Managed workflow lifecycle tables
CREATE TABLE IF NOT EXISTS workflow_definitions (
    id VARCHAR(64) PRIMARY KEY,
    name VARCHAR(255) NOT NULL UNIQUE,
    description TEXT,
    status VARCHAR(32) NOT NULL DEFAULT 'draft',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS workflow_versions (
    id VARCHAR(64) PRIMARY KEY,
    workflow_id VARCHAR(64) NOT NULL REFERENCES workflow_definitions(id) ON DELETE CASCADE,
    version_number INTEGER NOT NULL,
    yaml TEXT NOT NULL,
    validation JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_by VARCHAR(255) NOT NULL DEFAULT 'service-account',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    notes TEXT,
    UNIQUE(workflow_id, version_number)
);

CREATE TABLE IF NOT EXISTS workflow_releases (
    id VARCHAR(64) PRIMARY KEY,
    workflow_id VARCHAR(64) NOT NULL REFERENCES workflow_definitions(id) ON DELETE CASCADE,
    environment VARCHAR(64) NOT NULL,
    version_id VARCHAR(64) NOT NULL REFERENCES workflow_versions(id),
    published_by VARCHAR(255) NOT NULL DEFAULT 'service-account',
    published_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    rollback_from_release_id VARCHAR(64) REFERENCES workflow_releases(id)
);

CREATE TABLE IF NOT EXISTS workflow_runs (
    id VARCHAR(64) PRIMARY KEY,
    workflow_id VARCHAR(64),
    workflow_name VARCHAR(255) NOT NULL,
    environment VARCHAR(64) NOT NULL DEFAULT 'production',
    version_id VARCHAR(64) REFERENCES workflow_versions(id),
    event_id VARCHAR(255),
    routes_matched INTEGER NOT NULL DEFAULT 0,
    actions_executed INTEGER NOT NULL DEFAULT 0,
    errors JSONB NOT NULL DEFAULT '[]'::jsonb,
    duration_ms INTEGER NOT NULL DEFAULT 0,
    status VARCHAR(32) NOT NULL DEFAULT 'success',
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS workflow_approval_requests (
    id VARCHAR(64) PRIMARY KEY,
    workflow_id VARCHAR(64) NOT NULL REFERENCES workflow_definitions(id) ON DELETE CASCADE,
    target_version_id VARCHAR(64) NOT NULL REFERENCES workflow_versions(id),
    environment VARCHAR(64) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'pending',
    requested_by VARCHAR(255) NOT NULL DEFAULT 'service-account',
    reviewed_by VARCHAR(255),
    reviewed_at TIMESTAMPTZ,
    comment TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS workflow_audit_log (
    id VARCHAR(64) PRIMARY KEY,
    workflow_id VARCHAR(64) REFERENCES workflow_definitions(id) ON DELETE CASCADE,
    event_type VARCHAR(100) NOT NULL,
    actor VARCHAR(255) NOT NULL,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_workflow_versions_workflow_id ON workflow_versions(workflow_id);
CREATE INDEX IF NOT EXISTS idx_workflow_releases_workflow_env ON workflow_releases(workflow_id, environment, published_at DESC);
CREATE INDEX IF NOT EXISTS idx_workflow_runs_name_env_started ON workflow_runs(workflow_name, environment, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_workflow_approvals_workflow_env_status ON workflow_approval_requests(workflow_id, environment, status);
CREATE INDEX IF NOT EXISTS idx_workflow_audit_workflow_occurred ON workflow_audit_log(workflow_id, occurred_at DESC);

-- Grant permissions
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO fi_fhir;
GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO fi_fhir;
