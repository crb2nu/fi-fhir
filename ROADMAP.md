# fi-fhir Roadmap

> Last Updated: 2026-09-20
> Tier: 1 (see workspace AGENTS.md "Portfolio Tiers")
> Tracking issue: https://gitlab.flexinfer.ai/libs/fi-fhir/-/issues/19
> Completion spec: `.loom/20-product-spec-integration-engine-ide-completion.md`
> Execution plan: `.loom/30-implementation-plan-integration-engine-ide-completion.md`
> Plan store: `plan-complete-fi-fhir-as-a-production-integration-engine-and-ide-341d98`

## Current Status

fi-fhir implements profile-driven parsing, canonical events, durable ingestion
and delivery, and a Mapping Studio for authoring and operating integrations.
It remains pre-1.0: implemented capabilities and repository tests do not certify
all supported deployment profiles or every vendor feed.

The merged runtime includes authenticated HTTP and MLLP ingestion, S3/SFTP batch
processing, PostgreSQL acceptance and replay, durable Integration Sessions,
workflow simulation, reviewable publication, deployment controls, and identity
and PHI policy. Kafka, Redis Streams, and Google Cloud Pub/Sub share event
handlers and acknowledgment-aware consumers. FHIR transactions use a shared
projector across workflow actions and durable destinations.

