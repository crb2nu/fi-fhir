<script lang="ts">
  /**
   * Button Component
   *
   * Primary interactive element with multiple variants, sizes,
   * and optional loading/icon states.
   */

  export let variant: 'primary' | 'secondary' | 'danger' | 'ghost' = 'primary';
  export let size: 'sm' | 'md' | 'lg' = 'md';
  export let disabled = false;
  export let loading = false;
  export let fullWidth = false;
  export let type: 'button' | 'submit' | 'reset' = 'button';
</script>

<button
  {type}
  class="btn {variant} {size}"
  class:full-width={fullWidth}
  class:loading
  disabled={disabled || loading}
  on:click
  on:mouseenter
  on:mouseleave
  {...$$restProps}
>
  {#if loading}
    <span class="spinner" aria-hidden="true">
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
        <circle cx="12" cy="12" r="10" opacity="0.25" />
        <path d="M12 2a10 10 0 0 1 10 10" />
      </svg>
    </span>
  {/if}

  {#if $$slots.icon && !loading}
    <span class="icon">
      <slot name="icon" />
    </span>
  {/if}

  <span class="label" class:sr-only={loading && !$$slots.default}>
    <slot />
  </span>
</button>

<style>
  .btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: var(--space-2);
    border-radius: var(--radius-lg);
    border: 1px solid transparent;
    cursor: pointer;
    font-weight: var(--font-semibold);
    font-family: inherit;
    line-height: var(--leading-none);
    white-space: nowrap;
    transition: var(--transition-all);
    user-select: none;
  }

  .btn.full-width {
    width: 100%;
  }

  /* Size variants */
  .btn.sm {
    height: var(--btn-height-sm);
    padding: 0 var(--btn-padding-x-sm);
    font-size: var(--text-xs);
    border-radius: var(--radius-md);
  }

  .btn.md {
    height: var(--btn-height-md);
    padding: 0 var(--btn-padding-x-md);
    font-size: var(--text-sm);
  }

  .btn.lg {
    height: var(--btn-height-lg);
    padding: 0 var(--btn-padding-x-lg);
    font-size: var(--text-base);
  }

  /* Icon sizing */
  .icon {
    display: flex;
    flex-shrink: 0;
  }

  .btn.sm .icon :global(svg) {
    width: 14px;
    height: 14px;
  }

  .btn.md .icon :global(svg) {
    width: 16px;
    height: 16px;
  }

  .btn.lg .icon :global(svg) {
    width: 18px;
    height: 18px;
  }

  /* Variant: Primary */
  .btn.primary {
    color: var(--color-text-primary);
    background: var(--color-bg-surface);
    border-color: var(--color-border-strong);
  }

  .btn.primary:hover:not(:disabled) {
    background: var(--color-bg-hover);
    border-color: var(--color-primary-border);
    transform: translateY(-1px);
  }

  .btn.primary:active:not(:disabled) {
    transform: translateY(0);
    background: var(--color-bg-active);
  }

  /* Variant: Secondary */
  .btn.secondary {
    color: var(--color-text-secondary);
    background: var(--color-bg-elevated);
    border-color: var(--color-border-default);
  }

  .btn.secondary:hover:not(:disabled) {
    background: var(--color-bg-hover);
    border-color: var(--color-border-strong);
  }

  .btn.secondary:active:not(:disabled) {
    background: var(--color-bg-active);
  }

  /* Variant: Danger */
  .btn.danger {
    color: var(--color-danger-text);
    background: var(--color-danger-bg);
    border-color: var(--color-danger-border);
  }

  .btn.danger:hover:not(:disabled) {
    background: rgba(239, 68, 68, 0.18);
    border-color: rgba(239, 68, 68, 0.5);
  }

  .btn.danger:active:not(:disabled) {
    background: rgba(239, 68, 68, 0.25);
  }

  /* Variant: Ghost */
  .btn.ghost {
    color: var(--color-text-secondary);
    background: transparent;
    border-color: transparent;
  }

  .btn.ghost:hover:not(:disabled) {
    background: var(--color-bg-hover);
    color: var(--color-text-primary);
  }

  .btn.ghost:active:not(:disabled) {
    background: var(--color-bg-active);
  }

  /* Disabled state */
  .btn:disabled {
    opacity: 0.5;
    cursor: not-allowed;
    transform: none;
  }

  /* Focus state */
  .btn:focus-visible {
    outline: none;
    box-shadow: var(--shadow-focus);
    border-color: var(--color-border-focus);
  }

  .btn.danger:focus-visible {
    box-shadow: var(--shadow-focus-danger);
    border-color: var(--color-danger);
  }

  /* Loading state */
  .btn.loading {
    position: relative;
  }

  .spinner {
    display: flex;
    animation: spin 0.8s linear infinite;
  }

  .btn.sm .spinner svg {
    width: 14px;
    height: 14px;
  }

  .btn.md .spinner svg {
    width: 16px;
    height: 16px;
  }

  .btn.lg .spinner svg {
    width: 18px;
    height: 18px;
  }

  .label {
    display: inline-flex;
    align-items: center;
  }

  /* Screen reader only (for loading state) */
  .sr-only {
    position: absolute;
    width: 1px;
    height: 1px;
    padding: 0;
    margin: -1px;
    overflow: hidden;
    clip: rect(0, 0, 0, 0);
    white-space: nowrap;
    border: 0;
  }

  @keyframes spin {
    from { transform: rotate(0deg); }
    to { transform: rotate(360deg); }
  }
</style>
