# IDE Repair Program — Execution Specs (2026-09-25)

> Coordinator: Claude (Fable 5.1). Lanes: Opus 5.5 agents in isolated
> worktrees, **reviewed by the coordinator before any merge** (lanes open MRs
> and do not arm auto-merge). Board at launch: 0 open MRs, main `a62e9ee63`
> (Codex's tenant-scoped service operator access on top of Sprint 7).
> Predecessors: `.loom/35-sprint7-execution-specs.md`, `.loom/23` (functionality
> gap-fill: Wave 2 "wire Copilot to the existing LLM GraphQL" never shipped).

Cody's brief: "fix the busted IDE UI/UX experience and backend connection
issues, and ensure my trusted network access allows operator permissions."
The evidence below was gathered against the **live deployment**
(`fi-fhir.flexinfer.ai`, API image `v0.1.29075`, UI image `v0.1.28948`) from
the trusted LAN (`192.168.50.16`, split-horizon DNS → ingress `192.168.50.227`),
in the built-in browser and with `curl`, on 2026-09-25/26.

---

## What is actually broken (evidence, not guesses)

| # | Symptom Cody sees | Root cause | Where |
|---|---|---|---|
| 1 | Operator page: "Your account does not hold the operator role" + toast "operator control-plane action forbidden", from the trusted LAN **and** with the static bearer | **Two role vocabularies.** The GraphQL transport gate admits `graphql:operator` (log: "admitted through the compatibility grant"); the operator service then requires `integration.operator` (`internal/integration/operator/types.go:17`, `ReadRole`), with `integration.delivery.operator` / `integration.deployment.operator` for controls — exactly as `docs/planning/GRAPHQL-API.md:141-143` documents. `platform/gitops/k3s/fi-fhir/fi-fhir-api.yaml` grants only `integration:preview,graphql:operator,clinical:read` to the bearer, the trusted network (same list — `preview_runtime.go:878-883`) and both Access principals. **Nobody in production can reach the operator plane.** The trusted-network path is not the problem. | gitops env; `/api/auth/status` (reports `authVia` only, no roles, so the UI cannot explain); UI queries then fails |
| 2 | Live Stream / Debug / Workflow Monitor / observability streams never connect ("backend connection issues") | `POST /graphql` with `Accept: text/event-stream` → **404 "Integration Session streaming is unavailable"** (`server.go:453`). `IntegrationSessionStreaming = sessionStore != nil` (`main.go:5112`) and production never sets `FI_FHIR_INTEGRATION_SESSION_ENABLED`; the UI image is built with `VITE_FI_FHIR_INTEGRATION_SESSION_ENABLED=false` (`Dockerfile.ui:26`). WebSocket is closed by design (405). | gitops env; `Dockerfile.ui`; UI surfaces that assume a stream exists |
| 3 | Copilot: "Platform connection required"; status bar "Platform" dot grey forever | `PLATFORM_CONFIG.enabled = !!PUBLIC_LOOM_ENDPOINT` (`ui/src/lib/platform/config.ts`); nothing in `Dockerfile.ui`, CI or gitops sets it, so the loom-platform MCP client never connects and `CopilotPanel` gates itself on it — although the backend already exposes `llmCapability`, `explainWarnings`, `explainWorkflow`, `suggestMappings` and production has `LLM_BASE_URL` (litellm). Dead chrome + a dead feature. | UI only |
| 4 | Problems badge shows **3 errors on every page** on a fresh session (Workflow name is required, route name is required …) | `workflowDiagnostics` is derived from the *default empty* `workflowDraft` (`workflowProblemsStore.ts:66`), so the never-opened builder's empty draft leaks validation errors into the global badge | UI only |
| 5 | `/health` polls show repeated `net::ERR_ABORTED` | `connectionStore.start()` aborts/overlaps; cosmetic today, noise in the network panel | UI only |
| 6 | `/api/auth/status` says `{"authVia":"network","authenticated":true}` and nothing else | The UI has no way to know roles or deployment capabilities, so every surface discovers its own failure by failing | backend |

Verified working: page load, credential gate ("Trusted network access active"),
dashboard, HL7 intake, workflows, events browse (0 events — the deployment has
no traffic), `/health` → 200, status bar "Connected", no console errors, SSE
allowlist code path present, WebSocket intentionally closed.

---

## Program shape

```
R-0  coordinator  gitops: grant the documented operator bundle   → unblocks #1 today
R-A  opus         backend: /api/auth/status capabilities + config lint + docs
R-B  opus         UI: capability-gated honest surfaces, Copilot on the backend LLM, Problems/Platform/health fixes
R-C  opus         repo side of streaming: UI build flag on, docs, negative path   (gitops env flip = coordinator, after)
R-D  opus         browser e2e smoke gate in CI (Playwright) proving the four states above   → merges LAST
```

Merge order: **R-0 (gitops) → R-A → R-B → R-C → R-D**. R-B consumes R-A's
contract; R-D asserts the end state of A+B+C.

| Lane | Owns | Collides with | Merge position |
|---|---|---|---|
| **R-A** | `internal/api/graphql/server.go` (auth status handler only), new `internal/api/graphql/capabilities.go` (+tests), `internal/api/requestsecurity/**` if a helper is needed, `cmd/fi-fhir/preview_runtime.go` (one startup warning), `scripts/check-runtime-config.sh`, `.env.example`, `docs/planning/GRAPHQL-API.md`, `docs/operations/RUNBOOK.md`, `docs/operations/PRODUCTION-HARDENING.md` (roles section) | R-B on nothing (UI is R-B's); R-C on `.env.example` if both add a key — append at the end, one line each | first |
| **R-B** | `ui/src/**` except `ui/e2e/**`; `ui/src/lib/gen/**` via codegen after R-A merges | R-A's schema-free (status endpoint is REST) → no gqlgen; R-D reads R-B's data-testids | second |
| **R-C** | `Dockerfile.ui`, `.gitlab-ci.yml` **only** the `build:docker-ui` block, `docs/operations/INTEGRATION-SESSIONS.md`, `docs/operations/RUNBOOK.md` (streaming section — R-A owns the roles section of the same file: different headings) | R-D on `.gitlab-ci.yml` (include list vs build block — different hunks) | third |
| **R-D** | new `ui/e2e/**`, `ui/playwright.config.ts`, `ui/package.json` (devDependency + one script), new `ci/test-ui-e2e.yml`, ONE include line appended after `ci/test-fhir-official.yml`, ONE `.PHONY` line appended after the last one, Makefile targets, `ci/job-inventory.txt` via script | everyone (it asserts their end state) | **LAST** |
| **Coordinator** | this file, `platform/gitops` (R-0 now; streaming env flip after R-C's image lands), `ROADMAP.md`, `CHANGELOG.md` (once, at close), memory | — | first and last |

Nobody but the coordinator edits `ROADMAP.md`, `.loom/30-*.md`, `CHANGELOG.md`,
or `platform/gitops`. Worklog and decisions are one file per entry
(`make worklog-new TITLE=…`, `make decisions-new TITLE=…`).

---

## R-0 — Grant the documented operator bundle (coordinator, gitops)

`FI_FHIR_GRAPHQL_ROLES` and both `FI_FHIR_GRAPHQL_ACCESS_PRINCIPALS` entries
gain `integration.operator,integration.delivery.operator,integration.deployment.operator`.
The trusted-network identity inherits the same list, so LAN access becomes an
operator the moment Flux rolls the Deployment. Acceptance: from the LAN,
`{ operatorCircuits { state } }` returns data, the operator page lists
(empty) receipts, and the API log shows the transport grant followed by no
`ErrForbidden`. Decision entry in gitops's journal; this repo's docs are R-A's.

**Not changed:** the two vocabularies stay two. `graphql:operator` is the
transport compatibility grant; the service roles are the control-plane's own
defence in depth (Slice 4.2a). What was missing was the grant, not the design.

## R-A — The auth capabilities contract (backend)

**Outcome.** `GET /api/auth/status` returns, for an authenticated caller:

```json
{"authenticated":true,"authVia":"network","principal":"fi-fhir-ide-operator",
 "roles":["integration:preview","graphql:operator","clinical:read"],
 "capabilities":{
   "operatorRead":false,"operatorDelivery":false,"operatorDeployment":false,
   "clinicalRead":true,"integrationSessions":false,"streaming":false,
   "llm":{"configured":true}},
 "missingRoles":{"operatorRead":["integration.operator"], "...":[]}}
```

Every capability is derived server-side from the same `SecurityContext` the
transport gate uses plus the server's own config (`IntegrationSessionStreaming`,
session store presence, LLM configured). No PHI, no secrets, no hostnames.
Unauthenticated callers keep today's shape. `principal` is the principal id or
the Access email the server already logs.

**Also:**
- **Startup warning** (never fatal): when any configured identity holds
  `graphql:operator` but not `integration.operator`, log one WARN naming the
  identity and the missing roles ("transport grant without control-plane
  role: operator surfaces will be forbidden"). Today's production would have
  logged it on 2026-09-05.
- **`scripts/check-runtime-config.sh`**: the existing `graphql:operator`
  check for compose/local also requires the three `integration.*.operator`
  roles; `.env.example` carries the full bundle with a comment.
- **Docs**: `GRAPHQL-API.md` gets "What an operator's token must carry" — the
  bundle in one copy-pasteable line — and the status endpoint's contract;
  `RUNBOOK.md` gets "Operator page says the role is missing" pointing at the
  three places roles are granted (static env, trusted network inherits it,
  Access principals); `PRODUCTION-HARDENING.md` roles section cites the bundle.

**Day-1 gate** (passes on main, inverted at ship): a test that a trusted-network
request to `/api/auth/status` returns exactly the two keys today; then the new
shape. **Kill-test**: with roles `graphql:operator` only, `capabilities.operatorRead`
is `false` and `missingRoles.operatorRead == ["integration.operator"]`; with the
bundle, all three operator capabilities are `true`. Streaming capability flips
with `IntegrationSessionStreaming`.

**Not in scope**: any change to who is admitted, any role expansion, gqlgen.

## R-B — Honest surfaces (UI)

**Outcome.** The IDE never lets a user walk into a surface the deployment
cannot serve, and never pretends a feature exists that is not configured.

1. **Capabilities store**: the credential gate's status fetch populates an
   `accessCapabilities` store (typed from R-A's contract; tolerate the old
   two-key shape as "unknown").
2. **Operator page** (`features/operator/OperatorPage.svelte` and tabs): when
   `operatorRead` is false, render a pre-flight `Panel tone="warning"` — "This
   deployment's identity holds `graphql:operator` but not
   `integration.operator`; operator surfaces are unavailable. Roles are granted
   in the API's `FI_FHIR_GRAPHQL_ROLES` (static and trusted-network) and
   `FI_FHIR_GRAPHQL_ACCESS_PRINCIPALS`" — and **do not** issue the queries.
   The forbidden response, if it still happens, stays inline only (the toast
   is a B4 double against the inline error; `isErrorToasted` guard).
   Controls (replay/resubmit/discard; pause/resume/retire/deploy) are disabled
   with explanatory `title` when their capability is false.
3. **Streaming surfaces** (Events → Live Stream, `DebugPanel`,
   `WorkflowMonitor`, `observabilityStore`, `RuntimeOutputPanel` if it
   subscribes): when `streaming` is false, show one honest empty state —
   "Live streaming is not enabled on this deployment
   (`FI_FHIR_INTEGRATION_SESSION_ENABLED`)" — and do not open the stream. When
   a stream that was open fails with HTTP 404, treat it the same way (the
   status can be stale after a redeploy).
4. **Copilot on the backend LLM** (`.loom/23` Wave 2, finally): `CopilotPanel`
   drops the platform gate. Its source of truth is `llmCapability` (already
   probed by `llmCapabilityStore`) and the existing `explainWarnings` /
   `explainWorkflow` / `suggestMappings` operations. States: LLM configured and
   reachable → usable; configured but unreachable → "The deployment's LLM is
   not responding" with the capability warnings; not configured → "No LLM is
   configured for this deployment". The loom-platform client stays for the
   HUD ambient features only, and **only** when `PLATFORM_CONFIG.enabled`.
5. **Platform chrome**: the status-bar "Platform" indicator and `AlertBadge`
   render only when `PLATFORM_CONFIG.enabled`; `initializePlatform()` is not
   called otherwise (it already no-ops, but the indicator must go too).
6. **Problems badge**: `workflowDiagnostics` counts only when a draft is
   *live* — the builder has been opened in this session or the draft differs
   from the empty default. A fresh session shows 0. The panel copy for the
   empty state says where problems come from.
7. **connectionStore**: one owner, one in-flight request, no overlapping
   aborts; `stop()` on layout destroy.
8. Tests for every state above (vitest; `data-testid` on the new states for
   R-D: `operator-preflight`, `streaming-unavailable`, `copilot-llm-state`,
   `platform-indicator`, `problems-badge`).

**Day-1 gate**: a vitest that, with today's two-key status, the operator page
issues its list query immediately (passes today; inverted at ship: it does
not). **Kill-test** for the Copilot: with `llmCapability` returning
configured-and-healthy and `PLATFORM_CONFIG.enabled=false`, the panel is
usable and no "Platform connection required" text renders.

**Env note**: `ui/node_modules` — symlink from the canonical checkout
(`/Users/cblevins/workspace/libs/fi-fhir/ui/node_modules`) if it exists, else
`npm ci` inside the worktree's `ui/` (Lane S7-C found the canonical one gone).
`npx svelte-kit sync` before `npx vitest run`, `npm run check`, `npm run lint`.

## R-C — Streaming on, repo side

**Outcome.** The production UI image is built with
`VITE_FI_FHIR_INTEGRATION_SESSION_ENABLED=true` (Dockerfile default flipped,
with the comment explaining that the API's env decides at runtime and the UI
degrades honestly through R-A's `streaming` capability when the API has it
off); `INTEGRATION-SESSIONS.md` "Enable the workspace" gains a production
section (the exact gitops lines, the DB it migrates into, rollback);
`RUNBOOK.md` streaming section. Negative path: a test or script proving that
with the flag on and the API's streaming off, the UI shows R-B's honest empty
state rather than an error (coordinate the id with R-B: `streaming-unavailable`).
The **gitops env flip** (`FI_FHIR_INTEGRATION_SESSION_ENABLED=true`, same DB
env already present) is the coordinator's, after this lane's image is on main.

**Not in scope**: signed publication keys (separate feature, separate keys),
multi-replica fanout.

## R-D — The browser smoke gate (CI)

**Outcome.** A blocking job `test:ui-e2e` runs Playwright (Chromium) against
the **built** UI served by its nginx template and a real `fi-fhir serve` with
PostgreSQL, trusted network `127.0.0.1/32`, the full operator bundle, and
streaming on. It proves, with screenshots on failure as artifacts:

1. `/api/auth/status` → `authVia: network`, `capabilities.operatorRead: true`,
   `streaming: true`.
2. Operator page shows the Messages list (empty) — no `operator-preflight`,
   no "forbidden" text anywhere.
3. Events → Live Stream opens: the SSE POST returns 200 `text/event-stream`
   and the page shows its "listening" state within 10 s.
4. Copilot panel shows `copilot-llm-state` = not configured (no LLM in CI) and
   never "Platform connection required".
5. `problems-badge` absent on a fresh load; `platform-indicator` absent.
6. **Negative control**: a second run with `FI_FHIR_GRAPHQL_ROLES` lacking
   `integration.operator` must show `operator-preflight` naming that role —
   the gate has to be able to fail for the reason it exists.

`make ui-e2e` mirrors it. Two jobs if the browser image needs separating from
the Go image (`test:ui-e2e-build` artifacts → `test:ui-e2e`), both through
`scripts/ci-job-inventory.sh --write`. Merges last; rebases on A, B, C.

---

## Shared lane policy (verbatim in every prompt)

- Branch from a fresh `origin/main`: `feat/ide-repair-a-auth-capabilities`,
  `feat/ide-repair-b-honest-surfaces`, `feat/ide-repair-c-streaming-build`,
  `test/ide-repair-d-browser-smoke`.
- **Review gate**: open the MR, write `State: REVIEW-READY` with the MR iid to
  your status file, **do not arm auto-merge**. The coordinator reviews the
  diff and either arms it or sends you findings; address findings, push, and
  set `State: REVIEW-READY` again. Only the coordinator arms.
- **Git and API from this LAN**: probe `nc -z -G 3 192.168.50.227 443`; push
  with `git -c http.sslVerify=false -c http.extraHeader="Host: gitlab.flexinfer.ai" push https://oauth2:${GITLAB_PAT}@192.168.50.227/libs/fi-fhir.git <branch>`;
  API with `curl --resolve gitlab.flexinfer.ai:443:192.168.50.227 -H "PRIVATE-TOKEN: $GITLAB_PAT"`;
  print `http=%{http_code}` on every write and treat `000` as failure. Never
  the `gitlab` MCP tools. Never print the token.
- **Per-lane scratch directory** for every file you write outside the repo:
  `<scratchpad>/<lane>/` — Sprint 7 lanes overwrote each other's helpers.
- **Pipelines**: poll no more than every 180 s. Retry a job **once** only for
  a known flake (`test:mllp-runtime`, `test:observability-replicas`,
  `test:integration` under load, `security:govulncheck` OOM,
  `security:trivy-image` daily-DB drift, `lint:ui` heap). Never cancel-retry
  `lint:gqlgen`. After two failures of different jobs, or 3 hours on one
  pipeline, STOP and write status.
- **Never touch** `CHANGELOG.md`, `ROADMAP.md`, `.loom/30-*.md`,
  `.loom/50-worklog.md`, `.loom/40-decisions.md`, `platform/gitops`, or
  another lane's files.
- **Go env**: `.tmp/go-mod-cache/` is committed — never delete it or point
  `GOMODCACHE` at it. PostgreSQL via `docker --context 7900xtx run -d --name <lane>-pg -p <port>:5432 -e POSTGRES_PASSWORD=postgres postgres:16`,
  host `cblevins-7900xtx`; remove the container when done. `gofmt -l`,
  `golangci-lint run`, and the slice's `make` targets before pushing.
- **zsh**: unquoted `$VAR` does not word-split; `noclobber` (`>|`);
  `mv`/`rm`/`cp` are interactive (`command mv -f`); never `echo` JSON into
  `jq`.
- **PHI/secrets**: the production bearer and any key never enter a transcript
  or a status file. Use test values only.
- Commit messages conventional, scoped, with the `Co-Authored-By` trailer.

---

## Riskiest assumption of the program

> "A capabilities endpoint derived from the transport identity is enough for
> the UI to be honest."

If some surface's real gate lives deeper (a per-field check the transport does
not see), the UI will still discover a failure by failing. R-A's kill-test and
R-D's negative control decide whether the contract covers the operator plane;
streaming is a server config bit, so it is covered by construction. The
Copilot's honesty rests on `llmCapability` already answering truthfully, which
`.loom/23` Slice 3f established.

## Decisions taken by the coordinator (recorded here, entries in the repos)

1. **Grant, don't alias.** `graphql:operator` does not imply the service
   roles; the deployment grants the documented bundle. Rationale: 4.2a's
   defence in depth is the reason the control plane has its own roles.
2. **Copilot runs on the backend LLM, not the loom platform.** The platform
   client remains an optional HUD integration behind `PUBLIC_LOOM_ENDPOINT`.
3. **Streaming is enabled in production** (env flip after R-C), because the
   IDE's verification stage is designed around it and the DB it needs is
   already there; signed publication stays off.

---

## Corrections (2026-09-26, from Lane R-C's kill-test — supersede anything above that conflicts)

Lane R-C read the code before flipping the flag and stopped, correctly. Three
facts change the program:

1. **The UI session flag has no fallback.** `VITE_FI_FHIR_INTEGRATION_SESSION_ENABLED`
   drives one function, `isIntegrationSessionEngineEnabled()`
   (`ui/src/lib/features/integration-session/api.ts:48-50`). With it on, HL7
   intake preview abandons the stateless `previewIntegrationMessage` path (the
   "sole supported IDE preview path", `hl7Preview.ts:19`) for the session
   engine, and `DryRunPanel` offers a "Session" source and loads
   `integrationSessions` on open. If the API has sessions off, every one of
   those calls fails with "legacy integration execution is unavailable"
   (`schema.resolvers.go:991,2366`), toasted. **Flipping the default with
   today's UI would break HL7 intake in production.** → R-B now owns:
   session engine only when the build flag is on **and**
   `capabilities.integrationSessions` is true; stateless preview otherwise;
   dry-run offers the Session source only then. R-C's Dockerfile flip waits
   for R-B.
2. **Streaming is per subscription, and most panels can never stream.** The
   transport allowlist admits only `integrationSessionEvents` and
   `sessionRunEvents` (`internal/api/graphql/operation_authorization.go:156-173`,
   pinned by `server_security_test.go:466-477`, which shows `eventStream`
   answered `FORBIDDEN` with streaming on). Events → Live Stream
   (`eventStream`), Workflow Monitor (`workflowEvents`), Debug
   (`debugStepEvent`) and Runtime Output subscribe to roots the durable API
   will never serve. **Decision: the allowlist is not widened** — it is the
   durable path's PHI-minimal stream surface by design. Instead:
   - R-A adds `capabilities.subscriptions: [<allowlisted roots for this caller>]`
     derived from the transport's own constant (empty when streaming is off).
   - R-B keys every streaming surface on membership: the four legacy panels
     render the honest state ("Live streaming for … is not available on this
     deployment; the Events browser and, when enabled, the Integration
     Session stream are") and never subscribe; session panels are available
     only when their roots are listed. `data-testid="streaming-unavailable"`
     plus `data-stream="<root>"`.
   - R-D's check 3 becomes: with streaming on, an Integration Session stream
     opens (SSE 200 `text/event-stream` on `integrationSessionEvents`), **and**
     the Live Stream tab shows the honest state. R-D's negative control gains
     a second case: streaming off → `capabilities.subscriptions == []` and the
     session panels show the honest state.
3. **Spec errata.** The flag lives in `ui/Dockerfile:26` (there is no
   `Dockerfile.ui`); `build:docker-ui` (`.gitlab-ci.yml:1902-1912`) passes no
   build arg, so the Dockerfile default is what production gets and no CI
   change is needed for R-C.

**Lane environment note.** The coordinator launched the lanes while its shell
was in `platform/gitops`, so the harness created their worktrees there. R-A
cloned fi-fhir inside its worktree and works from the clone; R-B was told to
do the same; R-C stopped before needing one. Relaunches (R-C, R-D) must be
issued from the fi-fhir worktree. The stray gitops worktrees are removed at
close.

**Revised order:** R-0 (gitops, armed) → R-A → R-B (now including the two
items above) → R-C (Dockerfile flip + production docs; relaunch after R-B
merges) → coordinator gitops env flip → R-D (relaunch after A+B+C).
