# 29 - Spec: Dynamic Source Profile Management and Observability

**Status**: Ready for independent pickup
**Lane**: F - Product expansion speclets

## Goal

Define a dynamic Source Profile management slice that lets operators create, update, validate, and observe feed-specific profiles at runtime while preserving the profile-driven parsing model.

The next agent should treat Source Profiles as control-plane configuration with audit and rollback needs, not just YAML CRUD.

## Non-Goals

- Do not remove static YAML profile loading.
- Do not change the core profile schema without a migration/compatibility plan.
- Do not build broad UI redesigns or workflow builder changes in this slice.
- Do not store raw PHI in profile-management audit logs or observability tags.
- Do not make every parser dynamically reload profiles until the consistency model is explicit.

## Acceptance Criteria

- A runtime Source Profile API contract is specified or implemented for list, get, create, update, validate/lint, activate, deactivate, and version history.
- Static YAML profiles and dynamic profiles have an explicit precedence and migration model.
- Profile updates are audited with actor, timestamp, version, validation result, and reason/comment when supplied.
- Parsing paths can resolve a profile by ID and report the exact profile version used in parse warnings, events, or trace metadata.
- Observability includes per-profile metrics already described by the planning docs: message type distribution, segment presence, encoding anomalies, identifier validity, terminology coverage, and Z-segment frequency.
- Message tracing links raw payload identity, parse phases, semantic events, terminology transforms, and triggered workflow actions without exposing PHI by default.
- Rollback or safe activation is defined so an invalid profile cannot immediately break production feeds.

## Kill-Test

Before building broad management APIs, prove a single profile lifecycle: create a draft profile, lint it against a sample message, activate it, parse with its profile ID, and verify the emitted event or parse result records that exact profile ID/version. If profile version provenance cannot be observed, implement that before adding more CRUD surface.

## Dependencies

- `docs/planning/SOURCE-PROFILES.md` for profile schema, parser integration, quality metrics, and profile-per-feed thesis.
- Existing profile CLI inference/linting from FB-001.
- GraphQL/API contract patterns if this becomes a runtime management API.
- Workflow observability and tracing primitives for cross-system message tracing.
- Terminology governance only where profile-embedded terminology mappings interact with pending or approved mappings.

## Sources

- `.loom/20-product-spec.md` - Multi-tenant Profile Management and message tracing requirements.
- `docs/planning/README.md` - Source Profile concept and shipped FB-001 inference/linting status.
- `docs/planning/SOURCE-PROFILES.md` - Source Profile schema, quality metrics, parser usage, and registry model.
- `docs/planning/WORKFLOW-DSL.md` - Existing metrics, tracing, replay, and workflow action observability.

## Assignment Note

An agent can pick this up independently by starting with the lifecycle kill-test and an API contract sketch, then implementing only the minimum runtime storage/provenance needed to make profile activation observable and reversible.
