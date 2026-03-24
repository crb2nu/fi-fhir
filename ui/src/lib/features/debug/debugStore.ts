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
}

export function updateSessionState(state: DebugSessionState): void {
  debugSession.update((s) => (s ? { ...s, state } : null));
}

export function addStep(step: DebugStep): void {
  debugSession.update((s) =>
    s ? { ...s, steps: [...s.steps, step], state: 'paused' } : null
  );
}

export function addBreakpoint(bp: Breakpoint): void {
  debugSession.update((s) =>
    s ? { ...s, breakpoints: [...s.breakpoints, bp] } : null
  );
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

// Initialize with mock data for development
export function loadMockData(): void {
  debugSession.set(mockSession);
  traceSpans.set(mockTraceSpans);
  eventLineage.set(mockEventLineage);
}
