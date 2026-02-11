.PHONY: build test clean run lint lint-fix test-e2e test-integration e2e-up e2e-down fmt setup-hooks dev-setup check-deps docs-mermaid \
	vet lint-gqlgen lint-ui test-ui test-race \
	security-vulncheck security-gosec security-npm-audit \
	build-release docker-build-ui \
	ci-lint ci-test ci-security ci-build ci-full ci-quick \
	docker-push docker-push-ui docker-push-all \
	deploy deploy-ui deploy-all deploy-status deploy-logs deploy-delete deploy-forward \
	docs-status docs-status-quick docs-validate docs-all

# Tool versions (update these when upgrading)
GOLANGCI_LINT_VERSION := v2.8.0
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

# Coverage with GOCOVERDIR (Go 1.20+) — avoids lib/pq goroutine leak issue
test-cover-unit:
	@rm -rf coverage/unit && mkdir -p coverage/unit
	go test ./... -cover -args -test.gocoverdir=$(CURDIR)/coverage/unit
	@go tool covdata percent -i=coverage/unit

test-cover-integration:
	@rm -rf coverage/integration && mkdir -p coverage/integration
	go test -tags=integration ./cmd/fi-fhir/... ./pkg/terminology/db/... ./pkg/eventsourcing/... -cover -timeout=300s \
		-args -test.gocoverdir=$(CURDIR)/coverage/integration
	@go tool covdata percent -i=coverage/integration

test-cover-all: test-cover-unit test-cover-integration
	@mkdir -p coverage/merged
	go tool covdata merge -i=coverage/unit,coverage/integration -o=coverage/merged
	go tool covdata percent -i=coverage/merged
	go tool covdata textfmt -i=coverage/merged -o=coverage.out
	@echo "Coverage report: coverage.out"

test-cover-html: test-cover-all
	go tool cover -html=coverage.out -o coverage.html
	@echo "HTML report: coverage.html"

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
	@echo "  make build         - Build CLI binary"
	@echo "  make test          - Run all tests"
	@echo "  make lint          - Run linter"
	@echo "  make check         - Run lint + test"
	@echo ""
	@echo "CI simulation:"
	@echo "  make ci-quick      - Quick CI (fmt, vet, lint, test)"
	@echo "  make ci-lint       - Full lint stage (fmt, vet, lint, gqlgen, ui)"
	@echo "  make ci-test       - Full test stage (race, coverage, ui tests)"
	@echo "  make ci-security   - Security scans (govulncheck, gosec, npm audit)"
	@echo "  make ci-build      - Build binaries + Docker images"
	@echo "  make ci-full       - Run entire CI pipeline locally"
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

# Install lint/format tools locally
install-tools:
	@echo "Installing golangci-lint..."
	@GOBIN=$(shell go env GOPATH)/bin go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.61.0
	@echo "Installing staticcheck..."
	@GOBIN=$(shell go env GOPATH)/bin go install honnef.co/go/tools/cmd/staticcheck@latest
	@echo "Installing gci (import formatter)..."
	@GOBIN=$(shell go env GOPATH)/bin go install github.com/daixiang0/gci@latest
	@echo "Tools installed."

# Quick check: lint and test in one command
check: lint test
	@echo "✅ All checks passed!"

# Verify CI will pass locally
ci: fmt-check lint test
	@echo "✅ CI checks passed locally!"

# =============================================================================
# CI Simulation (mimics GitLab CI pipeline locally)
# =============================================================================

# Version for build info
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
SHA ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

# Run go vet (matches lint:vet in CI)
vet:
	go vet ./...

# Regenerate and check gqlgen (matches lint:gqlgen in CI)
lint-gqlgen:
	@echo "Checking GraphQL codegen..."
	cd internal/api/graphql && go run github.com/99designs/gqlgen generate --config gqlgen.yml
	@if ! git diff --quiet -- internal/api/graphql/; then \
		echo "❌ GraphQL generated code is out of sync. Run 'make lint-gqlgen' and commit."; \
		git diff -- internal/api/graphql/; \
		exit 1; \
	fi
	@echo "✓ GraphQL codegen is up to date"

