<script lang="ts">
  /**
   * Durable message browser: Operator › Messages, left side.
   *
   * Lists tenant-scoped admission receipts from the operator control plane; a
   * selected row opens its receipt-to-delivery trace in the pane beside it.
   * Filters and the cursor are server-owned: the backend clamps every page and
   * returns an opaque forward cursor, so this component never invents its own
   * paging. The recorded-at window is the store's own from/to filter, and
   * auto-refresh (off by default) re-reads the current page every 15 s.
   */

  import { createEventDispatcher, onMount } from 'svelte';
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
    Select,
    Table,
    Td,
    Th,
    Tr
  } from '$lib/ui/primitives';
  import AutoRefreshToggle from './AutoRefreshToggle.svelte';
  import { fetchReceipts, type OperatorReceipt } from './operatorApi';
  import { readTimeWindow } from './timeWindow';
  import { describeOperatorFailure } from './operatorErrors';
  import { formatTimestamp, shortDigest } from './attemptPresentation';

  const dispatch = createEventDispatcher<{ select: { receiptId: string } }>();

  export let selectedReceiptId: string | null = null;

  let receipts: OperatorReceipt[] = [];
  let loading = true;
  let error: string | null = null;
  let cursor: string | null = null;
  let cursors: string[] = [];
  let hasNextPage = false;

  let statusFilter = '';
  let correlationId = '';
  let sourceMessageId = '';
  let integrationArtifactId = '';
  let from = '';
  let to = '';
  let filterError: string | null = null;
  let seq = 0;

  type ReceiptFilterValue = NonNullable<Parameters<typeof fetchReceipts>[0]>;
  /**
   * The filters the list is showing, fixed when Apply is pressed; paging and
   * auto-refresh read this snapshot, never the live inputs.
   */
  let applied: ReceiptFilterValue = {
    status: null,
    correlationId: null,
    sourceMessageId: null,
    integrationArtifactId: null,
    from: null,
    to: null
  };

  const statusOptions = [
    { value: '', label: 'Any status' },
    { value: 'accepted', label: 'Accepted' },
    { value: 'rejected', label: 'Rejected' }
  ];

  async function load(nextCursor: string | null = null) {
    const current = ++seq;
    loading = true;
    error = null;
    try {
      const page = await fetchReceipts({ ...applied }, { first: 25, after: nextCursor });
      if (current !== seq) return;
      receipts = page.nodes;
      hasNextPage = page.pageInfo.hasNextPage;
      cursor = page.pageInfo.endCursor ?? null;
    } catch (err) {
      if (current !== seq) return;
      // Operator reads opt out of the global toast; the inline home below is
      // the only surface for this failure (toast-budget B4).
      error = describeOperatorFailure(err).message;
      receipts = [];
      hasNextPage = false;
    } finally {
      if (current === seq) loading = false;
    }
  }

  /** Re-reads the current page with the applied filters (auto-refresh). */
  export function reload() {
    void load(cursors.at(-1) ?? null);
  }

  function applyFilters() {
    const window = readTimeWindow(from, to);
    if (!window.ok) {
      filterError = window.message;
      return;
    }
    filterError = null;
    applied = {
      status: statusFilter || null,
      correlationId: correlationId.trim() || null,
      sourceMessageId: sourceMessageId.trim() || null,
      integrationArtifactId: integrationArtifactId.trim() || null,
      from: window.window.from,
      to: window.window.to
    };
    cursors = [];
    void load(null);
  }

  function nextPage() {
    if (!cursor) return;
    cursors = [...cursors, cursor];
    void load(cursor);
  }

  function previousPage() {
    const previous = cursors.slice(0, -1);
    cursors = previous;
    void load(previous.at(-1) ?? null);
  }

  function select(receiptId: string) {
    dispatch('select', { receiptId });
  }

  onMount(() => {
    void load(null);
  });
</script>

