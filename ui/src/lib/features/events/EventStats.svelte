<script lang="ts">
  import { onMount } from 'svelte';
  import { getEventStatistics } from './eventsApi';
  import type { EventStatisticsQuery } from '$lib/gen/graphql';
  import Panel from '$lib/ui/Panel.svelte';
  import EmptyState from '$lib/ui/EmptyState.svelte';
  import Button from '$lib/ui/Button.svelte';

  type Stats = EventStatisticsQuery['eventStatistics'];

  let stats: Stats | null = null;
  let loading = true;
  let error: string | null = null;

  async function load() {
    loading = true;
    error = null;
    try {
      stats = await getEventStatistics();
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to load statistics';
    } finally {
      loading = false;
    }
  }

  onMount(load);
</script>

<div class="stats-page">
  {#if error}
    <EmptyState icon="error" title="Failed to load statistics" description={error}>
      <Button variant="secondary" on:click={load}>Retry</Button>
    </EmptyState>
  {:else if loading}
    <div class="loading">Loading statistics...</div>
  {:else if stats}
    <div class="stat-cards">
      <div class="stat-card accent">
        <span class="stat-value">{stats.totalEvents.toLocaleString()}</span>
        <span class="stat-label">Total Events</span>
      </div>
      <div class="stat-card">
        <span class="stat-value">{stats.byType.length}</span>
        <span class="stat-label">Event Types</span>
      </div>
      <div class="stat-card">
        <span class="stat-value">{stats.bySource.length}</span>
        <span class="stat-label">Sources</span>
      </div>
    </div>

    <div class="breakdown-grid">
      <Panel title="By Event Type">
        {#if stats.byType.length === 0}
          <p class="muted">No events recorded yet.</p>
        {:else}
          <div class="breakdown-list">
            {#each stats.byType.sort((a, b) => b.count - a.count) as item (item.eventType)}
              <div class="breakdown-row">
                <span class="breakdown-label">{item.eventType.replace(/_/g, ' ')}</span>
                <span class="breakdown-count mono">{item.count.toLocaleString()}</span>
                <div class="breakdown-bar">
                  <div
                    class="breakdown-fill"
                    style="width: {Math.max(2, (item.count / stats.totalEvents) * 100)}%"
                  ></div>
                </div>
              </div>
            {/each}
          </div>
        {/if}
      </Panel>

      <Panel title="By Source">
        {#if stats.bySource.length === 0}
          <p class="muted">No events recorded yet.</p>
        {:else}
          <div class="breakdown-list">
            {#each stats.bySource.sort((a, b) => b.count - a.count) as item (item.source)}
              <div class="breakdown-row">
                <span class="breakdown-label mono">{item.source}</span>
                <span class="breakdown-count mono">{item.count.toLocaleString()}</span>
                <div class="breakdown-bar">
                  <div
                    class="breakdown-fill source"
                    style="width: {Math.max(2, (item.count / stats.totalEvents) * 100)}%"
                  ></div>
                </div>
              </div>
            {/each}
          </div>
        {/if}
      </Panel>
    </div>
  {/if}
</div>

<style>
  .stats-page {
    display: grid;
    gap: 16px;
  }

  .loading {
    color: var(--color-text-tertiary);
    text-align: center;
    padding: 24px;
  }

  .stat-cards {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));
    gap: 12px;
  }

  .stat-card {
    padding: 16px;
    border-radius: 12px;
    border: 1px solid var(--color-border-default);
    background: var(--color-bg-elevated);
    text-align: center;
    display: grid;
    gap: 4px;
  }

  .stat-card.accent {
    border-color: rgba(59, 130, 246, 0.3);
    background: rgba(59, 130, 246, 0.08);
  }

  .stat-value {
    font-size: 1.8rem;
    font-weight: 700;
    color: var(--color-text-primary);
    font-family: var(--font-mono);
  }

  .stat-label {
    font-size: 0.85rem;
    color: var(--color-text-tertiary);
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 0.05em;
  }

  .breakdown-grid {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 12px;
  }

  @media (max-width: 768px) {
    .breakdown-grid {
      grid-template-columns: 1fr;
    }
  }

  .breakdown-list {
    display: grid;
    gap: 8px;
  }

  .breakdown-row {
    display: grid;
    grid-template-columns: 1fr auto auto;
    gap: 8px;
    align-items: center;
  }

  .breakdown-label {
    font-size: 0.85rem;
    color: var(--color-text-secondary);
    text-transform: capitalize;
  }

  .breakdown-count {
    font-size: 0.85rem;
    color: var(--color-text-primary);
    font-weight: 700;
  }

  .breakdown-bar {
    width: 60px;
    height: 6px;
    border-radius: 3px;
    background: var(--color-bg-surface);
    overflow: hidden;
  }

  .breakdown-fill {
    height: 100%;
    border-radius: 3px;
    background: rgba(59, 130, 246, 0.6);
    transition: width 0.3s ease;
  }

  .breakdown-fill.source {
    background: rgba(16, 185, 129, 0.6);
  }

  .mono { font-family: var(--font-mono); }
  .muted { color: var(--color-text-muted); margin: 0; }
</style>
