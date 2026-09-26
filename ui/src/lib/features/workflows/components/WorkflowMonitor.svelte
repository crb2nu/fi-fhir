<script lang="ts">
  import { onDestroy, onMount } from 'svelte';
  import { subscribe as wsSubscribe } from '$lib/graphql/subscriptions';
  import { noteStreamError, streamStatus } from '$lib/graphql/streamAvailability';
  import { WorkflowEventsDocument, type WorkflowEventsSubscription } from '$lib/gen/graphql';
  import CircleAlert from '@lucide/svelte/icons/circle-alert';
  import MousePointerClick from '@lucide/svelte/icons/mouse-pointer-click';
  import Pause from '@lucide/svelte/icons/pause';
  import Play from '@lucide/svelte/icons/play';
  import RefreshCw from '@lucide/svelte/icons/refresh-cw';
  import StreamingUnavailable from '$lib/ui/StreamingUnavailable.svelte';
  import {
    Badge,
    Button,
    EmptyState,
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
  import type { KeyValueItem, SelectOption } from '$lib/ui/primitives';
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
  import { isErrorToasted } from '$lib/graphql/client';

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
  let selectedRunId: string | null = null;
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

  // The live section needs the `workflowEvents` subscription; when this
  // deployment cannot stream it, the section says so and never connects.
  // Run diagnostics and approvals below are plain queries and stay.
  const workflowEventsStatus = streamStatus('workflowEvents');
  $: liveUnavailable =
    $workflowEventsStatus.availability === 'unavailable' ? $workflowEventsStatus : null;
  $: if (liveUnavailable && unsubscribe) stopSubscription();

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
    if ($workflowEventsStatus.availability === 'unavailable') return;
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
          connected = false;
          if (noteStreamError('workflowEvents', err)) {
            liveError = null;
            return;
          }
          liveError = err.message;
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

  function selectRun(runID: string) {
    selectedRunId = runID;
    void loadRunDetail(runID);
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
      // Inline approvalsError carries the field context; only toast if the
      // global graphqlFetch net did not already (B4 dedupe).
      if (!isErrorToasted(err)) {
        toasts.error(message);
      }
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
    const date = new Date(ts);
    if (Number.isNaN(date.getTime())) return ts;
    const pad = (n: number) => String(n).padStart(2, '0');
    return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(
      date.getHours()
    )}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`;
  }

  const ENVIRONMENT_OPTIONS: SelectOption[] = [
    { value: '', label: 'All environments' },
    { value: 'staging', label: 'staging' },
    { value: 'production', label: 'production' }
  ];
  const RUN_STATUS_OPTIONS: SelectOption[] = [
    { value: '', label: 'All statuses' },
    { value: 'success', label: 'success' },
    { value: 'failed', label: 'failed' }
  ];
  const APPROVAL_STATUS_OPTIONS: SelectOption[] = [
    { value: '', label: 'All statuses' },
    { value: 'pending', label: 'pending' },
    { value: 'approved', label: 'approved' },
    { value: 'rejected', label: 'rejected' }
  ];

  function runDetailItems(run: NonNullable<GetWorkflowRunQuery['workflowRun']>): KeyValueItem[] {
    return [
      { key: 'Run id', value: run.id, mono: true, truncate: true },
      { key: 'Workflow', value: run.workflowName, mono: true },
      { key: 'Environment', value: run.environment },
      { key: 'Version id', value: run.versionId, mono: true, truncate: true },
      { key: 'Event id', value: run.eventId, mono: true, truncate: true },
      { key: 'Routes matched', value: run.routesMatched, mono: true },
      { key: 'Actions executed', value: run.actionsExecuted, mono: true },
      { key: 'Duration', value: `${run.durationMs} ms`, mono: true },
      { key: 'Started', value: formatDateTime(run.startedAt), mono: true }
    ];
  }

  // `defs` is passed from the template so the legacy-mode expression re-runs
  // when the definitions arrive after the approval requests.
  function workflowNameFromID(workflowID: string, defs = definitions): string {
    return defs.find((def) => def.id === workflowID)?.name ?? workflowID;
  }

  type StatusVariant = 'neutral' | 'success' | 'warning' | 'danger' | 'info';

  // Preserves prior styling: failed/rejected -> danger, pending -> warning,
  // any other value (success, approved, etc.) -> success.
  function runStatusVariant(status: string): StatusVariant {
    return status === 'failed' ? 'danger' : 'success';
  }

  function approvalStatusVariant(status: string): StatusVariant {
    if (status === 'rejected') return 'danger';
    if (status === 'pending') return 'warning';
    return 'success';
  }

  onDestroy(() => {
    stopSubscription();
  });
</script>

<div class="monitor">
  <Panel title="Live Stream" flush>
    {#if liveUnavailable}
      <div class="panel-inset">
        <StreamingUnavailable
          root="workflowEvents"
          subject="workflow events"
          reason={liveUnavailable.reason}
          alternative="Completed runs are listed under Run Diagnostics below."
        />
      </div>
    {:else}
      <div class="filters">
        <div class="filter-name">
          <Input
            mono
            bind:value={workflowName}
            placeholder="Workflow name, e.g. adt-routing"
            aria-label="Workflow name"
            onkeydown={(e) => e.key === 'Enter' && startSubscription()}
          />
        </div>
        <Button variant="primary" onclick={startSubscription} disabled={!workflowName.trim()}>
          {connected ? 'Reconnect' : 'Connect'}
        </Button>
        {#if connected}
          <Button variant="ghost" icon={paused ? Play : Pause} onclick={() => (paused = !paused)}>
            {paused ? 'Resume' : 'Pause'}
          </Button>
          <Button variant="ghost" onclick={clearEvents}>Clear</Button>
          <Button variant="ghost" onclick={stopSubscription}>Disconnect</Button>
        {/if}
        <div class="status" role="status" aria-live="polite">
          <span
            class="status-dot"
            class:is-connected={connected}
            class:is-error={!!liveError}
            aria-hidden="true"
          ></span>
          {#if liveError}
            <span class="status-text is-error">{liveError}</span>
          {:else if connected}
            <span class="status-text">Connected to <span class="text-mono">{workflowName}</span></span>
          {:else}
            <span class="status-text">Not connected</span>
          {/if}
          {#if paused}
            <Badge tone="warning">Paused</Badge>
          {/if}
        </div>
      </div>

      {#if events.length === 0}
        <EmptyState
          align="start"
          message={connected
            ? 'Waiting for workflow events.'
            : 'Enter a workflow name and connect to monitor live events.'}
        />
      {:else}
        <Table label="Live workflow events" class="events-table" layout="fixed">
          {#snippet head()}
            <tr>
              <Th width="104px">Time</Th>
              <Th width="200px">Type</Th>
              <Th>Routes</Th>
              <Th width="88px" numeric>Actions</Th>
              <Th width="96px" numeric>Duration</Th>
            </tr>
          {/snippet}
          {#each events as ev, i (i)}
            <Tr>
              <Td mono muted value={formatTimestamp(ev.event.timestamp)} />
              <Td><Badge mono>{ev.event.type}</Badge></Td>
              <Td mono truncate value={ev.routesMatched.join(', ') || '—'} />
              <Td numeric value={ev.actionsExecuted.length} />
              <Td numeric value={`${ev.duration} ms`} />
            </Tr>
          {/each}
        </Table>
        <div class="panel-foot text-mono">{events.length} events</div>
      {/if}
    {/if}
  </Panel>

  <Panel title="Run Diagnostics" flush>
    <div class="filters">
      <div class="filter-select">
        <Select
          aria-label="Workflow"
          bind:value={filterWorkflowName}
          disabled={loadingDefinitions}
        >
          <option value="">All workflows</option>
          {#each definitions as def (def.id)}
            <option value={def.name}>{def.name}</option>
          {/each}
        </Select>
      </div>
      <div class="filter-select">
        <Select aria-label="Environment" options={ENVIRONMENT_OPTIONS} bind:value={filterEnvironment} />
      </div>
      <div class="filter-select">
        <Select aria-label="Status" options={RUN_STATUS_OPTIONS} bind:value={filterStatus} />
      </div>
      <label class="filter-date">
        <span class="filter-date-label">From</span>
        <Input type="date" bind:value={filterFrom} />
      </label>
      <label class="filter-date">
        <span class="filter-date-label">To</span>
        <Input type="date" bind:value={filterTo} />
      </label>
      <Button onclick={loadRuns} loading={loadingRuns}>{loadingRuns ? 'Loading...' : 'Apply'}</Button>
      <Button
        variant="ghost"
        onclick={() => {
          clearRunFilters();
          void loadRuns();
        }}
      >
        Clear
      </Button>
      <span class="filter-count text-mono">{runs.length} runs</span>
    </div>

    {#if runsError}
      <p class="inline-error" role="alert">
        <Icon icon={CircleAlert} />
        <span>{runsError}</span>
      </p>
    {/if}

    {#if runs.length === 0 && !selectedRun}
      <EmptyState align="start" message="No workflow runs match the current filter." />
    {:else}
      <div class="split">
        {#if runs.length === 0}
          <EmptyState align="start" message="No workflow runs match the current filter." />
        {:else}
          <Table label="Workflow runs" class="runs-table" layout="fixed">
            {#snippet head()}
              <tr>
                <Th width="152px">Started</Th>
                <Th>Workflow</Th>
                <Th width="96px">Env</Th>
                <Th width="96px">Status</Th>
                <Th width="72px" numeric>Routes</Th>
                <Th width="72px" numeric>Actions</Th>
                <Th width="88px" numeric>Duration</Th>
                <Th width="132px">Version</Th>
              </tr>
            {/snippet}
            {#each runs as run (run.id)}
              <Tr selectable selected={selectedRunId === run.id} onselect={() => selectRun(run.id)}>
                <Td mono muted value={formatDateTime(run.startedAt)} />
                <Td mono truncate value={run.workflowName} />
                <Td value={run.environment} />
                <Td>
                  <Badge tone={runStatusVariant(run.status)} dot>{run.status}</Badge>
                </Td>
                <Td numeric value={run.routesMatched} />
                <Td numeric value={run.actionsExecuted} />
                <Td numeric value={`${run.durationMs} ms`} />
                <Td mono truncate muted value={run.versionId ?? '—'} />
              </Tr>
            {/each}
          </Table>
        {/if}

        <aside class="details" aria-label="Run detail" aria-busy={loadingSelectedRun}>
          {#if selectedRun}
            <div class="details-head">
              <span class="details-title text-mono" title={selectedRun.workflowName}
                >{selectedRun.workflowName}</span
              >
              <Badge tone={runStatusVariant(selectedRun.status)} dot>{selectedRun.status}</Badge>
            </div>
            <KeyValue items={runDetailItems(selectedRun)} />
            {#if selectedRun.errors.length > 0}
              <ul class="error-list">
                {#each selectedRun.errors as err, idx (idx)}
                  <li>{err}</li>
                {/each}
              </ul>
            {/if}
          {:else if loadingSelectedRun}
            <p class="details-note">Loading run...</p>
          {:else}
            <EmptyState icon={MousePointerClick} align="start" message="Select a run to see its detail." />
          {/if}
        </aside>
      </div>
    {/if}
  </Panel>

  <Panel title="Approval Queue" flush>
    <div class="filters">
      <div class="filter-select">
        <Select
          aria-label="Approval workflow"
          bind:value={approvalFilterWorkflowId}
          disabled={loadingDefinitions}
        >
          <option value="">All workflows</option>
          {#each definitions as def (def.id)}
            <option value={def.id}>{def.name}</option>
          {/each}
        </Select>
      </div>
      <div class="filter-select">
        <Select
          aria-label="Approval environment"
          options={ENVIRONMENT_OPTIONS}
          bind:value={approvalFilterEnvironment}
        />
      </div>
      <div class="filter-select">
        <Select
          aria-label="Approval status"
          options={APPROVAL_STATUS_OPTIONS}
          bind:value={approvalFilterStatus}
        />
      </div>
      <Button icon={RefreshCw} onclick={loadApprovals} loading={loadingApprovals}>
        {loadingApprovals ? 'Loading...' : 'Refresh queue'}
      </Button>
      <Button
        variant="ghost"
        onclick={() => {
          clearApprovalFilters();
          void loadApprovals();
        }}
      >
        Clear
      </Button>
      <span class="filter-count text-mono">{approvalRequests.length} requests</span>
    </div>

    {#if approvalsError}
      <p class="inline-error" role="alert">
        <Icon icon={CircleAlert} />
        <span>{approvalsError}</span>
      </p>
    {/if}

    {#if approvalRequests.length === 0}
      <EmptyState align="start" message="No approval requests match the current filter." />
    {:else}
      <Table label="Approval requests" class="approvals-table" layout="fixed">
        {#snippet head()}
          <tr>
            <Th width="160px">Workflow</Th>
            <Th width="96px">Env</Th>
            <Th width="96px">Status</Th>
            <Th width="132px">Version</Th>
            <Th width="120px">Requested by</Th>
            <Th width="120px">Reviewed by</Th>
            <Th>Comment</Th>
            <Th width="168px"><span class="sr-only">Actions</span></Th>
          </tr>
        {/snippet}
        {#each approvalRequests as req (req.id)}
          {@const inFlight = !!approvalActionInFlightById[req.id]}
          <Tr>
            <Td mono truncate value={workflowNameFromID(req.workflowId, definitions)} />
            <Td value={req.environment} />
            <Td>
              <Badge tone={approvalStatusVariant(req.status)} dot>{req.status}</Badge>
            </Td>
            <Td mono truncate muted value={req.targetVersionId} />
            <Td truncate value={req.requestedBy} />
            <Td truncate muted value={req.reviewedBy ?? '—'} />
            <Td>
              <Input
                value={approvalCommentById[req.id] ?? req.comment ?? ''}
                disabled={req.status !== 'pending'}
                aria-label={`Review comment for ${workflowNameFromID(req.workflowId, definitions)}`}
                oninput={(e) => {
                  approvalCommentById = {
                    ...approvalCommentById,
                    [req.id]: e.currentTarget.value
                  };
                }}
                placeholder="Optional review comment"
              />
            </Td>
            <Td>
              <span class="row-actions">
                <Button
                  onclick={() => runApprovalAction(req.id, 'approve')}
                  disabled={req.status !== 'pending'}
                  loading={inFlight}
                >
                  Approve
                </Button>
                <Button
                  variant="danger"
                  onclick={() => runApprovalAction(req.id, 'reject')}
                  disabled={req.status !== 'pending'}
                  loading={inFlight}
                >
                  Reject
                </Button>
              </span>
            </Td>
          </Tr>
        {/each}
      </Table>
    {/if}
  </Panel>
</div>

<style>
  .monitor {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    min-width: 0;
  }

  .panel-inset {
    padding: var(--panel-padding);
  }

  .filters {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2);
    padding: var(--space-2) var(--space-3);
    border-bottom: 1px solid var(--color-border-subtle);
  }

  .filter-name {
    width: 280px;
  }

  .filter-select {
    width: 160px;
  }

  .filter-date {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    width: 180px;
  }

  .filter-date-label {
    flex: 0 0 auto;
    font-size: var(--text-xs);
    color: var(--color-text-tertiary);
  }

  .filter-count {
    margin-left: auto;
    color: var(--color-text-tertiary);
  }

  .status {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    margin-left: auto;
    min-width: 0;
    font-size: var(--text-xs);
  }

  .status-dot {
    flex: 0 0 auto;
    width: 8px;
    height: 8px;
    border-radius: var(--radius-full);
    background: var(--color-text-muted);
  }

  .status-dot.is-connected {
    background: var(--color-success);
  }

  .status-dot.is-error {
    background: var(--color-danger);
  }

  .status-text {
    color: var(--color-text-secondary);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .status-text.is-error {
    color: var(--color-danger-text);
  }

  .monitor :global(.events-table) {
    max-height: 300px;
  }

  .panel-foot {
    padding: var(--space-2) var(--space-3);
    border-top: 1px solid var(--color-border-subtle);
    color: var(--color-text-tertiary);
  }

  .inline-error {
    display: flex;
    align-items: flex-start;
    gap: var(--space-2);
    margin: 0;
    padding: var(--space-2) var(--space-3);
    border-bottom: 1px solid var(--color-border-subtle);
    font-size: var(--text-xs);
    color: var(--color-danger-text);
    overflow-wrap: anywhere;
  }

  .split {
    display: grid;
    grid-template-columns: minmax(0, 1fr) 320px;
    min-height: 0;
  }

  .split :global(.runs-table) {
    max-height: 320px;
  }

  .details {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    min-width: 0;
    max-height: 320px;
    overflow: auto;
    padding: var(--space-3);
    border-left: 1px solid var(--color-border-subtle);
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
  }

  .details-note {
    margin: 0;
    font-size: var(--text-xs);
    color: var(--color-text-tertiary);
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

  .monitor :global(.approvals-table) {
    max-height: 360px;
  }

  .row-actions {
    display: inline-flex;
    gap: var(--space-1);
  }

  @media (max-width: 1100px) {
    .split {
      grid-template-columns: minmax(0, 1fr);
    }

    .details {
      max-height: none;
      border-left: 0;
      border-top: 1px solid var(--color-border-subtle);
    }
  }
</style>
