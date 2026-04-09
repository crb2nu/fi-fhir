<script lang="ts">
  import { onMount } from 'svelte';
  import { resolve } from '$app/paths';
  import { listPendingAutoroutes } from '$lib/features/terminology/terminologyApi';
  import type { ListPendingAutoroutesQuery } from '$lib/gen/graphql';
  import Panel from '$lib/ui/Panel.svelte';
  import Badge from '$lib/ui/Badge.svelte';
  import Button from '$lib/ui/Button.svelte';

  type PendingNode = ListPendingAutoroutesQuery['listPendingAutoroutes']['nodes'][number];

  let unmapped: PendingNode[] = [];
  let loading = true;
  let error: string | null = null;

  async function load() {
    loading = true;
    error = null;
    try {
      const result = await listPendingAutoroutes({
        status: 'PENDING',
        first: 5,
        offset: 0,
        minConfidence: 0,
        sourceSystem: '',
        targetSystem: ''
      });
      unmapped = result.nodes;
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to load unmapped codes';
    } finally {
      loading = false;
    }
  }

  onMount(load);
</script>

<Panel title="Terminology Coverage" padding="md">
  <div class="unmapped-container">
    <div class="header-row">
      <span class="sub-title">Recent Unmapped Codes</span>
      <Badge variant="warning" size="sm">Action Required</Badge>
    </div>

    {#if loading}
      <div class="loading">Loading unmapped codes...</div>
    {:else if error}
      <div class="error">{error}</div>
    {:else if unmapped.length === 0}
      <div class="empty">No unmapped codes detected.</div>
    {:else}
      <div class="code-list">
        {#each unmapped as item (item.id)}
          <div class="code-item">
            <div class="code-info">
              <span class="code mono">{item.sourceCode}</span>
              <span class="system">{item.sourceSystem}</span>
            </div>
            <div class="code-suggest">
              <span class="suggest-label">AI Suggestion:</span>
              <span class="suggest-value">{item.suggestedCode} ({Math.round(item.confidence * 100)}%)</span>
            </div>
            <a href={resolve('/terminology')} class="resolve-link">Review</a>
          </div>
        {/each}
      </div>
      <div class="actions">
        <Button variant="ghost" size="sm" href={resolve('/terminology')}>
          View all pending reviews
        </Button>
      </div>
    {/if}
  </div>
</Panel>

<style>
  .unmapped-container {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
  }

  .header-row {
    display: flex;
    justify-content: space-between;
    align-items: center;
  }

  .sub-title {
    font-size: var(--text-sm);
    font-weight: var(--font-semibold);
    color: var(--color-text-secondary);
    text-transform: uppercase;
    letter-spacing: var(--tracking-wider);
  }

  .loading, .empty, .error {
    font-size: var(--text-sm);
    color: var(--color-text-tertiary);
    text-align: center;
    padding: var(--space-4) 0;
  }

  .error { color: var(--color-danger); }

  .code-list {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  .code-item {
    display: grid;
    grid-template-columns: 1fr 1fr auto;
    align-items: center;
    gap: var(--space-3);
    padding: var(--space-2) var(--space-3);
    background: var(--color-bg-surface);
    border: 1px solid var(--color-border-subtle);
    border-radius: var(--radius-md);
  }

  .code-info {
    display: flex;
    flex-direction: column;
  }

  .code {
    font-size: var(--text-sm);
    font-weight: var(--font-bold);
    color: var(--color-text-primary);
  }

  .system {
    font-size: var(--text-xs);
    color: var(--color-text-muted);
  }

  .code-suggest {
    display: flex;
    flex-direction: column;
  }

  .suggest-label {
    font-size: 10px;
    color: var(--color-text-tertiary);
    text-transform: uppercase;
  }

  .suggest-value {
    font-size: var(--text-xs);
    color: var(--color-success);
    font-weight: 600;
  }

  .resolve-link {
    font-size: var(--text-xs);
    font-weight: var(--font-bold);
    color: var(--color-primary);
    text-decoration: none;
  }

  .resolve-link:hover {
    text-decoration: underline;
  }

  .actions {
    display: flex;
    justify-content: center;
    margin-top: var(--space-2);
  }

  .mono { font-family: var(--font-mono); }
</style>
