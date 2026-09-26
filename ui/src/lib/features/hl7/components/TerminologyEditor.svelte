<script lang="ts">
  import ArrowRight from '@lucide/svelte/icons/arrow-right';
  import Pencil from '@lucide/svelte/icons/pencil';
  import Plus from '@lucide/svelte/icons/plus';
  import Trash2 from '@lucide/svelte/icons/trash-2';
  import {
    Badge,
    Button,
    EmptyState,
    Field,
    Icon,
    IconButton,
    Input,
    Panel,
    Table,
    Td,
    Th,
    Tr
  } from '$lib/ui/primitives';
  import ConfirmModal from '$lib/ui/ConfirmModal.svelte';
  import { profileStore, selectedProfile } from '$lib/features/hl7/profile/profileStore';
  import { afterUpdate, tick } from 'svelte';
  import { createDialogFocusController } from '$lib/domain/a11yDialog';

  $: terminology = $selectedProfile?.terminology;
  $: mappings = terminology?.mappings || [];

  // Modal states
  let showMappingModal = false;
  let showEntryModal = false;
  let showDeleteMappingConfirm = false;
  let deletingMappingIndex: number | null = null;
  let editingMappingIndex: number | null = null;
  let editingEntryIndex: number | null = null;

  let mappingModalEl: HTMLDivElement | null = null;
  let entryModalEl: HTMLDivElement | null = null;
  let wasMappingModalOpen = false;
  let wasEntryModalOpen = false;
  let mappingFocusCtl: ReturnType<typeof createDialogFocusController> | null = null;
  let entryFocusCtl: ReturnType<typeof createDialogFocusController> | null = null;

  // Mapping form
  let mappingId = '';
  let mappingSourceSystem = '';
  let mappingTargetSystem = '';

  // Entry form
  let entrySourceCode = '';
  let entryTargetCode = '';
  let entryDisplay = '';

  // Common systems for quick selection
  const commonSystems = [
    { id: 'loinc', name: 'LOINC', uri: 'http://loinc.org' },
    { id: 'snomed', name: 'SNOMED CT', uri: 'http://snomed.info/sct' },
    { id: 'icd10', name: 'ICD-10-CM', uri: 'http://hl7.org/fhir/sid/icd-10-cm' },
    { id: 'rxnorm', name: 'RxNorm', uri: 'http://www.nlm.nih.gov/research/umls/rxnorm' },
    { id: 'cpt', name: 'CPT', uri: 'http://www.ama-assn.org/go/cpt' }
  ];

  function openMappingModal(index?: number) {
    if (index !== undefined && mappings[index]) {
      const m = mappings[index];
      editingMappingIndex = index;
      mappingId = m.id;
      mappingSourceSystem = m.sourceSystem;
      mappingTargetSystem = m.targetSystem;
    } else {
      editingMappingIndex = null;
      mappingId = '';
      mappingSourceSystem = '';
      mappingTargetSystem = '';
    }
    showMappingModal = true;
  }

  // Auto-generate ID from source system when empty
  $: if (mappingSourceSystem && !mappingId && editingMappingIndex === null) {
    mappingId = mappingSourceSystem.toLowerCase().replace(/[^a-z0-9]+/g, '_').replace(/^_|_$/g, '');
  }

  // Keyboard handler for mapping modal
  function handleMappingKeydown(e: KeyboardEvent) {
    if (e.key === 'Enter' && mappingId.trim() && mappingSourceSystem.trim() && mappingTargetSystem.trim()) {
      saveMapping();
    }
  }

  // Keyboard handler for entry modal
  function handleEntryKeydown(e: KeyboardEvent) {
    if (e.key === 'Enter' && entrySourceCode.trim() && entryTargetCode.trim()) {
      saveEntry();
    }
  }

  afterUpdate(() => {
    if (showMappingModal && !wasMappingModalOpen) {
      tick().then(() => {
        if (!mappingModalEl) return;
        mappingFocusCtl = createDialogFocusController(mappingModalEl);
        mappingFocusCtl.focusInitial();
      });
    }
    if (!showMappingModal && wasMappingModalOpen) {
      mappingFocusCtl?.restoreFocus();
      mappingFocusCtl = null;
    }
    wasMappingModalOpen = showMappingModal;

    if (showEntryModal && !wasEntryModalOpen) {
      tick().then(() => {
        if (!entryModalEl) return;
        entryFocusCtl = createDialogFocusController(entryModalEl);
        entryFocusCtl.focusInitial();
      });
    }
    if (!showEntryModal && wasEntryModalOpen) {
      entryFocusCtl?.restoreFocus();
      entryFocusCtl = null;
    }
    wasEntryModalOpen = showEntryModal;
  });

  function handleWindowKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      if (showEntryModal) showEntryModal = false;
      else if (showMappingModal) showMappingModal = false;
      return;
    }
    if (e.key === 'Tab') {
      if (showEntryModal) entryFocusCtl?.onKeydown(e);
      else if (showMappingModal) mappingFocusCtl?.onKeydown(e);
    }
  }

  function saveMapping() {
    if (!mappingId.trim() || !mappingSourceSystem.trim() || !mappingTargetSystem.trim()) return;

    const existingEntries = editingMappingIndex !== null ? mappings[editingMappingIndex]?.entries ?? [] : [];
    const newMapping = {
      id: mappingId.trim(),
      sourceSystem: mappingSourceSystem.trim(),
      targetSystem: mappingTargetSystem.trim(),
      entries: existingEntries
    };

    let newMappings: typeof mappings;
    if (editingMappingIndex !== null) {
      newMappings = mappings.map((m, i) => (i === editingMappingIndex ? newMapping : m));
    } else {
      newMappings = [...mappings, newMapping];
    }

    profileStore.updateLocal({
      terminology: {
        mappings: newMappings.map((m) => ({
          id: m.id,
          sourceSystem: m.sourceSystem,
          targetSystem: m.targetSystem,
          entries: m.entries.map((e) => ({
            sourceCode: e.sourceCode,
            targetCode: e.targetCode,
            display: e.display
          }))
        }))
      }
    });

    showMappingModal = false;
  }

  function confirmDeleteMapping(index: number) {
    deletingMappingIndex = index;
    showDeleteMappingConfirm = true;
  }

  function handleDeleteMappingConfirm() {
    if (deletingMappingIndex === null) return;

    const newMappings = mappings.filter((_, i) => i !== deletingMappingIndex);
    profileStore.updateLocal({
      terminology: {
        mappings: newMappings.map((m) => ({
          id: m.id,
          sourceSystem: m.sourceSystem,
          targetSystem: m.targetSystem,
          entries: m.entries.map((e) => ({
            sourceCode: e.sourceCode,
            targetCode: e.targetCode,
            display: e.display
          }))
        }))
      }
    });
    deletingMappingIndex = null;
  }

  // Selected mapping for editing entries
  let selectedMappingIndex: number | null = null;
  $: selectedMapping = selectedMappingIndex !== null ? mappings[selectedMappingIndex] : null;

  function openEntryModal(mappingIdx: number, entryIdx?: number) {
    selectedMappingIndex = mappingIdx;
    const mapping = mappings[mappingIdx];
    if (!mapping) return;

    if (entryIdx !== undefined && mapping.entries[entryIdx]) {
      const e = mapping.entries[entryIdx];
      editingEntryIndex = entryIdx;
      entrySourceCode = e.sourceCode;
      entryTargetCode = e.targetCode;
      entryDisplay = e.display || '';
    } else {
      editingEntryIndex = null;
      entrySourceCode = '';
      entryTargetCode = '';
      entryDisplay = '';
    }
    showEntryModal = true;
  }

  function saveEntry() {
    if (selectedMappingIndex === null) return;
    if (!entrySourceCode.trim() || !entryTargetCode.trim()) return;

    const mapping = mappings[selectedMappingIndex];
    if (!mapping) return;

    const newEntry = {
      sourceCode: entrySourceCode.trim(),
      targetCode: entryTargetCode.trim(),
      display: entryDisplay.trim() || null
    };

    let newEntries: typeof mapping.entries;
    if (editingEntryIndex !== null) {
      newEntries = mapping.entries.map((e, i) => (i === editingEntryIndex ? newEntry : e));
    } else {
      newEntries = [...mapping.entries, newEntry];
    }

    const newMappings = mappings.map((m, i) =>
      i === selectedMappingIndex ? { ...m, entries: newEntries } : m
    );

    profileStore.updateLocal({
      terminology: {
        mappings: newMappings.map((m) => ({
          id: m.id,
          sourceSystem: m.sourceSystem,
          targetSystem: m.targetSystem,
          entries: m.entries.map((e) => ({
            sourceCode: e.sourceCode,
            targetCode: e.targetCode,
            display: e.display
          }))
        }))
      }
    });

    showEntryModal = false;
  }

  function deleteEntry(mappingIdx: number, entryIdx: number) {
    const mapping = mappings[mappingIdx];
    if (!mapping) return;

    const newEntries = mapping.entries.filter((_, i) => i !== entryIdx);
    const newMappings = mappings.map((m, i) =>
      i === mappingIdx ? { ...m, entries: newEntries } : m
    );

    profileStore.updateLocal({
      terminology: {
        mappings: newMappings.map((m) => ({
          id: m.id,
          sourceSystem: m.sourceSystem,
          targetSystem: m.targetSystem,
          entries: m.entries.map((e) => ({
            sourceCode: e.sourceCode,
            targetCode: e.targetCode,
            display: e.display
          }))
        }))
      }
    });
  }
