import { derived, writable } from 'svelte/store';
import { workflowDraft } from '$lib/features/workflows/workflowStore';
import { validateWorkflowDraft, type WorkflowDraft } from '$lib/features/workflows/workflowTypes';
import type { IntegrationSessionPreviewMeta } from '$lib/features/integration-session';

export type WorkflowProblemSeverity = 'error' | 'warning' | 'info';

export type WorkflowProblem = {
  id: string;
  source: 'workflow' | 'session';
  scope: 'workflow' | 'route' | 'transform' | 'action' | 'session';
  location: string;
  message: string;
  severity: WorkflowProblemSeverity;
  targetPath: string | null;
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
    source: 'workflow',
    scope: scopeFromLocation(parsed.location),
    location: parsed.location,
    message: parsed.message,
    severity: 'error',
    targetPath: null
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

const sessionProblems = writable<WorkflowProblem[]>([]);

export const problemNavigation = writable<{ path: string; sequence: number } | null>(null);

let navigationSequence = 0;

export function setSessionDiagnostics(session: IntegrationSessionPreviewMeta | null): void {
  if (!session?.runId) {
    sessionProblems.set([]);
    return;
  }
  const deduplicated = new Map<string, WorkflowProblem>();
  for (const diagnostic of session.diagnostics) {
    const id = `${session.runId}:${diagnostic.id}`;
    const targetPath = diagnostic.lineage[0]?.sourcePath || diagnostic.path;
    deduplicated.set(id, {
      id,
      source: 'session',
      scope: 'session',
      location: targetPath || diagnostic.code,
      message: diagnostic.message,
      severity: normalizeSeverity(diagnostic.severity),
      targetPath
    });
  }
  sessionProblems.set([...deduplicated.values()]);
}

export function navigateToProblem(problem: WorkflowProblem): void {
  if (!problem.targetPath) return;
  navigationSequence += 1;
  problemNavigation.set({ path: problem.targetPath, sequence: navigationSequence });
}

function normalizeSeverity(severity: string | null): WorkflowProblemSeverity {
  if (severity === 'error' || severity === 'warning') return severity;
  return 'info';
}

export const problemsDiagnostics = derived(
  [workflowDiagnostics, sessionProblems],
  ([$workflow, $session]) => ({
    ...$workflow,
    issues: [...$session, ...$workflow.issues],
    isValid: $session.length === 0 && $workflow.isValid,
    sessionCount: $session.length
  })
);

/**
 * Severity-banded counts for the workflow draft diagnostics.
 *
 * The Problems-panel tab badge in {@link BottomPanel} consumes this so the badge
 * count tracks the same live signal the panel renders. All issues currently land
 * in the `error` band (see {@link toProblem}); `warning`/`info` are kept so the
 * badge's variant logic stays correct if validation grows softer severities.
 */
export const workflowProblemCounts = derived(
  problemsDiagnostics,
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