# Run UI lint and checks (matches lint:ui in CI)
lint-ui:
	@echo "Running UI lint and type checks..."
	cd ui && npm install --no-audit --no-fund
	cd ui && npm run lint
	cd ui && npm run codegen:check
	cd ui && npm run check
	cd ui && npm run typecheck
	@echo "✓ UI lint passed"

# Run UI tests
test-ui:
	@echo "Running UI tests..."
	cd ui && npm install --no-audit --no-fund
	cd ui && npm test
	@echo "✓ UI tests passed"

# Run unit tests with race detection and coverage (matches test:unit in CI)
test-race:
	CGO_ENABLED=1 go test -race -coverprofile=coverage.out -covermode=atomic ./...
	go tool cover -func=coverage.out

# Security: govulncheck (matches security:govulncheck in CI)
security-vulncheck:
	@echo "Running govulncheck..."
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...
	@echo "✓ No known vulnerabilities found"

# Security: gosec (matches security:gosec in CI)
security-gosec:
	@echo "Running gosec..."
	go run github.com/securego/gosec/v2/cmd/gosec@latest -fmt text -exclude-generated ./cmd/... ./internal/... ./pkg/... ./sdk/... || true
	@echo "✓ gosec scan complete"

# Security: npm audit (matches security:npm-audit in CI)
security-npm-audit:
	@echo "Running npm audit..."
	cd ui && npm audit --audit-level=high || true
	@echo "✓ npm audit complete"

# Build with version info (matches build:binary in CI)
build-release:
	go build -ldflags="-s -w -X main.version=$(VERSION)" -o bin/fi-fhir ./cmd/fi-fhir
	./bin/fi-fhir version

# Build UI Docker image (matches build:docker-ui in CI)
docker-build-ui:
	docker build \
		--build-arg VITE_BUILD_SHA=$(SHA) \
		--build-arg VITE_BUILD_TAG=local-$(VERSION) \
		--build-arg VITE_BUILD_TIME=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ") \
		-t fi-fhir-ui:latest \
		./ui

# Full lint stage (matches CI lint stage)
ci-lint: fmt-check vet lint lint-gqlgen lint-ui
	@echo ""
	@echo "✅ All lint checks passed!"

# Full test stage (matches CI test stage)
ci-test: test-race test-ui
	@echo ""
	@echo "✅ All tests passed!"

# Full security stage (matches CI security stage)
ci-security: security-vulncheck security-gosec security-npm-audit
	@echo ""
	@echo "✅ Security scans complete!"

# Full build stage (matches CI build stage)
ci-build: build-release docker-build docker-build-ui
	@echo ""
	@echo "✅ All builds complete!"
	@echo "  - bin/fi-fhir (CLI binary)"
	@echo "  - fi-fhir:latest (Docker image)"
	@echo "  - fi-fhir-ui:latest (UI Docker image)"

# Run full CI pipeline locally (all stages)
ci-full: ci-lint ci-test ci-security ci-build
	@echo ""
	@echo "════════════════════════════════════════════════════════════════"
	@echo "✅ Full CI pipeline passed locally!"
	@echo "════════════════════════════════════════════════════════════════"

# Quick CI check (lint + test only, skip security scans and docker builds)
ci-quick: fmt-check vet lint test
	@echo ""
	@echo "✅ Quick CI checks passed!"

# =============================================================================
# Deployment (K3s via Harbor)
# =============================================================================

# Harbor registry configuration
HARBOR_REGISTRY ?= registry.harbor.lan
HARBOR_PROJECT ?= library
KUBECONFIG ?= $(HOME)/workspace/platform/gitops/.kube/k3s.yaml
NAMESPACE ?= fi-fhir
RELEASE_NAME ?= fi-fhir

# Build and push backend image to Harbor
docker-push:
	@echo "Building and pushing fi-fhir backend to Harbor..."
	docker build -t $(HARBOR_REGISTRY)/$(HARBOR_PROJECT)/fi-fhir:$(SHA) .
	docker tag $(HARBOR_REGISTRY)/$(HARBOR_PROJECT)/fi-fhir:$(SHA) $(HARBOR_REGISTRY)/$(HARBOR_PROJECT)/fi-fhir:dev
	docker push $(HARBOR_REGISTRY)/$(HARBOR_PROJECT)/fi-fhir:$(SHA)
	docker push $(HARBOR_REGISTRY)/$(HARBOR_PROJECT)/fi-fhir:dev
	@echo "✓ Backend pushed: $(HARBOR_REGISTRY)/$(HARBOR_PROJECT)/fi-fhir:$(SHA)"

