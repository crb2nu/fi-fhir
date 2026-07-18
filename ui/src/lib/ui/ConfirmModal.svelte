<script lang="ts">
  /**
   * ConfirmModal Component
   *
   * Confirmation dialog with customizable title, message, and actions.
   * Includes focus trap and keyboard navigation support.
   */

  import { afterUpdate, createEventDispatcher, tick } from 'svelte';
  import Button from '$lib/ui/Button.svelte';
  import { createDialogFocusController } from '$lib/domain/a11yDialog';

  export let open = false;
  export let title = 'Confirm';
  export let message = 'Are you sure?';
  export let confirmText = 'Confirm';
  export let cancelText = 'Cancel';
  export let variant: 'primary' | 'danger' = 'primary';
  export let loading = false;
  export let confirmDisabled = false;
  export let closeOnConfirm = true;

  const dispatch = createEventDispatcher<{
    confirm: void;
    cancel: void;
  }>();

  let modalEl: HTMLDivElement | null = null;
  let wasOpen = false;
  let focusCtl: ReturnType<typeof createDialogFocusController> | null = null;

  afterUpdate(() => {
    if (open && !wasOpen) {
      tick().then(() => {
        if (!modalEl) return;
        focusCtl = createDialogFocusController(modalEl);
        focusCtl.focusInitial();
      });
    }
    if (!open && wasOpen) {
      focusCtl?.restoreFocus();
      focusCtl = null;
    }
    wasOpen = open;
  });

  function handleConfirm() {
    if (loading || confirmDisabled) return;
    dispatch('confirm');
    if (closeOnConfirm) {
      open = false;
    }
  }

  function handleCancel() {
    if (loading) return;
    dispatch('cancel');
    open = false;
  }

  function handleWindowKeydown(e: KeyboardEvent) {
    if (!open) return;
    if (e.key === 'Escape' && !loading) {
      handleCancel();
    }
    if (e.key === 'Tab') {
      focusCtl?.onKeydown(e);
    }
  }
</script>

<svelte:window on:keydown={handleWindowKeydown} />

{#if open}
  <div class="modal-overlay">
    <button
      type="button"
      class="modal-backdrop"
      tabindex="-1"
      aria-label="Close dialog"
      on:click={handleCancel}
      disabled={loading}
    ></button>
    <div
      class="modal"
      bind:this={modalEl}
      role="dialog"
      aria-modal="true"
      aria-labelledby="modal-title"
      aria-describedby="modal-message"
      tabindex="-1"
    >
      <header class="modal-header">
        <h3 id="modal-title" class="modal-title">{title}</h3>
      </header>

      <div class="modal-body">
        <p id="modal-message">{message}</p>
        <slot />
      </div>

      <footer class="modal-actions">
        <Button variant="secondary" on:click={handleCancel} disabled={loading}>
          {cancelText}
        </Button>
        <Button {variant} on:click={handleConfirm} {loading} disabled={confirmDisabled}>
          {confirmText}
        </Button>
      </footer>
    </div>
  </div>
{/if}

<style>
  .modal-overlay {
    position: fixed;
    inset: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: var(--z-modal);
    padding: var(--space-4);
  }

  .modal-backdrop {
    position: absolute;
    inset: 0;
    border: 0;
    padding: 0;
    background: var(--modal-backdrop);
    cursor: default;
    animation: fadeIn var(--duration-fast) var(--ease-out);
  }

  .modal-backdrop:disabled {
    cursor: not-allowed;
  }

  .modal {
    position: relative;
    z-index: 1;
    background: var(--color-bg-base);
    border: 1px solid var(--color-border-default);
    border-radius: var(--modal-radius);
    width: 100%;
    max-width: var(--modal-width-sm);
    box-shadow: var(--shadow-xl);
    animation: modalIn var(--duration-normal) var(--ease-out);
    outline: none;
  }

  .modal:focus {
    outline: none;
  }

  @keyframes fadeIn {
    from { opacity: 0; }
    to { opacity: 1; }
  }

  @keyframes modalIn {
    from {
      opacity: 0;
      transform: scale(0.95) translateY(-8px);
    }
    to {
      opacity: 1;
      transform: scale(1) translateY(0);
    }
  }

  .modal-header {
    padding: var(--space-5) var(--space-5) 0;
  }

  .modal-title {
    margin: 0;
    font-size: var(--text-lg);
    font-weight: var(--font-bold);
    color: var(--color-text-primary);
    line-height: var(--leading-tight);
  }

  .modal-body {
    padding: var(--space-4) var(--space-5);
  }

  .modal-body p {
    color: var(--color-text-secondary);
    line-height: var(--leading-relaxed);
    margin: 0;
    font-size: var(--text-sm);
  }

  .modal-actions {
    display: flex;
    gap: var(--space-3);
    justify-content: flex-end;
    padding: 0 var(--space-5) var(--space-5);
  }
</style>
