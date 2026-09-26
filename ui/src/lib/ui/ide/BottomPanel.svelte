<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import ChevronDown from '@lucide/svelte/icons/chevron-down';
  import ChevronUp from '@lucide/svelte/icons/chevron-up';
  import { Badge, IconButton } from '$lib/ui/primitives';
  import type { PanelTab } from './types';
  import { workflowProblemCounts } from './panels/workflowProblemsStore';
  import { isAvailable } from '$lib/features/copilot';
  import { CopilotPanel } from '$lib/features/copilot';

  /**
   * Collapsible bottom panel: a 28 px underline tab strip (Output, Problems,
   * Debug, Trace, Copilot) over a 12 px-padded body. Collapsed, only the strip
   * shows; selecting a tab opens it.
   *
   * The strip follows the `Tabs` primitive's look and keyboard model (one tab
   * stop, arrows/Home/End, activation on Enter/Space/click) but is rendered
   * here because the Problems tab carries the honest `problems-badge`
   * (a mono `Badge` with its own test id and "N problems" label), which the
   * primitive's plain `count` cannot express.
   */

  export let open: boolean = false;
  export let height: number = 200;
  export let activeTab: PanelTab = 'output';

  const dispatch = createEventDispatcher<{
    tabchange: PanelTab;
    toggle: void;
    navigate: { panel: string };
  }>();

  type PanelTabEntry = { key: PanelTab; label: string };

  const panelTabs: PanelTabEntry[] = [
    { key: 'output', label: 'Output' },
    { key: 'problems', label: 'Problems' },
    { key: 'debug', label: 'Debug' },
    { key: 'trace', label: 'Trace' },
    { key: 'copilot', label: 'Copilot' },
  ];

  function onTabClick(key: PanelTab): void {
    dispatch('tabchange', key);
  }

  function onToggle(): void {
    dispatch('toggle');
  }

  function onTabKeydown(event: KeyboardEvent, index: number): void {
    let next: number;
    switch (event.key) {
      case 'ArrowRight':
        next = (index + 1) % panelTabs.length;
        break;
      case 'ArrowLeft':
        next = (index - 1 + panelTabs.length) % panelTabs.length;
        break;
      case 'Home':
        next = 0;
        break;
      case 'End':
        next = panelTabs.length - 1;
        break;
      default:
        return;
    }
    event.preventDefault();
    const list = (event.currentTarget as HTMLElement).closest('[role="tablist"]');
    list?.querySelectorAll<HTMLButtonElement>('[role="tab"]')[next]?.focus();
  }

  $: badgeTone = ($workflowProblemCounts.error > 0
    ? 'danger'
    : $workflowProblemCounts.warning > 0
      ? 'warning'
      : 'info') as 'danger' | 'warning' | 'info';
</script>

<div class="bottom-panel" class:open style="--panel-h: {height}px">
  <div class="panel-header">
    <div class="panel-tabs" role="tablist" aria-label="Panel tabs">
      {#each panelTabs as tab, index (tab.key)}
        <button
          type="button"
          class="panel-tab"
          class:active={open && tab.key === activeTab}
          class:dimmed={tab.key === 'copilot' && !$isAvailable}
          role="tab"
          aria-selected={tab.key === activeTab}
          tabindex={tab.key === activeTab ? 0 : -1}
          on:click={() => onTabClick(tab.key)}
          on:keydown={(event) => onTabKeydown(event, index)}
        >
          <span>{tab.label}</span>
          {#if tab.key === 'problems' && $workflowProblemCounts.total > 0}
            <Badge
              mono
              tone={badgeTone}
              class={badgeTone}
              data-testid="problems-badge"
              aria-label="{$workflowProblemCounts.total} problems"
            >
              {$workflowProblemCounts.total}
            </Badge>
          {/if}
        </button>
      {/each}
    </div>

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

  .panel-tabs {
    display: flex;
    align-items: stretch;
    gap: var(--space-4);
    height: 100%;
    min-width: 0;
    overflow-x: auto;
    scrollbar-width: none;
  }

  .panel-tab {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 0 2px;
    border: none;
    border-top: 2px solid transparent;
    border-bottom: 2px solid transparent;
    background: none;
    color: var(--color-text-tertiary);
    font: inherit;
    font-size: var(--text-xs);
    font-weight: var(--font-medium);
    white-space: nowrap;
    cursor: pointer;
    transition: var(--transition-colors);
  }

  .panel-tab:hover {
    color: var(--color-text-primary);
  }

  .panel-tab.active {
    color: var(--color-text-primary);
    border-bottom-color: var(--color-primary);
  }

  .panel-tab.dimmed:not(.active) {
    color: var(--color-text-muted);
  }

  .panel-tab:focus-visible {
    outline: 2px solid var(--color-focus-ring);
    outline-offset: -2px;
    border-radius: var(--radius-sm);
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
