# 32 — Sprint 4 Execution Specs

**Status**: Ready for agent pickup (created 2026-08-08)
**Owner**: platform
**Base commit read**: `origin/main` @ `a9c8ea59` (merge `feat/phase4-slice-4-3-observability`, Lane S3-A). Every `file:line` below is an `origin/main` line number, read via `git show origin/main:<path>`. The local working tree is at `bfcffa2de` and is **missing S3-A**; do not size a diff against it.
**In-flight, unmerged at read time**: `origin/docs/worklog-one-file-per-entry` (!147), `origin/ci/lint-ui-heap-oom` + `origin/ci/ui-build-heap-headroom` (!144), `origin/docs/env-example-missing-vars` (!146). `.loom/worklog/` **does not exist** at `origin/main`; `.loom/50-worklog.md` is a single 1037-line file. Lanes must re-check on first rebase.
**Inputs**: `.loom/31-sprint3-execution-specs.md` (style model and Wave 3 list), `.loom/30-implementation-plan-integration-engine-ide-completion.md` (4.4, Phase 5, Phase 6, release gates), `.loom/20-product-spec-integration-engine-ide-completion.md` (budgets, journeys, matrix), the three Sprint 3 handoffs, `docs/operations/PHI-RETENTION.md`, `.loom/40-decisions.md` (2026-08-08 observability decision), `.loom/28-spec-fhir-ig-bulk-smart.md`.

## Goal

Turn Slice 4.1c-b, S3-C2, and Slice 4.4 into lanes that run in parallel without colliding — and correct, from code, the parts of the Sprint 4 scope description and the plan text that are wrong. Three of the corrections below invert a premise in the prompt that framed this sprint.

## Non-Goals

- Do not implement in this planning slice.
- Do not reopen 4.1a/4.1b/4.1c-a/4.1d-C1/4.3 contracts; extend them.
- Do not restructure S3-A's `runServe` background-component table. Lanes append.
- Do not start Phase 5.2 (SMART/Bulk) or 5.3 in any form.

---

## Current-State Corrections From Code

Adversarial toward the sprint scope description in the prompt, toward `.loom/30`, and toward `docs/operations/PHI-RETENTION.md`. Each item is what the code does.

### Destination delivery (Lane S4-A scope)

**1. "Invert `TestDeliveryDispatch_ContactsNoDestination`" would delete a proof that is still true.** That test is one of two under `make delivery-identity` (`Makefile:133-136`) and the blocking job `test:delivery-identity` guards both by name with `awk 'END { if (NR != 2) exit 1 }'` (`.gitlab-ci.yml:1054-1059`). Correction 13 of `.loom/31` — the durable worker publishes one Kafka command and contacts nothing — remains true for every `kafka`-transport destination, which is every destination in production today. Inverting the test in place removes the only regression guard for that. Correct move: **narrow** the existing test to `kafka`-class and add a **new** HTTPS-class test in a **new** job with its own guard. Arity of the existing guard stays 2.

**2. There is no Kafka consumer in production code.** `kgo.ConsumeTopics` / `PollFetches` appear only in `internal/integration/delivery/delivery_integration_test.go:473-486` and `internal/integration/delivery/destination_fixture_test.go:326-338`. `internal/integration/delivery/kafka.go:91` builds a producer client only. "In-process consumer of `integration.delivery.v1`" is therefore a new consumer group, new offset commits, new rebalance handling, and a **second** at-least-once boundary layered on the one the outbox already owns. That is not the smaller option; it is the larger one.

**3. Replacing the publish is strictly smaller, because the durable state machine is already destination-aware and transport-blind.** `Dispatcher.RunOnce` (`internal/integration/delivery/dispatcher.go:118-157`) is `Claim → decideIdentity → messageForWorkItem → publisher.Publish → MarkPublished`. `Publisher` is broker-neutral already (`internal/integration/delivery/types.go:62-65`). The circuit breaker is keyed on the **destination artifact**, not on Kafka: `Claim` joins `integration_delivery_circuits` on `a.destination_revision_json->>'artifact_id'` (`internal/integration/delivery/store.go:86-88`); `recordCircuitFailure` (`:620-660`) and `closeCircuit` (`:663+`, called from `MarkPublished:198` and `recover:474`) are transport-agnostic. Retry, backoff, DLQ, replay, resubmit, and discard all live in `store.go:216-330,346-500,510+`. **Nothing in the state machine knows what a broker is.** An HTTPS transport substituted at the `Publisher` seam inherits all of it and leaves `store.go` — still 4.2a's file — untouched.

**4. The dispatcher already resolves the destination revision, then throws it away.** `decideIdentity` (`dispatcher.go:166-182`) calls `DestinationDecider.Decide(ctx, tenantID, attemptID, ref)`; inside, `Authorizer.Decide` resolves the full `Revision` — `Transport`, `HTTPS.URL`, `HTTPS.Method`, `HTTPS.TokenBinding` — at `internal/integration/destination/identity.go:182`, and returns only `error`. 4.1c-b's smallest correct change is a **second primitives-only seam parallel to `DestinationDecider`**, not a widened `Decide`. The S3-B handoff records why: `delivery` declares the interfaces, `destination` satisfies them structurally, neither imports the other, and an earlier shape produced an import cycle the moment a test needed both (`.loom/slice-handoff-phase-4-slice-4-1c-a-destination-identity.md:85-90`).

**5. `SecretResolver` has no dispatch-time existence at all.** `verifyDestinationSecrets` (`cmd/fi-fhir/destination_identity_runtime.go:123-141`) resolves each declared binding once at startup, checks non-empty, and **zeroes the buffer**. `destinationIdentityRuntime` (`:20-24`) holds `authorizer`, `mode`, `registry` — and no resolver. So "resolve the scoped identity through the shipped `SecretResolver`" is not consumption of an existing wiring; it is a new field, a new object lifetime, and a new rule about where material may live, in a package (`cmd/`) deliberately chosen so no `internal/integration/*` type can hold it (`pkg/integration/secret.go:32-40`).

**6. File and env secrets cannot be version-pinned, so rotation is a file write with no cache to invalidate.** `destinationSecretResolver.Resolve` fails closed on any non-empty `SecretReference.Version` (`cmd/fi-fhir/destination_identity_runtime.go:189-191`); env resolution is `os.Getenv` (`:202-211`). A per-dispatch resolve re-reads the same path or variable every time. State that as the rotation contract; do not add a cache without a decision, and never hold material across dispatches.

**7. The destination registry is single-tenant, single-integration-revision, one file path.** `registryDocument` carries exactly one `tenant_id` and one `integration_revision` (`internal/integration/destination/registry.go:47-53`); `Resolve` refuses any other tenant (`:189-191`); one `FI_FHIR_DELIVERY_IDENTITY_REGISTRY_PATH` is read (`cmd/fi-fhir/destination_identity_runtime.go:42,98-114`). 4.1c-b inherits this. Do not spec a multi-tenant HTTPS fleet on top of it.

**8. The delivery worker cannot exist without Kafka configured, even for an HTTPS-only destination set.** `buildDeliveryDispatcher` hard-requires `FI_FHIR_QUEUE_DRIVER=kafka` and non-empty `FI_FHIR_QUEUE_BROKERS` (`cmd/fi-fhir/delivery_runtime.go:60-66`). An HTTPS-only deployment today must stand up a broker it never produces to. Either relax the requirement for a registry whose destinations are all `https`, or document the dependency — but do not leave it undecided.

**9. The "no destination address" assertion set is exactly five durable classes plus the broker record, and the advisory column is deliberately outside it.** `assertDurableRecordsCarryNoDestinationAddress` scans `integration_receipts.result_json`, `integration_canonical_events.payload_json`, `integration_message_lineage.artifact_revisions_json||routes_json`, `integration_delivery_attempts.destination_revision_json`, `integration_delivery_outbox.payload_json` (`internal/integration/delivery/destination_contact_integration_test.go:170-202`), forbidding the URL, `host:port`, `https://`, `http://`, `127.0.0.1` (`:204-213`). `integration_delivery_identity_decisions.destination_endpoint_advisory` holds the URL by design (`internal/integration/destination/migrations/0001_delivery_identity.sql:28,66-67`) and is not scanned. **Contacting a destination must not move an address into any of the five.** That separation is the lane's PHI/egress contract.

**10. "HTTPS destination" is a server-side transport, never a workflow-named thing.** The published DSL restricts destination names to `^[a-z][a-z0-9_.-]*$` and rejects a URL at planning before any durable row exists (proved at `destination_contact_integration_test.go:151-166` via `processor.ErrWorkflowPlanningFailed`). `Transport` is `kafka|https` on the destination revision (`internal/integration/destination/revision.go:54-62`), selected by the registry. Any spec sentence that says "an HTTPS destination in the workflow" is wrong.

### Retention and purge (Lane S4-B scope)

**11. S3-C1's triggers block *both* halves of a purge, and `PHI-RETENTION.md` does not say so.** `internal/integration/processor/migrations/0004_audit_immutability.sql:29-32` puts a blanket `BEFORE UPDATE OR DELETE` on `integration_canonical_events`. A purge is either a `DELETE` of the row or an `UPDATE` replacing `payload_json` with a tombstone. **Both raise.** `docs/operations/PHI-RETENTION.md:189-192` lists TTL and purge as "Not implemented — S3-C2" without noting that C1 removed the only two mechanisms. The C1 handoff flags it as a `DELETE` problem only (`.loom/slice-handoff-phase-4-slice-4-1d-c1-phi-audit.md:187-195`).

**12. Even with the trigger lifted, a canonical event row is undeletable, and two of its three referrers are themselves undeletable.** `integration_message_lineage` and `integration_delivery_attempts` FK to `integration_canonical_events (tenant_id, event_id)` `ON DELETE RESTRICT` (`internal/integration/processor/migrations/0001_atomic_submission.sql:52-54,73-75`); `integration_delivery_outbox` FKs to attempts `ON DELETE RESTRICT` (`:90-92`). `integration_delivery_attempts` and `integration_receipts` carry blanket `BEFORE DELETE` guards (`0004_audit_immutability.sql:57-65`); `integration_message_lineage` carries a blanket `BEFORE UPDATE OR DELETE` guard (`:34-37`). **Row deletion of a canonical event is structurally impossible today regardless of what a purge component does.** The only reachable shape is payload redaction in place, which needs a column-scoped exemption — C1's own idiom, applied in reverse.

**13. S3-C1 made every exported session permanently undeletable, while promising export TTL in S3-C2.** `integration_session_exports` got a blanket `BEFORE UPDATE OR DELETE` trigger (`internal/integration/session/migrations/0004_export_attribution.sql:55-58`) and FKs to `integration_sessions` with no `ON DELETE` clause, i.e. `NO ACTION` (`0001_session_workspace.sql:89-90`). Once a session is exported, neither the export row nor the session row can be removed. `docs/operations/PHI-RETENTION.md:191` nevertheless lists "TTL or expiry on session export snapshots — Not implemented — S3-C2".

**14. The one PHI-bearing table with no immutability trigger is the easiest purge target — start there.** No trigger names `integration_session_samples`. The session guards are in `0003_publications.sql:26-47` (publications, simulations), `0004_export_attribution.sql:55-58` (exports), `0005_session_stream_events.sql:55-57` (stream log). `raw_cipher` and the redacted `record_json` (`0001_session_workspace.sql:13-23`) are freely deletable today.

