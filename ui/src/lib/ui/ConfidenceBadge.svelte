<script lang="ts">
  /**
   * ConfidenceBadge Component
   *
   * Specialized badge for displaying confidence scores with color-coded
   * visual feedback. Uses a gradient color system based on thresholds.
   */

  export let confidence: number;
  export let showPercent = true;
  export let size: 'sm' | 'md' = 'md';

  $: percent = Math.round(confidence * 100);

  $: level = confidence >= 0.9 ? 'high'
           : confidence >= 0.7 ? 'medium'
           : confidence >= 0.5 ? 'low'
           : 'very-low';

  $: label = confidence >= 0.9 ? 'High'
           : confidence >= 0.7 ? 'Medium'
           : confidence >= 0.5 ? 'Low'
           : 'Very Low';
</script>

<span
  class="confidence-badge {level}"
  class:sm={size === 'sm'}
  title="{percent}% confidence ({label})"
  {...$$restProps}
>
  {#if showPercent}
    <span class="percent">{percent}%</span>
  {/if}
  <span class="bar">
    <span class="fill" style="width: {percent}%"></span>
  </span>
</span>

<style>
  .confidence-badge {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    padding: var(--space-1) var(--space-2);
    border-radius: var(--radius-md);
    font-size: var(--text-xs);
    font-weight: var(--font-semibold);
    font-variant-numeric: tabular-nums;
    line-height: var(--leading-none);
  }

  .confidence-badge.sm {
    padding: 2px var(--space-1);
    gap: var(--space-1);
    font-size: var(--text-2xs);
  }

  .percent {
    min-width: 32px;
  }

  .confidence-badge.sm .percent {
    min-width: 26px;
  }

  .bar {
    width: 40px;
    height: 4px;
    background: var(--color-border-subtle);
    border-radius: var(--radius-full);
    overflow: hidden;
  }

  .confidence-badge.sm .bar {
    width: 30px;
    height: 3px;
  }

  .fill {
    display: block;
    height: 100%;
    border-radius: var(--radius-full);
    transition: width var(--duration-slow) var(--ease-out);
  }

  /* Level: High (>= 90%) */
  .confidence-badge.high {
    color: var(--confidence-high);
    background: var(--confidence-high-bg);
  }

  .confidence-badge.high .fill {
    background: var(--confidence-high);
  }

  /* Level: Medium (70-89%) */
  .confidence-badge.medium {
    color: var(--confidence-medium);
    background: var(--confidence-medium-bg);
  }

  .confidence-badge.medium .fill {
    background: var(--confidence-medium);
  }

  /* Level: Low (50-69%) */
  .confidence-badge.low {
    color: var(--confidence-low);
    background: var(--confidence-low-bg);
  }

  .confidence-badge.low .fill {
    background: var(--confidence-low);
  }

  /* Level: Very Low (< 50%) */
  .confidence-badge.very-low {
    color: var(--confidence-very-low);
    background: var(--confidence-very-low-bg);
  }

  .confidence-badge.very-low .fill {
    background: var(--confidence-very-low);
  }
</style>
