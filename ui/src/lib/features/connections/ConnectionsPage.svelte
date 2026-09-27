<!--
  Connections: the source and destination connections this deployment's
  catalog holds, and the engine properties this replica composed at startup.

  Honest states, in the spec's precedence (.loom/38 C-1):
    - control plane not configured → the catalog tabs say so and name
      FI_FHIR_OPERATOR_CONTROL_PLANE_ENABLED (the Engine tab still reads the
      runtime, which does not depend on the catalog);
    - no connectionsRead → the page names the missing role and issues no query;
    - no connectionsWrite → every form is read only, with one status line.
  Unknown capabilities never block: the page queries and renders any failure
  inline.
-->
<script lang="ts">
  import { onDestroy } from 'svelte';
  import ServerOff from '@lucide/svelte/icons/server-off';
  import ShieldAlert from '@lucide/svelte/icons/shield-alert';
  import { accessCapabilities } from '$lib/graphql/accessCapabilities';
  import { EmptyState, Tabs, Toolbar, type TabItem } from '$lib/ui/primitives';
  import ConnectionCatalog from './ConnectionCatalog.svelte';
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
    { id: 'engine', label: 'Engine', controls: 'connections-view-engine', testid: 'connections-tab-engine' }
  ];

  const ROLE_GRANT_LOCATIONS = [
    { variable: 'FI_FHIR_GRAPHQL_ROLES', scope: 'the static bearer credential and the trusted network' },
    { variable: 'FI_FHIR_GRAPHQL_ACCESS_PRINCIPALS', scope: 'each Cloudflare Access email' }
  ];

  let view = $state<ConnectionsView>('sources');
  // A view mounts on first visit and stays mounted, so unsaved edits survive tab switches.
  let visited = $state<Record<ConnectionsView, boolean>>({ sources: true, destinations: false, engine: false });
  let newRequest = $state<Record<'source' | 'destination', number>>({ source: 0, destination: 0 });

  const catalog = $derived(catalogPreflight($accessCapabilities));
  const read = $derived(readPreflight($accessCapabilities));
  const writeBlocked = $derived(writeBlockedRoles($accessCapabilities));
  // Every read on the page needs the role; only "not configured" outranks it on the catalog tabs.
  const pagePreflight = $derived(read !== null && catalog?.reason !== 'not-configured' ? read : null);

  function show(next: string): void {
    const target = next as ConnectionsView;
    view = target;
    visited[target] = true;
  }

  const unsubscribe = connectionsIntent.subscribe((intent) => {
    if (!intent) return;
    takeConnectionsIntent();
    show(intent.view);
    if (intent.openNew && intent.view !== 'engine') {
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

    {#if visited.sources}
      <div class="view" id="connections-view-sources" role="tabpanel" aria-label="Sources" hidden={view !== 'sources'}>
        {#if catalog?.reason === 'not-configured'}
          {@render notConfigured(catalog.keys)}
        {:else}
          <ConnectionCatalog direction="source" {writeBlocked} newRequest={newRequest.source} />
        {/if}
      </div>
    {/if}
    {#if visited.destinations}
      <div
        class="view"
        id="connections-view-destinations"
        role="tabpanel"
        aria-label="Destinations"
        hidden={view !== 'destinations'}
      >
        {#if catalog?.reason === 'not-configured'}
          {@render notConfigured(catalog.keys)}
        {:else}
          <ConnectionCatalog direction="destination" {writeBlocked} newRequest={newRequest.destination} />
        {/if}
      </div>
    {/if}
    {#if visited.engine}
      <div class="view" id="connections-view-engine" role="tabpanel" aria-label="Engine" hidden={view !== 'engine'}>
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
