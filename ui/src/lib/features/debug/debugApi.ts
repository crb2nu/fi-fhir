import { graphqlFetch } from '$lib/graphql/client';
import { subscribe as wsSubscribe } from '$lib/graphql/subscriptions';
import {
  StartDebugSessionDocument,
  DebugStepDocument,
  DebugContinueDocument,
  DebugSetBreakpointDocument,
  DebugRemoveBreakpointDocument,
  DebugEndSessionDocument,
  DebugStepEventDocument,
  DebugSessionQueryDocument,
  WorkflowRunTraceDocument,
  type StartDebugSessionMutation,
  type DebugStepMutation,
  type DebugContinueMutation,
  type DebugSetBreakpointMutation
} from '$lib/gen/graphql';
import type { DebugSession, DebugStep, Breakpoint, TraceSpan } from './types';

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

export function subscribeDebugStepEvent(
  sessionId: string,
  callbacks: { onData: (step: DebugStep) => void; onError?: (err: Error) => void }
): () => void {
  const subCallbacks: import('$lib/graphql/subscriptions').SubscriptionCallbacks<
    import('$lib/gen/graphql').DebugStepEventSubscription
  > = {
    onData: (data) => {
      if (!data.debugStepEvent) return;
      const raw = data.debugStepEvent;
      callbacks.onData({
        stepNumber: raw.stepNumber,
        kind: raw.kind as DebugStep['kind'],
        name: raw.name,
        variables: toVariables(raw.variables),
        timestamp: raw.timestamp,
        spanName: raw.spanName
      });
    },
  };
  if (callbacks.onError) {
    subCallbacks.onError = callbacks.onError;
  }
  return wsSubscribe(DebugStepEventDocument, { sessionId }, subCallbacks);
}

export async function fetchDebugSession(id: string): Promise<DebugSession | null> {
  const data = await graphqlFetch(DebugSessionQueryDocument, { id }, { showErrorToast: false });
  if (!data.debugSession) return null;
  return normalizeSession(data.debugSession as StartDebugSessionMutation['startDebugSession']);
}

export async function fetchWorkflowRunTrace(runId: string): Promise<TraceSpan[]> {
  const data = await graphqlFetch(WorkflowRunTraceDocument, { runId }, { showErrorToast: false });
  return data.workflowRunTrace.map((span) => ({
    id: span.id,
    name: span.name,
    parentId: span.parentId,
    startTime: span.startTime,
    endTime: span.endTime,
    status: span.status as TraceSpan['status'],
    attributes: (span.attributes as Record<string, unknown>) ?? {},
    events: span.events.map((ev) => ({
      name: ev.name,
      timestamp: ev.timestamp,
      attributes: (ev.attributes as Record<string, unknown>) ?? {}
    }))
  }));
}
