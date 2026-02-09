<script lang="ts">
  import type { WarningGroup, WarningLike } from '$lib/domain/warnings';
  import { browser } from '$app/environment';
  import { createEventDispatcher } from 'svelte';
  import { SvelteSet } from 'svelte/reactivity';

  export let groups: readonly WarningGroup[];
  export let selectedPath: string | null = null;
  export let enableControls = true;
  /** Set of warning codes currently being explained */
  export let explainLoadingCodes: SvelteSet<string> = new SvelteSet();

  const dispatch = createEventDispatcher<{
    select: WarningLike;
    inspect: WarningLike;
    explain: WarningLike;
    explainAll: void;
  }>();

  /** Check if a specific warning is loading */
  function isWarningLoading(w: WarningLike): boolean {
    return explainLoadingCodes.has(w.code);
  }

  /** Count of warnings without explanations */
  $: unexplainedCount = groups.reduce(
    (acc, g) => acc + g.items.filter((w) => !w.explanation).length,
    0
  );

  /** Whether any explain operation is in progress */
  $: anyLoading = explainLoadingCodes.size > 0;

  // Track which explanations are expanded
  let expandedExplanations = new SvelteSet<string>();

  function toggleExplanation(warningKey: string) {
    if (expandedExplanations.has(warningKey)) {
      expandedExplanations.delete(warningKey);
    } else {
      expandedExplanations.add(warningKey);
    }
  }

  function warningKey(w: WarningLike, idx: number): string {
    return `${w.phase}:${w.code}:${idx}`;
  }

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
        <label class="sr-only" for="warning-filter">
          Filter warnings
        </label>
        <input
          id="warning-filter"
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

        <div class="filter-actions">
          <label class="checkbox">
            <input type="checkbox" bind:checked={onlyWithPath} />
            Has path
          </label>

          {#if unexplainedCount > 0}
            <button
              class="explain-all-btn"
              type="button"
              disabled={anyLoading}
              on:click={() => dispatch('explainAll')}
            >
              {#if anyLoading}
                Explaining...
              {:else}
                Explain All ({unexplainedCount})
              {/if}
            </button>
          {/if}
        </div>
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
            {@const wKey = warningKey(w, idx)}
            <li class="li">
              <div class="item" data-selected={selectedPath !== null && w.path === selectedPath}>
                <div class="item-content">
                  <button class="main" on:click={() => dispatch('select', w)} type="button">
                    <div class="top">
                      <span class="code">{w.code}</span>
                      {#if w.path}
                        <span class="path" title={w.path}>{w.path}</span>
                      {/if}
                    </div>
                    <div class="msg">{w.message}</div>
                  </button>

                  {#if w.explanation}
                    <div class="explanation">
                      <button
                        class="explain-toggle"
                        type="button"
                        on:click|stopPropagation={() => toggleExplanation(wKey)}
                      >
                        <span class="icon">💡</span>
                        {#if w.fromCache}
                          <span class="cache-badge">cached</span>
                        {/if}
                        {expandedExplanations.has(wKey) ? 'Hide' : 'View'} Explanation
                      </button>
                      {#if expandedExplanations.has(wKey)}
                        <div class="explain-content">
                          <p class="explain-text">{w.explanation}</p>
                          {#if w.fixSuggestion}
                            <div class="fix-suggestion">
                              <strong>How to fix:</strong>
                              <p>{w.fixSuggestion}</p>
                            </div>
                          {/if}
                          {#if w.impact}
                            <div class="impact">
                              <strong>Impact:</strong> {w.impact}
                            </div>
                          {/if}
                        </div>
                      {/if}
                    </div>
                  {/if}
                </div>

                <div class="actions">
                  {#if !w.explanation}
                    {@const loading = isWarningLoading(w)}
                    <button
                      class="mini explain-btn"
                      type="button"
                      title="Get LLM explanation"
                      disabled={loading}
                      on:click|stopPropagation={() => dispatch('explain', w)}
                    >
                      {loading ? '...' : 'Explain'}
                    </button>
                  {/if}
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
    color: var(--color-text-tertiary);
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
    border: 1px solid var(--color-border-default);
    background: var(--color-bg-input);
    color: var(--color-text-primary);
    outline: none;
    transition: var(--transition-all);
  }

  .input::placeholder {
    color: var(--color-text-muted);
  }

  .input:hover:not(:disabled):not(:focus) {
    border-color: var(--color-border-strong);
  }

  .input:focus {
    border-color: var(--color-border-focus);
    box-shadow: var(--shadow-focus);
  }

  .clear {
    padding: 8px 10px;
    border-radius: 10px;
    border: 1px solid var(--color-border-default);
    background: var(--color-bg-elevated);
    color: var(--color-text-secondary);
    font-weight: 700;
    cursor: pointer;
    transition: var(--transition-all);
  }

  .clear:hover {
    background: var(--color-bg-hover);
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
    border: 1px solid var(--color-border-default);
    background: var(--color-bg-elevated);
    color: var(--color-text-secondary);
    font-weight: 750;
    cursor: pointer;
    transition: var(--transition-all);
  }

  .chip:hover {
    background: var(--color-bg-hover);
  }

  .chip.active {
    border-color: var(--color-primary-border);
    background: var(--color-primary-muted);
  }

  .checkbox {
    display: flex;
    align-items: center;
    gap: 8px;
    color: var(--color-text-secondary);
    font-weight: 650;
    font-size: 0.9rem;
  }

  .filter-actions {
    display: flex;
    align-items: center;
    gap: 12px;
    flex-wrap: wrap;
  }

  .explain-all-btn {
    padding: 6px 12px;
    border-radius: 10px;
    border: 1px solid rgba(129, 140, 248, 0.4);
    background: rgba(129, 140, 248, 0.15);
    color: rgba(129, 140, 248, 0.95);
    font-weight: 700;
    font-size: 0.85rem;
    cursor: pointer;
    white-space: nowrap;
  }

  .explain-all-btn:hover:not(:disabled) {
    background: rgba(129, 140, 248, 0.25);
    border-color: rgba(129, 140, 248, 0.6);
  }

  .explain-all-btn:disabled {
    opacity: 0.6;
    cursor: not-allowed;
  }

  .groups {
    display: grid;
    gap: 12px;
  }

  .group {
    border-radius: 12px;
    border: 1px solid var(--color-border-default);
    background: var(--color-bg-elevated);
    padding: 10px 12px;
  }

  .group-title {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 8px;
    color: var(--color-text-primary);
    font-weight: 700;
  }

  .phase {
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, 'Liberation Mono', 'Courier New', monospace;
    font-size: 0.95rem;
  }

  .count {
    padding: 2px 8px;
    border-radius: 999px;
    border: 1px solid var(--color-border-default);
    background: var(--color-bg-surface);
    color: var(--color-text-secondary);
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
    border: 1px solid var(--color-border-default);
    background: var(--color-bg-surface);
    display: grid;
    grid-template-columns: 1fr auto;
    gap: 10px;
    padding: 10px;
  }

  .item-content {
    min-width: 0;
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
    border: 1px solid var(--color-border-default);
    background: var(--color-bg-elevated);
    color: var(--color-text-secondary);
    font-weight: 750;
    cursor: pointer;
    white-space: nowrap;
    transition: var(--transition-all);
  }

  .mini:hover {
    background: var(--color-bg-hover);
  }

  .top {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 10px;
  }

  .code {
    font-weight: 800;
    color: var(--color-text-primary);
  }

  .path {
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, 'Liberation Mono', 'Courier New', monospace;
    font-size: 0.85rem;
    color: var(--color-text-tertiary);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    max-width: 60%;
  }

  .msg {
    margin-top: 6px;
    color: var(--color-text-secondary);
    line-height: 1.4;
  }

  /* LLM Explanation styles */
  .explanation {
    margin-top: 10px;
    border-top: 1px solid var(--color-border-subtle);
    padding-top: 10px;
  }

  .explain-toggle {
    display: flex;
    align-items: center;
    gap: 8px;
    cursor: pointer;
    color: rgba(129, 140, 248, 0.9);
    font-weight: 600;
    font-size: 0.9rem;
    background: none;
    border: none;
    padding: 0;
  }

  .explain-toggle:hover {
    color: rgba(129, 140, 248, 1);
  }

  .icon {
    font-size: 1rem;
  }

  .cache-badge {
    font-size: 0.7rem;
    padding: 2px 6px;
    background: rgba(34, 197, 94, 0.2);
    border: 1px solid rgba(34, 197, 94, 0.3);
    border-radius: 4px;
    color: rgba(34, 197, 94, 0.9);
    margin-left: 2px;
  }

  .explain-content {
    margin-top: 10px;
    padding: 12px;
    background: var(--color-bg-surface);
    border-radius: 8px;
    border: 1px solid var(--color-border-subtle);
  }

  .explain-text {
    margin: 0;
    color: var(--color-text-secondary);
    line-height: 1.5;
  }

  .fix-suggestion {
    margin-top: 10px;
    padding-top: 10px;
    border-top: 1px solid var(--color-border-subtle);
  }

  .fix-suggestion strong {
    color: rgba(251, 191, 36, 0.9);
    font-weight: 700;
  }

  .fix-suggestion p {
    margin: 6px 0 0;
    color: var(--color-text-secondary);
    line-height: 1.4;
  }

  .impact {
    margin-top: 10px;
    padding-top: 10px;
    border-top: 1px solid var(--color-border-subtle);
    color: var(--color-text-secondary);
  }

  .impact strong {
    color: rgba(239, 68, 68, 0.9);
    font-weight: 700;
  }

  .explain-btn {
    border-color: rgba(129, 140, 248, 0.3);
    color: rgba(129, 140, 248, 0.9);
  }

  .explain-btn:hover {
    background: rgba(129, 140, 248, 0.1);
    border-color: rgba(129, 140, 248, 0.5);
  }

  .explain-btn:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
</style>
