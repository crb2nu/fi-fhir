<script lang="ts">
  /**
   * DebugPanel Component
   *
   * Main debug container composing StepControls, BreakpointList,
   * VariableInspector, and step history into a unified debug UI.
   * Uses debugStore for state management.
   */
  import { onMount, onDestroy } from 'svelte';
  import StepControls from './StepControls.svelte';
  import BreakpointList from './BreakpointList.svelte';
  import VariableInspector from './VariableInspector.svelte';
  import EventLineage from './EventLineage.svelte';
  import {
    startSession,
    debugSession,
    sessionState,
    currentStep,
    breakpoints as breakpointsStore,
    stepHistory,
    eventLineage,
    updateSessionState,
    addStep,
    subscribeToSession,
    addBreakpoint as addBpToStore,
    replaceBreakpoint,
    removeBreakpoint as removeBpFromStore,
    endSession
  } from './debugStore';
  import {
    startDebugSession,
    debugStep,
    debugContinue,
    setBreakpoint,
    removeBreakpointApi,
    endDebugSession
  } from './debugApi';
  import type { BreakpointType } from './types';
  import { workflowDraft } from '$lib/features/workflows/workflowStore';
  import { draftToYaml } from '$lib/features/workflows/workflowYaml';
  import { validateWorkflowDraft, type WorkflowDraft } from '$lib/features/workflows/workflowTypes';
  import { get } from 'svelte/store';
  import CodeEditor from '$lib/ui/editor/CodeEditor.svelte';
  import StreamingUnavailable from '$lib/ui/StreamingUnavailable.svelte';
  import { streamStatus } from '$lib/graphql/streamAvailability';

  // Live step delivery needs the `debugStepEvent` subscription. Without it
  // the session still works — Step and Continue fetch each step on request —
  // so the panel says so rather than waiting on a stream that never opens.
  const debugStepStatus = streamStatus('debugStepEvent');
  $: liveStepsUnavailable =
    $debugStepStatus.availability === 'unavailable' ? $debugStepStatus : null;
  $: if (liveStepsUnavailable && unsubscribeSession) cleanupSubscription();

  let historyExpanded = false;
  let debugEventJson = '';
  let unsubscribeSession: (() => void) | null = null;

  function cleanupSubscription(): void {
    if (unsubscribeSession) {
      unsubscribeSession();
      unsubscribeSession = null;
    }
  }

  function buildDefaultDebugEvent(): Record<string, unknown> {
    const draft = get(workflowDraft);
    const firstRoute = draft.routes[0];

    return {
      id: 'debug-event',
      type: firstRoute?.filter.eventTypes[0] ?? 'PATIENT_ADMIT',
      source: firstRoute?.filter.sources[0] ?? 'debug-ui'
    };
  }

  // Starting a session needs a structurally valid draft and a JSON event.
  // Both are state, so Play is disabled with the reason beside it instead of
  // toasting after the click (.loom/22 B1/B2).
  $: playBlocker = playBlockedReason($workflowDraft, debugEventJson);

  function playBlockedReason(draft: WorkflowDraft, eventJson: string): string | null {
    const issues = validateWorkflowDraft(draft);
    if (issues.length > 0) {
      return `The workflow draft is not ready to debug: ${issues[0]}${issues.length > 1 ? ` (and ${issues.length - 1} more in Problems)` : ''}.`;
    }
    try {
      JSON.parse(eventJson);
    } catch {
      return 'The debug event must be valid JSON.';
    }
    return null;
  }

  onMount(() => {
    if (!debugEventJson) {
      debugEventJson = JSON.stringify(buildDefaultDebugEvent(), null, 2);
    }
  });

  onDestroy(() => {
    cleanupSubscription();
  });

  async function handlePlay(): Promise<void> {
    if (playBlocker) return;
    const draft = get(workflowDraft);
    const event: unknown = JSON.parse(debugEventJson);

    updateSessionState('running');
    const session = await startDebugSession(draftToYaml(draft), event);
    if (session) {
      startSession(session);
      cleanupSubscription();
      unsubscribeSession = subscribeToSession(session.id);
    }
  }

  async function handleStep(): Promise<void> {
    if (!$debugSession) return;
    const step = await debugStep($debugSession.id);
    if (step) {
      addStep(step);
      return;
    }
    updateSessionState('completed');
  }

  async function handleContinue(): Promise<void> {
    if (!$debugSession) return;
    updateSessionState('running');
    const step = await debugContinue($debugSession.id);
    if (step) {
      addStep(step);
      return;
    }
    updateSessionState('completed');
  }

  async function handleRestart(): Promise<void> {
    cleanupSubscription();
    if ($debugSession) {
      await endDebugSession($debugSession.id);
      endSession();
    }
    await handlePlay();
  }

  async function handleStop(): Promise<void> {
    cleanupSubscription();
    if ($debugSession) {
      await endDebugSession($debugSession.id);
    }
    endSession();
  }

  async function handleToggleBreakpoint(id: string): Promise<void> {
    // The breakpoint controls are disabled without a session (BreakpointList
    // says so on each control); this guard only covers a race.
    if (!$debugSession) return;

    const breakpoint = $breakpointsStore.find((entry) => entry.id === id);
    if (!breakpoint) return;

    if (breakpoint.enabled) {
      await removeBreakpointApi($debugSession.id, breakpoint.id);
      replaceBreakpoint(id, { ...breakpoint, enabled: false });
      return;
    }

    const next = await setBreakpoint($debugSession.id, breakpoint.type, breakpoint.name);
    replaceBreakpoint(id, next);
  }

  async function handleRemoveBreakpoint(id: string): Promise<void> {
    if ($debugSession) {
      const breakpoint = $breakpointsStore.find((entry) => entry.id === id);
      if (breakpoint?.enabled) {
        await removeBreakpointApi($debugSession.id, breakpoint.id);
      }
    }
    removeBpFromStore(id);
  }

  async function handleAddBreakpoint(detail: { type: BreakpointType; name: string }): Promise<void> {
    if (!$debugSession) return;

    const { type, name } = detail;
    const breakpoint = await setBreakpoint($debugSession.id, type, name);
    addBpToStore(breakpoint);
  }
