<script lang="ts">
  /**
   * HL7 intake's status line for a server-owned Integration Session run: a
   * state Badge and the server stages inline (12px), on one line. When the
   * pane is narrow, stage names ellipsize; each stage's title keeps the full
   * name, status and duration. The run id is shown by the page's context row.
   * `state-{streamState}` on the region and the status wording are what the
   * browser smoke gate reads (check 3: `state-complete`, "Preview complete").
   */
  import { Badge, type BadgeTone } from '$lib/ui/primitives';
  import type { IntegrationSessionPreviewMeta } from './types';

  export let session: IntegrationSessionPreviewMeta;

  function label(value: string): string {
    return value.replaceAll('_', ' ');
  }

  const TONE: Record<IntegrationSessionPreviewMeta['streamState'], BadgeTone> = {
    connecting: 'neutral',
    running: 'info',
    complete: 'success',
    error: 'danger'
  };

  $: statusLabel =
    session.streamState === 'connecting'
      ? 'Connecting to server diagnostics'
      : session.streamState === 'running'
        ? 'Preview running'
        : session.streamState === 'complete'
          ? 'Preview complete'
          : 'Stream needs attention';
</script>

<section class="run-progress state-{session.streamState}" aria-label="Server preview progression">
  <Badge tone={TONE[session.streamState]} dot aria-live="polite">{statusLabel}</Badge>

  {#if session.stages.length > 0}
    <ol class="stage-list">
      {#each session.stages as stage (stage.id)}
        <li
          class="stage stage-{stage.status}"
          title="{label(stage.name)}: {stage.status}{stage.durationMs != null ? ` (${stage.durationMs} ms)` : ''}"
        >
          <span class="stage-mark" aria-hidden="true"></span>
          <span class="stage-name">{label(stage.name)}</span>
          <span class="sr-only">{stage.status}</span>
          {#if stage.completedAt && stage.durationMs != null}
            <span class="duration">{stage.durationMs} ms</span>
          {/if}
        </li>
      {/each}
    </ol>
  {:else}
    <span class="waiting">The stream is ready; waiting for the first server stage.</span>
  {/if}

  {#if session.error}
    <span class="stream-error" role="status">{session.error}</span>
  {/if}
</section>

<style>
  .run-progress {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-1) var(--space-3);
    min-width: 0;
    font-size: var(--text-xs);
    color: var(--color-text-secondary);
  }

  .stage-list {
    display: flex;
    flex: 1 1 0;
    align-items: center;
    gap: var(--space-3);
    min-width: 0;
    overflow: hidden;
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .stage {
    display: inline-flex;
    flex: 0 1 auto;
    align-items: center;
    gap: 5px;
    min-width: 0;
    white-space: nowrap;
  }

  .stage-name {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .stage-mark {
    flex: 0 0 auto;
    width: 6px;
    height: 6px;
    border-radius: var(--radius-full);
    background: var(--color-text-muted);
  }

  .stage-running .stage-mark {
    background: var(--color-info);
  }

  .stage-succeeded .stage-mark {
    background: var(--color-success);
  }

  .stage-failed .stage-mark {
    background: var(--color-danger);
  }

  .duration {
    flex: 0 0 auto;
    font-family: var(--font-mono);
    font-size: var(--text-label);
    color: var(--color-text-tertiary);
    font-variant-numeric: tabular-nums;
  }

  .waiting {
    color: var(--color-text-tertiary);
  }

  .stream-error {
    flex-basis: 100%;
    color: var(--color-danger-text);
  }
</style>
