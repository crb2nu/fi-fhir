# 31 - Sprint 3 Execution Specs

**Status**: Ready for agent pickup (created 2026-08-08)
**Owner**: platform
**Base commit read**: `origin/main` @ `ae95d55b` (merge `ci/integration-ci-hardening`). Citations to `cmd/fi-fhir/main.go`, `pkg/config/config.go`, `.gitlab-ci.yml`, `.env.example`, and `internal/terminology/autoroute/notify.go` are **origin/main line numbers**; every other file is byte-identical to `ea760bc3`.
**In-flight branches read**: `origin/feat/phase4-mllp-cert-identity` (4.1b2, MR !134), `feat/phase4-batch-workload-identity` @ `5a72e05f` (4.1b3, stacked), `origin/feat/phase4-operator-control-plane` @ `af80a950` (4.2a).
**Sprint entry assumption**: 4.1b2, 4.1b3, and 4.2a are merged to `main` before Sprint 3 lanes branch. 4.2b (operator UI) may still be in flight.

**Inputs**: `.loom/30-implementation-plan-integration-engine-ide-completion.md`, `.loom/20-product-spec-integration-engine-ide-completion.md`, `.loom/slice-handoff-phase-4-slice-4-1b1-oauth-http-submit-authorization.md`, `.loom/24-parallel-execution-specs.md` (style model).

## Goal

Turn Slices 4.3, 4.1c, and 4.1d into three lanes that can run in parallel without colliding on files or re-litigating shipped behavior — and correct, from code, the parts of the plan text that are wrong.

## Non-Goals

- Do not implement in this planning slice.
- Do not reopen 4.1a/4.1b1/4.1b2/4.1b3 identity contracts; extend them.
- Do not duplicate 4.2a's PHI-minimal payload renderer or its operation ledger.
- Do not begin 4.4 (backup/restore, chaos, rolling upgrade, numeric budgets under load). Several items below are explicitly pushed there.

---

## Current-State Corrections From Code

This section is adversarial toward the plan text and toward the sprint scope description. Each item is what the code actually does.

### Observability (Lane S3-A scope)

**1. `/health` is a hardcoded string literal. Gate 0B did not make it "real".**

`internal/api/graphql/server.go:180-184` mounts one handler that unconditionally writes `200` and the byte string `{"status":"healthy","service":"graphql"}`. It checks nothing — no database, no store, no background component. `scripts/smoke-test.sh:59-63` asserts only that the body contains the substring `status`, which that literal satisfies forever. Gate 0B's "real readiness" work was about **test-harness readiness barriers** (waiting for the process, deterministic subscription-ready barriers) — see `.loom/iteration-plan-gate-0b-runtime-verification.md:30,89` — not about the endpoint's semantics. The scope description's premise is wrong.

**2. `/ready` is not mounted anywhere in `serve`.** Lane E already recorded this (`.loom/24-parallel-execution-specs.md`, "E as shipped"). Confirmed independently: the only `/ready` route in the repo is `internal/workflow/health.go:68`, inside a `HealthService` whose constructor `NewHealthService` (`internal/workflow/health.go:88`) has **zero non-test callers**. That service already implements everything 4.3 asks for — liveness/readiness split, concurrent checks with timeout, 1-second readiness caching, `503` on unhealthy (`:246-259`), plus DLQ-depth, circuit-breaker, HTTP, and engine checkers (`:274-459`). 4.3 is a **wiring and check-authoring** slice, not a build-from-scratch slice.

**3. The GraphQL `health` query is hardcoded too — and it is what CI smoke-tests.** `internal/api/graphql/resolvers/schema.resolvers.go:2118-2145` returns `Status: "healthy"` and component `{Name: "event_store", Status: "healthy"}` **without touching the database**. `scripts/smoke-test.sh:68-72` runs `{health{status}}` and greps for `health`. So the assertion labeled "authenticated health query succeeds" proves the resolver executes, not that any dependency is alive.

**4. `/metrics` does not exist, and the prior finding understates the problem.** The prior finding ("the only Prometheus code is `internal/workflow/metrics_prometheus.go`") is correct as far as it goes — `NewPrometheusMetrics` (`internal/workflow/metrics_prometheus.go:85`) has zero non-test callers, and `SetGlobalMetrics`/`SetGlobalTracer` (`internal/workflow/metrics.go:458`, `internal/workflow/tracing.go:369`) are never called from production code, so `GetGlobalMetrics()` (`metrics.go:465`) permanently returns `NoOpMetrics` (`metrics.go:40`) and every `GetGlobalMetrics().HTTPRequestCompleted(...)` in `internal/workflow/actions.go:600,610` discards its data.

But a complete **observability façade** ships around that nonexistent endpoint:

| Artifact | Claim | Reality |
|---|---|---|
| `deploy/kubernetes/base/deployment.yaml:19-22` | `prometheus.io/scrape: "true"`, port `9090`, path `/metrics` | nothing listens on 9090 |
| `deploy/kubernetes/base/deployment.yaml:42-44` | `containerPort: 9090` named `metrics` | unbound |
| `deploy/kubernetes/base/service.yaml` | **two** Services target `metrics` | both black-hole |
| `deploy/docker/prometheus.yml:26-30` | scrape job `fi-fhir` → `fi-fhir:9090/metrics` | never succeeds |
| `dashboards/grafana/workflow-overview.json` | panels on `workflow_events_processed_total`, `workflow_dlq_depth`, … | never emitted |
| `dashboards/alerting/workflow-alerts.yaml` (18 rules) + `workflow-alerts-k8s.yaml` (14 rules) | 32 alerts | can never fire |
| `docker-compose.yaml:24,65` | publishes `9090:9090`, sets `FI_FHIR_METRICS_PORT` | unbound |
| `docs/operations/README.md:59` | "Prometheus metrics are exposed at `/metrics`" | false |
| `cmd/fi-fhir/main.go:3197` (`config env`) | `FI_FHIR_METRICS_ENABLED` — "Enable Prometheus metrics endpoint" — default `true` | inert |
| `.env.example:173-179` | Observability block | inert |

And `pkg/config.ObservabilityConfig` (`pkg/config/config.go:181-195`) is fully defaulted (`:417`), env-applied (`:606-613`), and validated (`:759-769`) — with **zero consumers**: `grep -rn '\.Observability' internal/ cmd/` outside `pkg/config` returns nothing. `runServe` reads only `runtimeConfig.Terminology.*` and `runtimeConfig.LLM.*` (`cmd/fi-fhir/main.go:4845,4872-4873,4913-4919`).

**5. Kubernetes probes do not probe the server.** `deploy/kubernetes/base/deployment.yaml:136-150` uses `exec: ["/fi-fhir","version"]` for **both** liveness and readiness. That proves a subprocess can print a version string. A wedged GraphQL handler, a dead connection pool, or a crashed MLLP listener all pass, and the Service keeps routing to the pod. The Helm chart instead hits `/health` (`deploy/helm/fi-fhir/templates/deployment.yaml:162-178`) — i.e., the static literal from item 1. Truthful probes are therefore a **manifest change plus a code change**, and the two deployment paths currently disagree.

**6. Dead e2e tests already assert the desired behavior and would fail today.** `test/e2e/integration_test.go:324-347` asserts `health["status"] == "ok"` — the handler says `"healthy"`. `:351-385` asserts `/metrics` returns `200` containing `workflow_events_processed_total`. Neither runs: the file is `//go:build e2e && integration` (`:8`) and no `.gitlab-ci.yml` job passes `-tags=e2e` (only `Makefile:62,66` do, and they need `docker-compose`, which has no local Docker Desktop). Both `t.Skipf` on connection error, and the metric-content check is `t.Logf`, not `t.Errorf`. **This is the identical failure shape as Lane E's MinIO finding**: a test that cannot fail, guarding a claim that is false. S3-A must delete or repair these, not leave them.

**7. Deferred-metrics `Observe` seams — complete enumeration.** Exactly two components have one:

- `internal/terminology/autoroute/sweeper.go:44` — `Observe func(SweepResult, error)`; serve prints it at `cmd/fi-fhir/main.go:4851-4860`.
- `internal/terminology/autoroute/notify.go` — `Observe` + `ObserveDelivery`; serve prints them at `cmd/fi-fhir/main.go:4881-4904`.

Everything else has **no observation seam at all**: the MLLP server/service, the delivery `Dispatcher` (`RunOnce` returns a typed `Outcome` at `internal/integration/delivery/dispatcher.go:50-86` that `Run` discards), the batch `Runner`, the session `Hub`, the GraphQL server, and the processor. Additionally, `PostgresCatalog.ReportHealth` (`internal/integration/lifecycle/queries.go:164-165`) — the one path that would write runtime health into `integration_lifecycle_snapshots.health` — has **zero non-test callers**, so that column is never updated after deployment.

