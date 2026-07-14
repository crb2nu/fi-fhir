CREATE TABLE integration_receipts (
    tenant_id TEXT NOT NULL,
    receipt_id TEXT NOT NULL,
    idempotency_key TEXT NOT NULL,
    request_fingerprint TEXT NOT NULL,
    integration_revision JSONB NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('accepted', 'rejected')),
    recorded_at TIMESTAMPTZ NOT NULL,
    correlation_id TEXT NOT NULL,
    raw_retention_mode TEXT NOT NULL CHECK (raw_retention_mode IN ('ephemeral', 'encrypted')),
    principal_json JSONB NOT NULL CHECK (jsonb_typeof(principal_json) = 'object'),
    reason TEXT NOT NULL DEFAULT '',
    result_json JSONB NOT NULL CHECK (jsonb_typeof(result_json) = 'object'),
    PRIMARY KEY (tenant_id, receipt_id),
    UNIQUE (tenant_id, idempotency_key),
    CHECK (octet_length(idempotency_key) BETWEEN 1 AND 512)
);

CREATE TABLE integration_canonical_events (
    tenant_id TEXT NOT NULL,
    event_id TEXT NOT NULL,
    receipt_id TEXT NOT NULL,
    event_type TEXT NOT NULL,
    source_message_id TEXT NOT NULL,
    correlation_id TEXT NOT NULL,
    classification TEXT NOT NULL CHECK (classification = 'phi'),
    payload_json JSONB NOT NULL CHECK (jsonb_typeof(payload_json) = 'object'),
    recorded_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (tenant_id, event_id),
    FOREIGN KEY (tenant_id, receipt_id)
        REFERENCES integration_receipts (tenant_id, receipt_id)
        ON DELETE RESTRICT
);

CREATE TABLE integration_message_lineage (
    tenant_id TEXT NOT NULL,
    lineage_id TEXT NOT NULL,
    receipt_id TEXT NOT NULL,
    event_id TEXT NOT NULL,
    trace_id TEXT NOT NULL,
    correlation_id TEXT NOT NULL,
    source_message_id TEXT NOT NULL,
    artifact_revisions_json JSONB NOT NULL CHECK (jsonb_typeof(artifact_revisions_json) = 'object'),
    routes_json JSONB NOT NULL CHECK (jsonb_typeof(routes_json) = 'array'),
    diagnostics_json JSONB NOT NULL CHECK (jsonb_typeof(diagnostics_json) = 'array'),
    recorded_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (tenant_id, lineage_id),
    UNIQUE (tenant_id, receipt_id, event_id),
    FOREIGN KEY (tenant_id, receipt_id)
        REFERENCES integration_receipts (tenant_id, receipt_id)
        ON DELETE RESTRICT,
    FOREIGN KEY (tenant_id, event_id)
        REFERENCES integration_canonical_events (tenant_id, event_id)
        ON DELETE RESTRICT
);

CREATE TABLE integration_delivery_attempts (
    tenant_id TEXT NOT NULL,
    attempt_id TEXT NOT NULL,
    receipt_id TEXT NOT NULL,
    event_id TEXT NOT NULL,
    trace_id TEXT NOT NULL,
    destination_revision_json JSONB NOT NULL CHECK (jsonb_typeof(destination_revision_json) = 'object'),
    route_name TEXT NOT NULL,
    action_id TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('queued', 'succeeded', 'failed')),
    attempt_count INTEGER NOT NULL CHECK (attempt_count > 0),
    recorded_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (tenant_id, attempt_id),
    FOREIGN KEY (tenant_id, receipt_id)
        REFERENCES integration_receipts (tenant_id, receipt_id)
        ON DELETE RESTRICT,
    FOREIGN KEY (tenant_id, event_id)
        REFERENCES integration_canonical_events (tenant_id, event_id)
        ON DELETE RESTRICT
);

CREATE TABLE integration_delivery_outbox (
    tenant_id TEXT NOT NULL,
    outbox_id TEXT NOT NULL,
    attempt_id TEXT NOT NULL,
    topic TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('pending', 'published', 'failed')),
    payload_json JSONB NOT NULL CHECK (jsonb_typeof(payload_json) = 'object'),
    created_at TIMESTAMPTZ NOT NULL,
    scheduled_at TIMESTAMPTZ NOT NULL,
    published_at TIMESTAMPTZ,
    PRIMARY KEY (tenant_id, outbox_id),
    UNIQUE (tenant_id, attempt_id),
    FOREIGN KEY (tenant_id, attempt_id)
        REFERENCES integration_delivery_attempts (tenant_id, attempt_id)
        ON DELETE RESTRICT
);

CREATE INDEX integration_delivery_outbox_pending_idx
    ON integration_delivery_outbox (status, scheduled_at, tenant_id, outbox_id);
