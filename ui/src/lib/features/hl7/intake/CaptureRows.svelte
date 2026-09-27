<!--
  The session's stream captures in the Samples panel: "2 / 5 captured ·
  expires in 4:12 · Cancel" while armed, then how each finished. The rows read
  the page's intake controller, which polls while any capture is armed; the
  countdown ticks locally and never decides an expiry (the server does).
-->
<script lang="ts">
  import { Badge, Button } from '$lib/ui/primitives';
  import type { ConnectionCaptureRow } from './intakeApi';
  import { EMPTY_INTAKE, type IntakeController, type IntakeControllerState } from './intakeController';
  import { problemGuidance } from './intakeErrors';
  import { KERNEL_LIMITATION_NOTE, anyArmed, captureRowView, streamCaptures } from './intakeState';

  interface Props {
    controller: IntakeController;
    disabled?: boolean | undefined;
    oncancel: (capture: ConnectionCaptureRow) => void;
  }

  let { controller, disabled = false, oncancel }: Props = $props();

  // The controller's state, mirrored into runes state (the controller is a store-backed module).
  let intake = $state<IntakeControllerState>(EMPTY_INTAKE);
  $effect(() => controller.state.subscribe((value) => (intake = value)));
  let now = $state(Date.now());

  const captures = $derived(streamCaptures(intake.captures));
  const armed = $derived(anyArmed(captures));

  $effect(() => {
    if (!armed) return;
    now = Date.now();
    const handle = setInterval(() => (now = Date.now()), 1000);
    return () => clearInterval(handle);
  });
</script>

{#if captures.length > 0 || intake.pollError}
  <div class="captures" aria-label="Connection captures" role="group">
    {#each captures as capture (capture.id)}
      {@const row = captureRowView(capture, now)}
      <div class="capture" data-testid="connection-capture-row" data-status={row.status} data-source={row.sourceId}>
        <div class="capture-line">
          <Badge tone={row.tone} dot>{row.armed ? 'Capturing' : 'Capture'}</Badge>
          <code class="capture-source" title={`Capture ${row.id}`}>{row.sourceId}</code>
          <span class="capture-text" aria-live={row.armed ? 'polite' : undefined}>
            {row.progress} · {row.detail}
          </span>
          {#if row.armed}
            <Button variant="ghost" onclick={() => oncancel(capture)} {disabled}>Cancel</Button>
          {/if}
        </div>
        {#if row.stalled}
          <p class="capture-note">{KERNEL_LIMITATION_NOTE}</p>
        {/if}
        {#each row.problems as problem, index (`${problem.code}:${index}`)}
          <p class="capture-problem" data-code={problem.code}>
            <code>{problem.code}</code>
            {problem.message}
            {#if problemGuidance(problem.code)}<span class="capture-hint">{problemGuidance(problem.code)}</span>{/if}
          </p>
        {/each}
      </div>
    {/each}
    {#if intake.pollError}
      <div class="poll-error" role="alert">
        <span>{intake.pollError}</span>
        {#if !intake.polling}
          <Button variant="ghost" onclick={() => void controller.refresh()} {disabled}>Retry</Button>
        {/if}
      </div>
    {/if}
  </div>
{/if}

<style>
  .captures {
    display: grid;
    gap: var(--space-1);
  }

  .capture {
    display: grid;
    gap: 2px;
    padding: var(--space-1) var(--space-2);
    border: 1px solid var(--color-border-subtle);
    border-radius: var(--radius-sm);
    background: var(--color-bg-surface);
  }

  .capture-line {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    min-width: 0;
    min-height: var(--size-control-sm);
    font-size: var(--text-xs);
    color: var(--color-text-secondary);
  }

  .capture-source,
  .capture-problem code {
    font-family: var(--font-mono);
    font-size: var(--text-mono);
    color: var(--color-text-primary);
  }

  .capture-source {
    max-width: 200px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .capture-text {
    flex: 1 1 auto;
    min-width: 0;
    font-variant-numeric: tabular-nums;
  }

  .capture-note,
  .capture-problem {
    margin: 0;
    font-size: var(--text-xs);
    line-height: var(--leading-snug);
    color: var(--color-text-tertiary);
  }

  .capture-problem {
    color: var(--color-warning-text);
  }

  .capture-hint {
    color: var(--color-text-tertiary);
  }

  .poll-error {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    padding: var(--space-1) var(--space-2);
    border: 1px solid var(--color-danger-border);
    border-radius: var(--radius-sm);
    background: var(--color-danger-bg);
    color: var(--color-danger-text);
    font-size: var(--text-xs);
  }
</style>
