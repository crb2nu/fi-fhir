<script lang="ts">
  /**
   * StatusPill Component
   *
   * A compact live-status indicator: a semantic status dot followed by a label.
   * Use for state that changes at runtime (run status, connection state, parse
   * result). For static semantic labels prefer `Badge`; for removable/clickable
   * filters use the chip primitives.
   *
   * The dot is intentionally static (no animation) to honor the design system's
   * "motion is functional, never decorative" principle.
   */

  export let variant: 'neutral' | 'success' | 'warning' | 'danger' | 'info' = 'neutral';
  export let size: 'sm' | 'md' = 'md';
  /** Hide the leading status dot (label-only). */
  export let dot = true;
</script>

<span
  class="status-pill {variant}"
  class:sm={size === 'sm'}
  {...$$restProps}
>
  {#if dot}
    <span class="dot" aria-hidden="true"></span>
  {/if}
  <slot />
</span>

<style>
  .status-pill {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
    height: var(--badge-height);
    padding: 0 var(--space-2);
    font-size: var(--badge-font-size);
    font-weight: var(--font-medium);
    line-height: var(--leading-none);
    border-radius: var(--radius-full);
    white-space: nowrap;
    color: var(--color-text-secondary);
    background: var(--color-bg-surface);
    border: 1px solid var(--color-border-default);
    transition: var(--transition-colors);
  }

  .status-pill.sm {
    height: 18px;
    padding: 0 var(--space-1);
    font-size: var(--text-2xs);
  }

  .dot {
    width: 6px;
    height: 6px;
    border-radius: var(--radius-full);
    background: var(--color-text-muted);
    flex: none;
  }

  /* Neutral keeps the surface defaults above. */
  .status-pill.success {
    color: var(--color-success-text);
    background: var(--color-success-bg);
    border-color: var(--color-success-border);
  }
  .status-pill.success .dot {
    background: var(--color-success);
  }

  .status-pill.warning {
    color: var(--color-warning-text);
    background: var(--color-warning-bg);
    border-color: var(--color-warning-border);
  }
  .status-pill.warning .dot {
    background: var(--color-warning);
  }

  .status-pill.danger {
    color: var(--color-danger-text);
    background: var(--color-danger-bg);
    border-color: var(--color-danger-border);
  }
  .status-pill.danger .dot {
    background: var(--color-danger);
  }

  .status-pill.info {
    color: var(--color-info-text);
    background: var(--color-info-bg);
    border-color: var(--color-info-border);
  }
  .status-pill.info .dot {
    background: var(--color-info);
  }
</style>
