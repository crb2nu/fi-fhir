<script lang="ts">
  import type { EventsQuery } from '$lib/gen/graphql';
  import Badge from '$lib/ui/Badge.svelte';
  import Button from '$lib/ui/Button.svelte';

  type EventNode = EventsQuery['events']['edges'][number]['node'];

  export let event: EventNode | null = null;
  export let onClose: () => void = () => {};

  function formatTimestamp(ts: string): string {
    try {
      return new Date(ts).toLocaleString();
    } catch {
      return ts;
    }
  }

  function formatType(type: string): string {
    return type.replace(/_/g, ' ');
  }
</script>

{#if event}
  <div class="detail-panel">
    <div class="detail-header">
      <h3 class="detail-title">Event Detail</h3>
      <Button variant="ghost" size="sm" on:click={onClose}>Close</Button>
    </div>

    <div class="field-grid">
      <div class="field">
        <span class="label">ID</span>
        <span class="value mono">{event.id}</span>
      </div>

      <div class="field">
        <span class="label">Type</span>
        <span class="value"><Badge variant="info">{formatType(event.type)}</Badge></span>
      </div>

      <div class="field">
        <span class="label">Timestamp</span>
        <span class="value">{formatTimestamp(event.timestamp)}</span>
      </div>

      <div class="field">
        <span class="label">Source</span>
        <span class="value mono">{event.source}</span>
      </div>

      {#if event.sourceFormat}
        <div class="field">
          <span class="label">Format</span>
          <span class="value"><Badge variant="default" size="sm">{event.sourceFormat}</Badge></span>
        </div>
      {/if}

      {#if event.correlationId}
        <div class="field">
          <span class="label">Correlation ID</span>
          <span class="value mono">{event.correlationId}</span>
        </div>
      {/if}
    </div>
  </div>
{/if}

<style>
  .detail-panel {
    padding: 16px;
    border: 1px solid var(--color-border-default);
    border-radius: 12px;
    background: var(--color-bg-elevated);
    display: grid;
    gap: 16px;
  }

  .detail-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
  }

  .detail-title {
    margin: 0;
    color: var(--color-text-primary);
    font-size: 1rem;
  }

  .field-grid {
    display: grid;
    gap: 12px;
  }

  .field {
    display: grid;
    gap: 4px;
  }

  .label {
    color: var(--color-text-tertiary);
    font-size: 0.8rem;
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 0.05em;
  }

  .value {
    color: var(--color-text-primary);
    font-size: 0.9rem;
    word-break: break-all;
  }

  .mono {
    font-family: var(--font-mono);
  }
</style>
