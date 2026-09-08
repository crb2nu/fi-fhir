# End-to-End Tests

Two files, both driving the built `bin/fi-fhir` binary as a subprocess.

| File | Build tags | Needs |
|---|---|---|
| `e2e_test.go` | `e2e` | the binary and `testdata/`; nothing else |
| `integration_test.go` | `e2e,integration` | PostgreSQL, an HTTP echo destination, a running `fi-fhir serve` |

CI job: **`test:e2e-legacy`** (`ci/test-e2e-legacy.yml`), blocking. It builds the
binary, starts the services, runs `make test-e2e` and then the tagged
integration run, and fails on any skip that does not name a filed issue.

## Running them

```bash
# No infrastructure. This is what `make test-e2e` runs.
make test-e2e

# With infrastructure. `make test-integration` runs the same command.
docker run -d --name fi-fhir-e2e-pg -p 5433:5432 \
  -e POSTGRES_DB=fi_fhir_test -e POSTGRES_USER=test -e POSTGRES_PASSWORD=test \
  postgres:16-alpine
docker run -d --name fi-fhir-e2e-echo -p 8888:8080 mendhak/http-https-echo:31
./bin/fi-fhir serve --port 8080 --no-playground --no-introspection &

FI_FHIR_E2E_REQUIRED_SERVICES=postgres,webhook-echo,fi-fhir,fi-fhir-metrics \
  go test -tags=e2e,integration -count=1 -v ./test/e2e/...
```

`FI_FHIR_E2E_REQUIRED_SERVICES` is the anti-vacuity control. It names the
dependencies the caller has actually provided; a service in that list which is
unreachable **fails** the suite instead of skipping it. Leave it unset on a
workstation with no services and the dependent tests skip as before. The CI job
sets it to exactly the services it stands up, which is what makes "zero skips
attributable to missing infrastructure" a property of the job rather than a
hope.

`serve` needs the environment documented in `ci/test-e2e-legacy.yml` — a
canonical integration registry and a GraphQL principal — or it refuses to start.

## Environment variables

| Variable | Default | Description |
|---|---|---|
| `FI_FHIR_E2E_REQUIRED_SERVICES` | *(empty)* | Comma-separated dependencies that must be reachable; anything else skips |
| `TEST_POSTGRES_URL` | `postgres://test:test@localhost:5433/fi_fhir_test?sslmode=disable` | PostgreSQL connection string |
| `TEST_WEBHOOK_URL` | `http://localhost:8888` | HTTP echo destination |
| `TEST_FHIR_URL` | `http://localhost:8090/fhir` | FHIR server base URL (only read by the test parked on issue #20) |
| `TEST_FIFHIR_URL` | `http://localhost:8080` | Running `fi-fhir serve` |
| `TEST_FIFHIR_METRICS_URL` | `http://localhost:9090` | Its metrics listener |

## Known-red tests

Both are skipped, both name the issue they are parked on, and both keep their
repaired bodies so that deleting the `t.Skipf` is the whole of the verification.

| Test | Issue |
|---|---|
| `TestFHIRAction` | [#20](https://gitlab.flexinfer.ai/libs/fi-fhir/-/issues/20) — the `fhir` action's `patient_admit` transaction bundle references `Patient/<MRN>` with no matching `fullUrl`, so a conformant server rejects the whole transaction |
| `TestConfigValidation/invalid_cel` | [#21](https://gitlab.flexinfer.ai/libs/fi-fhir/-/issues/21) — `workflow validate` never compiles the CEL condition, so it accepts an expression the engine cannot run |

## What slice S6-C changed, and why

Every test in both files was red, and had been since they were written. No CI
job passed `-tags=e2e` before slice 4.4c, and that job ran a single test
(`TestObservabilityEndpoints`). `ci/s5b-chaos-dr.yml` recorded the failures and
attributed them to the workflow-schema drift that commit `5d07101c4` corrected
in every document. Repairing every `{{.Patient.Status}}`-style Go dot-path to
the snake_case JSON keys the engine binds flipped **nothing**: the failing set
after the templates-only repair was identical, test for test.

The real causes were four contracts the tests asserted and the CLI never had:

- `parse` emits the event itself, not a `{"events": [...]}` envelope. `hl7v2`
  emits one object, `csv` emits an array.
- `workflow run` has no `--dry-run` flag; dry run is the `workflow dry-run`
  subcommand, and it prints route decisions rather than rendered actions.
- Event input is a JSON array or newline-delimited JSON. A pretty-printed
  multi-line object fails with `failed to parse JSON line`.
- Action config is flat scalars only. `Action.UnmarshalYAML` copies
  string/number/bool fields into a `map[string]string` and silently drops every
  nested block, so `headers:`, `retry:`, `auth:` and `fields:` were never read.

Also retired:

- **`test/e2e/docker-compose.yaml`** and the `e2e-up` / `e2e-down` /
  `test-e2e-full` targets. The file stood up PostgreSQL, HAPI FHIR, Kafka,
  Redis, Jaeger and an echo server — and no `fi-fhir`, which is why
  `TestObservabilityEndpoints` skipped even with the whole stack running. Two
  of its six services had no test at all. The CI runner has no Docker socket,
  so no job could ever have used it. The commands above replace it.
- **The golden-file machinery** (`GoldenDir`, `UPDATE_GOLDEN`,
  `compareOrUpdateGolden`, `make test-golden`). `test/e2e/golden/` never
  existed, so the helper always took its "file does not exist" branch and
  logged; and it could not have worked if the directory had existed, because
  `parse` stamps a fresh UUID, `timestamp` and `received_at` into every event.
  A comparison that cannot fail is the "greener rather than redder" shape
  `AGENTS.md` refuses.

## Adding new tests

1. Put it in `e2e_test.go` if it needs no external service, `integration_test.go`
   otherwise.
2. `ensureBinaryBuilt(t, cfg)` first; `runCLI(cfg, ...)` returns combined
   stdout and stderr, because the CLI reports action failures on stderr.
3. Build events with `createEventFile(t, map[string]interface{}{...})`. It
   marshals and writes NDJSON, which is the only single-event form the CLI
   accepts.
4. Guard a new external dependency with `requireService(t, "<name>", err)` and
   add `<name>` to the job's `FI_FHIR_E2E_REQUIRED_SERVICES`.
5. Never add a `t.Skipf` without a filed issue id in its message. The CI job
   greps for skips and fails on any that does not carry one.
