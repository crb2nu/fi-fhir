<script lang="ts" context="module">
  export type TabItem = {
    key: string;
    label: string;
    disabled?: boolean;
    count?: number;
    icon?: string;
  };
</script>

<script lang="ts">
  /**
   * Tabs Component
   *
   * Tab navigation with optional badge counts and variant styles.
   * Supports both pill and underline display modes.
   */

  export let tabs: readonly TabItem[];
  export let active: string;
  export let onChange: (key: string) => void;
  export let variant: 'pills' | 'underline' = 'pills';
  export let size: 'sm' | 'md' = 'md';
  export let fullWidth = false;
</script>

<div
  class="tabs {variant}"
  class:full-width={fullWidth}
  class:sm={size === 'sm'}
  role="tablist"
>
  {#each tabs as tab (tab.key)}
    <button
      class="tab"
      class:active={tab.key === active}
      disabled={tab.disabled}
      role="tab"
      aria-selected={tab.key === active}
      tabindex={tab.key === active ? 0 : -1}
      on:click={() => onChange(tab.key)}
    >
      <span class="tab-label">{tab.label}</span>
      {#if tab.count !== undefined}
        <span class="tab-count" class:active={tab.key === active}>
          {tab.count}
        </span>
      {/if}
    </button>
  {/each}
</div>

<style>
  .tabs {
    display: flex;
    gap: var(--space-2);
    flex-wrap: wrap;
  }

  .tabs.full-width {
    width: 100%;
  }

  .tabs.full-width .tab {
    flex: 1;
    justify-content: center;
  }

  /* Variant: Pills (default) */
  .tabs.pills .tab {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    padding: var(--space-2) var(--space-3);
    border-radius: var(--radius-lg);
    border: 1px solid var(--color-border-default);
    background: var(--color-bg-elevated);
    color: var(--color-text-secondary);
    cursor: pointer;
    font-size: var(--text-sm);
    font-weight: var(--font-semibold);
    transition: var(--transition-all);
  }

  .tabs.pills.sm .tab {
    padding: var(--space-1) var(--space-2);
    font-size: var(--text-xs);
  }

  .tabs.pills .tab:hover:not(:disabled):not(.active) {
    background: var(--color-bg-hover);
    border-color: var(--color-border-strong);
  }

  .tabs.pills .tab.active {
    background: var(--color-primary-muted);
    border-color: var(--color-primary-border);
    color: var(--color-text-primary);
  }

  .tabs.pills .tab:disabled {
    opacity: 0.45;
    cursor: not-allowed;
  }

  .tabs.pills .tab:focus-visible {
    outline: none;
    box-shadow: var(--shadow-focus);
    border-color: var(--color-border-focus);
  }

  /* Variant: Underline */
  .tabs.underline {
    gap: 0;
    border-bottom: 1px solid var(--color-border-default);
  }

  .tabs.underline .tab {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    padding: var(--space-3) var(--space-4);
    background: transparent;
    border: none;
    border-bottom: 2px solid transparent;
    margin-bottom: -1px;
    color: var(--color-text-tertiary);
    cursor: pointer;
    font-size: var(--text-sm);
    font-weight: var(--font-medium);
    transition: var(--transition-all);
  }

  .tabs.underline.sm .tab {
    padding: var(--space-2) var(--space-3);
    font-size: var(--text-xs);
  }

  .tabs.underline .tab:hover:not(:disabled):not(.active) {
    color: var(--color-text-secondary);
    border-bottom-color: var(--color-border-strong);
  }

  .tabs.underline .tab.active {
    color: var(--color-primary);
    border-bottom-color: var(--color-primary);
  }

  .tabs.underline .tab:disabled {
    opacity: 0.45;
    cursor: not-allowed;
  }

  .tabs.underline .tab:focus-visible {
    outline: none;
    background: var(--color-bg-hover);
    border-radius: var(--radius-md) var(--radius-md) 0 0;
  }

  /* Tab label */
  .tab-label {
    white-space: nowrap;
  }

  /* Tab count badge */
  .tab-count {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    min-width: 20px;
    height: 18px;
    padding: 0 var(--space-1);
    background: var(--color-bg-surface);
    border-radius: var(--radius-full);
    font-size: var(--text-2xs);
    font-weight: var(--font-bold);
    font-variant-numeric: tabular-nums;
    color: var(--color-text-muted);
    transition: var(--transition-colors);
  }

  .tabs.sm .tab-count {
    min-width: 16px;
    height: 14px;
    font-size: 9px;
  }

  .tab-count.active {
    background: var(--color-primary-muted);
    color: var(--color-primary);
  }
</style>
