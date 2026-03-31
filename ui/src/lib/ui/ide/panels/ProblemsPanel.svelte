<script lang="ts">
  import { workflowDiagnostics } from './workflowProblemsStore';

  function pluralize(count: number, noun: string): string {
    return `${count} ${noun}${count === 1 ? '' : 's'}`;
  }
</script>

<div class="panel">
  <div class="summary">
    <div class="summary-copy">
      <div class="summary-title">
        {#if $workflowDiagnostics.isValid}
          Workflow draft is valid
        {:else}
          Workflow draft has {$workflowDiagnostics.issues.length} issue{#if $workflowDiagnostics.issues.length !== 1}s{/if}
        {/if}
      </div>
      <div class="summary-subtitle">
        {$workflowDiagnostics.draft.name || 'Untitled workflow'} · version {$workflowDiagnostics.draft.version}
      </div>
    </div>

    <div class="summary-stats">
      <span class="stat">{pluralize($workflowDiagnostics.routeCount, 'route')}</span>
      <span class="stat">{pluralize($workflowDiagnostics.transformCount, 'transform')}</span>
      <span class="stat">{pluralize($workflowDiagnostics.actionCount, 'action')}</span>
    </div>
  </div>

  {#if $workflowDiagnostics.issues.length === 0}
    <div class="empty success">
      <div class="empty-title">No blocking problems detected.</div>
      <div class="empty-body">
        The current workflow draft has enough structure to keep moving: one or more routes are present and required action fields are satisfied.
      </div>
    </div>
  {:else}
    <div class="issue-list" role="list" aria-label="Workflow diagnostics">
      {#each $workflowDiagnostics.issues as issue (issue.id)}
        <article class="issue" role="listitem">
          <div class="issue-head">
            <span class="badge" class:error={issue.severity === 'error'} class:warning={issue.severity === 'warning'}>
              {issue.severity}
            </span>
            <span class="location">{issue.location}</span>
          </div>
          <div class="issue-message">{issue.message}</div>
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

  .summary {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: var(--space-3);
    flex-wrap: wrap;
  }

  .summary-copy {
    display: grid;
    gap: 4px;
    min-width: 0;
  }

  .summary-title {
    font-size: var(--text-sm);
    font-weight: var(--font-semibold);
    color: var(--color-text-primary);
  }

  .summary-subtitle {
    color: var(--color-text-tertiary);
    font-size: var(--text-xs);
  }

  .summary-stats {
    display: flex;
    gap: var(--space-2);
    flex-wrap: wrap;
  }

  .stat {
    padding: 2px 8px;
    border-radius: 999px;
    font-size: var(--text-xs);
    color: var(--color-text-secondary);
    background: var(--color-bg-surface);
    border: 1px solid var(--color-border-subtle);
  }

  .empty {
    padding: var(--space-4);
    border-radius: var(--radius-lg);
    border: 1px solid rgba(16, 185, 129, 0.2);
    background: rgba(16, 185, 129, 0.08);
    display: grid;
    gap: 6px;
  }

  .empty-title {
    color: var(--color-text-primary);
    font-weight: var(--font-semibold);
  }

  .empty-body {
    color: var(--color-text-secondary);
    font-size: var(--text-sm);
    line-height: 1.45;
  }

  .issue-list {
    display: grid;
    gap: var(--space-2);
    max-height: 100%;
    overflow: auto;
  }

  .issue {
    display: grid;
    gap: 4px;
    padding: var(--space-3);
    border: 1px solid rgba(239, 68, 68, 0.22);
    border-radius: var(--radius-lg);
    background: rgba(239, 68, 68, 0.07);
  }

  .issue-head {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    flex-wrap: wrap;
  }

  .badge {
    padding: 2px 8px;
    border-radius: 999px;
    font-size: 10px;
    font-weight: var(--font-semibold);
    letter-spacing: 0.04em;
    text-transform: uppercase;
    background: rgba(59, 130, 246, 0.12);
    border: 1px solid rgba(59, 130, 246, 0.22);
    color: rgba(191, 219, 254, 0.95);
  }

  .badge.warning {
    background: rgba(245, 158, 11, 0.12);
    border-color: rgba(245, 158, 11, 0.22);
    color: rgba(254, 240, 138, 0.95);
  }

  .badge.error {
    background: rgba(239, 68, 68, 0.12);
    border-color: rgba(239, 68, 68, 0.22);
    color: rgba(254, 202, 202, 0.96);
  }

  .location {
    font-weight: var(--font-semibold);
    color: var(--color-text-primary);
  }

  .issue-message {
    color: var(--color-text-secondary);
    font-size: var(--text-sm);
    line-height: 1.45;
  }
</style>
