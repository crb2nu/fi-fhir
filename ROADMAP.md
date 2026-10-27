# fi-fhir Roadmap

> Last Updated: 2026-10-27
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

Delivered FHIR Bundles are validated offline by the HL7 official validator in
CI against pinned R4 4.0.1 and US Core 9.0.0 packages; the findings ledger is
checked in and held by exact equality.

The Mapping Studio reports what its API grants it: `/api/auth/status` carries
the caller's roles, derived capabilities and missing roles, and every operator,
streaming and Copilot surface renders the honest state from that contract
instead of failing. The deployed shape is proven in CI by a blocking Playwright
smoke gate against the built UI, the production nginx template and real API
processes, with negative controls. The production deployment grants every IDE
identity the operator bundle and runs the Integration Session workspace with
SSE streaming on (2026-09-26).

The Mapping Studio was redesigned on 2026-09-26 (`.loom/37`): a design
system of neutral tokens and shared primitives, a single-row shell with
access and connection in the status bar, and every route rebuilt as a
toolbar over tables with details panes. No surface shows simulated data.
`test:ui-e2e` now captures every route at 1440×900 as review evidence and
fails if the retired marketing copy returns.

Source and destination connections are authorable in the IDE since
2026-09-27 (`.loom/38`): a durable catalog of drafts that compile, with the
existing document constructors, into the exact revision bytes `serve` mounts,
labelled against what each replica actually mounted, plus an Engine view of
the replica's composition and sample intake from a live source into an
Integration Session. Activation stays a GitOps rollout; whether the catalog
should ever activate configuration is the open question in `.loom/39`.

Since 2026-09-30 (`.loom/42`, IDE unlock) the Studio drives the lifecycle
catalog from draft to published: a Definitions tab on `/connections` authors
integration definitions through the same service the `lifecycle seed` CLI
uses (byte parity proved by a kill-test) and validates them in `serve`;
Operator shows deployment history, cross-receipt attempt search, paged audit
and the full trace; `/events` is Verification over the durable admissions;
Integration Sessions reopen, export and archive; and the shell's journey
reads evidence instead of route position.

