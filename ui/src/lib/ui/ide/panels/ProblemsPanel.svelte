<script lang="ts">
  import {
    navigateToProblem,
    problemsDiagnostics,
    type WorkflowProblem,
    type WorkflowProblemSeverity,
  } from './workflowProblemsStore';

  // ── Derived view model ────────────────────────────────────────────────
  // Workflow validation stays in `validateWorkflowDraft`; server diagnostics
  // join that derived view without duplicating validation rules in the panel.

  let issues = $derived($problemsDiagnostics.issues);
  let isValid = $derived($problemsDiagnostics.isValid);
  let errorCount = $derived(issues.filter((i) => i.severity === 'error').length);
  let warningCount = $derived(issues.filter((i) => i.severity === 'warning').length);
  let infoCount = $derived(issues.filter((i) => i.severity === 'info').length);

  // ── Helpers ───────────────────────────────────────────────────────────

  const SEVERITY_LABELS: Record<WorkflowProblemSeverity, string> = {
    error: 'Error',
    warning: 'Warning',
    info: 'Info',
  };

  function pluralize(count: number, noun: string): string {
    return `${count} ${noun}${count === 1 ? '' : 's'}`;
  }

  // Compose the headline count, leading with the most severe band present so
  // the text label (not color) carries the signal.
  function countSummary(): string {
    const parts: string[] = [];
    if (errorCount > 0) parts.push(pluralize(errorCount, 'error'));
    if (warningCount > 0) parts.push(pluralize(warningCount, 'warning'));
    if (infoCount > 0) parts.push(pluralize(infoCount, 'info'));
    return parts.join(' · ');
  }

  function severityLabel(severity: WorkflowProblem['severity']): string {
    return SEVERITY_LABELS[severity];
  }
</script>

