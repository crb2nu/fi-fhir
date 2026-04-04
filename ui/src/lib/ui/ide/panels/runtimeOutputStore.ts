import { writable } from 'svelte/store';
import type { EventStreamSubscription, WorkflowEventsSubscription } from '$lib/gen/graphql';

export type RuntimeOutputSeverity = 'info' | 'warning' | 'error';

export type RuntimeOutputKind = 'workflow' | 'event-stream';

export type RuntimeOutputEntry = {
  id: string;
  timestamp: string;
  severity: RuntimeOutputSeverity;
  kind: RuntimeOutputKind;
  title: string;
  message: string;
  source: string;
  details: string[];
  sessionId?: string;
};

export type RuntimeOutputState = {
  feedKey: string;
  feedLabel: string;
  feedKind: RuntimeOutputKind | null;
  connected: boolean;
  status: 'idle' | 'connecting' | 'connected' | 'error';
  error: string | null;
  entries: RuntimeOutputEntry[];
  updatedAt: string | null;
  activeSessionId: string | null;
};

const MAX_ENTRIES = 100;

const initialState: RuntimeOutputState = {
  feedKey: '',
  feedLabel: 'Runtime output',
  feedKind: null,
  connected: false,
  status: 'idle',
  error: null,
  entries: [],
  updatedAt: null,
  activeSessionId: null,
};

export const runtimeOutputState = writable<RuntimeOutputState>(initialState);

let entrySequence = 0;

function nextEntryId(prefix: string): string {
  entrySequence += 1;
  return `${prefix}-${Date.now()}-${entrySequence}`;
}

function toTitleCase(value: string): string {
  return value
    .toLowerCase()
    .replace(/(?:^|\s|-|_)([a-z])/g, (_, letter: string) => ` ${letter.toUpperCase()}`)
    .trim();
}

function formatCount(count: number, noun: string): string {
  return `${count} ${noun}${count === 1 ? '' : 's'}`;
}

function createWorkflowMessage(notification: WorkflowEventsSubscription['workflowEvents']): {
  severity: RuntimeOutputSeverity;
  message: string;
  details: string[];
} {
  const routeCount = notification.routesMatched.length;
  const actionCount = notification.actionsExecuted.length;
  const routeMessage = routeCount === 0
    ? 'No routes matched'
    : `Matched ${formatCount(routeCount, 'route')}`;
  const actionMessage = actionCount === 0
    ? 'No actions executed'
    : `Executed ${formatCount(actionCount, 'action')}`;

  return {
    severity: routeCount === 0 || actionCount === 0 ? 'warning' : 'info',
    message: `${routeMessage}. ${actionMessage}. Duration ${notification.duration}ms.`,
    details: [
      `Workflow: ${notification.workflow}`,
      `Event source: ${notification.event.source}`,
      `Event timestamp: ${notification.event.timestamp}`
    ]
  };
}

function createEventStreamMessage(event: EventStreamSubscription['eventStream']): {
  severity: RuntimeOutputSeverity;
  message: string;
  details: string[];
} {
  const formatLabel = event.sourceFormat ?? 'Unknown source format';
  const correlationLabel = event.correlationId ? `Correlation ${event.correlationId}` : 'No correlation ID';

  return {
    severity: event.sourceFormat ? 'info' : 'warning',
    message: `${formatLabel} event from ${event.source}. ${correlationLabel}.`,
    details: [
      `Event type: ${toTitleCase(event.type)}`,
      `Source format: ${event.sourceFormat ?? 'unknown'}`,
      `Correlation ID: ${event.correlationId ?? 'n/a'}`
    ]
  };
}

export function describeWorkflowOutput(notification: WorkflowEventsSubscription['workflowEvents']): RuntimeOutputEntry {
  const summary = createWorkflowMessage(notification);
  return {
    id: nextEntryId('workflow'),
    timestamp: notification.event.timestamp,
    severity: summary.severity,
    kind: 'workflow',
    title: `${notification.workflow} · ${toTitleCase(notification.event.type)}`,
    message: summary.message,
    source: notification.event.source,
    details: summary.details
  };
}

export function describeEventStreamOutput(event: EventStreamSubscription['eventStream']): RuntimeOutputEntry {
  const summary = createEventStreamMessage(event);
  return {
    id: nextEntryId('event'),
    timestamp: event.timestamp,
    severity: summary.severity,
    kind: 'event-stream',
    title: toTitleCase(event.type),
    message: summary.message,
    source: event.source,
    details: summary.details
  };
}

export function formatRuntimeOutputTimestamp(timestamp: string): string {
  try {
    return new Date(timestamp).toLocaleTimeString();
  } catch {
    return timestamp;
  }
}

export function activateRuntimeOutputFeed(feedKey: string, feedLabel: string, feedKind: RuntimeOutputKind): void {
  runtimeOutputState.update((state) => {
    if (state.feedKey === feedKey && state.feedKind === feedKind) {
      return {
        ...state,
        feedLabel,
        status: 'connecting',
        connected: false,
        error: null,
      };
    }

    return {
      ...initialState,
      feedKey,
      feedLabel,
      feedKind,
      status: 'connecting',
    };
  });
}

export function markRuntimeOutputConnected(): void {
  runtimeOutputState.update((state) => ({
    ...state,
    connected: true,
    status: 'connected',
    error: null,
  }));
}

export function markRuntimeOutputIdle(): void {
  runtimeOutputState.update((state) => ({
    ...state,
    connected: false,
    status: 'idle',
  }));
}

export function markRuntimeOutputError(message: string): void {
  runtimeOutputState.update((state) => ({
    ...state,
    connected: false,
    status: 'error',
    error: message,
  }));
}

export function appendRuntimeOutputEntry(entry: Omit<RuntimeOutputEntry, 'id'> & { id?: string }): RuntimeOutputEntry {
  let normalized: RuntimeOutputEntry = {
    ...entry,
    id: entry.id ?? nextEntryId(entry.kind),
  };

  runtimeOutputState.update((state) => {
    if (state.activeSessionId && !normalized.sessionId) {
      normalized = { ...normalized, sessionId: state.activeSessionId };
    }
    return {
      ...state,
      connected: true,
      status: 'connected',
      error: null,
      entries: [normalized, ...state.entries].slice(0, MAX_ENTRIES),
      updatedAt: normalized.timestamp,
    };
  });

  return normalized;
}

export function setRuntimeOutputSessionId(sessionId: string | null): void {
  runtimeOutputState.update((state) => ({
    ...state,
    activeSessionId: sessionId,
  }));
}

export function clearRuntimeOutputEntries(): void {
  runtimeOutputState.update((state) => ({
    ...state,
    entries: [],
  }));
}

export function resetRuntimeOutputFeed(): void {
  runtimeOutputState.set(initialState);
}
