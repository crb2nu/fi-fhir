# Testing Guidelines

This guide covers testing practices for fi-fhir development.

## Testing Philosophy

1. **Table-driven tests** for comprehensive coverage
2. **Real data samples** over mock data
3. **Warnings over errors** - test that parsers handle edge cases gracefully
4. **Golden files** for output validation

## Test Organization

```
fi-fhir/
├── internal/
│   └── parser/
│       └── hl7v2/
│           ├── parser.go
│           └── parser_test.go      # Unit tests
├── pkg/
│   └── events/
│       ├── events.go
│       └── events_test.go          # Unit tests
├── testdata/                        # Shared test fixtures
│   ├── hl7v2/
│   │   ├── adt_a01_sample.hl7
│   │   └── oru_r01_sample.hl7
│   └── edi/
│       └── 837p_sample.x12
└── test/
    └── e2e/
        ├── e2e_test.go             # E2E tests
        └── integration_test.go      # Integration tests
```

## Unit Tests

### Table-Driven Tests

```go
func TestParser_ParseADT(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        want    events.EventType
        wantMRN string
        wantErr bool
    }{
        {
            name:    "ADT_A01 admit",
            input:   loadTestData("adt_a01_basic.hl7"),
            want:    events.EventPatientAdmit,
            wantMRN: "MRN12345",
        },
        {
            name:    "ADT_A03 discharge",
            input:   loadTestData("adt_a03_basic.hl7"),
            want:    events.EventPatientDischarge,
            wantMRN: "MRN12345",
        },
        {
            name:    "missing PID segment",
            input:   "MSH|^~\\&|...\nPV1|...",
            wantErr: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            p := NewParser(defaultProfile())
            events, err := p.Parse([]byte(tt.input))

            if (err != nil) != tt.wantErr {
                t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
                return
            }

            if !tt.wantErr {
                if len(events) == 0 {
                    t.Fatal("expected at least one event")
                }
                if events[0].Type() != tt.want {
                    t.Errorf("Type() = %v, want %v", events[0].Type(), tt.want)
                }
                // More assertions...
            }
        })
    }
}
```

### Test Helpers

```go
// testdata.go
package hl7v2_test

import (
    "os"
    "path/filepath"
    "testing"
)

func loadTestData(t *testing.T, filename string) []byte {
    t.Helper()
    path := filepath.Join("..", "..", "..", "testdata", "hl7v2", filename)
    data, err := os.ReadFile(path)
    if err != nil {
        t.Fatalf("failed to load test data %s: %v", filename, err)
    }
    return data
}

func defaultProfile() *profile.Profile {
    return &profile.Profile{
        ID:   "test",
        Name: "Test Profile",
        HL7v2: profile.HL7v2Config{
            DefaultVersion: "2.5",
        },
    }
}
```

### Testing Warnings

```go
func TestParser_Warnings(t *testing.T) {
    // Message with missing optional segment
    input := loadTestData(t, "adt_missing_nk1.hl7")

    // Profile tolerates missing NK1
    profile := &profile.Profile{
        HL7v2: profile.HL7v2Config{
            Tolerate: profile.TolerateConfig{
                MissingSegments: []string{"NK1"},
            },
        },
    }

    p := NewParser(profile)
    events, err := p.Parse(input)

    // Should NOT error
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }

    // Should have warning
    if len(events) == 0 {
        t.Fatal("expected event")
    }

    warnings := events[0].Meta().ParseWarnings
    if len(warnings) == 0 {
        t.Error("expected warning for missing NK1")
    }

    found := false
    for _, w := range warnings {
        if w.Code == "MISSING_NK1" {
            found = true
            break
        }
    }
    if !found {
        t.Error("expected MISSING_NK1 warning")
    }
}
```

## Golden File Tests

For complex output validation:

```go
func TestParser_GoldenOutput(t *testing.T) {
    input := loadTestData(t, "complex_message.hl7")

    p := NewParser(defaultProfile())
    events, err := p.Parse(input)
    if err != nil {
        t.Fatal(err)
    }

    got, _ := json.MarshalIndent(events, "", "  ")

    goldenPath := "testdata/golden/complex_message.json"

    if *update {
        // Run with -update to regenerate golden files
        os.WriteFile(goldenPath, got, 0644)
        return
    }

    want, err := os.ReadFile(goldenPath)
    if err != nil {
        t.Fatalf("failed to read golden file: %v", err)
    }

    if !bytes.Equal(got, want) {
        t.Errorf("output differs from golden file\ngot:\n%s\nwant:\n%s", got, want)
    }
}

var update = flag.Bool("update", false, "update golden files")
```

Update golden files with:
```bash
go test -update ./internal/parser/hl7v2/...
```

## Integration Tests

Integration tests use real external services via Docker.

### Setup

