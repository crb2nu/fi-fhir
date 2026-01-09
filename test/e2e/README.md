# End-to-End Tests

This directory contains end-to-end integration tests for fi-fhir.

## Test Categories

### Unit E2E Tests (`-tags=e2e`)

Tests that run against the CLI binary without external dependencies:

- Message parsing (HL7v2, CSV, EDI)
- Workflow dry-run execution
- Configuration validation
- CEL filter evaluation
- Transform operations

```bash
# Run unit E2E tests
go test -tags=e2e -v ./test/e2e/...
```

### Integration Tests (`-tags=e2e,integration`)

Tests that require external services (PostgreSQL, FHIR server, Kafka):

- Database action with PostgreSQL
- FHIR action with HAPI FHIR server
- Webhook action with mock server
- Queue action with Kafka
- Retry and circuit breaker behavior

```bash
# Start dependencies
docker-compose -f test/e2e/docker-compose.yaml up -d

# Wait for services to be healthy
docker-compose -f test/e2e/docker-compose.yaml ps

# Run integration tests
go test -tags=e2e,integration -v ./test/e2e/...

# Stop dependencies
docker-compose -f test/e2e/docker-compose.yaml down
```

## Golden Files

Some tests use golden files to compare output against known-good baselines.
Golden files are stored in `test/e2e/golden/`.

To update golden files when output format changes:

```bash
UPDATE_GOLDEN=1 go test -tags=e2e -v ./test/e2e/...
```

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `UPDATE_GOLDEN` | `0` | Set to `1` to update golden files |
| `TEST_POSTGRES_URL` | `postgres://test:test@localhost:5433/fi_fhir_test?sslmode=disable` | PostgreSQL connection string |
| `TEST_FHIR_URL` | `http://localhost:8090/fhir` | HAPI FHIR server URL |
| `TEST_KAFKA_BROKERS` | `localhost:9094` | Kafka broker addresses |
| `TEST_WEBHOOK_URL` | `http://localhost:8888` | Mock webhook server URL |
| `TEST_JAEGER_URL` | `http://localhost:16687` | Jaeger UI URL |
| `TEST_FIFHIR_URL` | `http://localhost:8080` | fi-fhir server URL (for health/metrics tests) |

## Docker Compose Services

| Service | Port | Description |
|---------|------|-------------|
| `postgres` | 5433 | PostgreSQL 15 for database action testing |
| `fhir` | 8090 | HAPI FHIR R4 server |
| `kafka` | 9094 | Kafka 3.6 (KRaft mode) |
| `redis` | 6380 | Redis 7 (optional caching) |
| `jaeger` | 16687 | Jaeger all-in-one for tracing |
| `webhook` | 8888 | HTTP echo server for webhook testing |

## Running in CI

The GitLab CI pipeline runs E2E tests automatically:

```yaml
test:e2e:
  stage: test
  services:
    - postgres:15-alpine
    - hapiproject/hapi:latest
  script:
    - go test -tags=e2e,integration -v ./test/e2e/...
```

## Adding New Tests

1. Add test functions to `e2e_test.go` (no external deps) or `integration_test.go` (with external deps)
2. Use the `ensureBinaryBuilt()` helper to ensure CLI is compiled
3. Use `createTempFile()` to create test input files
4. Use `runCLI()` to execute CLI commands
5. For golden file tests, use `compareOrUpdateGolden()`

Example:

```go
func TestMyNewFeature(t *testing.T) {
    cfg := DefaultConfig()
    ensureBinaryBuilt(t, cfg)

    // Create test input
    input := `{"type": "patient_admit", ...}`
    inputFile := createTempFile(t, input, ".json")
    defer os.Remove(inputFile)

    // Run CLI command
    output, err := runCLI(cfg, "parse", "--format", "json", inputFile)
    if err != nil {
        t.Fatalf("command failed: %v", err)
    }

    // Verify output
    if !strings.Contains(output, "expected_value") {
        t.Errorf("unexpected output: %s", output)
    }
}
```

## Troubleshooting

### Tests skip with "binary not found"

Build the binary first:

```bash
go build -o bin/fi-fhir ./cmd/fi-fhir
```

### Integration tests skip with "service not available"

Start the Docker Compose services:

```bash
docker-compose -f test/e2e/docker-compose.yaml up -d
docker-compose -f test/e2e/docker-compose.yaml ps  # Check health
```

### FHIR tests fail with timeout

HAPI FHIR takes 60-90 seconds to start. Wait for it:

```bash
docker-compose -f test/e2e/docker-compose.yaml logs -f fhir
# Wait for "Started Application in X seconds"
```

### Database tests fail with connection refused

Check PostgreSQL is running on port 5433:

```bash
docker-compose -f test/e2e/docker-compose.yaml logs postgres
psql "postgres://test:test@localhost:5433/fi_fhir_test?sslmode=disable" -c "SELECT 1"
```
