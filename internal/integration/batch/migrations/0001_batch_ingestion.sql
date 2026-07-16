CREATE TABLE integration_batch_objects (
    tenant_id TEXT NOT NULL,
    source_id TEXT NOT NULL,
    source_revision_digest TEXT NOT NULL,
    integration_revision_digest TEXT NOT NULL,
    object_id TEXT NOT NULL,
    provider TEXT NOT NULL CHECK (provider IN ('s3', 'sftp')),
    object_size BIGINT NOT NULL CHECK (object_size > 0),
    object_modified_at TIMESTAMPTZ NOT NULL,
    phase TEXT NOT NULL CHECK (phase IN ('processing', 'awaiting_archive', 'completed', 'failed')),
    checkpoint_offset BIGINT NOT NULL DEFAULT 0 CHECK (checkpoint_offset >= 0),
    checkpoint_message BIGINT NOT NULL DEFAULT 0 CHECK (checkpoint_message >= 0),
    content_digest TEXT NOT NULL DEFAULT '',
    lease_owner TEXT NOT NULL DEFAULT '',
    lease_expires_at TIMESTAMPTZ,
    last_error_code TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    completed_at TIMESTAMPTZ,
    PRIMARY KEY (tenant_id, source_id, source_revision_digest, object_id),
    CHECK (octet_length(object_id) = 71),
    CHECK (octet_length(source_revision_digest) = 71),
    CHECK (octet_length(integration_revision_digest) = 71),
    CHECK (content_digest = '' OR octet_length(content_digest) = 71),
    CHECK (
        (lease_owner = '' AND lease_expires_at IS NULL)
        OR
        (lease_owner <> '' AND lease_expires_at IS NOT NULL)
    ),
    CHECK (
        (phase = 'completed' AND completed_at IS NOT NULL AND content_digest <> '')
        OR
        (phase <> 'completed' AND completed_at IS NULL)
    ),
    CHECK (phase NOT IN ('awaiting_archive', 'completed') OR content_digest <> '')
);

CREATE INDEX integration_batch_lease_idx
    ON integration_batch_objects (lease_expires_at, tenant_id, source_id)
    WHERE lease_owner <> '';

CREATE TABLE integration_batch_audit (
    tenant_id TEXT NOT NULL,
    source_id TEXT NOT NULL,
    source_revision_digest TEXT NOT NULL,
    object_id TEXT NOT NULL,
    sequence BIGINT GENERATED ALWAYS AS IDENTITY,
    event_kind TEXT NOT NULL CHECK (event_kind IN (
        'claimed', 'lease_reclaimed', 'checkpoint_advanced',
        'archive_pending', 'completed', 'failed', 'released'
    )),
    checkpoint_offset BIGINT NOT NULL CHECK (checkpoint_offset >= 0),
    checkpoint_message BIGINT NOT NULL CHECK (checkpoint_message >= 0),
    detail_json JSONB NOT NULL CHECK (jsonb_typeof(detail_json) = 'object'),
    recorded_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (tenant_id, sequence),
    FOREIGN KEY (tenant_id, source_id, source_revision_digest, object_id)
        REFERENCES integration_batch_objects (
            tenant_id, source_id, source_revision_digest, object_id
        ) ON DELETE RESTRICT
);

CREATE INDEX integration_batch_audit_object_idx
    ON integration_batch_audit (
        tenant_id, source_id, source_revision_digest, object_id, sequence
    );