# Build and push UI image to Harbor
docker-push-ui:
	@echo "Building and pushing fi-fhir-ui to Harbor..."
	docker build \
		--build-arg VITE_BUILD_SHA=$(SHA) \
		--build-arg VITE_BUILD_TAG=dev-$(SHA) \
		--build-arg VITE_BUILD_TIME=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ") \
		-t $(HARBOR_REGISTRY)/$(HARBOR_PROJECT)/fi-fhir-ui:$(SHA) \
		./ui
	docker tag $(HARBOR_REGISTRY)/$(HARBOR_PROJECT)/fi-fhir-ui:$(SHA) $(HARBOR_REGISTRY)/$(HARBOR_PROJECT)/fi-fhir-ui:dev
	docker push $(HARBOR_REGISTRY)/$(HARBOR_PROJECT)/fi-fhir-ui:$(SHA)
	docker push $(HARBOR_REGISTRY)/$(HARBOR_PROJECT)/fi-fhir-ui:dev
	@echo "✓ UI pushed: $(HARBOR_REGISTRY)/$(HARBOR_PROJECT)/fi-fhir-ui:$(SHA)"

# Build and push all images
docker-push-all: docker-push docker-push-ui
	@echo ""
	@echo "✅ All images pushed to Harbor"

# Deploy to K3s using Helm
deploy:
	@echo "Deploying fi-fhir to K3s namespace $(NAMESPACE)..."
	KUBECONFIG=$(KUBECONFIG) kubectl create namespace $(NAMESPACE) --dry-run=client -o yaml | KUBECONFIG=$(KUBECONFIG) kubectl apply -f -
	KUBECONFIG=$(KUBECONFIG) helm upgrade --install $(RELEASE_NAME) ./deploy/helm/fi-fhir \
		--namespace $(NAMESPACE) \
		--set image.repository=$(HARBOR_REGISTRY)/$(HARBOR_PROJECT)/fi-fhir \
		--set image.tag=$(SHA) \
		--set replicaCount=1 \
		--wait --timeout 5m
	@echo "✓ Backend deployed"

# Deploy UI to K3s (requires separate UI Helm chart or Kubernetes manifests)
deploy-ui:
	@echo "Deploying fi-fhir-ui to K3s namespace $(NAMESPACE)..."
	KUBECONFIG=$(KUBECONFIG) kubectl create namespace $(NAMESPACE) --dry-run=client -o yaml | KUBECONFIG=$(KUBECONFIG) kubectl apply -f -
	@if [ -f deploy/kubernetes/ui-deployment.yaml ]; then \
		KUBECONFIG=$(KUBECONFIG) kubectl apply -f deploy/kubernetes/ui-deployment.yaml -n $(NAMESPACE); \
	else \
		echo "Creating UI deployment..."; \
		KUBECONFIG=$(KUBECONFIG) kubectl create deployment fi-fhir-ui \
			--image=$(HARBOR_REGISTRY)/$(HARBOR_PROJECT)/fi-fhir-ui:$(SHA) \
			--port=3000 \
			--replicas=1 \
			-n $(NAMESPACE) --dry-run=client -o yaml | KUBECONFIG=$(KUBECONFIG) kubectl apply -f -; \
		KUBECONFIG=$(KUBECONFIG) kubectl expose deployment fi-fhir-ui --port=80 --target-port=3000 -n $(NAMESPACE) --dry-run=client -o yaml | KUBECONFIG=$(KUBECONFIG) kubectl apply -f -; \
	fi
	@echo "✓ UI deployed"

# Full deployment: build, push, deploy
deploy-all: docker-push-all deploy deploy-ui
	@echo ""
	@echo "════════════════════════════════════════════════════════════════"
	@echo "✅ Full deployment complete!"
	@echo "════════════════════════════════════════════════════════════════"
	@echo ""
	KUBECONFIG=$(KUBECONFIG) kubectl get pods -n $(NAMESPACE)

