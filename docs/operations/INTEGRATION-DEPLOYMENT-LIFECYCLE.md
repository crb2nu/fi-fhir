# Integration Deployment Lifecycle

## Purpose

Slice 2.1 adds a durable backend catalog for exact integration releases. The
catalog separates immutable tested content from mutable operational state.

Lifecycle controls are not yet exposed through GraphQL, REST, the CLI, or the
Mapping Studio. Slice 2.2's optional production MLLP adapter consumes the
catalog's deployed binding; authenticated HTTP ingress remains on the verified
startup registry.

## Versioned policy

Lifecycle-managed `IntegrationDefinitionRevision` values include a deployment
policy with four bounded areas:

| Area | Contract |
|---|---|
| Connection validation | Timeout and maximum evidence age |
| Schedule | Continuous or five-field cron with an IANA timezone |
| Health | Startup grace, interval, timeout, and failure threshold |
| Capacity | Maximum in-flight, queued, and per-second messages |

The policy participates in the revision digest. Legacy Slice 1 revisions omit
the policy and preserve their existing digest. The lifecycle catalog accepts
only revisions that pass `ValidateForDeployment`.

## State model

```mermaid
stateDiagram-v2
    [*] --> Draft
    Draft --> Draft: validation failed
    Draft --> Validated: validation passed
    Validated --> Approved: approve
    Approved --> Published: publish immutable release
    Published --> Deployed: deploy
    Deployed --> Paused: pause
    Paused --> Deployed: resume after fresh validation
    Published --> Retired: retire
    Deployed --> Retired: retire
    Paused --> Retired: retire
    Retired --> [*]
```

Every command supplies the expected snapshot version. A stale writer receives a
version conflict and creates no partial transition. Human commands require an
authenticated principal and reason.

Failed validation remains append-only evidence. It does not advance a draft and
invalidates older success evidence for later publication, deployment, or resume.

## Persistence boundary

The migration in
`internal/integration/lifecycle/migrations/0001_deployment_lifecycle.sql`
creates five data tables:

| Table | Mutability |
|---|---|
| `integration_definition_revisions` | Append-only |
| `integration_connection_validations` | Append-only |
| `integration_release_records` | Append-only |
| `integration_lifecycle_events` | Append-only |
| `integration_lifecycle_snapshots` | Expected-version updates only |

PostgreSQL triggers reject `UPDATE` and `DELETE` on append-only tables. A partial
unique index permits one deployed or paused revision per tenant and definition.

The catalog stores artifact references, safe validation codes, actor metadata,
and policy. It does not accept raw message bytes or inline secret values.

## Runtime resolution

`PostgresCatalog.ResolveRunnable` returns a server-owned binding containing the
release ID, snapshot version, health, exact integration/source revisions,
source ID, format, classification, deployment policy, and secret-binding names.
It returns no binding for draft, validated, approved, published, paused, or
retired state.

MLLP durable admission repeats this authorization inside the submission
transaction while holding a shared snapshot lock through commit. Pause and
retire take the conflicting update lock, so an admitted message linearizes
before the stop transition or fails closed after it.

The exact revision remains available for audit after pause or retirement. It is
not runnable until the state machine permits it.

## Verification

Run unit and contract tests:

```bash
go test -race -count=1 ./pkg/integration ./internal/integration/lifecycle
```

Run the PostgreSQL 16 kill-test with an isolated database:

```bash
POSTGRES_TEST_URL='postgres://user:pass@host:5432/db?sslmode=disable' \
  go test -tags=integration -race -count=1 \
  -run '^TestPostgresDeploymentLifecycle_RaceRestartImmutableRelease$' \
  ./internal/integration/lifecycle
```

The required `test:deployment-lifecycle` CI job also verifies that this exact
test exists before running it. The job has `allow_failure: false`.

## Rollback

The catalog is additive and not runtime-wired in Slice 2.1. Reverting the slice
leaves the existing startup registry and authenticated HTTP path unchanged. Do
not delete lifecycle records from a live database; preserve them for audit and
apply a reviewed forward migration.

## See also

- [Operations Runbook](RUNBOOK.md)
- [Production Hardening](PRODUCTION-HARDENING.md)
- [Production MLLP](PRODUCTION-MLLP.md)
- [Supported 1.0 Baseline](SUPPORTED-1.0.md)
- [Phase 2 iteration plan](../../.loom/iteration-plan-phase-2-slice-2-1-versioned-deployment-lifecycle.md)
