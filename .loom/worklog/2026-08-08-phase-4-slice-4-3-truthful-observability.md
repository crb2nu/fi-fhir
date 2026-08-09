### 2026-08-08 - Phase 4 Slice 4.3 truthful observability (Lane S3-A)

- What changed:
  - Added `internal/observability`: one `prometheus.Registry` on a second
    listener at `FI_FHIR_METRICS_PORT`, plus a health surface that wires the
    already-shipped `workflow.HealthService` for the first time. `/health` is
    process-only liveness; `/ready` is dependency-touching and returns 503.
  - Fixed the GraphQL `health` resolver body to project the same component set.
    No schema change; `generated.go` byte-identical.
  - Added `Observe` seams to the MLLP service, delivery `Dispatcher`, batch
    `Runner`, and session `Hub`, and bound them to the registry in `runServe`.
  - Replaced the process-local session SSE fanout with an envelope-only durable
    log (session migration `0005`) plus a per-replica relay.
  - Derived the batch worker ID from hostname+pid, moved autoroute notification
    de-duplication into a durable `notified_at` claim (terminology schema v3),
    and documented MLLP `CapacityPolicy` as per-replica.
  - Called `PostgresCatalog.ReportHealth` from a runtime health reporter.
  - Replaced `exec ["/fi-fhir","version"]` probes with `httpGet /health` and
    `/ready`; aligned the Helm chart so both deployment paths agree.
  - Rewrote 32 alert rules into 10 actionable ones and the Grafana dashboard
    against emitted metrics; corrected `.env.example` and `docs/operations/*`.
  - Extended `scripts/smoke-test.sh` (and its self-test) to assert the component
    projection, readiness/status agreement, and the exposition; added five
    blocking observability-truth checks to `scripts/check-runtime-config.sh`.
  - Added required CI job `test:observability-replicas` and
    `make observability-replicas`.
- Why:
  - Slice 4.3 required `/health`, `/ready`, and `/metrics` to report real
    component state and every background component to be correct under the
    `replicas: 2` the checked-in manifests already declared.
- Evidence:
  - Kill-test `TestServeObservability_TwoReplicasUnderDocumentedConfiguration`
    passes on PostgreSQL 16 with `-race`: two real `fi-fhir serve` processes,
    started from the documented environment block.
  - Negative control: `FI_FHIR_OBSERVABILITY_MODE=legacy` fails all four required
    assertions — `/ready` = 404, replica A receives zero events for a run on B,
    both batch runners claim `fi-fhir-batch-1`, and two notifiers page 6 times
    for 3 pending rows. Assertion 5 also fails there (no metrics listener).
  - Assertion 5 counts a real durable submission: HTTP 202 with a receipt, and no
    PHI sentinel (MRN, patient name, family name) reaches any metric label.
  - `gofmt` clean, `golangci-lint run` 0 issues, `go vet ./...` clean,
    `go test -race ./...` green (63 packages), `make lint-gqlgen` reports codegen
    up to date, `make check-runtime-config` 19 passed / 0 failures,
    `helm lint deploy/helm/fi-fhir` 0 failures,
    `scripts/validate-kustomize-preview.sh` passed,
    `bash scripts/smoke-test_test.sh` all assertions passed.
- Corrections made to `.loom/31-sprint3-execution-specs.md` before coding:
  - The session-migration row assigned `0004_*` to S3-C without noticing that
    S3-A task 6 also needs one. Settled by merge order rather than by claim:
    S3-C took `0004_export_attribution.sql`, so S3-A landed
    `0005_session_stream_events.sql`.
  - The kill-test's "stop PostgreSQL via the remote Docker context" is not
    runnable in a GitLab job whose PostgreSQL is a service container. Replaced
    with an in-test TCP proxy, which is portable and exercises pool reconnect.
- What's next:
  - S3-B and S3-C rebase onto this merge before judging their MR diffs; the
    `runServe` component table changed shape.
  - 4.4 owns the tracing exporter, structured logging, cardinality budgets under
    load, and a durable per-deployment MLLP token bucket.
- Sources:
  - [S1] `.loom/slice-handoff-phase-4-slice-4-3-observability.md`
  - [S2] `internal/observability/replicas_integration_test.go`
  - [S3] Command: `make observability-replicas` with `POSTGRES_TEST_URL` set
