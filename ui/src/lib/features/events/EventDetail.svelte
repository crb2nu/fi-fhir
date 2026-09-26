<!--
  EventDetail — the details pane beside the Events table: the record's fields
  as KeyValue, then the record exactly as the API returned it. The browse
  query selects the Event interface only (no clinical payload), so the viewer
  shows those fields and nothing more.
-->
<script lang="ts">
  import Copy from '@lucide/svelte/icons/copy';
  import X from '@lucide/svelte/icons/x';
  import type { EventsQuery } from '$lib/gen/graphql';
  import { IconButton, KeyValue } from '$lib/ui/primitives';
  import { formatEventTime } from './eventFormat';

  type EventNode = EventsQuery['events']['edges'][number]['node'];

  interface Props {
    event: EventNode;
    onClose?: (() => void) | undefined;
  }

  let { event, onClose }: Props = $props();

  const payload = $derived(JSON.stringify(event, null, 2));

  function copyId(): void {
    void navigator.clipboard?.writeText(event.id).catch(() => {});
  }
</script>

<div class="detail" data-testid="event-detail">
  <div class="detail-head">
    <span class="detail-title">{event.type}</span>
    <span class="detail-actions">
      <IconButton icon={Copy} label="Copy event id" onclick={copyId} />
      {#if onClose}
        <IconButton icon={X} label="Close details" onclick={onClose} />
      {/if}
    </span>
  </div>

  <KeyValue
    items={[
      { key: 'Event id', value: event.id, mono: true, truncate: true },
      { key: 'Time', value: formatEventTime(event.timestamp), mono: true },
      { key: 'Source', value: event.source, mono: true },
      { key: 'Format', value: event.sourceFormat },
      { key: 'Correlation id', value: event.correlationId, mono: true, truncate: true }
    ]}
  />

  <div class="payload-block">
    <span class="text-label">Record</span>
    <pre class="payload" aria-label="Event record as returned by the API">{payload}</pre>
  </div>
</div>

<style>
  .detail {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    min-width: 0;
  }

  .detail-head {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    min-width: 0;
  }

  .detail-title {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-family: var(--font-mono);
    font-size: var(--text-ui);
    font-weight: var(--font-semibold);
    color: var(--color-text-primary);
  }

  .detail-actions {
    display: flex;
    margin-left: auto;
  }

  .payload-block {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
    min-height: 0;
  }

  .payload {
    margin: 0;
    padding: var(--space-2);
    max-height: 320px;
    overflow: auto;
    background: var(--color-bg-input);
    border: 1px solid var(--color-border-subtle);
    border-radius: var(--radius-sm);
    font-family: var(--font-mono);
    font-size: var(--text-mono);
    line-height: var(--leading-ui);
    color: var(--color-text-secondary);
    white-space: pre;
  }
</style>
