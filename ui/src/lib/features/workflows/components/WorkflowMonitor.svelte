<script lang="ts">
  import { onDestroy, onMount } from 'svelte';
  import { subscribe as wsSubscribe } from '$lib/graphql/subscriptions';
  import { WorkflowEventsDocument, type WorkflowEventsSubscription } from '$lib/gen/graphql';
  import Panel from '$lib/ui/Panel.svelte';
  import Button from '$lib/ui/Button.svelte';
  import {
    approveWorkflowVersion,
    fetchWorkflowDefinitions,
    fetchWorkflowApprovalRequests,
    fetchWorkflowRun,
    fetchWorkflowRuns,
    rejectWorkflowVersion
  } from '../workflowApi';
  import type {
    GetWorkflowRunQuery,
    ListWorkflowApprovalRequestsQuery,
    ListWorkflowApprovalRequestsQueryVariables,
    ListWorkflowDefinitionsQuery,
    ListWorkflowRunsQuery,
    ListWorkflowRunsQueryVariables
  } from '$lib/gen/graphql';
  import { toasts } from '$lib/ui/toastStore';

  type WfEvent = WorkflowEventsSubscription['workflowEvents'];
  type WorkflowDefinitionItem = ListWorkflowDefinitionsQuery['workflowDefinitions'][number];
  type WorkflowRunItem = ListWorkflowRunsQuery['workflowRuns'][number];
  type WorkflowApprovalItem =
    ListWorkflowApprovalRequestsQuery['workflowApprovalRequests'][number];

  export let initialWorkflowName: string | null = null;

  let workflowName = '';
  let events: WfEvent[] = [];
  let connected = false;
  let liveError: string | null = null;
  let paused = false;
  let unsubscribe: (() => void) | null = null;
  const maxEvents = 100;

  let definitions: WorkflowDefinitionItem[] = [];
  let loadingDefinitions = false;
  let runs: WorkflowRunItem[] = [];
  let loadingRuns = false;
  let runsError: string | null = null;

  let filterWorkflowName = '';
  let filterEnvironment = '';
  let filterStatus = '';
  let filterFrom = '';
  let filterTo = '';

  let selectedRun: GetWorkflowRunQuery['workflowRun'] | null = null;
  let loadingSelectedRun = false;
  let appliedInitialWorkflowSelection = '';

  let approvalRequests: WorkflowApprovalItem[] = [];
  let loadingApprovals = false;
  let approvalsError: string | null = null;
  let approvalFilterWorkflowId = '';
  let approvalFilterEnvironment = '';
  let approvalFilterStatus = 'pending';
  let approvalCommentById: Record<string, string> = {};
  let approvalActionInFlightById: Record<string, boolean> = {};

  $: if (initialWorkflowName && initialWorkflowName !== appliedInitialWorkflowSelection) {
    appliedInitialWorkflowSelection = initialWorkflowName;
    filterWorkflowName = initialWorkflowName;
    workflowName = initialWorkflowName;
    void loadRuns();
  }

  onMount(() => {
    void loadDefinitions();
    void loadRuns();
    void loadApprovals();
  });

  function startSubscription() {
    if (!workflowName.trim()) return;
    stopSubscription();
    liveError = null;
    connected = false;

    unsubscribe = wsSubscribe(
      WorkflowEventsDocument,
      { workflowName },
      {
        onData: (data) => {
          connected = true;
          if (!paused && data.workflowEvents) {
            events = [data.workflowEvents, ...events].slice(0, maxEvents);
          }
        },
        onError: (err) => {
          liveError = err.message;
          connected = false;
        },
        onComplete: () => {
          connected = false;
        }
      }
    );
  }

  function stopSubscription() {
    if (unsubscribe) {
      unsubscribe();
      unsubscribe = null;
    }
    connected = false;
  }

  function clearEvents() {
    events = [];
  }

  async function loadDefinitions() {
    loadingDefinitions = true;
    try {
      const data = await fetchWorkflowDefinitions({
        paging: { limit: 200, offset: 0 }
      });
      definitions = data.workflowDefinitions;
      if (initialWorkflowName?.trim() && !approvalFilterWorkflowId) {
        const selected = definitions.find((def) => def.name === initialWorkflowName);
        if (selected) {
          approvalFilterWorkflowId = selected.id;
        }
      }
    } catch {
      definitions = [];
    } finally {
      loadingDefinitions = false;
    }
  }

  function toStartISO(dateValue: string): string | null {
    if (!dateValue.trim()) return null;
    return new Date(`${dateValue}T00:00:00`).toISOString();
  }

  function toEndISO(dateValue: string): string | null {
    if (!dateValue.trim()) return null;
    return new Date(`${dateValue}T23:59:59.999`).toISOString();
  }

  function buildRunFilter(): ListWorkflowRunsQueryVariables['filter'] {
    return {
      workflowName: filterWorkflowName.trim() ? filterWorkflowName.trim() : null,
      environment: filterEnvironment.trim() ? filterEnvironment.trim() : null,
      status: filterStatus.trim() ? filterStatus.trim() : null,
      fromStartedAt: toStartISO(filterFrom),
      toStartedAt: toEndISO(filterTo)
    };
  }

  async function loadRuns() {
    loadingRuns = true;
    runsError = null;
    try {
      const data = await fetchWorkflowRuns({
        filter: buildRunFilter(),
        paging: { limit: 100, offset: 0 }
      });
      runs = data.workflowRuns;
    } catch (err) {
      runsError = err instanceof Error ? err.message : 'Failed to load workflow runs';
      runs = [];
    } finally {
      loadingRuns = false;
    }
  }

  async function loadRunDetail(runID: string) {
    loadingSelectedRun = true;
    runsError = null;
    try {
      const data = await fetchWorkflowRun(runID);
      selectedRun = data.workflowRun ?? null;
    } catch (err) {
      runsError = err instanceof Error ? err.message : 'Failed to load run detail';
      selectedRun = null;
    } finally {
      loadingSelectedRun = false;
    }
  }

  function buildApprovalFilter(): ListWorkflowApprovalRequestsQueryVariables['filter'] {
    return {
      workflowId: approvalFilterWorkflowId.trim() ? approvalFilterWorkflowId : null,
      environment: approvalFilterEnvironment.trim() ? approvalFilterEnvironment : null,
      status: approvalFilterStatus.trim() ? approvalFilterStatus : null
    };
  }

  async function loadApprovals() {
    loadingApprovals = true;
    approvalsError = null;

    try {
      const data = await fetchWorkflowApprovalRequests({
        filter: buildApprovalFilter(),
        paging: { limit: 200, offset: 0 }
      });
      approvalRequests = data.workflowApprovalRequests;
    } catch (err) {
      approvalsError = err instanceof Error ? err.message : 'Failed to load approval requests';
      approvalRequests = [];
    } finally {
      loadingApprovals = false;
    }
  }

  function clearApprovalFilters() {
    approvalFilterWorkflowId = '';
    approvalFilterEnvironment = '';
    approvalFilterStatus = 'pending';
  }

  async function runApprovalAction(id: string, action: 'approve' | 'reject') {
    if (!id) return;
    approvalActionInFlightById = { ...approvalActionInFlightById, [id]: true };
    approvalsError = null;
    const comment = approvalCommentById[id]?.trim() ?? '';

    try {
      if (action === 'approve') {
        await approveWorkflowVersion({
          approvalRequestId: id,
          comment: comment || null
        });
        toasts.success('Approval request approved');
      } else {
        await rejectWorkflowVersion({
          approvalRequestId: id,
          comment: comment || null
        });
        toasts.success('Approval request rejected');
      }
      await loadApprovals();
      await loadRuns();
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to update approval request';
      approvalsError = message;
      toasts.error(message);
    } finally {
      approvalActionInFlightById = { ...approvalActionInFlightById, [id]: false };
    }
  }

  function clearRunFilters() {
    filterWorkflowName = '';
    filterEnvironment = '';
    filterStatus = '';
    filterFrom = '';
    filterTo = '';
  }

  function formatTimestamp(ts: string): string {
    try {
      return new Date(ts).toLocaleTimeString();
    } catch {
      return ts;
    }
  }

  function formatDateTime(ts: string): string {
    try {
      return new Date(ts).toLocaleString();
    } catch {
      return ts;
    }
  }

  function shortValue(value: string | null | undefined, max = 16): string {
    if (!value) return '-';
    if (value.length <= max) return value;
    return `${value.slice(0, max)}…`;
  }

  function workflowNameFromID(workflowID: string): string {
    return definitions.find((def) => def.id === workflowID)?.name ?? workflowID;
  }

  onDestroy(() => {
    stopSubscription();
  });
