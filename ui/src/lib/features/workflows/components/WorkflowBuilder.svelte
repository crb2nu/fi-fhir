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
    fetchWorkflowApprovalRequests,
    fetchWorkflowVersionById,
    fetchWorkflowVersions,
    publishWorkflowVersion,
    requestWorkflowApproval,
    saveWorkflowVersion
  } from '../workflowApi';
  import { WORKFLOW_TEMPLATES } from '../workflowTemplates';
  import type { GetWorkflowVersionsQuery, ListWorkflowApprovalRequestsQuery } from '$lib/gen/graphql';
  import { toasts } from '$lib/ui/toastStore';

  type ManagedSelection = {
    workflowId: string;
    name: string;
    description: string | null;
    versionId: string | null;
    versionNumber: number | null;
  };
  type WorkflowVersionItem = GetWorkflowVersionsQuery['workflowVersions'][number];
  type WorkflowApprovalItem = ListWorkflowApprovalRequestsQuery['workflowApprovalRequests'][number];

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
  let selectedTemplateId = WORKFLOW_TEMPLATES[0]?.id ?? '';
  let templateOverrideName = '';
  let approvalStateByVersion: WorkflowApprovalItem[] = [];
  let loadingApprovalState = false;
  let approvalStateError: string | null = null;

  $: if (managedSelection) {
    const nextKey = `${managedSelection.workflowId}:${managedSelection.versionId ?? ''}`;
    if (nextKey !== selectionSyncKey) {
      selectionSyncKey = nextKey;
      void syncFromManagedSelection(managedSelection);
    }
  }

  function getSelectedVersionRecord(): WorkflowVersionItem | null {
    return versionHistory.find((version) => version.id === selectedVersionId) ?? null;
  }

  function hasApprovedProductionRequest(): boolean {
    return approvalStateByVersion.some((item) => item.status === 'approved');
  }

  function hasPendingProductionRequest(): boolean {
    return approvalStateByVersion.some((item) => item.status === 'pending');
  }

  function getReadinessItems(workflowValid: boolean): Array<{ key: string; label: string; ready: boolean }> {
    const selectedVersion = getSelectedVersionRecord();
    return [
      {
        key: 'definition',
        label: 'Managed definition linked',
        ready: !!linkedWorkflowId
      },
      {
        key: 'version-selected',
        label: 'Version selected',
        ready: !!selectedVersionId
      },
      {
        key: 'draft-valid',
        label: 'Current draft is structurally valid',
        ready: workflowValid
      },
      {
        key: 'version-valid',
        label: 'Selected server version passed validation',
        ready: !!selectedVersion?.validation.valid
      },
      {
        key: 'production-approval',
        label:
          publishEnvironment === 'production'
            ? 'Production approval is approved'
            : 'Production approval not required for non-production publish',
        ready: publishEnvironment === 'production' ? hasApprovedProductionRequest() : true
      }
    ];
  }

  async function refreshApprovalStateIfNeeded() {
    if (publishEnvironment === 'production' && linkedWorkflowId && selectedVersionId) {
      await loadApprovalState();
      return;
    }
    approvalStateByVersion = [];
    approvalStateError = null;
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
    await refreshApprovalStateIfNeeded();
  }

  function applyTemplate() {
    const template = WORKFLOW_TEMPLATES.find((item) => item.id === selectedTemplateId);
    if (!template) {
      toasts.error('Select a workflow template first');
      return;
    }

    try {
      const draft = yamlToDraft(template.yaml);
      if (templateOverrideName.trim()) {
        draft.name = templateOverrideName.trim();
      }
      workflowDraft.loadDraft(draft);

      if (!linkedWorkflowId) {
        linkedWorkflowName = draft.name;
      }

      toasts.success(`Loaded template: ${template.name}`);
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to load template';
      toasts.error(message);
    }
  }

  async function loadApprovalState() {
    if (!linkedWorkflowId || !selectedVersionId || publishEnvironment !== 'production') {
      approvalStateByVersion = [];
      approvalStateError = null;
      return;
    }

    loadingApprovalState = true;
    approvalStateError = null;
    try {
      const data = await fetchWorkflowApprovalRequests({
        filter: {
          workflowId: linkedWorkflowId,
          environment: publishEnvironment,
          status: null
        },
        paging: {
          limit: 200,
          offset: 0
        }
      });
      approvalStateByVersion = data.workflowApprovalRequests.filter(
        (item) => item.targetVersionId === selectedVersionId
      );
    } catch (err) {
      approvalStateError =
        err instanceof Error ? err.message : 'Failed to load approval request state';
      approvalStateByVersion = [];
    } finally {
      loadingApprovalState = false;
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
      await refreshApprovalStateIfNeeded();
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
      await refreshApprovalStateIfNeeded();
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
    if (publishEnvironment === 'production' && !hasApprovedProductionRequest()) {
      toasts.error('Production publish is blocked until an approval request is approved');
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
      await loadApprovalState();
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
    await refreshApprovalStateIfNeeded();
  }

  function unlinkManagedDefinition() {
    linkedWorkflowId = '';
    linkedWorkflowName = '';
    versionHistory = [];
    selectedVersionId = '';
    loadedVersionNumber = null;
    lifecycleError = null;
    selectionSyncKey = '';
    approvalStateByVersion = [];
    approvalStateError = null;
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

      <div class="template-row">
        <label class="field-label">
          Workflow Template
          <select class="input" bind:value={selectedTemplateId}>
            {#each WORKFLOW_TEMPLATES as template (template.id)}
              <option value={template.id}>{template.name} - {template.description}</option>
            {/each}
          </select>
        </label>
        <label class="field-label">
          Template Name Override
          <input
            type="text"
            class="input"
            bind:value={templateOverrideName}
            placeholder="Optional name override before loading template"
          />
        </label>
        <div class="template-actions">
          <Button variant="secondary" size="sm" on:click={applyTemplate}>Create from Template</Button>
        </div>
      </div>

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
            on:change={() => {
              void refreshApprovalStateIfNeeded();
            }}
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
          <select
            class="input"
            bind:value={publishEnvironment}
            on:change={() => {
              void refreshApprovalStateIfNeeded();
            }}
          >
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

      {#if publishEnvironment === 'production'}
        <div class="gate-hint">
          Production publish is gated. Request approval first, then publish after approval is
          granted.
        </div>
        {#if loadingApprovalState}
          <div class="checklist-note muted">Checking approval state...</div>
        {:else if hasApprovedProductionRequest()}
          <div class="checklist-note success">
            Approval is granted for the selected version in production.
          </div>
        {:else if hasPendingProductionRequest()}
          <div class="checklist-note warning">
            Approval request is pending review for the selected version.
          </div>
        {:else}
          <div class="checklist-note warning">
            No approval request found for the selected version in production.
          </div>
        {/if}
      {/if}

      <div class="readiness">
        <div class="managed-label">Pre-Publish Readiness Checklist</div>
        <div class="readiness-list">
          {#each getReadinessItems($isWorkflowValid) as item (item.key)}
            <div class="readiness-item" class:ready={item.ready}>
              <span class="readiness-icon" aria-hidden="true">{item.ready ? '✓' : '○'}</span>
              <span>{item.label}</span>
            </div>
          {/each}
        </div>
        {#if approvalStateError}
          <div class="lifecycle-error" role="alert">{approvalStateError}</div>
        {/if}
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

  .template-row {
    display: grid;
    grid-template-columns: 1.2fr 1fr auto;
    gap: 10px;
    align-items: end;
  }

  .template-actions {
    display: flex;
    align-items: center;
    gap: 8px;
    padding-bottom: 2px;
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

  .readiness {
    display: grid;
    gap: 6px;
    padding: 10px;
    border-radius: 10px;
    border: 1px solid var(--color-border-subtle);
    background: var(--color-bg-surface);
  }

  .readiness-list {
    display: grid;
    gap: 5px;
  }

  .readiness-item {
    display: flex;
    align-items: center;
    gap: 8px;
    color: var(--color-text-secondary);
    font-size: 0.84rem;
  }

  .readiness-item.ready {
    color: rgba(187, 247, 208, 0.95);
  }

  .readiness-icon {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 16px;
    min-width: 16px;
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  }

  .gate-hint {
    padding: 8px 10px;
    border-radius: 8px;
    border: 1px solid rgba(245, 158, 11, 0.35);
    background: rgba(245, 158, 11, 0.14);
    color: rgba(253, 230, 138, 0.95);
    font-size: 0.82rem;
  }

  .checklist-note {
    font-size: 0.82rem;
    padding: 6px 0;
  }

  .checklist-note.success {
    color: rgba(187, 247, 208, 0.95);
  }

  .checklist-note.warning {
    color: rgba(253, 230, 138, 0.95);
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
    .template-row,
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
