<!--
  PipelineChips — HL7 intake's one-row view of the path a message takes:
  source · profile · workflow · destination. Each chip is 24px: a state dot,
  the stage label, and the value the last preview or process actually used
  (an em dash when nothing has run yet — never an invented default). Clicking
  a chip opens the Results view that owns that stage.
-->
<script module lang="ts">
  export type PipelineStage = 'source' | 'profile' | 'workflow' | 'destination';
  export type PipelineState = 'idle' | 'ok' | 'warning' | 'error';

  export interface PipelineStep {
    id: PipelineStage;
    label: string;
    /** What this stage resolved to; empty renders an em dash. */
    value: string;
    state: PipelineState;
    /** Tooltip: where the value came from and what a click opens. */
    title?: string | undefined;
  }
</script>

<script lang="ts">
  import ChevronRight from '@lucide/svelte/icons/chevron-right';
  import { Icon } from '$lib/ui/primitives';

  interface Props {
    steps: readonly PipelineStep[];
    disabled?: boolean;
    onselect?: ((id: PipelineStage) => void) | undefined;
  }

  let { steps, disabled = false, onselect }: Props = $props();

  const STATE_LABEL: Record<PipelineState, string> = {
    idle: 'not run',
    ok: 'ok',
    warning: 'warnings',
    error: 'failed'
  };
</script>

<nav class="pipeline" aria-label="Message pipeline" data-testid="hl7-pipeline">
  <ol class="pipeline-list">
    {#each steps as step, index (step.id)}
      {#if index > 0}
        <li class="pipeline-sep" aria-hidden="true">
          <Icon icon={ChevronRight} size={14} />
        </li>
      {/if}
      <li>
        <button
          type="button"
          class="chip state-{step.state}"
          title={step.title}
          {disabled}
          data-stage={step.id}
          data-state={step.state}
          onclick={() => onselect?.(step.id)}
        >
          <span class="dot" aria-hidden="true"></span>
          <span class="chip-label">{step.label}</span>
          <span class="chip-value">{step.value || '—'}</span>
          <span class="sr-only">({STATE_LABEL[step.state]})</span>
        </button>
      </li>
    {/each}
  </ol>
</nav>

<style>
  .pipeline {
    min-width: 0;
  }

  .pipeline-list {
    display: flex;
    align-items: center;
    gap: var(--space-1);
    min-width: 0;
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .pipeline-list > li {
    display: flex;
    min-width: 0;
  }

  .pipeline-sep {
    flex: 0 0 auto;
    color: var(--color-text-muted);
  }

  .chip {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    height: 24px;
    min-width: 0;
    max-width: 260px;
    padding: 0 var(--space-2);
    border: 1px solid var(--color-border-subtle);
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--color-text-secondary);
    font: inherit;
    font-size: var(--text-xs);
    cursor: pointer;
    transition: var(--transition-colors);
  }

  .chip:hover:not(:disabled) {
    background: var(--color-bg-hover);
    border-color: var(--color-border-default);
    color: var(--color-text-primary);
  }

  .chip:focus-visible {
    outline: 2px solid var(--color-focus-ring);
    outline-offset: 1px;
  }

  .chip:disabled {
    cursor: not-allowed;
    opacity: 0.6;
  }

  .dot {
    flex: 0 0 auto;
    width: 6px;
    height: 6px;
    border-radius: var(--radius-full);
    background: var(--color-text-muted);
  }

  .state-ok .dot {
    background: var(--color-success);
  }

  .state-warning .dot {
    background: var(--color-warning);
  }

  .state-error .dot {
    background: var(--color-danger);
  }

  .chip-label {
    flex: 0 0 auto;
    color: var(--color-text-tertiary);
  }

  .chip-value {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-family: var(--font-mono);
    font-size: var(--text-mono);
    color: var(--color-text-primary);
  }
</style>
