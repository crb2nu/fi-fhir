<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import type { PanelTab } from './types';
  import { workflowProblemCounts } from './panels/workflowProblemsStore';
  import { isAvailable } from '$lib/features/copilot';
  import { CopilotPanel } from '$lib/features/copilot';

  /**
   * Collapsible bottom panel with tabbed content areas.
   * Hosts Output, Problems, Debug, Trace, and Copilot views.
   */

  export let open: boolean = false;
  export let height: number = 200;
  export let activeTab: PanelTab = 'output';

  const dispatch = createEventDispatcher<{
    tabchange: PanelTab;
    toggle: void;
    navigate: { panel: string };
  }>();

  type PanelTabEntry = { key: PanelTab; label: string; indicator?: string };

  const panelTabs: PanelTabEntry[] = [
    { key: 'output', label: 'Output' },
    { key: 'problems', label: 'Problems' },
    { key: 'debug', label: 'Debug' },
    { key: 'trace', label: 'Trace' },
    { key: 'copilot', label: 'Copilot', indicator: '\u2726' },
  ];

  function onTabClick(key: PanelTab): void {
    dispatch('tabchange', key);
  }

  function onToggle(): void {
    dispatch('toggle');
  }

  // Track previous count to trigger the pulse animation on increase.
  let prevTotal = 0;
  let pulsing = false;
  let pulseTimer: ReturnType<typeof setTimeout> | undefined;

  /* eslint-disable svelte/infinite-reactive-loop -- one-shot pulse, no feedback loop */
  $: {
    const total = $workflowProblemCounts.total;
    if (total > prevTotal && prevTotal >= 0) {
      pulsing = true;
      clearTimeout(pulseTimer);
      pulseTimer = setTimeout(() => {
        pulsing = false;
      }, 400);
    }
    prevTotal = total;
  }
  /* eslint-enable svelte/infinite-reactive-loop */

  $: badgeVariant = $workflowProblemCounts.error > 0
    ? 'danger'
    : $workflowProblemCounts.warning > 0
      ? 'warning'
      : 'info';
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
          class:dimmed={tab.key === 'copilot' && !$isAvailable}
          role="tab"
          aria-selected={tab.key === activeTab}
          on:click={() => onTabClick(tab.key)}
        >
          {tab.label}
          {#if tab.key === 'problems' && $workflowProblemCounts.total > 0}
            <span
              class="diag-badge {badgeVariant}"
              class:pulse={pulsing}
              aria-label="{$workflowProblemCounts.total} problems"
            >
              {$workflowProblemCounts.total}
            </span>
          {/if}
          {#if tab.indicator}
            <span class="tab-indicator">{tab.indicator}</span>
          {/if}
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
          <!-- Open: chevron points down — click to collapse the panel downward. -->
          <path d="M6 9l6 6 6-6" />
        {:else}
          <!-- Closed: chevron points up — click to expand the panel upward. -->
          <path d="M6 15l6-6 6 6" />
        {/if}
      </svg>
    </button>
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
    gap: 6px;
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

  .panel-tab.dimmed {
    opacity: 0.5;
  }

  .tab-indicator {
    margin-left: 2px;
    font-size: 9px;
    color: var(--color-primary);
    line-height: 1;
  }

  .panel-tab.active .tab-indicator {
    color: var(--color-primary);
  }

  .panel-tab:focus-visible {
    outline: none;
    box-shadow: inset var(--shadow-focus);
  }

  /* ── Diagnostic count badge ──────────────────────────────────────── */
  .diag-badge {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    min-width: 16px;
    height: 16px;
    padding: 0 4px;
    border-radius: var(--radius-full);
    font-size: 9px;
    font-weight: var(--font-bold);
    line-height: 1;
  }

  .diag-badge.danger {
    background: var(--color-danger-bg);
    color: var(--color-danger-text);
    border: 1px solid var(--color-danger-border);
  }

  .diag-badge.warning {
    background: var(--color-warning-bg);
    color: var(--color-warning-text);
    border: 1px solid var(--color-warning-border);
  }

  .diag-badge.info {
    background: var(--color-info-bg);
    color: var(--color-info-text);
    border: 1px solid var(--color-info-border);
  }

  .diag-badge.pulse {
    animation: badgeBounce 400ms var(--ease-bounce);
  }

  @keyframes badgeBounce {
    0%, 100% {
      transform: translateY(0) scale(1);
    }
    40% {
      transform: translateY(-3px) scale(1.15);
    }
    60% {
      transform: translateY(-1px) scale(1.05);
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .diag-badge.pulse {
      animation: none;
    }
  }

  /* ── Existing styles ─────────────────────────────────────────────── */
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

  .panel-content-copilot {
    padding: 0;
    overflow: hidden;
  }
</style>
