<script lang="ts">
  /**
   * TraceTimeline Component
   *
   * Horizontal timeline rendering trace spans as bars.
   * Child spans are nested below parents with indentation.
   * Color-coded by status. Hover reveals span details.
   */
  import type { TraceSpan } from './types';

  export let spans: TraceSpan[] = [];

  interface FlatSpan {
    span: TraceSpan;
    depth: number;
    leftPct: number;
    widthPct: number;
  }

  $: timeRange = computeTimeRange(spans);
  $: flatSpans = flattenSpans(spans, timeRange);
  $: hasSpans = flatSpans.length > 0;

  let hoveredSpanId: string | null = null;

  function computeTimeRange(allSpans: TraceSpan[]): { min: number; max: number; duration: number } {
    if (allSpans.length === 0) return { min: 0, max: 1, duration: 1 };
    let min = Infinity;
    let max = -Infinity;
    for (const s of allSpans) {
      const start = new Date(s.startTime).getTime();
      const end = s.endTime ? new Date(s.endTime).getTime() : start + 10;
      if (start < min) min = start;
      if (end > max) max = end;
    }
    const duration = max - min || 1;
    return { min, max, duration };
  }

  function flattenSpans(
    allSpans: TraceSpan[],
    range: { min: number; max: number; duration: number }
  ): FlatSpan[] {
    if (allSpans.length === 0) return [];

    const rootParentId = '__root__';
    const byParent: Record<string, TraceSpan[]> = {};
    for (const s of allSpans) {
      const parentKey = s.parentId ?? rootParentId;
      byParent[parentKey] = [...(byParent[parentKey] ?? []), s];
    }

    const result: FlatSpan[] = [];

    function walk(parentId: string | null, depth: number): void {
      const children = byParent[parentId ?? rootParentId] ?? [];
      for (const span of children) {
        const start = new Date(span.startTime).getTime();
        const end = span.endTime ? new Date(span.endTime).getTime() : start + 10;
        const leftPct = ((start - range.min) / range.duration) * 100;
        const widthPct = Math.max(((end - start) / range.duration) * 100, 1);
        result.push({ span, depth, leftPct, widthPct });
        walk(span.id, depth + 1);
      }
    }

    walk(null, 0);
    return result;
  }

  function formatDuration(span: TraceSpan): string {
    if (!span.endTime) return 'in progress';
    const ms = new Date(span.endTime).getTime() - new Date(span.startTime).getTime();
    if (ms < 1) return '<1ms';
    return `${ms}ms`;
  }

  function statusClass(status: TraceSpan['status']): string {
    if (status === 'ok') return 'status-ok';
    if (status === 'error') return 'status-error';
    return 'status-unset';
  }
</script>

<div class="trace-timeline" role="figure" aria-label="Trace timeline">
  {#if !hasSpans}
    <div class="timeline-empty">No trace spans</div>
  {:else}
    <div class="timeline-container">
      {#each flatSpans as { span, depth, leftPct, widthPct } (span.id)}
        <div
          class="span-row"
          style="padding-left: {depth * 20 + 8}px;"
          role="listitem"
        >
          <span class="span-label">{span.name}</span>
          <div class="span-track">
            <div
              class="span-bar {statusClass(span.status)}"
              style="left: {leftPct}%; width: {widthPct}%;"
              on:mouseenter={() => { hoveredSpanId = span.id; }}
              on:mouseleave={() => { hoveredSpanId = null; }}
              role="button"
              tabindex="0"
              aria-label="{span.name}: {formatDuration(span)}"
            >
              {#if hoveredSpanId === span.id}
                <div class="span-tooltip">
                  <div class="tooltip-name">{span.name}</div>
                  <div class="tooltip-duration">{formatDuration(span)}</div>
                  <div class="tooltip-status">Status: {span.status}</div>
                  {#if span.events.length > 0}
                    <div class="tooltip-events">{span.events.length} event(s)</div>
                  {/if}
                </div>
              {/if}
            </div>
          </div>
          <span class="span-duration">{formatDuration(span)}</span>
        </div>
      {/each}
    </div>
  {/if}
</div>

<style>
  .trace-timeline {
    overflow: auto;
  }

  .timeline-empty {
    padding: var(--space-4);
    color: var(--color-text-muted);
    text-align: center;
    font-size: var(--text-xs);
    font-style: italic;
  }

  .timeline-container {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
    padding: var(--space-2);
  }

  .span-row {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    min-height: 28px;
  }

  .span-label {
    font-family: var(--font-mono);
    font-size: var(--text-2xs);
    color: var(--color-text-secondary);
    white-space: nowrap;
    min-width: 120px;
    flex-shrink: 0;
  }

  .span-track {
    flex: 1;
    position: relative;
    height: 18px;
    background: var(--color-bg-surface);
    border-radius: var(--radius-sm);
    overflow: visible;
  }

  .span-bar {
    position: absolute;
    top: 2px;
    height: 14px;
    border-radius: var(--radius-sm);
    cursor: pointer;
    transition: opacity var(--duration-fast) var(--ease-out);
  }

  .span-bar:hover {
    opacity: 0.85;
  }

  .span-bar:focus-visible {
    outline: 2px solid var(--color-border-focus);
    outline-offset: 1px;
  }

  .span-bar.status-ok {
    background: var(--color-success);
  }

  .span-bar.status-error {
    background: var(--color-danger);
  }

  .span-bar.status-unset {
    background: var(--color-text-muted);
  }

  .span-duration {
    font-family: var(--font-mono);
    font-size: var(--text-2xs);
    color: var(--color-text-muted);
    white-space: nowrap;
    min-width: 48px;
    text-align: right;
    flex-shrink: 0;
  }

  /* Tooltip */
  .span-tooltip {
    position: absolute;
    bottom: calc(100% + 6px);
    left: 50%;
    transform: translateX(-50%);
    padding: var(--space-2) var(--space-3);
    background: var(--color-bg-overlay);
    border: 1px solid var(--color-border-default);
    border-radius: var(--radius-md);
    box-shadow: var(--shadow-md);
    white-space: nowrap;
    z-index: var(--z-tooltip);
    pointer-events: none;
  }

  .tooltip-name {
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    font-weight: var(--font-semibold);
    color: var(--color-text-primary);
  }

  .tooltip-duration,
  .tooltip-status,
  .tooltip-events {
    font-size: var(--text-2xs);
    color: var(--color-text-tertiary);
    margin-top: 2px;
  }
</style>
