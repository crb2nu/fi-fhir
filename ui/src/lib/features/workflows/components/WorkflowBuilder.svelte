<script lang="ts">
  import { get } from 'svelte/store';
  import Check from '@lucide/svelte/icons/check';
  import Circle from '@lucide/svelte/icons/circle';
  import CircleAlert from '@lucide/svelte/icons/circle-alert';
  import FileCode from '@lucide/svelte/icons/file-code';
  import Info from '@lucide/svelte/icons/info';
  import Play from '@lucide/svelte/icons/play';
  import Plus from '@lucide/svelte/icons/plus';
  import RotateCcw from '@lucide/svelte/icons/rotate-ccw';
  import TriangleAlert from '@lucide/svelte/icons/triangle-alert';
  import WandSparkles from '@lucide/svelte/icons/wand-sparkles';
  import {
    Badge,
    Button,
    EmptyState,
    Field,
    Icon,
    Input,
    KeyValue,
    Panel,
    Select,
    Table,
    Td,
    Th,
    Tr
  } from '$lib/ui/primitives';
  import RouteEditor from './RouteEditor.svelte';
  import WorkflowPreview from './WorkflowPreview.svelte';
  import DryRunPanel from './DryRunPanel.svelte';
  import GenerateFromDescription from './GenerateFromDescription.svelte';
  import WorkflowDraftLibrary from './WorkflowDraftLibrary.svelte';
  import WorkflowConfirmDialog from './WorkflowConfirmDialog.svelte';
  import { workflowDraft, workflowSavedDrafts, isWorkflowValid } from '../workflowStore';
  import { draftToYaml, listYamlOnlyFields, yamlToDraft } from '../workflowYaml';
  import {
    approvalBlockers,
    blockerSentence,
    compareBlocker,
    createBlocker,
    draftSummary,
    issuesByRoute,
    liveDraftIssues,
    nameFieldError,
    promoteBlocker,
    publishBlockers,
    readinessItems,
    saveBlocker,
    type BuilderState
  } from '../builderValidation';
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
  import { isErrorToasted } from '$lib/graphql/client';

  // Opening the builder no longer makes the draft "live": the untouched
  // default draft is nobody's work, so it shows no errors here and puts
  // nothing in the Problems badge until it differs from the default.

  type ManagedSelection = {
    workflowId: string;
    name: string;
    description: string | null;
    /** The definition's status (`draft`, `archived`); absent from older callers. */
    status?: string | null;
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
  type ConfirmRequest = {
    title: string;
    message: string;
    confirmLabel: string;
    tone: 'primary' | 'danger';
    resolve: (confirmed: boolean) => void;
  };

  export let managedSelection: ManagedSelection | null = null;

  let showPreview = false;
  let showDryRun = false;
  let showGenerate = false;

  let linkedWorkflowId = '';
  let linkedWorkflowName = '';
  let linkedDescription = '';
  let linkedStatus: string | null = null;
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
  let templateError: string | null = null;
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
  let confirmRequest: ConfirmRequest | null = null;

  const ENVIRONMENT_OPTIONS = [
    { value: 'staging', label: 'staging' },
    { value: 'production', label: 'production' }
  ];
  const INVALID_DRAFT_REASON = 'Resolve workflow validation errors first';

  $: selectedVersionRecord = versionHistory.find((version) => version.id === selectedVersionId) ?? null;

  // One snapshot of everything the validation model reads. This is a
  // legacy-mode component, so naming every input here is what makes the
  // derived sentences below re-run when any of them changes.
  $: builderState = {
    draft: $workflowDraft,
    linkedWorkflowId,
    linkedWorkflowName,
    linkedStatus,
    versions: versionHistory,
    selectedVersionId,
    hasUnsavedManagedChanges,
    publishEnvironment,
    approvals: approvalStateByVersion,
    compareFromVersionId,
    compareToVersionId
  } satisfies BuilderState;

  $: draftIssues = liveDraftIssues($workflowDraft);
  $: routeIssues = issuesByRoute(draftIssues);
  $: summaryLine = draftSummary($workflowDraft);
  $: nameError = nameFieldError(builderState);
  $: yamlOnlyFields = listYamlOnlyFields($workflowDraft);
  $: saveReason = saveBlocker(builderState);
  $: createReason = createBlocker(builderState);
  $: compareReason = compareBlocker(builderState);
  $: readiness = {
    publishBlockers: publishBlockers(builderState),
    approvalBlockers: approvalBlockers(builderState),
    items: readinessItems(builderState),
    approvalGranted: approvalStateByVersion.some((item) => item.status === 'approved'),
    approvalPending: approvalStateByVersion.some((item) => item.status === 'pending')
  };
  $: canPublish = readiness.publishBlockers.length === 0;
  $: canApprove = readiness.approvalBlockers.length === 0;
  $: publishReason = blockerSentence('Publish', readiness.publishBlockers);

  function formatTime(ts: string): string {
    const date = new Date(ts);
    if (Number.isNaN(date.getTime())) return ts;
    const pad = (n: number) => String(n).padStart(2, '0');
    return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(
      date.getHours()
    )}:${pad(date.getMinutes())}`;
  }

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

  // Compares the live draft ($workflowDraft, so edits are seen) with the
  // baseline, which is always draftToYaml output so formatting never counts.
  // The draft model keeps every key the YAML carried (nested action config
  // included), so a baseline built this way no longer hides a divergence.
  $: {
    if (!managedBaselineYaml) {
      hasUnsavedManagedChanges = false;
    } else {
      try {
        const currentYaml = draftToYaml($workflowDraft);
        hasUnsavedManagedChanges = currentYaml !== managedBaselineYaml;
      } catch {
        hasUnsavedManagedChanges = false;
      }
    }
  }

  function getSelectedVersionRecord(): WorkflowVersionItem | null {
    return versionHistory.find((version) => version.id === selectedVersionId) ?? null;
  }

  /** Opens the confirmation dialog and resolves with the operator's choice. */
  function askConfirm(request: Omit<ConfirmRequest, 'resolve'>): Promise<boolean> {
    confirmRequest?.resolve(false);
    return new Promise((resolve) => {
      confirmRequest = { ...request, resolve };
    });
  }

  function settleConfirm(confirmed: boolean) {
    const request = confirmRequest;
    confirmRequest = null;
    request?.resolve(confirmed);
  }

  async function confirmPublishTarget(): Promise<boolean> {
    const selectedVersion = getSelectedVersionRecord();
    if (!selectedVersion) return false;
    return askConfirm({
      title: `Publish v${selectedVersion.versionNumber} to ${publishEnvironment}?`,
      message: `${linkedWorkflowName || 'This workflow'} v${selectedVersion.versionNumber} becomes the version ${publishEnvironment} runs. The previous release stays in the history and can be rolled back to from Inventory.`,
      confirmLabel: `Publish to ${publishEnvironment}`,
      tone: publishEnvironment === 'production' ? 'danger' : 'primary'
    });
  }

  async function shouldProceedWithManagedDiscard(actionLabel: string): Promise<boolean> {
    if (!hasUnsavedManagedChanges) return true;
    return askConfirm({
      title: 'Discard unsaved changes?',
      message: `The builder has changes that are not saved as a version. Continue and ${actionLabel}? The changes are lost.`,
      confirmLabel: 'Discard changes',
      tone: 'danger'
    });
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
    // The button is disabled with its reason shown while this is non-null.
    if (compareReason) return;
    compareError = null;
    compareLines = [];
    compareAddedCount = 0;
    compareRemovedCount = 0;

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
      // Inline compareError carries the field context; only toast if the global
      // graphqlFetch net did not already (B4 dedupe).
      if (!isErrorToasted(err)) {
        toasts.error(compareError);
      }
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
    linkedStatus = selection.status ?? null;
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

  async function applyTemplate() {
    templateError = null;
    const template = WORKFLOW_TEMPLATES.find((item) => item.id === selectedTemplateId);
    // The select always holds a template; nothing to do without one.
    if (!template) return;
    if (!(await shouldProceedWithManagedDiscard('replace the current draft with a template'))) {
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
    } catch (err) {
      // The builder shows the new draft on success; a failure stays beside
      // the Template field until the next attempt.
      templateError = err instanceof Error ? err.message : 'The template could not be loaded.';
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
      if (!isErrorToasted(err)) {
        toasts.error(lifecycleError);
      }
    } finally {
      loadingVersionHistory = false;
    }
  }

  async function createManagedDefinition() {
    if (createReason) return;
    const name = get(workflowDraft).name.trim();

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
      linkedStatus = data.createWorkflowDefinition.status;
      versionHistory = [];
      selectedVersionId = '';
      loadedVersionNumber = null;
      managedBaselineYaml = null;
      compareLines = [];
      compareError = null;
      toasts.success(`Created managed workflow: ${linkedWorkflowName}`);
    } catch (err) {
      lifecycleError = err instanceof Error ? err.message : 'Failed to create workflow definition';
      if (!isErrorToasted(err)) {
        toasts.error(lifecycleError);
      }
    } finally {
      creatingDefinition = false;
    }
  }

  async function saveManagedVersion() {
    if (saveReason) return;
    const draft = get(workflowDraft);

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
      if (!isErrorToasted(err)) {
        toasts.error(lifecycleError);
      }
    } finally {
      savingVersion = false;
    }
  }

  async function loadVersionIntoBuilder(versionId: string) {
    if (!versionId) return;
    if (!(await shouldProceedWithManagedDiscard('load a different managed version'))) {
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
      managedBaselineYaml = draftToYaml(parsedDraft);
      await refreshApprovalStateIfNeeded();
      toasts.success(`Loaded v${data.workflowVersion.versionNumber} into builder`);
    } catch (err) {
      lifecycleError = err instanceof Error ? err.message : 'Failed to load workflow version';
      if (!isErrorToasted(err)) {
        toasts.error(lifecycleError);
      }
    } finally {
      loadingVersion = false;
      loadingVersionId = '';
    }
  }

  async function publishManagedVersion() {
    if (!canPublish) return;
    if (!(await confirmPublishTarget())) {
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
      if (!isErrorToasted(err)) {
        toasts.error(lifecycleError);
      }
    } finally {
      publishingVersion = false;
    }
  }

  async function requestApproval() {
    if (!canApprove) return;

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
      if (!isErrorToasted(err)) {
        toasts.error(lifecycleError);
      }
    } finally {
      requestingApproval = false;
    }
  }

  async function refreshVersionHistory() {
    if (!linkedWorkflowId) return;
    await loadVersionHistory(linkedWorkflowId, selectedVersionId || undefined);
    await refreshApprovalStateIfNeeded();
  }

  async function resetDraftWithGuard() {
    if (!(await shouldProceedWithManagedDiscard('reset the current draft'))) {
      return;
    }
    workflowDraft.reset();
    managedBaselineYaml = null;
  }

  async function promoteSnapshotToServer(event: CustomEvent<{ snapshotId: string }>) {
    const snapshotId = event.detail.snapshotId;
    const snapshot = get(workflowSavedDrafts).find((item) => item.id === snapshotId);
    if (!snapshot) {
      lifecycleError = 'That local snapshot no longer exists.';
      return;
    }
    const blocked = promoteBlocker(builderState, snapshot.draft.name);
    if (blocked) {
      // Persistent state, not an event: it stays beside the managed version
      // until the snapshot or the definition changes (.loom/22 B1).
      lifecycleError = `Snapshot "${snapshot.name}" cannot be promoted. ${blocked}`;
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
      if (!isErrorToasted(err)) {
        toasts.error(lifecycleError);
      }
    } finally {
      pushedSnapshotId = null;
    }
  }

  async function promoteImportedYamlToServer(event: CustomEvent<{ yaml: string; draftName: string }>) {
    const blocked = promoteBlocker(builderState, event.detail.draftName);
    if (blocked) {
      lifecycleError = `The imported YAML cannot be promoted. ${blocked}`;
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
      if (!isErrorToasted(err)) {
        toasts.error(lifecycleError);
      }
    } finally {
      promotingImportYaml = false;
    }
  }

  async function unlinkManagedDefinition() {
    if (!(await shouldProceedWithManagedDiscard('unlink the managed definition'))) {
      return;
    }
    linkedWorkflowId = '';
    linkedWorkflowName = '';
    linkedStatus = null;
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
  <div class="builder-main">
    {#if summaryLine}
      <p
        class="summary-line"
        class:is-error={draftIssues.length > 0}
        role="status"
        data-testid="workflow-draft-summary"
      >
        <Icon icon={draftIssues.length > 0 ? CircleAlert : Check} />
        <span>{summaryLine}</span>
      </p>
    {/if}

    <Panel title="Definition">
      <div class="form-grid">
        <Field label="Workflow name" error={nameError}>
          <Input
            mono
            value={$workflowDraft.name}
            placeholder="e.g. adt-routing"
            oninput={(e) =>
              workflowDraft.update((d) => ({
                ...d,
                name: e.currentTarget.value
              }))}
          />
        </Field>
        <Field label="Version">
          <Input
            mono
            value={$workflowDraft.version}
            placeholder="1.0"
            oninput={(e) =>
              workflowDraft.update((d) => ({
                ...d,
                version: e.currentTarget.value
              }))}
          />
        </Field>
        <Field label="Description" class="span-2">
          <Input bind:value={linkedDescription} placeholder="Optional managed workflow description" />
        </Field>
        <Field label="Template" error={templateError}>
          <Select bind:value={selectedTemplateId}>
            {#each WORKFLOW_TEMPLATES as template (template.id)}
              <option value={template.id}>{template.name} · {template.description}</option>
            {/each}
          </Select>
        </Field>
        <Field label="Template name override">
          <div class="inline-control">
            <Input bind:value={templateOverrideName} placeholder="Optional name for the new draft" />
            <Button onclick={() => void applyTemplate()}>Create from template</Button>
          </div>
        </Field>
      </div>
    </Panel>

    <Panel title="Routes" flush>
      {#snippet actions()}
        <Badge mono>{$workflowDraft.routes.length}</Badge>
        <Button variant="ghost" icon={Plus} onclick={() => workflowDraft.addRoute()}>Add route</Button>
      {/snippet}
      {#if $workflowDraft.routes.length === 0}
        <EmptyState align="start" message="No routes. Add a route to match events and run actions." />
      {:else}
        <div class="routes">
          {#each $workflowDraft.routes as route (route._key)}
            <RouteEditor
              {route}
              issues={routeIssues[route._key] ?? []}
              dryRunResult={lastDryRunResult?.routeResults.find((r) => r.routeName === route.name) ?? null}
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
        </div>
      {/if}
    </Panel>

    {#if yamlOnlyFields.length > 0}
      <Panel title="YAML-only fields" data-testid="workflow-yaml-only">
        {#snippet actions()}
          <Badge mono>{yamlOnlyFields.length}</Badge>
        {/snippet}
        <div class="stack">
          <p class="note">
            <Icon icon={Info} />
            <span
              >This draft carries keys the builder has no control for. They are saved exactly as written; change
              them in YAML (Draft library, Import workflow YAML). Nested action values have no effect at runtime:
              the engine reads action settings as flat values.</span
            >
          </p>
          <ul class="yaml-only-list">
            {#each yamlOnlyFields as field, idx (idx)}
              <li>
                <span class="yaml-only-location">{field.location}</span>
                <code class="yaml-only-key">{field.key}</code>
                <span class="yaml-only-reason"
                  >{field.reason === 'nested'
                    ? 'nested value'
                    : field.reason === 'transform'
                      ? 'transform the builder cannot edit'
                      : 'no builder field'}</span
                >
              </li>
            {/each}
          </ul>
        </div>
      </Panel>
    {/if}

    <div class="draft-actions" role="group" aria-label="Draft actions">
      <Button
        icon={FileCode}
        onclick={() => {
          showPreview = !showPreview;
          showDryRun = false;
          showGenerate = false;
        }}
        disabled={!$isWorkflowValid}
        title={$isWorkflowValid ? undefined : INVALID_DRAFT_REASON}
      >
        {showPreview ? 'Hide preview' : 'Preview YAML'}
      </Button>
      <Button
        icon={Play}
        onclick={() => {
          showDryRun = !showDryRun;
          showPreview = false;
          showGenerate = false;
        }}
        disabled={!$isWorkflowValid}
        title={$isWorkflowValid ? undefined : INVALID_DRAFT_REASON}
      >
        {showDryRun ? 'Hide dry run' : 'Dry run'}
      </Button>
      <Button
        icon={WandSparkles}
        onclick={() => {
          showGenerate = !showGenerate;
          showPreview = false;
          showDryRun = false;
        }}
      >
        {showGenerate ? 'Hide generator' : 'Generate with AI'}
      </Button>
      <span class="spacer"></span>
      <Button variant="ghost" icon={RotateCcw} onclick={() => void resetDraftWithGuard()}>Reset</Button>
    </div>

    {#if showPreview}
      <WorkflowPreview />
    {/if}

    {#if showDryRun}
      <DryRunPanel on:result={(e) => handleDryRunResult(e.detail)} />
    {/if}

    {#if showGenerate}
      <GenerateFromDescription />
    {/if}

    <WorkflowDraftLibrary
      pushToServerEnabled={!!linkedWorkflowId && linkedStatus !== 'archived'}
      promoteImportEnabled={!!linkedWorkflowId && linkedStatus !== 'archived'}
      on:pushSnapshot={promoteSnapshotToServer}
      on:promoteImportYaml={promoteImportedYamlToServer}
    />
  </div>

  <aside class="builder-side" aria-label="Managed version">
    <Panel title="Managed version">
      <div class="stack">
        <KeyValue
          items={[
            {
              key: 'Definition',
              value: linkedWorkflowId || 'Not linked',
              mono: !!linkedWorkflowId,
              truncate: true
            },
            { key: 'Name', value: linkedWorkflowName, mono: true, truncate: true },
            { key: 'Status', value: linkedStatus },
            {
              key: 'Loaded',
              value: loadedVersionNumber !== null ? `v${loadedVersionNumber}` : null,
              mono: true
            }
          ]}
        />

        <Field label="Load version">
          <Select
            mono
            value={selectedVersionId}
            onchange={(e) => {
              selectedVersionId = e.currentTarget.value;
              void refreshApprovalStateIfNeeded();
            }}
            disabled={loadingVersionHistory || versionHistory.length === 0}
          >
            {#if versionHistory.length === 0}
              <option value="">No versions</option>
            {:else}
              {#each versionHistory as version (version.id)}
                <option value={version.id}>v{version.versionNumber} · {formatTime(version.createdAt)}</option>
              {/each}
            {/if}
          </Select>
        </Field>

        <div class="form-grid">
          <Field label="Publish env">
            <Select
              options={ENVIRONMENT_OPTIONS}
              value={publishEnvironment}
              onchange={(e) => {
                publishEnvironment = e.currentTarget.value;
                void refreshApprovalStateIfNeeded();
              }}
            />
          </Field>
          <Field label="Notes">
            <Input bind:value={versionNotes} placeholder="Version or approval note" />
          </Field>
        </div>

        <div class="button-wrap">
          <Button
            variant="primary"
            onclick={saveManagedVersion}
            loading={savingVersion}
            disabled={!!saveReason}
            data-testid="workflow-save-version"
          >
            {savingVersion ? 'Saving...' : 'Save version'}
          </Button>
          <Button
            onclick={publishManagedVersion}
            loading={publishingVersion}
            disabled={!canPublish}
            data-testid="workflow-publish-version"
          >
            {publishingVersion ? 'Publishing...' : 'Publish'}
          </Button>
          <Button onclick={requestApproval} loading={requestingApproval} disabled={!canApprove}>
            {requestingApproval
              ? 'Requesting...'
              : publishEnvironment === 'production'
                ? 'Request production approval'
                : 'Request approval'}
          </Button>
          <Button
            onclick={() => loadVersionIntoBuilder(selectedVersionId)}
            loading={loadingVersion}
            disabled={!selectedVersionId || !linkedWorkflowId}
          >
            {loadingVersion ? 'Loading...' : 'Load version'}
          </Button>
          <Button
            onclick={refreshVersionHistory}
            loading={loadingVersionHistory}
            disabled={!linkedWorkflowId}
          >
            {loadingVersionHistory ? 'Refreshing...' : 'Refresh versions'}
          </Button>
          <Button
            onclick={createManagedDefinition}
            loading={creatingDefinition}
            disabled={!!createReason}
            data-testid="workflow-create-definition"
          >
            {creatingDefinition ? 'Creating...' : 'Create definition'}
          </Button>
          <Button variant="ghost" onclick={() => void unlinkManagedDefinition()} disabled={!linkedWorkflowId}
            >Unlink</Button
          >
        </div>

        <ul class="reasons" aria-label="Unavailable actions">
          {#if saveReason}
            <li data-testid="workflow-save-blocked">Save version is unavailable: {saveReason}</li>
          {/if}
          {#if publishReason}
            <li data-testid="workflow-publish-blocked">{publishReason}</li>
          {/if}
          {#if createReason && !linkedWorkflowId}
            <li data-testid="workflow-create-blocked">Create definition is unavailable: {createReason}</li>
          {/if}
        </ul>

        {#if hasUnsavedManagedChanges}
          <p class="note is-warning">
            <Icon icon={TriangleAlert} />
            <span>Unsaved changes. Save a version before loading another, unlinking or resetting.</span>
          </p>
        {/if}
        {#if lifecycleError}
          <p class="note is-error" role="alert">
            <Icon icon={CircleAlert} />
            <span>{lifecycleError}</span>
          </p>
        {/if}
        {#if pushedSnapshotId}
          <p class="note">Promoting local snapshot to a managed version...</p>
        {/if}
        {#if promotingImportYaml}
          <p class="note">Promoting imported YAML to a managed version...</p>
        {/if}
      </div>
    </Panel>

    <Panel title="Readiness">
      <div class="stack">
        <div class="check-group">
          <h3 class="group-title">Publish</h3>
          {#if canPublish}
            <p class="check is-ready">
              <Icon icon={Check} />
              <span>Ready to publish to <span class="text-mono">{publishEnvironment}</span></span>
            </p>
          {:else}
            <ul class="check-list" role="alert">
              {#each readiness.publishBlockers as blocker (blocker)}
                <li class="check"><Icon icon={Circle} /><span>{blocker}</span></li>
              {/each}
            </ul>
          {/if}
        </div>

        <div class="check-group">
          <h3 class="group-title">Approval</h3>
          {#if canApprove}
            <p class="check is-ready">
              <Icon icon={Check} />
              <span>Ready for a production approval request</span>
            </p>
          {:else}
            <ul class="check-list" role="alert">
              {#each readiness.approvalBlockers as blocker (blocker)}
                <li class="check"><Icon icon={Circle} /><span>{blocker}</span></li>
              {/each}
            </ul>
          {/if}
          {#if publishEnvironment === 'production'}
            <p class="note">Production publish is gated: request approval, then publish once it is granted.</p>
            {#if loadingApprovalState}
              <p class="note">Checking approval state...</p>
            {:else if readiness.approvalGranted}
              <p class="check is-ready">
                <Icon icon={Check} />
                <span>Approval granted for the selected version</span>
              </p>
            {:else if readiness.approvalPending}
              <p class="note is-warning">
                <Icon icon={TriangleAlert} />
                <span>Approval request pending review for the selected version</span>
              </p>
            {:else}
              <p class="note is-warning">
                <Icon icon={TriangleAlert} />
                <span>No production approval request for the selected version</span>
              </p>
            {/if}
          {/if}
        </div>

        <div class="check-group">
          <h3 class="group-title">Pre-publish checklist</h3>
          <ul class="check-list">
            {#each readiness.items as item (item.key)}
              <li class="check" class:is-ready={item.ready}>
                <Icon icon={item.ready ? Check : Circle} />
                <span>{item.label}</span>
              </li>
            {/each}
          </ul>
          {#if approvalStateError}
            <p class="note is-error" role="alert">
              <Icon icon={CircleAlert} />
              <span>{approvalStateError}</span>
            </p>
          {/if}
        </div>
      </div>
    </Panel>

    {#if linkedWorkflowId}
      <Panel title="Version history" flush>
        {#snippet actions()}
          <Badge mono>{versionHistory.length}</Badge>
        {/snippet}
        {#if loadingVersionHistory}
          <p class="panel-note">Loading version history...</p>
        {:else if versionHistory.length === 0}
          <p class="panel-note">No saved versions yet.</p>
        {:else}
          <Table label="Version history" class="history-table" layout="fixed">
            {#snippet head()}
              <tr>
                <Th width="88px">Version</Th>
                <Th width="140px">Created</Th>
                <Th>By</Th>
                <Th width="72px">Check</Th>
              </tr>
            {/snippet}
            {#each versionHistory as version (version.id)}
              <Tr
                selectable
                selected={selectedVersionId === version.id}
                onselect={() => selectVersion(version.id)}
              >
                <Td mono>
                  v{version.versionNumber}
                  {#if loadedVersionNumber === version.versionNumber}
                    <Badge>loaded</Badge>
                  {/if}
                </Td>
                <Td mono muted value={formatTime(version.createdAt)} />
                <Td truncate value={version.createdBy || 'unknown'} />
                <Td title={summarizeValidation(version)}>
                  <Badge tone={version.validation.valid ? 'success' : 'danger'}>
                    {version.validation.valid ? 'valid' : 'invalid'}
                  </Badge>
                </Td>
              </Tr>
            {/each}
          </Table>
          {#if selectedVersionRecord}
            {@const version = selectedVersionRecord}
            <div class="version-actions">
              <span class="text-mono version-label">v{version.versionNumber}</span>
              <Button
                onclick={() => loadVersionIntoBuilder(version.id)}
                loading={loadingVersion && loadingVersionId === version.id}
              >
                Load
              </Button>
              <Button
                variant="ghost"
                onclick={() => setCompareFrom(version.id)}
                disabled={compareFromVersionId === version.id}
              >
                Compare from
              </Button>
              <Button
                variant="ghost"
                onclick={() => setCompareTo(version.id)}
                disabled={compareToVersionId === version.id}
              >
                Compare to
              </Button>
            </div>
            {#if version.notes}
              <p class="version-notes">{version.notes}</p>
            {/if}
          {/if}
        {/if}
      </Panel>
    {/if}

    {#if linkedWorkflowId && versionHistory.length > 1}
      <Panel title="Version compare">
        <div class="stack">
          <div class="form-grid">
            <Field label="From">
              <Select mono bind:value={compareFromVersionId}>
                {#each versionHistory as version (version.id)}
                  <option value={version.id}>v{version.versionNumber} · {formatTime(version.createdAt)}</option>
                {/each}
              </Select>
            </Field>
            <Field label="To">
              <Select mono bind:value={compareToVersionId}>
                {#each versionHistory as version (version.id)}
                  <option value={version.id}>v{version.versionNumber} · {formatTime(version.createdAt)}</option>
                {/each}
              </Select>
            </Field>
          </div>
          <div class="button-wrap">
            <Button
              onclick={compareSelectedVersions}
              loading={comparingVersions}
              disabled={!!compareReason}
            >
              {comparingVersions ? 'Comparing...' : 'Compare'}
            </Button>
            {#if compareLines.length > 0}
              <span class="diff-summary text-mono">
                <span class="is-add">+{compareAddedCount}</span>
                <span class="is-remove">−{compareRemovedCount}</span>
                changed lines
              </span>
            {/if}
          </div>

          {#if compareReason}
            <p class="note" data-testid="workflow-compare-blocked">Compare is unavailable: {compareReason}</p>
          {/if}
          {#if compareError}
            <p class="note is-error" role="alert">
              <Icon icon={CircleAlert} />
              <span>{compareError}</span>
            </p>
          {/if}
          {#if compareLines.length > 0}
            <div class="diff text-mono" role="region" aria-label="Version diff">
              {#each compareLines as line, idx (idx)}
                <div class="diff-line" class:is-add={line.kind === 'add'} class:is-remove={line.kind === 'remove'}>
                  <span class="diff-prefix" aria-hidden="true"
                    >{line.kind === 'add' ? '+' : line.kind === 'remove' ? '-' : ' '}</span
                  >
                  <span>{line.text}</span>
                </div>
              {/each}
            </div>
          {/if}
        </div>
      </Panel>
    {/if}
  </aside>
</div>

<WorkflowConfirmDialog
  open={!!confirmRequest}
  title={confirmRequest?.title ?? ''}
  message={confirmRequest?.message ?? ''}
  confirmLabel={confirmRequest?.confirmLabel ?? 'Confirm'}
  tone={confirmRequest?.tone ?? 'primary'}
  onconfirm={() => settleConfirm(true)}
  oncancel={() => settleConfirm(false)}
/>

<style>
  .builder {
    display: grid;
    grid-template-columns: minmax(0, 1fr) 380px;
    align-items: start;
    gap: var(--space-3);
    min-width: 0;
  }

  .summary-line {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    margin: 0;
    font-size: var(--text-ui);
    color: var(--color-text-secondary);
  }

  .summary-line.is-error {
    color: var(--color-danger-text);
  }

  .reasons {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
    margin: 0;
    padding: 0;
    list-style: none;
    font-size: var(--text-xs);
    line-height: var(--leading-snug);
    color: var(--color-text-tertiary);
  }

  .reasons:empty {
    display: none;
  }

  .yaml-only-list {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
    margin: 0;
    padding: 0;
    list-style: none;
    font-size: var(--text-ui);
  }

  .yaml-only-list li {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: var(--space-2);
  }

  .yaml-only-location {
    color: var(--color-text-secondary);
  }

  .yaml-only-key {
    font-family: var(--font-mono);
    color: var(--color-text-primary);
  }

  .yaml-only-reason {
    color: var(--color-text-tertiary);
    font-size: var(--text-xs);
  }

  .builder-main,
  .builder-side {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    min-width: 0;
  }

  .stack {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
  }

  .form-grid {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: var(--space-3);
  }

  .form-grid :global(.span-2) {
    grid-column: 1 / -1;
  }

  .inline-control {
    display: flex;
    gap: var(--space-2);
    min-width: 0;
  }

  .routes {
    display: flex;
    flex-direction: column;
  }

  .draft-actions,
  .button-wrap {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2);
  }

  .spacer {
    flex: 1 1 auto;
  }

  .note {
    display: flex;
    align-items: flex-start;
    gap: var(--space-2);
    margin: 0;
    font-size: var(--text-xs);
    line-height: var(--leading-snug);
    color: var(--color-text-tertiary);
  }

  .note :global(.ui-icon) {
    margin-top: 1px;
  }

  .note.is-warning {
    color: var(--color-warning-text);
  }

  .note.is-error {
    color: var(--color-danger-text);
    overflow-wrap: anywhere;
  }

  .check-group {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  .check-group + .check-group {
    padding-top: var(--space-3);
    border-top: 1px solid var(--color-border-subtle);
  }

  .group-title {
    margin: 0;
    font-size: var(--text-label);
    font-weight: var(--font-semibold);
    letter-spacing: var(--tracking-label);
    text-transform: uppercase;
    color: var(--color-text-tertiary);
  }

  .check-list {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .check {
    display: flex;
    align-items: flex-start;
    gap: var(--space-2);
    margin: 0;
    font-size: var(--text-xs);
    line-height: var(--leading-snug);
    color: var(--color-text-secondary);
  }

  .check :global(.ui-icon) {
    color: var(--color-text-tertiary);
  }

  .check.is-ready :global(.ui-icon) {
    color: var(--color-success-text);
  }

  .panel-note {
    margin: 0;
    padding: var(--space-3);
    font-size: var(--text-xs);
    color: var(--color-text-tertiary);
  }

  .builder-side :global(.history-table) {
    max-height: 240px;
  }

  .version-actions {
    display: flex;
    align-items: center;
    gap: var(--space-1);
    padding: var(--space-2) var(--space-3);
    border-top: 1px solid var(--color-border-subtle);
  }

  .version-label {
    margin-right: auto;
    color: var(--color-text-secondary);
  }

  .version-notes {
    margin: 0;
    padding: 0 var(--space-3) var(--space-3);
    font-size: var(--text-xs);
    color: var(--color-text-secondary);
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }

  .diff-summary {
    display: inline-flex;
    gap: var(--space-2);
    color: var(--color-text-tertiary);
  }

  .diff-summary .is-add {
    color: var(--color-success-text);
  }

  .diff-summary .is-remove {
    color: var(--color-danger-text);
  }

  .diff {
    max-height: 280px;
    overflow: auto;
    padding: var(--space-1) 0;
    border: 1px solid var(--color-border-subtle);
    border-radius: var(--radius-sm);
    background: var(--color-bg-input);
    line-height: var(--leading-snug);
  }

  .diff-line {
    display: grid;
    grid-template-columns: 16px minmax(0, 1fr);
    padding: 0 var(--space-2) 0 var(--space-1);
    white-space: pre-wrap;
    overflow-wrap: anywhere;
    color: var(--color-text-secondary);
  }

  .diff-line.is-add {
    background: var(--color-success-bg);
    color: var(--color-success-text);
  }

  .diff-line.is-remove {
    background: var(--color-danger-bg);
    color: var(--color-danger-text);
  }

  .diff-prefix {
    color: var(--color-text-tertiary);
  }

  @media (max-width: 1100px) {
    .builder {
      grid-template-columns: minmax(0, 1fr);
    }
  }
</style>
