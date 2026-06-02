<script lang="ts">
  import Panel from '$lib/ui/Panel.svelte';
  import Button from '$lib/ui/Button.svelte';
  import ConfirmModal from '$lib/ui/ConfirmModal.svelte';
  import Badge from '$lib/ui/Badge.svelte';
  import type { HL7Sample } from '$lib/features/hl7/samples/types';
  import type { HL7RedactionMode } from '$lib/domain/hl7Redact';
  import { afterUpdate, createEventDispatcher, tick } from 'svelte';
  import { createDialogFocusController } from '$lib/domain/a11yDialog';
  import { SvelteSet } from 'svelte/reactivity';

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

  let name = '';
  let feed = '';
  let tags = '';
  let sourceOverride = '';
  let filter = '';
  let redactionMode: HL7RedactionMode = 'none';
  let fileInputEl: HTMLInputElement | null = null;
  let isDragging = false;

  let showEditModal = false;
  let editSampleId: string | null = null;
  let editName = '';
  let editSource = '';
  let editFeed = '';
  let editTags = '';
  let editModalEl: HTMLDivElement | null = null;
  let wasEditModalOpen = false;
  let editFocusCtl: ReturnType<typeof createDialogFocusController> | null = null;

  let selectionMode = false;
  let selectedIds = new SvelteSet<string>();
  let bulkDeleteOpen = false;

  function toggleSelectionMode(): void {
    selectionMode = !selectionMode;
    if (!selectionMode) selectedIds.clear();
  }

  function toggleSelected(id: string): void {
    if (selectedIds.has(id)) selectedIds.delete(id);
    else selectedIds.add(id);
  }

  function clearSelected(): void {
    selectedIds.clear();
  }

  function selectAllFiltered(): void {
    selectedIds.clear();
    for (const s of filtered) selectedIds.add(s.id);
  }

  function requestBulkDelete(): void {
    if (selectedIds.size === 0) return;
    bulkDeleteOpen = true;
  }

  function confirmBulkDelete(): void {
    const ids = Array.from(selectedIds);
    if (ids.length === 0) return;
    dispatch('bulkRemove', { ids });
    selectedIds.clear();
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

  function openEdit(sample: HL7Sample): void {
    editSampleId = sample.id;
    editName = sample.name;
    editSource = sample.source;
    editFeed = sample.feed ?? '';
    editTags = (sample.tags ?? []).join(', ');
    showEditModal = true;
  }

  function closeEdit(): void {
    showEditModal = false;
    editSampleId = null;
  }

  function saveEdit(): void {
    if (!editSampleId) return;
    const name = editName.trim();
    const source = editSource.trim();
    if (!name || !source) return;
    const feed = editFeed.trim();
    const tags = parseTags(editTags);
    dispatch('updateMeta', {
      id: editSampleId,
      name,
      source,
      feed,
      tags
    });
    closeEdit();
  }

  afterUpdate(() => {
    if (showEditModal && !wasEditModalOpen) {
      tick().then(() => {
        if (!editModalEl) return;
        editFocusCtl = createDialogFocusController(editModalEl);
        editFocusCtl.focusInitial();
      });
    }
    if (!showEditModal && wasEditModalOpen) {
      editFocusCtl?.restoreFocus();
      editFocusCtl = null;
    }
    wasEditModalOpen = showEditModal;
  });

  function handleWindowKeydown(e: KeyboardEvent) {
    if (!showEditModal) return;
    if (e.key === 'Escape') {
      closeEdit();
      return;
    }
    if (e.key === 'Tab') {
      editFocusCtl?.onKeydown(e);
    }
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

  $: if (!selectionMode && selectedIds.size) selectedIds.clear();

  $: {
    const valid = new Set(samples.map((s) => s.id));
    for (const id of selectedIds) {
      if (!valid.has(id)) selectedIds.delete(id);
    }
  }
</script>

<svelte:window on:keydown={handleWindowKeydown} />

<Panel title="Samples (local)">
  <div
    class="dropzone"
    class:dragging={isDragging}
    on:dragover={onDragOver}
    on:dragleave={onDragLeave}
    on:drop={onDrop}
    role="region"
    aria-label="Samples inbox. Drag and drop HL7 files to import."
  >
  <p class="note">
    Stored in <span class="mono">localStorage</span>. Don’t paste PHI unless you’re on an approved machine/profile.
  </p>

  <div class="controls">
    <label class="label">
      Feed (optional)
      <input class="input" type="text" bind:value={feed} placeholder="e.g., epic_adt_icu" disabled={disabled} />
    </label>
    <label class="label">
      Tags (comma-separated)
      <input class="input" type="text" bind:value={tags} placeholder="e.g., icu, admit, demo" disabled={disabled} />
    </label>
    <label class="label">
      Source override (optional)
      <input class="input" type="text" bind:value={sourceOverride} placeholder="defaults to current source / file name" disabled={disabled} />
    </label>
    <label class="label">
      Redaction
      <select class="select" bind:value={redactionMode} disabled={disabled}>
        <option value="none">None</option>
        <option value="mask_basic">Mask basic (PID/NK1/PV1)</option>
        <option value="segment_sanitize">Sanitize segments (PID/NK1/IN*)</option>
      </select>
      <span class="hint">Best-effort; free-text fields may still contain PHI.</span>
    </label>
  </div>

  <div class="save">
    <label class="label">
      Sample name (optional)
      <input class="input" type="text" bind:value={name} placeholder="ADT A01 - ICU admit" disabled={disabled} />
    </label>
    <div class="save-actions">
      <Button on:click={save} disabled={disabled || !currentRaw.trim()}>Save current</Button>
      <input
        class="file-input"
        type="file"
        multiple
        accept=".hl7,.txt,.msg,.dat,text/plain"
        bind:this={fileInputEl}
        on:change={onFileChange}
        disabled={disabled}
      />
      <Button variant="secondary" on:click={triggerImport} disabled={disabled}>
        Import files
      </Button>
      <Button variant="secondary" on:click={() => dispatch('loadExamples', {})} disabled={disabled}>
        Load Examples
      </Button>
      <Button variant="secondary" on:click={() => dispatch('clear', {})} disabled={disabled || samples.length === 0}>
        Clear
      </Button>
    </div>
  </div>

  {#if samples.length === 0}
    <div class="empty-state">
      <p class="empty">No saved samples yet.</p>
      <Button variant="secondary" on:click={() => dispatch('loadExamples', {})} disabled={disabled}>
        Load Example Messages
      </Button>
    </div>
  {:else}
	    <div class="filter">
	      <input
	        class="input"
	        type="text"
	        bind:value={filter}
	        placeholder="Filter by name, source, feed, message type, tag…"
	        disabled={disabled}
	      />
	      {#if filter.trim()}
	        <Button variant="secondary" on:click={() => (filter = '')} disabled={disabled}>Clear</Button>
	      {/if}
	      <span class="count mono">{filtered.length}/{samples.length}</span>
	      <Button variant="secondary" on:click={toggleSelectionMode} disabled={disabled}>
	        {selectionMode ? 'Done' : 'Select'}
	      </Button>
	    </div>

		    {#if selectionMode}
		      <div class="bulk-bar" role="toolbar" aria-label="Bulk actions">
		        <div class="bulk-left">
		          <span class="mono">{selectedIds.size} selected</span>
		        </div>
		        <div class="bulk-actions">
	          <Button variant="secondary" size="sm" on:click={selectAllFiltered} disabled={disabled || filtered.length === 0}>
	            Select all
	          </Button>
	          <Button variant="secondary" size="sm" on:click={clearSelected} disabled={disabled || selectedIds.size === 0}>
	            Clear selected
	          </Button>
	          <Button variant="danger" size="sm" on:click={requestBulkDelete} disabled={disabled || selectedIds.size === 0}>
	            Delete selected
	          </Button>
		        </div>
		      </div>

		      <div class="bulk-bar" role="toolbar" aria-label="Bulk apply metadata">
		        <div class="bulk-left">
		          <span class="muted">Apply to selected</span>
		        </div>
		        <div class="bulk-actions">
	          <Button
	            variant="secondary"
	            size="sm"
	            on:click={applyTagsToSelected}
	            disabled={disabled || selectedIds.size === 0 || parseTags(tags).length === 0}
	            title="Uses Tags field above"
	          >
	            Apply tags
	          </Button>
	          <Button
	            variant="secondary"
	            size="sm"
	            on:click={clearTagsOnSelected}
	            disabled={disabled || selectedIds.size === 0}
	          >
	            Clear tags
	          </Button>
	          <Button
	            variant="secondary"
	            size="sm"
	            on:click={() => applyRedactionToSelected(redactionMode)}
	            disabled={disabled || selectedIds.size === 0}
	            title="Uses Redaction selector above"
	          >
	            Set redaction
	          </Button>
	          <Button
	            variant="secondary"
	            size="sm"
	            on:click={() => applyRedactionToSelected('none')}
	            disabled={disabled || selectedIds.size === 0}
	          >
	            Clear redaction
	          </Button>
	        </div>
	      </div>
	    {/if}

	    <ul class="list" class:selection={selectionMode}>
	      {#each filtered as s (s.id)}
	        <li class="li">
	          {#if selectionMode}
	            <label class="check">
	              <input
	                type="checkbox"
	                checked={selectedIds.has(s.id)}
	                on:change={() => toggleSelected(s.id)}
	                disabled={disabled}
	              />
	              <span class="sr-only">Select {s.name}</span>
	            </label>
	          {/if}
	          <button
	            type="button"
	            class="item"
	            class:active={activeId === s.id}
	            on:click={() => (selectionMode ? toggleSelected(s.id) : dispatch('select', { id: s.id }))}
	            disabled={disabled}
	          >
	            <div class="top">
	              <div class="title">{s.name}</div>
              <div class="meta">
                {#if s.messageType}<Badge mono>{s.messageType}</Badge>{/if}
                {#if s.version}<Badge mono>{s.version}</Badge>{/if}
                {#if s.feed}<Badge mono>feed:{s.feed}</Badge>{/if}
                {#if s.tags?.length}
                  {#each s.tags as t (t)}
                    <Badge variant="info">{t}</Badge>
                  {/each}
                {/if}
                {#if s.redactionMode && s.redactionMode !== 'none'}
                  <Badge variant="warning">redacted</Badge>
                {/if}
              </div>
            </div>
            <div class="sub">
              <span class="mono">{s.source}</span>
              <span class="dot">•</span>
              <span class="mono">{new Date(s.createdAt).toLocaleString()}</span>
              {#if s.controlId}
                <span class="dot">•</span>
                <span class="mono">MSH-10={s.controlId}</span>
              {/if}
            </div>
	          </button>
	          <div class="item-actions">
	            {#if selectionMode}
	              <Button variant="secondary" on:click={() => dispatch('select', { id: s.id })} disabled={disabled}>
	                Open
	              </Button>
	            {/if}
	            <Button variant="secondary" on:click={() => openEdit(s)} disabled={disabled}>Edit</Button>
	            <button
	              type="button"
	              class="trash"
	              title="Remove"
              on:click={() => dispatch('remove', { id: s.id })}
              disabled={disabled}
            >
              Remove
            </button>
          </div>
        </li>
      {/each}
	    </ul>
	  {/if}

	  <ConfirmModal
	    bind:open={bulkDeleteOpen}
	    title="Delete selected samples?"
	    message={`This will remove ${selectedIds.size} sample(s) from localStorage.`}
	    confirmText="Delete"
	    cancelText="Cancel"
	    variant="danger"
	    on:confirm={confirmBulkDelete}
	    on:cancel={() => (bulkDeleteOpen = false)}
	  />

	  {#if showEditModal}
	    <div class="modal-overlay">
	      <button
	        type="button"
        class="modal-backdrop"
        tabindex="-1"
        aria-label="Close dialog"
        on:click={closeEdit}
      ></button>
      <div
        class="modal"
        bind:this={editModalEl}
        role="dialog"
        aria-modal="true"
        aria-labelledby="edit-sample-modal-title"
        tabindex="-1"
      >
        <h3 id="edit-sample-modal-title" class="modal-title">Edit Sample</h3>
        <div class="modal-body">
          <label class="label">
            Name
            <input class="input" type="text" bind:value={editName} disabled={disabled} />
          </label>
          <label class="label">
            Source
            <input class="input" type="text" bind:value={editSource} disabled={disabled} />
          </label>
          <label class="label">
            Feed (optional)
            <input class="input" type="text" bind:value={editFeed} disabled={disabled} />
          </label>
          <label class="label">
            Tags (comma-separated)
            <input class="input" type="text" bind:value={editTags} disabled={disabled} />
          </label>
        </div>
        <div class="modal-actions">
          <Button variant="secondary" on:click={closeEdit}>Cancel</Button>
          <Button on:click={saveEdit} disabled={!editName.trim() || !editSource.trim()}>
            Save
          </Button>
        </div>
      </div>
    </div>
  {/if}
  </div>
</Panel>

<style>
  .mono {
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, 'Liberation Mono', 'Courier New', monospace;
  }

  .file-input {
    display: none;
  }

  .controls {
    display: grid;
    gap: 10px;
    grid-template-columns: 1fr;
    margin-bottom: 14px;
  }

  @media (min-width: 980px) {
    .controls {
      grid-template-columns: 1fr 1fr;
    }
  }

	  .hint {
	    font-size: 0.8rem;
	    color: var(--color-text-muted);
	  }

	  .select {
	    padding: 10px 12px;
	    border-radius: var(--radius-xl);
	    border: 1px solid var(--color-border-default);
	    background: var(--color-bg-input);
	    color: var(--color-text-primary);
	    outline: none;
	  }

	  .select:focus {
	    border-color: var(--color-border-focus);
	    box-shadow: var(--shadow-focus);
	  }

  .dropzone.dragging {
    outline: 2px dashed rgba(59, 130, 246, 0.7);
    outline-offset: 8px;
    border-radius: 12px;
  }

	  .note {
	    margin: 0 0 12px;
	    color: var(--color-text-secondary);
	    line-height: 1.45;
	  }

  .save {
    display: grid;
    gap: 10px;
    margin-bottom: 14px;
  }

	  .label {
	    display: grid;
	    gap: 6px;
	    color: var(--color-text-secondary);
	    font-size: 0.9rem;
	  }

	  .input {
	    padding: 10px 12px;
	    border-radius: var(--radius-xl);
	    border: 1px solid var(--color-border-default);
	    background: var(--color-bg-input);
	    color: var(--color-text-primary);
	    outline: none;
	  }

	  .input:focus {
	    border-color: var(--color-border-focus);
	    box-shadow: var(--shadow-focus);
	  }

  .save-actions {
    display: flex;
    gap: 10px;
    flex-wrap: wrap;
  }

	  .empty-state {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 12px;
    padding: 20px;
    border-radius: 12px;
	    border: 1px dashed var(--color-border-default);
	    background: var(--color-bg-elevated);
	  }

	  .empty {
	    color: var(--color-text-tertiary);
	    margin: 0;
	  }

  .filter {
    display: flex;
    align-items: center;
    gap: 10px;
    flex-wrap: wrap;
    margin-bottom: 12px;
  }

	  .bulk-bar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 10px;
    flex-wrap: wrap;
    padding: 10px 12px;
    margin-bottom: 12px;
    border-radius: 12px;
	    border: 1px solid var(--color-border-default);
	    background: var(--color-bg-elevated);
	  }

  .bulk-left {
    display: flex;
    align-items: center;
    gap: 10px;
  }

  .bulk-actions {
    display: flex;
    align-items: center;
    gap: 8px;
    flex-wrap: wrap;
    justify-content: flex-end;
  }

	  .muted {
	    color: var(--color-text-tertiary);
	    font-size: 0.9rem;
	    font-weight: 650;
	  }

	  .count {
	    color: var(--color-text-muted);
	    font-size: 0.85rem;
	    font-weight: 700;
	  }

  .list {
    padding: 0;
    margin: 0;
    display: grid;
    gap: 10px;
  }

  .li {
    list-style: none;
    display: grid;
    grid-template-columns: 1fr auto;
    gap: 10px;
    align-items: start;
  }

  .list.selection .li {
    grid-template-columns: auto 1fr auto;
  }

  .check {
    display: flex;
    align-items: start;
    padding-top: 12px;
  }

  .check input {
    width: 16px;
    height: 16px;
    accent-color: rgba(59, 130, 246, 0.85);
  }

  .item-actions {
    display: flex;
    flex-direction: column;
    gap: 8px;
    align-items: stretch;
  }

	  .item {
    width: 100%;
    text-align: left;
	    border-radius: var(--radius-xl);
	    border: 1px solid var(--color-border-default);
	    background: var(--color-bg-elevated);
	    padding: 12px;
	    cursor: pointer;
	    color: var(--color-text-primary);
	  }

	  .item:hover:enabled {
	    background: var(--color-bg-hover);
	  }

  .item.active {
    border-color: rgba(59, 130, 246, 0.45);
    background: rgba(59, 130, 246, 0.12);
  }

  .top {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 10px;
  }

	  .title {
	    font-weight: 800;
	    color: var(--color-text-primary);
	  }

	  .sub {
	    margin-top: 8px;
	    color: var(--color-text-tertiary);
	    font-size: 0.9rem;
    display: flex;
    align-items: baseline;
    gap: 8px;
    flex-wrap: wrap;
  }

	  .dot {
	    color: var(--color-text-muted);
	  }

  .meta {
    display: flex;
    gap: 8px;
    flex-wrap: wrap;
    justify-content: flex-end;
  }

  .trash {
    padding: 10px 12px;
    border-radius: 10px;
    border: 1px solid rgba(239, 68, 68, 0.35);
    background: rgba(239, 68, 68, 0.08);
    color: rgba(254, 226, 226, 0.9);
    cursor: pointer;
    font-weight: 700;
  }

  .trash:hover:enabled {
    background: rgba(239, 68, 68, 0.14);
  }

  .modal-overlay {
    position: fixed;
    inset: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 1000;
  }

	  .modal-backdrop {
    position: absolute;
    inset: 0;
    border: 0;
    padding: 0;
	    background: var(--modal-backdrop);
	    cursor: default;
	  }
	
	  .modal {
	    position: relative;
	    z-index: 1;
	    background: var(--color-bg-base);
	    border: 1px solid var(--color-border-default);
	    border-radius: var(--modal-radius);
	    padding: 24px;
	    min-width: 360px;
	    max-width: 520px;
	    width: calc(100vw - 32px);
	  }

	  .modal-title {
    margin: 0 0 16px;
    font-size: 1.1rem;
    font-weight: 800;
	    color: var(--color-text-primary);
	  }

  .modal-body {
    display: grid;
    gap: 14px;
    margin-bottom: 20px;
  }

  .modal-actions {
    display: flex;
    gap: 10px;
    justify-content: flex-end;
  }
</style>
