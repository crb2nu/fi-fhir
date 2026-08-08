<script lang="ts">
  /**
   * Durable message browser.
   *
   * Lists tenant-scoped admission receipts from the operator control plane and
   * drills into one receipt's full receipt-to-delivery lineage. Filters and the
   * cursor are server-owned: the backend clamps every page and returns an
   * opaque forward cursor, so this component never invents its own paging.
   */

  import { createEventDispatcher, onMount } from 'svelte';
  import Badge from '$lib/ui/Badge.svelte';
  import Button from '$lib/ui/Button.svelte';
  import EmptyState from '$lib/ui/EmptyState.svelte';
  import Input from '$lib/ui/Input.svelte';
  import Panel from '$lib/ui/Panel.svelte';
  import Select from '$lib/ui/Select.svelte';
  import Skeleton from '$lib/ui/Skeleton.svelte';
  import { fetchReceipts, type OperatorReceipt } from './operatorApi';
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

  const statusOptions = [
    { value: '', label: 'Any status' },
    { value: 'accepted', label: 'Accepted' },
    { value: 'rejected', label: 'Rejected' }
  ];

  async function load(nextCursor: string | null = null) {
    loading = true;
    error = null;
    try {
      const page = await fetchReceipts(
        {
          status: statusFilter || null,
          correlationId: correlationId.trim() || null,
          sourceMessageId: sourceMessageId.trim() || null,
          integrationArtifactId: integrationArtifactId.trim() || null,
          from: null,
          to: null
        },
        { first: 25, after: nextCursor }
      );
      receipts = page.nodes;
      hasNextPage = page.pageInfo.hasNextPage;
      cursor = page.pageInfo.endCursor ?? null;
    } catch (err) {
      // The global graphqlFetch net already toasted this failure; the inline
      // home below is the durable surface (toast-budget B4).
      error = describeOperatorFailure(err).message;
      receipts = [];
      hasNextPage = false;
    } finally {
      loading = false;
    }
  }

  function applyFilters() {
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
    void load(previous.length > 0 ? previous[previous.length - 1] : null);
  }

  onMount(() => {
    void load(null);
  });
</script>

