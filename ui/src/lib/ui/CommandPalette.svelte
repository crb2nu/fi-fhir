<script context="module" lang="ts">
  export type PaletteCommand = {
    id: string;
    label: string;
    hint?: string;
    keywords?: string[];
    run: () => void | Promise<void>;
  };
</script>

<script lang="ts">
  import { afterUpdate, createEventDispatcher, tick } from 'svelte';
  import { createDialogFocusController } from '$lib/domain/a11yDialog';
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
      const hay = [c.label, c.hint ?? '', ...(c.keywords ?? [])].join(' ').toLowerCase();
      return hay.includes(q);
    });
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
      <div class="head">
        <div class="title" id="cmd-title">{title}</div>
        <div class="kbd">
          <span class="key">Esc</span>
          <span class="key">Enter</span>
          <span class="key">↑</span>
          <span class="key">↓</span>
        </div>
      </div>

      <div class="search">
        <label class="sr-only" for="cmd-query">Search commands</label>
        <input
          id="cmd-query"
          bind:this={inputEl}
          class="input"
          type="text"
          bind:value={query}
          on:input={onQueryInput}
          placeholder="Type to filter commands…"
          autocomplete="off"
        />
      </div>

      <div class="list" bind:this={listEl} role="listbox" aria-label="Commands">
        {#if filtered.length === 0}
          <div class="empty">No matches</div>
        {:else}
          {#each filtered as c, i (c.id)}
            <button
              type="button"
              class="item"
              class:active={i === activeIndex}
              role="option"
              aria-selected={i === activeIndex}
              on:mouseenter={() => (activeIndex = i)}
              on:click={() => {
                activeIndex = i;
                void runActive();
              }}
            >
              <div class="label">{c.label}</div>
              {#if c.hint}
                <div class="hint">{c.hint}</div>
              {/if}
            </button>
          {/each}
        {/if}
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
    padding: var(--space-8) var(--space-4);
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
    width: 100%;
    max-width: 680px;
    border-radius: var(--radius-2xl);
    border: 1px solid var(--color-border-default);
    background: rgba(12, 18, 34, 0.92);
    box-shadow: var(--shadow-xl);
    overflow: hidden;
    outline: none;
    backdrop-filter: blur(10px);
  }

  .head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3);
    padding: var(--space-4) var(--space-4) var(--space-3);
    border-bottom: 1px solid var(--color-border-subtle);
  }

  .title {
    font-weight: var(--font-bold);
    letter-spacing: var(--tracking-tight);
    color: var(--color-text-primary);
  }

  .kbd {
    display: flex;
    gap: var(--space-1);
    flex-wrap: wrap;
    justify-content: flex-end;
  }

  .key {
    font-family: var(--font-mono);
    font-size: var(--text-2xs);
    color: var(--color-text-tertiary);
    border: 1px solid var(--color-border-subtle);
    background: rgba(255, 255, 255, 0.03);
    padding: 2px 6px;
    border-radius: var(--radius-sm);
  }

  .search {
    padding: var(--space-3) var(--space-4);
    border-bottom: 1px solid var(--color-border-subtle);
  }

  .input {
    width: 100%;
    padding: 10px 12px;
    border-radius: var(--radius-xl);
    border: 1px solid var(--color-border-default);
    background: rgba(255, 255, 255, 0.03);
    color: var(--color-text-primary);
    outline: none;
    transition: var(--transition-all);
  }

  .input:focus-visible {
    border-color: var(--color-border-focus);
    box-shadow: var(--shadow-focus);
  }

  .list {
    max-height: 360px;
    overflow: auto;
    padding: var(--space-2);
  }

  .empty {
    padding: var(--space-6) var(--space-4);
    color: var(--color-text-tertiary);
    text-align: center;
  }

  .item {
    width: 100%;
    text-align: left;
    border: 1px solid transparent;
    background: transparent;
    padding: var(--space-3) var(--space-3);
    border-radius: var(--radius-xl);
    color: var(--color-text-secondary);
    cursor: pointer;
    display: grid;
    gap: 2px;
    transition: var(--transition-colors);
  }

  .item:hover {
    background: var(--color-bg-hover);
    color: var(--color-text-primary);
  }

  .item.active {
    background: rgba(59, 130, 246, 0.10);
    border-color: rgba(59, 130, 246, 0.30);
    color: var(--color-text-primary);
  }

  .item:focus-visible {
    outline: none;
    box-shadow: var(--shadow-focus);
    border-color: var(--color-border-focus);
  }

  .label {
    font-weight: var(--font-semibold);
    color: inherit;
  }

  .hint {
    font-size: var(--text-xs);
    color: var(--color-text-tertiary);
  }
</style>
