# IDE design uplift — execution specs (2026-09-26)

Brief (Cody, 2026-09-26, after the IDE repair program shipped and verified):
"the UI/UX still looks rough / like a toy so we need to get back to work on
that." The repair program (`.loom/36`) made every surface *honest*; this
program makes the product look and feel like the professional integration
engine it is. Coordinator designs and reviews; Opus lanes implement and open
MRs for review; the coordinator arms.

## What "toy" means here (evidence, 1440×900, production, 2026-09-26 09:20 EDT)

Screenshots of `/`, `/hl7`, `/operator`, `/events`, `/workflows` on
`fi-fhir.flexinfer.ai` (UI `v0.1.29229`). The same five defects appear on
every route:

1. **Four layers of "where am I" before any content.** Top to bottom: the
   credential access strip ("Trusted network access active — Connected from
   the deployment trusted network"), the journey band ("STAGE 1 OF 5 / Source
   Intake / Load inbound interfaces…", five stage cards with ACTIVE/NEXT/DONE
   badges, a "NEXT UP → Continue to Normalization" card), the page hero
   (`<h1>` ≈ 28 px + subtitle), then a numbered stepper pill row ("01 RAW
   SOURCE · 02 WARNINGS + EXTRACTION · 03 PROFILE + PROCESS") and, on intake,
   a four-card flow diagram (SOURCE → SOURCE PROFILE → WORKFLOW →
   DESTINATION). On `/hl7` at 900 px tall the editor gets about 100 px.
2. **Marketing register in an operator tool.** "Build the interface from
   source to destination", "Mission control", "RECOMMENDED MOVE — Start
   Source Intake", explainer cards inside pages ("DOWNSTREAM VERIFICATION /
   Event browser / Use this view to confirm what arrived…" takes a third of
   the Events page), "Continue to Normalization". These are onboarding copy
   shown permanently to the one operator who wrote the system.
3. **Demo data on the production home.** "Active Alerts — DEMO DATA:
   HighErrorRate, DLQBacklog" (`AlertsPanel.svelte`, `observabilityStore`
   falls back to simulated alerts when the loom platform is not connected).
   Labelled or not, invented alerts on a health dashboard read as a toy.
4. **Visual language.** Navy-to-purple gradient background (`app.html`
   `#0b1220`, indigo/violet brand gradient tokens), glows
   (`--color-primary-glow`), a solid purple status bar, pill badges in
   green/purple everywhere ("4 segments", "0 warnings", "handoff ready",
   "1 events", "Delivered"), large rounded cards with shadows for everything
   including single form fields, illustration-sized empty-state icons, two
   heading fonts (Geist for headings, Inter body), the Copilot tab's "✦".
5. **Density.** Lists are stacks of cards, not tables (events, receipts,
   deliveries); inputs and buttons are ~40 px tall; panel padding 24 px;
   tabs are pill buttons; identifiers wrap instead of truncating in a
   monospace column. Nothing is keyboard-dense.

Facts about the code: 107 Svelte components, 105 with scoped `<style>`;
tokens exist (`ui/src/lib/styles/tokens.css`, 490 lines, light-first with
dark overrides, 4 px spacing scale, `--text-*` from 10.4 px to 24 px) and a
`base.css`; the shell is `IDEShell.svelte` (header → `JourneyProgress`
(538 lines) → `ActivityBar` + `EditorTabs` + `SplitPane`/`DocumentHost` +
`BottomPanel` + `Sidebar` + `StatusBar`); the access strip is rendered by
`GraphQLCredentialGate.svelte` above the shell; routes are `/`, `/events`,
`/hl7`, `/operator`, `/profiles`, `/terminology`, `/workflows`; the browser
smoke gate (`test:ui-e2e`, Lane R-D) already runs Chromium against the built
UI and the real API, and is the natural home for visual evidence.

## Design direction: calm, dense, precise

The reference register is VS Code, Linear, Grafana and Mirth Connect's
administrator: a dark, neutral workbench where colour means state, type is
small and consistent, chrome is thin, and data is in tables. Rules every
lane applies (U-0 encodes them in tokens, primitives and `ui/docs/DESIGN.md`):

