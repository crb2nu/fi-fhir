<script lang="ts">
  import type { WarningGroup } from '$lib/domain/warnings';
  import type { WarningLike } from '$lib/domain/warnings';
  import { browser } from '$app/environment';
  import { createEventDispatcher } from 'svelte';

  export let groups: readonly WarningGroup[];
  export let selectedPath: string | null = null;
  export let enableControls = true;

  const dispatch = createEventDispatcher<{ select: WarningLike; inspect: WarningLike }>();

  let query = '';
  let phase: string = 'all';
  let onlyWithPath = false;

  function matches(w: WarningLike): boolean {
    if (onlyWithPath && !w.path) return false;
    const q = query.trim().toLowerCase();
    if (!q) return true;
    const hay = [w.phase, w.code, w.message, w.path ?? ''].join(' ').toLowerCase();
    return hay.includes(q);
  }

  $: phases = groups.map((g) => g.phase);
  $: filteredGroups = groups
    .filter((g) => phase === 'all' || g.phase === phase)
    .map((g) => ({ phase: g.phase, items: g.items.filter(matches) }))
    .filter((g) => g.items.length > 0);

  $: total = groups.reduce((acc, g) => acc + g.items.length, 0);
  $: shown = filteredGroups.reduce((acc, g) => acc + g.items.length, 0);

  async function copyText(text: string): Promise<void> {
    if (!browser) return;
    if (!text) return;
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(text);
      return;
    }
    const ta = document.createElement('textarea');
    ta.value = text;
    ta.style.position = 'fixed';
    ta.style.opacity = '0';
    document.body.appendChild(ta);
    ta.select();
    document.execCommand('copy');
    document.body.removeChild(ta);
  }
</script>

