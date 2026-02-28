<script lang="ts">
  import { onMount } from 'svelte';
  import { queryEvents } from '$lib/features/events/eventsApi';
  import type { EventsQuery, EventOrderBy } from '$lib/gen/graphql';
  import { resolve } from '$app/paths';

  type EventNode = EventsQuery['events']['edges'][number]['node'];

  let events: EventNode[] = [];
  let loading = true;

  function formatTimestamp(ts: string): string {
    try {
      const d = new Date(ts);
      const now = new Date();
      const diff = now.getTime() - d.getTime();
      if (diff < 60_000) return 'just now';
      if (diff < 3600_000) return `${Math.floor(diff / 60_000)}m ago`;
      if (diff < 86400_000) return `${Math.floor(diff / 3600_000)}h ago`;
      return d.toLocaleDateString();
    } catch {
      return ts;
    }
  }

  function typeColor(type: string): string {
    if (type.startsWith('PATIENT_')) return 'adt';
    if (type.startsWith('LAB_')) return 'lab';
    if (type.startsWith('APPOINTMENT_')) return 'appt';
    if (type.startsWith('CLAIM_') || type.startsWith('PRIOR_') || type.startsWith('ELIGIBILITY_')) return 'claim';
    return 'default';
  }

  onMount(async () => {
    try {
      const result = await queryEvents(
        null,
        10,
        null,
        { field: 'TIMESTAMP', direction: 'DESC' } as EventOrderBy
      );
      events = result.edges.map((e) => e.node);
    } catch {
      // Silently fail on dashboard — events section is optional
    } finally {
      loading = false;
    }
  });
</script>

<div class="feed">
  {#if loading}
    <p class="muted">Loading recent events...</p>
  {:else if events.length === 0}
    <p class="muted">No events yet. Submit a message to get started.</p>
  {:else}
    <div class="event-list">
      {#each events as event (event.id)}
        <div class="event-row">
          <span class="type-dot {typeColor(event.type)}"></span>
          <span class="type-text">{event.type.replace(/_/g, ' ')}</span>
          <span class="source mono">{event.source}</span>
          <span class="time">{formatTimestamp(event.timestamp)}</span>
        </div>
      {/each}
    </div>
    <a class="view-all" href={resolve('/events')}>View all events &rarr;</a>
  {/if}
</div>

<style>
  .feed {
    display: grid;
    gap: 8px;
  }

  .muted {
    color: var(--color-text-muted);
    margin: 0;
    font-size: 0.9rem;
  }

  .event-list {
    display: grid;
    gap: 4px;
  }

  .event-row {
    display: grid;
    grid-template-columns: 10px 1fr auto auto;
    gap: 8px;
    align-items: center;
    padding: 6px 0;
  }

  .type-dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: var(--color-border-strong);
  }

  .type-dot.adt { background: rgba(59, 130, 246, 0.7); }
  .type-dot.lab { background: rgba(16, 185, 129, 0.7); }
  .type-dot.appt { background: rgba(245, 158, 11, 0.7); }
  .type-dot.claim { background: rgba(168, 85, 247, 0.7); }

  .type-text {
    font-size: 0.85rem;
    color: var(--color-text-secondary);
    text-transform: capitalize;
  }

  .source {
    font-size: 0.8rem;
    color: var(--color-text-muted);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .time {
    font-size: 0.8rem;
    color: var(--color-text-tertiary);
    white-space: nowrap;
  }

  .mono { font-family: var(--font-mono); }

  .view-all {
    font-size: 0.85rem;
    color: var(--color-primary);
    text-decoration: none;
    font-weight: 700;
    padding-top: 4px;
  }

  .view-all:hover {
    text-decoration: underline;
  }
</style>
