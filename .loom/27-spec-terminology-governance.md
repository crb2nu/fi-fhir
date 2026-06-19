# 27 - Spec: Terminology Approval Workflow Hardening

**Status**: Ready for independent pickup after terminology DB baseline is known
**Lane**: F - Product expansion speclets
**Tracking**: libs/fi-fhir#17

## Goal

Define the remaining hardening scope for human-in-the-loop terminology governance: clinical safety, auditability, and SME trust around pending autoroute review.

This is not a request to rebuild the shipped GraphQL/UI approval workflow. The next agent should verify the existing pending-review surface, identify hardening gaps, and avoid duplicating Lane C's pending-expiry sweep and notification implementation details.

## Non-Goals

- Do not duplicate Lane C work for background expiry sweeps or notification dispatch.
- Do not edit terminology DB implementation until Lane D records a reliable integration-test baseline.
- Do not rebuild `PendingReviewList.svelte` or shipped GraphQL approve/reject mutations without a verified defect.
- Do not auto-approve clinical mappings as part of this hardening slice.
- Do not change LLM ranking prompts unless the hardening gap is specifically about unsafe or unauditable recommendations.

## Acceptance Criteria

- Existing governance capabilities are inventoried: pending list, approve, reject, bulk approve, decision trace, CLI review commands, and audit persistence.
- Review actions have a clear audit contract: reviewer identity when available, timestamp, source/target code systems, confidence, rationale, and original autoroute trace.
- Rejection and modification paths preserve enough context for later analytics and do not silently lose clinical reasoning.
- Approval promotion behavior is explicitly tested or documented: approved autoroutes become persistent mappings and are distinguishable from uploaded CSV mappings.
- SME safety controls are scoped: confidence thresholds, bulk approval limits, stale-review handling, and role/permission expectations.
- Remaining analytics/polish gaps are separated from operational automation owned by Lane C.

## Kill-Test

Before adding governance features, run a focused review-flow test that creates a pending autoroute, approves it, resolves the same source code again, and asserts the result is a persistent human-reviewed mapping with an auditable decision trace. If this cannot run because terminology DB integration tests are red, stop and depend on Lane D rather than patching around the test base.

## Dependencies

- Lane D terminology DB integration-test recovery for reliable store-level proof.
- Lane C only for eventual expiry/notification automation; this spec should not implement it.
- `docs/planning/TERMINOLOGY-MAPPING.md` for shipped and partial mapping workflow status.
- GraphQL schema/resolvers and UI review surfaces only after verification shows a concrete governance gap.
- `AGENTS.md` guidance on identifier/terminology warnings and profile-driven rules.

## Sources

- `.loom/20-product-spec.md` - Terminology Governance goal and ReviewRequired requirement.
- `docs/planning/README.md` - P3 terminology approval workflow backlog item.
- `.loom/24-parallel-execution-specs.md` - Lane C and Lane D coordination notes.
- `docs/planning/TERMINOLOGY-MAPPING.md` - Shipped approval workflow, pending autoroutes, telemetry, and remaining analytics/polish.

## Assignment Note

An agent can pick this up independently after Lane D by writing the kill-test against the current review flow, then implementing only governance gaps proven by that test or by a documented SME safety requirement.
