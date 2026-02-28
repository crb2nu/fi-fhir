<script lang="ts">
  import { getPatientTimeline } from './eventsApi';
  import type { PatientTimelineQuery } from '$lib/gen/graphql';
  import Button from '$lib/ui/Button.svelte';
  import EmptyState from '$lib/ui/EmptyState.svelte';

  type Timeline = NonNullable<PatientTimelineQuery['patientTimeline']>;

  let mrn = '';
  let timeline: Timeline | null = null;
  let loading = false;
  let error: string | null = null;

  async function search() {
    if (!mrn.trim()) return;
    loading = true;
    error = null;
    try {
      timeline = await getPatientTimeline(mrn.trim());
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to load timeline';
      timeline = null;
    } finally {
      loading = false;
    }
  }

  function formatTimestamp(ts: string): string {
    try {
      return new Date(ts).toLocaleString();
    } catch {
      return ts;
    }
  }

  function typeColor(eventType: string): string {
    const t = eventType.toUpperCase();
    if (t.includes('ADMIT') || t.includes('DISCHARGE') || t.includes('TRANSFER')) return 'adt';
    if (t.includes('LAB')) return 'lab';
    if (t.includes('APPOINTMENT')) return 'appt';
    if (t.includes('CLAIM') || t.includes('ELIGIBILITY')) return 'claim';
    return 'default';
  }
</script>

<div class="timeline-page">
  <form class="search-bar" on:submit|preventDefault={search}>
    <input
      aria-label="Patient MRN"
      type="text"
      class="input"
      bind:value={mrn}
      placeholder="Enter patient MRN..."
    />
    <Button variant="primary" size="sm" type="submit" {loading}>
      Load Timeline
    </Button>
  </form>

  {#if error}
    <EmptyState icon="error" title="Timeline not found" description={error} />
  {:else if timeline}
    <div class="timeline-header">
      <span class="mrn-label">MRN: <strong class="mono">{timeline.mrn}</strong></span>
      <span class="event-count">{timeline.eventCount} events</span>
    </div>

    {#if timeline.events.length === 0}
      <EmptyState icon="inbox" title="No events" description="No events found for this patient." />
    {:else}
      <div class="timeline">
        {#each timeline.events as event, i (event.position)}
          <div class="timeline-item">
            <div class="timeline-dot {typeColor(event.eventType)}"></div>
            {#if i < timeline.events.length - 1}
              <div class="timeline-line"></div>
            {/if}
            <div class="timeline-content">
              <div class="timeline-time mono">{formatTimestamp(event.timestamp)}</div>
              <div class="timeline-type">{event.eventType.replace(/_/g, ' ')}</div>
              <div class="timeline-summary">{event.summary}</div>
              {#if event.source}
                <div class="timeline-source mono">{event.source}</div>
              {/if}
            </div>
          </div>
        {/each}
      </div>
    {/if}
  {:else if !loading}
    <EmptyState
      icon="search"
      title="Search for a patient"
      description="Enter a patient MRN to view their event timeline."
    />
  {/if}
</div>

<style>
  .timeline-page {
    display: grid;
    gap: 16px;
  }

  .search-bar {
    display: flex;
    gap: 8px;
    align-items: center;
  }

  .input {
    flex: 1;
    max-width: 300px;
    padding: 8px 12px;
    border-radius: 10px;
    border: 1px solid var(--color-border-default);
    background: var(--color-bg-input);
    color: var(--color-text-primary);
    outline: none;
  }

  .input:focus {
    border-color: var(--color-border-focus);
    box-shadow: var(--shadow-focus);
  }

  .timeline-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 8px 0;
    border-bottom: 1px solid var(--color-border-default);
  }

  .mrn-label {
    color: var(--color-text-secondary);
  }

  .event-count {
    color: var(--color-text-tertiary);
    font-size: 0.85rem;
    font-weight: 700;
  }

  .timeline {
    display: grid;
    gap: 0;
    padding-left: 20px;
  }

  .timeline-item {
    position: relative;
    padding-left: 24px;
    padding-bottom: 16px;
  }

  .timeline-dot {
    position: absolute;
    left: -6px;
    top: 4px;
    width: 12px;
    height: 12px;
    border-radius: 50%;
    background: var(--color-bg-surface);
    border: 2px solid var(--color-border-strong);
    z-index: 1;
  }

  .timeline-dot.adt { border-color: rgba(59, 130, 246, 0.7); background: rgba(59, 130, 246, 0.2); }
  .timeline-dot.lab { border-color: rgba(16, 185, 129, 0.7); background: rgba(16, 185, 129, 0.2); }
  .timeline-dot.appt { border-color: rgba(245, 158, 11, 0.7); background: rgba(245, 158, 11, 0.2); }
  .timeline-dot.claim { border-color: rgba(168, 85, 247, 0.7); background: rgba(168, 85, 247, 0.2); }

  .timeline-line {
    position: absolute;
    left: 0;
    top: 16px;
    bottom: 0;
    width: 1px;
    background: var(--color-border-default);
  }

  .timeline-content {
    display: grid;
    gap: 2px;
  }

  .timeline-time {
    font-size: 0.8rem;
    color: var(--color-text-tertiary);
  }

  .timeline-type {
    font-weight: 700;
    color: var(--color-text-primary);
    text-transform: capitalize;
  }

  .timeline-summary {
    color: var(--color-text-secondary);
    font-size: 0.9rem;
    line-height: 1.4;
  }

  .timeline-source {
    font-size: 0.8rem;
    color: var(--color-text-muted);
  }

  .mono { font-family: var(--font-mono); }
</style>
