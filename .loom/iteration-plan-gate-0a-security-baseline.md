# RALPH Iteration Plan: Gate 0A Security Baseline

**Status**: complete — MR !89 pipeline 18379 green; latest evidence-only SHA
must retain a terminal green MR pipeline before merge
**Date**: 2026-07-12

## Review

- Roadmap milestone: Completion Gate 0A
- Spec sections: secure production data plane; truthful reliability/operations
- Prior decision: extend the mature parser/workflow kernel; do not rewrite it.
- Evidence:
  - deployed-build pipeline `15878` found 14 reachable standard-library
    vulnerabilities with Go `1.25.7`;
  - the same pipeline found HIGH/HIGH G701 at
    `cmd/fi-fhir/eventstore.go`;
  - completion review found the same identifier class across IDE-authored
    `event_store` and `database` workflow action configuration;
  - official current stable Go is `1.26.5`:
    https://go.dev/doc/devel/release;
  - golangci-lint `v2.8.0` was built with Go 1.25; mirrored `v2.12.2`
    is built with Go 1.26 and its upstream release supports Go 1.26:
    https://github.com/golangci/golangci-lint/releases/tag/v2.12.2.

## Align

### Scope in

- Go `1.26.5` in `go.mod`, CI, Docker, and local minimum-version checks.
- golangci-lint `v2.12.2` in CI and Makefile.
- pinned govulncheck `v1.6.0` and gosec `v2.27.1` with reproducible package
  roots and matching local/CI HIGH/HIGH gates.
- one shared lowercase PostgreSQL identifier validator.
- validation at the event-store CLI, `event_store` action, and `database`
  action boundaries; defense-in-depth quoting at direct query construction.
- internal quoting in every public PostgreSQL event/checkpoint/projection/stream
  snapshot store constructor so library consumers cannot bypass caller validation.
- malicious, empty, NUL, case, qualification, and 63/64-byte boundary tests.
- completion plan, research, roadmap, inventory, and decision records.

### Scope out

- promotion of advisory CI security jobs to required merge gates (Gate 0B after
  this MR proves them green);
- UI/npm audit remediation and false-green UI/smoke repair (Gate 0B);
- dependency-wide Renovate upgrades;
- runtime spine, MLLP, or durable Integration Sessions.

### Acceptance criteria

- invalid table/column/conflict identifiers fail before database access;
- lowercase simple identifiers of 1-63 bytes continue to work;
- dynamically constructed PostgreSQL identifiers are quoted after validation;
- public event-sourcing stores safely quote raw, unqualified table and derived
  index names even when callers do not use the stricter CLI validator, mapping
  embedded NUL bytes and overlength names to deterministic physical names;
- the only G701 suppression is adjacent, reasoned, and backed by the validator
  regression; `gosec -nosec` may still expose analyzer taint but the normal
  gate has no unwaived HIGH/HIGH finding;
- all Go/build/lint pins are compatible with Go 1.26.5;
- govulncheck exits zero with no reachable/imported vulnerabilities;
- gosec exits zero with no unwaived HIGH/HIGH findings;
- targeted, race, full Go, vet, build, and terminal MR pipeline pass, or a
  pre-existing flake is reproduced and recorded precisely.
- the actual prebuilt-image `lint:go`, `security:govulncheck`, and
  `security:gosec` MR jobs each finish `success`; overall pipeline success with
  advisory security warnings is not Gate 0A evidence.

## Land

### Exact intended files

- `go.mod`
- `Dockerfile`
- `.gitlab-ci.yml`
- `.golangci.yml`
- `Makefile`
- `CHANGELOG.md`
- `internal/sqlutil/identifier.go`
- `internal/sqlutil/identifier_test.go`
- `cmd/fi-fhir/eventstore.go`
- `cmd/fi-fhir/cli_coverage_p2_test.go`
- `internal/workflow/event_store.go`
- `internal/workflow/event_store_test.go`
- `internal/workflow/database.go`
- `internal/workflow/workflow_test.go`
- `internal/workflow/engine.go`
- `internal/api/graphql/store/workflow_lifecycle_pg_store.go`
- `pkg/terminology/db/mappings.go`
- `pkg/eventsourcing/postgres_identifier.go`
- `pkg/eventsourcing/postgres_identifier_test.go`
- `pkg/eventsourcing/postgres_store.go`
- `pkg/eventsourcing/compaction.go`
- `pkg/eventsourcing/postgres_integration_test.go`
- `ROADMAP.md`
- `.loom/00-mcp-inventory.md`
- `.loom/10-research.md`
- `.loom/20-product-spec-integration-engine-ide-completion.md`
- `.loom/30-implementation-plan-integration-engine-ide-completion.md`
- `.loom/40-decisions.md`
- this iteration plan

### Implementation sequence

1. Add failing identifier regression cases.
2. Centralize and apply strict identifier validation at every discovered
   application configuration boundary; quote identifiers inside public stores.
3. Quote direct query identifiers and retain a documented G701 suppression only
   where the analyzer cannot recognize the validation/quoting boundary.