- **Surfaces**: neutral greys, not navy/purple. Base `#141518`-class
  background, elevated `#1b1d21`, borders `1px` at 8–12 % white, radius
  4 px (6 px for popovers), no shadows on in-layout panels, no gradients,
  no glows. Light theme keeps parity through the same token names.
- **One accent**, indigo, used only for focus rings, selection, the primary
  action and the active tab underline. Status colours (success/warning/
  danger/info) only ever describe state. The status bar is a neutral
  32 px strip with a coloured dot, never a solid accent bar.
- **Type**: Inter only (Geist stays for the wordmark). UI base 13 px /
  line-height 1.45; secondary 12 px; labels and table headers 11 px
  uppercase with 0.04 em tracking; page titles 15 px semibold in a toolbar,
  never an `<h1>` hero; monospace 12 px only for identifiers, code and
  numbers. Delete `--text-2xl` hero usage from pages.
- **Spacing and size**: 4 px grid. Toolbars 36 px, table rows 28–32 px,
  inputs/buttons/selects 28 px (32 px for primary actions in dialogs), panel
  padding 8–12 px, section gaps 12–16 px. Icons 16 px from one set
  (`lucide-svelte`); no emoji, no "✦".
- **Page pattern**: every route = toolbar (title · underline tabs · actions,
  right-aligned) + content region. No subtitles, no explainer cards, no
  "recommended move" cards; contextual help is a `?` icon button opening a
  popover. Empty states: one sentence, one primary action, a 16 px icon.
- **Data**: lists of records are tables (sticky header, sortable where the
  API sorts, monospace ids truncated with a title, right-aligned counts,
  row hover, selected row = accent-left-border), with a details pane to the
  right, not a stack of cards.
- **Copy register**: system voice, no exclamation marks, no journey
  metaphors. Stage names stay as navigation labels (Source Intake,
  Normalization, Translation, Delivery, Verification) because the sidebar
  and the roadmap use them; the *band* goes.
- **Chrome**: header 40 px (wordmark · stage breadcrumb `Intake ▸ HL7` ·
  command palette · connection chip). The access strip becomes a status-bar
  chip ("Trusted network", "Cloudflare Access: name", "Bearer") with a
  popover holding today's text and the "Clear access" action. The journey
  band becomes a compact segmented control in the header (five 24 px
  segments, current filled, done ticked) and a `Next: …` item in the status
  bar; `JourneyProgress.svelte` is retired.
- **Honesty stays**: every `data-testid` from `.loom/36` (operator-preflight,
  streaming-unavailable, copilot-llm-state, problems-badge,
  platform-indicator) keeps its meaning; the honest states are restyled,
  never removed. `test:ui-e2e` must stay green on every MR.

## Lanes

