<!--
  Primitives gallery — dev only (/design). Renders every primitive in the
  states pages use, plus one composed route in the target page pattern, so a
  reviewer can judge the system from a single 1440x900 screenshot.
  `?theme=light|dark` previews a theme without persisting it.
  All data here is synthetic.
-->
<script lang="ts">
  import { onDestroy } from 'svelte';
  import Activity from '@lucide/svelte/icons/activity';
  import CircleAlert from '@lucide/svelte/icons/circle-alert';
  import CircleHelp from '@lucide/svelte/icons/circle-help';
  import Copy from '@lucide/svelte/icons/copy';
  import Download from '@lucide/svelte/icons/download';
  import Inbox from '@lucide/svelte/icons/inbox';
  import Moon from '@lucide/svelte/icons/moon';
  import Play from '@lucide/svelte/icons/play';
  import Plus from '@lucide/svelte/icons/plus';
  import RefreshCw from '@lucide/svelte/icons/refresh-cw';
  import Search from '@lucide/svelte/icons/search';
  import Settings from '@lucide/svelte/icons/settings';
  import Sun from '@lucide/svelte/icons/sun';
  import Trash2 from '@lucide/svelte/icons/trash-2';
  import Upload from '@lucide/svelte/icons/upload';
  import X from '@lucide/svelte/icons/x';
  import { initTheme } from '$lib/theme/theme';
  import {
    Badge,
    Button,
    EmptyState,
    Field,
    Icon,
    IconButton,
    Input,
    KeyValue,
    Panel,
    Popover,
    Select,
    Table,
    Tabs,
    Td,
    Textarea,
    Th,
    Toolbar,
    Tr,
    type BadgeTone,
    type SortDirection
  } from '$lib/ui/primitives';

  type Theme = 'dark' | 'light';

  function initialTheme(): Theme {
    const fromQuery = new URLSearchParams(window.location.search).get('theme');
    if (fromQuery === 'light' || fromQuery === 'dark') return fromQuery;
    return document.documentElement.getAttribute('data-theme') === 'light' ? 'light' : 'dark';
  }

  let theme = $state<Theme>(initialTheme());

  // Preview only: write the attribute after the layout's initTheme() has run,
  // and restore the persisted preference when leaving the gallery.
  $effect(() => {
    const next = theme;
    const timer = setTimeout(() => document.documentElement.setAttribute('data-theme', next), 0);
    return () => clearTimeout(timer);
  });
  onDestroy(() => initTheme());

  // ── Page-pattern demo (synthetic) ─────────────────────────────────────────
  interface EventRow {
    id: string;
    time: string;
    type: string;
    source: string;
    patient: string;
    segments: number;
    ms: number;
    status: 'delivered' | 'warning' | 'failed' | 'pending';
  }

  const rows: EventRow[] = [
    { id: 'evt_01J8Q3ZK4M7Y2R9C5T1B6N0PXA', time: '10:42:18.204', type: 'ADT^A01', source: 'adt-http', patient: 'SYN-000184', segments: 7, ms: 4, status: 'delivered' },
    { id: 'evt_01J8Q3ZJ9W1D8H3K6F2Q5V7MZB', time: '10:42:17.991', type: 'ORU^R01', source: 'lab-mllp', patient: 'SYN-000377', segments: 23, ms: 11, status: 'warning' },
    { id: 'evt_01J8Q3ZH2P6T4X8N1C9R3L5GWC', time: '10:42:16.530', type: 'ADT^A08', source: 'adt-http', patient: 'SYN-000184', segments: 6, ms: 3, status: 'delivered' },
    { id: 'evt_01J8Q3ZG7S3B5M2V8K4D1J6YQD', time: '10:42:15.087', type: 'SIU^S12', source: 'sched-sftp', patient: 'SYN-000912', segments: 9, ms: 142, status: 'failed' },
    { id: 'evt_01J8Q3ZF5N8R1Q7W3H6T2C9KXE', time: '10:42:14.402', type: 'ADT^A03', source: 'adt-http', patient: 'SYN-000205', segments: 5, ms: 5, status: 'pending' },
    { id: 'evt_01J8Q3ZE3L4G9Y6P1Z8B5V2NHF', time: '10:42:13.776', type: 'ORM^O01', source: 'orders-mllp', patient: 'SYN-000377', segments: 12, ms: 8, status: 'delivered' }
  ];

  const statusTone: Record<EventRow['status'], BadgeTone> = {
    delivered: 'success',
    warning: 'warning',
    failed: 'danger',
    pending: 'neutral'
  };

  let view = $state('browse');
  let selectedId = $state(rows[1]?.id ?? '');
  let timeSort = $state<SortDirection>('descending');
  let query = $state('');
  let typeFilter = $state('');
  const selected = $derived(rows.find((row) => row.id === selectedId));
  const sortedRows = $derived(timeSort === 'ascending' ? [...rows].reverse() : rows);

  // ── Controls demo ─────────────────────────────────────────────────────────
  let demoTab = $state('mapping');
  let endpoint = $state('https://fhir.example.test/r4');
  let retries = $state('3');
  let expression = $state('msg.PID.3.1 != ""');
  let loading = $state(false);

  function fakeLoad(): void {
    loading = true;
    setTimeout(() => (loading = false), 1200);
  }

  const colorTokens = [
    ['--color-bg-base', 'canvas'],
    ['--color-bg-elevated', 'chrome, panels'],
    ['--color-bg-input', 'inset controls'],
    ['--color-border-default', 'borders'],
    ['--color-text-primary', 'text'],
    ['--color-text-secondary', 'secondary'],
    ['--color-text-tertiary', 'labels'],
    ['--color-primary', 'accent'],
    ['--color-success', 'success'],
    ['--color-warning', 'warning'],
    ['--color-danger', 'danger'],
    ['--color-info', 'info']
  ] as const;

  const typeScale = [
    ['--text-title', '15 / semibold', 'Page title in a toolbar'],
    ['--text-ui', '13 / regular', 'UI base: body, controls, table cells'],
    ['--text-xs', '12 / regular', 'Secondary text, hints'],
    ['--text-mono', '12 / mono', 'evt_01J8Q3ZK4M7Y2R9C5T1B6N0PXA'],
    ['--text-label', '11 / caps', 'Labels and table headers']
  ] as const;
