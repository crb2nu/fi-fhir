<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import ArrowDown from '@lucide/svelte/icons/arrow-down';
  import ArrowUp from '@lucide/svelte/icons/arrow-up';
  import Plus from '@lucide/svelte/icons/plus';
  import X from '@lucide/svelte/icons/x';
  import { Button, IconButton } from '$lib/ui/primitives';
  import TransformEditor from './TransformEditor.svelte';
  import type { TransformDraft } from '../workflowTypes';

  export let transforms: TransformDraft[];

  const dispatch = createEventDispatcher<{
    add: void;
    remove: { transformKey: string };
    change: { transformKey: string; transform: TransformDraft };
    move: { transformKey: string; direction: 'up' | 'down' };
  }>();
</script>

<div class="item-list">
  {#each transforms as transform, i (transform._key)}
    <div class="item">
      <div class="item-head">
        <span class="item-index">Transform {i + 1}</span>
        <div class="item-controls">
          <IconButton
            icon={ArrowUp}
            label={`Move transform ${i + 1} up`}
            disabled={i === 0}
            onclick={() => dispatch('move', { transformKey: transform._key, direction: 'up' })}
          />
          <IconButton
            icon={ArrowDown}
            label={`Move transform ${i + 1} down`}
            disabled={i === transforms.length - 1}
            onclick={() => dispatch('move', { transformKey: transform._key, direction: 'down' })}
          />
          <IconButton
            icon={X}
            label={`Remove transform ${i + 1}`}
            onclick={() => dispatch('remove', { transformKey: transform._key })}
          />
        </div>
      </div>
      <div class="item-body">
        <TransformEditor
          {transform}
          on:change={(e) => dispatch('change', { transformKey: transform._key, transform: e.detail })}
        />
      </div>
    </div>
  {/each}

  <div>
    <Button variant="ghost" icon={Plus} onclick={() => dispatch('add')}>Add transform</Button>
  </div>
</div>

<style>
  .item-list {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  .item {
    border: 1px solid var(--color-border-subtle);
    border-radius: var(--radius-sm);
    background: var(--color-bg-surface);
  }

  .item-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    height: 32px;
    padding: 0 var(--space-1) 0 var(--space-3);
    border-bottom: 1px solid var(--color-border-subtle);
  }

  .item-index {
    font-size: var(--text-label);
    font-weight: var(--font-semibold);
    letter-spacing: var(--tracking-label);
    text-transform: uppercase;
    color: var(--color-text-tertiary);
  }

  .item-controls {
    display: flex;
    gap: 2px;
  }

  .item-body {
    padding: var(--space-3);
  }
</style>
