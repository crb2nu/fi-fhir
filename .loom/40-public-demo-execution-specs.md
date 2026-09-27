# Public demo, portfolio links, docs coverage — execution specs (2026-09-27)

Brief (Cody, 2026-09-27, after the Connections program C-0..C-2 merged and
C-3/C-4 were in flight): "prepare a demo deployment for public access (like
the edilint in the browser) and linking from my portfolio site(s), also
ensuring the docs integration is updated and docs cover our new features."

Coordinator designs and reviews; lanes implement and open MRs for review; the
coordinator arms. Predecessors: `.loom/38` (connections), `.loom/39`
(config-plane brainstorm). Evidence for every claim below is in the two
surveys of 2026-09-27 (edilint demo hosting; fi-fhir demo deployment shape)
and the WASM kill-test, condensed in Appendix A.

## What "like edilint in the browser" means here

edilint's public demo is **WebAssembly only**: `cmd/edilint-wasm/main.go`
exposes six string-in/string-out JavaScript globals ("no filesystem, no
network. Nothing a page pastes into it leaves the page"); GitHub Actions on
the `crb2nu/edilint` mirror publishes `edilint_<ver>_wasm.tar.gz` +
`checksums.txt` on `v*` tags; flexinfer-site's `pnpm build` runs
`scripts/fetch-edilint-release.mjs` (resolve latest release → verify SHA-256 →
smoke-test in Node → stage into `public/wasm/` → write `edilint.json`), commits
the ~4.2 MB module, and `components/playground/edilint/useEdilint.ts` loads it
with `instantiateStreaming`. The page is served by the same Next.js pods that
serve flexinfer.ai and codyblevins.com (one image, two Deployments, variant by
Host header). No gitops, ingress, DNS, or Access change was needed.

fi-fhir already has a public browser playground at
`flexinfer.ai/playground/fi-fhir` (profiles · pipeline · mapper), but it runs a
**hand-written TypeScript HL7v2 splitter** on fixtures copied from a local
checkout, not the engine. The honest edilint-style demo is therefore: **the
real kernel compiled to WASM, powering that page**. The IDE proper (SvelteKit
+ GraphQL + PostgreSQL) cannot be WASM; a hosted public deployment of it is a
second, gated deliverable (see D-3 and "Then").

### Kill-test: does the kernel fit in a browser? — PASSED 2026-09-27

`GOOS=js GOARCH=wasm go build -trimpath -ldflags='-s -w'` on main `1f6ac1198`:

| linked packages | raw | gzip -9 |
|---|---|---|
| `internal/parser/hl7v2` | 3.05 MB | 0.87 MB |
| **slim core**: hl7v2 + `pkg/profile` + `pkg/fhir` + `pkg/events` + `pkg/integration` + `internal/integration/fhirout` + `pkg/validate` | **5.28 MB** | **1.49 MB** |
| all parsers (hl7v2, cda, edi, csv, fhir) | 5.55 MB | 1.57 MB |
| anything importing `internal/integration/processor` (preview, session) | 72.2 MB | 12.5 MB |

Every package compiles. The processor's weight is `internal/workflow`
(cel-go, protobuf, lib/pq, prometheus, otel, redis, `pkg/llm/prompts`), imported
by exactly one file, `internal/integration/processor/workflow_plan.go:52-56`.
The session runner's path (`internal/integration/session/runner.go:75-135`)
is: `processor.CompileProfileRevision` → `hl7v2.NewParser(source, config)` →
`ParseResult{events, warnings}` → `fhirout.MapEvent` /
`CreateConditionalTransactionBundle`. So the WASM kernel needs
`CompileProfileRevision` without `workflow_plan.go`, which is a build tag.

## Two production findings that gate any hosted public deployment

Found by the deployment survey; both reachable today by any holder of
`graphql:operator` (the LAN CIDR, the two Access identities, the bearer
token), so production exposure is low but real. They ship as lane D-4
regardless of the demo.

1. **The workflow debugger executes actions.** `startDebugSession`
   (`internal/api/graphql/resolvers/schema.resolvers.go:1766-1787` →
   `internal/workflow/debug.go:152` → `Engine.ProcessWithContext`) runs
   caller-supplied YAML; `NewEngine` registers `webhook`, `exec`, `file`,
   `database`, `queue`, `fhir`, `email`, `athena` by default
   (`internal/workflow/engine.go:124-133`); the `exec` allowlist is read from
   the same caller-supplied config (`internal/workflow/actions.go:383-409`).
2. **`createFhirSubscription` connects to any URL** (`schema.resolvers.go:
   850-870`, `resolvers/resolver.go:528-554`), no allowlist, unbounded client
   map; `graphql:operator` only (`operation_authorization_roles.go:256, 303`).
   The `fi-fhir` namespace has no NetworkPolicy (`k3s/fi-fhir/namespace.yaml:11`,
   `network-policy-mode: open-legacy`).

Also: `requestsecurity/trusted_network.go:95-116` accepts `0.0.0.0/0` with no
guard (documented "single-tenant LAN deployments only"); there is no GraphQL
rate limit anywhere; there is no viewer role and no demo mode.

## Design

### Deliverables

| # | Deliverable | Where | Public? | Depends on |
|---|---|---|---|---|
| D-0 | WASM kernel + release asset | fi-fhir | yes (module) | — |
| D-1 | `/playground/fi-fhir` runs the real kernel; portfolio links; runbook | flexinfer-site | yes | D-0 asset (or a local build of its branch) |
| D-2 | Docs cover the new features and the site's docs sync is current and checked | fi-fhir + flexinfer-site | yes | C-4 merged |
| D-3 | Hosted demo, **Tier A**: IDE shell with preview-only identity, no DB | gitops + fi-fhir (fourth e2e stack, honest states) | yes | D-4 for the NetworkPolicy pattern only |
| D-4 | Hardening: debugger action gate, subscription URL allowlist, bounded maps, fi-fhir NetworkPolicy | fi-fhir + gitops | n/a | — |
| Then | Hosted demo **Tier B** (full IDE, ephemeral Postgres, nightly reset, demo switch) | gitops + fi-fhir | yes | D-4 merged and live; Cody's go |

### D-0 — WASM kernel (`cmd/fi-fhir-wasm`)

The JavaScript contract (all globals take and return strings; JSON payloads
are documented in `docs/user-guide/playground.md` by D-2 and pinned by a Node
smoke test in D-0):

| global | input | output |
|---|---|---|
| `fiFhirReady` | — | `true` once `main` has registered the functions |
| `fiFhirVersion()` | — | `{version, commit, builtAt, kernel: "slim"}` |
| `fiFhirProfiles()` | — | `[{id, name, description, yaml}]`: the built-in example Source Profiles embedded from `testdata/` (synthetic, PHI-free) |
| `fiFhirSamples()` | — | `[{id, name, format, text}]`: the six demo samples the IDE ships (`ui/src/lib/features/hl7/samples/demoSamples.ts` is the list; embed the same bytes from `testdata/` so the two cannot drift, with a test) |
| `fiFhirPreview(json)` | `{message, format: "hl7v2", profileYaml?, timezone?}` | `{ok, segments: [...], events: [{type, payload}], diagnostics: [{severity, code, path, message}], bundle?: <FHIR transaction Bundle>, problems: [{code, path, message}]}` — parse with the compiled profile (or the parser defaults when `profileYaml` is absent), map every supported event with `fhirout`, and never throw: a bad profile or message is `ok: false` with `problems`, mirroring `ConnectionProblem`. |
| `fiFhirValidateProfile(yaml)` | profile YAML | `{ok, problems}` from `CompileProfileRevision` |

Rules: input capped at 1 MiB (`processor/message_processor.go:42`'s cap, same
constant); findings and events bounded; no `syscall/js` outside `cmd/fi-fhir-wasm`;
`//go:build !js` on `processor/workflow_plan.go` (and a `js` stub that returns
`ErrWorkflowUnavailable`) so the processor's compile path links slim; a
`TestWASMKernelSizeBudget`-style gate in CI (`GOOS=js GOARCH=wasm go build` +
`gzip -9 | wc -c` ≤ **2.5 MB**, a `ci/test-wasm.yml` proof with one `- local:`
line and a regenerated `ci/job-inventory.txt`); `make wasm` writes
`dist/wasm/fi-fhir.wasm` + `wasm_exec.js`; a Node smoke script
(`scripts/wasm-smoke.mjs`) loads the module and asserts the contract on the
built-in samples (this is what flexinfer-site's fetcher will re-run).
**Release asset**: `.github/workflows/wasm-release.yml` in fi-fhir (runs on
the `crb2nu/fi-fhir` mirror on `v*` tags, like edilint's `release.yml`):
builds, tars `fi_fhir_<ver>_wasm.tar.gz` (module, `wasm_exec.js`, LICENSE,
`samples/`, `profiles/`), writes `checksums.txt`, publishes a GitHub Release.
Verify the mirror job (`.gitlab-ci.yml` `mirror:github`) pushes tags — it does
(`git push --force-with-lease --tags github`). The first tag is cut by the
coordinator after merge. Not in scope: any UI.

### D-1 — the playground runs the kernel; the portfolio links it

In flexinfer-site (GitLab project 5 is canonical — the local checkout's only
remote is the GitHub mirror and is stale; add
`https://gitlab.flexinfer.ai/services/flexinfer-site.git` as `gitlab` and
branch from its `main`):

- `scripts/fetch-fi-fhir-release.mjs` modelled line-for-line on
  `fetch-edilint-release.mjs` (latest GitHub release of `crb2nu/fi-fhir` or
  `FI_FHIR_RELEASE` pin; SHA-256 against `checksums.txt`; Node smoke test of
  the contract above; stage into `public/wasm/fi-fhir.{wasm,json}`;
  fail-soft keeps the committed files); hook it into `package.json` `build`
  beside the edilint fetch; `lib/fi-fhir-release.ts`; `.gitignore` exception.
  Until the first fi-fhir tag exists, build the module locally from D-0's
  branch (`make wasm` in a fi-fhir checkout) and commit it with `fi-fhir.json`
  saying `source: local-build@<sha>`; the fetcher replaces it on the first
  release.
- `components/playground/fi-fhir/useFiFhirWasm.ts` (pattern:
  `useEdilint.ts`: `instantiateStreaming` with ArrayBuffer fallback, `fiFhirReady`
  poll with a 10 s timeout, CSP-refusal reload once) **running in a Web
  Worker**, with a 1 MiB input cap in the page; the existing
  `hl7v2-parse.ts` splitter is replaced by `fiFhirPreview`, and the three tools
  (profiles · pipeline · mapper) show the kernel's segments, events,
  diagnostics and FHIR Bundle. The fixtures generator
  (`scripts/gen-fi-fhir-fixtures.mjs`) is retired in favour of
  `fiFhirSamples()`/`fiFhirProfiles()`. Retire the edilint playground's
  main-thread lint into the same Worker helper only if it is a one-file change;
  otherwise leave it and note it.
- **Portfolio links, every list** (the survey found them duplicated):
  `data/projects.config.json` (fi-fhir entry: `playgroundLabel: "Try in
  browser"`, `demo` left for D-3's host, `docs` kept) then `pnpm gen-projects`;
  `app/portfolio/page.tsx` `workEntries`; `components/playground/hub/tools.ts`;
  `components/playground/shared/PlaygroundSectionNav.tsx`; `app/home/CodyHome.tsx`;
  `components/systems/hub/hub-data.ts`; `components/products/product-evidence.ts`;
  `app/llms.txt/route.ts`; `app/sitemap.ts`. Both variants (flexinfer.ai and
  codyblevins.com) render from the same lists; check both in the e2e.
- `lib/security-headers.ts:52-56` comment updated (two routes use WASM; the
  policy already has `'wasm-unsafe-eval'`).
- **Runbook** `docs/adding-a-project-demo.md` in flexinfer-site: how to add a
  project to the registry and every hand-written list, how to publish a WASM
  demo (fetcher, release contract, size budget, Worker), how to add a docs set
  (link to the existing "Adding a New Docs Project" in `AGENTS.md`).
- Tests: `__tests__/lib/fi-fhir-release.test.ts`, playground-hub test, an e2e
  `e2e/playground-fi-fhir.spec.ts` that pastes a sample and asserts a Bundle
  renders and no network request left the page (Playwright request log:
  nothing but the static assets).
- Evidence: before/after screenshots of `/playground/fi-fhir` on both hosts.

### D-2 — docs cover the new features; the docs integration is current

After C-4 merges (it writes `docs/user-guide/connections.md`,
`docs/operations/CONNECTION-CATALOG.md`, ROADMAP, decisions, worklog):

- fi-fhir: `docs/user-guide/playground.md` (the WASM contract, what runs in
  the browser vs the IDE, PHI stance); `docs/user-guide/README.md`/index and
  the root `README.md` feature list mention Connections, engine properties,
  sample intake, the playground, and the demo; publish the IDE's own
  `ui/docs/USER-GUIDE.md` by adding it to the docs sync (either move it under
  `docs/user-guide/ide.md` with a stub left behind, or add `ui/docs` to the
  sync sources — pick the one that keeps `test:docs-status` and `lint:docs`
  green); the `ui/docs/DESIGN.md` capture table gets the three connections
  captures if C-4 did not add them.
- flexinfer-site: run `pnpm sync:fi-fhir-docs`, update
  `content/fi-fhir-docs/nav.yaml` for the new pages, commit the synced content;
  add a scheduled CI job `docs:sync-check` (nightly) that runs the sync in a
  temp dir and fails with a diff when the committed content is stale — the
  "docs integration" is manual today (`AGENTS.md:178-180`) and this makes
  drift visible without automating commits; document it in
  `docs/flexinfer-docs-integration.md`. Automated MR-on-drift is a follow-up.
- edilint's docs live on GitHub Pages only; add a `doc-sets.ts` external entry
  pointing at `https://crb2nu.github.io/edilint/` so the docs hub lists it.

### D-3 — hosted demo, Tier A (`fi-fhir-demo.flexinfer.ai`)

What ships: the real IDE shell and the HL7 preview against a real API, with a
**preview-only identity and no database**. Honest by construction: every
surface that needs `graphql:operator` or the DB shows its existing
"not available on this deployment" state.

- gitops `k3s/fi-fhir-demo/` (a new directory beside `k3s/fi-fhir/`, added to
  `k3s/flux/apps/services/kustomization.yaml`; **not inside `k3s/fi-fhir`**,
  whose kustomization forces the namespace): namespace `fi-fhir-demo` with the
  security-posture classification labels, API + UI Deployments reusing the
  `fi-fhir` and `fi-fhir-ui` ImagePolicies with `$imagepolicy` markers and a
  new `fi-fhir-demo-imageupdateautomation.yaml` (`path: ./k3s/fi-fhir-demo`),
  the same `integration-registry` ConfigMap generator (hash-suffixed,
  immutable — read the memory on hashed ConfigMaps before touching
  kustomization.yaml), Ingress host `fi-fhir-demo.flexinfer.ai` (single label:
  two-label hosts get no edge certificate) with `limit-rps`/`limit-connections`
  annotations (pattern: `flexdeck-proxy-ingress.yaml:8-10`), an
  **egress-deny NetworkPolicy** (allow DNS only), resources as production.
  API env: `FI_FHIR_DEPLOYMENT_TENANT_ID=tenant-a`,
  `FI_FHIR_INTEGRATION_REGISTRY_PATH`, `FI_FHIR_GRAPHQL_ALLOWED_ORIGINS=https://fi-fhir-demo.flexinfer.ai`,
  `FI_FHIR_GRAPHQL_PRINCIPAL_ID=fi-fhir-demo-visitor`,
  `FI_FHIR_GRAPHQL_ROLES=integration:preview`, `FI_FHIR_GRAPHQL_BEARER_TOKEN_FILE`
  (a new random SOPS secret nobody is given), `FI_FHIR_GRAPHQL_TRUSTED_CIDRS=0.0.0.0/0,::/0`
  (every caller is the visitor; the transport gate admits only
  `previewIntegrationMessage` and `health` for that role —
  `operation_authorization.go:183-210`), `FI_FHIR_LOG_FORMAT=json`; args
  `serve --no-playground --no-introspection`. Unset: every `FI_FHIR_DATABASE_*`,
  sessions, control plane, LLM, Access, MLLP, batch, delivery.
  `docs/security/external-services.md` gains a row and
  `scripts/security/check-external-services-inventory.sh` passes.
- **Manual step for Cody (not in git):** a per-host Cloudflare Access
  **Bypass** application for `fi-fhir-demo.flexinfer.ai`, because the wildcard
  Access app gates every single-label host (`external-services.md:23-30`).
  The lane writes the exact policy to its status file; the coordinator
  reports it.
- fi-fhir: a **fourth e2e stack** `preview-only` (:3003 / API :18084) in
  `ui/e2e/run.sh` with exactly the demo's env (roles `integration:preview`,
  no DB, `--no-introspection`), and a spec that walks every route asserting
  the honest state (`EmptyState`/preflight `data-testid`s, no `.toast.error`,
  no "forbidden" copy where "not available" is meant) and that HL7 Preview
  works. Fix honest states in `ui/src` where a route is blank or wrong with
  this identity (the survey could not verify them). Also guard
  `trusted_network.go` so `0.0.0.0/0` requires an explicit
  `FI_FHIR_GRAPHQL_TRUSTED_CIDRS_ALLOW_ANY=true` opt-in that logs a startup
  warning, and set it in the demo overlay.
- A "Demo" banner in the IDE shell when `/api/auth/status` reports the
  principal id `fi-fhir-demo-visitor`? **No** — no synthetic branching on a
  name. Instead the StatusBar already shows the principal and roles; the demo
  page on flexinfer-site explains what the demo can do.
- `data/projects.config.json` `demo` field is set by D-1 to the host once D-3
  is live (coordinator sequences).

### D-4 — hardening (ships regardless of the demo)

- `startDebugSession`: the engine used by the debugger registers only the
  actions named by a deployment-owned allowlist `FI_FHIR_WORKFLOW_DEBUG_ACTIONS`
  (default: none → the debugger runs the workflow with every action stubbed as
  a recorded no-op, which is what a debugger should do); the caller's YAML can
  no longer widen the `exec` allowlist (the allowlist becomes
  deployment-owned `FI_FHIR_WORKFLOW_EXEC_ALLOWLIST`, empty by default);
  debug sessions are bounded (max concurrent per principal, TTL) with a
  metric. Existing debugger e2e/unit tests keep passing with stubbed actions.
- `createFhirSubscription`: destination host must match
  `FI_FHIR_FHIR_SUBSCRIPTION_ALLOWED_HOSTS` (empty → refused with an
  inventory-safe message); the client map is bounded and evicts.
- gitops: a NetworkPolicy for the `fi-fhir` namespace (egress: DNS, the
  namespace's Postgres, HAPI, LiteLLM, Qdrant, Temporal as actually used —
  read the manifests; ingress: ingress-nginx only), moving the namespace off
  `open-legacy`, with `make status`/Flux verification and a rollback note.
- `docs/operations/SECURITY.md` (or the existing security doc) gains the two
  findings as closed items with dates; a decision entry via `make decisions-new`.

## Lanes and order

**D-0, D-4, D-3, D-1 start in parallel; D-2 starts when C-4 merges.**
D-1 integrates D-0's module from a local build of D-0's branch and switches
to the release fetcher after the coordinator cuts the first tag. D-3's gitops
MR is opened but **not armed** until the coordinator confirms the Access
Bypass app exists (Cody) and D-3's fi-fhir e2e stack is green on main. Tier B
is not in this program.

Branches: `feat/demo-0-wasm-kernel` (fi-fhir), `feat/demo-1-playground-kernel`
(flexinfer-site), `docs/demo-2-docs-coverage` (fi-fhir) + `docs/demo-2-docs-sync`
(flexinfer-site), `feat/demo-3-preview-only-stack` (fi-fhir) + `feat/fi-fhir-demo`
(gitops), `fix/demo-4-debugger-subscription-hardening` (fi-fhir) +
`feat/fi-fhir-network-policy` (gitops).

## Shared lane policy (verbatim in every prompt)

- fi-fhir: as `.loom/38` "Shared lane policy" (LAN push fallback, API via
  `curl -H "PRIVATE-TOKEN: $GITLAB_PAT"` printing `http=%{http_code}`, never the
  gitlab MCP tools, never print the token; `go build ./... && go test ./...`
  before every push; new CI proof = `ci/test-<name>.yml` + one `- local:` line +
  regenerated `ci/job-inventory.txt`; never `GOMODCACHE` under `.tmp/`; status
  file `<scratchpad>/<lane>/status.md` with `## State`/`## MR`/`## Pipeline`/
  `## Notes`; REVIEW-READY then no pushes; never arm auto-merge; known-flake
  retry list; STOP rules).
- flexinfer-site (project 5): pnpm (this repo uses pnpm, unlike fi-fhir's UI);
  `pnpm install --frozen-lockfile`, `pnpm lint`, `pnpm test`, `pnpm build`
  (which runs the fetchers fail-soft), Playwright e2e per its README; branch
  from the `gitlab` remote's `main`; do not commit `.wasm` files other than the
  two demo modules; never commit secrets; the image is built by GitLab CI and
  rolled by Flux image automation, so a merged MR is live within minutes —
  screenshots of both hosts after merge are the evidence.
- platform/gitops (project 1): read the memories `gitops-mr-shipping-recipe`
  and `gitops-hashed-configmap-immutable-job` first (branch from
  `gitlab-vm/main` in `.worktrees/`; hooks run under Python 3.9 + bash 3.2;
  push via the named remote `gitlab-vm`; render with `kubectl kustomize` before
  and after any kustomization.yaml change; never `kubectl edit`; SOPS + age for
  the new bearer secret with the repo's public key; `scripts/security/check-external-services-inventory.sh`
  must pass). Never arm auto-merge on gitops MRs; the coordinator reviews the
  rendered diff.
- zsh: unquoted `$VAR` does not word-split; `noclobber` (`>|`); `mv`/`rm`/`cp`
  are interactive aliases (`command rm -f`); python f-strings cannot contain
  backslashes.
- PHI/secrets: synthetic samples only; the WASM module embeds only `testdata/`
  bytes; no secret value in any struct, log, fixture, screenshot, or MR.
- Commits conventional and scoped, trailer
  `Co-Authored-By: <the model you are> <noreply@anthropic.com>`.

## Riskiest assumption + kill-test (`spec-riskiest-assumption`)

**Load-bearing assumption**: the slim kernel not only links (verified: 1.49 MB
gzipped) but **produces the same events, diagnostics and Bundle as the IDE's
session preview** for the six demo samples, without `internal/workflow` —
i.e. nothing on the `CompileProfileRevision` → parse → `fhirout.MapEvent`
path secretly depends on the workflow planner or on a database.

**Kill test** (≤30 min, D-0 does it first): build `cmd/fi-fhir-wasm` with
`workflow_plan.go` excluded; in Node, run `fiFhirPreview` on each of the six
demo samples with the `adt-http` golden profile; compare `events` and
`diagnostics` byte-for-byte (after canonical JSON) with the output of the same
inputs through `session.Runner` in a Go test (`TestWASMKernelMatchesSessionRunner`,
table-driven, checked in). Negative control: mutate one PID field and assert
both outputs change identically. Disconfirming search: "Go WASM syscall/js
time.LoadLocation tzdata" (the kernel takes a `timezone`; wasm has no zoneinfo
unless `time/tzdata` is imported — embed it and count it in the size budget).

**Failure mode if wrong**: the playground shows a *different* engine from the
IDE, which is exactly the "simulated data" dishonesty `.loom/37` retired. If
the parity test cannot be made to pass, the playground ships parse + events
only and says so, and the Bundle tab stays IDE-only.

**Status**: size half PASSED 2026-09-27; parity half not run.

## Appendix A — survey evidence (2026-09-27, condensed)

- **edilint demo**: `cmd/edilint-wasm/main.go:3-7, 16-26, 55-69`;
  `Makefile:21-30`; `.goreleaser.yml:32-48, 66-83`;
  `.github/workflows/release.yml:3-5, 81-83`; flexinfer-site
  `package.json:11-12`, `scripts/fetch-edilint-release.mjs:13-16, 31-32,
  127-154, 177-271`, `public/wasm/edilint.json:3` (v0.4.1 shipped),
  `components/playground/edilint/useEdilint.ts:136-137, 158-174, 204-259`,
  `lib/security-headers.ts:51-86`, `middleware.ts:43-57`; hosting
  `k8s/base/ingress.yaml:14-41`, `k8s/overlays/prod/cody-ingress.yaml:11-43`,
  `k8s/base/deployment.yaml:29-30, 106-112, 140-142`, `k8s/base/hpa.yaml:13-21`;
  gitops `k3s/flux/apps/flexinfer-site.yaml:1-28`,
  `docs/security/external-services.md:13-19, 23-34, 52-53`,
  `k3s/net/cloudflared.yaml:45-51` (SOPS), `k3s/external-dns/helmrelease.yaml:37-45`.
- **Portfolio lists**: `data/projects.config.json:96-153` → `scripts/gen-projects.mjs`
  → `data/projects.generated.json`, types `data/projects.ts:8-36`, buttons
  `lib/projects/actions.ts:5, 26-62`; hand-written duplicates
  `app/portfolio/page.tsx:41-140`, `components/playground/hub/tools.ts:94-108`,
  `components/playground/shared/PlaygroundSectionNav.tsx:71-74`,
  `app/home/CodyHome.tsx:32-52`, `components/systems/hub/hub-data.ts:354-363`,
  `components/products/product-evidence.ts:187-203`, `app/llms.txt/route.ts:39-41`,
  `app/sitemap.ts:146-182`. codyblevins.com is the same app (`lib/site.ts:16-57`,
  `overlays/prod/cody-deployment.yaml:43-55`). kennedi-consulting-site: no mentions.
- **Docs integration**: `scripts/sync-docs.mjs:38-44, 47-116, 146-160`
  (fi-fhir at :71-88, source `../../libs/fi-fhir/docs` or `FI_FHIR_DOCS_SOURCE`),
  `package.json:42-47` (manual), `AGENTS.md:133-180`, `lib/project-docs.ts:471-475`,
  `components/docs/doc-sets.ts:23-80`, `content/fi-fhir-docs/` (last sync
  `f708464`); fi-fhir `AGENTS.md:600-607`; existing playground
  `app/playground/fi-fhir/README.md:1-46`, `components/playground/fi-fhir/hl7v2-parse.ts:1-7`,
  `scripts/gen-fi-fhir-fixtures.mjs`; edilint docs on GitHub Pages
  (`.github/workflows/pages.yml:1-44`), absent from the site.
- **fi-fhir production**: gitops `k3s/fi-fhir/fi-fhir-api.yaml:49-57, 73-219,
  220-274`, `fi-fhir-ui.yaml:33, 45-51`, `ingress.yaml:12-53` (no rate limits),
  `postgres.yaml:40-56, 104, 200-220` (five DBs, LAN LB), `backup/cronjob.yaml:7, 53-54`,
  `namespace.yaml:11`, image automation `k3s/flux/image-automation/fi-fhir-*.yaml`
  (`path: ./k3s/fi-fhir` only), Access audience `fi-fhir-api.yaml:113-114`,
  `FI_FHIR_FHIR_SERVER_URL` set but unread (code reads `FI_FHIR_FHIR_BASE_URL`,
  `pkg/config/config.go:510`); `k3s/security-posture/namespace-classification.yaml:14`
  classifies a `fi-fhir-dev` namespace that does not exist.
- **e2e stacks (the demo recipe)**: `ui/e2e/run.sh:9-57, 109-114, 123-186`,
  `ui/e2e/ci.sh:41-43`, `ci/test-ui-e2e.yml:51-73`,
  `testdata/golden/integration/adt-http/docker-compose.yaml` (preview-only
  identity, tmpfs Postgres); stacks pass `--no-playground` but **not**
  `--no-introspection`.
- **Safety controls**: auth `cmd/fi-fhir/preview_runtime.go:771-809, 856-879,
  903, 923-934`; trusted CIDRs `internal/api/requestsecurity/trusted_network.go:23, 95-116`;
  transport gate `operation_authorization_roles.go:96-137, 139-322` (~105
  fields behind `graphql:operator`; preview allowlist
  `operation_authorization.go:183-210`); limits `server.go:134-142, 190-192,
  246-264, 313-341, 450-458, 634-641, 682-755, 807-827`; nginx
  `ui/nginx/default.conf.template:25-63` (no CSP); PHI
  `session/postgres.go:365-383`, `PHI-RETENTION.md:214-236`,
  `retention/policy.go:76-82` (purge never deletes sessions/connections/profiles/workflows).
- **Multi-tenancy**: legacy tables untenanted (`store/postgres_event_store.go:35-50`,
  `store/profile_store.go:193-205`, `store/workflow_lifecycle_pg_store.go:113-160`);
  registry tenant must match (`preview_runtime.go:142-144`) → a demo needs its own DB.
- **Health/version**: `internal/observability/health.go:46-51`, `.gitlab-ci.yml:242-246`
  (!238), `ui/src/lib/features/system/SystemStatusPanel.svelte:42-53`,
  `ui/src/lib/ui/ide/StatusBar.svelte:32-33`.
- **WASM kill-test logs**: session scratchpad `research/wasm-kill-test.md`.
