<script lang="ts">
  import { onMount, createEventDispatcher } from "svelte";
  import ChevronLeft from "@lucide/svelte/icons/chevron-left";
  import ChevronRight from "@lucide/svelte/icons/chevron-right";
  import CircleAlert from "@lucide/svelte/icons/circle-alert";
  import Download from "@lucide/svelte/icons/download";
  import Inbox from "@lucide/svelte/icons/inbox";
  import Pencil from "@lucide/svelte/icons/pencil";
  import Trash2 from "@lucide/svelte/icons/trash-2";
  import {
    Badge,
    Button,
    EmptyState,
    Icon,
    Input,
    KeyValue,
    Select,
    Table,
    Td,
    Th,
    Tr,
    type SelectOption,
  } from "$lib/ui/primitives";
  import ConfirmModal from "$lib/ui/ConfirmModal.svelte";
  import { toasts } from "$lib/ui/toastStore";
  import { isErrorToasted } from "$lib/graphql/client";
  import {
    listMappings,
    deleteMapping,
    deleteMappingBatch,
    exportMappingsCSV,
  } from "./terminologyApi";
  import {
    equivalenceLabel,
    equivalenceTone,
    formatPercent,
    formatTimestamp,
    originLabel,
  } from "./terminologyFormat";
  import type {
    MappingEquivalence,
    MappingOrigin,
    ListMappingsQuery,
  } from "$lib/gen/graphql";

  // Use the actual type returned from the query
  type MappingNode = ListMappingsQuery["listMappings"]["nodes"][number];

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

  const originOptions: SelectOption[] = [
    { value: "", label: "All origins" },
    { value: "CSV_UPLOAD", label: "CSV upload" },
    { value: "APPROVED_AUTOROUTE", label: "Approved autoroute" },
    { value: "MANUAL", label: "Manual" },
  ];

  const equivalenceOptions: SelectOption[] = [
    { value: "", label: "All equivalences" },
    { value: "EQUIVALENT", label: "Equivalent" },
    { value: "WIDER", label: "Wider" },
    { value: "NARROWER", label: "Narrower" },
    { value: "INEXACT", label: "Inexact" },
  ];

  // Data state
  let mappings: MappingNode[] = [];
  let totalCount = 0;
  let offset = 0;
  let loading = true;
  let error: string | null = null;
  let exporting = false;

  // Filter state
  let filterSourceSystem = sourceSystem ?? "";
  let filterTargetSystem = targetSystem ?? "";
  let filterOrigin: MappingOrigin | "" = "";
  let filterEquivalence: MappingEquivalence | "" = "";
  let filterCreatedAfter = "";
  let filterCreatedBefore = "";

  // Selection (details pane)
  let selectedId: string | null = null;

  // Delete confirmation
  let showDeleteConfirm = false;
  let deletingId: string | null = null;
  let deletingBatchId: string | null = null;

  $: selected = mappings.find((m) => m.id === selectedId) ?? null;
  $: hasFilters = Boolean(
    filterSourceSystem ||
      filterTargetSystem ||
      filterOrigin ||
      filterEquivalence ||
      filterCreatedAfter ||
      filterCreatedBefore,
  );
  $: selectedItems = selected
    ? [
        { key: "Source system", value: selected.sourceSystem, mono: true },
        { key: "Source code", value: selected.sourceCode, mono: true },
        { key: "Source display", value: selected.sourceDisplay },
        { key: "Target system", value: selected.targetSystem, mono: true },
        { key: "Target code", value: selected.targetCode, mono: true },
        { key: "Target display", value: selected.targetDisplay },
        { key: "Equivalence", value: equivalenceLabel(selected.equivalence) },
        {
          key: "Confidence",
          value: formatPercent(selected.confidence),
          mono: true,
        },
        { key: "Origin", value: originLabel(selected.origin) },
        { key: "Profile", value: selected.profileId, mono: true },
        {
          key: "Upload batch",
          value: selected.uploadBatchId,
          mono: true,
          truncate: true,
        },
        { key: "Created", value: formatTimestamp(selected.createdAt), mono: true },
        { key: "Created by", value: selected.createdBy },
        { key: "Comment", value: selected.comment },
        { key: "Mapping id", value: selected.id, mono: true, truncate: true },
      ]
    : [];

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
        createdAfter: filterCreatedAfter
          ? new Date(filterCreatedAfter).toISOString()
          : null,
        createdBefore: filterCreatedBefore
          ? new Date(filterCreatedBefore).toISOString()
          : null,
        uploadBatchId: null,
      });
      mappings = result.nodes;
      totalCount = result.totalCount;
      // Keep the details pane on a row that is still listed.
      if (!mappings.some((m) => m.id === selectedId)) {
        selectedId = mappings[0]?.id ?? null;
      }
    } catch (err) {
      error = err instanceof Error ? err.message : "Failed to load mappings";
    } finally {
      loading = false;
    }
  }

  function applyFilters() {
    offset = 0;
    loadMappings();
  }

  function clearFilters() {
    filterSourceSystem = "";
    filterTargetSystem = "";
    filterOrigin = "";
    filterEquivalence = "";
    filterCreatedAfter = "";
    filterCreatedBefore = "";
    offset = 0;
    loadMappings();
  }

  function applyOnEnter(event: KeyboardEvent) {
    if (event.key === "Enter") applyFilters();
  }

  function editMapping(mapping: MappingNode) {
    dispatch("edit", { mapping });
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
        createdAfter: filterCreatedAfter
          ? new Date(filterCreatedAfter).toISOString()
          : null,
        createdBefore: filterCreatedBefore
          ? new Date(filterCreatedBefore).toISOString()
          : null,
        uploadBatchId: null,
      });

      // Download as file
      const blob = new Blob([csv], { type: "text/csv;charset=utf-8" });
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = `mappings-${new Date().toISOString().split("T")[0]}.csv`;
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      URL.revokeObjectURL(url);

      toasts.success("Mappings exported successfully");
    } catch (err) {
      // Global graphqlFetch net already toasts graphql failures (B4 dedupe).
      if (!isErrorToasted(err)) {
        toasts.error(err instanceof Error ? err.message : "Export failed");
      }
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
    selectedId = mapping.id;
    dispatch("select", { mapping });
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
        toasts.success("Mapping deleted");
        dispatch("delete", { id: deletingId });
      } else if (deletingBatchId) {
        const count = await deleteMappingBatch(deletingBatchId);
        toasts.success(`Deleted ${count} mappings from batch`);
      }
      await loadMappings();
    } catch (err) {
      // Global graphqlFetch net already toasts graphql failures (B4 dedupe).
      if (!isErrorToasted(err)) {
        toasts.error(err instanceof Error ? err.message : "Delete failed");
      }
    } finally {
      showDeleteConfirm = false;
      deletingId = null;
      deletingBatchId = null;
    }
  }
