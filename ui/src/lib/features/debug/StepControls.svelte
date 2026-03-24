<script lang="ts">
  /**
   * StepControls Component
   *
   * Toolbar with debug session controls: Play, Step Over, Continue, Restart, Stop.
   * Buttons are enabled/disabled based on current session state.
   */
  import type { DebugSessionState } from './types';

  export let state: DebugSessionState = 'idle';
  export let onPlay: (() => void) | undefined = undefined;
  export let onStep: (() => void) | undefined = undefined;
  export let onContinue: (() => void) | undefined = undefined;
  export let onRestart: (() => void) | undefined = undefined;
  export let onStop: (() => void) | undefined = undefined;

  $: canPlay = state === 'idle';
  $: canStep = state === 'paused';
  $: canContinue = state === 'paused';
  $: canRestart = state === 'paused' || state === 'completed';
  $: canStop = state === 'running' || state === 'paused';
</script>

<div class="step-controls" role="toolbar" aria-label="Debug controls">
  <button
    class="control-btn"
    class:active={canPlay}
    disabled={!canPlay}
    title="Play (F5)"
    aria-label="Play"
    on:click={() => onPlay?.()}
  >
    <svg viewBox="0 0 16 16" fill="currentColor" aria-hidden="true">
      <path d="M4 2l10 6-10 6V2z" />
    </svg>
    <span class="shortcut">F5</span>
  </button>

  <button
    class="control-btn"
    class:active={canStep}
    disabled={!canStep}
    title="Step Over (F10)"
    aria-label="Step Over"
    on:click={() => onStep?.()}
  >
    <svg viewBox="0 0 16 16" fill="currentColor" aria-hidden="true">
      <path d="M2 8h8M7 5l3 3-3 3" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" />
      <circle cx="13" cy="8" r="1.5" />
    </svg>
    <span class="shortcut">F10</span>
  </button>

  <button
    class="control-btn"
    class:active={canContinue}
    disabled={!canContinue}
    title="Continue (F8)"
    aria-label="Continue"
    on:click={() => onContinue?.()}
  >
    <svg viewBox="0 0 16 16" fill="currentColor" aria-hidden="true">
      <path d="M2 2l6 6-6 6V2z" />
      <path d="M9 2l6 6-6 6V2z" />
    </svg>
    <span class="shortcut">F8</span>
  </button>

  <div class="separator" aria-hidden="true"></div>

  <button
    class="control-btn"
    class:active={canRestart}
    disabled={!canRestart}
    title="Restart"
    aria-label="Restart"
    on:click={() => onRestart?.()}
  >
    <svg viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
      <path d="M2 8a6 6 0 1 1 1.76 4.24" />
      <path d="M2 13V9h4" />
    </svg>
  </button>

  <button
    class="control-btn danger"
    class:active={canStop}
    disabled={!canStop}
    title="Stop (Shift+F5)"
    aria-label="Stop"
    on:click={() => onStop?.()}
  >
    <svg viewBox="0 0 16 16" fill="currentColor" aria-hidden="true">
      <rect x="3" y="3" width="10" height="10" rx="1" />
    </svg>
    <span class="shortcut">Shift+F5</span>
  </button>
</div>

<style>
  .step-controls {
    display: flex;
    align-items: center;
    gap: var(--space-1);
    padding: var(--space-2) var(--space-3);
    background: var(--color-bg-elevated);
    border-bottom: 1px solid var(--color-border-subtle);
    border-radius: var(--radius-lg) var(--radius-lg) 0 0;
  }

  .control-btn {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
    padding: var(--space-1) var(--space-2);
    border: 1px solid transparent;
    border-radius: var(--radius-md);
    background: transparent;
    color: var(--color-text-muted);
    cursor: pointer;
    font-family: inherit;
    font-size: var(--text-xs);
    transition: var(--transition-all);
  }

  .control-btn svg {
    width: 16px;
    height: 16px;
    flex-shrink: 0;
  }

  .control-btn:hover:not(:disabled) {
    background: var(--color-bg-hover);
    color: var(--color-text-primary);
    border-color: var(--color-border-default);
  }

  .control-btn:focus-visible {
    outline: none;
    box-shadow: var(--shadow-focus);
    border-color: var(--color-border-focus);
  }

  .control-btn.active {
    color: var(--color-text-secondary);
  }

  .control-btn.active:hover:not(:disabled) {
    background: var(--color-primary-muted);
    color: var(--color-primary);
    border-color: var(--color-primary-border);
    box-shadow: 0 0 8px var(--color-primary-glow);
  }

  .control-btn:disabled {
    opacity: 0.35;
    cursor: not-allowed;
  }

  .control-btn.danger.active:hover:not(:disabled) {
    background: var(--color-danger-bg);
    color: var(--color-danger);
    border-color: var(--color-danger-border);
    box-shadow: none;
  }

  .shortcut {
    font-size: var(--text-2xs);
    color: var(--color-text-muted);
    opacity: 0.7;
  }

  .separator {
    width: 1px;
    height: 20px;
    background: var(--color-border-default);
    margin: 0 var(--space-1);
  }
</style>
