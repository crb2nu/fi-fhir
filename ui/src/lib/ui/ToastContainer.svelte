<script lang="ts">
  /**
   * ToastContainer Component
   *
   * Container for toast notifications with responsive positioning.
   */

  import { toastList, toasts } from './toastStore';
  import Toast from './Toast.svelte';

  function handleDismiss(e: CustomEvent<{ id: string }>) {
    toasts.dismiss(e.detail.id);
  }
</script>

{#if $toastList.length > 0}
  <div class="toast-container" role="region" aria-label="Notifications" aria-live="polite">
    {#each $toastList as toast (toast.id)}
      <Toast
        id={toast.id}
        message={toast.message}
        variant={toast.variant}
        dismissible={toast.dismissible}
        on:dismiss={handleDismiss}
      />
    {/each}
  </div>
{/if}

<style>
  .toast-container {
    position: fixed;
    top: var(--space-4);
    right: var(--space-4);
    z-index: var(--z-toast);
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    pointer-events: none;
    max-width: 400px;
  }

  .toast-container :global(.toast) {
    pointer-events: auto;
  }

  @media (max-width: 480px) {
    .toast-container {
      left: var(--space-4);
      right: var(--space-4);
      max-width: none;
    }
  }
</style>