</script>

<div class="debug-panel">
  <div class="debug-config">
    <div class="config-header">
      <span class="config-title">Debug Event Input</span>
      <button
        type="button"
        class="config-reset"
        on:click={() => {
          debugEventJson = JSON.stringify(buildDefaultDebugEvent(), null, 2);
        }}
      >
        Reset
      </button>
    </div>
    <CodeEditor
      language="json"
      value={debugEventJson}
      on:change={(e) => { debugEventJson = e.detail; }}
      height="120px"
    />
  </div>

  {#if liveStepsUnavailable}
    <StreamingUnavailable
      compact
      root="debugStepEvent"
      subject="debug steps"
      reason={liveStepsUnavailable.reason}
      alternative="Step and Continue still advance the session; each step is fetched on request."
    />
  {/if}

  <StepControls
    state={$sessionState}
    playBlockedReason={playBlocker}
    onPlay={handlePlay}
    onStep={handleStep}
    onContinue={handleContinue}
    onRestart={handleRestart}
    onStop={handleStop}
  />

  {#if playBlocker && $sessionState === 'idle'}
    <p class="play-blocked" data-testid="debug-play-blocked">Play is unavailable: {playBlocker}</p>
  {/if}

  <div class="debug-body">
    <aside class="debug-sidebar">
      <BreakpointList
        breakpoints={$breakpointsStore}
        hasSession={!!$debugSession}
        onToggle={handleToggleBreakpoint}
        onRemove={handleRemoveBreakpoint}
        onAdd={handleAddBreakpoint}
      />
    </aside>

    <main class="debug-main">
      <div class="inspector-section">
        <div class="section-header">
          <span class="section-title">Variables</span>
          {#if $currentStep}
            <span class="step-badge">
              Step {$currentStep.stepNumber}: {$currentStep.name}
            </span>
          {/if}
        </div>
        <VariableInspector variables={$currentStep?.variables ?? {}} />
      </div>
    </main>
  </div>

  <div class="debug-history">
    <button
      class="history-toggle"
      on:click={() => { historyExpanded = !historyExpanded; }}
      aria-expanded={historyExpanded}
    >
      <span class="toggle-icon" class:expanded={historyExpanded}>
        <svg viewBox="0 0 12 12" fill="currentColor" aria-hidden="true">
          <path d="M4 2l4 4-4 4" />
        </svg>
      </span>
      <span class="history-title">Step History</span>
      <span class="history-count">{$stepHistory.length}</span>
    </button>
    {#if historyExpanded}
      <div class="history-list">
        {#each $stepHistory as step (step.stepNumber)}
          <div class="history-item" class:current={step.stepNumber === $currentStep?.stepNumber}>
            <span class="history-step-num">{step.stepNumber}</span>
            <span class="history-kind {step.kind}">{step.kind}</span>
            <span class="history-name">{step.name}</span>
            <span class="history-span">{step.spanName}</span>
          </div>
        {/each}
      </div>
    {/if}
  </div>

  {#if $eventLineage.length > 0}
    <div class="debug-lineage">
      <div class="lineage-header">
        <span class="lineage-title">Event Lineage</span>
      </div>
      <EventLineage nodes={$eventLineage} />
    </div>
  {/if}
</div>

<style>
  .debug-panel {
    display: flex;
    flex-direction: column;
    border: 1px solid var(--color-border-subtle);
    border-radius: var(--panel-radius);
    background: var(--color-bg-elevated);
    overflow: hidden;
    box-shadow: var(--shadow-sm);
  }

  .play-blocked {
    margin: 0;
    padding: var(--space-2) var(--space-3);
    font-size: var(--text-xs);
    color: var(--color-text-tertiary);
    border-bottom: 1px solid var(--color-border-subtle);
  }

  .debug-config {
    display: grid;
    gap: var(--space-2);
    padding: var(--space-3);
    border-bottom: 1px solid var(--color-border-subtle);
    background: var(--color-bg-surface);
  }

  .config-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-2);
  }

  .config-title {
    font-size: var(--text-xs);
    font-weight: var(--font-bold);
    color: var(--color-text-primary);
    text-transform: uppercase;
    letter-spacing: var(--tracking-wider);
  }

  .config-reset {
    border: 1px solid var(--color-border-default);
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--color-text-secondary);
    padding: var(--space-1) var(--space-2);
    font: inherit;
    cursor: pointer;
  }

  .config-reset:hover {
    background: var(--color-bg-hover);
  }

  .debug-body {
    display: flex;
    min-height: 200px;
  }

  .debug-sidebar {
    width: 250px;
    flex-shrink: 0;
    border-right: 1px solid var(--color-border-subtle);
    overflow: auto;
  }

  .debug-main {
    flex: 1;
    min-width: 0;
    overflow: auto;
  }

  .inspector-section {
    display: flex;
    flex-direction: column;
  }

  .section-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: var(--space-2) var(--space-3);
    border-bottom: 1px solid var(--color-border-subtle);
  }

  .section-title {
    font-size: var(--text-xs);
    font-weight: var(--font-bold);
    color: var(--color-text-primary);
    text-transform: uppercase;
    letter-spacing: var(--tracking-wider);
  }

  .step-badge {
    font-size: var(--text-2xs);
    font-family: var(--font-mono);
    color: var(--color-primary);
    background: var(--color-primary-muted);
    padding: 2px var(--space-2);
    border-radius: var(--radius-sm);
  }

  /* History section */
  .debug-history {
    border-top: 1px solid var(--color-border-subtle);
  }

  .history-toggle {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    width: 100%;
    padding: var(--space-2) var(--space-3);
    background: none;
    border: none;
    cursor: pointer;
    font-family: inherit;
    transition: var(--transition-colors);
  }

  .history-toggle:hover {
    background: var(--color-bg-hover);
  }

  .history-toggle:focus-visible {
    outline: none;
    box-shadow: var(--shadow-focus);
  }

  .toggle-icon {
    display: flex;
    width: 12px;
    height: 12px;
    color: var(--color-text-muted);
    transition: transform var(--duration-fast) var(--ease-out);
    flex-shrink: 0;
  }

  .toggle-icon.expanded {
    transform: rotate(90deg);
  }

  .toggle-icon svg {
    width: 100%;
    height: 100%;
  }

  .history-title {
    font-size: var(--text-xs);
    font-weight: var(--font-bold);
    color: var(--color-text-primary);
    text-transform: uppercase;
    letter-spacing: var(--tracking-wider);
  }

  .history-count {
    font-size: var(--text-2xs);
    font-weight: var(--font-bold);
    color: var(--color-text-muted);
    background: var(--color-bg-surface);
    padding: 1px var(--space-1);
    border-radius: var(--radius-full);
    min-width: 18px;
    text-align: center;
  }

  .history-list {
    border-top: 1px solid var(--color-border-subtle);
  }

  .history-item {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    padding: var(--space-1) var(--space-3);
    min-height: 28px;
    border-bottom: 1px solid var(--color-border-subtle);
    font-size: var(--text-xs);
  }

  .history-item:last-child {
    border-bottom: none;
  }

  .history-item.current {
    background: var(--color-primary-muted);
  }

  .history-step-num {
    font-family: var(--font-mono);
    font-weight: var(--font-bold);
    color: var(--color-text-muted);
    min-width: 20px;
    flex-shrink: 0;
  }

  .history-kind {
    font-size: var(--text-2xs);
    font-weight: var(--font-medium);
    padding: 1px var(--space-1);
    border-radius: var(--radius-sm);
    white-space: nowrap;
    flex-shrink: 0;
  }

  .history-kind.route {
    color: var(--color-primary);
    background: var(--color-primary-muted);
  }

  .history-kind.action {
    color: var(--color-success-text);
    background: var(--color-success-bg);
  }

  .history-kind.transform {
    color: var(--color-warning-text);
    background: var(--color-warning-bg);
  }

  .history-name {
    font-family: var(--font-mono);
    color: var(--color-text-secondary);
    flex: 1;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .history-span {
    font-family: var(--font-mono);
    font-size: var(--text-2xs);
    color: var(--color-text-muted);
    flex-shrink: 0;
  }

  /* Event lineage section */
  .debug-lineage {
    border-top: 1px solid var(--color-border-subtle);
    padding: var(--space-2) var(--space-3);
  }

  .lineage-header {
    padding-bottom: var(--space-2);
  }

  .lineage-title {
    font-size: var(--text-xs);
    font-weight: var(--font-bold);
    color: var(--color-text-primary);
    text-transform: uppercase;
    letter-spacing: var(--tracking-wider);
  }
</style>
