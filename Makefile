.PHONY: build test clean run lint test-e2e test-integration e2e-up e2e-down

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

# Install golangci-lint and run
lint:
	@which golangci-lint > /dev/null || go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	golangci-lint run

# Build Docker image
docker-build:
	docker build -t fi-fhir:latest .

# Run benchmarks
bench:
	go test -bench=. -benchmem ./internal/workflow/...

# Build and test
all: build test lint
