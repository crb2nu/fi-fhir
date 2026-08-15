# RALPH Iteration Plan

## Review

- Roadmap milestone: Sprint 5 release-candidate hardening
- Spec sections: `.loom/32-sprint4-execution-specs.md` Lane S4-E; `.loom/40-decisions.md` GraphQL preview boundary
- Prior decisions to preserve: `integration:preview` remains limited to `health` and `previewIntegrationMessage`; `graphql:operator` remains an explicit compatibility grant; production preview defaults are not widened

## Align

- Slice name: Local full-IDE GraphQL operator access
- Scope in: local Docker Compose and `.env.example` operator roles, local development documentation, and a required drift check
- Scope out: production Helm/Kubernetes preview roles, new roles, transport-gate changes, resolver changes, and OIDC policy
- Acceptance criteria:
  - the local full-IDE runtime carries both `integration:preview` and `graphql:operator`
  - the runtime-config check fails if either local artifact drops either role
  - production preview deployment defaults remain `integration:preview` only
  - GraphQL authorization tests remain green
- Dependencies/blockers: Git metadata is read-only in this environment, so commit/push/CI verification may require handoff

## Land

- Planned file areas: `docker-compose.yaml`, `.env.example`, `scripts/check-runtime-config.sh`, `docs/developer-guide/development-setup.md`
- Implementation steps:
  1. Add a required failing runtime-config assertion for the two local artifacts.
  2. Correct both local operator role declarations and explanatory copy.
  3. Make the local development command match the working full-IDE role set.

## Prove

- Tests to run: `bash scripts/check-runtime-config.sh`; `go test ./internal/api/graphql/...`
- Lint/static checks: `bash -n scripts/check-runtime-config.sh`; scoped whitespace/diff checks
- CI checks: unavailable until Git write/push access is restored

## Handoff/Harvest

- Docs to update: local development setup and a worklog entry
- Agent-context entries to add: root cause, preserved production boundary, validation evidence
- Next-slice candidates: resume the highest-priority open Sprint 5 lane after reviewing `.loom/33-sprint5-execution-specs.md`
