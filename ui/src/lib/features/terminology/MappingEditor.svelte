<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import Button from '$lib/ui/Button.svelte';
  import { toasts } from '$lib/ui/toastStore';
  import { updateMapping } from './terminologyApi';
  import type { MappingEquivalence, ListMappingsQuery } from '$lib/gen/graphql';

  // Use the actual type returned from the query
  type MappingNode = ListMappingsQuery['listMappings']['nodes'][number];

  export let mapping: MappingNode;
  export let open = false;

  const dispatch = createEventDispatcher<{
    close: void;
    save: { mapping: MappingNode };
  }>();

  // Editable fields
  let editEquivalence: MappingEquivalence = mapping.equivalence;
  let editTargetDisplay = mapping.targetDisplay ?? '';
  let editComment = mapping.comment ?? '';
  let editConfidence = mapping.confidence ?? 0;

  let saving = false;

  // Reset form when mapping changes
  $: if (mapping) {
    editEquivalence = mapping.equivalence;
    editTargetDisplay = mapping.targetDisplay ?? '';
    editComment = mapping.comment ?? '';
    editConfidence = mapping.confidence ?? 0;
  }

  const equivalenceOptions: { value: MappingEquivalence; label: string }[] = [
    { value: 'EQUIVALENT', label: 'Equivalent' },
    { value: 'WIDER', label: 'Wider' },
    { value: 'NARROWER', label: 'Narrower' },
    { value: 'INEXACT', label: 'Inexact' }
  ];

  function handleClose() {
    open = false;
    dispatch('close');
  }

  async function handleSave() {
    saving = true;

    try {
      const updated = await updateMapping({
        id: mapping.id,
        sourceDisplay: null, // Not editable in this modal
        equivalence: editEquivalence !== mapping.equivalence ? editEquivalence : null,
        targetDisplay: editTargetDisplay !== (mapping.targetDisplay ?? '') ? editTargetDisplay : null,
        comment: editComment !== (mapping.comment ?? '') ? editComment : null,
        confidence: editConfidence !== (mapping.confidence ?? 0) ? editConfidence : null
      });

      toasts.success('Mapping updated successfully');
      dispatch('save', { mapping: updated });
      handleClose();
    } catch (err) {
      toasts.error(err instanceof Error ? err.message : 'Failed to update mapping');
    } finally {
      saving = false;
    }
  }

  function handleWindowKeydown(e: KeyboardEvent) {
    if (!open) return;
    if (e.key === 'Escape') {
      handleClose();
    }
  }

  function formatOrigin(origin: string): string {
    switch (origin) {
      case 'CSV_UPLOAD': return 'CSV Upload';
      case 'APPROVED_AUTOROUTE': return 'Approved';
      case 'MANUAL': return 'Manual';
      default: return origin;
    }
  }

  function formatDate(dateStr: string): string {
    const date = new Date(dateStr);
    return date.toLocaleDateString('en-US', {
      month: 'short',
      day: 'numeric',
      year: 'numeric',
      hour: '2-digit',
      minute: '2-digit'
    });
  }
</script>

<svelte:window on:keydown={handleWindowKeydown} />

