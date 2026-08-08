-- Slice 4.1c-a records the server-owned provenance of every integration.deliver
-- decision made on the durable dispatch path.
--
-- Trust model, following the idiom Slice 4.1b3 established for batch receipts:
-- every column here except destination_endpoint_advisory is produced by this
-- process from the deployed destination revision and the verified reference.
-- The decision is grounded on destination_digest_verified and granted_role, and
-- on nothing a destination could influence.
--
-- No retroactive vouching. Delivery attempts dispatched before this revision
-- have no row in this table at all, which is exactly how they stay visibly
-- distinguishable: absence of a decision row means the decision was never made,
-- never that it was made and allowed.
CREATE TABLE integration_delivery_identity_decisions (
    decision_id BIGINT GENERATED ALWAYS AS IDENTITY,
    tenant_id TEXT NOT NULL,
    attempt_id TEXT NOT NULL,
    decision TEXT NOT NULL CHECK (decision IN ('authorized', 'denied')),
    identity_mode TEXT NOT NULL CHECK (identity_mode IN ('strict', 'compatibility')),
    principal_subject TEXT NOT NULL DEFAULT '',
    principal_auth_method TEXT NOT NULL DEFAULT '',
    granted_role TEXT NOT NULL DEFAULT '',
    destination_artifact_id TEXT NOT NULL,
    destination_revision_id TEXT NOT NULL,
    destination_class TEXT NOT NULL,
    destination_digest_verified TEXT NOT NULL DEFAULT '',
    denial_code TEXT NOT NULL DEFAULT '',
    destination_endpoint_advisory TEXT NOT NULL DEFAULT '',
    decided_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (tenant_id, decision_id),
    CHECK (octet_length(attempt_id) BETWEEN 1 AND 256),
    CHECK (octet_length(destination_endpoint_advisory) BETWEEN 0 AND 2048)
);

CREATE INDEX integration_delivery_identity_decisions_attempt_idx
    ON integration_delivery_identity_decisions (tenant_id, attempt_id, decided_at, decision_id);

-- NOT VALID so the constraint governs every row written from this revision
-- forward without asserting anything about rows a later backfill might add for
-- dispatches that predate the decision. A backfilled row would have to be
-- validated explicitly, which is the point: nothing gets vouched for silently.
ALTER TABLE integration_delivery_identity_decisions
    ADD CONSTRAINT integration_delivery_identity_decisions_provenance_chk CHECK (
        (
            decision = 'authorized'
            AND principal_subject <> ''
            AND principal_auth_method <> ''
            AND granted_role <> ''
            AND destination_digest_verified <> ''
            AND denial_code = ''
        )
        OR
        (
            decision = 'denied'
            AND denial_code <> ''
            AND granted_role = ''
        )
    ) NOT VALID;

COMMENT ON TABLE integration_delivery_identity_decisions IS
    'Server-owned provenance of each integration.deliver decision; absence of a row means no decision was made, never that one was allowed.';
COMMENT ON COLUMN integration_delivery_identity_decisions.destination_digest_verified IS
    'Destination revision digest this process verified against the deployed set before authorizing; the trust anchor of the decision.';
COMMENT ON COLUMN integration_delivery_identity_decisions.granted_role IS
    'Server-issued grant that authorized the dispatch; never asserted by the attempt or by the destination.';
COMMENT ON COLUMN integration_delivery_identity_decisions.destination_endpoint_advisory IS
    'Remote address declared by the destination revision; advisory only, never a trust input. Slice 4.1c-a contacts no destination.';
COMMENT ON COLUMN integration_delivery_identity_decisions.decided_at IS
    'Server-owned decision timestamp; the authoritative moment this dispatch was authorized or refused.';
