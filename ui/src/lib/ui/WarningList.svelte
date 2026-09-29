<script lang="ts">
  /**
   * WarningList — parse warnings grouped by phase, with a filter, LLM
   * explanations and per-warning actions. Built on the primitives
   * (`.loom/37`, `.loom/42` E-5).
   *
   * `onAcceptFix` (optional): when set, a warning that carries a
   * `fixSuggestion` and a `diagnosticId` gets an "Accept fix" button that calls
   * it with that id. The page owns the mutation (`acceptDiagnosticFix`).
   */
  import type { WarningGroup, WarningLike } from '$lib/domain/warnings';
  import { browser } from '$app/environment';
  import { createEventDispatcher } from 'svelte';
  import { SvelteSet } from 'svelte/reactivity';
  import { Badge, Button, Input } from '$lib/ui/primitives';

  export let groups: readonly WarningGroup[];
  export let selectedPath: string | null = null;
  export let enableControls = true;
  /** Set of warning codes currently being explained */
  export let explainLoadingCodes: SvelteSet<string> = new SvelteSet();
  /** Accepts a diagnostic's fix suggestion; renders "Accept fix" when set. */
  export let onAcceptFix: ((diagnosticId: string) => void) | undefined = undefined;

  const dispatch = createEventDispatcher<{
    select: WarningLike;
    inspect: WarningLike;
    explain: WarningLike;
    explainAll: void;
    resolve: WarningLike;
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

  /** Check if any explain operation is in progress */
  $: anyLoading = explainLoadingCodes.size > 0;

  /** Check if a warning can be resolved via AI */
  function canResolve(w: WarningLike): boolean {
    return ['W042', 'E099'].includes(w.code) && !!w.explanation;
  }

  /** The fix can be accepted: the page wired the handler and the server named the diagnostic. */
  function canAcceptFix(w: WarningLike): boolean {
    return !!onAcceptFix && !!w.fixSuggestion && !!w.diagnosticId && !w.fixAccepted;
  }

  function acceptFix(w: WarningLike) {
    if (w.diagnosticId) onAcceptFix?.(w.diagnosticId);
  }

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
        <label class="sr-only" for="warning-filter">Filter warnings</label>
        <div class="search-input">
          <Input
            id="warning-filter"
            bind:value={query}
            placeholder="Filter warnings by code, message, phase, path…"
          />
        </div>
        {#if query.trim()}
          <Button variant="ghost" onclick={() => (query = '')}>Clear</Button>
        {/if}
        <span class="count text-mono">{shown}/{total}</span>
      </div>

      <div class="filters">
        <div class="phase-row" role="group" aria-label="Phase">
          <Button
            variant={phase === 'all' ? 'secondary' : 'ghost'}
            aria-pressed={phase === 'all'}
            onclick={() => (phase = 'all')}
          >
            all
          </Button>
          {#each phases as p (p)}
            <Button
              variant={phase === p ? 'secondary' : 'ghost'}
              aria-pressed={phase === p}
              onclick={() => (phase = p)}
            >
              {p}
            </Button>
          {/each}
        </div>

        <div class="filter-actions">
          <label class="checkbox">
            <input type="checkbox" bind:checked={onlyWithPath} />
            Has path
          </label>

          {#if unexplainedCount > 0}
            <Button disabled={anyLoading} onclick={() => dispatch('explainAll')}>
              {#if anyLoading}
                Explaining...
              {:else}
                Explain All ({unexplainedCount})
              {/if}
            </Button>
          {/if}
        </div>
      </div>
    </div>
  {/if}

  <div class="groups">
    {#each filteredGroups as g (g.phase)}
      <section class="group" aria-label={`${g.phase} warnings`}>
        <div class="group-title">
          <span class="phase">{g.phase}</span>
          <Badge mono>{g.items.length}</Badge>
        </div>
        <ul class="list">
          {#each g.items as w, idx (w.phase + ':' + w.code + ':' + idx)}
            {@const wKey = warningKey(w, idx)}
            <li class="li">
              <div class="item" data-selected={selectedPath !== null && w.path === selectedPath}>
                <div class="item-content">
                  <button class="main" on:click={() => dispatch('select', w)} type="button">
                    <span class="top">
                      <span class="code">{w.code}</span>
                      {#if w.path}
                        <span class="path" title={w.path}>{w.path}</span>
                      {/if}
                    </span>
                    <span class="msg">{w.message}</span>
                  </button>

                  {#if w.explanation}
                    <div class="explanation">
                      <button
                        class="explain-toggle"
                        type="button"
                        aria-expanded={expandedExplanations.has(wKey)}
                        on:click|stopPropagation={() => toggleExplanation(wKey)}
                      >
                        {#if w.fromCache}
                          <Badge>cached</Badge>
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
                  {:else if w.fixSuggestion}
                    <div class="fix-suggestion">
                      <strong>Suggested fix:</strong>
                      <p>{w.fixSuggestion}</p>
                    </div>
                  {/if}

                  {#if w.fixSuggestion && w.fixAccepted}
                    <Badge tone="success" dot data-testid="warning-fix-accepted">fix accepted</Badge>
                  {/if}
                </div>

                <div class="actions">
                  {#if canAcceptFix(w)}
                    <Button
                      variant="primary"
                      title="Accept the suggested fix for this diagnostic"
                      data-testid="warning-accept-fix"
                      onclick={() => acceptFix(w)}
                    >
                      Accept fix
                    </Button>
                  {/if}
                  {#if !w.explanation}
                    {@const loading = isWarningLoading(w)}
                    <Button
                      variant="ghost"
                      title="Get LLM explanation"
                      disabled={loading}
                      onclick={() => dispatch('explain', w)}
                    >
                      {loading ? '...' : 'Explain'}
                    </Button>
                  {/if}
                  {#if canResolve(w)}
                    <Button variant="ghost" title="Resolve mapping with AI" onclick={() => dispatch('resolve', w)}>
                      Resolve
                    </Button>
                  {/if}
                  {#if w.path}
                    <Button variant="ghost" title="Copy path" onclick={() => copyText(w.path ?? '')}>Copy</Button>
                    <Button variant="ghost" title="Open inspector" onclick={() => dispatch('inspect', w)}>
                      Inspect
                    </Button>
                  {/if}
                </div>
              </div>
            </li>
          {/each}
        </ul>
      </section>
    {/each}
  </div>
{/if}

<style>
  .empty {
    color: var(--color-text-tertiary);
    font-size: var(--text-ui);
  }

  .controls {
    display: grid;
    gap: var(--space-2);
    margin-bottom: var(--space-3);
  }

  .search {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }

  .search-input {
    flex: 1 1 auto;
    min-width: 0;
  }

  .count {
    color: var(--color-text-tertiary);
    font-size: var(--text-xs);
  }

  .filters {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-2);
    flex-wrap: wrap;
  }

  .phase-row {
    display: flex;
    flex-wrap: wrap;
    gap: 2px;
  }

  .filter-actions {
    display: flex;
    align-items: center;
    gap: var(--space-3);
  }

  .checkbox {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
    font-size: var(--text-ui);
    color: var(--color-text-secondary);
    cursor: pointer;
  }

  .groups {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
  }

  .group-title {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    margin-bottom: var(--space-1);
  }

  .phase {
    font-size: var(--text-label);
    font-weight: var(--font-semibold);
    letter-spacing: var(--tracking-label);
    text-transform: uppercase;
    color: var(--color-text-tertiary);
  }

  .list {
    display: flex;
    flex-direction: column;
    margin: 0;
    padding: 0;
    list-style: none;
    border: 1px solid var(--color-border-subtle);
    border-radius: var(--radius-sm);
  }

  .li + .li {
    border-top: 1px solid var(--color-border-subtle);
  }

  .item {
    display: flex;
    align-items: flex-start;
    gap: var(--space-2);
    padding: var(--space-2);
  }

  .item[data-selected='true'] {
    background: var(--color-primary-muted);
    box-shadow: inset 2px 0 0 var(--color-primary);
  }

  .item-content {
    display: flex;
    flex: 1 1 auto;
    flex-direction: column;
    gap: var(--space-1);
    min-width: 0;
  }

  .main {
    display: flex;
    flex-direction: column;
    gap: 2px;
    padding: 0;
    border: 0;
    background: none;
    color: inherit;
    font: inherit;
    text-align: left;
    cursor: pointer;
  }

  .main:focus-visible,
  .explain-toggle:focus-visible {
    outline: none;
    box-shadow: var(--shadow-focus);
    border-radius: var(--radius-sm);
  }

  .top {
    display: flex;
    align-items: baseline;
    gap: var(--space-2);
    min-width: 0;
  }

  .code {
    font-family: var(--font-mono);
    font-size: var(--text-ui);
    font-weight: var(--font-semibold);
    color: var(--color-text-primary);
  }

  .path {
    overflow: hidden;
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    color: var(--color-text-tertiary);
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .msg {
    font-size: var(--text-ui);
    line-height: var(--leading-snug);
    color: var(--color-text-secondary);
  }

  .actions {
    display: flex;
    flex: 0 0 auto;
    flex-wrap: wrap;
    justify-content: flex-end;
    gap: 2px;
  }

  .explanation {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
  }

  .explain-toggle {
    display: inline-flex;
    align-items: center;
    align-self: flex-start;
    gap: var(--space-1);
    padding: 0;
    border: 0;
    background: none;
    color: var(--color-accent-text);
    font: inherit;
    font-size: var(--text-xs);
    cursor: pointer;
  }

  .explain-content {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    padding: var(--space-2);
    border-left: 2px solid var(--color-border-default);
  }

  .explain-text,
  .fix-suggestion p {
    margin: 0;
    font-size: var(--text-ui);
    line-height: var(--leading-snug);
    color: var(--color-text-secondary);
  }

  .fix-suggestion,
  .impact {
    font-size: var(--text-ui);
    color: var(--color-text-secondary);
  }

  .fix-suggestion strong,
  .impact strong {
    font-weight: var(--font-semibold);
    color: var(--color-text-primary);
  }
</style>