**15. S3-A added an unbounded, undeletable append-only table that nothing prunes.** `integration_session_stream_events` (`0005_session_stream_events.sql:22-30`) grows one row per session stream event forever, has a blanket `BEFORE UPDATE OR DELETE` trigger (`:55-57`), and deliberately has no FK to the session (`:43-47`). It carries no PHI, so it is not a privacy item — it is an unbounded-growth item that lands in the same component and needs the same exemption. Neither `PHI-RETENTION.md` nor the S3-A handoff mentions pruning it.

**16. The application role owns the tables it guards, so one of the C1 handoff's three purge options is empty.** Every migration executes on the same `*sql.DB` the runtime uses (`internal/integration/processor/postgres_submission.go:121-160`, `internal/integration/session/postgres.go:63-160`), so the application role is the table owner and can `ALTER TABLE … DISABLE TRIGGER` or `DROP TRIGGER`. "A privileged role that bypasses the trigger" (`.loom/slice-handoff-phase-4-slice-4-1d-c1-phi-audit.md:190-193`) buys nothing when the ordinary role already outranks the guard. This is not a defect in C1 — `PHI-RETENTION.md` claims only that mutations raise "from the application role" — but real role separation (purge role + de-privileged app role + migration runner) is its own slice, not a line in S4-B.

**17. The prompt's "revision contract vs deployment config" is a false dichotomy; there is a third answer, and it is the right one.** `RawRetentionPolicy` (`pkg/integration/revision.go:108-157`) governs production raw bytes, which are rejected unless `ephemeral` (`internal/integration/processor/postgres_submission.go:179-181`, error declared at `:38-39`). `integration_canonical_events` has no policy column (`0001_atomic_submission.sql:19-33`). A revision is **immutable and content-addressed**; the retained data outlives it, and a retention policy must be changeable without minting a new integration revision and redeploying. So the policy belongs in **neither** — it belongs in a new, mutable, audited per-tenant policy record, with the deployment supplying only a fail-closed default.

**18. The retention-posture gate is designed to break the moment S4-B adds a column.** `TestPhiRetentionPosture_ProductionRejectsRetainedRawAndCanonicalEventsCarryNoPolicy` (`Makefile:151-153`, CI `test:phi-audit` at `.gitlab-ci.yml:1397-1444`) queries `information_schema.columns` for `%ttl%|%expire%|%retention%|%purge%` on `integration_canonical_events` and fails if any appears (`docs/operations/PHI-RETENTION.md:70-72`). Rewriting that assertion and `PHI-RETENTION.md` sections 2, 3, and 6 is a **task in the same MR**, not a surprise.

**19. `PHI-RETENTION.md` has already drifted two citations, and S3-A caused it.** `docs/operations/PHI-RETENTION.md:83-84` cites `internal/integration/session/postgres.go:303-305` (redact) and `:306-317` (retain); at `origin/main` those lines are the tail of a list helper and the head of `AddSample`. The real redact path is `:321-323` and the retain path is `:324-333`. `:144` cites `postgres.go:889-893` for the unconditional retained-raw strip; at `origin/main` that is the attribution comment and `req.Validate()` — the strip is `:907-911`. S3-A's `0005` work added 18 lines to the same file after C1 wrote the doc. (Checked and still correct: `postgres_submission.go:179-181` and `:262-278`.) The lane that owns the doc repairs these.

**20. There is a matching background-component shape, and the multi-replica answer is a durable claim, not a lease.** `autoroute.SweeperConfig` is `{Store, Interval, Observe}` with `SweepOnce`/`Run(ctx)`/`Interval()` (`internal/terminology/autoroute/sweeper.go:34-72,75-121`), wired at `cmd/fi-fhir/main.go:5258-5264`. S3-A rejected `pg_advisory_lock` for the notifier because a lock serialises scanners without making the decision durable (`.loom/40-decisions.md:1345-1350`), and accepted the sweeper's duplicate scan as benign because the `UPDATE` is idempotent and guarded (`:1398-1401`). A purge is the same shape: the scan is idempotent, so it needs no lease; the **audit row** needs a durable claim so two replicas do not double-record one purge.

### Recovery, upgrade, performance (Lane S4-C scope)

**21. `test:benchmark` measures none of the product spec's numeric budgets, and measures the legacy engine.** `.gitlab-ci.yml:1228` and `Makefile:203` run `go test -bench=. ./internal/workflow/... ./pkg/terminology/... ./pkg/validate/...`. `internal/workflow` is the legacy engine the durable path never executes (`.loom/40-decisions.md:1383-1386`). **Zero** of the seven budgets at `.loom/20-product-spec-integration-engine-ide-completion.md:267-280` are measured by anything in this repository. The green benchmark gate is not partial credit toward 4.4.

**22. `fi-fhir workflow loadtest` is a false affordance.** `cmd/fi-fhir/main.go:1944,2861-2905` builds `workflow.LoadTestConfig` and calls `workflow.NewLoadTester(engine)`. `LoadTestResult.Passed(minAchievedRatio, maxErrorRate, maxP99Latency)` (`internal/workflow/loadtest.go:146`) is exactly the shape 4.4 wants over exactly the wrong subject. Reusing it produces a green report about code the golden journeys never run.

**23. One-version rollback is broken today, provably, by S3-C1's own migration.** `internal/integration/session/migrations/0004_export_attribution.sql:31-34` sets `principal_json`, `reason`, `include_raw_payload` `NOT NULL` with **no `DEFAULT`**. The current insert is the eight-column form (`internal/integration/session/postgres.go:949-954`); the pre-4.1d binary issues the five-column form. Roll the binary back one version against the migrated schema and every `exportIntegrationBundle` fails with a not-null violation. The budget is "one-version rolling upgrade and rollback preserve receipts, revisions, and resumable work without schema downgrade corruption" (`20-product-spec…:279-280`). This is a **found defect** and the cheapest possible day-1 gate.

**24. The repository does not define "one version".** There are **zero** git tags. `const version = "0.1.0"` is hardcoded and never bumped (`cmd/fi-fhir/main.go:57`), and S3-A's `fi_fhir_build_info` metric carries it, so every replica in a rolling upgrade reports the same version. Six independent forward-only migration ledgers exist — `integration_submission_schema_migrations`, `integration_session_schema_migrations`, `integration_lifecycle_schema_migrations`, `integration_batch_schema_migrations`, `integration_destination_schema_migrations`, and `terminology.schema_version` (`postgres_submission.go:134`, `session/postgres.go:76`, `lifecycle/postgres.go:65`, `batch/store.go:112`, `destination/postgres.go:57`, `pkg/terminology/db/schema.go:7,515-522`) — with no down path in any of them. "N-1" must be **defined** before it can be proved.

**25. Five of the six migrators take an advisory lock; the terminology migrator does not.** `pg_advisory_xact_lock` is taken before applying in `postgres_submission.go:130`, `session/postgres.go:72`, `lifecycle/postgres.go:61`, `batch/store.go:108`, `destination/postgres.go:53`. `pkg/terminology/db.Migrator.Initialize` (`pkg/terminology/db/migrations.go:67-108`) takes none: two replicas starting simultaneously against a fresh or v1 database both execute `Schema` / `SchemaV2Migration` / `SchemaV3Migration`. That is a rolling-upgrade defect in the one migrator 4.4 is meant to prove, and it is in the same file S3-A's autoroute `notified_at` claim depends on.

**26. `./test/e2e/...` still runs in no CI job, and S3-A said so in the source.** `test/e2e/integration_test.go:8` is `//go:build e2e && integration`; no `.gitlab-ci.yml` job passes `-tags=e2e`; `Makefile:66` needs `docker-compose`, and there is no local Docker Desktop (`AGENTS.md:237-242`). S3-A repaired `TestObservabilityEndpoints`'s assertions and documented the remaining gap honestly in the test's own comment (`test/e2e/integration_test.go:334-338`), but it still `t.Skipf`s at `:345` and `:406`. The plan's "replacement of the two dead e2e tests" (`.loom/30-implementation-plan…:784-785`) is half true: the assertions became real, the runnability did not. Phase 6's "all golden journeys pass in Compose and supported Kubernetes deployment" (`:824`) has no executing proof.

**27. Backup/restore is prose with no executable proof, and the documented method cannot meet the documented RPO.** `docs/operations/PRODUCTION-HARDENING.md:883-905` gives `pg_dump | gzip | aws s3 cp` plus a restore; `:924-926` carries an RTO/RPO table. The spec fixes RPO ≤ 5 min, RTO ≤ 30 min (`20-product-spec…:277-278`). A periodic `pg_dump` cannot bound data loss to 5 minutes; that needs WAL archiving / PITR, which **no** doc, chart, or manifest configures. `docs/operations/SUPPORTED-1.0.md:59` already lists this as blocking.

**28. The pinned reference profile exists and contradicts the shipped chart, and the doc says so.** `docs/operations/SUPPORTED-1.0.md:33-48` fixes Linux amd64, 4 vCPU / 8 GiB, two replicas, PostgreSQL 16 on SSD, destinations decoupled, 2-KiB HL7v2 — then states the Helm chart requests `100m`/`128Mi` and limits `500m`/`512Mi`, that these are "scheduling defaults, not proven capacity recommendations", and that "Phase 4.4 must tune and archive the exact values". A 4-vCPU budget cannot be met inside a 500m limit. The manifest and the measurement must move in one lane or the measurement is not of the shipped artifact.

**29. CI's runner pool cannot truthfully host a latency budget, and the repo already admits it.** `.gitlab-ci.yml:1235-1240`: "The k3s pool spans hardware differing by >5x, so bench-check picks the profile matching the `cpu:` line". A p95 ≤ 250 ms / p99 ≤ 500 ms gate on a pool that varies 5× is either permanently red or calibrated into meaninglessness. Measuring the product spec's budgets requires a **pinned runner or a dedicated remote host**, which is an infrastructure decision, not a code slice — and it is the reason the performance sub-slice cannot land in Sprint 4.

**30. The tracing façade S3-A honestly labelled is still standing in five deployment artifacts.** `pkg/config/config.go:609-611` parses and `:758-762` validates the tracing settings; nothing consumes them (`git grep '\.Observability' internal/ cmd/` returns `cmd/fi-fhir/main.go:4884` and a comment at `internal/observability/server.go:18`). `.env.example:218-223` now says "NOT IMPLEMENTED" — but `docker-compose.yaml:104-105` points it at Jaeger, and `deploy/kubernetes/base/deployment.yaml:121`, `deploy/helm/fi-fhir/templates/deployment.yaml:103-105`, `deploy/kubernetes/overlays/production/kustomization.yaml:58`, `configs/full-stack.env:74-75`, and `README.md:407` all set or document it. This is exactly the façade shape S3-A removed for metrics, one artifact class short of removal.

**31. Structured logging has no logger, so "correlation-safe logs" is a build item and a prerequisite, not a companion, to tracing.** `git grep '"log/slog"' internal/ pkg/ cmd/` returns nothing at `origin/main`. The spec requires structured logs correlating receipt, source message, canonical event, workflow run, and delivery attempts without exposing PHI (`20-product-spec…:228-229`). Correlation IDs are plumbed through every durable record already (`.loom/31` correction 9); the missing half is emission.