This update records repository state through merge `7e146de64` on 2026-09-20.
[Pipeline 27875](https://gitlab.flexinfer.ai/libs/fi-fhir/-/pipelines/27875)
passed 53 automatic jobs, including live HAPI FHIR read-back, Kafka/Redis,
PostgreSQL/MinIO, and two-replica tests. These are internal GitLab records.
Production activation and release certification remain separate decisions;
this documentation update does not assert a new clinical-runtime deployment.

## Delivered — Sprint 6 and clinical delivery (2026-09-20)

- [x] **S6-0 merge surface** — CI includes, proof targets, and lane boundaries
  merged in [MR !202](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/202).
- [x] **S6-A Slice 4.1c-c FHIR destination class** — conditional-write
  transaction Bundles and durable delivery provenance, merged in
  [MR !206](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/206).
- [x] **S6-C legacy E2E repair** — a blocking service-backed test job, merged in
  [MR !204](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/204).
  MR !211 adds live HAPI acceptance with fourteen top-level tests and no skips.
- [x] **S6-D Slice 5.1b** — pinned R4 4.0.1 and US Core 9.0.0 archives plus
  structural validation, merged in
  [MR !205](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/205).
  Official validator evidence is still required for conformance.
- [x] **Broker processors and common handlers** — Kafka, Redis Streams,
  Pub/Sub, workflow queue drivers, `workflow consume`, and the outbox adapter,
  merged in [MR !210](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/210).
  See [event backends](docs/operations/EVENT-BACKENDS.md).
- [x] **CDA profile event selection** — profile-configured emitted event types,
  merged in [MR !209](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/209).
- [x] **Clinical FHIR delivery** — condition, procedure, immunization, vital
  sign, medication request, and allergy intolerance events reach both delivery
  paths. Workflow selection and Patient/Encounter references are resolved;
  issue #20 is closed. Merged in
  [MR !211](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/211).
  See [FHIR output](docs/user-guide/fhir-output.md).

## Now

- [ ] **S6-B budgets 1–3 certification** — the performance harness is present,
  but measured certification on the pinned `fi-fhir-perf` runner remains open.
  An ordinary green MR pipeline does not certify these budgets.
- [ ] **Slice 4.2c FHIR operator trace** — expose destination provenance in the
  operator delivery attempt view.
- [ ] **Slice 5.1c official validation** — run the CI-only official validator
  over delivered Bundles and close the remaining structural fixture gaps.
  The FHIR destination prerequisite has merged; this work is no longer blocked
  on Slice 4.1c-c. See the [conformance matrix](docs/planning/FHIR-CONFORMANCE-MATRIX.md).

## Delivered — Phases 0–2

- [x] **Gate 0A — secure baseline** — MR !89 pipeline 18379 green on
  2026-07-12; lint, govulncheck, and gosec each passed individually.
  - Go 1.26.5 in module, CI, and container builds.
  - Go-1.26-compatible golangci-lint.
  - Event-store SQL identifier injection closed with regression coverage.
  - govulncheck, gosec, tests, build, MR pipeline green.
- [x] **Gate 0B — truthful delivery** — MRs !90–!92 and subsequent main
  pipelines proved benchmark, security, build, scan, and deployment truth; the
  Slice 1.1a main pipeline `18542` remained green across all 33 jobs.
  - UI, binary, smoke, live WebSocket, contract, codegen, and security jobs run
    when applicable and cannot pass by skipping their subject.
  - npm is the canonical UI package-manager path; frozen installs are reproducible.
  - deployment/status documentation matches executable behavior.
- [x] **Golden Path 001 foundation**
  - [x] Slice 1.0 locked the 1.0 support matrix, tenancy/identity/PHI/secret
    contracts, minimal immutable integration revision, and result invariants.
  - [x] Slice 1.1a made exact profile/workflow revision resolution immutable and
    proved v1-after-v2 reconstruction in required PostgreSQL CI.
  - [x] Slices 1.1b and 1.1c shipped in MR `!96`: one deterministic ADT A01
    kernel, one authenticated typed GraphQL/IDE adapter, exact origins,
    memory-only browser data, and fail-closed legacy operations. MR pipeline
    `18604` passed 30/30 jobs; main pipeline `18621` passed 33/33 and published
    matching `v0.1.18621` images. GitOps MRs `!368` and `!369` rolled out the
    verified digests, passed the public live gate, and resumed healthy image
    automation.
  - [x] Slice 1.2 added the PostgreSQL-only production committer: one transaction
    records the receipt, canonical event, lineage, initial attempt, and outbox
    work. MR `!98` job `181669` passed the blocking PostgreSQL 16 race/fault/restart
    proof, collapsing 64 callers to one raw-free durable admission unit.
  - [x] Slice 1.3 added the first authenticated production adapter at exact
    `POST /v1/hl7v2`, with bearer/HMAC credentials, server-owned integration and
    source identity, bounded bodies, structured failures, and PHI-free durable
    responses. `make golden-path-001` passed 20 assertions across duplicate,
    restart, profile-delta, PostgreSQL cardinality, IDE parity, and leakage gates.
    MR `!99` pipeline `18898` passed 32/32; main pipeline `18951` repeated the
    Golden Path proof and passed 35/35 on merge commit `48d156d2`.
- [x] **Phase 2 production channel runtime**
  - [x] Slice 2.1 adds digest-bound connection-validation freshness, schedules,
    health thresholds, and capacity to immutable integration revisions. Its
    PostgreSQL catalog enforces draft/validate/approve/publish/deploy/pause/
    resume/retire, optimistic versions, append-only evidence, immutable release
  records, health projection, and deployed-only exact revision resolution;
- optional runtime-wired S3/SFTP batch ingestion now streams concatenated HL7v2
  through the shared durable processor with PostgreSQL lease/checkpoint recovery,
  pinned SFTP host keys, and verified digest-addressed archive-before-delete;
    MR `!101` pipeline `19014` passed 32/32, including required PostgreSQL 16
    lifecycle job `183463`; merge commit `a95bb44f` repeated that proof in main
    job `183702`. The first main run also exposed an existing concurrent receipt
    primary-key arbitration defect. MR `!102` fixed it, pipeline `19045` passed
    24/24, and final main pipeline `19052` passed 26/26 with durable-submission
    job `183938` and lifecycle job `183940` independently green.
  - [x] Slice 2.2 adds a content-addressed UTF-8 MLLP source, fragmented/multi-
    frame transport, TLS 1.3 mutual authentication and CIDR policy, bounded
    capacity, safe application/commit ACKs, and optional `serve` composition.
    Each frame starts from the lifecycle catalog's exact deployed binding and
    repeats authorization inside durable admission before a positive ACK. MR
    `!104` pipeline `19175` passed 33/33, including PostgreSQL 16/TCP MLLP job
    `184996`; merge commit `6205fa39` repeated the proof in main job `185093`.
    Main pipeline `19193` passed 36/36. Production GitOps activation remains
    intentionally pending.
  - [x] Slice 2.3 adds durable delivery attempts, bounded retry/circuit policy,
    DLQ replay/resubmit, and a real Kafka publisher. MR `!106` pipeline `19226`
    passed 34/34, including kill-test job `185433`; main pipeline `19235` passed
    37/37 and repeated the proof in job `185505`. Evidence MR `!107` reconciled
    the exact proof on main.
  - [x] Slice 2.4 adds exact deployed S3/SFTP
    sources, bounded streaming, PostgreSQL lease/checkpoint resume, deterministic
    admission identity, pinned host keys, and verified digest archive semantics.
    MR `!108` pipeline `19331` passed 35/35, including required PostgreSQL 16/
    MinIO/SSH-SFTP job `186259`, and merged as `ed32915f`. Main pipeline `19344`
    passed 38/38 and independently repeated the proof in job `186476`.
    Evidence MR `!109` reconciles the canonical completion records. Production
    GitOps activation remains intentionally pending.

## Delivered — Phase 3 (3.1, 3.2)

- [x] Phase 3 Slice 3.1 restart-safe Integration Session Workspace — MR `!111`
  pipeline `19409` passed 37/37, including required PostgreSQL restart/raw-PHI
  job `187425`, and merged as `15746ccd`. Main pipeline `19424` passed 40/40
  and independently repeated the proof in job `187618`.
- [x] Phase 3 Slice 3.2 streaming diagnostics and server lineage: feature-gated
  authenticated GraphQL SSE, durable run reconciliation, Problems diagnostics,
  and canonical inspector lineage. MR `!115` pipeline `19464` passed 34/34,
  including required session job `187950` and benchmark job `187953`, and merged
  as `36f2bb8c`. Main pipeline `19482` passed 37/37 and repeated the session
  proof in job `188135`. Production GitOps activation remains pending.

## Delivered — Phase 3 (3.3, 3.4) and Phase 4 (Sprints 3–5, 2026-08-08 → 2026-09-05)

- [x] Workflow simulation against durable session data (3.3) and reviewable
  bundle publication with exact-revision promotion (3.4).
- [x] Operator control plane: GraphQL API (4.2a) and UI (4.2b) — trace browser,
  deployment controls, DLQ tooling.
- [x] Identity, authorization, and PHI policy (4.1a–4.1e, including the
  destination identity contract 4.1c-a and the HTTPS destination consumer
  4.1c-b), truthful observability and multi-replica behaviour (4.3), and the
  recovery/upgrade/performance family (4.4a migration compatibility, 4.4b
  performance harness, 4.4c chaos/DR, 4.4d structured logging, 4.4e
  per-deployment MLLP rate quota); purge role separation; 5.1a mapper/checker
  reconciliation. Execution specs `.loom/31`–`.loom/33`; evidence per slice in
  `.loom/worklog/`.

## Then — the 1.0 remainder
- [ ] 5.2 SMART Backend Services and Bulk Data 3.0.0; 5.3 extension and
  compatibility contract.
- [ ] Phase 6 release evidence: budget 7 on Kubernetes 1.36, all six golden
  journeys on the supported deployment profile, no open P0/P1.

## 1.0 standards and release scope

- FHIR R4 4.0.1 with pinned US Core 9.0.0 validation.
- SMART App Launch 2.2.0 and Bulk Data 3.0.0 conformance journeys.
- Supported Compose and Kubernetes deployment profiles, current/previous
  evergreen browsers, documented compatibility and migration policy.
- All six golden journeys and numeric security, accessibility, latency,
  throughput, memory, RPO/RTO, and upgrade gates pass.
- No open P0/P1 completion issue and no false production-readiness claim.

## Program gates

| Gate | Exit evidence |
|---|---|
| Gate 0A | Security baseline MR merged with terminal green pipeline |
| Gate 0B | Deliberate-failure proofs plus complete positive CI path |
| Engine Alpha | Golden Path 001 kill-test passes through one processor |
| Engine Beta | MLLP, durable delivery/replay, and operational trace pass |
| IDE Beta | Restart-safe author/test/publish/deploy journey passes |
| Release Candidate | Governance, accessibility, scale, DR, and upgrade gates pass |
| 1.0 | Six golden journeys pass on the supported deployment profile |

## Backlog

Full backlog: [P1 issues](https://gitlab.flexinfer.ai/libs/fi-fhir/-/issues/?label_name[]=P1) ·
[P2](https://gitlab.flexinfer.ai/libs/fi-fhir/-/issues/?label_name[]=P2) ·
[P3](https://gitlab.flexinfer.ai/libs/fi-fhir/-/issues/?label_name[]=P3) ·
[Milestones](https://gitlab.flexinfer.ai/libs/fi-fhir/-/milestones)
