# fi-fhir Roadmap

> Last Updated: 2026-11-01
> Tier: 1 (see workspace AGENTS.md "Portfolio Tiers")
> Tracking issue: https://gitlab.flexinfer.ai/libs/fi-fhir/-/issues/19
> Completion spec: `.loom/20-product-spec-integration-engine-ide-completion.md`
> Execution plan: `.loom/30-implementation-plan-integration-engine-ide-completion.md`
> Plan store: this file's Now list, fed to Mills through GitLab issues labelled `mills-eligible` (see Mills posture). The July program plan `plan-complete-fi-fhir-as-a-production-integration-engine-and-ide-341d98` has been parked since 2026-10-11 and feeds nothing; close it as superseded.

- **Latest release**: v0.2.0 (2026-11-07)

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

The last week of October was serve telemetry and the first Mills train rows
(Delivered section below). [MR !279](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/279) landed opt-in OTLP/HTTP tracing for
admission, planning and claimed delivery behind a private provider and a
closed span-attribute allowlist, after a review that added the allowlist and
a `tracingleak` negative control. Its three filed findings became GitLab
issues #22–#24, labelled `mills-eligible`, and shipped through the Mills lane
on 2026-10-29/30: the delivery trace join is asserted (!285), the default-off
path is proven in-process (!283), and the correlation-header digest is keyed
with HMAC-SHA256 under `FI_FHIR_TELEMETRY_CORRELATION_KEY` (!282). Two
unblockers rode with them: go-jose v4.1.6 for CVE-2026-116587 (!284), which
had been failing `security:trivy-image` on every merge request, and the
QuickBenchmark wall-clock floor removed from `test:unit` (!286). Tracing stays
off in every deployment, and production sets no correlation key, so its
digests are still the unkeyed `sha256:` form and `serve` warns at startup.

This update records repository state through merge `6c00720f1` on 2026-11-01
(main pipeline 36738: 67 jobs green plus the manual `test:benchmark`, 72
minutes), which landed [MR !288](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/288)'s `workflow:` never-rule for push
pipelines on `mills-mq/` refs, the repository half of the merge-train repair.
Production and the demo run `v0.1.36738`, the tag gitops pins for both. The
four rows that merged through the Mills train on 2026-10-29/30 were proven, on
the train ref, by a push pipeline that ran only `lint:helm`; the gating
evidence for each is its own head pipeline plus main, and whether the next
train ref carries the queue's own `api` pipeline is the open acceptance check
(see Now). These are internal GitLab records.

On 2026-11-03 that acceptance check passed: the first row since !288,
[MR !290](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/290)
(documentation), got no push pipeline on its train ref and the queue minted
API pipeline 37258, 64 jobs across lint, test, security, build and scan,
where the October trains ran one. Issues #22–#24 are closed. The same day
fifteen `bl-fifhir-*` rows were posted to the Mills lane from the Now list
(`.loom/43-mills-push-2026-11-execution-specs.md`), `ci/**` became a
protected path (platform/gitops !1049), and the operator half of the proof
fix became a loom-core row. Production runs `v0.1.36862`, the demo
`v0.1.36738`.
Production activation and release certification remain separate decisions;
this documentation update does not assert a new clinical-runtime deployment.

On 2026-11-05 the repository declared its climb in `.loom/climb.yaml`
(problem, summit metrics, constraints and route for the Climbs page, loom-core
`.loom/208` §5), and each Mills push bullet keeps naming its `bl-` row id,
the written-intent key `libs/STANDARDS.md` now requires.

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
  maps. Merged in `2bf7145b6`. The cluster half, a default-deny NetworkPolicy
  for the fi-fhir namespace, merged in platform/gitops !842 on 2026-09-28
  (see Then for what it still leaves open).
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

## Delivered — Serve tracing, security unblockers, first Mills train rows (2026-10-27 → 2026-10-30)

No spec; the brief was MR !279's review and the Mills lane's first rows. The
worklog entries dated 2026-10-10 and 2026-10-27 carry the tracing evidence;
the five Mills rows are `gl-19-22`, `gl-19-23`, `gl-19-24`,
`bl-fifhir-go-jose-4-1-6-cve-2026-116587-trivy-image-202610` and
`bl-fifhir-quickbenchmark-throughput-assert-flakes-on-shared-runners-202610`.

