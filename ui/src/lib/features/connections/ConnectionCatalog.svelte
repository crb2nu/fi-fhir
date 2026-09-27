<!--
  One direction of the catalog (Sources or Destinations): a filter row, the
  connections table with their honest status, and the details pane of the
  selected one. Unsaved edits live per connection in this view's memory, so
  moving between rows keeps them; a row with unsaved edits says so.
-->
<script lang="ts">
  import { onMount } from 'svelte';
  import ChevronDown from '@lucide/svelte/icons/chevron-down';
  import CircleAlert from '@lucide/svelte/icons/circle-alert';
  import Plus from '@lucide/svelte/icons/plus';
  import Cable from '@lucide/svelte/icons/cable';
  import RefreshCw from '@lucide/svelte/icons/refresh-cw';
  import {
    Badge,
    Button,
    EmptyState,
    Icon,
    IconButton,
    Input,
    Popover,
    Table,
    Td,
    Th,
    Tr
  } from '$lib/ui/primitives';
  import ConnectionDetails from './ConnectionDetails.svelte';
  import { fetchConnections, type ConnectionRow } from './connectionsApi';
  import { writeBlockedReason } from './connectionsAccess';
  import { describeConnectionFailure } from './connectionsErrors';
  import { connectionStatus, connectionStatusText } from './connectionStatus';
  import {
    bufferFromConnection,
    isDirty,
    newBuffer,
    reconcileBuffer,
    specOf,
    type EditBuffer
  } from './editBuffer';
  import { formatMinute, shortHash } from './presentation';
  import { endpointOf, kindSchema, kindsFor, type ConnectionDirectionId, type SpecKind } from './specSchema';

  interface Props {
    direction: ConnectionDirectionId;
    /** Roles this identity lacks to change connections, or null. */
    writeBlocked: string[] | null;
    /** Incremented by the page to open the New menu (command palette). */
    newRequest?: number;
  }

  let { direction, writeBlocked, newRequest = 0 }: Props = $props();

  const NEW_KEY = '\u0000new';
  const noun = $derived(direction === 'source' ? 'source' : 'destination');
  const kinds = $derived(kindsFor(direction));
  const writeReason = $derived(writeBlockedReason(writeBlocked));

  let rows = $state<ConnectionRow[]>([]);
  let loading = $state(true);
  let error = $state<string | null>(null);
  let filter = $state('');
  let showArchived = $state(false);
  let selectedKey = $state<string | null>(null);
  let edits = $state<Record<string, EditBuffer>>({});
  let creating = $state<EditBuffer | null>(null);
  let newMenuOpen = $state(false);
  let loadSeq = 0;

  const selectedRow = $derived(rows.find((row) => row.id === selectedKey) ?? null);

  const visibleRows = $derived.by(() => {
    const needle = filter.trim().toLowerCase();
    if (!needle) return rows;
    return rows.filter((row) =>
      [row.name, row.id, kindSchema(row.kind)?.label ?? row.kind, endpointOf(row.kind, row.spec)]
        .join(' ')
        .toLowerCase()
        .includes(needle)
    );
  });

  const dirtyIds = $derived(
    new Set(
      Object.entries(edits)
        .filter(([, buffer]) => isDirty(buffer))
        .map(([id]) => id)
    )
  );

  async function load(): Promise<void> {
    const seq = ++loadSeq;
    loading = true;
    error = null;
    try {
      const list = await fetchConnections(direction === 'source' ? 'SOURCE' : 'DESTINATION', showArchived);
      if (seq !== loadSeq) return;
      rows = list;
      for (const row of list) {
        if (edits[row.id]) edits[row.id] = reconcileBuffer(edits[row.id], row);
      }
      if (selectedKey !== null && selectedKey !== NEW_KEY && !list.some((row) => row.id === selectedKey)) {
        selectedKey = null;
      }
    } catch (err) {
      if (seq !== loadSeq) return;
      error = describeConnectionFailure(err).message;
      rows = [];
    } finally {
      if (seq === loadSeq) loading = false;
    }
  }

  onMount(() => {
    void load();
  });

  // The command palette's "New … connection" opens this view's New menu.
  let handledRequest = 0;
  $effect(() => {
    if (newRequest > handledRequest) {
      handledRequest = newRequest;
      if (!writeReason) newMenuOpen = true;
    }
  });

  function toggleArchived(): void {
    showArchived = !showArchived;
    void load();
  }

  function select(row: ConnectionRow): void {
    edits[row.id] ??= bufferFromConnection(row);
    selectedKey = row.id;
  }

  function startCreate(kind: SpecKind): void {
    newMenuOpen = false;
    creating = newBuffer(kind);
    selectedKey = NEW_KEY;
  }

  function upsert(row: ConnectionRow): void {
    const others = rows.filter((candidate) => candidate.id !== row.id);
    const hidden = row.archived && !showArchived;
    rows = hidden ? others : [...others, row].sort((left, right) => (left.id < right.id ? -1 : left.id > right.id ? 1 : 0));
    edits[row.id] = bufferFromConnection(row);
    if (hidden) {
      delete edits[row.id];
      if (selectedKey === row.id) selectedKey = null;
    }
  }

  function changed(row: ConnectionRow): void {
    const wasCreating = selectedKey === NEW_KEY;
    upsert(row);
    if (wasCreating) {
      creating = null;
      selectedKey = row.archived && !showArchived ? null : row.id;
    }
  }

  function missing(): void {
    if (selectedKey && selectedKey !== NEW_KEY) {
      rows = rows.filter((row) => row.id !== selectedKey);
      delete edits[selectedKey];
    }
    selectedKey = null;
  }

  function discard(): void {
    if (selectedKey === NEW_KEY) {
      creating = null;
      selectedKey = null;
      return;
    }
    if (selectedRow) edits[selectedRow.id] = bufferFromConnection(selectedRow);
  }

  function revisionText(row: ConnectionRow): string {
    const latest = row.latestRevision;
    return latest ? `r${latest.revisionId} ${shortHash(latest.digest, 8)}` : '—';
  }
