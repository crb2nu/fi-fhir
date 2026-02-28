<script lang="ts">
  import Panel from '$lib/ui/Panel.svelte';
  import Tabs from '$lib/ui/Tabs.svelte';
  import type { TabItem } from '$lib/ui/types';
  import EventBrowser from '$lib/features/events/EventBrowser.svelte';
  import EventDetail from '$lib/features/events/EventDetail.svelte';
  import EventStats from '$lib/features/events/EventStats.svelte';
  import EventStreamPanel from '$lib/features/events/EventStreamPanel.svelte';
  import PatientTimeline from '$lib/features/events/PatientTimeline.svelte';
  import type { EventsQuery } from '$lib/gen/graphql';

  type EventNode = EventsQuery['events']['edges'][number]['node'];

  const tabs: readonly TabItem[] = [
    { key: 'browse', label: 'Browse' },
    { key: 'live', label: 'Live Stream' },
    { key: 'timeline', label: 'Patient Timeline' },
    { key: 'stats', label: 'Statistics' }
  ];

  let activeTab = 'browse';
  let selectedEvent: EventNode | null = null;

  function handleSelectEvent(e: CustomEvent<{ event: EventNode }>) {
    selectedEvent = e.detail.event;
  }

  function closeDetail() {
    selectedEvent = null;
  }
</script>

<h1>Events</h1>
<p class="sub">
  Browse processed events, monitor the live stream, view patient timelines, and review aggregate statistics.
</p>

<div class="tabs-wrapper">
  <Tabs {tabs} active={activeTab} onChange={(key) => (activeTab = key)} />
</div>

<Panel>
  {#if activeTab === 'browse'}
    <div class="tab-content">
      <div class="browse-layout" class:has-detail={!!selectedEvent}>
        <div class="browse-main">
          <EventBrowser on:select={handleSelectEvent} />
        </div>
        {#if selectedEvent}
          <div class="browse-detail">
            <EventDetail event={selectedEvent} onClose={closeDetail} />
          </div>
        {/if}
      </div>
    </div>
  {:else if activeTab === 'live'}
    <div class="tab-content">
      <p class="description">
        Real-time event stream via WebSocket. Events appear as they are processed by the backend.
      </p>
      <EventStreamPanel />
    </div>
  {:else if activeTab === 'timeline'}
    <div class="tab-content">
      <p class="description">
        View a patient's chronological event history. Enter a patient MRN to load their timeline.
      </p>
      <PatientTimeline />
    </div>
  {:else if activeTab === 'stats'}
    <div class="tab-content">
      <p class="description">
        Aggregate event statistics — total counts, breakdowns by type and source.
      </p>
      <EventStats />
    </div>
  {/if}
</Panel>

<style>
  h1 {
    color: var(--color-text-primary);
    margin: 0 0 8px;
  }

  .sub {
    color: var(--color-text-secondary);
    line-height: 1.55;
    margin: 0 0 16px;
    max-width: 70ch;
  }

  .tabs-wrapper {
    margin-bottom: 16px;
  }

  .tab-content {
    padding: 8px 0;
  }

  .description {
    color: var(--color-text-tertiary);
    font-size: 0.9rem;
    line-height: 1.55;
    margin: 0 0 16px;
  }

  .browse-layout {
    display: grid;
    gap: 16px;
  }

  .browse-layout.has-detail {
    grid-template-columns: 1fr 320px;
  }

  @media (max-width: 768px) {
    .browse-layout.has-detail {
      grid-template-columns: 1fr;
    }
  }
</style>
