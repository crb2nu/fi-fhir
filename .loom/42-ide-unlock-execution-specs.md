# IDE unlock: execution specs (lanes E-0..E-6)

> Brief (Cody, 2026-09-29): "continue building on this work and further
> improving our UI/UX surfaces to unlock the power of our backend."
> Coordinator = session 391c0868; Opus lanes implement; the coordinator
> reviews and arms merges. Survey evidence: `<scratchpad>/research/survey-*.md`
> (four read-only surveys, 2026-09-29) and Appendix A.

## What changed since `.loom/38` and `.loom/40`

Production is no longer an engine with nothing flowing through it. The demo
hospital environment (platform/gitops `k3s/fi-fhir/demo/`, plan
`.loom/30-implementation-plan-fi-fhir-demo-environment-2026-09-28.md`) put an
SFTP drop, an S3 bucket, a Kafka broker and a HAPI FHIR "hospital" beside the
API; `fi-fhir lifecycle seed` (MR !258) wrote the first real definition
`sftp-test-demo/v1` into the lifecycle catalog, and on 2026-09-29 04:11Z two
synthetic admits were ingested, projected and delivered to the hospital. Read
from the LAN today: one `deployed`/`healthy` deployment, two `accepted`
receipts, two `succeeded` `send-fhir` attempts to `fhir-primary`. The LLM is
configured (`llm.configured: true`; `FI_FHIR_LLM_ENABLED=true` with a
LiteLLM key), so Copilot is no longer blocked on infrastructure.

The IDE shows a fraction of this. The surveys found:

- **145 GraphQL root fields; 44 never reach the IDE.** The valuable ones:
  deployment lifecycle history (`operatorDeploymentEvents`), cross-receipt
  attempt search (`operatorDeliveryAttempts`), paged attempt audit
  (`operatorAttemptAudit`), session detail/history/archive/export
  (`integrationSession`, `sessionRuns`, `sessionDiagnostics`,
  `archiveIntegrationSession`, `exportIntegrationBundle`),
  `acceptDiagnosticFix` (the UI shows fix suggestions it cannot accept),
  workflow definition rename/archive. Three of the four operator wrappers
  already exist in `operatorApi.ts` with no consumer.
- **The lifecycle catalog is a full state machine the IDE can only drive from
  `published` onward.** Nothing in `serve` creates a draft, records a
  validation, approves or publishes directly; the seed CLI does it for batch
  sources only. Deploy and Resume dead-end once validation evidence ages past
  the seed's 300 s: the errors are unmapped and the Validation badge ignores
  `validationExpiresAt` it already fetches. The definition body, validation
  record and release record have no GraphQL projection.
- **`/events` is permanently empty on `serve`.** Browse reads the legacy
  `graphql_events` table whose only writer is gated off; Timeline and
  Statistics read an in-memory projection service `serve` never feeds; Live
  Stream subscribes to a broadcast that never fires. The durable truth
  (`integration_canonical_events`, receipts, attempts, destination ledger) is
  read only inside the per-receipt trace, and the trace hides fields it
  fetches (resubmit parent/child chain, lease, completion, ledger detail).
- **Sessions are write-only.** Home's recent-session rows cannot be opened;
  no archive, no export, no run history, no per-run stream.
- **The shell is cosmetic where it should be honest.** Journey stages show
  "complete" by route order, not data; two command palettes; no deep links
  anywhere (no `searchParams` in any feature); dirty state never set; split
  pane is a placeholder; all seven `.loom/37` polish follow-ups are open;
  per-replica observations shipped in !261 are not selected by any fragment.

## Design

One program, seven lanes, one principle carried over from `.loom/37`/`.loom/38`:
**every surface says what it knows, what it cannot know on this deployment,
and why.** New surfaces read durable data through existing roles; new
writes ride `integration.deployment.operator`; nothing simulates.

### Shared contracts (all lanes)

- **Deep links.** Routes accept search params and select the object on
  mount: `/operator?receipt=<id>`, `/operator?attempt=<id>`,
  `/operator?definition=<id>&revision=<id>` (Deployments tab),
  `/connections?connection=<id>`, `/connections?definition=<id>&revision=<id>`
  (Definitions tab), `/events?receipt=<id>`, `/hl7?session=<id>`. A link
  target that is absent, forbidden or not configured renders that surface's
  existing honest state, never a blank. Each lane implements reading for its
  own routes and emits links to the others.
