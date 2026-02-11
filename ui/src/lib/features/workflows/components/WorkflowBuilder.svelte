<script lang="ts">
  import { get } from 'svelte/store';
  import { slide } from 'svelte/transition';
  import Panel from '$lib/ui/Panel.svelte';
  import Button from '$lib/ui/Button.svelte';
  import RouteEditor from './RouteEditor.svelte';
  import WorkflowPreview from './WorkflowPreview.svelte';
  import DryRunPanel from './DryRunPanel.svelte';
  import GenerateFromDescription from './GenerateFromDescription.svelte';
  import WorkflowDraftLibrary from './WorkflowDraftLibrary.svelte';
  import { workflowDraft, isWorkflowValid } from '../workflowStore';
  import { draftToYaml, yamlToDraft } from '../workflowYaml';
  import {
    createWorkflowDefinition,
    fetchWorkflowVersionById,
    fetchWorkflowVersions,
    publishWorkflowVersion,
    requestWorkflowApproval,
    saveWorkflowVersion
  } from '../workflowApi';
  import type { GetWorkflowVersionsQuery } from '$lib/gen/graphql';
  import { toasts } from '$lib/ui/toastStore';

  type ManagedSelection = {
    workflowId: string;
    name: string;
    description: string | null;
    versionId: string | null;
    versionNumber: number | null;
  };
  type WorkflowVersionItem = GetWorkflowVersionsQuery['workflowVersions'][number];

  export let managedSelection: ManagedSelection | null = null;

  let showPreview = false;
  let showDryRun = false;
  let showGenerate = false;

  let linkedWorkflowId = '';
  let linkedWorkflowName = '';
  let linkedDescription = '';
  let versionNotes = '';
  let publishEnvironment = 'staging';
  let versionHistory: WorkflowVersionItem[] = [];
  let selectedVersionId = '';
  let loadedVersionNumber: number | null = null;

  let creatingDefinition = false;
  let loadingVersionHistory = false;
  let loadingVersion = false;
  let savingVersion = false;
  let publishingVersion = false;
  let requestingApproval = false;
  let lifecycleError: string | null = null;
  let selectionSyncKey = '';

  $: if (managedSelection) {
    const nextKey = `${managedSelection.workflowId}:${managedSelection.versionId ?? ''}`;
    if (nextKey !== selectionSyncKey) {
      selectionSyncKey = nextKey;
      void syncFromManagedSelection(managedSelection);
    }
  }

  async function syncFromManagedSelection(selection: ManagedSelection) {
    linkedWorkflowId = selection.workflowId;
    linkedWorkflowName = selection.name;
    linkedDescription = selection.description ?? '';
    lifecycleError = null;
    publishEnvironment = 'staging';
    loadedVersionNumber = selection.versionNumber ?? null;

    await loadVersionHistory(selection.workflowId, selection.versionId ?? undefined);
    if (selection.versionId) {
      await loadVersionIntoBuilder(selection.versionId);
    }
  }

  async function loadVersionHistory(workflowId: string, preferredVersionId?: string) {
    if (!workflowId) return;
    loadingVersionHistory = true;
    lifecycleError = null;
    try {
      const data = await fetchWorkflowVersions(workflowId, { limit: 100, offset: 0 });
      versionHistory = data.workflowVersions;

      const fallbackVersionId = data.workflowVersions[0]?.id ?? '';
      selectedVersionId = preferredVersionId && preferredVersionId.trim() ? preferredVersionId : fallbackVersionId;
    } catch (err) {
      lifecycleError = err instanceof Error ? err.message : 'Failed to load workflow versions';
      toasts.error(lifecycleError);
    } finally {
      loadingVersionHistory = false;
    }
  }

  async function createManagedDefinition() {
    const name = get(workflowDraft).name.trim();
    if (!name) {
      toasts.error('Workflow name is required to create a managed definition');
      return;
    }

    creatingDefinition = true;
    lifecycleError = null;
    try {
      const data = await createWorkflowDefinition({
        name,
        description: linkedDescription.trim() || null
      });

      linkedWorkflowId = data.createWorkflowDefinition.id;
      linkedWorkflowName = data.createWorkflowDefinition.name;
      linkedDescription = data.createWorkflowDefinition.description ?? '';
      versionHistory = [];
      selectedVersionId = '';
      loadedVersionNumber = null;
      toasts.success(`Created managed workflow: ${linkedWorkflowName}`);
    } catch (err) {
      lifecycleError = err instanceof Error ? err.message : 'Failed to create workflow definition';
      toasts.error(lifecycleError);
    } finally {
      creatingDefinition = false;
    }
  }

  async function saveManagedVersion() {
    if (!linkedWorkflowId) {
      toasts.error('Create or open a managed workflow definition first');
      return;
    }

    const draft = get(workflowDraft);
    if (!draft.name.trim()) {
      toasts.error('Workflow name is required');
      return;
    }

    if (linkedWorkflowName && draft.name.trim() !== linkedWorkflowName) {
      toasts.error(`Draft name must match managed definition name "${linkedWorkflowName}"`);
      return;
    }

    savingVersion = true;
    lifecycleError = null;

    try {
      const yaml = draftToYaml(draft);
      const data = await saveWorkflowVersion({
        workflowId: linkedWorkflowId,
        yaml,
        notes: versionNotes.trim() || null
      });

      const version = data.saveWorkflowVersion;
      selectedVersionId = version.id;
      loadedVersionNumber = version.versionNumber;
      versionNotes = '';
      await loadVersionHistory(linkedWorkflowId, version.id);
      toasts.success(`Saved workflow version v${version.versionNumber}`);
    } catch (err) {
      lifecycleError = err instanceof Error ? err.message : 'Failed to save workflow version';
      toasts.error(lifecycleError);
    } finally {
      savingVersion = false;
    }
  }

  async function loadVersionIntoBuilder(versionId: string) {
    if (!versionId) return;
    loadingVersion = true;
    lifecycleError = null;

    try {
      const data = await fetchWorkflowVersionById(versionId);
      if (!data.workflowVersion) {
        throw new Error('Workflow version not found');
      }

      const parsedDraft = yamlToDraft(data.workflowVersion.yaml);
      workflowDraft.loadDraft(parsedDraft);
      selectedVersionId = data.workflowVersion.id;
      loadedVersionNumber = data.workflowVersion.versionNumber;
      linkedWorkflowId = data.workflowVersion.workflowId;
      toasts.success(`Loaded v${data.workflowVersion.versionNumber} into builder`);
    } catch (err) {
      lifecycleError = err instanceof Error ? err.message : 'Failed to load workflow version';
      toasts.error(lifecycleError);
    } finally {
      loadingVersion = false;
    }
  }

  async function publishManagedVersion() {
    if (!linkedWorkflowId) {
      toasts.error('Create or open a managed workflow definition first');
      return;
    }
    if (!selectedVersionId) {
      toasts.error('Select a version to publish');
      return;
    }

    publishingVersion = true;
    lifecycleError = null;

    try {
      await publishWorkflowVersion({
        workflowId: linkedWorkflowId,
        versionId: selectedVersionId,
        environment: publishEnvironment
      });
      toasts.success(`Published version to ${publishEnvironment}`);
    } catch (err) {
      lifecycleError = err instanceof Error ? err.message : 'Failed to publish version';
      toasts.error(lifecycleError);
    } finally {
      publishingVersion = false;
    }
  }

  async function requestApproval() {
    if (!linkedWorkflowId) {
      toasts.error('Create or open a managed workflow definition first');
      return;
    }
    if (!selectedVersionId) {
      toasts.error('Select a version for approval');
      return;
    }

    requestingApproval = true;
    lifecycleError = null;
    try {
      const data = await requestWorkflowApproval({
        workflowId: linkedWorkflowId,
        targetVersionId: selectedVersionId,
        environment: publishEnvironment,
        comment: versionNotes.trim() || null
      });
      toasts.success(`Approval requested (${data.requestWorkflowApproval.status})`);
    } catch (err) {
      lifecycleError = err instanceof Error ? err.message : 'Failed to request approval';
      toasts.error(lifecycleError);
    } finally {
      requestingApproval = false;
    }
  }

  async function refreshVersionHistory() {
    if (!linkedWorkflowId) return;
    await loadVersionHistory(linkedWorkflowId, selectedVersionId || undefined);
  }

  function unlinkManagedDefinition() {
    linkedWorkflowId = '';
    linkedWorkflowName = '';
    versionHistory = [];
    selectedVersionId = '';
    loadedVersionNumber = null;
    lifecycleError = null;
    selectionSyncKey = '';
  }
