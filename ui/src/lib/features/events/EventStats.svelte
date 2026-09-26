<!--
  Events › Statistics: the event store's aggregate counts. Totals in one
  KeyValue strip, then counts by type and by source as tables with
  right-aligned numbers and a share column.
-->
<script lang="ts">
  import { onMount } from 'svelte';
  import CircleAlert from '@lucide/svelte/icons/circle-alert';
  import RefreshCw from '@lucide/svelte/icons/refresh-cw';
  import type { EventStatisticsQuery } from '$lib/gen/graphql';
  import {
    EmptyState,
    IconButton,
    KeyValue,
    Panel,
    Table,
    Td,
    Th,
    Tr
  } from '$lib/ui/primitives';
  import { getEventStatistics } from './eventsApi';

  type Stats = EventStatisticsQuery['eventStatistics'];

  let stats = $state<Stats | null>(null);
  let loading = $state(true);
  let error = $state<string | null>(null);

  async function load(): Promise<void> {
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

  const byType = $derived(stats ? [...stats.byType].sort((a, b) => b.count - a.count) : []);
  const bySource = $derived(stats ? [...stats.bySource].sort((a, b) => b.count - a.count) : []);

  function share(count: number): string {
    if (!stats || stats.totalEvents === 0) return '—';
    return `${((count / stats.totalEvents) * 100).toFixed(1)}%`;
  }

  onMount(load);
</script>

<div class="stats">
  {#if error}
    <EmptyState
      icon={CircleAlert}
      message={`Statistics could not be loaded: ${error}`}
      actionLabel="Retry"
      onaction={load}
    />
  {:else}
    <Panel title="Totals">
      {#snippet actions()}
        <IconButton icon={RefreshCw} label="Refresh statistics" {loading} onclick={load} />
      {/snippet}
      <KeyValue
        columns={2}
        items={[
          { key: 'Events', value: stats ? stats.totalEvents.toLocaleString() : null, mono: true },
          { key: 'Event types', value: stats ? stats.byType.length : null, mono: true },
          { key: 'Sources', value: stats ? stats.bySource.length : null, mono: true }
        ]}
      />
    </Panel>

    <div class="breakdowns">
      <Panel title="By event type" flush>
        {#if stats && byType.length === 0}
          <EmptyState align="start" message="No events recorded." />
        {:else}
          <Table label="Events by type">
            {#snippet head()}
              <tr>
                <Th>Type</Th>
                <Th width="96px" numeric>Count</Th>
                <Th width="80px" numeric>Share</Th>
              </tr>
            {/snippet}
            {#each byType as item (item.eventType)}
              <Tr>
                <Td mono value={item.eventType} />
                <Td numeric value={item.count.toLocaleString()} />
                <Td numeric muted value={share(item.count)} />
              </Tr>
            {/each}
          </Table>
        {/if}
      </Panel>

      <Panel title="By source" flush>
        {#if stats && bySource.length === 0}
          <EmptyState align="start" message="No events recorded." />
        {:else}
          <Table label="Events by source">
            {#snippet head()}
              <tr>
                <Th>Source</Th>
                <Th width="96px" numeric>Count</Th>
                <Th width="80px" numeric>Share</Th>
              </tr>
            {/snippet}
            {#each bySource as item (item.source)}
              <Tr>
                <Td mono value={item.source} />
                <Td numeric value={item.count.toLocaleString()} />
                <Td numeric muted value={share(item.count)} />
              </Tr>
            {/each}
          </Table>
        {/if}
      </Panel>
    </div>
  {/if}
</div>

<style>
  .stats {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    padding: var(--space-3);
  }

  .breakdowns {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: var(--space-3);
    align-items: start;
  }

  @media (max-width: 960px) {
    .breakdowns {
      grid-template-columns: minmax(0, 1fr);
    }
  }
</style>
