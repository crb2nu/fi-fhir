-- .loom/39 "Convergence" step 1: observed runtime status.
--
-- Every `serve` replica writes one row per adapter it composed (http, mllp,
-- batch, delivery) and one per destination in its delivery identity registry,
-- on the health-report timer. A row says which content-addressed document
-- that replica actually mounted, so the catalog and engineRuntime can say
-- "observed on N/N replicas" instead of only "mounted on the replica that
-- answered this request". Every replica writes its own rows, never a leader
-- alone: a leader-only report hides exactly the divergence this exists to show.
--
-- This is a heartbeat table, not a record. Rows are upserted in place on every
-- tick, so unlike the drafts, revisions, and captures in 0001 it carries no
-- immutability trigger, and a replica that stops reporting simply goes stale
-- (heartbeat_at older than 3x the report interval) rather than being deleted.
--
--   * replica_id is hostname-pid, the rate quota's holder id.
--   * adapter is the adapter kind, or "destination:<artifact id>" for one
--     destination of the delivery identity registry.
--   * definition_id, artifact_id, revision_id and digest are NULL when the
--     adapter is disabled or does not mount a document of its own.
--   * observed_at moves only when the mounted digest or revision changes;
--     heartbeat_at moves on every tick.
--
-- A new table, so AGENTS.md "Migration authoring" rule 1 has no N-1 INSERT to
-- protect; the NOT NULL timestamps carry a DEFAULT anyway.

CREATE TABLE integration_runtime_observations (
    tenant_id TEXT NOT NULL,
    replica_id TEXT NOT NULL,
    adapter TEXT NOT NULL,
    definition_id TEXT,
    artifact_id TEXT,
    revision_id TEXT,
    digest TEXT,
    observed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    heartbeat_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, replica_id, adapter)
);

CREATE INDEX integration_runtime_observations_digest
    ON integration_runtime_observations (tenant_id, digest);
