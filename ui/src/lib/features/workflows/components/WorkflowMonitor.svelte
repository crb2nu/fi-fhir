<script lang="ts">
  import { onDestroy } from 'svelte';
  import { subscribe as wsSubscribe } from '$lib/graphql/subscriptions';
  import { WorkflowEventsDocument, type WorkflowEventsSubscription } from '$lib/gen/graphql';
  import Panel from '$lib/ui/Panel.svelte';
  import Button from '$lib/ui/Button.svelte';

  type WfEvent = WorkflowEventsSubscription['workflowEvents'];

  let workflowName = '';
  let events: WfEvent[] = [];
  let connected = false;
  let error: string | null = null;
  let paused = false;
  let unsubscribe: (() => void) | null = null;
  const maxEvents = 100;

  function startSubscription() {
    if (!workflowName.trim()) return;
    stopSubscription();
    error = null;
    connected = false;

    unsubscribe = wsSubscribe(
      WorkflowEventsDocument,
      { workflowName },
      {
        onData: (data) => {
          connected = true;
          if (!paused && data.workflowEvents) {
            events = [data.workflowEvents, ...events].slice(0, maxEvents);
          }
        },
        onError: (err) => {
          error = err.message;
          connected = false;
        },
        onComplete: () => {
          connected = false;
        }
      }
    );
  }

  function stopSubscription() {
    if (unsubscribe) {
      unsubscribe();
      unsubscribe = null;
    }
    connected = false;
  }

  function clearEvents() {
    events = [];
  }

  function formatTimestamp(ts: string): string {
    try {
      return new Date(ts).toLocaleTimeString();
    } catch {
      return ts;
    }
  }

  onDestroy(() => {
    stopSubscription();
  });
</script>

<Panel title="Workflow Event Monitor">
  <div class="monitor">
    <div class="connect-bar">
      <label class="field">
        Workflow Name
        <input
          type="text"
          class="input"
          bind:value={workflowName}
          placeholder="e.g. adt-routing"
          on:keydown={(e) => e.key === 'Enter' && startSubscription()}
        />
      </label>
      <div class="actions">
        <Button on:click={startSubscription} disabled={!workflowName.trim()}>
          {connected ? 'Reconnect' : 'Connect'}
        </Button>
        {#if connected}
          <Button variant="secondary" on:click={() => (paused = !paused)}>
            {paused ? 'Resume' : 'Pause'}
          </Button>
          <Button variant="secondary" on:click={clearEvents}>Clear</Button>
          <Button variant="secondary" on:click={stopSubscription}>Disconnect</Button>
        {/if}
      </div>
    </div>

    <div class="status" role="status" aria-live="polite">
      <span class="indicator" class:connected class:error={!!error}></span>
      {#if error}
        <span class="status-text error">{error}</span>
      {:else if connected}
        <span class="status-text">Connected to {workflowName}</span>
      {:else}
        <span class="status-text">Not connected</span>
      {/if}
    </div>

    {#if events.length === 0}
      <div class="empty">
        {#if connected}
          Waiting for workflow events...
        {:else}
          Enter a workflow name and click Connect to monitor events.
        {/if}
      </div>
    {:else}
      <div class="event-list">
        {#each events as ev, i (i)}
          <div class="event-row">
            <span class="time mono">{formatTimestamp(ev.event.timestamp)}</span>
            <span class="type-pill">{ev.event.type.replace(/_/g, ' ')}</span>
            <span class="routes mono">{ev.routesMatched.join(', ')}</span>
            <span class="actions-col">{ev.actionsExecuted.length} actions</span>
            <span class="duration muted">{ev.duration}ms</span>
          </div>
        {/each}
      </div>
      <div class="footer muted">
        {events.length} events
        {#if paused}<span class="paused-badge">PAUSED</span>{/if}
      </div>
    {/if}
  </div>
</Panel>

<style>
  .monitor {
    display: grid;
    gap: 12px;
  }

  .connect-bar {
    display: flex;
    gap: 12px;
    align-items: flex-end;
    flex-wrap: wrap;
  }

  .field {
    display: grid;
    gap: 6px;
    color: var(--color-text-tertiary);
    font-size: 0.9rem;
    font-weight: 700;
    min-width: 200px;
  }

  .input {
    padding: 8px 12px;
    border-radius: 10px;
    border: 1px solid var(--color-border-default);
    background: var(--color-bg-input);
    color: var(--color-text-primary);
    outline: none;
    transition: var(--transition-all);
  }

  .input::placeholder {
    color: var(--color-text-muted);
  }

  .input:hover:not(:disabled):not(:focus) {
    border-color: var(--color-border-strong);
  }

  .input:focus {
    border-color: var(--color-border-focus);
    box-shadow: var(--shadow-focus);
  }

  .actions {
    display: flex;
    gap: 8px;
  }

  .status {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .indicator {
    width: 10px;
    height: 10px;
    border-radius: 50%;
    background: rgba(156, 163, 175, 0.5);
    transition: background 0.2s ease;
  }

  .indicator.connected {
    background: rgba(16, 185, 129, 0.85);
    box-shadow: 0 0 8px rgba(16, 185, 129, 0.4);
  }

  .indicator.error {
    background: rgba(239, 68, 68, 0.85);
    box-shadow: 0 0 8px rgba(239, 68, 68, 0.4);
  }

  .status-text {
    font-weight: 700;
    color: var(--color-text-secondary);
  }

  .status-text.error {
    color: rgba(254, 202, 202, 0.9);
  }

  .empty {
    color: var(--color-text-tertiary);
    padding: 24px;
    text-align: center;
    border: 1px dashed var(--color-border-subtle);
    border-radius: 12px;
  }

  .event-list {
    display: grid;
    gap: 6px;
    max-height: 400px;
    overflow-y: auto;
  }

  .event-row {
    display: grid;
    grid-template-columns: 80px auto 1fr auto auto;
    gap: 12px;
    align-items: center;
    padding: 8px 12px;
    border-radius: 8px;
    border: 1px solid var(--color-border-subtle);
    background: var(--color-bg-surface);
  }

  .event-row:hover {
    background: var(--color-bg-hover);
  }

  .time {
    color: var(--color-text-tertiary);
    font-size: 0.85rem;
  }

  .type-pill {
    padding: 2px 8px;
    border-radius: 999px;
    font-size: 0.75rem;
    font-weight: 700;
    text-transform: uppercase;
    white-space: nowrap;
    background: rgba(59, 130, 246, 0.15);
    border: 1px solid rgba(59, 130, 246, 0.3);
    color: rgba(147, 197, 253, 0.95);
  }

  .routes {
    color: var(--color-text-secondary);
    font-size: 0.85rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .actions-col {
    color: var(--color-text-tertiary);
    font-size: 0.85rem;
    white-space: nowrap;
  }

  .duration {
    font-size: 0.85rem;
    white-space: nowrap;
  }

  .mono {
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  }

  .muted {
    color: var(--color-text-muted);
  }

  .footer {
    display: flex;
    justify-content: space-between;
    align-items: center;
    font-size: 0.85rem;
    padding-top: 8px;
    border-top: 1px solid var(--color-border-subtle);
  }

  .paused-badge {
    padding: 2px 8px;
    border-radius: 4px;
    background: rgba(245, 158, 11, 0.2);
    border: 1px solid rgba(245, 158, 11, 0.4);
    color: rgba(253, 230, 138, 0.95);
    font-weight: 700;
    font-size: 0.75rem;
  }
</style>
