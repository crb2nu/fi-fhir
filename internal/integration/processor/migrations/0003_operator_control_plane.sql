-- Slice 4.2a: the operator control plane adds one genuinely new durable
-- recovery decision (discard) and makes a resolved dead letter state why it
-- closed. Replay and resubmit keep their existing idempotent machinery.

ALTER TABLE integration_delivery_operations
    DROP CONSTRAINT integration_delivery_operations_operation_kind_check,
    ADD CONSTRAINT integration_delivery_operations_operation_kind_check
        CHECK (operation_kind IN ('replay', 'resubmit', 'discard'));

ALTER TABLE integration_delivery_audit
    DROP CONSTRAINT integration_delivery_audit_event_kind_check,
    ADD CONSTRAINT integration_delivery_audit_event_kind_check
        CHECK (event_kind IN (
            'claimed',
            'lease_reclaimed',
            'circuit_deferred',
            'retry_scheduled',
            'dlq_entered',
            'published',
            'replayed',
            'resubmitted',
            'discarded'
        ));

ALTER TABLE integration_delivery_dlq
    ADD COLUMN resolution TEXT NOT NULL DEFAULT '',
    ADD COLUMN resolved_at TIMESTAMPTZ;

-- Dead letters closed before this migration were closed by replay or resubmit.
-- Recover the exact operation kind where an idempotent operation recorded it
-- and fall back to the conservative replay label otherwise.
UPDATE integration_delivery_dlq d
SET resolution = COALESCE((
        SELECT o.operation_kind || 'ed'
        FROM integration_delivery_operations o
        WHERE o.tenant_id = d.tenant_id
          AND o.source_attempt_id = d.attempt_id
        ORDER BY o.recorded_at DESC, o.operation_id DESC
        LIMIT 1
    ), 'replayed'),
    resolved_at = COALESCE(d.last_replayed_at, d.failed_at)
WHERE d.active = false;

ALTER TABLE integration_delivery_dlq
    ADD CONSTRAINT integration_delivery_dlq_resolution_check
        CHECK (resolution IN ('', 'replayed', 'resubmitted', 'discarded')),
    ADD CONSTRAINT integration_delivery_dlq_resolution_shape_check CHECK (
        (active = true AND resolution = '' AND resolved_at IS NULL)
        OR
        (active = false AND resolution <> '' AND resolved_at IS NOT NULL)
    );

CREATE INDEX integration_delivery_dlq_active_idx
    ON integration_delivery_dlq (tenant_id, failed_at DESC, attempt_id DESC)
    WHERE active = true;

CREATE INDEX integration_delivery_attempts_browse_idx
    ON integration_delivery_attempts (tenant_id, recorded_at DESC, attempt_id DESC);

CREATE INDEX integration_receipts_browse_idx
    ON integration_receipts (tenant_id, recorded_at DESC, receipt_id DESC);
