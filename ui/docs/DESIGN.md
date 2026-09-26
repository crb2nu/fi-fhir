# Design system (fi-fhir IDE)

The working reference for how the IDE looks and reads. The direction comes
from `.loom/37-ide-design-uplift-execution-specs.md`: **calm, dense, precise**,
a dark, neutral workbench in the register of VS Code, Linear, Grafana and
Mirth's administrator console, where colour means state, type is small and
consistent, chrome is thin and data lives in tables.

| What | Where |
|---|---|
| Tokens (single source of values) | `src/lib/styles/tokens.css` |
| Element defaults, focus ring, scrollbars, utilities | `src/lib/styles/base.css` |
| Primitives | `src/lib/ui/primitives/` (`import { Button, … } from '$lib/ui/primitives'`) |
| Theme switch | `src/lib/theme/theme.ts` + the early script in `src/app.html` |
| Live gallery (dev only) | `npm run dev`, then open `/design` (`?theme=light` to preview light) |

## Rules at a glance

1. Surfaces are neutral grey. No navy, no purple, no gradients, no glows.
2. One accent (indigo, `--color-primary`) for focus, selection, the primary
   action and the active tab underline. Nothing else is indigo.
3. Status colours (success, warning, danger, info) describe state only.
4. Inter everywhere. 13 px UI base; 11 px uppercase labels; 15 px page titles
   in a toolbar; monospace 12 px for ids, code and numbers.
5. 4 px grid. Controls 28 px, toolbars 36 px, table rows 30 px, header 40 px,
   status bar 24 px.
6. Every route is a toolbar plus a content region. No hero, subtitle,
   explainer card or "recommended move".
7. Lists of records are tables with a details pane.
8. System voice: plain statements, no exclamation marks, no journey metaphors.

## Theme

Dark is the default. `tokens.css` defines every colour on `:root` (dark) and
overrides the colour tokens only under `:root[data-theme="light"]`.
`theme.ts` always writes the resolved theme to `data-theme`: the "system"
preference writes `light` or `dark` from the OS, so the CSS never needs a
media query. The toggle in the header cycles system, light and dark and
persists the choice under `localStorage["fi-fhir-theme"]`.

To add a colour token, define it in both blocks. To add any other token,
define it once on `:root`. Never hard-code a hex value in a component
`<style>`: `npm run lint:css` blocks raw hex in `.svelte` files.

## Tokens

### Surfaces

| Token | Dark | Light | Use |
|---|---|---|---|
| `--color-bg-base` | `#141518` | `#f6f7f9` | Canvas: documents, editor, tables |
| `--color-bg-elevated` | `#1b1d21` | `#ffffff` | Chrome and panels: header, sidebar, bars, Panel, sticky table header |
| `--color-bg-surface` | white 3.5 % | black 3.5 % | Raised inside a panel (secondary button fill) |
| `--color-bg-input` | `#111215` | `#ffffff` | Inset controls: Input, Select, Textarea |
| `--color-bg-overlay` | `#202227` | `#ffffff` | Popovers, menus, dialogs (opaque; backdrops use `--modal-backdrop`) |
| `--color-bg-hover` / `--color-bg-active` | white 5 % / 8 % | black 5 % / 8 % | Row and control hover; pressed or selected neutral |

### Text

| Token | Dark | Light | Use |
|---|---|---|---|
| `--color-text-primary` | `#e6e7ea` | `#1b1d21` | Body, values, titles |
| `--color-text-secondary` | `#b4b8c0` | `#3d4149` | Secondary copy, ghost controls |
| `--color-text-tertiary` | `#9a9fa9` | `#565b64` | Labels, table headers, keys |
| `--color-text-muted` | `#8a8f99` | `#62666f` | Placeholders, empty values (still ≥ 4.5:1) |
| `--color-text-inverse` | `#ffffff` | `#ffffff` | Text on accent or danger fills |

### Accent, status, borders

