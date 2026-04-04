<script lang="ts">
  /**
   * MetricsPanel — Dashboard view with CSS-only sparkline charts
   * and per-workflow breakdown table.
   */
  import { onMount, onDestroy } from 'svelte';
  import {
    observabilityState,
    fetchMetrics,
    isAvailable,
    type MetricSeries,
  } from './observabilityStore';

  let refreshInterval: ReturnType<typeof setInterval> | null = null;
  let timeRange = '1h';

  function latestValue(series: MetricSeries[]): number {
    if (series.length === 0) return 0;
    const all = series.flatMap((s) => s.values);
    if (all.length === 0) return 0;
    const latest = all.reduce((a, b) => (b.timestamp > a.timestamp ? b : a));
    return latest.value;
  }

  function aggregateLatestByWorkflow(series: MetricSeries[]): Array<{ workflow: string; value: number }> {
    return series.map((s) => {
      const last = s.values[s.values.length - 1];
      return { workflow: s.labels['workflow'] ?? 'unknown', value: last?.value ?? 0 };
    });
  }

  function sparklineHeights(series: MetricSeries): number[] {
    const vals = series.values.map((v) => v.value);
    const min = Math.min(...vals);
    const max = Math.max(...vals);
    const range = max - min || 1;
    return vals.map((v) => Math.max(4, ((v - min) / range) * 100));
  }

  function aggregateSeries(list: MetricSeries[]): MetricSeries {
    if (list.length === 1) return list[0]!;
    const combined: MetricSeries = {
      name: list[0]?.name ?? '',
      labels: {},
      values: [],
    };
    const byTs = new Map<number, number[]>();
    for (const s of list) {
      for (const v of s.values) {
        const bucket = byTs.get(v.timestamp) ?? [];
        bucket.push(v.value);
        byTs.set(v.timestamp, bucket);
      }
    }
    for (const [ts, vals] of Array.from(byTs.entries()).sort((a, b) => a[0] - b[0])) {
      combined.values.push({ timestamp: ts, value: vals.reduce((a, b) => a + b, 0) / vals.length });
    }
    return combined;
  }

  function formatValue(v: number, unit: string): string {
    if (unit === 'events/min') return `${Math.round(v)}`;
    if (unit === 'ms') return `${v.toFixed(1)}`;
    if (unit === '%') return `${v.toFixed(1)}`;
    if (unit === 'msgs') return `${Math.round(v)}`;
    return `${v}`;
  }

  function handleRefresh() {
    fetchMetrics();
  }

  onMount(() => {
    fetchMetrics();
    refreshInterval = setInterval(() => fetchMetrics(), 30_000);
  });

  onDestroy(() => {
    if (refreshInterval) clearInterval(refreshInterval);
  });

  $: metrics = $observabilityState.metrics;
  $: loading = $observabilityState.isLoadingMetrics;
  $: connected = $isAvailable;

  // Aggregated current values
  $: throughputVal = metrics ? latestValue(metrics.throughput) : 0;
  $: latencyP50 = metrics?.latency.find((s) => s.labels['quantile'] === '0.5');
  $: latencyP95 = metrics?.latency.find((s) => s.labels['quantile'] === '0.95');
  $: latencyP99 = metrics?.latency.find((s) => s.labels['quantile'] === '0.99');
  $: errorRateVal = metrics ? latestValue(metrics.errorRate) : 0;
  $: dlqVal = metrics ? latestValue(metrics.dlqDepth) : 0;

  // Sparkline data
  $: throughputSpark = metrics ? aggregateSeries(metrics.throughput) : null;
  $: errorSpark = metrics ? aggregateSeries(metrics.errorRate) : null;
  $: dlqSpark = metrics?.dlqDepth[0] ?? null;

  // Per-workflow table
  $: workflowRows = metrics
    ? aggregateLatestByWorkflow(metrics.throughput).map((t) => {
        const errSeries = metrics!.errorRate.find((e) => e.labels['workflow'] === t.workflow);
        const errVal = errSeries?.values[errSeries.values.length - 1]?.value ?? 0;
        return {
          workflow: t.workflow,
          events: Math.round(t.value * 60),
          errorPct: errVal,
          latencyP95: latencyP95?.values[latencyP95.values.length - 1]?.value ?? 0,
        };
      })
    : [];
