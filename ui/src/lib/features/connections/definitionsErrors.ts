/**
 * The definition authoring surface's inventory-safe GraphQL messages
 * (definitionAuthoringError in internal/api/graphql/resolvers/
 * definition_authoring.go) turned into operator guidance. A version conflict
 * says "reload, then re-decide", as the Operator page does.
 */
import { CONNECTION_READ_ROLE, CONNECTION_WRITE_ROLE } from './connectionsErrors';

export interface DefinitionFailure {
  message: string;
  /** Reloading the definition is the correct next step. */
  staleView: boolean;
}

const CATALOG: Array<[needle: string, failure: DefinitionFailure]> = [
  [
    'integration definition version conflict',
    {
      message: 'Another change to this definition was recorded first. Reload, then re-decide.',
      staleView: true
    }
  ],
  [
    'invalid integration definition transition',
    {
      message: 'The definition is no longer in a state that allows this step. Reload, then re-decide.',
      staleView: true
    }
  ],
  [
    'current connection validation required',
    {
      message:
        'Validation evidence is missing, failed, or expired. Validate the definition again, then retry this step.',
      staleView: true
    }
  ],
  [
    'another revision of this definition is deployed or paused',
    {
      message: 'Another revision of this definition is deployed or paused. Retire it on the Operator page first.',
      staleView: false
    }
  ],
  [
    'real validation is unavailable for this source on this replica',
    {
      message:
        'This replica cannot contact this source: REAL runs only for the batch source it mounts. Run `fi-fhir lifecycle seed --validate real` where the source is mounted, or choose STATIC or SKIP.',
      staleView: false
    }
  ],
  [
    'a real validation is already running on this replica',
    { message: 'A REAL validation is already running on this replica. Try again in a few seconds.', staleView: false }
  ],
  [
    'integration definition not found',
    { message: 'That definition revision is not available in this tenant. Reload the list.', staleView: true }
  ],
  [
    'integration definition authoring forbidden',
    {
      message: `This identity does not hold the roles this action requires: ${CONNECTION_READ_ROLE} to read definitions, and ${CONNECTION_WRITE_ROLE} as well to author them.`,
      staleView: false
    }
  ],
  [
    'graphql operation forbidden',
    {
      message: `This identity's roles do not admit this operation: ${CONNECTION_READ_ROLE} reads definitions and ${CONNECTION_WRITE_ROLE} authors them.`,
      staleView: false
    }
  ],
  [
    'invalid integration definition request',
    {
      message: 'The request was rejected as invalid. Check the identifiers and the reason (skipping needs at least 16 characters).',
      staleView: false
    }
  ],
  [
    'integration definition authoring unavailable',
    {
      message:
        'Definition authoring is not configured on this deployment: it needs the lifecycle catalog, the connection catalog, and the static integration registry.',
      staleView: false
    }
  ],
  ['authentication required', { message: 'Your session is no longer authenticated. Sign in again to continue.', staleView: false }],
  [
    'integration definition request failed',
    { message: 'The definition catalog could not complete that request. Try again; the API log has the cause.', staleView: false }
  ]
];

export function describeDefinitionFailure(error: unknown): DefinitionFailure {
  const raw = error instanceof Error ? error.message : typeof error === 'string' ? error : '';
  if (!raw) return { message: 'The definition catalog could not complete that request.', staleView: false };
  const normalized = raw.toLowerCase();
  for (const [needle, failure] of CATALOG) {
    if (normalized.includes(needle)) return failure;
  }
  return { message: raw, staleView: false };
}