| Token | Use |
|---|---|
| `--color-primary` / `-hover` | Primary button fill, active tab underline, selected-row edge |
| `--color-primary-muted` | Selected row and selected option background |
| `--color-accent-text` | Accent-coloured text on a surface (rare: selection counts) |
| `--color-focus-ring` (= `--color-border-focus`) | Keyboard focus ring |
| `--color-{success,warning,danger,info}` | Solid state colour (dots, fills) |
| `--color-{…}-text` / `-bg` / `-border` | State text, subtle badge fill, state outline |
| `--color-border-subtle` / `-default` / `-strong` | 1 px dividers at 8 / 10 / 12 % white in dark |

Legacy names kept as aliases: `--color-*-soft` = `--color-*-text`,
`--color-accent-soft` = `--color-accent-text`, `--palette-*` = the matching
state colour. `--palette-violet-600` is a categorical document colour
waiting to be removed (see the migration notes).

### Elevation

In-layout surfaces use a 1 px border, never a shadow: `--shadow-sm` and
`--shadow-md` resolve to nothing. `--shadow-lg` is for popovers, menus and
tooltips; `--shadow-xl` for dialogs. `--shadow-focus` is a 1 px accent ring
that hugs a focused control's border.

### Type

| Token | Size | Use |
|---|---|---|
| `--text-title` (= `--text-xl`) | 15 px, semibold | Page title in a Toolbar |
| `--text-lg` | 14 px | Dialog titles, emphasis |
| `--text-ui` (= `--text-sm`, `--text-base`) | 13 px | UI base: body, controls, cells |
| `--text-xs` | 12 px | Secondary text, hints, keys |
| `--text-mono` | 12 px | Identifiers, code, numbers (`--font-mono`) |
| `--text-label` (= `--text-2xs`) | 11 px, uppercase, `--tracking-label` 0.04 em | Field labels, table headers, section labels |
| `--text-2xl` | 18 px | Stat figures only. Never a page hero. |

`--font-ui` is Inter; `--font-heading` (Geist) is reserved for the `fi-fhir`
wordmark in the header and status bar. `h1`–`h6` default to Inter
semibold. Line height is `--leading-ui` (1.45). Use the `.text-label`,
`.text-mono` and `.tabular-nums` utilities from `base.css` for one-off
text.

### Spacing and size

| Token | Value | Use |
|---|---|---|
| `--space-1 … --space-16` | 4 px grid | Panel padding 8–12 px, section gaps 12–16 px |
| `--size-control-sm` | 28 px | Default Input, Select, Button |
| `--size-control-md` | 32 px | Primary action of a dialog |
| `--size-icon` | 16 px | Every icon |
| `--header-height` | 40 px | App header |
| `--toolbar-height` | 36 px | Page toolbar |
| `--statusbar-height` | 24 px | Status bar |
| `--table-row-height` / `--table-header-height` | 30 px / 28 px | Table rows and header |
| `--radius-sm` | 4 px | Everything in layout |
| `--radius-md` | 6 px | Popovers, menus, dialogs |

The legacy scales were compressed onto these steps so untouched components
already read denser: `--btn-height-*` and `--input-height-*` are 24/28/32 px,
`--radius-lg` and `--radius-xl` are 4 px, `--radius-2xl` is 6 px, and
`--panel-padding` is 12 px. `--radius-full` is for dots and spinners, not
badges.

## Colour usage

- Accent: focus ring, the one primary button in a view, the active tab
  underline, the selected row's left edge and fill. Not links, headings,
  icons, badges or chart series.
- Status colours describe the state of a record or system (`delivered`,
  `failed`, `2 warnings`, connection dots). They are not decoration or
  categories: a document type or stage gets an icon and a label, not a
  colour.
- Selected rows use `--color-primary-muted` and a 2 px accent left edge;
  hovered rows use `--color-bg-hover`.
- The status bar is a neutral elevated strip with coloured dots. It is
  never an accent-filled bar.

## Page pattern

Every route renders a `Toolbar` (title, underline tabs for the route's views,
right-aligned actions) and then its content. Contextual help is a `?`
`IconButton` opening a `Popover`, never an explainer card.

