<!--
  AlertsPanel — firing alerts from the shared observability store (the same
  Alertmanager-backed list the StatusBar AlertBadge shows).

  There is no demo fallback: with no observability platform connected there is
  no alert source, and the panel says exactly that. It never renders a
  placeholder alert, labelled or not (`.loom/37` decision 3).
-->
<script lang="ts">
  import BellOff from '@lucide/svelte/icons/bell-off';
  import CircleCheck from '@lucide/svelte/icons/circle-check';
  import CircleAlert from '@lucide/svelte/icons/circle-alert';
  import RefreshCw from '@lucide/svelte/icons/refresh-cw';
  import {
    Badge,
    EmptyState,
    IconButton,
    Panel,
    Table,
    Td,
    Th,
    Tr,
    type BadgeTone
  } from '$lib/ui/primitives';
  import { PLATFORM_CONFIG } from '$lib/platform';
  import {
    alertSource,
    fetchAlerts,
    isAvailable,
    observabilityState,
    severityLabel,
    type Alert
  } from '$lib/features/observability/observabilityStore';

  const firing = $derived($observabilityState.alerts.filter((a) => a.state === 'firing'));

  const severityTone: Record<Alert['severity'], BadgeTone> = {
    critical: 'danger',
    warning: 'warning',
    info: 'info'
  };

  // Fetch on mount and whenever the platform connects or drops.
  $effect(() => {
    void $isAvailable;
    void fetchAlerts();
  });

  function since(ts: number): string {
    const diff = Math.max(0, Math.floor((Date.now() - ts) / 1000));
    if (diff < 60) return `${diff}s`;
    if (diff < 3600) return `${Math.floor(diff / 60)}m`;
    if (diff < 86400) return `${Math.floor(diff / 3600)}h`;
    return `${Math.floor(diff / 86400)}d`;
  }
</script>

<Panel title="Alerts" flush data-testid="alerts-panel" data-source={$alertSource}>
  {#snippet actions()}
    {#if $alertSource !== 'unconfigured'}
      <IconButton icon={RefreshCw} label="Refresh alerts" onclick={() => void fetchAlerts()} />
    {/if}
  {/snippet}

  {#if $alertSource === 'unconfigured'}
    <EmptyState
      icon={BellOff}
      align="start"
      message={PLATFORM_CONFIG.enabled
        ? 'The observability platform is not connected, so no alert source is available.'
        : 'No alert source configured.'}
    />
  {:else if $alertSource === 'unavailable'}
    <EmptyState
      icon={CircleAlert}
      align="start"
      message="The alert source did not answer."
      actionLabel="Retry"
      onaction={() => void fetchAlerts()}
    />
  {:else if firing.length === 0}
    <EmptyState icon={CircleCheck} align="start" message="No active alerts." />
  {:else}
    <Table label="Firing alerts" layout="fixed">
      {#snippet head()}
        <tr>
          <Th width="88px">Severity</Th>
          <Th width="30%">Alert</Th>
          <Th>Summary</Th>
          <Th width="56px" numeric>Since</Th>
        </tr>
      {/snippet}
      {#each firing as alert (alert.id)}
        <Tr>
          <Td>
            <Badge tone={severityTone[alert.severity]} dot>{severityLabel(alert.severity)}</Badge>
          </Td>
          <Td truncate value={alert.name} />
          <Td truncate muted value={alert.summary} />
          <Td numeric muted value={since(alert.startsAt)} />
        </Tr>
      {/each}
    </Table>
  {/if}
</Panel>
