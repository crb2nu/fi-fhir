# fi-fhir Roadmap

> Last updated: 2026-07-14
> Tier: 2 (see workspace AGENTS.md "Portfolio Tiers")
> Tracking issue: https://gitlab.flexinfer.ai/libs/fi-fhir/-/issues/19
> Completion spec: `.loom/20-product-spec-integration-engine-ide-completion.md`
> Execution plan: `.loom/30-implementation-plan-integration-engine-ide-completion.md`
> Plan store: `plan-complete-fi-fhir-as-a-production-integration-engine-and-ide-341d98`

## Current status

fi-fhir is a substantial healthcare integration **capability kernel**, not yet a
completed integration engine product. The Go backend contains profile-driven
parsers, canonical events, FHIR mapping, workflow actions, event-sourcing
primitives, terminology, GraphQL, and LLM features. The SvelteKit/npm Mapping
Studio contains real authoring, inspection, workflow, terminology, event, and
debug surfaces.

The remaining work is product assembly and operational truth:

- authenticated HL7v2 HTTP ingress is implemented but not yet activated in the
  production GitOps deployment; when enabled, one shared processor composes
  exact revision resolution, parsing, durable acceptance/idempotency, route
  planning, lineage, and transactional outbox admission behind PostgreSQL;
- durable Integration Sessions remain an in-memory, HL7-only prototype, while
  the current IDE preview now uses the same stateless, exact-revision kernel as
  GraphQL;
- a PostgreSQL-backed versioned deployment lifecycle now exists, but `serve`
  still uses the startup registry instead of that catalog; no production MLLP
  source exists and S3/SFTP discovery is not runtime-wired;
- a transitional single-domain preview bearer, exact-origin policy, and
  memory-only browser handling are deployed and live-verified; OIDC,
  fine-grained RBAC, audited token administration, and durable PHI policy remain
  incomplete;
- the current Flux deployment proves the authenticated ADT A01 preview and
  legacy-containment boundary, not the remaining completion journeys or the
  production-readiness contract.

The June 28 Integration Session Engine merge is preserved as useful foundation.
The July 12 completion review supersedes the earlier sibling-integration-first
sequence. flexinfer, mentatlab, and Loom integrations remain ecosystem work and
must not enter the clinical data plane before the engine spine is proven.

## Now

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
- [ ] **Golden Path 001 foundation**
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
- [ ] **Phase 2 production channel runtime**
  - [x] Slice 2.1 adds digest-bound connection-validation freshness, schedules,
    health thresholds, and capacity to immutable integration revisions. Its
    PostgreSQL catalog enforces draft/validate/approve/publish/deploy/pause/
    resume/retire, optimistic versions, append-only evidence, immutable release
    records, health projection, and deployed-only exact revision resolution.
    The required PostgreSQL 16 race/restart/immutability job is pending terminal
    MR evidence.
  - [ ] Slice 2.2 production MLLP consumes only the catalog's runnable binding.

## Next

- [ ] Production MLLP with bounded concurrency, TLS/client policy, and durable ACK/NACK.
- [ ] Durable delivery attempts, DLQ/replay/resubmit, and one real queue transport.
- [ ] Runtime-wired S3/SFTP streaming ingestion with checkpoint/resume.

## Then

- [ ] Restart-safe Integration Session workspace with exact artifact revisions.
- [ ] Live stage/diagnostic/lineage UI and workflow simulation against session data.
- [ ] Reviewable bundle publication and promotion of the exact tested revisions.
- [ ] Real operator message/trace browser, deployment controls, and DLQ tooling.
- [ ] Fine-grained RBAC, PHI retention controls, audit, readiness, metrics,
  multi-replica behavior, backup/restore, upgrade, DR, and performance.

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
