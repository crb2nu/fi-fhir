# Product Spec + Plan: Professional, Low-Noise UX for the fi-fhir IDE

**Status**: Draft — 2026-06-01
**Owner**: UI
**Scope decision**: Full visual overhaul (deliberate clinical aesthetic)
**Naming decision**: Domain terms primary, journey metaphor as subtitle/tooltip
**Theme decision**: Dark-first canonical theme (light retained, secondary)

---

## 1. Problem

fi-fhir ships a VS Code-style IDE (SvelteKit, 100 `.svelte` components) for clinical
data integration: HL7/CDA intake, profile normalization, terminology translation,
workflow routing, and event verification. The shell and design system are mature, but
the surface reads as a **consumer SaaS landing page**, not a focused tool for healthcare
integrators. The dominant failure mode is **too many things competing for attention at
once**, plus **drift away from the design tokens** that already exist.

### Evidence (current state, 2026-06-01)

| Symptom | Measure | Source |
|---|---|---|
| Raw hex colors bypassing tokens | **57** in `.svelte` style blocks | `grep '#[0-9a-fA-F]{6}'` across `ui/src/lib`+`routes` |
| Inline `style=`/`style:` attributes | **69** | grep across `ui/src` |
| Ad-hoc `.pill`/`.chip`/`.tag` classes (re-implementing `Badge`) | **92** | grep across `ui/src` |
| Toast call-sites | **120** | grep `toasts.*(add/success/error/...)` |
| Emoji / playful glyphs in clinical UI | 3 files | [WarningList.svelte](../ui/src/lib/ui/WarningList.svelte), [CopilotPanel.svelte](../ui/src/lib/features/copilot/CopilotPanel.svelte), [WorkflowBuilder.svelte](../ui/src/lib/features/workflows/components/WorkflowBuilder.svelte) |
| Infinite decorative animations | `pulseGlow` on status dots, glow shadows | [+page.svelte:792](../ui/src/routes/+page.svelte#L792), [StatusBar.svelte:104](../ui/src/lib/ui/ide/StatusBar.svelte#L104) |
| Decorative multi-layer gradients on hero | radial+linear stack | [+page.svelte:446](../ui/src/routes/+page.svelte#L446) |
| Metaphor labels obscuring function | Mission Control / Source Intake / Delivery / Verification / Normalization / Translation | [ActivityBar.svelte:24-49](../ui/src/lib/ui/ide/ActivityBar.svelte#L24) |
| Dashboard density | 6+ stacked widget sections, ~70 elements on load | [+page.svelte:228-362](../ui/src/routes/+page.svelte#L228) |

**Key insight**: the token system in [tokens.css](../ui/src/lib/styles/tokens.css) is
already comprehensive (semantic colors, spacing scale, type scale, confidence gradients,
IDE-layout tokens, light+dark). The work is **enforcement + recalibration + restructure**,
not greenfield design.

---

## 2. North Star

> A calm, clinical instrument. Color carries meaning, never decoration. Motion is
> functional and brief. Hierarchy comes from typography and spacing, not from badges,
> glows, and accent fills. Density appears only where the work is dense (editors, tables,
> diff views). The default screen is quiet; signal earns attention.

Reference point: a deployment console / observability tool (Vercel, Linear, Grafana
panels), **not** a marketing site.

### Design principles

1. **Color = state, not flair.** One restrained accent for interactive/primary; the
   semantic quartet (success/warning/danger/info) only for genuine state. Neutral-forward
   surfaces. No brand gradient on chrome.
2. **Motion is functional.** Allowed: ≤150ms transitions on hover/focus/enter. Banned:
   infinite loops (`pulseGlow`, decorative `bounce`/`shimmer`/`pulse`). Always honor
   `prefers-reduced-motion`.
3. **Elevation via border + faint shadow**, not glow. Recalibrate shadow alphas (current
   `--shadow-lg` = 0.35 is too heavy for a light-first clinical UI).
4. **Typography does the hierarchy work.** Weight + size + spacing; reserve color for state.
5. **Progressive disclosure.** Show what needs the operator now; tuck detail behind intent.
6. **Accessibility is professionalism.** AA contrast (audit muted text at 0.48 alpha),
   visible focus, state never signaled by color alone.

---

## 3. Riskiest assumption + kill-test

**Load-bearing assumption**: A *token-first* overhaul is tractable — recalibrating
token values in `tokens.css` (accent, shadows, removing glow tokens) and disabling
decorative keyframes will propagate to **most** of the 100 components, so the remaining
work is a bounded migration of known offenders (57 hex + 92 ad-hoc classes), not a
component-by-component rewrite.

**Kill test** (≤30 min, run as Slice 0):
1. In a scratch branch, change `--color-primary` to a clearly different hue, set
   `--shadow-glow-primary`/`--shadow-glow-success` to `none`, and comment out the
   `pulseGlow` usages.
2. `cd ui && npm run build`, run the app, capture screenshots of `/` (dashboard),
   `/profiles`, `/hl7`, `/terminology` via the `browserkit` screenshot MCP.
3. Count surfaces that **visibly fail to update** (still old indigo, still glowing/pulsing).

**Observable outcome**: ratio of "updated centrally" vs "needs hand editing."
- If ≤~30% need hand-editing → token-first plan holds; proceed as sliced below.
- If >~30% → re-scope: the migration surface is the dominant cost, front-load
  Slice 1 (primitives + token enforcement) before any recolor.

**Failure mode if wrong**: we recolor tokens, declare victory, but a third of the UI
still renders the old loud palette/glow because it hardcodes hex — shipping an
*inconsistent* product that looks half-migrated, which is worse than today.

**Status**: **passed 2026-06-01.** Token-first overhaul is tractable — proceed as sliced.

*Method*: static census + live dark-theme screenshots on throwaway branch
`scratch/slice0-killtest` (edits reverted after). Recolored `--color-primary`
(`#6366f1` → `#ec4899` pink), set `--shadow-glow-*: none`, disabled the two `pulseGlow`
call-sites, pinned `data-theme="dark"`; ran `vite dev` and captured `/`, `/profiles`,
`/hl7`, `/terminology` via headless Chrome.

*Positive evidence (propagation held)*: of **46** accent-bearing `.svelte` files, **44**
reference `var(--color-primary)` and repainted from the single token edit. Visually, the
status bar, active editor tabs, the active ActivityBar icon, the pipeline breadcrumb links,
and the primary-action cards all turned pink on **all four** routes.

*Negative/disconfirming evidence (residual hand-edit surface)*: **10 files (21.7% of
accent-bearing files; 39 hardcoded occurrences)** keep the old hue and need hand-editing —
e.g. `routes/+page.svelte` `stageColors.normalization = '#8b5cf6'` kept the "Normalization
ACTIVE" stage card violet. The `fi-fhir` logo also stayed violet, but that is the *separate*
`--color-brand-gradient-*` token, intentionally out of scope for the accent recolor (§5.1).
The 10: `collaboration/TaskPanel`, `copilot/CopilotPanel`, `dashboard/DashboardStats`,
`system/SystemStatusPanel`, `ui/QualityBadge`, `ui/WarningList`, `ui/ide/DocumentHost`,
`ui/ide/EditorTabs`, `ui/ide/panels/ProblemsPanel`, `routes/+page.svelte`.

*Verdict*: **21.7% < 30% threshold → token-first plan holds.** The residual is a bounded,
enumerated migration (it *is* the Slice 1 offender list), not a component-by-component
rewrite — exactly the bounded-cleanup outcome §3 predicted. Caveat: **glow** removal is
messier than the accent recolor — ~half of glow surfaces hardcode the `box-shadow` rather
than using `var(--shadow-glow-*)` — but it is still a small, known set (7 hardcoded glows +
2 `pulseGlow`), so Slice 2 should treat glow as its own explicit sweep, not assume the token
flip covers it. Evidence screenshots (ephemeral): `/tmp/fi-kt-{dashboard,profiles,hl7,terminology}.png`.

---

## 4. Scope

### In scope
- Visual language: color usage, elevation, motion, typography hierarchy, spacing rhythm.
- Token enforcement: eliminate raw hex / ad-hoc badge classes; add lint guardrail.
- Shell chrome: header, ActivityBar (rename), StatusBar, EditorTabs, BottomPanel.
- Dashboard (`/`) information architecture + progressive disclosure.
- Per-feature page migration to shared primitives + tokens.
- Notification policy (toast budget).
- Accessibility pass (contrast, focus, reduced-motion, non-color state).

### Out of scope (this effort)
- Backend/GraphQL/OpenAPI contracts and data flow.
- New features or route additions.
- Editor engine internals (CodeMirror/Monaco behavior) beyond theming tokens.

---

## 5. Workstreams & decisions

### 5.1 Color & elevation recalibration
- Demote the indigo→violet brand gradient (`--color-brand-gradient-*`) to brand-mark only;
  remove from chrome/surfaces. Keep **one** primary accent.
- Reserve semantic colors strictly for state. Audit dashboard stat-card gradients
  ([DashboardStats.svelte](../ui/src/lib/features/dashboard/DashboardStats.svelte)) → flat
  surfaces.
- Replace `--shadow-glow-*` usage with border + `--shadow-sm/md`.
- **Theme: dark-first** (decided). Recalibrate and tune the token palette against the dark
  surfaces first; treat light (`[data-theme="light"]`) as a maintained secondary. Shadow
  alphas, border contrast, and the accent hue must be chosen to read on `--color-bg-base`
  `#020617`, with light verified after.

### 5.2 Motion policy
- Delete `pulseGlow` and decorative use of `pulse`/`bounce`/`shimmer` keyframes
  ([tokens.css:382-418](../ui/src/lib/styles/tokens.css#L382)). Keep `fadeIn`/`slideIn`/
  `scaleIn`/`spin`(loaders only) as ≤250ms one-shots.
- Add a global `@media (prefers-reduced-motion: reduce)` block disabling non-essential motion.

### 5.3 Component primitives (the enforcement layer)
- One canonical `StatusPill`/`Badge` ([Badge.svelte](../ui/src/lib/ui/Badge.svelte)) — migrate
  all 92 ad-hoc instances; collapse redundant pills (e.g. ProfilesPage's triple "unsaved"
  → single "N unsaved changes" at [ProfilesPage.svelte:178](../ui/src/lib/features/profiles/ProfilesPage.svelte#L178)).
- Standardize page scaffolding through [PageHeader](../ui/src/lib/ui/PageHeader.svelte),
  [Panel](../ui/src/lib/ui/Panel.svelte), [EmptyState](../ui/src/lib/ui/EmptyState.svelte).
- Replace all 57 raw hex with tokens; convert data-driven inline `style=` to a bound CSS
  custom property pattern (`style="--v: {x}"` + class), eliminate the rest.
- **Guardrail**: add a stylelint rule forbidding raw hex/rgb and bare colors in `.svelte`
  `<style>` blocks, wired into `npm run check` and CI `lint` stage. This is what stops the
  drift from recurring.

### 5.4 Shell & navigation
- ActivityBar: domain term primary, metaphor as subtitle/tooltip.

  | Current (metaphor) | Primary label | Subtitle/tooltip | Route |
  |---|---|---|---|
  | Source Intake | HL7 / Intake | Source Intake | `/hl7` |
  | Delivery | Workflows | Delivery | `/workflows` |
  | Verification | Events | _(none)_ | `/events` |
  | Normalization | Profiles | Normalization | `/profiles` |
  | Translation | Terminology | Translation | `/terminology` |
  | Mission Control | Dashboard | Mission Control | `/` |

  **Subtitle principle**: the metaphor subtitle appears only where it adds meaning the
  literal term lacks. **Events stands alone** — "Events" is already the precise domain noun
  (the page lists parsed events); "Verification" describes the *reason* to look (confirm we
  parsed events from source per profile), which is workflow rationale, not a better label.
  All other rows keep the metaphor as subtitle/tooltip for identity continuity.
- Header: drop `text-gradient` brand treatment if it conflicts with the calmer palette.
- StatusBar: solid state dots, no glow; ensure state has text/icon, not color alone.

### 5.5 Dashboard restructure (`/`)
- Define a 3-tier hierarchy:
  1. **Now** — what needs the operator (active investigations, blocking alerts, unsaved work).
  2. **Health at a glance** — compact stage-health + key metrics, no pulsing.
  3. **On demand** — telemetry, recent events, unmapped codes behind tabs/disclosure.
- Remove hero gradients; replace with a quiet header + clear primary action.
- Collapse the 6+ always-on sections; target ~⅓ the on-load element count.

### 5.6 Notification policy
- Author a toast budget: toasts only for **transient confirmations** and **async results**.
  Persistent state → Problems/StatusBar/inline, not toasts.
- Audit all 120 call-sites; categorize (confirm / error / status-that-should-be-inline) and cut.

### 5.7 Typography
- **Heading font: replace "Outfit" with Geist Sans** (decided). Outfit's rounded geometric
  voice reads consumer/marketing and fights the clinical North Star; Geist is a neutral
  grotesk purpose-built for technical surfaces and tuned for dark backgrounds (our canonical
  theme). Update `--font-heading` at [tokens.css:97](../ui/src/lib/styles/tokens.css#L97).
- Self-host the variable font (SIL OFL) — no CDN/licensing dependency, important for
  locked-down/air-gapped healthcare deployments. Add the `@font-face` (and `font-display: swap`)
  alongside Inter.
- Body stays **Inter**. Optionally adopt **Geist Mono** for `--font-mono` to unify the system;
  defer unless it falls out cheaply.
- Verify heading rendering against `--color-bg-base` `#020617` at the type-scale sizes
  ([tokens.css:102-108](../ui/src/lib/styles/tokens.css#L102)); hierarchy comes from weight +
  size + tracking, not color.

### 5.8 Accessibility
- Contrast audit of muted text tokens (`--color-text-muted` 0.48 alpha) against surfaces; raise
  where below AA.
- Visible `:focus-visible` on every interactive element (token `--shadow-focus` exists — apply
  consistently).
- Verify no state is color-only.

---

## 6. Delivery slices

Each slice keeps the existing test suite green and ships behind its own MR. Visual changes are
verified with `browserkit` screenshots before/after.

| Slice | Title | Content | Exit criterion |
|---|---|---|---|
| **0** | Kill-test + foundations | Run §3 kill-test; add `prefers-reduced-motion` block; add stylelint hex/color rule (report-only first). | Kill-test result recorded; lint rule reports the 57 offenders. |
| **1** | Primitives + token enforcement | Migrate 57 hex → tokens; collapse 92 ad-hoc badges into `Badge`/`StatusPill`; flip stylelint rule to error in CI. | hex count = 0; ad-hoc badge classes = 0; CI blocks regressions. |
| **2** | Color + motion + type recalibration | Recalibrate accent/shadows; remove glow tokens & `pulseGlow`; flatten gradient surfaces; swap heading font Outfit → self-hosted Geist Sans (§5.7). | 0 infinite animations; glow removed; `--font-heading` = Geist, self-hosted; screenshots show calmer chrome. |
| **3** | Shell & nav | ActivityBar rename (domain+subtitle); header/StatusBar/tabs cleanup. | Nav labels lead with function; tests updated for new labels. |
| **4** | Dashboard restructure | 3-tier hierarchy + progressive disclosure on `/`. | On-load element count ↓ ~⅓; primary action obvious. |
| **5** | Feature-page migration | profiles → hl7 → terminology → workflows → events: PageHeader/Panel/EmptyState; remove emoji/inline styles. | Each page uses shared scaffolding; 69 inline styles → near-0. |
| **6** | Notification + a11y | Toast budget applied; contrast/focus/reduced-motion audit. | Toast call-sites categorized & reduced; AA contrast verified. |

Sequencing rationale: enforcement (1) precedes recalibration (2) so token changes actually
propagate (this is the kill-test's whole point). Shell/dashboard (3-4) are the most visible
"professional" wins. Per-page migration (5) is the long tail. Policy/a11y (6) hardens.

---

## 7. Success metrics

| Metric | Baseline (2026-06-01) | Target |
|---|---|---|
| Raw hex in `.svelte` style blocks | 57 | 0 (CI-enforced) |
| Ad-hoc `.pill`/`.chip`/`.tag` classes | 92 | 0 |
| Inline `style=` attributes | 69 | data-driven only (<10) |
| Infinite/decorative animations | several | 0 |
| Toast call-sites | 120 | categorized; transient-only |
| Emoji in clinical surfaces | 3 files | 0 |
| Dashboard on-load elements | ~70 | ~⅓ reduction |
| WCAG AA contrast on body/muted text | unverified | pass |
| `prefers-reduced-motion` honored | no | yes |

---

## 8. Risks

- **Token drift recurs** without the stylelint guardrail (Slice 1) — enforcement is the
  durability mechanism, not a nice-to-have.
- **Nav rename breaks tests/muscle memory** — `ActivityBar.test.ts` and any e2e asserting
  labels must update; keep metaphor discoverable as subtitle to soften the change.
- **"Full overhaul" scope creep** — slices are independently shippable; resist coupling.

## 9. Open questions

- ~~Light-first or dark-first canonical theme?~~ **Resolved: dark-first** (light secondary).
- ~~Final ActivityBar wording?~~ **Resolved**: §5.4 mapping accepted; Events stands alone
  (no subtitle).
- ~~Is "Outfit" the right heading voice?~~ **Resolved: replace with Geist Sans**
  (self-hosted variable font); body stays Inter. See §5.7.
