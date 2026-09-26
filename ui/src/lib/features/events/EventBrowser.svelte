<!--
  EventBrowser — Events › Browse: a filters row, the event table (newest
  first, server-paged), and a details pane for the selected row. Up/Down move
  the selection; the pane follows it.

  The table shows the Event interface the browse query selects: time, type,
  source, format, event id and correlation id. There is no per-event status in
  that contract, so the table does not invent one.
-->
<script lang="ts">
  import { untrack } from 'svelte';
  import CircleAlert from '@lucide/svelte/icons/circle-alert';
  import Inbox from '@lucide/svelte/icons/inbox';
  import MousePointerClick from '@lucide/svelte/icons/mouse-pointer-click';
  import RefreshCw from '@lucide/svelte/icons/refresh-cw';
  import type { EventOrderBy, EventsQuery, EventFilter, EventType } from '$lib/gen/graphql';
  import { Button, EmptyState, Input, Select, Table, Td, Th, Tr } from '$lib/ui/primitives';
  import EventDetail from './EventDetail.svelte';
  import { EVENT_TYPES, formatEventTime } from './eventFormat';
  import { queryEvents } from './eventsApi';

  type EventEdge = EventsQuery['events']['edges'][number];
  type EventNode = EventEdge['node'];

  interface Props {
    /** Total matching events, for the route toolbar's count badge. */
    totalCount?: number;
    /** Whether a page is loading, for the route toolbar. */
    loading?: boolean;
  }

  let { totalCount = $bindable(0), loading = $bindable(false) }: Props = $props();

  const ORDER: EventOrderBy = { field: 'TIMESTAMP', direction: 'DESC' };

  let edges = $state<EventEdge[]>([]);
  let error = $state<string | null>(null);
  let endCursor = $state<string | null>(null);
  let hasNextPage = $state(false);
  let loaded = $state(false);
  let selectedId = $state<string | null>(null);

  let filterType = $state('');
  let filterSource = $state('');
  let pageSize = $state('50');

  const typeOptions = [
    { value: '', label: 'All types' },
    ...EVENT_TYPES.map((type) => ({ value: type, label: type }))
  ];
  const windowOptions = [
    { value: '25', label: '25 events' },
    { value: '50', label: '50 events' },
    { value: '100', label: '100 events' }
  ];

  const selected = $derived(edges.find((edge) => edge.node.id === selectedId)?.node ?? null);

  function buildFilter(): EventFilter | null {
    const types = filterType ? [filterType as EventType] : null;
    const source = filterSource.trim();
    const sources = source ? [source] : null;
    if (!types && !sources) return null;
    return {
      types,
      sources,
      patientMrn: null,
      correlationId: null,
      fromTimestamp: null,
      toTimestamp: null
    };
  }

  async function loadEvents(append = false): Promise<void> {
    loading = true;
    error = null;
    try {
      const result = await queryEvents(
        buildFilter(),
        Number(pageSize),
        append ? endCursor : null,
        ORDER
      );
      edges = append ? [...edges, ...result.edges] : result.edges;
      totalCount = result.totalCount;
      endCursor = result.pageInfo.endCursor ?? null;
      hasNextPage = result.pageInfo.hasNextPage;
      if (selectedId && !edges.some((edge) => edge.node.id === selectedId)) selectedId = null;
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to load events';
    } finally {
      loading = false;
      loaded = true;
    }
  }

  function refresh(): void {
    endCursor = null;
    void loadEvents();
  }

  // Type and window apply at once; the source filter applies on Enter or
  // Refresh so typing does not fire a query per keystroke.
  $effect(() => {
    void filterType;
    void pageSize;
    untrack(refresh);
  });

  function select(node: EventNode): void {
    selectedId = node.id;
  }
</script>

<div class="browser">
  <form
    class="filters"
    aria-label="Event filters"
    onsubmit={(event) => {
      event.preventDefault();
      refresh();
    }}
  >
    <div class="filter filter-type">
      <Select aria-label="Type" bind:value={filterType} options={typeOptions} />
    </div>
    <div class="filter filter-source">
      <Input aria-label="Source" bind:value={filterSource} placeholder="Source" mono />
    </div>
    <div class="filter filter-window">
      <Select aria-label="Window" bind:value={pageSize} options={windowOptions} />
    </div>
    <Button type="submit" icon={RefreshCw} {loading}>Refresh</Button>
  </form>

  <div class="split">
    <div class="list">
      {#if error}
        <EmptyState
          icon={CircleAlert}
          message={`Events could not be loaded: ${error}`}
          actionLabel="Retry"
          onaction={refresh}
        />
      {:else if loaded && edges.length === 0}
        <EmptyState icon={Inbox} message="No events match these filters." />
      {:else}
        <Table label="Events" layout="fixed" class="events-table">
          {#snippet head()}
            <tr>
              <Th width="156px">Time</Th>
              <Th width="176px">Type</Th>
              <Th width="120px">Source</Th>
              <Th width="72px">Format</Th>
              <Th>Event id</Th>
              <Th>Correlation id</Th>
            </tr>
          {/snippet}
          {#each edges as edge (edge.cursor)}
            <Tr
              selectable
              selected={edge.node.id === selectedId}
              onselect={() => select(edge.node)}
              onfocus={() => select(edge.node)}
            >
              <Td mono muted value={formatEventTime(edge.node.timestamp)} />
              <Td mono truncate value={edge.node.type} />
              <Td mono truncate value={edge.node.source} />
              <Td muted value={edge.node.sourceFormat ?? '—'} />
              <Td mono truncate value={edge.node.id} />
              <Td mono truncate muted value={edge.node.correlationId ?? '—'} />
            </Tr>
          {/each}
        </Table>
        {#if hasNextPage}
          <div class="more">
            <Button variant="ghost" {loading} onclick={() => void loadEvents(true)}>Load more</Button>
          </div>
        {/if}
      {/if}
    </div>

    <aside class="details" aria-label="Selected event">
      {#if selected}
        <EventDetail event={selected} onClose={() => (selectedId = null)} />
      {:else}
        <EmptyState
          icon={MousePointerClick}
          align="start"
          message="Select an event to see its record."
        />
      {/if}
    </aside>
  </div>
</div>

<style>
  .browser {
    display: flex;
    flex-direction: column;
    flex: 1 1 auto;
    min-height: 0;
  }

  .filters {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    padding: var(--space-2) var(--space-3);
    border-bottom: 1px solid var(--color-border-subtle);
  }

  .filter-type {
    width: 200px;
  }

  .filter-source {
    width: 200px;
  }

  .filter-window {
    width: 128px;
  }

  .split {
    display: grid;
    grid-template-columns: minmax(0, 1fr) 340px;
    flex: 1 1 auto;
    min-height: 0;
  }

  .list {
    display: flex;
    flex-direction: column;
    min-width: 0;
    min-height: 0;
  }

  .list :global(.events-table) {
    flex: 0 1 auto;
    min-height: 0;
  }

  .more {
    display: flex;
    justify-content: center;
    padding: var(--space-2);
    border-top: 1px solid var(--color-border-subtle);
  }

  .details {
    min-width: 0;
    min-height: 0;
    overflow: auto;
    padding: var(--space-3);
    border-left: 1px solid var(--color-border-subtle);
    background: var(--color-bg-elevated);
  }

  @media (max-width: 960px) {
    .split {
      grid-template-columns: minmax(0, 1fr);
    }

    .details {
      border-left: 0;
      border-top: 1px solid var(--color-border-subtle);
    }
  }
</style>
