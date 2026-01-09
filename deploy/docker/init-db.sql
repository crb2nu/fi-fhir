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

-- Grant permissions
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO fi_fhir;
GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO fi_fhir;
