# Supported 1.0 Baseline

This document fixes the target environment for fi-fhir 1.0 evidence. It is a
test contract, not a claim that the current pre-1.0 repository has completed
every release, conformance, performance, recovery, or compatibility gate.

## Status vocabulary

- **locked**: the repository and required CI already enforce this choice;
- **reference target**: future 1.0 proof must use this exact profile unless a
  dated architecture decision changes it;
- **release gate**: still requires archived end-to-end evidence before 1.0.

## Platform matrix

| Area | 1.0 baseline | Current status |
|---|---|---|
| Security domain | One logical healthcare-organization security domain per deployment; tenant and actor identity remain mandatory on runtime and durable contracts | Reference target; shared multi-tenant hosting is not a 1.0 claim |
| Backend toolchain | Go 1.26.5 | Locked by `go.mod`, CI, and the production image build |
| UI and SDK toolchain | Node.js 22 and npm 10.9.3 | Locked by CI and frozen npm lockfiles |
| Runtime OS/architecture | Linux amd64 reference; Linux arm64 release target | amd64 is the reference target; arm64 needs release-gate runtime evidence |
| Persistence | PostgreSQL 16 | Locked for Compose and CI integration services; production recovery evidence remains a release gate |
| Local deployment | Docker Compose with PostgreSQL 16 | Development reference; not a production topology |
| Kubernetes deployment | Kubernetes 1.36.x through Helm and Kustomize | Pinned reference target; render, install, upgrade, rollback, and live golden-journey evidence remain release gates |
| Authoring UI | Current SvelteKit build served as static assets | Build/test locked; latest-two Chrome, Edge, and Firefox plus current Safari compatibility remains a release gate |
| Healthcare standards | FHIR R4 4.0.1, US Core 9.0.0, SMART App Launch 2.2.0, Bulk Data 3.0.0 | Release targets; official validator or conformance-suite evidence is not yet complete |

Kubernetes 1.36 is the pinned minor because it is an actively supported upstream
release during the Engine Alpha program. Patch releases may advance within 1.36
for security and defect fixes; changing the minor requires a dated decision and
a full deployment proof rerun.

## Reference application profile

The performance and recovery gates use this application-side reference profile:

- Linux amd64;
- 4 vCPU and 8 GiB RAM available to the fi-fhir workload;
- two application replicas for Kubernetes scenarios;
- PostgreSQL 16 on SSD-backed persistent storage;
- destinations decoupled from durable acceptance for latency measurements;
- 2-KiB HL7v2 messages for the baseline throughput journey.

The Helm chart currently requests `100m` CPU and `128Mi` memory and limits each
pod to `500m` CPU and `512Mi` memory. Those are scheduling defaults, not proven
capacity recommendations. Phase 4.4 must tune and archive the exact values used
for performance evidence instead of silently treating chart defaults as a
certified profile.

## Required evidence before a 1.0 support claim

The following remain blocking:

1. Golden Path 001 production/preview parity, durable receipt, duplicate,
   restart, and profile-revision tests;
2. authenticated HTTP and MLLP protocol journeys;
3. Helm and Kustomize render plus Kubernetes 1.36 install, upgrade, rollback,
   and uninstall proof;
4. PostgreSQL backup/restore and the documented RPO/RTO proof;
5. reference-profile latency, throughput, soak, and recovery reports;
6. browser/accessibility matrix evidence;
7. official healthcare standards conformance evidence where applicable;
8. security, PHI, secret, tenant-isolation, and audit kill-tests.

Until those gates pass, documentation must describe individual capabilities and
their evidence rather than label the whole product “1.0 certified,” “HIPAA
compliant,” or standards conformant.
