-- Slice 4.4e: the durable side of the deployment-wide MLLP rate quota.
--
-- A deployment declares one max_messages_per_second on its deployed revision.
-- Before this migration every replica enforced that number on its own, so N
-- replicas admitted N x the declared rate — documented behaviour, and a figure
-- that made the product's steady-state throughput budget unmeasurable.
--
-- What this table is NOT: a counter. Admission is not recorded here and no
-- query runs on the MLLP hot path. Each replica leases a *share* of the
-- declared rate, refills its in-memory token bucket from that share, and
-- renews on an interval measured in seconds; the admission decision itself
-- stays in memory and O(1). A per-frame durable counter under a row lock was
-- considered and rejected — at 250 msg/s it turns a rate limiter into a
-- throughput ceiling. See `.loom/40-decisions.md` (2026-08-09, "Distribute the
-- MLLP rate across replicas with a durable lease-partitioned quota").
--
-- Keyed on the deployment, deliberately not on the revision digest. A rolling
-- redeploy runs two digests at once; a digest-keyed pool would open a second
-- budget and let the deployment admit twice its declared rate for the length of
-- every rollout, which is the failure this table exists to prevent. The digest
-- rides on the claim row as attribution instead, so an operator can see which
-- revision each holder is serving.
--
-- Mutable, unlike everything else in this ledger. Every other table here is
-- append-only and carries a reject_integration_lifecycle_mutation() trigger,
-- because they are audit records. A lease is not an audit record: it is
-- renewed, superseded, and released, exactly like integration_delivery_outbox's
-- lease columns and integration_batch_objects, both of which are deliberately
-- unguarded for the same reason. Attaching an immutability trigger here would
-- make renewal impossible.
--
-- Not a durable class. A restore does not need to bring rate claims back: every
-- row is expired by definition after a restore, the reaper drops it on the
-- first claim, and each replica re-claims within one interval. It is
-- intentionally absent from migrationcompat's durableClasses.
--
-- No PHI, and no clinical content of any kind: tenant, definition, replica
-- identity, an integer rate, and two timestamps.

CREATE TABLE integration_mllp_rate_claims (
    tenant_id TEXT NOT NULL,
    definition_id TEXT NOT NULL,
    -- One row per live replica. hostname-pid in a deployment, matching
    -- delivery_runtime.go's worker-identity idiom; a replica that restarts
    -- under the same identity simply renews.
    holder_id TEXT NOT NULL,
    -- Attribution only. Never part of the key: see the header.
    revision_digest TEXT NOT NULL DEFAULT '',
    declared_rate INTEGER NOT NULL,
    granted_share INTEGER NOT NULL,
    -- Live holders at grant time. Recorded so an operator can tell a small
    -- share caused by many replicas from one caused by a low declared rate.
    holders INTEGER NOT NULL DEFAULT 1,
    -- Server-owned. The caller supplies the lease length, never the instant.
    claimed_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    expires_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (tenant_id, definition_id, holder_id)
);

-- The reaper's access path: every claim call deletes this deployment's expired
-- rows before counting live holders, because a share computed against a stale
-- holder count is not a bound.
CREATE INDEX integration_mllp_rate_claims_expiry_idx
    ON integration_mllp_rate_claims (tenant_id, definition_id, expires_at);

-- NOT VALID, the 4.1b3 idiom: the constraint governs every row written from
-- this revision forward without asserting anything about rows a later backfill
-- might add. Nothing gets vouched for silently.
--
-- The share bound is the table's whole reason to exist. granted_share <=
-- declared_rate holds even in the degenerate case of more replicas than
-- messages per second, where each of them is granted the floor of one and the
-- declared rate is at least one.
ALTER TABLE integration_mllp_rate_claims
    ADD CONSTRAINT integration_mllp_rate_claims_share_chk CHECK (
        declared_rate >= 1
        AND granted_share >= 1
        AND granted_share <= declared_rate
        AND holders >= 1
        AND expires_at > claimed_at
        AND octet_length(holder_id) BETWEEN 1 AND 256
        AND octet_length(revision_digest) BETWEEN 0 AND 256
    ) NOT VALID;

COMMENT ON TABLE integration_mllp_rate_claims IS
    'Lease-partitioned shares of a deployment''s declared MLLP message rate. '
    'One row per live replica, renewed on an interval and released on shutdown. '
    'Never written on the admission path.';
