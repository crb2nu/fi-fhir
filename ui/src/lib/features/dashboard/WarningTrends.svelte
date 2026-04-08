<script lang="ts">
  import Panel from '$lib/ui/Panel.svelte';
  import Badge from '$lib/ui/Badge.svelte';

  // Mock data for warning trends
  const topWarnings = [
    { code: 'W001', message: 'Missing segment', rate: 15, trend: 'up' },
    { code: 'W042', message: 'Invalid date format', rate: 8, trend: 'down' },
    { code: 'E099', message: 'Unknown facility', rate: 4, trend: 'stable' }
  ];
</script>

<Panel title="Warning Trends" padding="md">
  <div class="trends-container">
    <div class="metrics">
      <div class="metric-card">
        <span class="metric-value">1,402</span>
        <span class="metric-label">Warnings Today</span>
        <Badge variant="warning" size="sm">+12% vs yesterday</Badge>
      </div>
      <div class="metric-card">
        <span class="metric-value">4.2%</span>
        <span class="metric-label">Warning Rate</span>
        <Badge variant="success" size="sm">-0.5% vs yesterday</Badge>
      </div>
    </div>
    
    <h3 class="sub-title">Top Warning Codes</h3>
    <div class="top-warnings">
      {#each topWarnings as warning (warning.code)}
        <div class="warning-item">
          <div class="warning-info">
            <span class="code mono">{warning.code}</span>
            <span class="message">{warning.message}</span>
          </div>
          <div class="warning-stats">
            <span class="rate">{warning.rate}% of msgs</span>
            <span class="trend {warning.trend}">
              {#if warning.trend === 'up'}↑
              {:else if warning.trend === 'down'}↓
              {:else}→{/if}
            </span>
          </div>
        </div>
      {/each}
    </div>
  </div>
</Panel>

<style>
  .trends-container {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
  }

  .metrics {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: var(--space-3);
  }

  .metric-card {
    display: flex;
    flex-direction: column;
    gap: 4px;
    padding: var(--space-3);
    background: var(--color-bg-surface);
    border: 1px solid var(--color-border-default);
    border-radius: var(--radius-lg);
  }

  .metric-value {
    font-size: var(--text-2xl);
    font-weight: var(--font-bold);
    color: var(--color-text-primary);
  }

  .metric-label {
    font-size: var(--text-sm);
    color: var(--color-text-secondary);
    margin-bottom: var(--space-2);
  }

  .sub-title {
    font-size: var(--text-sm);
    font-weight: var(--font-semibold);
    color: var(--color-text-secondary);
    text-transform: uppercase;
    letter-spacing: var(--tracking-wider);
    margin: var(--space-2) 0 0 0;
  }

  .top-warnings {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  .warning-item {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: var(--space-3);
    background: var(--color-bg-surface);
    border: 1px solid var(--color-border-subtle);
    border-radius: var(--radius-md);
  }

  .warning-info {
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  .code {
    font-size: var(--text-xs);
    font-weight: var(--font-bold);
    color: var(--color-warning);
  }

  .mono {
    font-family: var(--font-mono);
  }

  .message {
    font-size: var(--text-sm);
    color: var(--color-text-primary);
  }

  .warning-stats {
    display: flex;
    align-items: center;
    gap: var(--space-3);
  }

  .rate {
    font-size: var(--text-xs);
    color: var(--color-text-secondary);
  }

  .trend {
    font-weight: var(--font-bold);
  }

  .trend.up { color: var(--color-danger); }
  .trend.down { color: var(--color-success); }
  .trend.stable { color: var(--color-text-muted); }
</style>
