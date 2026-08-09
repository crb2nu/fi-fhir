### 2026-08-08 — Slice 4.1d C1 lane claim (PHI audit immutability + export attribution)

- Lane: **S3-C1** (`.loom/31-sprint3-execution-specs.md`, "Lane S3-C").
  Branch `feat/phase4-slice-4-1d-phi-audit`, worktree
  `.worktrees/phase4-slice-4-1d-phi-audit`.
- **Claimed migration numbers** (verified free against `origin/main` @ `7111cca1`):
  - `internal/integration/processor/migrations/0004_audit_immutability.sql`
    (4.2a took `0003_operator_control_plane.sql`).
  - `internal/integration/session/migrations/0004_export_attribution.sql`
    (`0001`-`0003` used).
  - `internal/integration/batch/migrations/0003_batch_audit_immutability.sql`
    (4.1b3 took `0002_batch_provenance.sql`).
- **Owned files this sprint** (one-schema-owner rule):
  - `internal/api/graphql/schema.graphql` and every regenerated artifact —
    `internal/api/graphql/generated.go`, `internal/api/graphql/model/*`,
    `ui/src/lib/gen/graphql.ts`. The single schema change is
    `reason: String!` on `ExportIntegrationBundleInput`. 4.2b rebases onto the
    regenerated `ui/src/lib/gen/graphql.ts`; this lane touches no other `ui/`
    file.
  - `internal/integration/session/{types.go,store.go,postgres.go}` export path.
  - `internal/api/graphql/resolvers/integration_session_service.go` export path.
  - `docs/operations/PHI-RETENTION.md` (new).
  - `.gitlab-ci.yml`: appends `test:phi-audit` at the end of the `test` stage only.
  - `Makefile`: appends the `phi-audit` target only.
- **Not touched** (sibling lanes): `internal/integration/delivery/store.go`
  (4.2a), `internal/integration/authorization/policy.go` and
  `internal/integration/delivery/dispatcher.go` (S3-B), the serve component
  table and deploy manifests and smoke/check scripts (S3-A), `ui/src/**`
  except the regenerated codegen output (4.2b).
- **Day-1 gate result: both riskiest-assumption assertions PASSED**, so the
  spec's C1/C2 split stands unchanged and no correction to `.loom/31` is needed.
  Evidence in `.loom/iteration-plan-phase-4-slice-4-1d-c1-phi-audit.md`.
