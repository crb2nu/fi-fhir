<script lang="ts">
  import { onMount, createEventDispatcher } from "svelte";
  import { SvelteSet } from "svelte/reactivity";
  import Button from "$lib/ui/Button.svelte";
  import EmptyState from "$lib/ui/EmptyState.svelte";
  import ConfirmModal from "$lib/ui/ConfirmModal.svelte";
  import { toasts } from "$lib/ui/toastStore";
  import {
    listPendingAutoroutes,
    getPendingAutorouteStats,
    approvePendingAutoroute,
    rejectPendingAutoroute,
    bulkApprovePendingAutoroutes,
  } from "./terminologyApi";
  import type {
    PendingAutorouteStatus,
    MappingEquivalence,
    ListPendingAutoroutesQuery,
  } from "$lib/gen/graphql";

  // Use the actual type returned from the query
  type PendingNode =
    ListPendingAutoroutesQuery["listPendingAutoroutes"]["nodes"][number];

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
  let filterStatus: PendingAutorouteStatus | "" = "PENDING";
  let filterMinConfidence = "";
  let filterSourceSystem = sourceSystem ?? "";
  let filterTargetSystem = targetSystem ?? "";

  // Action state
  let showRejectModal = false;
  let showBulkApproveModal = false;
  let rejectingId: string | null = null;
  let rejectReason = "";
  let bulkMinConfidence = 0.95;
  let bulkApprovingSelected = false;
  let selectAllPendingEl: HTMLInputElement | null = null;
  let processingIds = new SvelteSet<string>();
  let selectedIds = new SvelteSet<string>();

  let visiblePendingIds: string[] = [];
  let selectedVisiblePendingIds: string[] = [];
  let allVisiblePendingSelected = false;
  let someVisiblePendingSelected = false;

  // Expanded rows for showing alternates/trace
  let expandedIds = new SvelteSet<string>();

  $: visiblePendingIds = pending
    .filter((item) => item.status === "PENDING")
    .map((item) => item.id);
  $: selectedVisiblePendingIds = visiblePendingIds.filter((id) =>
    selectedIds.has(id),
  );
  $: allVisiblePendingSelected =
    visiblePendingIds.length > 0 &&
    selectedVisiblePendingIds.length === visiblePendingIds.length;
  $: someVisiblePendingSelected =
    selectedVisiblePendingIds.length > 0 && !allVisiblePendingSelected;
  $: if (selectAllPendingEl) {
    selectAllPendingEl.indeterminate = someVisiblePendingSelected;
  }

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
        minConfidence: filterMinConfidence
          ? parseFloat(filterMinConfidence)
          : null,
        sourceSystem: filterSourceSystem || null,
        targetSystem: filterTargetSystem || null,
      });
      pending = result.nodes;
      totalCount = result.totalCount;
      syncSelectedWithVisiblePending();
    } catch (err) {
      error =
        err instanceof Error
          ? err.message
          : "Failed to load pending autoroutes";
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
    filterStatus = "PENDING";
    filterMinConfidence = "";
    filterSourceSystem = "";
    filterTargetSystem = "";
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

  function toggleSelected(id: string) {
    if (selectedIds.has(id)) {
      selectedIds.delete(id);
    } else {
      selectedIds.add(id);
    }
  }

  function toggleSelectAllVisiblePending(event: Event) {
    const checked =
      (event.currentTarget as HTMLInputElement | null)?.checked ?? false;
    for (const id of visiblePendingIds) {
      if (checked) selectedIds.add(id);
      else selectedIds.delete(id);
    }
  }

  function clearSelected() {
    for (const id of selectedVisiblePendingIds) {
      selectedIds.delete(id);
    }
  }

  function syncSelectedWithVisiblePending() {
    const visiblePendingSet = new Set(
      pending
        .filter((item) => item.status === "PENDING")
        .map((item) => item.id),
    );
    for (const id of selectedIds) {
      if (!visiblePendingSet.has(id)) {
        selectedIds.delete(id);
      }
    }
  }

  async function handleApprove(
    item: PendingNode,
    equivalence?: MappingEquivalence,
  ) {
    processingIds.add(item.id);

    try {
      const mapping = await approvePendingAutoroute({
        id: item.id,
        equivalence: equivalence ?? null,
        comment: null,
      });
      toasts.success(
        `Approved mapping: ${item.sourceCode} → ${item.suggestedCode}`,
      );
      dispatch("approve", { id: item.id, mappingId: mapping.id });
      await loadPending();
      await loadStats();
    } catch (err) {
      toasts.error(err instanceof Error ? err.message : "Approval failed");
    } finally {
      processingIds.delete(item.id);
    }
  }

  function confirmReject(id: string) {
    rejectingId = id;
    rejectReason = "";
    showRejectModal = true;
  }

  async function handleRejectConfirm() {
    if (!rejectingId) return;

    processingIds.add(rejectingId);

    try {
      await rejectPendingAutoroute({
        id: rejectingId,
        reason: rejectReason || "Rejected by reviewer",
      });
      toasts.success("Suggestion rejected");
      dispatch("reject", { id: rejectingId });
      await loadPending();
      await loadStats();
    } catch (err) {
      toasts.error(err instanceof Error ? err.message : "Rejection failed");
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
        maxCount: 100,
      });
      toasts.success(
        `Approved ${result.approved} mappings (${result.skipped} skipped)`,
      );
      dispatch("refresh");
      await loadPending();
      await loadStats();
    } catch (err) {
      toasts.error(err instanceof Error ? err.message : "Bulk approval failed");
    } finally {
      showBulkApproveModal = false;
    }
  }

  async function handleApproveSelected() {
    const ids = [...selectedVisiblePendingIds];
    if (ids.length === 0) return;

    bulkApprovingSelected = true;
    let approved = 0;
    let failed = 0;

    for (const id of ids) {
      processingIds.add(id);
    }

    try {
      for (const id of ids) {
        try {
          await approvePendingAutoroute({
            id,
            equivalence: null,
            comment: null,
          });
          approved += 1;
          selectedIds.delete(id);
        } catch {
          failed += 1;
        } finally {
          processingIds.delete(id);
        }
      }

      if (approved > 0) {
        toasts.success(
          `Approved ${approved} selected suggestion${approved === 1 ? "" : "s"}`,
        );
        dispatch("refresh");
      }
      if (failed > 0) {
        toasts.error(
          `Failed to approve ${failed} selected suggestion${failed === 1 ? "" : "s"}`,
        );
      }

      await loadPending();
      await loadStats();
    } finally {
      bulkApprovingSelected = false;
    }
  }

  function formatConfidence(confidence: number): string {
    return `${(confidence * 100).toFixed(1)}%`;
  }

  function getConfidenceClass(confidence: number): string {
    if (confidence >= 0.9) return "conf-high";
    if (confidence >= 0.7) return "conf-med";
    if (confidence >= 0.5) return "conf-low";
    return "conf-none";
  }

  function formatStatus(status: PendingAutorouteStatus): string {
    switch (status) {
      case "PENDING":
        return "Pending";
      case "APPROVED":
        return "Approved";
      case "REJECTED":
        return "Rejected";
      case "EXPIRED":
        return "Expired";
      default:
        return String(status);
    }
  }

  function formatEquivalence(
    eq: MappingEquivalence | null | undefined,
  ): string {
    if (!eq) return "—";
    switch (eq) {
      case "EQUIVALENT":
        return "Equivalent";
      case "WIDER":
        return "Wider";
      case "NARROWER":
        return "Narrower";
      case "INEXACT":
        return "Inexact";
      default:
        return String(eq);
    }
  }

  function formatDate(dateStr: string): string {
    const date = new Date(dateStr);
    return date.toLocaleDateString("en-US", {
      month: "short",
      day: "numeric",
      hour: "2-digit",
      minute: "2-digit",
    });
  }

  function truncateSystem(system: string): string {
    if (!system) return "";
    if (system.startsWith("http://")) {
      const rest = system.replace("http://", "");
      return rest.split("/")[0] ?? rest;
    }
    if (system.startsWith("https://")) {
      const rest = system.replace("https://", "");
      return rest.split("/")[0] ?? rest;
    }
    return system.length > 20 ? system.substring(0, 17) + "..." : system;
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
          <span class="stat-value">{formatConfidence(stats.avgConfidence)}</span
          >
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
          on:keydown={(e) => e.key === "Enter" && applyFilters()}
        />
      </label>
      <label class="filter-field">
        <span class="filter-label">Target System</span>
        <input
          type="text"
          class="filter-input"
          bind:value={filterTargetSystem}
          placeholder="e.g., http://loinc.org"
          on:keydown={(e) => e.key === "Enter" && applyFilters()}
        />
      </label>
      <div class="filter-actions">
        <Button variant="secondary" size="sm" on:click={clearFilters}
          >Clear</Button
        >
        <Button variant="primary" size="sm" on:click={applyFilters}
          >Apply</Button
        >
      </div>
    </div>
  </div>

  <!-- Content -->
  {#if loading}
    <div class="loading">Loading pending suggestions...</div>
  {:else if error}
    <div class="error-state">
      <div class="error-message">{error}</div>
      <Button variant="secondary" size="sm" on:click={loadPending}>Retry</Button
      >
    </div>
  {:else if pending.length === 0}
    <EmptyState
      icon="inbox"
      title="No pending suggestions"
      description={filterStatus !== "PENDING"
        ? 'Try filtering by "Pending" status'
        : "All suggestions have been reviewed"}
    />
  {:else}
    {#if visiblePendingIds.length > 0}
      <div class="bulk-toolbar" role="toolbar" aria-label="Bulk review actions">
        <div class="bulk-toolbar-left">
          <label class="bulk-select">
            <input
              bind:this={selectAllPendingEl}
              type="checkbox"
              checked={allVisiblePendingSelected}
              on:change={toggleSelectAllVisiblePending}
              disabled={bulkApprovingSelected}
            />
            <span>Select page</span>
          </label>
          <span class="bulk-count">
            {selectedVisiblePendingIds.length} selected
          </span>
        </div>
        <div class="bulk-toolbar-actions">
          <Button
            variant="secondary"
            size="sm"
            disabled={selectedVisiblePendingIds.length === 0 ||
              bulkApprovingSelected}
            on:click={clearSelected}
          >
            Clear Selected
          </Button>
          <Button
            variant="primary"
            size="sm"
            disabled={selectedVisiblePendingIds.length === 0 ||
              bulkApprovingSelected}
            on:click={handleApproveSelected}
          >
            {bulkApprovingSelected ? "Approving..." : "Approve Selected"}
          </Button>
        </div>
      </div>
    {/if}

    <div class="cards">
      {#each pending as item, i (item.id)}
        {@const isExpanded = expandedIds.has(item.id)}
        {@const isProcessing = processingIds.has(item.id)}
        <div
          class="card hover-lift"
          class:expanded={isExpanded}
          style="animation-delay: {Math.min(i, 20) * 0.05}s"
        >
          <div class="card-header">
            {#if item.status === "PENDING"}
              <label class="card-select">
                <input
                  type="checkbox"
                  checked={selectedIds.has(item.id)}
                  on:change={() => toggleSelected(item.id)}
                  disabled={isProcessing || bulkApprovingSelected}
                />
                <span class="sr-only"
                  >Select suggestion {item.sourceCode} to {item.suggestedCode}</span
                >
              </label>
            {/if}
            <div class="card-source">
              <span class="system-label"
                >{truncateSystem(item.sourceSystem)}</span
              >
              <span class="code-value">{item.sourceCode}</span>
              {#if item.sourceDisplay}
                <span class="display-value">{item.sourceDisplay}</span>
              {/if}
            </div>
            <div class="card-arrow">
              <svg
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                stroke-width="2"
              >
                <path d="M14 5l7 7m0 0l-7 7m7-7H3" />
              </svg>
            </div>
            <div class="card-target">
              <span class="system-label"
                >{truncateSystem(item.targetSystem)}</span
              >
              <span class="code-value suggested">{item.suggestedCode}</span>
              {#if item.suggestedDisplay}
                <span class="display-value">{item.suggestedDisplay}</span>
              {/if}
            </div>
          </div>

          <div class="card-meta">
            <span
              class="confidence-badge {getConfidenceClass(item.confidence)}"
            >
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
              type="button"
              class="expand-btn"
              on:click={() => toggleExpand(item.id)}
              title={isExpanded ? "Collapse details" : "Expand details"}
              aria-label={isExpanded
                ? "Collapse suggestion details"
                : "Expand suggestion details"}
              aria-expanded={isExpanded}
              aria-controls={`pending-details-${item.id}`}
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
              <strong>Reasoning:</strong>
              {item.reasoning}
            </div>
          {/if}

          {#if isExpanded}
            <div class="card-details" id={`pending-details-${item.id}`}>
              {#if item.alternates && item.alternates.length > 0}
                <div class="alternates">
                  <div class="detail-heading">Alternatives Considered</div>
                  {#each item.alternates as alt (alt.code)}
                    <div class="alternate-row">
                      <span class="alt-code">{alt.code}</span>
                      {#if alt.display}
                        <span class="alt-display">{alt.display}</span>
                      {/if}
                      <span class="alt-conf"
                        >{formatConfidence(alt.confidence)}</span
                      >
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

          {#if item.status === "PENDING"}
            <div class="card-actions">
              <Button
                variant="secondary"
                size="sm"
                disabled={isProcessing || bulkApprovingSelected}
                on:click={() => confirmReject(item.id)}
              >
                Reject
              </Button>
              <Button
                variant="primary"
                size="sm"
                disabled={isProcessing || bulkApprovingSelected}
                on:click={() => handleApprove(item)}
              >
                {isProcessing ? "Processing..." : "Approve"}
              </Button>
            </div>
          {:else if item.status === "REJECTED" && item.rejectionReason}
            <div class="rejection-reason">
              <strong>Reason:</strong>
              {item.rejectionReason}
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
        <button
          type="button"
          class="page-btn"
          on:click={prevPage}
          disabled={offset === 0}
        >
          Previous
        </button>
        <button
          type="button"
          class="page-btn"
          on:click={nextPage}
          disabled={offset + pageSize >= totalCount}
        >
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
      Only suggestions with confidence ≥ {formatConfidence(bulkMinConfidence)} will
      be approved.
    </p>
  </div>
</ConfirmModal>

<style>
  .review-list {
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
    background: var(--color-bg-elevated);
    border: 1px solid var(--color-border-subtle);
    align-items: center;
    flex-wrap: wrap;
  }

  .stat {
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  .stat-value {
    font-size: var(--text-2xl);
    font-weight: var(--font-semibold);
    color: var(--color-text-primary);
    font-variant-numeric: tabular-nums;
  }

  .stat-value.pending {
    color: var(--color-warning);
  }
  .stat-value.approved {
    color: var(--color-success);
  }
  .stat-value.rejected {
    color: var(--color-danger);
  }
  .stat-value.expired {
    color: var(--color-text-muted);
  }

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
    background: var(--color-bg-elevated);
    border: 1px solid var(--color-border-subtle);
  }

  .filter-row {
    display: flex;
    gap: var(--space-3);
    align-items: flex-end;
    flex-wrap: wrap;
  }

  .filter-field {
    flex: 1;
    min-width: 140px;
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
  }

  .filter-field.filter-sm {
    flex: 0.6;
    min-width: 100px;
  }

  .filter-label {
    font-size: var(--text-xs);
    font-weight: var(--font-semibold);
    color: var(--color-text-tertiary);
    text-transform: uppercase;
    letter-spacing: var(--tracking-wide);
  }

  .filter-input,
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

  .filter-input:focus,
  .filter-select:focus {
    border-color: var(--color-border-focus);
    box-shadow: var(--shadow-focus);
  }

  .filter-actions {
    display: flex;
    gap: var(--space-2);
  }

  /* Cards */
  .cards {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
  }

  .bulk-toolbar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3);
    flex-wrap: wrap;
    padding: var(--space-3) var(--space-4);
    border-radius: var(--radius-lg);
    border: 1px solid var(--color-border-subtle);
    background: var(--color-bg-elevated);
  }

  .bulk-toolbar-left {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    flex-wrap: wrap;
  }

  .bulk-toolbar-actions {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    flex-wrap: wrap;
    justify-content: flex-end;
  }

  .bulk-select {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    font-size: var(--text-sm);
    color: var(--color-text-secondary);
  }

  .bulk-select input {
    width: 16px;
    height: 16px;
    accent-color: var(--color-primary);
  }

  .bulk-count {
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    color: var(--color-text-muted);
  }

  .card {
    padding: var(--space-4);
    border-radius: var(--radius-lg);
    background: var(--color-bg-elevated);
    border: 1px solid var(--color-border-subtle);
    border-top: 1px solid rgba(255, 255, 255, 0.05);
    box-shadow: var(--shadow-sm);
    transition: var(--transition-all);
    animation: fade-in-up 0.4s ease-out both;
  }

  @keyframes fade-in-up {
    from {
      opacity: 0;
      transform: translateY(10px);
    }
    to {
      opacity: 1;
      transform: translateY(0);
    }
  }

  .card:hover {
    background: var(--color-bg-hover);
    border-color: var(--color-border-strong);
    transform: translateY(-2px);
    box-shadow: var(--shadow-md);
  }

  .card.expanded {
    border-color: var(--color-primary-border);
    box-shadow: 0 0 0 1px var(--color-primary-glow);
  }

  .card-header {
    display: flex;
    align-items: center;
    gap: var(--space-4);
  }

  .card-select {
    display: flex;
    align-items: center;
    justify-content: center;
    margin-right: var(--space-1);
  }

  .card-select input {
    width: 16px;
    height: 16px;
    accent-color: var(--color-primary);
  }

  .card-source,
  .card-target {
    flex: 1;
    display: flex;
    flex-direction: column;
    gap: 2px;
    min-width: 0;
  }

  .card-arrow {
    width: 24px;
    height: 24px;
    color: var(--color-text-muted);
    flex-shrink: 0;
  }

  .card-arrow svg {
    width: 100%;
    height: 100%;
  }

  .system-label {
    font-size: var(--text-xs);
    color: var(--color-text-muted);
  }

  .code-value {
    font-family: var(--font-mono);
    font-size: var(--text-base);
    color: var(--color-text-primary);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .code-value.suggested {
    color: var(--color-primary);
  }

  .display-value {
    font-size: var(--text-xs);
    color: var(--color-text-tertiary);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .card-meta {
    display: flex;
    gap: var(--space-2);
    align-items: center;
    margin-top: var(--space-3);
    flex-wrap: wrap;
  }

  .confidence-badge {
    padding: 2px var(--space-2);
    border-radius: var(--radius-sm);
    font-size: var(--text-xs);
    font-weight: var(--font-semibold);
  }

  .conf-high {
    color: var(--confidence-high);
    background: var(--confidence-high-bg);
  }
  .conf-med {
    color: var(--confidence-medium);
    background: var(--confidence-medium-bg);
  }
  .conf-low {
    color: var(--confidence-low);
    background: var(--confidence-low-bg);
  }
  .conf-none {
    color: var(--confidence-very-low);
    background: var(--confidence-very-low-bg);
  }

  .equiv-badge {
    padding: 2px var(--space-2);
    border-radius: var(--radius-sm);
    font-size: var(--text-xs);
    font-weight: var(--font-medium);
  }

  .equiv-equivalent {
    color: var(--color-success-text);
    background: var(--color-success-bg);
  }
  .equiv-wider {
    color: var(--color-info-text);
    background: var(--color-info-bg);
  }
  .equiv-narrower {
    color: var(--color-warning-text);
    background: var(--color-warning-bg);
  }
  .equiv-inexact {
    color: var(--color-text-tertiary);
    background: var(--color-bg-surface);
  }

  .status-badge {
    padding: 2px var(--space-2);
    border-radius: var(--radius-sm);
    font-size: var(--text-xs);
    font-weight: var(--font-medium);
  }

  .status-pending {
    color: var(--color-warning-text);
    background: var(--color-warning-bg);
  }
  .status-approved {
    color: var(--color-success-text);
    background: var(--color-success-bg);
  }
  .status-rejected {
    color: var(--color-danger-text);
    background: var(--color-danger-bg);
  }
  .status-expired {
    color: var(--color-text-muted);
    background: var(--color-bg-surface);
  }

  .date-label {
    font-size: var(--text-xs);
    color: var(--color-text-muted);
    margin-left: auto;
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

  .expand-btn:focus-visible {
    outline: none;
    box-shadow: var(--shadow-focus);
  }

  .card-reasoning {
    margin-top: var(--space-3);
    padding: var(--space-3);
    border-radius: var(--radius-md);
    background: var(--color-bg-surface);
    font-size: var(--text-xs);
    color: var(--color-text-secondary);
    line-height: var(--leading-relaxed);
  }

  .card-reasoning strong {
    color: var(--color-text-muted);
    font-weight: var(--font-semibold);
  }

  .card-details {
    margin-top: var(--space-3);
    padding: var(--space-3);
    border-radius: var(--radius-md);
    background: rgba(0, 0, 0, 0.2);
    animation: slideInUp var(--duration-fast) var(--ease-out);
  }

  .detail-heading {
    font-size: var(--text-xs);
    font-weight: var(--font-semibold);
    color: var(--color-text-tertiary);
    text-transform: uppercase;
    letter-spacing: var(--tracking-wide);
    margin-bottom: var(--space-2);
  }

  .alternates,
  .trace {
    margin-bottom: var(--space-3);
  }

  .alternate-row {
    display: flex;
    gap: var(--space-3);
    align-items: baseline;
    padding: var(--space-1) 0;
    border-bottom: 1px solid var(--color-border-subtle);
  }

  .alt-code {
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    color: var(--color-text-secondary);
  }

  .alt-display {
    flex: 1;
    font-size: var(--text-xs);
    color: var(--color-text-muted);
  }

  .alt-conf {
    font-size: var(--text-xs);
    color: var(--color-text-muted);
  }

  .trace-step {
    display: flex;
    gap: var(--space-3);
    padding: var(--space-1) 0;
    font-size: var(--text-xs);
  }

  .step-name {
    color: var(--color-text-secondary);
    font-weight: var(--font-medium);
    min-width: 120px;
  }

  .step-result {
    flex: 1;
    color: var(--color-text-tertiary);
  }

  .step-duration {
    color: var(--color-text-muted);
    font-family: var(--font-mono);
  }

  .card-actions {
    display: flex;
    justify-content: flex-end;
    gap: var(--space-2);
    margin-top: var(--space-4);
    padding-top: var(--space-3);
    border-top: 1px solid var(--color-border-subtle);
  }

  .rejection-reason {
    margin-top: var(--space-3);
    padding: var(--space-2) var(--space-3);
    border-radius: var(--radius-md);
    background: var(--color-danger-bg);
    font-size: var(--text-xs);
    color: var(--color-danger-text);
  }

  .rejection-reason strong {
    font-weight: var(--font-semibold);
  }

  /* Pagination */
  .pagination {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: var(--space-2) 0;
  }

  .pagination-info {
    font-size: var(--text-xs);
    color: var(--color-text-tertiary);
  }

  .pagination-controls {
    display: flex;
    gap: var(--space-2);
  }

  .page-btn {
    padding: var(--space-2) var(--space-3);
    border-radius: var(--radius-md);
    border: 1px solid var(--color-border-default);
    background: transparent;
    color: var(--color-text-secondary);
    font-size: var(--text-xs);
    cursor: pointer;
    transition: var(--transition-all);
  }

  .page-btn:hover:not(:disabled) {
    background: var(--color-bg-hover);
  }

  .page-btn:focus-visible {
    outline: none;
    box-shadow: var(--shadow-focus);
  }

  .page-btn:disabled {
    opacity: 0.4;
    cursor: not-allowed;
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
    color: var(--color-danger-text);
    font-size: var(--text-sm);
  }

  /* Modal Content */
  .reject-reason-input textarea {
    width: 100%;
    padding: var(--space-3);
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

  .reject-reason-input textarea:focus {
    border-color: var(--color-border-focus);
    box-shadow: var(--shadow-focus);
  }

  .bulk-config {
    margin-top: var(--space-3);
  }

  .bulk-config label {
    display: flex;
    align-items: center;
    gap: var(--space-3);
  }

  .bulk-label {
    font-size: var(--text-xs);
    color: var(--color-text-secondary);
    min-width: 120px;
  }

  .bulk-config input[type="range"] {
    flex: 1;
    accent-color: var(--color-primary);
  }

  .bulk-value {
    font-family: var(--font-mono);
    font-size: var(--text-sm);
    color: var(--color-text-primary);
    min-width: 50px;
    text-align: right;
  }

  .bulk-hint {
    margin-top: var(--space-2);
    font-size: var(--text-xs);
    color: var(--color-text-muted);
  }

  @keyframes slideInUp {
    from {
      opacity: 0;
      transform: translateY(-4px);
    }
    to {
      opacity: 1;
      transform: translateY(0);
    }
  }
</style>