<Panel title="Messages" padding="md">
  <svelte:fragment slot="actions">
    <Button size="sm" variant="secondary" on:click={() => applyFilters()} disabled={loading}>
      Refresh
    </Button>
  </svelte:fragment>

  <form
    class="filters"
    on:submit|preventDefault={applyFilters}
    aria-label="Message filters"
  >
    <Select label="Status" bind:value={statusFilter} options={statusOptions} size="sm" />
    <Input label="Correlation ID" bind:value={correlationId} size="sm" placeholder="correlation-…" />
    <Input label="Source message ID" bind:value={sourceMessageId} size="sm" placeholder="MSH-10" />
    <Input
      label="Integration"
      bind:value={integrationArtifactId}
      size="sm"
      placeholder="artifact ID"
    />
    <div class="filter-action">
      <Button size="sm" type="submit" disabled={loading}>Apply</Button>
    </div>
  </form>

  {#if loading}
    <div class="loading" aria-busy="true" aria-live="polite">
      <Skeleton lines={4} />
      <span class="sr-only">Loading messages</span>
    </div>
  {:else if error}
    <div class="error-state" role="alert">
      <p class="error-message">{error}</p>
      <Button size="sm" variant="secondary" on:click={() => load(null)}>Retry</Button>
    </div>
  {:else if receipts.length === 0}
    <EmptyState
      icon="inbox"
      title="No messages match these filters"
      description="Durable admissions appear here as soon as an integration accepts a message."
    />
  {:else}
    <div class="table-scroll">
      <table class="records">
        <caption class="sr-only">Durable admission receipts</caption>
        <thead>
          <tr>
            <th scope="col">Receipt</th>
            <th scope="col">Status</th>
            <th scope="col">Correlation</th>
            <th scope="col">Integration</th>
            <th scope="col">Events</th>
            <th scope="col">Failed</th>
            <th scope="col">Dead letters</th>
            <th scope="col">Recorded</th>
          </tr>
        </thead>
        <tbody>
          {#each receipts as receipt (receipt.receiptId)}
            <tr class:selected={receipt.receiptId === selectedReceiptId}>
              <th scope="row">
                <button
                  type="button"
                  class="link"
                  on:click={() => dispatch('select', { receiptId: receipt.receiptId })}
                >
                  {receipt.receiptId}
                </button>
              </th>
              <td>
                <Badge variant={receipt.status === 'accepted' ? 'success' : 'danger'} size="sm">
                  {receipt.status}
                </Badge>
              </td>
              <td class="mono">{receipt.correlationId}</td>
              <td class="mono" title={receipt.integrationRevision.digest}>
                {receipt.integrationRevision.artifactId}@{receipt.integrationRevision.revisionId}
                <span class="digest">{shortDigest(receipt.integrationRevision.digest)}</span>
              </td>
              <td class="numeric">{receipt.eventCount}</td>
              <td class="numeric">
                {#if receipt.failedAttemptCount > 0}
                  <Badge variant="danger" size="sm">{receipt.failedAttemptCount}</Badge>
                {:else}
                  {receipt.failedAttemptCount}
                {/if}
              </td>
              <td class="numeric">
                {#if receipt.deadLetterCount > 0}
                  <Badge variant="warning" size="sm">{receipt.deadLetterCount}</Badge>
                {:else}
                  {receipt.deadLetterCount}
                {/if}
              </td>
              <td class="mono">{formatTimestamp(receipt.recordedAt)}</td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>

    <div class="pagination">
      <Button
        size="sm"
        variant="secondary"
        on:click={previousPage}
        disabled={cursors.length === 0 || loading}
        title={cursors.length === 0 ? 'You are on the first page.' : undefined}
      >
        Previous
      </Button>
      <Button
        size="sm"
        variant="secondary"
        on:click={nextPage}
        disabled={!hasNextPage || loading}
        title={!hasNextPage ? 'No further pages match these filters.' : undefined}
      >
        Next
      </Button>
    </div>
  {/if}
</Panel>

<style>
  .filters {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(10rem, 1fr));
    gap: var(--space-3);
    align-items: end;
    margin-bottom: var(--space-4);
  }

  .filter-action {
    display: flex;
    align-items: flex-end;
  }

  .loading {
    padding: var(--space-2) 0;
  }

  .error-state {
    background: var(--color-danger-bg);
    border: 1px solid var(--color-danger-border);
    border-radius: var(--radius-md);
    padding: var(--space-3);
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3);
  }

  .error-message {
    margin: 0;
    color: var(--color-danger-text);
    font-size: var(--text-sm);
  }

  .table-scroll {
    overflow-x: auto;
  }

  .records {
    width: 100%;
    border-collapse: collapse;
    font-size: var(--text-sm);
  }

  .records th,
  .records td {
    text-align: left;
    padding: var(--space-2);
    border-bottom: 1px solid var(--color-border-subtle);
    white-space: nowrap;
  }

  .records thead th {
    color: var(--color-text-tertiary);
    font-size: var(--text-xs);
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }

  .records tbody tr.selected {
    background: var(--color-bg-active);
  }

  .numeric {
    text-align: right;
  }

  .mono {
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    color: var(--color-text-secondary);
  }

  .digest {
    color: var(--color-text-muted);
  }

  .link {
    background: none;
    border: none;
    padding: 0;
    color: var(--color-primary);
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    cursor: pointer;
    text-decoration: underline;
  }

  .link:hover {
    color: var(--color-primary-hover);
  }

  .pagination {
    display: flex;
    justify-content: flex-end;
    gap: var(--space-2);
    margin-top: var(--space-3);
  }

  .sr-only {
    position: absolute;
    width: 1px;
    height: 1px;
    padding: 0;
    margin: -1px;
    overflow: hidden;
    clip: rect(0, 0, 0, 0);
    white-space: nowrap;
    border: 0;
  }
</style>