</script>

<Panel title="Workflow Monitor">
  <div class="monitor">
    <div class="section-title">Live Stream</div>
    <div class="connect-bar">
      <label class="field">
        Workflow Name
        <input
          type="text"
          class="input"
          bind:value={workflowName}
          placeholder="e.g. adt-routing"
          on:keydown={(e) => e.key === 'Enter' && startSubscription()}
        />
      </label>
      <div class="actions">
        <Button on:click={startSubscription} disabled={!workflowName.trim()}>
          {connected ? 'Reconnect' : 'Connect'}
        </Button>
        {#if connected}
          <Button variant="secondary" on:click={() => (paused = !paused)}>
            {paused ? 'Resume' : 'Pause'}
          </Button>
          <Button variant="secondary" on:click={clearEvents}>Clear</Button>
          <Button variant="secondary" on:click={stopSubscription}>Disconnect</Button>
        {/if}
      </div>
    </div>

    <div class="status" role="status" aria-live="polite">
      <span class="indicator" class:connected class:error={!!liveError}></span>
      {#if liveError}
        <span class="status-text error">{liveError}</span>
      {:else if connected}
        <span class="status-text">Connected to {workflowName}</span>
      {:else}
        <span class="status-text">Not connected</span>
      {/if}
    </div>

    {#if events.length === 0}
      <div class="empty">
        {#if connected}
          Waiting for workflow events...
        {:else}
          Enter a workflow name and click Connect to monitor live events.
        {/if}
      </div>
    {:else}
      <div class="event-list">
        {#each events as ev, i (i)}
          <div class="event-row">
            <span class="time mono">{formatTimestamp(ev.event.timestamp)}</span>
            <span class="type-pill">{ev.event.type.replace(/_/g, ' ')}</span>
            <span class="routes mono">{ev.routesMatched.join(', ')}</span>
            <span class="actions-col">{ev.actionsExecuted.length} actions</span>
            <span class="duration muted">{ev.duration}ms</span>
          </div>
        {/each}
      </div>
      <div class="footer muted">
        {events.length} events
        {#if paused}<span class="paused-badge">PAUSED</span>{/if}
      </div>
    {/if}

    <div class="section-title">Run Diagnostics</div>
    <div class="filters">
      <label class="field">
        Workflow
        <select class="input" bind:value={filterWorkflowName} disabled={loadingDefinitions}>
          <option value="">All workflows</option>
          {#each definitions as def (def.id)}
            <option value={def.name}>{def.name}</option>
          {/each}
        </select>
      </label>

      <label class="field">
        Environment
        <select class="input" bind:value={filterEnvironment}>
          <option value="">All environments</option>
          <option value="staging">staging</option>
          <option value="production">production</option>
        </select>
      </label>

      <label class="field">
        Status
        <select class="input" bind:value={filterStatus}>
          <option value="">All statuses</option>
          <option value="success">success</option>
          <option value="failed">failed</option>
        </select>
      </label>

      <label class="field">
        From
        <input class="input" type="date" bind:value={filterFrom} />
      </label>

      <label class="field">
        To
        <input class="input" type="date" bind:value={filterTo} />
      </label>
    </div>

    <div class="actions">
      <Button on:click={loadRuns} loading={loadingRuns}>{loadingRuns ? 'Loading...' : 'Apply Filters'}</Button>
      <Button
        variant="secondary"
        on:click={() => {
          clearRunFilters();
          void loadRuns();
        }}
      >
        Clear Filters
      </Button>
    </div>

    {#if runsError}
      <div class="error-box" role="alert">{runsError}</div>
    {/if}

    {#if runs.length === 0}
      <div class="empty">No workflow runs found for the current filter.</div>
    {:else}
      <div class="run-table">
        <div class="run-header">
          <span>Started</span>
          <span>Workflow</span>
          <span>Env</span>
          <span>Status</span>
          <span>Routes</span>
          <span>Actions</span>
          <span>Duration</span>
          <span>Version</span>
          <span></span>
        </div>
        {#each runs as run (run.id)}
          <div class="run-row">
            <span class="mono">{formatDateTime(run.startedAt)}</span>
            <span class="mono">{run.workflowName}</span>
            <span>{run.environment}</span>
            <span>
              <span class="status-pill" class:failed={run.status === 'failed'}>{run.status}</span>
            </span>
            <span>{run.routesMatched}</span>
            <span>{run.actionsExecuted}</span>
            <span>{run.durationMs}ms</span>
            <span class="mono">{shortValue(run.versionId)}</span>
            <span>
              <Button
                variant="secondary"
                size="sm"
                on:click={() => loadRunDetail(run.id)}
                loading={loadingSelectedRun && selectedRun?.id === run.id}
              >
                Details
              </Button>
            </span>
          </div>
        {/each}
      </div>
    {/if}

    {#if selectedRun}
      <div class="detail">
        <div class="detail-title">Run Detail</div>
        <div class="detail-grid">
          <div class="detail-item"><span class="muted">Run ID</span><span class="mono">{selectedRun.id}</span></div>
          <div class="detail-item"><span class="muted">Workflow</span><span class="mono">{selectedRun.workflowName}</span></div>
          <div class="detail-item"><span class="muted">Environment</span><span>{selectedRun.environment}</span></div>
          <div class="detail-item"><span class="muted">Status</span><span>{selectedRun.status}</span></div>
          <div class="detail-item"><span class="muted">Version ID</span><span class="mono">{selectedRun.versionId ?? '-'}</span></div>
          <div class="detail-item"><span class="muted">Event ID</span><span class="mono">{selectedRun.eventId ?? '-'}</span></div>
          <div class="detail-item"><span class="muted">Routes Matched</span><span>{selectedRun.routesMatched}</span></div>
          <div class="detail-item"><span class="muted">Actions Executed</span><span>{selectedRun.actionsExecuted}</span></div>
          <div class="detail-item"><span class="muted">Duration</span><span>{selectedRun.durationMs}ms</span></div>
          <div class="detail-item"><span class="muted">Started At</span><span>{formatDateTime(selectedRun.startedAt)}</span></div>
        </div>
        {#if selectedRun.errors.length > 0}
          <div class="error-list">
            {#each selectedRun.errors as err, idx (idx)}
              <div class="error-item">{err}</div>
            {/each}
          </div>
        {/if}
      </div>
    {/if}

    <div class="section-title">Approval Queue</div>
    <div class="filters">
      <label class="field">
        Workflow
        <select class="input" bind:value={approvalFilterWorkflowId} disabled={loadingDefinitions}>
          <option value="">All workflows</option>
          {#each definitions as def (def.id)}
            <option value={def.id}>{def.name}</option>
          {/each}
        </select>
      </label>

      <label class="field">
        Environment
        <select class="input" bind:value={approvalFilterEnvironment}>
          <option value="">All environments</option>
          <option value="staging">staging</option>
          <option value="production">production</option>
        </select>
      </label>

      <label class="field">
        Status
        <select class="input" bind:value={approvalFilterStatus}>
          <option value="">all</option>
          <option value="pending">pending</option>
          <option value="approved">approved</option>
          <option value="rejected">rejected</option>
        </select>
      </label>
    </div>

    <div class="actions">
      <Button on:click={loadApprovals} loading={loadingApprovals}>
        {loadingApprovals ? 'Loading...' : 'Refresh Queue'}
      </Button>
      <Button
        variant="secondary"
        on:click={() => {
          clearApprovalFilters();
          void loadApprovals();
        }}
      >
        Clear Filters
      </Button>
    </div>

    {#if approvalsError}
      <div class="error-box" role="alert">{approvalsError}</div>
    {/if}

    {#if approvalRequests.length === 0}
      <div class="empty">No approval requests found for the current filter.</div>
    {:else}
      <div class="approval-table">
        <div class="approval-header">
          <span>Workflow</span>
          <span>Environment</span>
          <span>Status</span>
          <span>Version</span>
          <span>Requested By</span>
          <span>Reviewed By</span>
          <span>Comment</span>
          <span>Actions</span>
        </div>
        {#each approvalRequests as req (req.id)}
          {@const inFlight = !!approvalActionInFlightById[req.id]}
          <div class="approval-row">
            <span class="mono">{workflowNameFromID(req.workflowId)}</span>
            <span>{req.environment}</span>
            <span>
              <span
                class="status-pill"
                class:failed={req.status === 'rejected'}
                class:pending={req.status === 'pending'}>{req.status}</span
              >
            </span>
            <span class="mono">{shortValue(req.targetVersionId, 14)}</span>
            <span class="mono">{req.requestedBy}</span>
            <span class="mono">{req.reviewedBy ?? '-'}</span>
            <span>
              <input
                class="input comment-input"
                type="text"
                value={approvalCommentById[req.id] ?? req.comment ?? ''}
                disabled={req.status !== 'pending'}
                on:input={(e) => {
                  approvalCommentById = {
                    ...approvalCommentById,
                    [req.id]: (e.target as HTMLInputElement).value
                  };
                }}
                placeholder="Optional review comment"
              />
            </span>
            <span class="approval-actions">
              <Button
                size="sm"
                variant="secondary"
                on:click={() => runApprovalAction(req.id, 'approve')}
                disabled={req.status !== 'pending'}
                loading={inFlight}
              >
                Approve
              </Button>
              <Button
                size="sm"
                variant="danger"
                on:click={() => runApprovalAction(req.id, 'reject')}
                disabled={req.status !== 'pending'}
                loading={inFlight}
              >
                Reject
              </Button>
            </span>
          </div>
        {/each}
      </div>
    {/if}
  </div>
</Panel>

<style>
  .monitor {
    display: grid;
    gap: 12px;
  }

  .section-title {
    color: var(--color-text-tertiary);
    font-size: 0.8rem;
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    margin-top: 6px;
  }

  .connect-bar {
    display: flex;
    gap: 12px;
    align-items: flex-end;
    flex-wrap: wrap;
  }

  .filters {
    display: grid;
    grid-template-columns: repeat(5, minmax(140px, 1fr));
    gap: 10px;
  }

  .field {
    display: grid;
    gap: 6px;
    color: var(--color-text-tertiary);
    font-size: 0.85rem;
    font-weight: 700;
    min-width: 180px;
  }

  .input {
    padding: 8px 12px;
    border-radius: 10px;
    border: 1px solid var(--color-border-default);
    background: var(--color-bg-input);
    color: var(--color-text-primary);
    outline: none;
    transition: var(--transition-all);
    width: 100%;
    box-sizing: border-box;
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

  .actions {
    display: flex;
    gap: 8px;
    flex-wrap: wrap;
    align-items: center;
  }

  .status {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .indicator {
    width: 10px;
    height: 10px;
    border-radius: 50%;
    background: rgba(156, 163, 175, 0.5);
    transition: background 0.2s ease;
  }

  .indicator.connected {
    background: rgba(16, 185, 129, 0.85);
    box-shadow: 0 0 8px rgba(16, 185, 129, 0.4);
  }

  .indicator.error {
    background: rgba(239, 68, 68, 0.85);
    box-shadow: 0 0 8px rgba(239, 68, 68, 0.4);
  }

  .status-text {
    font-weight: 700;
    color: var(--color-text-secondary);
  }

  .status-text.error {
    color: rgba(254, 202, 202, 0.9);
  }

  .empty {
    color: var(--color-text-tertiary);
    padding: 20px;
    text-align: center;
    border: 1px dashed var(--color-border-subtle);
    border-radius: 12px;
  }

  .event-list {
    display: grid;
    gap: 6px;
    max-height: 300px;
    overflow-y: auto;
  }

  .event-row {
    display: grid;
    grid-template-columns: 80px auto 1fr auto auto;
    gap: 12px;
    align-items: center;
    padding: 8px 12px;
    border-radius: 8px;
    border: 1px solid var(--color-border-subtle);
    background: var(--color-bg-surface);
  }

  .event-row:hover {
    background: var(--color-bg-hover);
  }

  .time {
    color: var(--color-text-tertiary);
    font-size: 0.85rem;
  }

  .type-pill {
    padding: 2px 8px;
    border-radius: 999px;
    font-size: 0.75rem;
    font-weight: 700;
    text-transform: uppercase;
    white-space: nowrap;
    background: rgba(59, 130, 246, 0.15);
    border: 1px solid rgba(59, 130, 246, 0.3);
    color: rgba(147, 197, 253, 0.95);
  }

  .routes {
    color: var(--color-text-secondary);
    font-size: 0.85rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .actions-col {
    color: var(--color-text-tertiary);
    font-size: 0.85rem;
    white-space: nowrap;
  }

  .duration {
    font-size: 0.85rem;
    white-space: nowrap;
  }

  .footer {
    display: flex;
    justify-content: space-between;
    align-items: center;
    font-size: 0.85rem;
    padding-top: 8px;
    border-top: 1px solid var(--color-border-subtle);
  }

  .paused-badge {
    padding: 2px 8px;
    border-radius: 4px;
    background: rgba(245, 158, 11, 0.2);
    border: 1px solid rgba(245, 158, 11, 0.4);
    color: rgba(253, 230, 138, 0.95);
    font-weight: 700;
    font-size: 0.75rem;
  }

  .run-table {
    display: grid;
    border: 1px solid var(--color-border-subtle);
    border-radius: 10px;
    overflow: hidden;
  }

  .run-header,
  .run-row {
    display: grid;
    grid-template-columns: 180px 150px 90px 90px 70px 70px 90px 120px 100px;
    gap: 8px;
    align-items: center;
    padding: 8px 10px;
  }

  .run-header {
    font-size: 0.78rem;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--color-text-tertiary);
    background: var(--color-bg-elevated);
    border-bottom: 1px solid var(--color-border-subtle);
    font-weight: 700;
  }

  .run-row {
    background: var(--color-bg-surface);
    border-top: 1px solid var(--color-border-subtle);
    font-size: 0.85rem;
  }

  .run-row:hover {
    background: var(--color-bg-hover);
  }

  .status-pill {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    min-width: 64px;
    padding: 2px 8px;
    border-radius: 999px;
    font-size: 0.75rem;
    font-weight: 700;
    text-transform: lowercase;
    color: rgba(187, 247, 208, 0.95);
    background: rgba(16, 185, 129, 0.15);
    border: 1px solid rgba(16, 185, 129, 0.3);
  }

  .status-pill.failed {
    color: rgba(254, 202, 202, 0.95);
    background: rgba(239, 68, 68, 0.2);
    border: 1px solid rgba(239, 68, 68, 0.35);
  }

  .status-pill.pending {
    color: rgba(253, 230, 138, 0.95);
    background: rgba(245, 158, 11, 0.2);
    border: 1px solid rgba(245, 158, 11, 0.35);
  }

  .detail {
    display: grid;
    gap: 8px;
    padding: 10px;
    border-radius: 10px;
    border: 1px solid var(--color-border-default);
    background: var(--color-bg-surface);
  }

  .detail-title {
    color: var(--color-text-primary);
    font-weight: 700;
  }

  .detail-grid {
    display: grid;
    grid-template-columns: repeat(2, minmax(220px, 1fr));
    gap: 8px 12px;
  }

  .detail-item {
    display: grid;
    gap: 2px;
    font-size: 0.85rem;
  }

  .error-box,
  .error-list {
    padding: 8px 10px;
    border-radius: 8px;
    border: 1px solid var(--color-danger-border);
    background: var(--color-danger-bg);
    color: var(--color-danger-text);
    font-size: 0.85rem;
  }

  .error-list {
    display: grid;
    gap: 4px;
  }

  .approval-table {
    display: grid;
    border: 1px solid var(--color-border-subtle);
    border-radius: 10px;
    overflow: hidden;
  }

  .approval-header,
  .approval-row {
    display: grid;
    grid-template-columns: 150px 90px 90px 120px 120px 120px 1fr 170px;
    gap: 8px;
    align-items: center;
    padding: 8px 10px;
  }

  .approval-header {
    font-size: 0.78rem;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--color-text-tertiary);
    background: var(--color-bg-elevated);
    border-bottom: 1px solid var(--color-border-subtle);
    font-weight: 700;
  }

  .approval-row {
    background: var(--color-bg-surface);
    border-top: 1px solid var(--color-border-subtle);
    font-size: 0.85rem;
  }

  .approval-row:hover {
    background: var(--color-bg-hover);
  }

  .comment-input {
    min-width: 200px;
    padding: 6px 8px;
    border-radius: 8px;
    font-size: 0.8rem;
  }

  .approval-actions {
    display: flex;
    gap: 6px;
    flex-wrap: wrap;
    justify-content: flex-end;
  }

  .mono {
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  }

  .muted {
    color: var(--color-text-muted);
  }

  @media (max-width: 1200px) {
    .filters {
      grid-template-columns: repeat(3, minmax(160px, 1fr));
    }
  }

  @media (max-width: 900px) {
    .filters {
      grid-template-columns: 1fr;
    }

    .run-table {
      overflow-x: auto;
    }

    .run-header,
    .run-row {
      min-width: 980px;
    }

    .approval-table {
      overflow-x: auto;
    }

    .approval-header,
    .approval-row {
      min-width: 1160px;
    }

    .detail-grid {
      grid-template-columns: 1fr;
    }
  }
</style>
