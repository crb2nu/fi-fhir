<script lang="ts">
  import { onMount, createEventDispatcher } from 'svelte';
  import { SvelteSet } from 'svelte/reactivity';
  import Ban from '@lucide/svelte/icons/ban';
  import CircleAlert from '@lucide/svelte/icons/circle-alert';
  import Inbox from '@lucide/svelte/icons/inbox';
  import RefreshCw from '@lucide/svelte/icons/refresh-cw';
  import {
    Badge,
    Button,
    EmptyState,
    Field,
    KeyValue,
    Select,
    Table,
    Td,
    Textarea,
    Th,
    Tr,
    type SelectOption
  } from '$lib/ui/primitives';
  import ConfirmModal from '$lib/ui/ConfirmModal.svelte';
  import { toasts } from '$lib/ui/toastStore';
  import { isErrorToasted } from '$lib/graphql/client';
  import { listTemporalWorkflows, cancelTemporalWorkflow } from './temporalApi';
  import {
    formatDurationMs,
    formatTimestamp,
    workflowStatusLabel,
    workflowStatusTone
  } from './terminologyFormat';
  import type { TemporalWorkflowStatus, ListTemporalWorkflowsQuery } from '$lib/gen/graphql';

  // Use the actual type returned from the query
  type WorkflowNode = ListTemporalWorkflowsQuery['temporalWorkflows']['nodes'][number];

  export let workflowType: string | undefined = undefined;
  export let pageSize = 25;

  const dispatch = createEventDispatcher<{
    select: { workflow: WorkflowNode };
    refresh: void;
  }>();

  const statusOptions: SelectOption[] = [
    { value: '', label: 'All statuses' },
    { value: 'RUNNING', label: 'Running' },
    { value: 'COMPLETED', label: 'Completed' },
    { value: 'FAILED', label: 'Failed' },
    { value: 'CANCELED', label: 'Canceled' },
    { value: 'TIMED_OUT', label: 'Timed out' }
  ];

  // Data state
  let workflows: WorkflowNode[] = [];
  let totalCount = 0;
  let loading = true;
  let error: string | null = null;
  let endCursor: string | null = null;
  let hasNextPage = false;

  // Stats (computed from loaded data)
  $: stats = computeStats(workflows);

  // Filter state
  let filterStatus: TemporalWorkflowStatus | '' = '';

  // Action state
  let showCancelModal = false;
  let cancelingId: string | null = null;
  let cancelReason = '';
  let processingIds = new SvelteSet<string>();

  // Selection (details pane): keyed by id + runId, which is unique per row
  let selectedKey: string | null = null;

  $: selected = workflows.find((wf) => rowKey(wf) === selectedKey) ?? null;
  $: selectedItems = selected
    ? [
        { key: 'Workflow id', value: selected.id, mono: true },
        { key: 'Run id', value: selected.runId, mono: true },
        { key: 'Type', value: selected.workflowType },
        { key: 'Task queue', value: selected.taskQueue, mono: true },
        { key: 'Started', value: formatTimestamp(selected.startTime), mono: true },
        { key: 'Closed', value: formatTimestamp(selected.closeTime), mono: true },
        { key: 'Duration', value: formatDurationMs(selected.durationMs), mono: true }
      ]
    : [];

  onMount(() => {
    loadWorkflows();
  });

  function rowKey(wf: WorkflowNode): string {
    return `${wf.id}:${wf.runId}`;
  }

  function keepSelection() {
    if (!workflows.some((wf) => rowKey(wf) === selectedKey)) {
      const first = workflows[0];
      selectedKey = first ? rowKey(first) : null;
    }
  }

  async function loadWorkflows() {
    loading = true;
    error = null;

    try {
      const result = await listTemporalWorkflows(
        {
          workflowType: workflowType ?? null,
          status: filterStatus || null,
          startTimeAfter: null,
          startTimeBefore: null
        },
        pageSize
      );
      workflows = result.nodes;
      totalCount = result.totalCount;
      hasNextPage = result.pageInfo.hasNextPage;
      endCursor = result.pageInfo.endCursor ?? null;
      keepSelection();
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to load workflows';
    } finally {
      loading = false;
    }
  }

  async function loadMore() {
    if (!hasNextPage || !endCursor) return;

    loading = true;

    try {
      const result = await listTemporalWorkflows(
        {
          workflowType: workflowType ?? null,
          status: filterStatus || null,
          startTimeAfter: null,
          startTimeBefore: null
        },
        pageSize,
        endCursor
      );
      workflows = [...workflows, ...result.nodes];
      totalCount = result.totalCount;
      hasNextPage = result.pageInfo.hasNextPage;
      endCursor = result.pageInfo.endCursor ?? null;
    } catch (err) {
      // Global graphqlFetch net already toasts graphql failures (B4 dedupe).
      if (!isErrorToasted(err)) {
        toasts.error(err instanceof Error ? err.message : 'Failed to load more workflows');
      }
    } finally {
      loading = false;
    }
  }

  function applyFilters() {
    loadWorkflows();
  }

  function clearFilters() {
    filterStatus = '';
    loadWorkflows();
  }

  function selectWorkflow(wf: WorkflowNode) {
    selectedKey = rowKey(wf);
    dispatch('select', { workflow: wf });
  }

  function confirmCancel(id: string) {
    cancelingId = id;
    cancelReason = '';
    showCancelModal = true;
  }

  async function handleCancelConfirm() {
    if (!cancelingId) return;

    processingIds.add(cancelingId);

    try {
      await cancelTemporalWorkflow(cancelingId, cancelReason || undefined);
      toasts.success('Workflow cancellation requested');
      dispatch('refresh');
      await loadWorkflows();
    } catch (err) {
      // Global graphqlFetch net already toasts graphql failures (B4 dedupe).
      if (!isErrorToasted(err)) {
        toasts.error(err instanceof Error ? err.message : 'Failed to cancel workflow');
      }
    } finally {
      processingIds.delete(cancelingId);
      showCancelModal = false;
      cancelingId = null;
    }
  }

  function computeStats(wfs: WorkflowNode[]) {
    const counts = {
      running: 0,
      completed: 0,
      failed: 0,
      canceled: 0,
      other: 0
    };

    for (const wf of wfs) {
      switch (wf.status) {
        case 'RUNNING':
          counts.running++;
          break;
        case 'COMPLETED':
          counts.completed++;
          break;
        case 'FAILED':
        case 'TIMED_OUT':
          counts.failed++;
          break;
        case 'CANCELED':
        case 'TERMINATED':
          counts.canceled++;
          break;
        default:
          counts.other++;
      }
    }

    return counts;
  }
