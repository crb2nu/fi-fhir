<script lang="ts">
  import { createEventDispatcher, onMount } from 'svelte';
  import Panel from '$lib/ui/Panel.svelte';
  import CodeEditor from '$lib/ui/editor/CodeEditor.svelte';
  import Button from '$lib/ui/Button.svelte';
  import Badge from '$lib/ui/Badge.svelte';
  import { workflowDraft } from '../workflowStore';
  import { draftToYaml } from '../workflowYaml';
  import {
    dryRunWorkflow,
    fetchWorkflowSimulationSessions,
    saveSessionWorkflowDraft,
    simulateSessionWorkflow
  } from '../workflowApi';
  import { customEventsJsonError } from '../dryRunValidation';
  import { toasts } from '$lib/ui/toastStore';
  import { isErrorToasted } from '$lib/graphql/client';
  import { debugSession } from '$lib/features/debug/debugStore';
  import { runtimeOutputState } from '$lib/ui/ide/panels/runtimeOutputStore';
  import { isIntegrationSessionEngineEnabled } from '$lib/features/integration-session';
  import type {
    DryRunWorkflowMutation,
    ListWorkflowSimulationSessionsQuery,
    SimulateSessionWorkflowMutation
  } from '$lib/gen/graphql';

  type DryRunResult = DryRunWorkflowMutation['dryRunWorkflow'];
  type SessionSimulation = SimulateSessionWorkflowMutation['simulateSessionWorkflow'];
  type SimulationSession = ListWorkflowSimulationSessionsQuery['integrationSessions'][number];
  type EventSource = 'session' | 'presets' | 'debug' | 'recent' | 'custom';

  const presetEvents = [
    {
      label: 'Patient Admit',
      event: { type: 'PATIENT_ADMIT', source: 'epic', id: 'sample-1', isCritical: false }
    },
    {
      label: 'Lab Result (Critical)',
      event: { type: 'LAB_RESULT', source: 'epic', id: 'sample-2', isCritical: true }
    },
    {
      label: 'Patient Discharge',
      event: { type: 'PATIENT_DISCHARGE', source: 'cerner', id: 'sample-3', isCritical: false }
    },
    {
      label: 'Appointment Scheduled',
      event: { type: 'APPOINTMENT_SCHEDULED', source: 'epic', id: 'sample-4', isCritical: false }
    }
  ];

  const legacySourceOptions: { value: EventSource; label: string }[] = [
    { value: 'presets', label: 'Presets' },
    { value: 'debug', label: 'Debug Session' },
    { value: 'recent', label: 'Recent Output' },
    { value: 'custom', label: 'Custom JSON' },
  ];

  let eventSource: EventSource = 'presets';
  let selectedPresets = [0];
  let customEventJson = '';
  const customEventPlaceholder = '[{ "type": "PATIENT_ADMIT", "source": "epic" }]';
  let running = false;
  let result: DryRunResult | null = null;
  const sessionEngineEnabled = isIntegrationSessionEngineEnabled();
  let simulationSessions: SimulationSession[] = [];
  let selectedSessionId = '';
  let sessionLoading = false;
  let sessionLoadError = '';
  let sessionResult: SessionSimulation | null = null;

  const sourceOptions = sessionEngineEnabled
    ? [{ value: 'session' as EventSource, label: 'Session' }, ...legacySourceOptions]
    : legacySourceOptions;
  $: selectedSession = simulationSessions.find((session) => session.id === selectedSessionId);
  $: selectedSessionRunIds = selectedSession?.runs
    .filter((run) => run.status === 'completed' && run.events.length > 0)
    .map((run) => run.id) ?? [];
  $: selectedSessionEventCount = selectedSession?.runs
    .filter((run) => selectedSessionRunIds.includes(run.id))
    .reduce((count, run) => count + run.events.length, 0) ?? 0;

  onMount(() => {
    if (sessionEngineEnabled) void loadSimulationSessions();
  });

  async function loadSimulationSessions() {
    sessionLoading = true;
    sessionLoadError = '';
    try {
      const data = await fetchWorkflowSimulationSessions();
      simulationSessions = data.integrationSessions.filter((session) => !session.archived);
      if (!simulationSessions.some((session) => session.id === selectedSessionId)) {
        selectedSessionId = simulationSessions[0]?.id ?? '';
      }
    } catch (e) {
      sessionLoadError = 'Could not load integration sessions';
      if (!isErrorToasted(e)) toasts.error(sessionLoadError);
    } finally {
      sessionLoading = false;
    }
  }

  function togglePreset(index: number) {
    if (selectedPresets.includes(index)) {
      selectedPresets = selectedPresets.filter((i) => i !== index);
    } else {
      selectedPresets = [...selectedPresets, index];
    }
  }

  function toEventPayload(entry: { title: string; source: string; kind: string }): Record<string, unknown> {
    return { type: entry.title.toUpperCase().replace(/\s+/g, '_'), source: entry.source, kind: entry.kind };
  }

  function parseCustomJson(json: string): unknown[] {
    try {
      const parsed = JSON.parse(json);
      return Array.isArray(parsed) ? parsed : [parsed];
    } catch {
      return [];
    }
  }

  $: resolvedEvents = (() => {
    if (eventSource === 'debug') {
      const session = $debugSession;
      if (!session || session.steps.length === 0) return [];
      const firstStep = session.steps[0];
      const ev = firstStep?.variables?.['event'];
      if (ev && typeof ev === 'object') return [ev];
      return [firstStep?.variables ?? {}];
    }
    if (eventSource === 'recent') {
      return $runtimeOutputState.entries.slice(0, 5).map(toEventPayload);
    }
    if (eventSource === 'custom') {
      return parseCustomJson(customEventJson);
    }
    // presets
    return selectedPresets.map((i) => presetEvents[i]!.event);
  })();

  // Live inline validation for the custom-JSON field (persistent until fixed),
  // plus an explanatory reason for the disabled Run button (.loom/22 B1/B2/D2).
  $: customJsonError = customEventsJsonError(eventSource, customEventJson);
  $: selectedEventCount = eventSource === 'session' ? selectedSessionEventCount : resolvedEvents.length;
  $: runDisabledReason =
    selectedEventCount > 0
      ? undefined
      : eventSource === 'session'
        ? 'Select a session with at least one completed run'
      : customJsonError
        ? 'Fix the custom event JSON before running'
        : 'Add or select at least one event to run';

  const dispatch = createEventDispatcher<{
    result: DryRunResult | null;
  }>();

  async function handleRun() {
    // The Run button is disabled whenever no events resolve (which includes
    // invalid custom JSON, since parseCustomJson yields []), and the reason is
    // shown inline + in the button tooltip — so the old post-click validation
    // toasts were unreachable backstops. Keep a defensive guard, no toast.
    if (selectedEventCount === 0) return;

    running = true;
    result = null;
    sessionResult = null;
    dispatch('result', null);

    try {
      const yamlStr = draftToYaml($workflowDraft);
      if (eventSource === 'session' && selectedSession) {
        const draft = await saveSessionWorkflowDraft(selectedSession.id, yamlStr);
        const baseline = [...selectedSession.workflowSimulations]
          .reverse()
          .find((simulation) =>
            simulation.sourceRunIds.length === selectedSessionRunIds.length &&
            simulation.sourceRunIds.every((runId, index) => runId === selectedSessionRunIds[index])
          );
        const data = await simulateSessionWorkflow({
          sessionId: selectedSession.id,
          workflowRevisionId: draft.updateSessionWorkflowDraft.revisionId,
          sourceRunIds: selectedSessionRunIds,
          baselineSimulationId: baseline?.id ?? null
        });
        sessionResult = data.simulateSessionWorkflow;
        await loadSimulationSessions();
        return;
      }
      const data = await dryRunWorkflow(yamlStr, resolvedEvents);
      result = data.dryRunWorkflow;
      dispatch('result', result);
    } catch (e) {
      // Global graphqlFetch net already toasts graphql failures (B4 dedupe); a
      // local draftToYaml throw is not toasted by the net, so still surface it.
      if (!isErrorToasted(e)) {
        toasts.error('Dry run failed');
      }
    } finally {
      running = false;
    }
  }
