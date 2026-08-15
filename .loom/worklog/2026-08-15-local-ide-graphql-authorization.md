### 2026-08-15 - Local IDE GraphQL authorization

- What changed: The local Docker Compose runtime and `.env.example` now give the server-owned `local-operator` both `integration:preview` and the `graphql:operator` compatibility grant. Both developer setup guides match, and `scripts/check-runtime-config.sh` requires both local artifacts to keep both roles.
- Why: The full UI mounted by Docker Compose immediately calls profile, workflow, event, terminology, and subscription roots. Those roots intentionally remain behind `graphql:operator`, but the local runtime provisioned only `integration:preview`, so a valid token unlocked the UI and then received `FORBIDDEN` across the workspace.
- Evidence: The new runtime-config assertions failed twice before the role correction and pass afterward (23 checks, 0 warnings, 0 failures); `docker compose config --quiet`, `bash -n scripts/check-runtime-config.sh`, `bash scripts/worklog.sh check`, `git diff --check`, and `go test ./internal/api/graphql/...` pass.
- What's next: Keep production Helm/Kubernetes preview defaults least-privilege, and resume the highest-priority open Sprint 5 execution lane.
- Sources:
  - [S1] `.loom/32-sprint4-execution-specs.md` — Lane S4-E compatibility-grant contract
  - [S2] `.loom/40-decisions.md` — `integration:preview` remains limited to health and stateless preview
  - [S3] `internal/api/graphql/operation_authorization.go` and `operation_authorization_roles.go`