</script>

<svelte:head>
  <title>Design system · fi-fhir</title>
</svelte:head>

<div class="gallery">
  <Toolbar title="Design system" data-testid="design-gallery">
    {#snippet actions()}
      <Badge tone="warning">Dev only</Badge>
      <IconButton
        icon={Moon}
        label="Dark theme"
        pressed={theme === 'dark'}
        onclick={() => (theme = 'dark')}
      />
      <IconButton
        icon={Sun}
        label="Light theme"
        pressed={theme === 'light'}
        onclick={() => (theme = 'light')}
      />
    {/snippet}
  </Toolbar>

  <main class="gallery-body">
    <!-- ── The page pattern, composed ─────────────────────────────────── -->
    <section class="section" aria-labelledby="pattern-title">
      <h2 id="pattern-title" class="section-title">Page pattern · toolbar + filters + table + details</h2>
      <div class="frame">
        <Toolbar title="Events">
          {#snippet tabs()}
            <Tabs
              label="Event views"
              bind:value={view}
              items={[
                { id: 'browse', label: 'Browse' },
                { id: 'live', label: 'Live Stream' },
                { id: 'timeline', label: 'Patient Timeline' },
                { id: 'stats', label: 'Statistics' }
              ]}
            />
          {/snippet}
          {#snippet actions()}
            <Popover label="About Events" placement="bottom-end">
              {#snippet trigger(props)}
                <IconButton {...props} icon={CircleHelp} label="About Events" />
              {/snippet}
              Events are read from the event store for the selected tenant. The list shows the
              newest 500; narrow it with the filters.
            </Popover>
            <Button variant="ghost" icon={RefreshCw}>Refresh</Button>
            <Button icon={Download}>Export</Button>
          {/snippet}
        </Toolbar>

        <div class="filters">
          <div class="filter-search">
            <Icon icon={Search} class="filter-search-icon" />
            <Input bind:value={query} placeholder="Filter by id, patient or source" aria-label="Filter events" />
          </div>
          <div class="filter-select">
            <Select
              aria-label="Message type"
              bind:value={typeFilter}
              placeholder="All types"
              options={[
                { value: 'ADT', label: 'ADT' },
                { value: 'ORU', label: 'ORU' },
                { value: 'SIU', label: 'SIU' }
              ]}
            />
          </div>
          <span class="filter-count text-mono">6 of 1,284</span>
        </div>

        <div class="split">
          <Table label="Events" class="split-table">
            {#snippet head()}
              <tr>
                <Th
                  width="120px"
                  sort={timeSort}
                  onsort={() => (timeSort = timeSort === 'descending' ? 'ascending' : 'descending')}
                  >Time</Th
                >
                <Th width="96px">Type</Th>
                <Th width="110px">Source</Th>
                <Th>Event id</Th>
                <Th width="110px">Patient</Th>
                <Th width="64px" numeric>Segs</Th>
                <Th width="64px" numeric>ms</Th>
                <Th width="96px">Status</Th>
              </tr>
            {/snippet}
            {#each sortedRows as row (row.id)}
              <Tr selectable selected={row.id === selectedId} onselect={() => (selectedId = row.id)}>
                <Td mono muted value={row.time} />
                <Td mono value={row.type} />
                <Td muted value={row.source} />
                <Td mono truncate value={row.id} />
                <Td mono value={row.patient} />
                <Td numeric value={row.segments} />
                <Td numeric value={row.ms} />
                <Td><Badge tone={statusTone[row.status]} dot>{row.status}</Badge></Td>
              </Tr>
            {/each}
          </Table>

          <aside class="details" aria-label="Selected event">
            {#if selected}
              <div class="details-head">
                <span class="details-title text-mono">{selected.type}</span>
                <Badge tone={statusTone[selected.status]} dot>{selected.status}</Badge>
                <span class="details-actions">
                  <IconButton icon={Copy} label="Copy event id" />
                  <IconButton icon={X} label="Close details" />
                </span>
              </div>
              <KeyValue
                items={[
                  { key: 'Event id', value: selected.id, mono: true, truncate: true },
                  { key: 'Received', value: `2026-09-26 ${selected.time}`, mono: true },
                  { key: 'Source', value: selected.source },
                  { key: 'Patient', value: selected.patient, mono: true },
                  { key: 'Segments', value: selected.segments, mono: true },
                  { key: 'Latency', value: `${selected.ms} ms`, mono: true },
                  { key: 'Destination', value: null }
                ]}
              />
              {#if selected.status === 'warning'}
                <p class="details-note">
                  <Icon icon={CircleAlert} class="note-icon" />
                  OBX-5 exceeded 64 KB and was truncated before delivery.
                </p>
              {/if}
            {/if}
          </aside>
        </div>
      </div>
    </section>

    <div class="grid">
      <!-- ── Buttons ───────────────────────────────────────────────────── -->
      <section class="section" aria-labelledby="buttons-title">
        <h2 id="buttons-title" class="section-title">Button · IconButton</h2>
        <div class="row">
          <Button variant="primary" icon={Play}>Process</Button>
          <Button icon={Upload}>Load file</Button>
          <Button variant="ghost">Cancel</Button>
          <Button variant="danger" icon={Trash2}>Delete</Button>
          <Button variant="primary" {loading} onclick={fakeLoad}>{loading ? 'Running' : 'Run'}</Button>
          <Button disabled>Disabled</Button>
        </div>
        <div class="row">
          <Button variant="primary" size="md">Deploy revision</Button>
          <Button size="md">Save draft</Button>
          <IconButton icon={Settings} label="Settings" />
          <IconButton icon={Plus} label="Add" variant="secondary" />
          <IconButton icon={Activity} label="Toggle trace" pressed={true} />
          <Button iconOnly aria-label="Refresh" icon={RefreshCw} />
        </div>
      </section>

      <!-- ── Badges ────────────────────────────────────────────────────── -->
      <section class="section" aria-labelledby="badges-title">
        <h2 id="badges-title" class="section-title">Badge</h2>
        <div class="row">
          <Badge>neutral</Badge>
          <Badge tone="success" dot>delivered</Badge>
          <Badge tone="warning" dot>2 warnings</Badge>
          <Badge tone="danger" dot>failed</Badge>
          <Badge tone="info">streaming</Badge>
          <Badge tone="accent" mono>3 selected</Badge>
        </div>
        <div class="row">
          <Badge mono>1,284</Badge>
          <Badge mono>PID-3.1</Badge>
          <Badge mono tone="danger">E102</Badge>
        </div>
      </section>

      <!-- ── Tabs ──────────────────────────────────────────────────────── -->
      <section class="section" aria-labelledby="tabs-title">
        <h2 id="tabs-title" class="section-title">Tabs · underline, roving focus</h2>
        <div class="tabs-demo">
          <Tabs
            label="Workflow views"
            bind:value={demoTab}
            items={[
              { id: 'inventory', label: 'Inventory', count: 12 },
              { id: 'mapping', label: 'Design' },
              { id: 'verify', label: 'Verification', count: 2 },
              { id: 'archive', label: 'Archived', disabled: true }
            ]}
          />
        </div>
      </section>

      <!-- ── Fields ────────────────────────────────────────────────────── -->
      <section class="section" aria-labelledby="fields-title">
        <h2 id="fields-title" class="section-title">Field · Input · Select · Textarea</h2>
        <div class="fields">
          <Field label="Destination endpoint" hint="FHIR R4 base URL.">
            <Input bind:value={endpoint} mono />
          </Field>
          <Field label="Retry limit" required>
            <Select
              bind:value={retries}
              options={[
                { value: '0', label: 'No retries' },
                { value: '3', label: '3 attempts' },
                { value: '10', label: '10 attempts' }
              ]}
            />
          </Field>
          <Field label="Route filter" error="Expression must evaluate to a boolean.">
            <Input value="msg.PID.3" mono />
          </Field>
          <Field label="Guard expression">
            <Textarea bind:value={expression} mono rows={2} />
          </Field>
        </div>
      </section>

      <!-- ── Panel + EmptyState ────────────────────────────────────────── -->
      <section class="section" aria-labelledby="panel-title">
        <h2 id="panel-title" class="section-title">Panel · EmptyState</h2>
        <div class="panels">
          <Panel title="Deliveries">
            {#snippet actions()}
              <IconButton icon={RefreshCw} label="Refresh deliveries" />
            {/snippet}
            <EmptyState
              icon={Inbox}
              message="No deliveries in the last 24 hours."
              actionLabel="Send test message"
              onaction={() => {}}
            />
          </Panel>
          <Panel title="Alerts" flush>
            <EmptyState icon={CircleAlert} align="start">
              No alert source configured. Connect the observability platform to see alerts.
            </EmptyState>
          </Panel>
        </div>
      </section>

      <!-- ── Tokens ────────────────────────────────────────────────────── -->
      <section class="section" aria-labelledby="tokens-title">
        <h2 id="tokens-title" class="section-title">Colour · type</h2>
        <div class="swatches">
          {#each colorTokens as [token, use] (token)}
            <div class="swatch">
              <span class="swatch-chip" style:background="var({token})"></span>
              <span class="swatch-name text-mono">{token.replace('--color-', '')}</span>
              <span class="swatch-use">{use}</span>
            </div>
          {/each}
        </div>
        <dl class="type-scale">
          {#each typeScale as [token, spec, sample] (token)}
            <dt class="text-mono">{token} <span>{spec}</span></dt>
            <dd class="type-sample" data-token={token}>{sample}</dd>
          {/each}
        </dl>
      </section>
    </div>
  </main>
</div>

<style>
  .gallery {
    min-height: 100vh;
    background: var(--color-bg-base);
    color: var(--color-text-primary);
  }

  .gallery-body {
    display: flex;
    flex-direction: column;
    gap: var(--space-6);
    padding: var(--space-4);
  }

  .section {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    min-width: 0;
  }

  .section-title {
    margin: 0;
    font-size: var(--text-label);
    font-weight: var(--font-semibold);
    letter-spacing: var(--tracking-label);
    text-transform: uppercase;
    color: var(--color-text-tertiary);
  }

  .frame {
    display: flex;
    flex-direction: column;
    border: 1px solid var(--color-border-default);
    border-radius: var(--radius-sm);
    overflow: hidden;
  }

  .filters {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    padding: var(--space-2) var(--space-3);
    border-bottom: 1px solid var(--color-border-subtle);
  }

  .filter-search {
    position: relative;
    width: 280px;
  }

  .filter-search :global(.filter-search-icon) {
    position: absolute;
    left: 8px;
    top: 50%;
    transform: translateY(-50%);
    color: var(--color-text-tertiary);
    pointer-events: none;
  }

  .filter-search :global(.ui-input) {
    padding-left: 30px;
  }

  .filter-select {
    width: 140px;
  }

  .filter-count {
    margin-left: auto;
    color: var(--color-text-tertiary);
  }

  .split {
    display: grid;
    grid-template-columns: minmax(0, 1fr) 320px;
    min-height: 0;
  }

  .split :global(.split-table) {
    max-height: 240px;
  }

  .details {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    padding: var(--space-3);
    border-left: 1px solid var(--color-border-subtle);
    background: var(--color-bg-elevated);
  }

  .details-head {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }

  .details-title {
    font-size: var(--text-ui);
    font-weight: var(--font-semibold);
  }

  .details-actions {
    display: flex;
    margin-left: auto;
  }

  .details-note {
    display: flex;
    gap: var(--space-2);
    margin: 0;
    padding: var(--space-2);
    border: 1px solid var(--color-warning-border);
    border-radius: var(--radius-sm);
    background: var(--color-warning-bg);
    color: var(--color-text-primary);
    font-size: var(--text-xs);
  }

  .details-note :global(.note-icon) {
    color: var(--color-warning-text);
    margin-top: 1px;
  }

  .grid {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: var(--space-6) var(--space-8);
  }

  .row {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2);
  }

  .tabs-demo {
    display: flex;
    height: var(--toolbar-height);
    border-bottom: 1px solid var(--color-border-subtle);
  }

  .fields {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: var(--space-3);
  }

  .panels {
    display: grid;
    gap: var(--space-3);
  }

  .swatches {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 6px var(--space-3);
  }

  .swatch {
    display: grid;
    grid-template-columns: 16px auto 1fr;
    align-items: center;
    gap: var(--space-2);
    font-size: var(--text-xs);
  }

  .swatch-chip {
    width: 16px;
    height: 16px;
    border-radius: var(--radius-sm);
    border: 1px solid var(--color-border-default);
  }

  .swatch-use {
    color: var(--color-text-tertiary);
  }

  .type-scale {
    display: grid;
    grid-template-columns: max-content 1fr;
    align-items: baseline;
    gap: 6px var(--space-3);
    margin: 0;
  }

  .type-scale dt {
    color: var(--color-text-tertiary);
  }

  .type-scale dt span {
    color: var(--color-text-muted);
  }

  .type-scale dd {
    margin: 0;
  }

  .type-sample[data-token='--text-title'] {
    font-size: var(--text-title);
    font-weight: var(--font-semibold);
  }

  .type-sample[data-token='--text-ui'] {
    font-size: var(--text-ui);
  }

  .type-sample[data-token='--text-xs'] {
    font-size: var(--text-xs);
    color: var(--color-text-secondary);
  }

  .type-sample[data-token='--text-mono'] {
    font-family: var(--font-mono);
    font-size: var(--text-mono);
  }

  .type-sample[data-token='--text-label'] {
    font-size: var(--text-label);
    font-weight: var(--font-medium);
    letter-spacing: var(--tracking-label);
    text-transform: uppercase;
    color: var(--color-text-tertiary);
  }
</style>
