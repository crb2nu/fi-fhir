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
| Healthcare standards | FHIR R4 4.0.1, US Core 9.0.0, SMART App Launch 2.2.0, Bulk Data 3.0.0 | Release targets; official validator or conformance-suite evidence is not yet complete. Since Slice 5.1b the repository pins `hl7.fhir.r4.core#4.0.1` and `hl7.fhir.us.core#9.0.0` as offline `.tgz` under `testdata/fhir/packages/` and resolves against them — see "FHIR profile-version assertion policy" below for exactly what that does and does not establish  Since Slice 4.1c-c the durable engine delivers Patient/Encounter (ADT) and DiagnosticReport/Observation (lab) over the `fhir` transport as conditional transaction Bundles. |

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

- **The mapper's own output is not version-pinned, and the *packages* now are.**
  None of the 32 US Core profile constants carries a version suffix, and that
  stays deliberate. What changed in Slice 5.1b is that
  `hl7.fhir.r4.core#4.0.1` and `hl7.fhir.us.core#9.0.0` are checked in as
  offline `.tgz` under `testdata/fhir/packages/` with sha256 sums verified on
  every test run, and a resolver reads them. Both archives reproduce the
  registry-published `dist.shasum`. All 32 constants resolve to a real US Core
  9.0.0 profile. 9.0.0 is still a release *target* for the product's behaviour;
  it is now a *fact* about what the repository validates against.
- **The shipped checker is still a required-element and profile-URL presence
  check, not a profile validator.** `pkg/fhir/validate.go` is unchanged by
  5.1b. It has no terminology bindings, no primitive-type checks, no slicing,
  and no invariants.
- **The new structural validator resolves, and still is not the official
  validator.** `pkg/fhir/structural.go` (Slice 5.1b, Option C) checks, against
  the pinned archives: resource type existence in R4 4.0.1; profile resolution,
  resource-type agreement and `baseDefinition` chain termination; and
  cardinality — `min`, `max`, prohibition — from the R4 base snapshot and every
  resolved profile snapshot, at every element depth. It reports unpopulated
  must-support elements without failing on them, because US Core `mustSupport`
  binds the system rather than the instance. It does **not** evaluate
  terminology bindings, FHIRPath invariants, slicing, primitive-type formats,
  reference targets, or extensions, and it issues no conformance certificate.
  `validator_cli.jar` as a CI-only job (Option A) is Sprint 7, and item 7 of the
  evidence list below stays blocking until it lands.

The alternative — pinning all constants to `|9.0.0` and requiring an exact match —
was rejected in 5.1a. Without a package-resolution step a pinned constant asserts
a version it cannot verify, and it would reject a correct bare canonical. **The
package-resolution step now exists**, so the tolerance has teeth it did not have:
`…/us-core-patient|9.0.0` resolves against the pinned package and
`…/us-core-patient|8.0.0` is rejected, where 5.1a's suffix-stripping accepted
both. Re-pinning the constants is still not planned — a bare canonical is what a
US Core publisher is expected to emit — but the reason has narrowed from "we
cannot verify a version" to "we choose not to assert one".

Running the structural validator over the mapper's own generated fixtures found
seven cardinality violations across five of the twenty-five files after MR !211
fixed medication substitution serialization. The DocumentReference content gap
still violates base R4 as well as US Core. They are enumerated in
`docs/planning/FHIR-CONFORMANCE-MATRIX.md` §5.1, held to exact equality by
`make fhir-structural`, and are the subject of the next mapper slice. A reader
sizing up this row should read that table before quoting the standards row.

## Reference application profile

The performance and recovery gates use this application-side reference profile:

- Linux amd64;
- 4 vCPU and 8 GiB RAM **per application replica** — not across the
  deployment. Kubernetes `resources` are per container, so a total figure
  cannot be expressed in the values file that carries the profile, and
  halving the envelope to make the prose match would be a capacity change
  made on no measurement. Two replicas therefore total 8 vCPU / 16 GiB;
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
- **Slice 4.4b built the harness and made the runner decision** (see the
  budget table below). The decision was not "wait": CI's k3s pool spans hardware
  differing by more than 5×, so a latency budget measured there is either
  permanently red or calibrated into meaninglessness — but allocation counts do
  not depend on the machine, so the durable accept path is now gated on those,
  blocking, in the ordinary pool. Wall-clock and throughput are measured by a
  job restricted to the pinned `fi-fhir-perf` runner identified in Sprint 6
  planning. Its presence is not certification evidence.