**8. There is no structured logging anywhere.** `grep -rn '"log/slog"'` across `internal/ pkg/ cmd/` returns nothing. `serve` uses `fmt.Println` / `fmt.Fprintf(os.Stderr, …)`; there are five `log.Printf` calls total (`internal/workflow/actions.go:632,638`, `internal/api/graphql/server.go:216,218,221`). `internal/workflow/logging.go` contains a complete trace-correlating `StructuredLogger` with JSON output — `NewStructuredLogger` (`:120`) has zero non-test callers. So "correlation-safe logs" in the plan text has **no logger to correlate with**; that is a build item, not a hardening item.

**9. Correlation IDs already exist end-to-end — in records, not in logs.** HTTP ingress accepts `X-Correlation-ID` (`internal/integration/ingress/http.go:21,130-134`); MLLP generates one per frame (`internal/integration/mllp/service.go:118`); batch derives a deterministic UUID from object identity (`internal/integration/batch/service.go:276`). It is validated for equality across result/receipt/event (`pkg/integration/contracts.go:562,693,888`), persisted on `integration_receipts`, `integration_canonical_events`, and `integration_message_lineage`, and a separate `trace_id` is persisted on lineage/attempts/outbox and put on Kafka headers (`internal/integration/delivery/dispatcher.go:169-177`). **4.3's correlation work is emission, not plumbing.**

**10. Multi-replica classification — the plan text's "in-memory fanout" is one of five problems, not the only one.**

| Component | Verdict | Evidence |
|---|---|---|
| Delivery dispatcher | **Already safe** | `FOR UPDATE OF o SKIP LOCKED` + lease (`internal/integration/delivery/store.go:95-104`); every transition is fenced on `lease_owner = $n AND lease_expires_at > $now` (`:178,235-236,305`); worker ID defaults to `hostname-pid` (`cmd/fi-fhir/delivery_runtime.go:40-47`) so replicas differ. |
| Batch store | **Safe** | Row-lock + lease + reclaim (`internal/integration/batch/store.go:150-208`); every advance is owner-guarded (`:228,268,305,354`). |
| **Batch runtime as documented** | **Unsafe** | `FI_FHIR_BATCH_WORKER_ID` is a **required env var** (`cmd/fi-fhir/batch_runtime.go:44`) and both `.env.example:90` and `docs/operations/BATCH-INGESTION.md:38` hand out the single literal `fi-fhir-batch-1`. `store.go:180` treats a *matching* owner as re-lease, so two replicas sharing that ID steal each other's live leases and process the same object concurrently. The CI "replica exclusion" proof cannot catch this: it uses distinct IDs `worker-a`/`worker-b`/`worker-c` (`internal/integration/batch/batch_integration_test.go:190-210`). |
| Lifecycle / admission | **Safe** | `pg_advisory_xact_lock` for migration (`internal/integration/lifecycle/postgres.go:61`), optimistic `expected version` commands, transaction-scoped admission authorizer. |
| Autoroute sweeper | **Benign duplicate** | `ExpirePendingAutoroutes` is an idempotent `UPDATE … WHERE status='pending' AND expires_at < NOW()` (`pkg/terminology/db/mappings.go:1029-1039`). Two replicas waste a query; no external effect. Needs a lease only if you care about the wasted query. |
| **Autoroute notifier** | **Double-fires** | De-duplication is a per-process `seen map[int64]struct{}` + bounded FIFO (`internal/terminology/autoroute/notify.go:292-293`, `markUnseen` at `:403`). N replicas page reviewers N times; every restart re-pages the whole backlog. |
| **MLLP capacity** | **Multiplied** | `capacityGate` is one per `Service` (`internal/integration/mllp/service.go:53,80`), i.e. per process. `CapacityPolicy{MaxInFlight, MaxQueued, MaxMessagesPerSecond}` (`pkg/integration/deployment.go:61-66`) is an immutable declared policy of the revision — with N replicas the deployment serves N× what the revision declares. |
| **Session SSE hub** | **Needs fanout** | See item 11. |

**11. The SSE fanout failure is precise and reproducible.** `internal/integration/session/hub.go:34-95` is a process-local `map[string]subscription` with a 32-slot buffered channel per subscriber and a `default:` drop on full (`:90-93`). Exactly one `Hub` exists per process: `newIntegrationSessionServiceWithStore` constructs it (`internal/api/graphql/resolvers/integration_session_service.go:36`) and the resolver constructs the service once (`internal/api/graphql/resolvers/resolver.go:167,245`). `Runner.RunHL7v2` publishes stage events into that same in-process hub (`internal/integration/session/runner.go:237`). The subscription is a long-lived `POST /graphql` with `Accept: text/event-stream` (`internal/api/graphql/server.go:129,390,419`). **Failure mode with two replicas behind a normal L7 load balancer:** the client's SSE POST pins to replica A while its `runIntegrationSample` mutation POST lands on replica B; B publishes `run_started … run_completed` into B's hub; A's subscriber receives nothing. The run still completes durably, so the UI shows a stalled stream over a succeeded run.

**12. `deploy/kubernetes/base/deployment.yaml:10` already sets `replicas: 2`** (Helm `values.yaml:5` likewise), with `pdb.yaml` `minAvailable: 1`. The checked-in manifests already assume the behavior 4.3 is supposed to deliver.

### Delivery identity (Lane S3-B scope)

**13. The durable delivery worker never contacts a destination.** `Dispatcher.RunOnce` (`internal/integration/delivery/dispatcher.go:50-86`) claims one outbox row, marshals a `deliveryCommand`, and publishes it to Kafka. That is the whole dispatch path. The topic is a **constant**, `"integration.delivery.v1"` (`internal/integration/processor/postgres_submission.go:32`, written at `:376`) — one topic for every destination of every tenant. `cmd/fi-fhir/delivery_runtime.go:29-121` builds exactly one publisher, with one process-wide SASL credential (`FI_FHIR_QUEUE_USERNAME` / `FI_FHIR_QUEUE_PASSWORD`, `:83-98`).

Therefore, the scope question "what identity does the engine present to destinations?" has a factual answer: **none, because the engine does not call them.** An external consumer of `integration.delivery.v1` performs the actual destination call. Slice 2.3's "webhook/FHIR/database/file" are **plan-level action classes**, validated at `internal/integration/processor/workflow_plan.go:147-154` and turned into identical Kafka commands — not executed transports. The product spec's 1.0 destination matrix ("webhook, FHIR R4, PostgreSQL, file, and Kafka", `.loom/20-product-spec-integration-engine-ide-completion.md:262`) is therefore satisfied by Kafka only.

**14. The one real HTTP destination call in the repo takes plaintext inline credentials — and it is legacy.** `webhookAction` (`internal/workflow/actions.go:501-618`) reads `config["token"]` and `config["authorization"]` straight from workflow action config and sets them as headers (`:542-547`). It is registered on the legacy `workflow.Engine` (`internal/workflow/engine.go:126`). The **published** production DSL cannot carry it: `publishedActionDTO` has only `id`, `type`, and `destination` (`internal/workflow/published_yaml.go:581,709`), with no config map. So inline-credential actions are reachable only through the legacy GraphQL catalog, which requires the blanket `graphql:operator` role (item 20).

**15. A destination artifact does not exist as a resolvable thing.** `integration.DestinationRevisionRef` (`pkg/integration/revision.go:72-76`) is `{artifact_id, revision_id, digest, class}` — no URL, no auth mode, no secret binding. Nothing ever loads destination bytes: `processor.ResolvedArtifactRevisions` (`internal/integration/processor/revisions.go:51-56`) holds **only** `profileJSON` and `workflowYAML`. There is no destination store, no destination resolver, and no destination content digest verification. **S3-B cannot bind a scoped credential to a destination until a destination revision artifact exists**, analogous to the profile/workflow revision boundary proved in Slice 1.1a.

**16. Fail-closed unmapped destinations is already shipped.** `internal/integration/processor/workflow_plan.go:105-111`: a planned non-`log` action with an empty `DestinationArtifactID`, or one whose artifact ID is absent from `revision.Destinations`, returns `ErrInvalidWorkflowPlan`. Do not spec this as new work; spec it as a regression guard.

**17. The Slice 1.0 secret-reference contract is, in practice, a name-presence check.** `integration.SecretReference{Provider, Key, Version}` and `SecretBinding{Name, Reference}` exist (`pkg/integration/revision.go:87-98`) and are validated (`:458,573`). At runtime only the **name** is checked: `hasSecretBinding` in `internal/integration/mllp/source.go:257` and `internal/integration/batch/source.go:167`. The **value** always comes from a fixed process env var or file: `cmd/fi-fhir/batch_runtime.go:100-147`, `cmd/fi-fhir/delivery_runtime.go:79-107`. **There is no `SecretReference.Provider` resolver of any kind.** S3-B's "secret-reference contract from Slice 1.0" therefore needs a resolver built, not consumed.

**18. Adding `integration.deliver` requires editing the single most contended authorization function.** `authorization.Authorize` (`internal/integration/authorization/policy.go:58-75`) opens with `if request.Action != ActionSubmit || request.Object.Kind != ObjectIntegrationRevision { return ErrForbidden }`, and then hard-requires `principal.Kind == PrincipalKindService` **and** a non-empty `principal.SourceID` matching the object. A delivery principal has no source. Five call sites depend on the current shape: `ingress/service.go:133`, `processor/message_processor.go:142`, `lifecycle/admission.go:29`, `mllp/service.go:141`, `batch/service.go:278`.

