<!--
  Events › Patient Timeline: one patient's events in order, looked up by MRN.
  A filters row and a table; no timeline art.
-->
<script lang="ts">
  import CircleAlert from '@lucide/svelte/icons/circle-alert';
  import Inbox from '@lucide/svelte/icons/inbox';
  import Search from '@lucide/svelte/icons/search';
  import type { PatientTimelineQuery } from '$lib/gen/graphql';
  import { Badge, Button, EmptyState, Input, Table, Td, Th, Tr } from '$lib/ui/primitives';
  import { formatEventTime } from './eventFormat';
  import { getPatientTimeline } from './eventsApi';

  type Timeline = NonNullable<PatientTimelineQuery['patientTimeline']>;

  let mrn = $state('');
  let timeline = $state<Timeline | null>(null);
  let searched = $state(false);
  let loading = $state(false);
  let error = $state<string | null>(null);

  async function search(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    const value = mrn.trim();
    if (!value) return;
    loading = true;
    error = null;
    try {
      timeline = await getPatientTimeline(value);
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to load timeline';
      timeline = null;
    } finally {
      loading = false;
      searched = true;
    }
  }
</script>

<div class="timeline">
  <form class="filters" aria-label="Patient lookup" onsubmit={search}>
    <div class="filter-mrn">
      <Input aria-label="Patient MRN" bind:value={mrn} placeholder="Patient MRN" mono />
    </div>
    <Button type="submit" variant="primary" icon={Search} {loading} disabled={!mrn.trim()}>
      Load timeline
    </Button>
    {#if timeline}
      <span class="spacer"></span>
      <span class="mrn text-mono">{timeline.mrn}</span>
      <Badge mono>{timeline.eventCount} events</Badge>
    {/if}
  </form>

  {#if error}
    <EmptyState icon={CircleAlert} message={`The timeline could not be loaded: ${error}`} />
  {:else if timeline && timeline.events.length === 0}
    <EmptyState icon={Inbox} message="No events are recorded for this patient." />
  {:else if timeline}
    <Table label="Patient timeline" layout="fixed" class="timeline-table">
      {#snippet head()}
        <tr>
          <Th width="56px" numeric>#</Th>
          <Th width="156px">Time</Th>
          <Th width="176px">Type</Th>
          <Th>Summary</Th>
          <Th width="140px">Source</Th>
        </tr>
      {/snippet}
      {#each timeline.events as event (event.position)}
        <Tr>
          <Td numeric muted value={event.position} />
          <Td mono muted value={formatEventTime(event.timestamp)} />
          <Td mono truncate value={event.eventType} />
          <Td truncate value={event.summary} />
          <Td mono truncate muted value={event.source ?? '—'} />
        </Tr>
      {/each}
    </Table>
  {:else if !searched}
    <EmptyState icon={Search} message="Enter a patient MRN to list that patient's events." />
  {/if}
</div>

<style>
  .timeline {
    display: flex;
    flex-direction: column;
    flex: 1 1 auto;
    min-height: 0;
  }

  .filters {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    padding: var(--space-2) var(--space-3);
    border-bottom: 1px solid var(--color-border-subtle);
  }

  .filter-mrn {
    width: 240px;
  }

  .spacer {
    flex: 1 1 auto;
  }

  .mrn {
    color: var(--color-text-secondary);
  }

  .timeline :global(.timeline-table) {
    flex: 0 1 auto;
    min-height: 0;
  }
</style>
