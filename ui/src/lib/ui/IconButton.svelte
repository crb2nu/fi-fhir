<script lang="ts">
  /**
   * IconButton Component
   *
   * Icon-only button with tooltip support.
   * Use for compact action buttons in toolbars, tables, etc.
   */

  export let variant: 'default' | 'primary' | 'danger' | 'ghost' = 'default';
  export let size: 'sm' | 'md' | 'lg' = 'md';
  export let disabled = false;
  export let loading = false;
  export let label: string;
  /** @deprecated Use native title attribute instead */
  export const tooltipPosition: 'top' | 'bottom' | 'left' | 'right' = 'top';
  export let showTooltip = true;
</script>

<button
  class="icon-button {variant} {size}"
  {disabled}
  aria-label={label}
  title={showTooltip ? label : undefined}
  on:click
  on:mouseenter
  on:mouseleave
  {...$$restProps}
>
  {#if loading}
    <span class="spinner" aria-hidden="true">
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
        <circle cx="12" cy="12" r="10" opacity="0.25" />
        <path d="M12 2a10 10 0 0 1 10 10" />
      </svg>
    </span>
  {:else}
    <slot />
  {/if}
</button>

<style>
  .icon-button {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    border: 1px solid transparent;
    border-radius: var(--radius-md);
    cursor: pointer;
    transition: var(--transition-all);
    flex-shrink: 0;
  }

  .icon-button :global(svg) {
    width: 100%;
    height: 100%;
  }

  /* Size variants */
  .icon-button.sm {
    width: 28px;
    height: 28px;
    padding: 5px;
  }

  .icon-button.md {
    width: 36px;
    height: 36px;
    padding: 8px;
  }

  .icon-button.lg {
    width: 44px;
    height: 44px;
    padding: 10px;
  }

  /* Variant: Default */
  .icon-button.default {
    color: var(--color-text-tertiary);
    background: transparent;
    border-color: transparent;
  }

  .icon-button.default:hover:not(:disabled) {
    color: var(--color-text-primary);
    background: var(--color-bg-hover);
    border-color: var(--color-border-subtle);
  }

  .icon-button.default:active:not(:disabled) {
    background: var(--color-bg-active);
  }

  /* Variant: Primary */
  .icon-button.primary {
    color: var(--color-primary);
    background: var(--color-primary-muted);
    border-color: var(--color-primary-border);
  }

  .icon-button.primary:hover:not(:disabled) {
    background: rgba(59, 130, 246, 0.25);
  }

  .icon-button.primary:active:not(:disabled) {
    background: rgba(59, 130, 246, 0.3);
  }

  /* Variant: Danger */
  .icon-button.danger {
    color: var(--color-text-tertiary);
    background: transparent;
    border-color: transparent;
  }

  .icon-button.danger:hover:not(:disabled) {
    color: var(--color-danger-text);
    background: var(--color-danger-bg);
    border-color: var(--color-danger-border);
  }

  .icon-button.danger:active:not(:disabled) {
    background: rgba(239, 68, 68, 0.2);
  }

  /* Variant: Ghost */
  .icon-button.ghost {
    color: var(--color-text-muted);
    background: transparent;
    border-color: transparent;
  }

  .icon-button.ghost:hover:not(:disabled) {
    color: var(--color-text-secondary);
    background: var(--color-bg-elevated);
  }

  /* Disabled state */
  .icon-button:disabled {
    opacity: 0.4;
    cursor: not-allowed;
  }

  /* Focus state */
  .icon-button:focus-visible {
    outline: none;
    box-shadow: var(--shadow-focus);
    border-color: var(--color-border-focus);
  }

  /* Loading spinner */
  .spinner {
    display: flex;
    animation: spin 1s linear infinite;
  }

  .spinner svg {
    width: 100%;
    height: 100%;
  }

  @keyframes spin {
    from { transform: rotate(0deg); }
    to { transform: rotate(360deg); }
  }
</style>
