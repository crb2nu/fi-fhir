<script lang="ts">
  /**
   * DebugPanel Component
   *
   * Main debug container composing StepControls, BreakpointList,
   * VariableInspector, and step history into a unified debug UI.
   * Uses debugStore for state management.
   */
  import { onMount } from 'svelte';
  import StepControls from './StepControls.svelte';
  import BreakpointList from './BreakpointList.svelte';
  import VariableInspector from './VariableInspector.svelte';
  import {
    debugSession,
    sessionState,
    currentStep,
    breakpoints as breakpointsStore,
    stepHistory,
    loadMockData,
    updateSessionState,
    addStep,
    addBreakpoint as addBpToStore,
    removeBreakpoint as removeBpFromStore,
    toggleBreakpoint as toggleBpInStore,
    endSession
  } from './debugStore';
  import {
    startDebugSession,
    debugStep,
    debugContinue,
    endDebugSession
  } from './debugApi';
  import type { BreakpointType } from './types';

  let historyExpanded = false;

  onMount(() => {
    if (!$debugSession) {
      loadMockData();
    }
  });

  async function handlePlay(): Promise<void> {
    updateSessionState('running');
    const session = await startDebugSession('', {});
    if (session) {
      debugSession.set(session);
    }
  }

  async function handleStep(): Promise<void> {
    if (!$debugSession) return;
    const step = await debugStep($debugSession.id);
    if (step) {
      addStep(step);
    }
  }

  async function handleContinue(): Promise<void> {
    if (!$debugSession) return;
    updateSessionState('running');
    const step = await debugContinue($debugSession.id);
    if (step) {
      addStep(step);
    }
    updateSessionState('completed');
  }

  function handleRestart(): void {
    endSession();
    loadMockData();
  }

  async function handleStop(): Promise<void> {
    if ($debugSession) {
      await endDebugSession($debugSession.id);
    }
    updateSessionState('stopped');
  }

  function handleToggleBreakpoint(id: string): void {
    toggleBpInStore(id);
  }

  function handleRemoveBreakpoint(id: string): void {
    removeBpFromStore(id);
  }

  function handleAddBreakpoint(detail: { type: BreakpointType; name: string }): void {
    const { type, name } = detail;
    addBpToStore({
      id: `bp-${Date.now()}`,
      type,
      name,
      enabled: true
    });
  }
</script>

<div class="debug-panel">
  <StepControls
    state={$sessionState}
    onPlay={handlePlay}
    onStep={handleStep}
    onContinue={handleContinue}
    onRestart={handleRestart}
    onStop={handleStop}
  />

  <div class="debug-body">
    <aside class="debug-sidebar">
      <BreakpointList
        breakpoints={$breakpointsStore}
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
</style>
