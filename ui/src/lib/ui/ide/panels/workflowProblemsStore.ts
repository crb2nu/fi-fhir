import { derived } from 'svelte/store';
import { workflowDraft } from '$lib/features/workflows/workflowStore';
import { validateWorkflowDraft, type WorkflowDraft } from '$lib/features/workflows/workflowTypes';

export type WorkflowProblemSeverity = 'error' | 'warning' | 'info';

export type WorkflowProblem = {
  id: string;
  scope: 'workflow' | 'route' | 'transform' | 'action';
  location: string;
  message: string;
  severity: WorkflowProblemSeverity;
};

export type WorkflowDiagnostics = {
  draft: WorkflowDraft;
  issues: WorkflowProblem[];
  isValid: boolean;
  routeCount: number;
  actionCount: number;
  transformCount: number;
};

function scopeFromLocation(location: string): WorkflowProblem['scope'] {
  if (location.includes('transform')) return 'transform';
  if (location.includes('action')) return 'action';
  if (location.startsWith('Route ')) return 'route';
  return 'workflow';
}

function messageFromIssue(issue: string): { location: string; message: string } {
  const separator = issue.indexOf(': ');
  if (separator < 0) {
    return { location: 'Workflow', message: issue };
  }

  return {
    location: issue.slice(0, separator),
    message: issue.slice(separator + 2)
  };
}

function toProblem(issue: string): WorkflowProblem {
  const parsed = messageFromIssue(issue);
  return {
    id: issue,
    scope: scopeFromLocation(parsed.location),
    location: parsed.location,
    message: parsed.message,
    severity: 'error'
  };
}

export type WorkflowProblemCounts = {
  error: number;
  warning: number;
  info: number;
  total: number;
};

export const workflowDiagnostics = derived(workflowDraft, ($draft): WorkflowDiagnostics => {
  const issues = validateWorkflowDraft($draft).map(toProblem);

  return {
    draft: $draft,
    issues,
    isValid: issues.length === 0,
    routeCount: $draft.routes.length,
    actionCount: $draft.routes.reduce((count, route) => count + route.actions.length, 0),
    transformCount: $draft.routes.reduce((count, route) => count + route.transforms.length, 0)
  };
});

/**
 * Severity-banded counts for the workflow draft diagnostics.
 *
 * The Problems-panel tab badge in {@link BottomPanel} consumes this so the badge
 * count tracks the same live signal the panel renders. All issues currently land
 * in the `error` band (see {@link toProblem}); `warning`/`info` are kept so the
 * badge's variant logic stays correct if validation grows softer severities.
 */
export const workflowProblemCounts = derived(
  workflowDiagnostics,
  ($diag): WorkflowProblemCounts => {
    let error = 0;
    let warning = 0;
    let info = 0;

    for (const issue of $diag.issues) {
      if (issue.severity === 'error') error += 1;
      else if (issue.severity === 'warning') warning += 1;
      else info += 1;
    }

    return { error, warning, info, total: $diag.issues.length };
  }
);
