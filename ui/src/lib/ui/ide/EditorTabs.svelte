<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import type { WorkspaceDocument, DocumentType } from './types';

  /**
   * Multi-tab editor bar with artifact type badges, dirty indicators,
   * close buttons, and a quick-add button.
   */

  export let tabs: WorkspaceDocument[] = [];
  export let activeTabId: string | null = null;

  const dispatch = createEventDispatcher<{
    select: string;
    close: string;
    add: DocumentType;
  }>();

  /** Color mapping for artifact type badge dots. */
  const TYPE_COLORS: Record<DocumentType, string | null> = {
    route: null,
    'workflow-draft': 'var(--color-primary)',
    'debug-session': 'var(--color-warning)',
    trace: 'var(--color-info)',
    event: 'var(--color-success)',
    profile: 'var(--palette-violet-600)',
  };

  let addMenuOpen = false;

  type AddOption = { type: DocumentType; label: string };
  const addOptions: AddOption[] = [
    { type: 'workflow-draft', label: 'Workflow Draft' },
    { type: 'debug-session', label: 'Debug Session' },
    { type: 'trace', label: 'Trace View' },
    { type: 'event', label: 'Event Payload' },
    { type: 'profile', label: 'Source Profile' },
  ];

  function onSelect(id: string): void {
    dispatch('select', id);
  }

  function onAddSelect(type: DocumentType): void {
    addMenuOpen = false;
    dispatch('add', type);
  }

  function toggleAddMenu(): void {
    addMenuOpen = !addMenuOpen;
  }

  function closeAddMenu(): void {
    addMenuOpen = false;
  }
</script>

<svelte:window on:click={closeAddMenu} />

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
      {#if TYPE_COLORS[tab.type ?? 'route']}
        <span
          class="type-badge"
          style="background: {TYPE_COLORS[tab.type ?? 'route']}"
          title={tab.type}
        ></span>
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

  <!-- Quick add button -->
  <div class="tab-add-wrapper">
    <button
      type="button"
      class="tab-add"
      aria-label="Open new document"
      title="Open new document"
      on:click|stopPropagation={toggleAddMenu}
    >
      <svg
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        aria-hidden="true"
      >
        <path d="M12 5v14" />
        <path d="M5 12h14" />
      </svg>
    </button>

    {#if addMenuOpen}
      <div class="add-menu" role="menu" aria-label="New document type">
        {#each addOptions as opt (opt.type)}
          <button
            type="button"
            class="add-menu-item"
            role="menuitem"
            on:click|stopPropagation={() => onAddSelect(opt.type)}
          >
            <span
              class="add-menu-dot"
              style="background: {TYPE_COLORS[opt.type]}"
            ></span>
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
    max-width: 240px;
    animation: tabSlideIn var(--duration-slow) var(--ease-out);
  }

  @keyframes tabSlideIn {
    from {
      opacity: 0;
      transform: translateY(4px);
    }
    to {
      opacity: 1;
      transform: translateY(0);
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .tab {
      animation: none;
    }
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

  /* Artifact type badge */
  .type-badge {
    width: 7px;
    height: 7px;
    border-radius: 50%;
    flex: 0 0 auto;
  }

  .tab-title {
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .tab-subtitle {
    overflow: hidden;
    text-overflow: ellipsis;
    font-size: var(--text-2xs);
    color: var(--color-text-muted);
    max-width: 80px;
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

  /* Add button */
  .tab-add-wrapper {
    position: relative;
    flex: 0 0 auto;
    display: flex;
    align-items: center;
  }

  .tab-add {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 28px;
    height: 28px;
    margin: 0 var(--space-2);
    padding: 0;
    border: none;
    border-radius: var(--radius-md);
    background: transparent;
    color: var(--color-text-muted);
    cursor: pointer;
    transition: var(--transition-all);
  }

  .tab-add:hover {
    background: var(--color-bg-hover);
    color: var(--color-text-primary);
  }

  .tab-add:focus-visible {
    outline: none;
    box-shadow: var(--shadow-focus);
  }

  .tab-add svg {
    width: 14px;
    height: 14px;
  }

  /* Add menu dropdown */
  .add-menu {
    position: absolute;
    top: 100%;
    left: 0;
    z-index: var(--z-dropdown);
    min-width: 180px;
    padding: var(--space-1);
    border: 1px solid var(--color-border-default);
    border-radius: var(--radius-lg);
    background: var(--color-bg-elevated);
    box-shadow: var(--shadow-md);
    backdrop-filter: blur(8px);
    animation: scaleIn var(--duration-fast) var(--ease-out);
  }

  @keyframes scaleIn {
    from {
      opacity: 0;
      transform: scale(0.95) translateY(-4px);
    }
    to {
      opacity: 1;
      transform: scale(1) translateY(0);
    }
  }

  .add-menu-item {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    width: 100%;
    padding: var(--space-2) var(--space-3);
    border: none;
    border-radius: var(--radius-md);
    background: transparent;
    color: var(--color-text-secondary);
    font-size: var(--text-xs);
    font-weight: var(--font-medium);
    cursor: pointer;
    text-align: left;
    transition: var(--transition-colors);
  }

  .add-menu-item:hover {
    background: var(--color-bg-hover);
    color: var(--color-text-primary);
  }

  .add-menu-item:focus-visible {
    outline: none;
    box-shadow: var(--shadow-focus);
  }

  .add-menu-dot {
    width: 7px;
    height: 7px;
    border-radius: 50%;
    flex: 0 0 auto;
  }
</style>
