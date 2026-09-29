# Integration Deployment Lifecycle

## Purpose

Slice 2.1 adds a durable backend catalog for exact integration releases. The
catalog separates immutable tested content from mutable operational state.

Lifecycle controls are exposed through GraphQL and the Mapping Studio: the
operator control plane deploys, pauses, resumes, and retires a release
(`deployIntegrationRelease`, `pauseIntegrationDeployment`,
`resumeIntegrationDeployment`, `retireIntegrationDeployment`; the Operator
page), Integration Session publication publishes, approves, and deploys a
session's release when publication signing is configured
(`publishIntegrationSession`, `approveSessionPublication`,
`deploySessionPublication`), and the connection catalog shows which definition
revisions name each connection revision and their state (`Connection.references`,
the Connections page's Usage tab; see
[Connection catalog operations](CONNECTION-CATALOG.md)). REST exposes none of
them. The CLI's `fi-fhir lifecycle seed` is the supported way to create,
validate, approve, and publish a **batch** definition outside tests (see
[Seeding a batch definition from the CLI](#seeding-a-batch-definition-from-the-cli));
deploy can stay a Studio action. An MLLP definition still has no supported
path to a draft: the seed takes batch source revisions only. Slice 2.2's
optional production MLLP adapter consumes the catalog's deployed binding;
authenticated HTTP ingress remains on the verified startup registry.

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

## Seeding a batch definition from the CLI

`fi-fhir lifecycle seed` ([CLI reference](../user-guide/cli-reference.md#lifecycle-seed))
builds one `IntegrationDefinitionRevision` for a batch source and runs
`CreateDraft` → `ValidateConnection` → `Approve` → `Publish`, and with
`--through deployed` also `Deploy`. It is the sequence the batch proof's
`deployBatchRevision` helper runs, now an operator command. Every transition
names the snapshot version it read, the `--principal` (auth method
`postgres`: the database connection authenticates the operator, as for
`fi-fhir delivery replay`), and the `--reason`.

Where each part of the definition comes from:

| Field | Source |
|---|---|
| `source` | `--source`, the batch source revision the runner mounts: its ref and `source_id` |
| `profile`, `workflow` | the `--integration` entry of the static registry (`FI_FHIR_INTEGRATION_REGISTRY_PATH`) |
| `destinations` | each `--destination` revision's ref and class |
| `secret_bindings` | every name the source declares (provider `file`, key `batch/<name>`) and every name a destination declares (key `destinations/<name>`) |
| `deployment` | the policy defaults or flags; see the CLI reference |

**Why the profile and workflow come from the static registry.** The batch
runner never loads profile or workflow bytes from this catalog. `serve` builds
one artifact resolver over the static registry and hands it to the batch
runtime; at admission the processor resolves the deployed definition here,
then loads the definition's profile and workflow from the registry by
`(artifact_id, revision_id)` and refuses unless the recomputed digest equals
the definition's ref byte for byte. A profile or workflow published into the
Studio's own stores is not visible to that path. The seed therefore takes both
refs from one registry entry and resolves them with the same resolver before
writing anything, and refuses a workflow whose non-log actions deliver to a
destination that is not among `--destination` (the planner would refuse every
matching message).

**Idempotent and resumable.** Re-running with the same inputs resumes from the
stored state: a failed validation stays `draft` with its codes recorded, and
the next run validates again; stale evidence is refreshed before any gated
step. The creation audit is part of the digest, so resume compares the stored
revision with these inputs rebuilt under the stored audit. Different content
under the same revision ID is refused, never overwritten, because the tables
are append-only; seed the change as a new `--revision-id`. A `paused` or
`retired` revision is reported, not advanced.

**Deploying from the Studio.** The default `--through published` leaves the
deploy to the Operator page (`deployIntegrationRelease`). `Deploy` requires
validation evidence younger than the policy's max age (300 s by default), and
`serve`'s catalog has no connection validator, so deploy within the window
printed as `validation.expires_at`. Once it has passed, re-run the same seed
command: at `published` it refreshes the evidence and stops again.

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

The seed command's PostgreSQL proofs seed through `published`, resume to
`deployed`, and hand the result to the batch runtime `serve` builds, which must
ingest and archive a file; they also prove a changed source is refused with
nothing written and a failed validation resumes once fixed:

```bash
POSTGRES_TEST_URL='postgres://user:pass@host:5432/db?sslmode=disable' make lifecycle-seed
```

The required `test:lifecycle-seed` CI job asserts the three proof names exist
before running them. The flag, dry-run, destination-registry, validator, and
resume-state tests need no database and run in `test:unit`.

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