{#if groups.length === 0}
  <div class="empty">No warnings</div>
{:else}
  {#if enableControls}
    <div class="controls">
      <div class="search">
        <input
          class="input"
          type="text"
          bind:value={query}
          placeholder="Filter warnings by code, message, phase, path…"
        />
        {#if query.trim()}
          <button class="clear" type="button" on:click={() => (query = '')}>Clear</button>
        {/if}
        <span class="count">{shown}/{total}</span>
      </div>

      <div class="filters">
        <div class="chip-row">
          <button
            class="chip"
            class:active={phase === 'all'}
            type="button"
            on:click={() => (phase = 'all')}
          >
            all
          </button>
          {#each phases as p (p)}
            <button
              class="chip"
              class:active={phase === p}
              type="button"
              on:click={() => (phase = p)}
            >
              {p}
            </button>
          {/each}
        </div>

        <label class="checkbox">
          <input type="checkbox" bind:checked={onlyWithPath} />
          Has path
        </label>
      </div>
    </div>
  {/if}

  <div class="groups">
    {#each filteredGroups as g (g.phase)}
      <div class="group">
        <div class="group-title">
          <span class="phase">{g.phase}</span>
          <span class="count">{g.items.length}</span>
        </div>
        <ul class="list">
          {#each g.items as w, idx (w.phase + ':' + w.code + ':' + idx)}
            <li class="li">
              <div class="item" data-selected={selectedPath !== null && w.path === selectedPath}>
                <button class="main" on:click={() => dispatch('select', w)} type="button">
                  <div class="top">
                    <span class="code">{w.code}</span>
                    {#if w.path}
                      <span class="path" title={w.path}>{w.path}</span>
                    {/if}
                  </div>
                  <div class="msg">{w.message}</div>
                </button>

                <div class="actions">
                  {#if w.path}
                    <button
                      class="mini"
                      type="button"
                      title="Copy path"
                      on:click|stopPropagation={() => copyText(w.path ?? '')}
                    >
                      Copy
                    </button>
                    <button
                      class="mini"
                      type="button"
                      title="Open inspector"
                      on:click|stopPropagation={() => dispatch('inspect', w)}
                    >
                      Inspect
                    </button>
                  {/if}
                </div>
              </div>
            </li>
          {/each}
        </ul>
      </div>
    {/each}
  </div>
{/if}

<style>
  .empty {
    color: rgba(229, 231, 235, 0.7);
  }

  .controls {
    display: grid;
    gap: 10px;
    margin-bottom: 12px;
  }

  .search {
    display: flex;
    align-items: center;
    gap: 10px;
    flex-wrap: wrap;
  }

  .input {
    flex: 1;
    min-width: 240px;
    padding: 10px 12px;
    border-radius: 12px;
    border: 1px solid rgba(255, 255, 255, 0.12);
    background: rgba(255, 255, 255, 0.03);
    color: rgba(229, 231, 235, 0.92);
    outline: none;
  }

  .input:focus {
    border-color: rgba(59, 130, 246, 0.45);
    box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.15);
  }

  .clear {
    padding: 8px 10px;
    border-radius: 10px;
    border: 1px solid rgba(255, 255, 255, 0.12);
    background: rgba(255, 255, 255, 0.04);
    color: rgba(229, 231, 235, 0.86);
    font-weight: 700;
    cursor: pointer;
  }

  .clear:hover {
    background: rgba(255, 255, 255, 0.06);
  }

  .filters {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    flex-wrap: wrap;
  }

  .chip-row {
    display: flex;
    gap: 8px;
    flex-wrap: wrap;
  }

  .chip {
    padding: 6px 10px;
    border-radius: 999px;
    border: 1px solid rgba(255, 255, 255, 0.12);
    background: rgba(255, 255, 255, 0.04);
    color: rgba(229, 231, 235, 0.85);
    font-weight: 750;
    cursor: pointer;
  }

  .chip:hover {
    background: rgba(255, 255, 255, 0.06);
  }

  .chip.active {
    border-color: rgba(59, 130, 246, 0.45);
    background: rgba(59, 130, 246, 0.12);
  }

  .checkbox {
    display: flex;
    align-items: center;
    gap: 8px;
    color: rgba(229, 231, 235, 0.8);
    font-weight: 650;
    font-size: 0.9rem;
  }

  .count {
    color: rgba(229, 231, 235, 0.6);
    font-weight: 750;
    font-size: 0.85rem;
  }

  .groups {
    display: grid;
    gap: 12px;
  }

  .group {
    border-radius: 12px;
    border: 1px solid rgba(255, 255, 255, 0.08);
    background: rgba(255, 255, 255, 0.02);
    padding: 10px 12px;
  }

  .group-title {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 8px;
    color: rgba(243, 244, 246, 0.95);
    font-weight: 700;
  }

  .phase {
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, 'Liberation Mono', 'Courier New', monospace;
    font-size: 0.95rem;
  }

  .count {
    padding: 2px 8px;
    border-radius: 999px;
    border: 1px solid rgba(255, 255, 255, 0.12);
    background: rgba(255, 255, 255, 0.04);
    color: rgba(229, 231, 235, 0.88);
    font-size: 0.85rem;
  }

  .list {
    list-style: none;
    padding: 0;
    margin: 0;
    display: grid;
    gap: 10px;
  }

  .li {
    list-style: none;
  }

  .item {
    border-radius: 10px;
    border: 1px solid rgba(255, 255, 255, 0.08);
    background: rgba(255, 255, 255, 0.03);
    display: grid;
    grid-template-columns: 1fr auto;
    gap: 10px;
    padding: 10px;
  }

  .main {
    width: 100%;
    text-align: left;
    border: 0;
    padding: 0;
    background: transparent;
    cursor: pointer;
    color: inherit;
  }

  .main:hover {
    opacity: 0.98;
  }

  .item[data-selected='true'] {
    border-color: rgba(59, 130, 246, 0.45);
    background: rgba(59, 130, 246, 0.12);
  }

  .actions {
    display: flex;
    gap: 8px;
    align-items: start;
  }

  .mini {
    padding: 6px 10px;
    border-radius: 10px;
    border: 1px solid rgba(255, 255, 255, 0.12);
    background: rgba(255, 255, 255, 0.04);
    color: rgba(229, 231, 235, 0.86);
    font-weight: 750;
    cursor: pointer;
    white-space: nowrap;
  }

  .mini:hover {
    background: rgba(255, 255, 255, 0.06);
  }

  .top {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 10px;
  }

  .code {
    font-weight: 800;
    color: rgba(229, 231, 235, 0.92);
  }

  .path {
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, 'Liberation Mono', 'Courier New', monospace;
    font-size: 0.85rem;
    color: rgba(229, 231, 235, 0.7);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    max-width: 60%;
  }

  .msg {
    margin-top: 6px;
    color: rgba(229, 231, 235, 0.82);
    line-height: 1.4;
  }
</style>