- **Capabilities.** `/api/auth/status` gains `definitionAuthoring` (E-1) and
  `phiExport` (E-3). A page that needs a capability pre-flights it before
  querying, in the Connections precedence (not configured → missing role →
  read only), with a `data-testid="<page>-preflight"` and `data-reason`.
- **Copy register** is `.loom/37`'s: no "simulated", "sample data", "coming
  soon"; disabled controls say why in a sentence.
- **Evidence.** Each lane adds functional e2e checks to the
  `operator-bundle` project (prefix the check id with the lane, `E0-1`), one
  `check-report.mjs` line per lane, and refreshes only the `visual`
  screenshots its surfaces change. Screenshots are review artifacts; the
  gate asserts `data-testid`s and copy.
- **Fixtures.** E-0 gives the operator-bundle stack real durable data before
  the checks run (below). Every later lane may rely on it after E-0 merges.

### E-0 — Operator depth (`feat/unlock-0-operator-depth`)

The Operator page becomes the place where an operator can answer "what
happened to this message, and what did we do about it" without leaving the
page, over data the engine already keeps.

Backend (small):
- `operator/service.go:462-476`: map `ErrConnectionValidationRequired` and
  `ErrActiveDeployment` to typed operator errors; `operatorErrors.ts` gets
  entries that say what to do ("validation evidence has expired; validate
  the definition again from Connections › Definitions").
- `OperatorDeployment.validationCurrent: Boolean!` computed server-side
  from `validationPassed` and `validationExpiresAt` against now.
- Nothing else in the schema; every read below already exists.

UI (`ui/src/lib/features/operator/**` only, plus the Home IntegrationsPanel):
- **Deployments › History**: `operatorDeploymentEvents` per row (action,
  from→to, actor, reason, release id, time), reachable from the row and by
  deep link. Validation badge honours `validationCurrent` and shows expiry.
  `releaseId` rendered.
- **Delivery › Attempts**: `operatorDeliveryAttempts` with the filters the
  store supports (status, destination, receipt, route, from/to), paged,
  each row linking to its trace and to the attempt inspect.
- **Attempt audit**: `operatorAttemptAudit` paged inside the inspect view;
  render the audit `detail` the trace already fetches.
- **Trace**: render the fetched-but-hidden attempt fields (parent/child
  resubmit chain as a list, `scheduledAt`/`completedAt`, outbox topic and
  lease with expiry); select and render the full destination-ledger row
  (revision, `digestVerified`, `failureCode`, `httpStatusClass`,
  `servedCertificateSubjectAdvisory`, `completedAt`); show canonical-event
  tombstones (`purged_at`) when present (selecting them is a one-line change
  in `operator/postgres.go:152` plus the schema field).
- **Messages**: expose the receipt `from`/`to` window the store supports.
- **Pre-flight `controlPlane`** on Operator and Home IntegrationsPanel
  (connections already does), so "not configured" is said before any query.
- **Auto-refresh** toggle (off by default, 15 s) for Messages and Delivery.
- **Fleet observations**: the Engine tab and Home Health select
  `engineRuntime.observations` and `observedReplicas/totalReplicas` (types
  already generated) and show per-replica mounted digests and staleness.
- Deep links in and out (`?receipt=`, `?attempt=`, `?definition=&revision=`;
  links to `/connections?connection=` from a trace's source/destination and
  to `/events?receipt=`).

Fixture (owned here, reused by E-2): `ui/e2e/` gains a seed step for the
operator-bundle stack so it starts with **at least one `deployed` definition
in the catalog and at least two `accepted` receipts with queued delivery
attempts, produced by the real admission path**. Recommended: the stack
enables the durable HTTP ingress (`/v1/hl7v2`, bearer credential, definition
from the stack's static registry) and the seed posts two synthetic ADT^A01
messages from `testdata/`; `fi-fhir lifecycle seed --validate skip` (reason
≥ 16 chars) writes a batch definition through `deployed` for the Deployments
tab. No Kafka in the stack: attempts stay `queued`; deliveries are not
asserted. Document the fixture in `ui/e2e/README.md`.

Acceptance: E0 checks cover history, attempts list with a filter, audit
paging, the resubmit chain rendering after a `resubmitMessage`, the
`controlPlane` preflight on the missing-operator-role and preview-only
stacks (unchanged copy), and every deep link form above; `go test ./...`
and the operator integration tests green; CHANGELOG fragment.

### E-1 — Definition editor and lifecycle authoring (`feat/unlock-1-definition-editor`)

`.loom/38` Decision 5, made honest the way the seed CLI is honest.

Backend:
- GraphQL (role `integration.operator` + `integration.deployment.operator`
  for writes, `integration.operator` for reads; entries in
  `operation_authorization_roles.go`):
  - `integrationDefinitions(includeRetired)` and
    `integrationDefinition(definitionId, revisionId)`: the stored definition
    projected (source ref + sourceId, profile ref, workflow ref, destination
    refs + class, secret bindings as references, policy, deployment policy,
    digest, created audit), its snapshot, its current validation record
    (code list, checked at, expires at, source ref) and release record
    (release digest, approval event) when present.
  - `integrationRegistryArtifacts`: the profile and workflow refs the
    runtime resolver can actually resolve, per static-registry integration
    (`{integrationId, profile{artifactId,revisionId,digest},
    workflow{…}, sourceId, format}`), proven with the same
    `processor.RevisionResolver` `serve` uses. This is the honest source of
    refs until resolution moves onto the catalog (`.loom/39`/`41`).
  - `ConnectionRevision.sourceId` and `ConnectionRevision.destinationClass`
    projected from the revision (today only inside `revisionJson`).
  - `validateIntegrationDefinitionDraft(input)`: server-side pre-flight
    without writing: `ValidateForDeployment`, resolver proof of profile and
    workflow refs, planner rule (every non-log workflow action's destination
    is one of the definition's destinations), every secret binding named by
    the chosen source and destination revisions present. Returns problems in
    the Connections `ConnectionProblem` idiom.
  - `createIntegrationDefinitionDraft(input, reason)` → `CreateDraft`.
  - `validateIntegrationDefinition(definitionId, revisionId, expectedVersion,
    mode: REAL | STATIC | SKIP, reason)` → `ValidateConnection`. `serve`
    builds the catalog with a `ConnectionValidatorFunc`: for batch sources
    the CLI's validator moved into a shared package; for `mllp`/`http`
    sources a static check (revision digest is mounted on this replica per
    the observations ledger; bindings resolvable) recorded with the new code
    `VALIDATION_STATIC`; `SKIP` mirrors the CLI (reason ≥ 16 chars, code
    `VALIDATION_SKIPPED`). Env `FI_FHIR_LIFECYCLE_VALIDATION_MAX_AGE`
    (default 300 s) documented in `serve --help` and the `engineRuntime`
    allowlist.
  - `approveIntegrationDefinition`, `publishIntegrationDefinition`
    (expectedVersion + reason) → `Approve`, `Publish`. Deploy stays on
    Operator (E-0's page) and is linked, not duplicated.
  - Capability `definitionAuthoring` = control plane + catalog + resolver
    present; `missingRoles.definitionAuthoring`.
- The seed CLI is refactored to call the same service functions
  (`internal/integration/lifecycle/authoring/…`), so CLI and API cannot drift.

UI: a **Definitions** tab on `/connections` (Sources · Destinations ·
Definitions · Engine; no new route, so the eight registration points stay
untouched): table (definition, revision, state, health, validation, release,
referenced source/destinations) → detail with Overview (refs with digests,
bindings, policies, digest), Validation (records, codes, expiry), Release,
History (`operatorDeploymentEvents`, shared component from E-0 if merged,
else a local one), and **New definition**: pick a compiled source revision
from the catalog, a profile+workflow pair from `integrationRegistryArtifacts`
(shown with digests and the sentence that says where they come from),
compiled destination revisions, then bindings (auto-derived from the chosen
revisions' `secretBindings`, each shown as a reference), classification/raw
retention, deployment policy → **Check** (`validateIntegrationDefinitionDraft`,
problems inline) → **Create draft** (reason) → **Validate** (mode picker
with the honesty text per kind) → **Approve** → **Publish** → link to
Operator › Deployments to deploy. Every write goes through the reason
dialog and carries `expectedVersion`; version conflicts say "reload, then
re-decide" as Operator does. Read-only when `connectionsWrite` is false.

Acceptance: the kill-test below passes and is checked in; e2e E1 checks
create → check → draft → validate(STATIC) → approve → publish a definition
from the stack's MLLP source, and the Operator Deployments tab then offers
Deploy for it (E-0 fixture not required); `docs/user-guide/connections.md`
gains a "Definitions" section and `docs/operations/CONNECTION-CATALOG.md`
the authoring service and env; CHANGELOG fragment.

### E-2 — Verification over durable data (`feat/unlock-2-verification`, stacks on E-0)

Journey stage 5 stops pointing at views that can never fill.

Backend (role `integration.operator`):
- `operatorCanonicalEvents(filter{eventType, definitionId, receiptId,
  sourceMessageId, correlationId, from, to, includePurged}, page)`: rows from
  `integration_canonical_events` joined to receipts (event id, type,
  MSH-10, correlation, receipt id and status, definition ref, recorded at,
  purge marks), never payload values (`operator/payload.go:25` stays the
  rule; field paths and kinds only, as the trace does).
- `operatorAdmissionStatistics(window: {from,to}, bucket: HOUR|DAY)`:
  counts by event type, by definition, by receipt status, delivery attempts
  by status and destination, and a time series per bucket, all from
  columns, all cheap (add the two indexes the queries need in a processor
  migration; `AGENTS.md` § Migration authoring).
- Keep the legacy `events`/`patientTimeline`/`eventStatistics`/`eventStream`
  fields in the schema (ROADMAP marks them legacy); the IDE stops calling
  them.

UI: `/events` re-implemented as `features/verification/`: **Admissions**
(canonical events browse with the filters above, each row → its receipt
trace on Operator by deep link), **Statistics** (the counts and series,
honest window picker, "delivered" ratio from attempts), **Retention**
(purged/tombstoned counts and the `retentionPurge` flag from
`engineRuntime`). The page pre-flights `operatorRead` + `controlPlane` in
the Connections precedence. The Live tab is removed: the page says in one
sentence that admissions are not streamed and where the per-session run
stream is. No patient timeline: the page says why (payload values are never
read back by design) in its empty state, once.

Acceptance: E2 checks over E-0's fixture (browse shows the two admissions,
filter by type, deep link to the trace, statistics count 2 accepted); the
preview-only stack shows the preflight; `journey.ts` unchanged (E-4 owns
it); CHANGELOG fragment; `docs/user-guide/` page for Verification.

### E-3 — Sessions as first-class work (`feat/unlock-3-sessions`)

The authoring loop can be resumed, reviewed, archived and exported.

Backend: capability `phiExport` (identity holds `integration.phi.export`);
nothing else new (the reads and mutations exist).

UI:
- Home › Recent work rows open the session: `/hl7?session=<id>` loads the
  session (`integrationSession`: samples, profile draft, workflow draft,
  runs, publications, diagnostics) into the intake page's session state.
- **Session sidebar** on `/hl7` (sidebar contexts already exist): runs
  (`sessionRuns`, each with its diagnostics), publications, simulations;
  select a run → its diagnostics and the per-run stream (`sessionRunEvents`,
  SSE-allowed) when the run is live.
- **Archive** (reason) and **Export** (`exportIntegrationBundle` through a
  PHI reason dialog; `includeRawPayload` offered only when `phiExport` is
  true, with the sentence that says which role is missing otherwise); the
  export downloads as JSON.
- **Accept fix**: `acceptDiagnosticFix` from the warnings list. E-5 owns
  `WarningList.svelte`; E-3 adds the API wrapper and the page handler, and
  coordinates the button through a callback prop (E-5 adds the prop; until
  E-5 merges, E-3 wires a button in the Warnings tab header).
- E-3 owns `HL7PreviewPage.svelte`, `features/hl7/**` except `WarningList`,
  `dashboard/RecentWork.svelte`, `integration-session/**`.

Acceptance: E3 checks on the operator-bundle stack (create a session, run a
preview, reload the page by deep link and see the run; archive; export
without raw payload; the `phiExport` sentence when the role is absent);
CHANGELOG fragment; user-guide section.

### E-4 — Shell honesty (`feat/unlock-4-shell`)

- **Journey from data**: a stage is complete when its evidence exists, not
  when the route is behind you: intake = a session with ≥ 1 run;
  normalization = ≥ 1 published profile; translation = ≥ 1 mapping or ≥ 1
  resolved autoroute; delivery = ≥ 1 published workflow version;
  verification = ≥ 1 accepted receipt. One `journeyState.ts` that calls the
  existing API wrappers once per shell mount (cached, refreshed on route
  change), "unknown" (not complete) when a query is forbidden or not
  configured. StageControl, StatusBar `Next:` and the sidebar badge read it.
- **One palette registry**: `commandRegistry.ts`; the shell's commands and
  HL7's editor commands register into it; Cmd/Ctrl+K opens one palette
  everywhere; the HL7 palette component is removed. (E-3 owns
  `HL7PreviewPage.svelte`: E-4 adds the registry and the shell side and
  hands E-3 a 20-line registration snippet via the coordinator; if E-3 has
  merged first, E-4 does the migration itself.)
- `TabItem.badge` on the `Tabs` primitive; BottomPanel and EditorTabs use
  `Tabs`; the split pane and Cmd+\ are removed; `markDirty` wired for
  unsaved profile/workflow/connection drafts (tabs show it, close asks);
  `<svelte:head>` titles on every route; StatusBar dead items removed or
  fed.
- **Dialog primitive** (`primitives/Dialog.svelte`: focus trap, Esc, labelled)
  and migration of `ConfirmModal`, `ControlReasonDialog`,
  `ConnectionReasonDialog` (the legacy `$lib/ui/Button.svelte` goes with
  `ConfirmModal`). Delete the unused legacy `$lib/ui/*.svelte` list and the
  orphaned `collaboration/` and `observability/` modules (Appendix A).
- E-4 owns `ui/src/lib/ui/**`, `+layout.svelte`, and no feature page.

Acceptance: shell unit tests updated; a visual refresh of V-shell
screenshots; E4 checks (journey badge reflects data on the operator-bundle
stack vs the preview-only stack; one palette on `/hl7`); CHANGELOG fragment.

### E-5 — Workflow authoring honesty (`feat/unlock-5-workflow-authoring`)

- Inline validation replaces the 33 toasts in `WorkflowBuilder.svelte`
  (`.loom/22` B1/B2): field-level messages, a disabled Save/Publish that
  says why; `window.confirm` → the Dialog primitive (local copy until E-4
  merges, then fold).
- `yamlToDraft` keeps nested action config (or the builder shows a
  "YAML-only fields" notice with the exact keys) so the baseline cannot hide
  divergence; Problems counts the draft only once it differs from the
  default.
- Rename/re-describe/archive workflow definitions
  (`updateWorkflowDefinition`, `archiveWorkflowDefinition`); archived filter.
- Verification tab: a run's trace (`workflowRunTrace`) opens in the Trace
  panel (`loadRealTraceSpans` gets its caller); the debug mock scaffold is
  deleted; DebugPanel B2 toasts become disabled-with-reason.
- `WarningList.svelte` restyled on primitives, with the `onAcceptFix`
  callback prop for E-3; `PID-3[0].1` dash-with-repetition parses in
  `hl7Path.ts`.
- E-5 owns `features/workflows/**`, `WarningList.svelte`, `hl7Path.ts`,
  `EventLineagePanel.svelte`, `panels/DebugPanel.svelte`, `debugStore.ts`,
  `workflowProblemsStore.ts`.

Acceptance: E5 checks (create a definition, save a version with a nested
action config, reload and see it preserved; Problems badge absent on the
untouched draft; archive hides it); vitest for `hl7Path` and `workflowYaml`;
CHANGELOG fragment.

### E-6 — Close-out (`docs/unlock-6-close-out`, after E-0..E-5 merge)

ROADMAP: Now/Then items closed or rewritten (Events browser legacy, polish
follow-ups, definition editor), a Delivered block; CHANGELOG fragments
assembled; a decision entry (`make decisions-new`) recording the
coordinator decisions below; a worklog entry; `docs/STATUS.md`; the
flexinfer-site docs sync run (`pnpm sync:fi-fhir-docs`) as a separate
site MR; `.loom/00-index.md` marks this spec delivered; the `.loom/38` line
50 doc drift fixed.

## Lanes and order

**E-0, E-1, E-3, E-4, E-5 start in parallel from `origin/main`. E-2 branches
from E-0's branch** (it needs the fixture and the operator deep links) and
folds `origin/main` with `git merge --no-ff origin/main` after E-0 merges
(never rebase a branch carrying a stacking merge). E-6 starts after the
others merge. Expected merge order: E-0 → E-4 → E-5 → E-3 → E-1 → E-2 → E-6;
the coordinator may reorder by readiness. After every main merge the
coordinator runs a full main pipeline by hand if the auto-cancel trimmed the
deploy stages (`POST /projects/19/pipeline?ref=main`).

Known overlaps and their owners: `schema.graphql`, resolvers, role map and
`ui/src/lib/gen/graphql.ts` (E-0 tiny, E-1, E-2, E-3 tiny: fold main and
regenerate); `capabilities.go` (E-1, E-3); `ui/e2e/run.sh` +
`check-report.mjs` (every lane, one line each); `WarningList.svelte` (E-5
only); `HL7PreviewPage.svelte` (E-3 only); `ui/src/lib/ui/**` (E-4 only;
other lanes use primitives as they are).

## Shared lane policy (verbatim in every prompt)

- **Review gate**: open the MR, write `State: REVIEW-READY` with the iid to
  your status file, **do not arm auto-merge**; never push after
  REVIEW-READY unless the coordinator sends findings.
- **Git and API from this LAN**: push with
  `git -c http.sslVerify=false -c http.extraHeader="Host: gitlab.flexinfer.ai" push https://oauth2:${GITLAB_PAT}@192.168.50.227/libs/fi-fhir.git <branch>`
  (plain `git push origin` also works for small pushes); API with
  `curl --resolve gitlab.flexinfer.ai:443:192.168.50.227 -H "PRIVATE-TOKEN: $GITLAB_PAT"`;
  print `http=%{http_code}` on every write. Never the `gitlab` MCP tools.
  Never print the token.
- **Per-lane scratch directory** `<scratchpad>/<lane>/`; status file
  `status.md` with `## State` / `## MR` / `## Pipeline` / `## Notes`.
- **Node**: the host has no `ui/node_modules`; `npm ci` (and `npm install`
  for a new dependency) inside YOUR worktree's `ui/` only; `npx`, never
  pnpm; `npx svelte-kit sync` before vitest; `npm run codegen` after any
  schema or `.graphql` change and commit `ui/src/lib/gen/graphql.ts`.
- **Go**: `go build ./... && go test ./...` before every push; the
  PostgreSQL proofs run against a Postgres on `--context 7900xtx`
  (`AGENTS.md` § Integration tests gives the recipe; connect via
  `cblevins-7900xtx:PORT`); never set `GOMODCACHE` under `.tmp/`.
- **Migrations**: `AGENTS.md` § Migration authoring, all three rules;
  re-verify numbers against `origin/main` at every fold.
- **CI layout**: a new proof is `ci/test-<name>.yml` plus one `- local:`
  line and a regenerated `ci/job-inventory.txt`; never append a job to the
  root file; one `.PHONY` line per lane in the Makefile. Declare every file
  you add, including Makefile, `ci/*.yml`, generated types and migrations.
- **Pipelines**: poll ≤ every 180 s; retry a job once only for a known
  flake (`lint:ui` heap, `build:docker`/`build:docker-ui` BuildKit "context
  canceled" or Harbor "not found", `lint:docs` Alpine DNS, "Getting source"
  curl 56 resets, `test:observability-replicas`, `security:trivy-image` DB
  drift, `test:ui-e2e` V3/check-3 10 s preview budget, `TestQuickBenchmark`);
  never cancel-retry `lint:gqlgen`; after two failures of different jobs or
  3 h on one pipeline, STOP and write status. `test:ui-e2e` failures are
  yours to read (artifacts: JUnit + `ui/e2e-results/visual/`).
- **Never touch** `CHANGELOG.md` (fragments only), `ROADMAP.md`,
  `.loom/30-*.md`, `.loom/50-worklog.md`, `.loom/40-decisions.md`,
  `platform/gitops`, `ci/_shared.yml`, or another lane's files (E-6
  excepted for ROADMAP, CHANGELOG, decision and worklog entries). Worklog via
  `make worklog-new TITLE="..."`; decisions via `make decisions-new`.
- **zsh**: unquoted `$VAR` does not word-split; `noclobber` (`>|`);
  `mv`/`rm`/`cp` are interactive (`command mv -f`); python f-strings cannot
  contain backslashes.
- **PHI/secrets**: synthetic samples only; no production credential
  anywhere; no secret value in any struct, log, fixture, screenshot, or MR.
- Commit messages conventional, scoped, trailer
  `Co-Authored-By: <the model you are> <noreply@anthropic.com>`.

## Riskiest assumption + kill-test (`spec-riskiest-assumption`)

**Load-bearing assumption (E-1)**: a definition authored through the new
API service is byte-for-byte the definition the seed CLI writes for the
same inputs (same semantic digest, so `sftp-test-demo/v1` could have come
from the IDE), and `serve` can host connection validation in-process (the
CLI's batch validator) so Validate is a page action rather than a shell
command.

**Kill test** (≤ 30 min, E-1 first): a Go test builds the definition for
the repo's batch proof inputs twice: through `buildSeedDefinition`
(`cmd/fi-fhir/lifecycle_seed.go:625`) and through the new authoring
service, and asserts equal `Digest` and equal canonical JSON; a second test
constructs the catalog the way `serve` does but with the validator installed,
runs `ValidateConnection` in `SKIP` and `STATIC` modes against a Postgres on
`7900xtx`, and asserts the recorded codes and the `validated` state.
Negative control: change one destination class and assert both digests
change identically. Disconfirming search: "lifecycle validator goroutine
leak" and whether the batch validator opens SFTP/S3 connections that the
API pod's NetworkPolicy allows (it does today: the batch runner shares the
pod).

**Failure mode if wrong**: the IDE would author definitions the runtime
cannot ingest or that differ from what operators seeded. If digests differ,
E-1 ships read-only Definitions plus the pre-flight `Check`, and authoring
stays on the CLI with the IDE printing the exact `lifecycle seed` command
for the chosen inputs. If in-process validation is unsafe, `REAL` is
dropped and the page offers `STATIC`/`SKIP` with the CLI command for real
validation.

**Status**: not run.

## Decisions taken by the coordinator

1. **Definitions live on `/connections`, deploy stays on `/operator`.** A
   definition binds connections; deployment is an operator act. No new
   route, so the eight registration points are untouched; the two pages
   link each other by deep link.
2. **Refs come from what the runtime can resolve.** The editor offers
   profile and workflow refs from the static-registry integrations, proven
   by the runtime's own resolver, and says so in a sentence, because that is
   what admission resolves today (`lifecycle_seed.go:33-52`). Moving
   resolution onto the catalog stays with `.loom/39`/`.loom/41`.
3. **Validation is per source kind and always recorded.** Batch = the real
   check the CLI runs, hosted in `serve`; MLLP/HTTP = a static check with
   its own code; skip needs a reason. The page never claims a check it did
   not run.
4. **Verification means durable admissions.** `/events` reads
   `integration_canonical_events` and receipts through `integration.operator`,
   never payload values; the legacy clinical views leave the IDE; there is
   no patient timeline and the page says why once.
5. **Sessions are resumable and exportable through the audited path
   only.** Export always carries a PHI reason; raw payload only with
   `integration.phi.export`, surfaced as a capability.
6. **Journey stages are evidence, not position.** "Unknown" when a stage's
   query is forbidden or not configured; never "complete" by default.
7. **Evidence over pixel tests**, unchanged from `.loom/37`.

## Appendix A — survey evidence (2026-09-29, condensed)

Full condensed surveys with file:line evidence are in the coordinator's
scratchpad `research/`: `survey-graphql-vs-ui.md` (root-field inventory, 44
unreached fields, REST table), `survey-lifecycle-vs-operator.md` (catalog
verbs, seed CLI, editor gaps 1–10, page inventories),
`survey-durable-vs-events.md` (table-by-table writers and readers, the
`/events` verdicts, hidden trace fields), `survey-ide-routes-shell.md`
(routes, shell, the seven open polish items, e2e projects, primitives, the
legacy component and orphaned module lists). Lanes copy what they need into
their MR descriptions; the spec cites the load-bearing facts inline.