</script>

<Panel title="Dry Run Simulation">
  <div class="dry-run">
    <div class="event-selector">
      <div class="selector-header">
        <span class="label">Event Source</span>
        <div class="source-tabs" role="tablist">
          {#each sourceOptions as opt (opt.value)}
            <button
              type="button"
              class="source-tab"
              class:active={eventSource === opt.value}
              role="tab"
              aria-selected={eventSource === opt.value}
              on:click={() => { eventSource = opt.value; }}
            >
              {opt.label}
            </button>
          {/each}
        </div>
      </div>

      {#if eventSource === 'session'}
        <div class="session-source">
          <label class="session-label" for="simulation-session">Integration session</label>
          <select
            id="simulation-session"
            bind:value={selectedSessionId}
            disabled={sessionLoading || simulationSessions.length === 0}
          >
            {#if simulationSessions.length === 0}
              <option value="">{sessionLoading ? 'Loading sessions…' : 'No active sessions'}</option>
            {/if}
            {#each simulationSessions as session (session.id)}
              <option value={session.id}>{session.name}</option>
            {/each}
          </select>
          <span class="source-detail">
            Uses immutable events from completed server runs. Action configuration is never executed or retained.
          </span>
          {#if sessionLoadError}
            <div class="custom-json-error" role="alert">{sessionLoadError}</div>
          {/if}
        </div>
      {:else if eventSource === 'custom'}
        <CodeEditor
          language="json"
          value={customEventJson}
          on:change={(e) => { customEventJson = e.detail; }}
          placeholder={customEventPlaceholder}
          height="150px"
        />
        {#if customJsonError}
          <div class="custom-json-error" role="alert">{customJsonError}</div>
        {/if}
      {:else if eventSource === 'presets'}
        <div class="sample-list">
          {#each presetEvents as sample, i (i)}
            <label class="sample-item">
              <input
                type="checkbox"
                checked={selectedPresets.includes(i)}
                on:change={() => togglePreset(i)}
              />
              <span class="sample-label">{sample.label}</span>
              <span class="sample-type mono">{sample.event.type}</span>
            </label>
          {/each}
        </div>
      {:else if eventSource === 'debug'}
        <div class="source-info">
          {#if $debugSession}
            <span class="source-status active">Debug session active</span>
            <span class="source-detail mono">{$debugSession.id}</span>
          {:else}
            <span class="source-status">No active debug session</span>
          {/if}
        </div>
      {:else if eventSource === 'recent'}
        <div class="source-info">
          {#if $runtimeOutputState.entries.length > 0}
            <span class="source-status active">{Math.min(5, $runtimeOutputState.entries.length)} recent entries</span>
          {:else}
            <span class="source-status">No recent output entries</span>
          {/if}
        </div>
      {/if}

      <div class="run-row">
        <Button
          on:click={handleRun}
          loading={running}
          disabled={selectedEventCount === 0}
          title={runDisabledReason}
        >
          {running ? 'Running...' : 'Run Simulation'}
        </Button>
        <span class="event-count">{selectedEventCount} event{selectedEventCount === 1 ? '' : 's'}</span>
      </div>
    </div>

    {#if sessionResult}
      <div class="results session-results">
        <div class="simulation-provenance">
          <div>
            <span class="provenance-label">Workflow revision</span>
            <span class="mono">{sessionResult.workflowRevisionId}</span>
          </div>
          <div>
            <span class="provenance-label">Digest</span>
            <span class="mono">{sessionResult.workflowRevisionDigest}</span>
          </div>
          <div>
            <span class="provenance-label">Source runs</span>
            <span>{sessionResult.sourceRunIds.length}</span>
          </div>
        </div>

        {#if sessionResult.delta}
          <div class="delta-summary" aria-label="Simulation delta">
            <h4 class="results-title">Changes from previous simulation</h4>
            <div class="delta-counts">
              <Badge variant="success" size="sm">
                +{sessionResult.delta.addedMatchedRoutes.length} routes
              </Badge>
              <Badge variant="default" size="sm">
                −{sessionResult.delta.removedMatchedRoutes.length} routes
              </Badge>
              <span>+{sessionResult.delta.addedTransforms.length}/−{sessionResult.delta.removedTransforms.length} transforms</span>
              <span>+{sessionResult.delta.addedActions.length}/−{sessionResult.delta.removedActions.length} actions</span>
            </div>
          </div>
        {/if}

        <h4 class="results-title">Server-owned event traces</h4>
        <div class="trace-list">
          {#each sessionResult.events as event (`${event.runId}:${event.eventId}`)}
            <article class="event-trace">
              <header class="trace-header">
                <span class="mono">{event.eventType}</span>
                <span class="muted">{event.eventId}</span>
              </header>
              {#each event.routes as route (route.name)}
                <div class="route-trace">
                  <div class="route-heading">
                    <span class="mono">{route.name}</span>
                    <Badge variant={route.matched ? 'success' : 'default'} size="sm">
                      {route.matched ? 'Matched' : 'Skipped'}
                    </Badge>
                    {#if route.skipReason}<span class="muted">{route.skipReason}</span>{/if}
                  </div>
                  {#if route.transforms.length > 0 || route.actions.length > 0}
                    <div class="planned-steps">
                      {#each route.transforms as transform (transform.index)}
                        <span class="step-chip">Transform {transform.index + 1}: {transform.type}</span>
                      {/each}
                      {#each route.actions as action (action.id)}
                        <span class="step-chip">Action: {action.type}</span>
                      {/each}
                    </div>
                  {/if}
                </div>
              {/each}
            </article>
          {/each}
        </div>
      </div>
    {:else if result}
      <div class="results">
        {#if result.validationErrors.length > 0}
          <div class="errors" role="alert">
            <h4 class="results-title">Validation Errors</h4>
            {#each result.validationErrors as err (err)}
              <div class="error-item">{err}</div>
            {/each}
          </div>
        {/if}

        {#if result.warnings.length > 0}
          <div class="warnings" role="alert">
            <h4 class="results-title">Warnings</h4>
            {#each result.warnings as warn (warn)}
              <div class="warning-item">{warn}</div>
            {/each}
          </div>
        {/if}

        <h4 class="results-title">Route Results</h4>
        <div class="results-table">
          <div class="table-header">
            <span>Route</span>
            <span>Matched</span>
            <span>Actions</span>
            <span>Skip Reason</span>
          </div>
          {#each result.routeResults as rr (rr.routeName)}
            <div class="table-row">
              <span class="mono">{rr.routeName}</span>
              <span>
                <Badge variant={rr.matched ? 'success' : 'default'} size="sm">
                  {rr.matched ? 'Yes' : 'No'}
                </Badge>
              </span>
              <span>{rr.actionsWouldRun}</span>
              <span class="muted">{rr.skipReason || '—'}</span>
            </div>
          {/each}
        </div>
      </div>
    {/if}
  </div>
</Panel>

<style>
  .dry-run {
    display: grid;
    gap: 16px;
  }

  .event-selector {
    display: grid;
    gap: 10px;
  }

  .selector-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
  }

  .label {
    color: var(--color-text-tertiary);
    font-weight: 700;
    font-size: 0.9rem;
  }

  .mono {
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  }

  .source-tabs {
    display: flex;
    gap: 2px;
    background: var(--color-bg-surface);
    border: 1px solid var(--color-border-subtle);
    border-radius: 8px;
    padding: 2px;
  }

  .source-tab {
    padding: 4px 10px;
    border: none;
    border-radius: 6px;
    background: transparent;
    color: var(--color-text-tertiary);
    font-size: 0.8rem;
    font-weight: 600;
    cursor: pointer;
    transition: var(--transition-all);
  }

  .source-tab:hover {
    color: var(--color-text-secondary);
    background: var(--color-bg-hover);
  }

  .source-tab:focus-visible {
    outline: none;
    box-shadow: var(--shadow-focus);
    border-radius: var(--radius-sm);
  }

  .source-tab.active {
    color: var(--color-text-primary);
    background: var(--color-bg-elevated);
    box-shadow: var(--shadow-sm);
  }

  .source-info {
    display: grid;
    gap: 4px;
    padding: 8px 0;
  }

  .session-source {
    display: grid;
    gap: 6px;
  }

  .session-label,
  .provenance-label {
    color: var(--color-text-muted);
    font-size: 0.75rem;
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }

  .session-source select {
    width: 100%;
    padding: 8px 10px;
    border: 1px solid var(--color-border-subtle);
    border-radius: 6px;
    background: var(--color-bg-surface);
    color: var(--color-text-primary);
  }

  .source-status {
    font-size: 0.85rem;
    color: var(--color-text-muted);
  }

  .source-status.active {
    color: var(--color-success-text, var(--color-success));
  }

  .source-detail {
    font-size: 0.8rem;
    color: var(--color-text-tertiary);
  }

  .run-row {
    display: flex;
    align-items: center;
    gap: 10px;
  }

  .event-count {
    font-size: 0.8rem;
    color: var(--color-text-muted);
  }

  .sample-list {
    display: grid;
    gap: 4px;
  }

  .sample-item {
    display: flex;
    align-items: center;
    gap: 8px;
    cursor: pointer;
    padding: 4px 0;
  }

  .sample-item input {
    accent-color: rgba(59, 130, 246, 0.85);
  }

  .sample-label {
    color: var(--color-text-secondary);
    font-size: 0.9rem;
  }

  .sample-type {
    color: var(--color-text-muted);
    font-size: 0.8rem;
  }

  .results {
    display: grid;
    gap: 12px;
  }

  .simulation-provenance {
    display: grid;
    grid-template-columns: minmax(0, 1fr) minmax(0, 2fr) auto;
    gap: 12px;
    padding: 10px;
    border: 1px solid var(--color-border-subtle);
    border-radius: 8px;
    background: var(--color-bg-surface);
  }

  .simulation-provenance > div {
    display: grid;
    min-width: 0;
    gap: 3px;
  }

  .simulation-provenance .mono {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: var(--color-text-secondary);
    font-size: 0.8rem;
  }

  .delta-summary,
  .trace-list {
    display: grid;
    gap: 8px;
  }

  .delta-counts,
  .planned-steps,
  .route-heading,
  .trace-header {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 8px;
  }

  .delta-counts {
    color: var(--color-text-muted);
    font-size: 0.8rem;
  }

  .event-trace {
    display: grid;
    gap: 8px;
    padding: 10px;
    border: 1px solid var(--color-border-subtle);
    border-radius: 8px;
    background: var(--color-bg-surface);
  }

  .trace-header {
    justify-content: space-between;
  }

  .route-trace {
    display: grid;
    gap: 6px;
    padding-top: 8px;
    border-top: 1px solid var(--color-border-subtle);
  }

  .step-chip {
    padding: 3px 7px;
    border-radius: 999px;
    background: var(--color-bg-elevated);
    color: var(--color-text-tertiary);
    font-size: 0.75rem;
  }

  .results-title {
    color: var(--color-text-tertiary);
    font-size: 0.85rem;
    font-weight: 700;
    margin: 0;
  }

  .custom-json-error {
    margin-top: 6px;
    padding: 6px 10px;
    border-radius: 6px;
    background: var(--color-danger-bg);
    border: 1px solid var(--color-danger-border);
    color: var(--color-danger-text);
    font-size: 0.85rem;
  }

  .errors {
    display: grid;
    gap: 4px;
  }

  .error-item {
    padding: 6px 10px;
    border-radius: 6px;
    background: var(--color-danger-bg);
    border: 1px solid var(--color-danger-border);
    color: var(--color-text-primary);
    font-size: 0.85rem;
  }

  .warnings {
    display: grid;
    gap: 4px;
  }

  .warning-item {
    padding: 6px 10px;
    border-radius: 6px;
    background: var(--color-warning-bg);
    border: 1px solid var(--color-warning-border);
    color: var(--color-text-primary);
    font-size: 0.85rem;
  }

  .results-table {
    display: grid;
    gap: 4px;
  }

  .table-header {
    display: grid;
    grid-template-columns: 1fr 80px 80px 1fr;
    gap: 12px;
    padding: 6px 10px;
    color: var(--color-text-muted);
    font-size: 0.75rem;
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 0.05em;
  }

  .table-row {
    display: grid;
    grid-template-columns: 1fr 80px 80px 1fr;
    gap: 12px;
    padding: 8px 10px;
    border-radius: 6px;
    border: 1px solid var(--color-border-subtle);
    background: var(--color-bg-surface);
    align-items: center;
    font-size: 0.9rem;
    color: var(--color-text-secondary);
  }

  .muted {
    color: var(--color-text-muted);
    font-size: 0.85rem;
  }

  @media (max-width: 640px) {
    .simulation-provenance {
      grid-template-columns: 1fr;
    }

    .table-header,
    .table-row {
      grid-template-columns: 1fr 1fr;
    }

    .table-header span:nth-child(4),
    .table-row span:nth-child(4) {
      display: none;
    }
  }
</style>
