# Build stage
ARG GO_IMAGE=golang:1.26.10-alpine
FROM ${GO_IMAGE} AS builder

# Install git for go mod download and ca-certificates for HTTPS
RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app

# Copy go mod files first for better layer caching
COPY go.mod go.sum ./

# proxy.golang.org occasionally resets an HTTP/2 stream part-way through a
# module zip ("stream error: stream ID N; INTERNAL_ERROR; received from peer");
# one reset used to fail the whole image build (build:docker, 2026-11-03,
# pipelines 37258/37313/37378). Retry a bounded number of times with a short
# backoff. Modules that finished before the reset stay in the module cache, so
# each retry only fetches what was cut off. The attempt counter is printed so a
# job trace shows whether the retry path fired.
RUN set -eu; \
    attempt=1; max_attempts=3; \
    until go mod download; do \
      if [ "$attempt" -ge "$max_attempts" ]; then \
        echo "go mod download: attempt ${attempt}/${max_attempts} failed; giving up" >&2; \
        exit 1; \
      fi; \
      delay=$((attempt * 5)); \
      echo "go mod download: attempt ${attempt}/${max_attempts} failed; retrying in ${delay}s" >&2; \
      sleep "$delay"; \
      attempt=$((attempt + 1)); \
    done; \
    echo "go mod download: succeeded on attempt ${attempt}/${max_attempts}"

# Copy source code
COPY . .

# Build the binary
# CGO_ENABLED=0 produces a static binary
# -ldflags="-s -w" strips debug info for smaller binary
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-s -w -X main.version=${VERSION}" \
    -o /fi-fhir \
    ./cmd/fi-fhir

# Final stage - distroless for minimal attack surface
FROM gcr.io/distroless/static-debian12:nonroot

# Copy timezone data for time parsing
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

# Copy CA certificates for HTTPS connections
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copy the binary
COPY --from=builder /fi-fhir /fi-fhir

# Create directories for config and data
# Note: distroless doesn't have mkdir, so we create via COPY
WORKDIR /app

# Default configuration paths
ENV FI_FHIR_SERVER_HOST=0.0.0.0
ENV FI_FHIR_SERVER_PORT=8080
ENV FI_FHIR_LOG_LEVEL=info
ENV FI_FHIR_LOG_FORMAT=json

# Expose default ports
# 8080 - HTTP API
# 9090 - Metrics (Prometheus)
EXPOSE 8080 9090

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD ["/fi-fhir", "version"]

# Run as non-root user (provided by distroless:nonroot)
USER nonroot:nonroot

ENTRYPOINT ["/fi-fhir"]
CMD ["help"]
