<!--
  AdmissionsBrowser — Verification › Admissions: every canonical event the
  admission path committed, newest first (`operatorCanonicalEvents`), each
  with the receipt that admitted it and the revisions that produced it.

  The filters are the columns the store matches exactly: event type,
  definition, receipt, MSH-10, correlation, the recorded-at window, and
  whether retention-tombstoned rows are shown. Paging is the server's opaque
  cursor. A row's receipt opens its trace on Operator; its definition and
  source open Connections. Selecting a row shows the payload's structure —
  field paths and JSON kinds — never a value.
-->
<script lang="ts">
  import { onMount } from 'svelte';
  import ChevronLeft from '@lucide/svelte/icons/chevron-left';
  import ChevronRight from '@lucide/svelte/icons/chevron-right';
  import CircleAlert from '@lucide/svelte/icons/circle-alert';
  import Inbox from '@lucide/svelte/icons/inbox';
  import RefreshCw from '@lucide/svelte/icons/refresh-cw';
  import {
    Badge,
    Button,
    EmptyState,
    IconButton,
    Input,
    KeyValue,
    Panel,
    Table,
    Td,
    Th,
    Tr
  } from '$lib/ui/primitives';
  import { formatTimestamp } from '$lib/features/operator/attemptPresentation';
  import { describeOperatorFailure } from '$lib/features/operator/operatorErrors';
  import { readTimeWindow } from '$lib/features/operator/timeWindow';
  import { fetchAdmissions, type Admission } from './verificationApi';
  import { connectionHref, definitionHref, operatorReceiptHref } from './verificationLinks';

  interface Props {
    /** A `/events?receipt=` deep link: the receipt filter to open with. */
    initialReceiptId?: string | null;
  }

  let { initialReceiptId = null }: Props = $props();

  const PAGE_SIZE = 25;

  let admissions = $state<Admission[]>([]);
  let loading = $state(true);
  let error = $state<string | null>(null);
  let filterError = $state<string | null>(null);
  let cursor = $state<string | null>(null);
  let cursors = $state<string[]>([]);
  let hasNextPage = $state(false);
  let selectedId = $state<string | null>(null);

  let eventType = $state('');
  let definition = $state('');
  let receipt = $state('');
  let sourceMessage = $state('');
  let correlation = $state('');
  let from = $state('');
  let to = $state('');
  let includePurged = $state(false);
  let seq = 0;

  const selected = $derived(admissions.find((row) => row.eventId === selectedId) ?? null);
  const filtered = $derived(
    Boolean(eventType || definition || receipt || sourceMessage || correlation || from || to)
  );

  async function load(nextCursor: string | null): Promise<void> {
    const window = readTimeWindow(from, to);
    if (!window.ok) {
      filterError = window.message;
      return;
    }
    filterError = null;
    const current = ++seq;
    loading = true;
    error = null;
    try {
      const page = await fetchAdmissions(
        {
          eventType: eventType.trim() || null,
          definitionId: definition.trim() || null,
          receiptId: receipt.trim() || null,
          sourceMessageId: sourceMessage.trim() || null,
          correlationId: correlation.trim() || null,
          from: window.window.from,
          to: window.window.to,
          includePurged
        },
        { first: PAGE_SIZE, after: nextCursor }
      );
      if (current !== seq) return;
      admissions = page.nodes;
      hasNextPage = page.pageInfo.hasNextPage;
      cursor = page.pageInfo.endCursor ?? null;
      if (selectedId && !page.nodes.some((row) => row.eventId === selectedId)) selectedId = null;
    } catch (err) {
      if (current !== seq) return;
      error = describeOperatorFailure(err).message;
      admissions = [];
      hasNextPage = false;
    } finally {
      if (current === seq) loading = false;
    }
  }

  function apply(event?: Event): void {
    event?.preventDefault();
    cursors = [];
    void load(null);
  }

  function clearFilters(): void {
    eventType = '';
    definition = '';
    receipt = '';
    sourceMessage = '';
    correlation = '';
    from = '';
    to = '';
    apply();
  }

  function nextPage(): void {
    if (!cursor) return;
    cursors = [...cursors, cursor];
    void load(cursor);
  }

  function previousPage(): void {
    const previous = cursors.slice(0, -1);
    cursors = previous;
    void load(previous.at(-1) ?? null);
  }

  onMount(() => {
    if (initialReceiptId) receipt = initialReceiptId;
    void load(null);
  });

  function retentionLabel(row: Admission): string | null {
    if (row.purgedAt) return 'tombstoned';
    if (row.purgeAfter) return `purge after ${formatTimestamp(row.purgeAfter).slice(0, 10)}`;
    return null;
  }
</script>

