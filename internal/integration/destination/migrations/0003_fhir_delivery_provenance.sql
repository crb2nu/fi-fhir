-- Slice 4.1c-c teaches the delivery provenance ledger about the `fhir`
-- transport.
--
-- 0002 records the act of an `https` delivery. A `fhir` delivery is the same
-- act — this process contacted a destination under a verified revision — with
-- three facts an operator needs that an envelope delivery does not have: which
-- resource types crossed the wire, how many bundle entries carried them, and,
-- when the destination refused, which OperationOutcome issue codes it answered
-- with.
--
-- Migration rules (Slice 4.4a, enforced by make migration-compatibility):
-- additive only; every NOT NULL column carries a DEFAULT so a binary one
-- version behind — whose INSERT names neither the transport 'fhir' nor these
-- columns — still writes its rows; the widened transport CHECK is re-declared
-- rather than altered in place, and it is N-1 safe because the older binary
-- never writes 'fhir'.
--
-- PHI posture: fhir_outcome_codes_advisory carries OperationOutcome ISSUE CODES
-- ONLY — the closed FHIR issue-type vocabulary (`invalid`, `not-found`,
-- `conflict`, …), sanitised to lowercase letters and hyphens and bounded. It
-- NEVER carries `diagnostics`, `details`, or any other free text from the
-- response, because a destination's diagnostics routinely echo the request —
-- identifiers, names, dates — and this ledger must stay clinical-content-free
-- exactly as 0002 is.
ALTER TABLE integration_destination_deliveries
    DROP CONSTRAINT integration_destination_deliveries_transport_check;

ALTER TABLE integration_destination_deliveries
    ADD CONSTRAINT integration_destination_deliveries_transport_check
        CHECK (transport IN ('https', 'fhir'));

ALTER TABLE integration_destination_deliveries
    ADD COLUMN fhir_resource_types TEXT NOT NULL DEFAULT ''
        CHECK (octet_length(fhir_resource_types) BETWEEN 0 AND 512);

ALTER TABLE integration_destination_deliveries
    ADD COLUMN fhir_entry_count INTEGER NOT NULL DEFAULT 0
        CHECK (fhir_entry_count >= 0);

ALTER TABLE integration_destination_deliveries
    ADD COLUMN fhir_outcome_codes_advisory TEXT NOT NULL DEFAULT ''
        CHECK (octet_length(fhir_outcome_codes_advisory) BETWEEN 0 AND 512);

COMMENT ON COLUMN integration_destination_deliveries.fhir_resource_types IS
    'Comma-joined FHIR resource types this process projected and delivered in the transaction Bundle, in bundle order without repeats; empty for an https delivery. Server-owned.';
COMMENT ON COLUMN integration_destination_deliveries.fhir_entry_count IS
    'Number of entries in the delivered transaction Bundle; 0 for an https delivery. Server-owned.';
COMMENT ON COLUMN integration_destination_deliveries.fhir_outcome_codes_advisory IS
    'OperationOutcome issue CODES the destination answered with, sanitised and bounded. Destination-derived, advisory only, never a trust input. It never carries diagnostics or details text: a FHIR server''s diagnostics echo the request, and this ledger holds no clinical content.';
