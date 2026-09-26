<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import ArrowDown from '@lucide/svelte/icons/arrow-down';
  import ArrowRight from '@lucide/svelte/icons/arrow-right';
  import ArrowUp from '@lucide/svelte/icons/arrow-up';
  import ChevronRight from '@lucide/svelte/icons/chevron-right';
  import X from '@lucide/svelte/icons/x';
  import { Badge, Field, Icon, IconButton, Input } from '$lib/ui/primitives';
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
      return route.filter.eventTypes.join(', ');
    }
    return `${route.filter.eventTypes.length} event types`;
  }
</script>

<div class="route" class:is-expanded={route.expanded}>
  <div class="route-head">
    <button
      type="button"
      class="route-toggle"
      on:click={() => dispatch('toggleExpand')}
      aria-expanded={route.expanded}
      aria-controls={`route-body-${route._key}`}
    >
      <Icon icon={ChevronRight} class="route-chevron" />
      <span class="route-name" class:is-unnamed={!route.name}>{route.name || 'Unnamed route'}</span>

      {#if dryRunResult}
        <span
          class="route-result"
          title={dryRunResult.matched
            ? 'Matched in the last dry run'
            : dryRunResult.skipReason || 'Route skipped'}
        >
          <Badge tone={dryRunResult.matched ? 'success' : 'neutral'} dot={dryRunResult.matched}>
            {dryRunResult.matched ? 'Matched' : 'Skipped'}
          </Badge>
        </span>
      {/if}

      {#if !route.expanded}
        <span class="summary">
          <span class="summary-part text-mono">{filterSummary(route)}</span>
          {#if route.transforms.length > 0}
            <Icon icon={ArrowRight} size={12} />
            <span class="summary-part"
              >{route.transforms.length} transform{route.transforms.length > 1 ? 's' : ''}</span
            >
          {/if}
          <Icon icon={ArrowRight} size={12} />
          <span class="summary-part text-mono">{actionSummary(route)}</span>
          {#if route.actions.length === 0}
            <Badge tone="warning">Needs actions</Badge>
          {/if}
        </span>
      {/if}
    </button>
    <div class="route-controls">
      <IconButton icon={ArrowUp} label="Move route up" onclick={() => dispatch('moveRoute', 'up')} />
      <IconButton
        icon={ArrowDown}
        label="Move route down"
        onclick={() => dispatch('moveRoute', 'down')}
      />
      <IconButton icon={X} label="Remove route" onclick={() => dispatch('remove')} />
    </div>
  </div>

  {#if route.expanded}
    <div class="route-body" id={`route-body-${route._key}`}>
      <div class="name-row">
        <Field label="Route name">
          <Input
            mono
            value={route.name}
            placeholder="e.g. patient_admits"
            oninput={(e) => dispatch('updateName', e.currentTarget.value)}
          />
        </Field>
      </div>

      <section class="section">
        <h4 class="section-title">Filter</h4>
        <FilterEditor filter={route.filter} on:change={(e) => dispatch('updateFilter', e.detail)} />
      </section>

      <section class="section">
        <h4 class="section-title">Transforms</h4>
        <TransformList
          transforms={route.transforms}
          on:add={() => dispatch('addTransform')}
          on:remove={(e) => dispatch('removeTransform', e.detail)}
          on:change={(e) => dispatch('changeTransform', e.detail)}
          on:move={(e) => dispatch('moveTransform', e.detail)}
        />
      </section>

      <section class="section">
        <h4 class="section-title">Actions</h4>
        <ActionList
          actions={route.actions}
          on:add={() => dispatch('addAction')}
          on:remove={(e) => dispatch('removeAction', e.detail)}
          on:change={(e) => dispatch('changeAction', e.detail)}
          on:move={(e) => dispatch('moveAction', e.detail)}
        />
      </section>
    </div>
  {/if}
</div>

<style>
  .route {
    border-bottom: 1px solid var(--color-border-subtle);
  }

  .route:last-child {
    border-bottom: 0;
  }

  .route-head {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    height: 36px;
    padding-right: var(--space-2);
  }

  .route-head:hover {
    background: var(--color-bg-hover);
  }

  .route-toggle {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    flex: 1 1 auto;
    min-width: 0;
    height: 100%;
    padding: 0 var(--space-2) 0 var(--space-3);
    background: transparent;
    border: 0;
    color: inherit;
    font: inherit;
    text-align: left;
    cursor: pointer;
  }

  .route-toggle:focus-visible {
    outline: 2px solid var(--color-focus-ring);
    outline-offset: -2px;
  }

  .route-toggle :global(.route-chevron) {
    color: var(--color-text-tertiary);
    transition: transform var(--duration-fast) var(--ease-out);
  }

  .route.is-expanded .route-toggle :global(.route-chevron) {
    transform: rotate(90deg);
  }

  .route-name {
    flex: 0 1 auto;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-family: var(--font-mono);
    font-size: var(--text-mono);
    font-weight: var(--font-semibold);
    color: var(--color-text-primary);
  }

  .route-name.is-unnamed {
    font-family: var(--font-ui);
    font-weight: var(--font-normal);
    color: var(--color-text-tertiary);
  }

  .route-result {
    display: inline-flex;
  }

  .summary {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    min-width: 0;
    margin-left: auto;
    overflow: hidden;
    font-size: var(--text-xs);
    color: var(--color-text-tertiary);
    white-space: nowrap;
  }

  .summary-part {
    overflow: hidden;
    text-overflow: ellipsis;
    color: var(--color-text-secondary);
  }

  .route-controls {
    display: flex;
    gap: 2px;
    flex: 0 0 auto;
  }

  .route-body {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
    padding: var(--space-3) var(--space-3) var(--space-4) var(--space-8);
    border-top: 1px solid var(--color-border-subtle);
  }

  .name-row {
    max-width: 360px;
  }

  .section {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  .section-title {
    margin: 0;
    font-size: var(--text-label);
    font-weight: var(--font-semibold);
    letter-spacing: var(--tracking-label);
    text-transform: uppercase;
    color: var(--color-text-tertiary);
  }
</style>
