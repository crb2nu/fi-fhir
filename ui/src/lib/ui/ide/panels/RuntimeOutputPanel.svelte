<script lang="ts">
  import { onDestroy, onMount, createEventDispatcher } from 'svelte';
  import { subscribe as wsSubscribe } from '$lib/graphql/subscriptions';
  import {
    noteStreamError,
    streamStatus,
    type StreamRoot
  } from '$lib/graphql/streamAvailability';
  import { EventStreamDocument, WorkflowEventsDocument } from '$lib/gen/graphql';
  import Eraser from '@lucide/svelte/icons/eraser';
  import RefreshCw from '@lucide/svelte/icons/refresh-cw';
  import { Badge, Button } from '$lib/ui/primitives';
  import StreamingUnavailable from '$lib/ui/StreamingUnavailable.svelte';
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
  let statusVariant: 'neutral' | 'success' | 'danger' | 'info' = 'neutral';
  let feedLabel = 'Event stream';
  let stateMessage = 'Live output will appear here as workflow or event stream messages arrive.';

  // The feed rides `workflowEvents` (a named draft) or `eventStream`; when the
  // deployment cannot stream that root the panel says so and never subscribes.
  const workflowFeedStatus = streamStatus('workflowEvents');
  const eventFeedStatus = streamStatus('eventStream');
  $: feedRoot = (workflowName ? 'workflowEvents' : 'eventStream') as StreamRoot;
  $: feedStatus = feedRoot === 'workflowEvents' ? $workflowFeedStatus : $eventFeedStatus;
  $: feedUnavailable = feedStatus.availability === 'unavailable' ? feedStatus : null;
  $: if (feedUnavailable && unsubscribe) stopSubscription();

  function currentFeedUnavailable(): boolean {
    const status = workflowName ? $workflowFeedStatus : $eventFeedStatus;
    return status.availability === 'unavailable';
  }

  function handleStreamError(root: StreamRoot, err: Error): void {
    if (noteStreamError(root, err)) {
      markRuntimeOutputIdle();
      return;
    }
    markRuntimeOutputError(err.message);
  }

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
    if (currentFeedUnavailable()) return;
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
          onError: (err) => handleStreamError('workflowEvents', err),
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
        onError: (err) => handleStreamError('eventStream', err),
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
          : 'neutral';
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
  {#if feedUnavailable}
    <StreamingUnavailable
      root={feedRoot}
      subject={feedRoot === 'workflowEvents' ? `workflow ${workflowName} output` : 'runtime output'}
      reason={feedUnavailable.reason}
      alternative="Dry runs in the workflow builder and Run Diagnostics in the Workflows monitor show recorded results."
    />
  {:else}
  <div class="header">
    <div class="status-row" role="status" aria-live="polite">
      <span
        class="indicator"
        class:connected={$runtimeOutputState.connected}
        class:error={$runtimeOutputState.status === 'error'}
        aria-hidden="true"
      ></span>
      <span class="status-title">{ $runtimeOutputState.feedLabel }</span>
      <Badge tone={statusVariant} dot>{statusLabel}</Badge>
      <span class="status-text">{stateMessage}</span>
    </div>

    <dl class="metrics" aria-label="Runtime output summary">
      <div class="metric">
        <dt>Entries</dt>
        <dd>{entryCount}</dd>
      </div>
      <div class="metric">
        <dt>Latest</dt>
        <dd>{latestTimestamp ? formatRuntimeOutputTimestamp(latestTimestamp) : '—'}</dd>
      </div>
      <div class="metric">
        <dt>Feed</dt>
        <dd class="feed">{feedLabel}</dd>
      </div>
    </dl>

    <div class="actions">
      <Button variant="ghost" icon={Eraser} onclick={clearEntries}>Clear feed</Button>
      <Button variant="ghost" icon={RefreshCw} onclick={reconnect}>Reconnect</Button>
    </div>
  </div>

  {#if $runtimeOutputState.entries.length === 0}
    <p class="empty">
      {#if $runtimeOutputState.error}
        The console is disconnected. Try reconnecting once the backend is healthy.
      {:else if $runtimeOutputState.status === 'connected'}
        The feed is healthy and waiting for the next runtime event.
      {:else}
        Live output will appear here as workflow events or stream events arrive.
      {/if}
    </p>
  {:else}
    <div class="entry-list" role="list" aria-label="Runtime output entries">
      {#each $runtimeOutputState.entries as entry (entry.id)}
        <article
          class="entry"
          class:session-match={entry.sessionId && entry.sessionId === $runtimeOutputState.activeSessionId}
          role="listitem"
        >
          <span class="time mono">{formatRuntimeOutputTimestamp(entry.timestamp)}</span>
          <span class="severity" class:warning={entry.severity === 'warning'} class:error={entry.severity === 'error'}>
            {entry.severity}
          </span>
          <span class="kind">{entry.kind === 'workflow' ? 'Workflow' : 'Event stream'}</span>
          <span class="entry-text">
            <span class="entry-title">{entry.title}</span>
            <span class="entry-message">{entry.message}</span>
            {#if entry.details.length > 0}
              <span class="details">{entry.details.join(' • ')}</span>
            {/if}
          </span>
          <span class="source mono">{entry.source}</span>
          {#if entry.sessionId}
            <span class="jumps">
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
            </span>
          {/if}
        </article>
      {/each}
    </div>
  {/if}
  {/if}
</div>

<style>
  .panel {
    display: grid;
    gap: var(--space-2);
    font-size: var(--text-ui);
  }

  /* One header row: feed status, counts, actions. */
  .header {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2) var(--space-4);
  }

  .status-row {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    min-width: 0;
  }

  .indicator {
    width: 6px;
    height: 6px;
    flex: 0 0 auto;
    border-radius: var(--radius-full);
    background: var(--color-text-muted);
  }

  .indicator.connected {
    background: var(--color-success);
  }

  .indicator.error {
    background: var(--color-danger);
  }

  .status-title {
    color: var(--color-text-primary);
    font-weight: var(--font-semibold);
    white-space: nowrap;
  }

  .status-text {
    min-width: 0;
    overflow: hidden;
    color: var(--color-text-tertiary);
    font-size: var(--text-xs);
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .metrics {
    display: flex;
    gap: var(--space-4);
    margin: 0;
  }

  .metric {
    display: flex;
    align-items: baseline;
    gap: var(--space-1);
  }

  .metric dt {
    color: var(--color-text-tertiary);
    font-size: var(--text-label);
    font-weight: var(--font-semibold);
    letter-spacing: var(--tracking-label);
    text-transform: uppercase;
  }

  .metric dd {
    margin: 0;
    color: var(--color-text-primary);
    font-family: var(--font-mono);
    font-size: var(--text-mono);
    font-variant-numeric: tabular-nums;
  }

  .metric dd.feed {
    font-family: var(--font-ui);
    font-size: var(--text-xs);
  }

  .actions {
    display: flex;
    gap: 2px;
    margin-left: auto;
  }

  .empty {
    margin: 0;
    color: var(--color-text-tertiary);
    font-size: var(--text-xs);
  }

  /* Entries: one dense row each. */
  .entry-list {
    display: grid;
    border-top: 1px solid var(--color-border-subtle);
  }

  .entry {
    display: grid;
    grid-template-columns: 84px 64px 96px minmax(0, 1fr) auto auto;
    align-items: baseline;
    gap: var(--space-3);
    padding: 5px var(--space-2);
    border-bottom: 1px solid var(--color-border-subtle);
  }

  .entry.session-match {
    background: var(--color-primary-muted);
    box-shadow: inset 2px 0 0 var(--color-primary);
  }

  .mono {
    font-family: var(--font-mono);
    font-size: var(--text-mono);
  }

  .time {
    color: var(--color-text-tertiary);
    font-variant-numeric: tabular-nums;
  }

  .severity {
    color: var(--color-info-text);
    font-size: var(--text-label);
    font-weight: var(--font-semibold);
    letter-spacing: var(--tracking-label);
    text-transform: uppercase;
  }

  .severity.warning {
    color: var(--color-warning-text);
  }

  .severity.error {
    color: var(--color-danger-text);
  }

  .kind {
    color: var(--color-text-tertiary);
    font-size: var(--text-xs);
  }

  .entry-text {
    display: flex;
    flex-wrap: wrap;
    column-gap: var(--space-2);
    min-width: 0;
  }

  .entry-title {
    color: var(--color-text-primary);
    font-weight: var(--font-medium);
  }

  .entry-message {
    color: var(--color-text-secondary);
  }

  .details {
    color: var(--color-text-muted);
    font-size: var(--text-xs);
  }

  .source {
    color: var(--color-text-muted);
    white-space: nowrap;
  }

  .jumps {
    display: flex;
    gap: 2px;
  }

  .jump-btn {
    height: 20px;
    padding: 0 6px;
    border: 1px solid var(--color-border-default);
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--color-text-secondary);
    font: inherit;
    font-size: var(--text-xs);
    cursor: pointer;
    transition: var(--transition-colors);
  }

  .jump-btn:hover {
    background: var(--color-bg-hover);
    color: var(--color-text-primary);
  }

  .jump-btn:focus-visible {
    outline: 2px solid var(--color-focus-ring);
    outline-offset: 1px;
  }

  @media (max-width: 900px) {
    .entry {
      grid-template-columns: 84px 64px minmax(0, 1fr);
    }

    .kind,
    .source,
    .jumps {
      grid-column: 3;
    }
  }
</style>
