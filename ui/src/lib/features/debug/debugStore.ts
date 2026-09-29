import { writable, derived, get } from 'svelte/store';
import type {
  DebugSession,
  Breakpoint,
  DebugStep,
  TraceSpan,
  EventLineageNode,
  DebugSessionState
} from './types';
import { subscribeDebugStepEvent, fetchWorkflowRunTrace } from './debugApi';
import { canAttemptStream, noteStreamError } from '$lib/graphql/streamAvailability';

function deriveTraceSpansFromSession(session: DebugSession | null): TraceSpan[] {
  if (!session || session.steps.length === 0) return [];

  return session.steps.map((step, index) => ({
    id: `trace-${session.id}-${step.stepNumber}`,
    name: step.spanName,
    parentId: index === 0 ? null : `trace-${session.id}-${session.steps[index - 1]!.stepNumber}`,
    startTime: step.timestamp,
    endTime: step.timestamp,
    status: 'ok',
    attributes: step.variables,
    events: []
  }));
}

function deriveEventLineageFromSession(session: DebugSession | null): EventLineageNode[] {
  if (!session) return [];

  const current = session.steps[session.steps.length - 1];
  if (!current) {
    return [
      {
        stage: 'workflow',
        label: session.workflowId || 'Workflow debug session',
        detail: 'Session initialized',
        status: 'pending'
      }
    ];
  }

  const eventType = typeof current.variables['event.type'] === 'string'
    ? current.variables['event.type']
    : 'Unknown event';
  const eventSource = typeof current.variables['event.source'] === 'string'
    ? current.variables['event.source']
    : 'debug-ui';

  return [
    {
      stage: 'source',
      label: eventSource,
      detail: 'Debug event input',
      status: 'success'
    },
    {
      stage: 'events',
      label: eventType,
      detail: `${session.steps.length} debug step${session.steps.length === 1 ? '' : 's'} recorded`,
      status: 'success'
    },
    {
      stage: 'workflow',
      label: session.workflowId || 'workflow',
      detail: `Current state: ${session.state}`,
      status: session.state === 'completed' ? 'success' : 'pending'
    },
    {
      stage: 'actions',
      label: current.name,
      detail: current.spanName,
      status: 'success'
    }
  ];
}

function syncDerivedArtifacts(session: DebugSession | null): void {
  traceSpans.set(deriveTraceSpansFromSession(session));
  eventLineage.set(deriveEventLineageFromSession(session));
  traceSource.set(session ? { kind: 'debug-session', sessionId: session.id } : { kind: 'none' });
}

/**
 * Where the Trace panel's spans came from, so it can say what it shows and,
 * when a workflow run has none, why.
 */
export type TraceSource =
  | { kind: 'none' }
  | { kind: 'debug-session'; sessionId: string }
  | {
      kind: 'workflow-run';
      runId: string;
      state: 'loading' | 'loaded' | 'error';
      message?: string;
    };

// Session state
export const debugSession = writable<DebugSession | null>(null);
export const traceSpans = writable<TraceSpan[]>([]);
export const eventLineage = writable<EventLineageNode[]>([]);
export const traceSource = writable<TraceSource>({ kind: 'none' });

// Derived state
export const sessionState = derived(
  debugSession,
  ($session) => $session?.state ?? 'idle'
);

export const currentStep = derived(debugSession, ($session) => {
  if (!$session || $session.steps.length === 0) return null;
  return $session.steps[$session.steps.length - 1];
});

export const breakpoints = derived(
  debugSession,
  ($session) => $session?.breakpoints ?? []
);

export const stepHistory = derived(
  debugSession,
  ($session) => $session?.steps ?? []
);

// Actions
export function startSession(session: DebugSession): void {
  debugSession.set(session);
  syncDerivedArtifacts(session);
}

export function updateSessionState(state: DebugSessionState): void {
  debugSession.update((s) => {
    const next = s ? { ...s, state } : null;
    syncDerivedArtifacts(next);
    return next;
  });
}

export function addStep(step: DebugStep): void {
  debugSession.update((s) => {
    const next = s
      ? { ...s, steps: [...s.steps, step], state: 'paused' as const }
      : null;
    syncDerivedArtifacts(next);
    return next;
  });
}

export function addBreakpoint(bp: Breakpoint): void {
  debugSession.update((s) =>
    s ? { ...s, breakpoints: [...s.breakpoints, bp] } : null
  );
}

export function replaceBreakpoint(previousId: string, bp: Breakpoint): void {
  debugSession.update((s) => {
    if (!s) return null;
    return {
      ...s,
      breakpoints: s.breakpoints.map((existing) =>
        existing.id === previousId ? bp : existing
      )
    };
  });
}

export function removeBreakpoint(id: string): void {
  debugSession.update((s) =>
    s
      ? { ...s, breakpoints: s.breakpoints.filter((bp) => bp.id !== id) }
      : null
  );
}

export function toggleBreakpoint(id: string): void {
  debugSession.update((s) => {
    if (!s) return null;
    return {
      ...s,
      breakpoints: s.breakpoints.map((bp) =>
        bp.id === id ? { ...bp, enabled: !bp.enabled } : bp
      )
    };
  });
}

export function endSession(): void {
  debugSession.set(null);
  traceSpans.set([]);
  eventLineage.set([]);
  traceSource.set({ kind: 'none' });
}

// Subscription-based live step delivery. Returns null (and opens nothing) when
// this deployment cannot stream `debugStepEvent`; Step/Continue still work
// because each step is fetched on request.
export function subscribeToSession(sessionId: string): (() => void) | null {
  if (!canAttemptStream('debugStepEvent')) return null;
  return subscribeDebugStepEvent(sessionId, {
    onData: (step) => addStep(step),
    onError: (err) => {
      if (noteStreamError('debugStepEvent', err)) return;
      console.error('[debug] subscription error:', err.message);
    }
  });
}

let traceLoadSequence = 0;

/** True while the panel still shows the load `token` started for `runId`. */
function stillShowing(runId: string, token: number): boolean {
  const current = get(traceSource);
  return token === traceLoadSequence && current.kind === 'workflow-run' && current.runId === runId;
}

/**
 * Loads a recorded workflow run's spans (`workflowRunTrace`) into the Trace
 * panel, replacing whatever it showed. An empty answer is kept as empty: the
 * panel says the run has no spans on this API process rather than showing a
 * previous trace. Never throws; a failure is recorded on `traceSource`.
 *
 * A late answer is dropped when the panel has moved on (another run opened,
 * the same run reopened, or a debug session started), so it can never be
 * labelled with a run it does not belong to.
 */
export async function loadRealTraceSpans(runId: string): Promise<void> {
  const token = ++traceLoadSequence;
  traceSpans.set([]);
  traceSource.set({ kind: 'workflow-run', runId, state: 'loading' });
  try {
    const spans = await fetchWorkflowRunTrace(runId);
    if (!stillShowing(runId, token)) return;
    traceSpans.set(spans);
    traceSource.set({ kind: 'workflow-run', runId, state: 'loaded' });
  } catch (err) {
    if (!stillShowing(runId, token)) return;
    traceSource.set({
      kind: 'workflow-run',
      runId,
      state: 'error',
      message: err instanceof Error ? err.message : 'The trace could not be loaded.'
    });
  }
}
