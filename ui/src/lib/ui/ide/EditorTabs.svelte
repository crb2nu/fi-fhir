<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import Plus from '@lucide/svelte/icons/plus';
  import X from '@lucide/svelte/icons/x';
  import { Icon, IconButton } from '$lib/ui/primitives';
  import type { WorkspaceDocument, DocumentType } from './types';
  import { DOCUMENT_ICONS, DOCUMENT_TYPE_LABELS, VIEW_ICONS } from './viewIcons';

  /**
   * Editor tab strip, 32 px: an icon per document, the title, an unsaved dot,
   * and a close button that shows on hover, focus and the active tab. The
   * active tab is marked by an accent underline, not a fill. Keyboard: one
   * tab stop; ArrowLeft/ArrowRight/Home/End move focus, Enter or Space opens,
   * Delete closes.
   */

  export let tabs: WorkspaceDocument[] = [];
  export let activeTabId: string | null = null;

  const dispatch = createEventDispatcher<{
    select: string;
    close: string;
    add: DocumentType;
  }>();

  let addMenuOpen = false;

  type AddOption = { type: Exclude<DocumentType, 'route'>; label: string };
  const addOptions: AddOption[] = (
    ['workflow-draft', 'debug-session', 'trace', 'event', 'profile'] as const
  ).map((type) => ({ type, label: DOCUMENT_TYPE_LABELS[type] }));

  function iconFor(tab: WorkspaceDocument) {
    if (tab.type && tab.type !== 'route') return DOCUMENT_ICONS[tab.type];
    return tab.view ? VIEW_ICONS[tab.view] : null;
  }

  function onSelect(id: string): void {
    dispatch('select', id);
  }

  function onAddSelect(type: DocumentType): void {
    addMenuOpen = false;
    dispatch('add', type);
  }

  function toggleAddMenu(event: MouseEvent): void {
    event.stopPropagation();
    addMenuOpen = !addMenuOpen;
  }

  function closeAddMenu(): void {
    addMenuOpen = false;
  }

  $: tabStop = tabs.some((tab) => tab.id === activeTabId) ? activeTabId : (tabs[0]?.id ?? null);

  function onTabKeydown(event: KeyboardEvent, index: number, id: string): void {
    if (event.key === 'Enter' || event.key === ' ') {
      event.preventDefault();
      onSelect(id);
      return;
    }
    if (event.key === 'Delete') {
      event.preventDefault();
      dispatch('close', id);
      return;
    }
    let next: number;
    switch (event.key) {
      case 'ArrowRight':
        next = (index + 1) % tabs.length;
        break;
      case 'ArrowLeft':
        next = (index - 1 + tabs.length) % tabs.length;
        break;
      case 'Home':
        next = 0;
        break;
      case 'End':
        next = tabs.length - 1;
        break;
      default:
        return;
    }
    event.preventDefault();
    const list = (event.currentTarget as HTMLElement).closest('[role="tablist"]');
    list?.querySelectorAll<HTMLElement>('[role="tab"]')[next]?.focus();
  }
</script>

<svelte:window on:click={closeAddMenu} />