</script>

<svelte:window on:keydown={handleWindowKeydown} />

<div class="editor">
  <div class="bar">
    <h3 class="bar-title">Mapping tables</h3>
    <Badge mono>{mappings.length}</Badge>
    <span class="bar-actions">
      <Button icon={Plus} onclick={() => openMappingModal()}>Add mapping table</Button>
    </span>
  </div>

  {#if mappings.length === 0}
    <EmptyState
      align="start"
      message="No terminology mappings. Add a mapping table to translate local codes to a standard terminology such as LOINC or SNOMED CT."
    />
  {:else}
    {#each mappings as mapping, idx (mapping.id)}
      <Panel flush>
        {#snippet header()}
          <h4 class="mapping-head">
            <span class="mapping-id text-mono" title={mapping.id}>{mapping.id}</span>
            <span class="mapping-systems text-mono" title="{mapping.sourceSystem} to {mapping.targetSystem}">
              <span class="mapping-system">{mapping.sourceSystem}</span>
              <Icon icon={ArrowRight} size={12} class="mapping-arrow" />
              <span class="mapping-system">{mapping.targetSystem}</span>
            </span>
          </h4>
        {/snippet}
        {#snippet actions()}
          <Badge mono>{mapping.entries.length} {mapping.entries.length === 1 ? 'entry' : 'entries'}</Badge>
          <Button variant="ghost" icon={Plus} onclick={() => openEntryModal(idx)}>Add entry</Button>
          <IconButton icon={Pencil} label="Edit mapping table" onclick={() => openMappingModal(idx)} />
          <IconButton
            icon={Trash2}
            label="Delete mapping table"
            onclick={() => confirmDeleteMapping(idx)}
          />
        {/snippet}

        {#if mapping.entries.length > 0}
          <Table label="Entries in {mapping.id}" layout="fixed">
            {#snippet head()}
              <tr>
                <Th width="22%">Source code</Th>
                <Th width="22%">Target code</Th>
                <Th>Display</Th>
                <Th width="72px"><span class="sr-only">Actions</span></Th>
              </tr>
            {/snippet}
            {#each mapping.entries.slice(0, 10) as entry, entryIdx (entryIdx)}
              <Tr>
                <Td mono truncate value={entry.sourceCode} />
                <Td mono truncate value={entry.targetCode} />
                <Td truncate muted value={entry.display || '—'} />
                <Td class="row-actions">
                  <IconButton
                    icon={Pencil}
                    label="Edit entry"
                    onclick={() => openEntryModal(idx, entryIdx)}
                  />
                  <IconButton
                    icon={Trash2}
                    label="Delete entry"
                    onclick={() => deleteEntry(idx, entryIdx)}
                  />
                </Td>
              </Tr>
            {/each}
          </Table>
          {#if mapping.entries.length > 10}
            <p class="more text-mono">+{mapping.entries.length - 10} more entries</p>
          {/if}
        {:else}
          <p class="more">No entries yet.</p>
        {/if}
      </Panel>
    {/each}
  {/if}
</div>

<!-- Mapping Table Modal -->
{#if showMappingModal}
  <div class="modal-overlay">
    <button
      type="button"
      class="modal-backdrop"
      tabindex="-1"
      aria-label="Close dialog"
      on:click={() => (showMappingModal = false)}
    ></button>
    <div
      class="modal"
      bind:this={mappingModalEl}
      on:keydown={handleMappingKeydown}
      role="dialog"
      aria-modal="true"
      aria-labelledby="mapping-table-modal-title"
      tabindex="-1"
    >
      <h3 id="mapping-table-modal-title" class="modal-title">
        {editingMappingIndex !== null ? 'Edit mapping table' : 'Add mapping table'}
      </h3>
      <div class="modal-body">
        <Field label="Source system" hint="Your local code system identifier.">
          <Input mono bind:value={mappingSourceSystem} placeholder="e.g. LOCAL_LAB" />
        </Field>

        <Field label="Target system">
          <Input mono bind:value={mappingTargetSystem} placeholder="e.g. http://loinc.org" />
        </Field>
        <div class="quick-systems" role="group" aria-label="Common target systems">
          {#each commonSystems as sys (sys.id)}
            <button
              type="button"
              class="system-option"
              aria-pressed={mappingTargetSystem === sys.uri}
              on:click={() => (mappingTargetSystem = sys.uri)}
            >
              {sys.name}
            </button>
          {/each}
        </div>

        <Field
          label="Mapping id"
          hint="Unique id for this mapping table; generated from the source system."
        >
          <Input mono bind:value={mappingId} placeholder="e.g. local_lab_to_loinc" />
        </Field>
      </div>
      <div class="modal-actions">
        <Button size="md" onclick={() => (showMappingModal = false)}>Cancel</Button>
        <Button
          variant="primary"
          size="md"
          onclick={saveMapping}
          disabled={!mappingId.trim() || !mappingSourceSystem.trim() || !mappingTargetSystem.trim()}
        >
          {editingMappingIndex !== null ? 'Update' : 'Create'}
        </Button>
      </div>
    </div>
  </div>
{/if}

<!-- Entry Modal -->
{#if showEntryModal && selectedMapping}
  <div class="modal-overlay">
    <button
      type="button"
      class="modal-backdrop"
      tabindex="-1"
      aria-label="Close dialog"
      on:click={() => (showEntryModal = false)}
    ></button>
    <div
      class="modal"
      bind:this={entryModalEl}
      on:keydown={handleEntryKeydown}
      role="dialog"
      aria-modal="true"
      aria-labelledby="mapping-entry-modal-title"
      tabindex="-1"
    >
      <h3 id="mapping-entry-modal-title" class="modal-title">
        {editingEntryIndex !== null ? 'Edit entry' : 'Add entry'}
      </h3>
      <div class="modal-body">
        <p class="mapping-context text-mono">
          <span class="mapping-system">{selectedMapping.sourceSystem}</span>
          <Icon icon={ArrowRight} size={12} class="mapping-arrow" />
          <span class="mapping-system">{selectedMapping.targetSystem}</span>
        </p>

        <div class="form-grid">
          <Field label="Source code">
            <Input mono bind:value={entrySourceCode} placeholder="e.g. GLU" />
          </Field>
          <Field label="Target code">
            <Input mono bind:value={entryTargetCode} placeholder="e.g. 2345-7" />
          </Field>
        </div>

        <Field label="Display name (optional)">
          <Input bind:value={entryDisplay} placeholder="e.g. Glucose [Mass/volume] in Serum" />
        </Field>
      </div>
      <div class="modal-actions">
        <Button size="md" onclick={() => (showEntryModal = false)}>Cancel</Button>
        <Button
          variant="primary"
          size="md"
          onclick={saveEntry}
          disabled={!entrySourceCode.trim() || !entryTargetCode.trim()}
        >
          {editingEntryIndex !== null ? 'Update' : 'Add'}
        </Button>
      </div>
    </div>
  </div>
{/if}

<!-- Delete Mapping Confirmation Modal -->
<ConfirmModal
  bind:open={showDeleteMappingConfirm}
  title="Delete mapping table?"
  message="Delete this mapping table and all its entries? This cannot be undone."
  confirmText="Delete"
  variant="danger"
  on:confirm={handleDeleteMappingConfirm}
/>

<style>
  .editor {
    display: grid;
    gap: var(--space-3);
  }

  .editor :global(.row-actions) {
    text-align: right;
  }

  .bar {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }

  .bar-title {
    margin: 0;
    font-size: var(--text-label);
    font-weight: var(--font-semibold);
    letter-spacing: var(--tracking-label);
    text-transform: uppercase;
    color: var(--color-text-tertiary);
  }

  .bar-actions {
    margin-left: auto;
  }

  .mapping-head {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    min-width: 0;
    margin: 0;
    font-weight: var(--font-normal);
  }

  .mapping-id {
    flex: 0 1 auto;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: var(--color-text-primary);
    font-weight: var(--font-medium);
  }

  .mapping-systems,
  .mapping-context {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
    min-width: 0;
    color: var(--color-text-tertiary);
  }

  .mapping-system {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .editor :global(.mapping-arrow),
  .mapping-context :global(.mapping-arrow) {
    color: var(--color-text-muted);
  }

  .more {
    margin: 0;
    padding: var(--space-2) var(--space-3);
    font-size: var(--text-xs);
    color: var(--color-text-tertiary);
  }

  .mapping-context {
    margin: 0;
    padding: var(--space-2);
    border: 1px solid var(--color-border-subtle);
    border-radius: var(--radius-sm);
    background: var(--color-bg-surface);
  }

  .form-grid {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: var(--space-3);
  }

  .quick-systems {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-1);
    margin-top: calc(-1 * var(--space-2));
  }

  .system-option {
    height: 22px;
    padding: 0 6px;
    border: 1px solid var(--color-border-default);
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--color-text-secondary);
    font-size: var(--text-xs);
    cursor: pointer;
    transition: var(--transition-colors);
  }

  .system-option:hover {
    background: var(--color-bg-hover);
    color: var(--color-text-primary);
  }

  .system-option[aria-pressed='true'] {
    border-color: var(--color-border-strong);
    background: var(--color-bg-active);
    color: var(--color-text-primary);
  }

  .system-option:focus-visible {
    outline: 2px solid var(--color-focus-ring);
    outline-offset: 1px;
  }

  .modal-overlay {
    position: fixed;
    inset: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: var(--space-4);
    z-index: var(--z-modal);
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
    width: 100%;
    max-width: var(--modal-width-md);
    background: var(--color-bg-overlay);
    border: 1px solid var(--color-border-default);
    border-radius: var(--modal-radius);
    box-shadow: var(--shadow-xl);
    outline: none;
  }

  .modal-title {
    margin: 0;
    padding: var(--space-4) var(--space-4) 0;
    font-size: var(--text-title);
    font-weight: var(--font-semibold);
    color: var(--color-text-primary);
  }

  .modal-body {
    display: grid;
    gap: var(--space-3);
    padding: var(--space-3) var(--space-4) var(--space-4);
  }

  .modal-actions {
    display: flex;
    gap: var(--space-2);
    justify-content: flex-end;
    padding: var(--space-3) var(--space-4);
    border-top: 1px solid var(--color-border-subtle);
  }
</style>
