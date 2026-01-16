<script lang="ts">
  import { profileStore, selectedProfile } from '$lib/features/hl7/profile/profileStore';
  import Button from '$lib/ui/Button.svelte';
  import ConfirmModal from '$lib/ui/ConfirmModal.svelte';

  $: terminology = $selectedProfile?.terminology;
  $: mappings = terminology?.mappings || [];

  // Modal states
  let showMappingModal = false;
  let showEntryModal = false;
  let showDeleteMappingConfirm = false;
  let deletingMappingIndex: number | null = null;
  let editingMappingIndex: number | null = null;
  let editingEntryIndex: number | null = null;

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

  function saveMapping() {
    if (!mappingId.trim() || !mappingSourceSystem.trim() || !mappingTargetSystem.trim()) return;

    const newMapping = {
      id: mappingId.trim(),
      sourceSystem: mappingSourceSystem.trim(),
      targetSystem: mappingTargetSystem.trim(),
      entries: editingMappingIndex !== null ? mappings[editingMappingIndex].entries : []
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

<div class="editor">
  <div class="header">
    <div>
      <h4 class="title">Terminology Mappings</h4>
      <p class="desc">
        Define code mappings between local systems and standard terminologies (LOINC, SNOMED, etc.)
      </p>
    </div>
    <Button variant="secondary" on:click={() => openMappingModal()}>+ Add Mapping Table</Button>
  </div>

  {#if mappings.length === 0}
    <div class="empty">
      <p>No terminology mappings configured.</p>
      <p class="empty-hint">
        Add mapping tables to translate local codes to standard terminologies.
      </p>
    </div>
  {:else}
    <div class="mappings-list">
      {#each mappings as mapping, idx (mapping.id)}
        <div class="mapping-card">
          <div class="mapping-header">
            <div class="mapping-info">
              <span class="mapping-id mono">{mapping.id}</span>
              <span class="mapping-source mono">{mapping.sourceSystem}</span>
              <span class="mapping-arrow">-></span>
              <span class="mapping-target mono">{mapping.targetSystem}</span>
            </div>
            <div class="mapping-meta">
              <span class="entry-count">{mapping.entries.length} entries</span>
            </div>
          </div>

          <div class="mapping-actions">
            <Button variant="secondary" on:click={() => openEntryModal(idx)}>+ Add Entry</Button>
            <button class="action-btn" on:click={() => openMappingModal(idx)}>Edit</button>
            <button class="action-btn danger" on:click={() => confirmDeleteMapping(idx)}>Delete</button>
          </div>

          {#if mapping.entries.length > 0}
            <div class="entries-table">
              <div class="entries-header">
                <div class="col-source">Source Code</div>
                <div class="col-target">Target Code</div>
                <div class="col-display">Display</div>
                <div class="col-actions">Actions</div>
              </div>
              {#each mapping.entries.slice(0, 10) as entry, entryIdx (entryIdx)}
                <div class="entries-row">
                  <div class="col-source mono">{entry.sourceCode}</div>
                  <div class="col-target mono">{entry.targetCode}</div>
                  <div class="col-display">{entry.display || '-'}</div>
                  <div class="col-actions">
                    <button class="icon-btn" on:click={() => openEntryModal(idx, entryIdx)}>
                      Edit
                    </button>
                    <button class="icon-btn danger" on:click={() => deleteEntry(idx, entryIdx)}>
                      Del
                    </button>
                  </div>
                </div>
              {/each}
              {#if mapping.entries.length > 10}
                <div class="entries-more">
                  + {mapping.entries.length - 10} more entries
                </div>
              {/if}
            </div>
          {/if}
        </div>
      {/each}
    </div>
  {/if}
</div>

<!-- Mapping Table Modal -->
{#if showMappingModal}
  <div class="modal-overlay" on:click={() => (showMappingModal = false)} role="button" tabindex="-1">
    <div class="modal" on:click|stopPropagation on:keydown={handleMappingKeydown} role="dialog" aria-modal="true">
      <h3 class="modal-title">
        {editingMappingIndex !== null ? 'Edit Mapping Table' : 'Add Mapping Table'}
      </h3>
      <div class="modal-body">
        <label class="label">
          Source System
          <input
            class="input mono"
            type="text"
            bind:value={mappingSourceSystem}
            placeholder="e.g., LOCAL_LAB"
          />
          <span class="hint">Your local code system identifier</span>
        </label>

        <label class="label">
          Target System
          <input
            class="input mono"
            type="text"
            bind:value={mappingTargetSystem}
            placeholder="e.g., http://loinc.org"
          />
          <div class="quick-systems">
            {#each commonSystems as sys (sys.id)}
              <button
                type="button"
                class="system-chip"
                class:active={mappingTargetSystem === sys.uri}
                on:click={() => (mappingTargetSystem = sys.uri)}
              >
                {sys.name}
              </button>
            {/each}
          </div>
        </label>

        <label class="label">
          Mapping ID
          <input
            class="input mono"
            type="text"
            bind:value={mappingId}
            placeholder="e.g., local_lab_to_loinc"
          />
          <span class="hint">Unique identifier for this mapping table (auto-generated from source)</span>
        </label>
      </div>
      <div class="modal-actions">
        <Button variant="secondary" on:click={() => (showMappingModal = false)}>Cancel</Button>
        <Button
          on:click={saveMapping}
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
  <div class="modal-overlay" on:click={() => (showEntryModal = false)} role="button" tabindex="-1">
    <div class="modal" on:click|stopPropagation on:keydown={handleEntryKeydown} role="dialog" aria-modal="true">
      <h3 class="modal-title">
        {editingEntryIndex !== null ? 'Edit Entry' : 'Add Entry'}
      </h3>
      <div class="modal-body">
        <div class="mapping-context">
          <span class="mono">{selectedMapping.sourceSystem}</span>
          <span>-></span>
          <span class="mono">{selectedMapping.targetSystem}</span>
        </div>

        <label class="label">
          Source Code
          <input
            class="input mono"
            type="text"
            bind:value={entrySourceCode}
            placeholder="e.g., GLU"
          />
        </label>

        <label class="label">
          Target Code
          <input
            class="input mono"
            type="text"
            bind:value={entryTargetCode}
            placeholder="e.g., 2345-7"
          />
        </label>

        <label class="label">
          Display Name (optional)
          <input
            class="input"
            type="text"
            bind:value={entryDisplay}
            placeholder="e.g., Glucose [Mass/volume] in Serum"
          />
        </label>
      </div>
      <div class="modal-actions">
        <Button variant="secondary" on:click={() => (showEntryModal = false)}>Cancel</Button>
        <Button
          on:click={saveEntry}
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
  title="Delete Mapping Table?"
  message="Delete this mapping table and all its entries? This cannot be undone."
  confirmText="Delete"
  variant="danger"
  on:confirm={handleDeleteMappingConfirm}
/>

<style>
  .editor {
    display: grid;
    gap: 16px;
  }

  .header {
    display: flex;
    justify-content: space-between;
    align-items: flex-start;
    gap: 16px;
    flex-wrap: wrap;
  }

  .title {
    margin: 0 0 4px;
    font-size: 0.95rem;
    font-weight: 800;
    color: rgba(243, 244, 246, 0.95);
  }

  .desc {
    margin: 0;
    font-size: 0.85rem;
    color: rgba(229, 231, 235, 0.65);
    line-height: 1.4;
    max-width: 480px;
  }

  .empty {
    padding: 24px;
    text-align: center;
    border-radius: 12px;
    border: 1px dashed rgba(255, 255, 255, 0.15);
    color: rgba(229, 231, 235, 0.7);
  }

  .empty p {
    margin: 0;
  }

  .empty-hint {
    margin-top: 8px !important;
    font-size: 0.85rem;
    color: rgba(229, 231, 235, 0.5);
  }

  .mappings-list {
    display: grid;
    gap: 12px;
  }

  .mapping-card {
    padding: 14px;
    border-radius: 12px;
    border: 1px solid rgba(255, 255, 255, 0.08);
    background: rgba(255, 255, 255, 0.02);
  }

  .mapping-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 12px;
    flex-wrap: wrap;
    margin-bottom: 12px;
  }

  .mapping-info {
    display: flex;
    align-items: center;
    gap: 8px;
    flex-wrap: wrap;
  }

  .mapping-id {
    padding: 2px 8px;
    border-radius: 6px;
    background: rgba(255, 255, 255, 0.05);
    border: 1px solid rgba(255, 255, 255, 0.1);
    color: rgba(229, 231, 235, 0.6);
    font-size: 0.8rem;
    margin-right: 8px;
  }

  .mapping-source,
  .mapping-target {
    font-weight: 700;
    color: rgba(59, 130, 246, 0.95);
  }

  .mapping-arrow {
    color: rgba(229, 231, 235, 0.4);
  }

  .mono {
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, 'Liberation Mono',
      'Courier New', monospace;
  }

  .mapping-meta {
    display: flex;
    gap: 8px;
  }

  .entry-count {
    font-size: 0.85rem;
    color: rgba(229, 231, 235, 0.55);
  }

  .mapping-actions {
    display: flex;
    gap: 8px;
    margin-bottom: 12px;
  }

  .action-btn {
    padding: 4px 10px;
    border-radius: 6px;
    border: 1px solid rgba(255, 255, 255, 0.1);
    background: transparent;
    color: rgba(229, 231, 235, 0.7);
    font-size: 0.8rem;
    cursor: pointer;
  }

  .action-btn:hover {
    background: rgba(255, 255, 255, 0.06);
  }

  .action-btn.danger {
    color: rgba(239, 68, 68, 0.8);
  }

  .action-btn.danger:hover {
    background: rgba(239, 68, 68, 0.1);
  }

  .entries-table {
    border-radius: 8px;
    border: 1px solid rgba(255, 255, 255, 0.06);
    overflow: hidden;
  }

  .entries-header {
    display: grid;
    grid-template-columns: 1fr 1fr 1.5fr auto;
    gap: 8px;
    padding: 8px 12px;
    background: rgba(255, 255, 255, 0.03);
    font-size: 0.8rem;
    font-weight: 600;
    color: rgba(229, 231, 235, 0.6);
    text-transform: uppercase;
    letter-spacing: 0.02em;
  }

  .entries-row {
    display: grid;
    grid-template-columns: 1fr 1fr 1.5fr auto;
    gap: 8px;
    padding: 8px 12px;
    border-top: 1px solid rgba(255, 255, 255, 0.04);
    font-size: 0.9rem;
    color: rgba(229, 231, 235, 0.85);
  }

  .entries-row:hover {
    background: rgba(255, 255, 255, 0.02);
  }

  .col-display {
    color: rgba(229, 231, 235, 0.6);
    font-size: 0.85rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .col-actions {
    display: flex;
    gap: 4px;
  }

  .icon-btn {
    padding: 2px 6px;
    border-radius: 4px;
    border: none;
    background: transparent;
    color: rgba(229, 231, 235, 0.5);
    font-size: 0.75rem;
    cursor: pointer;
  }

  .icon-btn:hover {
    color: rgba(229, 231, 235, 0.85);
    background: rgba(255, 255, 255, 0.05);
  }

  .icon-btn.danger:hover {
    color: rgba(239, 68, 68, 0.9);
    background: rgba(239, 68, 68, 0.1);
  }

  .entries-more {
    padding: 8px 12px;
    text-align: center;
    font-size: 0.85rem;
    color: rgba(229, 231, 235, 0.5);
    border-top: 1px solid rgba(255, 255, 255, 0.04);
  }

  .modal-overlay {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.6);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 1000;
  }

  .modal {
    background: #1f2937;
    border: 1px solid rgba(255, 255, 255, 0.1);
    border-radius: 16px;
    padding: 24px;
    min-width: 400px;
    max-width: 520px;
  }

  .modal-title {
    margin: 0 0 16px;
    font-size: 1.1rem;
    font-weight: 800;
    color: #f3f4f6;
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

  .label {
    display: grid;
    gap: 6px;
    color: rgba(229, 231, 235, 0.8);
    font-size: 0.9rem;
  }

  .input {
    width: 100%;
    padding: 10px 12px;
    border-radius: 12px;
    border: 1px solid rgba(255, 255, 255, 0.12);
    background: rgba(255, 255, 255, 0.03);
    color: rgba(229, 231, 235, 0.92);
    outline: none;
    box-sizing: border-box;
  }

  .input:focus {
    border-color: rgba(59, 130, 246, 0.45);
    box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.15);
  }

  .hint {
    font-size: 0.8rem;
    color: rgba(229, 231, 235, 0.55);
  }

  .quick-systems {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
    margin-top: 8px;
  }

  .system-chip {
    padding: 4px 10px;
    border-radius: 6px;
    border: 1px solid rgba(255, 255, 255, 0.1);
    background: rgba(255, 255, 255, 0.03);
    color: rgba(229, 231, 235, 0.75);
    font-size: 0.8rem;
    cursor: pointer;
  }

  .system-chip:hover {
    background: rgba(255, 255, 255, 0.08);
  }

  .system-chip.active {
    background: rgba(59, 130, 246, 0.15);
    border-color: rgba(59, 130, 246, 0.3);
    color: rgba(59, 130, 246, 0.95);
  }

  .mapping-context {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 10px 12px;
    border-radius: 8px;
    background: rgba(255, 255, 255, 0.03);
    font-size: 0.85rem;
    color: rgba(229, 231, 235, 0.7);
  }
</style>