**19. Role naming is already split three ways. S3-B must pick one and say so.**
- Colon compatibility grants: `integration:submit`, `integration:mllp`, `integration:batch` (`authorization/policy.go:24-28`).
- Colon transport roles: `graphql:operator`, `integration:preview` (`internal/api/graphql/operation_authorization.go:17-18`).
- Dotted fine-grained roles: `integration.delivery.operator` (`internal/integration/delivery/types.go:17`, already on main from 2.3) and, from 4.2a, `integration.operator` / `integration.deployment.operator` (`internal/integration/operator/types.go:17-19`).
The action namespace is dotted (`integration.submit`). Recommendation: `integration.deliver` as the **action**, `integration.destination.client` as the **grant**, matching the dotted fine-grained convention that 2.3 and 4.2a already use.

**20. `graphql:operator` is a blanket bypass, and 4.2a did not narrow it.** `operationAuthorization.MutateOperationContext` returns `nil` — allow everything — for any caller holding `graphql:operator` (`internal/api/graphql/operation_authorization.go:50-52`). `integration:preview` is limited to `health` + `previewIntegrationMessage` (`:90-117`) and the stream context to two subscriptions (`:70-88`). 4.2a's diff **does not touch `operation_authorization.go`**, so every new control-plane query and mutation is reachable at the transport gate only under the blanket role, with the fine-grained `integration.operator` / `integration.deployment.operator` / `integration.delivery.operator` checks one layer deeper in `operator.Service.authorize`. That layering is defensible but it means the transport gate is still binary. Flag it; do not silently inherit it.

### PHI and audit (Lane S3-C scope)

**21. "Immutable audit storage" is partially real, and the durable-runtime half is the missing half.** `reject_integration_lifecycle_mutation()` triggers exist on exactly four tables — `integration_definition_revisions`, `integration_connection_validations`, `integration_release_records`, `integration_lifecycle_events` (`internal/integration/lifecycle/migrations/0001_deployment_lifecycle.sql:87-105`) — plus `integration_session_publications` and `integration_session_workflow_simulations` (`internal/integration/session/migrations/0003_publications.sql:26-47`). **No trigger** guards `integration_delivery_audit`, `integration_delivery_operations`, `integration_batch_audit`, `integration_receipts`, `integration_canonical_events`, or `integration_message_lineage`. Those are append-only *by code convention*; the schema permits `UPDATE` and `DELETE` by the application role.

**22. Retention/TTL/encryption fields are carried and, where checked, production *refuses* them.** `RawRetentionPolicy` (`pkg/integration/revision.go:108-157`) is a genuinely good deny-by-default contract with mode/TTL/purpose/storage/encryption-key/authorizer/access-audit and cross-field validation. But the durable committer rejects anything but `ephemeral`: `internal/integration/processor/postgres_submission.go:171-172` returns `ErrUnsupportedRawRetention`, and the receipt is written with a hardcoded `RawRetentionMode: ephemeral` (`:468`). **There is no retained production raw PHI whose TTL could be enforced.** The plan text's "retention/TTL/encryption enforcement" as written has no subject.

**23. The PHI that actually persists has no policy field, no TTL, no purge, and no expiry column.** `grep -ri 'ttl|expires|retention|purge' internal/*/*/migrations/*.sql` returns only **lease** columns. What is retained indefinitely:
- `integration_canonical_events.payload_json` — the canonical clinical event, written with `event.Classification` = the revision's PHI classification (`postgres_submission.go:254-272`; `internal/integration/processor/adt_a01.go:209` sets `DataClassificationPHI`). No expiry.
- `integration_session_samples.raw_cipher` — AES-256-GCM ciphertext under `PHIPolicyRetain` (`internal/integration/session/postgres.go:289-297`, `protector.go:34-50`). Encryption is real; **there is no TTL and no purge job**.
- `integration_session_samples.record_json` — the *redacted* raw under the default `PHIPolicyRedact` (`postgres.go:281-283`).
- `integration_session_exports.record_json` — a full snapshot of the above, per export (`postgres.go:886-899`).

**24. Export controls are half-built and *unattributed*.** The store always strips retained raw from a bundle (`internal/integration/session/postgres.go:861-865`), and the GraphQL layer strips redacted raw unless the caller passes `includeRawPayload: true` (`internal/api/graphql/resolvers/integration_session_service.go:480-484`; schema at `internal/api/graphql/schema.graphql:1137,1180`). But `integration_session_exports` has columns `(tenant_id, session_id, export_id, exported_at, record_json)` only (`internal/integration/session/migrations/0001_session_workspace.sql:82-91`) — **no principal, no reason**. That directly contradicts the product spec: "Terminology approvals, artifact publication, deployment, replay, and **data export** record actor, reason, timestamp, and revision" (`.loom/20-product-spec-integration-engine-ide-completion.md:220-222`). This is the cheapest, highest-value item in 4.1d.

**25. 4.2a already shipped the "policy-aware semantic payload rendering" policy.** `internal/integration/operator/payload.go:17-40` renders canonical payloads as **structure only** — field coordinates and JSON kinds, never values, with non-canonical keys collapsed to `"*"` and arrays collapsed to one entry. S3-C must reference and reuse this, not invent a second redaction policy.

**26. 4.1b3 establishes the program's "trusted provenance" idiom — reuse it verbatim.** `internal/integration/batch/migrations/0002_batch_provenance.sql` renames `object_modified_at` → `remote_modified_at_advisory` with a `COMMENT ON COLUMN` stating "advisory only, never a trust input", adds server-owned `object_version` / `object_etag` / `digest_state`, and adds the provenance CHECK as `NOT VALID` so pre-existing rows stay visibly distinguishable rather than being retroactively vouched for. S3-B's "trusted provenance on delivery results" should be the same shape: server-owned facts, remote facts explicitly labeled advisory, and no retroactive vouching.

### CI and process constraints

**27. `test:integration` runs exactly two package sets.** `./cmd/fi-fhir/...` then `./pkg/terminology/db/`, against shared `postgres` + `minio`, with `pkg/terminology/db` in its own database `fi_fhir_terms_test` and a 900s timeout (`.gitlab-ci.yml:569-595`). **A new test under `internal/integration/...` does not run in `test:integration`** and needs its own isolated proof job.

**28. The isolated-proof-job pattern is a two-step existence-then-execution contract.** `test:mllp-runtime` (`.gitlab-ci.yml:791-823`) first runs `go test -tags=integration -list '^TestX$' ./pkg | rg -x 'TestX' | awk 'END { if (NR != 1) exit 1 }'`, then `make mllp-runtime`. The `-list` step exists because integration helpers `t.Skipf` — not `t.Fatalf` — when a service is unreachable, so a broken service makes the job **greener** (`AGENTS.md:264-268`). Every new required job in Sprint 3 copies this exactly: dedicated Postgres service, dedicated database name, `-list | rg -x | awk NR!=N`, a `make` target, `allow_failure: false`.

**29. `make test-integration` (`Makefile:66`) is not CI's `test:integration`.** The Makefile target runs `go test -tags=e2e,integration ./test/e2e/...` and needs `docker-compose`. Do not confuse them in a runbook.

