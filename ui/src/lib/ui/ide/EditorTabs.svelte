<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import type { EditorTab } from './types';

  /**
   * Multi-tab editor bar with close buttons and dirty indicators.
   */

  export let tabs: EditorTab[] = [];
  export let activeTabId: string | null = null;

  const dispatch = createEventDispatcher<{ select: string; close: string }>();

  function onSelect(id: string): void {
    dispatch('select', id);
  }
</script>

<div class="editor-tabs" role="tablist" aria-label="Open editors">
  {#each tabs as tab (tab.id)}
    <div
      class="tab"
      class:active={tab.id === activeTabId}
      role="tab"
      aria-selected={tab.id === activeTabId}
      aria-label={tab.title}
      tabindex={tab.id === activeTabId ? 0 : -1}
      on:click={() => onSelect(tab.id)}
      on:keydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); onSelect(tab.id); } }}
    >
      <span class="tab-title">{tab.title}</span>
      {#if tab.dirty}
        <span class="dirty-indicator" aria-label="Unsaved changes"></span>
      {/if}
      <button
        type="button"
        class="tab-close"
        aria-label="Close {tab.title}"
        tabindex="-1"
        on:click|stopPropagation={() => dispatch('close', tab.id)}
      >
        <svg
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
          aria-hidden="true"
        >
          <path d="M18 6L6 18" />
          <path d="M6 6l12 12" />
        </svg>
      </button>
    </div>
  {/each}
</div>

<style>
  .editor-tabs {
    display: flex;
    overflow-x: auto;
    -webkit-overflow-scrolling: touch;
    height: var(--ide-tab-height, 36px);
    min-height: var(--ide-tab-height, 36px);
    background: var(--ide-tab-bg, var(--color-bg-surface));
    border-bottom: 1px solid var(--ide-tab-border, var(--color-border-subtle));
    scrollbar-width: none;
  }

  .editor-tabs::-webkit-scrollbar {
    display: none;
  }

  .tab {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    padding: 0 var(--space-3);
    height: 100%;
    border: none;
    border-right: 1px solid var(--ide-tab-border, var(--color-border-subtle));
    background: var(--ide-tab-bg, var(--color-bg-surface));
    color: var(--color-text-secondary);
    font-size: var(--text-xs);
    font-weight: var(--font-medium);
    cursor: pointer;
    white-space: nowrap;
    transition: var(--transition-colors);
    flex: 0 0 auto;
    max-width: 200px;
  }

  .tab:hover {
    background: var(--color-bg-hover);
    color: var(--color-text-primary);
  }

  .tab.active {
    background: var(--ide-tab-active-bg, var(--color-bg-base));
    color: var(--color-text-primary);
    border-bottom: 2px solid var(--color-primary);
  }

  .tab:focus-visible {
    outline: none;
    box-shadow: inset var(--shadow-focus);
  }

  .tab-title {
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .dirty-indicator {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: var(--color-warning);
    flex: 0 0 auto;
  }

  .tab-close {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 16px;
    height: 16px;
    padding: 0;
    border: none;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--color-text-muted);
    cursor: pointer;
    flex: 0 0 auto;
    opacity: 0;
    transition: var(--transition-all);
  }

  .tab:hover .tab-close,
  .tab.active .tab-close {
    opacity: 1;
  }

  .tab-close:hover {
    background: var(--color-bg-active);
    color: var(--color-text-primary);
  }

  .tab-close svg {
    width: 12px;
    height: 12px;
  }
</style>