Merge order: **U-0 → (U-1, U-2, U-3 in parallel, stacked on U-0's branch) →
U-4 last**. Each MR carries before/after screenshots (see Evidence).

### U-0 — Design system and primitives (first, alone)

**Outcome.** `tokens.css` rewritten dark-first to the rules above (keep every
existing token *name* so untouched components keep rendering; add
`--size-control-sm/md`, `--radius-sm/md`, `--font-ui`, `--text-ui`,
`--text-label`, table tokens; light theme via the same names under
`[data-theme="light"]`); `base.css` sets body 13 px Inter and neutral
scrollbars; `app.html` background matches the base token; a primitives
package `ui/src/lib/ui/primitives/` with `Button` (primary/secondary/ghost/
danger, sm/md, icon-only), `IconButton`, `Tabs` (underline, keyboard),
`Badge` (subtle, mono counts), `Panel` (flat, header slot), `Toolbar`,
`Table` (+ `Th/Td` helpers, sticky header, selection), `Field`/`Input`/
`Select`/`Textarea` (28 px), `EmptyState`, `KeyValue`, `Popover`, `Icon`
(lucide re-export with 16 px default) — each with a small vitest and
`data-testid` passthrough; `ui/docs/DESIGN.md` with the rules and a component
inventory; a `/design` route **behind `import.meta.env.DEV`** that renders the
primitives gallery for screenshots. Adds `lucide-svelte` (exact pin).
Removes the brand gradient/glow tokens and the purple status-bar colour.
**Not in scope**: restyling pages (U-1..U-3) beyond what the token rewrite
changes automatically. **Acceptance**: `npx vitest run` green, `npm run
check` 0 errors, `test:ui-e2e` green, gallery screenshot attached.

### U-1 — Shell

**Outcome.** `IDEShell.svelte` header at 40 px with wordmark, stage
breadcrumb segmented control (replaces `JourneyProgress`, whose tests move
to the new control), command palette, connection chip; `GraphQLCredentialGate`
renders no strip when authenticated — the state moves to a `StatusBar` chip
with popover (bearer entry UI when unauthenticated stays a centered dialog
styled with primitives); `ActivityBar` 40 px wide with 16 px icons and
tooltips; `EditorTabs` 32 px with underline active state; `BottomPanel` tabs
as `Tabs`, 28 px header, the Copilot tab loses "✦"; `StatusBar` 24 px
neutral with dots; `Sidebar` sections use `Panel`. Keyboard: `Cmd/Ctrl+K`,
`Cmd/Ctrl+B` (sidebar), `Cmd/Ctrl+J` (panel) documented in the palette.
**Acceptance**: every route renders inside the new chrome; `problems-badge`
and `platform-indicator` semantics unchanged (tests updated for markup only);
e2e green.

### U-2 — Home, Events, Operator

**Outcome.** `/` becomes an operational home: toolbar "Home"; "Recent"
(documents/sessions from existing stores, real data or an empty state);
"Integrations" (from the registry/lifecycle queries that already exist);
"Health" using only real sources — the alerts panel shows "No alert source
configured" when the observability platform is not connected and **never
simulated data** (delete the demo fallback from `observabilityStore` and
`AlertsPanel`; keep the `isSimulated` concept only if a test double needs
it). `/events`: toolbar with underline tabs (Browse · Live Stream · Patient
Timeline · Statistics); Browse = filters row (28 px controls) + `Table`
(time, type, source, ids, status) + details pane; Live Stream keeps
`streaming-unavailable` restyled as an `EmptyState`. `/operator`: toolbar
tabs Messages · Delivery · Deployments; Messages = filters + receipts `Table`
+ trace pane; Delivery and Deployments as tables with the reason-required
action dialogs restyled; `operator-preflight` restyled as an `EmptyState`
with the role names in mono. **Acceptance**: e2e checks 2, 5, 6a, 6b green
unchanged in meaning; screenshots.

### U-3 — Intake, Workflows, Profiles, Terminology

**Outcome.** `/hl7`: toolbar (title "HL7 intake", actions Load file ·
Preview · Process, redaction as a `Select`); the flow diagram becomes one
"pipeline" chip row (source · profile · workflow · destination, 24 px chips,
click to change); the 01/02/03 stepper is removed (the bottom panel and the
Results pane already carry that state); editor-first layout: editor ≥ 50 %
of the document height with the Results pane in a resizable split;
`SessionStreamNotice`/run progression restyled as a compact status line.
`/workflows`: toolbar tabs Inventory · Design · Verification; the "01 SOURCE
MAPPING ▸ 02 SHAPE DESTINATION ▸ 03 VERIFY HANDOFF" stepper is removed; the
builder form uses `Field`s on a 2-column grid at 28 px; Dry Run and Monitor
panels use `Panel`/`Table`. `/profiles` and `/terminology`: toolbar + table
+ details, same rules. **Acceptance**: e2e check 3 (session stream on HL7
intake) green; `make ui-e2e` local run attached; screenshots.

### U-4 — Visual evidence in the gate (last)

**Outcome.** A `visual` Playwright project in `ui/e2e/` that, against the
`operator-bundle` stack, captures 1440×900 PNGs of every route plus the
bottom panel open and the command palette, into `ui/e2e-results/visual/`
(artifacts, `expire_in: 2 weeks`), and asserts the copy register: none of
"Mission control", "Build the interface", "Recommended move", "Demo data",
"Continue to", "NEXT UP" appears in the rendered body of any route. It
does **not** do pixel diffing (no golden PNGs in git). `make ui-e2e
UI_E2E_ARGS="--project visual"` mirrors it; `ui/docs/DESIGN.md` gains "How
to review a UI MR" (open the artifact folder). **Acceptance**: the job runs
on every `ui/**` MR inside `test:ui-e2e` (same job, one more project) and
stays under the current 4 GiB limit.

## Evidence every lane attaches

Before/after PNGs at 1440×900 for each route the MR touches, taken with
Playwright against a local stack (`bin/fi-fhir serve` with
`FI_FHIR_GRAPHQL_TRUSTED_CIDRS=127.0.0.1/32`, the operator bundle in
`FI_FHIR_GRAPHQL_ROLES`, `FI_FHIR_OPERATOR_CONTROL_PLANE_ENABLED=true`, a
Postgres on the 7900xtx docker context, sessions on; or `make ui-e2e
UI_E2E_KEEP=1` and `npx playwright screenshot` inside the kept container),
uploaded with `POST /projects/19/uploads` and embedded in the MR
description. The coordinator reviews the screenshots first and the diff
second.

## Shared lane policy (verbatim in every prompt)

- Branch from `origin/main` for U-0; U-1..U-3 branch from `origin/main`
  and merge U-0's branch with `--no-ff` until U-0 merges, then
  `rebase --onto origin/main <stacking-merge>`; U-4 branches from main after
  U-1..U-3 merge. Names: `feat/ide-uplift-0-design-system`,
  `feat/ide-uplift-1-shell`, `feat/ide-uplift-2-home-events-operator`,
  `feat/ide-uplift-3-intake-workflows`, `test/ide-uplift-4-visual-gate`.
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
  pnpm; `npx svelte-kit sync` before vitest.
- **Pipelines**: poll ≤ every 180 s; retry a job once only for a known
  flake (`lint:ui` heap, `build:docker-ui` BuildKit "context canceled",
  "Getting source" curl 56 resets, `test:observability-replicas`,
  `security:trivy-image` DB drift); never cancel-retry `lint:gqlgen`; after
  two failures of different jobs or 3 h on one pipeline, STOP and write
  status. `test:ui-e2e` failures are yours to read (artifacts) — restyling
  must not change what the honest surfaces assert.
- **Never touch** `CHANGELOG.md`, `ROADMAP.md`, `.loom/30-*.md`,
  `.loom/50-worklog.md`, `.loom/40-decisions.md`, `platform/gitops`, Go
  code, `ci/*.yml` (U-4 excepted for `ci/test-ui-e2e.yml`), or another
  lane's files. Worklog via `make worklog-new TITLE="..."`.
- **zsh**: unquoted `$VAR` does not word-split; `noclobber` (`>|`);
  `mv`/`rm`/`cp` are interactive (`command mv -f`); python f-strings cannot
  contain backslashes.
- **PHI/secrets**: synthetic samples only; no production credential anywhere.
- Commit messages conventional, scoped, trailer
  `Co-Authored-By: <the model you are> <noreply@anthropic.com>`.

## Decisions taken by the coordinator

1. **Dark-first neutral workbench with one accent.** The purple/navy
   gradient theme is what reads as a toy; tokens keep their names so the
   migration is incremental and untouched components degrade gracefully.
2. **The journey band and access strip leave the document area.** Stage
   context is navigation, not content; it lives in the header and status
   bar at a fixed 40 + 24 px.
3. **No simulated data in production surfaces, labelled or not.** The
   honest empty state is the product.
4. **Evidence over pixel tests.** Screenshots are review artifacts, not
   golden files; the gate asserts the copy register and the honest
   `data-testid`s, which are stable.
