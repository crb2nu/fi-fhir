# fi-fhir Roadmap

> Last updated: 2026-07-16
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
- the restart-safe PostgreSQL Integration Session workspace now persists
  redacted samples, immutable profile revisions/runs, decisions, and exports;
  live streaming, workflow simulation, and publish/deploy remain Phase 3 work;
- a PostgreSQL-backed versioned deployment lifecycle now authorizes the optional
  production MLLP listener; profile/workflow bytes remain in the immutable
  startup registry and S3/SFTP discovery is not runtime-wired;
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

## Next

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

## Then

- [ ] Workflow simulation against durable session data.
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
