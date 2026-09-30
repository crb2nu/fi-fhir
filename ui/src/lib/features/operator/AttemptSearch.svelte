<!--
  AttemptSearch — Operator › Delivery › Attempts: every durable delivery
  attempt of the tenant, newest first, across receipts (`operatorDeliveryAttempts`).
  The filters are the ones the store supports: status, destination, receipt,
  route, and the recorded-at window. Paging is the server's opaque cursor.

  Each row opens its message trace (Messages) or its attempt inspector (the
  pane beside this list, with the attempt's own paged audit).
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
    Panel,
    Select,
    Table,
    Td,
    Th,
    Tr
  } from '$lib/ui/primitives';
  import AutoRefreshToggle from './AutoRefreshToggle.svelte';
  import { attemptStatusVariant, badgeTone, formatTimestamp } from './attemptPresentation';
  import { fetchAttempts, type OperatorAttempt } from './operatorApi';
  import { describeOperatorFailure } from './operatorErrors';
  import { readTimeWindow } from './timeWindow';

  interface Props {
    selectedAttemptId?: string | null;
    /** Open the attempt inspector for one attempt. */
    oninspect: (attemptId: string) => void;
    /** Open the message trace of one receipt. */
    ontrace: (receiptId: string) => void;
  }

  let { selectedAttemptId = null, oninspect, ontrace }: Props = $props();

  const PAGE_SIZE = 25;
  const statusOptions = [
    { value: '', label: 'Any status' },
    { value: 'queued', label: 'Queued' },
    { value: 'succeeded', label: 'Succeeded' },
    { value: 'failed', label: 'Failed' }
  ];

  let attempts = $state<OperatorAttempt[]>([]);
  let loading = $state(true);
  let error = $state<string | null>(null);
  let filterError = $state<string | null>(null);
  let cursor = $state<string | null>(null);
  let cursors = $state<string[]>([]);
  let hasNextPage = $state(false);

  let status = $state('');
  let destination = $state('');
  let receipt = $state('');
  let route = $state('');
  let from = $state('');
  let to = $state('');
  let seq = 0;

  type AttemptFilterValue = NonNullable<Parameters<typeof fetchAttempts>[0]>;
  /**
   * The filters the list is showing, fixed when Apply is pressed. Every read —
   * paging, auto-refresh, the reload after a control action — uses this
   * snapshot, never the live inputs: a half-typed filter must not be applied,
   * and a cursor belongs to the filters that produced it.
   */
  let applied: AttemptFilterValue = {
    status: null,
    destinationArtifactId: null,
    receiptId: null,
    route: null,
    from: null,
    to: null
  };

  async function load(nextCursor: string | null): Promise<void> {
    const current = ++seq;
    loading = true;
    error = null;
    try {
      const page = await fetchAttempts({ ...applied }, { first: PAGE_SIZE, after: nextCursor });
      if (current !== seq) return;
      attempts = page.nodes;
      hasNextPage = page.pageInfo.hasNextPage;
      cursor = page.pageInfo.endCursor ?? null;
    } catch (err) {
      if (current !== seq) return;
      // Operator reads opt out of the global toast; this panel is the home (B4).
      error = describeOperatorFailure(err).message;
      attempts = [];
      hasNextPage = false;
    } finally {
      if (current === seq) loading = false;
    }
  }

  /** Re-reads the current page (auto-refresh, and after a control action). */
  export function reload(): Promise<void> {
    return load(cursors.at(-1) ?? null);
  }

  function apply(event?: Event): void {
    event?.preventDefault();
    const window = readTimeWindow(from, to);
    if (!window.ok) {
      filterError = window.message;
      return;
    }
    filterError = null;
    applied = {
      status: status || null,
      destinationArtifactId: destination.trim() || null,
      receiptId: receipt.trim() || null,
      route: route.trim() || null,
      from: window.window.from,
      to: window.window.to
    };
    cursors = [];
    void load(null);
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
    void load(null);
  });
</script>

