<script lang="ts">
  /**
   * LogViewer — Filterable log stream with level/workflow filters and tail mode.
   */
  import { onMount, onDestroy, tick } from 'svelte';
  import {
    observabilityState,
    filteredLogs,
    fetchLogs,
    setLogFilter,
    isAvailable,
    type LogEntry,
  } from './observabilityStore';

  let tailMode = false;
  let logContainer: HTMLDivElement | undefined = undefined;
  let refreshInterval: ReturnType<typeof setInterval> | null = null;

  const LEVEL_OPTIONS = ['', 'debug', 'info', 'warn', 'error'] as const;
  const WORKFLOW_OPTIONS = ['', 'ADT-to-FHIR', 'ORM-routing', 'Lab-result-pipeline', 'Pharmacy-feed'] as const;

  let selectedLevel = '';
  let selectedWorkflow = '';
  let searchText = '';

  function formatTime(ts: number): string {
    return new Date(ts).toLocaleTimeString('en-US', { hour12: false });
  }

  function levelClass(level: LogEntry['level']): string {
    switch (level) {
      case 'debug': return 'level-debug';
      case 'info': return 'level-info';
      case 'warn': return 'level-warn';
      case 'error': return 'level-error';
      default: return '';
    }
  }

  function handleLevelChange(event: Event) {
    const target = event.target as HTMLSelectElement;
    selectedLevel = target.value;
    setLogFilter({ level: selectedLevel || undefined, workflowName: selectedWorkflow || undefined, search: searchText || undefined });
  }

  function handleWorkflowChange(event: Event) {
    const target = event.target as HTMLSelectElement;
    selectedWorkflow = target.value;
    setLogFilter({ level: selectedLevel || undefined, workflowName: selectedWorkflow || undefined, search: searchText || undefined });
  }

  function handleSearchInput(event: Event) {
    const target = event.target as HTMLInputElement;
    searchText = target.value;
    setLogFilter({ level: selectedLevel || undefined, workflowName: selectedWorkflow || undefined, search: searchText || undefined });
  }

  function toggleTail() {
    tailMode = !tailMode;
    if (tailMode) {
      scrollToBottom();
    }
  }

  async function scrollToBottom() {
    await tick();
    if (logContainer) {
      logContainer.scrollTop = 0;
    }
  }

  $: logs = $filteredLogs;
  $: connected = $isAvailable;
  $: loading = $observabilityState.isLoadingLogs;

  $: if (tailMode && logs.length > 0) {
    scrollToBottom();
  }

  onMount(() => {
    fetchLogs();
    refreshInterval = setInterval(() => fetchLogs(), 15_000);
  });

  onDestroy(() => {
    if (refreshInterval) clearInterval(refreshInterval);
  });
</script>

