<script lang="ts">
  import { onDestroy, onMount, createEventDispatcher } from 'svelte';
  import { subscribe as wsSubscribe } from '$lib/graphql/subscriptions';
  import { EventStreamDocument, WorkflowEventsDocument } from '$lib/gen/graphql';
  import Badge from '$lib/ui/Badge.svelte';
  import Button from '$lib/ui/Button.svelte';
  import { workflowDraft } from '$lib/features/workflows/workflowStore';
  import { debugSession } from '$lib/features/debug/debugStore';
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
    setRuntimeOutputSessionId,
  } from './runtimeOutputStore';

  const dispatch = createEventDispatcher<{ navigate: { panel: string } }>();

  let unsubscribe: (() => void) | null = null;
  let mounted = false;
  let workflowName = '';
  let entryCount = 0;
  let latestTimestamp: string | null = null;
  let statusLabel = 'Idle';
  let statusVariant: 'default' | 'primary' | 'success' | 'warning' | 'danger' | 'info' = 'default';
  let feedLabel = 'Event stream';
  let stateMessage = 'Live output will appear here as workflow or event stream messages arrive.';

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
  $: entryCount = $runtimeOutputState.entries.length;
  $: latestTimestamp = $runtimeOutputState.entries[0]?.timestamp ?? $runtimeOutputState.updatedAt;
  $: statusLabel =
    $runtimeOutputState.status === 'error'
      ? 'Disconnected'
      : $runtimeOutputState.connected || entryCount > 0
        ? 'Live'
        : $runtimeOutputState.status === 'connecting'
          ? 'Connecting'
          : 'Idle';
  $: statusVariant =
    $runtimeOutputState.status === 'error'
      ? 'danger'
      : $runtimeOutputState.connected || entryCount > 0
        ? 'success'
        : $runtimeOutputState.status === 'connecting'
          ? 'info'
          : 'default';
  $: feedLabel = $runtimeOutputState.feedKind === 'workflow' ? 'Workflow feed' : 'Event stream';
  $: stateMessage =
    $runtimeOutputState.error
      ? 'The console is disconnected. Try reconnecting once the backend is healthy.'
      : $runtimeOutputState.connected || entryCount > 0
        ? 'The feed is healthy and waiting for the next runtime event.'
        : $runtimeOutputState.status === 'connecting'
          ? 'Connecting to live output for workflow and event stream activity.'
          : 'Live output will appear here as workflow or event stream messages arrive.';
  $: if ($debugSession?.id) {
    setRuntimeOutputSessionId($debugSession.id);
  } else {
    setRuntimeOutputSessionId(null);
  }

  $: if (mounted) {
    subscribeToFeed();
  }

  onDestroy(() => {
    stopSubscription();
  });
</script>

<div class="panel">
  <div class="header">
    <div class="header-copy">
      <p class="eyebrow">Operational console</p>
      <div class="status-row" role="status" aria-live="polite">
        <div class="status">
          <span
            class="indicator"
            class:connected={$runtimeOutputState.connected}
            class:error={$runtimeOutputState.status === 'error'}
          ></span>
          <div class="status-copy">
            <div class="status-title">{ $runtimeOutputState.feedLabel }</div>
            <div class="status-text">{stateMessage}</div>
          </div>
        </div>

        <Badge variant={statusVariant} size="sm" pill>{statusLabel}</Badge>
      </div>
    </div>

    <div class="metrics" aria-label="Runtime output summary">
      <div class="metric">
        <span class="metric-label">Entries</span>
        <span class="metric-value">{entryCount}</span>
      </div>
      <div class="metric">
        <span class="metric-label">Latest</span>
        <span class="metric-value">{latestTimestamp ? formatRuntimeOutputTimestamp(latestTimestamp) : '—'}</span>
      </div>
      <div class="metric">
        <span class="metric-label">Feed</span>
        <span class="metric-value">{feedLabel}</span>
      </div>
    </div>
  </div>

  <div class="actions">
      <Button variant="secondary" on:click={clearEntries}>Clear feed</Button>
      <Button variant="secondary" on:click={reconnect}>Reconnect</Button>
  </div>

  {#if $runtimeOutputState.entries.length === 0}
    <div class="empty">
      {#if $runtimeOutputState.error}
        The console is disconnected. Try reconnecting once the backend is healthy.
      {:else if $runtimeOutputState.status === 'connected'}
        The feed is healthy and waiting for the next runtime event.
      {:else}
        Live output will appear here as workflow events or stream events arrive.
      {/if}
    </div>
  {:else}
    <div class="entry-list" role="list" aria-label="Runtime output entries">
      {#each $runtimeOutputState.entries as entry (entry.id)}
        <article
          class="entry"
          class:session-match={entry.sessionId && entry.sessionId === $runtimeOutputState.activeSessionId}
          role="listitem"
        >
          <div class="entry-head">
            <span class="time mono">{formatRuntimeOutputTimestamp(entry.timestamp)}</span>
            <span class="severity" class:warning={entry.severity === 'warning'} class:error={entry.severity === 'error'}>
              {entry.severity}
            </span>
            <span class="kind">{entry.kind === 'workflow' ? 'Workflow' : 'Event stream'}</span>
            {#if entry.sessionId}
              <button
                type="button"
                class="jump-btn"
                on:click={() => dispatch('navigate', { panel: 'debug' })}
                title="Jump to Debug"
              >Debug</button>
              <button
                type="button"
                class="jump-btn"
                on:click={() => dispatch('navigate', { panel: 'trace' })}
                title="Jump to Trace"
              >Trace</button>
            {/if}
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

  .header {
    display: grid;
    grid-template-columns: minmax(0, 1.3fr) minmax(240px, 0.8fr);
    gap: var(--space-3);
    align-items: start;
  }

  .header-copy {
    display: grid;
    gap: var(--space-2);
  }

  .eyebrow {
    margin: 0;
    text-transform: uppercase;
    letter-spacing: 0.12em;
    font-size: var(--text-2xs);
    font-weight: var(--font-bold);
    color: var(--color-text-tertiary);
  }

  .status-row {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    flex-wrap: wrap;
  }

  .metrics {
    display: grid;
    gap: 10px;
    padding: var(--space-3);
    border: 1px solid var(--color-border-subtle);
    border-radius: var(--radius-lg);
    background: var(--color-bg-surface);
  }

  .metric {
    display: grid;
    gap: 3px;
  }

  .metric-label {
    font-size: var(--text-2xs);
    text-transform: uppercase;
    letter-spacing: 0.08em;
    font-weight: var(--font-bold);
    color: var(--color-text-tertiary);
  }

  .metric-value {
    color: var(--color-text-primary);
    font-size: var(--text-sm);
    font-weight: var(--font-semibold);
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
    background: var(--color-success, var(--color-success));
    box-shadow: 0 0 0 4px rgba(16, 185, 129, 0.12);
  }

  .indicator.error {
    background: var(--color-danger, var(--color-danger));
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

  .entry.session-match {
    border-left: 3px solid var(--color-primary, var(--palette-blue-500));
  }

  .jump-btn {
    padding: 1px 6px;
    border: 1px solid var(--color-border-subtle);
    border-radius: 4px;
    background: transparent;
    color: var(--color-text-tertiary);
    font-size: 10px;
    font-weight: var(--font-semibold);
    cursor: pointer;
    transition: var(--transition-colors);
  }

  .jump-btn:hover {
    background: var(--color-bg-hover);
    color: var(--color-text-primary);
    border-color: var(--color-border-default);
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

  @media (max-width: 880px) {
    .header {
      grid-template-columns: 1fr;
    }
  }
</style>
