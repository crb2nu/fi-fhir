# Development Setup

This guide walks through setting up a development environment for fi-fhir.

## Prerequisites

### Required

- **Go 1.21+**: https://golang.org/dl/
- **Git**: https://git-scm.com/

### Optional (for full development)

- **Docker**: For integration tests
- **Node.js 18+**: For TypeScript SDK development
- **PostgreSQL 14+**: For event store development
- **golangci-lint**: For linting

## Quick Setup

```bash
# Clone repository
git clone https://gitlab.flexinfer.ai/libs/fi-fhir.git
cd fi-fhir

# Install dependencies and build
make dev-setup

# Verify setup
./bin/fi-fhir --version
make test
```

## Detailed Setup

### 1. Install Go

```bash
# macOS
brew install go

# Linux
wget https://go.dev/dl/go1.21.0.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.21.0.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin

# Verify
go version
```

### 2. Clone and Build

```bash
# Clone
git clone https://gitlab.flexinfer.ai/libs/fi-fhir.git
cd fi-fhir

# Build CLI
make build

# Run tests
make test
```

### 3. Install Development Tools

```bash
# golangci-lint
brew install golangci-lint
# or
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# gofumpt (stricter gofmt)
go install mvdan.cc/gofumpt@latest
```

### 4. Set Up Git Hooks

```bash
make setup-hooks
```

This installs pre-commit hooks for:
- Code formatting
- Linting
- Test execution

## Project Structure

After cloning, you'll see:

```
fi-fhir/
├── bin/                  # Built binaries
├── cmd/fi-fhir/          # CLI source
├── internal/             # Private packages
├── pkg/                  # Public packages
├── testdata/             # Test fixtures
├── sdk/typescript/       # TypeScript SDK
├── ui/                   # SvelteKit UI
├── Makefile              # Build commands
├── go.mod                # Go dependencies
└── .golangci.yml         # Linter config
```

## Makefile Targets

```bash
# Building
make build            # Build CLI
make docker-build     # Build Docker image

# Testing
make test             # Run unit tests
make test-v           # Verbose tests
make test-cover       # Coverage report
make test-e2e         # E2E tests
make test-integration # Integration tests (requires Docker)

# Code quality
make lint             # Run linter
make lint-fix         # Auto-fix lint issues
make fmt-check        # Check formatting

# Development
make dev-setup        # Full dev environment setup
make bench            # Run benchmarks
make test-golden      # Update golden files
```

## Running the CLI

### Parse a Message

```bash
# Sample HL7v2 message
./bin/fi-fhir parse --format hl7v2 --pretty testdata/adt_a01_sample.hl7

# With verbose output
./bin/fi-fhir parse --format hl7v2 --verbose testdata/adt_a01_sample.hl7
```

### Run Tests

```bash
# All tests
go test ./...

# Specific package
go test -v ./internal/parser/hl7v2/...

# Single test
go test -v -run TestParseADTA01 ./internal/parser/hl7v2/
```

## Docker Development

### Start Local Stack

```bash
# Start PostgreSQL, Kafka, FHIR server, Jaeger
docker-compose up -d

# Check status
docker-compose ps

# View logs
docker-compose logs -f
```

### Services

| Service | Port | Description |
|---------|------|-------------|
| PostgreSQL | 5432 | Event store, profile store, workflow lifecycle |
| Kafka | 9092 | Message queue |
| HAPI FHIR | 8090 | FHIR R4 server |
| Qdrant | 6333 | Vector search for terminology semantic index |
| Temporal | 7233 | Workflow orchestration for terminology mapping |
| Jaeger | 16686 | Tracing UI |

### Quick Development with `make dev`

The `make dev` target starts infrastructure, waits for readiness, and launches
the server with PostgreSQL persistence in a single command:

```bash
# Start everything
export FI_FHIR_GRAPHQL_BEARER_TOKEN="$(openssl rand -hex 32)"
make dev

# Expected output:
# docker-compose up -d postgres qdrant kafka
# Waiting for postgres...
# Profile store: PostgreSQL
# Event store: PostgreSQL
# Workflow lifecycle store: PostgreSQL
# Authenticated bounded GraphQL POST available at http://localhost:8081/graphql

# When done
make dev-down
```

### Run Integration Tests

```bash
# Start dependencies
docker-compose up -d

# Run integration tests
make test-integration

# Clean up
docker-compose down
```

## TypeScript SDK Development

### Setup

```bash
cd sdk/typescript

# Install dependencies
npm install

# Build
npm run build

# Test
npm test
```

### Watch Mode

```bash
npm run dev
```

