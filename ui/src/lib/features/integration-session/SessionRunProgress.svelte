<script lang="ts">
  import type { IntegrationSessionPreviewMeta } from './types';

  export let session: IntegrationSessionPreviewMeta;

  function label(value: string): string {
    return value.replaceAll('_', ' ');
  }

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
  <div class="run-head">
    <div>
      <div class="eyebrow">Server-owned run</div>
      <div class="run-status" aria-live="polite">{statusLabel}</div>
    </div>
    {#if session.runId}
      <span class="run-id" title={session.runId}>{session.runId}</span>
    {/if}
  </div>

  {#if session.stages.length > 0}
    <ol class="stage-list">
      {#each session.stages as stage (stage.id)}
        <li class="stage stage-{stage.status}">
          <span class="stage-mark" aria-hidden="true"></span>
          <span>{label(stage.name)}</span>
          <span class="sr-only">{stage.status}</span>
          {#if stage.completedAt && stage.durationMs != null}
            <span class="duration">{stage.durationMs} ms</span>
          {/if}
        </li>
      {/each}
    </ol>
  {:else}
    <div class="waiting">The stream is ready; waiting for the first server stage.</div>
  {/if}

  {#if session.error}
    <div class="stream-error" role="status">{session.error}</div>
  {/if}
</section>

<style>
  .run-progress {
    display: grid;
    gap: var(--space-2);
    margin-bottom: var(--space-3);
    padding: var(--space-3);
    border: 1px solid var(--color-border-subtle);
    border-radius: var(--radius-lg);
    background: var(--color-bg-elevated);
  }

  .run-progress.state-running {
    border-color: var(--color-info-border);
  }

  .run-progress.state-complete {
    border-color: var(--color-success-border);
  }

  .run-progress.state-error {
    border-color: var(--color-danger-border);
  }

  .run-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3);
  }

  .eyebrow {
    color: var(--color-text-muted);
    font-size: var(--text-2xs);
    font-weight: var(--font-bold);
    letter-spacing: var(--tracking-wider);
    text-transform: uppercase;
  }

  .run-status {
    color: var(--color-text-primary);
    font-size: var(--text-sm);
    font-weight: var(--font-semibold);
  }

  .run-id {
    max-width: 240px;
    overflow: hidden;
    color: var(--color-text-muted);
    font-family: var(--font-mono);
    font-size: var(--text-2xs);
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .stage-list {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .stage {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
    padding: 4px 8px;
    border: 1px solid var(--color-border-subtle);
    border-radius: var(--radius-full);
    color: var(--color-text-secondary);
    font-size: var(--text-xs);
  }

  .stage-mark {
    width: 7px;
    height: 7px;
    border-radius: 50%;
    background: var(--color-text-muted);
  }

  .stage-running .stage-mark {
    background: var(--color-info);
    box-shadow: 0 0 0 3px var(--color-info-bg);
  }

  .stage-succeeded .stage-mark {
    background: var(--color-success);
  }

  .stage-failed .stage-mark {
    background: var(--color-danger);
  }

  .duration {
    color: var(--color-text-muted);
    font-family: var(--font-mono);
    font-size: var(--text-2xs);
  }

  .waiting,
  .stream-error {
    color: var(--color-text-tertiary);
    font-size: var(--text-xs);
  }

  .stream-error {
    color: var(--color-danger-text);
  }

  .sr-only {
    position: absolute;
    width: 1px;
    height: 1px;
    padding: 0;
    margin: -1px;
    overflow: hidden;
    clip: rect(0, 0, 0, 0);
    white-space: nowrap;
    border: 0;
  }

  @media (max-width: 640px) {
    .run-head {
      align-items: flex-start;
      flex-direction: column;
    }

    .run-id {
      max-width: 100%;
    }
  }
</style>
