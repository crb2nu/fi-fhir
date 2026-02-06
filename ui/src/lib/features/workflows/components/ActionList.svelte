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
            class="icon-btn"
            disabled={i === 0}
            on:click={() => dispatch('move', { actionKey: action._key, direction: 'up' })}
            aria-label="Move up"
            title="Move up"
          >
            &uarr;
          </button>
          <button
            class="icon-btn"
            disabled={i === actions.length - 1}
            on:click={() => dispatch('move', { actionKey: action._key, direction: 'down' })}
            aria-label="Move down"
            title="Move down"
          >
            &darr;
          </button>
          <button
            class="icon-btn danger"
            on:click={() => dispatch('remove', { actionKey: action._key })}
            aria-label="Remove action"
            title="Remove action"
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
    border: 1px solid rgba(255, 255, 255, 0.08);
    background: rgba(255, 255, 255, 0.02);
  }

  .action-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 8px;
  }

  .action-index {
    color: rgba(229, 231, 235, 0.55);
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
    border: 1px solid rgba(255, 255, 255, 0.1);
    background: transparent;
    color: rgba(229, 231, 235, 0.6);
    cursor: pointer;
    font-size: 0.85rem;
    line-height: 1;
  }

  .icon-btn:hover:not(:disabled) {
    background: rgba(255, 255, 255, 0.06);
    color: rgba(229, 231, 235, 0.9);
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
