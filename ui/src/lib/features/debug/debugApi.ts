import type { DebugSession, DebugStep, Breakpoint } from './types';
import { mockSession, mockSteps } from './debugMocks';

const USE_MOCKS = true; // Toggle for standalone dev

export async function startDebugSession(
  workflowYaml: string,
  event: unknown
): Promise<DebugSession> {
  void workflowYaml;
  void event;
  if (USE_MOCKS) {
    return { ...mockSession, state: 'paused', steps: [mockSteps[0]] };
  }
  // TODO: Wire to GraphQL mutation startDebugSession
  throw new Error('Not implemented');
}

export async function debugStep(
  sessionId: string
): Promise<DebugStep | null> {
  void sessionId;
  if (USE_MOCKS) {
    return mockSteps[1] ?? null;
  }
  throw new Error('Not implemented');
}

export async function debugContinue(
  sessionId: string
): Promise<DebugStep | null> {
  void sessionId;
  if (USE_MOCKS) {
    return mockSteps[mockSteps.length - 1] ?? null;
  }
  throw new Error('Not implemented');
}

export async function setBreakpoint(
  sessionId: string,
  type: string,
  name: string
): Promise<Breakpoint> {
  void sessionId;
  if (USE_MOCKS) {
    return {
      id: `bp-${Date.now()}`,
      type: type as Breakpoint['type'],
      name,
      enabled: true
    };
  }
  throw new Error('Not implemented');
}

export async function removeBreakpointApi(
  sessionId: string,
  breakpointId: string
): Promise<boolean> {
  void sessionId;
  void breakpointId;
  if (USE_MOCKS) return true;
  throw new Error('Not implemented');
}

export async function endDebugSession(
  sessionId: string
): Promise<boolean> {
  void sessionId;
  if (USE_MOCKS) return true;
  throw new Error('Not implemented');
}
