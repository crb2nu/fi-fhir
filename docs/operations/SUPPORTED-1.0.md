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
| Healthcare standards | FHIR R4 4.0.1, US Core 9.0.0, SMART App Launch 2.2.0, Bulk Data 3.0.0 | Release targets. For FHIR R4 / US Core, the HL7 official validator (`validator_cli.jar` 6.10.4) runs offline in CI as a blocking gate (Slice 5.1c-β) over the mapper's fixtures and the Bundles the `fhir` transport delivers, against 23 pinned packages with no terminology server, and its findings — errors included — are held to an exact-equality ledger: official-validator evidence exists, a clean pass and a certificate do not (`docs/planning/FHIR-CONFORMANCE-MATRIX.md` §5.2). SMART App Launch 2.2.0 and Bulk Data 3.0.0 have no validator or conformance-suite evidence yet. Since Slice 5.1b the repository pins `hl7.fhir.r4.core#4.0.1` and `hl7.fhir.us.core#9.0.0` as offline `.tgz` under `testdata/fhir/packages/` and resolves against them — see "FHIR profile-version assertion policy" below for exactly what that does and does not establish. Since Slice 4.1c-c the durable engine delivers Patient/Encounter (ADT) and DiagnosticReport/Observation (lab) over the `fhir` transport as conditional transaction Bundles. |

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
- **The official validator now runs too, and is still not a pass.**
  `validator_cli.jar` 6.10.4 (Option A, Slice 5.1c-β) validates the mapper
  fixtures and the delivered Bundles offline in CI — invariants, slicing,
  datatype rules and local code-system membership included, external
  terminology not — and holds every finding to an exact-equality ledger that
  still records errors. It does not close item 7 of the evidence list below:
  the FHIR ledger still records errors, and SMART App Launch and Bulk Data have
  no evidence at all.

The alternative — pinning all constants to `|9.0.0` and requiring an exact match —
was rejected in 5.1a. Without a package-resolution step a pinned constant asserts
a version it cannot verify, and it would reject a correct bare canonical. **The
package-resolution step now exists**, so the tolerance has teeth it did not have:
`…/us-core-patient|9.0.0` resolves against the pinned package and
`…/us-core-patient|8.0.0` is rejected, where 5.1a's suffix-stripping accepted
both. Re-pinning the constants is still not planned — a bare canonical is what a
US Core publisher is expected to emit — but the reason has narrowed from "we
cannot verify a version" to "we choose not to assert one".

