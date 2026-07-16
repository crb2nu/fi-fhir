import { afterEach, describe, expect, it } from 'vitest';
import { get } from 'svelte/store';
import type { IntegrationSessionPreviewMeta } from '$lib/features/integration-session';
import {
  navigateToProblem,
  problemNavigation,
  problemsDiagnostics,
  setSessionDiagnostics
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