<div class="problems-panel">
  {#if isValid}
    <!-- Valid draft: calm, affirmative state (no blocking problems) -->
    <div class="state-card state-ok">
      <div class="state-icon" aria-hidden="true">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M9 12l2 2 4-4" />
          <circle cx="12" cy="12" r="10" />
        </svg>
      </div>
      <div class="state-text">
        <div class="state-title">Ready for runtime verification</div>
        <div class="state-body">No blocking problems detected.</div>
        <div class="state-meta">
          <span class="meta-chip">{pluralize($problemsDiagnostics.routeCount, 'route')}</span>
          <span class="meta-chip">{pluralize($problemsDiagnostics.actionCount, 'action')}</span>
          {#if $problemsDiagnostics.transformCount > 0}
            <span class="meta-chip">
              {pluralize($problemsDiagnostics.transformCount, 'transform')}
            </span>
          {/if}
        </div>
      </div>
    </div>
  {:else}
    <!-- Invalid draft: structured diagnostics, persistent until fixed -->
    <div class="state-card state-attention">
      <div class="state-icon" aria-hidden="true">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M12 9v4" />
          <path d="M12 17h.01" />
          <path d="M10.29 3.86 1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z" />
        </svg>
      </div>
      <div class="state-text">
        <div class="state-title">
          {$problemsDiagnostics.sessionCount > 0 ? 'Session diagnostics need attention' : 'Workflow draft needs attention'}
        </div>
        <div class="state-count">{countSummary()}</div>
        <div class="state-body">
          {$problemsDiagnostics.sessionCount > 0
            ? 'Select a server diagnostic to inspect its exact HL7 source field.'
            : 'Fix the listed issues before you trust the destination behavior.'}
        </div>
      </div>
    </div>

    <ul class="issue-list" aria-label="Workflow and session problems">
      {#each issues as issue (issue.id)}
        <li>
          <button
            class="issue-row severity-{issue.severity}"
            class:navigable={Boolean(issue.targetPath)}
            type="button"
            disabled={!issue.targetPath}
            onclick={() => navigateToProblem(issue)}
          >
            <span class="issue-strip severity-{issue.severity}" aria-hidden="true"></span>
            <span class="issue-severity severity-{issue.severity}">
              {severityLabel(issue.severity)}
            </span>
            <span class="issue-location">{issue.location}</span>
            <span class="issue-message">{issue.message}</span>
          </button>
        </li>
      {/each}
    </ul>
  {/if}
</div>

<style>
  /* ── Panel container ──────────────────────────────────────────────── */
  .problems-panel {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    height: 100%;
    overflow-y: auto;
    padding-top: var(--space-2);
    font-family: var(--font-sans);
  }

  /* ── Summary state card ───────────────────────────────────────────── */
  .state-card {
    display: flex;
    align-items: flex-start;
    gap: var(--space-3);
    padding: var(--space-3);
    border-radius: var(--radius-lg);
    border: 1px solid var(--color-border-subtle);
    flex-shrink: 0;
  }

  .state-card.state-ok {
    background: var(--color-success-bg);
    border-color: var(--color-success-border);
  }

  .state-card.state-attention {
    background: var(--color-danger-bg);
    border-color: var(--color-danger-border);
  }

  .state-icon {
    width: 28px;
    height: 28px;
    flex-shrink: 0;
  }

  .state-icon svg {
    width: 100%;
    height: 100%;
  }

  .state-ok .state-icon {
    color: var(--color-success-text);
  }

  .state-attention .state-icon {
    color: var(--color-danger-text);
  }

  .state-text {
    display: flex;
    flex-direction: column;
    gap: 2px;
    min-width: 0;
  }

  .state-title {
    font-family: var(--font-heading);
    font-size: var(--text-sm);
    font-weight: var(--font-semibold);
    color: var(--color-text-primary);
  }

  .state-count {
    font-size: var(--text-xs);
    font-weight: var(--font-bold);
    color: var(--color-danger-text);
  }

  .state-body {
    font-size: var(--text-xs);
    color: var(--color-text-tertiary);
    line-height: 1.5;
  }

  .state-meta {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-1);
    margin-top: var(--space-1);
  }

  .meta-chip {
    padding: 1px 8px;
    border-radius: var(--radius-full);
    background: rgba(16, 185, 129, 0.14);
    font-size: var(--text-2xs);
    font-weight: var(--font-medium);
    color: var(--color-success-text);
    white-space: nowrap;
  }

  /* ── Issue list ───────────────────────────────────────────────────── */
  .issue-list {
    display: flex;
    flex-direction: column;
    gap: 1px;
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .issue-row {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    padding: var(--space-1) var(--space-2);
    border-radius: var(--radius-md);
    overflow: hidden;
    width: 100%;
    border: 0;
    background: transparent;
    text-align: left;
    animation: slideInUp var(--duration-normal) var(--ease-out);
  }

  .issue-row:hover {
    background: var(--color-bg-hover);
  }

  .issue-row:disabled {
    cursor: default;
    opacity: 1;
  }

  .issue-row.navigable {
    cursor: pointer;
  }

  .issue-row.navigable:focus-visible {
    outline: none;
    box-shadow: var(--shadow-focus);
  }

  .issue-strip {
    width: 3px;
    align-self: stretch;
    flex-shrink: 0;
    border-radius: 2px;
  }

  .issue-strip.severity-error {
    background: var(--color-danger);
  }

  .issue-strip.severity-warning {
    background: var(--color-warning);
  }

  .issue-strip.severity-info {
    background: var(--color-info);
  }

  /* Text severity tag — carries the signal without relying on color (WCAG 1.4.1) */
  .issue-severity {
    flex-shrink: 0;
    padding: 1px 6px;
    border-radius: var(--radius-sm);
    font-size: 9px;
    font-weight: var(--font-bold);
    text-transform: uppercase;
    letter-spacing: var(--tracking-wider);
    line-height: 1.6;
    white-space: nowrap;
  }

  .issue-severity.severity-error {
    background: var(--color-danger-bg);
    color: var(--color-danger-text);
  }

  .issue-severity.severity-warning {
    background: var(--color-warning-bg);
    color: var(--color-warning-text);
  }

  .issue-severity.severity-info {
    background: var(--color-info-bg);
    color: var(--color-info-text);
  }

  .issue-location {
    flex-shrink: 0;
    font-family: var(--font-mono);
    font-size: var(--text-2xs);
    color: var(--color-text-muted);
  }

  .issue-message {
    flex: 1;
    min-width: 0;
    font-size: var(--text-xs);
    color: var(--color-text-secondary);
  }

  /* ── Keyframes ────────────────────────────────────────────────────── */
  @keyframes slideInUp {
    from {
      opacity: 0;
      transform: translateY(6px);
    }
    to {
      opacity: 1;
      transform: translateY(0);
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .issue-row {
      animation: none;
    }
  }
</style>
