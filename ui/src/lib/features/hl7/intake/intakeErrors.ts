/**
 * Turns the intake surface's inventory-safe GraphQL messages (C-2's
 * `intakeConnectionError`, internal/api/graphql/resolvers/connection_intake.go)
 * into guidance rendered inside the dialog or the capture row. The catalog's
 * own refusals keep C-1's wording (`describeConnectionFailure`).
 *
 * Two refusals need a next step the message alone does not give:
 *   - SOURCE_UNAVAILABLE (`extensions.code`): the tap cannot see the source, so
 *     the fix is to mount it or compile a stream source naming it — or, for a
 *     batch source, to peek instead.
 *   - "a capture is already armed for this source": one armed capture per
 *     source, possibly in another session; the row that owns it offers Cancel.
 */
import { graphQLErrorEntries } from '$lib/graphql/client';
import {
  CONNECTION_READ_ROLE,
  describeConnectionFailure
} from '$lib/features/connections/connectionsErrors';

/** What mounts a stream source on a replica, named in the SOURCE_UNAVAILABLE guidance. */
export const STREAM_SOURCE_ENV_KEYS = [
  'FI_FHIR_MLLP_SOURCE_CONFIG_PATH',
  'FI_FHIR_HTTP_INGRESS_*'
] as const;

/** Where a peek resolves a secret binding from (C-2 handoff §2). */
export const PEEK_SECRET_SOURCES = [
  'FI_FHIR_CONNECTION_SECRET_*',
  'FI_FHIR_DELIVERY_IDENTITY_SECRET_DIR/connections/'
] as const;

export const SOURCE_UNAVAILABLE = 'SOURCE_UNAVAILABLE';
export const CAPTURE_ALREADY_ARMED = 'a capture is already armed for this source';

export interface IntakeFailure {
  /** Inline-ready message. */
  message: string;
  /** The refusal's kind, when the surface does more than print the message. */
  kind: 'source-unavailable' | 'already-armed' | 'session' | 'other';
}

const SOURCE_UNAVAILABLE_GUIDANCE = `No replica can see this source: nothing mounts an MLLP listener or HTTP ingress with this source ID, and no compiled MLLP or HTTP source connection names it. Mount it (${STREAM_SOURCE_ENV_KEYS.join(', ')}) or compile a stream source connection with this source ID in Connections. A batch source is sampled with Browse objects instead.`;

const CATALOG: Array<[needle: string, failure: IntakeFailure]> = [
  [CAPTURE_ALREADY_ARMED, {
    message:
      'A capture is already armed for this source (one per source, possibly from another session). Cancel it from its capture row, or wait for it to finish.',
    kind: 'already-armed'
  }],
  ['connection sample intake unavailable', {
    message:
      'Sample intake is unavailable: this deployment has no Integration Session workspace (FI_FHIR_INTEGRATION_SESSION_ENABLED).',
    kind: 'session'
  }],
  ['integration session not found', {
    message: "This page's Integration Session no longer exists. Reload the page to start a new one.",
    kind: 'session'
  }],
  ['integration session is archived', {
    message: "This page's Integration Session is archived and accepts no samples. Reload the page to start a new one.",
    kind: 'session'
  }],
  ['connection capture not found', {
    message: 'That capture is not available in this tenant.',
    kind: 'other'
  }],
  ['connection capture is already finished', {
    message: 'That capture has already finished; its row shows how.',
    kind: 'other'
  }],
  ['peek requires a compiled batch source connection', {
    message:
      'Only a compiled batch_s3 or batch_sftp source connection can be browsed. Compile it in Connections first.',
    kind: 'other'
  }],
  ['invalid connection sample intake request', {
    message:
      'The request was rejected as invalid. Check the reason (1–1024 bytes) and the bounds, then try again.',
    kind: 'other'
  }],
  ['connection sample intake request failed', {
    message: 'Sample intake could not complete that request. Try again; the API log has the cause.',
    kind: 'other'
  }]
];

/** Maps a thrown value from an intake call onto inline guidance. */
export function describeIntakeFailure(error: unknown): IntakeFailure {
  const entries = graphQLErrorEntries(error);
  if (entries.some((entry) => entry.extensions?.['code'] === SOURCE_UNAVAILABLE)) {
    return { message: SOURCE_UNAVAILABLE_GUIDANCE, kind: 'source-unavailable' };
  }
  const raw = error instanceof Error ? error.message : typeof error === 'string' ? error : '';
  const normalized = raw.toLowerCase();
  if (normalized.startsWith('capture source unavailable')) {
    return { message: SOURCE_UNAVAILABLE_GUIDANCE, kind: 'source-unavailable' };
  }
  for (const [needle, failure] of CATALOG) {
    if (normalized.includes(needle)) return failure;
  }
  // The catalog's refusals (forbidden, not found, archived, unavailable) and
  // the operation gate's "GraphQL operation forbidden".
  return { message: describeConnectionFailure(error).message, kind: 'other' };
}

/**
 * One line of guidance for a peek or capture problem (`ConnectionProblem`),
 * beside the server's own message; empty when the code needs none.
 */
export function problemGuidance(code: string): string {
  switch (code) {
    case 'SECRET_UNRESOLVABLE':
      return `A peek resolves a secret binding only from ${PEEK_SECRET_SOURCES[0]} environment references or files under ${PEEK_SECRET_SOURCES[1]}; nothing was contacted.`;
    case 'SOURCE_UNAVAILABLE':
      return 'The provider could not be reached, listed, or opened from this replica.';
    case 'OBJECT_NOT_FOUND':
      return 'The object is no longer listed; list the objects again.';
    case 'MESSAGE_UNREADABLE':
      return 'The object could not be read as HL7v2 messages.';
    case 'SAMPLE_WRITE_FAILED':
      return 'A message could not be written to the session; it is not counted.';
    case 'CAPTURE_COUNT_FAILED':
      return 'A sample reached the session but its count could not be recorded.';
    default:
      return '';
  }
}

/** The role every intake call needs; a caller without it reads no captured text. */
export const INTAKE_ROLE = CONNECTION_READ_ROLE;
