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
| Healthcare standards | FHIR R4 4.0.1, US Core 9.0.0, SMART App Launch 2.2.0, Bulk Data 3.0.0 | Release targets; official validator or conformance-suite evidence is not yet complete. Nothing in the repository is pinned to a US Core package — see "FHIR profile-version assertion policy" below |

Kubernetes 1.36 is the pinned minor because it is an actively supported upstream
release during the Engine Alpha program. Patch releases may advance within 1.36
for security and defect fixes; changing the minor requires a dated decision and
a full deployment proof rerun.

## FHIR profile-version assertion policy

*Decided 2026-08-09, Slice 5.1a. Recorded in `.loom/40-decisions.md` and
`docs/planning/FHIR-CONFORMANCE-MATRIX.md`; asserted by
`TestFHIRConformance_ProfileVersionPolicy`.*

**The mapper asserts bare canonicals. The checker accepts a bare canonical or any
`|version`-pinned form of it.**

Concretely, a resource this product emits declares
`http://hl7.org/fhir/us/core/StructureDefinition/us-core-patient`, never
`…/us-core-patient|9.0.0`. A resource this product *validates* may declare either.

Two consequences worth stating plainly, because the row above lists US Core 9.0.0
and a reader could reasonably infer more than is true:

- **The product is not pinned to US Core 9.0.0.** None of the 32 US Core profile
  constants carries a version suffix, no IG package is vendored, and nothing
  resolves a profile against a package. 9.0.0 is a release *target*. Do not
  describe the repository as "US Core 9.0.0-pinned".
- **The shipped checker is a required-element and profile-URL presence check, not
  a profile validator.** It has no terminology bindings, no primitive-type
  checks, no slicing, and no invariants. Version *tolerance* is not version
  *resolution*: stripping a `|version` suffix lets a correctly pinned resource
  pass the presence check; it does not verify that the resource conforms to that
  version of that profile.

The alternative — pinning all constants to `|9.0.0` and requiring an exact match —
was rejected. Without a package-resolution step a pinned constant asserts a
version it cannot verify, and it would reject a correct bare canonical. Version
resolution arrives with the pinned offline `.tgz` packages in Slice 5.1b, and
item 7 of the evidence list below stays blocking until then.

## Reference application profile

The performance and recovery gates use this application-side reference profile:

- Linux amd64;
- 4 vCPU and 8 GiB RAM available to the fi-fhir workload;
- two application replicas for Kubernetes scenarios;
- PostgreSQL 16 on SSD-backed persistent storage;
- destinations decoupled from durable acceptance for latency measurements;
- 2-KiB HL7v2 messages for the baseline throughput journey.

### The chart defaults are not the reference profile

`deploy/helm/fi-fhir/values.yaml` requests `100m` CPU and `128Mi` memory and
limits each pod to `500m` CPU and `512Mi` memory. **A 4 vCPU / 8 GiB budget
cannot be met inside a `500m` limit**, so the chart defaults and the profile
above are not two views of one thing — they are a scheduling default and a
measurement target that happen to live in the same repository.

Slice 4.4a resolves the contradiction by naming it rather than by moving a
number. Raising the chart defaults to the profile would change what every
existing deployment schedules, on the strength of a profile nothing has yet
measured against — a capacity claim dressed as a chart edit.

The resolution:

- **The chart defaults stay.** They are what a small or evaluation deployment
  should schedule, and they are labelled as scheduling defaults in
  `values.yaml` itself.
- **`deploy/helm/fi-fhir/values-reference-profile.yaml` carries the profile.**
  It sets requests and limits to the 4 vCPU / 8 GiB envelope and two replicas,
  and is the file any performance run must use:
  `helm install ... -f deploy/helm/fi-fhir/values-reference-profile.yaml`.
- **Slice 4.4b owns the measurement**, and is blocked on a pinned-runner
  decision: CI's k3s pool spans hardware differing by more than 5×
  (`.gitlab-ci.yml`, `test:benchmark`), so a latency budget measured there is
  either permanently red or calibrated into meaninglessness. Numeric budgets 1,
  2, and 3 cannot be certified until that decision is made.

Until 4.4b archives a report, **no document may describe the chart defaults, or
the reference profile, as proven capacity.** The profile is the environment a
future measurement must use; it is not a claim that the software performs at
that scale.

## Required evidence before a 1.0 support claim

The following remain blocking:

1. Golden Path 001 production/preview parity, durable receipt, duplicate,
   restart, and profile-revision tests;
2. authenticated HTTP and MLLP protocol journeys;
3. Helm and Kustomize render plus Kubernetes 1.36 install, upgrade, rollback,
   and uninstall proof — **render and schema validation closed by slice 4.4c**
   (`scripts/validate-k8s-schema.sh`, blocking in `lint:helm`): the chart at
   default values, the chart at the reference profile, the Kustomize base, and
   the production overlay all validate `-strict` against the pinned 1.36 API
   schemas, with a negative control that must be rejected. Both application
   Deployments now declare a rolling-update budget, a grace period, and a
   `preStop`, and the PostgreSQL Deployment uses `Recreate` so it cannot wedge
   on its ReadWriteOnce volume. **Live install, upgrade, rollback, and uninstall
   evidence on a 1.36 cluster remains blocking** and is the last RC item;
4. PostgreSQL backup/restore and the documented RTO proof — **closed by slice
   4.4c**: `test:migration-compatibility` proves the restore is faithful (rows,
   PHI payloads, immutability guards attributable to those guards by SQLSTATE,
   the `NOT VALID` provenance CHECK, and all six schema ledgers at their
   declared versions), proves the delivery worker resumes from the restored
   state, and archives a measured recovery time as `recovery-rto.json`. The
   **RPO half stays open and is an operator responsibility with a stated
   method**, not a product claim: bounding data loss to minutes requires
   continuous WAL archiving and point-in-time recovery, which belongs to
   whoever runs the database. See `docs/operations/PRODUCTION-HARDENING.md`,
   "What this repository claims, and what it hands to the operator", and
   `.loom/40-decisions.md` (2026-08-09, "WAL/PITR posture");
5. reference-profile latency, throughput, soak, and recovery reports;
6. browser/accessibility matrix evidence;
7. official healthcare standards conformance evidence where applicable;
8. security, PHI, secret, tenant-isolation, and audit kill-tests.

Until those gates pass, documentation must describe individual capabilities and
their evidence rather than label the whole product “1.0 certified,” “HIPAA
compliant,” or standards conformant.
