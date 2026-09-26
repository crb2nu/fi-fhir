<script lang="ts">
  import ConfirmModal from '$lib/ui/ConfirmModal.svelte';
  import {
    Badge,
    Button,
    EmptyState,
    Field,
    Icon,
    IconButton,
    Input,
    KeyValue,
    Panel,
    Select,
    Table,
    Td,
    Th,
    Tr
  } from '$lib/ui/primitives';
  import type { SelectOption } from '$lib/ui/primitives';
  import Eraser from '@lucide/svelte/icons/eraser';
  import Pencil from '@lucide/svelte/icons/pencil';
  import X from '@lucide/svelte/icons/x';
  import Files from '@lucide/svelte/icons/files';
  import InboxIcon from '@lucide/svelte/icons/inbox';
  import Save from '@lucide/svelte/icons/save';
  import Search from '@lucide/svelte/icons/search';
  import Trash2 from '@lucide/svelte/icons/trash-2';
  import Upload from '@lucide/svelte/icons/upload';
  import type { HL7Sample } from '$lib/features/hl7/samples/types';
  import type { HL7RedactionMode } from '$lib/domain/hl7Redact';
  import { createEventDispatcher, tick } from 'svelte';

  export let samples: readonly HL7Sample[];
  export let activeId: string | null;
  export let disabled = false;

  export let currentRaw: string;

  const dispatch = createEventDispatcher<{
    select: { id: string };
    remove: { id: string };
    saveCurrent: { name?: string; source?: string; feed?: string; tags?: string[]; redactionMode?: HL7RedactionMode };
    importFiles: { files: File[]; source?: string; feed?: string; tags?: string[]; redactionMode?: HL7RedactionMode };
    updateMeta: {
      id: string;
      name: string;
      source: string;
      feed: string;
      tags: string[];
      redactionMode?: HL7RedactionMode;
    };
    bulkRemove: { ids: string[] };
    bulkUpdateMeta: { ids: string[]; changes: { tags?: string[]; redactionMode?: HL7RedactionMode } };
    clear: Record<string, never>;
    loadExamples: Record<string, never>;
  }>();

  const redactionOptions: SelectOption[] = [
    { value: 'none', label: 'None' },
    { value: 'mask_basic', label: 'Mask basic (PID/NK1/PV1)' },
    { value: 'segment_sanitize', label: 'Sanitize segments (PID/NK1/IN*)' }
  ];

  // Metadata applied by "Save current", "Import files" and dropped files.
  let name = '';
  let feed = '';
  let tags = '';
  let sourceOverride = '';
  let filter = '';
  let redactionMode: HL7RedactionMode = 'none';
  let fileInputEl: HTMLInputElement | null = null;
  let isDragging = false;

  // Details pane: edits one sample's metadata. It follows the active sample
  // unless a row's "Edit sample" picked another one; that never loads the
  // sample into the editor.
  let editingId: string | null = null;
  let editKey = '';
  let editNameEl: HTMLDivElement | null = null;
  let editName = '';
  let editSource = '';
  let editFeed = '';
  let editTags = '';

  // Bulk selection. This is a legacy-mode component: the markup only updates
  // when a top-level variable is reassigned, so the set is replaced on every
  // change, never mutated in place.
  let selectedIds: ReadonlySet<string> = new Set<string>();
  let bulkDeleteOpen = false;

  function toggleSelected(id: string): void {
    selectedIds = selectedIds.has(id)
      ? new Set(Array.from(selectedIds).filter((x) => x !== id))
      : new Set([...selectedIds, id]);
  }

  function clearSelected(): void {
    selectedIds = new Set<string>();
  }

  function selectAllFiltered(): void {
    selectedIds = new Set(filtered.map((s) => s.id));
  }

  function countSelected(list: readonly HL7Sample[], ids: ReadonlySet<string>): number {
    return list.filter((s) => ids.has(s.id)).length;
  }

  function allSelected(list: readonly HL7Sample[], ids: ReadonlySet<string>): boolean {
    return list.length > 0 && countSelected(list, ids) === list.length;
  }

  function someSelected(list: readonly HL7Sample[], ids: ReadonlySet<string>): boolean {
    const n = countSelected(list, ids);
    return n > 0 && n < list.length;
  }

  function toggleAllFiltered(): void {
    if (allSelected(filtered, selectedIds)) clearSelected();
    else selectAllFiltered();
  }

  function requestBulkDelete(): void {
    if (selectedIds.size === 0) return;
    bulkDeleteOpen = true;
  }

  function confirmBulkDelete(): void {
    const ids = Array.from(selectedIds);
    if (ids.length === 0) return;
    dispatch('bulkRemove', { ids });
    clearSelected();
    bulkDeleteOpen = false;
  }

  function applyTagsToSelected(): void {
    const parsedTags = parseTags(tags);
    if (parsedTags.length === 0) return;
    const ids = Array.from(selectedIds);
    if (ids.length === 0) return;
    dispatch('bulkUpdateMeta', { ids, changes: { tags: parsedTags } });
  }

  function clearTagsOnSelected(): void {
    const ids = Array.from(selectedIds);
    if (ids.length === 0) return;
    dispatch('bulkUpdateMeta', { ids, changes: { tags: [] } });
  }

  function applyRedactionToSelected(mode: HL7RedactionMode): void {
    const ids = Array.from(selectedIds);
    if (ids.length === 0) return;
    dispatch('bulkUpdateMeta', { ids, changes: { redactionMode: mode } });
  }

  function save() {
    const n = name.trim();
    const so = sourceOverride.trim();
    const f = feed.trim();
    const parsedTags = parseTags(tags);
    dispatch('saveCurrent', {
      ...(n ? { name: n } : {}),
      ...(so ? { source: so } : {}),
      ...(f ? { feed: f } : {}),
      ...(parsedTags.length ? { tags: parsedTags } : {}),
      ...(redactionMode !== 'none' ? { redactionMode } : {})
    });
    name = '';
  }

  function triggerImport() {
    fileInputEl?.click();
  }

  function onFileChange(e: Event) {
    const input = e.currentTarget as HTMLInputElement;
    const files = input.files ? Array.from(input.files) : [];
    if (files.length) {
      const so = sourceOverride.trim();
      const f = feed.trim();
      const parsedTags = parseTags(tags);
      dispatch('importFiles', {
        files,
        ...(so ? { source: so } : {}),
        ...(f ? { feed: f } : {}),
        ...(parsedTags.length ? { tags: parsedTags } : {}),
        ...(redactionMode !== 'none' ? { redactionMode } : {})
      });
    }
    input.value = '';
  }

  function onDragOver(e: DragEvent) {
    if (disabled) return;
    if (!e.dataTransfer?.types?.includes('Files')) return;
    e.preventDefault();
    isDragging = true;
  }

  function onDragLeave() {
    isDragging = false;
  }

  function onDrop(e: DragEvent) {
    if (disabled) return;
    const files = e.dataTransfer?.files ? Array.from(e.dataTransfer.files) : [];
    if (!files.length) return;
    e.preventDefault();
    isDragging = false;
    const so = sourceOverride.trim();
    const f = feed.trim();
    const parsedTags = parseTags(tags);
    dispatch('importFiles', {
      files,
      ...(so ? { source: so } : {}),
      ...(f ? { feed: f } : {}),
      ...(parsedTags.length ? { tags: parsedTags } : {}),
      ...(redactionMode !== 'none' ? { redactionMode } : {})
    });
  }

  function parseTags(raw: string): string[] {
    const parts = raw
      .split(',')
      .map((x) => x.trim())
      .filter((x) => x.length > 0);
    const uniq: string[] = [];
    for (const t of parts) {
      if (!uniq.includes(t)) uniq.push(t);
      if (uniq.length >= 12) break;
    }
    return uniq;
  }

  function metaKey(sample: HL7Sample | null): string {
    if (!sample) return '';
    return [sample.id, sample.name, sample.source, sample.feed ?? '', (sample.tags ?? []).join(',')].join('\u0000');
  }

  function resetEdit(sample: HL7Sample | null): void {
    editName = sample?.name ?? '';
    editSource = sample?.source ?? '';
    editFeed = sample?.feed ?? '';
    editTags = (sample?.tags ?? []).join(', ');
  }

  function saveEdit(): void {
    if (!detailSample) return;
    const name = editName.trim();
    const source = editSource.trim();
    if (!name || !source) return;
    const feed = editFeed.trim();
    const tags = parseTags(editTags);
    dispatch('updateMeta', {
      id: detailSample.id,
      name,
      source,
      feed,
      tags
    });
  }

  function openSample(id: string): void {
    if (disabled) return;
    editingId = null;
    dispatch('select', { id });
  }

  function editSample(event: MouseEvent, id: string): void {
    // The row loads its sample on click; editing metadata must not.
    event.preventDefault();
    editingId = id;
    void tick().then(() => editNameEl?.querySelector('input')?.focus());
  }

  function stopEditing(): void {
    editingId = null;
  }

  function removeSample(event: MouseEvent, id: string): void {
    // The row opens the sample on click; removing must not also open it.
    event.preventDefault();
    dispatch('remove', { id });
  }

  function keepRowClosed(event: MouseEvent): void {
    // Toggling the bulk checkbox must not open the row's sample.
    event.stopPropagation();
  }

  function segmentCount(raw: string): number {
    return raw.split(/\r\n|\r|\n/).filter((line) => line.trim().length > 0).length;
  }

  function formatSize(raw: string): string {
    const bytes = new TextEncoder().encode(raw).length;
    return bytes < 1024 ? `${bytes} B` : `${(bytes / 1024).toFixed(1)} KB`;
  }

  function redactionLabel(mode: HL7RedactionMode | undefined): string {
    return redactionOptions.find((o) => o.value === mode)?.label ?? mode ?? 'None';
  }

  function indeterminate(node: HTMLInputElement, value: boolean) {
    node.indeterminate = value;
    return {
      update(next: boolean) {
        node.indeterminate = next;
      }
    };
  }

  $: filtered = filter.trim()
    ? samples.filter((s) => {
        const q = filter.trim().toLowerCase();
        const hay = [
          s.name,
          s.source,
          s.feed ?? '',
          s.messageType ?? '',
          s.version ?? '',
          ...(s.tags ?? [])
        ]
          .join(' ')
          .toLowerCase();
        return hay.includes(q);
      })
    : samples;

  // Drop selections whose sample was removed.
  $: {
    const valid = new Set(samples.map((s) => s.id));
    const kept = Array.from(selectedIds).filter((id) => valid.has(id));
    if (kept.length !== selectedIds.size) selectedIds = new Set(kept);
  }

  // A sample picked for editing that has since been removed releases the pane.
  $: if (editingId && !samples.some((s) => s.id === editingId)) editingId = null;
  $: detailSample = samples.find((s) => s.id === (editingId ?? activeId)) ?? null;
  $: detailIsActive = detailSample !== null && detailSample.id === activeId;
  $: {
    const key = metaKey(detailSample);
    if (key !== editKey) {
      editKey = key;
      resetEdit(detailSample);
    }
  }
  $: editDirty =
    detailSample !== null &&
    (editName !== detailSample.name ||
      editSource !== detailSample.source ||
      editFeed !== (detailSample.feed ?? '') ||
      parseTags(editTags).join(',') !== (detailSample.tags ?? []).join(','));
  $: editValid = editName.trim().length > 0 && editSource.trim().length > 0;
