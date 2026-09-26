<script lang="ts">
  /**
   * ToastContainer Component
   *
   * Notifications stack in the bottom-right corner above the status bar,
   * the workbench convention (VS Code, Grafana).
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
    right: var(--space-3);
    bottom: calc(var(--statusbar-height, 24px) + var(--space-2));
    z-index: var(--z-toast);
    display: flex;
    flex-direction: column;
    align-items: flex-end;
    gap: var(--space-2);
    pointer-events: none;
    max-width: 400px;
  }

  .toast-container :global(.toast) {
    pointer-events: auto;
  }

  @media (max-width: 480px) {
    .toast-container {
      left: var(--space-3);
      right: var(--space-3);
      max-width: none;
    }
  }
</style>
