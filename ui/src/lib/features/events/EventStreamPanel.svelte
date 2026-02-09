<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { subscribe as wsSubscribe } from '$lib/graphql/subscriptions';
  import { EventStreamDocument, type EventStreamSubscription, type EventFilter, type EventType } from '$lib/gen/graphql';
  import Button from '$lib/ui/Button.svelte';

  /** Maximum number of events to display in the list */
  export let maxEvents: number = 100;
  /** Optional initial source filter value */
  export let initialSource: string = '';
  /** Optional initial correlationId filter value */
  export let initialCorrelationId: string = '';

  type StreamEvent = EventStreamSubscription['eventStream'];

  // Connection state
  let connected = false;
  let error: string | null = null;
  let events: StreamEvent[] = [];
  let unsubscribe: (() => void) | null = null;

  // Filtering
  let filterType: EventType | 'ALL' = 'ALL';
  let filterSource: string = initialSource;
  let filterCorrelationId: string = initialCorrelationId;
  let paused = false;

  // Available event types for filter dropdown
  const eventTypes: EventType[] = [
    'PATIENT_ADMIT',
    'PATIENT_DISCHARGE',
    'PATIENT_TRANSFER',
    'PATIENT_UPDATE',
    'LAB_RESULT',
    'LAB_ORDERED',
    'APPOINTMENT_SCHEDULED',
    'APPOINTMENT_CANCELLED',
    'APPOINTMENT_NOSHOW',
    'CLAIM_SUBMITTED',
    'CLAIM_ADJUDICATED',
    'VITAL_SIGN',
    'CONDITION',
    'PROCEDURE',
    'IMMUNIZATION',
    'DOCUMENT'
  ];

  function startSubscription() {
    if (unsubscribe) {
      unsubscribe();
    }

    error = null;
    connected = false;

    // Build filter based on current settings
    const typeFilterValue = filterType !== 'ALL' ? filterType : null;
    const sourceFilterValue = filterSource.trim() || null;
    const correlationIdValue = filterCorrelationId.trim() || null;
    const filter: EventFilter | null = typeFilterValue || sourceFilterValue || correlationIdValue
      ? {
          types: typeFilterValue ? [typeFilterValue] : null,
          sources: sourceFilterValue ? [sourceFilterValue] : null,
          patientMrn: null,
          correlationId: correlationIdValue,
          fromTimestamp: null,
          toTimestamp: null
        }
      : null;

    unsubscribe = wsSubscribe(
      EventStreamDocument,
      { filter },
      {
        onData: (data) => {
          connected = true;
          if (!paused && data.eventStream) {
            events = [data.eventStream, ...events].slice(0, maxEvents);
          }
        },
        onError: (err) => {
          error = err.message;
          connected = false;
        },
        onComplete: () => {
          connected = false;
        }
      }
    );
  }

  function stopSubscription() {
    if (unsubscribe) {
      unsubscribe();
      unsubscribe = null;
    }
    connected = false;
  }

  function clearEvents() {
    events = [];
  }

  function togglePause() {
    paused = !paused;
  }

  function formatTimestamp(ts: string): string {
    try {
      return new Date(ts).toLocaleTimeString();
    } catch {
      return ts;
    }
  }

  function formatEventType(type: EventType): string {
    return type.replace(/_/g, ' ');
  }

  function typeColor(type: EventType): string {
    switch (type) {
      case 'PATIENT_ADMIT':
      case 'PATIENT_DISCHARGE':
      case 'PATIENT_TRANSFER':
      case 'PATIENT_UPDATE':
        return 'adt';
      case 'LAB_RESULT':
      case 'LAB_ORDERED':
        return 'lab';
      case 'APPOINTMENT_SCHEDULED':
      case 'APPOINTMENT_CANCELLED':
      case 'APPOINTMENT_NOSHOW':
        return 'appt';
      case 'CLAIM_SUBMITTED':
      case 'CLAIM_ADJUDICATED':
        return 'claim';
      default:
        return 'default';
    }
  }

  onMount(() => {
    // Prevent an immediate resubscribe after the initial connect.
    lastFilterKey = `${filterType}|${filterSource.trim()}|${filterCorrelationId.trim()}`;
    startSubscription();
  });

  onDestroy(() => {
    stopSubscription();
  });

  // Restart subscription when filters change
  let lastFilterKey = '';
  $: {
    const key = `${filterType}|${filterSource.trim()}|${filterCorrelationId.trim()}`;
    if (unsubscribe && key !== lastFilterKey) {
      lastFilterKey = key;
      startSubscription();
    }
  }
</script>

