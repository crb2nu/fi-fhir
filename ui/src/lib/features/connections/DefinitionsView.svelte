<!--
  Definitions (.loom/42 E-1): the lifecycle catalog's integration definitions —
  which compiled source and destination revisions each binds, and where it is
  in draft → validated → approved → published → deployed — with a details
  pane and New definition. Deploy stays on the Operator page; this tab links
  there.
-->
<script lang="ts">
  import { onMount } from 'svelte';
  import CircleAlert from '@lucide/svelte/icons/circle-alert';
  import FileStack from '@lucide/svelte/icons/file-stack';
  import Plus from '@lucide/svelte/icons/plus';
  import RefreshCw from '@lucide/svelte/icons/refresh-cw';
  import { Badge, Button, EmptyState, IconButton, Input, Table, Td, Th, Tr } from '$lib/ui/primitives';
  import DefinitionDetail from './DefinitionDetail.svelte';
  import NewDefinitionForm from './NewDefinitionForm.svelte';
  import { fetchDefinitions, type DefinitionDetail as Detail, type DefinitionRow } from './definitionsApi';
  import { describeDefinitionFailure } from './definitionsErrors';
  import { validationLabel } from './definitionDraft';
  import { formatMinute, shortHash } from './presentation';
  import { stateTone, validationTone, VALIDATION_TEXT } from './definitionPresentation';

  interface Props {
    /** Roles this identity lacks to author, or null when it may. */
    writeBlocked: string[] | null;
    /** A deep link's target (`?definition=&revision=`), selected on mount. */
    initial?: { definitionId: string; revisionId: string } | null | undefined;
  }

  let { writeBlocked, initial = null }: Props = $props();

  let rows = $state<DefinitionRow[]>([]);
  let loading = $state(true);
  let error = $state<string | null>(null);
  let filter = $state('');
  let includeRetired = $state(false);
  let selected = $state<{ definitionId: string; revisionId: string } | null>(null);
  let creating = $state(false);
  let now = $state(new Date());
  let loadSeq = 0;

  const writeReason = $derived(
    writeBlocked ? `Read only: this identity does not hold ${writeBlocked.join(', ')}.` : undefined
  );

  const visibleRows = $derived.by(() => {
    const needle = filter.trim().toLowerCase();
    if (!needle) return rows;
    return rows.filter((row) =>
      [row.definitionId, row.revisionId, row.state, row.source.artifactId, ...row.destinations.map((d) => d.artifactId)]
        .join(' ')
        .toLowerCase()
        .includes(needle)
    );
  });

  async function load(): Promise<void> {
    const seq = ++loadSeq;
    loading = true;
    error = null;
    try {
      const list = await fetchDefinitions(includeRetired);
      if (seq !== loadSeq) return;
      rows = list;
      now = new Date();
    } catch (err) {
      if (seq !== loadSeq) return;
      error = describeDefinitionFailure(err).message;
      rows = [];
    } finally {
      if (seq === loadSeq) loading = false;
    }
  }

  onMount(() => {
    if (initial) selected = { ...initial };
    void load();
    const timer = setInterval(() => (now = new Date()), 15000);
    return () => clearInterval(timer);
  });

  function isSelected(row: DefinitionRow): boolean {
    return !creating && selected?.definitionId === row.definitionId && selected?.revisionId === row.revisionId;
  }

  function select(row: DefinitionRow): void {
    creating = false;
    selected = { definitionId: row.definitionId, revisionId: row.revisionId };
  }

  function changed(detail: Detail): void {
    const next = detail.definition;
    const others = rows.filter((row) => !(row.definitionId === next.definitionId && row.revisionId === next.revisionId));
    rows = [...others, next].sort((left, right) =>
      `${left.definitionId}/${left.revisionId}`.localeCompare(`${right.definitionId}/${right.revisionId}`)
    );
    creating = false;
    selected = { definitionId: next.definitionId, revisionId: next.revisionId };
    now = new Date();
  }

  function toggleRetired(): void {
    includeRetired = !includeRetired;
    void load();
  }
</script>

