# Build stage
FROM golang:1.22-alpine AS builder

# Install git for go mod download and ca-certificates for HTTPS
RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app

# Copy go mod files first for better layer caching
COPY go.mod go.sum ./
RUN go mod download

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
