<script lang="ts">
  import { get } from 'svelte/store';
  import { slide } from 'svelte/transition';
  import Panel from '$lib/ui/Panel.svelte';
  import Button from '$lib/ui/Button.svelte';
  import Badge from '$lib/ui/Badge.svelte';
  import RouteEditor from './RouteEditor.svelte';
  import WorkflowPreview from './WorkflowPreview.svelte';
  import DryRunPanel from './DryRunPanel.svelte';
  import GenerateFromDescription from './GenerateFromDescription.svelte';
  import WorkflowDraftLibrary from './WorkflowDraftLibrary.svelte';
  import { workflowDraft, workflowSavedDrafts, isWorkflowValid } from '../workflowStore';
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
  import type { GetWorkflowVersionsQuery, ListWorkflowApprovalRequestsQuery, DryRunResult } from '$lib/gen/graphql';
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
  type DiffLineKind = 'context' | 'add' | 'remove';
  type DiffLine = {
    kind: DiffLineKind;
    text: string;
  };

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
  let loadingVersionId = '';
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
  let managedBaselineYaml: string | null = null;
  let hasUnsavedManagedChanges = false;

  let compareFromVersionId = '';
  let compareToVersionId = '';
  let compareLines: DiffLine[] = [];
  let compareAddedCount = 0;
  let compareRemovedCount = 0;
  let comparingVersions = false;
  let compareError: string | null = null;
  let pushedSnapshotId: string | null = null;
  let promotingImportYaml = false;

  let lastDryRunResult: DryRunResult | null = null;

  // Explanatory tooltips for disabled managed-version controls (UX policy B2/D2:
  // preconditions are surfaced on the disabled control, not via a post-click toast).
  $: saveDisabledReason = !linkedWorkflowId
    ? 'Create or open a managed workflow definition first'
    : !$isWorkflowValid
      ? 'Resolve workflow validation errors before saving'
      : undefined;
  $: compareDisabledReason = !linkedWorkflowId
    ? 'Create or open a managed workflow definition first'
    : !compareFromVersionId || !compareToVersionId
      ? 'Select two versions to compare'
      : undefined;

  function handleDryRunResult(result: DryRunResult | null) {
    lastDryRunResult = result;
  }

  $: if (managedSelection) {
    const nextKey = `${managedSelection.workflowId}:${managedSelection.versionId ?? ''}`;
    if (nextKey !== selectionSyncKey) {
      selectionSyncKey = nextKey;
      void syncFromManagedSelection(managedSelection);
    }
  }

  $: {
    if (!managedBaselineYaml) {
      hasUnsavedManagedChanges = false;
    } else {
      try {
        const currentYaml = draftToYaml(get(workflowDraft));
        hasUnsavedManagedChanges = currentYaml !== managedBaselineYaml;
      } catch {
        hasUnsavedManagedChanges = false;
      }
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

  function getPublishBlockers(): string[] {
    const blockers: string[] = [];
    const selectedVersion = getSelectedVersionRecord();

    if (!linkedWorkflowId) {
      blockers.push('Managed definition is not linked');
    }
    if (!selectedVersionId) {
      blockers.push('No version is selected');
    }
    if (selectedVersion && !selectedVersion.validation.valid) {
      blockers.push('Selected version has validation errors');
    }
    if (hasUnsavedManagedChanges) {
      blockers.push('Builder has unsaved managed changes');
    }
    if (publishEnvironment === 'production' && !hasApprovedProductionRequest()) {
      blockers.push('Production approval is not approved for selected version');
    }

    return blockers;
  }

  function canPublishSelectedVersion(): boolean {
    return getPublishBlockers().length === 0;
  }

  function getApprovalBlockers(): string[] {
    const blockers: string[] = [];
    const selectedVersion = getSelectedVersionRecord();

    if (!linkedWorkflowId) {
      blockers.push('Managed definition is not linked');
    }
    if (!selectedVersionId) {
      blockers.push('No version is selected');
    }
    if (selectedVersion && !selectedVersion.validation.valid) {
      blockers.push('Selected version has validation errors');
    }
    if (hasUnsavedManagedChanges) {
      blockers.push('Builder has unsaved managed changes');
    }
    if (publishEnvironment !== 'production') {
      blockers.push('Set publish environment to production to request approval');
    }
    if (hasApprovedProductionRequest()) {
      blockers.push('Selected version is already approved for production');
    } else if (hasPendingProductionRequest()) {
      blockers.push('Approval request is already pending for selected version');
    }

    return blockers;
  }

  function canRequestApproval(): boolean {
    return getApprovalBlockers().length === 0;
  }

  function confirmPublishTarget(): boolean {
    const selectedVersion = getSelectedVersionRecord();
    if (!selectedVersion) return false;
    if (typeof window === 'undefined' || typeof window.confirm !== 'function') return true;

    return window.confirm(
      `Publish v${selectedVersion.versionNumber} to ${publishEnvironment}?`
    );
  }

  function shouldProceedWithManagedDiscard(actionLabel: string): boolean {
    if (!hasUnsavedManagedChanges) return true;
    if (typeof window === 'undefined' || typeof window.confirm !== 'function') return true;
    return window.confirm(
      `You have unsaved managed changes in the builder. Continue and ${actionLabel}?`
    );
  }

  function computeNaiveLineDiff(fromYaml: string, toYaml: string): {
    lines: DiffLine[];
    added: number;
    removed: number;
  } {
    const fromLines = fromYaml.split('\n');
    const toLines = toYaml.split('\n');
    const max = Math.max(fromLines.length, toLines.length);
    const lines: DiffLine[] = [];
    let added = 0;
    let removed = 0;

    for (let idx = 0; idx < max; idx++) {
      const left = fromLines[idx];
      const right = toLines[idx];
      if (left === right && left !== undefined) {
        lines.push({ kind: 'context', text: left });
        continue;
      }
      if (left !== undefined) {
        lines.push({ kind: 'remove', text: left });
        removed++;
      }
      if (right !== undefined) {
        lines.push({ kind: 'add', text: right });
        added++;
      }
    }

    return { lines, added, removed };
  }

  async function compareSelectedVersions() {
    compareError = null;
    compareLines = [];
    compareAddedCount = 0;
    compareRemovedCount = 0;

    if (!linkedWorkflowId) {
      toasts.error('Create or open a managed workflow definition first');
      return;
    }
    if (!compareFromVersionId || !compareToVersionId) {
      toasts.error('Select two versions to compare');
      return;
    }
    if (compareFromVersionId === compareToVersionId) {
      toasts.error('Choose two different versions to compare');
      return;
    }

    comparingVersions = true;
    try {
      const [fromData, toData] = await Promise.all([
        fetchWorkflowVersionById(compareFromVersionId),
        fetchWorkflowVersionById(compareToVersionId)
      ]);
      if (!fromData.workflowVersion || !toData.workflowVersion) {
        throw new Error('One or both selected versions could not be loaded');
      }

      const diff = computeNaiveLineDiff(fromData.workflowVersion.yaml, toData.workflowVersion.yaml);
      compareLines = diff.lines;
      compareAddedCount = diff.added;
      compareRemovedCount = diff.removed;
    } catch (err) {
      compareError = err instanceof Error ? err.message : 'Failed to compare versions';
      toasts.error(compareError);
    } finally {
      comparingVersions = false;
    }
  }

  function setDefaultCompareVersions() {
    if (versionHistory.length === 0) {
      compareFromVersionId = '';
      compareToVersionId = '';
      return;
    }

    if (!compareToVersionId || !versionHistory.some((v) => v.id === compareToVersionId)) {
      compareToVersionId = selectedVersionId || versionHistory[0]?.id || '';
    }

    if (!compareFromVersionId || !versionHistory.some((v) => v.id === compareFromVersionId)) {
      compareFromVersionId =
        versionHistory.find((v) => v.id !== compareToVersionId)?.id || compareToVersionId;
    }
  }

  function setCompareFrom(versionId: string) {
    compareFromVersionId = versionId;
  }

  function setCompareTo(versionId: string) {
    compareToVersionId = versionId;
  }

  function selectVersion(versionId: string) {
    selectedVersionId = versionId;
    void refreshApprovalStateIfNeeded();
  }

  function summarizeValidation(version: WorkflowVersionItem): string {
    const errorCount = version.validation.errors.length;
    const warningCount = version.validation.warnings.length;
    if (errorCount > 0) {
      return `${errorCount} validation error${errorCount === 1 ? '' : 's'}`;
    }
    if (warningCount > 0) {
      return `${warningCount} validation warning${warningCount === 1 ? '' : 's'}`;
    }
    return 'Validation passed';
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
    compareLines = [];
    compareError = null;
    managedBaselineYaml = null;

    await loadVersionHistory(selection.workflowId, selection.versionId ?? undefined);
    if (selection.versionId) {
      await loadVersionIntoBuilder(selection.versionId);
    }
    await refreshApprovalStateIfNeeded();
  }

  function applyTemplate() {
    if (!shouldProceedWithManagedDiscard('replace the current draft with a template')) {
      return;
    }

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
      managedBaselineYaml = null;

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
      setDefaultCompareVersions();
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
      managedBaselineYaml = null;
      compareLines = [];
      compareError = null;
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
      managedBaselineYaml = yaml;
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
    if (!shouldProceedWithManagedDiscard('load a different managed version')) {
      return;
    }
    loadingVersion = true;
    loadingVersionId = versionId;
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
      managedBaselineYaml = data.workflowVersion.yaml;
      await refreshApprovalStateIfNeeded();
      toasts.success(`Loaded v${data.workflowVersion.versionNumber} into builder`);
    } catch (err) {
      lifecycleError = err instanceof Error ? err.message : 'Failed to load workflow version';
      toasts.error(lifecycleError);
    } finally {
      loadingVersion = false;
      loadingVersionId = '';
    }
  }

  async function publishManagedVersion() {
    const blockers = getPublishBlockers();
    if (blockers.length > 0) {
      toasts.error(`Publish blocked: ${blockers[0]}`);
      return;
    }

    if (!confirmPublishTarget()) {
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
    const blockers = getApprovalBlockers();
    if (blockers.length > 0) {
      toasts.error(`Approval request blocked: ${blockers[0]}`);
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

  function resetDraftWithGuard() {
    if (!shouldProceedWithManagedDiscard('reset the current draft')) {
      return;
    }
    workflowDraft.reset();
    managedBaselineYaml = null;
  }

  async function promoteSnapshotToServer(event: CustomEvent<{ snapshotId: string }>) {
    const snapshotId = event.detail.snapshotId;
    if (!linkedWorkflowId) {
      toasts.error('Create or open a managed workflow definition first');
      return;
    }

    const snapshot = get(workflowSavedDrafts).find((item) => item.id === snapshotId);
    if (!snapshot) {
      toasts.error('Snapshot not found');
      return;
    }

    if (linkedWorkflowName && snapshot.draft.name.trim() !== linkedWorkflowName) {
      toasts.error(
        `Snapshot name "${snapshot.draft.name}" does not match managed definition "${linkedWorkflowName}"`
      );
      return;
    }

    pushedSnapshotId = snapshotId;
    lifecycleError = null;

    try {
      const yaml = draftToYaml(snapshot.draft);
      const data = await saveWorkflowVersion({
        workflowId: linkedWorkflowId,
        yaml,
        notes: `Promoted local snapshot: ${snapshot.name}`
      });
      selectedVersionId = data.saveWorkflowVersion.id;
      loadedVersionNumber = data.saveWorkflowVersion.versionNumber;
      await loadVersionHistory(linkedWorkflowId, data.saveWorkflowVersion.id);
      toasts.success(`Snapshot "${snapshot.name}" promoted as v${data.saveWorkflowVersion.versionNumber}`);
    } catch (err) {
      lifecycleError = err instanceof Error ? err.message : 'Failed to promote snapshot';
      toasts.error(lifecycleError);
    } finally {
      pushedSnapshotId = null;
    }
  }

  async function promoteImportedYamlToServer(event: CustomEvent<{ yaml: string; draftName: string }>) {
    if (!linkedWorkflowId) {
      toasts.error('Create or open a managed workflow definition first');
      return;
    }

    if (linkedWorkflowName && event.detail.draftName && event.detail.draftName !== linkedWorkflowName) {
      toasts.error(
        `Imported YAML name "${event.detail.draftName}" does not match managed definition "${linkedWorkflowName}"`
      );
      return;
    }

    promotingImportYaml = true;
    lifecycleError = null;

    try {
      const data = await saveWorkflowVersion({
        workflowId: linkedWorkflowId,
        yaml: event.detail.yaml,
        notes: 'Promoted imported YAML from Draft Library'
      });
      selectedVersionId = data.saveWorkflowVersion.id;
      loadedVersionNumber = data.saveWorkflowVersion.versionNumber;
      await loadVersionHistory(linkedWorkflowId, data.saveWorkflowVersion.id);
      toasts.success(`Imported YAML promoted as v${data.saveWorkflowVersion.versionNumber}`);
    } catch (err) {
      lifecycleError = err instanceof Error ? err.message : 'Failed to promote imported YAML';
      toasts.error(lifecycleError);
    } finally {
      promotingImportYaml = false;
    }
  }

  function unlinkManagedDefinition() {
    if (!shouldProceedWithManagedDiscard('unlink the managed definition')) {
      return;
    }
    linkedWorkflowId = '';
    linkedWorkflowName = '';
    versionHistory = [];
    selectedVersionId = '';
    loadedVersionNumber = null;
    managedBaselineYaml = null;
    compareFromVersionId = '';
    compareToVersionId = '';
    compareLines = [];
    compareError = null;
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
          title={saveDisabledReason}
        >
          {savingVersion ? 'Saving...' : 'Save Version'}
        </Button>
        <Button
          size="sm"
          variant="secondary"
          on:click={publishManagedVersion}
          loading={publishingVersion}
          disabled={!canPublishSelectedVersion()}
        >
          {publishingVersion ? 'Publishing...' : 'Publish'}
        </Button>
        <Button
          size="sm"
          variant="secondary"
          on:click={requestApproval}
          loading={requestingApproval}
          disabled={!canRequestApproval()}
        >
          {requestingApproval
            ? 'Requesting...'
            : publishEnvironment === 'production'
              ? 'Request Production Approval'
              : 'Request Approval'}
        </Button>
        <Button variant="secondary" size="sm" on:click={unlinkManagedDefinition}>
          Unlink
        </Button>
      </div>

      {#if hasUnsavedManagedChanges}
        <div class="unsaved-hint warning">
          Unsaved managed changes detected. Save a new version before loading another version,
          unlinking, or resetting this draft.
        </div>
      {/if}

      <div class="publish-readiness">
        <div class="managed-label">Publish Readiness</div>
        {#if canPublishSelectedVersion()}
          <div class="checklist-note success">
            Selected version is ready to publish to <span class="mono">{publishEnvironment}</span>.
          </div>
        {:else}
          <div class="publish-blockers" role="alert">
            {#each getPublishBlockers() as blocker (blocker)}
              <div class="publish-blocker-item">{blocker}</div>
            {/each}
          </div>
        {/if}
      </div>

      <div class="approval-readiness">
        <div class="managed-label">Approval Readiness</div>
        {#if canRequestApproval()}
          <div class="checklist-note success">
            Selected version is ready for a production approval request.
          </div>
        {:else}
          <div class="approval-blockers" role="alert">
            {#each getApprovalBlockers() as blocker (blocker)}
              <div class="approval-blocker-item">{blocker}</div>
            {/each}
          </div>
        {/if}
      </div>
      {#if linkedWorkflowId}
        <div class="version-history">
          <div class="version-history-header">
            <div class="managed-label">Version History</div>
            <div class="muted">{versionHistory.length} version{versionHistory.length === 1 ? '' : 's'}</div>
          </div>
          {#if loadingVersionHistory}
            <div class="checklist-note muted">Loading version history...</div>
          {:else if versionHistory.length === 0}
            <div class="checklist-note muted">No saved versions yet.</div>
          {:else}
            <div class="version-history-list">
              {#each versionHistory as version (version.id)}
                <div
                  class="version-card"
                  class:selected={selectedVersionId === version.id}
                  class:loaded={loadedVersionNumber === version.versionNumber}
                >
                  <div class="version-card-main">
                    <div class="version-card-title">
                      <span class="mono">v{version.versionNumber}</span>
                      {#if selectedVersionId === version.id}
                        <Badge variant="primary" size="sm" pill>selected</Badge>
                      {/if}
                      {#if loadedVersionNumber === version.versionNumber}
                        <Badge variant="default" size="sm" pill>loaded</Badge>
                      {/if}
                      <Badge variant={version.validation.valid ? 'success' : 'danger'} size="sm" pill>
                        {version.validation.valid ? 'valid' : 'invalid'}
                      </Badge>
                    </div>
                    <div class="version-card-meta muted">
                      {new Date(version.createdAt).toLocaleString()} · by {version.createdBy || 'unknown'}
                    </div>
                    <div class="version-card-meta muted">{summarizeValidation(version)}</div>
                    {#if version.notes}
                      <div class="version-card-notes">{version.notes}</div>
                    {/if}
                  </div>
                  <div class="version-card-actions">
                    <Button
                      variant="secondary"
                      size="sm"
                      on:click={() => selectVersion(version.id)}
                    >
                      Select
                    </Button>
                    <Button
                      variant="secondary"
                      size="sm"
                      on:click={() => loadVersionIntoBuilder(version.id)}
                      loading={loadingVersion && loadingVersionId === version.id}
                    >
                      Load
                    </Button>
                    <Button
                      variant="secondary"
                      size="sm"
                      on:click={() => setCompareFrom(version.id)}
                      disabled={compareFromVersionId === version.id}
                    >
                      Set Compare From
                    </Button>
                    <Button
                      variant="secondary"
                      size="sm"
                      on:click={() => setCompareTo(version.id)}
                      disabled={compareToVersionId === version.id}
                    >
                      Set Compare To
                    </Button>
                  </div>
                </div>
              {/each}
            </div>
          {/if}
        </div>
      {/if}

      {#if linkedWorkflowId && versionHistory.length > 1}
        <div class="version-compare">
          <div class="managed-label">Version Compare</div>
          <div class="compare-controls">
            <label class="field-label">
              Compare From
              <select class="input" bind:value={compareFromVersionId}>
                {#each versionHistory as version (version.id)}
                  <option value={version.id}>
                    v{version.versionNumber} · {new Date(version.createdAt).toLocaleString()}
                  </option>
                {/each}
              </select>
            </label>
            <label class="field-label">
              Compare To
              <select class="input" bind:value={compareToVersionId}>
                {#each versionHistory as version (version.id)}
                  <option value={version.id}>
                    v{version.versionNumber} · {new Date(version.createdAt).toLocaleString()}
                  </option>
                {/each}
              </select>
            </label>
            <div class="compare-actions">
              <Button
                size="sm"
                variant="secondary"
                on:click={compareSelectedVersions}
                loading={comparingVersions}
                disabled={!linkedWorkflowId || !compareFromVersionId || !compareToVersionId}
                title={compareDisabledReason}
              >
                {comparingVersions ? 'Comparing...' : 'Compare Versions'}
              </Button>
            </div>
          </div>

          {#if compareError}
            <div class="lifecycle-error" role="alert">{compareError}</div>
          {/if}
          {#if compareLines.length > 0}
            <div class="compare-summary muted">
              +{compareAddedCount} / -{compareRemovedCount} changed lines
            </div>
            <div class="compare-diff mono">
              {#each compareLines as line, idx (idx)}
                <div class="diff-line" class:add={line.kind === 'add'} class:remove={line.kind === 'remove'}>
                  <span class="diff-prefix">{line.kind === 'add' ? '+' : line.kind === 'remove' ? '-' : ' '}</span>
                  <span>{line.text}</span>
                </div>
              {/each}
            </div>
          {/if}
        </div>
      {/if}

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
      {#if pushedSnapshotId}
        <div class="checklist-note muted">Promoting local snapshot to managed version...</div>
      {/if}
      {#if promotingImportYaml}
        <div class="checklist-note muted">Promoting imported YAML to managed version...</div>
      {/if}
    </div>
  </Panel>

  <div class="routes">
    {#each $workflowDraft.routes as route (route._key)}
      <RouteEditor
        {route}
        dryRunResult={lastDryRunResult?.routeResults.find(r => r.routeName === route.name) ?? null}
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
    <Button variant="secondary" on:click={resetDraftWithGuard}>
      Reset
    </Button>
  </div>

  <WorkflowDraftLibrary
    pushToServerEnabled={!!linkedWorkflowId}
    promoteImportEnabled={!!linkedWorkflowId}
    on:pushSnapshot={promoteSnapshotToServer}
    on:promoteImportYaml={promoteImportedYamlToServer}
  />

  {#if showPreview}
    <div transition:slide={{ duration: 200 }}>
      <WorkflowPreview />
    </div>
  {/if}

  {#if showDryRun}
    <div transition:slide={{ duration: 200 }}>
      <DryRunPanel on:result={(e) => handleDryRunResult(e.detail)} />
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

  .unsaved-hint {
    padding: 8px 10px;
    border-radius: 8px;
    border: 1px solid rgba(245, 158, 11, 0.35);
    background: rgba(245, 158, 11, 0.14);
    color: rgba(253, 230, 138, 0.95);
    font-size: 0.82rem;
  }

  .publish-readiness {
    display: grid;
    gap: 6px;
    padding: 10px;
    border-radius: 10px;
    border: 1px solid var(--color-border-subtle);
    background: var(--color-bg-surface);
  }

  .publish-blockers {
    display: grid;
    gap: 4px;
    padding: 8px 10px;
    border-radius: 8px;
    border: 1px solid var(--color-danger-border);
    background: var(--color-danger-bg);
    color: var(--color-danger-text);
  }

  .publish-blocker-item {
    font-size: 0.82rem;
    line-height: 1.35;
  }

  .approval-readiness {
    display: grid;
    gap: 6px;
    padding: 10px;
    border-radius: 10px;
    border: 1px solid var(--color-border-subtle);
    background: var(--color-bg-surface);
  }

  .approval-blockers {
    display: grid;
    gap: 4px;
    padding: 8px 10px;
    border-radius: 8px;
    border: 1px solid rgba(245, 158, 11, 0.35);
    background: rgba(245, 158, 11, 0.14);
    color: rgba(253, 230, 138, 0.95);
  }

  .approval-blocker-item {
    font-size: 0.82rem;
    line-height: 1.35;
  }

  .loaded-version {
    font-size: 0.85rem;
  }

  .version-history {
    display: grid;
    gap: 8px;
    padding: 10px;
    border-radius: 10px;
    border: 1px solid var(--color-border-subtle);
    background: var(--color-bg-surface);
  }

  .version-history-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 8px;
  }

  .version-history-list {
    display: grid;
    gap: 8px;
    max-height: 280px;
    overflow: auto;
    padding-right: 2px;
  }

  .version-card {
    display: grid;
    gap: 8px;
    border: 1px solid var(--color-border-subtle);
    border-radius: 8px;
    padding: 8px;
    background: var(--color-bg-input);
  }

  .version-card.selected {
    border-color: var(--color-primary-border);
  }

  .version-card.loaded {
    box-shadow: inset 0 0 0 1px rgba(148, 163, 184, 0.28);
  }

  .version-card-main {
    display: grid;
    gap: 4px;
  }

  .version-card-title {
    display: flex;
    align-items: center;
    gap: 6px;
    flex-wrap: wrap;
    font-size: 0.84rem;
    color: var(--color-text-primary);
  }

  .version-card-meta {
    font-size: 0.8rem;
  }

  .version-card-notes {
    border-left: 2px solid var(--color-border-default);
    padding-left: 8px;
    color: var(--color-text-secondary);
    font-size: 0.82rem;
    line-height: 1.4;
    white-space: pre-wrap;
    word-break: break-word;
  }

  .version-card-actions {
    display: flex;
    gap: 6px;
    flex-wrap: wrap;
  }

  .version-compare {
    display: grid;
    gap: 8px;
    padding: 10px;
    border-radius: 10px;
    border: 1px solid var(--color-border-subtle);
    background: var(--color-bg-surface);
  }

  .compare-controls {
    display: grid;
    grid-template-columns: 1fr 1fr auto;
    gap: 10px;
    align-items: end;
  }

  .compare-actions {
    padding-bottom: 2px;
  }

  .compare-summary {
    font-size: 0.82rem;
  }

  .compare-diff {
    max-height: 220px;
    overflow: auto;
    border-radius: 8px;
    border: 1px solid var(--color-border-subtle);
    background: var(--color-bg-input);
    padding: 8px;
    display: grid;
    gap: 2px;
    font-size: 0.78rem;
    line-height: 1.35;
  }

  .diff-line {
    display: grid;
    grid-template-columns: 14px 1fr;
    gap: 6px;
    white-space: pre-wrap;
    word-break: break-word;
    color: var(--color-text-secondary);
  }

  .diff-line.add {
    color: rgba(187, 247, 208, 0.95);
  }

  .diff-line.remove {
    color: rgba(254, 202, 202, 0.95);
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

    .compare-controls {
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
