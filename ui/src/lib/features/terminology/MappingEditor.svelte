<script lang="ts">
  import { afterUpdate, createEventDispatcher, tick } from 'svelte';
  import Button from '$lib/ui/Button.svelte';
  import { toasts } from '$lib/ui/toastStore';
  import { updateMapping } from './terminologyApi';
  import type { MappingEquivalence, ListMappingsQuery } from '$lib/gen/graphql';
  import { createDialogFocusController } from '$lib/domain/a11yDialog';

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

  let modalEl: HTMLDivElement | null = null;
  let wasOpen = false;
  let focusCtl: ReturnType<typeof createDialogFocusController> | null = null;

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
      return;
    }
    if (e.key === 'Tab') {
      focusCtl?.onKeydown(e);
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

  afterUpdate(() => {
    if (open && !wasOpen) {
      tick().then(() => {
        if (!modalEl) return;
        focusCtl = createDialogFocusController(modalEl);
        focusCtl.focusInitial();
      });
    }
    if (!open && wasOpen) {
      focusCtl?.restoreFocus();
      focusCtl = null;
    }
    wasOpen = open;
  });
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
      bind:this={modalEl}
      role="dialog"
      aria-modal="true"
      aria-labelledby="modal-title"
      tabindex="-1"
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
    z-index: var(--z-modal);
    padding: var(--space-4);
  }

  .modal-backdrop {
    position: absolute;
    inset: 0;
    border: 0;
    padding: 0;
    background: var(--modal-backdrop);
    cursor: default;
    animation: fadeIn var(--duration-fast) var(--ease-out);
  }

  .modal {
    position: relative;
    z-index: 1;
    background: var(--color-bg-base);
    border: 1px solid var(--color-border-default);
    border-radius: var(--modal-radius);
    padding: var(--space-6);
    width: 100%;
    max-width: var(--modal-width-md);
    max-height: 90vh;
    overflow-y: auto;
    box-shadow: var(--shadow-xl);
    animation: modalIn var(--duration-normal) var(--ease-out);
  }

  @keyframes fadeIn {
    from { opacity: 0; }
    to { opacity: 1; }
  }

  @keyframes modalIn {
    from {
      opacity: 0;
      transform: scale(0.95) translateY(-8px);
    }
    to {
      opacity: 1;
      transform: scale(1) translateY(0);
    }
  }

  .modal-title {
    margin: 0 0 var(--space-5);
    font-size: var(--text-lg);
    font-weight: var(--font-bold);
    color: var(--color-text-primary);
  }

  .modal-body {
    margin-bottom: var(--space-5);
  }

  .divider {
    height: 1px;
    background: var(--color-border-subtle);
    margin: var(--space-4) 0;
  }

  .field-group {
    margin-bottom: var(--space-3);
  }

  .field-group.full-width {
    grid-column: 1 / -1;
  }

  .field-row {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: var(--space-4);
  }

  .field-label {
    display: block;
    font-size: var(--text-xs);
    font-weight: var(--font-semibold);
    color: var(--color-text-tertiary);
    text-transform: uppercase;
    letter-spacing: var(--tracking-wide);
    margin-bottom: var(--space-1);
  }

  .field-value {
    font-size: var(--text-sm);
    color: var(--color-text-primary);
    line-height: var(--leading-normal);
  }

  .field-value.readonly {
    padding: var(--space-2) var(--space-3);
    background: var(--color-bg-elevated);
    border-radius: var(--radius-md);
    border: 1px solid var(--color-border-subtle);
  }

  .field-value.mono {
    font-family: var(--font-mono);
  }

  .field-input,
  .field-select,
  .field-textarea {
    width: 100%;
    padding: var(--space-2) var(--space-3);
    border-radius: var(--radius-lg);
    border: 1px solid var(--color-border-default);
    background: var(--color-bg-input);
    color: var(--color-text-primary);
    font-size: var(--text-sm);
    font-family: inherit;
    outline: none;
    transition: var(--transition-all);
  }

  .field-input:focus,
  .field-select:focus,
  .field-textarea:focus {
    border-color: var(--color-border-focus);
    box-shadow: var(--shadow-focus);
  }

  .field-textarea {
    resize: vertical;
    min-height: 80px;
  }

  .confidence-input {
    display: flex;
    align-items: center;
    gap: var(--space-3);
  }

  .confidence-input input[type="range"] {
    flex: 1;
    accent-color: var(--color-primary);
    height: 6px;
  }

  .confidence-value {
    min-width: 45px;
    text-align: right;
    font-family: var(--font-mono);
    font-size: var(--text-sm);
    font-weight: var(--font-semibold);
    color: var(--color-text-secondary);
  }

  .metadata-section {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2) var(--space-6);
  }

  .metadata-row {
    display: flex;
    gap: var(--space-2);
    font-size: var(--text-xs);
  }

  .meta-label {
    color: var(--color-text-muted);
  }

  .meta-value {
    color: var(--color-text-tertiary);
  }

  .meta-value.mono {
    font-family: var(--font-mono);
    font-size: var(--text-2xs);
  }

  .modal-actions {
    display: flex;
    gap: var(--space-3);
    justify-content: flex-end;
  }

  @media (max-width: 480px) {
    .field-row {
      grid-template-columns: 1fr;
    }
  }
</style>