<Panel title="Delivery attempts" flush data-testid="attempt-search">
  {#snippet actions()}
    <AutoRefreshToggle subject="delivery attempts" onrefresh={() => void reload()} />
    <IconButton icon={RefreshCw} label="Refresh delivery attempts" {loading} onclick={() => void reload()} />
  {/snippet}

  <form class="filters" aria-label="Attempt filters" onsubmit={apply}>
    <div class="filter filter-status">
      <Select aria-label="Attempt status" bind:value={status} options={statusOptions} />
    </div>
    <div class="filter">
      <Input aria-label="Destination" bind:value={destination} placeholder="Destination" mono />
    </div>
    <div class="filter">
      <Input aria-label="Receipt" bind:value={receipt} placeholder="Receipt" mono />
    </div>
    <div class="filter">
      <Input aria-label="Route" bind:value={route} placeholder="Route" mono />
    </div>
    <div class="filter filter-time">
      <Input aria-label="Recorded from" type="datetime-local" bind:value={from} />
    </div>
    <div class="filter filter-time">
      <Input aria-label="Recorded to" type="datetime-local" bind:value={to} />
    </div>
    <Button type="submit" disabled={loading}>Apply</Button>
  </form>
  {#if filterError}
    <p class="filter-error" role="alert">{filterError}</p>
  {/if}

  {#if loading && attempts.length === 0}
    <EmptyState message="Loading delivery attempts" aria-busy="true" aria-live="polite" />
  {:else if error}
    <EmptyState icon={CircleAlert} role="alert" message={error} actionLabel="Retry" onaction={() => void reload()} />
  {:else if attempts.length === 0}
    <EmptyState
      icon={Inbox}
      message="No delivery attempts match these filters. An admission that plans a delivery creates one."
    />
  {:else}
    <Table label="Durable delivery attempts" layout="fixed">
      {#snippet head()}
        <tr>
          <Th width="152px">Recorded</Th>
          <Th>Attempt</Th>
          <Th width="96px">Status</Th>
          <Th>Destination</Th>
          <Th>Route</Th>
          <Th width="52px" numeric>Tries</Th>
          <Th>Receipt</Th>
        </tr>
      {/snippet}
      {#each attempts as attempt (attempt.attemptId)}
        <Tr
          selectable
          selected={attempt.attemptId === selectedAttemptId}
          onselect={() => oninspect(attempt.attemptId)}
          data-testid="attempt-row"
          data-attempt-id={attempt.attemptId}
          data-status={attempt.status}
        >
          <Td mono muted value={formatTimestamp(attempt.recordedAt)} />
          <Td>
            <button
              type="button"
              class="link"
              title={`Inspect ${attempt.attemptId}`}
              onclick={(event) => {
                event.stopPropagation();
                oninspect(attempt.attemptId);
              }}
            >
              {attempt.attemptId}
            </button>
          </Td>
          <Td>
            <Badge tone={badgeTone(attemptStatusVariant(attempt.status))} dot>{attempt.status}</Badge>
          </Td>
          <Td
            mono
            truncate
            title={`${attempt.destination.artifactId}@${attempt.destination.revisionId} (${attempt.destination.class})`}
            value={attempt.destination.artifactId}
          />
          <Td mono truncate value={`${attempt.route} → ${attempt.action}`} />
          <Td numeric value={attempt.attemptCount} />
          <Td>
            <button
              type="button"
              class="link"
              title={`Open the message trace of ${attempt.receiptId}`}
              onclick={(event) => {
                event.stopPropagation();
                ontrace(attempt.receiptId);
              }}
            >
              {attempt.receiptId}
            </button>
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
        title={!hasNextPage ? 'No further attempts match these filters.' : undefined}
      >
        Next
      </Button>
    </div>
  {/if}
</Panel>

<style>
  .filters {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2);
    padding: var(--space-2) var(--space-3);
    border-bottom: 1px solid var(--color-border-subtle);
  }

  .filter {
    width: 140px;
  }

  .filter-status {
    width: 124px;
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

  .link {
    max-width: 100%;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    padding: 0;
    border: 0;
    background: none;
    color: var(--color-text-primary);
    font-family: var(--font-mono);
    font-size: var(--text-mono);
    text-decoration: underline;
    text-decoration-color: var(--color-border-strong);
    text-underline-offset: 2px;
    cursor: pointer;
  }

  .link:hover {
    text-decoration-color: currentColor;
  }

  .link:focus-visible {
    outline: 2px solid var(--color-focus-ring);
    outline-offset: 1px;
    border-radius: var(--radius-sm);
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
