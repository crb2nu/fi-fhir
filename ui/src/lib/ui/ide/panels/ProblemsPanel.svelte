<script lang="ts">
  import CircleAlert from '@lucide/svelte/icons/circle-alert';
  import CircleCheck from '@lucide/svelte/icons/circle-check';
  import Info from '@lucide/svelte/icons/info';
  import { Badge, Icon } from '$lib/ui/primitives';
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
  // Fresh session: no draft opened or edited, no session run — nothing to check yet.
  let nothingChecked = $derived(isValid && !$problemsDiagnostics.draftLive);
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

  const SEVERITY_TONES: Record<WorkflowProblemSeverity, 'danger' | 'warning' | 'info'> = {
    error: 'danger',
    warning: 'warning',
    info: 'info',
  };
</script>

<div class="problems-panel">
  {#if nothingChecked}
    <!-- Fresh session: say where problems come from instead of inventing any -->
    <div class="summary" data-testid="problems-empty">
      <Icon icon={Info} size={14} class="summary-icon" />
      <p class="summary-text">
        <span class="summary-title">No problems</span>
        <span class="summary-body">
          Problems come from two places: validation of the workflow draft you edit in the
          Workflows builder, and diagnostics from Integration Session runs in HL7 intake.
          Nothing has been opened or run in this session yet.
        </span>
      </p>
    </div>
  {:else if isValid}
    <div class="summary state-ok">
      <Icon icon={CircleCheck} size={14} class="summary-icon" />
      <p class="summary-text">
        <span class="summary-title">Ready for runtime verification</span>
        <span class="summary-body">No blocking problems detected.</span>
      </p>
      <span class="summary-meta">
        <Badge mono>{pluralize($problemsDiagnostics.routeCount, 'route')}</Badge>
        <Badge mono>{pluralize($problemsDiagnostics.actionCount, 'action')}</Badge>
        {#if $problemsDiagnostics.transformCount > 0}
          <Badge mono>{pluralize($problemsDiagnostics.transformCount, 'transform')}</Badge>
        {/if}
      </span>
    </div>
  {:else}
    <div class="summary state-attention">
      <Icon icon={CircleAlert} size={14} class="summary-icon" />
      <p class="summary-text">
        <span class="summary-title">
          {$problemsDiagnostics.sessionCount > 0 ? 'Session diagnostics need attention' : 'Workflow draft needs attention'}
        </span>
        <span class="summary-count">{countSummary()}</span>
        <span class="summary-body">
          {$problemsDiagnostics.sessionCount > 0
            ? 'Select a server diagnostic to inspect its exact HL7 source field.'
            : 'Fix the listed issues before you trust the destination behavior.'}
        </span>
      </p>
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
            <span class="issue-severity">
              <Badge tone={SEVERITY_TONES[issue.severity]}>{severityLabel(issue.severity)}</Badge>
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
  .problems-panel {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    font-size: var(--text-ui);
  }

  /* One summary line: icon, title, counts, one sentence. No card. */
  .summary {
    display: flex;
    align-items: flex-start;
    gap: var(--space-2);
    color: var(--color-text-secondary);
  }

  .summary :global(.summary-icon) {
    margin-top: 3px;
    color: var(--color-text-tertiary);
  }

  .state-ok :global(.summary-icon) {
    color: var(--color-success-text);
  }

  .state-attention :global(.summary-icon) {
    color: var(--color-danger-text);
  }

  .summary-text {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    column-gap: var(--space-2);
    row-gap: 2px;
    min-width: 0;
    margin: 0;
    line-height: var(--leading-ui);
  }

  .summary-title {
    color: var(--color-text-primary);
    font-weight: var(--font-semibold);
  }

  .summary-count {
    color: var(--color-danger-text);
    font-family: var(--font-mono);
    font-size: var(--text-mono);
  }

  .summary-body {
    color: var(--color-text-tertiary);
    font-size: var(--text-xs);
  }

  .summary-meta {
    display: flex;
    gap: var(--space-1);
    margin-left: auto;
  }

  /* Issues: dense rows — severity, location (mono), message. */
  .issue-list {
    display: grid;
    margin: 0;
    padding: 0;
    list-style: none;
    border-top: 1px solid var(--color-border-subtle);
  }

  .issue-row {
    display: grid;
    grid-template-columns: 72px minmax(96px, 200px) minmax(0, 1fr);
    align-items: center;
    gap: var(--space-3);
    width: 100%;
    min-height: 28px;
    padding: 2px var(--space-2);
    border: none;
    border-bottom: 1px solid var(--color-border-subtle);
    background: transparent;
    color: var(--color-text-secondary);
    font: inherit;
    text-align: left;
  }

  .issue-row:disabled {
    cursor: default;
    opacity: 1;
  }

  .issue-row.navigable {
    cursor: pointer;
  }

  .issue-row.navigable:hover {
    background: var(--color-bg-hover);
    color: var(--color-text-primary);
  }

  .issue-row.navigable:focus-visible {
    outline: 2px solid var(--color-focus-ring);
    outline-offset: -2px;
  }

  .issue-location {
    min-width: 0;
    overflow: hidden;
    color: var(--color-text-tertiary);
    font-family: var(--font-mono);
    font-size: var(--text-mono);
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .issue-message {
    min-width: 0;
    color: var(--color-text-primary);
  }

  @media (max-width: 640px) {
    .issue-row {
      grid-template-columns: 72px minmax(0, 1fr);
    }

    .issue-message {
      grid-column: 1 / -1;
    }
  }
</style>