<div class="definitions">
  <div class="list">
    <div class="filters" role="search" aria-label="Filter definitions">
      <div class="filter">
        <Input aria-label="Filter definitions" placeholder="Filter by definition, state or connection" bind:value={filter} />
      </div>
      <Button variant="ghost" aria-pressed={includeRetired ? 'true' : 'false'} onclick={toggleRetired} disabled={loading}>
        Show retired
      </Button>
      <span class="spacer"></span>
      <IconButton icon={RefreshCw} label="Refresh definitions" {loading} onclick={load} />
      <Button
        icon={Plus}
        disabled={writeReason !== undefined}
        title={writeReason ?? 'New definition'}
        data-testid="definitions-new"
        onclick={() => (creating = true)}
      >
        New definition
      </Button>
    </div>

    {#if loading && rows.length === 0}
      <EmptyState message="Loading definitions" aria-busy="true" aria-live="polite" />
    {:else if error}
      <EmptyState icon={CircleAlert} role="alert" message={error} actionLabel="Retry" onaction={load} />
    {:else if rows.length === 0}
      <EmptyState
        icon={FileStack}
        message={`No integration definitions${includeRetired ? '' : ' outside retirement'} are in the lifecycle catalog.`}
      />
    {:else if visibleRows.length === 0}
      <EmptyState message="No definitions match this filter." actionLabel="Clear filter" onaction={() => (filter = '')} />
    {:else}
      <Table label="Integration definitions" layout="fixed" class="definitions-table" data-testid="definitions-table">
        {#snippet head()}
          <tr>
            <Th>Definition</Th>
            <Th width="72px">Revision</Th>
            <Th width="96px">State</Th>
            <Th width="84px">Health</Th>
            <Th width="100px">Validation</Th>
            <Th width="112px">Release</Th>
            <Th>Source</Th>
            <Th>Destinations</Th>
          </tr>
        {/snippet}
        {#each visibleRows as row (`${row.definitionId}/${row.revisionId}`)}
          {@const validation = validationLabel(row, now)}
          <Tr
            selectable
            selected={isSelected(row)}
            onselect={() => select(row)}
            data-row={`${row.definitionId}/${row.revisionId}`}
            data-state={row.state}
          >
            <Td mono truncate value={row.definitionId} title={row.digest} />
            <Td mono value={row.revisionId} />
            <Td><Badge tone={stateTone(row.state)}>{row.state}</Badge></Td>
            <Td muted value={row.health} />
            <Td title={row.validationExpiresAt ? `expires ${row.validationExpiresAt}` : undefined}>
              <Badge tone={validationTone(validation)}>{VALIDATION_TEXT[validation]}</Badge>
            </Td>
            <Td mono truncate muted={!row.releaseId} value={row.releaseId ? shortHash(row.releaseId.replace(/^release-/, ''), 8) : '—'} title={row.releaseId ?? undefined} />
            <Td mono truncate value={`${row.source.artifactId} r${row.source.revisionId}`} title={row.source.digest} />
            <Td mono truncate value={row.destinations.map((d) => d.artifactId).join(', ')} title={`updated ${formatMinute(row.updatedAt)}`} />
          </Tr>
        {/each}
      </Table>
    {/if}
  </div>

  <aside class="detail-pane" aria-label="Definition details">
    {#if creating}
      <NewDefinitionForm {writeBlocked} oncreated={changed} oncancel={() => (creating = false)} />
    {:else if selected}
      {#key `${selected.definitionId}/${selected.revisionId}`}
        <DefinitionDetail
          definitionId={selected.definitionId}
          revisionId={selected.revisionId}
          {writeBlocked}
          onchanged={changed}
        />
      {/key}
    {:else}
      <EmptyState message="No definition selected." />
    {/if}
  </aside>
</div>

<style>
  .definitions {
    display: grid;
    grid-template-columns: minmax(0, 1fr) minmax(520px, 40%);
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

  .list :global(.definitions-table) {
    flex: 0 1 auto;
    min-height: 0;
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
    .definitions {
      grid-template-columns: minmax(0, 1fr);
    }

    .detail-pane {
      border-left: 0;
      border-top: 1px solid var(--color-border-subtle);
    }
  }
</style>
