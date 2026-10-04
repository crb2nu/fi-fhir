<script lang="ts">
  import { resolve } from '$app/paths';
  import ChevronRight from '@lucide/svelte/icons/chevron-right';
  import PanelLeftClose from '@lucide/svelte/icons/panel-left-close';
  import RefreshCw from '@lucide/svelte/icons/refresh-cw';
  import Search from '@lucide/svelte/icons/search';
  import X from '@lucide/svelte/icons/x';
  import { Icon, IconButton } from '$lib/ui/primitives';
  import { accessCapabilities, integrationSessionsCapability } from '$lib/graphql/accessCapabilities';
  import { catalogPreflight } from '$lib/features/connections/connectionsAccess';
  import { fetchConnections, type ConnectionRow } from '$lib/features/connections/connectionsApi';
  import { describeConnectionFailure } from '$lib/features/connections/connectionsErrors';
  import { connectionLocation, connectionSelection } from '$lib/features/connections/connectionLocation';
  import { isIntegrationSessionBuildEnabled } from '$lib/features/integration-session/api';
  import { fetchRecentSessions, type RecentSession } from '$lib/features/dashboard/dashboardApi';
  import { ideState } from '../ideStore';
  import type { IDEView, IDEAppRoute } from '../types';
  import { VIEW_ICONS } from '../viewIcons';

  let { drawer = false, onclose, onnavigate }: {
    drawer?: boolean;
    onclose: () => void;
    onnavigate: (path: string) => void;
  } = $props();

  const groups: { id: string; label: string; links: { view: IDEView; label: string; path: IDEAppRoute }[] }[] = [
    { id: 'build', label: 'Build', links: [
      { view: 'hl7', label: 'HL7 / Intake', path: '/hl7' },
      { view: 'profiles', label: 'Profiles', path: '/profiles' },
      { view: 'terminology', label: 'Terminology', path: '/terminology' },
      { view: 'workflows', label: 'Workflows', path: '/workflows' }
    ] },
    { id: 'operate', label: 'Operate', links: [
      { view: 'connections', label: 'Connections', path: '/connections' },
      { view: 'operator', label: 'Operator', path: '/operator' },
      { view: 'events', label: 'Verification', path: '/events' }
    ] }
  ];
  const sectionsKey = 'fi-fhir-explorer-collapsed';
  function loadCollapsed(): string[] {
    try {
      const value: unknown = JSON.parse(localStorage.getItem(sectionsKey) ?? '[]');
      return Array.isArray(value) ? value.filter((id) => ['build', 'operate', 'sources', 'destinations', 'sessions'].includes(id)) : [];
    } catch { return []; }
  }
  let collapsed = $state(loadCollapsed());
  let filter = $state('');
  let filterInput: HTMLInputElement | undefined = $state();
  let sessions = $state<RecentSession[]>([]);
  let loading = $state(false);
  let error = $state(false);
  let refresh = $state(0);
  let connections = $state<ConnectionRow[]>([]);
  let connectionsLoading = $state(false);
  let connectionsError = $state<string | null>(null);
  let connectionsRefresh = $state(0);
  const visibleConnectionLimit = 8;
  const connectionsBlocked = $derived(catalogPreflight($accessCapabilities));
  const sessionsAvailable = $derived(isIntegrationSessionBuildEnabled() && $integrationSessionsCapability === true);
  const query = $derived(filter.trim().toLocaleLowerCase());
  const activeDocument = $derived($ideState.documents.find((doc) => doc.id === $ideState.activeDocumentId));
  const activeSession = $derived(activeDocument?.view === 'hl7'
    ? new URLSearchParams(activeDocument.path?.split('?')[1]).get('session')
    : null);
  const activeConnectionSelection = $derived(activeDocument?.view === 'connections'
    ? connectionSelection(activeDocument.path?.split('?')[1] ?? '')
    : null);
  const activeConnection = $derived(activeConnectionSelection?.kind === 'connection' ? activeConnectionSelection.id : null);
  const matches = (name: string) => name.toLocaleLowerCase().includes(query);
  const filteredGroups = $derived(groups.map((group) => ({ ...group, links: group.links.filter((link) => matches(link.label)) })));
  const filteredSessions = $derived(sessions.filter((session) => matches(session.name) || matches(session.id)));
  const connectionGroups = $derived([
    { id: 'sources', label: 'Sources', direction: 'SOURCE' },
    { id: 'destinations', label: 'Destinations', direction: 'DESTINATION' },
  ].map((group) => {
    const rows = connections.filter((row) => row.direction === group.direction);
    const filtered = rows.filter((row) => matches(row.name) || matches(row.id) || matches(row.kind));
    const visible = filtered.slice(0, visibleConnectionLimit);
    const selected = filtered.find((row) => row.id === activeConnection);
    // Keep the current location visible even when the catalog exceeds the
    // compact list, without increasing the number of displayed rows.
    if (selected && !visible.includes(selected)) visible[visibleConnectionLimit - 1] = selected;
    return { ...group, total: rows.length, filtered, visible };
  }));
  const noMatches = $derived(query && !matches('Home') && filteredGroups.every((group) => !group.links.length)
    && !filteredSessions.length && connectionGroups.every((group) => !group.filtered.length) && !loading && !connectionsLoading);

  $effect(() => {
    void connectionsRefresh;
    // Recheck the identity as well as the preflight result: an allowed new
    // identity must never inherit the previous identity's catalog rows.
    void $accessCapabilities;
    connections = [];
    connectionsError = null;
    connectionsLoading = !connectionsBlocked;
    if (connectionsBlocked) return;
    let current = true;
    void fetchConnections(null, false).then((rows) => {
      if (current) connections = rows.filter((row) => !row.archived)
        .sort((a, b) => a.name.localeCompare(b.name) || a.id.localeCompare(b.id));
    }).catch((failure: unknown) => {
      if (current) connectionsError = describeConnectionFailure(failure).message;
    }).finally(() => {
      if (current) connectionsLoading = false;
    });
    return () => { current = false; };
  });

  $effect(() => {
    void refresh;
    sessions = [];
    error = false;
    loading = sessionsAvailable;
    if (!sessionsAvailable) return;
    let current = true;
    // Reuse Home's recent-session read. The API has no pagination yet; only
    // eight recent rows are displayed. Read on open/refresh, never on a timer.
    void fetchRecentSessions(8).then((rows) => {
      if (current) sessions = rows;
    }).catch(() => {
      if (current) error = true;
    }).finally(() => {
      if (current) loading = false;
    });
    return () => { current = false; };
  });

  function toggleSection(id: string): void {
    collapsed = collapsed.includes(id) ? collapsed.filter((item) => item !== id) : [...collapsed, id];
    try { localStorage.setItem(sectionsKey, JSON.stringify(collapsed)); } catch { /* Storage may be disabled. */ }
  }

  function expanded(id: string): boolean { return !!query || !collapsed.includes(id); }
  function clearFilter(): void {
    filter = '';
    filterInput?.focus();
  }
  function location(view: IDEView, fallback: IDEAppRoute): string {
    return $ideState.documents.find((doc) => doc.view === view)?.path ?? fallback;
  }
  function follow(event: MouseEvent, path: string): void {
    if (event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
    event.preventDefault();
    onnavigate(path);
  }
</script>

<div id="workspace-explorer" class="explorer">
  <header>
    <span class="title">Explorer</span>
    <IconButton icon={drawer ? X : PanelLeftClose} label={drawer ? 'Close explorer' : 'Collapse explorer'} onclick={onclose} />
  </header>
  <div class="filter">
    <Icon icon={Search} size={14} />
    <input aria-label="Filter explorer" placeholder="Filter views, connections, sessions" bind:this={filterInput} bind:value={filter} />
    {#if filter}<IconButton icon={X} label="Clear explorer filter" onclick={clearFilter} />{/if}
  </div>
  <div class="tree">
    <nav aria-label="Workspace views">
      {#if matches('Home')}
        <a class="row home" class:active={$ideState.activeView === 'system'} aria-current={$ideState.activeView === 'system' ? 'page' : undefined} href={resolve('/')} onclick={(event) => follow(event, '/')}>
          <Icon icon={VIEW_ICONS.system} size={15} /><span>Home</span>
        </a>
      {/if}
      {#each filteredGroups as group (group.id)}
        {#if group.links.length}
          <button class="section" aria-expanded={expanded(group.id)} aria-controls={'explorer-' + group.id} onclick={() => toggleSection(group.id)} disabled={!!query}>
            <span class:expanded={expanded(group.id)} class="chevron"><Icon icon={ChevronRight} size={12} /></span>
            {group.label}
          </button>
          {#if expanded(group.id)}
            <div id={'explorer-' + group.id}>
              {#each group.links as link (link.view)}
                {@const path = location(link.view, link.path)}
                <a class="row" class:active={$ideState.activeView === link.view} aria-current={$ideState.activeView === link.view ? 'page' : undefined} href={resolve(path as IDEAppRoute)} onclick={(event) => follow(event, path)}>
                  <Icon icon={VIEW_ICONS[link.view]} size={15} />
                  <span>{link.label}</span>
                  {#if $ideState.documents.some((doc) => doc.view === link.view && doc.dirty)}<span class="dirty" aria-label="Unsaved changes">●</span>{/if}
                </a>
              {/each}
            </div>
          {/if}
        {/if}
      {/each}
    </nav>
    <section aria-label="Connection catalog" class="catalog">
      <div class="section-header">
        <span class="catalog-label">Connections</span>
        {#if !connectionsBlocked}<IconButton icon={RefreshCw} label="Refresh connections" disabled={connectionsLoading} onclick={() => connectionsRefresh++} />{/if}
      </div>
      <div aria-busy={connectionsLoading}>
        {#if connectionsBlocked?.reason === 'not-configured'}
          <p class="note">The connection catalog is not configured on this deployment.</p>
        {:else if connectionsBlocked?.reason === 'missing-role'}
          <p class="note">Reading connections requires <code>{connectionsBlocked.missingRoles.join(', ')}</code>.</p>
        {:else if connectionsLoading}
          <p class="note" role="status">Loading connections…</p>
        {:else if connectionsError}
          <div class="note" role="status">
            <p>Could not load connections. {connectionsError}</p>
            <button class="text-action" onclick={() => connectionsRefresh++}>Retry connections</button>
          </div>
        {:else}
          {#each connectionGroups as group (group.id)}
            {#if !query || group.filtered.length}
              <button class="section" aria-expanded={expanded(group.id)} aria-controls={'explorer-' + group.id} onclick={() => toggleSection(group.id)} disabled={!!query}>
                <span class:expanded={expanded(group.id)} class="chevron"><Icon icon={ChevronRight} size={12} /></span>
                {group.label} <span class="count">{query ? group.filtered.length : group.total}</span>
              </button>
              {#if expanded(group.id)}
                <nav id={'explorer-' + group.id} aria-label={(group.id === 'sources' ? 'Source' : 'Destination') + ' connections'}>
                  {#if !group.total}
                    <p class="note">No {group.id === 'sources' ? 'source' : 'destination'} connections yet.</p>
                  {:else}
                    {#each group.visible as connection (connection.id)}
                      {@const path = connectionLocation(connection.id)}
                      <a class="row connection" class:active={activeConnection === connection.id} aria-current={activeConnection === connection.id ? 'page' : undefined} href={resolve(path as IDEAppRoute)} title={connection.name + ' · ' + connection.id} onclick={(event) => follow(event, path)}>
                        <Icon icon={VIEW_ICONS.connections} size={14} /><span>{connection.name || connection.id}</span>
                      </a>
                    {/each}
                    {#if group.filtered.length > visibleConnectionLimit}
                      <p class="note">Showing {visibleConnectionLimit} of {group.filtered.length}{query ? ' matches' : ''}. Filter to find a connection or <a href={resolve('/connections')} onclick={(event) => follow(event, '/connections')}>open Connections</a>.</p>
                    {/if}
                  {/if}
                </nav>
              {/if}
            {/if}
          {/each}
          {#if connections.length >= 500}<p class="note">Only the first 500 connections are available here. <a href={resolve('/connections')} onclick={(event) => follow(event, '/connections')}>Open Connections</a> to manage the catalog.</p>{/if}
        {/if}
      </div>
    </section>
    <section aria-label="Recent sessions" class="sessions">
      <div class="section-header">
        <button class="section" aria-expanded={expanded('sessions')} aria-controls="explorer-sessions" onclick={() => toggleSection('sessions')} disabled={!!query}>
          <span class:expanded={expanded('sessions')} class="chevron"><Icon icon={ChevronRight} size={12} /></span>
          Recent sessions
          {#if sessions.length}<span class="count">{sessions.length}</span>{/if}
        </button>
        {#if sessionsAvailable}<IconButton icon={RefreshCw} label="Refresh recent sessions" disabled={loading} onclick={() => refresh++} />{/if}
      </div>
      {#if expanded('sessions')}
        <div id="explorer-sessions" aria-busy={loading}>
          {#if !sessionsAvailable}
            <p class="note">Sessions are unavailable for this connection.</p>
          {:else if loading}
            <p class="note" role="status">Loading sessions…</p>
          {:else if error}
            <p class="note" role="status">Could not load sessions. Use Refresh to try again.</p>
          {:else if !sessions.length}
            <p class="note">No saved sessions yet. Preview a message in HL7 / Intake to start one.</p>
          {:else}
            {#each filteredSessions as session (session.id)}
              {@const path = `/hl7?session=${encodeURIComponent(session.id)}`}
              {@const lastRun = session.runs[session.runs.length - 1]}
              <a class="row session" class:active={activeSession === session.id} aria-current={activeSession === session.id ? 'page' : undefined} href={resolve(path as IDEAppRoute)} title={session.name + ' · ' + session.id} onclick={(event) => follow(event, path)}>
                <Icon icon={VIEW_ICONS.hl7} size={14} />
                <span class="session-text"><span class="session-name">{session.name}</span><span class="session-status">{lastRun?.status ?? 'No runs'} · {new Date(session.updatedAt).toLocaleDateString(undefined, { month: 'short', day: 'numeric' })}</span></span>
              </a>
            {/each}
          {/if}
        </div>
      {/if}
    </section>
    {#if noMatches}<p class="note" role="status">No matching views, connections, or recent sessions.</p>{/if}
  </div>
</div>

<style>
  .explorer {
    display: flex;
    flex-direction: column;
    height: 100%;
    min-height: 0;
    font-size: var(--text-ui);
  }

  header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    flex: 0 0 35px;
    padding: 0 var(--space-2) 0 var(--space-3);
    border-bottom: 1px solid var(--color-border-subtle);
  }

  .title {
    font-weight: var(--font-semibold);
  }

  .filter {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    margin: var(--space-3);
    padding-left: var(--space-2);
    min-height: 28px;
    color: var(--color-text-muted);
    border: 1px solid var(--color-border-default);
    border-radius: var(--radius-sm);
    background: var(--color-bg-base);
  }

  .filter:focus-within {
    outline: 2px solid var(--color-focus-ring);
    outline-offset: 1px;
  }

  input {
    min-width: 0;
    width: 100%;
    height: 26px;
    padding: 0;
    border: 0;
    outline: 0;
    background: transparent;
    color: var(--color-text-primary);
    font: inherit;
    font-size: var(--text-xs);
  }

  input::placeholder {
    color: var(--color-text-muted);
  }

  .tree {
    overflow-y: auto;
    padding-bottom: var(--space-3);
  }

  .row {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    min-height: 30px;
    padding: 4px var(--space-3) 4px 26px;
    color: var(--color-text-secondary);
    text-decoration: none;
    border-left: 2px solid transparent;
  }

  .row:hover {
    background: var(--color-bg-hover);
    color: var(--color-text-primary);
  }

  .row.active {
    border-left-color: var(--color-primary);
    background: var(--color-primary-muted);
    color: var(--color-text-primary);
  }

  .row:focus-visible, .section:focus-visible {
    outline: 2px solid var(--color-focus-ring);
    outline-offset: -2px;
  }

  .home {
    padding-left: var(--space-3);
  }

  .section {
    display: flex;
    align-items: center;
    gap: var(--space-1);
    width: 100%;
    min-height: 30px;
    padding: 0 var(--space-2);
    border: 0;
    background: transparent;
    color: var(--color-text-tertiary);
    text-align: left;
    font: inherit;
    font-size: var(--text-label);
    font-weight: var(--font-semibold);
    cursor: pointer;
  }

  .section:hover {
    color: var(--color-text-primary);
  }

  .section:disabled {
    cursor: default;
  }

  .chevron {
    display: inline-flex;
  }

  .chevron.expanded {
    transform: rotate(90deg);
  }

  .count {
    margin-left: auto;
    color: var(--color-text-muted);
    font-variant-numeric: tabular-nums;
  }

  .dirty {
    margin-left: auto;
    font-size: 8px;
  }

  .sessions, .catalog {
    margin-top: var(--space-3);
    padding-top: var(--space-1);
    border-top: 1px solid var(--color-border-subtle);
  }

  .section-header {
    display: flex;
    align-items: center;
    padding-right: var(--space-2);
  }

  .catalog-label {
    flex: 1;
    padding-left: var(--space-3);
    color: var(--color-text-tertiary);
    font-size: var(--text-label);
    font-weight: var(--font-semibold);
  }

  .connection span {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    min-width: 0;
  }

  .note p {
    margin: 0 0 var(--space-1);
  }

  .note a, .text-action {
    color: var(--color-text-link, var(--color-primary));
    text-decoration: underline;
  }

  .text-action {
    border: 0;
    padding: 0;
    background: none;
    font: inherit;
    cursor: pointer;
  }

  .text-action:focus-visible {
    outline: 2px solid var(--color-focus-ring);
    outline-offset: 2px;
  }

  .note code {
    overflow-wrap: anywhere;
  }

  .note {
    margin: 0;
    padding: var(--space-2) var(--space-3) var(--space-2) 28px;
    color: var(--color-text-muted);
    font-size: var(--text-xs);
    line-height: var(--leading-snug);
  }

  .session {
    min-height: 44px;
  }

  .session-text {
    display: flex;
    flex-direction: column;
    min-width: 0;
    gap: 2px;
  }

  .session-name {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .session-status {
    color: var(--color-text-muted);
    font-size: var(--text-label);
  }

</style>
