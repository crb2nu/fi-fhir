# 28 - Spec: FHIR IG, Bulk Data, and SMART Scoping

**Status**: Ready for scoping and proof-first implementation planning
**Lane**: F - Product expansion speclets
**Tracking**: libs/fi-fhir#12

## Goal

Split the broad "advanced interoperability standards" product goal into a proof-first scope for FHIR IG conformance, Bulk Data export/import, and SMART App Launch support.

The next agent should produce or implement a small standards matrix before changing code, because the existing FHIR mapper already supports many US Core and Da Vinci resources while Bulk Data and SMART are broader protocol surfaces.

## Non-Goals

- Do not replace the existing R4/US Core mapper.
- Do not attempt full certification-grade IG conformance in one slice.
- Do not build a SMART app UI; scope server-side launch, authorization, and context contracts only.
- Do not implement a production job runner for Bulk Data before storage, authorization, and NDJSON manifest contracts are agreed.
- Do not make CDA section expansion depend on this work; consume canonical events only after they exist.

## Acceptance Criteria

- A standards/version matrix is created for the specific IGs and flows in scope: USCDI v3-related US Core expectations, Bulk Data `$export`/`$import`, and SMART App Launch.
- Current mapper support is compared to the matrix so already-shipped US Core, PAS, and PDex behavior is not duplicated.
- Bulk Data scope defines supported levels, minimum resource types, NDJSON output layout, job lifecycle states, download URL/security model, and storage backend assumptions.
- SMART scope defines supported launch mode, OAuth2/OIDC responsibilities, token validation boundaries, context parameters, and explicit non-support cases.
- Validation strategy is documented: existing structural validation, optional HL7 validator use, golden NDJSON fixtures, and API contract tests.
- Work is split into independently shippable follow-up tasks if the matrix shows Bulk and SMART are too large for one implementation lane.

## Kill-Test

Before implementing endpoints, create a minimal conformance matrix and run one sample Patient/Observation export fixture through the existing FHIR mapper plus validator path. If the current mapper cannot produce the minimum resources with required metadata, fix that narrow mapper gap before building asynchronous Bulk Data job machinery.

## Dependencies

- `docs/planning/FHIR-PROFILES.md` for current R4, US Core, Da Vinci, validation, and profile metadata behavior.
- `pkg/fhir/mapper.go` and current FHIR validation paths.
- Storage provider speclet for Bulk Data artifact storage and presigned/download URL assumptions.
- Source Profile and terminology governance work only when their canonical outputs or mappings affect FHIR resource content.
- External standards version verification at implementation start, because the planning doc may lag active IG versions.

## Sources

- `.loom/20-product-spec.md` - Advanced interoperability goals for USCDI v3, Bulk Data, and SMART App Launch.
- `docs/planning/README.md` - P3 backlog item for Additional FHIR Implementation Guides.
- `docs/planning/FHIR-PROFILES.md` - Existing US Core, Da Vinci, validation, and external validator guidance.
- `docs/planning/WORKFLOW-DSL.md` - Existing FHIR workflow action validation policy.

## Assignment Note

An agent can pick this up independently by first committing a standards matrix and kill-test fixture, then choosing the smallest code slice that proves one protocol path without blocking on the entire interoperability roadmap.
