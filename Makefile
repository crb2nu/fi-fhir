.PHONY: build test clean run lint lint-fix test-e2e test-integration e2e-up e2e-down fmt setup-hooks dev-setup check-deps

# Tool versions (update these when upgrading)
GOLANGCI_LINT_VERSION := v2.1.6
GO_MIN_VERSION := 1.21

# Build the CLI
build:
	go build -o bin/fi-fhir ./cmd/fi-fhir

# Run all tests
test:
	go test ./...

# Run tests with verbose output
test-v:
	go test -v ./...

# Run tests with coverage
test-cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# Run E2E tests (no external deps required)
test-e2e: build
	go test -tags=e2e -v ./test/e2e/...

# Run integration tests (requires Docker services)
test-integration: build
	go test -tags=e2e,integration -v ./test/e2e/...

# Start E2E test dependencies
e2e-up:
	docker-compose -f test/e2e/docker-compose.yaml up -d
	@echo "Waiting for services to be healthy..."
	@sleep 10
	docker-compose -f test/e2e/docker-compose.yaml ps

# Stop E2E test dependencies
e2e-down:
	docker-compose -f test/e2e/docker-compose.yaml down -v

# Run full E2E test suite with Docker dependencies
test-e2e-full: build e2e-up
	@echo "Waiting for FHIR server to start (may take 60-90s)..."
	@sleep 60
	go test -tags=e2e,integration -v ./test/e2e/...
	$(MAKE) e2e-down

# Update golden files
test-golden: build
	UPDATE_GOLDEN=1 go test -tags=e2e -v ./test/e2e/...

# Clean build artifacts
clean:
	rm -rf bin/ coverage.out coverage.html

# Run the CLI (example)
run:
	./bin/fi-fhir parse --pretty testdata/adt_a01_sample.hl7

# Tidy dependencies
tidy:
	go mod tidy

# Run linter using 'go run' for reliability (no PATH issues)
lint:
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION) run

# Run linter with auto-fix
lint-fix:
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION) run --fix

# Install linter to $GOPATH/bin (for IDE integration)
install-lint:
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
	@echo "Installed golangci-lint $(GOLANGCI_LINT_VERSION) to $$(go env GOPATH)/bin"
	@echo "Make sure $$(go env GOPATH)/bin is in your PATH for IDE integration"

# Build Docker image
docker-build:
	docker build -t fi-fhir:latest .

# Run benchmarks
bench:
	go test -bench=. -benchmem ./internal/workflow/...

# Format Go code
fmt:
	go fmt ./...

# Check formatting without modifying files
fmt-check:
	@echo "Checking formatting..."
	@unformatted=$$(gofmt -l cmd internal pkg sdk 2>/dev/null); \
	if [ -n "$$unformatted" ]; then \
		echo "The following files need formatting:"; \
		echo "$$unformatted"; \
		exit 1; \
	fi
	@echo "All files are properly formatted."

# Setup git hooks for pre-commit checks
setup-hooks:
	git config core.hooksPath .githooks
	@echo "Git hooks configured to use .githooks directory"

# Build and test
all: build test lint

# =============================================================================
# Development Setup
# =============================================================================

# Full development environment setup
dev-setup: check-deps setup-hooks tidy
	@echo ""
	@echo "✅ Development environment ready!"
	@echo ""
	@echo "Quick reference:"
	@echo "  make lint         - Run linter"
	@echo "  make lint-fix     - Run linter with auto-fix"
	@echo "  make test         - Run all tests"
	@echo "  make test-v       - Run tests with verbose output"
	@echo "  make test-cover   - Run tests with coverage report"
	@echo "  make build        - Build CLI binary"
	@echo "  make bench        - Run benchmarks"
	@echo "  make check        - Run lint + test"
	@echo "  make ci           - Simulate CI locally"
	@echo ""
	@echo "For IDE integration, also run: make install-lint"
	@echo ""

# Check development dependencies
check-deps:
	@echo "Checking Go version..."
	@go version | grep -q "go1\.\(2[1-9]\|[3-9][0-9]\)" || { \
		echo "❌ Go $(GO_MIN_VERSION)+ required. Current: $$(go version)"; \
		exit 1; \
	}
	@echo "✓ Go version OK"
	@echo ""
	@echo "Checking required tools..."
	@command -v git >/dev/null 2>&1 || { echo "❌ git not found"; exit 1; }
	@echo "✓ git"
	@command -v docker >/dev/null 2>&1 && echo "✓ docker (optional, for e2e tests)" || echo "⚠ docker not found (optional, needed for e2e tests)"
	@echo ""

# Quick check: lint and test in one command
check: lint test
	@echo "✅ All checks passed!"

# Verify CI will pass locally
ci: fmt-check lint test
	@echo "✅ CI checks passed locally!"
