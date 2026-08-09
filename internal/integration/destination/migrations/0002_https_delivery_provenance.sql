-- Slice 4.1c-b records the server-owned provenance of every destination
-- delivery this process performs itself.
--
-- 0001 records the *decision*: whether a dispatch was authorized to reach a
-- destination. This table records the *act*: that the process actually contacted
-- one, under which verified destination revision, and how the exchange ended.
-- They are separate ledgers on purpose — an authorized decision with no delivery
-- row means the attempt was authorized and then published to the broker, which
-- is exactly what every `kafka`-transport destination does and exactly what
-- every destination did before this slice.
--
-- Trust model, continuing the idiom Slice 4.1b3 established for batch receipts
-- and 4.1c-a applied to decisions: every column here is produced by this process
-- from the deployed destination revision, except the two carrying an `_advisory`
-- suffix, which are destination-derived and are never trust inputs.
--
-- http_status_class is deliberately *not* advisory. It is not the destination's
-- status line; it is this process's own reduction of the response to a closed
-- five-value vocabulary, and it is the only property of the response that is
-- read at all. The body is drained and discarded unparsed, and no response
-- header is consulted.
--
-- No retroactive vouching. Deliveries dispatched before this revision have no
-- row here, which is how they stay visibly distinguishable: absence of a row
-- means this process contacted no destination for that attempt, never that it
-- did so and the record was lost.
CREATE TABLE integration_destination_deliveries (
    delivery_id BIGINT GENERATED ALWAYS AS IDENTITY,
    tenant_id TEXT NOT NULL,
    attempt_id TEXT NOT NULL,
    transport TEXT NOT NULL CHECK (transport IN ('https')),
    destination_artifact_id TEXT NOT NULL,
    destination_revision_id TEXT NOT NULL,
    destination_class TEXT NOT NULL,
    destination_digest_verified TEXT NOT NULL,
    outcome TEXT NOT NULL CHECK (outcome IN ('delivered', 'retryable', 'refused')),
    failure_code TEXT NOT NULL DEFAULT '',
    http_status_class TEXT NOT NULL DEFAULT ''
        CHECK (http_status_class IN ('', '1xx', '2xx', '3xx', '4xx', '5xx')),
    destination_endpoint_advisory TEXT NOT NULL DEFAULT '',
    served_certificate_subject_advisory TEXT NOT NULL DEFAULT '',
    completed_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (tenant_id, delivery_id),
    CHECK (octet_length(attempt_id) BETWEEN 1 AND 256),
    CHECK (octet_length(destination_digest_verified) BETWEEN 1 AND 256),
    CHECK (octet_length(failure_code) BETWEEN 0 AND 128),
    CHECK (octet_length(destination_endpoint_advisory) BETWEEN 0 AND 2048),
    CHECK (octet_length(served_certificate_subject_advisory) BETWEEN 0 AND 256)
);

CREATE INDEX integration_destination_deliveries_attempt_idx
    ON integration_destination_deliveries (tenant_id, attempt_id, completed_at, delivery_id);

-- NOT VALID for the same reason 0001's provenance constraint is: the constraint
-- governs every row written from this revision forward without asserting
-- anything about rows a later backfill might add for deliveries that predate the
-- ledger. A backfilled row would have to be validated explicitly.
ALTER TABLE integration_destination_deliveries
    ADD CONSTRAINT integration_destination_deliveries_outcome_chk CHECK (
        (
            outcome = 'delivered'
            AND failure_code = ''
            AND http_status_class = '2xx'
        )
        OR
        (
            outcome <> 'delivered'
            AND failure_code <> ''
        )
    ) NOT VALID;

COMMENT ON TABLE integration_destination_deliveries IS
    'Server-owned provenance of each destination delivery this process performed itself; absence of a row means no destination was contacted for that attempt.';
COMMENT ON COLUMN integration_destination_deliveries.destination_digest_verified IS
    'Destination revision digest this process verified against the deployed set before contacting it; the trust anchor of the delivery.';
COMMENT ON COLUMN integration_destination_deliveries.http_status_class IS
    'This process''s own reduction of the response to a closed five-value vocabulary. It is the only property of the response that is read; the body is drained and discarded unparsed and no response header is consulted.';
COMMENT ON COLUMN integration_destination_deliveries.destination_endpoint_advisory IS
    'Remote address declared by the destination revision; advisory only, never a trust input.';
COMMENT ON COLUMN integration_destination_deliveries.served_certificate_subject_advisory IS
    'Subject of the certificate the destination served, bounded and stripped to printable ASCII. Destination-derived, advisory only, never a trust input: trust came from verifying that certificate against roots the deployment declared.';
COMMENT ON COLUMN integration_destination_deliveries.completed_at IS
    'Server-owned completion timestamp; the authoritative moment this process finished the exchange.';
