<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import { Tabs, type TabItem } from '$lib/ui/primitives';
  import type { WorkspaceDocument } from './types';
  import { VIEW_ICONS } from './viewIcons';

  /**
   * Editor tab strip, 32 px, on the `Tabs` primitive: the view's icon, the
   * title, an unsaved dot (a document marked dirty through
   * `ideStore.markDirty`), and a close button that shows on hover, focus and
   * the active tab. The active tab is marked by an accent underline, not a
   * fill. Keyboard: one tab stop; ArrowLeft/ArrowRight/Home/End move focus,
   * Enter or Space opens, Delete closes.
   */

  export let tabs: WorkspaceDocument[] = [];
  export let activeTabId: string | null = null;

  const dispatch = createEventDispatcher<{
    select: string;
    close: string;
  }>();

  $: items = tabs.map(
    (tab): TabItem => ({
      id: tab.id,
      label: tab.title,
      icon: tab.view ? VIEW_ICONS[tab.view] : undefined,
      dirty: tab.dirty,
      title: tab.subtitle ? `${tab.title} — ${tab.subtitle}` : tab.title,
    })
  );
</script>

<div class="editor-tabs">
  {#if items.length > 0}
    <Tabs
      class="editor-tab-list"
      label="Open editors"
      activation="manual"
      {items}
      value={activeTabId ?? undefined}
      onselect={(id) => dispatch('select', id)}
      onclose={(id) => dispatch('close', id)}
    />
  {/if}
</div>

<style>
  .editor-tabs {
    display: flex;
    align-items: stretch;
    height: 32px;
    min-height: 32px;
    background: var(--ide-tab-bg, var(--color-bg-elevated));
    border-bottom: 1px solid var(--ide-tab-border, var(--color-border-subtle));
  }

  .editor-tabs :global(.editor-tab-list) {
    gap: 0;
    min-height: 0;
  }

  .editor-tabs :global(.editor-tab-list::-webkit-scrollbar) {
    display: none;
  }

  .editor-tabs :global(.editor-tab-list .ui-tab-item) {
    position: relative;
    max-width: 240px;
    padding: 0 4px 0 0;
    border-right: 1px solid var(--ide-tab-border, var(--color-border-subtle));
    transition: var(--transition-colors);
  }

  .editor-tabs :global(.editor-tab-list .ui-tab-item:hover) {
    background: var(--color-bg-hover);
  }

  .editor-tabs :global(.editor-tab-list .ui-tab-item.is-active) {
    background: var(--ide-tab-active-bg, var(--color-bg-base));
  }

  .editor-tabs :global(.editor-tab-list .ui-tab-item.is-active::after) {
    content: '';
    position: absolute;
    left: 0;
    right: 0;
    bottom: -1px;
    height: 2px;
    background: var(--color-primary);
  }

  .editor-tabs :global(.editor-tab-list .ui-tab) {
    min-width: 0;
    padding: 0 2px 0 10px;
    border: none;
    font-size: var(--text-ui);
    font-weight: var(--font-normal);
  }

  .editor-tabs :global(.editor-tab-list .ui-tab.is-active) {
    border-bottom-color: transparent;
  }

  .editor-tabs :global(.editor-tab-list .ui-tab-icon) {
    width: 14px;
    height: 14px;
    color: var(--color-text-tertiary);
  }

  .editor-tabs :global(.editor-tab-list .ui-tab-label) {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
  }
</style>
