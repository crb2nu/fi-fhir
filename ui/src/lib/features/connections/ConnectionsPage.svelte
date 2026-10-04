<!--
  Connections: the source and destination connections this deployment's
  catalog holds, and the engine properties this replica composed at startup.

  Honest states, in the spec's precedence (.loom/38 C-1):
    - control plane not configured → the catalog tabs say so and name
      FI_FHIR_OPERATOR_CONTROL_PLANE_ENABLED (the Engine tab still reads the
      runtime, which does not depend on the catalog);
    - no connectionsRead → the page names the missing role and issues no query;
    - no connectionsWrite → every form is read only, with one status line.
  The Definitions tab (.loom/42 E-1) pre-flights definitionAuthoring in the
  same precedence (definitionsAccess.ts) and reads `?definition=&revision=`
  as the URL changes.
  Unknown capabilities never block: the page queries and renders any failure
  inline.
-->
<script lang="ts">
  import { onDestroy, onMount, untrack } from 'svelte';
  import { afterNavigate, goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import type { IDEAppRoute } from '$lib/ui/ide/types';
  import ServerOff from '@lucide/svelte/icons/server-off';
  import ShieldAlert from '@lucide/svelte/icons/shield-alert';
  import { accessCapabilities } from '$lib/graphql/accessCapabilities';
  import { clearDraftState, setDraftState } from '$lib/ui/ide/ideStore';
  import { EmptyState, Tabs, Toolbar, type TabItem } from '$lib/ui/primitives';
  import ConnectionCatalog from './ConnectionCatalog.svelte';
  import DefinitionsView from './DefinitionsView.svelte';
  import { definitionsPreflight } from './definitionsAccess';
  import { connectionLocation, connectionSelection, definitionLocation } from './connectionLocation';
  import { fetchConnection, type ConnectionRow } from './connectionsApi';
  import { describeConnectionFailure } from './connectionsErrors';
  import EngineRuntimePanel from './EngineRuntimePanel.svelte';
  import { catalogPreflight, readPreflight, writeBlockedRoles } from './connectionsAccess';
  import { connectionsIntent, takeConnectionsIntent, type ConnectionsView } from './connectionsIntent';

  const views: TabItem[] = [
    { id: 'sources', label: 'Sources', controls: 'connections-view-sources', testid: 'connections-tab-sources' },
    {
      id: 'destinations',
      label: 'Destinations',
      controls: 'connections-view-destinations',
      testid: 'connections-tab-destinations'
    },
    {
      id: 'definitions',
      label: 'Definitions',
      controls: 'connections-view-definitions',
      testid: 'connections-tab-definitions'
    },
    { id: 'engine', label: 'Engine', controls: 'connections-view-engine', testid: 'connections-tab-engine' }
  ];

  const ROLE_GRANT_LOCATIONS = [
    { variable: 'FI_FHIR_GRAPHQL_ROLES', scope: 'the static bearer credential and the trusted network' },
    { variable: 'FI_FHIR_GRAPHQL_ACCESS_PRINCIPALS', scope: 'each Cloudflare Access email' }
  ];

  let view = $state<ConnectionsView>('sources');
  // A view mounts on first visit and stays mounted, so unsaved edits survive tab switches.
  let visited = $state<Record<ConnectionsView, boolean>>({
    sources: true,
    destinations: false,
    definitions: false,
    engine: false
  });
  let definitionTarget = $state<{ definitionId: string; revisionId: string } | null>(null);
  type ConnectionTarget = { row: ConnectionRow; request: number; identity: string };
  let connectionTargets = $state<{ source: ConnectionTarget | null; destination: ConnectionTarget | null }>({ source: null, destination: null });
  let viewLocations = $state<Record<ConnectionsView, string>>({ sources: '/connections', destinations: '/connections', definitions: '/connections', engine: '/connections' });
  let selectionReset = $state(0);
  let targetNotice = $state<{ loading: boolean; message: string } | null>(null);
  let selectorSearch: string | null = null;
  let selectorSeq = 0;
  let disposed = false;
  let newRequest = $state<Record<'source' | 'destination', number>>({ source: 0, destination: 0 });
  let editorDirty = $state({ sources: false, destinations: false, definitions: false });
  $effect(() => {
    setDraftState('/connections', 'connections-editors', Object.values(editorDirty).some(Boolean));
  });
  onDestroy(() => clearDraftState('/connections', 'connections-editors'));

  const catalog = $derived(catalogPreflight($accessCapabilities));
  const read = $derived(readPreflight($accessCapabilities));
  const writeBlocked = $derived(writeBlockedRoles($accessCapabilities));
  // Every read on the page needs the role; only "not configured" outranks it on the catalog tabs.
  const pagePreflight = $derived(read !== null && catalog?.reason !== 'not-configured' ? read : null);
  const definitions = $derived(definitionsPreflight($accessCapabilities));

  // Catalog data and drafts belong to the identity that read them. Only an identity
  // change remounts those editors; record and query changes keep every buffer.
  const identity = $derived($accessCapabilities.state === 'known'
    ? JSON.stringify([$accessCapabilities.authVia, $accessCapabilities.principal]) : 'unknown');
  const accessKey = $derived(JSON.stringify([identity, catalog, definitions, writeBlocked]));
  let previousAccessKey: string | undefined;
  $effect(() => {
    const key = accessKey;
    untrack(() => {
      if (previousAccessKey === key) return;
      const changed = previousAccessKey !== undefined;
      previousAccessKey = key;
      if (changed) {
        connectionTargets = { source: null, destination: null };
        void readSelector(new URL(window.location.href), true);
      }
    });
  });

  function activate(target: ConnectionsView): void {
    view = target;
    visited[target] = true;
  }

  async function readSelector(url: URL, force = false): Promise<void> {
    if (!force && selectorSearch === url.search) return;
    selectorSearch = url.search;
    const seq = ++selectorSeq;
    const requestIdentity = identity;
    targetNotice = null;
    const target = connectionSelection(url.search);
    if (!target) {
      connectionTargets = { source: null, destination: null };
      definitionTarget = null;
      selectionReset += 1;
      viewLocations = { sources: '/connections', destinations: '/connections', definitions: '/connections', engine: '/connections' };
      activate('sources');
      return;
    }
    if (target.kind === 'invalid') {
      targetNotice = { loading: false, message: target.message };
      return;
    }
    if (target.kind === 'definition') {
      definitionTarget = { definitionId: target.definitionId, revisionId: target.revisionId };
      viewLocations.definitions = definitionLocation(target.definitionId, target.revisionId);
      activate('definitions');
      return;
    }
    if (catalog) {
      activate('sources');
      return;
    }
    targetNotice = { loading: true, message: `Loading connection ${target.id}…` };
    try {
      const row = await fetchConnection(target.id);
      if (disposed || seq !== selectorSeq || requestIdentity !== identity) return;
      if (!row) {
        targetNotice = { loading: false, message: `Connection ${target.id} is unavailable in this catalog.` };
        return;
      }
      const direction = row.direction === 'SOURCE' ? 'source' : 'destination';
      const next = direction === 'source' ? 'sources' : 'destinations';
      connectionTargets[direction] = { row, request: seq, identity: requestIdentity };
      viewLocations[next] = connectionLocation(row.id);
      activate(next);
      targetNotice = null;
    } catch (err) {
      if (disposed || seq !== selectorSeq || requestIdentity !== identity) return;
      targetNotice = { loading: false, message: describeConnectionFailure(err).message };
    }
  }

  onMount(() => { void readSelector(new URL(window.location.href)); });
  afterNavigate(({ to }) => {
    if (to?.url.pathname === resolve('/connections')) void readSelector(to.url);
  });
  onDestroy(() => { disposed = true; selectorSeq += 1; });

  function writeLocation(path: string, replace = false): void {
    if (typeof window === 'undefined') return;
    selectorSeq += 1;
    targetNotice = null;
    const search = new URL(path, window.location.origin).search;
    selectorSearch = search;
    if (window.location.search === search) return;
    void goto(resolve(path as IDEAppRoute), { replaceState: replace, noScroll: true, keepFocus: true });
  }

  function selected(next: ConnectionsView, path: string, replace = false): void {
    viewLocations[next] = path;
    if (view === next) writeLocation(path, replace);
  }

  function show(next: string): void {
    const target = next as ConnectionsView;
    activate(target);
    writeLocation(viewLocations[target]);
  }

  const unsubscribe = connectionsIntent.subscribe((intent) => {
    if (!intent) return;
    takeConnectionsIntent();
    show(intent.view);
    if (intent.openNew && (intent.view === 'sources' || intent.view === 'destinations')) {
      const direction = intent.view === 'sources' ? 'source' : 'destination';
      newRequest[direction] += 1;
    }
  });
  onDestroy(unsubscribe);

  const LIST_SEPARATOR = ', ';
  const CLAUSE_SEPARATOR = '; ';
</script>

{#snippet notConfigured(keys: readonly string[])}
  <div class="preflight-wrap">
    <EmptyState
      icon={ServerOff}
      align="start"
      class="preflight"
      data-testid="connections-preflight"
      data-reason="not-configured"
    >
      The connection catalog is not configured on this deployment. It needs the PostgreSQL submission store
      (<code>{keys[0]}</code>) or <code>{keys[1]}</code>.
    </EmptyState>
  </div>
{/snippet}

<div class="connections">
  {#if pagePreflight}
    <Toolbar title="Connections" />
    <div class="preflight-wrap">
      <EmptyState
        icon={ShieldAlert}
        align="start"
        class="preflight"
        data-testid="connections-preflight"
        data-reason="missing-role"
        data-missing-roles={pagePreflight.missingRoles.join(',')}
      >
        <span class="line">
          This identity{#if pagePreflight.principal}&nbsp;(<code>{pagePreflight.principal}</code>){/if} does not hold
          {#each pagePreflight.missingRoles as role, index (role)}{#if index > 0}{LIST_SEPARATOR}{/if}<code>{role}</code>{/each},
          so connections and the engine runtime were not queried.
        </span>
        <span class="line muted">
          Roles are granted in the API's environment:
          {#each ROLE_GRANT_LOCATIONS as location, index (location.variable)}{#if index > 0}{CLAUSE_SEPARATOR}{/if}<code
              >{location.variable}</code
            >&nbsp;— {location.scope}{/each}.
        </span>
      </EmptyState>
    </div>
  {:else}
    <Toolbar title="Connections">
      {#snippet tabs()}
        <Tabs label="Connection views" items={views} value={view} onchange={show} />
      {/snippet}
    </Toolbar>

    {#if targetNotice}
      <div class="preflight-wrap" data-testid="connection-target-state">
        <EmptyState
          message={targetNotice.message}
          aria-busy={targetNotice.loading}
          role={targetNotice.loading ? 'status' : 'alert'}
          actionLabel={targetNotice.loading ? undefined : 'Retry'}
          onaction={() => readSelector(new URL(window.location.href), true)}
        />
      </div>
    {/if}

    {#if visited.sources}
      <div class="view" id="connections-view-sources" role="tabpanel" aria-label="Sources" hidden={view !== 'sources' || targetNotice !== null}>
        {#if catalog?.reason === 'not-configured'}
          {@render notConfigured(catalog.keys)}
        {:else}
          {#key identity}
          <ConnectionCatalog
            direction="source"
            target={connectionTargets.source?.identity === identity ? connectionTargets.source : null}
            resetRequest={selectionReset}
            onselectionchange={(row, replace) => selected('sources', row ? connectionLocation(row.id) : '/connections', replace)}
            {writeBlocked}
            newRequest={newRequest.source}
            ondirtychange={(dirty) => (editorDirty.sources = dirty)}
          />
          {/key}
        {/if}
      </div>
    {/if}
    {#if visited.destinations}
      <div
        class="view"
        id="connections-view-destinations"
        role="tabpanel"
        aria-label="Destinations"
        hidden={view !== 'destinations' || targetNotice !== null}
      >
        {#if catalog?.reason === 'not-configured'}
          {@render notConfigured(catalog.keys)}
        {:else}
          {#key identity}
          <ConnectionCatalog
            direction="destination"
            target={connectionTargets.destination?.identity === identity ? connectionTargets.destination : null}
            resetRequest={selectionReset}
            onselectionchange={(row, replace) => selected('destinations', row ? connectionLocation(row.id) : '/connections', replace)}
            {writeBlocked}
            newRequest={newRequest.destination}
            ondirtychange={(dirty) => (editorDirty.destinations = dirty)}
          />
          {/key}
        {/if}
      </div>
    {/if}
    {#if visited.definitions}
      <div
        class="view"
        id="connections-view-definitions"
        role="tabpanel"
        aria-label="Definitions"
        hidden={view !== 'definitions' || targetNotice !== null}
      >
        {#if definitions?.reason === 'not-configured'}
          <div class="preflight-wrap">
            <EmptyState
              icon={ServerOff}
              align="start"
              class="preflight"
              data-testid="definitions-preflight"
              data-reason="not-configured"
            >
              Definition authoring is not configured on this deployment. It needs the lifecycle and connection catalogs
              (the PostgreSQL submission store, <code>{definitions.keys[0]}</code>, or <code>{definitions.keys[1]}</code>)
              and the static integration registry.
            </EmptyState>
          </div>
        {:else if definitions?.reason === 'missing-role'}
          <div class="preflight-wrap">
            <EmptyState
              icon={ShieldAlert}
              align="start"
              class="preflight"
              data-testid="definitions-preflight"
              data-reason="missing-role"
              data-missing-roles={definitions.missingRoles.join(',')}
            >
              Definitions need
              {#each definitions.missingRoles as role, index (role)}{#if index > 0}{LIST_SEPARATOR}{/if}<code>{role}</code>{/each},
              which this identity does not hold, so they were not queried.
            </EmptyState>
          </div>
        {:else}
          {#if definitions?.reason === 'read-only'}
            <p
              class="read-only"
              role="status"
              data-testid="definitions-preflight"
              data-reason="read-only"
              data-missing-roles={definitions.missingRoles.join(',')}
            >
              Read only: this identity does not hold
              {#each definitions.missingRoles as role, index (role)}{#if index > 0}{LIST_SEPARATOR}{/if}<code>{role}</code>{/each},
              so definitions cannot be created, validated, approved or published here.
            </p>
          {/if}
          {#key identity}
          <DefinitionsView
            writeBlocked={definitions?.reason === 'read-only' ? definitions.missingRoles : null}
            target={definitionTarget}
            resetRequest={selectionReset}
            onselectionchange={(record, replace) => selected('definitions', record ? definitionLocation(record.definitionId, record.revisionId) : '/connections', replace)}
            ondirtychange={(dirty) => (editorDirty.definitions = dirty)}
          />
          {/key}
        {/if}
      </div>
    {/if}
    {#if visited.engine}
      <div class="view" id="connections-view-engine" role="tabpanel" aria-label="Engine" hidden={view !== 'engine' || targetNotice !== null}>
        {#if read}
          <div class="preflight-wrap">
            <EmptyState
              icon={ShieldAlert}
              align="start"
              class="preflight"
              data-testid="connections-preflight"
              data-reason="missing-role"
              data-missing-roles={read.missingRoles.join(',')}
            >
              The engine runtime needs
              {#each read.missingRoles as role, index (role)}{#if index > 0}{LIST_SEPARATOR}{/if}<code>{role}</code>{/each},
              which this identity does not hold, so it was not queried.
            </EmptyState>
          </div>
        {:else}
          <EngineRuntimePanel />
        {/if}
      </div>
    {/if}
  {/if}
</div>

<style>
  .connections {
    display: flex;
    flex-direction: column;
    height: 100%;
    min-height: 0;
  }

  .view {
    display: flex;
    flex-direction: column;
    flex: 1 1 auto;
    min-height: 0;
  }

  .view[hidden] {
    display: none;
  }

  .preflight-wrap {
    padding: var(--space-3);
  }

  .preflight-wrap :global(.preflight) {
    padding: var(--space-3);
    border: 1px solid var(--color-warning-border);
    border-radius: var(--radius-sm);
    background: var(--color-bg-elevated);
  }

  .preflight-wrap :global(.preflight .ui-empty-icon) {
    color: var(--color-warning-text);
  }

  .line {
    display: block;
  }

  .read-only {
    margin: 0;
    padding: var(--space-2) var(--space-3);
    border-bottom: 1px solid var(--color-border-subtle);
    font-size: var(--text-xs);
    color: var(--color-text-secondary);
  }

  .line + .line {
    margin-top: var(--space-1);
  }

  .muted {
    font-size: var(--text-xs);
    color: var(--color-text-tertiary);
  }

  code {
    font-family: var(--font-mono);
    font-size: var(--text-mono);
    color: var(--color-text-primary);
    overflow-wrap: anywhere;
  }
</style>