</script>

<div class="browser">
  <div class="filters">
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
    <div class="filter-select">
      <Select
        bind:value={filterOrigin}
        options={originOptions}
        aria-label="Origin"
      />
    </div>
    <div class="filter-select">
      <Select
        bind:value={filterEquivalence}
        options={equivalenceOptions}
        aria-label="Equivalence"
      />
    </div>
    <div class="filter-dates">
      <span class="text-label" aria-hidden="true">Created</span>
      <div class="filter-date">
        <Input
          type="date"
          bind:value={filterCreatedAfter}
          aria-label="Created after"
          title="Created after"
          mono
        />
      </div>
      <span class="filter-sep" aria-hidden="true">to</span>
      <div class="filter-date">
        <Input
          type="date"
          bind:value={filterCreatedBefore}
          aria-label="Created before"
          title="Created before"
          mono
        />
      </div>
    </div>
    <Button variant="ghost" onclick={clearFilters} disabled={!hasFilters}
      >Clear</Button
    >
    <Button variant="primary" onclick={applyFilters}>Apply</Button>

    <div class="filters-end">
      <span class="filter-count text-mono" aria-live="polite"
        >{totalCount} mappings</span
      >
      <Button
        variant="ghost"
        icon={Download}
        onclick={handleExport}
        loading={exporting}
        disabled={totalCount === 0}
      >
        Export CSV
      </Button>
    </div>
  </div>

  {#if loading}
    <EmptyState message="Loading mappings…" aria-busy="true" />
  {:else if error}
    <EmptyState icon={CircleAlert} message="Mappings could not be loaded: {error}">
      {#snippet action()}
        <Button onclick={loadMappings}>Retry</Button>
      {/snippet}
    </EmptyState>
  {:else if mappings.length === 0}
    <EmptyState
      icon={Inbox}
      message={hasFilters
        ? "No mappings match these filters."
        : "No mappings yet. Upload a CSV to add some."}
    />
  {:else}
    <div class="split">
      <div class="list">
        <Table label="Mappings" layout="fixed" class="mapping-table">
          {#snippet head()}
            <tr>
              <Th>Source system</Th>
              <Th width="120px">Source code</Th>
              <Th>Target system</Th>
              <Th width="120px">Target code</Th>
              <Th width="112px">Equivalence</Th>
              <Th width="144px">Origin</Th>
              <Th width="96px" numeric>Confidence</Th>
              <Th width="104px">Created</Th>
            </tr>
          {/snippet}
          {#each mappings as mapping (mapping.id)}
            <Tr
              selectable
              selected={mapping.id === selectedId}
              onselect={() => selectMapping(mapping)}
              aria-label="Mapping {mapping.sourceCode} to {mapping.targetCode}"
            >
              <Td muted truncate value={mapping.sourceSystem} />
              <Td mono truncate value={mapping.sourceCode} />
              <Td muted truncate value={mapping.targetSystem} />
              <Td mono truncate value={mapping.targetCode} />
              <Td>
                <Badge tone={equivalenceTone(mapping.equivalence)}
                  >{equivalenceLabel(mapping.equivalence)}</Badge
                >
              </Td>
              <Td truncate value={originLabel(mapping.origin)} />
              <Td numeric value={formatPercent(mapping.confidence) || "—"} />
              <Td
                mono
                muted
                value={formatTimestamp(mapping.createdAt, false)}
                title={formatTimestamp(mapping.createdAt)}
              />
            </Tr>
          {/each}
        </Table>

        {#if totalCount > pageSize || offset > 0}
          <div class="pagination">
            <span class="pagination-info text-mono">
              {offset + 1}–{Math.min(offset + mappings.length, totalCount)} of {totalCount}
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

      <aside class="details" aria-label="Selected mapping">
        {#if selected}
          <div class="details-head">
            <span class="details-title text-mono" title="{selected.sourceCode} → {selected.targetCode}"
              >{selected.sourceCode} → {selected.targetCode}</span
            >
            <Badge tone={equivalenceTone(selected.equivalence)}
              >{equivalenceLabel(selected.equivalence)}</Badge
            >
          </div>
          <div class="details-actions">
            <Button
              icon={Pencil}
              aria-label="Edit mapping"
              onclick={() => selected && editMapping(selected)}
            >
              Edit
            </Button>
            <Button
              variant="danger"
              icon={Trash2}
              aria-label="Delete mapping"
              onclick={() => selected && confirmDelete(selected.id)}
            >
              Delete
            </Button>
          </div>
          <KeyValue items={selectedItems} />
        {:else}
          <EmptyState message="Select a mapping to see its details." />
        {/if}
      </aside>
    </div>
  {/if}
</div>

<ConfirmModal
  bind:open={showDeleteConfirm}
  title={deletingBatchId ? "Delete batch?" : "Delete mapping?"}
  message={deletingBatchId
    ? "This will delete all mappings from this upload batch. This action cannot be undone."
    : "This will permanently delete the mapping. This action cannot be undone."}
  confirmText="Delete"
  variant="danger"
  on:confirm={handleDeleteConfirm}
/>

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

  .filter-text {
    width: 160px;
  }

  .filter-select {
    width: 144px;
  }

  .filter-dates {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }

  .filter-date {
    width: 128px;
  }

  .filter-sep {
    font-size: var(--text-xs);
    color: var(--color-text-tertiary);
  }

  .filters-end {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    margin-left: auto;
  }

  .filter-count {
    color: var(--color-text-tertiary);
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

  .list :global(.mapping-table) {
    flex: 1 1 auto;
  }

  /* Fixed layout gives the unsized columns only what is left; keep a floor
     so they never collapse, and let the wrapper scroll sideways instead. */
  .list :global(.mapping-table > table) {
    min-width: 880px;
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
</style>