</script>

<div class="builder">
  <Panel>
    <div class="builder-header">
      <div class="name-version">
        <label class="field-label">
          Workflow Name
          <input
            type="text"
            class="input"
            value={$workflowDraft.name}
            placeholder="e.g. adt-routing"
            on:input={(e) =>
              workflowDraft.update((d) => ({
                ...d,
                name: (e.target as HTMLInputElement).value
              }))}
          />
        </label>
        <label class="field-label version-field">
          Version
          <input
            type="text"
            class="input"
            value={$workflowDraft.version}
            placeholder="1.0"
            on:input={(e) =>
              workflowDraft.update((d) => ({
                ...d,
                version: (e.target as HTMLInputElement).value
              }))}
          />
        </label>
      </div>

      <label class="field-label">
        Description
        <input
          type="text"
          class="input"
          bind:value={linkedDescription}
          placeholder="Optional managed workflow description"
        />
      </label>

      <div class="managed-row">
        <div class="managed-summary">
          <div class="managed-label">Managed Definition</div>
          {#if linkedWorkflowId}
            <div class="managed-value mono">{linkedWorkflowId}</div>
            <div class="managed-name muted">{linkedWorkflowName}</div>
          {:else}
            <div class="managed-value muted">Not connected</div>
          {/if}
        </div>

        <label class="field-label">
          Load Version
          <select
            class="input"
            bind:value={selectedVersionId}
            disabled={loadingVersionHistory || versionHistory.length === 0}
          >
            {#if versionHistory.length === 0}
              <option value="">No versions</option>
            {:else}
              {#each versionHistory as version (version.id)}
                <option value={version.id}>
                  v{version.versionNumber} · {new Date(version.createdAt).toLocaleString()}
                </option>
              {/each}
            {/if}
          </select>
        </label>

        <label class="field-label">
          Publish Env
          <select class="input" bind:value={publishEnvironment}>
            <option value="staging">staging</option>
            <option value="production">production</option>
          </select>
        </label>

        <label class="field-label">
          Notes
          <input
            type="text"
            class="input"
            bind:value={versionNotes}
            placeholder="Version notes or approval comment"
          />
        </label>
      </div>

      <div class="managed-actions">
        <Button
          variant="secondary"
          size="sm"
          on:click={createManagedDefinition}
          loading={creatingDefinition}
          disabled={!!linkedWorkflowId}
        >
          {creatingDefinition ? 'Creating...' : 'Create Definition'}
        </Button>
        <Button
          variant="secondary"
          size="sm"
          on:click={refreshVersionHistory}
          loading={loadingVersionHistory}
          disabled={!linkedWorkflowId}
        >
          {loadingVersionHistory ? 'Refreshing...' : 'Refresh Versions'}
        </Button>
        <Button
          variant="secondary"
          size="sm"
          on:click={() => loadVersionIntoBuilder(selectedVersionId)}
          loading={loadingVersion}
          disabled={!selectedVersionId || !linkedWorkflowId}
        >
          {loadingVersion ? 'Loading...' : 'Load Version'}
        </Button>
        <Button
          size="sm"
          on:click={saveManagedVersion}
          loading={savingVersion}
          disabled={!linkedWorkflowId || !$isWorkflowValid}
        >
          {savingVersion ? 'Saving...' : 'Save Version'}
        </Button>
        <Button
          size="sm"
          variant="secondary"
          on:click={publishManagedVersion}
          loading={publishingVersion}
          disabled={!linkedWorkflowId || !selectedVersionId}
        >
          {publishingVersion ? 'Publishing...' : 'Publish'}
        </Button>
        <Button
          size="sm"
          variant="secondary"
          on:click={requestApproval}
          loading={requestingApproval}
          disabled={!linkedWorkflowId || !selectedVersionId}
        >
          {requestingApproval ? 'Requesting...' : 'Request Approval'}
        </Button>
        <Button variant="secondary" size="sm" on:click={unlinkManagedDefinition}>
          Unlink
        </Button>
      </div>

      {#if loadedVersionNumber !== null}
        <div class="loaded-version muted">Loaded version: v{loadedVersionNumber}</div>
      {/if}
      {#if lifecycleError}
        <div class="lifecycle-error" role="alert">{lifecycleError}</div>
      {/if}
    </div>
  </Panel>

  <div class="routes">
    {#each $workflowDraft.routes as route (route._key)}
      <RouteEditor
        {route}
        on:toggleExpand={() => workflowDraft.toggleRouteExpanded(route._key)}
        on:remove={() => workflowDraft.removeRoute(route._key)}
        on:updateName={(e) => workflowDraft.updateRoute(route._key, { name: e.detail })}
        on:updateFilter={(e) => workflowDraft.updateRoute(route._key, { filter: e.detail })}
        on:addTransform={() => workflowDraft.addTransform(route._key)}
        on:removeTransform={(e) =>
          workflowDraft.removeTransform(route._key, e.detail.transformKey)}
        on:changeTransform={(e) =>
          workflowDraft.updateTransform(route._key, e.detail.transformKey, e.detail.transform)}
        on:moveTransform={(e) =>
          workflowDraft.moveTransform(route._key, e.detail.transformKey, e.detail.direction)}
        on:addAction={() => workflowDraft.addAction(route._key)}
        on:removeAction={(e) => workflowDraft.removeAction(route._key, e.detail.actionKey)}
        on:changeAction={(e) =>
          workflowDraft.updateAction(route._key, e.detail.actionKey, e.detail.action)}
        on:moveAction={(e) =>
          workflowDraft.moveAction(route._key, e.detail.actionKey, e.detail.direction)}
        on:moveRoute={(e) => workflowDraft.moveRoute(route._key, e.detail)}
      />
    {/each}

    <Button variant="secondary" on:click={() => workflowDraft.addRoute()}>
      + Add Route
    </Button>
  </div>

  <div class="toolbar">
    <Button
      on:click={() => {
        showPreview = !showPreview;
        showDryRun = false;
        showGenerate = false;
      }}
      disabled={!$isWorkflowValid}
    >
      {showPreview ? 'Hide Preview' : 'Preview YAML'}
    </Button>
    <Button
      variant="secondary"
      on:click={() => {
        showDryRun = !showDryRun;
        showPreview = false;
        showGenerate = false;
      }}
      disabled={!$isWorkflowValid}
    >
      {showDryRun ? 'Hide Dry Run' : 'Dry Run'}
    </Button>
    <Button
      variant="secondary"
      on:click={() => {
        showGenerate = !showGenerate;
        showPreview = false;
        showDryRun = false;
      }}
    >
      {showGenerate ? 'Hide Generator' : 'Generate with AI'}
    </Button>
    <div class="spacer"></div>
    <Button variant="secondary" on:click={() => workflowDraft.reset()}>
      Reset
    </Button>
  </div>

  <WorkflowDraftLibrary />

  {#if showPreview}
    <div transition:slide={{ duration: 200 }}>
      <WorkflowPreview />
    </div>
  {/if}

  {#if showDryRun}
    <div transition:slide={{ duration: 200 }}>
      <DryRunPanel />
    </div>
  {/if}

  {#if showGenerate}
    <div transition:slide={{ duration: 200 }}>
      <GenerateFromDescription />
    </div>
  {/if}
</div>

<style>
  .builder {
    display: grid;
    gap: 14px;
  }

  .builder-header {
    display: grid;
    gap: 12px;
  }

  .name-version {
    display: grid;
    grid-template-columns: 1fr 120px;
    gap: 12px;
  }

  .field-label {
    display: grid;
    gap: 4px;
    color: var(--color-text-tertiary);
    font-size: 0.85rem;
    font-weight: 600;
  }

  .input {
    padding: 8px 12px;
    border-radius: 10px;
    border: 1px solid var(--color-border-default);
    background: var(--color-bg-input);
    color: var(--color-text-primary);
    outline: none;
    width: 100%;
    box-sizing: border-box;
    transition: var(--transition-all);
  }

  .input::placeholder {
    color: var(--color-text-muted);
  }

  .input:hover:not(:disabled):not(:focus) {
    border-color: var(--color-border-strong);
  }

  .input:focus {
    border-color: var(--color-border-focus);
    box-shadow: var(--shadow-focus);
  }

  .managed-row {
    display: grid;
    grid-template-columns: 1.2fr 1fr 140px 1fr;
    gap: 10px;
    align-items: end;
  }

  .managed-summary {
    padding: 8px 10px;
    border: 1px solid var(--color-border-subtle);
    border-radius: 10px;
    background: var(--color-bg-surface);
    min-height: 70px;
  }

  .managed-label {
    color: var(--color-text-tertiary);
    font-size: 0.75rem;
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 0.05em;
  }

  .managed-value {
    color: var(--color-text-primary);
    font-size: 0.85rem;
    margin-top: 4px;
    line-break: anywhere;
  }

  .managed-name {
    margin-top: 4px;
    font-size: 0.8rem;
  }

  .managed-actions {
    display: flex;
    gap: 8px;
    flex-wrap: wrap;
    align-items: center;
  }

  .loaded-version {
    font-size: 0.85rem;
  }

  .lifecycle-error {
    padding: 8px 10px;
    border-radius: 8px;
    border: 1px solid var(--color-danger-border);
    background: var(--color-danger-bg);
    color: var(--color-danger-text);
    font-size: 0.85rem;
  }

  .routes {
    display: grid;
    gap: 10px;
  }

  .toolbar {
    display: flex;
    gap: 8px;
    flex-wrap: wrap;
    align-items: center;
  }

  .spacer {
    flex: 1;
  }

  .mono {
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  }

  .muted {
    color: var(--color-text-muted);
  }

  @media (max-width: 1080px) {
    .managed-row {
      grid-template-columns: 1fr;
    }
  }

  @media (max-width: 640px) {
    .name-version {
      grid-template-columns: 1fr;
    }

    .toolbar {
      flex-direction: column;
      align-items: stretch;
    }

    .spacer {
      display: none;
    }
  }
</style>