The public demo program (`.loom/40`, 2026-09-27 → 2026-09-29) compiled the
slim preview kernel to WebAssembly, put the flexinfer-site playground on that
kernel, covered the new features in the docs, added the preview-only hosted
demo identity and stack, and closed the two production findings its survey
raised (the workflow debugger's caller-supplied actions and
`createFhirSubscription`'s unrestricted egress).

In October the Studio's daily workflow was reworked (2026-10-03, MRs !271–!276):
a collapsible explorer with Build and Operate navigation, editor tabs and
commands that keep their session and record context, unsaved-work protection
across every navigation path, saved-session browsing on bounded queries, and
keyboard focus that survives closing a tab. The security baseline moved to Go
1.26.9 and `golang.org/x/net` v0.60.0 for the 2026-10-08 advisories, with the
UI tooling and transitive dependencies patched the same day (MR !277), and the
README's diagrams were replaced with composed vector figures (MR !278). On
2026-10-27 govulncheck and trivy, which scan every merge request against a
moving database, refused those pins (GO-2026-6809 in `crypto/x509`,
GHSA-cg2g-xh64-v4q7 in DOMPurify), so Go 1.26.10 and DOMPurify 3.4.17 rode
[MR !280](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/280).

This update records repository state through merge `a9be5577a` on 2026-10-09
(main pipeline 32232, green). The previous record was merge `d996cd610` on
2026-09-30, when production answered `v0.1.30316` from the LAN; the image
production runs today was not re-verified for this update. Three merge
requests are open: !279 (`serve` tracing and lineage logs, 2026-10-10,
pipeline 32379 green, mergeable, unreviewed), and !246 and !253 (the
operator-bundle browser gate's response waits, 2026-09-27, mergeable but with
no pipeline run since). These are internal GitLab records. Production
activation and release certification remain separate decisions; this
documentation update does not assert a new clinical-runtime deployment.

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

## Delivered — Sprint 7 (2026-09-24 → 2026-09-25)

Spec: `.loom/35-sprint7-execution-specs.md`
([MR !214](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/214)).

- [x] **Slice 5.1c-α mapper cardinality gaps** — the seven US Core
  cardinality violations the structural validator recorded in Slice 5.1b are
  closed from event data or US Core's `unknown` data-absent reason, never
  invented; a `DocumentReference` with no attachment is no longer emitted;
  `Encounter.type` derives from PV1-2 through the pinned US Core binding;
  `recordedCardinalityGaps()` is empty and all 25 fixtures validate clean.
  Merged in [MR !216](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/216).
- [x] **Slice 5.1c-β official validator gate** — HL7 `validator_cli.jar`
  6.10.4 runs **offline** in the blocking `test:fhir-official` job over all
  25 mapper fixtures and the 11 transaction Bundles the durable `fhir`
  transport actually delivers, against R4 4.0.1 and US Core 9.0.0 with the
  full 23-archive package closure pinned by digest. Findings are held to an
  exact-equality ledger (`testdata/fhir/official/findings.ledger.txt`) that
  can only shrink deliberately; a negative control proves the gate is live.
  Merged in [MR !219](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/219).
  See [conformance matrix §5.2](docs/planning/FHIR-CONFORMANCE-MATRIX.md).
- [x] **Slice 4.2c FHIR operator trace** — `OperatorDeliveryAttempt.deliveries`
  projects the destination provenance ledger (transport, outcome, verified
  revision, endpoint and certificate advisories, and the FHIR resource types,
  entry count, and OperationOutcome issue codes) into the operator message
  trace and delivery console; ledger-only, clinical-content-free by
  construction. Merged in
  [MR !217](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/217).
- [x] **Performance harness fix (S6-B, partial)** — the report script could
  set `certified: true` on the runner variable alone, comparing no
  measurement to any budget; it now requires budget 1's p95/p99 to be met on
  the tagged runner in a non-regression build, records the negative control
  and runner id, and reports budget 3 as `not_measured`. Merged in
  [MR !215](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/215).
  **Nothing is certified**: see Now.
- [x] **CI: MinIO service images mirrored** — quay.io and Docker Hub withdrew
  `minio/minio`; both pinned service images now come from the internal
  registry by digest. Merged in
  [MR !218](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/218).
- [x] **Close-out (`docs/sprint7-close-out`)** — this roadmap, the plan's 5.1
  block, one CHANGELOG block, and Lane S7-A's banked one-liner:
  `fhirout.mapEvent` now tells the mapper the event source, so the legacy
  workflow `fhir` action's raw Encounter carries the same
  `urn:fi-fhir:source:<source>` identifier system the durable transport
  already added through `ensureIdentifier`; the official-validator ledger is
  byte-identical.
- [x] **S6-B budget 1 certified on runner 8 (2026-09-25)** — the negative
  control ran first (job 308698, `-tags perfregress`, `certified: false`,
  `failed_budget: 1`), then three plays on identical `main` code (jobs 308841,
  309005, 309029, all `certified: true`, runner id 8): serial accept p95 under
  10 ms and parallel under 51 ms against a 250 ms target. Serial-path spread
  ≤ 1.06× across runs; HTTP-parallel spread 1.24× is reported, not certified.
  The run window was opened by lowering the resident model pod's memory
  request in place, with no model restart. See
  [SUPPORTED-1.0](docs/operations/SUPPORTED-1.0.md) rows 1–3.

## Delivered — IDE repair program (2026-09-25 → 2026-09-26)

Spec: `.loom/36-ide-repair-execution-specs.md`
([MR !223](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/223),
corrections in
[MR !224](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/224)).
Every symptom was measured against the live deployment from the LAN before a
lane was launched; none was in the trusted-network path itself.

- [x] **R-0 the operator bundle (platform/gitops MR 805)** — the deployment
  granted only the transport grant `graphql:operator`, so the gate admitted
  every operator query and the service refused every one. Every IDE identity
  (static bearer, trusted network, both Access principals) now holds
  `integration.operator`, `integration.delivery.operator` and
  `integration.deployment.operator` as well. No code change; decision
  "Grant the operator bundle rather than alias the transport grant".
- [x] **R-A the auth capabilities contract** — `/api/auth/status` reports
  `principal`, `roles`, derived `capabilities` (operator read/delivery/
  deployment, clinical read, integration sessions, streaming, the allowlisted
  `subscriptions`, `llm.configured`) and `missingRoles`, checked in tests
  against the real operator service; `serve` warns once per identity holding
  the grant without the bundle. Merged in
  [MR !225](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/225).
- [x] **R-B honest surfaces** — the operator page pre-flights on
  `capabilities.operatorRead` and names the missing roles; streaming surfaces
  are keyed per subscription root and the four legacy panels never subscribe
  (the SSE allowlist admits only the two session roots by design); the session
  engine runs only when the build flag is on and the API reports
  `integrationSessions`; the Copilot runs on the backend LLM capability, the
  loom platform chrome hides unless `PUBLIC_LOOM_ENDPOINT` is set; the Problems
  badge counts only a live draft. Merged in
  [MR !226](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/226).
- [x] **R-C streaming on, repo side** — `ui/Dockerfile` ships the session
  engine on; production and runbook sections in `docs/operations`; the SSE
  client keeps the `FORBIDDEN` code the server's catalog-safe presenter leaves
  in place of the stream-specific message; an end-to-end "flag on, API off"
  test through the real gate, store and pages. Merged in
  [MR !228](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/228).
- [x] **Production streaming (platform/gitops MR 811)** — one env entry,
  `FI_FHIR_INTEGRATION_SESSION_ENABLED=true`; the store migrated itself into
  the durable PostgreSQL; no retention key (raw retention stays refused),
  signed publication off. Verified from the LAN: `integrationSessionEvents`
  answers `200 text/event-stream`. Decision "Streaming flips before the UI
  image; the stream allowlist stays narrow".
- [x] **R-D the browser smoke gate** — blocking `test:ui-e2e`: Playwright
  against the built UI served by the production nginx template in front of
  three real `fi-fhir serve` stacks on PostgreSQL (the bundle with sessions
  on; the bundle minus `integration.operator`; sessions off), with an
  existence guard so a renamed check cannot make the gate greener;
  `make ui-e2e` mirrors it. First CI run 7/7 in 249 s. Merged in
  [MR !227](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/227).
- [x] **R-E HL7 intake line endings** (R-D's finding) — intake sends
  CR-terminated segments on every path whatever the editor holds, and the
  built-in sample is an executable ADT^A01 for the first time (its separators
  were the literal characters `\r`). The kernel's strict line-ending gate is
  unchanged. Merged in
  [MR !229](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/229).
- [x] **Close-out (`docs/ide-repair-close-out`)** — this roadmap, one
  CHANGELOG block, the decision entry, and `ci/test-ui-e2e.yml` running when
  its own definition changes.

## Delivered — IDE design uplift (2026-09-26)

Spec: `.loom/37-ide-design-uplift-execution-specs.md`
([MR !232](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/232)).
The brief was that the Mapping Studio "looks like a toy". The evidence was
1440×900 screenshots of every route in production: four navigation layers
above the content, marketing copy, simulated alerts on the home page, and
gradients, glows, pills and cards where records belonged. The direction is
calm, dense and precise; every lane attached before/after screenshots and was
reviewed screenshots first, then diff.

- [x] **U-0 design system** — dark-first neutral tokens with one accent
  reserved for focus, selection and primary actions; 13 px UI type, 4 px
  grid, 28 px controls, 36 px toolbars; primitives in `ui/src/lib/ui/primitives/`
  (Button, IconButton, Tabs, Badge, Panel, Toolbar, Table, Field/Input/Select/
  Textarea, EmptyState, KeyValue, Popover, Icon), Lucide icons, a dev-only
  `/design` gallery and `ui/docs/DESIGN.md`. Merged in
  [MR !233](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/233).
- [x] **U-1 shell** — one 40 px header with the five-stage control, a
  breadcrumb and Commands; access and connection live in the status bar (the
  credential strip and journey band are gone, the bearer entry is a centred
  dialog); document types with no editor removed; a layout saved on Operator
  restores. Merged in
  [MR !235](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/235).
- [x] **U-2 home, events, operator** — Home is Recent, Integrations, Health and
  Alerts on real sources; the simulated observability fallback is deleted, so
  no surface can show invented alerts; Events and Operator are toolbars over
  tables with details panes. Merged in
  [MR !236](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/236).
- [x] **U-3 intake, workflows, profiles, terminology** — HL7 intake is
  editor-first with one status line; workflows, profiles and terminology are
  toolbars over tables with details panes. Review found a failed preview
  invisible with the session engine on and a lost per-row sample edit; both
  fixed before merge. Merged in
  [MR !234](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/234).
- [x] **U-4 visual evidence in the gate** — `test:ui-e2e` captures 1440×900
  PNGs of every route as job artifacts and fails if the retired marketing copy
  returns; no pixel goldens. Merged in
  [MR !237](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/237).
- [x] **Production** — UI `v0.1.29328` verified from the LAN at 1440×900 on
  every route, including an HL7 Preview on the session engine.
- [x] **Close-out (`docs/ide-uplift-close-out`)** — this roadmap, one
  CHANGELOG block and the worklog entry.

## Delivered — Connections and engine properties (2026-09-26 → 2026-09-27)

Spec: `.loom/38-connections-execution-specs.md`
([MR !240](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/240)).
The brief was to view and configure the engine's properties, define source and
destination connections, and sample messages from a source to build a profile
against. Connections had no durable home and no API: sources, destinations and
definitions were immutable JSON documents `serve` mounted at startup. The
program makes them authorable, compilable, exportable, and honestly labelled
against what the running replica mounted; it does not hot-load anything
(Decision 1).

- [x] **C-0 connection catalog, engine runtime, capabilities** —
  `internal/integration/connection`, the seventh forward-only ledger
  (`integration_connection_schema_migrations`): drafts under an optimistic
  version, append-only revisions compiled by the existing MLLP, batch and
  destination constructors (plus a new content-addressed HTTP source document)
  and stored as exact bytes, the capture audit table, secret bindings as
  references only with a write gate that refuses secret material; GraphQL
  catalog fields and the allowlisted, secret-free `engineRuntime`;
  `/api/auth/status` gains `connectionsRead`, `connectionsWrite`,
  `controlPlane` and `connectionCatalog`; `test:connection-catalog`. Merged in
  [MR !241](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/241).
- [x] **C-1 `/connections`** — Sources, Destinations and Engine tabs built only
  from the design-system primitives: tables with the honest status (Draft,
  Compiled rN, Referenced, Deployed, Mounted here), a settings form generated
  from each kind's spec with server problems on their fields, a Secrets table
  with no value column, reason-required save/compile/archive, Download and
  Copy JSON of the exact `revisionJson`, and an Engine view with the env key
  behind every property. Merged in
  [MR !243](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/243).
- [x] **C-2 sample intake backend** — `peekBatchConnection` (no lease,
  checkpoint, archive or delete; proved byte-identical on MinIO) and an
  admission-time capture tap on the MLLP listener and HTTP ingress that never
  changes a result, ACK or receipt, with a 2 s armed-capture cache, a
  `SKIP LOCKED` slot protocol, one armed capture per source, a superset capture
  redactor (113 Safe Harbor fields, dates whole), a peek secret allow-list, and
  `test:connection-capture`. Merged in
  [MR !242](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/242).
- [x] **C-3 sample intake UI** — **From connection…** in HL7 intake's Samples
  panel: mounted and catalog sources with their state, an armed-capture row
  that polls while armed, audited batch browse and read, and captured samples
  that open from `redactedPayload` and preview by `sampleId`. Merged in
  [MR !245](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/245).
- [x] **C-4 close-out (`docs/connections-4-close-out`)** — the
  [Connections guide](docs/user-guide/connections.md),
  [Connection catalog operations](docs/operations/CONNECTION-CATALOG.md), the
  lifecycle doc's exposure line, seven ledgers in `AGENTS.md`, this roadmap,
  the decision and worklog entries, and the new captures in
  `ui/docs/DESIGN.md`. Merged in
  [MR !248](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/248).
- [x] **`.loom/39` configuration-plane brainstorm** — reopens Decision 1
  deliberately; see Then. Merged in
  [MR !244](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/244).

## Delivered — IDE unlock (2026-09-29 → 2026-09-30)

Spec: `.loom/42-ide-unlock-execution-specs.md`
([MR !263](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/263)). The brief was to keep improving the Studio "to unlock the power
of our backend": production had a real definition, receipts and deliveries
from the demo hospital flow, and 44 of 145 GraphQL root fields never reached
the IDE. Every surface says what it knows, what it cannot know on this
deployment, and why; new reads ride existing roles, new writes ride
`integration.deployment.operator`. The E-1 kill-test (seed and editor author
the same bytes; `serve` hosts validation in-process) passed before any
authoring UI shipped. Decision entry 2026-09-30, "IDE unlock: definitions on
connections, refs from the runtime resolver, verification over durable
admissions".

- [x] **E-4 shell honesty** — journey stages complete on evidence (a run, a
  published profile, a mapping, a published workflow, an accepted receipt),
  "unknown" when not readable; one command registry and palette on a new
  `Dialog` primitive; `TabItem.badge`, dirty tabs, split pane removed, legacy
  components deleted. Merged in [MR !265](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/265).
- [x] **E-5 workflow authoring** — inline validation instead of toasts, a
  faithful YAML round trip with a YAML-only fields panel, rename/archive/
  restore of workflow definitions, run traces in the Trace panel, `WarningList`
  on primitives with Accept fix, `PID-3[0].1`. Merged in [MR !266](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/266).
- [x] **E-0 operator depth** — deployment history and `validationCurrent`,
  attempt search with an inspector and paged audit, the full trace (resubmit
  chain, lease, ledger row, tombstones), fleet observations, pre-flight, deep
  links, and an e2e fixture written by the real admission path. Merged in
  [MR !267](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/267).
- [x] **E-3 sessions** — `/hl7?session=` reopens a session; a rail with runs,
  diagnostics, the live run stream and publications; Accept fix; archive; a
  PHI-minimal export behind `capabilities.phiExport`. Merged in [MR !264](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/264).
- [x] **E-2 verification** — `/events` reads `integration_canonical_events`
  and receipts (`operatorCanonicalEvents`, `operatorAdmissionStatistics`,
  migration `0006_verification_reads`): Admissions, Statistics, Retention;
  the legacy Events views are deleted. Merged in [MR !269](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/269).
- [x] **E-1 definition editor** — `internal/integration/lifecycle/authoring`
  shared by the seed CLI and seven GraphQL operations; `serve` validates
  (REAL for its mounted batch source, STATIC, SKIP with a reason);
  `capabilities.definitionAuthoring`; the Definitions tab from New definition
  to Publish, linked to Operator to deploy. Merged in [MR !268](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/268).
- [x] **E-6 close-out (`docs/unlock-6-close-out`)** — this roadmap, the
  decision and worklog entries, the spec's status, `docs/STATUS.md` rows.

## Delivered — Public demo, portfolio links, docs coverage (2026-09-27 → 2026-09-29)

Spec: `.loom/40-public-demo-execution-specs.md`
([MR !247](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/247)).
The brief was a public demo like edilint's browser playground, linked from the
portfolio sites, with the docs covering the new features. The size kill-test
passed first: the slim preview kernel links at 1.49 MB gzip, and the whole
processor is 12.5 MB only because `workflow_plan.go` pulls in the workflow
engine.

- [x] **D-0 WASM kernel** — `cmd/fi-fhir-wasm`: the production Source
  Profile compiler, HL7v2 parser and conditional FHIR transaction Bundle
  projection compiled to WebAssembly (2.24 MB gzip) with no filesystem or
  network, held byte-identical to the Integration Session runner on the six
  demo samples; `test:wasm` gates the dependency closure and the size budget,
  and `v*` tags on the GitHub mirror publish `fi_fhir_<version>_wasm.tar.gz`.
  Merged in `2e7b56927`.
- [x] **D-4 hardening** — SEC-2026-09-27-1 and -2 closed: the workflow
  debugger no longer executes caller-supplied actions (`exec` never, the
  others only when named in `FI_FHIR_WORKFLOW_DEBUG_ACTIONS`), the `exec`
  action needs the deployment-owned `FI_FHIR_WORKFLOW_EXEC_ALLOWLIST`, and
  subscription mutations contact only https hosts listed in
  `FI_FHIR_FHIR_SUBSCRIPTION_ALLOWED_HOSTS`, with bounded session and client
  maps. Merged in `2bf7145b6`. The fi-fhir NetworkPolicy half is Cody's (see
  Then).
- [x] **D-3 preview-only stack** — the hosted demo's identity
  (`integration:preview` only, no database, no control plane, no sessions, no
  LLM) as a fourth browser-gate stack that walks every route for its honest
  state; pages pre-flight the role they need instead of toasting "forbidden";
  every e2e stack runs `serve --no-introspection`; a `0.0.0.0/0` trusted CIDR
  is refused unless `FI_FHIR_GRAPHQL_TRUSTED_CIDRS_ALLOW_ANY=true`. Merged in
  `0150f156d`; the demo serves at fi-fhir-demo.flexinfer.ai.
- [x] **D-2 docs coverage** — the Browser Playground and Mapping Studio user
  guides (the January `ui/docs/USER-GUIDE.md` is a pointer now), with the root
  README and the user-guide index listing Connections, engine properties,
  sample intake, the playground and the hosted demo. Merged in `0240983e0`;
  the end-to-end demo guide, its seed path and sample shape followed on
  2026-09-29 (`59c345c0a`, `9bc8e8639`, `a6f456c11`).
- [x] **D-1 playground on the real kernel** — flexinfer-site runs
  `/playground/fi-fhir` on the WASM kernel and links it from the hub rows and
  the docs; delivered in the flexinfer-site repository (its MRs !166 and
  !168).

## Delivered — Workspace navigation, security baseline, diagrams (2026-10-03 → 2026-10-09)

No spec; the brief was the overall navigation and daily workflow after the IDE
unlock. The worklog entries dated 2026-10-03 carry the evidence per change;
the six `codex/ux-workspace-navigation` merges are MRs !271–!276
(`c9eb77e3b` … `25c3ea300`).

- [x] **Workspace explorer and drawer** (`8984674fc`) — a collapsible left
  explorer with Build and Operate navigation, filtering and recent intake
  sessions; its desktop and section choices persist, and narrow screens get a
  keyboard-accessible drawer that closes after navigation.
- [x] **Navigation keeps its context** (`1bc3e370d`) — editor tabs retain the
  latest session and record selectors; the activity bar and Go to commands
  resume the same location; closing a tab returns to its neighbour's record;
  Cmd/Ctrl+K works from inputs and CodeMirror and returns focus on Escape; at
  390 px the stage control keeps Commands and Theme in view.
- [x] **Explorer continuity and intake panes** (`3545b0ede`, `896a72c0f`) —
  saved tabs and record links restore before preferences are written;
  switching intake sessions loads the selected session's runs without
  replacing edited text, and a late response cannot overwrite the new
  context; the intake panes stay usable as the workspace narrows.
- [x] **Drafts and records across navigation** (`767eb04cf`, `30e838ac6`) —
  connection forms, profile YAML and managed workflow edits survive navigation
  and ask once before leaving; the Explorer opens real source and destination
  records, and connection, definition, receipt and attempt links stay aligned
  with the visible record through same-page navigation and browser history,
  with explicit archived and unavailable states.
- [x] **Saved-session browser** (`3fab67340`) — Home and the Explorer search
  and page through saved sessions, archived work included, on bounded
  metadata queries instead of expanding every session and its runs. The
  OpenTelemetry SDK moved to v1.45.0 for GO-2026-6505 (`8c986e042`).
- [x] **Keyboard focus on tab close** (`f09243b4e`) and the combined final UX
  verification record (`769335f8d`).
- [x] **Security baseline** ([MR !277](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/277)) —
  Go 1.26.9 in the module, CI and container builds and `golang.org/x/net`
  v0.60.0 for the 2026-10-08 standard-library and HTTP/2 advisories
  (`f828430c8`); Vitest 4.1.11, Busboy and DOMPurify patched, the SvelteKit
  cookie dependency constrained to 0.7.2, and the proposed braces
  nesting-depth fix carried in a documented private fork under `ui/vendor/`
  so the audit passes without suppressions (`0dd43717d`); the README's
  workspace guide refreshed and its diagrams regenerated (`af193537c`).
- [x] **Composed diagrams** ([MR !278](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/278)) —
  the rough auto-layout figures replaced with composed vector figures from
  `docs/diagrams/narratives.yaml` (`43b3d160a`).

## Now

- [ ] **Land or close the three open merge requests** —
  [MR !279](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/279)
  (opt-in OTLP/HTTP tracing for admission, planning and claimed delivery,
  joined to the durable receipt and ledger ids, with a private provider and a
  PHI allowlist proven in the structured-logging job; 34 files, pipeline
  32379 green, mergeable) needs a review of the exporter lifecycle and the
  allowlist, then a merge.
  [MR !246](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/246) and
  [MR !253](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/253) fix
  the e2e timing item below and have sat since 2026-09-27; rebase, re-run and
  land them (!253's helper supersedes !246's inline wait once both are in).
- [ ] **edilint pins** — `lint:edi` runs edilint v0.3.0 and `lint:edi-fixtures`
  v0.1.0 (`ci/`, `Makefile`); edilint is at v0.4.1 with v0.5.0 ready to cut
  (synthetic `gen` fixtures with targeted defects, EDIFACT trailer recounts).
  Bump both pins to v0.5.0 when it exists, then use `edilint gen` to grow the
  `testdata/edi` corpus (issue #15).
- [ ] **Mills merge train for this repository** — see Mills posture below:
  the lane flips to `merge_ref` once the CI proof-profile arm is on main;
  watch the first rows' `mergequeue.train_built` events and the
  `train_tree_mismatch` counter, then decide the watcher scope and the hub
  queue scope.
- [ ] **Stale capabilities in an open tab** — a tab reads `/api/auth/status`
  once at load; after a rollback of the streaming flip, or after a bearer is
  entered, it keeps the capabilities it loaded with. Re-probe the status when
  a session mutation is refused with "legacy integration execution is
  unavailable" and after credential entry; until then, reload the tab
  (documented in `INTEGRATION-SESSIONS.md`).
- [x] **Copilot: enable the backend LLM in production** — closed outside the
  IDE unlock program: `/api/auth/status` reports `llm.configured: true` in
  production (`.loom/42`'s 2026-09-29 read, repeated from the LAN on
  2026-09-30). The item as written: the IDE said "No LLM is configured for
  this deployment" because the runtime enables LLM features only with
  `FI_FHIR_LLM_ENABLED=true` and the in-cluster LiteLLM refused keyless
  calls; `fi-fhir-api` now carries `FI_FHIR_LLM_ENABLED=true` and an
  `LLM_API_KEY` from a secret, so the remaining check is `copilot-llm-state`
  = ready in the Copilot panel.
- [ ] **Terminology-store availability as a capability** — terminology without
  a mapping store shows only "GraphQL request failed" and an error toast,
  because the API's reason ("terminology mapping store not configured") does
  not reach the UI. Add it to the status contract and say so on the page; then
  give the `test:ui-e2e` operator-bundle stack a mapping store so its
  Terminology capture shows a real Browse view. (The operator-plane half of
  this item closed with the Connections program: `/api/auth/status` reports
  `controlPlane` and `connectionCatalog`, so "not configured on this
  deployment" is told apart from "forbidden".)
- [ ] **Stream capture of real ADT feeds** — the v1 production kernel admits
  only ADT^A01 frames with MSH/EVN/PID/PV1 (+NTE); a frame carrying NK1, IN1 or
  any other segment is answered `AR` and never durably accepted, so a stream
  capture of a typical ADT feed sits armed with nothing captured until it
  expires. Batch peek is unaffected. Widening admission is a kernel change with
  its own profile-compile rules (`processor/profile_compile.go` refuses the
  unknown-segments tolerance).
- [x] **Engine properties the Engine tab cannot show** — `serve --help` and the
  `engineRuntime` allowlist now cover the session, delivery identity and SFTP
  passphrase keys, plus `FI_FHIR_CONNECTION_SECRET_*` as the allowlist's one
  prefix rule (`docs/operations/CONNECTION-CATALOG.md`).
- [ ] **`operator-bundle` e2e timing budget** — check 2's 10 s
  `waitForResponse` starts before `page.goto`, so a slow first load on a
  saturated runner consumes it (one retry on MR !242's pipeline; 15/15 locally).
  Fixed in [MR !246](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/246)
  (each wait armed at the action that issues the request) and
  [MR !253](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/253)
  (the stream-open and run budgets separated), both open since 2026-09-27;
  closes when they merge. Until then the Mills queue retries `test:ui-e2e`
  once before evicting a row (gitops, 2026-10-27).
- [x] **Events browser reads the legacy store** — replaced by Verification
  over durable admissions ([MR !269](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/269)): `/events` reads
  `integration_canonical_events` joined to receipts, with Statistics and
  Retention from columns; the legacy Browse, Patient Timeline, Statistics and
  Live views are deleted.
- [x] **IDE polish follow-ups from the design uplift** — all seven closed by
  the IDE unlock: `TabItem.badge` (the bottom panel and editor tabs are
  `Tabs`), one palette registry, the split workspace removed, and
  `ConfirmModal` on the `Dialog` primitive ([MR !265](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/265)); HL7 intake registers
  into that registry and its shim is deleted ([MR !264](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/264)); `WarningList` on the
  primitives, Problems counting the workflow draft only once it differs from
  the default, `yamlToDraft` keeping nested action config, and `PID-3[0].1`
  resolving ([MR !266](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/266)). (The Health panel's build-tag
  mismatch closed in
  [MR !238](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/238):
  UI and API builds now carry the Harbor tag Flux deploys.)
- [ ] **Connection rows: per-connection replica counts** — the connection
  catalog's rows do not select `ConnectionRuntimeState.observedReplicas` /
  `totalReplicas` (shipped in MR !261); only Operator's fleet views count
  replicas, client-side from `engineRuntime.observations` (`fleet.ts`).
  Select them on `ConnectionUsage` and say "mounted on n of m".
- [x] **`?connection=` deep links are read by no page** — closed 2026-10-03
  (`30e838ac6`): `/connections?connection=<id>` selects the record, stays
  aligned through same-page navigation and browser history, and shows explicit
  archived and unavailable states (`ConnectionsPage.test.ts`).
- [x] **Connection edit buffer is never marked dirty** — closed 2026-10-03
  (`767eb04cf`): the source, destination and definition editors report
  `ondirtychange` and the page publishes it through
  `setDraftState('/connections', …)`, so closing the tab asks once.
- [ ] **Definition authoring follow-ups** — `integrationRegistryArtifacts`
  silently omits registry entries that do not resolve (needs a schema field
  for the count and reasons); `lifecycle.ListDefinitions` refuses a limit
  above 500 and has no paging; the planner refusal
  (`authoring.WorkflowDestinationError`) still says "--destination artifacts"
  when it reaches the IDE; `serve` builds the batch validator's credentials
  before it knows whether definition authoring is composed.
- [ ] **E1-1 cannot assert STATIC → validated** — the operator-bundle stack
  mounts no compiled catalog revision (its HTTP ingress side processes bind
  the golden registry's `adt-east`, whose digest is no catalog revision, and
  checks 8 and 9 pin "no adapter" on the bundle replica), so E1-1 records
  STATIC as `SOURCE_NOT_MOUNTED` and carries the draft on with SKIP. Proving
  the positive path needs a stack that mounts a compiled connection revision
  without breaking checks 8 and 9.
- [ ] **Workflows: Run event calls a refused mutation** — Inventory's **Run
  event** still calls `triggerWorkflow` (`WorkflowList.svelte`, re-checked
  2026-10-27), which shipped builds refuse
  (`legacyUnsafeExecution`), so `workflowRuns` (and Open trace) is normally
  empty on `serve`. Remove it or route it through a session simulation.
  `markWorkflowBuilderOpened` now has only test callers; retire or rename it.
  `EventLineagePanel` has no component test, and E5-1 does not assert the
  "Unsaved changes" state after an edit.
- [ ] **Shell accessibility nits (E-4 review 7–11)** — the `Dialog` focus
  fallback when nothing inside is focusable; Escape handled before focus
  enters the dialog; tab close buttons sit inside the `tablist`; the Copilot
  control dims without saying why. (The fifth nit, Cmd/Ctrl+K ignored while
  focus is in an input or editor, closed 2026-10-03 in `1bc3e370d`.)
- [x] **Journey evidence reads every session** — closed 2026-10-03
  (`3fab67340`): journey evidence and the session lists use bounded metadata
  queries; `journeyState.ts` no longer calls `fetchRecentSessions` with
  `MAX_SAFE_INTEGER`.
- [ ] **Operator deep-link polish (E-0 review N1, N6)** — a deep link is read
  once on mount, before the capability pre-flight has necessarily settled;
  the attempt inspector links the resubmit parent but not its children.
- [ ] **`TestExecAction_RunsAllowedCommand` flakes under load** —
  `internal/workflow` failed once under full-suite `go test ./...` in four
  IDE-unlock lanes' local runs and passed on rerun each time (package
  untouched). Make its timing independent of host load.
- [ ] **Budgets 2 and 3** — budget 2 needs a one-hour, two-replica run at the
  declared 250 msg/s (the single-process harness cannot certify it); budget 3
  needs a 1-GiB batch-import workload reading cgroup RSS on runner 8. The
  `test:performance-profile` job schedules on runner 8 only when
  `cblevins-5930k` has 11 GiB of schedulable headroom; the in-place resize
  recipe that opened the 2026-09-25 window is in that day's decision entry.
- [ ] **Slice 5.1c-γ official-validator findings** — the ledger records 157
  findings (28 errors) over 36 inputs after Slice 5.1c-α: FHIRPath
  invariants, identifier and URL datatypes, local code-system membership,
  in-Bundle reference matching, and two DiagnosticReport slices. Cardinality
  errors are zero. Closing these is the next mapper slice; VSAC-bound value
  sets stay "not found" warnings under `-tx n/a` by decision.
- [ ] **Coverage.relationship for dependents** — stays `unknown` until
  `EligibilityResponseEvent` carries the X12 INS02 relationship code.

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
- [x] **Integration definition editor** (`.loom/38` Decision 5) — delivered
  by the IDE unlock ([MR !268](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/268)): the Definitions tab binds a compiled source
  revision, a registry profile/workflow pair and compiled destination
  revisions into a draft, then validates, approves and publishes it; deploy
  is on Operator. Caveat (`.loom/42` Decision 2): profile and workflow refs
  come from the static registry, proven by the runtime's own resolver,
  because that is what admission resolves today. Moving resolution onto the
  lifecycle catalog, which removes the two-truths hazard, stays with the
  configuration-plane spec below.
- [ ] **Retire the legacy event GraphQL fields** — `events`,
  `patientTimeline` and `eventStatistics` have no IDE caller since
  [MR !269](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/269); `eventStream` is referenced only by HL7 intake's **Live events**
  panel, which renders the honest unavailable state on `serve` and never
  subscribes. Their only writer is the test-only legacy `submitMessage`
  path. Decide deprecation (schema `@deprecated`, then removal with the
  projection service) rather than keeping four dead read paths.
- [ ] **Configuration-plane execution spec** — turn
  `.loom/39-brainstorm-config-plane-2026-09-27.md`
  ([MR !244](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/244))
  into a `.loom/` execution spec (reserved `.loom/41`) if chosen; it now also
  owns moving profile and workflow resolution onto the lifecycle catalog. The
  recommendation is a provisioned GitOps baseline plus a
  catalog overlay for the hot-reloadable class, observed per replica, with
  operational verbs always in the database and listeners/TLS kept in GitOps
  (lanes: observed status, baseline import, catalog activation, promotion MR
  bridge). **Blocked on its kill-test**, not yet run: two `serve` replicas
  swap an `https` destination compiled in the catalog into the live delivery
  registry without a restart, both deliver to the new endpoint and report the
  new digest within 5 s, and a revision whose digest the definition does not
  name is refused by `ValidateAgainst` with the observed digest unchanged.
  If it fails, the catalog stays authoring-only and activation stays a rollout.
  Step 1 of its Convergence path, per-replica observed runtime status
  (`integration_runtime_observations`, `EngineRuntime.observations`,
  `ConnectionRuntimeState.observedReplicas`), landed through Mills on
  2026-09-29 (`bl-fifhir-config-plane-observed-runtime-status-202609`).
- [x] **Public demo, portfolio links and docs coverage** — delivered
  2026-09-27 → 2026-09-29 (see the Delivered section above). Still open from
  `.loom/40`: the **Tier B hosted demo** (the full IDE on an ephemeral
  PostgreSQL with a nightly reset and a demo switch; Cody's go), the
  **fi-fhir NetworkPolicy** in platform/gitops (the namespace is still
  `open-legacy`; D-4's cluster half), and the docs sync-check token for
  flexinfer-site.
- [ ] 5.2 SMART Backend Services and Bulk Data 3.0.0; 5.3 extension and
  compatibility contract.
- [ ] Phase 6 release evidence: budget 7 on Kubernetes 1.36, all six golden
  journeys on the supported deployment profile, no open P0/P1.

## Mills posture (2026-10-27)

fi-fhir has been admitted to the Mills cross-repo lane since 2026-09-01
(platform/gitops `k3s/mills/configmap-policy.yaml`; loom-core's HUD
`SPAWN_PROJECTS`): `cross_repo.demand_projects` and `intake.gitlab.projects`
list `libs/fi-fhir`, so a GitLab issue labelled `mills-eligible` becomes a
backlog item, as does a direct `bl-fifhir-*` post or a plan slice. The
protected paths are the GraphQL operation authorization, request security,
integration authorization, ingress and workflow OAuth, the ingest auth
adapter, every `migrations/**` tree, the secrets loaders, `deploy/**`, the
Dockerfile, `.gitlab-ci.yml` and the global `**/*auth*.go` and
`**/secret*.yaml` globs (an undeclared touch routes the run to human review);
the budget overlay is 3 USD per run and 8 runs per day. Nine items have merged
through the lane since 2026-09-01, the last four on 2026-09-29 (observed
runtime status, the engine-property allowlist, the `FI_FHIR_FHIR_SERVER_URL`
alias, `ConnectionRuntimeState.revisionId`).

This update adds the merged-result merge train for the lane
(`merge_queue.lanes."libs/fi-fhir"`: `proof_mode: merge_ref`,
`train_window: 1`; [platform/gitops MR !1005](https://gitlab.flexinfer.ai/platform/gitops/-/merge_requests/1005)) and the CI arm it
needs ([MR !280](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/280)): the queue reads GitLab's merged result for
the MR, creates a `mills-mq/train-*` ref at it, mints an API pipeline there
with `MILLS_MERGE_QUEUE=1`, and lands the MR with no rebase and no write to
its source branch. Every rule in this repository needed a merge-request event,
a tag or main, so the `$MILLS_MERGE_QUEUE == "1"` arm runs the whole gating
set on that pipeline (the three manual jobs excluded); the lane must not flip
before that arm is on main. Window 1 only: a wider window needs an operator
checkout of this repository, which exists for loom-core alone. The same gitops
change lets the queue retry `test:observability-replicas` and `test:ui-e2e`
once before evicting a row as red, since both fail on runner saturation with
the code unchanged.

Deliberately not changed, each a separate decision: the HUD merge-request
watcher (`LOOM_MRWATCH_PROJECTS` names loom-core only; adding fi-fhir would
let the shepherd enqueue every green non-draft MR, including !246, !253 and
!279, without a review); the hub's `MILLS_MERGE_QUEUE_PROJECTS` (agent merges
from the hub land directly today; listing fi-fhir routes them through the
train); the soak-posture budget caps; and main-pipeline dedup of a tree the
train already proved (loom-core's `validated_tree_skip.sh`; fi-fhir's main
pipeline runs about 55 minutes per merge).

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

The eleven open issues were triaged on 2026-10-27 against the sections above;
none had been touched since 2026-07-02 except the Renovate dashboard. The Now
list is the working backlog; these are the standing placeholders.

| Issue | Title | Status on 2026-10-27 |
|---|---|---|
| #19 | Roadmap tracking — fi-fhir 2026-H2 | Open; this file is the plan store it points at. |
| #9 | M1 backend↔frontend integration parity | Largely delivered by the IDE repair and unlock programs (the `/api/auth/status` contract; the 44 unreached root fields closed). Close with a pointer to `.loom/36` and `.loom/42`, or re-scope to the legacy event fields item in Then. |
| #10 | M2 flexinfer inference path with timeout/retry contracts | Partly delivered: the Copilot runs on the backend LLM through the in-cluster LiteLLM with a credential (2026-09-30). The timeout and retry envelope is still undocumented; keep open, re-scope. |
| #11 | M3 mentatlab run orchestration | Not started; nothing has asked for it since July. Candidate to close. |
| #12 | Additional FHIR IG support (USCDI v3, Bulk Data, SMART) | Open; it is Then 5.2 and 5.3. |
| #15 | Organize test data and edge-case fixtures | Open; edilint's generator (`gen 837p`, `gen 835`, `--defect`) is the first concrete slice (see the edilint ROADMAP). |
| #16 | UI accessibility, keyboard shortcuts, bulk operations | Partly delivered (Cmd/Ctrl+K from inputs and editors, focus on tab close); the remaining E-4 review nits are in Now. |
| #17 | Terminology approval workflow for autoroute suggestions | Open; speclet `.loom/27`. |
| #18 | Expand storage provider integration tests | Open; speclet `.loom/26`. |
| #8 | Execute next-wave enhancements backlog | Placeholder superseded by this file. Candidate to close. |
| #4 | Dependency Dashboard (Renovate) | Renovate's npm updates never refresh the lockfile, so they are unmergeable by construction; the Go updates are fine. |