**30. GraphQL codegen is one canonical command with a diff gate.** `cd internal/api/graphql && go run github.com/99designs/gqlgen generate --config gqlgen.yml` then `git diff --exit-code -- .` (`.gitlab-ci.yml:259-280`, `Makefile:283-291`). `internal/api/graphql/generated.go` is a ~34k-line regenerated blob (4.2a's diff rewrites it entirely). **Two concurrent lanes touching `schema.graphql` will conflict irreconcilably.**

---

## Parallelization Map

| Lane | Slice | Can start at sprint open? | Parallel with | Must coordinate with | Primary risk |
|---|---|---:|---|---|---|
| **S3-A** | 4.3 truthful observability + multi-replica | Yes | S3-B, S3-C | 4.2b (no overlap), S3-B/S3-C on `cmd/fi-fhir/main.go` | Building a metrics stack nobody agreed to; conflicting on serve wiring |
| **S3-B** | 4.1c destination-scoped identity | **Yes, but re-scoped** (see spec) | S3-A, S3-C | S3-C on `delivery/store.go`; 4.2a is merged so `Discard` is fixed | Building credential binding on a destination artifact that does not exist |
| **S3-C** | 4.1d PHI/audit enforcement | Yes, **as C1 only** | S3-A, S3-B | S3-B on `delivery/store.go`; owns migration numbering | Specifying TTL enforcement for data that is never retained |

### Exact shared-file risks

| File | Lanes | Rule |
|---|---|---|
| `cmd/fi-fhir/main.go` (`runServe`, ~`:4728-5200`) | **A, B, C** | A owns the observability block and the `errCh` / `waitForBackgroundStops` component table (`:5086`, `:5129-5164`). B appends its destination-identity loader **after** the delivery block (`:5096-5101`). C appends its retention-sweeper start **after** the autoroute block (`:5108-5120`). No lane edits another lane's block. A merges first because it restructures the component table. |
| `internal/integration/delivery/store.go` | **B, C** | 4.2a already owns `MarkFailed`/`recover`/`Discard`. B does **not** touch this file — B's dispatch changes live in `dispatcher.go`/`types.go`/a new `identity.go`. C adds only the immutability trigger in a migration, never in this file. |
| `internal/integration/authorization/policy.go` | **B only** | The `Action != ActionSubmit` guard at `:59` must be widened. C's audit work must not add an action here; if C needs `integration.export`, it lands **after** B's refactor and reuses B's shape. |
| `internal/api/graphql/schema.graphql` + `generated.go` + `model/models_gen.go` + `ui/src/lib/gen/graphql.ts` | **C, and 4.2b** | **One schema owner per sprint.** Assign it to C (export actor/reason input). S3-A must **not** expose health/metrics via GraphQL — use plain HTTP handlers. If 4.2b is still open, C waits for 4.2b to merge or 4.2b rebases; do not run both. |
| `internal/integration/processor/migrations/` | **C** | 4.2a took `0003_operator_control_plane.sql`. C takes `0004_*`. Claim the number in `.loom/50-worklog.md` before writing. |
| `internal/integration/session/migrations/` | **A, then C** | **Corrected by S3-A, 2026-08-08.** As written this row assigned session `0004_*` to C, but Lane S3-A task 6 (durable session fanout) needs a session migration and A merges first. **A takes `0004_session_stream_events.sql`; C takes `0005_*`** for export attribution. Claimed in `.loom/50-worklog.md`. |
| `internal/integration/batch/migrations/` | — | 4.1b3 took `0002_batch_provenance.sql`. Next free `0003_*`. |
| `.gitlab-ci.yml` | **A, B, C** | Append new jobs at the end of the `test` stage, distinct names: `test:observability-replicas` (A), `test:delivery-identity` (B), `test:phi-audit` (C). Do not modify existing jobs' `services:` blocks. |
| `deploy/kubernetes/base/*`, `deploy/helm/fi-fhir/*` | **A only** | Probes, ports, Services, annotations. |
| `scripts/smoke-test.sh`, `scripts/check-runtime-config.sh` | **A only** | B and C add env vars; A owns the assertion scripts and folds new vars into `check-runtime-config.sh` on request. |
| `.env.example`, `docs/operations/*` | all | Distinct sections; conflicts are trivial. **A** owns the batch-worker-ID correction (`.env.example:90`, `docs/operations/BATCH-INGESTION.md:38`) because it is a multi-replica defect. |

### Coordination rules

- One branch/worktree per lane, under the repo's own `.worktrees/` (AGENTS.md policy), named `feat/phase4-slice-4-3-observability`, `feat/phase4-slice-4-1c-destination-identity`, `feat/phase4-slice-4-1d-phi-audit`.
- Before the first commit, each lane records its owned files and its claimed migration number in `.loom/50-worklog.md`.
- **A lane that discovers a false premise corrects the source planning doc before writing code.** This document's Corrections section is the model; every item in it was verified against a file and a line, and three of them (items 13, 15, 22) invert a slice's stated scope.
- Any change to `schema.graphql` goes through `make lint-gqlgen` and commits the regenerated artifacts in the same commit. Never hand-edit `generated.go`.
- No lane promotes an existing CI job to blocking. Each lane adds its own blocking job.
- Every new proof job must include the `-list | rg -x | awk 'END { if (NR != N) exit 1 }'` existence step. A job that can pass because a test was skipped or renamed is not a proof.

---

## Lane S3-A — Slice 4.3: Truthful Observability and Multi-Replica Behavior

**Branch**: `feat/phase4-slice-4-3-observability`

### Goal

Make `/health`, `/ready`, and `/metrics` report real component state; make deployed probes and scrape config match what the process actually serves; and make every background component correct under the `replicas: 2` that the checked-in manifests already declare.

### Non-Goals

- No OpenTelemetry trace exporter wiring. `FI_FHIR_TRACING_*` stays inert and gets an explicit "not implemented" note in `.env.example` and `docs/operations/README.md`. Tracing belongs to 4.4.
- No load-generated cardinality or latency budget validation. That is 4.4's "every numeric budget passes on the pinned reference profile".
- No leader election for anything that is already lease-correct (delivery, batch store, lifecycle).
- No GraphQL surface for health or metrics. Plain HTTP only, to keep the schema owned by S3-C.
- No changes to the durable admission or dispatch paths.

### The explicit metrics decision (required deliverable, not an implementation detail)

There is no serve-wide registry. The lane **must record a dated decision in `.loom/40-decisions.md`** choosing one option and stating the rejected alternatives:

| Option | For | Against |
|---|---|---|
| **A. New `internal/observability` package owning one `prometheus.Registry`, served on a second listener at `FI_FHIR_METRICS_PORT`** | Matches every checked-in artifact (item 4): the `metrics` containerPort, both Services, the scrape job, the compose port, `ObservabilityConfig`. Keeps the metrics port off the PHI-bearing public listener. `prometheus/client_golang v1.23.2` is already a direct dependency (`go.mod:15`). | A second `http.Server` in the shutdown path; `internal/workflow`'s registry stays separate and its metrics remain unemitted. |
| **B. Mount `/metrics` on the existing GraphQL mux** | One listener, one shutdown. | Contradicts every deployment artifact; puts an unauthenticated scrape path on the same listener as raw clinical POSTs; `validateServerConfig`'s reserved-path map (`internal/api/graphql/server.go:307-310`) would need a third entry. |
| **C. Reuse `internal/workflow.PrometheusMetrics` as the serve registry** | Zero new code; the Grafana dashboard and 32 alert rules already target its metric names. | Its interface is events/actions/DLQ-shaped for the **legacy** engine (`internal/workflow/metrics.go:12-36`), which the durable path never executes. Adopting it would mean emitting `workflow_*` names from integration-engine code — a naming lie of the same class this slice exists to remove. |
| **D. Delete the façade instead** | Cheapest; makes docs true immediately. | The product spec requires `/metrics` (`20-product-spec…:225`); deleting only defers 4.3. |

Recommendation: **A**, with `internal/workflow`'s Prometheus adapter left untouched and explicitly documented as legacy-engine-only, and the 32 alert rules + Grafana dashboard **rewritten or deleted** in this lane (they currently target metrics option A will not emit).

### Tasks

1. **Decision + doc.** Write the metrics decision above into `.loom/40-decisions.md`. Correct `docs/operations/README.md:55-70` to state exactly what is served.
2. **Wire the existing health service.** Construct `workflow.NewHealthService` (or lift it to `internal/observability` if the `internal/workflow` dependency is unwanted) in `runServe`. Register readiness checks that actually touch dependencies:
   - `submission_db` — `db.PingContext` on `securePreviewRuntime.submissionDB`.
   - `session_store` / `profile_store` / `workflow_lifecycle_store` / `event_store` / `mapping_store` — present-and-pingable, or explicitly `not configured` (a truthful "absent" is not "healthy").
   - `mllp_listener`, `delivery_worker`, `batch_runner`, `autoroute_sweeper`, `autoroute_notifier` — running / not configured / stopped, sourced from the same component table at `cmd/fi-fhir/main.go:5129-5136`.
   Liveness stays process-only.
3. **Fix the GraphQL `health` resolver** (`schema.resolvers.go:2118-2145`) to project the same component set rather than hardcoding `"healthy"`. This is a resolver body change, not a schema change — verify with `make lint-gqlgen` that `generated.go` is unchanged.
4. **Add `Observe` seams to the four blind components** — MLLP service, delivery `Dispatcher` (it already computes a typed `Outcome`), batch `Runner`, session `Hub` — following the `SweeperConfig.Observe` shape at `sweeper.go:44` exactly: a typed result struct, an optional non-blocking hook, no metrics dependency inside the component package. Then bind them to the registry in `runServe`.
5. **Call `PostgresCatalog.ReportHealth`** (`lifecycle/queries.go:164`) from the MLLP/batch runtime health path so `integration_lifecycle_snapshots.health` stops being permanently stale.
6. **Durable session fanout.** Replace the process-local `Hub` broadcast with a durable path. Recommended: a `integration_session_stream_events` table written in the same transaction as the run's durable state, plus PostgreSQL `LISTEN`/`NOTIFY` carrying **only** `(tenant_id, session_id, run_id, seq)` — never a payload — with each replica re-reading rows after `seq` and feeding the existing per-subscriber channel. Keep `Hub` as the in-process delivery layer so `subscribe`/`toGraphQLEvent` (`integration_session_service.go:498-528`) is unchanged. On subscribe, replay from the client's last-seen `seq` so a replica flip is a gap, not a loss.
7. **Fix the batch worker-identity defect** (correction 10): derive `FI_FHIR_BATCH_WORKER_ID` from hostname+pid when unset, exactly as `delivery_runtime.go:40-47` already does; keep the env var as an override; correct `.env.example:90` and `docs/operations/BATCH-INGESTION.md:38` to state the uniqueness requirement.
8. **Lease the autoroute notifier.** Move de-duplication from the per-process `seen` map (`notify.go:292-293`) to a durable marker (a `notified_at` column or a small `terminology.autoroute_notifications` table). Alternatively take a `pg_advisory_lock` for the scan window — but state which, and why, in the decision log.
9. **State the MLLP capacity semantics.** Either (a) document that `CapacityPolicy` is per-replica and require the operator to divide, or (b) make it per-deployment via a durable token bucket. (a) is the honest cheap option for 4.3; (b) belongs in 4.4. Whichever is chosen, `pkg/integration/deployment.go:61-66` gets a doc comment and `docs/operations/PRODUCTION-MLLP.md` gets the rule.
10. **Deployment truth.** Replace the `exec: ["/fi-fhir","version"]` probes (`deploy/kubernetes/base/deployment.yaml:136-150`) with `httpGet /health` (liveness) and `httpGet /ready` (readiness); align the Helm chart (`deploy/helm/…/deployment.yaml:162-178`) so both paths agree; keep or delete the 9090 Services per the metrics decision.
11. **Clean up the dead e2e tests** (item 6): delete `TestHealthEndpoints` / `TestMetricsEndpoint` or repair their assertions and bring `./test/e2e/...` under a real CI job. Do not leave assertions that cannot run.
12. **Rewrite or delete the 32 alert rules and the Grafana dashboard** so they target metrics that are actually emitted.

### Acceptance Criteria

- `/ready` returns `503` when the submission database is unreachable and `200` when it is; `/health` stays `200` in both cases.
- `/metrics` (per the decision) serves a Prometheus exposition containing at least one counter that increments on a real durable submission, and the scrape config, Services, containerPort, and annotations all point at what is actually served — or all are removed.
- `grep -rn '\.Observability' internal/ cmd/` returns at least one production consumer, or `ObservabilityConfig` fields with no consumer are deleted from `pkg/config`.
- Two `serve` processes against one PostgreSQL: an SSE subscription opened on process A receives the ordered `run_started → stage_* → run_completed` sequence for a run executed on process B.
- Two batch runners started from the **documented** configuration (identical `FI_FHIR_BATCH_WORKER_ID` value copied from `.env.example`) do not both claim the same object.
- Two notifier instances against one webhook receiver deliver each pending row's digest **once**.
- No PHI in any metric label, any `NOTIFY` payload, or any log line added by this lane. Metric label values are drawn from a bounded set (component name, outcome, destination class) — never correlation ID, receipt ID, attempt ID, tenant-supplied string, or URL.
- `docs/operations/README.md`, `.env.example`, `deploy/**`, and `fi-fhir config env` output agree with the process.

### Kill-Test (negative-controlled)

**Primary: `TestServeObservability_TwoReplicasUnderDocumentedConfiguration`** — one PostgreSQL 16, two `fi-fhir serve` processes started from the **same** environment block that `.env.example` and `docs/operations/*` prescribe (not test-crafted distinct identifiers), asserting in one test:

1. `/ready` on both is `200`; make PostgreSQL unreachable; both go `503` within the probe budget; `/health` stays `200`; restore reachability; both return to `200`.

   **Corrected by S3-A, 2026-08-08.** As written this step said "stop PostgreSQL via the remote Docker context". That is not runnable inside the required CI job: `test:observability-replicas` gets PostgreSQL as a GitLab **service container**, and a job has no Docker socket with which to stop it, so the assertion would be forced to `t.Skip` in the one place it must be blocking — the same can-not-fail shape corrections 6 and 28 exist to remove. The proof instead interposes an in-test TCP proxy between the `serve` replicas and PostgreSQL (`FI_FHIR_DATABASE_HOST`/`FI_FHIR_DATABASE_PORT` point at the proxy) and closes the proxy's listener and live connections. That is strictly stronger: it is identical locally and in CI, it needs no Docker context, and it exercises pool-level reconnect rather than container restart timing.
2. SSE subscribe on A, run a sample on B, receive the full ordered event sequence on A within 2s.
3. Two batch runners with the documented identical worker ID: exactly one archives and deletes the object; `integration_batch_audit` shows no interleaved `claimed` pair inside one live lease.
4. Two notifiers against one recording HTTP receiver: exactly one digest per pending row.
5. `/metrics` on both replicas: a submission on A increments A's counter and not B's; no label value matches any of a PHI/identifier sentinel list (MRN, correlation ID, receipt ID, patient name from the fixture).

**Negative control (the part that proves the test can fail):** the same test file runs each of the five assertions against a `pre` mode that restores current behavior — `-ldflags` build tag, or an env switch such as `FI_FHIR_OBSERVABILITY_MODE=legacy` — and the CI job asserts that mode **fails** assertions 1, 2, 3, and 4. Concretely, before the change: assertion 1 gets `200 {"status":"healthy"}` with PostgreSQL stopped; assertion 2 receives zero events on A; assertions 3 and 4 both double-fire. A pipeline where the `pre` mode passes is a broken test, not a green lane.

**Existence guard:** the CI job's first step is `go test -tags=integration -list '^TestServeObservability_TwoReplicasUnderDocumentedConfiguration$' ./internal/observability | rg -x '…' | awk 'END { if (NR != 1) exit 1 }'`.

### Verification

```bash
# Unit + race
go test -race ./internal/observability/... ./internal/integration/session/... \
  ./internal/integration/batch/... ./internal/terminology/autoroute/... ./cmd/fi-fhir/...

# Codegen must be untouched by the resolver-body fix
make lint-gqlgen

# Runtime config truth
make check-runtime-config
bash scripts/smoke-test_test.sh

# Manifests
make lint-helm 2>/dev/null || helm lint deploy/helm/fi-fhir
bash scripts/validate-kustomize-preview.sh

# Integration proof (remote Docker context per AGENTS.md:237-258; no local Docker Desktop)
docker --context 7900xtx run --rm -d --name pg -e POSTGRES_USER=testuser \
  -e POSTGRES_PASSWORD=testpass -e POSTGRES_DB=fi_fhir_obs_test -p 15503:5432 postgres:16-alpine
export POSTGRES_TEST_URL="postgres://testuser:testpass@<docker-host>:15503/fi_fhir_obs_test?sslmode=disable"
make observability-replicas          # new Makefile target, mirrors mllp-runtime:98-101

# Live smoke against a real serve
BASE_URL=http://localhost:8080 bash scripts/smoke-test.sh
```

### Riskiest Assumption

> **"In-process fanout is the only thing that breaks with two replicas."**

That is the plan text's framing (`30-implementation-plan…:456-458`) and it is wrong: correction 10 finds four *additional* multi-replica defects, one of which (the shared batch worker ID) is actively prescribed by our own `.env.example` and operations doc, and is invisible to the existing CI proof because that proof uses distinct worker IDs.

The kill-test proves it because it refuses to construct a favorable configuration. It boots two replicas from **the documented environment block verbatim** and asserts on every background component at once. Any component whose safety depends on operator-supplied uniqueness — or on being the only process — fails there and only there. The negative-control `pre` mode makes the failure legible: four of five assertions must fail before the lane's changes land, or the test is not testing anything.

---

## Lane S3-B — Slice 4.1c: Destination-Scoped Identity for Durable Consumers

**Branch**: `feat/phase4-slice-4-1c-destination-identity`

### Re-scoping note (read before anything else)

The sprint scope says: "Delivery today (Slice 2.3): webhook/FHIR/database/file + Kafka commands. Verify how destination credentials/secrets are resolved and what identity the engine presents to destinations."

Verified answer: **the engine presents no identity, because it never contacts a destination.** The durable worker publishes one Kafka command per attempt to one constant topic (corrections 13, 15). webhook/FHIR/database/file are plan-level action *classes* (`workflow_plan.go:147-154`), never executed on the durable path; the only real webhook client is legacy, credential-inline, and reachable only under `graphql:operator` (correction 14). There is no destination artifact to scope a credential to (correction 15) and no `SecretReference` resolver anywhere (correction 17).

Therefore 4.1c as written is **two slices, not one**, and the first is a contract slice:

- **4.1c-a — destination revision artifact + `integration.deliver` decision.** Ship the missing contract and the authorization decision. Do not change the dispatch transport.
- **4.1c-b — the first durable HTTPS consumer.** Introduce an actual HTTPS destination transport that presents the scoped identity. This is genuinely new runtime, comparable in size to Slice 2.2, and should not be attempted in the same sprint as 4.1c-a.

**Sprint 3 ships 4.1c-a.** The spec below is for 4.1c-a; 4.1c-b's shape is sketched at the end so the contract is designed for it.

### Goal

Give a destination an immutable, resolvable, digest-verified revision that binds a scoped identity and a secret **reference** (never a value); add one `integration.deliver` authorization decision over `(tenant, integration revision, destination revision)` evaluated on the dispatch path; keep unmapped and unauthorized destinations fail-closed; and record server-owned provenance on each delivery outcome.

### Non-Goals

- No new destination transport. Kafka remains the only publisher in this slice.
- No changes to `internal/workflow`'s legacy `webhookAction`. It is out of the durable path; containing it is a separate decision.
- No touching `internal/integration/delivery/store.go` — 4.2a owns it (`MarkFailed`, `recover`, `Discard`).
- No new GraphQL schema. Destination revisions are server-owned, loaded like the static integration registry / lifecycle release, not authored over GraphQL in this slice.
- No secret **provider** backends beyond file/env in this slice; the resolver interface is the deliverable, one implementation is enough.

### Tasks

1. **Define `DestinationRevision`** in a new `internal/integration/destination` package, modeled on `internal/integration/mllp/source.go` and `internal/integration/batch/source.go`: `SchemaVersion`, `ArtifactID`, `RevisionID`, `SourceID`-equivalent (`DestinationID`), `Class`, transport kind, transport policy, `SecretBindingNames()`, a semantic digest, `Validate()`, and `ValidateAgainst(binding lifecycle.RunnableBinding)` that requires every named binding to be present in `binding.SecretBindings` — the same `hasSecretBinding` discipline as `mllp/source.go:257` and `batch/source.go:167`.
2. **Resolve destination bytes.** Extend the runnable binding path (`internal/integration/lifecycle/queries.go:123-162` `ResolveRunnable`) or add a destination-side loader so a planned delivery can resolve the exact destination revision and verify its digest against `DestinationRevisionRef.Digest` carried on the attempt (`integration_delivery_attempts.destination_revision_json`). This closes the gap in correction 15 without changing the plan-time contract.
3. **Build the secret resolver.** Add `integration.SecretResolver` with `Resolve(ctx, SecretReference) ([]byte, error)`, a file/env-backed implementation, bounded reads, and a hard rule that resolved material never enters a struct that is JSON-marshaled into a record, a log line, a metric label, or a Kafka value. Wire it in `cmd/fi-fhir/` beside the existing `loadSingleLineSecret` helpers, not inside `internal/integration/*`.
4. **Widen the authorization policy.** In `internal/integration/authorization/policy.go`, add `ActionDeliver Action = "integration.deliver"` and `ObjectDestinationRevision ObjectKind = "destination_revision"`. Restructure `Authorize` (`:58-75`) so the submit path is byte-for-byte behavior-preserving — the current `principal.SourceID` requirement must **not** leak onto the deliver path, and the deliver path must not weaken submit. Add `AuthorizeDelivery(security, tenantID, integrationRevision, destinationRevision)`. Add grant `integration.destination.client` (dotted, per correction 19), and record the naming decision.
5. **Enforce on the dispatch path.** Evaluate `AuthorizeDelivery` in `Dispatcher.RunOnce` (`dispatcher.go:50-86`) after `Claim` and **before** `messageForWorkItem` / `Publish`. A denial is a non-retryable failure routed through the existing `MarkFailed` with a distinct `Failure.Code` (e.g. `DELIVERY_FORBIDDEN`, `Retryable: false`) so it lands in the DLQ and is visible to 4.2a's control plane rather than spinning.
6. **Compatibility mode.** Mirror 4.1b1's approach exactly (`policy.go:23-28`, `submitGrants`): a destination revision without an identity binding, under an explicit `FI_FHIR_DELIVERY_IDENTITY_MODE=compatibility`, is authorized by a server-issued compatibility grant. `strict` rejects unmapped destinations closed. `compatibility` and `strict` are mutually exclusive settings that reject each other's configuration, exactly as OIDC/static mode do (`30-implementation-plan…:406-408`).
7. **Trusted provenance on delivery results.** Following 4.1b3's idiom (correction 26): record on the attempt or a provenance column which identity the deliver decision authorized, which destination revision digest was verified, and the server-owned decision timestamp. Anything derived from the destination side gets an `_advisory` suffix and a `COMMENT ON COLUMN` stating it is never a trust input. Use `NOT VALID` for any new CHECK so pre-slice rows are visibly distinguishable rather than retroactively vouched for.
8. **Regression-guard the already-shipped fail-closed behavior** (correction 16): an explicit test that `planWorkflow` rejects an action naming a destination absent from `revision.Destinations` (`workflow_plan.go:108-111`). Label it as a guard, not a new feature.

### Acceptance Criteria

- A destination revision is content-addressed, digest-verified at resolution, and rejected on any semantic mutation — the same properties Slice 1.1a proved for profiles/workflows.
- A revision with two destinations bound to two distinct identities dispatches each under its own identity; a work item for destination Y cannot be dispatched under X's identity.
- A destination present in a planned attempt but absent from the deployed revision's destination set fails closed **before** `Publish`, with a non-retryable DLQ entry.
- `strict` mode rejects an unbound destination; `compatibility` mode authorizes it under an explicit server-issued grant and logs which mode is active at startup.
- Secret **values** appear in no record, log, metric, GraphQL response, or Kafka value. A sentinel value planted in the secret file appears nowhere in a full dump of the five durable record classes or the produced Kafka message.
- Every existing submit-path authorization test passes unchanged (`ingress`, `processor`, `lifecycle/admission`, `mllp`, `batch`).
- `internal/integration/delivery/store.go` is untouched by this lane's diff.

### Kill-Test (negative-controlled)

**Primary: `TestDeliveryIdentity_PostgresKafkaScopedDispatch`** — PostgreSQL 16 + Kafka, one tenant, one integration revision with three destinations:

- `dest-alpha` → identity A, `dest-beta` → identity B, `dest-orphan` → planned by the workflow but removed from the deployed revision.
- Plant a sentinel string in identity A's secret file.

Assertions:
1. Attempts for `dest-alpha` and `dest-beta` both publish; the recorded provenance names A for alpha and B for beta; neither names the other.
2. A hand-crafted attempt row for `dest-beta` carrying `dest-alpha`'s destination digest fails the digest check and is dead-lettered, not published.
3. `dest-orphan` produces a `DELIVERY_FORBIDDEN` DLQ entry with `attempt_count` unchanged and **zero** Kafka records for its attempt ID.
4. The sentinel appears in none of: `integration_receipts`, `integration_canonical_events`, `integration_message_lineage`, `integration_delivery_attempts`, `integration_delivery_outbox`, `integration_delivery_audit`, the produced Kafka key/value/headers, or the process's stdout/stderr.
5. `compatibility` mode authorizes `dest-orphan`'s class of unbound destination; `strict` mode with a `compatibility`-only setting present **fails startup**.

**Negative control — and this one runs first, before any code changes:** `TestDeliveryDispatch_ContactsNoDestination` stands up a TLS listener at the URL a `webhook` destination would use, runs a full production submission through the durable path against current `main`, and asserts **zero inbound connections** to that listener while asserting exactly one Kafka produce. That test must **pass on `main`** — proving correction 13 — and it becomes the boundary marker for 4.1c-b. Then, for the primary kill-test: stub `AuthorizeDelivery` to unconditionally return `nil` and confirm assertions 2, 3, and 5 fail. A pipeline where the stubbed build passes means the decision is not on the dispatch path.

**Existence guard:** `-list '^(TestDeliveryIdentity_PostgresKafkaScopedDispatch|TestDeliveryDispatch_ContactsNoDestination)$' ./internal/integration/delivery | rg -x … | awk 'END { if (NR != 2) exit 1 }'`.

### Verification

```bash
go test -race ./internal/integration/authorization/... ./internal/integration/destination/... \
  ./internal/integration/delivery/... ./internal/integration/processor/... ./cmd/fi-fhir/...

# Submit path must be untouched
go test -race -run 'Authoriz|Submit' ./internal/integration/...

make check-runtime-config          # new FI_FHIR_DELIVERY_IDENTITY_* vars must be in .env.example
make security-gosec && make security-vulncheck

# Integration proof (Postgres + Kafka, mirrors delivery-reliability:105-109)
export POSTGRES_TEST_URL=... KAFKA_TEST_BROKERS=...
make delivery-identity
make delivery-reliability          # 2.3's proof must still pass
```

### Riskiest Assumption

> **"The engine authenticates to destinations today, so 4.1c is about scoping an existing credential."**

It does not (correction 13) and there is no destination artifact to scope one to (correction 15). If this assumption survives into implementation, the lane ships a credential-binding mechanism with no consumer — an elaborate no-op that reads as done.

`TestDeliveryDispatch_ContactsNoDestination` kills it **before the lane writes a line of production code**, because it must pass on unmodified `main`: a real TLS listener at the destination URL receives nothing while Kafka receives exactly one record. That single passing test converts the slice from "scope the existing thing" to "build the missing thing and prove the decision runs on the path that will later use it", and it justifies splitting 4.1c into a and b.

### Sketch of 4.1c-b (not this sprint)

Add an HTTPS destination transport that consumes `integration.delivery.v1` in-process (or replaces it for HTTPS-class destinations), presents the scoped identity resolved in 4.1c-a, honors the existing circuit/retry/DLQ state machine, and never accepts a destination-supplied header as a trust input. Sized comparably to Slice 2.2.

---

## Lane S3-C — Slice 4.1d: PHI and Audit Policy Enforcement

**Branch**: `feat/phase4-slice-4-1d-phi-audit`

### Re-scoping note

4.1d as written ("immutable audit storage, retention/TTL/encryption enforcement, export controls") is too big for one lane, and one third of it has no subject:

- **Immutable audit storage** is 60% shipped (correction 21) — six tables guarded, six unguarded. Closing the gap is a migration plus a proof. **Small.**
- **Export controls** are 80% shipped but **unattributed** (correction 24), directly contradicting the product spec. **Small, and the highest value per line in the slice.**
- **Retention/TTL/encryption enforcement**: production *rejects* every non-ephemeral retention mode (correction 22), so there is no retained production raw PHI to expire. Meanwhile the PHI that *is* retained indefinitely — canonical event payloads, session sample ciphertext, session export snapshots (correction 23) — has no policy field at all, no expiry column, and no purge job. This is **not** enforcement of an existing policy; it is designing a retention policy for a different data set and building a purge runtime. **Large.**

Split:

- **S3-C1 (this sprint):** audit immutability + export attribution + a truthful retention-posture statement.
- **S3-C2 (next sprint):** canonical-event and session-sample retention policy, TTL columns, and a durable purge component.

### S3-C1 Goal

Make every durable audit and provenance record immutable in the schema rather than by convention; make data export attributable to an actor with a reason; and replace the plan's "retention/TTL/encryption enforcement" claim with a documented, code-cited statement of what is and is not retained.

### Non-Goals

- No TTL columns and no purge component. That is S3-C2.
- No change to `ErrUnsupportedRawRetention` (`postgres_submission.go:171-172`). Encrypted production raw retention stays unimplemented and fail-closed; the lane documents that, it does not lift it.
- No second redaction policy. 4.2a's `internal/integration/operator/payload.go` is the policy for operator-facing payload rendering.
- No new roles at the transport gate. Correction 20 is filed as an issue against 4.2's follow-up, not fixed here.
- No touching `internal/integration/delivery/store.go` (4.2a) or the dispatch path (S3-B).

### Tasks

1. **Extend immutability to the durable-runtime audit tables.** New migration `internal/integration/processor/migrations/0004_audit_immutability.sql` (claim the number in the worklog first — 4.2a took `0003`). Reuse the existing function shape at `lifecycle/migrations/0001:87-92`. Guard `integration_delivery_audit`, `integration_delivery_operations`, `integration_message_lineage`, and `integration_canonical_events` against `UPDATE`/`DELETE`. Add a matching guard for `integration_batch_audit` in the batch package (`0003_*`, since 4.1b3 took `0002`).
   - `integration_receipts` and `integration_delivery_attempts` are **state** tables with legitimate updates; guard specific columns (principal, correlation, fingerprint, recorded_at) with a `BEFORE UPDATE` trigger that raises on change to those columns only. State this distinction in the migration comment; do not blanket-guard a table the runtime must mutate.
2. **Attribute exports.** Add `principal_json`, `reason`, and `include_raw_payload` to `integration_session_exports` (`internal/integration/session/migrations/0004_*.sql`), `NOT NULL` with a `CHECK (octet_length(reason) BETWEEN 1 AND 1024)` matching the delivery-operations convention (`0002_delivery_reliability.sql:107-108`). Thread the verified caller identity from `requestsecurity.SecurityContextFromContext` into `ExportBundle` (`internal/integration/session/postgres.go:852-904`). Add `reason: String!` to `ExportIntegrationBundleInput` (`schema.graphql:1137`) — the one schema change this sprint — and reject an empty reason. Regenerate through `make lint-gqlgen`.
3. **Gate raw export.** `includeRawPayload: true` (`integration_session_service.go:480-484`) currently needs only whatever role reached the mutation. Require an explicit distinct grant (`integration.phi.export`, dotted per correction 19) and record it on the export row. Default remains strip.
4. **Truthful retention posture.** Write a `docs/operations/PHI-RETENTION.md` that states, with file:line citations: production raw is ephemeral and non-ephemeral modes are rejected; canonical event payloads are PHI-classified and retained indefinitely; session samples are redacted by default and AES-256-GCM encrypted when retained, with no TTL; exports snapshot all of the above. Update `.loom/30-implementation-plan-integration-engine-ide-completion.md`'s 4.1 bullet so it stops claiming enforcement that does not exist, and open S3-C2 against the honest gap.

### Acceptance Criteria

- `UPDATE` and `DELETE` on each newly guarded table raise, from the application role, in a live PostgreSQL 16 test.
- `integration_receipts` and `integration_delivery_attempts` still accept their legitimate runtime state transitions — the full 2.3 and 1.2 proofs (`make delivery-reliability`, `test:durable-submission`) pass unchanged.
- Every export row carries a non-empty actor and reason; an export attempt without a reason is rejected before any row is written and before any bundle is assembled.
- `includeRawPayload: true` without the export grant is refused, and the refusal names the decision, not the inventory (matching 4.2a's catalog-safe presenter list, `graphql/server.go:574-590`).
- `docs/operations/PHI-RETENTION.md` exists and every claim in it has a file:line citation.
- `make lint-gqlgen` produces no diff after the schema change is committed with its regenerated artifacts.

### Kill-Test (negative-controlled)

**Primary: `TestPhiAudit_PostgresImmutableRecordsAndAttributedExport`** — PostgreSQL 16:

1. Run one full durable submission; then attempt, as the application role, `UPDATE integration_delivery_audit SET reason = ''`, `DELETE FROM integration_delivery_audit`, `UPDATE integration_message_lineage SET correlation_id = 'x'`, `DELETE FROM integration_canonical_events`, and `UPDATE integration_receipts SET principal_json = '{}'`. Every one must raise, and `SELECT count(*)` before and after must be identical.
2. Run the legitimate delivery state machine end to end (claim → retry → DLQ → replay) and assert it still succeeds — proving the column-scoped guard on state tables did not over-lock.
3. Export a session with a reason and assert the row records the exact verified principal, the reason, and `include_raw_payload=false`.
4. Export without a reason: rejected, and `SELECT count(*) FROM integration_session_exports` is unchanged.
5. Export with `includeRawPayload: true` and no `integration.phi.export` grant: rejected, no row, no bundle. With the grant: allowed, row records `include_raw_payload=true`.

**Negative control:** run steps 1, 3, 4, and 5 against the **pre-migration** schema and pre-change resolver in the same job (a second database provisioned from the prior migration set, exactly as `test:integration` provisions `fi_fhir_terms_test`). Step 1 must show the `UPDATE`/`DELETE` **succeeding** and row counts **changing**; steps 3-5 must show exports written with no principal and no reason. A pipeline where the pre-migration database also raises means the test is asserting against the wrong schema.

**Existence guard:** `-list '^TestPhiAudit_PostgresImmutableRecordsAndAttributedExport$' ./internal/integration/session | rg -x … | awk 'END { if (NR != 1) exit 1 }'`.

### Verification

```bash
go test -race ./internal/integration/session/... ./internal/integration/processor/... \
  ./internal/api/graphql/resolvers/... ./cmd/fi-fhir/...

make lint-gqlgen                   # must be clean after committing regenerated artifacts
cd ui && npm run codegen:check     # ui/src/lib/gen/graphql.ts is regenerated, not hand-edited

export POSTGRES_TEST_URL=...
make phi-audit                     # new target, mirrors integration-session:120-124

# Prior proofs must be unaffected by the immutability triggers
make integration-session
make delivery-reliability
```

### Riskiest Assumption

> **"Retention/TTL/encryption fields exist in the revision contract, so 4.1d is about enforcing them."**

The fields exist and are well designed (`pkg/integration/revision.go:108-157`), but the durable committer **rejects** every mode that would use them (`postgres_submission.go:171-172`). Enforcing a TTL on production raw PHI is enforcing a policy over an empty set. Meanwhile the PHI that is actually retained forever — canonical event payloads, session ciphertext, export snapshots — carries no policy field at all. A lane that "enforces retention" against the contract would pass its own tests, satisfy the plan text, and leave every byte of real retained PHI untouched.

Two assertions kill it, and both are cheap enough to run in the first hour of the lane:

1. Submit a production message whose revision declares `raw_retention.mode = "encrypted"` with a valid TTL, purpose, storage revision, encryption key, authorizer, and access-audit flag. Assert it is **rejected** with `ErrUnsupportedRawRetention` — proving there is no retained raw to expire.
2. Immediately after a successful ephemeral submission, `SELECT payload_json, classification FROM integration_canonical_events` and assert the row is present, classified PHI, and that `information_schema.columns` for that table contains **no** column matching `%ttl%|%expires%|%retention%` — proving the real retained PHI has no policy attached.

Together they convert 4.1d from "enforce the contract" into C1 (close the audit and attribution gaps that *are* actionable now) plus C2 (design retention for the data that actually exists), which is the honest split.

---

## Suggested Execution Order

**Pre-sprint (blocking):** 4.1b2 (!134), 4.1b3, and 4.2a merge to `main`. Sprint 3 branches from the resulting commit. If 4.2b is open, it holds sole ownership of `ui/src/**` and must rebase onto S3-C1's regenerated `ui/src/lib/gen/graphql.ts`.

**Wave 1 — all three in parallel, day 1.**
- **S3-A step 1** (the metrics decision) lands as a docs-only commit within the first day, because S3-B and S3-C both need to know whether a registry exists before deciding how to observe their own components.
- **S3-B's negative control** (`TestDeliveryDispatch_ContactsNoDestination`) lands as a standalone test-only MR against `main` within the first day. It is the cheapest possible proof of correction 13 and it decides whether 4.1c is one slice or two.
- **S3-C1's two riskiest-assumption assertions** run in the first hour, before any migration is written.

If any of the three above disconfirms this document, the affected lane re-scopes before writing production code, and corrects this file first.

**Wave 2 — implementation, parallel.**
1. **S3-A** lands first among the code changes because it restructures the `errCh` / `waitForBackgroundStops` component table at `cmd/fi-fhir/main.go:5086,5129-5164`, which B and C both append to. Order within A: real `/health` + `/ready` + probes (items 1-3, 10) → observe seams (4-5) → durable fanout (6) → worker identity + notifier lease + MLLP capacity (7-9) → façade cleanup (11-12).
2. **S3-C1** second, because it owns `schema.graphql` and the sooner it takes that lock the sooner 4.2b can rebase.
3. **S3-B (4.1c-a)** third in merge order, though it can be developed fully in parallel — its files (`authorization/policy.go`, a new `destination` package, `dispatcher.go`) collide with nothing else, and it only needs `cmd/fi-fhir/main.go` after A has restructured it.

**Wave 3 — next sprint.**
- **4.1c-b** — first durable HTTPS destination consumer presenting the scoped identity.
- **S3-C2** — retention policy for canonical events and session samples, TTL columns, durable purge component.
- **4.4** — tracing exporter, cardinality budgets under load, chaos/DR, rolling upgrade. Correction 4's `FI_FHIR_TRACING_*` inertness and correction 10's MLLP per-replica capacity both defer here.

---

## Sources

Observability:
- `internal/api/graphql/server.go:129,164-168,180-184,216-221,307-310,390,419,574-590`
- `internal/api/graphql/resolvers/schema.resolvers.go:2118-2145`
- `internal/api/graphql/operation_authorization.go:17-19,50-57,70-88,90-117`
- `internal/workflow/health.go:67-68,88,220-259,274-459`
- `internal/workflow/metrics.go:12-36,40,458,465`; `internal/workflow/metrics_prometheus.go:85`
- `internal/workflow/logging.go:15-31,120`; `internal/workflow/tracing.go:369,380`; `internal/workflow/tracing_otel.go:42-52`
- `internal/workflow/actions.go:501-618` (esp. `:542-547,600,610`); `internal/workflow/engine.go:126`
- `pkg/config/config.go:39,181-195,417,606-613,759-769` (origin/main)
- `cmd/fi-fhir/main.go:3197-3202,4728-4729,4841-4910,5086,5090-5120,5129-5164` (origin/main)
- `scripts/smoke-test.sh:8-10,59-72,84,98-104`
- `test/e2e/integration_test.go:8,324-347,351-385`
- `deploy/kubernetes/base/deployment.yaml:10,19-22,42-44,136-150`; `deploy/kubernetes/base/service.yaml`; `deploy/kubernetes/base/pdb.yaml`
- `deploy/helm/fi-fhir/templates/deployment.yaml:162-178`; `deploy/helm/fi-fhir/values.yaml:5,27-28,77-78,150`
- `deploy/docker/prometheus.yml:26-30`; `docker-compose.yaml:24,65`
- `dashboards/grafana/workflow-overview.json`; `dashboards/alerting/workflow-alerts.yaml`; `dashboards/alerting/workflow-alerts-k8s.yaml`
- `docs/operations/README.md:55-70`; `.env.example:173-179` (origin/main)
- `.loom/iteration-plan-gate-0b-runtime-verification.md:30,89,118`

Multi-replica:
- `internal/integration/session/hub.go:34-95`; `internal/integration/session/runner.go:237`
- `internal/api/graphql/resolvers/integration_session_service.go:20-41,498-528`; `internal/api/graphql/resolvers/resolver.go:167,245`
- `internal/integration/delivery/store.go:42-160,163-180,230-240,300-310`; `internal/integration/delivery/dispatcher.go:50-86,116-179`
- `cmd/fi-fhir/delivery_runtime.go:29-121`
- `internal/integration/batch/store.go:120-208,225-355`; `internal/integration/batch/service.go:85-135`; `cmd/fi-fhir/batch_runtime.go:36-147`
- `internal/integration/batch/batch_integration_test.go:190-210,244`
- `internal/integration/mllp/capacity.go:17-104`; `internal/integration/mllp/service.go:53,80,118`
- `internal/terminology/autoroute/sweeper.go:20-121`; `internal/terminology/autoroute/notify.go:285-300,395-440` (origin/main)
- `pkg/terminology/db/mappings.go:1027-1039`
- `internal/integration/lifecycle/postgres.go:61`; `internal/integration/lifecycle/queries.go:123-165`
- `pkg/integration/deployment.go:61-74`
- `.env.example:90` (origin/main); `docs/operations/BATCH-INGESTION.md:38`

Delivery identity:
- `pkg/integration/revision.go:60-98,108-157,165-237`; `pkg/integration/contracts.go:29-36,115-130,602-611`
- `internal/integration/authorization/policy.go:19-129`
- `internal/integration/processor/workflow_plan.go:20-131,147-154`; `internal/integration/processor/revisions.go:51-71`
- `internal/integration/processor/postgres_submission.go:32,162-189,203-207,321-385,467-473`
- `internal/integration/mllp/source.go:240-263`; `internal/integration/batch/source.go:160-208,246`
- `internal/workflow/published_yaml.go:581,690-713,801`; `internal/workflow/plan.go:19,135`
- Call sites: `internal/integration/ingress/service.go:133`, `internal/integration/processor/message_processor.go:142`, `internal/integration/lifecycle/admission.go:29`, `internal/integration/mllp/service.go:141`, `internal/integration/batch/service.go:278`
- `internal/integration/delivery/types.go:16-17,42-61,117`
- 4.1b3: `.worktrees/phase4-batch-workload-identity` @ `5a72e05f` — `internal/integration/batch/provenance.go:1-89`, `internal/integration/batch/migrations/0002_batch_provenance.sql:1-45`

PHI and audit:
- `internal/integration/lifecycle/migrations/0001_deployment_lifecycle.sql:87-105`
- `internal/integration/processor/migrations/0001_atomic_submission.sql:10-11,63`; `internal/integration/processor/migrations/0002_delivery_reliability.sql:89-137`
- `internal/integration/session/migrations/0001_session_workspace.sql:1-95`; `internal/integration/session/migrations/0003_publications.sql:26-47`
- `internal/integration/session/postgres.go:270-320,852-904`; `internal/integration/session/protector.go:12-70`; `internal/integration/session/types.go:177,287`
- `internal/api/graphql/resolvers/integration_session_service.go:471-496`; `internal/api/graphql/schema.graphql:1137,1180`
- `internal/integration/processor/adt_a01.go:89,98,209`

4.2a overlap (branch `origin/feat/phase4-operator-control-plane` @ `af80a950`):
- `internal/integration/operator/{types.go:16-19, service.go:44-108,235,299, payload.go:1-40, postgres.go, cursor.go}`
- `internal/integration/delivery/store.go` (+92: `MarkFailed`, `recover`, new `Discard`)
- `internal/integration/lifecycle/queries.go` (+33); `internal/api/graphql/server.go` (+15, presenter); `cmd/fi-fhir/main.go` (+42)
- `internal/integration/processor/migrations/0003_operator_control_plane.sql`
- `internal/api/graphql/schema.graphql` (+272), `generated.go`, `model/models_gen.go`, `ui/src/lib/gen/graphql.ts` (+349)
- Untouched by 4.2a: `internal/api/graphql/operation_authorization.go`, `internal/integration/delivery/dispatcher.go`, `internal/integration/delivery/kafka.go`, `internal/integration/authorization/policy.go`

CI and process:
- `.gitlab-ci.yml:259-280 (lint:gqlgen), 515-607 (test:integration), 613-658 (test:artifact-revisions), 791-823 (test:mllp-runtime), 829-880 (test:delivery-reliability), 934-966 (test:integration-session), 1219-1282 (test:smoke)` (origin/main)
- `Makefile:62-67,93-124,255-266,283-291,349`
- `AGENTS.md:214-268,413-427` (origin/main)

Plan and spec:
- `.loom/30-implementation-plan-integration-engine-ide-completion.md:326-343 (3.2), 260-279 (2.3), 390-443 (4.1), 445-458 (4.2/4.3), 459-464 (4.4), 496-509 (proof matrix)`
- `.loom/20-product-spec-integration-engine-ide-completion.md:210-244 (governance/reliability), 246-280 (budgets), 262 (destination matrix), 220-222 (export attribution)`
- `.loom/slice-handoff-phase-4-slice-4-1b1-oauth-http-submit-authorization.md:56-61 (Next Actions)`
- `.loom/24-parallel-execution-specs.md` (style model; Lane C1/C2/E "as shipped" sections)
