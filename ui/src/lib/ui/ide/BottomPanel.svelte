<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import type { PanelTab } from './types';

  /**
   * Collapsible bottom panel with tabbed content areas.
   * Hosts Output, Problems, and Trace views.
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
  ];

  function onTabClick(key: PanelTab): void {
    dispatch('tabchange', key);
  }

  function onToggle(): void {
    dispatch('toggle');
  }
</script>

<div
  class="bottom-panel"
  class:open
  style="--panel-h: {height}px"
>
  <div class="panel-header">
    <div class="panel-tabs" role="tablist" aria-label="Panel tabs">
      {#each panelTabs as tab (tab.key)}
        <button
          type="button"
          class="panel-tab"
          class:active={tab.key === activeTab}
          role="tab"
          aria-selected={tab.key === activeTab}
          on:click={() => onTabClick(tab.key)}
        >
          {tab.label}
        </button>
      {/each}
    </div>

    <button
      type="button"
      class="panel-toggle"
      aria-label={open ? 'Hide panel' : 'Show panel'}
      on:click={onToggle}
    >
      <svg
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        aria-hidden="true"
      >
        {#if open}
          <path d="M6 15l6-6 6 6" />
        {:else}
          <path d="M6 9l6 6 6-6" />
        {/if}
      </svg>
    </button>
  </div>

  {#if open}
    <div class="panel-content">
      <slot />
    </div>
  {/if}
</div>

<style>
  .bottom-panel {
    background: var(--ide-bottom-panel-bg, var(--color-bg-surface));
    border-top: 1px solid var(--color-border-subtle);
    display: flex;
    flex-direction: column;
    min-height: 0;
    height: auto;
  }

  .bottom-panel.open {
    height: var(--panel-h, 200px);
    min-height: var(--ide-bottom-panel-min-height, 100px);
  }

  .panel-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    height: 32px;
    min-height: 32px;
    padding: 0 var(--space-3);
    border-bottom: 1px solid var(--color-border-subtle);
  }

  .panel-tabs {
    display: flex;
    gap: 0;
    height: 100%;
  }

  .panel-tab {
    display: flex;
    align-items: center;
    padding: 0 var(--space-3);
    height: 100%;
    border: none;
    border-bottom: 2px solid transparent;
    background: transparent;
    color: var(--color-text-tertiary);
    font-size: var(--text-xs);
    font-weight: var(--font-medium);
    cursor: pointer;
    transition: var(--transition-colors);
  }

  .panel-tab:hover {
    color: var(--color-text-secondary);
  }

  .panel-tab.active {
    color: var(--color-text-primary);
    border-bottom-color: var(--color-primary);
  }

  .panel-tab:focus-visible {
    outline: none;
    box-shadow: inset var(--shadow-focus);
  }

  .panel-toggle {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 24px;
    height: 24px;
    padding: 0;
    border: none;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--color-text-tertiary);
    cursor: pointer;
    transition: var(--transition-all);
  }

  .panel-toggle:hover {
    background: var(--color-bg-hover);
    color: var(--color-text-primary);
  }

  .panel-toggle:focus-visible {
    outline: none;
    box-shadow: var(--shadow-focus);
  }

  .panel-toggle svg {
    width: 16px;
    height: 16px;
  }

  .panel-content {
    flex: 1;
    overflow: auto;
    padding: var(--space-2) var(--space-3);
    min-height: 0;
  }
</style>
