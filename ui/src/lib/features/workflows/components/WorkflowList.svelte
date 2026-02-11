<script lang="ts">
  import { onMount } from 'svelte';
  import Panel from '$lib/ui/Panel.svelte';
  import Button from '$lib/ui/Button.svelte';
  import Badge from '$lib/ui/Badge.svelte';
  import EmptyState from '$lib/ui/EmptyState.svelte';
  import Skeleton from '$lib/ui/Skeleton.svelte';
  import { fetchWorkflows, triggerWorkflow } from '../workflowApi';
  import type { ListWorkflowsQuery, TriggerWorkflowMutation } from '$lib/gen/graphql';
  import { toasts } from '$lib/ui/toastStore';

  type WorkflowItem = ListWorkflowsQuery['workflows'][number];
  type TriggerResult = TriggerWorkflowMutation['triggerWorkflow'];

  let workflows: WorkflowItem[] = [];
  let loading = true;
  let error: string | null = null;
  let runningWorkflowName: string | null = null;
  let expandedRunnerWorkflow: string | null = null;

  let eventJsonByWorkflow: Record<string, string> = {};
  let runResultByWorkflow: Record<string, TriggerResult | undefined> = {};
  let runErrorByWorkflow: Record<string, string | undefined> = {};

  onMount(() => {
    void loadWorkflows();
  });

  async function loadWorkflows() {
    loading = true;
    error = null;
    try {
      const data = await fetchWorkflows();
      workflows = data.workflows;
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to load workflows';
    } finally {
      loading = false;
    }
  }

  function formatTime(ts: string | null): string {
    if (!ts) return 'Never';
    try {
      return new Date(ts).toLocaleString();
    } catch {
      return ts;
    }
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

  function toggleRunner(workflowName: string) {
    if (expandedRunnerWorkflow === workflowName) {
      expandedRunnerWorkflow = null;
      return;
    }

    expandedRunnerWorkflow = workflowName;

    if (!eventJsonByWorkflow[workflowName]) {
      eventJsonByWorkflow = {
        ...eventJsonByWorkflow,
        [workflowName]: getDefaultEventJson(workflowName)
      };
    }
  }

  async function runWorkflow(workflowName: string) {
    if (runningWorkflowName) return;

    const eventJson = eventJsonByWorkflow[workflowName]?.trim() ?? '';
    if (!eventJson) {
      toasts.error('Provide an event JSON payload first');
      return;
    }

    let parsedEvent: unknown;
    try {
      parsedEvent = JSON.parse(eventJson);
    } catch {
      toasts.error('Invalid JSON payload');
      runErrorByWorkflow = {
        ...runErrorByWorkflow,
        [workflowName]: 'Invalid JSON payload'
      };
      return;
    }

    if (parsedEvent === null || typeof parsedEvent !== 'object' || Array.isArray(parsedEvent)) {
      toasts.error('Event payload must be a JSON object');
      runErrorByWorkflow = {
        ...runErrorByWorkflow,
        [workflowName]: 'Event payload must be a JSON object'
      };
      return;
    }

    runningWorkflowName = workflowName;
    runErrorByWorkflow = { ...runErrorByWorkflow, [workflowName]: undefined };

    try {
      const data = await triggerWorkflow(workflowName, parsedEvent);
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
      toasts.error(msg);
    } finally {
      runningWorkflowName = null;
    }
  }

  function setSampleEvent(workflowName: string) {
    eventJsonByWorkflow = {
      ...eventJsonByWorkflow,
      [workflowName]: getDefaultEventJson(workflowName)
    };
  }
</script>

<Panel>
  <div class="list-header">
    <div class="list-title">Workflow Registry</div>
    <Button variant="secondary" size="sm" on:click={loadWorkflows} disabled={loading}>
      {loading ? 'Refreshing...' : 'Refresh'}
    </Button>
  </div>

  {#if loading}
    <div class="skeleton-list">
      <Skeleton height="48px" />
      <Skeleton height="48px" />
      <Skeleton height="48px" />
    </div>
  {:else if error}
    <EmptyState icon="error" title="Failed to load workflows" description={error} />
  {:else if workflows.length === 0}
    <EmptyState
      icon="inbox"
      title="No workflows found"
      description="Workflows defined in YAML will appear here. Switch to the Builder tab to create one."
    />
  {:else}
    <div class="workflow-list">
      {#each workflows as wf (wf.name)}
        {@const isRunnerOpen = expandedRunnerWorkflow === wf.name}
        {@const isRunning = runningWorkflowName === wf.name}
        {@const runResult = runResultByWorkflow[wf.name]}
        {@const runError = runErrorByWorkflow[wf.name]}

        <div class="workflow-row" class:expanded={isRunnerOpen}>
          <div class="workflow-name">{wf.name}</div>
          <div class="workflow-meta">
            <Badge variant={wf.enabled ? 'success' : 'default'} size="sm">
              {wf.enabled ? 'Active' : 'Inactive'}
            </Badge>
            <span class="stat">{wf.routeCount} routes</span>
            <span class="stat">{wf.eventsProcessed} events</span>
            {#if wf.errors > 0}
              <Badge variant="danger" size="sm">{wf.errors} errors</Badge>
            {/if}
          </div>
          <div class="workflow-time muted">{formatTime(wf.lastEventTime)}</div>
          <div class="workflow-actions">
            <Button
              variant="secondary"
              size="sm"
              disabled={!wf.enabled}
              on:click={() => toggleRunner(wf.name)}
            >
              {isRunnerOpen ? 'Hide Runner' : 'Trigger'}
            </Button>
          </div>

          {#if isRunnerOpen}
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
                <Button variant="secondary" size="sm" on:click={() => setSampleEvent(wf.name)}>
                  Reset Sample
                </Button>
                <Button size="sm" loading={isRunning} on:click={() => runWorkflow(wf.name)}>
                  {isRunning ? 'Running...' : 'Run Event'}
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
                    <span class="mono">{runResult.duration.toFixed(2)} ms</span>
                  </div>
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
    gap: 6px;
  }

  .workflow-row {
    display: grid;
    grid-template-columns: 1fr auto auto auto;
    gap: 12px;
    align-items: center;
    padding: 10px 12px;
    border-radius: 8px;
    border: 1px solid var(--color-border-subtle);
    background: var(--color-bg-surface);
  }

  .workflow-row:hover {
    background: var(--color-bg-hover);
  }

  .workflow-row.expanded {
    border-color: var(--color-primary-border);
    background: var(--color-bg-elevated);
  }

  .workflow-name {
    font-weight: 700;
    color: var(--color-text-primary);
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  }

  .workflow-meta {
    display: flex;
    align-items: center;
    gap: 8px;
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
  }

  .runner {
    grid-column: 1 / -1;
    display: grid;
    gap: 8px;
    padding-top: 10px;
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
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
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