Running the structural validator over the mapper's own generated fixtures now
finds nothing: all twenty-five files are clean and the structural ledger is
empty as of Slice 5.1c-α (2026-09-24), which closed the seven cardinality
violations Slice 5.1b measured — including the DocumentReference content gap
that violated base R4 as well as US Core. The empty ledger is still held to
exact equality by `make fhir-structural`, so a violation that reappears fails
the build, and the official validator independently reports no cardinality
finding on the same fixtures. That is cardinality only: the official
validator's ledger still records errors of other kinds
(`docs/planning/FHIR-CONFORMANCE-MATRIX.md` §5.2). A reader sizing up this row
should read §5.1 and §5.2 before quoting the standards row.

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
| 1 | Authenticated MLLP and HTTP durable-accept latency (p95 ≤ 250 ms, p99 ≤ 500 ms) | **Certified on the serial accept paths — runner 8, 2026-09-25** | Three plays of `test:performance-profile` on identical `main` code (`4d8f31a8c`, [pipeline 28955](https://gitlab.flexinfer.ai/libs/fi-fhir/-/pipelines/28955); jobs [308841](https://gitlab.flexinfer.ai/libs/fi-fhir/-/jobs/308841), [309005](https://gitlab.flexinfer.ai/libs/fi-fhir/-/jobs/309005), [309029](https://gitlab.flexinfer.ai/libs/fi-fhir/-/jobs/309029)) on runner 8 (`fi-fhir-perf`, `Intel(R) Xeon(R) CPU E5-2680 v4 @ 2.40GHz`, 4 CPU / 8 GiB, PostgreSQL 16 in the pod, one replica) each archived a `performance-report.json` with `certified: true`: serial `IngressSubmit` p95 9.4–9.9 ms / p99 11.4–13.3 ms, serial `MLLPSubmit` p95 9.4–10.0 ms / p99 10.3–11.4 ms, and the `RunParallel` variants p95 38.9–50.6 ms / p99 53.5–65.2 ms — every figure at least four times inside the target. The harness is proven sensitive: job [308698](https://gitlab.flexinfer.ai/libs/fi-fhir/-/jobs/308698) on the same runner with `-tags perfregress` (a 300 ms sleep per accept) reported `certified: false`, `failed_budget: 1`, p95 ≈ 311 ms. Host stability: the serial paths spread ≤ 1.06× across the three runs and MLLP-parallel 1.11×, inside the ≤ 1.15× rule; **HTTP-parallel spread 1.24×**, so the parallel figures are reported, not certified — `RunParallel` contention on four CPUs beside the database is scheduler-dependent (the Slice 4.4b finding), while the serial paths show the host itself is stable. **Scope**: the in-process durable-accept path — ingress and MLLP `Submit` against PostgreSQL — excluding the HTTP handler, bearer/HMAC validation, TLS, and MLLP framing. Ordinary CI still gates only `allocs/op`. |
| 2 | One-hour steady-state throughput on the reference profile | **Harnessed, uncertified** | Slice 4.4e's per-deployment MLLP rate quota has merged, and the profile job now reports this budget as `harnessed` with its gap stated in the report: it runs 300 accepts per benchmark in one process, while the budget is the declared 250 two-KiB messages per second sustained for one hour against two replicas of the reference profile. A one-hour variant of the single-process harness was considered and rejected on 2026-09-25 (decision entry): it would certify a different claim. |
| 3 | 1-GiB batch import peak memory above idle | **Not measured — sampler only** | "Harnessed" overstated this row until 2026-09-25: no workload in `internal/integration/perf` imports a batch, and `perf.HeapSampler` runs only in a 32 MiB unit test and reads `HeapAlloc`, not the cgroup RSS the budget is written in (a Go process's RSS includes heap the collector has freed and not returned to the OS, so RSS is a property of GC timing as much as of the workload). The profile job now reports it as `not_measured`. Certification needs its own slice: a 1-GiB batch-import workload on runner 8 reading peak RSS from the cgroup and proving restart from the last durable checkpoint. |
| 4 | Recovery time objective | **Not started — slice 4.4c** | 4.4a proved a `pg_dump`/restore round-trip preserves every durable row and trigger. It measured no recovery *time*. |
| 5 | Recovery point objective | **Known unachievable as configured — slice 4.4c** | `PRODUCTION-HARDENING.md` states it directly: logical dumps cannot meet a minutes-scale RPO, and nothing in this repository configures WAL archiving or PITR. |
| 6 | One-version rollback safety | **Certified — slice 4.4a** | N-1 defined per migration ledger, a real defect found and fixed (`0004_export_attribution.sql` made three columns `NOT NULL` with no `DEFAULT`, so rollback failed every session export), and a restore round-trip proof in CI. |
| 7 | Golden-journey evidence on Kubernetes 1.36 | **Not started — slice 4.4c** | Needs a cluster. |

**In ordinary CI, budgets 1, 2 and 3 are gated on allocations only, and that
gate is narrower than it sounds** — budget 1's wall-clock is asserted only by
the pinned-runner profile job cited in row 1. Measured over three runs, the durable accept path's allocation
count varies by about 2 in 4650 — stable, but not the bit-identical figure the
legacy micro-benchmarks report. A ceiling roughly 1% above the observed count
detects a regression of some 40 allocations per message. It will not notice one.
It is a regression detector for the thing that *causes* latency, not a
measurement of latency.

**How budget 1 was certified, and what remains.** `FI_FHIR_PERF_RUNNER=1` was
set on 2026-09-24. Runner 8's job pod — fixed at 4 CPU / 8 GiB plus its helper
and PostgreSQL service, 11 GiB of requests in all — could not schedule beside
the node's resident inference server until the operator lowered that server
pod's memory *request* in place for the run window (Kubernetes 1.33 pod
resize; the model was neither restarted nor scaled). The negative control ran
first, then three plays on identical code; the four `performance-report.json`
artifacts are the evidence cited in row 1, and a green ordinary MR pipeline
still supplies none of it. Budget 2 needs a one-hour two-replica run and
budget 3 needs a workload that does not exist yet; the profile job supplies
neither, and both stay open.

Until those gates pass, documentation must describe individual capabilities and
their evidence rather than label the whole product “1.0 certified,” “HIPAA
compliant,” or standards conformant.
