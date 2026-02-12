# Loom Context Pack

## Quick Links

- Workspace snapshot: `00-workspace-snapshot.md`
- MCP inventory: `00-mcp-inventory.md`
- Research: `10-research.md`
- Product spec: `20-product-spec.md`
- Implementation plan: `30-implementation-plan.md`
- Decisions: `40-decisions.md`
- Worklog: `50-worklog.md`

## Current Goal

- [x] Define an implementation-ready plan to enhance backend ETL and format parsing/transform capabilities.
- [x] Ensure the plan includes robust API contract governance and end-to-end auditability.
- [x] Produce research, spec, and implementation milestones with sourced evidence.

## Success Criteria

- [x] Identified current-state capability baseline and concrete gaps.
- [x] Defined phased plan with acceptance criteria and test strategy.
- [x] Captured decisions and worklog for follow-on implementation agents.

## Open Questions

- [ ] Should REST `/api/v1/*` endpoints be implemented to match `api/openapi.yaml`, or should OpenAPI be explicitly scoped to generated/reference docs while GraphQL remains the runtime API?
- [ ] What retention policy and PHI redaction strategy is required for persisted raw payloads and parse warnings in audit logs?
- [ ] Which ingestion modes are in scope for ETL phase 1 (batch only, or batch + incremental/scheduled sync)?

## Risks

- [ ] Contract drift between canonical events, GraphQL schema, and OpenAPI can create client breakage if not governed by compatibility checks.
- [ ] ETL and storage low coverage areas increase regression risk during core pipeline changes.
- [ ] Audit trail expansion may increase storage/PII handling burden without clear retention and access controls.
