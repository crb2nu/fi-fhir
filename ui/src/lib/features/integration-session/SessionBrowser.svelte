<script lang="ts">
  import { onDestroy, untrack } from 'svelte';
  import { SvelteMap } from 'svelte/reactivity';
  import { resolve } from '$app/paths';
  import Search from '@lucide/svelte/icons/search';
  import { Badge, Button, Dialog, EmptyState, Input, type BadgeTone } from '$lib/ui/primitives';
  import { accessCapabilities, integrationSessionsCapability } from '$lib/graphql/accessCapabilities';
  import { fetchSessionSummaries, type RecentSession } from '$lib/features/dashboard/dashboardApi';
  import { isIntegrationSessionBuildEnabled } from './api';

  let { open, onclose, onnavigate, currentSessionId = null }: {
    open: boolean;
    onclose: () => void;
    onnavigate: (path: string) => void;
    currentSessionId?: string | null;
  } = $props();

  const PAGE_SIZE = 25;
  const available = $derived(isIntegrationSessionBuildEnabled() && $integrationSessionsCapability === true);
  let search = $state('');
  let includeArchived = $state(false);
  let appliedSearch = $state('');
  let appliedArchived = false;
  let rows = $state<RecentSession[]>([]);
  let loading = $state(false);
  let loaded = $state(false);
  let error = $state<string | null>(null);
  let hasMore = $state(false);
  let nextOffset = $state<number | null>(null);
  let failedAppend = $state(false);
  let generation = 0;

  // Closed dialogs retain their browse state; a different identity cannot inherit it.
  $effect(() => {
    void $accessCapabilities;
    void available;
    generation += 1;
    search = '';
    includeArchived = false;
    appliedSearch = '';
    appliedArchived = false;
    rows = [];
    loading = false;
    loaded = false;
    error = null;
    hasMore = false;
    nextOffset = null;
  });

  $effect(() => {
    if (open && available && !loaded) untrack(() => { void load(false); });
  });
  onDestroy(() => { generation += 1; });

  async function load(append: boolean): Promise<void> {
    if (!available || (append && nextOffset === null)) return;
    const current = ++generation;
    const offset = append ? nextOffset! : 0;
    loaded = true;
    loading = true;
    error = null;
    failedAppend = append;
    if (!append) {
      rows = [];
      hasMore = false;
      nextOffset = null;
    }
    try {
      const page = await fetchSessionSummaries({
        search: appliedSearch, includeArchived: appliedArchived, limit: PAGE_SIZE, offset
      });
      if (current !== generation) return;
      // Metadata can move between offset pages while the browser is open.
      const merged = new SvelteMap((append ? rows : []).map((row) => [row.id, row]));
      for (const row of page.nodes) merged.set(row.id, row);
      rows = [...merged.values()];
      hasMore = page.hasMore;
      nextOffset = page.nextOffset;
    } catch (failure) {
      if (current !== generation) return;
      error = failure instanceof Error ? failure.message : 'Sessions could not be loaded.';
    } finally {
      if (current === generation) loading = false;
    }
  }

  function apply(event?: Event): void {
    event?.preventDefault();
    appliedSearch = search.trim();
    appliedArchived = includeArchived;
    void load(false);
  }

  function follow(event: MouseEvent, id: string): void {
    if (event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
    event.preventDefault();
    // Close this modal before the shell can open its unsaved-draft confirmation.
    onclose();
    onnavigate(`/hl7?session=${encodeURIComponent(id)}`);
  }

  function statusTone(status: string | undefined): BadgeTone {
    return status === 'completed' ? 'success' : status === 'failed' ? 'danger' : status === 'running' ? 'info' : 'neutral';
  }

  function updated(iso: string): string {
    const value = new Date(iso);
    return Number.isNaN(value.getTime()) ? iso : value.toLocaleString();
  }
</script>

<Dialog {open} title="Browse sessions" description="Find saved sessions by name or ID." size="lg" initialFocus="[data-session-search]" {onclose} data-testid="session-browser">
  {#if !available}
    <EmptyState message="Sessions are unavailable for this connection." />
  {:else}
    <form class="search" onsubmit={apply} aria-label="Find sessions">
      <Input aria-label="Search sessions" placeholder="Session name or ID" bind:value={search} maxlength={256} data-session-search />
      <Button type="submit" icon={Search}>Search</Button>
      <label class="archived"><input type="checkbox" checked={includeArchived} onchange={(event) => { includeArchived = event.currentTarget.checked; apply(); }} /> Include archived</label>
    </form>
    <div class="results" aria-busy={loading}>
      {#if loading && rows.length === 0}
        <EmptyState message="Loading sessions…" aria-live="polite" />
      {:else if rows.length === 0 && !error}
        <EmptyState message={appliedSearch ? 'No sessions match this search.' : 'No saved sessions found.'} />
      {/if}
      {#if rows.length > 0}
        <p class="count" role="status">{rows.length} sessions shown · newest updates first</p>
        <ul aria-label="Saved sessions">
          {#each rows as session (session.id)}
            <li data-testid="session-browser-row" data-session-id={session.id}>
              <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- the path is resolved; only the encoded selector is appended -->
              <a href={`${resolve('/hl7')}?session=${encodeURIComponent(session.id)}`} onclick={(event) => follow(event, session.id)} aria-current={session.id === currentSessionId ? 'page' : undefined}>
                <span class="name">{session.name}</span>
                <code class="identifier">{session.id}</code>
                <span class="details">
                  <Badge tone={statusTone(session.latestRun?.status)}>{session.latestRun?.status ?? 'No runs'}</Badge>
                  {#if session.archived}<Badge>Archived</Badge>{/if}
                  {#if session.id === currentSessionId}<Badge tone="info">Current session</Badge>{/if}
                  <span>Updated <time datetime={session.updatedAt}>{updated(session.updatedAt)}</time></span>
                </span>
              </a>
            </li>
          {/each}
        </ul>
      {/if}
      {#if error}
        <div class="failure" role="alert">
          <p>Could not load {failedAppend ? 'more ' : ''}sessions. {error}</p>
          <Button variant="ghost" onclick={() => void load(failedAppend)} disabled={loading}>Retry sessions</Button>
        </div>
      {/if}
      {#if hasMore && nextOffset !== null}
        <div class="more"><Button onclick={() => void load(true)} {loading}>Load more</Button></div>
      {:else if hasMore}
        <p class="limit" role="status">More sessions are available. Refine your search to continue.</p>
      {/if}
    </div>
  {/if}
  {#snippet footer()}<Button variant="ghost" size="md" onclick={onclose}>Close</Button>{/snippet}
</Dialog>

<style>
  .search { display: flex; flex-wrap: wrap; align-items: center; gap: var(--space-2); }
  .search :global(.ui-input) { flex: 1 1 220px; }
  .archived { display: inline-flex; align-items: center; gap: var(--space-1); font-size: var(--text-xs); }
  .results { margin-top: var(--space-3); }
  .count, .limit { margin: var(--space-2) 0; font-size: var(--text-xs); color: var(--color-text-secondary); }
  ul { list-style: none; padding: 0; margin: 0; }
  li + li { border-top: 1px solid var(--color-border-subtle); }
  a { display: flex; flex-direction: column; gap: var(--space-1); padding: var(--space-3) var(--space-2); color: var(--color-text-primary); text-decoration: none; border-radius: var(--radius-sm); }
  a:hover { background: var(--color-bg-hover); }
  a:focus-visible { outline: 2px solid var(--color-focus-ring); outline-offset: -2px; }
  a[aria-current='page'] { background: var(--color-primary-muted); }
  .name { font-size: var(--text-sm); font-weight: var(--font-semibold); overflow-wrap: anywhere; }
  .identifier { font-size: var(--text-xs); color: var(--color-text-secondary); overflow-wrap: anywhere; }
  .details { display: flex; flex-wrap: wrap; align-items: center; gap: var(--space-2); font-size: var(--text-xs); color: var(--color-text-tertiary); }
  .failure { margin-top: var(--space-3); font-size: var(--text-sm); }
  .failure p { margin: 0 0 var(--space-1); }
  .more { display: flex; justify-content: center; margin-top: var(--space-3); }
</style>
