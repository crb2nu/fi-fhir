<script lang="ts">
  import { onDestroy, onMount } from 'svelte';
  import { subscribe as wsSubscribe } from '$lib/graphql/subscriptions';
  import { EventStreamDocument, WorkflowEventsDocument } from '$lib/gen/graphql';
  import Button from '$lib/ui/Button.svelte';
  import { workflowDraft } from '$lib/features/workflows/workflowStore';
  import {
    activateRuntimeOutputFeed,
    appendRuntimeOutputEntry,
    clearRuntimeOutputEntries,
    describeEventStreamOutput,
    describeWorkflowOutput,
    formatRuntimeOutputTimestamp,
    markRuntimeOutputConnected,
    markRuntimeOutputError,
    markRuntimeOutputIdle,
    runtimeOutputState,
  } from './runtimeOutputStore';

  let unsubscribe: (() => void) | null = null;
  let mounted = false;
  let workflowName = '';

  function stopSubscription(): void {
    if (unsubscribe) {
      unsubscribe();
      unsubscribe = null;
    }
    markRuntimeOutputIdle();
  }

  function subscribeToFeed(): void {
    const feedKey = workflowName ? `workflow:${workflowName}` : 'event-stream';
    const feedKind = workflowName ? 'workflow' : 'event-stream';
    const feedLabel = workflowName ? `Workflow ${workflowName}` : 'Event stream';

    if (feedKey === $runtimeOutputState.feedKey && unsubscribe) {
      return;
    }

    stopSubscription();
    if ($runtimeOutputState.feedKey && $runtimeOutputState.feedKey !== feedKey) {
      clearRuntimeOutputEntries();
    }

    activateRuntimeOutputFeed(feedKey, feedLabel, feedKind);

    if (feedKind === 'workflow') {
      unsubscribe = wsSubscribe(
        WorkflowEventsDocument,
        { workflowName },
        {
          onData: (data) => {
            if (!data.workflowEvents) return;
            appendRuntimeOutputEntry(describeWorkflowOutput(data.workflowEvents));
            markRuntimeOutputConnected();
          },
          onError: (err) => {
            markRuntimeOutputError(err.message);
          },
          onComplete: () => {
            markRuntimeOutputIdle();
          }
        }
      );
      return;
    }

    unsubscribe = wsSubscribe(
      EventStreamDocument,
      { filter: null },
      {
        onData: (data) => {
          if (!data.eventStream) return;
          appendRuntimeOutputEntry(describeEventStreamOutput(data.eventStream));
          markRuntimeOutputConnected();
        },
        onError: (err) => {
          markRuntimeOutputError(err.message);
        },
        onComplete: () => {
          markRuntimeOutputIdle();
        }
      }
    );
  }

  function reconnect(): void {
    stopSubscription();
    subscribeToFeed();
  }

  function clearEntries(): void {
    clearRuntimeOutputEntries();
  }

  onMount(() => {
    mounted = true;
    subscribeToFeed();
  });

  $: workflowName = $workflowDraft.name.trim();
  $: if (mounted) {
    subscribeToFeed();
  }

  onDestroy(() => {
    stopSubscription();
  });
</script>

