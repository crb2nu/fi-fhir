<!--
  Button — primary | secondary | ghost | danger, sm (28px) | md (32px).
  One primary per view. `md` is for the primary action of a dialog.
  `iconOnly` makes it square; give it an `aria-label` (or use IconButton).
  `loading` disables it and sets aria-busy while keeping its width.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { HTMLButtonAttributes } from 'svelte/elements';
  import Icon from './Icon.svelte';
  import type { ButtonVariant, ControlSize, IconComponent } from './types';

  interface Props extends HTMLButtonAttributes {
    variant?: ButtonVariant;
    size?: ControlSize;
    iconOnly?: boolean;
    loading?: boolean;
    /** Leading icon (16px). */
    icon?: IconComponent | undefined;
    children?: Snippet;
  }

  let {
    variant = 'secondary',
    size = 'sm',
    iconOnly = false,
    loading = false,
    icon,
    type = 'button',
    disabled = false,
    class: className,
    children,
    ...rest
  }: Props = $props();
</script>

<button
  {...rest}
  {type}
  class={[
    'ui-button',
    `ui-button--${variant}`,
    `ui-button--${size}`,
    { 'ui-button--icon-only': iconOnly, 'is-loading': loading },
    className
  ]}
  disabled={disabled || loading}
  aria-busy={loading ? 'true' : undefined}
  data-variant={variant}
>
  {#if loading}
    <span class="ui-button-spinner" aria-hidden="true"></span>
  {:else if icon}
    <Icon {icon} />
  {/if}
  {@render children?.()}
</button>

<style>
  .ui-button {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: 6px;
    height: var(--size-control-sm);
    padding: 0 10px;
    border: 1px solid transparent;
    border-radius: var(--radius-sm);
    font: inherit;
    font-size: var(--text-ui);
    font-weight: var(--font-medium);
    line-height: 1;
    white-space: nowrap;
    cursor: pointer;
    user-select: none;
    transition: var(--transition-colors);
  }

  .ui-button--md {
    height: var(--size-control-md);
    padding: 0 14px;
  }

  .ui-button--icon-only {
    width: var(--size-control-sm);
    padding: 0;
  }

  .ui-button--md.ui-button--icon-only {
    width: var(--size-control-md);
  }

  .ui-button:focus-visible {
    outline: 2px solid var(--color-focus-ring);
    outline-offset: 1px;
  }

  .ui-button:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  .ui-button.is-loading {
    cursor: progress;
  }

  /* primary — the accent fill; one per view */
  .ui-button--primary {
    background: var(--color-primary);
    color: var(--color-text-inverse);
  }

  .ui-button--primary:hover:not(:disabled) {
    background: var(--color-primary-hover);
  }

  /* secondary — bordered neutral */
  .ui-button--secondary {
    background: var(--color-bg-surface);
    border-color: var(--color-border-default);
    color: var(--color-text-primary);
  }

  .ui-button--secondary:hover:not(:disabled) {
    background: var(--color-bg-hover);
    border-color: var(--color-border-strong);
  }

  /* ghost — toolbars, rows, dense chrome */
  .ui-button--ghost {
    background: transparent;
    color: var(--color-text-secondary);
  }

  .ui-button--ghost:hover:not(:disabled) {
    background: var(--color-bg-hover);
    color: var(--color-text-primary);
  }

  .ui-button[aria-pressed='true'] {
    background: var(--color-bg-active);
    color: var(--color-text-primary);
  }

  /* danger — destructive; subtle until hovered */
  .ui-button--danger {
    background: var(--color-danger-bg);
    border-color: var(--color-danger-border);
    color: var(--color-danger-text);
  }

  .ui-button--danger:hover:not(:disabled) {
    background: var(--color-danger);
    border-color: var(--color-danger);
    color: var(--color-text-inverse);
  }

  .ui-button-spinner {
    width: 12px;
    height: 12px;
    border: 1.5px solid currentColor;
    border-right-color: transparent;
    border-radius: 50%;
    animation: spin 0.7s linear infinite;
  }
</style>
