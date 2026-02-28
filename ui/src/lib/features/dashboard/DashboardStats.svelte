<script lang="ts">
  import { onMount } from 'svelte';
  import { getEventStatistics } from '$lib/features/events/eventsApi';
  import { getPendingAutorouteStats } from '$lib/features/terminology/terminologyApi';

  let totalEvents = 0;
  let eventTypes = 0;
  let pendingReviews = 0;
  let loading = true;

  onMount(async () => {
    try {
      const [eventStats, reviewStats] = await Promise.allSettled([
        getEventStatistics(),
        getPendingAutorouteStats()
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
</script>

<div class="stats" class:loading>
  <div class="stat-card accent">
    <span class="stat-value">{loading ? '-' : totalEvents.toLocaleString()}</span>
    <span class="stat-label">Events</span>
  </div>
  <div class="stat-card">
    <span class="stat-value">{loading ? '-' : eventTypes}</span>
    <span class="stat-label">Event Types</span>
  </div>
  <div class="stat-card" class:warn={pendingReviews > 0}>
    <span class="stat-value">{loading ? '-' : pendingReviews}</span>
    <span class="stat-label">Pending Reviews</span>
  </div>
</div>

<style>
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

  .stat-card.warn {
    border-color: rgba(245, 158, 11, 0.3);
    background: rgba(245, 158, 11, 0.08);
  }

  .stat-value {
    font-size: 1.6rem;
    font-weight: 700;
    color: var(--color-text-primary);
    font-family: var(--font-mono);
  }

  .stat-label {
    font-size: 0.8rem;
    color: var(--color-text-tertiary);
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 0.05em;
  }
</style>