4. Update Go/linter/scanner pins and make local commands match CI.
5. Run targeted tests/scans, then broad quality gates and CI.

## Prove

### Targeted

```bash
go test -count=1 ./internal/sqlutil ./internal/workflow ./cmd/fi-fhir \
  ./pkg/eventsourcing \
  -run '^(TestValidatePostgresIdentifier|TestParseEventStoreConfig|TestDatabaseConfigParsing|TestGetEventStoreDB_|TestPostgresStoreConstructorsQuote|TestPostgresStoreConstructorsNormalizeNULIdentifiers|TestNormalizePostgresIdentifier|TestDerivedPostgresIdentifiersDoNotCollideAtMaximumBaseLength)'

# Against a disposable PostgreSQL 16 instance or POSTGRES_TEST_URL:
go test -p 1 -count=1 -tags=integration ./pkg/eventsourcing \
  -run '^TestPostgresStores_Integration_MaxLengthDerivedIdentifiers$'
```

### Security

```bash
env GOTOOLCHAIN=go1.26.5 CGO_ENABLED=0 GOWORK=off GOFLAGS=-mod=readonly \
  GOMAXPROCS=2 go run golang.org/x/vuln/cmd/govulncheck@v1.6.0 \
  ./cmd/... ./internal/... ./pkg/... ./scripts/...

env GOTOOLCHAIN=go1.26.5 CGO_ENABLED=0 GOWORK=off GOFLAGS=-mod=readonly \
  GOMAXPROCS=2 go run github.com/securego/gosec/v2/cmd/gosec@v2.27.1 \
  -quiet -fmt text -severity high -confidence high -exclude-generated \
  -exclude=G104,G201,G304,G301,G302,G306,G115,G404,G101,G602,G703,G704 \
  ./cmd/... ./internal/... ./pkg/... ./sdk/...
```

### Build and broad validation

```bash
go mod verify
go mod tidy -diff
go vet ./...
CGO_ENABLED=0 go build ./...
go test -race ./cmd/fi-fhir ./internal/workflow ./internal/sqlutil ./pkg/eventsourcing
go test -count=1 ./...
```

Harbor preflight:

```bash
docker --context 7900xtx manifest inspect \
  registry.harbor.lan/dockerhub-cache/library/golang:1.26.5-alpine
docker --context 7900xtx manifest inspect \
  registry.harbor.lan/dockerhub-cache/library/golang:1.26.5
docker --context 7900xtx manifest inspect \
  registry.harbor.lan/dockerhub-cache/golangci/golangci-lint:v2.12.2-alpine
```

### Evidence record

| Proof | Status/evidence |
|---|---|
| Regression tests red before fix | observed 2026-07-12 |
| Targeted identifier tests | green locally |
| Harbor Go/linter manifests | green locally |
| govulncheck v1.6.0 | green locally; module-only unimported GO-2026-5932 remains visible |
| gosec v2.27.1 HIGH/HIGH | green locally |
| Go 1.26.5 build/compile compatibility | green locally |
| golangci-lint v2.12.2 via local Go 1.26.5 | green locally |
| prebuilt linter image + auto toolchain resolution | pipeline 18379 job 177057 `lint:go`: success; image v2.12.2 built with Go 1.26.2 resolved the module's Go 1.26.5 requirement |
| vet + CGO-disabled build | green locally |
| targeted race tests | green locally |
| full `go test -count=1 ./...` | green locally |
| PostgreSQL 16 max-length/derived schema integration | green in disposable remote container |
| MR pipeline and individual advisory security jobs | pipeline 18379: success; job 177072 `security:govulncheck`: success; job 177073 `security:gosec`: success |

## Risks and rollback

- Lowercase-only application validation deliberately rejects legacy
  mixed-case/schema-qualified names to keep CLI/workflow behavior predictable.
  Migration is rename-to-lowercase; silently restoring unsafe names is not rollback.
- Public event-sourcing constructors treat `TableName` as one raw, unqualified
  identifier. A former schema-qualified value such as `public.events` now names a
  safe literal table instead of selecting a schema; callers must select schema in
  the connection/search path or migrate to an explicit future schema contract.
  Embedded NUL bytes and overlength names are mapped to deterministic safe
  physical names.
- The database action already emits PostgreSQL placeholders/upsert syntax despite
  advertising other DSNs. This slice secures current PostgreSQL semantics; driver
  parity is tracked separately.
- govuln database contents remain mutable even with a pinned client; CI evidence
  records date/tool version and the module-only advisory.
- If Go/linter compatibility fails in CI, keep the SQL fix, collect the exact
  incompatibility, and move to the newest mirrored 1.26-compatible lint image.
  Do not roll back to a vulnerable Go patch without an explicit security exception.
- Code rollback is the MR revert after disabling affected configuration entry
  points; database/schema destructive rollback is not required.

## Handoff/harvest

- Record Gate 0A commit, MR, pipeline, scan summaries, and any exception in the
  shared plan and agent context.
- Next slice: Gate 0B truthful binary/UI/smoke/security gates.
