<script lang="ts">
  import { onMount } from 'svelte';
  import { getEventStatistics } from '$lib/features/events/eventsApi';
  import { getPendingAutorouteStats } from '$lib/features/terminology/terminologyApi';

  type MetricTone = 'accent' | 'default' | 'warn';

  let totalEvents = 0;
  let eventTypes = 0;
  let pendingReviews = 0;
  let loading = true;

  onMount(async () => {
    try {
      const [eventStats, reviewStats] = await Promise.allSettled([
        getEventStatistics(),
        getPendingAutorouteStats(),
      ]);

      if (eventStats.status === 'fulfilled') {
        totalEvents = eventStats.value.totalEvents;
        eventTypes = eventStats.value.byType.length;
      }
      if (reviewStats.status === 'fulfilled') {
        pendingReviews = reviewStats.value.pendingCount;
      }
    } finally {
      loading = false;
    }
  });

  $: metrics = [
    {
      label: 'Events',
      value: loading ? '—' : totalEvents.toLocaleString(),
      hint: 'Observed downstream volume',
      tone: 'accent' as MetricTone
    },
    {
      label: 'Event types',
      value: loading ? '—' : String(eventTypes),
      hint: 'Semantic variety in flow',
      tone: 'default' as MetricTone
    },
    {
      label: 'Pending reviews',
      value: loading ? '—' : String(pendingReviews),
      hint: pendingReviews > 0 ? 'Terminology review queue needs attention' : 'No review backlog right now',
      tone: pendingReviews > 0 ? 'warn' as MetricTone : 'default' as MetricTone
    }
  ];
</script>

<div class="telemetry">
  <div class="intro">
    <div class="intro-label">Operator signals</div>
    <p>Track throughput, shape, and review pressure at a glance while you move through the workspace.</p>
  </div>

  <div class="stats" class:loading>
    {#each metrics as metric (metric.label)}
      <div class="stat-card" class:accent={metric.tone === 'accent'} class:warn={metric.tone === 'warn'}>
        <span class="stat-value">{metric.value}</span>
        <span class="stat-label">{metric.label}</span>
        <span class="stat-hint">{metric.hint}</span>
      </div>
    {/each}
  </div>
</div>

<style>
  .telemetry {
    display: grid;
    gap: 12px;
  }

  .intro {
    display: grid;
    gap: 4px;
  }

  .intro-label {
    color: var(--color-text-tertiary);
    font-size: var(--text-2xs);
    font-weight: var(--font-bold);
    letter-spacing: 0.12em;
    text-transform: uppercase;
  }

  .intro p {
    margin: 0;
    color: var(--color-text-secondary);
    font-size: var(--text-sm);
    line-height: 1.5;
  }

  .stats {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
    gap: 12px;
  }

  .stats.loading {
    opacity: 0.6;
  }

  .stat-card {
    padding: 14px 16px;
    border-radius: var(--radius-xl);
    border: 1px solid var(--color-border-subtle);
    background: rgba(255, 255, 255, 0.03);
    display: grid;
    gap: 6px;
    transition: var(--transition-all);
  }

  .stat-card:hover {
    transform: translateY(-2px);
    border-color: var(--color-border-strong);
    box-shadow: var(--shadow-sm);
  }

  .stat-card.accent {
    border-color: var(--color-primary-border);
    background: linear-gradient(145deg, rgba(99, 102, 241, 0.16), rgba(99, 102, 241, 0.05));
  }

  .stat-card.warn {
    border-color: var(--color-warning-border);
    background: linear-gradient(145deg, rgba(245, 158, 11, 0.14), rgba(245, 158, 11, 0.04));
  }

  .stat-value {
    font-size: 1.5rem;
    font-weight: 700;
    color: var(--color-text-primary);
    font-family: var(--font-mono);
  }

  .stat-label {
    font-size: var(--text-2xs);
    color: var(--color-text-tertiary);
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 0.08em;
  }

  .stat-hint {
    color: var(--color-text-secondary);
    font-size: var(--text-xs);
    line-height: 1.45;
  }
</style>
