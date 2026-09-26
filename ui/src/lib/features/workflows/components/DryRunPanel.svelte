<script lang="ts">
  import { createEventDispatcher, onMount } from 'svelte';
  import CircleAlert from '@lucide/svelte/icons/circle-alert';
  import Play from '@lucide/svelte/icons/play';
  import TriangleAlert from '@lucide/svelte/icons/triangle-alert';
  import {
    Badge,
    Button,
    Field,
    Icon,
    KeyValue,
    Panel,
    Select,
    Table,
    Tabs,
    Td,
    Th,
    Tr
  } from '$lib/ui/primitives';
  import type { TabItem } from '$lib/ui/primitives';
  import CodeEditor from '$lib/ui/editor/CodeEditor.svelte';
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
  import { integrationSessionEngineEnabled } from '$lib/features/integration-session';
  import SessionPublicationPanel from './SessionPublicationPanel.svelte';
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
  let simulationSessions: SimulationSession[] = [];
  let selectedSessionId = '';
  let sessionLoading = false;
  let sessionLoadError = '';
  let sessionResult: SessionSimulation | null = null;
  let mounted = false;
  let sessionsRequested = false;

  // The Session source exists only when the session engine is really there
  // (build flag AND the API's integrationSessions capability); otherwise the
  // panel neither offers it nor loads integrationSessions on open.
  $: sessionEngineEnabled = $integrationSessionEngineEnabled;
  $: sourceOptions = sessionEngineEnabled
    ? [{ value: 'session' as EventSource, label: 'Session' }, ...legacySourceOptions]
    : legacySourceOptions;
  $: sourceTabs = sourceOptions.map((opt): TabItem => ({ id: opt.value, label: opt.label }));
  $: if (!sessionEngineEnabled && eventSource === 'session') eventSource = 'presets';
  $: if (mounted && sessionEngineEnabled && !sessionsRequested) {
    sessionsRequested = true;
    void loadSimulationSessions();
  }
  $: selectedSession = simulationSessions.find((session) => session.id === selectedSessionId);
  $: selectedSessionRunIds = selectedSession?.runs
    .filter((run) => run.status === 'completed' && run.events.length > 0)
    .map((run) => run.id) ?? [];
  $: selectedSessionEventCount = selectedSession?.runs
    .filter((run) => selectedSessionRunIds.includes(run.id))
    .reduce((count, run) => count + run.events.length, 0) ?? 0;
  $: selectedProfileRevisionIds = Array.from(new Set(
    selectedSession?.runs
      .filter((run) => selectedSessionRunIds.includes(run.id))
      .map((run) => run.profileRevisionId)
      .filter((revisionId): revisionId is string => Boolean(revisionId)) ?? []
  ));
  $: selectedProfileRevisionId = selectedProfileRevisionIds.length === 1 ? selectedProfileRevisionIds[0]! : '';

  onMount(() => {
    mounted = true;
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

  function selectSource(id: string) {
    const option = sourceOptions.find((opt) => opt.value === id);
    if (option) eventSource = option.value;
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

<Panel title="Dry run" flush>
  <div class="source-bar">
    <span class="source-label">Event source</span>
    <Tabs label="Event source" items={sourceTabs} value={eventSource} onchange={selectSource} />
  </div>

  <div class="dry-run">
    {#if eventSource === 'session'}
      <div class="session-source">
        <Field label="Integration session" id="simulation-session">
          <Select
            bind:value={selectedSessionId}
            disabled={sessionLoading || simulationSessions.length === 0}
          >
            {#if simulationSessions.length === 0}
              <option value="">{sessionLoading ? 'Loading sessions…' : 'No active sessions'}</option>
            {/if}
            {#each simulationSessions as session (session.id)}
              <option value={session.id}>{session.name}</option>
            {/each}
          </Select>
        </Field>
        <p class="muted-line">
          Uses immutable events from completed server runs. Action configuration is never executed or
          retained.
        </p>
        {#if sessionLoadError}
          <p class="inline-error" role="alert">
            <Icon icon={CircleAlert} />
            <span>{sessionLoadError}</span>
          </p>
        {/if}
      </div>
    {:else if eventSource === 'custom'}
      <div class="code-frame">
        <CodeEditor
          language="json"
          value={customEventJson}
          on:change={(e) => {
            customEventJson = e.detail;
          }}
          placeholder={customEventPlaceholder}
          height="150px"
        />
      </div>
      {#if customJsonError}
        <p class="inline-error" role="alert">
          <Icon icon={CircleAlert} />
          <span>{customJsonError}</span>
        </p>
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
            <span class="sample-type text-mono">{sample.event.type}</span>
          </label>
        {/each}
      </div>
    {:else if eventSource === 'debug'}
      <p class="source-status">
        {#if $debugSession}
          <span class="status-dot is-active" aria-hidden="true"></span>
          <span>Debug session active</span>
          <span class="text-mono muted">{$debugSession.id}</span>
        {:else}
          <span class="status-dot" aria-hidden="true"></span>
          <span>No active debug session</span>
        {/if}
      </p>
    {:else if eventSource === 'recent'}
      <p class="source-status">
        {#if $runtimeOutputState.entries.length > 0}
          <span class="status-dot is-active" aria-hidden="true"></span>
          <span>{Math.min(5, $runtimeOutputState.entries.length)} recent entries</span>
        {:else}
          <span class="status-dot" aria-hidden="true"></span>
          <span>No recent output entries</span>
        {/if}
      </p>
    {/if}

    <div class="run-row">
      <Button
        variant="primary"
        icon={Play}
        onclick={handleRun}
        loading={running}
        disabled={selectedEventCount === 0}
        title={runDisabledReason}
      >
        {running ? 'Running...' : 'Run simulation'}
      </Button>
      <span class="event-count">{selectedEventCount} event{selectedEventCount === 1 ? '' : 's'}</span>
    </div>

    {#if sessionResult}
      <div class="results">
        <KeyValue
          items={[
            {
              key: 'Workflow revision',
              value: sessionResult.workflowRevisionId,
              mono: true,
              truncate: true
            },
            { key: 'Digest', value: sessionResult.workflowRevisionDigest, mono: true, truncate: true },
            { key: 'Source runs', value: sessionResult.sourceRunIds.length, mono: true }
          ]}
        />

        <SessionPublicationPanel
          sessionId={sessionResult.sessionId}
          profileRevisionId={selectedProfileRevisionId}
          workflowSimulationId={sessionResult.id}
        />

        {#if sessionResult.delta}
          <div class="result-block" role="group" aria-label="Simulation delta">
            <h4 class="results-title">Changes from previous simulation</h4>
            <div class="delta-counts">
              <Badge tone="success" mono>+{sessionResult.delta.addedMatchedRoutes.length} routes</Badge>
              <Badge mono>−{sessionResult.delta.removedMatchedRoutes.length} routes</Badge>
              <span class="text-mono"
                >+{sessionResult.delta.addedTransforms.length}/−{sessionResult.delta.removedTransforms
                  .length} transforms</span
              >
              <span class="text-mono"
                >+{sessionResult.delta.addedActions.length}/−{sessionResult.delta.removedActions
                  .length} actions</span
              >
            </div>
          </div>
        {/if}

        <div class="result-block">
          <h4 class="results-title">Server-owned event traces</h4>
          <div class="trace-list">
            {#each sessionResult.events as event (`${event.runId}:${event.eventId}`)}
              <div class="trace">
                <div class="trace-head">
                  <span class="text-mono">{event.eventType}</span>
                  <span class="text-mono muted trace-id" title={event.eventId}>{event.eventId}</span>
                </div>
                {#each event.routes as route (route.name)}
                  <div class="trace-route">
                    <span class="text-mono trace-route-name">{route.name}</span>
                    <Badge tone={route.matched ? 'success' : 'neutral'} dot={route.matched}>
                      {route.matched ? 'Matched' : 'Skipped'}
                    </Badge>
                    {#if route.skipReason}<span class="muted">{route.skipReason}</span>{/if}
                    {#if route.transforms.length > 0 || route.actions.length > 0}
                      <span class="steps">
                        {#each route.transforms as transform (transform.index)}
                          <Badge mono>Transform {transform.index + 1}: {transform.type}</Badge>
                        {/each}
                        {#each route.actions as action (action.id)}
                          <Badge mono>Action: {action.type}</Badge>
                        {/each}
                      </span>
                    {/if}
                  </div>
                {/each}
              </div>
            {/each}
          </div>
        </div>
      </div>
    {:else if result}
      <div class="results">
        {#if result.validationErrors.length > 0}
          <div class="message-block is-error" role="alert">
            <h4 class="results-title">Validation errors</h4>
            <ul class="message-list">
              {#each result.validationErrors as err (err)}
                <li><Icon icon={CircleAlert} /><span>{err}</span></li>
              {/each}
            </ul>
          </div>
        {/if}

        {#if result.warnings.length > 0}
          <div class="message-block is-warning" role="alert">
            <h4 class="results-title">Warnings</h4>
            <ul class="message-list">
              {#each result.warnings as warn (warn)}
                <li><Icon icon={TriangleAlert} /><span>{warn}</span></li>
              {/each}
            </ul>
          </div>
        {/if}

        <div class="result-block">
          <h4 class="results-title">Route results</h4>
          <div class="table-frame">
            <Table label="Route results" layout="fixed">
              {#snippet head()}
                <tr>
                  <Th>Route</Th>
                  <Th width="96px">Matched</Th>
                  <Th width="80px" numeric>Actions</Th>
                  <Th>Skip reason</Th>
                </tr>
              {/snippet}
              {#each result.routeResults as rr (rr.routeName)}
                <Tr>
                  <Td mono truncate value={rr.routeName} />
                  <Td>
                    <Badge tone={rr.matched ? 'success' : 'neutral'} dot={rr.matched}>
                      {rr.matched ? 'Yes' : 'No'}
                    </Badge>
                  </Td>
                  <Td numeric value={rr.actionsWouldRun} />
                  <Td muted truncate value={rr.skipReason || '—'} />
                </Tr>
              {/each}
            </Table>
          </div>
        </div>
      </div>
    {/if}
  </div>
</Panel>

<style>
  .source-bar {
    display: flex;
    align-items: stretch;
    gap: var(--space-3);
    height: var(--toolbar-height);
    padding: 0 var(--space-3);
    border-bottom: 1px solid var(--color-border-subtle);
  }

  .source-label {
    align-self: center;
    font-size: var(--text-label);
    font-weight: var(--font-medium);
    letter-spacing: var(--tracking-label);
    text-transform: uppercase;
    color: var(--color-text-tertiary);
    white-space: nowrap;
  }

  .dry-run {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    padding: var(--panel-padding);
  }

  .session-source {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    max-width: 480px;
  }

  .muted-line,
  .source-status {
    margin: 0;
    font-size: var(--text-xs);
    color: var(--color-text-tertiary);
  }

  .source-status {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    color: var(--color-text-secondary);
  }

  .status-dot {
    width: 8px;
    height: 8px;
    border-radius: var(--radius-full);
    background: var(--color-text-muted);
  }

  .status-dot.is-active {
    background: var(--color-success);
  }

  .muted {
    color: var(--color-text-tertiary);
  }

  .code-frame {
    border: 1px solid var(--color-border-default);
    border-radius: var(--radius-sm);
    overflow: hidden;
  }

  .inline-error {
    display: flex;
    align-items: flex-start;
    gap: var(--space-2);
    margin: 0;
    font-size: var(--text-xs);
    color: var(--color-danger-text);
  }

  .sample-list {
    display: flex;
    flex-direction: column;
  }

  .sample-item {
    display: grid;
    grid-template-columns: 16px 200px minmax(0, 1fr);
    align-items: center;
    gap: var(--space-2);
    height: 28px;
    cursor: pointer;
  }

  .sample-item input {
    margin: 0;
    accent-color: var(--color-primary);
  }

  .sample-label {
    font-size: var(--text-ui);
    color: var(--color-text-primary);
  }

  .sample-type {
    color: var(--color-text-tertiary);
  }

  .run-row {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    padding-top: var(--space-3);
    border-top: 1px solid var(--color-border-subtle);
  }

  .event-count {
    font-family: var(--font-mono);
    font-size: var(--text-mono);
    color: var(--color-text-tertiary);
  }

  .results {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
  }

  .result-block {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  .results-title {
    margin: 0;
    font-size: var(--text-label);
    font-weight: var(--font-semibold);
    letter-spacing: var(--tracking-label);
    text-transform: uppercase;
    color: var(--color-text-tertiary);
  }

  .delta-counts {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2);
    color: var(--color-text-tertiary);
  }

  .trace-list {
    display: flex;
    flex-direction: column;
    border: 1px solid var(--color-border-subtle);
    border-radius: var(--radius-sm);
  }

  .trace + .trace {
    border-top: 1px solid var(--color-border-subtle);
  }

  .trace-head {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    height: 30px;
    padding: 0 var(--space-3);
    background: var(--color-bg-surface);
  }

  .trace-id {
    min-width: 0;
    margin-left: auto;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .trace-route {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2);
    min-height: 30px;
    padding: var(--space-1) var(--space-3) var(--space-1) var(--space-6);
    border-top: 1px solid var(--color-border-subtle);
    font-size: var(--text-xs);
  }

  .trace-route-name {
    color: var(--color-text-primary);
  }

  .steps {
    display: inline-flex;
    flex-wrap: wrap;
    gap: var(--space-1);
    margin-left: auto;
  }

  .message-block {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    padding: var(--space-2) var(--space-3);
    border: 1px solid var(--color-border-subtle);
    border-radius: var(--radius-sm);
  }

  .message-block.is-error {
    border-color: var(--color-danger-border);
    background: var(--color-danger-bg);
  }

  .message-block.is-warning {
    border-color: var(--color-warning-border);
    background: var(--color-warning-bg);
  }

  .message-list {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
    margin: 0;
    padding: 0;
    list-style: none;
    font-size: var(--text-xs);
    color: var(--color-text-primary);
  }

  .message-list li {
    display: flex;
    align-items: flex-start;
    gap: var(--space-2);
  }

  .message-block.is-error :global(.ui-icon) {
    color: var(--color-danger-text);
  }

  .message-block.is-warning :global(.ui-icon) {
    color: var(--color-warning-text);
  }

  .table-frame {
    border: 1px solid var(--color-border-subtle);
    border-radius: var(--radius-sm);
    overflow: hidden;
  }
</style>
