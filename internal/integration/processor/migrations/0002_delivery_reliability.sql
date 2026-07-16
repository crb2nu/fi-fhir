ALTER TABLE integration_delivery_attempts
    ADD COLUMN parent_attempt_id TEXT,
    ADD COLUMN scheduled_at TIMESTAMPTZ,
    ADD COLUMN completed_at TIMESTAMPTZ,
    ADD COLUMN last_error_code TEXT NOT NULL DEFAULT '',
    ADD COLUMN last_error_detail TEXT NOT NULL DEFAULT '';

UPDATE integration_delivery_attempts
SET scheduled_at = recorded_at
WHERE scheduled_at IS NULL;

ALTER TABLE integration_delivery_attempts
    ALTER COLUMN scheduled_at SET NOT NULL,
    ADD CONSTRAINT integration_delivery_attempts_parent_fk
        FOREIGN KEY (tenant_id, parent_attempt_id)
        REFERENCES integration_delivery_attempts (tenant_id, attempt_id)
        ON DELETE RESTRICT;

ALTER TABLE integration_delivery_outbox
    DROP CONSTRAINT integration_delivery_outbox_status_check,
    ADD CONSTRAINT integration_delivery_outbox_status_check
        CHECK (status IN ('pending', 'leased', 'published', 'failed')),
    ADD COLUMN lease_owner TEXT NOT NULL DEFAULT '',
    ADD COLUMN lease_expires_at TIMESTAMPTZ,
    ADD COLUMN updated_at TIMESTAMPTZ;

UPDATE integration_delivery_outbox
SET updated_at = created_at
WHERE updated_at IS NULL;

ALTER TABLE integration_delivery_outbox
    ALTER COLUMN updated_at SET NOT NULL,
    ADD CONSTRAINT integration_delivery_outbox_lease_shape_check CHECK (
        (status = 'leased' AND lease_owner <> '' AND lease_expires_at IS NOT NULL)
        OR
        (status <> 'leased' AND lease_owner = '' AND lease_expires_at IS NULL)
    );

DROP INDEX integration_delivery_outbox_pending_idx;
CREATE INDEX integration_delivery_outbox_pending_idx
    ON integration_delivery_outbox (scheduled_at, tenant_id, outbox_id)
    WHERE status = 'pending';
CREATE INDEX integration_delivery_outbox_lease_idx
    ON integration_delivery_outbox (lease_expires_at, tenant_id, outbox_id)
    WHERE status = 'leased';

CREATE TABLE integration_delivery_dlq (
    tenant_id TEXT NOT NULL,
    attempt_id TEXT NOT NULL,
    outbox_id TEXT NOT NULL,
    failure_code TEXT NOT NULL,
    failure_detail TEXT NOT NULL,
    failed_at TIMESTAMPTZ NOT NULL,
    active BOOLEAN NOT NULL DEFAULT true,
    replay_count INTEGER NOT NULL DEFAULT 0 CHECK (replay_count >= 0),
    last_replayed_at TIMESTAMPTZ,
    PRIMARY KEY (tenant_id, attempt_id),
    UNIQUE (tenant_id, outbox_id),
    FOREIGN KEY (tenant_id, attempt_id)
        REFERENCES integration_delivery_attempts (tenant_id, attempt_id)
        ON DELETE RESTRICT,
    FOREIGN KEY (tenant_id, outbox_id)
        REFERENCES integration_delivery_outbox (tenant_id, outbox_id)
        ON DELETE RESTRICT
);

CREATE TABLE integration_delivery_circuits (
    tenant_id TEXT NOT NULL,
    destination_artifact_id TEXT NOT NULL,
    destination_revision_id TEXT NOT NULL,
    destination_digest TEXT NOT NULL,
    state TEXT NOT NULL CHECK (state IN ('closed', 'open')),
    consecutive_failures INTEGER NOT NULL CHECK (consecutive_failures >= 0),
    open_until TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (
        tenant_id,
        destination_artifact_id,
        destination_revision_id,
        destination_digest
    ),
    CHECK (
        (state = 'open' AND open_until IS NOT NULL)
        OR
        (state = 'closed' AND open_until IS NULL)
    )
);

CREATE TABLE integration_delivery_operations (
    tenant_id TEXT NOT NULL,
    operation_id TEXT NOT NULL,
    idempotency_key TEXT NOT NULL,
    operation_kind TEXT NOT NULL CHECK (operation_kind IN ('replay', 'resubmit')),
    source_attempt_id TEXT NOT NULL,
    result_attempt_id TEXT NOT NULL,
    principal_json JSONB NOT NULL CHECK (jsonb_typeof(principal_json) = 'object'),
    reason TEXT NOT NULL,
    recorded_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (tenant_id, operation_id),
    UNIQUE (tenant_id, idempotency_key),
    FOREIGN KEY (tenant_id, source_attempt_id)
        REFERENCES integration_delivery_attempts (tenant_id, attempt_id)
        ON DELETE RESTRICT,
    FOREIGN KEY (tenant_id, result_attempt_id)
        REFERENCES integration_delivery_attempts (tenant_id, attempt_id)
        ON DELETE RESTRICT,
    CHECK (octet_length(idempotency_key) BETWEEN 1 AND 512),
    CHECK (octet_length(reason) BETWEEN 1 AND 1024)
);

CREATE TABLE integration_delivery_audit (
    audit_id BIGINT GENERATED ALWAYS AS IDENTITY,
    tenant_id TEXT NOT NULL,
    attempt_id TEXT NOT NULL,
    event_kind TEXT NOT NULL CHECK (event_kind IN (
        'claimed',
        'lease_reclaimed',
        'circuit_deferred',
        'retry_scheduled',
        'dlq_entered',
        'published',
        'replayed',
        'resubmitted'
    )),
    attempt_count INTEGER NOT NULL CHECK (attempt_count > 0),
    principal_json JSONB NOT NULL CHECK (jsonb_typeof(principal_json) = 'object'),
    reason TEXT NOT NULL DEFAULT '',
    detail_json JSONB NOT NULL CHECK (jsonb_typeof(detail_json) = 'object'),
    recorded_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (tenant_id, audit_id),
    FOREIGN KEY (tenant_id, attempt_id)
        REFERENCES integration_delivery_attempts (tenant_id, attempt_id)
        ON DELETE RESTRICT
);

CREATE INDEX integration_delivery_audit_attempt_idx
    ON integration_delivery_audit (tenant_id, attempt_id, recorded_at, audit_id);
