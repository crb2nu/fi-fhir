<script lang="ts">
  import { createEventDispatcher, tick } from 'svelte';
  import Button from '$lib/ui/Button.svelte';

  export let open = false;
  export let title = 'Confirm';
  export let message = 'Are you sure?';
  export let confirmText = 'Confirm';
  export let cancelText = 'Cancel';
  export let variant: 'primary' | 'danger' = 'primary';

  const dispatch = createEventDispatcher<{
    confirm: void;
    cancel: void;
  }>();

  let modalEl: HTMLDivElement | null = null;
  let wasOpen = false;

  $: if (open && !wasOpen) {
    tick().then(() => modalEl?.focus());
  }

  $: wasOpen = open;

  function handleConfirm() {
    dispatch('confirm');
    open = false;
  }

  function handleCancel() {
    dispatch('cancel');
    open = false;
  }

  function handleWindowKeydown(e: KeyboardEvent) {
    if (!open) return;
    if (e.key === 'Escape') {
      handleCancel();
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
      <h3 id="modal-title" class="modal-title">{title}</h3>
      <div class="modal-body">
        <p id="modal-message">{message}</p>
        <slot />
      </div>
      <div class="modal-actions">
        <Button variant="secondary" on:click={handleCancel}>{cancelText}</Button>
        <Button variant={variant} on:click={handleConfirm}>{confirmText}</Button>
      </div>
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
    z-index: 1000;
  }

  .modal-backdrop {
    position: absolute;
    inset: 0;
    border: 0;
    padding: 0;
    background: rgba(0, 0, 0, 0.6);
    cursor: default;
  }

  .modal {
    position: relative;
    z-index: 1;
    background: #1f2937;
    border: 1px solid rgba(255, 255, 255, 0.1);
    border-radius: 16px;
    padding: 24px;
    min-width: 360px;
    max-width: 480px;
    animation: slideIn 0.15s ease-out;
  }

  @keyframes slideIn {
    from {
      opacity: 0;
      transform: scale(0.95) translateY(-10px);
    }
    to {
      opacity: 1;
      transform: scale(1) translateY(0);
    }
  }

  .modal-title {
    margin: 0 0 16px;
    font-size: 1.1rem;
    font-weight: 800;
    color: #f3f4f6;
  }

  .modal-body {
    margin-bottom: 20px;
  }

  .modal-body p {
    color: rgba(229, 231, 235, 0.85);
    line-height: 1.5;
    margin: 0;
  }

  .modal-actions {
    display: flex;
    gap: 10px;
    justify-content: flex-end;
  }
</style>