</script>

<div class="catalog" data-direction={direction}>
  <div class="list">
    <div class="filters" role="search" aria-label={`Filter ${noun} connections`}>
      <div class="filter">
        <Input
          aria-label={`Filter ${noun} connections`}
          placeholder="Filter by name, ID, kind or endpoint"
          bind:value={filter}
        />
      </div>
      <Button
        variant="ghost"
        aria-pressed={showArchived ? 'true' : 'false'}
        onclick={toggleArchived}
        disabled={loading}
      >
        Show archived
      </Button>
      <span class="spacer"></span>
      <IconButton icon={RefreshCw} label="Refresh connections" loading={loading} onclick={load} />
      <Popover label={`New ${noun} connection`} placement="bottom-end" bind:open={newMenuOpen}>
        {#snippet trigger(props)}
          <Button
            {...props}
            icon={Plus}
            disabled={writeReason !== undefined}
            title={writeReason ?? `New ${noun} connection`}
            data-testid="connections-new"
          >
            New <Icon icon={ChevronDown} size={14} />
          </Button>
        {/snippet}
        <div class="kind-menu">
          {#each kinds as kind (kind.kind)}
            <button type="button" class="kind-option" data-kind={kind.kind} onclick={() => startCreate(kind.kind)}>
              <span class="kind-label">{kind.label}</span>
              <span class="kind-summary">{kind.summary}</span>
            </button>
          {/each}
        </div>
      </Popover>
    </div>

    {#if writeBlocked}
      <p class="read-only" role="status" data-testid="connections-read-only">
        Read only: this identity does not hold
        {#each writeBlocked as role, index (role)}{#if index > 0},&nbsp;{/if}<code>{role}</code>{/each},
        so connections cannot be created, saved, compiled or archived here.
      </p>
    {/if}

    {#if loading && rows.length === 0}
      <EmptyState message="Loading connections" aria-busy="true" aria-live="polite" />
    {:else if error}
      <EmptyState icon={CircleAlert} role="alert" message={error} actionLabel="Retry" onaction={load} />
    {:else if rows.length === 0 && !creating}
      <EmptyState icon={Cable} message={`No ${noun} connections${showArchived ? '' : ' are defined'}.`}>
        {#snippet action()}
          {#if !writeReason}
            <Button variant="primary" icon={Plus} onclick={() => (newMenuOpen = true)}>New {noun} connection</Button>
          {/if}
        {/snippet}
      </EmptyState>
    {:else if visibleRows.length === 0 && !creating}
      <EmptyState message="No connections match this filter." actionLabel="Clear filter" onaction={() => (filter = '')} />
    {:else}
      <Table label={`${noun === 'source' ? 'Source' : 'Destination'} connections`} layout="fixed" class="connections-table" data-testid="connections-table">
        {#snippet head()}
          <tr>
            <Th>Name</Th>
            <Th width="92px">Kind</Th>
            <Th width="120px">ID</Th>
            <Th>Endpoint</Th>
            <Th width="112px">Revision</Th>
            <Th width="168px">Status</Th>
            <Th width="140px">Updated</Th>
          </tr>
        {/snippet}
        {#if creating}
          {@const schema = kindSchema(creating.kind)}
          <Tr selectable selected={selectedKey === NEW_KEY} onselect={() => (selectedKey = NEW_KEY)} data-row="new">
            <Td truncate value={creating.name.trim() || `New ${schema?.label ?? ''} ${noun}`} />
            <Td value={schema?.label ?? creating.kind} />
            <Td mono truncate value={creating.id.trim() || '—'} />
            <Td mono truncate muted value={endpointOf(creating.kind, specOf(creating)) || '—'} />
            <Td mono muted value="—" />
            <Td><Badge tone="warning">Not created</Badge></Td>
            <Td mono muted value="—" />
          </Tr>
        {/if}
        {#each visibleRows as row (row.id)}
          {@const tokens = connectionStatus(row)}
          {@const endpoint = endpointOf(row.kind, row.spec)}
          <Tr
            selectable
            selected={row.id === selectedKey}
            onselect={() => select(row)}
            data-row={row.id}
            data-status={connectionStatusText(tokens)}
          >
            <Td truncate title={row.description || row.name}>
              {row.name}{#if dirtyIds.has(row.id)}<span class="unsaved" title="Unsaved edits">&nbsp;·&nbsp;edited</span>{/if}
            </Td>
            <Td value={kindSchema(row.kind)?.label ?? row.kind} />
            <Td mono truncate value={row.id} />
            <Td mono truncate muted={!endpoint} value={endpoint || '—'} />
            <Td mono truncate title={row.latestRevision?.digest ?? 'Never compiled'} value={revisionText(row)} />
            <Td title={connectionStatusText(tokens)}>
              <span class="status">
                {#each tokens as token (token.key)}
                  <Badge tone={token.tone} dot={token.key === 'mounted'}>{token.label}</Badge>
                {/each}
              </span>
            </Td>
            <Td mono muted title={row.updatedAt} value={formatMinute(row.updatedAt)} />
          </Tr>
        {/each}
      </Table>
    {/if}
  </div>

  <aside class="detail-pane" aria-label="Connection details">
    {#if selectedKey === NEW_KEY && creating}
      <ConnectionDetails
        bind:buffer={() => creating as EditBuffer, (value) => (creating = value)}
        row={null}
        {writeBlocked}
        onchanged={changed}
        onmissing={missing}
        ondiscard={discard}
      />
    {:else if selectedRow && edits[selectedRow.id]}
      {@const current = selectedRow}
      {#key current.id}
        <ConnectionDetails
          bind:buffer={() => edits[current.id] as EditBuffer, (value) => (edits[current.id] = value)}
          row={current}
          {writeBlocked}
          onchanged={changed}
          onmissing={missing}
          ondiscard={discard}
        />
      {/key}
    {:else}
      <EmptyState message="No connection selected." />
    {/if}
  </aside>
</div>

<style>
  .catalog {
    display: grid;
    grid-template-columns: minmax(0, 1fr) minmax(480px, 36%);
    flex: 1 1 auto;
    min-height: 0;
  }

  .list {
    display: flex;
    flex-direction: column;
    min-width: 0;
    min-height: 0;
  }

  .filters {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    padding: var(--space-2) var(--space-3);
    border-bottom: 1px solid var(--color-border-subtle);
  }

  .filter {
    width: 260px;
  }

  .spacer {
    flex: 1 1 auto;
  }

  .read-only {
    margin: 0;
    padding: var(--space-2) var(--space-3);
    border-bottom: 1px solid var(--color-border-subtle);
    font-size: var(--text-xs);
    color: var(--color-text-secondary);
  }

  .read-only code {
    font-family: var(--font-mono);
    font-size: var(--text-mono);
    color: var(--color-text-primary);
  }

  .list :global(.connections-table) {
    flex: 0 1 auto;
    min-height: 0;
  }

  .status {
    display: inline-flex;
    gap: var(--space-1);
    max-width: 100%;
    overflow: hidden;
  }

  .unsaved {
    color: var(--color-warning-text);
    font-size: var(--text-xs);
  }

  .kind-menu {
    display: flex;
    flex-direction: column;
    gap: 2px;
    margin: calc(var(--space-2) * -1);
  }

  .kind-option {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 2px;
    padding: var(--space-2) var(--space-3);
    border: 0;
    border-radius: var(--radius-sm);
    background: none;
    color: var(--color-text-primary);
    font: inherit;
    text-align: left;
    cursor: pointer;
  }

  .kind-option:hover {
    background: var(--color-bg-hover);
  }

  .kind-option:focus-visible {
    outline: 2px solid var(--color-focus-ring);
    outline-offset: -2px;
  }

  .kind-label {
    font-size: var(--text-ui);
    font-weight: var(--font-medium);
  }

  .kind-summary {
    font-size: var(--text-xs);
    color: var(--color-text-tertiary);
  }

  .detail-pane {
    display: flex;
    flex-direction: column;
    min-width: 0;
    min-height: 0;
    overflow: hidden;
    border-left: 1px solid var(--color-border-subtle);
    background: var(--color-bg-elevated);
  }

  @media (max-width: 1100px) {
    .catalog {
      grid-template-columns: minmax(0, 1fr);
    }

    .detail-pane {
      border-left: 0;
      border-top: 1px solid var(--color-border-subtle);
    }
  }
</style>