**32. MLLP per-deployment capacity is documented, and the doc will invalidate a naive throughput proof.** `docs/operations/PRODUCTION-MLLP.md:42-71` states the per-replica semantics, the `N × MaxMessagesPerSecond` consequence, the two operator choices, and that a durable token bucket is "future work (Slice 4.4+), not shipped today". A throughput run on the two-replica reference profile against a revision declaring 250 msg/s will admit up to 500 and will not be measuring the declared policy. Decide before measuring.

### Transport gate (Lane S4-E scope)

**33. Correction 20 of `.loom/31` is unchanged, and the surface is 131 root fields.** `operationAuthorization.MutateOperationContext` returns `nil` — allow everything — for any caller holding `graphql:operator` (`internal/api/graphql/operation_authorization.go:50-52`); `integration:preview` remains limited to `health` + `previewIntegrationMessage` (`:90-117`). The schema now has **64 `Query`, 60 `Mutation`, 7 `Subscription`** root fields across 19 `extend type` blocks. Narrowing is an enumeration over 131 fields plus a default-deny rule. It touches exactly one Go file plus tests and docs, and **does not touch `schema.graphql`, `generated.go`, `models_gen.go`, or `ui/src/lib/gen/graphql.ts`** — so it does not contend for a schema lock.

**34. The merged 4.2b UI does not depend on the blanket role in code, but every documented way of running it does.** `ui/src/**` contains zero occurrences of `graphql:operator`. It appears in `internal/api/graphql/operation_authorization.go:17`, `cmd/fi-fhir/main.go`, two test files, and five docs that hand it out: `docs/developer-guide/development-setup.md:276`, `docs/operations/INTEGRATION-SESSIONS.md:43-46,76`, `docs/operations/PRODUCTION-HARDENING.md:668`, `docs/planning/GRAPHQL-API.md:97`, `ui/docs/DEVELOPER-GUIDE.md:51`. Every operator token minted from those docs carries `graphql:operator` and nothing else, and would lose access to **all** 4.2a control-plane operations the instant the gate narrows. The role mapping, the test fixtures, and the doc updates must ship in one MR.

### Standards (Lane S4-D scope)

**35. `.loom/28` is stale in three ways.** It targets "USCDI v3-related US Core expectations" (`.loom/28-spec-fhir-ig-bulk-smart.md:23`) where the product spec pins **US Core 9.0.0** (`20-product-spec…:262`); it tracks `libs/fi-fhir#12` (`:5`); and its dependency list (`:36-38`) treats `pkg/fhir/mapper.go` as the engine's FHIR path. It is not (correction 36).

**36. The durable integration engine never produces a FHIR resource, so 5.1 would certify a path the golden journeys do not run.** `pkg/fhir` is imported by exactly two non-test files: `internal/workflow/actions.go` (the legacy engine; `fhir.NewUSCoreMapper()` at `:680`) and `cmd/fi-fhir/main.go:49,309` (the `fhir validate` CLI). The durable processor writes canonical events (`internal/integration/processor/postgres_submission.go:262-278`); the dispatcher writes Kafka commands. Golden journey 1 ends "inspect trace and **FHIR delivery**" (`20-product-spec…:293`) and journey 6 requires validating a US Core 9.0.0 R4 payload (`:302-304`) — neither has a producing path. **5.1's real prerequisite is 4.1c-b**, which is Lane S4-A of this sprint.

**37. Current FHIR "validation" is a hand-rolled required-field and profile-URL-presence check with no pinned IG package.** `pkg/fhir/validate.go:153-238`: `validateUSCoreProfilePresence` checks that `meta.profile` contains an expected URL from a hardcoded per-resource-type map; `USCoreBaseURL` (`pkg/fhir/types.go:13`) carries no version. There is no `StructureDefinition`, no terminology binding evaluation, no `.tgz` IG package, no snapshot generation. "Integrate an official validator" (`.loom/30-implementation-plan…:805`) means the HL7 `validator_cli.jar` — a **Java** runtime in CI and possibly in the shipped image. That is a supply-chain and CI-image decision before it is a code slice.

**38. 5.1 is disjoint from Phase 4's Go packages and not disjoint from its shared plumbing.** `pkg/fhir/*` and `pkg/validate/*` are touched by no Sprint 4 lane. But `cmd/fi-fhir/main.go` (all lanes), `.gitlab-ci.yml` (all lanes), `Makefile` (all lanes), `go.mod`, and `docs/operations/*` are shared — and `pkg/validate/...` sits in the `test:benchmark` package set (`.gitlab-ci.yml:1228`) that Lane S4-C is going to rewrite.

### Corrections found by a lane during execution

**41. The Lane S4-B negative control cannot come from one schema, and splitting it is stronger.** The kill-test section below asks for "a second, independently provisioned database at the **pre-migration** schema" where "the purge must fail to write a tombstone (the trigger raises) and step 4's mutations must **succeed**". Those two outcomes are not available from the same schema. On the pre-4.1e schema (processor `0001`-`0004`, session `0001`-`0005`) Slice 4.1d C1's blanket guards are active, so step 4's mutations raise there too — for a different reason than the one the primary proof asserts. The only schema on which they succeed is the pre-**C1** schema (processor `0001`-`0003`). Lane S4-B therefore runs **two** controls: the pre-4.1e schema proves the purge fails and that no shape of `UPDATE` can tombstone a payload, so the primary proof's tombstone is attributable to this migration; and the pre-C1 schema proves every mutation the primary proof requires to raise does succeed somewhere, so the refusals are attributable to a guard rather than to referential integrity or a malformed statement. Both are in `internal/integration/retention/purge_integration_test.go`.

### CI and process

**39. The isolated-proof pattern is unchanged and now has two more instances to copy.** `test:observability-replicas` (`.gitlab-ci.yml:1455-1490`) and `test:phi-audit` (`:1397-1444`) both follow it: dedicated PostgreSQL service, dedicated database name, `-list | rg -x | awk 'END { if (NR != N) exit 1 }'`, a `make` target, `allow_failure: false`. `test:integration` (`:515-607`) still runs only `./cmd/fi-fhir/...` and `./pkg/terminology/db/`, so nothing new under `internal/integration/...` runs there.

**40. Migration numbering is settled by the ledger at rebase, not by a worklog claim.** `.loom/31:160` and `.loom/slice-handoff-phase-4-slice-4-3-observability.md:65` both record the lesson from S3-C1 taking session `0004` ahead of S3-A. Next free at `origin/main`: processor `0005`, session `0006`, batch `0004`, destination `0002`, lifecycle `0002`, terminology `SchemaV4Migration` with `SchemaVersion = 4` (`pkg/terminology/db/schema.go:7`).

---

## Parallelization Map

| Lane | Slice | Sprint | Can start day 1? | Parallel with | Primary risk |
|---|---|---:|---|---|---|
| **S4-A** | 4.1c-b — first durable HTTPS destination consumer | 4 | Yes | B, C, E | Building a second at-least-once boundary instead of substituting a transport |
| **S4-B** | 4.1e (was S3-C2) — retention policy + purge runtime | 4 | Yes | A, C, E | Designing TTLs for data the schema forbids purging |
| **S4-C** | **4.4a only** — migration compatibility, rollback safety, restore round-trip | 4 | Yes | A, B, E | Treating 4.4 as one slice and shipping none of it |
| **S4-E** | Transport-gate narrowing (`.loom/31` correction 20) | 4, small | Yes | A, B, C | Breaking every documented operator token and the merged 4.2b UI |
| **S4-D** | 5.1 FHIR R4 / US Core conformance | **Hold code; docs-only in 4** | Docs yes | all | Certifying a code path the golden journeys never execute |

