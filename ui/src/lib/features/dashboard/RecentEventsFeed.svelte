<script lang="ts">
  import { onMount } from "svelte";
  import { queryEvents } from "$lib/features/events/eventsApi";
  import type { EventsQuery, EventOrderBy } from "$lib/gen/graphql";
  import { resolve } from "$app/paths";

  type EventNode = EventsQuery["events"]["edges"][number]["node"];

  let events: EventNode[] = [];
  let loading = true;

  function formatTimestamp(ts: string): string {
    try {
      const d = new Date(ts);
      const now = new Date();
      const diff = now.getTime() - d.getTime();
      if (diff < 60_000) return "just now";
      if (diff < 3600_000) return `${Math.floor(diff / 60_000)}m ago`;
      if (diff < 86400_000) return `${Math.floor(diff / 3600_000)}h ago`;
      return d.toLocaleDateString();
    } catch {
      return ts;
    }
  }

  function typeColor(type: string): string {
    if (type.startsWith("PATIENT_")) return "adt";
    if (type.startsWith("LAB_")) return "lab";
    if (type.startsWith("APPOINTMENT_")) return "appt";
    if (
      type.startsWith("CLAIM_") ||
      type.startsWith("PRIOR_") ||
      type.startsWith("ELIGIBILITY_")
    )
      return "claim";
    return "default";
  }

  onMount(async () => {
    try {
      const result = await queryEvents(null, 10, null, {
        field: "TIMESTAMP",
        direction: "DESC",
      } as EventOrderBy);
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
    <div class="event-list loading-state">
      {#each [1, 2, 3] as i (i)}
        <div class="event-row skeleton">
          <div class="skeleton-dot"></div>
          <div class="skeleton-text" style="width: 120px;"></div>
          <div
            class="skeleton-text"
            style="width: 80px; margin-left: auto;"
          ></div>
          <div class="skeleton-text" style="width: 60px;"></div>
        </div>
      {/each}
    </div>
  {:else if events.length === 0}
    <div class="empty-state">
      <div class="empty-icon">⌘</div>
      <p class="muted">No events yet.</p>
      <p class="sub">Submit a message to get started.</p>
    </div>
  {:else}
    <div class="event-list">
      {#each events as event (event.id)}
        <div
          class="event-row {typeColor(event.type)} animate-slide-in-up"
          style="animation-duration: 0.3s; animation-fill-mode: both;"
        >
          <span class="type-dot {typeColor(event.type)}"></span>
          <span class="type-text">{event.type.replace(/_/g, " ")}</span>
          <span class="source mono">{event.source}</span>
          <span class="time">{formatTimestamp(event.timestamp)}</span>
        </div>
      {/each}
    </div>
    <a class="view-all" href={resolve("/events")}>View all events &rarr;</a>
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
    font-size: 0.95rem;
    font-weight: var(--font-medium);
  }

  .sub {
    color: var(--color-text-tertiary);
    font-size: 0.85rem;
    margin: 4px 0 0 0;
  }

  .event-list {
    display: flex;
    flex-direction: column;
    gap: 10px;
  }

  .event-row {
    display: grid;
    grid-template-columns: 10px 1fr auto auto;
    gap: 12px;
    align-items: center;
    padding: 12px 16px;
    background: var(--color-bg-elevated);
    border: 1px solid var(--color-border-default);
    border-radius: var(--radius-lg);
    border-left: 4px solid var(--color-border-default);
    transition: var(--transition-all);
  }

  .event-row:hover {
    transform: translateY(-2px) scale(1.01);
    box-shadow: var(--shadow-md);
    background: var(--color-bg-hover);
    border-color: var(--color-border-strong);
  }

  .type-dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: var(--color-border-strong);
  }

  .event-row.adt {
    border-left-color: rgba(59, 130, 246, 0.7);
  }
  .event-row.lab {
    border-left-color: rgba(16, 185, 129, 0.7);
  }
  .event-row.appt {
    border-left-color: rgba(245, 158, 11, 0.7);
  }
  .event-row.claim {
    border-left-color: rgba(168, 85, 247, 0.7);
  }

  .type-dot.adt {
    background: rgba(59, 130, 246, 0.8);
    box-shadow: 0 0 8px rgba(59, 130, 246, 0.4);
  }
  .type-dot.lab {
    background: rgba(16, 185, 129, 0.8);
    box-shadow: 0 0 8px rgba(16, 185, 129, 0.4);
  }
  .type-dot.appt {
    background: rgba(245, 158, 11, 0.8);
    box-shadow: 0 0 8px rgba(245, 158, 11, 0.4);
  }
  .type-dot.claim {
    background: rgba(168, 85, 247, 0.8);
    box-shadow: 0 0 8px rgba(168, 85, 247, 0.4);
  }

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

  .mono {
    font-family: var(--font-mono);
  }

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

  /* Empty & Loading States */
  .empty-state {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    padding: 32px 16px;
    text-align: center;
    border: 1px dashed var(--color-border-strong);
    border-radius: var(--radius-xl);
    background: var(--color-bg-surface);
  }

  .empty-icon {
    font-size: 2rem;
    color: var(--color-border-focus);
    margin-bottom: 12px;
    opacity: 0.5;
  }

  .skeleton {
    opacity: 0.6;
    animation: pulse 2s cubic-bezier(0.4, 0, 0.6, 1) infinite;
  }

  .skeleton-dot {
    width: 10px;
    height: 10px;
    border-radius: 50%;
    background: var(--color-border-strong);
  }

  .skeleton-text {
    height: 12px;
    border-radius: 6px;
    background: var(--color-border-strong);
  }

  @keyframes pulse {
    0%,
    100% {
      opacity: 0.6;
    }
    50% {
      opacity: 0.3;
    }
  }
</style>
