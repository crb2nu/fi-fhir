# fi-fhir Developer Guide

This guide is for developers contributing to fi-fhir or building extensions and integrations.

## Table of Contents

1. [Architecture Overview](architecture.md)
2. [Development Setup](development-setup.md)
3. [Adding a Format Parser](adding-parser.md)
4. [Adding Event Types](adding-events.md)
5. [Workflow Action Development](adding-actions.md)
6. [Testing Guidelines](testing.md)
7. [TypeScript SDK Development](typescript-sdk.md)

## Quick Links

| I want to... | Read... |
|--------------|---------|
| Set up my development environment | [Development Setup](development-setup.md) |
| Understand the architecture | [Architecture Overview](architecture.md) |
| Add support for a new format | [Adding a Format Parser](adding-parser.md) |
| Create a new event type | [Adding Event Types](adding-events.md) |
| Build a custom workflow action | [Adding Actions](adding-actions.md) |
| Write effective tests | [Testing Guidelines](testing.md) |
| Contribute to the TypeScript SDK | [TypeScript SDK Development](typescript-sdk.md) |

## Project Structure

```
fi-fhir/
├── cmd/fi-fhir/              # CLI entry point
│   └── main.go               # Command definitions
├── internal/                  # Private packages
│   ├── api/graphql/          # GraphQL API layer
│   ├── parser/               # Format parsers
│   │   ├── hl7v2/            # HL7v2 parser
│   │   ├── csv/              # CSV parser
│   │   ├── edi/              # EDI X12 parser
│   │   ├── cda/              # CDA/CCDA parser
│   │   └── fhir/             # FHIR parser
│   ├── fhir/subscription/    # FHIR subscriptions
│   ├── workflow/             # Workflow engine
│   └── semantic/             # Event transformation
├── pkg/                       # Public packages
│   ├── events/               # Canonical event types
│   ├── profile/              # Source profile system
│   ├── config/               # Configuration
│   ├── fhir/                 # FHIR mapping
│   ├── validate/             # Identifier validators
│   ├── eventsourcing/        # Event store, projections
│   ├── matching/             # Patient matching
│   └── terminology/          # Code system mapping
├── api/                       # OpenAPI specification
├── sdk/typescript/            # TypeScript SDK
├── ui/                        # SvelteKit Mapping Studio
├── deploy/                    # Deployment configs
│   ├── helm/                 # Helm chart
│   └── kubernetes/           # Kustomize manifests
├── dashboards/                # Grafana dashboards
├── examples/                  # Example workflows
├── profiles/                  # Profile templates
├── testdata/                  # Test data
└── test/e2e/                  # End-to-end tests
```

## Key Design Decisions

### 1. internal/ vs pkg/

- **pkg/**: Public API - stable, documented, intended for external use
- **internal/**: Implementation details - can change without notice

### 2. Dependency Injection

The codebase uses explicit dependency injection rather than globals:

```go
// Good: Explicit dependencies
type Parser struct {
    profile  *profile.Profile
    logger   Logger
    metrics  Metrics
}

// Bad: Global state
var globalLogger = log.Default()
```

### 3. Error Handling

Use wrapped errors with context:

```go
if err := parser.Parse(msg); err != nil {
    return fmt.Errorf("parsing message %s: %w", msg.ID, err)
}
```

### 4. Warnings Over Errors

Healthcare data is messy. Record warnings, don't fail:

```go
// Good: Continue with warning
if field == "" {
    p.addWarning("MISSING_FIELD", path)
    return defaultValue, nil
}

// Bad: Fail on recoverable issue
if field == "" {
    return nil, errors.New("missing field")
}
```

## Development Workflow

### 1. Fork and Clone

```bash
git clone https://gitlab.flexinfer.ai/libs/fi-fhir.git
cd fi-fhir
```

### 2. Install Dependencies

```bash
make dev-setup
```

### 3. Make Changes

```bash
# Create branch
git checkout -b feature/my-feature

# Make changes...

# Run tests
make test

# Run linter
make lint
```

### 4. Submit Changes

```bash
git commit -m "feat: add my feature"
git push origin feature/my-feature
# Create merge request
```

## Code Style

### Go Style

- Follow [Effective Go](https://golang.org/doc/effective_go)
- Run `gofmt` and `golangci-lint`
- Table-driven tests
- Meaningful variable names

### Commit Messages

Follow [Conventional Commits](https://www.conventionalcommits.org/):

```
feat: add MDM message parsing
fix: handle missing PV1 segment
docs: update workflow DSL reference
test: add integration tests for EDI parser
refactor: simplify identifier extraction
```

## See Also

- [AGENTS.md](../../AGENTS.md) - AI assistant guidance (also useful for humans)
- [User Guide](../user-guide/README.md) - End-user documentation
- [Planning Documents](../planning/README.md) - Technical specifications