Until an accepted run archives the required measurements, **no document may describe the
chart defaults, or the reference profile, as proven capacity.** The profile is
the environment a future measurement must use; it is not a claim that the
software performs at that scale.

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
5. reference-profile latency, throughput, soak, and recovery reports
   (**budget by budget, below**);
6. browser/accessibility matrix evidence;
7. official healthcare standards conformance evidence where applicable;
8. security, PHI, secret, tenant-isolation, and audit kill-tests.

### Performance and recovery budgets, one by one

The word "harnessed" below is doing real work. It means the path is measured and
a regression in it fails a blocking CI job — and it does **not** mean the
numeric budget has been demonstrated. Those are different claims and this table
keeps them apart on purpose.

| # | Budget | Status | What exists, and what is missing |
|---|---|---|---|
| 1 | Authenticated MLLP and HTTP durable-accept latency (p95 ≤ 250 ms, p99 ≤ 500 ms) | **Harnessed, uncertified** | `internal/integration/perf` benchmarks both paths against a real PostgreSQL, and `bench-check -set=durable` gates their `allocs/op` with `allow_failure: false`. Wall-clock is measured but **not** asserted anywhere: a millisecond ceiling calibrated for a pool spanning 5.3×, in a 1-CPU pod sharing space with a database container, is not evidence. Certification needs a pinned runner. |
| 2 | One-hour steady-state throughput on the reference profile | **Harnessed, uncertified** | Slice 4.4e's per-deployment MLLP rate quota has merged. The remaining proof is a one-hour run on the pinned reference profile; ordinary CI and allocation ceilings do not certify steady-state throughput. |
| 3 | 1-GiB batch import peak memory above idle | **Harnessed, uncertified** | `perf.HeapSampler` measures peak heap above an idle baseline; `runtime.ReadMemStats` appeared nowhere in first-party code before it. It reports `HeapAlloc`, not RSS: a Go process's RSS includes heap the collector has freed and not returned to the OS, so RSS is a property of GC timing as much as of the workload. A true RSS figure has to come from the pinned-runner job reading the cgroup. |
| 4 | Recovery time objective | **Not started — slice 4.4c** | 4.4a proved a `pg_dump`/restore round-trip preserves every durable row and trigger. It measured no recovery *time*. |
| 5 | Recovery point objective | **Known unachievable as configured — slice 4.4c** | `PRODUCTION-HARDENING.md` states it directly: logical dumps cannot meet a minutes-scale RPO, and nothing in this repository configures WAL archiving or PITR. |
| 6 | One-version rollback safety | **Certified — slice 4.4a** | N-1 defined per migration ledger, a real defect found and fixed (`0004_export_attribution.sql` made three columns `NOT NULL` with no `DEFAULT`, so rollback failed every session export), and a restore round-trip proof in CI. |
| 7 | Golden-journey evidence on Kubernetes 1.36 | **Not started — slice 4.4c** | Needs a cluster. |

**Budgets 1, 2 and 3 are gated on allocations only, and that gate is narrower
than it sounds.** Measured over three runs, the durable accept path's allocation
count varies by about 2 in 4650 — stable, but not the bit-identical figure the
legacy micro-benchmarks report. A ceiling roughly 1% above the observed count
detects a regression of some 40 allocations per message. It will not notice one.
It is a regression detector for the thing that *causes* latency, not a
measurement of latency.

**What unblocks certification.** Run the existing manual
`test:performance-profile` job on the `fi-fhir-perf` runner with the documented
4 CPU / 8 GiB reference profile and retain its `performance-report.json`.
`FI_FHIR_PERF_RUNNER=1` makes the job available; runner registration and a green
ordinary MR pipeline do not supply the required measurements. The Sprint 6
planning record identifies runner id 8; S6-B certification remains open.

Until those gates pass, documentation must describe individual capabilities and
their evidence rather than label the whole product “1.0 certified,” “HIPAA
compliant,” or standards conformant.
