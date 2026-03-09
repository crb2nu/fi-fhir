# Iteration Plan: M1 CDA Expansion

## Scope

Finalize the extraction and mapping of CDA sections for Medications, Allergies, and Social History.

- **In Scope**:
  - Verify existing parsers (`section_medications.go`, `section_allergies.go`, `section_social_history.go`).
  - Verify mappers in `mapper.go`.
  - Validate unit test coverage and correctness for the implemented CDA expansions.
  - Run the full test suite and linters to ensure CI readiness.
- **Out of Scope**:
  - Other CDA sections (e.g., family history).
  - Pull-based S3/SFTP ingestion (deferred to next slice).

## Acceptance Criteria

1. `make test` passes locally.
2. CDA parsing for Medications, Allergies, and Social History successfully maps to `events.MedicationRequestEvent`, `events.AllergyIntoleranceEvent`, and `events.ObservationEvent` respectively.
3. Code meets existing static analysis and coverage standards.

## Risk Notes

- If the current implementation on the branch has gaps in mapping nested entryRelationships (like severity or reactions in allergies), those must be patched.

## Test Plan

- Run `go test ./internal/parser/cda/... -v -cover`
- Check for parsing regressions in other templates.
- Ensure end-to-end extraction from a raw CDA XML string passes without losing required canonical fields.
