import { graphqlFetch } from '$lib/graphql/client';
import {
  StartDebugSessionDocument,
  DebugStepDocument,
  DebugContinueDocument,
  DebugSetBreakpointDocument,
  DebugRemoveBreakpointDocument,
  DebugEndSessionDocument,
  type StartDebugSessionMutation,
  type DebugStepMutation,
  type DebugContinueMutation,
  type DebugSetBreakpointMutation
} from '$lib/gen/graphql';
import type { DebugSession, DebugStep, Breakpoint } from './types';

function toVariables(value: unknown): Record<string, unknown> {
  if (!value || typeof value !== 'object' || Array.isArray(value)) {
    return {};
  }
  return value as Record<string, unknown>;
}

function normalizeStep(
  step:
    | StartDebugSessionMutation['startDebugSession']['steps'][number]
    | NonNullable<DebugStepMutation['debugStep']>
    | NonNullable<DebugContinueMutation['debugContinue']>
): DebugStep {
  return {
    stepNumber: step.stepNumber,
    kind: step.kind as DebugStep['kind'],
    name: step.name,
    variables: toVariables(step.variables),
    timestamp: step.timestamp,
    spanName: step.spanName
  };
}

function normalizeBreakpoint(
  breakpoint: StartDebugSessionMutation['startDebugSession']['breakpoints'][number] | DebugSetBreakpointMutation['debugSetBreakpoint']
): Breakpoint {
  return {
    id: breakpoint.id,
    type: breakpoint.type as Breakpoint['type'],
    name: breakpoint.name,
    enabled: breakpoint.enabled
  };
}

function normalizeSession(session: StartDebugSessionMutation['startDebugSession']): DebugSession {
  return {
    id: session.id,
    workflowId: session.workflowId,
    state: session.state as DebugSession['state'],
    breakpoints: session.breakpoints.map(normalizeBreakpoint),
    steps: session.steps.map(normalizeStep),
    createdAt: session.createdAt
  };
}

export async function startDebugSession(
  workflowYaml: string,
  event: unknown
): Promise<DebugSession> {
  const data = await graphqlFetch(StartDebugSessionDocument, {
    input: {
      workflowYaml,
      event: toVariables(event)
    }
  });
  return normalizeSession(data.startDebugSession);
}

export async function debugStep(
  sessionId: string
): Promise<DebugStep | null> {
  const data = await graphqlFetch(DebugStepDocument, { sessionId });
  return data.debugStep ? normalizeStep(data.debugStep) : null;
}

export async function debugContinue(
  sessionId: string
): Promise<DebugStep | null> {
  const data = await graphqlFetch(DebugContinueDocument, { sessionId });
  return data.debugContinue ? normalizeStep(data.debugContinue) : null;
}

export async function setBreakpoint(
  sessionId: string,
  type: string,
  name: string
): Promise<Breakpoint> {
  const data = await graphqlFetch(DebugSetBreakpointDocument, {
    input: { sessionId, type, name }
  });
  return normalizeBreakpoint(data.debugSetBreakpoint);
}

export async function removeBreakpointApi(
  sessionId: string,
  breakpointId: string
): Promise<boolean> {
  const data = await graphqlFetch(DebugRemoveBreakpointDocument, {
    sessionId,
    breakpointId
  });
  return data.debugRemoveBreakpoint;
}

export async function endDebugSession(
  sessionId: string
): Promise<boolean> {
  const data = await graphqlFetch(DebugEndSessionDocument, { sessionId });
  return data.debugEndSession;
}