<div class="browser">
  <form
    class="filters"
    aria-label="Message filters"
    on:submit|preventDefault={applyFilters}
  >
    <div class="filter filter-status">
      <Select aria-label="Status" bind:value={statusFilter} options={statusOptions} />
    </div>
    <div class="filter">
      <Input aria-label="Correlation ID" bind:value={correlationId} placeholder="Correlation id" mono />
    </div>
    <div class="filter">
      <Input aria-label="Source message ID" bind:value={sourceMessageId} placeholder="MSH-10" mono />
    </div>
    <div class="filter">
      <Input
        aria-label="Integration"
        bind:value={integrationArtifactId}
        placeholder="Integration"
        mono
      />
    </div>
    <div class="filter filter-time">
      <Input aria-label="Received from" type="datetime-local" bind:value={from} />
    </div>
    <div class="filter filter-time">
      <Input aria-label="Received to" type="datetime-local" bind:value={to} />
    </div>
    <Button type="submit" disabled={loading}>Apply</Button>
    <span class="spacer"></span>
    <AutoRefreshToggle subject="messages" onrefresh={reload} />
    <IconButton icon={RefreshCw} label="Refresh messages" {loading} onclick={reload} />
  </form>
  {#if filterError}
    <p class="filter-error" role="alert">{filterError}</p>
  {/if}

  {#if loading && receipts.length === 0}
    <EmptyState message="Loading messages" aria-busy="true" aria-live="polite" />
  {:else if error}
    <EmptyState
      icon={CircleAlert}
      role="alert"
      message={error}
      actionLabel="Retry"
      onaction={() => void load(null)}
    />
  {:else if receipts.length === 0}
    <EmptyState icon={Inbox} message="No messages match these filters" />
  {:else}
    <Table label="Durable admission receipts" layout="fixed" class="receipts">
      {#snippet head()}
        <tr>
          <Th width="152px">Recorded</Th>
          <Th>Receipt</Th>
          <Th width="88px">Status</Th>
          <Th>Correlation</Th>
          <Th>Integration</Th>
          <Th width="62px" numeric>Events</Th>
          <Th width="58px" numeric>Failed</Th>
          <Th width="44px" numeric>DLQ</Th>
        </tr>
      {/snippet}
      {#each receipts as receipt (receipt.receiptId)}
        <Tr
          selectable
          selected={receipt.receiptId === selectedReceiptId}
          onselect={() => select(receipt.receiptId)}
          data-testid="receipt-row"
          data-receipt-id={receipt.receiptId}
        >
          <Td mono muted value={formatTimestamp(receipt.recordedAt)} />
          <Td mono truncate value={receipt.receiptId} />
          <Td>
            <Badge tone={receipt.status === 'accepted' ? 'success' : 'danger'} dot>
              {receipt.status}
            </Badge>
          </Td>
          <Td mono truncate muted value={receipt.correlationId} />
          <Td
            mono
            truncate
            title={`${receipt.integrationRevision.artifactId}@${receipt.integrationRevision.revisionId} ${shortDigest(receipt.integrationRevision.digest)}`}
            value={`${receipt.integrationRevision.artifactId}@${receipt.integrationRevision.revisionId}`}
          />
          <Td numeric value={receipt.eventCount} />
          <Td numeric>
            {#if receipt.failedAttemptCount > 0}
              <Badge tone="danger" mono>{receipt.failedAttemptCount}</Badge>
            {:else}
              {receipt.failedAttemptCount}
            {/if}
          </Td>
          <Td numeric>
            {#if receipt.deadLetterCount > 0}
              <Badge tone="warning" mono>{receipt.deadLetterCount}</Badge>
            {:else}
              {receipt.deadLetterCount}
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
        title={!hasNextPage ? 'No further pages match these filters.' : undefined}
      >
        Next
      </Button>
    </div>
  {/if}
</div>

<style>
  .browser {
    display: flex;
    flex-direction: column;
    flex: 1 1 auto;
    min-height: 0;
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
    width: 150px;
  }

  .filter-status {
    width: 128px;
  }

  .filter-time {
    width: 188px;
  }

  .filter-error {
    margin: 0;
    padding: var(--space-1) var(--space-3);
    font-size: var(--text-xs);
    color: var(--color-danger-text);
  }

  .spacer {
    flex: 1 1 auto;
  }

  .browser :global(.receipts) {
    flex: 0 1 auto;
    min-height: 0;
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
</style>