```svelte
<script lang="ts">
  import CircleHelp from '@lucide/svelte/icons/circle-help';
  import Download from '@lucide/svelte/icons/download';
  import { Button, IconButton, Popover, Tabs, Toolbar } from '$lib/ui/primitives';

  let view = $state('browse');
</script>

<Toolbar title="Events">
  {#snippet tabs()}
    <Tabs label="Event views" bind:value={view} items={[
      { id: 'browse', label: 'Browse' },
      { id: 'live', label: 'Live Stream' }
    ]} />
  {/snippet}
  {#snippet actions()}
    <Popover label="About Events" placement="bottom-end">
      {#snippet trigger(props)}
        <IconButton {...props} icon={CircleHelp} label="About Events" />
      {/snippet}
      Events are read from the event store for the selected tenant.
    </Popover>
    <Button icon={Download}>Export</Button>
  {/snippet}
</Toolbar>
```

Content below the toolbar is a filters row (28 px controls, 8 px gaps) and a
table with a details pane on the right (`KeyValue`), or an editor with a
results pane in a split. Panels sit 12–16 px apart and pad 8–12 px.

## Copy register

- System voice: say what is true and what to do. "No deliveries in the last
  24 hours." not "Nothing here yet! Let's get started."
- No exclamation marks, no journey or mission metaphors, no marketing
  headlines. Stage names (Source Intake, Normalization, Translation,
  Delivery, Verification) are navigation labels only.
- Do not ship these phrases: "Mission control", "Build the interface",
  "Recommended move", "Demo data", "Continue to", "NEXT UP".
- Sentence case for titles and buttons ("Load file", not "Load File").
  Uppercase only through the label register.
- Never show simulated data on a production surface, labelled or not. The
  honest empty state is the product.

## Empty states

`EmptyState`: one sentence, at most one primary action, a 16 px icon. No
illustration, headline or paragraph. Honest states (operator preflight,
streaming unavailable, no alert source) use it and keep their `data-testid`
on the root; role names and identifiers go in `<code>` inside the sentence.

## Icons

One set: Lucide, through `@lucide/svelte` (pinned exactly; `lucide-svelte`
is deprecated upstream in favour of it). Import each glyph by deep path so
dev and tests load one module, not all 1,600:

```svelte
import Play from '@lucide/svelte/icons/play';
<Icon icon={Play} />                   <!-- 16 px, stroke 1.75, aria-hidden -->
<Icon icon={Play} label="Running" />   <!-- meaningful: role="img" + name -->
```

No emoji, no `✦`, no inline hand-drawn SVGs in new code.

## Primitives

Svelte 5 runes components. They work from legacy-syntax parents too
(`__fixtures__/LegacyConsumer.svelte` is the proof), with three rules:

- **Events are props**: `onclick={…}`, `onchange={…}`. `on:click` on a
  primitive does nothing.
- **Regions are snippets**: `{#snippet actions()}…{/snippet}`. Default content
  becomes `children`.
- **Every primitive accepts `class` and `data-testid`** (and other native
  attributes) and forwards them to its root element. Keep honest test ids on
  the primitive that now renders the state.

| Primitive | Use | Key props |
|---|---|---|
| `Button` | Actions | `variant` primary/secondary/ghost/danger · `size` sm/md · `icon` · `iconOnly` · `loading` |
| `IconButton` | Icon-only actions and toggles | `icon` · `label` (required: name + tooltip) · `pressed` · `variant` (ghost) |
| `Tabs` | Views of one route; underline | `items` · `bind:value` · `onchange` · `label` · `activation` auto/manual |
| `Badge` | State or count labels | `tone` neutral/accent/success/warning/danger/info · `mono` · `dot` |
| `Panel` | A bordered region | `title` · `titleTag` · `header`/`actions` snippets · `flush` |
| `Toolbar` | Top of every route | `title` · `titleTag` (h1) · `heading`/`tabs`/`actions` snippets |
| `Table`, `Th`, `Td`, `Tr` | Lists of records | Table `label` · `head` snippet · `layout`; Th `numeric` `width` `sort` `onsort`; Td `mono` `numeric` `truncate` `muted` `value`; Tr `selectable` `selected` `onselect` |
| `Field` + `Input`/`Select`/`Textarea` | Forms | Field `label` `hint` `error` `required`; controls `bind:value` `size` `mono` `invalid`; Select `options` `placeholder` |
| `EmptyState` | Nothing to show | `icon` · `message` or children · `actionLabel` + `onaction` or `action` snippet · `align` |
| `KeyValue` | Record details | `items` `{ key, value, mono?, truncate? }` · `columns` 1/2 |
| `Popover` | Help, chip details, small menus | `label` · `bind:open` · `placement` · `trigger` snippet (spread its props) |
| `Icon` | Every glyph | `icon` · `size` (16) · `label` |

