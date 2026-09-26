<script lang="ts">
  import { onMount, createEventDispatcher } from "svelte";
  import { SvelteSet } from "svelte/reactivity";
  import Check from "@lucide/svelte/icons/check";
  import ChevronLeft from "@lucide/svelte/icons/chevron-left";
  import ChevronRight from "@lucide/svelte/icons/chevron-right";
  import CircleAlert from "@lucide/svelte/icons/circle-alert";
  import Inbox from "@lucide/svelte/icons/inbox";
  import ListChecks from "@lucide/svelte/icons/list-checks";
  import X from "@lucide/svelte/icons/x";
  import {
    Badge,
    Button,
    EmptyState,
    Field,
    Icon,
    Input,
    KeyValue,
    Select,
    Table,
    Td,
    Textarea,
    Th,
    Tr,
    type KeyValueItem,
    type SelectOption,
  } from "$lib/ui/primitives";
  import ConfirmModal from "$lib/ui/ConfirmModal.svelte";
  import { toasts } from "$lib/ui/toastStore";
  import { isErrorToasted } from "$lib/graphql/client";
  import {
    listPendingAutoroutes,
    getPendingAutorouteStats,
    approvePendingAutoroute,
    rejectPendingAutoroute,
    bulkApprovePendingAutoroutes,
  } from "./terminologyApi";
  import {
    confidenceTone,
    equivalenceLabel,
    formatPercent,
    formatTimestamp,
    pendingStatusLabel,
    pendingStatusTone,
  } from "./terminologyFormat";
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

  const statusOptions: SelectOption[] = [
    { value: "", label: "All statuses" },
    { value: "PENDING", label: "Pending" },
    { value: "APPROVED", label: "Approved" },
    { value: "REJECTED", label: "Rejected" },
    { value: "EXPIRED", label: "Expired" },
  ];

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
  // Bulk selection: an array reassigned on every change so the `$:`
  // statements below re-run (legacy `$:` does not see SvelteSet mutations).
  let selectedIds: string[] = [];

  let visiblePendingIds: string[] = [];
  let selectedVisiblePendingIds: string[] = [];
  let allVisiblePendingSelected = false;
  let someVisiblePendingSelected = false;

  // The row shown in the details pane (independent of the bulk checkboxes)
  let activeId: string | null = null;

  $: visiblePendingIds = pending
    .filter((item) => item.status === "PENDING")
    .map((item) => item.id);
  $: selectedVisiblePendingIds = visiblePendingIds.filter((id) =>
    selectedIds.includes(id),
  );
  $: allVisiblePendingSelected =
    visiblePendingIds.length > 0 &&
    selectedVisiblePendingIds.length === visiblePendingIds.length;
  $: someVisiblePendingSelected =
    selectedVisiblePendingIds.length > 0 && !allVisiblePendingSelected;
  $: if (selectAllPendingEl) {
    selectAllPendingEl.indeterminate = someVisiblePendingSelected;
  }
  $: hasSelectColumn = visiblePendingIds.length > 0;
  $: hasFilters = Boolean(
    filterStatus !== "PENDING" ||
      filterMinConfidence ||
      filterSourceSystem ||
      filterTargetSystem,
  );

  $: active = pending.find((item) => item.id === activeId) ?? null;
  $: activeItems = active ? detailItems(active) : [];

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
      if (!pending.some((item) => item.id === activeId)) {
        activeId = pending[0]?.id ?? null;
      }
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

  function applyOnEnter(event: KeyboardEvent) {
    if (event.key === "Enter") applyFilters();
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

  function toggleSelected(id: string) {
    selectedIds = selectedIds.includes(id)
      ? selectedIds.filter((selected) => selected !== id)
      : [...selectedIds, id];
  }

  function toggleSelectAllVisiblePending(event: Event) {
    const checked =
      (event.currentTarget as HTMLInputElement | null)?.checked ?? false;
    selectedIds = checked
      ? [
          ...selectedIds,
          ...visiblePendingIds.filter((id) => !selectedIds.includes(id)),
        ]
      : selectedIds.filter((id) => !visiblePendingIds.includes(id));
  }

  function clearSelected() {
    selectedIds = selectedIds.filter(
      (id) => !selectedVisiblePendingIds.includes(id),
    );
  }

  function syncSelectedWithVisiblePending() {
    const visiblePending = pending
      .filter((item) => item.status === "PENDING")
      .map((item) => item.id);
    selectedIds = selectedIds.filter((id) => visiblePending.includes(id));
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
      // Global graphqlFetch net already toasts graphql failures (B4 dedupe).
      if (!isErrorToasted(err)) {
        toasts.error(err instanceof Error ? err.message : "Approval failed");
      }
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
      // Global graphqlFetch net already toasts graphql failures (B4 dedupe).
      if (!isErrorToasted(err)) {
        toasts.error(err instanceof Error ? err.message : "Rejection failed");
      }
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
      // Global graphqlFetch net already toasts graphql failures (B4 dedupe).
      if (!isErrorToasted(err)) {
        toasts.error(err instanceof Error ? err.message : "Bulk approval failed");
      }
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
          selectedIds = selectedIds.filter((selected) => selected !== id);
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

  function detailItems(item: PendingNode): KeyValueItem[] {
    // Codes and confidence are in the pane's title row; this is the rest.
    const items: KeyValueItem[] = [
      { key: "Source system", value: item.sourceSystem, mono: true },
      { key: "Source display", value: item.sourceDisplay },
      { key: "Target system", value: item.targetSystem, mono: true },
      { key: "Suggested display", value: item.suggestedDisplay },
      { key: "Equivalence", value: equivalenceLabel(item.equivalence) },
      { key: "Created", value: formatTimestamp(item.createdAt), mono: true },
      { key: "Expires", value: formatTimestamp(item.expiresAt), mono: true },
    ];
    if (item.status !== "PENDING") {
      items.push(
        { key: "Reviewed", value: formatTimestamp(item.reviewedAt), mono: true },
        { key: "Reviewed by", value: item.reviewedBy },
      );
    }
    if (item.status === "REJECTED") {
      items.push({ key: "Rejection reason", value: item.rejectionReason });
    }
    items.push({ key: "Suggestion id", value: item.id, mono: true, truncate: true });
    return items;
  }
</script>

<div class="review">
  <div class="filters">
    <div class="filter-status">
      <Select
        bind:value={filterStatus}
        options={statusOptions}
        aria-label="Status"
      />
    </div>
    <div class="filter-confidence">
      <Input
        type="number"
        bind:value={filterMinConfidence}
        placeholder="Min confidence"
        aria-label="Minimum confidence"
        min="0"
        max="1"
        step="0.05"
        onkeydown={applyOnEnter}
      />
    </div>
    <div class="filter-text">
      <Input
        bind:value={filterSourceSystem}
        placeholder="Source system"
        aria-label="Source system"
        onkeydown={applyOnEnter}
      />
    </div>
    <div class="filter-text">
      <Input
        bind:value={filterTargetSystem}
        placeholder="Target system"
        aria-label="Target system"
        onkeydown={applyOnEnter}
      />
    </div>
    <Button variant="ghost" onclick={clearFilters} disabled={!hasFilters}
      >Clear</Button
    >
    <Button onclick={applyFilters}>Apply</Button>

    <div class="filters-end">
      {#if stats}
        <dl class="stats" aria-label="Review totals">
          <div class="stat">
            <dt>Pending</dt>
            <dd class="text-mono">{stats.pendingCount}</dd>
          </div>
          <div class="stat">
            <dt>Approved</dt>
            <dd class="text-mono">{stats.approvedCount}</dd>
          </div>
          <div class="stat">
            <dt>Rejected</dt>
            <dd class="text-mono">{stats.rejectedCount}</dd>
          </div>
          <div class="stat">
            <dt>Expired</dt>
            <dd class="text-mono">{stats.expiredCount}</dd>
          </div>
          {#if stats.avgConfidence}
            <div class="stat">
              <dt>Avg confidence</dt>
              <dd class="text-mono">{formatPercent(stats.avgConfidence, 1)}</dd>
            </div>
          {/if}
        </dl>
        <Button icon={ListChecks} onclick={openBulkApprove}>Bulk approve</Button>
      {/if}
    </div>
  </div>

  {#if loading}
    <EmptyState message="Loading suggestions…" aria-busy="true" />
  {:else if error}
    <EmptyState
      icon={CircleAlert}
      message="Suggestions could not be loaded: {error}"
    >
      {#snippet action()}
        <Button onclick={loadPending}>Retry</Button>
      {/snippet}
    </EmptyState>
  {:else if pending.length === 0}
    <EmptyState
      icon={Inbox}
      message={filterStatus === "PENDING" && !hasFilters
        ? "No suggestions are waiting for review."
        : "No suggestions match these filters."}
    />
  {:else}
    {#if visiblePendingIds.length > 0}
      <div class="bulk-bar" role="toolbar" aria-label="Bulk review actions">
        <span class="bulk-count text-mono">
          {selectedVisiblePendingIds.length} selected
        </span>
        <Button
          variant="ghost"
          disabled={selectedVisiblePendingIds.length === 0 ||
            bulkApprovingSelected}
          onclick={clearSelected}
        >
          Clear selected
        </Button>
        <Button
          icon={Check}
          loading={bulkApprovingSelected}
          disabled={selectedVisiblePendingIds.length === 0}
          onclick={handleApproveSelected}
        >
          Approve selected
        </Button>
      </div>
    {/if}

    <div class="split">
      <div class="list">
        <Table label="Suggestions" layout="fixed" class="review-table">
          {#snippet head()}
            <tr>
              {#if hasSelectColumn}
                <Th width="36px">
                  <input
                    bind:this={selectAllPendingEl}
                    class="row-check"
                    type="checkbox"
                    aria-label="Select page"
                    checked={allVisiblePendingSelected}
                    on:change={toggleSelectAllVisiblePending}
                    disabled={bulkApprovingSelected}
                  />
                </Th>
              {/if}
              <Th>Source system</Th>
              <Th width="112px">Source code</Th>
              <Th width="120px">Suggested code</Th>
              <Th>Target system</Th>
              <Th width="96px" numeric>Confidence</Th>
              <Th width="104px">Equivalence</Th>
              <Th width="104px">Status</Th>
              <Th width="136px">Created</Th>
            </tr>
          {/snippet}
          {#each pending as item (item.id)}
            {@const isProcessing = processingIds.has(item.id)}
            <Tr
              selectable
              selected={item.id === activeId}
              onselect={() => (activeId = item.id)}
              aria-label="Suggestion {item.sourceCode} to {item.suggestedCode}"
            >
              {#if hasSelectColumn}
                <Td>
                  {#if item.status === "PENDING"}
                    <input
                      class="row-check"
                      type="checkbox"
                      aria-label="Select suggestion {item.sourceCode} to {item.suggestedCode}"
                      checked={selectedIds.includes(item.id)}
                      on:click={(event) => event.stopPropagation()}
                      on:change={() => toggleSelected(item.id)}
                      disabled={isProcessing || bulkApprovingSelected}
                    />
                  {/if}
                </Td>
              {/if}
              <Td muted truncate value={item.sourceSystem} />
              <Td mono truncate value={item.sourceCode} />
              <Td mono truncate value={item.suggestedCode} />
              <Td muted truncate value={item.targetSystem} />
              <Td numeric value={formatPercent(item.confidence, 1)} />
              <Td truncate value={equivalenceLabel(item.equivalence) || "—"} />
              <Td>
                <Badge tone={pendingStatusTone(item.status)} dot
                  >{pendingStatusLabel(item.status)}</Badge
                >
              </Td>
              <Td mono muted value={formatTimestamp(item.createdAt)} />
            </Tr>
          {/each}
        </Table>

        {#if totalCount > pageSize || offset > 0}
          <div class="pagination">
            <span class="pagination-info text-mono">
              {offset + 1}–{Math.min(offset + pending.length, totalCount)} of {totalCount}
            </span>
            <Button
              variant="ghost"
              icon={ChevronLeft}
              onclick={prevPage}
              disabled={offset === 0}
            >
              Previous
            </Button>
            <Button
              variant="ghost"
              onclick={nextPage}
              disabled={offset + pageSize >= totalCount}
            >
              Next
              <Icon icon={ChevronRight} />
            </Button>
          </div>
        {/if}
      </div>

      <aside class="details" aria-label="Selected suggestion">
        {#if active}
          <div class="details-head">
            <span
              class="details-title text-mono"
              title="{active.sourceCode} → {active.suggestedCode}"
              >{active.sourceCode} → {active.suggestedCode}</span
            >
            <Badge tone={pendingStatusTone(active.status)} dot
              >{pendingStatusLabel(active.status)}</Badge
            >
            <Badge tone={confidenceTone(active.confidence)} mono
              >{formatPercent(active.confidence, 1)}</Badge
            >
          </div>

          {#if active.status === "PENDING"}
            {@const target = active}
            {@const activeProcessing = processingIds.has(target.id)}
            <div class="details-actions">
              <Button
                variant="primary"
                icon={Check}
                loading={activeProcessing}
                disabled={bulkApprovingSelected}
                onclick={() => handleApprove(target)}
              >
                Approve
              </Button>
              <Button
                icon={X}
                disabled={activeProcessing || bulkApprovingSelected}
                onclick={() => confirmReject(target.id)}
              >
                Reject
              </Button>
            </div>
          {/if}

          {#if active.reasoning}
            <section class="details-section" aria-label="Reasoning">
              <h3 class="text-label">Reasoning</h3>
              <p class="details-text">{active.reasoning}</p>
            </section>
          {/if}

          <KeyValue items={activeItems} />

          {#if active.alternates && active.alternates.length > 0}
            <section class="details-section" aria-label="Alternatives considered">
              <h3 class="text-label">Alternatives considered</h3>
              <Table label="Alternatives considered" layout="fixed">
                {#snippet head()}
                  <tr>
                    <Th width="88px">Code</Th>
                    <Th>Display</Th>
                    <Th width="72px" numeric>Conf.</Th>
                  </tr>
                {/snippet}
                {#each active.alternates as alt (alt.code)}
                  <Tr>
                    <Td mono truncate value={alt.code} />
                    <Td truncate value={alt.display || "—"} />
                    <Td numeric value={formatPercent(alt.confidence, 1)} />
                  </Tr>
                {/each}
              </Table>
            </section>
          {/if}

          {#if active.decisionTrace}
            <section class="details-section" aria-label="Decision trace">
              <h3 class="text-label">
                Decision trace
                <span class="text-mono section-meta"
                  >{active.decisionTrace.totalDurationMs} ms</span
                >
              </h3>
              <Table label="Decision trace" layout="fixed">
                {#snippet head()}
                  <tr>
                    <Th width="136px">Step</Th>
                    <Th>Result</Th>
                    <Th width="64px" numeric>ms</Th>
                  </tr>
                {/snippet}
                {#each active.decisionTrace.steps as step, idx (idx)}
                  <Tr>
                    <Td mono truncate value={step.step} />
                    <Td truncate value={step.result} />
                    <Td numeric value={step.durationMs} />
                  </Tr>
                {/each}
              </Table>
            </section>
          {/if}
        {:else}
          <EmptyState message="Select a suggestion to see its details." />
        {/if}
      </aside>
    </div>
  {/if}
</div>

<!-- Reject Modal -->
<ConfirmModal
  bind:open={showRejectModal}
  title="Reject suggestion"
  message="Give a reason for rejecting this mapping suggestion."
  confirmText="Reject"
  variant="danger"
  on:confirm={handleRejectConfirm}
>
  <div class="modal-field">
    <Field label="Reason">
      <Textarea
        bind:value={rejectReason}
        placeholder="e.g., Incorrect mapping - codes are not semantically equivalent"
        rows={3}
      />
    </Field>
  </div>
</ConfirmModal>

<!-- Bulk Approve Modal -->
<ConfirmModal
  bind:open={showBulkApproveModal}
  title="Bulk approve high-confidence suggestions"
  message="Approve all pending suggestions above the confidence threshold."
  confirmText="Approve all"
  variant="primary"
  on:confirm={handleBulkApprove}
>
  <div class="modal-field">
    <Field
      label="Minimum confidence"
      id="bulk-min-confidence"
      hint="Only suggestions with confidence ≥ {formatPercent(bulkMinConfidence, 1)} will be approved."
    >
      <div class="range-row">
        <input
          id="bulk-min-confidence"
          class="range"
          type="range"
          min="0.7"
          max="0.99"
          step="0.01"
          bind:value={bulkMinConfidence}
        />
        <span class="range-value text-mono"
          >{formatPercent(bulkMinConfidence, 1)}</span
        >
      </div>
    </Field>
  </div>
</ConfirmModal>

<style>
  .review {
    display: flex;
    flex-direction: column;
    flex: 1 1 auto;
    min-height: 0;
  }

  .filters,
  .bulk-bar {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2);
    padding: var(--space-2) var(--space-3);
    border-bottom: 1px solid var(--color-border-subtle);
  }

  .filter-status {
    width: 132px;
  }

  .filter-confidence {
    width: 128px;
  }

  .filter-text {
    width: 160px;
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

  .bulk-count {
    color: var(--color-text-tertiary);
    margin-right: var(--space-1);
  }

  .row-check {
    width: 14px;
    height: 14px;
    margin: 0;
    vertical-align: middle;
    accent-color: var(--color-primary);
  }

  .split {
    display: grid;
    grid-template-columns: minmax(0, 1fr) 360px;
    flex: 1 1 auto;
    min-height: 0;
  }

  .list {
    display: flex;
    flex-direction: column;
    min-width: 0;
    min-height: 0;
  }

  .list :global(.review-table) {
    flex: 1 1 auto;
  }

  /* Fixed layout gives the unsized columns only what is left; keep a floor
     so they never collapse, and let the wrapper scroll sideways instead. */
  .list :global(.review-table > table) {
    min-width: 920px;
  }

  .pagination {
    display: flex;
    align-items: center;
    gap: var(--space-1);
    flex: 0 0 auto;
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

  .details-section {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  .details-section h3 {
    display: flex;
    align-items: baseline;
    gap: var(--space-2);
    margin: 0;
  }

  .section-meta {
    text-transform: none;
    letter-spacing: normal;
  }

  .details-text {
    margin: 0;
    font-size: var(--text-xs);
    line-height: var(--leading-ui);
    color: var(--color-text-secondary);
  }

  .modal-field {
    margin-top: var(--space-3);
  }

  .range-row {
    display: flex;
    align-items: center;
    gap: var(--space-3);
  }

  .range {
    flex: 1;
    accent-color: var(--color-primary);
  }

  .range-value {
    min-width: 48px;
    text-align: right;
    color: var(--color-text-primary);
  }
</style>
