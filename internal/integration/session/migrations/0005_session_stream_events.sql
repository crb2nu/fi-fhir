-- Durable Integration Session stream fanout (Slice 4.3, Lane S3-A).
--
-- Before this migration the session event stream was a process-local
-- map[string]subscription with a 32-slot buffered channel per subscriber
-- (internal/integration/session/hub.go). Exactly one Hub exists per process, so
-- with the two replicas the checked-in manifests already declare, a client's
-- long-lived SSE POST pins to replica A while its runIntegrationSample mutation
-- lands on replica B: B publishes run_started..run_completed into B's hub and
-- A's subscriber receives nothing. The run still completes durably, so the UI
-- shows a stalled stream over a succeeded run.
--
-- This table is the fanout log every replica reads.
--
-- It is deliberately an ENVELOPE log, not an event store. It carries no
-- payload, and it never will: the GraphQL projection
-- (internal/api/graphql/resolvers/integration_session_service.go, toGraphQLEvent)
-- already ignores StreamEvent.Payload and re-reads the session and the run from
-- the durable store, so a subscriber's view is fully reproducible from
-- (session_id, run_id, event_type). Keeping clinical content out of this table
-- means the multi-replica fix adds zero new PHI at rest and leaves retention
-- policy entirely to Slice 4.1d.
CREATE TABLE integration_session_stream_events (
    seq BIGSERIAL PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    event_id TEXT NOT NULL,
    session_id TEXT NOT NULL,
    run_id TEXT NOT NULL DEFAULT '',
    event_type TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL
);

COMMENT ON TABLE integration_session_stream_events IS
    'Envelope-only fanout log for Integration Session SSE across replicas. Never carries a payload.';
COMMENT ON COLUMN integration_session_stream_events.seq IS
    'Monotonic per-database cursor. A replica resumes from the last seq it delivered, so a replica flip is a gap, not a loss.';

-- Relay reads are always "everything after my cursor", so the cursor is the
-- leading index column. Tenant is second because one deployment security domain
-- owns one database.
CREATE INDEX integration_session_stream_events_cursor_idx
    ON integration_session_stream_events (seq, tenant_id);

-- There is deliberately no foreign key to integration_sessions. The log is
-- append-only telemetry about a session's lifecycle, including archival; a
-- referential constraint would let a session-table write order failure silently
-- suppress a subscriber's stream, which is the failure this table exists to
-- remove.
CREATE OR REPLACE FUNCTION reject_integration_session_stream_event_mutation()
RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'integration session stream events are append-only';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER integration_session_stream_events_immutable
    BEFORE UPDATE OR DELETE ON integration_session_stream_events
    FOR EACH ROW EXECUTE FUNCTION reject_integration_session_stream_event_mutation();
