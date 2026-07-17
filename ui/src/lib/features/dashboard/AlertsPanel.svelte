<script lang="ts">
  /**
   * AlertsPanel — firing alerts from the observability store.
   *
   * Previously rendered a hardcoded fictional alert list; now it shares the
   * real alertmanager-backed store with the StatusBar AlertBadge. When the
   * platform is disconnected the store falls back to demo data and sets
   * `isSimulated` — surfaced here as an unmissable "Demo data" tag so
   * operators never mistake demo alerts for live signals.
   */
  import { onMount } from 'svelte';
  import Panel from '$lib/ui/Panel.svelte';
  import Badge from '$lib/ui/Badge.svelte';
  import {
    observabilityState,
    isSimulated,
    fetchAlerts,
    severityLabel,
    type Alert,
  } from '$lib/features/observability/observabilityStore';

  $: firing = $observabilityState.alerts.filter((a) => a.state === 'firing');

  const severityVariant: Record<Alert['severity'], 'danger' | 'warning' | 'info'> = {
    critical: 'danger',
    warning: 'warning',
    info: 'info',
  };

  onMount(() => {
    void fetchAlerts();
  });
</script>

<Panel title="Active Alerts" padding="md">
  <svelte:fragment slot="actions">
    {#if $isSimulated}
      <span class="sim-tag" title="Platform not connected — showing demo data.">Demo data</span>
    {/if}
  </svelte:fragment>

  {#if firing.length === 0}
    <div class="empty">No active alerts.</div>
  {:else}
    <ul class="alerts-list">
      {#each firing as alert (alert.id)}
        <li class="alert-card" class:critical={alert.severity === 'critical'}>
          <div class="alert-header">
            <Badge variant={severityVariant[alert.severity]} size="sm">
              {severityLabel(alert.severity)}
            </Badge>
            <h4 class="title">{alert.name}</h4>
          </div>
          <p class="description">{alert.summary}</p>
        </li>
      {/each}
    </ul>
  {/if}
</Panel>

<style>
  .sim-tag {
    padding: 1px var(--space-1);
    border-radius: var(--radius-sm);
    border: 1px solid var(--color-warning-border);
    background: var(--color-warning-bg);
    color: var(--color-warning-text);
    font-size: var(--text-2xs, 10px);
    font-weight: var(--font-semibold);
    text-transform: uppercase;
    letter-spacing: 0.06em;
    white-space: nowrap;
  }

  .empty {
    color: var(--color-text-tertiary);
    font-size: var(--text-sm);
    padding: var(--space-4) 0;
    text-align: center;
  }

  .alerts-list {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .alert-card {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    padding: var(--space-3);
    background: var(--color-bg-surface);
    border: 1px solid var(--color-border-default);
    border-left: 3px solid var(--color-warning);
    border-radius: var(--radius-md);
  }

  .alert-card.critical {
    border-left-color: var(--color-danger);
  }

  .alert-header {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }

  .title {
    margin: 0;
    font-size: var(--text-sm);
    font-weight: var(--font-bold);
    color: var(--color-text-primary);
  }

  .description {
    margin: 0;
    font-size: var(--text-sm);
    color: var(--color-text-secondary);
    line-height: var(--leading-snug);
  }
</style>
