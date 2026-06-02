<script lang="ts">
  /**
   * Badge Component
   *
   * A versatile badge for displaying status, labels, or counts.
   * Supports multiple variants and sizes with consistent styling.
   */

  export let variant: 'default' | 'primary' | 'success' | 'warning' | 'danger' | 'info' = 'default';
  export let size: 'sm' | 'md' = 'md';
  export let outline = false;
  export let pill = false;
  /** Render content in the monospace font (for IDs, codes, versions). */
  export let mono = false;
</script>

<span
  class="badge {variant}"
  class:sm={size === 'sm'}
  class:outline
  class:pill
  class:mono
  {...$$restProps}
>
  <slot />
</span>

<style>
  .badge {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: var(--space-1);
    height: var(--badge-height);
    padding: 0 var(--badge-padding-x);
    font-size: var(--badge-font-size);
    font-weight: var(--font-medium);
    line-height: var(--leading-none);
    border-radius: var(--badge-radius);
    white-space: nowrap;
    transition: var(--transition-colors);
  }

  .badge.sm {
    height: 18px;
    padding: 0 var(--space-1);
    font-size: var(--text-2xs);
  }

  .badge.pill {
    border-radius: var(--radius-full);
    padding: 0 var(--space-3);
  }

  .badge.mono {
    font-family: var(--font-mono);
    font-weight: var(--font-normal);
  }

  /* Variant: Default */
  .badge.default {
    color: var(--color-text-secondary);
    background: var(--color-bg-surface);
    border: 1px solid var(--color-border-default);
  }

  .badge.default.outline {
    background: transparent;
  }

  /* Variant: Primary */
  .badge.primary {
    color: var(--color-primary);
    background: var(--color-primary-muted);
    border: 1px solid var(--color-primary-border);
  }

  .badge.primary.outline {
    background: transparent;
  }

  /* Variant: Success */
  .badge.success {
    color: var(--color-success-text);
    background: var(--color-success-bg);
    border: 1px solid var(--color-success-border);
  }

  .badge.success.outline {
    background: transparent;
  }

  /* Variant: Warning */
  .badge.warning {
    color: var(--color-warning-text);
    background: var(--color-warning-bg);
    border: 1px solid var(--color-warning-border);
  }

  .badge.warning.outline {
    background: transparent;
  }

  /* Variant: Danger */
  .badge.danger {
    color: var(--color-danger-text);
    background: var(--color-danger-bg);
    border: 1px solid var(--color-danger-border);
  }

  .badge.danger.outline {
    background: transparent;
  }

  /* Variant: Info */
  .badge.info {
    color: var(--color-info-text);
    background: var(--color-info-bg);
    border: 1px solid var(--color-info-border);
  }

  .badge.info.outline {
    background: transparent;
  }
</style>