<div class="admissions">
  <Panel title="Admissions" flush data-testid="verification-admissions">
    {#snippet actions()}
      <IconButton icon={RefreshCw} label="Refresh admissions" {loading} onclick={() => apply()} />
    {/snippet}

    <form class="filters" aria-label="Admission filters" onsubmit={apply}>
      <div class="filter">
        <Input aria-label="Event type" bind:value={eventType} placeholder="Event type" mono />
      </div>
      <div class="filter">
        <Input aria-label="Definition" bind:value={definition} placeholder="Definition" mono />
      </div>
      <div class="filter">
        <Input aria-label="Receipt" bind:value={receipt} placeholder="Receipt" mono />
      </div>
      <div class="filter">
        <Input aria-label="Source message ID (MSH-10)" bind:value={sourceMessage} placeholder="MSH-10" mono />
      </div>
      <div class="filter">
        <Input aria-label="Correlation" bind:value={correlation} placeholder="Correlation" mono />
      </div>
      <div class="filter filter-time">
        <Input aria-label="Recorded from" type="datetime-local" bind:value={from} />
      </div>
      <div class="filter filter-time">
        <Input aria-label="Recorded to" type="datetime-local" bind:value={to} />
      </div>
      <label class="toggle">
        <input type="checkbox" bind:checked={includePurged} data-testid="admissions-include-purged" />
        Include tombstoned
      </label>
      <Button type="submit" disabled={loading}>Apply</Button>
      {#if filtered}
        <Button variant="ghost" onclick={clearFilters} disabled={loading}>Clear</Button>
      {/if}
    </form>
    {#if filterError}
      <p class="filter-error" role="alert">{filterError}</p>
    {/if}

    {#if loading && admissions.length === 0}
      <EmptyState message="Loading admissions" aria-busy="true" aria-live="polite" />
    {:else if error}
      <EmptyState icon={CircleAlert} role="alert" message={error} actionLabel="Retry" onaction={() => apply()} />
    {:else if admissions.length === 0}
      <EmptyState
        icon={Inbox}
        data-testid="admissions-empty"
        message={filtered || includePurged
          ? 'No admissions match these filters.'
          : 'No admissions yet. A message the engine accepts over HTTP, MLLP or a batch source is recorded here once committed.'}
      />
    {:else}
      <Table label="Durable admissions" layout="fixed">
        {#snippet head()}
          <tr>
            <Th width="152px">Recorded</Th>
            <Th width="132px">Event type</Th>
            <Th>MSH-10</Th>
            <Th>Receipt</Th>
            <Th>Definition</Th>
            <Th>Source</Th>
            <Th width="140px">Retention</Th>
          </tr>
        {/snippet}
        {#each admissions as row (row.eventId)}
          <Tr
            selectable
            selected={row.eventId === selectedId}
            onselect={() => (selectedId = row.eventId)}
            data-testid="admission-row"
            data-event-id={row.eventId}
            data-receipt-id={row.receiptId}
            data-event-type={row.eventType}
          >
            <Td mono muted value={formatTimestamp(row.recordedAt)} />
            <Td mono truncate value={row.eventType} />
            <Td mono truncate value={row.sourceMessageId} />
            <Td>
              <span class="cell">
                <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- verificationLinks.ts builds this href from resolve() plus a query string -->
                <a href={operatorReceiptHref(row.receiptId)}
                  class="link"
                  title={`Open the trace of ${row.receiptId} on Operator`}
                  data-testid="admission-receipt-link"
                  onclick={(event) => event.stopPropagation()}>{row.receiptId}</a
                >
                {#if row.receiptStatus !== 'accepted'}
                  <Badge tone="danger">{row.receiptStatus}</Badge>
                {/if}
              </span>
            </Td>
            <Td>
              <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- verificationLinks.ts builds this href from resolve() plus a query string -->
              <a href={definitionHref(row.definition.artifactId, row.definition.revisionId)}
                class="link"
                title={`${row.definition.artifactId}@${row.definition.revisionId} on Connections › Definitions`}
                data-testid="admission-definition-link"
                onclick={(event) => event.stopPropagation()}
                >{row.definition.artifactId}@{row.definition.revisionId}</a
              >
            </Td>
            <Td>
              {#if row.source}
                <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- verificationLinks.ts builds this href from resolve() plus a query string -->
                <a href={connectionHref(row.source.artifactId)}
                  class="link"
                  title={`${row.source.artifactId}@${row.source.revisionId} on Connections`}
                  data-testid="admission-source-link"
                  onclick={(event) => event.stopPropagation()}>{row.source.artifactId}</a
                >
              {:else}
                <span class="muted" title="This event has no lineage row naming its source.">—</span>
              {/if}
            </Td>
            <Td>
              {#if retentionLabel(row)}
                <Badge tone={row.purgedAt ? 'warning' : 'neutral'} data-testid="admission-retention"
                  >{retentionLabel(row)}</Badge
                >
              {:else}
                <span class="muted">—</span>
              {/if}
            </Td>
          </Tr>
        {/each}
      </Table>

      <div class="pagination">
        <span class="page text-mono">Page {cursors.length + 1}</span>
        <Button
          variant="ghost"
          icon={ChevronLeft}
          onclick={previousPage}
          disabled={cursors.length === 0 || loading}
          title={cursors.length === 0 ? 'You are on the first page.' : undefined}
        >
          Previous
        </Button>
        <Button
          variant="ghost"
          icon={ChevronRight}
          onclick={nextPage}
          disabled={!hasNextPage || loading}
          title={!hasNextPage ? 'No further admissions match these filters.' : undefined}
        >
          Next
        </Button>
      </div>
    {/if}
  </Panel>

  {#if selected}
    <Panel title="Admission" data-testid="admission-detail">
      <KeyValue
        columns={2}
        items={[
          { key: 'Event', value: selected.eventId, mono: true, truncate: true },
          { key: 'Type', value: selected.eventType, mono: true },
          { key: 'MSH-10', value: selected.sourceMessageId, mono: true, truncate: true },
          { key: 'Correlation', value: selected.correlationId, mono: true, truncate: true },
          { key: 'Receipt', value: `${selected.receiptId} (${selected.receiptStatus})`, mono: true, truncate: true },
          { key: 'Recorded', value: formatTimestamp(selected.recordedAt), mono: true },
          {
            key: 'Definition',
            value: `${selected.definition.artifactId}@${selected.definition.revisionId}`,
            mono: true,
            truncate: true
          },
          { key: 'Classification', value: selected.classification, mono: true },
          { key: 'Purge after', value: selected.purgeAfter ? formatTimestamp(selected.purgeAfter) : null, mono: true },
          { key: 'Tombstoned', value: selected.purgedAt ? formatTimestamp(selected.purgedAt) : null, mono: true }
        ]}
      />
      <h3 class="fields-title">
        Payload structure
        <span class="muted">
          {selected.purgedAt
            ? 'of the tombstone retention left in place of the payload'
            : 'field paths and JSON kinds; stored values are never read back'}
        </span>
      </h3>
      {#if selected.payloadFields.length === 0}
        <p class="muted">No fields recorded.</p>
      {:else}
        <ul class="fields" data-testid="admission-fields">
          {#each selected.payloadFields as field (field.path)}
            <li>
              <code>{field.path}</code>
              <span class="kind">{field.kind}{field.repeated ? ' (repeated)' : ''}</span>
            </li>
          {/each}
        </ul>
        {#if selected.payloadTruncated}
          <p class="muted">The structure is truncated at the server's bound.</p>
        {/if}
      {/if}
    </Panel>
  {/if}
</div>

<style>
  .admissions {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
  }

  .filters {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2);
    padding: var(--space-2) var(--space-3);
    border-bottom: 1px solid var(--color-border-subtle);
  }

  .filter {
    width: 132px;
  }

  .filter-time {
    width: 188px;
  }

  .toggle {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
    font-size: var(--text-xs);
    color: var(--color-text-secondary);
  }

  .filter-error {
    margin: 0;
    padding: var(--space-1) var(--space-3);
    font-size: var(--text-xs);
    color: var(--color-danger-text);
  }

  .cell {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
    max-width: 100%;
  }

  .link {
    max-width: 100%;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: var(--color-text-primary);
    font-family: var(--font-mono);
    font-size: var(--text-mono);
    text-decoration: underline;
    text-decoration-color: var(--color-border-strong);
    text-underline-offset: 2px;
  }

  .link:hover {
    text-decoration-color: currentColor;
  }

  .link:focus-visible {
    outline: 2px solid var(--color-focus-ring);
    outline-offset: 1px;
    border-radius: var(--radius-sm);
  }

  .muted {
    font-size: var(--text-xs);
    color: var(--color-text-tertiary);
  }

  .pagination {
    display: flex;
    align-items: center;
    justify-content: flex-end;
    gap: var(--space-1);
    padding: var(--space-1) var(--space-3);
    border-top: 1px solid var(--color-border-subtle);
  }

  .page {
    margin-right: auto;
    color: var(--color-text-tertiary);
  }

  .fields-title {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: var(--space-2);
    margin: var(--space-3) 0 var(--space-1);
    font-size: var(--text-sm);
    font-weight: 600;
    color: var(--color-text-primary);
  }

  .fields {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(260px, 1fr));
    gap: var(--space-1) var(--space-3);
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .fields li {
    display: flex;
    justify-content: space-between;
    gap: var(--space-2);
    min-width: 0;
  }

  .fields code {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-family: var(--font-mono);
    font-size: var(--text-mono);
    color: var(--color-text-primary);
  }

  .kind {
    flex: none;
    font-size: var(--text-xs);
    color: var(--color-text-tertiary);
  }
</style>
