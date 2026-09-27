/**
 * Turns the connection catalog's inventory-safe GraphQL messages into
 * operator guidance.
 *
 * The server returns a small, fixed vocabulary (`catalogConnectionError` in
 * internal/api/graphql/resolvers/connection_catalog.go): not-found and
 * forbidden stay distinct, another tenant's connection reads as not-found, and
 * an unconfigured catalog is indistinguishable from a missing capability here —
 * `/api/auth/status` tells those apart, and the page reads that first. Two of
 * them need an explicit next step: a version conflict (reload, then re-apply)
 * and an archived connection (it accepts no change).
 *
 * The raw word "forbidden" is never rendered: it tells an operator nothing
 * about what to grant.
 */

import { graphQLErrorEntries } from '$lib/graphql/client';

/** The catalog refusal that carries `extensions.problems` (C-0 server.go). */
export const SPEC_REJECTED_MESSAGE = 'connection spec carries secret material';

/** What turns the operator control plane, and with it the catalog, on. */
export const CATALOG_PREREQUISITE_KEYS = ['FI_FHIR_DATABASE_*', 'FI_FHIR_OPERATOR_CONTROL_PLANE_ENABLED=true'] as const;

export interface ConnectionFailure {
  /** Inline-ready message rendered next to the action that failed. */
  message: string;
  /** True when reloading the connection is the correct next step. */
  staleView: boolean;
}

export const CONNECTION_READ_ROLE = 'integration.operator';
export const CONNECTION_WRITE_ROLE = 'integration.deployment.operator';
export const CONTROL_PLANE_ENV_KEY = 'FI_FHIR_OPERATOR_CONTROL_PLANE_ENABLED';

const ROLE_REFUSAL: ConnectionFailure = {
  message: `This identity's roles do not admit this operation: ${CONNECTION_READ_ROLE} reads connections and ${CONNECTION_WRITE_ROLE} changes them.`,
  staleView: false
};

const CATALOG: Array<[needle: string, failure: ConnectionFailure]> = [
  [
    'connection version conflict',
    {
      message:
        'Another change to this connection was saved first. Reload it to see the current version, then re-apply your edits if they are still needed.',
      staleView: true
    }
  ],
  [
    'connection is archived',
    {
      message: 'This connection is archived and accepts no change. Reload to see its current state.',
      staleView: true
    }
  ],
  [
    'connection already exists',
    {
      message: 'A connection with this ID already exists. Choose another ID.',
      staleView: false
    }
  ],
  [
    'connection not found',
    {
      message: 'That connection is not available in this tenant. It may have been removed; reload the list.',
      staleView: true
    }
  ],
  [
    'connection catalog action forbidden',
    {
      message: `This identity does not hold the role this action requires: ${CONNECTION_READ_ROLE} to read connections, and ${CONNECTION_WRITE_ROLE} as well to change them.`,
      staleView: false
    }
  ],
  ['graphql operation forbidden', ROLE_REFUSAL],
  [
    'invalid connection catalog request',
    {
      message:
        'The request was rejected as invalid. Check the ID, name, description and reason, then try again.',
      staleView: false
    }
  ],
  [
    SPEC_REJECTED_MESSAGE,
    {
      message:
        'Nothing was saved: the spec carries what the catalog refuses to store (a secret value, key material, or a key it does not know). The marked fields say where; name a secret binding instead of a value.',
      staleView: false
    }
  ],
  [
    'connection catalog unavailable',
    {
      message: `The connection catalog is unavailable: it is not configured on this deployment (it needs the PostgreSQL submission store, ${CATALOG_PREREQUISITE_KEYS[0]}, or ${CATALOG_PREREQUISITE_KEYS[1]}), or this identity cannot use it.`,
      staleView: false
    }
  ],
  [
    'engine runtime unavailable',
    {
      message: 'This API process has no engine runtime description. Only fi-fhir serve reports what it mounted.',
      staleView: false
    }
  ],
  [
    'authentication required',
    {
      message: 'Your session is no longer authenticated. Sign in again to continue.',
      staleView: false
    }
  ],
  [
    'connection catalog request failed',
    {
      message: 'The connection catalog could not complete that request. Try again; the API log has the cause.',
      staleView: false
    }
  ]
];

