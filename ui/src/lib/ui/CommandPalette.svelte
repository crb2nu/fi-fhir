<script context="module" lang="ts">
  export type PaletteCommand = {
    id: string;
    label: string;
    hint?: string;
    keywords?: string[];
    category?: string;
    /** Keyboard shortcut shown right-aligned, in the platform's notation (e.g. "⌘B", "Ctrl+B"). */
    shortcut?: string;
    run: () => void | Promise<void>;
  };
</script>

<script lang="ts">
  import { afterUpdate, createEventDispatcher, tick } from 'svelte';
  import Search from '@lucide/svelte/icons/search';
  import { createDialogFocusController } from '$lib/domain/a11yDialog';
  import Icon from '$lib/ui/primitives/Icon.svelte';
  import { toasts } from '$lib/ui/toastStore';

  export let open = false;
  export let title = 'Command palette';
  export let commands: readonly PaletteCommand[] = [];

  const dispatch = createEventDispatcher<{ close: void }>();

  let rootEl: HTMLDivElement | null = null;
  let inputEl: HTMLInputElement | null = null;
  let listEl: HTMLDivElement | null = null;
  let wasOpen = false;
  let focusCtl: ReturnType<typeof createDialogFocusController> | null = null;

  let query = '';
  let activeIndex = 0;
  let lastScrollIndex = -1;

  function close(): void {
    open = false;
    dispatch('close');
  }

  function onQueryInput(): void {
    activeIndex = 0;
  }

  function norm(s: string): string {
    return s.trim().toLowerCase();
  }

  $: filtered = (() => {
    const q = norm(query);
    if (!q) return commands;
    return commands.filter((c) => {
      const hay = [c.label, c.hint ?? '', c.category ?? '', ...(c.keywords ?? [])].join(' ').toLowerCase();
      return hay.includes(q);
    });
  })();

  /** Group filtered commands by category for display. */
  $: grouped = (() => {
    // eslint-disable-next-line svelte/prefer-svelte-reactivity -- local ephemeral grouping, not stored as reactive state
    const map = new Map<string, { cmds: PaletteCommand[]; startIdx: number }>();
    let idx = 0;
    for (const c of filtered) {
      const cat = c.category ?? '';
      if (!map.has(cat)) {
        map.set(cat, { cmds: [], startIdx: idx });
      }
      map.get(cat)!.cmds.push(c);
      idx++;
    }
    return [...map.entries()].map(([cat, val]) => ({
      category: cat,
      commands: val.cmds,
      startIdx: val.startIdx,
    }));
  })();

  $: if (activeIndex >= filtered.length) activeIndex = Math.max(0, filtered.length - 1);

  async function runActive(): Promise<void> {
    const cmd = filtered[activeIndex];
    if (!cmd) return;
    try {
      await cmd.run();
      close();
    } catch (err) {
      console.error('Command palette command failed:', cmd.id, err);
      toasts.error(`Command failed: ${cmd.label}`);
    }
  }

  function onKeydown(e: KeyboardEvent): void {
    if (!open) return;

    if (e.key === 'Escape') {
      e.preventDefault();
      close();
      return;
    }

    if (e.key === 'Tab') {
      focusCtl?.onKeydown(e);
      return;
    }

    if (e.key === 'ArrowDown') {
      e.preventDefault();
      if (filtered.length === 0) return;
      activeIndex = (activeIndex + 1) % filtered.length;
      return;
    }

    if (e.key === 'ArrowUp') {
      e.preventDefault();
      if (filtered.length === 0) return;
      activeIndex = (activeIndex - 1 + filtered.length) % filtered.length;
      return;
    }

    if (e.key === 'Enter') {
      e.preventDefault();
      void runActive();
    }
  }

  afterUpdate(() => {
    if (open && !wasOpen) {
      query = '';
      activeIndex = 0;
      lastScrollIndex = -1;
      tick().then(() => {
        if (!rootEl) return;
        focusCtl = createDialogFocusController(rootEl, { initialFocus: inputEl });
        focusCtl.focusInitial();
      });
    }
    if (!open && wasOpen) {
      focusCtl?.restoreFocus();
      focusCtl = null;
    }
    if (open && lastScrollIndex !== activeIndex) {
      lastScrollIndex = activeIndex;
      tick().then(() => {
        const active = listEl?.querySelector<HTMLElement>('.item.active');
        active?.scrollIntoView({ block: 'nearest' });
      });
    }
    wasOpen = open;
  });
</script>

<svelte:window on:keydown={onKeydown} />

