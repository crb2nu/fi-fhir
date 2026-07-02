# fi-fhir Roadmap

> Last Updated: 2026-07-02
> Tier: 2 (see workspace AGENTS.md "Portfolio Tiers")
> Tracking Issue: https://gitlab.flexinfer.ai/libs/fi-fhir/-/issues/19

## Current Status

Active. fi-fhir is a healthcare-data integration toolkit (Go backend, GraphQL
API, pnpm/React UI) for FHIR/HL7 transformation and projection workflows. Last
meaningful activity 2026-06-28: the integration session engine landed across
backend, GraphQL, and UI slices (`24491407`, `1b249293`, `94aa9252`), plus the
LLM config namespace work — `FI_FHIR_LLM_*` env keys, capability query, and
serve enablement gate (`4c067c3f`, `4fc5fa00`, `dc80ea82`). Deployed on k3s via
Flux (`platform/gitops/k3s/fi-fhir/` — app namespace with Temporal and MinIO
sidecars). Backlog groomed 2026-07-02: four stale CI/placeholder issues closed,
all open issues now carry P1/P2/P3 labels.

- **Plan store**: plan-workspace-portfolio-refresh-2026-h2-roadmaps-quality-baselin-f3db23
- **Deployed**: k3s `fi-fhir` namespace via Flux (`platform/gitops/k3s/fi-fhir/`)
- **CI**: custom multi-stage Go+Node pipeline on platform/gitops CI includes (cache) + tech-radar scan

## Now

- [ ] M1: backend↔frontend integration parity — GraphQL + UI runtime contracts; integration session engine landed 2026-06-28, parity closure remains (#9)
- [ ] M2: flexinfer inference path with timeout/retry/error contracts (#10)

## Next

- [ ] M3: mentatlab run orchestration and SSE lifecycle events (#11)
- [ ] Dependency Dashboard triage (#4)

## Later

- Next-wave enhancements backlog (#8): additional FHIR IG support (#12), test-data fixtures (#15), UI accessibility/bulk-ops (#16), terminology approval workflow (#17), storage provider integration tests (#18)

## Backlog

Full backlog: [P1 issues](https://gitlab.flexinfer.ai/libs/fi-fhir/-/issues/?label_name[]=P1) ·
[P2](https://gitlab.flexinfer.ai/libs/fi-fhir/-/issues/?label_name[]=P2) ·
[P3](https://gitlab.flexinfer.ai/libs/fi-fhir/-/issues/?label_name[]=P3) ·
[Milestones](https://gitlab.flexinfer.ai/libs/fi-fhir/-/milestones)
