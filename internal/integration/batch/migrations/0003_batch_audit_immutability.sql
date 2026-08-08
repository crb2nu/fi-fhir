-- Slice 4.1d C1: integration_batch_audit is the batch ingestion ledger. Every
-- write in internal/integration/batch/store.go is an INSERT; nothing updates or
-- deletes a row. Make that a schema guarantee instead of a code convention, in
-- the same shape as reject_integration_lifecycle_mutation()
-- (internal/integration/lifecycle/migrations/0001_deployment_lifecycle.sql:87-92).
--
-- integration_batch_objects is deliberately NOT guarded here: it is the leased
-- state table the runtime advances through claim -> checkpoint -> archive ->
-- complete, guarded instead by its owner-fenced UPDATE predicates.

CREATE OR REPLACE FUNCTION reject_integration_batch_audit_mutation()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'integration batch audit records are append-only';
END;
$$;

DROP TRIGGER IF EXISTS integration_batch_audit_immutable ON integration_batch_audit;
CREATE TRIGGER integration_batch_audit_immutable
    BEFORE UPDATE OR DELETE ON integration_batch_audit
    FOR EACH ROW EXECUTE FUNCTION reject_integration_batch_audit_mutation();