### Button and IconButton

```svelte
<Button variant="primary" icon={Play} onclick={process}>Process</Button>
<Button variant="ghost" onclick={cancel}>Cancel</Button>
<Button variant="danger" icon={Trash2} loading={deleting} onclick={remove}>Delete</Button>
<IconButton icon={RefreshCw} label="Refresh" onclick={refresh} />
<IconButton icon={PanelBottom} label="Toggle panel" pressed={panelOpen} onclick={toggle} />
```

One `primary` per view. `size="md"` is for the primary action of a dialog.
`loading` disables the button and sets `aria-busy`.

### Tabs

```svelte
<Tabs label="Operator views" bind:value={tab} items={[
  { id: 'messages', label: 'Messages', count: receipts.length },
  { id: 'delivery', label: 'Delivery', testid: 'operator-tab-delivery' },
  { id: 'deployments', label: 'Deployments', disabled: !canDeploy }
]} />
```

One tab stop; arrows, Home and End move between enabled tabs. `activation`
defaults to `auto` (select on focus); use `manual` when selecting a tab
starts expensive work.

### Table with a details pane

```svelte
<Table label="Receipts">
  {#snippet head()}
    <tr>
      <Th width="120px" sort={sort} onsort={toggleSort}>Received</Th>
      <Th>Message id</Th>
      <Th width="72px" numeric>Segs</Th>
      <Th width="96px">Status</Th>
    </tr>
  {/snippet}
  {#each receipts as r (r.id)}
    <Tr selectable selected={r.id === selectedId} onselect={() => (selectedId = r.id)}>
      <Td mono muted value={r.receivedAt} />
      <Td mono truncate value={r.id} />
      <Td numeric value={r.segments} />
      <Td><Badge tone={toneFor(r.status)} dot>{r.status}</Badge></Td>
    </Tr>
  {/each}
</Table>

<KeyValue items={[
  { key: 'Message id', value: selected.id, mono: true, truncate: true },
  { key: 'Destination', value: selected.destination }  <!-- null renders — -->
]} />
```

The table wrapper scrolls, so give the Table a height (or a flex parent)
for the sticky header to matter. Pass `onsort` only where the API sorts.
Rows are focusable when `selectable`; ArrowUp/ArrowDown move between them
and Enter or Space selects.

### Field, Input, Select, Textarea

```svelte
<Field label="Destination endpoint" hint="FHIR R4 base URL." error={endpointError}>
  <Input bind:value={endpoint} mono />
</Field>
<Field label="Redaction">
  <Select bind:value={redaction} options={[{ value: 'none', label: 'None' }, { value: 'phi', label: 'PHI' }]} />
</Field>
```

The control inside a `Field` is labelled, described (hint or error) and
marked invalid automatically. Outside a `Field`, pass `aria-label`.

### Panel, EmptyState, Popover

```svelte
<Panel title="Deliveries">
  {#snippet actions()}<IconButton icon={RefreshCw} label="Refresh deliveries" />{/snippet}
  <EmptyState icon={Inbox} message="No deliveries in the last 24 hours." />
</Panel>

<EmptyState icon={ShieldAlert} align="start" data-testid="operator-preflight">
  Operator actions need the <code>integration.operator</code> role.
</EmptyState>
```

`Popover` closes on Escape (focus returns to the trigger) and on a pointer
down outside. It is positioned `fixed` from the trigger, so it escapes
`overflow: hidden` chrome such as the status bar; use `placement="top-start"`
there.

## Migration notes for U-1..U-3

Map legacy patterns to primitives; delete the legacy CSS as you go.

