/**
 * Turns the control plane's catalog-safe GraphQL messages into operator
 * guidance.
 *
 * The server deliberately returns a small, fixed vocabulary that names the
 * decision and never the inventory (see `catalogSafeErrorPresenter`). Those
 * strings are correct but terse, and two of them — the optimistic-concurrency
 * conflict and the spent idempotency key — need an explicit next step, because
 * the right response is "reload and retry with the current version", never a
 * silent retry.
 */

export interface OperatorFailure {
  /** Inline-ready message rendered next to the action that failed. */
  message: string;
  /** True when reloading the current record is the correct next step. */
  staleView: boolean;
}

const CATALOG: Record<string, OperatorFailure> = {
  'integration deployment version conflict': {
    message:
      'Another operator changed this deployment first. Reload to pick up the current version, then re-apply your change if it is still needed.',
    staleView: true
  },
  'invalid integration deployment transition': {
    message:
      'That transition is not allowed from the deployment’s current state. Reload to see where it is now.',
    staleView: true
  },
  'delivery attempt is not dead-lettered': {
    message:
      'This attempt is no longer an open dead letter — it may have been replayed or discarded already. Reload to see its current state.',
    staleView: true
  },
  'operator operation idempotency conflict': {
    message:
      'That idempotency key was already used for a different operation. Change the reason (which derives a new key) or supply a distinct key.',
    staleView: false
  },
  'operator control-plane action forbidden': {
    message:
      'Your account does not hold the operator role this action requires. Ask an administrator to grant it.',
    staleView: false
  },
  'operator control-plane record not found': {
    message: 'That record is not available in this tenant.',
    staleView: true
  },
  'invalid operator control-plane request': {
    message: 'The request was rejected as invalid. Check the reason and any filters, then try again.',
    staleView: false
  },
  'operator control plane unavailable': {
    message:
      'The operator control plane is not enabled on this deployment. It requires the durable PostgreSQL delivery and lifecycle stores.',
    staleView: false
  },
  'authentication required': {
    message: 'Your session is no longer authenticated. Sign in again to continue.',
    staleView: false
  }
};

const FALLBACK: OperatorFailure = {
  message: 'The operator control plane could not complete that request.',
  staleView: false
};

/** Maps an unknown thrown value onto inline-ready operator guidance. */
export function describeOperatorFailure(error: unknown): OperatorFailure {
  const raw = extractMessage(error);
  if (!raw) {
    return FALLBACK;
  }
  const normalized = raw.toLowerCase();
  for (const [needle, failure] of Object.entries(CATALOG)) {
    if (normalized.includes(needle)) {
      return failure;
    }
  }
  return { message: raw, staleView: false };
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
