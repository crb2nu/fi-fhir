<script lang="ts">
  import { createEventDispatcher, onMount } from 'svelte';
  import RefreshCw from '@lucide/svelte/icons/refresh-cw';
  import CircleAlert from '@lucide/svelte/icons/circle-alert';
  import Inbox from '@lucide/svelte/icons/inbox';
  import MousePointerClick from '@lucide/svelte/icons/mouse-pointer-click';
  import {
    Badge,
    Button,
    EmptyState,
    Field,
    Icon,
    KeyValue,
    Select,
    Table,
    Td,
    Textarea,
    Th,
    Tr
  } from '$lib/ui/primitives';
  import type { BadgeTone, KeyValueItem } from '$lib/ui/primitives';
  import {
    fetchWorkflowDefinitions,
    fetchWorkflowVersions,
    publishWorkflowVersion,
    rollbackWorkflowVersion,
    triggerWorkflow
  } from '../workflowApi';
  import { validateEventPayload } from '../eventPayload';
  import type {
    GetWorkflowVersionsQuery,
    ListWorkflowDefinitionsQuery,
    TriggerWorkflowMutation
  } from '$lib/gen/graphql';
  import { toasts } from '$lib/ui/toastStore';
  import { isErrorToasted } from '$lib/graphql/client';

  type WorkflowItem = ListWorkflowDefinitionsQuery['workflowDefinitions'][number];
  type WorkflowVersionItem = GetWorkflowVersionsQuery['workflowVersions'][number];
  type TriggerResult = TriggerWorkflowMutation['triggerWorkflow'];
  type OpenBuilderPayload = {
    workflowId: string;
    name: string;
    description: string | null;
    versionId: string | null;
    versionNumber: number | null;
  };

  const dispatch = createEventDispatcher<{
    openBuilder: OpenBuilderPayload;
    openMonitor: { workflowName: string };
  }>();

  const ENVIRONMENT_OPTIONS = [
    { value: 'staging', label: 'staging' },
    { value: 'production', label: 'production' }
  ];

  let workflows: WorkflowItem[] = [];
  let loading = true;
  let error: string | null = null;

  let runningWorkflowName: string | null = null;
  let selectedWorkflowId: string | null = null;

  let eventJsonByWorkflow: Record<string, string> = {};
  let runResultByWorkflow: Record<string, TriggerResult | undefined> = {};
  let runErrorByWorkflow: Record<string, string | undefined> = {};

  let versionsByWorkflowId: Record<string, WorkflowVersionItem[] | undefined> = {};
  let loadingVersionsByWorkflowId: Record<string, boolean> = {};
  let versionErrorByWorkflowId: Record<string, string | undefined> = {};

  let selectedVersionByWorkflowId: Record<string, string | undefined> = {};
  let selectedEnvByWorkflowId: Record<string, string | undefined> = {};
  let publishingByWorkflowId: Record<string, boolean> = {};
  let rollingBackByWorkflowId: Record<string, boolean> = {};

  $: selected = workflows.find((wf) => wf.id === selectedWorkflowId) ?? null;

  onMount(() => {
    void loadWorkflows();
  });

  async function loadWorkflows() {
    loading = true;
    error = null;
    try {
      const data = await fetchWorkflowDefinitions({
        paging: {
          limit: 100,
          offset: 0
        }
      });
      workflows = data.workflowDefinitions;
      // Keep the details pane populated: the first record is selected until
      // the operator picks another one.
      const first = workflows[0];
      if (first && !workflows.some((wf) => wf.id === selectedWorkflowId)) {
        selectWorkflow(first);
      }
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to load workflows';
    } finally {
      loading = false;
    }
  }

  async function ensureWorkflowVersions(workflow: WorkflowItem) {
    if (loadingVersionsByWorkflowId[workflow.id]) return;
    if (versionsByWorkflowId[workflow.id]) return;

    loadingVersionsByWorkflowId = {
      ...loadingVersionsByWorkflowId,
      [workflow.id]: true
    };
    versionErrorByWorkflowId = {
      ...versionErrorByWorkflowId,
      [workflow.id]: undefined
    };

    try {
      const data = await fetchWorkflowVersions(workflow.id, {
        limit: 100,
        offset: 0
      });
      versionsByWorkflowId = {
        ...versionsByWorkflowId,
        [workflow.id]: data.workflowVersions
      };

      const publishedProd = getPublishedVersionId(workflow, 'production');
      const preferredVersionId =
        publishedProd ?? workflow.latestVersion?.id ?? data.workflowVersions[0]?.id;
      selectedVersionByWorkflowId = {
        ...selectedVersionByWorkflowId,
        [workflow.id]: preferredVersionId
      };
      selectedEnvByWorkflowId = {
        ...selectedEnvByWorkflowId,
        [workflow.id]: selectedEnvByWorkflowId[workflow.id] ?? 'staging'
      };
    } catch (err) {
      versionErrorByWorkflowId = {
        ...versionErrorByWorkflowId,
        [workflow.id]: err instanceof Error ? err.message : 'Failed to load versions'
      };
    } finally {
      loadingVersionsByWorkflowId = {
        ...loadingVersionsByWorkflowId,
        [workflow.id]: false
      };
    }
  }

  async function refreshWorkflowVersions(workflow: WorkflowItem) {
    versionsByWorkflowId = {
      ...versionsByWorkflowId,
      [workflow.id]: undefined
    };
    await ensureWorkflowVersions(workflow);
  }

  function getDefaultEventJson(workflowName: string): string {
    const normalized = workflowName.toLowerCase();
    let type = 'PATIENT_ADMIT';

    if (normalized.includes('lab') || normalized.includes('oru')) {
      type = 'LAB_RESULT';
    } else if (normalized.includes('discharge')) {
      type = 'PATIENT_DISCHARGE';
    } else if (normalized.includes('appoint') || normalized.includes('siu')) {
      type = 'APPOINTMENT_SCHEDULED';
    }

    return JSON.stringify(
      {
        type,
        source: 'ui-manual',
        id: `manual-${Date.now()}`,
        timestamp: new Date().toISOString()
      },
      null,
      2
    );
  }

  function selectWorkflow(workflow: WorkflowItem) {
    selectedWorkflowId = workflow.id;

    if (!eventJsonByWorkflow[workflow.name]) {
      eventJsonByWorkflow = {
        ...eventJsonByWorkflow,
        [workflow.name]: getDefaultEventJson(workflow.name)
      };
    }

    if (!selectedEnvByWorkflowId[workflow.id]) {
      selectedEnvByWorkflowId = {
        ...selectedEnvByWorkflowId,
        [workflow.id]: 'staging'
      };
    }

    void ensureWorkflowVersions(workflow);
  }

  function setSampleEvent(workflowName: string) {
    eventJsonByWorkflow = {
      ...eventJsonByWorkflow,
      [workflowName]: getDefaultEventJson(workflowName)
    };
  }

  function parsePublishedVersions(raw: unknown): Record<string, string> {
    if (!raw || typeof raw !== 'object') return {};

    const asRecord = raw as Record<string, unknown>;
    const out: Record<string, string> = {};
    for (const [environment, value] of Object.entries(asRecord)) {
      if (typeof value !== 'string' || !value.trim()) continue;
      out[environment] = value;
    }
    return out;
  }

  function getPublishedVersionId(workflow: WorkflowItem, environment: string): string | undefined {
    const published = parsePublishedVersions(workflow.publishedVersionsByEnv);
    return published[environment];
  }

  function publishedEnvironments(workflow: WorkflowItem): string[] {
    return Object.keys(parsePublishedVersions(workflow.publishedVersionsByEnv));
  }

  function summarizePublishedVersions(workflow: WorkflowItem): string {
    const published = parsePublishedVersions(workflow.publishedVersionsByEnv);
    const entries = Object.entries(published);
    if (entries.length === 0) return 'No published environments';
    return entries.map(([env, versionId]) => `${env}: ${versionId.slice(0, 12)}`).join(' · ');
  }

  /** State describes the lifecycle: archived, published somewhere, or a draft. */
  function workflowState(workflow: WorkflowItem): { label: string; tone: BadgeTone } {
    if (workflow.status === 'archived') return { label: 'archived', tone: 'danger' };
    if (publishedEnvironments(workflow).length > 0) return { label: 'published', tone: 'success' };
    return { label: workflow.status || 'draft', tone: 'neutral' };
  }

  function formatTime(ts: string | null): string {
    if (!ts) return 'Never';
    const date = new Date(ts);
    if (Number.isNaN(date.getTime())) return ts;
    const pad = (n: number) => String(n).padStart(2, '0');
    return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(
      date.getHours()
    )}:${pad(date.getMinutes())}`;
  }

  function resolveEnvironment(
    workflowId: string,
    envs: Record<string, string | undefined>
  ): string {
    return envs[workflowId] ?? 'staging';
  }

  function resolveVersionId(
    workflow: WorkflowItem,
    selectedVersions: Record<string, string | undefined>,
    versions: Record<string, WorkflowVersionItem[] | undefined>
  ): string | undefined {
    return (
      selectedVersions[workflow.id] ??
      getPublishedVersionId(workflow, 'production') ??
      workflow.latestVersion?.id ??
      versions[workflow.id]?.[0]?.id
    );
  }

  function getSelectedEnvironment(workflowId: string): string {
    return resolveEnvironment(workflowId, selectedEnvByWorkflowId);
  }

  function getSelectedVersionId(workflow: WorkflowItem): string | undefined {
    return resolveVersionId(workflow, selectedVersionByWorkflowId, versionsByWorkflowId);
  }

  function detailItems(workflow: WorkflowItem): KeyValueItem[] {
    const published = parsePublishedVersions(workflow.publishedVersionsByEnv);
    const publishedItems: KeyValueItem[] = Object.entries(published).map(([env, versionId]) => ({
      key: `Published · ${env}`,
      value: versionId,
      mono: true,
      truncate: true
    }));
    return [
      { key: 'Id', value: workflow.id, mono: true, truncate: true },
      { key: 'Description', value: workflow.description },
      {
        key: 'Latest version',
        value: workflow.latestVersion ? `v${workflow.latestVersion.versionNumber}` : 'No versions',
        mono: Boolean(workflow.latestVersion)
      },
      { key: 'Created by', value: workflow.latestVersion?.createdBy },
      { key: 'Updated', value: formatTime(workflow.updatedAt), mono: true },
      ...(publishedItems.length > 0
        ? publishedItems
        : [{ key: 'Published', value: 'Not published' } satisfies KeyValueItem])
    ];
  }

  function resultItems(result: TriggerResult): KeyValueItem[] {
    const items: KeyValueItem[] = [
      { key: 'Matched routes', value: result.routesMatched, mono: true },
      { key: 'Executed actions', value: result.actionsExecuted, mono: true },
      { key: 'Duration', value: `${result.duration.toFixed(2)} ms`, mono: true }
    ];
    if (result.runId) items.push({ key: 'Run id', value: result.runId, mono: true, truncate: true });
    if (result.environment) items.push({ key: 'Environment', value: result.environment, mono: true });
    if (result.versionId) {
      items.push({ key: 'Version', value: result.versionId, mono: true, truncate: true });
    }
    return items;
  }

  function emitOpenBuilder(workflow: WorkflowItem) {
    dispatch('openBuilder', {
      workflowId: workflow.id,
      name: workflow.name,
      description: workflow.description ?? null,
      versionId: getSelectedVersionId(workflow) ?? null,
      versionNumber: workflow.latestVersion?.versionNumber ?? null
    });
  }

  function emitOpenMonitor(workflowName: string) {
    dispatch('openMonitor', { workflowName });
  }

  async function runWorkflow(workflowId: string, workflowName: string) {
    if (runningWorkflowName) return;

    // Persistent payload validation belongs inline at the field, not in a
    // transient toast (.loom/22 B1/B4): show the message in runErrorByWorkflow,
    // which renders as an inline `role="alert"` next to the Event JSON textarea.
    const payload = validateEventPayload(eventJsonByWorkflow[workflowName] ?? '');
    if (!payload.ok) {
      runErrorByWorkflow = {
        ...runErrorByWorkflow,
        [workflowName]: payload.message
      };
      return;
    }
    const parsedEvent = payload.value;

    runningWorkflowName = workflowName;
    runErrorByWorkflow = { ...runErrorByWorkflow, [workflowName]: undefined };

    try {
      const data = await triggerWorkflow(workflowName, parsedEvent, {
        environment: getSelectedEnvironment(workflowId)
      });
      runResultByWorkflow = {
        ...runResultByWorkflow,
        [workflowName]: data.triggerWorkflow
      };

      if (data.triggerWorkflow.errors.length > 0) {
        toasts.error(
          `Workflow ran with ${data.triggerWorkflow.errors.length} error${data.triggerWorkflow.errors.length === 1 ? '' : 's'}`
        );
      } else {
        toasts.success(
          `Workflow executed: ${data.triggerWorkflow.actionsExecuted} action${data.triggerWorkflow.actionsExecuted === 1 ? '' : 's'}`
        );
      }
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'Failed to trigger workflow';
      runErrorByWorkflow = {
        ...runErrorByWorkflow,
        [workflowName]: msg
      };
      // Inline runErrorByWorkflow carries the field context; only toast if the
      // global graphqlFetch net did not already (B4 dedupe).
      if (!isErrorToasted(err)) {
        toasts.error(msg);
      }
    } finally {
      runningWorkflowName = null;
    }
  }

  async function publishSelectedVersion(workflow: WorkflowItem) {
    const workflowId = workflow.id;
    const versionId = getSelectedVersionId(workflow);
    if (!versionId) {
      toasts.error('Select a version to publish');
      return;
    }
    const environment = getSelectedEnvironment(workflowId);
    publishingByWorkflowId = { ...publishingByWorkflowId, [workflowId]: true };

    try {
      await publishWorkflowVersion({
        workflowId,
        versionId,
        environment
      });
      await loadWorkflows();
      await refreshWorkflowVersions(workflow);
      toasts.success(`Published ${workflow.name} to ${environment}`);
    } catch (err) {
      // Global graphqlFetch net already toasts graphql failures (B4 dedupe).
      if (!isErrorToasted(err)) {
        toasts.error(err instanceof Error ? err.message : 'Failed to publish workflow');
      }
    } finally {
      publishingByWorkflowId = {
        ...publishingByWorkflowId,
        [workflowId]: false
      };
    }
  }

  async function rollbackToSelectedVersion(workflow: WorkflowItem) {
    const workflowId = workflow.id;
    const versionId = getSelectedVersionId(workflow);
    if (!versionId) {
      toasts.error('Select a version to roll back to');
      return;
    }
    const environment = getSelectedEnvironment(workflowId);
    rollingBackByWorkflowId = {
      ...rollingBackByWorkflowId,
      [workflowId]: true
    };

    try {
      await rollbackWorkflowVersion({
        workflowId,
        targetVersionId: versionId,
        environment
      });
      await loadWorkflows();
      await refreshWorkflowVersions(workflow);
      toasts.success(`Rolled back ${workflow.name} in ${environment}`);
    } catch (err) {
      // Global graphqlFetch net already toasts graphql failures (B4 dedupe).
      if (!isErrorToasted(err)) {
        toasts.error(err instanceof Error ? err.message : 'Failed to roll back workflow');
      }
    } finally {
      rollingBackByWorkflowId = {
        ...rollingBackByWorkflowId,
        [workflowId]: false
      };
    }
  }
</script>

<div class="inventory">
  <div class="filters">
    <Button variant="ghost" icon={RefreshCw} onclick={loadWorkflows} disabled={loading}>
      {loading ? 'Refreshing...' : 'Refresh'}
    </Button>
    <span class="count text-mono">
      {workflows.length} workflow{workflows.length === 1 ? '' : 's'}
    </span>
  </div>

  {#if loading && workflows.length === 0}
    <p class="status-line" role="status">Loading workflows...</p>
  {:else if error}
    <EmptyState icon={CircleAlert} actionLabel="Retry" onaction={loadWorkflows}>
      Failed to load workflows: {error}
    </EmptyState>
  {:else if workflows.length === 0}
    <EmptyState icon={Inbox} message="No managed workflows. Create a definition in Design." />
  {:else}
    <div class="split">
      <Table label="Managed workflows" class="split-table" layout="fixed">
        {#snippet head()}
          <tr>
            <Th>Name</Th>
            <Th width="88px">Version</Th>
            <Th width="120px">State</Th>
            <Th width="200px">Published</Th>
            <Th width="150px">Updated</Th>
          </tr>
        {/snippet}
        {#each workflows as wf (wf.id)}
          {@const state = workflowState(wf)}
          {@const envs = publishedEnvironments(wf)}
          <Tr
            selectable
            selected={wf.id === selectedWorkflowId}
            onselect={() => selectWorkflow(wf)}
          >
            <Td mono truncate value={wf.name} />
            <Td mono muted={!wf.latestVersion}>
              {wf.latestVersion ? `v${wf.latestVersion.versionNumber}` : '—'}
            </Td>
            <Td>
              <Badge tone={state.tone} dot={state.tone !== 'neutral'}>{state.label}</Badge>
            </Td>
            <Td muted truncate title={summarizePublishedVersions(wf)}>
              {envs.length > 0 ? envs.join(', ') : '—'}
            </Td>
            <Td mono muted value={formatTime(wf.updatedAt)} />
          </Tr>
        {/each}
      </Table>

      <aside class="details" aria-label="Selected workflow">
        {#if selected}
          {@const wf = selected}
          {@const state = workflowState(wf)}
          {@const isRunning = runningWorkflowName === wf.name}
          {@const runResult = runResultByWorkflow[wf.name]}
          {@const runError = runErrorByWorkflow[wf.name]}
          {@const workflowVersions = versionsByWorkflowId[wf.id] ?? []}
          {@const loadingVersions = !!loadingVersionsByWorkflowId[wf.id]}
          {@const selectedEnv = resolveEnvironment(wf.id, selectedEnvByWorkflowId)}
          {@const selectedVersion = resolveVersionId(
            wf,
            selectedVersionByWorkflowId,
            versionsByWorkflowId
          )}
          {@const selectedPublishedVersion = getPublishedVersionId(wf, selectedEnv)}

          <div class="details-head">
            <span class="details-title text-mono" title={wf.name}>{wf.name}</span>
            <Badge tone={state.tone} dot={state.tone !== 'neutral'}>{state.label}</Badge>
          </div>
          <div class="details-actions">
            <Button onclick={() => emitOpenBuilder(wf)}>Open in Design</Button>
            <Button variant="ghost" onclick={() => emitOpenMonitor(wf.name)}>View runs</Button>
          </div>

          <KeyValue items={detailItems(wf)} />

          <section class="details-section" aria-labelledby="inventory-publish-title">
            <h3 id="inventory-publish-title" class="section-title">Publish</h3>
            <div class="form-grid">
              <Field label="Environment">
                <Select
                  options={ENVIRONMENT_OPTIONS}
                  value={selectedEnv}
                  onchange={(e) => {
                    selectedEnvByWorkflowId = {
                      ...selectedEnvByWorkflowId,
                      [wf.id]: e.currentTarget.value
                    };
                  }}
                />
              </Field>
              <Field label="Version">
                <Select
                  mono
                  value={selectedVersion ?? ''}
                  disabled={loadingVersions}
                  onchange={(e) => {
                    selectedVersionByWorkflowId = {
                      ...selectedVersionByWorkflowId,
                      [wf.id]: e.currentTarget.value
                    };
                  }}
                >
                  {#if workflowVersions.length === 0}
                    <option value="">No saved versions</option>
                  {:else}
                    {#each workflowVersions as version (version.id)}
                      <option value={version.id}>
                        v{version.versionNumber} · {formatTime(version.createdAt)}
                      </option>
                    {/each}
                  {/if}
                </Select>
              </Field>
            </div>

            <p class="note">
              {#if selectedPublishedVersion}
                Current {selectedEnv} version
                <span class="text-mono" title={selectedPublishedVersion}
                  >{selectedPublishedVersion.slice(0, 12)}</span
                >
              {:else}
                No version published to {selectedEnv}
              {/if}
            </p>

            <div class="button-row">
              <Button
                variant="primary"
                loading={!!publishingByWorkflowId[wf.id]}
                onclick={() => publishSelectedVersion(wf)}
                disabled={!selectedVersion || wf.status === 'archived'}
              >
                {publishingByWorkflowId[wf.id] ? 'Publishing...' : 'Publish'}
              </Button>
              <Button
                loading={!!rollingBackByWorkflowId[wf.id]}
                onclick={() => rollbackToSelectedVersion(wf)}
                disabled={!selectedVersion || wf.status === 'archived'}
              >
                {rollingBackByWorkflowId[wf.id] ? 'Rolling back...' : 'Rollback'}
              </Button>
              <Button
                variant="ghost"
                icon={RefreshCw}
                onclick={() => refreshWorkflowVersions(wf)}
                disabled={loadingVersions}
              >
                {loadingVersions ? 'Loading...' : 'Reload versions'}
              </Button>
            </div>

            {#if selectedEnv === 'production'}
              <p class="note">Production publish requires an approved request for the selected version.</p>
            {/if}

            {#if versionErrorByWorkflowId[wf.id]}
              <p class="inline-error" role="alert">
                <Icon icon={CircleAlert} />
                <span>{versionErrorByWorkflowId[wf.id]}</span>
              </p>
            {/if}
          </section>

          <section class="details-section" aria-labelledby="inventory-run-title">
            <h3 id="inventory-run-title" class="section-title">Run an event</h3>
            <Field label="Event JSON" id={`event-${wf.name}`}>
              <Textarea
                mono
                rows={7}
                bind:value={eventJsonByWorkflow[wf.name]}
                placeholder={'{"type":"PATIENT_ADMIT","source":"ui-manual"}'}
                spellcheck="false"
              />
            </Field>

            <div class="button-row">
              <Button
                loading={isRunning}
                onclick={() => runWorkflow(wf.id, wf.name)}
                disabled={!!runningWorkflowName && !isRunning}
              >
                {isRunning ? 'Running...' : 'Run event'}
              </Button>
              <Button variant="ghost" onclick={() => setSampleEvent(wf.name)}>Reset sample</Button>
            </div>

            {#if runError}
              <p class="inline-error" role="alert">
                <Icon icon={CircleAlert} />
                <span>{runError}</span>
              </p>
            {/if}

            {#if runResult}
              <KeyValue items={resultItems(runResult)} />
              {#if runResult.errors.length > 0}
                <ul class="error-list" role="alert">
                  {#each runResult.errors as err, idx (idx)}
                    <li>{err}</li>
                  {/each}
                </ul>
              {/if}
            {/if}
          </section>
        {:else}
          <EmptyState
            icon={MousePointerClick}
            align="start"
            message="Select a workflow to publish a version or run an event."
          />
        {/if}
      </aside>
    </div>
  {/if}
</div>

<style>
  .inventory {
    display: flex;
    flex-direction: column;
    flex: 1 1 auto;
    min-height: 0;
    min-width: 0;
  }

  .filters {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    flex: 0 0 auto;
    padding: var(--space-2) var(--space-3);
    border-bottom: 1px solid var(--color-border-subtle);
  }

  .count {
    margin-left: auto;
    color: var(--color-text-tertiary);
  }

  .status-line {
    margin: 0;
    padding: var(--space-3);
    color: var(--color-text-tertiary);
    font-size: var(--text-ui);
  }

  .split {
    display: grid;
    grid-template-columns: minmax(0, 1fr) 360px;
    flex: 1 1 auto;
    min-height: 0;
  }

  .split :global(.split-table) {
    height: 100%;
  }

  .details {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    min-height: 0;
    overflow: auto;
    padding: var(--space-3);
    border-left: 1px solid var(--color-border-subtle);
    background: var(--color-bg-elevated);
  }

  .details-head {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    min-width: 0;
  }

  .details-title {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: var(--text-ui);
    font-weight: var(--font-semibold);
    color: var(--color-text-primary);
  }

  .details-actions,
  .button-row {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2);
  }

  .details-section {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    padding-top: var(--space-3);
    border-top: 1px solid var(--color-border-subtle);
  }

  .section-title {
    margin: 0;
    font-size: var(--text-label);
    font-weight: var(--font-semibold);
    letter-spacing: var(--tracking-label);
    text-transform: uppercase;
    color: var(--color-text-tertiary);
  }

  .form-grid {
    display: grid;
    grid-template-columns: minmax(0, 2fr) minmax(0, 3fr);
    gap: var(--space-3);
  }

  .note {
    margin: 0;
    font-size: var(--text-xs);
    color: var(--color-text-tertiary);
  }

  .inline-error {
    display: flex;
    align-items: flex-start;
    gap: var(--space-2);
    margin: 0;
    font-size: var(--text-xs);
    color: var(--color-danger-text);
    overflow-wrap: anywhere;
  }

  .inline-error :global(.ui-icon) {
    margin-top: 1px;
  }

  .error-list {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
    margin: 0;
    padding: var(--space-2) var(--space-2) var(--space-2) var(--space-5);
    border: 1px solid var(--color-danger-border);
    border-radius: var(--radius-sm);
    background: var(--color-danger-bg);
    color: var(--color-danger-text);
    font-size: var(--text-xs);
  }

  @media (max-width: 960px) {
    .split {
      grid-template-columns: 1fr;
    }

    .details {
      border-left: 0;
      border-top: 1px solid var(--color-border-subtle);
    }
  }
</style>
