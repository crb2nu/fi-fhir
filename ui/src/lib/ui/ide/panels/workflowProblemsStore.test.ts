import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { get } from 'svelte/store';
import type { IntegrationSessionPreviewMeta } from '$lib/features/integration-session';
import {
  isEmptyDefaultDraft,
  markWorkflowBuilderOpened,
  resetWorkflowBuilderOpened,
  workflowDraft
} from '$lib/features/workflows/workflowStore';
import {
  navigateToProblem,
  problemNavigation,
  problemsDiagnostics,
  setSessionDiagnostics,
  workflowDiagnostics,
  workflowDraftLive,
  workflowProblemCounts
} from './workflowProblemsStore';

function sessionMeta(): IntegrationSessionPreviewMeta {
  return {
    mode: 'session',
    id: 'session-1',
    sampleId: 'sample-1',
    runId: 'run-1',
    state: 'completed',
    diagnostics: [
      {
        id: 'diag-1',
        code: 'INVALID_IDENTIFIER',
        message: 'Identifier failed validation',
        path: 'PID-3',
        severity: 'warning',
        fixSuggestion: null,
        accepted: false,
        acceptedAt: null,
        runId: 'run-1',
        lineage: [
          {
            sourcePath: 'PID-3[0].1',
            targetPath: 'event.patient.identifiers[0]',
            description: null
          }
        ]
      }
    ],
    stages: [],
    lineage: [],
    streamState: 'complete',
    error: null
  };
}

afterEach(() => {
  setSessionDiagnostics(null);
  problemNavigation.set(null);
  workflowDraft.reset();
  resetWorkflowBuilderOpened();
});

describe('draft liveness', () => {
  beforeEach(() => {
    workflowDraft.reset();
    resetWorkflowBuilderOpened();
  });

  it('a fresh session with the empty default draft contributes no problems', () => {
    expect(isEmptyDefaultDraft(get(workflowDraft))).toBe(true);
    expect(get(workflowDraftLive)).toBe(false);
    expect(get(workflowProblemCounts).total).toBe(0);
    expect(get(problemsDiagnostics).draftLive).toBe(false);
    expect(get(problemsDiagnostics).isValid).toBe(true);
    // The draft is still invalid — it just is not anyone's work yet.
    expect(get(workflowDiagnostics).isValid).toBe(false);
  });

  it('counts the draft once the builder has been opened this session', () => {
    markWorkflowBuilderOpened();
    expect(get(workflowDraftLive)).toBe(true);
    expect(get(workflowProblemCounts).total).toBe(get(workflowDiagnostics).issues.length);
    expect(get(workflowProblemCounts).total).toBeGreaterThan(0);
  });

  it('counts a draft that differs from the empty default (e.g. restored from storage)', () => {
    workflowDraft.update((draft) => ({ ...draft, name: 'adt-routing' }));
    expect(isEmptyDefaultDraft(get(workflowDraft))).toBe(false);
    expect(get(workflowDraftLive)).toBe(true);
    expect(get(workflowProblemCounts).total).toBeGreaterThan(0);
  });

  it('still reports session diagnostics on a fresh session', () => {
    setSessionDiagnostics(sessionMeta());
    expect(get(workflowProblemCounts).total).toBe(1);
    expect(get(problemsDiagnostics).issues.every((issue) => issue.source === 'session')).toBe(true);
  });
});

describe('session problems', () => {
  it('deduplicates run diagnostics and prefers canonical lineage for navigation', () => {
    const session = sessionMeta();
    session.diagnostics.push({ ...session.diagnostics[0]!, message: 'Latest diagnostic copy' });

    setSessionDiagnostics(session);

    const issues = get(problemsDiagnostics).issues.filter((issue) => issue.source === 'session');
    expect(issues).toHaveLength(1);
    expect(issues[0]).toMatchObject({
      id: 'run-1:diag-1',
      message: 'Latest diagnostic copy',
      targetPath: 'PID-3[0].1'
    });

    navigateToProblem(issues[0]!);
    expect(get(problemNavigation)).toMatchObject({ path: 'PID-3[0].1' });
  });

  it('clears stale diagnostics when no server run is active', () => {
    setSessionDiagnostics(sessionMeta());
    setSessionDiagnostics(null);

    expect(get(problemsDiagnostics).issues.filter((issue) => issue.source === 'session')).toEqual([]);
  });
});
