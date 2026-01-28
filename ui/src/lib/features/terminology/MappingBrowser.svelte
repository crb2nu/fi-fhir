<script lang="ts">
  import { onMount, createEventDispatcher } from 'svelte';
  import Button from '$lib/ui/Button.svelte';
  import ConfirmModal from '$lib/ui/ConfirmModal.svelte';
  import { toasts } from '$lib/ui/toastStore';
  import { listMappings, deleteMapping, deleteMappingBatch } from './terminologyApi';
  import type { MappingEquivalence, MappingOrigin, ListMappingsQuery } from '$lib/gen/graphql';

  // Use the actual type returned from the query
  type MappingNode = ListMappingsQuery['listMappings']['nodes'][number];

  export let profileId: string | undefined = undefined;
  export let sourceSystem: string | undefined = undefined;
  export let targetSystem: string | undefined = undefined;
  export let pageSize = 25;

  const dispatch = createEventDispatcher<{
    select: { mapping: MappingNode };
    delete: { id: string };
    refresh: void;
  }>();

  // Data state
  let mappings: MappingNode[] = [];
  let totalCount = 0;
  let offset = 0;
  let loading = true;
  let error: string | null = null;

  // Filter state
  let filterSourceSystem = sourceSystem ?? '';
  let filterTargetSystem = targetSystem ?? '';

  // Delete confirmation
  let showDeleteConfirm = false;
  let deletingId: string | null = null;
  let deletingBatchId: string | null = null;

  onMount(() => {
    loadMappings();
  });

  async function loadMappings() {
    loading = true;
    error = null;

    try {
      const result = await listMappings({
        first: pageSize,
        offset,
        profileId: profileId ?? null,
        sourceSystem: filterSourceSystem || null,
        targetSystem: filterTargetSystem || null,
        origin: null,
        uploadBatchId: null
      });
      mappings = result.nodes;
      totalCount = result.totalCount;
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to load mappings';
      toasts.error(error);
    } finally {
      loading = false;
    }
  }

  function applyFilters() {
    offset = 0;
    loadMappings();
  }

  function clearFilters() {
    filterSourceSystem = '';
    filterTargetSystem = '';
    offset = 0;
    loadMappings();
  }

  function prevPage() {
    if (offset > 0) {
      offset = Math.max(0, offset - pageSize);
      loadMappings();
    }
  }

  function nextPage() {
    if (offset + pageSize < totalCount) {
      offset += pageSize;
      loadMappings();
    }
  }

  function selectMapping(mapping: MappingNode) {
    dispatch('select', { mapping });
  }

  function confirmDelete(id: string) {
    deletingId = id;
    deletingBatchId = null;
    showDeleteConfirm = true;
  }

  async function handleDeleteConfirm() {
    try {
      if (deletingId) {
        await deleteMapping(deletingId);
        toasts.success('Mapping deleted');
        dispatch('delete', { id: deletingId });
      } else if (deletingBatchId) {
        const count = await deleteMappingBatch(deletingBatchId);
        toasts.success(`Deleted ${count} mappings from batch`);
      }
      await loadMappings();
    } catch (err) {
      toasts.error(err instanceof Error ? err.message : 'Delete failed');
    } finally {
      showDeleteConfirm = false;
      deletingId = null;
      deletingBatchId = null;
    }
  }

  function formatEquivalence(eq: MappingEquivalence): string {
    switch (eq) {
      case 'EQUIVALENT': return 'Equivalent';
      case 'WIDER': return 'Wider';
      case 'NARROWER': return 'Narrower';
      case 'INEXACT': return 'Inexact';
      default: return String(eq);
    }
  }

  function formatOrigin(origin: MappingOrigin): string {
    switch (origin) {
      case 'CSV_UPLOAD': return 'CSV Upload';
      case 'APPROVED_AUTOROUTE': return 'Approved';
      case 'MANUAL': return 'Manual';
      default: return String(origin);
    }
  }

  function formatDate(dateStr: string): string {
    const date = new Date(dateStr);
    return date.toLocaleDateString('en-US', {
      month: 'short',
      day: 'numeric',
      year: 'numeric'
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

<div class="browser">
  <!-- Filters -->
  <div class="filters">
    <div class="filter-row">
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

  <!-- Table -->
  {#if loading}
    <div class="loading">Loading mappings...</div>
  {:else if error}
    <div class="error-state">
      <div class="error-message">{error}</div>
      <Button variant="secondary" size="sm" on:click={loadMappings}>Retry</Button>
    </div>
  {:else if mappings.length === 0}
    <div class="empty-state">
      <div class="empty-icon">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
          <path d="M4 7h16M4 12h16M4 17h10" />
        </svg>
      </div>
      <div class="empty-text">No mappings found</div>
      <div class="empty-hint">
        {#if filterSourceSystem || filterTargetSystem}
          Try adjusting your filters
        {:else}
          Upload a CSV file to add mappings
        {/if}
      </div>
    </div>
  {:else}
    <div class="table-container">
      <div class="table">
        <div class="table-header">
          <span class="col-source">Source</span>
          <span class="col-target">Target</span>
          <span class="col-equiv">Equiv</span>
          <span class="col-origin">Origin</span>
          <span class="col-date">Created</span>
          <span class="col-actions"></span>
        </div>

        {#each mappings as mapping (mapping.id)}
          <div
            class="table-row"
            on:click={() => selectMapping(mapping)}
            on:keydown={(e) => e.key === 'Enter' && selectMapping(mapping)}
            role="button"
            tabindex="0"
          >
            <span class="col-source">
              <span class="system-label">{truncateSystem(mapping.sourceSystem)}</span>
              <span class="code-value">{mapping.sourceCode}</span>
            </span>
            <span class="col-target">
              <span class="system-label">{truncateSystem(mapping.targetSystem)}</span>
              <span class="code-value">{mapping.targetCode}</span>
            </span>
            <span class="col-equiv">
              <span class="equiv-badge equiv-{mapping.equivalence.toLowerCase()}">
                {formatEquivalence(mapping.equivalence)}
              </span>
            </span>
            <span class="col-origin">
              <span class="origin-badge">{formatOrigin(mapping.origin)}</span>
            </span>
            <span class="col-date">{formatDate(mapping.createdAt)}</span>
            <span class="col-actions">
              <button
                class="icon-btn danger"
                title="Delete mapping"
                on:click|stopPropagation={() => confirmDelete(mapping.id)}
              >
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                  <path d="M3 6h18M19 6v14a2 2 0 01-2 2H7a2 2 0 01-2-2V6m3 0V4a2 2 0 012-2h4a2 2 0 012 2v2" />
                </svg>
              </button>
            </span>
          </div>
        {/each}
      </div>
    </div>

    <!-- Pagination -->
    <div class="pagination">
      <div class="pagination-info">
        Showing {offset + 1}–{Math.min(offset + mappings.length, totalCount)} of {totalCount}
      </div>
      <div class="pagination-controls">
        <button
          class="page-btn"
          on:click={prevPage}
          disabled={offset === 0}
        >
          Previous
        </button>
        <button
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

<ConfirmModal
  bind:open={showDeleteConfirm}
  title={deletingBatchId ? 'Delete Batch?' : 'Delete Mapping?'}
  message={deletingBatchId
    ? 'This will delete all mappings from this upload batch. This action cannot be undone.'
    : 'This will permanently delete the mapping. This action cannot be undone.'}
  confirmText="Delete"
  variant="danger"
  on:confirm={handleDeleteConfirm}
/>

<style>
  .browser {
    display: flex;
    flex-direction: column;
    gap: 16px;
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

  .filter-label {
    font-size: 0.75rem;
    font-weight: 600;
    color: rgba(229, 231, 235, 0.6);
    text-transform: uppercase;
    letter-spacing: 0.02em;
  }

  .filter-input {
    padding: 8px 12px;
    border-radius: 8px;
    border: 1px solid rgba(255, 255, 255, 0.1);
    background: rgba(255, 255, 255, 0.03);
    color: rgba(229, 231, 235, 0.9);
    font-size: 0.9rem;
    outline: none;
  }

  .filter-input:focus {
    border-color: rgba(59, 130, 246, 0.5);
    box-shadow: 0 0 0 2px rgba(59, 130, 246, 0.15);
  }

  .filter-actions {
    display: flex;
    gap: 8px;
  }

  /* Table */
  .table-container {
    border-radius: 8px;
    border: 1px solid rgba(255, 255, 255, 0.06);
    overflow: hidden;
  }

  .table-header {
    display: grid;
    grid-template-columns: 1.5fr 1.5fr 100px 100px 100px 40px;
    gap: 8px;
    padding: 10px 16px;
    background: rgba(255, 255, 255, 0.03);
    font-size: 0.75rem;
    font-weight: 600;
    color: rgba(229, 231, 235, 0.6);
    text-transform: uppercase;
    letter-spacing: 0.02em;
  }

  .table-row {
    display: grid;
    grid-template-columns: 1.5fr 1.5fr 100px 100px 100px 40px;
    gap: 8px;
    padding: 10px 16px;
    border-top: 1px solid rgba(255, 255, 255, 0.04);
    font-size: 0.9rem;
    color: rgba(229, 231, 235, 0.85);
    cursor: pointer;
    transition: background 0.1s;
  }

  .table-row:hover {
    background: rgba(255, 255, 255, 0.03);
  }

  .col-source,
  .col-target {
    display: flex;
    flex-direction: column;
    gap: 2px;
    overflow: hidden;
  }

  .system-label {
    font-size: 0.75rem;
    color: rgba(229, 231, 235, 0.5);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .code-value {
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
    color: rgba(229, 231, 235, 0.9);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .equiv-badge {
    display: inline-block;
    padding: 2px 8px;
    border-radius: 4px;
    font-size: 0.75rem;
    font-weight: 500;
  }

  .equiv-equivalent {
    color: rgba(34, 197, 94, 0.9);
    background: rgba(34, 197, 94, 0.1);
  }

  .equiv-wider {
    color: rgba(59, 130, 246, 0.9);
    background: rgba(59, 130, 246, 0.1);
  }

  .equiv-narrower {
    color: rgba(251, 146, 60, 0.9);
    background: rgba(251, 146, 60, 0.1);
  }

  .equiv-inexact {
    color: rgba(229, 231, 235, 0.7);
    background: rgba(229, 231, 235, 0.1);
  }

  .origin-badge {
    font-size: 0.8rem;
    color: rgba(229, 231, 235, 0.6);
  }

  .col-date {
    font-size: 0.85rem;
    color: rgba(229, 231, 235, 0.6);
  }

  .col-actions {
    display: flex;
    justify-content: flex-end;
  }

  .icon-btn {
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

  .icon-btn svg {
    width: 16px;
    height: 16px;
  }

  .icon-btn:hover {
    color: rgba(229, 231, 235, 0.9);
    background: rgba(255, 255, 255, 0.05);
  }

  .icon-btn.danger:hover {
    color: rgba(239, 68, 68, 0.9);
    background: rgba(239, 68, 68, 0.1);
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
    color: rgba(229, 231, 235, 0.3);
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
</style>