{#if open}
  <div class="overlay">
    <button
      type="button"
      class="backdrop"
      aria-label="Close command palette"
      tabindex="-1"
      on:click={close}
    ></button>

    <div
      class="palette"
      bind:this={rootEl}
      role="dialog"
      aria-modal="true"
      aria-labelledby="cmd-title"
      tabindex="-1"
    >
      <h2 class="sr-only" id="cmd-title">{title}</h2>

      <div class="search">
        <Icon icon={Search} class="search-icon" />
        <label class="sr-only" for="cmd-query">Search commands</label>
        <input
          id="cmd-query"
          bind:this={inputEl}
          class="input"
          type="text"
          bind:value={query}
          on:input={onQueryInput}
          placeholder="Type a command or view"
          autocomplete="off"
          spellcheck="false"
        />
        <span class="scope">{title}</span>
      </div>

      <div class="list" bind:this={listEl} role="listbox" aria-label="Commands">
        {#if filtered.length === 0}
          <div class="empty">No commands match.</div>
        {:else}
          {#each grouped as group (group.category)}
            {#if group.category}
              <div class="category-header">{group.category}</div>
            {/if}
            {#each group.commands as c, ci (c.id)}
              {@const globalIdx = group.startIdx + ci}
              <button
                type="button"
                class="item"
                class:active={globalIdx === activeIndex}
                role="option"
                aria-selected={globalIdx === activeIndex}
                on:mouseenter={() => (activeIndex = globalIdx)}
                on:click={() => {
                  activeIndex = globalIdx;
                  void runActive();
                }}
              >
                <span class="label">{c.label}</span>
                {#if c.hint}
                  <span class="hint">{c.hint}</span>
                {/if}
                {#if c.shortcut}
                  <kbd class="shortcut">{c.shortcut}</kbd>
                {/if}
              </button>
            {/each}
          {/each}
        {/if}
      </div>

      <div class="footer" aria-hidden="true">
        <span><kbd>↑</kbd><kbd>↓</kbd> Move</span>
        <span><kbd>Enter</kbd> Run</span>
        <span><kbd>Esc</kbd> Close</span>
      </div>
    </div>
  </div>
{/if}

<style>
  .overlay {
    position: fixed;
    inset: 0;
    z-index: var(--z-modal);
    display: flex;
    align-items: start;
    justify-content: center;
    padding: 64px var(--space-4) var(--space-4);
  }

  .backdrop {
    position: absolute;
    inset: 0;
    border: 0;
    padding: 0;
    background: var(--modal-backdrop);
    cursor: default;
  }

  .palette {
    position: relative;
    z-index: 1;
    display: flex;
    flex-direction: column;
    width: 100%;
    max-width: 580px;
    max-height: min(480px, calc(100vh - 96px));
    overflow: hidden;
    border: 1px solid var(--color-border-default);
    border-radius: var(--radius-md);
    background: var(--color-bg-overlay);
    box-shadow: var(--shadow-xl);
    outline: none;
  }

  .search {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    flex: 0 0 auto;
    height: 40px;
    padding: 0 var(--space-3);
    border-bottom: 1px solid var(--color-border-subtle);
    color: var(--color-text-tertiary);
  }

  .input {
    flex: 1;
    min-width: 0;
    height: 100%;
    padding: 0;
    border: none;
    background: transparent;
    color: var(--color-text-primary);
    font: inherit;
    font-size: var(--text-ui);
    outline: none;
  }

  .input::placeholder {
    color: var(--color-text-muted);
  }

  .scope {
    flex: 0 0 auto;
    color: var(--color-text-tertiary);
    font-size: var(--text-label);
    font-weight: var(--font-semibold);
    letter-spacing: var(--tracking-label);
    text-transform: uppercase;
  }

  .list {
    flex: 1 1 auto;
    min-height: 0;
    overflow: auto;
    padding: var(--space-1);
  }

  .category-header {
    padding: var(--space-2) var(--space-2) var(--space-1);
    color: var(--color-text-tertiary);
    font-size: var(--text-label);
    font-weight: var(--font-semibold);
    letter-spacing: var(--tracking-label);
    text-transform: uppercase;
  }

  .empty {
    padding: var(--space-4) var(--space-3);
    color: var(--color-text-secondary);
  }

  .item {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    width: 100%;
    height: 30px;
    padding: 0 var(--space-2);
    border: none;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--color-text-secondary);
    font: inherit;
    font-size: var(--text-ui);
    text-align: left;
    cursor: pointer;
  }

  .item.active {
    background: var(--color-primary-muted);
    color: var(--color-text-primary);
  }

  .item:focus-visible {
    outline: 2px solid var(--color-focus-ring);
    outline-offset: -2px;
  }

  .label {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .hint {
    margin-left: auto;
    color: var(--color-text-muted);
    font-family: var(--font-mono);
    font-size: var(--text-label);
    white-space: nowrap;
  }

  .shortcut {
    margin-left: auto;
  }

  .hint + .shortcut {
    margin-left: 0;
  }

  kbd {
    display: inline-block;
    min-width: 18px;
    padding: 2px 5px;
    border: 1px solid var(--color-border-default);
    border-radius: var(--radius-sm);
    color: var(--color-text-tertiary);
    font-family: var(--font-mono);
    font-size: var(--text-label);
    line-height: 1;
    text-align: center;
  }

  .footer {
    display: flex;
    gap: var(--space-4);
    flex: 0 0 auto;
    padding: 6px var(--space-3);
    border-top: 1px solid var(--color-border-subtle);
    color: var(--color-text-muted);
    font-size: var(--text-label);
  }

  .footer kbd {
    margin-right: 2px;
  }
</style>
