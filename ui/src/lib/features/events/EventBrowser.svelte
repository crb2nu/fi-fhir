<script lang="ts">
  import { queryEvents } from './eventsApi';
  import type { EventsQuery, EventFilter, EventType, EventOrderBy } from '$lib/gen/graphql';
  import Button from '$lib/ui/Button.svelte';
  import EmptyState from '$lib/ui/EmptyState.svelte';
  import { createEventDispatcher } from 'svelte';

  type EventEdge = EventsQuery['events']['edges'][number];
  type EventNode = EventEdge['node'];

  const dispatch = createEventDispatcher<{ select: { event: EventNode } }>();

  let edges: EventEdge[] = [];
  let totalCount = 0;
  let loading = false;
  let error: string | null = null;
  let endCursor: string | null = null;
  let hasNextPage = false;

  // Filters
  let filterType: EventType | 'ALL' = 'ALL';
  let filterSource = '';
  let pageSize = 50;
  let filterSummary = 'All downstream events';
  let windowSummary = 'Showing 50 events per page';
  const pageSizes = [25, 50, 100] as const;

  const eventTypes: EventType[] = [
    'PATIENT_ADMIT', 'PATIENT_DISCHARGE', 'PATIENT_TRANSFER', 'PATIENT_UPDATE',
    'LAB_RESULT', 'LAB_ORDERED', 'APPOINTMENT_SCHEDULED', 'APPOINTMENT_CANCELLED',
    'CLAIM_SUBMITTED', 'CLAIM_ADJUDICATED', 'VITAL_SIGN', 'CONDITION',
    'PROCEDURE', 'IMMUNIZATION', 'DOCUMENT'
  ];

  function buildFilter(): EventFilter | null {
    const types = filterType !== 'ALL' ? [filterType] : null;
    const sources = filterSource.trim() ? [filterSource.trim()] : null;
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

  function handlePageSizeChange(event: Event): void {
    pageSize = Number((event.currentTarget as HTMLSelectElement).value);
  }

  async function loadEvents(append = false) {
    loading = true;
    error = null;
    try {
      const result = await queryEvents(
        buildFilter(),
        pageSize,
        append ? endCursor : null,
        { field: 'TIMESTAMP', direction: 'DESC' } as EventOrderBy
      );
      if (append) {
        edges = [...edges, ...result.edges];
      } else {
        edges = result.edges;
      }
      totalCount = result.totalCount;
      endCursor = result.pageInfo.endCursor ?? null;
      hasNextPage = result.pageInfo.hasNextPage;
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to load events';
    } finally {
      loading = false;
    }
  }

  function refresh() {
    endCursor = null;
    loadEvents();
  }

  function loadMore() {
    loadEvents(true);
  }

  function formatTimestamp(ts: string): string {
    try {
      return new Date(ts).toLocaleString();
    } catch {
      return ts;
    }
  }

  function formatEventType(type: string): string {
    return type.replace(/_/g, ' ');
  }

  function typeColor(type: string): string {
    if (type.startsWith('PATIENT_')) return 'adt';
    if (type.startsWith('LAB_')) return 'lab';
    if (type.startsWith('APPOINTMENT_')) return 'appt';
    if (type.startsWith('CLAIM_') || type.startsWith('PRIOR_') || type.startsWith('ELIGIBILITY_')) return 'claim';
    return 'default';
  }

  function formatFilterSummary(): string {
    const parts: string[] = [];

    if (filterType !== 'ALL') {
      parts.push(formatEventType(filterType));
    }
    if (filterSource.trim()) {
      parts.push(`source ${filterSource.trim()}`);
    }

    return parts.length > 0 ? `Filtered by ${parts.join(' · ')}` : 'All downstream events';
  }

  // Reload when filters change
  let lastFilterKey = '';
  $: {
    filterSummary = formatFilterSummary();
    windowSummary = `Showing ${pageSize} events per page`;

    const key = `${filterType}|${filterSource}|${pageSize}`;
    if (key !== lastFilterKey) {
      lastFilterKey = key;
      refresh();
    }
  }
</script>

<div class="browser">
  <div class="hero">
    <div class="hero-copy">
      <p class="eyebrow">Downstream verification</p>
      <h2>Event browser</h2>
      <p class="hero-text">
        Use this view to confirm what arrived after an integration run. Start broad, then narrow
        by source or event type when something looks off.
      </p>
    </div>

    <div class="hero-stats" aria-label="Event browser summary">
      <div class="stat">
        <span class="stat-label">Total</span>
        <span class="stat-value">{totalCount}</span>
      </div>
      <div class="stat">
        <span class="stat-label">Window</span>
        <span class="stat-value">{windowSummary}</span>
      </div>
      <div class="stat">
        <span class="stat-label">Scope</span>
        <span class="stat-value">{filterSummary}</span>
      </div>
    </div>
  </div>

  <div class="toolbar">
    <div class="filters">
      <label class="filter">
        Type
        <select class="select" bind:value={filterType}>
          <option value="ALL">All types</option>
          {#each eventTypes as type (type)}
            <option value={type}>{formatEventType(type)}</option>
          {/each}
        </select>
      </label>

      <label class="filter">
        Source
        <input
          aria-label="Filter by source"
          type="text"
          class="input"
          bind:value={filterSource}
          placeholder="Filter downstream source..."
        />
      </label>

      <label class="filter compact">
        Window
        <select class="select" value={pageSize} on:change={handlePageSizeChange}>
          {#each pageSizes as size (size)}
            <option value={size}>{size} events</option>
          {/each}
        </select>
      </label>
    </div>

    <div class="actions">
      <span class="count">{totalCount} events</span>
      <Button variant="secondary" size="sm" on:click={refresh} {loading}>
        Refresh
      </Button>
    </div>
  </div>

  {#if error}
    <EmptyState icon="error" title="Failed to load events" description={error}>
      <Button variant="secondary" on:click={refresh}>Retry</Button>
    </EmptyState>
  {:else if edges.length === 0 && !loading}
    <EmptyState
      icon="inbox"
      title="No downstream events found"
      description="Events will appear here once integrations emit processed records."
    />
  {:else}
    <div class="event-list">
      {#each edges as edge (edge.cursor)}
        <button
          class="event-row"
          type="button"
          aria-label={`${formatEventType(edge.node.type)} from ${edge.node.source} at ${formatTimestamp(edge.node.timestamp)}`}
          on:click={() => dispatch('select', { event: edge.node })}
        >
          <div class="event-main">
            <div class="event-topline">
              <span class="time mono">{formatTimestamp(edge.node.timestamp)}</span>
              <span class="type-pill {typeColor(edge.node.type)}">{formatEventType(edge.node.type)}</span>
            </div>

            <div class="event-bottomline">
              <span class="source mono">{edge.node.source}</span>
              <span class="id muted mono" title={edge.node.id}>ID {edge.node.id.slice(0, 10)}...</span>
            </div>
          </div>
        </button>
      {/each}
    </div>

    {#if hasNextPage}
      <div class="load-more">
        <Button variant="secondary" size="sm" on:click={loadMore} {loading}>
          Load more
        </Button>
      </div>
    {/if}
  {/if}
</div>

<style>
  .browser {
    display: grid;
    gap: 16px;
  }

  .hero {
    display: grid;
    grid-template-columns: minmax(0, 1fr) minmax(240px, 0.8fr);
    gap: 16px;
    align-items: stretch;
  }

  .hero-copy,
  .hero-stats {
    padding: 16px;
    border-radius: 18px;
    border: 1px solid var(--color-border-subtle);
    background: var(--color-bg-surface);
  }

  .eyebrow {
    margin: 0 0 6px;
    text-transform: uppercase;
    letter-spacing: 0.12em;
    font-size: 0.7rem;
    font-weight: 700;
    color: var(--color-text-tertiary);
  }

  h2 {
    margin: 0 0 8px;
    font-size: 1.4rem;
    color: var(--color-text-primary);
  }

  .hero-text {
    margin: 0;
    color: var(--color-text-secondary);
    line-height: 1.55;
  }

  .hero-stats {
    display: grid;
    gap: 10px;
  }

  .stat {
    display: grid;
    gap: 3px;
  }

  .stat-label {
    color: var(--color-text-tertiary);
    font-size: 0.72rem;
    text-transform: uppercase;
    letter-spacing: 0.08em;
    font-weight: 700;
  }

  .stat-value {
    color: var(--color-text-primary);
    font-weight: 700;
    line-height: 1.35;
  }

  .toolbar {
    display: flex;
    justify-content: space-between;
    align-items: flex-end;
    gap: 12px;
    flex-wrap: wrap;
  }

  .filters {
    display: flex;
    gap: 12px;
    flex-wrap: wrap;
    align-items: flex-end;
  }

  .filter {
    display: grid;
    gap: 6px;
    color: var(--color-text-secondary);
    font-size: 0.9rem;
    font-weight: 700;
    min-width: 160px;
  }

  .filter.compact {
    min-width: 140px;
  }

  .select,
  .input {
    padding: 8px 12px;
    border-radius: 10px;
    border: 1px solid var(--color-border-default);
    background: var(--color-bg-input);
    color: var(--color-text-primary);
    outline: none;
  }

  .select:focus,
  .input:focus {
    border-color: var(--color-border-focus);
    box-shadow: var(--shadow-focus);
  }

  .actions {
    display: flex;
    align-items: center;
    gap: 12px;
  }

  .count {
    color: var(--color-text-tertiary);
    font-size: 0.85rem;
    font-weight: 700;
  }

  .event-list {
    display: grid;
    gap: 8px;
    max-height: 600px;
    overflow-y: auto;
  }

  .event-row {
    display: grid;
    padding: 12px 14px;
    border-radius: 12px;
    border: 1px solid var(--color-border-default);
    background: var(--color-bg-elevated);
    cursor: pointer;
    text-align: left;
    width: 100%;
    color: inherit;
    font: inherit;
    transition: var(--transition-colors);
  }

  .event-row:focus-visible {
    outline: none;
    border-color: var(--color-border-focus);
    box-shadow: var(--shadow-focus);
  }

  .event-row:hover {
    background: var(--color-bg-hover);
    border-color: var(--color-border-strong);
  }

  .event-main {
    display: grid;
    gap: 6px;
    min-width: 0;
  }

  .event-topline,
  .event-bottomline {
    display: flex;
    align-items: center;
    gap: 10px;
    flex-wrap: wrap;
  }

  .time {
    color: var(--color-text-tertiary);
    font-size: 0.85rem;
  }

  .type-pill {
    padding: 2px 8px;
    border-radius: 999px;
    font-size: 0.75rem;
    font-weight: 700;
    text-transform: uppercase;
    white-space: nowrap;
  }

  .type-pill.adt { background: rgba(59, 130, 246, 0.15); border: 1px solid rgba(59, 130, 246, 0.3); color: rgba(147, 197, 253, 0.95); }
  .type-pill.lab { background: rgba(16, 185, 129, 0.15); border: 1px solid rgba(16, 185, 129, 0.3); color: rgba(110, 231, 183, 0.95); }
  .type-pill.appt { background: rgba(245, 158, 11, 0.15); border: 1px solid rgba(245, 158, 11, 0.3); color: rgba(253, 230, 138, 0.95); }
  .type-pill.claim { background: rgba(168, 85, 247, 0.15); border: 1px solid rgba(168, 85, 247, 0.3); color: rgba(216, 180, 254, 0.95); }
  .type-pill.default { background: var(--color-bg-surface); border: 1px solid var(--color-border-strong); color: var(--color-text-secondary); }

  .source {
    color: var(--color-text-secondary);
    font-size: 0.85rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .mono { font-family: var(--font-mono); }
  .muted { color: var(--color-text-muted); }

  .id { font-size: 0.8rem; }

  .load-more {
    display: flex;
    justify-content: center;
    padding-top: 8px;
  }

  @media (max-width: 880px) {
    .hero {
      grid-template-columns: 1fr;
    }
  }
</style>