</script>

<div class="workflow-list">
  <div class="filters">
    <div class="filter-status">
      <Select bind:value={filterStatus} options={statusOptions} aria-label="Status" />
    </div>
    <Button variant="ghost" onclick={clearFilters} disabled={!filterStatus}>Clear</Button>
    <Button onclick={applyFilters}>Apply</Button>

    <div class="filters-end">
      <!-- Counts cover the loaded page only; they mean nothing after a failed load. -->
      {#if !error}
        <dl class="stats" aria-label="Loaded workflows by status">
          <div class="stat">
            <dt>Running</dt>
            <dd class="text-mono">{stats.running}</dd>
          </div>
          <div class="stat">
            <dt>Completed</dt>
            <dd class="text-mono">{stats.completed}</dd>
          </div>
          <div class="stat">
            <dt>Failed</dt>
            <dd class="text-mono">{stats.failed}</dd>
          </div>
          <div class="stat">
            <dt>Canceled</dt>
            <dd class="text-mono">{stats.canceled}</dd>
          </div>
        </dl>
      {/if}
      <Button variant="ghost" icon={RefreshCw} onclick={loadWorkflows}>Refresh</Button>
    </div>
  </div>

  {#if loading && workflows.length === 0}
    <EmptyState message="Loading workflows…" aria-busy="true" />
  {:else if error}
    <EmptyState icon={CircleAlert} message="Workflows could not be loaded: {error}">
      {#snippet action()}
        <Button onclick={loadWorkflows}>Retry</Button>
      {/snippet}
    </EmptyState>
  {:else if workflows.length === 0}
    <EmptyState
      icon={Inbox}
      message={filterStatus
        ? 'No workflows match this status.'
        : 'No workflows yet. They appear here when a terminology review starts.'}
    />
  {:else}
    <div class="split">
      <div class="list">
        <Table label="Workflows" layout="fixed" class="workflow-table">
          {#snippet head()}
            <tr>
              <Th>Workflow id</Th>
              <Th width="200px">Type</Th>
              <Th width="120px">Status</Th>
              <Th width="136px">Started</Th>
              <Th width="136px">Closed</Th>
              <Th width="96px" numeric>Duration</Th>
            </tr>
          {/snippet}
          {#each workflows as wf (wf.id + wf.runId)}
            <Tr
              selectable
              selected={rowKey(wf) === selectedKey}
              onselect={() => selectWorkflow(wf)}
              aria-label="Workflow {wf.id}"
            >
              <Td mono truncate value={wf.id} />
              <Td muted truncate value={wf.workflowType} />
              <Td>
                <Badge tone={workflowStatusTone(wf.status)} dot
                  >{workflowStatusLabel(wf.status)}</Badge
                >
              </Td>
              <Td mono muted value={formatTimestamp(wf.startTime)} />
              <Td mono muted value={formatTimestamp(wf.closeTime) || '—'} />
              <Td numeric value={formatDurationMs(wf.durationMs) || '—'} />
            </Tr>
          {/each}
        </Table>

        <div class="pagination">
          <span class="pagination-info text-mono">
            {workflows.length} of {totalCount} workflows
          </span>
          {#if hasNextPage}
            <Button variant="ghost" onclick={loadMore} loading={loading}>Load more</Button>
          {/if}
        </div>
      </div>

      <aside class="details" aria-label="Selected workflow">
        {#if selected}
          {@const target = selected}
          {@const isProcessing = processingIds.has(target.id)}
          <div class="details-head">
            <span class="details-title" title={target.workflowType}>{target.workflowType}</span>
            <Badge tone={workflowStatusTone(target.status)} dot
              >{workflowStatusLabel(target.status)}</Badge
            >
          </div>
          {#if target.status === 'RUNNING'}
            <div class="details-actions">
              <Button
                variant="danger"
                icon={Ban}
                loading={isProcessing}
                onclick={() => confirmCancel(target.id)}
              >
                Cancel workflow
              </Button>
            </div>
          {/if}
          <KeyValue items={selectedItems} />
        {:else}
          <EmptyState message="Select a workflow to see its details." />
        {/if}
      </aside>
    </div>
  {/if}
</div>

<!-- Cancel Modal -->
<ConfirmModal
  bind:open={showCancelModal}
  title="Cancel workflow"
  message="Cancel this workflow? This action cannot be undone."
  confirmText="Cancel workflow"
  variant="danger"
  on:confirm={handleCancelConfirm}
>
  <div class="modal-field">
    <Field label="Reason" hint="Optional.">
      <Textarea bind:value={cancelReason} placeholder="Reason for cancellation" rows={2} />
    </Field>
  </div>
</ConfirmModal>

<style>
  .workflow-list {
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

  .filter-status {
    width: 144px;
  }

  .filters-end {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    margin-left: auto;
  }

  .stats {
    display: flex;
    align-items: baseline;
    gap: var(--space-3);
    margin: 0;
    font-size: var(--text-xs);
  }

  .stat {
    display: flex;
    align-items: baseline;
    gap: var(--space-1);
  }

  .stat dt {
    color: var(--color-text-tertiary);
  }

  .stat dd {
    margin: 0;
    color: var(--color-text-primary);
  }

  .split {
    display: grid;
    grid-template-columns: minmax(0, 1fr) 340px;
    flex: 1 1 auto;
    min-height: 0;
  }

  .list {
    display: flex;
    flex-direction: column;
    min-width: 0;
    min-height: 0;
  }

  .list :global(.workflow-table) {
    flex: 1 1 auto;
  }

  /* Fixed layout gives the unsized columns only what is left; keep a floor
     so they never collapse, and let the wrapper scroll sideways instead. */
  .list :global(.workflow-table > table) {
    min-width: 860px;
  }

  .pagination {
    display: flex;
    align-items: center;
    gap: var(--space-1);
    flex: 0 0 auto;
    min-height: 37px;
    padding: var(--space-1) var(--space-3);
    border-top: 1px solid var(--color-border-subtle);
  }

  .pagination-info {
    margin-right: auto;
    color: var(--color-text-tertiary);
  }

  .details {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    min-height: 0;
    overflow: auto;
    padding: var(--space-3);
    border-left: 1px solid var(--color-border-subtle);
    background: var(--color-bg-elevated);
  }

  .details-head {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    min-width: 0;
  }

  .details-title {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: var(--text-ui);
    font-weight: var(--font-semibold);
  }

  .details-actions {
    display: flex;
    gap: var(--space-2);
  }

  .modal-field {
    margin-top: var(--space-3);
  }
</style>
