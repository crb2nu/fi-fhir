# Changelog

All notable changes to fi-fhir will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- End-to-end test framework with Docker Compose integration
- OpenAPI 3.1 specification for REST API (`api/openapi.yaml`)
- Production hardening guide for HIPAA compliance
- Operations runbook with troubleshooting procedures

## [0.1.0] - 2024-01-15

### Added

#### Core Functionality
- HL7v2 message parsing (ADT A01-A04, A08, ORU R01, SIU S12-S15, S26)
- CSV/flatfile parsing with schema inference
- EDI X12 parsing (837P claims, 835 remittance, 270/271 eligibility, 276/277 status)
- Canonical semantic event model (`pkg/events/`)
- Source Profile system for per-interface configuration

#### Workflow Engine
- YAML-based workflow DSL for event routing
- CEL (Common Expression Language) filter conditions
- Transform pipeline (set_field, map_terminology, redact)
- Action types: log, webhook, fhir, database, queue
- Dry-run mode for testing workflows

#### FHIR Integration
- FHIR R4 resource generation (Patient, Encounter, Observation, DiagnosticReport)
- US Core profile mapper
- OAuth2 client credentials flow with token caching
- Automatic 401 retry with token refresh

#### Reliability Features
- Retry with exponential backoff for HTTP actions
- Circuit breaker pattern for failing external services
- Dead letter queue (DLQ) for failed events
- Rate limiting for high-volume event streams
- Event replay from DLQ or recordings

#### Observability
- Prometheus metrics (`workflow_events_processed_total`, etc.)
- OpenTelemetry distributed tracing
- Structured JSON logging with trace correlation
- Grafana dashboard templates
- Prometheus alerting rules

#### CLI
- `parse` - Parse messages (HL7v2, CSV, EDI)
- `workflow run` - Process events through workflow
- `workflow validate` - Validate workflow configuration
- `config show/validate/env/init` - Configuration management
- `version` - Version information

#### Deployment
- Multi-stage Dockerfile with distroless base
- Docker Compose for local development
- Kubernetes manifests with Kustomize overlays
- Helm chart with full templating
- GitLab CI/CD pipeline (lint, test, security, build, release)

#### SDK
- TypeScript SDK with CLI wrapper
- Type definitions for events and workflow

#### Validation
- NPI (National Provider Identifier) validation with Luhn check
- MBI (Medicare Beneficiary Identifier) validation
- SSN format validation
- DEA number validation

### Security
- Non-root container execution
- Read-only root filesystem
- Secret provider interface (env, file, Vault, AWS SSM, K8s secrets)
- TLS 1.3 support
- Pod security standards (restricted)
- Network policy templates

## Types of Changes

- `Added` for new features
- `Changed` for changes in existing functionality
- `Deprecated` for soon-to-be removed features
- `Removed` for now removed features
- `Fixed` for any bug fixes
- `Security` for vulnerability fixes

[Unreleased]: https://gitlab.flexinfer.ai/libs/fi-fhir/-/compare/v0.1.0...main
[0.1.0]: https://gitlab.flexinfer.ai/libs/fi-fhir/-/releases/v0.1.0