<div class="editor-tabs">
  <div class="tab-list" role="tablist" aria-label="Open editors">
    {#each tabs as tab, index (tab.id)}
      {@const glyph = iconFor(tab)}
      <div
        class="tab"
        class:active={tab.id === activeTabId}
        role="tab"
        aria-selected={tab.id === activeTabId}
        aria-label={tab.title}
        tabindex={tab.id === tabStop ? 0 : -1}
        title={tab.subtitle ? `${tab.title} — ${tab.subtitle}` : tab.title}
        on:click={() => onSelect(tab.id)}
        on:keydown={(e) => onTabKeydown(e, index, tab.id)}
      >
        {#if glyph}
          <Icon icon={glyph} size={14} class="tab-icon" />
        {/if}
        <span class="tab-title">{tab.title}</span>
        {#if tab.subtitle}
          <span class="tab-subtitle">{tab.subtitle}</span>
        {/if}
        {#if tab.dirty}
          <span class="dirty-indicator" aria-label="Unsaved changes"></span>
        {/if}
        <button
          type="button"
          class="tab-close"
          aria-label="Close {tab.title}"
          title="Close"
          tabindex="-1"
          on:click|stopPropagation={() => dispatch('close', tab.id)}
        >
          <Icon icon={X} size={14} />
        </button>
      </div>
    {/each}
  </div>

  <div class="tab-add-wrapper">
    <IconButton
      icon={Plus}
      label="Open new document"
      aria-haspopup="menu"
      aria-expanded={addMenuOpen}
      onclick={toggleAddMenu}
    />

    {#if addMenuOpen}
      <div class="add-menu" role="menu" aria-label="New document type">
        {#each addOptions as opt (opt.type)}
          <button
            type="button"
            class="add-menu-item"
            role="menuitem"
            on:click|stopPropagation={() => onAddSelect(opt.type)}
          >
            <Icon icon={DOCUMENT_ICONS[opt.type]} size={14} />
            <span>{opt.label}</span>
          </button>
        {/each}
      </div>
    {/if}
  </div>
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

  .tab-list {
    display: flex;
    min-width: 0;
    overflow-x: auto;
    scrollbar-width: none;
  }

  .tab-list::-webkit-scrollbar {
    display: none;
  }

  .tab {
    position: relative;
    display: inline-flex;
    align-items: center;
    gap: 6px;
    flex: 0 0 auto;
    max-width: 240px;
    height: 100%;
    padding: 0 4px 0 10px;
    border-right: 1px solid var(--ide-tab-border, var(--color-border-subtle));
    color: var(--color-text-tertiary);
    font-size: var(--text-ui);
    white-space: nowrap;
    cursor: pointer;
    transition: var(--transition-colors);
  }

  .tab:hover {
    background: var(--color-bg-hover);
    color: var(--color-text-primary);
  }

  .tab.active {
    background: var(--ide-tab-active-bg, var(--color-bg-base));
    color: var(--color-text-primary);
  }

  .tab.active::after {
    content: '';
    position: absolute;
    left: 0;
    right: 0;
    bottom: -1px;
    height: 2px;
    background: var(--color-primary);
  }

  .tab:focus-visible {
    outline: 2px solid var(--color-focus-ring);
    outline-offset: -2px;
  }

  .tab :global(.tab-icon) {
    color: var(--color-text-tertiary);
  }

  .tab-title {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .tab-subtitle {
    max-width: 96px;
    overflow: hidden;
    text-overflow: ellipsis;
    color: var(--color-text-muted);
    font-size: var(--text-xs);
  }

  .dirty-indicator {
    width: 6px;
    height: 6px;
    flex: 0 0 auto;
    border-radius: var(--radius-full);
    background: var(--color-warning);
  }

  .tab-close {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 20px;
    height: 20px;
    flex: 0 0 auto;
    padding: 0;
    border: none;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--color-text-tertiary);
    cursor: pointer;
    visibility: hidden;
  }

  .tab:hover .tab-close,
  .tab:focus-within .tab-close,
  .tab.active .tab-close {
    visibility: visible;
  }

  .tab-close:hover {
    background: var(--color-bg-active);
    color: var(--color-text-primary);
  }

  .tab-add-wrapper {
    position: relative;
    display: flex;
    align-items: center;
    flex: 0 0 auto;
    padding: 0 var(--space-1);
  }

  .add-menu {
    position: absolute;
    top: calc(100% + 2px);
    left: var(--space-1);
    z-index: var(--z-dropdown);
    min-width: 180px;
    padding: var(--space-1);
    border: 1px solid var(--color-border-default);
    border-radius: var(--radius-md);
    background: var(--color-bg-overlay);
    box-shadow: var(--shadow-lg);
  }

  .add-menu-item {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    width: 100%;
    height: 28px;
    padding: 0 var(--space-2);
    border: none;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--color-text-secondary);
    font: inherit;
    font-size: var(--text-ui);
    text-align: left;
    cursor: pointer;
    transition: var(--transition-colors);
  }

  .add-menu-item:hover {
    background: var(--color-bg-hover);
    color: var(--color-text-primary);
  }

  .add-menu-item:focus-visible {
    outline: 2px solid var(--color-focus-ring);
    outline-offset: -2px;
  }
</style>