- [x] **CI proof-profile arm and security pins** ([MR !280](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/280), `74ed28da1`) —
  the `$MILLS_MERGE_QUEUE == "1"` arm on 44 rules across `.gitlab-ci.yml` and
  `ci/*.yml`, the three manual jobs excluded; Go 1.26.10 (GO-2026-6809),
  DOMPurify 3.4.17 (GHSA-cg2g-xh64-v4q7), Vite 7.3.7 and Vitest 4.x for the
  audits that failed pipeline 35308.
- [x] **Operator-bundle browser gate waits** ([MR !246](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/246) `bab8e72b3`,
  [MR !253](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/253) `ab70a5483`) — each `waitForResponse` armed at the action that
  issues the request; `previewOnSessionStream` and `expectPreviewComplete` in
  `ui/e2e/support.ts` split the 10 s stream-open budget from the 60 s run
  budget.
- [x] **Opt-in serve tracing and lineage logs** ([MR !279](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/279) `d41f2f3a0`; 42
  files) — `internal/observability/tracing.go` builds a private OTLP/HTTP
  provider only when `FI_FHIR_TRACING_ENABLED` is true (endpoint required,
  sampler 0.1 by default), never installed globally and ignoring inbound
  `traceparent`; a processor `Observer` seam with `admission` and
  `workflow_plan` phases and a claimed-delivery span joined to receipt,
  ledger and attempt ids; spans may carry only the 20 keys in
  `permittedSpanAttributes`, refusals are counted and stamped as
  `dropped_attributes`, and `test:structured-logging` fails on them; the
  caller's `X-Correlation-ID` is digested, never logged; `tracingleak` is the
  span-side negative control beside `structuredloggingleak`. Deploy defaults
  are off everywhere.
