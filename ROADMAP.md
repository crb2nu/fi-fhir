# fi-fhir Roadmap

> Last updated: 2026-07-13
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

- production ingress does not yet compose profile resolution, parsing, durable
  acceptance/idempotency, workflow delivery, and traceability behind one runtime;
- Integration Sessions are an in-memory, HL7-only prototype and are not wired to
  the selected profile/workflow or live subscription in the shipped UI;
- no production MLLP source exists, S3/SFTP discovery is not runtime-wired, and
  `serve` loads one workflow instead of deployed integration revisions;
- auth/tenant/PHI policy, readiness/metrics, container entrypoints, and CI gates
  are incomplete or inconsistent;
- the current Flux deployment proves that artifacts can be deployed, not that
  the completion journeys or production-readiness contract are satisfied.

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
  - [ ] Slice 1.1b is proving the internal deterministic ADT A01 preview kernel;
    authenticated transport activation remains deliberately blocked.

## Next

- [ ] Complete and merge the deterministic `MessageProcessor` preview kernel.
- [ ] Add authenticated, POST-only GraphQL/IDE preview adapters, explicit HTTP
  and WebSocket origins, and fail-closed legacy submit/session containment.
- [ ] PostgreSQL receipts, idempotency, trace, and transactional outbox.
- [ ] Authenticated HL7v2 HTTP ingress and the restart/duplicate/IDE-parity kill-test.
- [ ] Versioned integration deployment lifecycle and production MLLP.
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