## UI Development

### Setup

```bash
cd ui

# Install dependencies
npm install

# Start dev server
npm run dev
```

The UI is available at http://localhost:5173

### Connect to API

```bash
# Start the API server
export FI_FHIR_DEPLOYMENT_TENANT_ID=tenant-a
export FI_FHIR_GRAPHQL_BEARER_TOKEN="$(openssl rand -hex 32)"
export FI_FHIR_GRAPHQL_PRINCIPAL_ID=local-operator
export FI_FHIR_GRAPHQL_ROLES=integration:preview
export FI_FHIR_GRAPHQL_ALLOWED_ORIGINS=http://localhost:5173
export FI_FHIR_INTEGRATION_REGISTRY_PATH="$(git rev-parse --show-toplevel)/testdata/golden/integration/adt-http/preview-registry.json"
./bin/fi-fhir serve --port 8081 --no-playground --no-introspection

# Configure UI
VITE_API_ORIGIN=http://localhost:8081 \
VITE_FI_FHIR_PREVIEW_INTEGRATION_ID=adt-east \
npm run dev
```

## IDE Setup

### VS Code

Recommended extensions:
- Go (official)
- golangci-lint
- EditorConfig

Settings (`.vscode/settings.json`):
```json
{
  "go.lintTool": "golangci-lint",
  "go.lintFlags": ["--fast"],
  "editor.formatOnSave": true,
  "[go]": {
    "editor.defaultFormatter": "golang.go"
  }
}
```

### GoLand / IntelliJ

- Enable golangci-lint integration
- Configure Go modules
- Set test flags: `-race -v`

## Debugging

### Delve Debugger

```bash
# Install delve
go install github.com/go-delve/delve/cmd/dlv@latest

# Debug CLI
dlv debug ./cmd/fi-fhir -- parse --format hl7v2 testdata/sample.hl7

# Debug test
dlv test ./internal/parser/hl7v2/ -- -test.run TestParseADTA01
```

### VS Code Launch Configuration

```json
{
  "version": "0.2.0",
  "configurations": [
    {
      "name": "Parse Command",
      "type": "go",
      "request": "launch",
      "mode": "debug",
      "program": "${workspaceFolder}/cmd/fi-fhir",
      "args": ["parse", "--format", "hl7v2", "testdata/adt_a01_sample.hl7"]
    }
  ]
}
```

## Common Issues

### "go: module not found"

```bash
# Ensure you're in the project root
pwd  # Should show fi-fhir directory

# Tidy modules
go mod tidy
```

### "permission denied" on bin/fi-fhir

```bash
chmod +x bin/fi-fhir
```

### Test failures with "timeout"

Some tests require Docker services:

```bash
# Start Docker services
docker-compose up -d

# Wait for services to be ready
sleep 10

# Re-run tests
make test-integration
```

### Lint errors

```bash
# Auto-fix what's possible
make lint-fix

# Manual fixes needed for remaining issues
make lint
```

## Environment Variables

For development, copy `.env.example` to `.env` and customize. Key variables:

```bash
# Database (enables persistent stores)
export FI_FHIR_DATABASE_DRIVER=postgres
export FI_FHIR_DATABASE_HOST=localhost
export FI_FHIR_DATABASE_PORT=5432
export FI_FHIR_DATABASE_NAME=fi_fhir
export FI_FHIR_DATABASE_USERNAME=fi_fhir
export FI_FHIR_DATABASE_PASSWORD=fi_fhir_dev
export FI_FHIR_DATABASE_SSL_MODE=disable

# Server
export FI_FHIR_SERVER_PORT=8081

# Workflow config (alternative to --workflow flag)
export FI_FHIR_WORKFLOW_CONFIG_PATH=./configs/adt-workflow.yaml

# LLM (for AI-powered terminology features)
export LLM_BASE_URL=http://localhost:11434/v1
export LLM_DEFAULT_MODEL=llama3.2

# Vector search
export QDRANT_URL=http://localhost:6333

# Embedding
export EMBEDDING_BASE_URL=http://localhost:11434/v1
export EMBEDDING_MODEL=nomic-embed-text

# Temporal
export TEMPORAL_ADDRESS=localhost:7233

# Debug logging
export FI_FHIR_LOG_LEVEL=debug
```

See `.env.example` for the complete list, or `configs/full-stack.env` for a
configuration matching the k3s production deployment (with sanitized secrets).

## Next Steps

- [Architecture Overview](architecture.md) - Understand the codebase
- [Adding a Parser](adding-parser.md) - Add new format support
- [Testing Guidelines](testing.md) - Write effective tests
