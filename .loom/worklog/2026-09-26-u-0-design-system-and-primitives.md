### 2026-09-26 - U-0: design system and primitives

- What changed:
  - **Tokens** (`ui/src/lib/styles/tokens.css`) rewritten dark-first to the
    `.loom/37` direction: neutral greys (base `#141518`, elevated `#1b1d21`,
    borders white 8–12 %), one indigo accent, status colours for state only,
    radius 4/6 px, `--shadow-sm/md` resolve to nothing, no gradients or glows.
    Every existing token name kept (only `--color-brand-gradient-*` and
    `--color-primary-glow` removed, every consumer made flat); legacy type,
    control, radius and panel scales compressed onto the new steps so
    untouched components read denser. New: `--font-ui`, `--text-ui/label/
    mono/title`, `--tracking-label`, `--size-control-sm/md`,
    `--header/toolbar/statusbar-height`, `--color-accent-text`,
    `--color-focus-ring`.
  - **Theme**: dark is the default preference; `theme.ts` and the `app.html`
    early script always write the resolved theme to `data-theme`, so the CSS
    is `:root` plus `[data-theme="light"]`. `base.css`: 13 px Inter / 1.45,
    accent focus ring, neutral scrollbars; Geist only on the wordmark.
  - **Primitives** (`ui/src/lib/ui/primitives/`, Svelte 5 runes): Button,
    IconButton, Tabs, Badge, Panel, Toolbar, Table/Th/Td/Tr, Field/Input/
    Select/Textarea, EmptyState, KeyValue, Popover, Icon; icons from
    `@lucide/svelte` 1.48.0 (exact).
  - **`/design` gallery**, dev only (production: `load` redirects to `/`,
    the route node is 536 bytes). **`ui/docs/DESIGN.md`** with rules, token
    tables, primitive usage and migration notes for U-1..U-3.
  - Dev server proxies `/api/auth/status` like nginx does.
- Why: `.loom/37` U-0 — the foundation U-1..U-3 stack on.
- Evidence:
  - `npx vitest run`: 864 passed / 3 skipped (baseline 800); 64 new
    (61 primitives incl. a legacy-syntax interop fixture, 3 theme).
    `npm run check` 0 errors; eslint, stylelint, tsc clean; prod build OK.
  - Screenshots (1440×900, local API + dev server) in MR !233: gallery dark
    and light, before/after `/` and `/hl7`.
- What's next: coordinator review of !233; U-1..U-3 stack on the branch.
- Findings for other lanes:
  - `lucide-svelte` is deprecated upstream; use `@lucide/svelte` and deep
    imports (`@lucide/svelte/icons/<name>`).
  - Local screenshot recipe: the API bearer token must be ≥ 24 bytes, and the
    Vite proxy reaches the API over `::1`, so trust `127.0.0.1/32,::1/128`;
    once authenticated wait on `load`, not `networkidle`.
  - Runes primitives take `onclick`, not `on:click`, from legacy parents.
- Sources:
  - [S1] `.loom/37-ide-design-uplift-execution-specs.md` (U-0, design direction)
  - [S2] `ui/docs/DESIGN.md`