<div class="panel">
  <div class="controls">
    <div class="status" role="status" aria-live="polite">
      <span class="indicator" class:connected={$runtimeOutputState.connected} class:error={$runtimeOutputState.status === 'error'}></span>
      <div class="status-copy">
        <div class="status-title">{ $runtimeOutputState.feedLabel }</div>
        <div class="status-text">
          {#if $runtimeOutputState.error}
            {$runtimeOutputState.error}
          {:else if $runtimeOutputState.status === 'connected'}
            Connected
          {:else if $runtimeOutputState.status === 'connecting'}
            Connecting to live output...
          {:else}
            Waiting for runtime output
          {/if}
        </div>
      </div>
    </div>

    <div class="actions">
      <Button variant="secondary" on:click={clearEntries}>Clear</Button>
      <Button variant="secondary" on:click={reconnect}>Reconnect</Button>
    </div>
  </div>

  {#if $runtimeOutputState.entries.length === 0}
    <div class="empty">
      {#if $runtimeOutputState.error}
        No live output is available right now. Try reconnecting once the backend is healthy.
      {:else if $runtimeOutputState.status === 'connected'}
        Waiting for the next runtime event...
      {:else}
        Live output will appear here as workflow events or stream events arrive.
      {/if}
    </div>
  {:else}
    <div class="entry-list" role="list" aria-label="Runtime output entries">
      {#each $runtimeOutputState.entries as entry (entry.id)}
        <article class="entry" role="listitem">
          <div class="entry-head">
            <span class="time mono">{formatRuntimeOutputTimestamp(entry.timestamp)}</span>
            <span class="severity" class:warning={entry.severity === 'warning'} class:error={entry.severity === 'error'}>
              {entry.severity}
            </span>
            <span class="kind">{entry.kind === 'workflow' ? 'Workflow' : 'Event stream'}</span>
          </div>

          <div class="entry-title">{entry.title}</div>
          <div class="entry-message">{entry.message}</div>

          <div class="entry-meta">
            <span class="source mono">{entry.source}</span>
            {#if entry.details.length > 0}
              <span class="details">{entry.details.join(' • ')}</span>
            {/if}
          </div>
        </article>
      {/each}
    </div>
  {/if}
</div>

<style>
  .panel {
    display: grid;
    gap: var(--space-3);
  }

  .controls {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3);
    flex-wrap: wrap;
  }

  .status {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    min-width: 0;
  }

  .indicator {
    width: 10px;
    height: 10px;
    border-radius: 999px;
    background: var(--color-border-strong);
  }

  .indicator.connected {
    background: var(--color-success, #10b981);
    box-shadow: 0 0 0 4px rgba(16, 185, 129, 0.12);
  }

  .indicator.error {
    background: var(--color-danger, #ef4444);
    box-shadow: 0 0 0 4px rgba(239, 68, 68, 0.12);
  }

  .status-copy {
    display: grid;
    gap: 2px;
    min-width: 0;
  }

  .status-title {
    font-size: var(--text-sm);
    font-weight: var(--font-semibold);
    color: var(--color-text-primary);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .status-text {
    font-size: var(--text-xs);
    color: var(--color-text-tertiary);
  }

  .actions {
    display: flex;
    gap: var(--space-2);
    flex-wrap: wrap;
  }

  .empty {
    padding: var(--space-4);
    border: 1px dashed var(--color-border-subtle);
    border-radius: var(--radius-lg);
    color: var(--color-text-tertiary);
    background: var(--color-bg-elevated);
  }

  .entry-list {
    display: grid;
    gap: var(--space-2);
    max-height: 100%;
    overflow: auto;
  }

  .entry {
    display: grid;
    gap: 4px;
    padding: var(--space-3);
    border: 1px solid var(--color-border-subtle);
    border-radius: var(--radius-lg);
    background: var(--color-bg-elevated);
  }

  .entry-head {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    flex-wrap: wrap;
  }

  .time {
    color: var(--color-text-tertiary);
    font-size: var(--text-xs);
  }

  .severity,
  .kind {
    padding: 2px 8px;
    border-radius: 999px;
    font-size: 10px;
    font-weight: var(--font-semibold);
    letter-spacing: 0.04em;
    text-transform: uppercase;
  }

  .severity {
    background: rgba(59, 130, 246, 0.14);
    border: 1px solid rgba(59, 130, 246, 0.24);
    color: rgba(191, 219, 254, 0.95);
  }

  .severity.warning {
    background: rgba(245, 158, 11, 0.14);
    border-color: rgba(245, 158, 11, 0.24);
    color: rgba(254, 240, 138, 0.95);
  }

  .severity.error {
    background: rgba(239, 68, 68, 0.14);
    border-color: rgba(239, 68, 68, 0.24);
    color: rgba(254, 202, 202, 0.96);
  }

  .kind {
    background: var(--color-bg-surface);
    border: 1px solid var(--color-border-subtle);
    color: var(--color-text-secondary);
  }

  .entry-title {
    font-weight: var(--font-semibold);
    color: var(--color-text-primary);
  }

  .entry-message {
    color: var(--color-text-secondary);
    font-size: var(--text-sm);
    line-height: 1.4;
  }

  .entry-meta {
    display: flex;
    gap: var(--space-2);
    flex-wrap: wrap;
    color: var(--color-text-tertiary);
    font-size: var(--text-xs);
  }

  .source {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .details {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
</style>
