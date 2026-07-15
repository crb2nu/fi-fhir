# fi-fhir Component Status Matrix

> **Single source of truth** for component maturity, test coverage, and freshness.
> Refresh with `make docs-status` after significant merges.

## Summary

| Maturity       | Count | Description                                                                                                                                                         |
| -------------- | ----- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Production** | 22    | Stable, tested, deployed                                                                                                                                            |
| **Beta**       | 9     | Feature-complete, needs coverage or hardening                                                                                                                       |
| **Alpha**      | 3     | Functional but limited testing or scope                                                                                                                             |
| **Planned**    | 2     | Designed but not yet implemented (tracked via [#7](https://gitlab.flexinfer.ai/libs/fi-fhir/-/issues/7), [#8](https://gitlab.flexinfer.ai/libs/fi-fhir/-/issues/8)) |

---

## Parsers (`internal/parser/*`)

| Component            | Path                             | Maturity   | Coverage | Notes                                       | Last Updated |
| -------------------- | -------------------------------- | ---------- | -------- | ------------------------------------------- | ------------ |
| HL7v2 Parser         | `internal/parser/hl7v2/`         | Production | 84.3%    | Legacy adapters plus strict published A01 path | 2026-07-13 |
| CSV Parser           | `internal/parser/csv/`           | Production | 82.0%    | Schema inference for patient/lab records    | 2026-01-14   |
| EDI X12 Parser       | `internal/parser/edi/`           | Production | 73.9%    | 837P, 835, 270/271, 276/277; loop parsing   | 2026-01-14   |
| EDI Companion Guides | `internal/parser/edi/companion/` | Production | 89.1%    | Medicare, BlueCross, United built-in guides | 2026-01-14   |
| CDA/CCDA Parser      | `internal/parser/cda/`           | Production | 85.8%    | Namespace-aware XML, section handlers       | 2026-02-07   |
| FHIR Parser          | `internal/parser/fhir/`          | Production | 95.2%    | FHIR R4 resource ingestion                  | 2026-01-14   |

## Core Libraries (`pkg/*`)

| Component                | Path                             | Maturity   | Coverage | Notes                                            | Last Updated |
| ------------------------ | -------------------------------- | ---------- | -------- | ------------------------------------------------ | ------------ |
| Events (canonical model) | `pkg/events/`                    | Production | 80.5%    | Immutable semantic event types                   | 2026-03-09   |
| Event Sourcing           | `pkg/eventsourcing/`             | Production | 72.7%    | Store, projections, snapshots, sagas, outbox     | 2026-02-27   |
| ES Projections           | `pkg/eventsourcing/projections/` | Production | 88.9%    | Patient timeline, stats, active encounters       | 2026-01-14   |
| Config                   | `pkg/config/`                    | Production | 89.3%    | Layered loading (defaults → file → env), secrets | 2026-02-27   |
| Integration Contracts    | `pkg/integration/`               | Alpha      | 88.7%    | Exact provenance plus strict preview/production invariants | 2026-07-14 |
| Source Profiles          | `pkg/profile/`                   | Beta       | 71.9%    | Inference, linting, vendor templates             | 2026-02-27   |
| Validators               | `pkg/validate/`                  | Production | 98.2%    | NPI, MBI, SSN, DEA; Luhn/checksum                | 2026-01-09   |
| FHIR Mapper              | `pkg/fhir/`                      | Production | 75.2%    | 24+ US Core resources, validation                | 2026-01-19   |
| ETL Pipeline             | `pkg/etl/`                       | Beta       | 84.8%    | Source/sink framework, CLI commands              | 2026-02-27   |
| Storage                  | `pkg/storage/`                   | Beta       | 56.5%    | Abstraction layer for file/S3/MinIO              | 2026-02-27   |
| Terminology (core)       | `pkg/terminology/`               | Production | 84.2%    | LOINC, ICD-10, fuzzy matching, UMLS              | 2026-03-07   |
| Terminology DB           | `pkg/terminology/db/`            | Beta       | 22.5%    | PostgreSQL loaders; needs testcontainers         | 2026-03-07   |
| Terminology Upload       | `pkg/terminology/upload/`        | Beta       | 85.7%    | Mapping file upload pipeline                     | 2026-02-03   |
| Terminology Suggest      | `pkg/terminology/suggest/`       | Beta       | 83.4%    | Suggestion engine; httptest-based coverage       | 2026-02-28   |
| Terminology Semantic     | `pkg/terminology/semantic/`      | Production | 95.0%    | Embedding-based semantic search                  | 2026-02-28   |
| Terminology Index        | `pkg/terminology/index/`         | Production | 92.2%    | Vector indexing; httptest + mock coverage        | 2026-03-03   |
| Patient Matching         | `pkg/matching/`                  | Production | 85.3%    | Deterministic + probabilistic, MPI, batch        | 2026-02-27   |
| LLM Client               | `pkg/llm/`                       | Production | 86.3%    | Multi-provider client, retry, embeddings         | 2026-03-06   |
| LLM Copilot              | `pkg/llm/copilot/`               | Production | 98.0%†   | CEL-based copilot + workflow generation          | 2026-02-08   |

## Internal Services (`internal/*`)

| Component             | Path                              | Maturity   | Coverage | Notes                                            | Last Updated |
| --------------------- | --------------------------------- | ---------- | -------- | ------------------------------------------------ | ------------ |
| Workflow Engine       | `internal/workflow/`              | Production | 78.6%    | Actions plus strict DSL v1 and pure planner      | 2026-07-13   |
| Integration Processor | `internal/integration/processor/` | Alpha      | 84.5%    | Shared A01 preview/production kernel plus PostgreSQL atomic admission | 2026-07-14 |
| HL7v2 HTTP Ingress    | `internal/integration/ingress/`   | Alpha      | —        | Bearer/HMAC, bounded body, durable response; GitOps activation pending | 2026-07-14 |
| GraphQL API           | `internal/api/graphql/`           | Beta       | 9.5%\*   | Preview role: health/preview; legacy + WS disabled | 2026-07-13 |
| FHIR Subscriptions    | `internal/fhir/subscription/`     | Production | 83.7%    | Bidirectional; client + webhook receiver         | 2026-02-27   |
| Terminology Autoroute | `internal/terminology/autoroute/` | Beta       | 88.5%    | Automatic code-system routing engine             | 2026-03-09   |
| Terminology Workflow  | `internal/terminology/workflow/`  | Production | 95.7%    | Temporal workflow + activity + worker unit tests | 2026-03-03   |
| LLM Explain           | `internal/llm/explain/`           | Production | 99.7%    | Warning + workflow explanation generation        | 2026-02-28   |
| LLM Extract           | `internal/llm/extract/`           | Production | 84.3%    | Structured data extraction from documents        | 2026-03-09   |
| LLM Quality           | `internal/llm/quality/`           | Production | 93.8%    | Data quality analysis                            | 2026-01-23   |

> \* GraphQL coverage is low because gqlgen generates ~2000 resolver stubs; actual hand-written resolver logic is well-tested (80.8% in `resolvers/` sub-package).
>
> † Copilot coverage is per-package (leaf package not exercised cross-package).

Slice 1.1c shipped in MR `!96`; main pipeline `18621` passed every blocking
gate and published matching API/UI images. GitOps MRs `!368` and `!369` rolled
out their verified digests, passed public auth/origin/containment/provenance/PHI
probes, and resumed healthy automation. Exact evidence is recorded in
`.loom/iteration-plan-phase-1-slice-1-1c-authenticated-preview-adapters.md`.

Slice 1.3 adds exact `POST /v1/hl7v2` as the first production adapter. MR `!99`
pipeline `18898` passed 32/32 jobs; required job `182088` ran all 20 PostgreSQL
16 restart, duplicate-collapse, profile-divergence, IDE-parity, preview-side-
effect, and leakage assertions. Merge commit `48d156d2` repeated the proof in
main job `182694`; pipeline `18951` passed 35/35 and published digest-addressed
API/UI images. Production GitOps activation and external outbox delivery remain
intentionally pending.

## Infrastructure

| CLI | `cmd/fi-fhir/` | Production | 83.9% | parse, workflow, config, etl, terminology, eventstore, subscription | 2026-03-24 |
| TypeScript SDK | `sdk/typescript/` | Production | — | CLI wrapper + type definitions; npm publishable | 2026-01-19 |
| UI / Mapping Studio | `ui/src/` | Beta | — | Deployed credential gate; bearer/raw samples stay in tab memory | 2026-07-13 |
| Helm Chart | `deploy/helm/fi-fhir/` | Production | — | Full templating; HPA, PDB, ServiceMonitor | 2026-02-03 |
| Kubernetes Manifests | `deploy/kubernetes/` | Production | — | Kustomize base + production overlay | 2026-02-03 |
| CI/CD Pipeline | `.gitlab-ci.yml` | Production | — | Blocking lint, test, security, build, scan, and API/UI publish gates | 2026-07-13 |
| Grafana Dashboards | `dashboards/grafana/` | Production | — | Workflow overview dashboard | 2026-01-14 |
| Alerting Rules | `dashboards/alerting/` | Production | — | Prometheus rules + K8s PrometheusRule CRD | 2026-01-14 |
| OpenAPI Spec | `api/openapi.yaml` | Production | — | REST API documentation | 2026-01-14 |
| Docker | `Dockerfile` | Production | — | Multi-stage distroless build | 2026-01-14 |

---

## Coverage Legend

| Range  | Indicator     |
| ------ | ------------- |
| ≥ 80%  | ✅ Good       |
| 60–79% | ⚠️ Acceptable |
| < 60%  | ❌ Needs work |

## Refreshing This Document

```bash
make docs-status        # Re-run tests + generate fresh data
make docs-status-quick  # Use existing coverage.out (faster)
make docs-status-check  # Check for coverage drift (CI gate)
```

### Contributor Workflow

1. After changing Go source code, coverage percentages may shift.
2. CI runs `test:docs-status` to detect drift > 5% between this file and computed values.
3. If the job fails, regenerate: `make docs-status` then commit the updated `docs/STATUS.md`.
4. The drift threshold is configurable via `DRIFT_THRESHOLD` env var (default: 5.0%).

The `scripts/docs-status.sh --check-drift` script compares computed coverage from `coverage.out` against the values in this file and exits non-zero when drift exceeds the threshold.
