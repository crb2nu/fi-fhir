# 33 — Sprint 5 Execution Specs (Release Candidate gate)

**Status**: Ready for agent pickup (created 2026-08-09)
**Owner**: platform
**Base commit read**: `main` @ `852d7f3ee` (merge `feat/phase4-slice-4-4a-migration-compatibility`). Every `file:line` below is a `852d7f3ee` line number, read or executed against the canonical checkout. Sprint 4 is fully merged: 4.1c-b (`e77c6218b`), 4.1e, transport-gate roles (`895b97412`), 4.4a (`e5f8e2082`), 5.1 docs prep (`67b874e5`).
**Inputs**: `.loom/32-sprint4-execution-specs.md` (structure, correction-log style, day-1 gates, ownership map — the template), `.loom/31-sprint3-execution-specs.md`, `.loom/30-implementation-plan-integration-engine-ide-completion.md` (Slice 4.4 split at `:887-946`, release gates), `.loom/20-product-spec-integration-engine-ide-completion.md:265-280` (the numeric budgets), `.loom/28-spec-fhir-ig-bulk-smart.md` (rewritten by !155), `docs/planning/FHIR-CONFORMANCE-MATRIX.md`, `docs/operations/{SUPPORTED-1.0.md,PRODUCTION-HARDENING.md,PRODUCTION-MLLP.md,PHI-RETENTION.md,DESTINATION-IDENTITY.md}`, `.loom/40-decisions.md` (esp. the **PROPOSED, unratified** 5.1 entry at `:1428`).

## Goal

Turn the Release Candidate gate — 4.4b, 4.4c, 4.4d, 4.4e, purge role separation — plus the 5.1 code question into lanes that run in parallel without colliding, and correct from code the parts of the sprint scope that are wrong. **Four premises in the scope description invert, one of them decisively: 5.1 is not unblocked.** Four defects merged in Sprint 4 are named below with executed evidence; two are release blockers.

## Non-Goals

- Do not implement in this planning slice.
- Do not reopen 4.1a/4.1b/4.1c/4.1d/4.1e/4.2/4.3/4.4a contracts; extend them.
- Do not restructure S3-A's `runServe` background-component table (`cmd/fi-fhir/main.go:5232-5237` says so in the code). Lanes append — and see correction 47 before they do.
- Do not start Phase 5.2 (SMART/Bulk) or 5.3 in any form.
- Do not measure a wall-clock budget in the shared CI pool. See the pinned-runner decision.

---

## Found Defects (Sprint 4 merged code)

Hunted adversarially against the five merged lanes, on the assumption that each lane's own tests do not cover its gaps. `go build ./...`, `go vet ./...`, and every unit test pass; `TestTransportGateRoleMapIsExhaustive` passes. The four below were found anyway, and the two HIGH ones were proved by execution against a live PostgreSQL 16 using `go test -overlay` with sources outside the repo.

### D1 — HIGH — The retention purge has a hard ceiling of 200 records per class per hour, and nothing drains the backlog

`internal/integration/retention/purger.go:142-159`: `Run` calls `PurgeOnce` **once** per ticker tick and then blocks on the tick. Every purge and stamp statement carries `LIMIT $3` (`internal/integration/retention/store.go:311,339,363,409,441,474,512`) bound to `defaultBatchSize = 200` (`store.go:33`). The shipped cadence is `defaultRetentionCadence = time.Hour` (`cmd/fi-fhir/retention_runtime.go:22-23`). There is no `continue`-on-full-batch.

**Proved by execution**: 500 expired canonical events drained in three `PurgeExpired` calls — 200, 200, 100. Under `Purger.Run` that is three hourly ticks. Sustained ceiling **200 records/class/hour = 0.056/sec**, on what `store.go:31-33` itself calls "the busiest table in the system".

Compounding: a canonical event cannot be purged until `purge_after` is stamped (`store.go:390`), nothing outside the retention store writes that column, and `stampCanonicalEvents` is bounded by the same 200. A newly ingested event is unpurgeable until a stamp pass reaches it.

**The contract it violates**: `docs/operations/PHI-RETENTION.md:114-115` documents cadence and batch size as independent knobs and never multiplies them. The batch bound's own stated rationale (`store.go:31-33`) is transaction size, never aggregate rate. The house pattern is the opposite — `internal/integration/session/stream.go:174-179` drains with the comment *"A full batch means there is more backlog; keep going rather than waiting a whole tick per batch."* The cited precedent `internal/terminology/autoroute/sweeper.go:104-119` has the same loop shape, but its pass is **unbounded** (`pkg/terminology/db/mappings.go:1030-1040`, no `LIMIT`), so one tick clears everything. The purger inherited the loop and added a `LIMIT` the original never had.

**Why no test catches it**: every retention test seeds ≤2 rows per class and calls `PurgeOnce` exactly once. `defaultBatchSize = 1` would leave the suite green. There is also no backlog gauge — retention metrics are counters only (`internal/observability/metrics.go:213-217`), so "falling behind" is indistinguishable from "healthy and busy".

**Owner**: Lane S5-F. **Fix shape**: drain-on-full-batch bounded by a wall-clock budget per tick, plus a backlog gauge and a test that seeds more than one batch.

### D2 — HIGH — The HTTPS provenance write shares the destination's timeout budget, so a slow-but-successful destination loses its ledger row and kills the delivery worker

`internal/integration/destination/transport.go:237-238` passes the **same** `ctx` to `deliverHTTPS` and to `t.record`. That ctx is `context.WithTimeout(ctx, d.config.PublishTimeout)` (`internal/integration/delivery/dispatcher.go:246`). A destination that answers 2xx slowly leaves no budget for the durable ledger write.

**Proved by execution**: destination hits = 1, HTTP 200 returned, provenance rows written = **0**, error = `context.DeadlineExceeded`. That error is not a `TransportFailure`, so `transportFailure` returns false (`internal/integration/delivery/transport.go:85-99`) and `dispatcher.go:260-262` surfaces it raw. `RunOnce` returns the error with `MarkPublished = 0, MarkFailed = 0`; `Run` returns the error and **the delivery worker component exits**. The attempt stays leased, the lease expires, and the same payload is redelivered to a destination that already accepted it.

**The contract it violates**: `internal/integration/destination/migrations/0002_https_delivery_provenance.sql:23-26` states that *"absence of a row means this process contacted no destination for that attempt, never that it did so and the record was lost."* That is exactly the state produced. `transport.go:239-245` frames the failure as "a provenance outage", but here it is self-inflicted by the shared budget rather than by an outage. The existing `TestTransportSurfacesAProvenanceOutage` (`internal/integration/destination/transport_test.go:367`) covers a recorder that *returns* an error, not one starved of budget.

**Owner**: Lane S5-0 (MR 0b). **Fix shape**: give `t.record` its own budget derived from the parent context, not the destination-facing one — the provenance write must be able to complete after the destination-facing deadline has expired.

### D3 — MEDIUM — 4.4a's restore round-trip: three of six immutability assertions are foreign-key-shadowed, and the restored database's ledgers are never checked

`internal/integration/migrationcompat/compatibility_integration_test.go:275-318`. Its own doc comment (`:270-274`) claims *"A dump/restore that recreated the tables without their triggers would leave a database that looks complete and silently permits every mutation C1 forbids."*

**Proved by execution**: built the full schema, seeded the fixture, dropped **all 17** non-internal triggers, and re-ran the six mutations. Three still failed — delete a canonical event, delete a receipt, delete a delivery attempt — every one with `violates foreign key constraint`, not a trigger message. Only three of six are real trigger proofs. The FK-shadowing is spelled out in `internal/integration/processor/migrations/0005_retention_expiry.sql:9-12`, so the test author knew about it. There is no negative control that drops a trigger and requires the round-trip to go red.

Two related gaps in the same test:
- `assertEveryLedgerAtDeclaredVersion` runs on the fresh, head, and N-1 databases (`:48,:60,:80`) but **not** on the restored one (`:103-109`). A restore that lost the six `*_schema_migrations` ledgers passes.
- `durableClasses` (`:216-225`) omits `integration_session_samples`, `integration_session_stream_events`, and all three retention tables, so they are empty in the dump. Five 4.1e triggers are never exercised post-restore: `integration_session_exports_undeletable`, `integration_session_stream_events_append_only`, `integration_session_stream_events_prunable`, `integration_retention_purge_audit_immutable`, `integration_retention_policy_audit_immutable`.

**Owner**: Lane S5-B. The restore proof is 4.4c's foundation; repairing it is the lane's first task, not a nice-to-have.

### D4 — MEDIUM — The migration-authoring rule 4.4a wrote does not enforce the rule it documents

`AGENTS.md:210` states the rule as *"A new `NOT NULL` column on an existing table carries a `DEFAULT`"* and names `TestMigrationRule_NotNullOnExistingColumnCarriesADefault` as mechanical enforcement. But `internal/integration/migrationcompat/migration_rule_test.go:25` matches only `ALTER\s+COLUMN\s+…\s+SET\s+NOT\s+NULL`.

