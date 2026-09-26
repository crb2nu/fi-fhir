<!--
  Home — an operational overview, not a landing page: what is open, what is
  deployed, and whether the engine is healthy. Every panel reads a real source
  or says it has none (`.loom/37` U-2).
-->
<script lang="ts">
  import { onMount } from 'svelte';
  import { Toolbar } from '$lib/ui/primitives';
  import AlertsPanel from '$lib/features/dashboard/AlertsPanel.svelte';
  import IntegrationsPanel from '$lib/features/dashboard/IntegrationsPanel.svelte';
  import RecentWork from '$lib/features/dashboard/RecentWork.svelte';
  import SystemStatusPanel from '$lib/features/system/SystemStatusPanel.svelte';
  import { restoreLayout } from '$lib/ui/ide/ideStore';

  // Reopen the documents the operator left open (the saved layout), so Recent
  // lists them.
  onMount(() => {
    restoreLayout();
  });
</script>

<svelte:head>
  <title>Home | fi-fhir</title>
</svelte:head>

<div class="home">
  <Toolbar title="Home" />

  <div class="home-body">
    <div class="column">
      <RecentWork />
      <IntegrationsPanel />
    </div>
    <div class="column">
      <SystemStatusPanel />
      <AlertsPanel />
    </div>
  </div>
</div>

<style>
  .home {
    display: flex;
    flex-direction: column;
    min-height: 100%;
  }

  .home-body {
    display: grid;
    grid-template-columns: minmax(0, 3fr) minmax(0, 2fr);
    gap: var(--space-3);
    padding: var(--space-3);
    align-items: start;
  }

  .column {
    display: grid;
    gap: var(--space-3);
    min-width: 0;
  }

  @media (max-width: 1080px) {
    .home-body {
      grid-template-columns: minmax(0, 1fr);
    }
  }
</style>
