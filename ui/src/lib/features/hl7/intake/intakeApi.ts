/**
 * Sample intake from connections over GraphQL (.loom/38 C-2's surface), one
 * function per operation. The sources the dialog lists are C-1's reads
 * (`fetchEngineRuntime`, `fetchConnections`), imported from connectionsApi.
 *
 * Every failure here has an inline home — the dialog's submit error, the
 * capture row's alert, or an EmptyState with Retry — so every call opts out of
 * the global error toast (toast-budget B4). No call toasts success either: an
 * armed or cancelled capture is its capture row, and a peek's samples are the
 * inbox rows they become, so a toast would repeat what another layer shows.
 */
import { graphqlFetch } from '$lib/graphql/client';
import {
  CancelConnectionCaptureDocument,
  ConnectionCapturesDocument,
  IntakeSessionSamplesDocument,
  PeekBatchConnectionDocument,
  StartConnectionCaptureDocument,
  type ConnectionCaptureFieldsFragment,
  type IntakeSessionSampleFieldsFragment,
  type PeekBatchConnectionInput,
  type PeekBatchConnectionMutation,
  type StartConnectionCaptureInput
} from '$lib/gen/graphql';

const INLINE_ERRORS = { showErrorToast: false } as const;

/** One audited peek or stream capture. */
export type ConnectionCaptureRow = ConnectionCaptureFieldsFragment;
/** A session sample as the inbox takes it (never `rawPayload`). */
export type IntakeSessionSample = IntakeSessionSampleFieldsFragment;
export type BatchPeekResultView = PeekBatchConnectionMutation['peekBatchConnection'];
export type BatchPeekObjectRow = BatchPeekResultView['objects'][number];
export type IntakeProblem = BatchPeekResultView['problems'][number];

export type { PeekBatchConnectionInput, StartConnectionCaptureInput };

/** One session's peeks and captures, newest first (at most 100). */
export async function fetchConnectionCaptures(sessionId: string): Promise<ConnectionCaptureRow[]> {
  const result = await graphqlFetch(ConnectionCapturesDocument, { sessionId }, INLINE_ERRORS);
  return result.connectionCaptures;
}

/** Every sample of the session; the caller keeps the captured and peeked ones. */
export async function fetchIntakeSessionSamples(sessionId: string): Promise<IntakeSessionSample[]> {
  const result = await graphqlFetch(IntakeSessionSamplesDocument, { sessionId }, INLINE_ERRORS);
  return result.sessionSamples;
}

/**
 * Lists a batch source's objects (no `objectPath`) or reads one object's first
 * messages into the session. Either way it is an audited, reason-required
 * peek; outcomes after the audit row exists are `problems`, not errors.
 */
export async function peekBatchConnection(input: PeekBatchConnectionInput): Promise<BatchPeekResultView> {
  const result = await graphqlFetch(PeekBatchConnectionDocument, { input }, INLINE_ERRORS);
  return result.peekBatchConnection;
}

export async function startConnectionCapture(input: StartConnectionCaptureInput): Promise<ConnectionCaptureRow> {
  const result = await graphqlFetch(StartConnectionCaptureDocument, { input }, INLINE_ERRORS);
  return result.startConnectionCapture;
}

export async function cancelConnectionCapture(id: string, reason: string): Promise<ConnectionCaptureRow> {
  const result = await graphqlFetch(CancelConnectionCaptureDocument, { id, reason }, INLINE_ERRORS);
  return result.cancelConnectionCapture;
}