<div class="panel">
  <div class="controls">
    <div class="status">
      <span class="indicator" class:connected class:error={!!error}></span>
      {#if error}
        <span class="status-text error">{error}</span>
      {:else if connected}
        <span class="status-text">Connected</span>
      {:else}
        <span class="status-text">Connecting...</span>
      {/if}
    </div>

    <div class="actions">
      <Button variant="secondary" on:click={togglePause}>
        {paused ? 'Resume' : 'Pause'}
      </Button>
      <Button variant="secondary" on:click={clearEvents}>
        Clear
      </Button>
      <Button variant="secondary" on:click={startSubscription}>
        Reconnect
      </Button>
    </div>
  </div>

  <div class="filters">
    <label class="filter">
      Event Type
      <select class="select" bind:value={filterType}>
        <option value="ALL">All Types</option>
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
        placeholder="Filter by source..."
      />
    </label>

    <label class="filter">
      Correlation ID
      <input
        aria-label="Filter by correlation ID"
        type="text"
        class="input"
        bind:value={filterCorrelationId}
        placeholder="Filter by correlationId..."
      />
    </label>
  </div>

  {#if events.length === 0}
    <div class="empty">
      {#if connected}
        Waiting for events... Events will appear here as they stream in real-time.
      {:else if error}
        Unable to connect to event stream. Check that the backend is running.
      {:else}
        Connecting to event stream...
      {/if}
    </div>
  {:else}
    <div class="event-list">
      {#each events as event (event.id)}
        <div class="event-row">
          <span class="time mono">{formatTimestamp(event.timestamp)}</span>
          <span class="type-pill {typeColor(event.type)}">{formatEventType(event.type)}</span>
          <span class="source mono">{event.source}</span>
          <span class="corr muted mono" title={event.correlationId ?? ''}>
            {event.correlationId ? `${event.correlationId.slice(0, 10)}…` : '-'}
          </span>
          <span class="id muted mono" title={event.id}>{event.id.slice(0, 8)}...</span>
        </div>
      {/each}
    </div>

    <div class="footer muted">
      Showing {events.length} of {maxEvents} max events
      {#if paused}
        <span class="paused-badge">PAUSED</span>
      {/if}
    </div>
  {/if}
</div>

<style>
  .panel {
    display: grid;
    gap: 12px;
  }

  .controls {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 12px;
    flex-wrap: wrap;
  }

  .status {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .indicator {
    width: 10px;
    height: 10px;
    border-radius: 50%;
    background: rgba(156, 163, 175, 0.5);
    transition: background 0.2s ease;
  }

  .indicator.connected {
    background: rgba(16, 185, 129, 0.85);
    box-shadow: 0 0 8px rgba(16, 185, 129, 0.4);
  }

  .indicator.error {
    background: rgba(239, 68, 68, 0.85);
    box-shadow: 0 0 8px rgba(239, 68, 68, 0.4);
  }

  .status-text {
    font-weight: 700;
    color: var(--color-text-secondary);
  }

  .status-text.error {
    color: rgba(254, 202, 202, 0.9);
  }

  .actions {
    display: flex;
    gap: 8px;
  }

  .filters {
    display: flex;
    gap: 12px;
    flex-wrap: wrap;
  }

  .filter {
    display: grid;
    gap: 6px;
    color: var(--color-text-secondary);
    font-size: 0.9rem;
    font-weight: 700;
    min-width: 180px;
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

  .empty {
    color: var(--color-text-tertiary);
    padding: 24px;
    text-align: center;
    border: 1px dashed var(--color-border-strong);
    border-radius: 12px;
  }

  .event-list {
    display: grid;
    gap: 6px;
    max-height: 400px;
    overflow-y: auto;
  }

  .event-row {
    display: grid;
    grid-template-columns: 80px auto 1fr 140px auto;
    gap: 12px;
    align-items: center;
    padding: 8px 12px;
    border-radius: 8px;
    border: 1px solid var(--color-border-default);
    background: var(--color-bg-elevated);
  }

  .event-row:hover {
    background: var(--color-bg-hover);
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

  .type-pill.adt {
    background: rgba(59, 130, 246, 0.15);
    border: 1px solid rgba(59, 130, 246, 0.3);
    color: rgba(147, 197, 253, 0.95);
  }

  .type-pill.lab {
    background: rgba(16, 185, 129, 0.15);
    border: 1px solid rgba(16, 185, 129, 0.3);
    color: rgba(110, 231, 183, 0.95);
  }

  .type-pill.appt {
    background: rgba(245, 158, 11, 0.15);
    border: 1px solid rgba(245, 158, 11, 0.3);
    color: rgba(253, 230, 138, 0.95);
  }

  .type-pill.claim {
    background: rgba(168, 85, 247, 0.15);
    border: 1px solid rgba(168, 85, 247, 0.3);
    color: rgba(216, 180, 254, 0.95);
  }

  .type-pill.default {
    background: var(--color-bg-surface);
    border: 1px solid var(--color-border-strong);
    color: var(--color-text-secondary);
  }

  .source {
    color: var(--color-text-secondary);
    font-size: 0.85rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .corr {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: 0.85rem;
  }

  .id {
    font-size: 0.8rem;
  }

  .mono {
    font-family: var(--font-mono);
  }

  .muted {
    color: var(--color-text-muted);
  }

  .footer {
    display: flex;
    justify-content: space-between;
    align-items: center;
    font-size: 0.85rem;
    padding-top: 8px;
    border-top: 1px solid var(--color-border-default);
  }

  .paused-badge {
    padding: 2px 8px;
    border-radius: 4px;
    background: rgba(245, 158, 11, 0.2);
    border: 1px solid rgba(245, 158, 11, 0.4);
    color: rgba(253, 230, 138, 0.95);
    font-weight: 700;
    font-size: 0.75rem;
  }
</style>
