<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import Button from '$lib/ui/Button.svelte';
  import { toasts } from '$lib/ui/toastStore';
  import { uploadMappingCSV } from './terminologyApi';
  import { validateCsvFile } from './csvFileValidation';
  import { isErrorToasted } from '$lib/graphql/client';
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

  // Inline file-type rejection (B1: persistent validation belongs inline, not a toast)
  let fileError: string | null = null;

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
    fileError = validateCsvFile(file.name);
    if (fileError) {
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
      // Global graphqlFetch net already toasts graphql failures (B4 dedupe).
      if (!isErrorToasted(err)) {
        toasts.error(message);
      }
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
      // Global graphqlFetch net already toasts graphql failures (B4 dedupe).
      if (!isErrorToasted(err)) {
        toasts.error(message);
      }
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
    fileError = null;
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
    <button
      type="button"
      class="drop-zone"
      on:dragover={handleDragOver}
      on:dragleave={handleDragLeave}
      on:drop={handleDrop}
      on:click={triggerFileSelect}
      disabled={disabled || isUploading}
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
    </button>

    {#if fileError}
      <p class="file-error" role="alert">{fileError}</p>
    {/if}

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
    gap: var(--space-4);
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
    gap: var(--space-3);
    padding: var(--space-10) var(--space-6);
    border: 2px dashed var(--color-border-default);
    border-radius: var(--radius-xl);
    background: var(--color-bg-surface);
    cursor: pointer;
    transition: var(--transition-all);
    color: inherit;
    font: inherit;
    text-align: center;
  }

  .drop-zone:hover,
  .uploader.dragging .drop-zone {
    border-color: var(--color-primary);
    background: var(--color-primary-subtle);
  }

  .drop-zone:focus-visible {
    outline: none;
    box-shadow: var(--shadow-focus);
    border-color: var(--color-border-focus);
  }

  .uploader.dragging .drop-zone {
    border-color: var(--color-primary);
  }

  .drop-icon {
    width: 48px;
    height: 48px;
    color: var(--color-text-muted);
    transition: var(--transition-all);
  }

  .uploader.dragging .drop-icon,
  .drop-zone:hover .drop-icon {
    color: var(--color-primary);
    transform: translateY(-2px);
  }

  .drop-icon svg {
    width: 100%;
    height: 100%;
  }

  .drop-text {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: var(--space-1);
  }

  .drop-primary {
    color: var(--color-text-primary);
    font-size: var(--text-base);
    font-weight: var(--font-medium);
  }

  .drop-secondary {
    color: var(--color-text-muted);
    font-size: var(--text-sm);
  }

  .file-error {
    margin: 0;
    padding: var(--space-3) var(--space-4);
    border-radius: var(--radius-lg);
    background: var(--color-danger-subtle);
    border: 1px solid var(--color-danger-muted);
    color: var(--color-danger);
    font-size: var(--text-sm);
  }

  .format-help {
    padding: var(--space-3) var(--space-4);
    border-radius: var(--radius-lg);
    background: var(--color-bg-surface);
    border: 1px solid var(--color-border-subtle);
  }

  .format-title {
    font-size: var(--text-xs);
    font-weight: var(--font-semibold);
    color: var(--color-text-tertiary);
    text-transform: uppercase;
    letter-spacing: var(--tracking-wide);
    margin-bottom: var(--space-2);
  }

  .format-item {
    font-size: var(--text-sm);
    color: var(--color-text-secondary);
    margin-bottom: var(--space-1);
  }

  .format-item strong {
    color: var(--color-text-primary);
  }

  /* Preview Styles */
  .preview {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
    padding: var(--space-4);
    border-radius: var(--radius-xl);
    background: var(--color-bg-surface);
    border: 1px solid var(--color-border-default);
  }

  .preview-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
  }

  .preview-title {
    font-weight: var(--font-semibold);
    color: var(--color-text-primary);
  }

  .preview-stats {
    display: flex;
    gap: var(--space-3);
  }

  .stat {
    font-size: var(--text-sm);
    padding: var(--space-0) var(--space-2);
    border-radius: var(--radius-sm);
  }

  .stat.valid {
    color: var(--color-success);
    background: var(--color-success-subtle);
  }

  .stat.error {
    color: var(--color-danger);
    background: var(--color-danger-subtle);
  }

  .errors-section {
    padding: var(--space-3);
    border-radius: var(--radius-lg);
    background: var(--color-danger-subtle);
    border: 1px solid var(--color-danger-muted);
  }

  .errors-title {
    font-size: var(--text-sm);
    font-weight: var(--font-semibold);
    color: var(--color-danger);
    margin-bottom: var(--space-2);
  }

  .errors-list {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
  }

  .error-item {
    font-size: var(--text-xs);
    color: var(--color-text-secondary);
    font-family: var(--font-mono);
  }

  .error-more {
    font-size: var(--text-xs);
    color: var(--color-text-muted);
    font-style: italic;
  }

  .preview-table {
    border-radius: var(--radius-lg);
    border: 1px solid var(--color-border-subtle);
    overflow: hidden;
  }

  .preview-row {
    display: grid;
    grid-template-columns: 1fr 1fr 40px;
    gap: var(--space-2);
    padding: var(--space-2) var(--space-3);
    font-size: var(--text-sm);
    color: var(--color-text-primary);
  }

  .preview-row.header {
    background: var(--color-bg-elevated);
    font-weight: var(--font-semibold);
    color: var(--color-text-tertiary);
    text-transform: uppercase;
    font-size: var(--text-xs);
    letter-spacing: var(--tracking-wide);
  }

  .preview-row:not(.header) {
    border-top: 1px solid var(--color-border-subtle);
  }

  .preview-row:not(.header):hover {
    background: var(--color-bg-hover);
  }

  .mono {
    font-family: var(--font-mono);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .equiv {
    text-align: center;
    color: var(--color-primary);
    font-weight: var(--font-semibold);
  }

  .preview-more {
    padding: var(--space-2) var(--space-3);
    text-align: center;
    font-size: var(--text-xs);
    color: var(--color-text-muted);
    border-top: 1px solid var(--color-border-subtle);
  }

  .preview-actions {
    display: flex;
    gap: var(--space-3);
    justify-content: flex-end;
  }
</style>
