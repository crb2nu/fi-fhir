<script lang="ts">
  /**
   * TaskPanel Component
   *
   * Sidebar-style task management panel for integration tasks.
   * Shows priority-sorted tasks with filtering, inline creation,
   * and expandable detail views.
   */
  import { onMount } from 'svelte';
  import {
    collaborationState,
    fetchPresence,
    createTask,
    updateTaskStatus,
    assignTask,
    sortTasks,
    CURRENT_AGENT_ID
  } from './collaborationStore';
  import type { IntegrationTask } from './collaborationStore';

  type StatusFilter = 'all' | IntegrationTask['status'];
  type SortMode = 'priority' | 'updated' | 'created';

  let statusFilter: StatusFilter = 'all';
  let sortMode: SortMode = 'priority';
  let expandedTaskId: string | null = null;
  let showNewForm = false;

  // New task form state
  let newTitle = '';
  let newDescription = '';
  let newPriority: IntegrationTask['priority'] = 'medium';
  let newAssignee = '';

  onMount(() => {
    if ($collaborationState.tasks.length === 0) {
      fetchPresence();
    }
  });

  function toggleExpand(id: string): void {
    expandedTaskId = expandedTaskId === id ? null : id;
  }

  function priorityColor(priority: IntegrationTask['priority']): string {
    if (priority === 'critical') return '#ef4444';
    if (priority === 'high') return '#f59e0b';
    if (priority === 'medium') return '#6366f1';
    return '#94a3b8';
  }

  function statusVariant(status: IntegrationTask['status']): string {
    if (status === 'pending') return 'default';
    if (status === 'in_progress') return 'primary';
    if (status === 'completed') return 'success';
    return 'danger';
  }

  function statusLabel(status: IntegrationTask['status']): string {
    if (status === 'in_progress') return 'In Progress';
    if (status === 'blocked') return 'Blocked';
    return status.charAt(0).toUpperCase() + status.slice(1);
  }

  function assigneeLabel(task: IntegrationTask): string {
    if (!task.assignee) return 'Unassigned';
    if (task.assignee === CURRENT_AGENT_ID) return 'You';
    const agent = $collaborationState.presence.find(
      (a) => a.agentId === task.assignee
    );
    return agent?.displayName ?? task.assignee;
  }

  function formatTimestamp(ts: number): string {
    const diff = Date.now() - ts;
    if (diff < 60_000) return 'just now';
    if (diff < 3_600_000) return `${Math.floor(diff / 60_000)}m ago`;
    if (diff < 86_400_000) return `${Math.floor(diff / 3_600_000)}h ago`;
    return `${Math.floor(diff / 86_400_000)}d ago`;
  }

  async function handleCreateTask(): Promise<void> {
    if (!newTitle.trim()) return;
    await createTask({
      title: newTitle.trim(),
      description: newDescription.trim() || undefined,
      status: 'pending',
      priority: newPriority,
      assignee: newAssignee || undefined,
      creator: CURRENT_AGENT_ID,
      stage: undefined,
      blockedBy: undefined
    });
    newTitle = '';
    newDescription = '';
    newPriority = 'medium';
    newAssignee = '';
    showNewForm = false;
  }

  async function handleAssignToSelf(id: string): Promise<void> {
    await assignTask(id, CURRENT_AGENT_ID);
  }

  async function handleMarkComplete(id: string): Promise<void> {
    await updateTaskStatus(id, 'completed');
  }

  async function handleMarkBlocked(id: string): Promise<void> {
    await updateTaskStatus(id, 'blocked');
  }

  function applyFilters(tasks: IntegrationTask[]): IntegrationTask[] {
    let filtered = tasks;
    if (statusFilter !== 'all') {
      filtered = filtered.filter((t) => t.status === statusFilter);
    }
    if (sortMode === 'priority') {
      return sortTasks(filtered);
    }
    if (sortMode === 'updated') {
      return [...filtered].sort((a, b) => b.updatedAt - a.updatedAt);
    }
    return [...filtered].sort((a, b) => b.createdAt - a.createdAt);
  }

  $: filteredTasks = applyFilters($collaborationState.tasks);
