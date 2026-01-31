<script lang="ts">
  import { onMount, createEventDispatcher } from 'svelte';
  import { SvelteSet } from 'svelte/reactivity';
  import Button from '$lib/ui/Button.svelte';
  import ConfirmModal from '$lib/ui/ConfirmModal.svelte';
  import { toasts } from '$lib/ui/toastStore';
  import {
    listPendingAutoroutes,
    getPendingAutorouteStats,
    approvePendingAutoroute,
    rejectPendingAutoroute,
    bulkApprovePendingAutoroutes
  } from './terminologyApi';
  import type {
    PendingAutorouteStatus,
    MappingEquivalence,
    ListPendingAutoroutesQuery
  } from '$lib/gen/graphql';

  // Use the actual type returned from the query
  type PendingNode = ListPendingAutoroutesQuery['listPendingAutoroutes']['nodes'][number];

  export let sourceSystem: string | undefined = undefined;
  export let targetSystem: string | undefined = undefined;
  export let pageSize = 25;

  const dispatch = createEventDispatcher<{
    approve: { id: string; mappingId: string };
    reject: { id: string };
    refresh: void;
  }>();

  // Data state
  let pending: PendingNode[] = [];
  let totalCount = 0;
  let offset = 0;
  let loading = true;
  let error: string | null = null;

  // Stats
  let stats: {
    pendingCount: number;
    approvedCount: number;
    rejectedCount: number;
    expiredCount: number;
    avgConfidence: number | null;
  } | null = null;

  // Filter state
  let filterStatus: PendingAutorouteStatus | '' = 'PENDING';
  let filterMinConfidence = '';
  let filterSourceSystem = sourceSystem ?? '';
  let filterTargetSystem = targetSystem ?? '';

  // Action state
  let showRejectModal = false;
  let showBulkApproveModal = false;
  let rejectingId: string | null = null;
  let rejectReason = '';
  let bulkMinConfidence = 0.95;
  let processingIds = new SvelteSet<string>();

  // Expanded rows for showing alternates/trace
  let expandedIds = new SvelteSet<string>();

  onMount(() => {
    loadPending();
    loadStats();
  });

  async function loadPending() {
    loading = true;
    error = null;

    try {
      const result = await listPendingAutoroutes({
        first: pageSize,
        offset,
        status: filterStatus || null,
        minConfidence: filterMinConfidence ? parseFloat(filterMinConfidence) : null,
        sourceSystem: filterSourceSystem || null,
        targetSystem: filterTargetSystem || null
      });
      pending = result.nodes;
      totalCount = result.totalCount;
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to load pending autoroutes';
      toasts.error(error);
    } finally {
      loading = false;
    }
  }

  async function loadStats() {
    try {
      stats = await getPendingAutorouteStats();
    } catch {
      // Stats are optional, don't show error
    }
  }

  function applyFilters() {
    offset = 0;
    loadPending();
  }

  function clearFilters() {
    filterStatus = 'PENDING';
    filterMinConfidence = '';
    filterSourceSystem = '';
    filterTargetSystem = '';
    offset = 0;
    loadPending();
  }

  function prevPage() {
    if (offset > 0) {
      offset = Math.max(0, offset - pageSize);
      loadPending();
    }
  }

  function nextPage() {
    if (offset + pageSize < totalCount) {
      offset += pageSize;
      loadPending();
    }
  }

  function toggleExpand(id: string) {
    if (expandedIds.has(id)) {
      expandedIds.delete(id);
    } else {
      expandedIds.add(id);
    }
  }

  async function handleApprove(item: PendingNode, equivalence?: MappingEquivalence) {
    processingIds.add(item.id);

    try {
      const mapping = await approvePendingAutoroute({
        id: item.id,
        equivalence: equivalence ?? null,
        comment: null
      });
      toasts.success(`Approved mapping: ${item.sourceCode} → ${item.suggestedCode}`);
      dispatch('approve', { id: item.id, mappingId: mapping.id });
      await loadPending();
      await loadStats();
    } catch (err) {
      toasts.error(err instanceof Error ? err.message : 'Approval failed');
    } finally {
      processingIds.delete(item.id);
    }
  }

  function confirmReject(id: string) {
    rejectingId = id;
    rejectReason = '';
    showRejectModal = true;
  }

  async function handleRejectConfirm() {
    if (!rejectingId) return;

    processingIds.add(rejectingId);

    try {
      await rejectPendingAutoroute({
        id: rejectingId,
        reason: rejectReason || 'Rejected by reviewer'
      });
      toasts.success('Suggestion rejected');
      dispatch('reject', { id: rejectingId });
      await loadPending();
      await loadStats();
    } catch (err) {
      toasts.error(err instanceof Error ? err.message : 'Rejection failed');
    } finally {
      processingIds.delete(rejectingId!);
      showRejectModal = false;
      rejectingId = null;
    }
  }

  function openBulkApprove() {
    showBulkApproveModal = true;
  }

  async function handleBulkApprove() {
    try {
      const result = await bulkApprovePendingAutoroutes({
        minConfidence: bulkMinConfidence,
        maxCount: 100
      });
      toasts.success(`Approved ${result.approved} mappings (${result.skipped} skipped)`);
      dispatch('refresh');
      await loadPending();
      await loadStats();
    } catch (err) {
      toasts.error(err instanceof Error ? err.message : 'Bulk approval failed');
    } finally {
      showBulkApproveModal = false;
    }
  }

  function formatConfidence(confidence: number): string {
    return `${(confidence * 100).toFixed(1)}%`;
  }

  function getConfidenceClass(confidence: number): string {
    if (confidence >= 0.9) return 'conf-high';
    if (confidence >= 0.7) return 'conf-med';
    if (confidence >= 0.5) return 'conf-low';
    return 'conf-none';
  }

  function formatStatus(status: PendingAutorouteStatus): string {
    switch (status) {
      case 'PENDING': return 'Pending';
      case 'APPROVED': return 'Approved';
      case 'REJECTED': return 'Rejected';
      case 'EXPIRED': return 'Expired';
      default: return String(status);
    }
  }

  function formatEquivalence(eq: MappingEquivalence | null | undefined): string {
    if (!eq) return '—';
    switch (eq) {
      case 'EQUIVALENT': return 'Equivalent';
      case 'WIDER': return 'Wider';
      case 'NARROWER': return 'Narrower';
      case 'INEXACT': return 'Inexact';
      default: return String(eq);
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

  function truncateSystem(system: string): string {
    if (!system) return '';
    if (system.startsWith('http://')) {
      const rest = system.replace('http://', '');
      return rest.split('/')[0] ?? rest;
    }
    if (system.startsWith('https://')) {
      const rest = system.replace('https://', '');
      return rest.split('/')[0] ?? rest;
    }
    return system.length > 20 ? system.substring(0, 17) + '...' : system;
  }
</script>

<div class="review-list">
  <!-- Stats Banner -->
  {#if stats}
    <div class="stats-banner">
      <div class="stat">
        <span class="stat-value pending">{stats.pendingCount}</span>
        <span class="stat-label">Pending</span>
      </div>
      <div class="stat">
        <span class="stat-value approved">{stats.approvedCount}</span>
        <span class="stat-label">Approved</span>
      </div>
      <div class="stat">
        <span class="stat-value rejected">{stats.rejectedCount}</span>
        <span class="stat-label">Rejected</span>
      </div>
      <div class="stat">
        <span class="stat-value expired">{stats.expiredCount}</span>
        <span class="stat-label">Expired</span>
      </div>
      {#if stats.avgConfidence}
        <div class="stat">
          <span class="stat-value">{formatConfidence(stats.avgConfidence)}</span>
          <span class="stat-label">Avg Confidence</span>
        </div>
      {/if}
      <div class="stat-action">
        <Button variant="primary" size="sm" on:click={openBulkApprove}>
          Bulk Approve
        </Button>
      </div>
    </div>
  {/if}

  <!-- Filters -->
  <div class="filters">
    <div class="filter-row">
      <label class="filter-field filter-sm">
        <span class="filter-label">Status</span>
        <select class="filter-select" bind:value={filterStatus}>
          <option value="">All</option>
          <option value="PENDING">Pending</option>
          <option value="APPROVED">Approved</option>
          <option value="REJECTED">Rejected</option>
          <option value="EXPIRED">Expired</option>
        </select>
      </label>
      <label class="filter-field filter-sm">
        <span class="filter-label">Min Confidence</span>
        <input
          type="number"
          class="filter-input"
          bind:value={filterMinConfidence}
          placeholder="e.g., 0.7"
          min="0"
          max="1"
          step="0.05"
        />
      </label>
      <label class="filter-field">
        <span class="filter-label">Source System</span>
        <input
          type="text"
          class="filter-input"
          bind:value={filterSourceSystem}
          placeholder="e.g., epic_labs"
          on:keydown={(e) => e.key === 'Enter' && applyFilters()}
        />
      </label>
      <label class="filter-field">
        <span class="filter-label">Target System</span>
        <input
          type="text"
          class="filter-input"
          bind:value={filterTargetSystem}
          placeholder="e.g., http://loinc.org"
          on:keydown={(e) => e.key === 'Enter' && applyFilters()}
        />
      </label>
      <div class="filter-actions">
        <Button variant="secondary" size="sm" on:click={clearFilters}>Clear</Button>
        <Button variant="primary" size="sm" on:click={applyFilters}>Apply</Button>
      </div>
    </div>
  </div>

  <!-- Content -->
  {#if loading}
    <div class="loading">Loading pending suggestions...</div>
  {:else if error}
    <div class="error-state">
      <div class="error-message">{error}</div>
      <Button variant="secondary" size="sm" on:click={loadPending}>Retry</Button>
    </div>
  {:else if pending.length === 0}
    <div class="empty-state">
      <div class="empty-icon">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
          <path d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
        </svg>
      </div>
      <div class="empty-text">No pending suggestions</div>
      <div class="empty-hint">
        {#if filterStatus !== 'PENDING'}
          Try filtering by "Pending" status
        {:else}
          All suggestions have been reviewed
        {/if}
      </div>
    </div>
  {:else}
    <div class="cards">
      {#each pending as item (item.id)}
        {@const isExpanded = expandedIds.has(item.id)}
        {@const isProcessing = processingIds.has(item.id)}
        <div class="card" class:expanded={isExpanded}>
          <div class="card-header">
            <div class="card-source">
              <span class="system-label">{truncateSystem(item.sourceSystem)}</span>
              <span class="code-value">{item.sourceCode}</span>
              {#if item.sourceDisplay}
                <span class="display-value">{item.sourceDisplay}</span>
              {/if}
            </div>
            <div class="card-arrow">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M14 5l7 7m0 0l-7 7m7-7H3" />
              </svg>
            </div>
            <div class="card-target">
              <span class="system-label">{truncateSystem(item.targetSystem)}</span>
              <span class="code-value suggested">{item.suggestedCode}</span>
              {#if item.suggestedDisplay}
                <span class="display-value">{item.suggestedDisplay}</span>
              {/if}
            </div>
          </div>

          <div class="card-meta">
            <span class="confidence-badge {getConfidenceClass(item.confidence)}">
              {formatConfidence(item.confidence)}
            </span>
            {#if item.equivalence}
              <span class="equiv-badge equiv-{item.equivalence.toLowerCase()}">
                {formatEquivalence(item.equivalence)}
              </span>
            {/if}
            <span class="status-badge status-{item.status.toLowerCase()}">
              {formatStatus(item.status)}
            </span>
            <span class="date-label">{formatDate(item.createdAt)}</span>
            <button
              class="expand-btn"
              on:click={() => toggleExpand(item.id)}
              title={isExpanded ? 'Collapse' : 'Expand'}
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

          {#if item.reasoning}
            <div class="card-reasoning">
              <strong>Reasoning:</strong> {item.reasoning}
            </div>
          {/if}

          {#if isExpanded}
            <div class="card-details">
              {#if item.alternates && item.alternates.length > 0}
                <div class="alternates">
                  <div class="detail-heading">Alternatives Considered</div>
                  {#each item.alternates as alt (alt.code)}
                    <div class="alternate-row">
                      <span class="alt-code">{alt.code}</span>
                      {#if alt.display}
                        <span class="alt-display">{alt.display}</span>
                      {/if}
                      <span class="alt-conf">{formatConfidence(alt.confidence)}</span>
                    </div>
                  {/each}
                </div>
              {/if}

              {#if item.decisionTrace}
                <div class="trace">
                  <div class="detail-heading">
                    Decision Trace ({item.decisionTrace.totalDurationMs}ms)
                  </div>
                  {#each item.decisionTrace.steps as step, idx (idx)}
                    <div class="trace-step">
                      <span class="step-name">{step.step}</span>
                      <span class="step-result">{step.result}</span>
                      <span class="step-duration">{step.durationMs}ms</span>
                    </div>
                  {/each}
                </div>
              {/if}
            </div>
          {/if}

          {#if item.status === 'PENDING'}
            <div class="card-actions">
              <Button
                variant="secondary"
                size="sm"
                disabled={isProcessing}
                on:click={() => confirmReject(item.id)}
              >
                Reject
              </Button>
              <Button
                variant="primary"
                size="sm"
                disabled={isProcessing}
                on:click={() => handleApprove(item)}
              >
                {isProcessing ? 'Processing...' : 'Approve'}
              </Button>
            </div>
          {:else if item.status === 'REJECTED' && item.rejectionReason}
            <div class="rejection-reason">
              <strong>Reason:</strong> {item.rejectionReason}
            </div>
          {/if}
        </div>
      {/each}
    </div>

    <!-- Pagination -->
    <div class="pagination">
      <div class="pagination-info">
        Showing {offset + 1}–{Math.min(offset + pending.length, totalCount)} of {totalCount}
      </div>
      <div class="pagination-controls">
        <button class="page-btn" on:click={prevPage} disabled={offset === 0}>
          Previous
        </button>
        <button class="page-btn" on:click={nextPage} disabled={offset + pageSize >= totalCount}>
          Next
        </button>
      </div>
    </div>
  {/if}
</div>

<!-- Reject Modal -->
<ConfirmModal
  bind:open={showRejectModal}
  title="Reject Suggestion"
  message="Please provide a reason for rejecting this mapping suggestion."
  confirmText="Reject"
  variant="danger"
  on:confirm={handleRejectConfirm}
>
  <div class="reject-reason-input">
    <textarea
      bind:value={rejectReason}
      placeholder="e.g., Incorrect mapping - codes are not semantically equivalent"
      rows="3"
    ></textarea>
  </div>
</ConfirmModal>

<!-- Bulk Approve Modal -->
<ConfirmModal
  bind:open={showBulkApproveModal}
  title="Bulk Approve High-Confidence Suggestions"
  message="Approve all pending suggestions above the confidence threshold."
  confirmText="Approve All"
  variant="primary"
  on:confirm={handleBulkApprove}
>
  <div class="bulk-config">
    <label>
      <span class="bulk-label">Minimum Confidence</span>
      <input
        type="range"
        min="0.7"
        max="0.99"
        step="0.01"
        bind:value={bulkMinConfidence}
      />
      <span class="bulk-value">{formatConfidence(bulkMinConfidence)}</span>
    </label>
    <p class="bulk-hint">
      Only suggestions with confidence ≥ {formatConfidence(bulkMinConfidence)} will be approved.
    </p>
  </div>
</ConfirmModal>

<style>
  .review-list {
    display: flex;
    flex-direction: column;
    gap: 16px;
  }

  /* Stats Banner */
  .stats-banner {
    display: flex;
    gap: 24px;
    padding: 16px 20px;
    border-radius: 8px;
    background: rgba(255, 255, 255, 0.02);
    border: 1px solid rgba(255, 255, 255, 0.06);
    align-items: center;
  }

  .stat {
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  .stat-value {
    font-size: 1.5rem;
    font-weight: 600;
    color: rgba(229, 231, 235, 0.9);
  }

  .stat-value.pending { color: rgba(251, 191, 36, 0.9); }
  .stat-value.approved { color: rgba(34, 197, 94, 0.9); }
  .stat-value.rejected { color: rgba(239, 68, 68, 0.9); }
  .stat-value.expired { color: rgba(229, 231, 235, 0.5); }

  .stat-label {
    font-size: 0.75rem;
    color: rgba(229, 231, 235, 0.5);
    text-transform: uppercase;
    letter-spacing: 0.02em;
  }

  .stat-action {
    margin-left: auto;
  }

  /* Filters */
  .filters {
    padding: 12px 16px;
    border-radius: 8px;
    background: rgba(255, 255, 255, 0.02);
    border: 1px solid rgba(255, 255, 255, 0.06);
  }

  .filter-row {
    display: flex;
    gap: 12px;
    align-items: flex-end;
  }

  .filter-field {
    flex: 1;
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .filter-field.filter-sm {
    flex: 0.6;
  }

  .filter-label {
    font-size: 0.75rem;
    font-weight: 600;
    color: rgba(229, 231, 235, 0.6);
    text-transform: uppercase;
    letter-spacing: 0.02em;
  }

  .filter-input,
  .filter-select {
    padding: 8px 12px;
    border-radius: 8px;
    border: 1px solid rgba(255, 255, 255, 0.1);
    background: rgba(255, 255, 255, 0.03);
    color: rgba(229, 231, 235, 0.9);
    font-size: 0.9rem;
    outline: none;
  }

  .filter-input:focus,
  .filter-select:focus {
    border-color: rgba(59, 130, 246, 0.5);
    box-shadow: 0 0 0 2px rgba(59, 130, 246, 0.15);
  }

  .filter-actions {
    display: flex;
    gap: 8px;
  }

  /* Cards */
  .cards {
    display: flex;
    flex-direction: column;
    gap: 12px;
  }

  .card {
    padding: 16px;
    border-radius: 8px;
    background: rgba(255, 255, 255, 0.02);
    border: 1px solid rgba(255, 255, 255, 0.06);
    transition: border-color 0.1s;
  }

  .card:hover {
    border-color: rgba(255, 255, 255, 0.12);
  }

  .card.expanded {
    border-color: rgba(59, 130, 246, 0.3);
  }

  .card-header {
    display: flex;
    align-items: center;
    gap: 16px;
  }

  .card-source,
  .card-target {
    flex: 1;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  .card-arrow {
    width: 24px;
    height: 24px;
    color: rgba(229, 231, 235, 0.3);
  }

  .card-arrow svg {
    width: 100%;
    height: 100%;
  }

  .system-label {
    font-size: 0.75rem;
    color: rgba(229, 231, 235, 0.5);
  }

  .code-value {
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
    font-size: 1rem;
    color: rgba(229, 231, 235, 0.9);
  }

  .code-value.suggested {
    color: rgba(59, 130, 246, 0.9);
  }

  .display-value {
    font-size: 0.85rem;
    color: rgba(229, 231, 235, 0.6);
  }

  .card-meta {
    display: flex;
    gap: 8px;
    align-items: center;
    margin-top: 12px;
  }

  .confidence-badge {
    padding: 2px 8px;
    border-radius: 4px;
    font-size: 0.8rem;
    font-weight: 600;
  }

  .conf-high { color: rgba(34, 197, 94, 0.9); background: rgba(34, 197, 94, 0.1); }
  .conf-med { color: rgba(251, 191, 36, 0.9); background: rgba(251, 191, 36, 0.1); }
  .conf-low { color: rgba(251, 146, 60, 0.9); background: rgba(251, 146, 60, 0.1); }
  .conf-none { color: rgba(239, 68, 68, 0.9); background: rgba(239, 68, 68, 0.1); }

  .equiv-badge {
    padding: 2px 8px;
    border-radius: 4px;
    font-size: 0.75rem;
    font-weight: 500;
  }

  .equiv-equivalent { color: rgba(34, 197, 94, 0.9); background: rgba(34, 197, 94, 0.1); }
  .equiv-wider { color: rgba(59, 130, 246, 0.9); background: rgba(59, 130, 246, 0.1); }
  .equiv-narrower { color: rgba(251, 146, 60, 0.9); background: rgba(251, 146, 60, 0.1); }
  .equiv-inexact { color: rgba(229, 231, 235, 0.7); background: rgba(229, 231, 235, 0.1); }

  .status-badge {
    padding: 2px 8px;
    border-radius: 4px;
    font-size: 0.75rem;
    font-weight: 500;
  }

  .status-pending { color: rgba(251, 191, 36, 0.9); background: rgba(251, 191, 36, 0.1); }
  .status-approved { color: rgba(34, 197, 94, 0.9); background: rgba(34, 197, 94, 0.1); }
  .status-rejected { color: rgba(239, 68, 68, 0.9); background: rgba(239, 68, 68, 0.1); }
  .status-expired { color: rgba(229, 231, 235, 0.5); background: rgba(229, 231, 235, 0.1); }

  .date-label {
    font-size: 0.8rem;
    color: rgba(229, 231, 235, 0.5);
    margin-left: auto;
  }

  .expand-btn {
    width: 28px;
    height: 28px;
    display: flex;
    align-items: center;
    justify-content: center;
    border-radius: 6px;
    border: none;
    background: transparent;
    color: rgba(229, 231, 235, 0.4);
    cursor: pointer;
    transition: all 0.1s;
  }

  .expand-btn svg {
    width: 16px;
    height: 16px;
    transition: transform 0.2s;
  }

  .expand-btn svg.rotated {
    transform: rotate(180deg);
  }

  .expand-btn:hover {
    color: rgba(229, 231, 235, 0.9);
    background: rgba(255, 255, 255, 0.05);
  }

  .card-reasoning {
    margin-top: 12px;
    padding: 10px 12px;
    border-radius: 6px;
    background: rgba(255, 255, 255, 0.02);
    font-size: 0.85rem;
    color: rgba(229, 231, 235, 0.7);
    line-height: 1.5;
  }

  .card-reasoning strong {
    color: rgba(229, 231, 235, 0.5);
    font-weight: 600;
  }

  .card-details {
    margin-top: 12px;
    padding: 12px;
    border-radius: 6px;
    background: rgba(0, 0, 0, 0.2);
  }

  .detail-heading {
    font-size: 0.75rem;
    font-weight: 600;
    color: rgba(229, 231, 235, 0.6);
    text-transform: uppercase;
    letter-spacing: 0.02em;
    margin-bottom: 8px;
  }

  .alternates,
  .trace {
    margin-bottom: 12px;
  }

  .alternate-row {
    display: flex;
    gap: 12px;
    align-items: baseline;
    padding: 4px 0;
    border-bottom: 1px solid rgba(255, 255, 255, 0.04);
  }

  .alt-code {
    font-family: ui-monospace, monospace;
    font-size: 0.85rem;
    color: rgba(229, 231, 235, 0.8);
  }

  .alt-display {
    flex: 1;
    font-size: 0.8rem;
    color: rgba(229, 231, 235, 0.5);
  }

  .alt-conf {
    font-size: 0.8rem;
    color: rgba(229, 231, 235, 0.5);
  }

  .trace-step {
    display: flex;
    gap: 12px;
    padding: 4px 0;
    font-size: 0.8rem;
  }

  .step-name {
    color: rgba(229, 231, 235, 0.7);
    font-weight: 500;
    min-width: 120px;
  }

  .step-result {
    flex: 1;
    color: rgba(229, 231, 235, 0.6);
  }

  .step-duration {
    color: rgba(229, 231, 235, 0.4);
    font-family: ui-monospace, monospace;
  }

  .card-actions {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
    margin-top: 16px;
    padding-top: 12px;
    border-top: 1px solid rgba(255, 255, 255, 0.06);
  }

  .rejection-reason {
    margin-top: 12px;
    padding: 8px 12px;
    border-radius: 6px;
    background: rgba(239, 68, 68, 0.1);
    font-size: 0.85rem;
    color: rgba(239, 68, 68, 0.8);
  }

  .rejection-reason strong {
    font-weight: 600;
  }

  /* Pagination */
  .pagination {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 8px 0;
  }

  .pagination-info {
    font-size: 0.85rem;
    color: rgba(229, 231, 235, 0.6);
  }

  .pagination-controls {
    display: flex;
    gap: 8px;
  }

  .page-btn {
    padding: 6px 12px;
    border-radius: 6px;
    border: 1px solid rgba(255, 255, 255, 0.1);
    background: transparent;
    color: rgba(229, 231, 235, 0.8);
    font-size: 0.85rem;
    cursor: pointer;
    transition: all 0.1s;
  }

  .page-btn:hover:not(:disabled) {
    background: rgba(255, 255, 255, 0.05);
  }

  .page-btn:disabled {
    opacity: 0.4;
    cursor: not-allowed;
  }

  /* States */
  .loading {
    padding: 48px;
    text-align: center;
    color: rgba(229, 231, 235, 0.6);
  }

  .error-state {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 12px;
    padding: 48px;
  }

  .error-message {
    color: rgba(239, 68, 68, 0.9);
    font-size: 0.9rem;
  }

  .empty-state {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 8px;
    padding: 48px;
  }

  .empty-icon {
    width: 48px;
    height: 48px;
    color: rgba(34, 197, 94, 0.4);
  }

  .empty-icon svg {
    width: 100%;
    height: 100%;
  }

  .empty-text {
    color: rgba(229, 231, 235, 0.7);
    font-size: 0.95rem;
  }

  .empty-hint {
    color: rgba(229, 231, 235, 0.5);
    font-size: 0.85rem;
  }

  /* Modal Content */
  .reject-reason-input textarea {
    width: 100%;
    padding: 10px 12px;
    border-radius: 8px;
    border: 1px solid rgba(255, 255, 255, 0.1);
    background: rgba(255, 255, 255, 0.03);
    color: rgba(229, 231, 235, 0.9);
    font-size: 0.9rem;
    font-family: inherit;
    resize: vertical;
    outline: none;
    margin-top: 12px;
  }

  .reject-reason-input textarea:focus {
    border-color: rgba(59, 130, 246, 0.5);
    box-shadow: 0 0 0 2px rgba(59, 130, 246, 0.15);
  }

  .bulk-config {
    margin-top: 12px;
  }

  .bulk-config label {
    display: flex;
    align-items: center;
    gap: 12px;
  }

  .bulk-label {
    font-size: 0.85rem;
    color: rgba(229, 231, 235, 0.7);
    min-width: 120px;
  }

  .bulk-config input[type="range"] {
    flex: 1;
    accent-color: rgb(59, 130, 246);
  }

  .bulk-value {
    font-family: ui-monospace, monospace;
    font-size: 0.9rem;
    color: rgba(229, 231, 235, 0.9);
    min-width: 50px;
    text-align: right;
  }

  .bulk-hint {
    margin-top: 8px;
    font-size: 0.8rem;
    color: rgba(229, 231, 235, 0.5);
  }
</style>
