# 25 - Spec: CDA/CCDA Section Expansion

**Status**: Ready for independent pickup
**Lane**: F - Product expansion speclets
**Tracking**: libs/fi-fhir#13

## Goal

Define a narrow implementation slice that proves and, where needed, deepens CDA/CCDA extraction for Medications, Allergies, and Social History without reopening the entire CDA parser.

The next agent should start by auditing the current CDA mapper behavior against real fixtures, because the planning sources disagree in tone: the CDA planning doc marks CDA/CCDA complete, but the product spec and P3 backlog still call out section expansion.

## Non-Goals

- Do not rewrite the CDA XML parser or namespace handling.
- Do not add a new document model outside `internal/parser/cda/` and `pkg/events`.
- Do not make broad FHIR mapper changes unless the section extraction produces canonical data that already has an established FHIR mapping path.
- Do not introduce external terminology services as a dependency for CDA parsing tests.

## Acceptance Criteria

- Current support for Medications, Allergies, and Social History is audited and documented with file/test references before code changes begin.
- Each target section has at least one representative fixture covering structured entries, coded values, narrative fallback, and missing/partial data.
- Extracted section data maps into canonical event or patient structures without losing the original CDA code/display/system where the source provided them.
- Recoverable malformed or missing section content emits warnings rather than failing the whole document parse, consistent with project parsing policy.
- Source Profile CDA section selection remains honored so feed-specific profiles can enable or suppress section event emission.
- The implementation result updates the relevant planning status with exact shipped behavior, not a generic "CDA complete" label.

## Kill-Test

Before implementing new section logic, run or add one fixture-driven test with a CCDA document that contains all three target sections. The test must fail for the specific missing or lossy behavior this slice intends to fix. If it already passes with preserved code/display/system and profile-controlled emission, stop implementation and convert the slice into documentation/status cleanup.

## Dependencies

- `docs/planning/CDA-CCDA.md` for current CDA structure, section OIDs, Source Profile integration, and test data guidance.
- `pkg/events/events.go` for canonical event and patient fields.
- `internal/parser/cda/parser.go`, `internal/parser/cda/mapper.go`, and CDA section mapper registration points.
- Representative CCDA samples under `testdata/` or newly added minimal fixtures derived from public sample structures.
- Optional follow-up with the FHIR IG speclet only after canonical extraction behavior is proven.

## Sources

- `.loom/20-product-spec.md` - Goals and Requirements: CDA/CCDA Medications, Allergies, and Social History expansion.
- `docs/planning/README.md` - P3 backlog item "CDA/CCDA section expansion".
- `docs/planning/CDA-CCDA.md` - CCDA section table, CDA package layout, Source Profile integration, and implementation phases.
- `AGENTS.md` - Warnings-over-errors and canonical event mapping guidance.

## Assignment Note

An agent can pick this up independently by creating a CDA-focused branch, adding the kill-test first under the existing CDA parser tests, and touching only CDA parser/mapper fixtures plus the minimal planning status update needed to describe the result.
