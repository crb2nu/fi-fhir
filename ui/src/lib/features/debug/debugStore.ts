import { writable, derived } from 'svelte/store';
import type {
  DebugSession,
  Breakpoint,
  DebugStep,
  TraceSpan,
  EventLineageNode,
  DebugSessionState
} from './types';
import { mockSession, mockTraceSpans, mockEventLineage } from './debugMocks';
import { subscribeDebugStepEvent, fetchWorkflowRunTrace } from './debugApi';

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
}

// Session state
export const debugSession = writable<DebugSession | null>(null);
export const traceSpans = writable<TraceSpan[]>([]);
export const eventLineage = writable<EventLineageNode[]>([]);

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
}

// Subscription-based live step delivery
export function subscribeToSession(sessionId: string): () => void {
  return subscribeDebugStepEvent(sessionId, {
    onData: (step) => addStep(step),
    onError: (err) => console.error('[debug] subscription error:', err.message)
  });
}

// Load real trace spans from persisted workflow run
export async function loadRealTraceSpans(runId: string): Promise<void> {
  const spans = await fetchWorkflowRunTrace(runId);
  if (spans.length > 0) {
    traceSpans.set(spans);
  }
}

// Initialize with mock data for development
export function loadMockData(): void {
  debugSession.set(mockSession);
  traceSpans.set(mockTraceSpans);
  eventLineage.set(mockEventLineage);
}
