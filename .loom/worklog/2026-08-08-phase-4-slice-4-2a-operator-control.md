### 2026-08-08 - Phase 4 Slice 4.2a operator control plane

- What changed:
  - Implemented Phase 4 Slice 4.2a, the operator control-plane GraphQL API.
  - Added `internal/integration/operator`: a tenant-scoped PostgreSQL read
    projection (receipts, canonical events, lineage, delivery attempts, DLQ,
    circuits, delivery audit) with keyset pagination and opaque cursors, plus a
    role-gated control service that delegates writes to
    `internal/integration/delivery` and `internal/integration/lifecycle`.
  - Added `PostgresStore.Discard` and a DLQ `resolution`/`resolved_at` column via
    submission migration `0003_operator_control_plane`, and
    `PostgresCatalog.ListSnapshots` for the deployment inventory.
  - Extended `schema.graphql` with nine operator queries and seven control
    mutations, regenerated gqlgen artifacts, and implemented the resolvers.
  - Allowlisted the control plane's catalog-safe messages in the GraphQL error
    presenter so conflicts are distinguishable without leaking inventory.
  - Wired the control plane into `serve` behind the existing durable submission
    database, sharing one lifecycle catalog with session publication.
  - Added required CI job `test:operator-control-plane` and `make
    operator-control-plane`.
- Why:
  - Slice 4.2 requires the failure/replay and operator-audit golden journeys to
    pass without SQL or manual filesystem intervention, over the durable records
    Slices 2.1 and 2.3 already own.
- Evidence:
  - Kill-test `TestOperatorControlPlane_FailureReplayAndAuditGoldenJourneys`
    passes on PostgreSQL 16 with `-race`.
  - Negative control: emitting scalar values from the payload summarizer makes
    the kill-test fail on the raw-PHI sentinel assertion, so the leak check is
    not vacuous.
  - The required delivery-reliability proof caught a real defect: the resubmit
    DLQ resolution label was built by string concatenation and produced an
    invalid value. Fixed, and the kill-test now exercises resubmit directly.
  - `gofmt` clean, `golangci-lint run` 0 issues, `go vet ./...` clean,
    `go test -race ./...` green, `go mod verify` and `go mod tidy -diff` clean.
  - Sibling PostgreSQL suites re-verified: lifecycle, processor, session, and
    the Kafka-backed delivery-reliability proof.
- What's next:
  - Ship 4.2a, then branch 4.2b from fresh main for the operator UI.
- Sources:
  - [S1] `.loom/iteration-plan-phase-4-slice-4-2-operator-control-plane.md`
  - [S2] `internal/api/graphql/operator_control_plane_integration_test.go`
  - [S3] Command: `make operator-control-plane` with `POSTGRES_TEST_URL` set