```go
// integration_test.go
//go:build integration

package e2e

import (
    "context"
    "testing"

    "github.com/testcontainers/testcontainers-go"
)

func TestFHIRIntegration(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping integration test in short mode")
    }

    // Start FHIR server container
    ctx := context.Background()
    fhirContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
        ContainerRequest: testcontainers.ContainerRequest{
            Image:        "hapiproject/hapi:latest",
            ExposedPorts: []string{"8080/tcp"},
            WaitingFor:   wait.ForHTTP("/fhir/metadata"),
        },
        Started: true,
    })
    if err != nil {
        t.Fatal(err)
    }
    defer fhirContainer.Terminate(ctx)

    endpoint, _ := fhirContainer.Endpoint(ctx, "http")

    // Run test against real FHIR server
    // ...
}
```

### Running Integration Tests

```bash
# Start dependencies
docker-compose up -d

# Run integration tests
go test -tags=integration ./test/e2e/...

# Or use make target
make test-integration
```

### Migration compatibility

`make migration-compatibility` (CI job `test:migration-compatibility`, blocking)
proves the properties the six forward-only migration ledgers have to hold
*together*: two replicas migrating one database concurrently converge, a binary
one version behind still writes, and a `pg_dump`/restore round-trip brings back
every row, every immutability trigger, and the `NOT VALID` provenance CHECK.

```bash
docker --context 7900xtx run --rm -d --name s4c-pg \
  -e POSTGRES_USER=testuser -e POSTGRES_PASSWORD=testpass \
  -e POSTGRES_DB=fi_fhir_migration_compat_test -p 15523:5432 postgres:16
export POSTGRES_TEST_URL="postgres://testuser:testpass@<docker-host>:15523/fi_fhir_migration_compat_test?sslmode=disable"

# The round-trip subtest shells out to scripts/pgdump-roundtrip.sh, which needs
# client tools whose MAJOR version matches the server. pg_dump 17+ writes
# `SET transaction_timeout = 0` into the archive and PostgreSQL 16 rejects it,
# so a newer client silently produces an unrestorable dump. On macOS:
#   brew install postgresql@16
export FI_FHIR_PG_BIN_DIR=/opt/homebrew/opt/postgresql@16/bin

make migration-compatibility
```

Each proof provisions its **own database**, not a `search_path` schema.
`pkg/terminology/db` creates a PostgreSQL schema literally named `terminology`,
which is database-wide; sharing a database silently leaves later proofs running
against a ledger an earlier one already migrated, and "two replicas against a
fresh database" stops testing the fresh-install path.

The job also runs `TestMigrationCompatibility_NegativeControls`, which removes
each fix in turn and asserts the corresponding assertion goes red. A control
that *passes* fails the job: it means the proof is not exercising the mechanism.

### Writing a migration

See [AGENTS.md](../../AGENTS.md) "Migration authoring". The short version: a new
`NOT NULL` column on an existing table carries a `DEFAULT` or it breaks
one-version rollback, every migrator takes its advisory lock and re-reads its
version inside it, and the migration number comes from the ledger at rebase
rather than from a planning document.
`TestMigrationRule_NotNullOnExistingColumnCarriesADefault` enforces the first
rule in `test:unit` and needs no database.

## E2E Tests

End-to-end tests verify the full CLI workflow:

```go
// e2e_test.go
package e2e

import (
    "bytes"
    "os/exec"
    "testing"
)

func TestCLI_Parse(t *testing.T) {
    cmd := exec.Command("./bin/fi-fhir", "parse",
        "--format", "hl7v2",
        "--pretty",
        "testdata/hl7v2/adt_a01_sample.hl7",
    )

    var stdout, stderr bytes.Buffer
    cmd.Stdout = &stdout
    cmd.Stderr = &stderr

    err := cmd.Run()
    if err != nil {
        t.Fatalf("command failed: %v\nstderr: %s", err, stderr.String())
    }

    output := stdout.String()

    // Verify output contains expected data
    if !strings.Contains(output, `"type": "patient_admit"`) {
        t.Errorf("output missing event type:\n%s", output)
    }
}

func TestCLI_WorkflowDryRun(t *testing.T) {
    // Parse message, pipe to workflow
    parseCmd := exec.Command("./bin/fi-fhir", "parse",
        "--format", "hl7v2",
        "testdata/hl7v2/adt_a01_sample.hl7",
    )

    workflowCmd := exec.Command("./bin/fi-fhir", "workflow", "run",
        "--dry-run",
        "--config", "testdata/workflows/test_workflow.yaml",
    )

    // Pipe parse output to workflow input
    pipe, _ := parseCmd.StdoutPipe()
    workflowCmd.Stdin = pipe

    var stdout bytes.Buffer
    workflowCmd.Stdout = &stdout

    parseCmd.Start()
    workflowCmd.Start()

    parseCmd.Wait()
    workflowCmd.Wait()

    // Verify dry-run output
    // ...
}
```

## Benchmarks

