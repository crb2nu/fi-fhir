<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import TransformEditor from './TransformEditor.svelte';
  import Button from '$lib/ui/Button.svelte';
  import type { TransformDraft } from '../workflowTypes';

  export let transforms: TransformDraft[];

  const dispatch = createEventDispatcher<{
    add: void;
    remove: { transformKey: string };
    change: { transformKey: string; transform: TransformDraft };
    move: { transformKey: string; direction: 'up' | 'down' };
  }>();
</script>

<div class="transform-list">
  {#each transforms as transform, i (transform._key)}
    <div class="transform-item">
      <div class="transform-header">
        <span class="transform-index">Transform {i + 1}</span>
        <div class="transform-controls">
          <button
            type="button"
            class="icon-btn"
            disabled={i === 0}
            on:click={() => dispatch('move', { transformKey: transform._key, direction: 'up' })}
            aria-label={`Move transform ${i + 1} up`}
            title={`Move transform ${i + 1} up`}
          >
            &uarr;
          </button>
          <button
            type="button"
            class="icon-btn"
            disabled={i === transforms.length - 1}
            on:click={() => dispatch('move', { transformKey: transform._key, direction: 'down' })}
            aria-label={`Move transform ${i + 1} down`}
            title={`Move transform ${i + 1} down`}
          >
            &darr;
          </button>
          <button
            type="button"
            class="icon-btn danger"
            on:click={() => dispatch('remove', { transformKey: transform._key })}
            aria-label={`Remove transform ${i + 1}`}
            title={`Remove transform ${i + 1}`}
          >
            &times;
          </button>
        </div>
      </div>
      <TransformEditor
        {transform}
        on:change={(e) => dispatch('change', { transformKey: transform._key, transform: e.detail })}
      />
    </div>
  {/each}

  <Button variant="secondary" on:click={() => dispatch('add')}>
    + Add Transform
  </Button>
</div>

<style>
  .transform-list {
    display: grid;
    gap: 10px;
  }

  .transform-item {
    padding: 10px 12px;
    border-radius: 8px;
    border: 1px solid var(--color-border-default);
    background: var(--color-bg-elevated);
  }

  .transform-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 8px;
  }

  .transform-index {
    color: var(--color-text-muted);
    font-size: 0.8rem;
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 0.05em;
  }

  .transform-controls {
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
