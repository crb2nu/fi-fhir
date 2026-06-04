<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import type { ToastVariant } from './toastStore';

  export let id: string;
  export let message: string;
  export let variant: ToastVariant = 'info';
  export let dismissible = true;

  const dispatch = createEventDispatcher<{ dismiss: { id: string } }>();

  function handleDismiss() {
    dispatch('dismiss', { id });
  }
</script>

<div class="toast {variant}" role="alert" aria-live="polite">
  <div class="icon">
    {#if variant === 'success'}
      <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor" class="icon-svg">
        <path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.857-9.809a.75.75 0 00-1.214-.882l-3.483 4.79-1.88-1.88a.75.75 0 10-1.06 1.061l2.5 2.5a.75.75 0 001.137-.089l4-5.5z" clip-rule="evenodd" />
      </svg>
    {:else if variant === 'error'}
      <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor" class="icon-svg">
        <path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.28 7.22a.75.75 0 00-1.06 1.06L8.94 10l-1.72 1.72a.75.75 0 101.06 1.06L10 11.06l1.72 1.72a.75.75 0 101.06-1.06L11.06 10l1.72-1.72a.75.75 0 00-1.06-1.06L10 8.94 8.28 7.22z" clip-rule="evenodd" />
      </svg>
    {:else if variant === 'warning'}
      <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor" class="icon-svg">
        <path fill-rule="evenodd" d="M8.485 2.495c.673-1.167 2.357-1.167 3.03 0l6.28 10.875c.673 1.167-.17 2.625-1.516 2.625H3.72c-1.347 0-2.189-1.458-1.515-2.625L8.485 2.495zM10 5a.75.75 0 01.75.75v3.5a.75.75 0 01-1.5 0v-3.5A.75.75 0 0110 5zm0 9a1 1 0 100-2 1 1 0 000 2z" clip-rule="evenodd" />
      </svg>
    {:else}
      <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor" class="icon-svg">
        <path fill-rule="evenodd" d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zm-7-4a1 1 0 11-2 0 1 1 0 012 0zM9 9a.75.75 0 000 1.5h.253a.25.25 0 01.244.304l-.459 2.066A1.75 1.75 0 0010.747 15H11a.75.75 0 000-1.5h-.253a.25.25 0 01-.244-.304l.459-2.066A1.75 1.75 0 009.253 9H9z" clip-rule="evenodd" />
      </svg>
    {/if}
  </div>

  <span class="message">{message}</span>

  {#if dismissible}
    <button class="dismiss" type="button" on:click={handleDismiss} aria-label="Dismiss notification">
      <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor" class="dismiss-icon">
        <path d="M6.28 5.22a.75.75 0 00-1.06 1.06L8.94 10l-3.72 3.72a.75.75 0 101.06 1.06L10 11.06l3.72 3.72a.75.75 0 101.06-1.06L11.06 10l3.72-3.72a.75.75 0 00-1.06-1.06L10 8.94 6.28 5.22z" />
      </svg>
    </button>
  {/if}
</div>

<style>
  .toast {
    display: flex;
    align-items: flex-start;
    gap: 10px;
    padding: 12px 14px;
    border-radius: 12px;
    background: rgba(30, 41, 59, 0.95);
    border: 1px solid rgba(255, 255, 255, 0.1);
    box-shadow: 0 10px 40px rgba(0, 0, 0, 0.35);
    backdrop-filter: blur(8px);
    animation: slideIn 0.25s ease-out;
    max-width: 360px;
    min-width: 280px;
  }

  @keyframes slideIn {
    from {
      opacity: 0;
      transform: translateY(-10px);
    }
    to {
      opacity: 1;
      transform: translateY(0);
    }
  }

  .toast.success {
    border-color: rgba(16, 185, 129, 0.35);
  }

  .toast.error {
    border-color: rgba(239, 68, 68, 0.45);
  }

  .toast.warning {
    border-color: rgba(245, 158, 11, 0.4);
  }

  .toast.info {
    border-color: rgba(59, 130, 246, 0.35);
  }

  .icon {
    flex-shrink: 0;
    width: 20px;
    height: 20px;
  }

  .icon-svg {
    width: 100%;
    height: 100%;
  }

  .toast.success .icon {
    color: rgba(16, 185, 129, 0.9);
  }

  .toast.error .icon {
    color: rgba(239, 68, 68, 0.9);
  }

  .toast.warning .icon {
    color: rgba(245, 158, 11, 0.9);
  }

  .toast.info .icon {
    color: rgba(59, 130, 246, 0.9);
  }

  .message {
    flex: 1;
    color: rgba(243, 244, 246, 0.95);
    font-size: 0.9rem;
    line-height: 1.4;
  }

  .dismiss {
    flex-shrink: 0;
    width: 20px;
    height: 20px;
    padding: 0;
    border: none;
    background: transparent;
    cursor: pointer;
    color: rgba(156, 163, 175, 0.7);
    transition: color 0.15s ease;
  }

  .dismiss:hover {
    color: rgba(243, 244, 246, 0.95);
  }

  .dismiss:focus-visible {
    outline: none;
    box-shadow: var(--shadow-focus);
    border-radius: var(--radius-sm);
    color: rgba(243, 244, 246, 0.95);
  }

  .dismiss-icon {
    width: 100%;
    height: 100%;
  }
</style>