</script>

<div class="metrics-panel" class:disconnected={!connected}>
  {#if !connected}
    <div class="overlay">
      <div class="overlay-content">
        <svg class="overlay-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
          <path d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
        </svg>
        <span>Connect to platform for live metrics</span>
      </div>
    </div>
  {/if}

  <header class="panel-header">
    <h3 class="panel-title">Workflow Metrics</h3>
    <div class="header-actions">
      <button class="btn-icon" on:click={handleRefresh} title="Refresh" disabled={loading}>
        <svg class="icon" class:spinning={loading} viewBox="0 0 20 20" fill="currentColor">
          <path fill-rule="evenodd" d="M4 2a1 1 0 011 1v2.101a7.002 7.002 0 0111.601 2.566 1 1 0 11-1.885.666A5.002 5.002 0 005.999 7H9a1 1 0 010 2H4a1 1 0 01-1-1V3a1 1 0 011-1zm.008 9.057a1 1 0 011.276.61A5.002 5.002 0 0014.001 13H11a1 1 0 110-2h5a1 1 0 011 1v5a1 1 0 11-2 0v-2.101a7.002 7.002 0 01-11.601-2.566 1 1 0 01.61-1.276z" clip-rule="evenodd" />
        </svg>
      </button>
      <select class="time-select" bind:value={timeRange}>
        <option value="15m">15m</option>
        <option value="1h">1h</option>
        <option value="6h">6h</option>
        <option value="24h">24h</option>
      </select>
    </div>
  </header>

  <div class="cards-grid">
    <!-- Throughput Card -->
    <div class="metric-card">
      <span class="metric-label">THROUGHPUT</span>
      <span class="metric-value throughput-color">{formatValue(throughputVal, 'events/min')}</span>
      <span class="metric-unit">events/min</span>
      {#if throughputSpark}
        <div class="sparkline" aria-hidden="true">
          {#each sparklineHeights(throughputSpark) as h, i}
            <div
              class="bar throughput-bar"
              style="height: {h}%; animation-delay: {i * 25}ms"
            ></div>
          {/each}
        </div>
      {/if}
    </div>

    <!-- Latency Card -->
    <div class="metric-card">
      <span class="metric-label">LATENCY</span>
      <div class="latency-breakdown">
        <div class="latency-row">
          <span class="latency-quantile">p50</span>
          <span class="metric-value latency-color">{formatValue(latencyP50?.values[latencyP50?.values.length - 1]?.value ?? 0, 'ms')}</span>
          <span class="metric-unit">ms</span>
        </div>
        <div class="latency-row">
          <span class="latency-quantile">p95</span>
          <span class="metric-value latency-color">{formatValue(latencyP95?.values[latencyP95?.values.length - 1]?.value ?? 0, 'ms')}</span>
          <span class="metric-unit">ms</span>
        </div>
        <div class="latency-row">
          <span class="latency-quantile">p99</span>
          <span class="metric-value latency-color">{formatValue(latencyP99?.values[latencyP99?.values.length - 1]?.value ?? 0, 'ms')}</span>
          <span class="metric-unit">ms</span>
        </div>
      </div>
    </div>

    <!-- Error Rate Card -->
    <div class="metric-card">
      <span class="metric-label">ERROR RATE</span>
      <span class="metric-value" class:error-high={errorRateVal > 1} class:error-normal={errorRateVal <= 1}>
        {formatValue(errorRateVal, '%')}
      </span>
      <span class="metric-unit">%</span>
      {#if errorSpark}
        <div class="sparkline" aria-hidden="true">
          {#each sparklineHeights(errorSpark) as h, i}
            <div
              class="bar"
              class:error-bar-high={errorRateVal > 1}
              class:error-bar-normal={errorRateVal <= 1}
              style="height: {h}%; animation-delay: {i * 25}ms"
            ></div>
          {/each}
        </div>
      {/if}
    </div>

    <!-- DLQ Depth Card -->
    <div class="metric-card">
      <span class="metric-label">DLQ DEPTH</span>
      <span class="metric-value dlq-color">{formatValue(dlqVal, 'msgs')}</span>
      <span class="metric-unit">messages</span>
      {#if dlqSpark}
        <div class="sparkline" aria-hidden="true">
          {#each sparklineHeights(dlqSpark) as h, i}
            <div
              class="bar dlq-bar"
              style="height: {h}%; animation-delay: {i * 25}ms"
            ></div>
          {/each}
        </div>
      {/if}
    </div>
  </div>

  <!-- Per-Workflow Table -->
  {#if workflowRows.length > 0}
    <div class="workflow-section">
      <h4 class="section-title">Per-Workflow Breakdown</h4>
      <div class="workflow-table">
        <div class="table-header">
          <span>Workflow</span>
          <span>Events</span>
          <span>Errors</span>
          <span>Latency</span>
        </div>
        {#each workflowRows as row (row.workflow)}
          <div class="table-row">
            <span class="workflow-name">{row.workflow}</span>
            <span class="cell-value">{row.events.toLocaleString()}</span>
            <span class="cell-value" class:cell-danger={row.errorPct > 1} class:cell-ok={row.errorPct <= 1}>
              {row.errorPct.toFixed(1)}%
            </span>
            <span class="cell-value">{row.latencyP95.toFixed(0)}ms p95</span>
          </div>
        {/each}
      </div>
    </div>
  {/if}
</div>

<style>
  .metrics-panel {
    display: grid;
    gap: var(--space-4);
    position: relative;
    padding: var(--space-4);
  }

  .metrics-panel.disconnected > :not(.overlay) {
    filter: blur(3px);
    opacity: 0.4;
    pointer-events: none;
  }

  .overlay {
    position: absolute;
    inset: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: var(--z-dropdown);
    background: transparent;
  }

  .overlay-content {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: var(--space-3);
    color: var(--color-text-tertiary);
    font-size: var(--text-sm);
    font-weight: var(--font-medium);
  }

  .overlay-icon {
    width: 32px;
    height: 32px;
    opacity: 0.6;
  }

  .panel-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
  }

  .panel-title {
    font-size: var(--text-sm);
    font-weight: var(--font-bold);
    color: var(--color-text-primary);
    letter-spacing: var(--tracking-tight);
    margin: 0;
  }

  .header-actions {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }

  .btn-icon {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 28px;
    height: 28px;
    border: 1px solid var(--color-border-default);
    border-radius: var(--radius-md);
    background: var(--color-bg-surface);
    color: var(--color-text-secondary);
    cursor: pointer;
    transition: var(--transition-all);
  }

  .btn-icon:hover {
    background: var(--color-bg-hover);
    color: var(--color-text-primary);
  }

  .btn-icon:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  .icon {
    width: 14px;
    height: 14px;
  }

  .icon.spinning {
    animation: spin 1s linear infinite;
  }

  .time-select {
    height: 28px;
    padding: 0 var(--space-2);
    font-size: var(--text-2xs);
    font-weight: var(--font-medium);
    border: 1px solid var(--color-border-default);
    border-radius: var(--radius-md);
    background: var(--color-bg-surface);
    color: var(--color-text-secondary);
    cursor: pointer;
    outline: none;
  }

  .time-select:focus {
    border-color: var(--color-border-focus);
    box-shadow: var(--shadow-focus);
  }

  /* Cards Grid */
  .cards-grid {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: var(--space-3);
  }

  .metric-card {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
    padding: var(--space-3) var(--space-4);
    background: var(--color-bg-surface);
    border: 1px solid var(--color-border-subtle);
    border-radius: var(--radius-lg);
    box-shadow: var(--shadow-sm), inset 0 1px 0 rgba(255, 255, 255, 0.05);
    transition: var(--transition-all);
    overflow: hidden;
  }

  .metric-card:hover {
    transform: translateY(-1px);
    box-shadow: var(--shadow-md), inset 0 1px 0 rgba(255, 255, 255, 0.05);
  }

  .metric-label {
    font-size: var(--text-2xs);
    font-weight: var(--font-semibold);
    color: var(--color-text-muted);
    letter-spacing: var(--tracking-wider);
    text-transform: uppercase;
  }

  .metric-value {
    font-size: var(--text-xl);
    font-weight: var(--font-bold);
    line-height: var(--leading-tight);
    transition: color var(--duration-slow) var(--ease-out);
  }

  .metric-unit {
    font-size: var(--text-2xs);
    color: var(--color-text-muted);
  }

  /* Metric semantic colors */
  .throughput-color { color: var(--color-primary); }
  .latency-color { color: var(--color-info); }
  .error-high { color: var(--color-danger); }
  .error-normal { color: var(--color-warning); }
  .dlq-color { color: var(--color-warning); }

  .metric-card:hover .throughput-color {
    text-shadow: 0 0 12px var(--color-primary-glow);
  }

  /* Latency breakdown */
  .latency-breakdown {
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  .latency-row {
    display: flex;
    align-items: baseline;
    gap: var(--space-2);
  }

  .latency-quantile {
    font-size: var(--text-2xs);
    font-weight: var(--font-medium);
    color: var(--color-text-muted);
    min-width: 24px;
    font-family: var(--font-mono);
  }

  .latency-row .metric-value {
    font-size: var(--text-sm);
  }

  .latency-row .metric-unit {
    font-size: var(--text-2xs);
  }

  /* CSS-only sparkline */
  .sparkline {
    display: grid;
    grid-template-columns: repeat(20, 1fr);
    align-items: end;
    height: 32px;
    gap: 1px;
    margin-top: var(--space-1);
  }

  .bar {
    width: 100%;
    min-height: 2px;
    border-radius: 1px 1px 0 0;
    animation: barGrow var(--duration-slow) var(--ease-out) both;
  }

  .throughput-bar {
    background: var(--color-primary);
    opacity: 0.7;
  }

  .error-bar-high {
    background: var(--color-danger);
    opacity: 0.8;
  }

  .error-bar-normal {
    background: var(--color-warning);
    opacity: 0.6;
  }

  .dlq-bar {
    background: var(--color-warning);
    opacity: 0.7;
  }

  @keyframes barGrow {
    from {
      transform: scaleY(0);
      transform-origin: bottom;
    }
    to {
      transform: scaleY(1);
      transform-origin: bottom;
    }
  }

  /* Per-Workflow Section */
  .workflow-section {
    display: grid;
    gap: var(--space-2);
  }

  .section-title {
    font-size: var(--text-xs);
    font-weight: var(--font-semibold);
    color: var(--color-text-secondary);
    margin: 0;
  }

  .workflow-table {
    display: grid;
    gap: 1px;
    border: 1px solid var(--color-border-subtle);
    border-radius: var(--radius-md);
    overflow: hidden;
    background: var(--color-border-subtle);
  }

  .table-header {
    display: grid;
    grid-template-columns: 1.5fr 1fr 1fr 1fr;
    gap: var(--space-3);
    padding: var(--space-2) var(--space-3);
    background: var(--color-bg-surface);
    font-size: var(--text-2xs);
    font-weight: var(--font-semibold);
    color: var(--color-text-muted);
    text-transform: uppercase;
    letter-spacing: var(--tracking-wider);
  }

  .table-row {
    display: grid;
    grid-template-columns: 1.5fr 1fr 1fr 1fr;
    gap: var(--space-3);
    padding: var(--space-2) var(--space-3);
    background: var(--color-bg-elevated);
    font-size: var(--text-xs);
    transition: var(--transition-colors);
  }

  .table-row:hover {
    background: var(--color-bg-hover);
  }

  .workflow-name {
    font-weight: var(--font-medium);
    color: var(--color-text-primary);
    font-family: var(--font-mono);
    font-size: var(--text-2xs);
  }

  .cell-value {
    color: var(--color-text-secondary);
    font-family: var(--font-mono);
    font-size: var(--text-2xs);
  }

  .cell-danger {
    color: var(--color-danger);
    font-weight: var(--font-semibold);
  }

  .cell-ok {
    color: var(--color-success-text);
  }

  /* Motion preferences */
  @media (prefers-reduced-motion: reduce) {
    .bar {
      animation: none;
    }
    .icon.spinning {
      animation: none;
    }
    .metric-card:hover {
      transform: none;
    }
  }

  @keyframes spin {
    from { transform: rotate(0deg); }
    to { transform: rotate(360deg); }
  }
</style>
