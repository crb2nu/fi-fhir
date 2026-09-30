<!--
  AuditDetail — one delivery audit record's `detail` document (failure code,
  scheduled retry, resubmit source …), as the server stored it. The audit trail
  is server-written and clinical-content-free; an empty document says so
  rather than rendering "{}".
-->
<script lang="ts">
  interface Props {
    detail: unknown;
  }

  let { detail }: Props = $props();

  const entries = $derived(
    detail && typeof detail === 'object' && !Array.isArray(detail)
      ? Object.entries(detail as Record<string, unknown>)
      : []
  );

  function show(value: unknown): string {
    if (value === null || value === undefined) return '—';
    return typeof value === 'string' ? value : JSON.stringify(value);
  }
</script>

{#if entries.length === 0}
  <span class="none">No detail recorded</span>
{:else}
  <span class="detail" data-testid="audit-detail">
    {#each entries as [key, value] (key)}
      <span class="pair"><span class="key">{key}</span> <code>{show(value)}</code></span>
    {/each}
  </span>
{/if}

<style>
  .none {
    color: var(--color-text-muted);
  }

  .detail {
    display: inline-flex;
    flex-wrap: wrap;
    gap: 0 var(--space-2);
    min-width: 0;
  }

  .pair {
    min-width: 0;
    overflow-wrap: anywhere;
  }

  .key {
    color: var(--color-text-tertiary);
  }

  code {
    font-family: var(--font-mono);
    font-size: var(--text-mono);
    color: var(--color-text-secondary);
  }
</style>
