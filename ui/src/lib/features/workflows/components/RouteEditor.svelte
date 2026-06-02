<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import Badge from '$lib/ui/Badge.svelte';
  import Tooltip from '$lib/ui/Tooltip.svelte';
  import FilterEditor from './FilterEditor.svelte';
  import TransformList from './TransformList.svelte';
  import ActionList from './ActionList.svelte';
  import type { RouteDraft, FilterDraft, ActionDraft, TransformDraft } from '../workflowTypes';
  import type { DryRunRouteResult } from '$lib/gen/graphql';

  export let route: RouteDraft;
  export let dryRunResult: DryRunRouteResult | null = null;

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
    <button
      type="button"
      class="route-header"
      on:click={() => dispatch('toggleExpand')}
      aria-expanded={route.expanded}
      aria-controls={`route-body-${route._key}`}
    >
      <span class="collapse-icon" class:rotated={route.expanded}>&#9654;</span>
      <span class="route-name">{route.name || 'Unnamed route'}</span>

      {#if dryRunResult}
        <div class="dry-run-badge">
          <Tooltip content={dryRunResult.matched ? 'Successfully matched this route' : (dryRunResult.skipReason || 'Route skipped')}>
            {#if dryRunResult.matched}
              <Badge variant="success" size="sm">MATCHED</Badge>
            {:else}
              <Badge variant="default" size="sm">SKIPPED</Badge>
            {/if}
          </Tooltip>
        </div>
      {/if}

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
      <button
        type="button"
        class="icon-btn"
        on:click={() => dispatch('moveRoute', 'up')}
        aria-label="Move route up"
        title="Move route up"
      >&uarr;</button>
      <button
        type="button"
        class="icon-btn"
        on:click={() => dispatch('moveRoute', 'down')}
        aria-label="Move route down"
        title="Move route down"
      >&darr;</button>
      <button
        type="button"
        class="icon-btn danger"
        on:click={() => dispatch('remove')}
        aria-label="Remove route"
        title="Remove route"
      >&times;</button>
    </div>
  </div>

  {#if route.expanded}
    <div class="route-body" id={`route-body-${route._key}`}>
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
    border: 1px solid var(--color-border-default);
    border-radius: 10px;
    background: var(--color-bg-elevated);
    overflow: hidden;
  }

  .route-editor.expanded {
    border-color: var(--color-primary-border);
  }

  .route-header-row {
    display: flex;
    align-items: center;
    padding: 0 12px 0 0;
    transition: background 0.15s ease;
  }

  .route-header-row:hover {
    background: var(--color-bg-hover);
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
    color: var(--color-text-muted);
    font-size: 0.7rem;
    transition: transform 0.2s ease;
    flex-shrink: 0;
  }

  .collapse-icon.rotated {
    transform: rotate(90deg);
  }

  .route-name {
    font-weight: 700;
    color: var(--color-text-primary);
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  }

  .dry-run-badge {
    margin-left: 8px;
    animation: fadeIn 0.2s ease-out;
  }

  @keyframes fadeIn {
    from {
      opacity: 0;
      transform: scale(0.9);
    }
    to {
      opacity: 1;
      transform: scale(1);
    }
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
    color: var(--color-primary);
  }

  .summary-sep {
    color: var(--color-text-muted);
  }

  .summary-transforms {
    color: var(--color-text-secondary);
  }

  .summary-actions {
    color: var(--color-success);
  }

  .summary-warn {
    color: var(--color-warning);
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

  .icon-btn.danger:hover:not(:disabled) {
    background: rgba(239, 68, 68, 0.12);
    border-color: rgba(239, 68, 68, 0.3);
    color: rgba(254, 202, 202, 0.9);
  }

  .route-body {
    padding: 12px 16px 16px;
    border-top: 1px solid var(--color-border-subtle);
    display: grid;
    gap: 16px;
  }

  .field-label {
    display: grid;
    gap: 4px;
    color: var(--color-text-tertiary);
    font-size: 0.85rem;
    font-weight: 600;
  }

  .input {
    padding: 8px 12px;
    border-radius: 10px;
    border: 1px solid var(--color-border-default);
    background: var(--color-bg-input);
    color: var(--color-text-primary);
    outline: none;
    width: 100%;
    box-sizing: border-box;
    transition: var(--transition-all);
  }

  .input::placeholder {
    color: var(--color-text-muted);
  }

  .input:hover:not(:disabled):not(:focus) {
    border-color: var(--color-border-strong);
  }

  .input:focus {
    border-color: var(--color-border-focus);
    box-shadow: var(--shadow-focus);
  }

  .section {
    display: grid;
    gap: 8px;
  }

  .section-title {
    color: var(--color-text-tertiary);
    font-size: 0.85rem;
    font-weight: 700;
    margin: 0;
    text-transform: uppercase;
    letter-spacing: 0.05em;
  }
</style>
