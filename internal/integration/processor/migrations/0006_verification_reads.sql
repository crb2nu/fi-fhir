-- .loom/42 E-2: indexes for the Verification reads over durable admissions.
--
-- `operatorCanonicalEvents` browses integration_canonical_events newest first
-- with a (recorded_at, event_id) keyset cursor, and `operatorAdmissionStatistics`
-- counts the same table over a recorded_at window. Until this migration the
-- table had only its primary key (tenant_id, event_id) and 0005's partial purge
-- index, so both reads were sequential scans. The receipt, lineage and attempt
-- sides of the same reads already have their indexes
-- (0003_operator_control_plane.sql integration_receipts_browse_idx and
-- integration_delivery_attempts_browse_idx; the lineage UNIQUE constraint).
--
-- The second index serves the receipt filter and the per-receipt event reads
-- (the trace's event list and the receipt browse's event count), which have
-- scanned the whole tenant's events since 0001 because a foreign key does not
-- create an index on the referencing side.
--
-- Index-only: no column, constraint or trigger changes, so a one-version
-- rollback runs the previous binary against this schema unchanged (AGENTS.md
-- "Migration authoring" rule 1 has nothing to hold here).

CREATE INDEX integration_canonical_events_browse_idx
    ON integration_canonical_events (tenant_id, recorded_at DESC, event_id DESC);

CREATE INDEX integration_canonical_events_receipt_idx
    ON integration_canonical_events (tenant_id, receipt_id);