- [x] **go-jose v4.1.6** ([MR !284](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/284) `1f34209ac`, Mills P1) —
  CVE-2026-116587 had `security:trivy-image` red on every merge request
  (!282, !283 and !285's first pipelines all failed there and nowhere else).
- [x] **Default-off proof at serve level** ([MR !283](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/283) `019ac3d55`, issue
  #23) — `newServeTracing` shared by `runServe` and the proof;
  `TestServeTracingDefaultOffExportsNothingAndKeepsLineage` counts collector
  connections and requests (both 0), requires lineage keys on processor and
  delivery lines and no `trace_id`, and fails if either component never logs;
  `scripts/check-runtime-config.sh` gains `check_tracing_defaults` over
  `deploy/`, `configs/`, `docker-compose.yaml` and `.env.example`.
- [x] **Delivery trace joins admission** ([MR !285](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/285) `b2347a127`, issue #22)
  — `assertDeliveryJoinsAdmission` requires the delivery span's `receipt_id`
  and `lineage_trace_id` to equal admission's, a present
  `delivery_attempt_id`, a distinct valid trace id and no parenting under
  admission; the structured-logging serve subtest boots the delivery worker
  against an unreachable broker and joins the attempt row and a worker log
  line to the span (claim plus retry, not a successful publish); the trace
  collector refuses an empty required-names list.
- [x] **QuickBenchmark floor out of `test:unit`** ([MR !286](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/286) `16ae22a21`) —
  `TestQuickBenchmark`'s 1000 ops/s in a 10 ms window, which !282's pipeline
  failed at 247 on a shared runner, replaced by a positive finite rate, an
  elapsed-time window and at least one processed event; throughput floors
  stay in the calibrated `test:benchmark` job.
- [x] **Keyed correlation digest** ([MR !282](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/282) `f9144fef7`, issue #24) —
  `spanCorrelationID` renders `hmac-sha256:` over the first 128 bits when
  `FI_FHIR_TELEMETRY_CORRELATION_KEY` (at least 32 bytes) is set and keeps
  `sha256:` otherwise; `serve` warns at startup when it is unset; the key is
  a redacted Engine-tab property; the replica proofs run under a fixed key;
  `PRODUCTION-HARDENING.md` § Correlation covers rotation and the unset
  state. Wiring the secret into the manifests was left as a follow-up and is
  in Now.

## Now

### Mills push (2026-11-03)

Fifteen rows from this list are in the Mills lane as `bl-fifhir-*` backlog
items with declared files; `.loom/43-mills-push-2026-11-execution-specs.md`
is the program view (order, dependencies, the lane's kill-test, what stays
with a person). Items in the lane name their row id in the first line below,
so a reader (or the roadmap carder, if it is ever turned on) sees at once
which bullets are already owned. The overlay stays at 3 USD per run and 8
runs per day, so the rows pace over three to four days. Not in the lane, with
the owner: the production correlation secret (Cody), budgets 2 and 3 (runner
window), Slice 5.1c-γ (a mapper program), the legacy event fields retirement
(a session; it regenerates `generated.go`), the configuration-plane
kill-test, stream capture of real ADT feeds, E1-1, the session preview cost,
and the definition authoring follow-ups (one row once the first UI rows
have landed).

- [x] **Make the merge-train proof real: loom-core adopts only `api` proofs on
  `mills-mq/train-*` refs (`pkg/mills/mergequeue/processor_train.go`)** — closed 2026-11-04 by loom-core [MR !3350](https://gitlab.flexinfer.ai/services/loom-core/-/merge_requests/3350) (`0038b25d0`, row `bl-mills-mergequeue-train-proof-api-source-only-202611`): `TrainProofPipelineStatus` adopts only the queue's own `api` pipeline on a `mills-mq/train-*` ref and records `pipeline_source`. the repository half and the acceptance check are
  done (below). The lane flipped to `merge_ref`
  on 2026-10-27 and four rows merged through `mills-mq/train-libs-fi-fhir-*`
  refs on 2026-10-29/30, but every train pipeline (35918, 35921, 35971,
  36007) was a push pipeline with no variables that ran exactly one job,
  `lint:helm`, which runs there only because of its bare `changes:` rule
  (`.gitlab-ci.yml`); no pipeline with `MILLS_MERGE_QUEUE=1` has ever been
  minted on this project, so the `$MILLS_MERGE_QUEUE == "1"` arm from
  [MR !280](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/280) has not run once. Nothing broken landed, because each MR's own
  head pipeline and main were green, but the merged-result premise did not
  hold (!285 landed on !283 and that tree was first tested by main pipeline
  35923). The cause has two halves and so does the repair. The queue adopts
  any green push or API pipeline on the train ref (loom-core
  `pkg/mills/mergequeue/processor_train.go`, `trainProofPipeline`; the
  operator logs `minted: false`), and this repository, unlike loom-core, has
  no `workflow:` rule refusing push pipelines on `mills-mq/` refs, so the
  bare-`changes:` job supplies a green pipeline before the queue would mint
  one. Repo side: add that never-rule to `workflow:rules` in `.gitlab-ci.yml`
  (`$CI_PIPELINE_SOURCE == "push" && $CI_COMMIT_BRANCH =~ /^mills-mq\//`),
  a protected-path change that goes through a human merge request, after
  which the queue has to mint the API pipeline that carries the variable and
  the !280 arm runs for the first time. Operator side: the queue adopts only
  its own `api` proof on a train ref (loom-core). The repository half merged
  as [MR !288](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/288) (`6c00720f1`, 2026-11-01). Acceptance observed
  2026-11-03 on [MR !290](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/290): train ref
  `mills-mq/train-libs-fi-fhir-290-1cd5e1c6` carried no push pipeline and one
  `api`-source pipeline, 37258, with 64 jobs (lint 11, test 42, security 6,
  build 3, scan 2), minted 30 seconds after enqueue; edilint's first train
  row (!38, pipeline 37257) behaved the same. A train's green here is now
  the gating set. Remaining: the loom-core half (row above), so that a
  repository without the never-rule is also safe.
- [ ] **Wire `FI_FHIR_TELEMETRY_CORRELATION_KEY` into production (gitops SOPS
  secret + `secretKeyRef`, `deploy/helm/fi-fhir`, `deploy/kubernetes`)** —
  the code half (`_FILE` variant and the key-format check in
  `cmd/fi-fhir/serve_tracing.go`) merged 2026-11-04 as [MR !295](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/295) (`d1e23bebf`, row
  `bl-fifhir-correlation-key-file-variant-and-format-202611`, train pipeline 37572):
  `FI_FHIR_TELEMETRY_CORRELATION_KEY_FILE` through the shared secret helper and a
  format check that requires the hex or base64 encoding of at least 32 random
  bytes (a raw passphrase is refused at startup); the secret is Cody's.
  [MR !282](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/282) keys the correlation-header digest with HMAC-SHA256 when the key
  is set and keeps the unkeyed `sha256:` form otherwise, and no manifest sets
  it: not `deploy/helm/fi-fhir`, not `deploy/kubernetes`, not gitops
  `k3s/fi-fhir/fi-fhir-api.yaml` or the demo. Production logged the startup
  WARN ("unset: correlation_id is an unkeyed sha256 digest") at
  2026-10-30T17:36Z, so its digests are still the form the issue set out to
  remove. Needs a SOPS secret and `secretKeyRef` in gitops (Cody; the
  auto-mode classifier refuses secret-store writes) and the same wiring in
  `deploy/helm/fi-fhir` and `deploy/kubernetes`; add a `_FILE` variant in
  `cmd/fi-fhir/serve_tracing.go` to match every other serve secret, and
  tighten the loader's `len >= 32` check, which accepts a 32-character
  passphrase as if it were 32 random bytes.
- [x] **`make tracing-negative-control` and `structured-logging-negative-control`
  assert the sentinel failure (`Makefile`, `ci/test-structured-logging.yml`)** — closed 2026-11-04 by [MR !292](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/292) (`1b812590c`, row `bl-fifhir-tracing-negative-control-asserts-sentinel-202611`, train pipeline 37525 after two infrastructure evictions).
  `Makefile` inverts the tagged run's exit status and discards its output, on
  the premise that every failure under `tracingleak` is the leak being
  caught; since [MR !285](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/285) the same subtest also fails when the delivery span
  never arrives, the attempt row is missing or no delivery log line joins, so
  on a saturated runner the control reports OK without the span scan having
  measured anything. Make the control assert the specific failure (the
  sentinel in captured output), as `structuredloggingleak` does. Owns
  `Makefile` and `ci/test-structured-logging.yml`.
- [ ] **Run `scripts/check-runtime-config.sh` in CI as `ci/lint-runtime-config.yml`
  with its `ci/job-inventory.txt` row** — in the lane as
  `bl-fifhir-check-runtime-config-ci-job-202611`. [MR !283](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/283) put the
  "tracing defaults stay opt-in" scan (`check_tracing_defaults`) into that
  script, and nothing in `.gitlab-ci.yml` or `ci/*.yml` calls it; only
  `make check-runtime-config` does. A manifest edit that flips
  `FI_FHIR_TRACING_ENABLED` to true under `deploy/` passes every pipeline
  today. Add it to `test:smoke` or as its own one-file job under `ci/` with a
  `ci/job-inventory.txt` line.
- [x] **Document the `FI_FHIR_TRACING_*` and `FI_FHIR_DELIVERY_*` keys in
  `cmd/fi-fhir/main.go` so `serveProperties()` carries them to the Engine tab** — closed 2026-11-04 by [MR !294](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/294) (`e52d39482`, row `bl-fifhir-engine-tab-tracing-delivery-keys-202611`, train pipeline 37464). `serve --help` documents
  only `FI_FHIR_TELEMETRY_CORRELATION_KEY` of the new keys, and
  `serveProperties()` is by contract exactly the keys `serve --help`
  documents (`cmd/fi-fhir/engine_runtime.go`,
  `TestServePropertiesAreExactlyTheDocumentedKeys`), so
  `FI_FHIR_TRACING_{ENABLED,ENDPOINT,SAMPLER}` and the delivery worker's
  `FI_FHIR_DELIVERY_{LEASE_DURATION,POLL_INTERVAL,PUBLISH_TIMEOUT}` never
  reach the Engine allowlist, and a replica's tracing state cannot be told
  apart in the IDE. Re-opens the ticked "engine properties" item below for
  these keys. Owns `cmd/fi-fhir/main.go` (help text),
  `cmd/fi-fhir/engine_runtime.go` and `docs/operations/CONNECTION-CATALOG.md`.
- [x] **Telemetry docs and changelog drift (`docs/operations/README.md`,
  `docs/user-guide/cli-reference.md`, `docs/operations/SECURITY.md`)** — closed 2026-11-04 by [MR !301](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/301) (`a5aaa5141`, row `bl-fifhir-telemetry-docs-drift-202611`, train pipeline 37559). `docs/operations/README.md`
  § Logging still says the processor logs a SHA-256 hash of the caller's
  correlation id (an HMAC when the key is set);
  `docs/user-guide/cli-reference.md` lacks the key;
  `docs/operations/SECURITY.md` never mentions spans, tracing or correlation;
  [MR !279](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/279) left no `.added` changelog fragment for the feature itself, and
  the `.fixed` fragment it did leave still says SHA-256. Owns those four docs
  and `changelog.d/`.
- [x] **Close issues #22, #23 and #24** — delivered by [MR !285](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/285), [MR !283](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/283)
  and [MR !282](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/282) on 2026-10-29/30; #22 was already closed, and #23 and #24
  were closed on 2026-11-03 with notes naming the merge request and the Mills
  row that delivered each, and why they had stayed open (no `Closes` line).
  #23's note says the default-off proof is an in-process composition test
  (`cmd/fi-fhir/serve_tracing_test.go`), not a booted `serve`, and that the
  replica proof asserts nothing about `trace_id` or collector traffic under
  defaults; #24's says production still sets no correlation key (the Now item
  above). No `mills-eligible` issue is open.
- [x] **Land or close the three open merge requests** — all three merged on
  2026-10-27: [MR !246](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/246) as `bab8e72b3`, [MR !253](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/253) as `ab70a5483` and
  [MR !279](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/279) as `d41f2f3a0`, each on a green pipeline (Delivered section
  above). No merge request is open.
- [x] **edilint pins** — closed 2026-11-01 by [MR !289](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/289): `lint:edi`
  (`ci/lint-edi.yml`, `Makefile` `EDILINT_VERSION`) moved from edilint v0.3.0
  and `lint:edi-fixtures` (`ci/lint-edi-fixtures.yml`,
  `EDILINT_FIXTURES_VERSION`) from v0.1.0 to v0.5.0, tagged the same day
  (GitHub release `crb2nu/edilint` v0.5.0, `ghcr.io/crb2nu/edilint:0.5.0`).
  Both job command lines, run locally with the v0.5.0 binary, report the
  seven-file `testdata/edi` corpus clean (stock rules: `7 files checked, no
  findings`; committed `.edilint.yml`: JUnit `tests="7" failures="0"`), so the
  bump changed no finding. Growing the corpus with `edilint gen 837p` and
  `gen 835 --defect` stays with issue #15.
- [x] **Re-probe `/api/auth/status` after credential entry and on the
  legacy-execution refusal (`ui/src/lib/graphql/accessCapabilities.ts`)** — closed 2026-11-04 by [MR !293](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/293) (`f9e3e1b9f`, row `bl-fifhir-stale-capabilities-reprobe-202611`, train pipeline 37418; the lane's kill-test, no hand on the row). a tab reads `/api/auth/status`
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
- [ ] **Terminology-store availability as a capability
  (`internal/api/graphql/capabilities.go`, `TerminologyPage.svelte`)** — in
  the lane as `bl-fifhir-terminology-store-capability-202611` (first half
  only). terminology without
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
- [x] **`operator-bundle` e2e timing budget** — closed by [MR !246](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/246) (each
  wait armed at the action that issues the request) and [MR !253](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/253)
  (`previewOnSessionStream` and `expectPreviewComplete` in `ui/e2e/support.ts`
  split the 10 s stream-open budget from the 60 s run budget), both merged
  2026-10-27. Residue: `ui/e2e/operator-bundle.spec.ts` check 3 still carries
  its own inline stream wait instead of the helper (in the lane as
  `bl-fifhir-operator-bundle-e2e-stream-helper-202611`, merged 2026-11-04 as [MR !302](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/302) `8e999185a`), and the gitops
  `test:ui-e2e` one-retry rule for Mills rows (2026-10-27) was justified by
  these two MRs being open, so it can be reconsidered.
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
- [ ] **Connection rows select `observedReplicas` and `totalReplicas`
  (`ui/src/lib/features/connections/ConnectionDetails.svelte`)** — in the
  lane as `bl-fifhir-connection-rows-replica-counts-202611`. the connection
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
- [x] **Workflows: Run event stops calling the refused mutation
  (`ui/src/lib/features/workflows/components/WorkflowList.svelte`)** — closed 2026-11-04 by [MR !297](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/297) (`2b57aa6bf`, row `bl-fifhir-workflows-run-event-refused-mutation-202611`, train pipeline 37547). Inventory's **Run
  event** still calls `triggerWorkflow` (`WorkflowList.svelte`, re-checked
  2026-10-27), which shipped builds refuse
  (`legacyUnsafeExecution`), so `workflowRuns` (and Open trace) is normally
  empty on `serve`. Remove it or route it through a session simulation.
  `markWorkflowBuilderOpened` now has only test callers; retire or rename it.
  `EventLineagePanel` has no component test, and E5-1 does not assert the
  "Unsaved changes" state after an edit.
- [ ] **Shell accessibility nits (E-4 review 7–11;
  `ui/src/lib/ui/primitives/Dialog.svelte`, `Tabs.svelte`)** — in the lane as
  `bl-fifhir-shell-a11y-nits-e4-review-202611`. the `Dialog` focus
  fallback when nothing inside is focusable; Escape handled before focus
  enters the dialog; tab close buttons sit inside the `tablist`; the Copilot
  control dims without saying why. (The fifth nit, Cmd/Ctrl+K ignored while
  focus is in an input or editor, closed 2026-10-03 in `1bc3e370d`.)
- [x] **Journey evidence reads every session** — closed 2026-10-03
  (`3fab67340`): journey evidence and the session lists use bounded metadata
  queries; `journeyState.ts` no longer calls `fetchRecentSessions` with
  `MAX_SAFE_INTEGER`.
- [ ] **Operator deep-link polish (E-0 review N1, N6;
  `ui/src/lib/features/operator/OperatorPage.svelte`)** — in the lane as
  `bl-fifhir-operator-deep-link-polish-202611`. a deep link is read
  once on mount, before the capability pre-flight has necessarily settled;
  the attempt inspector links the resubmit parent but not its children.
- [x] **`TestExecAction_RunsAllowedCommand` loses its wall-clock floor
  (`internal/workflow/exec_action_test.go`)** — closed 2026-11-04 by [MR !300](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/300) (`ecc842c35`, row `bl-fifhir-exec-action-test-flake-202611`, train pipeline 37566; reviewed under the spec-touching hold).
  `internal/workflow/exec_action_test.go` runs `/bin/sh` with a 2 s exec
  timeout under a 5 s context; it failed once under full-suite `go test ./...`
  in four IDE-unlock lanes' local runs and again in the pre-push hook on
  2026-10-27 (MRs !280 and !281), passing alone each time. Its sibling
  `TestQuickBenchmark` got the pattern in [MR !286](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/286): keep the behavioural
  assertion and leave wall-clock floors to the calibrated `test:benchmark`
  job. Owns that test file only.
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
- [x] **Trace a successful publish through a webhook sink in the
  structured-logging proof** — closed 2026-11-04 by [MR !296](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/296) (`4f9c71e51`, row `bl-fifhir-trace-successful-publish-webhook-sink-202611`, train pipeline 37577; reviewed under the spec-touching hold). the structured-logging delivery join
  runs against an unreachable broker, so only the claim-and-retry path is
  exported in CI (`PRODUCTION-HARDENING.md` § Correlation); no proof exports
  a successful `integration.delivery.dispatch`. Needs a reachable sink in
  `ci/test-structured-logging.yml` (the webhook destination is the cheapest)
  and one more required span name in the collector.
- [ ] **Session preview round-trip cost** — a preview run commits about 25
  store round trips (one `UpdateRun` per stage start and end, one
  `AppendStreamEvent` per publish; !253's verification notes), which is why
  the e2e budgets track host load. Batch the per-stage `UpdateRun` writes in
  `internal/integration/session`; the e2e budgets can then tighten.

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
  PostgreSQL with a nightly reset and a demo switch; Cody's go), the rest of
  the **fi-fhir NetworkPolicy** (platform/gitops !842 merged the default-deny
  policy for the namespace on 2026-09-28, labelled
  `security.flexinfer.ai/network-policy-mode: default-deny`; postgres,
  temporal and the other non-API workloads stay under
  `legacy-workloads-unrestricted`, which the manifest itself calls a
  follow-up), and the docs sync-check token for flexinfer-site.
- [ ] 5.2 SMART Backend Services and Bulk Data 3.0.0; 5.3 extension and
  compatibility contract.
- [ ] Phase 6 release evidence: budget 7 on Kubernetes 1.36, all six golden
  journeys on the supported deployment profile, no open P0/P1.

## Mills posture (2026-10-31)

**2026-11-03 update.** The never-rule from !288 was proven on !290 (train
ref `mills-mq/train-libs-fi-fhir-290-1cd5e1c6`, no push pipeline, API
pipeline 37258 with 64 jobs); protected paths are now sixteen with `ci/**`
(platform/gitops !1049, checksum re-synced); issues #22–#24 are closed;
fifteen `bl-fifhir-*` rows are in the lane (`.loom/43`), two implementing
within a minute of posting. Intake for this push is the direct row with
declared files, not the issue path. One trap for desktop-app sessions: the
GitLab MCP merge tool there has no queue URL in its environment and merges
directly, so a row is enqueued with
`POST $MILLS_MERGE_QUEUE_URL/api/mills/merge-queue/enqueue`. The rest of
this section is the 2026-10-31 reading and stays as the record.

fi-fhir has been in the Mills cross-repo lane since 2026-09-01 (platform/gitops
`k3s/mills/configmap-policy.yaml`; loom-core's HUD `SPAWN_PROJECTS`). Intake
is an open GitLab issue on this project labelled `mills-eligible` (polled every
five minutes; item `gl-19-<iid>`, title and description copied once as the
spec, priority from a `priority:P0`–`P3` label, default P2, no file list), a
direct `bl-fifhir-*` post with slices, or an approved plan routed to Mills
(`dispatch_target: mills`). An imported issue needs its files declared before
its first run, through the HUD or `mills_backlog_amend_scope`, or the
implementer stops on undeclared scope and escalates without retry (gl-19-23
and gl-19-24 did, 1.21 USD between them). Add `mills-eligible` last, when the
text is final, and have the merge request say `Closes #n`: nothing closes the
issue today. Protected paths are the GraphQL operation authorization, request
security, integration authorization, ingress and workflow OAuth, the ingest
auth adapter, every `migrations/**` tree, the secrets loaders, `deploy/**`,
the Dockerfile, `.gitlab-ci.yml` and the global `**/*auth*.go` and
`**/secret*.yaml` globs; `ci/**` is not, though its 26 included files define
most gating jobs, and should be. The budget overlay is 3 USD per run and 8
runs per day. Fourteen items have merged since 2026-09-01; five since the
train flip, on 2026-10-29 and 10-30, in eight runs for 12.77 USD with no cap
reached (one run cost 3.71 USD, because the per-run cap is checked only at
admission): the go-jose v4.1.6 bump for CVE-2026-116587 ([MR !284](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/284)), the
delivery-trace join proof (gl-19-22, [MR !285](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/285)), the tracing default-off
proof (gl-19-23, [MR !283](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/283)), the keyed correlation digest (gl-19-24,
[MR !282](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/282)) and the QuickBenchmark throughput floor ([MR !286](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/286)).

The lane has proved merge requests on the merged-result train since
[platform/gitops MR !1005](https://gitlab.flexinfer.ai/platform/gitops/-/merge_requests/1005)
(`proof_mode: merge_ref`, `train_window: 1`). Four rows rode it (!282, !283,
!285, !286); none wrote to its source branch and none tripped
`train_tree_mismatch`, but none was proven by the gating set. The queue adopts
any green push or API pipeline on the `mills-mq/train-*` ref, and a new ref's
push pipeline here runs only `lint:helm` (its last rule is a bare `changes:`),
so each row merged two to three minutes after its train was built, on
pipelines 35918, 35921, 35971 and 36007; the `$MILLS_MERGE_QUEUE == "1"` arm
from [MR !280](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/280) has never run. !283's tree equalled its green MR pipeline;
!282, !285 and !286 landed trees first proven by main's pipeline after the
merge (49 to 74 minutes; all gating jobs green, 35973 failed only
`mirror:github` on a DNS timeout). !284 merged on its own MR pipeline through
the base-equals-tip path. The `workflow:` never-rule landed as !288 and was
proven on 2026-11-03 (update above); until the queue itself requires an
`api` proof (the loom-core row), a repository without the rule is still
exposed.

No row landed without a hand. gl-19-23 and gl-19-24 first stopped on
undeclared scope (their files were declared on 2026-10-28); all three `gl`
rows then escalated `code` at `ci_watch` because `security:trivy-image`
failed on the go-jose advisory already in the base, a misclassification
cleared by !284 and a rebase of the three branches; gl-19-22's first run
escalated `infra` on a devbox golangci-lint timeout; !286 was held for review
as spec-touching and released by a reviewer agent; !282 was evicted once on a
CHANGELOG-fragment conflict that conflict autofix does not resolve for
cross-repo lanes and was rebased by hand. Every row entered the queue from a
workstation `merge_merge_request` (which routes every project through the
queue) or the review release. The flake retry for
`test:observability-replicas` and `test:ui-e2e` never fired; it could not,
since neither job ran in any train proof. Issues #22–#24 were closed on
2026-11-03.

Not changed, each a separate decision recorded on 2026-10-31
(`.loom/decisions/`): the HUD merge-request watcher (`LOOM_MRWATCH_PROJECTS`
names loom-core only; its shepherd is on and would enqueue every green
non-draft MR here, today onto a hollow proof: no); the hub's
`MILLS_MERGE_QUEUE_PROJECTS` (loom-core only; workstations already route this
project: yes, after the proof fix); main-pipeline dedup
(`validated_tree_skip.sh` trusts any green train pipeline and would have
skipped main's only full proof on three of the four merges: no until the
train mints real API proofs and the script checks the pipeline source);
window 1 (a wider window needs an operator checkout of this repository and a
loom-core change: no); the budget overlay (never the constraint: keep); and
roadmap carding (`intake.carding.target_projects` does not list this
repository; if it did, the carder would read `ROADMAP.md` on main every 15
minutes and card only the first line of each unchecked bullet under `## Now`,
so the first line of a Now item should carry the action and its paths before
carding is turned on). The 2026-10-30 draft
`plan-assert-the-unsaved-changes-state-in-the-e5-1-end-to-end-jour-f71ed6`
was spun from an operator-written brief, is inert, and invents a new spec
file where E5-1 lives in `ui/e2e/operator-bundle.workflows.spec.ts`; respin
it with that path or drop it. The program plan this file's header used to
name has been parked since 2026-10-11 and feeds nothing.

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

Eleven issues are open on 2026-11-03: the eleven triaged on 2026-10-27
against the sections above (none touched since 2026-07-02 except the Renovate
dashboard); the three `mills-eligible` telemetry follow-ups filed from the
!279 review on 2026-10-27, which Mills delivered, were closed on 2026-11-03. The Now
list is the working backlog; these are the standing placeholders.

| Issue | Title | Status on 2026-10-31 |
|---|---|---|
| #22 | Structured-logging proof: the asynchronous delivery trace joins admission | Delivered by [MR !285](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/285) on 2026-10-29; closed. |
| #23 | Prove the tracing default-off path at serve level | Delivered by [MR !283](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/283) on 2026-10-29 as an in-process composition proof; closed 2026-11-03 with that note. |
| #24 | Key the correlation-header digest with HMAC under a deployment secret | Delivered by [MR !282](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/282) on 2026-10-30; closed 2026-11-03; the production secret is not wired (Now). |
| #19 | Roadmap tracking — fi-fhir 2026-H2 | Open; this file is the plan store it points at. |
| #9 | M1 backend↔frontend integration parity | Largely delivered by the IDE repair and unlock programs (the `/api/auth/status` contract; the 44 unreached root fields closed). Close with a pointer to `.loom/36` and `.loom/42`, or re-scope to the legacy event fields item in Then. |
| #10 | M2 flexinfer inference path with timeout/retry contracts | Partly delivered: the Copilot runs on the backend LLM through the in-cluster LiteLLM with a credential (2026-09-30). The timeout and retry envelope is still undocumented; keep open, re-scope. |
| #11 | M3 mentatlab run orchestration | Not started; nothing has asked for it since July. Candidate to close. |
| #12 | Additional FHIR IG support (USCDI v3, Bulk Data, SMART) | Open; it is Then 5.2 and 5.3. |
| #15 | Organize test data and edge-case fixtures | Open; edilint's generator (`gen 837p`, `gen 835`, `--defect`) is the first concrete slice, in the lane as `bl-fifhir-edi-fixtures-from-edilint-gen-202611` (2026-11-03), merged 2026-11-04 as [MR !299](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/299) (`252777436`: generated 837P/835 corpus, four envelope defects, table-driven parser tests); the issue stays open for the remaining formats. |
| #16 | UI accessibility, keyboard shortcuts, bulk operations | Partly delivered (Cmd/Ctrl+K from inputs and editors, focus on tab close); the remaining E-4 review nits are in Now. |
| #17 | Terminology approval workflow for autoroute suggestions | Open; speclet `.loom/27`. |
| #18 | Expand storage provider integration tests | Open; speclet `.loom/26`. |
| #8 | Execute next-wave enhancements backlog | Placeholder superseded by this file. Candidate to close. |
| #4 | Dependency Dashboard (Renovate) | Renovate's npm updates never refresh the lockfile, so they are unmergeable by construction; the Go updates are fine. |
