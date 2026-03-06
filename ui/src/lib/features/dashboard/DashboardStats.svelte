<script lang="ts">
  import { onMount } from "svelte";
  import { getEventStatistics } from "$lib/features/events/eventsApi";
  import { getPendingAutorouteStats } from "$lib/features/terminology/terminologyApi";

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

      if (eventStats.status === "fulfilled") {
        totalEvents = eventStats.value.totalEvents;
        eventTypes = eventStats.value.byType.length;
      }
      if (reviewStats.status === "fulfilled") {
        pendingReviews = reviewStats.value.pendingCount;
      }
    } finally {
      loading = false;
    }
  });
</script>

<div class="stats" class:loading>
  <div class="stat-card accent">
    <span class="stat-value"
      >{loading ? "-" : totalEvents.toLocaleString()}</span
    >
    <span class="stat-label">Events</span>
  </div>
  <div class="stat-card">
    <span class="stat-value">{loading ? "-" : eventTypes}</span>
    <span class="stat-label">Event Types</span>
  </div>
  <div class="stat-card" class:warn={pendingReviews > 0}>
    <span class="stat-value">{loading ? "-" : pendingReviews}</span>
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
    padding: 16px 20px;
    border-radius: var(--radius-2xl);
    border: 1px solid var(--color-border-subtle);
    border-top: 1px solid var(--color-border-default);
    background: var(--color-bg-elevated);
    text-align: center;
    display: flex;
    flex-direction: column;
    justify-content: center;
    gap: 6px;
    box-shadow:
      var(--shadow-sm),
      inset 0 1px 0 rgba(255, 255, 255, 0.05);
    transition: var(--transition-all);
  }

  .stat-card:hover {
    transform: translateY(-2px) scale(1.02);
    box-shadow:
      var(--shadow-md),
      inset 0 1px 0 rgba(255, 255, 255, 0.05);
    border-color: var(--color-border-strong);
  }

  .stat-card.accent {
    border-color: var(--color-primary-border);
    background: linear-gradient(
      145deg,
      var(--color-bg-elevated),
      var(--color-primary-muted)
    );
  }

  .stat-card.accent:hover {
    box-shadow: var(--shadow-glow-primary);
  }

  .stat-card.warn {
    border-color: var(--color-warning-border);
    background: linear-gradient(
      145deg,
      var(--color-bg-elevated),
      var(--color-warning-bg)
    );
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
