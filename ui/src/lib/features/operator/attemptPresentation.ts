/**
 * Presentation rules for durable delivery attempts and dead letters.
 *
 * Pure so the tables, badges, and — critically — the *preconditions* for every
 * control action can be unit tested. Per the toast-budget policy (.loom/22,
 * B2), a control the operator cannot use is disabled with an explanatory
 * `title` rather than allowed to fire and then rejected by a toast.
 */

export type BadgeVariant = 'default' | 'primary' | 'success' | 'warning' | 'danger' | 'info';

export interface AttemptLike {
  status: string;
  outboxStatus: string;
  deadLetter?: { active: boolean; resolution: string } | null;
}

/** Maps a durable attempt status onto the shared Badge variants. */
export function attemptStatusVariant(status: string): BadgeVariant {
  switch (status) {
    case 'succeeded':
      return 'success';
    case 'failed':
      return 'danger';
    case 'queued':
      return 'info';
    default:
      return 'default';
  }
}

/** Maps a durable outbox status onto the shared Badge variants. */
export function outboxStatusVariant(status: string): BadgeVariant {
  switch (status) {
    case 'published':
      return 'success';
    case 'failed':
      return 'danger';
    case 'leased':
      return 'warning';
    case 'pending':
      return 'info';
    default:
      return 'default';
  }
}

/** Maps a circuit state onto the shared Badge variants. */
export function circuitStateVariant(state: string): BadgeVariant {
  return state === 'open' ? 'danger' : 'success';
}

/**
 * Describes what happened to a dead letter in operator language. An entry with
 * no recorded resolution is still open; the server guarantees a closed entry
 * always carries one.
 */
export function deadLetterStateLabel(entry: AttemptLike['deadLetter']): string {
  if (!entry) {
    return 'Never dead-lettered';
  }
  if (entry.active) {
    return 'Awaiting operator decision';
  }
  switch (entry.resolution) {
    case 'replayed':
      return 'Closed by replay';
    case 'resubmitted':
      return 'Closed by resubmit';
    case 'discarded':
      return 'Discarded by an operator';
    default:
      return 'Closed';
  }
}

export type DeliveryAction = 'replay' | 'resubmit' | 'discard';

/**
 * Returns null when the action is available, otherwise the reason it is not.
 *
 * The string is used as the disabled control's `title`, so a dead click never
 * happens and the operator still learns why. This mirrors the server's
 * precondition exactly: every recovery action requires an *active* dead letter.
 */
export function deliveryActionBlockedReason(
  attempt: AttemptLike | null,
  action: DeliveryAction
): string | null {
  if (!attempt) {
    return 'Select a delivery attempt first.';
  }
  if (!attempt.deadLetter) {
    return `Only a dead-lettered attempt can be ${pastTense(action)}. This attempt never entered the dead-letter queue.`;
  }
  if (!attempt.deadLetter.active) {
    return `This dead letter is already resolved (${deadLetterStateLabel(attempt.deadLetter).toLowerCase()}).`;
  }
  return null;
}

function pastTense(action: DeliveryAction): string {
  switch (action) {
    case 'replay':
      return 'replayed';
    case 'resubmit':
      return 'resubmitted';
    case 'discard':
      return 'discarded';
  }
}

export type DeploymentAction = 'pause' | 'resume' | 'retire' | 'deploy';

/**
 * Returns null when a lifecycle command is available for the current state,
 * otherwise why it is not. This encodes the closed Slice 2.1 state machine:
 * published -> deployed | retired; deployed -> paused | retired;
 * paused -> deployed | retired.
 */
export function deploymentActionBlockedReason(
  state: string | null,
  action: DeploymentAction
): string | null {
  if (!state) {
    return 'Select a deployment first.';
  }
  const allowed: Record<DeploymentAction, string[]> = {
    deploy: ['published'],
    pause: ['deployed'],
    resume: ['paused'],
    retire: ['published', 'deployed', 'paused']
  };
  if (allowed[action].includes(state)) {
    return null;
  }
  return `Cannot ${action} an integration in the "${state}" state. Allowed from: ${allowed[action].join(', ')}.`;
}

/** Maps a lifecycle state onto the shared Badge variants. */
export function deploymentStateVariant(state: string): BadgeVariant {
  switch (state) {
    case 'deployed':
      return 'success';
    case 'paused':
      return 'warning';
    case 'retired':
      return 'danger';
    case 'published':
    case 'approved':
      return 'primary';
    default:
      return 'default';
  }
}

/** Maps reported deployment health onto the shared Badge variants. */
export function deploymentHealthVariant(health: string): BadgeVariant {
  switch (health) {
    case 'healthy':
      return 'success';
    case 'degraded':
      return 'warning';
    case 'unhealthy':
      return 'danger';
    case 'starting':
      return 'info';
    default:
      return 'default';
  }
}

/** Shortens a `sha256:...` digest for dense tables without losing identity. */
export function shortDigest(digest: string): string {
  const value = digest.startsWith('sha256:') ? digest.slice('sha256:'.length) : digest;
  return value.length <= 12 ? value : `${value.slice(0, 12)}…`;
}

/** Formats a durable timestamp for display, tolerating absent values. */
export function formatTimestamp(value: string | null | undefined): string {
  if (!value) {
    return '—';
  }
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) {
    return value;
  }
  return parsed.toISOString().replace('T', ' ').replace(/\.\d+Z$/, 'Z');
}
