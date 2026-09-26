<!--
  Events — the event store, four views under one toolbar. Browse is the
  table + details pattern; Live Stream is honest about what this deployment
  can stream; Patient Timeline and Statistics are plain panels.
-->
<script lang="ts">
  import { Badge, Tabs, Toolbar, type TabItem } from '$lib/ui/primitives';
  import EventBrowser from '$lib/features/events/EventBrowser.svelte';
  import EventStats from '$lib/features/events/EventStats.svelte';
  import EventStreamPanel from '$lib/features/events/EventStreamPanel.svelte';
  import PatientTimeline from '$lib/features/events/PatientTimeline.svelte';

  const views: TabItem[] = [
    { id: 'browse', label: 'Browse', controls: 'events-view' },
    { id: 'live', label: 'Live Stream', controls: 'events-view' },
    { id: 'timeline', label: 'Patient Timeline', controls: 'events-view' },
    { id: 'stats', label: 'Statistics', controls: 'events-view' }
  ];

  let view = $state('browse');
  let totalCount = $state(0);
  let loading = $state(false);
</script>

<svelte:head>
  <title>Events | fi-fhir</title>
</svelte:head>

<div class="events-page">
  <Toolbar title="Events">
    {#snippet tabs()}
      <Tabs label="Event views" items={views} bind:value={view} />
    {/snippet}
    {#snippet actions()}
      {#if view === 'browse'}
        <Badge mono data-testid="events-total" aria-busy={loading}>
          {totalCount.toLocaleString()} events
        </Badge>
      {/if}
    {/snippet}
  </Toolbar>

  <div class="events-view" id="events-view" role="tabpanel" aria-label={views.find((v) => v.id === view)?.label}>
    {#if view === 'browse'}
      <EventBrowser bind:totalCount bind:loading />
    {:else if view === 'live'}
      <EventStreamPanel />
    {:else if view === 'timeline'}
      <PatientTimeline />
    {:else}
      <EventStats />
    {/if}
  </div>
</div>

<style>
  .events-page {
    display: flex;
    flex-direction: column;
    height: 100%;
    min-height: 0;
  }

  .events-view {
    display: flex;
    flex-direction: column;
    flex: 1 1 auto;
    min-height: 0;
  }
</style>
