<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import FilterEditor from './FilterEditor.svelte';
  import TransformList from './TransformList.svelte';
  import ActionList from './ActionList.svelte';
  import type { RouteDraft, FilterDraft, ActionDraft, TransformDraft } from '../workflowTypes';

  export let route: RouteDraft;

  const dispatch = createEventDispatcher<{
    toggleExpand: void;
    remove: void;
    updateName: string;
    updateFilter: FilterDraft;
    addTransform: void;
    removeTransform: { transformKey: string };
    changeTransform: { transformKey: string; transform: TransformDraft };
    moveTransform: { transformKey: string; direction: 'up' | 'down' };
    addAction: void;
    removeAction: { actionKey: string };
    changeAction: { actionKey: string; action: ActionDraft };
    moveAction: { actionKey: string; direction: 'up' | 'down' };
    moveRoute: 'up' | 'down';
  }>();

  function actionSummary(route: RouteDraft): string {
    if (route.actions.length === 0) return 'No actions';
    return route.actions.map((a) => a.type).join(', ');
  }

  function filterSummary(route: RouteDraft): string {
    if (route.filter.eventTypes.length === 0) return 'All events';
    if (route.filter.eventTypes.length <= 3) {
      return route.filter.eventTypes.map((t) => t.replace(/_/g, ' ')).join(', ');
    }
    return `${route.filter.eventTypes.length} event types`;
  }
</script>

<div class="route-editor" class:expanded={route.expanded}>
  <div class="route-header-row">
    <button class="route-header" on:click={() => dispatch('toggleExpand')} aria-expanded={route.expanded}>
      <span class="collapse-icon" class:rotated={route.expanded}>&#9654;</span>
      <span class="route-name">{route.name || 'Unnamed route'}</span>
      {#if !route.expanded}
        <span class="summary">
          <span class="summary-filter">{filterSummary(route)}</span>
          {#if route.transforms.length > 0}
            <span class="summary-sep">&rarr;</span>
            <span class="summary-transforms">{route.transforms.length} transform{route.transforms.length > 1 ? 's' : ''}</span>
          {/if}
          <span class="summary-sep">&rarr;</span>
          <span class="summary-actions">{actionSummary(route)}</span>
          {#if route.actions.length === 0}
            <span class="summary-warn">needs actions</span>
          {/if}
        </span>
      {/if}
    </button>
    <div class="route-controls">
      <button class="icon-btn" on:click={() => dispatch('moveRoute', 'up')} aria-label="Move up" title="Move up">&uarr;</button>
      <button class="icon-btn" on:click={() => dispatch('moveRoute', 'down')} aria-label="Move down" title="Move down">&darr;</button>
      <button class="icon-btn danger" on:click={() => dispatch('remove')} aria-label="Remove route" title="Remove route">&times;</button>
    </div>
  </div>

  {#if route.expanded}
    <div class="route-body">
      <label class="field-label">
        Route Name
        <input
          type="text"
          class="input"
          value={route.name}
          placeholder="e.g. patient_admits"
          on:input={(e) => dispatch('updateName', (e.target as HTMLInputElement).value)}
        />
      </label>

      <div class="section">
        <h4 class="section-title">Filter</h4>
        <FilterEditor filter={route.filter} on:change={(e) => dispatch('updateFilter', e.detail)} />
      </div>

      <div class="section">
        <h4 class="section-title">Transforms</h4>
        <TransformList
          transforms={route.transforms}
          on:add={() => dispatch('addTransform')}
          on:remove={(e) => dispatch('removeTransform', e.detail)}
          on:change={(e) => dispatch('changeTransform', e.detail)}
          on:move={(e) => dispatch('moveTransform', e.detail)}
        />
      </div>

      <div class="section">
        <h4 class="section-title">Actions</h4>
        <ActionList
          actions={route.actions}
          on:add={() => dispatch('addAction')}
          on:remove={(e) => dispatch('removeAction', e.detail)}
          on:change={(e) => dispatch('changeAction', e.detail)}
          on:move={(e) => dispatch('moveAction', e.detail)}
        />
      </div>
    </div>
  {/if}
</div>

<style>
  .route-editor {
    border: 1px solid rgba(255, 255, 255, 0.08);
    border-radius: 10px;
    background: rgba(255, 255, 255, 0.02);
    overflow: hidden;
  }

  .route-editor.expanded {
    border-color: rgba(59, 130, 246, 0.2);
  }

  .route-header-row {
    display: flex;
    align-items: center;
    padding: 0 12px 0 0;
    transition: background 0.15s ease;
  }

  .route-header-row:hover {
    background: rgba(255, 255, 255, 0.03);
  }

  .route-header {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 10px 12px;
    cursor: pointer;
    flex: 1;
    min-width: 0;
    background: transparent;
    border: none;
    color: inherit;
    font: inherit;
    text-align: left;
  }

  .collapse-icon {
    color: rgba(229, 231, 235, 0.5);
    font-size: 0.7rem;
    transition: transform 0.2s ease;
    flex-shrink: 0;
  }

  .collapse-icon.rotated {
    transform: rotate(90deg);
  }

  .route-name {
    font-weight: 700;
    color: rgba(229, 231, 235, 0.92);
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  }

  .summary {
    display: flex;
    align-items: center;
    gap: 6px;
    margin-left: auto;
    margin-right: 12px;
    font-size: 0.8rem;
  }

  .summary-filter {
    color: rgba(147, 197, 253, 0.7);
  }

  .summary-sep {
    color: rgba(229, 231, 235, 0.3);
  }

  .summary-transforms {
    color: rgba(192, 132, 252, 0.7);
  }

  .summary-actions {
    color: rgba(110, 231, 183, 0.7);
  }

  .summary-warn {
    color: rgba(245, 158, 11, 0.8);
    font-size: 0.75rem;
    font-weight: 600;
  }

  .route-controls {
    display: flex;
    gap: 6px;
    flex-shrink: 0;
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

  .icon-btn.danger:hover:not(:disabled) {
    background: rgba(239, 68, 68, 0.12);
    border-color: rgba(239, 68, 68, 0.3);
    color: rgba(254, 202, 202, 0.9);
  }

  .route-body {
    padding: 12px 16px 16px;
    border-top: 1px solid rgba(255, 255, 255, 0.06);
    display: grid;
    gap: 16px;
  }

  .field-label {
    display: grid;
    gap: 4px;
    color: rgba(229, 231, 235, 0.8);
    font-size: 0.85rem;
    font-weight: 600;
  }

  .input {
    padding: 8px 12px;
    border-radius: 10px;
    border: 1px solid rgba(255, 255, 255, 0.12);
    background: rgba(255, 255, 255, 0.03);
    color: rgba(229, 231, 235, 0.92);
    outline: none;
    width: 100%;
    box-sizing: border-box;
  }

  .input:focus {
    border-color: rgba(59, 130, 246, 0.45);
    box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.15);
  }

  .section {
    display: grid;
    gap: 8px;
  }

  .section-title {
    color: rgba(229, 231, 235, 0.7);
    font-size: 0.85rem;
    font-weight: 700;
    margin: 0;
    text-transform: uppercase;
    letter-spacing: 0.05em;
  }
</style>
