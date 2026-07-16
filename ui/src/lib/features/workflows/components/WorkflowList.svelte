<script lang="ts">
  import { createEventDispatcher, onMount } from "svelte";
  import Panel from "$lib/ui/Panel.svelte";
  import Button from "$lib/ui/Button.svelte";
  import Badge from "$lib/ui/Badge.svelte";
  import EmptyState from "$lib/ui/EmptyState.svelte";
  import Skeleton from "$lib/ui/Skeleton.svelte";
  import {
    fetchWorkflowDefinitions,
    fetchWorkflowVersions,
    publishWorkflowVersion,
    rollbackWorkflowVersion,
    triggerWorkflow,
  } from "../workflowApi";
  import { validateEventPayload } from "../eventPayload";
  import type {
    GetWorkflowVersionsQuery,
    ListWorkflowDefinitionsQuery,
    TriggerWorkflowMutation,
  } from "$lib/gen/graphql";
  import { toasts } from "$lib/ui/toastStore";
  import { isErrorToasted } from "$lib/graphql/client";

  type WorkflowItem =
    ListWorkflowDefinitionsQuery["workflowDefinitions"][number];
  type WorkflowVersionItem =
    GetWorkflowVersionsQuery["workflowVersions"][number];
  type TriggerResult = TriggerWorkflowMutation["triggerWorkflow"];
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

  let workflows: WorkflowItem[] = [];
  let loading = true;
  let error: string | null = null;

  let runningWorkflowName: string | null = null;
  let expandedWorkflowId: string | null = null;

  let eventJsonByWorkflow: Record<string, string> = {};
  let runResultByWorkflow: Record<string, TriggerResult | undefined> = {};
  let runErrorByWorkflow: Record<string, string | undefined> = {};

  let versionsByWorkflowId: Record<string, WorkflowVersionItem[] | undefined> =
    {};
  let loadingVersionsByWorkflowId: Record<string, boolean> = {};
  let versionErrorByWorkflowId: Record<string, string | undefined> = {};

  let selectedVersionByWorkflowId: Record<string, string | undefined> = {};
  let selectedEnvByWorkflowId: Record<string, string | undefined> = {};
  let publishingByWorkflowId: Record<string, boolean> = {};
  let rollingBackByWorkflowId: Record<string, boolean> = {};

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
          offset: 0,
        },
      });
      workflows = data.workflowDefinitions;
    } catch (err) {
      error = err instanceof Error ? err.message : "Failed to load workflows";
    } finally {
      loading = false;
    }
  }

  async function ensureWorkflowVersions(workflow: WorkflowItem) {
    if (loadingVersionsByWorkflowId[workflow.id]) return;
    if (versionsByWorkflowId[workflow.id]) return;

    loadingVersionsByWorkflowId = {
      ...loadingVersionsByWorkflowId,
      [workflow.id]: true,
    };
    versionErrorByWorkflowId = {
      ...versionErrorByWorkflowId,
      [workflow.id]: undefined,
    };

    try {
      const data = await fetchWorkflowVersions(workflow.id, {
        limit: 100,
        offset: 0,
      });
      versionsByWorkflowId = {
        ...versionsByWorkflowId,
        [workflow.id]: data.workflowVersions,
      };

      const publishedProd = getPublishedVersionId(workflow, "production");
      const preferredVersionId =
        publishedProd ??
        workflow.latestVersion?.id ??
        data.workflowVersions[0]?.id;
      selectedVersionByWorkflowId = {
        ...selectedVersionByWorkflowId,
        [workflow.id]: preferredVersionId,
      };
      selectedEnvByWorkflowId = {
        ...selectedEnvByWorkflowId,
        [workflow.id]: selectedEnvByWorkflowId[workflow.id] ?? "staging",
      };
    } catch (err) {
      versionErrorByWorkflowId = {
        ...versionErrorByWorkflowId,
        [workflow.id]:
          err instanceof Error ? err.message : "Failed to load versions",
      };
    } finally {
      loadingVersionsByWorkflowId = {
        ...loadingVersionsByWorkflowId,
        [workflow.id]: false,
      };
    }
  }

  async function refreshWorkflowVersions(workflow: WorkflowItem) {
    versionsByWorkflowId = {
      ...versionsByWorkflowId,
      [workflow.id]: undefined,
    };
    await ensureWorkflowVersions(workflow);
  }

  function getDefaultEventJson(workflowName: string): string {
    const normalized = workflowName.toLowerCase();
    let type = "PATIENT_ADMIT";

    if (normalized.includes("lab") || normalized.includes("oru")) {
      type = "LAB_RESULT";
    } else if (normalized.includes("discharge")) {
      type = "PATIENT_DISCHARGE";
    } else if (normalized.includes("appoint") || normalized.includes("siu")) {
      type = "APPOINTMENT_SCHEDULED";
    }

    return JSON.stringify(
      {
        type,
        source: "ui-manual",
        id: `manual-${Date.now()}`,
        timestamp: new Date().toISOString(),
      },
      null,
      2,
    );
  }

  function toggleWorkflowPanel(workflow: WorkflowItem) {
    if (expandedWorkflowId === workflow.id) {
      expandedWorkflowId = null;
      return;
    }

    expandedWorkflowId = workflow.id;

    if (!eventJsonByWorkflow[workflow.name]) {
      eventJsonByWorkflow = {
        ...eventJsonByWorkflow,
        [workflow.name]: getDefaultEventJson(workflow.name),
      };
    }

    if (!selectedEnvByWorkflowId[workflow.id]) {
      selectedEnvByWorkflowId = {
        ...selectedEnvByWorkflowId,
        [workflow.id]: "staging",
      };
    }

    void ensureWorkflowVersions(workflow);
  }

  function setSampleEvent(workflowName: string) {
    eventJsonByWorkflow = {
      ...eventJsonByWorkflow,
      [workflowName]: getDefaultEventJson(workflowName),
    };
  }

  function parsePublishedVersions(raw: unknown): Record<string, string> {
    if (!raw || typeof raw !== "object") return {};

    const asRecord = raw as Record<string, unknown>;
    const out: Record<string, string> = {};
    for (const [environment, value] of Object.entries(asRecord)) {
      if (typeof value !== "string" || !value.trim()) continue;
      out[environment] = value;
    }
    return out;
  }

  function getPublishedVersionId(
    workflow: WorkflowItem,
    environment: string,
  ): string | undefined {
    const published = parsePublishedVersions(workflow.publishedVersionsByEnv);
    return published[environment];
  }

  function summarizePublishedVersions(workflow: WorkflowItem): string {
    const published = parsePublishedVersions(workflow.publishedVersionsByEnv);
    const entries = Object.entries(published);
    if (entries.length === 0) return "No published environments";
    return entries
      .map(([env, versionId]) => `${env}: ${versionId.slice(0, 12)}`)
      .join(" · ");
  }

  function formatTime(ts: string | null): string {
    if (!ts) return "Never";
    try {
      return new Date(ts).toLocaleString();
    } catch {
      return ts;
    }
  }

  function getSelectedEnvironment(workflowId: string): string {
    return selectedEnvByWorkflowId[workflowId] ?? "staging";
  }

  function getSelectedVersionId(workflow: WorkflowItem): string | undefined {
    return (
      selectedVersionByWorkflowId[workflow.id] ??
      getPublishedVersionId(workflow, "production") ??
      workflow.latestVersion?.id ??
      versionsByWorkflowId[workflow.id]?.[0]?.id
    );
  }

  function emitOpenBuilder(workflow: WorkflowItem) {
    dispatch("openBuilder", {
      workflowId: workflow.id,
      name: workflow.name,
      description: workflow.description ?? null,
      versionId: getSelectedVersionId(workflow) ?? null,
      versionNumber: workflow.latestVersion?.versionNumber ?? null,
    });
  }

  function emitOpenMonitor(workflowName: string) {
    dispatch("openMonitor", { workflowName });
  }

  async function runWorkflow(workflowId: string, workflowName: string) {
    if (runningWorkflowName) return;

    // Persistent payload validation belongs inline at the field, not in a
    // transient toast (.loom/22 B1/B4): show the message in runErrorByWorkflow,
    // which renders as an inline `role="alert"` next to the Event JSON textarea.
    const payload = validateEventPayload(eventJsonByWorkflow[workflowName] ?? "");
    if (!payload.ok) {
      runErrorByWorkflow = {
        ...runErrorByWorkflow,
        [workflowName]: payload.message,
      };
      return;
    }
    const parsedEvent = payload.value;

    runningWorkflowName = workflowName;
    runErrorByWorkflow = { ...runErrorByWorkflow, [workflowName]: undefined };

    try {
      const data = await triggerWorkflow(workflowName, parsedEvent, {
        environment: getSelectedEnvironment(workflowId),
      });
      runResultByWorkflow = {
        ...runResultByWorkflow,
        [workflowName]: data.triggerWorkflow,
      };

      if (data.triggerWorkflow.errors.length > 0) {
        toasts.error(
          `Workflow ran with ${data.triggerWorkflow.errors.length} error${data.triggerWorkflow.errors.length === 1 ? "" : "s"}`,
        );
      } else {
        toasts.success(
          `Workflow executed: ${data.triggerWorkflow.actionsExecuted} action${data.triggerWorkflow.actionsExecuted === 1 ? "" : "s"}`,
        );
      }
    } catch (err) {
      const msg =
        err instanceof Error ? err.message : "Failed to trigger workflow";
      runErrorByWorkflow = {
        ...runErrorByWorkflow,
        [workflowName]: msg,
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
      toasts.error("Select a version to publish");
      return;
    }
    const environment = getSelectedEnvironment(workflowId);
    publishingByWorkflowId = { ...publishingByWorkflowId, [workflowId]: true };

    try {
      await publishWorkflowVersion({
        workflowId,
        versionId,
        environment,
      });
      await loadWorkflows();
      await refreshWorkflowVersions(workflow);
      toasts.success(`Published ${workflow.name} to ${environment}`);
    } catch (err) {
      // Global graphqlFetch net already toasts graphql failures (B4 dedupe).
      if (!isErrorToasted(err)) {
        toasts.error(
          err instanceof Error ? err.message : "Failed to publish workflow",
        );
      }
    } finally {
      publishingByWorkflowId = {
        ...publishingByWorkflowId,
        [workflowId]: false,
      };
    }
  }

  async function rollbackToSelectedVersion(workflow: WorkflowItem) {
    const workflowId = workflow.id;
    const versionId = getSelectedVersionId(workflow);
    if (!versionId) {
      toasts.error("Select a version to roll back to");
      return;
    }
    const environment = getSelectedEnvironment(workflowId);
    rollingBackByWorkflowId = {
      ...rollingBackByWorkflowId,
      [workflowId]: true,
    };

    try {
      await rollbackWorkflowVersion({
        workflowId,
        targetVersionId: versionId,
        environment,
      });
      await loadWorkflows();
      await refreshWorkflowVersions(workflow);
      toasts.success(`Rolled back ${workflow.name} in ${environment}`);
    } catch (err) {
      // Global graphqlFetch net already toasts graphql failures (B4 dedupe).
      if (!isErrorToasted(err)) {
        toasts.error(
          err instanceof Error ? err.message : "Failed to roll back workflow",
        );
      }
    } finally {
      rollingBackByWorkflowId = {
        ...rollingBackByWorkflowId,
        [workflowId]: false,
      };
    }
  }
</script>

<Panel>
  <div class="list-header">
    <div class="list-title">Managed Workflows</div>
    <Button
      variant="secondary"
      size="sm"
      on:click={loadWorkflows}
      disabled={loading}
    >
      {loading ? "Refreshing..." : "Refresh"}
    </Button>
  </div>

  {#if loading}
    <div class="skeleton-list">
      <Skeleton height="48px" />
      <Skeleton height="48px" />
      <Skeleton height="48px" />
    </div>
  {:else if error}
    <EmptyState
      icon="error"
      title="Failed to load workflows"
      description={error}
    />
  {:else if workflows.length === 0}
    <EmptyState
      icon="inbox"
      title="No managed workflows found"
      description="Create a managed definition in Builder to version and publish."
    />
  {:else}
    <div class="workflow-list cards">
      {#each workflows as wf, i (wf.id)}
        {@const isPanelOpen = expandedWorkflowId === wf.id}
        {@const isRunning = runningWorkflowName === wf.name}
        {@const runResult = runResultByWorkflow[wf.name]}
        {@const runError = runErrorByWorkflow[wf.name]}
        {@const workflowVersions = versionsByWorkflowId[wf.id] ?? []}
        {@const selectedEnv = getSelectedEnvironment(wf.id)}
        {@const selectedVersion = getSelectedVersionId(wf)}
        {@const selectedPublishedVersion = getPublishedVersionId(
          wf,
          selectedEnv,
        )}

        <div
          class="workflow-row card hover-lift"
          class:expanded={isPanelOpen}
          style="animation-delay: {Math.min(i, 20) * 0.05}s"
        >
          <div class="workflow-name">{wf.name}</div>
          <div class="workflow-meta">
            <Badge
              variant={wf.status === "archived" ? "danger" : "success"}
              size="sm"
            >
              {wf.status}
            </Badge>
            {#if wf.latestVersion}
              <span class="stat">v{wf.latestVersion.versionNumber}</span>
            {:else}
              <span class="stat">No versions</span>
            {/if}
            <span class="stat">{summarizePublishedVersions(wf)}</span>
          </div>
          <div class="workflow-time muted">{formatTime(wf.updatedAt)}</div>
          <div class="workflow-actions">
            <Button
              variant="secondary"
              size="sm"
              on:click={() => emitOpenBuilder(wf)}
            >
              Open in Builder
            </Button>
            <Button
              variant="secondary"
              size="sm"
              on:click={() => emitOpenMonitor(wf.name)}
            >
              View Runs
            </Button>
            <Button
              variant="secondary"
              size="sm"
              on:click={() => toggleWorkflowPanel(wf)}
            >
              {isPanelOpen ? "Hide" : "Manage"}
            </Button>
          </div>

          {#if isPanelOpen}
            <div class="panel-content">
              <div class="publish-controls">
                <label class="field-label">
                  Environment
                  <select
                    class="input"
                    value={selectedEnv}
                    on:change={(e) => {
                      selectedEnvByWorkflowId = {
                        ...selectedEnvByWorkflowId,
                        [wf.id]: (e.target as HTMLSelectElement).value,
                      };
                    }}
                  >
                    <option value="staging">staging</option>
                    <option value="production">production</option>
                  </select>
                </label>

                <label class="field-label">
                  Version
                  <select
                    class="input"
                    value={selectedVersion ?? ""}
                    on:change={(e) => {
                      selectedVersionByWorkflowId = {
                        ...selectedVersionByWorkflowId,
                        [wf.id]: (e.target as HTMLSelectElement).value,
                      };
                    }}
                    disabled={loadingVersionsByWorkflowId[wf.id]}
                  >
                    {#if workflowVersions.length === 0}
                      <option value="">No saved versions</option>
                    {:else}
                      {#each workflowVersions as version (version.id)}
                        <option value={version.id}>
                          v{version.versionNumber} · {new Date(
                            version.createdAt,
                          ).toLocaleString()}
                        </option>
                      {/each}
                    {/if}
                  </select>
                </label>

                <div class="publish-actions">
                  <Button
                    variant="secondary"
                    size="sm"
                    on:click={() => refreshWorkflowVersions(wf)}
                    disabled={!!loadingVersionsByWorkflowId[wf.id]}
                  >
                    {loadingVersionsByWorkflowId[wf.id]
                      ? "Loading..."
                      : "Reload Versions"}
                  </Button>
                  <Button
                    size="sm"
                    loading={!!publishingByWorkflowId[wf.id]}
                    on:click={() => publishSelectedVersion(wf)}
                    disabled={!selectedVersion || wf.status === "archived"}
                  >
                    {publishingByWorkflowId[wf.id]
                      ? "Publishing..."
                      : "Publish"}
                  </Button>
                  <Button
                    variant="secondary"
                    size="sm"
                    loading={!!rollingBackByWorkflowId[wf.id]}
                    on:click={() => rollbackToSelectedVersion(wf)}
                    disabled={!selectedVersion || wf.status === "archived"}
                  >
                    {rollingBackByWorkflowId[wf.id]
                      ? "Rolling Back..."
                      : "Rollback"}
                  </Button>
                </div>
              </div>

              {#if versionErrorByWorkflowId[wf.id]}
                <div class="runner-error" role="alert">
                  {versionErrorByWorkflowId[wf.id]}
                </div>
              {/if}

              <div class="published-summary muted">
                {#if selectedPublishedVersion}
                  Current {selectedEnv} version:
                  <span class="mono">{selectedPublishedVersion}</span>
                {:else}
                  No version currently published to {selectedEnv}
                {/if}
              </div>
              {#if selectedEnv === "production"}
                <div class="gate-hint">
                  Production publish requires an approved request for the
                  selected version.
                </div>
              {/if}

              <div class="runner">
                <label class="runner-label" for={`event-${wf.name}`}>
                  Event JSON
                </label>
                <textarea
                  id={`event-${wf.name}`}
                  class="runner-input mono"
                  rows="7"
                  bind:value={eventJsonByWorkflow[wf.name]}
                  placeholder={'{"type":"PATIENT_ADMIT","source":"ui-manual"}'}
                  spellcheck="false"
                ></textarea>

                <div class="runner-actions">
                  <Button
                    variant="secondary"
                    size="sm"
                    on:click={() => setSampleEvent(wf.name)}
                  >
                    Reset Sample
                  </Button>
                  <Button
                    size="sm"
                    loading={isRunning}
                    on:click={() => runWorkflow(wf.id, wf.name)}
                  >
                    {isRunning ? "Running..." : "Run Event"}
                  </Button>
                </div>

                {#if runError}
                  <div class="runner-error" role="alert">{runError}</div>
                {/if}

                {#if runResult}
                  <div class="runner-result">
                    <div class="result-row">
                      <span class="muted">Matched Routes</span>
                      <span class="mono">{runResult.routesMatched}</span>
                    </div>
                    <div class="result-row">
                      <span class="muted">Executed Actions</span>
                      <span class="mono">{runResult.actionsExecuted}</span>
                    </div>
                    <div class="result-row">
                      <span class="muted">Duration</span>
                      <span class="mono"
                        >{runResult.duration.toFixed(2)} ms</span
                      >
                    </div>
                    {#if runResult.runId}
                      <div class="result-row">
                        <span class="muted">Run ID</span>
                        <span class="mono">{runResult.runId}</span>
                      </div>
                    {/if}
                    {#if runResult.environment}
                      <div class="result-row">
                        <span class="muted">Environment</span>
                        <span class="mono">{runResult.environment}</span>
                      </div>
                    {/if}
                    {#if runResult.versionId}
                      <div class="result-row">
                        <span class="muted">Version</span>
                        <span class="mono">{runResult.versionId}</span>
                      </div>
                    {/if}
                    {#if runResult.errors.length > 0}
                      <div class="result-errors" role="alert">
                        {#each runResult.errors as err, idx (idx)}
                          <div class="result-error-item">{err}</div>
                        {/each}
                      </div>
                    {/if}
                  </div>
                {/if}
              </div>
            </div>
          {/if}
        </div>
      {/each}
    </div>
  {/if}
</Panel>

<style>
  .list-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 10px;
    margin-bottom: 10px;
  }

  .list-title {
    color: var(--color-text-tertiary);
    font-size: 0.85rem;
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 0.05em;
  }

  .skeleton-list {
    display: grid;
    gap: 8px;
  }

  .workflow-list {
    display: grid;
    gap: var(--space-3);
  }

  .workflow-row {
    display: grid;
    grid-template-columns: 1fr auto auto auto;
    gap: 12px;
    align-items: center;
    padding: var(--space-3) var(--space-4);
    border-radius: var(--radius-lg);
    background: var(--color-bg-surface);
    border: 1px solid var(--color-border-subtle);
    border-top: 1px solid rgba(255, 255, 255, 0.05); /* 3D depth */
    box-shadow: var(--shadow-sm);
    transition: var(--transition-all);
    animation: fade-in-up 0.4s ease-out both;
  }

  @keyframes fade-in-up {
    from {
      opacity: 0;
      transform: translateY(10px);
    }
    to {
      opacity: 1;
      transform: translateY(0);
    }
  }

  .workflow-row:hover {
    background: var(--color-bg-hover);
    border-color: var(--color-border-strong);
    transform: translateY(-2px);
    box-shadow: var(--shadow-md);
  }

  .workflow-row.expanded {
    border-color: var(--color-primary-border);
    background: var(--color-bg-elevated);
    box-shadow: 0 0 0 1px var(--color-primary-glow);
  }

  .workflow-name {
    font-weight: 700;
    color: var(--color-text-primary);
    font-family:
      ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  }

  .workflow-meta {
    display: flex;
    align-items: center;
    gap: 8px;
    flex-wrap: wrap;
  }

  .stat {
    color: var(--color-text-tertiary);
    font-size: 0.85rem;
  }

  .workflow-time {
    font-size: 0.85rem;
    white-space: nowrap;
  }

  .workflow-actions {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
    flex-wrap: wrap;
  }

  .panel-content {
    grid-column: 1 / -1;
    display: grid;
    gap: 8px;
    padding-top: 10px;
    border-top: 1px solid var(--color-border-subtle);
  }

  .publish-controls {
    display: grid;
    grid-template-columns: repeat(2, minmax(180px, 220px)) 1fr;
    gap: 10px;
    align-items: end;
  }

  .publish-actions {
    display: flex;
    align-items: center;
    gap: 8px;
    flex-wrap: wrap;
  }

  .field-label {
    display: grid;
    gap: 4px;
    color: var(--color-text-tertiary);
    font-size: 0.8rem;
    font-weight: 700;
  }

  .input {
    padding: 8px 10px;
    border-radius: 8px;
    border: 1px solid var(--color-border-default);
    background: var(--color-bg-input);
    color: var(--color-text-primary);
    outline: none;
    width: 100%;
    box-sizing: border-box;
  }

  .input:hover:not(:disabled):not(:focus) {
    border-color: var(--color-border-strong);
  }

  .input:focus {
    border-color: var(--color-border-focus);
    box-shadow: var(--shadow-focus);
  }

  .published-summary {
    font-size: 0.85rem;
  }

  .gate-hint {
    padding: 8px 10px;
    border-radius: 8px;
    border: 1px solid rgba(245, 158, 11, 0.35);
    background: rgba(245, 158, 11, 0.14);
    color: rgba(253, 230, 138, 0.95);
    font-size: 0.82rem;
  }

  .runner {
    display: grid;
    gap: 8px;
    padding-top: 6px;
    border-top: 1px solid var(--color-border-subtle);
  }

  .runner-label {
    color: var(--color-text-tertiary);
    font-size: 0.8rem;
    font-weight: 700;
  }

  .runner-input {
    padding: 8px 10px;
    border-radius: 8px;
    border: 1px solid var(--color-border-default);
    background: var(--color-bg-input);
    color: var(--color-text-primary);
    resize: vertical;
    outline: none;
    width: 100%;
    box-sizing: border-box;
    transition: var(--transition-all);
  }

  .runner-input:hover:not(:disabled):not(:focus) {
    border-color: var(--color-border-strong);
  }

  .runner-input:focus {
    border-color: var(--color-border-focus);
    box-shadow: var(--shadow-focus);
  }

  .runner-actions {
    display: flex;
    align-items: center;
    gap: 8px;
    flex-wrap: wrap;
  }

  .runner-result {
    display: grid;
    gap: 4px;
    padding: 8px 10px;
    border-radius: 8px;
    border: 1px solid var(--color-border-default);
    background: var(--color-bg-surface);
  }

  .result-row {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 8px;
    font-size: 0.85rem;
  }

  .runner-error,
  .result-errors {
    padding: 8px 10px;
    border-radius: 8px;
    border: 1px solid var(--color-danger-border);
    background: var(--color-danger-bg);
    color: var(--color-danger-text);
    font-size: 0.85rem;
  }

  .result-errors {
    margin-top: 4px;
    display: grid;
    gap: 4px;
  }

  .muted {
    color: var(--color-text-muted);
  }

  .mono {
    font-family:
      ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  }

  @media (max-width: 960px) {
    .publish-controls {
      grid-template-columns: 1fr;
    }
  }

  @media (max-width: 640px) {
    .workflow-row {
      grid-template-columns: 1fr;
      gap: 6px;
    }

    .workflow-meta {
      flex-wrap: wrap;
    }

    .workflow-actions {
      justify-content: flex-start;
    }
  }
</style>