</script>

<div class="task-panel">
  <header class="panel-header">
    <span class="panel-title">Integration Tasks</span>
    <button
      type="button"
      class="new-task-btn"
      on:click={() => { showNewForm = !showNewForm; }}
      aria-label="New task"
    >
      <svg viewBox="0 0 16 16" fill="currentColor" aria-hidden="true">
        <path d="M8 2a1 1 0 011 1v4h4a1 1 0 110 2H9v4a1 1 0 11-2 0V9H3a1 1 0 010-2h4V3a1 1 0 011-1z" />
      </svg>
      New
    </button>
  </header>

  {#if showNewForm}
    <form class="new-task-form" on:submit|preventDefault={handleCreateTask}>
      <input
        type="text"
        class="form-input"
        placeholder="Task title..."
        bind:value={newTitle}
      />
      <textarea
        class="form-textarea"
        placeholder="Description (optional)"
        rows="2"
        bind:value={newDescription}
      ></textarea>
      <div class="form-row">
        <select class="form-select" bind:value={newPriority}>
          <option value="low">Low</option>
          <option value="medium">Medium</option>
          <option value="high">High</option>
          <option value="critical">Critical</option>
        </select>
        <select class="form-select" bind:value={newAssignee}>
          <option value="">Unassigned</option>
          {#each $collaborationState.presence as agent (agent.agentId)}
            <option value={agent.agentId}>{agent.displayName}</option>
          {/each}
        </select>
      </div>
      <div class="form-actions">
        <button
          type="button"
          class="form-btn cancel"
          on:click={() => { showNewForm = false; }}
        >
          Cancel
        </button>
        <button
          type="submit"
          class="form-btn submit"
          disabled={!newTitle.trim()}
        >
          Create
        </button>
      </div>
    </form>
  {/if}

  <div class="task-list">
    {#each filteredTasks as task (task.id)}
      <div
        class="task-card"
        class:completed={task.status === 'completed'}
        class:expanded={expandedTaskId === task.id}
        style="border-left-color: {priorityColor(task.priority)}"
      >
        <button
          type="button"
          class="task-summary"
          on:click={() => toggleExpand(task.id)}
          aria-expanded={expandedTaskId === task.id}
        >
          <div class="task-priority-dot" style="background-color: {priorityColor(task.priority)}"></div>
          <div class="task-main">
            <span class="task-title">{task.title}</span>
            <div class="task-meta">
              <span class="task-status-badge {statusVariant(task.status)}">
                {#if task.status === 'blocked'}
                  <svg viewBox="0 0 12 12" fill="currentColor" class="chain-icon" aria-hidden="true">
                    <path d="M3.5 5.5a1 1 0 100 2h1a1 1 0 100-2h-1zm4 0a1 1 0 100 2h1a1 1 0 100-2h-1zM2 6.5a2.5 2.5 0 012.5-2.5h1a2.5 2.5 0 010 5h-1A2.5 2.5 0 012 6.5zm6-2.5a2.5 2.5 0 000 5h1a2.5 2.5 0 000-5h-1z" />
                  </svg>
                {/if}
                {statusLabel(task.status)}
              </span>
              <span class="task-assignee">{assigneeLabel(task)}</span>
            </div>
          </div>
        </button>

        {#if expandedTaskId === task.id}
          <div class="task-detail">
            {#if task.description}
              <p class="task-description">{task.description}</p>
            {/if}

            {#if task.stage}
              <div class="detail-row">
                <span class="detail-label">Stage</span>
                <span class="detail-value">{task.stage}</span>
              </div>
            {/if}

            {#if task.blockedBy && task.blockedBy.length > 0}
              <div class="detail-row">
                <span class="detail-label">Blocked by</span>
                <span class="detail-value">
                  {#each task.blockedBy as blockId (blockId)}
                    {@const blocking = $collaborationState.tasks.find(
                      (t) => t.id === blockId
                    )}
                    <span class="blocked-ref">{blocking?.title ?? blockId}</span>
                  {/each}
                </span>
              </div>
            {/if}

            <div class="detail-row">
              <span class="detail-label">Created</span>
              <span class="detail-value">{formatTimestamp(task.createdAt)}</span>
            </div>

            <div class="detail-row">
              <span class="detail-label">Updated</span>
              <span class="detail-value">{formatTimestamp(task.updatedAt)}</span>
            </div>

            <div class="task-actions">
              {#if task.assignee !== CURRENT_AGENT_ID && task.status !== 'completed'}
                <button
                  type="button"
                  class="action-btn"
                  on:click={() => handleAssignToSelf(task.id)}
                >
                  Assign to me
                </button>
              {/if}
              {#if task.status !== 'completed'}
                <button
                  type="button"
                  class="action-btn complete"
                  on:click={() => handleMarkComplete(task.id)}
                >
                  Mark complete
                </button>
              {/if}
              {#if task.status !== 'blocked' && task.status !== 'completed'}
                <button
                  type="button"
                  class="action-btn blocked"
                  on:click={() => handleMarkBlocked(task.id)}
                >
                  Mark blocked
                </button>
              {/if}
            </div>
          </div>
        {/if}
      </div>
    {/each}

    {#if filteredTasks.length === 0}
      <div class="empty-state">
        <span class="empty-text">No tasks match the current filter.</span>
      </div>
    {/if}
  </div>

  <footer class="panel-footer">
    <select
      class="filter-select"
      bind:value={statusFilter}
      aria-label="Filter by status"
    >
      <option value="all">All</option>
      <option value="pending">Pending</option>
      <option value="in_progress">In Progress</option>
      <option value="completed">Completed</option>
      <option value="blocked">Blocked</option>
    </select>
    <select
      class="filter-select"
      bind:value={sortMode}
      aria-label="Sort tasks"
    >
      <option value="priority">Priority</option>
      <option value="updated">Updated</option>
      <option value="created">Created</option>
    </select>
  </footer>
</div>

<style>
  .task-panel {
    display: flex;
    flex-direction: column;
    border: 1px solid var(--color-border-subtle);
    border-radius: var(--panel-radius);
    background: var(--color-bg-elevated);
    overflow: hidden;
    box-shadow: var(--shadow-sm);
  }

  /* Header */
  .panel-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: var(--space-3) var(--space-4);
    border-bottom: 1px solid var(--color-border-subtle);
  }

  .panel-title {
    font-size: var(--text-sm);
    font-weight: var(--font-bold);
    color: var(--color-text-primary);
    letter-spacing: var(--tracking-tight);
  }

  .new-task-btn {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
    padding: var(--space-1) var(--space-2);
    border: 1px solid var(--color-border-default);
    border-radius: var(--radius-md);
    background: transparent;
    color: var(--color-text-secondary);
    font-size: var(--text-xs);
    font-family: inherit;
    font-weight: var(--font-medium);
    cursor: pointer;
    transition: var(--transition-colors);
  }

  .new-task-btn:hover {
    background: var(--color-bg-hover);
    border-color: var(--color-primary-border);
    color: var(--color-primary);
  }

  .new-task-btn:focus-visible {
    outline: none;
    box-shadow: var(--shadow-focus);
  }

  .new-task-btn svg {
    width: 14px;
    height: 14px;
  }

  /* New task form */
  .new-task-form {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    padding: var(--space-3) var(--space-4);
    border-bottom: 1px solid var(--color-border-subtle);
    background: var(--color-bg-surface);
    animation: slideInDown var(--duration-fast) var(--ease-out);
  }

  .form-input,
  .form-textarea,
  .form-select {
    font-family: inherit;
    font-size: var(--text-xs);
    color: var(--color-text-primary);
    background: var(--color-bg-input);
    border: 1px solid var(--color-border-default);
    border-radius: var(--radius-md);
    padding: var(--space-2) var(--space-3);
    transition: var(--transition-colors);
  }

  .form-input:focus,
  .form-textarea:focus,
  .form-select:focus {
    outline: none;
    border-color: var(--color-border-focus);
    box-shadow: var(--shadow-focus);
  }

  .form-textarea {
    resize: vertical;
    min-height: 48px;
  }

  .form-row {
    display: flex;
    gap: var(--space-2);
  }

  .form-row .form-select {
    flex: 1;
  }

  .form-actions {
    display: flex;
    justify-content: flex-end;
    gap: var(--space-2);
  }

  .form-btn {
    padding: var(--space-1) var(--space-3);
    border-radius: var(--radius-md);
    border: 1px solid var(--color-border-default);
    background: transparent;
    color: var(--color-text-secondary);
    font-size: var(--text-xs);
    font-family: inherit;
    font-weight: var(--font-medium);
    cursor: pointer;
    transition: var(--transition-colors);
  }

  .form-btn:hover {
    background: var(--color-bg-hover);
  }

  .form-btn.submit {
    background: var(--color-bg-surface);
    border-color: var(--color-primary-border);
    color: var(--color-primary);
  }

  .form-btn.submit:hover:not(:disabled) {
    background: var(--color-primary-muted);
  }

  .form-btn:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  /* Task list */
  .task-list {
    flex: 1;
    overflow-y: auto;
  }

  /* Task card */
  .task-card {
    border-left: 3px solid transparent;
    border-bottom: 1px solid var(--color-border-subtle);
    transition:
      background-color var(--duration-fast) var(--ease-out),
      border-left-color var(--duration-fast) var(--ease-out);
    animation: slideInUp var(--duration-slow) var(--ease-out) both;
  }

  .task-card:last-child {
    border-bottom: none;
  }

  .task-card:hover {
    background: var(--color-bg-hover);
  }

  .task-card.completed {
    opacity: 0.5;
  }

  /* Task summary (clickable row) */
  .task-summary {
    display: flex;
    align-items: flex-start;
    gap: var(--space-2);
    width: 100%;
    padding: var(--space-3) var(--space-4);
    background: none;
    border: none;
    cursor: pointer;
    text-align: left;
    font-family: inherit;
    transition: var(--transition-colors);
  }

  .task-summary:focus-visible {
    outline: none;
    box-shadow: inset var(--shadow-focus);
  }

  .task-priority-dot {
    width: 8px;
    height: 8px;
    border-radius: var(--radius-full);
    flex-shrink: 0;
    margin-top: 4px;
  }

  .task-main {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
  }

  .task-title {
    font-size: var(--text-sm);
    font-weight: var(--font-medium);
    color: var(--color-text-primary);
    line-height: var(--leading-tight);
  }

  .completed .task-title {
    text-decoration: line-through;
    color: var(--color-text-muted);
  }

  .task-meta {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    flex-wrap: wrap;
  }

  /* Status badge */
  .task-status-badge {
    display: inline-flex;
    align-items: center;
    gap: 3px;
    font-size: var(--text-2xs);
    font-weight: var(--font-medium);
    padding: 1px var(--space-2);
    border-radius: var(--radius-sm);
    white-space: nowrap;
  }

  .task-status-badge.default {
    color: var(--color-text-secondary);
    background: var(--color-bg-surface);
    border: 1px solid var(--color-border-default);
  }

  .task-status-badge.primary {
    color: var(--color-primary);
    background: var(--color-primary-muted);
    border: 1px solid var(--color-primary-border);
  }

  .task-status-badge.success {
    color: var(--color-success-text);
    background: var(--color-success-bg);
    border: 1px solid var(--color-success-border);
  }

  .task-status-badge.danger {
    color: var(--color-danger-text);
    background: var(--color-danger-bg);
    border: 1px solid var(--color-danger-border);
  }

  .chain-icon {
    width: 10px;
    height: 10px;
  }

  .task-assignee {
    font-size: var(--text-2xs);
    color: var(--color-text-muted);
  }

  /* Task detail (expanded) */
  .task-detail {
    padding: 0 var(--space-4) var(--space-3) calc(var(--space-4) + 10px + var(--space-2));
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    animation: slideInDown var(--duration-fast) var(--ease-out);
  }

  .task-description {
    margin: 0;
    font-size: var(--text-xs);
    color: var(--color-text-secondary);
    line-height: var(--leading-normal);
  }

  .detail-row {
    display: flex;
    align-items: baseline;
    gap: var(--space-2);
  }

  .detail-label {
    font-size: var(--text-2xs);
    color: var(--color-text-muted);
    text-transform: uppercase;
    letter-spacing: var(--tracking-wider);
    flex-shrink: 0;
    min-width: 60px;
  }

  .detail-value {
    font-size: var(--text-xs);
    color: var(--color-text-secondary);
  }

  .blocked-ref {
    font-size: var(--text-2xs);
    font-family: var(--font-mono);
    background: var(--color-danger-bg);
    color: var(--color-danger-text);
    padding: 1px var(--space-1);
    border-radius: var(--radius-sm);
  }

  /* Task actions */
  .task-actions {
    display: flex;
    gap: var(--space-2);
    padding-top: var(--space-2);
    border-top: 1px solid var(--color-border-subtle);
  }

  .action-btn {
    padding: var(--space-1) var(--space-2);
    border: 1px solid var(--color-border-default);
    border-radius: var(--radius-md);
    background: transparent;
    color: var(--color-text-secondary);
    font-size: var(--text-2xs);
    font-family: inherit;
    font-weight: var(--font-medium);
    cursor: pointer;
    transition: var(--transition-colors);
  }

  .action-btn:hover {
    background: var(--color-bg-hover);
    border-color: var(--color-border-strong);
  }

  .action-btn.complete:hover {
    border-color: var(--color-success-border);
    color: var(--color-success-text);
    background: var(--color-success-bg);
  }

  .action-btn.blocked:hover {
    border-color: var(--color-danger-border);
    color: var(--color-danger-text);
    background: var(--color-danger-bg);
  }

  .action-btn:focus-visible {
    outline: none;
    box-shadow: var(--shadow-focus);
  }

  /* Empty state */
  .empty-state {
    display: flex;
    align-items: center;
    justify-content: center;
    padding: var(--space-8);
  }

  .empty-text {
    font-size: var(--text-xs);
    color: var(--color-text-muted);
  }

  /* Footer */
  .panel-footer {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    padding: var(--space-2) var(--space-4);
    border-top: 1px solid var(--color-border-subtle);
    background: var(--color-bg-surface);
  }

  .filter-select {
    flex: 1;
    font-family: inherit;
    font-size: var(--text-2xs);
    color: var(--color-text-secondary);
    background: var(--color-bg-input);
    border: 1px solid var(--color-border-default);
    border-radius: var(--radius-sm);
    padding: var(--space-1) var(--space-2);
    cursor: pointer;
  }

  .filter-select:focus {
    outline: none;
    border-color: var(--color-border-focus);
    box-shadow: var(--shadow-focus);
  }

  /* Animations */
  @keyframes slideInUp {
    from { opacity: 0; transform: translateY(6px); }
    to { opacity: 1; transform: translateY(0); }
  }

  @keyframes slideInDown {
    from { opacity: 0; transform: translateY(-4px); }
    to { opacity: 1; transform: translateY(0); }
  }

  @media (prefers-reduced-motion: reduce) {
    .task-card,
    .new-task-form,
    .task-detail {
      animation: none;
    }
  }
</style>
