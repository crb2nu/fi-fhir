<script lang="ts">
  /**
   * The Delivery block: what this process's destination provenance ledger
   * recorded for one delivery attempt, newest first (Slice 4.2c).
   *
   * It renders exactly the transport, the outcome, the FHIR resource types and
   * entry count, the OperationOutcome issue codes, and the declared endpoint.
   * The ledger is clinical-content-free by construction and this view keeps it
   * that way: it has no access to the payload, a response body, or diagnostics
   * text, because none of them was ever recorded.
   */

  import Badge from '$lib/ui/Badge.svelte';
  import { describeDestinationDelivery, type DestinationDeliveryLike } from './attemptPresentation';

  export let deliveries: readonly DestinationDeliveryLike[] = [];
  /** Accessible name of the list, so several blocks on a page stay distinct. */
  export let label = 'Destination deliveries';
</script>

{#if deliveries.length === 0}
  <p class="empty">
    No destination delivery recorded — this process contacted no destination for this attempt.
  </p>
{:else}
  <ol class="deliveries" aria-label={label}>
    {#each deliveries as delivery, index (index)}
      {@const display = describeDestinationDelivery(delivery)}
      <li class="delivery">
        <div class="summary">
          <Badge variant="primary" size="sm" mono>{display.transportLabel}</Badge>
          <Badge variant={display.outcomeVariant} size="sm">{display.outcomeLabel}</Badge>
          {#if display.entryCountText}
            <span class="muted">{display.entryCountText}</span>
          {/if}
        </div>
        {#if display.resourceTypes.length > 0}
          <ul class="chips" aria-label="FHIR resource types">
            {#each display.resourceTypes as resourceType (resourceType)}
              <li class="chip">{resourceType}</li>
            {/each}
          </ul>
        {/if}
        {#if display.outcomeCodes.length > 0}
          <ul class="chips" aria-label="OperationOutcome issue codes">
            {#each display.outcomeCodes as code (code)}
              <li class="chip code">{code}</li>
            {/each}
          </ul>
        {/if}
        <p class="endpoint" title="Declared by the destination revision; advisory, never a trust input">
          <span class="sr-only">Declared endpoint:</span>
          {display.endpointText}
        </p>
      </li>
    {/each}
  </ol>
{/if}

<style>
  .empty {
    margin: 0;
    font-size: var(--text-xs);
    color: var(--color-text-tertiary);
  }

  .deliveries {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  .delivery {
    border-left: 2px solid var(--color-border-subtle);
    padding-left: var(--space-2);
  }

  .summary {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
    align-items: center;
  }

  .chips {
    list-style: none;
    margin: var(--space-1) 0 0;
    padding: 0;
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-1);
  }

  .chip {
    background: var(--color-bg-surface);
    border: 1px solid var(--color-border-subtle);
    border-radius: var(--radius-sm);
    padding: 0 var(--space-2);
    font-family: var(--font-mono);
    font-size: var(--text-2xs);
    color: var(--color-text-secondary);
  }

  .chip.code {
    color: var(--color-danger-text);
  }

  .endpoint {
    margin: var(--space-1) 0 0;
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    color: var(--color-text-tertiary);
    overflow-wrap: anywhere;
  }

  .muted {
    font-size: var(--text-xs);
    color: var(--color-text-tertiary);
  }

  .sr-only {
    position: absolute;
    width: 1px;
    height: 1px;
    padding: 0;
    margin: -1px;
    overflow: hidden;
    clip: rect(0, 0, 0, 0);
    white-space: nowrap;
    border: 0;
  }
</style>
