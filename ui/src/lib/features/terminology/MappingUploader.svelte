<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import Button from '$lib/ui/Button.svelte';
  import { toasts } from '$lib/ui/toastStore';
  import { uploadMappingCSV } from './terminologyApi';
  import type { UploadMappingCsvInput, MappingEquivalence } from '$lib/gen/graphql';

  export let profileId: string | undefined = undefined;
  export let defaultSourceSystem = '';
  export let defaultTargetSystem = '';
  export let disabled = false;

  const dispatch = createEventDispatcher<{
    uploadComplete: { batchId: string; created: number; skipped: number };
    uploadError: { message: string };
  }>();

  let fileInputEl: HTMLInputElement | null = null;
  let isDragging = false;
  let isUploading = false;
  let csvContent = '';
  let filename = '';

  // Preview state
  let showPreview = false;
  let previewResult: Awaited<ReturnType<typeof uploadMappingCSV>> | null = null;

  function triggerFileSelect() {
    fileInputEl?.click();
  }

  function handleDragOver(e: DragEvent) {
    if (!e.dataTransfer?.types?.includes('Files')) return;
    e.preventDefault();
    isDragging = true;
  }

  function handleDragLeave() {
    isDragging = false;
  }

  function handleDrop(e: DragEvent) {
    e.preventDefault();
    isDragging = false;
    const files = e.dataTransfer?.files;
    const file = files?.[0];
    if (file) {
      handleFile(file);
    }
  }

  function handleFileSelect(e: Event) {
    const input = e.target as HTMLInputElement;
    const file = input.files?.[0];
    if (file) {
      handleFile(file);
    }
  }

  async function handleFile(file: File) {
    if (!file.name.endsWith('.csv')) {
      toasts.error('Please select a CSV file');
      return;
    }

    filename = file.name;
    csvContent = await file.text();

    // Auto-preview
    await previewUpload();
  }

  async function previewUpload() {
    if (!csvContent) return;

    isUploading = true;
    try {
      const input: UploadMappingCsvInput = {
        csv: csvContent,
        filename,
        dryRun: true,
        defaultSourceSystem: defaultSourceSystem || null,
        defaultTargetSystem: defaultTargetSystem || null,
        profileId: profileId ?? null
      };

      previewResult = await uploadMappingCSV(input);
      showPreview = true;
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Preview failed';
      toasts.error(message);
      dispatch('uploadError', { message });
    } finally {
      isUploading = false;
    }
  }

  async function confirmUpload() {
    if (!csvContent) return;

    isUploading = true;
    try {
      const input: UploadMappingCsvInput = {
        csv: csvContent,
        filename,
        dryRun: false,
        defaultSourceSystem: defaultSourceSystem || null,
        defaultTargetSystem: defaultTargetSystem || null,
        profileId: profileId ?? null
      };

      const result = await uploadMappingCSV(input);

      toasts.success(`Uploaded ${result.mappingsCreated} mappings`);
      dispatch('uploadComplete', {
        batchId: result.batch?.id ?? '',
        created: result.mappingsCreated,
        skipped: result.mappingsSkipped
      });

      // Reset state
      csvContent = '';
      filename = '';
      showPreview = false;
      previewResult = null;
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Upload failed';
      toasts.error(message);
      dispatch('uploadError', { message });
    } finally {
      isUploading = false;
    }
  }

  function cancelUpload() {
    csvContent = '';
    filename = '';
    showPreview = false;
    previewResult = null;
  }

  function formatEquivalence(eq: MappingEquivalence): string {
    switch (eq) {
      case 'EQUIVALENT': return '=';
      case 'WIDER': return '>';
      case 'NARROWER': return '<';
      case 'INEXACT': return '~';
      default: return eq;
    }
  }
</script>

