/**
 * Per-subscription streaming honesty.
 *
 * GraphQL subscriptions travel as SSE over `POST /graphql`. The API opens a
 * stream only when Integration Session streaming is on
 * (`FI_FHIR_INTEGRATION_SESSION_ENABLED`; otherwise HTTP 404), and even then
 * only for the allowlisted roots `integrationSessionEvents` and
 * `sessionRunEvents` — `eventStream`, `workflowEvents` and `debugStepEvent`
 * are refused by design. The refusal arrives as HTTP 200 `text/event-stream`
 * whose `next` event carries
 * `{"errors":[{"message":"GraphQL operation forbidden","extensions":{"code":"FORBIDDEN"}}]}`:
 * the transport builds "GraphQL stream operation forbidden", but the server's
 * catalog-safe error presenter rewrites every FORBIDDEN-coded message to the
 * generic one before it is written, so the code is what identifies it.
 *
 * Availability of a root therefore comes from two places:
 *  1. the identity's capabilities (`streaming`, and `subscriptions` — the
 *     allowlisted roots for this caller), recorded by the credential gate;
 *  2. what a stream actually answered this page session: a 404 or a refusal
 *     marks the root unavailable, because a status fetched before a redeploy
 *     can be stale.
 *
 * With unknown capabilities (an older API) a root is "unknown": the surface
 * tries once and the classified failure flips it to the honest state.
 */
import { derived, get, writable, type Readable } from 'svelte/store';
import { accessCapabilities, type AccessCapabilityState } from './accessCapabilities';

export type StreamRoot =
  | 'eventStream'
  | 'workflowEvents'
  | 'debugStepEvent'
  | 'integrationSessionEvents'
  | 'sessionRunEvents';

/** Why a root cannot stream. */
export type StreamUnavailableReason =
  /** The API has Integration Session streaming off (HTTP 404 on SSE). */
  | 'streaming-off'
  /** Streaming is on, but this root is not in the caller's allowlist. */
  | 'not-allowlisted';

export type StreamStatus =
  | { availability: 'available' }
  | { availability: 'unknown' }
  | { availability: 'unavailable'; reason: StreamUnavailableReason };

const observed = writable<Partial<Record<StreamRoot, StreamUnavailableReason>>>({});

/**
 * Classifies a subscription failure. Returns the reason when the failure means
 * "this deployment cannot stream this root", or null for an ordinary error
 * (network blip, server error) that a reconnect may fix.
 */
export function classifyStreamError(error: unknown): StreamUnavailableReason | null {
  const message = error instanceof Error ? error.message : typeof error === 'string' ? error : '';
  if (/GraphQL stream HTTP 404\b/.test(message)) return 'streaming-off';
  if (streamErrorCodes(error).includes('FORBIDDEN')) return 'not-allowlisted';
  // Without a code (a string, or an error from elsewhere): the sanitized
  // "GraphQL operation forbidden" and the unsanitized
  // "GraphQL stream operation forbidden" both match.
  if (/operation forbidden/i.test(message)) return 'not-allowlisted';
  return null;
}

/**
 * The `extensions.code` values a stream error carried (`GraphQLStreamError.codes`
 * from `./subscriptions`). Read structurally so this module does not import the
 * SSE client, which many surface tests replace with a mock.
 */
function streamErrorCodes(error: unknown): readonly string[] {
  if (!(error instanceof Error) || !('codes' in error)) return [];
  const codes = (error as { codes: unknown }).codes;
  return Array.isArray(codes) ? codes.filter((code): code is string => typeof code === 'string') : [];
}

/** Records that `root` cannot stream on this deployment. */
export function markStreamUnavailable(root: StreamRoot, reason: StreamUnavailableReason): void {
  observed.update((current) => (current[root] === reason ? current : { ...current, [root]: reason }));
}

/**
 * Feeds a subscription failure through the classifier. Returns true when the
 * root is now known to be unavailable (the caller should stop retrying and let
 * the honest state render), false for an ordinary error.
 */
export function noteStreamError(root: StreamRoot, error: unknown): boolean {
  const reason = classifyStreamError(error);
  if (!reason) return false;
  markStreamUnavailable(root, reason);
  return true;
}

/** Forgets what streams answered (tests; a fresh page load does this too). */
export function resetObservedStreams(): void {
  observed.set({});
}

/** Pure resolution of one root's status. */
export function resolveStreamStatus(
  state: AccessCapabilityState,
  seen: Partial<Record<StreamRoot, StreamUnavailableReason>>,
  root: StreamRoot
): StreamStatus {
  const failure = seen[root];
  if (failure) return { availability: 'unavailable', reason: failure };
  if (state.state !== 'known') return { availability: 'unknown' };
  if (!state.capabilities.streaming) return { availability: 'unavailable', reason: 'streaming-off' };
  const roots = state.capabilities.subscriptions;
  if (roots === null) return { availability: 'unknown' };
  return roots.includes(root)
    ? { availability: 'available' }
    : { availability: 'unavailable', reason: 'not-allowlisted' };
}

/** Reactive status for one subscription root. */
export function streamStatus(root: StreamRoot): Readable<StreamStatus> {
  return derived([accessCapabilities, observed], ([$state, $seen]) =>
    resolveStreamStatus($state, $seen, root)
  );
}

/** Snapshot: may a surface open `root` now? (available or unknown) */
export function canAttemptStream(root: StreamRoot): boolean {
  return resolveStreamStatus(get(accessCapabilities), get(observed), root).availability !== 'unavailable';
}
