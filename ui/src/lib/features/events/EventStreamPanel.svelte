<script lang="ts">
  /**
   * Events › Live Stream: a tail of the `eventStream` subscription.
   *
   * When the deployment cannot stream `eventStream` (streaming off, or the root
   * is not allowlisted) the panel renders the honest StreamingUnavailable state
   * and never opens the subscription.
   */
  import { onMount, onDestroy } from 'svelte';
  import Pause from '@lucide/svelte/icons/pause';
  import Play from '@lucide/svelte/icons/play';
  import RotateCw from '@lucide/svelte/icons/rotate-cw';
  import Radio from '@lucide/svelte/icons/radio';
  import Eraser from '@lucide/svelte/icons/eraser';
  import { subscribe as wsSubscribe } from '$lib/graphql/subscriptions';
  import { noteStreamError, streamStatus } from '$lib/graphql/streamAvailability';
  import { EventStreamDocument, type EventStreamSubscription, type EventFilter, type EventType } from '$lib/gen/graphql';
  import {
    Badge,
    Button,
    EmptyState,
    Input,
    Select,
    Table,
    Td,
    Th,
    Tr,
    type BadgeTone
  } from '$lib/ui/primitives';
  import StreamingUnavailable from '$lib/ui/StreamingUnavailable.svelte';
  import { EVENT_TYPES, formatEventClock } from './eventFormat';

  /** Maximum number of events to display in the list */
  export let maxEvents: number = 100;
  /** Optional initial source filter value */
  export let initialSource: string = '';
  /** Optional initial correlationId filter value */
  export let initialCorrelationId: string = '';
  /** What to point at when the deployment cannot stream events. */
  export let unavailableAlternative = 'Recorded events are in the Events browser.';

  type StreamEvent = EventStreamSubscription['eventStream'];

  // When the deployment cannot stream `eventStream`, render the honest state
  // and never open the subscription.
  const eventStreamStatus = streamStatus('eventStream');
  $: unavailable = $eventStreamStatus.availability === 'unavailable' ? $eventStreamStatus : null;
  $: if (unavailable && unsubscribe) stopSubscription();

  // Connection state
  let connected = false;
  let error: string | null = null;
  let events: StreamEvent[] = [];
  let unsubscribe: (() => void) | null = null;

  // Filtering
  let filterType: EventType | '' = '';
  let filterSource: string = initialSource;
  let filterCorrelationId: string = initialCorrelationId;
  let paused = false;

  const typeOptions = [
    { value: '', label: 'All types' },
    ...EVENT_TYPES.map((type) => ({ value: type, label: type }))
  ];

  function startSubscription() {
    if (unsubscribe) {
      unsubscribe();
      unsubscribe = null;
    }
    if ($eventStreamStatus.availability === 'unavailable') return;

    error = null;
    connected = false;

    // Build filter based on current settings
    const typeFilterValue = filterType || null;
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
          connected = false;
          // A 404 or a refused root means this deployment cannot stream
          // events at all: flip to the honest state instead of an error.
          if (noteStreamError('eventStream', err)) {
            error = null;
            return;
          }
          error = err.message;
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

  let connectionTone: BadgeTone = 'neutral';
  $: connectionTone = error ? 'danger' : connected ? 'success' : 'neutral';
  $: connectionLabel = error ? 'Error' : connected ? 'Connected' : 'Connecting…';
</script>

<div class="stream">
  {#if unavailable}
    <div class="unavailable">
      <StreamingUnavailable
        root="eventStream"
        subject="the event stream"
        reason={unavailable.reason}
        alternative={unavailableAlternative}
      />
    </div>
  {:else}
    <div class="filters" role="group" aria-label="Stream filters">
      <Badge tone={connectionTone} dot data-testid="stream-state">{connectionLabel}</Badge>
      <div class="filter filter-type">
        <Select aria-label="Event type" bind:value={filterType} options={typeOptions} />
      </div>
      <div class="filter">
        <Input aria-label="Filter by source" bind:value={filterSource} placeholder="Source" mono />
      </div>
      <div class="filter">
        <Input
          aria-label="Filter by correlation ID"
          bind:value={filterCorrelationId}
          placeholder="Correlation id"
          mono
        />
      </div>
      <span class="spacer"></span>
      {#if paused}
        <Badge tone="warning">Paused</Badge>
      {/if}
      <Badge mono>{events.length} / {maxEvents}</Badge>
      <Button variant="ghost" icon={paused ? Play : Pause} onclick={togglePause}>
        {paused ? 'Resume' : 'Pause'}
      </Button>
      <Button variant="ghost" icon={Eraser} onclick={clearEvents}>Clear</Button>
      <Button variant="ghost" icon={RotateCw} onclick={startSubscription}>Reconnect</Button>
    </div>

    {#if error}
      <p class="stream-error" role="alert">{error}</p>
    {/if}

    {#if events.length === 0}
      <EmptyState
        icon={Radio}
        message={connected
          ? 'Waiting for events. They appear here as they stream.'
          : error
            ? 'The event stream is not connected.'
            : 'Connecting to the event stream.'}
      />
    {:else}
      <Table label="Streamed events" layout="fixed" class="stream-table">
        {#snippet head()}
          <tr>
            <Th width="88px">Time</Th>
            <Th width="176px">Type</Th>
            <Th width="140px">Source</Th>
            <Th>Correlation id</Th>
            <Th>Event id</Th>
          </tr>
        {/snippet}
        {#each events as event (event.id)}
          <Tr>
            <Td mono muted value={formatEventClock(event.timestamp)} />
            <Td mono truncate value={event.type} />
            <Td mono truncate value={event.source} />
            <Td mono truncate muted value={event.correlationId ?? '—'} />
            <Td mono truncate muted value={event.id} />
          </Tr>
        {/each}
      </Table>
    {/if}
  {/if}
</div>

<style>
  .stream {
    display: flex;
    flex-direction: column;
    flex: 1 1 auto;
    min-height: 0;
  }

  .unavailable {
    padding: var(--space-3);
  }

  .filters {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    padding: var(--space-2) var(--space-3);
    border-bottom: 1px solid var(--color-border-subtle);
  }

  .filter {
    width: 180px;
  }

  .filter-type {
    width: 200px;
  }

  .spacer {
    flex: 1 1 auto;
  }

  .stream-error {
    margin: 0;
    padding: var(--space-2) var(--space-3);
    border-bottom: 1px solid var(--color-danger-border);
    background: var(--color-danger-bg);
    color: var(--color-danger-text);
    font-size: var(--text-xs);
  }

  .stream :global(.stream-table) {
    flex: 0 1 auto;
    min-height: 0;
  }
</style>