<div class="log-viewer">
  <header class="viewer-header">
    <h3 class="viewer-title">Logs</h3>
    <div class="filter-controls">
      <select class="filter-select" value={selectedLevel} on:change={handleLevelChange} aria-label="Filter by level">
        <option value="">All Levels</option>
        {#each LEVEL_OPTIONS.slice(1) as lvl (lvl)}
          <option value={lvl}>{lvl.toUpperCase()}</option>
        {/each}
      </select>

      <select class="filter-select" value={selectedWorkflow} on:change={handleWorkflowChange} aria-label="Filter by workflow">
        <option value="">All Workflows</option>
        {#each WORKFLOW_OPTIONS.slice(1) as wf (wf)}
          <option value={wf}>{wf}</option>
        {/each}
      </select>

      <input
        class="filter-input"
        type="text"
        placeholder="Search..."
        value={searchText}
        on:input={handleSearchInput}
        aria-label="Search logs"
      />

      <button
        class="tail-btn"
        class:active={tailMode}
        on:click={toggleTail}
        title={tailMode ? 'Stop auto-scroll' : 'Auto-scroll to newest'}
      >
        Tail
      </button>
    </div>
  </header>

  <div class="log-list" bind:this={logContainer}>
    {#if !connected}
      <div class="empty-state">
        <span class="empty-text">Connect to platform for live logs</span>
      </div>
    {:else if loading && logs.length === 0}
      <div class="empty-state">
        <span class="empty-text">Loading logs...</span>
      </div>
    {:else if logs.length === 0}
      <div class="empty-state">
        <span class="empty-text">No logs match the current filters</span>
      </div>
    {:else}
      {#each logs as entry, i (entry.timestamp.toString() + i)}
        <div
          class="log-row"
          class:row-error={entry.level === 'error'}
          class:row-alt={i % 2 === 1}
          class:tail-anim={tailMode && i === 0}
        >
          <span class="log-time">{formatTime(entry.timestamp)}</span>
          <span class="log-level {levelClass(entry.level)}">
            <span class="level-dot" aria-hidden="true"></span>
            {entry.level.toUpperCase()}
          </span>
          <span class="log-workflow">{entry.workflowName ?? '-'}</span>
          <span class="log-message">{entry.message}</span>
        </div>
      {/each}
    {/if}
  </div>

  <footer class="viewer-footer">
    <span class="footer-count">{logs.length} entries</span>
    {#if tailMode}
      <span class="tail-indicator">TAIL</span>
    {/if}
  </footer>
</div>

<style>
  .log-viewer {
    display: grid;
    grid-template-rows: auto 1fr auto;
    height: 100%;
    gap: 0;
    border: 1px solid var(--color-border-subtle);
    border-radius: var(--radius-lg);
    overflow: hidden;
    background: var(--color-bg-elevated);
  }

  .viewer-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: var(--space-2) var(--space-3);
    border-bottom: 1px solid var(--color-border-subtle);
    background: var(--color-bg-surface);
    flex-wrap: wrap;
    gap: var(--space-2);
  }

  .viewer-title {
    font-size: var(--text-xs);
    font-weight: var(--font-bold);
    color: var(--color-text-primary);
    margin: 0;
    white-space: nowrap;
  }

  .filter-controls {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    flex-wrap: wrap;
  }

  .filter-select,
  .filter-input {
    height: 26px;
    padding: 0 var(--space-2);
    font-size: var(--text-2xs);
    border: 1px solid var(--color-border-default);
    border-radius: var(--radius-sm);
    background: var(--color-bg-input);
    color: var(--color-text-primary);
    outline: none;
  }

  .filter-select:focus,
  .filter-input:focus {
    border-color: var(--color-border-focus);
    box-shadow: var(--shadow-focus);
  }

  .filter-input {
    width: 120px;
  }

  .tail-btn {
    height: 26px;
    padding: 0 var(--space-2);
    font-size: var(--text-2xs);
    font-weight: var(--font-semibold);
    border: 1px solid var(--color-border-default);
    border-radius: var(--radius-sm);
    background: var(--color-bg-surface);
    color: var(--color-text-secondary);
    cursor: pointer;
    transition: var(--transition-all);
  }

  .tail-btn:hover {
    background: var(--color-bg-hover);
  }

  .tail-btn.active {
    background: var(--color-primary-muted);
    border-color: var(--color-primary-border);
    color: var(--color-primary);
  }

  /* Log list */
  .log-list {
    overflow-y: auto;
    max-height: 360px;
  }

  .empty-state {
    display: flex;
    align-items: center;
    justify-content: center;
    padding: var(--space-8);
    color: var(--color-text-muted);
  }

  .empty-text {
    font-size: var(--text-xs);
  }

  /* Log rows */
  .log-row {
    display: grid;
    grid-template-columns: 68px 64px 130px 1fr;
    gap: var(--space-2);
    align-items: center;
    height: 28px;
    padding: 0 var(--space-3);
    font-size: var(--text-xs);
    transition: background-color var(--duration-fast) var(--ease-out);
    border-left: 2px solid transparent;
  }

  .log-row.row-alt {
    background: var(--color-bg-elevated);
  }

  .log-row.row-error {
    border-left-color: rgba(239, 68, 68, 0.3);
  }

  .log-row:hover {
    background: var(--color-bg-hover);
  }

  .log-row.tail-anim {
    animation: slideInUp var(--duration-normal) var(--ease-out);
  }

  .log-time {
    font-family: var(--font-mono);
    font-size: var(--text-2xs);
    color: var(--color-text-muted);
    white-space: nowrap;
  }

  .log-level {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    font-size: var(--text-2xs);
    font-weight: var(--font-semibold);
    text-transform: uppercase;
    letter-spacing: var(--tracking-wide);
    white-space: nowrap;
  }

  .level-dot {
    width: 4px;
    height: 4px;
    border-radius: 50%;
    flex: 0 0 auto;
  }

  .level-debug { color: var(--color-text-muted); }
  .level-debug .level-dot { background: var(--color-text-muted); }

  .level-info { color: var(--color-info); }
  .level-info .level-dot { background: var(--color-info); }

  .level-warn { color: var(--color-warning); }
  .level-warn .level-dot { background: var(--color-warning); }

  .level-error { color: var(--color-danger); }
  .level-error .level-dot { background: var(--color-danger); }

  .log-workflow {
    font-family: var(--font-mono);
    font-size: var(--text-2xs);
    color: var(--color-text-tertiary);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .log-message {
    font-family: var(--font-mono);
    font-size: var(--text-2xs);
    color: var(--color-text-secondary);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  /* Footer */
  .viewer-footer {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: var(--space-1) var(--space-3);
    border-top: 1px solid var(--color-border-subtle);
    background: var(--color-bg-surface);
  }

  .footer-count {
    font-size: var(--text-2xs);
    color: var(--color-text-muted);
  }

  .tail-indicator {
    font-size: var(--text-2xs);
    font-weight: var(--font-bold);
    color: var(--color-primary);
    letter-spacing: var(--tracking-wider);
  }

  /* Animations */
  @keyframes slideInUp {
    from {
      opacity: 0;
      transform: translateY(4px);
    }
    to {
      opacity: 1;
      transform: translateY(0);
    }
  }


  @media (prefers-reduced-motion: reduce) {
    .log-row.tail-anim {
      animation: none;
    }
    .tail-indicator {
      animation: none;
    }
  }
</style>