</script>

<div
  class="inbox"
  class:dragging={isDragging}
  on:dragover={onDragOver}
  on:dragleave={onDragLeave}
  on:drop={onDrop}
  role="region"
  aria-label="Samples inbox. Drag and drop HL7 files to import."
>
  <div class="action-row">
    <Button icon={Upload} onclick={triggerImport} {disabled}>Import files</Button>
    <Button icon={Save} onclick={save} disabled={disabled || !currentRaw.trim()}>Save current</Button>
    <Button variant="ghost" icon={Files} onclick={() => dispatch('loadExamples', {})} {disabled}>
      Load examples
    </Button>
    <Button
      variant="ghost"
      icon={Eraser}
      onclick={() => dispatch('clear', {})}
      disabled={disabled || samples.length === 0}
    >
      Clear
    </Button>
    <input
      class="file-input"
      type="file"
      multiple
      accept=".hl7,.txt,.msg,.dat,text/plain"
      bind:this={fileInputEl}
      on:change={onFileChange}
      {disabled}
    />
  </div>

  <p class="note">
    Samples stay in this tab's memory and clear on reload. Paste PHI only on an approved machine and profile.
  </p>

  <Panel title="Samples" titleTag="h3" flush>
    {#snippet actions()}
      {#if samples.length > 0}
        <div class="filter-search">
          <Icon icon={Search} size={14} class="filter-search-icon" />
          <Input
            bind:value={filter}
            placeholder="Filter by name, source, feed, type, tag"
            aria-label="Filter samples"
            {disabled}
          />
        </div>
        {#if filter.trim()}
          <Button variant="ghost" onclick={() => (filter = '')} {disabled}>Clear filter</Button>
        {/if}
        <span class="count text-mono">{filtered.length}/{samples.length}</span>
      {/if}
    {/snippet}

    {#if samples.length === 0}
      <EmptyState
        align="start"
        icon={InboxIcon}
        message="No saved samples. Import files, save the current message or load the examples."
      />
    {:else}
      {#if selectedIds.size > 0}
        <div class="bulk-bar" role="toolbar" aria-label="Bulk actions">
          <span class="bulk-count text-mono">{selectedIds.size} selected</span>
          <Button
            variant="ghost"
            onclick={applyTagsToSelected}
            disabled={disabled || parseTags(tags).length === 0}
            title="Applies the Tags option below"
          >
            Apply tags
          </Button>
          <Button variant="ghost" onclick={clearTagsOnSelected} {disabled}>Clear tags</Button>
          <Button
            variant="ghost"
            onclick={() => applyRedactionToSelected(redactionMode)}
            {disabled}
            title="Applies the Redaction option below"
          >
            Set redaction
          </Button>
          <Button variant="ghost" onclick={() => applyRedactionToSelected('none')} {disabled}>
            Clear redaction
          </Button>
          <span class="bulk-end">
            <Button variant="ghost" onclick={clearSelected} {disabled}>Clear selection</Button>
            <Button variant="danger" icon={Trash2} onclick={requestBulkDelete} {disabled}>Delete</Button>
          </span>
        </div>
      {/if}

      {#if filtered.length === 0}
        <EmptyState align="start" icon={Search} message="No samples match the filter." />
      {:else}
        <Table label="Saved samples" layout="fixed" class="samples-table">
          {#snippet head()}
            <tr>
              <Th width="36px">
                <input
                  class="check"
                  type="checkbox"
                  checked={allSelected(filtered, selectedIds)}
                  use:indeterminate={someSelected(filtered, selectedIds)}
                  on:change={toggleAllFiltered}
                  {disabled}
                  aria-label="Select all shown samples"
                />
              </Th>
              <Th>Name</Th>
              <Th width="112px">Source</Th>
              <Th width="96px">Feed</Th>
              <Th width="112px">Tags</Th>
              <Th width="92px">Redaction</Th>
              <Th width="52px" numeric>Segs</Th>
              <Th width="76px"><span class="sr-only">Actions</span></Th>
            </tr>
          {/snippet}
          {#each filtered as s (s.id)}
            <Tr
              selectable
              selected={activeId === s.id}
              class={editingId === s.id && activeId !== s.id ? 'is-editing' : undefined}
              onselect={() => openSample(s.id)}
            >
              <Td>
                <input
                  class="check"
                  type="checkbox"
                  checked={selectedIds.has(s.id)}
                  on:click={keepRowClosed}
                  on:change={() => toggleSelected(s.id)}
                  {disabled}
                  aria-label={`Select ${s.name}`}
                />
              </Td>
              <Td truncate value={s.name} />
              <Td mono truncate value={s.source} />
              <Td mono muted truncate value={s.feed ?? ''} />
              <Td truncate title={(s.tags ?? []).join(', ')}>
                {#each s.tags ?? [] as t (t)}
                  <Badge class="tag">{t}</Badge>
                {/each}
              </Td>
              <Td>
                {#if s.redactionMode && s.redactionMode !== 'none'}
                  <Badge tone="warning" title={redactionLabel(s.redactionMode)}>Redacted</Badge>
                {/if}
              </Td>
              <Td numeric value={segmentCount(s.raw)} />
              <Td class="row-actions">
                <IconButton
                  icon={Pencil}
                  label="Edit sample"
                  title={`Edit ${s.name} without loading it`}
                  pressed={editingId === s.id ? true : undefined}
                  onclick={(e) => editSample(e, s.id)}
                  {disabled}
                />
                <IconButton
                  icon={Trash2}
                  label={`Remove ${s.name}`}
                  onclick={(e) => removeSample(e, s.id)}
                  {disabled}
                />
              </Td>
            </Tr>
          {/each}
        </Table>
      {/if}
    {/if}
  </Panel>

  {#if detailSample}
    <Panel aria-label="Selected sample">
      {#snippet header()}
        <h3 class="details-title" title={detailSample?.name}>{detailSample?.name}</h3>
        {#if detailSample?.messageType}
          <Badge mono>{detailSample.messageType}</Badge>
        {/if}
        {#if detailSample?.version}
          <Badge mono>{detailSample.version}</Badge>
        {/if}
        {#if !detailIsActive}
          <Badge title="Editing metadata only; the editor still holds the active sample">Not loaded</Badge>
        {/if}
      {/snippet}
      {#snippet actions()}
        <Button variant="ghost" onclick={() => resetEdit(detailSample)} disabled={disabled || !editDirty}>
          Revert
        </Button>
        <Button onclick={saveEdit} disabled={disabled || !editDirty || !editValid}>Save changes</Button>
        {#if !detailIsActive}
          <IconButton icon={X} label="Stop editing" onclick={stopEditing} />
        {/if}
      {/snippet}

      <div class="details">
        <KeyValue
          columns={2}
          items={[
            { key: 'Control id', value: detailSample.controlId, mono: true, truncate: true },
            { key: 'Saved', value: new Date(detailSample.createdAt).toLocaleString(), mono: true },
            { key: 'Segments', value: segmentCount(detailSample.raw), mono: true },
            { key: 'Size', value: formatSize(detailSample.raw), mono: true },
            { key: 'Redaction', value: redactionLabel(detailSample.redactionMode) }
          ]}
        />
        <div class="form-grid" role="group" aria-label="Edit sample metadata">
          <div bind:this={editNameEl}>
            <Field label="Name" required>
              <Input bind:value={editName} {disabled} />
            </Field>
          </div>
          <Field label="Source" required>
            <Input mono bind:value={editSource} {disabled} />
          </Field>
          <Field label="Feed">
            <Input mono bind:value={editFeed} {disabled} />
          </Field>
          <Field label="Tags">
            <Input bind:value={editTags} placeholder="Comma-separated" {disabled} />
          </Field>
        </div>
      </div>
    </Panel>
  {/if}

  <Panel title="Save and import options" titleTag="h3">
    <div class="form-grid" role="group" aria-label="Metadata for saved and imported samples">
      <Field label="Sample name">
        <Input bind:value={name} placeholder="ADT A01 - ICU admit" {disabled} />
      </Field>
      <Field label="Source override">
        <Input mono bind:value={sourceOverride} placeholder="Current source or file name" {disabled} />
      </Field>
      <Field label="Feed">
        <Input mono bind:value={feed} placeholder="epic_adt_icu" {disabled} />
      </Field>
      <Field label="Tags">
        <Input bind:value={tags} placeholder="Comma-separated: icu, admit" {disabled} />
      </Field>
      <Field label="Redaction" hint="Best-effort; free-text fields may still contain PHI.">
        <Select bind:value={redactionMode} options={redactionOptions} {disabled} />
      </Field>
    </div>
  </Panel>

  <ConfirmModal
    bind:open={bulkDeleteOpen}
    title="Delete selected samples?"
    message={`This will remove ${selectedIds.size} sample(s) from this tab.`}
    confirmText="Delete"
    cancelText="Cancel"
    variant="danger"
    on:confirm={confirmBulkDelete}
    on:cancel={() => (bulkDeleteOpen = false)}
  />
</div>

<style>
  .inbox {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    min-width: 0;
  }

  .inbox.dragging {
    outline: 1px dashed var(--color-border-focus);
    outline-offset: var(--space-1);
    border-radius: var(--radius-sm);
  }

  .action-row {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2);
  }

  .file-input {
    display: none;
  }

  .note {
    margin: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: var(--text-xs);
    color: var(--color-text-tertiary);
  }

  .form-grid {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: var(--space-3);
  }

  .filter-search {
    position: relative;
    width: 240px;
  }

  .filter-search :global(.filter-search-icon) {
    position: absolute;
    left: 8px;
    top: 50%;
    transform: translateY(-50%);
    color: var(--color-text-tertiary);
    pointer-events: none;
  }

  .filter-search :global(.ui-input) {
    padding-left: 26px;
  }

  .count {
    padding: 0 var(--space-2) 0 var(--space-1);
    color: var(--color-text-tertiary);
  }

  .bulk-bar {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-1);
    padding: var(--space-1) var(--space-2) var(--space-1) var(--space-3);
    border-bottom: 1px solid var(--color-border-subtle);
    background: var(--color-bg-surface);
  }

  .bulk-count {
    margin-right: var(--space-2);
    color: var(--color-text-primary);
  }

  .bulk-end {
    display: flex;
    align-items: center;
    gap: var(--space-1);
    margin-left: auto;
  }

  .inbox :global(.samples-table) {
    max-height: 320px;
  }

  /* The row whose metadata is open in the details pane without being loaded. */
  .inbox :global(tr.is-editing > td) {
    background: var(--color-bg-hover);
  }

  .inbox :global(td.row-actions) {
    padding-right: var(--space-1);
  }

  .check {
    width: 14px;
    height: 14px;
    margin: 0;
    vertical-align: middle;
    accent-color: var(--color-primary);
    cursor: pointer;
  }

  .check:disabled {
    cursor: not-allowed;
  }

  .inbox :global(.tag + .tag) {
    margin-left: var(--space-1);
  }

  .details-title {
    min-width: 0;
    margin: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: var(--text-ui);
    font-weight: var(--font-semibold);
    color: var(--color-text-primary);
  }

  .details {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
  }
</style>
