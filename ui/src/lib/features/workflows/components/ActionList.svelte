<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import ActionEditor from './ActionEditor.svelte';
  import Button from '$lib/ui/Button.svelte';
  import type { ActionDraft } from '../workflowTypes';

  export let actions: ActionDraft[];

  const dispatch = createEventDispatcher<{
    add: void;
    remove: { actionKey: string };
    change: { actionKey: string; action: ActionDraft };
    move: { actionKey: string; direction: 'up' | 'down' };
  }>();
</script>

<div class="action-list">
  {#each actions as action, i (action._key)}
    <div class="action-item">
      <div class="action-header">
        <span class="action-index">Action {i + 1}</span>
        <div class="action-controls">
          <button
            type="button"
            class="icon-btn"
            disabled={i === 0}
            on:click={() => dispatch('move', { actionKey: action._key, direction: 'up' })}
            aria-label={`Move action ${i + 1} up`}
            title={`Move action ${i + 1} up`}
          >
            &uarr;
          </button>
          <button
            type="button"
            class="icon-btn"
            disabled={i === actions.length - 1}
            on:click={() => dispatch('move', { actionKey: action._key, direction: 'down' })}
            aria-label={`Move action ${i + 1} down`}
            title={`Move action ${i + 1} down`}
          >
            &darr;
          </button>
          <button
            type="button"
            class="icon-btn danger"
            on:click={() => dispatch('remove', { actionKey: action._key })}
            aria-label={`Remove action ${i + 1}`}
            title={`Remove action ${i + 1}`}
          >
            &times;
          </button>
        </div>
      </div>
      <ActionEditor
        {action}
        on:change={(e) => dispatch('change', { actionKey: action._key, action: e.detail })}
      />
    </div>
  {/each}

  <Button variant="secondary" on:click={() => dispatch('add')}>
    + Add Action
  </Button>
</div>

<style>
  .action-list {
    display: grid;
    gap: 10px;
  }

  .action-item {
    padding: 10px 12px;
    border-radius: 8px;
    border: 1px solid var(--color-border-default);
    background: var(--color-bg-elevated);
  }

  .action-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 8px;
  }

  .action-index {
    color: var(--color-text-muted);
    font-size: 0.8rem;
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 0.05em;
  }

  .action-controls {
    display: flex;
    gap: 6px;
  }

  .icon-btn {
    width: 28px;
    height: 28px;
    min-width: 28px;
    display: flex;
    align-items: center;
    justify-content: center;
    border-radius: 4px;
    border: 1px solid var(--color-border-default);
    background: transparent;
    color: var(--color-text-tertiary);
    cursor: pointer;
    font-size: 0.85rem;
    line-height: 1;
    transition: var(--transition-all);
  }

  .icon-btn:hover:not(:disabled) {
    background: var(--color-bg-hover);
    color: var(--color-text-primary);
  }

  .icon-btn:focus-visible {
    outline: none;
    box-shadow: var(--shadow-focus);
  }

  .icon-btn:disabled {
    opacity: 0.3;
    cursor: not-allowed;
  }

  .icon-btn.danger:hover:not(:disabled) {
    background: rgba(239, 68, 68, 0.12);
    border-color: rgba(239, 68, 68, 0.3);
    color: rgba(254, 202, 202, 0.9);
  }
</style>
