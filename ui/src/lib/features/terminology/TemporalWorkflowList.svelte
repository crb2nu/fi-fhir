<script lang="ts">
  import { onMount, createEventDispatcher } from 'svelte';
  import { SvelteSet } from 'svelte/reactivity';
  import Button from '$lib/ui/Button.svelte';
  import ConfirmModal from '$lib/ui/ConfirmModal.svelte';
  import { toasts } from '$lib/ui/toastStore';
  import { listTemporalWorkflows, cancelTemporalWorkflow } from './temporalApi';
  import type { TemporalWorkflowStatus, ListTemporalWorkflowsQuery } from '$lib/gen/graphql';

  // Use the actual type returned from the query
  type WorkflowNode = ListTemporalWorkflowsQuery['temporalWorkflows']['nodes'][number];

  export let workflowType: string | undefined = undefined;
  export let pageSize = 25;

  const dispatch = createEventDispatcher<{
    select: { workflow: WorkflowNode };
    refresh: void;
  }>();

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

  // Expanded rows
  let expandedIds = new SvelteSet<string>();

  onMount(() => {
    loadWorkflows();
  });

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
      toasts.error(err instanceof Error ? err.message : 'Failed to load more workflows');
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

  function toggleExpand(id: string) {
    if (expandedIds.has(id)) {
      expandedIds.delete(id);
    } else {
      expandedIds.add(id);
    }
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
      toasts.error(err instanceof Error ? err.message : 'Failed to cancel workflow');
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

  function formatStatus(status: TemporalWorkflowStatus): string {
    switch (status) {
      case 'RUNNING': return 'Running';
      case 'COMPLETED': return 'Completed';
      case 'FAILED': return 'Failed';
      case 'CANCELED': return 'Canceled';
      case 'TERMINATED': return 'Terminated';
      case 'TIMED_OUT': return 'Timed Out';
      case 'CONTINUED_AS_NEW': return 'Continued';
      default: return String(status);
    }
  }

  function getStatusClass(status: TemporalWorkflowStatus): string {
    switch (status) {
      case 'RUNNING': return 'status-running';
      case 'COMPLETED': return 'status-completed';
      case 'FAILED':
      case 'TIMED_OUT': return 'status-failed';
      case 'CANCELED':
      case 'TERMINATED': return 'status-canceled';
      default: return 'status-other';
    }
  }

  function formatDate(dateStr: string): string {
    const date = new Date(dateStr);
    return date.toLocaleDateString('en-US', {
      month: 'short',
      day: 'numeric',
      hour: '2-digit',
      minute: '2-digit'
    });
  }

  function formatDuration(ms: number | null | undefined): string {
    if (ms == null) return '—';
    if (ms < 1000) return `${ms}ms`;
    if (ms < 60000) return `${(ms / 1000).toFixed(1)}s`;
    const mins = Math.floor(ms / 60000);
    const secs = Math.floor((ms % 60000) / 1000);
    return `${mins}m ${secs}s`;
  }

  function truncateId(id: string): string {
    if (id.length <= 20) return id;
    return id.substring(0, 8) + '...' + id.substring(id.length - 8);
  }
</script>

<div class="workflow-list">
  <!-- Stats Banner -->
  <div class="stats-banner">
    <div class="stat">
      <span class="stat-value running">{stats.running}</span>
      <span class="stat-label">Running</span>
    </div>
    <div class="stat">
      <span class="stat-value completed">{stats.completed}</span>
      <span class="stat-label">Completed</span>
    </div>
    <div class="stat">
      <span class="stat-value failed">{stats.failed}</span>
      <span class="stat-label">Failed</span>
    </div>
    <div class="stat">
      <span class="stat-value canceled">{stats.canceled}</span>
      <span class="stat-label">Canceled</span>
    </div>
    <div class="stat-action">
      <Button variant="secondary" size="sm" on:click={loadWorkflows}>
        Refresh
      </Button>
    </div>
  </div>

  <!-- Filters -->
  <div class="filters">
    <div class="filter-row">
      <label class="filter-field filter-sm">
        <span class="filter-label">Status</span>
        <select class="filter-select" bind:value={filterStatus}>
          <option value="">All</option>
          <option value="RUNNING">Running</option>
          <option value="COMPLETED">Completed</option>
          <option value="FAILED">Failed</option>
          <option value="CANCELED">Canceled</option>
          <option value="TIMED_OUT">Timed Out</option>
        </select>
      </label>
      <div class="filter-actions">
        <Button variant="secondary" size="sm" on:click={clearFilters}>Clear</Button>
        <Button variant="primary" size="sm" on:click={applyFilters}>Apply</Button>
      </div>
    </div>
  </div>

  <!-- Content -->
  {#if loading && workflows.length === 0}
    <div class="loading">Loading workflows...</div>
  {:else if error}
    <div class="error-state">
      <div class="error-message">{error}</div>
      <Button variant="secondary" size="sm" on:click={loadWorkflows}>Retry</Button>
    </div>
  {:else if workflows.length === 0}
    <div class="empty-state">
      <div class="empty-icon">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
          <path d="M9 3v2m6-2v2M9 19v2m6-2v2M5 9H3m2 6H3m18-6h-2m2 6h-2M7 19h10a2 2 0 002-2V7a2 2 0 00-2-2H7a2 2 0 00-2 2v10a2 2 0 002 2z" />
        </svg>
      </div>
      <div class="empty-text">No workflows found</div>
      <div class="empty-hint">
        {#if filterStatus}
          Try adjusting your filters
        {:else}
          Workflows will appear here when terminology reviews are started
        {/if}
      </div>
    </div>
  {:else}
    <div class="cards">
      {#each workflows as wf (wf.id + wf.runId)}
        {@const isExpanded = expandedIds.has(wf.id)}
        {@const isProcessing = processingIds.has(wf.id)}
        <div class="card" class:expanded={isExpanded}>
          <div class="card-header">
            <div class="card-main">
              <div class="workflow-id" title={wf.id}>{truncateId(wf.id)}</div>
              <div class="workflow-type">{wf.workflowType}</div>
            </div>
            <div class="card-status">
              <span class="status-badge {getStatusClass(wf.status)}">
                {formatStatus(wf.status)}
              </span>
            </div>
          </div>

          <div class="card-meta">
            <span class="meta-item">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <circle cx="12" cy="12" r="10" />
                <polyline points="12 6 12 12 16 14" />
              </svg>
              {formatDate(wf.startTime)}
            </span>
            {#if wf.closeTime}
              <span class="meta-item">
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                  <path d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
                </svg>
                {formatDate(wf.closeTime)}
              </span>
            {/if}
            <span class="meta-item">
              Duration: {formatDuration(wf.durationMs)}
            </span>
            <button
              type="button"
              class="expand-btn"
              on:click={() => toggleExpand(wf.id)}
              title={isExpanded ? 'Collapse details' : 'Expand details'}
              aria-label={isExpanded ? 'Collapse workflow details' : 'Expand workflow details'}
              aria-expanded={isExpanded}
              aria-controls={`workflow-details-${wf.id}`}
            >
              <svg
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                stroke-width="2"
                class:rotated={isExpanded}
              >
                <path d="M19 9l-7 7-7-7" />
              </svg>
            </button>
          </div>

          {#if isExpanded}
            <div class="card-details" id={`workflow-details-${wf.id}`}>
              <div class="detail-row">
                <span class="detail-label">Workflow ID:</span>
                <span class="detail-value mono">{wf.id}</span>
              </div>
              <div class="detail-row">
                <span class="detail-label">Run ID:</span>
                <span class="detail-value mono">{wf.runId}</span>
              </div>
              <div class="detail-row">
                <span class="detail-label">Task Queue:</span>
                <span class="detail-value">{wf.taskQueue}</span>
              </div>
            </div>
          {/if}

          {#if wf.status === 'RUNNING'}
            <div class="card-actions">
              <Button
                variant="secondary"
                size="sm"
                disabled={isProcessing}
                on:click={() => confirmCancel(wf.id)}
              >
                {isProcessing ? 'Canceling...' : 'Cancel Workflow'}
              </Button>
            </div>
          {/if}
        </div>
      {/each}
    </div>

    <!-- Load More / Info -->
    <div class="pagination">
      <div class="pagination-info">
        Showing {workflows.length} of {totalCount} workflows
      </div>
      {#if hasNextPage}
        <Button variant="secondary" size="sm" on:click={loadMore} disabled={loading}>
          {loading ? 'Loading...' : 'Load More'}
        </Button>
      {/if}
    </div>
  {/if}
</div>

<!-- Cancel Modal -->
<ConfirmModal
  bind:open={showCancelModal}
  title="Cancel Workflow"
  message="Are you sure you want to cancel this workflow? This action cannot be undone."
  confirmText="Cancel Workflow"
  variant="danger"
  on:confirm={handleCancelConfirm}
>
  <div class="cancel-reason-input">
    <textarea
      bind:value={cancelReason}
      placeholder="Optional: reason for cancellation"
      rows="2"
    ></textarea>
  </div>
</ConfirmModal>

<style>
  .workflow-list {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
  }

  /* Stats Banner */
  .stats-banner {
    display: flex;
    gap: var(--space-6);
    padding: var(--space-4) var(--space-5);
    border-radius: var(--radius-lg);
    background: var(--color-bg-surface);
    border: 1px solid var(--color-border-subtle);
    align-items: center;
  }

  .stat {
    display: flex;
    flex-direction: column;
    gap: var(--space-0);
  }

  .stat-value {
    font-size: var(--text-2xl);
    font-weight: var(--font-semibold);
    color: var(--color-text-primary);
  }

  .stat-value.running { color: var(--color-warning); }
  .stat-value.completed { color: var(--color-success); }
  .stat-value.failed { color: var(--color-danger); }
  .stat-value.canceled { color: var(--color-text-muted); }

  .stat-label {
    font-size: var(--text-xs);
    color: var(--color-text-muted);
    text-transform: uppercase;
    letter-spacing: var(--tracking-wide);
  }

  .stat-action {
    margin-left: auto;
  }

  /* Filters */
  .filters {
    padding: var(--space-3) var(--space-4);
    border-radius: var(--radius-lg);
    background: var(--color-bg-surface);
    border: 1px solid var(--color-border-subtle);
  }

  .filter-row {
    display: flex;
    gap: var(--space-3);
    align-items: flex-end;
  }

  .filter-field {
    flex: 1;
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
  }

  .filter-field.filter-sm {
    flex: 0.4;
  }

  .filter-label {
    font-size: var(--text-xs);
    font-weight: var(--font-semibold);
    color: var(--color-text-tertiary);
    text-transform: uppercase;
    letter-spacing: var(--tracking-wide);
  }

  .filter-select {
    padding: var(--space-2) var(--space-3);
    border-radius: var(--radius-lg);
    border: 1px solid var(--color-border-default);
    background: var(--color-bg-input);
    color: var(--color-text-primary);
    font-size: var(--text-sm);
    outline: none;
    transition: var(--transition-all);
  }

  .filter-select:focus {
    border-color: var(--color-border-focus);
    box-shadow: var(--shadow-focus);
  }

  .filter-actions {
    display: flex;
    gap: var(--space-2);
    margin-left: auto;
  }

  /* Cards */
  .cards {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
  }

  .card {
    padding: var(--space-4);
    border-radius: var(--radius-lg);
    background: var(--color-bg-surface);
    border: 1px solid var(--color-border-subtle);
    transition: var(--transition-all);
  }

  .card:hover {
    border-color: var(--color-border-hover);
    transform: translateY(-2px);
    box-shadow: var(--shadow-lg);
  }

  .card.expanded {
    border-color: var(--color-primary-muted);
  }

  .card-header {
    display: flex;
    justify-content: space-between;
    align-items: flex-start;
    gap: var(--space-4);
  }

  .card-main {
    display: flex;
    flex-direction: column;
    gap: var(--space-0);
  }

  .workflow-id {
    font-family: var(--font-mono);
    font-size: var(--text-sm);
    color: var(--color-text-primary);
  }

  .workflow-type {
    font-size: var(--text-sm);
    color: var(--color-text-muted);
  }

  .status-badge {
    padding: var(--space-1) var(--space-2);
    border-radius: var(--radius-md);
    font-size: var(--text-xs);
    font-weight: var(--font-semibold);
  }

  .status-running {
    color: var(--color-warning);
    background: var(--color-warning-subtle);
    position: relative;
  }

  .status-running::before {
    content: '';
    position: absolute;
    left: 6px;
    top: 50%;
    transform: translateY(-50%);
    width: 6px;
    height: 6px;
    border-radius: var(--radius-full);
    background: var(--color-warning);
  }

  .status-running {
    padding-left: var(--space-5);
  }

  .status-completed {
    color: var(--color-success);
    background: var(--color-success-subtle);
  }

  .status-failed {
    color: var(--color-danger);
    background: var(--color-danger-subtle);
  }

  .status-canceled {
    color: var(--color-text-tertiary);
    background: var(--color-bg-elevated);
  }

  .status-other {
    color: var(--color-text-tertiary);
    background: var(--color-bg-elevated);
  }

  .card-meta {
    display: flex;
    gap: var(--space-4);
    align-items: center;
    margin-top: var(--space-3);
    flex-wrap: wrap;
  }

  .meta-item {
    display: flex;
    align-items: center;
    gap: var(--space-1);
    font-size: var(--text-xs);
    color: var(--color-text-tertiary);
  }

  .meta-item svg {
    width: 14px;
    height: 14px;
  }

  .expand-btn {
    width: 28px;
    height: 28px;
    display: flex;
    align-items: center;
    justify-content: center;
    border-radius: var(--radius-md);
    border: none;
    background: transparent;
    color: var(--color-text-muted);
    cursor: pointer;
    margin-left: auto;
    transition: var(--transition-all);
  }

  .expand-btn svg {
    width: 16px;
    height: 16px;
    transition: transform var(--duration-normal) var(--ease-out);
  }

  .expand-btn svg.rotated {
    transform: rotate(180deg);
  }

  .expand-btn:hover {
    color: var(--color-text-primary);
    background: var(--color-bg-hover);
  }

  .card-details {
    margin-top: var(--space-3);
    padding: var(--space-3);
    border-radius: var(--radius-md);
    background: var(--color-bg-inset);
  }

  .detail-row {
    display: flex;
    gap: var(--space-3);
    padding: var(--space-1) 0;
    font-size: var(--text-xs);
  }

  .detail-label {
    color: var(--color-text-muted);
    min-width: 100px;
  }

  .detail-value {
    color: var(--color-text-secondary);
  }

  .detail-value.mono {
    font-family: var(--font-mono);
    font-size: var(--text-2xs);
    word-break: break-all;
  }

  .card-actions {
    display: flex;
    justify-content: flex-end;
    gap: var(--space-2);
    margin-top: var(--space-4);
    padding-top: var(--space-3);
    border-top: 1px solid var(--color-border-subtle);
  }

  /* Pagination */
  .pagination {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: var(--space-2) 0;
  }

  .pagination-info {
    font-size: var(--text-sm);
    color: var(--color-text-tertiary);
  }

  /* States */
  .loading {
    padding: var(--space-12);
    text-align: center;
    color: var(--color-text-tertiary);
  }

  .error-state {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: var(--space-3);
    padding: var(--space-12);
  }

  .error-message {
    color: var(--color-danger);
    font-size: var(--text-sm);
  }

  .empty-state {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: var(--space-2);
    padding: var(--space-12);
  }

  .empty-icon {
    width: 48px;
    height: 48px;
    color: var(--color-text-muted);
    opacity: 0.5;
  }

  .empty-icon svg {
    width: 100%;
    height: 100%;
  }

  .empty-text {
    color: var(--color-text-secondary);
    font-size: var(--text-base);
  }

  .empty-hint {
    color: var(--color-text-muted);
    font-size: var(--text-sm);
  }

  /* Modal Content */
  .cancel-reason-input textarea {
    width: 100%;
    padding: var(--space-2) var(--space-3);
    border-radius: var(--radius-lg);
    border: 1px solid var(--color-border-default);
    background: var(--color-bg-input);
    color: var(--color-text-primary);
    font-size: var(--text-sm);
    font-family: inherit;
    resize: vertical;
    outline: none;
    margin-top: var(--space-3);
    transition: var(--transition-all);
  }

  .cancel-reason-input textarea:focus {
    border-color: var(--color-border-focus);
    box-shadow: var(--shadow-focus);
  }
</style>