<div class="uploader" class:disabled class:dragging={isDragging}>
  <input
    bind:this={fileInputEl}
    type="file"
    accept=".csv"
    on:change={handleFileSelect}
    class="hidden-input"
  />

  {#if !showPreview}
    <!-- Drop Zone -->
    <div
      class="drop-zone"
      on:dragover={handleDragOver}
      on:dragleave={handleDragLeave}
      on:drop={handleDrop}
      role="button"
      tabindex="0"
      on:click={triggerFileSelect}
      on:keydown={(e) => e.key === 'Enter' && triggerFileSelect()}
    >
      <div class="drop-icon">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M21 15v4a2 2 0 01-2 2H5a2 2 0 01-2-2v-4" />
          <polyline points="17,8 12,3 7,8" />
          <line x1="12" y1="3" x2="12" y2="15" />
        </svg>
      </div>
      <div class="drop-text">
        <span class="drop-primary">Drop CSV file here</span>
        <span class="drop-secondary">or click to browse</span>
      </div>
    </div>

    <!-- Format Help -->
    <div class="format-help">
      <div class="format-title">Supported CSV Formats:</div>
      <div class="format-item">
        <strong>Standard:</strong> source_system, source_code, target_system, target_code, equivalence
      </div>
      <div class="format-item">
        <strong>Simple:</strong> source_code, target_code (requires default systems above)
      </div>
    </div>
  {:else if previewResult}
    <!-- Preview Results -->
    <div class="preview">
      <div class="preview-header">
        <div class="preview-title">
          Preview: {filename}
        </div>
        <div class="preview-stats">
          <span class="stat valid">{previewResult.batch?.validRows ?? 0} valid</span>
          <span class="stat error">{previewResult.batch?.errorRows ?? 0} errors</span>
        </div>
      </div>

      {#if previewResult.batch?.validationErrors && previewResult.batch.validationErrors.length > 0}
        <div class="errors-section">
          <div class="errors-title">Validation Errors:</div>
          <div class="errors-list">
            {#each previewResult.batch.validationErrors.slice(0, 5) as error, i (i)}
              <div class="error-item">
                Row {error.row}{error.column ? `, ${error.column}` : ''}: {error.message}
              </div>
            {/each}
            {#if previewResult.batch.validationErrors.length > 5}
              <div class="error-more">
                ...and {previewResult.batch.validationErrors.length - 5} more errors
              </div>
            {/if}
          </div>
        </div>
      {/if}

      {#if previewResult.preview && previewResult.preview.length > 0}
        <div class="preview-table">
          <div class="preview-row header">
            <span>Source</span>
            <span>Target</span>
            <span>Eq</span>
          </div>
          {#each previewResult.preview.slice(0, 10) as mapping (mapping.id)}
            <div class="preview-row">
              <span class="mono">{mapping.sourceSystem}:{mapping.sourceCode}</span>
              <span class="mono">{mapping.targetSystem.split('/').pop()}:{mapping.targetCode}</span>
              <span class="equiv">{formatEquivalence(mapping.equivalence)}</span>
            </div>
          {/each}
          {#if previewResult.preview.length > 10}
            <div class="preview-more">
              ...and {previewResult.preview.length - 10} more mappings
            </div>
          {/if}
        </div>
      {/if}

      <div class="preview-actions">
        <Button variant="secondary" on:click={cancelUpload} disabled={isUploading}>
          Cancel
        </Button>
        <Button
          variant="primary"
          on:click={confirmUpload}
          disabled={isUploading || (previewResult.batch?.validRows ?? 0) === 0}
        >
          {#if isUploading}
            Uploading...
          {:else}
            Upload {previewResult.batch?.validRows ?? 0} Mappings
          {/if}
        </Button>
      </div>
    </div>
  {/if}
</div>

<style>
  .uploader {
    display: flex;
    flex-direction: column;
    gap: 16px;
  }

  .uploader.disabled {
    opacity: 0.5;
    pointer-events: none;
  }

  .hidden-input {
    display: none;
  }

  .drop-zone {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 12px;
    padding: 40px 24px;
    border: 2px dashed rgba(255, 255, 255, 0.15);
    border-radius: 12px;
    background: rgba(255, 255, 255, 0.02);
    cursor: pointer;
    transition: all 0.15s ease;
  }

  .drop-zone:hover,
  .uploader.dragging .drop-zone {
    border-color: rgba(59, 130, 246, 0.5);
    background: rgba(59, 130, 246, 0.05);
  }

  .drop-icon {
    width: 48px;
    height: 48px;
    color: rgba(229, 231, 235, 0.5);
  }

  .drop-icon svg {
    width: 100%;
    height: 100%;
  }

  .drop-text {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 4px;
  }

  .drop-primary {
    color: rgba(229, 231, 235, 0.85);
    font-size: 0.95rem;
    font-weight: 500;
  }

  .drop-secondary {
    color: rgba(229, 231, 235, 0.5);
    font-size: 0.85rem;
  }

  .format-help {
    padding: 12px 16px;
    border-radius: 8px;
    background: rgba(255, 255, 255, 0.03);
    border: 1px solid rgba(255, 255, 255, 0.06);
  }

  .format-title {
    font-size: 0.8rem;
    font-weight: 600;
    color: rgba(229, 231, 235, 0.6);
    text-transform: uppercase;
    letter-spacing: 0.02em;
    margin-bottom: 8px;
  }

  .format-item {
    font-size: 0.85rem;
    color: rgba(229, 231, 235, 0.7);
    margin-bottom: 4px;
  }

  .format-item strong {
    color: rgba(229, 231, 235, 0.9);
  }

  /* Preview Styles */
  .preview {
    display: flex;
    flex-direction: column;
    gap: 16px;
    padding: 16px;
    border-radius: 12px;
    background: rgba(255, 255, 255, 0.03);
    border: 1px solid rgba(255, 255, 255, 0.08);
  }

  .preview-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
  }

  .preview-title {
    font-weight: 600;
    color: rgba(229, 231, 235, 0.9);
  }

  .preview-stats {
    display: flex;
    gap: 12px;
  }

  .stat {
    font-size: 0.85rem;
    padding: 2px 8px;
    border-radius: 4px;
  }

  .stat.valid {
    color: rgba(34, 197, 94, 0.9);
    background: rgba(34, 197, 94, 0.1);
  }

  .stat.error {
    color: rgba(239, 68, 68, 0.9);
    background: rgba(239, 68, 68, 0.1);
  }

  .errors-section {
    padding: 12px;
    border-radius: 8px;
    background: rgba(239, 68, 68, 0.05);
    border: 1px solid rgba(239, 68, 68, 0.2);
  }

  .errors-title {
    font-size: 0.85rem;
    font-weight: 600;
    color: rgba(239, 68, 68, 0.9);
    margin-bottom: 8px;
  }

  .errors-list {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .error-item {
    font-size: 0.8rem;
    color: rgba(229, 231, 235, 0.7);
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  }

  .error-more {
    font-size: 0.8rem;
    color: rgba(229, 231, 235, 0.5);
    font-style: italic;
  }

  .preview-table {
    border-radius: 8px;
    border: 1px solid rgba(255, 255, 255, 0.06);
    overflow: hidden;
  }

  .preview-row {
    display: grid;
    grid-template-columns: 1fr 1fr 40px;
    gap: 8px;
    padding: 8px 12px;
    font-size: 0.85rem;
    color: rgba(229, 231, 235, 0.85);
  }

  .preview-row.header {
    background: rgba(255, 255, 255, 0.03);
    font-weight: 600;
    color: rgba(229, 231, 235, 0.6);
    text-transform: uppercase;
    font-size: 0.75rem;
    letter-spacing: 0.02em;
  }

  .preview-row:not(.header) {
    border-top: 1px solid rgba(255, 255, 255, 0.04);
  }

  .mono {
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .equiv {
    text-align: center;
    color: rgba(59, 130, 246, 0.9);
  }

  .preview-more {
    padding: 8px 12px;
    text-align: center;
    font-size: 0.8rem;
    color: rgba(229, 231, 235, 0.5);
    border-top: 1px solid rgba(255, 255, 255, 0.04);
  }

  .preview-actions {
    display: flex;
    gap: 12px;
    justify-content: flex-end;
  }
</style>