**Proved by execution** (ran the test's own regexes): `ALTER TABLE t ADD COLUMN new_col TEXT NOT NULL;` yields `tightened=0`, so `assertTightenedColumnsHaveDefaults` returns early at `:100-102` and the file is never inspected. That form produces the identical `SQLSTATE 23502` rollback failure the rule exists to prevent — and it is the form the rule's own title names. This is `.loom/32` correction 23 reappearing with the guard installed backwards.

Secondary: the baseline exemption at `migration_rule_test.go:37-43` reads *"The processor ledger head is 4"* and *"needs processor 0005, owned by Lane S4-B."* Processor head is now **5** (`internal/integration/processor/postgres_submission.go:80`) and 0005 shipped in Sprint 4 without the repair. Stale and unpoliced.

**Owner**: Lane S5-0 (MR 0c). This must merge before S5-D and S5-F author migrations, for exactly the reason 4.4a's task 4 existed.

### D5 — LOW, but live — `make dev-ui-down` has been broken since Sprint 4 by a stray `.PHONY` fragment inside a recipe

`Makefile:725` is a lone `\tdestination-transport \` line sitting between `dev-ui-down`'s recipe and the `destination-transport` target's comment block. GNU make treats a tab-indented line after a blank line as a continuation of the open recipe, and the trailing backslash joins the following comment, so `make dev-ui-down` runs `docker-compose down` and then tries to execute `destination-transport # Slice 4.1c-b: ...` as a shell command.

**Proved by execution**: `make -n dev-ui-down` prints three lines — `docker-compose down`, `destination-transport \`, and the comment.

It is a `.PHONY` continuation that a Sprint 4 rebase dropped 700 lines away from the `.PHONY` block. No CI job invokes `dev-ui-down`, which is why nothing caught it. It is listed here because it is the same failure mode as `test:destination-transport` at `:2672` — a rebase placing a line where nobody reads it — in the second of the two files this lane's structural fix covers, and because it is the concrete answer to "is the append point really a problem".

**Owner**: Lane S5-0 (MR 0a, alongside the `.PHONY` split). Found and fixed 2026-08-09.

### Suspected, not proved

- **S1 — MEDIUM-LOW.** `dispatcher.go:225-230` claims `PublishTimeout < LeaseDuration` means a slow destination cannot outlive its lease. `types.go:107-115` enforces only that inequality, while real elapsed time is `Claim → decideIdentity (DB round trip) + PublishTimeout + MarkPublished round trip`. `PublishTimeout = LeaseDuration - 1ms` is accepted by `validate()` and breaks the invariant. Defaults (10s/30s) have slack.
- **S2 — LOW-MEDIUM.** The 24-hour stream-prune floor is an exact equality across two clocks: Go's `s.now().Add(-window)` (`store.go:518`) versus `clock_timestamp() - interval '24 hours'` (`internal/integration/session/migrations/0006_retention_expiry.sql:151`). NTP skew of a few milliseconds selects rows the trigger then rejects, erroring the whole prune.
- **S3 — LOW.** `PurgeExpired` returns on the first class's error (`store.go:253-289`), so one poisoned class stops the rest for that pass. Compounds D1.
- **S4 — LOW, doc fix.** `transport.go:272-275` claims the credential is "zeroed before returning". The `[]byte` is, but `authorizationHeader` (`:456`) creates immutable `string` copies that live in `request.Header` until GC.
- **S5 — LOW.** A fresh `http.Transport` per dispatch (`transport.go:294-309`) means a TCP+TLS handshake per delivery. No FD leak (`CloseIdleConnections` is deferred), but the cost is undocumented.
- **Cosmetic.** `AGENTS.md:228,232` and `negative_control_integration_test.go:64,72,85,93` reference `0006_export_attribution_defaults.sql`; the file is `0007_…`.

### Adversarially probed and found sound

The tombstone exemption trigger is tight (setting `purged_at` without a tombstone, a tombstone without `purged_at`, and a tombstone plus any other column change were each refused live; the frozen-column list is exhaustive against `information_schema`). The purge audit row is genuinely same-transaction (`INSERT … SELECT FROM purged`, one statement) and the double-claim is safe (`FOR UPDATE SKIP LOCKED` plus a re-checked `purged_at IS NULL`). The delivery interlock is enforced **in SQL**, not only in a test (`store.go:392-405`). Unconfigured deployments purge nothing (`cmd/fi-fhir/retention_runtime.go:44-47`). Transport hardening holds: no `InsecureSkipVerify`, `MinVersion: tls.VersionTLS12`, `Proxy: nil`, redirects refused, body drain bounded at 64 KiB, served-cert advisory bounded to 256 printable-ASCII bytes matching its column CHECK, and no destination-derived value reaches a metric label. The transport gate has no privilege inversion: the three fine-grained buckets match `internal/integration/operator/service.go:71-92,235,299` exactly, `operatorRead` appears only on queries, `integration:preview` and the session-stream context rule are intact.

**One gate gap worth naming**: nothing logs, warns, or meters *per-request* use of `graphql:operator`. The only visibility is a one-line startup banner (`cmd/fi-fhir/main.go:5220`), and 115 of the 131 root fields are reachable only through that compatibility grant. Lane S5-C should emit a structured log line per compatibility-grant admission.

---

## Current-State Corrections From Code

Adversarial toward the sprint scope description, toward `.loom/30`, toward `.loom/32`'s own forward-looking claims, and toward the merged 5.1 docs. Each item is what the code does.

### Performance harness (Lane S5-A scope)

**1. `scripts/bench-check.*` does not exist.** The gate is a Go program: `cmd/bench-check/main.go` (364 lines) plus `cmd/bench-check/suggest.go` (172 lines), with thresholds in `internal/workflow/benchmark_util.go`. Invoked as `go run ./cmd/bench-check -confirm=3 benchmark.txt` (`.gitlab-ci.yml:1262`, `Makefile:262`).

**2. Every `.gitlab-ci.yml` and `cmd/fi-fhir/main.go` citation in `.loom/32` and in the merged 5.1 docs is stale.** `.gitlab-ci.yml` is now 2738 lines; `.loom/32:72` cites `:1228` and `Makefile:203` where the real lines are `.gitlab-ci.yml:1238` and `Makefile:252-262`. The 5.1 docs cite `:1667-1678` for the trivy CRITICAL gate (real: `:1851-1852`), `:1592-1595` for trivy-fs skip-dirs (real: `:1765-1768`), `:1397-1444` for `test:phi-audit` (real: `:1419`), `:1455-1490` for `test:observability-replicas` (real: `:1477`). `cmd/fi-fhir/main.go` moved `:49→:50`, `:309→:320`. **Re-verify every line number on first rebase; do not quote this or any prior spec's CI line numbers.** `pkg/fhir/*` citations are unchanged and remain exact.

**3. `test:benchmark` benches 32 functions and asserts on six.** `.gitlab-ci.yml:1238`, package list `./internal/workflow/... ./pkg/terminology/... ./pkg/validate/...` (`:1250`, mirrored at `Makefile:253,260`). `when: manual` on MRs (`:1289-1291`), automatic on tags (`:1285`) and default branch (`:1286-1287`), `allow_failure: false` (`:1293`). Census: **29** legacy-engine benchmarks (`internal/workflow/benchmark_test.go`, 29 `func Benchmark` at `:42…:914`), **3** other (`pkg/terminology/umls_test.go:591,601`; `pkg/validate/identifiers_test.go:210`), **0** under `internal/integration`. Only six are asserted on — `workflowAllocCeilings` (`internal/workflow/benchmark_util.go:328-335`) plus the profile maps cover `BenchmarkEngineProcess`, `BenchmarkCELEvaluate_Simple`, `BenchmarkFilterMatch_EventType`, `BenchmarkTransform_SetField`, `BenchmarkThroughput_Simple`, `BenchmarkThroughput_Complex`. The other 26 cost CI minutes and are never checked. This confirms `.loom/32` correction 21 and sharpens it.

**4. The legacy/durable separation is real but not total, and the shared code has zero benchmark coverage.** The durable path never calls `workflow.Engine.Process`; the runtime `.Process(` calls (`internal/integration/ingress/service.go:171`, `mllp/service.go:232`, `batch/service.go:383`, `preview/service.go:121`) are on the integration processor. But the durable path imports `internal/workflow` for parse/plan: `internal/integration/processor/workflow_plan.go:41,45` (`ParsePublishedWorkflow`, `NewPlanner`) and `internal/integration/session/workflow_simulation.go:43,47`. Neither `NewPlanner` nor `ParsePublishedWorkflow` nor `ParseDraftWorkflow` appears in `benchmark_test.go` — `BenchmarkWorkflowParse:713` exercises the legacy `ParseWorkflow`, a different entry point.

**5. bench-check already gates three metrics, not one.** `PerformanceThresholds` (`benchmark_util.go:281-285`) and `Check` (`:538-602`) assert `MaxNsPerOp` (`:571-578`, CPU-dependent), `MaxAllocsPerOp` (`:581-588`, CPU-**independent**), and `MinThroughput` (`:591-598`, CPU-dependent). Moving to allocs-only is a real code change, and it removes the only signal for the two `Throughput_*` benchmarks, which have no alloc-only equivalent.

**6. The alloc signal's truthfulness is already documented in-tree, with numbers.** `benchmark_util.go:321-327`: allocation counts "reported a bit-identical value" across all three CPU classes over 78-87 artifacts, "the sharpest and only flake-free part of the gate." Latency uses `LatencyMarginFactor = 1.6` (`:301`) derived from backtesting 87 artifacts (`:290-300`): hardware class spread up to **5.3x**, whole-run contention p99 1.37x, single-benchmark jitter up to 1.55x; above 1.6 the backtest failure count is flat, so extra width is pure loss of sensitivity. Profile selection is `lookupCPUProfile` (`:456-473`) against three calibrated classes (`:349-412`); unknown hardware **falls back to the slowest** (`FallbackCPUProfileID = "qemu-broadwell"`, `:415`) with a warning (`cmd/bench-check/main.go:163-169`). Two noise-suppression layers exist and survive an allocs-only cut: `ReduceToBest` (`:65-103`) and `-confirm=3`, which re-runs only violating benchmarks and requires the same CPU model for the confirmation to count (`cmd/bench-check/main.go:188-249`, esp. `:233-237`).

**7. There is not one `tags:` block in `.gitlab-ci.yml`'s 2738 lines.** Runner selection is 100% default-pool. `default:` (`:112-117`) sets only `retry`. Every `-tags=` in the file is a Go build tag. Sizing is per-job Kubernetes env vars, globally `KUBERNETES_CPU_LIMIT: "1"` / `KUBERNETES_MEMORY_LIMIT: "2Gi"` (`:41-44`). So the repo-side pin is genuinely a one-line `tags:` per job **plus** a quota bump — and a tagged runner registered in `platform/gitops`, outside this repo. Two remote includes exist (`.gitlab-ci.yml:1-8`, `platform/gitops@main:/k3s/ci/caches/gitlab-ci-cache.yml` and `services/tech-radar:/ci/radar.yml`); if a default tag is injected anywhere it comes from there, and the lane must check before assuming the pool is untagged.

**8. `fi-fhir workflow loadtest` is worse than "measures the legacy engine".** `cmd/fi-fhir/main.go:1955` → `runWorkflowLoadtest` (`:2758`) builds `workflow.NewEngine(wf)` (`:2868`) and `workflow.NewLoadTester(engine)` (`:2916`); `internal/workflow/loadtest.go` imports only stdlib (`:3-11`). **Executed** against the repo's own `configs/adt-workflow.yaml`: `Events/sec: 499.98`, `Achievement: 100.0% of target`, `P99: 103.792µs` — and `Errors: Count: 1000, Rate: 100.0000%`, with 1499 × `template: msg:1: function "event" not defined` on stderr. Two shipped-artifact defects: `configs/adt-workflow.yaml:14` uses templating the engine does not implement, and `NewHealthcareEventGenerator` (`loadtest.go:586-626`) emits events that fall through every route to the catch-all. Its pass constants are hardcoded at `main.go:2931` — `result.Passed(0.90, 0.01, 10*time.Millisecond)` — unrelated to the spec's p95 ≤250 ms / p99 ≤500 ms. Reusing this harness would time the legacy engine *failing*. Confirms and strengthens `.loom/32` correction 22.

**9. 4.4a did not reconcile the chart/profile contradiction — it named it.** The `deploy/helm/fi-fhir/values.yaml` diff in `e5f8e2082` is **pure comment insertion**; `cpu: 500m` / `memory: 512Mi` (`values.yaml:188-194`) are unchanged context lines, and `deploy/kubernetes/base/deployment.yaml:135-141` is identical. `docs/operations/SUPPORTED-1.0.md:44-75` adds a subsection *"The chart defaults are not the reference profile"*, with `:52-53` reading "Slice 4.4a resolves the contradiction by naming it rather than by moving a number" and `:59` "The chart defaults stay." It adds an opt-in `deploy/helm/fi-fhir/values-reference-profile.yaml` (37 lines; `:25-31` sets 4 CPU / 8Gi requests **and** limits; `:16-20` "IT IS NOT A CAPACITY CLAIM"). `SUPPORTED-1.0.md:66-70` assigns the measurement to 4.4b and names the pinned-runner decision as its blocker, in this repo's own words.

**10. The reference profile is ambiguous per-pod versus total, and nothing renders or lints the profile file.** `SUPPORTED-1.0.md:38` says 4 vCPU / 8 GiB "available to the fi-fhir workload"; `values-reference-profile.yaml:25-31` applies 4/8Gi **per container** across 2 replicas — 8 vCPU / 16 GiB, silently double. `values-reference-profile` appears only at `SUPPORTED-1.0.md:62,65`; no CI job, Makefile target, or script renders it, so it can rot silently. The production Kustomize overlay is worse: `deploy/kubernetes/overlays/production/kustomization.yaml:14-20,23-35` patches to **3 replicas** at 1 CPU / 1Gi, breaking even the two-replica half of the profile.

**11. Budget 2 cannot be certified on the two-replica reference profile until 4.4e lands.** `docs/operations/PRODUCTION-MLLP.md:51-55` states the consequence: N replicas admit up to `N × MaxMessagesPerSecond`. A 250 msg/s steady-state run on two replicas against a revision declaring 250 admits up to 500 and is not measuring the declared policy. `.loom/32:96` flagged it; nothing has changed. Either 4.4e merges first or the harness runs single-replica and the result does not certify the profile.

### Chaos, DR, upgrade (Lane S5-B scope)

**12. Nothing configures WAL archiving or PITR — confirmed, repo-wide.** A grep across the whole tree (excluding `.tmp/`, `.worktrees/`, `node_modules`) for `archive_mode|archive_command|wal_level|restore_command|recovery_target|pgbackrest|wal-g|barman|pg_basebackup|pg_receivewal|max_wal_senders` returns **zero hits**. Not in `docker-compose.yaml`, not in `deploy/**`, not in `scripts/`, not in `.gitlab-ci.yml`. There is no backup CronJob anywhere in `deploy/`.

**13. But 4.4a already recorded the gap and assigned PITR to 4.4c, in the doc, honestly.** `docs/operations/PRODUCTION-HARDENING.md:995-1003`: "**The backup method documented above cannot meet that RPO, and no amount of scheduling will make it.** … Bounding data loss to minutes requires continuous WAL archiving and point-in-time recovery, which **no chart, manifest, or document in this repository configures today**." Table rows `:1011-1012` read "**RPO unachievable with logical dumps.** Needs WAL archiving / PITR (slice 4.4c)". `:1038-1042` lists precisely what the restore proof does not cover: "any recovery-time or data-loss measurement, WAL archiving, point-in-time recovery, failover, or a restore onto a different host." So `.loom/32` correction 27 is discharged as a *doc* item; the *capability* is this lane's whole subject.

**14. The only PostgreSQL in `deploy/` is a single-replica `Deployment` on an RWO PVC with no `strategy: Recreate`.** `deploy/kubernetes/base/postgres.yaml:19` (kind `Deployment`), PVC `:1-16` (`storageClassName: longhorn`, 10Gi, ReadWriteOnce). Any image or config change attempts a surge pod that cannot mount the volume, and the rollout wedges. This is the first thing a "rolling upgrade on Kubernetes 1.36" exercise will hit, and it is a one-line fix that must land before the exercise, not during it.

**15. Neither Deployment declares a rolling-update strategy.** `grep -rn "strategy\|maxSurge\|maxUnavailable\|RollingUpdate\|terminationGracePeriodSeconds\|preStop\|startupProbe\|minReadySeconds" deploy/` returns **nothing**. Both workloads inherit the Kubernetes default 25%/25%. A one-version rolling-upgrade proof with no declared surge/unavailable budget, no `terminationGracePeriodSeconds`, and no `preStop` is measuring the defaults, not the product.

**16. Probes are genuinely real post-4.3 — verified, not assumed.** Helm `templates/deployment.yaml:172-179` (liveness `/health`) and `:186-193` (readiness `/ready`); Kustomize base `deployment.yaml:148-155,162-169`, same values. Backed by `internal/api/graphql/server.go:83` (`ReadinessPath = "/ready"`), `:86` (`LivenessPath = "/health"`), mounted at `:200-205`. PodDisruptionBudgets exist in both paths (`templates/pdb.yaml:1-13` / `base/pdb.yaml:1-12`, `minAvailable: 1`).

**17. `grep -n "e2e" .gitlab-ci.yml` returns nothing at all.** Not "no job passes `-tags=e2e`" — the string does not appear in the file. Every `-tags=` in CI (22 occurrences) is `integration`. `.loom/32` correction 26 stands with the strongest possible evidence.

**18. `test/e2e/docker-compose.yaml` does not run `fi-fhir`, and pins PostgreSQL 15.** Six services (`postgres:15-alpine` `:10-24`, `hapiproject/hapi` `:27-39`, `bitnami/kafka:3.6` `:42-61`, `redis` `:64-72`, `jaeger` `:75-82`, `http-https-echo` `:85-88`) — and no application container. So standing up that stack still leaves `TestObservabilityEndpoints` skipping at `test/e2e/integration_test.go:345` and `:406`, because nothing listens on `localhost:8080`/`9090`. `:11`'s PostgreSQL 15 also contradicts the PG16 lock (`SUPPORTED-1.0.md:22`) and reintroduces exactly the client/server-major class of bug `scripts/pgdump-roundtrip.sh:125-139` guards against. The *root* `docker-compose.yaml:13-129` does run `fi-fhir serve` with healthchecked postgres:16 and kafka — that is the file a real e2e job should extend.

**19. `test/e2e/integration_test.go` has five tests and seven `t.Skipf` calls; one test needs no infra.** Build tag `//go:build e2e && integration` (`:8`) — both required. Skips at `:62,:70` (PostgreSQL at `localhost:5433`), `:146,:151` (HAPI FHIR), `:227` (echo server), `:345,:406` (a running `fi-fhir`). `TestWorkflowWithRetry:274` needs nothing. The sibling `test/e2e/e2e_test.go` is `//go:build e2e` only, nine tests, no external infra — so `make test-e2e` (`Makefile:67-68`) compiles only that file and `test:e2e`'s absence from CI hides only `integration_test.go`.

**20. The chaos harness this lane needs already exists, and it was built for exactly this constraint.** `internal/observability/replicas_integration_test.go:882-969`: `type tcpProxy` with `Break()` (`:945-954`, refuses new connections *and* severs live ones) and `Repair()` (`:957-961`). Its rationale is in-source at `:875-881`: "A GitLab job receives PostgreSQL as a service container and has no Docker socket, so 'stop the container' is not available. Breaking the proxy is both portable and stronger." Consumed by `checkReadinessFollowsDatabase` (`:185-222`), which asserts `/ready` goes 503 within a 15s budget **while `/health` stays 200**. Alongside it, `startReplica` (`:793-857`) builds and `exec.Command`s a real `fi-fhir serve` binary with a documented env block and kills it in `t.Cleanup` (`:846`). **A chaos lane that reaches for Docker or a cluster before exhausting these two has skipped the cheapest 80%.**

**21. Six existing fault/restart tests are reusable, and all are already blocking in CI.** `TestPostgresProductionSubmission_64WayDuplicateFaultRestart` (`internal/integration/processor/postgres_submission_integration_test.go:25`, six named pre-commit fault checkpoints at `:56-70`), `TestPostgresDeploymentLifecycle_RaceRestartImmutableRelease` (`lifecycle/postgres_integration_test.go:24`), `TestPostgresMLLPRuntime_DurableACKPauseRestart` (`mllp/server_integration_test.go:44`), `TestBatchIngestion_PostgresS3SFTPKillResumeArchive` (`batch/batch_integration_test.go:38`), the delivery lease-loss/reclaim path (`delivery/delivery_integration_test.go:178-208`), and `TestMigrationCompatibility_NegativeControls` (`migrationcompat/negative_control_integration_test.go:31`, whose `preSliceUnlockedInitialize` at `:215` replays a pre-fix algorithm verbatim — the pattern for reverting a mechanism inside a test rather than behind a build flag).

**22. Nothing anywhere kills a Kubernetes pod, drains a node, or injects a cluster-level fault.** No chaos-mesh, no litmus, no `kubectl delete pod`. All existing chaos is in-process or in-test-harness. `lint:helm` (`.gitlab-ci.yml:341-353`) runs `helm lint`, `helm template … > /dev/null`, and `scripts/validate-kustomize-preview.sh` — the latter greps eight rendered assertions (`:33-55`), which is a preview/auth check, not schema validation. No kubeconform, no version-targeted API validation, no Helm test hook (`grep -rn "helm.sh/hook" deploy/` → nothing).

### Logging and tracing (Lane S5-C scope)

**23. `.loom/32` correction 30's six artifacts are all resolved, and 4.4a added the assertion — but the assertion is not wired into CI.** All six now remove the variable and leave a "NOT IMPLEMENTED" comment: `.env.example:375-380`, `docker-compose.yaml:104-108`, `deploy/kubernetes/base/deployment.yaml:121-125`, `deploy/helm/fi-fhir/templates/deployment.yaml:103-108`, `deploy/helm/fi-fhir/values.yaml:88-92`, `deploy/kubernetes/overlays/production/kustomization.yaml:58-62`, `configs/full-stack.env:74-77`, `README.md:402-409`. The new gate is `scripts/check-runtime-config.sh:218-247` ("no deployment artifact advertises unimplemented tracing"), scanning `deploy/`, `configs/`, and `docker-compose.yaml` at `:238`. But `grep -n 'check-runtime-config' .gitlab-ci.yml` returns **no matches** — it exists only at `Makefile:16,367-368`. **The gate protecting the tracing artifacts cannot block a regression today.** Wiring it in is prerequisite work for this lane.

**24. Four unlabelled tracing advertisements survive outside correction 30's list, and the gate structurally cannot catch the worst one.** `deploy/kubernetes/base/configmap.yaml:45-46` sets `tracing_enabled: true` and `tracing_sampler: 0.1` — the exact snake_case keys `pkg/config` binds (`pkg/config/config.go:188-190`) — inside the `config.yaml:` block of ConfigMap `fi-fhir-config`, mounted at `/app/config` (`base/deployment.yaml:129,180`). `check-runtime-config.sh:238` greps only the literal `FI_FHIR_TRACING_`, so the YAML form escapes entirely. Also: `README.md:67` ("**Observability**: Prometheus metrics, OpenTelemetry tracing, structured logging", contradicting `:402-409` 340 lines later), `deploy/helm/fi-fhir/templates/NOTES.txt:57-59` (a nil-false conditional that resurrects the false claim the moment the values keys return), and `docs/operations/PRODUCTION-HARDENING.md:579` (`tracingEnabled: true` in a values snippet for a key that no longer exists).

**25. OpenTelemetry is already a direct dependency, so the exporter is a version bump, not a new tree.** `go.mod:24-27`: `go.opentelemetry.io/otel v1.43.0`, `.../exporters/stdout/stdouttrace v1.39.0`, `.../otel/sdk v1.43.0`, `.../otel/trace v1.43.0`, plus indirect `auto/sdk`, `otelhttp`, `otel/metric` (`:113-115`). Consumed by `internal/workflow/tracing_otel.go` (`NewOTelTracer` at `:52`, which calls `otel.SetTracerProvider` globally at `:97`) — with **zero non-test callers**. `go.sum` already contains `otlptrace v1.19.0`, `otlptracehttp v1.19.0`, and `proto/otlp v1.0.0`, and every gRPC/protobuf transitive is already indirect (`grpc v1.82.1` `:128`, `protobuf v1.36.11` `:129`, `genproto/googleapis/api|rpc` `:126-127`, `backoff/v4` `:44`). Realistically two or three lines promoted indirect→direct plus bumping `otlptrace` to match the SDK. **This inverts the framing in `.loom/32` correction 30** — the tracing façade is not missing a dependency, it is missing a caller.

**26. A structured logger already exists, is complete, and is an orphan.** `internal/workflow/logging.go:17-32` declares a `Logger` interface (`Debug/Info/Warn/Error(ctx, msg, ...Field)` + `WithFields`); `StructuredLogger` (`:113-286`) emits JSON (`outputJSON`, `:198-211`) and **already correlates OTel identifiers**, pulling `spanCtx.TraceID()` / `SpanID()` off the context at `:169,:172` and emitting them as `trace_id` / `span_id`, with helpers at `:321,:334,:347`. And: `NewStructuredLogger` has zero non-test callers; `SetGlobalLogger`/`GetGlobalLogger` (`:301-317`) have zero callers at all; `globalLogger` is a `&NoOpLogger{}` (`:298`) nothing ever replaces; `Engine` defaults to `&NoOpLogger{}` (`internal/workflow/engine.go:121,175`). The only consumer of the interface is `internal/ingest/http.go:42,49`, and `internal/ingest` is itself dead (`git grep 'internal/ingest'` returns exactly one hit, its own subpackage import at `internal/ingest/temporal.go:10`). **This is the `diagnosticsStore` orphan shape: a built destination with no producers and no consumers.** The lane must adopt-or-retire it. Shipping `log/slog` beside it creates a second abstraction and a second lie.

**27. `internal/integration/**` logs nothing at all — and the three PHI-exposure sites are in the legacy engine, one of them on a default path.** A full sweep of `fmt.Print|fmt.Fprint|log.|println` over non-test `internal/integration/**` returns only three `fmt.Fprintf` calls whose writer is a `sha256` hasher, not a log sink (`batch/provider.go:87`, `batch/service.go:437`, `batch/sftp.go:341`). `pkg/**` non-test has zero print sites. But: **`internal/workflow/queue.go:321-322` prints the entire serialized event payload to stdout** (`"[Queue:%s] Topic: %s, Key: %s, Headers: %v, Value: %s\n"`), registered as the `log` driver (`:333`) and used as the **default when no driver name is configured** (`:313-314`) — and `log` is the only queue driver actually available. `internal/workflow/actions.go:48-51` marshals the whole event when `level: "debug"` and writes it to stderr/stdout (`:54,:56`). `internal/workflow/engine.go:489` prints an unbounded `%v` on a DLQ error. **A mechanical `fmt.Printf → slog.Info` conversion widens the leak**: it moves payload-bearing lines from ad-hoc stdout into a structured stream that aggregators index and retain. Redaction precedes conversion; it is not part of it.

**28. `docs/operations/PRODUCTION-HARDENING.md:583-598` publishes a compliance log schema that nothing emits.** `trace_id`, `span_id`, `event_type`, `source`, `action`, `duration_ms`, `status`. That JSON block is aspirational. It is either this lane's target shape or a doc correction; it must not stay as-is.

**29. Serve-path logging today is 23 `Fprintf(os.Stderr, …)` plus 30 `fmt.Print*` inside `runServe` alone** (`cmd/fi-fhir/main.go:4541-5426`), all unstructured single-line; 82 non-test `fmt.Fprintf(os.Stderr, …)` sites across `cmd/`, 70 of them in `main.go`. Stdlib `"log"` is imported by exactly three non-test files (`internal/api/graphql/server.go:9`, `internal/workflow/actions.go:10`, `pkg/eventsourcing/projection.go:6`) with five `log.Printf` call sites total. `git grep -n '"log/slog"'` over `*.go` returns **zero** — `.loom/32` correction 31 stands.

**30. There is no single correlation ID to thread; the repo declares an eight-field lineage bundle on purpose.** `pkg/integration/contracts.go:497-506` — `CorrelationIDs{TenantID, CorrelationID, TraceID, SourceMessageID, ReceiptID, EventIDs, WorkflowRunID, DeliveryAttemptIDs}`, with the comment "preserves distinct identifiers across ingress, events, traces, and delivery." In SQL: `integration_receipts` (`processor/migrations/0001_atomic_submission.sql:9`), `integration_canonical_events` (`:25`), and `integration_message_lineage` (`:41`) all carry `correlation_id NOT NULL`, but **`integration_delivery_attempts` does not** — it carries `trace_id NOT NULL` (`:62`) and joins back by FK (`:71-77`). No workflow-run table carries one. Go-side it is stringly-typed across ~14 declarations. Origins differ per ingress: HTTP reads the `X-Correlation-ID` header (`internal/integration/ingress/http.go:130`), MLLP **generates** one (`mllp/service.go:216`), batch derives it deterministically from the message identity hash (`batch/service.go:371`). `README.md:411-415` already commits to the position that correlation comes from these durable identifiers, not from spans.

**31. The observability seam is a callback field, not an interface, and three adapters already print.** `SetObserver` on `batch/service.go:79`, `delivery/dispatcher.go:73`, `mllp/service.go:77`, `session/hub.go:204`; `Observe` config fields on `retention/purger.go:65`, `session/stream.go:54`, `autoroute/sweeper.go:44`, `autoroute/notify.go:274,278`. All adapted in `cmd/fi-fhir/serve_observability.go` (`:20,38,61,80,129,144,170`), and `:132,138,147,153,160,173,178` **already write stderr/stdout** — deliberately, per the comment at `:127-128`. Those are the conversion sites. The architectural rule is recorded at `internal/observability/metrics.go:362-366`: middleware is wrapped in `runServe` so `internal/integration/*` keeps no observability dependency. **A logger must not be imported into `internal/integration/*`.** Note the callbacks receive `(Result, error)` and **no context**, so threading a logger means closing over one in the adapters, not widening eight signatures.

**32. S3-A's metrics substrate is a dedicated registry on its own eagerly-bound listener, with a three-layer label allowlist and one hand-maintained list.** `internal/observability/metrics.go:156` is `prometheus.NewRegistry()`, never the default registerer, with Go/process collectors added explicitly (`:157-160`); rationale at `:121-128`. `NewMetricsServer` (`server.go:45-91`) binds eagerly so a port conflict is a startup error not a silent background failure (`:34-37`), and unconfigured paths 404 (`:66-73`). Bounded label enforcement: `allOutcomes` (`:58-64`), `KnownOutcome` (`:67-70`), and `inc()` **coercing** an unknown outcome to `OutcomeError` rather than emitting it (`:349-360`, esp. `:357`); `SetSchemaLedgerVersion` silently drops an unknown ledger (`:281-283`); `GatheredLabelValues` (`:419-439`) enumerates every `fi_fhir_`-prefixed label value. 14 `Outcome` values (`:24-53`), 18 component constants (`:74-93`), six schema ledgers (`:99-104`). **The component list in the proof is hand-maintained in the test** — `internal/observability/observability_test.go:142-148` lists all 18 literally rather than deriving them, so any lane adding a component must edit it. `TestUnknownOutcomeIsRefusedRatherThanEmitted` (`:102-118`) feeds `Outcome("mrn-123456")` and asserts it never reaches exposition; `TestEveryLabelValueIsDrawnFromABoundedSet` (`:120-164`) carries an anti-vacuity guard at `:156-158`.

**33. `pkg/config` still parses and validates tracing and log settings that nothing reads.** Struct `config.go:188-190`, defaults `:420-421`, env binding `:609-611`, validation `:759-761` (`tracing_sampler must be between 0.0 and 1.0`). `git grep -n '\.Observability' internal/ cmd/` returns exactly two hits — `cmd/fi-fhir/main.go:4899` and a doc comment at `internal/observability/server.go:18` — and tracing `observabilityConfig` uses resolve to `main.go:5154,5157,5158`, **metrics only**. `LogLevel`/`LogFormat` (`:193-194,:612-613,:763-768`) are likewise parsed, validated, never read.

**34. The one request-scoped context seam that exists is `requestsecurity`.** `internal/api/requestsecurity/auth.go:126-128` (`WithSecurityContext`, defensive copy at `:143-146`, read at `:132-141`), installed in the GraphQL middleware chain at `internal/api/graphql/server.go:657,666`. Only two other non-test `context.WithValue` sites exist: `operation_authorization.go:77` (a bool, set at `server.go:412`) and `internal/workflow/tracing.go:243` (debug recorder). MLLP threads `ctx` but attaches nothing; the delivery dispatcher's `ctx` is plain cancellation and `TraceID` travels on the `WorkItem` struct (`dispatcher.go:319,343`). So the HTTP surface has a real hook point and the background components do not.

### MLLP capacity (Lane S5-D scope)

**35. The rate gate is `internal/integration/mllp/capacity.go`, not `internal/integration/ingress`.** `ingress` has no rate limiting of any kind — only body-size caps (`ingress/http.go:46,86,104,159,200`). Enforcement is `mllp/service.go:206` calling `s.capacity.acquire(processCtx, binding.Deployment.Capacity, binding.IntegrationRevision.Digest)`, decided in `capacityGate.acquire` (`capacity.go:35-83`), positioned after authorize and before `processor.Process`. Order: degenerate-policy guard → `ErrCapacityExceeded` (`:40-43`); queue depth `active+pending >= MaxQueued` → `ErrCapacityExceeded` (`:46-49`); rate token → `ErrRateExceeded` (`:50-53`); in-flight admit-or-block (`:54-82`).

**36. A hand-rolled continuous-refill token bucket already exists in memory, and a redeploy resets it to full.** `capacity.go:104-126`, refill rate = burst = `MaxMessagesPerSecond` (clamped `:116-118`), clock injectable (`:19,28-32`). `capacityGate` (`:17-26`) is `sync.Mutex` + `tokens float64` + `last time.Time`, one instance per `Service` (`service.go:55`, constructed `:122`). **Keyed on the revision digest, and a key change resets to a full bucket** (`:109-113`) — every redeploy hands the new revision a fresh full burst. Absence of durability: `grep "golang.org/x/time/rate" cmd/ internal/ pkg/` → 0 (indirect only, `go.mod:124`); no `rate_limit|token_bucket|capacity_counter` in any `.sql`; PostgreSQL is read-only w.r.t. capacity (`lifecycle/queries.go:157-193`). Red herring: `internal/workflow/ratelimit.go` is a real `TokenBucket` but throttles outbound Athena HTTP calls (`internal/workflow/actions_athena.go:81-111`), not MLLP.

**37. On exceed the frame gets a transient NAK and the connection stays open.** `mllp/server.go:202-205` maps `ErrCapacityExceeded`→`CAPACITY_EXCEEDED` and `ErrRateExceeded`→`RATE_EXCEEDED`, both `acknowledgementTransientError` → `AE` (application mode, `protocol.go:218`) or `CE` (commit mode, `:227`) — not `AR`/`CR`, which are reserved for permanent rejects (`server.go:198-201`). `PRODUCTION-MLLP.md:154` agrees. The rate check never blocks; in-flight saturation *does* block on `notify` (`capacity.go:59-82`), bounded by the `ProcessSeconds` timeout (`service.go:204`), which converts to `ErrRetryable` → `SUBMISSION_UNAVAILABLE`/`AE`. Separately, connection overflow is a **silent close**, not a NAK (`server.go:81,102-112`).

**38. Nothing asserts the end-to-end NAK.** `grep "RATE_EXCEEDED\|CAPACITY_EXCEEDED" internal/integration/mllp/*_test.go` → **0**. `capacity_test.go:12,51` test the gate in isolation with a frozen clock. No test proves a rate-limited frame returns `AE`/`CE` carrying `RATE_EXCEEDED` with the connection surviving. A durable bucket must not be the first thing to establish that contract.

**39. Capacity comes from the deployed revision in PostgreSQL, per frame, with no env var and no default struct.** `pkg/integration/deployment.go:74-78` (`max_in_flight`, `max_queued`, `max_messages_per_second`), embedded `:85`, validated `:104-109`, loaded per-frame at `lifecycle/queries.go:192`. So a redeploy re-tunes without restart, and there is no `DefaultCapacityPolicy()`. `MaxConnections` is separate and comes from the mounted source JSON (`mllp/source.go:120,309`). `PRODUCTION-MLLP.md:57-59`: "This is documented behavior, not a pending bug fix. A durable, per-deployment token bucket … is future work (Slice 4.4+), not shipped today."

### FHIR conformance (Lane S5-E scope)

**40. The sprint scope's premise is wrong: 5.1 is not unblocked. The `.loom/28` kill-test's stated answer is "still blocked".** `.loom/28-spec-fhir-ig-bulk-smart.md:206-212` defines the test: *"If no resource is captured — because the destination consumer delivers a canonical event rather than a FHIR resource — **5.1 is still blocked and the blocker is 4.1c-b's scope, not the validator.** Say so and stop."* Traced end to end: `delivery/dispatcher.go:162,166` builds `messageForWorkItem(*item)` then `deliverToDestination(ctx, *item, message.Value)`, so the HTTPS body **is** the Kafka command envelope, with `Event: item.EventPayload` (`:348`) sourced from `integration_canonical_events.payload_json` (`delivery/store.go:107,128`); `destination/transport.go:325` sets `Content-Type: application/json`, not `application/fhir+json`; `destination/revision.go:57,61` still restricts `TransportKind` to `kafka|https` with no FHIR class; `DestinationClass` is `production|sandbox` (`pkg/integration/contracts.go:602`), an environment class; and there are **zero `pkg/fhir` imports anywhere under `internal/integration/**`**. Timing makes this worse: 4.1c-b merged at `e77c6218b` (2026-08-09 08:57 UTC) **after** the 5.1 docs at `67b874e5` (03:55 UTC), so every 5.1 doc reads as though 4.1c-b is pending while the gate is now formally satisfied and substantively unsatisfied. **A "FHIR destination class" slice — call it 4.1c-c — does not exist and nobody has specced it.**

**41. Landmine 1 confirmed, and by execution: the shipped validator rejects the shipped mapper's own output.** `pkg/fhir/validate.go:218-219` — `case "DiagnosticReport": return map[string]bool{USCoreDiagnosticReportNoteProfile: true}` — the **only** accepted URL, where `USCoreDiagnosticReportNoteProfile = USCoreBaseURL + "us-core-diagnosticreport-note"` (`pkg/fhir/types.go:47`). `pkg/fhir/mapper.go:435` emits `Profile: []string{USCoreBaseURL + "us-core-diagnosticreport-lab"}` — a **bare literal**, never declared as a constant, which is how it fell out of the map. Ran `MapLabResult` output through the shipped CLI at default `--mode us-core --strict`: `WARNING value: meta.profile does not include an expected profile for DiagnosticReport` → `Error: fhir validation failed`, exit 1. The repo's own fixture `testdata/fhir/diagnosticreport_uscore_note.json` exits 0. **Prompt wording correction**: the validator does not accept the lab profile "only as `-note`" — it has no `-lab` entry at all. **CI structurally cannot catch it**: `pkg/fhir/validate_golden_test.go:17` uses the `-note` fixture and `testdata/fhir/` contains no lab-DR fixture, so `go test ./pkg/fhir/...` is green. The only failing input is the one the mapper actually produces.

**42. Landmine 2 confirmed.** `pkg/fhir/validate.go:171` is `if expectedSet[s] {` — a byte-exact Go map lookup against an unversioned canonical (`types.go:13`, `USCoreBaseURL`, no `|<version>`). Executed: a Patient pinned to `|9.0.0` warns; the unpinned control is clean. Range correction: the comparison is at `:171` inside `validateUSCoreProfilePresence` (`:153-177`), and `expectedProfilesForResourceType` ends at **`:237`**, not `:238`.

**43. Landmine 3 is partly wrong as stated: "55" is not in the code.** It appears in exactly three places, all prose, all sourced to the external IG: `docs/planning/FHIR-CONFORMANCE-MATRIX.md:175` and `:204`, and `.loom/worklog/2026-08-08-phase-5-1-fhir-conformance-docs-prep.md:59`. The matrix concedes it at `:207-211` ("must be re-verified against the pinned `.tgz` at implementation start") and there is no `.tgz`. The real in-code numbers: **31** profile constants (`pkg/fhir/types.go:15-56`), **0** version-pinned (all derive from the unversioned `USCoreBaseURL`), and **one is dead** (`USCoreMedicationProfile`, `:36`, zero references outside its declaration). Cite **0/31 in code**; do not cite 0/55, which mixes an unverifiable external denominator with a code numerator.

**44. Landmine 4 confirmed exactly: 6 of 24.** 24 non-Bundle resource types are produced — `resourceType` is injected by 25 custom `MarshalJSON` methods in `types.go`, minus `Bundle`, across 26 exported `Map*` entry points (`mapper.go:120,220,319,392,844,913,1287,1598,1925,2352,2564,2799,3111,3630,3981,4300,4668,4976,5377,5812,6082,6380,6532,6650,6813,6980`). Required-element checks at `validate.go:114-146` cover six: Patient (`:116-119`), Encounter (`:122-124`), Observation (`:127-129`), DiagnosticReport (`:132-134`), Condition (`:137`, one element), Coverage (`:140-142`), with an explicit no-op `default:` at `:144-146`. 21 have profile-presence checks (`:179-237`). **Three have neither**: `Claim`, `ExplanationOfBenefit`, `CoverageEligibilityResponse` — and `MapCoverageEligibilityResponse` never sets `Meta` (`mapper.go:1930-1935`) while `MapClaim` sets it only when `use == "preauthorization"` (`:1298-1313`).

**45. A fifth landmine nobody filed: the validator fails open on mode, case-sensitively.** `pkg/fhir/validate.go:109-111` — `if opts.Mode != "us-core" { return nil }` — disables required-element **and** profile-presence checks for any other string, and the CLI accepts `--mode` unvalidated (`cmd/fi-fhir/main.go:266`). Executed on the same failing DiagnosticReport: `mode="none"` clean, `mode=""` clean, and **`--mode US-Core` prints "FHIR validation passed" and exits 0**. A typo in a deployment's flag silently turns conformance checking off.

**46. A sixth: an ERROR-severity mapper/validator disagreement on Patient.** `mapper.go:133` maps identifiers only from `p.Identifiers`; the documented `Patient.MRN` convenience field (`pkg/events/events.go:396-398`) is dropped. Executed: MRN-only input → `Patient.Identifier len=0` → `[error/required] Patient.identifier is required (US Core)`; with `Identifiers` populated, clean. Latent rather than live — `internal/parser/hl7v2/parser.go`, `internal/parser/csv/parser.go`, and `internal/parser/edi/mapper.go` all populate `Identifiers` — but it is a **hard error**, unlike the DiagnosticReport warning, and it is a trap for any new producer. Separately, `MapLabResult` emits a duplicated LOINC coding in `code.coding` (verified in literal output) — cosmetic today, a real finding under any structural validator.

**47. The validator decision is PROPOSED and explicitly not in force.** `.loom/40-decisions.md:1428` — "### 2026-08-08: FHIR conformance validation strategy for Slice 5.1 (PROPOSED — not ratified)", with `:1430-1433` stating "**Status: proposed.** … It is **not** in force. It changes the CI image set and possibly the shipped image, so it requires human or next-sprint ratification before any lane acts on it." Recommendations (all open): `:1436-1437` Option C then Option A, never Option A in the shipped image; `:1446-1450` `validator_cli.jar` as a CI-only service; `:1451` "The shipped image stays distroless static."; `:1457-1461` mandatory offline `.tgz` pinning; `:1462-1468` where vendored artifacts sit relative to the trivy skip list. Left open as work items at `:1559-1573`, including "A profile-version assertion policy must be chosen — bare canonical or `|9.0.0` — and the mapper and checker must agree. Today they cannot, because the checker has no version concept," and `:1572-1573` "Ratification unblocks nothing in Sprint 4. Slice 5.1 remains blocked on Slice 4.1c-b regardless of which engine wins."

**48. No Java, no IG package, no FHIR dependency exists — only prose.** `git ls-files | grep '\.tgz$'` → nothing; no `ig.ini`, no `sushi-config`; no `java|openjdk|jre|jdk` in `.gitlab-ci.yml`, `Dockerfile`, or `Makefile`; `go.mod` has no FHIR dependency. `validator_cli.jar` and `hl7.fhir.us.core` appear only in `docs/planning/FHIR-PROFILES.md:541-542` (a suggested command still pinned to the stale 6.1.0), `FHIR-CONFORMANCE-MATRIX.md:174,226,259,264`, `.loom/28:106,157,204`, `.loom/32:110,487,493`, and `.loom/40-decisions.md:1446,1457-1458,1467,1495,1519`.

**49. The image and the blocking gate are exactly as the decision assumes, which is why the confinement half is safe to ratify.** `Dockerfile:27` — `FROM gcr.io/distroless/static-debian12:nonroot`; `:58` — `USER nonroot:nonroot`. No shell, no package manager, no JRE. `security:trivy-image` (`.gitlab-ci.yml:1840-1866`) scans the built image tarball with `--exit-code 1 --severity CRITICAL` (`:1851`) and `--exit-code 1 --severity HIGH --ignore-unfixed` (`:1852`), no `allow_failure`, on MR, default branch, and tags. A JRE in that image puts a continuously moving CVE surface behind a blocking gate — and per this repo's own history, a green main does not imply a green MR for trivy, because the database moves daily.

**50. There is a second, undocumented FHIR surface on the ingest side.** `internal/fhir/subscription/` (7 files; `mapper.go:30` — "FHIRMapper converts FHIR resources to canonical events") is an **inbound** FHIR→canonical-event mapper. It does not import `pkg/fhir` and does no validation. It is imported by `cmd/fi-fhir/main.go:28` and two GraphQL resolvers. The 5.1 docs never mention it — a conformance scope gap in the opposite direction from the one they analyse.

**51. `pkg/fhir` is reachable from exactly two non-test files, confirming `.loom/32` correction 36 with one citation drifted.** `git grep -n 'pkg/fhir' -- '*.go' | grep -v _test` → `cmd/fi-fhir/main.go:50` and `internal/workflow/actions.go:23`. The `fhirAction` is at `internal/workflow/actions.go:670` and the mapper is constructed at `:680`.

**52. `docs/planning/FHIR-PROFILES.md` self-contradicts on the exact row at issue.** `:28` claims `DiagnosticReport | us-core-diagnosticreport-note | ✅ | mapper.go:MapLabResult()` — but `MapLabResult` stamps `-lab`. MR !155 added an accuracy banner (`:5-20`) covering the 6.1.0 staleness and the `Meta.Profile` claim but **not** this row; the body still reads 6.1.0 at `:80,:445,:542`.

### Purge role separation (Lane S5-F scope)

**53. The follow-up is filed in two places with matching wording, and named.** `docs/operations/PHI-RETENTION.md:293` — "**Not implemented — named follow-up slice.** Every migration runs on the same connection the runtime uses, so the application role owns the tables it guards and can drop any trigger. The schema-enforced exemption is a guard against programmatic error, not against a hostile database role. Real separation needs a de-privileged application role, a separate migration runner, and a purge role". Same in `.loom/40-decisions.md:1631-1632` ("becomes a named follow-up slice, 'purge role separation', in the Sprint 5 …"), `:1659`, `:1667`.

**54. The premise holds: migration runner and application role are one connection.** Every migrator executes on the runtime `*sql.DB`. So `.loom/32` correction 16 is still true and the slice is real — but it is a **deployment and role-topology** slice, and its hardest part is not SQL. `internal/integration/retention/store.go` and the six migrators all assume table ownership; splitting the roles changes who runs migrations at startup, which is `runServe`'s current behaviour for six ledgers.

**55. Unconfigured deployments purge nothing — verified.** `cmd/fi-fhir/retention_runtime.go:44-47`: no `FI_FHIR_RETENTION_POLICY_PATH` → `return nil, nil, nil`, no component, no policy record. `:26-38` documents the fail-closed posture. Env surface: `FI_FHIR_RETENTION_POLICY_PATH`, `_PURGE_INTERVAL`, `_PURGE_BATCH_SIZE` (`:19-21`), documented at `.env.example:75,96-99`.

### CI, Makefile, process

**56. `.gitlab-ci.yml` is 2738 lines carrying 55 job definitions in one file, and Sprint 4's lanes produced non-append churn.** `e5f8e2082` changed CI/Makefile by +56/−40; `e77c6218b` by +31/−14; only `895b97412` was pure append (+57/−0). And `test:destination-transport` landed at **`:2672`** — after `radar-scan` and after the mirror stage, outside the test-stage block where all 20 other `test:` jobs live (`:441…:1656`). That is a rebase artifact, and it is the visible symptom the ownership-map rule "append at the end of the test stage with distinct names" was meant to prevent. **Corrected 2026-08-09 by Lane S5-0's day-1 gate:** **55**, not 59. The file has 59 top-level keys; four of them — `include`, `stages`, `variables`, `default` — are not jobs. `scripts/ci-job-inventory.sh` on `852d7f3ee` emits 55 lines, and every `.loom/33` reference to "59 jobs" has been corrected to match. The misplacement claim survives unchanged: the other 24 `test:` jobs are declared between `:441` and `:1656`; `test:destination-transport` is at `:2672`.

**57. YAML anchors are file-scoped in GitLab CI, so a naive `include: local:` split breaks every job it moves.** The file has **72** `<<: *anchor` merges and **59** uses of `*go-changes`, and **zero** `!reference` tags with exactly **one** `extends:` (`:2651`, `extends: .radar-scan`, itself from a remote include). `extends:` and `!reference` do cross include boundaries; YAML aliases do not. `.go-cache` (`:120-127`) and `.go-image-debian` (`:148-156`) are hidden jobs carrying job-level keywords, so they convert to `extends:` cleanly. **`.go-changes` (`:56-78`) is a bare top-level sequence, not a job map, and is reachable by neither `extends:` nor a two-element `!reference`** — it must be reshaped into a hidden job carrying a full `rules:` block (e.g. `.go-mr-rules`) before any job that uses it can move to an included file. That reshape is the load-bearing part of the structural fix, and it is a mechanical, testable change.

**58. The `Makefile` `.PHONY` block has become a lane-append log with the same conflict shape and a cheaper fix.** `Makefile:1-17`: `migration-compatibility` on its own line (`:13`), `phi-retention-purge` on its own line (`:14`), `transport-gate transport-gate-negative-control` on its own (`:15`), and `destination-transport` tacked onto the end of the `dev` line (`:17`). Proof targets are appended in a single block (`:150-220`).

**59. `errCh` has one slot of headroom against nine senders.** `cmd/fi-fhir/main.go:5238` — `errCh := make(chan componentError, 10)` — with nine component goroutines: GraphQL (`:5257-5259`), metrics (`:5260-5265`), session-stream (`:5266-5271`), MLLP (`:5279-5285`), delivery (`:5286-5297`), batch (`:5298-5304`), autoroute-sweep (`:5305-5311`), autoroute-notify (`:5312-5319`), retention-purge (`:5320-5325`). Two Sprint-5 lanes each appending a component makes 11 senders against capacity 10, and `waitForBackgroundStops` returns early on the first non-nil error (`:5382-5384`) — a latent shutdown hang, not a compile error. Any lane adding a component also edits `waiting` (`:5335-5344`, 8 entries) and `componentMetricNames` (`:5347-5356`, 8 entries), plus the hand-maintained test list at `internal/observability/observability_test.go:142-148`. The table's ownership note is at `main.go:5232-5237`.

**60. Migration ledger heads at `852d7f3ee`, read from the constants, not from any doc.** processor **5** (`internal/integration/processor/postgres_submission.go:80`), session **7** (`internal/integration/session/postgres.go:49`), batch **3** (`internal/integration/batch/store.go:18`), destination **2** (`internal/integration/destination/postgres.go:16`), lifecycle **1**, terminology **3** (`pkg/terminology/db/schema.go:7`). Next free: processor `0006`, session `0008`, batch `0004`, destination `0003`, lifecycle `0002`, terminology `SchemaV4Migration` with `SchemaVersion = 4`. `internal/integration/retention` has **no `migrations/` directory** — its tables live in `processor/migrations/0005_retention_expiry.sql` and `session/migrations/0006_retention_expiry.sql`, so a retention schema change consumes a processor or session number. **Re-verify against `origin/main` on every rebase; the ledger is the authority.**

**61. The worklog conditional in `.loom/32:156` is resolved — always write a per-entry file.** `.loom/worklog/` exists with 38 files; `.loom/50-worklog.md` is a 43-line pointer page that says "Do not add entries here"; `scripts/worklog.sh check` runs in the blocking `lint:docs` job (`.gitlab-ci.yml:371`, rules at `:384-386,:397-399`). Create entries with `make worklog-new TITLE="…"` (`Makefile:652-654`).

---

## The Sprint's Riskiest Load-Bearing Assumption

> **"The Release Candidate gate is a measurement problem. Phase 4's contracts are proven and merged, so Sprint 5 instruments, measures, and certifies."**

That is the framing of the sprint scope, of `.loom/30:887-946`, and of the release-gate map's "Release Candidate: 4.1-4.4". It is load-bearing for every lane: S5-A builds a harness to measure a system it assumes works, S5-B certifies recovery of a system it assumes is consistent, S5-C instruments a system it assumes is not leaking, S5-E certifies conformance of a path it assumes exists.

**It is already dead, and the evidence is in this document.** D1: the shipped purge drains 200 records per class per hour and nothing catches up — a PHI retention control that cannot honour any policy on a busy tenant. D2: a slow-but-successful HTTPS destination produces zero provenance rows and exits the delivery worker, so the attempt is redelivered to a destination that already accepted it — a duplicate-delivery generator inside the P0 definition at `.loom/20…:284-286`. Both merged in the last two days. Both passed their lanes' own kill-tests. Neither is measurable by any budget in the spec.

### Kill-test: `TestReleaseCandidate_MergedContractsHoldUnderSustainedLoad`

A single integration test, in a new `internal/integration/releasegate` package, run on unmodified `main` **before any lane writes production code**. It asserts only *documented* contracts — no new budgets, no wall-clock, no reference profile. PostgreSQL 16 plus the in-test TLS server pattern from `destination/transport_test.go`:

1. Seed **500** expired canonical events under a configured retention policy. Run `Purger.Run` for three simulated ticks with an injected clock. Assert every seeded record is tombstoned and audited. **On `main` this fails at 200 per tick** (D1).
2. Deliver one attempt to an `https` destination that returns 200 after `PublishTimeout + 1s`. Assert exactly one row in the provenance table and that `Dispatcher.Run` has not returned. **On `main` this fails with zero rows and a returned error** (D2).
3. `pg_dump`/restore a database populated with the 4.1e tables, drop every trigger on the restored copy, and re-run 4.4a's six immutability mutations. Assert all six raise. **On `main` three still raise via foreign keys with the triggers gone** (D3) — so assert the *reason*: each refusal's SQLSTATE must be `P0001` (raise_exception), not `23503`.
4. Feed `ALTER TABLE t ADD COLUMN c TEXT NOT NULL;` to `assertTightenedColumnsHaveDefaults`. Assert it is flagged. **On `main` it is silently skipped** (D4).

**Expected outcome on unmodified `main`: FAIL, four times, for four named reasons.** Already reproduced by execution for all four; the test's job is to make the reproduction repeatable and to become the negative control once S5-0, S5-B, and S5-F land.

**If it passes** — for any assertion — this document is wrong about that defect and the owning lane corrects this file before writing code. That is the rule that produced Sprint 4's corrections 11, 23, and 36, and it is the only reason those are in a plan rather than in a post-mortem.

**What surviving the kill-test changes:** Sprint 5's opening move is repair, not instrumentation. Lane S5-0 exists because of it, it merges first, and no lane may treat a Sprint 4 contract as proven merely because it merged with a green pipeline.

---

## Parallelization Map

| Lane | Slice | Can start day 1? | Parallel with | Primary risk |
|---|---|---|---|---|
| **S5-0** | Merge surface + release-blocker repair (CI include split, D2, D4) | Yes | all | Treating the CI split as cosmetic and doing it after two lanes have appended |
| **S5-A** | 4.4b — performance budget harness | **Gated**: needs the pinned-runner decision | B, C, D, E, F | Building a truthful-looking wall-clock gate on a 5.3x pool |
| **S5-B** | 4.4c — chaos, DR/PITR, K8s 1.36 upgrade/rollback, first real e2e job | Yes | all | Reaching for a cluster before exhausting the in-test TCP proxy |
| **S5-C** | 4.4d — `log/slog` structured logging, then the OTel exporter | Yes | all | Mechanically converting `fmt.Printf` and indexing a PHI payload |
| **S5-D** | 4.4e — durable per-deployment MLLP token bucket | Yes | all | Building durability before the in-memory NAK contract has any test |
| **S5-E** | 5.1 — FHIR R4 / US Core conformance | **Docs+repair yes; certification blocked** | all | Certifying a path that does not exist (correction 40) |
| **S5-F** | Purge role separation (+ D1) | Yes | all | Writing GRANTs before deciding who runs migrations |

**Deferred out of Sprint 5, with reasons:**
- **4.1c-c — a FHIR destination class.** Correction 40 makes it 5.1's real prerequisite. Not specced by anyone. Needs a coordinator decision before it can be sized (see Decisions Required).
- **Wall-clock certification of budgets 1-3 in shared CI.** Not deferred by preference — it is not truthfully possible (corrections 6, 7). S5-A ships the harness and the archived-report contract; certification waits on runner infrastructure in `platform/gitops`.
- **Budget 7 live golden-journey evidence on Kubernetes 1.36.** S5-B ships render/install/upgrade/rollback provability and the deployment-artifact fixes; live golden-journey evidence needs a cluster and is the last RC item.

---

## Schema Freeze Status Per Ledger

The schema was **frozen** for Sprint 4. Sprint 5 unfreezes exactly two ledgers. Heads and next-free numbers are correction 60; **re-verify against `origin/main` at rebase, never against this table**.

| Ledger | Head | Next free | Sprint 5 status | Owner |
|---|---:|---|---|---|
| `integration_lifecycle_schema_migrations` | 1 | `0002` | **UNFROZEN** | **S5-D only** — the durable token bucket is deployment-scoped state, and capacity is already read from the deployed revision (`lifecycle/queries.go:192`) |
| `integration_submission_schema_migrations` (processor) | 5 | `0006` | **UNFROZEN** | **S5-F only** — role GRANTs/REVOKEs over the retention tables, which live in `processor/migrations/0005_retention_expiry.sql` |
| `integration_session_schema_migrations` | 7 | `0008` | **FROZEN** | If S5-F's role split must reach session-owned retention state (`session/migrations/0006_retention_expiry.sql`), it takes `0008` and says so in the worklog before writing it |
| `integration_batch_schema_migrations` | 3 | `0004` | **FROZEN** | nobody |
| `integration_destination_schema_migrations` | 2 | `0003` | **FROZEN** | nobody. D2's fix is a context lifetime, not a schema change |
| `terminology.schema_version` | 3 | `SchemaV4Migration`, `SchemaVersion = 4` | **FROZEN** | nobody |

**GraphQL schema — `internal/api/graphql/schema.graphql` + `generated.go` + `model/models_gen.go` + `ui/src/lib/gen/graphql.ts` — stays FROZEN for Sprint 5.** No lane above needs a root field: S5-A's reports are CI artifacts, S5-B's evidence is CI artifacts and docs, S5-C's logs are stdout, S5-D's capacity is server-owned deployment config, S5-F is roles, S5-E is a validator. If that turns out wrong, the lock goes to **S5-D** (a backpressure/capacity-status query is the only plausible need) and every other lane waits. Note that `lint:gqlgen` is not `allow_failure` and its cold-`GOMODCACHE` `go run gqlgen generate` takes 16-24 minutes and looks hung while progressing — spending the lock is expensive in wall-clock, not just in coordination.

### Migration rules that bind every lane

1. **The ledger on `origin/main` at commit time is the numbering authority** — not this document, not a worklog claim. Sprint 4 proved this twice: Lane S4-C took session `0006` unannounced, and S4-B's map entry had to be corrected mid-sprint.
2. **A `NOT NULL` column on an existing table carries a `DEFAULT`** (`AGENTS.md:210`). This is currently enforced only for the `ALTER COLUMN … SET NOT NULL` form; **S5-0 MR 0c repairs the `ADD COLUMN … NOT NULL` hole (D4) and both S5-D and S5-F must rebase onto that repair before authoring a migration.**
3. **New CHECK constraints land `NOT VALID`** — the 4.1b3 idiom (`destination/migrations/0001_delivery_identity.sql:38-58`), and 4.4a's restore proof asserts the provenance CHECK is still `NOT VALID` after a round-trip (`compatibility_integration_test.go:325-342`).
4. **Declared-vs-applied drift guards exist per package** via the `SchemaVersion` constants; a new migration bumps the constant in the same commit or `assertEveryLedgerAtDeclaredVersion` (`compatibility_integration_test.go:149-182`) fails.
5. **No backfill that invents attribution.** A pre-slice row has no policy; inventing one is retroactive vouching.

---

## File-Ownership Map

| File / area | Lanes | Rule |
|---|---|---|
| `.gitlab-ci.yml` | **S5-0 owns the structure; everyone owns one include file** | S5-0 lands the `include: local:` split first (see Lane S5-0). After it merges, a lane's job lives in **its own file** under `ci/` and the root file gains **one line**. Before it merges, no lane appends. This is the structural fix for correction 56. |
| `ci/_shared.yml` (new, S5-0) | **S5-0 only** | Hidden jobs `.go-image-debian`, `.go-cache`, `.go-mr-rules`, `.integration-proof` reachable via `extends:`/`!reference` across include boundaries (correction 57). No lane edits it; a lane that needs a new shared shape asks S5-0's owner. |
| `Makefile` | all | Distinct proof targets appended after `phi-retention-purge` (`:150-220`), and **one `.PHONY` line per lane** — do not extend another lane's line (correction 58). |
| `cmd/fi-fhir/main.go` `runServe` component table (`:5232-5356`) | **S5-D only** | S5-D appends the token-bucket component **and bumps `errCh` at `:5238` from 10 to 12** so a later lane is not the one that discovers correction 59. Also `waiting` (`:5335-5344`), `componentMetricNames` (`:5347-5356`), the `markComponent` not-configured list (`:5248-5255`). No other lane appends a component. S5-C edits `runServe`'s *print statements*, which is a different edit in the same file — see the conflict note below. |
| `cmd/fi-fhir/main.go` print/`Fprintf` sites (~82 in `cmd/`, 53 inside `runServe`) | **S5-C only** | The widest textual footprint in the sprint. S5-C merges before S5-D for this reason (see Merge Order). |
| `cmd/fi-fhir/serve_observability.go` | **S5-C only** | The seven observer adapters and the three that already print (`:132,138,147,153,160,173,178`). S5-D adds its `Observe` adapter here **after** S5-C merges, in S5-C's new shape. |
| `internal/observability/metrics.go` component constants (`:74-93`), `Outcome` set (`:24-53`), ledger set (`:99-104`) | **S5-D, S5-F** | Both append at the end of the block; do not re-sort. **Both must also extend the hand-maintained component list in `internal/observability/observability_test.go:142-148`** (correction 32) in the same commit. S5-F adds a backlog **gauge**, which is a new metric family, not a new label value. |
| `internal/integration/retention/**` | **S5-F only** | D1's drain loop, the backlog gauge, and any role-aware store change. |
| `internal/integration/processor/migrations/0006_*.sql` | **S5-F only** | Re-verify the number at rebase (correction 60). |
| `internal/integration/mllp/**`, `internal/integration/lifecycle/migrations/0002_*.sql`, `pkg/integration/deployment.go` | **S5-D only** | The capacity gate, its durable backing, and the policy struct. |
| `internal/integration/destination/transport.go` | **S5-0 only** (MR 0b) | D2 is a ten-line context-lifetime change plus a test. No other lane touches this file; a lane that thinks it needs to is re-scoping. |
| `internal/integration/migrationcompat/**` | **S5-0** (MR 0c, `migration_rule_test.go` only) **and S5-B** (the restore proof, D3) | Disjoint files inside one package. S5-0 merges first, so S5-B rebases onto a settled `migration_rule_test.go`. |
| `internal/workflow/logging.go`, `queue.go:313-333`, `actions.go:38-56`, `internal/ingest/**` | **S5-C only** | The adopt-or-retire decision (correction 26) and the two PHI print sites (correction 27). Retiring `internal/ingest` is inside this lane's scope if the decision goes that way. |
| `internal/workflow/benchmark_util.go`, `cmd/bench-check/**`, `internal/workflow/benchmark_test.go` | **S5-A only** | The gate's thresholds and profiles. |
| `internal/workflow/loadtest.go`, `cmd/fi-fhir/main.go:2758-2940` (`workflow loadtest`) | **S5-A only** | Correction 8 makes this a delete-or-relabel decision, not a reuse. |
| `pkg/fhir/**`, `pkg/validate/**`, `testdata/fhir/**` | **S5-E only** | Note `pkg/validate/...` sits in the `test:benchmark` package set S5-A may narrow — coordinate the package list, not the files. |
| `internal/fhir/subscription/**` | **S5-E only** | Correction 50; in scope for a scoping statement, not necessarily for code. |
| `test/e2e/**`, `test/e2e/docker-compose.yaml`, `docker-compose.yaml`, `deploy/**`, `scripts/pgdump-roundtrip.sh` | **S5-B only** | Probes, strategies, the Postgres `Recreate` fix, PITR artifacts, the e2e job. |
| `scripts/check-runtime-config.sh` | **S5-C** (the tracing assertion, correction 24) **and S5-B** (wiring it into CI, correction 23) | Distinct concerns; S5-C merges first, S5-B adds the job. |
| `deploy/helm/fi-fhir/values-reference-profile.yaml` | **S5-A** (the per-pod/total resolution) **and S5-B** (rendering it in CI so it cannot rot) | S5-A decides the number, S5-B enforces that it renders. |
| `internal/api/graphql/operation_authorization.go` | **S5-C only**, and only to add the compatibility-grant log line (Found Defects, gate gap). No role remapping this sprint. |
| `docs/operations/*` | all, distinct files | S5-A: `SUPPORTED-1.0.md` (profile ambiguity). S5-B: `PRODUCTION-HARDENING.md` (PITR, recovery), `SUPPORTED-1.0.md` deployment rows. S5-C: `PRODUCTION-HARDENING.md:583-598` (the unemitted log schema) — **coordinate with S5-B, same file, different sections**. S5-D: `PRODUCTION-MLLP.md`. S5-E: `docs/planning/FHIR-*.md`, `.loom/28`. S5-F: `PHI-RETENTION.md`. |
| `.loom/40-decisions.md` | all | Append dated entries. S5-E's ratification **amends** the existing PROPOSED entry at `:1428` rather than adding a second. |
| `.loom/worklog/` | all | One file per entry via `make worklog-new TITLE="…"`. Never append `.loom/50-worklog.md` — `scripts/worklog.sh check` gates the blocking `lint:docs` job (correction 61). |
| `AGENTS.md` | **S5-0 only** | The D4 rule repair and the stale `0006_export_attribution_defaults.sql` references at `:228,232`. |

---

## Coordination Rules

- One branch/worktree per lane under the repo's own `.worktrees/`: `ci/sprint5-merge-surface`, `feat/phase4-slice-4-4b-performance-harness`, `feat/phase4-slice-4-4c-chaos-dr`, `feat/phase4-slice-4-4d-structured-logging`, `feat/phase4-slice-4-4e-mllp-token-bucket`, `feat/phase5-slice-5-1a-conformance-reconciliation`, `feat/phase4-purge-role-separation`.
- **Before the first commit**, each lane records its owned files in a worklog entry and, if it needs one, re-verifies its migration number against `origin/main` — not against this document.
- **A lane that discovers a false premise corrects this file before writing code.** Corrections 9, 23, 25, 26, 40, 43, and 57 already invert a prior document's claim; assign each doc repair to the owning lane rather than filing it.
- **No lane appends to `.gitlab-ci.yml` before S5-0 merges.** After it merges, a lane adds a file under `ci/` and exactly one `include:` line. This is the one hard sequencing rule in the sprint.
- No lane promotes an existing job to blocking, and no lane changes another job's `-list` arity. Every new required job carries the `-list | rg -x | awk 'END { if (NR != N) exit 1 }'` existence guard — integration helpers `t.Skipf` rather than `t.Fatalf` when a service is unreachable, so a renamed or deleted proof makes a job *greener* without it (`.gitlab-ci.yml:2711-2718` is the current model).
- Every new blocking job carries a negative control that must **fail**, run in the same invocation where possible. `Makefile:203-220` shows both idioms: `migration-compatibility` runs proofs and controls in one `go test`, and `transport-gate-negative-control` inverts the exit status of a build-tagged run.
- **No local Docker Desktop.** Local reproduction uses the remote context (`AGENTS.md:237-258`); CI uses service containers. A kill-test assertion that needs a Docker socket is not runnable in its own job — use the in-test TCP proxy (correction 20).
- Any new background component calls `markComponent(<name>, ComponentRunning)` so `/ready` and `fi_fhir_component_up` cannot disagree with what the process started.
- **CI gotchas that will cost a lane a day each if unknown**: `test:benchmark` is a blocking *manual* job on Go-touching MRs, so auto-merge never fires until it is played, and it parks all `security:*` jobs in `created` until then; pushing a new commit cancels an armed merge-when-pipeline-succeeds; `security:trivy-image` scans against a daily-moving database, so a green `main` does not imply a green MR; `lint:gqlgen` on a cold `GOMODCACHE` runs 16-24 minutes and looks hung while progressing — do not cancel and retry, it resets the download and it is not `allow_failure`.

---

## Merge Order

**S5-0 → S5-C → S5-D → S5-F → S5-B → S5-A**, with **S5-E** merging whenever it is ready.

### Load-bearing orderings (violating these costs correctness or rework, not just rebase time)

1. **S5-0 before everything.** Its `.gitlab-ci.yml` split changes the append point every other lane uses; merging it after even one lane means that lane's job moves and its rules are rewritten by someone who did not write them. Its D4 repair must exist before S5-D and S5-F author migrations — that is the same reasoning that made 4.4a's task 4 a prerequisite for 4.1e, and correction 60 shows the rule was written with a hole in it. And D2 is a release blocker sitting in a file no other lane owns.
2. **S5-D before S5-A.** Budget 2 cannot be certified on the two-replica reference profile until the per-deployment bucket exists (correction 11). Measuring first produces a number that certifies nothing and then has to be re-measured.
3. **S5-D and S5-F before S5-B.** Both add durable state — the token-bucket ledger and any role-scoped retention objects — and 4.4c's repaired restore proof must cover them. D3 already shows what an out-of-date `durableClasses` list costs: five 4.1e triggers are never exercised post-restore today.
4. **S5-0 before S5-B inside `internal/integration/migrationcompat`.** Both touch the package; S5-0's file is `migration_rule_test.go` and S5-B's is `compatibility_integration_test.go`, so the ordering is cheap but the reverse creates a needless conflict in a package with a 600-second CI job.

### Optimizations (cheaper this way, not incorrect the other way)

5. **S5-C third, immediately after S5-0.** It has the widest textual footprint in the sprint — 53 print sites inside `runServe` alone, plus all seven observer adapters (correction 29, 31). Every other lane touching `cmd/fi-fhir/` rebases a small diff onto it, rather than it rebasing a large diff over four merged lanes. It also gives S5-D and S5-F a structured logger to emit from at birth instead of retrofitting one, and it lands the compatibility-grant log line the transport gate is missing.
6. **S5-F before S5-B.** S5-F's D1 fix changes purge throughput, which changes how much retention state exists when S5-B dumps a database. Not load-bearing — S5-B seeds its own fixture — but it avoids a fixture rewrite.
7. **S5-A last.** Not by preference: by dependency (rule 2), by the pinned-runner decision, and because S5-B owns rendering the reference-profile values file that S5-A's numbers depend on.
8. **S5-E whenever.** `pkg/fhir/**`, `pkg/validate/**`, and `testdata/fhir/**` are touched by no other lane. The one contended surface is the `test:benchmark` package list, which includes `./pkg/validate/...` and which S5-A may narrow — a one-line coordination, not an ordering.

---

## Lane S5-0 — Merge Surface and Release-Blocker Repair

**Branch**: `ci/sprint5-merge-surface`. Three small MRs on one branch, merged in order. Small on purpose: each is reviewable in one sitting and none blocks the others' review.

### Why this is a lane and not a chore

Sprint 4 shipped five lanes into one 2738-line `.gitlab-ci.yml` and one `.PHONY` line. Two of three Go lanes produced non-append churn, and `test:destination-transport` ended up at `:2672` — outside the test-stage block, after the mirror stage (correction 56). Sprint 5 has six more lanes. The `.loom/50-worklog.md` split is the precedent: a shared append point was the repo's worst conflict source until it was replaced with one file per entry, enforced by `scripts/worklog.sh check` in a blocking job. This is the same fix for the same shape.

It also carries the two repairs nobody else owns: D2 is in `internal/integration/destination/transport.go`, which no Sprint 5 lane touches, and D4 is the migration rule two lanes are about to rely on.

### MR 0a — per-lane CI include files

**The structural constraint, verified**: YAML anchors are file-scoped in GitLab CI. The root file has 72 `<<: *anchor` merges and 59 `*go-changes` uses, zero `!reference` tags, and one `extends:` (correction 57). `extends:` and `!reference` cross include boundaries; aliases do not.

Tasks:
1. Add `ci/_shared.yml` with hidden jobs reachable across includes, and add `.go-mr-rules` carrying the **full `rules:` block** (tag / MR-with-go-changes / default-branch-with-go-changes / `when: never`). `.go-changes` (`:56-78`) is a bare top-level sequence and is reachable by neither `extends:` nor a two-element `!reference` — reshaping it into `.go-mr-rules` is the load-bearing part.

   **Corrected 2026-08-09 while implementing 0a, twice.** (a) `.go-image-debian` (`:148-156`) and `.go-cache` (`:120-127`) do **not** need moving or duplicating: they are already `.`-prefixed hidden jobs, and `extends:` is resolved after every include is merged into one document, so a job in `ci/*.yml` reaches them where they sit. Copying them into `ci/_shared.yml` would create a second definition of a shape 49 root jobs alias, which is drift waiting to happen. (b) `.go-mr-rules` belongs in `.gitlab-ci.yml` **beside `.go-changes`**, not in `ci/_shared.yml`: put it in the included file and its `changes:` list must be a second literal copy of `.go-changes`, because the alias cannot cross the boundary either. Beside the anchor there is exactly one copy. `ci/_shared.yml` therefore holds only what did not exist: `.integration-proof` and `.integration-proof-toolchain`.
2. Add `.integration-proof` to `ci/_shared.yml`: the PostgreSQL-16 service, the `apt-get` step, the `after_script` failure banner shape, and `allow_failure: false`. Every integration proof job repeats these today.
3. Move the six integration-proof jobs into `ci/` one file per proof (`ci/test-phi-audit.yml`, `ci/test-observability-replicas.yml`, `ci/test-migration-compatibility.yml`, `ci/test-phi-retention-purge.yml`, `ci/test-transport-gate.yml`, `ci/test-destination-transport.yml`), converting `<<: *anchor` → `extends:` and `rules: …` → `extends: .go-mr-rules`. **Move only these six.** Do not touch `test:unit`, `test:integration`, the lint stage, or anything in `security`/`build`/`deploy`/`release` — a bigger diff is a bigger chance of a silently dropped rule.
4. Extend the root `include:` (`:1-8`) with `- local: ci/_shared.yml` and the six files.
5. Split the `Makefile` `.PHONY` block so proof targets live on one line per lane, and add a comment naming the convention.
6. Document the convention in `AGENTS.md`: a new required proof adds `ci/test-<name>.yml` plus one `include:` line, never an append to the root file.

**Acceptance**: `helm`-style rendering is not available for CI, so the proof is behavioural — the MR pipeline must show the same six jobs, with the same names, the same `allow_failure: false`, the same `services:`, and the same rules evaluation (present on an MR touching Go, absent on a docs-only MR). Diff the job list against the pre-MR pipeline and attach it to the MR.

**Day-1 gate — must PASS on unmodified `main`: `scripts/ci-job-inventory.sh`.** A ~20-line script that parses `.gitlab-ci.yml` (plus includes, once they exist) and prints a sorted `name<TAB>stage<TAB>allow_failure` inventory. On `main` it must emit 55 jobs with `test:destination-transport` at `stage: test` and `allow_failure: false`. It proves the inventory is machine-readable *before* the split, so the split's own acceptance is a diff of two inventories rather than a reviewer's eye. Afterwards it becomes the negative control: a job silently dropped by the move turns the diff red. Wire it into `lint:docs` alongside `worklog.sh check`.

### MR 0b — D2: the provenance write gets its own budget

Tasks:
1. In `internal/integration/destination/transport.go:237-238`, derive the recorder's context from the **parent**, not from the destination-facing deadline. The delivery-facing timeout stays `PublishTimeout` (`dispatcher.go:246`); the provenance write gets a separate, smaller, independent budget that begins after the destination returns.
2. Update the comment at `transport.go:239-245`: absence of a provenance row now means a genuine provenance outage, which is what `migrations/0002_https_delivery_provenance.sql:23-26` already claims.
3. Consider — and record the answer to — whether a provenance-write failure should still return the raw error that exits the delivery worker (`dispatcher.go:260-262`), or should be a retryable `TransportError`. The current behaviour stops the whole component on one failed insert; that is defensible for a governance ledger but it is not written down anywhere.

**Day-1 gate — must FAIL on unmodified `main`, for the named reason: `TestTransportRecordsProvenanceWhenTheDestinationIsSlow`.** A destination returning 200 after `PublishTimeout + 1s`; assert exactly one provenance row and that `Dispatcher.Run` has not returned. On `main` it must fail with **zero rows and `context.DeadlineExceeded`** — not a harness error, not a skip. Land it in a **non-blocking** job with the expected failure recorded in the worklog, then the fix and the promotion to blocking land together. Reproducing before fixing is what separates this from a green test written after the code. Negative control after the fix: revert the context split behind a build tag and require the test to fail.

### MR 0c — D4: the migration rule enforces the rule it documents

Tasks:
1. Add an `addColumnNotNull` regexp to `internal/integration/migrationcompat/migration_rule_test.go` matching `ADD\s+COLUMN\s+(?:IF\s+NOT\s+EXISTS\s+)?<name>\s+<type>…NOT\s+NULL` and feed it into `assertTightenedColumnsHaveDefaults` alongside `setNotNull` (`:25`). An `ADD COLUMN … NOT NULL` without an inline `DEFAULT` is a violation.
2. Re-baseline `knownRollbackUnsafeColumns` (`:37-43`) against the real ledger heads (correction 60) and either repair the two `0002_delivery_reliability.sql` columns in S5-F's processor `0006` or re-date the exemption with the true reason. The current text names a head of 4 and a lane that no longer exists.
3. Fix the stale `0006_export_attribution_defaults.sql` references at `AGENTS.md:228,232` and `negative_control_integration_test.go:64,72,85,93`; the file is `0007_…`.
4. Restate the rule in `AGENTS.md:210` to name **both** forms explicitly.

**Day-1 gate — must FAIL on unmodified `main`: `TestMigrationRule_AddColumnNotNullWithoutDefaultIsFlagged`.** Feed the rule checker a synthetic `ALTER TABLE t ADD COLUMN c TEXT NOT NULL;` and assert it is flagged. On `main` it is silently skipped because `tightened == 0` short-circuits at `:100-102`. Already reproduced by running the test's own regexes.

### Verification

```bash
go build ./... && go vet ./...
go test -race ./internal/integration/destination/... ./internal/integration/delivery/... ./internal/integration/migrationcompat/...
make migration-compatibility          # POSTGRES_TEST_URL required
make destination-transport            # POSTGRES_TEST_URL + KAFKA_TEST_BROKERS
bash scripts/ci-job-inventory.sh > after.txt && diff before.txt after.txt   # must be empty for 0a
make check-runtime-config
```

### Riskiest Assumption

> **"The CI append point is a coordination annoyance, so splitting it is hygiene that can wait until a lane actually conflicts."**

Sprint 4 already paid for it in a way that is invisible in a green pipeline: `test:destination-transport` sits after the mirror stage, in a file section nobody reviewing "the test stage" will read, because a rebase put it there. Six lanes is more append pressure than five, and the failure mode is not a merge conflict — it is a job that lands in the wrong place, inherits the wrong rules by proximity, and is never noticed because it is green.

`scripts/ci-job-inventory.sh` kills the assumption cheaply: run it on `main` today and the misplacement is a line in a text file rather than an argument. If the inventory shows all 55 jobs already in coherent stages with coherent rules, the split is genuinely optional and this MR shrinks to the `.PHONY` fix.

---

## Lane S5-A — Slice 4.4b: Performance Budget Harness

**Branch**: `feat/phase4-slice-4-4b-performance-harness`

### The pinned-runner decision (required deliverable; the lane's gate)

`.loom/32` deferred 4.4b to Sprint 5 explicitly "blocked on a runner decision", and `docs/operations/SUPPORTED-1.0.md:66-70` repeats it in the shipped docs. The lane **must record a dated decision in `.loom/40-decisions.md` before writing a harness.**

| Option | For | Against |
|---|---|---|
| **A. Blocking `allocs/op` gates in the shared pool; wall-clock measured, archived, and non-blocking on a tagged runner that does not exist yet.** | The alloc signal is provably CPU-independent, and this repo already measured it: bit-identical across three CPU classes over 78-87 artifacts, "the sharpest and only flake-free part of the gate" (`internal/workflow/benchmark_util.go:321-327`). Extends a working mechanism to the durable path instead of inventing one. The wall-clock job, the harness, the report format, and the `tags:` value all ship in this repo; only the runner registration is external, so the infra dependency is a config change on the `platform/gitops` side rather than a code change here. | Budgets 1-3 are stated in milliseconds and messages/second, so allocs/op alone does not *certify* them — it detects regressions in the thing that causes them. Certification stays open until the runner exists, and `SUPPORTED-1.0.md` must say so. |
| **B. Calibrate a wall-clock p95 gate for the shared pool the way `test:benchmark` does.** | Precedent exists: three CPU profiles, `LatencyMarginFactor = 1.6`, fallback to the slowest class. | The 1.6x margin was backtested on *micro*-benchmarks (`benchmark_util.go:290-300`). A durable-accept path runs against PostgreSQL and Kafka **service containers in the same pod**, under `KUBERNETES_CPU_LIMIT: "1"` (`.gitlab-ci.yml:41-44`), on a pool spanning 5.3x. The variance is nowhere near 1.6x, so the gate is either permanently red or calibrated so wide it cannot detect a real regression. The repo already says this in its own CI comment (`:1257-1261`). |
| **C. Wait for the runner; ship nothing.** | No misleading numbers. | Budgets 1-3 stay unmeasured through the RC gate, and the alloc regression signal that *is* available goes unbuilt. |
| **D. Measure on a developer machine and archive the report.** | Cheap. | Unreproducible, unattributable, and not the shipped artifact. Rejected on sight. |

**Recommendation: A.** Record it, record that a wall-clock number produced in the shared pool is not evidence for any budget, and record the exact `tags:` value the future job will carry so the infra request is one sentence.

### Goal

Give the durable path its first performance measurement of any kind: allocation-count budgets that block in the shared pool, a wall-clock/throughput harness that runs on demand and archives a report, an honest statement in `SUPPORTED-1.0.md` of which budgets are certified and which are not, and the removal or relabelling of the false loadtest affordance.

### Non-Goals

- No blocking wall-clock or throughput gate in the shared CI pool, in any calibrated form.
- No Kubernetes cluster work. No chaos. Budget 4 and budget 7 are S5-B's.
- No changes to `internal/integration/**` production code. If a budget requires a code change to be measurable, that is a finding, not a task.
- No new `Benchmark*` in `internal/workflow`. The legacy engine is not the subject.
- No promotion of any of the 26 currently-unasserted benchmarks.

### Tasks

1. **Decision + doc.** The table above, in `.loom/40-decisions.md`. Then correct `SUPPORTED-1.0.md` so the per-pod-versus-total ambiguity is resolved (correction 10): state whether 4 vCPU / 8 GiB is per replica or across the deployment, and make `deploy/helm/fi-fhir/values-reference-profile.yaml:25-31` agree with the sentence at `:38`.
2. **Delete or relabel the false affordance** (correction 8). `fi-fhir workflow loadtest` reports "100.0% of target" and a 103 µs p99 while erroring on 100% of events against the repo's own shipped config. Either remove the subcommand, or rename it and print a banner naming the legacy engine as its subject. **Do not reuse it.** Record which, and why, in the same decision entry. Also fix or delete `configs/adt-workflow.yaml`'s `{{ event.type }}` templating, which the engine does not implement.
3. **First durable-path benchmarks.** A new `internal/integration/perf` package with `Benchmark*` functions driving the durable accept path — `ingress.Service.Submit` and `mllp.Service` — against a PostgreSQL service container, with destinations decoupled per the profile. Start with the two the spec's budget 1 names (authenticated MLLP and authenticated HTTP durable-accept) and the batch path budget 3 names. **These are the first benchmarks under `internal/integration` in the repo's history** (correction 3).
4. **Alloc ceilings for the durable path.** Extend `workflowAllocCeilings` (`benchmark_util.go:328-335`) — or add a sibling map so the legacy and durable sets stay distinguishable — with `MaxAllocsPerOp` for the new benchmarks, calibrated from at least three CI artifacts across different `cpu:` headers, as `-suggest` (`cmd/bench-check/suggest.go`) is built to do. Leave `MaxNsPerOp` and `MinThroughput` **unset** for the durable set, and make `Check` (`:538-602`) treat an unset threshold as "not gated" rather than zero.
5. **The pinned-runner job, inert until the runner exists.** `ci/test-performance-profile.yml`: `tags: [fi-fhir-perf]`, `when: manual`, `allow_failure: true`, `KUBERNETES_CPU_LIMIT: "4"` / `MEMORY_LIMIT: "8Gi"`, gated behind a project variable so it does not appear in the pipeline until infra is ready. It runs the wall-clock and throughput harness and archives `performance-report.json` for 12 months. **Write the report schema down** — budget name, measured value, target, the `cpu:` header, replica count, revision digest, and a `certified: false` field that only the pinned runner may set true.
6. **Memory measurement for budget 3.** `runtime.ReadMemStats` appears **nowhere** in first-party Go (`grep -rn "ReadMemStats\|MemStats\|MaxRSS" cmd/ internal/ pkg/ sdk/` → 0). Peak-RSS-above-idle for a 1-GiB batch import needs a sampler; add the smallest one that works and keep it in the perf package, not in `internal/integration/batch`.
7. **State plainly which budgets are certified.** Rewrite the `SUPPORTED-1.0.md` gate rows: budget 6 is certified by 4.4a; budgets 1, 2, 3 are *harnessed but uncertified pending a pinned runner*; budget 2 is additionally blocked on 4.4e (correction 11); budgets 4, 5, 7 are S5-B's. `.loom/30`'s Slice 4.4 section gets the same correction.
8. **Narrow `test:benchmark`'s package list if and only if the durable benchmarks live elsewhere.** `./pkg/validate/...` is in that list and is S5-E's file territory; coordinate the one-line change rather than editing S5-E's files.

### Acceptance Criteria

- At least four `Benchmark*` functions under `internal/integration/perf` exercise the durable accept path against a real PostgreSQL, and `bench-check` gates their `allocs/op` with `allow_failure: false`.
- A deliberate extra allocation on the durable accept path turns the blocking job red on any of the three calibrated CPU classes.
- No wall-clock or throughput threshold is asserted in any shared-pool job.
- `performance-report.json` is produced and archived by the pinned-runner job, and its schema is documented; the job is invisible in a normal pipeline.
- `SUPPORTED-1.0.md` states, budget by budget, what is certified, what is harnessed, and what is blocked on what — and does not contradict `deploy/helm/fi-fhir/values-reference-profile.yaml`.
- `fi-fhir workflow loadtest` either does not exist or cannot be mistaken for a durable-path measurement.
- `test:benchmark`'s existing six assertions still pass unchanged.

### Kill-Test (negative-controlled)

**Primary: `TestPerformanceHarness_DurableAcceptAllocationsAreBounded`** plus the `bench-check` gate over the new package. Assert: the harness drives the durable path (a receipt row exists per benchmark iteration, proving `internal/workflow` was not the subject); the reported `allocs/op` is identical across two runs on the same runner; and `bench-check` exits non-zero when a ceiling is exceeded.

**Negative control**: add one heap allocation per accepted message behind a build tag and require the blocking job to fail. A control that passes means the ceilings are not on the accept path.

**Day-1 gate — must PASS on unmodified `main`: `TestPerformanceHarness_NothingMeasuresAnyProductBudgetToday`.** Not a benchmark — an inventory assertion, ~30 lines. Parse `go test -list 'Benchmark.*' ./...` output and assert: **zero** benchmarks exist under `internal/integration/...`; the `test:benchmark` package list contains no `internal/integration` package; and `bench-check`'s threshold maps name exactly the six legacy benchmarks and no durable one. Passing on `main` converts "the green benchmark gate is partial credit toward 4.4" into a measured zero, in a form that inverts the moment task 3 lands.

### Riskiest Assumption

> **"A performance harness is the missing piece; the budgets are measurable once something measures them."**

Three of the seven are not measurable in this repository's CI at all (corrections 6, 7), one of those three is additionally not measurable on the reference profile until 4.4e ships (correction 11), and the affordance that looks like a harness reports 100% achievement while erroring on 100% of events (correction 8). A lane that opens by wiring `workflow loadtest` to the durable path, or by calibrating a p95 gate for the shared pool, will produce a green number that means nothing and will not discover that until someone asks what hardware it ran on.

The day-1 gate kills it by making the zero explicit and machine-checked, and the decision table forces the answer to "which pool?" before the first benchmark is written.

---

## Lane S5-B — Slice 4.4c: Chaos, DR, and Kubernetes Upgrade/Rollback

**Branch**: `feat/phase4-slice-4-4c-chaos-dr`

### The WAL/PITR posture decision (required deliverable)

`docs/operations/PRODUCTION-HARDENING.md:995-1003` already states, in the shipped docs, that the documented `pg_dump` method cannot meet RPO ≤ 5 min and that WAL archiving/PITR is configured by nothing in the repository. The lane's job is not to discover that — it is to decide what this repository ships.

| Option | For | Against |
|---|---|---|
| **A. Ship the PITR posture as documentation plus a verified restore *procedure*, and state the RPO the deployment achieves as a function of the operator's archiving choice.** | Honest and achievable inside one sprint. PITR belongs to whoever runs the database — the repo ships a `Deployment`-based PostgreSQL for dev only (`deploy/kubernetes/base/postgres.yaml:19`), and most real deployments will use a managed service or an operator. Lets the lane spend its budget on the parts only this repo can prove: that a restored database is faithful, that the app resumes from it, and that recovery *time* is bounded. | Budget 5's RPO number stays uncertified by this repository. `SUPPORTED-1.0.md:86` must say the RPO is an operator responsibility with a stated method, not a product guarantee. |
| **B. Ship a reference archiving configuration — `archive_mode`/`archive_command` to object storage, plus a restore-to-timestamp script — and prove it end to end in CI.** | Certifies budget 5 as written. | Requires turning the dev PostgreSQL `Deployment` into a real archiving setup, an object-storage service in CI, and a WAL-replay assertion. It also makes this repo the owner of an operational posture it does not otherwise own. Large, and it competes with budgets 4 and 7 for the same lane. |
| **C. Relax budget 5.** | — | Requires a dated decision amending a product-spec target (`.loom/20…:277-278`). Present it, reject it: the target is right, the *documented method* is what is wrong. |

**Recommendation: A**, with the RTO half measured and certified (that part *is* provable here), the RPO half stated as a method-dependent operator responsibility, and B filed as a named follow-up with its cost written down. Record it before touching `deploy/`.

### Goal

Make recovery, chaos, and rolling upgrade provable rather than asserted: repair the restore proof so its assertions attribute to triggers rather than to foreign keys, give the deployment artifacts the properties an upgrade exercise requires, stand up the first CI job in the repository's history that executes `./test/e2e/...`, and prove destination recovery (budget 4) under an injected fault.

### Non-Goals

- No performance measurement. Budgets 1-3 are S5-A's.
- No new durable schema. This lane reads and restores; it does not migrate.
- No chaos-mesh, litmus, or any cluster-level fault injector. Exhaust the in-test TCP proxy first (correction 20).
- No changes to `internal/integration/**` production code.
- No Kubernetes cluster in CI. The upgrade/rollback exercise is `helm`/`kustomize` render-install-upgrade-rollback provability plus documented manual evidence; live golden-journey evidence on 1.36 stays the last RC item.

### Tasks

1. **Decision + doc.** The table above, in `.loom/40-decisions.md`. Then update `PRODUCTION-HARDENING.md:990-1042` and `SUPPORTED-1.0.md:86` to match the decision rather than to restate the gap.
2. **Repair the restore proof (D3).** Three of six immutability assertions are foreign-key-shadowed. Assert on the **SQLSTATE**: a trigger refusal is `P0001`, an FK refusal is `23503`, and this proof must see `P0001` for all six. Add `assertEveryLedgerAtDeclaredVersion` to the restored database (`compatibility_integration_test.go:103-109` omits it). Extend `durableClasses` (`:216-225`) with `integration_session_samples`, `integration_session_stream_events`, and the three retention tables so the five 4.1e triggers are exercised post-restore. **Add the missing negative control**: drop one trigger on the restored copy and require the round-trip assertion to fail.
3. **Make the deployment artifacts upgradeable.** `deploy/kubernetes/base/postgres.yaml:19` is a single-replica `Deployment` on an RWO PVC with no `strategy: Recreate` — set it, or convert to a `StatefulSet` and say which and why (correction 14). Add explicit `strategy.rollingUpdate` (`maxSurge`/`maxUnavailable`), `terminationGracePeriodSeconds`, and a `preStop` to both the Helm and Kustomize app Deployments (correction 15). Reconcile `deploy/kubernetes/overlays/production/kustomization.yaml:14-20` (3 replicas at 1 CPU / 1Gi) with whatever S5-A decides the profile is.
4. **Render the reference profile in CI so it cannot rot** (correction 10). Extend `lint:helm` (`.gitlab-ci.yml:341-353`) to `helm template … -f deploy/helm/fi-fhir/values-reference-profile.yaml`, and add a kubeconform or `--validate`-style schema check targeting the pinned 1.36 API set. Today `scripts/validate-kustomize-preview.sh:33-55` greps eight rendered strings, which is a preview check, not validation.
5. **The first executing e2e job.** Build on the *root* `docker-compose.yaml:13-129`, which actually runs `fi-fhir serve` with healthchecked PostgreSQL 16 and Kafka — not `test/e2e/docker-compose.yaml`, which runs no application container and pins PostgreSQL 15 (correction 18). In CI, prefer the `startReplica` pattern (`internal/observability/replicas_integration_test.go:793-857`) — build the binary and exec it against service containers — over Compose, because the runner has no Docker socket. Fix `test/e2e/docker-compose.yaml:11` to PostgreSQL 16 regardless, and remove or repair the skips at `test/e2e/integration_test.go:345,406`.
6. **Budget 4: destination recovery under an injected fault.** Use the `tcpProxy` (`replicas_integration_test.go:882-969`) between the dispatcher and an in-test TLS destination. Queue attempts, `Break()`, assert the circuit opens and backoff is bounded, `Repair()`, assert every queued attempt resumes **without manual repair** and that `attempt_count` growth is bounded. This is the one budget of the seven that is fully provable in CI today, and every primitive it needs already exists.
7. **RTO measurement.** Time the documented restore procedure end to end in the round-trip job — dump, restore, first successful `Claim` — and archive the number. That certifies the RTO half of budget 5 (≤30 min) against the documented method, which is achievable; the RPO half is the decision above.
8. **Correct `.loom/30`.** Its 4.4c bullet list (`:932-946`) predates this lane's findings; rewrite it with the repaired-restore-proof task, the deployment-artifact fixes, and the A/B/C posture decision.

### Acceptance Criteria

- All six immutability assertions in the restore round-trip fail with `P0001`, and dropping any one trigger on the restored copy turns the job red.
- The restored database's six ledgers are asserted at their declared versions.
- `helm template` with the reference-profile values renders and schema-validates against the pinned Kubernetes minor, in a blocking job.
- Both application Deployments declare surge/unavailable, grace period, and `preStop`; the PostgreSQL Deployment cannot wedge on an RWO volume.
- A CI job executes `go test -tags=e2e,integration ./test/e2e/...` with a running `fi-fhir` and zero skips attributable to missing infrastructure.
- Queued delivery attempts survive a severed connection to their destination and resume with bounded retry growth, asserted against `integration_delivery_circuits` and `attempt_count`.
- Measured RTO for the documented restore is archived; `PRODUCTION-HARDENING.md` and `SUPPORTED-1.0.md` agree on what RPO the product claims and who owns it.
- `make migration-compatibility`, `make phi-audit`, and `make phi-retention-purge` all pass unchanged.

### Kill-Test (negative-controlled)

**Primary: `TestChaosRecovery_RestoredStateSurvivesFaultAndResumesDelivery`** — PostgreSQL 16, `-race`, the `tcpProxy`, and an in-test TLS destination:
1. Populate every durable class including the retention tables; `pg_dump`/restore; assert row-for-row equality, all six guards raising `P0001`, the provenance CHECK still `NOT VALID`, and all six ledgers at declared versions.
2. Sever the destination with `Break()`; run the dispatcher; assert the circuit opens after `CircuitFailureThreshold` and `attempt_count` growth is bounded.
3. `Repair()`; assert every queued attempt is delivered exactly once with no manual intervention.
4. Sever PostgreSQL with `Break()`; assert `/ready` goes 503 while `/health` stays 200 (the existing invariant at `replicas_integration_test.go:207-210`); repair; assert both recover.
5. Record and assert an RTO bound on the restore leg.

**Negative controls**: (a) drop one immutability trigger on the restored copy — step 1 must fail; (b) run step 3 against a build where the circuit never closes — it must fail. A control that passes means the assertion is not on the mechanism.

**Day-1 gate — must FAIL on unmodified `main`, for the named reason: `TestChaosRecovery_RestoreProofAssertionsAreTriggerAttributed`.** Step 1's guard assertions only, with SQLSTATE checking, run against a restored database on which all triggers have been dropped. On `main` **three of six still refuse, with `23503` foreign-key violations** — so the test fails by observing an FK code where it requires `P0001`. Already reproduced by execution. It converts the lane's opening task from "add PITR" into "the restore proof is half-shadowed, fix it first", and it becomes the negative control afterwards.

### Riskiest Assumption

> **"4.4a proved restore; 4.4c adds WAL archiving on top of a working proof."**

The proof is half-shadowed (D3): three of its six trigger assertions pass with every trigger dropped, its restored database's ledgers are never checked, and its `durableClasses` list omits every table 4.1e added, so five of the newest immutability triggers are never exercised after a restore. A lane that opens on PITR is building disaster recovery on a proof that would not notice the disaster it was written to catch.

The day-1 gate kills it in an hour, and its answer reorders the lane: repair the proof, extend it to the 4.1e surface, *then* decide the archiving posture.

---

## Lane S5-C — Slice 4.4d: Structured Logging then Tracing

**Branch**: `feat/phase4-slice-4-4d-structured-logging`

### The adopt-or-retire decision (required deliverable; the lane's gate)

The sprint scope says "`log/slog` + OTel tracing", and `.loom/32` correction 31 says `log/slog` appears nowhere. Both are true and both are misleading, because **a complete structured logger already exists and is an orphan** (correction 26), and **OTel is already a direct dependency with a global tracer-provider installer and zero callers** (correction 25). The lane **must record a dated decision before writing a line of logging code.**

| Option | For | Against |
|---|---|---|
| **A. Adopt `log/slog` at the `cmd/` seam; retire `internal/workflow/logging.go` and the dead `internal/ingest` package.** | One abstraction. `slog` is stdlib, has `slog.Handler` for JSON, and needs no dependency. Aligns with S3-A's rule that `internal/integration/*` holds no observability dependency (`internal/observability/metrics.go:362-366`) — the logger lives in `cmd/` and reaches components through the existing `Observe` callbacks. Removes 359 lines of code nothing calls plus a dead package. | `workflow.StructuredLogger` already does OTel `trace_id`/`span_id` correlation (`logging.go:169,172`); that logic is re-implemented, not reused. Retiring a package needs its own justification in the MR. |
| **B. Adopt `internal/workflow/logging.go` as the repo's logger and wire it into `runServe`.** | Zero new abstraction; the OTel correlation is already written. | It lives in `internal/workflow`, the *legacy engine* package the durable path deliberately does not depend on. Adopting it inverts a boundary three slices have defended. Its `Field`/`F()` API is bespoke where `slog.Attr` is standard. |
| **C. Ship `log/slog` and leave `workflow.Logger` in place.** | Smallest diff. | Two logger abstractions, one of them dead, in a repo whose UX program already paid for exactly this shape (an orphan store with no producers and no consumers, retired in its own MR). Rejected. |

**Recommendation: A**, with the OTel correlation logic ported rather than re-derived, and with `internal/ingest`'s retirement in the same MR if it is genuinely unreferenced at rebase time (verify: `git grep 'internal/ingest'` currently returns exactly one hit, its own subpackage import).

### The ordering constraint that is not negotiable

**Redaction precedes conversion.** `internal/workflow/queue.go:321-322` prints the entire serialized event payload to stdout, via the `log` driver registered at `:333` and used as the **default when no driver is configured** (`:313-314`) — and `log` is the only queue driver actually available. `internal/workflow/actions.go:48-51` marshals the whole event at `level: "debug"`. Converting those to structured logging moves an ad-hoc stdout leak into a stream that aggregators index and retain. **Task 2 below is a prerequisite for task 3, not a companion.**

### Goal

Give the serve path structured, correlation-carrying, PHI-safe logs; remove the PHI print sites that a structured stream would amplify; retire the orphan logger; and then — and only then — land the OTel exporter that the deployment artifacts have been advertising and the dependency tree has been carrying.

### Non-Goals

- No logging inside `internal/integration/**`. The rule is recorded at `internal/observability/metrics.go:362-366`; the seam is `cmd/fi-fhir/serve_observability.go`.
- No new metric families, no new component constants, no serve-component additions. That is S5-D and S5-F.
- No change to the transport-gate role mapping. The one gate edit is a log line.
- No spans inside the durable path's hot loop until the log half is merged and the PHI sites are closed.
- No new GraphQL field, no admin log-level mutation. Log level is deployment config.

### Tasks

1. **Decision + doc.** The table above in `.loom/40-decisions.md`, plus the ordering constraint stated as a rule.
2. **Close the PHI print sites first.** `internal/workflow/queue.go:321-322` must not print payloads — redact to topic, key, header names, and byte length, or make the `log` driver refuse to start when a payload could contain PHI. `internal/workflow/actions.go:48-51` must not marshal whole events. `internal/workflow/engine.go:489`'s unbounded `%v` needs bounding. Ship this as its own commit with its own test so it is reviewable as security code.
3. **A logger at the `cmd/` seam.** `slog` with a JSON handler, level and format from the already-parsed-and-ignored `pkg/config` settings (`config.go:193-194,612-613,763-768`), installed in `runServe` and closed over by the seven adapters in `cmd/fi-fhir/serve_observability.go` (`:20,38,61,80,129,144,170`). The three that already print (`:132,138,147,153,160,173,178`) convert first — they are the model.
4. **Correlation, honestly.** There is no single ID (correction 30). Emit the lineage subset each stage actually holds: `correlation_id` for ingress/receipt/event/lineage stages, `trace_id` for delivery (which has no `correlation_id` column — `processor/migrations/0001_atomic_submission.sql:62`), plus `tenant_id`. Document the join in `PRODUCTION-HARDENING.md` and make `:583-598`'s published schema either true or deleted (correction 28).
5. **A PHI budget for log fields, enforced like the metric one.** S3-A's label allowlist coerces an unknown outcome to `error` rather than emitting it (`internal/observability/metrics.go:349-360`). Logs need the equivalent: a bounded set of permitted field keys, a test that fails on an unlisted key, and an assertion that a planted PHI sentinel present in a durable payload appears in no captured log line. Reuse the sentinel technique from 4.2a's kill-test.
6. **The compatibility-grant log line.** Nothing logs per-request use of `graphql:operator`, and 115 of 131 root fields are reachable only through it (Found Defects, gate gap). Emit one structured line per admission that used the compatibility grant, with the field name and the principal — no token, no roles list beyond the grant name. One edit in `internal/api/graphql/operation_authorization.go`.
7. **Wire `check-runtime-config` into CI** (correction 23). The 4.4a tracing assertion (`scripts/check-runtime-config.sh:218-247`) runs in no pipeline job. Add it to `lint:docs` or its own job, blocking.
8. **Close the four unlabelled tracing advertisements** (correction 24), the worst being `deploy/kubernetes/base/configmap.yaml:45-46` (`tracing_enabled: true` in the exact keys `pkg/config` binds, invisible to a grep for `FI_FHIR_TRACING_`). Extend the assertion to cover the snake_case YAML form. Also `README.md:67`, `templates/NOTES.txt:57-59`, `PRODUCTION-HARDENING.md:579`.
9. **Then the exporter.** OTLP over HTTP, off by default, consuming the `FI_FHIR_TRACING_*` settings `pkg/config` already parses and validates. Bump `otlptrace`/`otlptracehttp` from the `go.sum`-resident v1.19.0 to match the SDK's v1.43.0 and promote them to direct requires (correction 25). Spans at the seams only — ingress accept, workflow plan, delivery dispatch — with the same field allowlist as logs. Remove the "NOT IMPLEMENTED" labels from all nine artifacts **in the same MR** that makes them true, and delete the `tracing_enabled: false` line from the generated config template at `cmd/fi-fhir/main.go:3405-3407` or make it honest.

### Acceptance Criteria

- `git grep -n '"log/slog"' cmd/ internal/` is non-empty and `internal/workflow/logging.go` is either deleted or is the adopted logger — not both present.
- Every line `runServe` emits is JSON with `tenant_id` plus the stage-appropriate correlation identifier, at the configured level.
- A PHI sentinel proven present in a durable canonical-event payload appears in **zero** captured log lines and **zero** spans, with the queue driver set to `log`.
- An unlisted log field key fails a unit test, not a production request.
- `make check-runtime-config` runs in a blocking CI job and fails on `deploy/kubernetes/base/configmap.yaml`'s unlabelled `tracing_enabled: true`.
- With tracing disabled (the default), no OTLP connection is attempted and no span is created.
- With tracing enabled against a collector, spans carry the same identifiers the logs carry, and the nine artifacts no longer say "NOT IMPLEMENTED".
- `make observability-replicas`, `make phi-audit`, and `make transport-gate` pass unchanged; `internal/integration/**` imports no logger.

### Kill-Test (negative-controlled)

**Primary: `TestStructuredLogging_CorrelatedAndPHIFree`** — a real `fi-fhir serve` via the `startReplica` pattern (`replicas_integration_test.go:793-857`), PostgreSQL 16, the `log` queue driver deliberately configured, one submission carrying a PHI sentinel in the payload and a second sentinel in a session sample:
1. Every captured stdout/stderr line parses as JSON and carries `tenant_id`.
2. The ingress, event, and lineage lines for one submission share one `correlation_id`; the delivery line carries the matching `trace_id`; the two are joinable through the durable records.
3. Neither sentinel appears in any captured line, in any field key, or in any span attribute.
4. No log field key falls outside the allowlist.
5. A request admitted through the `graphql:operator` compatibility grant emits exactly one grant line naming the field, and a request admitted through a fine-grained role emits none.

**Negative controls**: (a) restore `queue.go`'s payload print behind a build tag — assertion 3 must fail; (b) emit an unlisted field key — assertion 4 must fail. A control that passes means the sentinel scan is vacuous, which is exactly the failure 4.2a's negative control caught.

**Day-1 gate — must PASS on unmodified `main`: `TestStructuredLogging_ServeEmitsNoStructuredLogAndTheQueueDriverPrintsPayloads`.** Two assertions against `main`: (a) every line `runServe` writes is unparseable as JSON and carries no correlation identifier; (b) with the `log` queue driver configured, a planted PHI sentinel from the event payload **appears verbatim on stdout**. Passing on `main` proves both halves of this lane's premise — there is no structured logging, and the thing that would be converted is currently leaking. It inverts task by task and becomes the negative control.

### Riskiest Assumption

> **"4.4d is two build items: add `log/slog`, then add the OTel exporter."**

Both are already half-present in ways that make "add" the wrong verb. A complete `Logger` interface with a JSON handler and OTel correlation sits at `internal/workflow/logging.go:17-286` with zero production callers, its only consumer a dead package — so "add a logger" produces a second abstraction beside an orphaned first. OTel is a *direct* dependency at `go.mod:24-27` with `NewOTelTracer` installing a global provider (`tracing_otel.go:52,97`) and no caller, and the OTLP exporter modules are already in `go.sum` — so "add the exporter" is a version bump and a call site, not a dependency decision. And the highest-risk item is neither: converting `fmt.Printf` to structured logging without first redacting `queue.go:321` turns a stdout leak into an indexed, retained PHI record.

The day-1 gate kills the assumption by demonstrating the leak on `main` in the same test that demonstrates the absence of structure — which forces the lane's task order rather than leaving it to judgement.

---

## Lane S5-D — Slice 4.4e: Durable Per-Deployment MLLP Token Bucket

**Branch**: `feat/phase4-slice-4-4e-mllp-token-bucket`

### Goal

Make `max_messages_per_second` mean what the deployment declares rather than what one replica declares, so that N replicas admit the declared rate in aggregate — and do it without turning a per-frame admission decision into a per-frame database round trip.

### Why this lane is not optional and why it merges before S5-A

`docs/operations/PRODUCTION-MLLP.md:51-59` documents the current behaviour as intentional: N replicas admit up to `N × MaxMessagesPerSecond`, and "a durable, per-deployment token bucket … is future work (Slice 4.4+), not shipped today." Budget 2 of the product spec is a **250 msg/s** steady-state figure on the **two-replica** reference profile. Measuring that today admits up to 500 and certifies nothing (correction 11). S5-A cannot certify budget 2 until this lane lands.

### The distribution decision (required deliverable; the lane's gate)

The scope word "durable" is doing a lot of work and the repo's own precedent argues both ways. `.loom/40-decisions.md:1345-1350` rejected `pg_advisory_lock` for the autoroute notifier because a lock serialises scanners without making the decision durable; `:1398-1401` accepted the sweeper's duplicate scan as benign because the `UPDATE` is idempotent. **Neither precedent applies cleanly**: rate limiting is neither idempotent nor serialisable, and it sits on the per-frame hot path.

| Option | For | Against |
|---|---|---|
| **A. Durable lease-partitioned quota.** Each replica periodically claims a share of the deployment's rate from a durable per-deployment row, refills its in-memory bucket from the claim, and returns unused quota on release. Admission stays in-memory and O(1). | Preserves the hot path — no database round trip per frame. Aggregate rate is bounded by construction. The claim/lease shape matches the delivery outbox's existing lease idiom, which the team has already built and tested. Degrades safely: a replica that cannot renew falls back to its share, never to the full rate. | Bursty under uneven load — an idle replica holds quota a busy one wants, until the claim interval expires. The claim interval becomes a tuning knob and must be documented. |
| **B. Per-frame durable counter.** Every admission increments a durable counter under a row lock. | Exact. | A database round trip on the MLLP hot path, at 250+ msg/s, under a row lock shared by every replica. Turns a rate limiter into a throughput ceiling and would itself fail budget 1. Rejected. |
| **C. Advisory-lock-serialised admission.** | — | Serialises the hot path across replicas without making anything durable — the exact shape `.loom/40-decisions.md:1345-1350` already rejected, in a hotter loop. Rejected on sight. |
| **D. Leave it per-replica; divide the declared rate by the replica count in config.** | Zero code. | The deployment does not know its replica count, autoscaling breaks it silently, and it makes a correct-looking config wrong after a scale event. Also: `capacity.go:109-113` already resets the bucket to full on every revision-digest change, so a rolling redeploy would hand each new replica a full share of an already-divided rate. |

**Recommendation: A.** Record it, record the claim interval and its default, and record explicitly that admission remains an in-memory decision so budget 1's latency is unaffected.

### Non-Goals

- No change to the `AE`/`CE` transient-NAK semantics or to the `RATE_EXCEEDED` / `CAPACITY_EXCEEDED` reason codes (`mllp/server.go:198-205`, `protocol.go:218,227`).
- No change to `MaxInFlight` or `MaxQueued` semantics — those are legitimately per-replica resource bounds.
- No env-var configuration of capacity. It stays on the deployed revision (`pkg/integration/deployment.go:74-78`), loaded per frame from PostgreSQL (`lifecycle/queries.go:192`).
- No rate limiting on the HTTP ingress. `internal/integration/ingress` has none today (correction 35) and adding it is a separate decision.
- No GraphQL surface. If a capacity-status query is genuinely needed, that spends the schema lock and every other lane waits.

### Tasks

1. **Decision + doc.** The table above in `.loom/40-decisions.md`. Rewrite `PRODUCTION-MLLP.md:42-71` from "documented behavior, not a pending bug fix" to the new contract, and keep an honest statement of what remains per-replica.
2. **Establish the NAK contract before changing it** (correction 38). Nothing asserts end-to-end that a rate-limited frame returns `AE`/`CE` carrying `RATE_EXCEEDED` with the connection surviving; `capacity_test.go:12,51` test the gate in isolation with a frozen clock. **This is task 2 because a durable bucket must not be the first code to establish that contract** — write the end-to-end test against the current in-memory gate first, watch it pass, then change the mechanism underneath it.
3. **The durable quota record.** `internal/integration/lifecycle/migrations/0002_*.sql` (ledger `integration_lifecycle_schema_migrations`, head 1 — re-verify at rebase): per-deployment quota state with a claim holder, a claim expiry, and a claimed share.

   **Correction (S5-D, 2026-08-09): the pool is keyed on the deployment, not on the deployment *and* the revision digest.** This task originally read "per-deployment, per-revision-digest quota state". That is the obvious reading — capacity is declared on the deployed revision (`pkg/integration/deployment.go:74-78`) — and it defeats an acceptance criterion three lines below it. A rolling redeploy runs two digests at once; two digest-keyed pools each admit the full declared rate, so the deployment bursts to **twice** the declared rate for the length of every rollout, which is the failure mode task 5 separately exists to close. `QuotaKey` is `(tenant_id, definition_id)`; the revision digest rides on the claim row as attribution, so an operator can still see which revision each holder serves. Pinned by `TestQuotaCoordinatorIsKeyedOnTheDeploymentNotTheRevisionDigest`. Task 5 is answered accordingly: neither "carry quota across a digest change" nor "start empty" but **stop resetting the bucket at all** — seed it once on the process's first frame and let the balance carry, clamped to the current share. Seeding once is bounded because what it seeds is this replica's *share*; refilling per digest was not. Server-owned timestamps. New CHECKs land `NOT VALID`. Any `NOT NULL` column carries a `DEFAULT` — **rebase onto S5-0 MR 0c first** so the rule's `ADD COLUMN` hole is closed before you rely on it.
4. **The claim/refill loop.** A background component in `internal/integration/mllp` (or a sibling package) that claims a share, refills the in-memory bucket, and releases on shutdown. `capacity.go:104-126`'s continuous refill stays; only its rate source changes.
5. **Fix the redeploy full-burst reset** (correction 36). `capacity.go:109-113` resets to a full bucket whenever the revision-digest key changes, so a rolling redeploy hands each new replica a fresh full burst. Under a deployment-wide quota that is a real over-admission window. Either carry quota across a digest change or make the new revision claim before admitting; decide and document.
6. **Serve wiring, and the `errCh` repair** (correction 59). Append the component after the MLLP block; **bump `errCh` at `cmd/fi-fhir/main.go:5238` from 10 to 12** so S5-F is not the lane that discovers the overflow; add entries to `markComponent`'s not-configured list (`:5248-5255`), `waiting` (`:5335-5344`), and `componentMetricNames` (`:5347-5356`); add the component constant to `internal/observability/metrics.go:74-93` **and to the hand-maintained list at `internal/observability/observability_test.go:142-148`**; add a bounded `Outcome` if the existing 14 are insufficient. Call `markComponent(name, ComponentRunning)`.
7. **Fail-closed default.** A deployment whose quota record cannot be read or claimed must fall back to a documented conservative share, never to the full declared rate and never to zero (which would black-hole a live listener). State the choice.

### Acceptance Criteria

- Two MLLP replicas against one deployment declaring `max_messages_per_second: 100` admit ≤100 msg/s in aggregate over a measured window, not ≤200.
- A rate-limited frame returns `AE` (application mode) or `CE` (commit mode) carrying `RATE_EXCEEDED`, and the TCP connection stays open — asserted end to end, which nothing does today.
- A rolling redeploy does not produce an aggregate burst above the declared rate.
- A replica that loses its claim degrades to the documented conservative share and logs it; it never admits at the full deployment rate.
- Admission remains an in-memory decision: no database round trip per frame, provable by counting queries during a burst.
- `MaxInFlight`/`MaxQueued` behaviour, the connection-overflow silent close (`server.go:81,102-112`), and `make mllp-runtime` are unchanged.
- The two-replica observability proof (`make observability-replicas`) still passes with the new component.

### Kill-Test (negative-controlled)

**Primary: `TestMLLPCapacity_DeploymentWideRateIsBoundedAcrossReplicas`** — PostgreSQL 16, `-race`, two `mllp.Service` instances against one deployment, an injected clock, and a real TCP client:
1. Declare `max_messages_per_second: 100`. Drive both replicas for a measured window. Assert total admissions ≤100 per second, with the pre-slice behaviour (≤200) explicitly asserted as *not* observed.
2. Assert every rejected frame carries `RATE_EXCEEDED` in an `AE`/`CE` acknowledgement and that the connection is still usable for the next frame.
3. Kill one replica's claim renewal; assert it degrades to the conservative share rather than to the full rate, and that the surviving replica reclaims the freed quota.
4. Change the revision digest mid-run; assert no aggregate burst above the declared rate.
5. Count database queries during a 1000-frame burst; assert it is bounded by claim renewals, not by frame count.

**Negative controls**: (a) revert the deployment-wide claim behind a build tag — assertion 1 must fail with ~200 admissions; (b) restore `capacity.go:109-113`'s unconditional full-bucket reset — assertion 4 must fail.

**Day-1 gate — must PASS on unmodified `main`: `TestMLLPCapacity_TwoReplicasAdmitTwiceTheDeclaredRateToday`.** Two `mllp.Service` instances, one deployment declaring `max_messages_per_second: 100`, an injected clock; assert aggregate admissions approach 200/s and that **no** durable table records a rate decision. Passing on `main` proves the `N ×` consequence is real rather than documented-in-theory, quantifies exactly what budget 2 would have measured, and becomes the negative control. Cheap: both replicas are in-process, no TCP required for this assertion.

### Riskiest Assumption

> **"The capacity gate is already correct per replica; 4.4e moves its state to PostgreSQL."**

Two things make that the wrong shape. The end-to-end contract the durable bucket would preserve **has no test at all** — nothing asserts a rate-limited frame produces `RATE_EXCEEDED` in an `AE`/`CE` with a surviving connection (correction 38), so a lane that changes the mechanism first has no way to know it preserved the behaviour. And "move the state to PostgreSQL" read literally is option B, a row-locked round trip on a 250 msg/s hot path, which would fail budget 1 while fixing budget 2.

The day-1 gate quantifies the problem, and task 2 — writing the end-to-end NAK test against the *existing* gate and watching it pass — is what makes the subsequent mechanism swap verifiable rather than hopeful.

---

## Lane S5-E — Slice 5.1: FHIR R4 / US Core Conformance

**Branch**: `feat/phase5-slice-5-1a-conformance-reconciliation`

### Read this before anything else: 5.1 is not unblocked

The sprint scope lists "5.1 FHIR conformance code" as the "now-unblocked 5.1 code start", on the strength of 4.1c-b having merged. **`.loom/28-spec-fhir-ig-bulk-smart.md:206-212` defines the kill-test for exactly this moment**, and its stated answer is that the slice stays blocked:

> "If no resource is captured — because the destination consumer delivers a canonical event rather than a FHIR resource — **5.1 is still blocked and the blocker is 4.1c-b's scope, not the validator.** Say so and stop."

**No resource is captured.** Traced end to end (correction 40): the HTTPS body is the Kafka command envelope carrying `integration_canonical_events.payload_json` (`delivery/dispatcher.go:162,166,348`; `delivery/store.go:107,128`), the content type is `application/json` not `application/fhir+json` (`destination/transport.go:325`), `TransportKind` remains `kafka|https` with no FHIR class (`destination/revision.go:57,61`), and there are zero `pkg/fhir` imports anywhere under `internal/integration/**`. The timing makes it worse: 4.1c-b merged five hours *after* the 5.1 docs, so every 5.1 document reads as though the gate is pending while it is now formally satisfied and substantively unsatisfied.

**Consequence, and the coordinator decision this forces:** the real prerequisite is a slice nobody has written — call it **4.1c-c, a FHIR destination class**: a `TransportKind` or destination-class value that means "this destination receives FHIR R4 resources", a mapping step from canonical event to resource on the delivery path, `application/fhir+json`, and a decision about whether the mapper that produces those resources is `pkg/fhir` (legacy-engine-only today) or something new. That is a Phase 4 delivery slice, not a Phase 5 standards slice, and it is not in this sprint's scope.

### What this lane ships instead: 5.1a, conformance reconciliation

There is a full sprint of genuinely valuable, genuinely unblocked work that does not require a live path — because **the shipped validator rejects the shipped mapper's own output**, and four more disagreements sit behind that one. This is pure Go, no new dependency, no Java, no IG package, no CI image change, and it is a prerequisite for any future certification regardless of which engine wins.

### The validator decision: ratify the confinement half, amend the ordering half

`.loom/40-decisions.md:1428` is explicitly **PROPOSED — not ratified** (`:1430-1433`), requiring "human or next-sprint ratification before any lane acts on it". Recommendation:

**RATIFY unconditionally — the confinement half.** `validator_cli.jar` is confined to CI and never enters the shipped image (`:1446-1451`). The evidence is exactly as the decision assumes: `Dockerfile:27` is `gcr.io/distroless/static-debian12:nonroot` with `USER nonroot` at `:58` — no shell, no package manager, no JRE — and `security:trivy-image` (`.gitlab-ci.yml:1840-1866`) blocks on CRITICAL (`:1851`) and HIGH-fixed (`:1852`) with no `allow_failure`, on MR, default branch, and tags. A JRE puts a continuously moving CVE surface behind a blocking gate, and in this repo a green `main` does not imply a green MR because the trivy database moves daily. Ratify the offline `.tgz` pinning requirement (`:1457-1461`) and the vendored-artifact placement relative to the trivy skip list (`:1462-1468`) with it.

**AMEND — the ordering half.** The decision recommends "Option C now (a Go structural validator over pinned `.tgz` packages) and Option A later" (`:1436-1437`). Do neither first. The repo's actual defects are not that the checker is structurally shallow — they are that **the checker and the mapper disagree** (corrections 41, 46), that the checker **fails open on any mode string that is not exactly `us-core`** (correction 45), and that CI has **no fixture for the one input the mapper actually produces** (correction 41). Building a bigger validator on top of a mapper it disagrees with certifies the disagreement at higher resolution. **Reconcile first (5.1a, this lane); then Option C; then Option A.** The decision's own open item at `:1565-1567` — "A profile-version assertion policy must be chosen … and the mapper and checker must agree. Today they cannot" — is precisely this lane's subject.

### Non-Goals

- No `validator_cli.jar`, no Java in CI, no IG `.tgz`, no new `go.mod` dependency. Those come after ratification and after reconciliation.
- No change to the shipped image.
- No FHIR destination class. That is 4.1c-c and it needs a coordinator decision first.
- No SMART, no Bulk Data. Phase 5.2/5.3 stay closed.
- No new GraphQL field.
- No change to `internal/integration/**`.

### Tasks

1. **Decision + doc.** Amend `.loom/40-decisions.md:1428` in place — ratify the confinement half, amend the ordering half, and record the 4.1c-c finding with correction 40's citation chain. Do not add a second entry.
2. **Record that the `.loom/28` kill-test has been run and its answer is "still blocked."** `.loom/28:206-212` asks for exactly that statement. Write it, with the trace, so the next agent does not re-derive it.
3. **Reconcile DiagnosticReport** (correction 41). Decide whether `MapLabResult` should emit `-lab` (and the checker accept it) or `-note`, promote the bare literal at `mapper.go:435` to a declared constant beside the other 31 in `types.go:15-56`, and **add the missing golden fixture** so `pkg/fhir/validate_golden_test.go` exercises the input the mapper actually produces. Fix the contradicting row at `docs/planning/FHIR-PROFILES.md:28`.
4. **Close the fail-open mode** (correction 45). `validate.go:109-111` returns `nil` for any `Mode != "us-core"`; the CLI accepts `--mode` unvalidated (`cmd/fi-fhir/main.go:266`). Make the CLI reject an unknown mode, and make the mode comparison case-insensitive or the accepted set explicit. Today `--mode US-Core` prints "FHIR validation passed" on a non-conformant resource.
5. **Reconcile Patient** (correction 46). `mapper.go:133` drops the documented `Patient.MRN` field (`pkg/events/events.go:396-398`), producing a hard `[error] Patient.identifier is required`. Map it, or delete the field and its documentation. Also fix the duplicated LOINC coding `MapLabResult` emits.
6. **Choose the version-assertion policy** — bare canonical or `|9.0.0` (`.loom/40-decisions.md:1565-1567`). If `|9.0.0`, `validate.go:171`'s byte-exact map lookup must gain a version-aware comparison and all 31 constants must be pinned; if bare canonical, say so in `SUPPORTED-1.0.md` and stop describing the repo as US Core 9.0.0-pinned. **Do not leave it open** — it determines whether every future conformant resource passes or fails (correction 42).
7. **Publish honest coverage numbers.** Replace the unverifiable "0/55" with the code numbers (correction 43): **0 of 31** profile constants version-pinned, one of them dead (`USCoreMedicationProfile`, `types.go:36`). Keep the 6-of-24 required-element figure — it is exact (correction 44) — and name the three types with neither check: `Claim`, `ExplanationOfBenefit`, `CoverageEligibilityResponse`.
8. **Scope the second FHIR surface** (correction 50). `internal/fhir/subscription/` is an inbound FHIR→canonical-event mapper, imported by `cmd/fi-fhir/main.go:28` and two resolvers, doing no validation, and unmentioned in every 5.1 document. State whether inbound conformance is in 5.1's scope. A statement is sufficient; code is not required this sprint.
9. **Repair the stale citations** in `docs/planning/FHIR-CONFORMANCE-MATRIX.md`, `FHIR-PROFILES.md`, and `.loom/40-decisions.md` (correction 2, 52): every `.gitlab-ci.yml` and `cmd/fi-fhir/main.go` line number in the merged 5.1 docs is wrong; `pkg/fhir/*` numbers are exact. `FHIR-PROFILES.md` still reads 6.1.0 at `:80,:445,:542`.

### Acceptance Criteria

- Feeding `NewUSCoreMapper()`'s output for every one of the 26 `Map*` entry points back through `ValidateJSON` at `--mode us-core --strict` produces **zero** issues — the mapper's own output validates.
- A golden fixture exists for every resource type the mapper produces that has a profile-presence or required-element check, and `go test ./pkg/fhir/...` exercises all of them.
- `fi-fhir fhir validate --mode US-Core` on a non-conformant resource exits non-zero.
- The version-assertion policy is recorded, implemented consistently across mapper and checker, and stated in `SUPPORTED-1.0.md`.
- No new `go.mod` dependency, no CI image change, no Dockerfile change.
- `.loom/40-decisions.md:1428` is amended in place: the confinement half is in force, the ordering half is corrected, and 4.1c-c is named as 5.1's real prerequisite.
- Every coverage number published anywhere in the repo is derivable from code.

### Kill-Test (negative-controlled)

**Primary: `TestFHIRConformance_MapperOutputValidatesUnderItsOwnChecker`** — table-driven over all 26 `Map*` entry points: map a representative event, marshal, `ValidateJSON` at `--mode us-core --strict`, assert zero issues. Plus: `--mode US-Core` on a known-bad resource exits non-zero; a `|<version>`-pinned profile is handled per the chosen policy; the `Patient.MRN`-only path produces a populated `identifier`.

**Negative control**: revert the DiagnosticReport reconciliation behind a build tag and require the table to fail on exactly that row. A control that passes means the table is not actually round-tripping the mapper's bytes.

**Day-1 gate — must FAIL on unmodified `main`, for the named reason: `TestFHIRConformance_ValidatorRejectsMapperOutputToday`.** One row of the table above: `MapLabResult` → marshal → `ValidateJSON`. On `main` it must fail with `WARNING value: meta.profile does not include an expected profile for DiagnosticReport`. **Already reproduced by execution**, both through the library and through the shipped CLI (`Error: fhir validation failed`, exit 1), while the repo's own `-note` fixture exits 0.

**Second day-1 gate — must PASS on unmodified `main`: `TestFHIRConformance_DurableEngineProducesNoFHIRResource`.** Run one production submission and one delivery `RunOnce` against an `https` destination with an in-test TLS server; capture the request. Assert the body parses as the delivery-command envelope and **not** as a FHIR resource, that the content type is `application/json`, and that no `TransportKind` or destination-class value denotes FHIR. Passing on `main` is the `.loom/28:206-212` kill-test executed rather than argued, and it is the single most important artifact this lane produces — it is what a coordinator needs to decide whether to fund 4.1c-c.

### Riskiest Assumption

> **"4.1c-b merged, so 5.1's blocker is cleared and the remaining work is integrating a validator."**

The blocker was never "does a destination get contacted" — it was "does a FHIR resource exist on a path the golden journeys execute". It does not, and the gate that was supposed to detect that was written down in `.loom/28:206-212` and never run. A lane that opens by standing up `validator_cli.jar` in CI will spend the sprint building conformance infrastructure for a payload that is a Kafka command envelope, and will certify a `pkg/fhir` package that only the legacy engine and a CLI can reach.

The second day-1 gate kills it in one test, and the first day-1 gate shows that even the unblocked half of 5.1 opens with a defect rather than an integration: the validator this repo ships rejects the mapper this repo ships.

---

## Lane S5-F — Purge Role Separation (and D1, the purge throughput ceiling)

**Branch**: `feat/phase4-purge-role-separation`

### Honest scoping: this is a small lane carrying one large repair

The follow-up as filed is genuinely small in intent. `docs/operations/PHI-RETENTION.md:293` and `.loom/40-decisions.md:1631-1632,1659,1667` both name it identically: the application role owns the tables it guards and can drop any trigger, so "the schema-enforced exemption is a guard against programmatic error, not against a hostile database role", and real separation needs "a de-privileged application role, a separate migration runner, and a purge role."

But the hardest part is not SQL — it is that **`runServe` currently runs six migrators at startup on the runtime connection** (correction 54). De-privileging the application role means something else applies migrations, which changes the deployment's startup contract, the Helm chart, the Kustomize base, and 4.4a's concurrent-replica-startup proof. That is a real slice.

And the lane inherits **D1**, which is a release blocker in the same package: the shipped purge drains 200 records per class per hour with no catch-up. **D1 is task 1.** Role separation with a purge that cannot keep up is polishing the lock on a door that does not close.

### The role-topology decision (required deliverable; the lane's gate)

| Option | For | Against |
|---|---|---|
| **A. Three roles, migrations moved out of `serve`.** A `fi_fhir_migrator` owns the tables and runs migrations from an init container / Helm hook / explicit `fi-fhir migrate` command; `fi_fhir_app` gets DML only and cannot `ALTER`/`DROP TRIGGER`; `fi_fhir_purge` additionally holds the narrow grants the tombstone path needs. | The only option that makes the guarantee true — the app role genuinely cannot drop the trigger it is guarded by. Matches the filed description word for word. `fi-fhir migrate` as an explicit command is independently useful and makes 4.4a's ledger reporting a first-class surface. | Changes the startup contract for every deployment path. 4.4a's `TestMigrationCompatibility_ConcurrentReplicaMigrationRollbackAndRestore` step 1 asserts two `serve` processes converge on a fresh database — that assertion must move to the migrator, not be deleted. Needs Helm hook + Kustomize init container + docker-compose + docs, in one MR. |
| **B. Two roles; `serve` still migrates but under an elevated connection.** A second DSN used only at startup. | Smaller. No deployment-shape change beyond one secret. | The elevated credential is present in the process for its whole lifetime. Weaker than A and harder to explain in a compliance conversation than either A or the status quo. |
| **C. Keep one role; document the limitation precisely and close the slice.** | Zero risk. Already written at `PHI-RETENTION.md:293`. | The follow-up was filed because the documentation is not the answer. Choosing C requires saying so explicitly and removing the "named follow-up slice" language, not leaving it filed forever. |

**Recommendation: A**, and if A cannot be sized inside this sprint alongside D1, ship **D1 plus C-with-an-explicit-closing-decision** and re-file A as its own slice with the deployment work costed. Do not ship a half-A that creates roles nothing enforces.

### Goal

Make the retention purge able to honour a policy on a busy tenant, and make S3-C1's immutability guarantee hold against the application role rather than merely against application code.

### Non-Goals

- No change to the tombstone semantics, the exemption trigger's logic, or the delivery interlock (`retention/store.go:392-405`). Those were adversarially probed and are sound.
- No new retention policy shape, no GraphQL policy administration.
- No change to `internal/integration/delivery/store.go` or the destination packages.
- No lifting of `ErrUnsupportedRawRetention`. Production raw stays fail-closed.

### Tasks

1. **D1: drain the backlog.** `internal/integration/retention/purger.go:142-159` calls `PurgeOnce` once per tick against a `LIMIT 200` (`store.go:33`, `:311,339,363,409,441,474,512`) on an hourly cadence (`cmd/fi-fhir/retention_runtime.go:22-23`). Adopt the shape the repo already uses one package over — `internal/integration/session/stream.go:174-179`, *"A full batch means there is more backlog; keep going rather than waiting a whole tick per batch"* — bounded by a wall-clock budget per tick so one tick cannot monopolise the connection. Also fix `PurgeExpired`'s first-error return (`store.go:253-289`, S3), which stops every remaining class for that pass.
2. **A backlog gauge.** Retention metrics are counters only (`internal/observability/metrics.go:213-217`), so "falling behind" is indistinguishable from "healthy and busy" — which is why D1 was invisible. Add a gauge for rows past `purge_after` with `purged_at IS NULL`, per class. Register the metric family and extend the hand-maintained component list at `internal/observability/observability_test.go:142-148` in the same commit.
3. **A test that seeds more than one batch.** Every retention test today seeds ≤2 rows per class and calls `PurgeOnce` once; `defaultBatchSize = 1` would leave the suite green. Seed >2 batches and assert the backlog reaches zero within a bounded number of ticks.
4. **Decision + doc.** The role table above, in `.loom/40-decisions.md`. Update `PHI-RETENTION.md:293` to whichever answer is chosen — including, if C, an explicit statement that the follow-up is closed and why.
5. **(If A) The role migration.** `internal/integration/processor/migrations/0006_*.sql` — re-verify the number at rebase (correction 60) and **rebase onto S5-0 MR 0c first** so the `ADD COLUMN … NOT NULL` rule hole is closed. GRANT/REVOKE only; no table changes. Roles must be created idempotently and the migration must be safe on a database where the roles already exist.
6. **(If A) Move migrations out of `serve`.** An explicit `fi-fhir migrate` command plus a Helm pre-install/pre-upgrade hook and a Kustomize init container. **4.4a's concurrent-replica assertion moves with it** — two migrator invocations against one fresh database must still converge across all six ledgers, including the terminology migrator whose advisory lock 4.4a added. Update `docker-compose.yaml` and the developer setup docs.
7. **Prove the guarantee, do not assert it.** The point of the slice is that the application role *cannot* remove a guard. Test it directly: connect as `fi_fhir_app` and attempt `DROP TRIGGER`, `ALTER TABLE … DISABLE TRIGGER`, and `ALTER TABLE … OWNER TO`; every one must be refused by PostgreSQL, not by application code.

### Acceptance Criteria

- A tenant with 10,000 expired canonical events reaches zero backlog within a bounded, documented number of purge ticks — asserted, not reasoned about.
- The backlog gauge is non-zero while a backlog exists and returns to zero after it drains.
- One class erroring does not prevent the other classes purging in the same pass.
- (If A) `fi_fhir_app` cannot drop, disable, or take ownership of any immutability trigger; `fi_fhir_purge` can perform the tombstone path and nothing else; `fi_fhir_migrator` is the only role that can apply a migration.
- (If A) Two concurrent `fi-fhir migrate` invocations against one fresh database converge across all six ledgers — 4.4a's assertion, relocated and still passing.
- `make phi-audit`, `make phi-retention-purge`, `make migration-compatibility`, and `make observability-replicas` all pass unchanged; the tombstone exemption trigger's behaviour is byte-for-byte what it is today.
- An unconfigured deployment still purges nothing (`cmd/fi-fhir/retention_runtime.go:44-47`).

### Kill-Test (negative-controlled)

**Primary: `TestPurgeRoleSeparation_BacklogDrainsAndTheAppRoleCannotDisarmTheGuard`** — PostgreSQL 16, `-race`:
1. Seed 1,000 expired canonical events, 1,000 expired session samples, and one export. Run `Purger.Run` with an injected clock. Assert the backlog reaches zero within the documented tick bound and that the gauge tracks it down.
2. Poison one class (e.g. revoke a grant mid-run) and assert the other classes still purge in the same pass.
3. (If A) As `fi_fhir_app`: `DROP TRIGGER`, `ALTER TABLE … DISABLE TRIGGER`, `ALTER TABLE … OWNER TO` — each must be refused by PostgreSQL with an insufficient-privilege error.
4. (If A) As `fi_fhir_purge`: the tombstone path succeeds; every non-tombstone mutation from C1's kill-test still raises.
5. Run `make phi-audit`'s proof in the same job and assert it still passes.

**Negative controls**: (a) restore the single-`PurgeOnce`-per-tick loop behind a build tag — assertion 1 must fail at exactly one batch per tick; (b) (if A) grant `fi_fhir_app` ownership — assertion 3 must fail. A control that passes means the assertion is not on the mechanism.

**Day-1 gate — must FAIL on unmodified `main`, for the named reason: `TestPurgeThroughput_BacklogExceedsOneBatchPerTick`.** Seed 500 expired canonical events, run one `PurgeOnce`, and assert all 500 are tombstoned. On `main` it must fail with **exactly 200 tombstoned** — not a timeout, not a skip. **Already reproduced by execution** (500 drained in three calls: 200, 200, 100). Land it in a non-blocking job with the expected failure recorded in the worklog, then the fix and the promotion to blocking land together.

**Second day-1 gate — must PASS on unmodified `main`: `TestPurgeRoleSeparation_ApplicationRoleCanDropItsOwnGuardToday`.** Connect as the ordinary application role and `DROP TRIGGER` one immutability guard on a scratch database; assert it succeeds and that the mutation the guard forbade then succeeds too. Passing on `main` proves correction 54 in three statements and is the whole argument for the slice — `PHI-RETENTION.md:293` asserts it in prose today and nothing demonstrates it.

### Riskiest Assumption

> **"The purge works; role separation is the remaining hardening."**

The purge tombstones correctly, audits atomically, is safe under two replicas, and refuses every mutation it should — all verified adversarially. And it drains 200 records per class per hour with no catch-up (D1), on a table the code itself calls "the busiest table in the system", with no gauge that would reveal the backlog. A retention control that cannot honour its own policy is not a hardening candidate; it is the defect. Separately, "role separation" read as a SQL task understates it: the app role's privilege comes from `serve` running six migrators on the runtime connection, so de-privileging it is a deployment-shape change that must carry 4.4a's concurrent-startup proof with it.

The first day-1 gate kills the throughput half in one assertion; the second kills the "it's just GRANTs" half in three statements. Together they set the task order: repair, then re-topology, and re-file the re-topology if it does not fit.

---

## Suggested Execution Order

**Pre-sprint (blocking):** the three coordinator decisions below. Sprint 5 branches from `852d7f3ee`.

### Day 1 — ten gates, in parallel, each a standalone test-only MR against `main`

Each must produce its stated result **for its stated reason** before its lane writes production code. Four of the ten are already reproduced by execution while writing this document; landing them makes the reproduction repeatable and turns each into its lane's negative control.

| Gate | Lane | Expected on `main` | Kills |
|---|---|---|---|
| `scripts/ci-job-inventory.sh` | S5-0a | **Pass**, emitting 55 jobs — with `test:destination-transport` visibly outside the test-stage block | "the CI append point is hygiene that can wait" |
| `TestTransportRecordsProvenanceWhenTheDestinationIsSlow` | S5-0b | **Fail**: zero provenance rows, `context.DeadlineExceeded`. *Reproduced.* | "4.1c-b's provenance ledger records every contact" |
| `TestMigrationRule_AddColumnNotNullWithoutDefaultIsFlagged` | S5-0c | **Fail**: silently skipped, `tightened == 0`. *Reproduced.* | "the migration rule is mechanically enforced" |
| `TestPerformanceHarness_NothingMeasuresAnyProductBudgetToday` | S5-A | **Pass**: zero benchmarks under `internal/integration` | "the green benchmark gate is partial credit toward 4.4" |
| `TestChaosRecovery_RestoreProofAssertionsAreTriggerAttributed` | S5-B | **Fail**: three of six refuse with `23503`, not `P0001`. *Reproduced.* | "4.4a proved restore" |
| `TestStructuredLogging_ServeEmitsNoStructuredLogAndTheQueueDriverPrintsPayloads` | S5-C | **Pass**: no JSON line, and the PHI sentinel appears verbatim on stdout | "4.4d is two build items: add slog, add the exporter" |
| `TestMLLPCapacity_TwoReplicasAdmitTwiceTheDeclaredRateToday` | S5-D | **Pass**: ~200/s against a declared 100/s, zero durable rate rows | "the gate is correct per replica; just move the state" |
| `TestFHIRConformance_ValidatorRejectsMapperOutputToday` | S5-E | **Fail**: `meta.profile does not include an expected profile for DiagnosticReport`. *Reproduced.* | "5.1 opens by integrating a validator" |
| `TestFHIRConformance_DurableEngineProducesNoFHIRResource` | S5-E | **Pass**: the HTTPS body is a delivery-command envelope at `application/json` | "4.1c-b unblocked 5.1" — **the sprint's most consequential gate** |
| `TestPurgeThroughput_BacklogExceedsOneBatchPerTick` | S5-F | **Fail**: exactly 200 of 500 tombstoned. *Reproduced.* | "the purge works; role separation is the remaining hardening" |
| `TestPurgeRoleSeparation_ApplicationRoleCanDropItsOwnGuardToday` | S5-F | **Pass**: the `DROP TRIGGER` succeeds and the forbidden mutation then succeeds | "role separation is just GRANTs" |

Plus the sprint-wide `TestReleaseCandidate_MergedContractsHoldUnderSustainedLoad`, which must **fail four times for four named reasons** and is the kill-test for the sprint's riskiest assumption.

**If any gate disconfirms this document, the affected lane corrects this file before writing production code.** That rule produced Sprint 4's corrections 11, 23, and 36, and it is why those are in a plan rather than in a post-mortem. It is also why corrections 25, 26, 43, and 57 are in this one.

### Day 1 also — five decisions land as docs-only commits

Every lane except S5-0 has a decision as its gate, and three of them constrain other lanes:
- **S5-A's pinned-runner decision** determines whether S5-B needs to render the reference-profile values file (task 4) and what `SUPPORTED-1.0.md` claims.
- **S5-D's distribution decision** determines whether budget 2 is measurable at all, which is S5-A's scope.
- **S5-C's adopt-or-retire decision** determines whether S5-D and S5-F emit structured logs from birth or retrofit them.
- S5-B's WAL/PITR posture and S5-F's role-topology decisions are lane-local but gate their own migrations.

### Wave 2 — implementation, parallel; merge order S5-0 → S5-C → S5-D → S5-F → S5-B → S5-A, S5-E whenever

1. **S5-0** first, and its three MRs in order. Nothing appends to `.gitlab-ci.yml` until 0a merges. 0c must precede any migration authoring.
2. **S5-C** second: widest textual footprint, and it gives S5-D and S5-F a logger to emit from.
3. **S5-D** third: adds the serve component and repairs `errCh`; unblocks S5-A's budget 2.
4. **S5-F** fourth: D1 first, then the role topology or its explicit closure.
5. **S5-B** fifth: rebases onto S5-D's and S5-F's durable state so the repaired restore proof covers them.
6. **S5-A** last, by dependency.
7. **S5-E** merges whenever; it contends with nothing except one line of `test:benchmark`'s package list.

### Wave 3 — beyond Sprint 5

- **4.1c-c — a FHIR destination class.** 5.1's real prerequisite (correction 40). Needs a coordinator decision to exist at all.
- **5.1b — validator integration** (Option C then Option A) after 5.1a reconciles the mapper and the checker.
- **Wall-clock certification of budgets 1-3** on a registered pinned runner.
- **Budget 7 live golden-journey evidence** on Kubernetes 1.36.
- **Reference archiving configuration (S5-B option B)**, if the posture decision goes A and the product later needs to own the RPO.
- **Purge role separation (S5-F option A)**, if it does not fit alongside D1 this sprint.

---

## Decisions Required Before Lanes Launch

Three need a human or the coordinator. Five more are lane deliverables and are listed after them for completeness.

### 1. Does 4.1c-c — a FHIR destination class — get funded? (blocks S5-E's shape; decide before day 1)

**The finding**: 5.1 is not unblocked. The `.loom/28:206-212` kill-test's stated answer is "still blocked, and the blocker is 4.1c-b's scope" (correction 40). The durable engine delivers a Kafka command envelope over HTTPS at `application/json`; no FHIR resource exists on any path a golden journey executes.

**The options**:
- **(a) Fund 4.1c-c now**, as a Phase 4 delivery slice — a FHIR destination class, a canonical-event→resource mapping step on the delivery path, `application/fhir+json`, and a decision about which mapper produces it. Then 5.1 certification becomes possible in Sprint 6. This is the only path to journey 1's "inspect trace and FHIR delivery" and journey 6's US Core validation.
- **(b) Run 5.1a only this sprint** (this spec's lane S5-E: reconcile the mapper and the checker, close the fail-open mode, choose the version policy). High value, zero blockers, but it does not move the 1.0 gate.
- **(c) Both** — 5.1a this sprint, 4.1c-c specced in parallel for Sprint 6.

**Recommendation: (c).** S5-E as specced is (b) and needs no decision to start. But 4.1c-c is a genuinely new slice nobody has sized, it sits on the 1.0 critical path, and it will not appear on its own. It should be specced while 5.1a runs.

### 2. Is a pinned CI runner going to exist? (blocks S5-A's certification claim; decide before day 1)

**The finding**: there is not one `tags:` block in 2738 lines of `.gitlab-ci.yml` (correction 7). The repo-side pin is a one-line `tags:` per job plus a quota bump from `KUBERNETES_CPU_LIMIT: "1"` / `MEMORY_LIMIT: "2Gi"`. The runner itself must be registered in `platform/gitops`, **outside this repo**.

**Recommendation**: authorise the repo-side contract regardless — S5-A ships blocking `allocs/op` gates in the shared pool (the signal this repo has already proved is CPU-independent across 78-87 artifacts, `benchmark_util.go:321-327`) plus an inert, tagged, manual, `allow_failure: true` job carrying the wall-clock harness and a documented report schema. Then **either** commit to registering a `fi-fhir-perf` runner, **or** accept that budgets 1-3 stay *harnessed but uncertified* through the RC gate and say so in `SUPPORTED-1.0.md`. What must not happen is a calibrated p95 gate in the shared pool: the 1.6x margin was backtested on micro-benchmarks, and a durable-accept path with PostgreSQL and Kafka service containers in the same 1-CPU pod on a 5.3x-spread pool is nowhere near that variance. **Do not decide this inside the lane** — it determines whether the RC gate can be declared met.

### 3. Ratify or amend the 5.1 validator decision (blocks S5-E's first commit)

`.loom/40-decisions.md:1428` is explicitly PROPOSED and states it "requires human or next-sprint ratification before any lane acts on it" (`:1430-1433`).

**Recommendation — split it:**
- **RATIFY the confinement half unconditionally**: `validator_cli.jar` is CI-only and never enters the shipped image; the image stays `gcr.io/distroless/static-debian12:nonroot` (`Dockerfile:27`); IG packages are pinned `.tgz` offline artifacts. `security:trivy-image` blocks on CRITICAL and HIGH-fixed with no `allow_failure` (`.gitlab-ci.yml:1851-1852`), and this repo's trivy database moves daily, so a JRE in the image is a recurring blocking-gate failure with no owner.
- **AMEND the ordering half**: the decision recommends a Go structural validator over pinned `.tgz` first. Do reconciliation first instead. The shipped validator rejects the shipped mapper's own output (correction 41, reproduced through the CLI), fails open on any mode string that is not exactly `us-core` (correction 45, `--mode US-Core` prints "passed"), and CI has no fixture for the input the mapper produces. A bigger validator on a mapper it disagrees with certifies the disagreement. The decision's own open item at `:1565-1567` says the same thing.

### Coordinator rulings (2026-08-09, pre-launch)

1. **4.1c-c: option (c) adopted.** S5-E runs 5.1a exactly as specced below; 4.1c-c (the FHIR destination class) gets a dedicated coordinator-owned spec pass in parallel, targeting Sprint 6, so the 1.0 critical path is sized before Sprint 5 closes.
2. **Pinned runner: repo-side contract authorized.** S5-A ships blocking `allocs/op` gates in the shared pool plus the inert, tagged (`fi-fhir-perf`), manual, `allow_failure: true` wall-clock job with the archived-report schema. Runner registration is escalated to the operator as a `platform/gitops` task; until a runner exists, budgets 1–3 are stated as *harnessed but uncertified* in `SUPPORTED-1.0.md`. No calibrated p95 gate ships in the shared pool.
3. **Validator decision: split ruling adopted.** The confinement half is RATIFIED (`validator_cli.jar` CI-only, distroless image unchanged, IG packages as pinned offline `.tgz`). The ordering half is AMENDED: reconciliation (5.1a) precedes any validator integration. S5-E amends the PROPOSED entry at `.loom/40-decisions.md:1428` accordingly as its first docs commit.

### Lane-deliverable decisions (recorded in `.loom/40-decisions.md` on day 1, not escalated)

| Decision | Lane | Recommendation |
|---|---|---|
| Wall-clock gate placement and the `workflow loadtest` disposition | S5-A | Option A; delete or relabel the loadtest subcommand |
| WAL/PITR posture: documented procedure vs shipped archiving config | S5-B | Option A — certify RTO, state RPO as a method-dependent operator responsibility, file the archiving config |
| Adopt `log/slog` vs adopt `internal/workflow/logging.go` vs both | S5-C | Option A — adopt `slog`, retire the orphan and `internal/ingest`, port the OTel correlation |
| Rate distribution: lease-partitioned quota vs per-frame counter | S5-D | Option A — lease-partitioned; admission stays in-memory |
| Role topology: three roles + migrations out of `serve` vs document-and-close | S5-F | Option A if it fits alongside D1; otherwise D1 plus an explicit closure and a re-filed slice |

### Two things the coordinator should know even though they need no decision

- **D1 and D2 are release blockers that merged in the last two days**, both with green pipelines, both passing their own lanes' kill-tests. D2 in particular is inside the product spec's P0 definition (`.loom/20…:284-286`) — duplicate delivery for one idempotency key. They are assigned (S5-F task 1, S5-0 MR 0b) and both fixes are small, but neither was on anyone's plan before this document.
- **`errCh` has one slot of headroom against nine senders** (correction 59). S5-D's task 6 bumps it to 12. If S5-D is descoped, whichever lane adds a component must take that task with it, or shutdown hangs latently — it is not a compile error and no test covers it.

---

## Sources

All read or executed against `main` @ `852d7f3ee`. Claims marked *reproduced* in the text were proved by running code, using `go test -overlay` with probe sources outside the repository and a disposable PostgreSQL 16 container on the remote Docker context; the repository was not modified.

**Performance and benchmarking (S5-A):**
- `.gitlab-ci.yml:1-8,19-50,56-78,112-117,120-171,341-353,1238-1293,1257-1262,1840-1866,2650-2738`
- `Makefile:1-17,150-220,252-262,367-368,647-661`
- `internal/workflow/benchmark_util.go:200-205,281-308,321-335,349-412,415-432,456-473,538-602,65-103`; `benchmark_test.go` (29 `func Benchmark`, `:42…:914`)
- `cmd/bench-check/main.go:163-169,188-249,315-346`; `cmd/bench-check/suggest.go`
- `cmd/fi-fhir/main.go:1955,2758-2940,2868,2916,2931`; `internal/workflow/loadtest.go:3-11,84-86,146,292-293,340,510-516,586-626`
- `configs/adt-workflow.yaml:14`; executed run output (1000 events, 100% error rate, `function "event" not defined`)
- `docs/operations/SUPPORTED-1.0.md:14-31,22,24,33-48,44-75,52-53,59,62-70,84-86`
- `deploy/helm/fi-fhir/values.yaml:5,88-92,188-201,214-220`; `values-reference-profile.yaml:16-20,22-37`
- `deploy/kubernetes/base/deployment.yaml:10,121-125,129,135-141,148-169,180,187-202`; `overlays/production/kustomization.yaml:14-20,23-35,58-62`
- `.loom/20-product-spec-integration-engine-ide-completion.md:265-280,284-286,289-304`

**Chaos, DR, deployment (S5-B):**
- `internal/integration/migrationcompat/compatibility_integration_test.go:40,44-110,149-182,216-225,240-251,256-268,270-318,325-342,348-410`; `fixture_test.go:33-48,125-230,188-210`; `database_harness_test.go:40-62`; `negative_control_integration_test.go:31,48,97,157,215`; `postgres_harness_test.go:26-31`
- `scripts/pgdump-roundtrip.sh:31-34,114-121,125-139,153,159-167`
- `docs/operations/PRODUCTION-HARDENING.md:579,583-598,913-943,945-988,990-1016,1018-1042`
- `test/e2e/integration_test.go:8,38-42,54,62,70,138,146,151,219,227,274,323-338,339-427,345,406,413-426`; `test/e2e/e2e_test.go:8,66-466`; `test/e2e/docker-compose.yaml:10-88` (esp. `:11`)
- `docker-compose.yaml:13-129,104-108,117-126,132-179,182-294,226-243`
- `deploy/kubernetes/base/postgres.yaml:1-16,19,79-98`; `base/pdb.yaml:1-12`; `deploy/helm/fi-fhir/templates/deployment.yaml:2,8-10,103-108,172-193,195-196`; `templates/pdb.yaml:1-13`; `templates/NOTES.txt:57-59`
- `scripts/validate-kustomize-preview.sh:30-55`
- `internal/observability/replicas_integration_test.go:68,79,91-105,185-222,793-869,875-969`
- Fault/restart precedents: `internal/integration/processor/postgres_submission_integration_test.go:25,56-70`; `lifecycle/postgres_integration_test.go:24,201-226`; `mllp/server_integration_test.go:44,261-262`; `batch/batch_integration_test.go:38,55`; `batch/service_test.go:18,33-35`; `delivery/delivery_integration_test.go:30,178-208`; `session/postgres_integration_test.go:34`; `cmd/fi-fhir/serve_cli_test.go:13,109`

**Logging, tracing, observability (S5-C):**
- `internal/observability/metrics.go:24-53,58-64,67-70,74-93,99-113,121-128,156-160,213-217,255-267,281-283,349-360,362-377,410,419-439,447-467`; `server.go:18,20-45,34-37,45-91,66-73`; `health.go:68-71,180-209`; `mode.go:28-67`; `observability_test.go:102-118,120-164,142-148,156-158`
- `internal/workflow/logging.go:17-44,46-79,113-286,169,172,198-211,213-260,288-317,321-359`; `engine.go:121,171,175,489`; `queue.go:313-333,321-322`; `actions.go:10,23,38,47-56,632,638,670,680`; `tracing_otel.go:7-13,52,97`; `tracing.go:243`; `ratelimit.go:41-277`; `actions_athena.go:81-111`
- `internal/ingest/http.go:42,49,69-186`; `internal/ingest/temporal.go:10`
- `cmd/fi-fhir/serve_observability.go:19-182,20,38,61,80,127-128,129-182,132,138,147,153,160,173,178`
- `cmd/fi-fhir/main.go:28,50,266,3217-3219,3405-3407,4541-5426,4679,4778,4846,4874-4876,4893,4899,5154-5158,5220,5232-5356,5238,5382-5384`
- `pkg/config/config.go:188-195,420-421,609-613,758-768`
- `scripts/check-runtime-config.sh:218-247,220-225,232-234,238,241-245`
- `.env.example:67,75,96-99,375-380`; `configs/full-stack.env:74-77`; `README.md:67,402-415`; `deploy/kubernetes/base/configmap.yaml:15,45-46`
- `go.mod:15,24-27,44,71,98-100,113-115,124,126-129`; `go.sum` (otlptrace/otlptracehttp/proto-otlp v1.19.0/v1.0.0 resident)
- `pkg/integration/contracts.go:129,208,227,344,497-506,929`; `pkg/events/events.go:122,396-398`
- `internal/integration/processor/migrations/0001_atomic_submission.sql:9,25,41,52-54,62,71-77`
- `internal/api/requestsecurity/auth.go:126-146`; `internal/api/graphql/server.go:9,83,86,200-205,237-242,412,657,666`; `operation_authorization.go:77`
- `internal/integration/ingress/http.go:21,46,86,104,130,139,159,200`; `mllp/service.go:216-217,230`; `batch/service.go:371,437`; `delivery/dispatcher.go:319,343,364`

**MLLP capacity (S5-D):**
- `internal/integration/mllp/capacity.go:17-32,35-83,40-53,54-82,104-126,109-118`; `capacity_test.go:12,51`
- `internal/integration/mllp/service.go:46-55,55,77,122,204,206,232`; `server.go:81,102-112,198-205`; `protocol.go:218,227`; `source.go:120,309`
- `pkg/integration/deployment.go:61-78,85,104-109`; `internal/integration/lifecycle/queries.go:157-193,192`
- `docs/operations/PRODUCTION-MLLP.md:42-71,51-59,154`
- `.loom/40-decisions.md:1336-1350,1394-1401`

**FHIR conformance (S5-E):**
- `pkg/fhir/validate.go:104-152,109-111,114-146,153-177,171,179-237,218-219`; `types.go:13,15-56,36,47`; `mapper.go:120-133,220,319,392,435,844,913,1241,1263,1287,1298-1313,1598,1925-1935,2352,2564,2799,3111,3630,3981,4300,4668,4976,5377,5812,6082,6380,6532,6650,6813,6980`; `validate_golden_test.go:17`; `testdata/fhir/diagnosticreport_uscore_note.json`
- Executed: `MapLabResult` → `ValidateJSON` (1 issue) and via `bin/fi-fhir fhir validate` (exit 1); `--mode US-Core` (exit 0 on the same bytes); `|9.0.0`-pinned Patient (1 issue); MRN-only Patient (`[error] Patient.identifier is required`)
- `internal/fhir/subscription/mapper.go:30`; `internal/workflow/actions.go:23,670,680`
- `docs/planning/FHIR-CONFORMANCE-MATRIX.md:174-175,204,207-211,226,259,264`; `FHIR-PROFILES.md:5-20,28,80,445,541-542`; `.loom/28-spec-fhir-ig-bulk-smart.md:106,157,204,206-212`
- `.loom/40-decisions.md:1428-1433,1436-1437,1446-1451,1457-1468,1495,1508,1519,1524,1557,1559-1573`
- `.loom/worklog/2026-08-08-phase-5-1-fhir-conformance-docs-prep.md:59`
- `Dockerfile:27,58`; `.gitlab-ci.yml:1840-1866,1851-1852`
- `internal/integration/delivery/dispatcher.go:162,166,348`; `delivery/store.go:107,128`; `destination/transport.go:325`; `destination/revision.go:57,61`; `pkg/integration/contracts.go:602`; `cmd/fi-fhir/delivery_runtime.go:186`

**Retention and purge (S5-F):**
- `internal/integration/retention/purger.go:65,142-159`; `store.go:28-33,246-250,253-289,311,339,363,390,392-405,409,441,474,512,518`
- `cmd/fi-fhir/retention_runtime.go:19-23,26-47`
- `internal/integration/processor/migrations/0005_retention_expiry.sql:9-12,191`; `session/migrations/0006_retention_expiry.sql:151`; `session/migrations/0007_export_attribution_defaults.sql`
- `internal/integration/session/stream.go:174-179`; `internal/terminology/autoroute/sweeper.go:34-72,104-119`; `pkg/terminology/db/mappings.go:1030-1040`
- `docs/operations/PHI-RETENTION.md:114-115,293`; `.loom/40-decisions.md:1631-1632,1659,1667`
- `.loom/worklog/2026-08-08-slice-4-1e-retention-purge-day-1.md:62`; `…-runtime-shipped.md:69`

**Destination transport (D2):**
- `internal/integration/destination/transport.go:228-250,237-238,239-245,272-275,294-309,325,456`; `transport_test.go:367`
- `internal/integration/destination/migrations/0002_https_delivery_provenance.sql:23-26`
- `internal/integration/delivery/dispatcher.go:73,146,225-230,239,246,260-262,275`; `delivery/transport.go:85-99`; `delivery/types.go:107-115`

**Migration ledgers and process:**
- `internal/integration/processor/postgres_submission.go:80,149-172`; `session/postgres.go:49,95-144`; `batch/store.go:18,118-141`; `destination/postgres.go:16,25,78-101`; `lifecycle/postgres.go:70-90`; `pkg/terminology/db/schema.go:7,532,713,724`
- `internal/integration/migrationcompat/migration_rule_test.go:18-43,25,29,37-43,100-102`
- `AGENTS.md:210,214-268,228,232,237-258`
- `.loom/worklog/` (38 entries), `.loom/50-worklog.md` (43-line pointer page), `scripts/worklog.sh`, `.gitlab-ci.yml:354-399,371`
- `git worktree list`, `git show --stat` for `e5f8e2082`, `e77c6218b`, `895b97412`, `67b874e5`

**Plan and prior specs:**
- `.loom/30-implementation-plan-integration-engine-ide-completion.md:760-880,887-946,932-946`
- `.loom/32-sprint4-execution-specs.md:72,86,90,92,94,96,110,116,136,142-156,169,374-395,416,487-496,502-531`
- `.loom/31-sprint3-execution-specs.md:143-175,301-306,385-387,483-502`
- `.loom/slice-handoff-phase-4-slice-4-3-observability.md:63-93`; `.loom/slice-handoff-phase-4-slice-4-4a-migration-compatibility.md:16,99`
