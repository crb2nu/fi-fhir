<script lang="ts">
  /**
   * The one honest empty state for a live surface this deployment cannot
   * stream. Rendered instead of a spinner that never connects; the surface
   * that shows it does not open the subscription.
   *
   * `data-stream` names the GraphQL subscription root and `data-reason` says
   * whether streaming is off entirely or this root is not allowlisted, so the
   * browser smoke gate can assert both. Drawn with the design system's
   * EmptyState (one statement, a 16px icon, identifiers in mono).
   */
  import WifiOff from '@lucide/svelte/icons/wifi-off';
  import { EmptyState } from '$lib/ui/primitives';
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

<EmptyState
  icon={WifiOff}
  align="start"
  class={['streaming-unavailable', { 'is-compact': compact }]}
  role="status"
  data-testid="streaming-unavailable"
  data-stream={root}
  data-reason={reason}
>
  <span class="title">Live streaming for {subject} is not available on this deployment</span>
  <span class="detail">
    {#if reason === 'streaming-off'}
      The API has Integration Session streaming turned off
      (<code>FI_FHIR_INTEGRATION_SESSION_ENABLED</code>), so no live subscription can be opened.
    {:else if SESSION_ROOTS.has(root)}
      Streaming is on, but this identity's roles do not admit the
      <code>{root}</code> subscription.
    {:else}
      This deployment streams Integration Session runs only
      (<code>integrationSessionEvents</code>, <code>sessionRunEvents</code>);
      <code>{root}</code> is not an allowed subscription.
    {/if}
  </span>
  {#if alternative || !SESSION_ROOTS.has(root)}
    <span class="detail">
      {alternative}
      {#if !SESSION_ROOTS.has(root)}
        Integration Session runs stream in HL7 intake when the session workspace is enabled.
      {/if}
    </span>
  {/if}
</EmptyState>

<style>
  :global(.ui-empty.streaming-unavailable) {
    padding: var(--space-3);
    border: 1px solid var(--color-border-subtle);
    border-radius: var(--radius-sm);
    background: var(--color-bg-elevated);
  }

  :global(.ui-empty.streaming-unavailable.is-compact) {
    padding: var(--space-2) var(--space-3);
  }

  .title {
    display: block;
    color: var(--color-text-primary);
    font-weight: var(--font-medium);
  }

  .detail {
    display: block;
    margin-top: 2px;
    font-size: var(--text-xs);
    color: var(--color-text-secondary);
  }

  code {
    font-family: var(--font-mono);
    font-size: var(--text-mono);
    color: var(--color-text-primary);
    overflow-wrap: anywhere;
  }
</style>
