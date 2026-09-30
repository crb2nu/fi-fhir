/**
 * Reads and writes for a reopened Integration Session (.loom/42 E-3): the
 * session itself, its run history, one run in full, a run's diagnostics, the
 * per-run stream, archive, the audited export and accepting a fix.
 *
 * Every call renders its own failure inline (no toast), because the /hl7
 * session sidebar and its dialogs say what went wrong where it happened.
 */
import { graphQLErrorEntries, graphqlFetch } from '$lib/graphql/client';
import { subscribe } from '$lib/graphql/subscriptions';
import {
  AcceptSessionDiagnosticFixDocument,
  ArchiveIntegrationSessionDocument,
  ExportIntegrationSessionBundleDocument,
  IntegrationSessionWorkspaceDocument,
  SessionRunDetailDocument,
  SessionRunDiagnosticsDocument,
  SessionRunHistoryDocument,
  StreamSessionRunEventsDocument,
  type ExportIntegrationSessionBundleMutation,
  type IntegrationSessionRunFieldsFragment,
  type IntegrationSessionWorkspaceQuery,
  type SessionDiagnosticFieldsFragment,
  type SessionRunSummaryFieldsFragment,
  type StreamSessionRunEventsSubscription,
  type StreamSessionRunEventsSubscriptionVariables
} from '$lib/gen/graphql';

const INLINE = { showErrorToast: false } as const;

export type SessionWorkspace = NonNullable<IntegrationSessionWorkspaceQuery['integrationSession']>;
export type SessionRunSummary = SessionRunSummaryFieldsFragment;
export type SessionRunDetail = IntegrationSessionRunFieldsFragment;
export type SessionDiagnosticRow = SessionDiagnosticFieldsFragment;
export type SessionBundle = ExportIntegrationSessionBundleMutation['exportIntegrationBundle'];
export type SessionRunStreamEvent = StreamSessionRunEventsSubscription['sessionRunEvents'];

/** The catalog-safe message the transport gate's refusal carries. */
const FORBIDDEN_MESSAGE = /operation forbidden/i;

/** True when the failure is the transport gate's (or the service's) refusal. */
export function isForbidden(error: unknown): boolean {
  if (graphQLErrorEntries(error).some((entry) => entry.extensions?.['code'] === 'FORBIDDEN')) return true;
  return error instanceof Error && FORBIDDEN_MESSAGE.test(error.message);
}

export type WorkspaceLookup =
  | { kind: 'found'; session: SessionWorkspace }
  | { kind: 'absent' }
  | { kind: 'forbidden' };

/**
 * `integrationSession(id)`: null means this tenant's store holds no such
 * session. A refusal is reported as `forbidden`; any other failure throws.
 */
export async function fetchSessionWorkspace(id: string): Promise<WorkspaceLookup> {
  try {
    const data = await graphqlFetch(IntegrationSessionWorkspaceDocument, { id }, INLINE);
    return data.integrationSession ? { kind: 'found', session: data.integrationSession } : { kind: 'absent' };
  } catch (error) {
    if (isForbidden(error)) return { kind: 'forbidden' };
    throw error;
  }
}

/** `sessionRuns(sessionId)`, newest first. */
export async function fetchSessionRuns(sessionId: string): Promise<SessionRunSummary[]> {
  const data = await graphqlFetch(SessionRunHistoryDocument, { sessionId }, INLINE);
  return [...data.sessionRuns].sort((a, b) => b.createdAt.localeCompare(a.createdAt));
}

/** `sessionRun(id)` with its events, warnings, stages and lineage; null when absent. */
export async function fetchSessionRun(id: string): Promise<SessionRunDetail | null> {
  const data = await graphqlFetch(SessionRunDetailDocument, { id }, INLINE);
  return data.sessionRun;
}

/** `sessionDiagnostics(sessionId, runId)`. */
export async function fetchRunDiagnostics(sessionId: string, runId: string): Promise<SessionDiagnosticRow[]> {
  const data = await graphqlFetch(SessionRunDiagnosticsDocument, { sessionId, runId }, INLINE);
  return data.sessionDiagnostics;
}

/** `archiveIntegrationSession(id)`. The mutation takes no reason and records none. */
export async function archiveSession(id: string): Promise<{ archived: boolean; updatedAt: string }> {
  const data = await graphqlFetch(ArchiveIntegrationSessionDocument, { id }, INLINE);
  return { archived: data.archiveIntegrationSession.archived, updatedAt: data.archiveIntegrationSession.updatedAt };
}

/** `acceptDiagnosticFix`: the server records the verified caller as the acceptor. */
export async function acceptDiagnosticFix(sessionId: string, diagnosticId: string): Promise<SessionDiagnosticRow> {
  const data = await graphqlFetch(
    AcceptSessionDiagnosticFixDocument,
    { input: { sessionId, diagnosticId, acceptedBy: null } },
    INLINE
  );
  return data.acceptDiagnosticFix;
}

export interface ExportRequest {
  sessionId: string;
  reason: string;
  includeRawPayload: boolean;
}

/** `exportIntegrationBundle`: the server records the reason and the caller on the export row. */
export async function exportSessionBundle(request: ExportRequest): Promise<SessionBundle> {
  const data = await graphqlFetch(
    ExportIntegrationSessionBundleDocument,
    { input: { sessionId: request.sessionId, reason: request.reason, includeRawPayload: request.includeRawPayload } },
    INLINE
  );
  return data.exportIntegrationBundle;
}

export interface RunStreamCallbacks {
  onOpen?: () => void;
  onEvent: (event: SessionRunStreamEvent) => void;
  onError?: (error: Error) => void;
  onComplete?: () => void;
}

/** Opens `sessionRunEvents(sessionId, runId)`; returns the unsubscribe. */
export function subscribeRunEvents(sessionId: string, runId: string, callbacks: RunStreamCallbacks): () => void {
  return subscribe<StreamSessionRunEventsSubscription, StreamSessionRunEventsSubscriptionVariables>(
    StreamSessionRunEventsDocument,
    { sessionId, runId },
    {
      ...(callbacks.onOpen ? { onOpen: callbacks.onOpen } : {}),
      onData: (data) => callbacks.onEvent(data.sessionRunEvents),
      ...(callbacks.onError ? { onError: callbacks.onError } : {}),
      ...(callbacks.onComplete ? { onComplete: callbacks.onComplete } : {})
    }
  );
}
