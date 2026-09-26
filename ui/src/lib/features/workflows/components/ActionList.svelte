<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import ArrowDown from '@lucide/svelte/icons/arrow-down';
  import ArrowUp from '@lucide/svelte/icons/arrow-up';
  import Plus from '@lucide/svelte/icons/plus';
  import X from '@lucide/svelte/icons/x';
  import { Button, IconButton } from '$lib/ui/primitives';
  import ActionEditor from './ActionEditor.svelte';
  import type { ActionDraft } from '../workflowTypes';

  export let actions: ActionDraft[];

  const dispatch = createEventDispatcher<{
    add: void;
    remove: { actionKey: string };
    change: { actionKey: string; action: ActionDraft };
    move: { actionKey: string; direction: 'up' | 'down' };
  }>();
</script>

<div class="item-list">
  {#each actions as action, i (action._key)}
    <div class="item">
      <div class="item-head">
        <span class="item-index">Action {i + 1}</span>
        <div class="item-controls">
          <IconButton
            icon={ArrowUp}
            label={`Move action ${i + 1} up`}
            disabled={i === 0}
            onclick={() => dispatch('move', { actionKey: action._key, direction: 'up' })}
          />
          <IconButton
            icon={ArrowDown}
            label={`Move action ${i + 1} down`}
            disabled={i === actions.length - 1}
            onclick={() => dispatch('move', { actionKey: action._key, direction: 'down' })}
          />
          <IconButton
            icon={X}
            label={`Remove action ${i + 1}`}
            onclick={() => dispatch('remove', { actionKey: action._key })}
          />
        </div>
      </div>
      <div class="item-body">
        <ActionEditor
          {action}
          on:change={(e) => dispatch('change', { actionKey: action._key, action: e.detail })}
        />
      </div>
    </div>
  {/each}

  <div>
    <Button variant="ghost" icon={Plus} onclick={() => dispatch('add')}>Add action</Button>
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
