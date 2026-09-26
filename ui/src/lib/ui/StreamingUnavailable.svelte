<script lang="ts">
  /**
   * The one honest empty state for a live surface this deployment cannot
   * stream. Rendered instead of a spinner that never connects; the surface
   * that shows it does not open the subscription.
   *
   * `data-stream` names the GraphQL subscription root and `data-reason` says
   * whether streaming is off entirely or this root is not allowlisted, so the
   * browser smoke gate can assert both.
   */
  import type { StreamRoot, StreamUnavailableReason } from '$lib/graphql/streamAvailability';

  /** The subscription root the surface would have opened. */
  export let root: StreamRoot;
  /** What would have streamed, e.g. "the event stream", "debug steps". */
  export let subject: string;
  export let reason: StreamUnavailableReason;
  /** What the user can do instead, on this surface. */
  export let alternative = '';
  export let compact = false;

  const SESSION_ROOTS: ReadonlySet<StreamRoot> = new Set([
    'integrationSessionEvents',
    'sessionRunEvents'
  ]);
</script>

<div
  class="streaming-unavailable"
  class:compact
  role="status"
  data-testid="streaming-unavailable"
  data-stream={root}
  data-reason={reason}
>
  <svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" aria-hidden="true">
    <path d="M4.9 19.1a10 10 0 0 1 0-14.2M19.1 4.9a10 10 0 0 1 0 14.2" />
    <path d="M7.8 16.2a6 6 0 0 1 0-8.4M16.2 7.8a6 6 0 0 1 0 8.4" />
    <path d="M3 3l18 18" />
  </svg>
  <div class="copy">
    <p class="title">Live streaming for {subject} is not available on this deployment</p>
    <p class="detail">
      {#if reason === 'streaming-off'}
        The API has Integration Session streaming turned off
        (<code>FI_FHIR_INTEGRATION_SESSION_ENABLED</code>), so no live subscription can be opened.
      {:else}
        This deployment streams Integration Session runs only
        (<code>integrationSessionEvents</code>, <code>sessionRunEvents</code>);
        <code>{root}</code> is not an allowed subscription.
      {/if}
    </p>
    {#if alternative || !SESSION_ROOTS.has(root)}
      <p class="detail">
        {alternative}
        {#if !SESSION_ROOTS.has(root)}
          Integration Session runs stream in HL7 intake when the session workspace is enabled.
        {/if}
      </p>
    {/if}
  </div>
</div>

<style>
  .streaming-unavailable {
    display: flex;
    align-items: flex-start;
    gap: var(--space-3);
    padding: var(--space-4);
    border: 1px dashed var(--color-border-strong);
    border-radius: var(--radius-lg);
    background: var(--color-bg-elevated);
    color: var(--color-text-secondary);
  }

  .streaming-unavailable.compact {
    padding: var(--space-2) var(--space-3);
  }

  .icon {
    width: 20px;
    height: 20px;
    flex: 0 0 auto;
    margin-top: 2px;
    color: var(--color-text-tertiary);
  }

  .copy {
    display: grid;
    gap: var(--space-1);
    min-width: 0;
  }

  .title {
    margin: 0;
    font-size: var(--text-sm);
    font-weight: var(--font-semibold);
    color: var(--color-text-primary);
  }

  .detail {
    margin: 0;
    font-size: var(--text-xs);
    line-height: var(--leading-relaxed);
  }

  code {
    font-family: var(--font-mono);
    font-size: var(--text-2xs);
    overflow-wrap: anywhere;
  }
</style>