# Check deployment status
deploy-status:
	@echo "fi-fhir deployment status in namespace $(NAMESPACE):"
	@echo ""
	KUBECONFIG=$(KUBECONFIG) kubectl get pods -n $(NAMESPACE) -o wide
	@echo ""
	KUBECONFIG=$(KUBECONFIG) kubectl get svc -n $(NAMESPACE)

# View logs
deploy-logs:
	KUBECONFIG=$(KUBECONFIG) kubectl logs -n $(NAMESPACE) -l app.kubernetes.io/name=fi-fhir --tail=100 -f

# Delete deployment
deploy-delete:
	@echo "Deleting fi-fhir deployment from namespace $(NAMESPACE)..."
	KUBECONFIG=$(KUBECONFIG) helm uninstall $(RELEASE_NAME) -n $(NAMESPACE) || true
	KUBECONFIG=$(KUBECONFIG) kubectl delete deployment fi-fhir-ui -n $(NAMESPACE) || true
	KUBECONFIG=$(KUBECONFIG) kubectl delete svc fi-fhir-ui -n $(NAMESPACE) || true
	@echo "✓ Deployment deleted"

# Port forward for local testing
deploy-forward:
	@echo "Port forwarding fi-fhir services..."
	@echo "Backend: http://localhost:8080"
	@echo "UI: http://localhost:3000"
	@echo ""
	@echo "Press Ctrl+C to stop"
	KUBECONFIG=$(KUBECONFIG) kubectl port-forward -n $(NAMESPACE) svc/$(RELEASE_NAME) 8080:80 &
	KUBECONFIG=$(KUBECONFIG) kubectl port-forward -n $(NAMESPACE) svc/fi-fhir-ui 3000:80

# =============================================================================
# Documentation
# =============================================================================

# Render Mermaid diagrams to SVG (pre-rendered for GitLab Markdown reliability).
docs-mermaid:
	@command -v node >/dev/null 2>&1 || { echo "❌ node not found (required for mermaid-cli)"; exit 1; }
	@command -v npx >/dev/null 2>&1 || { echo "❌ npx not found (required for mermaid-cli)"; exit 1; }
	@echo "Rendering Mermaid diagrams to docs/mermaid/*.svg..."
	@npx -y @mermaid-js/mermaid-cli@latest -c docs/mermaid/config.json -b transparent -i docs/mermaid/overview-flow.mmd -o docs/mermaid/overview-flow.svg -q
	@npx -y @mermaid-js/mermaid-cli@latest -c docs/mermaid/config.json -b transparent -i docs/mermaid/parsing-phases.mmd -o docs/mermaid/parsing-phases.svg -q
	@npx -y @mermaid-js/mermaid-cli@latest -c docs/mermaid/config.json -b transparent -i docs/mermaid/cli-flow.mmd -o docs/mermaid/cli-flow.svg -q
	@npx -y @mermaid-js/mermaid-cli@latest -c docs/mermaid/config.json -b transparent -i docs/mermaid/ui-mapping-flow.mmd -o docs/mermaid/ui-mapping-flow.svg -q
	@echo "✓ Done"

# Generate status data from coverage + git (re-runs tests for fresh coverage)
# Uses merged unit + integration coverage (see test-cover-all) so components
# like Terminology DB reflect testcontainers-backed integration tests in CI.
docs-status: test-cover-all
	@echo "Generating component status data..."
	bash scripts/docs-status.sh
	@echo ""
	@echo "✓ Status data generated. Review and update docs/STATUS.md as needed."

# Generate status data using existing coverage.out (no test re-run)
docs-status-quick:
	@if [ ! -f coverage.out ]; then \
		echo "❌ coverage.out not found. Run 'make test-cover' first, or use 'make docs-status'."; \
		exit 1; \
	fi
	@echo "Generating component status data (using existing coverage.out)..."
	bash scripts/docs-status.sh --check-stale
	@echo ""
	@echo "✓ Status data generated."

# Check documentation consistency
docs-validate:
	@echo "Validating documentation..."
	bash scripts/validate-docs.sh

# Full documentation maintenance (mermaid diagrams + status + validation)
docs-all: docs-mermaid docs-status docs-validate
	@echo ""
	@echo "✅ Full documentation maintenance complete!"