| Legacy pattern | Replace with |
|---|---|
| Pill tabs, `$lib/ui/Tabs.svelte` (7 imports) | `Tabs` (underline) in the route's `Toolbar` |
| Numbered stepper pill rows ("01 RAW SOURCE ▸ 02 …") | Remove; the bottom panel and results pane already carry that state |
| Hero `<h1>` + subtitle, `$lib/ui/PageHeader.svelte` (6 imports), `DocumentHost` title block | `Toolbar title="…"` |
| Explainer / "recommended move" / "next up" cards | A `?` `IconButton` + `Popover`, or nothing |
| Stacks of cards for records (events, receipts, deliveries, workflows) | `Table` + `Tr`/`Td` with a `KeyValue` details pane |
| Card wrappers, `$lib/ui/Panel.svelte` (24 imports) | `Panel` (flat) or no wrapper at all |
| Illustration empty states, `$lib/ui/EmptyState.svelte` (8 imports) | `EmptyState` (one sentence, 16 px icon) |
| `$lib/ui/Button.svelte` (35), `IconButton`, `Input`, `Select`, `TextArea` | The primitive of the same name (`on:click` becomes `onclick`) |
| `$lib/ui/Badge.svelte` (26), `StatusPill` (2), green/purple pill badges | `Badge` with a state `tone` (or neutral) |
| `✦` on the Copilot tab (`BottomPanel.svelte`), emoji, hand-drawn SVG icons | `Icon` with a Lucide glyph, or no glyph |
| Categorical colour maps (`DocumentHost` doc types, `EditorTabs`, `routes/+page.svelte` stages) using status colours and `--palette-violet-600` | Neutral text plus an icon; drop `--palette-violet-600` when the last use goes |
| Full-width labelled form fields at 40 px | `Field` + control at 28 px, on a 2-column grid where it fits |
| Access strip above the shell (`GraphQLCredentialGate`) | Status-bar chip with a `Popover` holding today's text and "Clear access" (U-1) |
| Journey band (`JourneyProgress.svelte`) | Header segmented control + `Next: …` in the status bar (U-1) |

Visual debt the token rewrite could not reach (raw values in component
styles), by owning lane:

- **U-1 (shell)**: radial/linear gradients in `IDEShell.svelte`
  (`.workspace-secondary`), `Sidebar.svelte`, `DocumentHost.svelte` (plus its
  24 px glow on the doc icon), `JourneyProgress.svelte` (retired);
  `GraphQLCredentialGate.svelte` background gradient; the Copilot `✦`.
- **U-2 (home, events, operator)**: `SystemStatusPanel.svelte` indigo
  gradient card; glows in `RecentEventsFeed.svelte` and
  `EventStreamPanel.svelte`; the demo-alerts fallback.
- **U-3 (intake, workflows, profiles, terminology)**: `AuthoringFlowRail.svelte`
  gradients; `ExtractionPanel.svelte` and `QualityBadge.svelte` gradients;
  glows in `WorkflowMonitor.svelte` and `CopilotPanel.svelte`;
  `hover-lift` cards in `MappingBrowser`, `PendingReviewList`,
  `AutorouteResolver` and `WorkflowList`; `--text-2xl` in
  `TemporalWorkflowList`, `PendingReviewList` and `WarningTrends` (keep it
  only where it is a stat figure).

Keep every honest `data-testid` from `.loom/36` (`operator-preflight`,
`streaming-unavailable`, `copilot-llm-state`, `problems-badge`,
`platform-indicator`) with its meaning; move it onto the primitive that
renders the state.

## Reviewing a UI change

Attach before/after PNGs at 1440×900 for every route you touch. Against a
local stack:

```bash
# API (repo root). The token must be ≥ 24 bytes; the Vite proxy connects over ::1.
export FI_FHIR_GRAPHQL_TRUSTED_CIDRS=127.0.0.1/32,::1/128
export FI_FHIR_GRAPHQL_BEARER_TOKEN=local-only-test-token-0000   # any ≥ 24-byte local value
# … the other FI_FHIR_* variables from DEVELOPER-GUIDE.md …
bin/fi-fhir serve --port 8081

# UI (ui/)
npm run dev -- --port 5180 --strictPort
npx playwright screenshot --viewport-size=1440,900 --color-scheme=dark \
  --wait-for-timeout=4000 http://localhost:5180/hl7 hl7.png
```

The dev server proxies `/graphql`, `/health` and `/api/auth/status`, so the
credential gate sees trusted-network access exactly as behind nginx. Once
signed in the app holds streams open, so wait on `load` plus a fixed delay,
not `networkidle`.
