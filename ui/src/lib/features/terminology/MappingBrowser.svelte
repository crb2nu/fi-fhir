<script lang="ts">
  import { onMount, createEventDispatcher } from 'svelte';
  import Button from '$lib/ui/Button.svelte';
  import ConfirmModal from '$lib/ui/ConfirmModal.svelte';
  import { toasts } from '$lib/ui/toastStore';
  import { listMappings, deleteMapping, deleteMappingBatch, exportMappingsCSV } from './terminologyApi';
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
    edit: { mapping: MappingNode };
    refresh: void;
  }>();

  // Data state
  let mappings: MappingNode[] = [];
  let totalCount = 0;
  let offset = 0;
  let loading = true;
  let error: string | null = null;
  let exporting = false;

  // Filter state
  let filterSourceSystem = sourceSystem ?? '';
  let filterTargetSystem = targetSystem ?? '';
  let filterOrigin: MappingOrigin | '' = '';
  let filterEquivalence: MappingEquivalence | '' = '';
  let filterCreatedAfter = '';
  let filterCreatedBefore = '';

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
        origin: filterOrigin || null,
        equivalence: filterEquivalence || null,
        createdAfter: filterCreatedAfter ? new Date(filterCreatedAfter).toISOString() : null,
        createdBefore: filterCreatedBefore ? new Date(filterCreatedBefore).toISOString() : null,
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
    filterOrigin = '';
    filterEquivalence = '';
    filterCreatedAfter = '';
    filterCreatedBefore = '';
    offset = 0;
    loadMappings();
  }

  function editMapping(mapping: MappingNode) {
    dispatch('edit', { mapping });
  }

  async function handleExport() {
    exporting = true;
    try {
      const csv = await exportMappingsCSV({
        first: null,
        offset: null,
        profileId: profileId ?? null,
        sourceSystem: filterSourceSystem || null,
        targetSystem: filterTargetSystem || null,
        origin: filterOrigin || null,
        equivalence: filterEquivalence || null,
        createdAfter: filterCreatedAfter ? new Date(filterCreatedAfter).toISOString() : null,
        createdBefore: filterCreatedBefore ? new Date(filterCreatedBefore).toISOString() : null,
        uploadBatchId: null
      });

      // Download as file
      const blob = new Blob([csv], { type: 'text/csv;charset=utf-8' });
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `mappings-${new Date().toISOString().split('T')[0]}.csv`;
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      URL.revokeObjectURL(url);

      toasts.success('Mappings exported successfully');
    } catch (err) {
      toasts.error(err instanceof Error ? err.message : 'Export failed');
    } finally {
      exporting = false;
    }
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
  <!-- Header with Export -->
  <div class="browser-header">
    <div class="header-info">
      <span class="total-count">{totalCount} mappings</span>
    </div>
    <Button
      variant="secondary"
      size="sm"
      on:click={handleExport}
      disabled={exporting || totalCount === 0}
    >
      {exporting ? 'Exporting...' : 'Export CSV'}
    </Button>
  </div>

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
    </div>
    <div class="filter-row">
      <label class="filter-field filter-sm">
        <span class="filter-label">Origin</span>
        <select class="filter-select" bind:value={filterOrigin}>
          <option value="">All</option>
          <option value="CSV_UPLOAD">CSV Upload</option>
          <option value="APPROVED_AUTOROUTE">Approved</option>
          <option value="MANUAL">Manual</option>
        </select>
      </label>
      <label class="filter-field filter-sm">
        <span class="filter-label">Equivalence</span>
        <select class="filter-select" bind:value={filterEquivalence}>
          <option value="">All</option>
          <option value="EQUIVALENT">Equivalent</option>
          <option value="WIDER">Wider</option>
          <option value="NARROWER">Narrower</option>
          <option value="INEXACT">Inexact</option>
        </select>
      </label>
      <label class="filter-field filter-sm">
        <span class="filter-label">Created After</span>
        <input
          type="date"
          class="filter-input"
          bind:value={filterCreatedAfter}
        />
      </label>
      <label class="filter-field filter-sm">
        <span class="filter-label">Created Before</span>
        <input
          type="date"
          class="filter-input"
          bind:value={filterCreatedBefore}
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
          <div class="table-row">
            <button
              type="button"
              class="row-main"
              on:click={() => selectMapping(mapping)}
              aria-label="Select mapping {truncateSystem(mapping.sourceSystem)} {mapping.sourceCode} to {truncateSystem(mapping.targetSystem)} {mapping.targetCode}"
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
            </button>
            <span class="col-actions">
              <button
                type="button"
                class="icon-btn"
                aria-label="Edit mapping"
                title="Edit mapping"
                on:click={() => editMapping(mapping)}
              >
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                  <path d="M11 4H4a2 2 0 00-2 2v14a2 2 0 002 2h14a2 2 0 002-2v-7" />
                  <path d="M18.5 2.5a2.121 2.121 0 013 3L12 15l-4 1 1-4 9.5-9.5z" />
                </svg>
              </button>
              <button
                type="button"
                class="icon-btn danger"
                aria-label="Delete mapping"
                title="Delete mapping"
                on:click={() => confirmDelete(mapping.id)}
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
    gap: var(--space-4);
  }

  /* Header */
  .browser-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
  }

  .header-info {
    display: flex;
    align-items: center;
    gap: var(--space-3);
  }

  .total-count {
    font-size: var(--text-sm);
    color: var(--color-text-tertiary);
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

  .filter-row + .filter-row {
    margin-top: var(--space-3);
  }

  .filter-field {
    flex: 1;
    min-width: 140px;
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
  }

  .filter-field.filter-sm {
    flex: 0.7;
    min-width: 100px;
  }

  .filter-label {
    font-size: var(--text-xs);
    font-weight: var(--font-semibold);
    color: var(--color-text-tertiary);
    text-transform: uppercase;
    letter-spacing: var(--tracking-wide);
  }

  .filter-input {
    padding: var(--space-2) var(--space-3);
    border-radius: var(--radius-lg);
    border: 1px solid var(--color-border-default);
    background: var(--color-bg-input);
    color: var(--color-text-primary);
    font-size: var(--text-sm);
    outline: none;
    transition: var(--transition-all);
  }

  .filter-input:focus {
    border-color: var(--color-border-focus);
    box-shadow: var(--shadow-focus);
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
  }

  /* Table */
  .table-container {
    border-radius: var(--radius-lg);
    border: 1px solid var(--color-border-subtle);
    overflow: hidden;
  }

  .table-header {
    display: grid;
    grid-template-columns: 1.5fr 1.5fr 100px 100px 100px 70px;
    gap: var(--space-2);
    padding: var(--space-3) var(--space-4);
    background: var(--color-bg-elevated);
    font-size: var(--text-xs);
    font-weight: var(--font-semibold);
    color: var(--color-text-tertiary);
    text-transform: uppercase;
    letter-spacing: var(--tracking-wide);
  }

  .table-row {
    display: grid;
    grid-template-columns: 1.5fr 1.5fr 100px 100px 100px 70px;
    gap: var(--space-2);
    padding: var(--space-3) var(--space-4);
    border-top: 1px solid var(--color-border-subtle);
    font-size: var(--text-sm);
    color: var(--color-text-secondary);
    transition: var(--transition-colors);
  }

  .table-row:hover {
    background: var(--color-bg-hover);
  }

  .row-main {
    grid-column: 1 / span 5;
    display: grid;
    grid-template-columns: 1.5fr 1.5fr 100px 100px 100px;
    gap: var(--space-2);
    align-items: center;
    padding: 0;
    margin: 0;
    border: 0;
    background: transparent;
    color: inherit;
    font: inherit;
    text-align: left;
    cursor: pointer;
  }

  .row-main:focus-visible {
    outline: none;
    background: var(--color-bg-hover);
    box-shadow: inset 0 0 0 2px var(--color-primary-border);
  }

  .col-source,
  .col-target {
    display: flex;
    flex-direction: column;
    gap: 2px;
    overflow: hidden;
  }

  .system-label {
    font-size: var(--text-xs);
    color: var(--color-text-muted);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .code-value {
    font-family: var(--font-mono);
    color: var(--color-text-primary);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .equiv-badge {
    display: inline-block;
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

  .origin-badge {
    font-size: var(--text-xs);
    color: var(--color-text-tertiary);
  }

  .col-date {
    font-size: var(--text-xs);
    color: var(--color-text-tertiary);
  }

  .col-actions {
    display: flex;
    justify-content: flex-end;
    gap: var(--space-1);
  }

  .icon-btn {
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

  .icon-btn svg {
    width: 16px;
    height: 16px;
  }

  .icon-btn:hover {
    color: var(--color-text-primary);
    background: var(--color-bg-hover);
  }

  .icon-btn:focus-visible {
    outline: none;
    box-shadow: var(--shadow-focus);
  }

  .icon-btn.danger:hover {
    color: var(--color-danger-text);
    background: var(--color-danger-bg);
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
    font-size: var(--text-sm);
  }

  .empty-hint {
    color: var(--color-text-muted);
    font-size: var(--text-xs);
  }

  /* Responsive */
  @media (max-width: 768px) {
    .table-header,
    .table-row {
      grid-template-columns: 1fr 1fr auto;
    }

    .col-origin,
    .col-date {
      display: none;
    }
  }
</style>