{#if open}
  <div class="modal-overlay">
    <button
      type="button"
      class="modal-backdrop"
      tabindex="-1"
      aria-label="Close dialog"
      on:click={handleClose}
    ></button>
    <div
      class="modal"
      role="dialog"
      aria-modal="true"
      aria-labelledby="modal-title"
    >
      <h3 id="modal-title" class="modal-title">Edit Mapping</h3>

      <div class="modal-body">
        <!-- Read-only source info -->
        <div class="readonly-section">
          <div class="field-group">
            <span class="field-label">Source System</span>
            <div class="field-value readonly">{mapping.sourceSystem}</div>
          </div>
          <div class="field-group">
            <span class="field-label">Source Code</span>
            <div class="field-value readonly mono">{mapping.sourceCode}</div>
          </div>
          {#if mapping.sourceDisplay}
            <div class="field-group">
              <span class="field-label">Source Display</span>
              <div class="field-value readonly">{mapping.sourceDisplay}</div>
            </div>
          {/if}
        </div>

        <div class="divider"></div>

        <!-- Target info (partially editable) -->
        <div class="editable-section">
          <div class="field-group">
            <span class="field-label">Target System</span>
            <div class="field-value readonly">{mapping.targetSystem}</div>
          </div>
          <div class="field-group">
            <span class="field-label">Target Code</span>
            <div class="field-value readonly mono">{mapping.targetCode}</div>
          </div>
          <div class="field-group">
            <label class="field-label" for="target-display">Target Display</label>
            <input
              id="target-display"
              type="text"
              class="field-input"
              bind:value={editTargetDisplay}
              placeholder="Human-readable display name"
            />
          </div>
        </div>

        <div class="divider"></div>

        <!-- Mapping attributes -->
        <div class="attributes-section">
          <div class="field-row">
            <div class="field-group">
              <label class="field-label" for="equivalence">Equivalence</label>
              <select id="equivalence" class="field-select" bind:value={editEquivalence}>
                {#each equivalenceOptions as opt (opt.value)}
                  <option value={opt.value}>{opt.label}</option>
                {/each}
              </select>
            </div>
            <div class="field-group">
              <label class="field-label" for="confidence">Confidence</label>
              <div class="confidence-input">
                <input
                  id="confidence"
                  type="range"
                  min="0"
                  max="1"
                  step="0.01"
                  bind:value={editConfidence}
                />
                <span class="confidence-value">{(editConfidence * 100).toFixed(0)}%</span>
              </div>
            </div>
          </div>

          <div class="field-group full-width">
            <label class="field-label" for="comment">Comment</label>
            <textarea
              id="comment"
              class="field-textarea"
              bind:value={editComment}
              rows="3"
              placeholder="Optional notes about this mapping..."
            ></textarea>
          </div>
        </div>

        <div class="divider"></div>

        <!-- Metadata (read-only) -->
        <div class="metadata-section">
          <div class="metadata-row">
            <span class="meta-label">Origin:</span>
            <span class="meta-value">{formatOrigin(mapping.origin)}</span>
          </div>
          <div class="metadata-row">
            <span class="meta-label">Created:</span>
            <span class="meta-value">{formatDate(mapping.createdAt)}</span>
          </div>
          {#if mapping.createdBy}
            <div class="metadata-row">
              <span class="meta-label">Created By:</span>
              <span class="meta-value">{mapping.createdBy}</span>
            </div>
          {/if}
          {#if mapping.uploadBatchId}
            <div class="metadata-row">
              <span class="meta-label">Batch ID:</span>
              <span class="meta-value mono">{mapping.uploadBatchId}</span>
            </div>
          {/if}
        </div>
      </div>

      <div class="modal-actions">
        <Button variant="secondary" on:click={handleClose} disabled={saving}>
          Cancel
        </Button>
        <Button variant="primary" on:click={handleSave} disabled={saving}>
          {saving ? 'Saving...' : 'Save Changes'}
        </Button>
      </div>
    </div>
  </div>
{/if}

<style>
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
    background: rgba(0, 0, 0, 0.6);
    cursor: default;
  }

  .modal {
    position: relative;
    z-index: 1;
    background: #1f2937;
    border: 1px solid rgba(255, 255, 255, 0.1);
    border-radius: 16px;
    padding: 24px;
    width: 90%;
    max-width: 560px;
    max-height: 90vh;
    overflow-y: auto;
    animation: slideIn 0.15s ease-out;
  }

  @keyframes slideIn {
    from {
      opacity: 0;
      transform: scale(0.95) translateY(-10px);
    }
    to {
      opacity: 1;
      transform: scale(1) translateY(0);
    }
  }

  .modal-title {
    margin: 0 0 20px;
    font-size: 1.1rem;
    font-weight: 800;
    color: #f3f4f6;
  }

  .modal-body {
    margin-bottom: 20px;
  }

  .divider {
    height: 1px;
    background: rgba(255, 255, 255, 0.08);
    margin: 16px 0;
  }

  .field-group {
    margin-bottom: 12px;
  }

  .field-group.full-width {
    grid-column: 1 / -1;
  }

  .field-row {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 16px;
  }

  .field-label {
    display: block;
    font-size: 0.75rem;
    font-weight: 600;
    color: rgba(229, 231, 235, 0.6);
    text-transform: uppercase;
    letter-spacing: 0.02em;
    margin-bottom: 4px;
  }

  .field-value {
    font-size: 0.9rem;
    color: rgba(229, 231, 235, 0.9);
    line-height: 1.5;
  }

  .field-value.readonly {
    padding: 8px 12px;
    background: rgba(255, 255, 255, 0.02);
    border-radius: 6px;
    border: 1px solid rgba(255, 255, 255, 0.05);
  }

  .field-value.mono {
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  }

  .field-input,
  .field-select,
  .field-textarea {
    width: 100%;
    padding: 8px 12px;
    border-radius: 8px;
    border: 1px solid rgba(255, 255, 255, 0.1);
    background: rgba(255, 255, 255, 0.03);
    color: rgba(229, 231, 235, 0.9);
    font-size: 0.9rem;
    font-family: inherit;
    outline: none;
    transition: border-color 0.15s, box-shadow 0.15s;
  }

  .field-input:focus,
  .field-select:focus,
  .field-textarea:focus {
    border-color: rgba(59, 130, 246, 0.5);
    box-shadow: 0 0 0 2px rgba(59, 130, 246, 0.15);
  }

  .field-textarea {
    resize: vertical;
    min-height: 60px;
  }

  .confidence-input {
    display: flex;
    align-items: center;
    gap: 12px;
  }

  .confidence-input input[type="range"] {
    flex: 1;
    accent-color: rgb(59, 130, 246);
  }

  .confidence-value {
    min-width: 45px;
    text-align: right;
    font-family: ui-monospace, monospace;
    font-size: 0.9rem;
    color: rgba(229, 231, 235, 0.8);
  }

  .metadata-section {
    display: flex;
    flex-wrap: wrap;
    gap: 8px 24px;
  }

  .metadata-row {
    display: flex;
    gap: 6px;
    font-size: 0.8rem;
  }

  .meta-label {
    color: rgba(229, 231, 235, 0.5);
  }

  .meta-value {
    color: rgba(229, 231, 235, 0.7);
  }

  .meta-value.mono {
    font-family: ui-monospace, monospace;
    font-size: 0.75rem;
  }

  .modal-actions {
    display: flex;
    gap: 10px;
    justify-content: flex-end;
  }
</style>
