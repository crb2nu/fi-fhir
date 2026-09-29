<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import ChevronDown from '@lucide/svelte/icons/chevron-down';
  import ChevronUp from '@lucide/svelte/icons/chevron-up';
  import { IconButton, Tabs, type TabItem } from '$lib/ui/primitives';
  import type { PanelTab } from './types';
  import { workflowProblemCounts } from './panels/workflowProblemsStore';
  import { isAvailable } from '$lib/features/copilot';
  import { CopilotPanel } from '$lib/features/copilot';

  /**
   * Collapsible bottom panel: a 28 px `Tabs` strip (Output, Problems, Debug,
   * Trace, Copilot) over a 12 px-padded body. Collapsed, only the strip shows;
   * selecting a tab — including the selected one — opens it. The Problems tab
   * carries the honest `problems-badge` (a toned `TabItem.badge` with the
   * "N problems" label), present only while a live draft has problems.
   */

  export let open: boolean = false;
  export let height: number = 200;
  export let activeTab: PanelTab = 'output';

  const dispatch = createEventDispatcher<{
    tabchange: PanelTab;
    toggle: void;
    navigate: { panel: string };
  }>();

  const panelTabs: { key: PanelTab; label: string }[] = [
    { key: 'output', label: 'Output' },
    { key: 'problems', label: 'Problems' },
    { key: 'debug', label: 'Debug' },
    { key: 'trace', label: 'Trace' },
    { key: 'copilot', label: 'Copilot' },
  ];

  function onSelect(key: string): void {
    dispatch('tabchange', key as PanelTab);
  }

  function onToggle(): void {
    dispatch('toggle');
  }

  $: badgeTone = ($workflowProblemCounts.error > 0
    ? 'danger'
    : $workflowProblemCounts.warning > 0
      ? 'warning'
      : 'info') as 'danger' | 'warning' | 'info';

  $: tabItems = panelTabs.map((tab): TabItem => ({
    id: tab.key,
    label: tab.label,
    class: tab.key === 'copilot' && !$isAvailable ? 'dimmed' : undefined,
    badge:
      tab.key === 'problems' && $workflowProblemCounts.total > 0
        ? {
            value: $workflowProblemCounts.total,
            tone: badgeTone,
            class: badgeTone,
            label: `${$workflowProblemCounts.total} problems`,
            testid: 'problems-badge',
          }
        : undefined,
  }));
</script>

<div class="bottom-panel" class:open style="--panel-h: {height}px">
  <div class="panel-header">
    <Tabs
      class="panel-tabs"
      label="Panel tabs"
      activation="manual"
      items={tabItems}
      value={activeTab}
      onselect={onSelect}
    />

    <IconButton
      icon={open ? ChevronDown : ChevronUp}
      label={open ? 'Hide panel' : 'Show panel'}
      class="panel-toggle"
      onclick={onToggle}
    />
  </div>

  {#if open}
    {#if activeTab === 'copilot'}
      <div class="panel-content panel-content-copilot">
        <CopilotPanel />
      </div>
    {:else}
      <div class="panel-content">
        <slot />
      </div>
    {/if}
  {/if}
</div>

<style>
  .bottom-panel {
    display: flex;
    flex-direction: column;
    min-height: 0;
    height: auto;
    background: var(--ide-bottom-panel-bg, var(--color-bg-elevated));
    border-top: 1px solid var(--color-border-subtle);
  }

  .bottom-panel.open {
    height: var(--panel-h, 200px);
    min-height: var(--ide-bottom-panel-min-height, 100px);
  }

  .panel-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-2);
    flex: 0 0 auto;
    height: 28px;
    padding: 0 var(--space-1) 0 var(--space-3);
  }

  .bottom-panel.open .panel-header {
    border-bottom: 1px solid var(--color-border-subtle);
  }

  .panel-header :global(.panel-tabs) {
    align-self: stretch;
    min-height: 0;
  }

  .panel-header :global(.panel-tabs .ui-tab) {
    font-size: var(--text-xs);
  }

  /* Collapsed, the strip names the tabs but marks none as showing. */
  .bottom-panel:not(.open) .panel-header :global(.panel-tabs .ui-tab.is-active) {
    color: var(--color-text-tertiary);
    border-bottom-color: transparent;
  }

  .panel-header :global(.panel-tabs .ui-tab.dimmed:not(.is-active)) {
    color: var(--color-text-muted);
  }

  .panel-header :global(.panel-toggle) {
    width: 24px;
    height: 24px;
  }

  .panel-content {
    flex: 1;
    min-height: 0;
    overflow: auto;
    padding: var(--space-3);
  }

  .panel-content-copilot {
    padding: 0;
    overflow: hidden;
  }
</style>