```go
func BenchmarkParser_Parse(b *testing.B) {
    input := loadBenchData("large_message.hl7")
    profile := defaultProfile()

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        p := NewParser(profile)
        _, err := p.Parse(input)
        if err != nil {
            b.Fatal(err)
        }
    }
}

func BenchmarkParser_ParseParallel(b *testing.B) {
    input := loadBenchData("large_message.hl7")
    profile := defaultProfile()

    b.RunParallel(func(pb *testing.PB) {
        for pb.Next() {
            p := NewParser(profile)
            _, err := p.Parse(input)
            if err != nil {
                b.Fatal(err)
            }
        }
    })
}
```

Run benchmarks:
```bash
go test -bench=. -benchmem ./internal/parser/hl7v2/...
```

## Test Coverage

### Generate Coverage Report

```bash
# Run with coverage
go test -coverprofile=coverage.out ./...

# View coverage
go tool cover -html=coverage.out

# Coverage summary
go tool cover -func=coverage.out
```

### Coverage Targets

| Package | Target |
|---------|--------|
| pkg/* | 80%+ |
| internal/parser/* | 85%+ |
| internal/workflow/* | 80%+ |

## Test Data

### Organizing Test Data

```
testdata/
├── hl7v2/
│   ├── adt/
│   │   ├── a01_basic.hl7
│   │   ├── a01_with_z_segments.hl7
│   │   └── a01_missing_nk1.hl7
│   ├── oru/
│   │   └── r01_multi_obx.hl7
│   └── siu/
│       └── s12_appointment.hl7
├── edi/
│   ├── 837p_sample.x12
│   └── 835_remit.x12
├── profiles/
│   ├── default.yaml
│   └── strict.yaml
├── workflows/
│   └── test_workflow.yaml
└── golden/
    ├── adt_a01_output.json
    └── oru_r01_output.json
```

### Creating Test Data

- Use real-world samples when possible (sanitized)
- Include edge cases (missing fields, extra data)
- Document what each sample tests

## CI Integration

### GitLab CI

```yaml
test:unit:
  stage: test
  script:
    - go test -v -race -coverprofile=coverage.out ./...
  artifacts:
    reports:
      coverage_report:
        coverage_format: cobertura
        path: coverage.xml

test:integration:
  stage: test
  services:
    - name: postgres:16-alpine
      alias: postgres
    - name: minio/minio:latest
      alias: minio
      # Required: the image's default CMD prints usage and exits.
      command: ["server", "/data", "--console-address", ":9001"]
  script:
    - go test -tags=integration ./cmd/fi-fhir/...
    # Serialized: both paths reset the shared terminology schema.
    - go test -tags=integration -p 1 ./pkg/terminology/db/
  allow_failure: false # blocking merge gate since 2026-08-08
```

> `test:integration` and `lint:docs` are blocking merge gates.
> `test:docs-status` is still advisory. See `.loom/40-decisions.md` (2026-08-08)
> for the soft-fail policy and per-job promotion criteria.
>
> `setupTestInfra()` **skips** rather than fails when Postgres or MinIO is
> unreachable, so verify the skip count when debugging a suspiciously fast green
> run — a broken service container makes this job pass with less coverage.

## Common Patterns

### Testing Error Cases

```go
func TestParser_Errors(t *testing.T) {
    tests := []struct {
        name  string
        input string
        want  string // Expected error substring
    }{
        {
            name:  "empty input",
            input: "",
            want:  "empty message",
        },
        {
            name:  "invalid header",
            input: "NOT_VALID_HL7",
            want:  "invalid MSH segment",
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            p := NewParser(defaultProfile())
            _, err := p.Parse([]byte(tt.input))

            if err == nil {
                t.Fatal("expected error")
            }

            if !strings.Contains(err.Error(), tt.want) {
                t.Errorf("error = %q, want to contain %q", err, tt.want)
            }
        })
    }
}
```

### Testing with Profiles

```go
func TestParser_WithDifferentProfiles(t *testing.T) {
    input := loadTestData(t, "message_with_optional.hl7")

    tests := []struct {
        name        string
        profile     *profile.Profile
        wantErr     bool
        wantWarning bool
    }{
        {
            name:    "strict profile fails",
            profile: strictProfile(),
            wantErr: true,
        },
        {
            name:        "tolerant profile succeeds with warning",
            profile:     tolerantProfile(),
            wantErr:     false,
            wantWarning: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            p := NewParser(tt.profile)
            events, err := p.Parse(input)

            if (err != nil) != tt.wantErr {
                t.Errorf("error = %v, wantErr %v", err, tt.wantErr)
            }

            if tt.wantWarning && len(events) > 0 {
                if len(events[0].Meta().ParseWarnings) == 0 {
                    t.Error("expected warnings")
                }
            }
        })
    }
}
```

## See Also

- [Development Setup](development-setup.md) - Environment setup
- [Adding a Parser](adding-parser.md) - Parser development
- [Architecture Overview](architecture.md) - System architecture
