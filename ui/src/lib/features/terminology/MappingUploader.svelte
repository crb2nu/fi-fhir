<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import CircleAlert from '@lucide/svelte/icons/circle-alert';
  import Upload from '@lucide/svelte/icons/upload';
  import { Badge, Button, Icon, Panel, Table, Td, Th, Tr } from '$lib/ui/primitives';
  import { toasts } from '$lib/ui/toastStore';
  import { uploadMappingCSV } from './terminologyApi';
  import { validateCsvFile } from './csvFileValidation';
  import { equivalenceLabel } from './terminologyFormat';
  import { isErrorToasted } from '$lib/graphql/client';
  import type { UploadMappingCsvInput } from '$lib/gen/graphql';

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
</script>

<div class="uploader" class:disabled class:is-preview={showPreview}>
  <input
    bind:this={fileInputEl}
    type="file"
    accept=".csv"
    on:change={handleFileSelect}
    class="hidden-input"
  />

  {#if !showPreview}
    <Panel title="Upload CSV">
      <div class="upload-body">
        <!-- Drop Zone -->
        <button
          type="button"
          class="drop-zone"
          class:dragging={isDragging}
          on:dragover={handleDragOver}
          on:dragleave={handleDragLeave}
          on:drop={handleDrop}
          on:click={triggerFileSelect}
          disabled={disabled || isUploading}
          aria-busy={isUploading ? 'true' : undefined}
        >
          <Icon icon={Upload} />
          {#if isUploading}
            <span>Validating <span class="text-mono">{filename}</span>…</span>
          {:else}
            <span>Drop a CSV file here or <span class="drop-link">browse</span></span>
          {/if}
        </button>

        {#if fileError}
          <p class="file-error" role="alert">
            <Icon icon={CircleAlert} />
            <span>{fileError}</span>
          </p>
        {/if}

        <dl class="formats" aria-label="Accepted CSV formats">
          <dt>Standard</dt>
          <dd class="text-mono">source_system, source_code, target_system, target_code, equivalence</dd>
          <dt>Simple</dt>
          <dd>
            <span class="text-mono">source_code, target_code</span>
            <span class="formats-note">requires default source and target systems</span>
          </dd>
        </dl>
      </div>
    </Panel>
  {:else if previewResult}
    {@const validRows = previewResult.batch?.validRows ?? 0}
    {@const errorRows = previewResult.batch?.errorRows ?? 0}
    <!-- Preview Results -->
    <Panel flush aria-label="Preview of {filename}">
      {#snippet header()}
        <h2 class="preview-title text-label">Preview</h2>
        <span class="preview-file text-mono" title={filename}>{filename}</span>
      {/snippet}
      {#snippet actions()}
        <span class="preview-counts">
          <Badge tone={validRows > 0 ? 'success' : 'neutral'} mono>{validRows} valid</Badge>
          <Badge tone={errorRows > 0 ? 'danger' : 'neutral'} mono
            >{errorRows} {errorRows === 1 ? 'error' : 'errors'}</Badge
          >
        </span>
      {/snippet}

      {#if previewResult.batch?.validationErrors && previewResult.batch.validationErrors.length > 0}
        <section class="preview-section" aria-label="Validation errors">
          <h3 class="section-title text-label">Validation errors</h3>
          <Table label="Validation errors" layout="fixed">
            {#snippet head()}
              <tr>
                <Th width="72px" numeric>Row</Th>
                <Th width="160px">Column</Th>
                <Th>Message</Th>
              </tr>
            {/snippet}
            {#each previewResult.batch.validationErrors.slice(0, 5) as error, i (i)}
              <Tr>
                <Td numeric value={error.row} />
                <Td mono truncate value={error.column || '—'} />
                <Td truncate value={error.message} />
              </Tr>
            {/each}
          </Table>
          {#if previewResult.batch.validationErrors.length > 5}
            <p class="preview-more">
              and {previewResult.batch.validationErrors.length - 5} more errors
            </p>
          {/if}
        </section>
      {/if}

      {#if previewResult.preview && previewResult.preview.length > 0}
        <section class="preview-section" aria-label="Mappings to upload">
          <h3 class="section-title text-label">Mappings</h3>
          <Table label="Mappings to upload" layout="fixed">
            {#snippet head()}
              <tr>
                <Th>Source system</Th>
                <Th width="140px">Source code</Th>
                <Th>Target system</Th>
                <Th width="140px">Target code</Th>
                <Th width="112px">Equivalence</Th>
              </tr>
            {/snippet}
            {#each previewResult.preview.slice(0, 10) as mapping (mapping.id)}
              <Tr>
                <Td muted truncate value={mapping.sourceSystem} />
                <Td mono truncate value={mapping.sourceCode} />
                <Td muted truncate value={mapping.targetSystem} />
                <Td mono truncate value={mapping.targetCode} />
                <Td value={equivalenceLabel(mapping.equivalence)} />
              </Tr>
            {/each}
          </Table>
          {#if previewResult.preview.length > 10}
            <p class="preview-more">
              and {previewResult.preview.length - 10} more mappings
            </p>
          {/if}
        </section>
      {/if}

      <div class="preview-actions">
        <Button variant="ghost" onclick={cancelUpload} disabled={isUploading}>Cancel</Button>
        <Button
          variant="primary"
          icon={Upload}
          onclick={confirmUpload}
          loading={isUploading}
          disabled={validRows === 0}
        >
          Upload {validRows} {validRows === 1 ? 'mapping' : 'mappings'}
        </Button>
      </div>
    </Panel>
  {/if}
</div>

<style>
  .uploader {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    max-width: 720px;
    padding: var(--space-3);
  }

  .uploader.is-preview {
    max-width: none;
  }

  .uploader.disabled {
    opacity: 0.5;
    pointer-events: none;
  }

  .hidden-input {
    display: none;
  }

  .upload-body {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
  }

  .drop-zone {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: var(--space-2);
    min-height: 96px;
    padding: var(--space-4);
    border: 1px dashed var(--color-border-strong);
    border-radius: var(--radius-sm);
    background: var(--color-bg-input);
    color: var(--color-text-secondary);
    font: inherit;
    font-size: var(--text-ui);
    cursor: pointer;
    transition: var(--transition-colors);
  }

  .drop-zone:hover:not(:disabled),
  .drop-zone.dragging {
    background: var(--color-bg-hover);
    color: var(--color-text-primary);
  }

  .drop-zone.dragging {
    border-style: solid;
    border-color: var(--color-border-focus);
  }

  .drop-zone:focus-visible {
    outline: 2px solid var(--color-focus-ring);
    outline-offset: 1px;
  }

  .drop-zone:disabled {
    cursor: progress;
  }

  .drop-link {
    color: var(--color-text-primary);
    text-decoration: underline;
    text-underline-offset: 2px;
  }

  .file-error {
    display: flex;
    align-items: flex-start;
    gap: var(--space-2);
    margin: 0;
    font-size: var(--text-xs);
    color: var(--color-danger-text);
  }

  .formats {
    display: grid;
    grid-template-columns: max-content minmax(0, 1fr);
    column-gap: var(--space-4);
    row-gap: 6px;
    margin: 0;
    align-items: baseline;
  }

  .formats dt {
    font-size: var(--text-xs);
    color: var(--color-text-tertiary);
  }

  .formats dd {
    margin: 0;
    min-width: 0;
    overflow-wrap: anywhere;
    color: var(--color-text-primary);
  }

  .formats-note {
    margin-left: var(--space-2);
    font-size: var(--text-xs);
    color: var(--color-text-tertiary);
  }

  .preview-title {
    margin: 0;
  }

  .preview-file {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: var(--color-text-secondary);
  }

  .preview-counts {
    display: inline-flex;
    gap: var(--space-1);
    padding-right: var(--space-1);
  }

  .preview-section {
    display: flex;
    flex-direction: column;
    border-bottom: 1px solid var(--color-border-subtle);
  }

  .section-title {
    margin: 0;
    padding: var(--space-2) var(--space-3);
  }

  .preview-more {
    margin: 0;
    padding: var(--space-2) var(--space-3);
    font-size: var(--text-xs);
    color: var(--color-text-tertiary);
  }

  .preview-actions {
    display: flex;
    justify-content: flex-end;
    gap: var(--space-2);
    padding: var(--space-2) var(--space-3);
  }
</style>
