<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import CircleAlert from '@lucide/svelte/icons/circle-alert';
  import CircleCheck from '@lucide/svelte/icons/circle-check';
  import Info from '@lucide/svelte/icons/info';
  import TriangleAlert from '@lucide/svelte/icons/triangle-alert';
  import X from '@lucide/svelte/icons/x';
  import Icon from '$lib/ui/primitives/Icon.svelte';
  import type { ToastVariant } from './toastStore';

  /**
   * One notification: a state icon, one line of text, a dismiss button. The
   * variant colours the icon and a 2 px left edge only; the surface stays
   * neutral. `.toast.<variant>` class names are part of the e2e contract
   * (the browser gate records every `.toast.error`).
   */

  export let id: string;
  export let message: string;
  export let variant: ToastVariant = 'info';
  export let dismissible = true;

  const dispatch = createEventDispatcher<{ dismiss: { id: string } }>();

  const icons = {
    success: CircleCheck,
    error: CircleAlert,
    warning: TriangleAlert,
    info: Info
  } as const;

  function handleDismiss() {
    dispatch('dismiss', { id });
  }
</script>

<div class="toast {variant}" role="alert" aria-live="polite">
  <Icon icon={icons[variant] ?? Info} class="toast-icon" />

  <span class="message">{message}</span>

  {#if dismissible}
    <button class="dismiss" type="button" on:click={handleDismiss} aria-label="Dismiss notification">
      <Icon icon={X} size={14} />
    </button>
  {/if}
</div>

<style>
  .toast {
    display: flex;
    align-items: flex-start;
    gap: var(--space-2);
    min-width: 280px;
    max-width: 380px;
    padding: var(--space-2) var(--space-2) var(--space-2) var(--space-3);
    border: 1px solid var(--color-border-default);
    border-left: 2px solid var(--toast-edge, var(--color-border-strong));
    border-radius: var(--radius-md);
    background: var(--color-bg-overlay);
    box-shadow: var(--shadow-lg);
    color: var(--color-text-primary);
    font-size: var(--text-ui);
    line-height: var(--leading-ui);
    animation: fadeIn var(--duration-normal) var(--ease-out);
  }

  .toast.success {
    --toast-edge: var(--color-success);
    --toast-icon: var(--color-success-text);
  }

  .toast.error {
    --toast-edge: var(--color-danger);
    --toast-icon: var(--color-danger-text);
  }

  .toast.warning {
    --toast-edge: var(--color-warning);
    --toast-icon: var(--color-warning-text);
  }

  .toast.info {
    --toast-edge: var(--color-info);
    --toast-icon: var(--color-info-text);
  }

  .toast :global(.toast-icon) {
    margin-top: 2px;
    color: var(--toast-icon, var(--color-text-tertiary));
  }

  .message {
    flex: 1;
    min-width: 0;
    word-break: break-word;
  }

  .dismiss {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
    width: 20px;
    height: 20px;
    padding: 0;
    border: none;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--color-text-tertiary);
    cursor: pointer;
    transition: var(--transition-colors);
  }

  .dismiss:hover {
    background: var(--color-bg-hover);
    color: var(--color-text-primary);
  }

  .dismiss:focus-visible {
    outline: 2px solid var(--color-focus-ring);
    outline-offset: 1px;
  }

  @media (prefers-reduced-motion: reduce) {
    .toast {
      animation: none;
    }
  }
</style>