const FALLBACK: ConnectionFailure = {
  message: 'The connection catalog could not complete that request.',
  staleView: false
};

/**
 * The dialog message for a write refused with field problems. It is built
 * from the problems themselves — the catalog says the refusal's message is
 * not what a surface should render — and the problems are marked on the form.
 */
export function describeRefusal(problems: ReadonlyArray<{ code: string; path: string }>): string {
  const count = problems.length;
  const base = `Nothing was saved: the catalog refused ${count} field${count === 1 ? '' : 's'} of this connection; they are marked on the form.`;
  if (problems.some((problem) => problem.code === 'SECRET_VALUE_FORBIDDEN')) {
    return `${base} A spec names a secret binding, never a secret value.`;
  }
  if (problems.some((problem) => problem.path.startsWith('secret_bindings'))) {
    return `${base} Complete or remove the secret bindings marked in Secrets.`;
  }
  if (problems.some((problem) => problem.code === 'UNBOUND_SECRET')) {
    return `${base} Each binding field must name a binding declared in Secrets.`;
  }
  return base;
}

/**
 * Whether a problem is one a draft write is refused for (C-0 writeProblems):
 * secret material, a key the kind does not define, a `*_binding` field that
 * names no declared binding, or a malformed secret binding reference.
 * Everything else — a missing or out-of-range value, an unused binding — is
 * a compile problem, and the draft still saves.
 */
export function refusedAtWrite(problem: { code: string; path: string }): boolean {
  if (
    problem.code === 'SECRET_VALUE_FORBIDDEN' ||
    problem.code === 'UNKNOWN_FIELD' ||
    problem.code === 'UNBOUND_SECRET'
  ) {
    return true;
  }
  const binding = problem.path === 'secret_bindings' || problem.path.startsWith('secret_bindings[');
  return binding && problem.code !== 'UNUSED_BINDING';
}

/** Maps an unknown thrown value onto inline-ready guidance. */
export function describeConnectionFailure(error: unknown): ConnectionFailure {
  const raw = extractMessage(error);
  if (!raw) {
    return FALLBACK;
  }
  const normalized = raw.toLowerCase();
  for (const [needle, failure] of CATALOG) {
    if (normalized.includes(needle)) {
      return failure;
    }
  }
  if (normalized.includes('forbidden')) {
    // A refusal this catalog does not name: say what to grant, never the word.
    return ROLE_REFUSAL;
  }
  return { message: raw, staleView: false };
}

/**
 * The field problems of a refused draft write (`extensions.problems`, each
 * `{code, path, message}` — secret material, an unknown key, a malformed
 * binding), or null when the error carries none. Every path was computed from
 * the caller's own spec, so they land on the form's fields like any
 * validation problem, whatever their code.
 */
export function specRejectionProblems(
  error: unknown
): Array<{ code: string; path: string; message: string }> | null {
  const entries = graphQLErrorEntries(error);
  const entry =
    entries.find((candidate) => Array.isArray(candidate.extensions?.['problems'])) ??
    entries.find((candidate) => candidate.message === SPEC_REJECTED_MESSAGE);
  if (!entry) return null;
  const listed = entry.extensions?.['problems'];
  if (!Array.isArray(listed)) return [];
  return listed.flatMap((problem: unknown) => {
    if (typeof problem !== 'object' || problem === null) return [];
    const { code, path, message } = problem as Record<string, unknown>;
    return [
      {
        code: typeof code === 'string' ? code : 'INVALID_VALUE',
        path: typeof path === 'string' ? path : '',
        message: typeof message === 'string' ? message : 'was refused'
      }
    ];
  });
}

function extractMessage(error: unknown): string {
  if (error instanceof Error && error.message) {
    return error.message;
  }
  if (typeof error === 'string' && error.length > 0) {
    return error;
  }
  return '';
}
