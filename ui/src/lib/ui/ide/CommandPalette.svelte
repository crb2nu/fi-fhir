<!--
  The command palette: one for the whole IDE, opened by Cmd/Ctrl+K and the
  header's Commands button, listing every command in the registry whose
  `when` holds (the current route's first, then the shell's). A `Dialog` in
  its bare layout, 64 px from the top: a search field, the grouped list, and
  a key legend. ArrowUp/ArrowDown move, Enter runs, Escape closes.
-->
<script lang="ts">
  import Search from '@lucide/svelte/icons/search';
  import { Dialog, Icon } from '$lib/ui/primitives';
  import { toasts } from '$lib/ui/toastStore';
  import {
    closePalette,
    commands,
    filterCommands,
    paletteOpen,
    visibleCommands,
    type Command
  } from './commandRegistry';

  const uid = $props.id();
  const inputId = `${uid}-query`;
  const listId = `${uid}-list`;

  let query = $state('');
  let activeIndex = $state(0);
  let listEl: HTMLDivElement | undefined = $state();

  // `when` predicates are read as the palette opens and whenever the
  // registry changes while it is open.
  const available = $derived($paletteOpen ? visibleCommands($commands) : []);
  const filtered = $derived(filterCommands(available, query));
  const groups = $derived.by(() => {
    const out: { group: string; items: { command: Command; index: number }[] }[] = [];
    filtered.forEach((command, index) => {
      const last = out[out.length - 1];
      const existing = last && last.group === command.group ? last : out.find((g) => g.group === command.group);
      if (existing) existing.items.push({ command, index });
      else out.push({ group: command.group, items: [{ command, index }] });
    });
    return out;
  });
  // Display order (groups are contiguous) → the index the keyboard walks.
  const order = $derived(groups.flatMap((group) => group.items.map((item) => item.index)));

  $effect.pre(() => {
    if (!$paletteOpen) return;
    query = '';
    activeIndex = 0;
  });

  $effect(() => {
    if (activeIndex >= filtered.length) activeIndex = Math.max(0, filtered.length - 1);
  });

  $effect(() => {
    if (!$paletteOpen) return;
    const active = listEl?.querySelector<HTMLElement>(`[data-index="${activeIndex}"]`);
    active?.scrollIntoView?.({ block: 'nearest' });
  });

  function move(delta: number): void {
    if (order.length === 0) return;
    const at = Math.max(0, order.indexOf(activeIndex));
    activeIndex = order[(at + delta + order.length) % order.length] ?? 0;
  }

  async function run(command: Command | undefined): Promise<void> {
    if (!command) return;
    closePalette();
    try {
      await command.run();
    } catch (err) {
      console.error('Command palette command failed:', command.id, err);
      toasts.error(`Command failed: ${command.label}`);
    }
  }

  function onKeydown(event: KeyboardEvent): void {
    switch (event.key) {
      case 'ArrowDown':
        event.preventDefault();
        move(1);
        return;
      case 'ArrowUp':
        event.preventDefault();
        move(-1);
        return;
      case 'Enter':
        event.preventDefault();
        void run(filtered[activeIndex]);
        return;
    }
  }
</script>

<Dialog
  open={$paletteOpen}
  title="Commands"
  layout="bare"
  placement="top"
  size="md"
  initialFocus={`[id="${inputId}"]`}
  onclose={closePalette}
  class="command-palette"
  data-testid="command-palette"
>
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div class="palette" onkeydown={onKeydown}>
    <div class="search">
      <Icon icon={Search} class="search-icon" />
      <label class="sr-only" for={inputId}>Search commands</label>
      <input
        id={inputId}
        class="input"
        type="text"
        bind:value={query}
        oninput={() => (activeIndex = order[0] ?? 0)}
        placeholder="Type a command or view"
        autocomplete="off"
        spellcheck="false"
        aria-controls={listId}
        aria-activedescendant={filtered.length > 0 ? `${uid}-option-${activeIndex}` : undefined}
      />
    </div>

    <div class="list" id={listId} bind:this={listEl} role="listbox" aria-label="Commands">
      {#if filtered.length === 0}
        <div class="empty">No commands match.</div>
      {:else}
        {#each groups as group (group.group)}
          <div class="category-header" aria-hidden="true">{group.group}</div>
          {#each group.items as item (item.index)}
            <button
              type="button"
              id={`${uid}-option-${item.index}`}
              class="item"
              class:active={item.index === activeIndex}
              role="option"
              aria-selected={item.index === activeIndex}
              data-index={item.index}
              tabindex="-1"
              onmouseenter={() => (activeIndex = item.index)}
              onclick={() => void run(item.command)}
            >
              <span class="label">{item.command.label}</span>
              {#if item.command.hint}
                <span class="hint">{item.command.hint}</span>
              {/if}
              {#if item.command.shortcut}
                <kbd class="shortcut">{item.command.shortcut}</kbd>
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
</Dialog>

<style>
  .palette {
    display: flex;
    flex-direction: column;
    min-height: 0;
    flex: 1 1 auto;
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

  .sr-only {
    position: absolute;
    width: 1px;
    height: 1px;
    margin: -1px;
    padding: 0;
    overflow: hidden;
    clip: rect(0, 0, 0, 0);
    white-space: nowrap;
    border: 0;
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
