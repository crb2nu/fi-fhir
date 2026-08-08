-- Slice 4.1d C1: make the durable-runtime audit and provenance records immutable
-- in the schema instead of by code convention.
--
-- Two guard shapes, deliberately not one:
--
--   * Append-only ledgers get a blanket BEFORE UPDATE OR DELETE guard. Nothing in
--     the runtime ever mutates them; every write is an INSERT.
--
--   * integration_receipts and integration_delivery_attempts are STATE tables.
--     The shipped delivery state machine legitimately advances
--     integration_delivery_attempts through status / attempt_count /
--     scheduled_at / completed_at / last_error_code / last_error_detail
--     (internal/integration/delivery/store.go). Blanket-guarding those tables
--     would break claim -> retry -> DLQ -> replay. They instead get a
--     column-scoped BEFORE UPDATE guard that raises only when an identity,
--     provenance, or attribution column changes, plus a blanket DELETE guard,
--     because removing an admission or an attempt is never legitimate.
--
-- Row-level triggers do not affect DDL, so the integration suites' schema
-- teardown (DROP SCHEMA ... CASCADE) is unaffected.

CREATE OR REPLACE FUNCTION reject_integration_submission_mutation()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'integration submission audit records are append-only';
END;
$$;

DROP TRIGGER IF EXISTS integration_canonical_events_immutable ON integration_canonical_events;
CREATE TRIGGER integration_canonical_events_immutable
    BEFORE UPDATE OR DELETE ON integration_canonical_events
    FOR EACH ROW EXECUTE FUNCTION reject_integration_submission_mutation();

DROP TRIGGER IF EXISTS integration_message_lineage_immutable ON integration_message_lineage;
CREATE TRIGGER integration_message_lineage_immutable
    BEFORE UPDATE OR DELETE ON integration_message_lineage
    FOR EACH ROW EXECUTE FUNCTION reject_integration_submission_mutation();

DROP TRIGGER IF EXISTS integration_delivery_audit_immutable ON integration_delivery_audit;
CREATE TRIGGER integration_delivery_audit_immutable
    BEFORE UPDATE OR DELETE ON integration_delivery_audit
    FOR EACH ROW EXECUTE FUNCTION reject_integration_submission_mutation();

DROP TRIGGER IF EXISTS integration_delivery_operations_immutable ON integration_delivery_operations;
CREATE TRIGGER integration_delivery_operations_immutable
    BEFORE UPDATE OR DELETE ON integration_delivery_operations
    FOR EACH ROW EXECUTE FUNCTION reject_integration_submission_mutation();

-- State tables: deletion is never legitimate.
CREATE OR REPLACE FUNCTION reject_integration_submission_delete()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'integration submission state records cannot be deleted';
END;
$$;

DROP TRIGGER IF EXISTS integration_receipts_undeletable ON integration_receipts;
CREATE TRIGGER integration_receipts_undeletable
    BEFORE DELETE ON integration_receipts
    FOR EACH ROW EXECUTE FUNCTION reject_integration_submission_delete();

DROP TRIGGER IF EXISTS integration_delivery_attempts_undeletable ON integration_delivery_attempts;
CREATE TRIGGER integration_delivery_attempts_undeletable
    BEFORE DELETE ON integration_delivery_attempts
    FOR EACH ROW EXECUTE FUNCTION reject_integration_submission_delete();

-- State tables: identity, provenance, and attribution columns are frozen; the
-- runtime's own state columns stay writable.
CREATE OR REPLACE FUNCTION reject_integration_receipt_provenance_mutation()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.principal_json IS DISTINCT FROM OLD.principal_json
        OR NEW.correlation_id IS DISTINCT FROM OLD.correlation_id
        OR NEW.request_fingerprint IS DISTINCT FROM OLD.request_fingerprint
        OR NEW.recorded_at IS DISTINCT FROM OLD.recorded_at
        OR NEW.idempotency_key IS DISTINCT FROM OLD.idempotency_key
        OR NEW.integration_revision IS DISTINCT FROM OLD.integration_revision
        OR NEW.raw_retention_mode IS DISTINCT FROM OLD.raw_retention_mode
        OR NEW.reason IS DISTINCT FROM OLD.reason
        OR NEW.result_json IS DISTINCT FROM OLD.result_json
    THEN
        RAISE EXCEPTION 'integration receipt provenance is immutable';
    END IF;
    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS integration_receipts_provenance_immutable ON integration_receipts;
CREATE TRIGGER integration_receipts_provenance_immutable
    BEFORE UPDATE ON integration_receipts
    FOR EACH ROW EXECUTE FUNCTION reject_integration_receipt_provenance_mutation();

CREATE OR REPLACE FUNCTION reject_integration_attempt_provenance_mutation()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.receipt_id IS DISTINCT FROM OLD.receipt_id
        OR NEW.event_id IS DISTINCT FROM OLD.event_id
        OR NEW.trace_id IS DISTINCT FROM OLD.trace_id
        OR NEW.destination_revision_json IS DISTINCT FROM OLD.destination_revision_json
        OR NEW.route_name IS DISTINCT FROM OLD.route_name
        OR NEW.action_id IS DISTINCT FROM OLD.action_id
        OR NEW.parent_attempt_id IS DISTINCT FROM OLD.parent_attempt_id
        OR NEW.recorded_at IS DISTINCT FROM OLD.recorded_at
    THEN
        RAISE EXCEPTION 'integration delivery attempt provenance is immutable';
    END IF;
    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS integration_delivery_attempts_provenance_immutable ON integration_delivery_attempts;
CREATE TRIGGER integration_delivery_attempts_provenance_immutable
    BEFORE UPDATE ON integration_delivery_attempts
    FOR EACH ROW EXECUTE FUNCTION reject_integration_attempt_provenance_mutation();

COMMENT ON TRIGGER integration_receipts_provenance_immutable ON integration_receipts IS
    'Slice 4.1d C1: freezes admission identity, provenance, and attribution. State columns stay writable.';
COMMENT ON TRIGGER integration_delivery_attempts_provenance_immutable ON integration_delivery_attempts IS
    'Slice 4.1d C1: freezes attempt lineage and destination binding. status/attempt_count/scheduled_at/error columns stay writable for the delivery state machine.';
