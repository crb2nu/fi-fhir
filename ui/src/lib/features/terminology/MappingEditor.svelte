<script lang="ts">
  import { afterUpdate, createEventDispatcher, tick } from 'svelte';
  import X from '@lucide/svelte/icons/x';
  import {
    Button,
    Field,
    IconButton,
    Input,
    KeyValue,
    Select,
    Textarea,
    type KeyValueItem
  } from '$lib/ui/primitives';
  import { toasts } from '$lib/ui/toastStore';
  import { isErrorToasted } from '$lib/graphql/client';
  import { updateMapping } from './terminologyApi';
  import { formatTimestamp, originLabel } from './terminologyFormat';
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

  $: readonlyItems = [
    { key: 'Source system', value: mapping.sourceSystem, mono: true },
    { key: 'Source code', value: mapping.sourceCode, mono: true },
    { key: 'Source display', value: mapping.sourceDisplay },
    { key: 'Target system', value: mapping.targetSystem, mono: true },
    { key: 'Target code', value: mapping.targetCode, mono: true },
    { key: 'Origin', value: originLabel(mapping.origin) },
    { key: 'Created', value: formatTimestamp(mapping.createdAt), mono: true },
    { key: 'Created by', value: mapping.createdBy },
    { key: 'Batch id', value: mapping.uploadBatchId, mono: true, truncate: true }
  ] satisfies KeyValueItem[];

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
      // Global graphqlFetch net already toasts graphql failures (B4 dedupe).
      if (!isErrorToasted(err)) {
        toasts.error(err instanceof Error ? err.message : 'Failed to update mapping');
      }
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

  afterUpdate(() => {
    if (open && !wasOpen) {
      tick().then(() => {
        if (!modalEl) return;
        // Start on the first editable field, not the header's close button.
        focusCtl = createDialogFocusController(modalEl, {
          initialFocus: modalEl.querySelector<HTMLElement>('#target-display')
        });
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
      <header class="modal-header">
        <h3 id="modal-title" class="modal-title">Edit mapping</h3>
        <IconButton icon={X} label="Close" onclick={handleClose} />
      </header>

      <div class="modal-body">
        <KeyValue items={readonlyItems} />

        <div class="form-grid">
          <Field label="Target display" id="target-display" class="span-2">
            <Input bind:value={editTargetDisplay} placeholder="Human-readable display name" />
          </Field>

          <Field label="Equivalence" id="equivalence">
            <Select bind:value={editEquivalence} options={equivalenceOptions} />
          </Field>

          <Field label="Confidence" id="confidence">
            <div class="range-row">
              <input
                id="confidence"
                class="range"
                type="range"
                min="0"
                max="1"
                step="0.01"
                bind:value={editConfidence}
              />
              <span class="range-value text-mono">{(editConfidence * 100).toFixed(0)}%</span>
            </div>
          </Field>

          <Field label="Comment" id="comment" class="span-2">
            <Textarea
              bind:value={editComment}
              rows={3}
              placeholder="Optional notes about this mapping"
            />
          </Field>
        </div>
      </div>

      <footer class="modal-actions">
        <Button variant="ghost" size="md" onclick={handleClose} disabled={saving}>Cancel</Button>
        <Button variant="primary" size="md" onclick={handleSave} loading={saving}>
          Save changes
        </Button>
      </footer>
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
  }

  .modal {
    position: relative;
    z-index: 1;
    display: flex;
    flex-direction: column;
    width: 100%;
    max-width: var(--modal-width-md);
    max-height: 90vh;
    background: var(--color-bg-elevated);
    border: 1px solid var(--color-border-default);
    border-radius: var(--modal-radius);
    box-shadow: var(--shadow-xl);
    outline: none;
  }

  .modal-header {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    flex: 0 0 auto;
    height: 44px;
    padding: 0 var(--space-2) 0 var(--space-4);
    border-bottom: 1px solid var(--color-border-subtle);
  }

  .modal-title {
    margin: 0 auto 0 0;
    font-size: var(--text-title);
    font-weight: var(--font-semibold);
    color: var(--color-text-primary);
  }

  .modal-body {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
    min-height: 0;
    overflow-y: auto;
    padding: var(--space-4);
  }

  .form-grid {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: var(--space-3);
  }

  .form-grid :global(.span-2) {
    grid-column: 1 / -1;
  }

  .range-row {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    height: var(--size-control-sm);
  }

  .range {
    flex: 1;
    min-width: 0;
    accent-color: var(--color-primary);
  }

  .range-value {
    min-width: 40px;
    text-align: right;
    color: var(--color-text-secondary);
  }

  .modal-actions {
    display: flex;
    justify-content: flex-end;
    gap: var(--space-2);
    flex: 0 0 auto;
    padding: var(--space-3) var(--space-4);
    border-top: 1px solid var(--color-border-subtle);
  }

  @media (max-width: 480px) {
    .form-grid {
      grid-template-columns: 1fr;
    }
  }
</style>
