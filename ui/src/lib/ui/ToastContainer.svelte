<script lang="ts">
  import { toastList, toasts } from './toastStore';
  import Toast from './Toast.svelte';

  function handleDismiss(e: CustomEvent<{ id: string }>) {
    toasts.dismiss(e.detail.id);
  }
</script>

{#if $toastList.length > 0}
  <div class="toast-container" role="region" aria-label="Notifications">
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
    top: 16px;
    right: 16px;
    z-index: 9999;
    display: flex;
    flex-direction: column;
    gap: 10px;
    pointer-events: none;
  }

  .toast-container :global(.toast) {
    pointer-events: auto;
  }

  @media (max-width: 480px) {
    .toast-container {
      left: 16px;
      right: 16px;
    }

    .toast-container :global(.toast) {
      max-width: none;
    }
  }
</style>