Deferred to Sprint 5 with reasons: **4.4b** performance budget harness (correction 29 — needs a pinned runner decision first), **4.4c** chaos / rolling upgrade / DR on Kubernetes 1.36 (needs 4.4a's N-1 definition and a cluster), **4.4d** tracing exporter + `log/slog` structured logging (corrections 30-31 — two build items, sized as their own slice), **4.4e** durable per-deployment MLLP token bucket (correction 32).

### Exact shared-file risks

| File | Lanes | Rule |
|---|---|---|
| `cmd/fi-fhir/main.go` (`runServe`, ~`:4880-5400`) | **A, B** | S3-A owns the component table's shape (`:5181-5229`, `:5281-5309`) and said so in the code (`:5186-5191`). A appends nothing new (its change is inside `loadDeliveryDispatcherFromEnv`). **B appends one component after the autoroute block at `:5272`**, bumps `errCh` capacity at `:5192` from 9 to 10, adds one entry to the `markComponent` not-configured list at `:5202-5208`, one to `waiting` at `:5282-5290`, and one to `componentMetricNames` at `:5293-5301`. No lane restructures. |
| `internal/observability/metrics.go` component constants (`:72-90`) and `Outcome` set (`:24-63`) | **A, B** | Both append. A adds `OutcomeDelivered`-class values only if the existing set is insufficient; B adds `ComponentRetentionPurge`. Append at the end of the block; do not re-sort. The PHI-label allowlist test (`:53-63`) must be extended in the same commit. |
| `internal/integration/delivery/dispatcher.go`, `identity.go` | **A only** | The new transport seam is a sibling of `DestinationDecider`, in `identity.go` or a new `transport.go`. |
| `internal/integration/delivery/store.go` | **nobody** | 4.2a owns it. If a lane believes it needs `store.go`, that is a re-scope, not an edit. |
| `internal/integration/destination/**` + `migrations/0002_*` | **A only** | |
| `internal/integration/session/postgres.go`, `internal/integration/processor/postgres_submission.go` | **B only** | Migration embeds and the purge store. |
| `internal/integration/processor/migrations/0005_*`, `internal/integration/session/migrations/0006_*` | **B only** | Re-verify the numbers against `origin/main`'s `migrations/` directories on **every** rebase; the ledger is the authority (correction 40). |
| `pkg/terminology/db/migrations.go`, `pkg/terminology/db/schema.go` | **C only** | The advisory lock (correction 25). |
| `internal/api/graphql/operation_authorization.go` | **E only** | |
| `internal/api/graphql/schema.graphql` + `generated.go` + `model/models_gen.go` + `ui/src/lib/gen/graphql.ts` | **nobody** | **The schema is frozen for Sprint 4.** No lane above needs a root field. If that turns out wrong, the lock goes to **S4-B** (a purge-status query is the only plausible need) and every other lane waits. S3-A's deferred client-resumable session stream (`.loom/slice-handoff-phase-4-slice-4-3-observability.md:92-93`) is **not** in Sprint 4 precisely because it would spend the lock. |
| `test/e2e/**`, `deploy/**`, `docker-compose.yaml`, `configs/full-stack.env` | **C only** | Probes, limits, tracing artifacts, the e2e CI job. |
| `.gitlab-ci.yml` | **A, B, C, E** | Append at the end of the `test` stage with distinct names: `test:destination-transport` (A), `test:phi-retention-purge` (B), `test:migration-compatibility` (C), `test:transport-gate` (E). Do not modify another job's `services:` or `-list` arity. |
| `Makefile` | **A, B, C, E** | Distinct targets, appended to the `.PHONY` list at `:1-11` and after `observability-replicas` at `:159-162`. |
| `.env.example`, `docs/operations/*` | all | Distinct sections. **B owns `docs/operations/PHI-RETENTION.md` including the two drifted citations (correction 19).** A owns `DESTINATION-IDENTITY.md`. C owns `SUPPORTED-1.0.md` and `PRODUCTION-HARDENING.md`. E owns the five `graphql:operator` docs (correction 34). |
| `.loom/50-worklog.md` **or** `.loom/worklog/` | all | `.loom/worklog/` does not exist at `a9c8ea59`. If !147 has merged when a lane branches, write a per-entry file; otherwise append to `50-worklog.md`. Check with `git ls-tree -d origin/main -- .loom/worklog` before the first commit. |

### Coordination rules

- One branch/worktree per lane under the repo's own `.worktrees/`: `feat/phase4-slice-4-1c-b-https-destination`, `feat/phase4-slice-4-1e-retention-purge`, `feat/phase4-slice-4-4a-migration-compatibility`, `feat/phase4-transport-gate-roles`.
- Before the first commit each lane records its owned files and, if it needs one, re-verifies its migration number against `origin/main` — not against this document.
- **A lane that discovers a false premise corrects the source planning doc before writing code.** Corrections 11, 13, 19, 23, and 36 already require doc edits; assign each to the owning lane rather than filing them.
- No lane promotes an existing job to blocking, and no lane changes another job's `-list` arity. Every new required job carries the `-list | rg -x | awk` existence guard.
- No local Docker Desktop. Local reproduction uses the remote context (`AGENTS.md:237-258`); CI uses service containers. A kill-test assertion that needs a Docker socket is not runnable in its own job — S3-A already paid for that lesson and replaced container-stop with an in-test TCP proxy (`.loom/slice-handoff-phase-4-slice-4-3-observability.md:66`).
- Any new background component calls `markComponent(<name>, ComponentRunning)` so `/ready` and `fi_fhir_component_up` cannot disagree with what the process started (`.loom/slice-handoff-phase-4-slice-4-3-observability.md:74-77`).

### Merge order

**C → A → B → E.** C first because its rollback fix (correction 23) sets the migration-authoring rule that B's two migrations must follow, and because its footprint (`pkg/terminology/db`, `test/e2e`, `deploy/**`) collides with nothing. A second because it is the largest new runtime and touches only its own packages. B third because it is the only lane appending to the serve component table, so it rebases onto a settled `cmd/fi-fhir/main.go`. E last because it changes test fixtures across `internal/api/graphql`, `internal/integration/ingress`, and `cmd/fi-fhir`, and rebasing that over three merged lanes is cheaper than the reverse.

---

## Lane S4-A — Slice 4.1c-b: First Durable HTTPS Destination Consumer

**Branch**: `feat/phase4-slice-4-1c-b-https-destination`

### The transport decision (required deliverable, not an implementation detail)

The prompt frames this as "in-process consumer of `integration.delivery.v1` vs replacing the publish for HTTPS-class destinations". The lane **must record a dated decision in `.loom/40-decisions.md`**, stating the rejected alternatives.

| Option | For | Against |
|---|---|---|
| **A. Transport substitution at the `Publisher` seam.** `Dispatcher.RunOnce` asks a router for the claimed item's destination; an `https`-transport destination is delivered over HTTPS and marked with the existing `MarkPublished`/`MarkFailed`; everything else publishes to Kafka unchanged. | Inherits lease, retry, backoff, DLQ, replay, resubmit, discard, and the **per-destination-artifact circuit** for free (correction 3). Exactly one at-least-once boundary. `store.go` untouched — still 4.2a's file. The decision already runs at the right point (`dispatcher.go:129`), immediately before the publish. Mirrors S3-B's structural-satisfaction seam exactly. | `MarkPublished`'s name becomes slightly off for an HTTPS delivery (it means "handed off successfully"); needs a doc comment, not a rename. Kafka-class and HTTPS-class destinations share one worker's poll loop and one lease duration. |
| **B. In-process consumer of `integration.delivery.v1`.** | Preserves the external-consumer contract for anyone already consuming the topic. Decouples HTTPS retry cadence from the outbox poll. | No consumer exists anywhere in production code (correction 2): new consumer group, offsets, rebalance, and a **second** at-least-once boundary on top of the outbox's. Cannot reuse the circuit, the DLQ, or `recover`, because those are keyed to an outbox row the consumer does not hold a lease on. Duplicate delivery becomes a product of two independent at-least-once layers. |
| **C. Both — publish *and* deliver.** | — | Rejected on sight: two systems contacting one destination for one attempt is a duplicate-delivery generator, and the spec's P0 definition includes "duplicate durable acceptance/event/outbox work for one idempotency key" (`20-product-spec…:284-286`). |

**Recommendation: A.** Record it, and record that the Kafka topic remains the transport for `kafka`-class destinations so `TestDeliveryDispatch_ContactsNoDestination` stays meaningful.

### Goal

Deliver an `https`-transport destination revision over real TLS, exactly once per claimed attempt, under the identity that 4.1c-a's `integration.deliver` decision authorized, with the credential resolved at dispatch time through `integration.SecretResolver`, honoring the existing circuit/retry/DLQ state machine, and never accepting a destination-supplied header, redirect, or served certificate as a trust input.

### Non-Goals

- No Kafka consumer. `kafka`-class destinations keep publishing to `integration.delivery.v1` unchanged.
- No touching `internal/integration/delivery/store.go` (4.2a) or `internal/api/graphql/schema.graphql` (frozen).
- No destination authoring over GraphQL. The registry stays a server-owned file (correction 7).
- No new secret providers. File and env remain the only implementations (`cmd/fi-fhir/destination_identity_runtime.go:192-199`).
- No FHIR-specific transport. `https` is a generic transport; a FHIR R4 destination class is Lane S4-D's prerequisite, not this lane's deliverable.
- No mTLS to destinations, no per-destination client certificate. `CABundleBinding` (`internal/integration/destination/revision.go:86`) governs trust of the *server*, nothing more.

### Tasks

1. **Decision + doc.** Write the transport decision above into `.loom/40-decisions.md`. Extend `docs/operations/DESTINATION-IDENTITY.md` "What the engine does and does not do" (`:7-30`) so it stops saying the engine contacts no destination — it now contacts `https`-class ones and only those.
2. **Second seam, parallel to `DestinationDecider`.** In `internal/integration/delivery`, declare a primitives-only `DestinationTransport` alongside the existing interfaces (`identity.go:43-64`): given `(ctx, tenantID, attemptID, ref, payload []byte)` it either reports "not mine, publish it" or performs the delivery and returns `nil`, a `Refusal`, or an ordinary error. The `destination` package satisfies it structurally. Neither package imports the other (correction 4).
3. **Resolve the credential at dispatch time.** Give the `destination` package a resolver field supplied from `cmd/`. Resolve `HTTPSPolicy.TokenBinding` per dispatch, use it, and zero it before returning. It must never enter `Decision`, a log line, a metric label, a `Failure.Detail`, or any struct that is JSON-marshaled. State the rotation contract (correction 6) in `DESTINATION-IDENTITY.md`.
4. **Build the HTTPS client with every trust input closed.** `CheckRedirect` returns an error — a redirect is a refusal, never a follow. `Transport.TLSClientConfig` pins `MinVersion: tls.VersionTLS12` and uses the `CABundleBinding` roots when declared, the system pool otherwise; `InsecureSkipVerify` never appears. Response headers are read for nothing except the status class. Response bodies are bounded and discarded; a destination cannot influence any durable record.
5. **Map HTTP outcomes onto the existing `Failure` contract.** 2xx → `MarkPublished`. 408/429/5xx, connection failures, TLS handshake failures, and timeouts → `MarkFailed{Retryable: true}` with a bounded catalog-safe code. Other 4xx and any redirect → `MarkFailed{Retryable: false}`. The code and detail obey the same bounds `refusalFailure` enforces (`internal/integration/delivery/identity.go:112-125`): ≤128-byte code, ≤512-byte detail, no destination-supplied content.
6. **Bound the call inside the lease.** `Config.validate` already requires `PublishTimeout < LeaseDuration` (`internal/integration/delivery/types.go:107-114`). The HTTPS call reuses `PublishTimeout` so a slow destination cannot outlive its lease and produce a concurrent second delivery after reclaim. Do not add a second timeout knob.
7. **Record HTTPS-specific provenance in the destination package's own migration** (`internal/integration/destination/migrations/0002_*.sql`, ledger `integration_destination_schema_migrations`). Server-owned: attempt ID, verified destination digest, HTTP status class, server-owned completed-at. Anything derived from the destination side — served certificate subject, response `Server` header — gets an `_advisory` suffix and a `COMMENT ON COLUMN` stating it is never a trust input, or is not recorded at all. Any new CHECK lands `NOT VALID` (the 4.1b3 idiom, `0001_delivery_identity.sql:38-58`).
8. **Decide the Kafka dependency for HTTPS-only deployments** (correction 8): either relax `FI_FHIR_QUEUE_DRIVER=kafka` (`cmd/fi-fhir/delivery_runtime.go:60-62`) when every destination in the loaded registry is `https`, or document the dependency in `.env.example` and `DESTINATION-IDENTITY.md`. Do not leave it undecided.
9. **Narrow the boundary marker, do not delete it.** `TestDeliveryDispatch_ContactsNoDestination` keeps its name, keeps its place in `make delivery-identity` (`Makefile:133-136`) and `test:delivery-identity`'s arity-2 guard (`.gitlab-ci.yml:1054-1059`), and gains one sentence in its doc comment: it now proves that a **`kafka`-class** destination contacts nothing.

### Acceptance Criteria

- An `https`-class destination is contacted **exactly once** per claimed attempt, over real TLS, with the `Authorization` value derived from `HTTPSPolicy.TokenBinding` and from nothing the work item asserts.
- A `kafka`-class destination in the same registry, in the same run, is still published to `integration.delivery.v1` and contacts nothing.
- A destination that returns 503 is retried under the existing backoff and opens the existing per-destination circuit after `CircuitFailureThreshold`; a destination that returns 403 is dead-lettered non-retryably; a destination that returns 302 is dead-lettered non-retryably and the redirect target is never dialed.
- The secret sentinel appears in **none** of `integration_receipts`, `integration_canonical_events`, `integration_message_lineage`, `integration_delivery_attempts`, `integration_delivery_outbox`, `integration_delivery_identity_decisions`, the new provenance table, any produced Kafka record, or the process's stdout/stderr.
- No destination address appears in any of the five durable classes correction 9 enumerates. The advisory columns are the only place a URL lives.
- `internal/integration/delivery/store.go` and `internal/api/graphql/schema.graphql` are untouched by this lane's diff.
- `make delivery-identity` and `make delivery-reliability` both still pass unchanged.

### Kill-Test (negative-controlled)

**Primary: `TestDeliveryTransport_HTTPSClassContactedExactlyOnceUnderScopedIdentity`** — PostgreSQL 16 + Kafka + two in-test TLS servers, one registry with four destinations: `dest-https-alpha` (identity A, token binding with a planted sentinel), `dest-https-beta` (identity B, second TLS server), `dest-https-flaky` (503 then 200), `dest-kafka-legacy` (`kafka` transport).

1. Alpha's server records exactly one request; its `Authorization` carries A's material and never B's; beta's server records exactly one and never A's.
2. A second `RunOnce` after each success returns `OutcomeIdle` and neither server's count moves.
3. `dest-https-flaky` is retried on 503, the circuit row's `consecutive_failures` increments, the retry succeeds, and `closeCircuit` resets it — asserted directly against `integration_delivery_circuits`.
4. A 403 destination produces a non-retryable DLQ entry with `attempt_count` unchanged; a 302 destination produces one, and the redirect target's listener records **zero** connections.
5. `dest-kafka-legacy` produces exactly one Kafka record on `integration.delivery.v1` and contacts no HTTP endpoint.
6. The sentinel appears in none of the seven durable classes, the broker record, or captured stdout/stderr; no destination address appears in the five classes of correction 9.

**Day-1 gate, standalone test-only MR, must pass on unmodified `main`: `TestDeliveryTransport_HTTPSDestinationPublishesToBrokerToday`.** Load a registry whose destination declares `transport: https` with a live TLS endpoint at its URL, run one production submission and one `RunOnce` against unmodified `main`, and assert: the TLS endpoint records **zero** accepted connections, Kafka records **exactly one** command, and `integration_delivery_identity_decisions` records `authorized` with `destination_endpoint_advisory` equal to the URL. That proves 4.1c-a shipped an `https` transport it does not execute, that the `Transport` field routes nothing today, and that the advisory column is the only place the address lives. It becomes the negative control afterwards.

**Negative control (the part that proves the kill-test can fail):** run the primary under a build in which the transport router unconditionally reports "not mine". Assertions 1, 2, 3, and 4 must **fail** — alpha and beta record zero requests, and the flaky destination never opens a circuit — while assertion 5 still passes. A pipeline where the control passes means the router is not on the dispatch path.

**Existence guard:** `go test -tags=integration -list '^(TestDeliveryTransport_HTTPSClassContactedExactlyOnceUnderScopedIdentity|TestDeliveryTransport_HTTPSDestinationPublishesToBrokerToday)$' ./internal/integration/delivery | rg -x '…' | awk 'END { if (NR != 2) exit 1 }'`, in a **new** job `test:destination-transport`. `test:delivery-identity`'s guard stays at 2.

### Verification

```bash
go test -race ./internal/integration/destination/... ./internal/integration/delivery/... \
  ./internal/integration/authorization/... ./cmd/fi-fhir/...

make check-runtime-config          # any new FI_FHIR_DELIVERY_* var must be in .env.example
make security-gosec && make security-vulncheck

# Integration proof (remote Docker context per AGENTS.md:237-258; no local Docker Desktop)
docker --context 7900xtx run --rm -d --name pg -e POSTGRES_USER=testuser \
  -e POSTGRES_PASSWORD=testpass -e POSTGRES_DB=fi_fhir_destination_transport_test \
  -p 15505:5432 postgres:16-alpine
export POSTGRES_TEST_URL="postgres://testuser:testpass@<docker-host>:15505/fi_fhir_destination_transport_test?sslmode=disable"
export KAFKA_TEST_BROKERS=...
make destination-transport         # new target
make delivery-identity             # 4.1c-a's two proofs must still pass
make delivery-reliability          # 2.3's proof must still pass
```

### Riskiest Assumption

> **"4.1c-a resolves the destination revision on the dispatch path, so 4.1c-b is wiring a transport onto an existing resolution."**

It resolves it and discards it (correction 4), and it holds no secret resolver at all after startup (correction 5). If this assumption survives, the lane widens `DestinationDecider` into a transport interface, reintroduces the import cycle S3-B's handoff records solving, and discovers at the end that a credential has to be resolved somewhere no design accounted for.

`TestDeliveryTransport_HTTPSDestinationPublishesToBrokerToday` kills it before any production code is written: it must pass on unmodified `main`, and the only way it can is that an `https`-transport destination is fully authorized, fully digest-verified, provenance-recorded with its URL — and then published to Kafka anyway. That single passing test converts the lane from "wire the transport up" into "build the routing seam, the dispatch-time resolver lifetime, and the trust-closed client", and it fixes the seam's shape before the first line.

---

## Lane S4-B — Slice 4.1e: Retention Policy and Purge Runtime

**Branch**: `feat/phase4-slice-4-1e-retention-purge`
Formerly "S3-C2" in `.loom/31:406,501`. Renamed because it is a Phase 4 slice, not a Sprint 3 remainder.

### The immutability-exemption decision (required deliverable; the lane's gate)

Corrections 11-16 mean a purge cannot be built until this is settled. The lane **must record a dated decision in `.loom/40-decisions.md` before writing a migration**.

| Option | For | Against |
|---|---|---|
| **A. Column-scoped exemption on the payload column, tombstone semantics.** Replace `integration_canonical_events`'s blanket `BEFORE UPDATE OR DELETE` with a `BEFORE DELETE` blanket guard plus a `BEFORE UPDATE` guard that raises unless the update changes **only** `payload_json`, sets it to a canonical tombstone object, and sets `purged_at`. Same shape for `integration_session_exports.record_json`. | Keeps "the schema, not convention, is the guarantee" — the exemption is itself schema-enforced and narrower than a role. Mirrors C1's own `reject_integration_receipt_provenance_mutation` idiom exactly (`0004_audit_immutability.sql:69-91`). Survives correction 12 without touching a single FK. The row, its identity, its classification, and its `recorded_at` remain, so an audit still shows what existed. | The trigger function grows real logic; it must be reviewed as security code. A tombstone is not a deletion — it must be documented as such, and a database backup taken before the purge still holds the payload. |
| **B. `session_replication_role = 'replica'` around the purge transaction.** | One line. | Disables **every** trigger on **every** table for that session, including the six C1 guards and the four lifecycle guards. Requires superuser or an explicit `GRANT SET ON PARAMETER`. Turns a scalpel into a switch. |
| **C. A separate purge role that owns the tables and disables triggers.** | Conventional role separation. | Empty as stated (correction 16): the application role already owns the tables and can drop the guards. Real separation is its own slice, not a line in S4-B. |
| **D. Tombstone in a side table; leave the payload.** | No trigger change at all. | Does not purge anything. The PHI stays. Fails the slice's only purpose. |

**Recommendation: A**, with the explicit written consequence that a tombstone is not a backup-inclusive deletion, and with option C filed as a named follow-up slice rather than a bullet.

### Goal

Give the PHI that actually persists a retention policy that can change without minting a new integration revision; add expiry state to the four retained classes; build one durable, multi-replica-safe purge component under S3-A's serve component contract; make the purge itself an attributed, append-only audit event; and reconcile all of it with S3-C1's immutability guarantees in the schema rather than around them.

### Non-Goals

- No lifting of `ErrUnsupportedRawRetention` (`internal/integration/processor/postgres_submission.go:179-181`). Production raw stays fail-closed; the lane documents it, as C1 did.
- No GraphQL surface for policy administration. The policy is a server-owned durable record loaded like the destination registry; the schema is frozen (see the map).
- No new redaction policy. 4.2a's `internal/integration/operator/payload.go` remains the operator-facing renderer.
- No role separation for the purge (option C). Filed, not built.
- No touching `internal/integration/delivery/store.go`, `dispatcher.go`, or the destination packages.

### Tasks

1. **Decision + policy placement.** Record the exemption decision above, and record the answer to correction 17: a new mutable, audited per-tenant `integration_retention_policies` record, with a fail-closed deployment default of "retain indefinitely" so an unconfigured deployment never purges anything. State explicitly why not the revision contract (immutability) and why not deployment config alone (no audit trail, no per-tenant scope).
2. **Expiry state on the four retained classes.** `integration_canonical_events` and `integration_session_exports` get `purge_after TIMESTAMPTZ` and `purged_at TIMESTAMPTZ` (processor `0005_*`, session `0006_*`); `integration_session_samples` gets `purge_after` only, because it is deletable outright (correction 14). Every new column is `NULL`-able with no backfill: a pre-slice row has no policy, and inventing one would be retroactive vouching (the 4.1b3 idiom). Partial indexes on `(purge_after) WHERE purged_at IS NULL`.
3. **The exemption migration.** Replace the two blanket guards per option A. Keep `integration_message_lineage`, `integration_delivery_audit`, `integration_delivery_operations`, `integration_batch_audit`, `integration_session_publications`, `integration_session_workflow_simulations`, and the four lifecycle tables blanket-guarded and untouched.
4. **Prune the fanout log** (correction 15). `integration_session_stream_events` gets a `BEFORE DELETE` exemption scoped to rows older than a deployment-configured window, or an explicit decision that it is never pruned and grows forever. Say which; do not leave it.
5. **The purge component.** New `internal/integration/retention` package following `autoroute.SweeperConfig`'s shape exactly (`internal/terminology/autoroute/sweeper.go:34-72`): `{Store, Interval, BatchSize, Observe}`, `PurgeOnce(ctx) (PurgeResult, error)`, `Run(ctx) error`, `Interval()`. The scan is an idempotent guarded `UPDATE … WHERE purge_after < now() AND purged_at IS NULL … RETURNING`, so it needs no lease (correction 20) — the `RETURNING` clause **is** the claim, and only the replica that claims a row writes its audit entry.
6. **Purge audit.** One append-only row per purged record: tenant, class, record ID, the policy record and version that authorized it, the effective `purge_after`, and a server-owned `purged_at`. Blanket `BEFORE UPDATE OR DELETE` guard, matching the C1 convention. A purge with no audit row must be impossible: write the audit row in the same transaction as the tombstone.
7. **Serve wiring.** Append after the autoroute block at `cmd/fi-fhir/main.go:5272`; bump `errCh` at `:5192` from 9 to 10; add `ComponentRetentionPurge` to `internal/observability/metrics.go:72-90` and to the not-configured list at `main.go:5202-5208`, `waiting` at `:5282-5290`, and `componentMetricNames` at `:5293-5301`. Add a bounded `Outcome` for the purge and extend the label allowlist test (`metrics.go:53-63`). Do not restructure anything.
8. **Repair and rewrite the posture doc** (corrections 18, 19). Rewrite `docs/operations/PHI-RETENTION.md` sections 2, 3, and 6; rewrite the `information_schema.columns` assertion in `TestPhiRetentionPosture_…` in the same MR; and fix the two drifted citations at `:83-84` and `:144` to `internal/integration/session/postgres.go:321-323`, `:324-333`, and `:907-911`.
9. **Correct the plan and the handoff.** `.loom/30-implementation-plan-integration-engide-completion.md:413-418` and `.loom/slice-handoff-phase-4-slice-4-1d-c1-phi-audit.md:187-195` both frame the trigger interaction as a `DELETE` problem. Correct both to state that C1 blocked deletion **and** redaction, and that FK `ON DELETE RESTRICT` chains plus undeletable state tables make row deletion structurally impossible (corrections 11-13).

### Acceptance Criteria

- A canonical event past its `purge_after` has `payload_json` replaced by the canonical tombstone, `purged_at` set, and one append-only purge-audit row — from the ordinary application role, in a live PostgreSQL 16 test.
- **Every other mutation of that table still raises**: changing `classification`, `recorded_at`, `correlation_id`, or `receipt_id`; setting `payload_json` to anything other than the tombstone; deleting the row.
- The five blanket-guarded audit ledgers and the two column-scoped state guards still raise on every mutation C1's kill-test asserts. `make phi-audit`, `make delivery-reliability`, and `make integration-session` all pass unchanged.
- A session sample past its `purge_after` is deleted; the row is gone and its `raw_cipher` is unrecoverable through the application.
- An exported session's export row is tombstoned, not deleted, and the session row remains — because it cannot be removed (correction 13), and the doc says so.
- A queued or dead-lettered delivery attempt's canonical event is **never** purged while the attempt is unresolved: the delivery `Claim` join (`internal/integration/delivery/store.go:107`) must never see a tombstone.
- Two replicas running the purge concurrently produce exactly one tombstone and exactly one audit row per record.
- An unconfigured deployment purges nothing.
- No PHI in any purge metric label, log line, or audit row beyond the record identifier that already exists in the durable set.

### Kill-Test (negative-controlled)

**Primary: `TestPhiRetention_PostgresExpiryPurgeAndAuditedTombstone`** — PostgreSQL 16, `-race`, two purge components against one database:

1. Seed one submission with a PHI sentinel in the canonical payload, one retained session sample with a second sentinel in `raw_cipher`, one export, and one queued delivery attempt on a **different** event. Set every `purge_after` in the past.
2. Run both purge components concurrently. Assert: exactly one tombstone per record, exactly one audit row per record, both sentinels absent from a full dump of `integration_canonical_events` and `integration_session_samples`.
3. Assert the queued attempt's event was **not** purged, and that a subsequent `RunOnce` on the delivery dispatcher claims and publishes it normally.
4. From the application role, attempt each of: `DELETE FROM integration_canonical_events`, `UPDATE … SET classification = 'phi', payload_json = '{"x":1}'`, `UPDATE … SET recorded_at = now()`, `UPDATE … SET payload_json = <tombstone>, correlation_id = 'x'`. Every one raises; row counts before and after are identical.
5. Run C1's own kill-test in the same job (`make phi-audit`) and assert it still passes.
6. Assert `information_schema.columns` now shows the expiry columns and that `docs/operations/PHI-RETENTION.md` sections 2, 3, and 6 were updated — the posture gate's assertion inverted, in the same commit.

**Day-1 gate, standalone test-only MR, must pass on unmodified `main`: `TestPhiRetention_PurgeIsStructurallyBlockedToday`.** Three assertions, each under 20 lines:
(a) `DELETE FROM integration_canonical_events WHERE …` raises; (b) `UPDATE integration_canonical_events SET payload_json = '{}'` **also** raises; (c) for an exported session, `DELETE FROM integration_session_exports` raises on the trigger and `DELETE FROM integration_sessions` raises on the foreign key. Passing on `main` proves corrections 11-13 and forces the exemption decision before any policy is designed.

**Negative control:** run assertions 1-4 against a second, independently provisioned database at the **pre-migration** schema — the same pattern C1 used and the same pattern that caught two real defects in C1's own proof (`.loom/30-implementation-plan…:666-670`). There, the purge must fail to write a tombstone (the trigger raises) and step 4's mutations must **succeed**. A pipeline where the pre-migration database also tombstones cleanly means the test is asserting against the wrong schema.

**Existence guard:** `-list '^(TestPhiRetention_PostgresExpiryPurgeAndAuditedTombstone|TestPhiRetention_PurgeIsStructurallyBlockedToday)$' ./internal/integration/retention | rg -x '…' | awk 'END { if (NR != 2) exit 1 }'`, in a new job `test:phi-retention-purge`.

### Verification

```bash
go test -race ./internal/integration/retention/... ./internal/integration/session/... \
  ./internal/integration/processor/... ./internal/observability/... ./cmd/fi-fhir/...

export POSTGRES_TEST_URL=...
make phi-retention-purge     # new target
make phi-audit               # C1's proofs and the rewritten posture gate
make integration-session
make delivery-reliability
make observability-replicas  # the new serve component must not break the two-replica proof
make check-runtime-config
```

### Riskiest Assumption

> **"Retention is a policy-design problem; the purge itself is a `DELETE` statement in a lease-fenced sweeper."**

That is the sprint scope's framing, the C1 handoff's framing (`.loom/slice-handoff-phase-4-slice-4-1d-c1-phi-audit.md:187-195`), and `PHI-RETENTION.md`'s framing (`:189-192`). It is wrong in three independent ways: the `DELETE` raises, the redaction `UPDATE` raises too, and even with both triggers lifted, three `ON DELETE RESTRICT` chains terminating in two undeletable state tables make row deletion impossible (corrections 11-13). A lane that spends its first week on policy shape will design TTLs the schema cannot honour and discover it in week two.

`TestPhiRetention_PurgeIsStructurallyBlockedToday` kills it in the first hour, from three assertions against unmodified `main`, and it converts the lane's opening deliverable from "a policy document" into "a dated decision about how C1's guarantee bends without breaking".

---

## Lane S4-C — Slice 4.4a: Migration Compatibility, Rollback Safety, and Restore Round-Trip

**Branch**: `feat/phase4-slice-4-4a-migration-compatibility`

### Re-scoping note (read before anything else)

Slice 4.4 as written — "Backup/restore, migration compatibility, rolling upgrade/rollback, chaos and DR, ACK latency, throughput, queue recovery, and batch-memory gates. Proof: every numeric budget in the product spec passes on the pinned reference profile with archived reports" (`.loom/30-implementation-plan…:794-799`) — is at least five slices, and one of them cannot be done in CI at all.

The seven numeric budgets, **verbatim** from `.loom/20-product-spec-integration-engine-ide-completion.md:267-280`:

1. "authenticated MLLP/HTTP durable-accept latency: p95 <= 250 ms and p99 <= 500 ms at 100 messages/second on 4 vCPU/8 GiB with destinations decoupled";
2. "one-hour steady-state: >= 250 2-KiB HL7 messages/second with no loss and zero duplicate receipt/event/outbox records for one idempotency key; transport retries retain that identity and may repeat delivery only under the declared at-least-once contract";
3. "1-GiB batch import: peak RSS <= 512 MiB above idle and successful restart from the last durable checkpoint";
4. "destination recovery: queued attempts resume without manual repair and without unbounded retry growth";
5. "PostgreSQL-backed RPO <= 5 minutes and service RTO <= 30 minutes in the tested backup/restore exercise";
6. "one-version rolling upgrade and rollback preserve receipts, revisions, and resumable work without schema downgrade corruption";
7. (matrix) "Kubernetes 1.36.x through Helm and Kustomize … render, install, upgrade, rollback, and live golden-journey evidence" (`docs/operations/SUPPORTED-1.0.md:24`).

**Nothing in the repository measures any of them** (correction 21). Budgets 1, 2, and 3 require the pinned 4 vCPU / 8 GiB profile, which the CI pool cannot provide truthfully (correction 29) and the shipped chart contradicts (correction 28). Budget 5's method cannot meet its own target (correction 27). Budget 7 needs a cluster.

Honest split:

- **4.4a (this lane, Sprint 4):** budget 6 plus the CI-runnable half of budget 5 — migration compatibility across replicas, one-version rollback safety, and a `pg_dump`/restore round-trip that proves receipts, revisions, and resumable work survive. All provable with one PostgreSQL service container.
- **4.4b (Sprint 5):** the performance harness for budgets 1-3, on a pinned reference host. Blocked on a runner decision, a chart-limits change, and the MLLP capacity choice (correction 32).
- **4.4c (Sprint 5):** budget 4 and budget 7 — chaos, DR with WAL/PITR, rolling upgrade and rollback on Kubernetes 1.36, plus the first real CI job for `./test/e2e/...` (correction 26).
- **4.4d (Sprint 5):** `log/slog` structured logging then the OpenTelemetry exporter, in that order (corrections 30-31). 4.4a resolves only the *artifact* half: the five deployment artifacts that still advertise tracing.
- **4.4e (Sprint 5+):** durable per-deployment MLLP token bucket (correction 32).

### Goal

Define what "one version" means in a repository with no tags; make every migrator safe under concurrent replica startup; make a one-version binary rollback survivable against a migrated schema; and prove that a `pg_dump`/restore round-trip preserves receipts, revisions, and resumable delivery work.

### Non-Goals

- No performance measurement of any kind. No latency, throughput, or RSS assertion.
- No Kubernetes cluster work, no chaos injection, no Helm upgrade/rollback exercise.
- No OpenTelemetry exporter and no `log/slog` logger. This lane only removes or gates the *artifacts* that advertise tracing.
- No touching migrations owned by S4-B (processor, session). This lane's only schema change is a `DEFAULT` on three existing columns.
- No promotion of `test:benchmark` to cover anything new.

### Tasks

1. **Define N-1.** Record a dated decision: the compatibility boundary is the **per-package migration ledger version**, not a git tag, because there are none (correction 24). "One version back" means "the schema at the previous version of every ledger, running the binary from the merge commit that introduced the current one". Add a `SchemaVersion` accessor per package so `fi-fhir version` and `fi_fhir_build_info` can report the six ledger versions, and bump `const version` (`cmd/fi-fhir/main.go:57`) to something that changes.
2. **Fix the rollback defect** (correction 23). Add server-side `DEFAULT`s to `integration_session_exports.principal_json`, `reason`, and `include_raw_payload`, using the exact `unattributed_legacy_export` sentinel the migration already backfills with (`0004_export_attribution.sql:25-29`). This is the 4.1b3 no-retroactive-vouching idiom applied forward: an N-1 binary's five-column insert succeeds and is **visibly** unattributed, rather than failing. The alternative — declaring rollback unsupported — requires a dated decision relaxing a product-spec target (`20-product-spec…:249-250`); present it and reject it.
3. **Lock the terminology migrator** (correction 25). Add `pg_advisory_xact_lock` to `pkg/terminology/db.Migrator.Initialize` (`pkg/terminology/db/migrations.go:67-108`) inside a transaction, matching the five integration migrators exactly (`internal/integration/processor/postgres_submission.go:125-130`).
4. **A migration-authoring rule, written down.** Add it to `AGENTS.md` and `docs/developer-guide/testing.md`: a new `NOT NULL` column on an existing table carries a `DEFAULT` or the migration breaks one-version rollback. S4-B's two migrations are the first consumers.
5. **Restore round-trip.** A `scripts/` helper and a test that `pg_dump`s a database holding a receipt, a canonical event, a lineage row, a queued outbox row, a session with a retained sample, and an export; restores into a second database; and proves the delivery worker resumes the queued attempt from the restored state. Assert that the C1 immutability triggers and the 4.1c-a `NOT VALID` constraint survive the round-trip — a `pg_dump` that silently drops a trigger is a PHI-governance regression.
6. **State the RPO gap** (correction 27). `docs/operations/PRODUCTION-HARDENING.md:883-926` documents `pg_dump` against a 5-minute RPO it cannot meet. Correct the doc to say what the documented method actually achieves, and record WAL archiving / PITR as the 4.4c prerequisite for budget 5.
7. **Resolve the tracing artifacts** (correction 30). Either strip `FI_FHIR_TRACING_*` from `docker-compose.yaml:104-105`, `deploy/kubernetes/base/deployment.yaml:121`, `deploy/helm/fi-fhir/templates/deployment.yaml:103-105`, `deploy/kubernetes/overlays/production/kustomization.yaml:58`, `configs/full-stack.env:74-75`, and `README.md:407`, or add the same "NOT IMPLEMENTED" label `.env.example:218-220` carries. Extend `scripts/check-runtime-config.sh` with an assertion that no deployment artifact sets a tracing variable without the label — the same shape as its existing legacy-observability-mode check (`:210-213`).
8. **Correct the reference profile contradiction** (correction 28). Either raise the Helm chart's limits to the 4 vCPU / 8 GiB reference profile, or record in `docs/operations/SUPPORTED-1.0.md` that the chart defaults are explicitly not the profile and that 4.4b must supply a profile values file. Do not measure anything; just stop the two documents from disagreeing.
9. **File 4.4b/c/d/e** in `.loom/30-implementation-plan-integration-engine-ide-completion.md` under Slice 4.4, replacing the single bullet at `:794-799` with the honest split and the reasons, so the release-gate map's "Release Candidate: 4.1-4.4" (`:855`) names something achievable.

### Acceptance Criteria

- `fi-fhir version` and `fi_fhir_build_info` report the six ledger versions; two replicas at different ledger versions are distinguishable in metrics.
- Two `serve` processes starting simultaneously against one **fresh** database converge to one consistent schema across all six ledgers, with no duplicate-object error from any of them — including terminology.
- The pre-4.1d five-column export insert succeeds against the current schema and produces a row whose `principal_json` carries the `unattributed_legacy_export` sentinel.
- A `pg_dump`/restore round-trip preserves every receipt, canonical event, lineage row, session artifact revision, and export; the restored database still raises on every C1-guarded mutation; and a queued outbox row is claimed and published from the restored state without manual repair.
- `make check-runtime-config` fails if any deployment artifact sets a tracing variable that is not labelled unimplemented.
- `docs/operations/SUPPORTED-1.0.md` and `deploy/helm/fi-fhir/values.yaml` no longer contradict each other.
- Every existing blocking proof passes unchanged.

### Kill-Test (negative-controlled)

**Primary: `TestMigrationCompatibility_ConcurrentReplicaMigrationRollbackAndRestore`** — PostgreSQL 16, `-race`:

1. Two goroutines call every package's `Migrate`/`Initialize` simultaneously against one fresh database. Both return `nil`; every ledger holds exactly one row per version; no object is duplicated.
2. Repeat against a database pre-seeded at each ledger's previous version.
3. Issue the exact pre-4.1d five-column `INSERT INTO integration_session_exports` and assert it succeeds with the sentinel principal.
4. `pg_dump` a fully populated database, restore into a second, and assert row-for-row equality on the six durable classes; assert every C1 trigger still raises; assert the `integration_delivery_identity_decisions` provenance CHECK is still present and still `NOT VALID`.
5. Run a delivery `RunOnce` against the restored database and assert the queued attempt publishes.

**Day-1 gate, standalone MR, must FAIL on unmodified `main` for the stated reason: `TestMigrationCompatibility_ExportInsertShapeSurvivesOneVersionRollback`.** Step 3 alone. It must fail with a not-null violation on `principal_json` — not with a harness error, not with a connection skip. The MR lands the test in a **non-blocking** job with the expected-failure recorded in the worklog, then the fix and the promotion to blocking land together. Reproducing the defect *before* fixing it is what distinguishes this from a green test written after the code.

**Negative control:** revert task 2's `DEFAULT`s in a build flag and assert step 3 fails; revert task 3's advisory lock and assert step 1 fails against a fresh database under concurrency. Both controls must fail the job. A control that passes means the test is not exercising the mechanism.

**Existence guard:** `-list '^(TestMigrationCompatibility_ConcurrentReplicaMigrationRollbackAndRestore|TestMigrationCompatibility_ExportInsertShapeSurvivesOneVersionRollback)$' ./internal/integration/migrationcompat | rg -x '…' | awk 'END { if (NR != 2) exit 1 }'`, in a new job `test:migration-compatibility` with a dedicated `fi_fhir_migration_compat_test` database. The job installs `postgresql-client` for `pg_dump`.

### Riskiest Assumption

> **"Migration compatibility is a documentation and process concern; the ledgers already work, so 4.4a is mostly writing down what N-1 means."**

Two facts kill that: `pkg/terminology/db.Migrator.Initialize` takes no advisory lock while the other five migrators do (correction 25), so concurrent replica startup against a fresh database is already a race; and S3-C1's own migration makes a one-version rollback fail with a not-null violation (correction 23). The budget the plan text promises — "rolling upgrade and rollback preserve receipts, revisions, and resumable work" — is **currently false**, in code that merged today.

The day-1 gate kills the assumption by failing, once, for a named reason, before any fix is written. If it fails for any other reason the lane re-scopes; if it passes, correction 23 is wrong and this document gets corrected first.

---

## Lane S4-E — Transport-Gate Role Narrowing

**Branch**: `feat/phase4-transport-gate-roles`
Small. `.loom/31` correction 20, explicitly filed "do not silently inherit" and re-filed by C1 (`.loom/slice-handoff-phase-4-slice-4-1d-c1-phi-audit.md:202-206`, `docs/operations/PHI-RETENTION.md:194`).

**Does it deserve its own slice this sprint? Yes** — and the reason is specific rather than hygienic. Three slices have now shipped fine-grained roles one layer beneath a binary gate: `integration.delivery.operator` (2.3, `internal/integration/delivery/types.go:17`), `integration.operator` / `integration.deployment.operator` (4.2a), and `integration.phi.export` (S3-C1). The transport gate still says yes to all of them together for anyone holding `graphql:operator` (`internal/api/graphql/operation_authorization.go:50-52`). Every additional fine-grained role makes the gap wider, and the cost of closing it is one Go file (correction 33). It is also the last Phase 4 item that could plausibly be called an RBAC gap at the Release Candidate gate (`.loom/30-implementation-plan…:855`).

**Does anything in 4.2b's merged UI depend on the blanket role?** In code, no — `ui/src/**` has zero occurrences. In practice, yes: five documents instruct operators to grant exactly `graphql:operator` and nothing else, including the UI's own developer guide (correction 34). The narrowing therefore must ship the role mapping, the fixtures, and all five docs in one MR, or every existing operator deployment loses the control plane.

**Goal.** Replace the blanket allow with an explicit per-root-field role mapping over all 131 root fields, default-deny, with `graphql:operator` retained as a named compatibility grant that expands to the full set and logs at startup that it is in use.

**Non-Goals.** No schema change. No change to any resolver-layer or service-layer decision — the fine-grained checks inside `operator.Service.authorize` and `session` stay exactly as they are; this is defence in depth, not a relocation. No new roles beyond those already shipped.

**Tasks.** (1) Enumerate all 64 `Query`, 60 `Mutation`, and 7 `Subscription` fields into a compile-time map, with a test that fails when a schema field has no entry — so a future field cannot be added without a role. (2) Map each to an already-shipped role; anything that does not fit gets `graphql:operator` explicitly and a `TODO` naming the slice that should narrow it. (3) Keep `integration:preview` and the stream-context rule byte-for-byte (`operation_authorization.go:70-88,90-117`). (4) Update all five docs and the two test fixtures. (5) Log the compatibility grant at startup, as `serve` already does for the delivery identity mode (`cmd/fi-fhir/main.go:5245-5249`).

**Acceptance Criteria.** A token holding only `integration.operator` reaches every 4.2a read and no mutation; adding `integration.delivery.operator` reaches replay/resubmit/discard and no deployment command; `integration.phi.export` alone reaches nothing at the transport gate. A token holding only `graphql:operator` behaves exactly as today. A new root field with no mapping fails a unit test, not a production request.

**Kill-Test (negative-controlled): `TestTransportGate_FineGrainedRolesReplaceBlanketOperator`** — the real GraphQL handler with real 4.1a OIDC tokens, one assertion per role combination above, plus an assertion that the field-coverage map is exhaustive against the parsed schema. **Day-1 gate, must pass on unmodified `main`:** the same test's first case, asserting that a token holding `integration.operator` + `integration.deployment.operator` and **not** `graphql:operator` is refused at the transport gate for every 4.2a control-plane operation — proving that the merged UI's fine-grained roles are decorative at the gate today. **Negative control:** restore the blanket `return nil` at `:50-52` behind a build flag and assert every least-privilege case fails open. New job `test:transport-gate`; do not alter `test:operator-control-plane`'s arity.

**Riskiest Assumption.** *"The fine-grained roles 2.3, 4.2a, and S3-C1 shipped are already sufficient to express every operation."* They cover delivery, deployment, operator reads, and PHI export — but 131 root fields include the legacy workflow catalog, FHIR subscriptions, and the event/patient browser, none of which any slice has assigned a role to. The exhaustiveness test kills it on the first run: if more than a handful of fields land in the explicit-compatibility bucket, the lane is bigger than one file and should re-scope to the integration surface only, leaving the legacy catalog behind the compatibility grant with a named follow-up.

---

## Lane S4-D — Phase 5.1 Parallel Start: Recommendation

**Recommendation: hold the code slice; start a docs-only 5.1a in Sprint 4.**

The release-gate map requires 5.1-5.3 only for 1.0 and 4.1-4.4 for RC (`.loom/30-implementation-plan…:855-856`), so there is no gate pressure. The disjointness question has a mixed answer: `pkg/fhir/*` and `pkg/validate/*` are touched by no Sprint 4 lane, but `cmd/fi-fhir/main.go`, `.gitlab-ci.yml`, `Makefile`, `go.mod`, and `docs/operations/*` are shared with all of them, and `pkg/validate/...` sits inside the `test:benchmark` package set S4-C is rewriting (correction 38). That is manageable.

What is not manageable is correction 36: **the durable integration engine produces no FHIR resource.** `pkg/fhir` is reachable only from the legacy engine and the `fhir validate` CLI. Pinning R4 4.0.1 and US Core 9.0.0 and integrating the official validator would certify a code path that no golden journey executes, at the same time as Lane S4-A is building the transport that would eventually carry one. Journey 1's "inspect trace and FHIR delivery" and journey 6's US Core validation both wait on 4.1c-b, not on a validator.

Two further blockers make a code start unwise this sprint: the "official validator" is the HL7 `validator_cli.jar`, i.e. a Java runtime in CI and possibly in the shipped image — a supply-chain and image decision, not a code slice (correction 37); and `.loom/28` is stale on the IG version, the tracking issue, and the dependency map (correction 35).

**Start instead, as a fourth non-blocking lane touching only `.loom/` and `docs/`:**

1. Rewrite `.loom/28-spec-fhir-ig-bulk-smart.md` against `origin/main`, correcting USCDI v3 → US Core 9.0.0, and replacing "the existing FHIR mapper" framing with the fact that it is legacy-engine-only (`internal/workflow/actions.go:680`, `cmd/fi-fhir/main.go:49,309`).
2. Publish the USCDI / US Core 9.0.0 coverage matrix against `pkg/fhir/mapper.go` and `pkg/fhir/validate.go:153-238`, naming exactly which resources and which required elements the hand-rolled checker covers and which it does not.
3. Record a dated decision on validator integration: `validator_cli.jar` as a CI service container versus a Go structural validator over pinned `.tgz` IG packages versus keeping the current profile-URL presence check and saying so in `SUPPORTED-1.0.md`. Include the image-size and supply-chain consequences.
4. Record the sequencing dependency in `.loom/30`: 5.1's "conformance policy in artifacts and diagnostics" cannot be specified until 4.1c-b defines what a FHIR destination is.

This costs no CI, no schema lock, no shared Go file, and it de-risks a 1.0 gate. Zero code.

---

## Suggested Execution Order

**Pre-sprint (blocking):** none. Sprint 4 branches from `a9c8ea59`. If !147 merges, lanes write per-entry worklog files; if !144/!146 merge, they affect nothing in scope.

**Day 1 — four gates, in parallel, each a standalone test-only MR against `main`.** Each must produce its stated result for its stated reason before its lane writes production code:

| Gate | Lane | Expected on `main` | Kills |
|---|---|---|---|
| `TestDeliveryTransport_HTTPSDestinationPublishesToBrokerToday` | S4-A | **Pass** | "the `Transport` field already routes" |
| `TestPhiRetention_PurgeIsStructurallyBlockedToday` | S4-B | **Pass** | "purge is a `DELETE` in a sweeper" |
| `TestMigrationCompatibility_ExportInsertShapeSurvivesOneVersionRollback` | S4-C | **Fail**, on a not-null violation on `principal_json` | "rollback already works" |
| `TestTransportGate_LeastPrivilegeIsRefusedToday` | S4-E | **Pass** (the refusal is observed) | "4.2a's roles already gate the transport" |

**If any gate disconfirms this document, the affected lane corrects this file before writing production code.** That is the rule that produced corrections 11, 23, and 36, and it is the only reason those are in a plan rather than in a post-mortem.

**Day 1 also:** S4-A's transport decision and S4-B's immutability-exemption decision land as docs-only commits in `.loom/40-decisions.md` within the first day, because S4-B's migration shape and S4-C's migration-authoring rule both depend on knowing whether C1's triggers are being amended.

**Wave 2 — implementation, parallel; merge order C → A → B → E.**

1. **S4-C (4.4a)** first: smallest shared footprint, and its `DEFAULT`-on-`NOT NULL` rule (task 4) must exist before S4-B writes two migrations that would otherwise repeat correction 23.
2. **S4-A (4.1c-b)** second: largest new runtime, entirely within its own packages plus `cmd/fi-fhir/delivery_runtime.go` and `destination_identity_runtime.go`.
3. **S4-B (4.1e)** third: the only lane appending to the serve component table, so it rebases onto a settled `cmd/fi-fhir/main.go` and onto S4-C's migration rule.
4. **S4-E** last: its test-fixture changes span three packages and are cheapest to rebase over three merged lanes.
5. **S4-D-doc** merges whenever; it touches nothing any other lane touches.

**Wave 3 — Sprint 5.**
- **4.4b** — performance budget harness for spec budgets 1-3, on a pinned reference host. Blocked on: a runner decision (correction 29), the chart-limits reconciliation S4-C leaves as a decision (correction 28), and the MLLP capacity choice (correction 32).
- **4.4c** — budget 4 and 7: chaos, DR with WAL/PITR, rolling upgrade and rollback on Kubernetes 1.36, and the first real CI job for `./test/e2e/...` (correction 26).
- **4.4d** — `log/slog` structured logging, then the OpenTelemetry exporter, in that order (corrections 30-31).
- **4.4e** — durable per-deployment MLLP token bucket.
- **5.1** — FHIR R4 / US Core conformance code, after 4.1c-b defines a FHIR destination.
- **Purge role separation** — the follow-up S4-B files rather than builds (correction 16).

---

## Sources

Destination delivery:
- `internal/integration/delivery/dispatcher.go:15-19,44-49,52-61,92-115,118-157,166-182,222-235,237-277`
- `internal/integration/delivery/identity.go:14-24,43-50,52-64,112-125`; `types.go:15-20,62-65,82-115,117-126`
- `internal/integration/delivery/store.go:42,57,86-88,95,99,107-108,162-198,216-330,337-345,346-500,510+,620-660,663+`
- `internal/integration/delivery/kafka.go:91`; `delivery_integration_test.go:473-486`; `destination_fixture_test.go:326-338`
- `internal/integration/delivery/destination_contact_integration_test.go:20-45,46-145,151-166,170-202,204-213,218-229`
- `internal/integration/destination/revision.go:34-49,54-62,75-98,112-149,182-236,242-253,255-304`
- `internal/integration/destination/registry.go:30-42,47-53,71-133,150-203,205-228,242-252`
- `internal/integration/destination/identity.go:12-30,32-67,69-97,99-151,161-217,219-237,242-282`
- `internal/integration/destination/postgres.go:43-80`; `migrations/0001_delivery_identity.sql:14-33,28,35-58,60-69`
- `cmd/fi-fhir/destination_identity_runtime.go:16,20-24,37-96,98-114,116-141,149-166,168-200,202-211,213-231,233-255`
- `cmd/fi-fhir/delivery_runtime.go:19-29,31-51,53-66,60-66,67-73,143-148`
- `pkg/integration/secret.go:8-11,13-22,24-43,45-61`
- `.loom/slice-handoff-phase-4-slice-4-1c-a-destination-identity.md:83-110,111-127,129-155,156-179`

Retention and purge:
- `internal/integration/processor/migrations/0001_atomic_submission.sql:1-17,19-33,35-55,57-76,78-96`
- `internal/integration/processor/migrations/0004_audit_immutability.sql:1-20,22-47,49-65,67-114,116-119`
- `internal/integration/session/migrations/0001_session_workspace.sql:1-11,13-26,82-94`
- `internal/integration/session/migrations/0003_publications.sql:26-47`; `0004_export_attribution.sql:20-38,40-45,47-58`; `0005_session_stream_events.sql:12-30,32-41,43-47,48-57`
- `internal/integration/batch/migrations/0003_batch_audit_immutability.sql:19-20`
- `internal/integration/session/postgres.go:63-160,72,306-336,321-323,324-333,888-911,907-911,949-954`
- `internal/integration/processor/postgres_submission.go:38-39,121-160,130,176-182,262-278,359-389`
- `internal/integration/lifecycle/postgres.go:52-85,61`; `internal/integration/batch/store.go:99-140,108`; `internal/integration/destination/postgres.go:43-80,53`
- `pkg/integration/revision.go:108-157`
- `internal/terminology/autoroute/sweeper.go:34-72,75-121`
- `docs/operations/PHI-RETENTION.md:12-17,50-72,76-92,96-145,149-181,185-194,196-208`
- `.loom/slice-handoff-phase-4-slice-4-1d-c1-phi-audit.md:165-201,202-206`
- `.loom/40-decisions.md:1336-1350,1394-1401`

Recovery, upgrade, performance:
- `.gitlab-ci.yml:515-607,974-1006,1015-1068,1054-1059,1216-1271,1228,1235-1240,1397-1444,1455-1490,1476-1482`
- `Makefile:1-11,62-67,94-163,133-136,147-153,159-162,200-209`
- `pkg/terminology/db/migrations.go:16-62,65-108,110-117`; `pkg/terminology/db/schema.go:7,515-522,708,737`
- `cmd/fi-fhir/main.go:57,1944,2861-2905`; `internal/workflow/loadtest.go:13-62,98-104,146-158`
- `test/e2e/integration_test.go:8,54,138,219,274,323-338,339-427,345,406,413-426`
- `docs/operations/SUPPORTED-1.0.md:14-31,33-48,50-63`
- `docs/operations/PRODUCTION-HARDENING.md:668,883-905,924-926`
- `docs/operations/PRODUCTION-MLLP.md:42-71`
- `pkg/config/config.go:38-39,180-195,416-421,605-613,758-767`
- `.env.example:102,142,155-158,207-232`; `scripts/check-runtime-config.sh:66,205-213`
- `docker-compose.yaml:104-105`; `configs/full-stack.env:74-75`; `README.md:407`
- `deploy/kubernetes/base/deployment.yaml:10,19-22,121,145-162`; `deploy/helm/fi-fhir/templates/deployment.yaml:103-105,169-186`; `deploy/kubernetes/overlays/production/kustomization.yaml:58`
- `git tag` at `origin/main`: empty

Observability substrate (S3-A, the base every lane appends to):
- `internal/observability/mode.go:1-19,26-79`; `metrics.go:16-63,64-90,92-99,119-186,187-268,281-286`; `health.go:23-68,75-91,92-122,123-179,180-225,226-240,265-310`; `server.go:18,20-45,94-145`
- `cmd/fi-fhir/main.go:4884-4902,5094-5108,5110-5129,5131-5144,5146-5168,5181-5229,5230-5272,5273-5309`
- `cmd/fi-fhir/serve_observability.go:19-126,127-182,183-303`
- `internal/api/graphql/resolvers/schema.resolvers.go:2153-2178`
- `.loom/40-decisions.md:1309-1427`; `.loom/slice-handoff-phase-4-slice-4-3-observability.md:63-67,68-79,81-93`

Transport gate:
- `internal/api/graphql/operation_authorization.go:14-19,38-59,61-88,90-117,119-130`
- `internal/api/graphql/schema.graphql:519-565,831-871,885-894` plus 19 `extend type` blocks (64 Query / 60 Mutation / 7 Subscription root fields)
- `docs/developer-guide/development-setup.md:276`; `docs/operations/INTEGRATION-SESSIONS.md:43-46,76`; `docs/operations/PRODUCTION-HARDENING.md:668`; `docs/planning/GRAPHQL-API.md:97`; `ui/docs/DEVELOPER-GUIDE.md:51`
- `internal/integration/delivery/types.go:17`; `internal/integration/operator/types.go:17-19`

Standards:
- `pkg/fhir/validate.go:8-52,104-152,153-178,179-238`; `pkg/fhir/types.go:13,92,95`; `pkg/fhir/mapper.go`
- `internal/workflow/actions.go:650-652,679-680,720,797,858-861`; `internal/workflow/validate.go:645-649`
- `cmd/fi-fhir/main.go:49,309`
- `.loom/28-spec-fhir-ig-bulk-smart.md:1-51` (esp. `:5,23,36-38`)

Plan, spec, and process:
- `.loom/30-implementation-plan-integration-engine-ide-completion.md:388-418,558-634,635-670,672-753,755-799,801-829,831-844,846-856,858-872`
- `.loom/20-product-spec-integration-engine-ide-completion.md:213-233,246-264,265-287,289-304,316-327`
- `.loom/31-sprint3-execution-specs.md:143-175,301-306,385-387,483-502`
- `AGENTS.md:214-268,237-258,413-427`
- `.loom/50-worklog.md` (single file, 1037 lines, at `a9c8ea59`); `origin/docs/worklog-one-file-per-entry` unmerged
